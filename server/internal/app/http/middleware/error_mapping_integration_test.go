package middleware_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/bootstrap"
	"github.com/huing/cat/server/internal/app/http/middleware"
	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/pkg/auth"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
)

// TestErrorMapping_E2E_HandlerReturnsAppError 是 AC9 钦定的集成测试：
//
// 用 httptest.NewServer（真 HTTP roundtrip）+ bootstrap.NewRouter（真中间件链）
// + 一条临时挂入的 c.Error(AppError) handler，验证 envelope 端到端正确。
//
// 这条测试同时验证 "ErrorMappingMiddleware 已挂在 router.go 中间件链上" ——
// 如果 router.go 漏挂本中间件，response body 会是 Gin 默认的 200 空 body，
// envelope 解析会失败。
func TestErrorMapping_E2E_HandlerReturnsAppError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := bootstrap.NewRouter(bootstrap.Deps{})
	// 临时挂一条业务错误测试路由（生产 router 不含）
	r.GET("/test/apperror", func(c *gin.Context) {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "test"))
		c.Abort()
	})

	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/test/apperror")
	if err != nil {
		t.Fatalf("http.Get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200 (业务错误 + envelope code)", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var env struct {
		Code      int            `json:"code"`
		Message   string         `json:"message"`
		Data      map[string]any `json:"data"`
		RequestID string         `json:"requestId"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("body not envelope JSON: %v; raw=%s", err, body)
	}

	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d", env.Code, apperror.ErrInvalidParam)
	}
	if env.Message != "test" {
		t.Errorf("envelope.message = %q, want %q", env.Message, "test")
	}
	if env.Data == nil {
		t.Errorf("envelope.data 应是空对象 {}（V1 §2.4），不应为 JSON null")
	}
	if len(env.Data) != 0 {
		t.Errorf("envelope.data = %v, want empty {}", env.Data)
	}

	// requestId 应是 RequestID 中间件生成的 UUIDv4
	uuidV4 := regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidV4.MatchString(env.RequestID) {
		t.Errorf("envelope.requestId = %q, want UUIDv4 from RequestID middleware", env.RequestID)
	}

	// 响应 header 也应带 X-Request-Id 且与 body 一致
	headerRID := resp.Header.Get("X-Request-Id")
	if headerRID != env.RequestID {
		t.Errorf("X-Request-Id header = %q, body.requestId = %q; must match", headerRID, env.RequestID)
	}
}

// TestErrorMapping_E2E_NonAppError_Returns500Envelope 验证非 AppError 兜底路径
// 端到端：handler 推 stderrors.New → ErrorMappingMiddleware wrap 1009 → 500
// envelope。
func TestErrorMapping_E2E_NonAppError_Returns500Envelope(t *testing.T) {
	gin.SetMode(gin.TestMode)

	r := bootstrap.NewRouter(bootstrap.Deps{})
	r.GET("/test/raw-error", func(c *gin.Context) {
		_ = c.Error(io.EOF) // stdlib error，非 AppError
		c.Abort()
	})

	srv := httptest.NewServer(r)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/test/raw-error")
	if err != nil {
		t.Fatalf("http.Get: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (非 AppError 兜底为系统级)", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("body not envelope JSON: %v; raw=%s", err, body)
	}
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d", env.Code, apperror.ErrServiceBusy)
	}
	if env.Message != "服务繁忙" {
		t.Errorf("envelope.message = %q, want %q", env.Message, "服务繁忙")
	}
}

// TestAuth_E2E_HappyAndUnauthorized 验证 Auth 中间件在 bootstrap 真 router 链
// 上端到端工作（Story 4.5 AC6 集成补丁）。
//
// 用 httptest.NewServer 真 HTTP roundtrip + bootstrap.NewRouter（注入 Auth
// 中间件） + 真 token sign / verify 跨进程。覆盖：
//   - 合法 Bearer token → handler 看到 c.Get(UserIDKey) = uint64
//   - 缺 Authorization header → envelope.code = 1001（ErrUnauthorized）
//
// 这条 e2e 同时验证 Auth 已能与 bootstrap 真 router 链 + ErrorMappingMiddleware
// 联动（与 Story 4.7 Layer 2 真 service / dockertest 集成测试不同：4.7 才接 db）。
func TestAuth_E2E_HappyAndUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)

	signer, err := auth.New("e2e-secret-must-be-at-least-16-bytes", 3600)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}

	r := bootstrap.NewRouter(bootstrap.Deps{Signer: signer})
	// 模拟 4.6 / 4.8 落地后的"已认证"路由组：挂 Auth + 一条业务 handler
	authedGroup := r.Group("", middleware.Auth(signer))
	authedGroup.GET("/test/me", func(c *gin.Context) {
		v, ok := c.Get(middleware.UserIDKey)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"hasUserID": false})
			return
		}
		c.JSON(http.StatusOK, gin.H{"userID": v})
	})

	srv := httptest.NewServer(r)
	defer srv.Close()

	// happy path
	tok, err := signer.Sign(8888, 0)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/test/me", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("client Do: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("happy status = %d, want 200; body=%s", resp.StatusCode, body)
	}
	var happy struct {
		UserID uint64 `json:"userID"`
	}
	if err := json.Unmarshal(body, &happy); err != nil {
		t.Fatalf("happy body not JSON: %v; raw=%s", err, body)
	}
	if happy.UserID != 8888 {
		t.Errorf("happy userID = %d, want 8888", happy.UserID)
	}

	// unauthorized path: 不设 header
	req2, _ := http.NewRequest(http.MethodGet, srv.URL+"/test/me", nil)
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		t.Fatalf("client Do (unauth): %v", err)
	}
	body2, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Errorf("unauth HTTP status = %d, want 200 (V1 §2.4: 业务码与 HTTP status 正交)", resp2.StatusCode)
	}
	var env struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(body2, &env); err != nil {
		t.Fatalf("unauth body not envelope JSON: %v; raw=%s", err, body2)
	}
	if env.Code != apperror.ErrUnauthorized {
		t.Errorf("unauth envelope.code = %d, want 1001", env.Code)
	}
}

// TestRateLimit_E2E_61stReturns1005 验证 RateLimit 中间件在真 router 链 +
// 真 HTTP roundtrip 上 61 次后返 1005（Story 4.5 AC6 集成补丁）。
func TestRateLimit_E2E_61stReturns1005(t *testing.T) {
	gin.SetMode(gin.TestMode)

	cfg := config.RateLimitConfig{PerKeyPerMin: 60, BurstSize: 60, BucketsLimit: 100}
	r := bootstrap.NewRouter(bootstrap.Deps{RateLimitCfg: cfg})
	// 模拟 4.6 落地后的 /auth 子组：挂 RateLimit by IP + 一条 handler
	authGroup := r.Group("", middleware.RateLimit(cfg, middleware.RateLimitByIP))
	authGroup.GET("/test/limited", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })

	srv := httptest.NewServer(r)
	defer srv.Close()

	client := &http.Client{}
	for i := 0; i < 60; i++ {
		req, _ := http.NewRequest(http.MethodGet, srv.URL+"/test/limited", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("client Do %d: %v", i+1, err)
		}
		_, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("burst %d failed: status=%d", i+1, resp.StatusCode)
		}
	}
	// 第 61 次：1005 envelope（HTTP 200）
	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/test/limited", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("61st client Do: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("61st HTTP status = %d, want 200", resp.StatusCode)
	}
	var env struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("body not envelope JSON: %v; raw=%s", err, body)
	}
	if env.Code != apperror.ErrTooManyRequests {
		t.Errorf("61st envelope.code = %d, want 1005", env.Code)
	}
}
