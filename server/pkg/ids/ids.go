// Package ids defines typed string aliases for domain identifiers.
//
// Per docs/backend-architecture-guide.md §15.3 "Typed IDs", each logical
// identifier (UserID, FriendID, SkinID, …) is a distinct named type so the
// compiler catches accidental mixing — e.g. passing a friendId where a
// userId is expected. Aliases are `string` (not `struct{}`) because the
// persisted/serialized representation is always a bare string (Mongo
// ObjectID hex, UUID, or JWT subject) and a struct wrapper would force
// every Mongo/JSON boundary to marshal-unmarshal for no behavioural gain.
//
// Story 0.13 introduces only UserID and Platform — the two IDs the push
// platform needs. Later stories append as they land (1.1 real UserID
// issuance, 1.4 ApnsTokenID, 3.1 InviteTokenID, 4.2 RoomID, …) per the
// YAGNI rule.
package ids

import "github.com/google/uuid"

// UserID uniquely identifies a registered user. Story 1.1 issues these as
// UUID v4 strings (see NewUserID below) on first Sign in with Apple; the
// value is also the Mongo `_id` of the `users` document and the JWT
// subject. Callers treat UserID as opaque — the push platform, WS hub,
// and dedup layer all log it as-is without masking (§M13 — userId is not
// PII).
type UserID string

// Platform enumerates the device classes that may register APNs tokens.
// Persisted as a bare string in the `apns_tokens` Mongo collection and as
// a Redis field value in `apns:queue` — the typed constant prevents typos
// at Go call sites while the wire format stays a plain string for schema
// stability (FR58).
type Platform string

const (
	// PlatformWatch is the primary surface for push delivery (watchOS
	// companion — FR58 maps this to WatchTopic).
	PlatformWatch Platform = "watch"
	// PlatformIphone is the fallback surface when the user has an iPhone
	// registered (e.g. low-battery watch offline, FR44b cold-start recall).
	PlatformIphone Platform = "iphone"
)

// NewUserID returns a fresh UUID v4 encoded UserID. Story 1.1 calls this
// for new Mongo `users` documents on first Sign in with Apple; tests use
// it for fixture identity. A failure from the crypto RNG is fatal — the
// process cannot produce a safe user id and must not continue. Panic
// (rather than log.Fatal) is the right shape because this helper runs on
// the request path: panic surfaces as INTERNAL_ERROR via the recover
// middleware, instead of taking the whole process down for one bad
// request.
func NewUserID() UserID {
	id, err := uuid.NewRandom()
	if err != nil {
		panic("ids.NewUserID: " + err.Error())
	}
	return UserID(id.String())
}

// NewRefreshJTI returns a fresh UUID v4 encoded JTI string for a
// refresh token. The JTI is the server-side canonical identifier for
// the token — it is written to users.sessions[deviceId].current_jti and
// used as the refresh_blacklist key suffix. UUID v4 guarantees both
// uniqueness and colon-freedom, so `refresh_blacklist:<jti>` is
// injective without length-prefix encoding (review-antipatterns §8.2).
//
// Intentionally distinct from NewUserID to keep identity IDs and token
// IDs separable under grep / code review. Failure handling mirrors
// NewUserID: panic — surfaces as INTERNAL_ERROR via the recover
// middleware on the request path (Story 0.5 mechanism).
func NewRefreshJTI() string {
	id, err := uuid.NewRandom()
	if err != nil {
		panic("ids.NewRefreshJTI: " + err.Error())
	}
	return id.String()
}
