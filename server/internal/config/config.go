// Package config loads and validates the runtime configuration of the
// cat backend from a TOML file, with optional environment variable
// overrides for secret values.
//
// Rules:
//   - MustLoad is called exactly once from cmd/cat.initialize.
//   - Business code must never call os.Getenv directly; all secret
//     plumbing goes through overrideFromEnv in this package.
//   - Secret fields (jwt access/refresh secret) left empty after load
//     cause log.Fatal.
package config

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/rs/zerolog/log"
)

// Config is the root configuration tree.
type Config struct {
	Server ServerCfg `toml:"server"`
	Log    LogCfg    `toml:"log"`
	Mongo  MongoCfg  `toml:"mongo"`
	Redis  RedisCfg  `toml:"redis"`
	JWT    JWTCfg    `toml:"jwt"`
	APNs   APNsCfg   `toml:"apns"`
	CDN    CDNCfg    `toml:"cdn"`
}

// ServerCfg covers the Gin HTTP server + CORS whitelist.
type ServerCfg struct {
	Port               int      `toml:"port"`
	Mode               string   `toml:"mode"` // "debug" or "release"
	CORSAllowedOrigins []string `toml:"cors_allowed_origins"`
	ShutdownTimeoutSec int      `toml:"shutdown_timeout_sec"`
}

// LogCfg covers zerolog behaviour.
type LogCfg struct {
	Level  string `toml:"level"`
	Format string `toml:"format"` // "json" or "console"
}

// MongoCfg covers the MongoDB connection.
type MongoCfg struct {
	URI        string `toml:"uri"`
	Database   string `toml:"database"`
	TimeoutSec int    `toml:"timeout_sec"`
}

// RedisCfg covers the Redis connection.
type RedisCfg struct {
	Addr     string `toml:"addr"`
	Password string `toml:"password"`
	DB       int    `toml:"db"`
}

// JWTCfg covers JWT signing secrets and TTLs.
type JWTCfg struct {
	AccessSecret  string `toml:"access_secret"`
	RefreshSecret string `toml:"refresh_secret"`
	AccessTTLMin  int    `toml:"access_ttl_min"`
	RefreshTTLDay int    `toml:"refresh_ttl_day"`
	Issuer        string `toml:"issuer"`
}

// APNsCfg covers Apple Push Notification credentials.
type APNsCfg struct {
	KeyID    string `toml:"key_id"`
	TeamID   string `toml:"team_id"`
	BundleID string `toml:"bundle_id"`
	KeyPath  string `toml:"key_path"`
}

// CDNCfg covers the skin asset CDN.
type CDNCfg struct {
	BaseURL   string `toml:"base_url"`
	UploadKey string `toml:"upload_key"`
}

// Env var names that overrideFromEnv looks for. Exposed so tests can
// reference the exact keys without hardcoding strings.
const (
	EnvJWTAccessSecret  = "CAT_JWT_ACCESS_SECRET"
	EnvJWTRefreshSecret = "CAT_JWT_REFRESH_SECRET"
	EnvMongoURI         = "CAT_MONGO_URI"
	EnvRedisPassword    = "CAT_REDIS_PASSWORD"
	EnvAPNsKeyPath      = "CAT_APNS_KEY_PATH"
	EnvCDNUploadKey     = "CAT_CDN_UPLOAD_KEY"
)

// MustLoad parses the TOML file at path, applies environment overrides
// for sensitive fields, validates, and returns the Config. On any error
// it logs Fatal and exits the process.
func MustLoad(path string) *Config {
	cfg, err := Load(path)
	if err != nil {
		log.Fatal().Err(err).Str("path", path).Msg("config load failed")
	}
	return cfg
}

// Load is the testable core of MustLoad: it returns an error instead of
// calling log.Fatal.
func Load(path string) (*Config, error) {
	var c Config
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return nil, fmt.Errorf("config: decode %s: %w", path, err)
	}
	c.applyDefaults()
	c.overrideFromEnv()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// LoadFromString parses TOML content from a string. Used in tests.
func LoadFromString(body string) (*Config, error) {
	var c Config
	if _, err := toml.Decode(body, &c); err != nil {
		return nil, fmt.Errorf("config: decode string: %w", err)
	}
	c.applyDefaults()
	c.overrideFromEnv()
	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Port == 0 {
		c.Server.Port = 8080
	}
	if c.Server.Mode == "" {
		c.Server.Mode = "release"
	}
	if c.Server.ShutdownTimeoutSec == 0 {
		c.Server.ShutdownTimeoutSec = 30
	}
	if c.Log.Level == "" {
		c.Log.Level = "info"
	}
	if c.Log.Format == "" {
		c.Log.Format = "json"
	}
	if c.Mongo.TimeoutSec == 0 {
		c.Mongo.TimeoutSec = 5
	}
	if c.JWT.AccessTTLMin == 0 {
		c.JWT.AccessTTLMin = 60
	}
	if c.JWT.RefreshTTLDay == 0 {
		c.JWT.RefreshTTLDay = 30
	}
	if c.JWT.Issuer == "" {
		c.JWT.Issuer = "cat-backend"
	}
}

// overrideFromEnv pulls sensitive values from the process environment,
// but only if they are non-empty. This is the ONLY place allowed to call
// os.Getenv in the entire codebase.
func (c *Config) overrideFromEnv() {
	if v := os.Getenv(EnvJWTAccessSecret); v != "" {
		c.JWT.AccessSecret = v
	}
	if v := os.Getenv(EnvJWTRefreshSecret); v != "" {
		c.JWT.RefreshSecret = v
	}
	if v := os.Getenv(EnvMongoURI); v != "" {
		c.Mongo.URI = v
	}
	if v := os.Getenv(EnvRedisPassword); v != "" {
		c.Redis.Password = v
	}
	if v := os.Getenv(EnvAPNsKeyPath); v != "" {
		c.APNs.KeyPath = v
	}
	if v := os.Getenv(EnvCDNUploadKey); v != "" {
		c.CDN.UploadKey = v
	}
}

// validate enforces non-empty secrets and sane primitive ranges.
func (c *Config) validate() error {
	var missing []string
	if c.JWT.AccessSecret == "" {
		missing = append(missing, "jwt.access_secret")
	}
	if c.JWT.RefreshSecret == "" {
		missing = append(missing, "jwt.refresh_secret")
	}
	if c.Mongo.URI == "" {
		missing = append(missing, "mongo.uri")
	}
	if c.Mongo.Database == "" {
		missing = append(missing, "mongo.database")
	}
	if c.Redis.Addr == "" {
		missing = append(missing, "redis.addr")
	}
	if len(missing) > 0 {
		return fmt.Errorf("config: missing required fields: %v", missing)
	}
	if c.Server.Port < 1 || c.Server.Port > 65535 {
		return errors.New("config: server.port out of range")
	}
	if c.Server.Mode != "debug" && c.Server.Mode != "release" {
		return fmt.Errorf("config: server.mode must be debug|release, got %q", c.Server.Mode)
	}
	return nil
}

// AccessTTL returns the access-token lifetime as a duration.
func (c *Config) AccessTTL() time.Duration {
	return time.Duration(c.JWT.AccessTTLMin) * time.Minute
}

// RefreshTTL returns the refresh-token lifetime as a duration.
func (c *Config) RefreshTTL() time.Duration {
	return time.Duration(c.JWT.RefreshTTLDay) * 24 * time.Hour
}

// MongoTimeout returns the Mongo connect-timeout as a duration.
func (c *Config) MongoTimeout() time.Duration {
	return time.Duration(c.Mongo.TimeoutSec) * time.Second
}

// ShutdownTimeout returns the graceful shutdown budget.
func (c *Config) ShutdownTimeout() time.Duration {
	return time.Duration(c.Server.ShutdownTimeoutSec) * time.Second
}
