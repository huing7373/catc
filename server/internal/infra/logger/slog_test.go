package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInit_LevelWarn(t *testing.T) {
	l := Init("warn", "")
	if l == nil {
		t.Fatal("Init returned nil logger")
	}
	if l.Enabled(context.Background(), slog.LevelInfo) {
		t.Errorf("Info should be disabled at level=warn")
	}
	if !l.Enabled(context.Background(), slog.LevelWarn) {
		t.Errorf("Warn should be enabled at level=warn")
	}
}

func TestInit_InvalidLevelFallsBackToInfo(t *testing.T) {
	var buf bytes.Buffer
	oldDefault := slog.Default()
	defer slog.SetDefault(oldDefault)

	// Init 会把 fallback 通过 slog.Default().Warn 吐出；我们先把 default 换成 buffer handler 捕获。
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))

	l := Init("TRACE", "")

	if !l.Enabled(context.Background(), slog.LevelInfo) {
		t.Errorf("Info should be enabled after fallback to info level")
	}

	lvl, ok := parseLevel("TRACE")
	if ok {
		t.Errorf("parseLevel(TRACE) should return ok=false")
	}
	if lvl != slog.LevelInfo {
		t.Errorf("parseLevel(TRACE) level = %v, want Info", lvl)
	}
}

// TestInit_FilePathWritesToFile 验证传入 filePath 时，logger 同时写 stdout + 文件。
// 用 t.TempDir() 拿到隔离目录避免污染本机；测试结束 t.Cleanup 自动清理。
func TestInit_FilePathWritesToFile(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "server.log")

	l := Init("info", logPath)
	l.Info("hello-from-file-test", slog.String("k", "v"))

	// 关键：必须显式 close 文件让 buffered write flush 到磁盘
	// （Init 内部把 file 留在 currentFile，下次 Init 才 close。这里我们用空 path 触发 close）
	_ = Init("info", "")

	raw, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file failed: %v", err)
	}
	if !bytes.Contains(raw, []byte("hello-from-file-test")) {
		t.Errorf("log file does not contain expected message; raw=%q", raw)
	}
	// 确认是 JSON 格式
	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(raw), &m); err != nil {
		t.Fatalf("log file content is not valid JSON: %v; raw=%s", err, raw)
	}
	if m["msg"] != "hello-from-file-test" {
		t.Errorf("log file msg = %v, want hello-from-file-test", m["msg"])
	}
}

// TestInit_FilePathOpenFailureFallsBackToStdout 验证文件路径无效（目录不存在）时
// logger 退化为只写 stdout 不阻断启动。fail-soft 语义：日志落盘是辅助能力，
// 路径错配不应让 server 起不来。
func TestInit_FilePathOpenFailureFallsBackToStdout(t *testing.T) {
	// 不存在的目录路径 → OpenFile 必失败
	bogusPath := filepath.Join(t.TempDir(), "nonexistent-dir", "server.log")

	l := Init("info", bogusPath)
	if l == nil {
		t.Fatal("Init returned nil logger on file open failure (should fall back to stdout-only)")
	}
	if !l.Enabled(context.Background(), slog.LevelInfo) {
		t.Errorf("logger should still be functional at info level after file fallback")
	}

	// cleanup: 重置 currentFile 状态
	_ = Init("info", "")
}

func TestNewContext_FromContext_Roundtrip(t *testing.T) {
	var buf bytes.Buffer
	child := slog.New(slog.NewJSONHandler(&buf, nil)).With("request_id", "abc-123")

	ctx := NewContext(context.Background(), child)
	got := FromContext(ctx)

	if got == nil {
		t.Fatal("FromContext returned nil")
	}

	got.Info("hello")

	var m map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &m); err != nil {
		t.Fatalf("log output is not valid JSON: %v; raw=%s", err, buf.String())
	}
	if m["request_id"] != "abc-123" {
		t.Errorf("request_id = %v, want %q (child logger lost through ctx)", m["request_id"], "abc-123")
	}
}

func TestFromContext_FallbackToDefault(t *testing.T) {
	got := FromContext(context.Background())
	if got == nil {
		t.Fatal("FromContext on empty ctx returned nil")
	}
	// 对比指针或用 Enabled 断言都可。这里断言"能正常调用"即可。
	got.Info("no panic")
}

func TestParseLevel_Table(t *testing.T) {
	cases := []struct {
		in     string
		want   slog.Level
		wantOk bool
	}{
		{"debug", slog.LevelDebug, true},
		{"INFO", slog.LevelInfo, true},
		{"Warn", slog.LevelWarn, true},
		{"warning", slog.LevelWarn, true},
		{"error", slog.LevelError, true},
		{"", slog.LevelInfo, true},
		{"bogus", slog.LevelInfo, false},
	}
	for _, c := range cases {
		lvl, ok := parseLevel(c.in)
		if lvl != c.want || ok != c.wantOk {
			t.Errorf("parseLevel(%q) = (%v, %v), want (%v, %v)", c.in, lvl, ok, c.want, c.wantOk)
		}
	}
	_ = strings.TrimSpace // keep import
}
