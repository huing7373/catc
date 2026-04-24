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
	"github.com/huing/cat/server/internal/pkg/response"
)

// newErrorMappingRouter 构造一个挂好"完整中间件链"的 router 用于测试 envelope。
//
// 链：RequestID → Logging → ErrorMappingMiddleware → Recovery → testHandler
//
// 顺序与 router.go 一致：ErrorMappingMiddleware **必须外层于** Recovery，
// 这样 Recovery defer recover() 后通过 c.Error 推出去的 AppError 能被本中间件
// 在 after-c.Next() 阶段写成 envelope。
func newErrorMappingRouter(t *testing.T, h gin.HandlerFunc) (*gin.Engine, *bytes.Buffer) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	r := gin.New()
	r.Use(RequestID())
	r.Use(Logging())
	r.Use(ErrorMappingMiddleware())
	r.Use(Recovery())
	r.GET("/test", h)
	return r, &buf
}

// envelope 是 V1 §2.4 的统一响应结构（测试用，不导出）。
type envelope struct {
	Code      int            `json:"code"`
	Message   string         `json:"message"`
	Data      map[string]any `json:"data"`
	RequestID string         `json:"requestId"`
}

func mustEnvelope(t *testing.T, body []byte) envelope {
	t.Helper()
	var e envelope
	if err := json.Unmarshal(body, &e); err != nil {
		t.Fatalf("body not envelope JSON: %v; raw=%s", err, body)
	}
	return e
}

func TestErrorMapping_HandlerReturnsAppError(t *testing.T) {
	// 用 capture 中间件抓取 c.Keys[ResponseErrorCodeKey] 的最终态
	// （在 ErrorMappingMiddleware 之后、Logging 之前的位置读）
	var capturedKey any
	var capturedKeyExists bool

	gin.SetMode(gin.TestMode)
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&bytes.Buffer{}, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	r := gin.New()
	r.Use(RequestID())
	// 模拟 Logging 的位置（在 ErrorMappingMiddleware 上层）：在 c.Next() 之后读 key
	r.Use(func(c *gin.Context) {
		c.Next()
		capturedKey, capturedKeyExists = c.Get(ResponseErrorCodeKey)
	})
	r.Use(ErrorMappingMiddleware())
	r.Use(Recovery())
	r.GET("/test", func(c *gin.Context) {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "x 必填"))
		c.Abort()
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (业务错误走 200 + envelope code)", w.Code)
	}
	e := mustEnvelope(t, w.Body.Bytes())
	if e.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d", e.Code, apperror.ErrInvalidParam)
	}
	if e.Message != "x 必填" {
		t.Errorf("envelope.message = %q, want %q", e.Message, "x 必填")
	}
	if e.RequestID == "" {
		t.Errorf("envelope.requestId 不应为空（RequestID 中间件应注入）")
	}

	// 契约断言：写 envelope 后必须 Set ResponseErrorCodeKey 供 Logging 读取
	if !capturedKeyExists {
		t.Errorf("ResponseErrorCodeKey 未被设置；Logging 将无法写 error_code")
	}
	if code, _ := capturedKey.(int); code != apperror.ErrInvalidParam {
		t.Errorf("ResponseErrorCodeKey value = %v, want %d", capturedKey, apperror.ErrInvalidParam)
	}
}

func TestErrorMapping_HandlerReturnsNonAppError(t *testing.T) {
	r, buf := newErrorMappingRouter(t, func(c *gin.Context) {
		_ = c.Error(stderrors.New("db down"))
		c.Abort()
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 非 AppError 兜底属系统级问题 → HTTP 500（让 LB / 监控告警能识别）
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (非 AppError 兜底为系统级 1009)", w.Code)
	}
	e := mustEnvelope(t, w.Body.Bytes())
	if e.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d (非 AppError 兜底为 1009)", e.Code, apperror.ErrServiceBusy)
	}
	if e.Message != "服务繁忙" {
		t.Errorf("envelope.message = %q, want %q", e.Message, "服务繁忙")
	}

	logStr := buf.String()
	if !strings.Contains(logStr, `"cause":"db down"`) {
		t.Errorf("log 应含 cause=db down；实际 log:\n%s", logStr)
	}
	if !strings.Contains(logStr, `"error_code":1009`) {
		t.Errorf("log 应含 error_code=1009；实际 log:\n%s", logStr)
	}
	if !strings.Contains(logStr, `"level":"ERROR"`) {
		t.Errorf("非 AppError 兜底应 ERROR 级别（触发告警）；实际 log:\n%s", logStr)
	}
}

func TestErrorMapping_PanicHandledViaRecovery(t *testing.T) {
	r, buf := newErrorMappingRouter(t, func(c *gin.Context) {
		panic("boom")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500 (panic 路径保持 500)", w.Code)
	}
	e := mustEnvelope(t, w.Body.Bytes())
	if e.Code != apperror.ErrServiceBusy {
		t.Errorf("envelope.code = %d, want %d", e.Code, apperror.ErrServiceBusy)
	}
	if e.Message != "服务繁忙" {
		t.Errorf("envelope.message = %q, want %q", e.Message, "服务繁忙")
	}

	logStr := buf.String()
	if !strings.Contains(logStr, "handler panic") {
		t.Errorf("log 应含 'handler panic'（Recovery 写入）；实际 log:\n%s", logStr)
	}
	if !strings.Contains(logStr, `"error_code":1009`) {
		t.Errorf("log 应含 error_mapping error_code=1009；实际 log:\n%s", logStr)
	}
}

func TestErrorMapping_NoErrorIsNoOp(t *testing.T) {
	r, _ := newErrorMappingRouter(t, func(c *gin.Context) {
		response.Success(c, gin.H{"hello": "world"}, "ok")
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	e := mustEnvelope(t, w.Body.Bytes())
	if e.Code != 0 {
		t.Errorf("envelope.code = %d, want 0 (success)", e.Code)
	}
	if e.Message != "ok" {
		t.Errorf("envelope.message = %q, want %q (本中间件不应改写已写好的成功响应)", e.Message, "ok")
	}
	hello, _ := e.Data["hello"].(string)
	if hello != "world" {
		t.Errorf("envelope.data.hello = %q, want %q", hello, "world")
	}
}

func TestErrorMapping_DoubleWriteGuarded(t *testing.T) {
	// dev bug 场景：handler 已写响应又调 c.Error
	// 用 capture 中间件验证 ResponseErrorCodeKey 在 double-write 路径**未**被设置
	var capturedKeyExists bool

	gin.SetMode(gin.TestMode)
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	r := gin.New()
	r.Use(RequestID())
	r.Use(func(c *gin.Context) {
		c.Next()
		_, capturedKeyExists = c.Get(ResponseErrorCodeKey)
	})
	r.Use(ErrorMappingMiddleware())
	r.Use(Recovery())
	r.GET("/test", func(c *gin.Context) {
		response.Success(c, gin.H{"first": true}, "ok")
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "too late"))
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 第一次 Success 写入的响应应当保留，envelope code = 0
	e := mustEnvelope(t, w.Body.Bytes())
	if e.Code != 0 {
		t.Errorf("envelope.code = %d, want 0 (本中间件不应覆盖 handler 已写的响应)", e.Code)
	}

	// 但日志必须记录这条 dev bug，让排障时看得到
	logStr := buf.String()
	if !strings.Contains(logStr, "error_mapping skipped: response already written") {
		t.Errorf("double-write 应在日志中可见；实际 log:\n%s", logStr)
	}

	// 契约断言：double-write 路径**不**设 ResponseErrorCodeKey，
	// 这样 Logging 不会在响应实际是 success 的请求上写 error_code
	if capturedKeyExists {
		t.Errorf("double-write 路径不应设 ResponseErrorCodeKey；否则 Logging 会误标 error_code"+
			"（响应实际是 success），触发监控误告警")
	}
}
