package middleware

import "strings"

// ExtractBearerToken pulls the opaque token out of an Authorization
// header following RFC 6750. Returns "" on any format deviation — an
// empty value is the sole signal the caller surfaces as 401
// AUTH_TOKEN_EXPIRED (HTTP middleware) or AUTH_INVALID_IDENTITY_TOKEN
// (WS upgrade), depending on how strict that path needs to be.
//
// Shared between HTTP middleware (JWTAuth) and WS upgrade
// (internal/ws/upgrade_handler.go) so the two code paths cannot
// diverge on edge cases — the most insidious being case-insensitive
// "Bearer" handling (RFC 6750 §2.1 declares the scheme case-
// insensitive) and the "BearerToken" run-on form which must NOT match.
//
// The leading-token TrimSpace is deliberate: clients sometimes send
// `Authorization: Bearer  <token>` with a double-space; without the
// trim the token would arrive as " <token>" and Verify would treat it
// as a non-empty but malformed JWT, producing a confusing
// AUTH_INVALID_IDENTITY_TOKEN instead of a clean accept.
func ExtractBearerToken(header string) string {
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
