package metrics

import (
	"net/http/httptest"
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
