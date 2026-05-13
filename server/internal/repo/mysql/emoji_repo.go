package mysql

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/repo/tx"
)

// EmojiConfig 是 emoji_configs 表的完整 GORM domain struct（Story 17.2 引入；
// 与 server/migrations/0009_init_emoji_configs.up.sql 真实 schema 1:1 对齐）。
//
// 字段（与数据库设计.md §5.15 + 0009_init_emoji_configs.up.sql 1:1 对齐）：
//   - ID:        BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§5.15 + §3.1 主键约定）
//   - Code:      VARCHAR(64) NOT NULL（表情业务标识符；UNIQUE KEY uk_code 保证全局唯一）
//   - Name:      VARCHAR(64) NOT NULL（表情中文名，UI 展示文字）
//   - AssetURL:  VARCHAR(255) NOT NULL DEFAULT ''（表情资源 URL；enabled 表情必须非空）
//   - SortOrder: INT NOT NULL DEFAULT 0（表情显示顺序，升序）
//   - IsEnabled: TINYINT NOT NULL DEFAULT 1（§6 枚举：0=disabled / 1=enabled）
//   - CreatedAt: DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
//   - UpdatedAt: DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
//
// 表层 UNIQUE 约束（uk_code）+ 普通索引（idx_enabled_sort）由 SQL DDL 定义，
// **不**在 struct tag 中重复声明（与 ADR-0003 §3.2 "禁止 GORM AutoMigrate" 同源；
// GORM struct 仅为 Find / Create 提供字段映射，**不**作为 schema 真相源；
// 与 Story 11.2 落地的 RoomMember / Room struct 同模式）。
//
// **范围红线**：本 struct 仅为下游 Story 17.3（seed）/ 17.4（GET /emojis 接口）/
// 17.5（WS emoji.send 校验）提供字段映射；本 story 阶段**不**新建 EmojiRepo
// interface / 实装 List / Exists / Create 等方法（YAGNI；17.4 落地 List 方法 +
// 17.5 落地 Exists 方法）。
type EmojiConfig struct {
	ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Code      string    `gorm:"column:code;not null;size:64"`
	Name      string    `gorm:"column:name;not null;size:64"`
	AssetURL  string    `gorm:"column:asset_url;not null;size:255;default:''"`
	SortOrder int32     `gorm:"column:sort_order;not null;default:0"`
	IsEnabled int8      `gorm:"column:is_enabled;not null;default:1"`
	CreatedAt time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
	UpdatedAt time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

// TableName 显式声明 "emoji_configs"。
func (EmojiConfig) TableName() string { return "emoji_configs" }

// EmojiRepo 是 emoji_configs 表的读取接口（Story 17.4 引入）。
//
// 本 story 阶段仅含 List 方法（GET /emojis 端点）；future Story 17.5 加
// Exists(ctx, code) 方法（WS emoji.send 校验 emojiCode 合法性 —— 单 emoji code
// 查询路径，与 List 全列表查询路径分开实装）。
//
// 不在本 story 落地：Create / Update / Delete（MVP 节点 6 无 admin 后台需求；
// emoji_configs 写入路径目前仅 0010_seed migration 一次性 seed，无运行时 admin 改写场景）
type EmojiRepo interface {
	// List 返回所有 is_enabled=1 的 emoji_configs 行（V1 §11.1 服务端逻辑步骤 2 钦定）。
	//
	// SQL: SELECT id, code, name, asset_url, sort_order FROM emoji_configs
	//      WHERE is_enabled = 1 ORDER BY sort_order ASC, id ASC
	//
	// 关键约束（§11.1 钦定）：
	//   - **次要排序键 `id ASC`**：保证 sort_order 相同时返回顺序确定
	//     （避免 client 端"同 sort_order 表情顺序在不同请求间不一致"问题）
	//   - **`is_enabled = 1`** 过滤：disabled 表情**不**返回（被 admin 临时下架 /
	//     WIP 阶段不放出的表情）
	//   - 0 行（如 seed 未执行 / 全部 disabled）→ ([]EmojiConfig{}, nil)，**不**返 nil slice
	//   - 多行 → ([]EmojiConfig{...}, nil)
	//   - DB error → (nil, raw error 透传给 service（service 包成 1009）)
	//
	// **注**：本方法返回 `[]EmojiConfig` 含 ID / CreatedAt / UpdatedAt 等字段，但 service
	// 层 DTO 转换会过滤掉 client 不需要的字段（V1 §11.1 钦定 id / is_enabled /
	// created_at / updated_at 不下发）；repo 层不做字段裁剪 —— 让 service 层关心 wire
	// 契约，repo 层只关心表字段映射。
	//
	// **走索引**：emoji_configs 表 `idx_enabled_sort (is_enabled, sort_order ASC)` 索引
	// 覆盖 `WHERE is_enabled = 1 ORDER BY sort_order ASC`（数据库设计 §5.15 钦定）；
	// 无 filesort，性能稳定。
	List(ctx context.Context) ([]EmojiConfig, error)

	// Exists 查 code 是否存在且 is_enabled=1（Story 17.5 引入；V1 §12.2 服务端
	// 逻辑步骤 4 钦定）。
	//
	// SQL: SELECT 1 FROM emoji_configs WHERE code = ? AND is_enabled = 1 LIMIT 1
	//
	// 关键约束（§12.2 钦定）：
	//   - **`is_enabled = 1` 过滤**：disabled 表情视同"不存在"返 false（避免 server
	//     暴露 enabled / disabled 状态信息；与 §12.2 服务端逻辑步骤 4 "两种情况合并
	//     为同一错误" 钦定一致）
	//   - LIMIT 1 优化：UNIQUE KEY uk_code 已保证 code 唯一，LIMIT 1 是 query planner
	//     hint，让 DB 命中第一行就返回
	//   - **走索引**：`uk_code` UNIQUE KEY 直接定位单行 + 应用层 filter `is_enabled = 1`
	//     （O(1) 查询；数据库设计 §5.15 钦定）
	//   - 0 行 → (false, nil)（包括 code 不存在 / 存在但 is_enabled=0）
	//   - 1 行 → (true, nil)
	//   - DB error → (false, raw error 透传给 service（service 包成 1009）)
	//
	// **不**返 EmojiConfig 完整行：本方法仅供 emoji.send 校验路径用，service 层
	// 只需要 bool 信号而非完整字段；少 SELECT 字段 = 少 wire 字节 = 少 GORM Scan
	// 开销。如 future 需要"查 code 然后用 assetUrl"，由对应 story 加新方法
	// （如 GetByCode），**不**改 Exists 签名。
	Exists(ctx context.Context, code string) (bool, error)
}

// emojiRepo 是 EmojiRepo 的默认实装。
type emojiRepo struct {
	db *gorm.DB
}

// NewEmojiRepo 构造 EmojiRepo。
func NewEmojiRepo(db *gorm.DB) EmojiRepo {
	return &emojiRepo{db: db}
}

// Exists 实装：单 SELECT 查询 with LIMIT 1。详见 EmojiRepo.Exists 接口注释。
//
// 用 Select("1") + Limit(1) + Find(&result)：返 0 行时 result 为空 slice 而非
// error；与 GORM 既有 EmojiConfig.List 模式一致；不用 First 是因为 First 在 0
// 行时返 ErrRecordNotFound，需要在 caller 层额外 errors.Is 判断（增加复杂度）。
func (r *emojiRepo) Exists(ctx context.Context, code string) (bool, error) {
	db := tx.FromContext(ctx, r.db).WithContext(ctx)

	var dummy []int
	err := db.
		Model(&EmojiConfig{}).
		Select("1").
		Where("code = ? AND is_enabled = ?", code, 1).
		Limit(1).
		Find(&dummy).Error
	if err != nil {
		return false, err
	}
	return len(dummy) > 0, nil
}

// List 实装：单 SELECT 查询。详见 EmojiRepo.List 接口注释。
func (r *emojiRepo) List(ctx context.Context) ([]EmojiConfig, error) {
	// 用 tx.FromContext 取 db handle：事务外调用走 r.db；事务内调用走 txCtx 注入的
	// tx 句柄（与 11.6 既有 repo 同模式；本 story 阶段实际不会在事务内调，但保持
	// 与既有 repo 模式一致让 future 扩展无需改 method body）。
	db := tx.FromContext(ctx, r.db).WithContext(ctx)

	var rows []EmojiConfig
	// 显式 Select 字段集（不依赖 GORM 自动 SELECT *），与 §11.1 服务端逻辑步骤 2
	// 钦定字段 1:1 对齐；避免 future 表加字段时被自动拉过来污染 query payload。
	// **注**：CreatedAt / UpdatedAt **不**在 SELECT 列表中（client 不需要 + service
	// 层不做 wire DTO 转换），但 GORM Scan 会把它们填为 zero-value time.Time；
	// service 层 DTO 转换不读这两字段，所以 zero-value 是安全的。
	err := db.
		Select("id, code, name, asset_url, sort_order").
		Where("is_enabled = ?", 1).
		Order("sort_order ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	// GORM Find 在 0 行时返回空 slice 而非 nil（与 ListMembers 同模式 — 验证：
	// db.Find(&[]X{}) 返回 len(rows)==0 且 rows != nil）；保险起见显式兜底空 slice
	// 让 service 层调用方不需要 nil-check。
	if rows == nil {
		rows = []EmojiConfig{}
	}
	return rows, nil
}
