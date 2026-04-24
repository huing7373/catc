package middleware

import (
	"bytes"
	"encoding/json"
	stderrors "errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
)

// newRecoveryRouter 构造完整中间件链测试 Recovery + ErrorMappingMiddleware
// 协作：panic → Recovery 抓 → c.Error AppError → ErrorMappingMiddleware 写 envelope。
//
// 顺序与 router.go 一致：RequestID → ErrorMappingMiddleware → Recovery → handler。
// 这里省略 Logging（panic 路径不依赖它）。
func newRecoveryRouter(t *testing.T, panicValue any) (*gin.Engine, *bytes.Buffer) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	r := gin.New()
	r.Use(RequestID())
	r.Use(ErrorMappingMiddleware())
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
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("code = %d, want %d", env.Code, apperror.ErrServiceBusy)
	}
	if env.Message != "服务繁忙" {
		t.Errorf("message = %q, want %q", env.Message, "服务繁忙")
	}
	if env.RequestID == "" {
		t.Errorf("requestId 不应为空（RequestID 中间件应注入）")
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
	// 同时 panicAsErr 要原样返回 error 类型（保留底层错误链）
	innerErr := stderrors.New("boom from error")
	r, buf := newRecoveryRouter(t, innerErr)

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

func TestRecovery_PushesAppErrorToCErrors(t *testing.T) {
	// 直接验证 Recovery 与 ErrorMappingMiddleware 之间的**契约接口**：
	// Recovery 必须把 panic 值 wrap 成 *AppError(ErrServiceBusy) 推到 c.Errors。
	// 本测试**不**挂 ErrorMappingMiddleware（用一个 capture 中间件替代）—— 聚焦
	// Recovery 自身行为；envelope 写入由 TestErrorMapping_PanicHandledViaRecovery 覆盖。
	gin.SetMode(gin.TestMode)
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	var capturedErrors []*gin.Error
	r := gin.New()
	r.Use(RequestID())
	r.Use(func(c *gin.Context) {
		c.Next()
		capturedErrors = make([]*gin.Error, len(c.Errors))
		copy(capturedErrors, c.Errors)
	})
	r.Use(Recovery())
	r.GET("/panic", func(c *gin.Context) { panic("x") })

	req := httptest.NewRequest(http.MethodGet, "/panic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if len(capturedErrors) != 1 {
		t.Fatalf("c.Errors len = %d, want 1", len(capturedErrors))
	}
	ae, ok := apperror.As(capturedErrors[0].Err)
	if !ok {
		t.Fatalf("c.Errors[0].Err 应为 *AppError，实际 %T: %v", capturedErrors[0].Err, capturedErrors[0].Err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d", ae.Code, apperror.ErrServiceBusy)
	}
	if ae.Cause == nil {
		t.Errorf("AppError.Cause 应保留 panic 值，不应为 nil")
	} else if !strings.Contains(ae.Cause.Error(), "x") {
		t.Errorf("AppError.Cause.Error() = %q, 应含 panic 值 'x'", ae.Cause.Error())
	}
	// Recovery 自身**不**写 status（status 决策权交 ErrorMappingMiddleware）
	// 这里 w.Code 是 Gin 默认 200 —— 不做断言；envelope + status 由 ErrorMapping 测试覆盖
	_ = w
}
