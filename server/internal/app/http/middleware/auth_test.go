package middleware_test

import (
	"encoding/json"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/middleware"
	"github.com/huing/cat/server/internal/pkg/auth"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
)

const testAuthSecret = "test-secret-must-be-at-least-16-bytes"

// newAuthRouter 构造一个挂上 ErrorMappingMiddleware + Auth 的 router 用于
// 测试 envelope。
//
// 中间件链：ErrorMappingMiddleware → captureErrors → Auth → handler。
// captureErrors 在 Auth 之前 → 即使 Auth c.Abort，captureErrors 的 after-c.Next
// 代码段仍会运行（c.Abort 只阻止"下游"中间件，不影响已经在栈上的"after-Next"）。
func newAuthRouter(t *testing.T, signer *auth.Signer) (*gin.Engine, *capturedErrors) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	captured := &capturedErrors{}
	r.Use(middleware.ErrorMappingMiddleware())
	r.Use(func(c *gin.Context) {
		c.Next()
		if len(c.Errors) > 0 {
			captured.firstErr = c.Errors[0].Err
		}
	})
	r.Use(middleware.Auth(signer))
	r.GET("/test", func(c *gin.Context) {
		v, ok := c.Get(middleware.UserIDKey)
		if !ok {
			c.JSON(http.StatusOK, gin.H{"hasUserID": false})
			return
		}
		uid, _ := v.(uint64)
		c.JSON(http.StatusOK, gin.H{"hasUserID": true, "userID": uid})
	})
	return r, captured
}

type capturedErrors struct {
	firstErr error
}

type envelopeBody struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// AC5.1 auth happy: 合法 token → 通过 + userID 注入 c.Get（epics.md 行 1044）
func TestAuth_ValidToken_PassesAndInjectsUserID(t *testing.T) {
	signer, err := auth.New(testAuthSecret, 3600)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	tok, err := signer.Sign(12345, 0)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}

	r, _ := newAuthRouter(t, signer)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var body struct {
		HasUserID bool   `json:"hasUserID"`
		UserID    uint64 `json:"userID"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body not JSON: %v; raw=%s", err, w.Body.String())
	}
	if !body.HasUserID {
		t.Errorf("c.Get(UserIDKey) returned ok=false; expected true")
	}
	if body.UserID != 12345 {
		t.Errorf("UserID = %d, want 12345", body.UserID)
	}
}

// AC5.2 auth edge: 无 Authorization header → 1001（epics.md 行 1045）
func TestAuth_NoAuthorizationHeader_Returns1001(t *testing.T) {
	signer, err := auth.New(testAuthSecret, 3600)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	r, _ := newAuthRouter(t, signer)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (V1 §2.4: 业务码与 HTTP status 正交)", w.Code)
	}
	var env envelopeBody
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not envelope JSON: %v; raw=%s", err, w.Body.String())
	}
	if env.Code != apperror.ErrUnauthorized {
		t.Errorf("envelope.code = %d, want %d (1001)", env.Code, apperror.ErrUnauthorized)
	}
}

// AC5.3 auth edge: 错误 scheme（"Basic" / 无前缀 / 仅 "Bearer " / 空格 / 制表符）→ 1001
func TestAuth_InvalidScheme_Returns1001(t *testing.T) {
	signer, err := auth.New(testAuthSecret, 3600)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}

	cases := []struct {
		name       string
		authHeader string
	}{
		{"basic-scheme", "Basic abc"},
		{"no-prefix-only-token", "raw-token-without-prefix"},
		{"empty-after-bearer", "Bearer "},
		{"only-spaces-after-bearer", "Bearer    "},
		{"lowercase-bearer-with-invalid-token", "bearer xyz"}, // scheme OK，token 解析失败 → 仍 1001
		{"tab-separator", "Bearer\txyz"},                      // tab 不是合法 separator
		{"shorter-than-prefix", "Bea"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r, _ := newAuthRouter(t, signer)
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Authorization", tc.authHeader)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", w.Code)
			}
			var env envelopeBody
			if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
				t.Fatalf("body not envelope JSON: %v; raw=%s", err, w.Body.String())
			}
			if env.Code != apperror.ErrUnauthorized {
				t.Errorf("envelope.code = %d, want 1001 for header=%q", env.Code, tc.authHeader)
			}
		})
	}
}

// AC5.4 auth edge: token 过期 → 1001 + cause 链含 auth.ErrTokenExpired
// （epics.md 行 1046 + 让 future logger 区分日志级别）
func TestAuth_ExpiredToken_Returns1001WithExpiredCause(t *testing.T) {
	t.Parallel()
	signer, err := auth.New(testAuthSecret, 3600)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	tok, err := signer.Sign(1, 1) // 1 秒过期
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	time.Sleep(1100 * time.Millisecond) // 跨秒边界

	r, captured := newAuthRouter(t, signer)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var env envelopeBody
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not envelope JSON: %v; raw=%s", err, w.Body.String())
	}
	if env.Code != apperror.ErrUnauthorized {
		t.Errorf("envelope.code = %d, want 1001", env.Code)
	}
	// cause 链应包含 auth.ErrTokenExpired sentinel（让 future logger 区分级别）
	if captured.firstErr == nil {
		t.Fatalf("captured.firstErr is nil; expected wrapped AppError")
	}
	if !stderrors.Is(captured.firstErr, auth.ErrTokenExpired) {
		t.Errorf("expected cause chain to include auth.ErrTokenExpired, got: %v", captured.firstErr)
	}
	// 同时：apperror.Code 应当是 1001
	if got := apperror.Code(captured.firstErr); got != apperror.ErrUnauthorized {
		t.Errorf("apperror.Code(err) = %d, want %d", got, apperror.ErrUnauthorized)
	}
}

// AC5.5 auth edge: token 篡改 / 格式错 → 1001 + cause 链含 auth.ErrTokenInvalid
//
// 按 docs/lessons/2026-04-26-jwt-tamper-test-must-mutate-non-terminal-byte.md
// 钦定，改 payload 段首字节（**不**改末尾字符；末尾字符可能在 base64url
// 解码上对 padding 不敏感）。
func TestAuth_TamperedToken_Returns1001WithInvalidCause(t *testing.T) {
	signer, err := auth.New(testAuthSecret, 3600)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	tok, err := signer.Sign(1, 3600)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	firstDot := strings.Index(tok, ".")
	if firstDot < 0 {
		t.Fatalf("token format unexpected (no dot): %s", tok)
	}
	payloadStart := firstDot + 1
	if payloadStart >= len(tok) {
		t.Fatalf("token too short to mutate payload: %s", tok)
	}
	original := tok[payloadStart]
	swap := byte('A')
	if original == 'A' {
		swap = 'B'
	}
	tampered := tok[:payloadStart] + string(swap) + tok[payloadStart+1:]

	r, captured := newAuthRouter(t, signer)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("Authorization", "Bearer "+tampered)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var env envelopeBody
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("body not envelope JSON: %v; raw=%s", err, w.Body.String())
	}
	if env.Code != apperror.ErrUnauthorized {
		t.Errorf("envelope.code = %d, want 1001", env.Code)
	}
	if captured.firstErr == nil {
		t.Fatalf("captured.firstErr is nil; expected wrapped AppError")
	}
	if !stderrors.Is(captured.firstErr, auth.ErrTokenInvalid) {
		t.Errorf("expected cause chain to include auth.ErrTokenInvalid, got: %v", captured.firstErr)
	}
}
