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

// TestChestRepo_FindByID_HappyPath: Story 20.7 review r2 [P2] 引入。
// SELECT user_chests WHERE id = ? 返 1 行 → 验证字段填充。
func TestChestRepo_FindByID_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewChestRepo(gormDB)

	utcUnlock := time.Now().UTC().Add(10 * time.Minute)
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "status", "unlock_at", "open_cost_steps", "version",
		"created_at", "updated_at",
	}).AddRow(5001, 1001, 1, utcUnlock, 1000, 0, nil, nil)
	mock.ExpectQuery(`SELECT \* FROM .user_chests. WHERE id = \? ORDER BY .user_chests.\..id. LIMIT \?`).
		WithArgs(uint64(5001), 1).
		WillReturnRows(rows)

	got, err := repo.FindByID(context.Background(), 5001)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got == nil || got.ID != 5001 || got.UserID != 1001 {
		t.Errorf("got = %+v, want id=5001 user_id=1001", got)
	}
}

// TestChestRepo_FindByID_NotFound_ReturnsErrChestNotFound: Story 20.7 review r2 [P2] 引入。
func TestChestRepo_FindByID_NotFound_ReturnsErrChestNotFound(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewChestRepo(gormDB)

	rows := sqlmock.NewRows([]string{"id"}) // 0 行
	mock.ExpectQuery(`SELECT \* FROM .user_chests. WHERE id = \? ORDER BY .user_chests.\..id. LIMIT \?`).
		WithArgs(uint64(99999), 1).
		WillReturnRows(rows)

	got, err := repo.FindByID(context.Background(), 99999)
	if got != nil {
		t.Errorf("got = %+v, want nil on NotFound", got)
	}
	if !stderrors.Is(err, ErrChestNotFound) {
		t.Errorf("err = %v, want ErrChestNotFound", err)
	}
}

// TestChestRepo_UpdateUnlockAtByID_HappyPath_RowsAffectedOne: Story 20.7 引入；
// review r2 [P2] 改造 —— 不再依赖 RowsAffected 区分 NotFound。
//
// UPDATE user_chests SET unlock_at = ? WHERE id = ? → rows_affected=1 → 返 nil。
func TestChestRepo_UpdateUnlockAtByID_HappyPath_RowsAffectedOne(t *testing.T) {
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

// TestChestRepo_UpdateUnlockAtByID_RowsAffectedZero_ReturnsNil: Story 20.7 review r4 [P2] 改造。
//
// **r2 → r3 → r4 改造路径**：
//   - r2 在 rows_affected=0 返 nil（顾虑同毫秒重复 unlock 同 chest 值未变误判），引入二阶 race false success；
//   - r3 重新加回 RowsAffected==0 → ErrChestNotFound 翻译（修 r2 二阶 race），但引入"同毫秒重复 unlock 同 chest
//     误报 1003"bug（unlock_at 列 DATETIME(3) 毫秒精度，两次 unlock 落同毫秒 → 值未变 → rows_affected=0）；
//   - r4 跳出 over-correction chain：repo 不再用 RowsAffected==0 判 NotFound；caller（dev_chest_service）改用
//     事务 + FindByIDForUpdate + UpdateUnlockAtByID 三件套，行存在性由 FOR UPDATE 行锁保证，RowsAffected==0
//     唯一来源 = 值未变 = success。
//
// 本 case 校验 r4 实装：rows_affected=0 → 返 nil（不再翻译为 ErrChestNotFound）。
// caller 侧由事务保证 chest 存在（详见 chest_repo.go UpdateUnlockAtByID interface doc r4 改造说明）。
func TestChestRepo_UpdateUnlockAtByID_RowsAffectedZero_ReturnsNil(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewChestRepo(gormDB)

	mock.ExpectExec("UPDATE `user_chests` SET").
		WithArgs(sqlmock.AnyArg(), sqlmock.AnyArg(), uint64(5001)).
		WillReturnResult(sqlmock.NewResult(0, 0)) // rows_affected=0 → r4 [P2] 不再翻译为 NotFound，返 nil

	err := repo.UpdateUnlockAtByID(context.Background(), 5001, time.Now().UTC())
	if err != nil {
		t.Errorf("err = %v, want nil (r4 [P2]: RowsAffected==0 = 值未变 = success；NotFound 由 caller 事务内 FindByIDForUpdate 保证)", err)
	}
}

// TestChestRepo_FindByIDForUpdate_HappyPath: Story 20.7 review r4 [P2] 引入。
//
// SELECT * FROM user_chests WHERE id = ? FOR UPDATE 走 clause.Locking{Strength: "UPDATE"} 路径。
// 与 FindByUserIDForUpdate 同模式，但 Where 子句用 PRIMARY KEY id（dev_chest_service r4 改造的依赖：
// client 通过 GET /chest/current 拿到 chest.id，service 用本方法在事务内锁定行）。
func TestChestRepo_FindByIDForUpdate_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewChestRepo(gormDB)

	utcUnlock := time.Now().UTC().Add(10 * time.Minute)
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "status", "unlock_at", "open_cost_steps", "version",
		"created_at", "updated_at",
	}).AddRow(5001, 1001, 1, utcUnlock, 1000, 0, nil, nil)
	// SQL 含 "FOR UPDATE" 关键字 + Where 用 id（区别 FindByUserIDForUpdate 的 user_id）
	mock.ExpectQuery(`SELECT \* FROM .user_chests. WHERE id = \? ORDER BY .user_chests.\..id. LIMIT \? FOR UPDATE`).
		WithArgs(uint64(5001), 1).
		WillReturnRows(rows)

	got, err := repo.FindByIDForUpdate(context.Background(), 5001)
	if err != nil {
		t.Fatalf("FindByIDForUpdate: %v", err)
	}
	if got == nil || got.ID != 5001 || got.UserID != 1001 {
		t.Errorf("got = %+v, want id=5001 user=1001", got)
	}
}

// TestChestRepo_FindByIDForUpdate_NotFound_ReturnsErrChestNotFound: Story 20.7 review r4 [P2] 引入。
//
// 事务内 FOR UPDATE 查不到 → ErrChestNotFound 哨兵（service 层翻译为 1003）。
func TestChestRepo_FindByIDForUpdate_NotFound_ReturnsErrChestNotFound(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewChestRepo(gormDB)

	rows := sqlmock.NewRows([]string{"id"}) // 0 行
	mock.ExpectQuery(`SELECT \* FROM .user_chests. WHERE id = \? ORDER BY .user_chests.\..id. LIMIT \? FOR UPDATE`).
		WithArgs(uint64(99999), 1).
		WillReturnRows(rows)

	got, err := repo.FindByIDForUpdate(context.Background(), 99999)
	if got != nil {
		t.Errorf("got = %+v, want nil on NotFound", got)
	}
	if !stderrors.Is(err, ErrChestNotFound) {
		t.Errorf("err = %v, want ErrChestNotFound", err)
	}
}
