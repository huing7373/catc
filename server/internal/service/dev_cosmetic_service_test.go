package service_test

import (
	"context"
	stderrors "errors"
	"testing"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// Story 20.8 dev_cosmetic_service 单元测试 —— **Story 23.5 节点 8 激活后改造版**。
//
// 节点 7 阶段（已退役）：stub 返 ErrNotImplemented (1010 → HTTP 501 + WARN log)，
// service 单测断言 1010。Story 23.5 激活真实写库后：service 实装变为
// FindRandomByRarity + 逐条 CreateInTx，单测从"断言 1010"改为"happy return nil +
// repo 被调"+ "FindRandomByRarity 空 → 1009" / "CreateInTx 失败 → 1009"。
//
// **disambiguation 验证**：dev 发放 source=3 admin_grant（§6.11 枚举；本文件
// 原接口注释行写 source=2 与 §6.11 冲突，以枚举为准取 source=3 —— 见
// dev_cosmetic_service.go interface 注释 disambiguation 说明）。
//
// **专用 stub**（本文件独立定义，不复用 chest_open_service_test.go 的 stub ——
// 那带 //go:build !integration tag，dev_cosmetic_service_test.go 无 tag 需在
// integration build 下也能编译）。

// stubDevCosmeticItemRepo: mysql.CosmeticItemRepo 的 stub（仅 FindRandomByRarity
// 走真实路径，其他方法 panic 暴露意外调用）。
type stubDevCosmeticItemRepo struct {
	findRandomFn    func(ctx context.Context, rarity int8, count int32) ([]uint64, error)
	findRandomCalls int
	lastRarity      int8
	lastCount       int32
}

func (s *stubDevCosmeticItemRepo) ListEnabledForWeightedPick(ctx context.Context) ([]mysql.CosmeticItem, error) {
	panic("stubDevCosmeticItemRepo.ListEnabledForWeightedPick not expected (dev grant 仅走 FindRandomByRarity)")
}

func (s *stubDevCosmeticItemRepo) ListEnabledForCatalog(ctx context.Context) ([]mysql.CosmeticItem, error) {
	panic("stubDevCosmeticItemRepo.ListEnabledForCatalog not expected (dev grant 仅走 FindRandomByRarity)")
}

func (s *stubDevCosmeticItemRepo) ListByIDsForInventory(ctx context.Context, ids []uint64) ([]mysql.CosmeticItem, error) {
	panic("stubDevCosmeticItemRepo.ListByIDsForInventory not expected (dev grant 仅走 FindRandomByRarity)")
}

func (s *stubDevCosmeticItemRepo) FindRandomByRarity(ctx context.Context, rarity int8, count int32) ([]uint64, error) {
	s.findRandomCalls++
	s.lastRarity = rarity
	s.lastCount = count
	return s.findRandomFn(ctx, rarity, count)
}

// stubDevUserCosmeticItemRepo: mysql.UserCosmeticItemRepo 的 stub（仅 CreateInTx
// 走真实路径，ListByUserForInventory panic 暴露意外调用）。
type stubDevUserCosmeticItemRepo struct {
	createInTxFn   func(ctx context.Context, item *mysql.UserCosmeticItem) error
	createInTxCall []*mysql.UserCosmeticItem
}

func (s *stubDevUserCosmeticItemRepo) ListByUserForInventory(ctx context.Context, userID uint64) ([]mysql.UserCosmeticItem, error) {
	panic("stubDevUserCosmeticItemRepo.ListByUserForInventory not expected (dev grant 仅走 CreateInTx)")
}

func (s *stubDevUserCosmeticItemRepo) CreateInTx(ctx context.Context, item *mysql.UserCosmeticItem) error {
	s.createInTxCall = append(s.createInTxCall, item)
	if s.createInTxFn != nil {
		return s.createInTxFn(ctx, item)
	}
	return nil
}

// 1. HappyPath_RealWrite_ReturnsNil_ReposCalled（节点 8 激活）：
// FindRandomByRarity 返 3 个 id → 逐条 CreateInTx → return nil；
// 验 repo 被调 + item 字段（Status=1 / Source=3 admin_grant / SourceRefID=nil）。
func TestDevCosmeticService_GrantCosmeticBatch_HappyPath_RealWrite_ReturnsNil_ReposCalled(t *testing.T) {
	cosmRepo := &stubDevCosmeticItemRepo{
		findRandomFn: func(ctx context.Context, rarity int8, count int32) ([]uint64, error) {
			return []uint64{11, 22, 33}, nil
		},
	}
	ucRepo := &stubDevUserCosmeticItemRepo{}
	svc := service.NewDevCosmeticService(cosmRepo, ucRepo)

	err := svc.GrantCosmeticBatch(context.Background(), 1001, 1, 3)
	if err != nil {
		t.Fatalf("GrantCosmeticBatch happy path: err = %v, want nil (节点 8 激活返 nil)", err)
	}

	if cosmRepo.findRandomCalls != 1 {
		t.Errorf("FindRandomByRarity called %d times, want 1", cosmRepo.findRandomCalls)
	}
	if cosmRepo.lastRarity != 1 || cosmRepo.lastCount != 3 {
		t.Errorf("FindRandomByRarity(rarity=%d, count=%d), want (1, 3)", cosmRepo.lastRarity, cosmRepo.lastCount)
	}
	if len(ucRepo.createInTxCall) != 3 {
		t.Fatalf("CreateInTx called %d times, want 3 (每个 cosmetic_item_id 一行)", len(ucRepo.createInTxCall))
	}
	for i, item := range ucRepo.createInTxCall {
		if item.UserID != 1001 {
			t.Errorf("item[%d].UserID = %d, want 1001", i, item.UserID)
		}
		if item.Status != 1 {
			t.Errorf("item[%d].Status = %d, want 1 (in_bag)", i, item.Status)
		}
		if item.Source != 3 {
			t.Errorf("item[%d].Source = %d, want 3 (admin_grant；§6.11 枚举，disambiguation 取 3 非 2)", i, item.Source)
		}
		if item.SourceRefID != nil {
			t.Errorf("item[%d].SourceRefID = %v, want nil (dev 发放无来源记录)", i, *item.SourceRefID)
		}
		if item.ObtainedAt.IsZero() {
			t.Errorf("item[%d].ObtainedAt is zero, want time.Now().UTC()", i)
		}
	}
	wantIDs := []uint64{11, 22, 33}
	for i, item := range ucRepo.createInTxCall {
		if item.CosmeticItemID != wantIDs[i] {
			t.Errorf("item[%d].CosmeticItemID = %d, want %d", i, item.CosmeticItemID, wantIDs[i])
		}
	}
}

// 2. FindRandomByRarityEmpty_Returns1009：FindRandomByRarity 返空 slice →
// service 判 len==0 → ErrServiceBusy (1009)（理论 seed ≥15 不该发生）。
func TestDevCosmeticService_GrantCosmeticBatch_FindRandomByRarityEmpty_Returns1009(t *testing.T) {
	cosmRepo := &stubDevCosmeticItemRepo{
		findRandomFn: func(ctx context.Context, rarity int8, count int32) ([]uint64, error) {
			return []uint64{}, nil
		},
	}
	ucRepo := &stubDevUserCosmeticItemRepo{
		createInTxFn: func(ctx context.Context, item *mysql.UserCosmeticItem) error {
			t.Fatal("CreateInTx should NOT be called when FindRandomByRarity returns empty")
			return nil
		},
	}
	svc := service.NewDevCosmeticService(cosmRepo, ucRepo)

	err := svc.GrantCosmeticBatch(context.Background(), 1001, 1, 10)
	assertServiceBusyError(t, err)
}

// 2b. RarityPoolShorterThanCount_Returns1009_NoPartialInsert（守门测试 ——
// fix-review 23-5 r1 [P1]）：FindRandomByRarity 合法返回 < count 个 id（如 seed
// common 仅 8 件、demo count=10 → ORDER BY RAND() LIMIT 10 只返 8）→ service
// 必须在**写库前**拒绝并返回 1009，**不**静默少发、**不**留下部分插入。
// 断言：① 返回 ErrServiceBusy(1009) ② CreateInTx 一次都没被调（杜绝 8 行部分
// 写入即原子性/事务正确性视角，参考 23-5 AC8 回滚 case 风格 —— 池不足时整批拒绝）。
func TestDevCosmeticService_GrantCosmeticBatch_RarityPoolShorterThanCount_Returns1009_NoPartialInsert(t *testing.T) {
	cosmRepo := &stubDevCosmeticItemRepo{
		findRandomFn: func(ctx context.Context, rarity int8, count int32) ([]uint64, error) {
			// 模拟 seed common 仅 8 件：请求 count=10，池只能返 8 个 id（合法少返）
			return []uint64{1, 2, 3, 4, 5, 6, 7, 8}, nil
		},
	}
	ucRepo := &stubDevUserCosmeticItemRepo{
		createInTxFn: func(ctx context.Context, item *mysql.UserCosmeticItem) error {
			t.Fatal("CreateInTx must NOT be called when rarity pool (8) < count (10) — 池不足应整批拒绝，杜绝部分插入")
			return nil
		},
	}
	svc := service.NewDevCosmeticService(cosmRepo, ucRepo)

	err := svc.GrantCosmeticBatch(context.Background(), 1001, 1, 10)
	assertServiceBusyError(t, err)

	if len(ucRepo.createInTxCall) != 0 {
		t.Fatalf("CreateInTx called %d times, want 0 (池不足 count 时禁止部分写入)", len(ucRepo.createInTxCall))
	}
}

// 3. FindRandomByRarityDBError_Returns1009：FindRandomByRarity 返 DB error → 1009。
func TestDevCosmeticService_GrantCosmeticBatch_FindRandomByRarityDBError_Returns1009(t *testing.T) {
	cosmRepo := &stubDevCosmeticItemRepo{
		findRandomFn: func(ctx context.Context, rarity int8, count int32) ([]uint64, error) {
			return nil, stderrors.New("synthetic find random db error")
		},
	}
	svc := service.NewDevCosmeticService(cosmRepo, &stubDevUserCosmeticItemRepo{})

	err := svc.GrantCosmeticBatch(context.Background(), 1001, 1, 10)
	assertServiceBusyError(t, err)
}

// 4. CreateInTxFails_Returns1009：FindRandomByRarity 返 id，但 CreateInTx 失败 → 1009。
func TestDevCosmeticService_GrantCosmeticBatch_CreateInTxFails_Returns1009(t *testing.T) {
	cosmRepo := &stubDevCosmeticItemRepo{
		findRandomFn: func(ctx context.Context, rarity int8, count int32) ([]uint64, error) {
			return []uint64{11, 22}, nil
		},
	}
	ucRepo := &stubDevUserCosmeticItemRepo{
		createInTxFn: func(ctx context.Context, item *mysql.UserCosmeticItem) error {
			return stderrors.New("synthetic create in tx failure")
		},
	}
	svc := service.NewDevCosmeticService(cosmRepo, ucRepo)

	err := svc.GrantCosmeticBatch(context.Background(), 1001, 2, 2)
	assertServiceBusyError(t, err)
}

// 5. BoundaryRarities_AllSucceed：rarity ∈ {1,2,3,4} × count ∈ {1,10,100}
// 共 12 组合都 happy return nil（节点 8 激活后无差别成功；与原节点 7 stub
// "无差别 1010" 对称改造）。
func TestDevCosmeticService_GrantCosmeticBatch_BoundaryRarities_AllSucceed(t *testing.T) {
	ctx := context.Background()
	rarities := []int8{1, 2, 3, 4}
	counts := []int32{1, 10, 100}

	for _, r := range rarities {
		for _, c := range counts {
			r, c := r, c
			t.Run("rarity="+string(rune('0'+r))+"_count="+itoa(c), func(t *testing.T) {
				cosmRepo := &stubDevCosmeticItemRepo{
					findRandomFn: func(ctx context.Context, rarity int8, count int32) ([]uint64, error) {
						ids := make([]uint64, count)
						for i := range ids {
							ids[i] = uint64(i + 1)
						}
						return ids, nil
					},
				}
				ucRepo := &stubDevUserCosmeticItemRepo{}
				svc := service.NewDevCosmeticService(cosmRepo, ucRepo)

				if err := svc.GrantCosmeticBatch(ctx, 1001, r, c); err != nil {
					t.Errorf("GrantCosmeticBatch(rarity=%d, count=%d): err = %v, want nil", r, c, err)
				}
				if len(ucRepo.createInTxCall) != int(c) {
					t.Errorf("CreateInTx called %d times, want %d", len(ucRepo.createInTxCall), c)
				}
			})
		}
	}
}

// assertServiceBusyError 断言 err 是 *AppError + Code == ErrServiceBusy (1009)。
func assertServiceBusyError(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil, want *AppError(ErrServiceBusy=1009)")
	}
	var ae *apperror.AppError
	if !stderrors.As(err, &ae) {
		t.Fatalf("err = %v, want *apperror.AppError", err)
	}
	if ae.Code != apperror.ErrServiceBusy {
		t.Errorf("err.Code = %d, want %d (1009 ErrServiceBusy)", ae.Code, apperror.ErrServiceBusy)
	}
}

// itoa 简易 int32 → string（避免引 strconv 仅为子测试名；与原文件保留同实现）。
func itoa(n int32) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	buf := make([]byte, 0, 8)
	for n > 0 {
		buf = append([]byte{byte('0' + n%10)}, buf...)
		n /= 10
	}
	if neg {
		buf = append([]byte{'-'}, buf...)
	}
	return string(buf)
}
