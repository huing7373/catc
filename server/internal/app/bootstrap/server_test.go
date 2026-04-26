package bootstrap

import (
	"bytes"
	"context"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/huing/cat/server/internal/infra/config"
)

// captureSlog 暂时把 slog.Default() 重定向到 buf（TextHandler，方便 strings.Contains 断言），
// 返回 cleanup。Story 1.3 把 bootstrap/server.go 从 stdlib log 切到 slog，原来的
// `log.SetOutput(&buf)` 捕获策略失效 —— 这是从 Story 1.2 迁移过来的关键适配。
func captureSlog(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	return &buf, func() { slog.SetDefault(prev) }
}

// TestRun_BindFailureReturnsErrorAndNoStartedBanner 验证 bind 失败时：
//  1. Run 同步返回 error（不在 goroutine 里消失）
//  2. "server started" banner 没有被打印（不出现假阳性启动日志）
//
// 历史背景：未拆分 Listen/Serve 前，log 先于 ListenAndServe 执行，
// 端口占用时会留下 "server started" + 紧跟 "bind error" 的误导日志对。
func TestRun_BindFailureReturnsErrorAndNoStartedBanner(t *testing.T) {
	// Pre-bind on `127.0.0.1:port` —— 必须和 Run 使用的地址族一致，
	// 否则在 Windows 上 127.0.0.1:N 与 0.0.0.0:N 可以共存，bind 不会失败，
	// 测试会悬挂（历史坑：一次踩中就是因为 occupier 和 Run 绑的接口不一致）。
	// 本测试 cfg.Server.BindHost = "127.0.0.1" → Run 也绑 loopback；
	// loopback-only 绑定同时规避 Windows Firewall 对新 hash test 二进制的弹窗。
	occupier, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("pre-bind failed: %v", err)
	}
	defer occupier.Close()
	port := occupier.Addr().(*net.TCPAddr).Port

	buf, restore := captureSlog(t)
	defer restore()

	cfg := &config.Config{
		Server: config.ServerConfig{
			BindHost:        "127.0.0.1",
			HTTPPort:        port,
			ReadTimeoutSec:  1,
			WriteTimeoutSec: 1,
		},
	}

	// 加 2 秒超时保险：bind 失败应该毫秒级返回，
	// 超时触发即说明 Run 没能同步捕获 bind 失败，测试应失败而不是悬挂。
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = Run(ctx, cfg, nil, nil, nil)
	if err == nil {
		t.Fatalf("Run returned nil error, want bind failure on occupied port %d", port)
	}
	if !strings.Contains(err.Error(), "listen") {
		t.Errorf("error %q should mention listen failure", err.Error())
	}
	if strings.Contains(buf.String(), "server started") {
		t.Errorf("bind failure must not emit 'server started' banner; log was:\n%s", buf.String())
	}
}

// TestRun_ShutdownStopsServer 验证 bind 成功后，ctx 取消能触发 graceful shutdown
// 并打印 "server stopped"。同时确认 "server started" banner 在这个 happy path 下会出现。
func TestRun_ShutdownStopsServer(t *testing.T) {
	buf, restore := captureSlog(t)
	defer restore()

	cfg := &config.Config{
		Server: config.ServerConfig{
			BindHost:        "127.0.0.1", // loopback-only：避免 Windows Firewall 弹窗 + OS 分配端口
			HTTPPort:        0,           // 让 OS 分配端口
			ReadTimeoutSec:  1,
			WriteTimeoutSec: 1,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg, nil, nil, nil) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v, want nil after clean shutdown", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("Run did not return within 2s after ctx cancel")
	}

	out := buf.String()
	if !strings.Contains(out, "server started") {
		t.Errorf("happy path should log 'server started'; log was:\n%s", out)
	}
	if !strings.Contains(out, "server stopped") {
		t.Errorf("clean shutdown should log 'server stopped'; log was:\n%s", out)
	}
}
