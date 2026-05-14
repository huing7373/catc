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
// **Story 20.7 review r2 [P2] 改造**：
//   - service 不再注入 txMgr（r1 的"事务 + FOR UPDATE"对 race 修复无作用）
//   - service.ForceUnlockChest 接 chestID 参数（client 通过 GET /chest/current 拿到）
//
// **Story 20.7 review r3 [P2] 改造**：
//   - 重新加回 repo 层 RowsAffected==0 → ErrChestNotFound 翻译，修 r2 引入的二阶 race（FindByID 后
//     chest 被并发 OpenChest 删除 → UPDATE 0 行但 service 返 false success → 用户体验 dev 端点
//     "声称成功但 GET /chest/current 仍 counting"）
//   - case 3 从"同一毫秒重复 unlock 同 chest 应成功"改为"FindByID 后并发删除 chest → 1003"
//     —— 模拟二阶 race scenario：先 INSERT chest → 调 ForceUnlockChest 前先手工 DELETE → service 拿
//     不到 chest（步骤 1 FindByID 返 NotFound）；后续也可加纯 UPDATE 路径的删除（绕过 FindByID）但
//     stub 单测已覆盖 case 6 ConcurrentDeleteRace
//   - case 4 越权 unlock 他人 chest → 1003

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
	"github.com/huing/cat/server/internal/service"
)

// buildDevChestServiceIntegration: 起容器 → migrate → 装配 svc + 返清理 closure。
//
// **r2 [P2] 改造**：service 不再需要 txMgr —— 与 r1 之前的"事务内 FOR UPDATE + UPDATE"区分。
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
	svc = service.NewDevChestService(chestRepo)

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

	// 调 dev force-unlock（r2 改造后 service 接 chestID）
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
// AC9 case 3: 已删除的 chest（已为 PK 不存在）→ 1003（r3 [P2] 改造）
// ============================================================
//
// **r2 → r3 改造**：r2 的 case 3 验证"同一毫秒重复 unlock 同 chest 应成功"（基于"RowsAffected=0
// 不再误判"）；r3 把 RowsAffected==0 重新翻译为 ErrChestNotFound，恢复"chest 不存在 → 1003"语义。
// 本 case 改为验证"chest 在 ForceUnlockChest 调用前已被删除 → 返 1003"——FindByID 直接返
// NotFound 是步骤 1 短路路径，case 2 已覆盖；这里特意 INSERT 后立即 DELETE 来模拟"chest 曾存在
// 但 dev 调用瞬间不存在"的边界。
//
// 二阶 race（FindByID 拿到 chest 后 UPDATE 前被并发删除 → UPDATE 0 行 → repo 翻译为 ErrChestNotFound
// → service 翻译为 1003）的单元覆盖由 dev_chest_service_test.go case 6 ConcurrentDeleteRace_Returns1003
// 通过 stub 模拟（集成测试无法稳定复现"两个 query 之间另一个 transaction 完成 DELETE"的时序）。
func TestDevChestServiceIntegration_ForceUnlockChest_ChestAlreadyDeleted_Returns1003(t *testing.T) {
	svc, sqlDB, cleanup := buildDevChestServiceIntegration(t)
	defer cleanup()

	const userID = uint64(3)
	const chestID = uint64(5003)
	insertUser(t, sqlDB, userID, "uid-dev-force-unlock-3", "用户DEV3", "")
	unlockAtFuture := time.Now().UTC().Add(10 * time.Minute)
	insertChest(t, sqlDB, chestID, userID, 1, unlockAtFuture, 1000)

	// 立即手工删除（模拟"chest 已被并发 OpenChest 删除"的时序终点状态）
	if _, err := sqlDB.Exec(`DELETE FROM user_chests WHERE id = ?`, chestID); err != nil {
		t.Fatalf("manual DELETE: %v", err)
	}

	// dev force-unlock → FindByID 返 ErrChestNotFound → service 翻译为 1003
	err := svc.ForceUnlockChest(context.Background(), userID, chestID)
	if err == nil {
		t.Fatal("ForceUnlockChest should fail when chest already deleted (r3 [P2])")
	}
	if got := apperror.Code(err); got != apperror.ErrResourceNotFound {
		t.Errorf("apperror.Code = %d, want %d (1003)", got, apperror.ErrResourceNotFound)
	}
}

// ============================================================
// AC9 case 4: 越权 unlock 他人 chest → 1003（r2 [P2] 新增）
// ============================================================
//
// 验证 r2 [P2] 防御性 user_id 校验：dev 端点无 auth，恶意 client 可传任意 (userID, chestID)
// 组合（如 userID=自己的, chestID=别人的）；service 在 FindByID 后用 chest.user_id 比对
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
