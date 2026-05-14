package handler

import (
	"github.com/gin-gonic/gin"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// DevChestHandler 是 /dev/force-unlock-chest 等 dev 宝箱端点的 handler 集合（Story 20.7）。
//
// 与 ChestHandler (20.5 / 20.6) 区分：
//   - ChestHandler 处理 /api/v1/chest/*（业务接口；含 auth + rate_limit + 业务事务 / 幂等）
//   - DevChestHandler 处理 /dev/force-unlock-chest（dev 工具；不含 auth / rate_limit / 事务）
//
// 与 DevStepsHandler (Story 7.5) 平级：dev 工具按"业务模块"独立 handler,让未来加
// /dev/grant-cosmetic-batch (Epic 20.8) 时有独立 handler 槽位（DevCosmeticHandler），
// 避免单文件膨胀。
//
// 独立 handler 文件让"dev 工具"与"业务接口"边界清晰，dev 路径未来加新 chest 端点
// （如 /dev/reset-chest）也走本 handler。
type DevChestHandler struct {
	svc service.DevChestService
}

// NewDevChestHandler 构造 DevChestHandler。
func NewDevChestHandler(svc service.DevChestService) *DevChestHandler {
	return &DevChestHandler{svc: svc}
}

// PostForceUnlockChestRequest 是 POST /dev/force-unlock-chest 请求体的 Go mirror。
//
// epics.md §Story 20.7 行 2941 钦定：`{userId: int64}`。
//
// **userId 用 *uint64 指针类型**（不挂 binding:"required"）：
//   - validator/v10 把 0 视为 zero value 会误判 "required"，与 7.5 PostGrantStepsRequest 同模式
//   - 用 *uint64 指针 + handler 显式 nil 校验区分 "字段缺失" vs "显式传 0"
//   - userId=0 在 MySQL users 表里**不存在**（AUTO_INCREMENT 从 1 起），handler 显式拒
//     让错误更早 + 错误消息更精确（"userId 必须 > 0"）
//
// **不**接 unlockAt 字段（dev 产品语义是"立刻可开"；未来如需"滚动倒计时 demo"加独立端点）。
// **不**接 idempotencyKey 字段（dev 端点是"故意可重复"语义；重复调都把 unlock_at 推到本次 now）。
type PostForceUnlockChestRequest struct {
	UserID *uint64 `json:"userId"`
}

// PostForceUnlockChest 处理 POST /dev/force-unlock-chest（Story 20.7）。
//
// 流程：
//  1. ShouldBindJSON 兜一层（字段类型错 → 1002）
//  2. 手动校验：userId 非 nil（缺失 → 1002）+ != 0（非法 → 1002）
//  3. 调 svc.ForceUnlockChest(ctx, *userId) —— ctx = c.Request.Context()
//  4. 成功 → response.Success(c, postForceUnlockChestResponseDTO(...), "ok")
//  5. 失败 → c.Error(err) + return（middleware envelope；ADR-0006 单一 envelope 生产者）
//
// **不**做 auth 校验（dev 端点不要求 auth）；**不**取 c.Get(UserIDKey)（dev 路径无 auth 中间件）。
func (h *DevChestHandler) PostForceUnlockChest(c *gin.Context) {
	var req PostForceUnlockChestRequest
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

	if err := h.svc.ForceUnlockChest(c.Request.Context(), *req.UserID); err != nil {
		_ = c.Error(err) // service 已 wrap *AppError；ErrorMappingMiddleware 写 envelope
		return
	}

	// 成功响应 —— 简单 ack，不返当前 chest 状态（如要查 chest 用 GET /chest/current；端点单一职责）
	response.Success(c, postForceUnlockChestResponseDTO(*req.UserID), "ok")
}

// postForceUnlockChestResponseDTO 拼装 ack response。
//
// **schema 选择**：返 `{userId}` 简单 ack —— 不返当前 chest 状态。
//   - 调用方（demo / 自动化测试 / Epic 21 iOS）调本端点后再调 GET /chest/current
//     验证 status=2，而不是依赖本端点 response —— 端点单一职责（force-unlock 只负责"做了"，
//     get-current 只负责"读了"）
//   - 与 Story 7.5 postGrantStepsResponseDTO 同模式（dev 端点统一 ack 风格）
func postForceUnlockChestResponseDTO(userID uint64) gin.H {
	return gin.H{
		"userId": userID,
	}
}
