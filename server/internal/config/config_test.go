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

	// APNs defaults applied even when the explicit [apns] section only
	// sets the bare identity fields.
	assert.Equal(t, "KEY123", cfg.APNs.KeyID)
	assert.Equal(t, "TEAM123", cfg.APNs.TeamID)
	assert.Equal(t, "apns:queue", cfg.APNs.StreamKey)
	assert.Equal(t, "apns:dlq", cfg.APNs.DLQKey)
	assert.Equal(t, "apns:retry", cfg.APNs.RetryZSetKey)
	assert.Equal(t, "apns_workers", cfg.APNs.ConsumerGroup)
	assert.Equal(t, 2, cfg.APNs.WorkerCount)
	assert.Equal(t, 300, cfg.APNs.IdemTTLSec)
	assert.Equal(t, 1000, cfg.APNs.ReadBlockMs)
	assert.Equal(t, 10, cfg.APNs.ReadCount)
	assert.Equal(t, []int{1000, 3000, 9000}, cfg.APNs.RetryBackoffsMs)
	assert.Equal(t, 4, cfg.APNs.MaxAttempts)
	assert.Equal(t, 30, cfg.APNs.TokenExpiryDays)
	assert.False(t, cfg.APNs.Enabled, "default enabled must be false — release must opt in")
}

// TestMustLoad_APNsDefaultsAppliedWhenSectionOmitted verifies that an
// override config without an [apns] section still boots. Regression guard
// mirroring the [ws] precedent.
func TestMustLoad_APNsDefaultsAppliedWhenSectionOmitted(t *testing.T) {
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

	assert.Equal(t, "apns:queue", cfg.APNs.StreamKey)
	assert.Equal(t, "apns:dlq", cfg.APNs.DLQKey)
	assert.Equal(t, "apns:retry", cfg.APNs.RetryZSetKey)
	assert.Equal(t, "apns_workers", cfg.APNs.ConsumerGroup)
	assert.Equal(t, 2, cfg.APNs.WorkerCount)
	assert.Equal(t, 300, cfg.APNs.IdemTTLSec)
	assert.Equal(t, []int{1000, 3000, 9000}, cfg.APNs.RetryBackoffsMs)
	assert.Equal(t, 4, cfg.APNs.MaxAttempts)
	assert.Equal(t, 30, cfg.APNs.TokenExpiryDays)
	assert.False(t, cfg.APNs.Enabled)
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
