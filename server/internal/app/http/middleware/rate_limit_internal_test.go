package middleware

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/infra/config"
)

// TestRateLimit_XFFSpoofing_DoesNotBypass 验证攻击者循环伪造 X-Forwarded-For
// **不能**绕过 RateLimitByIP 的 60/min 限制。
//
// 背景（fix-review round 1 [P1]）：旧实现用 c.ClientIP()，Gin 默认信任 XFF
// header → 攻击者每次发不同 XFF 都被认为是新 IP key，60/min 完全绕过。
//
// 新实现用 c.RemoteIP() —— 直接取 Request.RemoteAddr 的 host 部分，跳过 XFF
// 解析。无论 XFF 怎么伪造，同一 RemoteAddr 的请求都计入同一 key。
//
// 测试构造：固定 RemoteAddr=1.2.3.4 的 N 个请求，每个 XFF 不同（伪造 N 个
// "假" 来源）。若实现仍信任 XFF → 每次都是新 key → 全部通过；若不信任 XFF
// → 共享一个 key → 第 PerKeyPerMin+1 次起被拒。
func TestRateLimit_XFFSpoofing_DoesNotBypass(t *testing.T) {
	gin.SetMode(gin.TestMode)
	cfg := config.RateLimitConfig{PerKeyPerMin: 5, BurstSize: 5, BucketsLimit: 100}
	r := gin.New()
	r.Use(ErrorMappingMiddleware())
	r.Use(RateLimit(cfg, RateLimitByIP))
	r.GET("/test", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	// 前 5 次（burst 内）：固定 RemoteAddr，XFF 每次不同 → 应该都通过（命中
	// 同一 key，burst 5 没耗光）。
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "203.0.113.42:55555"
		req.Header.Set("X-Forwarded-For", "10.0.0."+itoa(i))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("burst request %d: status=%d, body=%s", i+1, w.Code, w.Body.String())
		}
	}

	// 第 6 次：仍同 RemoteAddr，XFF 再变 → 必须被拒（HTTP 200 + envelope 1005）。
	// 旧实现（信任 XFF）会让这一次又变成新 key 通过 → 测试 fail。
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "203.0.113.42:55555"
	req.Header.Set("X-Forwarded-For", "10.0.0.99")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("6th request status=%d, want 200 (envelope code carries rate-limit signal)", w.Code)
	}
	body := w.Body.String()
	// 简单子串断言（envelope 形如 {"code":1005,...}）
	if !contains(body, `"code":1005`) {
		t.Errorf("6th request body should carry envelope.code=1005 (rate-limited), got: %s", body)
	}
}

// TestRateLimit_ConcurrentFlood_BucketsBounded 验证 BucketsLimit 在并发洪泛
// 下真实 bounded：N goroutine 各用不同 key 同时打第一次请求时，buckets 中独立
// 桶数 ≤ BucketsLimit，超出部分落 overflow shared bucket。
//
// 背景（fix-review round 1 [P2]）：旧实现 `if count.Load() < limit { LoadOrStore;
// count.Add(1) }` 不原子 —— 多 goroutine 都先看到 count<limit，各自创建独立
// limiter，最后 map 大小可远超 limit。
//
// 新实现用 CAS 预占槽位（CompareAndSwap 保证 count 增量原子），LoadOrStore
// 撞 key 已存在则撤销槽位。无论 N 多大、并发多激烈，独立桶数恒 ≤ limit。
//
// 同时这是 -race 友好的测试 —— 多 goroutine 并发触发 count 路径，配合
// `go test -race -count=10 ./...` 可暴露任何残留 race。
func TestRateLimit_ConcurrentFlood_BucketsBounded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	const (
		bucketsLimit = 50
		distinctKeys = 500 // 远超 limit，触发 overflow 路径
	)
	cfg := config.RateLimitConfig{PerKeyPerMin: 60, BurstSize: 1, BucketsLimit: bucketsLimit}

	// 用 newRateLimit（同包测试可见）拿到 count atomic 句柄
	keyFromHeader := func(c *gin.Context) string {
		return "k:" + c.GetHeader("X-Test-Key")
	}
	handler, count := newRateLimit(cfg, keyFromHeader)

	r := gin.New()
	r.Use(ErrorMappingMiddleware())
	r.Use(handler)
	r.GET("/test", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })

	var wg sync.WaitGroup
	wg.Add(distinctKeys)
	for i := 0; i < distinctKeys; i++ {
		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("X-Test-Key", itoa(idx))
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
		}(i)
	}
	wg.Wait()

	// 关键断言：buckets 独立桶数不能超过 BucketsLimit
	got := count.Load()
	if got > int64(bucketsLimit) {
		t.Errorf("buckets count = %d, must not exceed BucketsLimit (%d) under concurrent flood",
			got, bucketsLimit)
	}
	// 反向 sanity：洪泛下 count 应至少接近 limit（除非 limit 配置错）
	if got == 0 {
		t.Errorf("buckets count = 0, expected > 0 under flood (something broke key path)")
	}
}

// itoa 是简化版 strconv.Itoa（避免引入额外 import 在 _test.go 里）。
func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	neg := false
	if i < 0 {
		neg = true
		i = -i
	}
	var buf [20]byte
	pos := len(buf)
	for i > 0 {
		pos--
		buf[pos] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}

// contains 简单子串包含（避免引入 strings 包让本文件 import 列表更小）。
func contains(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	if len(sub) > len(s) {
		return false
	}
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
