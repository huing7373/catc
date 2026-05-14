package service_test

import (
	"context"
	stderrors "errors"
	"log/slog"
	"strings"
	"testing"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/testing/slogtest"
	"github.com/huing/cat/server/internal/service"
)

// Story 20.8 dev_cosmetic_service 单元测试（节点 7 阶段 explicit-failure 版本）。
//
// **节点 7 阶段 stub** —— service 内部 slog.WarnContext + return apperror.ErrServiceBusy (1009)
// → middleware 自动翻 HTTP 503。**绝不返 success**，避免 silent false-positive 让 e2e 调试链路
// 无故拉长（lesson: docs/lessons/2026-05-15-stub-endpoint-explicit-failure.md）。
//
// **无 repo 依赖** → **无 stub repo 需要**（与 7.5 / 20.7 单测大量 stub repo 不同）。
//
// 3 case（前缀 TestDevCosmeticService_GrantCosmeticBatch_<场景>）：
//   1. HappyPathStub_ReturnsServiceBusy_LogsWarn（验 1009 + WARN 日志结构化字段）
//   2. BoundaryCases_AlwaysReturnsServiceBusy（表驱动 rarity × count 共 12 组合都返 1009）
//   3. StubIgnoresInvalidParams_StillReturnsServiceBusy（验"service 不做参数防御，所有调用都失败"）

// captureSlog 替换 slog.Default() 为 slogtest handler，返回 handler + cleanup。
func captureSlog(t *testing.T) *slogtest.Handler {
	t.Helper()
	h := slogtest.NewHandler(slog.LevelDebug)
	orig := slog.Default()
	slog.SetDefault(slog.New(h))
	t.Cleanup(func() { slog.SetDefault(orig) })
	return h
}

// assertServiceBusyError 断言 err 是 *AppError + Code == 1009 + Message 含"node-7 stub"或"not yet implemented"
// 提示，让节点 8 激活时一旦改成 return nil，本断言会让本 case 失败提醒同步更新测试。
func assertServiceBusyError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil, want *AppError(ErrServiceBusy=1009) (stub 必须 explicit-failure，禁止 silent false-positive)")
	}
	var ae *apperror.AppError
	if !stderrors.As(err, &ae) {
		t.Fatalf("err = %v, want *apperror.AppError (stub 必须返 *AppError 让 middleware 翻 HTTP 503)", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("err.Code = %d, want %d (ErrServiceBusy → middleware 翻 HTTP 503)", ae.Code, apperror.ErrServiceBusy)
	}
	if !strings.Contains(ae.Message, "node-7 stub") && !strings.Contains(ae.Message, "not yet implemented") {
		t.Errorf("err.Message = %q, want contains 'node-7 stub' or 'not yet implemented' (让运维明确知道 stub 状态)", ae.Message)
	}
}

// 1. HappyPathStub_ReturnsServiceBusy_LogsWarn：合法 (userID=1001, rarity=1, count=10) →
// 返 *AppError(ErrServiceBusy=1009) + 触发 WARN 日志（含 phase=node-7-stub 字段）。
//
//   - 验"stub explicit failure（不返 nil！）"
//   - 验 WARN 日志触发（含 phase=node-7-stub 字段；标识 stub 状态以便运维 grep）
func TestDevCosmeticService_GrantCosmeticBatch_HappyPathStub_ReturnsServiceBusy_LogsWarn(t *testing.T) {
	h := captureSlog(t)
	svc := service.NewDevCosmeticService()

	err := svc.GrantCosmeticBatch(context.Background(), 1001, 1, 10)
	assertServiceBusyError(t, err)

	records := h.Records()
	if len(records) == 0 {
		t.Fatalf("expect at least one log record (slog.WarnContext); got 0")
	}
	var hit *slog.Record
	for i, rec := range records {
		if rec.Level == slog.LevelWarn && strings.Contains(rec.Message, "node-7 stub phase") {
			hit = &records[i]
			break
		}
	}
	if hit == nil {
		t.Fatalf("expect a WARN log with msg containing 'node-7 stub phase'; got records=%v", records)
	}

	// 验关键结构化字段：phase / todo
	phase, ok := slogtest.AttrValue(*hit, "phase")
	if !ok {
		t.Errorf("log should carry phase attr; got record=%v", *hit)
	} else if phase.String() != "node-7-stub" {
		t.Errorf("phase attr = %q, want 'node-7-stub'", phase.String())
	}

	if _, ok := slogtest.AttrValue(*hit, "todo"); !ok {
		t.Errorf("log should carry todo attr (节点 8 激活路径标注)")
	}

	// 验透传参数
	uid, ok := slogtest.AttrValue(*hit, "user_id")
	if !ok || uid.Uint64() != 1001 {
		t.Errorf("user_id attr = %v ok=%v, want 1001", uid, ok)
	}
	rarity, ok := slogtest.AttrValue(*hit, "rarity")
	if !ok || rarity.Int64() != 1 {
		t.Errorf("rarity attr = %v ok=%v, want 1", rarity, ok)
	}
	count, ok := slogtest.AttrValue(*hit, "count")
	if !ok || count.Int64() != 10 {
		t.Errorf("count attr = %v ok=%v, want 10", count, ok)
	}
}

// 2. BoundaryCases_AlwaysReturnsServiceBusy：表驱动 rarity ∈ {1,2,3,4} × count ∈ {1,10,100}
// 共 12 组合全部返 *AppError(ErrServiceBusy)（验 stub 无差别 reject，节点 8 激活前没有合法 happy
// path —— 不能让任何 demo / e2e 误以为"stub 偶尔放行了"）。
func TestDevCosmeticService_GrantCosmeticBatch_BoundaryCases_AlwaysReturnsServiceBusy(t *testing.T) {
	svc := service.NewDevCosmeticService()
	ctx := context.Background()

	rarities := []int8{1, 2, 3, 4}
	counts := []int32{1, 10, 100}

	for _, r := range rarities {
		for _, c := range counts {
			r := r
			c := c
			t.Run("rarity="+string(rune('0'+r))+"_count="+itoa(c), func(t *testing.T) {
				err := svc.GrantCosmeticBatch(ctx, 1001, r, c)
				assertServiceBusyError(t, err)
			})
		}
	}
}

// 3. StubIgnoresInvalidParams_StillReturnsServiceBusy：传 rarity=99 / count=0 / userID=0
// 等"handler 应该拦截但 service 不防御"的参数 → service stub 仍然无差别 return 1009
// （验"stub 不做 service 层参数防御，所有调用都失败"，与 7.5 dev grant 的 "steps<0 panic" 模式有区别）。
func TestDevCosmeticService_GrantCosmeticBatch_StubIgnoresInvalidParams_StillReturnsServiceBusy(t *testing.T) {
	svc := service.NewDevCosmeticService()
	ctx := context.Background()

	cases := []struct {
		name   string
		userID uint64
		rarity int8
		count  int32
	}{
		{name: "rarity=99 (handler 应拦截; service stub 仍 1009)", userID: 1001, rarity: 99, count: 10},
		{name: "rarity=0 (handler 应拦截; service stub 仍 1009)", userID: 1001, rarity: 0, count: 10},
		{name: "rarity=-1 (handler 应拦截; service stub 仍 1009)", userID: 1001, rarity: -1, count: 10},
		{name: "count=0 (handler 应拦截; service stub 仍 1009)", userID: 1001, rarity: 1, count: 0},
		{name: "count=-1 (handler 应拦截; service stub 仍 1009)", userID: 1001, rarity: 1, count: -1},
		{name: "count=1000 (handler 应拦截; service stub 仍 1009)", userID: 1001, rarity: 1, count: 1000},
		{name: "userID=0 (handler 应拦截; service stub 仍 1009)", userID: 0, rarity: 1, count: 10},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := svc.GrantCosmeticBatch(ctx, tc.userID, tc.rarity, tc.count)
			assertServiceBusyError(t, err)
		})
	}
}

// itoa 简易 int32 → string（避免引 strconv 仅为子测试名）。
func itoa(n int32) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 8)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}
