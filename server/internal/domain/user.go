package domain

import (
	"time"

	"github.com/huing/cat/server/pkg/ids"
)

// User is the aggregate root for the authenticated identity. Mongo persists
// one document per Apple-account-linked user in the `users` collection. Story
// 1.1 writes the fields marked "seed"; later stories (1.2 sessions /
// refresh, 1.4 APNs binding, 1.5 profile, 1.6 deletion, 3.x friend count)
// fill the rest. Repo ↔ Service boundary M7 applies: repos return
// *domain.User, never raw bson.M.
//
// Field-level BSON tags are snake_case per architecture §P1. JSON
// projections live in internal/dto (M8 — handler-layer DTOs); the domain
// type itself never serializes directly to the HTTP / WS wire.
type User struct {
	ID                  ids.UserID         `bson:"_id"`                            // seed; UUID v4 string
	AppleUserIDHash     string             `bson:"apple_user_id_hash"`             // seed; SHA-256 hex of Apple `sub`, unique index
	DisplayName         *string            `bson:"display_name"`                   // 1.5; nil until user sets one
	Timezone            *string            `bson:"timezone"`                       // 1.5; nil until user sets IANA tz
	Preferences         UserPreferences    `bson:"preferences"`                    // seed with defaults
	FriendCount         int                `bson:"friend_count"`                   // 3.x; seed as 0
	Consents            UserConsents       `bson:"consents"`                       // seed
	Sessions            map[string]Session `bson:"sessions"`                       // 1.2/1.4; seed empty map (BSON {})
	DeletionRequested   bool               `bson:"deletion_requested"`             // 1.6; seed false
	DeletionRequestedAt *time.Time         `bson:"deletion_requested_at,omitempty"` // 1.6; seed nil
	CreatedAt           time.Time          `bson:"created_at"`                     // seed = Clock.Now()
	UpdatedAt           time.Time          `bson:"updated_at"`                     // seed = Clock.Now()
}

// UserPreferences holds user-tunable preferences. QuietHours.Start/End are
// "HH:MM" strings to keep BSON / JSON encoding trivially comparable across
// the iOS / watchOS clients (Story 1.5 will let users edit these).
type UserPreferences struct {
	QuietHours QuietHours `bson:"quiet_hours"`
}

// QuietHours expresses a per-day silent window (PRD NFR-COMP-2 / Story 5.5).
// Start may be after End to express an overnight window (e.g. 23:00 → 07:00).
type QuietHours struct {
	Start string `bson:"start"` // "HH:MM"
	End   string `bson:"end"`   // "HH:MM"
}

// UserConsents records explicit data-use consents. Pointers distinguish
// "not asked yet" (nil) from explicit "yes" / "no" — important because the
// HealthKit step prompt (Story 2.3) only fires once and we must not
// re-prompt users who already declined.
type UserConsents struct {
	StepData *bool `bson:"step_data"` // nil until user answers the HealthKit prompt (Story 2.3)
}

// Session is reserved for 1.2 (refresh jti tracking) and 1.4 (APNs token
// binding). Story 1.1 seeds an empty map so the BSON shape is stable and
// 1.2 can write `$set sessions.<deviceId>` without a schema migration.
type Session struct {
	CurrentJTI   string    `bson:"current_jti"`
	IssuedAt     time.Time `bson:"issued_at"`
	HasApnsToken bool      `bson:"has_apns_token"`
}

// DefaultPreferences is the snapshot of preferences Story 1.1 seeds onto a
// brand-new User document on first sign-in. Returns a fresh value each
// call so callers cannot accidentally mutate a shared singleton — the M6
// "value types over pointers" rule applies for small leaf types.
func DefaultPreferences() UserPreferences {
	return UserPreferences{QuietHours: QuietHours{Start: "23:00", End: "07:00"}}
}
