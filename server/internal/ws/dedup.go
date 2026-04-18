package ws

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/logx"
	"github.com/huing/cat/server/pkg/redisx"
)

// DedupResult is the handler-outcome value cached under an eventId. Aliased
// from pkg/redisx so the internal/ws consumer interface and the pkg/redisx
// implementation refer to the same concrete type (Go uses nominal typing for
// interface parameters — see pkg/redisx/dedup.go for the rationale).
type DedupResult = redisx.DedupResult

// DedupStore is the consumer-side interface (defined in internal/ws per P2) for
// the write-path idempotency cache backing the WS dispatcher. Implementations
// persist the result of the first successful handler invocation for a given
// eventId and return it for subsequent duplicates within the configured TTL.
type DedupStore interface {
	// Acquire attempts to claim exclusive processing rights for eventID via
	// SET event:{eventID} "processing" NX EX ttl. Returns acquired=true on the
	// first attempt; false if an active/completed entry already exists.
	Acquire(ctx context.Context, eventID string) (acquired bool, err error)

	// StoreResult persists the handler's result and transitions the event marker
	// from "processing" to "done" (all with the configured TTL). Must be called
	// after a successful Acquire, regardless of handler success/failure/panic.
	StoreResult(ctx context.Context, eventID string, result DedupResult) error

	// GetResult loads a cached result. found=false if no result hash exists
	// (either never set or expired).
	GetResult(ctx context.Context, eventID string) (result DedupResult, found bool, err error)
}

// dedupMiddleware wraps a HandlerFunc so that the first call for a given
// envelope.ID executes the handler and caches its result; subsequent calls
// with the same ID return the cached response without re-invoking the handler.
//
// clock is used for durationMs measurement to satisfy M9 (no direct time.Now
// in internal/ws business code).
func dedupMiddleware(store DedupStore, clock clockx.Clock, fn HandlerFunc) HandlerFunc {
	return func(ctx context.Context, client *Client, env Envelope) (json.RawMessage, error) {
		if env.ID == "" {
			e := *dto.ErrValidationError
			e.Message = "envelope.id required"
			return nil, &e
		}

		logger := logx.Ctx(ctx)

		// Scope the dedup key by (userId, msgType) so collisions require the
		// same authenticated user replaying the same RPC — clients commonly
		// reuse short per-connection IDs like "1", "2" and different users or
		// different actions must not dedupe against each other.
		scopedID := string(client.userID) + ":" + env.Type + ":" + env.ID

		acquired, err := store.Acquire(ctx, scopedID)
		if err != nil {
			logger.Error().Err(err).
				Str("action", "ws_dedup").
				Str("connId", string(client.connID)).
				Str("userId", string(client.userID)).
				Str("eventId", env.ID).
				Str("msgType", env.Type).
				Msg("dedup store Acquire failed")
			return nil, err
		}

		if !acquired {
			cached, found, getErr := store.GetResult(ctx, scopedID)
			if getErr != nil {
				logger.Error().Err(getErr).
					Str("action", "ws_dedup").
					Str("connId", string(client.connID)).
					Str("userId", string(client.userID)).
					Str("eventId", env.ID).
					Str("msgType", env.Type).
					Msg("dedup store GetResult failed")
				return nil, getErr
			}
			if !found {
				logger.Info().
					Str("action", "ws_dedup").
					Str("connId", string(client.connID)).
					Str("userId", string(client.userID)).
					Str("eventId", env.ID).
					Str("msgType", env.Type).
					Str("outcome", "processing").
					Msg("ws_dedup")
				e := *dto.ErrEventProcessing
				return nil, &e
			}

			logger.Info().
				Str("action", "ws_dedup").
				Str("connId", string(client.connID)).
				Str("userId", string(client.userID)).
				Str("eventId", env.ID).
				Str("msgType", env.Type).
				Str("outcome", "replay").
				Msg("ws_dedup")

			if cached.OK {
				return cached.Payload, nil
			}
			return nil, &dto.AppError{
				Code:    cached.ErrorCode,
				Message: cached.ErrorMessage,
			}
		}

		// First execution — invoke handler and persist result via defer so
		// panics still write a terminal record before re-panicking.
		start := clock.Now()
		var result DedupResult
		var resultErr error

		func() {
			defer func() {
				if r := recover(); r != nil {
					result = DedupResult{
						OK:           false,
						ErrorCode:    "INTERNAL_ERROR",
						ErrorMessage: fmt.Sprintf("handler panic: %v", r),
					}
					if storeErr := store.StoreResult(ctx, scopedID, result); storeErr != nil {
						logger.Error().Err(storeErr).
							Str("action", "ws_dedup").
							Str("eventId", env.ID).
							Msg("dedup StoreResult after panic failed")
					}
					// Re-raise so the outer readPump recovery logs the panic.
					panic(r)
				}
			}()

			payload, err := fn(ctx, client, env)
			if err != nil {
				resultErr = err
				var ae *dto.AppError
				if errors.As(err, &ae) {
					result = DedupResult{OK: false, ErrorCode: ae.Code, ErrorMessage: ae.Message}
				} else {
					result = DedupResult{OK: false, ErrorCode: "INTERNAL_ERROR", ErrorMessage: err.Error()}
				}
				return
			}
			result = DedupResult{OK: true, Payload: payload}
		}()

		if storeErr := store.StoreResult(ctx, scopedID, result); storeErr != nil {
			logger.Error().Err(storeErr).
				Str("action", "ws_dedup").
				Str("eventId", env.ID).
				Msg("dedup StoreResult failed")
		}

		durationMs := clock.Now().Sub(start) / time.Millisecond
		logger.Info().
			Str("action", "ws_dedup").
			Str("connId", string(client.connID)).
			Str("userId", string(client.userID)).
			Str("eventId", env.ID).
			Str("msgType", env.Type).
			Str("outcome", "first").
			Int64("durationMs", int64(durationMs)).
			Msg("ws_dedup")

		if resultErr != nil {
			return nil, resultErr
		}
		return result.Payload, nil
	}
}
