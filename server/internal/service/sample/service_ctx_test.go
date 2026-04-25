package sample_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/huing/cat/server/internal/service/sample"
)

// slowCtxAwareRepo 是一个"响应 ctx 的慢 repo" —— 实装 sample.SampleRepo 接口，
// 在 FindByID 里 select ctx.Done() / time.After(delay)，模拟真实 DB driver
// 在 WithContext 下对 cancel 的即时响应（Epic 4 真 GORM repo 同样行为）。
//
// **为什么不用 testify/mock**：testify 的 Mock.Called 在 mock 侧不感知 ctx.Done()
// channel，无法模拟"sleep 里面被 cancel 唤醒"这种需要 ctx channel 语义的场景。
// 一次性的 struct 比 mock 更贴近真实意图，也更贴近 ADR-0007 §5 对 Epic 4+ repo
// cancel 测试的示范要求。
//
// 未来 Epic 4+ 新 service 的 ctx cancel 测试直接复制本结构，改 DTO 类型 + method
// 名即可。
type slowCtxAwareRepo struct {
	delay time.Duration     // 正常路径 sleep 多久（模拟 DB 查询耗时）
	dto   *sample.SampleDTO // 正常路径返回的 DTO
}

func (r *slowCtxAwareRepo) FindByID(ctx context.Context, _ string) (*sample.SampleDTO, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(r.delay):
		return r.dto, nil
	}
}

// TestSampleService_CtxCancelPropagates：ctx 100ms 超时 + repo delay 5s
// → service 必须 < 500ms 返回（而非 wait 5s）+ err 是 DeadlineExceeded。
//
// 这是 Story 1.9 AC4 的核心 case —— 证明"ctx cancel 确实能从 service 调用点
// 一路穿到 repo，让慢 repo 提前返回"。
//
// ADR-0007 §5.2 / §7 C6 对 Epic 4+ 的迁移清单强制要求每个新 service 至少有
// 一条同形态 case；本 case 是模板。
func TestSampleService_CtxCancelPropagates(t *testing.T) {
	repo := &slowCtxAwareRepo{
		delay: 5 * time.Second,
		dto:   &sample.SampleDTO{ID: "x", Value: 42},
	}
	svc := sample.NewSampleService(repo)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, err := svc.GetValue(ctx, "x")
	elapsed := time.Since(start)

	if err == nil {
		t.Fatalf("expected ctx deadline error, got nil（elapsed=%v）", elapsed)
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded（repo 必须响应 ctx.Done()）", err)
	}
	// 500ms 上限：100ms ctx timeout + Go runtime 调度 overhead + Windows timer
	// 精度（±15ms）+ test runner overhead 的综合阈值。严阈值会出 flake。
	if elapsed >= 500*time.Millisecond {
		t.Fatalf("elapsed = %v, want < 500ms（service 不能 wait repo sleep 跑完）", elapsed)
	}
	// 极端下限：若 elapsed < 50ms，说明 slowCtxAwareRepo 的 select 没真正响应
	// timer（或者 ctx 根本没到 repo），不是硬性 fail 但打警告便于诊断。
	if elapsed < 50*time.Millisecond {
		t.Logf("WARN: elapsed %v < 50ms；检查 slowCtxAwareRepo 的 select 是否真的 wait", elapsed)
	}
}

// TestSampleService_CtxNormalPath_ReturnsNormally：ctx 不 cancel + 10ms 短 repo
// → service 正常返回 value，elapsed 远 < 100ms。
//
// 这条 happy case 是防 TestSampleService_CtxCancelPropagates 的**假阴性**：
// 若 slowCtxAwareRepo.FindByID 因某种 bug 直接 return ctx.Err() 而不真正等
// timer（如 select 的两个 case 顺序写错），上面的 cancel 测试会"假通过"
// （error 存在 + elapsed 短都满足，但根因错了）。本 happy case 证明在 ctx 正常
// 时 slowCtxAwareRepo 确实会 sleep 到 delay 结束才返回。
func TestSampleService_CtxNormalPath_ReturnsNormally(t *testing.T) {
	repo := &slowCtxAwareRepo{
		delay: 10 * time.Millisecond,
		dto:   &sample.SampleDTO{ID: "x", Value: 42},
	}
	svc := sample.NewSampleService(repo)

	ctx := context.Background()

	start := time.Now()
	got, err := svc.GetValue(ctx, "x")
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v（elapsed=%v）", err, elapsed)
	}
	if got != 42 {
		t.Errorf("got = %d, want 42", got)
	}
	// 100ms 上限：10ms delay + runtime overhead 绰绰有余
	if elapsed >= 100*time.Millisecond {
		t.Errorf("elapsed = %v, want < 100ms（delay 只有 10ms，单次调用不应这么慢）", elapsed)
	}
}
