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

// HomeEquipRow 是 ListEquipsForHome 的 JOIN 投影行（Story 26.6 引入）。
//
// **轻量结构体接收 JOIN 投影列，不复用 UserPetEquip 全 struct** —— JOIN 跨
// 3 表（user_pet_equips / user_cosmetic_items / cosmetic_items）的列不在单
// struct 上，显式裁字段避免误读未 SELECT 列的 zero-value，与
// cosmetic_item_repo 轻量 struct 同模式。
//
// 字段类型与 3 表对应列 1:1：slot/rarity int8（TINYINT）、id 列 uint64
// （BIGINT UNSIGNED）、name/asset_url string（VARCHAR）。
type HomeEquipRow struct {
	Slot               int8   `gorm:"column:slot"`
	UserCosmeticItemID uint64 `gorm:"column:user_cosmetic_item_id"`
	CosmeticItemID     uint64 `gorm:"column:cosmetic_item_id"`
	Name               string `gorm:"column:name"`
	Rarity             int8   `gorm:"column:rarity"`
	AssetURL           string `gorm:"column:asset_url"`
}

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

	// FindUserCosmeticItemIDByPetSlotForUpdate 取 FOR UPDATE 排他锁查
	// (pet_id, slot) 对应的 user_cosmetic_item_id（Story 26.4 引入；V1
	// §8.4 服务端逻辑步骤 5，fix-review 26-1 r2 [P1] 锁定）。
	//
	// SQL: SELECT user_cosmetic_item_id FROM user_pet_equips
	//      WHERE pet_id = ? AND slot = ? LIMIT 1 FOR UPDATE
	//
	// **MySQL 8.0 语法红线**：LIMIT 必须在 FOR UPDATE **之前**
	// （`... FOR UPDATE LIMIT 1` 在 MySQL 5.7+ 是 ER_PARSE_ERROR 1064；
	// GORM 不重写 Raw SQL 顺序，与 room_member_repo.ExistsForShareByRoomAndUser
	// FOR SHARE 语法约束同源）。用 Raw + Scan 路径（**不**用 GORM
	// Clauses(clause.Locking{...})）以显式可控 LIMIT/FOR UPDATE 顺序。
	//
	// **必须在事务内调用**（与 §8.4 步骤 6 DELETE + UPDATE 实例 status
	// 同事务原子提交；事务外 FOR UPDATE 锁立即释放——autocommit 模式下
	// SELECT 完成即 commit，并发卸下串行化失效，V1 §8.4 行 1657 钦定的
	// "并发卸下串行化"契约破坏，SELECT-then-DELETE TOCTOU 竞态重现）。
	//
	// **NotFound 语义**：该 slot 无装备（0 行）→ 返 (0, ErrUserPetEquipNotFound)
	// 哨兵（**合法 case**，**非**异常 —— service 层用 errors.Is 区分"slot
	// 无装备 → 5004 装备槽位不匹配"vs "DB 异常 → 1009"）。**注意**：Raw +
	// Scan 在 0 行时**不**返 gorm.ErrRecordNotFound 而是保持 dst zero-value
	// 不报错（与 room_member_repo.ExistsForShareByRoomAndUser 行 457-458
	// 注释同源）—— 故须**显式判 0 行**：用 Scan(...).RowsAffected == 0 →
	// 返哨兵，**不**靠 errors.Is(err, gorm.ErrRecordNotFound)。
	// query 失败 → 返 (0, raw error 透传给 service（service 包成 1009）)。
	FindUserCosmeticItemIDByPetSlotForUpdate(ctx context.Context, petID uint64, slot int8) (uint64, error)

	// DeleteByPetSlotInTxReturningAffected 删除 (pet_id, slot) 对应的
	// user_pet_equips 行并返回 RowsAffected（Story 26.4 引入；V1 §8.4
	// 服务端逻辑步骤 6，fix-review 26-1 r2 [P1] 锁定）。
	//
	// SQL: DELETE FROM user_pet_equips WHERE pet_id = ? AND slot = ?
	//
	// **必须在事务内调用**（与同事务的 §8.4 步骤 5 FOR UPDATE 行锁查 +
	// UPDATE 实例 status 回 in_bag 一起原子提交；理由同 DeleteByPetSlotInTx）。
	//
	// **与 DeleteByPetSlotInTx 的关键差异**（**不**复用 DeleteByPetSlotInTx）：
	// 本方法返 RowsAffected 让 service 层做契约级冗余兜底分流（V1 §8.4
	// 行 1609 / 1651 / 1657 钦定，与 room_member_repo.DeleteByRoomAndUser
	// 行 432-441 同根因模式）：
	//   - == 1：删除成功（happy path）→ service 继续 UPDATE 实例 status
	//   - == 0：步骤 5 与本步之间该行已被并发事务删除（理论上已由步骤 5
	//     FOR UPDATE 排他锁阻止，本检查为不依赖锁实现细节的契约级冗余兜底）
	//     → service 层**回滚事务 + 返回 5004**（**禁止** 0 affected rows
	//     继续 commit 误返 unequipped: true）
	//   - >  1：理论不可能（user_pet_equips 有 uk_pet_slot UNIQUE(pet_id,
	//     slot)，最多 1 行匹配）；service 层兜底视为成功路径（!= 0 即继续）
	//
	// 返 (result.RowsAffected, result.Error)：result.Error != nil →
	// (0, raw error 透传给 service（service 包成 1009）)；否则
	// (result.RowsAffected, nil)。
	DeleteByPetSlotInTxReturningAffected(ctx context.Context, petID uint64, slot int8) (int64, error)

	// ListEquipsForHome 单 SQL JOIN 查某 pet 当前全部装备（GET /home pet.equips
	// 数据源，Story 26.6 引入；V1 §5.1 节点 9 真实数据 + epics.md §Story 26.6）。
	//
	// 主查 SQL（单查，禁止 N+1 —— epics.md §Story 26.6 AC4 edge 钦定）:
	//   SELECT upe.slot, upe.user_cosmetic_item_id,
	//          ci.id AS cosmetic_item_id, ci.name, ci.rarity, ci.asset_url
	//   FROM user_pet_equips upe
	//   JOIN user_cosmetic_items uci ON uci.id = upe.user_cosmetic_item_id
	//   JOIN cosmetic_items ci       ON ci.id  = uci.cosmetic_item_id
	//   WHERE upe.user_id = ? AND upe.pet_id = ?
	//   ORDER BY upe.slot ASC
	//
	// 走 0016 落地的 idx_user_pet (user_id, pet_id) 索引（数据库设计 §5.10）。
	// ORDER BY slot ASC：client 渲染稳定顺序（防 grid 抖动；单 pet 单 slot
	// UNIQUE，slot ASC 已是全序，与 catalog ORDER BY 同根因模式）。
	//
	// **INNER JOIN 自然 skip 配置缺失（AC3）**：若某 upe 行对应的
	// cosmetic_items 配置被 admin 物理删（理论不该，与 §8.2 态 C 同源），该行
	// 不进 JOIN 结果。为让 service 层 log warning，本方法**额外发 1 个 O(1)
	// COUNT**（`SELECT COUNT(*) FROM user_pet_equips WHERE user_id=? AND
	// pet_id=?`）返 rawCount；service 据 rawCount > len(rows) 判定有配置缺失
	// → log warn。**COUNT 是固定 1 次额外查询（与装备件数无关），不违反
	// N+1 约束**（N+1 指"每件装备 1 查"，此处主查仍是单 JOIN）。
	//
	// 空结果（pet 未穿任何装备）→ 返 ([]HomeEquipRow{}（**非 nil**), 0, nil)；
	// service 透传为 pet.equips=[] 非 error（AC2 happy: 没穿装备 → []）。
	// 主 query 失败 → 返 (nil, 0, raw error 透传)（service 包成 1009）。
	ListEquipsForHome(ctx context.Context, userID, petID uint64) (rows []HomeEquipRow, rawCount int64, err error)
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

// FindUserCosmeticItemIDByPetSlotForUpdate 实装：Raw FOR UPDATE 行锁 SELECT
// + Scan 显式判 0 行（详见接口注释；与 room_member_repo
// .ExistsForShareByRoomAndUser FOR SHARE Raw+Scan 模式同源，锁子句改 FOR
// UPDATE）。
func (r *userPetEquipRepo) FindUserCosmeticItemIDByPetSlotForUpdate(ctx context.Context, petID uint64, slot int8) (uint64, error) {
	db := tx.FromContext(ctx, r.db)
	var uciID uint64
	// MySQL 8.0 SQL syntax: LIMIT 必须在 locking clause（FOR UPDATE）**之前**；
	// `... FOR UPDATE LIMIT 1` 在 MySQL 5.7+ 是 ER_PARSE_ERROR (1064)。GORM 不会
	// 重写顺序，raw SQL 必须按 MySQL 钦定顺序写（与 room_member_repo
	// .ExistsForShareByRoomAndUser 行 463-466 注释钦定一致）。
	result := db.WithContext(ctx).
		Raw("SELECT user_cosmetic_item_id FROM user_pet_equips WHERE pet_id = ? AND slot = ? LIMIT 1 FOR UPDATE", petID, slot).
		Scan(&uciID)
	if result.Error != nil {
		// Raw + Scan 0 行不产 gorm.ErrRecordNotFound（与 ExistsForShareByRoomAndUser
		// 行 457-458 注释同源）；走到这里是真 query 失败 → raw 透传（service 包 1009）
		return 0, result.Error
	}
	if result.RowsAffected == 0 {
		// 该 slot 无装备 → 合法 case，返哨兵（service 翻 5004，**非** 1009）
		return 0, ErrUserPetEquipNotFound
	}
	return uciID, nil
}

// DeleteByPetSlotInTxReturningAffected 实装：DELETE WHERE pet_id=? AND slot=?
// 返 (result.RowsAffected, result.Error)（详见接口注释；与
// room_member_repo.DeleteByRoomAndUser 行 432-441 1:1 同模式）。
func (r *userPetEquipRepo) DeleteByPetSlotInTxReturningAffected(ctx context.Context, petID uint64, slot int8) (int64, error) {
	db := tx.FromContext(ctx, r.db)
	result := db.WithContext(ctx).
		Where("pet_id = ? AND slot = ?", petID, slot).
		Delete(&UserPetEquip{})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// ListEquipsForHome 实装：单 SQL 3 表 INNER JOIN 主查（显式列裁剪 + ORDER BY
// slot ASC + tx.FromContext）+ 1 个 O(1) COUNT 校验查。详见接口注释
// （V1 §5.1 节点 9 真实数据 + epics.md §Story 26.6 AC4 禁止 N+1）。
//
// 与本文件既有方法风格一致优先：本方法无锁子句，用 GORM
// Table().Select().Joins().Where().Order().Find() builder（显式列裁剪），
// 0 行 → []HomeEquipRow{} 兜底（与 cosmetic_item_repo.ListEnabledForCatalog
// 0 行非 nil 兜底同模式），**不**走 Raw（Raw 留给 FOR UPDATE 锁语法特例）。
func (r *userPetEquipRepo) ListEquipsForHome(ctx context.Context, userID, petID uint64) ([]HomeEquipRow, int64, error) {
	db := tx.FromContext(ctx, r.db).WithContext(ctx)

	var rows []HomeEquipRow
	// 显式列裁剪（不依赖 GORM 自动 SELECT *）：与 V1 §5.1 行 475-484 节点 9
	// 真实数据示例 6 字段 1:1（slot / userCosmeticItemId / cosmeticItemId /
	// name / rarity / assetUrl）。`ci.id AS cosmetic_item_id` 让 GORM 按
	// HomeEquipRow.CosmeticItemID 的 column tag 映射。
	err := db.
		Table("user_pet_equips upe").
		Select("upe.slot AS slot, upe.user_cosmetic_item_id AS user_cosmetic_item_id, "+
			"ci.id AS cosmetic_item_id, ci.name AS name, ci.rarity AS rarity, ci.asset_url AS asset_url").
		Joins("JOIN user_cosmetic_items uci ON uci.id = upe.user_cosmetic_item_id").
		Joins("JOIN cosmetic_items ci ON ci.id = uci.cosmetic_item_id").
		Where("upe.user_id = ? AND upe.pet_id = ?", userID, petID).
		Order("upe.slot ASC").
		Find(&rows).Error
	if err != nil {
		return nil, 0, err
	}
	// GORM Find 在 0 行时一般返回 nil slice；显式兜底空 slice 让 service 层
	// 调用方不需要 nil-check（AC2 happy: 没穿装备 → pet.equips=[] 非 null）。
	if rows == nil {
		rows = []HomeEquipRow{}
	}

	// O(1) COUNT 校验查（与装备件数无关的固定 1 次额外查询，**不**是 N+1）：
	// user_pet_equips 真实行数 vs JOIN 后行数。rawCount > len(rows) 说明有
	// 装备的 cosmetic_items 配置被删（INNER JOIN 过滤掉了），service 据此
	// log warning（AC3）。
	var rawCount int64
	if err := db.
		Model(&UserPetEquip{}).
		Where("user_id = ? AND pet_id = ?", userID, petID).
		Count(&rawCount).Error; err != nil {
		return nil, 0, err
	}

	return rows, rawCount, nil
}
