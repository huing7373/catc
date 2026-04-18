package ws

import (
	"context"
	"encoding/json"

	"github.com/rs/zerolog/log"
)

type HandlerFunc func(ctx context.Context, client *Client, env Envelope) (json.RawMessage, error)

type Dispatcher struct {
	handlers map[string]HandlerFunc
}

func NewDispatcher() *Dispatcher {
	return &Dispatcher{handlers: make(map[string]HandlerFunc)}
}

func (d *Dispatcher) Register(msgType string, fn HandlerFunc) {
	d.handlers[msgType] = fn
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
		resp := NewErrorResponse(env.ID, env.Type, "INTERNAL_ERROR", err.Error())
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
