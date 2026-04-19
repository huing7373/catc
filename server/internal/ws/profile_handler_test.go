package ws

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/domain"
	"github.com/huing/cat/server/internal/dto"
	"github.com/huing/cat/server/internal/repository"
	"github.com/huing/cat/server/pkg/ids"
)

// --- fake service ---

type fakeProfileSvc struct {
	calls       int
	lastUser    ids.UserID
	lastInput   ProfileUpdateInput
	returnUser  *domain.User
	returnError error
}

func (f *fakeProfileSvc) Update(_ context.Context, userID ids.UserID, p ProfileUpdateInput) (*domain.User, error) {
	f.calls++
	f.lastUser = userID
	f.lastInput = p
	return f.returnUser, f.returnError
}

func seedUser(id, dn, tz string) *domain.User {
	var dnPtr, tzPtr *string
	if dn != "" {
		dnPtr = &dn
	}
	if tz != "" {
		tzPtr = &tz
	}
	return &domain.User{
		ID:          ids.UserID(id),
		DisplayName: dnPtr,
		Timezone:    tzPtr,
		Preferences: domain.DefaultPreferences(),
	}
}

func newProfileTestClient(userID string) *Client {
	return &Client{
		connID: "conn-1",
		userID: userID,
		send:   make(chan []byte, 16),
		done:   make(chan struct{}),
	}
}

func profileEnv(t *testing.T, id string, body any) Envelope {
	t.Helper()
	payload, err := json.Marshal(body)
	require.NoError(t, err)
	return Envelope{ID: id, Type: "profile.update", Payload: payload}
}

// --- tests ---

func TestProfileHandler_HappyPath(t *testing.T) {
	t.Parallel()
	expected := seedUser("u1", "Alice", "Asia/Shanghai")
	svc := &fakeProfileSvc{returnUser: expected}
	h := NewProfileHandler(svc)

	client := newProfileTestClient("u1")
	env := profileEnv(t, "env-1", map[string]any{
		"displayName": "Alice",
		"timezone":    "Asia/Shanghai",
		"quietHours":  map[string]string{"start": "23:00", "end": "07:00"},
	})

	raw, err := h.HandleUpdate(context.Background(), client, env)
	require.NoError(t, err)

	var resp dto.ProfileUpdateResponse
	require.NoError(t, json.Unmarshal(raw, &resp))
	require.NotNil(t, resp.User.DisplayName)
	assert.Equal(t, "Alice", *resp.User.DisplayName)
	require.NotNil(t, resp.User.Timezone)
	assert.Equal(t, "Asia/Shanghai", *resp.User.Timezone)
	assert.Equal(t, "23:00", resp.User.Preferences.QuietHours.Start)
	assert.Equal(t, "07:00", resp.User.Preferences.QuietHours.End)

	assert.Equal(t, 1, svc.calls)
	assert.Equal(t, ids.UserID("u1"), svc.lastUser)
}

func TestProfileHandler_DisplayNameTrimmedBeforeService(t *testing.T) {
	t.Parallel()
	// The validator accepts " Alice " because trim yields "Alice"; the
	// handler must forward the *trimmed* value to the service so the
	// repo never persists the ambient spaces (Semantic #8).
	svc := &fakeProfileSvc{returnUser: seedUser("u1", "Alice", "")}
	h := NewProfileHandler(svc)
	client := newProfileTestClient("u1")
	env := profileEnv(t, "env-2", map[string]any{"displayName": "  Alice  "})

	_, err := h.HandleUpdate(context.Background(), client, env)
	require.NoError(t, err)
	require.NotNil(t, svc.lastInput.DisplayName)
	assert.Equal(t, "Alice", *svc.lastInput.DisplayName,
		"displayName must be trimmed before hitting the service")
}

func TestProfileHandler_InvalidJSON(t *testing.T) {
	t.Parallel()
	svc := &fakeProfileSvc{}
	h := NewProfileHandler(svc)
	client := newProfileTestClient("u1")

	env := Envelope{ID: "env-3", Type: "profile.update", Payload: json.RawMessage(`not json`)}
	_, err := h.HandleUpdate(context.Background(), client, env)
	require.Error(t, err)
	var ae *dto.AppError
	require.True(t, errors.As(err, &ae))
	assert.Equal(t, "VALIDATION_ERROR", ae.Code)
	assert.Equal(t, 0, svc.calls, "service must not be called on decode failure")
}

func TestProfileHandler_EmptyPayload_AllFieldsNil(t *testing.T) {
	t.Parallel()
	svc := &fakeProfileSvc{}
	h := NewProfileHandler(svc)
	client := newProfileTestClient("u1")

	env := profileEnv(t, "env-4", map[string]any{})
	_, err := h.HandleUpdate(context.Background(), client, env)
	require.Error(t, err)
	var ae *dto.AppError
	require.True(t, errors.As(err, &ae))
	assert.Equal(t, "VALIDATION_ERROR", ae.Code)
	assert.Contains(t, ae.Message, "at least one")
	assert.Equal(t, 0, svc.calls)
}

func TestProfileHandler_DisplayNameWhitespaceOnly(t *testing.T) {
	t.Parallel()
	svc := &fakeProfileSvc{}
	h := NewProfileHandler(svc)
	client := newProfileTestClient("u1")

	env := profileEnv(t, "env-5", map[string]any{"displayName": "   "})
	_, err := h.HandleUpdate(context.Background(), client, env)
	require.Error(t, err)
	assert.Equal(t, "VALIDATION_ERROR", mustAppErr(t, err).Code)
	assert.Equal(t, 0, svc.calls)
}

func TestProfileHandler_DisplayNameControlChar(t *testing.T) {
	t.Parallel()
	svc := &fakeProfileSvc{}
	h := NewProfileHandler(svc)
	client := newProfileTestClient("u1")

	env := profileEnv(t, "env-6", map[string]any{"displayName": "Ali\x00ce"})
	_, err := h.HandleUpdate(context.Background(), client, env)
	require.Error(t, err)
	assert.Equal(t, "VALIDATION_ERROR", mustAppErr(t, err).Code)
	assert.Equal(t, 0, svc.calls)
}

func TestProfileHandler_TimezoneInvalid(t *testing.T) {
	t.Parallel()
	svc := &fakeProfileSvc{}
	h := NewProfileHandler(svc)
	client := newProfileTestClient("u1")

	env := profileEnv(t, "env-7", map[string]any{"timezone": "Pacific/Nope"})
	_, err := h.HandleUpdate(context.Background(), client, env)
	require.Error(t, err)
	ae := mustAppErr(t, err)
	assert.Equal(t, "VALIDATION_ERROR", ae.Code)
	assert.Contains(t, ae.Message, "IANA")
	assert.Equal(t, 0, svc.calls)
}

func TestProfileHandler_QuietHoursNotHHMM(t *testing.T) {
	t.Parallel()
	svc := &fakeProfileSvc{}
	h := NewProfileHandler(svc)
	client := newProfileTestClient("u1")

	env := profileEnv(t, "env-8", map[string]any{
		"quietHours": map[string]string{"start": "25:00", "end": "07:00"},
	})
	_, err := h.HandleUpdate(context.Background(), client, env)
	require.Error(t, err)
	assert.Equal(t, "VALIDATION_ERROR", mustAppErr(t, err).Code)
	assert.Equal(t, 0, svc.calls)
}

func TestProfileHandler_ServiceReturnsUserNotFound_MapsToInternalError(t *testing.T) {
	t.Parallel()
	// Service returns ErrUserNotFound (data-corruption signal: JWT
	// middleware validated userId but Mongo has no matching row). The
	// handler MUST NOT leak existence — it maps this to INTERNAL_ERROR
	// exactly like any other repo error.
	svc := &fakeProfileSvc{returnError: repository.ErrUserNotFound}
	h := NewProfileHandler(svc)
	client := newProfileTestClient("u1")

	env := profileEnv(t, "env-9", map[string]any{"displayName": "Alice"})
	_, err := h.HandleUpdate(context.Background(), client, env)
	require.Error(t, err)
	ae := mustAppErr(t, err)
	assert.Equal(t, "INTERNAL_ERROR", ae.Code,
		"ErrUserNotFound must map to INTERNAL_ERROR — never NOT_FOUND — to avoid user-existence probe")
}

func TestProfileHandler_ServiceReturnsGenericError_MapsToInternalError(t *testing.T) {
	t.Parallel()
	svc := &fakeProfileSvc{returnError: errors.New("mongo io")}
	h := NewProfileHandler(svc)
	client := newProfileTestClient("u1")

	env := profileEnv(t, "env-10", map[string]any{"displayName": "Alice"})
	_, err := h.HandleUpdate(context.Background(), client, env)
	require.Error(t, err)
	ae := mustAppErr(t, err)
	assert.Equal(t, "INTERNAL_ERROR", ae.Code)
}

func TestProfileHandler_NilServicePanics(t *testing.T) {
	t.Parallel()
	assert.Panics(t, func() { NewProfileHandler(nil) })
}

// TestProfileHandler_SingleFieldHappyPath locks the partial-update
// shape end-to-end through the handler so a future refactor of
// ProfileUpdateInput cannot silently drop a single-field request.
func TestProfileHandler_SingleFieldHappyPath(t *testing.T) {
	t.Parallel()
	svc := &fakeProfileSvc{returnUser: seedUser("u1", "", "")}
	h := NewProfileHandler(svc)
	client := newProfileTestClient("u1")

	env := profileEnv(t, "env-11", map[string]any{"timezone": "America/New_York"})
	_, err := h.HandleUpdate(context.Background(), client, env)
	require.NoError(t, err)
	require.NotNil(t, svc.lastInput.Timezone)
	assert.Equal(t, "America/New_York", *svc.lastInput.Timezone)
	assert.Nil(t, svc.lastInput.DisplayName, "displayName must remain nil on single-field input")
	assert.Nil(t, svc.lastInput.QuietHours)
}

// --- helper ---

func mustAppErr(t *testing.T, err error) *dto.AppError {
	t.Helper()
	var ae *dto.AppError
	require.True(t, errors.As(err, &ae), "expected *dto.AppError, got %T: %v", err, err)
	return ae
}
