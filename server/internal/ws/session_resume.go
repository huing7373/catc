// Package ws session_resume defines the consumer-side interfaces and skeleton
// handler for the WS `session.resume` RPC. It satisfies the Story 0.12 scope:
// a Redis Hash-backed cache (resume_cache:{userId}, TTL 60s) that throttles
// repeat resume calls within the configured window so the J4 Watch
// reconnect-storm scenario cannot exhaust Mongo / provider connection pools
// (FR42, NFR-PERF-3, NFR-PERF-6, NFR-OBS-5).
//
// This story ships only the skeleton: six Provider interfaces (User / Friends /
// CatState / Skins / Blindboxes / RoomSnapshot) and paired Empty*Provider
// implementations that emit safe-JSON zero values (null / []). Later stories
// (Story 4.5 for the full snapshot handler, and the authoritative-write stories
// 1.5 / 2.2 / 3.2 / 6.4 / 7.3 that invalidate) replace the Empty providers via
// initialize.go without touching this file (P2 consumer-side interface pattern,
// same approach as DedupStore in dedup.go and Blacklist / ConnectRateLimiter in
// conn_guard.go).
//
// # Fail strategy
//
// Unlike the 0.11 upgrade guards (fail-closed on Redis error because those are
// security gates against the J4 root cause), this story is a performance
// optimization. ResumeCache read / write errors are logged and the handler
// degrades to providers — a transient Redis outage must not block all
// `session.resume` traffic. Provider errors *do* fail-closed (surfaced as
// ErrInternalError) because handing a client a silently-missing friends list
// is data corruption from the client's perspective.
//
// This is consistent with the user-stated principle that fallbacks must not
// mask core architectural risk: here the "core risk" is Mongo / provider
// failure, which *does* surface; only the cache layer (a performance tier)
// degrades transparently.
//
// # Redis key space (D16; PRD §Redis Key Convention lines 547-554)
//
//	resume_cache:{userID}  →  Hash { user, friends, catState, skins, blindboxes, roomSnapshot }
//
// Separated from event:* / event_result:* / lock:cron:* / ratelimit:ws:* /
// blacklist:device:* / presence:* / state:* / refresh_blacklist:* per D16.
package ws

import (
	"context"
	"encoding/json"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/logx"
	"github.com/huing/cat/server/pkg/redisx"
)

// ResumeSnapshot is the payload returned by `session.resume`. Aliased from
// pkg/redisx so the ws.ResumeCache interface declared below and the
// RedisResumeCache implementation refer to the same concrete type (Go uses
// nominal typing for interface method signatures — same pattern DedupResult
// and ConnectDecision follow). See pkg/redisx/resume_cache.go for the struct
// definition and rationale for the field shapes / JSON layout.
type ResumeSnapshot = redisx.ResumeSnapshot

// ResumeCache is the consumer-side read/write interface for the 60s resume
// payload cache. Get reports found=false on cache miss (not an error) so the
// handler can fall through to providers without an error branch.
type ResumeCache interface {
	Get(ctx context.Context, userID string) (ResumeSnapshot, bool, error)
	Put(ctx context.Context, userID string, snapshot ResumeSnapshot) error
}

// ResumeCacheInvalidator is the consumer-side interface for Service-layer
// stories that perform authoritative writes (state.tick, friend.accept,
// blindbox.redeem, skin.equip, profile.update, …). Calling Invalidate after a
// successful commit is a best-effort signal: the cache self-heals within TTL
// regardless, so services typically log warn on error rather than propagating.
//
// Defined here (not in internal/service/) because Story 0.12 predates any
// service package, and keeping the interface near the Redis implementation
// matches the DedupStore / Blacklist / ConnectRateLimiter precedent. Future
// service stories import it from internal/ws; the dependency direction
// (internal/service → internal/ws → pkg/redisx) stays acyclic.
type ResumeCacheInvalidator interface {
	Invalidate(ctx context.Context, userID string) error
}

// UserProvider resolves the `user` section of the resume payload. Real
// implementation lands in Epic 1 (Story 1.1).
type UserProvider interface {
	GetUser(ctx context.Context, userID string) (json.RawMessage, error)
}

// FriendsProvider resolves the `friends` section. Real implementation lands in
// Epic 3 (Story 3.4 — friend list with block filter).
type FriendsProvider interface {
	ListFriends(ctx context.Context, userID string) (json.RawMessage, error)
}

// CatStateProvider resolves the `catState` section. Real implementation lands
// in Epic 2 (Story 2.2 — state.tick persist).
type CatStateProvider interface {
	GetCatState(ctx context.Context, userID string) (json.RawMessage, error)
}

// SkinsProvider resolves the `skins` section. Real implementation lands in
// Epic 7 (Story 7.2 — unlocked skins list).
type SkinsProvider interface {
	ListUnlocked(ctx context.Context, userID string) (json.RawMessage, error)
}

// BlindboxesProvider resolves the `blindboxes` section. Real implementation
// lands in Epic 6 (Story 6.3 — blindbox inventory).
type BlindboxesProvider interface {
	ListActive(ctx context.Context, userID string) (json.RawMessage, error)
}

// RoomSnapshotProvider resolves the `roomSnapshot` section. Real
// implementation lands in Epic 4 (Story 4.2 — room snapshot).
type RoomSnapshotProvider interface {
	GetRoomSnapshot(ctx context.Context, userID string) (json.RawMessage, error)
}

// EmptyUserProvider is the Story 0.12 placeholder. Returns JSON null.
type EmptyUserProvider struct{}

func (EmptyUserProvider) GetUser(_ context.Context, _ string) (json.RawMessage, error) {
	return json.RawMessage(`null`), nil
}

// EmptyFriendsProvider is the Story 0.12 placeholder. Returns an empty JSON
// array (not null) so clients deserializing into []Friend receive an empty
// slice rather than nil.
type EmptyFriendsProvider struct{}

func (EmptyFriendsProvider) ListFriends(_ context.Context, _ string) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}

// EmptyCatStateProvider is the Story 0.12 placeholder. Returns JSON null —
// a brand-new user has no persisted cat state until Story 2.2 / 2.3 writes one.
type EmptyCatStateProvider struct{}

func (EmptyCatStateProvider) GetCatState(_ context.Context, _ string) (json.RawMessage, error) {
	return json.RawMessage(`null`), nil
}

// EmptySkinsProvider is the Story 0.12 placeholder. Returns an empty array.
type EmptySkinsProvider struct{}

func (EmptySkinsProvider) ListUnlocked(_ context.Context, _ string) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}

// EmptyBlindboxesProvider is the Story 0.12 placeholder. Returns an empty array.
type EmptyBlindboxesProvider struct{}

func (EmptyBlindboxesProvider) ListActive(_ context.Context, _ string) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}

// EmptyRoomSnapshotProvider is the Story 0.12 placeholder. Returns JSON null.
type EmptyRoomSnapshotProvider struct{}

func (EmptyRoomSnapshotProvider) GetRoomSnapshot(_ context.Context, _ string) (json.RawMessage, error) {
	return json.RawMessage(`null`), nil
}

// ResumeProviders bundles the six domain-side suppliers the handler calls on a
// cache miss. A struct (rather than eight positional parameters) keeps
// NewSessionResumeHandler readable and means later stories that add / reshape
// providers change this type instead of every construction site.
type ResumeProviders struct {
	User         UserProvider
	Friends      FriendsProvider
	CatState     CatStateProvider
	Skins        SkinsProvider
	Blindboxes   BlindboxesProvider
	RoomSnapshot RoomSnapshotProvider
}

// SessionResumeHandler is the dispatcher-registered handler for
// `session.resume`. Constructed once in initialize.go and registered via
// Dispatcher.Register (not RegisterDedup — see Handle for rationale).
//
// The embedded singleflight.Group coalesces concurrent cache misses for the
// same userID so a Watch reconnect storm (J4) produces at most one provider
// fan-out per user, not N. Without this, two envelopes arriving ~simultaneously
// would both observe "cache miss" and race to call every provider, defeating
// the purpose of the 60s cache window precisely when upstream pool pressure
// matters most.
type SessionResumeHandler struct {
	cache     ResumeCache
	clock     clockx.Clock
	providers ResumeProviders
	group     singleflight.Group
}

// NewSessionResumeHandler constructs a SessionResumeHandler. All dependencies
// are required — nil cache / clock / provider is a misconfiguration and
// panicking at startup is preferable to emitting malformed JSON at request
// time. (Contrast with Story 0.11's NewUpgradeHandler, which accepts nil
// blacklist / limiter as a "guard off" semantic; here the Empty*Provider
// singletons express "nothing to return yet" without leaving handler fields
// unset.)
func NewSessionResumeHandler(cache ResumeCache, clock clockx.Clock, providers ResumeProviders) *SessionResumeHandler {
	if cache == nil {
		panic("ws.NewSessionResumeHandler: cache must not be nil")
	}
	if clock == nil {
		panic("ws.NewSessionResumeHandler: clock must not be nil")
	}
	if providers.User == nil {
		panic("ws.NewSessionResumeHandler: providers.User must not be nil")
	}
	if providers.Friends == nil {
		panic("ws.NewSessionResumeHandler: providers.Friends must not be nil")
	}
	if providers.CatState == nil {
		panic("ws.NewSessionResumeHandler: providers.CatState must not be nil")
	}
	if providers.Skins == nil {
		panic("ws.NewSessionResumeHandler: providers.Skins must not be nil")
	}
	if providers.Blindboxes == nil {
		panic("ws.NewSessionResumeHandler: providers.Blindboxes must not be nil")
	}
	if providers.RoomSnapshot == nil {
		panic("ws.NewSessionResumeHandler: providers.RoomSnapshot must not be nil")
	}
	return &SessionResumeHandler{cache: cache, clock: clock, providers: providers}
}

// Handle satisfies ws.HandlerFunc and is registered on the Dispatcher via
// Register (NOT RegisterDedup): session.resume is a read-only operation, it is
// idempotent by design, and running it through the dedup middleware would
// force clients to always provide a non-empty envelope.id and would trap the
// intended 60s cache hit as a spurious EVENT_PROCESSING response (see epics
// line 679 and AC7 / AC12.c in the story file).
//
// Flow:
//  1. Read cache. On error: log Warn and treat as miss (fail-open).
//  2. On miss: invoke each Provider in sequence. Any Provider error is
//     surfaced as ErrInternalError — a partial payload would mislead the
//     client.
//  3. Write cache. On error: log Warn and still respond (fail-open).
//  4. Stamp ServerTime with Clock.Now() — always fresh, never cached.
//  5. Marshal and return.
func (h *SessionResumeHandler) Handle(ctx context.Context, client *Client, _ Envelope) (json.RawMessage, error) {
	userID := client.UserID()
	start := h.clock.Now()

	snapshot, found, err := h.cache.Get(ctx, userID)
	if err != nil {
		logx.Ctx(ctx).Warn().Err(err).
			Str("action", "resume_cache_get_error").
			Str("userId", userID).
			Str("connId", client.ConnID()).
			Msg("resume cache get failed, falling through to providers")
		found = false
	}
	cacheHit := found

	if !found {
		// Coalesce concurrent misses for the same userID. Without this, a
		// burst of reconnects (J4 scenario) lets every in-flight request
		// observe "miss" before the first Put lands and independently hit
		// every provider — turning one cold build into N fan-outs. The
		// singleflight key is the userID because that's also the cache key
		// and the only dimension that distinguishes a repeatable build from
		// one that needs independent work.
		result, groupErr, _ := h.group.Do(userID, func() (any, error) {
			// Re-read the cache inside the critical section — an earlier
			// singleflight winner may have already populated it before we
			// grabbed the slot (common when the wave arrives right as the
			// previous build finishes).
			if snap, hit, getErr := h.cache.Get(ctx, userID); getErr == nil && hit {
				return snap, nil
			}
			built, buildErr := h.buildSnapshot(ctx, userID)
			if buildErr != nil {
				return ResumeSnapshot{}, buildErr
			}
			if putErr := h.cache.Put(ctx, userID, built); putErr != nil {
				logx.Ctx(ctx).Warn().Err(putErr).
					Str("action", "resume_cache_put_error").
					Str("userId", userID).
					Str("connId", client.ConnID()).
					Msg("resume cache put failed, responding anyway")
			}
			return built, nil
		})
		if groupErr != nil {
			return nil, groupErr
		}
		snapshot = result.(ResumeSnapshot)
	}

	snapshot.ServerTime = h.clock.Now().UTC().Format(time.RFC3339Nano)

	durationMs := h.clock.Now().Sub(start).Milliseconds()
	logx.Ctx(ctx).Info().
		Str("action", "session_resume").
		Str("connId", client.ConnID()).
		Str("userId", userID).
		Bool("cacheHit", cacheHit).
		Int64("durationMs", durationMs).
		Msg("session_resume")

	return json.Marshal(snapshot)
}

// buildSnapshot runs all six providers sequentially. Sequential (not parallel)
// is adequate for Story 0.12 — every provider is the zero-cost Empty*Provider
// until later stories wire in real Mongo repos. Story 4.5 may opt into an
// errgroup if p95 shows pressure; that decision belongs in that story, not
// here.
func (h *SessionResumeHandler) buildSnapshot(ctx context.Context, userID string) (ResumeSnapshot, error) {
	user, err := h.providers.User.GetUser(ctx, userID)
	if err != nil {
		return ResumeSnapshot{}, dto.ErrInternalError.WithCause(err)
	}
	friends, err := h.providers.Friends.ListFriends(ctx, userID)
	if err != nil {
		return ResumeSnapshot{}, dto.ErrInternalError.WithCause(err)
	}
	catState, err := h.providers.CatState.GetCatState(ctx, userID)
	if err != nil {
		return ResumeSnapshot{}, dto.ErrInternalError.WithCause(err)
	}
	skins, err := h.providers.Skins.ListUnlocked(ctx, userID)
	if err != nil {
		return ResumeSnapshot{}, dto.ErrInternalError.WithCause(err)
	}
	blindboxes, err := h.providers.Blindboxes.ListActive(ctx, userID)
	if err != nil {
		return ResumeSnapshot{}, dto.ErrInternalError.WithCause(err)
	}
	roomSnapshot, err := h.providers.RoomSnapshot.GetRoomSnapshot(ctx, userID)
	if err != nil {
		return ResumeSnapshot{}, dto.ErrInternalError.WithCause(err)
	}
	return ResumeSnapshot{
		User:         user,
		Friends:      friends,
		CatState:     catState,
		Skins:        skins,
		Blindboxes:   blindboxes,
		RoomSnapshot: roomSnapshot,
	}, nil
}
