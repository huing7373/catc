package ws

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/clockx"
)

func newTestDispatcher(store DedupStore) *Dispatcher {
	return NewDispatcher(store, clockx.NewRealClock())
}

func newTestClient() *Client {
	hub := NewHub(HubConfig{SendBufSize: 16}, clockx.NewRealClock())
	c := &Client{connID: "c1", userID: "u1", send: make(chan []byte, 16), done: make(chan struct{}), hub: hub}
	hub.Register(c)
	return c
}

func TestDispatcher_KnownType(t *testing.T) {
	t.Parallel()

	d := newTestDispatcher(nil)
	d.Register("debug.echo", func(_ context.Context, _ *Client, env Envelope) (json.RawMessage, error) {
		return env.Payload, nil
	})

	c := newTestClient()
	raw := `{"id":"req-1","type":"debug.echo","payload":{"msg":"hello"}}`
	d.Dispatch(context.Background(), c, []byte(raw))

	select {
	case msg := <-c.send:
		var resp Response
		require.NoError(t, json.Unmarshal(msg, &resp))
		assert.True(t, resp.OK)
		assert.Equal(t, "req-1", resp.ID)
		assert.Equal(t, "debug.echo.result", resp.Type)
		assert.JSONEq(t, `{"msg":"hello"}`, string(resp.Payload))
	default:
		t.Fatal("expected response in send channel")
	}
}

func TestDispatcher_UnknownType(t *testing.T) {
	t.Parallel()

	d := newTestDispatcher(nil)
	c := newTestClient()

	raw := `{"id":"req-2","type":"nonexistent.action","payload":{}}`
	d.Dispatch(context.Background(), c, []byte(raw))

	select {
	case msg := <-c.send:
		var resp Response
		require.NoError(t, json.Unmarshal(msg, &resp))
		assert.False(t, resp.OK)
		assert.Equal(t, "req-2", resp.ID)
		assert.Equal(t, "nonexistent.action.result", resp.Type)
		require.NotNil(t, resp.Error)
		assert.Equal(t, "UNKNOWN_MESSAGE_TYPE", resp.Error.Code)
	default:
		t.Fatal("expected error response in send channel")
	}
}

func TestDispatcher_InvalidEnvelope(t *testing.T) {
	t.Parallel()

	d := newTestDispatcher(nil)
	c := newTestClient()

	d.Dispatch(context.Background(), c, []byte(`not-json`))

	select {
	case msg := <-c.send:
		var resp Response
		require.NoError(t, json.Unmarshal(msg, &resp))
		assert.False(t, resp.OK)
		assert.Equal(t, "VALIDATION_ERROR", resp.Error.Code)
	default:
		t.Fatal("expected error response in send channel")
	}
}

func TestDispatcher_RegisterPanicsOnDuplicate(t *testing.T) {
	t.Parallel()

	d := newTestDispatcher(newFakeDedupStore())
	d.Register("dup.type", func(_ context.Context, _ *Client, env Envelope) (json.RawMessage, error) {
		return env.Payload, nil
	})

	assert.PanicsWithValue(t, "ws.Dispatcher: msgType already registered: dup.type", func() {
		d.Register("dup.type", func(_ context.Context, _ *Client, env Envelope) (json.RawMessage, error) {
			return env.Payload, nil
		})
	})
}

func TestDispatcher_RegisterDedupPanicsOnDuplicate(t *testing.T) {
	t.Parallel()

	d := newTestDispatcher(newFakeDedupStore())
	d.Register("both.types", func(_ context.Context, _ *Client, env Envelope) (json.RawMessage, error) {
		return env.Payload, nil
	})

	assert.PanicsWithValue(t, "ws.Dispatcher: msgType already registered: both.types", func() {
		d.RegisterDedup("both.types", func(_ context.Context, _ *Client, env Envelope) (json.RawMessage, error) {
			return env.Payload, nil
		})
	})

	d.RegisterDedup("dedup.type", func(_ context.Context, _ *Client, env Envelope) (json.RawMessage, error) {
		return env.Payload, nil
	})

	assert.Panics(t, func() {
		d.RegisterDedup("dedup.type", func(_ context.Context, _ *Client, env Envelope) (json.RawMessage, error) {
			return env.Payload, nil
		})
	})
}

func TestDispatcher_RegisterDedupPanicsWithoutStore(t *testing.T) {
	t.Parallel()

	d := newTestDispatcher(nil)
	assert.Panics(t, func() {
		d.RegisterDedup("some.type", func(_ context.Context, _ *Client, env Envelope) (json.RawMessage, error) {
			return env.Payload, nil
		})
	})
}

func TestDispatcher_AppErrorCodePropagation(t *testing.T) {
	t.Parallel()

	d := newTestDispatcher(nil)
	d.Register("app.err", func(_ context.Context, _ *Client, _ Envelope) (json.RawMessage, error) {
		return nil, dto.ErrFriendBlocked
	})

	c := newTestClient()
	raw := `{"id":"req-ae","type":"app.err"}`
	d.Dispatch(context.Background(), c, []byte(raw))

	msg := <-c.send
	var resp Response
	require.NoError(t, json.Unmarshal(msg, &resp))
	assert.False(t, resp.OK)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "FRIEND_BLOCKED", resp.Error.Code)
	assert.Equal(t, "user is blocked", resp.Error.Message)
}

func TestDispatcher_NonAppErrorFallsBackToInternalError(t *testing.T) {
	t.Parallel()

	d := newTestDispatcher(nil)
	d.Register("plain.err", func(_ context.Context, _ *Client, _ Envelope) (json.RawMessage, error) {
		return nil, errors.New("boom")
	})

	c := newTestClient()
	d.Dispatch(context.Background(), c, []byte(`{"id":"req-pe","type":"plain.err"}`))

	msg := <-c.send
	var resp Response
	require.NoError(t, json.Unmarshal(msg, &resp))
	assert.False(t, resp.OK)
	require.NotNil(t, resp.Error)
	assert.Equal(t, "INTERNAL_ERROR", resp.Error.Code)
	assert.Equal(t, "boom", resp.Error.Message)
}

func TestDispatcher_RegisteredTypes(t *testing.T) {
	t.Parallel()

	noop := func(_ context.Context, _ *Client, env Envelope) (json.RawMessage, error) {
		return env.Payload, nil
	}

	tests := []struct {
		name   string
		build  func(d *Dispatcher)
		expect []string
	}{
		{
			name:   "empty_dispatcher",
			build:  func(_ *Dispatcher) {},
			expect: []string{},
		},
		{
			name: "single_register",
			build: func(d *Dispatcher) {
				d.Register("debug.echo", noop)
			},
			expect: []string{"debug.echo"},
		},
		{
			name: "register_and_register_dedup_sorted",
			build: func(d *Dispatcher) {
				d.Register("session.resume", noop)
				d.RegisterDedup("debug.echo.dedup", noop)
				d.Register("debug.echo", noop)
			},
			expect: []string{"debug.echo", "debug.echo.dedup", "session.resume"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			d := newTestDispatcher(newFakeDedupStore())
			tt.build(d)

			got := d.RegisteredTypes()
			assert.Equal(t, tt.expect, got)

			// Mutation of the returned slice must not affect internal state.
			if len(got) > 0 {
				got[0] = "mutated.type"
				again := d.RegisteredTypes()
				assert.NotEqual(t, "mutated.type", again[0], "returned slice must be a fresh copy")
			}
		})
	}
}

func TestDispatcher_DedupPath(t *testing.T) {
	t.Parallel()

	store := newFakeDedupStore()
	d := newTestDispatcher(store)

	var callCount int
	d.RegisterDedup("blindbox.redeem", func(_ context.Context, _ *Client, _ Envelope) (json.RawMessage, error) {
		callCount++
		return json.RawMessage(`{"n":1}`), nil
	})

	c := newTestClient()
	raw := `{"id":"evt-1","type":"blindbox.redeem","payload":{}}`

	d.Dispatch(context.Background(), c, []byte(raw))
	d.Dispatch(context.Background(), c, []byte(raw))
	d.Dispatch(context.Background(), c, []byte(raw))

	assert.Equal(t, 1, callCount, "handler should be called exactly once across 3 identical eventIds")

	for range 3 {
		msg := <-c.send
		var resp Response
		require.NoError(t, json.Unmarshal(msg, &resp))
		assert.True(t, resp.OK)
		assert.Equal(t, "evt-1", resp.ID)
		assert.JSONEq(t, `{"n":1}`, string(resp.Payload))
	}
}
