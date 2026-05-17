package mysql

import (
	"context"
	stderrors "errors"
	"time"

	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/repo/tx"
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

// UserCosmeticItemRepo 是 user_cosmetic_items 表的访问接口（Story 23.4 **首次**
// 落地 —— Story 23.2 仅落地 UserCosmeticItem GORM struct + TableName() 最小骨架，
// **没有** interface / 任何方法；本 story 在同文件追加 interface / impl）。
//
// service 注入 + 单测 mock 用（与 cosmetic_item_repo.go CosmeticItemRepo 同模式 ——
// struct + interface + impl 同文件组织）。
//
// **范围红线**：本 story（23.4）仅加 inventory 只读查询方法 ListByUserForInventory
// （GET /cosmetics/inventory 数据源）；**不**提前实装 BatchCreate / InsertInTx
// （开箱补入仓写方法）/ MarkConsumed（合成消耗）/ ListByUserAndStatus 等任何
// 写方法或其他查询方法（YAGNI；BatchCreate 写方法是 Story 23.5 钦定范围 —— 与
// 23.2 struct 注释行 44-52 范围红线一致；与 cosmetic_item_repo.go 仅含已落地
// 业务路径方法同源）。
type UserCosmeticItemRepo interface {
	// ListByUserForInventory 返回某用户 status IN (1,2)（in_bag + equipped）的所有
	// user_cosmetic_items 实例（GET /cosmetics/inventory 数据源，V1 §8.2 服务端
	// 逻辑步骤 2）。
	//
	// SQL: SELECT id, cosmetic_item_id, status FROM user_cosmetic_items
	//      WHERE user_id = ? AND status IN (1, 2)
	//
	// **status 过滤理由（§8.2 行 1340 钦定）**：仅 in_bag(1) + equipped(2)；
	// consumed(3)（合成已消耗）/ invalid(4)（无效）被 SQL 层 `WHERE status IN (1,2)`
	// 过滤 —— consumed / invalid 实例绝不进 inventory（过滤在 repo SQL 层做，
	// 不靠 service 层二次过滤，与 §8.2 服务端逻辑步骤 2 1:1）。
	//
	// 显式 Select 3 列（id / cosmetic_item_id / status）—— client 不需要 source /
	// obtained_at / consumed_at 等列（§8.2 instances[] 仅含 userCosmeticItemId +
	// status），与 cosmetic_item_repo.ListEnabledForCatalog 显式裁字段同模式。
	//
	// 空结果 → []UserCosmeticItem{}（**非 nil**）；service 透传为 {groups:[]}
	// 非 error（§8.2 行 1341：用户无任何实例返 {groups:[]} code=0 不报错）。
	// query 失败 → 返 raw error 透传（service 包成 1009）。
	ListByUserForInventory(ctx context.Context, userID uint64) ([]UserCosmeticItem, error)

	// CreateInTx 在事务内插入一行 user_cosmetic_items（开箱事务"创建实例"步骤数据出口，
	// Story 23.5 引入 —— epics.md §Story 23.5 + V1 §7.2.4h 节点 8 + DB §8.3 钦定）。
	//
	// GORM 在成功后回填 item.ID（AUTO_INCREMENT）—— 调用方拿这个真实 id 回填
	// chest_open_logs.reward_user_cosmetic_item_id + response.reward.userCosmeticItemId。
	//
	// **必须在事务内调用**（与同事务的扣步数 / 写 chest_open_logs / 刷新 chest /
	// MarkSuccess 一起原子提交；ADR-0007 §2.4 + DB §8.3"全部同事务"——
	// 任一步失败本 INSERT 必须跟随回滚，杜绝"孤儿实例 + 步数没扣"数据不一致）。
	//
	// query 失败 → 返 raw error 透传（service 包成 1009，与同事务其他写步骤一致）。
	//
	// **范围红线**：本 story（23.5）加 CreateInTx 入仓写方法（开箱事务步骤 5g.5
	// + dev /dev/grant-cosmetic-batch 批量发放复用，AC6）；status 1↔2↔3 推进 /
	// consumed 写方法归 Epic 26（穿戴）/ Epic 32-33（合成），本 story **不**预实装
	// （YAGNI；与既有 ListByUserForInventory 范围红线一致）。
	CreateInTx(ctx context.Context, item *UserCosmeticItem) error

	// FindByIDForEquip 按实例 id 查一行 user_cosmetic_items（V1 §8.3 服务端
	// 逻辑步骤 4 查实例归属，Story 26.3 引入）。
	//
	// SQL: SELECT id, cosmetic_item_id, status, user_id FROM user_cosmetic_items
	//      WHERE id = ? LIMIT 1
	//
	// **仅按 id 查，禁止加 `AND user_id = ?` 过滤**（V1 §8.3 行 1492 + fix-review
	// 26-1 r1 [P2] 强制锁定）：service 层拿到行后比对 user_id 自行分流
	//   - 行不存在（id 完全无 row）→ 返 (nil, ErrUserCosmeticItemNotFound)
	//     哨兵 → service 翻译 5001 道具不存在；
	//   - 行存在 → 返 *UserCosmeticItem（service 比对 row.UserID != 当前用户
	//     → 5002；==当前用户 → 继续校 status）。
	// 合并 `WHERE id=? AND user_id=?` 查不到即 5001 的实装被契约**禁止**——会
	// 使 epics.md §Story 26.3 AC "实例不属于当前用户 → 5002" 永不可达。
	//
	// 显式 Select 4 列（id / cosmetic_item_id / status / user_id）——
	// equip 步骤 4-5 只需这 4 列（source / obtained_at 等不需要），与
	// ListByUserForInventory 显式裁字段同模式。query 失败（非 NotFound）→ 返
	// raw error 透传（service 包成 1009）。
	//
	// **范围红线**：本 story（26.3）仅加 equip 步骤 4 查询方法；**不**预实装
	// 26.4/26.6/Epic 32 的查询方法（YAGNI；与该 interface 行 84-90 范围红线一致）。
	FindByIDForEquip(ctx context.Context, id uint64) (*UserCosmeticItem, error)

	// UpdateStatusInTx 在事务内按主键更新 user_cosmetic_items.status 单列
	// （V1 §8.3 服务端逻辑步骤 8 旧实例 status 回 1=in_bag + 步骤 9 当前实例
	// status 推进 2=equipped；Story 26.3 引入）。
	//
	// SQL: UPDATE user_cosmetic_items SET status=?, updated_at=NOW(3) WHERE id=?
	//
	// **必须在事务内调用**（与同事务的 user_pet_equips DELETE/INSERT 一起原子
	// 提交；ADR-0007 §2.4 + 数据库设计 §8.4 "全部同事务"）。用
	// Update("status", v) 单字段（与 pet_repo.UpdateCurrentStateByID 同模式
	// —— status int8 无 nil-skip 陷阱，不需要 Updates(map) 路径；**不**用
	// Save 避免写全字段引入并发数据丢失）。
	//
	// **err 二分**（与 pet_repo.UpdateCurrentStateByID 同模式）：err == nil →
	// 成功（**不**读 RowsAffected —— service 已在步骤 4 FindByIDForEquip 确认
	// 行存在；本 UPDATE 把 updated_at 也写新值，幂等场景 RowsAffected 语义不
	// 可靠，不依赖）；err != nil → 透传 raw error（service 包成 1009）。
	//
	// **范围红线**：本 story（26.3）仅加 status 1↔2 推进；consumed(3) 写方法
	// 归 Epic 32-33 合成，**不**预实装（YAGNI）。
	UpdateStatusInTx(ctx context.Context, id uint64, status int8) error
}

// userCosmeticItemRepo 是 UserCosmeticItemRepo 的默认实装。
type userCosmeticItemRepo struct {
	db *gorm.DB
}

// NewUserCosmeticItemRepo 构造 UserCosmeticItemRepo。Story 23.4 引入
// （与 NewCosmeticItemRepo 同模式）。
func NewUserCosmeticItemRepo(db *gorm.DB) UserCosmeticItemRepo {
	return &userCosmeticItemRepo{db: db}
}

// ListByUserForInventory 实装：单 SELECT，显式 3 列 + WHERE user_id=? AND
// status IN (1,2)。详见 UserCosmeticItemRepo.ListByUserForInventory 接口注释
// （§8.2 服务端逻辑步骤 2 钦定）。
//
// 与 cosmetic_item_repo.go ListEnabledForCatalog（行 149-177）1:1 同模式：
// tx.FromContext(ctx, r.db).WithContext(ctx) + 显式 Select + Where + Find +
// nil slice 兜底；差异：无 ORDER BY（两级排序在 service 层 sort.Slice 做，
// §8.2 行 1360 钦定不依赖 DB 天然顺序）、WHERE 条件不同（user_id + status IN）。
func (r *userCosmeticItemRepo) ListByUserForInventory(ctx context.Context, userID uint64) ([]UserCosmeticItem, error) {
	// 用 tx.FromContext 取 db handle：事务外调用走 r.db；事务内调用走 txCtx
	// 注入的 tx 句柄（与 cosmetic_item_repo.ListEnabledForCatalog / emoji_repo.List
	// 同模式；本 story inventory 只读不开事务，但保持模式一致让 future 扩展
	// 无需改 method body）。
	db := tx.FromContext(ctx, r.db).WithContext(ctx)

	var rows []UserCosmeticItem
	// 显式 Select 3 列（不依赖 GORM 自动 SELECT *），与 §8.2 服务端逻辑步骤 2
	// 钦定 1:1 对齐；user_id / source / source_ref_id / obtained_at /
	// consumed_at / created_at / updated_at **不**在 SELECT —— client 不需要，
	// GORM Scan 填 zero-value 安全（service 层 DTO 转换只读 ID /
	// CosmeticItemID / Status，与 cosmetic_item_repo.go 行 158-161 同模式）。
	//
	// status IN (1,2) 过滤在 SQL 层做（§8.2 行 1340）—— []int8{1, 2} 用 GORM
	// `IN ?` 占位（status TINYINT → Go int8，与 UserCosmeticItem.Status int8 一致）；
	// consumed(3) / invalid(4) 绝不进 inventory。
	err := db.
		Select("id, cosmetic_item_id, status").
		Where("user_id = ? AND status IN ?", userID, []int8{1, 2}).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	// GORM Find 在 0 行时返回空 slice 而非 nil（与 cosmetic_item_repo.ListEnabledForCatalog
	// 同模式）；保险起见显式兜底空 slice 让 service 层调用方不需要 nil-check
	// （§8.2 行 1341：用户无实例返 {groups:[]} code=0 不报错）。
	if rows == nil {
		rows = []UserCosmeticItem{}
	}
	return rows, nil
}

// CreateInTx 实装：INSERT 一行 user_cosmetic_items；GORM 自动回填 item.ID。
// 与 chest_open_log_repo.go Create（行 86-89）1:1 同模式：tx.FromContext 拿
// db handle（事务内调用走 txCtx 注入的 tx 句柄 → 与开箱事务同事务原子提交；
// 事务外调用走 r.db 直连 → dev /dev/grant-cosmetic-batch 批量发放路径）→
// WithContext → Create → GORM 回填 item.ID（AUTO_INCREMENT）。
//
// 详见 UserCosmeticItemRepo.CreateInTx 接口注释（Story 23.5 §AC1 钦定）。
func (r *userCosmeticItemRepo) CreateInTx(ctx context.Context, item *UserCosmeticItem) error {
	db := tx.FromContext(ctx, r.db)
	return db.WithContext(ctx).Create(item).Error
}

// FindByIDForEquip 实装：单 SELECT 显式 4 列 WHERE id=? LIMIT 1（**无**
// user_id 过滤 —— V1 §8.3 行 1492 + fix-review 26-1 r1 [P2] 强制）。
// NotFound → ErrUserCosmeticItemNotFound 哨兵（service 翻译 5001；与
// pet_repo.FindDefaultByUserID gorm.ErrRecordNotFound → 哨兵同模式）。
// 详见 UserCosmeticItemRepo.FindByIDForEquip 接口注释。
func (r *userCosmeticItemRepo) FindByIDForEquip(ctx context.Context, id uint64) (*UserCosmeticItem, error) {
	db := tx.FromContext(ctx, r.db)
	var item UserCosmeticItem
	err := db.WithContext(ctx).
		Select("id, cosmetic_item_id, status, user_id").
		Where("id = ?", id).
		First(&item).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrUserCosmeticItemNotFound
		}
		return nil, err
	}
	return &item, nil
}

// UpdateStatusInTx 实装：Update("status", v) 单字段更新（参考
// pet_repo.UpdateCurrentStateByID 模式 —— status int8 不存在 nil-skip 陷阱，
// **不**需要 Updates(map) 路径；**不**用 Save 避免写全字段）。GORM autoUpdateTime
// tag 自动写 updated_at。err 二分透传，不读 RowsAffected。详见
// UserCosmeticItemRepo.UpdateStatusInTx 接口注释。
func (r *userCosmeticItemRepo) UpdateStatusInTx(ctx context.Context, id uint64, status int8) error {
	db := tx.FromContext(ctx, r.db)
	return db.WithContext(ctx).
		Model(&UserCosmeticItem{}).
		Where("id = ?", id).
		Update("status", status).Error
}
