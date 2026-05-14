package mysql

import (
	"context"
	stderrors "errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/huing/cat/server/internal/repo/tx"
)

// UserChest 是 user_chests 表的 GORM domain struct（数据库设计 §5.6 +
// migrations/0005）。
//
// 字段语义：
//   - Status: TINYINT NOT NULL；§6.7 钦定 1=counting / 2=unlockable / 3=opening...
//     Story 4.6 首次创建固定 1（counting，倒计时中）
//   - UnlockAt: DATETIME(3) NOT NULL；可开启时间点。Story 4.6 钦定 = now() + 10min
//     **必须用 UTC**（time.Now().UTC()），与 V1 §2.5 钦定的 ISO 8601 UTC 下发对齐。
//   - OpenCostSteps: INT UNSIGNED NOT NULL DEFAULT 1000；开启所需步数（节点 7 才消费）
//   - Version: BIGINT UNSIGNED；乐观锁版本号
//
// **唯一约束**：UNIQUE KEY uk_user_id (user_id) —— 一个用户始终只有一个"当前宝箱"。
// Story 4.6 首次登录创建一行；future 开箱后会 UPDATE 同一行 status / unlock_at（不删
// 不增），所以 uk_user_id 永远不会冲突。
type UserChest struct {
	ID            uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	UserID        uint64    `gorm:"column:user_id"`
	Status        int8      `gorm:"column:status"`
	UnlockAt      time.Time `gorm:"column:unlock_at"`
	OpenCostSteps uint32    `gorm:"column:open_cost_steps"`
	Version       uint64    `gorm:"column:version"`
	CreatedAt     time.Time `gorm:"column:created_at;autoCreateTime:milli"`
	UpdatedAt     time.Time `gorm:"column:updated_at;autoUpdateTime:milli"`
}

// TableName 显式声明 "user_chests"。
func (UserChest) TableName() string { return "user_chests" }

// ChestRepo 是 user_chests 表的访问接口（节点 2 阶段 Create + FindByUserID；节点 7 开箱 epic
// 才加 UpdateStatus 等）。
type ChestRepo interface {
	// Create 插入一行 user_chests。GORM 在成功后回填 c.ID。
	Create(ctx context.Context, c *UserChest) error

	// FindByUserID 查指定 user 的当前宝箱（uk_user_id 唯一约束保证 ≤ 1 行）。
	//
	// NotFound → ErrChestNotFound 哨兵；其他 DB 异常透传给 service 由 service 包成
	// 1009（ADR-0006 三层映射）。
	//
	// 节点 2 阶段消费方：home_service.LoadHome（Story 4.8）；节点 7 chest_service.GetCurrent
	// （Story 20.5）也会消费 —— 与 home_service 共享同一查询，避免双查。
	FindByUserID(ctx context.Context, userID uint64) (*UserChest, error)

	// FindByUserIDForUpdate 取 user_chests 行并对该行加排他锁（FOR UPDATE）。Story 20.6 引入。
	//
	// **必须在事务内调用**（caller 传入的 ctx 必须是 txMgr.WithTx 注入的 txCtx）；
	// 事务外调用时 FOR UPDATE 锁会被 driver 立即释放（autocommit 模式下任何 SELECT
	// 完成即 commit），等同于普通 SELECT，违反 V1 §7.2.5c "FOR UPDATE 行锁串行化" 钦定。
	// ADR-0007 §2.4 钦定 fn 内全部 repo 调用用 txCtx。
	//
	// GORM clause.Locking{Strength: "UPDATE"} 路径生成 SELECT ... FOR UPDATE SQL；
	// 与 room_repo.FindByIDForUpdate 同模式。
	//
	// 找不到 → 返 ErrChestNotFound 哨兵（与 FindByUserID 共用同一哨兵；
	// service 层 errors.Is 后翻译为 4001 ErrChestNotFound）。
	FindByUserIDForUpdate(ctx context.Context, userID uint64) (*UserChest, error)

	// Delete 按 id 删除 user_chests 行。Story 20.6 引入；用于 V1 §7.2.5i 刷新下一轮 chest
	// （先 DELETE 旧 chest 再 INSERT 新 chest，避免与 uk_user_id UNIQUE 索引冲突）。
	//
	// **必须在事务内调用**（与同事务 INSERT 新 chest 一起原子提交）；事务外调用 = race
	// 风险（删完到 INSERT 之间用户可能读到 "chest 不存在" 中间态）。
	//
	// rows_affected 不做返回值约束（理论必删 1 行 —— 同事务前面 FindByUserIDForUpdate 已
	// 拿到这一行；rows_affected=0 = 上游调用顺序错乱，service 不感知该信号，由后续 INSERT
	// 或 commit 期失败兜底）。
	//
	// query 失败 → 返 raw error 透传（service 包成 1009）。
	Delete(ctx context.Context, id uint64) error

	// UpdateUnlockAtByID 把指定 chestID 的 user_chests 行的 unlock_at 列更新为 newUnlockAt。Story 20.7 引入；
	// Story 20.7 review r1 [P2] 改造（原 UpdateUnlockAt(userID) → UpdateUnlockAtByID(chestID)）。
	//
	// **r1 [P2] 改造原因（并发 race）**：原 `WHERE user_id = ?` 实装在与 `/chest/open` 并发时
	// 会跑偏到刷新出来的 next chest 行。具体 race：
	//   T0  client A: POST /chest/open 进入事务 → FOR UPDATE 锁住 chest row id=X
	//   T1  client B: POST /dev/force-unlock-chest → UPDATE WHERE user_id=? **阻塞**在 X 锁上
	//   T2  A 事务内 Delete(id=X) + Create(new chest id=Y) → uk_user_id 仍 = user_id
	//   T3  A commit → 锁释放
	//   T4  B 的 UPDATE 恢复 → WHERE user_id=? **匹配到 Y**（new chest）→ 把下一轮 chest 直接推到 unlock_at=now
	//   T5  用户拿到"连开 2 次"的非预期效果（dev 端点本意只 unlock current chest）
	//
	// **修复模式**：调用方必须先 `FindByUserIDForUpdate` 同事务内拿到当前 chest 的 id（FOR UPDATE 拿锁），
	// 再调本方法 `WHERE id = ?`。FOR UPDATE 锁让 B 阻塞到 A commit 之后，commit 后 SELECT 拿到的
	// 一定是 commit 后的"当前"chest（即新插入的 Y），B 再 UPDATE id=Y 也是把"当前"chest 推到 now —— 这是
	// 用户希望的"force-unlock current"语义，**不再**误伤"未来"chest（因为 SELECT 是"读当前快照"）。
	//
	// **不**改 status / version / open_cost_steps 任一字段（Story 20.7 dev force-unlock 业务语义：
	// 只动时间字段，让 Story 20.5 GET /chest/current 动态判定 "unlock_at <= now → unlockable" 生效）。
	//
	// **不**接 expectedVersion 乐观锁（dev 端点是"故意旁路"，与 Story 20.6 OpenChest 持锁路径独立；
	// 不参与 version+1 串行化）。
	//
	// **rows_affected 语义**：
	//   - rows_affected=1 → UPDATE 成功 → 返 nil
	//   - rows_affected=0 → chestID 不存在（理论上 service 内 FindByUserIDForUpdate 成功拿到后到本
	//     UPDATE 之间被并发 OpenChest 删除是不可能的 —— FOR UPDATE 锁覆盖到 commit）→ 返 ErrChestNotFound
	//     哨兵；service 层 errors.Is 后翻译为 1003 ErrResourceNotFound
	//     （epics.md §20.7 行 2947 钦定 "用户无 chest → 1003" 而非业务 4001）
	//   - 其他 DB error（连接断 / SQL 错 / 死锁等）→ raw error 透传（service 层包成 1009）
	//
	// **必须用 UTC 时间**：调用方传入的 newUnlockAt 必须是 UTC（time.Now().UTC()）；
	// 与 chest.UnlockAt 字段 UTC 语义对齐（chest_repo.go 顶部注释钦定，Story 4.6 / 20.5 / 20.6 同源）。
	// repo 层**不**做 UTC 强转，由 service 层保证；本约束在 service.ForceUnlockChest 落地。
	//
	// **必须在事务内调用**（与 FindByUserIDForUpdate 同事务）；事务外调用 = race 风险
	// （详见上面 r1 [P2] race 分析）。当前唯一调用方 dev_chest_service.ForceUnlockChest 在
	// txManager.WithTx 闭包内调用。
	UpdateUnlockAtByID(ctx context.Context, chestID uint64, newUnlockAt time.Time) error
}

// chestRepo 是 ChestRepo 的默认实装。
type chestRepo struct {
	db *gorm.DB
}

// NewChestRepo 构造 ChestRepo。
func NewChestRepo(db *gorm.DB) ChestRepo {
	return &chestRepo{db: db}
}

// Create 插入一行 user_chests。
//
// **关键**：调用方（service.firstTimeLogin）必须用 time.Now().UTC().Add(10*time.Minute)
// 作为 UnlockAt —— 不能用 time.Now()（带本地时区） —— V1 §2.5 钦定 ISO 8601 UTC，
// 客户端按 UTC 解析，存非 UTC 字面量到 DATETIME(3) 会让客户端解析时多出时区偏差。
// 该约束在 service 层落地，repo 不再校验（repo 只做 CRUD）。
func (r *chestRepo) Create(ctx context.Context, c *UserChest) error {
	db := tx.FromContext(ctx, r.db)
	return db.WithContext(ctx).Create(c).Error
}

// FindByUserID 查 (user_id) 的当前宝箱行。
//
// 走 uk_user_id 唯一索引（数据库设计 §5.6）；查不到返 ErrChestNotFound 哨兵；
// 其他 DB error 透传给 service 由 service 包成 1009（ADR-0006）。
//
// 节点 2 阶段 user_chests 表登录初始化必建一行；查不到 = 数据脏（service 层
// 视为 1009）。Story 4.6 firstTimeLogin 与本方法形成"写入 → 读取"对子。
func (r *chestRepo) FindByUserID(ctx context.Context, userID uint64) (*UserChest, error) {
	db := tx.FromContext(ctx, r.db)
	var c UserChest
	err := db.WithContext(ctx).Where("user_id = ?", userID).First(&c).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrChestNotFound
		}
		return nil, err
	}
	return &c, nil
}

// FindByUserIDForUpdate 实装：SELECT ... FOR UPDATE 走 clause.Locking{Strength: "UPDATE"}。
//
// 走 uk_user_id 唯一索引（与 FindByUserID 同）；事务内锁该行，让并发 OpenChest 串行化。
func (r *chestRepo) FindByUserIDForUpdate(ctx context.Context, userID uint64) (*UserChest, error) {
	db := tx.FromContext(ctx, r.db)
	var c UserChest
	err := db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("user_id = ?", userID).
		First(&c).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrChestNotFound
		}
		return nil, err
	}
	return &c, nil
}

// Delete 实装：DELETE FROM user_chests WHERE id = ?。
func (r *chestRepo) Delete(ctx context.Context, id uint64) error {
	db := tx.FromContext(ctx, r.db)
	return db.WithContext(ctx).Delete(&UserChest{}, id).Error
}

// UpdateUnlockAtByID 实装：UPDATE user_chests SET unlock_at = ? WHERE id = ?。
//
// 走 PRIMARY KEY id 索引（取代原 WHERE user_id 的 uk_user_id 索引）；rows_affected=0 → ErrChestNotFound 哨兵。
// 详见 interface doc 的 r1 [P2] race 分析 —— `WHERE id` 取代 `WHERE user_id` 的核心动机是防止
// 与 OpenChest 的 Delete+Create 刷新链路并发时跑偏到 next chest。
//
// **GORM 实装**：用 `gorm.DB.Model(&UserChest{}).Where("id = ?", chestID).Update("unlock_at", newUnlockAt)`
// 而非 `.Save(&chest)` —— Save 会写入全部字段（含 version / status / open_cost_steps）+
// 触发 GORM autoUpdateTime:milli 自动更新 updated_at；Update("unlock_at", ...) 只动 unlock_at +
// updated_at（GORM 会自动加 updated_at）+ rows_affected 通过 result.RowsAffected 取值精确。
func (r *chestRepo) UpdateUnlockAtByID(ctx context.Context, chestID uint64, newUnlockAt time.Time) error {
	db := tx.FromContext(ctx, r.db)
	result := db.WithContext(ctx).Model(&UserChest{}).Where("id = ?", chestID).Update("unlock_at", newUnlockAt)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrChestNotFound
	}
	return nil
}
