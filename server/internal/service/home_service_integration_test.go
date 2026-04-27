//go:build integration
// +build integration

// Story 4.8 集成测试：用 dockertest 起真实 mysql:8.0 容器跑 3 条 happy 链路 case：
//   1. 全表数据齐全 → svc.LoadHome 返完整 HomeOutput（user/pet/step/chest 字段对齐）
//   2. unlock_at 已过 → service 动态判定 chest.Status=2 / RemainingSeconds=0
//   3. 不 INSERT pets → svc.LoadHome 返 err=nil + Pet=nil（V1 §5.1 钦定可空）
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑这些；
// 只在 `bash scripts/build.sh --integration`（即 `go test -tags=integration`）触发。
//
// 复用 4.6 auth_service_integration_test.go 的 startMySQL / migrationsPath /
// runMigrations helper（同 service_test package 直接调，与 4.2 / 4.3 同模式 ——
// 故意复制不抽包，避免范围扩散）。
//
// **手工 INSERT 测试数据**（**不**调 4.6 auth_service.GuestLogin） —— 解耦 home_service
// 测试与 auth_service：home_service 集成测试只验 home 链路（4 repo 串行 + chest 动态
// 判定），调 auth_service 会引入 4.6 实装变更敏感性。

package service_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// buildHomeServiceIntegration: 起容器 → migrate → 装配 svc + 返清理 closure。
//
// 与 buildAuthService 同模式，但**不**起 txMgr / signer（home_service 不依赖）。
func buildHomeServiceIntegration(t *testing.T) (svc service.HomeService, sqlDB *sql.DB, cleanup func()) {
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

	userRepo := mysql.NewUserRepo(gormDB)
	petRepo := mysql.NewPetRepo(gormDB)
	stepRepo := mysql.NewStepAccountRepo(gormDB)
	chestRepo := mysql.NewChestRepo(gormDB)

	svc = service.NewHomeService(userRepo, petRepo, stepRepo, chestRepo)

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

// insertUser / insertPet / insertStepAccount / insertChest 是 INSERT 测试数据的便捷封装。
// 用 sqlDB.Exec 直接 SQL 而非 GORM Create —— 与 4.6 集成测试同模式，避免 GORM
// 回填字段意外覆盖测试数据。
func insertUser(t *testing.T, sqlDB *sql.DB, id uint64, guestUID, nickname, avatar string) {
	t.Helper()
	_, err := sqlDB.Exec(
		`INSERT INTO users (id, guest_uid, nickname, avatar_url, status) VALUES (?, ?, ?, ?, ?)`,
		id, guestUID, nickname, avatar, 1,
	)
	if err != nil {
		t.Fatalf("insert user: %v", err)
	}
}

func insertPet(t *testing.T, sqlDB *sql.DB, id, userID uint64, petType int, name string, currentState, isDefault int) {
	t.Helper()
	_, err := sqlDB.Exec(
		`INSERT INTO pets (id, user_id, pet_type, name, current_state, is_default) VALUES (?, ?, ?, ?, ?, ?)`,
		id, userID, petType, name, currentState, isDefault,
	)
	if err != nil {
		t.Fatalf("insert pet: %v", err)
	}
}

func insertStepAccount(t *testing.T, sqlDB *sql.DB, userID uint64, total, available, consumed uint64) {
	t.Helper()
	_, err := sqlDB.Exec(
		`INSERT INTO user_step_accounts (user_id, total_steps, available_steps, consumed_steps, version) VALUES (?, ?, ?, ?, ?)`,
		userID, total, available, consumed, 0,
	)
	if err != nil {
		t.Fatalf("insert step_account: %v", err)
	}
}

func insertChest(t *testing.T, sqlDB *sql.DB, id, userID uint64, status int, unlockAt time.Time, openCostSteps uint32) {
	t.Helper()
	_, err := sqlDB.Exec(
		`INSERT INTO user_chests (id, user_id, status, unlock_at, open_cost_steps, version) VALUES (?, ?, ?, ?, ?, ?)`,
		id, userID, status, unlockAt, openCostSteps, 0,
	)
	if err != nil {
		t.Fatalf("insert chest: %v", err)
	}
}

// ============================================================
// AC8.1: 全表数据齐全 → svc.LoadHome 返完整 HomeOutput
// ============================================================
func TestHomeService_LoadHome_HappyPath(t *testing.T) {
	svc, sqlDB, cleanup := buildHomeServiceIntegration(t)
	defer cleanup()

	unlockAt := time.Now().UTC().Add(10 * time.Minute)

	insertUser(t, sqlDB, 1, "uid-home-1", "用户1", "")
	insertPet(t, sqlDB, 2001, 1, 1, "默认小猫", 1, 1)
	insertStepAccount(t, sqlDB, 1, 0, 0, 0)
	insertChest(t, sqlDB, 5001, 1, 1, unlockAt, 1000)

	out, err := svc.LoadHome(context.Background(), 1)
	if err != nil {
		t.Fatalf("LoadHome: %v", err)
	}

	// user
	if out.User.ID != 1 {
		t.Errorf("User.ID = %d, want 1", out.User.ID)
	}
	if out.User.Nickname != "用户1" {
		t.Errorf("User.Nickname = %q, want 用户1", out.User.Nickname)
	}

	// pet
	if out.Pet == nil {
		t.Fatal("Pet should not be nil")
	}
	if out.Pet.ID != 2001 {
		t.Errorf("Pet.ID = %d, want 2001", out.Pet.ID)
	}
	if out.Pet.PetType != 1 {
		t.Errorf("Pet.PetType = %d, want 1", out.Pet.PetType)
	}
	if out.Pet.Name != "默认小猫" {
		t.Errorf("Pet.Name = %q, want 默认小猫", out.Pet.Name)
	}
	if out.Pet.CurrentState != 1 {
		t.Errorf("Pet.CurrentState = %d, want 1", out.Pet.CurrentState)
	}

	// step
	if out.StepAccount.TotalSteps != 0 {
		t.Errorf("StepAccount.TotalSteps = %d, want 0", out.StepAccount.TotalSteps)
	}
	if out.StepAccount.AvailableSteps != 0 {
		t.Errorf("StepAccount.AvailableSteps = %d, want 0", out.StepAccount.AvailableSteps)
	}
	if out.StepAccount.ConsumedSteps != 0 {
		t.Errorf("StepAccount.ConsumedSteps = %d, want 0", out.StepAccount.ConsumedSteps)
	}

	// chest（动态字段）
	if out.Chest.ID != 5001 {
		t.Errorf("Chest.ID = %d, want 5001", out.Chest.ID)
	}
	if out.Chest.Status != 1 {
		t.Errorf("Chest.Status = %d, want 1 (counting)", out.Chest.Status)
	}
	if out.Chest.OpenCostSteps != 1000 {
		t.Errorf("Chest.OpenCostSteps = %d, want 1000", out.Chest.OpenCostSteps)
	}
	// remainingSeconds ∈ [598, 602]：dockertest 容器 + GORM round-trip 可能 1-2s 延迟
	if out.Chest.RemainingSeconds < 598 || out.Chest.RemainingSeconds > 602 {
		t.Errorf("Chest.RemainingSeconds = %d, want ∈[598, 602]", out.Chest.RemainingSeconds)
	}
	// UnlockAt 必须是 UTC（service 强制 .UTC()）
	if out.Chest.UnlockAt.Location().String() != "UTC" {
		t.Errorf("Chest.UnlockAt location = %q, want UTC", out.Chest.UnlockAt.Location().String())
	}
}

// ============================================================
// AC8.2: chest.unlock_at 已过 → service 动态判定 Status=2 / RemainingSeconds=0
// ============================================================
func TestHomeService_LoadHome_ChestUnlocked_StatusIs2(t *testing.T) {
	svc, sqlDB, cleanup := buildHomeServiceIntegration(t)
	defer cleanup()

	pastUnlock := time.Now().UTC().Add(-1 * time.Minute)

	insertUser(t, sqlDB, 1, "uid-home-2", "用户1", "")
	insertPet(t, sqlDB, 2001, 1, 1, "默认小猫", 1, 1)
	insertStepAccount(t, sqlDB, 1, 0, 0, 0)
	// DB 原值 status=1 (counting)，但 unlock_at 已过
	insertChest(t, sqlDB, 5001, 1, 1, pastUnlock, 1000)

	out, err := svc.LoadHome(context.Background(), 1)
	if err != nil {
		t.Fatalf("LoadHome: %v", err)
	}

	// 关键：DB 原值 Status=1 但 service 算出 Status=2 (unlockable)
	if out.Chest.Status != 2 {
		t.Errorf("Chest.Status = %d, want 2 (unlockable —— service 必须动态判定)", out.Chest.Status)
	}
	if out.Chest.RemainingSeconds != 0 {
		t.Errorf("Chest.RemainingSeconds = %d, want 0 (unlockAt in past)", out.Chest.RemainingSeconds)
	}
}

// ============================================================
// AC8.3: 不 INSERT pets → svc.LoadHome 返 err=nil + Pet=nil（V1 §5.1 钦定可空）
// ============================================================
func TestHomeService_LoadHome_NoPet_PetIsNil(t *testing.T) {
	svc, sqlDB, cleanup := buildHomeServiceIntegration(t)
	defer cleanup()

	insertUser(t, sqlDB, 1, "uid-home-3", "用户1", "")
	// **不** insertPet —— 验证 ErrPetNotFound → service 视为 Pet=nil（不报错）
	insertStepAccount(t, sqlDB, 1, 0, 0, 0)
	insertChest(t, sqlDB, 5001, 1, 1, time.Now().UTC().Add(10*time.Minute), 1000)

	out, err := svc.LoadHome(context.Background(), 1)
	if err != nil {
		t.Fatalf("LoadHome: %v, want nil err (pet 缺失视为可空)", err)
	}
	if out.Pet != nil {
		t.Errorf("Pet = %+v, want nil", out.Pet)
	}
	// 其他字段仍正常
	if out.User.ID != 1 {
		t.Errorf("User.ID = %d, want 1", out.User.ID)
	}
	if out.Chest.ID != 5001 {
		t.Errorf("Chest.ID = %d, want 5001", out.Chest.ID)
	}
}
