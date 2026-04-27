package handler

import (
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/middleware"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// HomeHandler 是 /home 路由的 handler。
//
// 节点 2 阶段仅 LoadHome（GET /api/v1/home）；future epic 加 GET /me 等
// User / Home 模块路由（设计文档 §6.2）。
type HomeHandler struct {
	svc service.HomeService
}

// NewHomeHandler 构造 HomeHandler。
//
// 注入 HomeService（service 层 interface）—— handler 单测直接传 stub struct
// 实现该 interface，不需要起 *gorm.DB / 真 mysql。与 4.6 NewAuthHandler 同模式。
func NewHomeHandler(svc service.HomeService) *HomeHandler {
	return &HomeHandler{svc: svc}
}

// LoadHome 处理 GET /api/v1/home。
//
// # 流程
//
//  1. 从 gin.Context.Keys 取 userID（middleware.UserIDKey 由 Auth 中间件注入）
//  2. 调 svc.LoadHome(ctx, userID) 一次性聚合查询
//  3. 成功 → response.Success(c, dto, "ok")
//  4. 失败 → c.Error(err) + return（让 ErrorMappingMiddleware 写 envelope）
//
// # 关键
//
// 本 handler **不**做参数校验：路径 + auth 头由 router 中间件链兜底；userID 必然存在
// （Auth 中间件挂在前；不存在 → 已被 1001 拦截）；类型断言失败 → 走 1009 兜底
// （unreachable 但保险起见，与 4.5 RateLimitByUserID 同模式）。
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
func (h *HomeHandler) LoadHome(c *gin.Context) {
	v, ok := c.Get(middleware.UserIDKey)
	if !ok {
		// unreachable: Auth 中间件挂在前，userID 必然存在；保险兜底走 1009
		_ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
		return
	}
	userID, ok := v.(uint64)
	if !ok {
		// unreachable: Auth 中间件 c.Set(UserIDKey, claims.UserID) 永远是 uint64
		_ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
		return
	}

	out, err := h.svc.LoadHome(c.Request.Context(), userID)
	if err != nil {
		// service 已 wrap *AppError；handler 透传，让 ErrorMappingMiddleware 写 envelope
		_ = c.Error(err)
		return
	}

	response.Success(c, homeResponseDTO(out), "ok")
}

// homeResponseDTO 把 service 输出转成 V1 §5.1 钦定的 wire 格式。
//
// # 关键转换
//
//   - BIGINT id 用 strconv.FormatUint 转 string（V1 §2.5 钦定，避免 JS Number 精度丢失）
//   - pet 可空：out.Pet == nil → "pet": null（**不**是 {} 空对象 —— V1 §5.1 行 335 钦定）
//   - chest.unlockAt 用 RFC3339 格式（time.Time.Format(time.RFC3339)）
//   - chest.remainingSeconds 已在 service 层算好（int 秒数；可能为 0）
//   - room.currentRoomId 节点 2 阶段固定 nil（gin.H{"currentRoomId": nil} 序列化为 null）
//   - pet.equips 节点 2 阶段固定 `[]any{}`（**不**用 nil —— nil slice 序列化为 null）
//
// # V1 §5.1 节点 2 阶段必须严格返回的字段集（任一缺失 → iOS DTO 解码失败）
//
//   - data.user: id / nickname / avatarUrl
//   - data.pet（可空）: id / petType / name / currentState / equips（[]）
//   - data.stepAccount: totalSteps / availableSteps / consumedSteps
//   - data.chest: id / status / unlockAt / openCostSteps / remainingSeconds
//   - data.room: currentRoomId（null）
func homeResponseDTO(out *service.HomeOutput) gin.H {
	// petDTO 用 any 类型（非 gin.H）：让 nil 序列化为 JSON null（gin.H 是 map，
	// nil map 序列化也是 null，但 any 类型最稳定且 reviewer 一目了然）
	var petDTO any
	if out.Pet != nil {
		petDTO = gin.H{
			"id":           strconv.FormatUint(out.Pet.ID, 10),
			"petType":      out.Pet.PetType,
			"name":         out.Pet.Name,
			"currentState": out.Pet.CurrentState,
			// equips 节点 2 阶段强制 []：用 []any{} 而非 nil（nil slice 序列化为 null）。
			// Story 26.6 节点 9 才把这里替换成真实 []equipDTO{...}，DTO 嵌套结构对齐 V1 §5.1。
			"equips": []any{},
		}
	}
	return gin.H{
		"user": gin.H{
			"id":        strconv.FormatUint(out.User.ID, 10),
			"nickname":  out.User.Nickname,
			"avatarUrl": out.User.AvatarURL,
		},
		"pet": petDTO,
		"stepAccount": gin.H{
			"totalSteps":     out.StepAccount.TotalSteps,
			"availableSteps": out.StepAccount.AvailableSteps,
			"consumedSteps":  out.StepAccount.ConsumedSteps,
		},
		"chest": gin.H{
			"id":               strconv.FormatUint(out.Chest.ID, 10),
			"status":           out.Chest.Status,
			"unlockAt":         out.Chest.UnlockAt.Format(time.RFC3339),
			"openCostSteps":    out.Chest.OpenCostSteps,
			"remainingSeconds": out.Chest.RemainingSeconds,
		},
		"room": gin.H{
			// 节点 2 阶段强制 null（即便 users.current_room_id 字段已存在）；
			// Story 11.10 节点 4 才接 users.current_room_id 真值。
			"currentRoomId": nil,
		},
	}
}
