package jwtx

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/config"
)

func writeKeyFile(t *testing.T, dir, name string, key *rsa.PrivateKey) string {
	t.Helper()
	path := filepath.Join(dir, name)
	der, err := x509.MarshalPKCS8PrivateKey(key)
	require.NoError(t, err)
	f, err := os.Create(path)
	require.NoError(t, err)
	defer f.Close()
	require.NoError(t, pem.Encode(f, &pem.Block{Type: "PRIVATE KEY", Bytes: der}))
	return path
}

func setupManager(t *testing.T) (*Manager, *rsa.PrivateKey, *rsa.PrivateKey) {
	t.Helper()
	dir := t.TempDir()

	activeKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	oldKey, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	activePath := writeKeyFile(t, dir, "active.pem", activeKey)
	oldPath := writeKeyFile(t, dir, "old.pem", oldKey)

	cfg := config.JWTCfg{
		PrivateKeyPath:    activePath,
		PrivateKeyPathOld: oldPath,
		ActiveKID:         "kid-new",
		OldKID:            "kid-old",
		Issuer:            "test-issuer",
		AccessExpirySec:   900,
		RefreshExpirySec:  2592000,
	}

	m := New(cfg)
	return m, activeKey, oldKey
}

func TestManager_Issue_Verify_RoundTrip(t *testing.T) {
	t.Parallel()
	m, _, _ := setupManager(t)

	claims := CustomClaims{
		UserID:    "user-123",
		DeviceID:  "dev-456",
		Platform:  "watchos",
		TokenType: "access",
	}

	tokenStr, err := m.Issue(claims)
	require.NoError(t, err)
	require.NotEmpty(t, tokenStr)

	got, err := m.Verify(tokenStr)
	require.NoError(t, err)
	assert.Equal(t, "user-123", got.UserID)
	assert.Equal(t, "dev-456", got.DeviceID)
	assert.Equal(t, "watchos", got.Platform)
	assert.Equal(t, "access", got.TokenType)
	assert.Equal(t, "test-issuer", got.Issuer)
}

func TestManager_Issue_RefreshToken_LongerExpiry(t *testing.T) {
	t.Parallel()
	m, _, _ := setupManager(t)

	accessClaims := CustomClaims{TokenType: "access"}
	refreshClaims := CustomClaims{TokenType: "refresh"}

	accessToken, err := m.Issue(accessClaims)
	require.NoError(t, err)
	refreshToken, err := m.Issue(refreshClaims)
	require.NoError(t, err)

	accessParsed, err := m.Verify(accessToken)
	require.NoError(t, err)
	refreshParsed, err := m.Verify(refreshToken)
	require.NoError(t, err)

	accessDur := accessParsed.ExpiresAt.Time.Sub(accessParsed.IssuedAt.Time)
	refreshDur := refreshParsed.ExpiresAt.Time.Sub(refreshParsed.IssuedAt.Time)
	assert.Greater(t, refreshDur, accessDur)
}

func TestManager_Verify_ExpiredToken(t *testing.T) {
	t.Parallel()
	m, activeKey, _ := setupManager(t)

	claims := CustomClaims{
		UserID:    "user-expired",
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test-issuer",
			IssuedAt:  jwt.NewNumericDate(time.Now().Add(-2 * time.Hour)),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-1 * time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "kid-new"
	tokenStr, err := token.SignedString(activeKey)
	require.NoError(t, err)

	_, err = m.Verify(tokenStr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "expired")
}

func TestManager_Verify_UnknownKID(t *testing.T) {
	t.Parallel()
	m, activeKey, _ := setupManager(t)

	claims := CustomClaims{UserID: "user-bad-kid", TokenType: "access"}
	claims.RegisteredClaims = jwt.RegisteredClaims{
		Issuer:    "test-issuer",
		IssuedAt:  jwt.NewNumericDate(time.Now()),
		ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "kid-unknown"
	tokenStr, err := token.SignedString(activeKey)
	require.NoError(t, err)

	_, err = m.Verify(tokenStr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown kid")
}

func TestManager_Verify_OldKID_StillWorks(t *testing.T) {
	t.Parallel()
	m, _, oldKey := setupManager(t)

	claims := CustomClaims{
		UserID:    "user-old-key",
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test-issuer",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "kid-old"
	tokenStr, err := token.SignedString(oldKey)
	require.NoError(t, err)

	got, err := m.Verify(tokenStr)
	require.NoError(t, err)
	assert.Equal(t, "user-old-key", got.UserID)
}

func TestManager_New_WithoutOldKey(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	keyPath := writeKeyFile(t, dir, "key.pem", key)

	cfg := config.JWTCfg{
		PrivateKeyPath:   keyPath,
		ActiveKID:        "kid-only",
		Issuer:           "test",
		AccessExpirySec:  60,
		RefreshExpirySec: 120,
	}

	m := New(cfg)
	assert.NotNil(t, m)
	assert.Nil(t, m.oldPub)

	claims := CustomClaims{UserID: "u1", TokenType: "access"}
	tokenStr, err := m.Issue(claims)
	require.NoError(t, err)

	got, err := m.Verify(tokenStr)
	require.NoError(t, err)
	assert.Equal(t, "u1", got.UserID)
}

func TestManager_Issue_PreservesJTI(t *testing.T) {
	t.Parallel()
	m, _, _ := setupManager(t)

	claims := CustomClaims{
		UserID:    "u1",
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ID: "my-jti-123",
		},
	}

	tokenStr, err := m.Issue(claims)
	require.NoError(t, err)

	got, err := m.Verify(tokenStr)
	require.NoError(t, err)
	assert.Equal(t, "my-jti-123", got.ID)
}

func TestManager_Issue_PreservesRegisteredClaims(t *testing.T) {
	t.Parallel()
	m, _, _ := setupManager(t)

	claims := CustomClaims{
		UserID:    "u1",
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			ID:       "jti-abc",
			Subject:  "sub-xyz",
			Audience: jwt.ClaimStrings{"aud1", "aud2"},
		},
	}

	tokenStr, err := m.Issue(claims)
	require.NoError(t, err)

	got, err := m.Verify(tokenStr)
	require.NoError(t, err)
	assert.Equal(t, "jti-abc", got.ID)
	assert.Equal(t, "sub-xyz", got.Subject)
	assert.Equal(t, jwt.ClaimStrings{"aud1", "aud2"}, got.Audience)
}

func TestManager_Verify_WrongIssuer(t *testing.T) {
	t.Parallel()
	m, activeKey, _ := setupManager(t)

	claims := CustomClaims{
		UserID:    "u1",
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "wrong-issuer",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "kid-new"
	tokenStr, err := token.SignedString(activeKey)
	require.NoError(t, err)

	_, err = m.Verify(tokenStr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "issuer")
}

func TestManager_Verify_WrongSigningMethod(t *testing.T) {
	t.Parallel()
	m, activeKey, _ := setupManager(t)

	claims := CustomClaims{
		UserID:    "u1",
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "test-issuer",
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS384, claims)
	token.Header["kid"] = "kid-new"
	tokenStr, err := token.SignedString(activeKey)
	require.NoError(t, err)

	_, err = m.Verify(tokenStr)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "signing method")
}
