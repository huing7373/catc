package dto

// RegisterApnsTokenRequest is the wire format for
// POST /v1/devices/apns-token (Story 1.4).
//
// Field rationale:
//
//   - deviceToken: historically a 32-byte APNs identifier expressed as
//     64 hex chars. Apple reserves the right to change the length in
//     future OS releases, so the validator is intentionally liberal on
//     upper bound (max=200) while still rejecting obviously malformed /
//     truncated input (min=8). "hexadecimal" is the
//     go-playground/validator built-in tag verifying [0-9a-fA-F]+ only.
//   - platform: OPTIONAL. When present it MUST match the authenticated
//     JWT platform claim — DeviceHandler enforces that cross-check as
//     defense-in-depth (see AC5). When absent, the JWT's platform wins.
//     A JWT with NO platform claim is rejected with 401 — no fallback
//     to body (AC5 / §21.8 #6: attack surface closed).
type RegisterApnsTokenRequest struct {
	DeviceToken string `json:"deviceToken" binding:"required,min=8,max=200,hexadecimal"`
	Platform    string `json:"platform,omitempty" binding:"omitempty,oneof=watch iphone"`
}

// RegisterApnsTokenResponse is the success body — deliberately
// minimal. The client never reads a body value more specific than
// "we got it"; re-register is idempotent so no server-side sequence
// number is needed.
type RegisterApnsTokenResponse struct {
	Ok bool `json:"ok"`
}
