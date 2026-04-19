package config

import (
	"crypto/sha256"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
	"github.com/rs/zerolog/log"
)

type Config struct {
	Server ServerCfg `toml:"server"`
	Log    LogCfg    `toml:"log"`
	Mongo  MongoCfg  `toml:"mongo"`
	Redis  RedisCfg  `toml:"redis"`
	JWT    JWTCfg    `toml:"jwt"`
	WS     WSCfg     `toml:"ws"`
	APNs   APNsCfg   `toml:"apns"`
	Apple  AppleCfg  `toml:"apple"`
	CDN    CDNCfg    `toml:"cdn"`
	Hash   string    `toml:"-"`
}

type ServerCfg struct {
	Host string `toml:"host"`
	Port int    `toml:"port"`
	TLS  bool   `toml:"tls"`
	Mode string `toml:"mode"`
}

type LogCfg struct {
	Level  string `toml:"level"`
	Format string `toml:"format"`
	Output string `toml:"output"`
}

type MongoCfg struct {
	URI        string `toml:"uri"`
	DB         string `toml:"db"`
	TimeoutSec int    `toml:"timeout_sec"`
}

type RedisCfg struct {
	Addr string `toml:"addr"`
	DB   int    `toml:"db"`
}

type JWTCfg struct {
	PrivateKeyPath    string `toml:"private_key_path"`
	PrivateKeyPathOld string `toml:"private_key_path_old"`
	ActiveKID         string `toml:"active_kid"`
	OldKID            string `toml:"old_kid"`
	Issuer            string `toml:"issuer"`
	AccessExpirySec   int    `toml:"access_expiry_sec"`
	RefreshExpirySec  int    `toml:"refresh_expiry_sec"`
}

type WSCfg struct {
	MaxConnections         int `toml:"max_connections"`
	PingIntervalSec        int `toml:"ping_interval_sec"`
	PongTimeoutSec         int `toml:"pong_timeout_sec"`
	SendBufSize            int `toml:"send_buf_size"`
	DedupTTLSec            int `toml:"dedup_ttl_sec"`
	ConnectRatePerWindow   int `toml:"connect_rate_per_window"`
	ConnectRateWindowSec   int `toml:"connect_rate_window_sec"`
	BlacklistDefaultTTLSec int `toml:"blacklist_default_ttl_sec"`
	ResumeCacheTTLSec      int `toml:"resume_cache_ttl_sec"`
}

type APNsCfg struct {
	KeyID           string `toml:"key_id"`
	TeamID          string `toml:"team_id"`
	BundleID        string `toml:"bundle_id"`
	KeyPath         string `toml:"key_path"`
	WatchTopic      string `toml:"watch_topic"`
	IphoneTopic     string `toml:"iphone_topic"`
	StreamKey       string `toml:"stream_key"`
	DLQKey          string `toml:"dlq_key"`
	RetryZSetKey    string `toml:"retry_zset_key"`
	ConsumerGroup   string `toml:"consumer_group"`
	WorkerCount     int    `toml:"worker_count"`
	IdemTTLSec      int    `toml:"idem_ttl_sec"`
	ReadBlockMs     int    `toml:"read_block_ms"`
	ReadCount       int    `toml:"read_count"`
	RetryBackoffsMs []int  `toml:"retry_backoffs_ms"`
	MaxAttempts     int    `toml:"max_attempts"`
	TokenExpiryDays int    `toml:"token_expiry_days"`
	Enabled         bool   `toml:"enabled"`
}

// AppleCfg holds the Sign in with Apple verification knobs (Story 1.1).
// BundleID is the expected `aud` of every accepted Apple identity token
// — production must set it explicitly (no compile-time default), per
// the same fail-fast pattern as APNs.KeyID. JWKSURL / cache key / TTLs
// have safe defaults so a thin local override file boots without a full
// [apple] block (review-antipatterns §4.2).
type AppleCfg struct {
	BundleID            string `toml:"bundle_id"`
	JWKSURL             string `toml:"jwks_url"`
	JWKSCacheKey        string `toml:"jwks_cache_key"`
	JWKSCacheTTLSec     int    `toml:"jwks_cache_ttl_sec"`
	JWKSFetchTimeoutSec int    `toml:"jwks_fetch_timeout_sec"`
}

type CDNCfg struct {
	BaseURL string `toml:"base_url"`
}

func MustLoad(path string) *Config {
	data, err := os.ReadFile(path)
	if err != nil {
		log.Fatal().Err(err).Str("path", path).Msg("config read failed")
	}

	h := sha256.Sum256(data)
	hash := fmt.Sprintf("%x", h[:4])

	var c Config
	if _, err := toml.Decode(string(data), &c); err != nil {
		log.Fatal().Err(err).Str("path", path).Msg("config decode failed")
	}

	c.Hash = hash
	c.applyDefaults()
	c.mustValidate()
	return &c
}

// applyDefaults fills zero-valued fields with compile-time defaults so that
// override configs (e.g. config/local.toml) that omit a section keep
// working after new keys are added. Without this step, adding a required
// key forces every existing override config to add it or fail at startup —
// a real regression for any environment using a thin override file.
//
// default.toml remains the documented source of truth for operators; these
// compile-time defaults only engage when both the user's file and
// default.toml omit the key (which should not happen for default.toml).
func (c *Config) applyDefaults() {
	if c.WS.ConnectRatePerWindow == 0 {
		c.WS.ConnectRatePerWindow = 5
	}
	if c.WS.ConnectRateWindowSec == 0 {
		c.WS.ConnectRateWindowSec = 60
	}
	if c.WS.BlacklistDefaultTTLSec == 0 {
		c.WS.BlacklistDefaultTTLSec = 86400
	}
	if c.WS.ResumeCacheTTLSec == 0 {
		c.WS.ResumeCacheTTLSec = 60
	}
	if c.APNs.StreamKey == "" {
		c.APNs.StreamKey = "apns:queue"
	}
	if c.APNs.DLQKey == "" {
		c.APNs.DLQKey = "apns:dlq"
	}
	if c.APNs.RetryZSetKey == "" {
		c.APNs.RetryZSetKey = "apns:retry"
	}
	if c.APNs.ConsumerGroup == "" {
		c.APNs.ConsumerGroup = "apns_workers"
	}
	if c.APNs.WorkerCount == 0 {
		c.APNs.WorkerCount = 2
	}
	if c.APNs.IdemTTLSec == 0 {
		c.APNs.IdemTTLSec = 300
	}
	if c.APNs.ReadBlockMs == 0 {
		c.APNs.ReadBlockMs = 1000
	}
	if c.APNs.ReadCount == 0 {
		c.APNs.ReadCount = 10
	}
	if len(c.APNs.RetryBackoffsMs) == 0 {
		c.APNs.RetryBackoffsMs = []int{1000, 3000, 9000}
	}
	if c.APNs.MaxAttempts == 0 {
		c.APNs.MaxAttempts = 4
	}
	if c.APNs.TokenExpiryDays == 0 {
		c.APNs.TokenExpiryDays = 30
	}
	// Apple SIWA defaults — JWKS endpoint and cache knobs are operational
	// constants; only BundleID has no sane compile-time default (it is the
	// per-deployment app identity, must be set explicitly like APNs.KeyID).
	if c.Apple.JWKSURL == "" {
		c.Apple.JWKSURL = "https://appleid.apple.com/auth/keys"
	}
	if c.Apple.JWKSCacheKey == "" {
		c.Apple.JWKSCacheKey = "apple_jwk:cache"
	}
	if c.Apple.JWKSCacheTTLSec == 0 {
		c.Apple.JWKSCacheTTLSec = 86400
	}
	if c.Apple.JWKSFetchTimeoutSec == 0 {
		c.Apple.JWKSFetchTimeoutSec = 5
	}
}

func (c *Config) mustValidate() {
	if c.Server.Port < 0 {
		log.Fatal().Int("port", c.Server.Port).Msg("config: server.port must be >= 0")
	}
	// Post-applyDefaults: a still-<=0 value means the operator explicitly set
	// a negative (e.g. -1 to "disable"). Fail loudly rather than silently
	// re-opening the J4 WS-storm failure mode with no rate limiter.
	if c.WS.ConnectRatePerWindow <= 0 {
		log.Fatal().Int("connect_rate_per_window", c.WS.ConnectRatePerWindow).
			Msg("config: ws.connect_rate_per_window must be > 0")
	}
	if c.WS.ConnectRateWindowSec <= 0 {
		log.Fatal().Int("connect_rate_window_sec", c.WS.ConnectRateWindowSec).
			Msg("config: ws.connect_rate_window_sec must be > 0")
	}
	if c.WS.BlacklistDefaultTTLSec <= 0 {
		log.Fatal().Int("blacklist_default_ttl_sec", c.WS.BlacklistDefaultTTLSec).
			Msg("config: ws.blacklist_default_ttl_sec must be > 0")
	}
	// session.resume cache TTL is the NFR-PERF-6 60s window; a non-positive
	// value would make Put reject at Redis-command time (invalid TTL) and
	// every resume would hit the providers — FR42 regression. Fail fast.
	if c.WS.ResumeCacheTTLSec <= 0 {
		log.Fatal().Int("resume_cache_ttl_sec", c.WS.ResumeCacheTTLSec).
			Msg("config: ws.resume_cache_ttl_sec must be > 0")
	}
	c.validateAPNs()
	c.validateApple()
}

// validateApple enforces Story 1.1 Apple SIWA positive-int invariants
// (review-antipatterns §4.1). bundle_id is intentionally NOT validated
// here: the value is required only for the SIWA verifier construction
// path, and validating at config load makes every operations CLI
// (e.g. tools/blacklist_user) fatal on the default config even though
// it never instantiates the verifier. The actual fail-fast guard lives
// on jwtx.NewManagerWithApple (which the main server unconditionally
// constructs) — same pattern as APNs.KeyID / KeyPath, which only fail
// when apns.enabled=true forces them down a consumer path.
func (c *Config) validateApple() {
	if c.Apple.JWKSCacheTTLSec <= 0 {
		log.Fatal().Int("jwks_cache_ttl_sec", c.Apple.JWKSCacheTTLSec).
			Msg("config: apple.jwks_cache_ttl_sec must be > 0")
	}
	if c.Apple.JWKSFetchTimeoutSec <= 0 {
		log.Fatal().Int("jwks_fetch_timeout_sec", c.Apple.JWKSFetchTimeoutSec).
			Msg("config: apple.jwks_fetch_timeout_sec must be > 0")
	}
}

// validateAPNs enforces positive-integer invariants on APNs worker /
// retry knobs always, and — when push is explicitly enabled — requires
// the production secrets (key_path / key_id / team_id) and per-platform
// topics to be provided. Disable the push platform with `enabled = false`
// rather than zeroing numeric fields (0.11 / 0.12 discipline).
func (c *Config) validateAPNs() {
	if c.APNs.WorkerCount <= 0 {
		log.Fatal().Int("worker_count", c.APNs.WorkerCount).Msg("config: apns.worker_count must be > 0")
	}
	if c.APNs.IdemTTLSec <= 0 {
		log.Fatal().Int("idem_ttl_sec", c.APNs.IdemTTLSec).Msg("config: apns.idem_ttl_sec must be > 0")
	}
	if c.APNs.ReadBlockMs <= 0 {
		log.Fatal().Int("read_block_ms", c.APNs.ReadBlockMs).Msg("config: apns.read_block_ms must be > 0")
	}
	if c.APNs.ReadCount <= 0 {
		log.Fatal().Int("read_count", c.APNs.ReadCount).Msg("config: apns.read_count must be > 0")
	}
	if len(c.APNs.RetryBackoffsMs) == 0 {
		log.Fatal().Msg("config: apns.retry_backoffs_ms must be non-empty")
	}
	for _, ms := range c.APNs.RetryBackoffsMs {
		if ms <= 0 {
			log.Fatal().Int("backoff_ms", ms).Msg("config: apns.retry_backoffs_ms entries must be > 0")
		}
	}
	if c.APNs.MaxAttempts <= 0 {
		log.Fatal().Int("max_attempts", c.APNs.MaxAttempts).Msg("config: apns.max_attempts must be > 0")
	}
	if c.APNs.TokenExpiryDays <= 0 {
		log.Fatal().Int("token_expiry_days", c.APNs.TokenExpiryDays).Msg("config: apns.token_expiry_days must be > 0")
	}
	if !c.APNs.Enabled {
		return
	}
	if c.APNs.KeyPath == "" {
		log.Fatal().Msg("config: apns.enabled=true requires apns.key_path")
	}
	if c.APNs.KeyID == "" {
		log.Fatal().Msg("config: apns.enabled=true requires apns.key_id")
	}
	if c.APNs.TeamID == "" {
		log.Fatal().Msg("config: apns.enabled=true requires apns.team_id")
	}
	if c.APNs.WatchTopic == "" {
		log.Fatal().Msg("config: apns.enabled=true requires apns.watch_topic")
	}
	if c.APNs.IphoneTopic == "" {
		log.Fatal().Msg("config: apns.enabled=true requires apns.iphone_topic")
	}
}
