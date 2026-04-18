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
		c.closeSend()
		h.count.Add(-1)
	}
}

func (h *Hub) unregisterClient(c *Client) {
	if _, loaded := h.clients.LoadAndDelete(c.connID); loaded {
		c.closeSend()
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
	hub        *Hub
	dispatcher *Dispatcher
	closeOnce  sync.Once
	closed     atomic.Bool
}

func (c *Client) ConnID() ConnID { return c.connID }
func (c *Client) UserID() UserID { return c.userID }

func (c *Client) closeSend() {
	c.closeOnce.Do(func() {
		c.closed.Store(true)
		close(c.send)
	})
}

func (c *Client) trySend(msg []byte) bool {
	if c.closed.Load() {
		return false
	}
	select {
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
		case msg, ok := <-c.send:
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
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
