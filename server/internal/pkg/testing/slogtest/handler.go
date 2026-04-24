// Package slogtest 提供 in-memory slog.Handler，用于测试断言结构化日志字段。
//
// 背景：
//   - ADR 0001-test-stack.md §4 约定了 6 个结构化日志字段（request_id /
//     user_id / api_path / latency_ms / business_result / error_code）。
//   - Story 1.3 已经落地 request_id / api_path / latency_ms / method / status /
//     client_ip；Story 1.8 会落地 error_code；Epic 4+ 会落地 user_id /
//     business_result。
//   - 未来业务 story 的单元测试需要断言"某个 code path 输出了含 xxx=yyy 字段
//     的日志"——本包提供捕获 + 查询能力，替代 Story 1.3 logging_test.go 当前
//     使用的 bytes.Buffer + JSON unmarshal 一次性方案。
//
// 用法示例：
//
//	h := slogtest.NewHandler(slog.LevelDebug)
//	logger := slog.New(h)
//	// ... 被测代码调用 logger.InfoContext(ctx, "event", slog.String("request_id", "rid-1")) ...
//	records := h.Records()
//	require.Len(t, records, 1)
//	val, ok := slogtest.AttrValue(records[0], "request_id")
//	require.True(t, ok)
//	assert.Equal(t, "rid-1", val.String())
//
// # WithGroup 语义（保留，与 slog.JSONHandler 对齐）
//
// 调 `logger.WithGroup("error").Info("msg", slog.Int("code", 1001))` 时，本
// Handler 会把 attr key 加 group prefix（dot-joined），捕获为 "error.code"，
// 与 slog.JSONHandler 产出的 `{"error":{"code":1001}}` 形状等价（用扁平 key
// 方便 AttrValue 查询）。嵌套 group 按顺序拼 `.`；`WithAttrs` 在 group 后
// 登记的 attr 也带 prefix；`WithAttrs` 在 group 前登记的 attr **不**带
// prefix（snapshot 时的 group 上下文决定）。
//
// 断言示例：
//
//	logger.WithGroup("error").Info("e", slog.Int("code", 1001))
//	v, ok := slogtest.AttrValue(records[0], "error.code")  // ← 带 prefix
//	// 此时按 "code"（裸 key）查 **不会** 命中 —— 与生产 JSONHandler 行为一致
//
// 已知局限（MVP 实现）：
//   - AttrValue 按扁平 key 精确匹配，**不**深度展开 slog.GroupValue 类型的
//     attr（例如调用方直接构造 `slog.Group("error", slog.Int("code", 1))`
//     而非走 WithGroup）。如 Epic 4+ 真用到这种用法，扩展 AttrValueDeep 即可。
//   - 本包**不**负责把 Record 渲染成 JSON 字符串；如需断言 JSON 形状，继续
//     用 bytes.Buffer + slog.NewJSONHandler 直连。
package slogtest

import (
	"context"
	"log/slog"
	"sync"
)

// Handler 是 slog.Handler 的 in-memory 实装。线程安全。
//
// groupPrefix 跟踪当前 group 链（dot-joined，空串 = 顶层）。WithGroup 返回
// 新 Handler，prefix 扩展一层。`prefixedAttrs` 是 WithAttrs 时 snapshot 的
// attrs，key 已在登记的那个时刻按**当时的** groupPrefix 加好前缀 —— 这样
// `WithAttrs(...).WithGroup(...).WithAttrs(...)` 两批 attr 的 prefix 各自正确。
type Handler struct {
	level slog.Level

	// mu / records 由 parent Handler 与 WithAttrs/WithGroup 产生的子
	// Handler 共享（指针），保证所有 logger 分叉都写入同一个 records slice。
	mu      *sync.Mutex
	records *[]slog.Record

	prefixedAttrs []slog.Attr
	groupPrefix   string
}

// NewHandler 构造一个新 Handler，设定最低记录级别。
func NewHandler(level slog.Level) *Handler {
	var records []slog.Record
	return &Handler{
		level:   level,
		mu:      &sync.Mutex{},
		records: &records,
	}
}

// Enabled 按 level 过滤。
func (h *Handler) Enabled(_ context.Context, lvl slog.Level) bool {
	return lvl >= h.level
}

// Handle 把原 Record 的 attrs 按当前 groupPrefix 重写 key，再拼上 WithAttrs
// 链上的 prefixedAttrs（这些 attr 的 key 已经在 WithAttrs 时按登记时刻的
// prefix 处理过了），追加到 records slice。
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	newRec := slog.NewRecord(r.Time, r.Level, r.Message, r.PC)
	newRec.AddAttrs(h.prefixedAttrs...)

	r.Attrs(func(a slog.Attr) bool {
		newRec.AddAttrs(slog.Attr{
			Key:   prefixKey(h.groupPrefix, a.Key),
			Value: a.Value,
		})
		return true
	})

	*h.records = append(*h.records, newRec)
	return nil
}

// WithAttrs 在**当前** groupPrefix 下登记 attrs：每个 attr 的 key 都加上
// prefix 后再存入 prefixedAttrs。这样即使后续 WithGroup 改了 prefix，这批
// attr 仍然保留登记时刻的 namespace。
func (h *Handler) WithAttrs(as []slog.Attr) slog.Handler {
	newAttrs := make([]slog.Attr, 0, len(h.prefixedAttrs)+len(as))
	newAttrs = append(newAttrs, h.prefixedAttrs...)
	for _, a := range as {
		newAttrs = append(newAttrs, slog.Attr{
			Key:   prefixKey(h.groupPrefix, a.Key),
			Value: a.Value,
		})
	}
	return &Handler{
		level:         h.level,
		mu:            h.mu,
		records:       h.records,
		prefixedAttrs: newAttrs,
		groupPrefix:   h.groupPrefix,
	}
}

// WithGroup 扩展 groupPrefix 一层。空 name 按 slog.Handler 接口文档约定视为
// no-op（slog stdlib 的 JSONHandler 行为与此一致）。
func (h *Handler) WithGroup(name string) slog.Handler {
	if name == "" {
		return h
	}
	newPrefix := name
	if h.groupPrefix != "" {
		newPrefix = h.groupPrefix + "." + name
	}
	return &Handler{
		level:         h.level,
		mu:            h.mu,
		records:       h.records,
		prefixedAttrs: h.prefixedAttrs,
		groupPrefix:   newPrefix,
	}
}

// Records 返回已捕获的 Record 快照（slice 头复制，元素仍指向原 Record）。
func (h *Handler) Records() []slog.Record {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make([]slog.Record, len(*h.records))
	copy(out, *h.records)
	return out
}

// Reset 清空已捕获的 records；用于 subtest 之间复用同一个 Handler。
func (h *Handler) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	*h.records = (*h.records)[:0]
}

// AttrValue 在 Record 的顶层 attrs 里按扁平 key 精确匹配查找 slog.Value。
// 返回 (value, found)。
//
// WithGroup 场景下 key 会带 dot-joined 前缀（例如 "error.code"）；调用方
// 必须用带前缀的完整 key 查询，与生产 slog.JSONHandler 的 JSON 扁平化行为
// 一致。不自动回落裸 key（避免静默掩盖 group 语义漂移）。
func AttrValue(r slog.Record, key string) (slog.Value, bool) {
	var found bool
	var value slog.Value
	r.Attrs(func(a slog.Attr) bool {
		if a.Key == key {
			value = a.Value
			found = true
			return false
		}
		return true
	})
	return value, found
}

// prefixKey 把 group prefix 和 attr key 拼成扁平 key。prefix 空串时原样返回。
func prefixKey(prefix, key string) string {
	if prefix == "" {
		return key
	}
	return prefix + "." + key
}
