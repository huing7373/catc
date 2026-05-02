package service

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"math"
	"time"

	"github.com/huing/cat/server/internal/infra/config"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
)

// 业务常量（V1 §6.1.4 + GAP K 钦定，**默认值**进入冻结契约 —— 仅 dev/test 环境可通过 config 覆盖）。
//
// 在 service 包定义而非 config 包定义"默认值常量"的理由：默认值是**业务规则**
// （V1 §6.1.4 + GAP K 文档侧锚定的契约一部分），而非"配置可调参数"。config.StepsConfig
// 是**dev / test 覆盖通道**——loader 读 YAML 显式值则走 YAML，否则走这里的常量默认值。
const (
	// defaultStepsSingleSyncCap 是单次 sync 的 delta 上限（防作弊截断阈值）。
	//
	// 默认 5000：epics.md §Story 7.3 + V1 §6.1.4 钦定。**prod 必须用默认值**
	// （契约一部分，跨端一致）；dev/test 通过 YAML `steps.single_sync_cap` 覆盖。
	defaultStepsSingleSyncCap = 5000

	// defaultStepsDailyCap 是当日累计 accepted_delta_steps 封顶阈值。
	//
	// 默认 50000：epics.md §Story 7.3 + V1 §6.1.4 钦定。同上 prod / dev 约束。
	defaultStepsDailyCap = 50000
)

// StepService 是 steps handler 的依赖 interface。
type StepService interface {
	// SyncSteps 处理 POST /api/v1/steps/sync 业务。
	//
	// 流程（数据库设计 §8.2 + V1 §6.1.4）：
	//  1. 在事务内：FindLatestByUserAndDate(userID, syncDate) → lastClientTotalSteps
	//  2. 计算 delta：无 last → delta = clientTotalSteps；有 last → delta = max(0, clientTotalSteps - lastClientTotalSteps)
	//  3. 防作弊截断：if delta > singleSyncCap → delta = singleSyncCap + log warning
	//  4. 防作弊封顶：if SumAccepted(userID, syncDate) + delta > dailyCap → delta = 0 + log warning + ErrStepSyncInvalid
	//  5. FindByUserID(userID) 取 step_account 当前 version
	//  6. UpdateBalance(userID, delta, version) → 乐观锁 +delta
	//  7. Create(StepSyncLog{...}) 写日志（含 source=1 healthkit / accepted_delta_steps=delta）
	//  8. 返回 SyncStepsOutput{AcceptedDeltaSteps: delta, StepAccount: 三档值}
	//
	// 错误约定（ADR-0006 三层映射）：
	//   - 当日封顶触发 → apperror.New(ErrStepSyncInvalid, "...")（业务码 3001）
	//   - repo 任一失败（FindLatest 非 NotFound / FindByUserID / UpdateBalance / Create）→
	//     apperror.Wrap(err, ErrServiceBusy, ...)（1009）
	//   - 乐观锁失败 ErrStepAccountVersionMismatch → 包成 1009
	SyncSteps(ctx context.Context, in SyncStepsInput) (*SyncStepsOutput, error)
}

// SyncStepsInput 是 service 层 DTO（**不是** wire DTO；handler 转换）。
type SyncStepsInput struct {
	UserID           uint64
	SyncDate         time.Time // handler 已 parse YYYY-MM-DD → time.Time（midnight time.Local，与 DSN loc=Local 锁同步；详见 steps_handler.go ParseInLocation 注释）
	ClientTotalSteps uint64
	MotionState      int8
	ClientTimestamp  uint64 // ms
}

// SyncStepsOutput 是 service 层 DTO；handler 翻译成 V1 §6.1.5 wire DTO。
type SyncStepsOutput struct {
	AcceptedDeltaSteps int32
	StepAccount        StepAccountBrief // 复用 home_service.StepAccountBrief（已定义）
}

// stepServiceImpl 是 StepService 的默认实装。
type stepServiceImpl struct {
	txMgr           tx.Manager
	stepAccountRepo mysql.StepAccountRepo
	stepSyncLogRepo mysql.StepSyncLogRepo

	// 防作弊阈值（启动期从 config 读取；service 层运行期不变）。
	//
	// **类型选 int64**（不是 int32）：与 config.StepsConfig 字段类型一致，避免
	// "构造期 int64→int32 narrowing 把超大配置 wrap 成负数"那种静默灾难
	// （详见 NewStepService 的 fail-fast 注释 + lessons 2026-05-02 narrowing）。
	// 实际写入时 delta cast 回 int32（accepted_delta_steps 是 INT signed），cap
	// 已经在 NewStepService 验证过 ≤ math.MaxInt32，cast 安全。
	singleSyncCap int64
	dailyCap      int64
}

// NewStepService 构造 StepService。
//
// cfg 是配置侧的 StepsConfig（dev / test 可覆盖默认值；prod 必须用 0 让兜底接管）；
// 兜底逻辑：cfg.SingleSyncCap == 0 → 用 defaultStepsSingleSyncCap；DailyCap 同理。
//
// **fail-fast 范围校验**（Story 7.3 review r1 [P2]）：
//   - cfg.SingleSyncCap > math.MaxInt32 → panic（返回值要 cast 回 int32 写
//     accepted_delta_steps；越界会 wrap 成负数静默扣余额，必须早 fail）
//   - cfg.SingleSyncCap < 0 → panic（YAML 配错；负 cap 没有业务语义）
//   - cfg.DailyCap < 0 → panic（同上）
//   - 0 视为"用默认值"（与已有 zero-value 兜底语义一致；非错误）
//
// 反例（旧版无范围校验）：YAML `single_sync_cap: 5000000000` → int64→int32 cast
// wrap 为负 → service 层 `delta > cap` 永远命中 → delta 被截断为负数 →
// UpdateBalance / accepted_delta_steps 写入负值 → **余额被减少而非封顶**。
//
// **不**在本 service 内做"prod 环境检测拒绝覆盖"—— config 文档侧已声明（V1 §1）。
func NewStepService(
	txMgr tx.Manager,
	stepAccountRepo mysql.StepAccountRepo,
	stepSyncLogRepo mysql.StepSyncLogRepo,
	cfg config.StepsConfig,
) StepService {
	if cfg.SingleSyncCap < 0 {
		panic(fmt.Sprintf("step service: single_sync_cap=%d 不能为负数", cfg.SingleSyncCap))
	}
	if cfg.SingleSyncCap > math.MaxInt32 {
		panic(fmt.Sprintf("step service: single_sync_cap=%d 超过 int32 上限 %d（accepted_delta_steps 列是 INT signed）", cfg.SingleSyncCap, math.MaxInt32))
	}
	if cfg.DailyCap < 0 {
		panic(fmt.Sprintf("step service: daily_cap=%d 不能为负数", cfg.DailyCap))
	}
	singleSyncCap := int64(defaultStepsSingleSyncCap)
	if cfg.SingleSyncCap > 0 {
		singleSyncCap = cfg.SingleSyncCap
	}
	dailyCap := int64(defaultStepsDailyCap)
	if cfg.DailyCap > 0 {
		dailyCap = cfg.DailyCap
	}
	return &stepServiceImpl{
		txMgr:           txMgr,
		stepAccountRepo: stepAccountRepo,
		stepSyncLogRepo: stepSyncLogRepo,
		singleSyncCap:   singleSyncCap,
		dailyCap:        dailyCap,
	}
}

// SyncSteps 实装。
func (s *stepServiceImpl) SyncSteps(ctx context.Context, in SyncStepsInput) (*SyncStepsOutput, error) {
	var (
		out         SyncStepsOutput
		capExceeded bool // 当日封顶标记：在事务**外**翻译为业务错（3001）
	)

	// **关键**：当日封顶必须**先 commit 事务**（sync_log 行 + version + 1 必须落库做审计 +
	// 防重），再翻译为业务错（3001）。若在事务 fn 内返 *AppError，GORM 默认会 rollback，
	// 导致审计行 / version 递增丢失，违反 V1 §6.1.4 + 数据库设计 §8.2 钦定语义。
	// 因此用闭包外的 capExceeded flag：fn 内只在真 DB 错误时 return error；封顶场景
	// 让 fn 返 nil 让事务 commit，再在事务外用 capExceeded 判定后产 *AppError。
	err := s.txMgr.WithTx(ctx, func(txCtx context.Context) error {
		// (1) 查最近 sync_log（同 user 同 sync_date）
		var lastClientTotalSteps uint64
		latest, err := s.stepSyncLogRepo.FindLatestByUserAndDate(txCtx, in.UserID, in.SyncDate)
		if err != nil && !stderrors.Is(err, mysql.ErrStepSyncLogNotFound) {
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}
		if latest != nil {
			lastClientTotalSteps = latest.ClientTotalSteps
		}

		// (2) 计算 delta —— 中间用 int64 避免 uint64 减法 overflow
		var rawDelta int64
		if latest == nil {
			// 首次同步：delta = clientTotalSteps（防 INT 上限保护：理论日上限远低于）
			rawDelta = int64(in.ClientTotalSteps)
		} else {
			if in.ClientTotalSteps > lastClientTotalSteps {
				rawDelta = int64(in.ClientTotalSteps - lastClientTotalSteps)
			} else {
				rawDelta = 0 // 倒退 / 重复
			}
		}

		// (3) 防作弊单次截断
		delta := rawDelta
		if delta > s.singleSyncCap {
			slog.WarnContext(txCtx, "step sync single cap truncated",
				"user_id", in.UserID, "sync_date", in.SyncDate.Format("2006-01-02"),
				"raw_delta", rawDelta, "truncated_to", s.singleSyncCap)
			delta = s.singleSyncCap
		}

		// (4) 防作弊当日封顶（**入账后越界判断**：prevAccepted + curDelta > dailyCap → 拒绝）
		prevAccepted, err := s.stepSyncLogRepo.SumAcceptedDeltaByUserAndDate(txCtx, in.UserID, in.SyncDate)
		if err != nil {
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}
		// 重置为本次事务的判定结果（防上次调用残留）
		capExceeded = false
		if prevAccepted+delta > s.dailyCap {
			slog.WarnContext(txCtx, "step sync daily cap exceeded",
				"user_id", in.UserID, "sync_date", in.SyncDate.Format("2006-01-02"),
				"prev_accepted", prevAccepted, "cur_delta", delta, "daily_cap", s.dailyCap)
			delta = 0
			capExceeded = true
		}

		// (5) 取 step_account 当前 version
		account, err := s.stepAccountRepo.FindByUserID(txCtx, in.UserID)
		if err != nil {
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}

		// (6) UpdateBalance —— 即便 delta=0 也走，保持事务边界一致；version + 1 仍递增。
		if err := s.stepAccountRepo.UpdateBalance(txCtx, in.UserID, int32(delta), account.Version); err != nil {
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}

		// (7) 写 sync_log（**含**倒退 / 重复 / 截断 / 封顶场景；append-only 审计纪律）
		log := &mysql.StepSyncLog{
			UserID:             in.UserID,
			SyncDate:           in.SyncDate,
			ClientTotalSteps:   in.ClientTotalSteps,
			AcceptedDeltaSteps: int32(delta),
			MotionState:        in.MotionState,
			Source:             1, // healthkit（V1 §6.1.4 + 数据库设计 §6.6）；dev grant 走 source=2 在 7.5
			ClientTs:           in.ClientTimestamp,
		}
		if err := s.stepSyncLogRepo.Create(txCtx, log); err != nil {
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}

		// (8) 拼装 output —— 用更新**之后**的余额（account.X + delta）
		out = SyncStepsOutput{
			AcceptedDeltaSteps: int32(delta),
			StepAccount: StepAccountBrief{
				TotalSteps:     account.TotalSteps + uint64(delta),
				AvailableSteps: account.AvailableSteps + uint64(delta),
				ConsumedSteps:  account.ConsumedSteps, // sync 接口不改 consumed
			},
		}

		// fn 返 nil → 事务 commit（即便 capExceeded 也要让 sync_log + version+1 落库）。
		return nil
	})
	if err != nil {
		// 事务 fn 内返非 nil error → GORM rollback → 透传给调用方
		return nil, err
	}

	// 事务 commit 之后再判定 capExceeded：业务错 3001 不应 rollback 事务（审计 / 防重）。
	if capExceeded {
		return nil, apperror.New(apperror.ErrStepSyncInvalid, apperror.DefaultMessages[apperror.ErrStepSyncInvalid])
	}
	return &out, nil
}
