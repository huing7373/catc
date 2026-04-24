//go:build !devtools

// 本文件假设 forceDevEnabled=false（即 IsEnabled() 仅由 BUILD_DEV 驱动）。
// 带 `-tags devtools` 时 BUILD_DEV="" 下 Register 仍会注册 → case "BUILD_DEV
// empty → 404" 前置破裂；整体文件 build tag `!devtools`，与 devtools_test.go
// 保持一致策略：build-tag 路径由 AC9 手动验证覆盖，不混入自动化测试。

package bootstrap

import (
	"encoding/json"
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
		r := NewRouter()

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
		r := NewRouter()

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
