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
	"encoding/json"
	stderrors "errors"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"

	wsapp "github.com/huing/cat/server/internal/app/ws"
	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
	"github.com/huing/cat/server/internal/pkg/auth"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
	"github.com/huing/cat/server/internal/service"
)

// buildRoomServiceIntegration: 起容器 → migrate → 装配 svc + 返清理 closure。
//
// **Story 11.8 r2 修复后**：post-commit hook（broadcastMemberJoined / closeLeaverSession
// / broadcastMemberLeft）异步化（独立 goroutine + detached ctx + 10s timeout）；
// 集成测试需注入 *sync.WaitGroup 让 cleanup 时 wg.Wait() 等所有 post-commit goroutine
// 完成后再关 DB / 销毁容器，否则 goroutine 持有 DB 句柄可能在 cleanup 后才返回 →
// "sql: database is closed" 噪声 / race（虽然 detached ctx 避免了 cancel 误中断，
// 但 DB 物理 close 仍会让查询失败）。
//
// **wg 暴露**：返给 caller 让需要断言 post-commit 副作用的 case 显式调 wg.Wait()
// 后再做断言；不需要断言副作用的 case 不必显式 wait（cleanup 路径会兜底 Wait）。
func buildRoomServiceIntegration(t *testing.T) (svc service.RoomService, sqlDB *sql.DB, wg *sync.WaitGroup, cleanup func()) {
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
	petRepo := mysql.NewPetRepo(gormDB)
	// Story 11.8 集成测试默认走 no-op SessionManager + capture-less broadcastFn；
	// 单独的 broadcast 路径集成 case 由专门的 buildRoomServiceIntegrationWithCapture
	// 构造（注入 capture broadcastFn 让 case 可断言调用次数 + 入参 wire 内容）。
	noopSessionMgr := wsapp.NewSessionManager()
	noopBroadcastFn := wsapp.BroadcastFn(func(ctx context.Context, roomID uint64, msg []byte) (int, error) { return 0, nil })
	// r3 [P1] fix：NewRoomService 8 参数；no-op BroadcastExceptFn 与 BroadcastFn 同模式
	noopBroadcastExceptFn := wsapp.BroadcastExceptFn(func(ctx context.Context, roomID, excludeUserID uint64, msg []byte) (int, error) { return 0, nil })

	svc = service.NewRoomService(txMgr, userRepo, roomRepo, roomMemberRepo, petRepo, noopSessionMgr, noopBroadcastFn, noopBroadcastExceptFn)
	wg = &sync.WaitGroup{}
	service.SetPostCommitWaitGroupForTest(svc, wg)

	rawDB, err := gormDB.DB()
	if err != nil {
		dockerCleanup()
		t.Fatalf("gormDB.DB(): %v", err)
	}

	cleanup = func() {
		// 等所有 post-commit goroutine 完成再关 DB —— 避免 goroutine 在 DB 关闭后
		// 仍试图查 DB 触发 "sql: database is closed" 噪声 / race。
		wg.Wait()
		_ = rawDB.Close()
		dockerCleanup()
	}
	return svc, rawDB, wg, cleanup
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
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
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
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
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
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
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
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
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
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
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
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
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
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
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
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
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
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
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
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
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
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
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
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
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
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
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



// ============================================================
// Story 11.6 集成测试 case：GetCurrentRoom + GetRoomDetail（dockertest）
// ============================================================

// AC11.6-1: GetCurrentRoom happy 用户在房间 → 返 *uint64 指向 roomID。
func TestRoomServiceIntegration_GetCurrentRoom_Happy_UserInRoom(t *testing.T) {
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userA = uint64(1001)
	insertUser(t, sqlDB, userA, "uid-curr-a", "A", "")

	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("createRoom: %v", err)
	}

	out, err := svc.GetCurrentRoom(context.Background(), service.GetCurrentRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("GetCurrentRoom: %v", err)
	}
	if out.RoomID == nil {
		t.Fatalf("out.RoomID = nil, want &%d", createOut.RoomID)
	}
	if *out.RoomID != createOut.RoomID {
		t.Errorf("out.RoomID = %d, want %d", *out.RoomID, createOut.RoomID)
	}
}

// AC11.6-2: GetCurrentRoom happy 用户不在任何房间 → 返 nil。
func TestRoomServiceIntegration_GetCurrentRoom_Happy_UserNotInAnyRoom(t *testing.T) {
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userA = uint64(1001)
	insertUser(t, sqlDB, userA, "uid-curr-none", "A", "")

	out, err := svc.GetCurrentRoom(context.Background(), service.GetCurrentRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("GetCurrentRoom: %v", err)
	}
	if out.RoomID != nil {
		t.Errorf("out.RoomID = %d, want nil (用户不在任何房间)", *out.RoomID)
	}
}

// AC11.6-3: GetRoomDetail happy 3 成员含 1 pet-less + memberCount === len(members) 不变量
// + ORDER BY joined_at ASC 顺序（A 创建 → B → C 按顺序加入）。
//
// pet-less 构造：A / B 通过 insertPet 显式 seed 一行 is_default=1 pet；C 不 seed
// pets 行（LEFT JOIN pets 时 pet_id 列 NULL → service 下发 Pet=nil）。
func TestRoomServiceIntegration_GetRoomDetail_Happy_3Members_With1PetLess(t *testing.T) {
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userA = uint64(1001)
	const userB = uint64(1002)
	const userC = uint64(1003)
	insertUser(t, sqlDB, userA, "uid-detail-a", "A", "https://avatar/a")
	insertUser(t, sqlDB, userB, "uid-detail-b", "B", "")
	insertUser(t, sqlDB, userC, "uid-detail-c", "C", "https://avatar/c")
	// A / B 有默认 pet
	insertPet(t, sqlDB, 8001, userA, 1, "PetA", 1, 1)
	insertPet(t, sqlDB, 8002, userB, 1, "PetB", 1, 1)
	// C 是 pet-less（不 seed pets 行）

	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("createRoom: %v", err)
	}
	roomID := createOut.RoomID

	if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userB, RoomID: roomID}); err != nil {
		t.Fatalf("JoinRoom B: %v", err)
	}
	// 加一点小间隔确保 joined_at ORDER 稳定（DATETIME(3) 毫秒精度足够；同毫秒会按 INSERT 顺序排序）
	time.Sleep(15 * time.Millisecond)
	if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userC, RoomID: roomID}); err != nil {
		t.Fatalf("JoinRoom C: %v", err)
	}

	out, err := svc.GetRoomDetail(context.Background(), service.GetRoomDetailInput{UserID: userA, RoomID: roomID})
	if err != nil {
		t.Fatalf("GetRoomDetail: %v", err)
	}
	// 不变量
	if out.MemberCount != 3 {
		t.Errorf("MemberCount = %d, want 3", out.MemberCount)
	}
	if len(out.Members) != 3 {
		t.Fatalf("len(Members) = %d, want 3", len(out.Members))
	}
	if out.MemberCount != len(out.Members) {
		t.Errorf("invariant violated: MemberCount=%d != len(Members)=%d", out.MemberCount, len(out.Members))
	}
	// 顺序按 joined_at ASC：A → B → C
	if out.Members[0].UserID != userA || out.Members[1].UserID != userB || out.Members[2].UserID != userC {
		t.Errorf("order = [%d %d %d], want [%d %d %d] (joined_at ASC)",
			out.Members[0].UserID, out.Members[1].UserID, out.Members[2].UserID, userA, userB, userC)
	}
	// A: 真实 nickname / avatarUrl + pet 含 PetID = 8001 + CurrentState 固定 1 + Equips 固定 []
	if out.Members[0].Nickname != "A" || out.Members[0].AvatarURL != "https://avatar/a" {
		t.Errorf("Members[0] nick/avatar = %q/%q, want A/https://avatar/a", out.Members[0].Nickname, out.Members[0].AvatarURL)
	}
	if out.Members[0].Pet == nil {
		t.Fatalf("Members[0].Pet = nil, want non-nil")
	}
	if out.Members[0].Pet.PetID != 8001 {
		t.Errorf("Members[0].Pet.PetID = %d, want 8001", out.Members[0].Pet.PetID)
	}
	if out.Members[0].Pet.CurrentState != 1 {
		t.Errorf("Members[0].Pet.CurrentState = %d, want 1 (节点 4 固定)", out.Members[0].Pet.CurrentState)
	}
	if out.Members[0].Pet.Equips == nil || len(out.Members[0].Pet.Equips) != 0 {
		t.Errorf("Members[0].Pet.Equips = %v, want []EquipOutput{} 节点 4 阶段固定空", out.Members[0].Pet.Equips)
	}
	// B: avatarUrl 为空字符串（合法）
	if out.Members[1].AvatarURL != "" {
		t.Errorf("Members[1].AvatarURL = %q, want empty string", out.Members[1].AvatarURL)
	}
	// C: pet-less → Pet == nil
	if out.Members[2].Pet != nil {
		t.Errorf("Members[2].Pet = %+v, want nil (pet-less)", out.Members[2].Pet)
	}
	// room 字段
	if out.RoomID != roomID {
		t.Errorf("RoomID = %d, want %d", out.RoomID, roomID)
	}
	if out.CreatorUserID != userA {
		t.Errorf("CreatorUserID = %d, want %d", out.CreatorUserID, userA)
	}
	if out.Status != 1 {
		t.Errorf("Status = %d, want 1 (active)", out.Status)
	}
	if out.MaxMembers != 4 {
		t.Errorf("MaxMembers = %d, want 4", out.MaxMembers)
	}
}

// TestRoomServiceIntegration_GetRoomDetail_RealCurrentState_1_2_3:
// Story 14.3 集成测试 —— dockertest 真实 MySQL：3 user 各 seed pet.current_state=1/2/3
// + A 创建房间 + B/C 顺序加入 → A 调 GetRoomDetail → Output.Members[i].Pet.CurrentState
// 真实驱动 1/2/3（端到端验证 SQL SELECT + service 拼装 + 三档枚举均覆盖）。
//
// 验证 Story 14.3 三处统一切换路径之一（GET /rooms 真实驱动 pet.currentState）：
// service.GetRoomDetail 内 r.PetID != nil 分支把 `CurrentState: 1` 改为
// `CurrentState: *r.CurrentState` 后真实读 pets.current_state。
func TestRoomServiceIntegration_GetRoomDetail_RealCurrentState_1_2_3(t *testing.T) {
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userA = uint64(1001)
	const userB = uint64(1002)
	const userC = uint64(1003)
	insertUser(t, sqlDB, userA, "uid-cs-a", "A", "https://avatar/a")
	insertUser(t, sqlDB, userB, "uid-cs-b", "B", "https://avatar/b")
	insertUser(t, sqlDB, userC, "uid-cs-c", "C", "https://avatar/c")
	// 各 user pet.current_state 分别 1=rest / 2=walk / 3=run
	insertPet(t, sqlDB, 9001, userA, 1, "PetA", 1, 1)
	insertPet(t, sqlDB, 9002, userB, 1, "PetB", 2, 1)
	insertPet(t, sqlDB, 9003, userC, 1, "PetC", 3, 1)

	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("createRoom: %v", err)
	}
	roomID := createOut.RoomID
	if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userB, RoomID: roomID}); err != nil {
		t.Fatalf("JoinRoom B: %v", err)
	}
	time.Sleep(15 * time.Millisecond)
	if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userC, RoomID: roomID}); err != nil {
		t.Fatalf("JoinRoom C: %v", err)
	}

	out, err := svc.GetRoomDetail(context.Background(), service.GetRoomDetailInput{UserID: userA, RoomID: roomID})
	if err != nil {
		t.Fatalf("GetRoomDetail: %v", err)
	}
	if len(out.Members) != 3 {
		t.Fatalf("len(Members) = %d, want 3", len(out.Members))
	}
	wantStates := []int8{1, 2, 3}
	wantPetIDs := []uint64{9001, 9002, 9003}
	for i, want := range wantStates {
		if out.Members[i].Pet == nil {
			t.Fatalf("Members[%d].Pet = nil, want non-nil", i)
		}
		if out.Members[i].Pet.PetID != wantPetIDs[i] {
			t.Errorf("Members[%d].Pet.PetID = %d, want %d", i, out.Members[i].Pet.PetID, wantPetIDs[i])
		}
		if out.Members[i].Pet.CurrentState != want {
			t.Errorf("Members[%d].Pet.CurrentState = %d, want %d (Story 14.3 真实驱动 pets.current_state)", i, out.Members[i].Pet.CurrentState, want)
		}
	}
}

// AC11.6-4: user B 不加入房间 → user B 调 GetRoomDetail(A 的 roomID) → 6004。
func TestRoomServiceIntegration_GetRoomDetail_UserNotInRoom_Returns6004(t *testing.T) {
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userA = uint64(1001)
	const userB = uint64(1002)
	insertUser(t, sqlDB, userA, "uid-acl-a", "A", "")
	insertUser(t, sqlDB, userB, "uid-acl-b", "B", "")

	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("createRoom: %v", err)
	}
	roomID := createOut.RoomID

	// B 不加入房间 → B 调 GetRoomDetail(A 的 roomID) → 6004（步骤 1a 预检 user.current_room_id != roomID）
	_, err = svc.GetRoomDetail(context.Background(), service.GetRoomDetailInput{UserID: userB, RoomID: roomID})
	if err == nil {
		t.Fatalf("GetRoomDetail returned nil error, want 6004")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserNotInRoom {
		t.Errorf("AppError.Code = %d, want %d (6004)", ae.Code, apperror.ErrUserNotInRoom)
	}
}

// AC11.6-5: closed 房间 + caller 已离开 → 6004（V1 §10.3 行 1347 钦定 closed 房间允许
// 查询但前提是 caller 仍是该房间成员；caller 已离开必返 6004）。
//
// fixture：A 创建房间 → A leave（最后一人） → 此时 rooms.status=2 closed +
// users.current_room_id=NULL。A 调 GetRoomDetail(roomID) → 6004。
func TestRoomServiceIntegration_GetRoomDetail_ClosedRoom_CallerAlreadyLeft_Returns6004(t *testing.T) {
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userA = uint64(1001)
	insertUser(t, sqlDB, userA, "uid-closed-a", "A", "")

	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("createRoom: %v", err)
	}
	roomID := createOut.RoomID

	// A leave（最后一人 → status=2 + current_room_id=NULL）
	if _, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: userA, RoomID: roomID}); err != nil {
		t.Fatalf("LeaveRoom A: %v", err)
	}

	// 校验 status=2 + current_room_id=NULL
	if got := fetchRoomStatus(t, sqlDB, roomID); got != 2 {
		t.Errorf("rooms.status = %d, want 2 (closed)", got)
	}
	if got := fetchUserCurrentRoomID(t, sqlDB, userA); got != nil {
		t.Errorf("users.current_room_id (A) = %d, want NULL", *got)
	}

	// A 调 GetRoomDetail(roomID) → 步骤 1a 预检 current_room_id == nil → 6004
	_, err = svc.GetRoomDetail(context.Background(), service.GetRoomDetailInput{UserID: userA, RoomID: roomID})
	if err == nil {
		t.Fatalf("GetRoomDetail returned nil error, want 6004 (caller 已离开 closed 房间)")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserNotInRoom {
		t.Errorf("AppError.Code = %d, want %d (6004; closed 房间 caller 已离开)", ae.Code, apperror.ErrUserNotInRoom)
	}
}

// ============================================================
// Story 11.8 集成测试：broadcast / close 触发路径（dockertest + 真实 SessionManager
// + capture broadcastFn）
// ============================================================

// recordedBroadcastCall 记录 capture broadcastFn / broadcastExceptFn 的单次调用
// （r3 fix 起 excludeUserID 字段非 nil 表示 BroadcastExceptFn 路径调用）。
type recordedBroadcastCall struct {
	roomID        uint64
	msg           []byte
	seq           int64   // 单调序号（与 capture-aware close 4007 路径中 SessionManager Unregister 共用同一时钟）
	excludeUserID *uint64 // r3：BroadcastExceptFn 调用时记录；BroadcastFn 调用时 nil
}

// captureBroadcastFn 包装 ws.BroadcastFn / ws.BroadcastExceptFn；内部统一记录所有
// 调用便于断言（r3 起 service 层 broadcastMemberJoined / broadcastMemberLeft 走
// BroadcastExceptFn 路径必须 exclude 事件主体；本 capture 同时支持两种 fn 注入）。
type captureBroadcastFn struct {
	mu     sync.Mutex
	calls  []recordedBroadcastCall
	seqGen *atomic.Int64
}

func newCaptureBroadcastFn(seqGen *atomic.Int64) *captureBroadcastFn {
	return &captureBroadcastFn{seqGen: seqGen}
}

func (c *captureBroadcastFn) fn() wsapp.BroadcastFn {
	return func(ctx context.Context, roomID uint64, msg []byte) (int, error) {
		c.mu.Lock()
		copied := append([]byte(nil), msg...)
		c.calls = append(c.calls, recordedBroadcastCall{
			roomID: roomID,
			msg:    copied,
			seq:    c.seqGen.Add(1),
		})
		c.mu.Unlock()
		return 0, nil
	}
}

// exceptFn 返回 ws.BroadcastExceptFn type alias 兼容的 closure（r3 引入）。
// 与 fn 共享 calls 切片；excludeUserID 字段非 nil 让断言能区分调用类型。
func (c *captureBroadcastFn) exceptFn() wsapp.BroadcastExceptFn {
	return func(ctx context.Context, roomID, excludeUserID uint64, msg []byte) (int, error) {
		c.mu.Lock()
		copied := append([]byte(nil), msg...)
		excluded := excludeUserID
		c.calls = append(c.calls, recordedBroadcastCall{
			roomID:        roomID,
			msg:           copied,
			seq:           c.seqGen.Add(1),
			excludeUserID: &excluded,
		})
		c.mu.Unlock()
		return 0, nil
	}
}

func (c *captureBroadcastFn) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

// buildRoomServiceIntegrationWithCapture 同 buildRoomServiceIntegration，但注入
// 真实 ws.SessionManager + capture broadcastFn 让 case 可断言 broadcast / close 4007
// 触发路径。同时返回 sessionMgr / capture 给 case 直接消费。
func buildRoomServiceIntegrationWithCapture(t *testing.T) (
	svc service.RoomService,
	sqlDB *sql.DB,
	sessionMgr wsapp.SessionManager,
	capture *captureBroadcastFn,
	wg *sync.WaitGroup,
	cleanup func(),
) {
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
	petRepo := mysql.NewPetRepo(gormDB)

	sessionMgr = wsapp.NewSessionManager()
	seqGen := &atomic.Int64{}
	capture = newCaptureBroadcastFn(seqGen)

	svc = service.NewRoomService(txMgr, userRepo, roomRepo, roomMemberRepo, petRepo, sessionMgr, capture.fn(), capture.exceptFn())
	wg = &sync.WaitGroup{}
	service.SetPostCommitWaitGroupForTest(svc, wg)

	rawDB, err := gormDB.DB()
	if err != nil {
		dockerCleanup()
		t.Fatalf("gormDB.DB(): %v", err)
	}

	cleanup = func() {
		// 等所有 post-commit goroutine 完成再关 DB / SessionManager —— Story 11.8 r2
		// 修复后 post-commit hook 异步化，cleanup 必须先 wg.Wait() 才安全 close。
		wg.Wait()
		_ = sessionMgr.Close()
		_ = rawDB.Close()
		dockerCleanup()
	}
	return svc, rawDB, sessionMgr, capture, wg, cleanup
}

// integrationEnvelope 测试本地 wire mirror（与 ws.serverEnvelope 字段集合等价）。
type integrationEnvelope struct {
	Type      string          `json:"type"`
	RequestID string          `json:"requestId"`
	Payload   json.RawMessage `json:"payload"`
	Ts        int64           `json:"ts"`
}

// case 14: B join → broadcastFn 被调用 1 次 + payload 字段值正确（含 nickname /
// avatarUrl / pet）+ A 在 fixture 已建房；本 case 验证：commit 成功后 fire-and-forget
// 路径在真实 dockertest 数据库 + 真实 SessionManager 下端到端工作
func TestRoomServiceIntegration_JoinRoom_Happy_BroadcastFnInvokedOnce(t *testing.T) {
	svc, sqlDB, _, capture, wg, cleanup := buildRoomServiceIntegrationWithCapture(t)
	defer cleanup()

	const userA = uint64(1001)
	const userB = uint64(1002)
	insertUser(t, sqlDB, userA, "uid-118-a", "用户A", "")
	insertUser(t, sqlDB, userB, "uid-118-b", "用户B", "https://avatar/b")
	// B 有默认 pet → broadcast payload pet ≠ null
	insertPet(t, sqlDB, 8002, userB, 1, "PetB", 1, 1)

	// A 创建房间
	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("createRoom: %v", err)
	}
	roomID := createOut.RoomID
	// 重置 capture（A 的 createRoom 不广播 member.joined，但 B 的 join 才是本 case 焦点；
	// 这里清空让断言只看 join 触发的 1 次）
	capture.mu.Lock()
	capture.calls = nil
	capture.mu.Unlock()

	// B join
	if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userB, RoomID: roomID}); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	// **r2 修复后**：post-commit hook 异步化，必须等 wg 才安全断言 capture / sessionMgr 副作用
	wg.Wait()

	// 校验 capture：broadcastFn 被调 1 次 + roomID 正确 + payload 字段值正确
	if got := capture.callCount(); got != 1 {
		t.Fatalf("broadcastFn call count = %d, want 1 (after B join)", got)
	}
	call := capture.calls[0]
	if call.roomID != roomID {
		t.Errorf("call.roomID = %d, want %d", call.roomID, roomID)
	}

	var env integrationEnvelope
	if err := json.Unmarshal(call.msg, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.Type != "member.joined" {
		t.Errorf("env.Type = %q, want \"member.joined\"", env.Type)
	}

	var payload struct {
		UserID    string `json:"userId"`
		Nickname  string `json:"nickname"`
		AvatarURL string `json:"avatarUrl"`
		Pet       *struct {
			PetID        string `json:"petId"`
			CurrentState int    `json:"currentState"`
		} `json:"pet"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.UserID != "1002" {
		t.Errorf("payload.userId = %q, want \"1002\"", payload.UserID)
	}
	if payload.Nickname != "用户B" {
		t.Errorf("payload.nickname = %q, want \"用户B\"", payload.Nickname)
	}
	if payload.AvatarURL != "https://avatar/b" {
		t.Errorf("payload.avatarUrl = %q, want \"https://avatar/b\"", payload.AvatarURL)
	}
	if payload.Pet == nil {
		t.Fatalf("payload.pet = nil, want object (B 已 seed default pet)")
	}
	if payload.Pet.PetID != "8002" {
		t.Errorf("payload.pet.petId = %q, want \"8002\"", payload.Pet.PetID)
	}
	if payload.Pet.CurrentState != 1 {
		t.Errorf("payload.pet.currentState = %d, want 1", payload.Pet.CurrentState)
	}
}

// case J1b: r1 fix (Story 14.3 review) — broadcastMemberJoined 真实驱动 pet.currentState
// 的 **integration 端到端证明**。既有 J1a case 14 seed pets.current_state=1 → broadcast
// payload.pet.currentState==1 与切换前 hardcoded 路径**也**能通过，无法证明 site (iii)
// 真的切了。本 case seed pets.current_state=2 (walk) → join → 断言 broadcast 出来的
// payload.pet.currentState == 2 (= seeded mysql 真实值)，证明 broadcastMemberJoined
// 自 Story 14.3 起从 mysql.Pet.CurrentState 读真实值（V1 §12.3 line 2121 钦定）。
// unit 层对应 case：TestRoomService_BroadcastMemberJoined_PetCurrentState_2
// (room_service_test.go:2625)。
func TestRoomServiceIntegration_JoinRoom_BroadcastMemberJoined_PetCurrentState_2(t *testing.T) {
	svc, sqlDB, _, capture, wg, cleanup := buildRoomServiceIntegrationWithCapture(t)
	defer cleanup()

	const userA = uint64(1001)
	const userB = uint64(1002)
	insertUser(t, sqlDB, userA, "uid-143-r1-a", "用户A", "")
	insertUser(t, sqlDB, userB, "uid-143-r1-b", "用户B", "https://avatar/b")
	// 关键：B 的 pet seed current_state=2 (walk)，非 hardcoded 1
	insertPet(t, sqlDB, 8002, userB, 1, "PetB", 2, 1)

	// A 创建房间
	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("createRoom: %v", err)
	}
	roomID := createOut.RoomID
	capture.mu.Lock()
	capture.calls = nil
	capture.mu.Unlock()

	// B join
	if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userB, RoomID: roomID}); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	wg.Wait()

	if got := capture.callCount(); got != 1 {
		t.Fatalf("broadcastFn call count = %d, want 1", got)
	}
	call := capture.calls[0]
	if call.roomID != roomID {
		t.Errorf("call.roomID = %d, want %d", call.roomID, roomID)
	}

	var env integrationEnvelope
	if err := json.Unmarshal(call.msg, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.Type != "member.joined" {
		t.Errorf("env.Type = %q, want \"member.joined\"", env.Type)
	}

	var payload struct {
		Pet *struct {
			PetID        string `json:"petId"`
			CurrentState int    `json:"currentState"`
		} `json:"pet"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Pet == nil {
		t.Fatalf("payload.pet = nil, want non-nil (B 已 seed default pet)")
	}
	if payload.Pet.PetID != "8002" {
		t.Errorf("payload.pet.petId = %q, want \"8002\"", payload.Pet.PetID)
	}
	// 核心断言：currentState 必须 = 2（seed 值），证明 site (iii) 真的从 mysql.Pet 读真实值
	if payload.Pet.CurrentState != 2 {
		t.Errorf("payload.pet.currentState = %d, want 2 (Story 14.3 真实驱动 mysql.Pet.CurrentState；hardcoded 路径会返 1)", payload.Pet.CurrentState)
	}
}

// case 15: B leave → close 路径走 no-op（B 未持 WS Session，sessionMgr.ListSessionsByRoomID
// 返 []）+ broadcastFn 被调用 1 次 + payload type=member.left + payload.userId 正确
func TestRoomServiceIntegration_LeaveRoom_Happy_BroadcastsMemberLeft(t *testing.T) {
	svc, sqlDB, sessionMgr, capture, wg, cleanup := buildRoomServiceIntegrationWithCapture(t)
	defer cleanup()

	const userA = uint64(1001)
	const userB = uint64(1002)
	insertUser(t, sqlDB, userA, "uid-118-leave-a", "A", "")
	insertUser(t, sqlDB, userB, "uid-118-leave-b", "B", "")

	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("createRoom: %v", err)
	}
	roomID := createOut.RoomID

	if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userB, RoomID: roomID}); err != nil {
		t.Fatalf("JoinRoom: %v", err)
	}
	// 等 B join 的 post-commit goroutine 完成（不然下面 capture.calls = nil 可能在
	// goroutine append 之前生效，导致后面断言 leaver broadcast 时多 / 少计数）
	wg.Wait()

	capture.mu.Lock()
	capture.calls = nil
	capture.mu.Unlock()

	// B leave —— B 未持 WS Session，closeLeaverSession 走 no-op
	if _, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: userB, RoomID: roomID}); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}
	// **r2 修复后**：post-commit hook 异步化，必须等 wg 才安全断言 capture / sessionMgr 副作用
	wg.Wait()

	// 校验 sessionMgr：B 的 Session 不存在过（leave 路径不会创建 Session）
	if sessions := sessionMgr.ListSessionsByRoomID(context.Background(), roomID); len(sessions) != 0 {
		t.Errorf("sessions count = %d, want 0 (no WS Sessions registered)", len(sessions))
	}

	// 校验 capture：broadcastFn 被调用 1 次 + payload type=member.left + payload.userId 正确
	if got := capture.callCount(); got != 1 {
		t.Fatalf("broadcastFn call count = %d, want 1 (after B leave)", got)
	}
	call := capture.calls[0]
	if call.roomID != roomID {
		t.Errorf("call.roomID = %d, want %d", call.roomID, roomID)
	}

	var env integrationEnvelope
	if err := json.Unmarshal(call.msg, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.Type != "member.left" {
		t.Errorf("env.Type = %q, want \"member.left\"", env.Type)
	}
	var payload struct {
		UserID string `json:"userId"`
	}
	if err := json.Unmarshal(env.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.UserID != "1002" {
		t.Errorf("payload.userId = %q, want \"1002\"", payload.UserID)
	}
}

// case 16: 最后一人 leave → 房间 status=closed + broadcastFn 仍被调用 1 次（虽然
// 无其他成员收 broadcast，service 层仍 trigger broadcastFn；与 V1 §10.5 步骤 8 钦定
// 一致）
func TestRoomServiceIntegration_LeaveRoom_LastMember_StillBroadcasts(t *testing.T) {
	svc, sqlDB, _, capture, wg, cleanup := buildRoomServiceIntegrationWithCapture(t)
	defer cleanup()

	const userA = uint64(1001)
	insertUser(t, sqlDB, userA, "uid-118-last", "A", "")

	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("createRoom: %v", err)
	}
	roomID := createOut.RoomID

	capture.mu.Lock()
	capture.calls = nil
	capture.mu.Unlock()

	// A leave（最后一人）
	if _, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: userA, RoomID: roomID}); err != nil {
		t.Fatalf("LeaveRoom: %v", err)
	}
	// **r2 修复后**：post-commit hook 异步化，必须等 wg 才安全断言 capture 副作用
	wg.Wait()

	// 房间状态变为 closed=2
	if got := fetchRoomStatus(t, sqlDB, roomID); got != 2 {
		t.Errorf("rooms.status = %d, want 2 closed (last member leave)", got)
	}

	// broadcastFn 仍被调用 1 次（房间内无其他成员收 broadcast，但 service 层仍 trigger）
	if got := capture.callCount(); got != 1 {
		t.Errorf("broadcastFn call count = %d, want 1 (last member leave still broadcasts)", got)
	}
}

// ============================================================
// Story 11.9 集成测试：Layer 2 房间生命周期全流程（AC2 ~ AC10）
//
// 本段追加 ≥9 个新 case 覆盖 epics.md §Story 11.9 钦定的 10 类场景：
//
//	1. 完整生命周期 → AC2.1 TestRoomServiceIntegration_FullLifecycle_5UsersJoinFullLeaveAllClosed
//	2. 回滚 1 (创建 room_members 失败) → AC3 TestRoomServiceIntegration_CreateRoom_FaultInjectionRoomMembersStep_RoomsAlsoRollback
//	3. 回滚 2 (加入 users update 失败) → AC4 TestRoomServiceIntegration_JoinRoom_FaultInjectionUsersUpdate_RoomMembersAlsoRollback
//	4. 回滚 3 (退出 users update 失败) → AC5 TestRoomServiceIntegration_LeaveRoom_FaultInjectionUsersUpdate_RoomMembersDeleteAlsoRollback
//	5. 并发 1 (5 user 同时 join) → AC6 TestRoomServiceIntegration_JoinRoom_Concurrent5UsersIntoFullRoom_OnlyOneSucceeds
//	6. 并发 2 (100 user create) → AC7 TestRoomServiceIntegration_CreateRoom_Concurrent100DifferentUsers_100RoomsCreated
//	7. 边界 1 (A 在 X 创建新房间 → 6003) → 已被 11.3 TestRoomServiceIntegration_CreateRoom_AlreadyInRoom_PrecheckReturns6003 覆盖（不新增）
//	8. 边界 2 (A 在 X 调 X/join → 6003) → AC10 TestRoomServiceIntegration_JoinRoom_UserAlreadyInSameRoom_Returns6003
//	9. 边界 3 (closed 房间 join → 6005) → 已被 11.4 TestRoomServiceIntegration_JoinRoom_RoomClosed_Returns6005 覆盖（不新增）
//	10. WS 联动 (B leave → A 收 member.left) → AC8 TestRoomServiceIntegration_LeaveRoom_RealWSSession_BroadcastsMemberLeftToOtherMembers
//
// fault injection wrapper 模式与 4.7 落地的 faultPetRepo / faultChestRepo / faultUserRepo
// 同源（按方法包装真实 mysql repo + 在指定方法替换为 sentinel error，其他方法透传）。
// ============================================================

// faultRoomMemberRepo 包装真实 RoomMemberRepo，让 Create 抛 injectErr，其他方法透传。
// 用于 AC3：回滚 1（创建房间 - room_members.Create 失败 → rooms 也回滚）。
type faultRoomMemberRepo struct {
	delegate  mysql.RoomMemberRepo
	injectErr error
}

func (f *faultRoomMemberRepo) Create(ctx context.Context, m *mysql.RoomMember) error {
	return f.injectErr
}

func (f *faultRoomMemberRepo) RoomExists(ctx context.Context, roomID uint64) (bool, error) {
	return f.delegate.RoomExists(ctx, roomID)
}

func (f *faultRoomMemberRepo) IsUserInRoom(ctx context.Context, userID uint64, roomID uint64) (bool, error) {
	return f.delegate.IsUserInRoom(ctx, userID, roomID)
}

func (f *faultRoomMemberRepo) ListMembers(ctx context.Context, roomID uint64) ([]uint64, error) {
	return f.delegate.ListMembers(ctx, roomID)
}

func (f *faultRoomMemberRepo) CountByRoomID(ctx context.Context, roomID uint64) (int, error) {
	return f.delegate.CountByRoomID(ctx, roomID)
}

func (f *faultRoomMemberRepo) DeleteByRoomAndUser(ctx context.Context, roomID, userID uint64) (int64, error) {
	return f.delegate.DeleteByRoomAndUser(ctx, roomID, userID)
}

func (f *faultRoomMemberRepo) ExistsForShareByRoomAndUser(ctx context.Context, roomID, userID uint64) (bool, error) {
	return f.delegate.ExistsForShareByRoomAndUser(ctx, roomID, userID)
}

func (f *faultRoomMemberRepo) ListRosterByRoomID(ctx context.Context, roomID uint64) ([]mysql.RosterRow, error) {
	return f.delegate.ListRosterByRoomID(ctx, roomID)
}

// faultUserRepoForJoin 包装真实 UserRepo，让 UpdateCurrentRoomID 抛 injectErr，其他方法透传。
// 用于 AC4：回滚 2（加入房间 - users.UpdateCurrentRoomID 失败 → room_members 也回滚）。
type faultUserRepoForJoin struct {
	delegate  mysql.UserRepo
	injectErr error
}

func (f *faultUserRepoForJoin) Create(ctx context.Context, u *mysql.User) error {
	return f.delegate.Create(ctx, u)
}

func (f *faultUserRepoForJoin) UpdateNickname(ctx context.Context, userID uint64, nickname string) error {
	return f.delegate.UpdateNickname(ctx, userID, nickname)
}

func (f *faultUserRepoForJoin) FindByID(ctx context.Context, id uint64) (*mysql.User, error) {
	return f.delegate.FindByID(ctx, id)
}

func (f *faultUserRepoForJoin) UpdateCurrentRoomID(ctx context.Context, userID uint64, roomID *uint64) error {
	return f.injectErr
}

// faultUserRepoForLeave 包装真实 UserRepo，**仅在 leave 路径**（roomID == nil）抛 injectErr。
// create 与 join 路径（roomID != nil）透传。用于 AC5：回滚 3（退出房间 -
// users.UpdateCurrentRoomID(nil) 失败 → room_members 删除也回滚）。
type faultUserRepoForLeave struct {
	delegate  mysql.UserRepo
	injectErr error
}

func (f *faultUserRepoForLeave) Create(ctx context.Context, u *mysql.User) error {
	return f.delegate.Create(ctx, u)
}

func (f *faultUserRepoForLeave) UpdateNickname(ctx context.Context, userID uint64, nickname string) error {
	return f.delegate.UpdateNickname(ctx, userID, nickname)
}

func (f *faultUserRepoForLeave) FindByID(ctx context.Context, id uint64) (*mysql.User, error) {
	return f.delegate.FindByID(ctx, id)
}

func (f *faultUserRepoForLeave) UpdateCurrentRoomID(ctx context.Context, userID uint64, roomID *uint64) error {
	if roomID == nil {
		// leave 路径才注入 fault；create / join 路径透传以让 fixture 阶段成功
		return f.injectErr
	}
	return f.delegate.UpdateCurrentRoomID(ctx, userID, roomID)
}

// buildRoomServiceWithCustomRepos 装配 RoomService 但允许 caller 注入自定义 repo
// （用于 fault injection case）。返 (svc, sqlDB, wg, cleanup)。
//
// **关键**：noop sessionMgr / broadcastFn 与 buildRoomServiceIntegration 同；
// 仅 user / room / roomMember repo 可被 caller 替换。
func buildRoomServiceWithCustomRepos(
	t *testing.T,
	overrideUserRepo func(real mysql.UserRepo) mysql.UserRepo,
	overrideRoomRepo func(real mysql.RoomRepo) mysql.RoomRepo,
	overrideRoomMemberRepo func(real mysql.RoomMemberRepo) mysql.RoomMemberRepo,
) (svc service.RoomService, sqlDB *sql.DB, wg *sync.WaitGroup, cleanup func()) {
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
	realUserRepo := mysql.NewUserRepo(gormDB)
	realRoomRepo := mysql.NewRoomRepo(gormDB)
	realRoomMemberRepo := mysql.NewRoomMemberRepo(gormDB)
	petRepo := mysql.NewPetRepo(gormDB)

	userRepo := mysql.UserRepo(realUserRepo)
	if overrideUserRepo != nil {
		userRepo = overrideUserRepo(realUserRepo)
	}
	roomRepo := mysql.RoomRepo(realRoomRepo)
	if overrideRoomRepo != nil {
		roomRepo = overrideRoomRepo(realRoomRepo)
	}
	roomMemberRepo := mysql.RoomMemberRepo(realRoomMemberRepo)
	if overrideRoomMemberRepo != nil {
		roomMemberRepo = overrideRoomMemberRepo(realRoomMemberRepo)
	}

	noopSessionMgr := wsapp.NewSessionManager()
	noopBroadcastFn := wsapp.BroadcastFn(func(ctx context.Context, roomID uint64, msg []byte) (int, error) { return 0, nil })
	noopBroadcastExceptFn := wsapp.BroadcastExceptFn(func(ctx context.Context, roomID, excludeUserID uint64, msg []byte) (int, error) { return 0, nil })

	svc = service.NewRoomService(txMgr, userRepo, roomRepo, roomMemberRepo, petRepo, noopSessionMgr, noopBroadcastFn, noopBroadcastExceptFn)
	wg = &sync.WaitGroup{}
	service.SetPostCommitWaitGroupForTest(svc, wg)

	rawDB, err := gormDB.DB()
	if err != nil {
		dockerCleanup()
		t.Fatalf("gormDB.DB(): %v", err)
	}

	cleanup = func() {
		wg.Wait()
		_ = rawDB.Close()
		dockerCleanup()
	}
	return svc, rawDB, wg, cleanup
}

// AC2.1 完整生命周期: 跨 7 个事务 + 5 user + 跨 status transition
// A 创建 → B/C/D 依次 join → E join 返 6002 → A leave → B/C/D 依次 leave →
// 最后一人 leave 触发 status=2 closed。
//
// 每跨 1 个事务边界做 memberCount + status + users.current_room_id 三连断言。
func TestRoomServiceIntegration_FullLifecycle_5UsersJoinFullLeaveAllClosed(t *testing.T) {
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userA = uint64(1001)
	const userB = uint64(1002)
	const userC = uint64(1003)
	const userD = uint64(1004)
	const userE = uint64(1005)
	insertUser(t, sqlDB, userA, "uid-life-a", "A", "")
	insertUser(t, sqlDB, userB, "uid-life-b", "B", "")
	insertUser(t, sqlDB, userC, "uid-life-c", "C", "")
	insertUser(t, sqlDB, userD, "uid-life-d", "D", "")
	insertUser(t, sqlDB, userE, "uid-life-e", "E", "")

	// 阶段 1: A 创建 → memberCount=1, status=1, A.current_room_id=roomID
	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("CreateRoom A: %v", err)
	}
	roomID := createOut.RoomID
	assertCount(t, sqlDB, "room_members WHERE room_id=?", []any{roomID}, 1, "after A create")
	if got := fetchRoomStatus(t, sqlDB, roomID); got != 1 {
		t.Errorf("after A create rooms.status = %d, want 1", got)
	}
	if got := fetchUserCurrentRoomID(t, sqlDB, userA); got == nil || *got != roomID {
		t.Errorf("after A create A.current_room_id mismatch")
	}

	// 阶段 2-4: B / C / D 依次 join → memberCount 升到 4
	for i, u := range []uint64{userB, userC, userD} {
		if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: u, RoomID: roomID}); err != nil {
			t.Fatalf("JoinRoom user=%d: %v", u, err)
		}
		wantCount := int64(2 + i)
		assertCount(t, sqlDB, "room_members WHERE room_id=?", []any{roomID}, wantCount,
			"after joiner #"+string(rune('B'+i)))
		if got := fetchRoomStatus(t, sqlDB, roomID); got != 1 {
			t.Errorf("after joiner %d rooms.status = %d, want 1", u, got)
		}
		if got := fetchUserCurrentRoomID(t, sqlDB, u); got == nil || *got != roomID {
			t.Errorf("after joiner %d users.current_room_id mismatch", u)
		}
	}

	// 阶段 5: E join → 6002 + memberCount 仍 4 + E.current_room_id 仍 nil
	_, err = svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userE, RoomID: roomID})
	if err == nil {
		t.Fatalf("E join returned nil, want 6002")
	}
	ae, ok := apperror.As(err)
	if !ok || ae.Code != apperror.ErrRoomFull {
		t.Errorf("E join AppError = %v, want 6002 ErrRoomFull", err)
	}
	assertCount(t, sqlDB, "room_members WHERE room_id=?", []any{roomID}, 4, "after E rejected")
	if got := fetchUserCurrentRoomID(t, sqlDB, userE); got != nil {
		t.Errorf("after E reject E.current_room_id = %d, want nil", *got)
	}

	// 阶段 6-9: A / B / C / D 依次 leave → memberCount 4→3→2→1→0；最后一人 D leave 触发 status=2
	leaveOrder := []uint64{userA, userB, userC, userD}
	for i, u := range leaveOrder {
		if _, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: u, RoomID: roomID}); err != nil {
			t.Fatalf("LeaveRoom user=%d: %v", u, err)
		}
		wantCount := int64(3 - i)
		wantStatus := int8(1)
		if i == 3 {
			wantStatus = 2 // 最后一人 leave → closed
		}
		assertCount(t, sqlDB, "room_members WHERE room_id=?", []any{roomID}, wantCount,
			"after leaver #"+string(rune('A'+i)))
		if got := fetchRoomStatus(t, sqlDB, roomID); got != wantStatus {
			t.Errorf("after leaver %d rooms.status = %d, want %d", u, got, wantStatus)
		}
		if got := fetchUserCurrentRoomID(t, sqlDB, u); got != nil {
			t.Errorf("after leaver %d users.current_room_id = %d, want nil", u, *got)
		}
	}

	// 收尾断言：rooms.status=2 + room_members 0 行 + 5 user 全部 current_room_id=nil
	if got := fetchRoomStatus(t, sqlDB, roomID); got != 2 {
		t.Errorf("final rooms.status = %d, want 2 closed", got)
	}
	assertCount(t, sqlDB, "room_members WHERE room_id=?", []any{roomID}, 0, "final room_members empty")
	for _, u := range []uint64{userA, userB, userC, userD, userE} {
		if got := fetchUserCurrentRoomID(t, sqlDB, u); got != nil {
			t.Errorf("final user %d current_room_id = %d, want nil", u, *got)
		}
	}
}

// AC3 回滚 1: room_members.Create 失败 → rooms / users.current_room_id 也回滚（DB 全空）
//
// 验证 InnoDB 真实 ROLLBACK：roomRepo.Create 已 INSERT 但被 undo log 撤销。
func TestRoomServiceIntegration_CreateRoom_FaultInjectionRoomMembersStep_RoomsAlsoRollback(t *testing.T) {
	syntheticErr := stderrors.New("synthetic room_members create failure")
	svc, sqlDB, _, cleanup := buildRoomServiceWithCustomRepos(t,
		nil, // user repo 用真实
		nil, // room repo 用真实
		func(real mysql.RoomMemberRepo) mysql.RoomMemberRepo {
			return &faultRoomMemberRepo{delegate: real, injectErr: syntheticErr}
		},
	)
	defer cleanup()

	const userID = uint64(1001)
	insertUser(t, sqlDB, userID, "uid-fault-rm", "user-fault-rm", "")

	// 取事务前快照
	roomCountBefore := fetchRoomCount(t, sqlDB)
	memberCountBefore := fetchRoomMemberCount(t, sqlDB)

	// CreateRoom：fn 内 roomRepo.Create 成功 → roomMemberRepo.Create 注入 error → tx 回滚
	_, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userID})
	if err == nil {
		t.Fatalf("CreateRoom returned nil error, want 1009 ErrServiceBusy")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d (ErrServiceBusy 1009 - 注入 error 走 generic 兜底)", ae.Code, apperror.ErrServiceBusy)
	}

	// 核心断言：DB 回到事务前快照（rooms 表新插入的行被 InnoDB undo log 撤销）
	if got := fetchRoomCount(t, sqlDB); got != roomCountBefore {
		t.Errorf("post-rollback rooms count = %d, want %d (rolled back)", got, roomCountBefore)
	}
	if got := fetchRoomMemberCount(t, sqlDB); got != memberCountBefore {
		t.Errorf("post-rollback room_members count = %d, want %d (none added)", got, memberCountBefore)
	}
	if got := fetchUserCurrentRoomID(t, sqlDB, userID); got != nil {
		t.Errorf("users.current_room_id = %d, want nil (transaction rolled back)", *got)
	}
}

// AC4 回滚 2: 加入房间 - users.UpdateCurrentRoomID 失败 → room_members.Create 也回滚
//
// fault wrapper 让任意 UpdateCurrentRoomID 调用都抛 error；fixture 用 raw SQL 直接造
// （A 已在房间 + A.current_room_id = roomID），让 B.join 的事务内 step 2d
// roomMemberRepo.Create(B) 已 INSERT 后，step 2e users.UpdateCurrentRoomID(B) 抛 error
// → tx ROLLBACK → InnoDB 撤销 INSERT，B 不在 room_members + B.current_room_id 仍 nil。
func TestRoomServiceIntegration_JoinRoom_FaultInjectionUsersUpdate_RoomMembersAlsoRollback(t *testing.T) {
	syntheticErr := stderrors.New("synthetic users.UpdateCurrentRoomID failure")
	svc, sqlDB, _, cleanup := buildRoomServiceWithCustomRepos(t,
		func(real mysql.UserRepo) mysql.UserRepo {
			return &faultUserRepoForJoin{delegate: real, injectErr: syntheticErr}
		},
		nil,
		nil,
	)
	defer cleanup()

	const userA = uint64(1001)
	const userB = uint64(1002)
	insertUser(t, sqlDB, userA, "uid-fault-join-a", "A", "")
	insertUser(t, sqlDB, userB, "uid-fault-join-b", "B", "")

	// **fixture 用 raw SQL 直接造**（不能用 svc.CreateRoom —— fault wrapper 让 create
	// 路径也抛 error）：rooms 1 行 + room_members 1 行（A is creator）+ A.current_room_id = roomID。
	const fixtureRoomID uint64 = 7001
	if _, err := sqlDB.Exec(`INSERT INTO rooms (id, creator_user_id, status, max_members) VALUES (?, ?, 1, 4)`,
		fixtureRoomID, userA); err != nil {
		t.Fatalf("fixture insert rooms: %v", err)
	}
	if _, err := sqlDB.Exec(`INSERT INTO room_members (room_id, user_id) VALUES (?, ?)`, fixtureRoomID, userA); err != nil {
		t.Fatalf("fixture insert room_members A: %v", err)
	}
	if _, err := sqlDB.Exec(`UPDATE users SET current_room_id = ? WHERE id = ?`, fixtureRoomID, userA); err != nil {
		t.Fatalf("fixture set A.current_room_id: %v", err)
	}

	// 取事务前快照
	memberCountBefore := fetchRoomMemberCount(t, sqlDB)

	// B.join：fault wrapper 让 UpdateCurrentRoomID 抛 error → tx ROLLBACK
	_, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userB, RoomID: fixtureRoomID})
	if err == nil {
		t.Fatalf("JoinRoom returned nil, want 1009 ErrServiceBusy")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d (1009 generic)", ae.Code, apperror.ErrServiceBusy)
	}

	// 核心断言：room_members 行数没变（B 那行被 rollback；A 仍在）
	if got := fetchRoomMemberCount(t, sqlDB); got != memberCountBefore {
		t.Errorf("post-rollback room_members count = %d, want %d", got, memberCountBefore)
	}
	assertCount(t, sqlDB, "room_members WHERE room_id=? AND user_id=?",
		[]any{fixtureRoomID, userB}, 0, "B's row rolled back")
	assertCount(t, sqlDB, "room_members WHERE room_id=? AND user_id=?",
		[]any{fixtureRoomID, userA}, 1, "A's fixture row preserved")

	// users.current_room_id (B) 仍 nil
	if got := fetchUserCurrentRoomID(t, sqlDB, userB); got != nil {
		t.Errorf("B.current_room_id = %d, want nil (rolled back)", *got)
	}

	// users.current_room_id (A) 仍 = fixtureRoomID
	if got := fetchUserCurrentRoomID(t, sqlDB, userA); got == nil || *got != fixtureRoomID {
		t.Errorf("A.current_room_id mismatch (fixture should be preserved)")
	}
}

// AC5 回滚 3: 退出房间 - users.UpdateCurrentRoomID(nil) 失败 → room_members 删除也回滚
//
// fault wrapper 仅在 leave 路径（roomID == nil）注入 error；create / join 路径透传，
// 让 fixture 阶段（A.create + B.join）正常完成，再让 A.leave 失败。
//
// 验证 DELETE room_members 已发生但被 rollback，A 仍在房间，rooms.status 仍 active。
func TestRoomServiceIntegration_LeaveRoom_FaultInjectionUsersUpdate_RoomMembersDeleteAlsoRollback(t *testing.T) {
	syntheticErr := stderrors.New("synthetic users.UpdateCurrentRoomID(nil) failure")
	svc, sqlDB, _, cleanup := buildRoomServiceWithCustomRepos(t,
		func(real mysql.UserRepo) mysql.UserRepo {
			return &faultUserRepoForLeave{delegate: real, injectErr: syntheticErr}
		},
		nil,
		nil,
	)
	defer cleanup()

	const userA = uint64(1001)
	const userB = uint64(1002)
	insertUser(t, sqlDB, userA, "uid-fault-leave-a", "A", "")
	insertUser(t, sqlDB, userB, "uid-fault-leave-b", "B", "")

	// fixture：A create + B join（fault wrapper 在 roomID != nil 路径透传 → fixture OK）
	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("fixture CreateRoom A: %v", err)
	}
	roomID := createOut.RoomID
	if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userB, RoomID: roomID}); err != nil {
		t.Fatalf("fixture JoinRoom B: %v", err)
	}

	// 取 leave 前快照
	memberCountBefore := fetchRoomMemberCount(t, sqlDB)
	if memberCountBefore != 2 {
		t.Fatalf("fixture memberCount = %d, want 2", memberCountBefore)
	}

	// A.leave：fault wrapper 让 UpdateCurrentRoomID(nil) 抛 error → tx ROLLBACK
	_, err = svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: userA, RoomID: roomID})
	if err == nil {
		t.Fatalf("LeaveRoom returned nil, want 1009 ErrServiceBusy")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("AppError.Code = %d, want %d (1009 generic)", ae.Code, apperror.ErrServiceBusy)
	}

	// 核心断言：room_members 删除也回滚（A 仍在房间）+ rooms.status 仍 active +
	// A.current_room_id 仍 = roomID
	if got := fetchRoomMemberCount(t, sqlDB); got != 2 {
		t.Errorf("post-rollback room_members count = %d, want 2 (A's DELETE rolled back)", got)
	}
	assertCount(t, sqlDB, "room_members WHERE room_id=? AND user_id=?",
		[]any{roomID, userA}, 1, "A's row preserved (DELETE rolled back)")
	assertCount(t, sqlDB, "room_members WHERE room_id=? AND user_id=?",
		[]any{roomID, userB}, 1, "B's row preserved")

	if got := fetchRoomStatus(t, sqlDB, roomID); got != 1 {
		t.Errorf("rooms.status = %d, want 1 active (no UPDATE happened)", got)
	}
	if got := fetchUserCurrentRoomID(t, sqlDB, userA); got == nil || *got != roomID {
		t.Errorf("A.current_room_id mismatch (UPDATE rolled back)")
	}
}

// AC6 并发 1: 5 个用户同时 join 同一房间（已有 3 人留 1 空位）→ 只 1 个成功
//
// barrier channel 让 5 goroutine 真同时起跑（避免 OS 调度顺序固定）；
// FOR UPDATE 锁串行化让其中 1 个先 INSERT（count 4 满员），其他 4 个看到 count >= 4 → 6002。
//
// 不断言具体哪个 user 成功（取决于 OS 调度）；只断言收敛（恰好 1 / 恰好 4）+ DB 行数 = 4。
func TestRoomServiceIntegration_JoinRoom_Concurrent5UsersIntoFullRoom_OnlyOneSucceeds(t *testing.T) {
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	// fixture: A + B + C 已在房间（3 人，剩 1 个空位）
	const userA = uint64(1001)
	const userB = uint64(1002)
	const userC = uint64(1003)
	insertUser(t, sqlDB, userA, "uid-c1-a", "A", "")
	insertUser(t, sqlDB, userB, "uid-c1-b", "B", "")
	insertUser(t, sqlDB, userC, "uid-c1-c", "C", "")

	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("fixture create: %v", err)
	}
	roomID := createOut.RoomID
	for _, u := range []uint64{userB, userC} {
		if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: u, RoomID: roomID}); err != nil {
			t.Fatalf("fixture JoinRoom %d: %v", u, err)
		}
	}

	// 5 个新 user：D/E/F/G/H 同时 join
	competitors := []uint64{1004, 1005, 1006, 1007, 1008}
	for i, u := range competitors {
		insertUser(t, sqlDB, u, "uid-c1-"+string(rune('D'+i)), string(rune('D'+i)), "")
	}

	var successCount, fullCount, otherErrCount atomic.Int32
	var wg sync.WaitGroup
	barrier := make(chan struct{})
	for _, u := range competitors {
		u := u
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-barrier // 5 goroutine 同时起跑
			_, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: u, RoomID: roomID})
			if err == nil {
				successCount.Add(1)
				return
			}
			ae, ok := apperror.As(err)
			if !ok {
				otherErrCount.Add(1)
				return
			}
			if ae.Code == apperror.ErrRoomFull {
				fullCount.Add(1)
			} else {
				otherErrCount.Add(1)
			}
		}()
	}
	close(barrier)
	wg.Wait()

	// 核心断言：恰好 1 成功 + 4 返 6002
	if got := successCount.Load(); got != 1 {
		t.Errorf("successCount = %d, want 1", got)
	}
	if got := fullCount.Load(); got != 4 {
		t.Errorf("fullCount (6002) = %d, want 4", got)
	}
	if got := otherErrCount.Load(); got != 0 {
		t.Errorf("otherErrCount = %d, want 0 (no unexpected errors)", got)
	}

	// DB 收尾断言：room_members.count = 4
	assertCount(t, sqlDB, "room_members WHERE room_id=?", []any{roomID}, 4, "after concurrent join")
	if got := fetchRoomStatus(t, sqlDB, roomID); got != 1 {
		t.Errorf("rooms.status = %d, want 1 active", got)
	}
}

// AC7 并发 2: 100 个不同用户同时 create 100 个不同房间 → 全部成功
//
// 与 4.7 _Concurrent100DifferentGuestUIDs_NoCrossData 同模式：不同 key 空间无冲突 →
// 验证事务彼此独立 + 没有 race 把 user 串到别人房间。
func TestRoomServiceIntegration_CreateRoom_Concurrent100DifferentUsers_100RoomsCreated(t *testing.T) {
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const N = 100
	startUID := uint64(2001)
	for i := 0; i < N; i++ {
		uid := startUID + uint64(i)
		insertUser(t, sqlDB, uid, "uid-c2-"+strconv.FormatUint(uid, 10), "u"+strconv.FormatUint(uid, 10), "")
	}

	type result struct {
		userID uint64
		roomID uint64
		err    error
	}
	results := make(chan result, N)
	var wg sync.WaitGroup
	barrier := make(chan struct{})
	for i := 0; i < N; i++ {
		uid := startUID + uint64(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-barrier
			out, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: uid})
			if err != nil {
				results <- result{userID: uid, err: err}
				return
			}
			results <- result{userID: uid, roomID: out.RoomID}
		}()
	}
	close(barrier)
	wg.Wait()
	close(results)

	// 收集结果：100 个全部成功
	roomByUser := make(map[uint64]uint64, N)
	successCount := 0
	for r := range results {
		if r.err != nil {
			t.Errorf("user=%d CreateRoom err: %v", r.userID, r.err)
			continue
		}
		successCount++
		roomByUser[r.userID] = r.roomID
	}
	if successCount != N {
		t.Errorf("successCount = %d, want %d", successCount, N)
	}

	// DB 收尾断言：rooms = 100 + room_members = 100
	if got := fetchRoomCount(t, sqlDB); got != int64(N) {
		t.Errorf("rooms count = %d, want %d", got, N)
	}
	if got := fetchRoomMemberCount(t, sqlDB); got != int64(N) {
		t.Errorf("room_members count = %d, want %d", got, N)
	}

	// 串数据校验：每个 user 的 current_room_id 必须等于该 user 创建的 rooms.id
	for i := 0; i < N; i++ {
		uid := startUID + uint64(i)
		gotRoomID := fetchUserCurrentRoomID(t, sqlDB, uid)
		if gotRoomID == nil {
			t.Errorf("user=%d current_room_id = nil, want %d", uid, roomByUser[uid])
			continue
		}
		if *gotRoomID != roomByUser[uid] {
			t.Errorf("user=%d current_room_id = %d, want %d (cross-data leak)", uid, *gotRoomID, roomByUser[uid])
		}
	}
}

// AC10 边界 2: A 在房间 X 调 X/join（加入自己已在的房间）→ 6003
//
// service 层步骤 1 预检 users.current_room_id != nil 即返 6003，不区分目标 roomID 是否同 X。
func TestRoomServiceIntegration_JoinRoom_UserAlreadyInSameRoom_Returns6003(t *testing.T) {
	svc, sqlDB, _, cleanup := buildRoomServiceIntegration(t)
	defer cleanup()

	const userA = uint64(1001)
	insertUser(t, sqlDB, userA, "uid-bd2-a", "A", "")

	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("CreateRoom A: %v", err)
	}
	roomID := createOut.RoomID

	// A 试图加入自己已在的房间 X
	_, err = svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userA, RoomID: roomID})
	if err == nil {
		t.Fatalf("JoinRoom A→X returned nil, want 6003")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err is not *AppError: %v", err)
	}
	if ae.Code != apperror.ErrUserAlreadyInRoom {
		t.Errorf("AppError.Code = %d, want %d (ErrUserAlreadyInRoom 6003 - 预检 current_room_id != nil)", ae.Code, apperror.ErrUserAlreadyInRoom)
	}

	// DB 状态：room_members count 仍 = 1（A creator）+ rooms.status 仍 active +
	// A.current_room_id 仍 = roomID（事务未开，无变化）
	assertCount(t, sqlDB, "room_members WHERE room_id=?", []any{roomID}, 1, "no change after self-join attempt")
	if got := fetchRoomStatus(t, sqlDB, roomID); got != 1 {
		t.Errorf("rooms.status = %d, want 1", got)
	}
	if got := fetchUserCurrentRoomID(t, sqlDB, userA); got == nil || *got != roomID {
		t.Errorf("A.current_room_id changed; want %d", roomID)
	}
}

// AC8 WS 联动: A + B 在房间 → A 持有真实 SessionManager Session → B leave →
// SessionManager 内 A 的 Session 仍存在 + capture broadcastFn 收到 member.left wire +
// payload.userId == B
//
// 简化路径（按 AC8 钦定退化）：用 buildRoomServiceIntegrationWithCapture 的 capture
// broadcastFn + 真实 SessionManager；通过 startGatewayForWSIntegration helper 起一个
// 真实 WS gateway → A 真 Dial → A 的 Session 进 SessionManager → 验证 B leave 后
// 1) ListSessionsByRoomID 仍含 A（B 未持 Session）；2) capture 收到 1 次 member.left。
//
// 关键差异 vs 11.8 case 14/15/16：本 case 让 A 真持有 Session 进 SessionManager（不是
// 0 Session 的 no-op 路径），验证 leave 路径能正确**保留**其他成员 Session（同时验证
// 11.8 的 close 4007 不会误中 A 的 Session）。
func TestRoomServiceIntegration_LeaveRoom_RealWSSession_BroadcastsMemberLeftToOtherMembers(t *testing.T) {
	svc, sqlDB, sessionMgr, capture, wg, cleanup := buildRoomServiceIntegrationWithCapture(t)
	defer cleanup()

	const userA = uint64(1001)
	const userB = uint64(1002)
	insertUser(t, sqlDB, userA, "uid-ws-a", "A", "")
	insertUser(t, sqlDB, userB, "uid-ws-b", "B", "")

	// fixture: A create + B join
	createOut, err := svc.CreateRoom(context.Background(), service.CreateRoomInput{UserID: userA})
	if err != nil {
		t.Fatalf("createRoom A: %v", err)
	}
	roomID := createOut.RoomID
	if _, err := svc.JoinRoom(context.Background(), service.JoinRoomInput{UserID: userB, RoomID: roomID}); err != nil {
		t.Fatalf("JoinRoom B: %v", err)
	}
	// 等 join post-commit goroutine 完成，再清 capture
	wg.Wait()
	capture.mu.Lock()
	capture.calls = nil
	capture.mu.Unlock()

	// **关键步骤**：让 A 真持有 SessionManager Session
	//
	// **简化路径**（AC8 钦定 fallback）：sessionMgr 是真实 wsapp.NewSessionManager()
	// 实例（来自 buildRoomServiceIntegrationWithCapture），但 Session struct 构造私有 +
	// Register 接受 *Session —— 跨 service_test 包无法直接构造 Session。改用真实 WS
	// 拨号路径：起 gateway httptest server → A 用真 WS Dial → gateway.Handle 完成
	// 握手 → SessionManager.Register A 的 Session。
	wsURL, signer, gwSessionMgr, ts := startWSGatewayForLeaveCase(t, sqlDB, sessionMgr)
	defer ts.Close()
	_ = gwSessionMgr // gwSessionMgr 与 sessionMgr 同一实例（helper 会用 caller 传入的）

	tokenA, err := signer.Sign(userA, 3600)
	if err != nil {
		t.Fatalf("Sign tokenA: %v", err)
	}
	wsConn, _, err := websocket.DefaultDialer.Dial(
		wsURL+"/ws/rooms/"+strconv.FormatUint(roomID, 10)+"?token="+tokenA, nil)
	if err != nil {
		t.Fatalf("Dial WS: %v", err)
	}
	defer wsConn.Close()

	// 接收 A 的 first message = room.snapshot（验证握手成功 + Session 已 Register）
	_ = wsConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, snapshot, err := wsConn.ReadMessage()
	if err != nil {
		t.Fatalf("ReadMessage snapshot: %v", err)
	}
	var snapshotEnv integrationEnvelope
	if err := json.Unmarshal(snapshot, &snapshotEnv); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if snapshotEnv.Type != "room.snapshot" {
		t.Errorf("first msg type = %q, want room.snapshot", snapshotEnv.Type)
	}

	// 验证 SessionManager 索引：A 的 Session 已 Register
	sessions := sessionMgr.ListSessionsByRoomID(context.Background(), roomID)
	if len(sessions) != 1 {
		t.Fatalf("after A WS dial: ListSessionsByRoomID len = %d, want 1", len(sessions))
	}
	if sessions[0].UserID() != userA {
		t.Errorf("A's session userID = %d, want %d", sessions[0].UserID(), userA)
	}

	// **关键路径**：B leave —— svc.LeaveRoom 触发 post-commit broadcastMemberLeft（capture）+
	// SessionManager 内 B 的 Session 不存在（B 未拨号），Unregister 走 no-op；
	// A 的 Session 必须保留（不被 close 4007 误中）
	if _, err := svc.LeaveRoom(context.Background(), service.LeaveRoomInput{UserID: userB, RoomID: roomID}); err != nil {
		t.Fatalf("LeaveRoom B: %v", err)
	}
	// 等 post-commit goroutine 完成
	wg.Wait()

	// 核心断言 1: A 的 Session 仍在 SessionManager
	sessionsAfter := sessionMgr.ListSessionsByRoomID(context.Background(), roomID)
	if len(sessionsAfter) != 1 {
		t.Errorf("after B leave: ListSessionsByRoomID len = %d, want 1 (A's session preserved)", len(sessionsAfter))
	} else if sessionsAfter[0].UserID() != userA {
		t.Errorf("preserved session userID = %d, want %d", sessionsAfter[0].UserID(), userA)
	}

	// 核心断言 2: capture 收到 1 次 member.left wire + payload.userId = B
	if got := capture.callCount(); got != 1 {
		t.Fatalf("broadcast call count = %d, want 1 (member.left for B)", got)
	}
	call := capture.calls[0]
	if call.roomID != roomID {
		t.Errorf("call.roomID = %d, want %d", call.roomID, roomID)
	}
	if call.excludeUserID == nil || *call.excludeUserID != userB {
		var got string
		if call.excludeUserID == nil {
			got = "nil"
		} else {
			got = strconv.FormatUint(*call.excludeUserID, 10)
		}
		t.Errorf("call.excludeUserID = %s, want %d (broadcastExceptFn excludes leaver)", got, userB)
	}

	var leftEnv integrationEnvelope
	if err := json.Unmarshal(call.msg, &leftEnv); err != nil {
		t.Fatalf("unmarshal leftEnv: %v", err)
	}
	if leftEnv.Type != "member.left" {
		t.Errorf("leftEnv.type = %q, want member.left", leftEnv.Type)
	}
	var leftPayload struct {
		UserID string `json:"userId"`
	}
	if err := json.Unmarshal(leftEnv.Payload, &leftPayload); err != nil {
		t.Fatalf("unmarshal leftPayload: %v", err)
	}
	if leftPayload.UserID != strconv.FormatUint(userB, 10) {
		t.Errorf("leftPayload.userId = %q, want %q", leftPayload.UserID, strconv.FormatUint(userB, 10))
	}
}

// startWSGatewayForLeaveCase 装配最小 WS gateway httptest server，让 AC8 case 能真
// Dial WS 把 A 的 Session 注入 SessionManager。
//
// **设计约束**：
//   - 复用 caller 传入的 sessionMgr（与 svc 内部用同一实例 → broadcast 路径打通）
//   - 复用 caller 的 sqlDB（用 gormmysql.New(Conn: sqlDB) 包成 GORM 实例）
//   - 不抽到 cross-package testutil（与 ws_integration_test.go 的 startGatewayWithRealMySQL
//     模式同源 —— 跨 package helper 不复用是 4.7 / 11.3 ~ 11.8 已建立的规范）
//
// 返 (wsURL, signer, sessionMgr, ts)。
func startWSGatewayForLeaveCase(t *testing.T, sqlDB *sql.DB, sessionMgr wsapp.SessionManager) (string, *auth.Signer, wsapp.SessionManager, *httptest.Server) {
	t.Helper()

	signer, err := auth.New("integration-test-secret-32-bytes-min", 3600)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}

	// 复用 caller 的 *sql.DB 包成 GORM（与 mysql_test.go 的 sqlmock + GORM
	// New(Conn:) 模式同源；不需要单独 dsn）。
	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: sqlDB}), &gorm.Config{})
	if err != nil {
		t.Fatalf("gorm.Open with existing sqlDB: %v", err)
	}

	repo := mysql.NewRoomMemberRepo(gormDB)
	cfg := config.WSConfig{
		HeartbeatTimeoutSec: 60,
		MaxMessageSizeBytes: 16384,
		WriteTimeoutSec:     5,
	}
	builder := wsapp.NewPlaceholderSnapshotBuilder(repo)
	gateway := wsapp.NewGateway(signer, sessionMgr, repo, cfg, "test", builder)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/ws/rooms/:roomId", gateway.Handle)
	ts := httptest.NewServer(r)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http")
	return wsURL, signer, sessionMgr, ts
}
