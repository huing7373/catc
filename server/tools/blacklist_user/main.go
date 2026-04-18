// Command blacklist_user is a one-off operations CLI for adding, removing,
// or querying device blacklist entries managed by pkg/redisx/RedisBlacklist.
//
// Usage:
//
//	blacklist_user [-config path] add    <userId> [ttl]
//	blacklist_user [-config path] remove <userId>
//	blacklist_user [-config path] status <userId>
//
// ttl accepts Go duration syntax (e.g. "24h", "30m"); omitted means
// cfg.WS.BlacklistDefaultTTLSec.
//
// On success: one JSON line to stdout.
// On failure: error to stderr, exit 1.
//
// This file is deliberately thin — all logic lives in run() for testability
// (main_test.go drives run() directly with miniredis). main only wires the
// real redis connection and os.Exit.
package main

import (
	"flag"
	"os"

	"github.com/redis/go-redis/v9"

	"github.com/huing/cat/server/internal/config"
)

func main() {
	configPath := flag.String("config", "config/default.toml", "path to TOML config")
	flag.Parse()

	cfg := config.MustLoad(*configPath)

	cli := redis.NewClient(&redis.Options{
		Addr: cfg.Redis.Addr,
		DB:   cfg.Redis.DB,
	})
	defer cli.Close()

	os.Exit(run(flag.Args(), os.Stdout, os.Stderr, cfg, cli))
}
