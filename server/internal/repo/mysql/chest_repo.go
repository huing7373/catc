package mysql

import (
	"context"
	stderrors "errors"
	"time"

	"gorm.io/gorm"

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
