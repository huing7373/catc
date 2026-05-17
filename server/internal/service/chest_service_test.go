//go:build !integration

// Story 20.5 chest_service 单元测试：6 case 覆盖 GetCurrent 业务路径 + 错误翻译。
//
// 与 7.4 step_service.GetAccount 测试同模式（stub repo + 每 case 独立 stub instance）。
// **不**重复测 chestStatusDynamic / computeRemainingSeconds 行为（home_service_test 已覆盖）；
// 仅测 chest_service.GetCurrent 调 helper + 拼装 brief + 错误翻译的链路。
//
// **stubChestRepo 复用**：auth_service_test.go 已定义 stubChestRepo（同 package service_test），
// 字段 createFn / findByUserIDFn 完全够用 —— 本测试直接消费，不新建同义 stub。

package service_test

import (
	"context"
	stderrors "errors"
	"testing"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// newChestServiceForGetCurrent: Story 20.6 起 service.NewChestService 签名扩为 7 参数，
// Story 23.5 起扩为 8 参数（第 8 = userCosmeticItemRepo）；20.5 GetCurrent 测试只关心
// chestRepo，其他依赖传 nil（GetCurrent 不消费）。本 helper 集中签名扩展处，避免
// 每个 case 都重复构造 8 参数。
//
// 注意：传 nil 的 stepAccountRepo / idempotencyRepo / picker / userCosmeticItemRepo 等
// 不会被 GetCurrent 路径访问；OpenChest 路径在本文件 test 不被调
// （chest_open_service_test.go 已独立覆盖）。
func newChestServiceForGetCurrent(repo mysql.ChestRepo) service.ChestService {
	// Story 23.5 起 NewChestService 签名扩为 8 参数（第 8 = userCosmeticItemRepo）；
	// 20.5 GetCurrent 测试只关心 chestRepo，其他依赖传 nil（GetCurrent 不消费）。
	return service.NewChestService(repo, nil, nil, nil, nil, nil, nil, nil)
}

// 1. HappyPath_Counting: unlock_at 在未来 5 分钟 → status=1 (counting), remainingSeconds ≈ 300
func TestChestService_GetCurrent_HappyPath_Counting_Returns300s(t *testing.T) {
	unlockAt := time.Now().UTC().Add(5 * time.Minute)
	repo := &stubChestRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			if userID != 1001 {
				t.Errorf("FindByUserID userID = %d, want 1001 (透传校验)", userID)
			}
			return &mysql.UserChest{
				ID:            5001,
				UserID:        1001,
				Status:        1,
				UnlockAt:      unlockAt,
				OpenCostSteps: 1000,
				Version:       0,
			}, nil
		},
	}
	svc := newChestServiceForGetCurrent(repo)

	brief, err := svc.GetCurrent(context.Background(), 1001)
	if err != nil {
		t.Fatalf("GetCurrent: %v", err)
	}
	if brief == nil {
		t.Fatal("brief = nil, want non-nil")
	}
	if brief.ID != 5001 {
		t.Errorf("brief.ID = %d, want 5001", brief.ID)
	}
	if brief.Status != 1 {
		t.Errorf("brief.Status = %d, want 1 (counting)", brief.Status)
	}
	if brief.OpenCostSteps != 1000 {
		t.Errorf("brief.OpenCostSteps = %d, want 1000", brief.OpenCostSteps)
	}
	// 区间断言 (避免 ±1s 抖动)：stub setup 取一次 time.Now，service 内部又取一次
	if brief.RemainingSeconds < 299 || brief.RemainingSeconds > 300 {
		t.Errorf("brief.RemainingSeconds = %d, want ∈[299, 300] (5min ≈ 300s ± 1s jitter)", brief.RemainingSeconds)
	}
	if brief.UnlockAt.Location().String() != "UTC" {
		t.Errorf("brief.UnlockAt location = %q, want UTC (service 强制 .UTC())", brief.UnlockAt.Location().String())
	}
}

// 2. HappyPath_Unlockable_DBStatus1_UnlockAtPassed:
// DB 原值 status=1 + unlock_at 已过 5 分钟 → service 动态判定下发 status=2, remainingSeconds=0
// 核心断言：DB status=1 不代表 dispatch status=1（动态判定语义）
func TestChestService_GetCurrent_HappyPath_Unlockable_DBStatus1_UnlockAtPassed_Returns0s(t *testing.T) {
	repo := &stubChestRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			return &mysql.UserChest{
				ID:            5001,
				UserID:        1001,
				Status:        1, // DB 原值 counting
				UnlockAt:      time.Now().UTC().Add(-5 * time.Minute),
				OpenCostSteps: 1000,
			}, nil
		},
	}
	svc := newChestServiceForGetCurrent(repo)

	brief, err := svc.GetCurrent(context.Background(), 1001)
	if err != nil {
		t.Fatalf("GetCurrent: %v", err)
	}
	if brief.Status != 2 {
		t.Errorf("brief.Status = %d, want 2 (unlockable; 动态判定 DB=1 + unlock_at 已过 → 下发 2)", brief.Status)
	}
	if brief.RemainingSeconds != 0 {
		t.Errorf("brief.RemainingSeconds = %d, want 0 (unlock_at 已过)", brief.RemainingSeconds)
	}
}

// 3. HappyPath_DBStatus2: DB 原值已是 status=2 → service 透传 + remainingSeconds=0
func TestChestService_GetCurrent_HappyPath_DBStatus2_Returns2_0s(t *testing.T) {
	repo := &stubChestRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			return &mysql.UserChest{
				ID:            5001,
				UserID:        1001,
				Status:        2, // DB 原值已 unlockable
				UnlockAt:      time.Now().UTC().Add(-10 * time.Minute),
				OpenCostSteps: 1000,
			}, nil
		},
	}
	svc := newChestServiceForGetCurrent(repo)

	brief, err := svc.GetCurrent(context.Background(), 1001)
	if err != nil {
		t.Fatalf("GetCurrent: %v", err)
	}
	if brief.Status != 2 {
		t.Errorf("brief.Status = %d, want 2 (DB 原值即 2，透传)", brief.Status)
	}
	if brief.RemainingSeconds != 0 {
		t.Errorf("brief.RemainingSeconds = %d, want 0", brief.RemainingSeconds)
	}
}

// 4. ChestNotFound_Returns4001:
// stub 返 mysql.ErrChestNotFound → service 翻译为 *AppError(ErrChestNotFound, 4001)
// **不**包成 1009 / **不**包成 1003 —— V1 §7.1.6 钦定 4001（业务专用错误码）
func TestChestService_GetCurrent_ChestNotFound_Returns4001(t *testing.T) {
	repo := &stubChestRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			return nil, mysql.ErrChestNotFound
		},
	}
	svc := newChestServiceForGetCurrent(repo)

	brief, err := svc.GetCurrent(context.Background(), 1001)
	if brief != nil {
		t.Errorf("brief = %+v, want nil on error path", brief)
	}
	var appErr *apperror.AppError
	if !stderrors.As(err, &appErr) {
		t.Fatalf("err is not *AppError: %T (%v)", err, err)
	}
	if appErr.Code != apperror.ErrChestNotFound {
		t.Errorf("appErr.Code = %d, want %d (4001 ErrChestNotFound)；若 = 1009 → 错把 NotFound 包成 ServiceBusy；若 = 1003 → 错把 chest 业务专用错误用通用资源错误（违反 V1 §7.1.6）", appErr.Code, apperror.ErrChestNotFound)
	}
	// err 链必须含 ErrChestNotFound 哨兵
	if !stderrors.Is(err, mysql.ErrChestNotFound) {
		t.Errorf("err 链不含 ErrChestNotFound; err = %v", err)
	}
}

// 5. ClockBoundary_RemainingSecondsNotNegative:
// unlock_at 在过去 1ms → service 内 time.Now() > unlock_at → diff <= 0 → remainingSeconds = 0
// 验证 computeRemainingSeconds 的 max(0, ...) 兜底（never < 0）
func TestChestService_GetCurrent_ClockBoundary_RemainingSecondsNotNegative(t *testing.T) {
	repo := &stubChestRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			return &mysql.UserChest{
				ID:            5001,
				UserID:        1001,
				Status:        1,
				UnlockAt:      time.Now().UTC().Add(-1 * time.Millisecond),
				OpenCostSteps: 1000,
			}, nil
		},
	}
	svc := newChestServiceForGetCurrent(repo)

	brief, err := svc.GetCurrent(context.Background(), 1001)
	if err != nil {
		t.Fatalf("GetCurrent: %v", err)
	}
	if brief.Status != 2 {
		t.Errorf("brief.Status = %d, want 2 (边界已过, unlockable)", brief.Status)
	}
	if brief.RemainingSeconds != 0 {
		t.Errorf("brief.RemainingSeconds = %d, want 0 (never < 0; computeRemainingSeconds max(0, ...) 兜底)", brief.RemainingSeconds)
	}
	if brief.RemainingSeconds < 0 {
		t.Errorf("brief.RemainingSeconds = %d, NEGATIVE (违反 V1 §7.1 行 911 钦定 never < 0)", brief.RemainingSeconds)
	}
}

// 6. RepoOtherDBError_Returns1009:
// stub 返非 NotFound 的 DB error → service 翻译为 1009
// 验证未识别错误的兜底翻译（避免 raw error 透传给 handler 走 ErrorMappingMiddleware default 兜底）
func TestChestService_GetCurrent_RepoOtherDBError_Returns1009(t *testing.T) {
	dbErr := stderrors.New("simulated DB connection lost")
	repo := &stubChestRepo{
		findByUserIDFn: func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			return nil, dbErr
		},
	}
	svc := newChestServiceForGetCurrent(repo)

	brief, err := svc.GetCurrent(context.Background(), 1001)
	if brief != nil {
		t.Errorf("brief = %+v, want nil on DB error", brief)
	}
	if got := apperror.Code(err); got != apperror.ErrServiceBusy {
		t.Errorf("apperror.Code = %d, want %d (1009 非 NotFound 走 ServiceBusy)", got, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, dbErr) {
		t.Errorf("err 链不含 dbErr; err = %v", err)
	}
}
