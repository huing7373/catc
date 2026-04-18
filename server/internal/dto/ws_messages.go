// Package dto hosts the canonical WS message registry (source of truth) and
// the AppError catalog. The WS message registry below is the single source
// of truth for the envelope.type strings understood by internal/ws.Dispatcher.
//
// Downstream artifacts that must stay in lock-step with WSMessages:
//   - docs/api/ws-message-registry.md (human-readable registry, hand-maintained)
//   - docs/api/openapi.yaml            (HTTP surface for GET /v1/platform/ws-registry)
//   - internal/ws/dispatcher.go        (Register / RegisterDedup — one entry each)
//
// CI drift guards (Story 0.14):
//   - internal/dto/ws_messages_test.go      — compile-time invariants + dispatcher parity
//   - cmd/cat/initialize_test.go            — validateRegistryConsistency fail-fast
//   - cmd/cat/integration_test.go           — TestWSRegistryEndpoint wire shape
package dto

// WSDirection describes whether a message flows upstream (client→server),
// downstream (server→client push), or bidirectional (client-initiated RPC
// expecting a server response).
type WSDirection string

const (
	WSDirectionUp   WSDirection = "up"
	WSDirectionDown WSDirection = "down"
	WSDirectionBi   WSDirection = "bi"
)

// WSMessageMeta is the compile-time metadata for every WS message type the
// server currently understands. Treat values as immutable — mutation would
// race with handler.PlatformHandler.WSRegistry which reads WSMessages without
// locking.
type WSMessageMeta struct {
	Type          string      // canonical envelope.type, e.g. "session.resume"
	Version       string      // MVP: "v1" for all; bump independently per Story 0.14 AC10
	Direction     WSDirection // up | down | bi
	RequiresAuth  bool        // false only for unauthenticated types once introduced
	RequiresDedup bool        // true ⇔ dispatcher uses RegisterDedup
	DebugOnly     bool        // true if only registered when cfg.Server.Mode == "debug"
	Description   string      // one-line English summary for ws-message-registry.md
}

// WSMessages is the authoritative list. Every dispatcher Register /
// RegisterDedup call MUST have exactly one entry here; the Story 0.14 AC4
// consistency test enforces the invariant.
//
// To add a new message: (1) append an entry below, (2) Register/RegisterDedup
// in cmd/cat/initialize.go, (3) update docs/api/ws-message-registry.md,
// (4) run `bash scripts/build.sh --test`. Out-of-order additions fail the
// AC4 test — that is the design intent (G2 drift prevention).
var WSMessages = []WSMessageMeta{
	{
		Type:          "session.resume",
		Version:       "v1",
		Direction:     WSDirectionBi,
		RequiresAuth:  true,
		RequiresDedup: false,
		DebugOnly:     true,
		Description:   "Client requests a full session snapshot (user/friends/cat_state/skins/blindboxes/room) cached 60s.",
	},
	{
		Type:          "debug.echo",
		Version:       "v1",
		Direction:     WSDirectionBi,
		RequiresAuth:  true,
		RequiresDedup: false,
		DebugOnly:     true,
		Description:   "Debug-only: server echoes request payload verbatim. No business effect.",
	},
	{
		Type:          "debug.echo.dedup",
		Version:       "v1",
		Direction:     WSDirectionBi,
		RequiresAuth:  true,
		RequiresDedup: true,
		DebugOnly:     true,
		Description:   "Debug-only: exercises the dedup middleware; idempotent replay of envelope.id returns cached result.",
	},
	{
		Type:          "room.join",
		Version:       "v1",
		Direction:     WSDirectionBi,
		RequiresAuth:  true,
		RequiresDedup: false,
		DebugOnly:     true,
		Description:   "Client joins a room and receives a members snapshot. MVP only, removed when Epic 4.1 ships.",
	},
	{
		Type:          "action.update",
		Version:       "v1",
		Direction:     WSDirectionUp,
		RequiresAuth:  true,
		RequiresDedup: false,
		DebugOnly:     true,
		Description:   "Client publishes current action; server fans out to other room members. MVP only, removed when Epic 4.1 ships.",
	},
	{
		Type:          "action.broadcast",
		Version:       "v1",
		Direction:     WSDirectionDown,
		RequiresAuth:  true,
		RequiresDedup: false,
		DebugOnly:     true,
		Description:   "Server push: another room member's current action. MVP only, removed when Epic 4.1 ships.",
	},
}

// WSMessagesByType is an O(1) lookup map keyed by Type, built once at package
// init. Callers MUST treat it as read-only; mutation would race with
// concurrent handler reads.
//
// MVP invariant: Type is unique. When Story 0.14 AC10 v1/v2 coexistence lands
// (same Type with different Version), rekey on Type+Version.
var WSMessagesByType = func() map[string]WSMessageMeta {
	m := make(map[string]WSMessageMeta, len(WSMessages))
	for _, meta := range WSMessages {
		if _, dup := m[meta.Type]; dup {
			panic("dto.WSMessages: duplicate Type " + meta.Type)
		}
		m[meta.Type] = meta
	}
	return m
}()

