package ws

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	v := NewJWTValidator(fakeVerifier{out: &jwtx.CustomClaims{UserID: "u-1", TokenType: "access"}})
	uid, err := v.ValidateToken("real-token")
	require.NoError(t, err)
	assert.Equal(t, "u-1", uid)
}

func TestJWTValidator_EmptyToken(t *testing.T) {
	t.Parallel()
	v := NewJWTValidator(fakeVerifier{})
	_, err := v.ValidateToken("")
	require.Error(t, err)
}

func TestJWTValidator_VerifyError(t *testing.T) {
	t.Parallel()
	cause := errors.New("bad sig")
	v := NewJWTValidator(fakeVerifier{err: cause})
	_, err := v.ValidateToken("forged")
	assert.ErrorIs(t, err, cause)
}

func TestJWTValidator_RejectsRefreshToken(t *testing.T) {
	t.Parallel()
	v := NewJWTValidator(fakeVerifier{out: &jwtx.CustomClaims{UserID: "u-1", TokenType: "refresh"}})
	_, err := v.ValidateToken("refresh-token")
	require.Error(t, err, "refresh tokens must NOT open a WS session")
}

func TestJWTValidator_RejectsEmptyUID(t *testing.T) {
	t.Parallel()
	v := NewJWTValidator(fakeVerifier{out: &jwtx.CustomClaims{TokenType: "access"}})
	_, err := v.ValidateToken("malformed")
	require.Error(t, err)
}

func TestNewJWTValidator_PanicsOnNilVerifier(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() { NewJWTValidator(nil) })
}
