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

// TestChestRepo_FindByUserIDForUpdate_HappyPath: Story 20.6 引入。
// SELECT ... FOR UPDATE 走 clause.Locking{Strength: "UPDATE"}。
func TestChestRepo_FindByUserIDForUpdate_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewChestRepo(gormDB)

	utcUnlock := time.Now().UTC().Add(-1 * time.Minute) // unlockable 场景
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "status", "unlock_at", "open_cost_steps", "version",
		"created_at", "updated_at",
	}).AddRow(5001, 1001, 1, utcUnlock, 1000, 3, nil, nil)
	// SQL 含 "FOR UPDATE" 关键字（clause.Locking{Strength: "UPDATE"} 路径）
	mock.ExpectQuery(`SELECT \* FROM .user_chests. WHERE user_id = \? ORDER BY .user_chests.\..id. LIMIT \? FOR UPDATE`).
		WithArgs(uint64(1001), 1).
		WillReturnRows(rows)

	got, err := repo.FindByUserIDForUpdate(context.Background(), 1001)
	if err != nil {
		t.Fatalf("FindByUserIDForUpdate: %v", err)
	}
	if got == nil || got.ID != 5001 || got.UserID != 1001 || got.Version != 3 {
		t.Errorf("got = %+v, want id=5001 user=1001 version=3", got)
	}
}

// TestChestRepo_FindByUserIDForUpdate_NotFound_ReturnsErrChestNotFound:
// 事务内查不到 → ErrChestNotFound（service 层翻译为 4001）
func TestChestRepo_FindByUserIDForUpdate_NotFound_ReturnsErrChestNotFound(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewChestRepo(gormDB)

	rows := sqlmock.NewRows([]string{"id"}) // 0 行
	mock.ExpectQuery(`SELECT \* FROM .user_chests. WHERE user_id = \? ORDER BY .user_chests.\..id. LIMIT \? FOR UPDATE`).
		WithArgs(uint64(9999), 1).
		WillReturnRows(rows)

	got, err := repo.FindByUserIDForUpdate(context.Background(), 9999)
	if got != nil {
		t.Errorf("got = %+v, want nil on NotFound", got)
	}
	if !stderrors.Is(err, ErrChestNotFound) {
		t.Errorf("err = %v, want ErrChestNotFound", err)
	}
}

// TestChestRepo_Delete_HappyPath: Story 20.6 引入。
// DELETE FROM user_chests WHERE id = ?。
func TestChestRepo_Delete_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewChestRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `user_chests`")).
		WithArgs(uint64(5001)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.Delete(context.Background(), 5001); err != nil {
		t.Fatalf("Delete: %v", err)
	}
}

// TestChestRepo_UpdateUnlockAtByID_HappyPath_UpdatesRowsAffectedOne: Story 20.7 引入；
// review r1 [P2] 改造 —— 签名从 UpdateUnlockAt(userID) 改为 UpdateUnlockAtByID(chestID)，
// WHERE 子句从 user_id 改为 id，防止与 OpenChest 并发时跑偏到 next chest。
//
// UPDATE user_chests SET unlock_at = ? WHERE id = ? → rows_affected=1 → 返 nil。
//
// GORM .Update("unlock_at", ...) 会生成 SET unlock_at=?, updated_at=? WHERE id=?，
// 参数顺序 (newUnlockAt, updated_at(auto), chestID)。
func TestChestRepo_UpdateUnlockAtByID_HappyPath_UpdatesRowsAffectedOne(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewChestRepo(gormDB)

	newUnlockAt := time.Now().UTC().Add(-1 * time.Minute) // 模拟 dev force-unlock now()

	mock.ExpectExec("UPDATE `user_chests` SET").
		WithArgs(newUnlockAt, sqlmock.AnyArg() /* updated_at auto */, uint64(5001)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.UpdateUnlockAtByID(context.Background(), 5001, newUnlockAt); err != nil {
		t.Fatalf("UpdateUnlockAtByID: %v", err)
	}
}

// TestChestRepo_UpdateUnlockAtByID_ChestNotFound_ReturnsSentinel: Story 20.7 edge case；
// review r1 [P2] 改造后签名。
// rows_affected=0 → 返 ErrChestNotFound 哨兵（service 层 errors.Is 后翻译 1003）。
func TestChestRepo_UpdateUnlockAtByID_ChestNotFound_ReturnsSentinel(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewChestRepo(gormDB)

	mock.ExpectExec("UPDATE `user_chests` SET").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), uint64(99999)).
		WillReturnResult(sqlmock.NewResult(0, 0)) // rows_affected=0

	err := repo.UpdateUnlockAtByID(context.Background(), 99999, time.Now().UTC())
	if !stderrors.Is(err, ErrChestNotFound) {
		t.Errorf("err = %v, want ErrChestNotFound", err)
	}
}
