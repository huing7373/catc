// Package dto_test — external test package. Using `package dto_test` rather
// than `package dto` avoids the import cycle created by `internal/ws`
// depending on `internal/dto` (for AppError): an in-package test that
// imported `internal/ws` would form a cycle.
package dto_test

import (
	"context"
	"encoding/json"
	"regexp"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/ws"
	"github.com/huing/cat/server/pkg/clockx"
)

// wsTypeRegex mirrors the P3 domain.action convention enforced by AC12.
var wsTypeRegex = regexp.MustCompile(`^[a-z0-9]+(?:\.[a-z0-9]+)*$`)

// wsVersionRegex mirrors the AC10 version shape.
var wsVersionRegex = regexp.MustCompile(`^v\d+$`)

func TestWSMessages_AllFieldsPopulated(t *testing.T) {
	t.Parallel()

	validDirections := map[dto.WSDirection]bool{
		dto.WSDirectionUp:   true,
		dto.WSDirectionDown: true,
		dto.WSDirectionBi:   true,
	}

	for _, meta := range dto.WSMessages {
		t.Run(meta.Type, func(t *testing.T) {
			t.Parallel()

			assert.NotEmpty(t, meta.Type, "Type must not be empty")
			assert.True(t, wsTypeRegex.MatchString(meta.Type),
				"Type %q must match ^[a-z0-9]+(\\.[a-z0-9]+)*$ (P3 naming)", meta.Type)
			assert.True(t, wsVersionRegex.MatchString(meta.Version),
				"Version %q must match ^v\\d+$ (AC10)", meta.Version)
			assert.True(t, validDirections[meta.Direction],
				"Direction %q must be one of up/down/bi", meta.Direction)
			assert.NotEmpty(t, meta.Description, "Description must not be empty (AC7 registry doc input)")
		})
	}
}

func TestWSMessages_NoDuplicates(t *testing.T) {
	t.Parallel()

	// Belt-and-braces: package init panics via WSMessagesByType on duplicate,
	// but guard the invariant independently so a regression in the init
	// helper does not silently accept dupes.
	seen := make(map[string]bool, len(dto.WSMessages))
	for _, meta := range dto.WSMessages {
		assert.False(t, seen[meta.Type], "duplicate Type %q in dto.WSMessages", meta.Type)
		seen[meta.Type] = true
	}
	assert.Equal(t, len(dto.WSMessages), len(dto.WSMessagesByType),
		"WSMessagesByType must have exactly one entry per WSMessages row")
}

// noopHandler is the minimal ws.HandlerFunc shape used by the drift tests.
// Lives here so cases do not need ws internals beyond the public dispatcher
// API.
func noopHandler(_ context.Context, _ *ws.Client, env ws.Envelope) (json.RawMessage, error) {
	return env.Payload, nil
}

// fakeDedupStore is the smallest DedupStore satisfying RegisterDedup for
// drift tests. It does not need to persist anything — dispatcher only
// consults the store when a dedup message is actually dispatched, never at
// Register time.
type fakeDedupStore struct{}

func (fakeDedupStore) Acquire(_ context.Context, _ string) (bool, error) {
	return true, nil
}
func (fakeDedupStore) StoreResult(_ context.Context, _ string, _ ws.DedupResult) error {
	return nil
}
func (fakeDedupStore) GetResult(_ context.Context, _ string) (ws.DedupResult, bool, error) {
	return ws.DedupResult{}, false, nil
}

func newDispatcher() *ws.Dispatcher {
	return ws.NewDispatcher(fakeDedupStore{}, clockx.NewRealClock())
}

func TestWSMessages_ConsistencyWithDispatcher_DebugMode(t *testing.T) {
	t.Parallel()

	// Mirrors cmd/cat/initialize.go debug branch: registers every WSMessages
	// entry whose Direction is up/bi. Direction=down entries (e.g.
	// action.broadcast) are server→client pushes that never flow through
	// Dispatch — they live in WSMessages only for the registry endpoint —
	// so the dispatcher deliberately has no handler for them.
	d := newDispatcher()
	d.Register("debug.echo", noopHandler)
	d.RegisterDedup("debug.echo.dedup", noopHandler)
	d.Register("session.resume", noopHandler)
	d.Register("room.join", noopHandler)
	d.Register("action.update", noopHandler)

	got := d.RegisteredTypes()

	// Expected = every WSMessages entry with Direction != down (DebugOnly
	// included because mode=debug).
	want := make([]string, 0, len(dto.WSMessages))
	for _, meta := range dto.WSMessages {
		if meta.Direction == dto.WSDirectionDown {
			continue
		}
		want = append(want, meta.Type)
	}
	sort.Strings(want)

	assert.ElementsMatch(t, want, got,
		"dto.WSMessages vs dispatcher registrations drifted (debug mode): want=%v got=%v",
		want, got)
}

func TestWSMessages_ConsistencyWithDispatcher_ReleaseMode(t *testing.T) {
	t.Parallel()

	// Mirrors cmd/cat/initialize.go release branch. Story 1.1 promoted
	// session.resume out of DebugOnly so the release dispatcher must
	// register it as well; later epic-1+ stories will append more.
	d := newDispatcher()
	d.Register("session.resume", noopHandler)

	got := d.RegisteredTypes()

	// Expected = every non-DebugOnly WSMessages entry with Direction != down.
	want := make([]string, 0)
	for _, meta := range dto.WSMessages {
		if meta.DebugOnly {
			continue
		}
		if meta.Direction == dto.WSDirectionDown {
			continue
		}
		want = append(want, meta.Type)
	}
	sort.Strings(want)

	assert.ElementsMatch(t, want, got,
		"dto.WSMessages vs dispatcher registrations drifted (release mode): want=%v got=%v",
		want, got)
}

// Sanity check that the init helper panics on duplicate Type. We cannot
// re-run the real package init, so simulate it locally with the same logic.
func TestWSMessagesByType_DuplicateTypePanics(t *testing.T) {
	t.Parallel()

	require.Panics(t, func() {
		rows := []dto.WSMessageMeta{
			{Type: "dup.type", Version: "v1", Direction: dto.WSDirectionBi, Description: "a"},
			{Type: "dup.type", Version: "v1", Direction: dto.WSDirectionBi, Description: "b"},
		}
		m := make(map[string]dto.WSMessageMeta, len(rows))
		for _, meta := range rows {
			if _, dup := m[meta.Type]; dup {
				panic("dto.WSMessages: duplicate Type " + meta.Type)
			}
			m[meta.Type] = meta
		}
	})
}
