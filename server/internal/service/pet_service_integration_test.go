//go:build integration
// +build integration

// Story 14.2 集成测试：用 dockertest 起真实 mysql:8.0 容器跑 3 个 case：
//
//   1. HappyState3_PersistsToDB：
//      创建 user + 默认 pet（current_state=1）→ SyncCurrentState({state: 3}) →
//      DB pets.current_state = 3，updated_at 已变（> 初始 updated_at）。
//   2. SubsequentState1_ReverseSwitch：
//      接 case 1 后调 SyncCurrentState({state: 1}) → DB pets.current_state = 1
//      （幂等性 + 反向切换）。
//   3. PetLessAccount_Noop_NoDBChange：
//      DELETE FROM pets WHERE user_id=? → SyncCurrentState({state: 2}) →
//      HTTP 200 (output 回显 state=2) + DB pets 仍 0 行（noop 不重新创建 pet）+
//      err == nil（**断言不为 apperror.ErrResourceNotFound** —— r7 锁定）。
//
// 复用 4.6 / 7.3 / 11.3 的 startMySQL / runMigrations helper（来自同 service 包）。
//
// **手工 INSERT** user + pet（不调 auth_service.GuestLogin）—— 解耦 pet_service
// 测试与 auth_service。
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑这些；
// 只在 `bash scripts/build.sh --integration`（即 `go test -tags=integration`）触发。

package service_test

import (
	"context"
	"database/sql"
	"encoding/json"
	stderrors "errors"
	"sync"
	"testing"
	"time"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// buildPetServiceIntegration: 起容器 → migrate → 装配 svc + 返清理 closure。
//
// sessionMgr / broadcastFn 全部传 nil（与 router.go HTTP-only fixture wire 一致；
// 本 story 不广播 —— 14.4 才接管广播实装）。
func buildPetServiceIntegration(t *testing.T) (svc service.PetService, sqlDB *sql.DB, cleanup func()) {
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

	petRepo := mysql.NewPetRepo(gormDB)
	userRepo := mysql.NewUserRepo(gormDB)
	svc = service.NewPetService(petRepo, userRepo, nil, nil)

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

// fixturePetIntegrationCreateUserPet 直接 INSERT user + 默认 pet（current_state=1），
// 返回 (userID, petID)。**不**调 auth_service.GuestLogin（解耦 pet / auth 测试）。
//
// **仅 INSERT pets + users**（不创建 user_step_accounts / user_chests）—— 本接口
// 与步数账户 / 宝箱无业务依赖；缺哪些表 row 不影响 SyncCurrentState 路径。
func fixturePetIntegrationCreateUserPet(t *testing.T, sqlDB *sql.DB) (userID, petID uint64) {
	t.Helper()
	// INSERT users
	res, err := sqlDB.Exec(`INSERT INTO users (guest_uid, nickname, avatar_url, status) VALUES (?, ?, ?, ?)`,
		"uid-pet-integration", "", "", 1)
	if err != nil {
		t.Fatalf("INSERT users: %v", err)
	}
	uid, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("users LastInsertId: %v", err)
	}
	userID = uint64(uid)

	// INSERT pets（默认 pet：is_default=1, current_state=1）
	res, err = sqlDB.Exec(`INSERT INTO pets (user_id, pet_type, name, current_state, is_default) VALUES (?, ?, ?, ?, ?)`,
		userID, 1, "默认小猫", 1, 1)
	if err != nil {
		t.Fatalf("INSERT pets: %v", err)
	}
	pid, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("pets LastInsertId: %v", err)
	}
	petID = uint64(pid)
	return
}

// fetchPetCurrentStateAndUpdatedAt 直接 SELECT 验当前 pet.current_state + updated_at。
func fetchPetCurrentStateAndUpdatedAt(t *testing.T, sqlDB *sql.DB, userID uint64) (currentState int8, updatedAt time.Time) {
	t.Helper()
	row := sqlDB.QueryRow(`SELECT current_state, updated_at FROM pets WHERE user_id = ? AND is_default = 1`, userID)
	if err := row.Scan(&currentState, &updatedAt); err != nil {
		t.Fatalf("fetchPetCurrentStateAndUpdatedAt: %v", err)
	}
	return
}

// countPetsByUserID 查指定 user_id 的 pets 行数（pet-less 场景 = 0 行）。
func countPetsByUserID(t *testing.T, sqlDB *sql.DB, userID uint64) int64 {
	t.Helper()
	var n int64
	row := sqlDB.QueryRow(`SELECT COUNT(*) FROM pets WHERE user_id = ?`, userID)
	if err := row.Scan(&n); err != nil {
		t.Fatalf("countPetsByUserID: %v", err)
	}
	return n
}

// case 1 + case 2: happy state=3 + 反向切换 state=1
// 验证 SyncCurrentState 写入 pets.current_state；updated_at 自动更新；幂等反向切换正确。
func TestPetService_SyncCurrentState_Integration_HappyAndReverseSwitch(t *testing.T) {
	svc, sqlDB, cleanup := buildPetServiceIntegration(t)
	defer cleanup()

	userID, _ := fixturePetIntegrationCreateUserPet(t, sqlDB)

	// 取初始 updated_at（INSERT 时由 DEFAULT CURRENT_TIMESTAMP(3) 写入）
	initialState, initialUpdatedAt := fetchPetCurrentStateAndUpdatedAt(t, sqlDB, userID)
	if initialState != 1 {
		t.Fatalf("initial current_state = %d, want 1", initialState)
	}

	// 等 1ms 确保 updated_at 单调递增（CURRENT_TIMESTAMP(3) 毫秒精度）
	time.Sleep(2 * time.Millisecond)

	// === case 1: SyncCurrentState state=3 ===
	out, err := svc.SyncCurrentState(context.Background(), service.SyncCurrentStateInput{
		UserID: userID,
		State:  3,
	})
	if err != nil {
		t.Fatalf("SyncCurrentState state=3: %v", err)
	}
	if out == nil || out.State != 3 {
		t.Errorf("out = %+v, want &{State:3}", out)
	}

	got3, updatedAfter3 := fetchPetCurrentStateAndUpdatedAt(t, sqlDB, userID)
	if got3 != 3 {
		t.Errorf("after state=3: DB pets.current_state = %d, want 3", got3)
	}
	if !updatedAfter3.After(initialUpdatedAt) {
		t.Errorf("after state=3: updated_at = %v, want > initial %v", updatedAfter3, initialUpdatedAt)
	}

	// === case 2: SyncCurrentState state=1（反向切换 + 幂等性）===
	time.Sleep(2 * time.Millisecond)
	out, err = svc.SyncCurrentState(context.Background(), service.SyncCurrentStateInput{
		UserID: userID,
		State:  1,
	})
	if err != nil {
		t.Fatalf("SyncCurrentState state=1: %v", err)
	}
	if out == nil || out.State != 1 {
		t.Errorf("out = %+v, want &{State:1}", out)
	}

	got1, updatedAfter1 := fetchPetCurrentStateAndUpdatedAt(t, sqlDB, userID)
	if got1 != 1 {
		t.Errorf("after state=1: DB pets.current_state = %d, want 1", got1)
	}
	if !updatedAfter1.After(updatedAfter3) {
		t.Errorf("after state=1: updated_at = %v, want > after_state_3 %v", updatedAfter1, updatedAfter3)
	}
}

// case 3: pet-less 账号路径（DELETE pet 行 → SyncCurrentState → HTTP 200 + DB 不变）
//
// 验证 r7 lessons 锁定的 noop 路径：
//   - SyncCurrentState 返 (output, nil) 而非 apperror.ErrResourceNotFound (1003)
//   - DB pets 行仍为 0（service **不**重新创建 pet）
//   - output.State 回显入参（ack-only 信号）
func TestPetService_SyncCurrentState_Integration_PetLess_Noop(t *testing.T) {
	svc, sqlDB, cleanup := buildPetServiceIntegration(t)
	defer cleanup()

	userID, _ := fixturePetIntegrationCreateUserPet(t, sqlDB)

	// 模拟 pet-less 账号：手动 DELETE pet 行
	if _, err := sqlDB.Exec(`DELETE FROM pets WHERE user_id = ?`, userID); err != nil {
		t.Fatalf("DELETE pets: %v", err)
	}
	if c := countPetsByUserID(t, sqlDB, userID); c != 0 {
		t.Fatalf("after DELETE: pets count = %d, want 0", c)
	}

	// SyncCurrentState pet-less 路径
	out, err := svc.SyncCurrentState(context.Background(), service.SyncCurrentStateInput{
		UserID: userID,
		State:  2,
	})

	// r7 锁定：pet-less 走 server-acknowledged noop 路径 → nil error
	if err != nil {
		t.Fatalf("SyncCurrentState pet-less: expected nil err, got %v", err)
	}
	// **断言不为 apperror.ErrResourceNotFound (1003)** —— r7 锁定
	if got := apperror.Code(err); got != 0 {
		t.Errorf("apperror.Code(err) = %d, want 0 (pet-less 不触发 1003 / ErrResourceNotFound)", got)
	}
	// errors.Is 路径同样不应触发 mysql.ErrPetNotFound 透传给上层
	if stderrors.Is(err, mysql.ErrPetNotFound) {
		t.Errorf("err.Is(ErrPetNotFound) = true; service 应吸收 pet-less 走 noop")
	}
	// 回显入参 state（server-acknowledged ack 信号）
	if out == nil || out.State != 2 {
		t.Errorf("output = %+v, want &{State:2} (回显入参)", out)
	}

	// DB pets 仍 0 行（noop 不重新创建 pet）
	if c := countPetsByUserID(t, sqlDB, userID); c != 0 {
		t.Errorf("after pet-less noop: pets count = %d, want 0 (service 不应重新创建 pet)", c)
	}
}

// ============================================================
// case 4: Story 14.4 — ws-end-to-end broadcastFn 被触发（用户在房间）
//
// 场景：建 user A + 默认 pet（current_state=1）+ room X + insert room_members 行 +
// UPDATE users.current_room_id = X → 注入 captured-call mockBroadcastFn 构造
// PetService → 调 SyncCurrentState({UserID: A, State: 2}) → 等 broadcast goroutine
// 完成 → 验证：
//   - SyncCurrentStateOutput.State == 2
//   - DB pets.current_state = 2（既有 14.2 集成测试同模式断言）
//   - broadcastFn 调用次数 1 + roomID == X
//   - unmarshal msg bytes 后 envelope.type="pet.state.changed" +
//     payload.userId="<A.id>" + payload.petId="<A.pet.id>" + payload.currentState=2 +
//     ts != 0
//
// **fixture 区别值**（14-3 r1 lesson 钦定）：state=2（不用 default 1）让 wire
// 上的 payload.currentState=2 能与 hardcoded placeholder 区分开。
// ============================================================
func TestPetService_SyncCurrentState_Integration_Story144_BroadcastFnTriggered(t *testing.T) {
	dsn, dockerCleanup := startMySQL(t)
	defer dockerCleanup()
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
		t.Fatalf("db.Open: %v", err)
	}
	rawDB, err := gormDB.DB()
	if err != nil {
		t.Fatalf("gormDB.DB(): %v", err)
	}
	defer rawDB.Close()

	// fixture: user A + pet + room X + room_members + users.current_room_id = X
	userID, petID := fixturePetIntegrationCreateUserPet(t, rawDB)

	// 直接 INSERT rooms（max_members=4, status=1=active, creator=userID）
	res, err := rawDB.Exec(`INSERT INTO rooms (creator_user_id, status, max_members) VALUES (?, 1, 4)`, userID)
	if err != nil {
		t.Fatalf("INSERT rooms: %v", err)
	}
	rid, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("rooms LastInsertId: %v", err)
	}
	roomID := uint64(rid)

	// INSERT room_members + UPDATE users.current_room_id
	if _, err := rawDB.Exec(`INSERT INTO room_members (room_id, user_id) VALUES (?, ?)`, roomID, userID); err != nil {
		t.Fatalf("INSERT room_members: %v", err)
	}
	if _, err := rawDB.Exec(`UPDATE users SET current_room_id = ? WHERE id = ?`, roomID, userID); err != nil {
		t.Fatalf("UPDATE users.current_room_id: %v", err)
	}

	// 构造 captured-call mockBroadcastFn（与 case 7 单测模式同）
	wg := &sync.WaitGroup{}
	wg.Add(1)
	bcast := &broadcastRecorder{wg: wg, returnSent: 1}

	petRepo := mysql.NewPetRepo(gormDB)
	userRepo := mysql.NewUserRepo(gormDB)
	svc := service.NewPetService(petRepo, userRepo, nil, bcast.fn)

	// 调 SyncCurrentState({UserID: A, State: 2})
	out, err := svc.SyncCurrentState(context.Background(), service.SyncCurrentStateInput{
		UserID: userID,
		State:  2,
	})
	if err != nil {
		t.Fatalf("SyncCurrentState: %v", err)
	}
	if out == nil || out.State != 2 {
		t.Errorf("output = %+v, want &{State:2}", out)
	}

	// 等 broadcast goroutine 跑完
	waitWithTimeout(t, wg, 5*time.Second, "broadcast goroutine did not complete")

	// DB 校验：pets.current_state 已写为 2
	got, _ := fetchPetCurrentStateAndUpdatedAt(t, rawDB, userID)
	if got != 2 {
		t.Errorf("DB pets.current_state = %d, want 2", got)
	}

	// broadcastFn 调用次数 + roomID
	if c := bcast.callCount(); c != 1 {
		t.Fatalf("broadcastFn callCount = %d, want 1", c)
	}
	last, _ := bcast.lastCall()
	if last.roomID != roomID {
		t.Errorf("broadcast roomID = %d, want %d", last.roomID, roomID)
	}

	// envelope + payload 完整断言
	var env petEnvelopeForTest
	if err := json.Unmarshal(last.msg, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v; msg=%s", err, string(last.msg))
	}
	if env.Type != "pet.state.changed" {
		t.Errorf("envelope.Type = %q, want \"pet.state.changed\"", env.Type)
	}
	if env.RequestID != "" {
		t.Errorf("envelope.RequestID = %q, want \"\"", env.RequestID)
	}
	if env.Ts <= 0 {
		t.Errorf("envelope.Ts = %d, want > 0", env.Ts)
	}

	var p petStateChangedPayloadForTest
	if err := json.Unmarshal(env.Payload, &p); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	wantUserID := uint64ToString(userID)
	wantPetID := uint64ToString(petID)
	if p.UserID != wantUserID {
		t.Errorf("payload.userId = %q, want %q", p.UserID, wantUserID)
	}
	if p.PetID != wantPetID {
		t.Errorf("payload.petId = %q, want %q", p.PetID, wantPetID)
	}
	if p.CurrentState != 2 {
		t.Errorf("payload.currentState = %d, want 2", p.CurrentState)
	}
}

// uint64ToString 工具：避免在集成测试里 import "strconv" 多写一遍
func uint64ToString(v uint64) string {
	// 与 service 层 strconv.FormatUint 同形态；不引入新 import
	const digits = "0123456789"
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = digits[v%10]
		v /= 10
	}
	return string(buf[i:])
}
