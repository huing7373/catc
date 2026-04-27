package mysql

import (
	"context"
	stderrors "errors"
	"time"

	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/repo/tx"
)

// StepAccount 是 user_step_accounts 表的 GORM domain struct（数据库设计 §5.4 +
// migrations/0004）。
//
// **关键**：本表 PK = user_id（**不是**自增 id），1:1 关联 users —— "账户模型"，
// 一个用户只有一行步数账户。Story 4.6 首次登录初始化 INSERT 默认全 0；后续
// Epic 7 步数 epic 用乐观锁 (version + 1) 扣减。
//
// 字段语义：
//   - TotalSteps: 累计总步数（永远递增，审计用）
//   - AvailableSteps: 当前可用步数（开宝箱 / 等业务消费）
//   - ConsumedSteps: 已消耗步数 = TotalSteps - AvailableSteps（冗余但便于查询）
//   - Version: 乐观锁版本号；Story 4.6 初始化默认 0
type StepAccount struct {
	UserID         uint64    `gorm:"column:user_id;primaryKey"`
	TotalSteps     uint64    `gorm:"column:total_steps"`
	AvailableSteps uint64    `gorm:"column:available_steps"`
	ConsumedSteps  uint64    `gorm:"column:consumed_steps"`
	Version        uint64    `gorm:"column:version"`
	CreatedAt      time.Time `gorm:"column:created_at;autoCreateTime:milli"`
	UpdatedAt      time.Time `gorm:"column:updated_at;autoUpdateTime:milli"`
}

// TableName 显式声明 "user_step_accounts"。
func (StepAccount) TableName() string { return "user_step_accounts" }

// StepAccountRepo 是 user_step_accounts 表的访问接口（节点 2 阶段 Create + FindByUserID；
// 节点 3 步数 epic 才加 UpdateBalance 等）。
type StepAccountRepo interface {
	// Create 插入一行 step_accounts。
	//
	// **关键**：StepAccount 的 PK 是 user_id（非自增）→ GORM Create 不会回填 ID
	// （PK 由调用方填）；调用方在 service.firstTimeLogin 已经先 INSERT users
	// 拿到 user.ID 后再调本方法。
	Create(ctx context.Context, a *StepAccount) error

	// FindByUserID 查指定 user 的步数账户（PK = user_id 单行查询）。
	//
	// NotFound → ErrStepAccountNotFound 哨兵；其他 DB 异常透传给 service。
	// 节点 2 阶段调用方仅 home_service.LoadHome（Story 4.8）；节点 3 步数 epic 起 step_service 也消费。
	FindByUserID(ctx context.Context, userID uint64) (*StepAccount, error)
}

// stepAccountRepo 是 StepAccountRepo 的默认实装。
type stepAccountRepo struct {
	db *gorm.DB
}

// NewStepAccountRepo 构造 StepAccountRepo。
func NewStepAccountRepo(db *gorm.DB) StepAccountRepo {
	return &stepAccountRepo{db: db}
}

// Create 插入一行 step_accounts；走 GORM Create —— PK = user_id 由调用方填。
func (r *stepAccountRepo) Create(ctx context.Context, a *StepAccount) error {
	db := tx.FromContext(ctx, r.db)
	return db.WithContext(ctx).Create(a).Error
}

// FindByUserID 查 (user_id) 的步数账户行。
//
// 走 PK 单行查询；查不到返 ErrStepAccountNotFound 哨兵；其他 DB error 透传给
// service 由 service 包成 1009（ADR-0006 三层映射）。
//
// 与 FindByID 命名风格一致：以"按什么字段查"作为后缀，便于 future 再加
// FindByXxx 时保持惯例。
func (r *stepAccountRepo) FindByUserID(ctx context.Context, userID uint64) (*StepAccount, error) {
	db := tx.FromContext(ctx, r.db)
	var a StepAccount
	err := db.WithContext(ctx).Where("user_id = ?", userID).First(&a).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStepAccountNotFound
		}
		return nil, err
	}
	return &a, nil
}
