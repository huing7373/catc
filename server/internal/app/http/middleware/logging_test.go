package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newLoggingRouter(t *testing.T, registerRoutes func(r *gin.Engine)) (*gin.Engine, *bytes.Buffer) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	r := gin.New()
	r.Use(RequestID())
	r.Use(Logging())
	r.Use(Recovery())
	registerRoutes(r)
	return r, &buf
}

func parseLastLogLine(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	lines := bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n"))
	if len(lines) == 0 {
		t.Fatalf("log buffer is empty")
	}
	var m map[string]any
	if err := json.Unmarshal(lines[len(lines)-1], &m); err != nil {
		t.Fatalf("last log line not JSON: %v; raw=%s", err, string(lines[len(lines)-1]))
	}
	return m
}

func TestLogging_HappyPath_HasSixFields(t *testing.T) {
	r, buf := newLoggingRouter(t, func(r *gin.Engine) {
		r.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	})

	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	m := parseLastLogLine(t, buf)

	if m["msg"] != "http_request" {
		t.Errorf("msg = %v, want http_request", m["msg"])
	}
	if rid, _ := m["request_id"].(string); rid == "" {
		t.Errorf("request_id missing or empty: %v", m["request_id"])
	}
	if m["api_path"] != "/ping" {
		t.Errorf("api_path = %v, want /ping", m["api_path"])
	}
	if m["method"] != "GET" {
		t.Errorf("method = %v, want GET", m["method"])
	}
	if status, _ := m["status"].(float64); int(status) != 200 {
		t.Errorf("status = %v, want 200", m["status"])
	}
	if _, ok := m["latency_ms"].(float64); !ok {
		t.Errorf("latency_ms missing or non-numeric: %v", m["latency_ms"])
	}
	if cip, _ := m["client_ip"].(string); cip == "" {
		t.Errorf("client_ip should be non-empty when RemoteAddr set")
	}

	// user_id / business_result / error_code 留空 —— 断言 key 不出现
	for _, forbidden := range []string{"user_id", "business_result", "error_code"} {
		if _, present := m[forbidden]; present {
			t.Errorf("field %q should NOT appear in Story 1.3 log; got %v", forbidden, m[forbidden])
		}
	}
}

func TestLogging_UnmatchedRoute_EmptyApiPath(t *testing.T) {
	r, buf := newLoggingRouter(t, func(r *gin.Engine) {
		// 故意只注册 /ping；请求 /nonexistent 会走到 gin 404 处理
		r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	})

	req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("unmatched route status = %d, want 404", w.Code)
	}

	m := parseLastLogLine(t, buf)
	if m["api_path"] != "" {
		t.Errorf("api_path for unmatched route = %v, want empty string (gin c.FullPath())", m["api_path"])
	}
	if status, _ := m["status"].(float64); int(status) != 404 {
		t.Errorf("status = %v, want 404", m["status"])
	}
}
