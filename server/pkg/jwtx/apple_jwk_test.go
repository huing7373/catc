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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/pkg/clockx"
)

// jwksJSON renders one or more keys into Apple's JWKS JSON shape. Pass
// (kid, alg, *rsa.PublicKey) tuples — alg is per-key so a test can mix
// RS256 + RS384 to drive the §3.2 drop-non-RS256 invariant.
type jwkEntry struct {
	kid string
	alg string
	pub *rsa.PublicKey
}

func renderJWKS(t *testing.T, entries ...jwkEntry) []byte {
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
	}{}
	for _, e := range entries {
		doc.Keys = append(doc.Keys, k{
			Kty: "RSA",
			Kid: e.kid,
			Use: "sig",
			Alg: e.alg,
			N:   base64.RawURLEncoding.EncodeToString(e.pub.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(e.pub.E)).Bytes()),
		})
	}
	out, err := json.Marshal(doc)
	require.NoError(t, err)
	return out
}

func newTestKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	k, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	return k
}

// newMiniredisClient runs an in-process miniredis and returns the client +
// server (so tests can call mr.Close() to simulate a Redis outage).
func newMiniredisClient(t *testing.T) (*redis.Client, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = cli.Close() })
	return cli, mr
}

func newFetcher(t *testing.T, jwksURL string, cli *redis.Client) *AppleJWKFetcher {
	t.Helper()
	return NewAppleJWKFetcher(cli, clockx.NewRealClock(), AppleJWKConfig{
		JWKSURL:      jwksURL,
		CacheKey:     "apple_jwk:cache",
		CacheTTL:     24 * time.Hour,
		FetchTimeout: 2 * time.Second,
	})
}

func TestFetcher_CacheHit(t *testing.T) {
	t.Parallel()

	priv := newTestKey(t)
	body := renderJWKS(t, jwkEntry{kid: "k1", alg: "RS256", pub: &priv.PublicKey})

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cli, mr := newMiniredisClient(t)
	require.NoError(t, mr.Set("apple_jwk:cache", string(body)))

	f := newFetcher(t, srv.URL, cli)
	set, err := f.Get(context.Background())
	require.NoError(t, err)
	require.NotNil(t, set.Keys["k1"])
	assert.Equal(t, int32(0), atomic.LoadInt32(&hits), "cache hit must not call Apple")
}

func TestFetcher_CacheMissFetchesWritesThrough(t *testing.T) {
	t.Parallel()

	priv := newTestKey(t)
	body := renderJWKS(t, jwkEntry{kid: "k1", alg: "RS256", pub: &priv.PublicKey})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cli, mr := newMiniredisClient(t)
	f := newFetcher(t, srv.URL, cli)

	set, err := f.Get(context.Background())
	require.NoError(t, err)
	require.NotNil(t, set.Keys["k1"])

	cached, err := mr.Get("apple_jwk:cache")
	require.NoError(t, err)
	assert.Equal(t, string(body), cached, "miss path must write the raw response through")

	ttl := mr.TTL("apple_jwk:cache")
	assert.True(t, ttl > 23*time.Hour && ttl <= 24*time.Hour,
		"TTL must be configured 24h window (got %s)", ttl)
}

func TestFetcher_RedisReadErrorFallsBackToFetch(t *testing.T) {
	t.Parallel()

	priv := newTestKey(t)
	body := renderJWKS(t, jwkEntry{kid: "k1", alg: "RS256", pub: &priv.PublicKey})

	var hits int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cli, mr := newMiniredisClient(t)
	mr.Close() // simulate Redis outage — every command now returns connection-refused

	f := newFetcher(t, srv.URL, cli)
	set, err := f.Get(context.Background())
	require.NoError(t, err, "Redis outage must not fail the verifier — fail-open")
	require.NotNil(t, set.Keys["k1"])
	assert.GreaterOrEqual(t, atomic.LoadInt32(&hits), int32(1), "must have HTTP-fetched")
}

func TestFetcher_HTTPFetchErrorFailsClosed(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	cli, _ := newMiniredisClient(t)
	f := newFetcher(t, srv.URL, cli)

	set, err := f.Get(context.Background())
	assert.Nil(t, set)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "status 500")
}

func TestFetcher_HTTPTimeout(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, _ = w.Write([]byte(`{"keys":[]}`))
	}))
	defer srv.Close()

	cli, _ := newMiniredisClient(t)
	f := NewAppleJWKFetcher(cli, clockx.NewRealClock(), AppleJWKConfig{
		JWKSURL:      srv.URL,
		CacheKey:     "apple_jwk:cache",
		CacheTTL:     24 * time.Hour,
		FetchTimeout: 50 * time.Millisecond,
	})

	set, err := f.Get(context.Background())
	assert.Nil(t, set)
	require.Error(t, err)
}

// TestFetcher_SingleflightCoalesces holds the Apple HTTP handler open
// until 10 callers have all entered Get. Without singleflight all 10
// would fire 10 separate HTTP requests; with singleflight the counter is
// at most 1.
func TestFetcher_SingleflightCoalesces(t *testing.T) {
	t.Parallel()

	priv := newTestKey(t)
	body := renderJWKS(t, jwkEntry{kid: "k1", alg: "RS256", pub: &priv.PublicKey})

	var hits int32
	gate := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&hits, 1)
		<-gate
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cli, _ := newMiniredisClient(t)
	f := newFetcher(t, srv.URL, cli)

	const n = 10
	var wg sync.WaitGroup
	wg.Add(n)
	errs := make(chan error, n)
	for range n {
		go func() {
			defer wg.Done()
			_, err := f.Get(context.Background())
			errs <- err
		}()
	}

	// Give all goroutines time to enter Get and join the singleflight.
	time.Sleep(100 * time.Millisecond)
	close(gate)
	wg.Wait()
	close(errs)

	for err := range errs {
		require.NoError(t, err)
	}
	assert.Equal(t, int32(1), atomic.LoadInt32(&hits),
		"singleflight must coalesce concurrent misses to one Apple call")
}

func TestFetcher_NonRS256Dropped(t *testing.T) {
	t.Parallel()

	rs256 := newTestKey(t)
	rs384 := newTestKey(t)
	body := renderJWKS(t,
		jwkEntry{kid: "rs256-kid", alg: "RS256", pub: &rs256.PublicKey},
		jwkEntry{kid: "rs384-kid", alg: "RS384", pub: &rs384.PublicKey},
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	cli, _ := newMiniredisClient(t)
	f := newFetcher(t, srv.URL, cli)

	set, err := f.Get(context.Background())
	require.NoError(t, err)
	assert.NotNil(t, set.Keys["rs256-kid"], "RS256 key must be kept")
	assert.Nil(t, set.Keys["rs384-kid"], "RS384 key must be dropped (defense in depth behind §3.2)")
}

func TestFetcher_InvalidJSONNotCached(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("not json at all"))
	}))
	defer srv.Close()

	cli, mr := newMiniredisClient(t)
	f := newFetcher(t, srv.URL, cli)

	set, err := f.Get(context.Background())
	assert.Nil(t, set)
	require.Error(t, err)
	assert.False(t, mr.Exists("apple_jwk:cache"),
		"a malformed Apple response must NOT pin the cache to garbage")
}

// TestFetcher_ConfigHappyPath sanity-checks that a fully-populated
// AppleJWKConfig is accepted without panic / log.Fatal. The §4.1 fail-
// fast branches (JWKSURL / CacheKey empty, CacheTTL / FetchTimeout <= 0,
// nil redis / clock) all call log.Fatal which terminates the process —
// a per-branch test would need to re-exec the test binary, which is
// brittle on Windows; the source-level branches are the load-bearing
// guard, this test catches accidental construction regressions.
func TestFetcher_ConfigHappyPath(t *testing.T) {
	t.Parallel()

	cli, _ := newMiniredisClient(t)
	clk := clockx.NewRealClock()

	assert.NotPanics(t, func() {
		_ = NewAppleJWKFetcher(cli, clk, AppleJWKConfig{
			JWKSURL: "https://x", CacheKey: "k", CacheTTL: time.Second, FetchTimeout: time.Second,
		})
	})
}
