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
