package bootstrap

import (
	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/handler"
)

// NewRouter builds the Gin engine with no middleware.
// Story 1.3 will wrap request_id / recover / logging middleware here.
// Story 1.4 will add GET /version.
// Story 1.6 will register the /dev/* group behind BUILD_DEV flag.
func NewRouter() *gin.Engine {
	r := gin.New()
	r.GET("/ping", handler.PingHandler)
	return r
}
