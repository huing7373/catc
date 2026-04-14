package main

import (
	"context"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/huing7373/catc/server/internal/config"
)

// Runnable is the lifecycle contract every long-lived component
// satisfies. Start must either block (HTTP server loop) or return nil
// once its goroutines are spawned; Final must be idempotent.
type Runnable interface {
	Name() string
	Start(ctx context.Context) error
	Final(ctx context.Context) error
}

// App owns every Runnable and orchestrates startup + graceful shutdown.
type App struct {
	cfg  *config.Config
	runs []Runnable
	stop chan os.Signal
}

// NewApp wires an App with the given runnables. Order matters: Start is
// performed in the slice order, Final in reverse.
func NewApp(cfg *config.Config, runs ...Runnable) *App {
	return &App{
		cfg:  cfg,
		runs: runs,
		stop: make(chan os.Signal, 1),
	}
}

// Run starts every Runnable in its own goroutine, blocks on SIGINT /
// SIGTERM, then calls Final in reverse order with a bounded context.
func (a *App) Run() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	for _, r := range a.runs {
		wg.Add(1)
		go func(r Runnable) {
			defer wg.Done()
			if err := r.Start(ctx); err != nil {
				log.Fatal().Err(err).Str("runnable", r.Name()).Msg("start failed")
			}
		}(r)
	}

	log.Info().
		Int("port", a.cfg.Server.Port).
		Str("mode", a.cfg.Server.Mode).
		Msg("cat server started")

	signal.Notify(a.stop, os.Interrupt, syscall.SIGTERM)
	<-a.stop
	log.Info().Msg("shutdown signal received")

	shutCtx, c := context.WithTimeout(context.Background(), a.cfg.ShutdownTimeout())
	defer c()
	a.shutdown(shutCtx)

	// After Final() on each runnable, their Start goroutines should
	// return. Cancel the root context as a safety net and wait.
	cancel()
	wg.Wait()
}

// shutdown is factored so tests can invoke termination without sending
// an OS signal.
func (a *App) shutdown(ctx context.Context) {
	for i := len(a.runs) - 1; i >= 0; i-- {
		r := a.runs[i]
		fctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		if err := r.Final(fctx); err != nil {
			log.Error().Err(err).Str("runnable", r.Name()).Msg("final failed")
		}
		cancel()
	}
}
