package testutil

import (
	"context"
	"testing"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"
)

// SetupRedis starts a Redis container via Testcontainers and returns
// a connected client plus a cleanup function. The caller must NOT call
// t.Parallel() — container ports can conflict.
func SetupRedis(t *testing.T) (*redis.Client, func()) {
	t.Helper()
	ctx := context.Background()

	container, err := tcredis.Run(ctx, "redis:7")
	require.NoError(t, err)

	host, err := container.Host(ctx)
	require.NoError(t, err)
	port, err := container.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err)

	cli := redis.NewClient(&redis.Options{
		Addr: host + ":" + port.Port(),
	})
	require.NoError(t, cli.Ping(ctx).Err())

	cleanup := func() {
		_ = cli.Close()
		_ = container.Terminate(ctx)
	}
	return cli, cleanup
}
