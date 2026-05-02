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
// 节点 3 步数 epic 起加 UpdateBalance）。
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

	// UpdateBalance 在事务内更新步数账户三档值（乐观锁 version + 1）。Story 7.3 引入。
	//
	// 实装：UPDATE user_step_accounts
	//       SET total_steps = total_steps + ?, available_steps = available_steps + ?,
	//           version = version + 1
	//       WHERE user_id = ? AND version = ?
	//
	// **关键 1**：用 SQL 表达式 `total_steps + ?` 而非"读出来 +delta 再 UPDATE"——
	// 避免 race condition；GORM 用 gorm.Expr("total_steps + ?", delta) 表达。
	// **关键 2**：乐观锁 WHERE version = ? —— 若并发改动，rows affected = 0 →
	// 返 ErrStepAccountVersionMismatch（service 层包成 1009）。
	// **关键 3**：consumed_steps **不更新**（V1 §6.1.4 钦定 sync 接口仅加 total / available，
	// consumed 由 future 开宝箱 / 等消费场景扣减）。
	// **关键 4**：delta 类型 int32（与 sync_log.accepted_delta_steps 同；INT signed）。
	//
	// 参数：
	//   - userID: 目标账户 PK
	//   - delta: 本次入账增量（≥ 0；防作弊 service 层已截断 / 封顶为 0）
	//   - expectedVersion: 当前 step_account 行的 version 值（service 层先 FindByUserID 拿到）
	//
	// 错误：
	//   - ErrStepAccountVersionMismatch: 乐观锁失败（rows affected = 0）
	//   - 其他 DB error 透传
	UpdateBalance(ctx context.Context, userID uint64, delta int32, expectedVersion uint64) error
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

// UpdateBalance 实装：UPDATE user_step_accounts SET total/available/version
// WHERE user_id AND version；rows affected = 0 → ErrStepAccountVersionMismatch。
//
// 用 gorm.Expr 让 SQL 层做加法（race-free）；consumed_steps 不更新。
func (r *stepAccountRepo) UpdateBalance(ctx context.Context, userID uint64, delta int32, expectedVersion uint64) error {
	db := tx.FromContext(ctx, r.db)
	res := db.WithContext(ctx).
		Model(&StepAccount{}).
		Where("user_id = ? AND version = ?", userID, expectedVersion).
		Updates(map[string]interface{}{
			"total_steps":     gorm.Expr("total_steps + ?", delta),
			"available_steps": gorm.Expr("available_steps + ?", delta),
			"version":         gorm.Expr("version + 1"),
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrStepAccountVersionMismatch
	}
	return nil
}
