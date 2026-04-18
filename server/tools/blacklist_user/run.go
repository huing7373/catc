package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/huing/cat/server/internal/config"
	"github.com/huing/cat/server/pkg/redisx"
)

const usageMsg = "usage: blacklist_user <action> <userId> [ttl]\n" +
	"  actions: add, remove, status\n" +
	"  ttl: Go duration (e.g. 24h); omitted = cfg.WS.BlacklistDefaultTTLSec"

// run is the testable core of the CLI. It is pure with respect to I/O:
// no flag parsing, no os.Exit — the caller (main) handles those.
//
// Returns a process exit code (0 success, 1 failure).
func run(args []string, out, errOut io.Writer, cfg *config.Config, cli redis.Cmdable) int {
	if len(args) < 1 {
		fmt.Fprintln(errOut, usageMsg)
		return 1
	}

	bl := redisx.NewBlacklist(cli)
	ctx := context.Background()
	action := args[0]

	switch action {
	case "add":
		if len(args) < 2 {
			fmt.Fprintln(errOut, usageMsg)
			return 1
		}
		userID := args[1]
		var ttl time.Duration
		if len(args) >= 3 {
			d, err := time.ParseDuration(args[2])
			if err != nil {
				fmt.Fprintf(errOut, "invalid ttl %q: %v\n", args[2], err)
				return 1
			}
			ttl = d
		} else {
			ttl = time.Duration(cfg.WS.BlacklistDefaultTTLSec) * time.Second
		}
		if err := bl.Add(ctx, userID, ttl); err != nil {
			fmt.Fprintf(errOut, "add failed: %v\n", err)
			return 1
		}
		writeJSON(out, map[string]any{
			"action": "add",
			"userId": userID,
			"ttl":    ttl.String(),
		})
		return 0

	case "remove":
		if len(args) < 2 {
			fmt.Fprintln(errOut, usageMsg)
			return 1
		}
		userID := args[1]
		if err := bl.Remove(ctx, userID); err != nil {
			fmt.Fprintf(errOut, "remove failed: %v\n", err)
			return 1
		}
		writeJSON(out, map[string]any{
			"action": "remove",
			"userId": userID,
		})
		return 0

	case "status":
		if len(args) < 2 {
			fmt.Fprintln(errOut, usageMsg)
			return 1
		}
		userID := args[1]
		ttl, exists, err := bl.TTL(ctx, userID)
		if err != nil {
			fmt.Fprintf(errOut, "status failed: %v\n", err)
			return 1
		}
		writeJSON(out, map[string]any{
			"action":      "status",
			"userId":      userID,
			"blacklisted": exists,
			"ttl":         ttl.String(),
		})
		return 0

	default:
		fmt.Fprintf(errOut, "unknown action %q\n%s\n", action, usageMsg)
		return 1
	}
}

func writeJSON(w io.Writer, m map[string]any) {
	enc := json.NewEncoder(w)
	_ = enc.Encode(m)
}
