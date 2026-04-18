package ws

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/pkg/clockx"
)

func TestHub_RegisterUnregister(t *testing.T) {
	t.Parallel()

	hub := NewHub(HubConfig{SendBufSize: 16}, clockx.NewRealClock())

	c := &Client{
		connID: "conn-1",
		userID: "user-a",
		send:   make(chan []byte, 16),
	}

	hub.Register(c)
	assert.Equal(t, 1, hub.ConnectionCount())
	assert.Equal(t, 2, hub.GoroutineCount())

	hub.Unregister("conn-1")
	assert.Equal(t, 0, hub.ConnectionCount())
	assert.Equal(t, 0, hub.GoroutineCount())
}

func TestHub_UnregisterNonExistent(t *testing.T) {
	t.Parallel()

	hub := NewHub(HubConfig{}, clockx.NewRealClock())
	hub.Unregister("does-not-exist")
	assert.Equal(t, 0, hub.ConnectionCount())
}

func TestHub_FindByUser(t *testing.T) {
	t.Parallel()

	hub := NewHub(HubConfig{SendBufSize: 16}, clockx.NewRealClock())

	c1 := &Client{connID: "c1", userID: "alice", send: make(chan []byte, 16)}
	c2 := &Client{connID: "c2", userID: "bob", send: make(chan []byte, 16)}
	c3 := &Client{connID: "c3", userID: "alice", send: make(chan []byte, 16)}

	hub.Register(c1)
	hub.Register(c2)
	hub.Register(c3)

	aliceConns := hub.FindByUser("alice")
	require.Len(t, aliceConns, 2)

	bobConns := hub.FindByUser("bob")
	require.Len(t, bobConns, 1)
	assert.Equal(t, "c2", bobConns[0].connID)

	noConns := hub.FindByUser("charlie")
	assert.Empty(t, noConns)
}

func TestHub_GoroutineCount(t *testing.T) {
	t.Parallel()

	hub := NewHub(HubConfig{SendBufSize: 16}, clockx.NewRealClock())
	assert.Equal(t, 0, hub.GoroutineCount())

	for i := 0; i < 5; i++ {
		c := &Client{
			connID: ConnID(string(rune('a' + i))),
			userID: "u",
			send:   make(chan []byte, 16),
		}
		hub.Register(c)
	}
	assert.Equal(t, 10, hub.GoroutineCount())
}

func TestHub_Name(t *testing.T) {
	t.Parallel()
	hub := NewHub(HubConfig{}, clockx.NewRealClock())
	assert.Equal(t, "ws_hub", hub.Name())
}
