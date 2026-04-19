package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/internal/ws"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/ids"
	"github.com/huing/cat/server/pkg/logx"
)

// ProfileUpdate is the service-layer partial-update shape for Story 1.5.
// Each field is pointer-typed so non-nil means "asked to change this".
// Validation is the caller's responsibility (handler runs
// dto.ValidateProfileUpdateRequest before invoking the service); the
// service forwards the partial to the repo untouched except for string
// trimming that is semantically always safe to apply (displayName).
type ProfileUpdate struct {
	DisplayName *string
	Timezone    *string
	QuietHours  *domain.QuietHours
}

// profileUpdater is the repo surface ProfileService consumes. Declared
// here (consumer-side P2 §6.2) so service tests can substitute a fake
// without dragging in mongo-driver. FindByID is used by the preflight
// that rejects quietHours-only updates when the user still has no
// persisted timezone — see Update docstring for rationale.
type profileUpdater interface {
	UpdateProfile(ctx context.Context, userID ids.UserID, p repository.ProfileUpdate) (*domain.User, error)
	FindByID(ctx context.Context, id ids.UserID) (*domain.User, error)
}

// ProfileHandlerService is the contract the WS handler consumes.
// Declared in the service package so the handler does not import the
// concrete *ProfileService; tests substitute a fake via the same
// interface.
type ProfileHandlerService interface {
	Update(ctx context.Context, userID ids.UserID, p ProfileUpdate) (*domain.User, error)
}

// ProfileService orchestrates a profile partial update:
//
//  1. Persist the change via the repo (fail-closed on error).
//  2. Invalidate the 60s session.resume cache so the authenticated
//     user — and any session.resume reader observing their public
//     projection — sees the fresh values immediately. Cache
//     invalidation is fail-open: a Redis failure does not roll back
//     the Mongo write (TTL 60s self-heals).
//  3. Audit-log the change, emitting only the *field enum* for
//     displayName (PII §M13 — never log the value).
type ProfileService struct {
	repo        profileUpdater
	invalidator ws.ResumeCacheInvalidator
	clock       clockx.Clock
}

// NewProfileService fail-fast validates its collaborators so a
// mis-wired DI graph crashes at startup rather than on first request
// (§P3 startup fail-fast).
func NewProfileService(repo profileUpdater, invalidator ws.ResumeCacheInvalidator, clk clockx.Clock) *ProfileService {
	if repo == nil {
		panic("service.NewProfileService: repo must not be nil")
	}
	if invalidator == nil {
		panic("service.NewProfileService: invalidator must not be nil")
	}
	if clk == nil {
		panic("service.NewProfileService: clock must not be nil")
	}
	return &ProfileService{repo: repo, invalidator: invalidator, clock: clk}
}

// Update applies the partial update and returns the authoritative
// post-update User. Error semantics:
//
//   - Repo returns ErrUserNotFound → propagated unchanged so the
//     handler can decide how to surface it (the handler maps it to
//     INTERNAL_ERROR, NOT a NOT_FOUND, so a caller cannot probe for
//     user existence).
//   - Repo returns any other error → wrapped with context.
//   - Invalidator error → best-effort; the main response still
//     succeeds. Observable via warn log.
//
// # Preflight: quietHours requires a timezone
//
// Before writing, if the request sets quietHours but not timezone,
// the service reads the persisted user and rejects with
// VALIDATION_ERROR when the stored timezone is still nil/empty.
// Without this check, RealQuietHoursResolver would silently short-
// circuit to "not quiet" at resolve time (the user sets quiet hours,
// the write succeeds, but the window never takes effect) — a P2
// review finding on the initial Story 1.5 landing.
//
// The extra round-trip only fires on the quietHours-only code path;
// combined updates that also set timezone skip the preflight because
// the new tz becomes authoritative inside the same UpdateOne.
func (s *ProfileService) Update(ctx context.Context, userID ids.UserID, p ProfileUpdate) (*domain.User, error) {
	if p.QuietHours != nil && p.Timezone == nil {
		existing, err := s.repo.FindByID(ctx, userID)
		if err != nil {
			if errors.Is(err, repository.ErrUserNotFound) {
				return nil, err
			}
			return nil, fmt.Errorf("profile service: preflight find: %w", err)
		}
		if existing.Timezone == nil || *existing.Timezone == "" {
			e := *dto.ErrValidationError
			e.Message = "quietHours requires timezone to be set; include 'timezone' in this request or set it before updating quietHours"
			return nil, &e
		}
	}

	updated, err := s.repo.UpdateProfile(ctx, userID, repository.ProfileUpdate{
		DisplayName: p.DisplayName,
		Timezone:    p.Timezone,
		QuietHours:  p.QuietHours,
	})
	if err != nil {
		if errors.Is(err, repository.ErrUserNotFound) {
			return nil, err
		}
		return nil, fmt.Errorf("profile service: update: %w", err)
	}

	// Fail-open invalidate — the cache is a pure performance tier with
	// a 60s TTL self-heal. Rolling back the Mongo write on a Redis
	// hiccup would be worse UX (client thinks the update failed,
	// retries → duplicate writes → log noise) than a stale read for
	// at most 60 seconds. Observable via warn log.
	if invErr := s.invalidator.Invalidate(ctx, string(userID)); invErr != nil {
		logx.Ctx(ctx).Warn().Err(invErr).
			Str("action", "resume_cache_invalidate_error").
			Str("userId", string(userID)).
			Msg("resume cache invalidate failed; update persisted, cache will self-heal in 60s")
	}

	// Audit log — field *enum*, never the value. displayName is PII
	// per §M13; timezone / quietHours are not PII but we keep the
	// shape uniform for readability (enum-only).
	fields := changedFields(p)
	logx.Ctx(ctx).Info().
		Str("action", "profile_update").
		Str("userId", string(userID)).
		Strs("fields", fields).
		Msg("profile_update")

	return updated, nil
}

// changedFields returns the enum tokens for the non-nil fields in p.
// Order is stable so log assertions can compare slices directly. The
// caller guarantees at least one field is non-nil (handler ran
// ValidateProfileUpdateRequest upstream).
func changedFields(p ProfileUpdate) []string {
	out := make([]string, 0, 3)
	if p.DisplayName != nil {
		out = append(out, "displayName")
	}
	if p.Timezone != nil {
		out = append(out, "timezone")
	}
	if p.QuietHours != nil {
		out = append(out, "quietHours")
	}
	return out
}

// Compile-time check: a concrete *ProfileService must satisfy the
// consumer-side handler interface.
var _ ProfileHandlerService = (*ProfileService)(nil)
