package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMustLoad_ValidConfig(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")
	content := `
[server]
host = "0.0.0.0"
port = 8080

[log]
level = "info"
output = "stdout"

[mongo]
uri = "mongodb://localhost:27017"
db = "catdb"
timeout_sec = 5

[redis]
addr = "localhost:6379"
db = 0

[jwt]
private_key_path = "/path/to/active.pem"
private_key_path_old = "/path/to/old.pem"
active_kid = "key-2026-04"
old_kid = "key-2026-01"
issuer = "catserver"
access_expiry_sec = 900
refresh_expiry_sec = 2592000

[ws]
max_connections = 10000
connect_rate_per_window = 5
connect_rate_window_sec = 60
blacklist_default_ttl_sec = 86400

[apns]
key_id = "KEY123"
team_id = "TEAM123"
bundle_id = "com.test.cat"
key_path = "/path/to/key.p8"

[cdn]
base_url = "https://cdn.example.com"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	cfg := MustLoad(path)

	assert.Equal(t, "0.0.0.0", cfg.Server.Host)
	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, "mongodb://localhost:27017", cfg.Mongo.URI)
	assert.Equal(t, 5, cfg.Mongo.TimeoutSec)
	assert.Equal(t, "localhost:6379", cfg.Redis.Addr)
	assert.Equal(t, 10000, cfg.WS.MaxConnections)
	assert.Equal(t, "/path/to/active.pem", cfg.JWT.PrivateKeyPath)
	assert.Equal(t, "/path/to/old.pem", cfg.JWT.PrivateKeyPathOld)
	assert.Equal(t, "key-2026-04", cfg.JWT.ActiveKID)
	assert.Equal(t, "key-2026-01", cfg.JWT.OldKID)
	assert.Equal(t, "catserver", cfg.JWT.Issuer)
	assert.Equal(t, 900, cfg.JWT.AccessExpirySec)
	assert.Equal(t, 2592000, cfg.JWT.RefreshExpirySec)
	assert.NotEmpty(t, cfg.Hash)
	assert.Len(t, cfg.Hash, 8)
}

// TestMustLoad_OverrideWithoutWSSection verifies that an override config
// omitting [ws] entirely still loads — applyDefaults must fill the fields
// before validation. Regression guard for the review-round-1 finding: a
// thin local.toml without [ws] used to fail startup with
// "ws.connect_rate_per_window must be > 0".
func TestMustLoad_OverrideWithoutWSSection(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "override.toml")
	content := `
[server]
host = "0.0.0.0"
port = 8080

[redis]
addr = "localhost:6379"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	cfg := MustLoad(path)

	assert.Equal(t, 5, cfg.WS.ConnectRatePerWindow)
	assert.Equal(t, 60, cfg.WS.ConnectRateWindowSec)
	assert.Equal(t, 86400, cfg.WS.BlacklistDefaultTTLSec)
	assert.Equal(t, 60, cfg.WS.ResumeCacheTTLSec)
}

func TestMustLoad_HashDeterministic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")
	content := `
[server]
port = 9090

[ws]
connect_rate_per_window = 5
connect_rate_window_sec = 60
blacklist_default_ttl_sec = 86400
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	cfg1 := MustLoad(path)
	cfg2 := MustLoad(path)
	assert.Equal(t, cfg1.Hash, cfg2.Hash)
}
