package mysql

import (
	"context"
	stderrors "errors"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// TestStepSyncLogRepo_Create_HappyPath_GeneratesInsertSQL:
// Create → ExpectExec INSERT INTO `user_step_sync_logs`，sqlmock 返 (lastID=0, rows=1)。
// **PK 自增 service 层不需 ID 回填** —— LastInsertId 不参与断言。
func TestStepSyncLogRepo_Create_HappyPath_GeneratesInsertSQL(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewStepSyncLogRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_step_sync_logs`")).
		WillReturnResult(sqlmock.NewResult(101, 1))

	syncDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	log := &StepSyncLog{
		UserID:             1001,
		SyncDate:           syncDate,
		ClientTotalSteps:   100,
		AcceptedDeltaSteps: 100,
		MotionState:        2,
		Source:             1,
		ClientTs:           1714560000000,
	}
	if err := repo.Create(context.Background(), log); err != nil {
		t.Fatalf("Create: %v", err)
	}
}

// TestStepSyncLogRepo_FindLatestByUserAndDate_HappyPath:
// SELECT * FROM `user_step_sync_logs` WHERE user_id=? AND sync_date=? ORDER BY id DESC LIMIT 1
// → 1 行 → 验证返回字段完整。
func TestStepSyncLogRepo_FindLatestByUserAndDate_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewStepSyncLogRepo(gormDB)

	syncDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "sync_date", "client_total_steps",
		"accepted_delta_steps", "motion_state", "source", "client_ts", "created_at",
	}).AddRow(101, 1001, syncDate, 200, 100, 2, 1, 1714560000000, time.Now())

	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_step_sync_logs`")).
		WillReturnRows(rows)

	got, err := repo.FindLatestByUserAndDate(context.Background(), 1001, syncDate)
	if err != nil {
		t.Fatalf("FindLatestByUserAndDate: %v", err)
	}
	if got == nil {
		t.Fatal("got nil, want non-nil StepSyncLog")
	}
	if got.UserID != 1001 {
		t.Errorf("UserID = %d, want 1001", got.UserID)
	}
	if got.ClientTotalSteps != 200 {
		t.Errorf("ClientTotalSteps = %d, want 200", got.ClientTotalSteps)
	}
	if got.AcceptedDeltaSteps != 100 {
		t.Errorf("AcceptedDeltaSteps = %d, want 100", got.AcceptedDeltaSteps)
	}
	if got.MotionState != 2 {
		t.Errorf("MotionState = %d, want 2", got.MotionState)
	}
	if got.Source != 1 {
		t.Errorf("Source = %d, want 1", got.Source)
	}
}

// TestStepSyncLogRepo_FindLatestByUserAndDate_NotFound_ReturnsErrStepSyncLogNotFound:
// 0 行 → repo 必须翻译 gorm.ErrRecordNotFound → ErrStepSyncLogNotFound 哨兵。
func TestStepSyncLogRepo_FindLatestByUserAndDate_NotFound_ReturnsErrStepSyncLogNotFound(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewStepSyncLogRepo(gormDB)

	syncDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{"id"}) // 0 行 → GORM First 抛 ErrRecordNotFound
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_step_sync_logs`")).
		WillReturnRows(rows)

	got, err := repo.FindLatestByUserAndDate(context.Background(), 9999, syncDate)
	if got != nil {
		t.Errorf("got = %+v, want nil on NotFound", got)
	}
	if !stderrors.Is(err, ErrStepSyncLogNotFound) {
		t.Errorf("err = %v, want ErrStepSyncLogNotFound", err)
	}
}

// TestStepSyncLogRepo_SumAcceptedDeltaByUserAndDate_HappyPath:
// SELECT COALESCE(SUM(accepted_delta_steps), 0) → mock 返 sum=49000；验返 int64(49000)。
// 同时验证 0 行（mock 返 sum=0 模拟 COALESCE 兜底）→ 返 0。
func TestStepSyncLogRepo_SumAcceptedDeltaByUserAndDate_HappyPath(t *testing.T) {
	t.Run("Has49000Accepted", func(t *testing.T) {
		gormDB, mock := newGormWithMock(t)
		repo := NewStepSyncLogRepo(gormDB)

		syncDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		rows := sqlmock.NewRows([]string{"sum"}).AddRow(49000)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(SUM(accepted_delta_steps), 0)")).
			WillReturnRows(rows)

		got, err := repo.SumAcceptedDeltaByUserAndDate(context.Background(), 1001, syncDate)
		if err != nil {
			t.Fatalf("SumAcceptedDeltaByUserAndDate: %v", err)
		}
		if got != 49000 {
			t.Errorf("got = %d, want 49000", got)
		}
	})

	t.Run("ZeroFallback", func(t *testing.T) {
		gormDB, mock := newGormWithMock(t)
		repo := NewStepSyncLogRepo(gormDB)

		syncDate := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
		// COALESCE 兜底场景：当日无任何 sync_log → SQL 仍返 1 行 sum=0
		rows := sqlmock.NewRows([]string{"sum"}).AddRow(0)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(SUM(accepted_delta_steps), 0)")).
			WillReturnRows(rows)

		got, err := repo.SumAcceptedDeltaByUserAndDate(context.Background(), 9999, syncDate)
		if err != nil {
			t.Fatalf("SumAcceptedDeltaByUserAndDate: %v", err)
		}
		if got != 0 {
			t.Errorf("got = %d, want 0 (COALESCE 兜底)", got)
		}
	})
}
