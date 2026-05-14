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
