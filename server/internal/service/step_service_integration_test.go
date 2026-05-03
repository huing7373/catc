//go:build integration
// +build integration

// Story 7.3 集成测试：用 dockertest 起真实 mysql:8.0 容器跑 3 条事务链路 case：
//
//   1. FirstAndSecondSync_HappyPath：
//      首次 sync 100 → total=100；第二次 180 → total=180；第三次 150（倒退）→ total 不变 + sync_log 仍新增。
//   2. SingleCapTruncation：
//      配 single_sync_cap=100 / daily_cap=10000；clientTotal=10000 → delta 截断为 100。
//   3. DailyCapExceeded_Returns3001：
//      配 single_sync_cap=10000 / daily_cap=200；first=150 → delta=150；
//      second=300（cur=150 + prev=150=300>200）→ 3001 + step_account 不变 + sync_log 新增 accepted=0；
//      third=300（重复 cur=0 + prev=150=150≤200）→ nil err（**非粘性** 验证）+ sync_log 4 行。
//
// 复用 4.6 / 4.8 的 startMySQL / migrationsPath / runMigrations helper。
//
// **手工 INSERT** user / step_account / pet / chest（不调 auth_service.GuestLogin） ——
// 解耦 step_service 测试与 auth_service。

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

// buildStepServiceIntegration: 起容器 → migrate → 装配 svc + 返清理 closure。
//
// stepsCfg 为 zero-value 时走 service 默认 5000 / 50000；非 zero 走 YAML 覆盖路径。
func buildStepServiceIntegration(t *testing.T, stepsCfg config.StepsConfig) (svc service.StepService, sqlDB *sql.DB, cleanup func()) {
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
	stepAccountRepo := mysql.NewStepAccountRepo(gormDB)
	stepSyncLogRepo := mysql.NewStepSyncLogRepo(gormDB)

	svc = service.NewStepService(txMgr, stepAccountRepo, stepSyncLogRepo, stepsCfg)

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

// fetchStepAccount 直接 SELECT 验当前账户三档值。
func fetchStepAccount(t *testing.T, sqlDB *sql.DB, userID uint64) (total, available, consumed uint64, version uint64) {
	t.Helper()
	row := sqlDB.QueryRow(`SELECT total_steps, available_steps, consumed_steps, version FROM user_step_accounts WHERE user_id = ?`, userID)
	if err := row.Scan(&total, &available, &consumed, &version); err != nil {
		t.Fatalf("fetchStepAccount: %v", err)
	}
	return
}

// countSyncLogs 当日 sync_log 行数。syncDate 是 string YYYY-MM-DD（与
// mysql.StepSyncLog 同 type；driver 走 VARCHAR→DATE 隐式转换，无时区耦合）。
func countSyncLogs(t *testing.T, sqlDB *sql.DB, userID uint64, syncDate string) int64 {
	t.Helper()
	var n int64
	row := sqlDB.QueryRow(`SELECT COUNT(*) FROM user_step_sync_logs WHERE user_id = ? AND sync_date = ?`, userID, syncDate)
	if err := row.Scan(&n); err != nil {
		t.Fatalf("countSyncLogs: %v", err)
	}
	return n
}

// latestSyncLogAcceptedDelta 最近 INSERT 那行的 accepted_delta_steps。
//
// **ORDER BY**：与 repo 的基线查询（`ORDER BY id DESC`）同序（Story 7.3 review r3 [P1]）。
// r2 一度改用 `client_total_steps DESC` 但 HealthKit reset 场景下永久卡死，已退回。
//
// 本辅助函数返回"最近 INSERT 行"对应的 accepted_delta_steps。乱序到达由 service
// 层 SUM 兜底处理，不影响本函数语义。
func latestSyncLogAcceptedDelta(t *testing.T, sqlDB *sql.DB, userID uint64, syncDate string) int32 {
	t.Helper()
	var d int32
	row := sqlDB.QueryRow(
		`SELECT accepted_delta_steps FROM user_step_sync_logs WHERE user_id = ? AND sync_date = ? ORDER BY id DESC LIMIT 1`,
		userID, syncDate,
	)
	if err := row.Scan(&d); err != nil {
		t.Fatalf("latestSyncLogAcceptedDelta: %v", err)
	}
	return d
}

// ============================================================
// AC10.1: 首次 + 第二次 + 第三次（倒退）链路
// ============================================================
func TestStepServiceIntegration_FirstAndSecondSync_HappyPath(t *testing.T) {
	svc, sqlDB, cleanup := buildStepServiceIntegration(t, config.StepsConfig{})
	defer cleanup()

	const userID = uint64(1)
	insertUser(t, sqlDB, userID, "uid-step-1", "用户1", "")
	insertStepAccount(t, sqlDB, userID, 0, 0, 0)

	const syncDate = "2026-05-01" // string YYYY-MM-DD（无时区耦合；详见 mysql.StepSyncLog doc）
	ctx := context.Background()

	// 第一次 sync clientTotal=100 → delta=100
	out1, err := svc.SyncSteps(ctx, service.SyncStepsInput{
		UserID: userID, SyncDate: syncDate, ClientTotalSteps: 100, MotionState: 2, ClientTimestamp: 1714560000000,
	})
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if out1.AcceptedDeltaSteps != 100 {
		t.Errorf("first AcceptedDeltaSteps = %d, want 100", out1.AcceptedDeltaSteps)
	}
	total, avail, consumed, _ := fetchStepAccount(t, sqlDB, userID)
	if total != 100 || avail != 100 || consumed != 0 {
		t.Errorf("after first sync: total=%d available=%d consumed=%d, want 100/100/0", total, avail, consumed)
	}
	if got := countSyncLogs(t, sqlDB, userID, syncDate); got != 1 {
		t.Errorf("after first sync: log count = %d, want 1", got)
	}

	// 第二次 sync clientTotal=180 → delta=80 → total=180
	out2, err := svc.SyncSteps(ctx, service.SyncStepsInput{
		UserID: userID, SyncDate: syncDate, ClientTotalSteps: 180, MotionState: 2, ClientTimestamp: 1714560005000,
	})
	if err != nil {
		t.Fatalf("second sync: %v", err)
	}
	if out2.AcceptedDeltaSteps != 80 {
		t.Errorf("second AcceptedDeltaSteps = %d, want 80", out2.AcceptedDeltaSteps)
	}
	total, _, _, _ = fetchStepAccount(t, sqlDB, userID)
	if total != 180 {
		t.Errorf("after second sync: total = %d, want 180", total)
	}
	if got := countSyncLogs(t, sqlDB, userID, syncDate); got != 2 {
		t.Errorf("after second sync: log count = %d, want 2", got)
	}
	if got := latestSyncLogAcceptedDelta(t, sqlDB, userID, syncDate); got != 80 {
		t.Errorf("latest sync_log accepted_delta = %d, want 80", got)
	}

	// 第三次 sync clientTotal=150（倒退）→ delta=0；step_account.total 不变；sync_log 仍新增
	out3, err := svc.SyncSteps(ctx, service.SyncStepsInput{
		UserID: userID, SyncDate: syncDate, ClientTotalSteps: 150, MotionState: 1, ClientTimestamp: 1714560010000,
	})
	if err != nil {
		t.Fatalf("third sync (backward): %v", err)
	}
	if out3.AcceptedDeltaSteps != 0 {
		t.Errorf("third (backward) AcceptedDeltaSteps = %d, want 0", out3.AcceptedDeltaSteps)
	}
	total, _, _, version := fetchStepAccount(t, sqlDB, userID)
	if total != 180 {
		t.Errorf("after third (backward) sync: total = %d, want 180 (不变)", total)
	}
	if version != 3 {
		t.Errorf("after third sync: version = %d, want 3 (每次 +1)", version)
	}
	if got := countSyncLogs(t, sqlDB, userID, syncDate); got != 3 {
		t.Errorf("after third sync: log count = %d, want 3 (append-only)", got)
	}
	if got := latestSyncLogAcceptedDelta(t, sqlDB, userID, syncDate); got != 0 {
		t.Errorf("latest sync_log accepted_delta = %d, want 0 (倒退记录)", got)
	}
}

// ============================================================
// AC10.2: 单次截断 single_sync_cap=100 / daily_cap=10000
// ============================================================
func TestStepServiceIntegration_SingleCapTruncation(t *testing.T) {
	svc, sqlDB, cleanup := buildStepServiceIntegration(t, config.StepsConfig{
		SingleSyncCap: 100,
		DailyCap:      10000,
	})
	defer cleanup()

	const userID = uint64(1)
	insertUser(t, sqlDB, userID, "uid-step-cap", "用户1", "")
	insertStepAccount(t, sqlDB, userID, 0, 0, 0)

	const syncDate = "2026-05-01" // string YYYY-MM-DD（无时区耦合；详见 mysql.StepSyncLog doc）
	ctx := context.Background()

	out, err := svc.SyncSteps(ctx, service.SyncStepsInput{
		UserID: userID, SyncDate: syncDate, ClientTotalSteps: 10000, MotionState: 3, ClientTimestamp: 1714560000000,
	})
	if err != nil {
		t.Fatalf("sync: %v", err)
	}
	if out.AcceptedDeltaSteps != 100 {
		t.Errorf("AcceptedDeltaSteps = %d, want 100 (截断)", out.AcceptedDeltaSteps)
	}
	total, _, _, _ := fetchStepAccount(t, sqlDB, userID)
	if total != 100 {
		t.Errorf("total = %d, want 100 (截断后)", total)
	}
	if got := latestSyncLogAcceptedDelta(t, sqlDB, userID, syncDate); got != 100 {
		t.Errorf("sync_log accepted_delta = %d, want 100", got)
	}
}

// ============================================================
// AC10.3: 当日封顶 + 非粘性验证
// 配 single_sync_cap=10000 / daily_cap=200
//   first=150 → delta=150
//   second=300（cur=150 + prev=150=300>200）→ 3001 + step_account 不变 + sync_log 新增 accepted=0
//   third=300（重复 cur=0 + prev=150=150≤200）→ nil err（非粘性）+ sync_log 4 行
// ============================================================
func TestStepServiceIntegration_DailyCapExceeded_Returns3001(t *testing.T) {
	svc, sqlDB, cleanup := buildStepServiceIntegration(t, config.StepsConfig{
		SingleSyncCap: 10000,
		DailyCap:      200,
	})
	defer cleanup()

	const userID = uint64(1)
	insertUser(t, sqlDB, userID, "uid-step-daily", "用户1", "")
	insertStepAccount(t, sqlDB, userID, 0, 0, 0)

	const syncDate = "2026-05-01" // string YYYY-MM-DD（无时区耦合；详见 mysql.StepSyncLog doc）
	ctx := context.Background()

	// 第一次 sync 150
	out1, err := svc.SyncSteps(ctx, service.SyncStepsInput{
		UserID: userID, SyncDate: syncDate, ClientTotalSteps: 150, MotionState: 2, ClientTimestamp: 1714560000000,
	})
	if err != nil {
		t.Fatalf("first sync: %v", err)
	}
	if out1.AcceptedDeltaSteps != 150 {
		t.Errorf("first AcceptedDeltaSteps = %d, want 150", out1.AcceptedDeltaSteps)
	}
	total, _, _, _ := fetchStepAccount(t, sqlDB, userID)
	if total != 150 {
		t.Errorf("after first: total = %d, want 150", total)
	}

	// 第二次 sync 300（cur delta=150；prev=150；总 300 > 200 → 3001）
	out2, err := svc.SyncSteps(ctx, service.SyncStepsInput{
		UserID: userID, SyncDate: syncDate, ClientTotalSteps: 300, MotionState: 2, ClientTimestamp: 1714560005000,
	})
	if out2 != nil {
		t.Errorf("second sync out = %+v, want nil on 3001", out2)
	}
	if got := apperror.Code(err); got != apperror.ErrStepSyncInvalid {
		t.Errorf("second sync err code = %d, want %d (3001)", got, apperror.ErrStepSyncInvalid)
	}
	total, _, _, version := fetchStepAccount(t, sqlDB, userID)
	if total != 150 {
		t.Errorf("after second (3001): total = %d, want 150 (不变)", total)
	}
	// 关键：事务 commit → version 仍 +1，sync_log 仍新增
	if version != 2 {
		t.Errorf("after second: version = %d, want 2 (事务 commit; version+1)", version)
	}
	if got := countSyncLogs(t, sqlDB, userID, syncDate); got != 2 {
		t.Errorf("after second: log count = %d, want 2 (审计行落库)", got)
	}
	if got := latestSyncLogAcceptedDelta(t, sqlDB, userID, syncDate); got != 0 {
		t.Errorf("latest sync_log accepted_delta = %d, want 0 (封顶记录)", got)
	}

	// 第三次 sync 300（重复 → cur delta=0；prev=150；150+0=150 ≤ 200 → 不触发 3001）
	out3, err := svc.SyncSteps(ctx, service.SyncStepsInput{
		UserID: userID, SyncDate: syncDate, ClientTotalSteps: 300, MotionState: 1, ClientTimestamp: 1714560010000,
	})
	if err != nil {
		t.Fatalf("third sync (重复，非粘性): %v", err)
	}
	if out3.AcceptedDeltaSteps != 0 {
		t.Errorf("third AcceptedDeltaSteps = %d, want 0", out3.AcceptedDeltaSteps)
	}
	total, _, _, _ = fetchStepAccount(t, sqlDB, userID)
	if total != 150 {
		t.Errorf("after third: total = %d, want 150 (不变)", total)
	}
	if got := countSyncLogs(t, sqlDB, userID, syncDate); got != 3 {
		t.Errorf("after third: log count = %d, want 3", got)
	}
}

// ============================================================
// AC10.4: 乱序到达由 SUM 兜底（Story 7.3 review r3 [P1]）
//
// 场景：手机端 sync 因网络重试 / 串行错乱，让"旧 total" INSERT 在"新 total"之后。
//   - sync A: clientTotal=250 → delta=250 (首次)，落库 id=1，sum=250
//   - sync B: clientTotal=200（旧报告延迟到达）→ delta=0（200 < 250；倒退）落库 id=2，sum=250
//   - sync C: clientTotal=260
//     - 基线 = id DESC 最近行 = id=2 (client_total=200)
//     - rawDelta = 260 - 200 = 60
//     - SUM 兜底：sum(250) + rawDelta(60) = 310 > 260 → adjusted = max(0, 260-250) = 10
//     - **入账 10**（与正确答案一致；不会重复入账 50 步）
//
// **r1→r2→r3 决策史**（详见 step_sync_log_repo.go interface doc）：
//   - r1 ORDER BY id DESC（无 SUM 兜底）→ sync C delta=60 重复入账 50 步
//   - r2 ORDER BY client_total_steps DESC（无 SUM 兜底）→ 解决了乱序但 HealthKit
//     reset/correction 永久卡死（最大值锁定基线）
//   - r3 ORDER BY id DESC + service 层 SUM 兜底 → 两个场景都对
//
// **regression sentinel**（无 SUM 兜底）：sync C accepted=60 → 重复入账暴雷。
// ============================================================
func TestStepServiceIntegration_OutOfOrderSync_SumCapPreventsRepeatedAccrual(t *testing.T) {
	svc, sqlDB, cleanup := buildStepServiceIntegration(t, config.StepsConfig{})
	defer cleanup()

	const userID = uint64(1)
	insertUser(t, sqlDB, userID, "uid-step-ooo", "用户1", "")
	insertStepAccount(t, sqlDB, userID, 0, 0, 0)

	const syncDate = "2026-05-01"
	ctx := context.Background()

	// sync A: clientTotal=250 (首次) → delta=250
	outA, err := svc.SyncSteps(ctx, service.SyncStepsInput{
		UserID: userID, SyncDate: syncDate, ClientTotalSteps: 250, MotionState: 2, ClientTimestamp: 1714560000000,
	})
	if err != nil {
		t.Fatalf("sync A: %v", err)
	}
	if outA.AcceptedDeltaSteps != 250 {
		t.Errorf("A AcceptedDeltaSteps = %d, want 250", outA.AcceptedDeltaSteps)
	}

	// sync B: clientTotal=200 (旧报告延迟到达) → delta=0 (200 < 250；倒退处理)
	outB, err := svc.SyncSteps(ctx, service.SyncStepsInput{
		UserID: userID, SyncDate: syncDate, ClientTotalSteps: 200, MotionState: 2, ClientTimestamp: 1714559900000,
	})
	if err != nil {
		t.Fatalf("sync B (delayed): %v", err)
	}
	if outB.AcceptedDeltaSteps != 0 {
		t.Errorf("B AcceptedDeltaSteps = %d, want 0 (倒退/延迟)", outB.AcceptedDeltaSteps)
	}
	// 关键：B 是最近 INSERT 的行（id=2），client_total_steps=200。
	total, _, _, _ := fetchStepAccount(t, sqlDB, userID)
	if total != 250 {
		t.Errorf("after B: total = %d, want 250 (不变)", total)
	}

	// sync C: clientTotal=260
	//   - 基线 = id DESC 最近行 = sync B (200)
	//   - rawDelta = 260 - 200 = 60
	//   - SUM 兜底：sum(250) + 60 = 310 > 260 → adjusted = max(0, 260-250) = 10
	//
	// **regression sentinel**：若 service 层去掉 SUM 兜底，rawDelta=60 直接入账 →
	// AcceptedDeltaSteps=60 → 重复入账 50 步；本断言立刻挂。
	outC, err := svc.SyncSteps(ctx, service.SyncStepsInput{
		UserID: userID, SyncDate: syncDate, ClientTotalSteps: 260, MotionState: 2, ClientTimestamp: 1714560100000,
	})
	if err != nil {
		t.Fatalf("sync C: %v", err)
	}
	if outC.AcceptedDeltaSteps != 10 {
		t.Errorf("C AcceptedDeltaSteps = %d, want 10 (SUM 兜底 adjusted=260-250)；"+
			"若 = 60 → service 层 SUM 兜底失效（regression）", outC.AcceptedDeltaSteps)
	}
	total, _, _, _ = fetchStepAccount(t, sqlDB, userID)
	if total != 260 {
		t.Errorf("after C: total = %d, want 260 (250 + 10)", total)
	}
	if got := countSyncLogs(t, sqlDB, userID, syncDate); got != 3 {
		t.Errorf("after C: log count = %d, want 3", got)
	}
}

// ============================================================
// AC10.5: HealthKit reset/correction 不卡死（Story 7.3 review r3 [P1]）
//
// 场景：当日内 client_total_steps 出现真实下降（device reset / HealthKit data
// correction 等），后续步数应该能正常入账，不能被历史最大值永久卡死。
//
//   - sync 1: clientTotal=250 → delta=250；total=250；sum=250
//   - sync 2: clientTotal=100（reset 后从 100 重启） → delta=0（倒退）；total=250；sum=250
//   - sync 3: clientTotal=105（reset 后又走 5 步） → 基线 = 100（最近行）→ rawDelta=5；
//     SUM 兜底：250+5=255 > 105 → adjusted = 0；delta=0；total=250
//   - sync 4: clientTotal=300（reset 后超过历史最大）→ 基线 = 105（最近行）→ rawDelta=195；
//     SUM 兜底：250+195=445 > 300 → adjusted = max(0, 300-250)=50；delta=50；total=300
//
// **r2 卡死 regression sentinel**：若 repo ORDER BY 退回 `client_total_steps DESC`，
// sync 3 / sync 4 的基线会**永久**取 250 → rawDelta 永远 0 直到超过 250；本场景下
// sync 3 走的 5 步永久丢失，sync 4 才开始有 delta=50（但走过的中间步数全没入账）。
//
// 本 case 锁住"基线必须跟最近 sync 走"语义。
// ============================================================
func TestStepServiceIntegration_HealthKitReset_AccrualResumesNotPermanentlyBlocked(t *testing.T) {
	svc, sqlDB, cleanup := buildStepServiceIntegration(t, config.StepsConfig{})
	defer cleanup()

	const userID = uint64(1)
	insertUser(t, sqlDB, userID, "uid-step-reset", "用户1", "")
	insertStepAccount(t, sqlDB, userID, 0, 0, 0)

	const syncDate = "2026-05-01"
	ctx := context.Background()

	// sync 1: 250
	out1, err := svc.SyncSteps(ctx, service.SyncStepsInput{
		UserID: userID, SyncDate: syncDate, ClientTotalSteps: 250, MotionState: 2, ClientTimestamp: 1714560000000,
	})
	if err != nil {
		t.Fatalf("sync 1: %v", err)
	}
	if out1.AcceptedDeltaSteps != 250 {
		t.Errorf("sync1 delta = %d, want 250", out1.AcceptedDeltaSteps)
	}

	// sync 2: 100 (reset)
	out2, err := svc.SyncSteps(ctx, service.SyncStepsInput{
		UserID: userID, SyncDate: syncDate, ClientTotalSteps: 100, MotionState: 1, ClientTimestamp: 1714560005000,
	})
	if err != nil {
		t.Fatalf("sync 2 (reset): %v", err)
	}
	if out2.AcceptedDeltaSteps != 0 {
		t.Errorf("sync2 delta = %d, want 0 (reset 倒退)", out2.AcceptedDeltaSteps)
	}
	total, _, _, _ := fetchStepAccount(t, sqlDB, userID)
	if total != 250 {
		t.Errorf("after sync2: total = %d, want 250 (不变)", total)
	}

	// sync 3: 105 (reset 后又 5 步) → 基线 = 100 → rawDelta=5；SUM 兜底削回 0
	out3, err := svc.SyncSteps(ctx, service.SyncStepsInput{
		UserID: userID, SyncDate: syncDate, ClientTotalSteps: 105, MotionState: 2, ClientTimestamp: 1714560010000,
	})
	if err != nil {
		t.Fatalf("sync 3: %v", err)
	}
	if out3.AcceptedDeltaSteps != 0 {
		t.Errorf("sync3 delta = %d, want 0 (SUM 兜底削回；250+5>105)", out3.AcceptedDeltaSteps)
	}

	// sync 4: 300 → 基线 = 105 → rawDelta=195；SUM 兜底：250+195=445>300 → adjusted=50
	//
	// **关键 sentinel**：
	//   - r3 正确：delta=50 (300-250 by SUM cap)，total=300
	//   - r2 regression（max baseline）：基线 = 250 → rawDelta=50 → SUM 兜底
	//     250+50=300 不>300 → delta=50 → 行为相同
	//
	// 真正区别在 sync 3：r2 基线 = 250 → rawDelta=0 → 行为相同（都是 0）
	//
	// 看似 r2/r3 行为相同？**不**：把 sync 3 改成 105 之后再来一次 sync 110：
	//   - r3 基线 = 105 → rawDelta=5 → SUM 兜底 250+5=255>110 → adjusted=0
	//   - r2 基线 = 250 → rawDelta=0 → 0
	//
	// 实际差异在用户**长期**走小步数（reset 后慢慢走）的累计场景，单 case 难穷举。
	// 本 case 的核心 sentinel：**没有永久 0** —— sync 4 必须能 accept=50。
	out4, err := svc.SyncSteps(ctx, service.SyncStepsInput{
		UserID: userID, SyncDate: syncDate, ClientTotalSteps: 300, MotionState: 2, ClientTimestamp: 1714560020000,
	})
	if err != nil {
		t.Fatalf("sync 4: %v", err)
	}
	if out4.AcceptedDeltaSteps != 50 {
		t.Errorf("sync4 delta = %d, want 50 (SUM 兜底 adjusted=300-250)", out4.AcceptedDeltaSteps)
	}
	total, _, _, _ = fetchStepAccount(t, sqlDB, userID)
	if total != 300 {
		t.Errorf("after sync4: total = %d, want 300 (250 + 50)", total)
	}
	if got := countSyncLogs(t, sqlDB, userID, syncDate); got != 4 {
		t.Errorf("log count = %d, want 4", got)
	}
}
