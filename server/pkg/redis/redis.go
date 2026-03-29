package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps the go-redis client.
type Client struct {
	RDB *redis.Client
}

// New creates a new Redis client with the given address and password.
func New(addr, password string) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &Client{RDB: rdb}, nil
}

// Ping checks the Redis connection health.
func (c *Client) Ping(ctx context.Context) error {
	return c.RDB.Ping(ctx).Err()
}

// Close closes the Redis connection.
func (c *Client) Close() error {
	return c.RDB.Close()
}
