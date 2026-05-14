//go:build integration
// +build integration

// Story 20.7 集成测试：用 dockertest 起真实 mysql:8.0 容器跑 dev force-unlock-chest 链路。
//
// 复用 7.5 / 4.6 / 4.8 / 20.6 已建的 startMySQL / runMigrations / insertUser /
// insertChest helper（同 package service_test）。
//
// **手工 INSERT** user / chest（不调 auth_service.GuestLogin）—— 解耦 dev_chest_service
// 测试与 auth_service。
//
// **r4 [P2] 改造**：
//   - service 重新注入 txMgr（用 r4 真 tx.Manager；不再用 stub）
//   - service.ForceUnlockChest 用事务 + FindByIDForUpdate + UpdateUnlockAtByID 三件套
//   - case 3 改为验证"同毫秒重复 unlock 同 chest → 两次都 success"（r3 false-positive 1003 的修复见证）
//   - case 4 越权 unlock 他人 chest → 1003（r2 引入；r4 沿用）

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

// buildDevChestServiceIntegration: 起容器 → migrate → 装配 svc + 返清理 closure。
//
// **r4 [P2] 改造**：service 重新接 txMgr —— 与 r2/r3 的"无事务"区分。
// txMgr 用真 tx.NewManager（与 chest_open_service_integration_test.go 同模式）。
func buildDevChestServiceIntegration(t *testing.T) (svc service.DevChestService, sqlDB *sql.DB, cleanup func()) {
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
	txMgr := tx.NewManager(gormDB)
	svc = service.NewDevChestService(txMgr, chestRepo)

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
// AC9 case 1: HappyPath
// ============================================================
//
// 验证 epics.md §Story 20.7 行 2949 钦定：
// 用户 chest unlock_at 在未来 → /dev/force-unlock-chest → GET /chest/current 返回 status=2。
//
// 本 case 直接调 svc.ForceUnlockChest（绕过 handler）+ 直接 SELECT 验 unlock_at <= now；
// 完整 HTTP 链路（含 dev mode 闸门）由 dev_chest_handler_test 单测 + router_dev_test 覆盖。
func TestDevChestServiceIntegration_ForceUnlockChest_PushesUnlockAtToNow(t *testing.T) {
	svc, sqlDB, cleanup := buildDevChestServiceIntegration(t)
	defer cleanup()

	const userID = uint64(1)
	const chestID = uint64(5001)
	insertUser(t, sqlDB, userID, "uid-dev-force-unlock-1", "用户DEV", "")

	unlockAtFuture := time.Now().UTC().Add(10 * time.Minute)
	insertChest(t, sqlDB, chestID, userID, 1 /* status=counting */, unlockAtFuture, 1000)

	ctx := context.Background()
	beforeNow := time.Now().UTC()

	// 调 dev force-unlock（r4 改造后 service 仍接 chestID，事务内执行）
	if err := svc.ForceUnlockChest(ctx, userID, chestID); err != nil {
		t.Fatalf("ForceUnlockChest: %v", err)
	}

	// SELECT user_chests 验 unlock_at 已被推到 now（落在 [beforeNow, afterNow] 区间内）
	afterNow := time.Now().UTC()
	row := sqlDB.QueryRow(`SELECT unlock_at FROM user_chests WHERE id = ?`, chestID)
	var newUnlockAt time.Time
	if err := row.Scan(&newUnlockAt); err != nil {
		t.Fatalf("scan unlock_at: %v", err)
	}
	newUnlockAtUTC := newUnlockAt.UTC()
	// 容忍 1s 偏差
	if newUnlockAtUTC.Before(beforeNow.Add(-time.Second)) || newUnlockAtUTC.After(afterNow.Add(time.Second)) {
		t.Errorf("unlock_at = %v, want in [%v, %v]", newUnlockAtUTC, beforeNow, afterNow)
	}

	// 仿真 GET /chest/current 动态判定：unlock_at <= now → status=2 unlockable
	if newUnlockAtUTC.After(afterNow) {
		t.Errorf("unlock_at %v should be <= now %v (so GetCurrent would return status=2 unlockable)", newUnlockAtUTC, afterNow)
	}
}

// ============================================================
// AC9 case 2: ChestNotFound → 1003
// ============================================================
//
// dev force-unlock 一个不存在的 chestID → 返 1003。
// service 单测已用 stub 覆盖；集成补一次 MySQL 真返 ErrChestNotFound 链路。
func TestDevChestServiceIntegration_ForceUnlockChest_ChestNotFound_Returns1003(t *testing.T) {
	svc, sqlDB, cleanup := buildDevChestServiceIntegration(t)
	defer cleanup()
	_ = sqlDB

	const userID = uint64(1)
	const nonExistentChestID = uint64(999999)
	err := svc.ForceUnlockChest(context.Background(), userID, nonExistentChestID)
	if err == nil {
		t.Fatal("ForceUnlockChest should fail when chest not found")
	}
	if got := apperror.Code(err); got != apperror.ErrResourceNotFound {
		t.Errorf("apperror.Code = %d, want %d (1003 ErrResourceNotFound)", got, apperror.ErrResourceNotFound)
	}
}

// ============================================================
// AC9 case 3: 连续两次 unlock 同 chest → 两次都 success（r4 [P2] 修复见证）
// ============================================================
//
// **r3 → r4 改造关键 case**：
//   - r3 实装：两次 unlock 落在同一毫秒 → 第二次 RowsAffected=0 → 误报 1003（false positive）；
//   - r4 实装：事务 + FOR UPDATE + repo 不再用 RowsAffected==0 判 NotFound → 两次都 nil success；
//     即便 unlock_at 列值未变，repo 也视为"已是该状态"，返 success。
//
// 集成测试无法稳定保证两次调用落在同一毫秒（time.Now() 至少有微秒级偏移），但可以用以下两种验证：
//   (a) 连续两次调用 ForceUnlockChest 都成功；
//   (b) 第二次调用后 chest 仍在表中 + unlock_at 落在 [开始, 现在] 内（验证"chest 未被误删 / 未误报 NotFound"）。
//
// 这个测试在 r3 实装下若两次调用偶然落同毫秒会偶发 1003；r4 实装下永远 success。
func TestDevChestServiceIntegration_ForceUnlockChest_RepeatUnlock_BothSucceed(t *testing.T) {
	svc, sqlDB, cleanup := buildDevChestServiceIntegration(t)
	defer cleanup()

	const userID = uint64(3)
	const chestID = uint64(5003)
	insertUser(t, sqlDB, userID, "uid-dev-force-unlock-3", "用户DEV3", "")
	unlockAtFuture := time.Now().UTC().Add(10 * time.Minute)
	insertChest(t, sqlDB, chestID, userID, 1, unlockAtFuture, 1000)

	beforeNow := time.Now().UTC()

	// 第一次 unlock
	if err := svc.ForceUnlockChest(context.Background(), userID, chestID); err != nil {
		t.Fatalf("first ForceUnlockChest: %v", err)
	}

	// 立即第二次 unlock（可能落同毫秒；r4 实装下无论如何都 success）
	if err := svc.ForceUnlockChest(context.Background(), userID, chestID); err != nil {
		t.Fatalf("second ForceUnlockChest should succeed (r4 [P2] 修 r3 同毫秒重复误报 1003 bug): %v", err)
	}

	// 验证 chest 仍存在且 unlock_at 已被推到 now
	afterNow := time.Now().UTC()
	var unlockAt time.Time
	if err := sqlDB.QueryRow(`SELECT unlock_at FROM user_chests WHERE id = ?`, chestID).Scan(&unlockAt); err != nil {
		t.Fatalf("scan unlock_at after repeat unlock: %v", err)
	}
	unlockAtUTC := unlockAt.UTC()
	if unlockAtUTC.Before(beforeNow.Add(-time.Second)) || unlockAtUTC.After(afterNow.Add(time.Second)) {
		t.Errorf("unlock_at = %v after repeat unlock, want in [%v, %v]", unlockAtUTC, beforeNow, afterNow)
	}
}

// ============================================================
// AC9 case 4: 越权 unlock 他人 chest → 1003（r2 [P2] 引入；r4 沿用）
// ============================================================
//
// 验证 r2 [P2] 防御性 user_id 校验：dev 端点无 auth，恶意 client 可传任意 (userID, chestID)
// 组合（如 userID=自己的, chestID=别人的）；service 在 FindByIDForUpdate 后用 chest.user_id 比对
// 阻止越权 → 返 1003（与 ChestNotFound 同码，避免暴露"这个 chestID 存在但属于别人"信息）。
func TestDevChestServiceIntegration_ForceUnlockChest_CrossUserUnlock_Returns1003(t *testing.T) {
	svc, sqlDB, cleanup := buildDevChestServiceIntegration(t)
	defer cleanup()

	const userA = uint64(4)
	const userB = uint64(5)
	const chestB = uint64(5005)
	insertUser(t, sqlDB, userA, "uid-dev-A", "用户A", "")
	insertUser(t, sqlDB, userB, "uid-dev-B", "用户B", "")
	unlockAtFuture := time.Now().UTC().Add(10 * time.Minute)
	insertChest(t, sqlDB, chestB, userB, 1, unlockAtFuture, 1000)

	// userA 试图 unlock userB 的 chest → 应被拒
	err := svc.ForceUnlockChest(context.Background(), userA, chestB)
	if err == nil {
		t.Fatal("ForceUnlockChest should fail when chest belongs to another user")
	}
	if got := apperror.Code(err); got != apperror.ErrResourceNotFound {
		t.Errorf("apperror.Code = %d, want %d (1003; 与 ChestNotFound 同码避免信息泄露)", got, apperror.ErrResourceNotFound)
	}

	// 确认 userB 的 chest unlock_at 未被改动（unlock_at 仍 > now）
	var unlockAt time.Time
	if err := sqlDB.QueryRow(`SELECT unlock_at FROM user_chests WHERE id = ?`, chestB).Scan(&unlockAt); err != nil {
		t.Fatalf("scan unlock_at: %v", err)
	}
	if !unlockAt.UTC().After(time.Now().UTC()) {
		t.Errorf("userB chest unlock_at = %v, expected to remain in future (越权 unlock 必须无副作用)", unlockAt.UTC())
	}
}
