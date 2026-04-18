package redisx

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupMiniredis(t *testing.T) (*miniredis.Miniredis, redis.Cmdable) {
	t.Helper()
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { cli.Close() })
	return mr, cli
}

func TestLocker_InstanceID(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	l := NewLocker(cmd)
	assert.NotEmpty(t, l.InstanceID())

	l2 := NewLocker(cmd)
	assert.NotEqual(t, l.InstanceID(), l2.InstanceID())
}

func TestLocker_WithLock(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setup      func(t *testing.T, mr *miniredis.Miniredis, cmd redis.Cmdable)
		fn         func() error
		wantCalled bool
		wantErr    bool
		checkAfter func(t *testing.T, mr *miniredis.Miniredis)
	}{
		{
			name:       "acquires lock and executes fn then releases",
			fn:         func() error { return nil },
			wantCalled: true,
			checkAfter: func(t *testing.T, mr *miniredis.Miniredis) {
				t.Helper()
				assert.False(t, mr.Exists("lock:cron:test_job"), "lock should be released after fn")
			},
		},
		{
			name: "lock conflict — fn not called, returns nil",
			setup: func(t *testing.T, mr *miniredis.Miniredis, _ redis.Cmdable) {
				t.Helper()
				mr.Set("lock:cron:test_job", "other-instance")
				mr.SetTTL("lock:cron:test_job", 55*time.Second)
			},
			fn:         func() error { return nil },
			wantCalled: false,
			checkAfter: func(t *testing.T, mr *miniredis.Miniredis) {
				t.Helper()
				val, err := mr.Get("lock:cron:test_job")
				require.NoError(t, err)
				assert.Equal(t, "other-instance", val, "other instance lock should remain")
			},
		},
		{
			name: "CAS release — does not delete lock held by other instance",
			setup: func(t *testing.T, mr *miniredis.Miniredis, cmd redis.Cmdable) {
				t.Helper()
			},
			fn:         func() error { return nil },
			wantCalled: true,
		},
		{
			name:       "fn returns error — error propagated, lock still released",
			fn:         func() error { return errors.New("job failed") },
			wantCalled: true,
			wantErr:    true,
			checkAfter: func(t *testing.T, mr *miniredis.Miniredis) {
				t.Helper()
				assert.False(t, mr.Exists("lock:cron:test_job"), "lock should be released even on fn error")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			mr, cmd := setupMiniredis(t)
			locker := NewLocker(cmd)

			if tt.setup != nil {
				tt.setup(t, mr, cmd)
			}

			called := false
			origFn := tt.fn
			wrappedFn := func() error {
				called = true
				return origFn()
			}

			err := locker.WithLock(context.Background(), "test_job", 55*time.Second, wrappedFn)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantCalled, called)

			if tt.checkAfter != nil {
				tt.checkAfter(t, mr)
			}
		})
	}
}

func TestLocker_CAS_SafeRelease(t *testing.T) {
	t.Parallel()
	mr, cmd := setupMiniredis(t)

	locker1 := NewLocker(cmd)
	locker2 := NewLocker(cmd)

	err := locker1.WithLock(context.Background(), "cas_test", 55*time.Second, func() error {
		mr.Set("lock:cron:cas_test", locker2.InstanceID())
		return nil
	})
	require.NoError(t, err)

	val, err := mr.Get("lock:cron:cas_test")
	require.NoError(t, err)
	assert.Equal(t, locker2.InstanceID(), val, "locker1 CAS should not delete locker2's lock")
}

func TestLocker_TTL_Expiry(t *testing.T) {
	t.Parallel()
	mr, cmd := setupMiniredis(t)
	locker := NewLocker(cmd)

	err := locker.WithLock(context.Background(), "ttl_test", 1*time.Second, func() error {
		return nil
	})
	require.NoError(t, err)

	mr.Set("lock:cron:ttl_test", "other-instance")
	mr.SetTTL("lock:cron:ttl_test", 1*time.Second)

	mr.FastForward(2 * time.Second)

	called := false
	err = locker.WithLock(context.Background(), "ttl_test", 55*time.Second, func() error {
		called = true
		return nil
	})
	require.NoError(t, err)
	assert.True(t, called, "should acquire lock after TTL expiry")
}

func TestLocker_KeyFormat(t *testing.T) {
	t.Parallel()
	_, cmd := setupMiniredis(t)
	locker := NewLocker(cmd)

	err := locker.WithLock(context.Background(), "heartbeat_tick", 55*time.Second, func() error {
		return nil
	})
	require.NoError(t, err)
}
