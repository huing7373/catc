package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// CosmeticsHandler 是 /cosmetics/* 路由的 handler。
//
// 节点 8 阶段（Story 23.3）仅 GetCatalog（GET /api/v1/cosmetics/catalog）；
// Story 23.4 落地 GetInventory（GET /api/v1/cosmetics/inventory）时再补
// （不在本 story 范围）；future epic 加 POST / PATCH /cosmetics 等 admin 端
// 能力（MVP 节点 8 无 admin 后台需求，与 emojis_handler 行 10-13 同模式）。
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
