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
type PetRepo interface {
	// Create 插入一行 pets。GORM 在成功后回填 p.ID。
	Create(ctx context.Context, p *Pet) error

	// FindDefaultByUserID 查指定 user 的默认宠物（is_default=1）。
	//
	// NotFound → ErrPetNotFound（理论不应发生 —— firstTimeLogin 后必有默认猫；
	// 如发生说明数据脏，service 包成 1009）。
	FindDefaultByUserID(ctx context.Context, userID uint64) (*Pet, error)
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
