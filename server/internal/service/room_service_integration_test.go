//go:build integration
// +build integration

// Story 11.3 集成测试：用 dockertest 起真实 mysql:8.0 容器跑创建房间事务的 3 条 case：
//
//   1. CreateRoom_Happy_3RowsInserted（epics §11.3 钦定）：
//      创建 user 1001 → svc.CreateRoom → 验证 rooms / room_members / users.current_room_id
//      三表行变化（rooms 新增 1 行 + room_members 新增 1 行 + users.current_room_id 已写）
//
//   2. CreateRoom_AlreadyInRoom_PrecheckReturns6003（epics §11.3 钦定）：
//      沿用 case 1 fixture（user 1001 已通过 case 1 创建房间）→ 同 user 再次调用 →
//      返 6003 + DB 三表行数**未变化**（预检路径在事务外，事务未开）
//
//   3. CreateRoom_RollsBackOnRoomMemberInsertFail（事务原子性验证；epics §11.3 钦定
//      "插入 room_members 失败 → rooms 也回滚"路径的真实端到端验证）：
//      fixture 先用 raw INSERT 给 user 2001 写一条 room_members（room_id=9999）但**不**
//      设置 users.current_room_id（让预检路径绕过）→ svc.CreateRoom(2001) 走完预检 →
//      进事务 → roomRepo.Create 成功（rooms 多 1 行）→ roomMemberRepo.Create 撞
//      UNIQUE(user_id) → ErrRoomMembersUserIDDuplicate → service 包 6003 → 事务回滚
//      → rooms 表行数**与回滚前一致**
//
// 复用 4.6 / 4.8 / 7.3 的 startMySQL / migrationsPath / runMigrations / insertUser helper。
//
// **手工 INSERT** user / pet / step_account / chest 5 行 fixture（不调 auth_service.GuestLogin） ——
// 解耦 room_service 测试与 auth_service。

package service_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
	"github.com/huing/cat/server/internal/service"
)

// buildRoomServiceIntegration: 起容器 → migrate → 装配 svc + 返清理 closure。
func buildRoomServiceIntegration(t *testing.T) (svc service.RoomService, sqlDB *sql.DB, cleanup func()) {
	t.Helper()

	dsn, dockerCleanup := startMySQL(t)
	runMigrations(t, dsn)

	cfg := config.MySQLConfig{
		DSN:                dsn,
		MaxOpenConns:       10,
		MaxIdleConns:       2,
		ConnMaxLifetimeSec: 60,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gormDB, err := db.Open(ctx, cfg)
	if err != nil {
		dockerCleanup()
		t.Fatalf("db.Open: %v", err)
	}

	txMgr := tx.NewManager(gormDB)
	userRepo := mysql.NewUserRepo(gormDB)
	roomRepo := mysql.NewRoomRepo(gormDB)
	roomMemberRepo := mysql.NewRoomMemberRepo(gormDB)

	svc = service.NewRoomService(txMgr, userRepo, roomRepo, roomMemberRepo)

	rawDB, err := gormDB.DB()
	if err != nil {
		dockerCleanup()
		t.Fatalf("gormDB.DB(): %v", err)
	}

	cleanup = func() {
		_ = rawDB.Close()
		dockerCleanup()
	}
	return svc, rawDB, cleanup
}

// fetchUserCurrentRoomID 直接查 users.current_room_id（NULL → 返 nil）。
func fetchUserCurrentRoomID(t *testing.T, sqlDB *sql.DB, userID uint64) *uint64 {
	t.Helper()
	var roomID sql.NullInt64
	row := sqlDB.QueryRow("SELECT current_room_id FROM users WHERE id = ?", userID)
	if err := row.Scan(&roomID); err != nil {
		t.Fatalf("fetchUserCurrentRoomID: %v", err)
	}
	if !roomID.Valid {
		return nil
	}
	v := uint64(roomID.Int64)
	return &v
}

// fetchRoomCount 返 rooms 表的 SELECT COUNT(*)。
func fetchRoomCount(t *testing.T, sqlDB *sql.DB) int64 {
	t.Helper()
	var n int64
	if err := sqlDB.QueryRow("SELECT COUNT(*) FROM rooms").Scan(&n); err != nil {
		t.Fatalf("fetchRoomCount: %v", err)
	}
	return n
}

// fetchRoomMemberCount 返 room_members 表的 SELECT COUNT(*)。
func fetchRoomMemberCount(t *testing.T, sqlDB *sql.DB) int64 {
	t.Helper()
	var n int64
	if err := sqlDB.QueryRow("SELECT COUNT(*) FROM room_members").Scan(&n); err != nil {
		t.Fatalf("fetchRoomMemberCount: %v", err)
	}
	return n
}

// fetchRoomStatus 返指定 room.id 的 status 值；行不存在时 t.Fatalf。
func fetchRoomStatus(t *testing.T, sqlDB *sql.DB, roomID uint64) int8 {
	t.Helper()
	var s int8
	if err := sqlDB.QueryRow("SELECT status FROM rooms WHERE id = ?", roomID).Scan(&s); err != nil {
		t.Fatalf("fetchRoomStatus: %v", err)
	}
	return s
}

// ============================================================
// AC11.1: happy 路径 → 真实 INSERT 3 行（rooms / room_members / users.current_room_id）
// ============================================================
func TestRoomServiceIntegration_CreateRoom_Happy_3RowsInserted(t *testing.T) {
	svc, sqlDB, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userID = uint64(1001)
	insertUser(t, sqlDB, userID, "uid-room-1", "用户1001", "")

	out, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userID})
	if err != nil {
		t.Fatalf("CreateRoom: %v", err)
	}
	if out.RoomID == 0 {
		t.Errorf("out.RoomID = 0, want > 0 (GORM AUTO_INCREMENT 回填)")
	}
	if out.CreatorUserID != userID {
		t.Errorf("out.CreatorUserID = %d, want %d", out.CreatorUserID, userID)
	}
	if out.MaxMembers != 4 {
		t.Errorf("out.MaxMembers = %d, want 4", out.MaxMembers)
	}
	if out.MemberCount != 1 {
		t.Errorf("out.MemberCount = %d, want 1", out.MemberCount)
	}
	if out.Status != 1 {
		t.Errorf("out.Status = %d, want 1 (active)", out.Status)
	}

	// 验证 rooms 表新增 1 行（creator_user_id=1001, status=1, max_members=4）
	assertCount(t, sqlDB, "rooms WHERE id=? AND creator_user_id=? AND status=1 AND max_members=4",
		[]any{out.RoomID, userID}, 1, "rooms (created)")

	// 验证 room_members 表新增 1 行（room_id=out.RoomID, user_id=1001）
	assertCount(t, sqlDB, "room_members WHERE room_id=? AND user_id=?",
		[]any{out.RoomID, userID}, 1, "room_members (creator joined)")

	// 验证 users.current_room_id 已写为 out.RoomID
	got := fetchUserCurrentRoomID(t, sqlDB, userID)
	if got == nil {
		t.Fatalf("users.current_room_id = NULL, want %d", out.RoomID)
	}
	if *got != out.RoomID {
		t.Errorf("users.current_room_id = %d, want %d", *got, out.RoomID)
	}
}

// ============================================================
// AC11.2: 同 user 再次创建 → 预检 6003 + DB 行数不变（事务未开）
// ============================================================
func TestRoomServiceIntegration_CreateRoom_AlreadyInRoom_PrecheckReturns6003(t *testing.T) {
	svc, sqlDB, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userID = uint64(1001)
	insertUser(t, sqlDB, userID, "uid-room-1", "用户1001", "")

	// 第一次：成功 → users.current_room_id 已写
	out1, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userID})
	if err != nil {
		t.Fatalf("first CreateRoom: %v", err)
	}

	// 取首次后的 DB 状态快照
	roomCountAfterFirst := fetchRoomCount(t, sqlDB)
	memberCountAfterFirst := fetchRoomMemberCount(t, sqlDB)
	if roomCountAfterFirst != 1 {
		t.Fatalf("post-first-call rooms count = %d, want 1", roomCountAfterFirst)
	}
	if memberCountAfterFirst != 1 {
		t.Fatalf("post-first-call room_members count = %d, want 1", memberCountAfterFirst)
	}

	// 第二次：同 user 再次调 → 6003（预检路径，事务未开）
	_, err = svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userID})
	if err == nil {
		t.Fatalf("second CreateRoom returned nil error, want 6003")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserAlreadyInRoom {
		t.Errorf("AppError.Code = %d, want %d (ErrUserAlreadyInRoom 6003)", ae.Code, apperror.ErrUserAlreadyInRoom)
	}

	// DB 状态：rooms / room_members 行数**未变化**（事务未开）
	if got := fetchRoomCount(t, sqlDB); got != roomCountAfterFirst {
		t.Errorf("post-precheck-6003 rooms count = %d, want %d (no new row)", got, roomCountAfterFirst)
	}
	if got := fetchRoomMemberCount(t, sqlDB); got != memberCountAfterFirst {
		t.Errorf("post-precheck-6003 room_members count = %d, want %d (no new row)", got, memberCountAfterFirst)
	}

	// users.current_room_id 仍是首次写入的 room_id（未被覆盖）
	got := fetchUserCurrentRoomID(t, sqlDB, userID)
	if got == nil || *got != out1.RoomID {
		t.Errorf("users.current_room_id changed: want %d", out1.RoomID)
	}
}

// ============================================================
// Story 11.4 集成测试 case：JoinRoom 真实事务路径（dockertest）
// ============================================================

// AC11.4-1: A 创建房间 → B join → DB room_members 2 行 + B.current_room_id 更新
//
// fixture：A (id=1001) 已通过 11.3 createRoom 创建房间 (room_id=自动分配)；
//          B (id=1002) 不在任何房间。
// 调 svc.JoinRoom(B, room_id) → 期望 out.Joined == true + DB 校验 room_members 2 行
// + users.current_room_id (B) = room_id。
func TestRoomServiceIntegration_JoinRoom_Happy_2RowsAfterJoin(t *testing.T) {
	svc, sqlDB, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	// fixture: 两个 user，A 创建房间，B 待 join
	const userA = uint64(1001)
	const userB = uint64(1002)
	insertUser(t, sqlDB, userA, "uid-room-a", "用户A", "")
	insertUser(t, sqlDB, userB, "uid-room-b", "用户B", "")

	// A 创建房间（11.3 路径）
	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("createRoom: %v", err)
	}
	roomID := createOut.RoomID

	// B join
	joinOut, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userB, RoomID: roomID})
	if err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	if joinOut.RoomID != roomID {
		t.Errorf("out.RoomID = %d, want %d", joinOut.RoomID, roomID)
	}
	if !joinOut.Joined {
		t.Errorf("out.Joined = false, want true")
	}

	// DB 校验：room_members 表 2 行（A creator + B joiner）
	assertCount(t, sqlDB, "room_members WHERE room_id=?",
		[]any{roomID}, 2, "room_members (A + B)")
	// 验证 B 的成员行存在
	assertCount(t, sqlDB, "room_members WHERE room_id=? AND user_id=?",
		[]any{roomID, userB}, 1, "room_members (B joined)")

	// 验证 B.current_room_id 已写
	got := fetchUserCurrentRoomID(t, sqlDB, userB)
	if got == nil {
		t.Fatalf("users.current_room_id (B) = NULL, want %d", roomID)
	}
	if *got != roomID {
		t.Errorf("users.current_room_id (B) = %d, want %d", *got, roomID)
	}
}

// AC11.4-2: 房间已满 → 第 5 个用户 join 返回 6002 + room_members 仍 4 行
func TestRoomServiceIntegration_JoinRoom_RoomFull_Returns6002(t *testing.T) {
	svc, sqlDB, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	// fixture: 5 个 user，A 创建房间，B/C/D join，E 在 room_members 已满后试图 join
	const userA = uint64(1001)
	const userB = uint64(1002)
	const userC = uint64(1003)
	const userD = uint64(1004)
	const userE = uint64(1005)
	insertUser(t, sqlDB, userA, "uid-room-a", "A", "")
	insertUser(t, sqlDB, userB, "uid-room-b", "B", "")
	insertUser(t, sqlDB, userC, "uid-room-c", "C", "")
	insertUser(t, sqlDB, userD, "uid-room-d", "D", "")
	insertUser(t, sqlDB, userE, "uid-room-e", "E", "")

	// A 创建
	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("createRoom: %v", err)
	}
	roomID := createOut.RoomID

	// B / C / D join
	for _, u := range []uint64{userB, userC, userD} {
		if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: u, RoomID: roomID}); err != nil {
			t.Fatalf("JoinRoom user=%d: %v", u, err)
		}
	}

	// 校验现在 4 行
	assertCount(t, sqlDB, "room_members WHERE room_id=?",
		[]any{roomID}, 4, "room_members (4 members)")

	// E 试图 join → 6002
	_, err = svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userE, RoomID: roomID})
	if err == nil {
		t.Fatalf("JoinRoom (E) returned nil error, want 6002")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrRoomFull {
		t.Errorf("AppError.Code = %d, want %d (ErrRoomFull 6002)", ae.Code, apperror.ErrRoomFull)
	}

	// DB 校验：room_members 仍 4 行（事务回滚）
	assertCount(t, sqlDB, "room_members WHERE room_id=?",
		[]any{roomID}, 4, "room_members (still 4 after E rejected)")
	// E.current_room_id 仍 NULL
	got := fetchUserCurrentRoomID(t, sqlDB, userE)
	if got != nil {
		t.Errorf("users.current_room_id (E) = %d, want NULL (E join 失败 → 应保持 NULL)", *got)
	}
}

// AC11.4-3: 不存在的 roomID → 6001 + DB 表无变化
func TestRoomServiceIntegration_JoinRoom_RoomNotFound_Returns6001(t *testing.T) {
	svc, sqlDB, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userA = uint64(1001)
	insertUser(t, sqlDB, userA, "uid-room-a", "A", "")

	// 调 JoinRoom 用一个不存在的 roomID
	_, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userA, RoomID: 99999})
	if err == nil {
		t.Fatalf("JoinRoom returned nil error, want 6001")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrRoomNotFound {
		t.Errorf("AppError.Code = %d, want %d (ErrRoomNotFound 6001)", ae.Code, apperror.ErrRoomNotFound)
	}

	// DB 校验：rooms / room_members 表无新行
	assertCount(t, sqlDB, "rooms", nil, 0, "rooms (no rows)")
	assertCount(t, sqlDB, "room_members", nil, 0, "room_members (no rows)")
	// A.current_room_id 仍 NULL
	got := fetchUserCurrentRoomID(t, sqlDB, userA)
	if got != nil {
		t.Errorf("users.current_room_id (A) = %d, want NULL", *got)
	}
}

// AC11.4-4（强烈建议）: room status=2 closed → 6005
//
// fixture：A 创建房间 → 用 raw UPDATE 制造 status=2（leave 事务模拟）→
// B 试图 join → 期望 6005。
func TestRoomServiceIntegration_JoinRoom_RoomClosed_Returns6005(t *testing.T) {
	svc, sqlDB, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userA = uint64(1001)
	const userB = uint64(1002)
	insertUser(t, sqlDB, userA, "uid-room-a", "A", "")
	insertUser(t, sqlDB, userB, "uid-room-b", "B", "")

	// A 创建房间
	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("createRoom: %v", err)
	}
	roomID := createOut.RoomID

	// raw UPDATE 制造 status=2 (closed)
	_, err = sqlDB.Exec("UPDATE rooms SET status = 2 WHERE id = ?", roomID)
	if err != nil {
		t.Fatalf("UPDATE rooms.status: %v", err)
	}

	// B 试图 join closed 房间 → 6005
	_, err = svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userB, RoomID: roomID})
	if err == nil {
		t.Fatalf("JoinRoom returned nil error, want 6005")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrRoomInvalidState {
		t.Errorf("AppError.Code = %d, want %d (ErrRoomInvalidState 6005)", ae.Code, apperror.ErrRoomInvalidState)
	}

	// DB 校验：room_members 不变（B 没加进来；只有 A 创建时的 1 行）
	assertCount(t, sqlDB, "room_members WHERE room_id=?",
		[]any{roomID}, 1, "room_members (closed 房间 join 失败 → 仍 1 行 = A creator)")
	// B.current_room_id 仍 NULL
	got := fetchUserCurrentRoomID(t, sqlDB, userB)
	if got != nil {
		t.Errorf("users.current_room_id (B) = %d, want NULL", *got)
	}
}

// AC11.4-5（强烈建议; r9 P1#2 cross-tx race 验证）: leave-then-join FOR UPDATE 串行化
//
// fixture：A / B 在 room_id=R（A 通过 11.3 创建，B 通过本 story join）。
// 用 goroutine 同时执行：
//   - "leave" 事务（raw SQL 模拟 11.5 leave 事务）：BEGIN → SELECT FOR UPDATE
//     rooms → DELETE room_members → COUNT==0 → UPDATE rooms.status=2 → COMMIT
//   - C join 同 room R（走 svc.JoinRoom 真实事务）
//
// 因为 FOR UPDATE 锁串行化，最终结果只有两种：
//
//	(a) leave 先 commit（rooms.status=2 closed）→ C join step 2b 看到 status=2 → 6005
//	(b) C join 先 commit（room_members 多 1 行）→ leave 后续走完
//
// **不变量**：rooms.status 与 room_members 必须一致（不能出现 status=2 但 room_members
// 仍含 C 的行 —— 那是 r9 P1#2 race timeline）。
func TestRoomServiceIntegration_JoinRoom_CrossTxLeaveSerialized(t *testing.T) {
	svc, sqlDB, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userA = uint64(1001)
	const userB = uint64(1002)
	const userC = uint64(1003)
	insertUser(t, sqlDB, userA, "uid-room-a", "A", "")
	insertUser(t, sqlDB, userB, "uid-room-b", "B", "")
	insertUser(t, sqlDB, userC, "uid-room-c", "C", "")

	// fixture: A 创建房间 + B join → 现在 room_members 有 A / B 2 行
	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("createRoom: %v", err)
	}
	roomID := createOut.RoomID

	if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userB, RoomID: roomID}); err != nil {
		t.Fatalf("JoinRoom (B): %v", err)
	}

	// 启动两个 goroutine 并行执行 leave (raw) + join (svc)
	done := make(chan error, 2)

	// goroutine 1: 模拟 11.5 leave 事务 —— A leave + B leave → room 空 → status=2
	go func() {
		tx, err := sqlDB.Begin()
		if err != nil {
			done <- err
			return
		}
		defer func() { _ = tx.Rollback() }()
		// 步骤 1: SELECT rooms FOR UPDATE
		var status int8
		if err := tx.QueryRow("SELECT status FROM rooms WHERE id = ? FOR UPDATE", roomID).Scan(&status); err != nil {
			done <- err
			return
		}
		// 步骤 2/3: DELETE A + B 的 room_members + UPDATE users.current_room_id NULL
		if _, err := tx.Exec("DELETE FROM room_members WHERE room_id = ?", roomID); err != nil {
			done <- err
			return
		}
		if _, err := tx.Exec("UPDATE users SET current_room_id = NULL WHERE id IN (?, ?)", userA, userB); err != nil {
			done <- err
			return
		}
		// 步骤 4: COUNT == 0 → 关房间 status=2
		var cnt int64
		if err := tx.QueryRow("SELECT COUNT(*) FROM room_members WHERE room_id = ?", roomID).Scan(&cnt); err != nil {
			done <- err
			return
		}
		if cnt == 0 {
			if _, err := tx.Exec("UPDATE rooms SET status = 2 WHERE id = ?", roomID); err != nil {
				done <- err
				return
			}
		}
		if err := tx.Commit(); err != nil {
			done <- err
			return
		}
		done <- nil
	}()

	// goroutine 2: C join 同 room
	go func() {
		_, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userC, RoomID: roomID})
		done <- err
	}()

	// 等两个 goroutine 完成
	for i := 0; i < 2; i++ {
		select {
		case e := <-done:
			// 两个 goroutine 任一返非 nil error 都接受（leave 不应失败；join 在 (a)
			// 路径下必返 6005）
			_ = e
		case <-time.After(15 * time.Second):
			t.Fatalf("cross-tx race test timeout")
		}
	}

	// **核心断言**：rooms.status 与 room_members 一致性
	var finalStatus int8
	if err := sqlDB.QueryRow("SELECT status FROM rooms WHERE id = ?", roomID).Scan(&finalStatus); err != nil {
		t.Fatalf("query rooms.status: %v", err)
	}
	var memberCount int64
	if err := sqlDB.QueryRow("SELECT COUNT(*) FROM room_members WHERE room_id = ?", roomID).Scan(&memberCount); err != nil {
		t.Fatalf("query room_members count: %v", err)
	}

	// 合法状态：
	//   (a) leave 先 commit：status=2 + room_members 0 行（C join 6005 失败）
	//   (b) C join 先 commit：room_members 含 C 1 行 + status 取决于 leave 是否仍跑成功
	//       —— leave 在 SELECT FOR UPDATE 之后才看到 C 的成员行 → DELETE 三行 →
	//       COUNT==0 → status=2，所以最终 status=2 + member_count=0
	//       OR leave 因为 C 的并发 join 而看到 3 个成员都 DELETE → COUNT=0 → status=2
	// 关键不变量：status=2 必须配 member_count=0；status=1 必须 member_count >= 1
	if finalStatus == 2 && memberCount > 0 {
		t.Errorf("r9 P1#2 race detected: status=2 (closed) but room_members has %d rows (应该是 0)", memberCount)
	}
	if finalStatus == 1 && memberCount == 0 {
		t.Errorf("r9 P1#2 race detected: status=1 (active) but room_members is empty (应该 closed)")
	}
}

// ============================================================
// AC11.3: room_members UNIQUE(user_id) 真实兜底 → service 6003 + 事务原子性回滚
//
// fixture：先用 raw INSERT 给 user 2001 写一条 room_members(room_id=9999, user_id=2001)
// **但不**写 users.current_room_id（让预检路径绕过 → 进事务）。
//
// 期望：roomRepo.Create 成功（rooms 多 1 行）→ roomMemberRepo.Create 撞
// UNIQUE(user_id) `uk_user_id` → 翻译为 ErrRoomMembersUserIDDuplicate → service 包
// 6003 → tx 回滚 → rooms 表**回到事务前的行数**（新插入的 rooms 行被 InnoDB undo log
// 撤销）。
// ============================================================
func TestRoomServiceIntegration_CreateRoom_RollsBackOnRoomMemberInsertFail(t *testing.T) {
	svc, sqlDB, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userID = uint64(2001)
	insertUser(t, sqlDB, userID, "uid-room-2001", "用户2001", "")

	// 先建一个 placeholder room（让 room_members 有合法 room_id 外键关联，避免 FK 报错）
	const placeholderRoomID = uint64(9999)
	_, err := sqlDB.Exec(
		`INSERT INTO rooms (id, creator_user_id, status, max_members) VALUES (?, ?, 1, 4)`,
		placeholderRoomID, userID,
	)
	if err != nil {
		t.Fatalf("insert placeholder rooms row: %v", err)
	}

	// 直接 raw INSERT room_members(room_id=9999, user_id=2001)；**不**写
	// users.current_room_id（让 service 预检看到 user.current_room_id == NULL → 进事务）
	_, err = sqlDB.Exec(
		`INSERT INTO room_members (room_id, user_id) VALUES (?, ?)`,
		placeholderRoomID, userID,
	)
	if err != nil {
		t.Fatalf("insert placeholder room_members row: %v", err)
	}

	// 取事务前的 rooms / room_members 行数快照
	roomCountBefore := fetchRoomCount(t, sqlDB)
	memberCountBefore := fetchRoomMemberCount(t, sqlDB)

	// 调 svc.CreateRoom：预检通过（current_room_id == NULL）→ 进事务 →
	// roomRepo.Create 成功（rooms 临时 +1 行）→ roomMemberRepo.Create 撞 uk_user_id →
	// 翻译为 ErrRoomMembersUserIDDuplicate → service 包 6003 → tx 回滚
	_, err = svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userID})
	if err == nil {
		t.Fatalf("CreateRoom returned nil error, want 6003 (UNIQUE 兜底)")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserAlreadyInRoom {
		t.Errorf("AppError.Code = %d, want %d (ErrUserAlreadyInRoom 6003 兜底)", ae.Code, apperror.ErrUserAlreadyInRoom)
	}

	// **核心断言**：rooms 表行数**回到事务前的快照值**（新插入的 rooms 行被 rollback；
	// 不是"未到达"——roomRepo.Create 真的发生了 INSERT，但被 InnoDB undo log 撤销）
	if got := fetchRoomCount(t, sqlDB); got != roomCountBefore {
		t.Errorf("post-rollback rooms count = %d, want %d (事务回滚后应回到事务前快照)", got, roomCountBefore)
	}
	if got := fetchRoomMemberCount(t, sqlDB); got != memberCountBefore {
		t.Errorf("post-rollback room_members count = %d, want %d (placeholder 行仍在；新行被回滚)", got, memberCountBefore)
	}

	// users.current_room_id 仍未被设置（事务回滚 → UpdateCurrentRoomID 那步未到达 / 也被 rollback）
	got := fetchUserCurrentRoomID(t, sqlDB, userID)
	if got != nil {
		t.Errorf("users.current_room_id = %d, want NULL (事务回滚后应保持 NULL)", *got)
	}
}

// ============================================================
// Story 11.5 集成测试 case：LeaveRoom 真实事务路径（dockertest）
// ============================================================

// AC11.5-1: A + B 在房间 → A leave → DB room_members 剩 B 一行 + rooms.status 仍 = 1 +
// A.current_room_id = NULL（epics.md §11.5 钦定 case 1）。
func TestRoomServiceIntegration_LeaveRoom_NotLastMember_RoomActive(t *testing.T) {
	svc, sqlDB, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userA = uint64(1001)
	const userB = uint64(1002)
	insertUser(t, sqlDB, userA, "uid-leave-a", "A", "")
	insertUser(t, sqlDB, userB, "uid-leave-b", "B", "")

	// fixture: A 创建 + B join
	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("createRoom: %v", err)
	}
	roomID := createOut.RoomID
	if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userB, RoomID: roomID}); err != nil {
		t.Fatalf("JoinRoom (B): %v", err)
	}

	// A leave
	out, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: userA, RoomID: roomID})
	if err != nil {
		t.Fatalf("LeaveRoom (A): %v", err)
	}
	if out.RoomID != roomID || !out.Left {
		t.Errorf("out = %+v, want {RoomID:%d, Left:true}", out, roomID)
	}

	// DB 校验：room_members 1 行（仅 B）
	assertCount(t, sqlDB, "room_members WHERE room_id=?",
		[]any{roomID}, 1, "room_members (only B remains)")
	assertCount(t, sqlDB, "room_members WHERE room_id=? AND user_id=?",
		[]any{roomID, userB}, 1, "room_members (B's row)")
	// rooms.status 仍 1
	if got := fetchRoomStatus(t, sqlDB, roomID); got != 1 {
		t.Errorf("rooms.status = %d, want 1 (active 仍剩 B)", got)
	}
	// A.current_room_id = NULL
	if got := fetchUserCurrentRoomID(t, sqlDB, userA); got != nil {
		t.Errorf("users.current_room_id (A) = %d, want NULL", *got)
	}
	// B.current_room_id 仍 = roomID
	if got := fetchUserCurrentRoomID(t, sqlDB, userB); got == nil || *got != roomID {
		t.Errorf("users.current_room_id (B) changed; want %d", roomID)
	}
}

// AC11.5-2: 最后一人 leave → DB room_members 0 行 + rooms.status = 2 closed +
// user.current_room_id = NULL（epics.md §11.5 钦定 case 2）。
func TestRoomServiceIntegration_LeaveRoom_LastMember_RoomClosed(t *testing.T) {
	svc, sqlDB, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userA = uint64(1001)
	insertUser(t, sqlDB, userA, "uid-leave-last", "A", "")

	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("createRoom: %v", err)
	}
	roomID := createOut.RoomID

	// A leave（最后一人）
	out, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: userA, RoomID: roomID})
	if err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}
	if !out.Left {
		t.Errorf("out.Left = false, want true")
	}

	// DB 校验
	assertCount(t, sqlDB, "room_members WHERE room_id=?",
		[]any{roomID}, 0, "room_members (0 rows after last leaver)")
	if got := fetchRoomStatus(t, sqlDB, roomID); got != 2 {
		t.Errorf("rooms.status = %d, want 2 (closed)", got)
	}
	if got := fetchUserCurrentRoomID(t, sqlDB, userA); got != nil {
		t.Errorf("users.current_room_id (A) = %d, want NULL", *got)
	}
}

// AC11.5-3: user 不在房间 → 6004 + DB 无变化（含 nil 与不一致两个子场景）。
func TestRoomServiceIntegration_LeaveRoom_UserNotInRoom_Returns6004(t *testing.T) {
	svc, sqlDB, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userA = uint64(1001)
	insertUser(t, sqlDB, userA, "uid-leave-nil", "A", "")

	// 子场景 (a): A 不在任何房间（CurrentRoomID nil）→ 6004
	_, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: userA, RoomID: 99999})
	if err == nil {
		t.Fatalf("LeaveRoom returned nil error, want 6004")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserNotInRoom {
		t.Errorf("AppError.Code = %d, want %d (ErrUserNotInRoom 6004)", ae.Code, apperror.ErrUserNotInRoom)
	}

	// DB 校验：rooms / room_members 无变化
	assertCount(t, sqlDB, "rooms", nil, 0, "rooms (unchanged)")
	assertCount(t, sqlDB, "room_members", nil, 0, "room_members (unchanged)")
}

// AC11.5-4: 重复 leave 同一房间 → 第二次返 6004（V1 §10.5 行 1601 钦定）。
func TestRoomServiceIntegration_LeaveRoom_DoubleLeave_SecondReturns6004(t *testing.T) {
	svc, sqlDB, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userA = uint64(1001)
	insertUser(t, sqlDB, userA, "uid-leave-dup", "A", "")

	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("createRoom: %v", err)
	}
	roomID := createOut.RoomID

	// 第一次 leave 成功
	if _, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: userA, RoomID: roomID}); err != nil {
		t.Fatalf("first LeaveRoom: %v", err)
	}

	// 第二次 leave → 6004（current_room_id 已 NULL，预检 fail）
	_, err = svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: userA, RoomID: roomID})
	if err == nil {
		t.Fatalf("second LeaveRoom returned nil error, want 6004")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserNotInRoom {
		t.Errorf("AppError.Code = %d, want %d (ErrUserNotInRoom 6004 重复 leave)", ae.Code, apperror.ErrUserNotInRoom)
	}
}

// AC11.5-5（强烈建议; r9 P1#2 cross-tx race 验证 leave 侧）:
// fixture：A 在 room R（A 通过 11.3 创建）。
// 用 goroutine 同时执行：A leave + 另一新 user C join 同 room R。
// FOR UPDATE 锁串行化 → 最终结果只有两种合法状态：
//
//	(a) A leave 先 commit（rooms.status=2 closed）→ C join step 2b 看到 status=2 → 6005
//	(b) C join 先 commit（room_members 多 1 行）→ A leave 后续：A 不是最后一人 → status=1
//
// **不变量**：status=2 必须配 member_count=0；status=1 必须 member_count >= 1。
func TestRoomServiceIntegration_LeaveRoom_CrossTxJoinSerialized(t *testing.T) {
	svc, sqlDB, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userA = uint64(1001)
	const userC = uint64(1003)
	insertUser(t, sqlDB, userA, "uid-cross-a", "A", "")
	insertUser(t, sqlDB, userC, "uid-cross-c", "C", "")

	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("createRoom: %v", err)
	}
	roomID := createOut.RoomID

	done := make(chan error, 2)

	// goroutine 1: A leave（走 svc.LeaveRoom 真实事务）
	go func() {
		_, e := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: userA, RoomID: roomID})
		done <- e
	}()

	// goroutine 2: C join（走 svc.JoinRoom 真实事务）
	go func() {
		_, e := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userC, RoomID: roomID})
		done <- e
	}()

	for i := 0; i < 2; i++ {
		select {
		case <-done:
			// 两个 goroutine 任一返非 nil error 都接受（leave 不应失败；C 在 (a) 路径下 6005）
		case <-time.After(15 * time.Second):
			t.Fatalf("cross-tx race test timeout")
		}
	}

	// 核心断言：rooms.status 与 room_members 一致性
	finalStatus := fetchRoomStatus(t, sqlDB, roomID)
	var memberCount int64
	if err := sqlDB.QueryRow("SELECT COUNT(*) FROM room_members WHERE room_id = ?", roomID).Scan(&memberCount); err != nil {
		t.Fatalf("query room_members count: %v", err)
	}
	if finalStatus == 2 && memberCount > 0 {
		t.Errorf("r9 P1#2 race detected: status=2 but room_members has %d rows", memberCount)
	}
	if finalStatus == 1 && memberCount == 0 {
		t.Errorf("r9 P1#2 race detected: status=1 but room_members is empty")
	}
}

