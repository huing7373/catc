//go:build integration
// +build integration

// Story 20.5 chest_service 集成测试：用 dockertest 起真实 mysql:8.0 容器跑 2 条 happy 链路 case：
//   1. unlock_at = now + 10min → svc.GetCurrent → status=1, remainingSeconds ≈ 600
//   2. unlock_at = now - 5min  → svc.GetCurrent → status=2, remainingSeconds=0（动态判定端到端）
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑这些；
// 只在 `bash scripts/build.sh --integration`（即 `go test -tags=integration`）触发。
//
// 本机 Windows docker 不可用 → t.Skip（startMySQL 内已 skip）。CI Linux 跑。
//
// 复用 4.6 auth_service_integration_test.go 的 startMySQL / runMigrations helper +
// 4.8 home_service_integration_test.go 的 insertUser / insertChest helper。
//
// **手工 INSERT 测试数据**（**不**调 4.6 auth_service.GuestLogin） —— 解耦 chest_service
// 测试与 auth_service：chest_service 集成测试只验 chest 链路（chestRepo + 动态判定），
// 调 auth_service 会引入 4.6 实装变更敏感性。

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

// buildChestServiceIntegration: 起容器 → migrate → 装配 svc + 返清理 closure。
//
// 与 buildHomeServiceIntegration 同模式，但只用 chestRepo（chest_service 单 repo 依赖）。
func buildChestServiceIntegration(t *testing.T) (svc service.ChestService, sqlDB *sql.DB, cleanup func()) {
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

	chestRepo := mysql.NewChestRepo(gormDB)
	svc = service.NewChestService(chestRepo)

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

// AC6.1: HappyPath Counting
// INSERT chest with unlock_at=now+10min → svc.GetCurrent → status=1, remainingSeconds ≈ 600
func TestChestServiceIntegration_GetCurrent_HappyPath_Counting_Returns600s(t *testing.T) {
	svc, sqlDB, cleanup := buildChestServiceIntegration(t)
	defer cleanup()

	const userID = uint64(1)
	insertUser(t, sqlDB, userID, "uid-chest-get-1", "用户chest-counting", "")

	// 关键：unlock_at = now + 10min（节点 7 Story 4.6 firstTimeLogin 钦定的初始倒计时）
	unlockAt := time.Now().UTC().Add(10 * time.Minute)
	insertChest(t, sqlDB, 5001, userID, 1 /* status=counting */, unlockAt, 1000 /* open_cost_steps */)

	ctx := context.Background()
	out, err := svc.GetCurrent(ctx, userID)
	if err != nil {
		t.Fatalf("GetCurrent: %v", err)
	}
	if out.ID != 5001 {
		t.Errorf("ID = %d, want 5001", out.ID)
	}
	if out.Status != 1 {
		t.Errorf("Status = %d, want 1 (counting)", out.Status)
	}
	if out.OpenCostSteps != 1000 {
		t.Errorf("OpenCostSteps = %d, want 1000", out.OpenCostSteps)
	}
	// remainingSeconds ≈ 600（10min = 600s）；service 内部 time.Now() 与本测试 setup time.Now()
	// 间隔可能含 dockertest 容器 + GORM round-trip 1-2s 延迟，给宽容区间 [595, 601]
	if out.RemainingSeconds < 595 || out.RemainingSeconds > 601 {
		t.Errorf("RemainingSeconds = %d, want ≈ 600 (allow [595, 601] for clock jitter + dockertest delay)", out.RemainingSeconds)
	}
	// UnlockAt 必须是 UTC（service 强制 .UTC()）
	if out.UnlockAt.Location().String() != "UTC" {
		t.Errorf("UnlockAt location = %q, want UTC", out.UnlockAt.Location().String())
	}
}

// AC6.2 (bonus): UnlockAtPassed_DynamicReturns2
// INSERT chest with DB status=1 + unlock_at=now-5min → svc.GetCurrent → status=2, remainingSeconds=0
// 验证"DB 原值 status=1 但下发 status=2"动态判定的端到端正确性（覆盖 V1 §7.1 行 836 钦定动态判定的核心 case）
func TestChestServiceIntegration_GetCurrent_UnlockAtPassed_DynamicReturns2(t *testing.T) {
	svc, sqlDB, cleanup := buildChestServiceIntegration(t)
	defer cleanup()

	const userID = uint64(1)
	insertUser(t, sqlDB, userID, "uid-chest-get-2", "用户chest-unlockable", "")

	// 关键：DB status=1 (counting) + unlock_at 已过 5 分钟
	pastUnlockAt := time.Now().UTC().Add(-5 * time.Minute)
	insertChest(t, sqlDB, 5002, userID, 1 /* DB status=counting */, pastUnlockAt, 1000)

	ctx := context.Background()
	out, err := svc.GetCurrent(ctx, userID)
	if err != nil {
		t.Fatalf("GetCurrent: %v", err)
	}
	// 关键：DB 原值 Status=1 但 service 算出 Status=2 (unlockable)
	if out.Status != 2 {
		t.Errorf("Status = %d, want 2 (unlockable —— service 必须动态判定 DB=1 + unlock_at 已过 → 下发 2)", out.Status)
	}
	if out.RemainingSeconds != 0 {
		t.Errorf("RemainingSeconds = %d, want 0 (unlockAt 已过)", out.RemainingSeconds)
	}
}
