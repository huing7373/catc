package examples

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/huing/cat/server/pkg/logx"
)

// ExampleHandler demonstrates structured logging in a Gin handler.
//
//	logx.Ctx(c.Request.Context()) returns a logger pre-populated with
//	requestId (from middleware.RequestID) and userId (from auth middleware).
func ExampleHandler(c *gin.Context) {
	ctx := c.Request.Context()

	logx.Ctx(ctx).Info().
		Str("action", "exampleAction").
		Str("param", c.Param("id")).
		Msg("processing request")

	// business logic ...

	logx.Ctx(ctx).Info().
		Str("action", "exampleAction").
		Msg("request completed")

	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
