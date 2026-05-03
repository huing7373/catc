package handler

import (
	"github.com/gin-gonic/gin"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// DevStepsHandler 是 /dev/grant-steps 等 dev 步数端点的 handler 集合（Story 7.5）。
//
// 与 StepsHandler (7.3 / 7.4) 区分：
//   - StepsHandler 处理 /api/v1/steps/*（业务接口；含 auth + rate_limit + 防作弊）
//   - DevStepsHandler 处理 /dev/grant-steps（dev 工具；不含 auth / rate_limit / 防作弊）
//
// 独立 handler 文件让"dev 工具"与"业务接口"边界清晰，dev 路径未来加新端点
// （如 /dev/grant-cosmetic-batch in Epic 20.8）也走本 handler。
type DevStepsHandler struct {
	svc service.DevStepService
}

// NewDevStepsHandler 构造 DevStepsHandler。
func NewDevStepsHandler(svc service.DevStepService) *DevStepsHandler {
	return &DevStepsHandler{svc: svc}
}

// PostGrantStepsRequest 是 POST /dev/grant-steps 请求体的 Go mirror。
//
// epics.md §Story 7.5 钦定：`{userId: int64, steps: int}`。
//
// **userId 用 *uint64 指针类型**（不挂 binding:"required"）：
//   - validator/v10 把 0 视为 zero value 会误判 "required"，与 7.3 PostSyncRequest 同模式
//   - 用 *uint64 指针 + handler 显式 nil 校验区分 "字段缺失" vs "显式传 0"
//   - userId=0 在 MySQL users 表里**不存在**（AUTO_INCREMENT 从 1 起），handler 显式拒
//     让错误更早 + 错误消息更精确（"userId 必须 > 0"）
//
// **steps 用 *int32 指针类型**：
//   - 必须区分"字段缺失" vs "显式传 0"（dev grant 0 步是合法 fixture）
//   - service 层 accepts steps>=0；handler 层补 steps<0 → 1002（"steps 不能为负数"）
//   - 类型 int32（与 sync_log.accepted_delta_steps 同；INT signed；上界约 21 亿步够用）
type PostGrantStepsRequest struct {
	UserID *uint64 `json:"userId"`
	Steps  *int32  `json:"steps"`
}

// PostGrantSteps 处理 POST /dev/grant-steps（Story 7.5）。
//
// 流程：
//  1. ShouldBindJSON 兜一层（字段类型错 → 1002）
//  2. 手动校验：
//     - userId 非 nil（缺失 → 1002）+ != 0（非法 → 1002）
//     - steps 非 nil（缺失 → 1002）+ >= 0（负数 → 1002 epics.md §Story 7.5 钦定）
//  3. 调 svc.GrantSteps(ctx, *userId, *steps) —— ctx = c.Request.Context()
//  4. 成功 → response.Success(c, postGrantStepsResponseDTO(...), "ok")
//  5. 失败 → c.Error(err) + return（middleware envelope；ADR-0006 单一 envelope 生产者）
//
// **不**做 auth 校验（dev 端点不要求 auth）；**不**取 c.Get(UserIDKey)（dev 路径无 auth 中间件）。
func (h *DevStepsHandler) PostGrantSteps(c *gin.Context) {
	var req PostGrantStepsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.Wrap(err, apperror.ErrInvalidParam, apperror.DefaultMessages[apperror.ErrInvalidParam]))
		return
	}

	// === required 字段校验（指针 nil → 字段未传 → 1002）===
	if req.UserID == nil {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "userId 必填"))
		return
	}
	if *req.UserID == 0 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "userId 必须 > 0"))
		return
	}
	if req.Steps == nil {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "steps 必填"))
		return
	}
	if *req.Steps < 0 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "steps 不能为负数"))
		return
	}

	if err := h.svc.GrantSteps(c.Request.Context(), *req.UserID, *req.Steps); err != nil {
		_ = c.Error(err) // service 已 wrap *AppError；ErrorMappingMiddleware 写 envelope
		return
	}

	// 成功响应 —— 简单 ack，不返当前账户值（如要查账户用 GET /steps/account；端点单一职责）
	response.Success(c, postGrantStepsResponseDTO(*req.UserID, *req.Steps), "ok")
}

// postGrantStepsResponseDTO 拼装 ack response。
//
// **schema 选择**：返 `{userId, grantedSteps}` 简单 ack —— 不返当前账户三档值。
//   - 调用方（Story 9.1 e2e / 自动化测试）调本端点后再调 GET /steps/account 验证最终值，
//     而不是依赖本端点 response —— 端点单一职责（grant 只负责"做了"，account 只负责"读了"）
//   - 反模式：返当前账户值 → 调用方依赖该字段 → 未来加 grant 多个 user 批量端点时 schema 难扩展
func postGrantStepsResponseDTO(userID uint64, grantedSteps int32) gin.H {
	return gin.H{
		"userId":       userID,
		"grantedSteps": grantedSteps,
	}
}
