package mysql

import (
	"context"
	stderrors "errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
)

// Story 26.3 — UserPetEquipRepo sqlmock 单测（FindByPetSlot / DeleteByPetSlotInTx
// / InsertInTx + 1062 双哨兵翻译）。与既有 mysql repo 单测同风格（sqlmock，
// 非 dockertest —— 真事务回滚由 cosmetic_equip_service_integration_test.go 覆盖）。

// FindByPetSlot happy：命中 1 行 → 返 *UserPetEquip。
func TestUserPetEquipRepo_FindByPetSlot_Happy(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserPetEquipRepo(gormDB)

	rows := sqlmock.NewRows([]string{"id", "user_id", "pet_id", "slot", "user_cosmetic_item_id"}).
		AddRow(uint64(5), uint64(42), uint64(2001), int8(1), uint64(90001))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_pet_equips`")).
		WithArgs(uint64(2001), int8(1), 1). // pet_id, slot, LIMIT 1
		WillReturnRows(rows)

	got, err := repo.FindByPetSlot(context.Background(), 2001, 1)
	if err != nil {
		t.Fatalf("FindByPetSlot: %v", err)
	}
	if got.ID != 5 || got.UserCosmeticItemID != 90001 || got.Slot != 1 {
		t.Errorf("got = %+v, want id=5 uci=90001 slot=1", got)
	}
}

// FindByPetSlot NotFound → ErrUserPetEquipNotFound 哨兵（slot 无装备合法 case）。
func TestUserPetEquipRepo_FindByPetSlot_NotFound_ReturnsSentinel(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserPetEquipRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_pet_equips`")).
		WithArgs(uint64(2001), int8(1), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id"}))

	got, err := repo.FindByPetSlot(context.Background(), 2001, 1)
	if got != nil {
		t.Errorf("got = %+v, want nil", got)
	}
	if !stderrors.Is(err, ErrUserPetEquipNotFound) {
		t.Errorf("err = %v, want ErrUserPetEquipNotFound", err)
	}
}

// DeleteByPetSlotInTx happy：DELETE WHERE pet_id=? AND slot=? → nil。
func TestUserPetEquipRepo_DeleteByPetSlotInTx_Happy(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserPetEquipRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `user_pet_equips`")).
		WithArgs(uint64(2001), int8(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.DeleteByPetSlotInTx(context.Background(), 2001, 1); err != nil {
		t.Fatalf("DeleteByPetSlotInTx: %v", err)
	}
}

// InsertInTx happy：INSERT → LastInsertId 回填 e.ID。
func TestUserPetEquipRepo_InsertInTx_Happy(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserPetEquipRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_pet_equips`")).
		WillReturnResult(sqlmock.NewResult(77, 1))

	e := &UserPetEquip{UserID: 42, PetID: 2001, Slot: 1, UserCosmeticItemID: 90001}
	if err := repo.InsertInTx(context.Background(), e); err != nil {
		t.Fatalf("InsertInTx: %v", err)
	}
	if e.ID != 77 {
		t.Errorf("e.ID = %d, want 77 (AUTO_INCREMENT 回填)", e.ID)
	}
}

// InsertInTx 1062 + Message 含 uk_pet_slot → ErrUserPetEquipPetSlotDuplicate。
func TestUserPetEquipRepo_InsertInTx_DupPetSlot_ReturnsSentinel(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserPetEquipRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_pet_equips`")).
		WillReturnError(&mysql.MySQLError{
			Number:  1062,
			Message: "Duplicate entry '2001-1' for key 'user_pet_equips.uk_pet_slot'",
		})

	e := &UserPetEquip{UserID: 42, PetID: 2001, Slot: 1, UserCosmeticItemID: 90001}
	err := repo.InsertInTx(context.Background(), e)
	if !stderrors.Is(err, ErrUserPetEquipPetSlotDuplicate) {
		t.Errorf("err = %v, want ErrUserPetEquipPetSlotDuplicate", err)
	}
}

// InsertInTx 1062 + Message 含 uk_user_cosmetic_item_id →
// ErrUserPetEquipItemDuplicate。
func TestUserPetEquipRepo_InsertInTx_DupItem_ReturnsSentinel(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserPetEquipRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_pet_equips`")).
		WillReturnError(&mysql.MySQLError{
			Number:  1062,
			Message: "Duplicate entry '90001' for key 'user_pet_equips.uk_user_cosmetic_item_id'",
		})

	e := &UserPetEquip{UserID: 42, PetID: 2001, Slot: 1, UserCosmeticItemID: 90001}
	err := repo.InsertInTx(context.Background(), e)
	if !stderrors.Is(err, ErrUserPetEquipItemDuplicate) {
		t.Errorf("err = %v, want ErrUserPetEquipItemDuplicate", err)
	}
}

// InsertInTx 非 1062 DB 错 → raw 透传（非哨兵）。
func TestUserPetEquipRepo_InsertInTx_OtherDBError_RawPassThrough(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserPetEquipRepo(gormDB)

	rawErr := &mysql.MySQLError{Number: 1213, Message: "Deadlock found"}
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_pet_equips`")).
		WillReturnError(rawErr)

	e := &UserPetEquip{UserID: 42, PetID: 2001, Slot: 1, UserCosmeticItemID: 90001}
	err := repo.InsertInTx(context.Background(), e)
	if stderrors.Is(err, ErrUserPetEquipPetSlotDuplicate) || stderrors.Is(err, ErrUserPetEquipItemDuplicate) {
		t.Errorf("err = %v, want raw 透传（非哨兵）", err)
	}
	if err == nil {
		t.Errorf("err = nil, want raw DB error 透传")
	}
}

// ================================================================
// Story 26.4 — unequip 专用 2 方法 sqlmock 单测
// （FindUserCosmeticItemIDByPetSlotForUpdate FOR UPDATE 行锁 Raw SQL +
// DeleteByPetSlotInTxReturningAffected 返 RowsAffected）。
// FOR UPDATE Raw SQL 用 ExpectQuery 正则匹配 `FOR UPDATE` 子串（与
// room_member_repo_test ExistsForShare FOR SHARE 测试同模式）。
// ================================================================

// FindUserCosmeticItemIDByPetSlotForUpdate happy：命中 1 行 → 返
// user_cosmetic_item_id。
func TestUserPetEquipRepo_FindUCIDByPetSlotForUpdate_Happy(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserPetEquipRepo(gormDB)

	rows := sqlmock.NewRows([]string{"user_cosmetic_item_id"}).AddRow(uint64(90001))
	mock.ExpectQuery("FOR UPDATE").
		WithArgs(uint64(2001), int8(1)).
		WillReturnRows(rows)

	got, err := repo.FindUserCosmeticItemIDByPetSlotForUpdate(context.Background(), 2001, 1)
	if err != nil {
		t.Fatalf("FindUserCosmeticItemIDByPetSlotForUpdate: %v", err)
	}
	if got != 90001 {
		t.Errorf("got = %d, want 90001", got)
	}
}

// FindUserCosmeticItemIDByPetSlotForUpdate 0 行 → ErrUserPetEquipNotFound
// 哨兵（Raw + Scan 0 行不返 gorm.ErrRecordNotFound，须显式判 RowsAffected==0）。
func TestUserPetEquipRepo_FindUCIDByPetSlotForUpdate_NotFound_ReturnsSentinel(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserPetEquipRepo(gormDB)

	mock.ExpectQuery("FOR UPDATE").
		WithArgs(uint64(2001), int8(1)).
		WillReturnRows(sqlmock.NewRows([]string{"user_cosmetic_item_id"})) // 0 行

	got, err := repo.FindUserCosmeticItemIDByPetSlotForUpdate(context.Background(), 2001, 1)
	if got != 0 {
		t.Errorf("got = %d, want 0", got)
	}
	if !stderrors.Is(err, ErrUserPetEquipNotFound) {
		t.Errorf("err = %v, want ErrUserPetEquipNotFound", err)
	}
}

// FindUserCosmeticItemIDByPetSlotForUpdate query 失败 → raw error 透传。
func TestUserPetEquipRepo_FindUCIDByPetSlotForUpdate_DBError_RawPassThrough(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserPetEquipRepo(gormDB)

	rawErr := &mysql.MySQLError{Number: 1213, Message: "Deadlock found"}
	mock.ExpectQuery("FOR UPDATE").
		WithArgs(uint64(2001), int8(1)).
		WillReturnError(rawErr)

	got, err := repo.FindUserCosmeticItemIDByPetSlotForUpdate(context.Background(), 2001, 1)
	if got != 0 {
		t.Errorf("got = %d, want 0", got)
	}
	if stderrors.Is(err, ErrUserPetEquipNotFound) {
		t.Errorf("err = %v, want raw 透传（非哨兵）", err)
	}
	if err == nil {
		t.Errorf("err = nil, want raw DB error 透传")
	}
}

// DeleteByPetSlotInTxReturningAffected 删 1 行 → (1, nil)。
func TestUserPetEquipRepo_DeleteByPetSlotReturningAffected_DeleteOne(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserPetEquipRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `user_pet_equips`")).
		WithArgs(uint64(2001), int8(1)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	n, err := repo.DeleteByPetSlotInTxReturningAffected(context.Background(), 2001, 1)
	if err != nil {
		t.Fatalf("DeleteByPetSlotInTxReturningAffected: %v", err)
	}
	if n != 1 {
		t.Errorf("rowsAffected = %d, want 1", n)
	}
}

// DeleteByPetSlotInTxReturningAffected 删 0 行 → (0, nil)（步骤 5/6 间被并发删
// 模拟；service 层据此 → 5004 回滚兜底）。
func TestUserPetEquipRepo_DeleteByPetSlotReturningAffected_DeleteZero(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserPetEquipRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `user_pet_equips`")).
		WithArgs(uint64(2001), int8(1)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	n, err := repo.DeleteByPetSlotInTxReturningAffected(context.Background(), 2001, 1)
	if err != nil {
		t.Fatalf("DeleteByPetSlotInTxReturningAffected: %v", err)
	}
	if n != 0 {
		t.Errorf("rowsAffected = %d, want 0", n)
	}
}

// DeleteByPetSlotInTxReturningAffected DB 错 → (0, err)。
func TestUserPetEquipRepo_DeleteByPetSlotReturningAffected_DBError(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserPetEquipRepo(gormDB)

	rawErr := &mysql.MySQLError{Number: 1213, Message: "Deadlock found"}
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `user_pet_equips`")).
		WithArgs(uint64(2001), int8(1)).
		WillReturnError(rawErr)

	n, err := repo.DeleteByPetSlotInTxReturningAffected(context.Background(), 2001, 1)
	if n != 0 {
		t.Errorf("rowsAffected = %d, want 0", n)
	}
	if err == nil {
		t.Errorf("err = nil, want raw DB error")
	}
}

// ================================================================
// Story 26.6 — ListEquipsForHome sqlmock 单测
// （单 SQL JOIN 主查 + 1 个 O(1) COUNT 校验查；验证 JOIN SQL + 列映射 +
// 0 行返非 nil slice + rawCount + query err 透传）。
// ================================================================

// ListEquipsForHome happy：JOIN 命中 2 行 + COUNT=2 → 2 个 HomeEquipRow，
// rawCount=2，按 slot ASC。
func TestUserPetEquipRepo_ListEquipsForHome_Happy(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserPetEquipRepo(gormDB)

	joinRows := sqlmock.NewRows([]string{"slot", "user_cosmetic_item_id", "cosmetic_item_id", "name", "rarity", "asset_url"}).
		AddRow(int8(1), uint64(90001), uint64(12), "小黄帽", int8(1), "https://cdn/hat.png").
		AddRow(int8(2), uint64(90002), uint64(34), "白手套", int8(1), "https://cdn/gloves.png")
	mock.ExpectQuery(regexp.QuoteMeta("JOIN user_cosmetic_items uci ON uci.id = upe.user_cosmetic_item_id")).
		WithArgs(uint64(42), uint64(2001)).
		WillReturnRows(joinRows)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `user_pet_equips`")).
		WithArgs(uint64(42), uint64(2001)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))

	rows, rawCount, err := repo.ListEquipsForHome(context.Background(), 42, 2001)
	if err != nil {
		t.Fatalf("ListEquipsForHome: %v", err)
	}
	if rawCount != 2 {
		t.Errorf("rawCount = %d, want 2", rawCount)
	}
	if len(rows) != 2 {
		t.Fatalf("len(rows) = %d, want 2", len(rows))
	}
	if rows[0].Slot != 1 || rows[0].UserCosmeticItemID != 90001 || rows[0].CosmeticItemID != 12 ||
		rows[0].Name != "小黄帽" || rows[0].Rarity != 1 || rows[0].AssetURL != "https://cdn/hat.png" {
		t.Errorf("rows[0] = %+v, want slot=1 uci=90001 ci=12 name=小黄帽 rarity=1 url=https://cdn/hat.png", rows[0])
	}
	if rows[1].Slot != 2 || rows[1].UserCosmeticItemID != 90002 || rows[1].CosmeticItemID != 34 {
		t.Errorf("rows[1] = %+v, want slot=2 uci=90002 ci=34", rows[1])
	}
}

// ListEquipsForHome 0 行（pet 未穿任何装备）→ 返非 nil 空 slice + rawCount=0。
func TestUserPetEquipRepo_ListEquipsForHome_Empty_ReturnsNonNilSlice(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserPetEquipRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta("JOIN user_cosmetic_items uci ON uci.id = upe.user_cosmetic_item_id")).
		WithArgs(uint64(42), uint64(2001)).
		WillReturnRows(sqlmock.NewRows([]string{"slot", "user_cosmetic_item_id", "cosmetic_item_id", "name", "rarity", "asset_url"}))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `user_pet_equips`")).
		WithArgs(uint64(42), uint64(2001)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(0)))

	rows, rawCount, err := repo.ListEquipsForHome(context.Background(), 42, 2001)
	if err != nil {
		t.Fatalf("ListEquipsForHome: %v", err)
	}
	if rows == nil {
		t.Fatal("rows = nil, want 非 nil 空 slice []HomeEquipRow{}")
	}
	if len(rows) != 0 {
		t.Errorf("len(rows) = %d, want 0", len(rows))
	}
	if rawCount != 0 {
		t.Errorf("rawCount = %d, want 0", rawCount)
	}
}

// ListEquipsForHome 配置缺失（INNER JOIN 过滤掉 1 行）→ JOIN 返 1 行但
// COUNT=2（rawCount > len(rows) 让 service 层 log warning）。
func TestUserPetEquipRepo_ListEquipsForHome_ConfigMissing_RawCountGreater(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserPetEquipRepo(gormDB)

	joinRows := sqlmock.NewRows([]string{"slot", "user_cosmetic_item_id", "cosmetic_item_id", "name", "rarity", "asset_url"}).
		AddRow(int8(1), uint64(90001), uint64(12), "小黄帽", int8(1), "https://cdn/hat.png")
	mock.ExpectQuery(regexp.QuoteMeta("JOIN user_cosmetic_items uci ON uci.id = upe.user_cosmetic_item_id")).
		WithArgs(uint64(42), uint64(2001)).
		WillReturnRows(joinRows)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `user_pet_equips`")).
		WithArgs(uint64(42), uint64(2001)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(int64(2)))

	rows, rawCount, err := repo.ListEquipsForHome(context.Background(), 42, 2001)
	if err != nil {
		t.Fatalf("ListEquipsForHome: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("len(rows) = %d, want 1 (INNER JOIN 过滤掉配置缺失行)", len(rows))
	}
	if rawCount != 2 {
		t.Errorf("rawCount = %d, want 2 (user_pet_equips 实际 2 行)", rawCount)
	}
}

// ListEquipsForHome 主 JOIN query 失败 → (nil, 0, raw error 透传)。
func TestUserPetEquipRepo_ListEquipsForHome_QueryError_RawPassThrough(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserPetEquipRepo(gormDB)

	rawErr := &mysql.MySQLError{Number: 1213, Message: "Deadlock found"}
	mock.ExpectQuery(regexp.QuoteMeta("JOIN user_cosmetic_items uci ON uci.id = upe.user_cosmetic_item_id")).
		WithArgs(uint64(42), uint64(2001)).
		WillReturnError(rawErr)

	rows, rawCount, err := repo.ListEquipsForHome(context.Background(), 42, 2001)
	if rows != nil {
		t.Errorf("rows = %+v, want nil on query error", rows)
	}
	if rawCount != 0 {
		t.Errorf("rawCount = %d, want 0 on query error", rawCount)
	}
	if err == nil {
		t.Fatal("err = nil, want raw DB error 透传")
	}
}
