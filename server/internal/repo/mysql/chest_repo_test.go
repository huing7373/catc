package mysql

import (
	"context"
	stderrors "errors"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// TestChestRepo_Create_StoresUTCTime:
// 验证 chest.UnlockAt 使用 UTC 时区时 INSERT 成功 + repo 回填 ID。
// 这是 V1 §2.5 钦定 ISO 8601 UTC 在 service 层的延伸校验：service 必须用
// time.Now().UTC()，repo 只做 CRUD 不再二次校验时区。
func TestChestRepo_Create_StoresUTCTime(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewChestRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `user_chests`")).
		WillReturnResult(sqlmock.NewResult(3001, 1))

	utcUnlock := time.Now().UTC().Add(10 * time.Minute)
	c := &UserChest{
		UserID:        1001,
		Status:        1,
		UnlockAt:      utcUnlock,
		OpenCostSteps: 1000,
		Version:       0,
	}
	if err := repo.Create(context.Background(), c); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if c.ID != 3001 {
		t.Errorf("c.ID = %d, want 3001", c.ID)
	}
	// 关键：UnlockAt 仍保持 UTC（repo Create 不应改时区）
	if loc := c.UnlockAt.Location(); loc.String() != "UTC" {
		t.Errorf("c.UnlockAt location = %q, want UTC", loc.String())
	}
}

// TestChestRepo_FindByUserID_HappyPath:
// SELECT user_chests WHERE user_id = ? 返 1 行 → 验证字段填充。
func TestChestRepo_FindByUserID_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewChestRepo(gormDB)

	utcUnlock := time.Now().UTC().Add(10 * time.Minute)
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "status", "unlock_at", "open_cost_steps", "version",
		"created_at", "updated_at",
	}).AddRow(5001, 1001, 1, utcUnlock, 1000, 0, nil, nil)
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_chests`")).
		WithArgs(uint64(1001), 1).
		WillReturnRows(rows)

	got, err := repo.FindByUserID(context.Background(), 1001)
	if err != nil {
		t.Fatalf("FindByUserID: %v", err)
	}
	if got == nil {
		t.Fatal("got nil, want non-nil UserChest")
	}
	if got.ID != 5001 {
		t.Errorf("ID = %d, want 5001", got.ID)
	}
	if got.UserID != 1001 {
		t.Errorf("UserID = %d, want 1001", got.UserID)
	}
	if got.Status != 1 {
		t.Errorf("Status = %d, want 1 (counting)", got.Status)
	}
	if got.OpenCostSteps != 1000 {
		t.Errorf("OpenCostSteps = %d, want 1000", got.OpenCostSteps)
	}
	// UnlockAt 应保留传入的 UTC 时刻（容忍秒级精度）
	if delta := got.UnlockAt.Sub(utcUnlock); delta < -time.Second || delta > time.Second {
		t.Errorf("UnlockAt = %v, want ~%v", got.UnlockAt, utcUnlock)
	}
}

// TestChestRepo_FindByUserID_NotFound_ReturnsErrChestNotFound:
// 查不到行 → repo 翻译 gorm.ErrRecordNotFound 为 ErrChestNotFound 哨兵。
func TestChestRepo_FindByUserID_NotFound_ReturnsErrChestNotFound(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewChestRepo(gormDB)

	rows := sqlmock.NewRows([]string{"id"}) // 0 行
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `user_chests`")).
		WithArgs(uint64(9999), 1).
		WillReturnRows(rows)

	got, err := repo.FindByUserID(context.Background(), 9999)
	if got != nil {
		t.Errorf("got = %+v, want nil on NotFound", got)
	}
	if !stderrors.Is(err, ErrChestNotFound) {
		t.Errorf("err = %v, want ErrChestNotFound", err)
	}
}
