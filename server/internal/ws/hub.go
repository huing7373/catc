// Package ws hosts the WebSocket hub and per-connection client. It
// provides the plumbing for real-time features (touch, friend-cat
// status) that land in Epic 5; the hub and client skeletons below are
// fully functional while the message surface remains intentionally
// small.
package ws

import (
	"context"
	"errors"
	"sync"

	"github.com/rs/zerolog/log"

	"github.com/huing7373/catc/server/pkg/ids"
)

// Envelope is the JSON shape every WebSocket frame must satisfy.
type Envelope struct {
	Type    string         `json:"type"`
	Payload map[string]any `json:"payload,omitempty"`
}

// Hub owns the set of connected clients and routes messages between
// them.
type Hub struct {
	mu         sync.RWMutex
	clients    map[ids.UserID]*Client
	register   chan *Client
	unregister chan *Client
	broadcast  chan broadcastFrame
	done       chan struct{}
	once       sync.Once
}

type broadcastFrame struct {
	from   ids.UserID
	toUID  ids.UserID
	frame  []byte
}

// NewHub constructs a ready-to-start Hub. Buffers are sized for typical
// small fan-out; Epic 5 load-testing will tune them.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[ids.UserID]*Client),
		register:   make(chan *Client, 32),
		unregister: make(chan *Client, 32),
		broadcast:  make(chan broadcastFrame, 128),
		done:       make(chan struct{}),
	}
}

// Name identifies the hub in graceful-shutdown logs.
func (h *Hub) Name() string { return "ws-hub" }

// Start runs the hub's event loop until ctx is cancelled or Final is
// called. It blocks the caller's goroutine.
func (h *Hub) Start(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-h.done:
			return nil
		case c := <-h.register:
			h.mu.Lock()
			if prev, ok := h.clients[c.uid]; ok {
				prev.closeOnce()
			}
			h.clients[c.uid] = c
			h.mu.Unlock()
		case c := <-h.unregister:
			h.mu.Lock()
			if cur, ok := h.clients[c.uid]; ok && cur == c {
				delete(h.clients, c.uid)
			}
			h.mu.Unlock()
			c.closeOnce()
		case f := <-h.broadcast:
			h.mu.RLock()
			target, ok := h.clients[f.toUID]
			h.mu.RUnlock()
			if !ok {
				continue
			}
			if err := target.enqueue(f.frame); err != nil {
				log.Info().
					Str("to_uid", string(f.toUID)).
					Err(err).
					Msg("ws deliver dropped; closing client")
				h.unregister <- target
			}
		}
	}
}

// Final signals Start to return and closes every tracked client. Safe
// to call multiple times.
func (h *Hub) Final(ctx context.Context) error {
	h.once.Do(func() {
		close(h.done)
		h.mu.Lock()
		for _, c := range h.clients {
			c.closeOnce()
		}
		h.clients = map[ids.UserID]*Client{}
		h.mu.Unlock()
	})
	return nil
}

// Register adds c to the hub. Intended to be called by the HTTP upgrade
// handler after JWT validation.
func (h *Hub) Register(c *Client) error {
	select {
	case h.register <- c:
		return nil
	case <-h.done:
		return errors.New("ws: hub closed")
	}
}

// Unregister removes c from the hub.
func (h *Hub) Unregister(c *Client) {
	select {
	case h.unregister <- c:
	case <-h.done:
	}
}

// Deliver asks the hub to forward frame to the client with the given
// user id. Returns an error if the hub is closed; delivery failure to
// an offline peer is a silent drop.
func (h *Hub) Deliver(toUID ids.UserID, frame []byte) error {
	select {
	case h.broadcast <- broadcastFrame{toUID: toUID, frame: frame}:
		return nil
	case <-h.done:
		return errors.New("ws: hub closed")
	}
}

// ClientCount returns the number of currently tracked clients. Used by
// tests and /admin/metrics in the future.
func (h *Hub) ClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.clients)
}
