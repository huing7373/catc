package mysql

import (
	"context"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// Story 23.4 — UserCosmeticItemRepo.ListByUserForInventory sqlmock 单测（≥3 case）
//
// 与 cosmetic_item_repo_test.go 同测试风格（sqlmock，非 dockertest —— 既有
// mysql repo 单测均用 sqlmock，dockertest 在 *_integration_test.go 走
// build tag）。验证 SQL 生成形态（显式 3 列 Select + WHERE user_id=? AND
// status IN (1,2) + **无** ORDER BY）+ 透传 + 空集兜底。status IN (1,2) 过滤的
// 真实 SQL 真值 + consumed/invalid 不返回 在集成测试覆盖（手工 INSERT
// status=3 行验证不返回）。

// TestUserCosmeticItemRepo_ListByUserForInventory_HappyPath:
// user 有多实例 status 1/2 → 显式 3 列 SELECT + WHERE user_id=? AND
// status IN (1,2) + 透传完整 slice（含 status=2 equipped）。
func TestUserCosmeticItemRepo_ListByUserForInventory_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserCosmeticItemRepo(gormDB)

	rows := sqlmock.NewRows([]string{"id", "cosmetic_item_id", "status"}).
		AddRow(uint64(101), uint64(12), int8(1)).
		AddRow(uint64(102), uint64(12), int8(2)). // equipped 仍返回
		AddRow(uint64(201), uint64(24), int8(1))

	// 显式 3 列 Select + WHERE user_id=? AND status IN (?,?) + 无 ORDER BY
	// （GORM `status IN ?` + []int8{1,2} → `status IN (?,?)` 占位，args: userID,1,2）
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT id, cosmetic_item_id, status FROM `user_cosmetic_items` " +
			"WHERE user_id = ? AND status IN (?,?)")).
		WithArgs(uint64(7), int8(1), int8(2)).
		WillReturnRows(rows)

	got, err := repo.ListByUserForInventory(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListByUserForInventory: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len(got) = %d, want 3", len(got))
	}
	if got[0].ID != 101 || got[0].CosmeticItemID != 12 || got[0].Status != 1 {
		t.Errorf("got[0] = %+v, want id=101 cosmeticItemId=12 status=1", got[0])
	}
	// status=2 equipped 实例仍返回（status IN (1,2) 含 2）
	if got[1].ID != 102 || got[1].Status != 2 {
		t.Errorf("got[1] = %+v, want id=102 status=2 (equipped 不被过滤)", got[1])
	}
	if got[2].CosmeticItemID != 24 {
		t.Errorf("got[2].CosmeticItemID = %d, want 24", got[2].CosmeticItemID)
	}
}

// TestUserCosmeticItemRepo_ListByUserForInventory_StatusFilteredInSQL:
// status=3(consumed) / status=4(invalid) 被 SQL `WHERE status IN (1,2)` 过滤 ——
// sqlmock 模拟 SQL 已只返 status IN (1,2) 行（status=3/4 行不在 ExpectQuery
// 返回集），验证 repo 不二次过滤、透传 SQL 结果（status 过滤真值在 SQL 层；
// 真实 status=3 不返回的端到端真值在集成测试覆盖）。
func TestUserCosmeticItemRepo_ListByUserForInventory_StatusFilteredInSQL(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserCosmeticItemRepo(gormDB)

	// SQL `WHERE status IN (1,2)` 已在 DB 层过滤掉 status=3/4；mock 只返回
	// 通过过滤的 status IN (1,2) 行（模拟真实 SQL 行为）。
	rows := sqlmock.NewRows([]string{"id", "cosmetic_item_id", "status"}).
		AddRow(uint64(101), uint64(12), int8(1)).
		AddRow(uint64(103), uint64(12), int8(2))

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT id, cosmetic_item_id, status FROM `user_cosmetic_items` " +
			"WHERE user_id = ? AND status IN (?,?)")).
		WithArgs(uint64(7), int8(1), int8(2)).
		WillReturnRows(rows)

	got, err := repo.ListByUserForInventory(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListByUserForInventory: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (consumed/invalid 被 SQL status IN (1,2) 过滤)", len(got))
	}
	for _, r := range got {
		if r.Status != 1 && r.Status != 2 {
			t.Errorf("got 含 status=%d 行，want 仅 status IN (1,2)（SQL 层已过滤 consumed/invalid）", r.Status)
		}
	}
}

// TestUserCosmeticItemRepo_ListByUserForInventory_EmptyResult_ReturnsEmptySlice:
// user 无任何 status IN (1,2) 实例 → 返 []UserCosmeticItem{}（非 nil），
// service 层透传为 {groups:[]} 非 error（§8.2 行 1341）。
func TestUserCosmeticItemRepo_ListByUserForInventory_EmptyResult_ReturnsEmptySlice(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserCosmeticItemRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT id, cosmetic_item_id, status FROM `user_cosmetic_items` " +
			"WHERE user_id = ? AND status IN (?,?)")).
		WithArgs(uint64(7), int8(1), int8(2)).
		WillReturnRows(sqlmock.NewRows([]string{"id"})) // 0 行

	got, err := repo.ListByUserForInventory(context.Background(), 7)
	if err != nil {
		t.Fatalf("ListByUserForInventory: %v", err)
	}
	if got == nil {
		t.Errorf("got == nil, want []UserCosmeticItem{}")
	}
	if len(got) != 0 {
		t.Errorf("len(got) = %d, want 0", len(got))
	}
}
