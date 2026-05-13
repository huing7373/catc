package mysql

import (
	"time"
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
