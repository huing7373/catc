// Package push — Pusher façade + Redis Streams enqueue (Story 0.13).
//
// # Responsibility
//
// Pusher is the single entry-point consumed by every service that sends
// APNs notifications (touch_service in Story 5.2 / blindbox_service in
// 6.2 / cold_start_recall_job in 8.2 / profile + account flows in 1.5).
// A call to Enqueue is fire-and-forget: it persists the intent to Redis
// Streams and returns immediately — the APNs send happens out-of-band in
// APNsWorker.
//
// # Interface co-location (P2 shape)
//
// Unlike Story 0.12's ResumeCache (defined in internal/ws because only WS
// consumes it), Pusher has fan-in from service + cron + (future) WS
// handlers. The interface lives in internal/push — the same package as
// its primary impl — because **this package IS the push abstraction**,
// analogous to io.Reader in io. Consumers import internal/push and see
// {Pusher, PushPayload, PushKind} as a single API surface; they do not
// pull in Redis internals.
//
// # Fail-open vs fail-closed (AC11)
//
// | concern                        | mode        | reason                     |
// |--------------------------------|-------------|----------------------------|
// | Enqueue XADD error             | fail-closed | caller must know          |
// | Enqueue idempotency SETNX err  | fail-open   | duplicate < lost          |
// | Enqueue validation error       | fail-closed | wrong payload shape       |
//
// # IDs / FRs / NFRs / Ds / Ps / Ms referenced
//
//	FR27 (touch offline fallback) / FR30 (quiet hours) / FR43 (410 cleanup)
//	FR44b (cold-start recall)     / FR58 (platform topic routing)
//	NFR-REL-5 (fire-and-forget)   / NFR-REL-8 (exp backoff + 410)
//	NFR-SEC-7 (token encryption)  / NFR-SEC-9 (idempotency 5 min)
//	NFR-COMP-3 (background push)  / NFR-INT-2 (sideshow/apns2)
//	D3 (APNs queue) / D16 (Redis key space)
//	P2 (consumer-side interface) / P4 (error category) / P5 (camelCase logs)
//	M9 (Clock) / M13 (PII) / M14 (APNs token mask)
package push

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/ids"
	"github.com/huing/cat/server/pkg/logx"
	"github.com/huing/cat/server/pkg/redisx"
)

// PushKind enumerates the two APNs delivery modes.
//
//	PushKindAlert  → apns-push-type: alert     (visible banner/sound)
//	PushKindSilent → apns-push-type: background (content-available=1)
//
// Kept as a typed string for compile-time checks on Enqueue callers and
// for stable JSON values when the payload is serialised into the queue.
type PushKind string

const (
	PushKindAlert  PushKind = "alert"
	PushKindSilent PushKind = "silent"
)

// PushPayload is the caller-constructed descriptor of a single logical
// notification (one user, many devices after router fan-out). Every field
// is exported because service-layer code builds instances directly.
//
// Validation at Enqueue time:
//   - Kind must be PushKindAlert or PushKindSilent.
//   - When Kind == Alert, Title must be non-empty (empty-alert is an
//     APNs protocol violation — AC1).
type PushPayload struct {
	// Kind selects alert vs silent delivery. Required.
	Kind PushKind

	// Title is shown as the notification headline. Required for alert,
	// optional (ignored) for silent.
	Title string

	// Body is the secondary line of alert text. Always optional.
	Body string

	// DeepLink is a URL passed in the APS custom payload so the client
	// can route on tap (e.g. "cat://touch?from=<userId>"). Optional.
	DeepLink string

	// RespectsQuietHours requests that the worker coerce Kind to
	// PushKindSilent at consume time if the receiver is currently inside
	// their configured quiet window. FR30 touch / FR44b recall set this
	// to true; "account security alert"-style flows leave it false.
	RespectsQuietHours bool

	// IdempotencyKey, when non-empty, gates enqueue on a SETNX of
	// apns:idem:{key} with 5-minute TTL (NFR-SEC-9). Two Enqueues within
	// the window collapse into one XADD. Callers typically embed the
	// business event ID (eventId / envelopeId / touchId).
	IdempotencyKey string
}

// queueMessage is the JSON envelope pushed into the Redis Stream. Private
// — it is not an API surface, only a serialisation detail shared between
// Pusher and APNsWorker (same package).
type queueMessage struct {
	UserID       ids.UserID  `json:"userId"`
	Payload      PushPayload `json:"payload"`
	Attempt      int         `json:"attempt"`
	EnqueuedAtMs int64       `json:"enqueuedAtMs"`
}

// Pusher is the service-layer seam for APNs. See package docs for why the
// interface lives in internal/push.
type Pusher interface {
	Enqueue(ctx context.Context, userID ids.UserID, p PushPayload) error
}

// RedisStreamsPusher is the production Pusher, backed by a Redis Streams
// entry plus an optional SETNX idempotency gate.
type RedisStreamsPusher struct {
	stream  *redisx.StreamPusher
	idemCmd redis.Cmdable
	clock   clockx.Clock
	idemTTL time.Duration
}

// NewRedisStreamsPusher constructs a RedisStreamsPusher. Panics on any
// invalid argument — startup invariants, not runtime conditions. There is
// no "disabled" mode here: operators disable push by setting
// cfg.APNs.Enabled = false, which routes cmd/cat/initialize.go to
// NoopPusher instead.
func NewRedisStreamsPusher(
	stream *redisx.StreamPusher,
	idemCmd redis.Cmdable,
	clock clockx.Clock,
	idemTTL time.Duration,
) *RedisStreamsPusher {
	if stream == nil {
		panic("push.NewRedisStreamsPusher: stream must not be nil")
	}
	if idemCmd == nil {
		panic("push.NewRedisStreamsPusher: idemCmd must not be nil")
	}
	if clock == nil {
		panic("push.NewRedisStreamsPusher: clock must not be nil")
	}
	if idemTTL <= 0 {
		panic("push.NewRedisStreamsPusher: idemTTL must be > 0")
	}
	return &RedisStreamsPusher{
		stream:  stream,
		idemCmd: idemCmd,
		clock:   clock,
		idemTTL: idemTTL,
	}
}

// Enqueue validates the payload, enforces idempotency when a key is
// provided, then XADDs the queueMessage to the stream. Fire-and-forget:
// no waiting for APNs (D3, NFR-REL-5).
func (p *RedisStreamsPusher) Enqueue(ctx context.Context, userID ids.UserID, payload PushPayload) error {
	if payload.Kind != PushKindAlert && payload.Kind != PushKindSilent {
		return dto.ErrValidationError.WithCause(errors.New("push: invalid Kind"))
	}
	if payload.Kind == PushKindAlert && payload.Title == "" {
		return dto.ErrValidationError.WithCause(errors.New("push: alert requires Title"))
	}

	maskedIdem := ""
	if payload.IdempotencyKey != "" {
		maskedIdem = logx.MaskPII(payload.IdempotencyKey)
		key := "apns:idem:" + payload.IdempotencyKey
		ok, err := p.idemCmd.SetNX(ctx, key, "1", p.idemTTL).Result()
		if err != nil {
			// Fail-open: duplicate push is annoying, lost push is worse.
			log.Warn().
				Err(err).
				Str("action", "apns_enqueue_idem_error").
				Str("userId", string(userID)).
				Str("idemKey", maskedIdem).
				Msg("idempotency SETNX failed; continuing to XADD")
		} else if !ok {
			log.Info().
				Str("action", "apns_enqueue_idem_dedup").
				Str("userId", string(userID)).
				Str("idemKey", maskedIdem).
				Msg("duplicate enqueue suppressed by idempotency key")
			return nil
		}
	}

	qm := queueMessage{
		UserID:       userID,
		Payload:      payload,
		Attempt:      0,
		EnqueuedAtMs: p.clock.Now().UnixMilli(),
	}
	raw, err := json.Marshal(qm)
	if err != nil {
		// Marshal failure on exported-only fields is a programming error.
		return dto.ErrInternalError.WithCause(err)
	}

	id, err := p.stream.XAdd(ctx, map[string]string{
		"userId":  string(userID),
		"msg":     string(raw),
		"attempt": "0",
	})
	if err != nil {
		return dto.ErrInternalError.WithCause(err)
	}

	log.Info().
		Str("action", "apns_enqueue").
		Str("userId", string(userID)).
		Str("kind", string(payload.Kind)).
		Str("idemKey", maskedIdem).
		Str("streamId", id).
		Msg("apns enqueue ok")
	return nil
}

// NoopPusher satisfies Pusher without performing any I/O. Used in
// cmd/cat/initialize.go when cfg.APNs.Enabled = false so downstream
// services always receive a non-nil Pusher and can call Enqueue without
// a nil-check.
type NoopPusher struct{}

// Enqueue returns nil and does nothing.
func (NoopPusher) Enqueue(context.Context, ids.UserID, PushPayload) error { return nil }
