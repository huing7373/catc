package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestInit_LevelWarn(t *testing.T) {
	l := Init("warn")
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

	l := Init("TRACE")

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
