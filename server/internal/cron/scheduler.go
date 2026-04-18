package cron

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"

	"github.com/huing/cat/server/internal/push"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/redisx"
)

const defaultLockTTL = 55 * time.Second

type Scheduler struct {
	cron                 *cron.Cron
	locker               *redisx.Locker
	redisCmd             redis.Cmdable
	clock                clockx.Clock
	tokenCleaner         push.TokenCleaner
	tokenExpiryRetention time.Duration
	cancel               context.CancelFunc
	ctx                  context.Context
}

// NewScheduler accepts the push.TokenCleaner used by the apns_token_cleanup
// job plus the operator-configured retention window (cfg.APNs.TokenExpiryDays
// converted to Duration). Epic 0 passes push.EmptyTokenCleaner{}; Story 1.4
// swaps in a real repository without changing this signature further.
//
// tokenExpiry must be > 0. Validation is enforced by config.validateAPNs; a
// non-positive value here is a programming error, not a runtime condition.
func NewScheduler(locker *redisx.Locker, redisCmd redis.Cmdable, clock clockx.Clock, tokenCleaner push.TokenCleaner, tokenExpiry time.Duration) *Scheduler {
	if tokenCleaner == nil {
		panic("cron.NewScheduler: tokenCleaner must not be nil")
	}
	if tokenExpiry <= 0 {
		panic("cron.NewScheduler: tokenExpiry must be > 0")
	}
	return &Scheduler{
		cron:                 cron.New(cron.WithChain(cron.Recover(cronLogger{}))),
		locker:               locker,
		redisCmd:             redisCmd,
		clock:                clock,
		tokenCleaner:         tokenCleaner,
		tokenExpiryRetention: tokenExpiry,
	}
}

func (s *Scheduler) Name() string { return "cron_scheduler" }

func (s *Scheduler) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.registerJobs()
	s.cron.Start()
	log.Info().Str("instanceId", s.locker.InstanceID()).Msg("cron scheduler started")
	return nil
}

func (s *Scheduler) Final(_ context.Context) error {
	s.cancel()
	stopCtx := s.cron.Stop()
	<-stopCtx.Done()
	log.Info().Msg("cron scheduler stopped")
	return nil
}

func (s *Scheduler) registerJobs() {
	s.addLockedJob("@every 1m", "heartbeat_tick", func(ctx context.Context) error {
		return heartbeatTick(ctx, s.redisCmd, s.clock)
	})
	s.addLockedJob("@daily", "apns_token_cleanup", func(ctx context.Context) error {
		return apnsTokenCleanupJob(ctx, s.tokenCleaner, s.clock, s.tokenExpiryRetention)
	})
}

func (s *Scheduler) addLockedJob(spec string, name string, fn func(ctx context.Context) error) {
	_, err := s.cron.AddFunc(spec, func() {
		if err := s.locker.WithLock(s.ctx, name, defaultLockTTL, func() error {
			return fn(s.ctx)
		}); err != nil {
			log.Error().Err(err).Str("job", name).Msg("cron job failed")
		}
	})
	if err != nil {
		log.Fatal().Err(err).Str("job", name).Msg("failed to register cron job")
	}
}

type cronLogger struct{}

func (cronLogger) Info(_ string, _ ...any)  {}
func (cronLogger) Error(err error, msg string, keysAndValues ...any) {
	log.Error().Err(err).Interface("details", keysAndValues).Msg(msg)
}
