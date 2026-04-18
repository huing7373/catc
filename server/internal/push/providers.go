// Package push — APNs push platform (Story 0.13).
//
// providers.go defines the four consumer-side interfaces that the APNs
// worker and cron cleanup job depend on, plus Epic-0 Empty* implementations
// that let the pipeline boot end-to-end before Story 1.4 / 1.5 land real
// Mongo-backed repositories and user-preference resolution.
//
// # Epic 0 / future owners
//
//	TokenProvider        — real impl: Story 1.4 (apns_tokens repo)
//	TokenDeleter         — real impl: Story 1.4 (apns_tokens repo, FR43 410 cleanup)
//	TokenCleaner         — real impl: Story 1.4 (apns_tokens repo, 30-day retention)
//	QuietHoursResolver   — real impl: Story 1.5 (user preferences.quietHours + timezone)
//
// Keeping the interfaces in `internal/push/` (alongside Pusher /
// ApnsSender) is the correct co-location because this package IS the
// push abstraction — every future consumer imports `internal/push` and
// sees the interfaces plus the Pusher entry-point as a single API surface.
package push

import (
	"context"
	"time"

	"github.com/huing/cat/server/pkg/ids"
)

// TokenInfo is a minimal view of a registered APNs device token — enough
// for the router to pick a topic bundle and for the worker to send. The
// DeviceToken field is the unencrypted hex string; Story 1.4's repository
// is expected to decrypt on read (NFR-SEC-7), so consumers of this struct
// receive plaintext.
//
// Platform is a bare `string` (not `ids.Platform`) so the JSON and Mongo
// shapes stay schema-flat — this avoids forcing Story 1.4 to depend on
// `pkg/ids` at the repository layer. Valid values are the `ids.Platform`
// string literals ("watch", "iphone").
type TokenInfo struct {
	Platform    string
	DeviceToken string
}

// TokenProvider lists every currently-registered (user, token) pair. Used
// by APNsRouter to fan out one logical enqueue to every device the user
// has registered.
//
// Real impl (Story 1.4): reads `apns_tokens` collection with index on
// userId. Returns empty slice (not error) when the user has no tokens —
// this is the MVP default (user has not yet registered any device) and
// the worker treats it as "silently ACK and move on".
type TokenProvider interface {
	ListTokens(ctx context.Context, userID ids.UserID) ([]TokenInfo, error)
}

// TokenDeleter removes a single (user, token) pair after APNs responds
// with HTTP 410 (Unregistered / BadDeviceToken). This is the FR43
// contract.
//
// Real impl (Story 1.4): DELETE on `apns_tokens` by (userId, deviceToken).
// Missing row is a no-op (returns nil) — idempotent against concurrent
// 410s across replicas. Returns error only on Mongo I/O failure.
type TokenDeleter interface {
	Delete(ctx context.Context, userID ids.UserID, deviceToken string) error
}

// TokenCleaner bulk-deletes stale rows on the daily cron (NFR-SEC-7
// 30-day retention). cutoff is an absolute timestamp — all rows with
// updatedAt < cutoff are removed. Returns the deleted-row count for
// logging.
//
// Real impl (Story 1.4): DeleteMany on `apns_tokens` where updatedAt <
// cutoff.
type TokenCleaner interface {
	DeleteExpired(ctx context.Context, cutoff time.Time) (int64, error)
}

// QuietHoursResolver reports whether the user is inside their
// locally-configured quiet window right now. The worker calls this at
// consume time (not enqueue time — see Dev Notes) so the freshest
// preference wins, even if the user flipped quiet mode between Enqueue
// and Send.
//
// # Fail-open contract
//
// Missing user / missing timezone / missing quietHours → returns
// (false, nil). Erring on the side of delivering the push rather than
// silencing it: a silenced-but-wanted notification is a product regression
// (user never hears about a touch) while a loud-but-should-be-silent
// notification is annoying-but-recoverable. Story 1.5's real impl mirrors
// this.
type QuietHoursResolver interface {
	Resolve(ctx context.Context, userID ids.UserID) (quiet bool, err error)
}

// EmptyTokenProvider is the Epic-0 stub — no registered tokens for any
// user, so every enqueue results in the worker ACKing with "no tokens"
// and never invoking the APNs sender. Swap for the Story 1.4 real impl in
// cmd/cat/initialize.go when that story lands.
type EmptyTokenProvider struct{}

// ListTokens always returns (nil, nil).
func (EmptyTokenProvider) ListTokens(context.Context, ids.UserID) ([]TokenInfo, error) {
	return nil, nil
}

// EmptyTokenDeleter is the Epic-0 stub — Delete is a no-op. Safe default
// because with EmptyTokenProvider returning no tokens, the worker never
// invokes the sender and therefore never sees a 410 to act on.
type EmptyTokenDeleter struct{}

// Delete always returns nil.
func (EmptyTokenDeleter) Delete(context.Context, ids.UserID, string) error { return nil }

// EmptyTokenCleaner is the Epic-0 stub — reports zero rows deleted so the
// cron heartbeat still runs (proves the wiring) without touching Mongo.
type EmptyTokenCleaner struct{}

// DeleteExpired always returns (0, nil).
func (EmptyTokenCleaner) DeleteExpired(context.Context, time.Time) (int64, error) {
	return 0, nil
}

// EmptyQuietHoursResolver is the Epic-0 stub — always "not quiet", so the
// worker delivers every alert-kind push as alert. Matches the
// fail-open contract.
type EmptyQuietHoursResolver struct{}

// Resolve always returns (false, nil).
func (EmptyQuietHoursResolver) Resolve(context.Context, ids.UserID) (bool, error) {
	return false, nil
}
