# Story 7.4: GET /steps/account 接口

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want 任何时候可以查询当前步数账户的三档值（totalSteps / availableSteps / consumedSteps）,
so that 主界面 / 仓库页 / 开宝箱前置步骤等任何"读账户"场景都能拿到 server 权威值，不依赖客户端缓存的本地估算。

## 故事定位（Epic 7 第四条 = 节点 3 第二条 server 业务实装；上承 7.3 入账事务，下启 7.5 dev 端点 + iOS 8.5 触发器）

- **Epic 7 进度**：7.1（契约最终化，**done** —— V1 §1 / §6.1 / §6.2 schema + 错误码 + syncDate 时区契约 + 节点 3 冻结声明落地）→ 7.2（user_step_sync_logs migration，**done** —— 0006 表 + dockertest 6 表断言落地）→ 7.3（POST /steps/sync handler + service + 累计差值入账事务 + 防作弊，**done** —— `step_sync_log_repo` + `step_account_repo.UpdateBalance` + `step_service.SyncSteps` + `steps_handler.PostSync` + `bootstrap/router.go` wire + 单测 ≥20 + 集成 ≥3 落地）→ **7.4（本 story，GET /steps/account handler + service.GetAccount 纯读 + 单测 ≥3 + 集成 ≥1）** → 7.5（dev 端点 POST /dev/grant-steps，复用 7.3 的 step_sync_log_repo / step_account_repo.UpdateBalance）。
- **本 story 是 Epic 7 最简单的一条**：纯读 + 无事务 + 无防作弊 + 无配置 + 无新 repo —— 仅扩 `service.StepService` interface 加 `GetAccount` 方法 + `steps_handler.GetAccount` handler + `bootstrap/router.go` wire 一行 + 单测 / 集成测试。
- **下游 7.5 (POST /dev/grant-steps)**：**不**复用本 story 的 `service.StepService.GetAccount`（dev grant 走自己的 dev_step_service 写入路径，与 GET /steps/account 完全独立）；但**集成测试**会调本 story 的 GET /steps/account 验账户值。
- **下游 iOS Story 8.5（步数同步触发器）+ Story 21.5（开箱前主动同步步数）**：iOS 端 SyncStepsUseCase 调 POST /steps/sync 拿最新账户；但**节点 3 主界面**还有"刚启动 / 回前台时不一定立即触发 sync"场景，HomeViewModel 用 GET /steps/account 拿 server 权威账户做兜底展示；GET 是纯读没副作用，可频繁调（限频 60/min/userID）。
- **epics.md AC 钦定**（`_bmad-output/planning-artifacts/epics.md` §Story 7.4 行 1404-1414）：
  - 接口要求 auth（带 Bearer token），无 token 返回 1001
  - account 不存在（理论不该发生，因登录时已初始化）→ 返回 1003 资源不存在
  - **单元测试 ≥3 case**（mocked repo）：happy account 存在 / edge account 不存在返 1003 / edge 三档全 0
  - **集成测试**（dockertest）：创建用户 + step_account → curl GET /steps/account → 返回正确值
- **V1 §6.2 已冻结契约**（`docs/宠物互动App_V1接口设计.md` 行 603-660；7.1 已 done）：
  - request：无 body；token from `Authorization: Bearer <token>` header
  - response data（成功 code=0）：`{totalSteps: int64, availableSteps: int64, consumedSteps: int64}`（**扁平结构**，**不**包一层 `stepAccount`）
  - **错误码全集**：1001 (auth) / 1003 (account 不存在；理论不该发生但兜底) / 1005 (rate_limit) / 1009 (DB)
  - 认证：Bearer token；限频：默认 60/min/userID（已认证子组）
- **V1 §6.2 关键 schema 嵌套差异**（行 628 `> schema 嵌套差异` 引用块）：
  - §6.1 `POST /steps/sync` 响应：`data.stepAccount.{totalSteps, availableSteps, consumedSteps}`（**嵌套** stepAccount 子对象，因还要并列 `acceptedDeltaSteps` 动作返回值）
  - §6.2 `GET /steps/account` 响应：`data.{totalSteps, availableSteps, consumedSteps}`（**扁平**，无 stepAccount 子对象）
  - 这是文档侧**显式钦定的差异**（行 628 给出原因：动作型 vs 纯读型）；本 story handler `getAccountResponseDTO` **不能**复用 7.3 的 `postSyncResponseDTO`，必须独立构造扁平 gin.H
- **数据库设计 §5.4 钦定**（`docs/宠物互动App_数据库设计.md`）：本 story 仅调 `stepAccountRepo.FindByUserID` 单行 SELECT，**无**写入；`user_step_accounts` 行在 Story 4.6 firstTimeLogin 五表事务中已建（默认全 0）

## 范围红线（明确不做）

**本 story 只做**：(1) `service.StepService` interface 末尾追加 `GetAccount(ctx, userID) (*StepAccountBrief, error)` 方法 + 实装；(2) `steps_handler.go` 末尾追加 `GetAccount(c *gin.Context)` handler + `getAccountResponseDTO` helper；(3) `bootstrap/router.go` 在 authedGroup 末尾追加 `authedGroup.GET("/steps/account", stepsHandler.GetAccount)` 一行；(4) service 单测 ≥3 case；(5) handler 单测 ≥3 case；(6) 集成测试 ≥1 case；(7) 本 story 文件 + sprint-status.yaml。

**本 story 不做**：

- **不**新增任何 repo 文件（`step_account_repo.FindByUserID` 已实装，本 story 仅**消费**）
- **不**新增任何 repo 方法（`FindByUserID` 已存在，**不**改也**不**追加 `FindBalanceByUserID` 等同义新方法）
- **不**新增任何 service 文件（仅扩 `step_service.go` 末尾追加 `GetAccount` 方法到现有 `StepService` interface + impl struct）
- **不**新增任何 handler 文件（仅扩 `steps_handler.go` 末尾追加 `GetAccount` 方法到现有 `StepsHandler` struct）
- **不**接事务（GET /steps/account 全只读，**不**调 `txMgr.WithTx`；与 `home_service.LoadHome` 同模式 —— 纯读不上事务）
- **不**接 Redis（节点 3 阶段无 Redis；纯查询无幂等需求）
- **不**接防作弊 / 阈值（GET 不写入，与单次封顶 / 当日封顶无关；`config.StepsConfig` 不需要新字段）
- **不**改 V1 §6.2 接口契约任一字（7.1 已冻结）
- **不**改 `migrations/0001-0006` 任一文件（7.2 已锁定）
- **不**改 `docs/宠物互动App_*.md` 任一份（消费方）
- **不**改 `step_sync_log_repo.go` 任一行（GET 不读 sync_log）
- **不**改 `step_account_repo.go` 任一行（仅消费已实装的 `FindByUserID`）
- **不**改 `internal/pkg/errors/codes.go`（1003 / 1005 / 1009 全已注册；本 story 仅**消费**）
- **不**改 `internal/app/http/middleware/auth.go` / `rate_limit.go` / `error_mapping.go`（已实装；GET /steps/account 与 POST /steps/sync 共享 `authedGroup` 中间件链）
- **不**改 `Deps` struct（GET /steps/account 不引入新依赖；`stepSvc` 已在 7.3 wire 阶段构造，本 story 复用同一个 `stepsHandler` 实例）
- **不**修改 `cmd/server/main.go` / `internal/app/bootstrap/server.go`（除 router.go 一行 wire 外）
- **不**改 README / 部署文档（节点 3 / Epic 36 才统一更新）
- **不**改 `local.yaml` 任一行（无新配置项）
- **不**写 e2e 跨端测试（Epic 9 才做）

**任何超出上述清单的改动 → HALT 并问设计**。

## Acceptance Criteria

**AC1 — `service.StepService` interface 末尾追加 `GetAccount` 方法 + impl 段**

修改 `server/internal/service/step_service.go`：

1. 在 `StepService` interface 末尾**追加** `GetAccount` 方法签名（保留现有 `SyncSteps` 不动）
2. 在文件末尾**追加** `GetAccount` 方法实装（`*stepServiceImpl` 接收者）

接口扩展（追加到现有 `StepService` interface 末尾）：

```go
    // GetAccount 处理 GET /api/v1/steps/account 业务（Story 7.4）。
    //
    // 流程（V1 §6.2.4 + 数据库设计 §5.4）：
    //  1. stepAccountRepo.FindByUserID(ctx, userID) → step_account（必有；登录初始化已建）
    //  2. 拼装 StepAccountBrief 返回（三档值原样透传，无动态判定）
    //
    // 错误约定（ADR-0006 三层映射）：
    //   - mysql.ErrStepAccountNotFound（理论不该发生 —— Story 4.6 五表事务必建一行）→
    //     **包成 ErrResourceNotFound (1003)**（V1 §6.2.6 行 657 钦定 1003，**不**包成 1009 ——
    //     即便是"理论不该发生"的数据 invariant 损坏，仍按 V1 钦定的错误码下发，让客户端
    //     能区分"账户缺失"vs"DB 异常"，便于运维排查）
    //   - 其他 DB 异常（连接断 / SQL 错 / 死锁等）→ 包成 ErrServiceBusy (1009)
    //
    // **不**接事务（纯读，与 home_service.LoadHome 同模式）；**不**接防作弊（GET 不写入）；
    // **不**调 stepSyncLogRepo（账户值是 user_step_accounts 单表查询，**不**经 sync_log 重算）。
    GetAccount(ctx context.Context, userID uint64) (*StepAccountBrief, error)
```

实装段（追加到文件末尾，与 `SyncSteps` 实装并列）：

```go
// GetAccount 实装：单表查询 user_step_accounts → StepAccountBrief。
//
// **关键**：account 必有（登录初始化已建）；查不到 = 数据 invariant 损坏 → 1003（V1 §6.2.6 钦定）。
// 任何其他 DB 异常 → 1009 服务繁忙（连接断 / 死锁等运行期失败）。
//
// **复用 StepAccountBrief**（home_service.go 已定义）：节点 2 阶段 home_service.LoadHome
// 已经把"账户三档值"抽象成 StepAccountBrief；本 service 直接复用，**不**新建 GetAccountOutput
// 等同义类型（避免类型膨胀；与 7.3 SyncStepsOutput 嵌套 StepAccountBrief 同模式）。
//
// **返回 *StepAccountBrief 而非 StepAccountBrief**（指针类型）：与 7.3 SyncStepsOutput 同模式 ——
// nil 表示 error 路径未拼装，调用方 handler 的 `if err != nil { return }` 短路；
// 成功路径返回 &StepAccountBrief{...}。
func (s *stepServiceImpl) GetAccount(ctx context.Context, userID uint64) (*StepAccountBrief, error) {
    account, err := s.stepAccountRepo.FindByUserID(ctx, userID)
    if err != nil {
        // 理论不该发生（Story 4.6 五表事务必建一行）→ 但 V1 §6.2.6 钦定 1003，按契约下发。
        if stderrors.Is(err, mysql.ErrStepAccountNotFound) {
            return nil, apperror.Wrap(err, apperror.ErrResourceNotFound, apperror.DefaultMessages[apperror.ErrResourceNotFound])
        }
        // 其他 DB 异常 → 1009
        return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
    }
    return &StepAccountBrief{
        TotalSteps:     account.TotalSteps,
        AvailableSteps: account.AvailableSteps,
        ConsumedSteps:  account.ConsumedSteps,
    }, nil
}
```

**关键约束**：

- **复用 `StepAccountBrief`**（home_service.go 已定义；7.3 SyncStepsOutput 也复用）—— **不**新建 GetAccountOutput / AccountBalance 等同义类型
- **`stderrors.Is` 区分 NotFound 哨兵**：与 7.3 service 内 `stderrors.Is(err, mysql.ErrStepSyncLogNotFound)` 同模式（包名是 `stderrors` 因为 `errors` 已被 `apperror` 占用 alias）
- **NotFound → 1003**（V1 §6.2.6 钦定）；**不**与 home_service.LoadHome 一样把 stepAccount NotFound 包成 1009 —— LoadHome 的逻辑是"任一聚合查询失败整体 1009 不部分降级"（home 是聚合接口，部分失败影响全屏），本 story 是单查接口，按 V1 §6.2.6 钦定 1003 更精确
- **不**调 `stepSyncLogRepo`（与 7.3 防作弊 SUM 不同；GET 是账户单表查询，**不**经 sync_log 重新计算 —— sync 接口已在事务内把 account 的 total / available 维护到正确值）
- **不**接事务（纯读；与 `home_service.LoadHome` 同模式）
- **不**做 `time.Now().UTC()` 之类的动态判定（与 home_service.computeRemainingSeconds 不同 —— 三档值是 DB 原值，无时间相关动态计算）

**AC2 — `steps_handler.GetAccount` handler + `getAccountResponseDTO` helper**

修改 `server/internal/app/http/handler/steps_handler.go`：

1. 在 `*StepsHandler` 结构体接收者末尾**追加** `GetAccount(c *gin.Context)` 方法
2. 在文件末尾**追加** `getAccountResponseDTO` helper

handler 段（追加到现有 `PostSync` 方法之后）：

```go
// GetAccount 处理 GET /api/v1/steps/account（Story 7.4）。
//
// 流程：
//  1. 从 c.Get(UserIDKey) 拿 userID（auth 中间件已注入；不存在 → 1009 unreachable 兜底）
//  2. 调 svc.GetAccount(ctx, userID) —— ctx = c.Request.Context()（**不**用 *gin.Context；ADR-0007 §2.2）
//  3. 成功 → response.Success(c, dto, "ok")；失败 → c.Error(err) + return（middleware envelope）
//
// **不**做参数校验（GET 无 body / 无 query；userID 由 auth 中间件兜底）；与 home_handler.LoadHome 同模式。
//
// **不**直接调 response.Error 写 envelope（ADR-0006 单一 envelope 生产者；与 4.6 / 4.8 / 7.3 同模式）。
func (h *StepsHandler) GetAccount(c *gin.Context) {
    // 从 auth 中间件取 userID（与 home_handler.LoadHome / steps_handler.PostSync 同模式）
    v, ok := c.Get(middleware.UserIDKey)
    if !ok {
        // unreachable: Auth 中间件挂在前；保险兜底走 1009
        _ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
        return
    }
    userID, ok := v.(uint64)
    if !ok {
        // unreachable: Auth 中间件 c.Set(UserIDKey, claims.UserID) 永远是 uint64
        _ = c.Error(apperror.New(apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy]))
        return
    }

    out, err := h.svc.GetAccount(c.Request.Context(), userID)
    if err != nil {
        _ = c.Error(err) // service 已 wrap *AppError；ErrorMappingMiddleware 写 envelope
        return
    }

    response.Success(c, getAccountResponseDTO(out), "ok")
}

// getAccountResponseDTO 把 service 输出转成 V1 §6.2 wire 格式。
//
// **关键 schema 嵌套差异**（V1 §6.2 行 628 引用块钦定）：
//   - §6.1 POST /steps/sync 响应：`data.stepAccount.{totalSteps, ...}`（**嵌套** stepAccount 子对象）
//   - §6.2 GET /steps/account 响应：`data.{totalSteps, ...}`（**扁平**，无 stepAccount 子对象）
//
// 这两端不同设计的原因：§6.1 是"动作型"接口，data 还要并列 acceptedDeltaSteps 等动作返回值，
// 故聚合 stepAccount 子对象避免与同级字段平铺混淆；§6.2 是"纯读型"接口，data 只含三档值，
// 无嵌套必要。**本 helper 不能复用 postSyncResponseDTO**（嵌套结构不同）。
//
// 客户端 Codable 解析 V1 §6.2 行 628 已警告：StepsSyncResponse.data.stepAccount.* 与
// StepsAccountResponse.data.* 是**两种结构**，必须分别解析。
func getAccountResponseDTO(out *service.StepAccountBrief) gin.H {
    return gin.H{
        "totalSteps":     out.TotalSteps,
        "availableSteps": out.AvailableSteps,
        "consumedSteps":  out.ConsumedSteps,
    }
}
```

**关键约束**：

- handler 不做参数校验（GET 无 body / query；userID 走 auth 中间件）—— 与 home_handler.LoadHome 同模式
- userID 取出与类型断言用 `c.Get(middleware.UserIDKey)` + `v.(uint64)` —— 与 home_handler.LoadHome / steps_handler.PostSync 完全同模式（AC 期望文案统一）
- `c.Error + return` 而非 `response.Error`（ADR-0006）
- `getAccountResponseDTO` 是**独立 helper**，**不能**复用 `postSyncResponseDTO`（V1 §6.2 行 628 钦定的扁平 vs 嵌套差异）
- 不引入新 DTO 类型 —— 直接 `gin.H{...}` 返回
- DTO 字段命名严格 `totalSteps` / `availableSteps` / `consumedSteps`（V1 §6.2.3 钦定 camelCase）

**AC3 — `bootstrap/router.go` 在 authedGroup 末尾追加 GET /steps/account 路由**

修改 `server/internal/app/bootstrap/router.go`：在 `if deps.GormDB != nil && ...` 块内、`authedGroup.POST("/steps/sync", stepsHandler.PostSync)` 那一行之后**追加一行**：

```go
authedGroup.GET("/steps/account", stepsHandler.GetAccount) // Story 7.4 加
```

**关键约束**：

- **不**新建 repo / service / handler 实例（7.3 已构造 stepsHandler；本 story 复用同一实例 —— 调它新加的 GetAccount 方法）
- **不**改 `Deps` struct（无新依赖）
- **不**改 `cmd/server/main.go`（无新配置 / 依赖透传）
- 路由挂在 `authedGroup`（与 POST /steps/sync 共享 auth + rate_limit by userID 中间件 —— V1 §6.2 钦定需要 Bearer + 限频）
- 路径 `/steps/account` 不带 `/api/v1` 前缀（前缀由 `api := r.Group("/api/v1")` + `authedGroup := api.Group(...)` 自动加）
- HTTP method `GET`（V1 §6.2 钦定）；**不**用 POST

**AC4 — service 单元测试覆盖（≥3 case，stub repo + stub txMgr）**

修改 `server/internal/service/step_service_test.go`：在文件末尾**追加** 3 个 case（前缀 `TestStepService_GetAccount_`）。**不**改现有 9 个 SyncSteps case。

**必须覆盖 3 case**：

1. **`TestStepService_GetAccount_HappyPath_ReturnsThreeBalances`**：stubStepAccountRepo.findByUserIDFn 返 `&mysql.StepAccount{TotalSteps: 1140, AvailableSteps: 840, ConsumedSteps: 300, Version: 5}` → service 返 `*StepAccountBrief{Total: 1140, Available: 840, Consumed: 300}`，err = nil；stubStepSyncLogRepo 三个方法**完全不被调**（GET 不读 sync_log）

2. **`TestStepService_GetAccount_AccountNotFound_Returns1003`**：stubStepAccountRepo.findByUserIDFn 返 `(nil, mysql.ErrStepAccountNotFound)` → service 返 nil + `*apperror.AppError` with Code = `apperror.ErrResourceNotFound (1003)`（**不**是 1009）

3. **`TestStepService_GetAccount_AllZero_NewUser_ReturnsZeros`**：新用户场景，stubStepAccountRepo.findByUserIDFn 返 `&mysql.StepAccount{TotalSteps: 0, AvailableSteps: 0, ConsumedSteps: 0, Version: 0}` → service 返 `*StepAccountBrief{0, 0, 0}`，err = nil（验证 0 不被特殊处理为 NotFound）

**stub repo 复用**：直接复用 `step_service_test.go` 已定义的 `stubStepAccountRepo` / `stubStepSyncLogRepo` / `stubTxMgr`（7.3 已建）。如果现有 stub `findByUserIDFn` 字段缺失，按 7.3 既有 stub 模式补上 `findByUserIDFn func(ctx context.Context, userID uint64) (*mysql.StepAccount, error)` 字段并补 satisfy interface 方法 —— 但**先**读 step_service_test.go 现有 stub，**不要**重复定义。

**断言模式**（与 home_service_test 单测断 *AppError 同模式）：

```go
// 断 NotFound case 返 1003：
out, err := svc.GetAccount(ctx, 1001)
if out != nil {
    t.Errorf("out = %+v, want nil on error path", out)
}
var appErr *apperror.AppError
if !errors.As(err, &appErr) {
    t.Fatalf("err is not *AppError: %T (%v)", err, err)
}
if appErr.Code != apperror.ErrResourceNotFound {
    t.Errorf("appErr.Code = %d, want %d (1003)", appErr.Code, apperror.ErrResourceNotFound)
}
```

**关键约束**：

- 3 case 命名前缀 `TestStepService_GetAccount_<场景>` 一目了然（与 7.3 SyncSteps case 命名同风格，便于 -run filter）
- **不**断言 stub 内"sync_log 三个方法不被调"用计数器 `findLatestCalls` —— 直接默认值 0；改用 stub 函数 `func(...) { t.Errorf("should NOT be called") }` 主动 fail（与 7.3 handler 单测中 syncStepsFn 内主动 t.Errorf 同模式，更醒目）
- 每 case 用**独立 stub instance**（避免 createCalls 计数串扰 —— 7.3 既有约束）
- 不需要 stub txMgr 的 WithTx 被调（GetAccount 不接事务）—— 但 stubTxMgr 仍需注入（NewStepService 签名要求）

**AC5 — handler 单元测试覆盖（≥3 case，stub service + 测试 router）**

修改 `server/internal/app/http/handler/steps_handler_test.go`：

1. 在 `stubStepService` struct 末尾**追加** `getAccountFn` 字段
2. 在 `stubStepService` 末尾**追加** `GetAccount` 方法实装
3. 在文件末尾**追加** 3 个 case（前缀 `TestStepsHandler_GetAccount_`）

stub service 扩展（追加到现有 `stubStepService` struct 字段 + `SyncSteps` 方法之后）：

```go
type stubStepService struct {
    syncStepsFn func(ctx context.Context, in service.SyncStepsInput) (*service.SyncStepsOutput, error)
    getAccountFn func(ctx context.Context, userID uint64) (*service.StepAccountBrief, error) // Story 7.4 加
}

// GetAccount 实装（Story 7.4 加）。
func (s *stubStepService) GetAccount(ctx context.Context, userID uint64) (*service.StepAccountBrief, error) {
    return s.getAccountFn(ctx, userID)
}
```

测试 router helper（**复用** `newStepsHandlerRouter` —— 给现有 helper 末尾追加一行注册 GET /steps/account）：

```go
// 在现有 newStepsHandlerRouter 末尾 r.POST("/api/v1/steps/sync", h.PostSync) 之后追加：
r.GET("/api/v1/steps/account", h.GetAccount) // Story 7.4 加
```

**必须覆盖 3 case**（前缀 `TestStepsHandler_GetAccount_`）：

1. **`TestStepsHandler_GetAccount_HappyPath_ReturnsFlatSchema`**：合法 GET（mockUserID = 1001 注入）→ stub service 返 `&service.StepAccountBrief{TotalSteps: 1140, AvailableSteps: 840, ConsumedSteps: 300}` → 200 + `envelope.code=0` + `data.totalSteps=1140` + `data.availableSteps=840` + `data.consumedSteps=300`；**关键断言**：`envelope.data` **直接含三档值**（`data.totalSteps`），**没有** `data.stepAccount` 子对象（V1 §6.2 行 628 钦定的扁平结构 vs §6.1 嵌套结构）

2. **`TestStepsHandler_GetAccount_ServiceReturns1003_ForwardsAsCode1003_HTTP200`**：stub service 返 `*apperror.AppError(ErrResourceNotFound, "资源不存在")` → handler `c.Error` → middleware envelope `code=1003`，HTTP **200**（V1 §2.4 钦定业务错也走 HTTP 200，不是 4xx）

3. **`TestStepsHandler_GetAccount_MissingUserIDInContext_Returns1009`**：单测启动 router 时 mockUserID = nil（不注入 userID 到 c.Keys）→ handler 走 unreachable 兜底分支（与 home_handler_test / 7.3 PostSync 同 fail-safe）→ envelope code=1009

**关键约束**：

- 3 case 命名前缀 `TestStepsHandler_GetAccount_<场景>` —— 与现有 PostSync case 命名同风格，便于 -run filter
- 单测启动的 router **必须挂 ErrorMappingMiddleware**（newStepsHandlerRouter 已挂）
- 注入 userID 通过 `mockUserID *uint64` 参数（newStepsHandlerRouter 已支持；nil 不挂 mock 模拟 unreachable 分支）
- happy path **必须断扁平结构**（`data.totalSteps` 顶级访问；如果误访问 `data.stepAccount` 应该读到 nil）—— 这是 V1 §6.2 行 628 钦定差异的核心断言点
- **不**真起 auth 中间件 / signer
- decodeStepsEnvelope helper 已存在（7.3 落地），直接复用
- 不需要 stub `getAccountFn` 内部"主动校验 in.UserID == 1001" —— 因为 GetAccount 入参只有 userID，且测试 router 注入了 1001，stub 函数被调即说明 wire 链路正确，但**仍可**像 7.3 PostSync HappyPath 一样在 stub 内 `if userID != 1001 { t.Errorf(...) }` 验 ID 透传

**AC6 — 集成测试（dockertest，≥1 case）**

修改 `server/internal/service/step_service_integration_test.go`：在文件末尾**追加** 1 个 case（与现有 7.3 集成测试并列）。**不**改现有 6 个 SyncSteps 集成测试 case。

**必须覆盖 1 case**（epics.md §Story 7.4 行 1414 钦定）：

`TestStepServiceIntegration_GetAccount_ReturnsLatestBalances`：

- 容器内 migrate up（含 0006）→ 复用现有 `buildStepServiceIntegration` helper（7.3 已建）
- 手工 INSERT user + step_account（用 7.3 已建的 `insertUser` / `insertStepAccount` helper —— 注意 step_account 初始化为非零 `total=500, available=400, consumed=100`，验证三档值正确独立读出）
- 调 `svc.GetAccount(ctx, userID)` → 验返 `*StepAccountBrief{TotalSteps: 500, AvailableSteps: 400, ConsumedSteps: 100}`，err = nil
- **可选断言**：再调一次 SyncSteps（clientTotal=200）→ step_account.total/available 各 +200 → 再调 GetAccount → 验三档值跟随更新（500+200=700 / 400+200=600 / 100 不变）—— 这一步**可加**作为"GetAccount 与 SyncSteps 联动"的端到端验证，但**不强制**（epics.md 钦定的最小集成 case 只是"创建用户 + step_account → curl GET /steps/account → 返回正确值"）。**本 story 选做** —— 增加置信度，时间成本极低（多调一次 sync + 一次 get）

**实装模式**（参考 `TestStepServiceIntegration_FirstAndSecondSync_HappyPath` 行 125-198）：

```go
//go:build integration
// 追加到 step_service_integration_test.go 末尾

// ============================================================
// AC6: GetAccount 单查 + sync 联动
// ============================================================
func TestStepServiceIntegration_GetAccount_ReturnsLatestBalances(t *testing.T) {
    svc, sqlDB, cleanup := buildStepServiceIntegration(t, config.StepsConfig{})
    defer cleanup()

    const userID = uint64(1)
    insertUser(t, sqlDB, userID, "uid-step-get-1", "用户GET", "")
    insertStepAccount(t, sqlDB, userID, 500, 400, 100)

    ctx := context.Background()

    // 1. 初始查询
    out, err := svc.GetAccount(ctx, userID)
    if err != nil {
        t.Fatalf("GetAccount initial: %v", err)
    }
    if out.TotalSteps != 500 || out.AvailableSteps != 400 || out.ConsumedSteps != 100 {
        t.Errorf("GetAccount initial: total=%d available=%d consumed=%d, want 500/400/100",
            out.TotalSteps, out.AvailableSteps, out.ConsumedSteps)
    }

    // 2. 联动：sync 一次（clientTotal=200，首次同步当日 → delta=200）
    const syncDate = "2026-05-01"
    _, err = svc.SyncSteps(ctx, service.SyncStepsInput{
        UserID: userID, SyncDate: syncDate, ClientTotalSteps: 200, MotionState: 2, ClientTimestamp: 1714560000000,
    })
    if err != nil {
        t.Fatalf("SyncSteps: %v", err)
    }

    // 3. 再查 GetAccount → 三档值跟随更新
    out2, err := svc.GetAccount(ctx, userID)
    if err != nil {
        t.Fatalf("GetAccount after sync: %v", err)
    }
    if out2.TotalSteps != 700 || out2.AvailableSteps != 600 || out2.ConsumedSteps != 100 {
        t.Errorf("GetAccount after sync: total=%d available=%d consumed=%d, want 700/600/100",
            out2.TotalSteps, out2.AvailableSteps, out2.ConsumedSteps)
    }
}
```

**关键约束**：

- 集成测试**必须**复用 7.3 已建的 `buildStepServiceIntegration` / `insertUser` / `insertStepAccount` helper —— **不**重新建一套
- envName 由 helper 内固定 `"dev"` 传入 NewStepService（cap 覆盖配置允许；本 story 不传 cap，走默认）
- **不**新增独立的 `step_handler_integration_test.go`（handler 集成由 service 集成 + 单测已覆盖；e2e 走 Epic 9 跨端测试）
- 本机 Windows docker 不可用 → t.Skip（4.3 graceful skip 模式；4.6 / 4.7 / 4.8 / 7.3 已用同模式）—— `buildStepServiceIntegration` 内部已有 skip 逻辑，本 case 自动继承
- **不**新增 `service.GetAccount` 与"account 不存在 → 1003"的集成 case —— 这一分支理论不该发生（数据 invariant）；单测已用 stub 覆盖该返回路径，集成测试再造一次"故意不 INSERT step_account"成本 > 收益

**AC7 — `bash scripts/build.sh` 全量绿**

完成后必须能跑通：

```bash
bash scripts/build.sh                      # vet + build → no failures
bash scripts/build.sh --test               # 全单测过（service 3 + handler 3 = 6 新 case + 既有全过）
bash scripts/build.sh --race --test        # CI Linux 必过；Windows race skip 不阻塞
bash scripts/build.sh --integration        # 既有集成 + 新 1 case；docker 不可用 → t.Skip
```

**关键约束**：

- 本 story 引入的新代码全部含单测；`go test ./internal/service/... -run TestStepService_GetAccount -v` 必须 3 个 case 全过
- `go test ./internal/app/http/handler/... -run TestStepsHandler_GetAccount -v` 必须 3 个 case 全过
- `--race` 在 Windows skip（ADR-0001 §3.5）；CI Linux 跑
- **不**改 `scripts/build.sh` 自身

**AC8 — 验证清单（人工 + 自动化）**

完成后**人工**核对以下 8 项（结果记到 Completion Notes List）：

| # | 验证项 | 验证方式 |
|---|---|---|
| 1 | service 层 GetAccount 流程严格按 V1 §6.2.4 实装：FindByUserID → 拼装 StepAccountBrief；NotFound 哨兵 → 1003（**非** 1009） | Read service 源文件 + diff against §6.2.4 / §6.2.6 |
| 2 | handler `getAccountResponseDTO` 返**扁平** gin.H（顶级 totalSteps / availableSteps / consumedSteps）；**无** stepAccount 子对象 | Read steps_handler.go + diff against V1 §6.2 行 628 引用块 |
| 3 | handler 复用 `c.Get(middleware.UserIDKey)` + `v.(uint64)` 兜底（与 home_handler / steps_handler.PostSync 完全同模式） | Read steps_handler.GetAccount |
| 4 | service 复用 `StepAccountBrief`（home_service.go 已定义）；**未**新建 GetAccountOutput / AccountBalance 等同义类型 | Grep `StepAccountBrief` 出现位置（home_service / step_service / handler 三处） |
| 5 | bootstrap/router.go 仅追加一行 `authedGroup.GET("/steps/account", stepsHandler.GetAccount)`；**未**新建 stepsHandler / stepSvc 实例（复用 7.3 wire 的实例） | Read router.go diff |
| 6 | service 单测 NotFound case 断 `*apperror.AppError.Code == ErrResourceNotFound (1003)`，**非** 1009 | Read step_service_test.go GetAccount NotFound case |
| 7 | handler 单测 happy case 断 `data.totalSteps != nil && data.stepAccount == nil`（扁平 schema 验证） | Read steps_handler_test.go GetAccount happy case |
| 8 | `bash scripts/build.sh --test` 全绿（含本 story 新增 6 case）；`git status --short` 改动文件清单匹配预期范围（5-6 个文件） | bash 实跑 + git status |

**AC9 — 不 commit（流水线由 epic-loop 下游收口）**

epics.md §Story 7.4 AC 钦定"≥3 单测 + 集成测试覆盖"，**不**钦定"git commit 单独提交"。本 story 是 server 业务代码 story，commit 由 epic-loop 流水线在下游 fix-review / story-done sub-agent 阶段统一收口。

- 本 story 的 dev workflow **不** commit / **不** push
- commit message 模板（story-done 阶段使用）：

  ```text
  feat(steps-account): GET /steps/account 账户三档值查询接口（Story 7.4）

  - service step_service.GetAccount 实装：单表查询 user_step_accounts →
    StepAccountBrief；NotFound → 1003（V1 §6.2.6 钦定）；其他 DB 错 → 1009
  - handler steps_handler.GetAccount + getAccountResponseDTO（**扁平**结构，
    与 §6.1 PostSync 嵌套 stepAccount 不同；V1 §6.2 行 628 钦定差异）
  - bootstrap/router.go wire authedGroup.GET /steps/account 一行
  - 单测 6 case（service 3 + handler 3）+ 集成测试 1 case（GetAccount 与 SyncSteps 联动）

  依据 epics.md §Story 7.4 + V1 §6.2 + apperror.ErrResourceNotFound。

  Story: 7-4-get-steps-account-接口
  ```

- commit hash 待 story-done 阶段产生后回填到本文件

## Tasks / Subtasks

- [x] **Task 1（AC1）**：在 `step_service.go` interface + impl 末尾追加 `GetAccount` 方法
  - [x] 1.1 Read `step_service.go` 完整文件理解既有 interface / impl 结构（**避免**误删 7.3 的 SyncSteps 任一行）
  - [x] 1.2 Read `home_service.go` 复习"纯读 service 模式"（无事务 / 直接 repo 调用 / 简单 DTO 返回）
  - [x] 1.3 Edit `StepService` interface 末尾追加 `GetAccount` 方法签名 + 完整 godoc 注释
  - [x] 1.4 Edit 文件末尾追加 `(s *stepServiceImpl) GetAccount` 实装段（NotFound → 1003 / 其他 → 1009）
  - [x] 1.5 Read 回检：(1) 复用 `StepAccountBrief`（不新建类型）；(2) NotFound 翻译为 `apperror.ErrResourceNotFound (1003)`，**非** 1009；(3) 不调 stepSyncLogRepo / 不调 txMgr.WithTx；(4) `stderrors.Is` 区分哨兵 error

- [x] **Task 2（AC2）**：在 `steps_handler.go` 末尾追加 `GetAccount` handler + `getAccountResponseDTO` helper
  - [x] 2.1 Read `steps_handler.go` + `home_handler.go` 完整文件理解 handler 模式
  - [x] 2.2 Edit `*StepsHandler` 末尾追加 `GetAccount(c *gin.Context)` 方法（参考 home_handler.LoadHome 完整模式）
  - [x] 2.3 Edit 文件末尾追加 `getAccountResponseDTO(out *service.StepAccountBrief) gin.H` helper（**扁平**结构）
  - [x] 2.4 Read 回检：(1) UserID 取出与类型断言用 `c.Get(middleware.UserIDKey) + v.(uint64)`；(2) ctx 用 `c.Request.Context()`；(3) `c.Error + return` 不直接 response.Error；(4) DTO 扁平（**无** stepAccount 子对象，与 postSyncResponseDTO 不同）

- [x] **Task 3（AC3）**：在 `bootstrap/router.go` authedGroup 末尾追加 GET /steps/account 路由一行
  - [x] 3.1 Read 现有 `router.go` 完整文件（确认 7.3 wire 的 stepsHandler 实例可复用）
  - [x] 3.2 Edit 在 `authedGroup.POST("/steps/sync", stepsHandler.PostSync)` 那一行**之后**追加 `authedGroup.GET("/steps/account", stepsHandler.GetAccount) // Story 7.4 加`
  - [x] 3.3 Read 回检：(1) Deps struct 未改；(2) main.go 未改；(3) 仅一行新增

- [x] **Task 4（AC4）**：service 单测 3 case（stub repo + stub txMgr）
  - [x] 4.1 Read `step_service_test.go` 完整文件理解既有 stub 模式（stubStepAccountRepo / stubStepSyncLogRepo / stubTxMgr / 测试 helper）
  - [x] 4.2 Read 既有 stubStepAccountRepo 的 `findByUserIDFn` 字段是否已有（7.3 单测已经用过 → 应该已有）；如缺失按既有 stub 字段添加模式补
  - [x] 4.3 Edit 在文件末尾追加 GetAccount case（HappyPath / NotFound / AllZero + 1 bonus DB error case = 4 case，超过 AC 钦定的 ≥3）
  - [x] 4.4 Read 回检：(1) 每 case 独立 stub instance；(2) NotFound case 用 `stderrors.As(err, &appErr)` + `appErr.Code == apperror.ErrResourceNotFound`；(3) stubStepSyncLogRepo 的方法在 GetAccount case 中如被调用主动 `t.Errorf("should NOT be called")` —— 提取 `failOnSyncLogStub(t)` helper 复用

- [x] **Task 5（AC5）**：handler 单测 3 case（stub service + 测试 router）
  - [x] 5.1 Read `steps_handler_test.go` 完整文件理解既有 stubStepService + newStepsHandlerRouter 模式
  - [x] 5.2 Edit `stubStepService` 字段末尾追加 `getAccountFn` + `GetAccount` 方法实装
  - [x] 5.3 Edit `newStepsHandlerRouter` 末尾追加 `r.GET("/api/v1/steps/account", h.GetAccount)`
  - [x] 5.4 Edit 在文件末尾追加 GetAccount case（HappyPath_FlatSchema / 1003_HTTP200 / MissingUserID_1009 + 1 bonus 1009 case = 4 case，超过 AC 钦定的 ≥3）
  - [x] 5.5 Read 回检：(1) HappyPath case 强断 `data.stepAccount == nil` + `data.totalSteps != nil`（扁平 schema）；(2) 1003 case 验 HTTP 200 + envelope.code=1003；(3) MissingUserID case 用 mockUserID = nil

- [x] **Task 6（AC6）**：集成测试 1 case（dockertest，复用 7.3 helper）
  - [x] 6.1 Read `step_service_integration_test.go` 完整文件理解既有 buildStepServiceIntegration / insertUser / insertStepAccount helper
  - [x] 6.2 Edit 在文件末尾追加 `TestStepServiceIntegration_GetAccount_ReturnsLatestBalances`（按 AC6 模板：初始查 + sync 一次 + 再查验证联动）
  - [x] 6.3 验证本机 Windows docker 不可用 → t.Skip 不阻塞（buildStepServiceIntegration 内已有 skip 逻辑；本机实跑确认 SKIP 输出正常）

- [x] **Task 7（AC7 / AC8）**：全量验证
  - [x] 7.1 `bash scripts/build.sh`（vet + build）过 → BUILD SUCCESS
  - [x] 7.2 `bash scripts/build.sh --test` 全绿（含本 story 新增 8 case：service 4 + handler 4）
  - [ ] 7.3 `bash scripts/build.sh --race --test`（Windows race skip ok；本地未跑，CI Linux 跑）
  - [x] 7.4 `bash scripts/build.sh --integration`（docker 不可用 → t.Skip ok；BUILD SUCCESS）
  - [x] 7.5 `git status --short` 改动文件清单核对（实际 7 文件改 + 1 文件新；与预期范围一致）
  - [x] 7.6 在下方 Completion Notes List 勾选 AC8 验证清单 8 项

- [x] **Task 8（AC9）**：本 story 不做 git commit
  - [x] 8.1 epic-loop 流水线约束：dev-story 阶段不 commit；由下游 fix-review / story-done sub-agent 收口
  - [x] 8.2 commit message 模板保留在 story 文件中
  - [ ] 8.3 commit hash 待 story-done 阶段回填

## Dev Notes

### 关键设计原则

1. **纯读不上事务**：与 home_service.LoadHome 同模式 —— GET /steps/account 全只读（user_step_accounts 单表 SELECT），**不**调 `txMgr.WithTx`。理由：(1) 单表 PK 查询本身原子；(2) 事务上下文成本（连接占用 / 锁追踪）对纯读没有收益；(3) 反模式：把所有 service 方法都包 `txMgr.WithTx` 会让事务边界泄露到无意义的地方，掩盖真实事务边界（7.3 SyncSteps 的事务边界含三步：FindLatest + UpdateBalance + Create —— 这种**多步骤一致性**才是事务的用途）。

2. **NotFound 翻译为 1003，不是 1009**：V1 §6.2.6 行 657 钦定。区别于 home_service.LoadHome 的"任一查询失败 → 1009 不部分降级"策略 —— LoadHome 是聚合接口，stepAccount 缺失会让"主界面整体渲染异常"，故合并为 1009 一刀切；本 story 是单查接口，1003 vs 1009 区分有运维价值（1003 = 数据 invariant 损坏，需查 4.6 firstTimeLogin 是否漏掉某个用户；1009 = 运行期 DB 异常，需查 MySQL 健康状态）。**反模式**：本 story 也包成 1009 → 客户端无法区分两种失败，运维排查更难。

3. **不复用 postSyncResponseDTO**：V1 §6.2 行 628 引用块**显式钦定**两端 schema 嵌套差异（§6.1 嵌套 / §6.2 扁平）。理由：(1) §6.1 是动作型接口，data 还要并列 acceptedDeltaSteps，故聚合 stepAccount 子对象避免与 acceptedDeltaSteps 同级混淆；(2) §6.2 是纯读型接口，data 只含三档值，无嵌套必要；(3) 客户端 Codable 解析（iOS Story 8.5）的 `StepsSyncResponse.data.stepAccount.*` 与 `StepsAccountResponse.data.*` 是两套结构 —— 本 story 强制扁平，避免下游 iOS 同事误以为可以共用一个 Codable struct。

4. **复用 StepAccountBrief**（home_service.go 已定义；7.3 SyncStepsOutput 也复用）：避免类型膨胀。**反模式**：本 story 新建 `GetAccountOutput` / `AccountBalance` 等同义类型 → 同样三个字段三套类型，未来 add field 要改三处。

5. **复用 stepsHandler / stepSvc 实例**（7.3 wire 阶段已构造）：bootstrap/router.go 本 story 仅追加一行路由注册，**不**重新构造实例。这是节点 3 阶段"按 method 维度而非 endpoint 维度组织 handler"的纪律 —— 7.3 已挂的 `stepsHandler` 是 `/api/v1/steps/*` 全部端点的 handler 集合，新加 endpoint 走"扩 stepsHandler 方法 + 追加路由"模式。

6. **不写 e2e 跨端测试**：Epic 9 才做（e2e/node-3-steps-cat-e2e.md by Story 9.1）。本 story 在 server 自包含层完成 service / handler / 集成测试 ≥1 case 即足以验证；与 iPhone 端联动靠 Story 9.1 跨端 e2e 兜底。

7. **限频按 userID 共享 60/min**：路由挂在 `authedGroup`（POST /steps/sync 共享同一限频策略），用户 60 次 sync + 60 次 GET 共用一个 token bucket（4.5 RateLimitByUserID 策略）。理由：(1) sync + get 都属"步数模块"业务，按用户限频统一就够；(2) 反模式：给本接口单独建 RateLimit 作用域 → 维护成本高，且节点 3 阶段限频文档（V1 §4）只钦定一个 60/min/userID。

### 架构对齐

**领域模型层**（`docs/宠物互动App_总体架构设计.md`）：

- 步数账户是节点 3 核心可消费资产；本 story 是步数账户**读取**链路的实装（与 7.3 入账写入链路并列）
- "状态以 server 为准"原则：客户端任何时候可调本接口拿权威值，不依赖本地缓存

**数据库层**（`docs/宠物互动App_数据库设计.md`）：

- §5.4 user_step_accounts：本 story 消费（FindByUserID 单行查询）
- 索引：PK = user_id，单行查询直接走 PK，无需新增索引

**接口契约层**（`docs/宠物互动App_V1接口设计.md`）：

- §1 节点 3 契约冻结声明：本 story 严格按已冻结的 §6.2 实装
- §6.2.1 接口元信息：HTTP GET / Path /api/v1/steps/account / 需要 Bearer / 限频 60/min/userID / 节点 3
- §6.2.2 响应体 schema：`data.{totalSteps, availableSteps, consumedSteps}`（**扁平**，无嵌套）
- §6.2 行 628 引用块：与 §6.1 的嵌套 schema 显式差异（**关键差异点**）
- §6.2.4 服务端行为：(1) 认证 & 限频；(2) 单表查询；(3) 响应；(4) edge case NotFound → 1003
- §6.2.6 错误码：1001 by auth / 1003 by service NotFound 翻译 / 1005 by rate_limit / 1009 by service DB 异常兜底

**服务端架构层**（`docs/宠物互动App_Go项目结构与模块职责设计.md`）：

- §5.1 handler 层：本 story GetAccount 严格按 handler 职责（DTO 转换 + 不直接接触 *gorm.DB）
- §5.2 service 层：本 story GetAccount 严格按 service 职责（业务规则薄到只有"NotFound 翻译为 1003"，是合规的"GET 接口 service 层最小化"模式）
- §5.3 repo 层：本 story 仅消费已实装的 `step_account_repo.FindByUserID`，**不**改 repo 层

**ADR 对齐**：

- ADR-0006 三层错误映射：repo 返哨兵 ErrStepAccountNotFound → service 翻译为 *AppError(1003) → handler c.Error + middleware envelope
- ADR-0007 ctx 传播：service / repo 第一参数 ctx；handler 用 `c.Request.Context()` 不直接传 *gin.Context
- ADR-0006 单一 envelope 生产者：handler 一律 `c.Error + return`，由 ErrorMappingMiddleware 写 envelope

### 关于 Story 7.4 与 7.3 的关键差异

| 维度 | 7.3 POST /steps/sync | 7.4 GET /steps/account |
|------|---------------------|-----------------------|
| HTTP method | POST | GET |
| body | 有（4 字段） | 无 |
| 事务 | 有（FindLatest + UpdateBalance + Create） | 无（单表 SELECT） |
| 防作弊 | 有（5000 / 50000） | 无（GET 不写入） |
| 响应 schema | 嵌套（data.stepAccount.*） | **扁平**（data.*） |
| 错误码全集 | 1001 / 1002 / 1005 / 3001 / 1009 | 1001 / 1003 / 1005 / 1009 |
| NotFound 翻译 | n/a（首次同步走 first 分支） | 1003（V1 §6.2.6 钦定） |
| 新 repo / repo 方法 | 新增 step_sync_log_repo + UpdateBalance | 无（消费现有 FindByUserID） |
| 新配置 | 有（StepsConfig） | 无 |
| 测试规模 | service 9 + handler 6 + repo 5 + 集成 3+ = 23+ | service 3 + handler 3 + 集成 1 = 7 |
| 改动文件数 | ~13 文件 | 5-6 文件 |

本 story 是 7.3 之后**最简单**的一条 —— 几乎所有"基础设施"已落地（repo / handler 集合 / service 接口 / wire / 配置），只是给现有 stepsHandler / stepSvc 加一个新方法。

### Project Structure Notes

**与 `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 钦定的 server/ 工程结构对齐**：

```
server/
├─ internal/
│  ├─ app/
│  │  ├─ http/
│  │  │  ├─ handler/
│  │  │  │  └─ steps_handler.go       # 7.3 已建；本 story 末尾追加 GetAccount 方法 + getAccountResponseDTO helper
│  │  │  └─ middleware/
│  │  │     └─ auth.go / rate_limit.go / ...  # 已实装；GET /steps/account 与 POST /steps/sync 共享同一中间件链
│  │  └─ bootstrap/
│  │     └─ router.go                  # 7.3 已 wire stepsHandler；本 story 末尾追加 authedGroup.GET /steps/account 一行
│  ├─ service/
│  │  ├─ step_service.go               # 7.3 已建 SyncSteps；本 story 末尾追加 GetAccount 方法（interface + impl）
│  │  └─ step_service_test.go          # 7.3 已建 9 case；本 story 末尾追加 3 case
│  ├─ repo/
│  │  └─ mysql/
│  │     ├─ step_account_repo.go       # 7.3 已加 UpdateBalance；本 story 仅**消费** FindByUserID（**不**改）
│  │     └─ step_sync_log_repo.go      # 7.3 已建；本 story **不**调（GET 不读 sync_log）
│  └─ pkg/
│     └─ errors/codes.go               # 1003 ErrResourceNotFound 已注册；本 story 仅**消费**
└─ migrations/                         # 0001-0006 已锁定；本 story **不**改
```

**变更范围（预期 git status 文件清单，5-6 文件）**：

1. `server/internal/service/step_service.go` — 追加 GetAccount interface 方法 + impl
2. `server/internal/service/step_service_test.go` — 追加 3 case
3. `server/internal/app/http/handler/steps_handler.go` — 追加 GetAccount handler + getAccountResponseDTO
4. `server/internal/app/http/handler/steps_handler_test.go` — 追加 3 case + stubStepService.GetAccount
5. `server/internal/app/bootstrap/router.go` — 追加一行路由
6. `server/internal/service/step_service_integration_test.go` — 追加 1 集成 case
7. `_bmad-output/implementation-artifacts/sprint-status.yaml` — 7-4 状态 backlog → ready-for-dev → in-progress → review → done
8. `_bmad-output/implementation-artifacts/7-4-get-steps-account-接口.md` — 本 story 文件本身

**未变更文件（明确 NOT touch；超出范围 → HALT）**：

- `server/internal/repo/mysql/step_account_repo.go` / `step_sync_log_repo.go`
- `server/internal/repo/mysql/errors.go` / `step_account_repo_test.go` / `step_sync_log_repo_test.go`
- `server/internal/infra/config/config.go` / `loader.go` / 任一 `*.yaml`
- `server/internal/pkg/errors/codes.go`
- `server/internal/app/http/middleware/*.go`
- `server/cmd/server/main.go` / `internal/app/bootstrap/server.go`
- `migrations/*.sql`
- `docs/宠物互动App_*.md`（消费方）
- `_bmad-output/implementation-artifacts/decisions/*.md`（无新决策）

### References

**优先级 P0（必读）**：

- [Source: docs/宠物互动App_V1接口设计.md#6.2 获取步数账户] — 接口契约定义（行 603-660）；**特别**注意行 628 的"schema 嵌套差异"引用块
- [Source: docs/宠物互动App_V1接口设计.md#1] — 节点 3 接口冻结声明（行 23）
- [Source: docs/宠物互动App_数据库设计.md#5.4 user_step_accounts] — 表结构（4.6 firstTimeLogin 已建行）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 7.4] — 本 story 钦定 AC（行 1404-1414）
- [Source: server/internal/service/home_service.go] — "纯读 service 模式"参考（无事务 / NotFound 处理）
- [Source: server/internal/service/step_service.go] — 7.3 已建 stepServiceImpl + StepService interface（本 story 在末尾追加 GetAccount）

**优先级 P1（参考）**：

- [Source: server/internal/app/http/handler/home_handler.go] — handler 模式参考（c.Get UserIDKey + ctx 传播 + DTO 转换）
- [Source: server/internal/app/http/handler/steps_handler.go] — 7.3 已建 PostSync handler（本 story 在末尾追加 GetAccount）
- [Source: server/internal/repo/mysql/step_account_repo.go] — 已实装 FindByUserID（本 story **仅消费**）
- [Source: server/internal/repo/mysql/errors.go] — ErrStepAccountNotFound 哨兵定义
- [Source: server/internal/pkg/errors/codes.go] — 错误码全集（ErrResourceNotFound = 1003）
- [Source: server/internal/app/bootstrap/router.go] — 7.3 已 wire stepsHandler（本 story 复用实例）
- [Source: server/internal/service/step_service_test.go] — 7.3 已建 stub 模式 + 9 case（本 story 复用 stub + 末尾追加 3 case）
- [Source: server/internal/app/http/handler/steps_handler_test.go] — 7.3 已建 stubStepService + newStepsHandlerRouter（本 story 扩 stub + 末尾追加 3 case）
- [Source: server/internal/service/step_service_integration_test.go] — 7.3 已建 buildStepServiceIntegration + insertUser / insertStepAccount helper（本 story 复用 + 末尾追加 1 case）
- [Source: _bmad-output/implementation-artifacts/decisions/0006-error-mapping.md] — ADR-0006 三层错误映射（repo 哨兵 → service 翻译为 *AppError → handler c.Error）
- [Source: _bmad-output/implementation-artifacts/decisions/0007-context-propagation.md] — ADR-0007 ctx 传播

**优先级 P2（背景）**：

- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#5] — 分层职责定义
- [Source: docs/宠物互动App_总体架构设计.md] — "状态以 server 为准"原则
- [Source: _bmad-output/implementation-artifacts/7-3-post-steps-sync-接口-累计差值入账-service.md] — 7.3 完整实装文档（前 story；本 story 复用其落地的所有基础设施）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- 红绿循环顺序：先写 service 4 case 单测（RED：`go vet` 报 `svc.GetAccount undefined`）→ Edit step_service.go 加 interface 方法 + impl（GREEN：4 case 全过）→ 写 handler 4 case 单测（RED：`go vet` 报 `h.GetAccount undefined`）→ Edit steps_handler.go 加 GetAccount + getAccountResponseDTO（GREEN：4 case 全过）→ Edit router.go 一行 wire → Append 集成测试 1 case
- service 单测：4 case 全 PASS（happy / 1003 NotFound / all-zero / 1009 DB error）
- handler 单测：4 case 全 PASS（happy 扁平 schema / 1003 HTTP 200 / missing userID 1009 / service busy 1009）
- 集成测试：本机 docker 不可用 → SKIP（与 7.3 / 4.6 / 4.8 同模式）
- 全量回归：`bash scripts/build.sh --test` BUILD SUCCESS（22 包全过，无 regression）；`bash scripts/build.sh --integration` BUILD SUCCESS（docker 路径自动 skip）

### Completion Notes List

AC8 验证清单（8 项人工核对）：

- [x] **#1** service 层 GetAccount 流程严格按 V1 §6.2.4：FindByUserID → 拼装 StepAccountBrief；NotFound 哨兵 → 1003（**非** 1009）。**已核对** `internal/service/step_service.go` GetAccount 实装：`stderrors.Is(err, mysql.ErrStepAccountNotFound)` 分支返 `apperror.Wrap(err, apperror.ErrResourceNotFound, ...)`，其他 DB error 返 `apperror.Wrap(err, apperror.ErrServiceBusy, ...)`
- [x] **#2** handler `getAccountResponseDTO` 返**扁平** gin.H（顶级 totalSteps / availableSteps / consumedSteps）；**无** stepAccount 子对象。**已核对** `internal/app/http/handler/steps_handler.go` getAccountResponseDTO 实装与 postSyncResponseDTO 嵌套结构对比，符合 V1 §6.2 行 628 钦定差异
- [x] **#3** handler 复用 `c.Get(middleware.UserIDKey)` + `v.(uint64)` 兜底（与 home_handler / steps_handler.PostSync 完全同模式）。**已核对** GetAccount handler 取 userID 完全照抄 PostSync 模式
- [x] **#4** service 复用 `StepAccountBrief`（home_service.go 已定义）；**未**新建 GetAccountOutput / AccountBalance 等同义类型。**已核对** GetAccount 返回类型 `*StepAccountBrief`（与 home_service / step_service.SyncStepsOutput 共享同一类型）
- [x] **#5** bootstrap/router.go 仅追加一行 `authedGroup.GET("/steps/account", stepsHandler.GetAccount)`；**未**新建 stepsHandler / stepSvc 实例（复用 7.3 wire 的实例）。**已核对** router.go diff 仅 +1 行
- [x] **#6** service 单测 NotFound case 断 `*apperror.AppError.Code == ErrResourceNotFound (1003)`，**非** 1009。**已核对** TestStepService_GetAccount_AccountNotFound_Returns1003：`appErr.Code != apperror.ErrResourceNotFound` 断言
- [x] **#7** handler 单测 happy case 断 `data["stepAccount"]` 不存在 + `data["totalSteps"]` 顶级访问（扁平 schema 验证）。**已核对** TestStepsHandler_GetAccount_HappyPath_ReturnsFlatSchema：`if _, hasStepAccount := data["stepAccount"]; hasStepAccount { t.Errorf(...) }`
- [x] **#8** `bash scripts/build.sh --test` 全绿（含本 story 新增 8 case：service 4 + handler 4，超过 AC 钦定的 ≥3+≥3=≥6）；`git status --short` 改动文件清单匹配预期范围（5 server 源文件 + 1 集成测试 + 1 sprint-status + 1 story = 8 文件）

### File List

实装文件（5 个 server 文件）：

- `server/internal/service/step_service.go` — interface 末尾追加 GetAccount 签名 + godoc；文件末尾追加 `(s *stepServiceImpl) GetAccount` 实装段
- `server/internal/service/step_service_test.go` — 文件末尾追加 4 case（HappyPath / NotFound / AllZero / RepoOtherDBError）+ failOnSyncLogStub helper
- `server/internal/app/http/handler/steps_handler.go` — 文件末尾追加 `(h *StepsHandler) GetAccount` handler + `getAccountResponseDTO` helper
- `server/internal/app/http/handler/steps_handler_test.go` — stubStepService 字段 + 方法扩展；newStepsHandlerRouter 末尾追加 GET 路由；文件末尾追加 4 case（HappyPath_FlatSchema / 1003_HTTP200 / MissingUserID_1009 / ServiceBusy_1009）
- `server/internal/app/bootstrap/router.go` — 在 POST /steps/sync 那行之后追加 `authedGroup.GET("/steps/account", stepsHandler.GetAccount) // Story 7.4 加`

集成测试（1 个文件）：

- `server/internal/service/step_service_integration_test.go` — 文件末尾追加 `TestStepServiceIntegration_GetAccount_ReturnsLatestBalances`（初始查 + sync 一次 + 再查联动）

流程文件（2 个文件）：

- `_bmad-output/implementation-artifacts/sprint-status.yaml` — 7-4 状态 ready-for-dev → in-progress → review；last_updated 同步
- `_bmad-output/implementation-artifacts/7-4-get-steps-account-接口.md` — 本文件（Status / Tasks / Dev Agent Record / File List 更新）

### Change Log

| 日期 | 改动 | 状态变更 |
|---|---|---|
| 2026-05-02 | Story 7.4 created (backlog → ready-for-dev) | backlog → ready-for-dev |
| 2026-05-03 | dev-story 实装：GetAccount handler + service + 单测 8 + 集成 1 | ready-for-dev → in-progress → review |
