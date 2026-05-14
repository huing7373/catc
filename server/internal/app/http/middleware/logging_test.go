package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/infra/metrics"
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

// TestLogging_DevURL_Unregistered_NotCounted 验当 /dev/* URL 命中**未注册**路由（prod
// build / devtools=false 场景）时，Logging 中间件**不**调 ObserveHTTP —— 也就不会让
// api_path="UNKNOWN" 桶吃到 dev probe / e2e 流量。
//
// 关键防回归点（Story 20.8 r4 lesson）：
//
// metrics.isDevPath 只能识别**已注册**的 Gin route pattern（如 "/dev/grant-cosmetic-batch"）。
// 但 prod build 不挂 dev handler 时，c.FullPath() 返**空串** → metrics 层 isDevPath("") = false
// → 落到 api_path="UNKNOWN" 桶污染 5xx 告警。caller-side（本中间件）用 raw URL.Path
// 检查才是真正的根因 fix。
//
// 不依赖 metrics 包私有 Counter；用 /metrics 文本快照断言 dev URL 不出现在任一系列里。
func TestLogging_DevURL_Unregistered_NotCounted(t *testing.T) {
	r, _ := newLoggingRouter(t, func(r *gin.Engine) {
		// 故意**不**注册任何 /dev/* 路由，模拟 prod build（无 devtools tag）的真实状态：
		// dev handler 物理不存在，所有 /dev/* 请求都会走 Gin NoRoute → 404 + 空 FullPath()。
		r.GET("/ping", func(c *gin.Context) { c.String(http.StatusOK, "pong") })
	})

	devURL := "/dev/grant-cosmetic-batch-probe-unregistered"
	req := httptest.NewRequest(http.MethodPost, devURL, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("unregistered dev URL status = %d, want 404", w.Code)
	}

	// 抓 /metrics 文本快照：dev URL 子串 + api_path="UNKNOWN" 这一桶都不能因这次请求出现增量。
	// 由于 "UNKNOWN" 桶可能被同包/同进程其它测试用过，**精确语义**是检查 dev URL 本身没泄漏 +
	// 抓 snapshot 前后 UNKNOWN counter 不变。后者更强 —— 用 httpRequestsTotal 不可见（私有），
	// 退化为：metrics 文本里不能含 dev URL 这段 path 子串（dev URL 也不会被原样写进 metrics，
	// 因为 caller 侧已跳过）。
	snapshot := scrapeMetrics(t)
	if strings.Contains(snapshot, devURL) {
		t.Errorf("/dev/* URL 不应出现在 metrics 文本中；实际泄漏：\n%s", snapshot)
	}
}

// TestLogging_DevURL_Registered_NotCounted 验当 /dev/* 路由**已注册**（devtools build）
// 时，logging 中间件也不调 ObserveHTTP（双层防御中 caller 侧那一层直接命中前缀，
// 不依赖 metrics 层的 isDevPath）。
func TestLogging_DevURL_Registered_NotCounted(t *testing.T) {
	r, _ := newLoggingRouter(t, func(r *gin.Engine) {
		// 已注册 dev 路由：模拟 devtools build。期望仍 skip metrics。
		r.POST("/dev/registered-probe", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	})

	devURL := "/dev/registered-probe"
	req := httptest.NewRequest(http.MethodPost, devURL, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("registered dev route status = %d, want 200", w.Code)
	}

	snapshot := scrapeMetrics(t)
	if strings.Contains(snapshot, devURL) {
		t.Errorf("已注册的 /dev/* 路由也不应出现在 metrics 文本中；实际：\n%s", snapshot)
	}
}

// TestLogging_NonDevURL_StillCounted 反向 case：非 /dev/* 路径必须正常进 metrics（双层
// 检查不能误伤）。/dev 无尾斜杠 / /devops/... 等同名前缀仍应被计数（与 metrics
// http_test.go::TestObserveHTTP_DevPrefixDiscipline 对称）。
func TestLogging_NonDevURL_StillCounted(t *testing.T) {
	r, _ := newLoggingRouter(t, func(r *gin.Engine) {
		r.GET("/devops/healthz", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
		r.GET("/ping", func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"ok": true}) })
	})

	for _, p := range []string{"/devops/healthz", "/ping"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("%s status = %d, want 200", p, w.Code)
		}
	}

	snapshot := scrapeMetrics(t)
	// 这两条路径都该出现在 cat_api_requests_total 序列里
	for _, p := range []string{"/devops/healthz", "/ping"} {
		expected := `cat_api_requests_total{api_path="` + p + `"`
		if !strings.Contains(snapshot, expected) {
			t.Errorf("非 /dev/* 路径 %s 应出现在 metrics 文本中；snapshot:\n%s", p, snapshot)
		}
	}
}

// scrapeMetrics 抓取 metrics 包 /metrics 文本输出，便于断言。
func scrapeMetrics(t *testing.T) string {
	t.Helper()
	h := metrics.Handler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("metrics handler status = %d, want 200", w.Code)
	}
	return w.Body.String()
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
