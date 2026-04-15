package dto

import "time"

// LoginReq is the inbound payload for POST /v1/auth/login.
//
// AppleJWT is the raw Apple identity token (iOS / watchOS receives it
// from ASAuthorizationAppleIDCredential.identityToken).
//
// Nonce is the RAW client-side nonce. The client also supplies
// sha256(nonce) hex to Apple's button; the backend recomputes it
// internally and compares against the token's nonce claim. Sending the
// hashed nonce here will fail verification.
//
// DeviceID is identifierForVendor — used to attribute the active
// session to a specific device for diagnostics and future migration
// flows (Story 2.5).
type LoginReq struct {
	AppleJWT string `json:"apple_jwt" binding:"required"`
	Nonce    string `json:"nonce"     binding:"required,min=16,max=128"`
	DeviceID string `json:"device_id" binding:"max=128"`
}

// LoginResp is the success payload for POST /v1/auth/login. It is
// returned verbatim (no {"data": ...} wrapper).
type LoginResp struct {
	UserID           string    `json:"user_id"`
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
	LoginOutcome     string    `json:"login_outcome"` // "created" | "existing" | "restored"
}

// RefreshReq is the inbound payload for POST /v1/auth/refresh.
type RefreshReq struct {
	RefreshToken string `json:"refresh_token" binding:"required"`
}

// RefreshResp mirrors LoginResp without the LoginOutcome field.
type RefreshResp struct {
	UserID           string    `json:"user_id"`
	AccessToken      string    `json:"access_token"`
	RefreshToken     string    `json:"refresh_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}
