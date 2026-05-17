package mysql

import (
	"context"
	stderrors "errors"
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
// Story 23.3 扩展加 ListEnabledForCatalog —— GET /cosmetics/catalog 配置目录查询；
// Story 23.4 扩展加 ListByIDsForInventory —— GET /cosmetics/inventory config 关联）。
//
// **三方法语义独立、各自演进，不可互相复用**：
//   - ListEnabledForWeightedPick（Story 20.6）：开箱事务步骤 5g 加权抽取专用，
//     **无 ORDER BY**（service 内加权采样不需要排序），SELECT *，WHERE is_enabled=1。
//   - ListEnabledForCatalog（Story 23.3）：catalog 配置目录查询专用，**必须**
//     `ORDER BY rarity ASC, slot ASC, id ASC` 三级全序（§8.1 契约 + client grid
//     防抖动），显式 7 列 Select，WHERE is_enabled=1。
//   - ListByIDsForInventory（Story 23.4）：inventory config 关联专用，按 id 集合
//     批量查（`WHERE id IN (?)`），**无 ORDER BY**（两级排序在 service 层
//     sort.Slice 做）、**无 is_enabled=1 过滤**（§8.2 契约：实例可见性与配置
//     enabled 状态完全解耦，态 B disabled-but-exists 必须返回真实 metadata），
//     显式 6 列 Select（**不**含 code —— §8.2 groups[] 无 code 字段，与 §8.1
//     catalog 含 code 不同）。
//
// 故意**不**互相复用：复用会让各路径排序 / 过滤契约耦合（任一方改字段 / 改排序 /
// 改过滤就破坏另一方；下游评审找不到各专用查询的明确边界）。三方法语义独立、
// 并列存在。**特别地，ListByIDsForInventory 故意不复用 ListEnabledForCatalog**
// —— 后者带 `WHERE is_enabled=1` 会让 admin 下架（is_enabled=0）配置的已拥有项
// 从 inventory config map 消失被误判为态 C（错误降级 + 错误 log error），实际应
// 是态 B（row 真实值 + 不 log），违背 §8.2 行 1437 "禁止加 is_enabled=1 过滤"
// 关键约束（用户可见数据丢失回归）。
//
// **范围红线**：本 interface 仅含已落地业务路径所需方法；**不**提前实装
// Create / Update / Delete / FindByID 等方法（YAGNI）。
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
	// **范围红线**：本 story（23.3）仅加 catalog 方法；inventory config 关联
	// 方法 ListByIDsForInventory 由 Story 23.4 落地（见下）。
	ListEnabledForCatalog(ctx context.Context) ([]CosmeticItem, error)

	// ListByIDsForInventory 按 id 集合批量查 cosmetic_items 配置元信息
	// （GET /cosmetics/inventory 服务端逻辑步骤 3 config 关联，V1 §8.2）。
	//
	// SQL: SELECT id, name, slot, rarity, icon_url, asset_url FROM cosmetic_items
	//      WHERE id IN (?)
	//
	// **禁止加 is_enabled=1 过滤**（§8.2 行 1342 / 1437 关键约束：实例可见性与
	// 配置 enabled 状态完全解耦 —— 已拥有道具不得因 admin 下架配置而静默丢失；
	// 态 B disabled-but-exists 行必须返回真实 metadata，与态 A 一致）。service 层
	// 据 config map 命中后读 CosmeticItem.IsEnabled 区分态 A/B，据 config map
	// 是否命中区分态 B/C —— 故本方法必须 SELECT is_enabled 列才能让 service
	// 区分态 A/B（见下方 impl Select 列说明）。
	//
	// 显式 Select（**不**含 code —— §8.2 groups[] 无 code 字段，与 §8.1 catalog
	// items[] 含 code 不同；**不** SELECT drop_weight / created_at / updated_at ——
	// inventory 响应不需要）；**含 is_enabled 列**供 service 区分态 A/B。
	//
	// **无 ORDER BY**：两级确定性全序排序在 service 层 sort.Slice 做
	// （§8.2 行 1360 钦定不依赖 DB 天然顺序）。
	//
	// ids 为空 → 直接返 []CosmeticItem{}（service 层在 ids 为空时不调本方法 ——
	// 空背包早已在步骤 2 返回；但本方法仍兜底空 ids → 空 slice，**不**发
	// `WHERE id IN ()` 空集 SQL）。query 失败 → 返 raw error 透传（service 包成 1009）。
	//
	// 故意**不**复用 ListEnabledForCatalog（带 is_enabled=1 会让 disabled 配置
	// 已拥有项消失 → 违背态 B 契约）/ **不**复用 ListEnabledForWeightedPick
	// （SELECT * 全表扫，inventory 只需按 id 集合查指定字段）。
	ListByIDsForInventory(ctx context.Context, ids []uint64) ([]CosmeticItem, error)

	// ListEnabledIDsByRarity 返回某 rarity 下**全部** enabled cosmetic_item_id
	// （Story 23.5 引入；fix-review 23-5 r2 [P2] 根因修复 —— 替换 r1 的
	// FindRandomByRarity(rarity, count) `ORDER BY RAND() LIMIT count`）。
	//
	// SQL: SELECT id FROM cosmetic_items WHERE rarity = ? AND is_enabled = 1
	//      （**无 LIMIT，无 ORDER BY RAND()**）
	//
	// **为何返回全池而非 LIMIT count（根因 / over-correction chain 收敛）**：
	// dev grant 的 `count` 是要授予的**实例（instance）数**，**不是** distinct
	// 配置数。`user_cosmetic_items`（DB §5.9）只有非唯一 KEY
	// idx_user_id_cosmetic_item_id（**无** UNIQUE(user_id, cosmetic_item_id)），
	// 同一 cosmetic_item_id 多实例合法且为合成 feature 核心所必需（§22 "手动
	// 选择 10 个同品质道具实例"）。原 `LIMIT count` 把 pick 上限钉死在 distinct
	// 池大小（seed common 仅 8 件 → 无法产 10 个 common 实例供合成 demo）；r1
	// 的 `len<count→1009` 拒绝是对错误契约的二次 over-correct（把合法主 demo
	// 用例打死）。根因解法：repo 只负责给出**池**（distinct enabled id 列表），
	// 由 service 在 Go 层**有放回**抽 count 个 —— 数量语义与池大小彻底解耦。
	//
	// **WHERE is_enabled = 1 过滤理由**：dev 端点发放的应是可用配置（与
	// ListEnabledForCatalog / ListEnabledForWeightedPick 一致 —— 不发已下架配置）。
	// **不复用** ListEnabledForWeightedPick（SELECT * 全表 + prod 加权采样语义）
	// / ListEnabledForCatalog（带全序 ORDER BY，catalog 语义）—— 本方法只需
	// 按 rarity 取 id 集合，语义独立。
	//
	// 返回 cosmetic_item_id slice（仅 id 列，dev 发放不需要其他元信息）；
	// 空结果（该 rarity 无任何 enabled 配置 —— 理论 Story 20.3 seed ≥15 行
	// 覆盖 4 rarity 不该发生）→ 返 []uint64{}（非 nil），service 层判 len==0
	// 翻译为 1009（seed 数据完整性异常 —— 这是**真正**的错误档；池非空但
	// < count **不是**错误，service 有放回抽满 count）。
	// query 失败 → 返 raw error 透传（service 包成 1009）。
	ListEnabledIDsByRarity(ctx context.Context, rarity int8) ([]uint64, error)

	// FindSlotNameByID 按 cosmetic_item id 查 slot + name（V1 §8.3 服务端逻辑
	// 步骤 7 查配置槽位，Story 26.3 引入）。
	//
	// SQL: SELECT slot, name FROM cosmetic_items WHERE id = ? LIMIT 1
	//
	// **返回三态（found bool 区分 missing-no-row vs DB 异常）**：
	//   - 行存在 → (slot, name, true, nil)；service 拿 slot 继续步骤 8、name
	//     用于步骤 11 response。
	//   - 行不存在（missing-no-row：admin 物理删了 cosmetic_items 行但实例仍
	//     status=1 在 user_cosmetic_items，与 §8.2 态 C 同源）→
	//     (0, "", **false**, **nil**)。**故意不返 NotFound error** —— 让
	//     service 据 found==false 走 missing-no-row → 5003 + log error 分支
	//     （V1 §8.3 行 1501-1502 + fix-review 26-1 r2 [P2] 锁定），与"DB 异常
	//     err != nil → 1009"路径明确区分（若复用 NotFound error 哨兵则 service
	//     无法区分"配置被删的合法 equip 输入"和"DB 查询失败"两种语义）。
	//   - DB 异常 → (0, "", false, rawErr)；service 包成 1009。
	//
	// 显式 Select 2 列（slot / name）—— equip 步骤 7/11 只需这 2 列。**不**复用
	// ListByIDsForInventory（带 is_enabled 语义 + 批量 IN 查 + 多字段，与单行
	// 按 id 查 slot/name 语义不符；与该 interface 行 77-83 范围红线一致）。
	//
	// **范围红线**：本 story（26.3）仅加 equip 步骤 7 查询方法；**不**预实装
	// 其他 FindByID 类方法（YAGNI）。
	FindSlotNameByID(ctx context.Context, id uint64) (slot int8, name string, found bool, err error)
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

// ListByIDsForInventory 实装：空 ids 早返 + 单 SELECT，显式列（含 is_enabled
// 供 service 区分态 A/B）+ WHERE id IN ?，**无** is_enabled=1 过滤、**无**
// ORDER BY。详见 CosmeticItemRepo.ListByIDsForInventory 接口注释
// （§8.2 服务端逻辑步骤 3 config 关联钦定）。
//
// **不**复用 ListEnabledForCatalog（带 is_enabled=1 会让 disabled 配置已拥有项
// 消失，违背 §8.2 行 1437 态 B 契约）。
func (r *cosmeticItemRepo) ListByIDsForInventory(ctx context.Context, ids []uint64) ([]CosmeticItem, error) {
	// 空 ids 早返空 slice —— 避免 GORM 生成 `WHERE id IN (NULL)` / `IN ()` 空集
	// 退化 SQL（service 层空背包已在步骤 2 早返，正常不会走到这；本方法仍兜底）。
	if len(ids) == 0 {
		return []CosmeticItem{}, nil
	}

	// 与 ListEnabledForCatalog 同模式取 db handle（事务外 r.db / 事务内 txCtx 句柄）；
	// 本 story inventory 只读不开事务，保持模式一致让 future 扩展无需改 method body。
	db := tx.FromContext(ctx, r.db).WithContext(ctx)

	var rows []CosmeticItem
	// 显式 Select：id / name / slot / rarity / icon_url / asset_url（§8.2 groups[]
	// 字段集，**不**含 code —— 与 §8.1 catalog 含 code 不同）+ **is_enabled**
	// （service 据此区分态 A enabled vs 态 B disabled-but-exists；缺这一列
	// service 无法区分两态，会把态 B 误当态 A 也没差，但语义上必须 SELECT 出来
	// 让 config map 携带真实 is_enabled）。**不** SELECT code / drop_weight /
	// created_at / updated_at —— inventory 响应不需要，GORM Scan 填 zero-value
	// 安全（与 ListEnabledForCatalog 行 158-161 同模式）。
	//
	// **无 WHERE is_enabled=1**（§8.2 行 1437 关键约束，禁止加该过滤 —— 否则
	// disabled 配置的已拥有项被静默隐藏，违背态 B 契约）。
	// **无 ORDER BY**（两级排序在 service 层 sort.Slice 做，§8.2 行 1360）。
	err := db.
		Select("id, name, slot, rarity, icon_url, asset_url, is_enabled").
		Where("id IN ?", ids).
		Find(&rows).Error
	if err != nil {
		return nil, err
	}
	// GORM Find 0 行返空 slice 而非 nil；显式兜底让 service 层无需 nil-check
	// （与 ListEnabledForCatalog 同模式；inventory 中某些 id 在 cosmetic_items
	// 无匹配行 = 态 C，由 service 层据 config map 是否命中判定，不在 repo 层处理）。
	if rows == nil {
		rows = []CosmeticItem{}
	}
	return rows, nil
}

// ListEnabledIDsByRarity 实装：SELECT id ... WHERE rarity=? AND is_enabled=1
// （**无 LIMIT / 无 ORDER BY RAND()** —— 返回全池）。详见 CosmeticItemRepo
// .ListEnabledIDsByRarity 接口注释（fix-review 23-5 r2 [P2] 根因修复）。
//
// **不**复用 ListEnabledForWeightedPick（SELECT * 全表 + service 加权采样，prod
// 抽奖语义）/ ListEnabledForCatalog（带 ORDER BY rarity/slot/id 全序，catalog 语义）。
func (r *cosmeticItemRepo) ListEnabledIDsByRarity(ctx context.Context, rarity int8) ([]uint64, error) {
	db := tx.FromContext(ctx, r.db).WithContext(ctx)

	var ids []uint64
	err := db.
		Model(&CosmeticItem{}).
		Select("id").
		Where("rarity = ? AND is_enabled = ?", rarity, 1).
		Find(&ids).Error
	if err != nil {
		return nil, err
	}
	// GORM Find 0 行返空 slice 而非 nil；显式兜底让 service 层无需 nil-check
	// （理论 Story 20.3 seed ≥15 行覆盖 4 个 rarity 不该发生空集；service 判
	// len==0 翻译为 1009 seed 数据完整性异常 —— 池非空但 < count **不是**
	// 错误，service 有放回抽满 count）。
	if ids == nil {
		ids = []uint64{}
	}
	return ids, nil
}

// FindSlotNameByID 实装：单 SELECT 显式 2 列 WHERE id=? LIMIT 1。
// **三态返回**：行存在 → (slot, name, true, nil)；missing-no-row →
// (0, "", false, nil)（gorm.ErrRecordNotFound 不当 error 返回 —— 让 service
// 据 found==false 走 5003 + log error 分支，与 DB 异常 1009 区分）；DB 异常 →
// (0, "", false, rawErr)。详见 CosmeticItemRepo.FindSlotNameByID 接口注释
// （V1 §8.3 步骤 7 + fix-review 26-1 r2 [P2] 锁定）。
func (r *cosmeticItemRepo) FindSlotNameByID(ctx context.Context, id uint64) (int8, string, bool, error) {
	db := tx.FromContext(ctx, r.db).WithContext(ctx)

	// 只取 slot / name 两列；用轻量结构体接收（不复用 CosmeticItem 全 struct ——
	// 显式裁字段避免误读未 SELECT 列的 zero-value，与 emoji_repo 显式列同模式）。
	var row struct {
		Slot int8
		Name string
	}
	err := db.
		Model(&CosmeticItem{}).
		Select("slot, name").
		Where("id = ?", id).
		First(&row).Error
	if err != nil {
		if stderrors.Is(err, gorm.ErrRecordNotFound) {
			// missing-no-row：故意 found=false + err=nil（**非** error）——
			// service 据此走 V1 §8.3 步骤 7 missing-no-row → 5003 + log error
			// 分支（fix-review 26-1 r2 [P2] 锁定）。
			return 0, "", false, nil
		}
		return 0, "", false, err
	}
	return row.Slot, row.Name, true, nil
}
