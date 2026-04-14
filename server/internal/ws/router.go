package ws

import (
	"context"
	"encoding/json"
	"errors"
)

// ErrUnknownMessageType is returned for frames whose "type" field does
// not match any registered handler.
var ErrUnknownMessageType = errors.New("ws: unknown message type")

// HandlerFunc processes one inbound message on behalf of a user.
type HandlerFunc func(ctx context.Context, uid string, payload map[string]any) error

// Router dispatches parsed envelopes to per-type handlers. It is safe
// for concurrent reads once populated during startup.
type Router struct {
	handlers map[string]HandlerFunc
}

// NewRouter builds an empty router. Callers register handlers via
// Handle before the hub starts.
func NewRouter() *Router {
	return &Router{handlers: make(map[string]HandlerFunc)}
}

// Handle registers fn under the message type name.
// TODO(#epic-5-5): register touch_send / friend_status handlers here.
func (r *Router) Handle(name string, fn HandlerFunc) {
	r.handlers[name] = fn
}

// Dispatch routes env to the registered handler. It returns
// ErrUnknownMessageType if no handler was registered.
func (r *Router) Dispatch(ctx context.Context, uid string, env Envelope) error {
	h, ok := r.handlers[env.Type]
	if !ok {
		return ErrUnknownMessageType
	}
	return h(ctx, uid, env.Payload)
}

// parseEnvelope is exposed for the readPump; returns an error on invalid
// JSON or a frame whose "type" field is missing.
func parseEnvelope(raw []byte) (Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return env, err
	}
	if env.Type == "" {
		return env, errors.New("ws: missing type")
	}
	return env, nil
}
