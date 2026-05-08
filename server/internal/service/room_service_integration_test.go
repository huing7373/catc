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

