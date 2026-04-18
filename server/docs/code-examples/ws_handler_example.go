package examples

// This file demonstrates the standard WebSocket message handler pattern.
// Each WS handler follows: parse payload → call service → return ack or error.
//
// Registration (in initialize.go):
//   dispatcher.Register("blindbox.redeem", blindboxHandlers.HandleRedeem)
//
// Handler signature:
//   func(ctx context.Context, client *ws.Client, env ws.Envelope) (json.RawMessage, error)

/*
import (
	"context"
	"encoding/json"
	"errors"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/ws"
)

type BlindboxHandlers struct {
	svc BlindboxService
}

type redeemRequest struct {
	BoxID string `json:"box_id"`
}

func (h *BlindboxHandlers) HandleRedeem(ctx context.Context, client *ws.Client, env ws.Envelope) (json.RawMessage, error) {
	var req redeemRequest
	if err := json.Unmarshal(env.Payload, &req); err != nil {
		return nil, dto.ErrValidationError.WithCause(err)
	}

	result, err := h.svc.Redeem(ctx, client.UserID(), req.BoxID)
	if err != nil {
		var ae *dto.AppError
		if errors.As(err, &ae) {
			return nil, ae
		}
		return nil, dto.ErrInternalError.WithCause(err)
	}

	return json.Marshal(result)
}
*/
