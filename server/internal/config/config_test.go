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

[redis]
addr = "localhost:6379"
db = 0

[jwt]
secret = "test-secret"
issuer = "cat"
expiry = 3600

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
	assert.Equal(t, "localhost:6379", cfg.Redis.Addr)
	assert.Equal(t, "test-secret", cfg.JWT.Secret)
	assert.NotEmpty(t, cfg.Hash)
	assert.Len(t, cfg.Hash, 8)
}

func TestMustLoad_HashDeterministic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")
	content := `
[server]
port = 9090
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))

	cfg1 := MustLoad(path)
	cfg2 := MustLoad(path)
	assert.Equal(t, cfg1.Hash, cfg2.Hash)
}
