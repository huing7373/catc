package cron

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/redisx"
)

var _ interface {
	Name() string
	Start(ctx context.Context) error
	Final(ctx context.Context) error
} = (*Scheduler)(nil)

func setupTestScheduler(t *testing.T) (*miniredis.Miniredis, *Scheduler) {
	t.Helper()
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { cli.Close() })

	locker := redisx.NewLocker(cli)
	clock := clockx.NewFakeClock(time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC))
	sch := NewScheduler(locker, cli, clock)
	return mr, sch
}

func TestScheduler_Name(t *testing.T) {
	t.Parallel()
	_, sch := setupTestScheduler(t)
	assert.Equal(t, "cron_scheduler", sch.Name())
}

func TestHeartbeatTick_SetsRedisKey(t *testing.T) {
	t.Parallel()
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { cli.Close() })

	fixedTime := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	clock := clockx.NewFakeClock(fixedTime)

	err := heartbeatTick(context.Background(), cli, clock)
	require.NoError(t, err)

	val, err := mr.Get(cronLastTickKey)
	require.NoError(t, err)
	assert.Equal(t, "2026-04-18T12:00:00Z", val)
}

func TestScheduler_StartAndFinal(t *testing.T) {
	t.Parallel()
	_, sch := setupTestScheduler(t)

	err := sch.Start(context.Background())
	require.NoError(t, err)

	err = sch.Final(context.Background())
	require.NoError(t, err)
}
