package dto

import (
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/huing/cat/server/internal/domain"
)

// hhmmRegex locks HH:MM to a 24h calendar time: hours 00-23, minutes
// 00-59. A looser pattern (e.g. \d{1,2}:\d{1,2}) would let 25:90 through
// validation, and the downstream QuietHoursResolver would then compute
// nonsense boundary math — see Story 1.5 Semantic-correctness §2.
var hhmmRegex = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// displayNameMinRunes / displayNameMaxRunes bound the trimmed UTF-8
// rune count for displayName. A 32-rune cap keeps the friend-list UI
// readable on Apple Watch without truncation; 1 rune minimum rejects
// all-whitespace submissions after trim.
const (
	displayNameMinRunes = 1
	displayNameMaxRunes = 32
)

// QuietHoursDTO is the wire shape for preferences.quietHours.
// Dual JSON/BSON tags let this type travel on both HTTP/WS requests
// and straight into repo-layer filters without hand copy (P2 DTO double
// tag convention).
type QuietHoursDTO struct {
	Start string `json:"start" bson:"start"`
	End   string `json:"end"   bson:"end"`
}

// ProfileUpdateRequest is the wire format for the `profile.update` WS
// RPC. All three fields are *optional* — callers send only the fields
// they want to change (Story 1.5 FR48/49/50). Payload is rejected if
// every field is nil.
//
// Validator runs in handler code (see ValidateProfileUpdateRequest)
// rather than via go-playground/validator binding because the semantic
// checks (IANA tz, HH:MM range, display-name trim rules) are easier
// and safer to express imperatively than as struct tags.
type ProfileUpdateRequest struct {
	DisplayName *string        `json:"displayName,omitempty"`
	Timezone    *string        `json:"timezone,omitempty"`
	QuietHours  *QuietHoursDTO `json:"quietHours,omitempty"`
}

// UserPublicProfile extends UserPublic with the preferences.quietHours
// block so clients get the authoritative snapshot after a profile
// write without a follow-up session.resume. Reuses UserPublic fields
// verbatim to keep the SignInWithApple / session.resume contract
// forward-compatible.
type UserPublicProfile struct {
	UserPublic
	Preferences UserPublicPreferences `json:"preferences"`
}

// UserPublicPreferences is the public projection of domain.UserPreferences.
// Currently only QuietHours; additional preference blocks (Epic 5+ touch
// mute, Epic 7 skin slots) will extend this type.
type UserPublicPreferences struct {
	QuietHours QuietHoursDTO `json:"quietHours"`
}

// ProfileUpdateResponse is the success body for `profile.update`.
// Returns the authoritative post-write User so the client can replace
// its local cache without relying on in-band inferred state.
type ProfileUpdateResponse struct {
	User UserPublicProfile `json:"user"`
}

// UserPublicProfileFromDomain projects a domain.User into the wire
// shape. Safe for nil QuietHours entries — domain.User always carries
// DefaultPreferences on first sign-in (Story 1.1 seed), so the strings
// are guaranteed non-empty.
func UserPublicProfileFromDomain(u *domain.User) UserPublicProfile {
	return UserPublicProfile{
		UserPublic: UserPublicFromDomain(u),
		Preferences: UserPublicPreferences{
			QuietHours: QuietHoursDTO{
				Start: u.Preferences.QuietHours.Start,
				End:   u.Preferences.QuietHours.End,
			},
		},
	}
}

// ValidateProfileUpdateRequest performs the full semantic validation
// for a ProfileUpdateRequest. Returns a *AppError (ErrValidationError
// wrapped with a specific message) on failure, nil on success. The
// WS handler calls this directly because the dispatcher path does not
// benefit from Gin's automatic struct-tag validation.
//
// Rules mirror Story 1.5 AC2:
//   - At least one of displayName / timezone / quietHours must be non-nil.
//   - displayName, if non-nil, trims to [1,32] UTF-8 runes, must be
//     valid UTF-8, and must not contain ASCII control characters.
//   - timezone, if non-nil, must parse via time.LoadLocation (IANA).
//   - quietHours, if non-nil, start AND end must match HH:MM regex;
//     start == end is *allowed* (means "24h silent"; see Dev Notes
//     Semantic-correctness thought #4).
func ValidateProfileUpdateRequest(req *ProfileUpdateRequest) error {
	if req == nil {
		return profileValidationErr("payload required")
	}
	if req.DisplayName == nil && req.Timezone == nil && req.QuietHours == nil {
		return profileValidationErr("at least one of displayName/timezone/quietHours must be provided")
	}
	if req.DisplayName != nil {
		if err := validateDisplayName(*req.DisplayName); err != nil {
			return err
		}
	}
	if req.Timezone != nil {
		if err := validateTimezone(*req.Timezone); err != nil {
			return err
		}
	}
	if req.QuietHours != nil {
		if err := validateQuietHoursDTO(*req.QuietHours); err != nil {
			return err
		}
	}
	return nil
}

// validateDisplayName checks UTF-8 validity, control-character absence,
// and trimmed rune count. All three checks run regardless of order so
// the error message always identifies the first failing rule — callers
// get a deterministic reason string useful for client UI.
func validateDisplayName(s string) error {
	if !utf8.ValidString(s) {
		return profileValidationErr("displayName must be valid UTF-8")
	}
	trimmed := strings.TrimSpace(s)
	if containsASCIIControl(trimmed) {
		return profileValidationErr("displayName must not contain control characters")
	}
	n := utf8.RuneCountInString(trimmed)
	if n < displayNameMinRunes {
		return profileValidationErr(fmt.Sprintf("displayName must be at least %d character after trim", displayNameMinRunes))
	}
	if n > displayNameMaxRunes {
		return profileValidationErr(fmt.Sprintf("displayName must be at most %d characters", displayNameMaxRunes))
	}
	return nil
}

// containsASCIIControl reports whether s contains any ASCII control
// character (U+0000..U+001F or U+007F). Non-ASCII unicode control
// categories (Cc Cf …) are intentionally not screened here — clients
// send emoji and non-Latin scripts; we only refuse bytes that would
// wreck terminals, logs, or JSON transport.
func containsASCIIControl(s string) bool {
	for i := 0; i < len(s); i++ {
		b := s[i]
		if b < 0x20 || b == 0x7F {
			return true
		}
	}
	return false
}

// validateTimezone rejects strings that time.LoadLocation cannot parse.
// LoadLocation consults the zoneinfo database (tzdata) shipped with Go;
// on Windows servers it falls back to the extracted tzdata tarball —
// either way, an IANA zone like "Asia/Shanghai" passes and
// "Pacific/Nope" does not.
func validateTimezone(tz string) error {
	if tz == "" {
		return profileValidationErr("timezone must not be empty")
	}
	if _, err := time.LoadLocation(tz); err != nil {
		return profileValidationErr(fmt.Sprintf("timezone %q is not a valid IANA zone", tz))
	}
	return nil
}

// validateQuietHoursDTO checks both start and end against the HH:MM
// regex. Start == end is explicitly *allowed* per AC2 — it encodes a
// 24h silent window (see Resolver logic in real_quiet_hours_resolver.go).
func validateQuietHoursDTO(q QuietHoursDTO) error {
	if !hhmmRegex.MatchString(q.Start) {
		return profileValidationErr(fmt.Sprintf("quietHours.start %q must be HH:MM (00-23):(00-59)", q.Start))
	}
	if !hhmmRegex.MatchString(q.End) {
		return profileValidationErr(fmt.Sprintf("quietHours.end %q must be HH:MM (00-23):(00-59)", q.End))
	}
	return nil
}

// profileValidationErr returns a fresh *AppError instance (so callers
// mutating its Message field via WithCause do not poison the shared
// registry singleton).
func profileValidationErr(msg string) *AppError {
	e := *ErrValidationError
	e.Message = msg
	return &e
}
