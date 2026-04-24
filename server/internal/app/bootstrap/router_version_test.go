package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/buildinfo"
)

// TestRouter_Version 验证挂完三件套中间件后，模拟 ldflags 注入
// commit / builtAt → /version 端点返回完整 envelope：
//   - code == 0, message == "ok"
//   - data.commit / data.builtAt 与注入值一致
//   - requestId 符合 UUIDv4 格式（由 RequestID middleware 生成）
//   - body.requestId 与响应 header X-Request-Id 一致
//
// 说明：该测试不能 t.Parallel()，因为 buildinfo 变量是全局包级。
func TestRouter_Version(t *testing.T) {
	origCommit, origBuiltAt := buildinfo.Commit, buildinfo.BuiltAt
	defer func() { buildinfo.Commit, buildinfo.BuiltAt = origCommit, origBuiltAt }()
	buildinfo.Commit = "abc1234"
	buildinfo.BuiltAt = "2026-04-26T00:00:00Z"

	gin.SetMode(gin.TestMode)
	r := NewRouter()

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Commit  string `json:"commit"`
			BuiltAt string `json:"builtAt"`
		} `json:"data"`
		RequestID string `json:"requestId"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON body: %v; body=%s", err, w.Body.String())
	}
	if env.Code != 0 {
		t.Errorf("code = %d, want 0", env.Code)
	}
	if env.Message != "ok" {
		t.Errorf("message = %q, want %q", env.Message, "ok")
	}
	if env.Data.Commit != "abc1234" {
		t.Errorf("data.commit = %q, want %q", env.Data.Commit, "abc1234")
	}
	if env.Data.BuiltAt != "2026-04-26T00:00:00Z" {
		t.Errorf("data.builtAt = %q, want %q", env.Data.BuiltAt, "2026-04-26T00:00:00Z")
	}

	headerRID := w.Header().Get("X-Request-Id")
	if !uuidV4Pattern.MatchString(headerRID) {
		t.Errorf("X-Request-Id header = %q, want UUIDv4", headerRID)
	}
	if env.RequestID != headerRID {
		t.Errorf("body.requestId = %q, header = %q; must match", env.RequestID, headerRID)
	}
}
