package logx

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

func TestContextWithRequestID_RoundTrip(t *testing.T) {
	ctx := ContextWithRequestID(context.Background(), "req-abc")
	if got := RequestIDFromContext(ctx); got != "req-abc" {
		t.Fatalf("request id roundtrip: got %q", got)
	}
}

func TestRequestIDFromContext_Missing(t *testing.T) {
	if got := RequestIDFromContext(context.Background()); got != "" {
		t.Fatalf("expected empty request id, got %q", got)
	}
}

func TestWithUserID_LogCtxInherits(t *testing.T) {
	// Redirect zerolog to a buffer and install it as global + context.
	var buf bytes.Buffer
	logger := zerolog.New(&buf)
	log.Logger = logger

	ctx := logger.WithContext(context.Background())
	ctx = ContextWithRequestID(ctx, "req-xyz")
	ctx = WithUserID(ctx, "user-123")

	log.Ctx(ctx).Info().Msg("hello")

	out := buf.String()
	if !strings.Contains(out, `"request_id":"req-xyz"`) {
		t.Errorf("expected request_id in log, got: %s", out)
	}
	if !strings.Contains(out, `"user_id":"user-123"`) {
		t.Errorf("expected user_id in log, got: %s", out)
	}
	if got := UserIDFromContext(ctx); got != "user-123" {
		t.Errorf("UserIDFromContext: got %q", got)
	}
}

func TestInit_Levels(t *testing.T) {
	cases := []struct {
		name   string
		level  string
		expect zerolog.Level
	}{
		{"debug", "debug", zerolog.DebugLevel},
		{"info default", "", zerolog.InfoLevel},
		{"warn", "warn", zerolog.WarnLevel},
		{"warning alias", "warning", zerolog.WarnLevel},
		{"error", "error", zerolog.ErrorLevel},
		{"unknown falls back to info", "xxx", zerolog.InfoLevel},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			Init(Config{Level: tc.level, Format: "json"})
			if got := zerolog.GlobalLevel(); got != tc.expect {
				t.Errorf("level %q → %v, want %v", tc.level, got, tc.expect)
			}
		})
	}
}
