//go:build integration

package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/push"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/internal/testutil"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/ids"
	"github.com/huing/cat/server/pkg/mongox"
)

// TestQuietHoursResolver_Integration_EndToEnd drives the full
// RealQuietHoursResolver against real Mongo (Testcontainers) plus the
// MongoUserRepository adapter. It asserts two invariants:
//
//  1. The resolver's `.In(loc)` arithmetic really uses the user's
//     timezone, not the server UTC. (If it didn't, a Shanghai user
//     at UTC 16:00 = local 00:00 and at UTC 00:00 = local 08:00 would
//     yield the same answer — the test locks the split.)
//  2. The resolver reads `preferences.quiet_hours.start/end` from
//     the actual Mongo document, not a hard-coded default.
//
// See Story 1.5 AC11 (second block) for the spec.
func TestQuietHoursResolver_Integration_EndToEnd(t *testing.T) {
	cli, cleanup := testutil.SetupMongo(t)
	t.Cleanup(cleanup)

	dbName := "test_quiet_" + strings.ReplaceAll(uuid.NewString(), "-", "_")
	mongoCli := mongox.WrapClient(cli, dbName)
	t.Cleanup(func() { _ = cli.Database(dbName).Drop(context.Background()) })

	// FakeClock lets us move "now" independent of wall clock.
	clk := clockx.NewFakeClock(time.Date(2026, 4, 20, 16, 0, 0, 0, time.UTC))
	userRepo := repository.NewMongoUserRepository(mongoCli.DB(), clk)
	require.NoError(t, userRepo.EnsureIndexes(context.Background()))

	// Seed a Shanghai user with a 23:00-07:00 quiet window.
	tz := "Asia/Shanghai"
	u := &domain.User{
		ID:              ids.NewUserID(),
		AppleUserIDHash: "hash:quiet-e2e",
		Timezone:        &tz,
		Preferences:     domain.UserPreferences{QuietHours: domain.QuietHours{Start: "23:00", End: "07:00"}},
		Sessions:        map[string]domain.Session{},
		CreatedAt:       clk.Now(),
		UpdatedAt:       clk.Now(),
	}
	require.NoError(t, userRepo.Insert(context.Background(), u))

	resolver := push.NewRealQuietHoursResolver(
		&quietHoursUserLookupAdapter{repo: userRepo}, clk,
	)

	// Phase 1 — clock = UTC 16:00 = Shanghai local 00:00 ⇒ quiet.
	quiet, err := resolver.Resolve(context.Background(), u.ID)
	require.NoError(t, err)
	assert.True(t, quiet, "UTC 16:00 → Shanghai 00:00 MUST be quiet (inside 23:00-07:00)")

	// Phase 2 — advance clock to UTC 00:00 next day = Shanghai 08:00
	// ⇒ not quiet.
	clk.Advance(8 * time.Hour) // now = UTC 2026-04-21 00:00
	quiet, err = resolver.Resolve(context.Background(), u.ID)
	require.NoError(t, err)
	assert.False(t, quiet, "UTC 00:00 → Shanghai 08:00 MUST be not-quiet (outside 23:00-07:00)")

	// Phase 3 — missing user (lookup returns found=false) MUST yield
	// (false, nil) per fail-open contract.
	quiet, err = resolver.Resolve(context.Background(), ids.UserID("nonexistent-user-id"))
	require.NoError(t, err)
	assert.False(t, quiet, "missing user must yield (false, nil) per fail-open contract")
}
