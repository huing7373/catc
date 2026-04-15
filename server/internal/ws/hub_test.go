package ws

import (
	"context"
	"testing"
	"time"
)

func TestHub_FinalIdempotent(t *testing.T) {
	h := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() { _ = h.Start(ctx) }()
	// give Start a moment to enter its loop
	time.Sleep(20 * time.Millisecond)

	if err := h.Final(context.Background()); err != nil {
		t.Fatalf("Final #1: %v", err)
	}
	if err := h.Final(context.Background()); err != nil {
		t.Fatalf("Final #2 must be idempotent: %v", err)
	}
}

func TestHub_DeliverToUnknownIsSilent(t *testing.T) {
	h := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = h.Start(ctx) }()
	time.Sleep(20 * time.Millisecond)

	// Delivering to a peer that never connected should queue and be
	// silently discarded by the hub loop without panicking.
	if err := h.Deliver("ghost", []byte(`{"type":"x"}`)); err != nil {
		t.Fatalf("Deliver: %v", err)
	}
	// Allow the hub loop to consume the frame.
	time.Sleep(30 * time.Millisecond)

	if got := h.ClientCount(); got != 0 {
		t.Errorf("ClientCount: %d", got)
	}
	_ = h.Final(context.Background())
}

func TestParseEnvelope(t *testing.T) {
	cases := []struct {
		name  string
		input string
		ok    bool
	}{
		{"valid", `{"type":"ping","payload":{"x":1}}`, true},
		{"missing type", `{"payload":{}}`, false},
		{"bad json", `not-json`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseEnvelope([]byte(tc.input))
			if (err == nil) != tc.ok {
				t.Errorf("parseEnvelope(%q): err=%v wantOK=%v", tc.input, err, tc.ok)
			}
		})
	}
}

func TestRouter_Dispatch(t *testing.T) {
	r := NewRouter()
	got := ""
	r.Handle("ping", func(ctx context.Context, uid string, payload map[string]any) error {
		got = uid + ":" + payload["v"].(string)
		return nil
	})
	err := r.Dispatch(context.Background(), "u-1", Envelope{Type: "ping", Payload: map[string]any{"v": "hi"}})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if got != "u-1:hi" {
		t.Errorf("handler saw: %q", got)
	}

	if err := r.Dispatch(context.Background(), "u", Envelope{Type: "unknown"}); err != ErrUnknownMessageType {
		t.Errorf("unknown type error: %v", err)
	}
}
