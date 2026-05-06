package mysql

import (
	"context"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
)

// TestRoomMemberRepo_RoomExists_True：room_members 表中存在该 room_id 任何行 → true
func TestRoomMemberRepo_RoomExists_True(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM room_members WHERE room_id = ? LIMIT 1")).
		WithArgs(uint64(3001)).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))

	got, err := repo.RoomExists(context.Background(), 3001)
	if err != nil {
		t.Fatalf("RoomExists: %v", err)
	}
	if !got {
		t.Errorf("got = false, want true")
	}
}

// TestRoomMemberRepo_RoomExists_FalseOnZeroRows：0 行 → false, nil
func TestRoomMemberRepo_RoomExists_FalseOnZeroRows(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM room_members WHERE room_id = ? LIMIT 1")).
		WithArgs(uint64(9999)).
		WillReturnRows(sqlmock.NewRows([]string{"1"})) // 0 行

	got, err := repo.RoomExists(context.Background(), 9999)
	if err != nil {
		t.Fatalf("RoomExists: %v", err)
	}
	if got {
		t.Errorf("got = true, want false (0 rows)")
	}
}

// TestRoomMemberRepo_IsUserInRoom_True：(user_id, room_id) 命中 → true
func TestRoomMemberRepo_IsUserInRoom_True(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM room_members WHERE user_id = ? AND room_id = ? LIMIT 1")).
		WithArgs(uint64(1001), uint64(3001)).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))

	got, err := repo.IsUserInRoom(context.Background(), 1001, 3001)
	if err != nil {
		t.Fatalf("IsUserInRoom: %v", err)
	}
	if !got {
		t.Errorf("got = false, want true")
	}
}

// TestRoomMemberRepo_IsUserInRoom_FalseOnZeroRows：用户不在房间 → false, nil
func TestRoomMemberRepo_IsUserInRoom_FalseOnZeroRows(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM room_members WHERE user_id = ? AND room_id = ? LIMIT 1")).
		WithArgs(uint64(9999), uint64(3001)).
		WillReturnRows(sqlmock.NewRows([]string{"1"})) // 0 行

	got, err := repo.IsUserInRoom(context.Background(), 9999, 3001)
	if err != nil {
		t.Fatalf("IsUserInRoom: %v", err)
	}
	if got {
		t.Errorf("got = true, want false (user not in room)")
	}
}

// TestRoomMemberRepo_ListMembers_HappyPath：返按 user_id ASC 排序的全部成员
func TestRoomMemberRepo_ListMembers_HappyPath(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	rows := sqlmock.NewRows([]string{"user_id"}).
		AddRow(uint64(1001)).
		AddRow(uint64(1002))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id FROM room_members WHERE room_id = ? ORDER BY user_id ASC")).
		WithArgs(uint64(3001)).
		WillReturnRows(rows)

	got, err := repo.ListMembers(context.Background(), 3001)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0] != 1001 || got[1] != 1002 {
		t.Errorf("got = %v, want [1001, 1002]", got)
	}
}

// TestRoomMemberRepo_ListMembers_EmptyRoom：0 行 → ([], nil)（不返 nil）
func TestRoomMemberRepo_ListMembers_EmptyRoom(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT user_id FROM room_members WHERE room_id = ? ORDER BY user_id ASC")).
		WithArgs(uint64(9999)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id"}))

	got, err := repo.ListMembers(context.Background(), 9999)
	if err != nil {
		t.Fatalf("ListMembers: %v", err)
	}
	if got == nil {
		t.Errorf("got = nil, want non-nil empty slice")
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}
