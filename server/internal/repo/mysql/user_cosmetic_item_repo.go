package mysql

import (
	"time"
)

// UserCosmeticItem 是 user_cosmetic_items 表的完整 GORM domain struct
// （Story 23.2 引入；与 server/migrations/0015_init_user_cosmetic_items.up.sql
// 真实 schema 1:1 对齐）。
//
// 字段（与数据库设计.md §5.9 + 0015_init_user_cosmetic_items.up.sql 1:1 对齐）：
//   - ID:             BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§5.9 + §3.1 主键约定；
//                     §5.9 字段说明"玩家道具实例 id，即每一个道具的唯一 id"）
//   - UserID:         BIGINT UNSIGNED NOT NULL（归属用户；语义上 ref users.id，**不**建 FK）
//   - CosmeticItemID: BIGINT UNSIGNED NOT NULL（对应的装扮配置 id；语义上 ref
//                     cosmetic_items.id，**不**建 FK）
//   - Status:         TINYINT NOT NULL DEFAULT 1（§6.10 枚举：1=in_bag / 2=equipped /
//                     3=consumed / 4=invalid；DEFAULT 1=in_bag）
//   - Source:         TINYINT NOT NULL DEFAULT 1（§6.11 枚举：1=chest / 2=compose /
//                     3=admin_grant / 4=event_reward；DEFAULT 1=chest）
//   - SourceRefID:    BIGINT UNSIGNED NULL（来源关联记录 id；**指针** *uint64 映射
//                     NULL 可空列 —— 开箱时=chest_id 非空，合成产出实例先 NULL 后
//                     回填 compose_log_id；NULL → nil，避免 0 与 NULL 语义混淆，
//                     下游 23.4 / Epic 32 判"是否已回填"才正确）
//   - ObtainedAt:     DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)（获得时间）
//   - ConsumedAt:     DATETIME(3) NULL（消耗时间；**指针** *time.Time 映射 NULL
//                     可空列 —— §5.9 字段说明"未消耗时为空"；未消耗 → nil，
//                     合成消耗时写 NOW(3)；用值类型会让 NULL → 零值，下游判
//                     "是否已消耗"出错）
//   - CreatedAt:      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
//   - UpdatedAt:      DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
//                     ON UPDATE CURRENT_TIMESTAMP(3)（status / consumed_at 推进时
//                     自动更新 —— 实例可变表，与 0013 append-only 日志表无
//                     updated_at 形成对照）
//
// 表层**无** UNIQUE 约束（实例表，同 user_id + 同 cosmetic_item_id 可持有多件
// —— FR16"同种配置可被持有多件"）；三个普通索引（idx_user_id_status /
// idx_user_id_cosmetic_item_id / idx_source）由 SQL DDL 定义，**不**在 struct
// tag 中重复声明（与 ADR-0003 §3.2 "禁止 GORM AutoMigrate" 同源；GORM struct
// 仅为 Find / Create 提供字段映射，**不**作为 schema 真相源；与 Story 20.2 落地
// 的 CosmeticItem / Story 17.2 落地的 EmojiConfig / Story 11.2 落地的
// RoomMember / Room struct 同模式）。
//
// **范围红线**：本 struct 仅为下游 Story 23.4（GET /cosmetics/inventory 聚合
// 查询）/ Story 23.5（开箱事务补入仓 INSERT）/ Story 20.8（dev /dev/grant-
// cosmetic-batch）/ Epic 26（穿戴事务 status 1↔2 推进）/ Epic 32 / 33（合成
// 事务消耗实例 status→3 + 产出新实例）提供字段映射；本 story 阶段**不**新建
// UserCosmeticItemRepo interface / 实装 ListByUserAndStatus / InsertInTx /
// AggregateByCosmetic / MarkConsumed 等任何方法（YAGNI；对标 Story 20.2 阶段
// cosmetic_item_repo.go 仅 struct+TableName 的最小集 —— 注意 cosmetic_item_repo.go
// 现含 CosmeticItemRepo interface 是 Story 20.6 后续扩展加的，**不**是 20.2
// 阶段产物，本 story 对应 20.2 阶段最小集，**不**提前加 interface / 方法）。
//
// **不**引入 gorm.Model（避免引入 DeletedAt / 软删除字段，与 0015 真实 schema
// 不符；user_cosmetic_items 用 status 字段表达生命周期，**不**软删除）；**不**
// 预留任何 §5.9 之外的字段。
type UserCosmeticItem struct {
	ID             uint64     `gorm:"column:id;primaryKey;autoIncrement"`
	UserID         uint64     `gorm:"column:user_id;not null"`
	CosmeticItemID uint64     `gorm:"column:cosmetic_item_id;not null"`
	Status         int8       `gorm:"column:status;not null;default:1"`
	Source         int8       `gorm:"column:source;not null;default:1"`
	SourceRefID    *uint64    `gorm:"column:source_ref_id"`
	ObtainedAt     time.Time  `gorm:"column:obtained_at;not null;default:CURRENT_TIMESTAMP(3)"`
	ConsumedAt     *time.Time `gorm:"column:consumed_at"`
	CreatedAt      time.Time  `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
	UpdatedAt      time.Time  `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

// TableName 显式声明 "user_cosmetic_items"。
func (UserCosmeticItem) TableName() string { return "user_cosmetic_items" }
