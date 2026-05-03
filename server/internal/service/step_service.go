package service

import (
	"context"
	stderrors "errors"
	"fmt"
	"log/slog"
	"math"
	"strings"

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
	// 流程（数据库设计 §8.2 + V1 §6.1.4 + Story 7.3 review r3/r6 双层防御）：
	//  1. 在事务内：FindLatestByUserAndDate(userID, syncDate) → 基线 lastClientTotalSteps
	//     （**latest 必须由 id DESC 取最近 INSERT 行；不是 MAX(client_total_steps)**——
	//      reset 场景必须让基线跟最近 sync 走，否则永久卡死历史高水位）
	//  2. **基线减法 + 倒退削成 0**：
	//     - latest == nil → 首次 sync：rawDelta = clientTotalSteps
	//     - clientTotalSteps > latest.ClientTotalSteps → rawDelta = clientTotalSteps - latest.ClientTotalSteps
	//     - 否则（≤）→ rawDelta = 0（倒退 / 重复 / 乱序到达均归 0）
	//  3. **SUM 兜底**（r3 第二层防御）：
	//     SumAccepted(today) + rawDelta > clientTotalSteps
	//       → rawDelta = max(0, clientTotalSteps - SumAccepted)
	//     这是当前唯一的乱序兜底；不彻底（"截断+乱序"组合下仍可能小幅多算），
	//     与 single_sync_cap=5000 / daily_cap=50000 联合兜底为"接受的 known limitation"，
	//     详见步骤 (3) 实装注释 + docs/lessons/2026-05-04-*-r6-reset-vs-ooo-tradeoff.md
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
// envName 是部署环境名（"prod" / "staging" / "dev" / "test"，默认 "prod"），
// 由 main.go 从环境变量 `CAT_ENV` 读取传入；用于"prod 必须用默认值"契约**强制**：
//   - envName == "prod"（含空 / 不识别值，按 prod 严格策略）+ cfg.SingleSyncCap > 0
//     → panic（"prod env must use default caps; got single_sync_cap=X"）
//   - envName == "prod" + cfg.DailyCap > 0 → panic
//   - envName ∈ {"dev", "staging", "test"} → 接受 YAML 任何正值覆盖（仅供单测 / 调试 / fixture）
//
// **fail-fast 设计**：契约文档（V1 §6.1.4 + Story 7.3）钦定 prod 必须 5000/50000，
// 旧版无机制实施这个钦定（仅依靠"开发者读文档"）—— 一旦运维误把 dev YAML 推到 prod
// 或 prod YAML 被改，会出现"跨实例阈值漂移引发跨端契约不一致"且**无声**。
// 现在启动期就 panic，与 db.Open / auth.New 同 fail-fast 模式（CLAUDE.md
// "No Backup Fallback" + ADR-0006 钦定）。
//
// cfg 是配置侧的 StepsConfig；兜底逻辑：cfg.SingleSyncCap == 0 → 用
// defaultStepsSingleSyncCap；DailyCap 同理。
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
func NewStepService(
	txMgr tx.Manager,
	stepAccountRepo mysql.StepAccountRepo,
	stepSyncLogRepo mysql.StepSyncLogRepo,
	cfg config.StepsConfig,
	envName string,
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

	// **prod 配置覆盖强制**（Story 7.3 review r6 [P2]）：
	// envName 归一化为小写；只有显式 "dev" / "staging" / "test" 才允许 cap 覆盖。
	// **空 / 未知 / "prod" / "production"** 全部按 prod 严格策略（safe-by-default：
	// 未注入 CAT_ENV 或 typo 都视为 prod，避免 dev YAML 静默流到 prod 引发跨实例契约漂移）。
	envLower := strings.ToLower(strings.TrimSpace(envName))
	isOverrideAllowed := envLower == "dev" || envLower == "staging" || envLower == "test"
	if !isOverrideAllowed && (cfg.SingleSyncCap > 0 || cfg.DailyCap > 0) {
		panic(fmt.Sprintf(
			"step service: prod env (CAT_ENV=%q) must use default caps; got single_sync_cap=%d daily_cap=%d (V1 §6.1.4 钦定 5000/50000；dev/test 覆盖必须 export CAT_ENV=dev|staging|test)",
			envName, cfg.SingleSyncCap, cfg.DailyCap,
		))
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
		// (1) 查最近 sync_log（同 user 同 sync_date）—— **作基线**：rawDelta 用 latest.ClientTotalSteps 算
		latest, err := s.stepSyncLogRepo.FindLatestByUserAndDate(txCtx, in.UserID, in.SyncDate)
		if err != nil && !stderrors.Is(err, mysql.ErrStepSyncLogNotFound) {
			return apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
		}

		// (2) **基线减法 + 倒退削成 0**（Story 7.3 review r6 [P1]：回退到 r3 状态）
		//
		// **决策史**（详见 step_sync_log_repo.go interface doc + lessons 2026-05-04 r6）：
		//   - r1 基线 id DESC（最近 INSERT）—— 乱序到达让旧 sync 成新基线 → 重复入账
		//   - r2 基线 max(client_total_steps)—— 乱序 OK 但 HealthKit reset 永久卡死
		//   - r3 基线 id DESC + service 层 SUM 兜底 —— 解决 reset 与常规乱序，但
		//     "截断 + 乱序"组合下 SUM 兜底失效，可能小幅多算（见 r5 review）
		//   - r5 maxReported clamp（基线改用 MAX(client_total_steps)）—— 修了"截断 + 乱序"
		//     但**重新破坏 reset 修复**：reset 后用户走的步数永远 < 历史高水位 → 永久 0 入账（r6 review）
		//   - **r6 当前**：选 **reset 优先** 路径 —— 回到 r3 状态，接受"截断 + 乱序"小幅多算作为
		//     **known limitation**（已被 single_sync_cap=5000 + daily_cap=50000 兜底，损失有限）
		//
		// **产品权衡**（reset vs 截断+乱序，二选一）：
		//   - reset 是 HealthKit 常见 correction 场景，永久少算几千步会让用户当日体验严重退化
		//     （看到步数停滞）→ **必须**正确处理
		//   - "截断+乱序"组合是少见 corner case（要先 5000+ 步突发被截断，又恰好乱序到达）
		//     → 多算几百步对用户是"占小便宜"无感知，对系统是有限的安全损失（cap 兜底）
		//   - 选 reset 优先（产品 UX > 小幅多算）。详见 docs/lessons/2026-05-04-*-r6-*.md
		//
		// 中间用 int64 避免 uint64 减法 overflow。
		var rawDelta int64
		if latest == nil {
			// 首次同步：rawDelta = clientTotalSteps
			rawDelta = int64(in.ClientTotalSteps)
		} else if in.ClientTotalSteps > latest.ClientTotalSteps {
			// 新高 → delta = clientTotalSteps - latest（**latest 必须是 id DESC 取的最近 INSERT 行**——
			// reset 场景靠"基线跟最近 sync 走"，不能用 MAX 锁死历史高水位）
			rawDelta = int64(in.ClientTotalSteps - latest.ClientTotalSteps)
		} else {
			// clientTotalSteps <= latest.ClientTotalSteps → 倒退 / 重复 / 乱序到达，全部 rawDelta=0
			// （DB 仍写 sync_log 行做审计，accepted_delta_steps=0）
			rawDelta = 0
		}

		// (3) 当日 SUM 兜底（r3 的第二层防御 —— 防绝大多数乱序到达场景下"基线偏小 → rawDelta 算多"）
		//
		// **顺序关键**：必须在单次截断之前 / 当日封顶之前算 prevAccepted —— 否则截断
		// 后的小 delta 会跑进封顶判断，但兜底失效；且兜底削回必须以 rawDelta 为基础。
		//
		// **known limitation**（r6 接受）：当用户**先**有 sync 被 single_sync_cap 截断（导致 SUM(accepted) < MAX(client_total)），
		// **再**有旧 sync 乱序到达（拉低 latest），**再**有新高 sync —— 此时 prevAccepted + rawDelta 可能
		// 仍 < clientTotalSteps（因为 prevAccepted 漏掉了被截断的部分），SUM 兜底不触发，多算 = (latest_drop * 1)。
		// 单次损失上限 = 5000 步（被 single_sync_cap 兜住）；当日上限 = 50000 步（被 daily_cap 兜住）；
		// 触发概率极低（要 5000+ 步突发 + 恰好乱序）。详见 lessons 2026-05-04 r6。
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
