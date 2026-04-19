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

	t.Run("UpsertSession_CreateThenFind", func(t *testing.T) {
		u := newSeedUser(clk, "hash:upsert-create")
		require.NoError(t, repo.Insert(ctx, u))
		deviceA := "00000000-0000-4000-8000-0000000000a1"

		s := domain.Session{
			CurrentJTI: "jti-a-1",
			IssuedAt:   clk.Now().Add(-1 * time.Minute),
		}
		require.NoError(t, repo.UpsertSession(ctx, u.ID, deviceA, s))

		got, ok, err := repo.GetSession(ctx, u.ID, deviceA)
		require.NoError(t, err)
		require.True(t, ok, "session must exist after UpsertSession")
		assert.Equal(t, "jti-a-1", got.CurrentJTI)
		assert.Equal(t, s.IssuedAt.UTC(), got.IssuedAt.UTC())
	})

	t.Run("UpsertSession_Overwrite", func(t *testing.T) {
		u := newSeedUser(clk, "hash:upsert-overwrite")
		require.NoError(t, repo.Insert(ctx, u))
		device := "00000000-0000-4000-8000-0000000000b1"

		require.NoError(t, repo.UpsertSession(ctx, u.ID, device, domain.Session{
			CurrentJTI: "jti-1", IssuedAt: clk.Now(),
		}))
		require.NoError(t, repo.UpsertSession(ctx, u.ID, device, domain.Session{
			CurrentJTI: "jti-2", IssuedAt: clk.Now().Add(time.Second),
		}))

		got, ok, err := repo.GetSession(ctx, u.ID, device)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "jti-2", got.CurrentJTI, "rolling rotation: second Upsert replaces first")
	})

	t.Run("UpsertSession_IndependentDevices", func(t *testing.T) {
		u := newSeedUser(clk, "hash:upsert-independent")
		require.NoError(t, repo.Insert(ctx, u))
		watch := "00000000-0000-4000-8000-0000000000c1"
		phone := "00000000-0000-4000-8000-0000000000c2"

		require.NoError(t, repo.UpsertSession(ctx, u.ID, watch, domain.Session{
			CurrentJTI: "jti-watch", IssuedAt: clk.Now(),
		}))
		require.NoError(t, repo.UpsertSession(ctx, u.ID, phone, domain.Session{
			CurrentJTI: "jti-phone", IssuedAt: clk.Now(),
		}))

		ws, ok, err := repo.GetSession(ctx, u.ID, watch)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "jti-watch", ws.CurrentJTI)

		ps, ok, err := repo.GetSession(ctx, u.ID, phone)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "jti-phone", ps.CurrentJTI)

		ids, err := repo.ListDeviceIDs(ctx, u.ID)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{watch, phone}, ids)
	})

	t.Run("GetSession_AbsentDevice", func(t *testing.T) {
		u := newSeedUser(clk, "hash:get-session-absent")
		require.NoError(t, repo.Insert(ctx, u))

		got, ok, err := repo.GetSession(ctx, u.ID, "00000000-0000-4000-8000-0000000000d9")
		require.NoError(t, err, "absent device must NOT be ErrUserNotFound")
		assert.False(t, ok)
		assert.Equal(t, domain.Session{}, got)
	})

	t.Run("GetSession_UserNotFound", func(t *testing.T) {
		_, _, err := repo.GetSession(ctx, ids.NewUserID(), "00000000-0000-4000-8000-0000000000e1")
		assert.ErrorIs(t, err, repository.ErrUserNotFound)
	})

	t.Run("UpsertSession_UserNotFound", func(t *testing.T) {
		err := repo.UpsertSession(ctx, ids.NewUserID(), "00000000-0000-4000-8000-0000000000f1", domain.Session{
			CurrentJTI: "orphan-jti", IssuedAt: clk.Now(),
		})
		assert.ErrorIs(t, err, repository.ErrUserNotFound)
	})

	t.Run("UpsertSession_PreservesOtherFields", func(t *testing.T) {
		displayName := "preserved-user"
		tz := "Asia/Tokyo"
		stepConsent := false
		u := newSeedUser(clk, "hash:upsert-preserve")
		u.DisplayName = &displayName
		u.Timezone = &tz
		u.FriendCount = 5
		u.Consents.StepData = &stepConsent
		require.NoError(t, repo.Insert(ctx, u))

		device := "00000000-0000-4000-8000-0000000000a9"
		require.NoError(t, repo.UpsertSession(ctx, u.ID, device, domain.Session{
			CurrentJTI: "jti-preserve", IssuedAt: clk.Now(),
		}))

		got, err := repo.FindByID(ctx, u.ID)
		require.NoError(t, err)
		require.NotNil(t, got.DisplayName)
		assert.Equal(t, "preserved-user", *got.DisplayName)
		require.NotNil(t, got.Timezone)
		assert.Equal(t, "Asia/Tokyo", *got.Timezone)
		assert.Equal(t, 5, got.FriendCount)
		require.NotNil(t, got.Consents.StepData)
		assert.False(t, *got.Consents.StepData)
		assert.False(t, got.DeletionRequested)
	})

	t.Run("ListDeviceIDs_Empty", func(t *testing.T) {
		u := newSeedUser(clk, "hash:list-empty")
		require.NoError(t, repo.Insert(ctx, u))

		ids, err := repo.ListDeviceIDs(ctx, u.ID)
		require.NoError(t, err)
		assert.Equal(t, []string{}, ids, "no sessions ⇒ non-nil empty slice")
	})

	// Round-1 review P1 (rotation CAS): UpsertSessionIfJTIMatches must
	// win the race only when the expected jti matches, and fail with
	// ErrSessionStale otherwise.
	t.Run("UpsertSessionIfJTIMatches_Succeeds", func(t *testing.T) {
		u := newSeedUser(clk, "hash:cas-ok")
		require.NoError(t, repo.Insert(ctx, u))
		dev := "00000000-0000-4000-8000-0000000000ca"
		require.NoError(t, repo.UpsertSession(ctx, u.ID, dev, domain.Session{
			CurrentJTI: "jti-old", IssuedAt: clk.Now(),
		}))

		// CAS with matching expected jti should win.
		require.NoError(t, repo.UpsertSessionIfJTIMatches(ctx, u.ID, dev, "jti-old", domain.Session{
			CurrentJTI: "jti-new", IssuedAt: clk.Now().Add(time.Second),
		}))
		got, ok, err := repo.GetSession(ctx, u.ID, dev)
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "jti-new", got.CurrentJTI)
	})

	t.Run("UpsertSessionIfJTIMatches_StaleReturnsErr", func(t *testing.T) {
		u := newSeedUser(clk, "hash:cas-stale")
		require.NoError(t, repo.Insert(ctx, u))
		dev := "00000000-0000-4000-8000-0000000000cb"
		require.NoError(t, repo.UpsertSession(ctx, u.ID, dev, domain.Session{
			CurrentJTI: "jti-first", IssuedAt: clk.Now(),
		}))

		// Simulate a race winner rotating the session.
		require.NoError(t, repo.UpsertSessionIfJTIMatches(ctx, u.ID, dev, "jti-first", domain.Session{
			CurrentJTI: "jti-second", IssuedAt: clk.Now(),
		}))

		// The loser (still presenting the old expected jti) must fail.
		err := repo.UpsertSessionIfJTIMatches(ctx, u.ID, dev, "jti-first", domain.Session{
			CurrentJTI: "jti-loser", IssuedAt: clk.Now(),
		})
		require.ErrorIs(t, err, repository.ErrSessionStale)

		got, _, err := repo.GetSession(ctx, u.ID, dev)
		require.NoError(t, err)
		assert.Equal(t, "jti-second", got.CurrentJTI, "winner's jti must remain")
	})

	t.Run("UpsertSessionIfJTIMatches_UserMissing_AsStale", func(t *testing.T) {
		err := repo.UpsertSessionIfJTIMatches(ctx, ids.NewUserID(),
			"00000000-0000-4000-8000-0000000000cc", "jti-whatever",
			domain.Session{CurrentJTI: "new", IssuedAt: clk.Now()})
		assert.ErrorIs(t, err, repository.ErrSessionStale,
			"absent user is conflated with CAS mismatch — service semantic is identical")
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
