package cron

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/huing/cat/server/internal/push"
	"github.com/huing/cat/server/pkg/clockx"
)

// apnsTokenCleanupJob deletes every apns_tokens row with updatedAt older
// than (now - retention). The retention window is operator-controlled via
// cfg.APNs.TokenExpiryDays (plumbed in by the Scheduler) so deployments
// can tune NFR-SEC-7 without a code change — the default is 30 days
// (applyDefaults in internal/config).
//
// Errors propagate so the scheduler log records the failure, but
// returning an error does not retry within the same tick — the next
// @daily run will try again.
func apnsTokenCleanupJob(ctx context.Context, cleaner push.TokenCleaner, clock clockx.Clock, retention time.Duration) error {
	cutoff := clock.Now().Add(-retention)
	count, err := cleaner.DeleteExpired(ctx, cutoff)
	if err != nil {
		log.Error().Err(err).
			Str("action", "apns_token_cleanup").
			Time("cutoff", cutoff).
			Dur("retention", retention).
			Msg("apns token cleanup failed")
		return err
	}
	log.Info().
		Str("action", "apns_token_cleanup").
		Int64("deletedCount", count).
		Time("cutoff", cutoff).
		Dur("retention", retention).
		Msg("apns token cleanup ok")
	return nil
}
