// Package handler 内 pets_handler.go 节点 5 / Epic 14 引入。
//
// **范围红线**：本文件仅承载 /api/v1/pets/* 路由的 handler 层。当前只实装
// `PostStateSync`（POST /api/v1/pets/current/state-sync, Story 14.2）。
//
// **future 演进**：Story 14.6 / Epic 26 可能加 GetCurrent / GET /pets/current 等
// 查询接口；同 handler 扩方法即可，不另起 PetsHandlerV2。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/middleware"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// PetsHandler 是 /api/v1/pets/* 路由的 handler 集合。
//
// 节点 5 阶段：PostStateSync（POST /pets/current/state-sync，Story 14.2）。
// future Story 14.6 / Epic 26 可能加 GetCurrent / GetWardrobe 等查询接口。
type PetsHandler struct {
	svc service.PetService
}

// NewPetsHandler 构造 PetsHandler。
func NewPetsHandler(svc service.PetService) *PetsHandler {
	return &PetsHandler{svc: svc}
}

// PostStateSyncRequest 是 V1 §5.2 钦定请求体的 Go mirror。
//
// **State 用 *int8 指针类型**（与 7.3 PostSyncRequest.MotionState 同模式 + r2 lessons）：
//   - V1 §5.2 规定 state 字段 required
//   - 若用值类型 int8，client 缺字段 JSON 解析为 zero value（0），与显式传 0 无法区分
//     → 漏掉的 state-sync 会被静默接受为 "state=0" → handler 范围校验 [1,3] 拦截
//     时给出错误信息 "state 必须是 1 / 2 / 3"，但**真实场景是字段缺失** —— 用指针类型 +
//     handler 显式 `if x == nil` 校验，能拦截"字段缺失"并给出更精确的错误信息 "state 必填"
//   - 不用 binding:"required"（validator/v10 在数值字段上把 0 视为缺失 —— 与 motionState
//     同 trap，详见 7.3 PostSyncRequest 注释）
//
// JSON tag 严格对齐 V1 §5.2（camelCase；state 字段就是 "state"）。
type PostStateSyncRequest struct {
	State *int8 `json:"state"` // 指针：区分缺失与显式 0
}

// PostStateSync 处理 POST /api/v1/pets/current/state-sync（Story 14.2）。
//
// 流程：
//  1. ShouldBindJSON 兜一层（字段类型错 → 1002）
//  2. 手动校验：
//     - State 指针非 nil（缺失 → 1002 "state 必填"）
//     - State ∈ {1, 2, 3}（V1 §5.2 + 数据库设计 §6.4）→ 1002 "state 必须是 1 / 2 / 3"
//  3. 从 c.Get(UserIDKey) 拿 userID（auth 中间件已注入）
//  4. 调 svc.SyncCurrentState —— ctx = c.Request.Context()（**不**用 *gin.Context；ADR-0007 §2.2）
//  5. 成功 → response.Success(c, dto)；失败 → c.Error(err) + return（middleware envelope）
//
// **不**直接调 response.Error 写 envelope（ADR-0006 单一 envelope 生产者；与 4.6 / 4.8 / 7.3 / 11.3 同模式）。
//
// **错误码映射**（V1 §5.2）：
//   - 1001：auth 失败（middleware Auth 兜底）
//   - 1002：state 字段缺失 / 类型非 int / 不在 {1,2,3}
//   - 1005：限频（middleware RateLimit 兜底）
//   - 1009：DB 异常（service 层 apperror.Wrap）
//   - **不**触发 1003 / ErrResourceNotFound（pet-less 走 noop，r7 锁定）
//   - **不**触发 3xxx / 4xxx / 5xxx / 6xxx / 7xxx（V1 §5.2 钦定）
func (h *PetsHandler) PostStateSync(c *gin.Context) {
	var req PostStateSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.Wrap(err, apperror.ErrInvalidParam, apperror.DefaultMessages[apperror.ErrInvalidParam]))
		return
	}

	// === required 字段缺失校验（V1 §5.2 钦定 required；指针 nil → 字段未传 → 1002）===
	if req.State == nil {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "state 必填"))
		return
	}

	// === state ∈ {1, 2, 3} 校验（V1 §5.2 + 数据库设计 §6.4 + r9 sweep）===
	if *req.State < 1 || *req.State > 3 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "state 必须是 1 / 2 / 3"))
		return
	}

	// 从 auth 中间件取 userID（与 home_handler.LoadHome / steps_handler.PostSync / room_handler.* 同模式）
	v, ok := c.Get(middleware.UserIDKey)
	if !ok {
		// unreachable: Auth 中间件挂在前；保险兜底走 1009
		_ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
		return
	}
	userID, ok := v.(uint64)
	if !ok {
		_ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
		return
	}

	out, err := h.svc.SyncCurrentState(c.Request.Context(), service.SyncCurrentStateInput{
		UserID: userID,
		State:  *req.State,
	})
	if err != nil {
		_ = c.Error(err) // service 已 wrap *AppError；ErrorMappingMiddleware 写 envelope
		return
	}

	response.Success(c, postStateSyncResponseDTO(out), "ok")
}

// postStateSyncResponseDTO 把 service 输出转成 V1 §5.2 wire 格式 `data: {state: int}`。
//
// data.state 字段是 server-acknowledged ack 信号（回显入参，与 service step 4 UPDATE
// 入库的入参值完全等价；r2 / r10 lessons 锁定 ack 不入权威等价桶）。
func postStateSyncResponseDTO(out *service.SyncCurrentStateOutput) gin.H {
	return gin.H{
		"state": out.State,
	}
}
