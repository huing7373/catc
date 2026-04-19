package push

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/ids"
	"github.com/huing/cat/server/pkg/logx"
)

// quietHoursUserLookup is the minimal user-surface
// RealQuietHoursResolver needs. The (user, found, err) shape lets the
// resolver distinguish "no such user, treat as not-quiet" (found=false,
// err=nil) from "mongo io error" (err!=nil) WITHOUT this package
// importing internal/repository — which would form a cycle because
// internal/repository already imports internal/push (for push.TokenInfo
// in apns_token_repo.go). The cmd/cat/initialize.go wiring supplies a
// thin adapter that maps repository.ErrUserNotFound onto found=false.
type quietHoursUserLookup interface {
	FindByID(ctx context.Context, id ids.UserID) (user *domain.User, found bool, err error)
}

// RealQuietHoursResolver is the Story 1.5 replacement for
// EmptyQuietHoursResolver. It reads the user's timezone + preferences
// .quietHours from Mongo and decides whether the *current wall-clock
// moment in the user's local tz* falls inside the quiet window.
//
// Window semantics (left-closed, right-open):
//   - start == end  → always quiet (24h silent).
//   - start < end   → quiet when start ≤ now < end (same-day).
//   - start > end   → quiet when now ≥ start OR now < end (overnight).
//
// Fail-open contract (Story 0.13 providers.go godoc and §21.3):
//   - Missing user                  → (false, nil).
//   - user.Timezone nil / empty     → (false, nil).
//   - LoadLocation fails (dirty tz) → (false, nil), warn-logged.
//   - HH:MM parse fails             → (false, nil), warn-logged.
//
// Other Mongo errors → (false, err); the APNs worker then warn-logs and
// delivers the alert anyway (loud-but-should-be-silent beats
// silenced-but-wanted for product UX).
type RealQuietHoursResolver struct {
	repo  quietHoursUserLookup
	clock clockx.Clock
}

// NewRealQuietHoursResolver wires the resolver. Panics on nil
// dependencies per §P3 startup fail-fast.
func NewRealQuietHoursResolver(repo quietHoursUserLookup, clk clockx.Clock) *RealQuietHoursResolver {
	if repo == nil {
		panic("push.NewRealQuietHoursResolver: repo must not be nil")
	}
	if clk == nil {
		panic("push.NewRealQuietHoursResolver: clock must not be nil")
	}
	return &RealQuietHoursResolver{repo: repo, clock: clk}
}

// Resolve implements QuietHoursResolver.
func (r *RealQuietHoursResolver) Resolve(ctx context.Context, userID ids.UserID) (bool, error) {
	u, found, err := r.repo.FindByID(ctx, userID)
	if err != nil {
		return false, err
	}
	if !found {
		// Missing user → fail-open (Story 0.13 contract).
		return false, nil
	}

	if u.Timezone == nil || *u.Timezone == "" {
		return false, nil
	}
	loc, err := time.LoadLocation(*u.Timezone)
	if err != nil {
		logx.Ctx(ctx).Warn().Err(err).
			Str("action", "quiet_hours_bad_timezone").
			Str("userId", string(userID)).
			Str("timezone", *u.Timezone).
			Msg("user timezone is not a valid IANA zone; treating as not-quiet (fail-open)")
		return false, nil
	}

	startMin, startOK := parseHHMM(u.Preferences.QuietHours.Start)
	endMin, endOK := parseHHMM(u.Preferences.QuietHours.End)
	if !startOK || !endOK {
		logx.Ctx(ctx).Warn().
			Str("action", "quiet_hours_bad_quiet_hours").
			Str("userId", string(userID)).
			Str("start", u.Preferences.QuietHours.Start).
			Str("end", u.Preferences.QuietHours.End).
			Msg("user quietHours is not HH:MM; treating as not-quiet (fail-open)")
		return false, nil
	}

	nowLocal := r.clock.Now().In(loc)
	nowMin := nowLocal.Hour()*60 + nowLocal.Minute()

	return isQuiet(nowMin, startMin, endMin), nil
}

// isQuiet applies the left-closed, right-open window arithmetic. Keeps
// the branching logic isolated from I/O so each branch is unit-testable
// without a fake lookup fixture.
func isQuiet(nowMin, startMin, endMin int) bool {
	if startMin == endMin {
		// start == end ⇒ 24h silent (Story 1.5 Semantic #4: MUST NOT
		// fall through the start < end branch because that degenerate
		// case would read "nowMin >= start && nowMin < start" =
		// always false, i.e. silently "never quiet").
		return true
	}
	if startMin < endMin {
		// Same-day window: [start, end).
		return nowMin >= startMin && nowMin < endMin
	}
	// Overnight window (start > end): [start, 24:00) ∪ [00:00, end).
	// Written as OR — Story 1.5 Semantic #3 traps the AND mistake.
	return nowMin >= startMin || nowMin < endMin
}

// parseHHMM decodes a "HH:MM" string into a minute-of-day int in
// [0, 1440). Returns ok=false for any shape deviation — no panic, no
// partial parse. The DTO validator runs the canonical regex; this
// function is the runtime defense against stored values that predate
// the validator or were inserted manually during migration.
func parseHHMM(s string) (int, bool) {
	// Exact length / colon position check first — `len(s) == 5 &&
	// s[2] == ':'` rules out "23:5" / "0:00" / "023:00" before we
	// touch Atoi.
	if len(s) != 5 || s[2] != ':' {
		return 0, false
	}
	hh, err := strconv.Atoi(s[:2])
	if err != nil {
		return 0, false
	}
	mm, err := strconv.Atoi(s[3:])
	if err != nil {
		return 0, false
	}
	if hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return 0, false
	}
	// Reject sign / whitespace sneak-ins that Atoi accepts ("+1" / "-1"
	// for hh become 1 / −1; hh < 0 already rejects the negative but
	// strconv.Atoi also accepts leading "+" which is not HH:MM format).
	if strings.ContainsAny(s[:2], "+- ") || strings.ContainsAny(s[3:], "+- ") {
		return 0, false
	}
	return hh*60 + mm, true
}

// Interface assertion — fails the build if Resolve drifts from the
// QuietHoursResolver contract.
var _ QuietHoursResolver = (*RealQuietHoursResolver)(nil)
