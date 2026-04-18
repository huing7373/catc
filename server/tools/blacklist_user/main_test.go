package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/config"
	"github.com/huing/cat/server/pkg/redisx"
)

func setupRun(t *testing.T) (*miniredis.Miniredis, *config.Config, redis.Cmdable) {
	t.Helper()
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { cli.Close() })
	cfg := &config.Config{}
	cfg.WS.BlacklistDefaultTTLSec = 3600
	return mr, cfg, cli
}

type runResult struct {
	exit   int
	stdout string
	stderr string
}

func invokeRun(args []string, cfg *config.Config, cli redis.Cmdable) runResult {
	var out, errOut bytes.Buffer
	exit := run(args, &out, &errOut, cfg, cli)
	return runResult{exit: exit, stdout: out.String(), stderr: errOut.String()}
}

func TestRun_AddWithExplicitTTL(t *testing.T) {
	t.Parallel()
	mr, cfg, cli := setupRun(t)

	r := invokeRun([]string{"add", "u1", "2h"}, cfg, cli)
	assert.Equal(t, 0, r.exit)
	assert.Empty(t, r.stderr)

	var js map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(r.stdout)), &js))
	assert.Equal(t, "add", js["action"])
	assert.Equal(t, "u1", js["userId"])
	assert.Equal(t, "2h0m0s", js["ttl"])

	assert.True(t, mr.Exists("blacklist:device:u1"))
	ttl := mr.TTL("blacklist:device:u1")
	assert.LessOrEqual(t, ttl, 2*time.Hour)
	assert.Greater(t, ttl, time.Hour)
}

func TestRun_AddUsesDefaultTTLWhenOmitted(t *testing.T) {
	t.Parallel()
	mr, cfg, cli := setupRun(t)
	cfg.WS.BlacklistDefaultTTLSec = 7200 // 2h

	r := invokeRun([]string{"add", "u2"}, cfg, cli)
	assert.Equal(t, 0, r.exit, "stderr: %s", r.stderr)

	var js map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(r.stdout)), &js))
	assert.Equal(t, "2h0m0s", js["ttl"])

	ttl := mr.TTL("blacklist:device:u2")
	assert.LessOrEqual(t, ttl, 2*time.Hour)
	assert.Greater(t, ttl, time.Hour)
}

func TestRun_Remove(t *testing.T) {
	t.Parallel()
	mr, cfg, cli := setupRun(t)
	bl := redisx.NewBlacklist(cli)
	require.NoError(t, bl.Add(context.Background(), "u3", time.Hour))
	require.True(t, mr.Exists("blacklist:device:u3"))

	r := invokeRun([]string{"remove", "u3"}, cfg, cli)
	assert.Equal(t, 0, r.exit)

	var js map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(r.stdout)), &js))
	assert.Equal(t, "remove", js["action"])
	assert.Equal(t, "u3", js["userId"])

	assert.False(t, mr.Exists("blacklist:device:u3"))
}

func TestRun_StatusBlacklisted(t *testing.T) {
	t.Parallel()
	_, cfg, cli := setupRun(t)
	bl := redisx.NewBlacklist(cli)
	require.NoError(t, bl.Add(context.Background(), "u4", time.Hour))

	r := invokeRun([]string{"status", "u4"}, cfg, cli)
	assert.Equal(t, 0, r.exit)

	var js map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(r.stdout)), &js))
	assert.Equal(t, "status", js["action"])
	assert.Equal(t, "u4", js["userId"])
	assert.Equal(t, true, js["blacklisted"])
	assert.NotEmpty(t, js["ttl"])
}

func TestRun_StatusNotBlacklisted(t *testing.T) {
	t.Parallel()
	_, cfg, cli := setupRun(t)

	r := invokeRun([]string{"status", "ghost"}, cfg, cli)
	assert.Equal(t, 0, r.exit)

	var js map[string]any
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(r.stdout)), &js))
	assert.Equal(t, false, js["blacklisted"])
}

func TestRun_InvalidAction(t *testing.T) {
	t.Parallel()
	_, cfg, cli := setupRun(t)

	r := invokeRun([]string{"bogus", "u1"}, cfg, cli)
	assert.Equal(t, 1, r.exit)
	assert.Empty(t, r.stdout)
	assert.Contains(t, r.stderr, "unknown action")
}

func TestRun_MissingArgs(t *testing.T) {
	t.Parallel()
	_, cfg, cli := setupRun(t)

	r := invokeRun([]string{"add"}, cfg, cli)
	assert.Equal(t, 1, r.exit)
	assert.Contains(t, r.stderr, "usage")
}

func TestRun_InvalidTTL(t *testing.T) {
	t.Parallel()
	_, cfg, cli := setupRun(t)

	r := invokeRun([]string{"add", "u1", "not-a-duration"}, cfg, cli)
	assert.Equal(t, 1, r.exit)
	assert.Contains(t, r.stderr, "ttl")
}

func TestRun_ZeroTTL(t *testing.T) {
	t.Parallel()
	_, cfg, cli := setupRun(t)

	r := invokeRun([]string{"add", "u1", "0s"}, cfg, cli)
	assert.Equal(t, 1, r.exit)
	assert.Contains(t, strings.ToLower(r.stderr), "ttl")
}

func TestRun_NoArgs(t *testing.T) {
	t.Parallel()
	_, cfg, cli := setupRun(t)

	r := invokeRun([]string{}, cfg, cli)
	assert.Equal(t, 1, r.exit)
	assert.Contains(t, r.stderr, "usage")
}
