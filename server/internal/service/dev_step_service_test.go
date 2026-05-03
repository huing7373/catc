package service_test

import (
	"context"
	stderrors "errors"
	"regexp"
	"strings"
	"testing"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// 复用 auth_service_test.go 中已定义的 stubUserRepo（同 package；只用 findByIDFn 字段）。
//
// 注意：auth_service_test.go 的 stub 在 fn 未设时直接 panic（fn 调 nil）。本文件 6 个 case
// 中 NegativeSteps_Panics 是唯一不会触发 FindByID 的（panic 在 service 入口；txMgr.WithTx 之前）；
// 其他 case 都显式 set findByIDFn。

// buildDevStepService 用 stub repo 构造 DevStepService。
//
// 复用 7.3 既有的 stubStepTxMgr（fn 直接 invoke，不真起事务）；事务真边界由集成测试覆盖。
func buildDevStepService(
	userRepo mysql.UserRepo,
	accRepo mysql.StepAccountRepo,
	logRepo mysql.StepSyncLogRepo,
) service.DevStepService {
	return service.NewDevStepService(&stubStepTxMgr{}, userRepo, accRepo, logRepo)
}

// dateRegex 校验 sync_date 字面值符合 YYYY-MM-DD 格式（HappyPath 哨兵）。
var dateRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// ============================================================
// 6 个 case
// ============================================================

// 1. HappyPath: stubUserRepo / stubStepStepAccountRepo / stubStepSyncLogRepo 全合法 →
//    GrantSteps(1001, 5000) 返 nil；
//    断言 UpdateBalance 调一次（delta=5000, version=3）；
//    断言 sync_log Create 调一次，字段全：UserID/AcceptedDeltaSteps/Source=2/MotionState=1/
//    ClientTotalSteps=0/ClientTs=0/SyncDate matches today UTC YYYY-MM-DD。
func TestDevStepService_GrantSteps_HappyPath_AppliesAccountAndAuditLog(t *testing.T) {
	userRepo := &stubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			if id != 1001 {
				t.Errorf("FindByID id = %d, want 1001 (透传校验)", id)
			}
			return &mysql.User{ID: 1001, GuestUID: "uid-1001", Nickname: "用户1001"}, nil
		},
	}
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{
				UserID: 1001, TotalSteps: 100, AvailableSteps: 100, ConsumedSteps: 0, Version: 3,
			}, nil
		},
	}
	logRepo := &stubStepSyncLogRepo{}

	svc := buildDevStepService(userRepo, accRepo, logRepo)
	if err := svc.GrantSteps(context.Background(), 1001, 5000); err != nil {
		t.Fatalf("GrantSteps: %v", err)
	}

	// === 账户更新断言 ===
	if accRepo.updateCalls != 1 {
		t.Errorf("UpdateBalance calls = %d, want 1", accRepo.updateCalls)
	}
	if accRepo.lastUpdateDelta != 5000 {
		t.Errorf("UpdateBalance delta = %d, want 5000", accRepo.lastUpdateDelta)
	}
	if accRepo.lastUpdateVer != 3 {
		t.Errorf("UpdateBalance version = %d, want 3", accRepo.lastUpdateVer)
	}

	// === sync_log 字段断言（dev grant 与 7.3 healthkit grant 的核心差异） ===
	if logRepo.createCalls != 1 {
		t.Errorf("Create calls = %d, want 1", logRepo.createCalls)
	}
	if logRepo.lastCreated == nil {
		t.Fatal("lastCreated = nil, want non-nil")
	}
	if logRepo.lastCreated.UserID != 1001 {
		t.Errorf("sync_log.UserID = %d, want 1001", logRepo.lastCreated.UserID)
	}
	if logRepo.lastCreated.AcceptedDeltaSteps != 5000 {
		t.Errorf("sync_log.AcceptedDeltaSteps = %d, want 5000", logRepo.lastCreated.AcceptedDeltaSteps)
	}
	if logRepo.lastCreated.Source != 2 {
		t.Errorf("sync_log.Source = %d, want 2 (admin_grant; §6.6 钦定；与 7.3 healthkit source=1 区分)", logRepo.lastCreated.Source)
	}
	if logRepo.lastCreated.MotionState != 1 {
		t.Errorf("sync_log.MotionState = %d, want 1 (stationary_or_unknown 中性值)", logRepo.lastCreated.MotionState)
	}
	if logRepo.lastCreated.ClientTotalSteps != 0 {
		t.Errorf("sync_log.ClientTotalSteps = %d, want 0 (dev 无客户端累计语义)", logRepo.lastCreated.ClientTotalSteps)
	}
	if logRepo.lastCreated.ClientTs != 0 {
		t.Errorf("sync_log.ClientTs = %d, want 0 (dev 无客户端时刻语义)", logRepo.lastCreated.ClientTs)
	}
	if !dateRegex.MatchString(logRepo.lastCreated.SyncDate) {
		t.Errorf("sync_log.SyncDate = %q, want YYYY-MM-DD format", logRepo.lastCreated.SyncDate)
	}
	// 严校：必须 == today UTC（容忍跨日 race，比 today 与 yesterday 都合法）
	todayUTC := time.Now().UTC().Format("2006-01-02")
	yesterdayUTC := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	got := logRepo.lastCreated.SyncDate
	if got != todayUTC && got != yesterdayUTC {
		t.Errorf("sync_log.SyncDate = %q, want today UTC %q (or yesterday %q for cross-day race)", got, todayUTC, yesterdayUTC)
	}
}

// 2. UserNotFound: stubUserRepo.findByIDFn 返 (nil, ErrUserNotFound) → service 返
//    *AppError(Code=1003)；UpdateBalance / Create 0 调用（事务 fn 内 step 1 失败短路）。
func TestDevStepService_GrantSteps_UserNotFound_Returns1003(t *testing.T) {
	userRepo := &stubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return nil, mysql.ErrUserNotFound
		},
	}
	accRepo := &stubStepStepAccountRepo{}
	logRepo := &stubStepSyncLogRepo{}

	svc := buildDevStepService(userRepo, accRepo, logRepo)
	err := svc.GrantSteps(context.Background(), 99999, 5000)
	if err == nil {
		t.Fatal("GrantSteps should fail when user not found")
	}
	if got := apperror.Code(err); got != apperror.ErrResourceNotFound {
		t.Errorf("apperror.Code = %d, want %d (1003)", got, apperror.ErrResourceNotFound)
	}
	if !stderrors.Is(err, mysql.ErrUserNotFound) {
		t.Errorf("err 链不含 ErrUserNotFound 哨兵; err = %v", err)
	}
	if accRepo.updateCalls != 0 {
		t.Errorf("UpdateBalance calls = %d, want 0 (事务 fn 内 step 1 失败短路)", accRepo.updateCalls)
	}
	if logRepo.createCalls != 0 {
		t.Errorf("Create calls = %d, want 0 (事务 fn 内 step 1 失败短路)", logRepo.createCalls)
	}
}

// 3. StepAccountNotFound: stubUserRepo 返合法 user / stubStepStepAccountRepo.findByUserIDFn
//    返 ErrStepAccountNotFound → service 返 *AppError(Code=1003)；UpdateBalance / Create 0 调用。
func TestDevStepService_GrantSteps_StepAccountNotFound_Returns1003(t *testing.T) {
	userRepo := &stubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: id}, nil
		},
	}
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return nil, mysql.ErrStepAccountNotFound
		},
	}
	logRepo := &stubStepSyncLogRepo{}

	svc := buildDevStepService(userRepo, accRepo, logRepo)
	err := svc.GrantSteps(context.Background(), 1001, 5000)
	if err == nil {
		t.Fatal("GrantSteps should fail when step_account not found")
	}
	if got := apperror.Code(err); got != apperror.ErrResourceNotFound {
		t.Errorf("apperror.Code = %d, want %d (1003)", got, apperror.ErrResourceNotFound)
	}
	if !stderrors.Is(err, mysql.ErrStepAccountNotFound) {
		t.Errorf("err 链不含 ErrStepAccountNotFound 哨兵; err = %v", err)
	}
	if accRepo.updateCalls != 0 {
		t.Errorf("UpdateBalance calls = %d, want 0 (FindByUserID 失败短路)", accRepo.updateCalls)
	}
	if logRepo.createCalls != 0 {
		t.Errorf("Create calls = %d, want 0 (FindByUserID 失败短路)", logRepo.createCalls)
	}
}

// 4. ZeroSteps: GrantSteps(1001, 0) → 仍走完整 4 步事务（version+1，sync_log 写入）。
//    审计纪律 —— dev 端点 0 步是合法 fixture（如重置 version / 写一行审计行表示"调用过端点"）。
func TestDevStepService_GrantSteps_ZeroSteps_StillWritesSyncLogAndIncrementsVersion(t *testing.T) {
	userRepo := &stubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: id}, nil
		},
	}
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{
				UserID: 1001, TotalSteps: 100, AvailableSteps: 100, ConsumedSteps: 0, Version: 5,
			}, nil
		},
	}
	logRepo := &stubStepSyncLogRepo{}

	svc := buildDevStepService(userRepo, accRepo, logRepo)
	if err := svc.GrantSteps(context.Background(), 1001, 0); err != nil {
		t.Fatalf("GrantSteps zero: %v", err)
	}

	if accRepo.updateCalls != 1 {
		t.Errorf("UpdateBalance calls = %d, want 1 (zero 也走 UpdateBalance 让 version+1)", accRepo.updateCalls)
	}
	if accRepo.lastUpdateDelta != 0 {
		t.Errorf("UpdateBalance delta = %d, want 0", accRepo.lastUpdateDelta)
	}
	if logRepo.createCalls != 1 {
		t.Errorf("Create calls = %d, want 1 (审计纪律：zero 也写一行)", logRepo.createCalls)
	}
	if logRepo.lastCreated.AcceptedDeltaSteps != 0 {
		t.Errorf("sync_log.AcceptedDeltaSteps = %d, want 0", logRepo.lastCreated.AcceptedDeltaSteps)
	}
	if logRepo.lastCreated.Source != 2 {
		t.Errorf("sync_log.Source = %d, want 2 (admin_grant)", logRepo.lastCreated.Source)
	}
}

// 5. UpdateBalanceDBError: stubStepStepAccountRepo.updateBalanceFn 返 simulated DB error
//    → service 返 *AppError(Code=1009)；Create 0 调用（step 3 失败，step 4 不执行）。
func TestDevStepService_GrantSteps_UpdateBalanceDBError_Returns1009(t *testing.T) {
	dbErr := stderrors.New("simulated UpdateBalance failure")
	userRepo := &stubUserRepo{
		findByIDFn: func(ctx context.Context, id uint64) (*mysql.User, error) {
			return &mysql.User{ID: id}, nil
		},
	}
	accRepo := &stubStepStepAccountRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
			return &mysql.StepAccount{UserID: 1001, Version: 1}, nil
		},
		updateBalanceFn: func(ctx context.Context, userID uint64, delta int32, expectedVersion uint64) error {
			return dbErr
		},
	}
	logRepo := &stubStepSyncLogRepo{}

	svc := buildDevStepService(userRepo, accRepo, logRepo)
	err := svc.GrantSteps(context.Background(), 1001, 5000)
	if err == nil {
		t.Fatal("GrantSteps should fail on UpdateBalance DB error")
	}
	if got := apperror.Code(err); got != apperror.ErrServiceBusy {
		t.Errorf("apperror.Code = %d, want %d (1009)", got, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, dbErr) {
		t.Errorf("err 链不含 dbErr; err = %v", err)
	}
	if logRepo.createCalls != 0 {
		t.Errorf("Create calls = %d, want 0 (UpdateBalance 失败后 step 4 不执行)", logRepo.createCalls)
	}
}

// 6. NegativeSteps_Panics: 防御性 panic 验证。
//    handler 层应已 1002 拦截；service 走到这里说明 handler 漏校验 → panic 让 fail-fast。
//    accRepo / logRepo 0 调用（panic 在 txMgr.WithTx 之前）。
func TestDevStepService_GrantSteps_NegativeSteps_Panics(t *testing.T) {
	userRepo := &stubUserRepo{}
	accRepo := &stubStepStepAccountRepo{}
	logRepo := &stubStepSyncLogRepo{}

	svc := buildDevStepService(userRepo, accRepo, logRepo)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("GrantSteps with negative steps should panic")
		}
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("panic value is not string: %T (%v)", r, r)
		}
		if !strings.Contains(msg, "steps must be >= 0") {
			t.Errorf("panic msg = %q, want contains 'steps must be >= 0'", msg)
		}
		if accRepo.updateCalls != 0 {
			t.Errorf("UpdateBalance calls = %d, want 0 (panic 在 txMgr 之前)", accRepo.updateCalls)
		}
		if logRepo.createCalls != 0 {
			t.Errorf("Create calls = %d, want 0 (panic 在 txMgr 之前)", logRepo.createCalls)
		}
	}()
	_ = svc.GrantSteps(context.Background(), 1001, -1)
}
