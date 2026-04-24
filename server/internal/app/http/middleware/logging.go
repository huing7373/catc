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
		reqLogger.LogAttrs(ctx, slog.LevelInfo, "http_request", attrs...)

		metrics.ObserveHTTP(c.FullPath(), c.Request.Method, status, latency)
	}
}
