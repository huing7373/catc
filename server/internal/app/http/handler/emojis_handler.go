package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// EmojisHandler 是 /emojis 路由的 handler。
//
// 节点 6 阶段仅 GetEmojis（GET /api/v1/emojis）；future epic 加 POST / PATCH /emojis
// 等 admin 端能力（不在 MVP 范围）。
type EmojisHandler struct {
	svc service.EmojiService
}

// NewEmojisHandler 构造 EmojisHandler。
//
// 注入 EmojiService（service 层 interface）—— handler 单测直接传 stub struct
// 实现该 interface，不需要起 *gorm.DB / 真 mysql。与 HomeHandler / RoomHandler 同模式。
func NewEmojisHandler(svc service.EmojiService) *EmojisHandler {
	return &EmojisHandler{svc: svc}
}

// GetEmojis 处理 GET /api/v1/emojis。
//
// # 流程
//
//  1. 调 svc.ListAvailable(ctx) 取所有 enabled 表情
//  2. 成功 → response.Success(c, emojiResponseDTO(briefs), "ok")
//  3. 失败 → c.Error(err) + return（让 ErrorMappingMiddleware 写 envelope）
//
// # 关键
//
// 本 handler **不**做参数校验：V1 §11.1 钦定不接受任何 query 参数 / body 字段，
// 也不读 userID（接口要求 auth 但 service 不需要 user 维度过滤）；auth 头由 router
// 中间件链兜底（authedGroup 已挂 Auth 中间件）。
//
// # ADR-0006 单一 envelope 生产者
//
// 本 handler **不**直接调 response.Error 写 envelope —— 一律走 c.Error + return，
// 由 ErrorMappingMiddleware 兜底翻译成 envelope。详见
// docs/lessons/2026-04-24-error-envelope-single-producer.md。
//
// # ADR-0007 §2.2 ctx 传播
//
// 用 c.Request.Context() 传给 service —— **不**直接传 *gin.Context（其 Done() 是
// nil channel，service 层 select ctx.Done() 不会响应 client 断开）。
func (h *EmojisHandler) GetEmojis(c *gin.Context) {
	briefs, err := h.svc.ListAvailable(c.Request.Context())
	if err != nil {
		// service 已 wrap *AppError；handler 透传，让 ErrorMappingMiddleware 写 envelope
		_ = c.Error(err)
		return
	}

	response.Success(c, emojiResponseDTO(briefs), "ok")
}

// emojiResponseDTO 把 service 输出转成 V1 §11.1 钦定的 wire 格式。
//
// # 关键转换
//
//   - 字段名 snake_case → camelCase：AssetURL → "assetUrl" / SortOrder → "sortOrder"
//     （V1 §2.4 行 138 钦定 wire 全 camelCase；与 home_handler / room_handler 同模式）
//   - **不**下发 ID / IsEnabled / CreatedAt / UpdatedAt（V1 §11.1 行 1815 钦定）
//   - **永远**下发 `items: []` 而非 `items: null`（V1 §11.1 行 1833 钦定）：用
//     `make([]gin.H, 0, len(briefs))` 兜底空 slice → JSON 序列化为 `[]` 而非 null
//     （nil slice 序列化为 null，与 home_handler.go pet null 同模式区分）
//
// # V1 §11.1 钦定的 wire 字段集（任一缺失 → iOS DTO 解码失败）
//
//   - data.items[].code: string
//   - data.items[].name: string
//   - data.items[].assetUrl: string
//   - data.items[].sortOrder: number (int)
func emojiResponseDTO(briefs []service.EmojiBrief) gin.H {
	// 永远返非 nil items slice（即便 briefs 是空）—— V1 §11.1 行 1833 钦定。
	// service 层已经保证 briefs 非 nil（用 make([]EmojiBrief, 0, len(rows)) 兜底），
	// 这里再保险一次避免任何 edge case 触发 JSON null（如 future 改动 service 实装）。
	items := make([]gin.H, 0, len(briefs))
	for _, b := range briefs {
		items = append(items, gin.H{
			"code":      b.Code,
			"name":      b.Name,
			"assetUrl":  b.AssetURL,
			"sortOrder": b.SortOrder,
		})
	}

	return gin.H{
		"items": items,
	}
}
