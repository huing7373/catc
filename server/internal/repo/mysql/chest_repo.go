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
	// dev_chest_service.ForceUnlockChest 接 client 传入的 chestID 参数 —— client 必须
	// 先 GET /chest/current 拿到当前 chest.id，再 POST /dev/force-unlock-chest 带这个 id 来。
	// service 用 FindByIDForUpdate（r4 改造后；见下方）校验存在 + 归属 + UPDATE 全部在事务内。
	//
	// **r4 [P2] 改造说明**：本方法在 r2/r3 是 dev_chest_service.ForceUnlockChest 的主路径；
	// r4 后改用 FindByIDForUpdate（事务 + FOR UPDATE），本 FindByID 保留供其他场景使用
	// （目前 home_service / chest_service 已用 FindByUserID，不依赖本方法 —— 但 interface 上
	// 保留 FindByID 作为通用"按 PK 取行"能力，避免每次新场景都加一个变种）。
	//
	// NotFound（PK 不存在）→ ErrChestNotFound 哨兵；service 层 errors.Is 翻译为 1003。
	FindByID(ctx context.Context, chestID uint64) (*UserChest, error)

	// FindByIDForUpdate 按 chestID 查 user_chests 行并对该行加排他锁（FOR UPDATE）。Story 20.7 review r4 [P2] 引入。
	//
	// **必须在事务内调用**（caller 传入的 ctx 必须是 txMgr.WithTx 注入的 txCtx）；
	// 事务外调用时 FOR UPDATE 锁会被 driver 立即释放（autocommit 模式下任何 SELECT
	// 完成即 commit），等同于普通 SELECT，违反 V1 §7.2.5c "FOR UPDATE 行锁串行化" 钦定。
	// ADR-0007 §2.4 钦定 fn 内全部 repo 调用用 txCtx。
	//
	// **为何加这个方法（与 FindByUserIDForUpdate 区分）**：
	// dev_chest_service.ForceUnlockChest 在 r4 [P2] 改造后用事务 + FOR UPDATE 锁定 chest 行，
	// 然后 UPDATE unlock_at —— FOR UPDATE 锁住后并发 /chest/open 的 DELETE 必须等当前事务
	// commit；事务内 SELECT 后的 UPDATE 必然命中（行不会"突然消失"），让 RowsAffected==0
	// 仅可能是"值未变"（同毫秒重复 unlock 同行）→ 视为 success，不再误判 NotFound。
	// 这是 r2-r3-r4 over-correction chain 的根因解决：用事务把 RowsAffected==0 的语义模糊性
	// 从 driver 层迁到 DB 层（事务内不变量保证）。
	//
	// 与 FindByUserIDForUpdate 区分：本方法用 PRIMARY KEY id（client 已通过 GET /chest/current
	// 知道是哪个 chest），FindByUserIDForUpdate 用 uk_user_id 唯一索引（chest_open_service
	// 在不知道 chestID 的场景下用 user 维度锁"当前 chest"）。
	//
	// GORM clause.Locking{Strength: "UPDATE"} 路径生成 SELECT ... WHERE id = ? FOR UPDATE SQL；
	// 与 FindByUserIDForUpdate / room_repo.FindByIDForUpdate 同模式。
	//
	// 找不到 → 返 ErrChestNotFound 哨兵（与 FindByID / FindByUserID 共用同一哨兵；
	// service 层 errors.Is 后翻译为 1003 ErrResourceNotFound）。
	FindByIDForUpdate(ctx context.Context, chestID uint64) (*UserChest, error)

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
	// r3 [P2]（在 force-unlock 场景下重新加回 RowsAffected==0 → ErrChestNotFound）、
	// r4 [P2]（重新移除 RowsAffected 检查；改用事务 + FindByIDForUpdate 在 caller 侧保证存在性）四次改造。
	//
	// # 历史改造路径
	//
	// **r0 → r1 改造**：原 `WHERE user_id = ?` 实装在与 `/chest/open` 并发时跑偏到 next chest。
	// r1 改成 `WHERE id = ?`。
	//
	// **r1 → r2 改造**：r1 的"FOR UPDATE SELECT 拿 id"在 OpenChest commit 后看到的 id 也是新 Y。
	// r2 把"哪个 chest"决策权交给 client，service 用 FindByID 校验 + 直 UPDATE；同时移除 RowsAffected
	// 检查（顾虑同毫秒重复 unlock 同 chest 值未变误判）。
	//
	// **r2 → r3 改造**：r2 的"完全不看 RowsAffected"是 over-correction，引入二阶 race：service.FindByID
	// 后 chest 被并发删除 → UPDATE 0 行但 service 返 success。r3 重新加回 RowsAffected==0 → ErrChestNotFound。
	//
	// **r3 → r4 改造（当前）**：r3 把 RowsAffected==0 翻译为 NotFound，引入同毫秒重复 unlock 同 chest
	// 误报 1003 bug（unlock_at 列是 DATETIME(3)，毫秒精度；两次 force-unlock 落在同一毫秒 → 值未变 →
	// rows_affected=0 → 误判 NotFound）。**r4 的根因解决**：跳出 r2/r3 表层 RowsAffected 微调，
	// 改用事务 + FindByIDForUpdate + UpdateUnlockAtByID 三件套 —— FindByIDForUpdate 在事务内 SELECT
	// FOR UPDATE 锁住 chest 行（并发 OpenChest 的 DELETE 必须等当前事务 commit），之后的 UPDATE 必然
	// 命中行，RowsAffected==0 唯一可能是"值未变"语义（force-unlock 场景下视为 success）。
	//
	// # rows_affected 语义（r4 改造后）
	//
	// **本方法不再用 RowsAffected 判断 NotFound**：
	//   - 调用方必须先在事务内调用 FindByIDForUpdate 校验存在性 + 归属（行锁让"并发 DELETE"
	//     必须等当前事务 commit）
	//   - 调用本方法时，事务内 chest 行存在性已保证 → RowsAffected==0 唯一来源 = 值未变
	//     （unlock_at 列已是 newUnlockAt；如同毫秒重复 unlock 同 chest），语义"chest 已是该
	//     unlock_at 状态" → 视为 success 返 nil
	//   - RowsAffected==1 → 行被更新 → 返 nil
	//   - DB error（连接断 / SQL 错 / 死锁等）→ raw error 透传（service 层包成 1009）
	//
	// **r4 必须配合 r4 调用契约使用**：
	//   - 必须在事务内调用（外层 caller 持有 txCtx 才能保证 FOR UPDATE 锁有效）
	//   - 必须在 FindByIDForUpdate 之后调用（事务保证 chest 行存在 + 锁住）
	//   - 当前唯一调用方：service.ForceUnlockChest（dev_chest_service r4 改造后；详见该 service 注释）
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
	// **r4 强制事务**：caller 必须在 txMgr.WithTx fn 内调用本方法（与 FindByIDForUpdate 同事务）；
	// 事务外调用会失去"行锁串行化"保证，RowsAffected==0 语义重新模糊。
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

// FindByIDForUpdate 实装：SELECT * FROM user_chests WHERE id = ? FOR UPDATE。Story 20.7 review r4 [P2] 引入。
//
// 走 PRIMARY KEY id（与 FindByID 同）；事务内锁该行，让并发 /chest/open 的 DELETE 必须等当前事务 commit。
// dev_chest_service.ForceUnlockChest r4 改造的核心依赖 —— 锁住后 UPDATE 必命中行，RowsAffected==0
// 不再代表 NotFound，跳出 r2-r3-r4 over-correction chain（详见 UpdateUnlockAtByID interface doc r4 改造说明）。
func (r *chestRepo) FindByIDForUpdate(ctx context.Context, chestID uint64) (*UserChest, error) {
	db := tx.FromContext(ctx, r.db)
	var c UserChest
	err := db.WithContext(ctx).
		Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("id = ?", chestID).
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
// **r4 [P2] 改造**：不再用 RowsAffected==0 判断 NotFound（详见 interface doc r4 改造说明）。
// caller 必须在事务内先调 FindByIDForUpdate 锁定行 + 校验存在性；本方法在 caller 已 FOR UPDATE
// 之后调用，行存在性由事务保证，RowsAffected==0 仅可能是"值未变"（同毫秒重复 unlock 同 chest），
// 视为 success 返 nil（force-unlock 业务语义"chest 已是该 unlock_at 状态"完全合理）。
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
	// r4 [P2]: 不再检查 RowsAffected==0 —— caller 已在事务内通过 FindByIDForUpdate 保证行存在；
	// RowsAffected==0 仅可能是同毫秒重复 unlock 同 chest（值未变），视为 success。
	return nil
}
