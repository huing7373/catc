package config

import (
	"os"
	"strings"
	"testing"
	"time"
)

const sampleTOML = `
[server]
port = 9090
mode = "debug"
cors_allowed_origins = ["http://localhost:3000"]

[log]
level = "debug"
format = "json"

[mongo]
uri = "mongodb://localhost:27017"
database = "cat"
timeout_sec = 3

[redis]
addr = "localhost:6379"

[jwt]
access_secret = "a-secret"
refresh_secret = "r-secret"
access_secret_previous = "a-prev"
refresh_secret_previous = "r-prev"
access_ttl_min = 15
refresh_ttl_day = 14
issuer = "cat-test"

[apns]
bundle_id = "com.test.cat"

[apple]
jwks_url = "https://example.test/keys"
jwks_cache_ttl_min = 30
allowed_audiences = ["com.test.cat", "com.test.cat.watch"]
`

func TestLoadFromString_Valid(t *testing.T) {
	cfg, err := LoadFromString(sampleTOML)
	if err != nil {
		t.Fatalf("LoadFromString: %v", err)
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("port: %d", cfg.Server.Port)
	}
	if cfg.AccessTTL().Minutes() != 15 {
		t.Errorf("access ttl: %v", cfg.AccessTTL())
	}
	if cfg.RefreshTTL().Hours() != 14*24 {
		t.Errorf("refresh ttl: %v", cfg.RefreshTTL())
	}
	if cfg.JWT.Issuer != "cat-test" {
		t.Errorf("issuer: %q", cfg.JWT.Issuer)
	}
}

func TestLoadFromString_MissingSecretsFail(t *testing.T) {
	noSecrets := strings.Replace(sampleTOML, `access_secret = "a-secret"`, `access_secret = ""`, 1)
	noSecrets = strings.Replace(noSecrets, `refresh_secret = "r-secret"`, `refresh_secret = ""`, 1)

	_, err := LoadFromString(noSecrets)
	if err == nil {
		t.Fatal("expected validation error for missing secrets")
	}
	if !strings.Contains(err.Error(), "jwt.access_secret") {
		t.Errorf("error should mention jwt.access_secret, got: %v", err)
	}
}

func TestLoadFromString_InvalidMode(t *testing.T) {
	bad := strings.Replace(sampleTOML, `mode = "debug"`, `mode = "wrong"`, 1)
	_, err := LoadFromString(bad)
	if err == nil || !strings.Contains(err.Error(), "server.mode") {
		t.Fatalf("expected mode validation error, got: %v", err)
	}
}

func TestOverrideFromEnv(t *testing.T) {
	t.Setenv(EnvJWTAccessSecret, "env-access")
	t.Setenv(EnvRedisPassword, "env-redis")

	// Strip access secret from TOML so env fills it in.
	stripped := strings.Replace(sampleTOML, `access_secret = "a-secret"`, `access_secret = ""`, 1)
	cfg, err := LoadFromString(stripped)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.JWT.AccessSecret != "env-access" {
		t.Errorf("access secret override: %q", cfg.JWT.AccessSecret)
	}
	if cfg.Redis.Password != "env-redis" {
		t.Errorf("redis password override: %q", cfg.Redis.Password)
	}
}

func TestDefaults(t *testing.T) {
	// Minimal config; rely on defaults for timeouts & modes. APNs
	// bundle_id is set so applyDefaults can backfill apple.allowed_audiences.
	minimal := `
[server]
[log]
[mongo]
uri = "mongodb://x"
database = "cat"
[redis]
addr = "x:1"
[jwt]
access_secret = "a"
refresh_secret = "b"
[apns]
bundle_id = "com.example.cat"
`
	cfg, err := LoadFromString(minimal)
	if err != nil {
		t.Fatalf("LoadFromString: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("default port: %d", cfg.Server.Port)
	}
	if cfg.Server.Mode != "release" {
		t.Errorf("default mode: %q", cfg.Server.Mode)
	}
	if cfg.Log.Level != "info" {
		t.Errorf("default level: %q", cfg.Log.Level)
	}
	if cfg.MongoTimeout().Seconds() != 5 {
		t.Errorf("default mongo timeout: %v", cfg.MongoTimeout())
	}
	if cfg.ShutdownTimeout().Seconds() != 30 {
		t.Errorf("default shutdown: %v", cfg.ShutdownTimeout())
	}
	if cfg.JWT.Issuer != "cat-backend" {
		t.Errorf("default issuer: %q", cfg.JWT.Issuer)
	}
	if cfg.JWT.AccessTTLMin != 10080 {
		t.Errorf("default access ttl min: %d (want 10080 = 7d)", cfg.JWT.AccessTTLMin)
	}
	if cfg.AccessTTL() != 7*24*time.Hour {
		t.Errorf("default access ttl duration: %v", cfg.AccessTTL())
	}
	// Apple defaults (URL/TTL/audience backfill from bundle_id).
	if cfg.Apple.JWKSURL != "https://appleid.apple.com/auth/keys" {
		t.Errorf("default jwks url: %q", cfg.Apple.JWKSURL)
	}
	if cfg.Apple.JWKSCacheTTLMin != 60 {
		t.Errorf("default jwks ttl min: %d", cfg.Apple.JWKSCacheTTLMin)
	}
	if cfg.AppleJWKSCacheTTL() != time.Hour {
		t.Errorf("default jwks ttl duration: %v", cfg.AppleJWKSCacheTTL())
	}
	if len(cfg.Apple.AllowedAudiences) != 1 || cfg.Apple.AllowedAudiences[0] != "com.example.cat" {
		t.Errorf("aud backfill: %v", cfg.Apple.AllowedAudiences)
	}
}

func TestApple_Parsed(t *testing.T) {
	cfg, err := LoadFromString(sampleTOML)
	if err != nil {
		t.Fatalf("LoadFromString: %v", err)
	}
	if cfg.Apple.JWKSURL != "https://example.test/keys" {
		t.Errorf("jwks url: %q", cfg.Apple.JWKSURL)
	}
	if cfg.Apple.JWKSCacheTTLMin != 30 {
		t.Errorf("jwks ttl: %d", cfg.Apple.JWKSCacheTTLMin)
	}
	if len(cfg.Apple.AllowedAudiences) != 2 {
		t.Errorf("aud len: %d", len(cfg.Apple.AllowedAudiences))
	}
	if cfg.JWT.AccessSecretPrevious != "a-prev" || cfg.JWT.RefreshSecretPrevious != "r-prev" {
		t.Errorf("prev secrets parse: %+v", cfg.JWT)
	}
}

func TestOverrideFromEnv_NewKeys(t *testing.T) {
	t.Setenv(EnvJWTAccessSecretPrevious, "env-a-prev")
	t.Setenv(EnvJWTRefreshSecretPrevious, "env-r-prev")
	t.Setenv(EnvAppleJWKSURL, "https://override.test/keys")

	cfg, err := LoadFromString(sampleTOML)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.JWT.AccessSecretPrevious != "env-a-prev" {
		t.Errorf("env access prev: %q", cfg.JWT.AccessSecretPrevious)
	}
	if cfg.JWT.RefreshSecretPrevious != "env-r-prev" {
		t.Errorf("env refresh prev: %q", cfg.JWT.RefreshSecretPrevious)
	}
	if cfg.Apple.JWKSURL != "https://override.test/keys" {
		t.Errorf("env jwks url: %q", cfg.Apple.JWKSURL)
	}
}

func TestApple_ValidationFailsWhenAudAndBundleEmpty(t *testing.T) {
	noAud := `
[server]
[log]
[mongo]
uri = "mongodb://x"
database = "cat"
[redis]
addr = "x:1"
[jwt]
access_secret = "a"
refresh_secret = "b"
[apns]
bundle_id = ""
[apple]
allowed_audiences = []
`
	_, err := LoadFromString(noAud)
	if err == nil {
		t.Fatal("expected validation error when both apple.allowed_audiences and apns.bundle_id are empty")
	}
	if !strings.Contains(err.Error(), "allowed_audiences") {
		t.Errorf("error should mention allowed_audiences, got: %v", err)
	}
}

func TestLoad_FromFile(t *testing.T) {
	dir := t.TempDir()
	path := dir + string(os.PathSeparator) + "c.toml"
	if err := os.WriteFile(path, []byte(sampleTOML), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Mongo.Database != "cat" {
		t.Errorf("db: %q", cfg.Mongo.Database)
	}
}
