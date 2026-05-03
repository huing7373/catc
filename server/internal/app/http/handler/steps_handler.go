package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/middleware"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// syncDateToleranceDays 是 syncDate 相对 server today 的允许偏差天数（±N）。
//
// = 2：覆盖跨时区合理场景（含极端 PST↔JST 17 小时差跨日界线）+ 客户端时钟轻微漂移；
// 同时把"旋转日期重复入账攻击"窗口压到 5 天（5×daily_cap=250000 步上限）。
//
// 详见 PostSync 中 syncDate 范围校验段注释 + V1 §6.1.2 GAP E 后续条款 +
// docs/lessons/2026-05-03-step-sync-syncdate-rotation-attack-tolerance-window.md。
const syncDateToleranceDays = 2

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

	// === syncDate 范围校验（Story 7.3 review r7 [P1] anti-cheat）===
	//
	// 攻击面：V1 §6.1.2 GAP E "client 本机时区算今天 / server 直接采用不二次转换"
	// 完全信任客户端提交的日期。若 server 不加任何范围限制，恶意客户端可旋转日期
	// 重复入账：
	//   - sync syncDate=2026-05-01, clientTotalSteps=1000 → 入账 1000（first sync of day）
	//   - sync syncDate=2026-05-02, clientTotalSteps=1000 → 又入账 1000（new day baseline=0）
	//   - sync syncDate=2026-05-03, clientTotalSteps=1000 → 又入账 1000
	//   - 旋转 N 天 → N×1000，且每天独立 50000 daily_cap，等效完全绕过封顶。
	//
	// 修复方向（review r7 推荐选项 A）：保留 GAP E 信任客户端时区的语义，但加
	// server-side syncDate 范围校验：syncDate 必须在 [server today - 2 days,
	// server today + 2 days] 范围内（±2 天覆盖跨时区合理场景，包括极端 PST↔JST
	// 17 小时差跨日界线 + 客户端时钟轻微漂移）。
	//
	// **trade-off（已知 known limitation）**：±2 天窗口仍允许"在小窗口内绕过
	// 5×daily_cap"——5 天独立账本 = 250000 步上限，但不再是无限累积。完全防御
	// 需要 server-side trusted time + device id 绑定（未来 epic 升级方向，
	// 见 lessons 2026-05-04 r7）。
	//
	// **未来引入 clock interface 时**：替换 time.Now() 调用以便测试可控时钟；
	// 当前 service 层未引入 clock，handler 直接 time.Now() + UTC 即可
	// （UTC 比 time.Local 稳定；server 部署 loc 不应影响业务逻辑）。
	parsed, _ := time.Parse("2006-01-02", req.SyncDate) // 此处 err 已被 isValidYYYYMMDD 校验
	serverNow := time.Now().UTC()
	serverToday := time.Date(serverNow.Year(), serverNow.Month(), serverNow.Day(), 0, 0, 0, 0, time.UTC)
	earliest := serverToday.AddDate(0, 0, -syncDateToleranceDays)
	latest := serverToday.AddDate(0, 0, syncDateToleranceDays)
	if parsed.Before(earliest) || parsed.After(latest) {
		_ = c.Error(apperror.New(
			apperror.ErrInvalidParam,
			"syncDate 必须在 server today ± 2 天范围内（跨时区容忍窗口；防止恶意客户端旋转日期重复入账）",
		))
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

// isValidYYYYMMDD 校验字符串是否为合法 YYYY-MM-DD（有效月日 / 闰年 + MySQL DATE 范围）。
//
// 实装用 time.Parse 走纯字符串校验（不引入 time.Local / DSN loc 任何耦合）。
// caller 已校验 len == 10。
//
// time.Parse 默认 loc=UTC；本函数只看 err（合法性）+ 年份范围，**不**把返回的 time 实例
// 往下传 → 完全无 loc 语义。日期合法性（如闰年 / 月日范围）不依赖 loc。
//
// **MySQL DATE range 校验（Story 7.3 review r4 [P2]）**：
// time.Parse 接受 "0999-12-31" 这类 pre-1000 日期，但 MySQL DATE 列只接受
// `1000-01-01` ~ `9999-12-31`（参 https://dev.mysql.com/doc/refman/8.0/en/datetime.html）。
// 若 handler 不拦，request 会跳过 1002 参数校验直接走到 DB → mysql driver 拒 →
// repo 返 ErrServiceBusy → client 看到 1009（"服务繁忙"）而非预期的 1002（"参数错误"）。
//
// 解法：在格式校验通过后追加一道年份范围校验 [1000, 9999]。**不**加业务上界
// （如 ≤ 当前年）—— syncDate 应该是"今天"附近的日期，但 client 时钟漂移 / 跨日 race 会
// 让"未来日期"也偶发合法；保守只用 MySQL DATE 物理范围作为入口拦截，业务侧合理性
// 由 service 层（如 rate-limit / 跨日去重）兜底。
//
// **不**加下界 ≥ 1970（Unix epoch）或 ≥ 2020（业务上线年）—— 同样属于"业务上界"语义，
// 走 service 层而不是 handler 入口。
//
// 反例（r4 之前）：只 `time.Parse` → "0999-12-31" 通过 → DB error → 1009。
func isValidYYYYMMDD(s string) bool {
	parsed, err := time.Parse("2006-01-02", s)
	if err != nil {
		return false
	}
	// MySQL DATE 物理范围 1000-01-01 ~ 9999-12-31。
	// time.Parse("2006-01-02") 已隐式拒掉 5 位年份（如 "10000-01-01"）—— 字符长度不符；
	// 此处只需补 pre-1000 下界。上界保险写一遍，防御未来 layout 调整。
	year := parsed.Year()
	if year < 1000 || year > 9999 {
		return false
	}
	return true
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
