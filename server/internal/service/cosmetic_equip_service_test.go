//go:build !integration

// Story 26.3 cosmetic_equip_service 单元测试：≥6 case（epics.md §26.3
// 行 3557-3563 钦定全覆盖）+ 建议补的 missing-no-row → 5003 + slog 路径 /
// DB UNIQUE 哨兵 → 1009 / 状态分支矩阵补全（status=4 → 5003、status=2 → 5008）。
//
// 模式与 chest_open_service_test.go 同：注入 stub repo（实现 repo interface）
// + stub tx.Manager（WithTx 直接调 fn 透传 ctx 不真起事务；真事务回滚由
// cosmetic_equip_service_integration_test.go dockertest 覆盖）。
//
// 复用 service_test package 已有共享 stub：
//   - stubTxMgr / defaultStubTxMgr（auth_service_test.go，无 build tag）
//   - stubUserCosmeticItemRepo（cosmetic_service_test.go，无 build tag；
//     Story 26.3 已扩 findByIDForEquipFn / updateStatusInTxFn /
//     updateStatusInTxCalls 供 equip 路径用）
//
// 本文件新增专用 stub（仅 equip 路径用，避免污染既有共享 stub 的 panic-default
// 语义）：stubEquipPetRepo / stubEquipCosmeticItemRepo / stubUserPetEquipRepo。

package service_test

import (
	"context"
	stderrors "errors"
	"testing"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// ================================================================
// equip 专用 stub
// ================================================================

// stubEquipPetRepo 实现 mysql.PetRepo；仅 FindByID 注入（equip 步骤 6 用）；
// 其他方法 panic-default（equip 不调）。
type stubEquipPetRepo struct {
	findByIDFn func(ctx context.Context, petID uint64) (*mysql.Pet, error)
}

func (s *stubEquipPetRepo) Create(ctx context.Context, p *mysql.Pet) error {
	panic("stubEquipPetRepo.Create not expected (equip 不调)")
}
func (s *stubEquipPetRepo) FindDefaultByUserID(ctx context.Context, userID uint64) (*mysql.Pet, error) {
	panic("stubEquipPetRepo.FindDefaultByUserID not expected (equip 步骤 6 走 FindByID)")
}
func (s *stubEquipPetRepo) UpdateCurrentStateByID(ctx context.Context, petID uint64, state int8) error {
	panic("stubEquipPetRepo.UpdateCurrentStateByID not expected (equip 不调)")
}
func (s *stubEquipPetRepo) FindByID(ctx context.Context, petID uint64) (*mysql.Pet, error) {
	return s.findByIDFn(ctx, petID)
}

// stubEquipCosmeticItemRepo 实现 mysql.CosmeticItemRepo；仅 FindSlotNameByID
// 注入（equip 步骤 7 用）；其他方法 panic-default（equip 不调）。
type stubEquipCosmeticItemRepo struct {
	findSlotNameFn func(ctx context.Context, id uint64) (int8, string, bool, error)
}

func (s *stubEquipCosmeticItemRepo) ListEnabledForWeightedPick(ctx context.Context) ([]mysql.CosmeticItem, error) {
	panic("stubEquipCosmeticItemRepo.ListEnabledForWeightedPick not expected (equip 不调)")
}
func (s *stubEquipCosmeticItemRepo) ListEnabledForCatalog(ctx context.Context) ([]mysql.CosmeticItem, error) {
	panic("stubEquipCosmeticItemRepo.ListEnabledForCatalog not expected (equip 不调)")
}
func (s *stubEquipCosmeticItemRepo) ListByIDsForInventory(ctx context.Context, ids []uint64) ([]mysql.CosmeticItem, error) {
	panic("stubEquipCosmeticItemRepo.ListByIDsForInventory not expected (equip 不调)")
}
func (s *stubEquipCosmeticItemRepo) ListEnabledIDsByRarity(ctx context.Context, rarity int8) ([]uint64, error) {
	panic("stubEquipCosmeticItemRepo.ListEnabledIDsByRarity not expected (equip 不调)")
}
func (s *stubEquipCosmeticItemRepo) FindSlotNameByID(ctx context.Context, id uint64) (int8, string, bool, error) {
	return s.findSlotNameFn(ctx, id)
}

// stubUserPetEquipRepo 实现 mysql.UserPetEquipRepo（Story 26.3 首个 stub）。
//   - findByPetSlotFn：步骤 8 同槽换装判定（注入返 (*UserPetEquip,nil) =
//     有旧装备 / (nil, ErrUserPetEquipNotFound) = slot 无装备）
//   - deleteByPetSlotInTxCalls：记录步骤 8 删旧行调用 (petID, slot)
//   - insertInTxFn：步骤 9 INSERT（注入返 ErrUserPetEquipPetSlotDuplicate
//     等哨兵模拟 DB UNIQUE 兜底）；insertInTxCall 记录插入的 *UserPetEquip
type stubUserPetEquipRepo struct {
	findByPetSlotFn        func(ctx context.Context, petID uint64, slot int8) (*mysql.UserPetEquip, error)
	deleteByPetSlotInTxFn  func(ctx context.Context, petID uint64, slot int8) error
	insertInTxFn           func(ctx context.Context, e *mysql.UserPetEquip) error
	deleteByPetSlotInTxArg []petSlotArg
	insertInTxCall         []*mysql.UserPetEquip
}

type petSlotArg struct {
	petID uint64
	slot  int8
}

func (s *stubUserPetEquipRepo) FindByPetSlot(ctx context.Context, petID uint64, slot int8) (*mysql.UserPetEquip, error) {
	return s.findByPetSlotFn(ctx, petID, slot)
}
func (s *stubUserPetEquipRepo) DeleteByPetSlotInTx(ctx context.Context, petID uint64, slot int8) error {
	s.deleteByPetSlotInTxArg = append(s.deleteByPetSlotInTxArg, petSlotArg{petID: petID, slot: slot})
	if s.deleteByPetSlotInTxFn != nil {
		return s.deleteByPetSlotInTxFn(ctx, petID, slot)
	}
	return nil
}
func (s *stubUserPetEquipRepo) InsertInTx(ctx context.Context, e *mysql.UserPetEquip) error {
	s.insertInTxCall = append(s.insertInTxCall, e)
	if s.insertInTxFn != nil {
		return s.insertInTxFn(ctx, e)
	}
	return nil
}

// buildEquipService 装配 CosmeticEquipService（defaultStubTxMgr 直接调 fn）。
func buildEquipService(
	uc *stubUserCosmeticItemRepo,
	ci *stubEquipCosmeticItemRepo,
	pet *stubEquipPetRepo,
	upe *stubUserPetEquipRepo,
) service.CosmeticEquipService {
	return service.NewCosmeticEquipService(defaultStubTxMgr(), uc, ci, pet, upe)
}

// assertEquipAppErrCode 断言 err 是 *AppError 且 Code == want。
func assertEquipAppErrCode(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("err = nil, want AppError code=%d", want)
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("err 不是 *AppError: %v", err)
	}
	if ae.Code != want {
		t.Errorf("AppError.Code = %d, want %d", ae.Code, want)
	}
}

// ================================================================
// AC5 钦定 case
// ================================================================

// happy 1: 该 slot 无装备 → 直接装上（user_pet_equips 多 1 行 + 当前实例
// status→2 + 旧装备相关 mock 未被调）。
func TestEquip_HappyPath_SlotEmpty_DirectEquip(t *testing.T) {
	uc := &stubUserCosmeticItemRepo{
		findByIDForEquipFn: func(ctx context.Context, id uint64) (*mysql.UserCosmeticItem, error) {
			return &mysql.UserCosmeticItem{ID: 90001, CosmeticItemID: 12, Status: 1, UserID: 42}, nil
		},
	}
	ci := &stubEquipCosmeticItemRepo{
		findSlotNameFn: func(ctx context.Context, id uint64) (int8, string, bool, error) {
			return 1, "小黄帽", true, nil
		},
	}
	pet := &stubEquipPetRepo{
		findByIDFn: func(ctx context.Context, petID uint64) (*mysql.Pet, error) {
			return &mysql.Pet{ID: 2001, UserID: 42}, nil
		},
	}
	upe := &stubUserPetEquipRepo{
		findByPetSlotFn: func(ctx context.Context, petID uint64, slot int8) (*mysql.UserPetEquip, error) {
			return nil, mysql.ErrUserPetEquipNotFound // slot 无装备
		},
	}
	svc := buildEquipService(uc, ci, pet, upe)

	out, err := svc.Equip(context.Background(), service.EquipParams{UserID: 42, PetID: 2001, UserCosmeticItemID: 90001})
	if err != nil {
		t.Fatalf("Equip happy: err = %v, want nil", err)
	}
	if out.PetID != 2001 || out.Equipped.Slot != 1 || out.Equipped.UserCosmeticItemID != 90001 ||
		out.Equipped.CosmeticItemID != 12 || out.Equipped.Name != "小黄帽" {
		t.Errorf("EquipResult = %+v, want petId=2001 slot=1 uci=90001 ci=12 name=小黄帽", out)
	}
	// InsertInTx 被调 1 次，插入字段正确
	if len(upe.insertInTxCall) != 1 {
		t.Fatalf("InsertInTx called %d times, want 1", len(upe.insertInTxCall))
	}
	ins := upe.insertInTxCall[0]
	if ins.UserID != 42 || ins.PetID != 2001 || ins.Slot != 1 || ins.UserCosmeticItemID != 90001 {
		t.Errorf("inserted UserPetEquip = %+v, want {UserID:42 PetID:2001 Slot:1 UserCosmeticItemID:90001}", ins)
	}
	// 当前实例 status→2，且旧装备删除 / 旧实例 status→1 mock 未被调
	if len(upe.deleteByPetSlotInTxArg) != 0 {
		t.Errorf("DeleteByPetSlotInTx 被调 %d 次，want 0（slot 无旧装备）", len(upe.deleteByPetSlotInTxArg))
	}
	if len(uc.updateStatusInTxCalls) != 1 {
		t.Fatalf("UpdateStatusInTx called %d times, want 1 (仅当前实例 status→2)", len(uc.updateStatusInTxCalls))
	}
	if uc.updateStatusInTxCalls[0].id != 90001 || uc.updateStatusInTxCalls[0].status != 2 {
		t.Errorf("UpdateStatusInTx = %+v, want {id:90001 status:2}", uc.updateStatusInTxCalls[0])
	}
}

// happy 2: 该 slot 已有装备 → 旧装备 status 改 in_bag(1) + 旧 user_pet_equips
// 行删除 + 新行 INSERT + 新装备 status equipped(2)（断言调用顺序：删旧行 →
// 旧实例 status→1 → InsertInTx → 新实例 status→2）。
func TestEquip_HappyPath_SlotOccupied_SwapEquip(t *testing.T) {
	uc := &stubUserCosmeticItemRepo{
		findByIDForEquipFn: func(ctx context.Context, id uint64) (*mysql.UserCosmeticItem, error) {
			return &mysql.UserCosmeticItem{ID: 90002, CosmeticItemID: 13, Status: 1, UserID: 42}, nil
		},
	}
	ci := &stubEquipCosmeticItemRepo{
		findSlotNameFn: func(ctx context.Context, id uint64) (int8, string, bool, error) {
			return 1, "小红帽", true, nil
		},
	}
	pet := &stubEquipPetRepo{
		findByIDFn: func(ctx context.Context, petID uint64) (*mysql.Pet, error) {
			return &mysql.Pet{ID: 2001, UserID: 42}, nil
		},
	}
	upe := &stubUserPetEquipRepo{
		findByPetSlotFn: func(ctx context.Context, petID uint64, slot int8) (*mysql.UserPetEquip, error) {
			// slot 1 已有旧装备：旧实例 id=90001
			return &mysql.UserPetEquip{ID: 5, UserID: 42, PetID: 2001, Slot: 1, UserCosmeticItemID: 90001}, nil
		},
	}
	svc := buildEquipService(uc, ci, pet, upe)

	out, err := svc.Equip(context.Background(), service.EquipParams{UserID: 42, PetID: 2001, UserCosmeticItemID: 90002})
	if err != nil {
		t.Fatalf("Equip swap: err = %v, want nil", err)
	}
	if out.Equipped.UserCosmeticItemID != 90002 || out.Equipped.Slot != 1 {
		t.Errorf("EquipResult = %+v, want uci=90002 slot=1", out)
	}
	// 删旧行被调 1 次（pet=2001, slot=1）
	if len(upe.deleteByPetSlotInTxArg) != 1 || upe.deleteByPetSlotInTxArg[0] != (petSlotArg{petID: 2001, slot: 1}) {
		t.Errorf("DeleteByPetSlotInTx args = %+v, want [{2001 1}]", upe.deleteByPetSlotInTxArg)
	}
	// InsertInTx 被调 1 次
	if len(upe.insertInTxCall) != 1 {
		t.Fatalf("InsertInTx called %d times, want 1", len(upe.insertInTxCall))
	}
	// UpdateStatusInTx 调 2 次：先旧实例 90001→1，再新实例 90002→2（顺序）
	if len(uc.updateStatusInTxCalls) != 2 {
		t.Fatalf("UpdateStatusInTx called %d times, want 2 (旧→1 + 新→2)", len(uc.updateStatusInTxCalls))
	}
	if uc.updateStatusInTxCalls[0].id != 90001 || uc.updateStatusInTxCalls[0].status != 1 {
		t.Errorf("UpdateStatusInTx[0] = %+v, want {id:90001 status:1}（旧实例回 in_bag）", uc.updateStatusInTxCalls[0])
	}
	if uc.updateStatusInTxCalls[1].id != 90002 || uc.updateStatusInTxCalls[1].status != 2 {
		t.Errorf("UpdateStatusInTx[1] = %+v, want {id:90002 status:2}（新实例 equipped）", uc.updateStatusInTxCalls[1])
	}
}

// edge: 实例不存在 → 5001。
func TestEquip_InstanceNotFound_Returns5001(t *testing.T) {
	uc := &stubUserCosmeticItemRepo{
		findByIDForEquipFn: func(ctx context.Context, id uint64) (*mysql.UserCosmeticItem, error) {
			return nil, mysql.ErrUserCosmeticItemNotFound
		},
	}
	svc := buildEquipService(uc, &stubEquipCosmeticItemRepo{}, &stubEquipPetRepo{}, &stubUserPetEquipRepo{})

	_, err := svc.Equip(context.Background(), service.EquipParams{UserID: 42, PetID: 2001, UserCosmeticItemID: 99999})
	assertEquipAppErrCode(t, err, apperror.ErrCosmeticNotFound) // 5001
}

// edge: 实例存在但不属于当前用户 → 5002（恒为 5002，实装无自由度）。
func TestEquip_InstanceNotOwned_Returns5002(t *testing.T) {
	uc := &stubUserCosmeticItemRepo{
		findByIDForEquipFn: func(ctx context.Context, id uint64) (*mysql.UserCosmeticItem, error) {
			return &mysql.UserCosmeticItem{ID: 90001, CosmeticItemID: 12, Status: 1, UserID: 999}, nil // 属他人
		},
	}
	svc := buildEquipService(uc, &stubEquipCosmeticItemRepo{}, &stubEquipPetRepo{}, &stubUserPetEquipRepo{})

	_, err := svc.Equip(context.Background(), service.EquipParams{UserID: 42, PetID: 2001, UserCosmeticItemID: 90001})
	assertEquipAppErrCode(t, err, apperror.ErrCosmeticNotOwned) // 5002
}

// edge: 实例 status=consumed(3) → 5003。
func TestEquip_InstanceConsumed_Returns5003(t *testing.T) {
	uc := &stubUserCosmeticItemRepo{
		findByIDForEquipFn: func(ctx context.Context, id uint64) (*mysql.UserCosmeticItem, error) {
			return &mysql.UserCosmeticItem{ID: 90001, CosmeticItemID: 12, Status: 3, UserID: 42}, nil
		},
	}
	svc := buildEquipService(uc, &stubEquipCosmeticItemRepo{}, &stubEquipPetRepo{}, &stubUserPetEquipRepo{})

	_, err := svc.Equip(context.Background(), service.EquipParams{UserID: 42, PetID: 2001, UserCosmeticItemID: 90001})
	assertEquipAppErrCode(t, err, apperror.ErrCosmeticInvalidState) // 5003
}

// edge（补全状态矩阵）：实例 status=invalid(4) → 5003。
func TestEquip_InstanceInvalid_Returns5003(t *testing.T) {
	uc := &stubUserCosmeticItemRepo{
		findByIDForEquipFn: func(ctx context.Context, id uint64) (*mysql.UserCosmeticItem, error) {
			return &mysql.UserCosmeticItem{ID: 90001, CosmeticItemID: 12, Status: 4, UserID: 42}, nil
		},
	}
	svc := buildEquipService(uc, &stubEquipCosmeticItemRepo{}, &stubEquipPetRepo{}, &stubUserPetEquipRepo{})

	_, err := svc.Equip(context.Background(), service.EquipParams{UserID: 42, PetID: 2001, UserCosmeticItemID: 90001})
	assertEquipAppErrCode(t, err, apperror.ErrCosmeticInvalidState) // 5003
}

// edge（补全状态矩阵）：实例 status=equipped(2) → 5008。
func TestEquip_InstanceAlreadyEquipped_Returns5008(t *testing.T) {
	uc := &stubUserCosmeticItemRepo{
		findByIDForEquipFn: func(ctx context.Context, id uint64) (*mysql.UserCosmeticItem, error) {
			return &mysql.UserCosmeticItem{ID: 90001, CosmeticItemID: 12, Status: 2, UserID: 42}, nil
		},
	}
	svc := buildEquipService(uc, &stubEquipCosmeticItemRepo{}, &stubEquipPetRepo{}, &stubUserPetEquipRepo{})

	_, err := svc.Equip(context.Background(), service.EquipParams{UserID: 42, PetID: 2001, UserCosmeticItemID: 90001})
	assertEquipAppErrCode(t, err, apperror.ErrCosmeticAlreadyEquipped) // 5008
}

// edge: pet 不属于当前用户 → 5002。
func TestEquip_PetNotOwned_Returns5002(t *testing.T) {
	uc := &stubUserCosmeticItemRepo{
		findByIDForEquipFn: func(ctx context.Context, id uint64) (*mysql.UserCosmeticItem, error) {
			return &mysql.UserCosmeticItem{ID: 90001, CosmeticItemID: 12, Status: 1, UserID: 42}, nil
		},
	}
	pet := &stubEquipPetRepo{
		findByIDFn: func(ctx context.Context, petID uint64) (*mysql.Pet, error) {
			return &mysql.Pet{ID: 2001, UserID: 999}, nil // pet 属他人
		},
	}
	svc := buildEquipService(uc, &stubEquipCosmeticItemRepo{}, pet, &stubUserPetEquipRepo{})

	_, err := svc.Equip(context.Background(), service.EquipParams{UserID: 42, PetID: 2001, UserCosmeticItemID: 90001})
	assertEquipAppErrCode(t, err, apperror.ErrCosmeticNotOwned) // 5002
}

// edge（补）：pet 不存在 → 5002（契约只给 5002 一个出口）。
func TestEquip_PetNotFound_Returns5002(t *testing.T) {
	uc := &stubUserCosmeticItemRepo{
		findByIDForEquipFn: func(ctx context.Context, id uint64) (*mysql.UserCosmeticItem, error) {
			return &mysql.UserCosmeticItem{ID: 90001, CosmeticItemID: 12, Status: 1, UserID: 42}, nil
		},
	}
	pet := &stubEquipPetRepo{
		findByIDFn: func(ctx context.Context, petID uint64) (*mysql.Pet, error) {
			return nil, mysql.ErrPetNotFound
		},
	}
	svc := buildEquipService(uc, &stubEquipCosmeticItemRepo{}, pet, &stubUserPetEquipRepo{})

	_, err := svc.Equip(context.Background(), service.EquipParams{UserID: 42, PetID: 9999, UserCosmeticItemID: 90001})
	assertEquipAppErrCode(t, err, apperror.ErrCosmeticNotOwned) // 5002
}

// edge（建议补）：missing-no-row（cosmetic_items 查不到 found=false）→ 5003
// + 验证不落 1009 / 不复用 5001（fix-review 26-1 r2 [P2] 锁定）。slog error
// 路径在本 case 自然触发（service 内 slog.ErrorContext，无 panic 即覆盖）。
func TestEquip_MissingNoRow_Returns5003(t *testing.T) {
	uc := &stubUserCosmeticItemRepo{
		findByIDForEquipFn: func(ctx context.Context, id uint64) (*mysql.UserCosmeticItem, error) {
			return &mysql.UserCosmeticItem{ID: 90001, CosmeticItemID: 12, Status: 1, UserID: 42}, nil
		},
	}
	ci := &stubEquipCosmeticItemRepo{
		findSlotNameFn: func(ctx context.Context, id uint64) (int8, string, bool, error) {
			// admin 物理删了 cosmetic_items 行 → found=false, err=nil
			return 0, "", false, nil
		},
	}
	pet := &stubEquipPetRepo{
		findByIDFn: func(ctx context.Context, petID uint64) (*mysql.Pet, error) {
			return &mysql.Pet{ID: 2001, UserID: 42}, nil
		},
	}
	svc := buildEquipService(uc, ci, pet, &stubUserPetEquipRepo{})

	_, err := svc.Equip(context.Background(), service.EquipParams{UserID: 42, PetID: 2001, UserCosmeticItemID: 90001})
	assertEquipAppErrCode(t, err, apperror.ErrCosmeticInvalidState) // 5003（**不**5001 / **不**1009）
}

// edge（建议补）：DB UNIQUE 哨兵（步骤 9 InsertInTx 返
// ErrUserPetEquipPetSlotDuplicate）→ 1009（service errors.Is 识别后回滚）。
func TestEquip_InsertPetSlotDuplicate_Returns1009(t *testing.T) {
	uc := &stubUserCosmeticItemRepo{
		findByIDForEquipFn: func(ctx context.Context, id uint64) (*mysql.UserCosmeticItem, error) {
			return &mysql.UserCosmeticItem{ID: 90001, CosmeticItemID: 12, Status: 1, UserID: 42}, nil
		},
	}
	ci := &stubEquipCosmeticItemRepo{
		findSlotNameFn: func(ctx context.Context, id uint64) (int8, string, bool, error) {
			return 1, "小黄帽", true, nil
		},
	}
	pet := &stubEquipPetRepo{
		findByIDFn: func(ctx context.Context, petID uint64) (*mysql.Pet, error) {
			return &mysql.Pet{ID: 2001, UserID: 42}, nil
		},
	}
	upe := &stubUserPetEquipRepo{
		findByPetSlotFn: func(ctx context.Context, petID uint64, slot int8) (*mysql.UserPetEquip, error) {
			return nil, mysql.ErrUserPetEquipNotFound
		},
		insertInTxFn: func(ctx context.Context, e *mysql.UserPetEquip) error {
			return mysql.ErrUserPetEquipPetSlotDuplicate // 并发：DB UNIQUE 拒绝
		},
	}
	svc := buildEquipService(uc, ci, pet, upe)

	_, err := svc.Equip(context.Background(), service.EquipParams{UserID: 42, PetID: 2001, UserCosmeticItemID: 90001})
	assertEquipAppErrCode(t, err, apperror.ErrServiceBusy) // 1009
}

// edge（补）：DB UNIQUE item 哨兵（ErrUserPetEquipItemDuplicate）→ 1009。
func TestEquip_InsertItemDuplicate_Returns1009(t *testing.T) {
	uc := &stubUserCosmeticItemRepo{
		findByIDForEquipFn: func(ctx context.Context, id uint64) (*mysql.UserCosmeticItem, error) {
			return &mysql.UserCosmeticItem{ID: 90001, CosmeticItemID: 12, Status: 1, UserID: 42}, nil
		},
	}
	ci := &stubEquipCosmeticItemRepo{
		findSlotNameFn: func(ctx context.Context, id uint64) (int8, string, bool, error) {
			return 1, "小黄帽", true, nil
		},
	}
	pet := &stubEquipPetRepo{
		findByIDFn: func(ctx context.Context, petID uint64) (*mysql.Pet, error) {
			return &mysql.Pet{ID: 2001, UserID: 42}, nil
		},
	}
	upe := &stubUserPetEquipRepo{
		findByPetSlotFn: func(ctx context.Context, petID uint64, slot int8) (*mysql.UserPetEquip, error) {
			return nil, mysql.ErrUserPetEquipNotFound
		},
		insertInTxFn: func(ctx context.Context, e *mysql.UserPetEquip) error {
			return mysql.ErrUserPetEquipItemDuplicate
		},
	}
	svc := buildEquipService(uc, ci, pet, upe)

	_, err := svc.Equip(context.Background(), service.EquipParams{UserID: 42, PetID: 2001, UserCosmeticItemID: 90001})
	assertEquipAppErrCode(t, err, apperror.ErrServiceBusy) // 1009
}

// edge（补）：步骤 4 DB 异常（非 NotFound 哨兵）→ 1009（验证 service 把 raw
// DB error wrap 成 1009 而非误判 5001）。
func TestEquip_FindInstanceDBError_Returns1009(t *testing.T) {
	uc := &stubUserCosmeticItemRepo{
		findByIDForEquipFn: func(ctx context.Context, id uint64) (*mysql.UserCosmeticItem, error) {
			return nil, stderrors.New("mysql: connection reset")
		},
	}
	svc := buildEquipService(uc, &stubEquipCosmeticItemRepo{}, &stubEquipPetRepo{}, &stubUserPetEquipRepo{})

	_, err := svc.Equip(context.Background(), service.EquipParams{UserID: 42, PetID: 2001, UserCosmeticItemID: 90001})
	assertEquipAppErrCode(t, err, apperror.ErrServiceBusy) // 1009
}

// edge（补）：入参兜底（UserID=0）→ 1002（handler 已校验，service 防御性兜底）。
func TestEquip_ZeroUserID_Returns1002(t *testing.T) {
	svc := buildEquipService(&stubUserCosmeticItemRepo{}, &stubEquipCosmeticItemRepo{}, &stubEquipPetRepo{}, &stubUserPetEquipRepo{})

	_, err := svc.Equip(context.Background(), service.EquipParams{UserID: 0, PetID: 2001, UserCosmeticItemID: 90001})
	assertEquipAppErrCode(t, err, apperror.ErrInvalidParam) // 1002
}
