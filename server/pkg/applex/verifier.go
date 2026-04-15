// Package applex verifies Apple identity tokens (Sign in with Apple).
//
// The verifier fetches Apple's RSA public-key set (JWKS) lazily and
// caches it for cfg.JWKSCacheTTL. On a refresh failure with a populated
// cache we fall back to the previous keys and emit a warn log; only the
// FIRST fetch failure surfaces ErrJWKSFetchFailed.
//
// All authentication failures map to typed sentinel errors so the
// service layer can branch with errors.Is.
package applex

import (
	"context"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/rs/zerolog/log"
)

// Defaults applied when Config fields are zero.
const (
	// DefaultIssuer is the canonical Apple identity issuer claim.
	DefaultIssuer = "https://appleid.apple.com"
	// DefaultJWKSURL is Apple's public JWKS endpoint.
	DefaultJWKSURL = "https://appleid.apple.com/auth/keys"
	// DefaultJWKSCacheTTL bounds how long a successful JWKS response is
	// trusted before a refresh is attempted.
	DefaultJWKSCacheTTL = 60 * time.Minute
	// iatToleranceFuture allows minor clock skew — Apple-issued tokens
	// occasionally land with iat slightly in the future.
	iatToleranceFuture = 5 * time.Minute
)

// Sentinel errors. Wrap with fmt.Errorf("%w: ...") to add context while
// remaining errors.Is-comparable.
var (
	// ErrInvalidToken signals a structurally bad token: wrong signature,
	// missing kid, malformed claims, etc.
	ErrInvalidToken = errors.New("applex: invalid token")
	// ErrExpiredToken signals a well-formed but past-exp token.
	ErrExpiredToken = errors.New("applex: token expired")
	// ErrAudienceMismatch signals the aud claim is not in the configured
	// allowed-audiences list.
	ErrAudienceMismatch = errors.New("applex: audience mismatch")
	// ErrIssuerMismatch signals the iss claim is not Apple.
	ErrIssuerMismatch = errors.New("applex: issuer mismatch")
	// ErrNonceMismatch signals sha256(rawNonce) hex != token.nonce.
	ErrNonceMismatch = errors.New("applex: nonce mismatch")
	// ErrJWKSFetchFailed signals the FIRST JWKS fetch failed and no
	// cached key set is available to fall back to.
	ErrJWKSFetchFailed = errors.New("applex: jwks fetch failed")
)

// Identity is the value object returned after a successful Verify.
type Identity struct {
	Sub            string // stable Apple user id
	Email          string // optional, may be empty
	EmailVerified  bool
	IsPrivateEmail bool
}

// Config wires verifier behaviour. JWKSURL, JWKSCacheTTL, Issuer,
// HTTPClient, and Now are optional and fall back to package defaults.
// AllowedAudiences must contain at least one entry.
type Config struct {
	JWKSURL          string
	JWKSCacheTTL     time.Duration
	AllowedAudiences []string
	Issuer           string // override the default Apple issuer
	HTTPClient       *http.Client
	Now              func() time.Time
}

// Verifier holds JWKS cache state and validates Apple identity tokens.
type Verifier struct {
	cfg   Config
	httpc *http.Client

	mu         sync.Mutex
	cachedAt   time.Time
	cachedKeys map[string]*rsa.PublicKey // kid → public key
}

// New constructs a Verifier. Empty AllowedAudiences is accepted at
// construction time so the caller (initialize) can fail-fatal with a
// clearer message; Verify itself will reject every token if the slice
// stays empty.
func New(cfg Config) *Verifier {
	if cfg.JWKSURL == "" {
		cfg.JWKSURL = DefaultJWKSURL
	}
	if cfg.JWKSCacheTTL <= 0 {
		cfg.JWKSCacheTTL = DefaultJWKSCacheTTL
	}
	if cfg.Issuer == "" {
		cfg.Issuer = DefaultIssuer
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	httpc := cfg.HTTPClient
	if httpc == nil {
		httpc = &http.Client{Timeout: 5 * time.Second}
	}
	return &Verifier{cfg: cfg, httpc: httpc}
}

// Verify validates an Apple identity JWT. When rawNonce is non-empty
// the token's nonce claim must equal sha256(rawNonce) hex.
//
// The order of checks matches AC #1: fetch key → RS256 sig → exp → iat
// future-skew → nbf → iss → aud → nonce. exp is checked before
// signature-related sentinels to keep "expired" first-class.
func (v *Verifier) Verify(ctx context.Context, idToken, rawNonce string) (*Identity, error) {
	// jwt.Parse with MapClaims and our keyfunc.
	parsed, err := jwt.Parse(idToken, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("%w: unexpected alg %v", ErrInvalidToken, t.Header["alg"])
		}
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, fmt.Errorf("%w: missing kid", ErrInvalidToken)
		}
		return v.lookupKey(ctx, kid)
	}, jwt.WithoutClaimsValidation()) // we run our own checks below
	if err != nil {
		switch {
		case errors.Is(err, ErrJWKSFetchFailed):
			return nil, ErrJWKSFetchFailed
		case errors.Is(err, ErrInvalidToken):
			return nil, err
		case errors.Is(err, jwt.ErrTokenExpired):
			return nil, ErrExpiredToken
		default:
			return nil, fmt.Errorf("%w: %v", ErrInvalidToken, err)
		}
	}
	if !parsed.Valid {
		return nil, ErrInvalidToken
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("%w: claims type %T", ErrInvalidToken, parsed.Claims)
	}

	now := v.cfg.Now()
	if exp, ok := numericClaim(claims, "exp"); ok {
		if !now.Before(time.Unix(exp, 0)) {
			return nil, ErrExpiredToken
		}
	} else {
		return nil, fmt.Errorf("%w: missing exp", ErrInvalidToken)
	}
	// iat is mandatory per OIDC and Apple always sets it; treat absence
	// as malformed rather than silently skipping the future-skew check.
	iat, ok := numericClaim(claims, "iat")
	if !ok {
		return nil, fmt.Errorf("%w: missing iat", ErrInvalidToken)
	}
	if time.Unix(iat, 0).After(now.Add(iatToleranceFuture)) {
		return nil, fmt.Errorf("%w: iat too far in future", ErrInvalidToken)
	}
	if nbf, ok := numericClaim(claims, "nbf"); ok {
		if now.Before(time.Unix(nbf, 0)) {
			return nil, fmt.Errorf("%w: not yet valid", ErrInvalidToken)
		}
	}

	if iss, _ := claims["iss"].(string); iss != v.cfg.Issuer {
		return nil, ErrIssuerMismatch
	}
	if !audMatches(claims["aud"], v.cfg.AllowedAudiences) {
		return nil, ErrAudienceMismatch
	}
	if rawNonce != "" {
		tokNonce, _ := claims["nonce"].(string)
		sum := sha256.Sum256([]byte(rawNonce))
		if hex.EncodeToString(sum[:]) != tokNonce {
			return nil, ErrNonceMismatch
		}
	}

	sub, _ := claims["sub"].(string)
	if sub == "" {
		return nil, fmt.Errorf("%w: empty sub", ErrInvalidToken)
	}
	return &Identity{
		Sub:            sub,
		Email:          stringClaim(claims["email"]),
		EmailVerified:  boolClaim(claims["email_verified"]),
		IsPrivateEmail: boolClaim(claims["is_private_email"]),
	}, nil
}

// lookupKey returns the RSA public key for kid. It refreshes the cache
// when the cache is empty, expired, or the kid is missing. On a refresh
// failure with a populated cache we fall back to the cache; first-time
// failures surface ErrJWKSFetchFailed.
func (v *Verifier) lookupKey(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.Lock()
	defer v.mu.Unlock()

	now := v.cfg.Now()
	cacheValid := v.cachedKeys != nil && now.Sub(v.cachedAt) < v.cfg.JWKSCacheTTL
	if cacheValid {
		if k, ok := v.cachedKeys[kid]; ok {
			return k, nil
		}
		// Unknown kid in a still-valid cache → force one refresh in
		// case Apple rotated keys early.
	}

	keys, err := v.fetchKeys(ctx)
	if err != nil {
		if v.cachedKeys == nil {
			return nil, ErrJWKSFetchFailed
		}
		log.Warn().Err(err).Str("jwks_url", v.cfg.JWKSURL).
			Msg("applex: jwks refresh failed, using cached keys")
		if k, ok := v.cachedKeys[kid]; ok {
			return k, nil
		}
		return nil, fmt.Errorf("%w: unknown kid %q (refresh failed)", ErrInvalidToken, kid)
	}
	v.cachedKeys = keys
	v.cachedAt = now
	if k, ok := v.cachedKeys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("%w: unknown kid %q", ErrInvalidToken, kid)
}

// jwk and jwkSet describe Apple's JWKS payload. Fields we don't use are
// intentionally omitted.
type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwkSet struct {
	Keys []jwk `json:"keys"`
}

func (v *Verifier) fetchKeys(ctx context.Context) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.cfg.JWKSURL, nil)
	if err != nil {
		return nil, fmt.Errorf("applex: build request: %w", err)
	}
	resp, err := v.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("applex: do request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("applex: jwks status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("applex: read body: %w", err)
	}
	var set jwkSet
	if err := json.Unmarshal(body, &set); err != nil {
		return nil, fmt.Errorf("applex: decode jwks: %w", err)
	}
	out := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := jwkToRSA(k)
		if err != nil {
			return nil, err
		}
		out[k.Kid] = pub
	}
	if len(out) == 0 {
		return nil, errors.New("applex: jwks empty")
	}
	return out, nil
}

func jwkToRSA(k jwk) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, fmt.Errorf("applex: decode N: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, fmt.Errorf("applex: decode E: %w", err)
	}
	e := 0
	for _, b := range eBytes {
		e = e<<8 | int(b)
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: e}, nil
}

func audMatches(aud any, allowed []string) bool {
	switch a := aud.(type) {
	case string:
		return slices.Contains(allowed, a)
	case []any:
		for _, x := range a {
			if s, ok := x.(string); ok && slices.Contains(allowed, s) {
				return true
			}
		}
	case []string:
		return slices.ContainsFunc(a, func(s string) bool { return slices.Contains(allowed, s) })
	}
	return false
}

func numericClaim(c jwt.MapClaims, name string) (int64, bool) {
	switch v := c[name].(type) {
	case float64:
		return int64(v), true
	case int64:
		return v, true
	case json.Number:
		n, err := v.Int64()
		if err == nil {
			return n, true
		}
	}
	return 0, false
}

func stringClaim(v any) string {
	s, _ := v.(string)
	return s
}

// boolClaim accepts both real bools and the "true"/"false" string form
// Apple sometimes returns for email_verified / is_private_email.
func boolClaim(v any) bool {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		return x == "true"
	}
	return false
}
