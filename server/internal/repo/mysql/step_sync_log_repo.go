package mysql

import (
	"context"
	stderrors "errors"
	"time"

	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/repo/tx"
)

// StepSyncLog 是 user_step_sync_logs 表的 GORM domain struct（数据库设计 §5.5 +
// migrations/0006）。
//
// **关键**：本表是 append-only 日志表（与 user_step_accounts §5.4 账户态表不同）：
//   - PK = id BIGINT UNSIGNED AUTO_INCREMENT（**非** user_id）
//   - 无 updated_at（一行写入后再不修改；7.2 §"不加 updated_at" 已锁定）
//   - 无 version 乐观锁（无并发更新场景）
//   - 无 deleted_at（不软删）
//
// 字段语义（每个字段的"为什么"详见 docs/宠物互动App_数据库设计.md §5.5 / §6.5 / §6.6）：
//   - ID: 自增 PK
//   - UserID: 归属用户
//   - SyncDate: 客户端按本机时区算出的"今天" YYYY-MM-DD（V1 §6.1.2 + GAP E）
//   - ClientTotalSteps: 客户端读取的当天系统累计（非增量）
//   - AcceptedDeltaSteps: 服务端实际确认入账的增量（可能 < client 期望，因截断 / 封顶 / 倒退）
//   - MotionState: 1=stationary_or_unknown / 2=walking / 3=running（§6.5）
//   - Source: 1=healthkit（客户端正常上报） / 2=admin_grant（Story 7.5 dev grant）（§6.6）
//   - ClientTs: 客户端调用接口时的本机毫秒时间戳（仅审计，不参与差值计算）
//   - CreatedAt: server 端写入毫秒时间（DATETIME(3)）
type StepSyncLog struct {
	ID                 uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	UserID             uint64    `gorm:"column:user_id"`
	SyncDate           time.Time `gorm:"column:sync_date;type:date"` // GORM 用 time.Time 映射 DATE
	ClientTotalSteps   uint64    `gorm:"column:client_total_steps"`
	AcceptedDeltaSteps int32     `gorm:"column:accepted_delta_steps"` // INT signed（§5.5；保留 future 负向修正可能）
	MotionState        int8      `gorm:"column:motion_state"`         // TINYINT
	Source             int8      `gorm:"column:source"`               // TINYINT
	ClientTs           uint64    `gorm:"column:client_ts"`            // BIGINT UNSIGNED
	CreatedAt          time.Time `gorm:"column:created_at;autoCreateTime:milli"`
}

// TableName 显式声明 "user_step_sync_logs"。
func (StepSyncLog) TableName() string { return "user_step_sync_logs" }

// StepSyncLogRepo 是 user_step_sync_logs 表的访问接口。
//
// **本 story 只实装三个方法**：
//   - Create（事务内插入一行）
//   - FindLatestByUserAndDate（差值计算依据）
//   - SumAcceptedDeltaByUserAndDate（防作弊当日封顶判断）
//
// future Story 7.5 dev grant 复用 Create；future 审计 query 可能加 ListByUser。
type StepSyncLogRepo interface {
	// Create 在当前 ctx 携带的事务内 INSERT 一行 sync_log。
	//
	// ctx 必须来自 txMgr.WithTx 的 txCtx —— 否则会脱离事务，与 §8.2 钦定事务边界相违。
	// service 层调用方负责传 txCtx；repo 内部用 tx.FromContext(ctx, r.db) 取 db handle。
	Create(ctx context.Context, log *StepSyncLog) error

	// FindLatestByUserAndDate 查最近一条 sync_log（用 idx_user_date 复合索引最左前缀）。
	//
	// **排序**：ORDER BY id DESC LIMIT 1（用 PK 自增天然时序；**不**用 created_at）
	// **NotFound 语义**：当日首次 sync → 返 ErrStepSyncLogNotFound（service 层捕获走"首次"分支）；
	//                    其他 DB 异常透传给 service 包成 1009
	//
	// ctx 必须是 txCtx（service 层 sync 流程在事务内调）—— 即使读操作也用事务一致视图。
	FindLatestByUserAndDate(ctx context.Context, userID uint64, syncDate time.Time) (*StepSyncLog, error)

	// SumAcceptedDeltaByUserAndDate 当日累计 accepted_delta_steps 求和（防作弊当日封顶判断用）。
	//
	// 实装：SELECT COALESCE(SUM(accepted_delta_steps), 0) FROM user_step_sync_logs
	//       WHERE user_id = ? AND sync_date = ?
	//
	// **返回**：当日已入账的 delta 累计值（INT64，避免 INT overflow）。
	// 当日无任何 sync_log（首次同步） → COALESCE 兜底返 0。
	//
	// ctx 必须是 txCtx（与 FindLatest 同语义；事务内一致视图）。
	//
	// **关键**：sum 必须**包含**当前正要写入的行**之外**的历史 —— 即调用 SumAccepted 时**还没**写本次 sync_log。
	SumAcceptedDeltaByUserAndDate(ctx context.Context, userID uint64, syncDate time.Time) (int64, error)
}

// stepSyncLogRepo 是 StepSyncLogRepo 的默认实装。
type stepSyncLogRepo struct {
	db *gorm.DB
}

// NewStepSyncLogRepo 构造 StepSyncLogRepo。
func NewStepSyncLogRepo(db *gorm.DB) StepSyncLogRepo {
	return &stepSyncLogRepo{db: db}
}

// Create 实装：tx.FromContext(ctx) → db.WithContext(ctx).Create。
func (r *stepSyncLogRepo) Create(ctx context.Context, log *StepSyncLog) error {
	db := tx.FromContext(ctx, r.db)
	return db.WithContext(ctx).Create(log).Error
}

// FindLatestByUserAndDate 实装：WHERE + ORDER BY id DESC LIMIT 1 + NotFound 翻译。
func (r *stepSyncLogRepo) FindLatestByUserAndDate(ctx context.Context, userID uint64, syncDate time.Time) (*StepSyncLog, error) {
	db := tx.FromContext(ctx, r.db)
	var log StepSyncLog
	err := db.WithContext(ctx).
		Where("user_id = ? AND sync_date = ?", userID, syncDate).
		Order("id DESC").
		First(&log).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrStepSyncLogNotFound
		}
		return nil, err
	}
	return &log, nil
}

// SumAcceptedDeltaByUserAndDate 实装：SELECT COALESCE(SUM(...), 0)。
func (r *stepSyncLogRepo) SumAcceptedDeltaByUserAndDate(ctx context.Context, userID uint64, syncDate time.Time) (int64, error) {
	db := tx.FromContext(ctx, r.db)
	var sum int64
	err := db.WithContext(ctx).
		Model(&StepSyncLog{}).
		Where("user_id = ? AND sync_date = ?", userID, syncDate).
		Select("COALESCE(SUM(accepted_delta_steps), 0)").
		Scan(&sum).Error
	if err != nil {
		return 0, err
	}
	return sum, nil
}
