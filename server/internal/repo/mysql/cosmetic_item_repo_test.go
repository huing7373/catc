package mysql

import (
	"context"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// Story 20.6 — CosmeticItemRepo.ListEnabledForWeightedPick sqlmock 单测（≥2 case）

// TestCosmeticItemRepo_ListEnabledForWeightedPick_HappyPath:
// 多行 enabled cosmetic_items → repo 透传完整 slice。
func TestCosmeticItemRepo_ListEnabledForWeightedPick_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewCosmeticItemRepo(gormDB)

	rows := sqlmock.NewRows([]string{
		"id", "code", "name", "slot", "rarity", "asset_url", "icon_url",
		"drop_weight", "is_enabled", "created_at", "updated_at",
	}).
		AddRow(uint64(24), "scarf_star", "星星围巾", int8(4), int8(2), "https://x/a", "https://x/i", uint32(30), int8(1), nil, nil).
		AddRow(uint64(25), "hat_red", "红色帽子", int8(1), int8(1), "https://x/a2", "https://x/i2", uint32(50), int8(1), nil, nil).
		AddRow(uint64(26), "glove_blue", "蓝色手套", int8(2), int8(3), "https://x/a3", "https://x/i3", uint32(20), int8(1), nil, nil)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `cosmetic_items` WHERE is_enabled = ?")).
		WithArgs(1).
		WillReturnRows(rows)

	got, err := repo.ListEnabledForWeightedPick(context.Background())
	if err != nil {
		t.Fatalf("ListEnabledForWeightedPick: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if got[0].ID != 24 || got[0].DropWeight != 30 || got[0].Name != "星星围巾" {
		t.Errorf("got[0] = %+v, want id=24 weight=30 name=星星围巾", got[0])
	}
	if got[1].DropWeight != 50 {
		t.Errorf("got[1].DropWeight = %d, want 50", got[1].DropWeight)
	}
	if got[2].Rarity != 3 {
		t.Errorf("got[2].Rarity = %d, want 3", got[2].Rarity)
	}
}

// TestCosmeticItemRepo_ListEnabledForWeightedPick_EmptyResult_ReturnsEmptySlice:
// 空结果（seed 未执行）→ 返 []CosmeticItem{}（非 nil），service 层判 len==0 翻译为 1009。
func TestCosmeticItemRepo_ListEnabledForWeightedPick_EmptyResult_ReturnsEmptySlice(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewCosmeticItemRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `cosmetic_items` WHERE is_enabled = ?")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // 0 行

	got, err := repo.ListEnabledForWeightedPick(context.Background())
	if err != nil {
		t.Fatalf("ListEnabledForWeightedPick: %v", err)
	}
	if got == nil {
		t.Errorf("got == nil, want []CosmeticItem{}")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

// Story 23.3 — CosmeticItemRepo.ListEnabledForCatalog sqlmock 单测（≥2 case）
//
// 与既有 ListEnabledForWeightedPick 测同模式（sqlmock，非 dockertest）。
// 排序契约的真实 SQL 验证在集成测试（cosmetic_service_integration_test.go），
// 本 repo 层 sqlmock 验证 SQL 生成形态（显式 7 列 Select + WHERE + ORDER BY）+
// 透传 + 空集兜底。

// TestCosmeticItemRepo_ListEnabledForCatalog_HappyPath:
// 多行 enabled → repo 显式 7 列 SELECT + WHERE is_enabled=1 + ORDER BY
// rarity ASC, slot ASC, id ASC + 透传完整 slice。
func TestCosmeticItemRepo_ListEnabledForCatalog_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewCosmeticItemRepo(gormDB)

	rows := sqlmock.NewRows([]string{
		"id", "code", "name", "slot", "rarity", "icon_url", "asset_url",
	}).
		AddRow(uint64(1), "hat_yellow", "小黄帽", int8(1), int8(1), "https://x/i1", "https://x/a1").
		AddRow(uint64(2), "hat_red", "小红帽", int8(1), int8(1), "https://x/i2", "https://x/a2").
		AddRow(uint64(13), "hat_crown", "金王冠", int8(1), int8(3), "https://x/i3", "https://x/a3")

	// 显式 Select 7 列 + WHERE is_enabled=? + ORDER BY rarity ASC, slot ASC, id ASC
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT id, code, name, slot, rarity, icon_url, asset_url FROM `cosmetic_items` " +
			"WHERE is_enabled = ? ORDER BY rarity ASC, slot ASC, id ASC")).
		WithArgs(1).
		WillReturnRows(rows)

	got, err := repo.ListEnabledForCatalog(context.Background())
	if err != nil {
		t.Fatalf("ListEnabledForCatalog: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if got[0].ID != 1 || got[0].Code != "hat_yellow" || got[0].Name != "小黄帽" ||
		got[0].Slot != 1 || got[0].Rarity != 1 ||
		got[0].IconURL != "https://x/i1" || got[0].AssetURL != "https://x/a1" {
		t.Errorf("got[0] = %+v, want id=1 code=hat_yellow name=小黄帽 slot=1 rarity=1 icon=https://x/i1 asset=https://x/a1", got[0])
	}
	if got[2].ID != 13 || got[2].Rarity != 3 {
		t.Errorf("got[2] = %+v, want id=13 rarity=3", got[2])
	}
}

// TestCosmeticItemRepo_ListEnabledForCatalog_EmptyResult_ReturnsEmptySlice:
// 空结果 → 返 []CosmeticItem{}（非 nil），service 层透传为 {items:[]} 非 error
// （§8.1 行 1301：catalog 为空 code=0 不报错）。
func TestCosmeticItemRepo_ListEnabledForCatalog_EmptyResult_ReturnsEmptySlice(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewCosmeticItemRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT id, code, name, slot, rarity, icon_url, asset_url FROM `cosmetic_items` " +
			"WHERE is_enabled = ? ORDER BY rarity ASC, slot ASC, id ASC")).
		WithArgs(1).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // 0 行

	got, err := repo.ListEnabledForCatalog(context.Background())
	if err != nil {
		t.Fatalf("ListEnabledForCatalog: %v", err)
	}
	if got == nil {
		t.Errorf("got == nil, want []CosmeticItem{}")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

// Story 23.4 — CosmeticItemRepo.ListByIDsForInventory sqlmock 单测（≥3 case）
//
// 与既有 ListEnabledForCatalog 测同模式（sqlmock，非 dockertest）。验证 SQL
// 生成形态（显式列含 is_enabled、**无** WHERE is_enabled=1、**无** ORDER BY、
// WHERE id IN ?）+ 空 ids 早返不发 SQL + 透传 + 空集兜底。真实 SQL 关联 + 态 B
// 不被过滤的真值在集成测试覆盖。

// TestCosmeticItemRepo_ListByIDsForInventory_HappyPath:
// 多 id → 显式列 SELECT（含 is_enabled）+ WHERE id IN ? + **无** is_enabled=1 +
// **无** ORDER BY + 透传完整 slice（含 is_enabled=0 行 —— 不被过滤，态 B 契约）。
func TestCosmeticItemRepo_ListByIDsForInventory_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewCosmeticItemRepo(gormDB)

	rows := sqlmock.NewRows([]string{
		"id", "name", "slot", "rarity", "icon_url", "asset_url", "is_enabled",
	}).
		AddRow(uint64(12), "小黄帽", int8(1), int8(1), "https://x/i12", "https://x/a12", int8(1)).
		AddRow(uint64(50), "旧帽子", int8(1), int8(2), "https://x/i50", "https://x/a50", int8(0)) // disabled 行仍返回（态 B）

	// 显式列含 is_enabled + WHERE id IN ? + 无 is_enabled=1 过滤 + 无 ORDER BY
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT id, name, slot, rarity, icon_url, asset_url, is_enabled FROM `cosmetic_items` " +
			"WHERE id IN (?,?)")).
		WithArgs(uint64(12), uint64(50)).
		WillReturnRows(rows)

	got, err := repo.ListByIDsForInventory(context.Background(), []uint64{12, 50})
	if err != nil {
		t.Fatalf("ListByIDsForInventory: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2", len(got))
	}
	if got[0].ID != 12 || got[0].Name != "小黄帽" || got[0].Slot != 1 || got[0].Rarity != 1 ||
		got[0].IconURL != "https://x/i12" || got[0].AssetURL != "https://x/a12" || got[0].IsEnabled != 1 {
		t.Errorf("got[0] = %+v, want id=12 小黄帽 slot=1 rarity=1 icon/asset 非空 is_enabled=1", got[0])
	}
	// disabled 行（is_enabled=0）仍被返回 —— **不**加 is_enabled=1 过滤（§8.2 行 1437 态 B 契约）
	if got[1].ID != 50 || got[1].IsEnabled != 0 {
		t.Errorf("got[1] = %+v, want id=50 is_enabled=0（disabled 行不被过滤，态 B 契约）", got[1])
	}
}

// TestCosmeticItemRepo_ListByIDsForInventory_EmptyIDs_ReturnsEmptySliceNoSQL:
// ids=[] → 早返 []CosmeticItem{} 非 nil，**不**发任何 SQL（避免 `IN ()` 空集
// 退化 SQL）。mock 不 ExpectQuery → 若误发 SQL 会 ExpectationsWereMet 失败。
func TestCosmeticItemRepo_ListByIDsForInventory_EmptyIDs_ReturnsEmptySliceNoSQL(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewCosmeticItemRepo(gormDB)

	got, err := repo.ListByIDsForInventory(context.Background(), []uint64{})
	if err != nil {
		t.Fatalf("ListByIDsForInventory: %v", err)
	}
	if got == nil {
		t.Errorf("got == nil, want []CosmeticItem{}")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
	// 验证未发任何 SQL（mock 无 ExpectQuery；若误发会有 unexpected query 错误）
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("空 ids 不应发 SQL，但 mock 收到未预期调用: %v", err)
	}
}

// TestCosmeticItemRepo_ListByIDsForInventory_NoMatch_ReturnsEmptySlice:
// IN 不命中任何 id（全部态 C）→ 返 []CosmeticItem{} 非 nil（service 层据
// config map 是否命中判态 C，repo 仅返空集）。
func TestCosmeticItemRepo_ListByIDsForInventory_NoMatch_ReturnsEmptySlice(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewCosmeticItemRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT id, name, slot, rarity, icon_url, asset_url, is_enabled FROM `cosmetic_items` " +
			"WHERE id IN (?)")).
		WithArgs(uint64(999)).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // 0 行

	got, err := repo.ListByIDsForInventory(context.Background(), []uint64{999})
	if err != nil {
		t.Fatalf("ListByIDsForInventory: %v", err)
	}
	if got == nil {
		t.Errorf("got == nil, want []CosmeticItem{}")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}

// Story 23.5 — CosmeticItemRepo.FindRandomByRarity sqlmock 单测（AC6）
//
// 验证 SQL 形态：SELECT id ... WHERE rarity=? AND is_enabled=? ORDER BY RAND()
// LIMIT ? + 返回 cosmetic_item_id slice + 空集兜底（返 []uint64{} 非 nil，
// service 层判 len==0 → 1009 seed 数据完整性异常）。RAND() 真随机性 + dev
// grant 端到端在 dev_cosmetic_service 单测 + e2e 覆盖。

// TestCosmeticItemRepo_FindRandomByRarity_HappyPath:
// rarity=1 抽 3 个 → 返 3 个 enabled cosmetic_item_id。
func TestCosmeticItemRepo_FindRandomByRarity_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewCosmeticItemRepo(gormDB)

	rows := sqlmock.NewRows([]string{"id"}).
		AddRow(uint64(25)).
		AddRow(uint64(31)).
		AddRow(uint64(42))

	// SELECT `id` FROM `cosmetic_items` WHERE rarity = ? AND is_enabled = ?
	// ORDER BY RAND() LIMIT ?（GORM Limit 渲染为 LIMIT 占位）
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id` FROM `cosmetic_items` WHERE rarity = ? AND is_enabled = ? ORDER BY RAND() LIMIT ?")).
		WithArgs(int8(1), 1, 3).
		WillReturnRows(rows)

	got, err := repo.FindRandomByRarity(context.Background(), 1, 3)
	if err != nil {
		t.Fatalf("FindRandomByRarity: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if got[0] != 25 || got[1] != 31 || got[2] != 42 {
		t.Errorf("got = %v, want [25 31 42]", got)
	}
}

// TestCosmeticItemRepo_FindRandomByRarity_EmptyResult_ReturnsEmptySlice:
// 无匹配 rarity（理论 seed ≥15 行不该发生）→ 返 []uint64{}（非 nil），
// service 层判 len==0 翻译为 1009。
func TestCosmeticItemRepo_FindRandomByRarity_EmptyResult_ReturnsEmptySlice(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewCosmeticItemRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id` FROM `cosmetic_items` WHERE rarity = ? AND is_enabled = ? ORDER BY RAND() LIMIT ?")).
		WithArgs(int8(9), 1, 5).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // 0 行

	got, err := repo.FindRandomByRarity(context.Background(), 9, 5)
	if err != nil {
		t.Fatalf("FindRandomByRarity: %v", err)
	}
	if got == nil {
		t.Errorf("got == nil, want []uint64{}（service 层无需 nil-check）")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}
