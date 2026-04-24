//go:build !devtools

// 本测试文件假设 forceDevEnabled=false，即 IsEnabled() 仅由 BUILD_DEV 驱动。
// 带 `-tags devtools` 时 forceDevEnabled=true，多数 case 的前置会破裂（尤其是
// case 2/5/6/7 依赖 "BUILD_DEV=false → IsEnabled()==false"）；整体文件 build tag
// 为 `!devtools`，只在默认构建下编译 + 运行。
//
// AC9 决策：build-tag 强制启用路径由 `go build -tags devtools` + 手动验证覆盖，
// 自动化测试不跑。这里用 build tag 让 `go test -tags devtools ./...` 跳过整个文件，
// 避免"env-var 测试被 build-tag 污染结果"的误伤。

package devtools_test

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/app/http/devtools"
	"github.com/huing/cat/server/internal/app/http/middleware"
	"github.com/huing/cat/server/internal/pkg/testing/slogtest"
)

// envelope 与 response.Envelope 对齐（只读，不引 response 包避免测试包 import 扩散）。
type envelope struct {
	Code      int            `json:"code"`
	Message   string         `json:"message"`
	Data      map[string]any `json:"data"`
	RequestID string         `json:"requestId"`
}

// newEngine 构造一个**裸** Engine，不挂任何中间件，用于 devtools 包自己的最小单测。
// 生产 NewRouter 的集成行为由 bootstrap/router_test.go 验证（AC8）。
func newEngine() *gin.Engine {
	gin.SetMode(gin.TestMode)
	return gin.New()
}

// doGet 帮助函数：对给定 engine 发一次 GET 请求，返回 recorder。
func doGet(r *gin.Engine, path string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// --- AC7 case 1 ------------------------------------------------------------

// TestRegister_BuildDevTrue_PingDevReturns200 验证 BUILD_DEV=true 时
// /dev/ping-dev 返回 200 + envelope.data.mode=="dev"。
func TestRegister_BuildDevTrue_PingDevReturns200(t *testing.T) {
	t.Setenv("BUILD_DEV", "true")

	r := newEngine()
	devtools.Register(r)

	w := doGet(r, "/dev/ping-dev")

	require.Equal(t, http.StatusOK, w.Code, "status should be 200; body=%s", w.Body.String())

	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "body must be JSON envelope")
	assert.Equal(t, 0, env.Code, "envelope.code should be 0")
	assert.Equal(t, "ok", env.Message, "envelope.message should be 'ok'")
	require.NotNil(t, env.Data, "envelope.data must not be nil")
	assert.Equal(t, "dev", env.Data["mode"], "envelope.data.mode should be 'dev'")
}

// --- AC7 case 2 ------------------------------------------------------------

// TestRegister_BuildDevFalse_PingDevReturns404 验证 BUILD_DEV 不为 "true" 时
// Register 跳过注册 → /dev/ping-dev 走 Gin 默认 NoRoute → 返回 404（**非** envelope）。
//
// 关键：此 case 断言的是 Gin 原生 404 响应（文本 "404 page not found"），
// 而非 DevOnlyMiddleware 的 envelope 404。后者由 case 4 单独覆盖。
func TestRegister_BuildDevFalse_PingDevReturns404(t *testing.T) {
	t.Setenv("BUILD_DEV", "")

	r := newEngine()
	devtools.Register(r)

	w := doGet(r, "/dev/ping-dev")

	assert.Equal(t, http.StatusNotFound, w.Code, "status should be 404 (Gin NoRoute)")
	body := w.Body.String()
	// Gin 默认 NoRoute 返回文本 "404 page not found"；不应是 JSON envelope。
	assert.Contains(t, body, "404 page not found", "body should be Gin default text, not envelope; got=%q", body)
}

// --- AC7 case 4 ------------------------------------------------------------

// TestDevOnlyMiddleware_RejectsWhenDisabled 单独测 DevOnlyMiddleware：
// BUILD_DEV 未设时推 ErrResourceNotFound 到 c.Errors → ErrorMappingMiddleware
// 统一翻成 envelope (code=1003, HTTP 200) + 打 WARN 日志。
//
// 本测试挂 ErrorMappingMiddleware：因为选 A（envelope 必须经 ErrorMappingMiddleware
// 统一产出）之后，DevOnlyMiddleware 本身不再直接写 envelope，脱离 ErrorMappingMiddleware
// 测响应结果无意义。
//
// HTTP status=200 原因：ErrorMappingMiddleware 的 status 决策规则（error_mapping.go:98-103）
// 只有 ErrServiceBusy(1009) 走 500，其它业务码一律 200（业务码与 HTTP status 正交）。
//
// 断言日志：slogtest.Handler 捕获至少一条 level=WARN 的记录，msg 含
// "dev_only middleware rejected"，attrs 含 api_path / method / client_ip。
func TestDevOnlyMiddleware_RejectsWhenDisabled(t *testing.T) {
	t.Setenv("BUILD_DEV", "")

	// 先接管 slog.Default 再构造 engine：保证 middleware 内 logger.FromContext
	// 回落到 slog.Default() 时拿到的是 slogtest 接管的 logger。
	origDefault := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origDefault) })
	h := slogtest.NewHandler(slog.LevelDebug)
	slog.SetDefault(slog.New(h))

	r := newEngine()
	// 挂 ErrorMappingMiddleware：DevOnlyMiddleware 依赖它把 c.Errors 翻成 envelope。
	r.Use(middleware.ErrorMappingMiddleware())
	g := r.Group("/dev")
	g.Use(devtools.DevOnlyMiddleware())
	// 挂一个 noop handler，保证路由存在，middleware 会真正执行。
	g.GET("/foo", func(c *gin.Context) { c.String(http.StatusOK, "should not be reached") })

	w := doGet(r, "/dev/foo")

	// 1. HTTP 层：HTTP 200 + envelope code=1003（ErrorMappingMiddleware 规则）
	assert.Equal(t, http.StatusOK, w.Code, "ErrorMappingMiddleware 对非 1009 的 AppError 走 HTTP 200")
	var env envelope
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "body must be JSON envelope; got=%s", w.Body.String())
	assert.Equal(t, 1003, env.Code, "envelope.code should be 1003 (资源不存在)")
	assert.Equal(t, "资源不存在", env.Message)

	// 2. 日志层：至少一条 WARN 含 "dev_only middleware rejected"
	// （ErrorMappingMiddleware 也会打自己的 WARN "error_mapping"，不影响本匹配）
	records := h.Records()
	var hit *slog.Record
	for i, rec := range records {
		if rec.Level == slog.LevelWarn && strings.Contains(rec.Message, "dev_only middleware rejected") {
			hit = &records[i]
			break
		}
	}
	require.NotNil(t, hit, "expect a WARN log with msg 'dev_only middleware rejected'; got records=%v", records)

	apiPath, ok := slogtest.AttrValue(*hit, "api_path")
	require.True(t, ok, "log should carry api_path attr")
	assert.Equal(t, "/dev/foo", apiPath.String())

	method, ok := slogtest.AttrValue(*hit, "method")
	require.True(t, ok, "log should carry method attr")
	assert.Equal(t, "GET", method.String())

	_, ok = slogtest.AttrValue(*hit, "client_ip")
	assert.True(t, ok, "log should carry client_ip attr")
}

// --- AC7 加分 case 5 -------------------------------------------------------

// TestIsEnabled_EnvVarStrictMatchesOnlyTrue 验证 BUILD_DEV 的严格字面 "true" 语义：
// 只有精确等于 "true" 才视为启用；"1" / "yes" / "TRUE" / 空串 / 未设 均为 false。
//
// 不测试 forceDevEnabled==true 场景（那需要 -tags devtools 重编译；默认 go test
// 下该常量=false，IsEnabled 只由 env 驱动，本 case 结果稳定）。
func TestIsEnabled_EnvVarStrictMatchesOnlyTrue(t *testing.T) {
	cases := []struct {
		name string
		val  string
		want bool
	}{
		{name: "exact true", val: "true", want: true},
		{name: "uppercase TRUE not accepted", val: "TRUE", want: false},
		{name: "mixed-case True not accepted", val: "True", want: false},
		{name: "numeric 1 not accepted", val: "1", want: false},
		{name: "yes not accepted", val: "yes", want: false},
		{name: "false literal", val: "false", want: false},
		{name: "empty string", val: "", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("BUILD_DEV", tc.val)
			assert.Equal(t, tc.want, devtools.IsEnabled(), "IsEnabled() with BUILD_DEV=%q", tc.val)
		})
	}
}

// --- AC7 加分 case 6 -------------------------------------------------------

// TestRegister_WhenDisabled_EmitsNoLogs 验证未启用时 Register 完全静默：
// 不打任何日志，不注册任何路由。
func TestRegister_WhenDisabled_EmitsNoLogs(t *testing.T) {
	t.Setenv("BUILD_DEV", "")

	origDefault := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origDefault) })
	h := slogtest.NewHandler(slog.LevelDebug)
	slog.SetDefault(slog.New(h))

	r := newEngine()
	devtools.Register(r)

	assert.Empty(t, h.Records(), "Register should emit ZERO log records when IsEnabled()==false")
	// 间接验证路由未注册：/dev/ping-dev 返回 Gin NoRoute 404。
	assert.Equal(t, http.StatusNotFound, doGet(r, "/dev/ping-dev").Code)
}

// --- AC7 加分 case 7 -------------------------------------------------------

// TestRegister_WhenEnabled_EmitsExactlyOneWarn 验证启用时 Register 只打一条
// 恰好含 build_tag_devtools / env_build_dev 两个字段的 WARN。
func TestRegister_WhenEnabled_EmitsExactlyOneWarn(t *testing.T) {
	t.Setenv("BUILD_DEV", "true")

	origDefault := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origDefault) })
	h := slogtest.NewHandler(slog.LevelDebug)
	slog.SetDefault(slog.New(h))

	r := newEngine()
	devtools.Register(r)

	records := h.Records()
	require.Len(t, records, 1, "Register should emit exactly one log record")
	rec := records[0]

	assert.Equal(t, slog.LevelWarn, rec.Level, "log level should be WARN")
	assert.Contains(t, rec.Message, "DEV MODE ENABLED", "log msg should be the production warning")

	// env_build_dev 应记录原始 env 值 "true"。
	envBD, ok := slogtest.AttrValue(rec, "env_build_dev")
	require.True(t, ok, "log should carry env_build_dev attr")
	assert.Equal(t, "true", envBD.String())

	// build_tag_devtools 字段必须存在；默认 `go test` 下构建不带 -tags devtools，
	// 故值为 false。真正验证 true 路径依赖 `go test -tags devtools`（手动，AC9）。
	tagVal, ok := slogtest.AttrValue(rec, "build_tag_devtools")
	require.True(t, ok, "log should carry build_tag_devtools attr")
	assert.Equal(t, false, tagVal.Bool())
}
