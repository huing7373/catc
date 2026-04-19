// Command process_deletion_queue is the Story 1.6 ops CLI that sweeps
// the users collection for deletion_requested=true rows older than a
// configurable grace period (default 30 days — NFR-COMP-5 MVP SLA)
// and performs CASCADE cleanup: removes the user row + every
// apns_tokens row belonging to the user.
//
// Production ops use only — see docs/runbook/process_deletion_queue.md
// for the preflight checklist, two-eyes review policy, and rollback
// guidance. This tool MUST be invoked manually by an ops engineer
// after confirming a recent Mongo backup (PITR) exists.
//
// Usage:
//
//	process_deletion_queue [-config path] [-dry-run] [-older-than-days N] [-limit N]
//
// Before any write, the tool prints a destructive-action banner to
// stderr and blocks on stdin reading "CONFIRM" (case-sensitive) — any
// other input aborts with exit 1 (§21.5 CONFIRM guard; §21.8 #9).
// Use -dry-run to preview candidates without writing.
//
// On success: one JSON summary line to stdout ({deletedUsers,
// deletedApnsTokens, durationMs, dryRun, olderThanDays}). Errors
// go to stderr.
//
// This file is deliberately thin — the run() function lives in run.go
// and takes injected io.Reader/Writer + Mongo + Redis handles so
// main_test.go can drive it without spawning a subprocess.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/huing/cat/server/internal/config"
)

func main() {
	configPath := flag.String("config", "config/default.toml", "path to TOML config")
	dryRun := flag.Bool("dry-run", false, "print candidates without writing")
	olderThanDays := flag.Int("older-than-days", 30, "grace period before cascade delete (NFR-COMP-5)")
	limit := flag.Int("limit", 100, "max users to process in a single run (safety cap)")
	flag.Parse()

	cfg := config.MustLoad(*configPath)

	mongoCli, err := connectMongo(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "mongo connect: %v\n", err)
		os.Exit(1)
	}
	defer func() { _ = mongoCli.Disconnect(context.Background()) }()

	redisCli := redis.NewClient(&redis.Options{
		Addr: cfg.Redis.Addr,
		DB:   cfg.Redis.DB,
	})
	defer redisCli.Close()

	db := mongoCli.Database(cfg.Mongo.DB)

	os.Exit(run(runArgs{
		in:            os.Stdin,
		out:           os.Stdout,
		errOut:        os.Stderr,
		db:            db,
		redis:         redisCli,
		dryRun:        *dryRun,
		olderThanDays: *olderThanDays,
		limit:         *limit,
	}))
}

// connectMongo builds a minimal Mongo client using the same URI /
// timeout hints as cmd/cat. Kept here (instead of reusing
// pkg/mongox.MustConnect) because that helper calls log.Fatal on
// error; a CLI tool that fatalbombs during testing is harder to
// reason about than an explicit error return.
func connectMongo(cfg *config.Config) (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(cfg.Mongo.TimeoutSec)*time.Second)
	defer cancel()
	opts := options.Client().ApplyURI(cfg.Mongo.URI)
	cli, err := mongo.Connect(opts)
	if err != nil {
		return nil, err
	}
	if err := cli.Ping(ctx, nil); err != nil {
		_ = cli.Disconnect(context.Background())
		return nil, err
	}
	return cli, nil
}
