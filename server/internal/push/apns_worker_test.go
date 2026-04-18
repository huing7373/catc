package push

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/sideshow/apns2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/ids"
	"github.com/huing/cat/server/pkg/redisx"
)

// ---- fakes ---------------------------------------------------------------

type fakeSender struct {
	mu        sync.Mutex
	responses []*apns2.Response
	errors    []error
	calls     []*apns2.Notification
}

func (f *fakeSender) Send(_ context.Context, n *apns2.Notification) (*apns2.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, n)
	idx := len(f.calls) - 1
	if idx < len(f.errors) && f.errors[idx] != nil {
		return nil, f.errors[idx]
	}
	if idx < len(f.responses) {
		return f.responses[idx], nil
	}
	return &apns2.Response{StatusCode: 200}, nil
}

func (f *fakeSender) Calls() []*apns2.Notification {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]*apns2.Notification, len(f.calls))
	copy(out, f.calls)
	return out
}

type fakeDeleter struct {
	mu    sync.Mutex
	calls []struct {
		User  ids.UserID
		Token string
	}
	err error
}

func (f *fakeDeleter) Delete(_ context.Context, u ids.UserID, tok string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, struct {
		User  ids.UserID
		Token string
	}{u, tok})
	return f.err
}

type fakeQuiet struct {
	quiet bool
	err   error
}

func (f fakeQuiet) Resolve(context.Context, ids.UserID) (bool, error) {
	return f.quiet, f.err
}

// ---- helpers -------------------------------------------------------------

func defaultWorkerCfg() APNsWorkerConfig {
	return APNsWorkerConfig{
		InstanceID:      "w-test",
		StreamKey:       "apns:queue",
		DLQKey:          "apns:dlq",
		RetryZSetKey:    "apns:retry",
		ConsumerGroup:   "apns_workers",
		WorkerCount:     1,
		ReadBlock:       50 * time.Millisecond,
		ReadCount:       10,
		RetryBackoffsMs: []int{1000, 3000, 9000},
		MaxAttempts:     4,
	}
}

// setupWorkerEnv creates miniredis, a worker pointed at it, and
// pre-creates the consumer group so subsequent enqueues (via primeQueue)
// are visible to XREADGROUP readers. Production Start() has the same
// ordering: EnsureGroup runs synchronously before any XADD happens.
func setupWorkerEnv(t *testing.T, sender ApnsSender, tokens []TokenInfo, quiet QuietHoursResolver, deleter TokenDeleter) (*miniredis.Miniredis, redis.Cmdable, *APNsWorker, *clockx.FakeClock) {
	t.Helper()
	mr := miniredis.RunT(t)
	cmd := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { cmd.Close() })

	clk := clockx.NewFakeClock(time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC))
	router := NewAPNsRouter(&fakeTokenProvider{tokens: tokens}, "bundle.app.watch", "bundle.app")

	w := NewAPNsWorker(defaultWorkerCfg(), cmd, sender, router, quiet, deleter, clk)

	seed := redisx.NewStreamConsumer(cmd, w.cfg.StreamKey, w.cfg.ConsumerGroup, "seed", 50*time.Millisecond, 10)
	require.NoError(t, seed.EnsureGroup(context.Background()))
	return mr, cmd, w, clk
}

// primeQueue enqueues a queueMessage and returns the streamID. Uses the
// Pusher path so serialization matches production.
func primeQueue(t *testing.T, cmd redis.Cmdable, clk clockx.Clock, userID ids.UserID, payload PushPayload) {
	t.Helper()
	sp := redisx.NewStreamPusher(cmd, "apns:queue")
	p := NewRedisStreamsPusher(sp, cmd, clk, 5*time.Minute)
	require.NoError(t, p.Enqueue(context.Background(), userID, payload))
}

// primeRawQueue inserts a raw XADD with arbitrary msg JSON (used for the
// decode-error test).
func primeRawQueue(t *testing.T, cmd redis.Cmdable, rawMsg string) {
	t.Helper()
	_, err := cmd.XAdd(context.Background(), &redis.XAddArgs{
		Stream: "apns:queue",
		ID:     "*",
		Values: map[string]any{"userId": "u1", "msg": rawMsg, "attempt": "0"},
	}).Result()
	require.NoError(t, err)
}

// readOneAndHandle creates a consumer (group already exists per
// setupWorkerEnv), reads one message, and calls worker.handle for it.
// Returns the streamID consumed.
func readOneAndHandle(t *testing.T, w *APNsWorker, cmd redis.Cmdable) (streamID string, ok bool) {
	t.Helper()
	ctx := context.Background()
	c := redisx.NewStreamConsumer(cmd, w.cfg.StreamKey, w.cfg.ConsumerGroup, "t-reader", 200*time.Millisecond, 10)
	msgs, err := c.Read(ctx)
	require.NoError(t, err)
	if len(msgs) == 0 {
		return "", false
	}
	w.handle(ctx, c, msgs[0])
	return msgs[0].ID, true
}

// ---- tests ---------------------------------------------------------------

func TestHandle_AllTokensSucceed_Acks(t *testing.T) {
	t.Parallel()
	sender := &fakeSender{responses: []*apns2.Response{
		{StatusCode: 200},
		{StatusCode: 200},
	}}
	_, cmd, w, clk := setupWorkerEnv(t, sender,
		[]TokenInfo{{Platform: "watch", DeviceToken: "wt-1"}, {Platform: "iphone", DeviceToken: "ip-1"}},
		EmptyQuietHoursResolver{}, &fakeDeleter{},
	)
	primeQueue(t, cmd, clk, "u1", PushPayload{Kind: PushKindAlert, Title: "hi"})

	_, ok := readOneAndHandle(t, w, cmd)
	require.True(t, ok)

	assert.Len(t, sender.Calls(), 2, "both tokens should be sent")

	retrySet := cmd.ZCard(context.Background(), "apns:retry").Val()
	assert.EqualValues(t, 0, retrySet, "no retry should be scheduled")
}

func TestHandle_NoTokens_AcksAndLogs(t *testing.T) {
	t.Parallel()
	sender := &fakeSender{}
	_, cmd, w, clk := setupWorkerEnv(t, sender, nil, EmptyQuietHoursResolver{}, &fakeDeleter{})
	primeQueue(t, cmd, clk, "u1", PushPayload{Kind: PushKindAlert, Title: "hi"})

	_, ok := readOneAndHandle(t, w, cmd)
	require.True(t, ok)
	assert.Empty(t, sender.Calls())
}

func TestHandle_410Response_DeletesTokenAndAcks(t *testing.T) {
	t.Parallel()
	sender := &fakeSender{responses: []*apns2.Response{{StatusCode: 410, Reason: "Unregistered"}}}
	deleter := &fakeDeleter{}
	_, cmd, w, clk := setupWorkerEnv(t, sender,
		[]TokenInfo{{Platform: "watch", DeviceToken: "wt-dead"}},
		EmptyQuietHoursResolver{}, deleter,
	)
	primeQueue(t, cmd, clk, "u1", PushPayload{Kind: PushKindAlert, Title: "hi"})

	_, ok := readOneAndHandle(t, w, cmd)
	require.True(t, ok)

	require.Len(t, deleter.calls, 1)
	assert.Equal(t, ids.UserID("u1"), deleter.calls[0].User)
	assert.Equal(t, "wt-dead", deleter.calls[0].Token)
	assert.EqualValues(t, 0, cmd.ZCard(context.Background(), "apns:retry").Val(), "410 alone must not cause retry")
}

func TestHandle_500Response_SchedulesRetry(t *testing.T) {
	t.Parallel()
	sender := &fakeSender{responses: []*apns2.Response{{StatusCode: 500}}}
	_, cmd, w, clk := setupWorkerEnv(t, sender,
		[]TokenInfo{{Platform: "watch", DeviceToken: "wt-1"}},
		EmptyQuietHoursResolver{}, &fakeDeleter{},
	)
	primeQueue(t, cmd, clk, "u1", PushPayload{Kind: PushKindAlert, Title: "hi"})

	_, ok := readOneAndHandle(t, w, cmd)
	require.True(t, ok)

	ctx := context.Background()
	assert.EqualValues(t, 1, cmd.ZCard(ctx, "apns:retry").Val(), "one retry should be scheduled")
	// Score should equal now+1000ms (first-attempt backoff).
	dueAt := clk.Now().UnixMilli() + 1000
	items, err := cmd.ZRangeByScoreWithScores(ctx, "apns:retry", &redis.ZRangeBy{Min: "0", Max: "+inf"}).Result()
	require.NoError(t, err)
	require.Len(t, items, 1)
	assert.EqualValues(t, dueAt, int64(items[0].Score))
}

func TestHandle_RetryBackoffCorrect(t *testing.T) {
	t.Parallel()
	// Three-in-a-row 500s: observe score equals now + 1000/3000/9000 per attempt.
	sender := &fakeSender{responses: []*apns2.Response{
		{StatusCode: 500}, {StatusCode: 500}, {StatusCode: 500},
	}}
	_, cmd, w, clk := setupWorkerEnv(t, sender,
		[]TokenInfo{{Platform: "watch", DeviceToken: "wt-1"}},
		EmptyQuietHoursResolver{}, &fakeDeleter{},
	)
	ctx := context.Background()

	// Attempt 0 (initial send) — expected backoff entry at +1000ms.
	primeQueue(t, cmd, clk, "u1", PushPayload{Kind: PushKindAlert, Title: "hi"})
	_, ok := readOneAndHandle(t, w, cmd)
	require.True(t, ok)
	wantDue := clk.Now().UnixMilli() + 1000
	assertZSetHasScore(t, cmd, "apns:retry", wantDue)

	// Advance past dueAt, promote → stream has retry (attempt=1); fail again.
	clk.Advance(1100 * time.Millisecond)
	w.PromoteOnce(ctx)
	_, ok = readOneAndHandle(t, w, cmd)
	require.True(t, ok)
	wantDue = clk.Now().UnixMilli() + 3000
	assertZSetHasScore(t, cmd, "apns:retry", wantDue)

	// Advance past dueAt, promote → stream has retry (attempt=2); fail again.
	clk.Advance(3100 * time.Millisecond)
	w.PromoteOnce(ctx)
	_, ok = readOneAndHandle(t, w, cmd)
	require.True(t, ok)
	wantDue = clk.Now().UnixMilli() + 9000
	assertZSetHasScore(t, cmd, "apns:retry", wantDue)
}

func assertZSetHasScore(t *testing.T, cmd redis.Cmdable, key string, wantScore int64) {
	t.Helper()
	items, err := cmd.ZRangeByScoreWithScores(context.Background(), key, &redis.ZRangeBy{Min: "0", Max: "+inf"}).Result()
	require.NoError(t, err)
	require.Len(t, items, 1, "zset %s expected 1 item", key)
	assert.EqualValues(t, wantScore, int64(items[0].Score))
}

func TestHandle_MaxAttemptsExceeded_DLQ(t *testing.T) {
	t.Parallel()
	sender := &fakeSender{responses: []*apns2.Response{{StatusCode: 500}}}
	_, cmd, w, clk := setupWorkerEnv(t, sender,
		[]TokenInfo{{Platform: "watch", DeviceToken: "wt-1"}},
		EmptyQuietHoursResolver{}, &fakeDeleter{},
	)
	ctx := context.Background()

	// Build a queueMessage whose Attempt is already at MaxAttempts-1 so
	// one more failed send triggers DLQ.
	qm := queueMessage{
		UserID:       "u1",
		Payload:      PushPayload{Kind: PushKindAlert, Title: "hi"},
		Attempt:      w.cfg.MaxAttempts - 1,
		EnqueuedAtMs: clk.Now().UnixMilli(),
	}
	raw, err := json.Marshal(qm)
	require.NoError(t, err)
	primeRawQueue(t, cmd, string(raw))

	_, ok := readOneAndHandle(t, w, cmd)
	require.True(t, ok)

	dlq, err := cmd.XLen(ctx, "apns:dlq").Result()
	require.NoError(t, err)
	assert.EqualValues(t, 1, dlq, "one DLQ entry expected")
	assert.EqualValues(t, 0, cmd.ZCard(ctx, "apns:retry").Val(), "no retry when exhausted")
}

func TestHandle_QuietHours_CoercesKindToSilent(t *testing.T) {
	t.Parallel()
	sender := &fakeSender{responses: []*apns2.Response{{StatusCode: 200}}}
	_, cmd, w, clk := setupWorkerEnv(t, sender,
		[]TokenInfo{{Platform: "watch", DeviceToken: "wt-1"}},
		fakeQuiet{quiet: true}, &fakeDeleter{},
	)
	primeQueue(t, cmd, clk, "u1", PushPayload{Kind: PushKindAlert, Title: "hi", RespectsQuietHours: true})

	_, ok := readOneAndHandle(t, w, cmd)
	require.True(t, ok)

	calls := sender.Calls()
	require.Len(t, calls, 1)
	assert.Equal(t, apns2.PushTypeBackground, calls[0].PushType, "quiet hours should downgrade to background")
}

func TestHandle_QuietResolverError_FailsOpenToLoud(t *testing.T) {
	t.Parallel()
	sender := &fakeSender{responses: []*apns2.Response{{StatusCode: 200}}}
	_, cmd, w, clk := setupWorkerEnv(t, sender,
		[]TokenInfo{{Platform: "watch", DeviceToken: "wt-1"}},
		fakeQuiet{err: errors.New("boom")}, &fakeDeleter{},
	)
	primeQueue(t, cmd, clk, "u1", PushPayload{Kind: PushKindAlert, Title: "hi", RespectsQuietHours: true})

	_, ok := readOneAndHandle(t, w, cmd)
	require.True(t, ok)
	calls := sender.Calls()
	require.Len(t, calls, 1)
	assert.Equal(t, apns2.PushTypeAlert, calls[0].PushType, "quiet resolver error must fail open to alert")
}

func TestHandle_UnknownPlatform_SkipsToken(t *testing.T) {
	t.Parallel()
	sender := &fakeSender{}
	_, cmd, w, clk := setupWorkerEnv(t, sender,
		[]TokenInfo{{Platform: "blackberry", DeviceToken: "bb-1"}},
		EmptyQuietHoursResolver{}, &fakeDeleter{},
	)
	primeQueue(t, cmd, clk, "u1", PushPayload{Kind: PushKindAlert, Title: "hi"})

	_, ok := readOneAndHandle(t, w, cmd)
	require.True(t, ok)
	assert.Empty(t, sender.Calls(), "unknown-platform token must be routed to nothing")
}

func TestHandle_DecodeError_DLQ(t *testing.T) {
	t.Parallel()
	sender := &fakeSender{}
	_, cmd, w, _ := setupWorkerEnv(t, sender, nil, EmptyQuietHoursResolver{}, &fakeDeleter{})

	primeRawQueue(t, cmd, "not-json")

	_, ok := readOneAndHandle(t, w, cmd)
	require.True(t, ok)

	dlq, err := cmd.XLen(context.Background(), "apns:dlq").Result()
	require.NoError(t, err)
	assert.EqualValues(t, 1, dlq)
}

func TestHandle_4xxNon410_NoRetry(t *testing.T) {
	t.Parallel()
	sender := &fakeSender{responses: []*apns2.Response{{StatusCode: 403, Reason: "Forbidden"}}}
	_, cmd, w, clk := setupWorkerEnv(t, sender,
		[]TokenInfo{{Platform: "watch", DeviceToken: "wt-1"}},
		EmptyQuietHoursResolver{}, &fakeDeleter{},
	)
	primeQueue(t, cmd, clk, "u1", PushPayload{Kind: PushKindAlert, Title: "hi"})
	_, ok := readOneAndHandle(t, w, cmd)
	require.True(t, ok)

	assert.EqualValues(t, 0, cmd.ZCard(context.Background(), "apns:retry").Val())
}

func TestNewAPNsWorker_NilArgs_Panic(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	cmd := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { cmd.Close() })
	clk := clockx.NewFakeClock(time.Now())
	router := NewAPNsRouter(EmptyTokenProvider{}, "x", "y")
	cfg := defaultWorkerCfg()

	cases := []struct {
		name string
		fn   func()
	}{
		{"nil streamCmd", func() {
			_ = NewAPNsWorker(cfg, nil, &fakeSender{}, router, EmptyQuietHoursResolver{}, EmptyTokenDeleter{}, clk)
		}},
		{"nil sender", func() {
			_ = NewAPNsWorker(cfg, cmd, nil, router, EmptyQuietHoursResolver{}, EmptyTokenDeleter{}, clk)
		}},
		{"nil router", func() {
			_ = NewAPNsWorker(cfg, cmd, &fakeSender{}, nil, EmptyQuietHoursResolver{}, EmptyTokenDeleter{}, clk)
		}},
		{"nil quiet", func() {
			_ = NewAPNsWorker(cfg, cmd, &fakeSender{}, router, nil, EmptyTokenDeleter{}, clk)
		}},
		{"nil deleter", func() {
			_ = NewAPNsWorker(cfg, cmd, &fakeSender{}, router, EmptyQuietHoursResolver{}, nil, clk)
		}},
		{"nil clock", func() {
			_ = NewAPNsWorker(cfg, cmd, &fakeSender{}, router, EmptyQuietHoursResolver{}, EmptyTokenDeleter{}, nil)
		}},
		{"empty instance", func() {
			cfg2 := cfg
			cfg2.InstanceID = ""
			_ = NewAPNsWorker(cfg2, cmd, &fakeSender{}, router, EmptyQuietHoursResolver{}, EmptyTokenDeleter{}, clk)
		}},
		{"zero worker count", func() {
			cfg2 := cfg
			cfg2.WorkerCount = 0
			_ = NewAPNsWorker(cfg2, cmd, &fakeSender{}, router, EmptyQuietHoursResolver{}, EmptyTokenDeleter{}, clk)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Panics(t, tc.fn)
		})
	}
}

// keep strconv import tidy across future edits
var _ = strconv.Itoa

// ---- shutdown resilience (review round 2) --------------------------------
//
// Every early-return branch in handle() must finalise the stream entry
// (XACK / XADD dlq / ZADD retry + XACK) through a ctx that survives a
// cancelled parent. Otherwise a shutdown during quiet.Resolve /
// RouteTokens / routing-empty would leave the message in the consumer
// group's pending list forever, because the worker only reads
// XREADGROUP ... ">" and never reclaims PEL entries on restart.

// assertPELEmpty fails the test if the consumer group still has pending
// entries for the stream. Exercises miniredis's XPENDING semantics.
func assertPELEmpty(t *testing.T, cmd redis.Cmdable, stream, group string) {
	t.Helper()
	pending, err := cmd.XPending(context.Background(), stream, group).Result()
	require.NoError(t, err)
	assert.EqualValues(t, 0, pending.Count, "message must not be stuck in PEL")
}

func TestHandle_DecodeError_CancelledCtx_StillDlqAndAcks(t *testing.T) {
	t.Parallel()
	_, cmd, w, _ := setupWorkerEnv(t, &fakeSender{}, nil, EmptyQuietHoursResolver{}, &fakeDeleter{})
	primeRawQueue(t, cmd, "not-json")

	// Read the message under a live ctx, then cancel before calling
	// handle so every subsequent Redis write goes through writeCtxFor.
	ctx, cancel := context.WithCancel(context.Background())
	c := redisx.NewStreamConsumer(cmd, w.cfg.StreamKey, w.cfg.ConsumerGroup, "t-reader", 200*time.Millisecond, 10)
	msgs, err := c.Read(ctx)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	cancel()
	w.handle(ctx, c, msgs[0])

	dlq, err := cmd.XLen(context.Background(), "apns:dlq").Result()
	require.NoError(t, err)
	assert.EqualValues(t, 1, dlq, "decode-error must still reach DLQ under cancelled ctx")
	assertPELEmpty(t, cmd, w.cfg.StreamKey, w.cfg.ConsumerGroup)
}

func TestHandle_RouteError_CancelledCtx_SchedulesRetry(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	cmd := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { cmd.Close() })

	clk := clockx.NewFakeClock(time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC))
	// Route error path: TokenProvider returns an error.
	router := NewAPNsRouter(&fakeTokenProvider{err: errors.New("mongo transient")}, "x", "y")
	w := NewAPNsWorker(defaultWorkerCfg(), cmd, &fakeSender{}, router, EmptyQuietHoursResolver{}, &fakeDeleter{}, clk)

	seed := redisx.NewStreamConsumer(cmd, w.cfg.StreamKey, w.cfg.ConsumerGroup, "seed", 50*time.Millisecond, 10)
	require.NoError(t, seed.EnsureGroup(context.Background()))
	primeQueue(t, cmd, clk, "u1", PushPayload{Kind: PushKindAlert, Title: "hi"})

	ctx, cancel := context.WithCancel(context.Background())
	c := redisx.NewStreamConsumer(cmd, w.cfg.StreamKey, w.cfg.ConsumerGroup, "t-reader", 200*time.Millisecond, 10)
	msgs, err := c.Read(ctx)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	cancel()
	w.handle(ctx, c, msgs[0])

	assert.EqualValues(t, 1, cmd.ZCard(context.Background(), "apns:retry").Val(),
		"route error must schedule retry even when ctx is cancelled")
	assertPELEmpty(t, cmd, w.cfg.StreamKey, w.cfg.ConsumerGroup)
}

func TestHandle_NoTokens_CancelledCtx_StillAcks(t *testing.T) {
	t.Parallel()
	_, cmd, w, clk := setupWorkerEnv(t, &fakeSender{}, nil, EmptyQuietHoursResolver{}, &fakeDeleter{})
	primeQueue(t, cmd, clk, "u1", PushPayload{Kind: PushKindAlert, Title: "hi"})

	ctx, cancel := context.WithCancel(context.Background())
	c := redisx.NewStreamConsumer(cmd, w.cfg.StreamKey, w.cfg.ConsumerGroup, "t-reader", 200*time.Millisecond, 10)
	msgs, err := c.Read(ctx)
	require.NoError(t, err)
	require.Len(t, msgs, 1)
	cancel()
	w.handle(ctx, c, msgs[0])

	assertPELEmpty(t, cmd, w.cfg.StreamKey, w.cfg.ConsumerGroup)
}
