package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/ids"
)

// --- fakes ---

type fakeApnsRepo struct {
	err       error
	lastCall  *domain.ApnsToken
	callCount int
}

func (f *fakeApnsRepo) Upsert(_ context.Context, t *domain.ApnsToken) error {
	f.callCount++
	cp := *t
	f.lastCall = &cp
	return f.err
}

type fakeSessionRepo struct {
	err        error
	callCount  int
	lastUserID ids.UserID
	lastDev    string
	lastHas    bool
}

func (f *fakeSessionRepo) SetSessionHasApnsToken(_ context.Context, userID ids.UserID, deviceID string, has bool) error {
	f.callCount++
	f.lastUserID = userID
	f.lastDev = deviceID
	f.lastHas = has
	return f.err
}

type fakeLimiter struct {
	allowed bool
	retry   time.Duration
	err     error
	count   int
}

func (f *fakeLimiter) Acquire(_ context.Context, _ ids.UserID) (bool, time.Duration, error) {
	f.count++
	return f.allowed, f.retry, f.err
}

func newOkFakes() (*fakeApnsRepo, *fakeSessionRepo, *fakeLimiter) {
	return &fakeApnsRepo{}, &fakeSessionRepo{}, &fakeLimiter{allowed: true}
}

func fixedSvcClock() *clockx.FakeClock {
	return clockx.NewFakeClock(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
}

// --- tests ---

func TestRegisterApnsToken_HappyPath(t *testing.T) {
	t.Parallel()
	r, sr, lim := newOkFakes()
	clk := fixedSvcClock()
	svc := NewApnsTokenService(r, sr, lim, clk)

	err := svc.RegisterApnsToken(context.Background(), RegisterApnsTokenRequest{
		UserID:      ids.UserID("u1"),
		DeviceID:    "d1",
		Platform:    ids.PlatformWatch,
		DeviceToken: "abc-plain",
	})
	require.NoError(t, err)

	assert.Equal(t, 1, r.callCount)
	require.NotNil(t, r.lastCall)
	assert.Equal(t, ids.UserID("u1"), r.lastCall.UserID)
	assert.Equal(t, ids.PlatformWatch, r.lastCall.Platform)
	assert.Equal(t, "abc-plain", r.lastCall.DeviceToken)
	assert.True(t, r.lastCall.UpdatedAt.Equal(clk.Now()))

	assert.Equal(t, 1, sr.callCount)
	assert.Equal(t, ids.UserID("u1"), sr.lastUserID)
	assert.Equal(t, "d1", sr.lastDev)
	assert.True(t, sr.lastHas)
}

func TestRegisterApnsToken_LimiterBlocks(t *testing.T) {
	t.Parallel()
	r, sr, _ := newOkFakes()
	lim := &fakeLimiter{allowed: false, retry: 2 * time.Second}
	svc := NewApnsTokenService(r, sr, lim, fixedSvcClock())

	err := svc.RegisterApnsToken(context.Background(), RegisterApnsTokenRequest{
		UserID: ids.UserID("u1"), DeviceID: "d1",
		Platform: ids.PlatformWatch, DeviceToken: "tok",
	})
	require.Error(t, err)
	var appErr *dto.AppError
	require.True(t, errors.As(err, &appErr), "expected AppError, got %T", err)
	assert.Equal(t, "RATE_LIMIT_EXCEEDED", appErr.Code)
	assert.Equal(t, 2, appErr.RetryAfter,
		"service must propagate limiter's retry hint; hard-coded 60s default would mislead clients")

	assert.Equal(t, 0, r.callCount, "repo must not be called when rate-limited")
	assert.Equal(t, 0, sr.callCount, "session flag must not flip when rate-limited")
}

// TestRegisterApnsToken_LimiterBlocks_SubSecondRetryRoundsUpTo1 covers
// the ceiling helper's clamp: a 1 ms boundary retry (matches §9.3
// sliding-window behaviour) must surface as Retry-After: 1, never 0 —
// a zero header would tell the client "retry immediately" while the
// limiter already decided the slot is blocked.
func TestRegisterApnsToken_LimiterBlocks_SubSecondRetryRoundsUpTo1(t *testing.T) {
	t.Parallel()
	r, sr, _ := newOkFakes()
	lim := &fakeLimiter{allowed: false, retry: time.Millisecond}
	svc := NewApnsTokenService(r, sr, lim, fixedSvcClock())

	err := svc.RegisterApnsToken(context.Background(), RegisterApnsTokenRequest{
		UserID: ids.UserID("u1"), DeviceID: "d1",
		Platform: ids.PlatformWatch, DeviceToken: "tok",
	})
	require.Error(t, err)
	var appErr *dto.AppError
	require.True(t, errors.As(err, &appErr))
	assert.Equal(t, 1, appErr.RetryAfter,
		"sub-second retry must ceil to 1s — zero would read as 'retry immediately'")
}

func TestRegisterApnsToken_LimiterError_FailsClosed(t *testing.T) {
	t.Parallel()
	r, sr, _ := newOkFakes()
	lim := &fakeLimiter{err: errors.New("redis down")}
	svc := NewApnsTokenService(r, sr, lim, fixedSvcClock())

	err := svc.RegisterApnsToken(context.Background(), RegisterApnsTokenRequest{
		UserID: ids.UserID("u1"), DeviceID: "d1",
		Platform: ids.PlatformWatch, DeviceToken: "tok",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rate limit acquire")
	assert.Equal(t, 0, r.callCount)
	assert.Equal(t, 0, sr.callCount)
}

func TestRegisterApnsToken_RepoError(t *testing.T) {
	t.Parallel()
	r := &fakeApnsRepo{err: errors.New("mongo boom")}
	sr := &fakeSessionRepo{}
	lim := &fakeLimiter{allowed: true}
	svc := NewApnsTokenService(r, sr, lim, fixedSvcClock())

	err := svc.RegisterApnsToken(context.Background(), RegisterApnsTokenRequest{
		UserID: ids.UserID("u1"), DeviceID: "d1",
		Platform: ids.PlatformWatch, DeviceToken: "tok",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "upsert")
	assert.Equal(t, 0, sr.callCount, "session repo not invoked after upsert err")
}

func TestRegisterApnsToken_SessionWriteError_Swallowed(t *testing.T) {
	t.Parallel()
	r := &fakeApnsRepo{}
	sr := &fakeSessionRepo{err: errors.New("session write fail")}
	lim := &fakeLimiter{allowed: true}
	svc := NewApnsTokenService(r, sr, lim, fixedSvcClock())

	err := svc.RegisterApnsToken(context.Background(), RegisterApnsTokenRequest{
		UserID: ids.UserID("u1"), DeviceID: "d1",
		Platform: ids.PlatformWatch, DeviceToken: "tok",
	})
	assert.NoError(t, err,
		"session flag write failure is best-effort — main request stays 200 (AC11 fail-open)")
	assert.Equal(t, 1, r.callCount)
	assert.Equal(t, 1, sr.callCount)
}

func TestRegisterApnsToken_ClockInjection(t *testing.T) {
	t.Parallel()
	r, sr, lim := newOkFakes()
	clk := clockx.NewFakeClock(time.Date(2026, 4, 20, 0, 0, 0, 0, time.UTC))
	svc := NewApnsTokenService(r, sr, lim, clk)

	require.NoError(t, svc.RegisterApnsToken(context.Background(), RegisterApnsTokenRequest{
		UserID: ids.UserID("u1"), DeviceID: "d1",
		Platform: ids.PlatformIphone, DeviceToken: "tok",
	}))
	require.NotNil(t, r.lastCall)
	assert.True(t, r.lastCall.UpdatedAt.Equal(clk.Now()),
		"service must stamp UpdatedAt via injected clock, not time.Now()")
}

func TestRegisterApnsToken_NilDepsPanic(t *testing.T) {
	t.Parallel()
	r, sr, lim := newOkFakes()
	clk := fixedSvcClock()

	cases := []struct {
		name   string
		repo   apnsTokenRepo
		sess   userSessionRepo
		lim    apnsTokenRegisterRateLimiter
		clkArg clockx.Clock
	}{
		{name: "nil repo", repo: nil, sess: sr, lim: lim, clkArg: clk},
		{name: "nil sess", repo: r, sess: nil, lim: lim, clkArg: clk},
		{name: "nil lim", repo: r, sess: sr, lim: nil, clkArg: clk},
		{name: "nil clk", repo: r, sess: sr, lim: lim, clkArg: nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assert.Panics(t, func() {
				NewApnsTokenService(c.repo, c.sess, c.lim, c.clkArg)
			})
		})
	}
}
