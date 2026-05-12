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
	stderrors "errors"
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
