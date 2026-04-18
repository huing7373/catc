package push

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/redisx"
)

func setupMiniredis(t *testing.T) (*miniredis.Miniredis, redis.Cmdable) {
	t.Helper()
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { cli.Close() })
	return mr, cli
}

func newTestPusher(t *testing.T) (*miniredis.Miniredis, redis.Cmdable, *RedisStreamsPusher, *clockx.FakeClock) {
	t.Helper()
	mr, cmd := setupMiniredis(t)
	clk := clockx.NewFakeClock(time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC))
	sp := redisx.NewStreamPusher(cmd, "apns:queue")
	p := NewRedisStreamsPusher(sp, cmd, clk, 5*time.Minute)
	return mr, cmd, p, clk
}

func TestEnqueue_XAddsToStream(t *testing.T) {
	t.Parallel()
	mr, _, p, _ := newTestPusher(t)

	err := p.Enqueue(context.Background(), "u1", PushPayload{
		Kind:  PushKindAlert,
		Title: "hi",
		Body:  "hello",
	})
	require.NoError(t, err)

	entries, err := mr.Stream("apns:queue")
	require.NoError(t, err)
	assert.Len(t, entries, 1)
}

func TestEnqueue_IdempotencyKeySkipsDuplicate(t *testing.T) {
	t.Parallel()
	mr, _, p, _ := newTestPusher(t)
	ctx := context.Background()

	pl := PushPayload{
		Kind:           PushKindAlert,
		Title:          "hi",
		IdempotencyKey: "touch-abc",
	}
	require.NoError(t, p.Enqueue(ctx, "u1", pl))
	require.NoError(t, p.Enqueue(ctx, "u1", pl), "second call must return nil (dedup)")

	entries, err := mr.Stream("apns:queue")
	require.NoError(t, err)
	assert.Len(t, entries, 1, "only first Enqueue should hit the stream")
}

type errSetNXCmd struct {
	redis.Cmdable
}

func (e errSetNXCmd) SetNX(ctx context.Context, key string, value any, ttl time.Duration) *redis.BoolCmd {
	cmd := redis.NewBoolCmd(ctx, "setnx", key, value, ttl)
	cmd.SetErr(errors.New("redis down"))
	return cmd
}

func TestEnqueue_IdempotencySetnxError_FallsThrough(t *testing.T) {
	t.Parallel()
	mr, cmd := setupMiniredis(t)
	clk := clockx.NewFakeClock(time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC))
	sp := redisx.NewStreamPusher(cmd, "apns:queue")
	// idemCmd returns error on SETNX but the stream XADD still uses the
	// real miniredis cmd (so we can observe stream state).
	p := NewRedisStreamsPusher(sp, errSetNXCmd{Cmdable: cmd}, clk, 5*time.Minute)

	err := p.Enqueue(context.Background(), "u1", PushPayload{
		Kind:           PushKindAlert,
		Title:          "hi",
		IdempotencyKey: "touch-xyz",
	})
	require.NoError(t, err, "SETNX error is fail-open")

	entries, err := mr.Stream("apns:queue")
	require.NoError(t, err)
	assert.Len(t, entries, 1, "XADD must still have happened")
}

func TestEnqueue_AlertWithoutTitle_ValidationError(t *testing.T) {
	t.Parallel()
	_, _, p, _ := newTestPusher(t)
	err := p.Enqueue(context.Background(), "u1", PushPayload{Kind: PushKindAlert})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrValidationError))
}

func TestEnqueue_InvalidKind_ValidationError(t *testing.T) {
	t.Parallel()
	_, _, p, _ := newTestPusher(t)
	err := p.Enqueue(context.Background(), "u1", PushPayload{Kind: "weird"})
	require.Error(t, err)
	assert.True(t, errors.Is(err, dto.ErrValidationError))
}

func TestEnqueue_NilConstructorArgs_Panic(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	clk := clockx.NewFakeClock(time.Now())
	sp := redisx.NewStreamPusher(cmd, "apns:queue")

	cases := []struct {
		name string
		fn   func()
	}{
		{"nil stream", func() { _ = NewRedisStreamsPusher(nil, cmd, clk, time.Minute) }},
		{"nil idemCmd", func() { _ = NewRedisStreamsPusher(sp, nil, clk, time.Minute) }},
		{"nil clock", func() { _ = NewRedisStreamsPusher(sp, cmd, nil, time.Minute) }},
		{"zero idemTTL", func() { _ = NewRedisStreamsPusher(sp, cmd, clk, 0) }},
		{"negative idemTTL", func() { _ = NewRedisStreamsPusher(sp, cmd, clk, -time.Second) }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			assert.Panics(t, tc.fn)
		})
	}
}

func TestNoopPusher_Enqueue_AlwaysNil(t *testing.T) {
	t.Parallel()
	var p Pusher = NoopPusher{}
	assert.NoError(t, p.Enqueue(context.Background(), "u1", PushPayload{Kind: PushKindAlert, Title: "x"}))
}
