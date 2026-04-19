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

// ClientObserver receives notifications when a client disconnects. Used by
// post-connect bookkeeping that lives outside internal/ws (e.g. Story 10.1
// RoomManager removes the user from its in-memory room map; Epic 4.1 Presence
// will implement this interface too).
//
// Implementations MUST be non-blocking and MUST NOT panic — observers fan out
// from Hub.unregisterClient / Hub.Unregister inside the read-pump defer, so a
// slow observer would hold up connection cleanup and a panicking observer
// would crash the read goroutine. All bookkeeping should happen under the
// observer's own lock, not the Hub's.
type ClientObserver interface {
	OnDisconnect(connID ConnID, userID UserID)
}

type Hub struct {
	cfg       HubConfig
	clock     clockx.Clock
	clients   sync.Map // connID → *Client
	count     atomic.Int64
	observers []ClientObserver // set-at-init, read-only at runtime (see AddObserver)
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

// AddObserver subscribes obs to post-disconnect notifications. MUST be called
// during initialize() before Hub.Start / any connection is accepted — the
// observers slice is read without locking on the disconnect path and so must
// be frozen once serving begins. This mirrors Dispatcher.Register's
// init-time-only contract (see dispatcher.go RegisteredTypes godoc).
func (h *Hub) AddObserver(obs ClientObserver) {
	h.observers = append(h.observers, obs)
}

// notifyDisconnect fans out to observers. Called from Unregister and
// unregisterClient after LoadAndDelete succeeds, so at-most-once per client.
func (h *Hub) notifyDisconnect(c *Client) {
	for _, obs := range h.observers {
		obs.OnDisconnect(c.connID, c.userID)
	}
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
		h.notifyDisconnect(c)
	}
}

// unregisterClient removes c from the hub if it is still registered
// and returns whether anything was actually torn down. The bool lets
// concurrent eviction paths (e.g. DisconnectUser racing readPump's
// own unregister-on-defer) attribute their counters honestly — both
// callers may race to LoadAndDelete the same connID, but only the
// winning call performs the close-and-notify side effects, so only
// the winner should count it.
func (h *Hub) unregisterClient(c *Client) bool {
	if _, loaded := h.clients.LoadAndDelete(c.connID); loaded {
		c.stop()
		c.conn.Close()
		h.count.Add(-1)
		h.notifyDisconnect(c)
		return true
	}
	return false
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

// DisconnectUser closes every connection currently held for userID and
// returns the number of connections actually torn down. Safe to call
// when userID has no live connection (returns 0, nil).
//
// Called by Story 1.6 account deletion and any future admin-tool
// revocation: after Story 1.2 RevokeAllUserTokens blacks out the
// refresh jti in Redis, this method is the only way to evict the WS
// session that was opened BEFORE the revocation — WS connections do
// not re-validate the access token on each inbound message (epic line
// 823: access TTL is not enforced mid-connection, fail-open by design).
//
// Known race window (Story 1.3 Dev Notes): FindByUser ranges
// sync.Map, so a connection registered AFTER the Range starts is not
// included in the snapshot. Story 1.6 accepts this — the new
// connection still went through JWT validation at upgrade time, and
// the same revocation flow can re-call DisconnectUser. A future story
// may add an "access jti blacklist" that is checked on every WS
// upgrade to close the gap completely.
//
// Returned error is reserved for unexpected internal state (currently
// always nil); callers may treat it as INTERNAL_ERROR. Per-conn
// close-frame write failures are logged at Warn and swallowed so one
// misbehaving connection does not starve the eviction loop — the
// unregisterClient call still runs and the conn is force-closed
// inside it.
func (h *Hub) DisconnectUser(userID UserID) (int, error) {
	clients := h.FindByUser(userID)
	count := 0
	for _, c := range clients {
		deadline := h.clock.Now().Add(5 * time.Second)
		if err := c.conn.WriteControl(
			websocket.CloseMessage,
			websocket.FormatCloseMessage(websocket.CloseNormalClosure, "revoked"),
			deadline,
		); err != nil {
			log.Warn().
				Err(err).
				Str("action", "ws_disconnect_close_frame_failed").
				Str("userId", userID).
				Str("connId", c.connID).
				Msg("ws_disconnect_close_frame_failed")
		}
		// Only count when this call actually evicted the client.
		// FindByUser produced a snapshot a few lines ago; between
		// then and now the client may have disconnected on its own
		// (readPump's defer unregister) and LoadAndDelete will miss.
		// Bumping count regardless would break the documented
		// contract ("connections actually torn down") and mislead
		// the audit log + any caller that reasons about return.
		if h.unregisterClient(c) {
			count++
		}
	}
	if count > 0 {
		log.Info().
			Str("action", "ws_disconnect_user").
			Str("userId", userID).
			Int("connectionsClosed", count).
			Msg("ws_disconnect_user")
	}
	return count, nil
}

func (h *Hub) ConnectionCount() int {
	return int(h.count.Load())
}

type Client struct {
	connID     ConnID
	userID     UserID
	deviceID   string // Story 1.3 — from jwtx.CustomClaims.DeviceID via JWTValidator
	platform   string // Story 1.3 — from jwtx.CustomClaims.Platform via JWTValidator
	conn       *websocket.Conn
	send       chan []byte
	done       chan struct{}
	hub        *Hub
	dispatcher *Dispatcher
	closeOnce  sync.Once
}

func (c *Client) ConnID() ConnID   { return c.connID }
func (c *Client) UserID() UserID   { return c.userID }
func (c *Client) DeviceID() string { return c.deviceID }
func (c *Client) Platform() string { return c.platform }

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
