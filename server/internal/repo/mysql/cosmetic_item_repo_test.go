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
