package service

import (
	"context"
	stderrors "errors"
	"log/slog"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// HomeService 是 home handler 的依赖 interface（便于 handler 单测 mock）。
//
// **接口而非具体类型**：handler 单测注入 stub struct，与 4.6 AuthService 同模式。
type HomeService interface {
	// LoadHome 一次性聚合查询主界面所需全部数据（user / pet / stepAccount / chest）。
	//
	// 流程：
	//  1. userRepo.FindByID(ctx, userID) → user（必有）
	//  2. petRepo.FindDefaultByUserID(ctx, userID) → pet（可空：ErrPetNotFound 视为 Pet=nil）
	//  3. stepAccountRepo.FindByUserID(ctx, userID) → step_account（必有）
	//  4. chestRepo.FindByUserID(ctx, userID) → user_chest（必有）
	//  5. service 层动态判定 chest.status / remainingSeconds（基于 time.Now().UTC() vs unlockAt）
	//  6. 拼装 HomeOutput 返回
	//
	// 错误约定（**不部分降级** —— epics.md §Story 4.8 行 1136 钦定）：
	//   - userRepo 失败（含 ErrUserNotFound）→ 包成 1009（auth 中间件已校验 token，user 必须存在）
	//   - petRepo NotFound → 不视为错误，HomeOutput.Pet = nil（V1 §5.1 行 335 钦定 pet 容器可空）
	//   - petRepo 其他失败 → 包成 1009
	//   - stepAccountRepo / chestRepo 失败（含 NotFound）→ 包成 1009（这两张表登录初始化必建）
	//
	// 任一聚合查询失败 → 整体 1009 服务繁忙；不返"半截 HomeOutput"，避免主界面渲染异常。
	LoadHome(ctx context.Context, userID uint64) (*HomeOutput, error)
}

// HomeOutput 是 service 层 DTO（**不是** wire DTO，handler 转换为 V1 §5.1 钦定 wire 格式）。
//
// 字段语义：
//   - User: 必有（登录后 user 必然存在；查询失败 → 1009）
//   - Pet: 可空（用户无默认 pet → nil；V1 §5.1 钦定的 edge case）
//   - StepAccount: 必有（登录初始化时已建；缺 → 1009）
//   - Chest: 必有 + Status / RemainingSeconds 已动态计算（不是 DB 原值）
//   - Room: 必有容器（即便用户不在任何房间也是 RoomBrief{} 而非 nil）—— V1 §5.1 行 374
//     钦定 data.room **容器永远存在**，currentRoomId 字段才可空；详见 RoomBrief 注释
type HomeOutput struct {
	User        UserBrief
	Pet         *PetBrief // 可空（nil = 用户无默认 pet）
	StepAccount StepAccountBrief
	Chest       ChestBrief
	Room        RoomBrief
}

// UserBrief 是 V1 §5.1 data.user 的 service 层映射。
type UserBrief struct {
	ID        uint64
	Nickname  string
	AvatarURL string
}

// EquipBrief 是 V1 §5.1 data.pet.equips[] 元素的 service 层映射（Story 26.6
// 引入）。节点 9 阶段含 6 字段；节点 10 由 Story 29.6 加 RenderConfig（本
// story 不做 —— V1 §5.1 行 517 钦定，严格 6 字段无 renderConfig）。
//
// 字段类型与跨接口同义对齐（AC7）：slot/rarity int 数字、2 个 id uint64
// （handler 层 strconv.FormatUint 字符串化）、name/assetUrl string 直出。
type EquipBrief struct {
	Slot               int    // V1 §6.8 枚举 {1,2,3,4,5,6,7,99}
	UserCosmeticItemID uint64 // handler 层 strconv.FormatUint 字符串化
	CosmeticItemID     uint64 // handler 层 strconv.FormatUint 字符串化
	Name               string
	Rarity             int // V1 §6.9 枚举 {1,2,3,4}
	AssetURL           string
}

// PetBrief 是 V1 §5.1 data.pet 的 service 层映射。
//
// **Equips（Story 26.6 节点 9 加）**：value 切片，**空时为 []EquipBrief{}
// 非 nil** —— handler 序列化为 [] 非 null，对齐 V1 §5.1 行 408 "节点 2
// 强制 []" 语义在节点 9 延续（无装备时仍是 []）。节点 2 阶段（4.8）由
// handler 写死 []any{}；节点 9（本 story）改为 service 查真实数据填充。
type PetBrief struct {
	ID           uint64
	PetType      int
	Name         string
	CurrentState int
	Equips       []EquipBrief // Story 26.6：空时 []EquipBrief{} 非 nil
}

// StepAccountBrief 是 V1 §5.1 data.stepAccount 的 service 层映射。
type StepAccountBrief struct {
	TotalSteps     uint64
	AvailableSteps uint64
	ConsumedSteps  uint64
}

// ChestBrief 是 V1 §5.1 data.chest 的 service 层映射，**含动态判定后**字段。
//
// 关键：Status / RemainingSeconds 是 service 层 time.Now() 比较 unlockAt 后算出的值，
// **不是** DB 原值（DB user_chests.status 节点 2 阶段恒为 1，但客户端期望
// "unlock_at 已过 → status=2 unlockable"）。详见 chestStatusDynamic 注释。
type ChestBrief struct {
	ID               uint64
	Status           int       // 1=counting / 2=unlockable（动态判定）
	UnlockAt         time.Time // UTC（与 V1 §2.5 一致）
	OpenCostSteps   uint32
	RemainingSeconds int64 // max(0, int64(unlockAt - now))
}

// RoomBrief 是 V1 §5.1 data.room 的 service 层映射。
//
// **节点 4 阶段唯一字段**：CurrentRoomID（*uint64，nil = 用户不在任何房间）。
//
// **不**含 roomCode / room.id / memberCount 等：V1 §5.1 data.room 在节点 4 阶段
// 仅声明 `currentRoomId: string | null` 一个字段（详见 V1 §5.1 行 374）；房间详情
// 由 §10.2 GET /rooms/current + §10.3 GET /rooms/{id} 单独查询。本 struct 故意只含
// 1 个字段而非提前展开 —— 任何字段扩展（如 roomCode）由 future epic 决策（不在 V1 §5.1
// schema 钦定范围内 → 不属于本 story 11.10）。
//
// 字段类型 *uint64 而非 uint64：
//   - users.current_room_id 是 BIGINT UNSIGNED NULL（数据库设计 §5.1）
//   - 用户不在任何房间 → user.CurrentRoomID == nil → 本字段也是 nil
//   - 用户在房间 → user.CurrentRoomID == &roomID → 本字段也是 &roomID
//   - 与 mysql.User.CurrentRoomID 字段类型 1:1 对齐，handler 层做 nil → null /
//     非 nil → strconv.FormatUint 转字符串两路分支
type RoomBrief struct {
	CurrentRoomID *uint64 // nil = 用户不在任何房间（V1 §5.1 行 374 可空语义）
}

// homeServiceImpl 是 HomeService 的默认实装。
//
// 依赖（DI 注入；bootstrap.NewRouter 内 wire）：
//   - userRepo / petRepo / stepAccountRepo / chestRepo: 4.8 落地的 4 个 mysql repo
//   - userPetEquipRepo: Story 26.6 加 —— pet.equips 单 SQL JOIN 查询数据源
//
// **不**依赖：
//   - authBindingRepo（home 不查 binding 表）
//   - txMgr（GET /home 全是只读查询，无事务需求）
//   - signer（auth 中间件已校验 token，handler 已注入 userID）
//   - logger 注入：本 service log warning 用 slog.WarnContext(ctx, ...) 直调
//     —— 与 pet_service / cosmetic_service / dev_*_service 既有 service log
//     模式一致（项目无 service 层 logger 构造注入先例；Story 26.6 AC2(d)
//     "以既有模式为准，不臆造"，故 NewHomeService 仅 +1 个 repo 参数，
//     **不**加 logger 参数）。
type homeServiceImpl struct {
	userRepo         mysql.UserRepo
	petRepo          mysql.PetRepo
	stepAccountRepo  mysql.StepAccountRepo
	chestRepo        mysql.ChestRepo
	userPetEquipRepo mysql.UserPetEquipRepo
}

// NewHomeService 构造 HomeService。
//
// Story 26.6 加第 5 个入参 userPetEquipRepo（追加在 4.8 既有 4 repo 之后，
// 保持 user/pet/stepAccount/chest 顺序不变 —— 最小化对既有调用点 diff）。
func NewHomeService(
	userRepo mysql.UserRepo,
	petRepo mysql.PetRepo,
	stepAccountRepo mysql.StepAccountRepo,
	chestRepo mysql.ChestRepo,
	userPetEquipRepo mysql.UserPetEquipRepo,
) HomeService {
	return &homeServiceImpl{
		userRepo:         userRepo,
		petRepo:          petRepo,
		stepAccountRepo:  stepAccountRepo,
		chestRepo:        chestRepo,
		userPetEquipRepo: userPetEquipRepo,
	}
}

// LoadHome 实装：4 repo 串行 + chest 动态判定 + 拼装 DTO。
//
// **串行 vs 并发**：MVP 阶段用 4 个串行调用；不引入 errgroup 并发。理由：
//  1. 单 user 单查询，4 个简单 SELECT < 50ms（GORM 连接池复用）
//  2. errgroup 引入 cancel 传播 / error 收敛复杂度，节点 2 阶段不需要
//  3. 节点 36 性能 epic 才考虑并发；MVP 简单优于过早优化
//
// **chest 动态判定**：DB user_chests.status 节点 2 阶段恒为 1（counting），
// 不会被 update（开箱功能 Epic 20 才上线）。但 V1 §5.1 钦定客户端期望
// "unlock_at 已过 → status=2 unlockable"，所以本 service 必须基于 time.Now().UTC()
// 与 unlockAt 比较动态计算下发的 status / remainingSeconds（不写回 DB）。
//
// **不部分降级**：任一 repo 失败 → 包成 1009 整体返回，**不**返半截 HomeOutput。
// epics.md §Story 4.8 行 1136 钦定：避免主界面渲染异常引发更深的客户端错误链。
func (s *homeServiceImpl) LoadHome(ctx context.Context, userID uint64) (*HomeOutput, error) {
	// (1) user — 必有
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		// 即便是 ErrUserNotFound 也包成 1009：auth 中间件已校验 token，user 必须存在；
		// 不存在 → 数据脏（auth_binding 已删但 users 行残留 / 反之），让 client 看到 1009 + SilentRelogin 兜底。
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// (2) pet — 可空（唯一容许的 NotFound 分支）
	var petBrief *PetBrief
	pet, err := s.petRepo.FindDefaultByUserID(ctx, userID)
	if err != nil {
		if stderrors.Is(err, mysql.ErrPetNotFound) {
			// V1 §5.1 行 335 钦定 data.pet 可空：用户无默认 pet → petBrief = nil；
			// **不**包成 error，继续后续查询。
			petBrief = nil
		} else {
			// 其他 DB 错误（连接断 / 死锁等）→ 1009 中断
			return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}
	} else {
		petBrief = &PetBrief{
			ID:           pet.ID,
			PetType:      int(pet.PetType),
			Name:         pet.Name,
			CurrentState: int(pet.CurrentState),
			// Equips 在下方 (2b) 块按 pet.ID 查真实数据填充（节点 9 / Story 26.6）。
		}
	}

	// (2b) pet.equips — Story 26.6 节点 9 真实数据（替换 4.8 节点 2 阶段
	// handler 写死的 []any{}）。
	//
	//   - petBrief == nil（用户无默认 pet）→ 跳过装备查询（无 pet 无装备
	//     语义，也避免 petID 无值）；**不**调 ListEquipsForHome。
	//   - petBrief != nil → 单 SQL JOIN 查（用 (2) 块 pet 变量的 pet.ID）。
	//     query err → 整体 1009 不部分降级（与 stepAccount/chest 失败同
	//     契约，epics.md §Story 4.8 行 1136）；成功 → 转 []EquipBrief，
	//     空结果 → []EquipBrief{}（非 nil；AC2 happy: 没穿装备 → []）。
	//
	// **AC3 配置缺失 skip + log warning**：repo 用 INNER JOIN（cosmetic_items
	// 配置被 admin 物理删的 upe 行自然不进结果，自然 skip）+ 返 rawCount
	// （user_pet_equips 真实行数，O(1) COUNT 非 N+1）。rawCount > len(rows)
	// → slog.WarnContext 一条（含 userID/petID/差值），**不**报 error /
	// **不**中断（与 §8.2 态 C "missing-no-row 仍返回 + log" 同根因；log
	// 用 slog.WarnContext 直调，与 cosmetic_service / pet_service 既有 service
	// log 模式一致）。
	if petBrief != nil {
		rows, rawCount, equipErr := s.userPetEquipRepo.ListEquipsForHome(ctx, userID, pet.ID)
		if equipErr != nil {
			return nil, apperror.Wrap(equipErr, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}
		equips := make([]EquipBrief, 0, len(rows))
		for _, r := range rows {
			equips = append(equips, EquipBrief{
				Slot:               int(r.Slot),
				UserCosmeticItemID: r.UserCosmeticItemID,
				CosmeticItemID:     r.CosmeticItemID,
				Name:               r.Name,
				Rarity:             int(r.Rarity),
				AssetURL:           r.AssetURL,
			})
		}
		petBrief.Equips = equips // 空时为长度 0 的非 nil 切片（make(...,0,...)）

		if rawCount > int64(len(rows)) {
			// 某些 user_pet_equips 行对应的 cosmetic_items 配置被删（INNER
			// JOIN 过滤掉了）→ skip + warn（不报 error 不中断）。
			slog.WarnContext(ctx, "home pet.equips: some equipped cosmetic_items config missing; skipped via INNER JOIN",
				slog.Uint64("userId", userID),
				slog.Uint64("petId", pet.ID),
				slog.Int64("rawEquipCount", rawCount),
				slog.Int("joinedEquipCount", len(rows)),
				slog.Int64("skippedCount", rawCount-int64(len(rows))))
		}
	}

	// (3) stepAccount — 必有（登录初始化时已建；缺 → 1009）
	stepAccount, err := s.stepAccountRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// (4) chest — 必有（登录初始化时已建；缺 → 1009）
	chest, err := s.chestRepo.FindByUserID(ctx, userID)
	if err != nil {
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	// (5) chest 动态判定（Story 20.5 chest_service.GetCurrent 复用本逻辑）
	now := time.Now().UTC()
	chestStatus := chestStatusDynamic(chest.Status, chest.UnlockAt, now)
	remainingSeconds := computeRemainingSeconds(chest.UnlockAt, now)

	return &HomeOutput{
		User: UserBrief{
			ID:        user.ID,
			Nickname:  user.Nickname,
			AvatarURL: user.AvatarURL,
		},
		Pet: petBrief,
		StepAccount: StepAccountBrief{
			TotalSteps:     stepAccount.TotalSteps,
			AvailableSteps: stepAccount.AvailableSteps,
			ConsumedSteps:  stepAccount.ConsumedSteps,
		},
		Chest: ChestBrief{
			ID:               chest.ID,
			Status:           chestStatus,
			UnlockAt:         chest.UnlockAt.UTC(), // 强制 UTC 视图（防 GORM driver loc=Local 漂移）
			OpenCostSteps:    chest.OpenCostSteps,
			RemainingSeconds: remainingSeconds,
		},
		// Room: V1 §5.1 行 374 钦定 currentRoomId 类型 string | null。
		// **节点 4 阶段（Story 11.10）落地真实数据** —— 节点 2 阶段（4.8）的"强制 nil"
		// 红线在此解除：直接透传 user.CurrentRoomID（mysql.User.CurrentRoomID *uint64）。
		//   - 用户不在任何房间 → user.CurrentRoomID == nil → RoomBrief{CurrentRoomID: nil}
		//   - 用户在房间 X → user.CurrentRoomID == &X → RoomBrief{CurrentRoomID: &X}
		// **零额外 repo 调用**：current_room_id 是 users 表字段，已在 (1) FindByID 一次性
		// 返回；无需新建 RoomRepo / 不查 rooms 表 cross-check（epics.md §Story 11.10
		// edge case 钦定 service **不做** rooms.status 校验）。
		Room: RoomBrief{CurrentRoomID: user.CurrentRoomID},
	}, nil
}

// chestStatusDynamic 基于当前时间与 unlockAt 动态判定 chest 状态。
//
// 节点 2 阶段：DB 原值固定为 1 (counting)；超过 unlockAt → 下发 2 (unlockable)。
// Story 20.5 节点 7 chest_service.GetCurrent 会**复用**本函数（函数签名 + 行为契约**冻结**）。
//
// 参数：
//   - dbStatus: DB 中 user_chests.status 原值（节点 2 阶段始终 1）
//   - unlockAt: DB 中 user_chests.unlock_at（UTC）
//   - now: 当前时间（UTC，调用方传入便于单测注入）
//
// 返回：1 (counting) 或 2 (unlockable)
//
// **节点 2 阶段简化**：dbStatus 始终 1，所以函数等价于 "if now >= unlockAt then 2 else 1"。
// 但本函数保留 dbStatus 参数是**为节点 7 准备**：Story 20.5 chest_service 在用户开箱后
// 会把 status 写为其他值（如 3=opening 中间态，但该值在 V1 §5.1 钦定 /home 永远不返），
// 届时本函数会扩成 switch 语句穷举节点 7 阶段的 status 值集；**不能改函数签名**。
func chestStatusDynamic(dbStatus int8, unlockAt, now time.Time) int {
	// 节点 2 阶段：dbStatus 永远 1 (counting)；忽略 dbStatus 参数（保留为 future-proof）。
	_ = dbStatus
	if !now.Before(unlockAt) { // now >= unlockAt
		return 2 // unlockable
	}
	return 1 // counting
}

// computeRemainingSeconds 计算距离 unlockAt 的剩余秒数。
//
// 返回值：
//   - 已过 unlockAt（diff <= 0）：返 0（**不**返负数 —— V1 §5.1 钦定
//     "> 0 表示 counting，≤ 0 表示已可开启"，0 是边界值；客户端按 ≤0 判 unlockable）
//   - 未过 unlockAt：剩余整秒数（向下取整）
//
// 用 int64 而非 int，避免 32-bit 平台 overflow（unlockAt 在远未来不太可能但保险）。
//
// Story 20.5 chest_service.GetCurrent 复用本函数；函数签名冻结。
func computeRemainingSeconds(unlockAt, now time.Time) int64 {
	diff := unlockAt.Sub(now)
	if diff <= 0 {
		return 0
	}
	return int64(diff.Seconds())
}
