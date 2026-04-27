//go:build integration
// +build integration

// Story 4.7 AC8 边界 2 集成测试：guestUid 长度 129 → handler 层校验拦截 → 1002。
//
// **位置**：handler 包独立 integration test 文件 —— 与 4.6 auth_handler_test.go 同包
// （package handler_test）但 build tag 隔离（默认 test 不跑；只在 -tags=integration 触发）。
//
// **不起 dockertest 容器**：handler 层校验是纯 Go 内存逻辑（utf8.RuneCountInString），
// 不需要 MySQL；但仍挂 integration build tag 是为对齐 epics.md §Story 4.7 行 1104 钦定
// "全部场景在 integration build tag" 的位置约定（与 ADR-0001 §3.5 兼容 —— integration
// tag 是触发集成范畴的开关，不是必须起容器的硬约束）。
//
// **关键反模式（已规避）**：
//   - **不**在本文件复用 4.6 auth_handler_test.go 的 stubAuthService —— 同 package
//     不同 build tag 编译时两文件都参与编译 → 同名 type 会编译期冲突
//   - **不**用 testify assert（项目沿用 stdlib testing；与 4.5 / 4.6 / 4.8 同模式）
//   - **不**用 c.JSON 直接写 400（V1 §2.4 钦定业务码与 HTTP status 正交）
//
// **断言核心**：service.GuestLogin **不应被调到** —— handler 长度校验提前拦截。
// 通过 stub 的 closure 标志 `serviceCalled bool` 验证（同步无 race）。

package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/handler"
	"github.com/huing/cat/server/internal/app/http/middleware"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// integrationStubAuthService 是 service.AuthService 的 integration test stub。
//
// **命名差异**：4.6 auth_handler_test.go 的 `stubAuthService`（默认编译，无 build tag）
// 与本文件 `integrationStubAuthService`（build tag integration）必须不同名 —— 同 package
// 同 build tag 集合内（即 -tags=integration 编译时）两个文件**都**参与编译 → 同名 type
// 会编译期 redeclared 错误。
type integrationStubAuthService struct {
	guestLoginFn func(ctx context.Context, in service.GuestLoginInput) (*service.GuestLoginOutput, error)
}

func (s *integrationStubAuthService) GuestLogin(ctx context.Context, in service.GuestLoginInput) (*service.GuestLoginOutput, error) {
	return s.guestLoginFn(ctx, in)
}

// TestAuthHandler_GuestLogin_GuestUIDExactly129Chars_Returns1002 (AC8):
// guestUid 长度 129 → handler 层 utf8.RuneCountInString 校验拦截 → envelope.code=1002。
//
// 三层断言：
//  1. HTTP status = 200（V1 §2.4 钦定业务码与 HTTP status 正交，1002 走 200 + envelope.code=1002）
//  2. envelope.code = 1002 + envelope.message 含 "guestUid 长度"（防文案漂移）
//  3. service.GuestLogin **未被调到**（handler 拦截语义验证 —— 失效会让 service
//     在长度溢出场景被打到，DB 可能存到非法长度 guestUid）
func TestAuthHandler_GuestLogin_GuestUIDExactly129Chars_Returns1002(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorMappingMiddleware())

	// stub AuthService：必须**没被调到** —— 否则 handler 长度校验失效
	var serviceCalled bool
	stubSvc := &integrationStubAuthService{
		guestLoginFn: func(ctx context.Context, in service.GuestLoginInput) (*service.GuestLoginOutput, error) {
			serviceCalled = true
			return &service.GuestLoginOutput{}, nil
		},
	}

	h := handler.NewAuthHandler(stubSvc)
	r.POST("/auth/guest-login", h.GuestLogin)

	// 构造 129 字符 guestUid（utf8.RuneCountInString = 129，超过 handler maxLen=128）
	guestUID129 := strings.Repeat("a", 129)
	bodyMap := map[string]any{
		"guestUid": guestUID129,
		"device": map[string]any{
			"platform":    "ios",
			"appVersion":  "1.0.0",
			"deviceModel": "iPhone15,2",
		},
	}
	bodyBytes, err := json.Marshal(bodyMap)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/auth/guest-login", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// 断言 1：HTTP 200（业务码与 HTTP status 正交）
	if w.Code != http.StatusOK {
		t.Errorf("HTTP status = %d, want %d (业务码 1002 走 200，V1 §2.4)", w.Code, http.StatusOK)
	}

	// 解析 envelope
	var resp response.Envelope
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal envelope: %v; body=%s", err, w.Body.String())
	}

	// 断言 2：envelope.code == 1002
	if resp.Code != apperror.ErrInvalidParam {
		t.Errorf("envelope.code = %d, want %d (ErrInvalidParam)", resp.Code, apperror.ErrInvalidParam)
	}

	// 断言 3：envelope.message 含 "guestUid 长度" 关键字（auth_handler.go:102 钦定文案）
	if !strings.Contains(resp.Message, "guestUid 长度") {
		t.Errorf("envelope.message = %q, want containing %q", resp.Message, "guestUid 长度")
	}

	// 断言 4：service 层未被调到（handler 校验提前拦截）—— **核心**断言
	if serviceCalled {
		t.Error("service.GuestLogin 不应被调用 —— handler 长度校验失效")
	}
}
