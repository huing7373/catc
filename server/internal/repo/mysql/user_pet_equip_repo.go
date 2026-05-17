package mysql

import (
	"context"
	stderrors "errors"
	"strings"
	"time"

	"github.com/go-sql-driver/mysql"
	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/repo/tx"
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

// UserPetEquipRepo 是 user_pet_equips 表的访问接口（Story 26.3 **首次**落地 ——
// Story 26.2 仅落地 UserPetEquip GORM struct + TableName() 最小骨架，**没有**
// interface / 任何方法；本 story 在同文件追加 interface / impl，与
// user_cosmetic_item_repo.go Story 23.4 阶段在 struct 下追加 interface/impl
// 同文件组织模式一致）。
//
// service 注入 + 单测 mock 用。
//
// **范围红线（YAGNI）**：方法最小集仅本 story（26.3 equip 事务）+ Story 26.4
// （unequip 事务）明确会复用的部分；**不**预实装 Story 26.6（GET /home
// pet.equips JOIN 查询）的 List 类方法（与 user_cosmetic_item_repo.go 行 84-90
// 范围红线一致）。
type UserPetEquipRepo interface {
	// FindByPetSlot 查 (pet_id, slot) 对应的 user_pet_equips 行（V1 §8.3
	// 服务端逻辑步骤 8 同槽换装判定数据源；走 0016 落地的 uk_pet_slot 索引）。
	//
	// SQL: SELECT * FROM user_pet_equips WHERE pet_id = ? AND slot = ? LIMIT 1
	//
	// **NotFound 语义**：该 slot 无装备 → 返 (nil, ErrUserPetEquipNotFound)
	// 哨兵（**合法 case**，**非**异常 —— service 层用 errors.Is 区分"slot 无
	// 装备 → 跳过同槽换装"vs "DB 异常 → 1009"，与 step_sync_log_repo
	// .FindLatestByUserAndDate NotFound 哨兵同模式）。query 失败 → 返 raw error
	// 透传（service 包成 1009）。
	//
	// 26.4 unequip 复用本方法（先查目标 slot 当前装备行再 DELETE）。
	FindByPetSlot(ctx context.Context, petID uint64, slot int8) (*UserPetEquip, error)

	// DeleteByPetSlotInTx 删除 (pet_id, slot) 对应的 user_pet_equips 行（V1
	// §8.3 服务端逻辑步骤 8 同槽换装"删旧 user_pet_equips 行"；Story 26.4
	// unequip 事务亦复用）。
	//
	// SQL: DELETE FROM user_pet_equips WHERE pet_id = ? AND slot = ?
	//
	// **必须在事务内调用**（与同事务的旧实例 status 回 in_bag + INSERT 新行 +
	// 新实例 status→equipped 一起原子提交；ADR-0007 §2.4 + 数据库设计 §8.4
	// "全部同事务" —— 任一步失败本 DELETE 必须跟随回滚，杜绝"旧装备已删但新
	// 装备没装上"数据不一致）。
	//
	// **err 二分**（与 pet_repo.UpdateCurrentStateByID 同模式）：err == nil →
	// 成功（**不**读 RowsAffected —— service 层在 FindByPetSlot 命中后才调本
	// 方法，目标行必存在；RowsAffected==0 理论不发生，即便发生也由后续
	// INSERT/UNIQUE 兜底，不在本方法分流）；err != nil → 透传 raw error
	// （service 包成 1009）。
	DeleteByPetSlotInTx(ctx context.Context, petID uint64, slot int8) error

	// InsertInTx 在事务内插入一行 user_pet_equips（V1 §8.3 服务端逻辑步骤 9
	// "绑定"；GORM 成功后回填 e.ID）。
	//
	// **必须在事务内调用**（与同事务的 status 推进一起原子提交，理由同
	// DeleteByPetSlotInTx）。
	//
	// **ER_DUP_ENTRY 1062 双路径翻译**（V1 §8.3 关键约束行 1560 + NFR11；
	// 模式抄 room_member_repo.go Create 行 366-384）：user_pet_equips 表有两个
	// 唯一约束（0016 schema）：
	//   - uk_pet_slot (pet_id, slot)               → ErrUserPetEquipPetSlotDuplicate
	//   - uk_user_cosmetic_item_id (user_cosmetic_item_id) → ErrUserPetEquipItemDuplicate
	//
	// 不解析 mysql.MySQLError.Message 的引号 / locale 部分（国际化不可靠）——
	// 用 Message contains "uk_pet_slot" / "uk_user_cosmetic_item_id" substring
	// 是稳定 contract（key 名 part 在所有版本 + 语言下都是英文 ASCII）。
	// 两个哨兵 service 层均 → 1009。
	//
	// **fallback 路径**：1062 但 Message 既不含 uk_pet_slot 也不含
	// uk_user_cosmetic_item_id → raw error 透传给 service（service 兜底
	// 1009，与 room_member_repo.go fallback 行 379-380 同模式）。0016 两个
	// UNIQUE 约束已穷举，本 fallback 理论不触发。
	InsertInTx(ctx context.Context, e *UserPetEquip) error
}

// userPetEquipRepo 是 UserPetEquipRepo 的默认实装。
type userPetEquipRepo struct {
	db *gorm.DB
}

// NewUserPetEquipRepo 构造 UserPetEquipRepo。Story 26.3 引入
// （与 NewUserCosmeticItemRepo / NewCosmeticItemRepo 同模式）。
func NewUserPetEquipRepo(db *gorm.DB) UserPetEquipRepo {
	return &userPetEquipRepo{db: db}
}

// FindByPetSlot 实装：单 SELECT WHERE pet_id=? AND slot=? LIMIT 1。
// NotFound → ErrUserPetEquipNotFound 哨兵（详见接口注释；与
// pet_repo.FindDefaultByUserID gorm.ErrRecordNotFound → 哨兵同模式）。
func (r *userPetEquipRepo) FindByPetSlot(ctx context.Context, petID uint64, slot int8) (*UserPetEquip, error) {
	db := tx.FromContext(ctx, r.db)
	var e UserPetEquip
	err := db.WithContext(ctx).
		Where("pet_id = ? AND slot = ?", petID, slot).
		First(&e).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserPetEquipNotFound
		}
		return nil, err
	}
	return &e, nil
}

// DeleteByPetSlotInTx 实装：DELETE WHERE pet_id=? AND slot=?。
// 与接口注释一致 —— err 二分透传，不读 RowsAffected。
func (r *userPetEquipRepo) DeleteByPetSlotInTx(ctx context.Context, petID uint64, slot int8) error {
	db := tx.FromContext(ctx, r.db)
	return db.WithContext(ctx).
		Where("pet_id = ? AND slot = ?", petID, slot).
		Delete(&UserPetEquip{}).Error
}

// InsertInTx 实装：INSERT 一行 user_pet_equips；GORM 回填 e.ID。1062 → 按
// Message 含约束名分流双哨兵（模式抄 room_member_repo.go Create 行 366-384，
// 改约束名为 uk_pet_slot / uk_user_cosmetic_item_id）。
func (r *userPetEquipRepo) InsertInTx(ctx context.Context, e *UserPetEquip) error {
	db := tx.FromContext(ctx, r.db)
	err := db.WithContext(ctx).Create(e).Error
	if err != nil {
		var mysqlErr *mysql.MySQLError
		if stderrors.As(err, &mysqlErr) && mysqlErr.Number == mysqlErrCodeDupEntry {
			msg := mysqlErr.Message
			if strings.Contains(msg, "uk_pet_slot") {
				return ErrUserPetEquipPetSlotDuplicate
			}
			if strings.Contains(msg, "uk_user_cosmetic_item_id") {
				return ErrUserPetEquipItemDuplicate
			}
			// 极罕见：1062 但既不含 uk_pet_slot 也不含 uk_user_cosmetic_item_id → raw 透传
		}
		return err
	}
	return nil
}
