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
	// **race 不再成立**：chest 一旦绑定 user_id，user_id 字段不会改；UPDATE WHERE id 走 PK。
	// r3 [P2] 改造后，若 FindByID 后 chest 被另一并发 OpenChest 删除 → UpdateUnlockAtByID 影响 0 行
	// → 翻译为 ErrChestNotFound → service 返 1003（让 client 重新 GET /chest/current 拿新 id 后重试）；
	// 而非 r2 的"视为成功"——避免"dev 端点声称 unlock 成功但 GET /chest/current 仍 counting"的二阶 race。
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
	// 历经 r1 [P2]（→ UpdateUnlockAtByID(chestID)）、r2 [P2]（移除 RowsAffected 检查）、
	// r3 [P2]（在 force-unlock 场景下重新加回 RowsAffected==0 → ErrChestNotFound）三次改造。
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
	// server 不再猜"current"语义，service 用 FindByID 校验存在性 + 归属。r2 同时**移除** RowsAffected
	// 检查（顾虑"同毫秒重复 unlock 同 chest"值未变误判）。
	//
	// **r2 → r3 改造（当前）**：r2 的"完全不看 RowsAffected"是 over-correction，引入二阶 race：
	// service.ForceUnlockChest 步骤 (1) FindByID 拿 chest + 校验归属 → 步骤 (2) UPDATE 之间，
	// 另一并发 /chest/open 把 chest 删除 → UPDATE 影响 0 行但 service 返 success → 用户体验
	// 是"dev 端点声称 unlock 成功但 GET /chest/current 仍 counting"。r3 重新加回 RowsAffected==0
	// → ErrChestNotFound 检查。
	//
	// # rows_affected 语义（r3 改造后）
	//
	// **在 force-unlock 场景下，RowsAffected==0 等同于 NotFound**：
	//   - newUnlockAt = time.Now().UTC()，系统时钟单调推进，毫秒级唯一 → unlock_at 列值必然变化
	//     → 行存在时 MySQL 必返 RowsAffected=1
	//   - RowsAffected==0 **唯一**来源 = 行已不存在（被并发 OpenChest 删除）
	//   - r2 顾虑的"同毫秒重复 unlock 同行 → 值未变 → rows_affected=0"是理论 case：
	//     time.Now() 毫秒级冲突极罕见 + 重试可恢复 + dev 端点对此容忍度高
	//
	// **r3 实装**：
	//   - DB error（连接断 / SQL 错 / 死锁等）→ raw error 透传（service 层包成 1009）
	//   - RowsAffected==0 → ErrChestNotFound 哨兵（service 层翻译为 1003）
	//   - RowsAffected==1 → 返 nil
	//
	// **未来调用方注意**：若其他场景调用本方法且 newUnlockAt 可能与现有值"按位相同"
	// （如固定时间常量、或传入 chest.UnlockAt 自身），需重新评估 RowsAffected==0 语义；
	// 当前唯一调用方是 service.ForceUnlockChest，其 newUnlockAt 是 time.Now().UTC()。
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
	// 同事务），但事务对 race 修复**无作用**（详见上面历史路径），徒增复杂度，r2 移除。r3 通过 RowsAffected==0
	// → NotFound 让 race（"FindByID 后 chest 被并发删除"）从"false success"变成"显式 1003"，无需事务。
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
// **r3 [P2] 改造**：重新加回 RowsAffected==0 → ErrChestNotFound 检查（修复 r2 移除后引入的二阶
// race —— service.FindByID 与 UPDATE 之间 chest 被并发 OpenChest 删除 → UPDATE 0 行但 service
// 返 false success）。在 force-unlock 场景下，newUnlockAt = time.Now().UTC() 毫秒级唯一，行存在
// 时 UPDATE 必返 RowsAffected=1；RowsAffected==0 唯一来源 = 行已不存在。
//
// **GORM 实装**：用 `gorm.DB.Model(&UserChest{}).Where("id = ?", chestID).Update("unlock_at", newUnlockAt)`
// 而非 `.Save(&chest)` —— Save 会写入全部字段（含 version / status / open_cost_steps）+
// 触发 GORM autoUpdateTime:milli 自动更新 updated_at；Update("unlock_at", ...) 只动 unlock_at +
// updated_at（GORM 会自动加 updated_at）。
func (r *chestRepo) UpdateUnlockAtByID(ctx context.Context, chestID uint64, newUnlockAt time.Time) error {
	db := tx.FromContext(ctx, r.db)
	result := db.WithContext(ctx).Model(&UserChest{}).Where("id = ?", chestID).Update("unlock_at", newUnlockAt)
	if result.Error != nil {
		return result.Error
	}
	// r3 [P2]: 在 force-unlock 场景下 RowsAffected==0 等同于 NotFound（详见 interface doc）。
	if result.RowsAffected == 0 {
		return ErrChestNotFound
	}
	return nil
}
