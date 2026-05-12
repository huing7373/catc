package mysql

import (
	"context"
	stderrors "errors"
	"time"

	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/repo/tx"
)

// Pet 是 pets 表的 GORM domain struct（数据库设计 §5.3 + migrations/0003）。
//
// 字段语义：
//   - PetType: TINYINT NOT NULL DEFAULT 1；节点 2 阶段固定 1（猫）；future epic 扩展更多
//   - Name: VARCHAR(64) NOT NULL DEFAULT ''；首次创建为 "默认小猫"（V1 §4.1 行 187）
//   - CurrentState: TINYINT NOT NULL DEFAULT 1；§6 状态枚举（1=rest）；节点 5 pet state
//     epic 才动态更新
//   - IsDefault: TINYINT NOT NULL DEFAULT 1；标识默认宠物
//
// **唯一约束**：UNIQUE KEY uk_user_default_pet (user_id, is_default) —— 一个用户最多
// 一只默认宠物。MVP 阶段每个用户首次登录创建一只 is_default=1 的猫。
type Pet struct {
	ID           uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	UserID       uint64    `gorm:"column:user_id"`
	PetType      int8      `gorm:"column:pet_type"`
	Name         string    `gorm:"column:name"`
	CurrentState int8      `gorm:"column:current_state"`
	IsDefault    int8      `gorm:"column:is_default"`
	CreatedAt    time.Time `gorm:"column:created_at;autoCreateTime:milli"`
	UpdatedAt    time.Time `gorm:"column:updated_at;autoUpdateTime:milli"`
}

// TableName 显式声明 "pets"（GORM inflection 默认即 "pets"，显式声明防漂移）。
func (Pet) TableName() string { return "pets" }

// PetRepo 是 pets 表的访问接口。
//
// 节点 2 阶段消费方：
//   - service.AuthService.firstTimeLogin: Create（事务内）
//   - service.AuthService.reuseLogin: FindDefaultByUserID（事务外）
//
// 节点 5 阶段消费方（Story 14.2 加）：
//   - service.PetService.SyncCurrentState: FindDefaultByUserID + UpdateCurrentStateByID
type PetRepo interface {
	// Create 插入一行 pets。GORM 在成功后回填 p.ID。
	Create(ctx context.Context, p *Pet) error

	// FindDefaultByUserID 查指定 user 的默认宠物（is_default=1）。
	//
	// NotFound → ErrPetNotFound（理论不应发生 —— firstTimeLogin 后必有默认猫；
	// 如发生说明数据脏，service 包成 1009）。
	FindDefaultByUserID(ctx context.Context, userID uint64) (*Pet, error)

	// UpdateCurrentStateByID 按主键更新 pets.current_state 列（Story 14.2 引入；
	// service 层先 FindDefaultByUserID 拿 pet.ID 后用主键定位更新）。
	//
	// **state 取值**：1 = rest, 2 = walk, 3 = run（与数据库设计 §6.4 + V1
	// §5.2 / §12.3 `### 宠物状态变更` 同义）；调用方（service 层）已确保 state ∈ {1,2,3}
	// （handler 层 1002 拦截在前）；本方法不重复校验入参枚举范围，仅做 SQL UPDATE。
	//
	// **更新字段**：current_state（显式 SET）+ updated_at（GORM autoUpdateTime 自动写
	// 当前时间，与数据库 §3.2 ON UPDATE CURRENT_TIMESTAMP(3) 语义一致）；**不**显式 SET
	// updated_at —— 让 GORM tag 处理避免与 ORM autoUpdateTime 双写冲突（与 user_repo
	// .UpdateNickname / UpdateCurrentRoomID 同模式）。
	//
	// **err 二分**（V1 §5.2 line 532-537 + r6 lessons 锁定）：
	//   - err == nil → 成功（**不**读 RowsAffected）：service 层一律视为成功，返 200 OK + code = 0
	//   - err != nil → 失败（driver / 网络 / 约束冲突 / 任何 DB 异常）：service 层包成 1009
	//
	// **不**读 RowsAffected：MySQL/GORM 语义下"同 user 同 state 重复上报"幂等场景的
	// `RowsAffected == 0` 是合法路径（V1 §5.2 关键约束 + r1 lessons + r6 实装锁定 + r9 sweep）；
	// 本接口的 UPDATE 把 updated_at 也写新值，理论上即便 current_state 未变 updated_at 仍变
	// → MySQL 通常仍报 RowsAffected == 1；但 GORM/driver 在某些 time-zone / 配置组合下
	// 可能仍返 0，service 层**不**依赖该值判断成功失败。
	//
	// ctx 用法（ADR-0007 §2.3）：本方法第一参数 ctx；GORM 调用 .WithContext(ctx)；
	// 本接口**不**入事务（数据库设计 §8.x 不含 state-sync 事务行；service 层不开 txMgr.WithTx）
	// —— 即便如此 repo 仍走 tx.FromContext(ctx, r.db) 模式（与 UpdateNickname /
	// UpdateBalance 一致），让本方法**未来若**被纳入事务（如多接口聚合）也能 ctx-aware
	// 无需改 repo signature。
	UpdateCurrentStateByID(ctx context.Context, petID uint64, state int8) error
}

// petRepo 是 PetRepo 的默认实装。
type petRepo struct {
	db *gorm.DB
}

// NewPetRepo 构造 PetRepo。
func NewPetRepo(db *gorm.DB) PetRepo {
	return &petRepo{db: db}
}

// Create 插入一行 pets。
func (r *petRepo) Create(ctx context.Context, p *Pet) error {
	db := tx.FromContext(ctx, r.db)
	return db.WithContext(ctx).Create(p).Error
}

// FindDefaultByUserID 查 (user_id, is_default=1) 的宠物。
//
// 走 uk_user_default_pet 索引；查不到返 ErrPetNotFound 哨兵。
func (r *petRepo) FindDefaultByUserID(ctx context.Context, userID uint64) (*Pet, error) {
	db := tx.FromContext(ctx, r.db)
	var p Pet
	err := db.WithContext(ctx).
		Where("user_id = ? AND is_default = ?", userID, 1).
		First(&p).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrPetNotFound
		}
		return nil, err
	}
	return &p, nil
}

// UpdateCurrentStateByID 实装：用 Update("current_state", v) 单字段更新（参考
// user_repo.UpdateNickname 模式 —— state 是 int8 不存在 nil-skip 陷阱，**不**需要
// Updates(map[string]interface{}) 路径）。
//
// **关键**：用 db.Model(&Pet{}).Where("id = ?", petID).Update("current_state", state)
// 而非 Save(&pet) —— Save 会写**全部**字段（含 created_at / pet_type / name /
// is_default 等），可能引入并发数据丢失 / autoUpdateTime 行为差异。Update 单字段
// 仅触发 SET current_state=?, updated_at=NOW() WHERE id=? + tag autoUpdateTime
// 自动加 updated_at SET（与数据库 §3.2 一致）。
//
// **err 二分锁定**（V1 §5.2 line 532-537 + r1 / r6 / r9 lessons）：service 层只看
// err == nil / err != nil 二分，repo 不读 RowsAffected。本方法透传 GORM err；service
// 层用 apperror.Wrap 包成 1009。
func (r *petRepo) UpdateCurrentStateByID(ctx context.Context, petID uint64, state int8) error {
	db := tx.FromContext(ctx, r.db)
	return db.WithContext(ctx).
		Model(&Pet{}).
		Where("id = ?", petID).
		Update("current_state", state).Error
}
