package ws

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/pkg/clockx"
)

func TestDispatcher_KnownType(t *testing.T) {
	t.Parallel()

	d := NewDispatcher()
	d.Register("debug.echo", func(_ context.Context, _ *Client, env Envelope) (json.RawMessage, error) {
		return env.Payload, nil
	})

	hub := NewHub(HubConfig{SendBufSize: 16}, clockx.NewRealClock())
	c := &Client{connID: "c1", userID: "u1", send: make(chan []byte, 16), done: make(chan struct{}), hub: hub}
	hub.Register(c)

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

	d := NewDispatcher()
	hub := NewHub(HubConfig{SendBufSize: 16}, clockx.NewRealClock())
	c := &Client{connID: "c1", userID: "u1", send: make(chan []byte, 16), done: make(chan struct{}), hub: hub}
	hub.Register(c)

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

	d := NewDispatcher()
	hub := NewHub(HubConfig{SendBufSize: 16}, clockx.NewRealClock())
	c := &Client{connID: "c1", userID: "u1", send: make(chan []byte, 16), done: make(chan struct{}), hub: hub}
	hub.Register(c)

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
