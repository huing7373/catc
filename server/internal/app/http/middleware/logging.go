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
// 挂载顺序要求：**必须在 RequestID 之后、Recovery 之前**（见 recover.go 注释）。
//
// 日志字段（本 story 阶段能输出的 6 字段 + 额外 3 个）：
//   request_id / api_path / latency_ms       ← 必有
//   method / status / client_ip              ← 必有（epics.md 1.3 AC）
//   user_id / business_result / error_code   ← 留空（Epic 4 / service 层 / Story 1.8）
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
		reqLogger.InfoContext(ctx, "http_request",
			slog.String("method", c.Request.Method),
			slog.Int("status", status),
			slog.Int64("latency_ms", latency.Milliseconds()),
			slog.String("client_ip", c.ClientIP()),
		)

		metrics.ObserveHTTP(c.FullPath(), c.Request.Method, status, latency)
	}
}
