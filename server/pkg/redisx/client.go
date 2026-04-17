package redisx

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"

	"github.com/huing/cat/server/internal/config"
)

const redisPingTimeout = 10 * time.Second

// Client wraps a Redis client with convenience helpers.
type Client struct {
	cli *redis.Client
}

// MustConnect creates a Redis client and pings it.
// Calls log.Fatal on any failure (startup-only I/O).
func MustConnect(cfg config.RedisCfg) *Client {
	cli := redis.NewClient(&redis.Options{
		Addr: cfg.Addr,
		DB:   cfg.DB,
	})

	ctx, cancel := context.WithTimeout(context.Background(), redisPingTimeout)
	defer cancel()
	if err := cli.Ping(ctx).Err(); err != nil {
		log.Fatal().Err(err).Msg("redis ping failed")
	}

	return &Client{cli: cli}
}

// Cmdable returns the underlying redis.Cmdable for command execution.
func (c *Client) Cmdable() redis.Cmdable {
	return c.cli
}

// HealthCheck pings Redis and returns any error.
func (c *Client) HealthCheck(ctx context.Context) error {
	return c.cli.Ping(ctx).Err()
}

// Name implements Runnable.
func (c *Client) Name() string { return "redis" }

// Start implements Runnable. No-op because MustConnect already established the connection.
func (c *Client) Start(_ context.Context) error { return nil }

// Final implements Runnable. Closes the Redis connection. Idempotent.
func (c *Client) Final(_ context.Context) error {
	return c.cli.Close()
}
