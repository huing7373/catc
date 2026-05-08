package mysql

import (
	"context"
	stderrors "errors"
	"regexp"
	"testing"

	sqlmock "github.com/DATA-DOG/go-sqlmock"
	driverMysql "github.com/go-sql-driver/mysql"
)

// TestRoomMemberRepo_RoomExists_TrueOnActive：rooms 表中存在该 id 且 status=1 → true
// （review r4 P2 修：查 rooms 不查 room_members；r7 P2 加 status 过滤）
func TestRoomMemberRepo_RoomExists_TrueOnActive(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM rooms WHERE id = ? AND status = 1 LIMIT 1")).
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

// TestRoomMemberRepo_RoomExists_FalseOnZeroRows：0 行（rooms 表不存在该 id）→ false, nil
// （review r4 P2 修：查 rooms 不查 room_members）
func TestRoomMemberRepo_RoomExists_FalseOnZeroRows(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM rooms WHERE id = ? AND status = 1 LIMIT 1")).
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

// TestRoomMemberRepo_RoomExists_RoomMissingButMembersExist：rooms 不存在但
// room_members 有 stale 残留 → false, nil（核心 fix verification：review r4 P2
// 指出旧实装会误判 true，导致 archived rooms 仍 accept WS 连接）。
func TestRoomMemberRepo_RoomExists_RoomMissingButMembersExist(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	// 模拟"rooms 表无该 id（room 已 archived 删除）"；即便 room_members
	// 表里仍有 (3001, 1001) 的 stale 行，RoomExists 也必须返 false。
	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM rooms WHERE id = ? AND status = 1 LIMIT 1")).
		WithArgs(uint64(3001)).
		WillReturnRows(sqlmock.NewRows([]string{"1"})) // rooms 0 行

	got, err := repo.RoomExists(context.Background(), 3001)
	if err != nil {
		t.Fatalf("RoomExists: %v", err)
	}
	if got {
		t.Errorf("got = true, want false (rooms missing even though room_members has stale rows)")
	}
}

// TestRoomMemberRepo_RoomExists_EmptyRoomStillExists：rooms 表存在 + room_members
// 空 → true（房间刚创建尚无成员的边界场景；review r4 P2 hint 列出的第三个用例）。
func TestRoomMemberRepo_RoomExists_EmptyRoomStillExists(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM rooms WHERE id = ? AND status = 1 LIMIT 1")).
		WithArgs(uint64(3002)).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))

	got, err := repo.RoomExists(context.Background(), 3002)
	if err != nil {
		t.Fatalf("RoomExists: %v", err)
	}
	if !got {
		t.Errorf("got = false, want true (empty room still exists in rooms table)")
	}
}

// TestRoomMemberRepo_RoomExists_FalseOnClosedRoom：rooms 表存在该 id 但
// status=2 (closed) → false, nil（review r7 P2 fix 核心：过滤 closed 房间，
// 防 archived rooms 仍被 Gateway accept 而错误下发 room.snapshot）。
//
// SQL 语义：raw query 中 `status = 1` 直接在 DB 层过滤，sqlmock 模拟"DB 返 0 行"
// 即可（不需要校验 status 列值，因为 query 已带 status=1 谓词，DB 不会返 closed
// 行）。这里 expect 的 args 仅 roomID（status=1 是 SQL 字面量谓词，**不**作为
// `?` 占位）。
func TestRoomMemberRepo_RoomExists_FalseOnClosedRoom(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	// 模拟"rooms.id=3001 行存在但 status=2"：query 带 `status = 1` 过滤 → DB 返 0 行
	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM rooms WHERE id = ? AND status = 1 LIMIT 1")).
		WithArgs(uint64(3001)).
		WillReturnRows(sqlmock.NewRows([]string{"1"})) // 0 行（被 status=1 过滤掉）

	got, err := repo.RoomExists(context.Background(), 3001)
	if err != nil {
		t.Fatalf("RoomExists: %v", err)
	}
	if got {
		t.Errorf("got = true, want false (closed room must be rejected even if rooms.id row exists)")
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

// ============================================================
// Story 11.3 新增：RoomMemberRepo.Create 路径覆盖
// ============================================================

// TestRoomMemberRepo_Create_AssignsAutoIncrementID:
// INSERT room_members → sqlmock 返 LastInsertId=5001 → 验证 m.ID 被 GORM 回填。
func TestRoomMemberRepo_Create_AssignsAutoIncrementID(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `room_members`")).
		WillReturnResult(sqlmock.NewResult(5001, 1))

	m := &RoomMember{
		RoomID: 3001,
		UserID: 1001,
	}
	if err := repo.Create(context.Background(), m); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m.ID != 5001 {
		t.Errorf("m.ID = %d, want 5001 (回填的 LastInsertId)", m.ID)
	}
}

// TestRoomMemberRepo_Create_UniqueUserIDDuplicate_ReturnsErrRoomMembersUserIDDuplicate:
// 模拟 ER_DUP_ENTRY 1062 + Message 含 'uk_user_id' → 翻译为 ErrRoomMembersUserIDDuplicate。
//
// 这是 Story 11.3 创建房间事务"用户已在房间中"语义的 race 兜底点：service 层用
// errors.Is 识别后翻译为 6003。
func TestRoomMemberRepo_Create_UniqueUserIDDuplicate_ReturnsErrRoomMembersUserIDDuplicate(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	dupErr := &driverMysql.MySQLError{
		Number:  1062,
		Message: "Duplicate entry '1001' for key 'room_members.uk_user_id'",
	}
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `room_members`")).
		WillReturnError(dupErr)

	err := repo.Create(context.Background(), &RoomMember{
		RoomID: 3001,
		UserID: 1001,
	})
	if !stderrors.Is(err, ErrRoomMembersUserIDDuplicate) {
		t.Errorf("err = %v, want ErrRoomMembersUserIDDuplicate (uk_user_id 1062 应被翻译)", err)
	}
}

// TestRoomMemberRepo_Create_UniqueRoomUserDuplicate_ReturnsErrRoomMembersRoomUserDuplicate:
// 模拟 ER_DUP_ENTRY 1062 + Message 含 'uk_room_user' → 翻译为 ErrRoomMembersRoomUserDuplicate。
//
// 与 uk_user_id 路径在 service 层语义等价（都翻译为 6003），但分两个独立哨兵让日志
// 能区分哪个约束被打破。
func TestRoomMemberRepo_Create_UniqueRoomUserDuplicate_ReturnsErrRoomMembersRoomUserDuplicate(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	dupErr := &driverMysql.MySQLError{
		Number:  1062,
		Message: "Duplicate entry '3001-1001' for key 'room_members.uk_room_user'",
	}
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `room_members`")).
		WillReturnError(dupErr)

	err := repo.Create(context.Background(), &RoomMember{
		RoomID: 3001,
		UserID: 1001,
	})
	if !stderrors.Is(err, ErrRoomMembersRoomUserDuplicate) {
		t.Errorf("err = %v, want ErrRoomMembersRoomUserDuplicate (uk_room_user 1062 应被翻译)", err)
	}
}

// TestRoomMemberRepo_Create_OtherDBError_Propagates:
// 非 1062 的 DB error → repo 透传 raw error（service 层包成 1009）。
func TestRoomMemberRepo_Create_OtherDBError_Propagates(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	wantErr := stderrors.New("synthetic db connection error")
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `room_members`")).
		WillReturnError(wantErr)

	err := repo.Create(context.Background(), &RoomMember{
		RoomID: 3001,
		UserID: 1001,
	})
	if !stderrors.Is(err, wantErr) {
		t.Errorf("err = %v, want wantErr (非 1062 DB error 应原样透传)", err)
	}
}

// ============================================================
// Story 11.4 新增：RoomMemberRepo.CountByRoomID 路径覆盖
// ============================================================

// TestRoomMemberRepo_CountByRoomID_Happy_Empty:
// SELECT COUNT(*) → 0 → repo 返 0, nil。
func TestRoomMemberRepo_CountByRoomID_Happy_Empty(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `room_members` WHERE room_id = ?")).
		WithArgs(uint64(3001)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))

	got, err := repo.CountByRoomID(context.Background(), 3001)
	if err != nil {
		t.Fatalf("CountByRoomID: %v", err)
	}
	if got != 0 {
		t.Errorf("got = %d, want 0", got)
	}
}

// TestRoomMemberRepo_CountByRoomID_Happy_Single:
// SELECT COUNT(*) → 1 → repo 返 1, nil（创建房间后初始状态：只有 creator）。
func TestRoomMemberRepo_CountByRoomID_Happy_Single(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `room_members` WHERE room_id = ?")).
		WithArgs(uint64(3001)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))

	got, err := repo.CountByRoomID(context.Background(), 3001)
	if err != nil {
		t.Fatalf("CountByRoomID: %v", err)
	}
	if got != 1 {
		t.Errorf("got = %d, want 1", got)
	}
}

// TestRoomMemberRepo_CountByRoomID_Happy_Full:
// SELECT COUNT(*) → 4 → repo 返 4, nil（房间已满；service 层会判定 >= 4 并返 6002）。
func TestRoomMemberRepo_CountByRoomID_Happy_Full(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `room_members` WHERE room_id = ?")).
		WithArgs(uint64(3001)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(4))

	got, err := repo.CountByRoomID(context.Background(), 3001)
	if err != nil {
		t.Fatalf("CountByRoomID: %v", err)
	}
	if got != 4 {
		t.Errorf("got = %d, want 4 (满员)", got)
	}
}

// TestRoomMemberRepo_CountByRoomID_DBError_Propagates:
// 任意 DB error → repo 透传 raw error（service 层包成 1009）。
func TestRoomMemberRepo_CountByRoomID_DBError_Propagates(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	wantErr := stderrors.New("synthetic db connection error")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `room_members` WHERE room_id = ?")).
		WithArgs(uint64(3001)).
		WillReturnError(wantErr)

	got, err := repo.CountByRoomID(context.Background(), 3001)
	if got != 0 {
		t.Errorf("got = %d, want 0 on error", got)
	}
	if !stderrors.Is(err, wantErr) {
		t.Errorf("err = %v, want wantErr", err)
	}
}

// ============================================================
// Story 11.5 新增：RoomMemberRepo.DeleteByRoomAndUser 路径覆盖
// ============================================================

// TestRoomMemberRepo_DeleteByRoomAndUser_Happy_RowsAffected1:
// DELETE FROM room_members WHERE room_id = ? AND user_id = ? → 1 行 → repo 返 (1, nil)。
func TestRoomMemberRepo_DeleteByRoomAndUser_Happy_RowsAffected1(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `room_members` WHERE room_id = ? AND user_id = ?")).
		WithArgs(uint64(3001), uint64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	got, err := repo.DeleteByRoomAndUser(context.Background(), 3001, 1001)
	if err != nil {
		t.Fatalf("DeleteByRoomAndUser: %v", err)
	}
	if got != 1 {
		t.Errorf("rowsAffected = %d, want 1", got)
	}
}

// TestRoomMemberRepo_DeleteByRoomAndUser_RowsAffected0_NoError:
// 行不存在（同 user 并发两次 leave 输家场景）→ repo 返 (0, nil)。**不**视为异常 ——
// rows-affected 0 是合法业务场景，由 service 层翻译为 6004。
func TestRoomMemberRepo_DeleteByRoomAndUser_RowsAffected0_NoError(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `room_members` WHERE room_id = ? AND user_id = ?")).
		WithArgs(uint64(3001), uint64(1001)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	got, err := repo.DeleteByRoomAndUser(context.Background(), 3001, 1001)
	if err != nil {
		t.Fatalf("DeleteByRoomAndUser unexpected err: %v", err)
	}
	if got != 0 {
		t.Errorf("rowsAffected = %d, want 0", got)
	}
}

// TestRoomMemberRepo_DeleteByRoomAndUser_DBError_Propagates:
// 任意 DB error → repo 返 (0, error) 透传给 service（service 包成 1009）。
func TestRoomMemberRepo_DeleteByRoomAndUser_DBError_Propagates(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	wantErr := stderrors.New("synthetic db connection error during DELETE")
	mock.ExpectExec(regexp.QuoteMeta("DELETE FROM `room_members` WHERE room_id = ? AND user_id = ?")).
		WithArgs(uint64(3001), uint64(1001)).
		WillReturnError(wantErr)

	got, err := repo.DeleteByRoomAndUser(context.Background(), 3001, 1001)
	if got != 0 {
		t.Errorf("rowsAffected = %d, want 0 on error", got)
	}
	if !stderrors.Is(err, wantErr) {
		t.Errorf("err = %v, want wantErr", err)
	}
}

// ============================================================
// Story 11.6 新增：RoomMemberRepo.ExistsForShareByRoomAndUser 路径覆盖（FOR SHARE 锁）
// ============================================================

// TestRoomMemberRepo_ExistsForShareByRoomAndUser_Happy_OneRow:
// FOR SHARE 命中 1 行 → (true, nil)。SQL 含 "FOR SHARE" 关键字（review 阶段必查）。
func TestRoomMemberRepo_ExistsForShareByRoomAndUser_Happy_OneRow(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM room_members WHERE room_id = ? AND user_id = ? LIMIT 1 FOR SHARE")).
		WithArgs(uint64(3001), uint64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{"1"}).AddRow(1))

	got, err := repo.ExistsForShareByRoomAndUser(context.Background(), 3001, 1001)
	if err != nil {
		t.Fatalf("ExistsForShareByRoomAndUser: %v", err)
	}
	if !got {
		t.Errorf("got = false, want true (1 行命中)")
	}
}

// TestRoomMemberRepo_ExistsForShareByRoomAndUser_ZeroRows:
// FOR SHARE 0 行（caller 已被并发 leave DELETE）→ (false, nil)，不视为异常。
func TestRoomMemberRepo_ExistsForShareByRoomAndUser_ZeroRows(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM room_members WHERE room_id = ? AND user_id = ? LIMIT 1 FOR SHARE")).
		WithArgs(uint64(3001), uint64(1001)).
		WillReturnRows(sqlmock.NewRows([]string{"1"})) // 0 行

	got, err := repo.ExistsForShareByRoomAndUser(context.Background(), 3001, 1001)
	if err != nil {
		t.Fatalf("ExistsForShareByRoomAndUser unexpected err: %v", err)
	}
	if got {
		t.Errorf("got = true, want false (0 行)")
	}
}

// TestRoomMemberRepo_ExistsForShareByRoomAndUser_DBError_Propagates:
// 任意 DB error → repo 返 (false, raw error) 透传给 service（service 包成 1009）。
func TestRoomMemberRepo_ExistsForShareByRoomAndUser_DBError_Propagates(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	wantErr := stderrors.New("synthetic db connection error during FOR SHARE")
	mock.ExpectQuery(regexp.QuoteMeta("SELECT 1 FROM room_members WHERE room_id = ? AND user_id = ? LIMIT 1 FOR SHARE")).
		WithArgs(uint64(3001), uint64(1001)).
		WillReturnError(wantErr)

	got, err := repo.ExistsForShareByRoomAndUser(context.Background(), 3001, 1001)
	if got {
		t.Errorf("got = true, want false on DB error")
	}
	if !stderrors.Is(err, wantErr) {
		t.Errorf("err = %v, want wantErr (raw DB error 应原样透传)", err)
	}
}

// ============================================================
// Story 11.6 新增：RoomMemberRepo.ListRosterByRoomID 路径覆盖（INNER JOIN users + LEFT JOIN pets）
// ============================================================

// TestRoomMemberRepo_ListRosterByRoomID_Happy_3Members_With1PetLess:
// 3 个 member（前 2 个有 pet，第 3 个 LEFT JOIN pets pet_id 列 NULL → pet-less）。
// SQL 必须包含 LEFT JOIN pets / INNER JOIN users / ORDER BY room_members.joined_at ASC。
func TestRoomMemberRepo_ListRosterByRoomID_Happy_3Members_With1PetLess(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	rows := sqlmock.NewRows([]string{"user_id", "nickname", "avatar_url", "pet_id"}).
		AddRow(uint64(1001), "用户A", "https://a", uint64(8001)).
		AddRow(uint64(1002), "用户B", "", uint64(8002)).
		AddRow(uint64(1003), "用户C", "https://c", nil) // pet-less → pet_id 列 NULL

	// SQL 关键字：LEFT JOIN pets + INNER JOIN users + ORDER BY joined_at ASC
	mock.ExpectQuery(`LEFT JOIN pets.*ORDER BY room_members\.joined_at ASC`).
		WithArgs(uint64(3001)).
		WillReturnRows(rows)

	got, err := repo.ListRosterByRoomID(context.Background(), 3001)
	if err != nil {
		t.Fatalf("ListRosterByRoomID: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	// member 1: 有 pet
	if got[0].UserID != 1001 || got[0].Nickname != "用户A" || got[0].AvatarURL != "https://a" {
		t.Errorf("got[0] = %+v, want {UserID:1001 Nickname:用户A AvatarURL:https://a}", got[0])
	}
	if got[0].PetID == nil || *got[0].PetID != 8001 {
		t.Errorf("got[0].PetID = %v, want &8001", got[0].PetID)
	}
	// member 2: avatarUrl 为空字符串（合法）
	if got[1].AvatarURL != "" {
		t.Errorf("got[1].AvatarURL = %q, want empty string", got[1].AvatarURL)
	}
	// member 3: **pet-less**（PetID 为 nil pointer）
	if got[2].PetID != nil {
		t.Errorf("got[2].PetID = %v (deref %d), want nil pointer (pet-less)", got[2].PetID, *got[2].PetID)
	}
}

// TestRoomMemberRepo_ListRosterByRoomID_ZeroRows_ReturnsEmptySlice:
// 0 行（房间存在但无成员，theoretical closed 路径）→ ([]RosterRow{}, nil)，不返 nil。
func TestRoomMemberRepo_ListRosterByRoomID_ZeroRows_ReturnsEmptySlice(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	mock.ExpectQuery(`LEFT JOIN pets.*ORDER BY room_members\.joined_at ASC`).
		WithArgs(uint64(9999)).
		WillReturnRows(sqlmock.NewRows([]string{"user_id", "nickname", "avatar_url", "pet_id"}))

	got, err := repo.ListRosterByRoomID(context.Background(), 9999)
	if err != nil {
		t.Fatalf("ListRosterByRoomID: %v", err)
	}
	if got == nil {
		t.Errorf("got = nil, want non-nil empty slice ([]RosterRow{})")
	}
	if len(got) != 0 {
		t.Errorf("len = %d, want 0", len(got))
	}
}

// TestRoomMemberRepo_ListRosterByRoomID_DBError_Propagates:
// 任意 DB error → (nil, raw error) 透传给 service（service 包成 1009）。
func TestRoomMemberRepo_ListRosterByRoomID_DBError_Propagates(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	wantErr := stderrors.New("synthetic db connection error during ListRoster")
	mock.ExpectQuery(`LEFT JOIN pets.*ORDER BY room_members\.joined_at ASC`).
		WithArgs(uint64(3001)).
		WillReturnError(wantErr)

	got, err := repo.ListRosterByRoomID(context.Background(), 3001)
	if got != nil {
		t.Errorf("got = %v, want nil on error", got)
	}
	if !stderrors.Is(err, wantErr) {
		t.Errorf("err = %v, want wantErr (raw DB error 应原样透传)", err)
	}
}

// TestRoomMemberRepo_ListRosterByRoomID_SingleMember_StableOrder:
// 单 member 顺序验证（确保 ORDER BY 子句生效；与 sqlmock 期望的 SQL 含
// `ORDER BY room_members.joined_at ASC` 子句对齐）。
func TestRoomMemberRepo_ListRosterByRoomID_SingleMember_StableOrder(t *testing.T) {
	gormDB, mock := newGormWithMock(t)
	repo := NewRoomMemberRepo(gormDB)

	rows := sqlmock.NewRows([]string{"user_id", "nickname", "avatar_url", "pet_id"}).
		AddRow(uint64(1001), "Solo", "", uint64(8001))

	// SQL 必含 INNER JOIN users 子句（review 阶段必查 vs LEFT JOIN users 反模式）
	mock.ExpectQuery(`INNER JOIN users.*LEFT JOIN pets.*ORDER BY room_members\.joined_at ASC`).
		WithArgs(uint64(3001)).
		WillReturnRows(rows)

	got, err := repo.ListRosterByRoomID(context.Background(), 3001)
	if err != nil {
		t.Fatalf("ListRosterByRoomID: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].UserID != 1001 || got[0].Nickname != "Solo" {
		t.Errorf("got[0] = %+v, want UserID=1001 Nickname=Solo", got[0])
	}
}
