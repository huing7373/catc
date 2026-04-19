package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/redis/go-redis/v9"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// runArgs bundles everything run() needs. Exported struct fields keep
// tests readable (no positional-argument soup) while the type itself
// stays unexported so no external package wires it accidentally.
type runArgs struct {
	in            io.Reader
	out           io.Writer
	errOut        io.Writer
	db            *mongo.Database
	redis         redis.Cmdable // reserved for future Redis cascade; currently unused
	dryRun        bool
	olderThanDays int
	limit         int
	// clockNow is an optional injection point so tests can pin "now"
	// deterministically. Production leaves it nil and the tool uses
	// time.Now() — this is the one place the tool is allowed to call
	// time.Now() because the CLI is not subject to the business-code
	// M9 guard (tools/ is exempted from the build-script check).
	clockNow func() time.Time
}

// runSummary is the JSON shape written to stdout on success.
type runSummary struct {
	DeletedUsers      int64 `json:"deletedUsers"`
	DeletedApnsTokens int64 `json:"deletedApnsTokens"`
	DurationMs        int64 `json:"durationMs"`
	DryRun            bool  `json:"dryRun"`
	OlderThanDays     int   `json:"olderThanDays"`
}

const (
	banner = "!!! This will PERMANENTLY DELETE user data from Mongo + Redis. Type CONFIRM to proceed:\n"
	// confirmPhrase is the literal stdin input required to proceed.
	// Must be case-sensitive — see main.go doc comment.
	confirmPhrase = "CONFIRM"
)

// run is the testable core of the CLI. It prints the destructive-action
// banner to errOut, reads CONFIRM from in, then runs the sweep (dry or
// real) and writes a JSON summary to out on success.
//
// Returns a process exit code (0 success, 1 failure).
func run(args runArgs) int {
	start := time.Now()

	// --- §21.5 CONFIRM guard ---
	// Skipping the banner in dry-run would be convenient but dangerous:
	// if a future retro lowers the default to dry-run=false, an ops
	// engineer who rehearsed `go run ./tools/process_deletion_queue
	// -dry-run=true` would destroy data the second they copy-paste the
	// command without reviewing the flag.
	if _, err := fmt.Fprint(args.errOut, banner); err != nil {
		fmt.Fprintf(args.errOut, "banner write: %v\n", err)
		return 1
	}
	reader := bufio.NewReader(args.in)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		fmt.Fprintf(args.errOut, "confirmation read failed: %v\n", err)
		return 1
	}
	// Trim newline without using strings.TrimSpace — we want
	// whitespace intolerance so an accidental leading space aborts.
	confirmed := stripTrailingNewline(line)
	if confirmed != confirmPhrase {
		fmt.Fprintf(args.errOut, "aborted: confirmation phrase %q did not match %q\n", confirmed, confirmPhrase)
		return 1
	}

	// --- Parameter validation ---
	if args.olderThanDays < 0 {
		fmt.Fprintf(args.errOut, "older-than-days must be >= 0; got %d\n", args.olderThanDays)
		return 1
	}
	if args.limit <= 0 {
		fmt.Fprintf(args.errOut, "limit must be > 0; got %d\n", args.limit)
		return 1
	}

	now := time.Now()
	if args.clockNow != nil {
		now = args.clockNow()
	}
	cutoff := now.Add(-time.Duration(args.olderThanDays) * 24 * time.Hour)

	ctx := context.Background()

	// --- Step 1: find candidate users ---
	users := args.db.Collection("users")
	findOpts := options.Find().
		SetLimit(int64(args.limit)).
		SetProjection(bson.M{"_id": 1})
	cur, err := users.Find(ctx, bson.M{
		"deletion_requested":    true,
		"deletion_requested_at": bson.M{"$lt": cutoff},
	}, findOpts)
	if err != nil {
		fmt.Fprintf(args.errOut, "mongo find: %v\n", err)
		return 1
	}
	defer cur.Close(ctx)

	var candidateIDs []string
	for cur.Next(ctx) {
		var doc struct {
			ID string `bson:"_id"`
		}
		if decErr := cur.Decode(&doc); decErr != nil {
			fmt.Fprintf(args.errOut, "mongo decode: %v\n", decErr)
			return 1
		}
		candidateIDs = append(candidateIDs, doc.ID)
	}
	if err := cur.Err(); err != nil {
		fmt.Fprintf(args.errOut, "mongo cursor: %v\n", err)
		return 1
	}

	// --- Step 2: dry-run short-circuit ---
	if args.dryRun {
		summary := runSummary{
			DeletedUsers:      int64(len(candidateIDs)),
			DeletedApnsTokens: 0,
			DurationMs:        time.Since(start).Milliseconds(),
			DryRun:            true,
			OlderThanDays:     args.olderThanDays,
		}
		writeJSON(args.out, summary)
		return 0
	}

	// --- Step 3: cascade delete ---
	// Order: apns_tokens FIRST, then users. If we deleted users first
	// and then crashed before apns_tokens, Epic 8 cold-start recall
	// (reads apns_tokens by user_id) would try to push to a user that
	// no longer exists — §21.8 #10 leak.
	apnsTokens := args.db.Collection("apns_tokens")
	var totalTokens int64
	var totalUsers int64
	for _, uid := range candidateIDs {
		tokRes, err := apnsTokens.DeleteMany(ctx, bson.M{"user_id": uid})
		if err != nil {
			fmt.Fprintf(args.errOut, "delete apns_tokens for %q: %v\n", uid, err)
			return 1
		}
		totalTokens += tokRes.DeletedCount

		// TODO(Epic 2.x cat_states): users.DeleteOne blocks cascade
		// into cat_states; add `cat_states.DeleteMany({user_id: uid})`
		// once Epic 2 lands the collection.
		// TODO(Epic 3.x friendships): same for friendships + blocks.
		// TODO(Epic 6.x blindbox_drops): same.
		// TODO(Epic 7.x skin_ownership): same.
		// Refresh blacklist: no explicit cleanup — the blacklist is
		// keyed by jti, not userId, and entries TTL-expire naturally
		// within the refresh token's lifetime (see run.go doc).

		userRes, err := users.DeleteOne(ctx, bson.M{"_id": uid})
		if err != nil {
			fmt.Fprintf(args.errOut, "delete user %q: %v\n", uid, err)
			return 1
		}
		totalUsers += userRes.DeletedCount
	}

	summary := runSummary{
		DeletedUsers:      totalUsers,
		DeletedApnsTokens: totalTokens,
		DurationMs:        time.Since(start).Milliseconds(),
		DryRun:            false,
		OlderThanDays:     args.olderThanDays,
	}
	writeJSON(args.out, summary)
	return 0
}

func writeJSON(w io.Writer, v any) {
	enc := json.NewEncoder(w)
	_ = enc.Encode(v)
}

// stripTrailingNewline removes a trailing \n or \r\n if present.
// We deliberately do NOT call strings.TrimSpace — leading / mid
// whitespace should fail the CONFIRM match so ops cannot accidentally
// type "  CONFIRM" and have it pass.
func stripTrailingNewline(s string) string {
	n := len(s)
	if n >= 2 && s[n-2] == '\r' && s[n-1] == '\n' {
		return s[:n-2]
	}
	if n >= 1 && s[n-1] == '\n' {
		return s[:n-1]
	}
	return s
}
