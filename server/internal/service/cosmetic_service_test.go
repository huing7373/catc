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
	// Story 23.4 加：ListByIDsForInventory（GET /cosmetics/inventory config 关联）。
	listByIDsForInventoryFn func(ctx context.Context, ids []uint64) ([]mysql.CosmeticItem, error)
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

// ListByIDsForInventory: Story 23.4 加。ListCatalog 既有 case 不配置此 fn →
// 防御性 panic（catalog 路径不调 inventory config 关联）；ListInventory case
// 注入 listByIDsForInventoryFn 自定义返回。
func (s *stubCatalogCosmeticItemRepo) ListByIDsForInventory(ctx context.Context, ids []uint64) ([]mysql.CosmeticItem, error) {
	if s.listByIDsForInventoryFn == nil {
		panic("stubCatalogCosmeticItemRepo.ListByIDsForInventory not configured (本 case 走 ListCatalog 路径，不期望走 GET /cosmetics/inventory config 关联)")
	}
	return s.listByIDsForInventoryFn(ctx, ids)
}

// ListEnabledIDsByRarity: Story 23.5 给 CosmeticItemRepo interface 加了本方法
// （/dev/grant-cosmetic-batch 真实写库数据源；fix-review 23-5 r2 [P2] 根因
// 修复后 FindRandomByRarity(rarity,count) → ListEnabledIDsByRarity(rarity)）；
// cosmetic_service_test 仅测 ListCatalog / ListInventory 路径，不走 dev grant ——
// 防御性 panic 让任何意外调用暴露（仅为 satisfy 扩展后的 interface 编译）。
func (s *stubCatalogCosmeticItemRepo) ListEnabledIDsByRarity(ctx context.Context, rarity int8) ([]uint64, error) {
	panic("stubCatalogCosmeticItemRepo.ListEnabledIDsByRarity not configured (本 case 走 ListCatalog / ListInventory 路径，不期望走 /dev/grant-cosmetic-batch)")
}

// stubUserCosmeticItemRepo: mysql.UserCosmeticItemRepo 的统一 stub
// （service_test package 内唯一定义；Story 23.4 ListByUserForInventory 只读 +
// Story 23.5 CreateInTx 入仓写两路径共用，去重为单一定义避免同 package 重声明）。
//
// **本定义放在无 build tag 的 cosmetic_service_test.go**（而非
// `//go:build !integration` 的 chest_open_service_test.go）—— 因本文件在单测
// 与集成测试两种构建下都参与编译且引用本 stub；放在 !integration 文件会让
// `-tags integration` 构建找不到本类型（半成品 quota 中断遗留的 build tag
// 错位，本次续做修正）。
//
//   - ListByUserForInventory：注入 listByUserForInventoryFn 自定义返回
//     （cosmetic_service_test ListInventory case 用）；不注入则 panic
//     （chest_open_service_test 仅测开箱入仓写路径，不走 GET /cosmetics/inventory；
//     cosmetic_service_test ListCatalog case noop —— 均不调本方法）。
//   - CreateInTx：可注入 createInTxFn；不注入则默认回填 item.ID=90001 + return nil。
//     createInTxCall 记录每次调用的 item 供断言（chest_open_service_test 入仓 case 用）。
type stubUserCosmeticItemRepo struct {
	listByUserForInventoryFn func(ctx context.Context, userID uint64) ([]mysql.UserCosmeticItem, error)
	createInTxFn             func(ctx context.Context, item *mysql.UserCosmeticItem) error
	createInTxCall           []*mysql.UserCosmeticItem
}

func (s *stubUserCosmeticItemRepo) ListByUserForInventory(ctx context.Context, userID uint64) ([]mysql.UserCosmeticItem, error) {
	if s.listByUserForInventoryFn == nil {
		panic("stubUserCosmeticItemRepo.ListByUserForInventory not expected (chest_open_service_test 仅测开箱入仓写路径 / cosmetic_service_test ListCatalog 路径不读 user repo；ListInventory case 须注入 listByUserForInventoryFn)")
	}
	return s.listByUserForInventoryFn(ctx, userID)
}

func (s *stubUserCosmeticItemRepo) CreateInTx(ctx context.Context, item *mysql.UserCosmeticItem) error {
	s.createInTxCall = append(s.createInTxCall, item)
	if s.createInTxFn != nil {
		return s.createInTxFn(ctx, item)
	}
	// 默认 happy：模拟 GORM 回填 AUTO_INCREMENT ID
	if item.ID == 0 {
		item.ID = 90001
	}
	return nil
}

// buildCosmeticService: ListCatalog 既有 case 用 —— catalogFn 注入 catalog
// 路径返回，user repo 传 noop stub（catalog 路径不调 user repo；Story 23.4
// 扩 NewCosmeticService 2 参后既有调用同步改 2 参，否则编译不过）。
func buildCosmeticService(catalogFn func(ctx context.Context) ([]mysql.CosmeticItem, error)) service.CosmeticService {
	return service.NewCosmeticService(
		&stubCatalogCosmeticItemRepo{catalogFn: catalogFn},
		&stubUserCosmeticItemRepo{}, // noop：ListCatalog 路径不调 user repo
	)
}

// buildCosmeticInventoryService: ListInventory case 用 —— 注入 user repo
// ListByUserForInventory 返回 + cosmetic repo ListByIDsForInventory 返回
// （catalogFn 传 noop panic stub —— inventory 路径不调 ListEnabledForCatalog）。
func buildCosmeticInventoryService(
	userFn func(ctx context.Context, userID uint64) ([]mysql.UserCosmeticItem, error),
	cfgFn func(ctx context.Context, ids []uint64) ([]mysql.CosmeticItem, error),
) service.CosmeticService {
	return service.NewCosmeticService(
		&stubCatalogCosmeticItemRepo{
			catalogFn: func(ctx context.Context) ([]mysql.CosmeticItem, error) {
				panic("stubCatalogCosmeticItemRepo.ListEnabledForCatalog not expected (ListInventory case 不走 catalog 路径)")
			},
			listByIDsForInventoryFn: cfgFn,
		},
		&stubUserCosmeticItemRepo{listByUserForInventoryFn: userFn},
	)
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

// ============================================================================
// Story 23.4 — CosmeticService.ListInventory 单测（≥8 case，mocked 双 repo stub）
//
// 对齐 epics.md 行 3287-3292 + V1 §8.2 三态矩阵 A/B/C + 两级全序排序。
// 关键纠偏：epics.md 行 3292 "skip + log warning" 与 §8.2 三态矩阵冲突 —— 态 C
// **不 skip**（降级占位保留组）+ log error（非 warning）；以 §8.2 为准。
// ============================================================================

// AC7.1 happy: 3 hat (cosmeticId=12) + 1 scarf (cosmeticId=24) → groups 长度=2，
// hat 组 count=3 instances 长度=3，scarf 组 count=1（均态 A enabled）。
func TestCosmeticService_ListInventory_HappyPath_GroupsAndCount(t *testing.T) {
	svc := buildCosmeticInventoryService(
		func(ctx context.Context, userID uint64) ([]mysql.UserCosmeticItem, error) {
			return []mysql.UserCosmeticItem{
				{ID: 101, CosmeticItemID: 12, Status: 1},
				{ID: 102, CosmeticItemID: 12, Status: 1},
				{ID: 103, CosmeticItemID: 12, Status: 1},
				{ID: 201, CosmeticItemID: 24, Status: 1},
			}, nil
		},
		func(ctx context.Context, ids []uint64) ([]mysql.CosmeticItem, error) {
			return []mysql.CosmeticItem{
				{ID: 12, Name: "小黄帽", Slot: 1, Rarity: 1, IconURL: "https://x/i12", AssetURL: "https://x/a12", IsEnabled: 1},
				{ID: 24, Name: "星星围巾", Slot: 4, Rarity: 2, IconURL: "https://x/i24", AssetURL: "https://x/a24", IsEnabled: 1},
			}, nil
		},
	)

	got, err := svc.ListInventory(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListInventory: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (2 不同 cosmetic_item_id 聚合)", len(got))
	}
	// 两级排序：rarity ASC → hat(rarity=1) 在 scarf(rarity=2) 前
	hat := got[0]
	if hat.CosmeticItemID != 12 || hat.Count != 3 || len(hat.Instances) != 3 {
		t.Errorf("got[0] = %+v, want cosmeticItemId=12 count=3 instances=3", hat)
	}
	if hat.Name != "小黄帽" || hat.Slot != 1 || hat.Rarity != 1 || hat.IconURL != "https://x/i12" || hat.AssetURL != "https://x/a12" {
		t.Errorf("got[0] metadata = %+v, want 小黄帽/slot1/rarity1/真实非空 URL (态 A)", hat)
	}
	scarf := got[1]
	if scarf.CosmeticItemID != 24 || scarf.Count != 1 || len(scarf.Instances) != 1 {
		t.Errorf("got[1] = %+v, want cosmeticItemId=24 count=1 instances=1", scarf)
	}
}

// AC7.2 happy: 0 件 → groups=[]（非 nil 空 slice，保证 handler 下发 groups:[]）。
func TestCosmeticService_ListInventory_EmptyBag_ReturnsEmptySliceNotNil(t *testing.T) {
	svc := buildCosmeticInventoryService(
		func(ctx context.Context, userID uint64) ([]mysql.UserCosmeticItem, error) {
			return []mysql.UserCosmeticItem{}, nil
		},
		func(ctx context.Context, ids []uint64) ([]mysql.CosmeticItem, error) {
			panic("空背包不应调 ListByIDsForInventory（步骤 2 早返）")
		},
	)

	got, err := svc.ListInventory(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListInventory err = %v, want nil (空背包非 error，§8.2 行 1341)", err)
	}
	if got == nil {
		t.Errorf("got == nil, want []InventoryGroup{} (§8.2 行 1440: groups:[] not null)")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

// AC7.3 happy: 1 件 status=equipped → 仍包含 status=2（equipped 不被过滤；
// repo SQL `status IN (1,2)` 已含 2，service 不二次过滤）。
func TestCosmeticService_ListInventory_EquippedInstance_StillIncludedStatus2(t *testing.T) {
	svc := buildCosmeticInventoryService(
		func(ctx context.Context, userID uint64) ([]mysql.UserCosmeticItem, error) {
			return []mysql.UserCosmeticItem{
				{ID: 500, CosmeticItemID: 12, Status: 2}, // equipped
			}, nil
		},
		func(ctx context.Context, ids []uint64) ([]mysql.CosmeticItem, error) {
			return []mysql.CosmeticItem{
				{ID: 12, Name: "小黄帽", Slot: 1, Rarity: 1, IconURL: "https://x/i", AssetURL: "https://x/a", IsEnabled: 1},
			}, nil
		},
	)

	got, err := svc.ListInventory(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListInventory: %v", err)
	}
	if len(got) != 1 || got[0].Count != 1 || len(got[0].Instances) != 1 {
		t.Fatalf("got = %+v, want 1 组 count=1 instances=1", got)
	}
	if got[0].Instances[0].Status != 2 {
		t.Errorf("instances[0].Status = %d, want 2 (equipped 不被过滤)", got[0].Instances[0].Status)
	}
}

// AC7.4 edge 态 C: config map 不命中 → **不 skip**，降级占位保留组 + log error。
func TestCosmeticService_ListInventory_StateC_MissingConfig_PlaceholderNotSkip(t *testing.T) {
	svc := buildCosmeticInventoryService(
		func(ctx context.Context, userID uint64) ([]mysql.UserCosmeticItem, error) {
			return []mysql.UserCosmeticItem{
				{ID: 900, CosmeticItemID: 999, Status: 1},
			}, nil
		},
		func(ctx context.Context, ids []uint64) ([]mysql.CosmeticItem, error) {
			// 不含 999 → 态 C missing-no-row
			return []mysql.CosmeticItem{}, nil
		},
	)

	got, err := svc.ListInventory(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListInventory: %v", err)
	}
	// **关键**：态 C **不** skip！降级占位**保留**该组（§8.2 行 1437 + 行 1432：
	// 已拥有不得静默丢失；epics.md 行 3292 "skip" 已被 §8.2 收紧纠偏）。
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (态 C 不 skip，降级占位保留组)", len(got))
	}
	g := got[0]
	if g.CosmeticItemID != 999 || g.Count != 1 || len(g.Instances) != 1 {
		t.Errorf("态 C 组 = %+v, want cosmeticItemId=999 count=1 instances=1 (实例数据始终来自 user_cosmetic_items)", g)
	}
	if g.Name != "未知装扮" || g.Slot != 99 || g.Rarity != 1 || g.IconURL != "" || g.AssetURL != "" {
		t.Errorf("态 C 降级占位 = {Name:%q Slot:%d Rarity:%d IconURL:%q AssetURL:%q}, want {未知装扮 99 1 \"\" \"\"} (§8.2 行 1352)",
			g.Name, g.Slot, g.Rarity, g.IconURL, g.AssetURL)
	}
	// 注：态 C log error 行为（slog.ErrorContext）由集成测试 / 人工验证覆盖 ——
	// 单测层断言降级占位 + 保留组（不 skip）即可覆盖契约核心约束（§8.2 三态矩阵）。
}

// AC7.5 edge 态 B: config 命中但 is_enabled=0 → row 真实值（**非** placeholder，
// **不** log）—— 区分态 B vs 态 C 的关键 case。
func TestCosmeticService_ListInventory_StateB_DisabledButExists_UsesRealValues(t *testing.T) {
	svc := buildCosmeticInventoryService(
		func(ctx context.Context, userID uint64) ([]mysql.UserCosmeticItem, error) {
			return []mysql.UserCosmeticItem{
				{ID: 800, CosmeticItemID: 50, Status: 1},
			}, nil
		},
		func(ctx context.Context, ids []uint64) ([]mysql.CosmeticItem, error) {
			// 命中 50 但 is_enabled=0 → 态 B disabled-but-exists
			return []mysql.CosmeticItem{
				{ID: 50, Name: "旧帽子", Slot: 1, Rarity: 2, IconURL: "https://x/i", AssetURL: "https://x/a", IsEnabled: 0},
			}, nil
		},
	)

	got, err := svc.ListInventory(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListInventory: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1 (态 B 保留组)", len(got))
	}
	g := got[0]
	// **关键**：态 B 用 row 真实值（与态 A 一致），**非** 态 C 降级占位。
	if g.Name != "旧帽子" || g.Slot != 1 || g.Rarity != 2 || g.IconURL != "https://x/i" || g.AssetURL != "https://x/a" {
		t.Errorf("态 B 组 = {Name:%q Slot:%d Rarity:%d IconURL:%q AssetURL:%q}, want row 真实值 {旧帽子 1 2 https://x/i https://x/a}（**非**占位 未知装扮/99/1/空串，§8.2 行 1347/1351）",
			g.Name, g.Slot, g.Rarity, g.IconURL, g.AssetURL)
	}
}

// AC7.6 edge 两级全序排序: 多组乱序实例 → groups 按 (rarity, slot, cosmeticItemId)
// 升序 + 每组 instances 按 userCosmeticItemId 升序（逐对相邻 lexicographic 断言）。
func TestCosmeticService_ListInventory_TwoLevelDeterministicTotalOrder(t *testing.T) {
	svc := buildCosmeticInventoryService(
		func(ctx context.Context, userID uint64) ([]mysql.UserCosmeticItem, error) {
			// 乱序：cosmetic 30(rarity3,slot2) / 10(rarity1,slot1) /
			// 20(rarity1,slot1) / 40(rarity1,slot5)；同组实例 id 乱序
			return []mysql.UserCosmeticItem{
				{ID: 9003, CosmeticItemID: 30, Status: 1},
				{ID: 9001, CosmeticItemID: 10, Status: 2},
				{ID: 9005, CosmeticItemID: 10, Status: 1},
				{ID: 9002, CosmeticItemID: 10, Status: 1}, // 同组 id 乱序: 9001,9005,9002
				{ID: 9004, CosmeticItemID: 20, Status: 1},
				{ID: 9006, CosmeticItemID: 40, Status: 1},
			}, nil
		},
		func(ctx context.Context, ids []uint64) ([]mysql.CosmeticItem, error) {
			return []mysql.CosmeticItem{
				{ID: 30, Name: "c30", Slot: 2, Rarity: 3, IconURL: "https://x/i", AssetURL: "https://x/a", IsEnabled: 1},
				{ID: 10, Name: "c10", Slot: 1, Rarity: 1, IconURL: "https://x/i", AssetURL: "https://x/a", IsEnabled: 1},
				{ID: 20, Name: "c20", Slot: 1, Rarity: 1, IconURL: "https://x/i", AssetURL: "https://x/a", IsEnabled: 1},
				{ID: 40, Name: "c40", Slot: 5, Rarity: 1, IconURL: "https://x/i", AssetURL: "https://x/a", IsEnabled: 1},
			}, nil
		},
	)

	got, err := svc.ListInventory(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListInventory: %v", err)
	}
	// 期望 groups 序：10(r1,s1) → 20(r1,s1,id20) → 40(r1,s5) → 30(r3,s2)
	wantOrder := []uint64{10, 20, 40, 30}
	if len(got) != len(wantOrder) {
		t.Fatalf("len(got) = %d, want %d", len(got), len(wantOrder))
	}
	for i, wantCID := range wantOrder {
		if got[i].CosmeticItemID != wantCID {
			t.Errorf("got[%d].CosmeticItemID = %d, want %d (groups 按 rarity,slot,cosmeticItemId 升序)", i, got[i].CosmeticItemID, wantCID)
		}
	}
	// 逐对相邻 lexicographic <= 断言
	for i := 0; i+1 < len(got); i++ {
		a, b := got[i], got[i+1]
		if !lexLEService(a.Rarity, a.Slot, a.CosmeticItemID, b.Rarity, b.Slot, b.CosmeticItemID) {
			t.Errorf("groups 排序违约 got[%d]=(r=%d,s=%d,id=%d) 应 <= got[%d]=(r=%d,s=%d,id=%d)",
				i, a.Rarity, a.Slot, a.CosmeticItemID, i+1, b.Rarity, b.Slot, b.CosmeticItemID)
		}
	}
	// cosmetic 10 组内 instances 应按 userCosmeticItemId 升序: 9001,9002,9005
	c10 := got[0]
	wantInst := []uint64{9001, 9002, 9005}
	if len(c10.Instances) != 3 {
		t.Fatalf("c10 instances len = %d, want 3", len(c10.Instances))
	}
	for i, want := range wantInst {
		if c10.Instances[i].UserCosmeticItemID != want {
			t.Errorf("c10.Instances[%d].UserCosmeticItemID = %d, want %d (instances 按 userCosmeticItemId 升序)", i, c10.Instances[i].UserCosmeticItemID, want)
		}
	}
}

// AC7.7 edge: userRepo DB 错误 → 1009 ErrServiceBusy；nil slice。
func TestCosmeticService_ListInventory_UserRepoDBError_Returns1009(t *testing.T) {
	dbErr := stderrors.New("db down")
	svc := buildCosmeticInventoryService(
		func(ctx context.Context, userID uint64) ([]mysql.UserCosmeticItem, error) {
			return nil, dbErr
		},
		func(ctx context.Context, ids []uint64) ([]mysql.CosmeticItem, error) {
			panic("userRepo 错误后不应调 cosmeticRepo")
		},
	)

	got, err := svc.ListInventory(context.Background(), 7)
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
	if !stderrors.Is(err, dbErr) {
		t.Errorf("errors.Is(err, dbErr) = false, want true (cause 应保留供 log)")
	}
}

// AC7.8 edge: cosmeticRepo ListByIDsForInventory DB 错误 → 1009
// （config 关联失败也走 1009 路径）。
func TestCosmeticService_ListInventory_CosmeticRepoDBError_Returns1009(t *testing.T) {
	dbErr := stderrors.New("config query failed")
	svc := buildCosmeticInventoryService(
		func(ctx context.Context, userID uint64) ([]mysql.UserCosmeticItem, error) {
			return []mysql.UserCosmeticItem{
				{ID: 1, CosmeticItemID: 12, Status: 1},
			}, nil
		},
		func(ctx context.Context, ids []uint64) ([]mysql.CosmeticItem, error) {
			return nil, dbErr
		},
	)

	got, err := svc.ListInventory(context.Background(), 7)
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
}

// lexLEService 返回 (r1,s1,i1) <= (r2,s2,i2) 的 lexicographic 比较结果
// （service_test 单测包内用；集成测试有同名 lexLE，本处用独立名避免冲突）。
func lexLEService(r1, s1 int8, i1 uint64, r2, s2 int8, i2 uint64) bool {
	if r1 != r2 {
		return r1 < r2
	}
	if s1 != s2 {
		return s1 < s2
	}
	return i1 <= i2
}
