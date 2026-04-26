package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

var uuidV4Pattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestRouter_Ping(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := NewRouter(Deps{})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var env struct {
		Code      int            `json:"code"`
		Message   string         `json:"message"`
		Data      map[string]any `json:"data"`
		RequestID string         `json:"requestId"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON body: %v; body=%s", err, w.Body.String())
	}
	if env.Code != 0 {
		t.Errorf("code = %d, want 0", env.Code)
	}
	if env.Message != "pong" {
		t.Errorf("message = %q, want %q", env.Message, "pong")
	}
	if env.Data == nil {
		t.Errorf("data should be non-nil empty object, got nil (JSON null)")
	}
	if len(env.Data) != 0 {
		t.Errorf("data = %v, want empty object", env.Data)
	}
	if env.RequestID == "" {
		t.Errorf("requestId is empty, want non-empty fallback")
	}
}

// TestRouter_PingRequestIDIsUUIDv4 验证中间件挂载后，/ping 响应的 requestId
// 是真 UUIDv4（而非 Story 1.2 的占位 "req_xxx"），且 body.requestId 与响应
// header X-Request-Id 一致。
func TestRouter_PingRequestIDIsUUIDv4(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := NewRouter(Deps{})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	headerRID := w.Header().Get("X-Request-Id")
	if !uuidV4Pattern.MatchString(headerRID) {
		t.Errorf("X-Request-Id header = %q, want UUIDv4", headerRID)
	}

	var env struct {
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if env.RequestID != headerRID {
		t.Errorf("body.requestId = %q, header = %q; must match", env.RequestID, headerRID)
	}
}

// TestRouter_PanicRouteAndSubsequentPing 验证 panic 路由 → 500 envelope；
// 后续 /ping 请求仍然 200（服务未挂）。
func TestRouter_PanicRouteAndSubsequentPing(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := NewRouter(Deps{})
	// 在 NewRouter 的基础上额外注册一个 /panic 路由
	r.GET("/panic-for-test", func(c *gin.Context) { panic("deliberate panic for integration test") })

	// 第一次：触发 panic
	req1 := httptest.NewRequest(http.MethodGet, "/panic-for-test", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusInternalServerError {
		t.Fatalf("panic route status = %d, want 500", w1.Code)
	}
	var env1 struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	_ = json.Unmarshal(w1.Body.Bytes(), &env1)
	if env1.Code != 1009 {
		t.Errorf("panic response code = %d, want 1009", env1.Code)
	}

	// 第二次：/ping 仍正常
	req2 := httptest.NewRequest(http.MethodGet, "/ping", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("ping after panic status = %d, want 200", w2.Code)
	}
}

// TestRouter_MetricsEndpoint 验证 /metrics 返回 prometheus 文本格式且含两个
// Story 1.3 指标名。
func TestRouter_MetricsEndpoint(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := NewRouter(Deps{})

	// 先发一次 /ping 触发 metric observe
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/ping", nil))

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("/metrics status = %d, want 200", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "cat_api_requests_total") {
		t.Errorf("/metrics body missing cat_api_requests_total; body (first 500 chars):\n%.500s", body)
	}
	if !strings.Contains(body, "cat_api_request_duration_seconds") {
		t.Errorf("/metrics body missing cat_api_request_duration_seconds; body (first 500 chars):\n%.500s", body)
	}
}
