package mysql

import (
	"context"
	stderrors "errors"
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

// TestStepAccountRepo_FindByUserID_HappyPath:
// SELECT user_step_accounts WHERE user_id = ? 返 1 行 → 验证字段填充 + 走索引。
func TestStepAccountRepo_FindByUserID_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewStepAccountRepo(gormDB)

	rows := sqlmock.NewRows([]string{
		"user_id", "total_steps", "available_steps", "consumed_steps", "version",
		"created_at", "updated_at",
	}).AddRow(1001, 12345, 6789, 5556, 3, nil, nil)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_step_accounts`")).
		WithArgs(uint64(1001), 1).
		WillReturnRows(rows)

	got, err := repo.FindByUserID(context.Background(), 1001)
	if err != nil {
		t.Fatalf("FindByUserID: %v", err)
	}
	if got == nil {
		t.Fatal("got nil, want non-nil StepAccount")
	}
	if got.UserID != 1001 {
		t.Errorf("UserID = %d, want 1001", got.UserID)
	}
	if got.TotalSteps != 12345 {
		t.Errorf("TotalSteps = %d, want 12345", got.TotalSteps)
	}
	if got.AvailableSteps != 6789 {
		t.Errorf("AvailableSteps = %d, want 6789", got.AvailableSteps)
	}
	if got.ConsumedSteps != 5556 {
		t.Errorf("ConsumedSteps = %d, want 5556", got.ConsumedSteps)
	}
}

// TestStepAccountRepo_FindByUserID_NotFound_ReturnsErrStepAccountNotFound:
// 查不到行 → repo 必须翻译 gorm.ErrRecordNotFound 为 ErrStepAccountNotFound 哨兵。
func TestStepAccountRepo_FindByUserID_NotFound_ReturnsErrStepAccountNotFound(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewStepAccountRepo(gormDB)

	rows := sqlmock.NewRows([]string{"user_id"}) // 0 行 → GORM First 抛 ErrRecordNotFound
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_step_accounts`")).
		WithArgs(uint64(9999), 1).
		WillReturnRows(rows)

	got, err := repo.FindByUserID(context.Background(), 9999)
	if got != nil {
		t.Errorf("got = %+v, want nil on NotFound", got)
	}
	if !stderrors.Is(err, ErrStepAccountNotFound) {
		t.Errorf("err = %v, want ErrStepAccountNotFound", err)
	}
}
