//go:build integration
// +build integration

// Story 7.5 集成测试：用 dockertest 起真实 mysql:8.0 容器跑 dev grant-steps 链路。
//
// 复用 7.3 / 4.6 / 4.8 已建的 startMySQL / runMigrations / insertUser /
// insertStepAccount / fetchStepAccount / countSyncLogs helper。
//
// **手工 INSERT** user / step_account（不调 auth_service.GuestLogin） ——
// 解耦 dev_step_service 测试与 auth_service。

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

// buildDevStepServiceIntegration: 起容器 → migrate → 装配 svc + 返清理 closure。
//
// 与 buildStepServiceIntegration (7.3) 的区别：本 helper 还构造 userRepo（dev grant 需要 FindByID）。
func buildDevStepServiceIntegration(t *testing.T) (svc service.DevStepService, sqlDB *sql.DB, cleanup func()) {
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
	stepAccountRepo := mysql.NewStepAccountRepo(gormDB)
	stepSyncLogRepo := mysql.NewStepSyncLogRepo(gormDB)

	svc = service.NewDevStepService(txMgr, userRepo, stepAccountRepo, stepSyncLogRepo)

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

// ============================================================
// AC8 case 1: HappyPath 创建用户 → grant 5000 → 验账户 + sync_log 字段全
// ============================================================
//
// 验证 epics.md §Story 7.5 行 1442 钦定 + 数据库设计 §6.6 source=2 admin_grant：
//   1. INSERT user + step_account → svc.GrantSteps(userID, 5000)
//   2. SELECT user_step_accounts → total=5000 / available=5000 / consumed=0 / version=1
//   3. SELECT user_step_sync_logs → 1 行 source=2 / accepted=5000 / motion_state=1 /
//      client_total=0 / client_ts=0
//   4. 加分：再 grant 3000 → total=8000 / version=2 / sync_log count=2（验幂等性 = "故意可重复"）
func TestDevStepServiceIntegration_GrantSteps_AppliesAccountAndAuditLog(t *testing.T) {
	svc, sqlDB, cleanup := buildDevStepServiceIntegration(t)
	defer cleanup()

	const userID = uint64(1)
	insertUser(t, sqlDB, userID, "uid-dev-grant-1", "用户DEV", "")
	insertStepAccount(t, sqlDB, userID, 0, 0, 0)

	ctx := context.Background()

	// 1. dev grant 5000
	if err := svc.GrantSteps(ctx, userID, 5000); err != nil {
		t.Fatalf("GrantSteps 5000: %v", err)
	}

	// 2. 验 user_step_accounts 三档值（total / available 各 +5000；consumed 不变）
	total, avail, consumed, version := fetchStepAccount(t, sqlDB, userID)
	if total != 5000 || avail != 5000 || consumed != 0 {
		t.Errorf("after grant: total=%d available=%d consumed=%d, want 5000/5000/0", total, avail, consumed)
	}
	if version != 1 {
		t.Errorf("version = %d, want 1 (initial 0 + 1)", version)
	}

	// 3. 验 user_step_sync_logs 写入了 1 行 source=2 admin_grant
	syncDate := time.Now().UTC().Format("2006-01-02")
	if got := countSyncLogs(t, sqlDB, userID, syncDate); got != 1 {
		t.Errorf("sync_log count for today = %d, want 1", got)
	}

	// 4. 验 sync_log 字段（source / accepted_delta / motion_state / client_total / client_ts）
	row := sqlDB.QueryRow(
		`SELECT source, accepted_delta_steps, motion_state, client_total_steps, client_ts FROM user_step_sync_logs WHERE user_id = ? AND sync_date = ? ORDER BY id DESC LIMIT 1`,
		userID, syncDate,
	)
	var (
		source        int8
		acceptedDelta int32
		motionState   int8
		clientTotal   uint64
		clientTs      uint64
	)
	if err := row.Scan(&source, &acceptedDelta, &motionState, &clientTotal, &clientTs); err != nil {
		t.Fatalf("scan sync_log: %v", err)
	}
	if source != 2 {
		t.Errorf("source = %d, want 2 (admin_grant; §6.6 钦定)", source)
	}
	if acceptedDelta != 5000 {
		t.Errorf("accepted_delta_steps = %d, want 5000", acceptedDelta)
	}
	if motionState != 1 {
		t.Errorf("motion_state = %d, want 1 (stationary_or_unknown 中性值)", motionState)
	}
	if clientTotal != 0 {
		t.Errorf("client_total_steps = %d, want 0 (dev grant 无客户端累计语义)", clientTotal)
	}
	if clientTs != 0 {
		t.Errorf("client_ts = %d, want 0 (dev grant 无客户端时刻语义)", clientTs)
	}

	// 5. 加分：再 grant 3000 → total=8000 / available=8000 / version=2 / sync_log count=2
	if err := svc.GrantSteps(ctx, userID, 3000); err != nil {
		t.Fatalf("GrantSteps 3000: %v", err)
	}
	total2, avail2, consumed2, version2 := fetchStepAccount(t, sqlDB, userID)
	if total2 != 8000 || avail2 != 8000 || consumed2 != 0 {
		t.Errorf("after second grant: total=%d available=%d consumed=%d, want 8000/8000/0", total2, avail2, consumed2)
	}
	if version2 != 2 {
		t.Errorf("version after second = %d, want 2", version2)
	}
	if got := countSyncLogs(t, sqlDB, userID, syncDate); got != 2 {
		t.Errorf("sync_log count after second = %d, want 2 (dev 端点 = 故意可重复)", got)
	}
}

// ============================================================
// AC8 case 2: UserNotFound → 1003（验 epics.md AC 钦定 edge case）
// ============================================================
//
// service 单测已用 stub 覆盖；集成补一次 MySQL 真返 ErrUserNotFound 链路。
func TestDevStepServiceIntegration_GrantSteps_UserNotFound_Returns1003(t *testing.T) {
	svc, sqlDB, cleanup := buildDevStepServiceIntegration(t)
	defer cleanup()
	_ = sqlDB

	const nonExistentUserID = uint64(99999)
	err := svc.GrantSteps(context.Background(), nonExistentUserID, 5000)
	if err == nil {
		t.Fatal("GrantSteps should fail when user does not exist")
	}
	if got := apperror.Code(err); got != apperror.ErrResourceNotFound {
		t.Errorf("apperror.Code = %d, want %d (1003 ErrResourceNotFound)", got, apperror.ErrResourceNotFound)
	}
}
