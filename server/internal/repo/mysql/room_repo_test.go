package mysql

import (
	"context"
	stderrors "errors"
	"regexp"
	"testing"
	"time"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// TestRoomRepo_Create_AssignsAutoIncrementID:
// INSERT rooms → sqlmock 返 LastInsertId=3001 → 验证 r.ID 被 GORM 回填。
func TestRoomRepo_Create_AssignsAutoIncrementID(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `rooms`")).
		WillReturnResult(sqlmock.NewResult(3001, 1))

	r := &Room{
		CreatorUserID: 1001,
		Status:        1,
		MaxMembers:    4,
	}
	if err := repo.Create(context.Background(), r); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if r.ID != 3001 {
		t.Errorf("r.ID = %d, want 3001 (回填的 LastInsertId)", r.ID)
	}
}

// TestRoomRepo_Create_DBError_Propagates:
// INSERT rooms 触发任意非 1062 DB error → repo 透传 raw error。
//
// rooms 表只有 PRIMARY KEY 唯一约束（AUTO_INCREMENT 不会冲突），无业务唯一约束 →
// 1062 路径理论不可达；本测试覆盖一般 DB error 透传路径（service 包成 1009）。
func TestRoomRepo_Create_DBError_Propagates(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomRepo(gormDB)

	wantErr := stderrors.New("synthetic db connection error")
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `rooms`")).
		WillReturnError(wantErr)

	err := repo.Create(context.Background(), &Room{
		CreatorUserID: 1001,
		Status:        1,
		MaxMembers:    4,
	})
	if !stderrors.Is(err, wantErr) {
		t.Errorf("err = %v, want wantErr (raw DB error 应原样透传给 service)", err)
	}
}

// ============================================================
// Story 11.4 新增：RoomRepo.FindByIDForUpdate 路径覆盖
// ============================================================

// TestRoomRepo_FindByIDForUpdate_Happy:
// SELECT ... FOR UPDATE LIMIT 1 → sqlmock 返 1 行 → repo 返 *Room + nil。
// **关键**：query 必须含 "FOR UPDATE" 关键字（GORM clause.Locking 生成）。
func TestRoomRepo_FindByIDForUpdate_Happy(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomRepo(gormDB)

	now := time.Now()
	rows := sqlmock.NewRows([]string{"id", "creator_user_id", "status", "max_members", "created_at", "updated_at"}).
		AddRow(uint64(3001), uint64(1001), int8(1), uint8(4), now, now)
	// GORM clause.Locking{Strength: "UPDATE"} 把 SQL 编译成 `... FOR UPDATE`；
	// sqlmock 用 regex 匹配，关键是末尾包含 FOR UPDATE 子句。
	mock.ExpectQuery(`SELECT \* FROM .rooms. WHERE id = \? ORDER BY .rooms.\..id. LIMIT \? FOR UPDATE`).
		WithArgs(uint64(3001), 1).
		WillReturnRows(rows)

	room, err := repo.FindByIDForUpdate(context.Background(), 3001)
	if err != nil {
		t.Fatalf("FindByIDForUpdate: %v", err)
	}
	if room == nil {
		t.Fatalf("room is nil, want non-nil")
	}
	if room.ID != 3001 {
		t.Errorf("room.ID = %d, want 3001", room.ID)
	}
	if room.CreatorUserID != 1001 {
		t.Errorf("room.CreatorUserID = %d, want 1001", room.CreatorUserID)
	}
	if room.Status != 1 {
		t.Errorf("room.Status = %d, want 1", room.Status)
	}
	if room.MaxMembers != 4 {
		t.Errorf("room.MaxMembers = %d, want 4", room.MaxMembers)
	}
}

// TestRoomRepo_FindByIDForUpdate_NotFound:
// 0 行（GORM 翻译为 ErrRecordNotFound）→ repo 返 nil, ErrRoomNotFound 哨兵。
//
// service 层用 errors.Is(err, mysql.ErrRoomNotFound) 识别后翻译为 6001。
func TestRoomRepo_FindByIDForUpdate_NotFound(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomRepo(gormDB)

	// 返 0 行 → GORM First() 返 ErrRecordNotFound
	mock.ExpectQuery(`SELECT \* FROM .rooms. WHERE id = \? ORDER BY .rooms.\..id. LIMIT \? FOR UPDATE`).
		WithArgs(uint64(9999), 1).
		WillReturnRows(sqlmock.NewRows([]string{"id", "creator_user_id", "status", "max_members", "created_at", "updated_at"}))

	room, err := repo.FindByIDForUpdate(context.Background(), 9999)
	if room != nil {
		t.Errorf("room = %+v, want nil on NotFound", room)
	}
	if !stderrors.Is(err, ErrRoomNotFound) {
		t.Errorf("err = %v, want ErrRoomNotFound 哨兵", err)
	}
}

// TestRoomRepo_FindByIDForUpdate_DBError_Propagates:
// 任意非 ErrRecordNotFound 的 raw DB error → repo 透传给 service（service 包 1009）。
func TestRoomRepo_FindByIDForUpdate_DBError_Propagates(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomRepo(gormDB)

	wantErr := stderrors.New("synthetic db connection error")
	mock.ExpectQuery(`SELECT \* FROM .rooms. WHERE id = \? ORDER BY .rooms.\..id. LIMIT \? FOR UPDATE`).
		WithArgs(uint64(3001), 1).
		WillReturnError(wantErr)

	room, err := repo.FindByIDForUpdate(context.Background(), 3001)
	if room != nil {
		t.Errorf("room = %+v, want nil on DB error", room)
	}
	if !stderrors.Is(err, wantErr) {
		t.Errorf("err = %v, want wantErr (raw DB error 应原样透传)", err)
	}
}
