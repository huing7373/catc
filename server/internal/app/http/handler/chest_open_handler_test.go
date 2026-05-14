//go:build !integration

// Story 20.6 chest_handler.Open 单元测试：≥6 case 覆盖 POST /chest/open
// HTTP 边界 + envelope schema 断言 + handler 内层 rate_limit 行为。
//
// 模式与 chest_handler_test.go (20.5 GetCurrent) 同（stubChestService +
// newChestHandlerRouter + decodeChestEnvelope helper）。
//
// **关键断言点**：
//   - cached replay 路径跳过 rate_limit（V1 §7.2.5.4 r10 钦定 + 单测 case 2）
//   - idempotencyKey regex 校验 → 1002（V1 §7.2 钦定 + 单测 case 4）
//   - nextChest.status / remainingSeconds 动态补算同源同时刻（V1 §7.2 r11 + 单测 case 8）

package handler_test

import (
	"bytes"
	"context"
	stderrors "errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/handler"
	"github.com/huing/cat/server/internal/app/http/middleware"
	"github.com/huing/cat/server/internal/infra/config"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// stubHandlerIdempotencyRepo: mysql.IdempotencyRepo stub（handler 入口预检 + cached replay 判定）
type stubHandlerIdempotencyRepo struct {
	findFn         func(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error)
	findCalls      int
	lastFindUserID uint64
	lastFindKey    string
}

func (s *stubHandlerIdempotencyRepo) FindByUserIDAndKey(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error) {
	s.findCalls++
	s.lastFindUserID = userID
	s.lastFindKey = key
	return s.findFn(ctx, userID, key)
}
func (s *stubHandlerIdempotencyRepo) ClaimPending(ctx context.Context, userID uint64, key string) (int64, error) {
	panic("stubHandlerIdempotencyRepo.ClaimPending not configured (handler should not call it)")
}
func (s *stubHandlerIdempotencyRepo) MarkSuccess(ctx context.Context, userID uint64, key string, json []byte) error {
	panic("stubHandlerIdempotencyRepo.MarkSuccess not configured (handler should not call it)")
}

func intPtrLocal(v int64) *int64 { return &v }

// permissiveRateLimitCfg: 永远允许（high quota）；本测试 case 不验证超限路径
// （超限路径由 middleware/rate_limit_checker_test.go 单测覆盖）。
func permissiveRateLimitCfg() config.RateLimitConfig {
	return config.RateLimitConfig{
		PerKeyPerMin: intPtrLocal(6000),
		BurstSize:    intPtrLocal(6000),
		BucketsLimit: intPtrLocal(10000),
	}
}

// newChestOpenHandlerRouter: 测试 router 工厂（注入 mock auth + RequestID + ErrorMapping）。
func newChestOpenHandlerRouter(svc service.ChestService, idemRepo mysql.IdempotencyRepo, rateLimitCfg config.RateLimitConfig, mockUserID *uint64) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// RequestID 中间件：handler 需要从 c.Get(RequestIDKey) 读取
	r.Use(middleware.RequestID())
	r.Use(middleware.ErrorMappingMiddleware())
	if mockUserID != nil {
		uid := *mockUserID
		r.Use(func(c *gin.Context) {
			c.Set(middleware.UserIDKey, uid)
			c.Next()
		})
	}
	h := handler.NewChestHandler(svc, idemRepo, rateLimitCfg)
	r.POST("/api/v1/chest/open", h.Open)
	return r
}

// happyOpenChestOutput: 默认 service.OpenChestOutput（service 已返成功）
func happyOpenChestOutput() *service.OpenChestOutput {
	return &service.OpenChestOutput{
		Reward: service.ChestRewardBrief{
			UserCosmeticItemID: 0,
			CosmeticItemID:     24,
			Name:               "星星围巾",
			Slot:               4,
			Rarity:             2,
			AssetURL:           "https://x/a",
			IconURL:            "https://x/i",
		},
		StepAccount: service.StepAccountBrief{
			TotalSteps:     1500,
			AvailableSteps: 500,
			ConsumedSteps:  1000,
		},
		NextChest: service.ChestBrief{
			ID:            6001,
			UnlockAt:      time.Now().UTC().Add(10 * time.Minute),
			OpenCostSteps: 1000,
		},
	}
}

// 1. HappyPath_FirstTime: 合法 POST → 200 + envelope code=0 + 完整 data 字段
func TestChestHandler_Open_HappyPath_FirstTime(t *testing.T) {
	uid := uint64(1001)
	idem := &stubHandlerIdempotencyRepo{
		findFn: func(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error) {
			return nil, mysql.ErrIdempotencyRecordNotFound
		},
	}
	svc := &stubChestService{
		openChestFn: func(ctx context.Context, in service.OpenChestInput) (*service.OpenChestOutput, error) {
			if in.UserID != 1001 || in.IdempotencyKey != "test_key_001" {
				t.Errorf("svc 收到 in=%+v, want UserID=1001 IdempotencyKey=test_key_001", in)
			}
			return happyOpenChestOutput(), nil
		},
	}
	r := newChestOpenHandlerRouter(svc, idem, permissiveRateLimitCfg(), &uid)

	body := strings.NewReader(`{"idempotencyKey":"test_key_001"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chest/open", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	env := decodeChestEnvelope(t, w.Body.Bytes())
	if env.Code != 0 {
		t.Errorf("env.Code = %d, want 0", env.Code)
	}
	data, ok := env.Data.(map[string]any)
	if !ok {
		t.Fatalf("data not map: %T", env.Data)
	}
	reward := data["reward"].(map[string]any)
	if reward["userCosmeticItemId"] != "0" {
		t.Errorf("userCosmeticItemId = %v, want \"0\" (string 占位 + V1 §7.2.4h)", reward["userCosmeticItemId"])
	}
	if reward["cosmeticItemId"] != "24" {
		t.Errorf("cosmeticItemId = %v, want \"24\" (BIGINT 字符串化)", reward["cosmeticItemId"])
	}
	nextChest := data["nextChest"].(map[string]any)
	nextChestStatus, _ := nextChest["status"].(float64)
	if int(nextChestStatus) != 1 {
		t.Errorf("nextChest.status = %v, want 1 (UnlockAt 在 future)", nextChest["status"])
	}
	// remainingSeconds 应在 [598, 601] 范围（10min ≈ 600s ± 抖动）
	rs, _ := nextChest["remainingSeconds"].(float64)
	if rs < 598 || rs > 601 {
		t.Errorf("nextChest.remainingSeconds = %v, want ∈[598, 601]", nextChest["remainingSeconds"])
	}
}

// 2. HappyPath_CachedReplay_SkipsRateLimit: 命中 committed success → 跳过 rate_limit + 仍走 service
// **核心断言**：rate_limit 不被消耗（用极严配额：1/min；若 handler 走了 rate_limit，第 2 次请求必超限；
// 但因为 cached replay 跳过 rate_limit，5 次连续请求都应成功）。
func TestChestHandler_Open_HappyPath_CachedReplay_SkipsRateLimit(t *testing.T) {
	uid := uint64(1001)
	idem := &stubHandlerIdempotencyRepo{
		findFn: func(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error) {
			// 总返 cached success → handler 应跳过 rate_limit
			return &mysql.IdempotencyRecord{
				ID: 99, UserID: userID, IdempotencyKey: key,
				Status: mysql.IdempotencyStatusSuccess,
			}, nil
		},
	}
	svcCalls := 0
	svc := &stubChestService{
		openChestFn: func(ctx context.Context, in service.OpenChestInput) (*service.OpenChestOutput, error) {
			svcCalls++
			return happyOpenChestOutput(), nil
		},
	}
	// 极严配额：1/min；若 handler 漏调跳过逻辑，第 2 次必超限
	severeCfg := config.RateLimitConfig{
		PerKeyPerMin: intPtrLocal(1),
		BurstSize:    intPtrLocal(1),
		BucketsLimit: intPtrLocal(100),
	}
	r := newChestOpenHandlerRouter(svc, idem, severeCfg, &uid)

	for i := 0; i < 5; i++ {
		body := strings.NewReader(`{"idempotencyKey":"test_key_001"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/chest/open", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Fatalf("call #%d: status = %d, want 200; body=%s (cached replay should skip rate_limit)", i+1, w.Code, w.Body.String())
		}
		env := decodeChestEnvelope(t, w.Body.Bytes())
		if env.Code != 0 {
			t.Errorf("call #%d: env.Code = %d, want 0", i+1, env.Code)
		}
	}
	if svcCalls != 5 {
		t.Errorf("svc.openChestFn called %d times, want 5 (cached replay 仍调 service)", svcCalls)
	}
}

// 3. RateLimitExceeded_1005: 未命中 success + rate_limit 超限 → 1005
func TestChestHandler_Open_RateLimitExceeded_1005(t *testing.T) {
	// 重置 process 级 limiter，确保本 case 使用本地严苛 cfg（避免被前序 case 的 permissive cfg "锁定"）
	middleware.ResetChestOpenUserIDLimiterForTest()
	defer middleware.ResetChestOpenUserIDLimiterForTest()

	uid := uint64(1002) // 用独立 userID 隔离 process-level limiter 状态
	idem := &stubHandlerIdempotencyRepo{
		findFn: func(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error) {
			return nil, mysql.ErrIdempotencyRecordNotFound
		},
	}
	svcCalls := 0
	svc := &stubChestService{
		openChestFn: func(ctx context.Context, in service.OpenChestInput) (*service.OpenChestOutput, error) {
			svcCalls++
			return happyOpenChestOutput(), nil
		},
	}
	// burst=1 → 第 2 次必超限
	cfg := config.RateLimitConfig{
		PerKeyPerMin: intPtrLocal(60),
		BurstSize:    intPtrLocal(1),
		BucketsLimit: intPtrLocal(100),
	}
	r := newChestOpenHandlerRouter(svc, idem, cfg, &uid)

	// 第 1 次：通过
	body1 := strings.NewReader(`{"idempotencyKey":"k1"}`)
	req1 := httptest.NewRequest(http.MethodPost, "/api/v1/chest/open", body1)
	req1.Header.Set("Content-Type", "application/json")
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	if w1.Code != http.StatusOK {
		t.Fatalf("call #1: status=%d body=%s", w1.Code, w1.Body.String())
	}
	// 第 2 次：超限
	body2 := strings.NewReader(`{"idempotencyKey":"k2"}`)
	req2 := httptest.NewRequest(http.MethodPost, "/api/v1/chest/open", body2)
	req2.Header.Set("Content-Type", "application/json")
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	env := decodeChestEnvelope(t, w2.Body.Bytes())
	if env.Code != apperror.ErrTooManyRequests {
		t.Errorf("call #2: env.Code = %d, want %d (1005)", env.Code, apperror.ErrTooManyRequests)
	}
	if svcCalls != 1 {
		t.Errorf("svc.openChestFn called %d times, want 1 (rate-limited 2nd call should NOT reach service)", svcCalls)
	}
}

// 4. InvalidIdempotencyKey_1002: 空 / 非法字符 / 超长 → 1002 + service / idempotencyChecker 都不被调
func TestChestHandler_Open_InvalidIdempotencyKey_1002(t *testing.T) {
	uid := uint64(1001)
	cases := []struct {
		name string
		body string
	}{
		{"empty", `{"idempotencyKey":""}`},
		{"space_in_key", `{"idempotencyKey":"key with space"}`},
		{"too_long", `{"idempotencyKey":"` + strings.Repeat("a", 130) + `"}`},
		{"non_ascii_char", `{"idempotencyKey":"键中文"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idem := &stubHandlerIdempotencyRepo{
				findFn: func(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error) {
					t.Fatal("idempotencyChecker.FindByUserIDAndKey should NOT be called when key is invalid")
					return nil, nil
				},
			}
			svc := &stubChestService{
				openChestFn: func(ctx context.Context, in service.OpenChestInput) (*service.OpenChestOutput, error) {
					t.Fatal("svc.OpenChest should NOT be called when key is invalid")
					return nil, nil
				},
			}
			r := newChestOpenHandlerRouter(svc, idem, permissiveRateLimitCfg(), &uid)

			body := strings.NewReader(tc.body)
			req := httptest.NewRequest(http.MethodPost, "/api/v1/chest/open", body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			env := decodeChestEnvelope(t, w.Body.Bytes())
			if env.Code != apperror.ErrInvalidParam {
				t.Errorf("envelope.code = %d, want %d (1002 ErrInvalidParam)", env.Code, apperror.ErrInvalidParam)
			}
		})
	}
}

// 5. ServiceReturns4002_HTTP200: service 返 ErrChestNotUnlocked → envelope code=4002
func TestChestHandler_Open_ServiceReturns4002(t *testing.T) {
	uid := uint64(1001)
	idem := &stubHandlerIdempotencyRepo{
		findFn: func(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error) {
			return nil, mysql.ErrIdempotencyRecordNotFound
		},
	}
	svc := &stubChestService{
		openChestFn: func(ctx context.Context, in service.OpenChestInput) (*service.OpenChestOutput, error) {
			return nil, apperror.New(apperror.ErrChestNotUnlocked, apperror.DefaultMessages[apperror.ErrChestNotUnlocked])
		},
	}
	r := newChestOpenHandlerRouter(svc, idem, permissiveRateLimitCfg(), &uid)

	body := strings.NewReader(`{"idempotencyKey":"k_test_4002"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chest/open", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	env := decodeChestEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrChestNotUnlocked {
		t.Errorf("env.Code = %d, want %d (4002)", env.Code, apperror.ErrChestNotUnlocked)
	}
}

// 6. ServiceReturns3002_HTTP200: service 返 ErrInsufficientSteps → envelope code=3002
func TestChestHandler_Open_ServiceReturns3002(t *testing.T) {
	uid := uint64(1001)
	idem := &stubHandlerIdempotencyRepo{
		findFn: func(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error) {
			return nil, mysql.ErrIdempotencyRecordNotFound
		},
	}
	svc := &stubChestService{
		openChestFn: func(ctx context.Context, in service.OpenChestInput) (*service.OpenChestOutput, error) {
			return nil, apperror.New(apperror.ErrInsufficientSteps, apperror.DefaultMessages[apperror.ErrInsufficientSteps])
		},
	}
	r := newChestOpenHandlerRouter(svc, idem, permissiveRateLimitCfg(), &uid)

	body := strings.NewReader(`{"idempotencyKey":"k_test_3002"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chest/open", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	env := decodeChestEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInsufficientSteps {
		t.Errorf("env.Code = %d, want %d (3002)", env.Code, apperror.ErrInsufficientSteps)
	}
}

// 7. MissingUserIDInContext_Returns1009: auth 中间件未挂 → c.Get(UserIDKey) 不存在 → 1009
func TestChestHandler_Open_MissingUserIDInContext_Returns1009(t *testing.T) {
	idem := &stubHandlerIdempotencyRepo{
		findFn: func(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error) {
			t.Fatal("idempotencyChecker.FindByUserIDAndKey should NOT be called when userID missing")
			return nil, nil
		},
	}
	svc := &stubChestService{
		openChestFn: func(ctx context.Context, in service.OpenChestInput) (*service.OpenChestOutput, error) {
			t.Fatal("svc.OpenChest should NOT be called when userID missing")
			return nil, nil
		},
	}
	// mockUserID = nil → 不挂 mock auth middleware
	r := newChestOpenHandlerRouter(svc, idem, permissiveRateLimitCfg(), nil)

	body := strings.NewReader(`{"idempotencyKey":"k"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chest/open", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	env := decodeChestEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("env.Code = %d, want %d (1009)", env.Code, apperror.ErrServiceBusy)
	}
}

// 8. NextChestStatusAndRemainingSeconds_DynamicAtRequestTime:
// 验证 V1 §7.2 r11 锁定的 "同源同时刻" 补算 —— stub service 返 UnlockAt = T；
// handler 用 nowFn 注入两个不同时刻 → 期望 status / remainingSeconds 切换。
// 由于 ChestHandler.nowFn 是 unexported，通过两个独立 router (with stubbed UnlockAt 直接相对 time.Now())
// 验证：UnlockAt=now+10min → status=1 + remainingSeconds≈600；
// UnlockAt=now-5min → status=2 + remainingSeconds=0。
func TestChestHandler_Open_NextChestStatusAndRemainingSeconds_DynamicAtRequestTime(t *testing.T) {
	uid := uint64(1001)
	idem := &stubHandlerIdempotencyRepo{
		findFn: func(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error) {
			return nil, mysql.ErrIdempotencyRecordNotFound
		},
	}

	t.Run("future_unlockAt_status_1_remainingSeconds_~600", func(t *testing.T) {
		svc := &stubChestService{
			openChestFn: func(ctx context.Context, in service.OpenChestInput) (*service.OpenChestOutput, error) {
				out := happyOpenChestOutput()
				out.NextChest.UnlockAt = time.Now().UTC().Add(10 * time.Minute)
				return out, nil
			},
		}
		r := newChestOpenHandlerRouter(svc, idem, permissiveRateLimitCfg(), &uid)
		body := strings.NewReader(`{"idempotencyKey":"k_future"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/chest/open", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		env := decodeChestEnvelope(t, w.Body.Bytes())
		data := env.Data.(map[string]any)
		nc := data["nextChest"].(map[string]any)
		if int(nc["status"].(float64)) != 1 {
			t.Errorf("status = %v, want 1", nc["status"])
		}
		rs := nc["remainingSeconds"].(float64)
		if rs < 598 || rs > 601 {
			t.Errorf("remainingSeconds = %v, want ∈[598, 601]", rs)
		}
	})

	t.Run("past_unlockAt_status_2_remainingSeconds_0", func(t *testing.T) {
		svc := &stubChestService{
			openChestFn: func(ctx context.Context, in service.OpenChestInput) (*service.OpenChestOutput, error) {
				out := happyOpenChestOutput()
				out.NextChest.UnlockAt = time.Now().UTC().Add(-5 * time.Minute)
				return out, nil
			},
		}
		r := newChestOpenHandlerRouter(svc, idem, permissiveRateLimitCfg(), &uid)
		body := strings.NewReader(`{"idempotencyKey":"k_past"}`)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/chest/open", body)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		env := decodeChestEnvelope(t, w.Body.Bytes())
		data := env.Data.(map[string]any)
		nc := data["nextChest"].(map[string]any)
		if int(nc["status"].(float64)) != 2 {
			t.Errorf("status = %v, want 2 (UnlockAt in past)", nc["status"])
		}
		rs := nc["remainingSeconds"].(float64)
		if int(rs) != 0 {
			t.Errorf("remainingSeconds = %v, want 0 (UnlockAt in past)", rs)
		}
	})
}

// 9. InvalidJSONBody_Returns1002: 请求体不是合法 JSON → ShouldBindJSON 失败 → 1002
func TestChestHandler_Open_InvalidJSONBody_Returns1002(t *testing.T) {
	uid := uint64(1001)
	idem := &stubHandlerIdempotencyRepo{
		findFn: func(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error) {
			t.Fatal("idempotencyChecker.FindByUserIDAndKey should NOT be called when body is invalid JSON")
			return nil, nil
		},
	}
	svc := &stubChestService{}
	r := newChestOpenHandlerRouter(svc, idem, permissiveRateLimitCfg(), &uid)

	body := bytes.NewBufferString(`not-a-json`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chest/open", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	env := decodeChestEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrInvalidParam {
		t.Errorf("env.Code = %d, want %d (1002 ErrInvalidParam)", env.Code, apperror.ErrInvalidParam)
	}
}

// 10. IdempotencyDBError_Returns1009: idempotencyChecker 返非 NotFound 的 DB error → 1009
func TestChestHandler_Open_IdempotencyDBError_Returns1009(t *testing.T) {
	uid := uint64(1001)
	idem := &stubHandlerIdempotencyRepo{
		findFn: func(ctx context.Context, userID uint64, key string) (*mysql.IdempotencyRecord, error) {
			return nil, stderrors.New("synthetic db error")
		},
	}
	svcCalls := 0
	svc := &stubChestService{
		openChestFn: func(ctx context.Context, in service.OpenChestInput) (*service.OpenChestOutput, error) {
			svcCalls++
			return happyOpenChestOutput(), nil
		},
	}
	r := newChestOpenHandlerRouter(svc, idem, permissiveRateLimitCfg(), &uid)

	body := strings.NewReader(`{"idempotencyKey":"k_db_err"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/chest/open", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	env := decodeChestEnvelope(t, w.Body.Bytes())
	if env.Code != apperror.ErrServiceBusy {
		t.Errorf("env.Code = %d, want %d (1009)", env.Code, apperror.ErrServiceBusy)
	}
	if svcCalls != 0 {
		t.Errorf("svc.OpenChest called %d times, want 0 (DB error on idempotency check should abort before service)", svcCalls)
	}
}
