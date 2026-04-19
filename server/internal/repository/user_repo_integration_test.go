//go:build integration

package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/internal/testutil"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/ids"
)

// fixedClock returns a deterministic clock pinned to the AC10 reference
// timestamp so updated_at / deletion stamps are byte-stable across runs.
func fixedClock() *clockx.FakeClock {
	return clockx.NewFakeClock(time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC))
}

func TestMongoUserRepo_Integration(t *testing.T) {
	cli, cleanup := testutil.SetupMongo(t)
	defer cleanup()

	clk := fixedClock()
	dbName := "test_user_" + uuid.New().String()
	db := cli.Database(dbName)
	t.Cleanup(func() { _ = db.Drop(context.Background()) })
	repo := repository.NewMongoUserRepository(db, clk)
	ctx := context.Background()

	require.NoError(t, repo.EnsureIndexes(ctx), "EnsureIndexes must succeed first time")

	t.Run("EnsureIndexesIdempotent", func(t *testing.T) {
		require.NoError(t, repo.EnsureIndexes(ctx), "second call must be a no-op")

		cur, err := db.Collection("users").Indexes().List(ctx)
		require.NoError(t, err)
		var names []string
		for cur.Next(ctx) {
			var raw map[string]any
			require.NoError(t, cur.Decode(&raw))
			if name, ok := raw["name"].(string); ok {
				names = append(names, name)
			}
		}
		assert.Contains(t, names, "apple_user_id_hash_1",
			"expected `apple_user_id_hash_1` unique index, got %v", names)
	})

	t.Run("InsertThenFindByAppleHash", func(t *testing.T) {
		u := newSeedUser(clk, "hash:insert-find")

		require.NoError(t, repo.Insert(ctx, u))

		got, err := repo.FindByAppleHash(ctx, "hash:insert-find")
		require.NoError(t, err)
		assert.Equal(t, u.ID, got.ID)
		assert.Equal(t, "hash:insert-find", got.AppleUserIDHash)
		assert.Equal(t, "23:00", got.Preferences.QuietHours.Start)
		assert.Equal(t, "07:00", got.Preferences.QuietHours.End)
		assert.NotNil(t, got.Sessions, "sessions must round-trip as non-nil empty map")
	})

	t.Run("FindByAppleHash_NotFound", func(t *testing.T) {
		_, err := repo.FindByAppleHash(ctx, "hash:does-not-exist")
		assert.ErrorIs(t, err, repository.ErrUserNotFound)
	})

	t.Run("FindByID_RoundTrip", func(t *testing.T) {
		u := newSeedUser(clk, "hash:find-by-id")
		require.NoError(t, repo.Insert(ctx, u))

		got, err := repo.FindByID(ctx, u.ID)
		require.NoError(t, err)
		assert.Equal(t, u.AppleUserIDHash, got.AppleUserIDHash)
	})

	t.Run("FindByID_NotFound", func(t *testing.T) {
		_, err := repo.FindByID(ctx, ids.NewUserID())
		assert.ErrorIs(t, err, repository.ErrUserNotFound)
	})

	t.Run("Insert_DuplicateHash", func(t *testing.T) {
		u1 := newSeedUser(clk, "hash:dup")
		require.NoError(t, repo.Insert(ctx, u1))

		u2 := newSeedUser(clk, "hash:dup") // different ID, same hash
		err := repo.Insert(ctx, u2)
		assert.ErrorIs(t, err, repository.ErrUserDuplicateHash,
			"unique index must reject second insert sharing apple_user_id_hash")
	})

	t.Run("ClearDeletion_Success", func(t *testing.T) {
		u := newSeedUser(clk, "hash:clear-deletion")
		u.DeletionRequested = true
		dt := clk.Now().Add(-24 * time.Hour)
		u.DeletionRequestedAt = &dt
		require.NoError(t, repo.Insert(ctx, u))

		require.NoError(t, repo.ClearDeletion(ctx, u.ID))

		got, err := repo.FindByAppleHash(ctx, "hash:clear-deletion")
		require.NoError(t, err)
		assert.False(t, got.DeletionRequested, "deletion_requested must flip false")
		assert.Nil(t, got.DeletionRequestedAt, "deletion_requested_at must be cleared (unset / nil)")
		assert.Equal(t, clk.Now().UTC(), got.UpdatedAt.UTC(), "updated_at must reflect clock.Now()")
	})

	t.Run("ClearDeletion_NotFound", func(t *testing.T) {
		err := repo.ClearDeletion(ctx, ids.NewUserID())
		assert.ErrorIs(t, err, repository.ErrUserNotFound)
	})

	t.Run("BSONRoundtrip_FullDocument", func(t *testing.T) {
		displayName := "kuachan-rt"
		tz := "Asia/Shanghai"
		stepConsent := true
		u := newSeedUser(clk, "hash:roundtrip-full")
		u.DisplayName = &displayName
		u.Timezone = &tz
		u.FriendCount = 3
		u.Consents.StepData = &stepConsent

		require.NoError(t, repo.Insert(ctx, u))

		got, err := repo.FindByID(ctx, u.ID)
		require.NoError(t, err)
		require.NotNil(t, got.DisplayName)
		assert.Equal(t, "kuachan-rt", *got.DisplayName)
		require.NotNil(t, got.Timezone)
		assert.Equal(t, "Asia/Shanghai", *got.Timezone)
		assert.Equal(t, 3, got.FriendCount)
		require.NotNil(t, got.Consents.StepData)
		assert.True(t, *got.Consents.StepData)
	})
}

func newSeedUser(clk clockx.Clock, hash string) *domain.User {
	now := clk.Now()
	return &domain.User{
		ID:              ids.NewUserID(),
		AppleUserIDHash: hash,
		Preferences:     domain.DefaultPreferences(),
		Sessions:        map[string]domain.Session{},
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}
