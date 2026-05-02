package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/middleware"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// StepsHandler 是 /api/v1/steps/* 路由的 handler 集合。
//
// 节点 3 阶段：PostSync（POST /steps/sync，Story 7.3）；
// future Story 7.4 加 GetAccount（GET /steps/account）。
type StepsHandler struct {
	svc service.StepService
}

// NewStepsHandler 构造 StepsHandler。
func NewStepsHandler(svc service.StepService) *StepsHandler {
	return &StepsHandler{svc: svc}
}

// PostSyncRequest 是 V1 §6.1.2 钦定请求体的 Go mirror。
//
// **不**用 binding:"min/max" tags 做范围校验（与 4.6 同模式）：
//   - validator/v10 错误信息英文不可控；手动校验后用 apperror.New + 中文具体描述
//   - syncDate 格式校验需要 time.Parse("2006-01-02")，binding 不支持
//
// JSON tag 严格对齐 V1 §6.1.2（camelCase）。
//
// **不**给 ClientTotalSteps / MotionState / ClientTimestamp 加 binding:"required"：
// `required` 在 int 上把 0 视为缺失（validator/v10 默认行为）；本字段允许 0
// （如首次同步当日 0 步 / clientTimestamp 不允许 0 但由手动校验拦截）。
// 全部用手动校验，文案统一可控。SyncDate 是 string，"required" 拦空串可用。
type PostSyncRequest struct {
	SyncDate         string `json:"syncDate" binding:"required"`
	ClientTotalSteps int64  `json:"clientTotalSteps"` // 用 int64 接 client；service 转 uint64
	MotionState      int8   `json:"motionState"`
	ClientTimestamp  int64  `json:"clientTimestamp"` // ms；用 int64 接，service 转 uint64
}

// PostSync 处理 POST /api/v1/steps/sync。
//
// 流程：
//  1. ShouldBindJSON 兜一层（字段缺失 / 类型错 → 1002）
//  2. 手动校验：syncDate 格式 YYYY-MM-DD / clientTotalSteps ≥ 0 / motionState ∈ {1,2,3} / clientTimestamp > 0
//  3. 从 c.Get(UserIDKey) 拿 userID（auth 中间件已注入）
//  4. 调 svc.SyncSteps —— ctx = c.Request.Context()（**不**用 *gin.Context；ADR-0007 §2.2）
//  5. 成功 → response.Success(c, dto)；失败 → c.Error(err) + return（middleware envelope）
//
// **不**直接调 response.Error 写 envelope（ADR-0006 单一 envelope 生产者；与 4.6 / 4.8 同模式）。
func (h *StepsHandler) PostSync(c *gin.Context) {
	var req PostSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.Wrap(err, apperror.ErrInvalidParam, apperror.DefaultMessages[apperror.ErrInvalidParam]))
		return
	}

	// syncDate 格式校验（V1 §6.1.2 钦定 YYYY-MM-DD 严格 10 字符）
	if len(req.SyncDate) != 10 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "syncDate 必须是 YYYY-MM-DD 格式（10 字符）"))
		return
	}
	// **关键时区规约**：用 time.ParseInLocation + time.Local 而非 time.Parse。
	//
	// 背景：DSN 是 `parseTime=true&loc=Local`（见 configs/local.yaml + infra/config
	// config.go §35）；mysql driver 序列化 time.Time → DATE 列时会把 time 转
	// 到连接的 loc（time.Local）再 format YYYY-MM-DD。
	//
	// 反例（旧 `time.Parse`）：返回 `2026-05-01 00:00:00 UTC`。
	//   - 服务器 TZ +08:00 → driver 转为 `2026-05-01 08:00:00 Local` → DATE 写
	//     `2026-05-01`（看似对，但是巧合 —— 正偏移让日期"飘进"同一天）。
	//   - 服务器 TZ -05:00（如 us-east-1）→ driver 转为 `2026-04-30 19:00:00
	//     Local` → DATE 写 `2026-04-30`（**bug：少一天**）。后果：当日步数
	//     差值入错日期分桶 / dailyCap 累计错日 / 跨端审计串日。
	//
	// 修复（time.ParseInLocation, time.Local）：返回 `2026-05-01 00:00:00 Local`。
	// 不论服务器 TZ 是 +08 / -05 / 0，driver 转回 Local 都是 no-op（同一 loc），
	// DATE 列写入就是用户传的那个日历日。time.Local 与 DSN loc=Local 锁同步。
	//
	// 详见 docs/lessons/2026-05-02-mysql-date-gorm-time-tz-pitfall.md。
	syncDate, err := time.ParseInLocation("2006-01-02", req.SyncDate, time.Local)
	if err != nil {
		_ = c.Error(apperror.Wrap(err, apperror.ErrInvalidParam, "syncDate 格式不符 YYYY-MM-DD"))
		return
	}

	// clientTotalSteps ≥ 0（V1 §6.1.2；JSON int64 < 0 → 1002）
	if req.ClientTotalSteps < 0 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "clientTotalSteps 不能为负数"))
		return
	}

	// motionState ∈ {1, 2, 3}（V1 §6.1.3 + 数据库设计 §6.5）
	if req.MotionState < 1 || req.MotionState > 3 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "motionState 必须是 1 / 2 / 3"))
		return
	}

	// clientTimestamp > 0（V1 §6.1.2；ms epoch）
	if req.ClientTimestamp <= 0 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "clientTimestamp 必须 > 0"))
		return
	}

	// 从 auth 中间件取 userID
	v, ok := c.Get(middleware.UserIDKey)
	if !ok {
		// unreachable: Auth 中间件挂在前；保险兜底走 1009（与 home_handler 同模式）
		_ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
		return
	}
	userID, ok := v.(uint64)
	if !ok {
		_ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
		return
	}

	out, err := h.svc.SyncSteps(c.Request.Context(), service.SyncStepsInput{
		UserID:           userID,
		SyncDate:         syncDate,
		ClientTotalSteps: uint64(req.ClientTotalSteps),
		MotionState:      req.MotionState,
		ClientTimestamp:  uint64(req.ClientTimestamp),
	})
	if err != nil {
		_ = c.Error(err) // service 已 wrap *AppError；ErrorMappingMiddleware 写 envelope
		return
	}

	response.Success(c, postSyncResponseDTO(out), "ok")
}

// postSyncResponseDTO 把 service 输出转成 V1 §6.1.5 wire 格式。
//
// **关键**：嵌套 stepAccount 子对象（区别于 §6.2 GET /steps/account 的扁平结构）。
// V1 §6.2 引用块明确声明这两端不同设计的原因，DTO 不能复用。
func postSyncResponseDTO(out *service.SyncStepsOutput) gin.H {
	return gin.H{
		"acceptedDeltaSteps": out.AcceptedDeltaSteps,
		"stepAccount": gin.H{
			"totalSteps":     out.StepAccount.TotalSteps,
			"availableSteps": out.StepAccount.AvailableSteps,
			"consumedSteps":  out.StepAccount.ConsumedSteps,
		},
	}
}
