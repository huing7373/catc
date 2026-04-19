package middleware

import (
	"errors"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/ids"
	"github.com/huing/cat/server/pkg/jwtx"
	"github.com/huing/cat/server/pkg/logx"
)

// ctxKey is a private type so accidental callers cannot collide on
// "userId" / "deviceId" plain strings — the zerolog ctx logger already
// claims those names as log field names (logx.WithUserID writes a
// "userId" field), so reusing them as gin context keys would create a
// silent bug surface (review-antipatterns §10.x). Handlers MUST go
// through the exported UserIDFrom / DeviceIDFrom / PlatformFrom
// accessors below, not c.Get("userId") directly.
type ctxKey string

const (
	ctxUserID   ctxKey = "middleware.userId"
	ctxDeviceID ctxKey = "middleware.deviceId"
	ctxPlatform ctxKey = "middleware.platform"
)

// JWTVerifier is the minimal surface JWTAuth needs from pkg/jwtx.
// Declared in this package so the middleware does not pull pkg/jwtx
// runtime state — *jwtx.Manager satisfies this implicitly. Production
// wires jwtMgr; tests pass a fake (review-antipatterns §13.1: pkg/
// must not import internal/, but consumer-side interfaces in internal/
// referencing pkg/ types are fine).
type JWTVerifier interface {
	Verify(tokenStr string) (*jwtx.CustomClaims, error)
}

// JWTAuth returns a gin handler that verifies the
// `Authorization: Bearer <access-token>` header and injects
// (userId, deviceId, platform) into the gin + std context. Mounted on
// /v1/* by wire.go from Story 1.3 onward; bootstrap endpoints
// (/auth/apple, /auth/refresh, /v1/platform/ws-registry, /healthz,
// /readyz, /ws) MUST stay outside the group.
//
// A nil verifier panics at construction so a misconfigured DI graph
// cannot silently mount an "always-allow" middleware (the exact
// failure mode Story 1.1 round 1 review caught on the WS path:
// release branch installed StubValidator instead of JWTValidator).
//
// Every reject path goes through dto.RespondAppError(c, ...) +
// c.Abort() so the client always sees a structured AppError JSON, not
// a bare 401 with empty body. Mapping rules (AC2, AC11):
//
//   - missing / non-Bearer / empty token → 401 AUTH_TOKEN_EXPIRED.
//     Client treats "no credential" the same as "credential expired" —
//     both trigger the /auth/refresh flow (epic line 818).
//   - Verify error (signature / iss / alg / kid / exp) → 401
//     AUTH_INVALID_IDENTITY_TOKEN. exp errors collapse here too for
//     MVP parity with the WS validator; future story may split them
//     if the client UX needs to distinguish.
//   - claims.TokenType != "access" → 401 AUTH_INVALID_IDENTITY_TOKEN.
//     A refresh token must NEVER unlock /v1/* endpoints.
//   - claims.UserID == "" → 401 AUTH_INVALID_IDENTITY_TOKEN. Belt-
//     and-suspenders for a Verify path that returns a structurally
//     valid token with an empty subject.
//   - claims.DeviceID == "" → 401 AUTH_INVALID_IDENTITY_TOKEN. Story
//     1.4 device registration depends on a non-empty deviceId; MVP
//     fail-closed beats "default to empty".
//
// platform may be empty and the middleware lets the request through —
// downstream endpoints that need platform (e.g. Story 2.3 POST /state)
// validate it themselves.
func JWTAuth(verifier JWTVerifier) gin.HandlerFunc {
	if verifier == nil {
		panic("middleware.JWTAuth: verifier must not be nil")
	}
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		path := c.FullPath()
		if path == "" {
			path = c.Request.URL.Path
		}

		header := c.GetHeader("Authorization")
		if header == "" {
			rejectAuth(c, "missing_header", path, dto.ErrAuthTokenExpired)
			return
		}
		token := ExtractBearerToken(header)
		if token == "" {
			// Distinguish "non-Bearer scheme" from "Bearer + empty
			// token" in the audit log so on-call can tell a confused
			// client from a buggy one.
			reason := "not_bearer"
			// ExtractBearerToken returns "" both for non-Bearer and
			// for Bearer-with-empty-token. Re-check the prefix to
			// classify.
			if len(header) >= 7 && eqFoldAscii(header[:6], "Bearer") {
				reason = "empty_token"
			}
			rejectAuth(c, reason, path, dto.ErrAuthTokenExpired)
			return
		}

		claims, err := verifier.Verify(token)
		if err != nil {
			rejectAuth(c, "verify_failed", path, dto.ErrAuthInvalidIdentityToken.WithCause(err))
			return
		}
		if claims.TokenType != "access" {
			rejectAuth(c, "token_type_mismatch", path,
				dto.ErrAuthInvalidIdentityToken.WithCause(errors.New("jwt_auth: token_type must be access")))
			return
		}
		if claims.UserID == "" {
			rejectAuth(c, "claims_missing_uid", path,
				dto.ErrAuthInvalidIdentityToken.WithCause(errors.New("jwt_auth: claims missing uid")))
			return
		}
		if claims.DeviceID == "" {
			rejectAuth(c, "claims_missing_device_id", path,
				dto.ErrAuthInvalidIdentityToken.WithCause(errors.New("jwt_auth: claims missing deviceId")))
			return
		}

		c.Set(string(ctxUserID), claims.UserID)
		c.Set(string(ctxDeviceID), claims.DeviceID)
		c.Set(string(ctxPlatform), claims.Platform)
		// Inherit userId into the std context's logger so every
		// downstream logx.Ctx(ctx).Info() call carries the field
		// (NFR-OBS-3 camelCase). Story 0.5 wired requestId the same
		// way; this is the equivalent for userId.
		c.Request = c.Request.WithContext(logx.WithUserID(ctx, claims.UserID))
		c.Next()
	}
}

// rejectAuth centralizes the 401 reject path: log an audit event with
// reason + path (NEVER userId — claims are unverified at this point;
// trusting them in the log would be the inverse of the threat model)
// then surface the AppError via the standard responder + abort.
func rejectAuth(c *gin.Context, reason string, path string, err *dto.AppError) {
	logx.Ctx(c.Request.Context()).Info().
		Str("action", "jwt_auth_reject").
		Str("reason", reason).
		Str("path", path).
		Msg("jwt_auth_reject")
	dto.RespondAppError(c, err)
	c.Abort()
}

// eqFoldAscii is an inline ASCII-only EqualFold for the 6-char
// "Bearer" prefix check; avoids allocating a substring just to call
// strings.EqualFold from the reject classification path.
func eqFoldAscii(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range len(a) {
		ca, cb := a[i], b[i]
		if ca >= 'A' && ca <= 'Z' {
			ca += 'a' - 'A'
		}
		if cb >= 'A' && cb <= 'Z' {
			cb += 'a' - 'A'
		}
		if ca != cb {
			return false
		}
	}
	return true
}

// UserIDFrom reads the userId injected by JWTAuth. Returns the empty
// UserID if the middleware did not run on this request — callers MUST
// treat empty as a programmer error (a handler reachable without the
// middleware having run), never as a valid user identity. Returning
// the typed ids.UserID rather than a bare string forces handler
// signatures to stay typed (architecture §15.3) so a service-layer
// caller cannot accidentally pass a deviceId where a userId is wanted.
func UserIDFrom(c *gin.Context) ids.UserID {
	v, ok := c.Get(string(ctxUserID))
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return ids.UserID(s)
}

// DeviceIDFrom reads the deviceId injected by JWTAuth. Returns "" if
// the middleware did not run. Bare string for now — Story 1.4 may
// introduce an ids.DeviceID alias as the device-registration API
// solidifies; bumping this signature then is a small refactor.
func DeviceIDFrom(c *gin.Context) string {
	v, ok := c.Get(string(ctxDeviceID))
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return s
}

// PlatformFrom reads the platform claim injected by JWTAuth. May be
// "" even on a happy-path request because platform is an optional
// claim — endpoints that require it (Story 2.3 POST /state) validate
// the value themselves.
func PlatformFrom(c *gin.Context) ids.Platform {
	v, ok := c.Get(string(ctxPlatform))
	if !ok {
		return ""
	}
	s, _ := v.(string)
	return ids.Platform(s)
}
