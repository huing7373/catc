package applex

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// --- test fixtures ---

type fixture struct {
	priv     *rsa.PrivateKey
	kid      string
	jwksJSON []byte
	srv      *httptest.Server
	hits     *int32
	mode     *atomic.Value // string: "ok" | "500"
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa keygen: %v", err)
	}
	kid := "test-kid-1"

	// Build JWKS payload manually so we don't depend on a JWKS lib.
	js := jwkSet{Keys: []jwk{{
		Kty: "RSA",
		Kid: kid,
		Alg: "RS256",
		N:   base64.RawURLEncoding.EncodeToString(priv.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(priv.E)).Bytes()),
	}}}
	body, err := json.Marshal(js)
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}

	hits := int32(0)
	mode := &atomic.Value{}
	mode.Store("ok")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		if mode.Load().(string) == "500" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}))
	t.Cleanup(srv.Close)

	return &fixture{
		priv:     priv,
		kid:      kid,
		jwksJSON: body,
		srv:      srv,
		hits:     &hits,
		mode:     mode,
	}
}

type tokenOpts struct {
	iss     string
	aud     any // string or []string
	sub     string
	nonce   string // already hex-sha256 form (matches Apple)
	expDur  time.Duration
	iatSkew time.Duration // added to iat
	kid     string        // override
	signKey *rsa.PrivateKey
	alg     jwt.SigningMethod
}

func (f *fixture) signToken(t *testing.T, o tokenOpts) string {
	t.Helper()
	if o.iss == "" {
		o.iss = DefaultIssuer
	}
	if o.sub == "" {
		o.sub = "001234.abcdef.0001"
	}
	if o.expDur == 0 {
		o.expDur = time.Hour
	}
	if o.alg == nil {
		o.alg = jwt.SigningMethodRS256
	}
	signKey := o.signKey
	if signKey == nil {
		signKey = f.priv
	}
	kid := o.kid
	if kid == "" {
		kid = f.kid
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"iss":              o.iss,
		"sub":              o.sub,
		"exp":              now.Add(o.expDur).Unix(),
		"iat":              now.Add(o.iatSkew).Unix(),
		"email":            "test@example.com",
		"email_verified":   "true",
		"is_private_email": "false",
	}
	if o.aud != nil {
		claims["aud"] = o.aud
	}
	if o.nonce != "" {
		claims["nonce"] = o.nonce
	}
	tok := jwt.NewWithClaims(o.alg, claims)
	tok.Header["kid"] = kid
	signed, err := tok.SignedString(signKey)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return signed
}

// hashedNonce mirrors Apple: client SHA256s the raw nonce and gives the
// hex form to the SIWA button. Tokens carry that hex.
func hashedNonce(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}

func newVerifierFromFixture(f *fixture, audiences []string) *Verifier {
	return New(Config{
		JWKSURL:          f.srv.URL,
		AllowedAudiences: audiences,
		HTTPClient:       f.srv.Client(),
		JWKSCacheTTL:     500 * time.Millisecond,
	})
}

// --- happy path ---

func TestVerify_Happy(t *testing.T) {
	f := newFixture(t)
	v := newVerifierFromFixture(f, []string{"com.example.cat"})

	raw := "raw-nonce-12345678"
	tok := f.signToken(t, tokenOpts{aud: "com.example.cat", nonce: hashedNonce(raw)})

	id, err := v.Verify(context.Background(), tok, raw)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if id.Sub != "001234.abcdef.0001" {
		t.Errorf("sub: got %q", id.Sub)
	}
	if !id.EmailVerified {
		t.Errorf("email_verified should be true")
	}
	if id.IsPrivateEmail {
		t.Errorf("is_private_email should be false")
	}
	if id.Email != "test@example.com" {
		t.Errorf("email: got %q", id.Email)
	}
}

// --- failure branches ---

func TestVerify_BadSignature(t *testing.T) {
	f := newFixture(t)
	v := newVerifierFromFixture(f, []string{"com.example.cat"})

	other, _ := rsa.GenerateKey(rand.Reader, 2048)
	tok := f.signToken(t, tokenOpts{aud: "com.example.cat", signKey: other})

	_, err := v.Verify(context.Background(), tok, "")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
}

func TestVerify_Expired(t *testing.T) {
	f := newFixture(t)
	v := newVerifierFromFixture(f, []string{"com.example.cat"})

	tok := f.signToken(t, tokenOpts{aud: "com.example.cat", expDur: -time.Minute})
	_, err := v.Verify(context.Background(), tok, "")
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("want ErrExpiredToken, got %v", err)
	}
}

func TestVerify_WrongIssuer(t *testing.T) {
	f := newFixture(t)
	v := newVerifierFromFixture(f, []string{"com.example.cat"})

	tok := f.signToken(t, tokenOpts{iss: "https://evil.example", aud: "com.example.cat"})
	_, err := v.Verify(context.Background(), tok, "")
	if !errors.Is(err, ErrIssuerMismatch) {
		t.Fatalf("want ErrIssuerMismatch, got %v", err)
	}
}

func TestVerify_WrongAudience(t *testing.T) {
	f := newFixture(t)
	v := newVerifierFromFixture(f, []string{"com.example.cat"})

	tok := f.signToken(t, tokenOpts{aud: "com.someoneelse.cat"})
	_, err := v.Verify(context.Background(), tok, "")
	if !errors.Is(err, ErrAudienceMismatch) {
		t.Fatalf("want ErrAudienceMismatch, got %v", err)
	}
}

func TestVerify_NonceMismatch(t *testing.T) {
	f := newFixture(t)
	v := newVerifierFromFixture(f, []string{"com.example.cat"})

	tok := f.signToken(t, tokenOpts{aud: "com.example.cat", nonce: hashedNonce("server-side-raw")})
	_, err := v.Verify(context.Background(), tok, "client-different-raw")
	if !errors.Is(err, ErrNonceMismatch) {
		t.Fatalf("want ErrNonceMismatch, got %v", err)
	}
}

func TestVerify_KidNotInJWKS(t *testing.T) {
	f := newFixture(t)
	v := newVerifierFromFixture(f, []string{"com.example.cat"})

	tok := f.signToken(t, tokenOpts{aud: "com.example.cat", kid: "unknown-kid"})
	_, err := v.Verify(context.Background(), tok, "")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
}

func TestVerify_AudienceAsArray(t *testing.T) {
	f := newFixture(t)
	v := newVerifierFromFixture(f, []string{"com.example.cat"})

	tok := f.signToken(t, tokenOpts{aud: []string{"com.example.cat", "extra"}})
	if _, err := v.Verify(context.Background(), tok, ""); err != nil {
		t.Fatalf("array aud should match: %v", err)
	}
}

func TestVerify_MissingIATRejected(t *testing.T) {
	f := newFixture(t)
	v := newVerifierFromFixture(f, []string{"com.example.cat"})

	// Hand-craft a token without iat.
	priv := f.priv
	claims := jwt.MapClaims{
		"iss": DefaultIssuer,
		"aud": "com.example.cat",
		"sub": "x",
		"exp": time.Now().Add(time.Hour).Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = f.kid
	signed, err := tok.SignedString(priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}

	_, err = v.Verify(context.Background(), signed, "")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken (missing iat), got %v", err)
	}
}

func TestVerify_IATFutureRejected(t *testing.T) {
	f := newFixture(t)
	v := newVerifierFromFixture(f, []string{"com.example.cat"})

	// 10 minutes in the future > 5min tolerance.
	tok := f.signToken(t, tokenOpts{aud: "com.example.cat", iatSkew: 10 * time.Minute})
	_, err := v.Verify(context.Background(), tok, "")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken (iat future), got %v", err)
	}
}

func TestVerify_HMACAlgRejected(t *testing.T) {
	f := newFixture(t)
	v := newVerifierFromFixture(f, []string{"com.example.cat"})

	// Hand-craft an HS256 token with the same kid: the keyfunc must
	// reject the alg before the signature check.
	claims := jwt.MapClaims{
		"iss": DefaultIssuer,
		"aud": "com.example.cat",
		"sub": "x",
		"exp": time.Now().Add(time.Hour).Unix(),
		"iat": time.Now().Unix(),
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tok.Header["kid"] = f.kid
	signed, _ := tok.SignedString([]byte("secret"))

	_, err := v.Verify(context.Background(), signed, "")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("want ErrInvalidToken, got %v", err)
	}
}

// --- JWKS cache behaviour ---

func TestJWKS_FirstFetchFailureSurfaces(t *testing.T) {
	f := newFixture(t)
	f.mode.Store("500")
	v := newVerifierFromFixture(f, []string{"com.example.cat"})

	tok := f.signToken(t, tokenOpts{aud: "com.example.cat"})
	_, err := v.Verify(context.Background(), tok, "")
	if !errors.Is(err, ErrJWKSFetchFailed) {
		t.Fatalf("want ErrJWKSFetchFailed, got %v", err)
	}
}

func TestJWKS_CacheHit(t *testing.T) {
	f := newFixture(t)
	v := newVerifierFromFixture(f, []string{"com.example.cat"})

	tok := f.signToken(t, tokenOpts{aud: "com.example.cat"})
	if _, err := v.Verify(context.Background(), tok, ""); err != nil {
		t.Fatalf("first verify: %v", err)
	}
	hits := atomic.LoadInt32(f.hits)
	if hits != 1 {
		t.Fatalf("expected 1 jwks hit, got %d", hits)
	}
	// Second call must reuse cache.
	if _, err := v.Verify(context.Background(), tok, ""); err != nil {
		t.Fatalf("second verify: %v", err)
	}
	if got := atomic.LoadInt32(f.hits); got != 1 {
		t.Fatalf("expected cached (still 1), got %d", got)
	}
}

func TestJWKS_StaleOnError_FallsBackToCache(t *testing.T) {
	f := newFixture(t)
	v := newVerifierFromFixture(f, []string{"com.example.cat"})

	// Populate cache with one good fetch.
	tok := f.signToken(t, tokenOpts{aud: "com.example.cat"})
	if _, err := v.Verify(context.Background(), tok, ""); err != nil {
		t.Fatalf("seed verify: %v", err)
	}

	// Force JWKS to fail, expire cache, and verify again — fallback
	// path must succeed.
	f.mode.Store("500")
	time.Sleep(600 * time.Millisecond) // > 500ms TTL

	if _, err := v.Verify(context.Background(), tok, ""); err != nil {
		t.Fatalf("fallback verify: %v", err)
	}
	if got := atomic.LoadInt32(f.hits); got < 2 {
		t.Fatalf("expected refresh attempt, hits=%d", got)
	}
}

func TestJWKS_RefreshAfterTTL(t *testing.T) {
	f := newFixture(t)
	v := newVerifierFromFixture(f, []string{"com.example.cat"})

	tok := f.signToken(t, tokenOpts{aud: "com.example.cat"})
	if _, err := v.Verify(context.Background(), tok, ""); err != nil {
		t.Fatalf("first: %v", err)
	}
	time.Sleep(600 * time.Millisecond)
	if _, err := v.Verify(context.Background(), tok, ""); err != nil {
		t.Fatalf("second: %v", err)
	}
	if got := atomic.LoadInt32(f.hits); got != 2 {
		t.Fatalf("expected 2 fetches after TTL, got %d", got)
	}
}

// --- defaults ---

func TestNew_AppliesDefaults(t *testing.T) {
	v := New(Config{AllowedAudiences: []string{"a"}})
	if v.cfg.JWKSURL != DefaultJWKSURL {
		t.Errorf("JWKSURL default: %q", v.cfg.JWKSURL)
	}
	if v.cfg.JWKSCacheTTL != DefaultJWKSCacheTTL {
		t.Errorf("TTL default: %v", v.cfg.JWKSCacheTTL)
	}
	if v.cfg.Issuer != DefaultIssuer {
		t.Errorf("Issuer default: %q", v.cfg.Issuer)
	}
	if v.cfg.Now == nil {
		t.Errorf("Now default missing")
	}
	if v.httpc == nil {
		t.Errorf("HTTPClient default missing")
	}
}
