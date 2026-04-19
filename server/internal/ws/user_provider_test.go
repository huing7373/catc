package ws

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/pkg/clockx"
	"github.com/huing/cat/server/pkg/ids"
)

type fakeUserLookup struct {
	got ids.UserID
	out *domain.User
	err error
}

func (f *fakeUserLookup) FindByID(_ context.Context, id ids.UserID) (*domain.User, error) {
	f.got = id
	return f.out, f.err
}

func TestRealUserProvider_HappyPath(t *testing.T) {
	t.Parallel()
	displayName := "kuachan"
	tz := "Asia/Shanghai"
	repo := &fakeUserLookup{out: &domain.User{
		ID:          ids.UserID("u-1"),
		DisplayName: &displayName,
		Timezone:    &tz,
		Preferences: domain.DefaultPreferences(),
	}}
	clk := clockx.NewFakeClock(time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC))
	p := NewRealUserProvider(repo, clk)

	raw, err := p.GetUser(context.Background(), "u-1")
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	assert.Equal(t, "u-1", decoded["id"])
	assert.Equal(t, "kuachan", decoded["displayName"])
	assert.Equal(t, "Asia/Shanghai", decoded["timezone"])
	prefs, ok := decoded["preferences"].(map[string]any)
	require.True(t, ok, "preferences must be a JSON object")
	qh, ok := prefs["quietHours"].(map[string]any)
	require.True(t, ok, "preferences.quietHours must be a JSON object")
	assert.Equal(t, "23:00", qh["start"])
	assert.Equal(t, "07:00", qh["end"])
}

func TestRealUserProvider_OmitsNilOptionalFields(t *testing.T) {
	t.Parallel()
	repo := &fakeUserLookup{out: &domain.User{
		ID:          ids.UserID("u-2"),
		Preferences: domain.DefaultPreferences(),
	}}
	clk := clockx.NewFakeClock(time.Date(2026, 4, 19, 12, 0, 0, 0, time.UTC))
	p := NewRealUserProvider(repo, clk)

	raw, err := p.GetUser(context.Background(), "u-2")
	require.NoError(t, err)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(raw, &decoded))
	_, hasDisplay := decoded["displayName"]
	_, hasTz := decoded["timezone"]
	assert.False(t, hasDisplay, "nil displayName must be omitted")
	assert.False(t, hasTz, "nil timezone must be omitted")
}

func TestRealUserProvider_NotFoundIsError(t *testing.T) {
	t.Parallel()
	repo := &fakeUserLookup{err: repository.ErrUserNotFound}
	p := NewRealUserProvider(repo, clockx.NewRealClock())
	_, err := p.GetUser(context.Background(), "u-missing")
	require.Error(t, err)
	assert.NotErrorIs(t, err, repository.ErrUserNotFound,
		"NotFound is wrapped — caller surfaces as ErrInternalError")
}

func TestRealUserProvider_RepoErrorPropagates(t *testing.T) {
	t.Parallel()
	cause := errors.New("mongo down")
	repo := &fakeUserLookup{err: cause}
	p := NewRealUserProvider(repo, clockx.NewRealClock())
	_, err := p.GetUser(context.Background(), "u-1")
	require.Error(t, err)
	assert.ErrorIs(t, err, cause)
}

func TestRealUserProvider_RejectsEmptyID(t *testing.T) {
	t.Parallel()
	p := NewRealUserProvider(&fakeUserLookup{}, clockx.NewRealClock())
	_, err := p.GetUser(context.Background(), "")
	require.Error(t, err)
}
