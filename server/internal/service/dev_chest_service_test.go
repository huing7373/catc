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
// **stubChestRepo / stubTxMgr 复用**：auth_service_test.go 已定义。本文件 r4 [P2] 改造后用
// stubChestRepo.findByIDForUpdateFn + defaultStubTxMgr（mock 不真开事务，直接执行 fn —— 业务正确性
// 靠 fn 内 repo 调用顺序 + 计数断言）。
//
// **r2 → r3 → r4 改造路径**（详见 dev_chest_service.go 顶部注释）：
//   - r2 用 FindByID + UPDATE（无事务）；
//   - r3 加回 repo 层 RowsAffected==0 → ErrChestNotFound 翻译（修 r2 二阶 race false success）；
//   - r4 用事务 + FindByIDForUpdate + UPDATE（修 r3 引入的"同毫秒重复 unlock 同 chest 误报 1003"bug）。
//
// **r4 关键变化**：
//   - service.NewDevChestService 双参数（txMgr + chestRepo） —— 重新注入 txMgr（r2 移除过）
//   - service 内调 chestRepo.FindByIDForUpdate（非 FindByID）锁定 chest 行
//   - case "同毫秒重复 unlock 同 chest"从"返 1003"改为"返 nil success"
//     —— RowsAffected==0 在事务内 + FOR UPDATE 之后唯一可能是"值未变" = success
//
// 复用模式与 chest_service_test.go (20.5) 同：每 case 用独立 stubChestRepo instance（避免计数器串扰）。

// buildDevChestService 用 stub repo + stub txMgr 构造 DevChestService。
//
// **r4 [P2] 改造**：双参数（重新引入 txMgr）—— defaultStubTxMgr 直接执行 fn（mock 不真开事务；
// 真事务边界由集成测试 + Story 4.7 验证）。
func buildDevChestService(repo mysql.ChestRepo) service.DevChestService {
	return service.NewDevChestService(defaultStubTxMgr(), repo)
}

// ============================================================
// 6 case（前缀 TestDevChestService_ForceUnlockChest_<场景>）
// ============================================================

// 1. HappyPath: stubChestRepo.findByIDForUpdateFn 返 chest (id=5001, user_id=1001)；
//    stubChestRepo.updateUnlockAtByIDFn 接收并断言：
//    (a) chestID=5001 透传正确
//    (b) 传入的 newUnlockAt **必须是 UTC**（time.Now().UTC() 同源）
//    (c) newUnlockAt 与 time.Now().UTC() 偏差 < 1s
//    → 返 nil → service.ForceUnlockChest(ctx, 1001, 5001) 返 err=nil；
//    断言：findByIDForUpdateCalls=1, updateCalls=1
func TestDevChestService_ForceUnlockChest_HappyPath_FindByIDForUpdateThenUpdates(t *testing.T) {
	findCalls := 0
	updateCalls := 0
	repo := &stubChestRepo{
		findByIDForUpdateFn: func(ctx context.Context, chestID uint64) (*mysql.UserChest, error) {
			findCalls++
			if chestID != 5001 {
				t.Errorf("FindByIDForUpdate chestID = %d, want 5001 (透传校验)", chestID)
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
			// **r4 [P2] 核心断言**：service 透传 client 传入的 chestID
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

	if findCalls != 1 {
		t.Errorf("FindByIDForUpdate calls = %d, want 1", findCalls)
	}
	if updateCalls != 1 {
		t.Errorf("UpdateUnlockAtByID calls = %d, want 1", updateCalls)
	}
}

// 2. ChestNotFound: stubChestRepo.findByIDForUpdateFn 返 mysql.ErrChestNotFound →
//    service 返 *AppError(Code=1003)（**注意：1003 而非 4001**，与 epics.md §20.7 行 2947 钦定一致）；
//    UpdateUnlockAtByID 不被调用（步 1 失败短路）
func TestDevChestService_ForceUnlockChest_ChestNotFound_Returns1003(t *testing.T) {
	findCalls := 0
	updateCalls := 0
	repo := &stubChestRepo{
		findByIDForUpdateFn: func(ctx context.Context, chestID uint64) (*mysql.UserChest, error) {
			findCalls++
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
	if findCalls != 1 {
		t.Errorf("FindByIDForUpdate calls = %d, want 1", findCalls)
	}
	if updateCalls != 0 {
		t.Errorf("UpdateUnlockAtByID calls = %d, want 0 (FindByIDForUpdate 失败短路)", updateCalls)
	}
}

// 3. CrossUserUnlock_Returns1003: r2 [P2] 引入；r4 沿用。
//    FindByIDForUpdate 返 chest，但 chest.UserID != claimedUserID → service 拒绝 + 返 1003。
//    防御性：dev 端点无 auth，恶意 client 可传任意 userID + 别人的 chestID；
//    service 用 chest.user_id 比对防御越权。
//    错误码 1003 而非 1006 ErrForbidden —— 与 ChestNotFound 同码 1003，避免暴露
//    "这个 chestID 存在但属于别人"信息（信息泄露防护）。
//    UpdateUnlockAtByID 不被调用。
func TestDevChestService_ForceUnlockChest_CrossUserUnlock_Returns1003(t *testing.T) {
	findCalls := 0
	updateCalls := 0
	repo := &stubChestRepo{
		findByIDForUpdateFn: func(ctx context.Context, chestID uint64) (*mysql.UserChest, error) {
			findCalls++
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
	if findCalls != 1 {
		t.Errorf("FindByIDForUpdate calls = %d, want 1", findCalls)
	}
	if updateCalls != 0 {
		t.Errorf("UpdateUnlockAtByID calls = %d, want 0 (越权校验失败短路)", updateCalls)
	}
}

// 4. FindByIDForUpdateDBError: stubChestRepo.findByIDForUpdateFn 返非哨兵错误
//    （模拟 DB 连接断 / SQL 错 / 死锁等）→ service 返 *AppError(Code=1009)；UpdateUnlockAtByID 不被调用
func TestDevChestService_ForceUnlockChest_FindByIDForUpdateDBError_Returns1009(t *testing.T) {
	dbErr := stderrors.New("simulated db conn lost during FindByIDForUpdate")
	findCalls := 0
	updateCalls := 0
	repo := &stubChestRepo{
		findByIDForUpdateFn: func(ctx context.Context, chestID uint64) (*mysql.UserChest, error) {
			findCalls++
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
	if findCalls != 1 {
		t.Errorf("FindByIDForUpdate calls = %d, want 1", findCalls)
	}
	if updateCalls != 0 {
		t.Errorf("UpdateUnlockAtByID calls = %d, want 0 (FindByIDForUpdate 失败短路)", updateCalls)
	}
}

// 5. UpdateDBError: stubChestRepo.findByIDForUpdateFn 返合法 chest；
//    stubChestRepo.updateUnlockAtByIDFn 返非哨兵错误 → service 返 *AppError(Code=1009)
func TestDevChestService_ForceUnlockChest_UpdateDBError_Returns1009(t *testing.T) {
	dbErr := stderrors.New("simulated db conn lost during UPDATE")
	findCalls := 0
	updateCalls := 0
	repo := &stubChestRepo{
		findByIDForUpdateFn: func(ctx context.Context, chestID uint64) (*mysql.UserChest, error) {
			findCalls++
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
	if findCalls != 1 {
		t.Errorf("FindByIDForUpdate calls = %d, want 1", findCalls)
	}
	if updateCalls != 1 {
		t.Errorf("UpdateUnlockAtByID calls = %d, want 1", updateCalls)
	}
}

// 6. SameMillisecondRepeatUnlock_ReturnsSuccess: r4 [P2] 引入的关键 case。
//
//    模拟"同毫秒重复 unlock 同 chest"scenario：
//    - FindByIDForUpdate 成功拿到 chest + 校验通过；
//    - UpdateUnlockAtByID 在事务内 + FOR UPDATE 后执行 —— 行存在性已锁定保证；
//      但 unlock_at 列已是相同值（如上一次同毫秒 force-unlock 刚写过）→ MySQL 返
//      RowsAffected=0，repo r4 实装**不再翻译为 ErrChestNotFound**，直接返 nil。
//    - service 透传 nil → 整体返 nil（success）。
//
//    **r3 → r4 关键回归 case**：r3 实装在此场景返 1003（误报）；r4 必须返 nil。
//    这是 r3 over-correction（用 RowsAffected==0 判 NotFound）引入的"同毫秒重复 unlock 误报"bug
//    的修复见证，也是本次 r4 改造的核心动机。
func TestDevChestService_ForceUnlockChest_SameMillisecondRepeatUnlock_ReturnsSuccess(t *testing.T) {
	findCalls := 0
	updateCalls := 0
	repo := &stubChestRepo{
		findByIDForUpdateFn: func(ctx context.Context, chestID uint64) (*mysql.UserChest, error) {
			findCalls++
			// FindByIDForUpdate 成功（FOR UPDATE 锁定行）
			return &mysql.UserChest{ID: 5001, UserID: 1001}, nil
		},
		updateUnlockAtByIDFn: func(ctx context.Context, chestID uint64, newUnlockAt time.Time) error {
			updateCalls++
			// 模拟"同毫秒重复 unlock 同 chest" → unlock_at 列值未变 → RowsAffected=0；
			// r4 [P2] repo 实装不再翻译为 ErrChestNotFound（事务内 + FOR UPDATE 之后行必存在）→ 返 nil
			return nil
		},
	}

	svc := buildDevChestService(repo)
	err := svc.ForceUnlockChest(context.Background(), 1001, 5001)
	if err != nil {
		t.Fatalf("ForceUnlockChest should succeed on same-millisecond repeat unlock (r3 false-positive 1003 fixed by r4): %v", err)
	}
	if findCalls != 1 {
		t.Errorf("FindByIDForUpdate calls = %d, want 1", findCalls)
	}
	if updateCalls != 1 {
		t.Errorf("UpdateUnlockAtByID calls = %d, want 1", updateCalls)
	}
}
