//go:build !devtools

// 本文件假设 forceDevEnabled=false（即 IsEnabled() 仅由 BUILD_DEV 驱动）。
// 带 `-tags devtools` 时 BUILD_DEV="" 下 Register 仍会注册 → case "BUILD_DEV
// empty → 404" 前置破裂；整体文件 build tag `!devtools`，与 devtools_test.go
// 保持一致策略：build-tag 路径由 AC9 手动验证覆盖，不混入自动化测试。

package bootstrap

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// TestRouter_DevPingEnabled_EnvToggle 集成测试 Story 1.6 Dev Tools 框架：
// 同一进程内**两次** NewRouter() 构造 engine，分别在 BUILD_DEV=true / BUILD_DEV=""
// 下访问 /dev/ping-dev：
//   - BUILD_DEV=true → Register 挂 /dev 路由组 → 200 + envelope.data.mode="dev"
//   - BUILD_DEV="" → Register 跳过 → Gin NoRoute → 404（文本 "404 page not found"）
//
// 同时验证 AC7 case 3："dev 启用不影响业务路由" —— BUILD_DEV=true 下 /ping 仍正常。
//
// 本测试替代 epics.md AC "两次启动应用" 的真实进程启动场景：httptest 足够，
// 进程级 env 由 t.Setenv 在子测试间独立管理。
func TestRouter_DevPingEnabled_EnvToggle(t *testing.T) {
	t.Run("BUILD_DEV=true → /dev/ping-dev 200 + envelope.data.mode=dev", func(t *testing.T) {
		t.Setenv("BUILD_DEV", "true")
		gin.SetMode(gin.TestMode)
		r := NewRouter(Deps{})

		req := httptest.NewRequest(http.MethodGet, "/dev/ping-dev", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
		}
		var env struct {
			Code int            `json:"code"`
			Data map[string]any `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
			t.Fatalf("invalid JSON body: %v; body=%s", err, w.Body.String())
		}
		if env.Code != 0 {
			t.Errorf("code = %d, want 0", env.Code)
		}
		if mode, _ := env.Data["mode"].(string); mode != "dev" {
			t.Errorf("data.mode = %v, want 'dev'", env.Data["mode"])
		}

		// AC7 case 3：dev 启用不影响业务路由
		req2 := httptest.NewRequest(http.MethodGet, "/ping", nil)
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)
		if w2.Code != http.StatusOK {
			t.Errorf("/ping status = %d, want 200 when BUILD_DEV=true", w2.Code)
		}
	})

	t.Run("BUILD_DEV empty → /dev/ping-dev 404 (Gin NoRoute)", func(t *testing.T) {
		t.Setenv("BUILD_DEV", "")
		gin.SetMode(gin.TestMode)
		r := NewRouter(Deps{})

		req := httptest.NewRequest(http.MethodGet, "/dev/ping-dev", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotFound {
			t.Fatalf("status = %d, want 404; body=%s", w.Code, w.Body.String())
		}
		// Gin 默认 NoRoute 返回文本 "404 page not found"，**非** envelope。
		if !strings.Contains(w.Body.String(), "404 page not found") {
			t.Errorf("body = %q, want Gin default NoRoute text", w.Body.String())
		}
	})
}

// TestRouter_DevOnlyMiddleware_FallbackPath_LogsCanonicalErrorCode 覆盖 DevOnlyMiddleware
// 的**兜底**触发路径：启动时 BUILD_DEV=true 已把 /dev/* 路由挂上 engine，运行期
// 运维切 BUILD_DEV="" —— 路由仍存在，middleware 在请求期触发 reject。
//
// 本测试是 fix-review P2（DevOnlyMiddleware 绕过 canonical error_code 广播）的防回归：
// 验证在**真实**中间件栈（Logging → ErrorMapping → Recovery → DevOnly）下
//
//	客户端看到的 envelope.code
//	==
//	http_request 日志的 error_code 字段
//
// 始终一致。若 DevOnlyMiddleware 未来再次退化为自己写 envelope（response.Error）
// 而不 Set canonical key，本测试会因日志缺 error_code 红掉。
//
// 见 docs/lessons/2026-04-24-error-envelope-single-producer.md。
func TestRouter_DevOnlyMiddleware_FallbackPath_LogsCanonicalErrorCode(t *testing.T) {
	// Step 1：启动时 BUILD_DEV=true → Register 挂 /dev/* 路由组
	t.Setenv("BUILD_DEV", "true")
	gin.SetMode(gin.TestMode)

	// 捕获 slog：在 NewRouter 前接管，保证 Logging 中间件的 slog.Default() 走 buf
	var buf bytes.Buffer
	origDefault := slog.Default()
	t.Cleanup(func() { slog.SetDefault(origDefault) })
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	r := NewRouter(Deps{})

	// Step 2：运维热切换 BUILD_DEV="" → 路由仍在，但 DevOnlyMiddleware 将在请求期 reject
	t.Setenv("BUILD_DEV", "")

	req := httptest.NewRequest(http.MethodGet, "/dev/ping-dev", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Step 3：HTTP 层断言 —— HTTP 200 + envelope code=1003
	// （ErrorMappingMiddleware 的 status 决策：非 1009 的 AppError → HTTP 200）
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not JSON envelope: %v; body=%s", err, w.Body.String())
	}
	if env.Code != 1003 {
		t.Errorf("envelope.code = %d, want 1003", env.Code)
	}
	if env.Message != "资源不存在" {
		t.Errorf("envelope.message = %q, want '资源不存在'", env.Message)
	}

	// Step 4：日志层断言 —— 找到 msg=http_request 的那条，必须含 error_code=1003
	lines := bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n"))
	var httpReqLog map[string]any
	for _, line := range lines {
		var m map[string]any
		if err := json.Unmarshal(line, &m); err != nil {
			continue
		}
		if msg, _ := m["msg"].(string); msg == "http_request" {
			httpReqLog = m
			break
		}
	}
	if httpReqLog == nil {
		t.Fatalf("未找到 http_request 日志行；buf=%s", buf.String())
	}
	code, ok := httpReqLog["error_code"].(float64)
	if !ok {
		t.Fatalf("http_request 日志缺 error_code 字段（canonical envelope.code 广播契约破坏）；log=%v", httpReqLog)
	}
	if int(code) != 1003 {
		t.Errorf("http_request.error_code = %v, want 1003 （envelope.code 与日志 error_code 必须一致）", code)
	}
}
