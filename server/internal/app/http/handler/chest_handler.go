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

// ChestHandler 是 /api/v1/chest/* 路由的 handler 集合。
//
// 节点 7 阶段：GetCurrent（GET /chest/current，Story 20.5）；
// future Story 20.6 加 PostOpen（POST /chest/open）。
type ChestHandler struct {
	svc service.ChestService
}

// NewChestHandler 构造 ChestHandler。
func NewChestHandler(svc service.ChestService) *ChestHandler {
	return &ChestHandler{svc: svc}
}

// GetCurrent 处理 GET /api/v1/chest/current（Story 20.5）。
//
// 流程：
//  1. 从 c.Get(UserIDKey) 拿 userID（auth 中间件已注入；不存在 → 1009 unreachable 兜底）
//  2. 调 svc.GetCurrent(ctx, userID) —— ctx = c.Request.Context()（**不**用 *gin.Context；ADR-0007 §2.2）
//  3. 成功 → response.Success(c, dto, "ok")；失败 → c.Error(err) + return（middleware envelope）
//
// **不**做参数校验（GET 无 body / 无 query；userID 由 auth 中间件兜底）；与 home_handler.LoadHome /
// steps_handler.GetAccount 同模式。
//
// **不**直接调 response.Error 写 envelope（ADR-0006 单一 envelope 生产者；与 4.6 / 4.8 / 7.4 同模式）。
func (h *ChestHandler) GetCurrent(c *gin.Context) {
	// 从 auth 中间件取 userID（与 home_handler.LoadHome / steps_handler.GetAccount 同模式）
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

	out, err := h.svc.GetCurrent(c.Request.Context(), userID)
	if err != nil {
		_ = c.Error(err) // service 已 wrap *AppError；ErrorMappingMiddleware 写 envelope
		return
	}

	response.Success(c, getCurrentChestResponseDTO(out), "ok")
}

// getCurrentChestResponseDTO 把 service 输出转成 V1 §7.1 wire 格式。
//
// **关键 schema**（V1 §7.1 行 848-854 钦定）：扁平 5 字段：
//   - id: string（BIGINT 字符串化，V1 §2.5 + §7.1 行 850 钦定）
//   - status: int 1 / 2（**动态判定后**值，非 DB 原值）
//   - unlockAt: string ISO 8601 UTC（如 "2026-04-23T10:20:00Z"；time.RFC3339 格式）
//   - openCostSteps: int（节点 7 阶段固定 1000）
//   - remainingSeconds: int（≥ 0；service 层已用 max(0, ...) 兜底）
//
// **与 home_handler chest 块字段一致**（home_handler.go 行 141-147）：
// 节点 2 阶段 GET /home 已下发同一组字段；本 story 下发同一组字段；future 任一字段调整需要同步 V1 §5.1 + §7.1。
//
// **不复用 home_handler 的 chest 块**（home_handler 直接 inline gin.H 在 LoadHome 内）—— 本 story
// 独立 helper 函数，让 chest_handler 自包含；当 V1 §7.1 与 §5.1 chest schema 出现差异时
// （虽然当前一致）便于独立演化。
func getCurrentChestResponseDTO(out *service.ChestBrief) gin.H {
	return gin.H{
		"id":               strconv.FormatUint(out.ID, 10),
		"status":           out.Status,
		"unlockAt":         out.UnlockAt.Format(time.RFC3339),
		"openCostSteps":    out.OpenCostSteps,
		"remainingSeconds": out.RemainingSeconds,
	}
}
