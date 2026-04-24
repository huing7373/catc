package bootstrap

import (
	"bytes"
	"context"
	"log"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/huing/cat/server/internal/infra/config"
)

// TestRun_BindFailureReturnsErrorAndNoStartedBanner 验证 bind 失败时：
//  1. Run 同步返回 error（不在 goroutine 里消失）
//  2. "server started" banner 没有被打印（不出现假阳性启动日志）
//
// 历史背景：未拆分 Listen/Serve 前，log 先于 ListenAndServe 执行，
// 端口占用时会留下 "server started" + 紧跟 "bind error" 的误导日志对。
func TestRun_BindFailureReturnsErrorAndNoStartedBanner(t *testing.T) {
	// Pre-bind on `:port`（0.0.0.0）—— 必须和 Run 使用的地址族一致，
	// 否则在 Windows 上 127.0.0.1:N 与 0.0.0.0:N 可以共存，bind 不会失败，
	// 测试会悬挂（历史坑：一次踩中就是因为 occupier 绑在了 127.0.0.1）。
	occupier, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("pre-bind failed: %v", err)
	}
	defer occupier.Close()
	port := occupier.Addr().(*net.TCPAddr).Port

	var buf bytes.Buffer
	oldOut := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(oldOut)
		log.SetFlags(oldFlags)
	}()

	cfg := &config.Config{
		Server: config.ServerConfig{
			HTTPPort:        port,
			ReadTimeoutSec:  1,
			WriteTimeoutSec: 1,
		},
	}

	// 加 2 秒超时保险：bind 失败应该毫秒级返回，
	// 超时触发即说明 Run 没能同步捕获 bind 失败，测试应失败而不是悬挂。
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err = Run(ctx, cfg)
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
	var buf bytes.Buffer
	oldOut := log.Writer()
	oldFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(oldOut)
		log.SetFlags(oldFlags)
	}()

	cfg := &config.Config{
		Server: config.ServerConfig{
			HTTPPort:        0, // 让 OS 分配端口；为此需用 127.0.0.1:0 的能力
			ReadTimeoutSec:  1,
			WriteTimeoutSec: 1,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- Run(ctx, cfg) }()

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
