package mysql

import (
	"context"
	stderrors "errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
)

// TestAuthBindingRepo_Create_DuplicateEntry_ReturnsErrAuthBindingDuplicate:
// 模拟 MySQL ER_DUP_ENTRY 1062（uk_auth_type_identifier 冲突）→
// repo 必须翻译为 ErrAuthBindingDuplicate 哨兵 error。
func TestAuthBindingRepo_Create_DuplicateEntry_ReturnsErrAuthBindingDuplicate(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewAuthBindingRepo(gormDB)

	dupErr := &mysql.MySQLError{
		Number:  1062,
		Message: "Duplicate entry 'guest-uid-x' for key 'uk_auth_type_identifier'",
	}
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_auth_bindings`")).
		WillReturnError(dupErr)

	err := repo.Create(context.Background(), &AuthBinding{
		UserID:         42,
		AuthType:       1,
		AuthIdentifier: "guest-uid-x",
	})
	if !stderrors.Is(err, ErrAuthBindingDuplicate) {
		t.Errorf("err = %v, want ErrAuthBindingDuplicate (ER_DUP_ENTRY 1062 应被翻译)", err)
	}
}

// TestAuthBindingRepo_FindByGuestUID_NotFound_ReturnsErrAuthBindingNotFound:
// SELECT 返空 → 返 ErrAuthBindingNotFound 哨兵（service 用此分支区分首次 vs 复用）
func TestAuthBindingRepo_FindByGuestUID_NotFound_ReturnsErrAuthBindingNotFound(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewAuthBindingRepo(gormDB)

	rows := sqlmock.NewRows([]string{"id", "user_id"})
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_auth_bindings`")).
		WithArgs(int8(AuthTypeGuest), "uid-missing", 1).
		WillReturnRows(rows)

	got, err := repo.FindByGuestUID(context.Background(), "uid-missing")
	if got != nil {
		t.Errorf("got = %+v, want nil on NotFound", got)
	}
	if !stderrors.Is(err, ErrAuthBindingNotFound) {
		t.Errorf("err = %v, want ErrAuthBindingNotFound", err)
	}
}
