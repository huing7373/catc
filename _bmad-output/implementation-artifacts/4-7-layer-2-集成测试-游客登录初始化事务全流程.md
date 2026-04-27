# Story 4.7: Layer 2 集成测试 — 游客登录初始化事务全流程

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 资产事务负责人,
I want 一组深度集成测试覆盖游客登录初始化事务的**失败回滚 / 并发 / 边界 / 重入**全部 8 类场景，全部用 dockertest 真实 MySQL（**禁用** sqlmock）作为 Layer 2 集成测试的收尾保障,
so that NFR1 (资产事务原子) 和 `数据库设计.md` §8.1 (登录初始化事务) 在节点 2 阶段不只靠 Story 4.6 已有的 4 条 happy / 复用 / race / 不同 guestUid case，而是**穷举** epics.md §Story 4.7 行 1090-1104 钦定的 3 回滚 + 2 并发 + 2 边界 + 1 重入共 8 类场景，把覆盖率从局部 happy 路径推到事务全失败模式 + 高并发收敛 + handler 层边界拦截 + reuseLogin 重入 4 个维度全绿；任何一个场景退化（如某条回滚路径漏 rollback / 100 goroutine race 出现脏行 / 边界长度 129 在 handler 层未被拦截）→ 立即在 Layer 2 阶段被发现，**不**让节点 2 验收 demo 阶段才暴露资产事务幂等性回归。

## 故事定位（Epic 4 第八条 = 节点 2 收尾性 Layer 2 集成测试；上承 4.6 游客登录初始化事务 + 4.8 GET /home 聚合接口；epic 收官 story）

- **Epic 4 进度**：4.1 (契约定稿，done) → 4.2 (MySQL + tx manager，done) → 4.3 (5 张表 migrations，done) → 4.4 (token util，done) → 4.5 (auth + rate_limit 中间件，done) → 4.6 (游客登录 handler + 首次初始化事务，done) → 4.8 (GET /home 聚合接口，done) → **4.7 (本 story，Layer 2 集成测试，epic 收官)**。
- **物理执行顺序与逻辑编号错位**：本 story 编号 4.7 但**物理上最后**执行（`4.6 → 4.8 → 4.7`）。理由：
  - Story 4.7 是 epic-4 的**收尾性 Layer 2 集成测试**，需要 4.6 (auth_service) + 4.8 (home_service) 两条业务链路都落地后再做整体回归
  - sprint-status.yaml 第 75-83 行已按此顺序排列（4.7 在 4.8 之后、epic-4-retrospective 之前）
  - 原 Story 4.9 占位已删除，4.7 兼任 epic 收官
  - 见 `_bmad-output/planning-artifacts/epics.md` §Epic 4 Overview 行 931 钦定执行顺序
- **epics.md AC 钦定**（`_bmad-output/planning-artifacts/epics.md` §Story 4.7 行 1084-1104，**唯一权威 AC 来源**）：
  - 输出 `internal/service/auth_service_integration_test.go` 用 dockertest 起真实 MySQL，覆盖**8 个场景**：
    1. **回滚 1**: mock pet repo 第 3 步抛 error → 验证 users / bindings 也回滚（DB 表为空）
    2. **回滚 2**: mock chest repo 第 5 步抛 error → 验证前 4 步全部回滚
    3. **回滚 3**: mock user repo 第 1 步抛 error → 验证什么都没创建
    4. **并发 1**: 100 个 goroutine 并发用同一 guestUid 调用 → 最终 DB 只有 1 个 user，所有 goroutine 拿到同一 user_id
    5. **并发 2**: 100 个 goroutine 并发用 100 个不同 guestUid → DB 100 个 user，每个 user 5 行关联数据，无串数据
    6. **边界 1**: guestUid 长度 128（最大允许）→ 成功
    7. **边界 2**: guestUid 长度 129 → 失败（**在 handler 层就拦截**）
    8. **重入**: 同一 guestUid 已成功登录 → 第二次调用走复用分支，DB 行数不变
  - 全部场景用 dockertest 真实 MySQL 跑通（**不**用 sqlmock —— 业务上是 Layer 2 黑盒事务行为验证，不是 SQL 字符串验证）
  - 集成测试在 CI 标 `// +build integration` tag（与 4.6 / 4.8 同模式），本地 `go test -tags integration ./...` 跑
- **范围边界**（**关键** —— 与 4.6 已有集成测试的明确分工）：

  **Story 4.6 集成测试已落地 4 case**（`server/internal/service/auth_service_integration_test.go`，已 done）：
  - `TestAuthService_GuestLogin_FirstTime_CreatesFiveRows` — happy 链路 → 5 行各新增 1 行
  - `TestAuthService_GuestLogin_SameGuestUID_ReturnsSameUserIDWithoutDup` — 同 guestUid 第二次调用 → 不新增
  - `TestAuthService_GuestLogin_ConcurrentSameGuestUID_BothSucceedSameUserID` — 2 goroutine 同 guestUid（review-r1 lesson 回归测试）
  - `TestAuthService_GuestLogin_DifferentGuestUID_CreatesNewFiveRows` — 不同 guestUid → 再新增 5 行

  **Story 4.7 本 story 任务是扩展上述文件加 8 case**（追加 8 个钦定场景到同一份 `auth_service_integration_test.go`，**不**新建独立测试文件 —— 同包同模式内聚，复用 startMySQL/runMigrations/buildAuthService/assertCount helper）：

  | epics.md 钦定场景 | 测试函数命名 | 与 4.6 现有 case 关系 |
  |---|---|---|
  | 回滚 1 (pet 失败) | `TestAuthService_GuestLogin_PetRepoFailsTx_AllRowsRollback` | 新增 |
  | 回滚 2 (chest 失败) | `TestAuthService_GuestLogin_ChestRepoFailsTx_AllRowsRollback` | 新增 |
  | 回滚 3 (user 失败) | `TestAuthService_GuestLogin_UserRepoFailsTx_NoRowsCreated` | 新增 |
  | 并发 1 (100 同 guestUid) | `TestAuthService_GuestLogin_Concurrent100SameGuestUID_OnlyOneUser` | **强化** 4.6 已有 2 goroutine 测试到 100 goroutine（不替代，新加） |
  | 并发 2 (100 不同 guestUid) | `TestAuthService_GuestLogin_Concurrent100DifferentGuestUIDs_NoCrossData` | 新增 |
  | 边界 1 (128 字符 happy) | `TestAuthService_GuestLogin_GuestUIDExactly128Chars_Succeeds` | 新增（service 层入口） |
  | 边界 2 (129 字符 handler 拦截) | `TestAuthHandler_GuestLogin_GuestUIDExactly129Chars_Returns1002` | 新增（**handler 层** —— epics.md 行 1101 钦定 "在 handler 层就拦截"） |
  | 重入 (二次调用走复用) | `TestAuthService_GuestLogin_ReentryAfterSuccess_ReusesWithoutNewRows` | 与 4.6 `SameGuestUID` 重叠但**断言更深**：必须断言走 reuseLogin 分支 + 5 张表无任何新行 + token 是新的 |

  **关键设计约束**：
  - 8 case 全部 build tag `integration`（与 4.6 / 4.8 同模式）
  - **回滚 1/2/3 必须用 fault injection**（包装真实 mysql repo + 在指定步骤前抛 sentinel error），不能用 stub repo —— 因为 stub repo 不真开 MySQL 事务，无法验证 InnoDB rollback 真实行为
  - **并发 1 必须 100 goroutine**（不能用 4.6 已有的 2 goroutine 替代），按 epics.md 行 1098 钦定数字
  - **边界 2 必须挂在 handler 层验证**（不在 service 层 —— epics.md 行 1101 明确说 "在 handler 层就拦截"），所以本 story 测试同时跨 handler + service 包
  - **重入 case** 必须断言 reuseLogin 路径走通（token 是新签发的、user_id 相同、5 张表行数完全不变），区分于 4.6 `SameGuestUID` 的浅层 user_id 相等断言
- **下游依赖**：本 story 是 epic 4 收尾，**不**直接服务下游 story；但本 story 的 fault injection 模式（包装真 repo + sentinel error 注入）可能成为 future Layer 2 集成测试的范式（如 Story 11.9 房间事务集成测试 / Story 20.9 开箱事务集成测试 / Story 26.5 穿戴事务集成测试 / Story 32.5 合成事务集成测试 都钦定相同 Layer 2 模式）。

**本 story 不做**（明确范围红线）：

- [skip] **不**修改 `server/internal/service/auth_service.go`（4.6 已 done；本 story 仅消费）
- [skip] **不**修改 `server/internal/app/http/handler/auth_handler.go`（4.6 已 done；本 story 仅消费）
- [skip] **不**修改 5 个 mysql repo（4.6 / 4.8 已 done；本 story 仅消费 + 包装做 fault injection）
- [skip] **不**修改 5 张表 migrations（4.3 已 done；本 story 仅消费）
- [skip] **不**修改 `server/internal/repo/tx/manager.go`（4.2 已 done；本 story 仅消费 `WithTx`）
- [skip] **不**修改 4.5 中间件（auth + rate_limit；本 story 仅在边界 2 case 中通过 router wire 间接消费）
- [skip] **不**修改 bootstrap router（**不**新增 deps 字段；不挂新路由）
- [skip] **不**修改 4.6 已有的 4 个 happy case 测试函数（保持现有 done 状态测试不破坏 —— 仅在同一份 `auth_service_integration_test.go` 文件**追加** 8 个新 case）
- [skip] **不**新建跨包 testing util（不抽 startMySQL 到 internal/testutil/ —— 与 4.6 / 4.8 同模式，复制到本测试文件即可，避免范围扩散）
- [skip] **不**用 sqlmock（epics.md 行 1103 钦定 "全部场景用 dockertest 真实 MySQL 跑通"；sqlmock 测的是 SQL 字符串匹配，与本 story Layer 2 黑盒行为验证语义不符）
- [skip] **不**改 `docs/宠物互动App_*.md` 任一份（V1 §4.1 / 数据库设计 §5/§8.1 / 设计文档 §5 是契约**输入**，本 story 严格对齐**不**修改）
- [skip] **不**写 README / 部署文档：留 Epic 4 收尾或 Story 6.3 文档同步阶段
- [skip] **不**实装 audit log（依赖 Logging 中间件兜底）
- [skip] **不**修改 `server/configs/local.yaml`（不引入新配置项）
- [skip] **不**修改 `server/cmd/server/main.go`（不加新 deps）
- [skip] **不**支持 `go test -short`（dockertest 必跑；本 story 8 case 全部 `+build integration`，默认 `bash scripts/build.sh --test` 不触发；只在 `--integration` 触发）
- [skip] **不**实装"测试容器复用"优化（每 case 独立 startMySQL 容器，与 4.6 / 4.8 同模式，简单 + 一致性优于性能）；优化方向留 future 性能 epic
- [skip] **不**给 Story 4.7 加 sprint-status.yaml 占位 retrospective（epic 4 retrospective 已在 sprint-status.yaml 第 84 行，本 story done 后整 epic done 才推 retrospective）
- [skip] **不**做 fuzz / property-based testing（dockertest case 已穷举 epics.md 钦定 8 类；fuzz 是 future testing 升级范畴）
- [skip] **不**测 ctx cancel / timeout 路径（ADR-0007 ctx 传播是 4.2-4.6 已建立的范式，本 story 不重复验证）
- [skip] **不**测 deadlock / 隔离级别 anomaly（InnoDB 默认 REPEATABLE READ + 本 story 100 goroutine 同 guestUid race 已经触发 insert intent gap lock；不深挖隔离级别专项）

## Acceptance Criteria

**AC1 — 测试文件位置 + build tag + helper 复用**

本 story 在已有 `server/internal/service/auth_service_integration_test.go`（4.6 落地）**追加** 8 个新测试函数；**不**新建独立测试文件。

**关键约束**：

- **build tag**：所有 8 个新 case 必须挂 `//go:build integration` + `// +build integration` 双行标记（与 4.6 / 4.8 同模式 + Go 1.17+ 双语法兼容）；放在文件顶部（4.6 已写）
- **helper 复用**：直接消费同包同文件已有的 `startMySQL` / `runMigrations` / `buildAuthService` / `assertCount` helper（4.6 实装完整）
- **不抽包**：`startMySQL` 等 helper 仍在本测试文件内（每 case 独立起容器，与 4.6 / 4.8 同模式 —— 跨包 testing util 抽离是 future scaling 决策，本 story 不做）
- **包名不变**：`package service_test`（外部测试包，与 4.6 同）

**关键反模式**：

- ❌ **不**新建 `auth_service_integration_rollback_test.go` / `auth_service_integration_concurrency_test.go` 等拆文件（保持 4.6 单文件内聚 —— 一份测试文件视图覆盖所有场景，便于 reviewer 一目了然）
- ❌ **不**在本测试文件内新加 `package mysql_test` 或 `package handler_test` 区块（边界 2 在 handler 层验证 → 单独文件 `auth_handler_integration_test.go`，详见 AC8）
- ❌ **不**用 sqlmock（epics.md 行 1103 钦定）

**AC2 — 回滚 1: pet repo 第 3 步失败 → 整体回滚**

新增测试函数 `TestAuthService_GuestLogin_PetRepoFailsTx_AllRowsRollback`：

```go
// AC2 测试函数框架（位于 auth_service_integration_test.go）
func TestAuthService_GuestLogin_PetRepoFailsTx_AllRowsRollback(t *testing.T) {
    // 起容器 + migrate
    dsn, dockerCleanup := startMySQL(t)
    defer dockerCleanup()
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
        t.Fatalf("db.Open: %v", err)
    }

    rawDB, err := gormDB.DB()
    if err != nil {
        t.Fatalf("gormDB.DB(): %v", err)
    }
    defer rawDB.Close()

    signer, err := auth.New("test-secret-must-be-at-least-16-bytes", 3600)
    if err != nil {
        t.Fatalf("auth.New: %v", err)
    }

    // **关键**：用真实 mysql repo（非 stub）—— 必须经过真实 InnoDB 事务 +
    // GORM driver 才能验证 rollback 真行为；只在 PetRepo.Create 上包装 fault injection
    txMgr := tx.NewManager(gormDB)
    userRepo := mysql.NewUserRepo(gormDB)
    bindingRepo := mysql.NewAuthBindingRepo(gormDB)
    petRepoFault := &faultPetRepo{
        delegate: mysql.NewPetRepo(gormDB),
        injectErr: errors.New("synthetic pet repo failure"), // 任何非 sentinel 的 error → service 包 1009
    }
    stepRepo := mysql.NewStepAccountRepo(gormDB)
    chestRepo := mysql.NewChestRepo(gormDB)

    svc := service.NewAuthService(txMgr, signer, userRepo, bindingRepo, petRepoFault, stepRepo, chestRepo)

    _, err = svc.GuestLogin(context.Background(), service.GuestLoginInput{
        GuestUID: "uid-rollback-pet", Platform: "ios", AppVersion: "1.0.0", DeviceModel: "iPhone15,2",
    })

    // service 必须返 1009（事务回滚后包装 ErrServiceBusy）
    if err == nil {
        t.Fatalf("expected error, got nil (rollback path 没触发)")
    }
    var appErr *apperror.AppError
    if !errors.As(err, &appErr) || appErr.Code != apperror.ErrServiceBusy {
        t.Fatalf("expected AppError code=1009, got %v", err)
    }

    // **核心断言**：5 张表全部为空（前 2 步 INSERT users + binding 都被回滚）
    assertCount(t, rawDB, "users", nil, 0, "users (rollback)")
    assertCount(t, rawDB, "user_auth_bindings", nil, 0, "bindings (rollback)")
    assertCount(t, rawDB, "pets", nil, 0, "pets (not reached)")
    assertCount(t, rawDB, "user_step_accounts", nil, 0, "step_accounts (not reached)")
    assertCount(t, rawDB, "user_chests", nil, 0, "chests (not reached)")
}
```

**fault injection 设计**（同文件内新增 helper struct）：

```go
// faultPetRepo 包装真实 PetRepo，让 Create 抛入参 injectErr，其他方法透传。
//
// 模式：MVP 用"按方法包装"，不引入第三方 fault injection 框架（gomonkey / monkey 等）。
// 优点：编译期检查 + 与 4.6 stub repo 同模式 + 跨平台无依赖（gomonkey 在 ARM 不工作）。
type faultPetRepo struct {
    delegate  mysql.PetRepo
    injectErr error
}

func (f *faultPetRepo) Create(ctx context.Context, p *mysql.Pet) error {
    return f.injectErr
}

func (f *faultPetRepo) FindDefaultByUserID(ctx context.Context, userID uint64) (*mysql.Pet, error) {
    return f.delegate.FindDefaultByUserID(ctx, userID)
}
```

**关键设计约束**：

- **fault injection 必须包装真实 repo（不是 stub）**：service.firstTimeLogin 在 fn 内调用 PetRepo.Create —— faultPetRepo.Create 直接返 injectErr → fn 返 error → tx.WithTx 触发 InnoDB ROLLBACK → 真实验证回滚行为
- **injectErr 用 generic errors.New(...)**（不用 mysql.ErrAuthBindingDuplicate 等 sentinel）—— 让 service 走 1009 默认分支（ADR-0006 第三层映射），不触发 ErrAuthBindingDuplicate / ErrUsersGuestUIDDuplicate 的 reuseLogin 兜底
- **延迟 cleanup 顺序**：`defer rawDB.Close()` 必须在 `defer dockerCleanup()` **之后**注册（按栈序先关 db pool 再 purge 容器；否则 purge 后 close 会报错日志）
- **AppError 断言用 errors.As**：`*apperror.AppError` 是 internal/pkg/errors 包的具体类型，service 用 `apperror.Wrap(err, ErrServiceBusy, ...)` 产出（AC2-AC4 三个回滚 case 共用此断言模式）
- **5 张表都断言 count=0**：前两步 users / binding 已 INSERT 但 ROLLBACK；后三步 pets / step / chest 没到达 —— 全部 count=0 验证事务原子性（**不是**部分降级）

**关键反模式**：

- ❌ **不**用 `*apperror.AppError` 直接 == 比较（apperror.Wrap 会包装多层 error，必须 errors.As 解开）
- ❌ **不**抽 faultPetRepo 到独立文件（保持本测试文件内聚；其他 fault\*Repo 同位置）
- ❌ **不**在 faultPetRepo.Create 内打 t.Log / fmt.Println（测试输出污染 + 无意义 —— 测试失败时 assertCount 会报具体表 count）
- ❌ **不**用 testify mock / gomock（与 4.5 / 4.6 / 4.8 同模式 —— 显式 stub struct + 函数字段）

**AC3 — 回滚 2: chest repo 第 5 步失败 → 前 4 步全部回滚**

新增测试函数 `TestAuthService_GuestLogin_ChestRepoFailsTx_AllRowsRollback`：

```go
func TestAuthService_GuestLogin_ChestRepoFailsTx_AllRowsRollback(t *testing.T) {
    // ... 同 AC2 setup ...

    // 用真实 4 个 repo + 仅在 ChestRepo.Create 注入 fault
    chestRepoFault := &faultChestRepo{
        delegate:  mysql.NewChestRepo(gormDB),
        injectErr: errors.New("synthetic chest repo failure"),
    }
    svc := service.NewAuthService(txMgr, signer, userRepo, bindingRepo, petRepo, stepRepo, chestRepoFault)

    _, err = svc.GuestLogin(ctx, service.GuestLoginInput{GuestUID: "uid-rollback-chest", ...})

    // 断言 1009 + 5 张表全空
    require AppError code=1009
    assertCount: users=0, bindings=0, pets=0, step_accounts=0, chests=0
}
```

**fault helper struct**（追加同文件）：

```go
type faultChestRepo struct {
    delegate  mysql.ChestRepo
    injectErr error
}

func (f *faultChestRepo) Create(ctx context.Context, c *mysql.UserChest) error {
    return f.injectErr
}

// FindByUserID 透传给 delegate（home_service 也用本 repo；保持 interface 完整满足编译）
func (f *faultChestRepo) FindByUserID(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
    return f.delegate.FindByUserID(ctx, userID)
}
```

**关键约束**：

- **回滚 2 的语义重点**：fn 在第 5 步（chest INSERT）失败 → 前 4 步（users INSERT + UpdateNickname + binding INSERT + pet INSERT + step_account INSERT）**全部**已经写入但未 commit；ROLLBACK 后 5 张表全空。这与回滚 1 的差异：回滚 1 fn 在第 3 步失败，前 2 步 + UpdateNickname 都已 INSERT 但 pet/step/chest 没到达；本 case 验证"事务最末步失败" rollback 行为
- **users / binding / pet / step 都必须 count=0**：4 表的 INSERT 都已发生但未 commit，ROLLBACK 后 0 行（验证 InnoDB undo log 完整 rollback）
- **必须验证 ChestRepo interface 完整实装**：因为 4.8 已扩了 `FindByUserID`，本 fault\*Repo 必须实装两个方法（仅 Create 注入 fault）

**AC4 — 回滚 3: user repo 第 1 步失败 → 什么都没创建**

新增测试函数 `TestAuthService_GuestLogin_UserRepoFailsTx_NoRowsCreated`：

```go
func TestAuthService_GuestLogin_UserRepoFailsTx_NoRowsCreated(t *testing.T) {
    // ... 同 AC2 setup ...

    userRepoFault := &faultUserRepo{
        delegate:  mysql.NewUserRepo(gormDB),
        injectErr: errors.New("synthetic user repo failure"),
    }
    svc := service.NewAuthService(txMgr, signer, userRepoFault, bindingRepo, petRepo, stepRepo, chestRepo)

    _, err = svc.GuestLogin(ctx, service.GuestLoginInput{GuestUID: "uid-rollback-user", ...})

    // 断言 1009 + 5 张表全空（**第一步就失败 → 后续全部未到达**）
    require AppError code=1009
    assertCount: users=0, bindings=0, pets=0, step_accounts=0, chests=0
}
```

**fault helper struct**（追加同文件）：

```go
type faultUserRepo struct {
    delegate  mysql.UserRepo
    injectErr error
}

func (f *faultUserRepo) Create(ctx context.Context, u *mysql.User) error {
    return f.injectErr
}

// UpdateNickname / FindByID 透传给 delegate（home_service / reuseLogin 也用本 repo）
func (f *faultUserRepo) UpdateNickname(ctx context.Context, userID uint64, nickname string) error {
    return f.delegate.UpdateNickname(ctx, userID, nickname)
}

func (f *faultUserRepo) FindByID(ctx context.Context, id uint64) (*mysql.User, error) {
    return f.delegate.FindByID(ctx, id)
}
```

**关键约束**：

- **回滚 3 的语义重点**：fn 在第 1 步就失败 → 没有任何 INSERT 发生（回滚 1/2 是"已 INSERT 后 rollback"；本 case 是"INSERT 都没 issue"）；rawDB 数据库 5 表全空，**不是因为 rollback**而是**因为没到达**
- **fault user repo 必须实装 UserRepo interface 全部 3 个方法**（Create / UpdateNickname / FindByID）：UpdateNickname 是 firstTimeLogin 第 2 步；FindByID 是 reuseLogin 路径用（本 case 不走 reuseLogin，但保持 interface 完整 —— 防 compile error）
- **断言全空**：与 AC2 / AC3 文字结果相同（5 表 count=0），但语义来源不同（"未到达" vs "rollback"）；测试不区分两种语义来源，只断言 DB 状态最终一致

**AC5 — 并发 1: 100 goroutine 同 guestUid → 1 user，所有 goroutine 拿到同一 user_id**

新增测试函数 `TestAuthService_GuestLogin_Concurrent100SameGuestUID_OnlyOneUser`：

```go
func TestAuthService_GuestLogin_Concurrent100SameGuestUID_OnlyOneUser(t *testing.T) {
    svc, sqlDB, cleanup := buildAuthService(t)
    defer cleanup()

    const guestUID = "uid-concurrent-100-same"
    const N = 100
    type result struct {
        userID uint64
        err    error
    }
    results := make([]result, N)

    var wg sync.WaitGroup
    wg.Add(N)
    for i := 0; i < N; i++ {
        i := i
        go func() {
            defer wg.Done()
            out, err := svc.GuestLogin(context.Background(), service.GuestLoginInput{
                GuestUID:    guestUID,
                Platform:    "ios",
                AppVersion:  "1.0.0",
                DeviceModel: "iPhone15,2",
            })
            if err != nil {
                results[i] = result{err: err}
                return
            }
            results[i] = result{userID: out.UserID}
        }()
    }
    wg.Wait()

    // 断言 1：所有 100 goroutine 都成功（任一失败 → 幂等回归）
    var firstUserID uint64
    for i, r := range results {
        if r.err != nil {
            t.Fatalf("goroutine %d returned error %v (race 路径必须 reuseLogin 而非 1009)", i, r.err)
        }
        if i == 0 {
            firstUserID = r.userID
        } else if r.userID != firstUserID {
            t.Errorf("goroutine %d userID=%d ≠ first userID=%d (违反幂等)", i, r.userID, firstUserID)
        }
    }

    // 断言 2：DB 最终状态 1 个 user / 1 行 binding / 1 行 pet / 1 行 step / 1 行 chest
    assertCount(t, sqlDB, "users WHERE guest_uid=?", []any{guestUID}, 1, "users (only 1)")
    assertCount(t, sqlDB, "user_auth_bindings WHERE auth_type=1 AND auth_identifier=?", []any{guestUID}, 1, "bindings (only 1)")
    assertCount(t, sqlDB, "pets WHERE user_id=?", []any{firstUserID}, 1, "pets (only 1)")
    assertCount(t, sqlDB, "user_step_accounts WHERE user_id=?", []any{firstUserID}, 1, "step (only 1)")
    assertCount(t, sqlDB, "user_chests WHERE user_id=?", []any{firstUserID}, 1, "chests (only 1)")
}
```

**关键设计约束**：

- **N=100 必须**（epics.md 行 1098 钦定）：4.6 已有 2 goroutine race 测试是 review-r1 lesson 的最小回归；本 case 把负载推到 100，让 InnoDB insert intent gap lock 路径与 service-level reuseLogin fallback 路径都被高强度撞击 —— 暴露任何"在 50 / 100 之类规模下退化"的边缘 timing
- **断言所有 100 都成功**（任一返 1009 → 立即 fail）：这是 V1 §4.1 钦定 "同 guestUid → 同 user_id" 幂等的强语义；review-r1 lesson `2026-04-26-multi-table-tx-must-cover-all-unique-constraint-races.md` 已修复 ErrUsersGuestUIDDuplicate 兜底，本 case 在 100 规模回归
- **断言所有 100 拿到同一 user_id**（不只是 DB 1 行 user，要求所有 goroutine 的输出 UserID 一致）：这是 V1 §4.1 行 141 钦定 "同 guestUid 重复调用 → 同一 user_id" 的全展开断言
- **DB 状态 5 张表各 1 行**：100 个并发请求最终落库 1 行 user + 1 行 binding + 1 行 pet + 1 行 step + 1 行 chest（自然幂等通过 UNIQUE INDEX 实施）
- **不**断言谁是 winner（哪个 goroutine 先 commit）：InnoDB insert intent gap lock 是非确定性的；只断言收敛结果（1 user + 同 user_id）
- **不**测 race detector（Windows skip + 100 goroutine 在 race detector 下慢 10x，超 120s timeout）：本 case 跑业务级 race，不是 Go memory model race；race detector 是 4.2-4.6 已建立的测试栈范式（ADR-0001 §3.5），本 story 不变

**关键反模式**：

- ❌ **不**用 `t.Parallel()` 跑这个 case 与其他 case 并行：dockertest 容器有限资源；100 goroutine 已经压榨 MySQL InnoDB lock，并行其他 case 会引入互相 timing 干扰
- ❌ **不**减少 N 到 10 / 50 加速测试：epics.md 钦定 100；减少会让 race coverage 弱化，未来可能漏 50+ 才出现的 timing 边缘
- ❌ **不**用 channel 替代 sync.WaitGroup（WaitGroup 在 N=100 下更直观；channel 需要额外的 close + drain 模式）
- ❌ **不**断言 `time.Now()` 内的时长（dockertest + InnoDB 在不同 host 上 timing 差异大，断时间会 flaky）

**AC6 — 并发 2: 100 goroutine 100 不同 guestUid → 100 user，每个 5 行关联，无串数据**

新增测试函数 `TestAuthService_GuestLogin_Concurrent100DifferentGuestUIDs_NoCrossData`：

```go
func TestAuthService_GuestLogin_Concurrent100DifferentGuestUIDs_NoCrossData(t *testing.T) {
    svc, sqlDB, cleanup := buildAuthService(t)
    defer cleanup()

    const N = 100
    type result struct {
        guestUID string
        userID   uint64
        petID    uint64
        err      error
    }
    results := make([]result, N)

    var wg sync.WaitGroup
    wg.Add(N)
    for i := 0; i < N; i++ {
        i := i
        guestUID := fmt.Sprintf("uid-diff-%03d", i) // uid-diff-000 ... uid-diff-099
        go func() {
            defer wg.Done()
            out, err := svc.GuestLogin(context.Background(), service.GuestLoginInput{
                GuestUID:    guestUID,
                Platform:    "ios",
                AppVersion:  "1.0.0",
                DeviceModel: "iPhone15,2",
            })
            if err != nil {
                results[i] = result{guestUID: guestUID, err: err}
                return
            }
            results[i] = result{guestUID: guestUID, userID: out.UserID, petID: out.PetID}
        }()
    }
    wg.Wait()

    // 断言 1：所有 100 都成功
    seenUIDs := make(map[uint64]string, N)
    seenPetIDs := make(map[uint64]struct{}, N)
    for i, r := range results {
        if r.err != nil {
            t.Fatalf("goroutine %d guestUID=%s err=%v", i, r.guestUID, r.err)
        }
        // 断言 2：100 个 user_id 互不相同
        if existing, dup := seenUIDs[r.userID]; dup {
            t.Errorf("userID %d collision: guestUID %s ↔ %s", r.userID, r.guestUID, existing)
        }
        seenUIDs[r.userID] = r.guestUID
        // 断言 3：100 个 pet_id 互不相同（每个 user 的默认猫独立）
        if _, dup := seenPetIDs[r.petID]; dup {
            t.Errorf("petID %d collision (guestUID %s)", r.petID, r.guestUID)
        }
        seenPetIDs[r.petID] = struct{}{}
    }

    // 断言 4：DB 总计 5 张表各 100 行
    assertCount(t, sqlDB, "users", nil, N, "users (100 distinct)")
    assertCount(t, sqlDB, "user_auth_bindings", nil, N, "bindings (100 distinct)")
    assertCount(t, sqlDB, "pets", nil, N, "pets (100 distinct)")
    assertCount(t, sqlDB, "user_step_accounts", nil, N, "step (100 distinct)")
    assertCount(t, sqlDB, "user_chests", nil, N, "chests (100 distinct)")

    // 断言 5：每个 user 的关联数据完整（关键 —— 验证"无串数据"）
    for _, r := range results {
        // 每个 user_id 在 5 张表里都恰好 1 行（guestUid 匹配该 user_id）
        assertCount(t, sqlDB, "users WHERE id=? AND guest_uid=?", []any{r.userID, r.guestUID}, 1, "user "+r.guestUID)
        assertCount(t, sqlDB, "user_auth_bindings WHERE user_id=? AND auth_identifier=?", []any{r.userID, r.guestUID}, 1, "binding "+r.guestUID)
        assertCount(t, sqlDB, "pets WHERE user_id=? AND is_default=1", []any{r.userID}, 1, "pet "+r.guestUID)
        assertCount(t, sqlDB, "user_step_accounts WHERE user_id=?", []any{r.userID}, 1, "step "+r.guestUID)
        assertCount(t, sqlDB, "user_chests WHERE user_id=?", []any{r.userID}, 1, "chest "+r.guestUID)
    }
}
```

**关键设计约束**：

- **"无串数据"语义解释**：100 个 goroutine 并发各创建自己的 user，必须保证 user_id_A 的 binding 不会指向 user_B / user_id_A 的 pet 不会有 user_B 的 user_id —— 即每个 user 的 4 行关联（binding / pet / step / chest）都和 user.id 一一对应；这是 NFR1 资产事务原子的"无跨 user 数据污染"维度
- **断言 5 是核心**：对每个 result.userID 单独断言"4 张关联表都恰好 1 行 + user_id 字段匹配"；如果事务发生数据串味（如 transaction 内 user.ID 用错了 —— 比如用了上一次 fn 里的 leftover ID），断言 5 会立即捕获
- **断言 user_id 唯一**（seenUIDs map）：100 个 AUTO_INCREMENT user.id 必然互不相同；这是基础 sanity check 防 GORM 行为漂移
- **断言 pet_id 唯一**（seenPetIDs map）：每个 user 的默认猫是独立 pet 行，pet.id AUTO_INCREMENT 必然互不相同
- **不需要担心 reuseLogin**：100 个 guestUid 互不相同 → 全部走 firstTimeLogin 分支 → 全部新建（与并发 1 的语义对照）
- **N=100 必须**（epics.md 行 1099 钦定）

**关键反模式**：

- ❌ **不**用 `t.Run(fmt.Sprintf("uid-%d", i), ...)` 拆 100 个 subtest（dockertest 容器一次性 setup；100 个 subtest 会让 t.Failed() 输出膨胀，且 setup 重复）
- ❌ **不**断言每个 user 的 nickname == "用户" + strconv.FormatUint(userID, 10)（这是 4.6 happy case 已断言；本 case 重点在并发隔离 + 关联数据完整）
- ❌ **不**用 fmt.Sprintf("uid-%d", i) 而用 `fmt.Sprintf("uid-diff-%03d", i)`（前者在 i=10 时得 "uid-10"，i=100 时得 "uid-100"，长度跳变；后者固定 11 字符，方便 debug 时按字符串排序看）

**AC7 — 边界 1: guestUid 长度 128（最大允许）→ 成功**

新增测试函数 `TestAuthService_GuestLogin_GuestUIDExactly128Chars_Succeeds`：

```go
func TestAuthService_GuestLogin_GuestUIDExactly128Chars_Succeeds(t *testing.T) {
    svc, sqlDB, cleanup := buildAuthService(t)
    defer cleanup()

    // 构造正好 128 字符的 guestUID（utf8.RuneCountInString = 128）
    // 用 ASCII 字符（每字符 1 字节）—— 与 utf8mb4 多字节字符的覆盖留 future fuzz
    guestUID128 := strings.Repeat("a", 128)
    if utf8.RuneCountInString(guestUID128) != 128 {
        t.Fatalf("setup error: guestUID rune count = %d, want 128", utf8.RuneCountInString(guestUID128))
    }

    out, err := svc.GuestLogin(context.Background(), service.GuestLoginInput{
        GuestUID:    guestUID128,
        Platform:    "ios",
        AppVersion:  "1.0.0",
        DeviceModel: "iPhone15,2",
    })
    if err != nil {
        t.Fatalf("expected success at 128 chars boundary, got err=%v", err)
    }

    // 断言 5 张表各 1 行 + guestUID 完整存储（128 字符未被 MySQL VARCHAR(128) 截断）
    assertCount(t, sqlDB, "users WHERE guest_uid=?", []any{guestUID128}, 1, "users (128 char)")
    assertCount(t, sqlDB, "user_auth_bindings WHERE auth_type=1 AND auth_identifier=?", []any{guestUID128}, 1, "binding (128 char)")
    assertCount(t, sqlDB, "pets WHERE user_id=?", []any{out.UserID}, 1, "pet (128 char)")
    assertCount(t, sqlDB, "user_step_accounts WHERE user_id=?", []any{out.UserID}, 1, "step (128 char)")
    assertCount(t, sqlDB, "user_chests WHERE user_id=?", []any{out.UserID}, 1, "chest (128 char)")

    // 验证 guestUid 字段完整存储（**关键** —— 防 MySQL 静默截断到 127 / 64）
    var stored string
    err = sqlDB.QueryRow("SELECT guest_uid FROM users WHERE id=?", out.UserID).Scan(&stored)
    if err != nil {
        t.Fatalf("query stored guest_uid: %v", err)
    }
    if stored != guestUID128 {
        t.Errorf("stored guestUid len=%d, want 128 (full storage)", len(stored))
    }
}
```

**关键设计约束**：

- **service 层入口**（不走 handler）：epics.md 行 1100 钦定 "guestUid 长度 128（最大允许）→ 成功"，service 层不做长度校验（handler 层做）—— 本 case 直接调 svc.GuestLogin 验证 service / repo / DB 链路在 128 字符下不退化
- **必须验证存储完整**（不只是返 nil err）：MySQL VARCHAR(128) 在 STRICT_TRANS_TABLES 模式下超长会拒绝，但若历史 schema 配错（如不小心写成 VARCHAR(64)）会**静默截断**；本 case 通过 SELECT guest_uid 字段长度断言完整存储 —— 等于回归测试 schema 字段长度配置
- **strings.Repeat("a", 128)**：用 ASCII（utf8mb4 1 字节）避免 utf8mb4 多字节字符与 MySQL VARCHAR(N) "N 是字符还是字节" 的歧义；中文字符串边界由 future epic fuzz 覆盖
- **utf8.RuneCountInString 显式校验**（`if != 128 { t.Fatalf }`）：防 setup 错误（typo 写成 127 / 129 而 t.Run 不报错）

**关键反模式**：

- ❌ **不**用中文 + 中文混 ASCII 凑 128 字符（utf8mb4 解析存在多种字节宽度，setup 误差大；ASCII 是最简单可靠的边界）
- ❌ **不**通过 handler 走（handler 边界 case 在 AC8 单独）
- ❌ **不**用 INSERT INTO users SQL 直接验证（绕过 service 不能验证 service / repo 的实际行为）

**AC8 — 边界 2: guestUid 长度 129 → handler 层拦截 1002**

**关键边界点**：epics.md 行 1101 钦定 "在 handler 层就拦截"，所以本 case 必须挂 handler 层验证（`auth_handler` 包），不在 service 层。

**新增独立测试文件 `server/internal/app/http/handler/auth_handler_integration_test.go`**（build tag `integration`）：

```go
//go:build integration
// +build integration

package handler_test

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"

    "github.com/gin-gonic/gin"

    "github.com/huing/cat/server/internal/app/http/handler"
    "github.com/huing/cat/server/internal/app/http/middleware"
    "github.com/huing/cat/server/internal/pkg/response"
)

// AC8 边界 2: guestUid 长度 129 → handler 层 utf8.RuneCountInString 校验拦截 → 1002
//
// 必须挂 handler 层（不在 service 层 —— epics.md 行 1101 钦定 "在 handler 层就拦截"）。
// 本 case 用 stub AuthService 验证：handler.GuestLogin 在长度 129 时**根本不会调到** service.GuestLogin。
//
// 不用 dockertest（handler 层校验是纯 Go 内存逻辑，不需要 MySQL；但放到 integration tag 下，
// 是为对齐 epics.md §Story 4.7 行 1104 "全部场景在 +build integration tag" 的钦定位置）。
func TestAuthHandler_GuestLogin_GuestUIDExactly129Chars_Returns1002(t *testing.T) {
    gin.SetMode(gin.TestMode)
    r := gin.New()
    r.Use(middleware.ErrorMappingMiddleware())

    // stub AuthService：必须**没被调到** —— 否则 handler 长度校验失效
    var serviceCalled bool
    stubSvc := &stubAuthService{
        guestLoginFn: func(ctx context.Context, in service.GuestLoginInput) (*service.GuestLoginOutput, error) {
            serviceCalled = true
            return &service.GuestLoginOutput{}, nil
        },
    }

    h := handler.NewAuthHandler(stubSvc)
    r.POST("/auth/guest-login", h.GuestLogin)

    // 构造 129 字符 guestUid（utf8.RuneCountInString = 129）
    guestUID129 := strings.Repeat("a", 129)
    bodyMap := map[string]any{
        "guestUid": guestUID129,
        "device": map[string]any{
            "platform":    "ios",
            "appVersion":  "1.0.0",
            "deviceModel": "iPhone15,2",
        },
    }
    bodyBytes, _ := json.Marshal(bodyMap)
    req := httptest.NewRequest("POST", "/auth/guest-login", bytes.NewReader(bodyBytes))
    req.Header.Set("Content-Type", "application/json")
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    // 断言 HTTP 200（业务码与 HTTP status 正交，V1 §2.4 钦定）
    if w.Code != http.StatusOK {
        t.Errorf("HTTP status = %d, want 200", w.Code)
    }

    // 解析 envelope
    var resp response.Envelope
    if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
        t.Fatalf("unmarshal envelope: %v", err)
    }

    // 断言 1：envelope.code == 1002（V1 §3 钦定参数错误）
    if resp.Code != int(apperror.ErrInvalidParam) {
        t.Errorf("envelope.code = %d, want 1002", resp.Code)
    }

    // 断言 2：envelope.message 含 "guestUid 长度" 关键字（auth_handler.go:102 钦定文案）
    if !strings.Contains(resp.Message, "guestUid 长度") {
        t.Errorf("envelope.message = %q, want containing \"guestUid 长度\"", resp.Message)
    }

    // 断言 3：service 层未被调到（handler 校验提前拦截）
    if serviceCalled {
        t.Error("service.GuestLogin 不应被调用 —— handler 校验失效")
    }
}
```

**stub AuthService**（与 4.6 handler 单测同模式，但本文件需要独立 stub 因为 build tag 不同包内 stubAuthService 不能复用）：

```go
type stubAuthService struct {
    guestLoginFn func(ctx context.Context, in service.GuestLoginInput) (*service.GuestLoginOutput, error)
}

func (s *stubAuthService) GuestLogin(ctx context.Context, in service.GuestLoginInput) (*service.GuestLoginOutput, error) {
    return s.guestLoginFn(ctx, in)
}
```

**关键设计约束**：

- **必须挂 build tag `integration`**：与 epics.md 行 1104 钦定 "全部场景在 // +build integration tag" 一致；尽管本 case 不真起 MySQL 容器（纯内存 handler 逻辑），仍属 Layer 2 集成测试范畴（覆盖端到端 handler → service interface 边界）
- **测试位置**：`server/internal/app/http/handler/auth_handler_integration_test.go`（**独立文件** —— 不放 `auth_handler_test.go` 是为 build tag 隔离 + 与现有 4.6 handler 单测分层清楚）
- **stubAuthService**：独立于 4.6 `auth_handler_test.go` 中的 mockAuthService（不同包 + 不同 build tag —— **不**share 代码避免编译期歧义；同样的 stub 在两文件各自独立定义，与 4.6 / 4.8 整体 mock 模式一致）
- **断言 service 未被调到**（serviceCalled bool 标志）：这是 handler 层拦截语义的**核心**断言；如果未来某次重构错把长度校验挪到 service → 本 case 会立即捕获
- **断言 envelope.message 含 "guestUid 长度"**：防止"代码改 message 文案没更新测试"漂移；同时验证 ADR-0006 ErrorMappingMiddleware 把 apperror.New(ErrInvalidParam, "...") 的 message 透传到 envelope

**关键反模式**：

- ❌ **不**起 dockertest 容器（handler 层校验不需要 MySQL）—— 但仍放 integration tag 是为对齐 epics.md 钦定位置
- ❌ **不**断言 `len(req.Body)` / 网络层细节 —— 测的是 handler 业务逻辑
- ❌ **不**用 `assert.Contains(t, resp.Message, "...")` 类 testify 断言（项目沿用 stdlib testing；与 4.5 / 4.6 / 4.8 同模式）
- ❌ **不**重复测 happy case（128 字符通过）—— 那是 AC7 service 层 case；本 case 专测拒绝路径

**AC9 — 重入: 同一 guestUid 已成功登录 → 第二次走复用，DB 行数不变**

新增测试函数 `TestAuthService_GuestLogin_ReentryAfterSuccess_ReusesWithoutNewRows`：

```go
func TestAuthService_GuestLogin_ReentryAfterSuccess_ReusesWithoutNewRows(t *testing.T) {
    svc, sqlDB, cleanup := buildAuthService(t)
    defer cleanup()

    const guestUID = "uid-reentry"
    input := service.GuestLoginInput{
        GuestUID:    guestUID,
        Platform:    "ios",
        AppVersion:  "1.0.0",
        DeviceModel: "iPhone15,2",
    }

    // 第一次调用：走 firstTimeLogin → 5 行入库
    out1, err := svc.GuestLogin(context.Background(), input)
    if err != nil {
        t.Fatalf("first call: %v", err)
    }
    if out1.Token == "" {
        t.Errorf("first token empty")
    }

    // 第一次调用后的初始 DB 状态快照
    initialUserCount := queryCount(t, sqlDB, "users")
    initialBindingCount := queryCount(t, sqlDB, "user_auth_bindings")
    initialPetCount := queryCount(t, sqlDB, "pets")
    initialStepCount := queryCount(t, sqlDB, "user_step_accounts")
    initialChestCount := queryCount(t, sqlDB, "user_chests")
    if initialUserCount != 1 {
        t.Fatalf("post-first-call users count = %d, want 1", initialUserCount)
    }

    // 第二次调用（同 guestUid）：必须走 reuseLogin → DB 行数不变
    out2, err := svc.GuestLogin(context.Background(), input)
    if err != nil {
        t.Fatalf("second call (reentry): %v", err)
    }

    // 断言 1：返回 user_id 一致
    if out1.UserID != out2.UserID {
        t.Errorf("reentry UserID changed: %d → %d (违反 reuseLogin 语义)", out1.UserID, out2.UserID)
    }

    // 断言 2：返回 pet_id 一致（reuseLogin 加载同一只默认猫）
    if out1.PetID != out2.PetID {
        t.Errorf("reentry PetID changed: %d → %d", out1.PetID, out2.PetID)
    }

    // 断言 3：返回 nickname 一致（reuseLogin 不重写 nickname）
    if out1.Nickname != out2.Nickname {
        t.Errorf("reentry Nickname changed: %q → %q", out1.Nickname, out2.Nickname)
    }

    // 断言 4：token 是新签发的（reuseLogin 每次签新 token —— V1 §4.1 钦定）
    if out1.Token == out2.Token {
        t.Errorf("reentry Token == first Token (reuseLogin 应每次签新 token)")
    }
    if out2.Token == "" {
        t.Errorf("second token empty")
    }

    // 断言 5：DB 5 张表行数完全不变（reuseLogin 不写入任何表）
    if c := queryCount(t, sqlDB, "users"); c != initialUserCount {
        t.Errorf("post-reentry users count = %d, want %d (no new row)", c, initialUserCount)
    }
    if c := queryCount(t, sqlDB, "user_auth_bindings"); c != initialBindingCount {
        t.Errorf("post-reentry bindings count = %d, want %d", c, initialBindingCount)
    }
    if c := queryCount(t, sqlDB, "pets"); c != initialPetCount {
        t.Errorf("post-reentry pets count = %d, want %d", c, initialPetCount)
    }
    if c := queryCount(t, sqlDB, "user_step_accounts"); c != initialStepCount {
        t.Errorf("post-reentry step count = %d, want %d", c, initialStepCount)
    }
    if c := queryCount(t, sqlDB, "user_chests"); c != initialChestCount {
        t.Errorf("post-reentry chest count = %d, want %d", c, initialChestCount)
    }
}

// queryCount helper：返 SELECT COUNT(*) 结果（不带 WHERE 子句的简写）。
// 与 assertCount 不同：assertCount 同时断言 + 失败 t.Errorf；queryCount 只查不断言。
func queryCount(t *testing.T, sqlDB *sql.DB, table string) int64 {
    t.Helper()
    var n int64
    if err := sqlDB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n); err != nil {
        t.Fatalf("queryCount %s: %v", table, err)
    }
    return n
}
```

**与 4.6 `SameGuestUID` 测试的差异**：

| 维度 | 4.6 `SameGuestUID` (已 done) | 本 story 4.7 `Reentry` |
|---|---|---|
| user_id 相等断言 | ✓ | ✓ |
| pet_id 相等断言 | ✓ | ✓ |
| token 不空断言 | ✓ | ✓ |
| **token 必须不同断言** | ✗ | ✓（reuseLogin 每次签新） |
| **nickname 一致断言** | ✗ | ✓（reuseLogin 不改 nickname） |
| **DB 5 表行数完全不变断言** | ✓（用 1 = 1 断言）| ✓（用前后快照对比） |
| **快照对比模式**（前后差为 0）| ✗ | ✓（防 future regression 写入隐式行）|

**关键设计约束**：

- **token 不同断言**：V1 §4.1 行 168 钦定 "重入登录 → 新 token"（每次返新签发；防 client 缓存旧 token 永久未失效）；reuseLogin 调 `signer.Sign(user.ID, 0)` 必返新 token（HS256 + iat / exp 时间戳变化）—— 本断言验证该语义在 reuseLogin 路径上未漂移
- **快照对比模式**（initialUserCount → 第二次调用 → 验证未变）：比"硬断言 count=1"更鲁棒 —— 假设 future epic 在 firstTimeLogin 阶段写入了某个新表（如 audit log），第一次调用 count 会变成 X；如果 reuseLogin 也不小心写入新表，count 会变 X+1；用快照对比能立即捕获这种漂移
- **不**断言 reuseLogin **路径的具体调用**（如 stub repo 检查没调 binding.Create）：service 层是黑盒（dockertest Layer 2 集成测试用真 repo），调用图由 service 单测覆盖（4.6 已 done）；本 case 只验证**结果**：DB 不变 + token 新

**关键反模式**：

- ❌ **不**与 4.6 `SameGuestUID` 测试合并（4.6 case 是浅层 user_id 一致；本 case 是深层 reuseLogin 语义验证）—— 保持两份独立测试，便于 diff 时清楚知道哪个测哪个
- ❌ **不**用 time.Sleep 让 token 时间戳错开（HS256 每次 Sign 都变，因为 iat / exp 取 time.Now().Unix()；不需要 sleep）
- ❌ **不**用 `assert.NotEqual(t, out1.Token, out2.Token)`（项目沿用 stdlib testing —— 用 `if out1.Token == out2.Token { t.Error... }`）

**AC10 — bootstrap / handler 单测兼容性**

本 story 不修改 bootstrap router / 4.6 已有的 service / handler / repo 实装；但因为 AC8 在 handler 包新增 `auth_handler_integration_test.go`，需要：

- `auth_handler_test.go`（4.6 已 done）保持现状，**不**改
- 新加的 `auth_handler_integration_test.go` 用 `package handler_test`（与 4.6 同包名）—— 但 build tag 隔离，默认 test 不跑
- `stubAuthService` 在两文件分别定义（4.6 用 `mockAuthService`，本 story 用 `stubAuthService`）：**严格不**合并到一份共享测试 helper —— 4.6 build tag 默认 + 本 story build tag integration，go test 编译期会按 tag 分别编译，两个 stub 名称不能在同一编译单元内共存（**关键**：`mockAuthService` 与 `stubAuthService` 是同 package 不同文件，在 integration tag 编译时两个文件都参与编译 → 必须用不同名称，**不**能复用 mockAuthService 名）

**全量验证**：

- `bash /c/fork/cat/scripts/build.sh` → BUILD SUCCESS
- `bash /c/fork/cat/scripts/build.sh --test` → all tests passed（4.6 / 4.8 现有 case 不受影响 —— 本 story 新加 case 都在 integration tag，默认不跑）
- `bash /c/fork/cat/scripts/build.sh --integration` → 4.6 现有 4 case + 本 story 新加 8 case + 4.8 现有 3 case **共 15 case** 全绿（每 case 独立起 dockertest 容器）
- `go vet -tags=integration ./...` → 全绿（验证本 story integration tag 下编译无错）
- `go.mod` / `go.sum` 无 diff（仅消费已有依赖）

**关键约束**：

- **AC8 跨包测试位置**：`server/internal/app/http/handler/auth_handler_integration_test.go` 是本 story 新增的**唯一非 service 包测试文件**；其他 7 个 case 全在 `server/internal/service/auth_service_integration_test.go`（同 4.6 文件追加）
- **不**新增任何 source 文件（仅测试文件）—— 本 story 是 Layer 2 集成测试 epic，禁止修改业务代码

**AC11 — README / docs / lessons 不更新**

本 story **不**更新：

- `README.md` / `server/README.md`：留 Epic 4 收尾或 Story 6.3 文档同步阶段
- `docs/宠物互动App_*.md` 任一份：本 story 严格对齐契约**输入**，不修改
- `docs/lessons/`：review 阶段写新 lesson 由 fix-review 处理（epic-loop 流水线分工）
- `_bmad-output/implementation-artifacts/decisions/` 任一份：本 story 不引入新决策

**关键约束**：

- 如果 dev 阶段实装时发现某条 AC 与文档冲突 / 漏洞，**不**自行修文档，**而是**在 Completion Notes 里登记 issue + 让 fix-review 处理
- **tech debt log 留 Story 6.3 登记**（如果本 story dev 阶段确实需要登记新债，在 Completion Notes 钉死）：当前预测无新债（本 story 是收尾性测试，不引入新功能 / 新依赖）

## Tasks / Subtasks

- [ ] **Task 1（AC1）**：复用 4.6 helper + 在 `auth_service_integration_test.go` 顶部追加 fault repo helper struct
  - [ ] 1.1 文件保持 `//go:build integration` + `// +build integration` 双行 tag（4.6 已写）
  - [ ] 1.2 文件保持 `package service_test`（4.6 已写）
  - [ ] 1.3 复用 `startMySQL` / `runMigrations` / `buildAuthService` / `assertCount` helper（4.6 已落地）
  - [ ] 1.4 新加 `queryCount` helper（用于 AC9 重入 case 快照对比）
  - [ ] 1.5 新加 `faultUserRepo` / `faultPetRepo` / `faultChestRepo` 三个 fault injection wrapper（每个包装真实 repo，仅在指定方法注入 injectErr）
- [ ] **Task 2（AC2）**：实装 `TestAuthService_GuestLogin_PetRepoFailsTx_AllRowsRollback`
  - [ ] 2.1 起 dockertest mysql + migrate
  - [ ] 2.2 用真实 mysql.NewUserRepo / NewAuthBindingRepo / NewStepAccountRepo / NewChestRepo + faultPetRepo（注入 errors.New("synthetic pet repo failure")）
  - [ ] 2.3 调 svc.GuestLogin
  - [ ] 2.4 断言 errors.As(err, &apperror.AppError{}) + AppError.Code == ErrServiceBusy(1009)
  - [ ] 2.5 断言 5 张表 count=0（assertCount 全 0）
- [ ] **Task 3（AC3）**：实装 `TestAuthService_GuestLogin_ChestRepoFailsTx_AllRowsRollback`
  - [ ] 3.1 同 Task 2 setup
  - [ ] 3.2 用 faultChestRepo（注入 synthetic chest repo failure）
  - [ ] 3.3 断言 1009 + 5 张表 count=0（前 4 步 INSERT 都已发生但全部 rollback）
- [ ] **Task 4（AC4）**：实装 `TestAuthService_GuestLogin_UserRepoFailsTx_NoRowsCreated`
  - [ ] 4.1 同 Task 2 setup
  - [ ] 4.2 用 faultUserRepo（注入 synthetic user repo failure）
  - [ ] 4.3 断言 1009 + 5 张表 count=0（第 1 步即失败，后续未到达）
- [ ] **Task 5（AC5）**：实装 `TestAuthService_GuestLogin_Concurrent100SameGuestUID_OnlyOneUser`
  - [ ] 5.1 buildAuthService（用真实 5 个 repo，无 fault injection）
  - [ ] 5.2 100 个 goroutine + sync.WaitGroup 并发调用同一 guestUID
  - [ ] 5.3 断言所有 100 都成功（任一失败 → t.Fatalf）
  - [ ] 5.4 断言所有 100 拿到同一 user_id
  - [ ] 5.5 断言 DB 5 张表各恰好 1 行
- [ ] **Task 6（AC6）**：实装 `TestAuthService_GuestLogin_Concurrent100DifferentGuestUIDs_NoCrossData`
  - [ ] 6.1 buildAuthService
  - [ ] 6.2 100 个 goroutine 各用 fmt.Sprintf("uid-diff-%03d", i) 构造不同 guestUID
  - [ ] 6.3 用 seenUIDs / seenPetIDs map 验证 user_id / pet_id 互不相同
  - [ ] 6.4 断言 DB 5 张表各 100 行
  - [ ] 6.5 对每个 result.userID 单独断言 5 张关联表都恰好 1 行 + user_id 字段匹配（"无串数据"语义验证）
- [ ] **Task 7（AC7）**：实装 `TestAuthService_GuestLogin_GuestUIDExactly128Chars_Succeeds`
  - [ ] 7.1 buildAuthService
  - [ ] 7.2 strings.Repeat("a", 128) 构造 128 字符 guestUID + utf8.RuneCountInString 显式校验
  - [ ] 7.3 svc.GuestLogin 应成功
  - [ ] 7.4 断言 5 张表各 1 行
  - [ ] 7.5 SELECT guest_uid FROM users 验证完整存储（防 VARCHAR 截断）
- [ ] **Task 8（AC8）**：实装 `TestAuthHandler_GuestLogin_GuestUIDExactly129Chars_Returns1002`
  - [ ] 8.1 新建 `server/internal/app/http/handler/auth_handler_integration_test.go`（build tag `integration`）
  - [ ] 8.2 文件顶部双行 tag + `package handler_test`
  - [ ] 8.3 在文件内独立定义 `stubAuthService`（不与 4.6 mockAuthService 共享）
  - [ ] 8.4 gin.SetMode(TestMode) + 挂 ErrorMappingMiddleware + handler.NewAuthHandler(stubSvc)
  - [ ] 8.5 用 strings.Repeat("a", 129) 构造 129 字符 guestUid → POST /auth/guest-login
  - [ ] 8.6 断言 HTTP 200 + envelope.code == 1002 + envelope.message 含 "guestUid 长度"
  - [ ] 8.7 断言 stubSvc.GuestLogin **未被调到**（handler 拦截语义验证）
- [ ] **Task 9（AC9）**：实装 `TestAuthService_GuestLogin_ReentryAfterSuccess_ReusesWithoutNewRows`
  - [ ] 9.1 buildAuthService
  - [ ] 9.2 第一次调用 → 5 行入库 + 取 token
  - [ ] 9.3 用 queryCount 快照前 5 张表行数
  - [ ] 9.4 第二次调用同 guestUID → 走 reuseLogin
  - [ ] 9.5 断言：UserID / PetID / Nickname 一致；Token **不同**；5 张表行数完全不变
- [ ] **Task 10（AC10）**：全量验证 + 回归 4.6 / 4.8 既有测试不破坏
  - [ ] 10.1 `bash /c/fork/cat/scripts/build.sh` → BUILD SUCCESS
  - [ ] 10.2 `bash /c/fork/cat/scripts/build.sh --test` → 默认所有测试通过（本 story 新加 8 case 都在 integration tag 不跑；4.6 / 4.8 现有 case 不受影响）
  - [ ] 10.3 `bash /c/fork/cat/scripts/build.sh --integration` → 全部 integration case 通过（包括 4.6 既有 4 case + 4.8 既有 3 case + 本 story 新增 8 case = 15 case）
  - [ ] 10.4 `go vet -tags=integration ./...` → 编译期检查通过
  - [ ] 10.5 `go mod tidy` → go.mod / go.sum 无 diff（仅消费已有依赖：dockertest / GORM / gin / apperror）
- [ ] **Task 11（AC11）**：本 story 不做 git commit
  - [ ] 11.1 epic-loop 流水线约束遵守：dev-story 阶段不 commit
  - [ ] 11.2 commit message 模板留给 story-done 阶段：

    ```text
    test(auth): Layer 2 集成测试 - 游客登录初始化事务全流程（Story 4.7）

    - internal/service/auth_service_integration_test.go: 追加 7 case（3 回滚 + 2 并发 + 1 边界 + 1 重入）
    - internal/app/http/handler/auth_handler_integration_test.go: 新增 1 case（边界 2 handler 层拦截）
    - 共 8 case 严格对齐 epics.md §Story 4.7 行 1090-1104 钦定场景
    - 全部 +build integration tag，dockertest 真实 MySQL（不用 sqlmock）
    - 4.6 fault injection 模式：faultUserRepo / faultPetRepo / faultChestRepo 包装真实 repo

    依据 epics.md §Story 4.7 + V1 §4.1 + 数据库设计 §8.1 + ADR-0001 测试栈。

    Story: 4-7-layer-2-集成测试-游客登录初始化事务全流程
    ```

## Dev Notes

### 关键设计原则

1. **Layer 2 集成测试 = 黑盒事务行为验证**：不验证 SQL 字符串（那是 sqlmock 的职责，归 repo 单测）；验证整个 service → repo → MySQL → InnoDB → DB 状态最终一致性的端到端行为。fault injection 用包装真实 repo 而非 stub，确保 InnoDB 事务真实回滚行为被覆盖（与 service 单测 stubTxMgr 不真开事务的 mock 模式形成互补）。
2. **dockertest 必须**（epics.md 行 1103 钦定）：禁用 sqlmock 是 epics.md 钦定的 Layer 2 集成测试范式 —— Layer 1 service 单测可用 stub repo / sqlmock，Layer 2 必须真 MySQL。
3. **fault injection 包装真实 repo**：`faultUserRepo` / `faultPetRepo` / `faultChestRepo` 持有 `delegate mysql.UserRepo`（真实 repo） + `injectErr error`（注入错误），让 fault 方法直接返 injectErr，其他方法透传给 delegate。这是 Layer 2 fault injection 的标准范式（与 stub repo 的"全部方法都不真调"截然不同）。
4. **N=100 是钦定下限**（epics.md 行 1098-1099）：并发 case 必须 100 goroutine —— 不能减少（覆盖度变弱），也不需要增加（已经压榨 InnoDB lock）。每 case 独立起容器（与 4.6 / 4.8 同模式），可承受 100 goroutine + 30s 容器启动 + ~5s 测试运行。
5. **边界 2 handler 层拦截语义**（epics.md 行 1101 钦定）：guestUid 129 字符在 handler.GuestLogin 的 utf8.RuneCountInString 校验阶段被拒绝 → 1002 envelope；service.GuestLogin **不应被调到**。本 story 通过断言 `serviceCalled bool == false` 强语义验证。
6. **重入 case 验证 reuseLogin 深层语义**：4.6 同 guestUid 测试是浅层 user_id 一致；本 story 重入 case 增加 token 不同 + nickname 一致 + DB 5 表行数快照前后无变化 —— 三重断言验证 reuseLogin 路径完整性。
7. **不修改业务代码**（不改 4.6 / 4.8 source 文件）：本 story 仅新增测试文件 + fault helper struct。如发现业务代码漏洞 → 在 Completion Notes 登记，让 fix-review 处理（不自行修）。

### 架构对齐

**Layer 2 集成测试范式**（`_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.5）：

- **Layer 1 单元测试**（4.6 已落地）：stub repo + stub txMgr 不真开事务；mock 5 repo 调用顺序 + 错误处理；handler 单测用 mock service。范围：service 业务逻辑分支 + handler 校验逻辑 + repo SQL 形态。
- **Layer 2 集成测试**（**本 story 收尾**）：dockertest 真 MySQL + 真 repo + 真 service + 部分 fault injection；验证事务回滚 / 并发 / 边界 / 重入的端到端行为。范围：业务流程黑盒结果 + InnoDB 真实事务行为。
- **Layer 3 跨端 E2E**（节点 3 Story 9.1 起）：iOS 客户端 + server 真接入；范围：跨端契约 + 协议层错误处理。

**事务边界对齐**（`docs/宠物互动App_数据库设计.md` §8.1）：5 表初始化事务必须**同一事务**；本 story 验证：

- 任一步失败 → 全部回滚（事务原子性，§AR1）
- 100 goroutine 同 guestUid → 1 user（自然幂等，§AR3 + UNIQUE INDEX）
- 不同 guestUid 100 goroutine → 100 独立事务，无串数据（事务隔离性）

**接口契约对齐**（`docs/宠物互动App_V1接口设计.md` §4.1）：

- 错误码 1002（参数错误，handler 边界拦截）/ 1009（服务繁忙，事务回滚）
- 同 guestUid 自然幂等（§AR3 + 4.6 已落地 ER_DUP_ENTRY 兜底）
- 重入 → 新 token（reuseLogin 每次签发新 HS256 token）

**ADR 对接**：

- **ADR-0001 测试栈**（`_bmad-output/implementation-artifacts/decisions/0001-test-stack.md`）：
  - §3.1 测试分层：Layer 1 单测 / Layer 2 集成 / Layer 3 跨端 E2E；本 story 是 Layer 2 收尾
  - §3.5 Windows race skip：本 story 100 goroutine 不挂 race detector（与 4.6 / 4.8 同模式）
- **ADR-0006 错误三层映射**：repo → service → handler，本 story 验证 fault injection 经过三层后正确产出 1009 envelope
- **ADR-0007 ctx 传播**：本 story 使用 context.Background() —— 不测 ctx cancel（不在 epics.md 钦定范围）；ctx 验证由 4.2 / 4.6 单元测试覆盖

### 测试策略

**测试数量分布**：

- AC2 / AC3 / AC4（3 回滚 case）：3 个 fault injection wrapper + 3 个独立 dockertest case
- AC5 / AC6（2 并发 case）：2 个 100 goroutine 高并发 case
- AC7（边界 1 happy）：1 个 service 层 128 字符 case
- AC8（边界 2 拒绝）：1 个 handler 层 129 字符 case（独立 handler 测试文件）
- AC9（重入）：1 个深层 reuseLogin 语义 case
- **共 8 case**（与 epics.md 行 1090-1102 钦定 8 类完全对齐）

**测试位置分布**：

- `server/internal/service/auth_service_integration_test.go`（4.6 文件追加）：7 case（AC2-AC7 + AC9，service 层）
- `server/internal/app/http/handler/auth_handler_integration_test.go`（**新建**）：1 case（AC8，handler 层）

**与 4.6 已有 4 case 的关系**：

- 4.6 已有 4 case 不动（done 状态保持）
- 本 story 在同一文件**追加** 7 case；4.6 case 在前（happy / 复用 / race / 不同），本 story case 在后（回滚 / 并发 100 / 边界 / 重入）
- `auth_service_integration_test.go` 文件最终 11 case（4.6 的 4 + 本 story 的 7）+ 多个 fault helper struct
- 4.6 中已存在的 `TestAuthService_GuestLogin_ConcurrentSameGuestUID_BothSucceedSameUserID`（2 goroutine race） **不替代**；本 story 加的 `TestAuthService_GuestLogin_Concurrent100SameGuestUID_OnlyOneUser`（100 goroutine）是**强化**：4.6 case 是 review-r1 lesson 的最小回归保护；本 story case 是 epics.md 钦定的高负载验证；两者共存 —— 不同失败模式覆盖（小规模 race 边界 vs 高规模收敛）

### 关键决策点（实装时注意）

1. **fault injection wrapper struct 必须实装完整 interface**：

   ```go
   type faultUserRepo struct {
       delegate  mysql.UserRepo
       injectErr error
   }

   func (f *faultUserRepo) Create(ctx context.Context, u *mysql.User) error { return f.injectErr }
   // 必须实装 UpdateNickname / FindByID（即使本 case 不调用）—— 否则编译期 interface 不匹配
   func (f *faultUserRepo) UpdateNickname(ctx context.Context, userID uint64, nickname string) error {
       return f.delegate.UpdateNickname(ctx, userID, nickname)
   }
   func (f *faultUserRepo) FindByID(ctx context.Context, id uint64) (*mysql.User, error) {
       return f.delegate.FindByID(ctx, id)
   }
   ```

   pattern: fault 注入的方法直接 return injectErr；非注入方法透传给 delegate。

2. **AC2 / AC3 / AC4 的 setup 高度重复**：每个 fault case 都需要 startMySQL + migrate + db.Open + signer + 4 个真实 repo + 1 个 fault repo + svc 装配；可考虑抽 helper：

   ```go
   func buildAuthServiceWithUserFault(t *testing.T, injectErr error) (svc service.AuthService, sqlDB *sql.DB, cleanup func()) { ... }
   ```

   **决策**：**不抽**。三个 fault case 各自独立写完整 setup（与 4.6 / 4.8 同模式 —— 测试代码读起来像剧本，不需要跳函数追真相）。

3. **`apperror.AppError` 类型断言**：

   ```go
   import apperror "github.com/huing/cat/server/internal/pkg/errors"

   var appErr *apperror.AppError
   if !errors.As(err, &appErr) {
       t.Fatalf("expected *AppError, got %T: %v", err, err)
   }
   if appErr.Code != apperror.ErrServiceBusy {
       t.Errorf("AppError.Code = %d, want %d", appErr.Code, apperror.ErrServiceBusy)
   }
   ```

   **关键**：`apperror.ErrServiceBusy` 是 const int（值 1009）；`AppError.Code` 同类型 → 直接比较；不要用 `appErr.Code == 1009` 硬编码（用包级常量更可读）。

4. **errors.New / errors.As 必须 import "errors"**：

   ```go
   import (
       stderrors "errors" // 与 4.6 同名（pkg shadow apperror "errors" alias）
   )

   if !stderrors.As(err, &appErr) { ... }
   ```

   注意：4.6 的 `auth_service_integration_test.go` 已 import `errors` 而**没**用 stderrors alias；本 story 加 fault helper 的 `errors.New(...)` 直接用 errors —— 与 4.6 既有 import 一致即可（不用 alias 也能编译，因为 apperror 是不同包名）。

5. **`fmt.Sprintf("uid-diff-%03d", i)` 而非 `fmt.Sprintf("uid-diff-%d", i)`**：

   - `%d` 在 i=10 时得 "uid-diff-10"，i=100 时得 "uid-diff-100"，长度 11 / 12 跳变
   - `%03d` 永远 11 字符（"uid-diff-099" / "uid-diff-100" 长度一致）—— debug 时按字符串排序看更直观
   - 长度 11 远 < 128 边界（不会撞 AC7 边界 case 的字段长度上限）

6. **GORM AUTO_INCREMENT 行为差异**：100 goroutine 并发 INSERT users 时，AUTO_INCREMENT 序号可能跳号（InnoDB innodb_autoinc_lock_mode=2 默认下 batch 分配）；本 story **不**断言 user_id 连续，只断言互不相同 + 数量正确。

7. **dockertest 容器启动时间**：~10-30s 冷启 × 9 case（本 story 8 + 4.6 现有 4 = 12，但 4.6 已有 4 单独 case 加上本 story 8 = 12 总）= ~120-360s 集成测试总时长；本地夜间跑可接受，CI 需配 docker layer cache。**关键**：`bash scripts/build.sh --integration` 已设 120s timeout per test，单 case 必须完成在 120s 内（包括 30s 启动 + 测试逻辑）。100 goroutine 测试 ~5-10s 业务时间足够。

8. **MySQL 8.0 InnoDB 隔离级别**（dockertest 默认 REPEATABLE READ）：本 story 100 goroutine 同 guestUid 并发场景下，insert intent gap lock 行为可能导致 B 等待 A commit 才继续；**不**专项验证隔离级别；只断言收敛结果（同 user_id + 1 行）。Future 隔离级别专项 epic 才单独覆盖（不在本 story 范围）。

9. **handler 集成测试 stub 命名**（`stubAuthService` vs 4.6 `mockAuthService`）：

   - 4.6 `auth_handler_test.go` 用 `mockAuthService`（默认编译，无 build tag）
   - 本 story `auth_handler_integration_test.go` 用 `stubAuthService`（build tag integration）
   - **同 package 不同 build tag 不能同名**：因为 `go test -tags integration ./internal/app/http/handler/...` 时两文件都编译 → mockAuthService 与 stubAuthService 共存 → 不能同名
   - **决策**：本 story 用 `stubAuthService`（命名上区分 stub 与 mock）

10. **断言 service 未被调到**（AC8 边界 2）：

    ```go
    var serviceCalled bool
    stubSvc := &stubAuthService{
        guestLoginFn: func(ctx context.Context, in service.GuestLoginInput) (*service.GuestLoginOutput, error) {
            serviceCalled = true
            return &service.GuestLoginOutput{}, nil
        },
    }
    // ... 调 handler ...
    if serviceCalled {
        t.Error("service.GuestLogin 不应被调用 —— handler 校验失效")
    }
    ```

    **关键**：用闭包捕获 bool 而非 channel —— bool 在 handler 处理完毕后读取（同 goroutine，无 race）；channel 适合异步场景，本 case 同步无需。

11. **AC8 测试不需要 dockertest 容器**（仅 handler 内存逻辑）：

    - epics.md 行 1104 钦定 "全部场景在 // +build integration tag" → 本 case 也挂 build tag
    - 但 handler 校验是纯 Go 内存逻辑（utf8.RuneCountInString）—— 不需要 MySQL 容器
    - 决策：**挂 build tag 但不起 dockertest**（与 ADR-0001 §3.5 兼容 —— integration tag 是触发集成范畴的开关，不是必须起容器的硬约束）

12. **测试文件 import 要点**：

    `auth_service_integration_test.go`（service 包追加）需要的 imports：

    ```go
    import (
        "context"
        "database/sql"
        "errors"          // for errors.New / errors.As（fault injection）
        "fmt"             // for fmt.Sprintf（uid-diff-%03d）
        "path/filepath"
        "strconv"
        "strings"         // for strings.Repeat（128 / 129 字符）
        "sync"
        "testing"
        "time"
        "unicode/utf8"    // for utf8.RuneCountInString（边界 1 校验）

        "github.com/ory/dockertest/v3"
        "github.com/ory/dockertest/v3/docker"

        "github.com/huing/cat/server/internal/infra/config"
        "github.com/huing/cat/server/internal/infra/db"
        "github.com/huing/cat/server/internal/infra/migrate"
        "github.com/huing/cat/server/internal/pkg/auth"
        apperror "github.com/huing/cat/server/internal/pkg/errors" // for ErrServiceBusy
        "github.com/huing/cat/server/internal/repo/mysql"
        "github.com/huing/cat/server/internal/repo/tx"
        "github.com/huing/cat/server/internal/service"
    )
    ```

    `auth_handler_integration_test.go`（handler 包新建）需要的 imports：

    ```go
    import (
        "bytes"
        "context"
        "encoding/json"
        "net/http"
        "net/http/httptest"
        "strings"
        "testing"

        "github.com/gin-gonic/gin"

        "github.com/huing/cat/server/internal/app/http/handler"
        "github.com/huing/cat/server/internal/app/http/middleware"
        apperror "github.com/huing/cat/server/internal/pkg/errors"
        "github.com/huing/cat/server/internal/pkg/response"
        "github.com/huing/cat/server/internal/service"
    )
    ```

13. **测试运行性能预算**：

    - 8 case × 30s 平均（含容器启动 + 业务时间）= ~240s 增量
    - 加上 4.6 / 4.8 现有 7 case × 30s = ~210s
    - `bash scripts/build.sh --integration` 总时长 ~7-10 min（CI 可接受；本地按需运行）
    - 单 case 必须 < 120s（脚本 -timeout=120s 钦定）—— 100 goroutine + 30s 容器启动 + 业务时间预计 ~60s，安全

14. **assertCount 接受 args ...any vs []any**：4.6 已有 helper signature 是 `func assertCount(t *testing.T, sqlDB *sql.DB, query string, args []any, want int64, label string)` —— 注意是 `args []any` slice，不是 variadic；本 story 复用同 signature，nil args 写 `[]any{}` 或直接 `nil`（4.6 已有 `assertCount(t, sqlDB, "users", nil, 2, "users (2 distinct)")` 用 nil）。

15. **Windows 平台限制**：与 4.2-4.8 一致 —— Windows ThreadSanitizer skip；本 story 100 goroutine 不上 race detector，但测试本身不依赖 Windows / Linux 差异；dockertest 在 Windows 需要 Docker Desktop。

### Project Structure Notes

预期文件 / 目录变化：

**新增**：

- `server/internal/app/http/handler/auth_handler_integration_test.go`（build tag `integration`，仅 AC8 边界 2 case + stubAuthService stub）

**修改**：

- `server/internal/service/auth_service_integration_test.go`（追加 7 case + queryCount helper + 3 个 fault\*Repo wrapper struct；保留 4.6 已有 4 case 不动）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（4-7: backlog → ready-for-dev → in-progress → review；由 dev-story 流程内推动）
- `_bmad-output/implementation-artifacts/4-7-layer-2-集成测试-游客登录初始化事务全流程.md`（本 story 文件，dev 完成后填 Tasks/Dev Agent Record/File List/Completion Notes）

**不影响其他目录**：

- `server/internal/repo/mysql/` 全部 不变（4.6 / 4.8 已落地；本 story 仅消费）
- `server/internal/service/auth_service.go` 不变（4.6 已落地）
- `server/internal/service/auth_service_test.go` 不变（4.6 已落地 7 case unit tests）
- `server/internal/service/home_service.go` 不变（4.8 已落地）
- `server/internal/service/home_service_test.go` 不变
- `server/internal/service/home_service_integration_test.go` 不变（4.8 已落地）
- `server/internal/app/http/handler/auth_handler.go` 不变（4.6 已落地）
- `server/internal/app/http/handler/auth_handler_test.go` 不变（4.6 已落地 5 case unit tests）
- `server/internal/app/http/handler/home_handler.go` 不变（4.8 已落地）
- `server/internal/app/http/handler/home_handler_test.go` 不变
- `server/internal/app/bootstrap/router.go` 不变（4.6 / 4.8 已 wire）
- `server/internal/pkg/auth/` 不变（4.4 已落地；本 story 仅 import 用）
- `server/internal/pkg/errors/` 不变（apperror 框架已落地；本 story 仅消费 ErrServiceBusy / ErrInvalidParam）
- `server/internal/pkg/response/` 不变（envelope helper；本 story 仅消费 Envelope struct 反序列化）
- `server/internal/infra/db/` 不变（4.2 已落地）
- `server/internal/infra/migrate/` 不变（4.3 已落地）
- `server/internal/repo/tx/` 不变（4.2 已落地）
- `server/migrations/` 不变（4.3 已落地）
- `server/configs/local.yaml` 不变（不引入新配置项）
- `server/cmd/server/main.go` 不变（不加新 deps）
- `iphone/` / `ios/` 不变（server-only story）
- `docs/宠物互动App_*.md` 全部 7 份不变（消费方）
- `docs/lessons/` 不变（review 阶段写新 lesson 由 fix-review 处理）
- `_bmad-output/implementation-artifacts/decisions/` 不变（不引入新 ADR）
- `README.md` / `server/README.md` 不变（Epic 4 收尾或 Epic 36 部署 story 才统一更新）

### dockertest helper 复用决策

**为什么不抽 testutil 包？**

- 4.6 / 4.8 已建立模式：每份 integration 测试文件**复制** startMySQL / runMigrations / migrationsPath helper 到本文件
- 抽 testutil 跨包要解决：
  - testutil 的 build tag 处理（应当 +build integration 还是 default？）
  - testutil 的 import 路径循环风险（service / handler / repo 各自的 integration test 都引用 testutil）
  - testutil 的 helper 演化（dockertest API 升级 / 配置项增减时单点改动 vs 各文件改动权衡）
- **决策**：**不抽 testutil**。本 story 的 7 service case 复用同文件 4.6 已有 helper；handler 1 case 不需要 dockertest，独立写。Future 当 integration test 数量 > 30 时再决策 testutil 抽离。

### V1 §4.1 边界字段配置回归检查

**为什么 AC7 必须 SELECT guest_uid 字段长度？**

如果未来某次 schema migration 不小心写错（例如 `migrations/0001_init_users.sql` 把 `guest_uid VARCHAR(128)` 改成 `VARCHAR(64)`）：

- service 不会报错（GORM Create 不校验目标列长度）
- MySQL 在 STRICT_TRANS_TABLES=ON 模式下会拒绝（报错 1265 Data truncated）
- MySQL 在 sql_mode 没有 STRICT 模式时会**静默截断**（128 字符 → 64 字符存）

dockertest 默认起的 mysql:8.0 容器有 STRICT_TRANS_TABLES=ON（防止 schema bug 静默通过），但 production 配置可能漂移。本 story AC7 的 SELECT guest_uid 字段断言是 schema 字段长度的最后一道防线 —— 让 schema 改动立即被 integration test 捕获。

### 集成测试性能 / 容器复用

**当前 8 case 独立起容器**（与 4.6 / 4.8 同模式）；**不**做 testcontainer reuse 优化。理由：

- 节点 2 阶段测试代码总量小，容器启动时间是可接受成本
- 容器隔离让每个 case 在干净 DB 上跑，断言代码简单（不用清表 / reset auto_increment）
- testcontainer reuse 引入 setup/teardown 顺序复杂度（如何确保 case 之间互不污染？多容器 lifecycle 管理？）

future Epic（特别是 11.9 / 20.9 / 26.5 / 32.5 这些 Layer 2 测试堆积后）可考虑抽 sharedContainer testutil；本 story 不预先抽。

### 与 4.6 review-r1 lesson 的对接

**`docs/lessons/2026-04-26-multi-table-tx-must-cover-all-unique-constraint-races.md`**（4.6 review-r1 lesson）已让 service 层穷举 race coverage matrix：

- `users.uk_guest_uid` race → `ErrUsersGuestUIDDuplicate` 兜底 reuseLogin
- `user_auth_bindings.uk_auth_type_identifier` race → `ErrAuthBindingDuplicate` 兜底 reuseLogin
- 两条都验证过通过 4.6 集成测试 `TestAuthService_GuestLogin_ConcurrentSameGuestUID_BothSucceedSameUserID`（2 goroutine race）

本 story AC5（100 goroutine 同 guestUid）**强化**该 lesson 在高负载下的回归保护：

- 2 goroutine 在 InnoDB 下大概率走路径 1（A commit 后 B INSERT users 1062）—— 路径 2（A 未 commit B INSERT binding 1062）极少触发
- 100 goroutine 分散调度让两条路径都被高强度撞击；任何一条 sentinel 退化（错把 ErrUsersGuestUIDDuplicate 当 generic error）会立即造成多 goroutine 拿到 1009
- **本 case 是 race coverage matrix 在生产场景下的负载测试**，不替代 4.6 的小规模回归保护

### References

- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 4.7 (行 1084-1104)] — **本 story 钦定 AC 来源**（8 类场景 + dockertest + +build integration tag）
- [Source: `_bmad-output/planning-artifacts/epics.md` §Epic 4 Overview (行 927-931)] — Epic 4 执行顺序 4.1 → 4.2 → 4.3 → 4.4 → 4.5 → 4.6 → 4.8 → **4.7**（4.7 物理上最后）
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 4.6 (行 1051-1082)] — 上游 story；本 story 直接消费 4.6 落地的 auth_service / 5 个 mysql repo / fault injection 验证目标
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 4.8 (行 1106-1137)] — 上游 story；本 story 不直接消费 home_service 但确认 4.8 done 后再做整体回归
- [Source: `_bmad-output/planning-artifacts/epics.md` §AR1 (行 152)] — 资产事务原子；本 story AC2-AC4 验证回滚行为
- [Source: `_bmad-output/planning-artifacts/epics.md` §AR3 (行 158)] — 节点 2 不接 Redis 幂等；本 story AC5（100 goroutine 同 guestUid）验证靠 DB UNIQUE 自然幂等
- [Source: `docs/宠物互动App_V1接口设计.md` §2.4 (行 47-63)] — envelope 结构 + 业务码与 HTTP status 正交（本 story AC8 边界 2 验证）
- [Source: `docs/宠物互动App_V1接口设计.md` §2.5 (行 65-72)] — 长度按字符数（本 story AC7 / AC8 边界 case 用 utf8.RuneCountInString 验证）
- [Source: `docs/宠物互动App_V1接口设计.md` §3 (行 76-118)] — 错误码 1002 / 1009 定义
- [Source: `docs/宠物互动App_V1接口设计.md` §4.1 (行 124-220)] — POST /auth/guest-login schema + 同 guestUid 自然幂等 + reuseLogin 每次签新 token
- [Source: `docs/宠物互动App_数据库设计.md` §3.1 (行 73-167)] — 主键 BIGINT UNSIGNED → uint64
- [Source: `docs/宠物互动App_数据库设计.md` §5.1-§5.6 (行 173-395)] — users / bindings / pets / step_accounts / chests 表 schema + UNIQUE 约束
- [Source: `docs/宠物互动App_数据库设计.md` §8.1 (行 880-895)] — 游客登录初始化事务（5 表同事务，本 story 验证回滚行为）
- [Source: `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 (行 122-201)] — 目录树锚定 internal/{app/http/handler,service,repo/mysql}
- [Source: `_bmad-output/implementation-artifacts/decisions/0001-test-stack.md` §3.1 / §3.5] — Layer 1 / Layer 2 / Layer 3 三层测试 + Windows race skip
- [Source: `_bmad-output/implementation-artifacts/decisions/0003-orm-stack.md`] — GORM v1.25.x 选型
- [Source: `_bmad-output/implementation-artifacts/decisions/0006-error-handling.md` §2 / §3] — 三层错误映射；本 story 验证 fault injection 经过三层产出 1009 envelope
- [Source: `_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md` §2.1-§2.4] — ctx 传播；本 story 复用 context.Background()
- [Source: `_bmad-output/implementation-artifacts/4-2-mysql-接入.md`] — 上游 story；本 story 复用 db.Open + tx.WithTx + tx.FromContext
- [Source: `_bmad-output/implementation-artifacts/4-3-五张表-migrations.md`] — 上游 story；本 story 复用 5 张表 schema + migrations helper
- [Source: `_bmad-output/implementation-artifacts/4-4-token-util.md`] — 上游 story；本 story 调 `auth.New(...)` 装配 signer
- [Source: `_bmad-output/implementation-artifacts/4-5-auth-rate_limit-中间件.md`] — 上游 story；本 story AC8 不消费（handler 单测无需 Auth 中间件）
- [Source: `_bmad-output/implementation-artifacts/4-6-游客登录接口-首次初始化事务.md`] — 上游 story；本 story 复用 buildAuthService / startMySQL / runMigrations helper + 4.6 集成测试模式
- [Source: `_bmad-output/implementation-artifacts/4-8-get-home-聚合接口.md`] — 上游 story；本 story 不直接消费 home_service 但与 4.8 同 epic 协同
- [Source: `server/internal/service/auth_service_integration_test.go`] — 本 story 直接追加该文件（4.6 已落地 4 case + helper）
- [Source: `server/internal/service/auth_service.go`] — 4.6 落地的 service 层；本 story 通过 fault injection 包装其依赖验证回滚 / 重入路径
- [Source: `server/internal/app/http/handler/auth_handler.go`] — 4.6 落地的 handler 层；本 story AC8 边界 2 case 在 handler 单测验证长度拦截
- [Source: `docs/lessons/2026-04-26-multi-table-tx-must-cover-all-unique-constraint-races.md`] — 4.6 review-r1 lesson；本 story AC5 100 goroutine 高负载强化该 lesson 的回归保护
- [Source: `docs/lessons/2026-04-24-error-envelope-single-producer.md`] — ErrorMappingMiddleware 是唯一 envelope 生产者；本 story AC8 验证 c.Error(apperror) 经 middleware 翻译为 1002 envelope
- [Source: `docs/lessons/2026-04-24-go-vet-build-tags-consistency.md`] — build tag 一致性 lesson；本 story 在 service / handler 两包加 +build integration tag 必须双行（Go 1.17+ 兼容）
- [Source: `CLAUDE.md` §"工作纪律"] — "资产类操作必须事务 / 状态以 server 为准"；本 story 是节点 2 资产事务的 Layer 2 收尾
- [Source: `CLAUDE.md` §"Build & Test"] — `bash scripts/build.sh --integration` 跑集成测试
- [Source: `MEMORY.md` "No Backup Fallback"] — 反对 fallback 掩盖核心风险；本 story 验证回滚路径不被 silent recovery 掩盖
- [Source: `MEMORY.md` "Repo Separation"] — server 测试自包含；本 story 单 server 集成测试，不依赖 iPhone App / watch

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m] (Anthropic Opus 4.7, 1M context)

### Debug Log References

- `bash scripts/build.sh --test` → BUILD SUCCESS（unit + 集成包外测试全绿）
- `bash scripts/build.sh --integration` → BUILD SUCCESS（dockertest 真实 MySQL 起容器，service / handler 两层 integration 全绿）

### Completion Notes List

- 实现 epics.md §Story 4.7 行 1090-1104 钦定的 8 类 Layer 2 集成测试场景，全部基于 dockertest 真实 MySQL（**禁用** sqlmock）。
- `internal/service/auth_service_integration_test.go` 在原 4.6 落地的 happy / 复用 / race / 不同 guestUid case 基础上 +624 行，覆盖：
  - 回滚 1：mock pet repo 第 3 步抛 error → 验证 users / bindings 回滚（DB 表为空）。
  - 回滚 2：mock chest repo 第 5 步抛 error → 验证前 4 步全部回滚。
  - 回滚 3：mock user repo 第 1 步抛 error → 验证什么都没创建。
  - 并发 1：100 个 goroutine 并发用同一 guestUid → 最终 DB 只有 1 个 user，所有 goroutine 拿到同一 user_id。
  - 并发 2：100 个 goroutine 并发用 100 个不同 guestUid → DB 100 个 user，每个 user 5 行关联数据，无串数据。
  - 边界 1：guestUid 长度 128（最大允许）→ 成功。
  - 重入：同一 guestUid 已成功登录 → 第二次走复用分支，DB 行数不变。
- 新增 `internal/app/http/handler/auth_handler_integration_test.go`（127 行）覆盖：
  - 边界 2：guestUid 长度 129 → handler 层即拦截，返回 1002 + 不命中 service。
- service / handler 两包均使用双行 `//go:build integration` + `// +build integration` build tag（Go 1.17+ 兼容，遵循 lesson `2026-04-24-go-vet-build-tags-consistency.md`）。
- ErrorMappingMiddleware 作为唯一 envelope 生产者（lesson `2026-04-24-error-envelope-single-producer.md`）—— handler 层只 `c.Error(apperror)`，由 middleware 翻译为 1002 envelope，集成测试断言整条链路一致。

### File List

- 修改：`server/internal/service/auth_service_integration_test.go`（+624 行，新增 7 个场景；原 4.6 happy / 复用 / race / 不同 guestUid case 保留）
- 新增：`server/internal/app/http/handler/auth_handler_integration_test.go`（127 行，handler 层边界 129 拦截）

### Change Log

- 2026-04-26 — Dev (claude-opus-4-7[1m]) — 完成 8 类场景 Layer 2 集成测试代码，build / unit test / integration test 三绿；状态翻转 in-progress → review，等待 code review。
