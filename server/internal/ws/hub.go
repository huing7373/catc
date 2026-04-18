package ws

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rs/zerolog/log"

	"github.com/huing/cat/server/pkg/clockx"
)

type HubConfig struct {
	PingInterval   time.Duration
	PongTimeout    time.Duration
	SendBufSize    int
	MaxConnections int
}

type Hub struct {
	cfg     HubConfig
	clock   clockx.Clock
	clients sync.Map // connID → *Client
	count   atomic.Int64
}

func NewHub(cfg HubConfig, clock clockx.Clock) *Hub {
	if cfg.PingInterval == 0 {
		cfg.PingInterval = 30 * time.Second
	}
	if cfg.PongTimeout == 0 {
		cfg.PongTimeout = 60 * time.Second
	}
	if cfg.SendBufSize == 0 {
		cfg.SendBufSize = 256
	}
	return &Hub{cfg: cfg, clock: clock}
}

func (h *Hub) Name() string { return "ws_hub" }

func (h *Hub) Start(_ context.Context) error { return nil }

func (h *Hub) Final(_ context.Context) error {
	deadline := h.clock.Now().Add(5 * time.Second)
	h.clients.Range(func(_, value any) bool {
		c := value.(*Client)
		_ = c.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutdown"),
			deadline,
		)
		c.conn.Close()
		return true
	})
	return nil
}

func (h *Hub) GoroutineCount() int {
	return int(h.count.Load()) * 2
}

func (h *Hub) Register(client *Client) {
	h.clients.Store(client.connID, client)
	h.count.Add(1)
}

func (h *Hub) Unregister(connID ConnID) {
	if v, loaded := h.clients.LoadAndDelete(connID); loaded {
		c := v.(*Client)
		c.stop()
		h.count.Add(-1)
	}
}

func (h *Hub) unregisterClient(c *Client) {
	if _, loaded := h.clients.LoadAndDelete(c.connID); loaded {
		c.stop()
		c.conn.Close()
		h.count.Add(-1)
	}
}

func (h *Hub) FindByUser(userID UserID) []*Client {
	var result []*Client
	h.clients.Range(func(_, value any) bool {
		c := value.(*Client)
		if c.userID == userID {
			result = append(result, c)
		}
		return true
	})
	return result
}

func (h *Hub) ConnectionCount() int {
	return int(h.count.Load())
}

type Client struct {
	connID     ConnID
	userID     UserID
	conn       *websocket.Conn
	send       chan []byte
	done       chan struct{}
	hub        *Hub
	dispatcher *Dispatcher
	closeOnce  sync.Once
}

func (c *Client) ConnID() ConnID { return c.connID }
func (c *Client) UserID() UserID { return c.userID }

// stop signals the client to shut down. Safe to call from any goroutine, any
// number of times. It closes the done channel (once) which causes writePump to
// drain and exit. The send channel is never closed — publishers simply see
// a full channel or a done signal and silently drop.
func (c *Client) stop() {
	c.closeOnce.Do(func() { close(c.done) })
}

// trySend enqueues msg for delivery. Returns false (silently drops) if the
// client is stopped or the send buffer is full. Because send is never closed,
// this cannot panic.
func (c *Client) trySend(msg []byte) bool {
	select {
	case <-c.done:
		return false
	case c.send <- msg:
		return true
	default:
		return false
	}
}

func (h *Hub) readPump(c *Client) {
	defer func() {
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Str("conn_id", c.connID).Msg("readPump panic recovered")
		}
		h.Unregister(c.connID)
		c.conn.Close()
	}()

	c.conn.SetReadDeadline(h.clock.Now().Add(h.cfg.PongTimeout))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(h.clock.Now().Add(h.cfg.PongTimeout))
		return nil
	})

	limiter := newConnLimiter(100)

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				log.Warn().Err(err).Str("conn_id", c.connID).Msg("ws read error")
			}
			return
		}

		if !limiter.Allow() {
			log.Warn().Str("conn_id", c.connID).Str("user_id", c.userID).Msg("ws rate limit exceeded, closing connection")
			return
		}

		c.dispatcher.Dispatch(context.Background(), c, message)
	}
}

func (h *Hub) writePump(c *Client) {
	ticker := time.NewTicker(h.cfg.PingInterval)
	defer func() {
		if r := recover(); r != nil {
			log.Error().Interface("panic", r).Str("conn_id", c.connID).Msg("writePump panic recovered")
		}
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case <-c.done:
			return
		case msg := <-c.send:
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
