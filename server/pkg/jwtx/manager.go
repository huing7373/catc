package jwtx

import (
	"context"
	"crypto/rsa"
	"crypto/subtle"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"

	"github.com/huing/cat/server/pkg/clockx"
)

// AppleIssuer is the literal `iss` claim Apple identity tokens carry.
// Hard-coded — not configurable — because any deviation is a bug or
// attack, never a deployment knob.
const AppleIssuer = "https://appleid.apple.com"

// CustomClaims holds the application-specific JWT claims.
type CustomClaims struct {
	UserID    string `json:"uid"`
	DeviceID  string `json:"did"`
	Platform  string `json:"plat"`
	TokenType string `json:"ttype"`
	jwt.RegisteredClaims
}

// Options holds the parameters needed to create a JWT Manager.
type Options struct {
	PrivateKeyPath    string
	PrivateKeyPathOld string
	ActiveKID         string
	OldKID            string
	Issuer            string
	AccessExpirySec   int
	RefreshExpirySec  int
}

// Manager handles RS256 JWT signing and verification with key rotation support.
//
// The Apple-side fields (appleFetcher / appleBundleID / appleClock) are
// optional: they are nil for the Story 0.3 sign-only construction path
// (New) and are populated by NewManagerWithApple for Story 1.1+.
// VerifyApple panics if called on a sign-only Manager — see its doc.
//
// issueClock drives the IssuedAt / ExpiresAt stamps on Issue AND the
// exp-comparison clock in Verify (jwt.WithTimeFunc). Kept as one field
// because the two calls MUST share a timebase — otherwise a FakeClock
// test that pins Issue's "now" would still be at the real wall clock's
// mercy in Verify, and any token with a non-trivial expiry would start
// failing Verify as soon as real-now outran fake-now. New(opts)
// defaults to RealClock (time.Now-backed); NewManagerWithApple
// overrides with the injected AppleVerifyDeps.Clock so the same clock
// drives Apple-token Verify (appleClock) and server-token Issue/Verify
// (issueClock) — production code path must go through
// NewManagerWithApple, per review-antipatterns §4.1.
type Manager struct {
	activeKey     *rsa.PrivateKey
	activePub     *rsa.PublicKey
	activeKID     string
	oldPub        *rsa.PublicKey
	oldKID        string
	issuer        string
	accessExpiry  time.Duration
	refreshExpiry time.Duration
	issueClock    clockx.Clock

	appleFetcher  *AppleJWKFetcher
	appleBundleID string
	appleClock    clockx.Clock
}

// AppleVerifyDeps wires the Apple identity-token verification path. All
// three fields are required: NewManagerWithApple log.Fatals if any is
// nil / empty. Kept separate from Options so that Epic 0 sign-only
// construction (New) keeps compiling without an Apple-shaped detour.
type AppleVerifyDeps struct {
	Fetcher  *AppleJWKFetcher
	BundleID string // expected `aud` of every accepted Apple identity token
	Clock    clockx.Clock
}

// New creates a Manager from options. Reads RSA PEM key files.
// Calls log.Fatal if required fields are missing or keys cannot be loaded.
func New(opts Options) *Manager {
	if opts.ActiveKID == "" {
		log.Fatal().Msg("jwt: active_kid must not be empty")
	}
	if opts.Issuer == "" {
		log.Fatal().Msg("jwt: issuer must not be empty")
	}
	if opts.AccessExpirySec <= 0 {
		log.Fatal().Int("access_expiry_sec", opts.AccessExpirySec).Msg("jwt: access_expiry_sec must be positive")
	}
	if opts.RefreshExpirySec <= 0 {
		log.Fatal().Int("refresh_expiry_sec", opts.RefreshExpirySec).Msg("jwt: refresh_expiry_sec must be positive")
	}

	activeKey := mustLoadPrivateKey(opts.PrivateKeyPath)

	m := &Manager{
		activeKey:     activeKey,
		activePub:     &activeKey.PublicKey,
		activeKID:     opts.ActiveKID,
		issuer:        opts.Issuer,
		accessExpiry:  time.Duration(opts.AccessExpirySec) * time.Second,
		refreshExpiry: time.Duration(opts.RefreshExpirySec) * time.Second,
		issueClock:    clockx.NewRealClock(),
	}

	if opts.PrivateKeyPathOld != "" {
		if opts.OldKID == "" {
			log.Fatal().Msg("jwt: old_kid must not be empty when private_key_path_old is set")
		}
		oldKey := mustLoadPrivateKey(opts.PrivateKeyPathOld)
		m.oldPub = &oldKey.PublicKey
		m.oldKID = opts.OldKID
	}

	return m
}

// Issue signs a JWT with the active key. The kid header is set for key
// selection during verification.
//
// Issue overwrites Issuer / IssuedAt / ExpiresAt but deliberately does
// NOT touch the caller-supplied RegisteredClaims.ID (jti) or Subject —
// that contract is load-bearing for Story 1.2 rolling-rotation (a
// refresh token whose jti got stomped silently would make reuse
// detection fail open). Tests
// TestManager_Issue_PreservesRegisteredClaimsID and
// TestManager_Issue_EmptyJTIStaysEmpty lock the contract.
//
// Timebase: m.issueClock — RealClock when built via New(opts), the
// injected AppleVerifyDeps.Clock when built via NewManagerWithApple.
// Production paths MUST go through NewManagerWithApple so that
// FakeClock tests can drive Issue deterministically (antipattern §4.1).
func (m *Manager) Issue(claims CustomClaims) (string, error) {
	now := m.issueClock.Now()

	expiry := m.accessExpiry
	if claims.TokenType == "refresh" {
		expiry = m.refreshExpiry
	}

	claims.Issuer = m.issuer
	claims.IssuedAt = jwt.NewNumericDate(now)
	claims.ExpiresAt = jwt.NewNumericDate(now.Add(expiry))

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = m.activeKID

	return token.SignedString(m.activeKey)
}

// Verify parses and validates a JWT. It selects the public key by
// matching the kid header. Expiration validation runs through
// m.issueClock so Issue / Verify share a single timebase — a FakeClock
// test harness that pins "now" for Issue MUST observe the same pinned
// "now" during Verify, otherwise a token issued at fake-now with a
// 15-minute access expiry would fail Verify the moment the real wall
// clock passes fake-now+15min (round-2 review finding).
//
// kid extraction defense-in-depth (review-antipatterns §3.3): missing
// header / non-string header / empty string are each rejected with a
// distinct error before the keyfunc compares against activeKID. The
// production Issue path always sets a valid kid (line 157), so this
// only triggers on forged or otherwise malformed tokens; aligning the
// branch shape with VerifyApple (line 287-295) keeps the two code
// paths from drifting and stops a future refactor of the kid==X check
// from re-collapsing the missing/non-string cases into "unknown kid: ".
func (m *Manager) Verify(tokenStr string) (*CustomClaims, error) {
	token, err := jwt.ParseWithClaims(tokenStr, &CustomClaims{}, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodRS256 {
			return nil, errors.New("unexpected signing method: " + t.Method.Alg())
		}

		kidRaw, present := t.Header["kid"]
		if !present {
			return nil, errors.New("jwtx.Verify: missing kid header")
		}
		kid, ok := kidRaw.(string)
		if !ok || kid == "" {
			return nil, errors.New("jwtx.Verify: kid header must be a non-empty string")
		}
		if kid == m.activeKID {
			return m.activePub, nil
		}
		if m.oldPub != nil && kid == m.oldKID {
			return m.oldPub, nil
		}
		return nil, fmt.Errorf("jwtx.Verify: unknown kid %q", kid)
	}, jwt.WithIssuer(m.issuer), jwt.WithExpirationRequired(), jwt.WithTimeFunc(m.issueClock.Now))
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*CustomClaims)
	if !ok || !token.Valid {
		return nil, errors.New("invalid token claims")
	}
	return claims, nil
}

// NewManagerWithApple is the Story 1.1+ construction path: it returns the
// same Manager but with the Apple identity-token verification path wired
// up. The Story 0.3 sign-only New(opts) entry point is preserved for the
// Epic 0 tests that build a Manager without the Apple JWK fetcher.
//
// All AppleVerifyDeps fields are required and validated fail-fast at
// startup — see review-antipatterns §4.1: a sign-only Manager that
// silently accepts VerifyApple calls and returns "ok" would be a
// catastrophic auth bypass.
func NewManagerWithApple(opts Options, apple AppleVerifyDeps) *Manager {
	if apple.Fetcher == nil {
		log.Fatal().Msg("jwtx.NewManagerWithApple: AppleVerifyDeps.Fetcher must not be nil")
	}
	if apple.BundleID == "" {
		log.Fatal().Msg("jwtx.NewManagerWithApple: AppleVerifyDeps.BundleID must not be empty")
	}
	if apple.Clock == nil {
		log.Fatal().Msg("jwtx.NewManagerWithApple: AppleVerifyDeps.Clock must not be nil")
	}
	m := New(opts)
	m.appleFetcher = apple.Fetcher
	m.appleBundleID = apple.BundleID
	m.appleClock = apple.Clock
	m.issueClock = apple.Clock
	return m
}

// AppleIdentityClaims is the subset of Apple identity-token claims the
// server trusts after VerifyApple. `Subject` is Apple's opaque per-team
// user id (callers SHOULD hash before storing — NFR-SEC-6). `Nonce` is
// the SHA-256 hex of the raw nonce the client sent to Apple, echoed
// back in the identity token; the server compares it against
// SHA-256(request.Nonce) to bind this token to the current request and
// prevent replay.
//
// Email / EmailVerified are decoded for completeness but the MVP does
// not consume them — Apple may omit Email after first sign-in, and
// keeping it out of business code minimizes the PII the server handles
// (NFR-COMP-1).
type AppleIdentityClaims struct {
	Nonce         string `json:"nonce,omitempty"`
	Email         string `json:"email,omitempty"`
	EmailVerified string `json:"email_verified,omitempty"` // string-typed "true"/"false" per Apple SIWA spec
	jwt.RegisteredClaims
}

// VerifyApple parses and verifies an Apple identity token end-to-end.
// Implements every guard from review-antipatterns §3.1-§3.5 in source —
// no JWT-library default is trusted to enforce one of them silently:
//
//   - §3.1 issuer: explicit equality against AppleIssuer constant.
//   - §3.2 algorithm: header MUST be RS256 (RS384 / HS256 / "none"
//     refused at the keyfunc gate).
//   - §3.3 kid: header MUST be a non-empty string; missing kid → reject
//     before the JWKS lookup so a single bad token cannot cost an
//     Apple HTTP fetch.
//   - §3.4 expiration: jwt.WithExpirationRequired() refuses tokens
//     without an exp claim, and jwt.WithTimeFunc(clock.Now) lets the
//     test harness drive the wall clock with FakeClock.
//   - §3.5 audience: jwt.WithAudience covers the contains-check, and
//     an additional explicit `len(Audience)==1 && Audience[0]==BundleID`
//     guard catches the multi-audience attack where a legitimate token
//     is repackaged with the victim's BundleID alongside an attacker
//     audience.
//
// expectedNonceSHA256 is the server-side hex(sha256(request.nonce)). An
// empty value means "skip nonce check" — the production handler MUST
// always pass a non-empty value; only the test harness may pass "" to
// exercise the code path. Comparison uses crypto/subtle.ConstantTime
// to defeat timing oracles.
func (m *Manager) VerifyApple(ctx context.Context, idToken string, expectedNonceSHA256 string) (*AppleIdentityClaims, error) {
	if m.appleFetcher == nil || m.appleBundleID == "" || m.appleClock == nil {
		return nil, errors.New("jwtx.VerifyApple: Manager constructed without AppleVerifyDeps")
	}

	var claims AppleIdentityClaims
	parser := jwt.NewParser(
		jwt.WithExpirationRequired(),
		jwt.WithIssuer(AppleIssuer),
		jwt.WithAudience(m.appleBundleID),
		jwt.WithTimeFunc(m.appleClock.Now),
	)
	token, err := parser.ParseWithClaims(idToken, &claims, func(t *jwt.Token) (any, error) {
		// §3.2 — RS256 ONLY. The type assertion alone is not enough;
		// jwt.SigningMethodRS384 also satisfies *SigningMethodRSA, so
		// we must check the alg name explicitly.
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, errors.New("apple: signing method must be RSA")
		}
		if t.Method.Alg() != "RS256" {
			return nil, errors.New("apple: alg must be RS256")
		}
		// §3.3 — kid header required and non-empty.
		kidRaw, present := t.Header["kid"]
		if !present {
			return nil, errors.New("apple: missing kid header")
		}
		kid, ok := kidRaw.(string)
		if !ok || kid == "" {
			return nil, errors.New("apple: kid header must be a non-empty string")
		}
		set, fetchErr := m.appleFetcher.Get(ctx)
		if fetchErr != nil {
			return nil, fmt.Errorf("apple: fetch jwks: %w", fetchErr)
		}
		pub, ok := set.Keys[kid]
		if !ok {
			return nil, fmt.Errorf("apple: unknown kid %q", kid)
		}
		return pub, nil
	})
	if err != nil {
		return nil, err
	}
	if !token.Valid {
		return nil, errors.New("apple: token marked invalid by parser")
	}

	// §3.5 — explicit single-audience equality. jwt.WithAudience above
	// is a contains-check; the `len==1 && [0]==bundleID` guard is the
	// load-bearing one against the multi-aud attack.
	if len(claims.Audience) != 1 || claims.Audience[0] != m.appleBundleID {
		return nil, fmt.Errorf("apple: audience mismatch (expected single %q)", m.appleBundleID)
	}
	// §3.1 — explicit issuer equality (defense in depth behind WithIssuer).
	if claims.Issuer != AppleIssuer {
		return nil, fmt.Errorf("apple: issuer mismatch (got %q)", claims.Issuer)
	}
	// Subject must be present — anchor for the per-user hash downstream.
	if claims.Subject == "" {
		return nil, errors.New("apple: missing sub claim")
	}

	// Nonce — constant-time compare to defeat timing oracles. Apple
	// SIWA spec: the `nonce` claim is hex(sha256(client_raw_nonce)).
	if expectedNonceSHA256 != "" {
		if subtle.ConstantTimeCompare([]byte(claims.Nonce), []byte(expectedNonceSHA256)) != 1 {
			return nil, errors.New("apple: nonce mismatch")
		}
	}

	return &claims, nil
}

// AccessExpiry returns the configured access token duration.
func (m *Manager) AccessExpiry() time.Duration { return m.accessExpiry }

// RefreshExpiry returns the configured refresh token duration.
func (m *Manager) RefreshExpiry() time.Duration { return m.refreshExpiry }

func mustLoadPrivateKey(path string) *rsa.PrivateKey {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal().Err(err).Str("path", path).Msg("jwt: read private key failed")
	}

	block, _ := pem.Decode(data)
	if block == nil {
		log.Fatal().Str("path", path).Msg("jwt: no PEM block found")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		key, err = x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			log.Fatal().Err(err).Str("path", path).Msg("jwt: parse private key failed")
		}
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		log.Fatal().Str("path", path).Msg("jwt: key is not RSA")
	}
	return rsaKey
}
