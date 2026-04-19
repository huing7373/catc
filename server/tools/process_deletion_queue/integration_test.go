//go:build integration

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/huing/cat/server/internal/testutil"
)

// setupToolTestDB spins a real Mongo container and returns a scoped
// Database plus a miniredis-backed client so the tool's full
// signature is satisfied. Tests drive the tool through run() directly.
func setupToolTestDB(t *testing.T) (db *mongo.Database, redisCli redis.Cmdable) {
	t.Helper()
	cli, cleanup := testutil.SetupMongo(t)
	t.Cleanup(cleanup)

	dbName := "test_pdq_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	t.Cleanup(func() { _ = cli.Database(dbName).Drop(context.Background()) })

	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })

	return cli.Database(dbName), rc
}

// seedUser inserts one users-doc with the given id + deletion stamp.
func seedUser(t *testing.T, db *mongo.Database, userID string, deletionReq bool, stamp time.Time) {
	t.Helper()
	_, err := db.Collection("users").InsertOne(context.Background(), bson.M{
		"_id":                   userID,
		"apple_user_id_hash":    "hash:" + userID,
		"deletion_requested":    deletionReq,
		"deletion_requested_at": stamp,
		"sessions":              bson.M{},
		"created_at":            stamp,
		"updated_at":            stamp,
	})
	require.NoError(t, err)
}

// seedApnsToken inserts an apns_tokens row for userID so cascade can
// find something to delete.
func seedApnsToken(t *testing.T, db *mongo.Database, userID, platform string) {
	t.Helper()
	_, err := db.Collection("apns_tokens").InsertOne(context.Background(), bson.M{
		"_id":          uuid.NewString(),
		"user_id":      userID,
		"platform":     platform,
		"device_token": []byte("opaque"),
		"created_at":   time.Now(),
		"updated_at":   time.Now(),
	})
	require.NoError(t, err)
}

// runTool invokes run() with CONFIRM stdin injected + a fixed clock.
func runTool(t *testing.T, db *mongo.Database, rc redis.Cmdable, now time.Time, dryRun bool, olderThanDays, limit int) (stdout, stderr string, code int) {
	t.Helper()
	in := strings.NewReader("CONFIRM\n")
	var out, errOut bytes.Buffer
	code = run(runArgs{
		in:            in,
		out:           &out,
		errOut:        &errOut,
		db:            db,
		redis:         rc,
		dryRun:        dryRun,
		olderThanDays: olderThanDays,
		limit:         limit,
		clockNow:      func() time.Time { return now },
	})
	return out.String(), errOut.String(), code
}

func parseSummary(t *testing.T, stdout string) runSummary {
	t.Helper()
	var s runSummary
	require.NoError(t, json.Unmarshal([]byte(strings.TrimSpace(stdout)), &s))
	return s
}

func TestRunIntegration_DryRun_DoesNotWrite(t *testing.T) {
	db, rc := setupToolTestDB(t)

	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	// Two users past 30-day grace.
	seedUser(t, db, "u1", true, now.Add(-40*24*time.Hour))
	seedUser(t, db, "u2", true, now.Add(-35*24*time.Hour))

	stdout, stderr, code := runTool(t, db, rc, now, true /* dryRun */, 30, 100)
	require.Equal(t, 0, code, "dry run must succeed; stderr=%s", stderr)

	s := parseSummary(t, stdout)
	assert.True(t, s.DryRun)
	assert.Equal(t, int64(2), s.DeletedUsers, "dry-run summary reports CANDIDATES in DeletedUsers field")
	assert.Equal(t, int64(0), s.DeletedApnsTokens, "dry-run must not delete tokens")

	// Mongo unchanged.
	ctx := context.Background()
	n, err := db.Collection("users").CountDocuments(ctx, bson.M{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), n, "dry-run must not delete users")
}

func TestRunIntegration_DeletesExpiredUsersAndApnsTokens(t *testing.T) {
	db, rc := setupToolTestDB(t)

	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	seedUser(t, db, "expired-1", true, now.Add(-40*24*time.Hour))
	seedUser(t, db, "expired-2", true, now.Add(-60*24*time.Hour))
	seedUser(t, db, "within-grace", true, now.Add(-20*24*time.Hour))
	seedUser(t, db, "not-requested", false, now)

	seedApnsToken(t, db, "expired-1", "watch")
	seedApnsToken(t, db, "expired-1", "iphone")
	seedApnsToken(t, db, "expired-2", "watch")
	seedApnsToken(t, db, "within-grace", "watch")   // must survive
	seedApnsToken(t, db, "not-requested", "iphone") // must survive

	stdout, stderr, code := runTool(t, db, rc, now, false, 30, 100)
	require.Equal(t, 0, code, "real run must succeed; stderr=%s", stderr)

	s := parseSummary(t, stdout)
	assert.False(t, s.DryRun)
	assert.Equal(t, int64(2), s.DeletedUsers)
	assert.Equal(t, int64(3), s.DeletedApnsTokens,
		"§21.8 #10 lock: cascade MUST delete apns_tokens rows too (3 = 2 for expired-1 + 1 for expired-2)")

	// Remaining users
	ctx := context.Background()
	var doc bson.M
	err := db.Collection("users").FindOne(ctx, bson.M{"_id": "within-grace"}).Decode(&doc)
	require.NoError(t, err)
	err = db.Collection("users").FindOne(ctx, bson.M{"_id": "not-requested"}).Decode(&doc)
	require.NoError(t, err)

	// Expired users gone
	err = db.Collection("users").FindOne(ctx, bson.M{"_id": "expired-1"}).Decode(&doc)
	assert.Error(t, err)
	err = db.Collection("users").FindOne(ctx, bson.M{"_id": "expired-2"}).Decode(&doc)
	assert.Error(t, err)

	// Surviving apns_tokens — 2 (within-grace + not-requested).
	n, err := db.Collection("apns_tokens").CountDocuments(ctx, bson.M{})
	require.NoError(t, err)
	assert.Equal(t, int64(2), n, "cascade deleted exactly the expired users' tokens")
}

func TestRunIntegration_RespectsLimit(t *testing.T) {
	db, rc := setupToolTestDB(t)

	now := time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC)
	for i := 0; i < 5; i++ {
		seedUser(t, db, "exp-"+strings.Repeat("x", i+1), true, now.Add(-40*24*time.Hour))
	}

	stdout, stderr, code := runTool(t, db, rc, now, false, 30, 2)
	require.Equal(t, 0, code, "stderr=%s", stderr)

	s := parseSummary(t, stdout)
	assert.Equal(t, int64(2), s.DeletedUsers, "safety cap — limit=2 must delete exactly 2")

	ctx := context.Background()
	n, err := db.Collection("users").CountDocuments(ctx, bson.M{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), n, "5 seeded - 2 deleted = 3 remaining")
}
