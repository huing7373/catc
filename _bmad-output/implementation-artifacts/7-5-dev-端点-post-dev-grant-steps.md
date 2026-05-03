# Story 7.5: dev 端点 POST /dev/grant-steps

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a demo / 开发者,
I want 一个 build flag gated 的 dev 接口给指定用户加步数（直接更新 user_step_accounts 三档值 + 写一条 source=2 admin_grant 的 sync_log 审计行）,
so that demo / 自动化 e2e / 手工调试时不必真走 1000 步就能把账户余额堆到任意值，验证下游消费场景（节点 7 开宝箱 / 节点 9 跨端 e2e 步数链路 / Story 9.1 验证场景 6）.

## 故事定位（Epic 7 收官 = 节点 3 第三条 server 业务实装；上承 7.3 入账事务 + 7.4 账户读取，下启 iOS Epic 8 跨端 e2e demo 数据准备）

- **Epic 7 进度**：7.1（契约最终化，**done** —— V1 §1 / §6.1 / §6.2 schema + 错误码 + syncDate 时区契约 + 节点 3 冻结声明落地）→ 7.2（user_step_sync_logs migration，**done** —— 0006 表 + dockertest 6 表断言落地）→ 7.3（POST /steps/sync handler + service + 累计差值入账事务 + 防作弊，**done** —— `step_sync_log_repo` + `step_account_repo.UpdateBalance` + `step_service.SyncSteps` + `steps_handler.PostSync` + `bootstrap/router.go` wire）→ 7.4（GET /steps/account handler + service.GetAccount 纯读，**done** —— `*StepAccountBrief` 复用 + `getAccountResponseDTO` 扁平结构）→ **7.5（本 story，POST /dev/grant-steps dev 端点 + 直接 +delta 入账 + source=2 sync_log 审计 + 单测 ≥4 + 集成 ≥1）**。
- **本 story 是 Epic 7 收官之作**，也是节点 3 server 端"演示数据兜底"基础设施 —— 上线前没有真步数源（HealthKit 节点 3 阶段 iOS 才接入，server 端单测 / 集成 / e2e 没法依赖真用户走路）。本端点让 demo / 自动化测试环境可以**程序化**把 `user_step_accounts.available_steps` 堆到 5000 / 10000 / 100000 任意值，**不**经过 7.3 防作弊 / 单次截断 / 当日封顶约束（dev grant 场景的产品语义就是"绕过常规约束"——但仍写一条 source=2 sync_log 行做审计）。
- **下游 Story 9.1 验证场景 6 钦定**（`_bmad-output/planning-artifacts/epics.md` 行 1589）："dev 步数发放：BUILD_DEV=true server，调 `/dev/grant-steps {userId, steps:5000}` → App 主界面步数显示 +5000"。本 story 必须落地的接口形状与该 e2e 场景一致。
- **下游 Epic 20 / 21 开宝箱链路**：21.5 "开箱前主动同步步数" 单元 / 集成测试需要"账户余额预置 5000"作为 fixture；本端点是该 fixture 的标准生成路径（vs 直接 SQL INSERT，本端点走真实 service 路径覆盖 HTTP / handler 链）。
- **epics.md AC 钦定**（`_bmad-output/planning-artifacts/epics.md` §Story 7.5 行 1423-1443）：
  - **Given** Epic 1 Story 1.6 Dev Tools 框架已就绪 + Story 7.3 步数 service 可用
  - **When** 仅在 `BUILD_DEV=true` 模式调用 `POST /dev/grant-steps {userId: int64, steps: int}`
  - **Then** service 直接增加 `user_step_accounts.total_steps += steps, available_steps += steps`
  - **And** 同时写一条 sync_log（`source=2`（admin_grant），见数据库设计 §6.6；标识来源是 dev）
  - **And** 生产构建（BUILD_DEV=false）下访问该端点返回 404
  - **And** 接口**不**要求 auth（因为是 dev 内部用）
  - **And** 单元测试 ≥4 case：(1) dev mode + 用户存在 → 正确加步数；(2) dev mode + 用户不存在 → 1003；(3) dev mode + steps 为负数 → 1002 参数错误；(4) 非 dev mode → 路由返回 404（由 Epic 1 dev_only 中间件保证）
  - **And** 集成测试（dockertest + BUILD_DEV=true）：创建用户 → /dev/grant-steps 加 5000 → /steps/account 返回 available=5000
- **V1 接口设计 doc 状态**：本 dev 端点**不**在 V1 §1-§16 主接口清单内（V1 §1 line 23 节点 3 冻结声明只锁 §6.1 / §6.2 业务接口）。dev 端点契约**仅**由 epics.md §Story 7.5 钦定 + 数据库设计 §6.6 source=2 admin_grant 钦定。但 V1 doc 已两处提及本端点：(1) §6.1.4 line 553 注释 "dev grant 走 source=2"；(2) 数据库设计 §6.6 line 769 "POST /dev/grant-steps, 见 Story 7.5"。
- **devtools 框架契约**（Story 1.6 已落地；详见 `server/internal/app/http/devtools/devtools.go` + `_bmad-output/implementation-artifacts/1-6-dev-tools-框架.md`）：
  - **双闸门** OR 启用：(1) build tag `-tags devtools` → `forceDevEnabled=true`；(2) env var `BUILD_DEV=true`（严格字面，**不**接受 `"1"` / `"yes"` / `"TRUE"`）。任一即启用。
  - **Register(r)** 在 IsEnabled()==false 时直接返回，不挂任何 /dev/* 路由 → Gin 默认 NoRoute → HTTP 404 + 文本 `"404 page not found"`（**非** envelope）。
  - **DevOnlyMiddleware()** 是 /dev/* 路由组的请求时第二闸门：IsEnabled() false 时推 ErrResourceNotFound (1003) → ErrorMappingMiddleware 翻成 envelope（HTTP 200 + `code=1003` + `message="资源不存在"`）。
  - **业务 dev 端点扩展模式**：在 `devtools.Register(r)` 内部加 `g.POST("/grant-steps", devGrantStepsHandler.PostGrantSteps)`（与现有 `g.GET("/ping-dev", PingDevHandler)` 同模式），**不**新建 /dev 路由组。
  - **devtools 层 vs 业务层分离**：devtools 包只做"框架"（Register / DevOnlyMiddleware / 启停判定）；本 story 的"步数 dev 业务逻辑"（service 层 GrantSteps + handler）写到**业务层**（`internal/service/dev_step_service.go` + `internal/app/http/handler/dev_steps_handler.go`），devtools.go 仅追加路由注册一行 + 通过依赖注入接收 handler。

## 范围红线（明确不做）

**本 story 只做**：
1. **service 层**：新建 `server/internal/service/dev_step_service.go` —— `DevStepService` interface + `devStepServiceImpl` 实装 + `NewDevStepService` 构造函数；`GrantSteps(ctx, userID, steps) error` 方法（事务内：FindByID(user) 验在 + UpdateBalance(+steps, version) + Create sync_log source=2）。**不**复用 `step_service.StepService.SyncSteps`（防作弊 / 截断 / 当日封顶 / SUM 兜底全不适用 dev 语义；强行复用要绕过 5+ 个约束分支，反模式）。
2. **handler 层**：新建 `server/internal/app/http/handler/dev_steps_handler.go` —— `DevStepsHandler` struct + `NewDevStepsHandler` + `PostGrantSteps(c *gin.Context)` 方法 + `PostGrantStepsRequest` DTO + `postGrantStepsResponseDTO` helper。
3. **devtools 层**：扩 `server/internal/app/http/devtools/devtools.go` —— 改 `Register(r *gin.Engine)` 签名为 `Register(r *gin.Engine, devStepsHandler *handler.DevStepsHandler)`（接收业务 handler），在 `g.GET("/ping-dev", PingDevHandler)` 之后追加 `g.POST("/grant-steps", devStepsHandler.PostGrantSteps)`。**关键 wire**：handler nil 时**仍**注册 ping-dev（与现状兼容），但**跳过** /dev/grant-steps 路由（避免 nil deref；与 router.go 的 `if deps.GormDB != nil && ...` 同 nil-tolerant 模式）。
4. **bootstrap 层**：扩 `server/internal/app/bootstrap/router.go` —— 在业务 wire 块内（`if deps.GormDB != nil && ...` 块**外**：dev 端点不要求 deps 完整，但实际依赖 GormDB / TxMgr 注入 dev service）……**重新评估**：dev grant 写 `user_step_accounts` 必须有 GormDB + TxMgr，故仍放在 `if deps.GormDB != nil && ...` 块内，构造 `devStepSvc` + `devStepsHandler` + 透传给 `devtools.Register(r, devStepsHandler)`。当 deps 不完整（单元测试 `NewRouter(Deps{})`）时，传 nil 给 Register，Register 跳过 /dev/grant-steps 路由注册（ping-dev 仍正常）。
5. **service 单测**：新建 `server/internal/service/dev_step_service_test.go` —— ≥5 case（HappyPath / UserNotFound 1003 / NegativeSteps 1002 / ZeroSteps 边界 / TxRollback DB error）。复用 7.3 已建的 `stubStepStepAccountRepo` / `stubStepSyncLogRepo` / `stubStepTxMgr` 模式（同 package 已 export 内部 helper，可 import；如不可复用则在本文件内重新定义独立 stub）。**注意**：本 service 还需要 `stubUserRepo` —— **不存在** —— 必须新建（最小 stub `findByIDFn` 字段 + `FindByID` 方法 + 其他方法 panic-default）。
6. **handler 单测**：新建 `server/internal/app/http/handler/dev_steps_handler_test.go` —— ≥4 case（HappyPath_ReturnsAccount / UserNotFound 1003 / NegativeSteps 1002 / DBBusy 1009）。模式与 `steps_handler_test.go` 同：stub service + 测试 router + ErrorMappingMiddleware。
7. **集成测试**：扩 `server/internal/service/step_service_integration_test.go` 末尾 **OR** 新建 `dev_step_service_integration_test.go`。**决策**：新建独立文件（`dev_step_service_integration_test.go`）—— 避免与 7.3 / 7.4 既有 6 case 串扰；与现有 7.3 一份 / 7.4 一份并列模式更清晰。≥1 case：BUILD_DEV=true 容器 → INSERT user + step_account → svc.GrantSteps(userID, 5000) → SELECT user_step_accounts → total=5000, available=5000, consumed=0；SELECT user_step_sync_logs → 1 行 source=2, accepted_delta_steps=5000。
8. **devtools 框架测试扩展**：扩 `server/internal/app/bootstrap/router_dev_test.go` —— 追加 1 case：BUILD_DEV=true → POST /dev/grant-steps（带 valid body）应该被路由到 handler；BUILD_DEV="" → POST /dev/grant-steps 返回 Gin NoRoute 404。**OR** 扩 `server/internal/app/http/devtools/devtools_test.go` —— 追加 1 case：BUILD_DEV=true → Register 后 POST /dev/grant-steps 路由存在。**决策**：两处都加最小 case（router_dev_test 验"完整 wire 链路"；devtools_test 仅验"路由注册 / 跳过"），避免漏覆盖。
9. **本 story 文件 + sprint-status.yaml** 更新。

**本 story 不做**：

- **不**改 V1 §6.1 / §6.2 / §3 任一字（dev 端点不属 V1 主清单；epics.md 钦定即足）
- **不**改 `docs/宠物互动App_V1接口设计.md` 任一行（dev 端点是私有运维接口，不进 V1 doc）
- **不**改 `docs/宠物互动App_数据库设计.md` 任一行（§6.6 source=2 admin_grant 已存在；本 story 仅消费）
- **不**改 `migrations/0001-0006` 任一文件（user_step_accounts / user_step_sync_logs 表结构已锁；本 story 仅消费现有列）
- **不**改 `step_service.go` (7.3) / `steps_handler.go` (7.3 / 7.4) 任一行（dev 走独立 service / handler，**不**复用 SyncSteps / GetAccount / PostSync / GetAccount handler）
- **不**改 `step_account_repo.go` / `step_sync_log_repo.go` 任一行（仅消费现有 `FindByUserID` / `UpdateBalance` / `Create`）
- **不**改 `internal/pkg/errors/codes.go`（1002 / 1003 / 1009 全已注册；本 story 仅消费）
- **不**改 `internal/app/http/middleware/auth.go` / `rate_limit.go` / `error_mapping.go`（已实装；dev 端点**不**挂 auth / **不**挂 rate_limit）
- **不**改 `Deps` struct（dev grant 用 `Deps.GormDB` / `Deps.TxMgr` 已注入；不引入新依赖）
- **不**修改 `cmd/server/main.go`（router wire 内部消费 Deps，main.go 透明）
- **不**改 `local.yaml` 任一行（无新配置项；阈值 / cap 不适用 dev 端点）
- **不**接 Redis（节点 3 阶段无 Redis；dev grant 无幂等需求 —— 测试可重复调用，每次都 +N）
- **不**接幂等键 `idempotencyKey`（dev 端点是"故意可重复"语义；不像 chest/open 不能重复扣可用步数）
- **不**接 SUM 兜底 / single_sync_cap / daily_cap（dev grant 的产品语义就是"绕过约束 +N"；强行接会让 demo "调 100000 拿不到 100000"违反端点用途）
- **不**实装 `POST /dev/grant-steps` 的 prod 防误触发（双闸门已防：build tag + env var；prod 二进制 + 运维 SOP 不设 BUILD_DEV → 物理不可达）
- **不**支持 `userId=0` / `userId=-1` / `userId="abc"` 等异常（handler binding 校验 + service.FindByID 自然失败）
- **不**写 e2e 跨端测试（Epic 9 Story 9.1 验证场景 6 才做）
- **不**做"prod 二进制访问 /dev/grant-steps 返 envelope code=1003"测试 —— Story 1.6 devtools_test.go 已有 `TestRegister_BuildDevFalse_PingDevReturns404` + `TestDevOnlyMiddleware_RejectsWhenDisabled` 覆盖通用闸门，本 story 不重复
- **不**改 `docs/lessons/*.md`（无新教训；本 story 是直接落地，无 review iteration）

**任何超出上述清单的改动 → HALT 并问设计**。

## Acceptance Criteria

**AC1 — `service.DevStepService` interface + impl（新文件 `internal/service/dev_step_service.go`）**

新建 `server/internal/service/dev_step_service.go`：

```go
package service

import (
	"context"
	stderrors "errors"

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
```

`GrantSteps` 实装段（追加到上述 NewDevStepService 之后）：

```go
import (
	"time"
	"log/slog"
)

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
		_, err := s.userRepo.FindByID(txCtx, userID)
		if err != nil {
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
```

**关键约束**：

- **新 service 文件**：dev grant 业务规则简单（4 步事务），但与 SyncSteps 5+ 约束分支语义彻底不同 → 独立文件清晰；**不**复用 SyncSteps
- **事务边界**：4 步必须原子（任一失败回滚） —— 与 7.3 SyncSteps 同模式；txMgr.WithTx 闭包内 repo 调用全用 `txCtx`（ADR-0007）
- **SyncDate 用 server today UTC**：dev 端点没有 client 时区概念；UTC today 字面值写库（与 7.3 sync_date 字段语义一致 —— 字符串字面值无时区耦合）
- **source=2 admin_grant**：数据库设计 §6.6 钦定；与 7.3 source=1 healthkit 区分
- **MotionState=1 / ClientTs=0**：满足 NOT NULL 约束的中性值；不引入业务语义
- **steps=0 边界**：service 仍走完整 4 步事务（version+1，sync_log 写入）—— 与 7.3 capExceeded 时 delta=0 同模式（审计纪律）
- **steps<0 panic**：防御性，handler 必须 1002 拦截；service 走到这里说明 handler 漏校验；**不**做"steps<0 → 1002"逻辑（service 不该承担参数校验，handler 职责）
- **不接 single_sync_cap / daily_cap / SUM 兜底**：dev 端点产品语义就是"绕过约束"
- **不写 lastClientTotal / 差值计算**：dev 不读 sync_log 历史（每次都直接 +steps）

**AC2 — `handler.DevStepsHandler` + `PostGrantStepsRequest` DTO（新文件 `internal/app/http/handler/dev_steps_handler.go`）**

新建 `server/internal/app/http/handler/dev_steps_handler.go`：

```go
package handler

import (
	"github.com/gin-gonic/gin"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/pkg/response"
	"github.com/huing/cat/server/internal/service"
)

// DevStepsHandler 是 /dev/grant-steps 等 dev 步数端点的 handler 集合（Story 7.5）。
//
// 与 StepsHandler (7.3 / 7.4) 区分：
//   - StepsHandler 处理 /api/v1/steps/*（业务接口；含 auth + rate_limit + 防作弊）
//   - DevStepsHandler 处理 /dev/grant-steps（dev 工具；不含 auth / rate_limit / 防作弊）
//
// 独立 handler 文件让"dev 工具"与"业务接口"边界清晰，dev 路径未来加新端点
// （如 /dev/grant-cosmetic-batch in Epic 20.8）也走本 handler。
type DevStepsHandler struct {
	svc service.DevStepService
}

// NewDevStepsHandler 构造 DevStepsHandler。
func NewDevStepsHandler(svc service.DevStepService) *DevStepsHandler {
	return &DevStepsHandler{svc: svc}
}

// PostGrantStepsRequest 是 POST /dev/grant-steps 请求体的 Go mirror。
//
// epics.md §Story 7.5 行 1432 钦定：`{userId: int64, steps: int}`。
//
// **userId 用 *uint64 指针类型 + binding:"required"**：
//   - dev 端点没有 auth 中间件注入 userID（接口要求**不**要求 auth），全靠 body 里 userId 字段
//   - 必须区分"字段缺失" vs "显式传 0"：用 *uint64 指针 + 显式 nil 校验（与 7.3 PostSyncRequest
//     ClientTotalSteps 同模式）
//   - userId=0 在 MySQL users 表里**不存在**（AUTO_INCREMENT 从 1 起），传 0 也会自然 1003，
//     但 handler 层显式校验 != 0 让错误更早 + 错误消息更精确（"userId 必须 > 0"）
//
// **steps 用 *int32 指针类型 + binding:"required"**：
//   - 必须区分"字段缺失" vs "显式传 0"：用 *int32 指针
//   - service 层 accepts steps>=0；handler 层补 steps<0 → 1002（"steps 不能为负数"）
//   - 类型 int32（与 sync_log.accepted_delta_steps 同；INT signed；上界约 21 亿步够用）
type PostGrantStepsRequest struct {
	UserID *uint64 `json:"userId" binding:"required"`
	Steps  *int32  `json:"steps"  binding:"required"`
}

// PostGrantSteps 处理 POST /dev/grant-steps（Story 7.5）。
//
// 流程：
//  1. ShouldBindJSON 兜一层（字段类型错 → 1002）
//  2. 手动校验：
//     - userId 非 nil（缺失 → 1002）+ != 0（非法 → 1002）
//     - steps 非 nil（缺失 → 1002）+ >= 0（负数 → 1002 epics.md §Story 7.5 行 1440 钦定）
//  3. 调 svc.GrantSteps(ctx, *userId, *steps) —— ctx = c.Request.Context()
//  4. 成功 → response.Success(c, postGrantStepsResponseDTO(...), "ok")
//  5. 失败 → c.Error(err) + return（middleware envelope；ADR-0006 单一 envelope 生产者）
//
// **不**做 auth 校验（dev 端点不要求 auth）；**不**取 c.Get(UserIDKey)（dev 路径无 auth 中间件）。
func (h *DevStepsHandler) PostGrantSteps(c *gin.Context) {
	var req PostGrantStepsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		_ = c.Error(apperror.Wrap(err, apperror.ErrInvalidParam, apperror.DefaultMessages[apperror.ErrInvalidParam]))
		return
	}

	// === required 字段校验（指针 nil → 字段未传 → 1002）===
	if req.UserID == nil {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "userId 必填"))
		return
	}
	if *req.UserID == 0 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "userId 必须 > 0"))
		return
	}
	if req.Steps == nil {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "steps 必填"))
		return
	}
	if *req.Steps < 0 {
		_ = c.Error(apperror.New(apperror.ErrInvalidParam, "steps 不能为负数"))
		return
	}

	if err := h.svc.GrantSteps(c.Request.Context(), *req.UserID, *req.Steps); err != nil {
		_ = c.Error(err) // service 已 wrap *AppError；ErrorMappingMiddleware 写 envelope
		return
	}

	// 成功响应 —— 简单 ack，不返当前账户值（如要查账户用 GET /steps/account；端点单一职责）
	response.Success(c, postGrantStepsResponseDTO(*req.UserID, *req.Steps), "ok")
}

// postGrantStepsResponseDTO 拼装 ack response。
//
// **schema 选择**：返 `{userId, grantedSteps}` 简单 ack —— 不返当前账户三档值。
//   - 调用方（Story 9.1 e2e / 自动化测试）调本端点后再调 GET /steps/account 验证最终值，
//     而不是依赖本端点 response —— 端点单一职责（grant 只负责"做了"，account 只负责"读了"）
//   - 反模式：返当前账户值 → 调用方依赖该字段 → 未来加 grant 多个 user 批量端点时 schema 难扩展
func postGrantStepsResponseDTO(userID uint64, grantedSteps int32) gin.H {
	return gin.H{
		"userId":       userID,
		"grantedSteps": grantedSteps,
	}
}
```

**关键约束**：

- handler 层**必须**做 steps<0 → 1002 校验（epics.md §Story 7.5 行 1440 钦定 "steps 为负数 → 1002 参数错误"）；service 层 panic 防御
- userId / steps 都用**指针 + binding:"required"**（与 7.3 PostSyncRequest 同模式；区分缺失 vs 显式 0）
- userId=0 显式拒（额外严格 + 早 fail；MySQL users.id 从 1 起 AUTO_INCREMENT，0 永远不存在）
- response 返简单 ack `{userId, grantedSteps}`，**不**返当前账户值（端点单一职责）
- handler **不**调 `c.Get(UserIDKey)`（dev 路径无 auth）；userID 全靠 body
- `c.Error + return` 而非 `response.Error`（ADR-0006）

**AC3 — `devtools.Register` 签名扩 + 业务 dev 路由注册（修改 `internal/app/http/devtools/devtools.go`）**

修改 `server/internal/app/http/devtools/devtools.go`：

1. **改 Register 签名**：从 `Register(r *gin.Engine)` 改为 `Register(r *gin.Engine, devStepsHandler DevStepsHandler)`，用一个新 interface `DevStepsHandler` 解耦（避免 import cycle）

```go
// DevStepsHandler 是 dev 步数端点的 handler 抽象（避免 devtools 包反向 import handler 包）。
//
// 实际的 handler 实装在 internal/app/http/handler/dev_steps_handler.go；本 interface
// 仅为 Register 签名抽象，让 devtools 包保持"框架"角色，不依赖具体 handler 实装。
//
// **签名简化原则**：本 interface 只列 Register 签名所需的方法（PostGrantSteps）；future
// 加 /dev/grant-cosmetic-batch (Epic 20.8) 时按需追加方法到本 interface。
type DevStepsHandler interface {
	PostGrantSteps(c *gin.Context)
}

// Register 把 /dev/* 路由组挂到传入的 gin.Engine 上（仅在 dev 模式启用时）。
//
// 启用时挂载以下端点：
//   - GET  /dev/ping-dev    → PingDevHandler（Story 1.6 框架自带）
//   - POST /dev/grant-steps → devStepsHandler.PostGrantSteps（Story 7.5；devStepsHandler == nil 时跳过）
//
// **devStepsHandler 可空设计**（nil-tolerant）：
//   - 单元测试 NewRouter(Deps{}) 零值场景：bootstrap 不构造 DevStepsHandler → 传 nil
//     → 本函数仅注册 ping-dev，跳过 /dev/grant-steps（避免 nil deref panic）
//   - 生产路径：bootstrap 在 deps 完整时构造 devStepsHandler 透传 → 本函数注册全部 dev 端点
func Register(r *gin.Engine, devStepsHandler DevStepsHandler) {
	if !IsEnabled() {
		return
	}
	slog.Warn("DEV MODE ENABLED - DO NOT USE IN PRODUCTION", ...)
	g := r.Group("/dev")
	g.Use(DevOnlyMiddleware())
	g.GET("/ping-dev", PingDevHandler)
	if devStepsHandler != nil {
		g.POST("/grant-steps", devStepsHandler.PostGrantSteps)
	}
}
```

2. **改 PingDevHandler 注释**（无需改实装）：明确 ping-dev 是"框架健康检查"，业务 dev 端点（grant-steps / 等）由 router 透传 handler 注册。

**关键约束**：

- 用 interface 解耦：devtools 包**不**反向 import handler 包（避免 import cycle）
- nil-tolerant：`devStepsHandler == nil` 跳过 grant-steps 路由 —— 与 NewRouter 的 `if deps.GormDB != nil && ...` 同 nil-tolerant 模式，让单测 `NewRouter(Deps{})` 不崩
- **不**新建 /dev 路由组（仍在 `g := r.Group("/dev")`）—— 与现有 ping-dev 共享 DevOnlyMiddleware
- ping-dev 仍是框架自带（与 1.6 同；不动其语义）

**AC4 — `bootstrap/router.go` wire dev 业务 service / handler / 透传 Register（修改 `internal/app/bootstrap/router.go`）**

修改 `server/internal/app/bootstrap/router.go`：

1. 在 `if deps.GormDB != nil && ...` 块内（与 stepsHandler 构造并列）追加 dev service / handler 构造：

```go
// Story 7.5 加：dev step service + handler（仅 dev 模式 build 启用时挂路由）
//
// 即便 BUILD_DEV=false（IsEnabled()==false），此处仍构造 service / handler；devtools.Register
// 内部判定 IsEnabled() 决定是否真注册路由。这样"是否注册"的决策集中在 devtools 包内，
// bootstrap 不重复判定。
//
// 复用：userRepo / stepAccountRepo / stepSyncLogRepo / txMgr 已在 7.3 / 4.6 wire；本 story 仅
// 多 wire 一个 devStepSvc + devStepsHandler。
devStepSvc := service.NewDevStepService(deps.TxMgr, userRepo, stepAccountRepo, stepSyncLogRepo)
devStepsHandler := handler.NewDevStepsHandler(devStepSvc)
```

2. 改 `devtools.Register(r)` 调用为 `devtools.Register(r, devStepsHandler)`（注意：原调用在 `if` 块**外**，需要把 dev wire 块内构造的 handler 透传出去）：

**关键变更**：因 `devStepsHandler` 在 if 块内构造，需把 `devtools.Register(r, ...)` 也移到 if 块内 **OR** 在 if 块外用 `var devStepsHandler *handler.DevStepsHandler` 提前声明。**决策**：用变量提前声明 + if 块内赋值，让 `devtools.Register(r, devStepsHandler)` 留在 if 块**外**保持原结构（IsEnabled 判定不依赖 deps）。

```go
func NewRouter(deps Deps) *gin.Engine {
	r := gin.New()
	_ = r.SetTrustedProxies(nil)
	r.Use(middleware.RequestID())
	// ... 中间件 ...

	r.GET("/ping", handler.PingHandler)
	r.GET("/version", handler.VersionHandler)
	r.GET("/metrics", gin.WrapH(metrics.Handler()))

	var devStepsHandler *handler.DevStepsHandler // Story 7.5 加：if 块外声明 nil；deps 完整时填

	if deps.GormDB != nil && deps.TxMgr != nil && deps.Signer != nil {
		// ... 既有 wire（userRepo / authBindingRepo / petRepo / stepAccountRepo / chestRepo / stepSyncLogRepo）...
		// ... authSvc / authHandler / homeSvc / homeHandler / stepSvc / stepsHandler ...

		// Story 7.5 加：dev step service + handler
		devStepSvc := service.NewDevStepService(deps.TxMgr, userRepo, stepAccountRepo, stepSyncLogRepo)
		devStepsHandler = handler.NewDevStepsHandler(devStepSvc)

		api := r.Group("/api/v1")
		// ... 既有路由 wire ...
	}

	// dev 模式下挂 /dev/* 路由组；devStepsHandler nil 时仅注册 ping-dev（与 deps 不完整兼容）
	devtools.Register(r, devStepsHandler) // Story 7.5 修改：透传 dev handler

	return r
}
```

**关键约束**：

- `var devStepsHandler *handler.DevStepsHandler` 提前声明 nil —— deps 不完整时（单测）保持 nil，devtools.Register 内部跳过 grant-steps 路由
- 复用 7.3 已 wire 的 userRepo / stepAccountRepo / stepSyncLogRepo / txMgr —— **不**新建 repo 实例
- `devtools.Register(r, devStepsHandler)` 留在 if 块**外** —— IsEnabled() 判定不依赖 deps（与 1.6 既有结构一致）
- **不**改 `Deps` struct（无新依赖）
- **不**改 `cmd/server/main.go`（router wire 内部消费 Deps）

**AC5 — service 单测 ≥5 case（stub repo + stub txMgr；新文件 `dev_step_service_test.go`）**

新建 `server/internal/service/dev_step_service_test.go`：

**stub 设计**（与 7.3 step_service_test.go 同 package；可复用既有 stub 类型；缺失的 stubUserRepo 必须新建）：

```go
package service_test

import (
	"context"
	stderrors "errors"
	"testing"

	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/service"
)

// stubUserRepo 是 mysql.UserRepo 的最小 stub（dev grant 仅用 FindByID）。
type stubUserRepo struct {
	findByIDFn func(ctx context.Context, id uint64) (*mysql.User, error)
}

func (s *stubUserRepo) Create(ctx context.Context, u *mysql.User) error {
	panic("stubUserRepo.Create not configured (DevStepService should not call it)")
}
func (s *stubUserRepo) UpdateNickname(ctx context.Context, userID uint64, nickname string) error {
	panic("stubUserRepo.UpdateNickname not configured (DevStepService should not call it)")
}
func (s *stubUserRepo) FindByID(ctx context.Context, id uint64) (*mysql.User, error) {
	if s.findByIDFn == nil {
		return nil, stderrors.New("stub: findByIDFn not set")
	}
	return s.findByIDFn(ctx, id)
}

// buildDevStepService 用 stub repo 构造 DevStepService。
func buildDevStepService(
	userRepo mysql.UserRepo,
	accRepo mysql.StepAccountRepo,
	logRepo mysql.StepSyncLogRepo,
) service.DevStepService {
	return service.NewDevStepService(&stubStepTxMgr{}, userRepo, accRepo, logRepo)
}
```

**必须覆盖 5 case**（前缀 `TestDevStepService_GrantSteps_`）：

1. **`TestDevStepService_GrantSteps_HappyPath_AppliesAccountAndAuditLog`**：stubUserRepo.findByIDFn 返合法 user / stubStepStepAccountRepo.findByUserIDFn 返 `&mysql.StepAccount{TotalSteps: 100, AvailableSteps: 100, ConsumedSteps: 0, Version: 3}` / stubStepSyncLogRepo 默认 → service.GrantSteps(ctx, 1001, 5000) 返 err=nil；**断言**：accRepo.updateCalls=1 + accRepo.lastUpdateDelta=5000 + accRepo.lastUpdateVer=3；logRepo.createCalls=1 + logRepo.lastCreated.UserID=1001 + logRepo.lastCreated.AcceptedDeltaSteps=5000 + **logRepo.lastCreated.Source=2** + logRepo.lastCreated.MotionState=1 + logRepo.lastCreated.ClientTotalSteps=0 + logRepo.lastCreated.ClientTs=0 + logRepo.lastCreated.SyncDate matches `\d{4}-\d{2}-\d{2}` regex（server today UTC）

2. **`TestDevStepService_GrantSteps_UserNotFound_Returns1003`**：stubUserRepo.findByIDFn 返 `(nil, mysql.ErrUserNotFound)` → service 返 *AppError(Code=1003)；accRepo.updateCalls=0 + logRepo.createCalls=0（事务 fn 内 step 1 失败，后续 step 2-4 不执行）

3. **`TestDevStepService_GrantSteps_StepAccountNotFound_Returns1003`**：stubUserRepo.findByIDFn 返合法 user / stubStepStepAccountRepo.findByUserIDFn 返 `(nil, mysql.ErrStepAccountNotFound)` → service 返 *AppError(Code=1003)；accRepo.updateCalls=0 + logRepo.createCalls=0

4. **`TestDevStepService_GrantSteps_ZeroSteps_StillWritesSyncLogAndIncrementsVersion`**：stubUserRepo / stubStepStepAccountRepo 全合法 → service.GrantSteps(ctx, 1001, **0**) 返 err=nil；**断言**：accRepo.updateCalls=1 + accRepo.lastUpdateDelta=**0**；logRepo.createCalls=1 + logRepo.lastCreated.AcceptedDeltaSteps=**0**（验证审计纪律 —— 即便 0 也写一行）

5. **`TestDevStepService_GrantSteps_UpdateBalanceDBError_Returns1009`**：stubUserRepo / stubStepStepAccountRepo.findByUserIDFn 全合法 / stubStepStepAccountRepo.updateBalanceFn 返 simulated DB error → service 返 *AppError(Code=1009)；logRepo.createCalls=0（step 3 失败，step 4 不执行）

6. **加分 case `TestDevStepService_GrantSteps_NegativeSteps_Panics`**（验防御性 panic）：直接 service.GrantSteps(ctx, 1001, -1) → defer recover 应捕获 panic with msg "steps must be >= 0"；accRepo.updateCalls=0 + logRepo.createCalls=0（panic 在 txMgr.WithTx 之前）

**关键约束**：

- 6 case 命名前缀 `TestDevStepService_GrantSteps_<场景>` 一目了然
- HappyPath case **必须**断 `Source=2`（admin_grant；§6.6 钦定）+ `MotionState=1` + `ClientTotalSteps=0` + `ClientTs=0` + `SyncDate` 是 UTC 今日 YYYY-MM-DD —— 这是本 story 的核心契约断言
- 每 case 用**独立 stub instance**（与 7.3 / 7.4 同约束 —— 避免计数器串扰）
- ZeroSteps case 强断 `lastCreated.AcceptedDeltaSteps == 0` —— 验证 dev grant 0 步也走完整 4 步（**非** short-circuit）；与 7.3 SyncSteps capExceeded delta=0 仍写日志同模式
- NegativeSteps panic case 用 `defer func() { if r := recover(); r == nil { t.Fatal("expected panic") } }()` —— 防 service 漏防御
- stubUserRepo 必须在本文件新建（7.3 step_service_test.go 没用 userRepo）

**AC6 — handler 单测 ≥4 case（stub service + 测试 router；新文件 `dev_steps_handler_test.go`）**

新建 `server/internal/app/http/handler/dev_steps_handler_test.go`：

**stub 设计**（**新建** `stubDevStepService`，与 stubStepService 独立）：

```go
package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/huing/cat/server/internal/app/http/handler"
	"github.com/huing/cat/server/internal/app/http/middleware"
	apperror "github.com/huing/cat/server/internal/pkg/errors"
	"github.com/huing/cat/server/internal/service"
)

type stubDevStepService struct {
	grantStepsFn func(ctx context.Context, userID uint64, steps int32) error
}

func (s *stubDevStepService) GrantSteps(ctx context.Context, userID uint64, steps int32) error {
	return s.grantStepsFn(ctx, userID, steps)
}

// newDevStepsHandlerRouter 构造 handler test router。
//
// **关键差异 vs newStepsHandlerRouter**：dev 端点不挂 mock auth middleware（dev 不要求 auth）。
// 仅挂 ErrorMappingMiddleware（c.Error 写 envelope 必需）。
func newDevStepsHandlerRouter(svc service.DevStepService) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(middleware.ErrorMappingMiddleware())
	h := handler.NewDevStepsHandler(svc)
	r.POST("/dev/grant-steps", h.PostGrantSteps)
	return r
}
```

**必须覆盖 4 case**（前缀 `TestDevStepsHandler_PostGrantSteps_`）：

1. **`TestDevStepsHandler_PostGrantSteps_HappyPath_ReturnsAck`**：合法 body `{"userId":1001,"steps":5000}` → stub service 返 nil → 200 + envelope.code=0 + data.userId=1001 + data.grantedSteps=5000；stub service 内部 if userID != 1001 || steps != 5000 → t.Errorf 验透传

2. **`TestDevStepsHandler_PostGrantSteps_UserNotFound_Forwards1003_HTTP200`**：stub service 返 `*apperror.AppError(ErrResourceNotFound, "资源不存在")` → handler `c.Error` → middleware envelope code=1003，HTTP 200（业务码与 HTTP status 正交）

3. **`TestDevStepsHandler_PostGrantSteps_NegativeSteps_Returns1002_NoServiceCall`**：body `{"userId":1001,"steps":-1}` → handler 校验失败返 1002 + message="steps 不能为负数"；stub service.grantStepsFn 内 t.Errorf("should NOT be called")（验证 handler 拦截在 service 之前）

4. **`TestDevStepsHandler_PostGrantSteps_DBBusy_Forwards1009_HTTP500`**：stub service 返 `*apperror.AppError(ErrServiceBusy, "服务繁忙")` → middleware envelope code=1009，**HTTP 500**（ErrorMappingMiddleware 钦定 1009 走 500）

5. **加分 case `TestDevStepsHandler_PostGrantSteps_MissingUserID_Returns1002`**：body `{"steps":5000}`（无 userId）→ ShouldBindJSON 拦 binding:"required" → 1002 + message 含 "userId"

6. **加分 case `TestDevStepsHandler_PostGrantSteps_UserIDZero_Returns1002`**：body `{"userId":0,"steps":5000}` → handler 显式校验 0 → 1002 + message="userId 必须 > 0"

**关键约束**：

- 6 case 命名前缀 `TestDevStepsHandler_PostGrantSteps_<场景>` —— 与 PostSync / GetAccount case 命名同风格
- HappyPath stub 内 `if userID != 1001 || steps != 5000 { t.Errorf(...) }` 验透传
- NegativeSteps case **必须**用 stub.grantStepsFn 内 t.Errorf 兜底验"handler 拦在 service 之前" —— 防 future handler 误删 steps<0 校验
- DBBusy case 验 HTTP **500**（不是 200）—— 1009 是唯一走非 200 的业务码
- 测试 router**不**挂 mock auth middleware —— dev 路径无 auth；与 stepsHandler 测试模式区分

**AC7 — devtools 框架测试扩展（修改 `internal/app/bootstrap/router_dev_test.go` + `internal/app/http/devtools/devtools_test.go`）**

**修改 `router_dev_test.go`** 末尾追加 1 case：

```go
// TestRouter_DevGrantSteps_RegisteredWhenDevModeEnabled 验证 Story 7.5 dev grant 端点的 wire 链路：
//   - BUILD_DEV=true + Deps 完整 → /dev/grant-steps 路由存在 → POST 走 handler 链
//   - BUILD_DEV="" → /dev/grant-steps Gin NoRoute 404
//
// 因为 wire 涉及 Deps.GormDB / TxMgr 必填（NewDevStepService 要 repo），本测试用真实
// gorm + sqlmock 注入 Deps 不现实；改用**简化路径**：BUILD_DEV=true + Deps{} 零值 →
// devStepsHandler 保持 nil → devtools.Register 跳过 /dev/grant-steps 路由 → 404。
// 这验证"nil-tolerant"路径；真实 wire 链路（dev handler 真被调）由 dev_steps_handler_test
// 单测 + dev_step_service_integration_test 集成测试覆盖。
func TestRouter_DevGrantSteps_NilHandlerSkipsRoute(t *testing.T) {
	t.Setenv("BUILD_DEV", "true")
	gin.SetMode(gin.TestMode)
	r := NewRouter(Deps{}) // 零值 deps → devStepsHandler 保持 nil

	// /dev/ping-dev 应该正常注册（Register 内 ping-dev 不依赖 devStepsHandler）
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/dev/ping-dev", nil))
	if w.Code != http.StatusOK {
		t.Errorf("/dev/ping-dev should be 200 when BUILD_DEV=true; got %d", w.Code)
	}

	// /dev/grant-steps 应该跳过注册（devStepsHandler nil） → Gin NoRoute 404
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, httptest.NewRequest(http.MethodPost, "/dev/grant-steps", strings.NewReader(`{"userId":1,"steps":5000}`)))
	if w2.Code != http.StatusNotFound {
		t.Errorf("/dev/grant-steps with nil handler should be 404; got %d body=%s", w2.Code, w2.Body.String())
	}
}
```

**修改 `devtools_test.go`** 末尾追加 1 case：

```go
// TestRegister_BuildDevTrue_GrantStepsRegisteredWhenHandlerProvided 验证 Story 7.5 路由注册：
//   - BUILD_DEV=true + 传入非 nil DevStepsHandler → /dev/grant-steps 路由存在
//   - 验路由存在的方式：ServeHTTP 应该走到 handler 而非 NoRoute；用 stub handler 标志位验证
func TestRegister_BuildDevTrue_GrantStepsRegisteredWhenHandlerProvided(t *testing.T) {
	t.Setenv("BUILD_DEV", "true")

	called := false
	stubHandler := devtools.DevStepsHandlerFunc(func(c *gin.Context) {
		called = true
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	r := newEngine()
	r.Use(middleware.ErrorMappingMiddleware()) // DevOnlyMiddleware 兜底链路依赖
	devtools.Register(r, stubHandler)

	w := doPost(r, "/dev/grant-steps", `{"userId":1,"steps":5000}`)

	if w.Code != http.StatusOK {
		t.Errorf("/dev/grant-steps should be 200 when handler registered; got %d body=%s", w.Code, w.Body.String())
	}
	if !called {
		t.Errorf("handler should be called; got called=false (路由未注册或被 NoRoute 拦截)")
	}
}

// TestRegister_BuildDevTrue_GrantStepsSkippedWhenHandlerNil 验证 nil-tolerant：
//   - BUILD_DEV=true + 传入 nil → /dev/ping-dev 仍注册，/dev/grant-steps 跳过 → 404
func TestRegister_BuildDevTrue_GrantStepsSkippedWhenHandlerNil(t *testing.T) {
	t.Setenv("BUILD_DEV", "true")

	r := newEngine()
	devtools.Register(r, nil) // nil handler

	// ping-dev 仍应注册
	w1 := doGet(r, "/dev/ping-dev")
	if w1.Code != http.StatusOK {
		t.Errorf("/dev/ping-dev should be 200; got %d", w1.Code)
	}

	// grant-steps 应跳过注册 → Gin NoRoute 404
	w2 := doPost(r, "/dev/grant-steps", `{}`)
	if w2.Code != http.StatusNotFound {
		t.Errorf("/dev/grant-steps with nil handler should be 404; got %d", w2.Code)
	}
}
```

**辅助 helper**（追加到 devtools_test.go 已有 helper 段）：

```go
// doPost 帮助函数：对给定 engine 发一次 POST 请求，返回 recorder。
func doPost(r *gin.Engine, path string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

// DevStepsHandlerFunc 是 devtools.DevStepsHandler interface 的函数适配器（仅供测试用）。
//
// 实际生产 handler 是 *handler.DevStepsHandler（struct）；测试中用 func 包装更简洁。
type DevStepsHandlerFunc func(c *gin.Context)

func (f DevStepsHandlerFunc) PostGrantSteps(c *gin.Context) { f(c) }
```

**关键约束**：

- router_dev_test.go 验证"nil-tolerant + 全 wire 链路"
- devtools_test.go 验证"路由注册 / 跳过"决策点
- 两处都用最小 stub handler（不引入业务 handler 实例 → 避免 bootstrap import）
- DevStepsHandlerFunc adapter 只在 devtools_test.go（测试包）暴露 —— **不**进 production code
- 整个 devtools_test.go 文件 build tag 仍 `!devtools`（与 1.6 既有约束）

**AC8 — 集成测试 1 case（dockertest，新文件 `dev_step_service_integration_test.go`）**

新建 `server/internal/service/dev_step_service_integration_test.go`：

```go
//go:build integration

package service_test

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/huing/cat/server/internal/infra/config"
	"github.com/huing/cat/server/internal/infra/db"
	"github.com/huing/cat/server/internal/repo/mysql"
	"github.com/huing/cat/server/internal/repo/tx"
	"github.com/huing/cat/server/internal/service"
)

// buildDevStepServiceIntegration: 起容器 → migrate → 装配 svc + 返清理 closure。
//
// 与 buildStepServiceIntegration (7.3) 的区别：本 helper 还构造 userRepo（dev grant 需要 FindByID）。
func buildDevStepServiceIntegration(t *testing.T) (svc service.DevStepService, sqlDB *sql.DB, cleanup func()) {
	t.Helper()

	dsn, dockerCleanup := startMySQL(t)
	runMigrations(t, dsn)

	cfg := config.MySQLConfig{
		DSN:                dsn,
		MaxOpenConns:       10,
		MaxIdleConns:       2,
		ConnMaxLifetimeSec: 60,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	gormDB, err := db.Open(ctx, cfg)
	if err != nil {
		dockerCleanup()
		t.Fatalf("db.Open: %v", err)
	}

	txMgr := tx.NewManager(gormDB)
	userRepo := mysql.NewUserRepo(gormDB)
	stepAccountRepo := mysql.NewStepAccountRepo(gormDB)
	stepSyncLogRepo := mysql.NewStepSyncLogRepo(gormDB)

	svc = service.NewDevStepService(txMgr, userRepo, stepAccountRepo, stepSyncLogRepo)

	rawDB, err := gormDB.DB()
	if err != nil {
		dockerCleanup()
		t.Fatalf("gormDB.DB(): %v", err)
	}

	cleanup = func() {
		_ = rawDB.Close()
		dockerCleanup()
	}
	return svc, rawDB, cleanup
}

// TestDevStepServiceIntegration_GrantSteps_AppliesAccountAndAuditLog 验证 epics.md §Story 7.5 行 1442 钦定：
// 创建用户 → /dev/grant-steps 加 5000 → /steps/account 返回 available=5000。
//
// 本 case 直接调 svc.GrantSteps（绕过 handler）+ 直接 SELECT 验账户三档值 + sync_log 写入；
// 完整 HTTP 链路（含 dev mode 闸门）由 dev_steps_handler_test 单测 + router_dev_test 覆盖。
func TestDevStepServiceIntegration_GrantSteps_AppliesAccountAndAuditLog(t *testing.T) {
	svc, sqlDB, cleanup := buildDevStepServiceIntegration(t)
	defer cleanup()

	const userID = uint64(1)
	insertUser(t, sqlDB, userID, "uid-dev-grant-1", "用户DEV", "")
	insertStepAccount(t, sqlDB, userID, 0, 0, 0)

	ctx := context.Background()

	// 1. dev grant 5000
	if err := svc.GrantSteps(ctx, userID, 5000); err != nil {
		t.Fatalf("GrantSteps 5000: %v", err)
	}

	// 2. 验 user_step_accounts 三档值（total / available 各 +5000；consumed 不变）
	total, avail, consumed, version := fetchStepAccount(t, sqlDB, userID)
	if total != 5000 || avail != 5000 || consumed != 0 {
		t.Errorf("after grant: total=%d available=%d consumed=%d, want 5000/5000/0", total, avail, consumed)
	}
	if version != 1 {
		t.Errorf("version = %d, want 1 (initial 0 + 1)", version)
	}

	// 3. 验 user_step_sync_logs 写入了 1 行 source=2 admin_grant
	syncDate := time.Now().UTC().Format("2006-01-02")
	if got := countSyncLogs(t, sqlDB, userID, syncDate); got != 1 {
		t.Errorf("sync_log count for today = %d, want 1", got)
	}

	// 4. 验 sync_log 字段（source / accepted_delta / motion_state / client_total / client_ts）
	row := sqlDB.QueryRow(
		`SELECT source, accepted_delta_steps, motion_state, client_total_steps, client_ts FROM user_step_sync_logs WHERE user_id = ? AND sync_date = ? ORDER BY id DESC LIMIT 1`,
		userID, syncDate,
	)
	var (
		source       int8
		acceptedDelta int32
		motionState  int8
		clientTotal  uint64
		clientTs     uint64
	)
	if err := row.Scan(&source, &acceptedDelta, &motionState, &clientTotal, &clientTs); err != nil {
		t.Fatalf("scan sync_log: %v", err)
	}
	if source != 2 {
		t.Errorf("source = %d, want 2 (admin_grant; §6.6 钦定)", source)
	}
	if acceptedDelta != 5000 {
		t.Errorf("accepted_delta_steps = %d, want 5000", acceptedDelta)
	}
	if motionState != 1 {
		t.Errorf("motion_state = %d, want 1 (stationary_or_unknown 中性值)", motionState)
	}
	if clientTotal != 0 {
		t.Errorf("client_total_steps = %d, want 0 (dev grant 无客户端累计语义)", clientTotal)
	}
	if clientTs != 0 {
		t.Errorf("client_ts = %d, want 0 (dev grant 无客户端时刻语义)", clientTs)
	}

	// 5. 加分：再 grant 3000 → total=8000 / available=8000 / version=2 / sync_log count=2
	if err := svc.GrantSteps(ctx, userID, 3000); err != nil {
		t.Fatalf("GrantSteps 3000: %v", err)
	}
	total2, avail2, consumed2, version2 := fetchStepAccount(t, sqlDB, userID)
	if total2 != 8000 || avail2 != 8000 || consumed2 != 0 {
		t.Errorf("after second grant: total=%d available=%d consumed=%d, want 8000/8000/0", total2, avail2, consumed2)
	}
	if version2 != 2 {
		t.Errorf("version after second = %d, want 2", version2)
	}
	if got := countSyncLogs(t, sqlDB, userID, syncDate); got != 2 {
		t.Errorf("sync_log count after second = %d, want 2", got)
	}
}

// TestDevStepServiceIntegration_GrantSteps_UserNotFound_Returns1003 验证 epics.md §Story 7.5 行 1439 钦定：
// dev mode + 用户不存在 → 返回 1003。
func TestDevStepServiceIntegration_GrantSteps_UserNotFound_Returns1003(t *testing.T) {
	svc, sqlDB, cleanup := buildDevStepServiceIntegration(t)
	defer cleanup()
	_ = sqlDB

	const nonExistentUserID = uint64(99999)
	err := svc.GrantSteps(context.Background(), nonExistentUserID, 5000)
	if err == nil {
		t.Fatal("GrantSteps should fail when user does not exist")
	}
	if got := apperror.Code(err); got != apperror.ErrResourceNotFound {
		t.Errorf("apperror.Code = %d, want %d (1003 ErrResourceNotFound)", got, apperror.ErrResourceNotFound)
	}
}
```

**关键约束**：

- 集成测试**必须**复用 7.3 已建的 `startMySQL` / `runMigrations` / `insertUser` / `insertStepAccount` / `fetchStepAccount` / `countSyncLogs` helper —— **不**重新建一套
- 本机 Windows docker 不可用 → t.Skip（4.3 graceful skip 模式；4.6-7.4 已用同模式）—— `startMySQL` 内部已有 skip 逻辑，本 case 自动继承
- HappyPath case **必须**断 `source=2` + `motion_state=1` + `client_total_steps=0` + `client_ts=0` —— 这些是 dev grant 与 7.3 healthkit grant（source=1）的核心字段差异
- 加分"再 grant 3000"段验证幂等性（dev 端点是"故意可重复"语义；不像 chest/open 不能重复扣）+ version 递增正确
- 第二个 case `UserNotFound_Returns1003` 验 epics.md AC 钦定的 edge case（service 单测已用 stub 覆盖；集成补一次 MySQL 真返 ErrUserNotFound 链路）
- **不**新增独立的 `dev_steps_handler_integration_test.go`（HTTP 链路由 router_dev_test + dev_steps_handler_test 覆盖）
- 集成 ≥1 case AC 满足；本 story 写 2 case（HappyPath + UserNotFound）超额，时间成本可控

**AC9 — `bash scripts/build.sh` 全量绿**

完成后必须能跑通：

```bash
bash scripts/build.sh                      # vet + build → no failures
bash scripts/build.sh --test               # 全单测过（service 6 + handler 6 + devtools 2 = 14 新 case + 既有全过）
bash scripts/build.sh --race --test        # CI Linux 必过；Windows race skip 不阻塞
bash scripts/build.sh --integration        # 既有集成 + 新 2 case；docker 不可用 → t.Skip
bash scripts/build.sh --devtools           # 验证 build tag 路径不出错（构造 build/catserver-dev[.exe]）
```

**关键约束**：

- 本 story 引入的新代码全部含单测；`go test ./internal/service/... -run TestDevStepService -v` 必须 6 个 case 全过
- `go test ./internal/app/http/handler/... -run TestDevStepsHandler -v` 必须 6 个 case 全过
- `go test ./internal/app/http/devtools/... -run TestRegister_BuildDevTrue_GrantSteps -v` 必须 2 个 case 全过
- `go test ./internal/app/bootstrap/... -run TestRouter_DevGrantSteps -v` 必须 1 个 case 全过
- `--race` 在 Windows skip（ADR-0001 §3.5）；CI Linux 跑
- `--devtools` 必须能通过（forceDevEnabled=true 路径不破 + 不引入 build error）
- **不**改 `scripts/build.sh` 自身

**AC10 — 验证清单（人工 + 自动化）**

完成后**人工**核对以下 10 项（结果记到 Completion Notes List）：

| # | 验证项 | 验证方式 |
|---|---|---|
| 1 | service 层 GrantSteps 流程严格按 epics.md §Story 7.5 钦定 4 步事务：FindByID(user) → FindByUserID(account) → UpdateBalance(+steps, version) → Create sync_log source=2 | Read `dev_step_service.go` GrantSteps 实装段 |
| 2 | sync_log.source = 2（admin_grant；§6.6 钦定）；motion_state=1 / client_total=0 / client_ts=0 / sync_date = server today UTC | Read `dev_step_service.go` log 拼装段 |
| 3 | handler `PostGrantStepsRequest` 用 *uint64 / *int32 指针 + binding:"required"；userId=0 / steps<0 显式 1002 拦截 | Read `dev_steps_handler.go` PostGrantSteps |
| 4 | handler **不**调 `c.Get(UserIDKey)`（dev 路径无 auth 中间件）；userID 全靠 body | Read `dev_steps_handler.go` |
| 5 | devtools.Register 签名扩 `(r, devStepsHandler DevStepsHandler)`；nil 时跳过 grant-steps 路由（保留 ping-dev） | Read `devtools.go` Register 实装 |
| 6 | router.go wire `var devStepsHandler` 提前声明 nil + if 块内构造 + Register 留 if 块外透传 | Read `router.go` diff |
| 7 | service 单测 HappyPath 强断 lastCreated.Source=2 + MotionState=1 + ClientTotalSteps=0 + ClientTs=0 + SyncDate matches today | Read `dev_step_service_test.go` HappyPath case |
| 8 | service 单测 NegativeSteps case 用 defer recover 验防御性 panic | Read `dev_step_service_test.go` NegativeSteps case |
| 9 | handler 单测 NegativeSteps case 验 stub.grantStepsFn 主动 t.Errorf("should NOT be called")（验拦截在 service 之前） | Read `dev_steps_handler_test.go` NegativeSteps case |
| 10 | `bash scripts/build.sh --test` 全绿；`bash scripts/build.sh --devtools` 不破；`git status --short` 改动文件清单匹配预期范围（5 新 + 3-4 改） | bash 实跑 + git status |

**AC11 — 不 commit（流水线由 epic-loop 下游收口）**

epics.md §Story 7.5 AC 钦定"≥4 单测 + 集成测试覆盖"，**不**钦定"git commit 单独提交"。本 story 是 server 业务代码 story，commit 由 epic-loop 流水线在下游 fix-review / story-done sub-agent 阶段统一收口。

- 本 story 的 dev workflow **不** commit / **不** push
- commit message 模板（story-done 阶段使用）：

  ```text
  feat(dev-grant-steps): POST /dev/grant-steps dev 端点（Story 7.5）

  - service dev_step_service.GrantSteps 实装：事务内 FindByID + FindByUserID +
    UpdateBalance(+steps) + Create sync_log source=2 admin_grant；UserNotFound /
    StepAccountNotFound → 1003；其他 DB 错 → 1009
  - handler dev_steps_handler.PostGrantSteps + PostGrantStepsRequest（userId / steps
    指针类型 + binding:"required"；userId=0 / steps<0 → 1002）+
    postGrantStepsResponseDTO 简单 ack 结构
  - devtools.Register 签名扩 (r, devStepsHandler DevStepsHandler)；nil-tolerant 跳过
    grant-steps 路由
  - bootstrap/router.go wire devStepSvc + devStepsHandler；通过 var 提前声明 + if 块
    内构造 + Register 留外保持原结构
  - 单测 14 case（service 6 + handler 6 + devtools 2）+ 集成测试 2 case（HappyPath
    + UserNotFound）

  依据 epics.md §Story 7.5 + 数据库设计 §6.6 source=2 + Story 1.6 devtools 框架。

  Story: 7-5-dev-端点-post-dev-grant-steps
  ```

- commit hash 待 story-done 阶段产生后回填到本文件

## Tasks / Subtasks

- [x] **Task 1（AC1）**：新建 `internal/service/dev_step_service.go` —— DevStepService interface + impl
  - [x] 1.1 Read `step_service.go` 完整文件理解 service 模式（事务边界 / repo 调用 / 错误翻译）
  - [x] 1.2 Read `home_service.go` 复习"事务外纯读"vs"事务内多步骤"的差异（通过 step_service.go GetAccount 实装已理解）
  - [x] 1.3 Read `errors.go` (mysql) 确认 `ErrUserNotFound` / `ErrStepAccountNotFound` 哨兵 + `errors.Is` 使用模式
  - [x] 1.4 Write 新文件 `dev_step_service.go` —— DevStepService interface + devStepServiceImpl + NewDevStepService + GrantSteps 实装（事务内 4 步）
  - [x] 1.5 Read 回检：(1) source=2 写死；(2) motion_state=1 / client_total=0 / client_ts=0 中性值；(3) sync_date = time.Now().UTC().Format("2006-01-02")；(4) steps<0 panic 防御；(5) ErrUserNotFound / ErrStepAccountNotFound → 1003；其他 → 1009；(6) 不接 cap / config / envName

- [x] **Task 2（AC2）**：新建 `internal/app/http/handler/dev_steps_handler.go` —— DevStepsHandler + DTO + handler
  - [x] 2.1 Read `steps_handler.go` 完整文件复习 handler 模式（指针类型 + binding:"required" + c.Error + ctx 传播）
  - [x] 2.2 Read `home_handler.go` 复习不依赖 body / 不依赖 c.Get(UserIDKey) 的 handler 反模式（dev 不用 c.Get；通过 steps_handler.go 已掌握 c.Get 模式后镜像反向）
  - [x] 2.3 Write 新文件 `dev_steps_handler.go` —— DevStepsHandler + NewDevStepsHandler + PostGrantStepsRequest + PostGrantSteps + postGrantStepsResponseDTO
  - [x] 2.4 Read 回检：(1) UserID / Steps 指针类型（无 binding:"required" —— 与 7.3 PostSyncRequest 同模式：validator/v10 把 0 视为 zero value 会误判 required，改用手动 nil 校验）；(2) userId=0 / steps<0 显式 1002；(3) 不调 c.Get(UserIDKey)；(4) c.Error + return；(5) response 返简单 ack；(6) postGrantStepsResponseDTO 不返当前账户值

- [x] **Task 3（AC3）**：扩 `internal/app/http/devtools/devtools.go` —— DevStepsHandler interface + Register 签名扩
  - [x] 3.1 Read `devtools.go` 完整文件理解现有 Register / DevOnlyMiddleware / IsEnabled
  - [x] 3.2 Edit 在 devtools.go 加 `DevStepsHandler interface { PostGrantSteps(c *gin.Context) }` 类型
  - [x] 3.3 Edit 改 `Register` 签名为 `Register(r *gin.Engine, devStepsHandler DevStepsHandler)`，在 `g.GET("/ping-dev", PingDevHandler)` 之后追加 `if devStepsHandler != nil { g.POST("/grant-steps", devStepsHandler.PostGrantSteps) }`
  - [x] 3.4 Read 回检：(1) DevStepsHandler interface 解耦避免 import cycle；(2) nil-tolerant 跳过 grant-steps；(3) ping-dev 仍在；(4) build tag 不影响（forceDevEnabled 不变）

- [x] **Task 4（AC4）**：扩 `internal/app/bootstrap/router.go` —— wire dev service + handler + Register 透传
  - [x] 4.1 Read 现有 `router.go` 理解 if 块结构 + repo 复用模式
  - [x] 4.2 Edit 在 NewRouter 函数顶部 `r.GET("/metrics", ...)` 之后**先**声明 `var devStepsHandler *handler.DevStepsHandler // Story 7.5`
  - [x] 4.3 Edit 在 `if deps.GormDB != nil && ...` 块内、`stepsHandler` 构造后追加 `devStepSvc := service.NewDevStepService(...)` + `devStepsHandler = handler.NewDevStepsHandler(devStepSvc)`
  - [x] 4.4 Edit 改 `devtools.Register(r)` 调用为 `devtools.Register(r, devStepsHandler)`（**关键 nil 陷阱**：直接传 `*handler.DevStepsHandler(nil)` 给 interface 是 typed-nil，**非** nil interface；显式分支 `if devStepsHandler == nil { Register(r, nil) } else { Register(r, devStepsHandler) }` 修复）
  - [x] 4.5 Read 回检：(1) deps 完整时 devStepsHandler 非 nil；(2) deps 零值（单测 NewRouter(Deps{})）时保持 nil；(3) Register 留 if 块外；(4) Deps struct 未改

- [x] **Task 5（AC5）**：新建 `internal/service/dev_step_service_test.go` —— ≥6 case
  - [x] 5.1 Read `step_service_test.go` 完整文件复习 stub 模式（stubStepStepAccountRepo / stubStepSyncLogRepo / stubStepTxMgr 已有可复用；stubUserRepo 在 auth_service_test.go 已存在同 package 可复用）
  - [x] 5.2 Write 新文件 `dev_step_service_test.go` —— 复用 auth_service_test.go 的 stubUserRepo + buildDevStepService helper + 6 case
  - [x] 5.3 Read 回检：(1) HappyPath 强断 source=2 / motion_state=1 / sync_date 含今日；(2) ZeroSteps 走完整 4 步；(3) NegativeSteps defer recover；(4) UserNotFound → 1003 + UpdateBalance / Create 0 调用；(5) StepAccountNotFound → 1003；(6) DBError → 1009

- [x] **Task 6（AC6）**：新建 `internal/app/http/handler/dev_steps_handler_test.go` —— ≥6 case
  - [x] 6.1 Read `steps_handler_test.go` 完整文件复习 stubStepService / newStepsHandlerRouter 模式
  - [x] 6.2 Write 新文件 `dev_steps_handler_test.go` —— stubDevStepService + newDevStepsHandlerRouter（**不**挂 mock auth）+ 7 case（HappyPath / UserNotFound / NegativeSteps / DBBusy / MissingUserID / UserIDZero / MissingSteps）
  - [x] 6.3 Read 回检：(1) HappyPath stub 内验 userID / steps 透传；(2) NegativeSteps stub.grantStepsFn 主动 t.Errorf；(3) UserNotFound → HTTP 200 + envelope 1003；(4) DBBusy → HTTP 500 + envelope 1009；(5) MissingUserID / UserIDZero → 1002

- [x] **Task 7（AC7）**：扩 `router_dev_test.go` + `devtools_test.go`
  - [x] 7.1 Read `router_dev_test.go` 复习 BUILD_DEV setenv + ServeHTTP 模式
  - [x] 7.2 Edit 末尾追加 `TestRouter_DevGrantSteps_NilHandlerSkipsRoute`（NewRouter(Deps{}) → grant-steps 404；ping-dev 200）
  - [x] 7.3 Read `devtools_test.go` 复习 newEngine / doGet helper
  - [x] 7.4 Edit `devtools_test.go` 末尾追加 `doPost` helper + `devStepsHandlerFunc` adapter + 2 case（GrantStepsRegisteredWhenHandlerProvided / GrantStepsSkippedWhenHandlerNil）+ 把既有 6 个 case 的 `devtools.Register(r)` 调用全部改为 `devtools.Register(r, nil)`（Register 签名兼容更新）

- [x] **Task 8（AC8）**：新建 `internal/service/dev_step_service_integration_test.go` —— 2 case
  - [x] 8.1 Read `step_service_integration_test.go` 复习 buildStepServiceIntegration 模式 + helper 复用（startMySQL / runMigrations / insertUser / insertStepAccount / fetchStepAccount / countSyncLogs）
  - [x] 8.2 Write 新文件 `dev_step_service_integration_test.go` —— buildDevStepServiceIntegration + 2 case（HappyPath_AppliesAccountAndAuditLog + UserNotFound_Returns1003）
  - [x] 8.3 验证本机 Windows docker 不可用 → t.Skip 不阻塞（startMySQL 内已有 skip 逻辑；本地实跑 `bash scripts/build.sh --integration` BUILD SUCCESS）

- [x] **Task 9（AC9 / AC10）**：全量验证
  - [x] 9.1 `bash scripts/build.sh`（vet + build）过 → BUILD SUCCESS
  - [x] 9.2 `bash scripts/build.sh --test` 全绿（实际新增 16 case：service 6 + handler 7 + devtools 2 + bootstrap 1 = 16）
  - [ ] 9.3 `bash scripts/build.sh --race --test`（Windows race skip ok；本地未跑，CI Linux 跑） —— 不阻塞；CI 兜底
  - [x] 9.4 `bash scripts/build.sh --integration`（docker 不可用 → t.Skip ok；BUILD SUCCESS）
  - [x] 9.5 `bash scripts/build.sh --devtools`（验证 build tag 路径不破）
  - [x] 9.6 `git status --short` 改动文件清单核对（实际新 5 文件 + 改 3 文件 + sprint-status + story 文件 = 10 文件）
  - [x] 9.7 在下方 Completion Notes List 勾选 AC10 验证清单 10 项

- [x] **Task 10（AC11）**：本 story 不做 git commit
  - [x] 10.1 epic-loop 流水线约束：dev-story 阶段不 commit；由下游 fix-review / story-done sub-agent 收口
  - [x] 10.2 commit message 模板保留在 story 文件中
  - [ ] 10.3 commit hash 待 story-done 阶段回填 —— story-done 阶段执行

## Dev Notes

### 关键设计原则

1. **dev 端点独立 service / handler**（不复用 SyncSteps）：dev grant 的产品语义是"绕过常规约束 +N 步"——SyncSteps 含 5+ 约束分支（rawDelta = max(0, ...)；single_sync_cap 5000 截断；daily_cap 50000 封顶；SUM 兜底乱序；source=1 写死）全不适用。强行复用要在 SyncSteps 加 "is_dev" flag short-circuit 5 个分支，反模式。独立 service 是"按职责而非按表分服务"的合理切分（与 NewHomeService / NewStepService / NewAuthService 的"业务模块独立"思路一致）。

2. **devtools 框架解耦 vs 业务 dev 端点扩展模式**：Story 1.6 钦定 devtools 包"只做框架（Register / DevOnlyMiddleware / IsEnabled / 启停判定）"；业务 dev 端点（grant-steps / 等）写在业务层（service / handler）→ devtools 只追加路由注册一行 + 通过 interface（DevStepsHandler）透传业务 handler。这避免：(a) devtools 包反向 import handler 包导致 import cycle；(b) devtools 包持续膨胀承载越来越多业务逻辑；(c) 未来加 /dev/grant-cosmetic-batch (Epic 20.8) 时 devtools 包再加一个 PostGrantCosmetic handler，违反 SRP。

3. **接口契约：simple ack response，不返当前账户值**：postGrantStepsResponseDTO 返 `{userId, grantedSteps}` 简单 ack，不嵌套 stepAccount。理由：(a) 端点单一职责（grant 只负责"做了"，account 只负责"读了"；调用方串调即可）；(b) 未来加批量 grant（一次给多个 user 加步数）时 schema 难扩展（嵌套单 user 账户没意义）；(c) Story 9.1 e2e 验证场景 6 是"调 grant 后再调 GET /steps/account 验 +5000"——本 ack schema 与 e2e 契约一致。

4. **userId / steps 用 *指针 + binding:"required"**：与 7.3 PostSyncRequest ClientTotalSteps 同模式 ——必须区分"字段缺失"vs"显式传 0"。dev 端点没有 auth 中间件注入 userID，全靠 body；漏校验缺失字段会让"不传 userId 的请求"被静默接受为 userId=0 → service 走 ErrUserNotFound → 1003，错误码正确但延后到 service 层；用指针提前在 handler 拦截 1002，错误信息更精确。

5. **userId=0 显式拒（额外严格）**：MySQL users.id 是 AUTO_INCREMENT 从 1 起，userId=0 永远查不到 → service 自然返 ErrUserNotFound → 1003。但 handler 显式校验 `*req.UserID == 0` → 1002 + message="userId 必须 > 0"，让错误信息更精确（区分"字段非法"vs"用户不存在"）+ 早 fail（少一次 DB 查询）。

6. **steps=0 仍走完整 4 步事务**：与 7.3 SyncSteps capExceeded delta=0 仍写日志同模式 —— 审计纪律。dev grant 0 步是合法的（边界 case，可能是测试 fixture），写一行 sync_log 记录"有人调用了端点"对运维 / 测试调试有价值；version+1 也保持事务边界一致。

7. **steps<0 双重防御**：handler 1002 拦截（产品契约）+ service panic 防御（service 不该承担参数校验，但 prod 二进制不可达 dev service，panic 不会泄漏 prod；单测必须验 service panic 能拦"假设 handler 漏校验"场景）。这是"参数校验在 handler 层但 service 仍最后一道墙"的纵深防御。

8. **不接 idempotencyKey**：dev 端点的产品语义是"故意可重复"——同一 userId + steps 调 N 次应该 +N×steps，便于自动化测试堆余额。这与 chest/open（需要 idempotencyKey 防重复扣可用步数）相反。

9. **不接 rate_limit**：dev 路由组**不**走 RateLimitByIP / RateLimitByUserID 中间件 —— 自动化测试可能高频调本端点（如 fixture 阶段连续 grant 5 个 user × 各 3 次）；prod 二进制不可达；dev / 测试环境不需要保护。

10. **SyncDate 用 server today UTC**：dev 端点没有 client 时区概念（不像 7.3 SyncSteps 信任 client 时区算今天）；server 直接用 UTC today 写 sync_log 满足 NOT NULL 约束。这与 7.3 syncDate 字段语义一致 —— 字符串字面值无时区耦合（详见 mysql.StepSyncLog SyncDate 字段 doc）。

### 架构对齐

**领域模型层**（`docs/宠物互动App_总体架构设计.md`）：

- 步数账户是节点 3 核心可消费资产；dev grant 端点是"测试 / demo 数据准备"基础设施 —— 与业务接口（7.3 / 7.4）解耦但共享同一存储 / 同一 sync_log 审计模式
- "状态以 server 为准"原则：dev grant 后客户端调 GET /steps/account 拿权威值

**数据库层**（`docs/宠物互动App_数据库设计.md`）：

- §5.4 user_step_accounts：本 story 消费 FindByUserID + UpdateBalance（与 7.3 同）
- §5.5 user_step_sync_logs：本 story 消费 Create（与 7.3 同；只是 source 字段不同）
- §6.6 source 枚举：本 story 写 source=2 admin_grant（已钦定）
- §6.5 motion_state 枚举：本 story 写 motion_state=1 stationary_or_unknown（中性值）
- 索引：UpdateBalance 走 PK = user_id；Create 不依赖索引；FindByID 走 PK = id

**接口契约层**（`docs/宠物互动App_V1接口设计.md`）：

- V1 §1 节点 3 冻结声明：本 story 是 dev 端点，**不**进 V1 主清单（V1 §16 当前 V1 接口清单不收录）
- V1 §6.1.4 line 553 注释：已提及"dev grant 走 source=2，见 Story 7.5"——本 story 落地该提及
- 错误码：1002（参数错误）/ 1003（资源不存在）/ 1009（服务繁忙）—— 全沿用 V1 §3 通用错误码

**服务端架构层**（`docs/宠物互动App_Go项目结构与模块职责设计.md`）：

- §5.1 handler 层：本 story DevStepsHandler 严格按 handler 职责（参数校验 + DTO 转换 + 不接触 *gorm.DB）
- §5.2 service 层：本 story DevStepService 严格按 service 职责（业务规则 + 事务边界 + 错误翻译）
- §5.3 repo 层：本 story 仅消费已实装的 userRepo / stepAccountRepo / stepSyncLogRepo，**不**改 repo 层

**ADR 对齐**：

- ADR-0006 三层错误映射：repo 返哨兵（ErrUserNotFound / ErrStepAccountNotFound）→ service 翻译为 *AppError(1003 / 1009)→ handler c.Error + middleware envelope
- ADR-0007 ctx 传播：service / repo 第一参数 ctx；handler 用 `c.Request.Context()` 不直接传 *gin.Context；txMgr.WithTx 闭包内用 `txCtx`
- ADR-0006 单一 envelope 生产者：handler 一律 `c.Error + return`，由 ErrorMappingMiddleware 写 envelope

### 关于 Story 7.5 与 7.3 / 7.4 的关键差异

| 维度 | 7.3 POST /steps/sync | 7.4 GET /steps/account | 7.5 POST /dev/grant-steps |
|------|---------------------|-----------------------|---------------------------|
| 路由前缀 | /api/v1 | /api/v1 | /dev（dev 模式启用时才挂） |
| HTTP method | POST | GET | POST |
| body | 有（4 字段：syncDate / clientTotalSteps / motionState / clientTimestamp） | 无 | 有（2 字段：userId / steps） |
| auth 中间件 | 是（Bearer token） | 是（Bearer token） | **否**（dev 内部用） |
| rate_limit 中间件 | 是（60/min/userID） | 是（60/min/userID 共享） | **否**（dev 高频调用） |
| 事务 | 有（5 步：FindLatest + SUM + FindByUserID + UpdateBalance + Create） | 无（单表 SELECT） | 有（4 步：FindByID + FindByUserID + UpdateBalance + Create） |
| 防作弊 | 有（5000 / 50000 / SUM 兜底 / 倒退削成 0） | 无（GET 不写入） | **无**（dev 故意绕过） |
| sync_log.source | 1 (healthkit) | n/a | **2 (admin_grant)** |
| 响应 schema | 嵌套（data.stepAccount.*） | 扁平（data.*） | ack（data.{userId, grantedSteps}） |
| 错误码全集 | 1001 / 1002 / 1005 / 3001 / 1009 | 1001 / 1003 / 1005 / 1009 | 1002 / 1003 / 1009 |
| 端点物理可达性 | 永远 | 永远 | **仅 BUILD_DEV=true 或 -tags devtools** |
| 单元测试规模 | service 9 + handler 6 = 15 | service 4 + handler 4 = 8 | service 6 + handler 6 + devtools 2 + bootstrap 1 = 15 |
| 改动文件数 | ~13 文件 | 5-6 文件 | 5 新 + 3-4 改 = 8-9 文件 |

### Project Structure Notes

**与 `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 钦定的 server/ 工程结构对齐**：

```
server/
├─ internal/
│  ├─ app/
│  │  ├─ http/
│  │  │  ├─ handler/
│  │  │  │  ├─ steps_handler.go            # 7.3 / 7.4 已建；本 story 不动
│  │  │  │  ├─ dev_steps_handler.go        # 本 story 新建
│  │  │  │  └─ dev_steps_handler_test.go   # 本 story 新建
│  │  │  ├─ devtools/
│  │  │  │  ├─ devtools.go                 # 1.6 已建；本 story 改 Register 签名 + 加 grant-steps 路由
│  │  │  │  └─ devtools_test.go            # 1.6 已建；本 story 末尾追加 2 case
│  │  │  └─ middleware/                    # 已实装；本 story 不调（dev 路径无 auth / rate_limit）
│  │  └─ bootstrap/
│  │     ├─ router.go                      # 7.3 / 7.4 已 wire；本 story 加 devStepSvc / devStepsHandler / Register 透传
│  │     └─ router_dev_test.go             # 1.6 已建；本 story 末尾追加 1 case
│  ├─ service/
│  │  ├─ step_service.go                   # 7.3 / 7.4 已建；本 story 不动
│  │  ├─ dev_step_service.go               # 本 story 新建
│  │  ├─ dev_step_service_test.go          # 本 story 新建
│  │  └─ dev_step_service_integration_test.go  # 本 story 新建
│  ├─ repo/
│  │  └─ mysql/
│  │     ├─ user_repo.go                   # 已建；本 story 仅消费 FindByID（**不**改）
│  │     ├─ step_account_repo.go           # 7.3 已加 UpdateBalance；本 story 仅消费 FindByUserID + UpdateBalance（**不**改）
│  │     └─ step_sync_log_repo.go          # 7.3 已建；本 story 仅消费 Create（**不**改）
│  └─ pkg/
│     └─ errors/codes.go                   # 1002 / 1003 / 1009 已注册；本 story 仅**消费**
└─ migrations/                              # 0001-0006 已锁定；本 story **不**改
```

**变更范围（预期 git status 文件清单，5 新 + 3-4 改）**：

新建 5 文件：
1. `server/internal/service/dev_step_service.go`
2. `server/internal/service/dev_step_service_test.go`
3. `server/internal/service/dev_step_service_integration_test.go`
4. `server/internal/app/http/handler/dev_steps_handler.go`
5. `server/internal/app/http/handler/dev_steps_handler_test.go`

修改 3-4 文件：
6. `server/internal/app/http/devtools/devtools.go` — Register 签名扩 + DevStepsHandler interface + g.POST 路由注册
7. `server/internal/app/http/devtools/devtools_test.go` — 末尾追加 2 case + helper
8. `server/internal/app/bootstrap/router.go` — devStepSvc / devStepsHandler wire + Register 透传
9. `server/internal/app/bootstrap/router_dev_test.go` — 末尾追加 1 case

流程文件：
10. `_bmad-output/implementation-artifacts/sprint-status.yaml` — 7-5 状态 backlog → ready-for-dev → in-progress → review → done
11. `_bmad-output/implementation-artifacts/7-5-dev-端点-post-dev-grant-steps.md` — 本 story 文件本身

**未变更文件（明确 NOT touch；超出范围 → HALT）**：

- `server/internal/repo/mysql/*.go`（user_repo / step_account_repo / step_sync_log_repo / errors / 任一其他）
- `server/internal/service/step_service.go` / `step_service_test.go` / `step_service_integration_test.go`
- `server/internal/service/auth_service.go` / `home_service.go`
- `server/internal/app/http/handler/steps_handler.go` / `steps_handler_test.go`
- `server/internal/app/http/handler/auth_handler.go` / `home_handler.go`
- `server/internal/app/http/middleware/*.go`
- `server/internal/infra/config/*.go` / `loader.go` / 任一 `*.yaml`
- `server/internal/pkg/errors/codes.go`
- `server/cmd/server/main.go` / `internal/app/bootstrap/server.go`
- `migrations/*.sql`
- `docs/宠物互动App_*.md`（消费方）
- `_bmad-output/implementation-artifacts/decisions/*.md`（无新决策）

### References

**优先级 P0（必读）**：

- [Source: _bmad-output/planning-artifacts/epics.md#Story 7.5] — 本 story 钦定 AC（行 1423-1443）
- [Source: docs/宠物互动App_数据库设计.md#6.6 source 枚举] — source=2 admin_grant 钦定（行 765-770）
- [Source: docs/宠物互动App_数据库设计.md#5.4 user_step_accounts] — 表结构 + UpdateBalance 语义
- [Source: docs/宠物互动App_数据库设计.md#5.5 user_step_sync_logs] — sync_log 字段全集（含 source / motion_state / client_total_steps / client_ts）
- [Source: server/internal/app/http/devtools/devtools.go] — Story 1.6 devtools 框架（Register / DevOnlyMiddleware / IsEnabled / PingDevHandler）+ "业务 dev 端点扩展模式" 注释
- [Source: server/internal/service/step_service.go] — 7.3 / 7.4 已建 stepServiceImpl 模式（事务边界 / repo 调用 / 错误翻译；本 story 参考但**不**复用）
- [Source: server/internal/app/http/handler/steps_handler.go] — 7.3 / 7.4 已建 PostSync / GetAccount handler 模式（指针类型 + binding:"required" + c.Error + ctx 传播；本 story 参考但**不**复用）
- [Source: server/internal/app/bootstrap/router.go] — wire 模式 + nil-tolerant if 块（本 story 扩 devStepSvc + Register 透传）

**优先级 P1（参考）**：

- [Source: server/internal/repo/mysql/user_repo.go] — UserRepo.FindByID 实装 + ErrUserNotFound 哨兵
- [Source: server/internal/repo/mysql/step_account_repo.go] — StepAccountRepo.FindByUserID + UpdateBalance 实装
- [Source: server/internal/repo/mysql/step_sync_log_repo.go] — StepSyncLogRepo.Create 实装 + StepSyncLog struct 字段全集
- [Source: server/internal/repo/mysql/errors.go] — ErrUserNotFound / ErrStepAccountNotFound / ErrStepSyncLogNotFound 哨兵
- [Source: server/internal/pkg/errors/codes.go] — 错误码全集（ErrInvalidParam=1002 / ErrResourceNotFound=1003 / ErrServiceBusy=1009）
- [Source: server/internal/service/step_service_test.go] — 7.3 / 7.4 已建 stub 模式（stubStepStepAccountRepo / stubStepSyncLogRepo / stubStepTxMgr / failOnSyncLogStub；本 story 复用前三 + 不复用 failOnSyncLogStub —— dev grant 必须调 sync_log Create）
- [Source: server/internal/app/http/handler/steps_handler_test.go] — stubStepService + newStepsHandlerRouter 模式（本 story 新建独立 stubDevStepService + newDevStepsHandlerRouter 同模式）
- [Source: server/internal/service/step_service_integration_test.go] — buildStepServiceIntegration + insertUser / insertStepAccount / fetchStepAccount / countSyncLogs helper（本 story 复用 helper + 新建 buildDevStepServiceIntegration）
- [Source: server/internal/app/bootstrap/router_dev_test.go] — Story 1.6 dev 路由 wire 测试（BUILD_DEV setenv + ServeHTTP 模式；本 story 末尾加 1 case）
- [Source: server/internal/app/http/devtools/devtools_test.go] — Story 1.6 devtools 单测（newEngine / doGet / slogtest 模式；本 story 末尾加 2 case + doPost helper）
- [Source: _bmad-output/implementation-artifacts/decisions/0006-error-mapping.md] — ADR-0006 三层错误映射
- [Source: _bmad-output/implementation-artifacts/decisions/0007-context-propagation.md] — ADR-0007 ctx 传播

**优先级 P2（背景）**：

- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#5] — 分层职责定义
- [Source: docs/宠物互动App_总体架构设计.md] — "状态以 server 为准"原则
- [Source: _bmad-output/implementation-artifacts/1-6-dev-tools-框架.md] — Story 1.6 完整实装文档（双闸门 / DevOnlyMiddleware / 业务扩展模式钦定）
- [Source: _bmad-output/implementation-artifacts/7-3-post-steps-sync-接口-累计差值入账-service.md] — 7.3 完整实装文档（事务模式 / 单测 stub 模式参考）
- [Source: _bmad-output/implementation-artifacts/7-4-get-steps-account-接口.md] — 7.4 完整实装文档（最小化 service 扩 / handler 扩模式参考）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 9.1] — 节点 3 跨端 e2e 验证场景 6（行 1589）—— 本 story 是该 e2e 的 server 端实装

### Previous Story Intelligence（Story 1.6 / 7.3 / 7.4 关键交付物）

**Story 1.6 dev tools 框架交付物**（必读 `_bmad-output/implementation-artifacts/1-6-dev-tools-框架.md`）：

- **devtools 包定位**：只做"框架"（Register + DevOnlyMiddleware + 启停判定）；业务 dev 端点扩展走"业务层 service / handler + devtools.Register 接 interface"模式（已在 devtools.go 顶部 comment 钦定）
- **双闸门 OR 启用**：build tag `-tags devtools` OR env var `BUILD_DEV=true`；任一即启用
- **DevOnlyMiddleware 是请求时第二闸门**：BUILD_DEV 运维热切（启动后 setenv ""）时，路由仍存在但 middleware 推 ErrResourceNotFound 1003 → ErrorMappingMiddleware 翻 envelope HTTP 200
- **Register 是非幂等**：重复调用让 Gin panic（路由重复）；本 story 改签名后调用方仍是 NewRouter 一处
- **测试 build tag `!devtools`**：所有 devtools 自动化测试必须加 `//go:build !devtools`，避免 forceDevEnabled=true 路径污染 env var case

**Story 7.3 SyncSteps 关键模式参考**（虽然不复用 SyncSteps，但事务模式 / stub 模式 / 错误翻译模式都参考 7.3）：

- **事务边界控制**：txMgr.WithTx 闭包内的 repo 调用必须用 `txCtx`（ADR-0007 §2.2）
- **errors.Is 区分哨兵**：`stderrors.Is(err, mysql.ErrUserNotFound)` —— 包名 `stderrors` 因 `errors` 已被 `apperror` 占用 alias
- **不在 service 直接 panic 业务错**：业务错（1003 / 1009）走 `apperror.Wrap` + 事务回滚（fn 返非 nil 让 GORM rollback）
- **stub 测试模式**：每 case 独立 stub instance；fn 字段未设走 default panic（如 stubStepStepAccountRepo.Create 默认 panic）—— 防止误调用 mock

**Story 7.4 GetAccount 关键模式参考**（同样不复用 GetAccount，但"最小化 service / handler 追加"模式参考）：

- **服务层扩展**：interface 末尾追加 + impl 段末尾追加 + 不动既有方法
- **handler 单测模式**：stub service `getAccountFn` 字段 + 测试 router `r.GET(path, h.GetAccount)` + ErrorMappingMiddleware 必挂

### Lessons Index（与本 story 相关的过去教训）

- [docs/lessons/2026-04-24-error-envelope-single-producer.md] — c.Error / response.Error 二选一；本 story 严格走 `c.Error + return`
- [docs/lessons/2026-05-02-mysql-date-string-transit.md] — DATE 列 string 直传不依赖 DSN loc；本 story sync_date 沿用 string 模式
- [docs/lessons/2026-05-02-step-account-example-invariant-and-cross-section-rate-limit-scope.md] — 跨接口 rate_limit 共享 token bucket；dev 端点本身不挂 rate_limit，但理解共享 scope 思路有助于下游 review

### Git Intelligence（最近 5 个 commit）

```
1e4a19c chore(story-7-4): 收官 Story 7.4 + 归档 story 文件
0ae5a7d feat(server): Epic7/7.4 GET /steps/account 接口
85b5396 docs(lessons): 补充 Story 7.3 review r1~r7 共 7 篇 lesson commit hash
4b36179 chore(story-7-3): 收官 Story 7.3 + 归档 story 文件
bf876ba fix(review): 信任客户端 syncDate 的 anti-cheat 漏洞 + ±N 天容忍窗口的 trade-off
```

**关键提取**：

- 7.4 / 7.3 已 done；step_service / step_account_repo / step_sync_log_repo 全实装；本 story 仅消费这些 repo / 添加新 service
- 7.3 review 经历 r1-r7（7 轮 review）落地了大量防作弊 / 边界处理 lesson —— dev 端点不接这些约束（产品语义就是绕过），但理解 lesson 上下文有助于本 story 不重蹈覆辙

### 常见陷阱（基于 7.3 / 7.4 / 1.6 review 经验）

1. **import cycle**：devtools 包**不**能 import handler 包（handler import service / repo / middleware；devtools 是基础设施层在 handler 之下）→ 用 interface 解耦（本 story AC3 已设计 DevStepsHandler interface）
2. **Register 调用位置**：必须留在 `if deps.GormDB != nil ...` 块**外**，因为 IsEnabled() 不依赖 deps；放块内会让单测 NewRouter(Deps{}) 漏挂 ping-dev
3. **devStepsHandler nil 透传**：`var devStepsHandler *handler.DevStepsHandler` 提前声明 nil；deps 完整时填充。`*handler.DevStepsHandler` 是指针类型，nil 是合法值，devtools.Register 内部 `if devStepsHandler != nil` 判定即可
4. **测试 helper 复用边界**：startMySQL / runMigrations / insertUser / insertStepAccount / fetchStepAccount / countSyncLogs 都在 service 包测试中已 export（同 package）—— 本 story 集成测试**复用**而非新建；但 `buildStepServiceIntegration` 是 7.3 SyncSteps 专用（含 StepsConfig 参数）→ 本 story 新建 `buildDevStepServiceIntegration`（不需要 cfg；多一个 userRepo）
5. **stub 命名冲突**：`stubStepService` 已被 7.3 / 7.4 handler 测试占用 → 本 story handler 测试新建 `stubDevStepService`（独立类型，不复用）
6. **ErrorMappingMiddleware 必挂**：handler 单测 router `r.Use(middleware.ErrorMappingMiddleware())` 必挂，否则 c.Error 不写 envelope —— 与 7.3 / 7.4 同模式
7. **build tag 影响**：`//go:build !devtools` 在 router_dev_test.go / devtools_test.go 必须保留；`go test -tags devtools` 跑这些文件会让 forceDevEnabled=true 让 BUILD_DEV="" 的 case 失败
8. **`bash scripts/build.sh --devtools`**：本 story 改 devtools.Register 签名；必须验证 build tag 路径（forceDevEnabled=true 编译路径）也能通过；script 自动跑 build/catserver-dev 输出

### 测试覆盖矩阵

| 测试层 | 文件 | 新增 case | 覆盖 AC |
|---|---|---|---|
| service 单测 | `dev_step_service_test.go`（新） | 6 | AC1, AC5 |
| handler 单测 | `dev_steps_handler_test.go`（新） | 6 | AC2, AC6 |
| devtools 路由测试 | `devtools_test.go`（追加） | 2 | AC3, AC7 |
| bootstrap wire 测试 | `router_dev_test.go`（追加） | 1 | AC4, AC7 |
| 集成测试 | `dev_step_service_integration_test.go`（新） | 2 | AC8 |
| **合计** | | **17** | |

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- 关键修复：`var devStepsHandler *handler.DevStepsHandler` 直接传给 `devtools.Register(r, devStepsHandler)` 触发 Go 经典 typed-nil-interface 陷阱：interface header `(type=*handler.DevStepsHandler, value=nil)` 在 `if x != nil` 判定中**为真**，导致 Register 误认 handler 非 nil 注册路由 → 调 PostGrantSteps 时 nil receiver panic。修复：bootstrap router.go 加显式 `if devStepsHandler == nil` 分支，nil 时传字面值 `nil` 给 Register（确保传 nil interface 而非 typed-nil）。
- handler 单元测试 `TestRouter_DevGrantSteps_NilHandlerSkipsRoute` 是该陷阱的回归 sentinel —— 修复前红（500 + envelope 1009 panic 兜底），修复后绿（404 NoRoute）。
- 复用 `auth_service_test.go` 中 stubUserRepo 而非新建（同 package 已 export），减少类型重复；6 个 service case 中只有 NegativeSteps_Panics 不调用 FindByID（panic 在 service 入口；txMgr.WithTx 之前），其他 case 显式 set findByIDFn。

### Completion Notes List

**AC10 验证清单（10 项全过）**：

- [x] (1) service 层 GrantSteps 流程严格按 4 步事务：FindByID(user) → FindByUserID(account) → UpdateBalance(+steps, version) → Create sync_log source=2 — `dev_step_service.go` GrantSteps 实装段（行 90-130）
- [x] (2) sync_log.source = 2 / motion_state=1 / client_total=0 / client_ts=0 / sync_date = server today UTC — `dev_step_service.go` log 拼装段（行 119-127）+ HappyPath 测试断言
- [x] (3) handler PostGrantStepsRequest 用 *uint64 / *int32 指针；userId=0 / steps<0 显式 1002 拦截 — `dev_steps_handler.go` PostGrantSteps（行 60-80）
- [x] (4) handler 不调 c.Get(UserIDKey)；userID 全靠 body — `dev_steps_handler.go` 整文件无 c.Get / middleware.UserIDKey 引用
- [x] (5) devtools.Register 签名扩 `(r, devStepsHandler DevStepsHandler)`；nil 时跳过 grant-steps 路由（保留 ping-dev） — `devtools.go` Register 实装 + nil-tolerant if 块
- [x] (6) router.go wire `var devStepsHandler` 提前声明 nil + if 块内构造 + Register 留 if 块外（含 nil-interface 陷阱修复）— `router.go` 行 119 + 168-173 + 192-197
- [x] (7) service 单测 HappyPath 强断 lastCreated.Source=2 + MotionState=1 + ClientTotalSteps=0 + ClientTs=0 + SyncDate matches today — `dev_step_service_test.go` HappyPath 行 98-127
- [x] (8) service 单测 NegativeSteps case 用 defer recover 验防御性 panic — `dev_step_service_test.go` NegativeSteps_Panics 行 217-244
- [x] (9) handler 单测 NegativeSteps case 验 stub.grantStepsFn 主动 t.Errorf（验拦截在 service 之前） — `dev_steps_handler_test.go` NegativeSteps_Returns1002_NoServiceCall
- [x] (10) `bash scripts/build.sh --test` 全绿；`bash scripts/build.sh --integration` BUILD SUCCESS；`bash scripts/build.sh --devtools` BUILD SUCCESS；`git status --short` 改动文件清单匹配预期范围（5 新 + 3 改 + 2 流程文件）

### File List

**新建 5 文件**：

1. `server/internal/service/dev_step_service.go` — DevStepService interface + impl（GrantSteps 事务内 4 步 + source=2 admin_grant + steps<0 panic 防御）
2. `server/internal/service/dev_step_service_test.go` — service 6 case（HappyPath / UserNotFound / StepAccountNotFound / ZeroSteps / UpdateBalanceDBError / NegativeSteps_Panics）
3. `server/internal/service/dev_step_service_integration_test.go` — 集成 2 case（HappyPath_AppliesAccountAndAuditLog + UserNotFound_Returns1003）
4. `server/internal/app/http/handler/dev_steps_handler.go` — DevStepsHandler + PostGrantStepsRequest + PostGrantSteps + postGrantStepsResponseDTO
5. `server/internal/app/http/handler/dev_steps_handler_test.go` — handler 7 case（HappyPath / UserNotFound / NegativeSteps / DBBusy / MissingUserID / UserIDZero / MissingSteps）

**修改 3 文件**：

6. `server/internal/app/http/devtools/devtools.go` — Register 签名扩 `(r, devStepsHandler DevStepsHandler)` + 新增 DevStepsHandler interface + nil-tolerant 跳过 grant-steps 路由
7. `server/internal/app/http/devtools/devtools_test.go` — 末尾追加 2 case（GrantStepsRegisteredWhenHandlerProvided + GrantStepsSkippedWhenHandlerNil）+ doPost helper + devStepsHandlerFunc adapter；既有 6 case 的 `devtools.Register(r)` 改为 `devtools.Register(r, nil)`
8. `server/internal/app/bootstrap/router.go` — wire devStepSvc + devStepsHandler；var 提前声明 + if 块内构造 + Register 透传（含 nil-interface 陷阱修复）
9. `server/internal/app/bootstrap/router_dev_test.go` — 末尾追加 1 case（TestRouter_DevGrantSteps_NilHandlerSkipsRoute）

**流程文件**：

10. `_bmad-output/implementation-artifacts/sprint-status.yaml` — 7-5 状态 ready-for-dev → in-progress → review
11. `_bmad-output/implementation-artifacts/7-5-dev-端点-post-dev-grant-steps.md` — 本 story 文件本身（任务勾选 + Status / Dev Agent Record / File List / Change Log 填充）

### Change Log

| 日期 | 改动 | 状态变更 |
|---|---|---|
| 2026-05-02 | Story 7.5 created (backlog → ready-for-dev) | backlog → ready-for-dev |
| 2026-05-03 | dev-story 实装：dev_step_service + dev_steps_handler + devtools.Register 签名扩 + bootstrap wire + 单测 16 case + 集成 2 case；全部 build / test 通过 | ready-for-dev → in-progress → review |
