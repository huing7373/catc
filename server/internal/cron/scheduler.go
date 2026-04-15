// Package cron schedules recurring background work. The scheduler
// wraps robfig/cron/v3 and exposes a minimal Runnable contract so the
// App container starts/stops it alongside HTTP and WebSocket.
package cron

import (
	"context"
	"sync"

	"github.com/robfig/cron/v3"
	"github.com/rs/zerolog/log"
)

// Job is the signature required for every scheduled task. Jobs must be
// idempotent: the scheduler may retry or duplicate triggers under load.
type Job func(ctx context.Context) error

// Scheduler is the adapter registered with the App container.
type Scheduler struct {
	c    *cron.Cron
	done chan struct{}
	once sync.Once
}

// NewScheduler builds an idle scheduler. Callers register jobs via
// Register before invoking Start.
func NewScheduler() *Scheduler {
	return &Scheduler{
		c:    cron.New(cron.WithSeconds()),
		done: make(chan struct{}),
	}
}

// Register adds fn under spec (standard cron expression with seconds).
func (s *Scheduler) Register(spec, name string, fn Job) error {
	wrapper := func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := fn(ctx); err != nil {
			log.Error().Err(err).Str("job", name).Msg("cron job failed")
		}
	}
	_, err := s.c.AddFunc(spec, wrapper)
	return err
}

// Name identifies the scheduler in shutdown logs.
func (s *Scheduler) Name() string { return "cron" }

// Start begins firing registered jobs. It returns immediately; job
// execution happens on the cron library's own goroutines.
func (s *Scheduler) Start(ctx context.Context) error {
	s.c.Start()
	<-s.done
	return nil
}

// Final stops the scheduler and waits for in-flight jobs to drain. Safe
// to call multiple times.
func (s *Scheduler) Final(ctx context.Context) error {
	s.once.Do(func() {
		close(s.done)
		stopCtx := s.c.Stop()
		select {
		case <-stopCtx.Done():
		case <-ctx.Done():
		}
	})
	return nil
}

// RegisterJobs is the well-known entry point the wiring layer calls to
// register all scheduled jobs at startup. It is currently empty — Epic
// 2-4 account-deletion cleanup and Epic 7 data summaries will fill in.
//
// TODO(#epic-2-4): register account-deletion sweeper.
// TODO(#epic-7-2): register daily data summary backup job.
func RegisterJobs(s *Scheduler, _ any) error {
	_ = s
	return nil
}
