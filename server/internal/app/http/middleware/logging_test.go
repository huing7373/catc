package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
)

// newLoggingRouter 构造完整中间件链测试 Logging 行为。
//
// 顺序与 router.go 一致：
//
//	RequestID → Logging → ErrorMappingMiddleware → Recovery → handler
//
// **必须挂 ErrorMappingMiddleware**：Logging 现在通过 ResponseErrorCodeKey
// （c.Keys）从 ErrorMappingMiddleware 读取 canonical envelope.code。漏挂会让
// error_code 字段在所有错误路径下都消失（不是 bug，是 Logging 设计契约
// 失去前置依赖）。
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
	r.Use(ErrorMappingMiddleware())
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

	// user_id / business_result / error_code / ctx_done 留空 —— 断言 key 不出现
	// （error_code 由 Story 1.8 落地；ctx_done 由 Story 1.9 落地；两者成功路径都省略）
	for _, forbidden := range []string{"user_id", "business_result", "error_code", "ctx_done"} {
		if _, present := m[forbidden]; present {
			t.Errorf("field %q should NOT appear in success-path log; got %v", forbidden, m[forbidden])
		}
	}
}

func TestLogging_AddsErrorCodeFromAppError(t *testing.T) {
	// handler 通过 c.Error 推 *AppError；Logging 中间件应在 http_request log
	// 中追加 error_code 字段（ADR-0001 §4 "Story 1.8 生效"）
	r, buf := newLoggingRouter(t, func(r *gin.Engine) {
		r.GET("/biz-err", func(c *gin.Context) {
			_ = c.Error(apperror.New(apperror.ErrInvalidParam, "x"))
			c.Abort()
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/biz-err", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	_ = w

	m := parseLastLogLine(t, buf)
	if m["msg"] != "http_request" {
		t.Fatalf("expected http_request log line; got %v", m["msg"])
	}
	code, ok := m["error_code"].(float64)
	if !ok {
		t.Fatalf("error_code missing or non-numeric: %v (full log: %v)", m["error_code"], m)
	}
	if int(code) != apperror.ErrInvalidParam {
		t.Errorf("error_code = %v, want %d", code, apperror.ErrInvalidParam)
	}
}

func TestLogging_NoErrorCodeWhenSuccess(t *testing.T) {
	// 成功请求不应出现 error_code 字段（ADR-0001 §4 "成功请求省略该字段"）
	// 注意：本 case 与 TestLogging_HappyPath_HasSixFields 的 forbidden 检查重叠，
	// 但保留是为了显式文档化"success path 不写 error_code"这条契约
	r, buf := newLoggingRouter(t, func(r *gin.Engine) {
		r.GET("/ok", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	_ = w

	m := parseLastLogLine(t, buf)
	if _, present := m["error_code"]; present {
		t.Errorf("成功请求不应出现 error_code 字段；实际 log 含 error_code=%v", m["error_code"])
	}
}

// TestLogging_ErrorCode_ForNonAppErrorFallback：handler `c.Error` 推非 AppError
// （如 stdlib io.EOF / errors.New）时，ErrorMappingMiddleware wrap 为 1009 envelope；
// Logging 必须读到 ResponseErrorCodeKey=1009 而非"漏报"error_code。
//
// 防回归：fix-review P2 "Preserve mapped error_code on raw handler errors"。
// 历史 bug：Logging 自行扫 c.Errors 用 apperror.As，对非 AppError 返回 (nil, false)
// → http_request log 缺 error_code → 与响应 envelope 1009 不一致 → 监控失配。
func TestLogging_ErrorCode_ForNonAppErrorFallback(t *testing.T) {
	r, buf := newLoggingRouter(t, func(r *gin.Engine) {
		r.GET("/raw-err", func(c *gin.Context) {
			_ = c.Error(io.EOF) // stdlib error，**非** AppError
			c.Abort()
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/raw-err", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500（ErrorMappingMiddleware 应兜底为 1009）", w.Code)
	}

	m := parseLastLogLine(t, buf)
	if m["msg"] != "http_request" {
		t.Fatalf("expected http_request log line; got %v", m["msg"])
	}
	code, ok := m["error_code"].(float64)
	if !ok {
		t.Fatalf("error_code 字段缺失或非数字（应为 1009 与响应 envelope 对齐）；实际 log: %v", m)
	}
	if int(code) != 1009 {
		t.Errorf("error_code = %v, want 1009 (ErrServiceBusy)", code)
	}
}

// TestLogging_NoErrorCodeForDoubleWrite：handler 已 response.Success 后又调
// c.Error，ErrorMappingMiddleware 检测 Writer.Written()==true 保留成功响应。
// Logging 必须**不**写 error_code（响应 envelope 实际是 success，日志不应说错）。
//
// 防回归：fix-review P3 "Avoid logging an error_code for skipped double-write
// responses"。历史 bug：Logging 扫 c.Errors[0] 见 AppError 就追加 error_code，
// 即使响应是 success → 日志声称业务错误 → 监控误触发告警。
func TestLogging_NoErrorCodeForDoubleWrite(t *testing.T) {
	r, buf := newLoggingRouter(t, func(r *gin.Engine) {
		r.GET("/dbl", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"ok": true}) // 成功响应已写
			_ = c.Error(io.EOF)                      // dev bug：又推 error
		})
	})

	req := httptest.NewRequest(http.MethodGet, "/dbl", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200（成功响应已写，ErrorMappingMiddleware 不应覆写）", w.Code)
	}

	m := parseLastLogLine(t, buf)
	if m["msg"] != "http_request" {
		t.Fatalf("expected http_request log line; got %v", m["msg"])
	}
	if _, present := m["error_code"]; present {
		t.Errorf("double-write 场景下 http_request log 不应有 error_code；响应是 success，"+
			"日志声称错误会误触发告警。实际 log: %v", m)
	}
}

// TestLogging_NoCtxDoneOnSuccess：正常完成的请求 http_request log 中
// **不**出现 ctx_done 字段（缺省即 false，与 error_code 惯例一致）。
//
// Story 1.9 AC3 / ADR-0007 §4.3。
func TestLogging_NoCtxDoneOnSuccess(t *testing.T) {
	r, buf := newLoggingRouter(t, func(r *gin.Engine) {
		r.GET("/ok", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	})

	req := httptest.NewRequest(http.MethodGet, "/ok", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	m := parseLastLogLine(t, buf)
	if _, present := m["ctx_done"]; present {
		t.Errorf("正常请求不应出现 ctx_done 字段；实际 log 含 ctx_done=%v", m["ctx_done"])
	}
}

// TestLogging_CtxDoneWhenClientDisconnects：通过 req.WithContext 注入一个
// 已 cancel 的 ctx 模拟客户端断开；http_request log 必须含 ctx_done=true。
//
// Gin 默认不主动中断 handler —— handler 仍会跑完。但 c.Request.Context().Err()
// 在 c.Next() 之后读时为非 nil，Logging 中间件应追加 ctx_done=true 字段。
//
// Story 1.9 AC3 / ADR-0007 §4.1-4.2。
func TestLogging_CtxDoneWhenClientDisconnects(t *testing.T) {
	r, buf := newLoggingRouter(t, func(r *gin.Engine) {
		// handler 正常成功返回；真正的 cancel 信号通过 req.Context() 注入
		r.GET("/canceled", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即 cancel：模拟 client 在请求处理期间断开

	req := httptest.NewRequest(http.MethodGet, "/canceled", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	m := parseLastLogLine(t, buf)
	if m["msg"] != "http_request" {
		t.Fatalf("expected http_request log line; got %v", m["msg"])
	}
	got, ok := m["ctx_done"].(bool)
	if !ok {
		t.Fatalf("ctx_done 字段缺失或非 bool；实际 log: %v", m)
	}
	if !got {
		t.Errorf("ctx_done = false, want true（ctx 已 cancel，Logging 应识别）")
	}
}

// TestLogging_CtxDoneOnDeadlineExceeded：ctx WithTimeout 到期也应触发 ctx_done=true
// （ADR-0007 §4.4 "不区分 Canceled vs DeadlineExceeded"的对称验证）。
func TestLogging_CtxDoneOnDeadlineExceeded(t *testing.T) {
	r, buf := newLoggingRouter(t, func(r *gin.Engine) {
		r.GET("/timeout", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	})

	// 0ns timeout：创建即过期
	ctx, cancel := context.WithTimeout(context.Background(), 0)
	defer cancel()

	req := httptest.NewRequest(http.MethodGet, "/timeout", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	m := parseLastLogLine(t, buf)
	got, ok := m["ctx_done"].(bool)
	if !ok {
		t.Fatalf("ctx_done 字段缺失或非 bool；实际 log: %v", m)
	}
	if !got {
		t.Errorf("ctx_done = false, want true（ctx deadline 已过，Logging 应识别）")
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
