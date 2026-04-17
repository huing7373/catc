//go:build integration

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func writeTestConfig(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.toml")
	// port=0 lets the OS assign a free ephemeral port, avoiding TOCTOU races.
	content := `[server]
host = "127.0.0.1"
port = 0

[log]
level = "info"
output = "stdout"

[mongo]
uri = ""
db = ""

[redis]
addr = ""
db = 0

[jwt]
secret = ""
issuer = ""
expiry = 0

[apns]
key_id = ""
team_id = ""
bundle_id = ""
key_path = ""

[cdn]
base_url = ""
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func TestGracefulShutdown_SIGTERM(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("SIGTERM not supported on Windows; run in CI (Linux)")
	}

	binary := filepath.Join("..", "..", "..", "build", "catserver")
	if _, err := os.Stat(binary); os.IsNotExist(err) {
		t.Fatalf("binary not found at %s — run 'bash scripts/build.sh' first", binary)
	}

	configPath := writeTestConfig(t)

	cmd := exec.Command(binary, "-config", configPath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())

	time.Sleep(2 * time.Second)

	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		require.NoError(t, err, "process should exit with code 0")
	case <-time.After(30 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatal("process did not exit within 30 seconds after SIGTERM")
	}
}
