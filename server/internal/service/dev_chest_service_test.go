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

// Story 20.7 dev_chest_service 单元测试。
//
// **stubChestRepo 复用**：auth_service_test.go 已定义 stubChestRepo（同 package service_test），
// 含 Create / FindByUserID / FindByUserIDForUpdate / Delete / UpdateUnlockAtByID 五方法 panic-default；
// Story 20.7 在 auth_service_test.go 追加 UpdateUnlockAtByID 方法 + updateUnlockAtByIDFn 字段
// + findByUserIDForUpdateFn 字段 —— ChestRepo interface 加方法后，stub 必须同步追加，
// 否则 auth_service_test 全部 case 编译失败。
//
// **Story 20.7 review r1 [P2] 改造**：service 现在事务内两步调用：
//  1. FindByUserIDForUpdate(txCtx, userID) → 返当前 chest（含 id）
//  2. UpdateUnlockAtByID(txCtx, chest.ID, time.Now().UTC()) → UPDATE WHERE id
//
// 复用 7.3 既有的 stubStepTxMgr（step_service_test.go 顶部已定义；fn 直接 invoke 不真起事务）；
// 事务真边界由 dev_chest_service_integration_test.go 用 dockertest 起真实 MySQL 容器覆盖。
//
// 复用模式与 chest_service_test.go (20.5) 同：每 case 用独立 stubChestRepo instance（避免计数器串扰）。

// buildDevChestService 用 stub repo + stub txMgr 构造 DevChestService。
//
// 与 r1 [P2] 之前的区别：现在多注入一个 txMgr（之前是单 UPDATE 不开事务，service 签名只 1 个 repo 参数）。
func buildDevChestService(repo mysql.ChestRepo) service.DevChestService {
	return service.NewDevChestService(&stubStepTxMgr{}, repo)
}

// ============================================================
// 4 case（前缀 TestDevChestService_ForceUnlockChest_<场景>）
// ============================================================

// 1. HappyPath: stubChestRepo.findByUserIDForUpdateFn 返当前 chest (id=5001, user_id=1001)；
//    stubChestRepo.updateUnlockAtByIDFn 接收并断言：
//    (a) chestID=5001 透传正确（**关键**：r1 [P2] 改造后 service 应传 chest.ID 而非 user_id）
//    (b) 传入的 newUnlockAt **必须是 UTC**（time.Now().UTC() 同源）
//    (c) newUnlockAt 与 time.Now().UTC() 偏差 < 1s
//    → 返 nil → service.ForceUnlockChest(ctx, 1001) 返 err=nil；
//    断言：findForUpdateCalls=1, updateCalls=1
func TestDevChestService_ForceUnlockChest_HappyPath_FetchesChestIDThenUpdates(t *testing.T) {
	findForUpdateCalls := 0
	updateCalls := 0
	repo := &stubChestRepo{
		findByUserIDForUpdateFn: func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			findForUpdateCalls++
			if userID != 1001 {
				t.Errorf("FindByUserIDForUpdate userID = %d, want 1001 (透传校验)", userID)
			}
			return &mysql.UserChest{
				ID:            5001,
				UserID:        1001,
				Status:        1,
				UnlockAt:      time.Now().UTC().Add(10 * time.Minute),
				OpenCostSteps: 1000,
				Version:       0,
			}, nil
		},
		updateUnlockAtByIDFn: func(ctx context.Context, chestID uint64, newUnlockAt time.Time) error {
			updateCalls++
			// **r1 [P2] 核心断言**：service 应传 chest.ID=5001 而非 user_id=1001
			// （WHERE id=? 取代 WHERE user_id=? 防止跑偏到 next chest）
			if chestID != 5001 {
				t.Errorf("UpdateUnlockAtByID chestID = %d, want 5001 (r1 [P2] 改造后必须传 chest.ID 非 user_id)", chestID)
			}
			// 核心契约：UTC 时区钦定（V1 §2.5 + Story 4.6 / 20.5 / 20.6 同源）
			if newUnlockAt.Location() != time.UTC {
				t.Errorf("newUnlockAt.Location() = %q, want UTC (V1 §2.5 钦定)", newUnlockAt.Location())
			}
			// 偏差 < 1s（防 service 误传未来 / 历史时间）
			now := time.Now().UTC()
			delta := now.Sub(newUnlockAt)
			if delta < -time.Second || delta > time.Second {
				t.Errorf("newUnlockAt deviation from now = %v, want |delta| < 1s", delta)
			}
			return nil
		},
	}

	svc := buildDevChestService(repo)
	if err := svc.ForceUnlockChest(context.Background(), 1001); err != nil {
		t.Fatalf("ForceUnlockChest: %v", err)
	}

	if findForUpdateCalls != 1 {
		t.Errorf("FindByUserIDForUpdate calls = %d, want 1", findForUpdateCalls)
	}
	if updateCalls != 1 {
		t.Errorf("UpdateUnlockAtByID calls = %d, want 1", updateCalls)
	}
}

// 2. ChestNotFound: stubChestRepo.findByUserIDForUpdateFn 返 mysql.ErrChestNotFound →
//    service 返 *AppError(Code=1003)（**注意：1003 而非 4001**，与 epics.md §20.7 行 2947 钦定一致）；
//    UpdateUnlockAtByID 不被调用（事务 fn 内步 1 失败短路）
func TestDevChestService_ForceUnlockChest_ChestNotFound_Returns1003(t *testing.T) {
	findForUpdateCalls := 0
	updateCalls := 0
	repo := &stubChestRepo{
		findByUserIDForUpdateFn: func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			findForUpdateCalls++
			return nil, mysql.ErrChestNotFound
		},
		updateUnlockAtByIDFn: func(ctx context.Context, chestID uint64, newUnlockAt time.Time) error {
			updateCalls++
			return nil
		},
	}

	svc := buildDevChestService(repo)
	err := svc.ForceUnlockChest(context.Background(), 99999)
	if err == nil {
		t.Fatal("ForceUnlockChest should fail when chest not found")
	}
	// 强断 1003 而非 4001 ErrChestNotFound（防 service 误用业务码）
	if got := apperror.Code(err); got != apperror.ErrResourceNotFound {
		t.Errorf("apperror.Code = %d, want %d (1003 ErrResourceNotFound; epics.md §20.7 行 2947 钦定，**非** 4001)", got, apperror.ErrResourceNotFound)
	}
	// cause 链穿透：err 链必须含 mysql.ErrChestNotFound 哨兵
	if !stderrors.Is(err, mysql.ErrChestNotFound) {
		t.Errorf("err 链不含 ErrChestNotFound 哨兵; err = %v", err)
	}
	if findForUpdateCalls != 1 {
		t.Errorf("FindByUserIDForUpdate calls = %d, want 1", findForUpdateCalls)
	}
	if updateCalls != 0 {
		t.Errorf("UpdateUnlockAtByID calls = %d, want 0 (FindByUserIDForUpdate 失败短路)", updateCalls)
	}
}

// 3. FindForUpdateDBError: stubChestRepo.findByUserIDForUpdateFn 返非哨兵错误（模拟 DB 连接断 / SQL 错 / 死锁等）→
//    service 返 *AppError(Code=1009)；UpdateUnlockAtByID 不被调用
func TestDevChestService_ForceUnlockChest_FindForUpdateDBError_Returns1009(t *testing.T) {
	dbErr := stderrors.New("simulated db conn lost during FOR UPDATE select")
	findForUpdateCalls := 0
	updateCalls := 0
	repo := &stubChestRepo{
		findByUserIDForUpdateFn: func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			findForUpdateCalls++
			return nil, dbErr
		},
		updateUnlockAtByIDFn: func(ctx context.Context, chestID uint64, newUnlockAt time.Time) error {
			updateCalls++
			return nil
		},
	}

	svc := buildDevChestService(repo)
	err := svc.ForceUnlockChest(context.Background(), 1001)
	if err == nil {
		t.Fatal("ForceUnlockChest should fail on DB error")
	}
	if got := apperror.Code(err); got != apperror.ErrServiceBusy {
		t.Errorf("apperror.Code = %d, want %d (1009)", got, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, dbErr) {
		t.Errorf("err 链不含 dbErr; err = %v", err)
	}
	if findForUpdateCalls != 1 {
		t.Errorf("FindByUserIDForUpdate calls = %d, want 1", findForUpdateCalls)
	}
	if updateCalls != 0 {
		t.Errorf("UpdateUnlockAtByID calls = %d, want 0 (FindByUserIDForUpdate 失败短路)", updateCalls)
	}
}

// 4. UpdateDBError: stubChestRepo.findByUserIDForUpdateFn 返合法 chest；
//    stubChestRepo.updateUnlockAtByIDFn 返非哨兵错误 → service 返 *AppError(Code=1009)
func TestDevChestService_ForceUnlockChest_UpdateDBError_Returns1009(t *testing.T) {
	dbErr := stderrors.New("simulated db conn lost during UPDATE")
	findForUpdateCalls := 0
	updateCalls := 0
	repo := &stubChestRepo{
		findByUserIDForUpdateFn: func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
			findForUpdateCalls++
			return &mysql.UserChest{ID: 5001, UserID: userID}, nil
		},
		updateUnlockAtByIDFn: func(ctx context.Context, chestID uint64, newUnlockAt time.Time) error {
			updateCalls++
			return dbErr
		},
	}

	svc := buildDevChestService(repo)
	err := svc.ForceUnlockChest(context.Background(), 1001)
	if err == nil {
		t.Fatal("ForceUnlockChest should fail on UPDATE DB error")
	}
	if got := apperror.Code(err); got != apperror.ErrServiceBusy {
		t.Errorf("apperror.Code = %d, want %d (1009)", got, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, dbErr) {
		t.Errorf("err 链不含 dbErr; err = %v", err)
	}
	if findForUpdateCalls != 1 {
		t.Errorf("FindByUserIDForUpdate calls = %d, want 1", findForUpdateCalls)
	}
	if updateCalls != 1 {
		t.Errorf("UpdateUnlockAtByID calls = %d, want 1", updateCalls)
	}
}
