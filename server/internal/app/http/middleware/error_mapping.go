package middleware

import (
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/infra/logger"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
)

// ResponseErrorCodeKey 是 ErrorMappingMiddleware 与 Logging 之间的契约键：
// 写完 envelope 后，本中间件把它实际选定的 envelope.code 存到 c.Keys，
// Logging 中间件读取该 key 决定 http_request 日志中的 `error_code` 字段值。
//
// # 唯一权威生产者：ErrorMappingMiddleware
//
// 本项目钦定：**只有 ErrorMappingMiddleware 写 error envelope + Set 本 key**。
// 其它任何中间件 / handler 想报告业务错误，一律通过 `c.Error(apperror.New(...))` +
// `c.Abort()` 把错误推到 c.Errors，由 ErrorMappingMiddleware 统一翻译成 envelope。
//
// **禁止**直接调 `response.Error(...)` 绕开本管道 —— 即便随手 `c.Set(ResponseErrorCodeKey, ...)`
// 同步 canonical key 看似也行，但这会把 HTTP status 决策权分散到各中间件，让
// V1接口设计 §2.4 "业务码与 HTTP status 正交，仅 1009 走 500" 的集中决策失控。
// 见 docs/lessons/2026-04-24-error-envelope-single-producer.md。
//
// # 为什么需要本 key
//
// 让下游消费者（目前只有 Logging）一律读本 key 拿 canonical envelope.code，
// 不再各自从 c.Errors 原始状态推断 —— 否则会出现两类 bug：
//
//  1. 非 AppError 路径：handler `c.Error(io.EOF)` 时 c.Errors[0] 是 io.EOF，
//     ErrorMappingMiddleware wrap 成 1009 envelope；Logging 若仍扫 c.Errors 用
//     apperror.As → 拿不到 code → http_request 日志缺 error_code，与响应不符
//  2. double-write 路径：handler 先写 success 又 c.Error，ErrorMappingMiddleware
//     保留 success 响应（不覆写）；Logging 若仍扫 c.Errors → 误标 error_code，
//     日志声称业务错误而响应实际是成功
//
// 本 key **不存在** 表达两类语义之一：
//   - success path（c.Errors 为空）
//   - double-write path（ErrorMappingMiddleware 故意跳过 envelope 写入，
//     成功响应是客户端实际看到的，不应在日志声称错误）
//
// 见 docs/lessons/2026-04-24-middleware-canonical-decision-key.md +
// docs/lessons/2026-04-24-error-envelope-single-producer.md。
const ResponseErrorCodeKey = "response_error_code"

// ErrorMappingMiddleware 在 c.Next() 之后扫描 c.Errors，把错误统一翻译成
// V1接口设计 §2.4 envelope `{code, message, data, requestId}`。
//
// # 中间件挂载顺序（router.go）
//
//	RequestID → Logging → ErrorMappingMiddleware → Recovery → handler
//
// 关键：ErrorMappingMiddleware **必须外层于** Recovery —— 因为 panic 流程是
// "handler panic → Recovery defer recover() 抓住 → Recovery 调 c.Error(...) +
// c.AbortWithStatus(500) → Recovery 正常返回 → 控制权回到 ErrorMappingMiddleware
// 的 'after c.Next()' 代码 → 写 envelope"。如果把 ErrorMappingMiddleware 放
// Recovery 的内层（更靠近 handler），panic 会让本中间件的 after-c.Next() 代码
// 被 unwind 跳过，envelope 永远写不出来。
//
// # 行为
//
//  1. c.Next() 跑完后扫 c.Errors（Gin 错误队列，按 c.Error(err) 顺序追加）
//  2. 若 c.Errors 为空 → 假设 handler 已用 response.Success 写完响应，no-op
//  3. 若 c.Errors[0] 链上能 As 出 *AppError → 用其 Code/Message 写 envelope
//     （日志 WARN 级别，业务错误属正常路径）
//  4. 若 c.Errors[0] 链上 As 不出 *AppError → wrap 为 ErrServiceBusy(1009)，
//     写 envelope（日志 ERROR 级别，系统级问题应触发告警）
//  5. 若 c.Writer.Written() 已 true（handler 自己写过响应又调了 c.Error）→
//     跳过响应写入避免 double-write panic，但**仍**打 log（让 dev 能在日志里
//     诊断这条逻辑漏洞）
//
// # HTTP status 取舍
//
// 本中间件**统一决策** HTTP status，规则简单：
//   - AppError.Code == ErrServiceBusy（1009）→ HTTP 500：panic 兜底 + 非
//     AppError 兜底都归此码，属系统级问题，应触发 LB / 监控告警
//   - 其他业务码（任意非 1009 的 AppError）→ HTTP 200：业务码与 HTTP status
//     正交，客户端永远先看 envelope.code。V1接口设计 §2.4 钦定
//
// Recovery 中间件**故意不**自己设置 500 status（避免 WriteHeaderNow 让
// Writer.Written() 提前为 true），把 status 决策权完全留给本中间件。
func ErrorMappingMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		// 无错误 → handler 用 response.Success 已写完，本中间件 no-op
		if len(c.Errors) == 0 {
			return
		}

		// 取第一条 error；多条 c.Error 的场景目前业务无用例（应当 fast-fail return）
		firstErr := c.Errors[0].Err
		if firstErr == nil {
			// 防御性编码：c.Error(nil) 不应发生，但若出现也不能让本中间件 panic
			return
		}

		reqLogger := logger.FromContext(c.Request.Context())

		ae, ok := apperror.As(firstErr)
		if !ok {
			// 非 AppError 兜底：wrap 为 ErrServiceBusy
			ae = apperror.Wrap(firstErr, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}

		// status 决策：ErrServiceBusy → 500（系统级），其他 → 200（业务级）
		httpStatus := http.StatusOK
		logLevel := slog.LevelWarn
		if ae.Code == apperror.ErrServiceBusy {
			httpStatus = http.StatusInternalServerError
			logLevel = slog.LevelError
		}

		if c.Writer.Written() {
			// double-write：handler 已写响应又 c.Error —— dev bug，记日志但不再写 body。
			// **故意不** Set ResponseErrorCodeKey：成功响应是客户端实际看到的，
			// Logging 必须保持 http_request 日志与响应一致（不写 error_code）。
			reqLogger.LogAttrs(c.Request.Context(), logLevel, "error_mapping skipped: response already written",
				slog.Int("error_code", ae.Code),
				slog.String("error_msg", ae.Message),
				slog.String("cause", causeChain(ae)),
			)
			return
		}

		response.Error(c, httpStatus, ae.Code, ae.Message)
		// 把 canonical envelope.code 存到 c.Keys 供 Logging 读取。
		// 必须在 response.Error 之后调（响应已写成功才"声明" error_code），
		// 与 double-write 路径形成对立：写过响应才设 key，跳过响应则不设 key。
		c.Set(ResponseErrorCodeKey, ae.Code)
		reqLogger.LogAttrs(c.Request.Context(), logLevel, "error_mapping",
			slog.Int("error_code", ae.Code),
			slog.String("error_msg", ae.Message),
			slog.String("cause", causeChain(ae)),
		)
	}
}

// causeChain 把 AppError.Cause 链拼成一行字符串，用于日志诊断。
// 为 nil 时返回空串；嵌套多层时按 ": " 分隔。
func causeChain(ae *apperror.AppError) string {
	if ae.Cause == nil {
		return ""
	}
	return ae.Cause.Error()
}
