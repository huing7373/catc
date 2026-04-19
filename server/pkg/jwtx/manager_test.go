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

	"github.com/huing/cat/server/pkg/clockx"
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

	opts := Options{
		PrivateKeyPath:    activePath,
		PrivateKeyPathOld: oldPath,
		ActiveKID:         "kid-new",
		OldKID:            "kid-old",
		Issuer:            "test-issuer",
		AccessExpirySec:   900,
		RefreshExpirySec:  2592000,
	}

	m := New(opts)
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

	opts := Options{
		PrivateKeyPath:   keyPath,
		ActiveKID:        "kid-only",
		Issuer:           "test",
		AccessExpirySec:  60,
		RefreshExpirySec: 120,
	}

	m := New(opts)
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

func TestManager_Issue_PreservesRegisteredClaimsID(t *testing.T) {
	t.Parallel()
	m, _, _ := setupManager(t)

	// Deliberately picks a non-UUID sentinel so a silent m.issueClock.Now-
	// tied mutation would be visible.
	claims := CustomClaims{
		UserID:    "u1",
		TokenType: "refresh",
		RegisteredClaims: jwt.RegisteredClaims{
			ID: "jti-rolling-rotation-sentinel",
		},
	}

	tokenStr, err := m.Issue(claims)
	require.NoError(t, err)

	got, err := m.Verify(tokenStr)
	require.NoError(t, err)
	assert.Equal(t, "jti-rolling-rotation-sentinel", got.ID,
		"Issue must not overwrite caller-supplied RegisteredClaims.ID (jti)")
}

// TestManager_VerifyAndIssue_ShareInjectedClock locks the round-2
// review contract: Issue stamps IssuedAt / ExpiresAt through
// m.issueClock, Verify must run jwt's exp-check through the SAME
// clock. A token issued at fake-now with a short expiry must be
// accepted by Verify at fake-now (not "expired" because the real
// wall clock happens to have passed fake-now + expiry).
func TestManager_VerifyAndIssue_ShareInjectedClock(t *testing.T) {
	t.Parallel()
	m, _, _ := setupManager(t)

	// Pin the clock deep in the past so, if Verify silently reached
	// for time.Now(), the issued token's exp (= pinned + 15min) would
	// sit in the reviewer's nightmare window ≪ real wall clock →
	// "expired" failure. The pinned time is a fixed literal so this
	// test is deterministic across runs.
	pinned := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	m.issueClock = clockx.NewFakeClock(pinned)

	claims := CustomClaims{UserID: "u1", TokenType: "access"}
	tokenStr, err := m.Issue(claims)
	require.NoError(t, err)

	got, err := m.Verify(tokenStr)
	require.NoError(t, err, "Verify MUST use the injected clock; real wall clock is years past 2020-01-01 + 15min")
	assert.Equal(t, "u1", got.UserID)
	// Belt-and-suspenders: confirm exp really is set back in 2020.
	require.NotNil(t, got.ExpiresAt)
	assert.True(t, got.ExpiresAt.Time.Before(time.Date(2020, 1, 2, 0, 0, 0, 0, time.UTC)),
		"token exp must be anchored to the injected clock, got %s", got.ExpiresAt.Time)
}

// TestManager_Verify_ExpiredAgainstInjectedClock confirms the other
// direction: advance the injected clock past exp and Verify must
// produce an "expired" error.
func TestManager_Verify_ExpiredAgainstInjectedClock(t *testing.T) {
	t.Parallel()
	m, _, _ := setupManager(t)

	base := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	fake := clockx.NewFakeClock(base)
	m.issueClock = fake

	tokenStr, err := m.Issue(CustomClaims{UserID: "u1", TokenType: "access"})
	require.NoError(t, err)

	// Advance past the configured access expiry (900s).
	fake.Advance(2 * time.Hour)

	_, err = m.Verify(tokenStr)
	require.Error(t, err, "Verify must recognize exp relative to the injected clock")
	assert.Contains(t, err.Error(), "expired")
}

func TestManager_Issue_EmptyJTIStaysEmpty(t *testing.T) {
	t.Parallel()
	m, _, _ := setupManager(t)

	// Caller passes no jti — Issue MUST NOT magically fill one in; Story
	// 1.2 services build their own jti via ids.NewRefreshJTI so Issue
	// stays side-effect free.
	claims := CustomClaims{
		UserID:    "u1",
		TokenType: "refresh",
	}

	tokenStr, err := m.Issue(claims)
	require.NoError(t, err)

	got, err := m.Verify(tokenStr)
	require.NoError(t, err)
	assert.Empty(t, got.ID,
		"Issue must leave RegisteredClaims.ID empty when the caller did not supply one")
}

func TestManager_Verify_MissingExpiration(t *testing.T) {
	t.Parallel()
	m, activeKey, _ := setupManager(t)

	claims := CustomClaims{
		UserID:    "u1",
		TokenType: "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:   "test-issuer",
			IssuedAt: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "kid-new"
	tokenStr, err := token.SignedString(activeKey)
	require.NoError(t, err)

	_, err = m.Verify(tokenStr)
	assert.Error(t, err)
}
