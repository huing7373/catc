package service

// Story 26.3 — cosmetic_equip_service.go：CosmeticEquipService.Equip 单事务
// 实装（V1 §8.3 服务端逻辑步骤 4-11 + 数据库设计 §8.4 穿戴事务钦定）。
//
// 范围：本文件落地 CosmeticEquipService interface + impl + NewCosmeticEquipService
// 构造 + EquipParams / EquipResult / EquippedItem DTO。
//
// 与 cosmetic_service.go 关系（与 chest_open_service.go vs chest_service.go
// 分文件先例一致）：
//   - cosmetic_service.go 持有 CosmeticService（catalog / inventory 只读查询）
//   - 本文件是**写事务**（equip：校验 + 同槽换装卸旧 + INSERT user_pet_equips
//     + status 1↔2 推进），与只读查询职责不同 → 独立 interface + 独立文件
//
// **决策锚定**（V1 §8.3 r1/r2 [P2] 锁定 + epics.md §26.3 + 数据库设计 §8.4）：
//   - 步骤 4 仅按实例 id 查（**禁** AND user_id 过滤）；row 无 → 5001；
//     row 存在但属他人 → 5002（fix-review 26-1 r1 [P2] 锁定，5001 仅"完全无 row"）
//   - 步骤 7 missing-no-row（cosmetic_items 行被 admin 物理删但实例仍 status=1）
//     → 5003 + slog error（fix-review 26-1 r2 [P2] 锁定；映射到已冻结集合内
//     5003，**不**新造 / **不**复用 5001 / **不**落 1009）
//   - equip **无** idempotencyKey（§8.3 行 1468/1559 钦定；重复 equip 同实例由
//     status=2 → 5008 + DB UNIQUE 兜底，**不**抄 chest_open 的 ClaimPending）
//   - DB UNIQUE 并发兜底（uk_pet_slot / uk_user_cosmetic_item_id）→ repo 双
//     哨兵 → service errors.Is → 1009（§8.3 关键约束行 1560 + NFR11）

import (
	"context"
	stderrors "errors"
	"log/slog"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// user_cosmetic_items.status 枚举（数据库设计 §6.10）—— equip 步骤 5/8/9 用。
const (
	cosmeticStatusInBag    int8 = 1 // 在背包（可被 equip）
	cosmeticStatusEquipped int8 = 2 // 已装备
	cosmeticStatusConsumed int8 = 3 // 已合成消耗（终态，不可 equip）
	cosmeticStatusInvalid  int8 = 4 // 无效（终态，不可 equip）
)

// EquipParams 是 CosmeticEquipService.Equip 输入 DTO（handler → service 转换）。
//
// 三字段均为 uint64（handler 已把 BIGINT 字符串 strconv.ParseUint 解析；
// 解析失败在 handler 层 1002 拦截，service 不再校验字符串形态）。
type EquipParams struct {
	UserID             uint64 // auth 中间件注入；handler 兜底校验非 0
	PetID              uint64 // 目标 pet id（请求 petId 解析）
	UserCosmeticItemID uint64 // 要穿戴的实例 id（请求 userCosmeticItemId 解析）
}

// EquippedItem 是 V1 §8.3 响应 `equipped` 对象的 service 侧表达。
//
// 字段值规则（V1 §8.3 步骤 11 + 字段表行 1514-1521）：
//   - Slot：步骤 7 从 cosmetic_items 查到的配置 slot（§6.8 枚举）
//   - UserCosmeticItemID：回显请求实例 id
//   - CosmeticItemID：实例的 cosmetic_item_id（步骤 4 查到）
//   - Name：步骤 7 从 cosmetic_items 查到的 name
type EquippedItem struct {
	Slot               int8
	UserCosmeticItemID uint64
	CosmeticItemID     uint64
	Name               string
}

// EquipResult 是 CosmeticEquipService.Equip 输出 DTO（handler 转译为
// V1 §8.3 wire DTO：所有 BIGINT id 字符串化、slot 为 int）。
type EquipResult struct {
	PetID    uint64
	Equipped EquippedItem
}

// UnequipParams 是 CosmeticEquipService.Unequip 输入 DTO（handler → service
// 转换；Story 26.4 引入，V1 §8.4 请求体行 1590-1600）。
//
// petId 由 handler 已 strconv.ParseUint 解析（解析失败在 handler 1002 拦截）；
// slot 由 handler 已校验在枚举 {1,2,3,4,5,6,7,99} 内（与 EquipParams 三字段
// 均 uint64 同模式 —— 但 unequip 请求**有 slot 无 userCosmeticItemId**，按
// slot 不按实例 id 定位，§5.10 UNIQUE(pet_id,slot) 保证唯一）。
type UnequipParams struct {
	UserID uint64 // auth 中间件注入；handler 兜底校验非 0
	PetID  uint64 // 目标 pet id（请求 petId 解析）
	Slot   int8   // 要卸下的槽位（请求 slot；§6.8 枚举）
}

// UnequipResult 是 CosmeticEquipService.Unequip 输出 DTO（handler 转译为
// V1 §8.4 wire DTO：petId 字符串化、slot int 直下、unequipped bool 直下）。
//
// 字段值规则（V1 §8.4 响应体字段表行 1617-1621）：
//   - PetID：回显请求 petId
//   - Slot：回显请求 slot
//   - Unequipped：**恒 true**（成功路径才返结果；失败路径 return error 不构造
//     result —— V1 §8.4 行 1611 / 行 1660 钦定，防 client 解析为可选/可 false）
type UnequipResult struct {
	PetID      uint64
	Slot       int8
	Unequipped bool
}

// CosmeticEquipService 是 POST /api/v1/cosmetics/equip + /unequip 的 service
// 接口（Story 26.3 引入 Equip；Story 26.4 加 Unequip —— equip/unequip 同为
// user_pet_equips 写事务，职责同族故同 interface/同 impl，**不**新建第三个
// service）。handler 单测注入 stub 实现本 interface。
type CosmeticEquipService interface {
	// Equip 执行穿戴事务（V1 §8.3 服务端逻辑步骤 3-11；步骤 4-9 全部在同一
	// txMgr.WithTx 事务内，任一步 err → 整体回滚 NFR1/NFR2）。
	//
	// 错误码（service 层翻译，handler 仅 c.Error + return）：
	//   - 实例完全无 row → 5001 ErrCosmeticNotFound
	//   - 实例存在但属他人 / pet 不属于当前用户（含 pet 不存在）→ 5002 ErrCosmeticNotOwned
	//   - 实例 status=2 → 5008 ErrCosmeticAlreadyEquipped
	//   - 实例 status=3/4 → 5003 ErrCosmeticInvalidState
	//   - missing-no-row（cosmetic_items 行被删）→ 5003 + slog error
	//   - DB UNIQUE 并发冲突 / 任何其他 DB 错 → 1009 ErrServiceBusy
	Equip(ctx context.Context, in EquipParams) (*EquipResult, error)

	// Unequip 执行卸下事务（V1 §8.4 服务端逻辑步骤 3-8；步骤 4-7 全部在同一
	// txMgr.WithTx 事务内，任一步 err → 整体回滚 NFR1/NFR2）。
	//
	// 错误码集合 {5002,5004,1002,1009}（V1 §1 节点 9 冻结的 unequip 错误码集
	// {1001,1002,1005,5002,5004,1009} 中由 service 层产出的子集；1001/1005
	// 由 authedGroup 中间件兜底，**不**在 service 层；**无** 5001/5003/5008
	// —— unequip 按 slot 不按实例 id，不查实例归属/不校实例状态）：
	//   - 入参兜底（UserID==0 / PetID==0 / slot 不在枚举）→ 1002 ErrInvalidParam
	//   - pet 不属于当前用户（含 pet 不存在）→ 5002 ErrCosmeticNotOwned
	//   - 该 slot 无装备（步骤 5 NotFound）/ 步骤 6 RowsAffected==0 并发兜底
	//     → 5004 ErrCosmeticSlotMismatch（回滚）
	//   - 任何 DB 错（步骤 4/5/6 raw error）→ 1009 ErrServiceBusy
	Unequip(ctx context.Context, in UnequipParams) (*UnequipResult, error)
}

// cosmeticEquipServiceImpl 是 CosmeticEquipService 的默认实装。
type cosmeticEquipServiceImpl struct {
	txMgr            txManager
	userCosmeticRepo mysql.UserCosmeticItemRepo
	cosmeticRepo     mysql.CosmeticItemRepo
	petRepo          mysql.PetRepo
	userPetEquipRepo mysql.UserPetEquipRepo
}

// txManager 是本 service 依赖的事务管理器抽象（= repo/tx.Manager 同签名）；
// 单测注入 stub 直接调 fn 不真起事务，与 chest_open_service 同模式。
type txManager interface {
	WithTx(ctx context.Context, fn func(txCtx context.Context) error) error
}

// NewCosmeticEquipService 构造 CosmeticEquipService。
//
// 注入 tx.Manager + 4 个 repo（router 复用既有 userCosmeticItemRepo /
// cosmeticItemRepo / petRepo 实例，仅 userPetEquipRepo 是 Story 26.3 新建）。
func NewCosmeticEquipService(
	txMgr txManager,
	userCosmeticRepo mysql.UserCosmeticItemRepo,
	cosmeticRepo mysql.CosmeticItemRepo,
	petRepo mysql.PetRepo,
	userPetEquipRepo mysql.UserPetEquipRepo,
) CosmeticEquipService {
	return &cosmeticEquipServiceImpl{
		txMgr:            txMgr,
		userCosmeticRepo: userCosmeticRepo,
		cosmeticRepo:     cosmeticRepo,
		petRepo:          petRepo,
		userPetEquipRepo: userPetEquipRepo,
	}
}

// Equip 实装：入参兜底校验 + txMgr.WithTx 内严格按 V1 §8.3 步骤 4-9 顺序。
//
// **关键**：WithTx fn 内所有 repo 调用用传入的 `txCtx`（**不**是外层 ctx）；
// ADR-0007 §2.4 + CLAUDE.md ctx 必传节（参照 chest_open_service.runOpenChestTx）。
func (s *cosmeticEquipServiceImpl) Equip(ctx context.Context, in EquipParams) (*EquipResult, error) {
	// 入参兜底（handler 已校验过 BIGINT 字符串；这里防御性兜底非 0）
	if in.UserID == 0 || in.PetID == 0 || in.UserCosmeticItemID == 0 {
		return nil, apperror.New(apperror.ErrInvalidParam, apperror.DefaultMessages[apperror.ErrInvalidParam])
	}

	var output *EquipResult
	err := s.txMgr.WithTx(ctx, func(txCtx context.Context) error {
		out, err := s.runEquipTx(txCtx, in)
		if err != nil {
			return err
		}
		output = out
		return nil
	})
	if err != nil {
		return nil, err
	}
	return output, nil
}

// runEquipTx 步骤 4-9 业务全流程（事务内调用）。
//
// **关键**：本函数内所有 repo 调用必须用传入的 `txCtx`（**不**是外层 ctx）；
// 与 ADR-0007 §2.4 钦定一致（与 chest_open_service.runOpenChestTx 同模式）。
func (s *cosmeticEquipServiceImpl) runEquipTx(txCtx context.Context, in EquipParams) (*EquipResult, error) {
	// 步骤 4 — 查实例归属（仅按 id 查，**禁** AND user_id 过滤）
	item, err := s.userCosmeticRepo.FindByIDForEquip(txCtx, in.UserCosmeticItemID)
	if err != nil {
		if stderrors.Is(err, mysql.ErrUserCosmeticItemNotFound) {
			// 行完全不存在 → 5001（fix-review 26-1 r1 [P2]：5001 仅此一种）
			return nil, apperror.Wrap(err, apperror.ErrCosmeticNotFound, apperror.DefaultMessages[apperror.ErrCosmeticNotFound])
		}
		// DB 异常 → 1009
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	if item.UserID != in.UserID {
		// 行存在但属他人 → 5002（恒为 5002，实装无自由度；fix-review 26-1 r1 [P2]）
		return nil, apperror.New(apperror.ErrCosmeticNotOwned, apperror.DefaultMessages[apperror.ErrCosmeticNotOwned])
	}

	// 步骤 5 — 校验实例状态
	switch item.Status {
	case cosmeticStatusInBag:
		// in_bag → 继续
	case cosmeticStatusEquipped:
		// 该实例当前已被穿戴，重复 equip 拒绝 → 5008
		return nil, apperror.New(apperror.ErrCosmeticAlreadyEquipped, apperror.DefaultMessages[apperror.ErrCosmeticAlreadyEquipped])
	case cosmeticStatusConsumed, cosmeticStatusInvalid:
		// 已合成消耗 / 无效（终态，不可穿戴）→ 5003
		return nil, apperror.New(apperror.ErrCosmeticInvalidState, apperror.DefaultMessages[apperror.ErrCosmeticInvalidState])
	default:
		// 枚举外状态（理论不应发生；按 5003 道具状态不可用兜底，
		// **不**落未定义路径 / 1009 —— 与 §8.3 步骤 5 "status != 1" 语义一致）
		return nil, apperror.New(apperror.ErrCosmeticInvalidState, apperror.DefaultMessages[apperror.ErrCosmeticInvalidState])
	}

	// 步骤 6 — 校验 pet 归属（pet 不存在亦视为 5002 —— 契约只给 5002 一个出口）
	pet, err := s.petRepo.FindByID(txCtx, in.PetID)
	if err != nil {
		if stderrors.Is(err, mysql.ErrPetNotFound) {
			// pet 不存在 → 与"非本人 pet"同处理为 5002（§8.3 步骤 6 + 错误码表行 1550）
			return nil, apperror.Wrap(err, apperror.ErrCosmeticNotOwned, apperror.DefaultMessages[apperror.ErrCosmeticNotOwned])
		}
		// DB 异常 → 1009
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	if pet.UserID != in.UserID {
		// pet 不属于当前用户 → 5002
		return nil, apperror.New(apperror.ErrCosmeticNotOwned, apperror.DefaultMessages[apperror.ErrCosmeticNotOwned])
	}

	// 步骤 7 — 查配置槽位（slot + name）
	slot, name, found, err := s.cosmeticRepo.FindSlotNameByID(txCtx, item.CosmeticItemID)
	if err != nil {
		// DB 异常（非 missing-no-row）→ 1009
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	if !found {
		// missing-no-row：admin 物理删了 cosmetic_items 行但实例仍 status=1
		// （与 §8.2 态 C 同源；client 可合法发起 equip）→ 5003 + slog error
		// （fix-review 26-1 r2 [P2] 锁定：映射到已冻结集合内 5003，**不**新造 /
		// **不**复用 5001 / **不**落 1009）。事务回滚（本步在事务内 return error）。
		slog.ErrorContext(txCtx,
			"equip: cosmetic_items row missing for owned user_cosmetic_items still status=in_bag (data governance event: admin physically deleted a cosmetic config still referenced by an equippable instance; slot unresolvable so the item cannot be equipped; ops should restore config / add down-migration / run deprecation flow)",
			"cosmetic_item_id", item.CosmeticItemID,
			"user_cosmetic_item_id", item.ID,
			"user_id", in.UserID,
			"pet_id", in.PetID,
		)
		return nil, apperror.New(apperror.ErrCosmeticInvalidState, apperror.DefaultMessages[apperror.ErrCosmeticInvalidState])
	}

	// 步骤 8 — 同槽换装（自动卸下旧装备）
	old, err := s.userPetEquipRepo.FindByPetSlot(txCtx, in.PetID, slot)
	if err != nil {
		if stderrors.Is(err, mysql.ErrUserPetEquipNotFound) {
			// 该 slot 无装备 → 跳过本步（合法 case，**非**异常）
		} else {
			// DB 异常 → 1009
			return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}
	} else {
		// 已有旧装备 → 删旧 user_pet_equips 行 + 旧实例 status 回 in_bag(1)
		if err := s.userPetEquipRepo.DeleteByPetSlotInTx(txCtx, in.PetID, slot); err != nil {
			return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}
		if err := s.userCosmeticRepo.UpdateStatusInTx(txCtx, old.UserCosmeticItemID, cosmeticStatusInBag); err != nil {
			return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}
	}

	// 步骤 9 — 绑定 + 状态推进
	equip := &mysql.UserPetEquip{
		UserID:             in.UserID,
		PetID:              in.PetID,
		Slot:               slot,
		UserCosmeticItemID: in.UserCosmeticItemID,
	}
	if err := s.userPetEquipRepo.InsertInTx(txCtx, equip); err != nil {
		// DB UNIQUE 并发兜底（uk_pet_slot / uk_user_cosmetic_item_id 双哨兵）→ 1009
		// （§8.3 关键约束行 1560 + NFR11；errors.Is 两哨兵均 → 1009；
		// 非哨兵 raw error 同样兜底 1009）
		if stderrors.Is(err, mysql.ErrUserPetEquipPetSlotDuplicate) ||
			stderrors.Is(err, mysql.ErrUserPetEquipItemDuplicate) {
			return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	if err := s.userCosmeticRepo.UpdateStatusInTx(txCtx, in.UserCosmeticItemID, cosmeticStatusEquipped); err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 步骤 10 — 提交（WithTx fn return nil → 自动 commit）
	// 步骤 11 — 响应
	return &EquipResult{
		PetID: in.PetID,
		Equipped: EquippedItem{
			Slot:               slot,
			UserCosmeticItemID: in.UserCosmeticItemID,
			CosmeticItemID:     item.CosmeticItemID,
			Name:               name,
		},
	}, nil
}

// validUnequipSlot 判 slot 是否在 §6.8 枚举 {1,2,3,4,5,6,7,99} 内（V1 §8.4
// 行 1593 / 行 1643；handler 已校验过，service 入参兜底再校一遍 —— 与 Equip
// 入参兜底 UserID/PetID 非 0 同防御性原则）。
func validUnequipSlot(s int8) bool {
	switch s {
	case 1, 2, 3, 4, 5, 6, 7, 99:
		return true
	default:
		return false
	}
}

// Unequip 实装：入参兜底校验 + txMgr.WithTx 内严格按 V1 §8.4 步骤 4-7 顺序。
//
// **关键**：WithTx fn 内所有 repo 调用用传入的 `txCtx`（**不**是外层 ctx）；
// ADR-0007 §2.4 + CLAUDE.md ctx 必传节（与 Equip 同骨架）。
func (s *cosmeticEquipServiceImpl) Unequip(ctx context.Context, in UnequipParams) (*UnequipResult, error) {
	// 入参兜底（handler 已校验过 BIGINT 字符串 + slot 枚举；这里防御性兜底）
	if in.UserID == 0 || in.PetID == 0 || !validUnequipSlot(in.Slot) {
		return nil, apperror.New(apperror.ErrInvalidParam, apperror.DefaultMessages[apperror.ErrInvalidParam])
	}

	var output *UnequipResult
	err := s.txMgr.WithTx(ctx, func(txCtx context.Context) error {
		out, err := s.runUnequipTx(txCtx, in)
		if err != nil {
			return err
		}
		output = out
		return nil
	})
	if err != nil {
		return nil, err
	}
	return output, nil
}

// runUnequipTx 步骤 4-7 业务全流程（事务内调用）。
//
// **关键**：本函数内所有 repo 调用必须用传入的 `txCtx`（**不**是外层 ctx）；
// 与 ADR-0007 §2.4 钦定一致（与 runEquipTx 同模式）。
//
// 步骤顺序严格锚定 V1 §8.4：先校 pet 归属（ACL 边界，不属主泄漏"该 pet
// 该 slot 有无装备"信息）→ 再 FOR UPDATE 查装备关系（并发卸下串行化）→
// DELETE 检查 RowsAffected（==0 回滚 + 5004 冗余兜底）→ UPDATE 实例
// status 回 in_bag。
func (s *cosmeticEquipServiceImpl) runUnequipTx(txCtx context.Context, in UnequipParams) (*UnequipResult, error) {
	// 步骤 4 — 校验 pet 归属（pet 不存在亦视为 5002 —— V1 §8.4 错误码表只给
	// 5002 一个出口；与 runEquipTx 步骤 6 pet 不存在恒 5002 不变量 1:1 一致）。
	// 顺序锚定：步骤 4 在步骤 5 查装备关系**之前**（先校 ACL 再查资源）。
	pet, err := s.petRepo.FindByID(txCtx, in.PetID)
	if err != nil {
		if stderrors.Is(err, mysql.ErrPetNotFound) {
			// pet 不存在 → 与"非本人 pet"同处理为 5002（V1 §8.4 步骤 4 + 行 1645）
			return nil, apperror.Wrap(err, apperror.ErrCosmeticNotOwned, apperror.DefaultMessages[apperror.ErrCosmeticNotOwned])
		}
		// DB 异常 → 1009
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	if pet.UserID != in.UserID {
		// pet 不属于当前用户 → 5002
		return nil, apperror.New(apperror.ErrCosmeticNotOwned, apperror.DefaultMessages[apperror.ErrCosmeticNotOwned])
	}

	// 步骤 5 — 查装备关系（FOR UPDATE 行锁串行化；fix-review 26-1 r2 [P1]
	// 锁定）。并发 unequip 在该 pet_id+slot 行排他锁上排队，输家进入本步时
	// 行已被赢家 DELETE → 查不到 → 5004，杜绝两个并发请求都越过本步。
	uciID, err := s.userPetEquipRepo.FindUserCosmeticItemIDByPetSlotForUpdate(txCtx, in.PetID, in.Slot)
	if err != nil {
		if stderrors.Is(err, mysql.ErrUserPetEquipNotFound) {
			// 该 slot 当前无装备，无可卸下对象 → 5004（V1 §8.4 行 1608 + 1646；
			// **非**幂等成功 —— V1 §8.4 行 1649/1651 钦定空槽显式报错）
			return nil, apperror.Wrap(err, apperror.ErrCosmeticSlotMismatch, apperror.DefaultMessages[apperror.ErrCosmeticSlotMismatch])
		}
		// DB 异常 → 1009
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 步骤 6 — 解绑 + 状态回退（DELETE 必须检查 affected rows；fix-review
	// 26-1 r2 [P1] 锁定）。
	affected, err := s.userPetEquipRepo.DeleteByPetSlotInTxReturningAffected(txCtx, in.PetID, in.Slot)
	if err != nil {
		// DB 异常 → 1009
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}
	if affected == 0 {
		// 步骤 5 与本步之间该行已被并发事务删除（理论上已由步骤 5 FOR UPDATE
		// 排他锁阻止，本检查为不依赖锁实现细节的契约级冗余兜底）→ 回滚事务 +
		// 返回 5004（**禁止**带着 0 affected rows 继续 commit 而误返
		// unequipped: true；V1 §8.4 行 1609 / 1651 / 1657 钦定）。
		return nil, apperror.New(apperror.ErrCosmeticSlotMismatch, apperror.DefaultMessages[apperror.ErrCosmeticSlotMismatch])
	}
	// affected >= 1（理论上恒 1，uk_pet_slot UNIQUE 保证至多 1 行；> 1 不可能
	// 但按 != 0 即成功兜底，与 room_member_repo.DeleteByRoomAndUser service
	// 兜底同模式）→ 继续 UPDATE 实例 status 回 in_bag(1)
	if err := s.userCosmeticRepo.UpdateStatusInTx(txCtx, uciID, cosmeticStatusInBag); err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// 步骤 7 — 提交（WithTx fn return nil → 自动 commit）
	// 步骤 8 — 响应（unequipped 恒 true —— 失败走错误码不返 false）
	return &UnequipResult{
		PetID:      in.PetID,
		Slot:       in.Slot,
		Unequipped: true,
	}, nil
}
