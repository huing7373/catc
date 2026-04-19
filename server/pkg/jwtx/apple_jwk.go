package jwtx

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"golang.org/x/sync/singleflight"

	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/logx"
)

// JWKSet is the decoded Apple JWKS — a kid → *rsa.PublicKey map. Only
// RS256 / RSA keys are kept; any other alg/kty is dropped at parse time
// as defense in depth behind the §3.2 RS256-pinning check inside
// Manager.VerifyApple. Callers must NOT mutate the returned map.
type JWKSet struct {
	Keys map[string]*rsa.PublicKey
}

// AppleJWKConfig wires the runtime knobs of the fetcher. JWKSURL,
// CacheKey, CacheTTL, FetchTimeout are required positive / non-empty —
// NewAppleJWKFetcher fails fast otherwise (review-antipatterns §4.1).
type AppleJWKConfig struct {
	JWKSURL      string        // e.g. "https://appleid.apple.com/auth/keys"
	CacheKey     string        // e.g. "apple_jwk:cache" (D16 namespace)
	CacheTTL     time.Duration // e.g. 24h (NFR-INT-1)
	FetchTimeout time.Duration // e.g. 5s — bounds Apple HTTP call
}

// AppleJWKFetcher retrieves Apple's JWKS from appleid.apple.com and caches
// it in Redis (key cfg.CacheKey, TTL cfg.CacheTTL). On Redis read / write
// error the fetcher transparently degrades to a fresh HTTPS fetch
// (architecture §21.3 fail-open — Apple JWK caching is a performance
// optimization, NOT a security boundary: cryptographic verification
// happens regardless of cache hit / miss, so a dead Redis only costs
// latency and Apple QPS). A fresh HTTPS fetch that itself fails is
// fail-closed: the caller (Manager.VerifyApple) must reject the identity
// token with AUTH_INVALID_IDENTITY_TOKEN — cannot verify signature →
// cannot trust the token.
//
// Concurrency: in-flight parallel /auth/apple requests during cache miss
// are coalesced via singleflight.Group keyed by the constant cache key,
// so Apple sees at most 1 QPS regardless of local burst (review-
// antipatterns §12.1). The winner re-reads the cache before issuing the
// HTTP call to absorb the N+1 thundering-herd scenario where the cache
// just got populated by a previous winner.
type AppleJWKFetcher struct {
	redis      redis.Cmdable
	clock      clockx.Clock
	httpClient *http.Client
	jwksURL    string
	cacheKey   string
	cacheTTL   time.Duration
	fetchSF    singleflight.Group
}

// NewAppleJWKFetcher constructs an AppleJWKFetcher. All AppleJWKConfig
// positive-int / non-empty fields are validated fail-fast at startup
// (review-antipatterns §4.1) — there is no sane runtime fallback for
// "JWKS URL was empty" or "cache TTL was -1" so refusing to boot is
// strictly better than silently degrading auth.
func NewAppleJWKFetcher(r redis.Cmdable, clk clockx.Clock, cfg AppleJWKConfig) *AppleJWKFetcher {
	if r == nil {
		log.Fatal().Msg("jwtx.NewAppleJWKFetcher: redis Cmdable must not be nil")
	}
	if clk == nil {
		log.Fatal().Msg("jwtx.NewAppleJWKFetcher: clock must not be nil")
	}
	if cfg.JWKSURL == "" {
		log.Fatal().Msg("jwtx.NewAppleJWKFetcher: JWKSURL must not be empty")
	}
	if cfg.CacheKey == "" {
		log.Fatal().Msg("jwtx.NewAppleJWKFetcher: CacheKey must not be empty")
	}
	if cfg.CacheTTL <= 0 {
		log.Fatal().Dur("cache_ttl", cfg.CacheTTL).Msg("jwtx.NewAppleJWKFetcher: CacheTTL must be > 0")
	}
	if cfg.FetchTimeout <= 0 {
		log.Fatal().Dur("fetch_timeout", cfg.FetchTimeout).Msg("jwtx.NewAppleJWKFetcher: FetchTimeout must be > 0")
	}
	return &AppleJWKFetcher{
		redis:      r,
		clock:      clk,
		httpClient: &http.Client{Timeout: cfg.FetchTimeout},
		jwksURL:    cfg.JWKSURL,
		cacheKey:   cfg.CacheKey,
		cacheTTL:   cfg.CacheTTL,
	}
}

// Get returns the currently-trusted JWKS. Tries Redis first; on miss /
// Redis error / cached-value-corrupt it singleflight-coalesces a fresh
// HTTPS fetch and writes through. Returns an error only when the fresh
// HTTPS fetch itself fails (fail-closed for the security boundary).
func (f *AppleJWKFetcher) Get(ctx context.Context) (*JWKSet, error) {
	if set, ok := f.tryCacheRead(ctx); ok {
		return set, nil
	}

	res, err, _ := f.fetchSF.Do(f.cacheKey, func() (any, error) {
		// Re-read cache inside the critical section: an earlier
		// singleflight winner may have populated it before we grabbed
		// the slot. Without this, a wave of N concurrent callers all
		// observe the original miss and all serialize through the HTTP
		// fetch one after another instead of N-1 of them seeing the
		// just-populated value.
		if set, ok := f.tryCacheRead(ctx); ok {
			return set, nil
		}
		return f.fetchAndCache(ctx)
	})
	if err != nil {
		return nil, err
	}
	return res.(*JWKSet), nil
}

// tryCacheRead returns (set, true) on a hit with a parseable cached
// payload. On Redis miss (redis.Nil) it returns (nil, false) silently.
// On any other Redis error it logs warn and returns (nil, false) — the
// caller falls through to a fresh fetch (fail-open). On a parse failure
// it logs warn and returns (nil, false) so a corrupt cached value cannot
// pin the fetcher to a permanent broken state.
func (f *AppleJWKFetcher) tryCacheRead(ctx context.Context) (*JWKSet, bool) {
	raw, err := f.redis.Get(ctx, f.cacheKey).Bytes()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return nil, false
		}
		logx.Ctx(ctx).Warn().Err(err).
			Str("action", "apple_jwk_cache_read_error").
			Msg("redis cache get failed; falling through to HTTP fetch")
		return nil, false
	}
	set, parseErr := parseJWKS(raw)
	if parseErr != nil {
		logx.Ctx(ctx).Warn().Err(parseErr).
			Str("action", "apple_jwk_cache_decode_error").
			Msg("cached Apple JWKS unparseable; refetching")
		return nil, false
	}
	return set, true
}

// fetchAndCache performs the HTTPS fetch and writes the raw response
// through to Redis. Returns error only on HTTP failure or unparseable
// body (fail-closed); a failing Redis SET is logged warn but does NOT
// fail the call (fail-open — see fetcher doc comment).
func (f *AppleJWKFetcher) fetchAndCache(ctx context.Context) (*JWKSet, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("apple jwk: build request: %w", err)
	}
	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("apple jwk: http get: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("apple jwk: status %d", resp.StatusCode)
	}
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("apple jwk: read body: %w", err)
	}
	set, err := parseJWKS(raw)
	if err != nil {
		return nil, fmt.Errorf("apple jwk: parse body: %w", err)
	}
	if len(set.Keys) == 0 {
		return nil, fmt.Errorf("apple jwk: zero RS256 keys after parse")
	}

	if writeErr := f.redis.Set(ctx, f.cacheKey, raw, f.cacheTTL).Err(); writeErr != nil {
		logx.Ctx(ctx).Warn().Err(writeErr).
			Str("action", "apple_jwk_cache_write_error").
			Msg("redis cache set failed; returning anyway")
	}

	logx.Ctx(ctx).Info().
		Str("action", "apple_jwk_fetched").
		Int("kidCount", len(set.Keys)).
		Msg("apple jwk fetched")
	return set, nil
}

// rawJWK is the on-the-wire shape of a single key in Apple's JWKS
// response. Apple uses RFC 7517 base64url-encoded big-endian modulus and
// exponent for RSA keys.
type rawJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type rawJWKS struct {
	Keys []rawJWK `json:"keys"`
}

// parseJWKS decodes Apple's JWKS JSON and keeps only RS256 / RSA keys.
// Any other alg / kty is silently dropped — defense in depth behind the
// §3.2 RS256-pinning check in Manager.VerifyApple, so that even if a
// future bug let a non-RS256 key through here, the verifier would still
// refuse to use it.
func parseJWKS(raw []byte) (*JWKSet, error) {
	var doc rawJWKS
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("decode jwks: %w", err)
	}
	set := &JWKSet{Keys: make(map[string]*rsa.PublicKey, len(doc.Keys))}
	for _, k := range doc.Keys {
		if k.Alg != "RS256" || k.Kty != "RSA" {
			continue
		}
		if k.Kid == "" || k.N == "" || k.E == "" {
			continue
		}
		nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		set.Keys[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(nBytes),
			E: int(new(big.Int).SetBytes(eBytes).Int64()),
		}
	}
	return set, nil
}
