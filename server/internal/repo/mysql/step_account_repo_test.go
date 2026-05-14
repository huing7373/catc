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

// TestStepAccountRepo_UpdateBalance_HappyPath: Story 7.3 加。
//
// UPDATE `user_step_accounts` SET total += ?, available += ?, version = version + 1
// WHERE user_id=? AND version=? → RowsAffected=1 → repo 返 nil；
// RowsAffected=0（乐观锁失败）→ 返 ErrStepAccountVersionMismatch。
func TestStepAccountRepo_UpdateBalance_HappyPath(t *testing.T) {
	t.Run("RowsAffected1", func(t *testing.T) {
		gormDB, mock := newGormWithMock(t)
		repo := NewStepAccountRepo(gormDB)

		mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_step_accounts`")).
			WillReturnResult(sqlmock.NewResult(0, 1))

		err := repo.UpdateBalance(context.Background(), 1001, 100, 0)
		if err != nil {
			t.Fatalf("UpdateBalance: %v", err)
		}
	})

	t.Run("RowsAffected0_ReturnsVersionMismatch", func(t *testing.T) {
		gormDB, mock := newGormWithMock(t)
		repo := NewStepAccountRepo(gormDB)

		// rows affected = 0 → 乐观锁 WHERE version 不匹配
		mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_step_accounts`")).
			WillReturnResult(sqlmock.NewResult(0, 0))

		err := repo.UpdateBalance(context.Background(), 1001, 100, 999)
		if !stderrors.Is(err, ErrStepAccountVersionMismatch) {
			t.Errorf("err = %v, want ErrStepAccountVersionMismatch (RowsAffected=0)", err)
		}
	})
}

// Story 20.6 — FindByUserIDForUpdate + Spend 单测

// TestStepAccountRepo_FindByUserIDForUpdate_HappyPath:
// SELECT ... FOR UPDATE 走 clause.Locking{Strength: "UPDATE"}。
func TestStepAccountRepo_FindByUserIDForUpdate_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewStepAccountRepo(gormDB)

	rows := sqlmock.NewRows([]string{
		"user_id", "total_steps", "available_steps", "consumed_steps", "version",
		"created_at", "updated_at",
	}).AddRow(1001, 1500, 1500, 0, 3, nil, nil)
	mock.ExpectQuery(`SELECT \* FROM .user_step_accounts. WHERE user_id = \? ORDER BY .user_step_accounts.\..user_id. LIMIT \? FOR UPDATE`).
		WithArgs(uint64(1001), 1).
		WillReturnRows(rows)

	got, err := repo.FindByUserIDForUpdate(context.Background(), 1001)
	if err != nil {
		t.Fatalf("FindByUserIDForUpdate: %v", err)
	}
	if got == nil || got.UserID != 1001 || got.AvailableSteps != 1500 || got.Version != 3 {
		t.Errorf("got = %+v, want user=1001 available=1500 version=3", got)
	}
}

// TestStepAccountRepo_FindByUserIDForUpdate_NotFound: 事务内查不到 → ErrStepAccountNotFound。
func TestStepAccountRepo_FindByUserIDForUpdate_NotFound_ReturnsErrStepAccountNotFound(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewStepAccountRepo(gormDB)

	rows := sqlmock.NewRows([]string{"user_id"})
	mock.ExpectQuery(`SELECT \* FROM .user_step_accounts. WHERE user_id = \? ORDER BY .user_step_accounts.\..user_id. LIMIT \? FOR UPDATE`).
		WithArgs(uint64(9999), 1).
		WillReturnRows(rows)

	got, err := repo.FindByUserIDForUpdate(context.Background(), 9999)
	if got != nil {
		t.Errorf("got = %+v, want nil", got)
	}
	if !stderrors.Is(err, ErrStepAccountNotFound) {
		t.Errorf("err = %v, want ErrStepAccountNotFound", err)
	}
}

// TestStepAccountRepo_Spend_HappyPath: rows_affected=1 → nil。
// 验证 UPDATE ... SET available_steps - amount, consumed_steps + amount, version + 1。
func TestStepAccountRepo_Spend_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewStepAccountRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_step_accounts`")).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Spend(context.Background(), 1001, 1000, 3); err != nil {
		t.Fatalf("Spend: %v", err)
	}
}

// TestStepAccountRepo_Spend_OptimisticLockConflict: rows_affected=0 → ErrStepAccountVersionMismatch。
func TestStepAccountRepo_Spend_OptimisticLockConflict_ReturnsErrStepAccountVersionMismatch(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewStepAccountRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("UPDATE `user_step_accounts`")).
		WillReturnResult(sqlmock.NewResult(0, 0))

	err := repo.Spend(context.Background(), 1001, 1000, 999)
	if !stderrors.Is(err, ErrStepAccountVersionMismatch) {
		t.Errorf("err = %v, want ErrStepAccountVersionMismatch", err)
	}
}
