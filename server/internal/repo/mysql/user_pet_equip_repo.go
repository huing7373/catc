package mysql

import (
	"time"
)

// UserPetEquip 是 user_pet_equips 表的完整 GORM domain struct
// （Story 26.2 引入；与 server/migrations/0016_init_user_pet_equips.up.sql
// 真实 schema 1:1 对齐）。
//
// 字段（与数据库设计.md §5.10 + 0016_init_user_pet_equips.up.sql 1:1 对齐；
// **全列 NOT NULL → 全部值类型，无任何指针字段** —— 与 Story 23.2 落地的
// UserCosmeticItem 有 SourceRefID *uint64 / ConsumedAt *time.Time 指针映射
// NULL 可空列**正好相反**；user_pet_equips 无任何可空列，本 struct 不出现
// 任何 *uint64 / *time.Time）：
//   - ID:                 BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT（§5.10 +
//                         §3.1 主键约定）
//   - UserID:             BIGINT UNSIGNED NOT NULL（归属用户；语义上 ref
//                         users.id，**不**建 FK，与本设计其他表一致）
//   - PetID:              BIGINT UNSIGNED NOT NULL（归属宠物；语义上 ref
//                         pets.id，**不**建 FK）
//   - Slot:               TINYINT NOT NULL（§6.8 枚举：1=hat / 2=gloves /
//                         3=glasses / 4=neck / 5=back / 6=body / 7=tail /
//                         99=other；**int8** 映射 TINYINT，与 CosmeticItem.Slot
//                         int8 跨表同类型对齐 —— 用 uint8 会让跨表 slot 比较
//                         类型不一致；**无 DEFAULT** —— slot 由 equip 时
//                         cosmetic_items 配置决定必传，非客户端传入）
//   - UserCosmeticItemID: BIGINT UNSIGNED NOT NULL（被穿戴的装扮实例 id；
//                         语义上 ref user_cosmetic_items.id，**不**建 FK）
//   - CreatedAt:          DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
//   - UpdatedAt:          DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3)
//                         ON UPDATE CURRENT_TIMESTAMP(3)
//
// 表层**两个** UNIQUE 约束（uk_pet_slot (pet_id, slot) 复合 +
// uk_user_cosmetic_item_id (user_cosmetic_item_id) 单列）+ 普通索引
// idx_user_pet (user_id, pet_id) 由 SQL DDL 定义，**不**在 struct tag 中
// 重复声明（与 ADR-0003 §3.2 "禁止 GORM AutoMigrate" 同源；GORM struct 仅为
// Find / Create 提供字段映射，**不**作为 schema 真相源；与 Story 23.2 落地的
// UserCosmeticItem / Story 20.2 落地的 CosmeticItem / Story 11.2 落地的
// RoomMember struct 同模式）。
//
// **范围红线**：本 struct 仅为下游 Story 26.3（POST /cosmetics/equip 事务，
// 含同槽换装 INSERT user_pet_equips + 删旧行 + status 1↔2 推进）/ Story 26.4
// （POST /cosmetics/unequip 事务 DELETE 行）/ Story 26.6（GET /home
// pet.equips JOIN 查询）提供字段映射；本 story 阶段**不**新建
// UserPetEquipRepo interface / 实装 InsertInTx / DeleteByPetSlotInTx /
// FindByPetSlot / ListByUserPetForHome 等任何方法（YAGNI；对标 Story 23.2
// 阶段 user_cosmetic_item_repo.go 仅 struct+TableName 的最小集 —— 注意
// user_cosmetic_item_repo.go 现含 UserCosmeticItemRepo interface 是 Story
// 23.4 / 23.5 后续扩展加的，**不**是 23.2 阶段产物，本 story 对应 23.2 阶段
// 最小集，**不**提前加 interface / 方法）。
//
// **不**引入 gorm.Model（避免引入 DeletedAt / 软删除字段，与 0016 真实
// schema 不符；user_pet_equips 用 DELETE 行表达卸下，**不**软删除）；**不**
// 预留任何 §5.10 之外的字段。
type UserPetEquip struct {
	ID                 uint64    `gorm:"column:id;primaryKey;autoIncrement"`
	UserID             uint64    `gorm:"column:user_id;not null"`
	PetID              uint64    `gorm:"column:pet_id;not null"`
	Slot               int8      `gorm:"column:slot;not null"`
	UserCosmeticItemID uint64    `gorm:"column:user_cosmetic_item_id;not null"`
	CreatedAt          time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
	UpdatedAt          time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

// TableName 显式声明 "user_pet_equips"。
func (UserPetEquip) TableName() string { return "user_pet_equips" }
