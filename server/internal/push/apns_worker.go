// Package push — APNs worker Runnable (AC7–AC10).
//
// The worker is the async consumer of apns:queue. It is a Runnable
// (Name / Start / Final) so it hangs off cmd/cat/app.go's graceful
// shutdown chain, and a single process runs `WorkerCount` consumer
// goroutines plus one retry-promoter goroutine, all keyed to the same
// apns2.Client (HTTP/2-safe for concurrent use).
//
// The core state machine per message (handle):
//
//  1. decode queueMessage; malformed → DLQ.
//  2. quiet-hours resolve: when Payload.RespectsQuietHours, coerce alert
//     → silent if receiver is inside their quiet window (fail-open on
//     resolver error — deliver the alert).
//  3. router.RouteTokens: fan out into per-device RoutedToken. No tokens
//     → ACK + info log.
//  4. per-token Send:
//     - 200                  → log ok, keep message non-retryable for this token.
//     - 410                  → delete token via TokenDeleter; do NOT retry.
//     - 4xx non-410 / bad    → log error; permanent; do NOT retry.
//     - 429 / 5xx / transport → mark retryable.
//  5. Retry decision:
//     - any token retryable AND attempt+1 < MaxAttempts → scheduleRetry
//       (ZADD apns:retry dueAt) + XACK queue.
//     - any token retryable AND attempt+1 == MaxAttempts → xaddDLQ + XACK.
//     - no tokens retryable → XACK (retry wouldn't help).
//
// See AC11 for the complete fail-open / fail-closed matrix.
package push

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/sideshow/apns2"
	"github.com/sideshow/apns2/payload"

	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/logx"
	"github.com/huing/cat/server/pkg/redisx"
)

// APNsWorkerConfig is construction-time config (values already validated
// by config.mustValidate — applyDefaults ensures zero-fills here are
// belt-and-braces rather than load-bearing).
type APNsWorkerConfig struct {
	InstanceID      string
	StreamKey       string
	DLQKey          string
	RetryZSetKey    string
	ConsumerGroup   string
	WorkerCount     int
	ReadBlock       time.Duration
	ReadCount       int64
	RetryBackoffsMs []int
	MaxAttempts     int
}

// APNsWorker consumes apns:queue + promotes due retries.
type APNsWorker struct {
	cfg       APNsWorkerConfig
	streamCmd redis.Cmdable
	sender    ApnsSender
	router    *APNsRouter
	quiet     QuietHoursResolver
	deleter   TokenDeleter
	clock     clockx.Clock

	wg     sync.WaitGroup
	cancel context.CancelFunc
}

// NewAPNsWorker panics on any nil dependency or invalid numeric config —
// startup invariants. The expected "off" state is cfg.APNs.Enabled=false
// which short-circuits construction entirely in initialize.go.
func NewAPNsWorker(
	cfg APNsWorkerConfig,
	streamCmd redis.Cmdable,
	sender ApnsSender,
	router *APNsRouter,
	quiet QuietHoursResolver,
	deleter TokenDeleter,
	clock clockx.Clock,
) *APNsWorker {
	if streamCmd == nil {
		panic("push.NewAPNsWorker: streamCmd must not be nil")
	}
	if sender == nil {
		panic("push.NewAPNsWorker: sender must not be nil")
	}
	if router == nil {
		panic("push.NewAPNsWorker: router must not be nil")
	}
	if quiet == nil {
		panic("push.NewAPNsWorker: quiet must not be nil")
	}
	if deleter == nil {
		panic("push.NewAPNsWorker: deleter must not be nil")
	}
	if clock == nil {
		panic("push.NewAPNsWorker: clock must not be nil")
	}
	if cfg.InstanceID == "" {
		panic("push.NewAPNsWorker: cfg.InstanceID must not be empty")
	}
	if cfg.StreamKey == "" || cfg.DLQKey == "" || cfg.RetryZSetKey == "" || cfg.ConsumerGroup == "" {
		panic("push.NewAPNsWorker: cfg stream/dlq/retry/group keys must not be empty")
	}
	if cfg.WorkerCount <= 0 {
		panic("push.NewAPNsWorker: cfg.WorkerCount must be > 0")
	}
	if cfg.ReadBlock <= 0 {
		panic("push.NewAPNsWorker: cfg.ReadBlock must be > 0")
	}
	if cfg.ReadCount <= 0 {
		panic("push.NewAPNsWorker: cfg.ReadCount must be > 0")
	}
	if len(cfg.RetryBackoffsMs) == 0 {
		panic("push.NewAPNsWorker: cfg.RetryBackoffsMs must be non-empty")
	}
	if cfg.MaxAttempts <= 0 {
		panic("push.NewAPNsWorker: cfg.MaxAttempts must be > 0")
	}
	return &APNsWorker{
		cfg:       cfg,
		streamCmd: streamCmd,
		sender:    sender,
		router:    router,
		quiet:     quiet,
		deleter:   deleter,
		clock:     clock,
	}
}

// Name satisfies Runnable.
func (w *APNsWorker) Name() string { return "apns_worker" }

// Start spawns WorkerCount consumer loops plus one retry-promoter
// goroutine. EnsureGroup is executed synchronously so any Redis-side
// misconfiguration surfaces as a Start error, failing App.Run cleanly.
func (w *APNsWorker) Start(ctx context.Context) error {
	workerCtx, cancel := context.WithCancel(ctx)
	w.cancel = cancel

	seed := redisx.NewStreamConsumer(
		w.streamCmd, w.cfg.StreamKey, w.cfg.ConsumerGroup,
		w.cfg.InstanceID+"-seed", w.cfg.ReadBlock, w.cfg.ReadCount,
	)
	if err := seed.EnsureGroup(workerCtx); err != nil {
		cancel()
		return err
	}

	for i := 0; i < w.cfg.WorkerCount; i++ {
		consumer := redisx.NewStreamConsumer(
			w.streamCmd, w.cfg.StreamKey, w.cfg.ConsumerGroup,
			w.cfg.InstanceID+"-"+strconv.Itoa(i),
			w.cfg.ReadBlock, w.cfg.ReadCount,
		)
		w.wg.Go(func() { w.loop(workerCtx, consumer) })
	}

	w.wg.Go(func() { w.promoteRetries(workerCtx) })

	log.Info().
		Str("action", "apns_worker_start").
		Str("instanceId", w.cfg.InstanceID).
		Int("workerCount", w.cfg.WorkerCount).
		Msg("apns worker started")
	return nil
}

// Final waits for in-flight work to drain, bounded at 5 s — if APNs Send
// blocks longer than that we log and move on (architecture graceful-
// shutdown budget per line 218).
func (w *APNsWorker) Final(_ context.Context) error {
	if w.cancel != nil {
		w.cancel()
	}

	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		log.Warn().Str("action", "apns_worker_final_timeout").
			Msg("apns worker final: 5s drain timeout exceeded; continuing shutdown")
	}
	log.Info().Str("action", "apns_worker_stop").Msg("apns worker stopped")
	return nil
}

func (w *APNsWorker) loop(ctx context.Context, c *redisx.StreamConsumer) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgs, err := c.Read(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return
			}
			log.Error().Err(err).Str("action", "apns_read_error").Msg("xreadgroup failed")
			// Sleep bounded by ctx so shutdown is still snappy.
			select {
			case <-ctx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}
		for _, m := range msgs {
			w.handle(ctx, c, m)
		}
	}
}

// shutdownWriteBudget bounds Redis-write work that has to outlive a
// cancelled parent ctx (finalising XACK / ZADD retry / XADD dlq during
// graceful shutdown). 2s is comfortably inside the App.Final 5s budget
// for this Runnable (architecture §Graceful Shutdown line 218).
const shutdownWriteBudget = 2 * time.Second

// writeCtxFor returns a ctx appropriate for final Redis writes: the
// original parent if still live, or a detached Background-with-timeout
// if the parent was already cancelled. The second return value is always
// safe to call — either the real cancel or a no-op — so callers can
// `defer cancel()` without branching.
//
// This is the guardrail for "message stuck in PEL forever" — EVERY path
// in handle() that finishes the consumer-group entry (ACK / XADD dlq /
// ZADD retry + XACK) must use this, not the raw incoming ctx, because a
// shutdown wave may arrive between the XREADGROUP and any of those
// writes. The worker only reads XREADGROUP ... ">", so messages that
// fail to ACK during shutdown are never reclaimed on restart.
func (w *APNsWorker) writeCtxFor(parent context.Context) (context.Context, context.CancelFunc) {
	if parent.Err() == nil {
		return parent, func() {}
	}
	return context.WithTimeout(context.Background(), shutdownWriteBudget)
}

func (w *APNsWorker) handle(ctx context.Context, c *redisx.StreamConsumer, m redis.XMessage) {
	raw, _ := m.Values["msg"].(string)
	var qm queueMessage
	if err := json.Unmarshal([]byte(raw), &qm); err != nil {
		log.Error().Err(err).Str("action", "apns_decode_error").Str("streamId", m.ID).Msg("decode error → dlq")
		wctx, cancel := w.writeCtxFor(ctx)
		defer cancel()
		w.xaddDLQ(wctx, qm, "decode_error")
		_ = c.Ack(wctx, m.ID)
		return
	}

	if qm.Payload.RespectsQuietHours {
		quiet, qerr := w.quiet.Resolve(ctx, qm.UserID)
		if qerr != nil {
			log.Warn().Err(qerr).Str("action", "apns_quiet_resolve_error").
				Str("userId", string(qm.UserID)).Msg("quiet resolve failed; delivering as alert (fail-open)")
		} else if quiet && qm.Payload.Kind == PushKindAlert {
			qm.Payload.Kind = PushKindSilent
		}
	}

	tokens, err := w.router.RouteTokens(ctx, qm.UserID)
	if err != nil {
		log.Warn().Err(err).Str("action", "apns_route_error").
			Str("userId", string(qm.UserID)).Msg("route error; will retry/dlq")
		wctx, cancel := w.writeCtxFor(ctx)
		defer cancel()
		w.retryOrDLQ(wctx, c, m.ID, qm)
		return
	}
	if len(tokens) == 0 {
		log.Info().Str("action", "apns_no_tokens").Str("userId", string(qm.UserID)).
			Msg("no registered tokens; ack without send")
		wctx, cancel := w.writeCtxFor(ctx)
		defer cancel()
		_ = c.Ack(wctx, m.ID)
		return
	}

	anyRetryable := false
	for _, rt := range tokens {
		n := buildNotification(rt, qm.Payload, w.clock.Now())
		resp, err := w.sender.Send(ctx, n)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				// Shutdown aborted the send mid-flight. Leaving the
				// message in the consumer group's PEL without XACK
				// would strand it forever — this worker never reclaims
				// pending entries (XREADGROUP ... ">"). Break out and
				// let retryOrDLQ reschedule + XACK via a detached
				// write context so the retry ZSET entry survives the
				// shutdown.
				anyRetryable = true
				log.Info().Str("action", "apns_send_ctx_canceled").
					Str("userId", string(qm.UserID)).Str("platform", rt.Platform).
					Msg("send aborted by shutdown; scheduling retry so PEL drains")
				break
			}
			anyRetryable = true
			log.Warn().Err(err).Str("action", "apns_send_transport_error").
				Str("userId", string(qm.UserID)).Str("platform", rt.Platform).
				Str("deviceToken", logx.MaskAPNsToken(rt.DeviceToken)).
				Msg("transport error; retry eligible")
			continue
		}

		switch resp.StatusCode {
		case 200:
			log.Info().Str("action", "apns_send_ok").
				Str("userId", string(qm.UserID)).Str("platform", rt.Platform).
				Str("deviceToken", logx.MaskAPNsToken(rt.DeviceToken)).
				Int("statusCode", resp.StatusCode).Msg("apns send ok")
		case 410:
			if derr := w.deleter.Delete(ctx, qm.UserID, rt.DeviceToken); derr != nil {
				log.Warn().Err(derr).Str("action", "apns_token_delete_error").
					Str("userId", string(qm.UserID)).
					Str("deviceToken", logx.MaskAPNsToken(rt.DeviceToken)).
					Msg("failed to delete expired token (will retry next 410)")
			}
			log.Info().Str("action", "apns_token_410_deleted").
				Str("userId", string(qm.UserID)).Str("platform", rt.Platform).
				Str("deviceToken", logx.MaskAPNsToken(rt.DeviceToken)).
				Str("reason", resp.Reason).Msg("apns 410; token deleted")
		case 400, 403, 404, 413:
			log.Error().Str("action", "apns_send_fatal").
				Str("userId", string(qm.UserID)).Str("platform", rt.Platform).
				Int("statusCode", resp.StatusCode).Str("reason", resp.Reason).
				Msg("apns permanent failure; no retry")
		case 429, 500, 503:
			anyRetryable = true
			log.Warn().Str("action", "apns_send_retryable").
				Str("userId", string(qm.UserID)).Str("platform", rt.Platform).
				Int("statusCode", resp.StatusCode).Str("reason", resp.Reason).
				Msg("apns transient failure; retry eligible")
		default:
			anyRetryable = true
			log.Warn().Str("action", "apns_send_unknown_status").
				Str("userId", string(qm.UserID)).Str("platform", rt.Platform).
				Int("statusCode", resp.StatusCode).Str("reason", resp.Reason).
				Msg("apns unknown status; retry eligible")
		}
	}

	// Same shutdown-resilience contract as the early return branches —
	// see writeCtxFor godoc for why the raw ctx is unsafe here.
	wctx, cancel := w.writeCtxFor(ctx)
	defer cancel()
	if anyRetryable {
		w.retryOrDLQ(wctx, c, m.ID, qm)
		return
	}
	_ = c.Ack(wctx, m.ID)
}

// retryOrDLQ branches on whether attempts remain: schedule a retry
// (ZADD + XACK) or give up (XADD dlq + XACK).
func (w *APNsWorker) retryOrDLQ(ctx context.Context, c *redisx.StreamConsumer, streamID string, qm queueMessage) {
	// Attempt counts sends that have already completed; on the N-th call
	// into retryOrDLQ we've sent (qm.Attempt+1) times total. If that is
	// still less than MaxAttempts, schedule another retry.
	if qm.Attempt+1 < w.cfg.MaxAttempts {
		w.scheduleRetry(ctx, c, streamID, qm)
		return
	}
	w.xaddDLQ(ctx, qm, "retries_exhausted")
	_ = c.Ack(ctx, streamID)
}

// scheduleRetry advances Attempt, computes dueAt from the backoff table
// indexed by the new Attempt (1-based into RetryBackoffsMs), and ZADDs
// the re-marshalled qm to apns:retry together with XACK of the original
// stream entry in one pipeline. The pair is the atomic hand-off from
// consumer-group PEL to scheduled retry.
func (w *APNsWorker) scheduleRetry(ctx context.Context, c *redisx.StreamConsumer, streamID string, qm queueMessage) {
	qm.Attempt++
	idx := qm.Attempt - 1
	if idx >= len(w.cfg.RetryBackoffsMs) {
		idx = len(w.cfg.RetryBackoffsMs) - 1
	}
	backoffMs := w.cfg.RetryBackoffsMs[idx]
	dueAtMs := w.clock.Now().UnixMilli() + int64(backoffMs)

	raw, err := json.Marshal(qm)
	if err != nil {
		log.Error().Err(err).Str("action", "apns_retry_marshal_error").Msg("cannot marshal for retry; dlq")
		w.xaddDLQ(ctx, qm, "retry_marshal_error")
		_ = c.Ack(ctx, streamID)
		return
	}

	pipe := w.streamCmd.Pipeline()
	pipe.ZAdd(ctx, w.cfg.RetryZSetKey, redis.Z{Score: float64(dueAtMs), Member: string(raw)})
	pipe.XAck(ctx, w.cfg.StreamKey, w.cfg.ConsumerGroup, streamID)
	if _, err := pipe.Exec(ctx); err != nil {
		log.Error().Err(err).Str("action", "apns_retry_schedule_error").Msg("pipeline failed")
		return
	}
	log.Info().Str("action", "apns_retry_scheduled").
		Str("userId", string(qm.UserID)).Int("attempt", qm.Attempt).
		Int64("dueAtMs", dueAtMs).Msg("retry scheduled")
}

// promoteRetries is the 100 ms ticker goroutine that moves due retries
// from apns:retry ZSET back into apns:queue stream.
//
// The ticker uses real wall-clock time (time.NewTicker) — it's a
// scheduling primitive, not a timestamp source per M9 — while the
// "what is due" check uses w.clock.Now().UnixMilli(), so tests can still
// drive the retry math deterministically via FakeClock.Advance().
func (w *APNsWorker) promoteRetries(ctx context.Context) {
	// real ticker is a scheduling primitive (not a timestamp) per M9
	// exemption — scoring uses clock.Now().UnixMilli()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		w.PromoteOnce(ctx)
	}
}

// PromoteOnce performs a single scan of apns:retry for due entries and
// promotes them back to the queue. Exported so integration tests can
// drive promotion deterministically without timing-dependent sleeps.
func (w *APNsWorker) PromoteOnce(ctx context.Context) {
	nowMs := w.clock.Now().UnixMilli()
	due, err := w.streamCmd.ZRangeByScore(ctx, w.cfg.RetryZSetKey, &redis.ZRangeBy{
		Min:    "0",
		Max:    strconv.FormatInt(nowMs, 10),
		Offset: 0,
		Count:  100,
	}).Result()
	if err != nil {
		log.Warn().Err(err).Str("action", "apns_retry_scan_error").Msg("zrangebyscore failed")
		return
	}
	for _, member := range due {
		// Decode enough of the retry payload to re-XADD with updated
		// attempt header for ops debugging.
		var qm queueMessage
		_ = json.Unmarshal([]byte(member), &qm)

		pipe := w.streamCmd.Pipeline()
		pipe.ZRem(ctx, w.cfg.RetryZSetKey, member)
		pipe.XAdd(ctx, &redis.XAddArgs{
			Stream: w.cfg.StreamKey,
			ID:     "*",
			Values: map[string]any{
				"userId":  string(qm.UserID),
				"msg":     member,
				"attempt": strconv.Itoa(qm.Attempt),
			},
		})
		if _, err := pipe.Exec(ctx); err != nil {
			log.Warn().Err(err).Str("action", "apns_retry_promote_error").Msg("promote pipeline failed")
			continue
		}
		log.Info().Str("action", "apns_retry_promoted").
			Str("userId", string(qm.UserID)).Int("attempt", qm.Attempt).Msg("retry promoted")
	}
}

// xaddDLQ writes a DLQ stream entry. Best-effort — if the DLQ write fails
// we log and continue; dropping a retries-exhausted message is bad but
// blocking shutdown on a dead Redis is worse.
func (w *APNsWorker) xaddDLQ(ctx context.Context, qm queueMessage, reason string) {
	raw, err := json.Marshal(qm)
	if err != nil {
		log.Error().Err(err).Str("action", "apns_dlq_marshal_error").Msg("cannot marshal for dlq")
		return
	}
	_, err = w.streamCmd.XAdd(ctx, &redis.XAddArgs{
		Stream: w.cfg.DLQKey,
		ID:     "*",
		Values: map[string]any{
			"userId":     string(qm.UserID),
			"msg":        string(raw),
			"reason":     reason,
			"attempts":   strconv.Itoa(qm.Attempt + 1),
			"failedAtMs": strconv.FormatInt(w.clock.Now().UnixMilli(), 10),
		},
	}).Result()
	if err != nil {
		log.Error().Err(err).Str("action", "apns_dlq_write_error").Msg("dlq XADD failed")
		return
	}
	log.Error().Str("action", "apns_dlq").
		Str("userId", string(qm.UserID)).Int("attempts", qm.Attempt+1).
		Str("reason", reason).Msg("message sent to dlq")
}

// buildNotification produces the apns2.Notification bound to a single
// RoutedToken according to the Kind policy. The caller never mutates it
// further.
//
// NFR-COMP-3: silent push carries PushType=background and no alert
// fields; alert push carries PushType=alert with Title/Body/Sound.
func buildNotification(rt RoutedToken, p PushPayload, now time.Time) *apns2.Notification {
	n := &apns2.Notification{
		DeviceToken: rt.DeviceToken,
		Topic:       rt.Topic,
		Expiration:  now.Add(1 * time.Hour),
	}
	pl := payload.NewPayload()
	switch p.Kind {
	case PushKindAlert:
		n.PushType = apns2.PushTypeAlert
		n.Priority = apns2.PriorityHigh
		pl = pl.AlertTitle(p.Title).Sound("default")
		if p.Body != "" {
			pl = pl.AlertBody(p.Body)
		}
		if p.DeepLink != "" {
			pl = pl.Custom("deepLink", p.DeepLink)
		}
	case PushKindSilent:
		n.PushType = apns2.PushTypeBackground
		n.Priority = apns2.PriorityLow
		pl = pl.ContentAvailable()
		if p.DeepLink != "" {
			pl = pl.Custom("deepLink", p.DeepLink)
		}
	}
	n.Payload = pl
	return n
}
