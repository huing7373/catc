package ws

import (
	"context"
	"encoding/json"
	"errors"
	"sort"

	"github.com/rs/zerolog/log"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/clockx"
)

type HandlerFunc func(ctx context.Context, client *Client, env Envelope) (json.RawMessage, error)

type Dispatcher struct {
	handlers   map[string]HandlerFunc
	types      map[string]bool
	dedupStore DedupStore
	clock      clockx.Clock
}

// NewDispatcher constructs a Dispatcher. store may be nil (the dispatcher will
// still accept non-dedup Register calls), but RegisterDedup panics if store is
// nil. clock is required — dedup middleware uses it for durationMs logging
// (M9: no direct time.Now in internal/ws business code).
func NewDispatcher(store DedupStore, clock clockx.Clock) *Dispatcher {
	if clock == nil {
		panic("ws.NewDispatcher: clock is required")
	}
	return &Dispatcher{
		handlers:   make(map[string]HandlerFunc),
		types:      make(map[string]bool),
		dedupStore: store,
		clock:      clock,
	}
}

// Register binds a non-dedup handler (authoritative-read RPCs such as
// users.me, friends.list, session.resume). Panics if msgType is already
// registered via Register or RegisterDedup (prevents configuration drift).
func (d *Dispatcher) Register(msgType string, fn HandlerFunc) {
	if d.types[msgType] {
		panic("ws.Dispatcher: msgType already registered: " + msgType)
	}
	d.types[msgType] = true
	d.handlers[msgType] = fn
}

// RegisterDedup binds an authoritative-write handler wrapped in dedup
// middleware. Required for blindbox.redeem / touch.send / friend.accept /
// friend.delete / friend.block / friend.unblock / skin.equip / profile.update
// per NFR-SEC-9. Panics if the dispatcher was constructed without a store or
// the msgType is already registered.
func (d *Dispatcher) RegisterDedup(msgType string, fn HandlerFunc) {
	if d.dedupStore == nil {
		panic("ws.Dispatcher: RegisterDedup called on dispatcher without DedupStore")
	}
	if d.types[msgType] {
		panic("ws.Dispatcher: msgType already registered: " + msgType)
	}
	d.types[msgType] = true
	d.handlers[msgType] = dedupMiddleware(d.dedupStore, d.clock, fn)
}

// RegisteredTypes returns the sorted list of message types bound to this
// dispatcher via Register or RegisterDedup. The return value is a fresh
// slice — callers may mutate it without affecting dispatcher state.
//
// Used by Story 0.14 AC4 (dto.TestWSMessages_ConsistencyWithDispatcher_*)
// and AC15 (cmd/cat validateRegistryConsistency). Not on the message-handling
// hot path.
//
// Concurrency: reads d.types without locking. Dispatcher registration happens
// exclusively in cmd/cat/initialize.go before any Hub readPump goroutine
// starts; callers MUST invoke RegisteredTypes only after initialize() returns
// (i.e. before Hub.Start). Calling after Hub.Start is undefined.
func (d *Dispatcher) RegisteredTypes() []string {
	out := make([]string, 0, len(d.types))
	for t := range d.types {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func (d *Dispatcher) Dispatch(ctx context.Context, c *Client, raw []byte) {
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		log.Ctx(ctx).Warn().Err(err).Str("conn_id", c.connID).Msg("invalid envelope")
		resp := NewErrorResponse("", "", "VALIDATION_ERROR", "invalid envelope format")
		d.sendResponse(c, resp)
		return
	}

	fn, ok := d.handlers[env.Type]
	if !ok {
		resp := NewErrorResponse(env.ID, env.Type, "UNKNOWN_MESSAGE_TYPE", "unknown message type")
		d.sendResponse(c, resp)
		return
	}

	payload, err := fn(ctx, c, env)
	if err != nil {
		code, message := "INTERNAL_ERROR", err.Error()
		var ae *dto.AppError
		if errors.As(err, &ae) {
			code, message = ae.Code, ae.Message
		}
		resp := NewErrorResponse(env.ID, env.Type, code, message)
		d.sendResponse(c, resp)
		return
	}

	resp := NewAckResponse(env.ID, env.Type, payload)
	d.sendResponse(c, resp)
}

func (d *Dispatcher) sendResponse(c *Client, resp Response) {
	data, err := json.Marshal(resp)
	if err != nil {
		log.Error().Err(err).Str("conn_id", c.connID).Msg("failed to marshal response")
		return
	}
	if !c.trySend(data) {
		c.hub.unregisterClient(c)
	}
}
