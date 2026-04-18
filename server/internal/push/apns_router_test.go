package push

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/pkg/ids"
)

type fakeTokenProvider struct {
	tokens []TokenInfo
	err    error
}

func (f *fakeTokenProvider) ListTokens(context.Context, ids.UserID) ([]TokenInfo, error) {
	return f.tokens, f.err
}

func TestRouteTokens_WatchGoesToWatchTopic(t *testing.T) {
	t.Parallel()
	p := &fakeTokenProvider{tokens: []TokenInfo{
		{Platform: "watch", DeviceToken: "wt-1"},
	}}
	r := NewAPNsRouter(p, "bundle.app.watchkitapp", "bundle.app")
	got, err := r.RouteTokens(context.Background(), "u1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "bundle.app.watchkitapp", got[0].Topic)
	assert.Equal(t, "wt-1", got[0].DeviceToken)
	assert.Equal(t, "watch", got[0].Platform)
}

func TestRouteTokens_IphoneGoesToIphoneTopic(t *testing.T) {
	t.Parallel()
	p := &fakeTokenProvider{tokens: []TokenInfo{
		{Platform: "iphone", DeviceToken: "ip-1"},
	}}
	r := NewAPNsRouter(p, "bundle.app.watchkitapp", "bundle.app")
	got, err := r.RouteTokens(context.Background(), "u1")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "bundle.app", got[0].Topic)
	assert.Equal(t, "iphone", got[0].Platform)
}

func TestRouteTokens_MixedPlatforms(t *testing.T) {
	t.Parallel()
	p := &fakeTokenProvider{tokens: []TokenInfo{
		{Platform: "watch", DeviceToken: "wt-1"},
		{Platform: "iphone", DeviceToken: "ip-1"},
	}}
	r := NewAPNsRouter(p, "bundle.app.watchkitapp", "bundle.app")
	got, err := r.RouteTokens(context.Background(), "u1")
	require.NoError(t, err)
	require.Len(t, got, 2)
	topics := map[string]string{got[0].Platform: got[0].Topic, got[1].Platform: got[1].Topic}
	assert.Equal(t, "bundle.app.watchkitapp", topics["watch"])
	assert.Equal(t, "bundle.app", topics["iphone"])
}

func TestRouteTokens_UnknownPlatformSkipped(t *testing.T) {
	t.Parallel()
	p := &fakeTokenProvider{tokens: []TokenInfo{
		{Platform: "android", DeviceToken: "a-1"},
		{Platform: "watch", DeviceToken: "wt-1"},
	}}
	r := NewAPNsRouter(p, "bundle.app.watchkitapp", "bundle.app")
	got, err := r.RouteTokens(context.Background(), "u1")
	require.NoError(t, err)
	require.Len(t, got, 1, "unknown platform entries must be skipped")
	assert.Equal(t, "watch", got[0].Platform)
}

func TestRouteTokens_EmptyProviderReturnsEmpty(t *testing.T) {
	t.Parallel()
	r := NewAPNsRouter(EmptyTokenProvider{}, "x", "y")
	got, err := r.RouteTokens(context.Background(), "u1")
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestRouteTokens_ProviderError_Bubbles(t *testing.T) {
	t.Parallel()
	p := &fakeTokenProvider{err: errors.New("mongo down")}
	r := NewAPNsRouter(p, "x", "y")
	got, err := r.RouteTokens(context.Background(), "u1")
	assert.Error(t, err)
	assert.Nil(t, got)
}
