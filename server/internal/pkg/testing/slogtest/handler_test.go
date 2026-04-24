package slogtest_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/huing/cat/server/internal/pkg/testing/slogtest"
)

// TestHandler_CaptureAndQuery 验证 Handler 能捕获日志并按 key 取值。
func TestHandler_CaptureAndQuery(t *testing.T) {
	h := slogtest.NewHandler(slog.LevelDebug)
	logger := slog.New(h)

	ctx := context.Background()
	logger.InfoContext(ctx, "http_request",
		slog.String("request_id", "rid-1"),
		slog.String("api_path", "/ping"),
		slog.Int64("latency_ms", 3),
	)

	records := h.Records()
	require.Len(t, records, 1, "should capture exactly one record")

	rec := records[0]
	assert.Equal(t, slog.LevelInfo, rec.Level)
	assert.Equal(t, "http_request", rec.Message)

	got, ok := slogtest.AttrValue(rec, "request_id")
	require.True(t, ok, "request_id attr should exist")
	assert.Equal(t, "rid-1", got.String())

	pathVal, ok := slogtest.AttrValue(rec, "api_path")
	require.True(t, ok)
	assert.Equal(t, "/ping", pathVal.String())

	latencyVal, ok := slogtest.AttrValue(rec, "latency_ms")
	require.True(t, ok)
	assert.Equal(t, int64(3), latencyVal.Int64())

	_, ok = slogtest.AttrValue(rec, "missing_key")
	assert.False(t, ok, "missing key should not be found")
}

// TestHandler_WithAttrsPropagation 验证 WithAttrs 继承的 attr 链会被附加到
// 子 logger 输出的每条 Record 上，并与 parent Handler 共享 records 存储。
func TestHandler_WithAttrsPropagation(t *testing.T) {
	h := slogtest.NewHandler(slog.LevelInfo)
	parent := slog.New(h)

	child := parent.With(slog.String("request_id", "rid-7"))
	child.Info("child_event", slog.String("api_path", "/version"))

	records := h.Records()
	require.Len(t, records, 1)

	rid, ok := slogtest.AttrValue(records[0], "request_id")
	require.True(t, ok)
	assert.Equal(t, "rid-7", rid.String())

	path, ok := slogtest.AttrValue(records[0], "api_path")
	require.True(t, ok)
	assert.Equal(t, "/version", path.String())
}

// TestHandler_LevelFiltering 验证 Enabled 按 level 过滤。
func TestHandler_LevelFiltering(t *testing.T) {
	h := slogtest.NewHandler(slog.LevelWarn)
	logger := slog.New(h)

	logger.Info("should be dropped")
	logger.Warn("should be kept")
	logger.Error("should be kept")

	records := h.Records()
	require.Len(t, records, 2, "Info should be filtered out at WarnLevel")
	assert.Equal(t, slog.LevelWarn, records[0].Level)
	assert.Equal(t, slog.LevelError, records[1].Level)
}

// TestHandler_Reset 验证 Reset 清空 records。
func TestHandler_Reset(t *testing.T) {
	h := slogtest.NewHandler(slog.LevelDebug)
	logger := slog.New(h)

	logger.Info("e1")
	logger.Info("e2")
	require.Len(t, h.Records(), 2)

	h.Reset()
	assert.Len(t, h.Records(), 0)

	logger.Info("e3")
	assert.Len(t, h.Records(), 1)
}
