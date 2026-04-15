package repository

import "github.com/huing7373/catc/server/pkg/ids"

// Centralised Redis key builders. No key literal is allowed anywhere
// outside this file; every Set must have an explicit TTL at its call
// site.

func userCacheKey(uid ids.UserID) string {
	return "user:" + string(uid)
}

func touchRateLimitKey(uid ids.UserID) string {
	return "ratelimit:touch:" + string(uid)
}

func tokenBlacklistKey(jti string) string {
	return "token:blacklist:" + jti
}

func skinListCacheKey(uid ids.UserID) string {
	return "skins:owned:" + string(uid)
}

// TouchRateLimitKey exposes the touch rate-limit key builder to the
// middleware layer.
func TouchRateLimitKey(uid ids.UserID) string { return touchRateLimitKey(uid) }

// TokenBlacklistKey exposes the token-blacklist key builder to service
// layer callers (e.g. logout).
func TokenBlacklistKey(jti string) string { return tokenBlacklistKey(jti) }

// SkinListCacheKey exposes the skin list cache key builder to future
// skin repository code.
func SkinListCacheKey(uid ids.UserID) string { return skinListCacheKey(uid) }

// RateLimitKey returns the Redis key used by the sliding-window
// rate-limiter. bucket is a coarse bucket name ("auth-login",
// "touch-send", etc.); subject is the per-actor identifier (IP, user
// id…). Centralising the key prevents fmt.Sprintf scatter at call sites.
func RateLimitKey(bucket, subject string) string {
	return "ratelimit:" + bucket + ":" + subject
}
