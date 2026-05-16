//go:build !integration

// Story 20.6 chest_open_service 单元测试：≥7 case + 推荐 3 case = ≥10 case
// 覆盖 OpenChest 业务路径 + 错误翻译。
//
// 模式与 auth_service_test.go / chest_service_test.go 同（stub repo + 每 case 独立
// stub instance）。
//
// 关键不重测项（已由其他 layer 覆盖）：
//   - stubTxMgr.WithTx 直接调 fn（与既有 auth_service_test 同模式）；事务回滚由 dockertest
//   - random.WeightedPicker.Pick 行为由 weighted_test.go 单测覆盖；本文件 stub picker
//   - chestStatusDynamic / computeRemainingSeconds 由 home_service_test 已覆盖

package service_test

import (
	"context"
	"encoding/json"
	stderrors "errors"
	"reflect"
	"testing"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/random"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// ================================================================
// stubs
// ================================================================

// stubIdempotencyRepo: mysql.IdempotencyRepo 的 stub（每 case 独立实例）
type stubIdempotencyRepo struct {
	findFn        func(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error)
	claimFn       func(ctx context.Context, userID uint64, key string) (int64, error)
	markSuccessFn func(ctx context.Context, userID uint64, key string, responseJSON []byte) error

	findCalls   int
	claimCalls  int
	markCalls   int
	lastMarkJSON []byte
}

func (s *stubIdempotencyRepo) FindByUserIDAndKey(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error) {
	s.findCalls++
	return s.findFn(ctx, userID, key)
}

func (s *stubIdempotencyRepo) ClaimPending(ctx context.Context, userID uint64, key string) (int64, error) {
	s.claimCalls++
	return s.claimFn(ctx, userID, key)
}

func (s *stubIdempotencyRepo) MarkSuccess(ctx context.Context, userID uint64, key string, responseJSON []byte) error {
	s.markCalls++
	s.lastMarkJSON = responseJSON
	return s.markSuccessFn(ctx, userID, key, responseJSON)
}

// stubCosmeticItemRepo: mysql.CosmeticItemRepo 的 stub
type stubCosmeticItemRepo struct {
	listFn    func(ctx context.Context) ([]mysql.CosmeticItem, error)
	listCalls int
}

func (s *stubCosmeticItemRepo) ListEnabledForWeightedPick(ctx context.Context) ([]mysql.CosmeticItem, error) {
	s.listCalls++
	return s.listFn(ctx)
}

// ListEnabledForCatalog: Story 23.3 给 CosmeticItemRepo interface 加了本方法
// （GET /cosmetics/catalog 配置目录查询）；本 20.6 stub 仅测开箱加权抽取路径，
// 不走 catalog 路径 —— 加防御性 panic 让任何意外调用暴露（与 cosmetic_service_test
// 同模式；本方法**仅**为 satisfy 扩展后的 interface 编译，不改 20.6 任何既有行为）。
func (s *stubCosmeticItemRepo) ListEnabledForCatalog(ctx context.Context) ([]mysql.CosmeticItem, error) {
	panic("stubCosmeticItemRepo.ListEnabledForCatalog not expected (chest_open_service_test 仅测开箱加权抽取路径，不走 GET /cosmetics/catalog 路径)")
}

// ListByIDsForInventory: Story 23.4 给 CosmeticItemRepo interface 加了本方法
// （GET /cosmetics/inventory config 关联）；本 20.6 stub 仅测开箱加权抽取路径，
// 不走 inventory 路径 —— 加防御性 panic 让任何意外调用暴露（与
// ListEnabledForCatalog 同模式；本方法**仅**为 satisfy 扩展后的 interface
// 编译，不改 20.6 任何既有行为）。
func (s *stubCosmeticItemRepo) ListByIDsForInventory(ctx context.Context, ids []uint64) ([]mysql.CosmeticItem, error) {
	panic("stubCosmeticItemRepo.ListByIDsForInventory not expected (chest_open_service_test 仅测开箱加权抽取路径，不走 GET /cosmetics/inventory 路径)")
}

// stubChestOpenLogRepo: mysql.ChestOpenLogRepo 的 stub
type stubChestOpenLogRepo struct {
	createFn    func(ctx context.Context, log *mysql.ChestOpenLog) error
	createCalls []*mysql.ChestOpenLog
}

func (s *stubChestOpenLogRepo) Create(ctx context.Context, log *mysql.ChestOpenLog) error {
	s.createCalls = append(s.createCalls, log)
	if s.createFn != nil {
		return s.createFn(ctx, log)
	}
	return nil
}

// stubWeightedPicker: random.WeightedPicker 的 stub（注入指定 selectedIndex）
type stubWeightedPicker struct {
	pickFn    func(items []random.WeightedItem) (int, error)
	pickCalls int
}

func (s *stubWeightedPicker) Pick(items []random.WeightedItem) (int, error) {
	s.pickCalls++
	if s.pickFn != nil {
		return s.pickFn(items)
	}
	return 0, nil
}

// stubOpenChestChestRepo: mysql.ChestRepo 的 stub（覆盖 20.6 引入的 FindByUserIDForUpdate / Delete 方法）。
// 与 auth_service_test 的 stubChestRepo 同 type name 会 conflict —— 本 stub 用独立 type。
type stubOpenChestChestRepo struct {
	findFn              func(ctx context.Context, userID uint64) (*mysql.UserChest, error)
	findForUpdateFn     func(ctx context.Context, userID uint64) (*mysql.UserChest, error)
	createFn            func(ctx context.Context, c *mysql.UserChest) error
	deleteFn            func(ctx context.Context, id uint64) error
	findForUpdateCalls  int
	createCalls         []*mysql.UserChest
	deleteCalls         []uint64
}

func (s *stubOpenChestChestRepo) Create(ctx context.Context, c *mysql.UserChest) error {
	s.createCalls = append(s.createCalls, c)
	if s.createFn != nil {
		return s.createFn(ctx, c)
	}
	// GORM 模拟回填 ID（与 stubChestRepo 的 nil createFn 行为对齐）
	if c.ID == 0 {
		c.ID = 9999 // 默认回填测试值
	}
	return nil
}
func (s *stubOpenChestChestRepo) FindByUserID(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
	if s.findFn != nil {
		return s.findFn(ctx, userID)
	}
	panic("stubOpenChestChestRepo.FindByUserID not configured")
}

// FindByID Story 20.7 review r2 [P2] 加：chest_open_service (20.6 OpenChest) 不调；保留以满足 interface。
func (s *stubOpenChestChestRepo) FindByID(ctx context.Context, chestID uint64) (*mysql.UserChest, error) {
	panic("stubOpenChestChestRepo.FindByID not configured (chest_open_service should not call it)")
}

// FindByIDForUpdate Story 20.7 review r4 [P2] 加：chest_open_service (20.6 OpenChest) 不调；保留以满足 interface。
func (s *stubOpenChestChestRepo) FindByIDForUpdate(ctx context.Context, chestID uint64) (*mysql.UserChest, error) {
	panic("stubOpenChestChestRepo.FindByIDForUpdate not configured (chest_open_service should not call it)")
}

func (s *stubOpenChestChestRepo) FindByUserIDForUpdate(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
	s.findForUpdateCalls++
	return s.findForUpdateFn(ctx, userID)
}
func (s *stubOpenChestChestRepo) Delete(ctx context.Context, id uint64) error {
	s.deleteCalls = append(s.deleteCalls, id)
	if s.deleteFn != nil {
		return s.deleteFn(ctx, id)
	}
	return nil
}

// UpdateUnlockAtByID Story 20.7 加（review r1 [P2] 改造后签名 userID → chestID）：
// chest_open_service (20.6 OpenChest) 不调；保留以满足 interface。
func (s *stubOpenChestChestRepo) UpdateUnlockAtByID(ctx context.Context, chestID uint64, newUnlockAt time.Time) error {
	panic("stubOpenChestChestRepo.UpdateUnlockAtByID not configured (chest_open_service should not call it)")
}

// stubOpenChestStepAccountRepo: mysql.StepAccountRepo stub（覆盖 20.6 引入的 FindByUserIDForUpdate / Spend）
type stubOpenChestStepAccountRepo struct {
	findForUpdateFn func(ctx context.Context, userID uint64) (*mysql.StepAccount, error)
	spendFn         func(ctx context.Context, userID uint64, amount uint64, expectedVersion uint64) error

	findForUpdateCalls int
	spendCalls         int
	lastSpendAmount    uint64
	lastSpendVersion   uint64
}

func (s *stubOpenChestStepAccountRepo) Create(ctx context.Context, a *mysql.StepAccount) error {
	panic("stubOpenChestStepAccountRepo.Create not configured (OpenChest should not call it)")
}
func (s *stubOpenChestStepAccountRepo) FindByUserID(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
	panic("stubOpenChestStepAccountRepo.FindByUserID not configured (OpenChest uses FindByUserIDForUpdate)")
}
func (s *stubOpenChestStepAccountRepo) UpdateBalance(ctx context.Context, userID uint64, delta int32, expectedVersion uint64) error {
	panic("stubOpenChestStepAccountRepo.UpdateBalance not configured (OpenChest uses Spend)")
}
func (s *stubOpenChestStepAccountRepo) FindByUserIDForUpdate(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
	s.findForUpdateCalls++
	return s.findForUpdateFn(ctx, userID)
}
func (s *stubOpenChestStepAccountRepo) Spend(ctx context.Context, userID uint64, amount uint64, expectedVersion uint64) error {
	s.spendCalls++
	s.lastSpendAmount = amount
	s.lastSpendVersion = expectedVersion
	return s.spendFn(ctx, userID, amount, expectedVersion)
}

// ================================================================
// helpers
// ================================================================

// fixtureService 构造一个 chestServiceImpl，注入所有 stub + nowFn。
// 单测可在拿到 service 后覆盖 nowFn 字段（service 内部用 reflect 也可，但导出 hook 更简单）：
// 这里我们用 reflect-free 路径：service.NewChestService 返回 ChestService interface；
// 测试 case 内通过传入"控制 nowFn"的方式靠 stub 注入 chestRepo.findForUpdateFn 的 UnlockAt 字段
// 来间接控制 unlockable 判定（now 由 service 内部 time.Now().UTC()）。
//
// 注：本文件多数 case 用 chestRepo.findForUpdateFn 返回 unlockAt=time.Now().UTC().Add(-1*time.Minute)
// 让 unlockable 判定走 chest.UnlockAt <= now 路径，避免 nowFn 注入 ceremony。
func fixtureService(
	t *testing.T,
	idemRepo mysql.IdempotencyRepo,
	chestRepo mysql.ChestRepo,
	stepRepo mysql.StepAccountRepo,
	cosmRepo mysql.CosmeticItemRepo,
	logRepo mysql.ChestOpenLogRepo,
	picker random.WeightedPicker,
) service.ChestService {
	t.Helper()
	txMgr := defaultStubTxMgr()
	return service.NewChestService(chestRepo, txMgr, idemRepo, stepRepo, cosmRepo, logRepo, picker)
}

// nowUTC: 单测用一致的 UTC 时间锚点。
func nowUTC() time.Time { return time.Now().UTC() }

// happyChestRepo / happyStepRepo / happyCosmeticRepo / happyLogRepo / happyPicker:
// "happy" 路径默认 stubs；个别 case 替换字段验证错误分支。
func happyChestRepo() *stubOpenChestChestRepo {
	return &stubOpenChestChestRepo{
		findForUpdateFn: func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			// unlockAt 已过 1min → unlockable
			return &mysql.UserChest{ID: 5001, UserID: 1001, Status: 1, UnlockAt: nowUTC().Add(-time.Minute), OpenCostSteps: 1000, Version: 0}, nil
		},
	}
}

func happyStepRepo() *stubOpenChestStepAccountRepo {
	return &stubOpenChestStepAccountRepo{
		findForUpdateFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1001, TotalSteps: 1500, AvailableSteps: 1500, ConsumedSteps: 0, Version: 3}, nil
		},
		spendFn: func(ctx context.Context, userID uint64, amount uint64, expectedVersion uint64) error {
			return nil
		},
	}
}

func happyCosmeticRepo() *stubCosmeticItemRepo {
	return &stubCosmeticItemRepo{
		listFn: func(ctx context.Context) ([]mysql.CosmeticItem, error) {
			return []mysql.CosmeticItem{
				{ID: 24, Code: "scarf_star", Name: "星星围巾", Slot: 4, Rarity: 2, AssetURL: "https://x/a", IconURL: "https://x/i", DropWeight: 30, IsEnabled: 1},
			}, nil
		},
	}
}

func happyLogRepo() *stubChestOpenLogRepo {
	return &stubChestOpenLogRepo{}
}

func happyPicker() *stubWeightedPicker {
	return &stubWeightedPicker{
		pickFn: func(items []random.WeightedItem) (int, error) {
			return 0, nil
		},
	}
}

// happyIdempotencyRepo: 首次到达（FindByUserIDAndKey 返 NotFound + ClaimPending 返 affected=1 + MarkSuccess 返 nil）
func happyIdempotencyRepo() *stubIdempotencyRepo {
	return &stubIdempotencyRepo{
		findFn: func(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error) {
			return nil, mysql.ErrIdempotencyRecordNotFound
		},
		claimFn: func(ctx context.Context, userID uint64, key string) (int64, error) {
			return 1, nil
		},
		markSuccessFn: func(ctx context.Context, userID uint64, key string, responseJSON []byte) error {
			return nil
		},
	}
}

// ================================================================
// cases
// ================================================================

// 1. HappyPath_FirstTime
func TestChestService_OpenChest_HappyPath_FirstTime(t *testing.T) {
	idem := happyIdempotencyRepo()
	chestR := happyChestRepo()
	stepR := happyStepRepo()
	cosm := happyCosmeticRepo()
	logR := happyLogRepo()
	picker := happyPicker()
	svc := fixtureService(t, idem, chestR, stepR, cosm, logR, picker)

	out, err := svc.OpenChest(context.Background(), service.OpenChestInput{
		UserID: 1001, IdempotencyKey: "test_key_001",
	})
	if err != nil {
		t.Fatalf("OpenChest: %v", err)
	}
	if out == nil {
		t.Fatal("out = nil, want non-nil")
	}
	if out.Reward.UserCosmeticItemID != 0 {
		t.Errorf("Reward.UserCosmeticItemID = %d, want 0 (节点 7 占位)", out.Reward.UserCosmeticItemID)
	}
	if out.Reward.CosmeticItemID != 24 {
		t.Errorf("Reward.CosmeticItemID = %d, want 24", out.Reward.CosmeticItemID)
	}
	if out.StepAccount.AvailableSteps != 500 {
		t.Errorf("StepAccount.AvailableSteps = %d, want 500 (1500-1000)", out.StepAccount.AvailableSteps)
	}
	if out.StepAccount.ConsumedSteps != 1000 {
		t.Errorf("StepAccount.ConsumedSteps = %d, want 1000", out.StepAccount.ConsumedSteps)
	}
	if out.NextChest.OpenCostSteps != 1000 {
		t.Errorf("NextChest.OpenCostSteps = %d, want 1000", out.NextChest.OpenCostSteps)
	}
	// 步骤 5h 写 log 应被调
	if len(logR.createCalls) != 1 {
		t.Errorf("chestOpenLogRepo.Create called %d times, want 1", len(logR.createCalls))
	} else {
		log := logR.createCalls[0]
		if log.RewardUserCosmeticItemID != 0 {
			t.Errorf("log.RewardUserCosmeticItemID = %d, want 0", log.RewardUserCosmeticItemID)
		}
		if log.RewardCosmeticItemID != 24 {
			t.Errorf("log.RewardCosmeticItemID = %d, want 24", log.RewardCosmeticItemID)
		}
		if log.ChestID != 5001 {
			t.Errorf("log.ChestID = %d, want 5001", log.ChestID)
		}
	}
	// 步骤 5i 删旧 chest + 建新 chest
	if len(chestR.deleteCalls) != 1 || chestR.deleteCalls[0] != 5001 {
		t.Errorf("chestRepo.Delete calls = %v, want [5001]", chestR.deleteCalls)
	}
	if len(chestR.createCalls) != 1 {
		t.Errorf("chestRepo.Create called %d times, want 1", len(chestR.createCalls))
	}
	// MarkSuccess 写入的 responseJSON 应**不**含 nextChest.status / nextChest.remainingSeconds / requestId
	if idem.markCalls != 1 {
		t.Errorf("MarkSuccess calls = %d, want 1", idem.markCalls)
	}
	var rawCache map[string]any
	if err := json.Unmarshal(idem.lastMarkJSON, &rawCache); err != nil {
		t.Fatalf("unmarshal cached JSON: %v", err)
	}
	if _, has := rawCache["requestId"]; has {
		t.Errorf("cached response_json should NOT contain top-level requestId; got: %v", rawCache)
	}
	dataMap := rawCache["data"].(map[string]any)
	nextChestMap := dataMap["nextChest"].(map[string]any)
	if _, has := nextChestMap["status"]; has {
		t.Errorf("cached nextChest should NOT contain status (time-derived; r9/r11)")
	}
	if _, has := nextChestMap["remainingSeconds"]; has {
		t.Errorf("cached nextChest should NOT contain remainingSeconds (time-derived; r9/r11)")
	}
}

// 2. IdempotencyReplay_CachedSuccess: handler 入口预检命中 → 直接 cached replay，不进事务。
func TestChestService_OpenChest_IdempotencyReplay_CachedSuccess(t *testing.T) {
	cachedJSON := []byte(`{"code":0,"message":"ok","data":{"reward":{"userCosmeticItemId":0,"cosmeticItemId":24,"name":"星星围巾","slot":4,"rarity":2,"assetUrl":"https://x/a","iconUrl":"https://x/i"},"stepAccount":{"totalSteps":1500,"availableSteps":500,"consumedSteps":1000},"nextChest":{"id":6001,"unlockAt":"2026-04-23T11:00:00Z","openCostSteps":1000}}}`)

	idem := &stubIdempotencyRepo{
		findFn: func(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error) {
			return &mysql.IdempotencyRecord{
				ID: 99, UserID: 1001, IdempotencyKey: key,
				Status: mysql.IdempotencyStatusSuccess, ResponseJSON: cachedJSON,
			}, nil
		},
	}
	// 其他 repo 应不被调用；任何一个被调都用 panic-default stub 暴露
	chestR := &stubOpenChestChestRepo{
		findForUpdateFn: func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			t.Fatal("chestRepo.FindByUserIDForUpdate should NOT be called on cached replay")
			return nil, nil
		},
	}
	stepR := &stubOpenChestStepAccountRepo{
		findForUpdateFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			t.Fatal("stepRepo.FindByUserIDForUpdate should NOT be called on cached replay")
			return nil, nil
		},
	}
	cosm := &stubCosmeticItemRepo{
		listFn: func(ctx context.Context) ([]mysql.CosmeticItem, error) {
			t.Fatal("cosmeticItemRepo.List should NOT be called on cached replay")
			return nil, nil
		},
	}
	logR := happyLogRepo()
	picker := happyPicker()
	svc := fixtureService(t, idem, chestR, stepR, cosm, logR, picker)

	out, err := svc.OpenChest(context.Background(), service.OpenChestInput{
		UserID: 1001, IdempotencyKey: "test_key_001",
	})
	if err != nil {
		t.Fatalf("OpenChest cached replay: %v", err)
	}
	if out == nil {
		t.Fatal("out = nil")
	}
	if out.Reward.CosmeticItemID != 24 {
		t.Errorf("out.Reward.CosmeticItemID = %d, want 24 (from cached JSON)", out.Reward.CosmeticItemID)
	}
	if out.StepAccount.AvailableSteps != 500 {
		t.Errorf("out.StepAccount.AvailableSteps = %d, want 500", out.StepAccount.AvailableSteps)
	}
	if out.NextChest.ID != 6001 {
		t.Errorf("out.NextChest.ID = %d, want 6001", out.NextChest.ID)
	}
	// stub picker / log / claim 也不应被调用
	if picker.pickCalls != 0 {
		t.Errorf("picker.pickCalls = %d, want 0", picker.pickCalls)
	}
	if logR.createCalls != nil {
		t.Errorf("logRepo.Create should NOT be called on cached replay; got %d calls", len(logR.createCalls))
	}
	if idem.claimCalls != 0 {
		t.Errorf("idem.ClaimPending should NOT be called on cached replay; got %d", idem.claimCalls)
	}
}

// 3. ChestNotFound_4001: chestRepo.FindByUserIDForUpdate 返 ErrChestNotFound → 4001
func TestChestService_OpenChest_ChestNotFound_4001(t *testing.T) {
	idem := happyIdempotencyRepo()
	chestR := &stubOpenChestChestRepo{
		findForUpdateFn: func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			return nil, mysql.ErrChestNotFound
		},
	}
	svc := fixtureService(t, idem, chestR, happyStepRepo(), happyCosmeticRepo(), happyLogRepo(), happyPicker())

	out, err := svc.OpenChest(context.Background(), service.OpenChestInput{UserID: 1001, IdempotencyKey: "k"})
	if out != nil {
		t.Errorf("out should be nil on error")
	}
	appErr, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err = %v, want *AppError", err)
	}
	if appErr.Code != apperror.ErrChestNotFound {
		t.Errorf("err.Code = %d, want %d (4001)", appErr.Code, apperror.ErrChestNotFound)
	}
}

// 4. ChestNotUnlockable_4002: chest.UnlockAt 未到 → 4002
func TestChestService_OpenChest_ChestNotUnlockable_4002(t *testing.T) {
	idem := happyIdempotencyRepo()
	chestR := &stubOpenChestChestRepo{
		findForUpdateFn: func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			// unlockAt 在 future → counting，不可开
			return &mysql.UserChest{ID: 5001, UserID: 1001, Status: 1, UnlockAt: nowUTC().Add(5 * time.Minute), OpenCostSteps: 1000, Version: 0}, nil
		},
	}
	svc := fixtureService(t, idem, chestR, happyStepRepo(), happyCosmeticRepo(), happyLogRepo(), happyPicker())

	_, err := svc.OpenChest(context.Background(), service.OpenChestInput{UserID: 1001, IdempotencyKey: "k"})
	appErr, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err = %v, want *AppError", err)
	}
	if appErr.Code != apperror.ErrChestNotUnlocked {
		t.Errorf("err.Code = %d, want %d (4002)", appErr.Code, apperror.ErrChestNotUnlocked)
	}
}

// 5. InsufficientSteps_3002: available_steps < 1000 → 3002
func TestChestService_OpenChest_InsufficientSteps_3002(t *testing.T) {
	idem := happyIdempotencyRepo()
	stepR := &stubOpenChestStepAccountRepo{
		findForUpdateFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1001, TotalSteps: 500, AvailableSteps: 500, ConsumedSteps: 0, Version: 3}, nil
		},
	}
	svc := fixtureService(t, idem, happyChestRepo(), stepR, happyCosmeticRepo(), happyLogRepo(), happyPicker())

	_, err := svc.OpenChest(context.Background(), service.OpenChestInput{UserID: 1001, IdempotencyKey: "k"})
	appErr, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err = %v, want *AppError", err)
	}
	if appErr.Code != apperror.ErrInsufficientSteps {
		t.Errorf("err.Code = %d, want %d (3002)", appErr.Code, apperror.ErrInsufficientSteps)
	}
}

// 6. StepAccountNotFound_1009: V1 §7.2 钦定 "account 行缺失视为数据完整性异常 = 1009"，**非** 3002
func TestChestService_OpenChest_StepAccountNotFound_1009(t *testing.T) {
	idem := happyIdempotencyRepo()
	stepR := &stubOpenChestStepAccountRepo{
		findForUpdateFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return nil, mysql.ErrStepAccountNotFound
		},
	}
	svc := fixtureService(t, idem, happyChestRepo(), stepR, happyCosmeticRepo(), happyLogRepo(), happyPicker())

	_, err := svc.OpenChest(context.Background(), service.OpenChestInput{UserID: 1001, IdempotencyKey: "k"})
	appErr, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err = %v, want *AppError", err)
	}
	if appErr.Code != apperror.ErrServiceBusy {
		t.Errorf("err.Code = %d, want %d (1009 ErrServiceBusy)", appErr.Code, apperror.ErrServiceBusy)
	}
}

// 7. OptimisticLockFails_1009: Spend 返 ErrStepAccountVersionMismatch → 1009
func TestChestService_OpenChest_OptimisticLockFails_1009(t *testing.T) {
	idem := happyIdempotencyRepo()
	stepR := &stubOpenChestStepAccountRepo{
		findForUpdateFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1001, TotalSteps: 1500, AvailableSteps: 1500, ConsumedSteps: 0, Version: 3}, nil
		},
		spendFn: func(ctx context.Context, userID uint64, amount uint64, expectedVersion uint64) error {
			return mysql.ErrStepAccountVersionMismatch
		},
	}
	svc := fixtureService(t, idem, happyChestRepo(), stepR, happyCosmeticRepo(), happyLogRepo(), happyPicker())

	_, err := svc.OpenChest(context.Background(), service.OpenChestInput{UserID: 1001, IdempotencyKey: "k"})
	appErr, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err = %v, want *AppError", err)
	}
	if appErr.Code != apperror.ErrServiceBusy {
		t.Errorf("err.Code = %d, want %d (1009)", appErr.Code, apperror.ErrServiceBusy)
	}
}

// 8. NoEnabledCosmetic_1009: seed 未执行 → 1009
func TestChestService_OpenChest_NoEnabledCosmetic_1009(t *testing.T) {
	idem := happyIdempotencyRepo()
	cosm := &stubCosmeticItemRepo{
		listFn: func(ctx context.Context) ([]mysql.CosmeticItem, error) {
			return []mysql.CosmeticItem{}, nil // empty
		},
	}
	svc := fixtureService(t, idem, happyChestRepo(), happyStepRepo(), cosm, happyLogRepo(), happyPicker())

	_, err := svc.OpenChest(context.Background(), service.OpenChestInput{UserID: 1001, IdempotencyKey: "k"})
	appErr, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err = %v, want *AppError", err)
	}
	if appErr.Code != apperror.ErrServiceBusy {
		t.Errorf("err.Code = %d, want %d (1009)", appErr.Code, apperror.ErrServiceBusy)
	}
}

// 9. IdempotencyClaim_ExistingRow_ShortCircuitReplay:
// 事务首条 ClaimPending 返 affected=0 → 短路读 success → 返 cached output。
// 其他 repo (chest / step / cosmetic) 都不应被调用。
func TestChestService_OpenChest_IdempotencyClaim_ExistingRow_ShortCircuitReplay(t *testing.T) {
	cachedJSON := []byte(`{"code":0,"message":"ok","data":{"reward":{"userCosmeticItemId":0,"cosmeticItemId":25,"name":"红色帽子","slot":1,"rarity":1,"assetUrl":"","iconUrl":""},"stepAccount":{"totalSteps":1500,"availableSteps":500,"consumedSteps":1000},"nextChest":{"id":7001,"unlockAt":"2026-04-23T11:00:00Z","openCostSteps":1000}}}`)
	findCallCount := 0
	idem := &stubIdempotencyRepo{
		findFn: func(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error) {
			findCallCount++
			if findCallCount == 1 {
				// 首次（autocommit handler 入口预检）→ NotFound（MVCC pending 不可见）
				return nil, mysql.ErrIdempotencyRecordNotFound
			}
			// 第二次（事务内步骤 5b 短路 SELECT）→ 命中 success
			return &mysql.IdempotencyRecord{
				ID: 100, UserID: 1001, IdempotencyKey: key,
				Status: mysql.IdempotencyStatusSuccess, ResponseJSON: cachedJSON,
			}, nil
		},
		claimFn: func(ctx context.Context, userID uint64, key string) (int64, error) {
			return 0, nil // 行已存在
		},
	}
	chestR := &stubOpenChestChestRepo{
		findForUpdateFn: func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			t.Fatal("chestRepo.FindByUserIDForUpdate should NOT be called in 5b short-circuit replay")
			return nil, nil
		},
	}
	stepR := &stubOpenChestStepAccountRepo{
		findForUpdateFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			t.Fatal("stepRepo.FindByUserIDForUpdate should NOT be called in 5b short-circuit replay")
			return nil, nil
		},
	}
	cosm := &stubCosmeticItemRepo{
		listFn: func(ctx context.Context) ([]mysql.CosmeticItem, error) {
			t.Fatal("cosmeticItemRepo.List should NOT be called in 5b short-circuit replay")
			return nil, nil
		},
	}
	svc := fixtureService(t, idem, chestR, stepR, cosm, happyLogRepo(), happyPicker())

	out, err := svc.OpenChest(context.Background(), service.OpenChestInput{UserID: 1001, IdempotencyKey: "k"})
	if err != nil {
		t.Fatalf("OpenChest: %v", err)
	}
	if out == nil {
		t.Fatal("out = nil")
	}
	if out.Reward.CosmeticItemID != 25 {
		t.Errorf("out.Reward.CosmeticItemID = %d, want 25 (from cached short-circuit)", out.Reward.CosmeticItemID)
	}
	if idem.claimCalls != 1 {
		t.Errorf("idem.claimCalls = %d, want 1 (事务首条 ClaimPending)", idem.claimCalls)
	}
}

// 10. WeightedPickDistribution: 用确定性 seed 验证抽样命中正确 item。
// 此 case 是 service 层"picker 注入 + service 内 weightedItems 转换"链路的端到端断言；
// 详细分布断言由 random/weighted_test.go 覆盖，本 case 只验证"service 把 picker 返回 index
// 正确转回 cosmetic_items.id"。
func TestChestService_OpenChest_WeightedPicker_IndexReverseMapsToItem(t *testing.T) {
	idem := happyIdempotencyRepo()
	cosm := &stubCosmeticItemRepo{
		listFn: func(ctx context.Context) ([]mysql.CosmeticItem, error) {
			return []mysql.CosmeticItem{
				{ID: 24, Name: "scarf", Rarity: 2, DropWeight: 30},
				{ID: 25, Name: "hat", Rarity: 1, DropWeight: 50},
				{ID: 26, Name: "glove", Rarity: 3, DropWeight: 20},
			}, nil
		},
	}
	picker := &stubWeightedPicker{
		pickFn: func(items []random.WeightedItem) (int, error) {
			// stub picker 强制返 index 2（最后一个 = glove）
			// 验证 service 接收的 items slice 顺序与 cosmetic_items 一致
			expected := []random.WeightedItem{{Weight: 30}, {Weight: 50}, {Weight: 20}}
			if !reflect.DeepEqual(items, expected) {
				t.Errorf("picker 收到 items = %+v, want %+v (顺序应与 cosmetic_items.ListEnabledForWeightedPick 一致)", items, expected)
			}
			return 2, nil
		},
	}
	svc := fixtureService(t, idem, happyChestRepo(), happyStepRepo(), cosm, happyLogRepo(), picker)

	out, err := svc.OpenChest(context.Background(), service.OpenChestInput{UserID: 1001, IdempotencyKey: "k"})
	if err != nil {
		t.Fatalf("OpenChest: %v", err)
	}
	if out.Reward.CosmeticItemID != 26 {
		t.Errorf("out.Reward.CosmeticItemID = %d, want 26 (picker 返 index=2 → glove)", out.Reward.CosmeticItemID)
	}
	if out.Reward.Rarity != 3 {
		t.Errorf("out.Reward.Rarity = %d, want 3", out.Reward.Rarity)
	}
}

// 11. CachedReplayUnreadable_1009: cached.Status='pending' 直接返 1009（兜底 r11；理论 MVCC 不可见，
// 但 driver 异常下 service 必须按 1009 走，**禁止** 1008）
func TestChestService_OpenChest_CachedPending_Returns1009(t *testing.T) {
	idem := &stubIdempotencyRepo{
		findFn: func(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error) {
			return &mysql.IdempotencyRecord{
				ID: 1, UserID: 1001, IdempotencyKey: key,
				Status: mysql.IdempotencyStatusPending, ResponseJSON: nil,
			}, nil
		},
	}
	svc := fixtureService(t, idem, happyChestRepo(), happyStepRepo(), happyCosmeticRepo(), happyLogRepo(), happyPicker())

	_, err := svc.OpenChest(context.Background(), service.OpenChestInput{UserID: 1001, IdempotencyKey: "k"})
	appErr, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err = %v, want *AppError", err)
	}
	if appErr.Code != apperror.ErrServiceBusy {
		t.Errorf("err.Code = %d, want %d (1009, **非** 1008 r15)", appErr.Code, apperror.ErrServiceBusy)
	}
	// 检验：禁止翻译为 1008
	if appErr.Code == apperror.ErrIdempotencyConflict {
		t.Errorf("err.Code = 1008 ErrIdempotencyConflict; V1 §7.2 r11/r15 锁定本接口不应触发 1008")
	}
}

// 12. UserID_Zero_Returns1009: handler 兜底 / service 入口校验
func TestChestService_OpenChest_UserIDZero_Returns1009(t *testing.T) {
	idem := happyIdempotencyRepo()
	svc := fixtureService(t, idem, happyChestRepo(), happyStepRepo(), happyCosmeticRepo(), happyLogRepo(), happyPicker())

	_, err := svc.OpenChest(context.Background(), service.OpenChestInput{UserID: 0, IdempotencyKey: "k"})
	appErr, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err = %v, want *AppError", err)
	}
	if appErr.Code != apperror.ErrServiceBusy {
		t.Errorf("err.Code = %d, want %d (1009)", appErr.Code, apperror.ErrServiceBusy)
	}
}

// 13. IdempotencyDBError_Returns1009: FindByUserIDAndKey 返非 NotFound DB error → 1009
func TestChestService_OpenChest_IdempotencyFindDBError_Returns1009(t *testing.T) {
	idem := &stubIdempotencyRepo{
		findFn: func(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error) {
			return nil, stderrors.New("synthetic db error")
		},
	}
	svc := fixtureService(t, idem, happyChestRepo(), happyStepRepo(), happyCosmeticRepo(), happyLogRepo(), happyPicker())

	_, err := svc.OpenChest(context.Background(), service.OpenChestInput{UserID: 1001, IdempotencyKey: "k"})
	appErr, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err = %v, want *AppError", err)
	}
	if appErr.Code != apperror.ErrServiceBusy {
		t.Errorf("err.Code = %d, want 1009", appErr.Code)
	}
}
