package middleware_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/middleware"
)

// TestCtxPropagation_HandlerSeesCancelViaCRequestContext：
//
// in-process 路径：构造一个已 cancel 的 ctx，通过 req.WithContext 注入，
// engine.ServeHTTP(w, req) 后断言 handler 看到的 c.Request.Context().Err()
// 非 nil（ctx 已 cancel），且 Logging 中间件最终 log 中含 ctx_done=true。
//
// 这条 case 验证 ADR-0007 §3 cancel 协作图的最短闭环："client ctx
// → req.Context() → handler c.Request.Context()"。
//
// Story 1.9 AC5 Case 1。
func TestCtxPropagation_HandlerSeesCancelViaCRequestContext(t *testing.T) {
	gin.SetMode(gin.TestMode)

	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })

	handlerSawCancel := false
	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Logging())
	r.Use(middleware.ErrorMappingMiddleware())
	r.Use(middleware.Recovery())
	r.GET("/cancel-me", func(c *gin.Context) {
		// handler 应该看到 ctx 已 cancel（req.Context 已被 cancel）
		if err := c.Request.Context().Err(); err != nil {
			handlerSawCancel = true
		}
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // 立即 cancel

	req := httptest.NewRequest(http.MethodGet, "/cancel-me", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	start := time.Now()
	r.ServeHTTP(w, req)
	elapsed := time.Since(start)

	if elapsed > 200*time.Millisecond {
		t.Errorf("elapsed = %v, want < 200ms（in-process 链路应秒级）", elapsed)
	}
	if !handlerSawCancel {
		t.Errorf("handler 未看到 ctx cancel（c.Request.Context().Err() == nil）；ctx 未正确传递到 handler")
	}

	// 断言最后一行 http_request log 含 ctx_done=true
	m := parseLastLogLineFromBuf(t, &buf)
	if m["msg"] != "http_request" {
		t.Fatalf("expected http_request log; got msg=%v", m["msg"])
	}
	got, ok := m["ctx_done"].(bool)
	if !ok {
		t.Fatalf("ctx_done 字段缺失或非 bool；实际 log: %v", m)
	}
	if !got {
		t.Errorf("ctx_done = false, want true")
	}
}

// TestCtxPropagation_SlowHandlerCanceledByClientDisconnect：
//
// 真 HTTP roundtrip 路径：httptest.NewServer 启真 TCP server；handler 故意
// select { case <-c.Request.Context().Done(): return ... case <-time.After(5s): ... }；
// client 端用 context.WithTimeout(100ms) + client.Do(req) → 断言：
//   - client 侧：err 满足 errors.Is(err, context.DeadlineExceeded)
//   - server 侧：handler 在 500ms 内退出（goroutine 没 hang 5s）
//
// 这条 case 验证 ADR-0007 §3 "client 断开 → Go net/http cancel ctx → handler
// 感知 ctx.Done() → goroutine 立即退出" 完整链路。
//
// Story 1.9 AC5 Case 2。
func TestCtxPropagation_SlowHandlerCanceledByClientDisconnect(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handlerExited := make(chan time.Duration, 1)

	r := gin.New()
	r.Use(middleware.RequestID())
	r.Use(middleware.Logging())
	r.Use(middleware.ErrorMappingMiddleware())
	r.Use(middleware.Recovery())
	r.GET("/slow", func(c *gin.Context) {
		start := time.Now()
		defer func() { handlerExited <- time.Since(start) }()

		select {
		case <-c.Request.Context().Done():
			// 正确路径：ctx cancel 时立即退出
			return
		case <-time.After(5 * time.Second):
			// 错误路径：ctx 没传过来，handler hang 5s
			c.JSON(http.StatusOK, gin.H{"ok": true})
			return
		}
	})

	srv := httptest.NewServer(r)
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL+"/slow", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}

	start := time.Now()
	resp, err := http.DefaultClient.Do(req)
	clientElapsed := time.Since(start)
	if resp != nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}

	// client 侧断言：应收到 ctx deadline error
	if err == nil {
		t.Fatalf("expected client error, got nil（resp=%v）", resp)
	}
	// net/http 包会把 ctx error 包成 url.Error，errors.Is 穿透
	// （err 可能是 "context deadline exceeded" 也可能是 "request canceled"）
	if clientElapsed > 500*time.Millisecond {
		t.Errorf("client elapsed = %v, want < 500ms", clientElapsed)
	}

	// server 侧断言：handler goroutine 500ms 内退出（证明 ctx cancel 穿到了 handler）
	select {
	case elapsed := <-handlerExited:
		if elapsed > 500*time.Millisecond {
			t.Errorf("handler goroutine elapsed = %v, want < 500ms（ctx cancel 应立即唤醒 handler 的 select）", elapsed)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("handler goroutine 未在 2s 内退出；ctx cancel 未传到 handler")
	}
}

// parseLastLogLineFromBuf 从 *bytes.Buffer 读最后一行 JSON log。
// 与 logging_test.go 的 parseLastLogLine 行为一致，但本文件在 _test 包，
// 无法直接复用，简单复制一份（工具级函数，不值得抽 helper 包）。
func parseLastLogLineFromBuf(t *testing.T, buf *bytes.Buffer) map[string]any {
	t.Helper()
	lines := bytes.Split(bytes.TrimRight(buf.Bytes(), "\n"), []byte("\n"))
	if len(lines) == 0 {
		t.Fatalf("log buffer empty")
	}
	var m map[string]any
	if err := json.Unmarshal(lines[len(lines)-1], &m); err != nil {
		t.Fatalf("last log line not JSON: %v; raw=%s", err, string(lines[len(lines)-1]))
	}
	return m
}
