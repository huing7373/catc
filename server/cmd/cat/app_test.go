package main

import (
	"context"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockRunnable struct {
	name     string
	started  bool
	finaled  bool
	startSeq int
	finalSeq int
	mu       sync.Mutex
}

var (
	seqMu     sync.Mutex
	globalSeq int
)

func nextSeq() int {
	seqMu.Lock()
	defer seqMu.Unlock()
	globalSeq++
	return globalSeq
}

func (m *mockRunnable) Name() string { return m.name }

func (m *mockRunnable) Start(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
	m.startSeq = nextSeq()
	return nil
}

func (m *mockRunnable) Final(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.finaled = true
	m.finalSeq = nextSeq()
	return nil
}

func TestApp_StartsAndFinalsAllRunnables(t *testing.T) {
	globalSeq = 0

	r1 := &mockRunnable{name: "first"}
	r2 := &mockRunnable{name: "second"}
	app := NewApp(r1, r2)

	go func() {
		time.Sleep(100 * time.Millisecond)
		app.stop <- syscall.SIGTERM
	}()

	app.Run()

	r1.mu.Lock()
	defer r1.mu.Unlock()
	r2.mu.Lock()
	defer r2.mu.Unlock()

	assert.True(t, r1.started, "first runnable should be started")
	assert.True(t, r2.started, "second runnable should be started")
	assert.True(t, r1.finaled, "first runnable should be finaled")
	assert.True(t, r2.finaled, "second runnable should be finaled")
}

func TestApp_FinalInReverseOrder(t *testing.T) {
	globalSeq = 0

	r1 := &mockRunnable{name: "first"}
	r2 := &mockRunnable{name: "second"}
	r3 := &mockRunnable{name: "third"}
	app := NewApp(r1, r2, r3)

	go func() {
		time.Sleep(100 * time.Millisecond)
		app.stop <- syscall.SIGTERM
	}()

	app.Run()

	r1.mu.Lock()
	defer r1.mu.Unlock()
	r2.mu.Lock()
	defer r2.mu.Unlock()
	r3.mu.Lock()
	defer r3.mu.Unlock()

	require.True(t, r3.finalSeq < r2.finalSeq, "third should final before second")
	require.True(t, r2.finalSeq < r1.finalSeq, "second should final before first")
}

func TestApp_NoRunnables(t *testing.T) {
	app := NewApp()

	go func() {
		time.Sleep(50 * time.Millisecond)
		app.stop <- syscall.SIGTERM
	}()

	app.Run()
}
