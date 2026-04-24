package metrics

import (
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// cat_api_requests_total：HTTP 请求总数（按 api_path / method / code 维度分桶）
// cat_api_request_duration_seconds：HTTP 请求延迟分布（按 api_path / method 维度分桶）
//
// 标签基数控制：api_path 用 Gin c.FullPath()（模式化，≈ 30 个路由），**禁止**用 raw URL
// 以免 /rooms/123456 / /rooms/234567 ... 各一个 label 爆炸。
var (
	httpRequestsTotal = promauto.With(Registry).NewCounterVec(
		prometheus.CounterOpts{
			Name: "cat_api_requests_total",
			Help: "Total HTTP requests served, partitioned by api_path, method, and response code.",
		},
		[]string{"api_path", "method", "code"},
	)
	httpRequestDuration = promauto.With(Registry).NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "cat_api_request_duration_seconds",
			Help:    "HTTP request duration in seconds, partitioned by api_path and method.",
			Buckets: []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5},
		},
		[]string{"api_path", "method"},
	)
)

// ObserveHTTP 给一条已完成的 HTTP 请求记录 metric。
// 由 logging 中间件在 c.Next() 之后调用（确保读到 final status）。
//
// apiPath 推荐传 Gin c.FullPath()；空串（未命中路由）会被替换为 "UNKNOWN"，
// 保留"未命中路由"这种信号但避免空 label value。
func ObserveHTTP(apiPath, method string, statusCode int, latency time.Duration) {
	if apiPath == "" {
		apiPath = "UNKNOWN"
	}
	code := strconv.Itoa(statusCode)
	httpRequestsTotal.WithLabelValues(apiPath, method, code).Inc()
	httpRequestDuration.WithLabelValues(apiPath, method).Observe(latency.Seconds())
}
