package ws

import (
	"errors"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/jwtx"
)

// jwtVerifier is the small surface jwtValidator needs from jwtx.Manager.
// Defined locally so this file does not import any concrete Manager
// state — *jwtx.Manager satisfies it implicitly. Production wiring
// passes jwtMgr; tests pass a fake.
type jwtVerifier interface {
	Verify(tokenStr string) (*jwtx.CustomClaims, error)
}

type jwtValidator struct {
	verifier jwtVerifier
}

// NewJWTValidator returns the release-mode TokenValidator that accepts
// access tokens minted by jwtx.Manager.Issue. It refuses tokens whose
// `ttype` claim is not `access` so a stolen refresh token cannot open
// a WS session — refresh tokens are only consumable on /auth/refresh
// (Story 1.2).
//
// All rejection paths return dto.ErrAuthInvalidIdentityToken (with the
// real cause attached for the server log) so UpgradeHandler.Handle can
// hand the result to dto.RespondAppError and the client sees 401
// AUTH_INVALID_IDENTITY_TOKEN — never 500. The same sentinel was used
// by the Story 0 StubValidator; round 2 of Story 1.1 review caught the
// regression where ValidateToken returned plain errors and the WS
// upgrade response collapsed expired / forged / refresh-as-access /
// missing-uid tokens all into INTERNAL_ERROR.
//
// verifier MUST be non-nil; passing nil panics at construction so a
// misconfigured DI graph cannot reach request time and silently
// accept every token (the bug Round 1 of Story 1.1 review caught
// when the release branch installed StubValidator instead).
func NewJWTValidator(verifier jwtVerifier) TokenValidator {
	if verifier == nil {
		panic("ws.NewJWTValidator: verifier must not be nil")
	}
	return jwtValidator{verifier: verifier}
}

func (v jwtValidator) ValidateToken(token string) (string, error) {
	if token == "" {
		return "", dto.ErrAuthInvalidIdentityToken.WithCause(errors.New("empty token"))
	}
	claims, err := v.verifier.Verify(token)
	if err != nil {
		return "", dto.ErrAuthInvalidIdentityToken.WithCause(err)
	}
	if claims.TokenType != "access" {
		return "", dto.ErrAuthInvalidIdentityToken.WithCause(errors.New("ws: token_type must be access"))
	}
	if claims.UserID == "" {
		return "", dto.ErrAuthInvalidIdentityToken.WithCause(errors.New("ws: claims missing uid"))
	}
	return claims.UserID, nil
}
