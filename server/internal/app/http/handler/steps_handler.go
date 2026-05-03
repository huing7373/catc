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
//   - syncDate 格式校验需要严格 10 字符 + 字符组合，binding 不支持
//
// JSON tag 严格对齐 V1 §6.1.2（camelCase）。
//
// **关键：ClientTotalSteps / MotionState / ClientTimestamp 用 *指针类型***
// （Story 7.3 review r2 [P2]）：
//   - V1 §6.1.2 规定这三个字段都是 required
//   - 若用值类型 int64 / int8，client 缺字段 JSON 解析为 zero value（0），
//     与显式传 0 无法区分 → 漏掉的 sync 会被静默接受为"0 步同步"
//   - 用指针类型 + handler 显式 `if x == nil` 校验，能拦截"字段缺失"场景
//   - 不能用 binding:"required"（validator/v10 在数值字段上把 0 视为缺失，
//     首次同步当日 0 步会被误拒；详见 PostSyncRequest 旧注释 + 7.3 r1 lessons）
//
// SyncDate 用 string + binding:"required"（空串会被拦截）即可，无歧义。
type PostSyncRequest struct {
	SyncDate         string `json:"syncDate" binding:"required"`
	ClientTotalSteps *int64 `json:"clientTotalSteps"` // 指针：区分缺失与显式 0；service 转 uint64
	MotionState      *int8  `json:"motionState"`      // 指针：区分缺失与显式 0
	ClientTimestamp  *int64 `json:"clientTimestamp"`  // 指针：区分缺失与显式 0；service 转 uint64
}

// PostSync 处理 POST /api/v1/steps/sync。
//
// 流程：
//  1. ShouldBindJSON 兜一层（字段类型错 → 1002）
//  2. 手动校验：
//     - 三个 required pointer 字段非 nil（缺失 → 1002）
//     - syncDate 格式 YYYY-MM-DD（10 字符 + ParseInLocation 校格式合法性）
//     - clientTotalSteps ≥ 0 / motionState ∈ {1,2,3} / clientTimestamp > 0
//  3. 从 c.Get(UserIDKey) 拿 userID（auth 中间件已注入）
//  4. 调 svc.SyncSteps —— ctx = c.Request.Context()（**不**用 *gin.Context；ADR-0007 §2.2）
//     SyncDate 全程作为 string 透传（避免 time.Time loc 与 DSN loc 错配；详见 service.SyncStepsInput）
//  5. 成功 → response.Success(c, dto)；失败 → c.Error(err) + return（middleware envelope）
//
// **不**直接调 response.Error 写 envelope（ADR-0006 单一 envelope 生产者；与 4.6 / 4.8 同模式）。
func (h *StepsHandler) PostSync(c *gin.Context) {
	var req PostSyncRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.Wrap(err, apperror.ErrInvalidParam, apperror.DefaultMessages[apperror.ErrInvalidParam]))
		return
	}

	// === required 字段缺失校验（Story 7.3 review r2 [P2]）===
	// V1 §6.1.2 钦定 required；指针 nil → 字段未传 → 1002。
	if req.ClientTotalSteps == nil {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "clientTotalSteps 必填"))
		return
	}
	if req.MotionState == nil {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "motionState 必填"))
		return
	}
	if req.ClientTimestamp == nil {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "clientTimestamp 必填"))
		return
	}

	// === syncDate 格式校验（V1 §6.1.2 钦定 YYYY-MM-DD 严格 10 字符）===
	if len(req.SyncDate) != 10 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "syncDate 必须是 YYYY-MM-DD 格式（10 字符）"))
		return
	}
	// **关键：syncDate 全程 string 穿透 service / repo / DB**（Story 7.3 review r2 [P2]）：
	//
	// 旧路径（time.Time）：handler ParseInLocation → service.SyncStepsInput.SyncDate (time.Time)
	// → repo `WHERE sync_date = ?` 传 time.Time → mysql driver 用 DSN loc 把 time.Time
	// 转 'YYYY-MM-DD' 字面值再发给 server。**两次时区转换**（Go time.Time loc → DSN loc
	// → DATE 字符串）任意一环错配都会让日历日漂移。
	//   - r1 修复用 ParseInLocation(time.Local) 假设 DSN loc=Local；但 DSN 是配置项，
	//     prod 常见 loc=UTC 时仍会漂日。
	//
	// 新路径（string）：handler 校验 `len == 10` + 格式合法性 → 直传 string 到
	// service / repo → mysql driver 走"VARCHAR → DATE"隐式转换（按 'YYYY-MM-DD'
	// 字面值解释，无时区语义）→ **完全无时区耦合，不依赖 DSN loc 配置**。
	//
	// 详见 docs/lessons/2026-05-02-mysql-date-string-transit.md 与上一轮
	// docs/lessons/2026-05-02-mysql-date-gorm-time-tz-pitfall.md 的递进。
	//
	// 此处仍调 ParseInLocation **仅用于校验**字符串是否合法 YYYY-MM-DD（有效月日 / 闰年 /
	// 非数字字符等），**不**把返回的 time.Time 往下传。loc 用 time.UTC 即可（无副作用，
	// 校验只看 err；不会出现"2026-02-29 在某 loc 合法在另一个 loc 非法"的歧义，因为
	// time.ParseInLocation 不依赖 loc 校验日期合法性）。
	if !isValidYYYYMMDD(req.SyncDate) {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "syncDate 格式不符 YYYY-MM-DD"))
		return
	}

	// clientTotalSteps ≥ 0（V1 §6.1.2；JSON int64 < 0 → 1002）
	if *req.ClientTotalSteps < 0 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "clientTotalSteps 不能为负数"))
		return
	}

	// motionState ∈ {1, 2, 3}（V1 §6.1.3 + 数据库设计 §6.5）
	if *req.MotionState < 1 || *req.MotionState > 3 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "motionState 必须是 1 / 2 / 3"))
		return
	}

	// clientTimestamp > 0（V1 §6.1.2；ms epoch）
	if *req.ClientTimestamp <= 0 {
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
		SyncDate:         req.SyncDate, // string 直传（无时区转换；详见上方注释）
		ClientTotalSteps: uint64(*req.ClientTotalSteps),
		MotionState:      *req.MotionState,
		ClientTimestamp:  uint64(*req.ClientTimestamp),
	})
	if err != nil {
		_ = c.Error(err) // service 已 wrap *AppError；ErrorMappingMiddleware 写 envelope
		return
	}

	response.Success(c, postSyncResponseDTO(out), "ok")
}

// isValidYYYYMMDD 校验字符串是否为合法 YYYY-MM-DD（有效月日 / 闰年）。
//
// 实装用 time.Parse 走纯字符串校验（不引入 time.Local / DSN loc 任何耦合）。
// caller 已校验 len == 10。
//
// time.Parse 默认 loc=UTC；本函数只看 err（合法性），**不**把返回的 time 实例
// 往下传 → 完全无 loc 语义。日期合法性（如闰年 / 月日范围）不依赖 loc。
func isValidYYYYMMDD(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
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
