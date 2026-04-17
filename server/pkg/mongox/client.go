package mongox

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/huing/cat/server/internal/config"
)

// Client wraps a MongoDB client with convenience helpers.
type Client struct {
	cli *mongo.Client
	db  string
}

// MustConnect creates a MongoDB client, pings it, and returns a Client.
// Calls log.Fatal on any failure (startup-only I/O).
func MustConnect(cfg config.MongoCfg) *Client {
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	cli, err := mongo.Connect(options.Client().ApplyURI(cfg.URI))
	if err != nil {
		log.Fatal().Err(err).Msg("mongo connect failed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	if err := cli.Ping(ctx, nil); err != nil {
		log.Fatal().Err(err).Msg("mongo ping failed")
	}

	return &Client{cli: cli, db: cfg.DB}
}

// DB returns the configured database handle.
func (c *Client) DB() *mongo.Database {
	return c.cli.Database(c.db)
}

// Raw returns the underlying *mongo.Client for transaction helpers.
func (c *Client) Raw() *mongo.Client {
	return c.cli
}

// HealthCheck pings MongoDB and returns any error.
func (c *Client) HealthCheck(ctx context.Context) error {
	return c.cli.Ping(ctx, nil)
}

// Name implements Runnable.
func (c *Client) Name() string { return "mongo" }

// Start implements Runnable. No-op because MustConnect already established the connection.
func (c *Client) Start(_ context.Context) error { return nil }

// Final implements Runnable. Disconnects from MongoDB. Idempotent.
func (c *Client) Final(ctx context.Context) error {
	return c.cli.Disconnect(ctx)
}
