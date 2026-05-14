package service

import (
	"context"
	stderrors "errors"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
)

// ChestService 是 chest handler 的依赖 interface（便于 handler 单测 mock）。
//
// **接口而非具体类型**：handler 单测注入 stub struct，与 7.4 / 17.4 / 11.6 同模式。
//
// 节点 7 阶段仅 GetCurrent；future Story 20.6 落地 POST /chest/open 时**另起独立 interface**
// （或在本 interface 末尾追加 OpenChest 方法 + 复用 chestServiceImpl）—— 本 story 不预实装。
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
}

// chestServiceImpl 是 ChestService 的默认实装。
//
// 依赖（DI 注入；bootstrap.NewRouter 内 wire）：
//   - chestRepo: mysql.ChestRepo（4.6 已实装）
//
// **不**依赖：
//   - txMgr（GET /chest/current 全只读，无事务）
//   - userRepo / petRepo / stepAccountRepo（chest 单表查询不聚合其他实体）
//   - signer（auth 中间件已校验 token，handler 已注入 userID）
type chestServiceImpl struct {
	chestRepo mysql.ChestRepo
}

// NewChestService 构造 ChestService。
func NewChestService(chestRepo mysql.ChestRepo) ChestService {
	return &chestServiceImpl{chestRepo: chestRepo}
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
