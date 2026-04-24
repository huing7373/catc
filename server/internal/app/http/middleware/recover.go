package middleware

import (
	stderrors "errors"
	"fmt"
	"log/slog"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/infra/logger"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
)

// Recovery 中间件：defer + recover 捕获 handler 链里的 panic，把 panic 值
// wrap 为 *AppError 推到 c.Errors，由外层 ErrorMappingMiddleware 写 envelope。
//
// # 挂载顺序要求
//
//	RequestID → Logging → ErrorMappingMiddleware → Recovery → handler
//
// 即 **ErrorMappingMiddleware 必须外层于 Recovery**。理由：
//   - panic 在 handler 抛出 → unwind 经过 Recovery 的 defer → recover() 捕获
//   - Recovery 的 defer：打 panic 日志 + c.Error(apperror.Wrap(...)) +
//     c.AbortWithStatus(500)
//   - Recovery 中间件正常返回 → 控制权回到 ErrorMappingMiddleware 的
//     after-c.Next() 代码 → 后者扫 c.Errors 写 envelope
//
// 反过来把 ErrorMappingMiddleware 放 Recovery 内层，panic 会让 ErrorMappingMiddleware
// 的 after-c.Next() 代码被 unwind 跳过，envelope 永远写不出来。
//
// # 与上游 story 的契约
//
// Story 1.3 时 Recovery 直接用 `response.Error(c, 500, 1009, "服务繁忙")`
// 写 envelope；Story 1.8（本次）按 NFR18 三层映射约定，把"写 envelope"
// 的责任收敛到 ErrorMappingMiddleware，本中间件只负责"捕获 panic + 把
// panic 值 wrap 成 ErrServiceBusy(*AppError) 推到 c.Errors + c.Abort"。
//
// **不**调 AbortWithStatus(500)：那会触发 c.Writer.WriteHeaderNow()，让
// Writer.Written() 返回 true，导致下游 ErrorMappingMiddleware 误判"响应
// 已写过"而跳过 envelope。HTTP status 由 ErrorMappingMiddleware 根据
// AppError code（ErrServiceBusy → 500，业务码 → 200）统一决策。
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
			_ = c.Error(apperror.Wrap(panicAsErr(rec), apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
			c.Abort()
		}()
		c.Next()
	}
}

// panicAsErr 把 recover() 返回的 any 转成 error，给 apperror.Wrap 用。
//
// 三种典型 panic 值：
//   - error 类型：原样返回（保留底层错误链，errors.Is 仍可穿透）
//   - string 类型：用 stderrors.New 包成 error
//   - 其他类型（int / struct / nil 等）：用 fmt.Errorf("panic: %v", rec) 兜底
func panicAsErr(rec any) error {
	switch v := rec.(type) {
	case error:
		return v
	case string:
		return stderrors.New(v)
	default:
		return fmt.Errorf("panic: %v", rec)
	}
}
