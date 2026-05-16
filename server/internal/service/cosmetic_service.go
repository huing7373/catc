package service

import (
	"context"
	"log/slog"
	"sort"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// CosmeticService 是 cosmetics handler 的依赖 interface（便于 handler 单测 mock）。
//
// **接口而非具体类型**：handler 单测注入 stub struct，与 emoji_service /
// home_service / room_service 同模式。
//
// Story 23.3 落地 ListCatalog（GET /api/v1/cosmetics/catalog）；
// Story 23.4 扩展加 ListInventory（GET /api/v1/cosmetics/inventory —— 聚合 +
// 实例列表 + config 三态矩阵 + 两级全序排序）。
type CosmeticService interface {
	// ListCatalog 返回所有 is_enabled=1 的 cosmetic_items 配置（V1 §8.1 服务端
	// 逻辑钦定）。
	//
	// 流程（与 emoji_service.ListAvailable 1:1 同模式）：
	//  1. cosmeticItemRepo.ListEnabledForCatalog(ctx) → []mysql.CosmeticItem
	//     （仅 is_enabled=1 已被 repo 层 SQL 过滤 + 按 rarity ASC, slot ASC,
	//     id ASC 三级全序排序）
	//  2. DTO 转换：mysql.CosmeticItem → CosmeticBrief（裁掉 drop_weight /
	//     is_enabled / created_at / updated_at；§8.1 钦定 client 不需要这些字段）
	//  3. 0 行 → []CosmeticBrief{}（**永远**非 nil；让 handler / wire 层下发
	//     `items: []` 而非 `null`，与 §8.1 行 1301 "catalog 为空返 {items:[]}
	//     code=0 不报错" 一致）
	//
	// 错误约定：
	//   - cosmeticItemRepo.ListEnabledForCatalog 失败（含 DB 异常 / 连接断 /
	//     慢查询超时等）→ apperror.Wrap 包成 1009 ErrServiceBusy（与
	//     emoji_service ListAvailable 同模式 + lesson 2026-05-13 Lesson 2 钦定
	//     DB error 必须有 1009 路径）。
	//   - **不**触发 1002（§8.1 行 1301：GET 无 body / 无 query 参数，无可校验
	//     输入）；auth(1001) / rate_limit(1005) 由 router authedGroup 中间件兜底，
	//     service 层不感知。
	//
	// **不**做空字符串过滤：§8.1 行 1267-1268 钦定 enabled cosmetic 的 iconUrl /
	// assetUrl 必非空字符串（Story 20.3 0012 seed 已保证 + admin 写入层负责
	// 校验）；本方法**不**做 `if IconURL == "" 跳过` 分支 —— 让意外有空 URL 的
	// enabled 行透传到 client 触发渲染失败而不是被 server 静默过滤（与"server 是
	// cosmetic 数据 single source of truth"语义一致，与 emoji_service.go 行
	// 38-42 钦定同源）。
	//
	// **范围红线**：本 story（23.3）仅查 cosmetic_items 配置表（§5.8），**不**读
	// userID / **不**查 23.2 落地的 user_cosmetic_items 实例表（§5.9）/ **不**做
	// 任何 user 维度聚合（catalog = 全局静态目录，与 user 无关；inventory 是
	// Story 23.4 钦定范围）。
	ListCatalog(ctx context.Context) ([]CosmeticBrief, error)

	// ListInventory 返回某用户已拥有装扮的聚合 + 实例列表（V1 §8.2 服务端逻辑
	// 步骤 2-6 钦定）。
	//
	// 流程（严格按 §8.2 服务端逻辑步骤 2-6）：
	//  1. userCosmeticItemRepo.ListByUserForInventory(ctx, userID) → 用户
	//     status IN (1,2) 实例（consumed/invalid 已被 repo SQL 过滤，§8.2 行 1340）。
	//  2. len(instances)==0 → 返 []InventoryGroup{}（非 nil）；§8.2 行 1341
	//     空背包 {groups:[]} code=0 不报错。
	//  3. 按 CosmeticItemID 聚合（去重收集 cosmeticItemIDs）。
	//  4. cosmeticItemRepo.ListByIDsForInventory(ctx, cosmeticItemIDs) → 建
	//     config map（id → CosmeticItem；**无** is_enabled=1 过滤，§8.2 行 1437）。
	//  5. **config 三态完整矩阵 A/B/C**（互斥穷尽，**禁止**只处理两态，详见
	//     ListInventory impl 注释 + §8.2 行 1342-1353）：
	//       态 A enabled（map 命中 + IsEnabled==1）→ row 真实值，无 log
	//       态 B disabled-but-exists（map 命中 + IsEnabled==0）→ row 真实值
	//         （与态 A 一致，**非** placeholder），无 log
	//       态 C missing-no-row（map **不**命中）→ 降级占位（Name="未知装扮"/
	//         Slot=99/Rarity=1/空 URL）+ slog.ErrorContext log；**不** skip 该组。
	//  6. **两级确定性全序排序**（§8.2 行 1355-1358 + 1443 契约必需）：
	//       groups[] sort.Slice by (Rarity ASC, Slot ASC, CosmeticItemID ASC)；
	//       每组 Instances[] sort.Slice by UserCosmeticItemID ASC。
	//     Count = len(Instances)（含 status=1 与 status=2，§8.2 行 1374）。
	//
	// 错误约定：
	//   - userCosmeticItemRepo.ListByUserForInventory 失败 → apperror.Wrap 包成
	//     1009 ErrServiceBusy（与 ListCatalog 同模式 + lesson 2026-05-13 Lesson 2）。
	//   - cosmeticItemRepo.ListByIDsForInventory 失败 → 同样包成 1009
	//     （config 关联失败也走 1009 路径）。
	//   - **不**触发 1002（GET 无 body / 无 query）；auth(1001) / rate_limit(1005)
	//     由 router authedGroup 中间件兜底，service 不感知。
	//   - **单组配置缺失（态 C）/ 禁用（态 B）不报错**（§8.2 行 1432：只读
	//     inventory 不因单组脏配置整体失败 —— 态 B/C 仍保留组）。
	//
	// **关键纠偏（覆盖 epics.md §Story 23.4 AC 文字陷阱）**：epics.md 行 3292
	// 写"配置不存在 → skip + log warning"，但 V1 §8.2 冻结契约（行 1342-1353
	// + 1437）已收紧为三态矩阵：态 C **不 skip**（降级占位**保留**组，因"已拥有
	// 实例不得静默丢失"是用户可见数据丢失回归），**且 log error 不是 warning**
	// （物理删仍有用户持有的 cosmetic_items 行是需运维介入的数据治理事件）。
	// 本方法按 V1 §8.2 三态矩阵实装，**不**按 epics.md "skip + warning"
	// （冲突点以契约文档为准 —— CLAUDE.md "状态以 server 为准" + 23.3 r1
	// 同源原则；详见 story Completion Notes 跨文档自检）。
	//
	// **不**做空 URL 过滤（态 A/B 透传真实非空；态 C 降级占位空串是契约**合法**
	// 路径 —— §8.2 行 1352，**不**是要过滤的脏数据）。
	//
	// **范围红线**：本 story（23.4）仅做只读 inventory 查询，**不**写
	// user_cosmetic_items（开箱补入仓 INSERT 是 Story 23.5 钦定范围）。
	ListInventory(ctx context.Context, userID uint64) ([]InventoryGroup, error)
}

// CosmeticBrief 是 V1 §8.1 data.items[] 的 service 层映射（**不是** wire DTO，
// handler 转换为 §8.1 钦定 wire 格式：cosmeticItemId 字符串化 + 字段名 camelCase）。
//
// 字段（与 §8.1 data.items[] 钦定 7 字段集 1:1 对齐，行 1260-1268）：
//   - CosmeticItemID: uint64（§8.1 `cosmeticItemId`；handler 层 strconv.FormatUint
//     字符串化 → 与 §2.5 BIGINT 字符串化全局约定 + cosmetic_items.id BIGINT
//     UNSIGNED 一致）
//   - Code:           string（§8.1 `code`；全局唯一业务编码）
//   - Name:           string（§8.1 `name`；装扮中文名 UI 展示文字）
//   - Slot:           int8（§8.1 `slot`，§6.8 枚举 {1,2,3,4,5,6,7,99}；handler
//     层 int 直接下发，**不**字符串化）
//   - Rarity:         int8（§8.1 `rarity`，§6.9 枚举 {1,2,3,4}；handler 层 int
//     直接下发，**不**字符串化）
//   - IconURL:        string（§8.1 `iconUrl`；小尺寸预览图 URL 非空字符串）
//   - AssetURL:       string（§8.1 `assetUrl`；装扮资源 URL 非空字符串）
//
// **不**含 DropWeight / IsEnabled / CreatedAt / UpdatedAt：§8.1 钦定 client 不
// 需要这些字段（与 emoji_service EmojiBrief 裁字段同模式）。
type CosmeticBrief struct {
	CosmeticItemID uint64
	Code           string
	Name           string
	Slot           int8
	Rarity         int8
	IconURL        string
	AssetURL       string
}

// InventoryGroup 是 V1 §8.2 data.groups[] 的 service 层映射（**不是** wire DTO，
// handler 转换为 §8.2 钦定 wire 格式：cosmeticItemId / userCosmeticItemId
// 字符串化 + 字段名 camelCase）。
//
// 字段（与 §8.2 data.groups[] 钦定字段集 1:1 对齐，行 1366-1377）：
//   - CosmeticItemID: uint64（§8.2 `cosmeticItemId`；handler 层 strconv.FormatUint
//     字符串化 → BIGINT 字符串化，与 §2.5 全局约定 + cosmetic_items.id 一致；
//     聚合 key）
//   - Name:           string（§8.2 `name`；态 A/B row 真实值，态 C "未知装扮"）
//   - Slot:           int8（§8.2 `slot`，§6.8 枚举 {1..7,99}；handler int 下发
//     不字符串化；态 C 占位 99=other）
//   - Rarity:         int8（§8.2 `rarity`，§6.9 枚举 {1..4}；handler int 下发
//     不字符串化；态 C 占位 1=common）
//   - IconURL:        string（§8.2 `iconUrl`；态 A/B 非空真实值，态 C 空串 ——
//     §8.2 行 1352 唯一合法空串路径）
//   - AssetURL:       string（§8.2 `assetUrl`；同 IconURL）
//   - Count:          int（§8.2 `count` = len(Instances)，含 in_bag + equipped，
//     **不**只算 in_bag；§8.2 行 1374）
//   - Instances:      []InventoryInstance（§8.2 `instances`；按 UserCosmeticItemID
//     ASC 全序）
type InventoryGroup struct {
	CosmeticItemID uint64
	Name           string
	Slot           int8
	Rarity         int8
	IconURL        string
	AssetURL       string
	Count          int
	Instances      []InventoryInstance
}

// InventoryInstance 是 V1 §8.2 data.groups[].instances[] 的 service 层映射。
//
// 字段（与 §8.2 instances[] 钦定字段集 1:1 对齐，行 1376-1377）：
//   - UserCosmeticItemID: uint64（§8.2 `userCosmeticItemId`；handler 层
//     strconv.FormatUint 字符串化 → user_cosmetic_items.id BIGINT 字符串化；
//     玩家**实例** id，**不是**配置 id）
//   - Status:             int8（§8.2 `status`，枚举 {1=in_bag,2=equipped}；
//     consumed(3)/invalid(4) 已被 repo SQL `status IN (1,2)` 过滤不会出现；
//     handler int 下发不字符串化）
type InventoryInstance struct {
	UserCosmeticItemID uint64
	Status             int8
}

// 态 C missing-no-row 降级占位元信息（**仅**态 C 使用，§8.2 行 1352 钦定值）。
// 这些占位值均落在既有枚举值域内（Slot=99=other §6.8 / Rarity=1=common §6.9），
// **不**扩展任何 schema；client 据此渲染"配置已下架的已拥有道具"占位卡。
// 空串 IconURL/AssetURL 是 §8.2 行 1352 钦定的**唯一合法空串路径**（态 A/B 非空）。
const (
	missingItemPlaceholderName    = "未知装扮"
	missingItemPlaceholderSlot    = int8(99) // §6.8 other（枚举内）
	missingItemPlaceholderRarity  = int8(1)  // §6.9 common（枚举内）
	missingItemPlaceholderIconURL = ""       // 态 C 唯一合法空串（§8.2 行 1352）
	missingItemPlaceholderAsset   = ""       // 态 C 唯一合法空串（§8.2 行 1352）
)

// cosmeticServiceImpl 是 CosmeticService 的默认实装。
type cosmeticServiceImpl struct {
	cosmeticItemRepo mysql.CosmeticItemRepo
	// Story 23.4 加：inventory 实例数据源（ListByUserForInventory）。
	userCosmeticItemRepo mysql.UserCosmeticItemRepo
}

// NewCosmeticService 构造 CosmeticService。Story 23.3 引入；
// Story 23.4 **扩签名**注入 userCosmeticItemRepo（GET /cosmetics/inventory
// 实例数据源）。
//
// 注入：
//   - cosmeticItemRepo（Story 20.6 既有 interface，23.3 扩 ListEnabledForCatalog，
//     23.4 扩 ListByIDsForInventory）
//   - userCosmeticItemRepo（Story 23.4 首次落地的 UserCosmeticItemRepo interface）
//
// router wire 复用 line 486 既有 cosmeticItemRepo 实例（**不**新建第二个，与
// chestSvc 复用同实例同模式）+ 新建 userCosmeticItemRepo 实例。
//
// **回归点（Story 23.4 关键纠偏点 4）**：本构造从 1 参扩到 2 参，现有调用方
// = router.go cosmeticSvc 构造 + cosmetic_service_test.go 全部 case +
// cosmetic_service_integration_test.go buildCosmeticServiceIntegration，扩签名后
// 全部已同步改 2 参（否则 build 红）。
func NewCosmeticService(cosmeticItemRepo mysql.CosmeticItemRepo, userCosmeticItemRepo mysql.UserCosmeticItemRepo) CosmeticService {
	return &cosmeticServiceImpl{
		cosmeticItemRepo:     cosmeticItemRepo,
		userCosmeticItemRepo: userCosmeticItemRepo,
	}
}

// ListCatalog 实装：单 repo query + DTO 转换 + nil slice 兜底。
// 详见 CosmeticService.ListCatalog 接口注释（§8.1 服务端逻辑钦定）。
func (s *cosmeticServiceImpl) ListCatalog(ctx context.Context) ([]CosmeticBrief, error) {
	rows, err := s.cosmeticItemRepo.ListEnabledForCatalog(ctx)
	if err != nil {
		// V1 §8.1 错误码表行 1299：DB 异常 → 1009 ErrServiceBusy
		// （lesson 2026-05-13 Lesson 2 钦定 DB error 必须有 1009 路径）
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 永远返非 nil slice（即便 rows 是空）—— 让 handler / wire 层下发 `items: []`
	// 而非 `null`（V1 §8.1 行 1301：catalog 为空返 {items:[]} code=0 不报错）。
	// **不**做空 URL 过滤（§8.1 行 1267-1268 钦定 enabled 必非空，0012 seed 已
	// 保证；server 透传真实 row，与 emoji_service 行 38-42 同源钦定）。
	briefs := make([]CosmeticBrief, 0, len(rows))
	for _, r := range rows {
		briefs = append(briefs, CosmeticBrief{
			CosmeticItemID: r.ID,
			Code:           r.Code,
			Name:           r.Name,
			Slot:           r.Slot,
			Rarity:         r.Rarity,
			IconURL:        r.IconURL,
			AssetURL:       r.AssetURL,
		})
	}
	return briefs, nil
}

// ListInventory 实装：查实例 → 空返 [] → 聚合 → 关联配置 → config 三态完整
// 矩阵 A/B/C → 组装 → 两级确定性全序排序。详见 CosmeticService.ListInventory
// 接口注释（§8.2 服务端逻辑步骤 2-6 钦定 + 三态矩阵 + 两级排序 + epics.md
// 冲突纠偏）。
func (s *cosmeticServiceImpl) ListInventory(ctx context.Context, userID uint64) ([]InventoryGroup, error) {
	// 步骤 2：查用户 status IN (1,2) 实例（consumed/invalid 已被 repo SQL
	// `WHERE status IN (1,2)` 过滤，§8.2 行 1340）。
	instances, err := s.userCosmeticItemRepo.ListByUserForInventory(ctx, userID)
	if err != nil {
		// §8.2 错误码表行 1430：DB 异常 → 1009 ErrServiceBusy
		// （lesson 2026-05-13 Lesson 2 钦定 DB error 必须有 1009 路径）。
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 步骤 2 续：空背包 → 返非 nil 空 slice（§8.2 行 1341：用户无实例返
	// {groups:[]} code=0 不报错；让 handler 下发 groups:[] 非 null）。
	if len(instances) == 0 {
		return []InventoryGroup{}, nil
	}

	// 步骤 3：按 cosmetic_item_id 聚合。grouped[cid] = 该配置下所有实例。
	// orderedCIDs 记录首次出现顺序仅为收集去重 ids（最终顺序由步骤 6 sort.Slice
	// 决定，**不**依赖此处的 map 遍历 / 首现顺序）。
	grouped := make(map[uint64][]InventoryInstance)
	for _, ins := range instances {
		grouped[ins.CosmeticItemID] = append(grouped[ins.CosmeticItemID], InventoryInstance{
			UserCosmeticItemID: ins.ID,
			Status:             ins.Status,
		})
	}
	cosmeticItemIDs := make([]uint64, 0, len(grouped))
	for cid := range grouped {
		cosmeticItemIDs = append(cosmeticItemIDs, cid)
	}

	// 步骤 4：关联 cosmetic_items 配置元信息（**无** is_enabled=1 过滤，
	// §8.2 行 1437 关键约束；ListByIDsForInventory 内部对空 ids 早返，但此处
	// len(grouped)>=1 必有 ids）。建 config map：id → CosmeticItem。
	configs, err := s.cosmeticItemRepo.ListByIDsForInventory(ctx, cosmeticItemIDs)
	if err != nil {
		// config 关联失败也走 1009（§8.2 行 1430：DB 异常 → 1009）。
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	configByID := make(map[uint64]mysql.CosmeticItem, len(configs))
	for _, c := range configs {
		configByID[c.ID] = c
	}

	// 步骤 5：config 三态完整矩阵 A/B/C（互斥穷尽，**禁止**只处理两态）。
	// 步骤 6（Count）：Count = len(Instances)（含 status=1 与 status=2，
	// §8.2 行 1374，**不**只算 in_bag）。
	groups := make([]InventoryGroup, 0, len(grouped))
	for cid, insts := range grouped {
		g := InventoryGroup{
			CosmeticItemID: cid,
			Count:          len(insts),
			Instances:      insts,
		}
		cfg, hit := configByID[cid]
		switch {
		case !hit:
			// 态 C missing-no-row：config map **不**命中（admin 物理删了 row，
			// 但用户已拥有实例仍在 user_cosmetic_items）。降级占位**保留**该组
			// （**不** skip —— "已拥有不得静默丢失"是用户可见数据丢失回归，
			// §8.2 行 1437 + 行 1432）+ log error（数据治理事件，需运维介入；
			// **不是** epics.md 行 3292 的 warning —— 详见接口注释 epics.md
			// 冲突纠偏）。
			g.Name = missingItemPlaceholderName
			g.Slot = missingItemPlaceholderSlot
			g.Rarity = missingItemPlaceholderRarity
			g.IconURL = missingItemPlaceholderIconURL
			g.AssetURL = missingItemPlaceholderAsset
			slog.ErrorContext(ctx,
				"inventory: cosmetic_items row missing for owned user_cosmetic_items (data governance event: admin physically deleted a cosmetic config still referenced by user-owned instances; ops should restore config / add down-migration / run deprecation flow)",
				"cosmetic_item_id", cid,
				"user_id", userID,
				"instance_count", len(insts),
			)
		case cfg.IsEnabled == 0:
			// 态 B disabled-but-exists：config map 命中但 is_enabled=0。
			// 用 **row 真实值**（与态 A 完全一致，含真实非空 URL，**非**
			// placeholder —— §8.2 行 1347/1351：inventory 是"已拥有清单"语义，
			// 数据可得，admin 下架不影响已拥有展示）；admin 下架是常规运维
			// 动作，**不** log error（§8.2 行 1347/1353）。
			g.Name = cfg.Name
			g.Slot = cfg.Slot
			g.Rarity = cfg.Rarity
			g.IconURL = cfg.IconURL
			g.AssetURL = cfg.AssetURL
		default:
			// 态 A enabled：config map 命中且 is_enabled=1。row 真实值，无 log。
			g.Name = cfg.Name
			g.Slot = cfg.Slot
			g.Rarity = cfg.Rarity
			g.IconURL = cfg.IconURL
			g.AssetURL = cfg.AssetURL
		}
		groups = append(groups, g)
	}

	// 步骤 6：两级确定性全序排序（§8.2 行 1355-1358 + 1443 契约必需，非可选
	// 优化；**不**依赖 SQL / Go map 迭代顺序 —— Go map range 无序，必须 service
	// 层显式 sort.Slice）。
	//
	// groups[]：(Rarity ASC, Slot ASC, CosmeticItemID ASC) —— 与 §8.1 catalog
	// rarity ASC, slot ASC, id ASC 同根因风格对齐；CosmeticItemID ASC 是决定性
	// tie-breaker（同 (rarity, slot) 必有多配置）。态 C 组用降级占位
	// Rarity=1/Slot=99 参与排序（占位值落枚举内，排序仍全序确定）。
	sort.Slice(groups, func(i, j int) bool {
		if groups[i].Rarity != groups[j].Rarity {
			return groups[i].Rarity < groups[j].Rarity
		}
		if groups[i].Slot != groups[j].Slot {
			return groups[i].Slot < groups[j].Slot
		}
		return groups[i].CosmeticItemID < groups[j].CosmeticItemID
	})
	// 每组 instances[]：UserCosmeticItemID ASC（user_cosmetic_items.id §5.9
	// 全局唯一主键，单值即决定性全序 tie-breaker；Status **不**参与排序）。
	for gi := range groups {
		insts := groups[gi].Instances
		sort.Slice(insts, func(i, j int) bool {
			return insts[i].UserCosmeticItemID < insts[j].UserCosmeticItemID
		})
	}

	return groups, nil
}
