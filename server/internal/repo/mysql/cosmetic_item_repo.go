package mysql

import (
	"time"
)

// CosmeticItem 是 cosmetic_items 表的完整 GORM domain struct（Story 20.2 引入；
// 与 server/migrations/0011_init_cosmetic_items.up.sql 真实 schema 1:1 对齐）。
//
// 字段（与数据库设计.md §5.8 + 0011_init_cosmetic_items.up.sql 1:1 对齐）：
//   - ID:         BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§5.8 + §3.1 主键约定）
//   - Code:       VARCHAR(64) NOT NULL（装扮业务编码；UNIQUE KEY uk_code 保证全局唯一）
//   - Name:       VARCHAR(64) NOT NULL（装扮中文名，UI 展示文字）
//   - Slot:       TINYINT NOT NULL（§6.8 枚举：1=hat / 2=gloves / 3=glasses /
//                 4=neck / 5=back / 6=body / 7=tail / 99=other）
//   - Rarity:     TINYINT NOT NULL（§6.9 枚举：1=common / 2=rare / 3=epic / 4=legendary）
//   - AssetURL:   VARCHAR(255) NOT NULL DEFAULT ''（装扮资源 URL；enabled 装扮必须非空）
//   - IconURL:    VARCHAR(255) NOT NULL DEFAULT ''（图标资源 URL；enabled 装扮必须非空）
//   - DropWeight: INT UNSIGNED NOT NULL DEFAULT 0（加权抽奖权重；0 = 不参与抽奖）
//   - IsEnabled:  TINYINT NOT NULL DEFAULT 1（§6 枚举：0=disabled / 1=enabled）
//   - CreatedAt:  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
//   - UpdatedAt:  DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
//
// 表层 UNIQUE 约束（uk_code）+ 普通索引（idx_slot_rarity / idx_enabled_weight）
// 由 SQL DDL 定义，**不**在 struct tag 中重复声明（与 ADR-0003 §3.2 "禁止 GORM
// AutoMigrate" 同源；GORM struct 仅为 Find / Create 提供字段映射，**不**作为
// schema 真相源；与 Story 17.2 落地的 EmojiConfig / Story 11.2 落地的
// RoomMember / Room struct 同模式）。
//
// **范围红线**：本 struct 仅为下游 Story 20.3（seed）/ 20.6（POST /chest/open 加权抽取）/
// Epic 23（GET /cosmetics/catalog / inventory）/ Epic 32 / 33（合成事务）提供字段
// 映射；本 story 阶段**不**新建 CosmeticItemRepo interface / 实装 List /
// WeightedRandomPick / Exists / Create 等方法（YAGNI；20.6 落地加权抽取方法 +
// 23.x 落地 catalog / inventory 方法）。
//
// **不**包含 RenderConfig 字段（节点 10 / Epic 29 Story 29.2 落地 add_column
// migration 后由该 story 同步加 RenderConfig string `gorm:"column:render_config"` 字段）。
type CosmeticItem struct {
	ID         uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	Code       string    `gorm:"column:code;not null;size:64"`
	Name       string    `gorm:"column:name;not null;size:64"`
	Slot       int8      `gorm:"column:slot;not null"`
	Rarity     int8      `gorm:"column:rarity;not null"`
	AssetURL   string    `gorm:"column:asset_url;not null;size:255;default:''"`
	IconURL    string    `gorm:"column:icon_url;not null;size:255;default:''"`
	DropWeight uint32    `gorm:"column:drop_weight;not null;default:0"`
	IsEnabled  int8      `gorm:"column:is_enabled;not null;default:1"`
	CreatedAt  time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
	UpdatedAt  time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

// TableName 显式声明 "cosmetic_items"。
func (CosmeticItem) TableName() string { return "cosmetic_items" }
