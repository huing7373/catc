//go:build integration

package push

import (
	"context"
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

// setupIntegration wires up a fully-started worker with per-test
// miniredis + FakeClock. The caller receives (stop, cmd, worker, clock)
// where stop performs the graceful Final step.
func setupIntegration(t *testing.T, sender ApnsSender, tokens []TokenInfo, quiet QuietHoursResolver, deleter TokenDeleter) (stop func(), mr *miniredis.Miniredis, cmd redis.Cmdable, w *APNsWorker, clk *clockx.FakeClock) {
	t.Helper()
	mr = miniredis.RunT(t)
	cmd = redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { cmd.(*redis.Client).Close() })

	clk = clockx.NewFakeClock(time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC))
	router := NewAPNsRouter(&fakeTokenProvider{tokens: tokens}, "bundle.app.watch", "bundle.app")
	cfg := defaultWorkerCfg()
	cfg.ReadBlock = 50 * time.Millisecond

	w = NewAPNsWorker(cfg, cmd, sender, router, quiet, deleter, clk)
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, w.Start(ctx))
	stop = func() {
		cancel()
		_ = w.Final(context.Background())
	}
	t.Cleanup(stop)
	return stop, mr, cmd, w, clk
}

func enqueueViaPusher(t *testing.T, cmd redis.Cmdable, clk clockx.Clock, userID ids.UserID, pl PushPayload) {
	t.Helper()
	sp := redisx.NewStreamPusher(cmd, "apns:queue")
	p := NewRedisStreamsPusher(sp, cmd, clk, 5*time.Minute)
	require.NoError(t, p.Enqueue(context.Background(), userID, pl))
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("waitFor: condition not satisfied within %s", timeout)
}

func TestIntegration_APNs_EndToEnd_Success(t *testing.T) {
	sender := &fakeSender{responses: []*apns2.Response{{StatusCode: 200}}}
	_, _, cmd, _, clk := setupIntegration(t, sender,
		[]TokenInfo{{Platform: "watch", DeviceToken: "wt-1"}},
		EmptyQuietHoursResolver{}, &fakeDeleter{},
	)

	enqueueViaPusher(t, cmd, clk, "u1", PushPayload{Kind: PushKindAlert, Title: "hi"})

	waitFor(t, 2*time.Second, func() bool { return len(sender.Calls()) >= 1 })
	assert.Len(t, sender.Calls(), 1)
}

func TestIntegration_APNs_RetryPromotion_Succeeds(t *testing.T) {
	// First call: 500 (transient); second call: 200.
	sender := &fakeSender{responses: []*apns2.Response{
		{StatusCode: 500}, {StatusCode: 200},
	}}
	_, _, cmd, w, clk := setupIntegration(t, sender,
		[]TokenInfo{{Platform: "watch", DeviceToken: "wt-1"}},
		EmptyQuietHoursResolver{}, &fakeDeleter{},
	)

	enqueueViaPusher(t, cmd, clk, "u1", PushPayload{Kind: PushKindAlert, Title: "hi"})
	// First Send should happen.
	waitFor(t, 2*time.Second, func() bool { return len(sender.Calls()) >= 1 })
	// Retry was scheduled.
	waitFor(t, 2*time.Second, func() bool {
		return cmd.ZCard(context.Background(), "apns:retry").Val() == 1
	})

	// Advance the fake clock past the 1s backoff, let the retry-promoter
	// tick find it, then the second read happens.
	clk.Advance(1100 * time.Millisecond)
	w.PromoteOnce(context.Background())
	waitFor(t, 2*time.Second, func() bool { return len(sender.Calls()) >= 2 })

	// After second 200, no entries remain in retry.
	waitFor(t, 2*time.Second, func() bool {
		return cmd.ZCard(context.Background(), "apns:retry").Val() == 0
	})
}

func TestIntegration_APNs_MaxRetries_DLQ(t *testing.T) {
	sender := &fakeSender{responses: []*apns2.Response{
		{StatusCode: 500}, {StatusCode: 500}, {StatusCode: 500}, {StatusCode: 500},
	}}
	_, _, cmd, w, clk := setupIntegration(t, sender,
		[]TokenInfo{{Platform: "watch", DeviceToken: "wt-1"}},
		EmptyQuietHoursResolver{}, &fakeDeleter{},
	)
	enqueueViaPusher(t, cmd, clk, "u1", PushPayload{Kind: PushKindAlert, Title: "hi"})

	driveAllRetriesToDLQ(t, cmd, w, clk, 1)
	assert.EqualValues(t, 4, len(sender.Calls()), "4 total send attempts (initial + 3 retries)")
}

// driveAllRetriesToDLQ walks the queue through the full retry schedule:
// for each backoff step wait for the failure to be scheduled in the ZSET,
// advance the clock past it, call PromoteOnce (belt-and-braces with the
// worker's own 100ms ticker), and finally assert the DLQ received the
// expected number of entries.
func driveAllRetriesToDLQ(t *testing.T, cmd redis.Cmdable, w *APNsWorker, clk *clockx.FakeClock, wantDLQ int) {
	t.Helper()
	for _, d := range []time.Duration{1100 * time.Millisecond, 3100 * time.Millisecond, 9100 * time.Millisecond} {
		waitFor(t, 3*time.Second, func() bool { return cmd.ZCard(context.Background(), "apns:retry").Val() == 1 })
		clk.Advance(d)
		w.PromoteOnce(context.Background())
	}
	waitFor(t, 3*time.Second, func() bool {
		n, _ := cmd.XLen(context.Background(), "apns:dlq").Result()
		return int(n) >= wantDLQ
	})
}

func TestIntegration_APNs_410_DeletesToken(t *testing.T) {
	deleter := &fakeDeleter{}
	sender := &fakeSender{responses: []*apns2.Response{{StatusCode: 410, Reason: "Unregistered"}}}
	_, _, cmd, _, clk := setupIntegration(t, sender,
		[]TokenInfo{{Platform: "watch", DeviceToken: "wt-dead"}},
		EmptyQuietHoursResolver{}, deleter,
	)
	enqueueViaPusher(t, cmd, clk, "u1", PushPayload{Kind: PushKindAlert, Title: "hi"})

	waitFor(t, 2*time.Second, func() bool {
		deleter.mu.Lock()
		defer deleter.mu.Unlock()
		return len(deleter.calls) >= 1
	})
	assert.EqualValues(t, 0, cmd.ZCard(context.Background(), "apns:retry").Val(), "410 alone must not retry")
}

func TestIntegration_APNs_IdempotencyDedupes(t *testing.T) {
	// Both sends (if they both reached the worker) would fail → DLQ.
	// Because the second Enqueue is deduped, only one message reaches the
	// worker, so exactly one DLQ entry is produced.
	sender := &fakeSender{responses: []*apns2.Response{
		{StatusCode: 500}, {StatusCode: 500}, {StatusCode: 500}, {StatusCode: 500},
	}}
	_, _, cmd, w, clk := setupIntegration(t, sender,
		[]TokenInfo{{Platform: "watch", DeviceToken: "wt-1"}},
		EmptyQuietHoursResolver{}, &fakeDeleter{},
	)

	pl := PushPayload{Kind: PushKindAlert, Title: "hi", IdempotencyKey: "dedup-xyz"}
	enqueueViaPusher(t, cmd, clk, "u1", pl)
	enqueueViaPusher(t, cmd, clk, "u1", pl)

	driveAllRetriesToDLQ(t, cmd, w, clk, 1)

	n, _ := cmd.XLen(context.Background(), "apns:dlq").Result()
	assert.EqualValues(t, 1, n, "second enqueue must be deduped so only one message reaches DLQ")
}
