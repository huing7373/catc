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
// 已知局限（MVP 实现）：
//   - WithAttrs / WithGroup 返回的子 Handler 共享 parent 的 records 指针但各自
//     持有 mu；单线程测试足够，并发复杂场景（parent 和子 Handler 同时并发写）
//     可能少量丢失记录。Story 1.8 / Epic 4+ 的标准用法是"单 Handler 顺序
//     捕获"，该简化不影响当前目标。
//   - AttrValue 不递归展开 group 内部的 attr；如果未来用到
//     logger.WithGroup("error").Info(...)，扩展 AttrValueInGroup 即可。
//   - 本包**不**负责把 Record 渲染成 JSON 字符串；如需断言 JSON 形状，继续
//     用 bytes.Buffer + slog.NewJSONHandler 直连。
package slogtest

import (
	"context"
	"log/slog"
	"sync"
)

// Handler 是 slog.Handler 的 in-memory 实装。线程安全。
type Handler struct {
	level slog.Level

	// mu 保护 records slice。WithAttrs / WithGroup 返回的子 Handler 自持 mu,
	// 但 records 指针共享 —— 详见 package doc 的"已知局限"。
	mu      *sync.Mutex
	records *[]slog.Record

	// 继承自 WithAttrs / WithGroup 的 attr 链（按顺序 append 到每条 Record）。
	attrs  []slog.Attr
	groups []string
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

// Handle 克隆 record（深拷贝 attrs）并追加到 records。
func (h *Handler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	cloned := r.Clone()
	for _, a := range h.attrs {
		cloned.AddAttrs(a)
	}
	*h.records = append(*h.records, cloned)
	return nil
}

// WithAttrs 返回一个继承 attr 链的新 Handler，共享 records 与 mu。
func (h *Handler) WithAttrs(as []slog.Attr) slog.Handler {
	newAttrs := append([]slog.Attr{}, h.attrs...)
	newAttrs = append(newAttrs, as...)
	return &Handler{
		level:   h.level,
		mu:      h.mu,
		records: h.records,
		attrs:   newAttrs,
		groups:  h.groups,
	}
}

// WithGroup 返回一个记录 group 名的子 Handler。MVP 实现不深入 group 渲染。
func (h *Handler) WithGroup(name string) slog.Handler {
	newGroups := append([]string{}, h.groups...)
	newGroups = append(newGroups, name)
	return &Handler{
		level:   h.level,
		mu:      h.mu,
		records: h.records,
		attrs:   h.attrs,
		groups:  newGroups,
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

// AttrValue 在 Record 的顶层 attrs 里查找指定 key 的 slog.Value。
// 返回 (value, found)；groups / 嵌套 attr 不递归展开（MVP 够用）。
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
