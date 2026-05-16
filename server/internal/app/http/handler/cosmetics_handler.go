package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/middleware"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// CosmeticsHandler 是 /cosmetics/* 路由的 handler。
//
// 节点 8 阶段：
//   - GetCatalog（GET /api/v1/cosmetics/catalog，Story 23.3）
//   - GetInventory（GET /api/v1/cosmetics/inventory，Story 23.4）
//
// future epic 加 POST / PATCH /cosmetics 等 admin 端能力（MVP 节点 8 无 admin
// 后台需求，与 emojis_handler 行 10-13 同模式）。
type CosmeticsHandler struct {
	svc service.CosmeticService
}

// NewCosmeticsHandler 构造 CosmeticsHandler。
//
// 注入 CosmeticService（service 层 interface）—— handler 单测直接传 stub struct
// 实现该 interface，不需要起 *gorm.DB / 真 mysql。与 EmojisHandler /
// HomeHandler / ChestHandler 同模式。
func NewCosmeticsHandler(svc service.CosmeticService) *CosmeticsHandler {
	return &CosmeticsHandler{svc: svc}
}

// GetCatalog 处理 GET /api/v1/cosmetics/catalog。
//
// # 流程
//
//  1. 调 svc.ListCatalog(ctx) 取所有 enabled 装扮配置（已按 §8.1 三级全序排序）
//  2. 成功 → response.Success(c, catalogResponseDTO(briefs), "ok")
//  3. 失败 → c.Error(err) + return（让 ErrorMappingMiddleware 写 envelope）
//
// # 关键
//
// 本 handler **不**做参数校验：V1 §8.1 行 1241 钦定不接受任何 query 参数 /
// body 字段，也不读 userID（接口要求 auth 但 service 不需要 user 维度过滤 ——
// catalog 是全局静态目录）；auth 头由 router authedGroup 中间件链兜底（已挂
// Auth + RateLimitByUserID 中间件，对应 §8.1 错误码 1001 / 1005）。**不**触发
// 1002（§8.1 行 1301：GET 无可校验输入）。
//
// # ADR-0006 单一 envelope 生产者
//
// 本 handler **不**直接调 response.Error 写 envelope —— 一律走 c.Error +
// return，由 ErrorMappingMiddleware 兜底翻译成 envelope。详见
// docs/lessons/2026-04-24-error-envelope-single-producer.md。
//
// # ADR-0007 §2.2 ctx 传播
//
// 用 c.Request.Context() 传给 service —— **不**直接传 *gin.Context（其 Done()
// 是 nil channel，service 层 select ctx.Done() 不会响应 client 断开）。
func (h *CosmeticsHandler) GetCatalog(c *gin.Context) {
	briefs, err := h.svc.ListCatalog(c.Request.Context())
	if err != nil {
		// service 已 wrap *AppError；handler 透传，让 ErrorMappingMiddleware 写 envelope
		_ = c.Error(err)
		return
	}

	response.Success(c, catalogResponseDTO(briefs), "ok")
}

// catalogResponseDTO 把 service 输出转成 V1 §8.1 钦定的 wire 格式。
//
// # 关键转换
//
//   - **`cosmeticItemId` 必须字符串化**：§8.1 字段表行 1262 钦定 `cosmeticItemId`
//     是 string 类型（BIGINT 字符串化，与 §2.5 全局约定 + cosmetic_items.id
//     BIGINT UNSIGNED 一致）；用 strconv.FormatUint(b.CosmeticItemID, 10) ——
//     **不**直接塞 uint64（那会序列化成 JSON number 破坏契约，iOS String 解码
//     失败）。这是与 emoji handler 的关键差异（emoji 无 id 下发）。
//   - `slot` / `rarity` 是 **int** 直接下发（§8.1 行 1265-1266 钦定 int 类型，
//     **不**字符串化 —— 只有 id 类字段字符串化，枚举类 int 字段不字符串化）。
//   - 字段名全 camelCase：IconURL → "iconUrl" / AssetURL → "assetUrl"
//     （V1 §2.4 + §8.1 钦定 wire 全 camelCase；与 home_handler / emojis_handler
//     同模式）。
//   - **不**下发 DropWeight / IsEnabled / CreatedAt / UpdatedAt（§8.1 钦定 client
//     不需要）。
//   - **永远**下发 `items: []` 而非 `items: null`（§8.1 行 1301 钦定 catalog 为
//     空返 {items:[]} code=0 不报错）：用 make([]gin.H, 0, len(briefs)) 兜底空
//     slice → JSON 序列化为 `[]` 而非 null（nil slice 序列化为 null，与
//     home_handler.go pet null 同模式区分；与 emojis_handler 行 78-95 同模式）。
//
// # V1 §8.1 钦定的 wire 字段集（任一缺失 → iOS DTO 解码失败，行 1260-1268）
//
//   - data.items[].cosmeticItemId: string（BIGINT 字符串化）
//   - data.items[].code:           string
//   - data.items[].name:           string
//   - data.items[].slot:           number (int，§6.8 枚举)
//   - data.items[].rarity:         number (int，§6.9 枚举)
//   - data.items[].iconUrl:        string（非空）
//   - data.items[].assetUrl:       string（非空）
func catalogResponseDTO(briefs []service.CosmeticBrief) gin.H {
	// 永远返非 nil items slice（即便 briefs 是空）—— V1 §8.1 行 1301 钦定。
	// service 层已经保证 briefs 非 nil（用 make([]CosmeticBrief, 0, len(rows))
	// 兜底），这里再保险一次避免任何 edge case 触发 JSON null（如 future 改动
	// service 实装）。
	items := make([]gin.H, 0, len(briefs))
	for _, b := range briefs {
		items = append(items, gin.H{
			// cosmeticItemId 必须 string（§8.1 行 1262 BIGINT 字符串化）
			"cosmeticItemId": strconv.FormatUint(b.CosmeticItemID, 10),
			"code":           b.Code,
			"name":           b.Name,
			// slot / rarity 是 int 直接下发（§8.1 行 1265-1266，**不**字符串化）
			"slot":     b.Slot,
			"rarity":   b.Rarity,
			"iconUrl":  b.IconURL,
			"assetUrl": b.AssetURL,
		})
	}

	return gin.H{
		"items": items,
	}
}

// GetInventory 处理 GET /api/v1/cosmetics/inventory（Story 23.4）。
//
// # 流程
//
//  1. 从 c.Get(UserIDKey) 拿 userID（auth 中间件已注入；不存在 / 类型断言失败
//     → 1009 unreachable 兜底）—— 与 chest_handler.GetCurrent（行 59-81）1:1 同模式
//  2. 调 svc.ListInventory(ctx, userID) —— ctx = c.Request.Context()
//     （**不**用 *gin.Context；ADR-0007 §2.2）
//  3. 成功 → response.Success(c, inventoryResponseDTO(groups), "ok")
//  4. 失败 → c.Error(err) + return（让 ErrorMappingMiddleware 写 envelope）
//
// # 关键差异 vs GetCatalog
//
// GetCatalog **不**读 userID（catalog 是全局静态目录）；GetInventory **必须**
// 读 userID（user 维度查询）—— 故首次需 import middleware（既有文件 GetCatalog
// 不读 userID 未 import）。本 handler **不**做参数校验（GET 无 body / 无 query；
// V1 §8.2 行 1330 钦定不接受任何 query string；userID 由 auth 中间件兜底）；
// auth 头由 router authedGroup 中间件链兜底（已挂 Auth + RateLimitByUserID，
// 对应 §8.2 错误码 1001 / 1005）。**不**触发 1002（§8.2 行 1432：GET 无可校验输入）。
//
// # ADR-0006 单一 envelope 生产者
//
// 本 handler **不**直接调 response.Error 写 envelope —— 一律走 c.Error +
// return，由 ErrorMappingMiddleware 兜底翻译成 envelope。详见
// docs/lessons/2026-04-24-error-envelope-single-producer.md。
//
// # ADR-0007 §2.2 ctx 传播
//
// 用 c.Request.Context() 传给 service —— **不**直接传 *gin.Context（其 Done()
// 是 nil channel，service 层 select ctx.Done() 不会响应 client 断开）。
func (h *CosmeticsHandler) GetInventory(c *gin.Context) {
	// 从 auth 中间件取 userID（与 chest_handler.GetCurrent 行 59-72 1:1 同模式）
	v, ok := c.Get(middleware.UserIDKey)
	if !ok {
		// unreachable: Auth 中间件挂在前；保险兜底走 1009
		_ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
		return
	}
	userID, ok := v.(uint64)
	if !ok {
		// unreachable: Auth 中间件 c.Set(UserIDKey, claims.UserID) 永远是 uint64
		_ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
		return
	}

	groups, err := h.svc.ListInventory(c.Request.Context(), userID)
	if err != nil {
		// service 已 wrap *AppError；handler 透传，让 ErrorMappingMiddleware 写 envelope
		_ = c.Error(err)
		return
	}

	response.Success(c, inventoryResponseDTO(groups), "ok")
}

// inventoryResponseDTO 把 service 输出转成 V1 §8.2 钦定的 wire 格式。
//
// # 关键转换
//
//   - **`cosmeticItemId` / `userCosmeticItemId` 必须字符串化**：§8.2 字段表
//     行 1368 / 1376 钦定均为 string 类型（BIGINT 字符串化，与 §2.5 全局约定 +
//     cosmetic_items.id / user_cosmetic_items.id BIGINT UNSIGNED 一致）；用
//     strconv.FormatUint —— **不**直接塞 uint64（那会序列化成 JSON number
//     破坏契约，iOS String 解码失败）。
//   - `slot` / `rarity` / `status` / `count` 是 **int** 直接下发（§8.2 行
//     1370-1377 钦定 int 类型，**不**字符串化 —— 只有 id 类字段字符串化，
//     枚举 / 计数类 int 字段不字符串化）。
//   - 字段名全 camelCase：IconURL → "iconUrl" / AssetURL → "assetUrl" /
//     UserCosmeticItemID → "userCosmeticItemId"（V1 §2.4 + §8.2 钦定 wire
//     全 camelCase；与 catalogResponseDTO / home_handler 同模式）。
//   - **不**下发 `code` 字段（§8.2 groups[] **无** code 字段，与 §8.1 catalog
//     items[] 含 code 不同 —— 这是 inventory 与 catalog 的关键 wire 差异）。
//   - **永远**下发 `groups: []` 非 null + 每组 `instances: []` 非 null
//     （§8.2 行 1440 防 Swift Codable 解析 nil；service 层已保证 groups 非 nil
//     用 make([]InventoryGroup, 0, len) 兜底，这里再保险一次 make([]gin.H,
//     0, len) → JSON 序列化为 `[]` 而非 null；与 catalogResponseDTO items
//     非 null 同模式）。
//
// # V1 §8.2 钦定的 wire 字段集（任一缺失 / 多余 → iOS Codable 解码失败，
// 行 1366-1377）
//
//   - data.groups[].cosmeticItemId:               string（BIGINT 字符串化）
//   - data.groups[].name:                         string
//   - data.groups[].slot:                         number (int，§6.8 枚举)
//   - data.groups[].rarity:                       number (int，§6.9 枚举)
//   - data.groups[].iconUrl:                      string（态 A/B 非空，态 C 空串）
//   - data.groups[].assetUrl:                     string（同 iconUrl）
//   - data.groups[].count:                        number (int = len(instances))
//   - data.groups[].instances[].userCosmeticItemId: string（BIGINT 字符串化）
//   - data.groups[].instances[].status:           number (int，枚举 {1,2})
//   - （**无** code 字段）
func inventoryResponseDTO(groups []service.InventoryGroup) gin.H {
	// 永远返非 nil groups slice（即便 groups 是空）—— §8.2 行 1440 钦定空背包
	// 返 {groups:[]} 而非 null。service 层已用 make([]InventoryGroup, 0, len)
	// 兜底非 nil，这里再保险一次避免任何 future 改动触发 JSON null。
	groupsOut := make([]gin.H, 0, len(groups))
	for _, g := range groups {
		// 每组 instances 同样永不 null（§8.2 行 1440：每组 instances:[] 非 null；
		// service 层组装时 instances 来自聚合必非空，但兜底 make 防御）。
		instances := make([]gin.H, 0, len(g.Instances))
		for _, ins := range g.Instances {
			instances = append(instances, gin.H{
				// userCosmeticItemId 必须 string（§8.2 行 1376 BIGINT 字符串化）
				"userCosmeticItemId": strconv.FormatUint(ins.UserCosmeticItemID, 10),
				// status 是 int 直接下发（§8.2 行 1377，**不**字符串化）
				"status": ins.Status,
			})
		}
		groupsOut = append(groupsOut, gin.H{
			// cosmeticItemId 必须 string（§8.2 行 1368 BIGINT 字符串化）
			"cosmeticItemId": strconv.FormatUint(g.CosmeticItemID, 10),
			"name":           g.Name,
			// slot / rarity / count 是 int 直接下发（§8.2 行 1370-1374，**不**字符串化）
			"slot":      g.Slot,
			"rarity":    g.Rarity,
			"iconUrl":   g.IconURL,
			"assetUrl":  g.AssetURL,
			"count":     g.Count,
			"instances": instances,
		})
	}

	return gin.H{
		"groups": groupsOut,
	}
}
