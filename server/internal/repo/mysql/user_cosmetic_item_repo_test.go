package mysql

import (
	"context"
	"errors"
	"regexp"
	"testing"
	"time"

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

// Story 23.5 — UserCosmeticItemRepo.CreateInTx sqlmock 单测（AC1）
//
// 与 ListByUserForInventory 同测试风格（sqlmock，非 dockertest）。验证
// CreateInTx 走 GORM Create → INSERT user_cosmetic_items + GORM 回填 item.ID
// （AUTO_INCREMENT，LastInsertId）+ query 失败 raw error 透传（service 包 1009）。
// "事务内调用走 txCtx 注入的 tx 句柄" 的端到端真值在开箱事务回滚集成测试覆盖。

// TestUserCosmeticItemRepo_CreateInTx_HappyPath:
// INSERT 成功 → GORM 回填 item.ID（AUTO_INCREMENT，来自 LastInsertId）。
func TestUserCosmeticItemRepo_CreateInTx_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserCosmeticItemRepo(gormDB)

	srcRef := uint64(5001)
	item := &UserCosmeticItem{
		UserID:         7,
		CosmeticItemID: 24,
		Status:         1,
		Source:         1,
		SourceRefID:    &srcRef,
		ObtainedAt:     time.Now().UTC(),
	}

	// GORM Create → INSERT INTO `user_cosmetic_items` (...) VALUES (...)
	// （本 gormDB mock 配置 SkipDefaultTransaction —— 单条 Create 无 implicit
	// tx，直接 Exec）。回填 ID=90001 走 sqlmock LastInsertId。
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_cosmetic_items`")).
		WillReturnResult(sqlmock.NewResult(90001, 1))

	if err := repo.CreateInTx(context.Background(), item); err != nil {
		t.Fatalf("CreateInTx: %v", err)
	}
	if item.ID != 90001 {
		t.Errorf("item.ID = %d, want 90001 (GORM 回填 AUTO_INCREMENT)", item.ID)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("sqlmock expectations: %v", err)
	}
}

// TestUserCosmeticItemRepo_CreateInTx_DBError_ReturnsRawError:
// INSERT 失败 → 返 raw error 透传（不在 repo 层翻译；service 包成 1009）。
func TestUserCosmeticItemRepo_CreateInTx_DBError_ReturnsRawError(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserCosmeticItemRepo(gormDB)

	item := &UserCosmeticItem{UserID: 7, CosmeticItemID: 24, Status: 1, Source: 1, ObtainedAt: time.Now().UTC()}

	wantErr := errors.New("synthetic insert failure")
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_cosmetic_items`")).
		WillReturnError(wantErr)

	err := repo.CreateInTx(context.Background(), item)
	if err == nil {
		t.Fatal("CreateInTx err = nil, want raw error 透传")
	}
	if !errors.Is(err, wantErr) {
		t.Errorf("CreateInTx err = %v, want 透传底层 %v（repo 不翻译，service 层包 1009）", err, wantErr)
	}
}

// Story 26.3 — FindByIDForEquip / UpdateStatusInTx sqlmock 单测。

// FindByIDForEquip happy：显式 4 列 SELECT WHERE id=?（**无** user_id 过滤）。
func TestUserCosmeticItemRepo_FindByIDForEquip_Happy(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserCosmeticItemRepo(gormDB)

	rows := sqlmock.NewRows([]string{"id", "cosmetic_item_id", "status", "user_id"}).
		AddRow(uint64(90001), uint64(12), int8(1), uint64(42))
	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT id, cosmetic_item_id, status, user_id FROM `user_cosmetic_items` WHERE id = ?")).
		WithArgs(uint64(90001), 1). // id, LIMIT 1（**无** user_id arg）
		WillReturnRows(rows)

	got, err := repo.FindByIDForEquip(context.Background(), 90001)
	if err != nil {
		t.Fatalf("FindByIDForEquip: %v", err)
	}
	if got.ID != 90001 || got.CosmeticItemID != 12 || got.Status != 1 || got.UserID != 42 {
		t.Errorf("got = %+v, want id=90001 ci=12 status=1 userID=42", got)
	}
}

// FindByIDForEquip NotFound → ErrUserCosmeticItemNotFound 哨兵（service 翻 5001）。
func TestUserCosmeticItemRepo_FindByIDForEquip_NotFound_ReturnsSentinel(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserCosmeticItemRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT id, cosmetic_item_id, status, user_id FROM `user_cosmetic_items`")).
		WithArgs(uint64(99999), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	got, err := repo.FindByIDForEquip(context.Background(), 99999)
	if got != nil {
		t.Errorf("got = %+v, want nil", got)
	}
	if !errors.Is(err, ErrUserCosmeticItemNotFound) {
		t.Errorf("err = %v, want ErrUserCosmeticItemNotFound", err)
	}
}

// UpdateStatusInTx happy：UPDATE status 单字段 WHERE id=? → nil。
func TestUserCosmeticItemRepo_UpdateStatusInTx_Happy(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserCosmeticItemRepo(gormDB)

	// GORM Update("status", v) 带 updated_at autoUpdateTime → args: status, updated_at, id
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_cosmetic_items` SET")).
		WithArgs(int8(2), sqlmock.AnyArg(), uint64(90001)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.UpdateStatusInTx(context.Background(), 90001, 2); err != nil {
		t.Fatalf("UpdateStatusInTx: %v", err)
	}
}
