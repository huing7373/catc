package mysql

import (
	"context"
	stderrors "errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	driverMysql "github.com/go-sql-driver/mysql"
	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// newGormWithMock 复用 4.2 tx/manager_test.go 的范式：sqlmock 注入 GORM。
//
// SkipInitializeWithVersion 必须开 —— GORM Open 阶段不主动 SELECT VERSION()
// 触发 unset expectation。
// SkipDefaultTransaction 与生产 db.Open 配置一致（不让单条 SQL 自动包事务）。
func newGormWithMock(t *testing.T) (*gorm.DB, sqlmock.Sqlmock) {
	t.Helper()
	sqlDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Errorf("sqlmock expectations not met: %v", err)
		}
	})
	gormDB, err := gorm.Open(mysql.New(mysql.Config{
		Conn:                      sqlDB,
		SkipInitializeWithVersion: true,
	}), &gorm.Config{
		SkipDefaultTransaction: true,
	})
	if err != nil {
		t.Fatalf("gorm.Open: %v", err)
	}
	return gormDB, mock
}

// TestUserRepo_Create_AssignsAutoIncrementID:
// INSERT users → sqlmock 返 LastInsertId=42 → 验证 u.ID 被 GORM 回填。
func TestUserRepo_Create_AssignsAutoIncrementID(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserRepo(gormDB)

	// SkipDefaultTransaction=true → 单条 INSERT 不被自动包事务
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `users`")).
		WillReturnResult(sqlmock.NewResult(42, 1))

	u := &User{
		GuestUID:  "uid-test",
		Nickname:  "",
		AvatarURL: "",
		Status:    1,
	}
	if err := repo.Create(context.Background(), u); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if u.ID != 42 {
		t.Errorf("u.ID = %d, want 42 (回填的 LastInsertId)", u.ID)
	}
}

// TestUserRepo_Create_DuplicateGuestUID_ReturnsErrUsersGuestUIDDuplicate:
// 模拟 MySQL ER_DUP_ENTRY 1062（uk_guest_uid 冲突，并发场景下另一 Tx 已 commit）→
// repo 必须翻译为 ErrUsersGuestUIDDuplicate 哨兵 error。
//
// 此 case 与 auth_binding_repo 同位 case 配对：5 表事务下两个表各有一个唯一约束
// 都可能在并发场景下抛 1062，service 层必须**两个都识别**才能保证
// "同 guestUid 重复调用 → 同一 user_id" 的幂等语义（V1 §4.1）。
func TestUserRepo_Create_DuplicateGuestUID_ReturnsErrUsersGuestUIDDuplicate(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserRepo(gormDB)

	dupErr := &driverMysql.MySQLError{
		Number:  1062,
		Message: "Duplicate entry 'uid-concurrent' for key 'uk_guest_uid'",
	}
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `users`")).
		WillReturnError(dupErr)

	err := repo.Create(context.Background(), &User{
		GuestUID:  "uid-concurrent",
		Nickname:  "",
		AvatarURL: "",
		Status:    1,
	})
	if !stderrors.Is(err, ErrUsersGuestUIDDuplicate) {
		t.Errorf("err = %v, want ErrUsersGuestUIDDuplicate (ER_DUP_ENTRY 1062 应被翻译)", err)
	}
}

// TestUserRepo_FindByID_NotFound_ReturnsErrUserNotFound:
// SELECT users WHERE id = ? 返空 → repo 返 ErrUserNotFound 哨兵。
func TestUserRepo_FindByID_NotFound_ReturnsErrUserNotFound(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserRepo(gormDB)

	rows := sqlmock.NewRows([]string{"id", "guest_uid", "nickname"}) // 空 rowset
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `users`")).
		WithArgs(uint64(999), 1).
		WillReturnRows(rows)

	got, err := repo.FindByID(context.Background(), 999)
	if got != nil {
		t.Errorf("got = %+v, want nil on NotFound", got)
	}
	if !stderrors.Is(err, ErrUserNotFound) {
		t.Errorf("err = %v, want ErrUserNotFound", err)
	}
}

// ============================================================
// Story 11.3 新增：UserRepo.UpdateCurrentRoomID 路径覆盖
// ============================================================

// TestUserRepo_UpdateCurrentRoomID_SetNonNull:
// roomID 非 nil → UPDATE users SET current_room_id=? WHERE id=? 期望参数 (3001, 1001)
// → 返 RowsAffected=1。
func TestUserRepo_UpdateCurrentRoomID_SetNonNull(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserRepo(gormDB)

	roomID := uint64(3001)
	// GORM Updates(map) 会带 updated_at 自动列；用 sqlmock.AnyArg() 匹配
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `users` SET")).
		WithArgs(uint64(3001), sqlmock.AnyArg(), uint64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.UpdateCurrentRoomID(context.Background(), 1001, &roomID); err != nil {
		t.Fatalf("UpdateCurrentRoomID: %v", err)
	}
}

// TestUserRepo_UpdateCurrentRoomID_SetNull:
// roomID == nil → UPDATE users SET current_room_id=NULL WHERE id=?（GORM map 路径
// 强制 SET col=NULL；不能用 Update("current_room_id", v) 因为 v=nil 会被跳过）。
func TestUserRepo_UpdateCurrentRoomID_SetNull(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewUserRepo(gormDB)

	// nil 参数会被 GORM 编码为 SQL NULL；sqlmock 匹配 nil interface
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `users` SET")).
		WithArgs(nil, sqlmock.AnyArg(), uint64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.UpdateCurrentRoomID(context.Background(), 1001, nil); err != nil {
		t.Fatalf("UpdateCurrentRoomID nil: %v", err)
	}
}
