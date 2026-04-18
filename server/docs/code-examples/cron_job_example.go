package examples

import (
	"context"

	"github.com/redis/go-redis/v9"

	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/logx"
)

// ExampleCronJob demonstrates the standard cron job pattern.
//
// Every cron job receives its dependencies and is wrapped with
// locker.WithLock in scheduler.registerJobs — the job itself
// does not manage locking.
//
// Key conventions:
//   - Use clock.Now() instead of time.Now() (M9)
//   - Check ctx.Done() in long loops (M4)
//   - Return error on failure; the scheduler logs it
func ExampleCronJob(ctx context.Context, cmd redis.Cmdable, clock clockx.Clock) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
	}

	now := clock.Now()
	logx.Ctx(ctx).Info().
		Str("job", "example_job").
		Time("timestamp", now).
		Msg("example cron job executed")

	return nil
}
