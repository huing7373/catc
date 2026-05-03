package service

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"math"

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
	// 流程（数据库设计 §8.2 + V1 §6.1.4 + Story 7.3 review r3/r5 [P1] 三层防御）：
	//  1. 在事务内：FindLatestByUserAndDate(userID, syncDate) → lastClientTotalSteps（用于"基线跟得上 reset"）
	//  2. **MAX-Reported clamp**（Story 7.3 review r5 [P1] 修复"截断 + 乱序"反例）：
	//     maxReported = MAX(client_total_steps) of today；
	//     - if clientTotalSteps <= maxReported → rawDelta = 0（旧 / 重复 sync，已报告过）
	//     - else → rawDelta = clientTotalSteps - maxReported（**不**用 latest.ClientTotalSteps 当基线，
	//       因为 latest 在乱序场景下可能比 maxReported 小，让 rawDelta 算多；用 max 才能 + SUM 兜底叠加防御）
	//  3. **SUM 兜底**（保留 r3 的第二层防御 —— 即使 maxReported 当前为 0，SUM 也能拦多次"低高低高"乱序）：
	//     SumAccepted(today) + rawDelta > clientTotalSteps → rawDelta = max(0, clientTotalSteps - SumAccepted)
	//  4. 防作弊单次截断：if rawDelta > singleSyncCap → delta = singleSyncCap + log warning
	//  5. 防作弊当日封顶：if SumAccepted(userID, syncDate) + delta > dailyCap → delta = 0 + log warning + ErrStepSyncInvalid
	//  6. FindByUserID(userID) 取 step_account 当前 version
	//  7. UpdateBalance(userID, delta, version) → 乐观锁 +delta
	//  8. Create(StepSyncLog{...}) 写日志（含 source=1 healthkit / accepted_delta_steps=delta）
	//  9. 返回 SyncStepsOutput{AcceptedDeltaSteps: delta, StepAccount: 三档值}
	//
	// 错误约定（ADR-0006 三层映射）：
	//   - 当日封顶触发 → apperror.New(ErrStepSyncInvalid, "...")（业务码 3001）
	//   - repo 任一失败（FindLatest 非 NotFound / FindByUserID / UpdateBalance / Create）→
	//     apperror.Wrap(err, ErrServiceBusy, ...)（1009）
	//   - 乐观锁失败 ErrStepAccountVersionMismatch → 包成 1009
	SyncSteps(ctx context.Context, in SyncStepsInput) (*SyncStepsOutput, error)
}

// SyncStepsInput 是 service 层 DTO（**不是** wire DTO；handler 转换）。
//
// **SyncDate 用 string 而非 time.Time**（Story 7.3 review r2 [P2]）：
// handler 已校验过 len==10 + 合法 YYYY-MM-DD；service 直接透传到 repo / mysql
// driver 走"VARCHAR → DATE"隐式转换，**完全无时区耦合，不依赖 DSN loc 配置**。
// 详见 mysql.StepSyncLog 与 steps_handler 的注释 + lessons 2026-05-02 string-transit。
type SyncStepsInput struct {
	UserID           uint64
	SyncDate         string // YYYY-MM-DD；handler 已校验
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
		// (1) 查最近 sync_log（同 user 同 sync_date）—— 仅作"是否首次"判断；基线**不**从这里取（见 (2)）
		latest, err := s.stepSyncLogRepo.FindLatestByUserAndDate(txCtx, in.UserID, in.SyncDate)
		if err != nil && !stderrors.Is(err, mysql.ErrStepSyncLogNotFound) {
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}

		// (2) **MAX-Reported clamp**（Story 7.3 review r5 [P1] 修复"截断 + 乱序"反例）
		//
		// **背景**（r1 → r2 → r3 → r5 决策史；详见 step_sync_log_repo.go interface doc）：
		//   - r1 基线 id DESC（最近 INSERT）—— 乱序到达让旧 sync 成新基线 → 重复入账
		//   - r2 基线 max(client_total_steps)—— 乱序 OK 但 HealthKit reset 永久卡死
		//   - r3 基线 id DESC + service 层 SUM 兜底 —— 解决 r1/r2 二选一，但**截断 + 乱序**
		//     组合反例下仍能多入账（见 r5 反例）
		//   - r5 综合方案：基线**改用 maxReported**（不是 latest.ClientTotalSteps），
		//     因为 latest 在乱序场景下可能 < maxReported，让 rawDelta 算多。
		//     SUM 兜底保留作为**第二层防御**（应对未来更复杂的乱序模式）
		//
		// **r5 反例验证**：
		//   - sync A: clientTotal=10000，maxReported=0 → rawDelta=10000 → 截断 5000 → accepted=5000；
		//     DB 写 client_total=10000 (即使 accepted 被截，client_total_steps 落库就是用户报的 10000)
		//   - sync B: clientTotal=8000 (delayed)，maxReported=10000 → 8000 ≤ 10000 → rawDelta=0 → accepted=0
		//   - sync C: clientTotal=12000，maxReported=10000 → 12000 > 10000 → rawDelta=2000 → accepted=2000
		//   - 总 accepted = 5000 + 0 + 2000 = 7000；正确（A 截断丢的 5000 永久丢，但不会被 C 重复入账）
		//
		// **不**回退到老的"latest.ClientTotalSteps 当基线"路径：r5 之后 latest 的角色仅
		// 限于"是否首次同步"判断（latest == nil → 首次）和审计调试。
		//
		// 中间用 int64 避免 uint64 减法 overflow。
		var rawDelta int64
		maxReported, err := s.stepSyncLogRepo.MaxClientTotalStepsByUserAndDate(txCtx, in.UserID, in.SyncDate)
		if err != nil {
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}
		if latest == nil {
			// 首次同步：maxReported 一定是 0（COALESCE 兜底）；rawDelta = clientTotalSteps
			rawDelta = int64(in.ClientTotalSteps)
		} else if in.ClientTotalSteps > maxReported {
			// 新高 → delta = clientTotalSteps - maxReported
			rawDelta = int64(in.ClientTotalSteps - maxReported)
		} else {
			// clientTotalSteps <= maxReported → 旧 / 重复 / 乱序到达，全部 rawDelta=0
			// （DB 仍写 sync_log 行做审计，accepted_delta_steps=0）
			rawDelta = 0
		}

		// (3) 当日 SUM 兜底（保留 r3 的第二层防御 —— 即使 maxReported 漏判，SUM 也能拦）
		//
		// **为什么 r5 后还保留**：
		//   - maxReported clamp 已经在大多数场景下让 rawDelta 不会算多；
		//   - 但 SUM 兜底是**入账总额**视角的硬约束："今日 accepted 总和 + 本次 ≤ client 当日累计"；
		//   - 两层防御独立，叠加更安全（防御纵深 / defense-in-depth）；
		//   - 在"无截断"场景下 SUM 兜底是**冗余**的（maxReported 已削过），但**不会改变**结果；
		//   - 在"有截断"场景下 SUM(prevAccepted) < maxReported（prevAccepted 漏掉了被截断的部分），
		//     SUM 兜底反而比 maxReported clamp 更**宽松**，主防御还是 maxReported clamp。
		//
		// **顺序关键**：必须在单次截断之前 / 当日封顶之前算 prevAccepted —— 否则截断
		// 后的小 delta 会跑进封顶判断，但兜底失效；且兜底削回必须以 rawDelta 为基础。
		prevAccepted, err := s.stepSyncLogRepo.SumAcceptedDeltaByUserAndDate(txCtx, in.UserID, in.SyncDate)
		if err != nil {
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}
		if latest != nil && prevAccepted+rawDelta > int64(in.ClientTotalSteps) {
			adjusted := int64(in.ClientTotalSteps) - prevAccepted
			if adjusted < 0 {
				adjusted = 0
			}
			slog.WarnContext(txCtx, "step sync sum cap adjusted",
				"user_id", in.UserID, "sync_date", in.SyncDate,
				"prev_accepted", prevAccepted, "raw_delta", rawDelta,
				"client_total_steps", in.ClientTotalSteps, "adjusted_to", adjusted)
			rawDelta = adjusted
		}

		// (4) 防作弊单次截断
		delta := rawDelta
		if delta > s.singleSyncCap {
			slog.WarnContext(txCtx, "step sync single cap truncated",
				"user_id", in.UserID, "sync_date", in.SyncDate,
				"raw_delta", rawDelta, "truncated_to", s.singleSyncCap)
			delta = s.singleSyncCap
		}

		// (5) 防作弊当日封顶（**入账后越界判断**：prevAccepted + curDelta > dailyCap → 拒绝）
		// 重置为本次事务的判定结果（防上次调用残留）
		capExceeded = false
		if prevAccepted+delta > s.dailyCap {
			slog.WarnContext(txCtx, "step sync daily cap exceeded",
				"user_id", in.UserID, "sync_date", in.SyncDate,
				"prev_accepted", prevAccepted, "cur_delta", delta, "daily_cap", s.dailyCap)
			delta = 0
			capExceeded = true
		}

		// (6) 取 step_account 当前 version
		account, err := s.stepAccountRepo.FindByUserID(txCtx, in.UserID)
		if err != nil {
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}

		// (7) UpdateBalance —— 即便 delta=0 也走，保持事务边界一致；version + 1 仍递增。
		if err := s.stepAccountRepo.UpdateBalance(txCtx, in.UserID, int32(delta), account.Version); err != nil {
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}

		// (8) 写 sync_log（**含**倒退 / 重复 / 截断 / 封顶场景；append-only 审计纪律）
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

		// (9) 拼装 output —— 用更新**之后**的余额（account.X + delta）
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
