package ws

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/pkg/clockx"
)

func TestInMemoryBroadcaster_BroadcastToUser(t *testing.T) {
	t.Parallel()

	hub := NewHub(HubConfig{SendBufSize: 16}, clockx.NewRealClock())
	b := NewInMemoryBroadcaster(hub)

	c1 := &Client{connID: "c1", userID: "alice", send: make(chan []byte, 16)}
	c2 := &Client{connID: "c2", userID: "alice", send: make(chan []byte, 16)}
	c3 := &Client{connID: "c3", userID: "bob", send: make(chan []byte, 16)}

	hub.Register(c1)
	hub.Register(c2)
	hub.Register(c3)

	msg := []byte(`{"type":"friend.state","payload":{}}`)
	err := b.BroadcastToUser(context.Background(), "alice", msg)
	require.NoError(t, err)

	select {
	case got := <-c1.send:
		assert.Equal(t, msg, got)
	default:
		t.Fatal("c1 should receive message")
	}

	select {
	case got := <-c2.send:
		assert.Equal(t, msg, got)
	default:
		t.Fatal("c2 should receive message")
	}

	select {
	case <-c3.send:
		t.Fatal("bob should not receive alice's message")
	default:
	}
}

func TestInMemoryBroadcaster_BroadcastToUser_NoConnections(t *testing.T) {
	t.Parallel()

	hub := NewHub(HubConfig{}, clockx.NewRealClock())
	b := NewInMemoryBroadcaster(hub)

	err := b.BroadcastToUser(context.Background(), "nobody", []byte(`{}`))
	require.NoError(t, err)
}

func TestInMemoryBroadcaster_InterfaceCompliance(t *testing.T) {
	t.Parallel()
	var _ Broadcaster = (*InMemoryBroadcaster)(nil)
}
