package mysql

import (
	"context"
	stderrors "errors"
	"time"

	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/repo/tx"
)

// IdempotencyRecord 是 chest_open_idempotency_records 表的 GORM domain struct
// （Story 20.6 引入；与 server/migrations/0014_init_chest_open_idempotency_records.up.sql
// 真实 schema 1:1 对齐 + 数据库设计 §5.16）。
//
// 字段语义：
//   - ID:             BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§3.1 主键约定）
//   - UserID:         BIGINT UNSIGNED NOT NULL（归属用户 id，语义上 ref users.id，**不**建 FK）
//   - IdempotencyKey: VARCHAR(128) NOT NULL（client 传入幂等键；V1 §7.2 钦定
//                     [A-Za-z0-9_:-] 字符集 + length 1-128；handler 层 regex 校验，repo 不再二次校验）
//   - Status:         ENUM('pending', 'success') NOT NULL DEFAULT 'pending'
//                     **二态机**（r7 锁定，从 r6 三态机简化）；**无** 'failed' 状态。
//                     GORM 用 string 映射 ENUM；service 层用 IdempotencyStatusSuccess /
//                     IdempotencyStatusPending 常量比较。
//   - ResponseJSON:   JSON NULL（status='success' 时存可缓存 payload；不含 time-derived 字段）
//                     用 []byte 映射 raw JSON，让 service 层 json.Marshal / json.Unmarshal 直处理。
//   - CreatedAt / UpdatedAt: §3.2 毫秒精度时间戳；UpdatedAt 在 status 推进时由 DDL
//                     `ON UPDATE CURRENT_TIMESTAMP(3)` 自动维护。
//
// **关键 PK 约束**：PK 是 id 自增；user_id 是 UNIQUE 索引 uk_user_id_key 一部分，**不是** PK
// （与 step_account_repo.StepAccount 的 user_id 作 PK 不同 —— 那是账户模型 1:1，
// 这里是日志模型 1:N）。
type IdempotencyRecord struct {
	ID             uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	UserID         uint64    `gorm:"column:user_id;not null"`
	IdempotencyKey string    `gorm:"column:idempotency_key;not null;size:128"`
	Status         string    `gorm:"column:status;not null;size:16"` // ENUM('pending', 'success') —— GORM 用 string 映射
	ResponseJSON   []byte    `gorm:"column:response_json"`            // JSON NULL；用 []byte 让 GORM 走 raw JSON
	CreatedAt      time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

// TableName 显式声明 "chest_open_idempotency_records"。
func (IdempotencyRecord) TableName() string { return "chest_open_idempotency_records" }

// 二态机常量（V1 §7.2 r7 + DB §5.16 r11 钦定；service 层用做字段比较 / claim 写值）。
const (
	IdempotencyStatusPending = "pending"
	IdempotencyStatusSuccess = "success"
)

// IdempotencyRepo 是 chest_open_idempotency_records 表的访问接口（Story 20.6 引入）。
//
// 设计原则（V1 §7.2 r5/r6/r7/r11 + DB §5.16）：
//   - **FindByUserIDAndKey**: handler 入口 autocommit 预检；仅命中 status='success' 时短路（MVCC 决定 pending 不可见）
//   - **ClaimPending**: 事务内**首条**语句；借 UNIQUE(user_id, idempotency_key) 做 single-statement 原子 claim；
//                       返 affected_rows：1 = 新行 / 0 = 行已存在（首事务已 commit success；本事务走步骤 5b 短路）
//   - **MarkSuccess**: 事务内最终化（步骤 5k）；UPDATE status='success' + response_json
//
// **不**实装 MarkFailed（r7 决策：彻底移除 best-effort failed upsert，schema 已无 'failed' 状态）。
// **不**实装 Delete（运维清理任务由 future epic owner，本 story 仅落地业务路径所需方法）。
type IdempotencyRepo interface {
	// FindByUserIDAndKey 查 (user_id, idempotency_key) 行（走 uk_user_id_key 唯一索引）。
	//
	// **autocommit 调用**（V1 §7.2.3 钦定）：传入的 ctx **不**应带 tx 句柄 —— 这是 handler 入口
	// 在业务事务之前的预检。
	//
	// NotFound → ErrIdempotencyRecordNotFound 哨兵；其他 DB error 透传给 service。
	//
	// **MVCC 不可见 r11**：如果首事务正在 INSERT pending 但未 commit，本 autocommit SELECT
	// 在 InnoDB REPEATABLE READ 隔离级别下**看不到** pending 行 → 返 NotFound；这是
	// 协议层硬约束，不要试图通过降低隔离级别"修复"（详见 V1 §7.2 r11 决策段）。
	FindByUserIDAndKey(ctx context.Context, userID uint64, idempotencyKey string) (*IdempotencyRecord, error)

	// ClaimPending 事务内首条语句：原子声明 idempotency 行。
	//
	// SQL: INSERT INTO chest_open_idempotency_records (user_id, idempotency_key, status, ...)
	//      VALUES (?, ?, 'pending', NULL, NOW(3), NOW(3))
	//      ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)
	//
	// **必须**通过事务 ctx 调用（service 层走 txMgr.WithTx）—— 否则 ON DUPLICATE 的语义错乱。
	//
	// 返回值：
	//   - affected_rows = 1: 新行 INSERT，本请求是同 key 首次到达 **或** 首事务已 rollback 后到达
	//   - affected_rows = 0: 行已存在，且首事务已 commit；service 走步骤 5b 短路
	//
	// **InnoDB unique-key X-lock**：同 key 并发请求只有一个能拿锁；其他事务在 INSERT 语句上
	// 阻塞等首事务释放锁后再继续。这是协议层并发兜底（详见 V1 §7.2 关键约束「事务边界」段）。
	ClaimPending(ctx context.Context, userID uint64, idempotencyKey string) (int64, error)

	// MarkSuccess 事务内最终化（V1 §7.2.5k 钦定）：UPDATE status='success' + response_json。
	//
	// 必须在业务事务内调用（与业务表 INSERT / UPDATE / DELETE 同事务原子 commit）。
	//
	// 返回值：
	//   - nil: UPDATE 成功（rows_affected ≥ 1）
	//   - ErrIdempotencyRecordNotFound: rows_affected = 0（理论不该发生 —— 同事务前面 ClaimPending 必已 INSERT；
	//                                  实际触发说明上游调用顺序错乱，应作为 1009 透传）
	//   - 其他 DB error 透传
	MarkSuccess(ctx context.Context, userID uint64, idempotencyKey string, responseJSON []byte) error
}

// idempotencyRepo 是 IdempotencyRepo 的默认实装。
type idempotencyRepo struct {
	db *gorm.DB
}

// NewIdempotencyRepo 构造 IdempotencyRepo。
func NewIdempotencyRepo(db *gorm.DB) IdempotencyRepo {
	return &idempotencyRepo{db: db}
}

// FindByUserIDAndKey 实装：autocommit 单查；走 uk_user_id_key UNIQUE 索引。
//
// tx.FromContext(ctx, r.db) 兜底兼容：autocommit 路径与 tx 路径共用同一方法实装
// —— service 层在事务外（handler 入口预检）/ 事务内（步骤 5b 短路读）都可调，
// 由 ctx 决定走 tx 还是 r.db。
func (r *idempotencyRepo) FindByUserIDAndKey(ctx context.Context, userID uint64, idempotencyKey string) (*IdempotencyRecord, error) {
	db := tx.FromContext(ctx, r.db)
	var rec IdempotencyRecord
	err := db.WithContext(ctx).
		Where("user_id = ? AND idempotency_key = ?", userID, idempotencyKey).
		First(&rec).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrIdempotencyRecordNotFound
		}
		return nil, err
	}
	return &rec, nil
}

// ClaimPending 实装：INSERT ... ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)。
//
// **关键 1**：本 SQL 走 raw `db.Exec` 而非 GORM Create / Upsert —— GORM 高级 API 默认行为
// 在 ON DUPLICATE 上有版本差异（v1.21+ 的 OnConflict 在某些版本生成不同 SQL）；用 raw SQL
// 保证 cross-version 行为一致。
//
// **关键 2**：affected_rows 用 `result.RowsAffected` 取（GORM 的 Raw / Exec 都暴露此字段）。
// affected_rows = 1 = INSERT 生效（新行）；affected_rows = 0 = ON DUPLICATE 触发但 update 列
// 是 no-op（id = LAST_INSERT_ID(id) 不改任何字段，故 update path rows_affected = 0）。
// **注**：MySQL ON DUPLICATE 在 update 路径下若有真实字段变更，affected_rows = 2；本 SQL 用
// `id = LAST_INSERT_ID(id)` 是惯用 no-op upsert pattern，affected_rows 只能是 0 或 1。
func (r *idempotencyRepo) ClaimPending(ctx context.Context, userID uint64, idempotencyKey string) (int64, error) {
	db := tx.FromContext(ctx, r.db)
	sql := "INSERT INTO chest_open_idempotency_records (user_id, idempotency_key, status, response_json, created_at, updated_at) " +
		"VALUES (?, ?, ?, NULL, NOW(3), NOW(3)) " +
		"ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)"
	result := db.WithContext(ctx).Exec(sql, userID, idempotencyKey, IdempotencyStatusPending)
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// MarkSuccess 实装：UPDATE status='success' + response_json + updated_at。
//
// updated_at 由 DDL `ON UPDATE CURRENT_TIMESTAMP(3)` 自动维护，**不**手动 set
// （与 0014 migration DDL ON UPDATE clause 一致）。
func (r *idempotencyRepo) MarkSuccess(ctx context.Context, userID uint64, idempotencyKey string, responseJSON []byte) error {
	db := tx.FromContext(ctx, r.db)
	res := db.WithContext(ctx).
		Model(&IdempotencyRecord{}).
		Where("user_id = ? AND idempotency_key = ?", userID, idempotencyKey).
		Updates(map[string]interface{}{
			"status":        IdempotencyStatusSuccess,
			"response_json": responseJSON,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return ErrIdempotencyRecordNotFound
	}
	return nil
}
