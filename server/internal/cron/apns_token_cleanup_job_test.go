package cron

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/pkg/clockx"
)

type fakeCleaner struct {
	lastCutoff time.Time
	count      int64
	err        error
	called     int
}

func (f *fakeCleaner) DeleteExpired(_ context.Context, cutoff time.Time) (int64, error) {
	f.called++
	f.lastCutoff = cutoff
	return f.count, f.err
}

func TestApnsTokenCleanupJob_CutoffHonoursRetention(t *testing.T) {
	t.Parallel()
	fixed := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	clk := clockx.NewFakeClock(fixed)
	cleaner := &fakeCleaner{count: 7}

	err := apnsTokenCleanupJob(context.Background(), cleaner, clk, 30*24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, cleaner.called)
	assert.Equal(t, fixed.Add(-30*24*time.Hour), cleaner.lastCutoff)
	assert.Equal(t, time.Date(2026, 3, 19, 12, 0, 0, 0, time.UTC), cleaner.lastCutoff)
}

func TestApnsTokenCleanupJob_CutoffNonDefaultRetention(t *testing.T) {
	t.Parallel()
	fixed := time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC)
	clk := clockx.NewFakeClock(fixed)
	cleaner := &fakeCleaner{}

	err := apnsTokenCleanupJob(context.Background(), cleaner, clk, 7*24*time.Hour)
	require.NoError(t, err)
	assert.Equal(t, fixed.Add(-7*24*time.Hour), cleaner.lastCutoff,
		"retention must be honoured end-to-end, not overridden by a hard-coded constant")
}

func TestApnsTokenCleanupJob_CleanerError_Propagates(t *testing.T) {
	t.Parallel()
	clk := clockx.NewFakeClock(time.Date(2026, 4, 18, 12, 0, 0, 0, time.UTC))
	sentinel := errors.New("mongo unreachable")
	cleaner := &fakeCleaner{err: sentinel}

	err := apnsTokenCleanupJob(context.Background(), cleaner, clk, 30*24*time.Hour)
	assert.ErrorIs(t, err, sentinel)
}
