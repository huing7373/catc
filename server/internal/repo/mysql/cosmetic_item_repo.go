package mysql

import (
	"context"
	"time"

	"gorm.io/gorm"

	"github.com/huing/cat/server/internal/repo/tx"
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

// CosmeticItemRepo 是 cosmetic_items 表的访问接口（Story 20.6 引入；
// Story 23.3 扩展加 ListEnabledForCatalog —— GET /cosmetics/catalog 配置目录查询）。
//
// **两方法语义独立、各自演进，不可互相复用**：
//   - ListEnabledForWeightedPick（Story 20.6）：开箱事务步骤 5g 加权抽取专用，
//     **无 ORDER BY**（service 内加权采样不需要排序），SELECT *。
//   - ListEnabledForCatalog（Story 23.3）：catalog 配置目录查询专用，**必须**
//     `ORDER BY rarity ASC, slot ASC, id ASC` 三级全序（§8.1 契约 + client grid
//     防抖动），显式 7 列 Select。
//
// 故意**不**复用 ListEnabledForWeightedPick 做 catalog：复用会让 catalog 排序契约
// 与开箱抽奖路径耦合（任一方改字段 / 改排序就破坏另一方；下游评审找不到 catalog
// 专用查询的明确边界）。两方法语义独立、并列存在。
//
// **范围红线**：本 interface 仅含已落地业务路径所需方法；**不**提前实装
// Create / Update / Delete / FindByID / 23.4 inventory 聚合查询等方法（YAGNI；
// inventory 由 Epic 23 Story 23.4 落地 GET /cosmetics/inventory 时再补）。
type CosmeticItemRepo interface {
	// ListEnabledForWeightedPick 返回所有 is_enabled=1 的 cosmetic_items 行
	// （含 id / rarity / drop_weight / name / slot / asset_url / icon_url；
	// V1 §7.2.5g 钦定的字段集）。
	//
	// 调用方语义：开箱事务步骤 5g 加权抽取一次性拉全表 enabled 集合
	// （节点 7 阶段 enabled 集合约 15-20 行，单次扫表 + service 内加权采样足够；
	// N+1 单查每件反而劣化）。事务内 / 事务外都可调，由 ctx 决定走 tx 还是 db。
	//
	// 返回空切片 → service 层翻译为 1009 "seed 未执行"数据完整性异常（V1 §7.2.5g 钦定）。
	//
	// query 失败 → 返 raw error 透传（service 包成 1009）。
	ListEnabledForWeightedPick(ctx context.Context) ([]CosmeticItem, error)

	// ListEnabledForCatalog 返回所有 is_enabled=1 的 cosmetic_items 行，按
	// V1 §8.1 服务端逻辑步骤 2 钦定排序（GET /cosmetics/catalog 配置目录查询）。
	//
	// SQL: SELECT id, code, name, slot, rarity, icon_url, asset_url FROM cosmetic_items
	//      WHERE is_enabled = 1 ORDER BY rarity ASC, slot ASC, id ASC
	//
	// **排序契约（§8.1 行 1306 + 23.1 r1 [P2] 钦定全序确定）**：
	//   - rarity ASC → slot ASC → **id ASC**（决定性 tie-breaker，不可省）。
	//   - id ASC 不可省理由：§1 catalog seed AR18 数量约束下同 (rarity, slot) 必有
	//     多行（如 hat_yellow/hat_red 同为 (rarity=1, slot=1)、gloves_white/
	//     gloves_brown 同为 (rarity=1, slot=2)），缺 id ASC 则 MySQL 同 (rarity,
	//     slot) 行顺序跨请求可抖动 → client grid 抖动违背契约。
	//
	// 显式 Select 7 列（id / code / name / slot / rarity / icon_url / asset_url）
	// 与 §8.1 服务端逻辑步骤 2 钦定 1:1；**不** SELECT *（避免 future 表加列污染
	// payload；drop_weight / is_enabled / created_at / updated_at 不在 SELECT —
	// client 不需要，GORM Scan 填 zero-value 安全，与 emoji_repo.List 同模式）。
	//
	// 空结果集 → 返回 []CosmeticItem{}（非 nil）；service 层透传为 {items:[]}
	// 非 error（§8.1 行 1301：catalog 为空 code=0 不报错）。query 失败 → 返 raw
	// error 透传（service 包成 1009）。
	//
	// **范围红线**：本 story（23.3）仅加 catalog 方法；**不**实装 23.4 inventory
	// 聚合查询方法（YAGNI；inventory 是 Story 23.4 钦定范围）。
	ListEnabledForCatalog(ctx context.Context) ([]CosmeticItem, error)
}

// cosmeticItemRepo 是 CosmeticItemRepo 的默认实装。
type cosmeticItemRepo struct {
	db *gorm.DB
}

// NewCosmeticItemRepo 构造 CosmeticItemRepo。Story 20.6 引入。
func NewCosmeticItemRepo(db *gorm.DB) CosmeticItemRepo {
	return &cosmeticItemRepo{db: db}
}

// ListEnabledForWeightedPick 实装：SELECT 所有 is_enabled=1 行。
//
// GORM Find 自动把空结果集映射为空切片（非 nil）；service 层用 len(items)==0 判断
// 而非 nil 判断。
func (r *cosmeticItemRepo) ListEnabledForWeightedPick(ctx context.Context) ([]CosmeticItem, error) {
	db := tx.FromContext(ctx, r.db)
	var items []CosmeticItem
	err := db.WithContext(ctx).
		Where("is_enabled = ?", 1).
		Find(&items).Error
	if err != nil {
		return nil, err
	}
	return items, nil
}

// ListEnabledForCatalog 实装：单 SELECT，显式 7 列 + WHERE is_enabled=1 +
// ORDER BY rarity ASC, slot ASC, id ASC 三级全序。详见 CosmeticItemRepo.ListEnabledForCatalog
// 接口注释（§8.1 服务端逻辑步骤 2 钦定）。
//
// **不**复用 ListEnabledForWeightedPick（后者无 ORDER BY、SELECT *、加权抽取语义）。
func (r *cosmeticItemRepo) ListEnabledForCatalog(ctx context.Context) ([]CosmeticItem, error) {
	// 用 tx.FromContext 取 db handle：事务外调用走 r.db；事务内调用走 txCtx 注入的
	// tx 句柄（与 emoji_repo.List / 20.6 既有 repo 同模式；本 story 阶段实际不在
	// 事务内调，但保持模式一致让 future 扩展无需改 method body）。
	db := tx.FromContext(ctx, r.db).WithContext(ctx)

	var rows []CosmeticItem
	// 显式 Select 字段集（不依赖 GORM 自动 SELECT *），与 §8.1 服务端逻辑步骤 2
	// 钦定 7 列 1:1 对齐；避免 future 表加字段时被自动拉过来污染 query payload。
	// **注**：drop_weight / is_enabled / created_at / updated_at **不**在 SELECT
	// 列表中（client 不需要 + service 层不做 wire DTO 转换），但 GORM Scan 会把
	// 它们填为 zero-value；service 层 DTO 转换不读这些字段，所以 zero-value 安全
	// （与 emoji_repo.go 行 144-148 同模式）。
	err := db.
		Select("id, code, name, slot, rarity, icon_url, asset_url").
		Where("is_enabled = ?", 1).
		Order("rarity ASC, slot ASC, id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	// GORM Find 在 0 行时返回空 slice 而非 nil（与 emoji_repo.List 同模式）；
	// 保险起见显式兜底空 slice 让 service 层调用方不需要 nil-check（§8.1 行 1301：
	// catalog 为空返 {items:[]} code=0 不报错）。
	if rows == nil {
		rows = []CosmeticItem{}
	}
	return rows, nil
}
