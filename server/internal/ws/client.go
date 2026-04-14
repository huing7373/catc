package ws

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/gorilla/websocket"

	"github.com/huing7373/catc/server/pkg/ids"
)

// Client is a per-connection WebSocket actor. The send channel bounds
// the hub's fan-out: if it fills, the hub closes this client rather
// than block.
type Client struct {
	uid       ids.UserID
	conn      *websocket.Conn
	send      chan []byte
	closeOne  sync.Once
	closed    chan struct{}
}

// DefaultSendBuffer is the size of a client's outbound queue.
const DefaultSendBuffer = 32

// NewClient wraps an upgraded WebSocket connection.
func NewClient(uid ids.UserID, conn *websocket.Conn) *Client {
	return &Client{
		uid:    uid,
		conn:   conn,
		send:   make(chan []byte, DefaultSendBuffer),
		closed: make(chan struct{}),
	}
}

// UserID returns the id of the authenticated user on this connection.
func (c *Client) UserID() ids.UserID { return c.uid }

// enqueue tries to buffer frame for sending. Returns an error if the
// buffer is full or the client is already closed — the hub then treats
// the peer as gone.
func (c *Client) enqueue(frame []byte) error {
	select {
	case <-c.closed:
		return errors.New("ws: client closed")
	case c.send <- frame:
		return nil
	default:
		return errors.New("ws: client send buffer full")
	}
}

// closeOnce tears down the connection exactly once.
func (c *Client) closeOnce() {
	c.closeOne.Do(func() {
		close(c.closed)
		_ = c.conn.Close()
	})
}

// readPump pumps messages from the WebSocket connection into handle. It
// returns when the peer closes, a timeout fires, or ctx is cancelled.
//
// This skeleton is wired correctly; Epic 5 fills in the message
// dispatch to the service layer.
func (c *Client) ReadPump(ctx context.Context, handle func(context.Context, Envelope) error) {
	defer c.closeOnce()
	c.conn.SetReadLimit(1 << 16)
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	})
	for {
		if err := ctx.Err(); err != nil {
			return
		}
		mt, payload, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		if mt != websocket.TextMessage {
			continue
		}
		env, err := parseEnvelope(payload)
		if err != nil {
			continue
		}
		if handle != nil {
			if err := handle(ctx, env); err != nil {
				return
			}
		}
	}
}

// WritePump drains the send channel to the WebSocket, emitting pings
// every 30 seconds. It returns when the channel closes or the
// connection errors.
func (c *Client) WritePump(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.closeOnce()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-c.closed:
			return
		case msg, ok := <-c.send:
			if !ok {
				return
			}
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
