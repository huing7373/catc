package repository

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/pkg/ids"
)

// TestUser_BSONRoundtrip is the unit-level guard for the snake_case
// schema contract: marshal a fully-populated User to BSON, unmarshal
// back, assert byte-for-byte field equality. A regression here (e.g.
// dropping the `bson:"_id"` tag, renaming display_name) would break
// the live `users` collection silently — this test catches it before
// integration tests run.
func TestUser_BSONRoundtrip(t *testing.T) {
	t.Parallel()

	displayName := "kuachan"
	tz := "Asia/Shanghai"
	stepConsent := true
	deleteRequestedAt := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)

	original := domain.User{
		ID:              ids.NewUserID(),
		AppleUserIDHash: "a1b2c3d4",
		DisplayName:     &displayName,
		Timezone:        &tz,
		Preferences:     domain.DefaultPreferences(),
		FriendCount:     7,
		Consents:        domain.UserConsents{StepData: &stepConsent},
		Sessions: map[string]domain.Session{
			"dev-1": {
				CurrentJTI:   "jti-1",
				IssuedAt:     time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC),
				HasApnsToken: true,
			},
		},
		DeletionRequested:   true,
		DeletionRequestedAt: &deleteRequestedAt,
		CreatedAt:           time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC),
		UpdatedAt:           time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC),
	}

	raw, err := bson.Marshal(original)
	require.NoError(t, err)

	var decoded domain.User
	require.NoError(t, bson.Unmarshal(raw, &decoded))

	assert.Equal(t, original.ID, decoded.ID)
	assert.Equal(t, original.AppleUserIDHash, decoded.AppleUserIDHash)
	require.NotNil(t, decoded.DisplayName)
	assert.Equal(t, displayName, *decoded.DisplayName)
	require.NotNil(t, decoded.Timezone)
	assert.Equal(t, tz, *decoded.Timezone)
	assert.Equal(t, "23:00", decoded.Preferences.QuietHours.Start)
	assert.Equal(t, "07:00", decoded.Preferences.QuietHours.End)
	assert.Equal(t, 7, decoded.FriendCount)
	require.NotNil(t, decoded.Consents.StepData)
	assert.True(t, *decoded.Consents.StepData)
	require.Contains(t, decoded.Sessions, "dev-1")
	assert.Equal(t, "jti-1", decoded.Sessions["dev-1"].CurrentJTI)
	assert.True(t, decoded.Sessions["dev-1"].HasApnsToken)
	assert.True(t, decoded.DeletionRequested)
	require.NotNil(t, decoded.DeletionRequestedAt)
	assert.Equal(t, deleteRequestedAt.UTC(), decoded.DeletionRequestedAt.UTC())
	assert.Equal(t, original.CreatedAt.UTC(), decoded.CreatedAt.UTC())
}

// TestUser_BSONFieldNames asserts the on-the-wire field names are
// exactly snake_case (architecture §P1). Catches a future contributor
// adding camelCase or omitting tags.
func TestUser_BSONFieldNames(t *testing.T) {
	t.Parallel()

	u := domain.User{
		ID:              "u1",
		AppleUserIDHash: "h",
		Preferences:     domain.DefaultPreferences(),
		Sessions:        map[string]domain.Session{},
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}
	raw, err := bson.Marshal(u)
	require.NoError(t, err)

	var asMap bson.M
	require.NoError(t, bson.Unmarshal(raw, &asMap))

	for _, name := range []string{
		"_id", "apple_user_id_hash", "display_name", "timezone",
		"preferences", "friend_count", "consents", "sessions",
		"deletion_requested", "created_at", "updated_at",
	} {
		_, present := asMap[name]
		assert.Truef(t, present, "expected snake_case field %q in BSON output (got %v)", name, asMap)
	}
	for _, name := range []string{"ID", "AppleUserIDHash", "DisplayName", "Timezone"} {
		_, present := asMap[name]
		assert.Falsef(t, present, "BSON output must NOT contain Go field name %q", name)
	}
	prefs := asMap["preferences"]
	hasQuiet := false
	switch v := prefs.(type) {
	case bson.M:
		_, hasQuiet = v["quiet_hours"]
	case bson.D:
		for _, e := range v {
			if e.Key == "quiet_hours" {
				hasQuiet = true
			}
		}
	}
	assert.True(t, hasQuiet, "preferences must contain quiet_hours subdocument (got %T %v)", prefs, prefs)
}
