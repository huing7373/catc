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
	KeyID    string `toml:"key_id"`
	TeamID   string `toml:"team_id"`
	BundleID string `toml:"bundle_id"`
	KeyPath  string `toml:"key_path"`
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
}
