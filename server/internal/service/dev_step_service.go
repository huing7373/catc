package service

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"time"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
)

// DevStepService 是 /dev/grant-steps 端点的依赖 interface（Story 7.5）。
//
// **dev 端点的产品语义**：绕过 7.3 SyncSteps 的所有约束（单次截断 / 当日封顶 /
// SUM 兜底 / 倒退削成 0），直接给指定用户 +N 步入账，并写一条 source=2 admin_grant
// 审计行。仅供 demo / 自动化 e2e / 手工调试，**不**走 prod。
//
// **不**复用 step_service.StepService.SyncSteps：
//   - SyncSteps 含 5+ 约束分支（rawDelta = max(0, ...)；single_sync_cap 5000 截断；
//     daily_cap 50000 封顶；SUM 兜底乱序到达；source=1 healthkit 写死）—— 全不适用 dev
//   - 强行复用要新增 "is_dev" flag 把 5 个分支 short-circuit，反模式 → 独立 service 更清晰
//
// 错误约定（ADR-0006 三层映射）：
//   - mysql.ErrUserNotFound（FindByID 查不到）→ 包成 ErrResourceNotFound (1003)
//   - mysql.ErrStepAccountNotFound（FindByUserID 查不到，理论不该发生）→ 包成 ErrResourceNotFound (1003)
//   - 其他 DB 异常（连接断 / SQL 错 / 死锁等）→ 包成 ErrServiceBusy (1009)
//   - **service 层不做 steps < 0 校验**（handler 层在调用本 service 前必须已校验 1002）—— 但 service 仍
//     防御性 panic（"dev step service: steps must be >= 0"）防 handler 漏校验
type DevStepService interface {
	// GrantSteps 在事务内给指定 userID 直接 +steps 入账：
	//  1. userRepo.FindByID(ctx, userID) → 验用户存在（理论 dev 测试不该传不存在的 ID）
	//  2. stepAccountRepo.FindByUserID(ctx, userID) → 取当前 version
	//  3. stepAccountRepo.UpdateBalance(ctx, userID, +steps, version) → total / available 各 +steps
	//  4. stepSyncLogRepo.Create(ctx, &StepSyncLog{Source: 2, AcceptedDeltaSteps: steps, ...}) → 审计行
	//
	// **steps 类型 int32**：与 sync_log.accepted_delta_steps 同（INT signed），handler 校验 >= 0。
	//
	// **SyncDate 用 server today UTC**（dev 端点不接受 client 提供 syncDate，因为 dev 场景
	// 没有 client 时区概念；server 直接用 UTC today 写 sync_log 满足 NOT NULL 约束）。
	//
	// **MotionState 写死 1**（stationary_or_unknown，§6.5；dev grant 没有运动状态语义，
	// 用 1 中性值满足 NOT NULL）。
	//
	// **ClientTs 写 0**（dev grant 没有"客户端调用时刻"语义；BIGINT UNSIGNED 0 合法值）。
	GrantSteps(ctx context.Context, userID uint64, steps int32) error
}

// devStepServiceImpl 是 DevStepService 的默认实装。
type devStepServiceImpl struct {
	txMgr           tx.Manager
	userRepo        mysql.UserRepo
	stepAccountRepo mysql.StepAccountRepo
	stepSyncLogRepo mysql.StepSyncLogRepo
}

// NewDevStepService 构造 DevStepService。
//
// 依赖：
//   - txMgr：事务边界控制（FindByID + FindByUserID + UpdateBalance + Create 必须原子）
//   - userRepo：FindByID 验用户存在
//   - stepAccountRepo：FindByUserID 取 version + UpdateBalance +steps
//   - stepSyncLogRepo：Create 审计行（source=2）
//
// 与 NewStepService (7.3) 不同：**不**接 config.StepsConfig（dev grant 不受 cap / 阈值约束）；
// **不**接 envName（dev grant 已经被 build tag / env var 双闸门防 prod，不再做 envName 检查）。
func NewDevStepService(
	txMgr tx.Manager,
	userRepo mysql.UserRepo,
	stepAccountRepo mysql.StepAccountRepo,
	stepSyncLogRepo mysql.StepSyncLogRepo,
) DevStepService {
	return &devStepServiceImpl{
		txMgr:           txMgr,
		userRepo:        userRepo,
		stepAccountRepo: stepAccountRepo,
		stepSyncLogRepo: stepSyncLogRepo,
	}
}

// devGrantSyncDateLayout 是 sync_log.sync_date 写入格式（与 7.3 SyncSteps 同；YYYY-MM-DD）。
const devGrantSyncDateLayout = "2006-01-02"

// GrantSteps 实装：事务内 FindByID + FindByUserID + UpdateBalance + Create sync_log。
//
// **steps < 0 防御性 panic**（handler 层在 1002 校验前防御）：
//   - prod 二进制不可达本 service（双闸门），故 panic 不会泄漏到 prod
//   - 单测用本路径覆盖"假设 handler 漏校验时 service 仍能拦"
func (s *devStepServiceImpl) GrantSteps(ctx context.Context, userID uint64, steps int32) error {
	if steps < 0 {
		// 防御性：handler 层应已 1002 拦截；走到这里说明 handler 漏校验
		panic(fmt.Sprintf("dev step service: steps must be >= 0; got %d", steps))
	}

	// SyncDate 用 server today UTC（dev 端点不接受 client 提供 syncDate；详见 interface doc）
	syncDate := time.Now().UTC().Format(devGrantSyncDateLayout)

	return s.txMgr.WithTx(ctx, func(txCtx context.Context) error {
		// (1) 验用户存在
		if _, err := s.userRepo.FindByID(txCtx, userID); err != nil {
			if stderrors.Is(err, mysql.ErrUserNotFound) {
				return apperror.Wrap(err, apperror.ErrResourceNotFound, apperror.DefaultMessages[apperror.ErrResourceNotFound])
			}
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}

		// (2) 取 step_account 当前 version
		account, err := s.stepAccountRepo.FindByUserID(txCtx, userID)
		if err != nil {
			if stderrors.Is(err, mysql.ErrStepAccountNotFound) {
				// 理论不该发生（4.6 firstTimeLogin 已建）→ 但 dev 端点也按 1003 钦定
				return apperror.Wrap(err, apperror.ErrResourceNotFound, apperror.DefaultMessages[apperror.ErrResourceNotFound])
			}
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}

		// (3) UpdateBalance(+steps, version) —— 即便 steps=0 也走，保持事务边界一致；version 仍 +1
		if err := s.stepAccountRepo.UpdateBalance(txCtx, userID, steps, account.Version); err != nil {
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}

		// (4) 写 sync_log 审计行（source=2 admin_grant；§6.6 钦定）
		log := &mysql.StepSyncLog{
			UserID:             userID,
			SyncDate:           syncDate,
			ClientTotalSteps:   0, // dev 没有"客户端累计"语义
			AcceptedDeltaSteps: steps,
			MotionState:        1, // stationary_or_unknown（§6.5）—— 中性值满足 NOT NULL
			Source:             2, // admin_grant（§6.6）
			ClientTs:           0, // dev 没有"客户端调用时刻"语义
		}
		if err := s.stepSyncLogRepo.Create(txCtx, log); err != nil {
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}

		slog.WarnContext(txCtx, "dev grant steps applied",
			"user_id", userID, "steps", steps, "sync_date", syncDate,
		)
		return nil
	})
}
