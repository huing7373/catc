package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry 是本进程所有 prom collector 的注册中心。
// 显式用独立 Registry，避免被 Go runtime default collector 污染输出；
// domain 层 (Epic 20 / Epic 32) 注册自己的 metric 时用 `metrics.Registry.MustRegister(...)` 或 `promauto.With(Registry).NewXxx(...)`。
var Registry = prometheus.NewRegistry()

// Handler 返回绑定本 Registry 的 prometheus HTTP handler，供 Gin 挂 `/metrics` 时用：
//
//	r.GET("/metrics", gin.WrapH(metrics.Handler()))
func Handler() http.Handler {
	return promhttp.HandlerFor(Registry, promhttp.HandlerOpts{})
}
