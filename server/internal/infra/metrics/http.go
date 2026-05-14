package metrics

import (
	"strconv"
	"strings"
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

// devPathPrefix 标记 dev / stub / preview 端点路由前缀。命中的请求**完全 skip metrics**，
// 不计入 counter / histogram 任一维度。
//
// 设计动机（Story 20.8 r3 lesson）：
//
// dev 端点（/dev/grant-steps / /dev/force-unlock-chest / /dev/grant-cosmetic-batch / 未来新增
// dev 端点等）**本就不该影响生产监控**。它们：
//   - 只在 BUILD_DEV / -tags devtools 启用，生产构建物理不可达
//   - stub 阶段经常返 HTTP 501（ErrNotImplemented），按 5xx 维度被 dashboard 当 server error 误报
//   - e2e / demo 流量与生产业务流量混在同一 counter 会污染 latency / error rate 计算
//
// 把 dev 路径在 metrics 层一刀切排除，比"5xx-alert 告警规则里手工排除每个 dev 路径"更根因
// （白名单 vs 黑名单，前者 fail-closed）—— 加 dev 端点的人不需要同步改告警规则。
const devPathPrefix = "/dev/"

// isDevPath 判定 api_path 是否为 dev 端点（命中即跳 metrics）。
//
// 严格按 prefix 匹配 "/dev/"（带斜杠），不匹配 "/dev"（无斜杠尾）或 "/devops/..."。
// 用 Gin c.FullPath() 传进来的 pattern path（如 "/dev/grant-cosmetic-batch"）。
// "" 或 "UNKNOWN" 不算 dev 路径（NoRoute 兜底必须可观测）。
func isDevPath(apiPath string) bool {
	return strings.HasPrefix(apiPath, devPathPrefix)
}

// ObserveHTTP 给一条已完成的 HTTP 请求记录 metric。
// 由 logging 中间件在 c.Next() 之后调用（确保读到 final status）。
//
// apiPath 推荐传 Gin c.FullPath()；空串（未命中路由）会被替换为 "UNKNOWN"，
// 保留"未命中路由"这种信号但避免空 label value。
//
// **dev 端点豁免**：apiPath 以 "/dev/" 开头时**完全 skip**（不调 counter / histogram）。
// 见 devPathPrefix 注释中的设计动机。
func ObserveHTTP(apiPath, method string, statusCode int, latency time.Duration) {
	if isDevPath(apiPath) {
		return
	}
	if apiPath == "" {
		apiPath = "UNKNOWN"
	}
	code := strconv.Itoa(statusCode)
	httpRequestsTotal.WithLabelValues(apiPath, method, code).Inc()
	httpRequestDuration.WithLabelValues(apiPath, method).Observe(latency.Seconds())
}
