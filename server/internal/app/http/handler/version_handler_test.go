package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/buildinfo"
)

// TestVersionHandler_HappyPath 验证注入了 commit / builtAt 时，/version 返回
// 正确的 data 字段值与统一 envelope 结构。
func TestVersionHandler_HappyPath(t *testing.T) {
	origCommit, origBuiltAt := buildinfo.Commit, buildinfo.BuiltAt
	defer func() { buildinfo.Commit, buildinfo.BuiltAt = origCommit, origBuiltAt }()
	buildinfo.Commit = "abc1234"
	buildinfo.BuiltAt = "2026-04-26T10:00:00Z"

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/version", VersionHandler)

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var env struct {
		Code      int             `json:"code"`
		Message   string          `json:"message"`
		Data      VersionResponse `json:"data"`
		RequestID string          `json:"requestId"`
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
	if env.Data.BuiltAt != "2026-04-26T10:00:00Z" {
		t.Errorf("data.builtAt = %q, want %q", env.Data.BuiltAt, "2026-04-26T10:00:00Z")
	}
	if env.RequestID == "" {
		t.Errorf("requestId is empty, want non-empty fallback")
	}
}

// TestVersionHandler_UnknownDefault 验证未注入 ldflags（变量保持默认值
// "unknown"）时，/version 返回 "unknown" 而非空串或 null。
func TestVersionHandler_UnknownDefault(t *testing.T) {
	origCommit, origBuiltAt := buildinfo.Commit, buildinfo.BuiltAt
	defer func() { buildinfo.Commit, buildinfo.BuiltAt = origCommit, origBuiltAt }()
	buildinfo.Commit = "unknown"
	buildinfo.BuiltAt = "unknown"

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/version", VersionHandler)

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var env struct {
		Data VersionResponse `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("invalid JSON body: %v; body=%s", err, w.Body.String())
	}
	if env.Data.Commit != "unknown" {
		t.Errorf("data.commit = %q, want %q", env.Data.Commit, "unknown")
	}
	if env.Data.BuiltAt != "unknown" {
		t.Errorf("data.builtAt = %q, want %q", env.Data.BuiltAt, "unknown")
	}
}

// TestVersionHandler_EnvelopeShape 验证响应严格符合统一 envelope：
// 四顶层字段 code / message / data(object) / requestId 都存在，data 是非 null
// 非 array 的 object（含 commit / builtAt 两字段）。
func TestVersionHandler_EnvelopeShape(t *testing.T) {
	origCommit, origBuiltAt := buildinfo.Commit, buildinfo.BuiltAt
	defer func() { buildinfo.Commit, buildinfo.BuiltAt = origCommit, origBuiltAt }()
	buildinfo.Commit = "abc1234"
	buildinfo.BuiltAt = "2026-04-26T10:00:00Z"

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/version", VersionHandler)

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &raw); err != nil {
		t.Fatalf("body not JSON object: %v; body=%s", err, w.Body.String())
	}
	for _, key := range []string{"code", "message", "data", "requestId"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("envelope missing field %q; got keys=%v", key, keys(raw))
		}
	}

	var code int
	if err := json.Unmarshal(raw["code"], &code); err != nil {
		t.Fatalf("code not int: %v", err)
	}
	if code != 0 {
		t.Errorf("code = %d, want 0", code)
	}

	var requestID string
	if err := json.Unmarshal(raw["requestId"], &requestID); err != nil {
		t.Fatalf("requestId not string: %v", err)
	}
	if requestID == "" {
		t.Errorf("requestId empty, want non-empty fallback")
	}

	// data 必须是 non-null object（不是 null / array / string）
	dataBytes := raw["data"]
	if len(dataBytes) == 0 || string(dataBytes) == "null" {
		t.Fatalf("data is null or empty; want object")
	}
	if dataBytes[0] != '{' {
		t.Fatalf("data is not a JSON object; first byte = %q", dataBytes[0])
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(dataBytes, &data); err != nil {
		t.Fatalf("data not object: %v", err)
	}
	for _, key := range []string{"commit", "builtAt"} {
		if _, ok := data[key]; !ok {
			t.Errorf("data missing field %q; got keys=%v", key, keys(data))
		}
	}
}

func keys(m map[string]json.RawMessage) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
