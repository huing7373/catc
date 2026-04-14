package main

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/huing7373/catc/server/internal/config"
)

// fakeRunnable records Start/Final calls for deterministic assertions.
// All shared state lives in *sharedState; per-instance counters use
// atomics so repeated Final is trivially idempotent.
type sharedState struct {
	mu    sync.Mutex
	start []string
	final []string
}

type fakeRunnable struct {
	name     string
	startErr error

	startCalls atomic.Int32
	finalCalls atomic.Int32
	shared     *sharedState

	closeOnce sync.Once
	unblock   chan struct{}
}

func newFake(name string, shared *sharedState) *fakeRunnable {
	return &fakeRunnable{
		name:    name,
		shared:  shared,
		unblock: make(chan struct{}),
	}
}

func (f *fakeRunnable) Name() string { return f.name }

func (f *fakeRunnable) Start(ctx context.Context) error {
	f.startCalls.Add(1)
	if f.shared != nil {
		f.shared.mu.Lock()
		f.shared.start = append(f.shared.start, f.name)
		f.shared.mu.Unlock()
	}
	if f.startErr != nil {
		return f.startErr
	}
	select {
	case <-ctx.Done():
	case <-f.unblock:
	}
	return nil
}

func (f *fakeRunnable) Final(ctx context.Context) error {
	f.finalCalls.Add(1)
	if f.shared != nil {
		f.shared.mu.Lock()
		f.shared.final = append(f.shared.final, f.name)
		f.shared.mu.Unlock()
	}
	f.closeOnce.Do(func() { close(f.unblock) })
	return nil
}

func TestApp_StartAndFinal_OrderAndIdempotence(t *testing.T) {
	shared := &sharedState{}
	r1 := newFake("r1", shared)
	r2 := newFake("r2", shared)
	r3 := newFake("r3", shared)

	cfg := &config.Config{Server: config.ServerCfg{Port: 1, Mode: "release", ShutdownTimeoutSec: 1}}
	app := NewApp(cfg, r1, r2, r3)

	done := make(chan struct{})
	go func() {
		app.Run()
		close(done)
	}()

	// Allow Start goroutines to enter their select.
	time.Sleep(50 * time.Millisecond)

	app.stop <- syscall.SIGTERM

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("app did not shut down within budget")
	}

	for _, r := range []*fakeRunnable{r1, r2, r3} {
		if n := r.startCalls.Load(); n != 1 {
			t.Errorf("%s startCalls = %d", r.name, n)
		}
		if n := r.finalCalls.Load(); n != 1 {
			t.Errorf("%s finalCalls = %d", r.name, n)
		}
	}

	// Read the order snapshot under the lock, then release before the
	// idempotent-Final loop (which also grabs the lock).
	shared.mu.Lock()
	order := append([]string(nil), shared.final...)
	shared.mu.Unlock()

	if len(order) != 3 || order[0] != "r3" || order[1] != "r2" || order[2] != "r1" {
		t.Errorf("final order: %v (expected r3, r2, r1)", order)
	}

	// Calling Final again must be idempotent (no panic, no blocking).
	ctx := context.Background()
	for _, r := range []*fakeRunnable{r1, r2, r3} {
		if err := r.Final(ctx); err != nil {
			t.Errorf("%s idempotent Final: %v", r.name, err)
		}
	}
}

func TestApp_FakeRunnable_StartErrPropagates(t *testing.T) {
	errStart := errors.New("boom")
	r := &fakeRunnable{name: "broken", startErr: errStart, unblock: make(chan struct{})}
	if err := r.Start(context.Background()); err != errStart {
		t.Errorf("fake returned unexpected error: %v", err)
	}
}
