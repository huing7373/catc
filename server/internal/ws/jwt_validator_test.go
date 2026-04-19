package ws

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/pkg/jwtx"
)

type fakeVerifier struct {
	out *jwtx.CustomClaims
	err error
}

func (f fakeVerifier) Verify(_ string) (*jwtx.CustomClaims, error) {
	return f.out, f.err
}

func TestJWTValidator_HappyPath(t *testing.T) {
	t.Parallel()
	v := NewJWTValidator(fakeVerifier{out: &jwtx.CustomClaims{
		UserID:    "u-1",
		DeviceID:  "d-1",
		Platform:  "iphone",
		TokenType: "access",
	}})
	identity, err := v.ValidateToken("real-token")
	require.NoError(t, err)
	assert.Equal(t, "u-1", identity.UserID)
	assert.Equal(t, "d-1", identity.DeviceID)
	assert.Equal(t, "iphone", identity.Platform)
}

// TestJWTValidator_RejectsEmptyDeviceID locks the AC5 fail-closed
// branch: a JWT that survives signature + iss + alg + exp + ttype +
// uid checks but carries an empty deviceId is still rejected. Mirrors
// the HTTP middleware behavior so the (userId, deviceId) pair is a
// load-bearing invariant on both auth paths.
func TestJWTValidator_RejectsEmptyDeviceID(t *testing.T) {
	t.Parallel()
	v := NewJWTValidator(fakeVerifier{out: &jwtx.CustomClaims{
		UserID:    "u-1",
		DeviceID:  "",
		TokenType: "access",
	}})
	_, err := v.ValidateToken("missing-device-token")
	require.Error(t, err)
	assert.ErrorIs(t, err, dto.ErrAuthInvalidIdentityToken,
		"empty-deviceId reject must surface as AUTH_INVALID_IDENTITY_TOKEN")
}

// All four reject paths must wrap the cause inside
// dto.ErrAuthInvalidIdentityToken so RespondAppError returns 401, not
// 500. This is the Round 2 regression — a plain `errors.New` here
// silently became INTERNAL_ERROR in the production WS upgrade.
func TestJWTValidator_EmptyToken(t *testing.T) {
	t.Parallel()
	v := NewJWTValidator(fakeVerifier{})
	_, err := v.ValidateToken("")
	require.Error(t, err)
	assert.ErrorIs(t, err, dto.ErrAuthInvalidIdentityToken,
		"empty-token reject must surface as AUTH_INVALID_IDENTITY_TOKEN, not INTERNAL_ERROR")
}

func TestJWTValidator_VerifyError(t *testing.T) {
	t.Parallel()
	cause := errors.New("bad sig")
	v := NewJWTValidator(fakeVerifier{err: cause})
	_, err := v.ValidateToken("forged")
	assert.ErrorIs(t, err, dto.ErrAuthInvalidIdentityToken,
		"verifier error must surface as AUTH_INVALID_IDENTITY_TOKEN, not INTERNAL_ERROR")
	assert.ErrorIs(t, err, cause, "underlying cause must remain reachable for server logs")
}

func TestJWTValidator_RejectsRefreshToken(t *testing.T) {
	t.Parallel()
	v := NewJWTValidator(fakeVerifier{out: &jwtx.CustomClaims{UserID: "u-1", TokenType: "refresh"}})
	_, err := v.ValidateToken("refresh-token")
	require.Error(t, err, "refresh tokens must NOT open a WS session")
	assert.ErrorIs(t, err, dto.ErrAuthInvalidIdentityToken,
		"refresh-as-access reject must surface as AUTH_INVALID_IDENTITY_TOKEN")
}

func TestJWTValidator_RejectsEmptyUID(t *testing.T) {
	t.Parallel()
	v := NewJWTValidator(fakeVerifier{out: &jwtx.CustomClaims{TokenType: "access"}})
	_, err := v.ValidateToken("malformed")
	require.Error(t, err)
	assert.ErrorIs(t, err, dto.ErrAuthInvalidIdentityToken,
		"empty-uid reject must surface as AUTH_INVALID_IDENTITY_TOKEN")
}

func TestNewJWTValidator_PanicsOnNilVerifier(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() { NewJWTValidator(nil) })
}
