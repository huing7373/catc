//go:build integration

package repository_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/internal/testutil"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/cryptox"
	"github.com/huing/cat/server/pkg/ids"
)

// fixedApnsClock returns the same AC10 deterministic clock used by
// user_repo_integration_test.go (keeps updated_at comparisons stable).
func fixedApnsClock() *clockx.FakeClock {
	return clockx.NewFakeClock(time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC))
}

func aesKey(b byte) []byte {
	k := make([]byte, 32)
	for i := range k {
		k[i] = b
	}
	return k
}

func mustSealer(t *testing.T, key []byte) *cryptox.AESGCMSealer {
	t.Helper()
	s, err := cryptox.NewAESGCMSealer(key)
	require.NoError(t, err)
	return s
}

// newApnsRepoHarness wires a scratch Mongo db + repo + sealer. The DB
// is dropped on test cleanup so parallel test files can share the same
// shared Mongo container without stepping on each other's collections.
func newApnsRepoHarness(t *testing.T, cli *mongo.Client, key []byte) (*repository.MongoApnsTokenRepository, *mongo.Database) {
	t.Helper()
	dbName := "test_apns_" + uuid.New().String()
	db := cli.Database(dbName)
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	clk := fixedApnsClock()
	sealer := mustSealer(t, key)
	repo := repository.NewMongoApnsTokenRepository(db, clk, sealer)
	require.NoError(t, repo.EnsureIndexes(context.Background()))
	return repo, db
}

func TestApnsTokenRepo_Integration(t *testing.T) {
	cli, cleanup := testutil.SetupMongo(t)
	defer cleanup()

	ctx := context.Background()

	const watchPlain = "a1b2c3d4e5f60102030405060708090a0b0c0d0e0f10111213141516171819"
	const phonePlain = "ff00112233445566778899aabbccddeeff00112233445566778899aabbccdd"

	t.Run("UpsertAndList", func(t *testing.T) {
		repo, _ := newApnsRepoHarness(t, cli, aesKey(0x11))
		u := ids.NewUserID()
		require.NoError(t, repo.Upsert(ctx, &domain.ApnsToken{
			UserID: u, Platform: ids.PlatformWatch,
			DeviceToken: watchPlain, UpdatedAt: time.Now().UTC(),
		}))
		require.NoError(t, repo.Upsert(ctx, &domain.ApnsToken{
			UserID: u, Platform: ids.PlatformIphone,
			DeviceToken: phonePlain, UpdatedAt: time.Now().UTC(),
		}))
		toks, err := repo.ListByUserID(ctx, u)
		require.NoError(t, err)
		require.Len(t, toks, 2)

		seen := map[ids.Platform]string{}
		for _, tk := range toks {
			seen[tk.Platform] = tk.DeviceToken
		}
		assert.Equal(t, watchPlain, seen[ids.PlatformWatch])
		assert.Equal(t, phonePlain, seen[ids.PlatformIphone])
	})

	t.Run("UpsertReplacesSamePlatform", func(t *testing.T) {
		repo, _ := newApnsRepoHarness(t, cli, aesKey(0x22))
		u := ids.NewUserID()
		t0 := time.Date(2026, 4, 19, 10, 0, 0, 0, time.UTC)
		require.NoError(t, repo.Upsert(ctx, &domain.ApnsToken{
			UserID: u, Platform: ids.PlatformWatch,
			DeviceToken: watchPlain, UpdatedAt: t0,
		}))
		t1 := t0.Add(5 * time.Minute)
		const newPlain = "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
		require.NoError(t, repo.Upsert(ctx, &domain.ApnsToken{
			UserID: u, Platform: ids.PlatformWatch,
			DeviceToken: newPlain, UpdatedAt: t1,
		}))

		toks, err := repo.ListByUserID(ctx, u)
		require.NoError(t, err)
		require.Len(t, toks, 1, "re-register same platform must overwrite, not duplicate")
		assert.Equal(t, newPlain, toks[0].DeviceToken)
		assert.True(t, toks[0].UpdatedAt.Equal(t1))
	})

	t.Run("UpsertCrossPlatformCoexists", func(t *testing.T) {
		repo, _ := newApnsRepoHarness(t, cli, aesKey(0x33))
		u := ids.NewUserID()
		now := time.Now().UTC()
		require.NoError(t, repo.Upsert(ctx, &domain.ApnsToken{
			UserID: u, Platform: ids.PlatformWatch,
			DeviceToken: watchPlain, UpdatedAt: now,
		}))
		require.NoError(t, repo.Upsert(ctx, &domain.ApnsToken{
			UserID: u, Platform: ids.PlatformIphone,
			DeviceToken: phonePlain, UpdatedAt: now,
		}))
		toks, err := repo.ListByUserID(ctx, u)
		require.NoError(t, err)
		assert.Len(t, toks, 2, "unique index on (user_id, platform) must allow cross-platform rows")
	})

	t.Run("AtRestEncrypted", func(t *testing.T) {
		key := aesKey(0x44)
		repo, db := newApnsRepoHarness(t, cli, key)
		u := ids.NewUserID()
		require.NoError(t, repo.Upsert(ctx, &domain.ApnsToken{
			UserID: u, Platform: ids.PlatformWatch,
			DeviceToken: watchPlain, UpdatedAt: time.Now().UTC(),
		}))

		// Bypass the repo and read the raw BSON — device_token must be
		// sealed bytes, never the plaintext string.
		var raw bson.M
		err := db.Collection("apns_tokens").FindOne(ctx, bson.M{"user_id": string(u)}).Decode(&raw)
		require.NoError(t, err)
		rawField, ok := raw["device_token"]
		require.True(t, ok)
		// mongo-driver/v2 decodes BSON binary as bson.Binary.
		bin, ok := rawField.(bson.Binary)
		require.True(t, ok, "device_token must be bson.Binary, got %T", rawField)
		assert.NotEqual(t, watchPlain, string(bin.Data),
			"device_token must be encrypted at rest")

		// A sibling sealer with the same key must round-trip.
		sealer := mustSealer(t, key)
		pt, err := sealer.Open(bin.Data)
		require.NoError(t, err)
		assert.Equal(t, watchPlain, string(pt))
	})

	t.Run("WrongKeyRejectsOpen_ListSkipsRow", func(t *testing.T) {
		// Write with key A, then construct a second repo with key B
		// pointing at the same collection. Every Open fails with
		// ErrCipherTampered → ListByUserID returns 0 rows + warn log.
		keyA := aesKey(0x55)
		keyB := aesKey(0x66)
		repoA, db := newApnsRepoHarness(t, cli, keyA)
		u := ids.NewUserID()
		require.NoError(t, repoA.Upsert(ctx, &domain.ApnsToken{
			UserID: u, Platform: ids.PlatformWatch,
			DeviceToken: watchPlain, UpdatedAt: time.Now().UTC(),
		}))

		// A repo with a different key against the same collection.
		repoB := repository.NewMongoApnsTokenRepository(db, fixedApnsClock(), mustSealer(t, keyB))
		got, err := repoB.ListByUserID(ctx, u)
		require.NoError(t, err, "tampered rows must be skipped, not errored")
		assert.Empty(t, got)

		// DeleteExpired does NOT decrypt → should still delete the row.
		n, err := repoB.DeleteExpired(ctx, time.Now().Add(10*time.Minute))
		require.NoError(t, err)
		assert.Equal(t, int64(1), n)
	})

	t.Run("DeleteByPlaintextToken", func(t *testing.T) {
		repo, _ := newApnsRepoHarness(t, cli, aesKey(0x77))
		u := ids.NewUserID()
		now := time.Now().UTC()
		require.NoError(t, repo.Upsert(ctx, &domain.ApnsToken{
			UserID: u, Platform: ids.PlatformWatch,
			DeviceToken: watchPlain, UpdatedAt: now,
		}))
		require.NoError(t, repo.Upsert(ctx, &domain.ApnsToken{
			UserID: u, Platform: ids.PlatformIphone,
			DeviceToken: phonePlain, UpdatedAt: now,
		}))

		require.NoError(t, repo.Delete(ctx, u, watchPlain))
		toks, err := repo.ListByUserID(ctx, u)
		require.NoError(t, err)
		require.Len(t, toks, 1)
		assert.Equal(t, ids.PlatformIphone, toks[0].Platform)
	})

	t.Run("DeleteMissingIsNoop", func(t *testing.T) {
		repo, _ := newApnsRepoHarness(t, cli, aesKey(0x88))
		u := ids.NewUserID()
		// unknown user → nil
		assert.NoError(t, repo.Delete(ctx, ids.NewUserID(), watchPlain))
		// known user but token not registered → nil
		require.NoError(t, repo.Upsert(ctx, &domain.ApnsToken{
			UserID: u, Platform: ids.PlatformWatch,
			DeviceToken: watchPlain, UpdatedAt: time.Now().UTC(),
		}))
		assert.NoError(t, repo.Delete(ctx, u, phonePlain))
	})

	t.Run("Delete_SkipsTamperedRowDeletesValidSibling", func(t *testing.T) {
		// Seed two rows for the same user:
		//   - row A (watch) sealed with keyA  — "valid"
		//   - row B (iphone) sealed with keyB — "tampered" from keyA's view
		// Delete(userID, plaintextA) must:
		//   (a) not error when it encounters row B's cipher-tampered state,
		//   (b) log warn + skip it,
		//   (c) still delete row A.
		keyA := aesKey(0xAA)
		keyB := aesKey(0xBB)
		repoA, db := newApnsRepoHarness(t, cli, keyA)
		sealerB := mustSealer(t, keyB)
		u := ids.NewUserID()

		// Row A via normal repo.
		require.NoError(t, repoA.Upsert(ctx, &domain.ApnsToken{
			UserID: u, Platform: ids.PlatformWatch,
			DeviceToken: watchPlain, UpdatedAt: time.Now().UTC(),
		}))
		// Row B — bypass repoA's sealer and insert sealed-with-B bytes
		// directly so ListByUserID / Delete see a tampered ciphertext.
		sealedB, err := sealerB.Seal([]byte(phonePlain))
		require.NoError(t, err)
		_, err = db.Collection("apns_tokens").InsertOne(ctx, bson.M{
			"user_id":      string(u),
			"platform":     string(ids.PlatformIphone),
			"device_token": sealedB,
			"updated_at":   time.Now().UTC(),
		})
		require.NoError(t, err)

		require.NoError(t, repoA.Delete(ctx, u, watchPlain),
			"tampered sibling must NOT block deletion of valid row A")

		// List via repoA: row A deleted, row B tampered → skipped, so 0.
		gotA, err := repoA.ListByUserID(ctx, u)
		require.NoError(t, err)
		assert.Empty(t, gotA)

		// Raw count in Mongo: row B should still be there.
		n, err := db.Collection("apns_tokens").CountDocuments(ctx, bson.M{"user_id": string(u)})
		require.NoError(t, err)
		assert.Equal(t, int64(1), n, "row B (tampered) must survive — only row A was matched for deletion")
	})

	t.Run("DeleteExpired", func(t *testing.T) {
		repo, _ := newApnsRepoHarness(t, cli, aesKey(0x99))
		u := ids.NewUserID()
		old := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
		fresh := time.Date(2026, 4, 15, 0, 0, 0, 0, time.UTC)
		require.NoError(t, repo.Upsert(ctx, &domain.ApnsToken{
			UserID: u, Platform: ids.PlatformWatch, DeviceToken: watchPlain, UpdatedAt: old,
		}))
		require.NoError(t, repo.Upsert(ctx, &domain.ApnsToken{
			UserID: ids.NewUserID(), Platform: ids.PlatformIphone, DeviceToken: phonePlain, UpdatedAt: fresh,
		}))
		require.NoError(t, repo.Upsert(ctx, &domain.ApnsToken{
			UserID: ids.NewUserID(), Platform: ids.PlatformIphone, DeviceToken: phonePlain, UpdatedAt: fresh,
		}))

		cutoff := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
		n, err := repo.DeleteExpired(ctx, cutoff)
		require.NoError(t, err)
		assert.Equal(t, int64(1), n)
	})

	t.Run("EnsureIndexes_Idempotent", func(t *testing.T) {
		repo, db := newApnsRepoHarness(t, cli, aesKey(0x01))
		// newApnsRepoHarness already called EnsureIndexes once; a second
		// call must be a no-op (Mongo returns success when the index
		// already exists with the same spec).
		require.NoError(t, repo.EnsureIndexes(ctx))

		cur, err := db.Collection("apns_tokens").Indexes().List(ctx)
		require.NoError(t, err)
		var saw bool
		for cur.Next(ctx) {
			var raw map[string]any
			require.NoError(t, cur.Decode(&raw))
			if name, _ := raw["name"].(string); name == "user_id_1_platform_1" {
				saw = true
				assert.Equal(t, true, raw["unique"])
			}
		}
		assert.True(t, saw, "expected user_id_1_platform_1 unique index present")
	})

	t.Run("UniqueConstraint_RaceSafe", func(t *testing.T) {
		repo, _ := newApnsRepoHarness(t, cli, aesKey(0x02))
		u := ids.NewUserID()
		const tokA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		const tokB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, tok := range []string{tokA, tokB} {
			wg.Add(1)
			go func(dt string) {
				defer wg.Done()
				errs <- repo.Upsert(ctx, &domain.ApnsToken{
					UserID: u, Platform: ids.PlatformWatch,
					DeviceToken: dt, UpdatedAt: time.Now().UTC(),
				})
			}(tok)
		}
		wg.Wait()
		close(errs)
		for err := range errs {
			require.NoError(t, err, "upsert must be idempotent-safe under concurrency (unique index allows overwrite)")
		}

		toks, err := repo.ListByUserID(ctx, u)
		require.NoError(t, err)
		require.Len(t, toks, 1, "concurrent upserts must coalesce into one row")
		assert.Contains(t, []string{tokA, tokB}, toks[0].DeviceToken)
	})
}
