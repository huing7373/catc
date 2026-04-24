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

// TestHandler_WithGroupNamespacing 验证 WithGroup 正确保留 group 命名空间：
//   - 单层 group：record 上的 attr key 加 group prefix（"error.code"）
//   - 嵌套 group：prefix 按顺序 dot-join（"a.b.k"）
//   - WithAttrs 在 group 前登记：不带 prefix（snapshot 时 prefix 为空）
//   - WithAttrs 在 group 后登记：带当时的 prefix
//   - 空 group name：按 slog 接口约定为 no-op
//
// 这与生产 slog.JSONHandler 输出 `{"error":{"code":1001}}` 的嵌套行为在
// 语义上等价（用扁平 key 方便 AttrValue 查询）。
func TestHandler_WithGroupNamespacing(t *testing.T) {
	h := slogtest.NewHandler(slog.LevelDebug)
	logger := slog.New(h)

	// Case 1: 单层 group
	logger.WithGroup("error").Info("e1", slog.Int("code", 1001))

	// Case 2: 嵌套 group
	logger.WithGroup("a").WithGroup("b").Info("e2", slog.String("k", "v"))

	// Case 3: WithAttrs 在 group 前后的 namespace 差异
	mixed := logger.With(slog.String("top", "T")).
		WithGroup("ns").
		With(slog.String("mid", "M"))
	mixed.Info("e3", slog.String("leaf", "L"))

	// Case 4: 空 group name → no-op（不应多一层 prefix）
	logger.WithGroup("").Info("e4", slog.String("flat", "F"))

	records := h.Records()
	require.Len(t, records, 4)

	// Case 1 断言
	v, ok := slogtest.AttrValue(records[0], "error.code")
	require.True(t, ok, "attr should be captured under group prefix 'error.code'")
	assert.Equal(t, int64(1001), v.Int64())

	_, ok = slogtest.AttrValue(records[0], "code")
	assert.False(t, ok, "plain 'code' (no prefix) must not match under WithGroup; fixture must mirror JSONHandler namespacing")

	// Case 2 断言
	v, ok = slogtest.AttrValue(records[1], "a.b.k")
	require.True(t, ok, "nested WithGroup should dot-join prefixes")
	assert.Equal(t, "v", v.String())

	// Case 3 断言
	top, ok := slogtest.AttrValue(records[2], "top")
	require.True(t, ok, "WithAttrs before WithGroup → attr stays at top level")
	assert.Equal(t, "T", top.String())

	mid, ok := slogtest.AttrValue(records[2], "ns.mid")
	require.True(t, ok, "WithAttrs after WithGroup → attr carries group prefix")
	assert.Equal(t, "M", mid.String())

	leaf, ok := slogtest.AttrValue(records[2], "ns.leaf")
	require.True(t, ok, "Info attrs under an active group → carry group prefix")
	assert.Equal(t, "L", leaf.String())

	// Case 4 断言
	flat, ok := slogtest.AttrValue(records[3], "flat")
	require.True(t, ok, "empty group name should be a no-op, attr stays flat")
	assert.Equal(t, "F", flat.String())
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
