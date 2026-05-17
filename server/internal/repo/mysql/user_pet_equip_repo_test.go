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
