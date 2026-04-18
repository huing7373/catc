package cron

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/logx"
)

const cronLastTickKey = "cron:last_tick"

func heartbeatTick(ctx context.Context, cmd redis.Cmdable, clock clockx.Clock) error {
	now := clock.Now().Format(time.RFC3339)
	if err := cmd.Set(ctx, cronLastTickKey, now, 0).Err(); err != nil {
		return err
	}
	logx.Ctx(ctx).Info().Str("job", "heartbeat_tick").Str("tick", now).Msg("cron heartbeat")
	return nil
}
