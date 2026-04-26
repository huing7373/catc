package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/middleware"
	"github.com/huing/cat/server/internal/infra/config"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
)

// newRateLimitRouter 构造一个挂上 ErrorMappingMiddleware + RateLimit 的 router。
func newRateLimitRouter(t *testing.T, cfg config.RateLimitConfig, extractor middleware.KeyExtractor) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorMappingMiddleware())
	r.Use(middleware.RateLimit(cfg, extractor))
	r.GET("/test", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	return r
}

// sendFromIP 发请求并返回响应。req.RemoteAddr 决定 c.ClientIP() 的值。
func sendFromIP(t *testing.T, r *gin.Engine, ip string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = ip + ":12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func parseEnvCode(t *testing.T, body []byte) int {
	t.Helper()
	var env struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("body not envelope JSON: %v; raw=%s", err, body)
	}
	return env.Code
}

// AC5.6 rate_limit happy: 1 分钟内 60 次内 → 通过（epics.md 行 1047）
func TestRateLimit_60RequestsIn1Minute_Pass(t *testing.T) {
	cfg := config.RateLimitConfig{PerKeyPerMin: 60, BurstSize: 60, BucketsLimit: 100}
	r := newRateLimitRouter(t, cfg, middleware.RateLimitByIP)

	for i := 0; i < 60; i++ {
		w := sendFromIP(t, r, "1.2.3.4")
		if w.Code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200; body=%s", i+1, w.Code, w.Body.String())
		}
	}
}

// AC5.7 rate_limit edge: 1 分钟内第 61 次 → 1005（epics.md 行 1048）
func TestRateLimit_61stRequestIn1Minute_Returns1005(t *testing.T) {
	cfg := config.RateLimitConfig{PerKeyPerMin: 60, BurstSize: 60, BucketsLimit: 100}
	r := newRateLimitRouter(t, cfg, middleware.RateLimitByIP)

	for i := 0; i < 60; i++ {
		w := sendFromIP(t, r, "1.2.3.4")
		if w.Code != http.StatusOK {
			t.Fatalf("burst request %d failed: %d", i+1, w.Code)
		}
	}
	// 第 61 次：拒
	w := sendFromIP(t, r, "1.2.3.4")
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (V1 §2.4: 业务码与 HTTP status 正交)", w.Code)
	}
	if got := parseEnvCode(t, w.Body.Bytes()); got != apperror.ErrTooManyRequests {
		t.Errorf("envelope.code = %d, want 1005", got)
	}
}

// AC5.8 rate_limit edge: 跨分钟边界 → token 平滑回填（epics.md 行 1049 兼容性见 Dev Notes）
//
// 60 次耗光 → 第 61 次拒 → time.Sleep(1.1s) 让 token 回填 ≥ 1 → 第 62 次通过。
// 这是 token bucket 的"持续平滑回填"语义；与 fixed-window 的"分钟边界 hard
// reset"在外部观察上等价（"耗光后等待时间过去再次可用"）。
func TestRateLimit_CrossMinuteBoundary_ResetsTokens(t *testing.T) {
	t.Parallel()
	cfg := config.RateLimitConfig{PerKeyPerMin: 60, BurstSize: 60, BucketsLimit: 100}
	r := newRateLimitRouter(t, cfg, middleware.RateLimitByIP)

	// burst 60 次耗光
	for i := 0; i < 60; i++ {
		w := sendFromIP(t, r, "9.9.9.9")
		if w.Code != http.StatusOK {
			t.Fatalf("burst request %d failed: %d", i+1, w.Code)
		}
	}
	// 第 61 次：拒（验证耗光路径）
	w := sendFromIP(t, r, "9.9.9.9")
	if got := parseEnvCode(t, w.Body.Bytes()); got != apperror.ErrTooManyRequests {
		t.Fatalf("expected 1005 after burst, got code=%d", got)
	}

	// 跨秒边界等待 token 回填（PerKeyPerMin=60 → 每秒回填 1 个）
	time.Sleep(1100 * time.Millisecond)

	// 第 62 次：通过
	w = sendFromIP(t, r, "9.9.9.9")
	if w.Code != http.StatusOK {
		t.Errorf("after sleep, status = %d, want 200", w.Code)
	}
	if got := parseEnvCode(t, w.Body.Bytes()); got != 0 {
		t.Errorf("after sleep, envelope.code = %d, want 0 (success)", got)
	}
}

// AC5.9 rate_limit edge: 不同 key 隔离 → IP A 满了不影响 IP B
func TestRateLimit_DifferentKeysIsolated(t *testing.T) {
	cfg := config.RateLimitConfig{PerKeyPerMin: 60, BurstSize: 60, BucketsLimit: 100}
	r := newRateLimitRouter(t, cfg, middleware.RateLimitByIP)

	// IP A 60 次
	for i := 0; i < 60; i++ {
		w := sendFromIP(t, r, "1.1.1.1")
		if w.Code != http.StatusOK {
			t.Fatalf("IP A burst %d failed: %d", i+1, w.Code)
		}
	}
	// IP A 第 61 次：拒
	w := sendFromIP(t, r, "1.1.1.1")
	if got := parseEnvCode(t, w.Body.Bytes()); got != apperror.ErrTooManyRequests {
		t.Fatalf("IP A 61st should be 1005, got %d", got)
	}

	// IP B 第 1 次：通过（与 IP A 桶隔离）
	w = sendFromIP(t, r, "2.2.2.2")
	if w.Code != http.StatusOK {
		t.Errorf("IP B first request status = %d, want 200", w.Code)
	}
	if got := parseEnvCode(t, w.Body.Bytes()); got != 0 {
		t.Errorf("IP B first request code = %d, want 0", got)
	}
}

// AC5.10 rate_limit edge: BucketsLimit 触发降级 bucket 防 IP 洪泛 OOM
//
// 配置 BucketsLimit=2，每 key BurstSize=2：
//   - IP1, IP2 创建独立 bucket（各 burst 2）
//   - IP3 触发降级（与 future 新 IP 共享 overflow limiter 同样 burst 2）
//   - IP4 也走降级 bucket → 与 IP3 共享 bucket → 联合 burst 2 共享耗光后第 3 次拒
func TestRateLimit_OverflowBuckets_FallsBackToSharedLimiter(t *testing.T) {
	cfg := config.RateLimitConfig{PerKeyPerMin: 60, BurstSize: 2, BucketsLimit: 2}
	r := newRateLimitRouter(t, cfg, middleware.RateLimitByIP)

	// IP1 用 1 个 token（独立 bucket，剩 1）
	w := sendFromIP(t, r, "10.0.0.1")
	if w.Code != http.StatusOK {
		t.Fatalf("IP1 first failed: %d", w.Code)
	}
	// IP2 用 1 个 token（独立 bucket，剩 1）
	w = sendFromIP(t, r, "10.0.0.2")
	if w.Code != http.StatusOK {
		t.Fatalf("IP2 first failed: %d", w.Code)
	}
	// 此时 buckets count = 2，达上限。后续 IP 都走 overflow（共享 burst=2）。
	// IP3 用 1 个 token（共享 bucket 剩 1）
	w = sendFromIP(t, r, "10.0.0.3")
	if w.Code != http.StatusOK {
		t.Fatalf("IP3 (overflow) first failed: %d", w.Code)
	}
	// IP4 用 1 个 token（共享 bucket 剩 0）
	w = sendFromIP(t, r, "10.0.0.4")
	if w.Code != http.StatusOK {
		t.Fatalf("IP4 (overflow) first failed: %d", w.Code)
	}
	// IP5 第 1 次（共享 bucket 已耗光）→ 1005
	w = sendFromIP(t, r, "10.0.0.5")
	if got := parseEnvCode(t, w.Body.Bytes()); got != apperror.ErrTooManyRequests {
		t.Errorf("IP5 (overflow shared depleted) code = %d, want 1005", got)
	}

	// IP1 仍可使用自己 bucket 第 2 个 token（验证独立桶仍生效）
	w = sendFromIP(t, r, "10.0.0.1")
	if w.Code != http.StatusOK {
		t.Errorf("IP1 second token (independent bucket) failed: %d", w.Code)
	}
}

// 防御性 fail-fast：extractor nil → panic（启动期暴露）
func TestRateLimit_PanicsOnNilExtractor(t *testing.T) {
	cfg := config.RateLimitConfig{PerKeyPerMin: 60, BurstSize: 60, BucketsLimit: 100}
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on nil extractor")
		}
	}()
	_ = middleware.RateLimit(cfg, nil)
}

// 防御性 fail-fast：PerKeyPerMin <= 0 → panic
func TestRateLimit_PanicsOnInvalidPerKeyPerMin(t *testing.T) {
	cfg := config.RateLimitConfig{PerKeyPerMin: 0, BurstSize: 60, BucketsLimit: 100}
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("expected panic on PerKeyPerMin = 0")
		}
	}()
	_ = middleware.RateLimit(cfg, middleware.RateLimitByIP)
}

// RateLimitByUserID extractor: 优先用 UserIDKey，缺失时 fallback IP
func TestRateLimitByUserID_PrefersUserID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// 模拟 Auth 中间件：注入 UserIDKey
	r.Use(func(c *gin.Context) {
		c.Set(middleware.UserIDKey, uint64(7777))
		c.Next()
	})
	captured := ""
	r.Use(func(c *gin.Context) {
		captured = middleware.RateLimitByUserID(c)
		c.Next()
	})
	r.GET("/x", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "1.2.3.4:5"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if captured != "user:7777" {
		t.Errorf("RateLimitByUserID = %q, want %q", captured, "user:7777")
	}
}

// RateLimitByUserID 无 UserIDKey 时 fallback IP
func TestRateLimitByUserID_FallbackToIP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	captured := ""
	r.Use(func(c *gin.Context) {
		captured = middleware.RateLimitByUserID(c)
		c.Next()
	})
	r.GET("/x", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{}) })
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.RemoteAddr = "9.8.7.6:1"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if captured == "" || captured[:3] != "ip:" {
		t.Errorf("RateLimitByUserID fallback = %q, want prefix 'ip:'", captured)
	}
}
