package service_test

import (
	"context"
	stderrors "errors"
	"math"
	"strings"
	"testing"
	"time"

	"github.com/huing/cat/server/internal/infra/config"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// ============================================================
// stub repo / stub txMgr（每个 stub 必须实装完整 interface）
// 与 4.6 / 4.8 同模式：通过 fn 字段让每个 case 自定义返回。
// ============================================================

type stubStepStepAccountRepo struct {
	findByUserIDFn  func(ctx context.Context, userID uint64) (*mysql.StepAccount, error)
	updateBalanceFn func(ctx context.Context, userID uint64, delta int32, expectedVersion uint64) error
	updateCalls     int
	lastUpdateDelta int32
	lastUpdateVer   uint64
}

func (s *stubStepStepAccountRepo) Create(ctx context.Context, a *mysql.StepAccount) error {
	panic("stubStepStepAccountRepo.Create not configured (step_service should not call it)")
}

func (s *stubStepStepAccountRepo) FindByUserID(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
	if s.findByUserIDFn == nil {
		return nil, stderrors.New("stub: findByUserIDFn not set")
	}
	return s.findByUserIDFn(ctx, userID)
}

func (s *stubStepStepAccountRepo) UpdateBalance(ctx context.Context, userID uint64, delta int32, expectedVersion uint64) error {
	s.updateCalls++
	s.lastUpdateDelta = delta
	s.lastUpdateVer = expectedVersion
	if s.updateBalanceFn == nil {
		return nil
	}
	return s.updateBalanceFn(ctx, userID, delta, expectedVersion)
}

type stubStepSyncLogRepo struct {
	findLatestFn  func(ctx context.Context, userID uint64, syncDate time.Time) (*mysql.StepSyncLog, error)
	sumAcceptedFn func(ctx context.Context, userID uint64, syncDate time.Time) (int64, error)
	createFn      func(ctx context.Context, log *mysql.StepSyncLog) error
	createCalls   int
	lastCreated   *mysql.StepSyncLog
}

func (s *stubStepSyncLogRepo) Create(ctx context.Context, log *mysql.StepSyncLog) error {
	s.createCalls++
	s.lastCreated = log
	if s.createFn == nil {
		return nil
	}
	return s.createFn(ctx, log)
}

func (s *stubStepSyncLogRepo) FindLatestByUserAndDate(ctx context.Context, userID uint64, syncDate time.Time) (*mysql.StepSyncLog, error) {
	if s.findLatestFn == nil {
		return nil, mysql.ErrStepSyncLogNotFound
	}
	return s.findLatestFn(ctx, userID, syncDate)
}

func (s *stubStepSyncLogRepo) SumAcceptedDeltaByUserAndDate(ctx context.Context, userID uint64, syncDate time.Time) (int64, error) {
	if s.sumAcceptedFn == nil {
		return 0, nil
	}
	return s.sumAcceptedFn(ctx, userID, syncDate)
}

// stubStepTxMgr 直接调 fn（不真起事务）；事务边界由集成测试覆盖。
type stubStepTxMgr struct{}

func (s *stubStepTxMgr) WithTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	return fn(ctx)
}

// buildStepService 用 stub repo 构造 StepService。每个 case 独立设置 fn。
func buildStepService(
	accRepo mysql.StepAccountRepo,
	logRepo mysql.StepSyncLogRepo,
	cfg config.StepsConfig,
) service.StepService {
	return service.NewStepService(&stubStepTxMgr{}, accRepo, logRepo, cfg)
}

// fixedSyncDate 给所有 case 共用一个固定 syncDate（避免 time.Now 不稳）。
var fixedSyncDate = time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)

// ============================================================
// 9 个 case
// ============================================================

// 1. FirstTimeSync_DeltaEqualsClientTotal:
// FindLatest → ErrNotFound；clientTotal = 100；预期 delta = 100，
// UpdateBalance 调 (delta=100, version=v0)，Create sync_log AcceptedDeltaSteps = 100，
// out.StepAccount.Total = (account.Total + 100)
func TestStepService_SyncSteps_FirstTimeSync_DeltaEqualsClientTotal(t *testing.T) {
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1001, TotalSteps: 0, AvailableSteps: 0, ConsumedSteps: 0, Version: 0}, nil
		},
	}
	logRepo := &stubStepSyncLogRepo{
		// findLatestFn 不设 → 默认返 ErrStepSyncLogNotFound
	}

	svc := buildStepService(accRepo, logRepo, config.StepsConfig{})
	out, err := svc.SyncSteps(context.Background(), service.SyncStepsInput{
		UserID:           1001,
		SyncDate:         fixedSyncDate,
		ClientTotalSteps: 100,
		MotionState:      2,
		ClientTimestamp:  1714560000000,
	})
	if err != nil {
		t.Fatalf("SyncSteps: %v", err)
	}
	if out.AcceptedDeltaSteps != 100 {
		t.Errorf("AcceptedDeltaSteps = %d, want 100", out.AcceptedDeltaSteps)
	}
	if out.StepAccount.TotalSteps != 100 {
		t.Errorf("TotalSteps = %d, want 100", out.StepAccount.TotalSteps)
	}
	if out.StepAccount.AvailableSteps != 100 {
		t.Errorf("AvailableSteps = %d, want 100", out.StepAccount.AvailableSteps)
	}
	if accRepo.updateCalls != 1 {
		t.Errorf("UpdateBalance calls = %d, want 1", accRepo.updateCalls)
	}
	if accRepo.lastUpdateDelta != 100 {
		t.Errorf("UpdateBalance delta = %d, want 100", accRepo.lastUpdateDelta)
	}
	if accRepo.lastUpdateVer != 0 {
		t.Errorf("UpdateBalance version = %d, want 0", accRepo.lastUpdateVer)
	}
	if logRepo.createCalls != 1 {
		t.Errorf("Create calls = %d, want 1", logRepo.createCalls)
	}
	if logRepo.lastCreated.AcceptedDeltaSteps != 100 {
		t.Errorf("sync_log.AcceptedDeltaSteps = %d, want 100", logRepo.lastCreated.AcceptedDeltaSteps)
	}
	if logRepo.lastCreated.Source != 1 {
		t.Errorf("sync_log.Source = %d, want 1 (healthkit)", logRepo.lastCreated.Source)
	}
}

// 2. SecondSync_DeltaEqualsDifference:
// FindLatest → last.ClientTotal = 100；in.ClientTotal = 180；预期 delta = 80。
func TestStepService_SyncSteps_SecondSync_DeltaEqualsDifference(t *testing.T) {
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1001, TotalSteps: 100, AvailableSteps: 100, ConsumedSteps: 0, Version: 1}, nil
		},
	}
	logRepo := &stubStepSyncLogRepo{
		findLatestFn: func(ctx context.Context, userID uint64, syncDate time.Time) (*mysql.StepSyncLog, error) {
			return &mysql.StepSyncLog{ID: 100, UserID: 1001, SyncDate: syncDate, ClientTotalSteps: 100, AcceptedDeltaSteps: 100}, nil
		},
		sumAcceptedFn: func(ctx context.Context, userID uint64, syncDate time.Time) (int64, error) {
			return 100, nil
		},
	}

	svc := buildStepService(accRepo, logRepo, config.StepsConfig{})
	out, err := svc.SyncSteps(context.Background(), service.SyncStepsInput{
		UserID: 1001, SyncDate: fixedSyncDate, ClientTotalSteps: 180, MotionState: 2, ClientTimestamp: 1714560000000,
	})
	if err != nil {
		t.Fatalf("SyncSteps: %v", err)
	}
	if out.AcceptedDeltaSteps != 80 {
		t.Errorf("AcceptedDeltaSteps = %d, want 80", out.AcceptedDeltaSteps)
	}
	if out.StepAccount.TotalSteps != 180 {
		t.Errorf("TotalSteps = %d, want 180", out.StepAccount.TotalSteps)
	}
	if accRepo.lastUpdateDelta != 80 {
		t.Errorf("UpdateBalance delta = %d, want 80", accRepo.lastUpdateDelta)
	}
	if accRepo.lastUpdateVer != 1 {
		t.Errorf("UpdateBalance version = %d, want 1", accRepo.lastUpdateVer)
	}
	if logRepo.lastCreated.AcceptedDeltaSteps != 80 {
		t.Errorf("sync_log.AcceptedDeltaSteps = %d, want 80", logRepo.lastCreated.AcceptedDeltaSteps)
	}
}

// 3. Backward_DeltaIsZero_LogStillWritten:
// FindLatest → last = 200；in = 100（倒退）；预期 delta = 0；
// UpdateBalance 仍调（delta=0，version+1）；Create sync_log AcceptedDeltaSteps = 0。
func TestStepService_SyncSteps_Backward_DeltaIsZero_LogStillWritten(t *testing.T) {
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1001, TotalSteps: 200, AvailableSteps: 200, ConsumedSteps: 0, Version: 2}, nil
		},
	}
	logRepo := &stubStepSyncLogRepo{
		findLatestFn: func(ctx context.Context, userID uint64, syncDate time.Time) (*mysql.StepSyncLog, error) {
			return &mysql.StepSyncLog{ClientTotalSteps: 200}, nil
		},
		sumAcceptedFn: func(ctx context.Context, userID uint64, syncDate time.Time) (int64, error) {
			return 200, nil
		},
	}

	svc := buildStepService(accRepo, logRepo, config.StepsConfig{})
	out, err := svc.SyncSteps(context.Background(), service.SyncStepsInput{
		UserID: 1001, SyncDate: fixedSyncDate, ClientTotalSteps: 100, MotionState: 1, ClientTimestamp: 1714560000000,
	})
	if err != nil {
		t.Fatalf("SyncSteps: %v", err)
	}
	if out.AcceptedDeltaSteps != 0 {
		t.Errorf("AcceptedDeltaSteps = %d, want 0 (倒退)", out.AcceptedDeltaSteps)
	}
	if out.StepAccount.TotalSteps != 200 {
		t.Errorf("TotalSteps = %d, want 200 (delta=0 不变)", out.StepAccount.TotalSteps)
	}
	if accRepo.updateCalls != 1 {
		t.Errorf("UpdateBalance calls = %d, want 1（仍调，version+1）", accRepo.updateCalls)
	}
	if accRepo.lastUpdateDelta != 0 {
		t.Errorf("UpdateBalance delta = %d, want 0", accRepo.lastUpdateDelta)
	}
	if logRepo.createCalls != 1 {
		t.Errorf("Create calls = %d, want 1（append-only 审计纪律）", logRepo.createCalls)
	}
	if logRepo.lastCreated.AcceptedDeltaSteps != 0 {
		t.Errorf("sync_log.AcceptedDeltaSteps = %d, want 0", logRepo.lastCreated.AcceptedDeltaSteps)
	}
}

// 4. Duplicate_DeltaIsZero_LogStillWritten:
// FindLatest → last = 200；in = 200（重复）；预期 delta = 0。
func TestStepService_SyncSteps_Duplicate_DeltaIsZero_LogStillWritten(t *testing.T) {
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1001, TotalSteps: 200, AvailableSteps: 200, Version: 1}, nil
		},
	}
	logRepo := &stubStepSyncLogRepo{
		findLatestFn: func(ctx context.Context, userID uint64, syncDate time.Time) (*mysql.StepSyncLog, error) {
			return &mysql.StepSyncLog{ClientTotalSteps: 200}, nil
		},
	}

	svc := buildStepService(accRepo, logRepo, config.StepsConfig{})
	out, err := svc.SyncSteps(context.Background(), service.SyncStepsInput{
		UserID: 1001, SyncDate: fixedSyncDate, ClientTotalSteps: 200, MotionState: 1, ClientTimestamp: 1714560000000,
	})
	if err != nil {
		t.Fatalf("SyncSteps: %v", err)
	}
	if out.AcceptedDeltaSteps != 0 {
		t.Errorf("AcceptedDeltaSteps = %d, want 0 (重复 sync)", out.AcceptedDeltaSteps)
	}
	if logRepo.createCalls != 1 {
		t.Errorf("Create calls = %d, want 1（append-only）", logRepo.createCalls)
	}
}

// 5. CrossDay_NoLatestForNewDate:
// FindLatest（new sync_date）→ ErrNotFound；in.ClientTotal = 50（新一天小步数）；
// 预期 delta = 50（首次语义；**不**读上一天 lastTotal）。
func TestStepService_SyncSteps_CrossDay_NoLatestForNewDate(t *testing.T) {
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1001, TotalSteps: 1000, AvailableSteps: 1000, Version: 5}, nil
		},
	}
	logRepo := &stubStepSyncLogRepo{
		// 新一天首次 → 默认返 ErrStepSyncLogNotFound
	}

	newDay := fixedSyncDate.Add(24 * time.Hour)
	svc := buildStepService(accRepo, logRepo, config.StepsConfig{})
	out, err := svc.SyncSteps(context.Background(), service.SyncStepsInput{
		UserID: 1001, SyncDate: newDay, ClientTotalSteps: 50, MotionState: 2, ClientTimestamp: 1714560000000,
	})
	if err != nil {
		t.Fatalf("SyncSteps: %v", err)
	}
	if out.AcceptedDeltaSteps != 50 {
		t.Errorf("AcceptedDeltaSteps = %d, want 50（新一天首次 = clientTotal）", out.AcceptedDeltaSteps)
	}
	if out.StepAccount.TotalSteps != 1050 {
		t.Errorf("TotalSteps = %d, want 1050 (1000 + 50)", out.StepAccount.TotalSteps)
	}
}

// 6. TxFailure_RollsBack:
// UpdateBalance 返 db error；预期 service 返 1009；sync_log Create **不**被调
// （Create 在 UpdateBalance 之后；mock 设置 stepSyncLogRepo.Create 不期待调用）。
func TestStepService_SyncSteps_TxFailure_RollsBack(t *testing.T) {
	dbErr := stderrors.New("simulated UpdateBalance failure")
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1001, TotalSteps: 0, Version: 0}, nil
		},
		updateBalanceFn: func(ctx context.Context, userID uint64, delta int32, expectedVersion uint64) error {
			return dbErr
		},
	}
	logRepo := &stubStepSyncLogRepo{
		// Create 不期待调用；createFn 不设置 → 默认 nil 也行；createCalls 验证
	}

	svc := buildStepService(accRepo, logRepo, config.StepsConfig{})
	out, err := svc.SyncSteps(context.Background(), service.SyncStepsInput{
		UserID: 1001, SyncDate: fixedSyncDate, ClientTotalSteps: 100, MotionState: 2, ClientTimestamp: 1714560000000,
	})
	if out != nil {
		t.Errorf("out = %+v, want nil on UpdateBalance failure", out)
	}
	if got := apperror.Code(err); got != apperror.ErrServiceBusy {
		t.Errorf("apperror.Code = %d, want %d (1009)", got, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, dbErr) {
		t.Errorf("err 链不含 dbErr; err = %v", err)
	}
	if logRepo.createCalls != 0 {
		t.Errorf("Create calls = %d, want 0 (UpdateBalance 失败后不应调 Create)", logRepo.createCalls)
	}
}

// 7. SingleCapTruncation_DeltaCapped:
// FindLatest → last = 0；in.ClientTotal = 10000；预期 delta 截断为 5000（service 用 default 5000）。
func TestStepService_SyncSteps_SingleCapTruncation_DeltaCapped(t *testing.T) {
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1001, TotalSteps: 0, Version: 0}, nil
		},
	}
	logRepo := &stubStepSyncLogRepo{
		// 首次 sync → 默认 ErrStepSyncLogNotFound
	}

	svc := buildStepService(accRepo, logRepo, config.StepsConfig{})
	out, err := svc.SyncSteps(context.Background(), service.SyncStepsInput{
		UserID: 1001, SyncDate: fixedSyncDate, ClientTotalSteps: 10000, MotionState: 3, ClientTimestamp: 1714560000000,
	})
	if err != nil {
		t.Fatalf("SyncSteps: %v", err)
	}
	if out.AcceptedDeltaSteps != 5000 {
		t.Errorf("AcceptedDeltaSteps = %d, want 5000 (截断 default)", out.AcceptedDeltaSteps)
	}
	if accRepo.lastUpdateDelta != 5000 {
		t.Errorf("UpdateBalance delta = %d, want 5000 (截断后)", accRepo.lastUpdateDelta)
	}
	if logRepo.lastCreated.AcceptedDeltaSteps != 5000 {
		t.Errorf("sync_log.AcceptedDeltaSteps = %d, want 5000", logRepo.lastCreated.AcceptedDeltaSteps)
	}
}

// 8. DailyCapExceeded_DeltaZero_Returns3001:
// SumAccepted → 49000；rawDelta = 4000（< singleCap 5000，但 49000+4000=53000 > dailyCap 50000）；
// 预期 delta = 0；Create sync_log AcceptedDeltaSteps = 0；UpdateBalance(delta=0, version+1) 调；
// service 返 *AppError(3001)；事务 commit（不 rollback）。
func TestStepService_SyncSteps_DailyCapExceeded_DeltaZero_Returns3001(t *testing.T) {
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1001, TotalSteps: 49000, AvailableSteps: 49000, Version: 10}, nil
		},
	}
	logRepo := &stubStepSyncLogRepo{
		findLatestFn: func(ctx context.Context, userID uint64, syncDate time.Time) (*mysql.StepSyncLog, error) {
			return &mysql.StepSyncLog{ClientTotalSteps: 49000}, nil
		},
		sumAcceptedFn: func(ctx context.Context, userID uint64, syncDate time.Time) (int64, error) {
			return 49000, nil
		},
	}

	svc := buildStepService(accRepo, logRepo, config.StepsConfig{})
	out, err := svc.SyncSteps(context.Background(), service.SyncStepsInput{
		UserID: 1001, SyncDate: fixedSyncDate, ClientTotalSteps: 53000, MotionState: 2, ClientTimestamp: 1714560000000,
	})
	if out != nil {
		t.Errorf("out = %+v, want nil on dailyCap exceeded", out)
	}
	if got := apperror.Code(err); got != apperror.ErrStepSyncInvalid {
		t.Errorf("apperror.Code = %d, want %d (3001)", got, apperror.ErrStepSyncInvalid)
	}
	// 关键：UpdateBalance + Create sync_log 仍被调（事务 commit；审计行落库）
	if accRepo.updateCalls != 1 {
		t.Errorf("UpdateBalance calls = %d, want 1（封顶仍 commit）", accRepo.updateCalls)
	}
	if accRepo.lastUpdateDelta != 0 {
		t.Errorf("UpdateBalance delta = %d, want 0 (封顶 delta=0)", accRepo.lastUpdateDelta)
	}
	if logRepo.createCalls != 1 {
		t.Errorf("Create calls = %d, want 1（封顶审计行）", logRepo.createCalls)
	}
	if logRepo.lastCreated.AcceptedDeltaSteps != 0 {
		t.Errorf("sync_log.AcceptedDeltaSteps = %d, want 0", logRepo.lastCreated.AcceptedDeltaSteps)
	}
}

// 9. DailyCapNonSticky_PrevAt50000_DeltaZero_Returns0:
// SumAccepted → 50000（已封顶）；in.ClientTotal == last.ClientTotal（重复 sync）→ rawDelta = 0；
// 50000+0 = 50000 **不超** 50000（用 `>` 而非 `>=`）→ 不触发 3001；
// service 返 nil err（成功 code=0）；out.AcceptedDeltaSteps = 0。
//
// **关键**：3001 非粘性。一旦用户已达上限，重复 / 倒退 sync 不应再触发 3001
// （curDelta=0 + prev=50000 = 50000 不超 50000；用 `>` 而非 `>=`）。
func TestStepService_SyncSteps_DailyCapNonSticky_PrevAt50000_DeltaZero_Returns0(t *testing.T) {
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1001, TotalSteps: 50000, AvailableSteps: 50000, Version: 20}, nil
		},
	}
	logRepo := &stubStepSyncLogRepo{
		findLatestFn: func(ctx context.Context, userID uint64, syncDate time.Time) (*mysql.StepSyncLog, error) {
			return &mysql.StepSyncLog{ClientTotalSteps: 60000}, nil // 上次报 60000，已被截断为 5000
		},
		sumAcceptedFn: func(ctx context.Context, userID uint64, syncDate time.Time) (int64, error) {
			return 50000, nil // 当日已累计 50000（封顶）
		},
	}

	svc := buildStepService(accRepo, logRepo, config.StepsConfig{})
	out, err := svc.SyncSteps(context.Background(), service.SyncStepsInput{
		// 重复 sync 同 clientTotal → rawDelta = 0
		UserID: 1001, SyncDate: fixedSyncDate, ClientTotalSteps: 60000, MotionState: 1, ClientTimestamp: 1714560000000,
	})
	if err != nil {
		t.Fatalf("SyncSteps: %v, want nil err（非粘性 3001）", err)
	}
	if out == nil {
		t.Fatal("out = nil, want non-nil")
	}
	if out.AcceptedDeltaSteps != 0 {
		t.Errorf("AcceptedDeltaSteps = %d, want 0", out.AcceptedDeltaSteps)
	}
	// 验非粘性：50000 + 0 = 50000，**不超** 50000（用 `>` 不用 `>=`）
}

// 10. NewStepService_OversizedSingleSyncCap_Panics:
// Story 7.3 review r1 [P2] 修复后行为：YAML 配 single_sync_cap > math.MaxInt32 →
// 构造函数 panic（fail-fast）；不再静默 narrow 成负值导致余额被减少。
//
// 反例（修复前）：cfg.SingleSyncCap = 5_000_000_000 → int32 cast wrap 为
// 705_032_704（实际 wrap 取决于值；够大就会成负数）→ delta > cap 永远命中 →
// truncate 为 wrap 后的负值 → UpdateBalance 写负 delta → 余额被扣减。
func TestStepService_NewStepService_OversizedSingleSyncCap_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("NewStepService 应 panic（cfg.SingleSyncCap > MaxInt32）；实际未 panic")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic 值不是 string: %T", r)
		}
		if !strings.Contains(msg, "single_sync_cap") {
			t.Errorf("panic msg 不含 single_sync_cap 定位字段: %q", msg)
		}
	}()
	_ = service.NewStepService(
		&stubStepTxMgr{},
		&stubStepStepAccountRepo{},
		&stubStepSyncLogRepo{},
		config.StepsConfig{SingleSyncCap: int64(math.MaxInt32) + 1},
	)
}

// 11. NewStepService_NegativeSingleSyncCap_Panics:
// YAML 配 single_sync_cap < 0 → 构造函数 panic。负 cap 没有业务语义。
func TestStepService_NewStepService_NegativeSingleSyncCap_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("NewStepService 应 panic（cfg.SingleSyncCap < 0）；实际未 panic")
		}
	}()
	_ = service.NewStepService(
		&stubStepTxMgr{},
		&stubStepStepAccountRepo{},
		&stubStepSyncLogRepo{},
		config.StepsConfig{SingleSyncCap: -1},
	)
}

// 12. NewStepService_NegativeDailyCap_Panics:
// YAML 配 daily_cap < 0 → 构造函数 panic。
func TestStepService_NewStepService_NegativeDailyCap_Panics(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("NewStepService 应 panic（cfg.DailyCap < 0）；实际未 panic")
		}
	}()
	_ = service.NewStepService(
		&stubStepTxMgr{},
		&stubStepStepAccountRepo{},
		&stubStepSyncLogRepo{},
		config.StepsConfig{DailyCap: -1},
	)
}

// 13. NewStepService_MaxInt32SingleSyncCap_OK:
// YAML 配 single_sync_cap == math.MaxInt32 → 构造成功（边界值合法）。
// 防"上限改边界条件"回归（避免误用 `>=` 把合法上限也当非法）。
func TestStepService_NewStepService_MaxInt32SingleSyncCap_OK(t *testing.T) {
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewStepService 不应 panic（边界值 MaxInt32 合法）: %v", r)
		}
	}()
	_ = service.NewStepService(
		&stubStepTxMgr{},
		&stubStepStepAccountRepo{},
		&stubStepSyncLogRepo{},
		config.StepsConfig{SingleSyncCap: int64(math.MaxInt32)},
	)
}
