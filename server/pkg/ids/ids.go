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

// UserID uniquely identifies a registered user. In release mode this is
// the Mongo `_id` of the `users` document (ObjectID hex); in debug mode it
// can be an arbitrary string carried in the JWT subject or bearer token.
// Callers should treat UserID as opaque — the push platform, WS hub, and
// dedup layer all log it as-is without masking (§M13 — userId is not PII).
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
