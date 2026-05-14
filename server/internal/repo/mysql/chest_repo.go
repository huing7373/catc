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

	// FindByID 按 chestID 查 user_chests 行（走 PRIMARY KEY）。Story 20.7 review r2 [P2] 引入。
	//
	// **为何加这个方法（不走 FindByUserID）**：
	// dev_chest_service.ForceUnlockChest 在 r2 改造后接 client 传入的 chestID 参数 —— client 必须
	// 先 GET /chest/current 拿到当前 chest.id，再 POST /dev/force-unlock-chest 带这个 id 来。
	// service 用 FindByID 校验 (a) chest 存在 (b) chest.user_id == claimedUserID（防越权 unlock
	// 他人 chest）—— 然后 UPDATE WHERE id 直接动 unlock_at。
	//
	// **不**用 FOR UPDATE（与 FindByUserIDForUpdate 区分）：
	// 旧 r1 实装试图用"FOR UPDATE SELECT 拿 id → UPDATE WHERE id"两步同事务模式来防 race，
	// 但 FOR UPDATE 阻塞结束后 SELECT 返回的是 commit 后的"当前 chest"（即 OpenChest 刚 INSERT
	// 的 next chest Y），跟 r1 之前一样跑偏到 next chest。r2 的彻底解：把"哪个 chest"决策权
	// 交给 client（GET /chest/current 时刻 client 知道是哪个 id），server 只负责"按这个 id
	// unlock"，不再做"current = 调用时刻看到的 chest"的盲打猜测。
	//
	// **race 不再成立**：chest 一旦绑定 user_id，user_id 字段不会改；UPDATE WHERE id 走 PK，
	// chest 在 SELECT 后被 OpenChest 删除时 UPDATE 影响 0 行也不算错（service 不看 RowsAffected）。
	// 极端场景：client 拿到 chest.id=X 后，X 被另一并发 OpenChest 删除并 INSERT Y → dev 端点
	// UPDATE WHERE id=X 命中 0 行 → 视为成功（陈旧的 X 已不复存在，dev 端点已无须维护"current"语义）。
	//
	// NotFound（PK 不存在）→ ErrChestNotFound 哨兵；service 层 errors.Is 翻译为 1003。
	FindByID(ctx context.Context, chestID uint64) (*UserChest, error)

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
	// 历经 r1 [P2]（→ UpdateUnlockAtByID(chestID)）和 r2 [P2]（彻底放弃 RowsAffected 语义）两次改造。
	//
	// # 历史改造路径
	//
	// **r0 → r1 改造**：原 `WHERE user_id = ?` 实装在与 `/chest/open` 并发时跑偏到 next chest
	// （commit 后 user_id 匹配的是新插入的 chest Y）。r1 改成 `WHERE id = ?`，并在 service 层加 FOR UPDATE
	// 拿 id 后再 UPDATE。
	//
	// **r1 → r2 改造**：r1 的"FOR UPDATE SELECT 拿 id"在 OpenChest commit 后看到的 id **也是新 Y**
	// （FOR UPDATE 锁释放后 SELECT 返回 commit 后的快照），race 没真正修好。r2 的彻底解：把"哪个 chest"
	// 决策权交给 client —— client 先 GET /chest/current 拿到当前 chest.id，再 POST 这个 id；
	// server 不再猜"current"语义，service 用 FindByID 校验存在性 + 归属，UPDATE 不看 RowsAffected。
	//
	// # rows_affected 语义（r2 改造后）
	//
	// **不再依赖 RowsAffected 区分 NotFound** —— r2 之前的实装依赖 "rows_affected=0 → ErrChestNotFound"
	// 翻译为 1003，但有两个陷阱：
	//   1. MySQL UPDATE 在值未变时也返 rows_affected=0（默认连接行为 changedRows 而非 matchedRows）。
	//      同一毫秒两次 dev force-unlock 同一 chest → 第二次 unlock_at 与现值相同 → rows_affected=0
	//      → **误判**为 ChestNotFound → 误返 1003。
	//   2. r2 把"chest 是否存在"语义前置到 service.FindByID —— 此处 UPDATE 影响 0 行已合法（极端场景：
	//      client 拿到 id=X 后，X 被并发 OpenChest 删除，UPDATE WHERE id=X 命中 0 行，dev 语义视为成功）。
	//
	// **r2 实装**：
	//   - DB error（连接断 / SQL 错 / 死锁等）→ raw error 透传（service 层包成 1009）
	//   - 否则 → 返 nil（不再区分 rows_affected 0/1）
	//
	// # 不改的字段
	//
	// **不**改 status / version / open_cost_steps 任一字段（Story 20.7 dev force-unlock 业务语义：
	// 只动时间字段，让 Story 20.5 GET /chest/current 动态判定 "unlock_at <= now → unlockable" 生效）。
	//
	// **不**接 expectedVersion 乐观锁（dev 端点是"故意旁路"，与 Story 20.6 OpenChest 持锁路径独立；
	// 不参与 version+1 串行化）。
	//
	// # 时区
	//
	// **必须用 UTC 时间**：调用方传入的 newUnlockAt 必须是 UTC（time.Now().UTC()）；
	// 与 chest.UnlockAt 字段 UTC 语义对齐（chest_repo.go 顶部注释钦定，Story 4.6 / 20.5 / 20.6 同源）。
	// repo 层**不**做 UTC 强转，由 service 层保证；本约束在 service.ForceUnlockChest 落地。
	//
	// # 事务边界
	//
	// **不强制事务**（r2 改造后）：service 用 FindByID（非 FOR UPDATE）做存在性 + 归属校验后直接 UPDATE，
	// chest 一旦绑定 user 不会改 user_id；UPDATE 走 PK 索引。r2 之前的 r1 实装强制事务（FOR UPDATE + UPDATE
	// 同事务），但事务对 race 修复**无作用**（详见上面历史路径），徒增复杂度，r2 移除。
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

// FindByID 实装：SELECT * FROM user_chests WHERE id = ? LIMIT 1。Story 20.7 review r2 [P2] 引入。
//
// 走 PRIMARY KEY id 索引；NotFound → ErrChestNotFound 哨兵。**不**走 FOR UPDATE
// （与 FindByUserIDForUpdate 区分；dev_chest_service r2 改造放弃事务 + FOR UPDATE 模式，
// 详见 interface doc r2 [P2] 改造说明）。
func (r *chestRepo) FindByID(ctx context.Context, chestID uint64) (*UserChest, error) {
	db := tx.FromContext(ctx, r.db)
	var c UserChest
	err := db.WithContext(ctx).Where("id = ?", chestID).First(&c).Error
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
// 走 PRIMARY KEY id 索引。
//
// **r2 [P2] 改造**：不再检查 RowsAffected → 不再返 ErrChestNotFound 哨兵。
// MySQL UPDATE 在值未变时 rows_affected=0（默认连接 changedRows 而非 matchedRows），
// 会让"同一毫秒重复 unlock 同 chest"被误判为 ChestNotFound → 误返 1003。
// 存在性 + 归属校验在 service 层用 FindByID 前置完成；本方法只做"UPDATE 这个 PK"。
//
// **GORM 实装**：用 `gorm.DB.Model(&UserChest{}).Where("id = ?", chestID).Update("unlock_at", newUnlockAt)`
// 而非 `.Save(&chest)` —— Save 会写入全部字段（含 version / status / open_cost_steps）+
// 触发 GORM autoUpdateTime:milli 自动更新 updated_at；Update("unlock_at", ...) 只动 unlock_at +
// updated_at（GORM 会自动加 updated_at）。
func (r *chestRepo) UpdateUnlockAtByID(ctx context.Context, chestID uint64, newUnlockAt time.Time) error {
	db := tx.FromContext(ctx, r.db)
	result := db.WithContext(ctx).Model(&UserChest{}).Where("id = ?", chestID).Update("unlock_at", newUnlockAt)
	return result.Error
}
