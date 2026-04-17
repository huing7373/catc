package middleware

import (
	"fmt"
	"runtime/debug"

	"github.com/gin-gonic/gin"
	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/logx"
)

func Recover() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				logx.Ctx(c.Request.Context()).Error().
					Interface("panic", r).
					Str("stack", string(debug.Stack())).
					Msg("panic recovered")

				if !c.Writer.Written() {
					dto.RespondAppError(c, dto.ErrInternalError.WithCause(fmt.Errorf("panic: %v", r)))
				} else {
					c.Abort()
				}
			}
		}()
		c.Next()
	}
}
