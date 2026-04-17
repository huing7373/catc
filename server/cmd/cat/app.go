package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
)

type Runnable interface {
	Name() string
	Start(ctx context.Context) error
	Final(ctx context.Context) error
}

type ReadySignaler interface {
	Ready() <-chan struct{}
}

type App struct {
	runs    []Runnable
	stop    chan os.Signal
	onReady func()
}

func NewApp(runs ...Runnable) *App {
	return &App{
		runs: runs,
		stop: make(chan os.Signal, 1),
	}
}

func (a *App) OnReady(fn func()) {
	a.onReady = fn
}

func (a *App) Run() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	startErr := make(chan error, len(a.runs))
	for _, r := range a.runs {
		go func(r Runnable) {
			if err := r.Start(ctx); err != nil {
				log.Error().Err(err).Str("runnable", r.Name()).Msg("start failed")
				startErr <- err
			}
		}(r)
	}

	// Wait for all ReadySignaler runnables to be ready before marking app ready.
	for _, r := range a.runs {
		if rs, ok := r.(ReadySignaler); ok {
			select {
			case <-rs.Ready():
			case err := <-startErr:
				startErr <- err // put it back for the shutdown path
				goto shutdown
			}
		}
	}
	if a.onReady != nil {
		a.onReady()
	}

	signal.Notify(a.stop, os.Interrupt, syscall.SIGTERM)
	select {
	case <-a.stop:
	case <-startErr:
	}
shutdown:
	cancel()

	shutCtx, c := context.WithTimeout(context.Background(), 30*time.Second)
	defer c()

	for i := len(a.runs) - 1; i >= 0; i-- {
		if err := a.runs[i].Final(shutCtx); err != nil {
			log.Error().Err(err).Str("runnable", a.runs[i].Name()).Msg("final failed")
		}
	}

	select {
	case <-startErr:
		os.Exit(1)
	default:
	}
}
