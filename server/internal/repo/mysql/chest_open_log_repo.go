package mysql

import (
	"time"
)

// ChestOpenLog 是 chest_open_logs 表的完整 GORM domain struct（Story 20.4 引入；
// 与 server/migrations/0013_init_chest_open_logs.up.sql 真实 schema 1:1 对齐）。
//
// 字段（与数据库设计.md §5.7 + 0013_init_chest_open_logs.up.sql 1:1 对齐）：
//   - ID:                       BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§5.7 + §3.1 主键约定）
//   - UserID:                   BIGINT UNSIGNED NOT NULL（归属用户 id，语义上 ref users.id）
//   - ChestID:                  BIGINT UNSIGNED NOT NULL（被开启的宝箱 id，语义上 ref user_chests.id）
//   - CostSteps:                INT UNSIGNED NOT NULL（实际消耗步数；节点 7 阶段固定 1000）
//   - RewardUserCosmeticItemID: BIGINT UNSIGNED NOT NULL（产出的装扮实例 id；
//                               **节点 7 阶段固定 0 占位** —— V1接口设计 §7.2.4h +
//                               数据库设计 §8 注解钦定；节点 8 Epic 23 Story 23.5
//                               切换为真实 user_cosmetic_items.id）
//   - RewardCosmeticItemID:     BIGINT UNSIGNED NOT NULL（产出的装扮配置 id，
//                               语义上 ref cosmetic_items.id）
//   - RewardRarity:             TINYINT NOT NULL（§6.9 枚举：1=common / 2=rare /
//                               3=epic / 4=legendary）
//   - CreatedAt:                DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
//
// **关键**：本 struct **无** UpdatedAt 字段 —— chest_open_logs 是 append-only
// 日志表，**无** UPDATE 语义（与 0006 user_step_sync_logs / UserStepSyncLog struct
// 同模式）。
//
// 表层普通索引（idx_user_id_created_at / idx_reward_cosmetic_item_id）由 SQL DDL
// 定义，**不**在 struct tag 中重复声明（与 ADR-0003 §3.2 "禁止 GORM AutoMigrate"
// 同源；GORM struct 仅为 Find / Create 提供字段映射，**不**作为 schema 真相源；
// 与 Story 20.2 落地的 CosmeticItem / Story 17.2 落地的 EmojiConfig / Story 11.2
// 落地的 RoomMember / Room struct 同模式）。
//
// **范围红线**：本 struct 仅为下游 Story 20.6（POST /chest/open 事务步骤 5h
// 写一条 chest_open_logs 行）/ Story 23.5（节点 8 修改开箱事务回填
// reward_user_cosmetic_item_id）/ 未来运营接口提供字段映射；本 story 阶段
// **不**新建 ChestOpenLogRepo interface / 实装 Create / FindByUserID 等方法
// （YAGNI；20.6 落地 Create 方法 + 未来运营 epic 落地查询方法）。
type ChestOpenLog struct {
	ID                       uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	UserID                   uint64    `gorm:"column:user_id;not null"`
	ChestID                  uint64    `gorm:"column:chest_id;not null"`
	CostSteps                uint32    `gorm:"column:cost_steps;not null"`
	RewardUserCosmeticItemID uint64    `gorm:"column:reward_user_cosmetic_item_id;not null"`
	RewardCosmeticItemID     uint64    `gorm:"column:reward_cosmetic_item_id;not null"`
	RewardRarity             int8      `gorm:"column:reward_rarity;not null"`
	CreatedAt                time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

// TableName 显式声明 "chest_open_logs"。
func (ChestOpenLog) TableName() string { return "chest_open_logs" }
