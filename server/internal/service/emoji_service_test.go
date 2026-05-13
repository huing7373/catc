package service_test

import (
	"context"
	stderrors "errors"
	"testing"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// Story 17.4 — EmojiService.ListAvailable 单测（≥4 case stub repo）
//
// 与 home_service_test / auth_service_test 同模式：用 stub repo struct 注入
// fn 字段让每 case 自定义返回；不依赖 sqlmock / 真 mysql。

// stubEmojiRepo 用 fn 字段让每个 case 自定义返回。
type stubEmojiRepo struct {
	listFn func(ctx context.Context) ([]mysql.EmojiConfig, error)
}

func (s *stubEmojiRepo) List(ctx context.Context) ([]mysql.EmojiConfig, error) {
	return s.listFn(ctx)
}

func buildEmojiService(listFn func(ctx context.Context) ([]mysql.EmojiConfig, error)) service.EmojiService {
	return service.NewEmojiService(&stubEmojiRepo{listFn: listFn})
}

// AC4.1 happy: 4 个 enabled 表情 → 4 个 EmojiBrief 按 sort_order 顺序返回
func TestEmojiService_ListAvailable_HappyPath_4Rows(t *testing.T) {
	svc := buildEmojiService(func(ctx context.Context) ([]mysql.EmojiConfig, error) {
		return []mysql.EmojiConfig{
			{ID: 1, Code: "wave", Name: "挥手", AssetURL: "https://x/wave", SortOrder: 1, IsEnabled: 1},
			{ID: 2, Code: "love", Name: "爱心", AssetURL: "https://x/love", SortOrder: 2, IsEnabled: 1},
			{ID: 3, Code: "laugh", Name: "大笑", AssetURL: "https://x/laugh", SortOrder: 3, IsEnabled: 1},
			{ID: 4, Code: "cry", Name: "哭", AssetURL: "https://x/cry", SortOrder: 4, IsEnabled: 1},
		}, nil
	})

	got, err := svc.ListAvailable(context.Background())
	if err != nil {
		t.Fatalf("ListAvailable: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("len(got) = %d, want 4", len(got))
	}
	want := []service.EmojiBrief{
		{Code: "wave", Name: "挥手", AssetURL: "https://x/wave", SortOrder: 1},
		{Code: "love", Name: "爱心", AssetURL: "https://x/love", SortOrder: 2},
		{Code: "laugh", Name: "大笑", AssetURL: "https://x/laugh", SortOrder: 3},
		{Code: "cry", Name: "哭", AssetURL: "https://x/cry", SortOrder: 4},
	}
	for i, w := range want {
		if got[i] != w {
			t.Errorf("got[%d] = %+v, want %+v", i, got[i], w)
		}
	}
}

// AC4.2 happy: repo 已过滤 disabled —— service 层从未见到 IsEnabled=0 行；本 case
// 模拟 repo 返 3 行 enabled（如果 DB 有 1 行 disabled，repo SQL `is_enabled = 1` 已
// 过滤）→ service 返 3 行。
func TestEmojiService_ListAvailable_DisabledFilteredAtRepoLayer_ReturnsOnlyEnabled(t *testing.T) {
	svc := buildEmojiService(func(ctx context.Context) ([]mysql.EmojiConfig, error) {
		return []mysql.EmojiConfig{
			{ID: 1, Code: "wave", Name: "挥手", AssetURL: "https://x/wave", SortOrder: 1, IsEnabled: 1},
			{ID: 2, Code: "love", Name: "爱心", AssetURL: "https://x/love", SortOrder: 2, IsEnabled: 1},
			{ID: 3, Code: "laugh", Name: "大笑", AssetURL: "https://x/laugh", SortOrder: 3, IsEnabled: 1},
			// IsEnabled=0 行被 repo SQL 过滤，stub repo 不返回
		}, nil
	})

	got, err := svc.ListAvailable(context.Background())
	if err != nil {
		t.Fatalf("ListAvailable: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("len(got) = %d, want 3 (disabled filtered at repo layer)", len(got))
	}
}

// AC4.3 edge: 0 行 → 返 []EmojiBrief{} 非 nil；不报错
func TestEmojiService_ListAvailable_EmptyResult_ReturnsEmptySliceNotNil(t *testing.T) {
	svc := buildEmojiService(func(ctx context.Context) ([]mysql.EmojiConfig, error) {
		return []mysql.EmojiConfig{}, nil
	})

	got, err := svc.ListAvailable(context.Background())
	if err != nil {
		t.Fatalf("ListAvailable err = %v, want nil", err)
	}
	if got == nil {
		t.Errorf("got == nil, want []EmojiBrief{} (V1 §11.1 行 1833: items: [] not null)")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

// AC4.4 edge: DB 错误 → 1009 ErrServiceBusy；nil slice
func TestEmojiService_ListAvailable_DBError_Returns1009(t *testing.T) {
	dbErr := stderrors.New("driver: connection lost")
	svc := buildEmojiService(func(ctx context.Context) ([]mysql.EmojiConfig, error) {
		return nil, dbErr
	})

	got, err := svc.ListAvailable(context.Background())
	if err == nil {
		t.Fatal("err == nil, want 1009 AppError")
	}
	if got != nil {
		t.Errorf("got != nil on error, want nil slice")
	}
	var appErr *apperror.AppError
	if !stderrors.As(err, &appErr) {
		t.Fatalf("err is not *apperror.AppError: %T", err)
	}
	if appErr.Code != apperror.ErrServiceBusy {
		t.Errorf("appErr.Code = %d, want %d (ErrServiceBusy)", appErr.Code, apperror.ErrServiceBusy)
	}
	// 验证 cause 保留（errors.Is 穿透）
	if !stderrors.Is(err, dbErr) {
		t.Errorf("errors.Is(err, dbErr) = false, want true (cause should be preserved for log)")
	}
}
