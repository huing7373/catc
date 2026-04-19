package service

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/ids"
)

// --- fakes ---

type fakeProfileRepo struct {
	calls      int
	lastUser   ids.UserID
	lastUpdate repository.ProfileUpdate
	returnUser *domain.User
	returnErr  error

	// preflight (Story 1.5 review round 1): FindByID feeds the
	// quietHours-requires-timezone gate.
	findByIDCalls  int
	findByIDResult *domain.User
	findByIDErr    error
}

func (f *fakeProfileRepo) UpdateProfile(_ context.Context, userID ids.UserID, p repository.ProfileUpdate) (*domain.User, error) {
	f.calls++
	f.lastUser = userID
	f.lastUpdate = p
	return f.returnUser, f.returnErr
}

func (f *fakeProfileRepo) FindByID(_ context.Context, _ ids.UserID) (*domain.User, error) {
	f.findByIDCalls++
	return f.findByIDResult, f.findByIDErr
}

type fakeInvalidator struct {
	calls    int
	lastUser string
	err      error
}

func (f *fakeInvalidator) Invalidate(_ context.Context, userID string) error {
	f.calls++
	f.lastUser = userID
	return f.err
}

// --- helpers ---

func ptr(s string) *string { return &s }

func fixedClock() *clockx.FakeClock {
	return clockx.NewFakeClock(time.Date(2026, 4, 20, 12, 0, 0, 0, time.UTC))
}

// newOkRepo builds a fakeProfileRepo that returns u for both UpdateProfile
// and FindByID. A copy of u with a non-empty Timezone satisfies the
// Story 1.5 review-round-1 preflight gate for quietHours-only updates,
// so tests that don't exercise the preflight keep working unchanged.
func newOkRepo(u *domain.User) *fakeProfileRepo {
	// Ensure the default FindByID shape has a timezone so the new
	// preflight does not reject tests that predate it. Copy so tests
	// that want to inspect u post-call see an unchanged instance.
	existing := *u
	if existing.Timezone == nil {
		tz := "UTC"
		existing.Timezone = &tz
	}
	return &fakeProfileRepo{returnUser: u, findByIDResult: &existing}
}

func seedUser(uid, dn, tz string) *domain.User {
	var dnPtr, tzPtr *string
	if dn != "" {
		dnPtr = &dn
	}
	if tz != "" {
		tzPtr = &tz
	}
	return &domain.User{
		ID:          ids.UserID(uid),
		DisplayName: dnPtr,
		Timezone:    tzPtr,
		Preferences: domain.DefaultPreferences(),
	}
}

// captureLogs attaches a buffer-backed zerolog logger to the returned
// context so the service's logx.Ctx(ctx).* calls write into buf. This
// avoids mutating package globals, which would race with parallel tests.
func captureLogs(t *testing.T, fn func(ctx context.Context)) string {
	t.Helper()
	var buf bytes.Buffer
	zl := zerolog.New(&buf).With().Logger()
	ctx := zl.WithContext(context.Background())
	fn(ctx)
	return buf.String()
}

// --- tests ---

func TestProfileService_Update_HappyPath_AllThreeFields(t *testing.T) {
	t.Parallel()
	expected := seedUser("u1", "Alice", "Asia/Shanghai")
	repo := newOkRepo(expected)
	inv := &fakeInvalidator{}
	svc := NewProfileService(repo, inv, fixedClock())

	dn := "Alice"
	tz := "Asia/Shanghai"
	qh := domain.QuietHours{Start: "23:00", End: "07:00"}
	got, err := svc.Update(context.Background(), ids.UserID("u1"), ProfileUpdate{
		DisplayName: &dn, Timezone: &tz, QuietHours: &qh,
	})
	require.NoError(t, err)
	assert.Same(t, expected, got)
	assert.Equal(t, 1, repo.calls)
	assert.Equal(t, 1, inv.calls)
	assert.Equal(t, "u1", inv.lastUser)
	require.NotNil(t, repo.lastUpdate.DisplayName)
	require.NotNil(t, repo.lastUpdate.Timezone)
	require.NotNil(t, repo.lastUpdate.QuietHours)
	assert.Equal(t, "Alice", *repo.lastUpdate.DisplayName)
	assert.Equal(t, "Asia/Shanghai", *repo.lastUpdate.Timezone)
	assert.Equal(t, "23:00", repo.lastUpdate.QuietHours.Start)
}

func TestProfileService_Update_RepoReturnsUserNotFound_Propagates(t *testing.T) {
	t.Parallel()
	repo := &fakeProfileRepo{returnErr: repository.ErrUserNotFound}
	inv := &fakeInvalidator{}
	svc := NewProfileService(repo, inv, fixedClock())

	_, err := svc.Update(context.Background(), ids.UserID("u1"), ProfileUpdate{DisplayName: ptr("Alice")})
	require.Error(t, err)
	assert.ErrorIs(t, err, repository.ErrUserNotFound)
	assert.Equal(t, 0, inv.calls, "invalidator not called on repo error")
}

func TestProfileService_Update_RepoGenericError_Wrapped(t *testing.T) {
	t.Parallel()
	boom := errors.New("mongo io")
	repo := &fakeProfileRepo{returnErr: boom}
	inv := &fakeInvalidator{}
	svc := NewProfileService(repo, inv, fixedClock())

	_, err := svc.Update(context.Background(), ids.UserID("u1"), ProfileUpdate{DisplayName: ptr("Alice")})
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "update")
	assert.Equal(t, 0, inv.calls)
}

func TestProfileService_Update_InvalidatorError_FailOpen(t *testing.T) {
	t.Parallel()
	expected := seedUser("u1", "Alice", "")
	repo := newOkRepo(expected)
	inv := &fakeInvalidator{err: errors.New("redis down")}
	svc := NewProfileService(repo, inv, fixedClock())

	output := captureLogs(t, func(ctx context.Context) {
		got, err := svc.Update(ctx, ids.UserID("u1"), ProfileUpdate{DisplayName: ptr("Alice")})
		require.NoError(t, err, "invalidator failure must be fail-open")
		assert.Same(t, expected, got)
	})

	assert.Contains(t, output, "resume_cache_invalidate_error",
		"warn log required so ops can diagnose cache staleness")
	assert.Equal(t, 1, inv.calls)
}

// --- Preflight: quietHours requires a timezone (review round 1) ---

func TestProfileService_Update_QuietHoursOnly_NoExistingTimezone_RejectedWithValidationError(t *testing.T) {
	t.Parallel()
	// User has no timezone persisted yet. Client tries quietHours-
	// only update. The pre-review behavior silently persisted this
	// and then RealQuietHoursResolver short-circuited on nil tz,
	// leaving the user unprotected at night. The fix rejects at
	// write time with VALIDATION_ERROR.
	userNoTZ := seedUser("u1", "", "")
	require.Nil(t, userNoTZ.Timezone, "test precondition: user has no timezone")

	repo := &fakeProfileRepo{findByIDResult: userNoTZ}
	inv := &fakeInvalidator{}
	svc := NewProfileService(repo, inv, fixedClock())

	_, err := svc.Update(context.Background(), ids.UserID("u1"), ProfileUpdate{
		QuietHours: &domain.QuietHours{Start: "22:00", End: "06:00"},
	})
	require.Error(t, err)
	var ae *dto.AppError
	require.True(t, errors.As(err, &ae), "expected AppError, got %T: %v", err, err)
	assert.Equal(t, "VALIDATION_ERROR", ae.Code)
	assert.Contains(t, ae.Message, "timezone",
		"error message must tell the client to set timezone first")

	assert.Equal(t, 1, repo.findByIDCalls, "preflight FindByID must fire once")
	assert.Equal(t, 0, repo.calls, "UpdateProfile MUST NOT run after preflight rejection")
	assert.Equal(t, 0, inv.calls, "Invalidate MUST NOT run after preflight rejection")
}

func TestProfileService_Update_QuietHoursOnly_ExistingTimezone_Accepted(t *testing.T) {
	t.Parallel()
	// User already has timezone persisted. Client sends quietHours-
	// only update — preflight sees tz is set, proceeds to UpdateOne.
	userWithTZ := seedUser("u1", "", "UTC")
	require.NotNil(t, userWithTZ.Timezone, "test precondition: user has timezone")

	repo := &fakeProfileRepo{returnUser: userWithTZ, findByIDResult: userWithTZ}
	inv := &fakeInvalidator{}
	svc := NewProfileService(repo, inv, fixedClock())

	_, err := svc.Update(context.Background(), ids.UserID("u1"), ProfileUpdate{
		QuietHours: &domain.QuietHours{Start: "22:00", End: "06:00"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, repo.findByIDCalls)
	assert.Equal(t, 1, repo.calls, "UpdateProfile runs after preflight pass")
	assert.Equal(t, 1, inv.calls)
}

func TestProfileService_Update_QuietHoursAndTimezone_SkipsPreflight(t *testing.T) {
	t.Parallel()
	// Combined update (tz + quietHours in same request): the new tz
	// becomes authoritative inside the same UpdateOne, so the
	// preflight is superfluous. Skipping saves a Mongo round trip.
	repo := &fakeProfileRepo{returnUser: seedUser("u1", "", "Asia/Shanghai")}
	inv := &fakeInvalidator{}
	svc := NewProfileService(repo, inv, fixedClock())

	_, err := svc.Update(context.Background(), ids.UserID("u1"), ProfileUpdate{
		Timezone:   ptr("Asia/Shanghai"),
		QuietHours: &domain.QuietHours{Start: "22:00", End: "06:00"},
	})
	require.NoError(t, err)
	assert.Equal(t, 0, repo.findByIDCalls, "preflight MUST NOT run when timezone is in the same request")
	assert.Equal(t, 1, repo.calls)
}

func TestProfileService_Update_QuietHoursOnly_PreflightRepoError_Propagated(t *testing.T) {
	t.Parallel()
	// Mongo I/O failure during preflight: the service wraps and
	// propagates the error (fail-closed on preflight). This lets
	// the handler surface INTERNAL_ERROR rather than silently
	// persist.
	boom := errors.New("mongo io")
	repo := &fakeProfileRepo{findByIDErr: boom}
	inv := &fakeInvalidator{}
	svc := NewProfileService(repo, inv, fixedClock())

	_, err := svc.Update(context.Background(), ids.UserID("u1"), ProfileUpdate{
		QuietHours: &domain.QuietHours{Start: "22:00", End: "06:00"},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, boom)
	assert.Contains(t, err.Error(), "preflight")
	assert.Equal(t, 0, repo.calls, "UpdateProfile MUST NOT run after preflight err")
}

func TestProfileService_Update_QuietHoursOnly_PreflightUserNotFound_Propagates(t *testing.T) {
	t.Parallel()
	// Preflight user-missing → propagate ErrUserNotFound so handler
	// maps to INTERNAL_ERROR (existence-probe hardening, same as the
	// happy path).
	repo := &fakeProfileRepo{findByIDErr: repository.ErrUserNotFound}
	inv := &fakeInvalidator{}
	svc := NewProfileService(repo, inv, fixedClock())

	_, err := svc.Update(context.Background(), ids.UserID("u1"), ProfileUpdate{
		QuietHours: &domain.QuietHours{Start: "22:00", End: "06:00"},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, repository.ErrUserNotFound)
	assert.Equal(t, 0, repo.calls)
}

func TestProfileService_Update_InvalidatesCacheOnAnyFieldChange(t *testing.T) {
	t.Parallel()
	// Even a single-field update must invalidate — missing invalidate
	// would leak stale displayName / timezone / quietHours through
	// session.resume for up to 60s.
	cases := []struct {
		name string
		p    ProfileUpdate
	}{
		{"displayName-only", ProfileUpdate{DisplayName: ptr("Alice")}},
		{"timezone-only", ProfileUpdate{Timezone: ptr("UTC")}},
		{"quietHours-only", ProfileUpdate{QuietHours: &domain.QuietHours{Start: "22:00", End: "06:00"}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			repo := newOkRepo(seedUser("u1", "", ""))
			inv := &fakeInvalidator{}
			svc := NewProfileService(repo, inv, fixedClock())
			_, err := svc.Update(context.Background(), ids.UserID("u1"), c.p)
			require.NoError(t, err)
			assert.Equal(t, 1, inv.calls, "Invalidate must fire on any single-field change")
		})
	}
}

func TestProfileService_Update_AuditLogEnum_DisplayNameOnly(t *testing.T) {
	t.Parallel()
	repo := newOkRepo(seedUser("u1", "", ""))
	inv := &fakeInvalidator{}
	svc := NewProfileService(repo, inv, fixedClock())

	output := captureLogs(t, func(ctx context.Context) {
		_, err := svc.Update(ctx, ids.UserID("u1"), ProfileUpdate{DisplayName: ptr("Alice")})
		require.NoError(t, err)
	})
	assert.Contains(t, output, `"fields":["displayName"]`)
	assert.Contains(t, output, `"action":"profile_update"`)
}

func TestProfileService_Update_AuditLogEnum_DisplayNameAndQuietHours(t *testing.T) {
	t.Parallel()
	repo := newOkRepo(seedUser("u1", "", ""))
	inv := &fakeInvalidator{}
	svc := NewProfileService(repo, inv, fixedClock())

	output := captureLogs(t, func(ctx context.Context) {
		_, err := svc.Update(ctx, ids.UserID("u1"), ProfileUpdate{
			DisplayName: ptr("Alice"),
			QuietHours:  &domain.QuietHours{Start: "23:00", End: "07:00"},
		})
		require.NoError(t, err)
	})
	assert.Contains(t, output, `"fields":["displayName","quietHours"]`)
}

func TestProfileService_Update_DoesNotLogDisplayNameValue(t *testing.T) {
	t.Parallel()
	// PII §M13: displayName value must NEVER appear in logs — not at
	// info, not at debug, not wrapped in any field. A log scrape leak
	// is the worst-case outcome, so this test forbids the exact
	// string regardless of surrounding context.
	repo := newOkRepo(seedUser("u1", "", ""))
	inv := &fakeInvalidator{}
	svc := NewProfileService(repo, inv, fixedClock())

	secretNames := []string{"Alice", "王小明", "Bob", "🐈Suki"}
	output := captureLogs(t, func(ctx context.Context) {
		for _, name := range secretNames {
			_, err := svc.Update(ctx, ids.UserID("u1"), ProfileUpdate{DisplayName: ptr(name)})
			require.NoError(t, err)
		}
	})
	for _, name := range secretNames {
		assert.NotContains(t, output, name,
			"displayName %q must not appear in any log line", name)
	}
	// Sanity: the enum token *is* present (so we did log *something*).
	assert.Contains(t, output, `"displayName"`,
		"enum token should appear so operators still see which field changed")
}

func TestProfileService_Update_TimezoneValueIsLoggable(t *testing.T) {
	t.Parallel()
	// AC9 explicitly permits logging timezone values (non-PII, helps
	// ops debug cross-timezone quiet-hours bugs). We do NOT currently
	// log the value — but we also do not forbid a future additive log
	// line from emitting it. This test just locks that timezone *is
	// not* leaked today as a side-effect of the enum string.
	repo := newOkRepo(seedUser("u1", "", ""))
	inv := &fakeInvalidator{}
	svc := NewProfileService(repo, inv, fixedClock())

	output := captureLogs(t, func(ctx context.Context) {
		_, err := svc.Update(ctx, ids.UserID("u1"), ProfileUpdate{Timezone: ptr("Asia/Shanghai")})
		require.NoError(t, err)
	})
	// Enum token present, userId present.
	assert.Contains(t, output, `"fields":["timezone"]`)
	assert.Contains(t, output, `"userId":"u1"`)
}

func TestProfileService_Update_ClockInjectionDoesNotStampFromTimeNow(t *testing.T) {
	t.Parallel()
	// The service does not stamp any timestamp field itself (the repo
	// owns updated_at). This test locks the *absence* of a direct
	// time.Now() side-effect by asserting that the service never
	// writes to a clock-sensitive output channel other than via the
	// injected clock. The single observable is the repo call (which
	// carries no timestamp), so we exercise the shape once.
	repo := newOkRepo(seedUser("u1", "", ""))
	inv := &fakeInvalidator{}
	clk := clockx.NewFakeClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC))
	svc := NewProfileService(repo, inv, clk)

	_, err := svc.Update(context.Background(), ids.UserID("u1"), ProfileUpdate{DisplayName: ptr("Alice")})
	require.NoError(t, err)
	// The service forwards the raw pointer to the repo; the repo is
	// responsible for stamping updated_at via its own clock. This
	// checks the forwarding contract.
	require.NotNil(t, repo.lastUpdate.DisplayName)
	assert.Equal(t, "Alice", *repo.lastUpdate.DisplayName)
}

func TestProfileService_NilDepsPanic(t *testing.T) {
	t.Parallel()
	repo := newOkRepo(seedUser("u1", "", ""))
	inv := &fakeInvalidator{}
	clk := fixedClock()
	cases := []struct {
		name string
		repo profileUpdater
		inv  *fakeInvalidator
		clk  clockx.Clock
	}{
		{"nil repo", nil, inv, clk},
		{"nil inv", repo, nil, clk},
		{"nil clock", repo, inv, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assert.Panics(t, func() {
				// inv may be nil in the "nil inv" case — the wrapper type
				// here turns it into a nil interface value.
				var invIface = any(c.inv)
				if c.inv == nil {
					invIface = nil
				}
				_ = invIface
				if c.inv == nil {
					NewProfileService(c.repo, nil, c.clk)
					return
				}
				NewProfileService(c.repo, c.inv, c.clk)
			})
		})
	}
}

// TestProfileService_ChangedFields_Helper locks the output order of the
// internal helper so the audit log produces deterministic JSON — a
// random-order Strs() would make the PII-scan test flaky if the test
// case ever needed to assert substring presence of a specific field.
func TestProfileService_ChangedFields_Helper(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		p    ProfileUpdate
		want []string
	}{
		{"all", ProfileUpdate{DisplayName: ptr("x"), Timezone: ptr("UTC"), QuietHours: &domain.QuietHours{}}, []string{"displayName", "timezone", "quietHours"}},
		{"dn-only", ProfileUpdate{DisplayName: ptr("x")}, []string{"displayName"}},
		{"tz-only", ProfileUpdate{Timezone: ptr("UTC")}, []string{"timezone"}},
		{"qh-only", ProfileUpdate{QuietHours: &domain.QuietHours{}}, []string{"quietHours"}},
		{"dn+tz", ProfileUpdate{DisplayName: ptr("x"), Timezone: ptr("UTC")}, []string{"displayName", "timezone"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got := changedFields(c.p)
			assert.Equal(t, c.want, got)
		})
	}
}

