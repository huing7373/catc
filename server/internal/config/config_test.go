package config

import (
	"os"
	"strings"
	"testing"
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
access_ttl_min = 15
refresh_ttl_day = 14
issuer = "cat-test"
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
	// Minimal config; rely on defaults for timeouts & modes.
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
