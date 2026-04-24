package middleware

import (
	"bytes"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newRecoveryRouter(t *testing.T, panicValue any) (*gin.Engine, *bytes.Buffer) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	r := gin.New()
	r.Use(RequestID())
	r.Use(Recovery())
	r.GET("/panic", func(c *gin.Context) { panic(panicValue) })
	return r, &buf
}

func TestRecovery_StringPanicReturns500Envelope(t *testing.T) {
	r, buf := newRecoveryRouter(t, "oops something went wrong")

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	var env struct {
		Code      int            `json:"code"`
		Message   string         `json:"message"`
		Data      map[string]any `json:"data"`
		RequestID string         `json:"requestId"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not JSON: %v; raw=%s", err, w.Body.String())
	}
	if env.Code != panicFallbackCode {
		t.Errorf("code = %d, want %d", env.Code, panicFallbackCode)
	}
	if env.Message != "服务繁忙" {
		t.Errorf("message = %q, want %q", env.Message, "服务繁忙")
	}
	if env.RequestID == "" {
		t.Errorf("requestId should be non-empty after RequestID middleware")
	}

	// log 断言：必须含 handler panic + panic value + stack 字段
	logStr := buf.String()
	if !strings.Contains(logStr, `"msg":"handler panic"`) {
		t.Errorf("log missing 'handler panic' message; log:\n%s", logStr)
	}
	if !strings.Contains(logStr, `oops something went wrong`) {
		t.Errorf("log missing panic value; log:\n%s", logStr)
	}
	if !strings.Contains(logStr, `"stack"`) {
		t.Errorf("log missing stack field; log:\n%s", logStr)
	}
}

func TestRecovery_ErrorPanicDoesNotDoubleFault(t *testing.T) {
	// panic(error) 而非 panic(string) —— 要求 slog.Any 能安全序列化 error 值
	r, buf := newRecoveryRouter(t, errors.New("boom from error"))

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", w.Code)
	}
	if !strings.Contains(buf.String(), "boom from error") {
		t.Errorf("log should include error panic value; log:\n%s", buf.String())
	}
}

func TestRecovery_SecondRequestStillWorks(t *testing.T) {
	r, _ := newRecoveryRouter(t, "first-panic")
	r.GET("/ok", func(c *gin.Context) { c.String(http.StatusOK, "still alive") })

	// 第一次：panic 路由 → 500
	req1 := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusInternalServerError {
		t.Fatalf("first request status = %d, want 500", w1.Code)
	}

	// 第二次：普通路由仍然 200（进程 / router 未崩）
	req2 := httptest.NewRequest(http.MethodGet, "/ok", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("second request after panic status = %d, want 200", w2.Code)
	}
	if w2.Body.String() != "still alive" {
		t.Errorf("second request body = %q, want %q", w2.Body.String(), "still alive")
	}
}
