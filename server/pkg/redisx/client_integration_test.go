//go:build integration

package redisx_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/huing/cat/server/internal/config"
	"github.com/huing/cat/server/pkg/redisx"
)

func TestMustConnect_Integration(t *testing.T) {
	ctx := context.Background()

	container, err := tcredis.Run(ctx, "redis:7")
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err)

	cfg := config.RedisCfg{
		Addr: host + ":" + port.Port(),
		DB:   0,
	}

	cli := redisx.MustConnect(cfg)
	t.Cleanup(func() { _ = cli.Final(ctx) })

	t.Run("HealthCheck", func(t *testing.T) {
		err := cli.HealthCheck(ctx)
		assert.NoError(t, err)
	})

	t.Run("SetGet", func(t *testing.T) {
		cmd := cli.Cmdable()
		require.NoError(t, cmd.Set(ctx, "hello", "world", 0).Err())
		val, err := cmd.Get(ctx, "hello").Result()
		require.NoError(t, err)
		assert.Equal(t, "world", val)
	})
}
