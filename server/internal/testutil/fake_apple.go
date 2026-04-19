package testutil

import (
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
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/jwtx"
)

// FakeApple bundles every moving part needed to exercise Apple Sign in
// with Apple end-to-end without touching the real appleid.apple.com:
//
//   - Priv: an RSA 2048 key whose public half is served as Apple's JWKS.
//   - KID: the kid header that goes into every signed identity token
//     and the matching JWKS entry.
//   - JWKSServer: a httptest server that answers GET requests with the
//     JWKS document.
//   - Fetcher: an AppleJWKFetcher pointed at JWKSServer.URL (caller
//     stores it on a Manager via NewManagerWithApple).
//   - Clock: a FakeClock so tests control exp / iat math precisely.
//   - BundleID: the audience that signed tokens must carry; same value
//     must be plumbed into AppleVerifyDeps.BundleID.
//
// Caller calls Close() (or registers t.Cleanup) to tear down the
// httptest server and the in-process miniredis.
type FakeApple struct {
	Priv       *rsa.PrivateKey
	KID        string
	JWKSServer *httptest.Server
	Fetcher    *jwtx.AppleJWKFetcher
	Redis      *redis.Client
	Miniredis  *miniredis.Miniredis
	Clock      *clockx.FakeClock
	BundleID   string
}

// NewFakeApple stands up the whole fake stack rooted at the supplied
// bundle ID. Defaults: clock pinned to 2026-04-19T12:00:00Z, kid =
// "apple-test-kid". The httptest server returns one RS256 key matching
// Priv. Every dependency that bare integration tests reach for (a
// Redis cache, an Apple JWKS endpoint, a wall clock) is contained
// here so the suite stays self-contained per architecture §21.7.
func NewFakeApple(t *testing.T, bundleID string) *FakeApple {
	t.Helper()
	if bundleID == "" {
		bundleID = "com.test.cat"
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	kid := "apple-test-kid"

	// JWKS handler — deliberately uses ServeHTTP so per-test mutations
	// (e.g. swapping the served body) can be added later without
	// re-architecting.
	body := encodeJWKS(t, kid, &priv.PublicKey)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = cli.Close() })

	clk := clockx.NewFakeClock(time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC))

	fetcher := jwtx.NewAppleJWKFetcher(cli, clk, jwtx.AppleJWKConfig{
		JWKSURL:      srv.URL,
		CacheKey:     "apple_jwk:cache",
		CacheTTL:     24 * time.Hour,
		FetchTimeout: 2 * time.Second,
	})

	return &FakeApple{
		Priv:       priv,
		KID:        kid,
		JWKSServer: srv,
		Fetcher:    fetcher,
		Redis:      cli,
		Miniredis:  mr,
		Clock:      clk,
		BundleID:   bundleID,
	}
}

// SignIdentityToken mints an Apple identity token using the FakeApple
// private key. Pass overrides via the SignOptions struct to drive
// negative-path tests (wrong issuer, expired, missing kid, …) without
// reimplementing the encoder per case.
type SignOptions struct {
	Sub           string
	Aud           string    // "" → fa.BundleID
	Iss           string    // "" → AppleIssuer
	Nonce         string    // SHA-256 hex of raw nonce
	IssuedAt      time.Time // zero → fa.Clock.Now()
	ExpiresAt     time.Time // zero → IssuedAt + 5min
	Alg           string    // "" → "RS256"; set to "RS384" / "" to drive §3.2
	Kid           string    // "" → fa.KID; set "" + OmitKid=true to drop entirely
	OmitKid       bool
	OmitExp       bool
	Email         string
	EmailVerified string
}

// SignIdentityToken signs an Apple identity token using the FakeApple
// keypair. SignOptions.Sub is required; everything else falls back to
// FakeApple defaults so the happy path is one short call.
func (fa *FakeApple) SignIdentityToken(t *testing.T, opts SignOptions) string {
	t.Helper()
	require.NotEmpty(t, opts.Sub, "SignOptions.Sub is required")

	now := opts.IssuedAt
	if now.IsZero() {
		now = fa.Clock.Now()
	}
	exp := opts.ExpiresAt
	if exp.IsZero() {
		exp = now.Add(5 * time.Minute)
	}
	aud := opts.Aud
	if aud == "" {
		aud = fa.BundleID
	}
	iss := opts.Iss
	if iss == "" {
		iss = jwtx.AppleIssuer
	}
	alg := opts.Alg
	if alg == "" {
		alg = "RS256"
	}
	kid := opts.Kid
	if kid == "" {
		kid = fa.KID
	}

	claims := jwtx.AppleIdentityClaims{
		Nonce:         opts.Nonce,
		Email:         opts.Email,
		EmailVerified: opts.EmailVerified,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:  opts.Sub,
			Issuer:   iss,
			Audience: jwt.ClaimStrings{aud},
			IssuedAt: jwt.NewNumericDate(now),
		},
	}
	if !opts.OmitExp {
		claims.ExpiresAt = jwt.NewNumericDate(exp)
	}

	var method jwt.SigningMethod
	switch alg {
	case "RS256":
		method = jwt.SigningMethodRS256
	case "RS384":
		method = jwt.SigningMethodRS384
	case "RS512":
		method = jwt.SigningMethodRS512
	default:
		t.Fatalf("FakeApple.SignIdentityToken: unsupported alg %q", alg)
	}

	tok := jwt.NewWithClaims(method, claims)
	if !opts.OmitKid {
		tok.Header["kid"] = kid
	}

	signed, err := tok.SignedString(fa.Priv)
	require.NoError(t, err)
	return signed
}

// encodeJWKS produces Apple's JWKS JSON shape with one RS256 / RSA key.
func encodeJWKS(t *testing.T, kid string, pub *rsa.PublicKey) []byte {
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
	}{
		Keys: []k{{
			Kty: "RSA",
			Kid: kid,
			Use: "sig",
			Alg: "RS256",
			N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
		}},
	}
	out, err := json.Marshal(doc)
	require.NoError(t, err)
	return out
}
