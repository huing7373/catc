//go:build integration
// +build integration

// Story 20.6 chest_open_service 集成测试：用 dockertest 起真实 mysql:8.0 容器跑 ≥2 case：
//   1. HappyPath_FullFlow: 创建 user + 1500 步 + force-unlock chest → 调 OpenChest →
//      验证 DB user_step_accounts.available_steps=500 + consumed_steps=1000 +
//      chest_open_logs 多 1 行 + 旧 chest 删除 + 新 chest 创建 + idempotency 行 status=success
//   2. HappyPath_IdempotencyReplay: 第一次 open success → 第二次同 idempotencyKey → 短路 + DB 无副作用
//
// build tag `integration` 隔离 → 默认 `bash scripts/build.sh --test` 不跑这些；
// 只在 `bash scripts/build.sh --integration` 触发。
//
// 本机 Windows docker 不可用 → t.Skip（startMySQL 内已 skip）。CI Linux 跑。
//
// 复用既有 helper：startMySQL / runMigrations / insertUser / insertStepAccount / insertChest。
// **手工 INSERT 测试数据**（不调 auth_service.GuestLogin），与既有 chest_service_integration_test
// 同模式。

package service_test

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
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
