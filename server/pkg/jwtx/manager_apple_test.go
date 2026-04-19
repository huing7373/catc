package jwtx

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/pkg/clockx"
)

// appleHarness is the in-package fake-Apple stack used by the
// VerifyApple suite. It deliberately duplicates the simpler version of
// `internal/testutil.FakeApple` to keep this test file inside the
// pkg/jwtx tree (importing internal/testutil would create the
// pkg → internal cycle review-antipatterns §13.1 forbids).
type appleHarness struct {
	priv     *rsa.PrivateKey
	kid      string
	clock    *clockx.FakeClock
	bundleID string
	manager  *Manager
	rest     func() // tear-down hook for httptest + miniredis
}

func setupAppleHarness(t *testing.T) *appleHarness {
	t.Helper()

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	const kid = "apple-test-kid"

	body := encodeAppleJWKS(t, kid, &priv.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))

	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	clk := clockx.NewFakeClock(time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC))
	fetcher := NewAppleJWKFetcher(cli, clk, AppleJWKConfig{
		JWKSURL:      srv.URL,
		CacheKey:     "apple_jwk:cache",
		CacheTTL:     24 * time.Hour,
		FetchTimeout: 2 * time.Second,
	})

	mgr := NewManagerWithApple(setupSignerOptions(t), AppleVerifyDeps{
		Fetcher:  fetcher,
		BundleID: "com.test.cat",
		Clock:    clk,
	})

	return &appleHarness{
		priv:     priv,
		kid:      kid,
		clock:    clk,
		bundleID: "com.test.cat",
		manager:  mgr,
		rest: func() {
			srv.Close()
			_ = cli.Close()
		},
	}
}

// setupSignerOptions returns the minimum jwtx.Options required to build
// a Manager. Apple verify only needs the Apple side wired — but New()
// still wants a private key file for the sign-side, so we synthesize
// one. Re-uses writeKeyFile / setupManager helpers from manager_test.go.
func setupSignerOptions(t *testing.T) Options {
	t.Helper()
	dir := t.TempDir()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	path := writeKeyFile(t, dir, "active.pem", key)
	return Options{
		PrivateKeyPath:   path,
		ActiveKID:        "kid-active",
		Issuer:           "test-issuer",
		AccessExpirySec:  900,
		RefreshExpirySec: 2592000,
	}
}

func encodeAppleJWKS(t *testing.T, kid string, pub *rsa.PublicKey) []byte {
	t.Helper()
	type k struct {
		Kty string `json:"kty"`
		Kid string `json:"kid"`
		Use string `json:"use"`
		Alg string `json:"alg"`
		N   string `json:"n"`
		E   string `json:"e"`
	}
	doc := struct {
		Keys []k `json:"keys"`
	}{Keys: []k{{
		Kty: "RSA", Kid: kid, Use: "sig", Alg: "RS256",
		N: base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E: base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}}}
	out, err := json.Marshal(doc)
	require.NoError(t, err)
	return out
}

// signOpts mirrors the variant test cases need: every default is the
// happy path, override only the field under test.
type signOpts struct {
	sub       string
	aud       string
	iss       string
	alg       string // default RS256
	kid       string // default appleHarness.kid
	omitKid   bool
	omitExp   bool
	iat, exp  time.Time // default now / now+5m
	nonce     string
	signWith  *rsa.PrivateKey // default appleHarness.priv
	headerKid any             // override kid header type — used to drive the §3.3 non-string case
}

func (h *appleHarness) sign(t *testing.T, o signOpts) string {
	t.Helper()

	if o.alg == "" {
		o.alg = "RS256"
	}
	if o.kid == "" {
		o.kid = h.kid
	}
	if o.aud == "" {
		o.aud = h.bundleID
	}
	if o.iss == "" {
		o.iss = AppleIssuer
	}
	if o.iat.IsZero() {
		o.iat = h.clock.Now()
	}
	if o.exp.IsZero() {
		o.exp = o.iat.Add(5 * time.Minute)
	}
	if o.signWith == nil {
		o.signWith = h.priv
	}

	claims := AppleIdentityClaims{
		Nonce: o.nonce,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:  o.sub,
			Issuer:   o.iss,
			Audience: jwt.ClaimStrings{o.aud},
			IssuedAt: jwt.NewNumericDate(o.iat),
		},
	}
	if !o.omitExp {
		claims.ExpiresAt = jwt.NewNumericDate(o.exp)
	}

	var method jwt.SigningMethod
	switch o.alg {
	case "RS256":
		method = jwt.SigningMethodRS256
	case "RS384":
		method = jwt.SigningMethodRS384
	default:
		t.Fatalf("unsupported alg %q", o.alg)
	}

	tok := jwt.NewWithClaims(method, claims)
	if !o.omitKid {
		if o.headerKid != nil {
			tok.Header["kid"] = o.headerKid
		} else {
			tok.Header["kid"] = o.kid
		}
	}

	out, err := tok.SignedString(o.signWith)
	require.NoError(t, err)
	return out
}

// ---- Cases ----

func TestVerifyApple_HappyPath(t *testing.T) {
	t.Parallel()
	h := setupAppleHarness(t)
	defer h.rest()

	tok := h.sign(t, signOpts{sub: "apple:user:1", nonce: "abc123"})

	claims, err := h.manager.VerifyApple(context.Background(), tok, "abc123")
	require.NoError(t, err)
	assert.Equal(t, "apple:user:1", claims.Subject)
	assert.Equal(t, AppleIssuer, claims.Issuer)
	assert.Equal(t, "abc123", claims.Nonce)
}

func TestVerifyApple_InvalidSignature(t *testing.T) {
	t.Parallel()
	h := setupAppleHarness(t)
	defer h.rest()

	other, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	tok := h.sign(t, signOpts{sub: "apple:user:1", signWith: other})

	_, err = h.manager.VerifyApple(context.Background(), tok, "")
	require.Error(t, err)
}

func TestVerifyApple_WrongIssuer(t *testing.T) {
	t.Parallel()
	h := setupAppleHarness(t)
	defer h.rest()

	tok := h.sign(t, signOpts{sub: "apple:user:1", iss: "https://evil.example.com"})

	_, err := h.manager.VerifyApple(context.Background(), tok, "")
	require.Error(t, err)
}

func TestVerifyApple_WrongAudience(t *testing.T) {
	t.Parallel()
	h := setupAppleHarness(t)
	defer h.rest()

	tok := h.sign(t, signOpts{sub: "apple:user:1", aud: "com.attacker.app"})

	_, err := h.manager.VerifyApple(context.Background(), tok, "")
	require.Error(t, err)
}

func TestVerifyApple_ExpiredToken(t *testing.T) {
	t.Parallel()
	h := setupAppleHarness(t)
	defer h.rest()

	past := h.clock.Now().Add(-2 * time.Hour)
	tok := h.sign(t, signOpts{
		sub: "apple:user:1",
		iat: past,
		exp: past.Add(time.Hour),
	})

	_, err := h.manager.VerifyApple(context.Background(), tok, "")
	require.Error(t, err)
}

func TestVerifyApple_MissingKid(t *testing.T) {
	t.Parallel()
	h := setupAppleHarness(t)
	defer h.rest()

	tok := h.sign(t, signOpts{sub: "apple:user:1", omitKid: true})

	_, err := h.manager.VerifyApple(context.Background(), tok, "")
	require.Error(t, err)
}

func TestVerifyApple_NonStringKid(t *testing.T) {
	t.Parallel()
	h := setupAppleHarness(t)
	defer h.rest()

	// kid set to a JSON number — header decodes to float64 — must reject.
	tok := h.sign(t, signOpts{sub: "apple:user:1", headerKid: 42})

	_, err := h.manager.VerifyApple(context.Background(), tok, "")
	require.Error(t, err)
}

func TestVerifyApple_UnknownKid(t *testing.T) {
	t.Parallel()
	h := setupAppleHarness(t)
	defer h.rest()

	tok := h.sign(t, signOpts{sub: "apple:user:1", kid: "unknown-kid"})

	_, err := h.manager.VerifyApple(context.Background(), tok, "")
	require.Error(t, err)
}

func TestVerifyApple_WrongAlgorithm(t *testing.T) {
	t.Parallel()
	h := setupAppleHarness(t)
	defer h.rest()

	tok := h.sign(t, signOpts{sub: "apple:user:1", alg: "RS384"})

	_, err := h.manager.VerifyApple(context.Background(), tok, "")
	require.Error(t, err)
}

func TestVerifyApple_NonceMismatch(t *testing.T) {
	t.Parallel()
	h := setupAppleHarness(t)
	defer h.rest()

	tok := h.sign(t, signOpts{sub: "apple:user:1", nonce: "claim-nonce"})

	_, err := h.manager.VerifyApple(context.Background(), tok, "different-nonce")
	require.Error(t, err)
}

func TestVerifyApple_NoExpClaim(t *testing.T) {
	t.Parallel()
	h := setupAppleHarness(t)
	defer h.rest()

	tok := h.sign(t, signOpts{sub: "apple:user:1", omitExp: true})

	_, err := h.manager.VerifyApple(context.Background(), tok, "")
	require.Error(t, err)
}

func TestVerifyApple_MalformedToken(t *testing.T) {
	t.Parallel()
	h := setupAppleHarness(t)
	defer h.rest()

	_, err := h.manager.VerifyApple(context.Background(), "not.a.jwt", "")
	require.Error(t, err)
}

func TestVerifyApple_MultipleAudienceRejected(t *testing.T) {
	t.Parallel()
	h := setupAppleHarness(t)
	defer h.rest()

	// Mint a token with both audiences so the WithAudience contains-
	// check passes; the explicit single-audience guard inside
	// VerifyApple is the load-bearing reject (§3.5 multi-aud attack).
	claims := AppleIdentityClaims{
		Nonce: "n",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "apple:user:1",
			Issuer:    AppleIssuer,
			Audience:  jwt.ClaimStrings{"com.attacker.app", h.bundleID},
			IssuedAt:  jwt.NewNumericDate(h.clock.Now()),
			ExpiresAt: jwt.NewNumericDate(h.clock.Now().Add(time.Minute)),
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = h.kid
	out, err := tok.SignedString(h.priv)
	require.NoError(t, err)

	_, err = h.manager.VerifyApple(context.Background(), out, "n")
	require.Error(t, err)
}

func TestVerifyApple_MissingSub(t *testing.T) {
	t.Parallel()
	h := setupAppleHarness(t)
	defer h.rest()

	tok := h.sign(t, signOpts{sub: ""})
	// signOpts requires sub for happy path; sign with empty sub by
	// driving the underlying signer directly:
	claims := AppleIdentityClaims{
		Nonce: "n",
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "",
			Issuer:    AppleIssuer,
			Audience:  jwt.ClaimStrings{h.bundleID},
			IssuedAt:  jwt.NewNumericDate(h.clock.Now()),
			ExpiresAt: jwt.NewNumericDate(h.clock.Now().Add(time.Minute)),
		},
	}
	rawTok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	rawTok.Header["kid"] = h.kid
	out, err := rawTok.SignedString(h.priv)
	require.NoError(t, err)
	_ = tok

	_, err = h.manager.VerifyApple(context.Background(), out, "n")
	require.Error(t, err)
}

func TestVerifyApple_SignOnlyManagerRefuses(t *testing.T) {
	t.Parallel()

	// A Manager constructed via the Story 0.3 sign-only New(opts) path
	// MUST refuse VerifyApple — silently returning ok would be a
	// catastrophic auth bypass.
	dir := t.TempDir()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	path := writeKeyFile(t, dir, "active.pem", key)
	mgr := New(Options{
		PrivateKeyPath: path, ActiveKID: "k", Issuer: "iss",
		AccessExpirySec: 60, RefreshExpirySec: 60,
	})

	_, err = mgr.VerifyApple(context.Background(), "x.y.z", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "without AppleVerifyDeps")
}
