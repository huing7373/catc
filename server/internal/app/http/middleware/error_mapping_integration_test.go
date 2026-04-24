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

	r := bootstrap.NewRouter()
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

	r := bootstrap.NewRouter()
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
