package service

import (
	"context"
	stderrors "errors"
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
type HomeOutput struct {
	User        UserBrief
	Pet         *PetBrief // 可空（nil = 用户无默认 pet）
	StepAccount StepAccountBrief
	Chest       ChestBrief
}

// UserBrief 是 V1 §5.1 data.user 的 service 层映射。
type UserBrief struct {
	ID        uint64
	Nickname  string
	AvatarURL string
}

// PetBrief 是 V1 §5.1 data.pet 的 service 层映射（不含 equips —— 节点 2 阶段 handler
// 直接构造 `[]any{}`，避免 service 层关心展示用空切片）。
type PetBrief struct {
	ID           uint64
	PetType      int
	Name         string
	CurrentState int
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

// homeServiceImpl 是 HomeService 的默认实装。
//
// 依赖（DI 注入；bootstrap.NewRouter 内 wire）：
//   - userRepo / petRepo / stepAccountRepo / chestRepo: 4 个 mysql repo
//
// **不**依赖：
//   - authBindingRepo（home 不查 binding 表）
//   - txMgr（GET /home 全是只读查询，无事务需求）
//   - signer（auth 中间件已校验 token，handler 已注入 userID）
type homeServiceImpl struct {
	userRepo        mysql.UserRepo
	petRepo         mysql.PetRepo
	stepAccountRepo mysql.StepAccountRepo
	chestRepo       mysql.ChestRepo
}

// NewHomeService 构造 HomeService。
func NewHomeService(
	userRepo mysql.UserRepo,
	petRepo mysql.PetRepo,
	stepAccountRepo mysql.StepAccountRepo,
	chestRepo mysql.ChestRepo,
) HomeService {
	return &homeServiceImpl{
		userRepo:        userRepo,
		petRepo:         petRepo,
		stepAccountRepo: stepAccountRepo,
		chestRepo:       chestRepo,
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
