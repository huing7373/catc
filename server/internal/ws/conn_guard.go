// Package ws conn_guard defines the consumer-side interfaces that the WS
// upgrade handler uses to enforce FR41 (per-user connect rate limit) and FR45
// (device blacklist) before a TCP upgrade completes.
//
// Why two interfaces rather than one:
//   - tools/blacklist_user writes blacklist entries and never touches the
//     rate-limit key space; a single combined interface would force the CLI
//     to depend on a rate limiter it does not use (violates minimum
//     dependencies / P2 single-concern).
//   - Unit tests frequently fake only one side. E.g. the
//     "blacklist overrides rate limit" case uses a fake limiter that is
//     always Allowed and toggles blacklist state.
//   - A future read-path endpoint (e.g. a platform debug ws-registry) may
//     want rate limiting without blacklist; an independent interface keeps
//     that composition trivial.
//
// Both interfaces are consumer-defined in internal/ws (P2). The Redis-backed
// implementations live in pkg/redisx and satisfy these interfaces through
// Go's structural typing — no explicit "implements" declaration is needed.
//
// Spec refs: FR41, FR45, NFR-SCALE-5 (60s ≤ 5), NFR-SEC-8, NFR-SEC-10.
package ws

import (
	"context"

	"github.com/huing/cat/server/pkg/redisx"
)

// ConnectDecision is the value returned by ConnectRateLimiter. Aliased from
// pkg/redisx so the interface defined here and the RedisConnectRateLimiter
// implementation refer to the same concrete type (Go uses nominal typing for
// interface method signatures — see pkg/redisx/conn_ratelimit.go and the
// same pattern used for DedupResult in dedup.go).
type ConnectDecision = redisx.ConnectDecision

// Blacklist is the read-only check performed by the WS upgrade handler
// before accepting a connection. The write path (Add/Remove/TTL) lives on
// the Redis implementation type and is consumed only by tools/ — exposing
// writes here would drag operational concerns into the protocol layer (P2).
//
// userID is the JWT subject resolved by the token validator. It is a
// controlled format (ObjectID hex in production; bearer token in debug mode)
// and is assumed not to contain key-space delimiters such as ":".
type Blacklist interface {
	IsBlacklisted(ctx context.Context, userID string) (bool, error)
}

// ConnectRateLimiter bounds WS connect attempts per user within a sliding
// window. On over-threshold attempts, Allowed=false and RetryAfter is the
// remaining window duration; callers translate that into the Retry-After
// HTTP header via dto.ErrRateLimitExceeded.WithRetryAfter.
type ConnectRateLimiter interface {
	AcquireConnectSlot(ctx context.Context, userID string) (ConnectDecision, error)
}
