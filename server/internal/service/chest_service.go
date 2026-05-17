package service

import (
	"context"
	stderrors "errors"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/random"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
)

// ChestService 是 chest handler 的依赖 interface（便于 handler 单测 mock）。
//
// **接口而非具体类型**：handler 单测注入 stub struct，与 7.4 / 17.4 / 11.6 同模式。
//
// Story 20.5 落地 GetCurrent；Story 20.6 追加 OpenChest（POST /chest/open）。
type ChestService interface {
	// GetCurrent 处理 GET /api/v1/chest/current 业务（Story 20.5）。
	//
	// 流程（V1 §7.1.4 + 数据库设计 §5.6）：
	//  1. chestRepo.FindByUserID(ctx, userID) → user_chest（必有；登录初始化已建）
	//  2. service 层动态判定 chest.status / remainingSeconds（基于 time.Now().UTC() vs unlockAt）
	//     **复用** home_service.go 已落地的 chestStatusDynamic + computeRemainingSeconds 两个 helper
	//     （home_service.go 顶部注释钦定 "Story 20.5 chest_service.GetCurrent 复用"）
	//  3. 拼装 ChestBrief 返回（**复用** home_service.go 已落地的 ChestBrief struct）
	//
	// 错误约定（ADR-0006 三层映射）：
	//   - mysql.ErrChestNotFound（理论不该发生 —— Story 4.6 五表事务必建一行）→
	//     **包成 ErrChestNotFound (4001)**（V1 §7.1.6 钦定 4001，**不**包成 1009 / 1003 ——
	//     V1 §7.1 行 904 钦定 "user 在 user_chests 表中无任何行" 是 4001 而非 1009）
	//   - 其他 DB 异常（连接断 / SQL 错 / 死锁等）→ 包成 ErrServiceBusy (1009)
	//
	// **不**接事务（纯读，与 home_service.LoadHome / step_service.GetAccount 同模式）。
	// **不**更新 DB（动态判定，节省写；真状态变更在开箱事务 Story 20.6）。
	GetCurrent(ctx context.Context, userID uint64) (*ChestBrief, error)

	// OpenChest 处理 POST /api/v1/chest/open 业务（Story 20.6）。
	//
	// 流程（V1 §7.2.5 8 步事务 + handler 内层 rate_limit 不在本 service 做）：
	//   1. 入参校验（idempotencyKey length 兜底；非业务必走，handler 已校验）
	//   2. 步骤 3: 幂等命中预检（autocommit SELECT idempotency 行）→ 命中 success → 直接返 cached
	//   3. 步骤 5: 业务事务（txMgr.WithTx fn）：5a 预声明 → 5b 短路 / 5c-l 全流程
	//
	// 步骤 4 (rate_limit) 由 handler 层做；本 service 不感知。
	//
	// 详见 chest_open_service.go 顶部注释 + V1 §7.2 r1~r15 决策段。
	OpenChest(ctx context.Context, in OpenChestInput) (*OpenChestOutput, error)
}

// chestServiceImpl 是 ChestService 的默认实装。
//
// 依赖（DI 注入；bootstrap.NewRouter 内 wire）：
//   - chestRepo: mysql.ChestRepo（4.6 已实装）
//   - Story 20.6 起追加：txMgr / idempotencyRepo / stepAccountRepo / cosmeticItemRepo /
//     chestOpenLogRepo / weightedPicker / nowFn（OpenChest 业务事务依赖）
//   - Story 23.5 起追加：userCosmeticItemRepo（OpenChest 入仓依赖 —— 开箱事务
//     步骤 5g.5 创建 user_cosmetic_items 实例；GetCurrent 不消费）
type chestServiceImpl struct {
	chestRepo mysql.ChestRepo

	// Story 20.6 引入字段（OpenChest 业务事务依赖）；Story 20.5 GetCurrent 不消费。
	txMgr            tx.Manager
	idempotencyRepo  mysql.IdempotencyRepo
	stepAccountRepo  mysql.StepAccountRepo
	cosmeticItemRepo mysql.CosmeticItemRepo
	chestOpenLogRepo mysql.ChestOpenLogRepo
	weightedPicker   random.WeightedPicker
	nowFn            func() time.Time

	// Story 23.5 引入：开箱事务补入仓写 user_cosmetic_items 实例依赖
	// （runOpenChestTx 步骤 5g.5；OpenChest 消费，GetCurrent 不消费）。
	userCosmeticItemRepo mysql.UserCosmeticItemRepo
}

// NewChestService 构造 ChestService。Story 20.6 扩签名为 7 参数 + nowFn 默认 UTC；
// Story 23.5 扩签名为 8 参数（节点 8 入仓）—— 新增第 8 参 userCosmeticItemRepo
// （开箱事务步骤 5g.5 创建 user_cosmetic_items 实例）。
//
// **签名变更（向后不兼容）**：Story 20.5 仅 1 参数（chestRepo）；Story 20.6 起需要
// 注入业务事务全部依赖（txMgr / idempotencyRepo / stepAccountRepo /
// cosmeticItemRepo / chestOpenLogRepo / weightedPicker）；Story 23.5 追加
// userCosmeticItemRepo（追加在 weightedPicker 之后）。router.go + 全部 chest_open
// 测试 fixture 同步扩参（同 Story 23.4 NewCosmeticService 扩签名模式）。
//
// nowFn 内部默认 `func() time.Time { return time.Now().UTC() }`；单测可注入 mock
// 时钟覆盖 chestServiceImpl.nowFn 字段。
func NewChestService(
	chestRepo mysql.ChestRepo,
	txMgr tx.Manager,
	idempotencyRepo mysql.IdempotencyRepo,
	stepAccountRepo mysql.StepAccountRepo,
	cosmeticItemRepo mysql.CosmeticItemRepo,
	chestOpenLogRepo mysql.ChestOpenLogRepo,
	weightedPicker random.WeightedPicker,
	userCosmeticItemRepo mysql.UserCosmeticItemRepo,
) ChestService {
	return &chestServiceImpl{
		chestRepo:            chestRepo,
		txMgr:                txMgr,
		idempotencyRepo:      idempotencyRepo,
		stepAccountRepo:      stepAccountRepo,
		cosmeticItemRepo:     cosmeticItemRepo,
		chestOpenLogRepo:     chestOpenLogRepo,
		weightedPicker:       weightedPicker,
		userCosmeticItemRepo: userCosmeticItemRepo,
		nowFn:                func() time.Time { return time.Now().UTC() },
	}
}

// GetCurrent 实装：单表查询 user_chests → 动态判定 → ChestBrief。
//
// **关键**：
//   - chest 必有（登录初始化已建）；查不到 → V1 §7.1.6 钦定 4001
//   - chestStatusDynamic 在 home_service.go 顶部注释钦定 "Story 20.5 复用，签名 + 行为冻结"
//   - computeRemainingSeconds 同上
//
// **time.Now().UTC()**：必须 UTC（与 V1 §2.5 ISO 8601 UTC 钦定一致；
// 与 chest.UnlockAt 字段 UTC 语义对齐 —— chest_repo.go 顶部注释钦定）。
func (s *chestServiceImpl) GetCurrent(ctx context.Context, userID uint64) (*ChestBrief, error) {
	chest, err := s.chestRepo.FindByUserID(ctx, userID)
	if err != nil {
		// 理论不该发生（Story 4.6 五表事务必建一行）→ 但 V1 §7.1.6 钦定 4001，按契约下发。
		if stderrors.Is(err, mysql.ErrChestNotFound) {
			return nil, apperror.Wrap(err, apperror.ErrChestNotFound, apperror.DefaultMessages[apperror.ErrChestNotFound])
		}
		// 其他 DB 异常（含 driver / 网络 / 慢查询）→ 1009
		return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
	}

	now := time.Now().UTC()
	chestStatus := chestStatusDynamic(chest.Status, chest.UnlockAt, now)
	remainingSeconds := computeRemainingSeconds(chest.UnlockAt, now)

	return &ChestBrief{
		ID:               chest.ID,
		Status:           chestStatus,
		UnlockAt:         chest.UnlockAt.UTC(), // 强制 UTC 视图（防 GORM driver loc=Local 漂移；与 home_service.LoadHome 同模式）
		OpenCostSteps:    chest.OpenCostSteps,
		RemainingSeconds: remainingSeconds,
	}, nil
}
