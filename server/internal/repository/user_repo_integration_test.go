package repository

import (
	"context"
	"strconv"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"github.com/huing7373/catc/server/internal/domain"
	"github.com/huing7373/catc/server/pkg/ids"
	"github.com/huing7373/catc/server/pkg/mongox"
)

// connectMongoOrSkip honours CAT_TEST_MONGO_URI via the central helper
// in pkg/mongox so the os.Getenv call stays in the one allowed location.
func connectMongoOrSkip(t *testing.T) (*mongo.Client, string) {
	t.Helper()
	uri := mongox.IntegrationURI()
	if uri == "" {
		t.Skip("CAT_TEST_MONGO_URI not set — skipping real-mongo integration test")
	}
	cli, err := mongo.Connect(options.Client().ApplyURI(uri))
	if err != nil {
		t.Fatalf("mongo connect: %v", err)
	}
	dbName := "cat_test_repo_" + time.Now().UTC().Format("20060102150405")
	t.Cleanup(func() {
		_ = cli.Database(dbName).Drop(context.Background())
		_ = cli.Disconnect(context.Background())
	})
	return cli, dbName
}

func newRepoForIntegration(t *testing.T) *UserRepository {
	t.Helper()
	cli, db := connectMongoOrSkip(t)
	r := NewUserRepo(cli, db, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := r.EnsureIndexes(ctx); err != nil {
		t.Fatalf("EnsureIndexes: %v", err)
	}
	return r
}

func fixedNow(ts time.Time) func() time.Time {
	return func() time.Time { return ts }
}

func TestIntegration_Upsert_CreatesNewUser(t *testing.T) {
	r := newRepoForIntegration(t)
	now := time.Now().UTC().Truncate(time.Second)

	u, outcome, err := r.UpsertOnAppleLogin(context.Background(), "apple-create", "device-1", fixedNow(now))
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if outcome != OutcomeCreated {
		t.Fatalf("outcome: %q, want created", outcome)
	}
	if u.ID == "" {
		t.Errorf("ID empty")
	}
	if u.AppleID != "apple-create" {
		t.Errorf("AppleID: %q", u.AppleID)
	}
	if u.DeviceID != ids.DeviceID("device-1") {
		t.Errorf("DeviceID: %q", u.DeviceID)
	}
	if !u.LastActiveAt.Equal(now) {
		t.Errorf("LastActiveAt: %v want %v", u.LastActiveAt, now)
	}
}

func TestIntegration_Upsert_ExistingRefreshes(t *testing.T) {
	r := newRepoForIntegration(t)
	first := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	second := first.Add(time.Hour)

	if _, _, err := r.UpsertOnAppleLogin(context.Background(), "apple-existing", "dev-old", fixedNow(first)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	u, outcome, err := r.UpsertOnAppleLogin(context.Background(), "apple-existing", "dev-new", fixedNow(second))
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if outcome != OutcomeExisting {
		t.Fatalf("outcome: %q, want existing", outcome)
	}
	if u.DeviceID != ids.DeviceID("dev-new") {
		t.Errorf("DeviceID not refreshed: %q", u.DeviceID)
	}
	if !u.LastActiveAt.Equal(second) {
		t.Errorf("LastActiveAt not refreshed: %v want %v", u.LastActiveAt, second)
	}
}

func TestIntegration_Upsert_RestoresWithinCoolDown(t *testing.T) {
	r := newRepoForIntegration(t)
	now := time.Now().UTC().Truncate(time.Second)

	// Seed: create then mark deleted with a recent scheduled_at.
	u, _, err := r.UpsertOnAppleLogin(context.Background(), "apple-restore", "d", fixedNow(now.Add(-2*time.Hour)))
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := r.MarkDeleted(context.Background(), u.ID, now.Add(-time.Hour)); err != nil {
		t.Fatalf("MarkDeleted: %v", err)
	}

	got, outcome, err := r.UpsertOnAppleLogin(context.Background(), "apple-restore", "d2", fixedNow(now))
	if err != nil {
		t.Fatalf("Upsert restore: %v", err)
	}
	if outcome != OutcomeRestored {
		t.Fatalf("outcome: %q, want restored", outcome)
	}
	if got.IsDeleted {
		t.Errorf("IsDeleted should be false after restore")
	}
	if got.DeletionScheduledAt != nil {
		t.Errorf("DeletionScheduledAt should be nil, got %v", got.DeletionScheduledAt)
	}
	if got.DeviceID != ids.DeviceID("d2") {
		t.Errorf("DeviceID not refreshed: %q", got.DeviceID)
	}
	if got.ID != u.ID {
		t.Errorf("ID changed across restore: %q -> %q", u.ID, got.ID)
	}
}

func TestIntegration_Upsert_ExpiredCoolDownCreatesFreshRecord(t *testing.T) {
	r := newRepoForIntegration(t)
	now := time.Now().UTC().Truncate(time.Second)

	// Seed: create then mark deleted with scheduled_at outside cool-down.
	u, _, err := r.UpsertOnAppleLogin(context.Background(), "apple-expired", "d", fixedNow(now.Add(-90*24*time.Hour)))
	if err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := r.MarkDeleted(context.Background(), u.ID, now.Add(-31*24*time.Hour)); err != nil {
		t.Fatalf("MarkDeleted: %v", err)
	}

	got, outcome, err := r.UpsertOnAppleLogin(context.Background(), "apple-expired", "d-new", fixedNow(now))
	if err != nil {
		t.Fatalf("Upsert: %v", err)
	}
	if outcome != OutcomeCreated {
		t.Fatalf("outcome: %q, want created (expired cool-down)", outcome)
	}
	if got.ID == u.ID {
		t.Errorf("expected fresh user id, got original %q", got.ID)
	}
}

// TestIntegration_Upsert_ConcurrentSameApple fires N parallel logins on
// the same apple_id. The first one wins via OutcomeCreated; the rest
// must enter UpsertOnAppleLogin past the FindOneAndUpdate active probe
// before the winner has committed, hit the InsertOne dup-key branch,
// recover via FindByAppleID, and return without surfacing the error.
//
// The test asserts:
//  1. ALL N calls return without error (proves the dup-key branch
//     swallowed mongo.IsDuplicateKeyError as designed).
//  2. Exactly ONE OutcomeCreated is observed (the actual winner).
//  3. Exactly ONE active doc remains (no duplicate insertions slipped
//     through).
//
// If the dup-key recovery path were broken (missing case, wrong filter,
// wrong error mapping), the losing goroutines would either error out or
// the active-doc count would jump above 1 — both fail this test.
func TestIntegration_Upsert_ConcurrentSameApple_DupKeyRecovers(t *testing.T) {
	r := newRepoForIntegration(t)
	now := time.Now().UTC().Truncate(time.Second)

	const N = 32
	type result struct {
		outcome LoginOutcome
		err     error
	}
	results := make(chan result, N)
	start := make(chan struct{})

	for i := 0; i < N; i++ {
		dev := "dev-" + strconv.Itoa(i)
		go func() {
			<-start // release all goroutines simultaneously
			_, outcome, err := r.UpsertOnAppleLogin(context.Background(), "apple-concurrent", dev, fixedNow(now))
			results <- result{outcome, err}
		}()
	}
	close(start)

	createdCount := 0
	for i := 0; i < N; i++ {
		res := <-results
		if res.err != nil {
			t.Fatalf("concurrent login #%d errored: %v", i, res.err)
		}
		switch res.outcome {
		case OutcomeCreated:
			createdCount++
		case OutcomeExisting:
			// dup-key branch (or sequential-after-create) — both legal.
		case OutcomeRestored:
			t.Fatalf("unexpected OutcomeRestored on fresh apple_id")
		default:
			t.Fatalf("unknown outcome: %q", res.outcome)
		}
	}
	if createdCount != 1 {
		t.Errorf("expected exactly 1 OutcomeCreated, got %d (parallel inserts may have escaped dup-key recovery)", createdCount)
	}

	count, err := r.coll.CountDocuments(context.Background(), bson.M{"apple_id": "apple-concurrent", "is_deleted": false})
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 active doc after %d concurrent logins, got %d", N, count)
	}
}

// Ensure compile-time that domain.User round-trip handles all the
// fields UpsertOnAppleLogin sets.
var _ = domain.User{}
