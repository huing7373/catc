// Package redisx wraps go-redis/v9 with MustConnect + Runnable in the
// same style as mongox.
package redisx

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

// Config is the minimal shape redisx needs to establish a connection.
type Config struct {
	Addr     string
	Password string
	DB       int
}

// MustConnect dials Redis and pings once, or log.Fatal on failure.
func MustConnect(cfg Config) *redis.Client {
	cli, err := Connect(cfg)
	if err != nil {
		log.Fatal().Err(err).Str("addr", cfg.Addr).Msg("redis connect failed")
	}
	return cli
}

// Connect is the testable core of MustConnect.
func Connect(cfg Config) (*redis.Client, error) {
	if cfg.Addr == "" {
		return nil, errors.New("redisx: empty addr")
	}
	cli := redis.NewClient(&redis.Options{
		Addr:     cfg.Addr,
		Password: cfg.Password,
		DB:       cfg.DB,
	})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := cli.Ping(ctx).Err(); err != nil {
		_ = cli.Close()
		return nil, fmt.Errorf("redisx: ping: %w", err)
	}
	return cli, nil
}

// HealthCheck pings with the caller's context. Returns nil if healthy.
func HealthCheck(ctx context.Context, cli *redis.Client) error {
	if cli == nil {
		return errors.New("redisx: nil client")
	}
	return cli.Ping(ctx).Err()
}

// Runnable adapts a *redis.Client into the Runnable lifecycle.
type Runnable struct {
	cli *redis.Client
}

// NewRunnable returns a lifecycle adapter for cli.
func NewRunnable(cli *redis.Client) *Runnable { return &Runnable{cli: cli} }

// Name identifies this component in shutdown logs.
func (r *Runnable) Name() string { return "redis" }

// Start is a no-op.
func (r *Runnable) Start(ctx context.Context) error { return nil }

// Final closes the Redis client. Safe to call multiple times.
func (r *Runnable) Final(ctx context.Context) error {
	if r.cli == nil {
		return nil
	}
	err := r.cli.Close()
	r.cli = nil
	return err
}
