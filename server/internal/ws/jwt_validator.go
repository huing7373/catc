package ws

import (
	"errors"

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
		return "", errors.New("empty token")
	}
	claims, err := v.verifier.Verify(token)
	if err != nil {
		return "", err
	}
	if claims.TokenType != "access" {
		return "", errors.New("ws: token_type must be access")
	}
	if claims.UserID == "" {
		return "", errors.New("ws: claims missing uid")
	}
	return claims.UserID, nil
}
