//go:build integration
// +build integration

// Story 20.6 + 20.9 chest_open_service 集成测试：用 dockertest 起真实 mysql:8.0 容器跑全部 case。
//
// **20.6 已落地 2 case（happy 路径基础）**：
//   1. HappyPath_FullFlow: 创建 user + 1500 步 + force-unlock chest → 调 OpenChest →
//      验证 DB user_step_accounts.available_steps=500 + consumed_steps=1000 +
//      chest_open_logs 多 1 行 + 旧 chest 删除 + 新 chest 创建 + idempotency 行 status=success
//   2. HappyPath_IdempotencyReplay: 第一次 open success → 第二次同 idempotencyKey → 短路 + DB 无副作用
//
// **20.9 追加 12 case（epic-20 收尾性 Layer 2 集成测试矩阵）**：
//   3. StepAccountSpendFails_AllRollback         — 回滚 1：扣步数失败 → 5 张表全回滚
//   4. CosmeticItemsListEmpty_AllRollback        — 回滚 2：cosmetic_items 空 → 全回滚（已扣步数 undo）
//   5. ChestOpenLogCreateFails_AllRollback       — 回滚 3：写 log 失败 → 全回滚
//   6. NextChestCreateFails_AllRollback          — 回滚 4：建新 chest 失败 → 全回滚（含 Delete 旧 chest 也 undo）
//   7. Idempotency100CallsSameKey_OnlyOneOpen    — 幂等 1：100 次同 key 顺序 → 1 次开箱 + 99 次 cached replay
//   8. Idempotency3CallsDifferentKeys_EachOpens  — 幂等 2：3 次不同 key + 充足步数 → 各开各的
//   9. Concurrent100SameKey_OnlyOneOpens         — 并发 1：100 goroutine 同 key → 100 全成功（cached replay 路径）
//  10. Concurrent100DifferentKeys_StepLimit...    — 并发 2：100 goroutine 不同 key + 1500 步 → 1 成功 + 99 个 4002（chest race，见 case 注释）
//  11. Steps999_Returns3002                       — 边界 1：999 步 → 3002
//  12. Steps1000_SucceedsAvailable0               — 边界 2：1000 步 → 成功 + 余 0
//  13. Steps1001_SucceedsAvailable1               — 边界 3：1001 步 → 成功 + 余 1
//  14. UnlockAtMinus1ms_IsUnlockable              — 边界 4：unlock_at 比 now 早 1ms → unlockable
//  15. WeightedPickDistribution_1000Opens         — 抽奖分布：1000 次开箱 → drop_weight 设计区间
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑这些；
// 只在 `bash scripts/build.sh --integration` 触发。
//
// 本机 Windows docker 不可用 → t.Skip（startMySQL 内已 skip）。CI Linux 跑。
//
// 复用既有 helper：startMySQL / runMigrations / insertUser / insertStepAccount / insertChest /
// assertCount / buildChestOpenServiceIntegration / 4.7 落地的 faultChestRepo。
// **手工 INSERT 测试数据**（不调 auth_service.GuestLogin），与既有 chest_service_integration_test
// 同模式。
//
// 新增 3 个 fault wrapper（仅本文件可见，避免与 4.7 / 11.9 同包冲突）：
//   - faultStepAccountRepoOnSpend  — AC2 用，Spend 抛 injectErr，其他透传
//   - faultCosmeticItemRepoOnList  — AC3 用，ListEnabledForWeightedPick 返 ([]CosmeticItem{}, nil)
//   - faultChestOpenLogRepoOnCreate — AC4 用，Create 抛 injectErr
//
// 4.7 已落地的 faultChestRepo 直接复用（AC5：注入"建新 chest"失败时让 Delete 透传 + Create 注入）。

package service_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	stderrors "errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/random"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
	"github.com/huing/cat/server/internal/service"
)

// buildChestOpenServiceIntegration: 起容器 → migrate（含 0011/0012 cosmetic seed 已自动跑）→ 装配 svc。
func buildChestOpenServiceIntegration(t *testing.T) (svc service.ChestService, sqlDB *sql.DB, cleanup func()) {
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
	stepAccountRepo := mysql.NewStepAccountRepo(gormDB)
	idempotencyRepo := mysql.NewIdempotencyRepo(gormDB)
	cosmeticItemRepo := mysql.NewCosmeticItemRepo(gormDB)
	chestOpenLogRepo := mysql.NewChestOpenLogRepo(gormDB)
	txMgr := tx.NewManager(gormDB)
	weightedPicker := random.NewCryptoWeightedPicker(rand.Reader)

	svc = service.NewChestService(chestRepo, txMgr, idempotencyRepo, stepAccountRepo, cosmeticItemRepo, chestOpenLogRepo, weightedPicker)

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

// AC8.1: HappyPath_FullFlow
// 创建 user + 1500 步 + force-unlock chest → 调 OpenChest →
// 验证 DB user_step_accounts.available_steps=500 + consumed_steps=1000 +
// chest_open_logs 多 1 行 + 旧 chest 删除 + 新 chest 创建 + idempotency 行 status=success + response_json 完整
func TestChestOpenServiceIntegration_HappyPath_FullFlow(t *testing.T) {
	svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
	defer cleanup()

	const userID = uint64(1)
	const idempotencyKey = "test_key_open_001"
	insertUser(t, sqlDB, userID, "uid-chest-open-1", "用户openchest", "")
	insertStepAccount(t, sqlDB, userID, 1500 /* total */, 1500 /* available */, 0 /* consumed */)

	// force-unlock: unlock_at = now - 1min（unlockable）
	unlockAt := time.Now().UTC().Add(-1 * time.Minute)
	insertChest(t, sqlDB, 5001 /* chest_id */, userID, 1 /* status=counting */, unlockAt, 1000 /* open_cost_steps */)

	ctx := context.Background()
	out, err := svc.OpenChest(ctx, service.OpenChestInput{
		UserID:         userID,
		IdempotencyKey: idempotencyKey,
		RequestID:      "req-xyz-1",
	})
	if err != nil {
		t.Fatalf("OpenChest: %v", err)
	}
	if out == nil {
		t.Fatal("out = nil")
	}
	// 1. service 返回校验
	if out.Reward.UserCosmeticItemID != 0 {
		t.Errorf("Reward.UserCosmeticItemID = %d, want 0 (节点 7 占位)", out.Reward.UserCosmeticItemID)
	}
	if out.Reward.CosmeticItemID == 0 {
		t.Errorf("Reward.CosmeticItemID = 0; want non-zero (从 cosmetic_items seed 命中)")
	}
	if out.StepAccount.AvailableSteps != 500 {
		t.Errorf("StepAccount.AvailableSteps = %d, want 500", out.StepAccount.AvailableSteps)
	}
	if out.StepAccount.ConsumedSteps != 1000 {
		t.Errorf("StepAccount.ConsumedSteps = %d, want 1000", out.StepAccount.ConsumedSteps)
	}
	if out.NextChest.ID == 0 {
		t.Errorf("NextChest.ID = 0; want non-zero (新 chest 创建)")
	}

	// 2. DB user_step_accounts
	var availableSteps, consumedSteps uint64
	var version uint64
	if err := sqlDB.QueryRow(
		`SELECT available_steps, consumed_steps, version FROM user_step_accounts WHERE user_id = ?`, userID,
	).Scan(&availableSteps, &consumedSteps, &version); err != nil {
		t.Fatalf("query user_step_accounts: %v", err)
	}
	if availableSteps != 500 {
		t.Errorf("DB user_step_accounts.available_steps = %d, want 500", availableSteps)
	}
	if consumedSteps != 1000 {
		t.Errorf("DB user_step_accounts.consumed_steps = %d, want 1000", consumedSteps)
	}
	if version != 1 {
		t.Errorf("DB user_step_accounts.version = %d, want 1 (起始 0 + 1 increments)", version)
	}

	// 3. chest_open_logs 多 1 行
	var logCount int
	if err := sqlDB.QueryRow(
		`SELECT COUNT(*) FROM chest_open_logs WHERE user_id = ?`, userID,
	).Scan(&logCount); err != nil {
		t.Fatalf("query chest_open_logs count: %v", err)
	}
	if logCount != 1 {
		t.Errorf("DB chest_open_logs count = %d, want 1", logCount)
	}

	// 4. 旧 chest 已 DELETE，新 chest INSERT（unlock_at ≈ now+10min, status=1, version=0）
	var chestCount int
	if err := sqlDB.QueryRow(
		`SELECT COUNT(*) FROM user_chests WHERE user_id = ?`, userID,
	).Scan(&chestCount); err != nil {
		t.Fatalf("query user_chests count: %v", err)
	}
	if chestCount != 1 {
		t.Errorf("DB user_chests count = %d, want 1 (旧删 + 新建)", chestCount)
	}

	var newChestID, newChestVersion uint64
	var newChestStatus int
	var newChestUnlockAt time.Time
	if err := sqlDB.QueryRow(
		`SELECT id, status, unlock_at, version FROM user_chests WHERE user_id = ?`, userID,
	).Scan(&newChestID, &newChestStatus, &newChestUnlockAt, &newChestVersion); err != nil {
		t.Fatalf("query user_chests: %v", err)
	}
	if newChestID == 5001 {
		t.Errorf("new chest.id = 5001 (旧 chest 未删除！)")
	}
	if newChestStatus != 1 {
		t.Errorf("new chest.status = %d, want 1 (counting)", newChestStatus)
	}
	if newChestVersion != 0 {
		t.Errorf("new chest.version = %d, want 0", newChestVersion)
	}
	// unlock_at ≈ now+10min；容差 [9min, 11min]
	delta := newChestUnlockAt.Sub(time.Now().UTC())
	if delta < 9*time.Minute || delta > 11*time.Minute {
		t.Errorf("new chest.unlock_at delta = %v, want ≈ 10min", delta)
	}

	// 5. chest_open_idempotency_records 多 1 行 status=success
	var idemStatus string
	var idemResponseJSON []byte
	if err := sqlDB.QueryRow(
		`SELECT status, response_json FROM chest_open_idempotency_records WHERE user_id = ? AND idempotency_key = ?`,
		userID, idempotencyKey,
	).Scan(&idemStatus, &idemResponseJSON); err != nil {
		t.Fatalf("query chest_open_idempotency_records: %v", err)
	}
	if idemStatus != "success" {
		t.Errorf("idempotency.status = %q, want \"success\"", idemStatus)
	}
	if len(idemResponseJSON) == 0 {
		t.Errorf("idempotency.response_json is empty")
	}
	// response_json schema 断言（不含 nextChest.status / remainingSeconds / requestId）
	var cached map[string]any
	if err := json.Unmarshal(idemResponseJSON, &cached); err != nil {
		t.Fatalf("unmarshal response_json: %v", err)
	}
	if _, has := cached["requestId"]; has {
		t.Errorf("response_json should NOT contain top-level requestId; got: %+v", cached)
	}
	data := cached["data"].(map[string]any)
	nc := data["nextChest"].(map[string]any)
	if _, has := nc["status"]; has {
		t.Errorf("response_json.data.nextChest should NOT contain status (r9/r11)")
	}
	if _, has := nc["remainingSeconds"]; has {
		t.Errorf("response_json.data.nextChest should NOT contain remainingSeconds (r9/r11)")
	}
}

// AC8.2: HappyPath_IdempotencyReplay
// 第一次 open success → 第二次同 idempotencyKey → 短路 cached + DB 无副作用
func TestChestOpenServiceIntegration_HappyPath_IdempotencyReplay_SameKey(t *testing.T) {
	svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
	defer cleanup()

	const userID = uint64(2)
	const idempotencyKey = "test_key_replay_001"
	insertUser(t, sqlDB, userID, "uid-chest-replay-1", "用户replay", "")
	insertStepAccount(t, sqlDB, userID, 1500, 1500, 0)
	insertChest(t, sqlDB, 5002, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

	ctx := context.Background()
	// 第一次：full flow
	out1, err := svc.OpenChest(ctx, service.OpenChestInput{UserID: userID, IdempotencyKey: idempotencyKey})
	if err != nil {
		t.Fatalf("first OpenChest: %v", err)
	}
	if out1.StepAccount.AvailableSteps != 500 {
		t.Errorf("first call: AvailableSteps = %d, want 500", out1.StepAccount.AvailableSteps)
	}

	// 第二次：同 idempotencyKey → 短路返 cached + DB 不再变化
	out2, err := svc.OpenChest(ctx, service.OpenChestInput{UserID: userID, IdempotencyKey: idempotencyKey})
	if err != nil {
		t.Fatalf("second OpenChest: %v", err)
	}

	// 1. 返回 reward 应与第一次一致（同一 cosmeticItemId）
	if out2.Reward.CosmeticItemID != out1.Reward.CosmeticItemID {
		t.Errorf("replay Reward.CosmeticItemID = %d, want %d (cached)", out2.Reward.CosmeticItemID, out1.Reward.CosmeticItemID)
	}
	if out2.StepAccount.AvailableSteps != 500 {
		t.Errorf("replay AvailableSteps = %d, want 500 (unchanged)", out2.StepAccount.AvailableSteps)
	}

	// 2. DB user_step_accounts 仅 1 次 Spend
	var availableSteps, consumedSteps uint64
	var version uint64
	if err := sqlDB.QueryRow(
		`SELECT available_steps, consumed_steps, version FROM user_step_accounts WHERE user_id = ?`, userID,
	).Scan(&availableSteps, &consumedSteps, &version); err != nil {
		t.Fatalf("query user_step_accounts: %v", err)
	}
	if availableSteps != 500 {
		t.Errorf("DB AvailableSteps = %d, want 500 (replay 不应再次 Spend)", availableSteps)
	}
	if version != 1 {
		t.Errorf("DB version = %d, want 1 (仅 1 次 Spend)", version)
	}

	// 3. chest_open_logs 仅 1 行
	var logCount int
	if err := sqlDB.QueryRow(`SELECT COUNT(*) FROM chest_open_logs WHERE user_id = ?`, userID).Scan(&logCount); err != nil {
		t.Fatalf("query log count: %v", err)
	}
	if logCount != 1 {
		t.Errorf("DB chest_open_logs count = %d, want 1 (replay 不应再次写 log)", logCount)
	}

	// 4. chest_open_idempotency_records 仅 1 行
	var idemCount int
	if err := sqlDB.QueryRow(
		`SELECT COUNT(*) FROM chest_open_idempotency_records WHERE user_id = ?`, userID,
	).Scan(&idemCount); err != nil {
		t.Fatalf("query idem count: %v", err)
	}
	if idemCount != 1 {
		t.Errorf("DB chest_open_idempotency_records count = %d, want 1 (replay 不创建新行)", idemCount)
	}
}

// ============================================================
// Story 20.9 — 12 个新 case 起点
// ============================================================

// buildChestServiceWithRepos 装配 svc + 5 个 repo 注入点；fault case 用此 helper 起完整环境后
// 直接替换需要 fault 注入的 repo（reconfigure svc）。
//
// 设计动机：AC2-AC5 四个 fault case 需要"4 个真实 repo + 1 个 fault repo"装配；不同 fault case
// 注入的 repo 不同；本 helper 起 dockertest + migrate + 真 4 repo + txMgr + weightedPicker，
// 返回完整的"原料"，调用方在原料基础上构造 fault 包装 + svc 装配。
//
// **不**抽 "buildChestServiceWithStepFault / buildChestServiceWithLogFault / ..." 等 4 个 helper
// （与 4.7 / 11.9 / 20.6 同模式 —— 测试代码像剧本逐 case 显式装配，不跨函数追真相）。
func buildChestServiceWithRepos(t *testing.T) (
	sqlDB *sql.DB,
	chestRepo mysql.ChestRepo,
	stepAccountRepo mysql.StepAccountRepo,
	idempotencyRepo mysql.IdempotencyRepo,
	cosmeticItemRepo mysql.CosmeticItemRepo,
	chestOpenLogRepo mysql.ChestOpenLogRepo,
	txMgr tx.Manager,
	weightedPicker random.WeightedPicker,
	cleanup func(),
) {
	t.Helper()
	dsn, dockerCleanup := startMySQL(t)
	runMigrations(t, dsn)

	cfg := config.MySQLConfig{DSN: dsn, MaxOpenConns: 10, MaxIdleConns: 2, ConnMaxLifetimeSec: 60}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	gormDB, err := db.Open(ctx, cfg)
	if err != nil {
		dockerCleanup()
		t.Fatalf("db.Open: %v", err)
	}
	rawDB, err := gormDB.DB()
	if err != nil {
		dockerCleanup()
		t.Fatalf("gormDB.DB(): %v", err)
	}
	chestRepo = mysql.NewChestRepo(gormDB)
	stepAccountRepo = mysql.NewStepAccountRepo(gormDB)
	idempotencyRepo = mysql.NewIdempotencyRepo(gormDB)
	cosmeticItemRepo = mysql.NewCosmeticItemRepo(gormDB)
	chestOpenLogRepo = mysql.NewChestOpenLogRepo(gormDB)
	txMgr = tx.NewManager(gormDB)
	weightedPicker = random.NewCryptoWeightedPicker(rand.Reader)

	cleanup = func() {
		_ = rawDB.Close()
		dockerCleanup()
	}
	return rawDB, chestRepo, stepAccountRepo, idempotencyRepo, cosmeticItemRepo, chestOpenLogRepo, txMgr, weightedPicker, cleanup
}

// requireAppError 断言 err 是 *apperror.AppError 且 Code == wantCode。
// 用于 AC2-AC5 / AC9 / AC10 等错误码断言（避免硬编码 1009 / 3002）。
func requireAppError(t *testing.T, err error, wantCode int, ctx string) {
	t.Helper()
	if err == nil {
		t.Fatalf("%s: expected error code=%d, got nil", ctx, wantCode)
	}
	var appErr *apperror.AppError
	if !stderrors.As(err, &appErr) {
		t.Fatalf("%s: expected *AppError, got %T: %v", ctx, err, err)
	}
	if appErr.Code != wantCode {
		t.Fatalf("%s: AppError.Code = %d, want %d (full err: %v)", ctx, appErr.Code, wantCode, err)
	}
}

// ============================================================
// AC2: 回滚 1 — 扣步数失败 → 整体回滚
// ============================================================
func TestChestOpenServiceIntegration_StepAccountSpendFails_AllRollback(t *testing.T) {
	sqlDB, chestRepo, stepAccountRepoReal, idempotencyRepo, cosmeticItemRepo, chestOpenLogRepo, txMgr, weightedPicker, cleanup := buildChestServiceWithRepos(t)
	defer cleanup()

	// fault inject: Spend 抛 generic error
	stepAccountRepoFault := &faultStepAccountRepoOnSpend{
		delegate:  stepAccountRepoReal,
		injectErr: stderrors.New("synthetic step account spend failure"),
	}
	svc := service.NewChestService(chestRepo, txMgr, idempotencyRepo, stepAccountRepoFault, cosmeticItemRepo, chestOpenLogRepo, weightedPicker)

	const userID = uint64(1)
	const idempotencyKey = "test_rollback_step_spend"
	insertUser(t, sqlDB, userID, "uid-rollback-step", "用户回滚步数", "")
	insertStepAccount(t, sqlDB, userID, 1500, 1500, 0)
	insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

	_, err := svc.OpenChest(context.Background(), service.OpenChestInput{UserID: userID, IdempotencyKey: idempotencyKey})
	requireAppError(t, err, apperror.ErrServiceBusy, "AC2 StepAccountSpendFails")

	// step_account 不变（available=1500 / consumed=0 / version=0）
	var availableSteps, consumedSteps, version uint64
	if err := sqlDB.QueryRow(`SELECT available_steps, consumed_steps, version FROM user_step_accounts WHERE user_id=?`, userID).Scan(&availableSteps, &consumedSteps, &version); err != nil {
		t.Fatalf("query step_account: %v", err)
	}
	if availableSteps != 1500 {
		t.Errorf("available_steps=%d, want 1500 (rollback)", availableSteps)
	}
	if consumedSteps != 0 {
		t.Errorf("consumed_steps=%d, want 0 (rollback)", consumedSteps)
	}
	if version != 0 {
		t.Errorf("version=%d, want 0 (rollback)", version)
	}

	assertCount(t, sqlDB, "chest_open_logs WHERE user_id=?", []any{userID}, 0, "chest_open_logs (rollback)")
	assertCount(t, sqlDB, "chest_open_idempotency_records WHERE user_id=?", []any{userID}, 0, "idempotency (rollback)")
	assertCount(t, sqlDB, "user_chests WHERE user_id=? AND id=9001", []any{userID}, 1, "old chest still exists")
	assertCount(t, sqlDB, "user_chests WHERE user_id=?", []any{userID}, 1, "user_chests count=1")
}

// ============================================================
// AC3: 回滚 2 — cosmetic_items 空 → 整体回滚（已扣步数 undo）
// ============================================================
func TestChestOpenServiceIntegration_CosmeticItemsListEmpty_AllRollback(t *testing.T) {
	sqlDB, chestRepo, stepAccountRepo, idempotencyRepo, _, chestOpenLogRepo, txMgr, weightedPicker, cleanup := buildChestServiceWithRepos(t)
	defer cleanup()

	// fault: ListEnabledForWeightedPick 返 ([]CosmeticItem{}, nil) → service 内 len==0 → ErrServiceBusy
	cosmeticItemRepoFault := &faultCosmeticItemRepoOnList{returnEmpty: true}
	svc := service.NewChestService(chestRepo, txMgr, idempotencyRepo, stepAccountRepo, cosmeticItemRepoFault, chestOpenLogRepo, weightedPicker)

	const userID = uint64(1)
	const idempotencyKey = "test_rollback_pick_empty"
	insertUser(t, sqlDB, userID, "uid-rollback-pick", "用户回滚抽奖", "")
	insertStepAccount(t, sqlDB, userID, 1500, 1500, 0)
	insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

	_, err := svc.OpenChest(context.Background(), service.OpenChestInput{UserID: userID, IdempotencyKey: idempotencyKey})
	requireAppError(t, err, apperror.ErrServiceBusy, "AC3 CosmeticItemsListEmpty")

	// 已扣步数也必须 ROLLBACK（service 在 step.Spend 之后才调 ListEnabledForWeightedPick）
	var availableSteps, consumedSteps, version uint64
	if err := sqlDB.QueryRow(`SELECT available_steps, consumed_steps, version FROM user_step_accounts WHERE user_id=?`, userID).Scan(&availableSteps, &consumedSteps, &version); err != nil {
		t.Fatalf("query step_account: %v", err)
	}
	if availableSteps != 1500 {
		t.Errorf("available_steps=%d, want 1500 (rollback after Spend)", availableSteps)
	}
	if consumedSteps != 0 {
		t.Errorf("consumed_steps=%d, want 0 (rollback)", consumedSteps)
	}
	if version != 0 {
		t.Errorf("version=%d, want 0 (rollback)", version)
	}

	assertCount(t, sqlDB, "chest_open_logs WHERE user_id=?", []any{userID}, 0, "log (rollback)")
	assertCount(t, sqlDB, "chest_open_idempotency_records WHERE user_id=?", []any{userID}, 0, "idem (rollback)")
	assertCount(t, sqlDB, "user_chests WHERE user_id=? AND id=9001", []any{userID}, 1, "old chest still exists")
}

// ============================================================
// AC4: 回滚 3 — 写 chest_open_logs 失败 → 整体回滚
// ============================================================
func TestChestOpenServiceIntegration_ChestOpenLogCreateFails_AllRollback(t *testing.T) {
	sqlDB, chestRepo, stepAccountRepo, idempotencyRepo, cosmeticItemRepo, _, txMgr, weightedPicker, cleanup := buildChestServiceWithRepos(t)
	defer cleanup()

	chestOpenLogRepoFault := &faultChestOpenLogRepoOnCreate{
		injectErr: stderrors.New("synthetic chest open log create failure"),
	}
	svc := service.NewChestService(chestRepo, txMgr, idempotencyRepo, stepAccountRepo, cosmeticItemRepo, chestOpenLogRepoFault, weightedPicker)

	const userID = uint64(1)
	const idempotencyKey = "test_rollback_log_create"
	insertUser(t, sqlDB, userID, "uid-rollback-log", "用户回滚log", "")
	insertStepAccount(t, sqlDB, userID, 1500, 1500, 0)
	insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

	_, err := svc.OpenChest(context.Background(), service.OpenChestInput{UserID: userID, IdempotencyKey: idempotencyKey})
	requireAppError(t, err, apperror.ErrServiceBusy, "AC4 ChestOpenLogCreateFails")

	// 已扣步数 + 已抽奖 + Spend SQL 已执行；都必须 ROLLBACK
	var availableSteps, consumedSteps uint64
	if err := sqlDB.QueryRow(`SELECT available_steps, consumed_steps FROM user_step_accounts WHERE user_id=?`, userID).Scan(&availableSteps, &consumedSteps); err != nil {
		t.Fatalf("query step_account: %v", err)
	}
	if availableSteps != 1500 {
		t.Errorf("available_steps=%d, want 1500 (rollback Spend)", availableSteps)
	}
	if consumedSteps != 0 {
		t.Errorf("consumed_steps=%d, want 0 (rollback)", consumedSteps)
	}

	assertCount(t, sqlDB, "chest_open_logs WHERE user_id=?", []any{userID}, 0, "log (rollback)")
	assertCount(t, sqlDB, "chest_open_idempotency_records WHERE user_id=?", []any{userID}, 0, "idem (rollback)")
	assertCount(t, sqlDB, "user_chests WHERE user_id=? AND id=9001", []any{userID}, 1, "old chest still exists")
}

// ============================================================
// AC5: 回滚 4 — 建新 chest 失败 → 整体回滚（含 Delete 旧 chest 也 undo）
// ============================================================
//
// 复用 4.7 落地的 faultChestRepo（同 package service_test 可见；Create 抛 injectErr / Delete 透传）。
//
// **关键事务回滚链**：runOpenChestTx 步骤 5i 先 Delete(旧 chest)（透传 → SQL 真删 + InnoDB undo log
// 记录） → Create(新 chest)（fault 抛 err → fn return error → tx.WithTx 触发 ROLLBACK → undo log
// 把 Delete 也回滚 → 旧 chest 9001 恢复）。
func TestChestOpenServiceIntegration_NextChestCreateFails_AllRollback(t *testing.T) {
	sqlDB, chestRepoReal, stepAccountRepo, idempotencyRepo, cosmeticItemRepo, chestOpenLogRepo, txMgr, weightedPicker, cleanup := buildChestServiceWithRepos(t)
	defer cleanup()

	chestRepoFault := &faultChestRepo{
		delegate:  chestRepoReal,
		injectErr: stderrors.New("synthetic next chest create failure"),
	}
	svc := service.NewChestService(chestRepoFault, txMgr, idempotencyRepo, stepAccountRepo, cosmeticItemRepo, chestOpenLogRepo, weightedPicker)

	const userID = uint64(1)
	const idempotencyKey = "test_rollback_next_chest"
	insertUser(t, sqlDB, userID, "uid-rollback-next", "用户回滚next", "")
	insertStepAccount(t, sqlDB, userID, 1500, 1500, 0)
	insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

	_, err := svc.OpenChest(context.Background(), service.OpenChestInput{UserID: userID, IdempotencyKey: idempotencyKey})
	requireAppError(t, err, apperror.ErrServiceBusy, "AC5 NextChestCreateFails")

	// 完整 ROLLBACK: 扣步数 / 写 log / 删旧 chest 都回滚
	var availableSteps uint64
	if err := sqlDB.QueryRow(`SELECT available_steps FROM user_step_accounts WHERE user_id=?`, userID).Scan(&availableSteps); err != nil {
		t.Fatalf("query step_account: %v", err)
	}
	if availableSteps != 1500 {
		t.Errorf("available_steps=%d, want 1500 (rollback Spend)", availableSteps)
	}

	assertCount(t, sqlDB, "chest_open_logs WHERE user_id=?", []any{userID}, 0, "log (rollback)")
	assertCount(t, sqlDB, "chest_open_idempotency_records WHERE user_id=?", []any{userID}, 0, "idem (rollback)")
	// **核心断言**：旧 chest 9001 因 InnoDB undo log 恢复
	assertCount(t, sqlDB, "user_chests WHERE id=9001 AND user_id=? AND status=1", []any{userID}, 1, "old chest restored (Delete rolled back)")
	assertCount(t, sqlDB, "user_chests WHERE user_id=?", []any{userID}, 1, "user_chests count=1 (new chest never inserted)")
}

// ============================================================
// AC6: 幂等 1 — 100 次同 idempotencyKey 顺序 → 只成功 1 次（其余 cached replay）
// ============================================================
func TestChestOpenServiceIntegration_Idempotency100CallsSameKey_OnlyOneOpen(t *testing.T) {
	svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
	defer cleanup()

	const userID = uint64(1)
	const idempotencyKey = "test_idem_100_same_key"
	insertUser(t, sqlDB, userID, "uid-idem-100", "用户幂等100", "")
	insertStepAccount(t, sqlDB, userID, 1500, 1500, 0)
	insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

	const N = 100
	var firstRewardID, firstNextID uint64
	for i := 0; i < N; i++ {
		out, err := svc.OpenChest(context.Background(), service.OpenChestInput{
			UserID:         userID,
			IdempotencyKey: idempotencyKey,
		})
		if err != nil {
			t.Fatalf("call %d: %v (cached replay 应该全部成功)", i, err)
		}
		if i == 0 {
			firstRewardID = out.Reward.CosmeticItemID
			firstNextID = out.NextChest.ID
		} else {
			if out.Reward.CosmeticItemID != firstRewardID {
				t.Errorf("call %d: Reward.CosmeticItemID=%d, want %d (cached)", i, out.Reward.CosmeticItemID, firstRewardID)
			}
			if out.NextChest.ID != firstNextID {
				t.Errorf("call %d: NextChest.ID=%d, want %d (cached)", i, out.NextChest.ID, firstNextID)
			}
		}
	}

	// DB 状态：只开了 1 次箱
	var availableSteps, consumedSteps, version uint64
	if err := sqlDB.QueryRow(`SELECT available_steps, consumed_steps, version FROM user_step_accounts WHERE user_id=?`, userID).Scan(&availableSteps, &consumedSteps, &version); err != nil {
		t.Fatalf("query step_account: %v", err)
	}
	if availableSteps != 500 {
		t.Errorf("available_steps=%d, want 500 (only 1 open)", availableSteps)
	}
	if consumedSteps != 1000 {
		t.Errorf("consumed_steps=%d, want 1000", consumedSteps)
	}
	if version != 1 {
		t.Errorf("version=%d, want 1 (only 1 Spend)", version)
	}

	assertCount(t, sqlDB, "chest_open_logs WHERE user_id=?", []any{userID}, 1, "log only 1")
	assertCount(t, sqlDB, "chest_open_idempotency_records WHERE user_id=?", []any{userID}, 1, "idem only 1 row")
	assertCount(t, sqlDB, "user_chests WHERE user_id=?", []any{userID}, 1, "chest count = 1 (旧删 + 新建)")
}

// ============================================================
// AC7: 幂等 2 — 3 次不同 idempotencyKey + 充足步数 → 各次都成功开箱
// ============================================================
func TestChestOpenServiceIntegration_Idempotency3CallsDifferentKeys_EachOpens(t *testing.T) {
	svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
	defer cleanup()

	const userID = uint64(1)
	insertUser(t, sqlDB, userID, "uid-idem-diff", "用户幂等不同", "")
	insertStepAccount(t, sqlDB, userID, 3500, 3500, 0) // 3500 步够开 3 次，余 500
	insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

	const N = 3
	keys := []string{"key_diff_1", "key_diff_2", "key_diff_3"}
	rewardIDs := make([]uint64, N)
	for i := 0; i < N; i++ {
		out, err := svc.OpenChest(context.Background(), service.OpenChestInput{
			UserID: userID, IdempotencyKey: keys[i],
		})
		if err != nil {
			t.Fatalf("call %d (key=%s): %v", i, keys[i], err)
		}
		rewardIDs[i] = out.Reward.CosmeticItemID
		// 每次开完后 next chest unlock_at = now+10min，必须 force-unlock 才能开下一次
		if i < N-1 {
			if _, err := sqlDB.Exec(`UPDATE user_chests SET unlock_at = ? WHERE user_id = ?`,
				time.Now().UTC().Add(-1*time.Minute), userID); err != nil {
				t.Fatalf("force-unlock next chest: %v", err)
			}
		}
	}

	// DB 断言：开了 3 次箱
	var availableSteps, consumedSteps uint64
	if err := sqlDB.QueryRow(`SELECT available_steps, consumed_steps FROM user_step_accounts WHERE user_id=?`, userID).Scan(&availableSteps, &consumedSteps); err != nil {
		t.Fatalf("query: %v", err)
	}
	if availableSteps != 500 {
		t.Errorf("available_steps=%d, want 500 (3 opens, each -1000)", availableSteps)
	}
	if consumedSteps != 3000 {
		t.Errorf("consumed_steps=%d, want 3000", consumedSteps)
	}

	assertCount(t, sqlDB, "chest_open_logs WHERE user_id=?", []any{userID}, 3, "log 3 rows")
	assertCount(t, sqlDB, "chest_open_idempotency_records WHERE user_id=?", []any{userID}, 3, "idem 3 distinct keys")
	assertCount(t, sqlDB, "user_chests WHERE user_id=?", []any{userID}, 1, "chest only 1 row (旧删 + 新建 3 次 → 最终 1 行)")
}

// ============================================================
// AC8: 并发 1 — 100 goroutine 同 idempotencyKey → 全部 100 个成功收敛
// ============================================================
//
// 核心验证 ClaimPending INSERT ... ON DUPLICATE KEY UPDATE + uk_user_id_key UNIQUE 索引
// 串行化保证 + 短路 cached replay 路径。
//
// 期望：100 个 goroutine 全部返回成功（无 1009 / 3002） + 全部拿到同一 reward + nextChest。
func TestChestOpenServiceIntegration_Concurrent100SameKey_OnlyOneOpens(t *testing.T) {
	svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
	defer cleanup()

	const userID = uint64(1)
	const idempotencyKey = "test_concurrent_100_same"
	insertUser(t, sqlDB, userID, "uid-conc-same", "并发同key", "")
	insertStepAccount(t, sqlDB, userID, 1500, 1500, 0)
	insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

	const N = 100
	type result struct {
		rewardID uint64
		nextID   uint64
		err      error
	}
	results := make([]result, N)

	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			out, err := svc.OpenChest(context.Background(), service.OpenChestInput{
				UserID:         userID,
				IdempotencyKey: idempotencyKey,
			})
			if err != nil {
				results[i] = result{err: err}
				return
			}
			results[i] = result{rewardID: out.Reward.CosmeticItemID, nextID: out.NextChest.ID}
		}()
	}
	wg.Wait()

	// 断言 1: 全部 100 goroutine 都成功（任一 err → fail）
	var firstReward, firstNextID uint64
	firstSet := false
	for i, r := range results {
		if r.err != nil {
			t.Fatalf("goroutine %d err=%v (同 key 并发应收敛到 cached replay 而非 1009)", i, r.err)
		}
		if !firstSet {
			firstReward = r.rewardID
			firstNextID = r.nextID
			firstSet = true
			continue
		}
		if r.rewardID != firstReward {
			t.Errorf("g%d: rewardID=%d, want %d (cached)", i, r.rewardID, firstReward)
		}
		if r.nextID != firstNextID {
			t.Errorf("g%d: nextID=%d, want %d (cached)", i, r.nextID, firstNextID)
		}
	}

	// DB 断言：只开了 1 次
	var availableSteps uint64
	if err := sqlDB.QueryRow(`SELECT available_steps FROM user_step_accounts WHERE user_id=?`, userID).Scan(&availableSteps); err != nil {
		t.Fatalf("query step_account: %v", err)
	}
	if availableSteps != 500 {
		t.Errorf("available_steps=%d, want 500", availableSteps)
	}
	assertCount(t, sqlDB, "chest_open_logs WHERE user_id=?", []any{userID}, 1, "log only 1")
	assertCount(t, sqlDB, "chest_open_idempotency_records WHERE user_id=? AND status='success'", []any{userID}, 1, "idem 1 success")
	assertCount(t, sqlDB, "chest_open_idempotency_records WHERE user_id=?", []any{userID}, 1, "idem only 1 row")
	assertCount(t, sqlDB, "user_chests WHERE user_id=?", []any{userID}, 1, "chest 1")
}

// ============================================================
// AC9: 并发 2 — 100 goroutine 不同 idempotencyKey + 仅 1500 步 → 1 成功 + 99 个 4002
// ============================================================
//
// 核心验证 FOR UPDATE 行锁串行化 + 失败事务全 ROLLBACK 干净（ClaimPending pending 行不残留）。
//
// **实际 race 行为**（见 chest_open_service.go runOpenChestTx 步骤 5c-5f）：
//   - 100 个请求各自 INSERT pending idempotency 行（uk_user_id_key UNIQUE 是 user+key，
//     不同 key 互不冲突 → 100 行全部 INSERT 成功 + affectedRows=1 → 全部走步骤 5c 业务流程）
//   - 100 个事务在步骤 5c FindByUserIDForUpdate(userID) 等同一行 user_chests 的 X-lock
//   - 第一个事务拿到锁 → 步骤 5d unlock_at 通过 → 5e 步数足够 → 5f 扣 1000 → 5i DELETE 旧 chest
//     + INSERT 新 chest（unlock_at = now+10min，**未来时刻**）→ commit
//   - 其他 99 个事务 unblock 后 FOR UPDATE 拿到的是**新 chest**（因为旧 chest 已被 DELETE）
//   - 步骤 5d 检查新 chest.unlock_at > now → **isUnlockable=false** → 返回 4002（ErrChestNotUnlocked）
//   - **关键**：5d unlock_at 检查在 5e available_steps 检查**之前**，所以失败错误码是 4002 而非 3002
//   - 99 个失败事务 ROLLBACK → 各自 pending idempotency 行也回滚干净
//
// **关键断言**：99 个失败事务的 ClaimPending 行**必须**全部 ROLLBACK → DB idempotency 表只剩 1 行
// （成功事务的 status=success）；如果失败事务的 pending 行残留 → 下次同 key 再来会被锁定
// → 业务严重 bug（已开过的 key 被误标"开过了 → cached replay"）。
//
// **可选 step 边界场景**：若想构造 99 个事务都过 5d unlock_at 检查、然后在 5e 拿 3002，
// 需要在步骤 5i 不刷新 chest.unlock_at（或刷成 now-1ms）—— 但这违反 V1 §7.2.5i 钦定的
// "下一轮 unlock_at = now + 10min"。该路径在生产不存在，所以本测试断言遵循生产真实行为：
// 锁排队后 99 次都是 4002，而非 3002。
func TestChestOpenServiceIntegration_Concurrent100DifferentKeys_StepLimitOnlyOneOpens(t *testing.T) {
	svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
	defer cleanup()

	const userID = uint64(1)
	insertUser(t, sqlDB, userID, "uid-conc-diff", "并发不同key", "")
	insertStepAccount(t, sqlDB, userID, 1500, 1500, 0) // 仅够 1 次开箱
	insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

	const N = 100
	type result struct {
		succeeded bool
		errCode   int
	}
	results := make([]result, N)

	var wg sync.WaitGroup
	wg.Add(N)
	for i := 0; i < N; i++ {
		i := i
		key := fmt.Sprintf("conc_diff_%03d", i)
		go func() {
			defer wg.Done()
			_, err := svc.OpenChest(context.Background(), service.OpenChestInput{
				UserID: userID, IdempotencyKey: key,
			})
			if err == nil {
				results[i] = result{succeeded: true}
				return
			}
			var appErr *apperror.AppError
			if stderrors.As(err, &appErr) {
				results[i] = result{errCode: appErr.Code}
			} else {
				results[i] = result{errCode: -1}
			}
		}()
	}
	wg.Wait()

	// race 期望（详见函数头注释）：
	//   - 1 个 succeeded（第一个事务）
	//   - 99 个 ErrChestNotUnlocked (4002)（其他事务在新 chest.unlock_at 检查失败）
	//   - 0 个 ErrInsufficientSteps (3002)（5d 在 5e 之前，never reach available_steps 检查）
	//   - 0 个 other
	succeededCount, chestNotUnlockedCount, insufficientCount, otherErr := 0, 0, 0, 0
	for _, r := range results {
		if r.succeeded {
			succeededCount++
		} else if r.errCode == apperror.ErrChestNotUnlocked {
			chestNotUnlockedCount++
		} else if r.errCode == apperror.ErrInsufficientSteps {
			insufficientCount++
		} else {
			otherErr++
		}
	}
	if succeededCount != 1 {
		t.Errorf("succeededCount=%d, want 1", succeededCount)
	}
	if chestNotUnlockedCount != N-1 {
		t.Errorf("chestNotUnlockedCount=%d, want %d (4002 race 后新 chest unlock_at 未到)", chestNotUnlockedCount, N-1)
	}
	if insufficientCount != 0 {
		t.Errorf("insufficientCount=%d, want 0 (5d unlock_at 检查在 5e step 检查之前 → never reach 3002)", insufficientCount)
	}
	if otherErr != 0 {
		t.Errorf("otherErr=%d (unexpected error codes)", otherErr)
	}

	// DB 断言：只开了 1 次 + 仅 1 行 idempotency（99 个失败事务 ROLLBACK 干净）
	var availableSteps uint64
	if err := sqlDB.QueryRow(`SELECT available_steps FROM user_step_accounts WHERE user_id=?`, userID).Scan(&availableSteps); err != nil {
		t.Fatalf("query step_account: %v", err)
	}
	if availableSteps != 500 {
		t.Errorf("available_steps=%d, want 500", availableSteps)
	}
	assertCount(t, sqlDB, "chest_open_logs WHERE user_id=?", []any{userID}, 1, "log only 1")
	assertCount(t, sqlDB, "chest_open_idempotency_records WHERE user_id=?", []any{userID}, 1, "idem only 1 (99 failed tx ROLLBACK clean)")
	assertCount(t, sqlDB, "chest_open_idempotency_records WHERE user_id=? AND status='success'", []any{userID}, 1, "idem the 1 row is success")
}

// ============================================================
// AC10: 边界 1 — 步数恰好 999 → 3002
// ============================================================
func TestChestOpenServiceIntegration_Steps999_Returns3002(t *testing.T) {
	svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
	defer cleanup()

	const userID = uint64(1)
	insertUser(t, sqlDB, userID, "uid-step-999", "边界999", "")
	insertStepAccount(t, sqlDB, userID, 999, 999, 0)
	insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

	_, err := svc.OpenChest(context.Background(), service.OpenChestInput{
		UserID: userID, IdempotencyKey: "test_steps_999",
	})
	requireAppError(t, err, apperror.ErrInsufficientSteps, "AC10 Steps999")

	// 不变：step / log / idem / chest
	assertCount(t, sqlDB, "chest_open_logs WHERE user_id=?", []any{userID}, 0, "no log")
	assertCount(t, sqlDB, "chest_open_idempotency_records WHERE user_id=?", []any{userID}, 0, "no idem (ROLLBACK)")
	var availableSteps uint64
	if err := sqlDB.QueryRow(`SELECT available_steps FROM user_step_accounts WHERE user_id=?`, userID).Scan(&availableSteps); err != nil {
		t.Fatalf("query step_account: %v", err)
	}
	if availableSteps != 999 {
		t.Errorf("available_steps=%d, want 999 (no spend)", availableSteps)
	}
}

// ============================================================
// AC11: 边界 2 — 步数恰好 1000 → 成功，余 0
// ============================================================
func TestChestOpenServiceIntegration_Steps1000_SucceedsAvailable0(t *testing.T) {
	svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
	defer cleanup()

	const userID = uint64(1)
	insertUser(t, sqlDB, userID, "uid-step-1000", "边界1000", "")
	insertStepAccount(t, sqlDB, userID, 1000, 1000, 0)
	insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

	out, err := svc.OpenChest(context.Background(), service.OpenChestInput{
		UserID: userID, IdempotencyKey: "test_steps_1000",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if out.StepAccount.AvailableSteps != 0 {
		t.Errorf("AvailableSteps=%d, want 0", out.StepAccount.AvailableSteps)
	}
	if out.StepAccount.ConsumedSteps != 1000 {
		t.Errorf("ConsumedSteps=%d, want 1000", out.StepAccount.ConsumedSteps)
	}

	// DB 校验
	var availableSteps uint64
	if err := sqlDB.QueryRow(`SELECT available_steps FROM user_step_accounts WHERE user_id=?`, userID).Scan(&availableSteps); err != nil {
		t.Fatalf("query: %v", err)
	}
	if availableSteps != 0 {
		t.Errorf("DB available_steps=%d, want 0", availableSteps)
	}
}

// ============================================================
// AC12: 边界 3 — 步数恰好 1001 → 成功，余 1
// ============================================================
func TestChestOpenServiceIntegration_Steps1001_SucceedsAvailable1(t *testing.T) {
	svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
	defer cleanup()

	const userID = uint64(1)
	insertUser(t, sqlDB, userID, "uid-step-1001", "边界1001", "")
	insertStepAccount(t, sqlDB, userID, 1001, 1001, 0)
	insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

	out, err := svc.OpenChest(context.Background(), service.OpenChestInput{
		UserID: userID, IdempotencyKey: "test_steps_1001",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if out.StepAccount.AvailableSteps != 1 {
		t.Errorf("AvailableSteps=%d, want 1", out.StepAccount.AvailableSteps)
	}

	// DB 校验
	var availableSteps uint64
	if err := sqlDB.QueryRow(`SELECT available_steps FROM user_step_accounts WHERE user_id=?`, userID).Scan(&availableSteps); err != nil {
		t.Fatalf("query: %v", err)
	}
	if availableSteps != 1 {
		t.Errorf("DB available_steps=%d, want 1", availableSteps)
	}
}

// ============================================================
// AC13: 边界 4 — chest unlock_at 比 now 早 1ms → unlockable（V1 §7.2.5d 公式边界）
// ============================================================
//
// **20.9 r2 修正（fixed clock）**：本 case 必须用 fixed clock 才能精确验证
// service 内 `chest.UnlockAt.After(now) == false` 的边界 —— 即 "now - 1ms <= now"
// 必须为真。
//
// **r1 缺陷**：r1 实装走 wall clock，`unlockAt = time.Now().UTC().Add(-1ms)` 后
// service 内再调 `s.nowFn() = time.Now().UTC()`，在 busy CI runner 上两次
// time.Now() 间隔可能 >> 1ms（DB INSERT / GORM 反射 / RTT 等耗时），实际 delta
// 远大于 1ms；即使 service 错把 `!After(now)` 改成 `Before(now)` 或 `< 5ms` 等
// regression，本测试仍可能误判通过 —— 没精确锁住边界语义。
//
// **r2 修正**：通过 service.SetChestServiceNowFn 注入 fixed nowFn → T；构造
// unlockAt = T - 1ms；精确验证"差 1ms 仍 unlockable"。lesson 见
// docs/lessons/2026-05-15-fixed-clock-for-boundary-tests.md。
func TestChestOpenServiceIntegration_UnlockAtMinus1ms_IsUnlockable(t *testing.T) {
	svc, sqlDB, cleanup := buildChestOpenServiceIntegration(t)
	defer cleanup()

	// fixed clock: 固定时刻 T；service 内 s.nowFn() 必返 T，
	// 与 wall clock 解耦 → unlockAt = T - 1ms 精确对应边界。
	fixedNow := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	service.SetChestServiceNowFn(svc, func() time.Time { return fixedNow })

	const userID = uint64(1)
	insertUser(t, sqlDB, userID, "uid-unlock-1ms", "边界1ms", "")
	insertStepAccount(t, sqlDB, userID, 1500, 1500, 0)
	// unlock_at = fixedNow - 1ms（status=1 counting，时间精确早 1ms）
	unlockAt := fixedNow.Add(-1 * time.Millisecond)
	insertChest(t, sqlDB, 9001, userID, 1, unlockAt, 1000)

	out, err := svc.OpenChest(context.Background(), service.OpenChestInput{
		UserID: userID, IdempotencyKey: "test_unlock_1ms",
	})
	if err != nil {
		t.Fatalf("expected unlockable (status=1 + unlock_at == fixedNow-1ms), got %v", err)
	}
	if out == nil {
		t.Fatal("out = nil")
	}
}

// ============================================================
// AC14: 抽奖分布 — 1000 次开箱 → 各品质比例符合 drop_weight 设计
// ============================================================
//
// **20.9 r2 修正（deterministic picker）**：本 case **不**走真实 crypto-weighted
// picker —— 真随机 + 1000 样本必然偶发命中 tail outcome（如 legendary 期望 1.1
// 件，P(count=0) ≈ 33%；rare 期望 90 件，σ ≈ 9.6 → ±3σ 区间内仍有 ~5% 漏掉），
// 把集成测试 `--integration` 退化为 flaky CI gate（lesson：
// docs/lessons/2026-05-15-no-real-rng-in-integration-tests.md）。
//
// **r2 设计**：注入 deterministic stub picker，按理论比例（common 900 / rare 90 /
// epic 9 / legendary 1）钦定调用序列。本 case 验证的是 **service 把 picker 返回
// 的 index 正确映射到 reward_rarity 字段 + 1000 次循环正确顺序执行 1000 个事务**
// —— 是集成 layer 的关注点；picker 自身权重算法的分布正确性留给
// `weighted_test.go` 的 stand-alone unit test（可用大样本 + chi-square 或
// 确定性 seed 验证）。
//
// **picker 策略**：stub 按 desired weight 找匹配 item index 返回 —— common 件
// drop_weight=100 / rare=20 / epic=4 / legendary=1 唯一可区分 → 不依赖
// cosmetic_items 表 ORDER BY（GORM Find 默认 MySQL 顺序未必稳定）。
func TestChestOpenServiceIntegration_WeightedPickDistribution_1000Opens(t *testing.T) {
	sqlDB, chestRepo, stepAccountRepo, idempotencyRepo, cosmeticItemRepo, chestOpenLogRepo, txMgr, _, cleanup := buildChestServiceWithRepos(t)
	defer cleanup()

	// deterministic stub picker：900 common (weight=100) → 90 rare (weight=20) →
	// 9 epic (weight=4) → 1 legendary (weight=1)；总 1000 次。
	stub := newRaritySequencePicker(t,
		raritySequenceSpec{desiredWeight: 100, count: 900}, // common
		raritySequenceSpec{desiredWeight: 20, count: 90},   // rare
		raritySequenceSpec{desiredWeight: 4, count: 9},     // epic
		raritySequenceSpec{desiredWeight: 1, count: 1},     // legendary
	)

	svc := service.NewChestService(chestRepo, txMgr, idempotencyRepo, stepAccountRepo, cosmeticItemRepo, chestOpenLogRepo, stub)

	const userID = uint64(1)
	insertUser(t, sqlDB, userID, "uid-dist", "分布", "")
	insertStepAccount(t, sqlDB, userID, 1500, 1500, 0)
	insertChest(t, sqlDB, 9001, userID, 1, time.Now().UTC().Add(-1*time.Minute), 1000)

	const N = 1000
	for i := 0; i < N; i++ {
		_, err := svc.OpenChest(context.Background(), service.OpenChestInput{
			UserID:         userID,
			IdempotencyKey: fmt.Sprintf("dist_%04d", i),
		})
		if err != nil {
			t.Fatalf("call %d: %v", i, err)
		}
		// 重置步数 + 下一轮 chest force-unlock（每次循环准备开下一次）
		if i < N-1 {
			if _, err := sqlDB.Exec(`UPDATE user_step_accounts SET available_steps=1500, consumed_steps=0, version=version+1 WHERE user_id=?`, userID); err != nil {
				t.Fatalf("reset steps: %v", err)
			}
			if _, err := sqlDB.Exec(`UPDATE user_chests SET unlock_at=?, status=1 WHERE user_id=?`,
				time.Now().UTC().Add(-1*time.Minute), userID); err != nil {
				t.Fatalf("force-unlock: %v", err)
			}
		}
	}

	// 验证 stub 被调用 N 次（防 service 旁路抽奖逻辑的 regression）
	if got := stub.calls(); got != N {
		t.Errorf("stub picker calls=%d, want %d", got, N)
	}

	// 统计 chest_open_logs.reward_rarity 分布
	rows, err := sqlDB.Query(`SELECT reward_rarity, COUNT(*) FROM chest_open_logs WHERE user_id=? GROUP BY reward_rarity`, userID)
	if err != nil {
		t.Fatalf("query distribution: %v", err)
	}
	defer rows.Close()

	counts := map[int8]int{}
	for rows.Next() {
		var rarity int8
		var n int
		if err := rows.Scan(&rarity, &n); err != nil {
			t.Fatalf("scan: %v", err)
		}
		counts[rarity] = n
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("rows.Err: %v", err)
	}

	// **精确断言**（deterministic stub → 0 flakiness）：
	//   rarity=1 (common):    900
	//   rarity=2 (rare):       90
	//   rarity=3 (epic):        9
	//   rarity=4 (legendary):   1
	if counts[1] != 900 {
		t.Errorf("common count=%d, want exactly 900 (deterministic stub)", counts[1])
	}
	if counts[2] != 90 {
		t.Errorf("rare count=%d, want exactly 90 (deterministic stub)", counts[2])
	}
	if counts[3] != 9 {
		t.Errorf("epic count=%d, want exactly 9 (deterministic stub)", counts[3])
	}
	if counts[4] != 1 {
		t.Errorf("legendary count=%d, want exactly 1 (deterministic stub)", counts[4])
	}
	// 总和必须 == 1000
	total := counts[1] + counts[2] + counts[3] + counts[4]
	if total != N {
		t.Errorf("total=%d, want %d", total, N)
	}
}

// ============================================================
// Fault injection wrapper struct（Story 20.9 范围；仅本文件可见同 package service_test 内）
// ============================================================

// faultStepAccountRepoOnSpend 包装真实 StepAccountRepo —— 让 Spend 直接抛 injectErr，其他方法透传。
//
// 模式：MVP 用"按方法包装"，不引入第三方 fault injection 框架（与 4.7 fault*Repo 同模式）。
// 优点：编译期 interface 检查 + 跨平台无依赖（gomonkey 在 ARM 不工作）。
type faultStepAccountRepoOnSpend struct {
	delegate  mysql.StepAccountRepo
	injectErr error
}

func (f *faultStepAccountRepoOnSpend) Create(ctx context.Context, a *mysql.StepAccount) error {
	return f.delegate.Create(ctx, a)
}

func (f *faultStepAccountRepoOnSpend) FindByUserID(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
	return f.delegate.FindByUserID(ctx, userID)
}

func (f *faultStepAccountRepoOnSpend) UpdateBalance(ctx context.Context, userID uint64, delta int32, expectedVersion uint64) error {
	return f.delegate.UpdateBalance(ctx, userID, delta, expectedVersion)
}

func (f *faultStepAccountRepoOnSpend) FindByUserIDForUpdate(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
	return f.delegate.FindByUserIDForUpdate(ctx, userID)
}

func (f *faultStepAccountRepoOnSpend) Spend(ctx context.Context, userID uint64, amount uint64, expectedVersion uint64) error {
	return f.injectErr
}

// faultCosmeticItemRepoOnList 让 ListEnabledForWeightedPick 返 ([]CosmeticItem{}, nil) —— 触发 service
// 内 `len(items) == 0 → return ErrServiceBusy`（chest_open_service.go:255-258）。
//
// **不**注入"抛 error"路径（也可以走 1009，但与 chest_open_service.go 现有 service 内 len==0
// 分支语义对应；epics.md 行 2985 钦定的"mock 抛 error"在实际代码中等价于"返空 + service 内
// 自己 return error"，两者最终都让 fn return non-nil error → tx.WithTx ROLLBACK）。
//
// 可选 injectErr 字段：未来若需直接走"DB 错"路径，构造 instance 时设置 injectErr 非 nil 即可。
type faultCosmeticItemRepoOnList struct {
	returnEmpty bool
	injectErr   error
}

func (f *faultCosmeticItemRepoOnList) ListEnabledForWeightedPick(ctx context.Context) ([]mysql.CosmeticItem, error) {
	if f.injectErr != nil {
		return nil, f.injectErr
	}
	return []mysql.CosmeticItem{}, nil
}

// faultChestOpenLogRepoOnCreate 让 Create 直接抛 injectErr —— ChestOpenLogRepo interface 仅 Create 一个方法
// 所以本 wrapper 不需要透传其他方法。
type faultChestOpenLogRepoOnCreate struct {
	injectErr error
}

func (f *faultChestOpenLogRepoOnCreate) Create(ctx context.Context, log *mysql.ChestOpenLog) error {
	return f.injectErr
}

// ============================================================
// raritySequencePicker — deterministic WeightedPicker stub
// （Story 20.9 r2 引入，仅本 file 用于 AC14 分布 case）
// ============================================================
//
// 用途：替代 random.NewCryptoWeightedPicker，按预定 sequence 返回 item index。
// 让"1000 次开箱分布断言"完全 deterministic → 0 flakiness。
//
// 设计：
//
//   - 构造时接收一组 raritySequenceSpec（desiredWeight + count）；按顺序铺平成
//     1×count[0] + 1×count[1] + ... 的 weight sequence
//   - 每次 Pick(items) 调用 → 取出当前 sequence 首位 desiredWeight → 在 items
//     里线性扫描找到第一个 Weight == desiredWeight 的 index 返回
//   - cosmetic_items 表中 4 个 drop_weight 值（100 / 20 / 4 / 1）唯一可区分 →
//     找匹配 weight 的 index 等价于"选某一 rarity 桶里第一件 item"
//   - 调用次数耗尽 sequence 后再 Pick → panic（防 caller 多调）
//
// **为何不用 mathrand.New(seed)**：mathrand 仍是 RNG，分布是统计意义的；只在
// 大样本意义上接近期望，单次 1000 抽样仍可能落 tail。本 stub 完全 deterministic。
type raritySequenceSpec struct {
	desiredWeight uint64 // items[i].Weight 必须 == 该值才被选中
	count         int    // 该 weight 连续返回次数
}

type raritySequencePicker struct {
	t        *testing.T
	sequence []uint64 // 铺平后的 desiredWeight 序列
	cursor   int
	mu       sync.Mutex
}

func newRaritySequencePicker(t *testing.T, specs ...raritySequenceSpec) *raritySequencePicker {
	t.Helper()
	total := 0
	for _, s := range specs {
		total += s.count
	}
	seq := make([]uint64, 0, total)
	for _, s := range specs {
		for i := 0; i < s.count; i++ {
			seq = append(seq, s.desiredWeight)
		}
	}
	return &raritySequencePicker{t: t, sequence: seq}
}

// Pick 实现 random.WeightedPicker：返回 items 中第一个 Weight 等于
// sequence[cursor] 的 index；cursor 越界 panic。
func (p *raritySequencePicker) Pick(items []random.WeightedItem) (int, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cursor >= len(p.sequence) {
		p.t.Fatalf("raritySequencePicker: cursor=%d >= sequence len=%d (caller over-called)", p.cursor, len(p.sequence))
	}
	desired := p.sequence[p.cursor]
	p.cursor++
	for i, it := range items {
		if it.Weight == desired {
			return i, nil
		}
	}
	p.t.Fatalf("raritySequencePicker: no item with Weight=%d in %d items (cursor was %d)", desired, len(items), p.cursor-1)
	return 0, nil // unreachable; t.Fatalf 已终止
}

func (p *raritySequencePicker) calls() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cursor
}
