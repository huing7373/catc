package bootstrap

import (
	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/handler"
	"github.com/huing/cat/server/internal/app/http/middleware"
	"github.com/huing/cat/server/internal/infra/metrics"
)

// NewRouter 构造挂好三件套中间件的 Gin engine。
//
// 中间件顺序严格：RequestID → Logging → Recovery → handler。
// 反过来会导致 panic 请求在日志里消失（详情见 middleware/recover.go 顶部注释）。
//
// 运维端点（不走 /api/v1 前缀，不走业务 auth）：
//   - GET /ping    — liveness 探活
//   - GET /version — 构建信息（commit / builtAt，编译期 ldflags 注入）
//   - GET /metrics — Prometheus scrape 端点
//
// Future: Story 1.6 registers /dev/* group behind BUILD_DEV flag.
// TODO(Epic-36): /metrics 上线前需要加 auth / 独立端口。
func NewRouter() *gin.Engine {
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Logging())
	r.Use(middleware.Recovery())

	r.GET("/ping", handler.PingHandler)
	r.GET("/version", handler.VersionHandler)
	r.GET("/metrics", gin.WrapH(metrics.Handler()))

	return r
}
