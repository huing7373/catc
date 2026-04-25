package middleware

import (
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/infra/logger"
	"github.com/huing/cat/server/internal/infra/metrics"
)

// Logging 中间件：每个请求末尾输出一行结构化日志 + 更新 2 个 HTTP metric。
//
// 挂载顺序要求：**必须在 RequestID 之后、ErrorMappingMiddleware 之前**
// （ErrorMappingMiddleware 又必须外层于 Recovery，见 error_mapping.go 注释）。
//
// 日志字段（ADR-0001 §4 表）：
//   request_id / api_path / latency_ms       ← 必有（Story 1.3 落地）
//   method / status / client_ip              ← 必有（Story 1.3 落地）
//   error_code                               ← canonical envelope.code（Story 1.8 落地）
//   ctx_done                                 ← 请求被 cancel 时 true（Story 1.9 落地）
//   user_id / business_result                ← 留空（Epic 4 auth / service 层）
//
// **error_code 字段语义**：
//
// 该字段值取自 ErrorMappingMiddleware 通过 c.Keys 设置的 ResponseErrorCodeKey
// （即客户端实际看到的 envelope.code）。本中间件**不**自行扫描 c.Errors 推断
// error_code —— 那会与 ErrorMappingMiddleware 的决策不一致：
//   - 非 AppError 路径：c.Errors[0] 是 io.EOF 这类原生 error，
//     ErrorMappingMiddleware wrap 成 1009 envelope；扫 c.Errors 用
//     apperror.As 会拿不到 code → 日志缺 error_code，与响应不符
//   - double-write 路径：handler 先写 success 又 c.Error，
//     ErrorMappingMiddleware 保留 success 响应（不覆写、不设 key）；
//     扫 c.Errors 会误标 error_code → 日志声称业务错误而响应实际是成功
//
// ResponseErrorCodeKey 不存在 = success path 或 double-write path = 不写 error_code。
// 详见 ResponseErrorCodeKey 常量注释 + lessons/2026-04-24-middleware-canonical-decision-key.md。
//
// **ctx_done 字段语义**（ADR-0007 §4）：
//
// c.Next() 之后读 c.Request.Context().Err()：
//   - 非 nil（context.Canceled / context.DeadlineExceeded）→ 追加 ctx_done=true
//   - nil（请求正常完成）→ **省略字段**（与 error_code 成功路径"省略字段"惯例一致）
//
// 不区分 Canceled vs DeadlineExceeded：监控聚合 count(ctx_done=true) by api_path
// 信号足够。若未来需区分，扩 ctx_done_reason 字段（不破坏 bool 语义）。
//
// ctx.Err() 是 Gin ctx 的**原生状态**（不是上游中间件的推断结果），两个中间件
// 各自读不会漂移，**不需要**走 c.Keys canonical 广播（对比 error_code 必须走广播，
// 因为 ErrorMappingMiddleware wrap 了非 AppError）。见 ADR-0007 §4.5。
//
// 本中间件同时把 child logger（已 With request_id + api_path）塞进 ctx，
// 下游 handler / service 用 logger.FromContext(ctx) 继承，不需要手工传 request_id。
func Logging() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		rid, _ := c.Get(RequestIDKey)
		ridStr, _ := rid.(string)
		reqLogger := slog.Default().With(
			slog.String("request_id", ridStr),
			slog.String("api_path", c.FullPath()),
		)
		ctx := logger.NewContext(c.Request.Context(), reqLogger)
		c.Request = c.Request.WithContext(ctx)

		c.Next()

		latency := time.Since(start)
		status := c.Writer.Status()

		// 基础 6 字段（method/status/latency_ms/client_ip）
		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.Int("status", status),
			slog.Int64("latency_ms", latency.Milliseconds()),
			slog.String("client_ip", c.ClientIP()),
		}
		// error_code：从 ErrorMappingMiddleware 设置的 ResponseErrorCodeKey 读，
		// 而非自行扫 c.Errors（见本文件顶部 "error_code 字段语义" 注释）。
		// key 不存在 → 不追加 error_code（ADR-0001 §4 表："成功请求省略该字段"
		// + double-write 场景成功响应也省略）。
		if v, exists := c.Get(ResponseErrorCodeKey); exists {
			if code, ok := v.(int); ok {
				attrs = append(attrs, slog.Int("error_code", code))
			}
		}
		// ctx_done：请求被 cancel（client 断开 / deadline exceeded）→ 追加 ctx_done=true；
		// ctx 正常 → 省略字段（缺省即 false，见本文件顶部 "ctx_done 字段语义" 注释）。
		// 读取时机必须是 c.Next() 之后 —— handler 执行过程中 ctx 状态不稳定。
		if err := c.Request.Context().Err(); err != nil {
			attrs = append(attrs, slog.Bool("ctx_done", true))
		}
		reqLogger.LogAttrs(ctx, slog.LevelInfo, "http_request", attrs...)

		metrics.ObserveHTTP(c.FullPath(), c.Request.Method, status, latency)
	}
}
