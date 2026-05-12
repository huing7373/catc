package mysql

import (
	"context"
	stderrors "errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// TestPetRepo_Create_AssignsAutoIncrementID:
// INSERT pets → LastInsertId=2001 → 验证 p.ID 被回填。
func TestPetRepo_Create_AssignsAutoIncrementID(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewPetRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `pets`")).
		WillReturnResult(sqlmock.NewResult(2001, 1))

	p := &Pet{
		UserID:       1001,
		PetType:      1,
		Name:         "默认小猫",
		CurrentState: 1,
		IsDefault:    1,
	}
	if err := repo.Create(context.Background(), p); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if p.ID != 2001 {
		t.Errorf("p.ID = %d, want 2001", p.ID)
	}
}

// TestPetRepo_FindDefaultByUserID_NotFound_ReturnsErrPetNotFound:
// SELECT 返空 → ErrPetNotFound 哨兵。
func TestPetRepo_FindDefaultByUserID_NotFound_ReturnsErrPetNotFound(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewPetRepo(gormDB)

	rows := sqlmock.NewRows([]string{"id", "user_id"})
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `pets`")).
		WithArgs(uint64(999), 1, 1). // user_id, is_default=1, LIMIT 1
		WillReturnRows(rows)

	got, err := repo.FindDefaultByUserID(context.Background(), 999)
	if got != nil {
		t.Errorf("got = %+v, want nil", got)
	}
	if !stderrors.Is(err, ErrPetNotFound) {
		t.Errorf("err = %v, want ErrPetNotFound", err)
	}
}

// TestPetRepo_UpdateCurrentStateByID_Happy:
// 验证 Update("current_state", state) 单字段路径在 happy case 下：
//   - 命中 UPDATE `pets` SET ... WHERE id = ? 模式
//   - args 严格匹配 (state, sqlmock.AnyArg() /* updated_at */, petID)
//   - 返 sqlmock.NewResult(0, 1) → repo 透传 nil error
//
// **不**新增 RowsAffected 相关 case（与 V1 §5.2 + r1 / r6 / r9 lessons 锁定一致：
// service 层不读 RowsAffected，repo 层也不该为 RowsAffected 分流）。
func TestPetRepo_UpdateCurrentStateByID_Happy(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewPetRepo(gormDB)

	// GORM Update("current_state", v) 会带 updated_at 自动列（autoUpdateTime tag）;
	// args 顺序：current_state, updated_at, id（与 user_repo.UpdateNickname 同模式）
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `pets` SET")).
		WithArgs(int8(2), sqlmock.AnyArg(), uint64(100)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.UpdateCurrentStateByID(context.Background(), 100, 2); err != nil {
		t.Fatalf("UpdateCurrentStateByID: %v", err)
	}
}

// TestPetRepo_UpdateCurrentStateByID_DBError:
// 模拟 driver 层错误（如连接断 / 死锁）→ repo 透传 raw error；
// service 层用 apperror.Wrap 包成 1009（service 层单测验证）。
func TestPetRepo_UpdateCurrentStateByID_DBError(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewPetRepo(gormDB)

	dbErr := stderrors.New("connection refused")
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `pets` SET")).
		WithArgs(int8(3), sqlmock.AnyArg(), uint64(200)).
		WillReturnError(dbErr)

	err := repo.UpdateCurrentStateByID(context.Background(), 200, 3)
	if err == nil {
		t.Fatal("UpdateCurrentStateByID: expected DB error, got nil")
	}
	if !stderrors.Is(err, dbErr) {
		t.Errorf("err = %v, want connection refused", err)
	}
}
