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

	// SyncDate 是 string YYYY-MM-DD（Story 7.3 review r2 [P2]：repo 全程不走 time.Time）
	log := &StepSyncLog{
		UserID:             1001,
		SyncDate:           "2026-05-01",
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
// SELECT * FROM `user_step_sync_logs` WHERE user_id=? AND sync_date=?
// ORDER BY id DESC LIMIT 1
// → 1 行 → 验证返回字段完整。
//
// **ORDER BY**（Story 7.3 review r3 [P1]）：基线 = 最近 INSERT（id DESC），乱序到达
// 由 service 层 SUM 兜底处理。r2 用 client_total_steps DESC 在 HealthKit reset 场景
// 下永久卡死，已退回。详见 step_sync_log_repo.go interface doc r1→r2→r3 决策史。
func TestStepSyncLogRepo_FindLatestByUserAndDate_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewStepSyncLogRepo(gormDB)

	const syncDate = "2026-05-01"
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

// TestStepSyncLogRepo_FindLatestByUserAndDate_OrderByIDDesc_BaselineSQLAssertion:
// 验证 SQL 包含 `ORDER BY id DESC`（Story 7.3 review r3 [P1]）。
//
// **r1→r2→r3 决策史**：
//   - r1 ORDER BY id DESC（最近 INSERT）—— 乱序场景重复入账
//   - r2 ORDER BY client_total_steps DESC, id DESC（最大累计）—— HealthKit
//     reset/correction 永久卡死
//   - r3 退回 ORDER BY id DESC + service 层 SUM 兜底（详见 step_service.go
//     SyncSteps 步骤 (3)）
//
// 本断言锁住"基线 = 最近 INSERT"语义，防 regression 改回 `client_total_steps DESC`。
// 同时 service 层有专门 SUM 兜底单测覆盖乱序场景（step_service_test.go 14/15）。
func TestStepSyncLogRepo_FindLatestByUserAndDate_OrderByIDDesc_BaselineSQLAssertion(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewStepSyncLogRepo(gormDB)

	const syncDate = "2026-05-01"
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "sync_date", "client_total_steps",
		"accepted_delta_steps", "motion_state", "source", "client_ts", "created_at",
	}).AddRow(101, 1001, syncDate, 250, 50, 2, 1, 1714560000000, time.Now())

	// 关键 SQL 片段断言：必须包含 ORDER BY `id` DESC（防 regression 改回 client_total_steps）
	// GORM 会把列名 quote 成反引号；正则匹两种形式以兼容 GORM 版本差异。
	mock.ExpectQuery("ORDER BY (`id`|id) DESC").
		WillReturnRows(rows)

	if _, err := repo.FindLatestByUserAndDate(context.Background(), 1001, syncDate); err != nil {
		t.Fatalf("FindLatestByUserAndDate: %v", err)
	}
}

// TestStepSyncLogRepo_FindLatestByUserAndDate_NotFound_ReturnsErrStepSyncLogNotFound:
// 0 行 → repo 必须翻译 gorm.ErrRecordNotFound → ErrStepSyncLogNotFound 哨兵。
func TestStepSyncLogRepo_FindLatestByUserAndDate_NotFound_ReturnsErrStepSyncLogNotFound(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewStepSyncLogRepo(gormDB)

	rows := sqlmock.NewRows([]string{"id"}) // 0 行 → GORM First 抛 ErrRecordNotFound
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_step_sync_logs`")).
		WillReturnRows(rows)

	got, err := repo.FindLatestByUserAndDate(context.Background(), 9999, "2026-05-01")
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

		rows := sqlmock.NewRows([]string{"sum"}).AddRow(49000)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(SUM(accepted_delta_steps), 0)")).
			WillReturnRows(rows)

		got, err := repo.SumAcceptedDeltaByUserAndDate(context.Background(), 1001, "2026-05-01")
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

		// COALESCE 兜底场景：当日无任何 sync_log → SQL 仍返 1 行 sum=0
		rows := sqlmock.NewRows([]string{"sum"}).AddRow(0)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT COALESCE(SUM(accepted_delta_steps), 0)")).
			WillReturnRows(rows)

		got, err := repo.SumAcceptedDeltaByUserAndDate(context.Background(), 9999, "2026-05-01")
		if err != nil {
			t.Fatalf("SumAcceptedDeltaByUserAndDate: %v", err)
		}
		if got != 0 {
			t.Errorf("got = %d, want 0 (COALESCE 兜底)", got)
		}
	})
}
