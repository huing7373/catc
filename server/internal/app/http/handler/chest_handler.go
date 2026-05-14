package handler

import (
	stderrors "errors"
	"regexp"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/middleware"
	"github.com/huing/cat/server/internal/infra/config"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// ChestHandler 是 /api/v1/chest/* 路由的 handler 集合。
//
// 节点 7 阶段：
//   - GetCurrent（GET /chest/current，Story 20.5）
//   - Open（POST /chest/open，Story 20.6）
type ChestHandler struct {
	svc service.ChestService

	// Story 20.6 引入字段：
	//   - idempotencyChecker: handler 入口 autocommit idempotency 预检（V1 §7.2.3 钦定）；
	//     决定"是否需要 rate_limit"（命中 committed success → 跳过 rate_limit；否则做）
	//   - rateLimitCfg: handler 内层 rate_limit 配置（V1 §7.2.5.4 r10 钦定）
	//   - nowFn: openChestResponseDTO 同源同时刻补算 status / remainingSeconds（V1 §7.2 r11）；
	//     单测可覆盖，生产默认 `func() time.Time { return time.Now().UTC() }`
	idempotencyChecker mysql.IdempotencyRepo
	rateLimitCfg       config.RateLimitConfig
	nowFn              func() time.Time
}

// NewChestHandler 构造 ChestHandler。Story 20.6 扩签名为 3 参数。
func NewChestHandler(svc service.ChestService, idempotencyChecker mysql.IdempotencyRepo, rateLimitCfg config.RateLimitConfig) *ChestHandler {
	return &ChestHandler{
		svc:                svc,
		idempotencyChecker: idempotencyChecker,
		rateLimitCfg:       rateLimitCfg,
		nowFn:              func() time.Time { return time.Now().UTC() },
	}
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

// Open 处理 POST /api/v1/chest/open（Story 20.6）。
//
// 流程（V1 §7.2.5）：
//  1. 入参解析 + idempotencyKey regex 校验（[A-Za-z0-9_:-] + 1-128 length）
//  2. handler 入口 autocommit idempotency 预检（V1 §7.2.3）：
//     - 命中 status='success' → committedSuccessReplay = true → 跳过 rate_limit（V1 §7.2.5.4 r10 钦定）
//     - 未命中 / pending → 必须调 middleware.CheckRateLimitByUserID 兜底
//  3. 调 service.OpenChest（service 内部独立做幂等命中复查 + 业务事务；幂等安全）
//  4. 响应转译（含 NextChest.Status / RemainingSeconds 补算）
//
// **关键差异 vs GetCurrent**：本接口路由层**不**挂 RateLimit middleware（router.go
// chestOpenGroup 仅挂 Auth），rate_limit 在 handler 内层做；V1 §7.2.5.4 r10 钦定。
//
// **双重 SELECT 设计**：handler 入口 SELECT 决定"是否做 rate_limit"，service 内 SELECT
// 决定"是否走 cached replay"；两层独立判断幂等状态是契约要求（V1 §7.2.3 + §7.2.5b 钦定）。
func (h *ChestHandler) Open(c *gin.Context) {
	// 1. 从 c.Get(UserIDKey) 拿 userID（auth 中间件已注入）
	v, ok := c.Get(middleware.UserIDKey)
	if !ok {
		_ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
		return
	}
	userID, ok := v.(uint64)
	if !ok {
		_ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
		return
	}

	// 2. 解析 + 校验 idempotencyKey
	var req openChestRequestDTO
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.Wrap(err, apperror.ErrInvalidParam, "request body invalid"))
		return
	}
	if !isValidIdempotencyKey(req.IdempotencyKey) {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "idempotencyKey invalid (must be 1-128 chars, [A-Za-z0-9_:-])"))
		return
	}

	// 3. handler 入口 idempotency 预检 + cached success replay 短路决策 + rate_limit 兜底
	committedSuccessReplay := false
	if h.idempotencyChecker != nil { // 单测构造可注入 nil 跳过预检
		cached, cachedErr := h.idempotencyChecker.FindByUserIDAndKey(c.Request.Context(), userID, req.IdempotencyKey)
		if cachedErr != nil && !stderrors.Is(cachedErr, mysql.ErrIdempotencyRecordNotFound) {
			_ = c.Error(apperror.Wrap(cachedErr, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
			return
		}
		if cached != nil && cached.Status == mysql.IdempotencyStatusSuccess {
			committedSuccessReplay = true
		}
	}

	if !committedSuccessReplay {
		// 未命中 success → 必须 rate_limit 兜底（V1 §7.2.5.4 r10）
		if rlErr := middleware.CheckRateLimitByUserID(c.Request.Context(), h.rateLimitCfg, userID); rlErr != nil {
			_ = c.Error(rlErr)
			return
		}
	}
	// 命中 success → 跳过 rate_limit，直接走 service（service 内部再做 cached replay 解析）

	// 4. 调 service
	requestID, _ := c.Get(middleware.RequestIDKey)
	requestIDStr, _ := requestID.(string)
	in := service.OpenChestInput{
		UserID:         userID,
		IdempotencyKey: req.IdempotencyKey,
		RequestID:      requestIDStr,
	}
	out, err := h.svc.OpenChest(c.Request.Context(), in)
	if err != nil {
		_ = c.Error(err)
		return
	}

	// 5. 响应转译（含 NextChest.Status / RemainingSeconds 补算）—— 同源同时刻（V1 §7.2 r11）
	response.Success(c, openChestResponseDTO(out, h.nowFn()), "ok")
}

// openChestRequestDTO V1 §7.2 钦定请求体。
type openChestRequestDTO struct {
	IdempotencyKey string `json:"idempotencyKey" binding:"required"`
}

// idempotencyKeyRegex 是 V1 §7.2 钦定字符集 [A-Za-z0-9_:-] + length 1-128 校验
// （package-level var 编译期建立，避免每次请求 re-parse）。
var idempotencyKeyRegex = regexp.MustCompile(`^[A-Za-z0-9_:-]{1,128}$`)

func isValidIdempotencyKey(key string) bool {
	return idempotencyKeyRegex.MatchString(key)
}

// openChestResponseDTO 转译 service 输出为 V1 §7.2 wire 格式。
//
// **关键 schema**（V1 §7.2 钦定）：
//   - data.reward.{userCosmeticItemId, cosmeticItemId, name, slot, rarity, assetUrl, iconUrl}
//     - userCosmeticItemId: string "0" 占位（节点 7 阶段）
//     - cosmeticItemId: BIGINT 字符串化
//   - data.stepAccount.{totalSteps, availableSteps, consumedSteps}: number
//   - data.nextChest.{id (string), status (int 1/2), unlockAt (RFC3339), openCostSteps (int), remainingSeconds (int)}
//     - status / remainingSeconds 由本 helper **同源同时刻**按 now 补算（V1 §7.2 r11）
//
// **入参 now 显式注入**而非内部调 time.Now()：让 status / remainingSeconds 用同一时刻
// 计算，避免出现 "status=1 + remainingSeconds=0" 不可能组合（V1 §7.2 r11 锁定）。
func openChestResponseDTO(out *service.OpenChestOutput, now time.Time) gin.H {
	nextChestStatus := int8(1)
	if !out.NextChest.UnlockAt.After(now) {
		nextChestStatus = 2
	}
	diff := out.NextChest.UnlockAt.Sub(now)
	remainingSeconds := int64(0)
	if diff > 0 {
		// ceil((unlockAt - now) / 1s)
		remainingSeconds = int64((diff + time.Second - 1) / time.Second)
	}

	return gin.H{
		"reward": gin.H{
			"userCosmeticItemId": "0", // V1 §7.2.4h 节点 7 阶段占位
			"cosmeticItemId":     strconv.FormatUint(out.Reward.CosmeticItemID, 10),
			"name":               out.Reward.Name,
			"slot":               out.Reward.Slot,
			"rarity":             out.Reward.Rarity,
			"assetUrl":           out.Reward.AssetURL,
			"iconUrl":            out.Reward.IconURL,
		},
		"stepAccount": gin.H{
			"totalSteps":     out.StepAccount.TotalSteps,
			"availableSteps": out.StepAccount.AvailableSteps,
			"consumedSteps":  out.StepAccount.ConsumedSteps,
		},
		"nextChest": gin.H{
			"id":               strconv.FormatUint(out.NextChest.ID, 10),
			"status":           nextChestStatus,
			"unlockAt":         out.NextChest.UnlockAt.Format(time.RFC3339),
			"openCostSteps":    out.NextChest.OpenCostSteps,
			"remainingSeconds": remainingSeconds,
		},
	}
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
