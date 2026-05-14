package metrics

import (
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestObserveHTTP_CounterIncrementsAndHistogramObserves(t *testing.T) {
	// 记录初值 —— 其它测试也用同一套全局 Counter/Histogram，不能假定从 0 开始。
	before := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("/ping", "GET", "200"))

	ObserveHTTP("/ping", "GET", 200, 10*time.Millisecond)
	ObserveHTTP("/ping", "GET", 200, 20*time.Millisecond)
	ObserveHTTP("/ping", "GET", 200, 30*time.Millisecond)

	after := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("/ping", "GET", "200"))
	if delta := after - before; delta != 3 {
		t.Errorf("counter delta = %v, want 3", delta)
	}

	// histogram 的 count 通过 CollectAndCount 难以直接拿到精确增量，用 metric 文本抓样本数
	// 这里换成断言 `/metrics` 抓取含该系列即可 —— 详细数量断言交给集成测试
	got := metricsTextSnapshot(t)
	if !strings.Contains(got, `cat_api_request_duration_seconds_sum{api_path="/ping",method="GET"}`) {
		t.Errorf("expected histogram sample in /metrics output; got:\n%s", got)
	}
}

func TestObserveHTTP_EmptyApiPathBecomesUNKNOWN(t *testing.T) {
	before := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("UNKNOWN", "GET", "404"))
	ObserveHTTP("", "GET", 404, 1*time.Millisecond)
	after := testutil.ToFloat64(httpRequestsTotal.WithLabelValues("UNKNOWN", "GET", "404"))
	if delta := after - before; delta != 1 {
		t.Errorf("UNKNOWN counter delta = %v, want 1", delta)
	}
}

// TestObserveHTTP_DevEndpoint_NotCounted 验 /dev/* 路径完全 skip metrics，
// counter / histogram 均不增长。覆盖 Story 20.8 r3 lesson "dev 端点 metrics 豁免"。
//
// 关键 case：/dev/grant-cosmetic-batch + status=501（stub 阶段 ErrNotImplemented）—— 这是 r3 推动
// 设计的直接动机：501 不能计入 cat_api_requests_total{code="501"}，否则 5xx-based dashboard 告警
// 会把每次 dev stub 调用当成 server error 误报。
func TestObserveHTTP_DevEndpoint_NotCounted(t *testing.T) {
	devPaths := []struct {
		path   string
		method string
		status int
	}{
		{"/dev/grant-steps", "POST", 200},             // 7.5 happy path
		{"/dev/force-unlock-chest", "POST", 200},      // 20.7 happy path
		{"/dev/grant-cosmetic-batch", "POST", 501},    // 20.8 r3 核心 case：stub 501 不污染 5xx
		{"/dev/ping-dev", "GET", 200},                 // 1.6 框架自带
	}
	for _, dp := range devPaths {
		labels := []string{dp.path, dp.method, strconv.Itoa(dp.status)}
		before := testutil.ToFloat64(httpRequestsTotal.WithLabelValues(labels...))
		ObserveHTTP(dp.path, dp.method, dp.status, 5*time.Millisecond)
		after := testutil.ToFloat64(httpRequestsTotal.WithLabelValues(labels...))
		if delta := after - before; delta != 0 {
			t.Errorf("%s %s %d: counter delta = %v, want 0 (dev endpoint must be skipped)",
				dp.method, dp.path, dp.status, delta)
		}
	}

	// histogram 维度也必须 skip：抓 /metrics 文本断言 dev 路径不出现在 _sum / _count 序列里
	got := metricsTextSnapshot(t)
	for _, dp := range devPaths {
		sumSeries := `cat_api_request_duration_seconds_sum{api_path="` + dp.path + `"`
		countSeries := `cat_api_request_duration_seconds_count{api_path="` + dp.path + `"`
		if strings.Contains(got, sumSeries) || strings.Contains(got, countSeries) {
			t.Errorf("dev path %s leaked into histogram metrics output", dp.path)
		}
	}
}

// TestObserveHTTP_DevPrefixDiscipline 验严格 "/dev/" 前缀匹配（防 over-match）：
//   - "/dev/xxx"        → skip
//   - "/dev"            → 计数（无尾斜杠，不算 dev 路由组）
//   - "/devops/healthz" → 计数（同名前缀但非 dev 路由组）
//   - "/api/dev/foo"    → 计数（dev 不在路径首位）
func TestObserveHTTP_DevPrefixDiscipline(t *testing.T) {
	cases := []struct {
		path    string
		wantInc float64
	}{
		{"/dev/anything", 0},
		{"/dev", 1},
		{"/devops/healthz", 1},
		{"/api/dev/foo", 1},
	}
	for _, c := range cases {
		before := testutil.ToFloat64(httpRequestsTotal.WithLabelValues(c.path, "GET", "200"))
		ObserveHTTP(c.path, "GET", 200, 1*time.Millisecond)
		after := testutil.ToFloat64(httpRequestsTotal.WithLabelValues(c.path, "GET", "200"))
		if delta := after - before; delta != c.wantInc {
			t.Errorf("path %s: counter delta = %v, want %v", c.path, delta, c.wantInc)
		}
	}
}

// TestIsDevPath 单独验内部 helper 的语义。
func TestIsDevPath(t *testing.T) {
	cases := []struct {
		path string
		want bool
	}{
		{"/dev/", true},
		{"/dev/grant-steps", true},
		{"/dev/force-unlock-chest", true},
		{"/dev/grant-cosmetic-batch", true},
		{"/dev", false},
		{"/devops/healthz", false},
		{"/api/dev/foo", false},
		{"", false},
		{"UNKNOWN", false},
		{"/ping", false},
	}
	for _, c := range cases {
		if got := isDevPath(c.path); got != c.want {
			t.Errorf("isDevPath(%q) = %v, want %v", c.path, got, c.want)
		}
	}
}

// metricsTextSnapshot 抓取 prom 文本格式，便于 substring 断言。
func metricsTextSnapshot(t *testing.T) string {
	t.Helper()
	h := Handler()
	req := httptest.NewRequest("GET", "/metrics", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != 200 {
		t.Fatalf("metrics handler status = %d, want 200", w.Code)
	}
	return w.Body.String()
}
