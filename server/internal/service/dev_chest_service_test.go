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
// r2 [P2] 改造后含 Create / FindByUserID / FindByID / FindByUserIDForUpdate / Delete /
// UpdateUnlockAtByID 六方法 panic-default + findByIDFn 字段；ChestRepo interface 加方法后，
// stub 必须同步追加，否则同 package 其他 service_test 编译失败。
//
// **Story 20.7 review r2 [P2] 改造**：service 现在 2 步直接调用（不开事务）：
//  1. FindByID(ctx, chestID) → 拿 chest，校验存在 + chest.UserID == claimedUserID
//  2. UpdateUnlockAtByID(ctx, chestID, time.Now().UTC()) → UPDATE WHERE id
//
// r1 → r2 的核心变化：
//   - 不再注入 txMgr（NewDevChestService 单参数）
//   - 不再调 FindByUserIDForUpdate（client 通过 chestID 入参把"哪个 chest"信息传进来）
//   - 增加"越权 unlock 他人 chest → 1003"case（service 防御性 user_id 比对）
//
// **Story 20.7 review r3 [P2] 改造**：r2 的"移除 RowsAffected 检查"是 over-correction，引入二阶 race
// （FindByID 后 chest 被并发 OpenChest 删除 → UPDATE 0 行但 service 返 false success）。r3 重新加回
// repo 层 RowsAffected==0 → ErrChestNotFound 翻译；service 在 UpdateUnlockAtByID 返 ErrChestNotFound
// 时同样翻译为 1003。case 6 ConcurrentDeleteRace_Returns1003 是 r3 修复的核心见证。
//
// 复用模式与 chest_service_test.go (20.5) 同：每 case 用独立 stubChestRepo instance（避免计数器串扰）。

// buildDevChestService 用 stub repo 构造 DevChestService。
//
// **r2 [P2] 改造**：单参数（去掉 txMgr）—— 与 r1 之前的"事务内 FOR UPDATE + UPDATE"区分。
func buildDevChestService(repo mysql.ChestRepo) service.DevChestService {
	return service.NewDevChestService(repo)
}

// ============================================================
// 5 case（前缀 TestDevChestService_ForceUnlockChest_<场景>）
// ============================================================

// 1. HappyPath: stubChestRepo.findByIDFn 返 chest (id=5001, user_id=1001)；
//    stubChestRepo.updateUnlockAtByIDFn 接收并断言：
//    (a) chestID=5001 透传正确
//    (b) 传入的 newUnlockAt **必须是 UTC**（time.Now().UTC() 同源）
//    (c) newUnlockAt 与 time.Now().UTC() 偏差 < 1s
//    → 返 nil → service.ForceUnlockChest(ctx, 1001, 5001) 返 err=nil；
//    断言：findByIDCalls=1, updateCalls=1
func TestDevChestService_ForceUnlockChest_HappyPath_FindByIDThenUpdates(t *testing.T) {
	findByIDCalls := 0
	updateCalls := 0
	repo := &stubChestRepo{
		findByIDFn: func(ctx context.Context, chestID uint64) (*mysql.UserChest, error) {
			findByIDCalls++
			if chestID != 5001 {
				t.Errorf("FindByID chestID = %d, want 5001 (透传校验)", chestID)
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
			// **r2 [P2] 核心断言**：service 透传 client 传入的 chestID（不再 FOR UPDATE 拿"current"）
			if chestID != 5001 {
				t.Errorf("UpdateUnlockAtByID chestID = %d, want 5001", chestID)
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
	if err := svc.ForceUnlockChest(context.Background(), 1001, 5001); err != nil {
		t.Fatalf("ForceUnlockChest: %v", err)
	}

	if findByIDCalls != 1 {
		t.Errorf("FindByID calls = %d, want 1", findByIDCalls)
	}
	if updateCalls != 1 {
		t.Errorf("UpdateUnlockAtByID calls = %d, want 1", updateCalls)
	}
}

// 2. ChestNotFound: stubChestRepo.findByIDFn 返 mysql.ErrChestNotFound →
//    service 返 *AppError(Code=1003)（**注意：1003 而非 4001**，与 epics.md §20.7 行 2947 钦定一致）；
//    UpdateUnlockAtByID 不被调用（步 1 失败短路）
func TestDevChestService_ForceUnlockChest_ChestNotFound_Returns1003(t *testing.T) {
	findByIDCalls := 0
	updateCalls := 0
	repo := &stubChestRepo{
		findByIDFn: func(ctx context.Context, chestID uint64) (*mysql.UserChest, error) {
			findByIDCalls++
			return nil, mysql.ErrChestNotFound
		},
		updateUnlockAtByIDFn: func(ctx context.Context, chestID uint64, newUnlockAt time.Time) error {
			updateCalls++
			return nil
		},
	}

	svc := buildDevChestService(repo)
	err := svc.ForceUnlockChest(context.Background(), 1001, 99999)
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
	if findByIDCalls != 1 {
		t.Errorf("FindByID calls = %d, want 1", findByIDCalls)
	}
	if updateCalls != 0 {
		t.Errorf("UpdateUnlockAtByID calls = %d, want 0 (FindByID 失败短路)", updateCalls)
	}
}

// 3. CrossUserUnlock_Returns1003: r2 [P2] 新增 case。
//    FindByID 返 chest，但 chest.UserID != claimedUserID → service 拒绝 + 返 1003。
//    防御性：dev 端点无 auth，恶意 client 可传任意 userID + 别人的 chestID；
//    service 用 chest.user_id 比对防御越权。
//    错误码 1003 而非 1006 ErrForbidden —— 与 ChestNotFound 同码 1003，避免暴露
//    "这个 chestID 存在但属于别人"信息（信息泄露防护）。
//    UpdateUnlockAtByID 不被调用。
func TestDevChestService_ForceUnlockChest_CrossUserUnlock_Returns1003(t *testing.T) {
	findByIDCalls := 0
	updateCalls := 0
	repo := &stubChestRepo{
		findByIDFn: func(ctx context.Context, chestID uint64) (*mysql.UserChest, error) {
			findByIDCalls++
			// chest 存在但属于别的 user
			return &mysql.UserChest{
				ID:            5001,
				UserID:        2002, // 注意：与请求里的 claimedUserID=1001 不同
				Status:        1,
				UnlockAt:      time.Now().UTC().Add(10 * time.Minute),
				OpenCostSteps: 1000,
			}, nil
		},
		updateUnlockAtByIDFn: func(ctx context.Context, chestID uint64, newUnlockAt time.Time) error {
			updateCalls++
			return nil
		},
	}

	svc := buildDevChestService(repo)
	// 1001 试图 unlock 5001（属于 2002）→ 应被拒
	err := svc.ForceUnlockChest(context.Background(), 1001, 5001)
	if err == nil {
		t.Fatal("ForceUnlockChest should fail when chest belongs to another user")
	}
	if got := apperror.Code(err); got != apperror.ErrResourceNotFound {
		t.Errorf("apperror.Code = %d, want %d (1003; 与 ChestNotFound 同码避免信息泄露)", got, apperror.ErrResourceNotFound)
	}
	if findByIDCalls != 1 {
		t.Errorf("FindByID calls = %d, want 1", findByIDCalls)
	}
	if updateCalls != 0 {
		t.Errorf("UpdateUnlockAtByID calls = %d, want 0 (越权校验失败短路)", updateCalls)
	}
}

// 4. FindByIDDBError: stubChestRepo.findByIDFn 返非哨兵错误（模拟 DB 连接断 / SQL 错 / 死锁等）→
//    service 返 *AppError(Code=1009)；UpdateUnlockAtByID 不被调用
func TestDevChestService_ForceUnlockChest_FindByIDDBError_Returns1009(t *testing.T) {
	dbErr := stderrors.New("simulated db conn lost during FindByID")
	findByIDCalls := 0
	updateCalls := 0
	repo := &stubChestRepo{
		findByIDFn: func(ctx context.Context, chestID uint64) (*mysql.UserChest, error) {
			findByIDCalls++
			return nil, dbErr
		},
		updateUnlockAtByIDFn: func(ctx context.Context, chestID uint64, newUnlockAt time.Time) error {
			updateCalls++
			return nil
		},
	}

	svc := buildDevChestService(repo)
	err := svc.ForceUnlockChest(context.Background(), 1001, 5001)
	if err == nil {
		t.Fatal("ForceUnlockChest should fail on DB error")
	}
	if got := apperror.Code(err); got != apperror.ErrServiceBusy {
		t.Errorf("apperror.Code = %d, want %d (1009)", got, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, dbErr) {
		t.Errorf("err 链不含 dbErr; err = %v", err)
	}
	if findByIDCalls != 1 {
		t.Errorf("FindByID calls = %d, want 1", findByIDCalls)
	}
	if updateCalls != 0 {
		t.Errorf("UpdateUnlockAtByID calls = %d, want 0 (FindByID 失败短路)", updateCalls)
	}
}

// 5. UpdateDBError: stubChestRepo.findByIDFn 返合法 chest；
//    stubChestRepo.updateUnlockAtByIDFn 返非哨兵错误 → service 返 *AppError(Code=1009)
func TestDevChestService_ForceUnlockChest_UpdateDBError_Returns1009(t *testing.T) {
	dbErr := stderrors.New("simulated db conn lost during UPDATE")
	findByIDCalls := 0
	updateCalls := 0
	repo := &stubChestRepo{
		findByIDFn: func(ctx context.Context, chestID uint64) (*mysql.UserChest, error) {
			findByIDCalls++
			return &mysql.UserChest{ID: 5001, UserID: 1001}, nil
		},
		updateUnlockAtByIDFn: func(ctx context.Context, chestID uint64, newUnlockAt time.Time) error {
			updateCalls++
			return dbErr
		},
	}

	svc := buildDevChestService(repo)
	err := svc.ForceUnlockChest(context.Background(), 1001, 5001)
	if err == nil {
		t.Fatal("ForceUnlockChest should fail on UPDATE DB error")
	}
	if got := apperror.Code(err); got != apperror.ErrServiceBusy {
		t.Errorf("apperror.Code = %d, want %d (1009)", got, apperror.ErrServiceBusy)
	}
	if !stderrors.Is(err, dbErr) {
		t.Errorf("err 链不含 dbErr; err = %v", err)
	}
	if findByIDCalls != 1 {
		t.Errorf("FindByID calls = %d, want 1", findByIDCalls)
	}
	if updateCalls != 1 {
		t.Errorf("UpdateUnlockAtByID calls = %d, want 1", updateCalls)
	}
}

// 6. ConcurrentDeleteRace_Returns1003: r3 [P2] 新增 case。
//
//    模拟二阶 race scenario：FindByID 成功拿到 chest + 校验 user 归属通过 →
//    紧接着另一并发 /chest/open 把 chest 删除 → UpdateUnlockAtByID 命中 0 行 →
//    repo 翻译为 mysql.ErrChestNotFound → service 透传翻译为 1003（与步骤 (1) FindByID NotFound 同码）。
//
//    **r2 → r3 关键回归 case**：r2 实装会返 nil（false success），r3 实装必须返 1003。
//    这是 r2 over-correction（"移除 RowsAffected 检查"）引入的二阶 bug 的修复见证。
func TestDevChestService_ForceUnlockChest_ConcurrentDeleteRace_Returns1003(t *testing.T) {
	findByIDCalls := 0
	updateCalls := 0
	repo := &stubChestRepo{
		findByIDFn: func(ctx context.Context, chestID uint64) (*mysql.UserChest, error) {
			findByIDCalls++
			// FindByID 成功（模拟"校验通过的瞬间 chest 还存在"）
			return &mysql.UserChest{ID: 5001, UserID: 1001}, nil
		},
		updateUnlockAtByIDFn: func(ctx context.Context, chestID uint64, newUnlockAt time.Time) error {
			updateCalls++
			// 模拟: 校验后 chest 被并发 OpenChest 删除 → UPDATE 0 行 → repo 翻译为 ErrChestNotFound
			return mysql.ErrChestNotFound
		},
	}

	svc := buildDevChestService(repo)
	err := svc.ForceUnlockChest(context.Background(), 1001, 5001)
	if err == nil {
		t.Fatal("ForceUnlockChest should fail when UPDATE hits ErrChestNotFound (concurrent delete race; r2 would falsely return nil)")
	}
	if got := apperror.Code(err); got != apperror.ErrResourceNotFound {
		t.Errorf("apperror.Code = %d, want %d (1003; r3 [P2] 修复 r2 false-success bug)", got, apperror.ErrResourceNotFound)
	}
	if !stderrors.Is(err, mysql.ErrChestNotFound) {
		t.Errorf("err 链不含 ErrChestNotFound 哨兵; err = %v", err)
	}
	if findByIDCalls != 1 {
		t.Errorf("FindByID calls = %d, want 1", findByIDCalls)
	}
	if updateCalls != 1 {
		t.Errorf("UpdateUnlockAtByID calls = %d, want 1", updateCalls)
	}
}
