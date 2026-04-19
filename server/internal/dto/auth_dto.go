package dto

import (
	"github.com/huing/cat/server/internal/domain"
)

// SignInWithAppleRequest is the wire format for POST /auth/apple. Gin
// validates the struct via go-playground/validator using the binding
// tags below; failures get re-wrapped as ErrValidationError before
// reaching the AuthService.
//
// Field rationale:
//   - identityToken: Apple JWTs are typically ~1 KB; 8 KB cap leaves
//     room for future Apple field additions while still bounding what
//     a malicious client can ship.
//   - authorizationCode: optional in the MVP (we don't exchange it for
//     server-to-Apple refresh tokens yet); accept up to 1 KB for
//     forward compatibility.
//   - deviceId: client-generated UUID v4 stored in Keychain (pre-auth
//     identity used for Story 1.2 refresh tracking and Story 1.4 APNs
//     binding).
//   - platform: enum locked to "watch" / "iphone" — Story 0.13 ids
//     enum.
//   - nonce: the *raw* nonce the client supplied to Apple SIWA. The
//     service hashes it and compares against claims.Nonce. 8-128 char
//     range covers Apple's 32-byte base64-encoded recommendation
//     (~44 chars) with comfortable headroom both ways.
type SignInWithAppleRequest struct {
	IdentityToken     string `json:"identityToken"               binding:"required,min=1,max=8192"`
	AuthorizationCode string `json:"authorizationCode,omitempty" binding:"omitempty,max=1024"`
	DeviceID          string `json:"deviceId"                    binding:"required,uuid"`
	Platform          string `json:"platform"                    binding:"required,oneof=watch iphone"`
	Nonce             string `json:"nonce"                       binding:"required,min=8,max=128"`
}

// SignInWithAppleResponse is the success body for POST /auth/apple
// (200). Tokens are per-device; the User block is the minimum
// projection a client needs to land on the home screen — full profile
// preferences land via /me in Story 1.5.
type SignInWithAppleResponse struct {
	AccessToken  string     `json:"accessToken"`
	RefreshToken string     `json:"refreshToken"`
	User         UserPublic `json:"user"`
}

// UserPublic is the wire-safe projection of a domain.User for the
// SignInWithApple / SessionResume responses. No PII / hash fields,
// no internal flags (deletion / friend count / sessions). Story 1.5
// extends with the preferences block once the profile endpoint lands.
type UserPublic struct {
	ID          string  `json:"id"`
	DisplayName *string `json:"displayName,omitempty"`
	Timezone    *string `json:"timezone,omitempty"`
}

// UserPublicFromDomain projects a domain.User into UserPublic. Living
// here in the dto package keeps internal/handler one less import deep
// and makes the conversion reusable from internal/ws.RealUserProvider
// (Story 1.1 AC11).
func UserPublicFromDomain(u *domain.User) UserPublic {
	return UserPublic{
		ID:          string(u.ID),
		DisplayName: u.DisplayName,
		Timezone:    u.Timezone,
	}
}
