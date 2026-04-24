package middleware

import (
	"log/slog"
	"net/http"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/infra/logger"
	"github.com/huing/cat/server/internal/pkg/response"
)

// panicFallbackCode 是 panic 被本中间件兜住时返回给 client 的业务 code。
// Story 1.8 引入 AppError + ErrorMappingMiddleware 后，此常量会被替换为
// errors.ErrServiceBusy 之类的枚举。现在先用 literal，避免提前引入 AppError 框架。
const panicFallbackCode = 1009

// Recovery 中间件：defer + recover 捕获 handler 链里的 panic，转成 500 JSON 响应。
//
// 挂载顺序要求：**必须在 Logging 之后、handler 之前**。
// 这样 panic 路径是：handler panic → 本 defer 捕获 → response.Error 写 500 → 正常返回
// → Logging 中间件的 "after c.Next()" 代码读到 status=500 并打日志。
// 反之把 Logging 放在 Recovery 之后，logging 的后续代码会被 panic unwind 跳过，
// 该请求会在日志里彻底消失。
func Recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			stack := debug.Stack()
			reqLogger := logger.FromContext(c.Request.Context())
			reqLogger.ErrorContext(c.Request.Context(), "handler panic",
				slog.Any("panic", rec),
				slog.String("stack", string(stack)),
			)
			response.Error(c, http.StatusInternalServerError, panicFallbackCode, "服务繁忙")
			c.Abort()
		}()
		c.Next()
	}
}
