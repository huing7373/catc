package service_test

import (
	"context"
	stderrors "errors"
	"strconv"
	"testing"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// Story 23.3 — CosmeticService.ListCatalog 单测（≥5 case stub repo）
//
// 与 emoji_service_test / home_service_test 同模式：用 stub repo struct 注入 fn
// 字段让每 case 自定义返回；不依赖 sqlmock / 真 mysql。
//
// **注**：本 story 新增独立 ListEnabledForCatalog；stub 同时实现既有
// ListEnabledForWeightedPick（CosmeticItemRepo interface 已含该方法 → stub 必须
// 实现以 satisfy interface 编译），但 ListCatalog 路径不调用它。

// stubCatalogCosmeticItemRepo 用 fn 字段让每个 case 自定义返回。
//
// **命名**：service_test package 内 Story 20.6 chest_open_service_test.go 已有
// `stubCosmeticItemRepo`（仅实现 ListEnabledForWeightedPick）；本 story 用独立
// 名 `stubCatalogCosmeticItemRepo` 避免同 package 重声明冲突（既有 20.6 stub 由
// 本 story 在其原文件补 ListEnabledForCatalog 以 satisfy 扩展后的 interface）。
//
// weightedPickFn 占位字段（默认 nil；本 service test 不调
// ListEnabledForWeightedPick，但 mysql.CosmeticItemRepo interface 已含该方法 →
// stub 必须实现以 satisfy interface 编译）。
type stubCatalogCosmeticItemRepo struct {
	catalogFn      func(ctx context.Context) ([]mysql.CosmeticItem, error)
	weightedPickFn func(ctx context.Context) ([]mysql.CosmeticItem, error)
}

func (s *stubCatalogCosmeticItemRepo) ListEnabledForCatalog(ctx context.Context) ([]mysql.CosmeticItem, error) {
	return s.catalogFn(ctx)
}

func (s *stubCatalogCosmeticItemRepo) ListEnabledForWeightedPick(ctx context.Context) ([]mysql.CosmeticItem, error) {
	if s.weightedPickFn == nil {
		// 本 service test 不应调 ListEnabledForWeightedPick；防御性 panic 让漂移暴露
		panic("stubCatalogCosmeticItemRepo.ListEnabledForWeightedPick not configured (cosmetic_service_test 仅测 ListCatalog 路径，不期望走开箱加权抽取路径)")
	}
	return s.weightedPickFn(ctx)
}

func buildCosmeticService(catalogFn func(ctx context.Context) ([]mysql.CosmeticItem, error)) service.CosmeticService {
	return service.NewCosmeticService(&stubCatalogCosmeticItemRepo{catalogFn: catalogFn})
}

// AC5.1 happy: 15 个 enabled cosmetic → 15 个 CosmeticBrief 逐字段值正确映射
// （id / code / name / slot / rarity / iconUrl / assetUrl）。
func TestCosmeticService_ListCatalog_HappyPath_15Rows(t *testing.T) {
	// 模拟 repo 已按 rarity ASC, slot ASC, id ASC 排序后返回的 15 行（0012 seed
	// 形态）；service 层透传不重排（排序在 repo SQL 层）。
	rows := make([]mysql.CosmeticItem, 0, 15)
	for i := 1; i <= 15; i++ {
		s := strconv.Itoa(i)
		rows = append(rows, mysql.CosmeticItem{
			ID: uint64(i), Code: "c" + s, Name: "装扮" + s,
			Slot: int8((i % 7) + 1), Rarity: int8((i % 4) + 1),
			IconURL: "https://x/i" + s, AssetURL: "https://x/a" + s,
			IsEnabled: 1,
		})
	}
	svc := buildCosmeticService(func(ctx context.Context) ([]mysql.CosmeticItem, error) {
		return rows, nil
	})

	got, err := svc.ListCatalog(context.Background())
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	if len(got) != 15 {
		t.Fatalf("len(got) = %d, want 15", len(got))
	}
	for i, r := range rows {
		want := service.CosmeticBrief{
			CosmeticItemID: r.ID, Code: r.Code, Name: r.Name,
			Slot: r.Slot, Rarity: r.Rarity, IconURL: r.IconURL, AssetURL: r.AssetURL,
		}
		if got[i] != want {
			t.Errorf("got[%d] = %+v, want %+v", i, got[i], want)
		}
	}
}

// AC5.2 happy: disabled 过滤在 repo SQL 层（WHERE is_enabled=1），service 层不
// 重复过滤 —— 本 case 用 stub 模拟"repo 已过滤后返回的就是 enabled 行"，验证
// service 透传不丢字段。
//
// **注**：disabled 过滤的真实验证在 AC6 集成测试（真实 SQL 跑 0012 全 enabled
// seed + UPDATE 一行 is_enabled=0 验证不返回 —— 见
// cosmetic_service_integration_test.go TestCosmeticServiceIntegration_ListCatalog_DisabledExcluded）；
// epics.md AC "1 个 disabled → 不返回" 的真值在 SQL 层，单测层 stub 无法测 SQL
// 过滤，故该 case 在集成测试覆盖，单测层覆盖 service 透传逻辑。
func TestCosmeticService_ListCatalog_DisabledFilteredAtRepoLayer_ReturnsOnlyEnabled(t *testing.T) {
	svc := buildCosmeticService(func(ctx context.Context) ([]mysql.CosmeticItem, error) {
		return []mysql.CosmeticItem{
			{ID: 1, Code: "hat_yellow", Name: "小黄帽", Slot: 1, Rarity: 1, IconURL: "https://x/i1", AssetURL: "https://x/a1", IsEnabled: 1},
			{ID: 2, Code: "hat_red", Name: "小红帽", Slot: 1, Rarity: 1, IconURL: "https://x/i2", AssetURL: "https://x/a2", IsEnabled: 1},
			// is_enabled=0 行被 repo SQL `WHERE is_enabled=1` 过滤，stub repo 不返回
		}, nil
	})

	got, err := svc.ListCatalog(context.Background())
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len(got) = %d, want 2 (disabled filtered at repo SQL layer)", len(got))
	}
	if got[0].Code != "hat_yellow" || got[1].Code != "hat_red" {
		t.Errorf("got codes = [%s, %s], want [hat_yellow, hat_red]", got[0].Code, got[1].Code)
	}
}

// AC5.3 edge: response 严格符合 §8.1 schema —— 返 1 行 → 断言 CosmeticBrief
// 字段集 = §8.1 data.items[] 钦定 7 字段，逐字段值正确无多余 / 无缺失。
func TestCosmeticService_ListCatalog_SchemaStrict_AllSevenFields(t *testing.T) {
	svc := buildCosmeticService(func(ctx context.Context) ([]mysql.CosmeticItem, error) {
		return []mysql.CosmeticItem{
			{
				ID: 12, Code: "hat_yellow", Name: "小黄帽", Slot: 1, Rarity: 1,
				IconURL: "https://placehold.co/64x64?text=Hat-Yellow",
				AssetURL: "https://placehold.co/128x128?text=Hat-Yellow",
				// repo SELECT 不取 drop_weight / is_enabled / created_at /
				// updated_at；即便 stub 填了 service 也不应映射进 CosmeticBrief
				DropWeight: 100, IsEnabled: 1,
			},
		}, nil
	})

	got, err := svc.ListCatalog(context.Background())
	if err != nil {
		t.Fatalf("ListCatalog: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	// CosmeticBrief 结构体本身即 §8.1 钦定 7 字段（编译期保证无多余字段）；
	// 这里逐字段断言值映射正确。
	want := service.CosmeticBrief{
		CosmeticItemID: 12, Code: "hat_yellow", Name: "小黄帽",
		Slot: 1, Rarity: 1,
		IconURL:  "https://placehold.co/64x64?text=Hat-Yellow",
		AssetURL: "https://placehold.co/128x128?text=Hat-Yellow",
	}
	if got[0] != want {
		t.Errorf("got[0] = %+v, want %+v (§8.1 7 字段严格映射)", got[0], want)
	}
}

// AC5.4 edge: DB 错误 → 1009 ErrServiceBusy；nil slice。
func TestCosmeticService_ListCatalog_DBError_Returns1009(t *testing.T) {
	dbErr := stderrors.New("driver: connection lost")
	svc := buildCosmeticService(func(ctx context.Context) ([]mysql.CosmeticItem, error) {
		return nil, dbErr
	})

	got, err := svc.ListCatalog(context.Background())
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
	// 验证 cause 保留（errors.Is 穿透 —— 供 log 用）
	if !stderrors.Is(err, dbErr) {
		t.Errorf("errors.Is(err, dbErr) = false, want true (cause should be preserved for log)")
	}
}

// AC5.5 edge: 空集 → 返 []CosmeticBrief{} 非 nil；不报错（§8.1 行 1301 catalog
// 为空返 {items:[]} code=0 不报错；保证 handler 下发 items:[] 非 null）。
func TestCosmeticService_ListCatalog_EmptyResult_ReturnsEmptySliceNotNil(t *testing.T) {
	svc := buildCosmeticService(func(ctx context.Context) ([]mysql.CosmeticItem, error) {
		return []mysql.CosmeticItem{}, nil
	})

	got, err := svc.ListCatalog(context.Background())
	if err != nil {
		t.Fatalf("ListCatalog err = %v, want nil (空 catalog 非 error)", err)
	}
	if got == nil {
		t.Errorf("got == nil, want []CosmeticBrief{} (§8.1 行 1301: items: [] not null)")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}
