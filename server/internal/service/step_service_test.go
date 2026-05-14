package service_test

import (
	"context"
	stderrors "errors"
	"math"
	"strings"
	"testing"

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

// FindByUserIDForUpdate / Spend: Story 20.6 引入；step_service 不调，stub 默认 panic。
func (s *stubStepStepAccountRepo) FindByUserIDForUpdate(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
	panic("stubStepStepAccountRepo.FindByUserIDForUpdate not configured (step_service should not call it)")
}

func (s *stubStepStepAccountRepo) Spend(ctx context.Context, userID uint64, amount uint64, expectedVersion uint64) error {
	panic("stubStepStepAccountRepo.Spend not configured (step_service should not call it)")
}

type stubStepSyncLogRepo struct {
	findLatestFn  func(ctx context.Context, userID uint64, syncDate string) (*mysql.StepSyncLog, error)
	sumAcceptedFn func(ctx context.Context, userID uint64, syncDate string) (int64, error)
	maxReportedFn func(ctx context.Context, userID uint64, syncDate string) (uint64, error) // Story 7.3 review r5 [P1]
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

func (s *stubStepSyncLogRepo) FindLatestByUserAndDate(ctx context.Context, userID uint64, syncDate string) (*mysql.StepSyncLog, error) {
	if s.findLatestFn == nil {
		return nil, mysql.ErrStepSyncLogNotFound
	}
	return s.findLatestFn(ctx, userID, syncDate)
}

func (s *stubStepSyncLogRepo) SumAcceptedDeltaByUserAndDate(ctx context.Context, userID uint64, syncDate string) (int64, error) {
	if s.sumAcceptedFn == nil {
		return 0, nil
	}
	return s.sumAcceptedFn(ctx, userID, syncDate)
}

// MaxClientTotalStepsByUserAndDate 默认返 0（首次 sync 场景）。
// 有 latest 行的 case 必须显式 set maxReportedFn —— 反映真实 SQL MAX 行为。
//
// **与 sumAcceptedFn 的关系**：sumAcceptedFn 返"已入账总和"，maxReportedFn 返
// "已报告的最大累计"；在被截断 / 乱序场景下两者会**不相等**，是 r5 综合方案
// 之所以需要叠加 maxReported clamp + SUM 兜底两层防御的根本原因。
func (s *stubStepSyncLogRepo) MaxClientTotalStepsByUserAndDate(ctx context.Context, userID uint64, syncDate string) (uint64, error) {
	if s.maxReportedFn == nil {
		return 0, nil
	}
	return s.maxReportedFn(ctx, userID, syncDate)
}

// stubStepTxMgr 直接调 fn（不真起事务）；事务边界由集成测试覆盖。
type stubStepTxMgr struct{}

func (s *stubStepTxMgr) WithTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	return fn(ctx)
}

// buildStepService 用 stub repo 构造 StepService。每个 case 独立设置 fn。
//
// envName 默认 "dev"：单测用任意 SingleSyncCap / DailyCap 值（含 0 / 默认值）都允许；
// 验"prod env 拒绝 YAML cap 覆盖"语义请直接调 `service.NewStepService(..., "prod")`。
func buildStepService(
	accRepo mysql.StepAccountRepo,
	logRepo mysql.StepSyncLogRepo,
	cfg config.StepsConfig,
) service.StepService {
	return service.NewStepService(&stubStepTxMgr{}, accRepo, logRepo, cfg, "dev")
}

// fixedSyncDate 给所有 case 共用一个固定 syncDate string（YYYY-MM-DD）。
//
// **string 而非 time.Time**（Story 7.3 review r2 [P2]）：service / repo 全程
// string 透传，无时区耦合；详见 mysql.StepSyncLog 与 service.SyncStepsInput doc。
const fixedSyncDate = "2026-05-01"

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
		findLatestFn: func(ctx context.Context, userID uint64, syncDate string) (*mysql.StepSyncLog, error) {
			return &mysql.StepSyncLog{ID: 100, UserID: 1001, SyncDate: syncDate, ClientTotalSteps: 100, AcceptedDeltaSteps: 100}, nil
		},
		sumAcceptedFn: func(ctx context.Context, userID uint64, syncDate string) (int64, error) {
			return 100, nil
		},
		// Story 7.3 review r5 [P1]：maxReported = 100（与 latest.ClientTotalSteps 一致；无截断、无乱序场景下相等）
		maxReportedFn: func(ctx context.Context, userID uint64, syncDate string) (uint64, error) {
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
		findLatestFn: func(ctx context.Context, userID uint64, syncDate string) (*mysql.StepSyncLog, error) {
			return &mysql.StepSyncLog{ClientTotalSteps: 200}, nil
		},
		sumAcceptedFn: func(ctx context.Context, userID uint64, syncDate string) (int64, error) {
			return 200, nil
		},
		// r5：maxReported = 200；clientTotal=100 ≤ 200 → rawDelta=0（倒退分支）
		maxReportedFn: func(ctx context.Context, userID uint64, syncDate string) (uint64, error) {
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
		findLatestFn: func(ctx context.Context, userID uint64, syncDate string) (*mysql.StepSyncLog, error) {
			return &mysql.StepSyncLog{ClientTotalSteps: 200}, nil
		},
		// r5：maxReported = 200；clientTotal=200 ≤ 200 → rawDelta=0（重复分支）
		maxReportedFn: func(ctx context.Context, userID uint64, syncDate string) (uint64, error) {
			return 200, nil
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

	const newDay = "2026-05-02" // fixedSyncDate + 1 day（string YYYY-MM-DD）
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
		findLatestFn: func(ctx context.Context, userID uint64, syncDate string) (*mysql.StepSyncLog, error) {
			return &mysql.StepSyncLog{ClientTotalSteps: 49000}, nil
		},
		sumAcceptedFn: func(ctx context.Context, userID uint64, syncDate string) (int64, error) {
			return 49000, nil
		},
		// r5：maxReported = 49000；clientTotal=53000 > 49000 → rawDelta = 4000
		maxReportedFn: func(ctx context.Context, userID uint64, syncDate string) (uint64, error) {
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
		findLatestFn: func(ctx context.Context, userID uint64, syncDate string) (*mysql.StepSyncLog, error) {
			return &mysql.StepSyncLog{ClientTotalSteps: 60000}, nil // 上次报 60000，已被截断为 5000
		},
		sumAcceptedFn: func(ctx context.Context, userID uint64, syncDate string) (int64, error) {
			return 50000, nil // 当日已累计 50000（封顶）
		},
		// r5：maxReported = 60000（已经报告过的最大）；clientTotal=60000 ≤ 60000 → rawDelta=0（重复 sync）
		maxReportedFn: func(ctx context.Context, userID uint64, syncDate string) (uint64, error) {
			return 60000, nil
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
		"dev", // envName: dev 允许任何正值；本 case 验 MaxInt32+1 越界（独立于 prod gate）
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
		"dev",
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
		"dev",
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
		"dev",
	)
}

// 14. HealthKitReset_BaselineFollowsLatestNotMax_DeltasResume:
// **Story 7.3 review r3 [P1] 回归哨兵**：HealthKit reset/correction 场景，
// client_total_steps 在当日内出现真实下降（device reset / data correction）。
//
// 流程：
//   - 历史：累计入账 250（client_total_steps=250 → accepted=250；prevAccepted SUM=250）
//   - 现在 sync 报 105（reset 后从 100 重启，又走了 5 步）
//   - 基线 = 最近一次 sync = 105 之前的最新行（client_total=250）
//   - rawDelta = max(0, 105-250) = 0（倒退处理）
//   - SUM 兜底判断：250+0=250 > 105 → adjusted = max(0, 105-250) = 0；rawDelta 仍 0
//   - 入账：accepted=0；sync_log 落 client_total=105
//
// 然后下一次 sync 110（reset 后又 5 步）：
//   - 基线 = 最近一次 sync = 105（**关键**：r2 的 max ORDER BY 会取 250 永久卡死）
//   - rawDelta = 110-105 = 5
//   - SUM 兜底：250+5=255 > 110 → adjusted = max(0, 110-250) = 0
//
// **正确性**：reset 场景 SUM 兜底正确削回到 0（避免重复入账），但**关键**对比 r2：
// 基线已经跟着新 sync 走了，**不会**永久卡死。
//
// 这条 case 锁住"基线必须跟最近一次 sync 走"的语义；若 repo 退回 r2 的
// `client_total_steps DESC` ORDER BY，基线会取 250 → rawDelta = 0 → SUM 兜底也是 0
// → 看似"通过"但行为已退化。本 case 的 sentinel 在第二次 sync 上：
// **stub 显式返 latest.ClientTotalSteps=105**（模拟 repo 按 id DESC 取最近行的语义），
// 若 service 层缓存或调用方式改了，本 case 也立刻挂。
func TestStepService_SyncSteps_HealthKitReset_BaselineFollowsLatestNotMax_DeltasResume(t *testing.T) {
	// 模拟 reset 之后的第一次 sync：105（基线最近行 = client_total=250 历史高水位）
	t.Run("FirstSyncAfterReset_DeltaZero_SumCapAdjusted", func(t *testing.T) {
		accRepo := &stubStepStepAccountRepo{
			findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
				return &mysql.StepAccount{UserID: 1001, TotalSteps: 250, AvailableSteps: 250, Version: 1}, nil
			},
		}
		logRepo := &stubStepSyncLogRepo{
			findLatestFn: func(ctx context.Context, userID uint64, syncDate string) (*mysql.StepSyncLog, error) {
				// 基线 = 最近一次 sync（id DESC）。reset 前最后一次 = 250。
				return &mysql.StepSyncLog{ClientTotalSteps: 250}, nil
			},
			sumAcceptedFn: func(ctx context.Context, userID uint64, syncDate string) (int64, error) {
				return 250, nil
			},
			// r5：maxReported = 250（reset 前的最大）；clientTotal=105 ≤ 250 → rawDelta=0
			maxReportedFn: func(ctx context.Context, userID uint64, syncDate string) (uint64, error) {
				return 250, nil
			},
		}

		svc := buildStepService(accRepo, logRepo, config.StepsConfig{})
		out, err := svc.SyncSteps(context.Background(), service.SyncStepsInput{
			UserID: 1001, SyncDate: fixedSyncDate, ClientTotalSteps: 105, MotionState: 2, ClientTimestamp: 1714560000000,
		})
		if err != nil {
			t.Fatalf("SyncSteps after reset: %v", err)
		}
		// 倒退（105 < maxReported=250）→ rawDelta = 0；SUM 兜底也是 0
		if out.AcceptedDeltaSteps != 0 {
			t.Errorf("AcceptedDeltaSteps = %d, want 0 (reset 倒退场景)", out.AcceptedDeltaSteps)
		}
		if logRepo.lastCreated.ClientTotalSteps != 105 {
			t.Errorf("sync_log.ClientTotalSteps = %d, want 105 (落库新 reset 值)", logRepo.lastCreated.ClientTotalSteps)
		}
	})

	// reset 后第二次 sync：110。**关键**：基线必须取 105（最近行），不取 250（max）。
	t.Run("SecondSyncAfterReset_BaselineIsLatestNotMax", func(t *testing.T) {
		accRepo := &stubStepStepAccountRepo{
			findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
				return &mysql.StepAccount{UserID: 1001, TotalSteps: 250, AvailableSteps: 250, Version: 2}, nil
			},
		}
		logRepo := &stubStepSyncLogRepo{
			findLatestFn: func(ctx context.Context, userID uint64, syncDate string) (*mysql.StepSyncLog, error) {
				// 基线（latest）= 105（最近一次 sync，按 id DESC）—— r5 之后这只用于 latest != nil 判断
				return &mysql.StepSyncLog{ClientTotalSteps: 105}, nil
			},
			sumAcceptedFn: func(ctx context.Context, userID uint64, syncDate string) (int64, error) {
				// SUM 累计 = 250（reset 前）+ 0（reset 后第一次） = 250
				return 250, nil
			},
			// r5：maxReported = 250（仍是 reset 前的高水位；reset 后的 105 < 250）
			// clientTotal=110 ≤ maxReported=250 → rawDelta=0（不需要 SUM 兜底削回，r5 已经在 maxReported clamp 阶段直接削成 0）
			maxReportedFn: func(ctx context.Context, userID uint64, syncDate string) (uint64, error) {
				return 250, nil
			},
		}

		svc := buildStepService(accRepo, logRepo, config.StepsConfig{})
		out, err := svc.SyncSteps(context.Background(), service.SyncStepsInput{
			UserID: 1001, SyncDate: fixedSyncDate, ClientTotalSteps: 110, MotionState: 2, ClientTimestamp: 1714560005000,
		})
		if err != nil {
			t.Fatalf("SyncSteps second after reset: %v", err)
		}
		// r5：maxReported clamp 直接削回 0（旧/重复 sync 分支）；SUM 兜底冗余但不改变结果
		if out.AcceptedDeltaSteps != 0 {
			t.Errorf("AcceptedDeltaSteps = %d, want 0 (r5 maxReported clamp)", out.AcceptedDeltaSteps)
		}
	})

	// reset 后用户走出超过历史高水位 270：基线 = 110（最近）；rawDelta = 160；
	// SUM 兜底：250+160=410 > 270 → adjusted = max(0, 270-250) = 20。
	// **关键**：r2 方案下基线锁死 250 → rawDelta = 20；本场景 r2/r3 行为相同。
	// 但**接下来一次** sync 280：r3 基线 = 270（最近），rawDelta=10；r2 基线 = 270 也对。
	// 真正的 r2 卡死场景是 reset 后**始终**没超过 250 → r3 这里用单次 case 演示。
	t.Run("ThirdSyncAfterReset_ExceedsHistoricalMax_AccrualResumes", func(t *testing.T) {
		accRepo := &stubStepStepAccountRepo{
			findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
				return &mysql.StepAccount{UserID: 1001, TotalSteps: 250, AvailableSteps: 250, Version: 3}, nil
			},
		}
		logRepo := &stubStepSyncLogRepo{
			findLatestFn: func(ctx context.Context, userID uint64, syncDate string) (*mysql.StepSyncLog, error) {
				// latest = 110（最近 sync）。
				return &mysql.StepSyncLog{ClientTotalSteps: 110}, nil
			},
			sumAcceptedFn: func(ctx context.Context, userID uint64, syncDate string) (int64, error) {
				return 250, nil
			},
			// r5：maxReported = 250（仍是历史高水位）；clientTotal=270 > 250 → rawDelta = 20
			maxReportedFn: func(ctx context.Context, userID uint64, syncDate string) (uint64, error) {
				return 250, nil
			},
		}

		svc := buildStepService(accRepo, logRepo, config.StepsConfig{})
		out, err := svc.SyncSteps(context.Background(), service.SyncStepsInput{
			UserID: 1001, SyncDate: fixedSyncDate, ClientTotalSteps: 270, MotionState: 2, ClientTimestamp: 1714560010000,
		})
		if err != nil {
			t.Fatalf("SyncSteps after reset > historical max: %v", err)
		}
		// r5：rawDelta = 270 - maxReported(250) = 20；SUM 兜底：250+20=270 不>270 → 不触发；delta=20
		if out.AcceptedDeltaSteps != 20 {
			t.Errorf("AcceptedDeltaSteps = %d, want 20 (r5 maxReported clamp = 270-250)", out.AcceptedDeltaSteps)
		}
		if out.StepAccount.TotalSteps != 270 {
			t.Errorf("TotalSteps = %d, want 270 (250 + 20)", out.StepAccount.TotalSteps)
		}
	})
}

// 15. OutOfOrderSync_SumCapPreventsRepeatedAccrual:
// **Story 7.3 review r3 [P1] 替代原 r2 乱序到达单测**：基线退回 id DESC 后，
// 乱序到达由 service 层 SUM 兜底捕获。
//
// 场景（与 integration test OutOfOrderSync_BaselineUsesMaxClientTotalSteps 镜像）：
//   - sync A: clientTotal=250 → delta=250；之后 SumAccepted=250
//   - sync B (旧报告延迟): clientTotal=200 → 倒退 rawDelta=0；SumAccepted 仍 250
//   - sync C: clientTotal=260
//     - 基线 = 最近 sync = 200（按 id DESC，B 是最近 INSERT）
//     - rawDelta = 260 - 200 = 60
//     - SUM 兜底：250 + 60 = 310 > 260 → adjusted = max(0, 260-250) = 10
//     - **入账 10**（与正确答案一致；不会重复入账 50 步）
//
// 反例（无 SUM 兜底）：rawDelta=60 直接入账 → 重复入账 50 步（攻击者套利）。
func TestStepService_SyncSteps_OutOfOrderSync_SumCapPreventsRepeatedAccrual(t *testing.T) {
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1001, TotalSteps: 250, AvailableSteps: 250, Version: 2}, nil
		},
	}
	logRepo := &stubStepSyncLogRepo{
		findLatestFn: func(ctx context.Context, userID uint64, syncDate string) (*mysql.StepSyncLog, error) {
			// latest = sync B = 200（最近 INSERT，id DESC）—— r5 之后只用于 latest != nil 判断
			return &mysql.StepSyncLog{ClientTotalSteps: 200}, nil
		},
		sumAcceptedFn: func(ctx context.Context, userID uint64, syncDate string) (int64, error) {
			// 历史 SUM = sync A 入账 250 + sync B 入账 0 = 250
			return 250, nil
		},
		// r5：maxReported = 250（sync A 报的）；clientTotal=260 > 250 → rawDelta = 260 - 250 = 10
		// SUM 兜底：250 + 10 = 260 不 > 260 → 不触发；delta = 10
		maxReportedFn: func(ctx context.Context, userID uint64, syncDate string) (uint64, error) {
			return 250, nil
		},
	}

	svc := buildStepService(accRepo, logRepo, config.StepsConfig{})
	out, err := svc.SyncSteps(context.Background(), service.SyncStepsInput{
		UserID: 1001, SyncDate: fixedSyncDate, ClientTotalSteps: 260, MotionState: 2, ClientTimestamp: 1714560100000,
	})
	if err != nil {
		t.Fatalf("SyncSteps OOO: %v", err)
	}
	// r5：maxReported clamp 直接削到 10（rawDelta=260-250=10）；若退化到 r3 的 latest 基线
	// 会得 60 → 然后 SUM 兜底削到 10；本 case 任一层防御失效都立即挂。
	if out.AcceptedDeltaSteps != 10 {
		t.Errorf("AcceptedDeltaSteps = %d, want 10 (r5 maxReported clamp + SUM 兜底叠加；若 = 60 → 防御失效 regression)", out.AcceptedDeltaSteps)
	}
	if out.StepAccount.TotalSteps != 260 {
		t.Errorf("TotalSteps = %d, want 260 (250 + 10)", out.StepAccount.TotalSteps)
	}
	if accRepo.lastUpdateDelta != 10 {
		t.Errorf("UpdateBalance delta = %d, want 10", accRepo.lastUpdateDelta)
	}
	if logRepo.lastCreated.AcceptedDeltaSteps != 10 {
		t.Errorf("sync_log.AcceptedDeltaSteps = %d, want 10", logRepo.lastCreated.AcceptedDeltaSteps)
	}
}

// 16. KnownLimitation_TruncationPlusOutOfOrder_MayOverAccrue_ByDesign:
// **Story 7.3 review r6 [P1] 选 reset 优先后引入的 known limitation 哨兵**。
//
// 决策史 r3 → r5 → r6（详见 docs/lessons/2026-05-04-*-r6-reset-vs-ooo-tradeoff.md）：
//   - r3：基线 id DESC + SUM 兜底 → reset 正确，但"截断 + 乱序"小幅多算
//   - r5：基线改 MAX(reported) clamp 修了"截断 + 乱序"，但 reset 永久卡死历史高水位
//   - r6：reset 与"截断+乱序"在概念上无法用单一规则同时满足；选 **reset 优先** 路径
//     （回到 r3 状态），接受"截断+乱序"小幅多算作为 known limitation
//
// **本 case 不是 regression sentinel —— 是 by-design 行为锁定**：
// 若未来谁再加 maxReported clamp（重蹈 r5 覆辙），accepted 会从 4000 变 2000，本 case
// 立即挂，提示 "r6 决策被无意改回 r5 路径"。
//
// 场景：
//   - sync A: clientTotal=10000 → rawDelta=10000 → 截断 5000 → accepted=5000;
//     DB 落 client_total=10000 / accepted=5000; prevAccepted=5000
//   - sync B: clientTotal=8000（旧报告延迟到达）→ 8000 < latest(10000) → rawDelta=0 → accepted=0;
//     DB 落 client_total=8000 / accepted=0; prevAccepted 仍 5000; latest 现在 = sync B（id DESC = 8000）
//   - sync C (本测试 mock 状态): clientTotal=12000;
//     - latest = sync B（client_total=8000）
//     - r6 (latest 当基线): rawDelta = 12000 - 8000 = 4000
//     - SUM 兜底: 5000 + 4000 = 9000 < 12000 → **不触发**
//     - 单次截断: 4000 < 5000 → 不触发
//     - delta = 4000 (**多算了 2000**: 实际客户端只新增 2000 步 10000→12000)
//
// **损失上限**：单次 ≤ single_sync_cap=5000；当日 SUM ≤ daily_cap=50000；
// 触发概率极低（要 5000+ 步突发被截断 + 恰好乱序到达）；接受作为 known limitation。
//
// **若未来要修这个 limitation**：必须同时保留 reset 修复 —— 唯一路径是引入"区分 reset 与
// 乱序"的额外信号（如 client 提交 sync_seq 或 device_id）；不要再尝试纯 server 端的
// MAX/SUM 单一规则微调。
func TestStepService_SyncSteps_KnownLimitation_TruncationPlusOutOfOrder_MayOverAccrue_ByDesign(t *testing.T) {
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			// 当前账户：sync A 入账 5000 + sync B 入账 0 = total 5000
			return &mysql.StepAccount{UserID: 1001, TotalSteps: 5000, AvailableSteps: 5000, Version: 2}, nil
		},
	}
	logRepo := &stubStepSyncLogRepo{
		findLatestFn: func(ctx context.Context, userID uint64, syncDate string) (*mysql.StepSyncLog, error) {
			// latest = sync B（最近 INSERT，id DESC，client_total=8000）—— r6 用此作为基线
			return &mysql.StepSyncLog{ClientTotalSteps: 8000}, nil
		},
		sumAcceptedFn: func(ctx context.Context, userID uint64, syncDate string) (int64, error) {
			// 历史 SUM = sync A 入账 5000 + sync B 入账 0 = 5000
			return 5000, nil
		},
		// 注：r6 service 不再调 MaxClientTotalStepsByUserAndDate（repo 方法保留供未来用），
		// 故本 case 不设 maxReportedFn（默认返 0）。
	}

	svc := buildStepService(accRepo, logRepo, config.StepsConfig{})
	out, err := svc.SyncSteps(context.Background(), service.SyncStepsInput{
		UserID: 1001, SyncDate: fixedSyncDate, ClientTotalSteps: 12000, MotionState: 2, ClientTimestamp: 1714560200000,
	})
	if err != nil {
		t.Fatalf("SyncSteps truncation+OOO known limitation: %v", err)
	}

	// **关键断言（by-design known limitation）**：accepted = 4000（不是 r5 maxReported clamp 下的 2000）
	// 若改回 maxReported clamp（r5）→ accepted=2000，此 case 立即挂，提示重蹈 r5 覆辙
	if out.AcceptedDeltaSteps != 4000 {
		t.Errorf("AcceptedDeltaSteps = %d, want 4000 (r6 by-design known limitation: 12000-latest(8000)=4000)；"+
			"若 = 2000 → 重新引入 r5 maxReported clamp（破坏 reset 修复），违反 r6 决策", out.AcceptedDeltaSteps)
	}
	if accRepo.lastUpdateDelta != 4000 {
		t.Errorf("UpdateBalance delta = %d, want 4000", accRepo.lastUpdateDelta)
	}
	if logRepo.lastCreated.AcceptedDeltaSteps != 4000 {
		t.Errorf("sync_log.AcceptedDeltaSteps = %d, want 4000", logRepo.lastCreated.AcceptedDeltaSteps)
	}
	// total = 5000 (现状) + 4000 = 9000（多算 2000，by-design）
	if out.StepAccount.TotalSteps != 9000 {
		t.Errorf("TotalSteps = %d, want 9000 (5000 + 4000，by-design known limitation)", out.StepAccount.TotalSteps)
	}
	if logRepo.lastCreated.ClientTotalSteps != 12000 {
		t.Errorf("sync_log.ClientTotalSteps = %d, want 12000", logRepo.lastCreated.ClientTotalSteps)
	}
}

// 17. NewStepService_ProdEnv_RejectsYAMLCapOverride:
// **Story 7.3 review r6 [P2]**：prod 环境必须用契约钦定的默认值（5000/50000）。
// envName="prod" + 任何正值 cap → panic（"prod env must use default caps"）。
//
// 反例（旧版无 prod gate）：运维误把 dev YAML（含 single_sync_cap: 100 fixture）推到 prod
// → server 启动正常，但当日封顶从 50000 降到自定义值 → 跨实例契约漂移、用户步数体验异常。
func TestStepService_NewStepService_ProdEnv_RejectsYAMLCapOverride(t *testing.T) {
	t.Run("ProdEnv_PositiveSingleSyncCap_Panics", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("NewStepService 应 panic（prod env + cfg.SingleSyncCap > 0）；实际未 panic")
			}
			msg, ok := r.(string)
			if !ok {
				t.Fatalf("panic 值不是 string: %T", r)
			}
			if !strings.Contains(msg, "prod env") || !strings.Contains(msg, "default caps") {
				t.Errorf("panic msg 不含 prod env 提示: %q", msg)
			}
		}()
		_ = service.NewStepService(
			&stubStepTxMgr{},
			&stubStepStepAccountRepo{},
			&stubStepSyncLogRepo{},
			config.StepsConfig{SingleSyncCap: 100},
			"prod",
		)
	})

	t.Run("ProdEnv_PositiveDailyCap_Panics", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("NewStepService 应 panic（prod env + cfg.DailyCap > 0）；实际未 panic")
			}
		}()
		_ = service.NewStepService(
			&stubStepTxMgr{},
			&stubStepStepAccountRepo{},
			&stubStepSyncLogRepo{},
			config.StepsConfig{DailyCap: 500},
			"prod",
		)
	})

	t.Run("ProdEnv_BothZero_OK", func(t *testing.T) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("NewStepService 不应 panic（prod env + 全部 zero-value，走默认）: %v", r)
			}
		}()
		_ = service.NewStepService(
			&stubStepTxMgr{},
			&stubStepStepAccountRepo{},
			&stubStepSyncLogRepo{},
			config.StepsConfig{},
			"prod",
		)
	})

	t.Run("EmptyEnv_DefaultsToProdStrict_Panics", func(t *testing.T) {
		// safe-by-default：envName 空 / 未注入 CAT_ENV → 按 prod 严格策略
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("NewStepService 应 panic（envName='' 按 prod 严格策略 + cfg.SingleSyncCap > 0）；实际未 panic")
			}
		}()
		_ = service.NewStepService(
			&stubStepTxMgr{},
			&stubStepStepAccountRepo{},
			&stubStepSyncLogRepo{},
			config.StepsConfig{SingleSyncCap: 100},
			"",
		)
	})

	t.Run("UnknownEnv_DefaultsToProdStrict_Panics", func(t *testing.T) {
		// 不识别的 envName（拼写错 / 历史值）→ 按 prod 严格策略（safe-by-default）
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("NewStepService 应 panic（unknown envName 按 prod 严格策略 + cfg.DailyCap > 0）；实际未 panic")
			}
		}()
		_ = service.NewStepService(
			&stubStepTxMgr{},
			&stubStepStepAccountRepo{},
			&stubStepSyncLogRepo{},
			config.StepsConfig{DailyCap: 500},
			"unknown-env-name",
		)
	})

	t.Run("ProductionAlias_StrictSameAsProd_Panics", func(t *testing.T) {
		defer func() {
			r := recover()
			if r == nil {
				t.Fatal("NewStepService 应 panic（envName='production' 与 prod 同语义 + cfg.SingleSyncCap > 0）；实际未 panic")
			}
		}()
		_ = service.NewStepService(
			&stubStepTxMgr{},
			&stubStepStepAccountRepo{},
			&stubStepSyncLogRepo{},
			config.StepsConfig{SingleSyncCap: 100},
			"production",
		)
	})

	for _, env := range []string{"dev", "DEV", "Dev", "staging", "test"} {
		env := env
		t.Run("Env_"+env+"_AllowsOverride", func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("NewStepService 不应 panic（envName=%q 允许 YAML cap 覆盖）: %v", env, r)
				}
			}()
			_ = service.NewStepService(
				&stubStepTxMgr{},
				&stubStepStepAccountRepo{},
				&stubStepSyncLogRepo{},
				config.StepsConfig{SingleSyncCap: 100, DailyCap: 500},
				env,
			)
		})
	}
}

// ============================================================
// Story 7.4: GetAccount 3 case（service 层单测，复用 stub repo + stub txMgr）
//
// 关键约束（详见 Story 7.4 AC4）：
//   - NotFound → 1003 (ErrResourceNotFound)，**不**是 1009（V1 §6.2.6 钦定）
//   - 复用 StepAccountBrief（home_service.go 已定义；不新建 GetAccountOutput）
//   - 不调 stepSyncLogRepo（GET 不读 sync_log）—— stub 三个方法主动 t.Errorf
//   - 不接事务（GET 纯读；stubStepTxMgr 仍需注入因 NewStepService 签名要求）
// ============================================================

// failOnSyncLogStub 返回一个 stubStepSyncLogRepo，其中三个查询方法被调用都会 t.Errorf（GET 不应触达 sync_log）。
//
// 与 7.3 stub 默认行为不同：7.3 的 stubStepSyncLogRepo 默认返回 ErrStepSyncLogNotFound 等"无害"值，
// 让 SyncSteps case 不显式 set fn 也能跑过；本 helper 强制让"被调"=fail，更醒目地哨兵 GET 路径
// 不应触达 sync_log（详见 step_service.go GetAccount godoc：「不调 stepSyncLogRepo」）。
func failOnSyncLogStub(t *testing.T) *stubStepSyncLogRepo {
	t.Helper()
	return &stubStepSyncLogRepo{
		findLatestFn: func(ctx context.Context, userID uint64, syncDate string) (*mysql.StepSyncLog, error) {
			t.Errorf("GetAccount should NOT call FindLatestByUserAndDate")
			return nil, mysql.ErrStepSyncLogNotFound
		},
		sumAcceptedFn: func(ctx context.Context, userID uint64, syncDate string) (int64, error) {
			t.Errorf("GetAccount should NOT call SumAcceptedDeltaByUserAndDate")
			return 0, nil
		},
		maxReportedFn: func(ctx context.Context, userID uint64, syncDate string) (uint64, error) {
			t.Errorf("GetAccount should NOT call MaxClientTotalStepsByUserAndDate")
			return 0, nil
		},
		createFn: func(ctx context.Context, log *mysql.StepSyncLog) error {
			t.Errorf("GetAccount should NOT call Create (sync_log)")
			return nil
		},
	}
}

// 1. HappyPath: stepAccountRepo.FindByUserID 返三档非零值 → service 返 *StepAccountBrief 完整透传
func TestStepService_GetAccount_HappyPath_ReturnsThreeBalances(t *testing.T) {
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			if userID != 1001 {
				t.Errorf("FindByUserID userID = %d, want 1001 (透传校验)", userID)
			}
			return &mysql.StepAccount{
				UserID: 1001, TotalSteps: 1140, AvailableSteps: 840, ConsumedSteps: 300, Version: 5,
			}, nil
		},
	}
	logRepo := failOnSyncLogStub(t)

	svc := buildStepService(accRepo, logRepo, config.StepsConfig{})
	out, err := svc.GetAccount(context.Background(), 1001)
	if err != nil {
		t.Fatalf("GetAccount: %v", err)
	}
	if out == nil {
		t.Fatal("out = nil, want non-nil")
	}
	if out.TotalSteps != 1140 {
		t.Errorf("TotalSteps = %d, want 1140", out.TotalSteps)
	}
	if out.AvailableSteps != 840 {
		t.Errorf("AvailableSteps = %d, want 840", out.AvailableSteps)
	}
	if out.ConsumedSteps != 300 {
		t.Errorf("ConsumedSteps = %d, want 300", out.ConsumedSteps)
	}
	// 哨兵：UpdateBalance 不应被调（GET 纯读）
	if accRepo.updateCalls != 0 {
		t.Errorf("UpdateBalance calls = %d, want 0 (GET 纯读)", accRepo.updateCalls)
	}
}

// 2. AccountNotFound: stepAccountRepo.FindByUserID 返 ErrStepAccountNotFound → service 翻译为 1003
//    （**非** 1009；V1 §6.2.6 钦定）
func TestStepService_GetAccount_AccountNotFound_Returns1003(t *testing.T) {
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return nil, mysql.ErrStepAccountNotFound
		},
	}
	logRepo := failOnSyncLogStub(t)

	svc := buildStepService(accRepo, logRepo, config.StepsConfig{})
	out, err := svc.GetAccount(context.Background(), 1001)
	if out != nil {
		t.Errorf("out = %+v, want nil on error path", out)
	}
	var appErr *apperror.AppError
	if !stderrors.As(err, &appErr) {
		t.Fatalf("err is not *AppError: %T (%v)", err, err)
	}
	if appErr.Code != apperror.ErrResourceNotFound {
		t.Errorf("appErr.Code = %d, want %d (1003 ErrResourceNotFound)；若 = 1009 → 错把 NotFound 包成 ServiceBusy（违反 V1 §6.2.6）", appErr.Code, apperror.ErrResourceNotFound)
	}
	// err 链必须含 ErrStepAccountNotFound 哨兵（apperror.Wrap 应保留 cause）
	if !stderrors.Is(err, mysql.ErrStepAccountNotFound) {
		t.Errorf("err 链不含 ErrStepAccountNotFound; err = %v", err)
	}
}

// 3. AllZero (新用户场景): stepAccountRepo.FindByUserID 返 0/0/0 → service 返 *StepAccountBrief{0,0,0}, err=nil
//    （验证 0 不被误特殊处理为 NotFound）
func TestStepService_GetAccount_AllZero_NewUser_ReturnsZeros(t *testing.T) {
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1001, TotalSteps: 0, AvailableSteps: 0, ConsumedSteps: 0, Version: 0}, nil
		},
	}
	logRepo := failOnSyncLogStub(t)

	svc := buildStepService(accRepo, logRepo, config.StepsConfig{})
	out, err := svc.GetAccount(context.Background(), 1001)
	if err != nil {
		t.Fatalf("GetAccount (all zero): %v", err)
	}
	if out == nil {
		t.Fatal("out = nil, want non-nil (zero values 不应被误判为 NotFound)")
	}
	if out.TotalSteps != 0 || out.AvailableSteps != 0 || out.ConsumedSteps != 0 {
		t.Errorf("out = %+v, want all zero", out)
	}
}

// 4. RepoOtherDBError_Returns1009: 非 NotFound 的 DB error → 包成 1009
func TestStepService_GetAccount_RepoOtherDBError_Returns1009(t *testing.T) {
	dbErr := stderrors.New("simulated DB connection lost")
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return nil, dbErr
		},
	}
	logRepo := failOnSyncLogStub(t)

	svc := buildStepService(accRepo, logRepo, config.StepsConfig{})
	out, err := svc.GetAccount(context.Background(), 1001)
	if out != nil {
		t.Errorf("out = %+v, want nil on DB error", out)
	}
	if got := apperror.Code(err); got != apperror.ErrServiceBusy {
		t.Errorf("apperror.Code = %d, want %d (1009 非 NotFound 走 ServiceBusy)", got, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, dbErr) {
		t.Errorf("err 链不含 dbErr; err = %v", err)
	}
}
