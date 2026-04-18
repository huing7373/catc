package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/ws"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/redisx"
)

type fakeDedupStore struct{}

func (fakeDedupStore) Acquire(_ context.Context, _ string) (bool, error) { return true, nil }
func (fakeDedupStore) StoreResult(_ context.Context, _ string, _ redisx.DedupResult) error {
	return nil
}
func (fakeDedupStore) GetResult(_ context.Context, _ string) (redisx.DedupResult, bool, error) {
	return redisx.DedupResult{}, false, nil
}

func newStubDispatcher() *ws.Dispatcher {
	return ws.NewDispatcher(fakeDedupStore{}, clockx.NewRealClock())
}

func noopHandler(_ context.Context, _ *ws.Client, env ws.Envelope) (json.RawMessage, error) {
	return env.Payload, nil
}

func TestValidateRegistryConsistency_DebugModeFullyRegistered(t *testing.T) {
	t.Parallel()

	d := newStubDispatcher()
	d.Register("debug.echo", noopHandler)
	d.RegisterDedup("debug.echo.dedup", noopHandler)
	d.Register("session.resume", noopHandler)
	d.Register("room.join", noopHandler)
	d.Register("action.update", noopHandler)
	// action.broadcast is Direction=down; MUST NOT be registered. The
	// drift check exempts downstream-only types (Story 10.1).

	require.NoError(t, validateRegistryConsistency(d, "debug"))
}

func TestValidateRegistryConsistency_DownstreamOnlyExempt(t *testing.T) {
	t.Parallel()

	// action.broadcast is Direction=down: it lives in dto.WSMessages so the
	// /v1/platform/ws-registry endpoint advertises it, but it never flows
	// through Dispatcher (server→client push). The consistency check must
	// therefore exempt it from the "must be registered in debug mode"
	// requirement. Equally, registering a downstream type would also be
	// wrong — but that case is already caught by unknownRegistered in the
	// existing machinery (action.broadcast IS in WSMessages, so it
	// wouldn't trip unknownRegistered; the exemption is one-sided — we
	// don't test registering a downstream type because it would panic at
	// Dispatch time anyway when Direction conventions are respected).
	d := newStubDispatcher()
	d.Register("debug.echo", noopHandler)
	d.RegisterDedup("debug.echo.dedup", noopHandler)
	d.Register("session.resume", noopHandler)
	d.Register("room.join", noopHandler)
	d.Register("action.update", noopHandler)

	// Debug mode must accept this configuration despite action.broadcast
	// being in WSMessages but not registered.
	require.NoError(t, validateRegistryConsistency(d, "debug"))
}

func TestValidateRegistryConsistency_ReleaseModeNothingRegistered(t *testing.T) {
	t.Parallel()

	// At Story 0.14 every WSMessages entry is DebugOnly, so release mode has
	// nothing to register and MUST pass.
	d := newStubDispatcher()
	require.NoError(t, validateRegistryConsistency(d, "release"))
}

func TestValidateRegistryConsistency_UnknownRegisteredFails(t *testing.T) {
	t.Parallel()

	d := newStubDispatcher()
	d.Register("ghost.type", noopHandler)

	err := validateRegistryConsistency(d, "debug")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ghost.type",
		"error must name the drifting type for triage")
	assert.Contains(t, err.Error(), "unknownRegistered",
		"error must classify the drift bucket")
}

func TestValidateRegistryConsistency_DebugModeMissingRegistrationFails(t *testing.T) {
	t.Parallel()

	// Debug mode drift the endpoint can actually expose: dto.WSMessages still
	// lists a type (advertised to clients on /v1/platform/ws-registry) but
	// the dispatcher registration was removed / forgotten, so clients that
	// send it get UNKNOWN_MESSAGE_TYPE. Simulate by registering only a
	// proper subset of the three WSMessages entries.
	d := newStubDispatcher()
	d.Register("debug.echo", noopHandler)
	d.RegisterDedup("debug.echo.dedup", noopHandler)
	// session.resume deliberately missing.

	err := validateRegistryConsistency(d, "debug")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session.resume",
		"error must name the drifting type for triage")
	assert.Contains(t, err.Error(), "missingInDebug",
		"error must classify the drift bucket")
}

func TestValidateRegistryConsistency_DebugOnlyInReleaseFails(t *testing.T) {
	t.Parallel()

	// Registering a DebugOnly entry (session.resume) in release mode is the
	// Story 0.12 regression the guard is supposed to catch.
	d := newStubDispatcher()
	d.Register("session.resume", noopHandler)

	err := validateRegistryConsistency(d, "release")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "session.resume")
	assert.Contains(t, err.Error(), "debugOnlyInRelease")
}
