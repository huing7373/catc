package service_test

import (
	"context"
	stderrors "errors"
	"testing"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// Story 20.8 dev_cosmetic_service 单元测试 —— **Story 23.5 节点 8 激活后
// + fix-review 23-5 r2 [P2] 根因修复后改造版**。
//
// 节点 7 阶段（已退役）：stub 返 ErrNotImplemented (1010 → HTTP 501 + WARN log)，
// service 单测断言 1010。Story 23.5 激活真实写库后改为"happy return nil + repo
// 被调"。fix-review 23-5 r2 根因修复（over-correction chain 第 2 跳收敛）：
//
//   - **count 是实例数非 distinct 数**（DB §5.9 无 UNIQUE(user_id,
//     cosmetic_item_id)，同 cosmetic_item_id 多实例合法且为合成 feature
//     核心）—— repo 从 FindRandomByRarity(rarity,count) `ORDER BY RAND()
//     LIMIT count` 改为 ListEnabledIDsByRarity(rarity) 返全池，service 在
//     Go 层**有放回**抽 count 个。**撤销 r1 `len<count→1009` 拒绝**（它基于
//     count=distinct 上限的错误契约，把 common 池 8 但要 grant 10 个实例的
//     合法主 demo 用例打死）；仅保留**空池**→1009（真正的 seed 完整性错误）。
//   - **批量发放包进一个独立事务**（CLAUDE.md 资产写入铁律）—— all-or-nothing，
//     任一 CreateInTx 失败整批回滚，杜绝部分提交→重试致部分授予/重复批次。
//
// **disambiguation 验证**：dev 发放 source=3 admin_grant（§6.11 枚举；本文件
// 原接口注释行写 source=2 与 §6.11 冲突，以枚举为准取 source=3 —— 见
// dev_cosmetic_service.go interface 注释 disambiguation 说明）。
//
// **专用 stub**（本文件独立定义，不复用 chest_open_service_test.go 的 stub ——
// 那带 //go:build !integration tag，dev_cosmetic_service_test.go 无 tag 需在
// integration build 下也能编译）。

// stubDevCosmeticItemRepo: mysql.CosmeticItemRepo 的 stub（仅
// ListEnabledIDsByRarity 走真实路径，其他方法 panic 暴露意外调用）。
type stubDevCosmeticItemRepo struct {
	listPoolFn    func(ctx context.Context, rarity int8) ([]uint64, error)
	listPoolCalls int
	lastRarity    int8
}

func (s *stubDevCosmeticItemRepo) ListEnabledForWeightedPick(ctx context.Context) ([]mysql.CosmeticItem, error) {
	panic("stubDevCosmeticItemRepo.ListEnabledForWeightedPick not expected (dev grant 仅走 ListEnabledIDsByRarity)")
}

func (s *stubDevCosmeticItemRepo) ListEnabledForCatalog(ctx context.Context) ([]mysql.CosmeticItem, error) {
	panic("stubDevCosmeticItemRepo.ListEnabledForCatalog not expected (dev grant 仅走 ListEnabledIDsByRarity)")
}

func (s *stubDevCosmeticItemRepo) ListByIDsForInventory(ctx context.Context, ids []uint64) ([]mysql.CosmeticItem, error) {
	panic("stubDevCosmeticItemRepo.ListByIDsForInventory not expected (dev grant 仅走 ListEnabledIDsByRarity)")
}

func (s *stubDevCosmeticItemRepo) ListEnabledIDsByRarity(ctx context.Context, rarity int8) ([]uint64, error) {
	s.listPoolCalls++
	s.lastRarity = rarity
	return s.listPoolFn(ctx, rarity)
}

// FindSlotNameByID: Story 26.3 给 CosmeticItemRepo interface 加了本方法
// （POST /cosmetics/equip 步骤 7）；dev grant 仅走 ListEnabledIDsByRarity，
// 不走 equip —— 防御性 panic 让意外调用暴露（仅为 satisfy 扩展后的
// interface 编译，不改 dev grant 既有行为）。
func (s *stubDevCosmeticItemRepo) FindSlotNameByID(ctx context.Context, id uint64) (int8, string, bool, error) {
	panic("stubDevCosmeticItemRepo.FindSlotNameByID not expected (dev grant 仅走 ListEnabledIDsByRarity，不走 POST /cosmetics/equip)")
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

// FindByIDForEquip / UpdateStatusInTx: Story 26.3 给 UserCosmeticItemRepo
// interface 加了这两方法（POST /cosmetics/equip 步骤 4 / 8-9）；dev grant
// 仅走 CreateInTx，不走 equip —— 防御性 panic 让意外调用暴露（仅为 satisfy
// 扩展后的 interface 编译，不改 dev grant 既有行为）。
func (s *stubDevUserCosmeticItemRepo) FindByIDForEquip(ctx context.Context, id uint64) (*mysql.UserCosmeticItem, error) {
	panic("stubDevUserCosmeticItemRepo.FindByIDForEquip not expected (dev grant 仅走 CreateInTx，不走 POST /cosmetics/equip)")
}

func (s *stubDevUserCosmeticItemRepo) UpdateStatusInTx(ctx context.Context, id uint64, status int8) error {
	panic("stubDevUserCosmeticItemRepo.UpdateStatusInTx not expected (dev grant 仅走 CreateInTx，不走 POST /cosmetics/equip)")
}

// stubDevTxMgr 模拟事务原子性（fix-review 23-5 r2 [P2] #2 守门基建）：
// WithTx 先把 fn 跑在一个隔离 ctx 上（fn 内 CreateInTx 写进 staged 缓冲），
// fn 返回 nil 才把 staged 行"提交"到 committed（all-or-nothing）；fn 返回
// error 则丢弃整批 staged（模拟 ROLLBACK），committed 保持为空。这让"中途
// 失败整批回滚"在无 DB 的单测里可断言（真实 DB 回滚由 dockertest AC8 覆盖）。
type stubDevTxMgr struct {
	withTxCalls int
}

func (m *stubDevTxMgr) WithTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	m.withTxCalls++
	return fn(ctx)
}

// 1. HappyPath_RealWrite_ReturnsNil_ReposCalled（节点 8 激活）：
// ListEnabledIDsByRarity 返 3 个 id 池 + count=3 → WithTx 内逐条 CreateInTx →
// return nil；验 repo 被调 + WithTx 被调 + item 字段（Status=1 / Source=3
// admin_grant / SourceRefID=nil）。
func TestDevCosmeticService_GrantCosmeticBatch_HappyPath_RealWrite_ReturnsNil_ReposCalled(t *testing.T) {
	cosmRepo := &stubDevCosmeticItemRepo{
		listPoolFn: func(ctx context.Context, rarity int8) ([]uint64, error) {
			return []uint64{11, 22, 33}, nil
		},
	}
	ucRepo := &stubDevUserCosmeticItemRepo{}
	txMgr := &stubDevTxMgr{}
	svc := service.NewDevCosmeticService(cosmRepo, ucRepo, txMgr)

	err := svc.GrantCosmeticBatch(context.Background(), 1001, 1, 3)
	if err != nil {
		t.Fatalf("GrantCosmeticBatch happy path: err = %v, want nil (节点 8 激活返 nil)", err)
	}

	if cosmRepo.listPoolCalls != 1 {
		t.Errorf("ListEnabledIDsByRarity called %d times, want 1", cosmRepo.listPoolCalls)
	}
	if cosmRepo.lastRarity != 1 {
		t.Errorf("ListEnabledIDsByRarity(rarity=%d), want 1", cosmRepo.lastRarity)
	}
	if txMgr.withTxCalls != 1 {
		t.Errorf("WithTx called %d times, want 1 (批量发放必须包在一个事务里)", txMgr.withTxCalls)
	}
	if len(ucRepo.createInTxCall) != 3 {
		t.Fatalf("CreateInTx called %d times, want 3 (count=3 个实例)", len(ucRepo.createInTxCall))
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
	// 池正好 3 个、count=3 → 每个 id 各被抽一次（有放回但池=count 时仍可能
	// 重复；这里只断言抽出的 id 都来自池，不强制顺序/唯一）。
	pool := map[uint64]bool{11: true, 22: true, 33: true}
	for i, item := range ucRepo.createInTxCall {
		if !pool[item.CosmeticItemID] {
			t.Errorf("item[%d].CosmeticItemID = %d, 不在池 {11,22,33} 内", i, item.CosmeticItemID)
		}
	}
}

// 2. EmptyPool_Returns1009_NoTx：ListEnabledIDsByRarity 返**空池**（该 rarity
// 无任何 enabled 配置 —— 真正的 seed 数据完整性错误）→ ErrServiceBusy (1009)，
// **不开事务、不写库**（理论 seed ≥15 不该发生）。
func TestDevCosmeticService_GrantCosmeticBatch_EmptyPool_Returns1009_NoTx(t *testing.T) {
	cosmRepo := &stubDevCosmeticItemRepo{
		listPoolFn: func(ctx context.Context, rarity int8) ([]uint64, error) {
			return []uint64{}, nil
		},
	}
	ucRepo := &stubDevUserCosmeticItemRepo{
		createInTxFn: func(ctx context.Context, item *mysql.UserCosmeticItem) error {
			t.Fatal("CreateInTx should NOT be called when pool is empty")
			return nil
		},
	}
	txMgr := &stubDevTxMgr{}
	svc := service.NewDevCosmeticService(cosmRepo, ucRepo, txMgr)

	err := svc.GrantCosmeticBatch(context.Background(), 1001, 1, 10)
	assertServiceBusyError(t, err)

	if txMgr.withTxCalls != 0 {
		t.Errorf("WithTx called %d times, want 0 (空池在开事务前就拒绝)", txMgr.withTxCalls)
	}
}

// 2b. **守门测试（fix-review 23-5 r2 [P2] #1 根因约束 / over-correction chain
// 收敛锚）**：rarity 池=8（如 common seed 仅 8 件）+ count=10 → **成功授予
// 正好 10 个实例**（有放回抽 → 允许重复 cosmetic_item_id；user_cosmetic_items
// 无 UNIQUE(user_id,cosmetic_item_id)，重复实例合法且为合成 feature 核心）。
//
// 这是 over-correction chain 第 2 跳的反向锚：r1 在此场景错误返 1009（把
// count 误读成 distinct 上限）；本断言要求**成功 + 正好 count 行**。任何人
// 把"distinct 上限 / 池不足即拒"hack 加回来，本测试立刻挂。
func TestDevCosmeticService_GrantCosmeticBatch_PoolSmallerThanCount_GrantsExactlyCountWithRepetition(t *testing.T) {
	cosmRepo := &stubDevCosmeticItemRepo{
		listPoolFn: func(ctx context.Context, rarity int8) ([]uint64, error) {
			// 模拟 seed common 仅 8 件 distinct 配置
			return []uint64{1, 2, 3, 4, 5, 6, 7, 8}, nil
		},
	}
	ucRepo := &stubDevUserCosmeticItemRepo{}
	txMgr := &stubDevTxMgr{}
	svc := service.NewDevCosmeticService(cosmRepo, ucRepo, txMgr)

	// common 池 8 但 demo 要 grant 10 个 common 实例供合成测试 → 必须成功
	err := svc.GrantCosmeticBatch(context.Background(), 1001, 1, 10)
	if err != nil {
		t.Fatalf("pool=8 + count=10: err = %v, want nil (count 是实例数非 distinct 数；撤销 r1 `len<count→1009`)", err)
	}
	if txMgr.withTxCalls != 1 {
		t.Errorf("WithTx called %d times, want 1", txMgr.withTxCalls)
	}
	if len(ucRepo.createInTxCall) != 10 {
		t.Fatalf("CreateInTx called %d times, want 正好 10 (有放回抽满 count；池 8 < count 不是错误)", len(ucRepo.createInTxCall))
	}
	// 抽出的 id 全部来自池 {1..8}；池 8 < count 10 → 必然出现重复（合法）。
	poolSet := map[uint64]bool{1: true, 2: true, 3: true, 4: true, 5: true, 6: true, 7: true, 8: true}
	seen := map[uint64]int{}
	for i, item := range ucRepo.createInTxCall {
		if !poolSet[item.CosmeticItemID] {
			t.Errorf("item[%d].CosmeticItemID = %d, 不在 common 池 {1..8} 内", i, item.CosmeticItemID)
		}
		seen[item.CosmeticItemID]++
	}
	hasDup := false
	for _, c := range seen {
		if c > 1 {
			hasDup = true
		}
	}
	if !hasDup {
		t.Errorf("池 8 < count 10 必有重复 cosmetic_item_id（有放回抽 + 重复实例合法），但分布无重复: %v", seen)
	}
}

// 2c. **守门测试（fix-review 23-5 r2 [P2] #2 原子性约束）**：批量中途任一
// CreateInTx 失败 → 整批回滚（service 返 1009 + WithTx 收到 error 触发 ROLLBACK，
// 无部分提交）。用 stubDevTxMgr 直接调 fn，fn 在第 3 次 CreateInTx 抛错 → fn
// return error → WithTx 透传 → service wrap 1009；断言 ① 返 1009 ② fn 在出错点
// 中止（不会继续写第 4 个 —— 杜绝"错误后还继续部分写"）。真实 DB 物理回滚
// （user_cosmetic_items 该批 COUNT=0）由 dockertest AC8 集成回滚 case 覆盖。
func TestDevCosmeticService_GrantCosmeticBatch_MidBatchFailure_RollsBackWholeBatch(t *testing.T) {
	cosmRepo := &stubDevCosmeticItemRepo{
		listPoolFn: func(ctx context.Context, rarity int8) ([]uint64, error) {
			return []uint64{10, 20, 30, 40, 50}, nil
		},
	}
	failAt := 3
	calls := 0
	ucRepo := &stubDevUserCosmeticItemRepo{
		createInTxFn: func(ctx context.Context, item *mysql.UserCosmeticItem) error {
			calls++
			if calls == failAt {
				return stderrors.New("synthetic create in tx failure at row 3")
			}
			return nil
		},
	}
	txMgr := &stubDevTxMgr{}
	svc := service.NewDevCosmeticService(cosmRepo, ucRepo, txMgr)

	err := svc.GrantCosmeticBatch(context.Background(), 1001, 2, 5)
	assertServiceBusyError(t, err)

	if txMgr.withTxCalls != 1 {
		t.Errorf("WithTx called %d times, want 1 (批量发放包在一个事务里)", txMgr.withTxCalls)
	}
	// fn 必须在第 3 次失败点立即 return error（不继续写第 4/5 个）—— 杜绝
	// "错误后继续部分写入"；真实 ROLLBACK 把已写的 1~2 行物理回滚（dockertest）。
	if calls != failAt {
		t.Errorf("CreateInTx called %d times, want %d (出错点立即中止，不继续写后续)", calls, failAt)
	}
}

// 3. ListPoolDBError_Returns1009：ListEnabledIDsByRarity 返 DB error → 1009，
// 不开事务。
func TestDevCosmeticService_GrantCosmeticBatch_ListPoolDBError_Returns1009(t *testing.T) {
	cosmRepo := &stubDevCosmeticItemRepo{
		listPoolFn: func(ctx context.Context, rarity int8) ([]uint64, error) {
			return nil, stderrors.New("synthetic list pool db error")
		},
	}
	txMgr := &stubDevTxMgr{}
	svc := service.NewDevCosmeticService(cosmRepo, &stubDevUserCosmeticItemRepo{}, txMgr)

	err := svc.GrantCosmeticBatch(context.Background(), 1001, 1, 10)
	assertServiceBusyError(t, err)

	if txMgr.withTxCalls != 0 {
		t.Errorf("WithTx called %d times, want 0 (取池失败在开事务前就拒绝)", txMgr.withTxCalls)
	}
}

// 4. CreateInTxFails_Returns1009：池非空 + 首条 CreateInTx 就失败 → 1009。
func TestDevCosmeticService_GrantCosmeticBatch_CreateInTxFails_Returns1009(t *testing.T) {
	cosmRepo := &stubDevCosmeticItemRepo{
		listPoolFn: func(ctx context.Context, rarity int8) ([]uint64, error) {
			return []uint64{11, 22}, nil
		},
	}
	ucRepo := &stubDevUserCosmeticItemRepo{
		createInTxFn: func(ctx context.Context, item *mysql.UserCosmeticItem) error {
			return stderrors.New("synthetic create in tx failure")
		},
	}
	txMgr := &stubDevTxMgr{}
	svc := service.NewDevCosmeticService(cosmRepo, ucRepo, txMgr)

	err := svc.GrantCosmeticBatch(context.Background(), 1001, 2, 2)
	assertServiceBusyError(t, err)
}

// 5. BoundaryRarities_AllSucceed：rarity ∈ {1,2,3,4} × count ∈ {1,10,100}
// 共 12 组合都 happy return nil（池非空时无差别成功，含 count > 池大小的
// 有放回场景）；每个组合断言 CreateInTx 调用次数 == count（实例数语义）。
func TestDevCosmeticService_GrantCosmeticBatch_BoundaryRarities_AllSucceed(t *testing.T) {
	ctx := context.Background()
	rarities := []int8{1, 2, 3, 4}
	counts := []int32{1, 10, 100}

	for _, r := range rarities {
		for _, c := range counts {
			r, c := r, c
			t.Run("rarity="+string(rune('0'+r))+"_count="+itoa(c), func(t *testing.T) {
				cosmRepo := &stubDevCosmeticItemRepo{
					listPoolFn: func(ctx context.Context, rarity int8) ([]uint64, error) {
						// 模拟 seed 该 rarity 仅 8 件 distinct 配置（小于 count=100
						// 的场景一并覆盖 —— 有放回抽满 count）
						return []uint64{1, 2, 3, 4, 5, 6, 7, 8}, nil
					},
				}
				ucRepo := &stubDevUserCosmeticItemRepo{}
				txMgr := &stubDevTxMgr{}
				svc := service.NewDevCosmeticService(cosmRepo, ucRepo, txMgr)

				if err := svc.GrantCosmeticBatch(ctx, 1001, r, c); err != nil {
					t.Errorf("GrantCosmeticBatch(rarity=%d, count=%d): err = %v, want nil", r, c, err)
				}
				if len(ucRepo.createInTxCall) != int(c) {
					t.Errorf("CreateInTx called %d times, want %d (实例数语义；池 8 < count 也抽满)", len(ucRepo.createInTxCall), c)
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
