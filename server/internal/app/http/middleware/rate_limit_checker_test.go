package middleware

import (
	"context"
	"testing"

	"github.com/huing/cat/server/internal/infra/config"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
)

// Story 20.6 — CheckRateLimitByUserID 单测（≥3 case）
//
// 验证：
//   1. HappyPath: 单次调用 → nil（通过）
//   2. ExceedQuota: burst=2 + 第 3 次调用 → ErrTooManyRequests
//   3. UserID=0: 走 "key 为空 → 放行" 兜底分支 → nil
//
// 测试间用 resetChestOpenUserIDLimiterForTest 避免状态串扰。

func intPtr(v int64) *int64 {
	return &v
}

// TestCheckRateLimitByUserID_HappyPath_Allows: 首次调用合法 userID → 通过。
func TestCheckRateLimitByUserID_HappyPath_Allows(t *testing.T) {
	resetChestOpenUserIDLimiterForTest()
	defer resetChestOpenUserIDLimiterForTest()

	cfg := config.RateLimitConfig{
		PerKeyPerMin: intPtr(60),
		BurstSize:    intPtr(10),
		BucketsLimit: intPtr(100),
	}
	err := CheckRateLimitByUserID(context.Background(), cfg, 1001)
	if err != nil {
		t.Errorf("err = %v, want nil (happy path)", err)
	}
}

// TestCheckRateLimitByUserID_ExceedQuota_ReturnsTooManyRequests: burst=2 → 第 3 次超限。
func TestCheckRateLimitByUserID_ExceedQuota_ReturnsTooManyRequests(t *testing.T) {
	resetChestOpenUserIDLimiterForTest()
	defer resetChestOpenUserIDLimiterForTest()

	// burst=2 让 rate.Limiter 在 1s 时间窗口允许至多 2 次（perKeyPerMin=60 → 1 req/s
	// 但 burst=2 允许瞬时 2 个 token）；连续调用 3 次 → 第 3 次必超限。
	cfg := config.RateLimitConfig{
		PerKeyPerMin: intPtr(60),
		BurstSize:    intPtr(2),
		BucketsLimit: intPtr(100),
	}
	// 前 2 次应通过
	for i := 1; i <= 2; i++ {
		if err := CheckRateLimitByUserID(context.Background(), cfg, 1001); err != nil {
			t.Fatalf("call #%d: err = %v, want nil (within burst)", i, err)
		}
	}
	// 第 3 次应超限
	err := CheckRateLimitByUserID(context.Background(), cfg, 1001)
	if err == nil {
		t.Fatal("call #3: err = nil, want ErrTooManyRequests")
	}
	appErr, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err = %v not *AppError", err)
	}
	if appErr.Code != apperror.ErrTooManyRequests {
		t.Errorf("err.Code = %d, want %d (ErrTooManyRequests)", appErr.Code, apperror.ErrTooManyRequests)
	}
}

// TestCheckRateLimitByUserID_UserIDZero_AllowsBypass: userID=0 → 走 "key 为空 → 放行" 兜底分支。
func TestCheckRateLimitByUserID_UserIDZero_AllowsBypass(t *testing.T) {
	resetChestOpenUserIDLimiterForTest()
	defer resetChestOpenUserIDLimiterForTest()

	cfg := config.RateLimitConfig{
		PerKeyPerMin: intPtr(1), // 极严：1/min；如果不走兜底，第 2 次必超
		BurstSize:    intPtr(1),
		BucketsLimit: intPtr(100),
	}
	// 跑 5 次 userID=0；按"放行"兜底应全过
	for i := 1; i <= 5; i++ {
		if err := CheckRateLimitByUserID(context.Background(), cfg, 0); err != nil {
			t.Fatalf("call #%d: err = %v, want nil (userID=0 bypass)", i, err)
		}
	}
}
