package logx

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInit_JSONFormat(t *testing.T) {
	Init(Options{Level: "info", Format: "json", BuildVersion: "v1.0.0", ConfigHash: "abc123"})

	var buf bytes.Buffer
	log.Logger = log.Logger.Output(&buf)
	log.Info().Msg("test")

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m))
	assert.Equal(t, "v1.0.0", m["buildVersion"])
	assert.Equal(t, "abc123", m["configHash"])
	assert.Equal(t, "test", m["message"])
}

func TestInit_ConsoleFormat(t *testing.T) {
	Init(Options{Level: "debug", Format: "console", BuildVersion: "v2.0.0", ConfigHash: "def456"})
	assert.Equal(t, zerolog.DebugLevel, zerolog.GlobalLevel())
}

func TestInit_DefaultLevel(t *testing.T) {
	Init(Options{Level: "", Format: "json"})
	assert.Equal(t, zerolog.InfoLevel, zerolog.GlobalLevel())
}

func TestInit_InvalidLevel(t *testing.T) {
	Init(Options{Level: "bogus", Format: "json"})
	assert.Equal(t, zerolog.InfoLevel, zerolog.GlobalLevel())
}

func TestCtx_InheritsFields(t *testing.T) {
	var buf bytes.Buffer
	logger := zerolog.New(&buf).With().Str("requestId", "req-123").Logger()
	ctx := logger.WithContext(context.Background())

	l := Ctx(ctx)
	l.Info().Msg("hello")

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m))
	assert.Equal(t, "req-123", m["requestId"])
	assert.Equal(t, "hello", m["message"])
}

func TestWithRequestID(t *testing.T) {
	var buf bytes.Buffer
	base := zerolog.New(&buf)
	ctx := base.WithContext(context.Background())

	ctx = WithRequestID(ctx, "rid-abc")
	Ctx(ctx).Info().Msg("with request id")

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m))
	assert.Equal(t, "rid-abc", m["requestId"])
}

func TestWithUserID(t *testing.T) {
	var buf bytes.Buffer
	base := zerolog.New(&buf)
	ctx := base.WithContext(context.Background())

	ctx = WithUserID(ctx, "user-42")
	Ctx(ctx).Info().Msg("with user id")

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m))
	assert.Equal(t, "user-42", m["userId"])
}

func TestWithConnID(t *testing.T) {
	var buf bytes.Buffer
	base := zerolog.New(&buf)
	ctx := base.WithContext(context.Background())

	ctx = WithConnID(ctx, "conn-99")
	Ctx(ctx).Info().Msg("with conn id")

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m))
	assert.Equal(t, "conn-99", m["connId"])
}

func TestWithChainedFields(t *testing.T) {
	var buf bytes.Buffer
	base := zerolog.New(&buf)
	ctx := base.WithContext(context.Background())

	ctx = WithRequestID(ctx, "rid-1")
	ctx = WithUserID(ctx, "uid-2")
	ctx = WithConnID(ctx, "cid-3")
	Ctx(ctx).Info().Msg("chained")

	var m map[string]interface{}
	require.NoError(t, json.Unmarshal(buf.Bytes(), &m))
	assert.Equal(t, "rid-1", m["requestId"])
	assert.Equal(t, "uid-2", m["userId"])
	assert.Equal(t, "cid-3", m["connId"])
}
