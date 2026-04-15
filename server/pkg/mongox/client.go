// Package mongox wraps the official mongo-driver/v2 client with:
//   - MustConnect: one-shot connect + ping, log.Fatal on failure.
//   - Runnable adapter so the App container can Final() it during shutdown.
//   - WithTx: session-scoped transaction helper for cross-collection writes.
//
// mongox does not depend on internal/ packages; the cmd/cat wiring layer
// adapts internal/config.MongoCfg into mongox.Config.
package mongox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// Config is the minimal shape mongox needs to establish a connection.
type Config struct {
	URI        string
	Database   string
	TimeoutSec int
}

// MustConnect dials Mongo and Pings once, or log.Fatal if it fails. The
// returned client is safe for concurrent use.
func MustConnect(cfg Config) *mongo.Client {
	cli, err := Connect(cfg)
	if err != nil {
		log.Fatal().Err(err).Str("uri", redactURI(cfg.URI)).Msg("mongo connect failed")
	}
	return cli
}

// Connect is the testable core of MustConnect.
func Connect(cfg Config) (*mongo.Client, error) {
	if cfg.URI == "" {
		return nil, errors.New("mongox: empty uri")
	}
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cli, err := mongo.Connect(options.Client().ApplyURI(cfg.URI))
	if err != nil {
		return nil, fmt.Errorf("mongox: connect: %w", err)
	}
	if err := cli.Ping(ctx, nil); err != nil {
		_ = cli.Disconnect(context.Background())
		return nil, fmt.Errorf("mongox: ping: %w", err)
	}
	return cli, nil
}

// HealthCheck pings the server with the caller's context. It returns nil
// if healthy.
func HealthCheck(ctx context.Context, cli *mongo.Client) error {
	if cli == nil {
		return errors.New("mongox: nil client")
	}
	return cli.Ping(ctx, nil)
}

// redactURI strips the "userinfo" portion from a mongo URI so secrets do
// not leak into logs.
func redactURI(uri string) string {
	const scheme = "://"
	i := indexOf(uri, scheme)
	if i < 0 {
		return uri
	}
	rest := uri[i+len(scheme):]
	at := indexOf(rest, "@")
	if at < 0 {
		return uri
	}
	return uri[:i+len(scheme)] + "****@" + rest[at+1:]
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
