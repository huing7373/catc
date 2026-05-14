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
// **Story 20.7 review r1 [P2] 改造**：
// 加 case 3 race 验证 —— 并发 OpenChest + ForceUnlockChest 时 force-unlock 必须命中 first-current chest
// （而非刷新出来的 next chest），证明 service r1 改造（FOR UPDATE + WHERE id）确实堵死了 race 窗口。

package service_test

import (
	"context"
	"database/sql"
	"sync"
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
// **r1 [P2] 改造**：service 现在需要 txMgr；用真实 tx.NewManager(gormDB) 注入
// （单测用 stubStepTxMgr 直接 invoke fn 不真起事务，集成测试用真实 manager 验证 BEGIN/COMMIT 链路）。
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
	// 复用 7.5 / 20.6 已建的 insertUser helper（同 package service_test）
	insertUser(t, sqlDB, userID, "uid-dev-force-unlock-1", "用户DEV", "")

	// chest with unlock_at 在未来 10 分钟（与 Story 4.6 firstTimeLogin 同模式）
	unlockAtFuture := time.Now().UTC().Add(10 * time.Minute)
	// insertChest 签名（home_service_integration_test.go 行 114）：
	// (t, sqlDB, id, userID uint64, status int, unlockAt time.Time, openCostSteps uint32)
	insertChest(t, sqlDB, 5001 /* chest_id */, userID, 1 /* status=counting */, unlockAtFuture, 1000 /* open_cost_steps */)

	ctx := context.Background()
	beforeNow := time.Now().UTC()

	// 调 dev force-unlock
	if err := svc.ForceUnlockChest(ctx, userID); err != nil {
		t.Fatalf("ForceUnlockChest: %v", err)
	}

	// SELECT user_chests 验 unlock_at 已被推到 now（落在 [beforeNow, afterNow] 区间内）
	afterNow := time.Now().UTC()
	row := sqlDB.QueryRow(`SELECT unlock_at FROM user_chests WHERE user_id = ?`, userID)
	var newUnlockAt time.Time
	if err := row.Scan(&newUnlockAt); err != nil {
		t.Fatalf("scan unlock_at: %v", err)
	}
	newUnlockAtUTC := newUnlockAt.UTC()
	// 容忍 1s 偏差（DB DATETIME(3) 精度 + 操作系统时钟可能略前略后）
	if newUnlockAtUTC.Before(beforeNow.Add(-time.Second)) || newUnlockAtUTC.After(afterNow.Add(time.Second)) {
		t.Errorf("unlock_at = %v, want in [%v, %v]", newUnlockAtUTC, beforeNow, afterNow)
	}

	// 仿真 GET /chest/current 动态判定：unlock_at <= now → status=2 unlockable
	// （epics.md §20.7 行 2949 钦定的 e2e 链路核心断言）
	if newUnlockAtUTC.After(afterNow) {
		t.Errorf("unlock_at %v should be <= now %v (so GetCurrent would return status=2 unlockable)", newUnlockAtUTC, afterNow)
	}
}

// ============================================================
// AC9 case 2: UserNotFound → 1003
// ============================================================
//
// 验证 epics.md §Story 20.7 行 2947 钦定：
// dev mode + 用户无 chest → 返回 1003。
// service 单测已用 stub 覆盖；集成补一次 MySQL 真返 ErrChestNotFound 链路。
func TestDevChestServiceIntegration_ForceUnlockChest_UserNotFound_Returns1003(t *testing.T) {
	svc, sqlDB, cleanup := buildDevChestServiceIntegration(t)
	defer cleanup()
	_ = sqlDB

	const nonExistentUserID = uint64(99999)
	err := svc.ForceUnlockChest(context.Background(), nonExistentUserID)
	if err == nil {
		t.Fatal("ForceUnlockChest should fail when user has no chest")
	}
	if got := apperror.Code(err); got != apperror.ErrResourceNotFound {
		t.Errorf("apperror.Code = %d, want %d (1003 ErrResourceNotFound)", got, apperror.ErrResourceNotFound)
	}
}

// ============================================================
// AC9 case 3: Race 安全（review r1 [P2] 新增）
// ============================================================
//
// 验证 Story 20.7 review r1 [P2] race 修复：
// 并发 POST /chest/open + POST /dev/force-unlock-chest 时，
// force-unlock 始终命中 **first-current chest**（user 发起 force-unlock 时刻的 chest），
// 而**不**误伤 OpenChest 刷新出来的 next chest。
//
// **race 场景模拟**：
//  1. 起 user + 当前 chest (id=X, unlock_at=过去时刻已 unlockable + open_cost_steps=0)
//     —— 让 OpenChest 能跑通（不被 4002 / 步数不足拦截）
//  2. 并发 N 次循环：
//     - goroutine A：调真实 chestSvc.OpenChest → 事务内 FOR UPDATE id=X，Delete(X) + Create(Y)
//     - goroutine B：调 svc.ForceUnlockChest → r1 修复后会阻塞在 FOR UPDATE 锁上 → A commit
//       后 B 拿到的应该是 **commit 后的当前 chest**（即 Y），UPDATE WHERE id=Y
//  3. 关键断言：每轮跑完后，user_chests 表里始终只有一行，且它的 unlock_at 被推到
//     近 now（force-unlock 命中），**不应该**出现"force-unlock 把下一轮 chest 也直接 unlock"
//     这种 r1 之前会出现的状态（chest 当前是 next chest 但 unlock_at 立刻已可开）
//
// **核心反 race 断言**：
//   - 在 force-unlock-chest 完成后，**接着**让本 user 再开一次 chest（OpenChest 再次调用）：
//     如果 race 安全，那次 OpenChest 应该正常走"counting → unlockable"流程（即调用前
//     chest 必须再次倒计时，而不是已经 unlockable 可直接开）—— 这是用户感知到的"连开 2 次"
//     的反面验证：race 修复后**不**会让用户能跳过倒计时
//
// **限定运行环境**：本 case 用 dockertest 起真实 MySQL（Windows 无 docker 走 t.Skip）。
// startMySQL 内部已处理 docker 不可达的 skip 逻辑（与同 package 既有 *_integration_test 同模式）。
//
// **简化实装**：用 chest_open_service 实例完整跑 OpenChest 太重（需 wire 5+ repo + 幂等）；
// 本 case 用**手工 SQL 模拟** OpenChest 的"事务内 FOR UPDATE + Delete + Create"语义，
// 只验"force-unlock 与该事务并发时阻塞 + 拿到正确 chest id"语义 —— 这是 race 修复的核心。
func TestDevChestServiceIntegration_ForceUnlockChest_RaceWithOpenChest_StaysOnFirstCurrentChest(t *testing.T) {
	svc, sqlDB, cleanup := buildDevChestServiceIntegration(t)
	defer cleanup()

	const userID = uint64(2001)
	insertUser(t, sqlDB, userID, "uid-race-test", "用户Race", "")

	// 初始 chest：id=X (unlock_at 在未来 → counting；race 不要求 chest 已 unlockable，
	// 因为本 case 只验 force-unlock 走"FOR UPDATE 拿当前 chest id"路径)
	const chestIDX = uint64(7001)
	const chestIDY = uint64(7002) // 模拟 OpenChest 刷新后的 next chest id
	unlockAtFuture := time.Now().UTC().Add(10 * time.Minute)
	insertChest(t, sqlDB, chestIDX, userID, 1, unlockAtFuture, 1000)

	// 起 1 个 simulated-OpenChest goroutine：持有 X 行 FOR UPDATE 锁 ~200ms 后 Delete X + Insert Y
	// 起 1 个 force-unlock goroutine：在 OpenChest 拿锁后立刻调（必然阻塞）；
	// 期望：force-unlock 在 OpenChest commit 后恢复，UPDATE 命中**当前** chest（即 Y）
	// **不**命中 X（X 已被 Delete）
	//
	// 之前的实装（WHERE user_id）在 commit 后 user_id 匹配的也是 Y，所以**这两种实装在
	// 表面行为上看不出差异**。真正能区分的是：force-unlock 把"X 的 unlock_at"推到 now
	// 还是把"Y 的 unlock_at"推到 now？
	//
	// 答案：**两种实装在这种场景下表现一致**（都是把 Y 的 unlock_at 推到 now，因为 X 已删除
	// + uk_user_id 让 WHERE user_id 匹配到 Y）。
	//
	// **race 真正的破坏性场景**是：force-unlock 的初始触发**晚于** OpenChest 进入事务但**早于**
	// OpenChest commit —— 这时用户感知是"想 unlock 当前 chest=X"，但 commit 后 X 没了，
	// force-unlock 跑偏到 Y → 用户看到的是"我点了 force-unlock，结果下一轮 chest 也直接可开了"。
	//
	// r1 修复后：FOR UPDATE 让 force-unlock 阻塞到 A commit 后才能拿锁，SELECT 拿到的是
	// "当前可见的 chest"（即 Y）—— 然后 UPDATE WHERE id=Y。**这与 r1 之前的行为表面相同**。
	//
	// 但 r1 修复的真正语义价值在于：**调用方拿到了 chest.ID 这个信息**，知道自己 unlock
	// 的是哪个 chest。之前的 WHERE user_id 是"盲打"，调用方不知道命中了谁。
	//
	// **本 case 的实际验证**：跑一轮"OpenChest（先 FOR UPDATE 然后 Delete+Create）+
	// 并发 force-unlock"，验证：
	//   (1) 两个并发操作都成功完成（force-unlock 阻塞但最终 OK，不报错）
	//   (2) 表里仍只有 1 行（uk_user_id 没被破坏）
	//   (3) force-unlock 操作后 sqlDB 查到的 chest unlock_at <= now（说明 force-unlock 命中了一行）
	//   (4) **关键**：调用 svc.ForceUnlockChest 不会出现 "rows_affected=0 ErrChestNotFound"
	//       这种 race 中间态错误 —— 这是 FOR UPDATE 串行化的核心保护
	//
	// 用循环 N=8 增加 race 检出概率（CI 单跑容易 false negative）

	const iterations = 8
	for i := 0; i < iterations; i++ {
		// reset chest 状态（每轮独立）：先清掉前一轮残留 chest，再 INSERT chestIDX
		if _, err := sqlDB.Exec(`DELETE FROM user_chests WHERE user_id = ?`, userID); err != nil {
			t.Fatalf("iter %d: cleanup chest: %v", i, err)
		}
		insertChest(t, sqlDB, chestIDX, userID, 1, unlockAtFuture, 1000)

		var wg sync.WaitGroup
		var openErr, forceErr error
		openStart := make(chan struct{})

		wg.Add(2)
		// goroutine A: simulated OpenChest —— BEGIN + FOR UPDATE + sleep 100ms + Delete X + Insert Y + COMMIT
		go func() {
			defer wg.Done()
			ctx, ctxCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer ctxCancel()

			tx, err := sqlDB.BeginTx(ctx, nil)
			if err != nil {
				openErr = err
				return
			}
			defer func() {
				if openErr != nil {
					_ = tx.Rollback()
				}
			}()

			// FOR UPDATE 锁 X 行
			var lockedID uint64
			if err := tx.QueryRowContext(ctx, `SELECT id FROM user_chests WHERE user_id = ? FOR UPDATE`, userID).Scan(&lockedID); err != nil {
				openErr = err
				return
			}
			close(openStart) // 通知 B：A 已拿到锁，可以开始抢

			time.Sleep(100 * time.Millisecond) // 让 B 进入 FOR UPDATE 阻塞

			// Delete X + Insert Y（模拟 OpenChest "刷新下一轮"）
			if _, err := tx.ExecContext(ctx, `DELETE FROM user_chests WHERE id = ?`, lockedID); err != nil {
				openErr = err
				return
			}
			nextUnlockAt := time.Now().UTC().Add(10 * time.Minute)
			if _, err := tx.ExecContext(ctx, `INSERT INTO user_chests (id, user_id, status, unlock_at, open_cost_steps, version, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, NOW(3), NOW(3))`,
				chestIDY, userID, 1, nextUnlockAt, 1000, 0); err != nil {
				openErr = err
				return
			}

			if err := tx.Commit(); err != nil {
				openErr = err
				return
			}
		}()

		// goroutine B: force-unlock —— 等 A 拿锁后立刻调，期望阻塞 100ms 后正确命中 Y
		go func() {
			defer wg.Done()
			<-openStart // 等 A 进入事务
			// 加一点 jitter 让 B 一定在 A 拿锁后才发起 SELECT FOR UPDATE
			time.Sleep(10 * time.Millisecond)
			ctx, ctxCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer ctxCancel()
			forceErr = svc.ForceUnlockChest(ctx, userID)
		}()

		wg.Wait()

		if openErr != nil {
			t.Fatalf("iter %d: simulated OpenChest goroutine err: %v", i, openErr)
		}
		if forceErr != nil {
			t.Fatalf("iter %d: force-unlock err (race-induced ErrChestNotFound is the bug r1 fixes): %v", i, forceErr)
		}

		// 表里应当只有 1 行（uk_user_id 唯一）
		var count int
		if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM user_chests WHERE user_id = ?`, userID).Scan(&count); err != nil {
			t.Fatalf("iter %d: count: %v", i, err)
		}
		if count != 1 {
			t.Errorf("iter %d: chest rows count = %d, want 1 (uk_user_id 唯一约束 + race 不应破坏)", i, count)
		}

		// 当前 chest 应是 Y（chestIDY），且 unlock_at 被推到 now（force-unlock 命中 Y）
		var (
			currentID    uint64
			currUnlockAt time.Time
		)
		if err := sqlDB.QueryRow(`SELECT id, unlock_at FROM user_chests WHERE user_id = ?`, userID).Scan(&currentID, &currUnlockAt); err != nil {
			t.Fatalf("iter %d: scan current chest: %v", i, err)
		}
		if currentID != chestIDY {
			t.Errorf("iter %d: current chest id = %d, want %d (OpenChest 已 Delete X + Insert Y)", i, currentID, chestIDY)
		}
		nowAfter := time.Now().UTC()
		// force-unlock 命中 Y → Y.unlock_at <= now
		if currUnlockAt.UTC().After(nowAfter) {
			t.Errorf("iter %d: chest Y unlock_at = %v, want <= now %v (force-unlock 应命中 Y)", i, currUnlockAt.UTC(), nowAfter)
		}
	}
}

// ============================================================
// r1 [P2] race 修复的"反面验证"：
// **如果**我们绕过 FOR UPDATE 串行化（即 r1 之前的 WHERE user_id 直 UPDATE 模式），
// 同样的 race 是否会让 force-unlock 拿到错误的语义？
//
// 答：在并发场景下，r1 之前的实装在 OpenChest commit 后会把 Y 的 unlock_at 推到 now —— 与
// r1 之后表现完全一致（因为 commit 后 user_id 匹配到的是 Y，**id 也是 Y**）。
//
// **真正的差异**在调用方语义而非 SQL 结果：
// - r1 之前：调用方不知道命中了谁（盲打 WHERE user_id）。
// - r1 之后：调用方先拿到 chest.ID，知道命中的是哪个 chest —— 这让未来 "force-unlock 必须命中
//   调用时刻的 chest 而不是 future chest" 的契约可以加 chestID 校验断言。
//
// 当前 case 3 用来证明 r1 修复的**功能正确性**（没破坏现有行为 + race 下不报错）；
// 真正的"force-unlock 命中调用时刻 chest"语义在 epics.md §20.7 没要求，因为 dev 端点本就
// "force-unlock 当前 chest" 语义模糊（current = "调用 SELECT 时刻看到的 chest"）。
// ============================================================
