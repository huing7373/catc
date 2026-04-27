package mysql

import (
	"context"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// TestStepAccountRepo_Create_PrimaryKeyIsUserID:
// user_step_accounts 的 PK 是 user_id（非自增）→ Create 不依赖 LastInsertId 回填。
// 验证 Create 调用走 INSERT 且不报错。
func TestStepAccountRepo_Create_PrimaryKeyIsUserID(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewStepAccountRepo(gormDB)

	// PK = user_id 由调用方填；GORM 仍会发 INSERT
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_step_accounts`")).
		WillReturnResult(sqlmock.NewResult(0, 1))

	a := &StepAccount{
		UserID:         1001,
		TotalSteps:     0,
		AvailableSteps: 0,
		ConsumedSteps:  0,
		Version:        0,
	}
	if err := repo.Create(context.Background(), a); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if a.UserID != 1001 {
		t.Errorf("a.UserID = %d, want 1001 (PK should not be overwritten)", a.UserID)
	}
}
