package push

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
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/ids"
)

// --- fake lookup ---

type fakeLookup struct {
	user  *domain.User
	found bool
	err   error
	calls int
}

func (f *fakeLookup) FindByID(_ context.Context, _ ids.UserID) (*domain.User, bool, error) {
	f.calls++
	return f.user, f.found, f.err
}

// --- helpers ---

func userWith(tz, start, end string) *domain.User {
	var tzPtr *string
	if tz != "" {
		tzPtr = &tz
	}
	u := &domain.User{
		ID:       "u1",
		Timezone: tzPtr,
		Preferences: domain.UserPreferences{
			QuietHours: domain.QuietHours{Start: start, End: end},
		},
	}
	return u
}

func fixedClockAt(y, mo, d, h, min int) *clockx.FakeClock {
	return clockx.NewFakeClock(time.Date(y, time.Month(mo), d, h, min, 0, 0, time.UTC))
}

// captureLogs attaches a buffer-backed zerolog logger to the context
// so logx.Ctx(ctx).* calls land in buf without touching globals.
func captureLogs(t *testing.T, fn func(ctx context.Context)) string {
	t.Helper()
	var buf bytes.Buffer
	zl := zerolog.New(&buf).With().Logger()
	ctx := zl.WithContext(context.Background())
	fn(ctx)
	return buf.String()
}

// --- Resolve: fail-open edge cases ---

func TestRealQuietHoursResolver_UserNotFound_ReturnsFalseNil(t *testing.T) {
	t.Parallel()
	r := NewRealQuietHoursResolver(&fakeLookup{found: false}, fixedClockAt(2026, 4, 20, 0, 0))
	got, err := r.Resolve(context.Background(), ids.UserID("u1"))
	require.NoError(t, err)
	assert.False(t, got)
}

func TestRealQuietHoursResolver_TimezoneNil_ReturnsFalseNil(t *testing.T) {
	t.Parallel()
	r := NewRealQuietHoursResolver(
		&fakeLookup{user: userWith("", "23:00", "07:00"), found: true},
		fixedClockAt(2026, 4, 20, 0, 0),
	)
	got, err := r.Resolve(context.Background(), ids.UserID("u1"))
	require.NoError(t, err)
	assert.False(t, got, "nil timezone → fail-open not-quiet")
}

func TestRealQuietHoursResolver_TimezoneInvalid_ReturnsFalseNilWithWarn(t *testing.T) {
	t.Parallel()
	u := userWith("Pacific/Nope", "23:00", "07:00")
	r := NewRealQuietHoursResolver(&fakeLookup{user: u, found: true}, fixedClockAt(2026, 4, 20, 0, 0))

	output := captureLogs(t, func(ctx context.Context) {
		got, err := r.Resolve(ctx, ids.UserID("u1"))
		require.NoError(t, err)
		assert.False(t, got)
	})
	assert.Contains(t, output, "quiet_hours_bad_timezone")
}

func TestRealQuietHoursResolver_QuietHoursInvalid_ReturnsFalseNilWithWarn(t *testing.T) {
	t.Parallel()
	u := userWith("UTC", "bad", "07:00")
	r := NewRealQuietHoursResolver(&fakeLookup{user: u, found: true}, fixedClockAt(2026, 4, 20, 0, 0))

	output := captureLogs(t, func(ctx context.Context) {
		got, err := r.Resolve(ctx, ids.UserID("u1"))
		require.NoError(t, err)
		assert.False(t, got)
	})
	assert.Contains(t, output, "quiet_hours_bad_quiet_hours")
}

func TestRealQuietHoursResolver_LookupReturnsError_PropagatesFalseErr(t *testing.T) {
	t.Parallel()
	boom := errors.New("mongo io")
	r := NewRealQuietHoursResolver(&fakeLookup{err: boom}, fixedClockAt(2026, 4, 20, 0, 0))
	got, err := r.Resolve(context.Background(), ids.UserID("u1"))
	assert.False(t, got)
	assert.ErrorIs(t, err, boom, "non-notfound errors surface up")
}

// --- Same-day window ---

func TestRealQuietHoursResolver_SameDay_Inside(t *testing.T) {
	t.Parallel()
	u := userWith("UTC", "10:00", "15:00")
	r := NewRealQuietHoursResolver(&fakeLookup{user: u, found: true}, fixedClockAt(2026, 4, 20, 12, 0))
	got, err := r.Resolve(context.Background(), ids.UserID("u1"))
	require.NoError(t, err)
	assert.True(t, got)
}

func TestRealQuietHoursResolver_SameDay_Before(t *testing.T) {
	t.Parallel()
	u := userWith("UTC", "10:00", "15:00")
	r := NewRealQuietHoursResolver(&fakeLookup{user: u, found: true}, fixedClockAt(2026, 4, 20, 9, 59))
	got, err := r.Resolve(context.Background(), ids.UserID("u1"))
	require.NoError(t, err)
	assert.False(t, got)
}

func TestRealQuietHoursResolver_SameDay_StartBoundaryIncluded(t *testing.T) {
	t.Parallel()
	// Left-closed — 10:00 exactly IS quiet.
	u := userWith("UTC", "10:00", "15:00")
	r := NewRealQuietHoursResolver(&fakeLookup{user: u, found: true}, fixedClockAt(2026, 4, 20, 10, 0))
	got, err := r.Resolve(context.Background(), ids.UserID("u1"))
	require.NoError(t, err)
	assert.True(t, got, "10:00 (start) must be included (left-closed)")
}

func TestRealQuietHoursResolver_SameDay_EndBoundaryExcluded(t *testing.T) {
	t.Parallel()
	// Right-open — 15:00 exactly is NOT quiet.
	u := userWith("UTC", "10:00", "15:00")
	r := NewRealQuietHoursResolver(&fakeLookup{user: u, found: true}, fixedClockAt(2026, 4, 20, 15, 0))
	got, err := r.Resolve(context.Background(), ids.UserID("u1"))
	require.NoError(t, err)
	assert.False(t, got, "15:00 (end) must be excluded (right-open)")
}

// --- Overnight window ---

func TestRealQuietHoursResolver_Overnight_StartIncluded(t *testing.T) {
	t.Parallel()
	// UTC user, window 23:00-07:00. At UTC 23:00 we are inside.
	u := userWith("UTC", "23:00", "07:00")
	r := NewRealQuietHoursResolver(&fakeLookup{user: u, found: true}, fixedClockAt(2026, 4, 20, 23, 0))
	got, err := r.Resolve(context.Background(), ids.UserID("u1"))
	require.NoError(t, err)
	assert.True(t, got)
}

func TestRealQuietHoursResolver_Overnight_MidOfNight(t *testing.T) {
	t.Parallel()
	u := userWith("UTC", "23:00", "07:00")
	r := NewRealQuietHoursResolver(&fakeLookup{user: u, found: true}, fixedClockAt(2026, 4, 20, 3, 0))
	got, err := r.Resolve(context.Background(), ids.UserID("u1"))
	require.NoError(t, err)
	assert.True(t, got)
}

func TestRealQuietHoursResolver_Overnight_EndBoundaryExcluded(t *testing.T) {
	t.Parallel()
	u := userWith("UTC", "23:00", "07:00")
	r := NewRealQuietHoursResolver(&fakeLookup{user: u, found: true}, fixedClockAt(2026, 4, 20, 7, 0))
	got, err := r.Resolve(context.Background(), ids.UserID("u1"))
	require.NoError(t, err)
	assert.False(t, got, "07:00 (end) must be excluded in overnight window too")
}

func TestRealQuietHoursResolver_Overnight_BeforeStart(t *testing.T) {
	t.Parallel()
	u := userWith("UTC", "23:00", "07:00")
	r := NewRealQuietHoursResolver(&fakeLookup{user: u, found: true}, fixedClockAt(2026, 4, 20, 22, 59))
	got, err := r.Resolve(context.Background(), ids.UserID("u1"))
	require.NoError(t, err)
	assert.False(t, got, "22:59 is 1 minute before start in overnight window")
}

// --- start == end (24h silent) ---

func TestRealQuietHoursResolver_StartEqualsEnd_AlwaysQuiet(t *testing.T) {
	t.Parallel()
	u := userWith("UTC", "22:00", "22:00")
	for _, tt := range []struct {
		hour, min int
	}{{0, 0}, {11, 30}, {22, 0}, {23, 59}} {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			r := NewRealQuietHoursResolver(&fakeLookup{user: u, found: true}, fixedClockAt(2026, 4, 20, tt.hour, tt.min))
			got, err := r.Resolve(context.Background(), ids.UserID("u1"))
			require.NoError(t, err)
			assert.True(t, got, "start==end means permanent-silent; must be quiet at any moment")
		})
	}
}

// --- cross-timezone: same UTC moment, different local decisions ---

func TestRealQuietHoursResolver_Timezone_NewYork_vs_Shanghai(t *testing.T) {
	t.Parallel()
	// At UTC 2026-04-20 04:00:
	//   America/New_York (UTC-4 DST): local 00:00 → inside 23:00-07:00
	//   Asia/Shanghai   (UTC+8):      local 12:00 → NOT inside 23:00-07:00
	// If the resolver forgot .In(loc) and compared server UTC (04:00)
	// both users would read "quiet" (04:00 < 07:00); the assertion
	// pair below would break.
	nowAt := fixedClockAt(2026, 4, 20, 4, 0)

	ny := userWith("America/New_York", "23:00", "07:00")
	sh := userWith("Asia/Shanghai", "23:00", "07:00")

	rNY := NewRealQuietHoursResolver(&fakeLookup{user: ny, found: true}, nowAt)
	rSH := NewRealQuietHoursResolver(&fakeLookup{user: sh, found: true}, nowAt)

	quietNY, err := rNY.Resolve(context.Background(), ids.UserID("u1"))
	require.NoError(t, err)
	quietSH, err := rSH.Resolve(context.Background(), ids.UserID("u1"))
	require.NoError(t, err)

	assert.True(t, quietNY, "NY user at local midnight: inside overnight window")
	assert.False(t, quietSH, "Shanghai user at local noon: NOT inside overnight window")
}

// --- parseHHMM internal helper ---

func TestParseHHMM(t *testing.T) {
	t.Parallel()
	ok := map[string]int{
		"00:00": 0,
		"00:59": 59,
		"01:00": 60,
		"23:59": 23*60 + 59,
		"10:30": 10*60 + 30,
	}
	for s, want := range ok {
		t.Run("ok/"+s, func(t *testing.T) {
			t.Parallel()
			got, okFlag := parseHHMM(s)
			require.True(t, okFlag, "%q should parse", s)
			assert.Equal(t, want, got)
		})
	}
	bad := []string{"24:00", "25:90", "23:5", "0:00", "ab:cd", "", "123:45", "23:60", "+1:00", "23: 0"}
	for _, s := range bad {
		t.Run("bad/"+s, func(t *testing.T) {
			t.Parallel()
			_, okFlag := parseHHMM(s)
			assert.False(t, okFlag, "%q should be rejected", s)
		})
	}
}

// --- nil dep panics ---

func TestRealQuietHoursResolver_NilDepsPanic(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() { NewRealQuietHoursResolver(nil, fixedClockAt(2026, 4, 20, 0, 0)) })
	assert.Panics(t, func() { NewRealQuietHoursResolver(&fakeLookup{}, nil) })
}

// --- isQuiet internal helper (extra coverage) ---

func TestIsQuiet_EquivalenceTable(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		now, start, end int
		want  bool
	}{
		{"same-day-inside", 600, 480, 720, true},   // 10:00 start, 12:00 now, 12:00 end→ wait 720==720
		{"same-day-before", 120, 600, 900, false},  // 02:00 before 10:00
		{"same-day-after", 1000, 600, 900, false},  // 16:40 after 15:00
		{"overnight-late-night", 180, 1380, 420, true}, // 03:00 within 23:00-07:00
		{"overnight-just-after-end", 421, 1380, 420, false}, // 07:01
		{"equal-always-quiet", 0, 600, 600, true},
		{"equal-at-midnight", 1439, 0, 0, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.want, isQuiet(c.now, c.start, c.end))
		})
	}
}
