# ADR-0007: Go context 传播框架

- **Status**: Accepted
- **Date**: 2026-04-24
- **Decider**: Developer
- **Supersedes**: N/A
- **Related Stories**: 1.3 (Logging 中间件落地 + ctx_done 字段 defer), 1.5 (sample service 第一参数 ctx 示范 + ctx cancel 测试 defer), 1.8 (AppError Wrap 保留 Cause 让 errors.Is 穿透 ctx.Err), **1.9 (本决策落地)**, 1.10 (server README 引用本 ADR), Epic 4+ (repo WithContext + txManager.WithTx 首次真实装)

---

## 1. Context

### 1.1 设计文档 §5 / §10 的隐含约定

`docs/宠物互动App_Go项目结构与模块职责设计.md` §5（分层职责）与 §10（事务边界）里钦定了：

- §5.2 Service 层：负责"核心业务逻辑 + 跨模块编排 + 事务边界控制"
- §5.3 Repository 层：负责"数据读写 / SQL 查询与更新 / Redis key 读写"
- §10 事务管理：`err := txManager.WithTx(ctx, func(txCtx context.Context) error { ... })`

但**隐含而未明写**的约定：

1. service / repo 所有导出函数第一个参数是什么？文档没说
2. repo 调 DB / Redis 时怎么传 ctx？文档没说
3. `txManager.WithTx` 里 fn 拿到的 `txCtx` 与外层 `ctx` 的关系？文档没说（只给了函数签名）

Epic 4+（首个真实 repo + 首个真实事务）落地时，如果这些约定没有显式文档兜底，三种典型 bug 会出现：

- **bug A**：service 写 `func (s *X) DoIt(uid string, ctx context.Context)` —— ctx 不是第一参数，风格漂移，review 看不出来
- **bug B**：repo 写 `db.Where(...).First(...)` 不带 `WithContext(ctx)` —— 客户端断开时 GORM 不感知，SQL 继续跑到底 + 占用连接池 + 脏事务
- **bug C**：tx fn 写 `userRepo.Find(ctx, ...)` 用**外层 ctx** 而非 `txCtx` —— 这次 Find 不走 tx 连接，读不到本 tx 里未 commit 的数据，业务语义错乱

本 ADR 的目的：**一次性固化 4 条 ctx 约定 + cancel 传播协作图 + 与 Logging `ctx_done` 字段的契约 + Epic 4+ 迁移清单**，让 bug A/B/C 在 code review 时有明确出处可拒。

### 1.2 上游 story 留下的两处 "等 1.9" 借条

- **Story 1.3 Logging 中间件** 陷阱 #5 原文："Story 1.9 追加字段 `ctx_done`（epics.md Story 1.9 AC）：logging 中间件末尾读 `c.Request.Context().Err()` 判断 cancel。**本 story 不做**，Story 1.9 会 revisit 本文件。"
- **Story 1.5 sample service** 顶部注释原文："所有导出方法第一个参数是 ctx context.Context —— Story 1.9 即将固化为 ADR 0007，本包先示范该约定。" + Story 1.5 范围红线："**不**在 sample service 里做 ctx cancellation 测试：Story 1.9 AC 明示它会**追加**一条 ctx cancel 测试到 sample 模板"

### 1.3 Story 1.8 的前置依赖

ADR-0006 §1.4 明示："Story 1.9 context 传播框架的 AC 第一行 'Given Story 1.8 AppError 框架已就绪 + Gin 默认会把 client 断开的信号传到 ctx' —— 本 ADR 落地是 1.9 的前置。"

本 ADR 依赖 Story 1.8 提供的：

- `apperror.Wrap(err, code, msg)` —— service 层可以用 `apperror.Wrap(ctxErr, apperror.ErrServiceBusy, "请求已取消")` 包 ctx.Err()，让 `stderrors.Is(wrapped, context.Canceled)` 穿透（AppError.Unwrap 返回 Cause）
- `ErrorMappingMiddleware` —— handler 抛 `c.Error(ctxErr)` 时，middleware 兜底 1009 + HTTP 500（见 §6 业务错误码对接策略）

### 1.4 ADR-0001 §4 的 logger 字段先例

ADR-0001 §4 钦定：

- `error_code` 字段：成功请求**省略**字段（缺省即 false）
- 所有日志通过 `slog.InfoContext(ctx, msg, ...)` / `slog.ErrorContext(ctx, msg, ...)`，**禁止**裸 `slog.Info`（丢 ctx）

本 ADR §4 的 `ctx_done` 字段沿用同一"缺省即 false"惯例，保持日志体积不膨胀。

---

## 2. Decision — 四条 ctx 约定

### 2.1 签名约定

所有 service / repo 层**导出**函数的第一个参数必须是 `ctx context.Context`，变量名就叫 `ctx`。

```go
// ✅ 正确
func (s *AuthService) GuestLogin(ctx context.Context, uid string) (*Token, error)
func (r *UserRepo) FindByGuestUID(ctx context.Context, uid string) (*User, error)

// ❌ ctx 不是第一参数
func (s *AuthService) GuestLogin(uid string, ctx context.Context) (*Token, error)

// ❌ 变量名不是 ctx
func (s *AuthService) GuestLogin(c context.Context, uid string) (*Token, error)   // c 易与 *gin.Context 混淆
func (s *AuthService) GuestLogin(ctxt context.Context, uid string) (*Token, error) // 拼错
func (s *AuthService) GuestLogin(_ context.Context, uid string) (*Token, error)    // 丢弃 ctx
```

**禁用范围**：

- ❌ 禁止 service 内部再起 `context.Background()` / `context.TODO()` 截断上游 cancel 链路
- ❌ 禁止 repo 的导出方法不接受 ctx（如 `func (r *UserRepo) FindAll() ([]*User, error)`）
- ✅ 允许：`cmd/server/main.go` 启动时用 `context.Background()` 作为 server 生命周期根 ctx
- ✅ 允许：测试里用 `context.Background()` 作为测试 ctx 起点

**检查手段**：code review 看签名第一行；`go vet` / `gopls` 不强检，靠人 review。本 ADR 固化后，review 看到不合规签名**直接打回**。

### 2.2 Handler 约定

Gin handler 层从 `c.Request.Context()` 取 ctx 并向下传。

```go
// ✅ 正确
func GuestLoginHandler(c *gin.Context) {
    token, err := authSvc.GuestLogin(c.Request.Context(), req.GuestUID)
    // ...
}

// ❌ 把 c 直接当 ctx 传（c 实现了 context.Context 接口，但 c.Done() 返回 nil channel —— 不响应 client 断开）
func GuestLoginHandler(c *gin.Context) {
    token, err := authSvc.GuestLogin(c, req.GuestUID)  // 错！
}

// ❌ 用 context.Background() 截断 cancel 链路
func GuestLoginHandler(c *gin.Context) {
    token, err := authSvc.GuestLogin(context.Background(), req.GuestUID)  // 错！
}

// ❌ 用 context.TODO()
func GuestLoginHandler(c *gin.Context) {
    token, err := authSvc.GuestLogin(context.TODO(), req.GuestUID)  // 错！
}
```

**为什么 `c *gin.Context` 不能当 `context.Context` 用**：

- `c` 实现了 `context.Context` 接口（有 `Deadline / Done / Err / Value` 方法）
- 但 Gin 的 `c.Done()` 默认返回 `nil` channel（即永不 cancel）—— 除非调用方显式 `c.Request.Context()`
- client 断开时 net/http server 只 cancel `c.Request.Context()`，不 cancel `c` 本身
- 结论：handler 传 `c` 给 service/repo 等于传了一个永远不会 cancel 的 ctx —— client 断开时链路仍在运行，资源不释放

Gin v1.12.0（本项目 pin 版本）已修复历史上 `c.Request.Context()` 与 HTTP request ctx 一致性的 bug（见 gin-gonic/gin#1847）。不需要额外配置。

### 2.3 Repo 约定

Repo 层调 DB / Redis 必须用 ctx-aware 方法。

```go
// ✅ 正确（GORM）：db.WithContext(ctx) 让 GORM 把 ctx 传给 database/sql driver
func (r *UserRepo) FindByGuestUID(ctx context.Context, uid string) (*User, error) {
    var user User
    if err := r.db.WithContext(ctx).Where("guest_uid = ?", uid).First(&user).Error; err != nil {
        return nil, err
    }
    return &user, nil
}

// ❌ 错误（GORM）：不带 WithContext，GORM 忽略 ctx cancel
func (r *UserRepo) FindByGuestUID(ctx context.Context, uid string) (*User, error) {
    var user User
    if err := r.db.Where("guest_uid = ?", uid).First(&user).Error; err != nil {
        return nil, err  // ctx 已 canceled 时仍会 wait SQL 跑完
    }
    return &user, nil
}

// ✅ 正确（Redis v9）：client.Get(ctx, key) API 就是 ctx-first
func (r *SessionRepo) Get(ctx context.Context, uid string) (string, error) {
    return r.client.Get(ctx, "session:"+uid).Result()
}
```

**Repo 层的关键职责**：确保 ctx cancel 时，**DB/Redis driver 层**立即停止等待，返回 `context.Canceled` / `context.DeadlineExceeded`。然后 repo 原样向上返回 err（不 wrap，让 service 层决策是否 Wrap 为 AppError，见 ADR-0006 §2.2）。

**Service 层无需 `ctx.Err()` 预检**：只要 repo 正确 WithContext，service 层的 `s.repo.X(ctx)` 在 ctx canceled 时会**立即**返回 ctx.Err()，service 原样向上 return 即可。pre-check `if ctx.Err() != nil` 是性能微优化（极罕见场景能省 1 次 repo 调用），**不是**正确性要求。不推荐 service 普遍写 pre-check。

### 2.4 Tx 约定

`txManager.WithTx(ctx, fn)` 模式下，fn 内部所有 repo 调用必须用 `txCtx`（不是外层 `ctx`）。

```go
// ✅ 正确：fn 内 repo 调用都用 txCtx
func (s *AuthService) GuestInitialize(ctx context.Context, uid string) (*Token, error) {
    err := s.txManager.WithTx(ctx, func(txCtx context.Context) error {
        user, err := s.userRepo.CreateUser(txCtx, uid)  // ← txCtx
        if err != nil { return err }
        _, err = s.petRepo.CreateDefaultPet(txCtx, user.ID)  // ← txCtx
        if err != nil { return err }
        return s.stepRepo.CreateAccount(txCtx, user.ID)  // ← txCtx
    })
    if err != nil {
        return nil, apperror.Wrap(err, apperror.ErrServiceBusy, "服务繁忙")
    }
    return token, nil
}

// ❌ 错误：fn 内用外层 ctx 而非 txCtx
func (s *AuthService) GuestInitialize(ctx context.Context, uid string) (*Token, error) {
    err := s.txManager.WithTx(ctx, func(txCtx context.Context) error {
        user, err := s.userRepo.CreateUser(ctx, uid)  // ← 外层 ctx，不走 tx 连接！
        // ...
    })
    // 问题：CreateUser 用 ctx 而非 txCtx，底层 GORM 会从 ctx 取 *sql.DB 而非 *sql.Tx
    // → 该调用不在 tx 中，与后续 txCtx 调用分离
    // → tx 里未 commit 的 user 记录对 petRepo / stepRepo 不可见
    // → 业务语义错乱
}
```

**txCtx 与外层 ctx 的关系**：

- `txCtx` 是 `ctx` 的 child context，携带了 tx 句柄（`*sql.Tx` / `*gorm.DB` 的 tx instance）
- `txCtx` 继承了外层 `ctx` 的 cancel 语义 —— 外层 cancel 会传播到 txCtx，让 tx 整体 rollback
- GORM 的实装：`db.WithContext(txCtx).Where(...).First(...)` 会从 txCtx 里取出 tx instance 当连接用；若用外层 ctx 则取不到，回退到 db pool 起新连接
- 本规则的真正含义：**fn 内的 repo 调用，tx 感知是通过 ctx 透传的**，漏传一次 = 一次调用脱离 tx

**checker / linter 支持**：Go 社区无现成 linter 强检 txCtx 正确使用。靠 code review + 测试覆盖（每个 tx 落地必有 layer-2 集成测试验证 "要么全 commit 要么全 rollback"，漏传 txCtx 时 rollback 会留下脏数据）。

---

## 3. Cancel 传播协作图

```
[client 发起 HTTP 请求]
         │
         ▼
[Gin HTTP server 接收]
         │ 创建 c *gin.Context，c.Request.Context() 与 request 的 ctx 关联
         ▼
[RequestID middleware]  ── 生成 request_id 存入 ctx（通过 c.Request.WithContext）
         │
         ▼
[Logging middleware]    ── reqLogger 存 ctx；c.Next() 内部执行下游
         │
         ▼
[ErrorMappingMiddleware]── after c.Next() 扫 c.Errors 写 envelope
         │
         ▼
[Recovery middleware]   ── defer recover() 抓 panic，c.Error(wrap)
         │
         ▼
[handler]               ── svc.Do(c.Request.Context(), ...)
         │                 ↓ ctx 原样下传
         ▼
[service]               ── s.repo.X(ctx, ...)
         │                 ↓ ctx 原样下传
         ▼
[repo]                  ── db.WithContext(ctx).Where(...).First(...)
         │                 ↓ ctx 传给 GORM / database/sql driver
         ▼
[MySQL driver]          ── driver 在 Wait 系统调用里 select ctx.Done() / socket read

================ 正常路径 ================

MySQL 返回结果 → repo 返回 (dto, nil) → service 处理 → handler response.Success
         │
         ▼
ErrorMappingMiddleware 看 c.Errors 为空 → no-op
         │
         ▼
Logging: c.Next() 返回后读 c.Request.Context().Err() = nil
         → ctx_done 字段**省略**（缺省即 false）
         → http_request 日志: {msg:"http_request", status:200, ...}

================ cancel 路径 ================

client 断开 TCP → Go net/http server 检测到 → cancel request 的 ctx
         │
         ▼
MySQL driver 在 select 里见 ctx.Done() 关闭
         │ 立即返回 context.Canceled / context.DeadlineExceeded
         ▼
GORM 把 driver 的错包一层，err = "context canceled"（错误链上 errors.Is(err, context.Canceled) == true）
         │
         ▼
repo 原样 return err → service 原样 return err → handler c.Error(err) + c.Abort()
         │
         ▼
ErrorMappingMiddleware: 扫 c.Errors[0]，As 出不了 AppError（err 是 GORM 包的原生错）
         → wrap 为 ErrServiceBusy(1009) + HTTP 500
         → 但此时 client 已断开，response.Error 的 JSON 写到 TCP 会失败（被 net/http 吞掉）
         → 实际副作用：server 侧 goroutine 快速退出，不再占用 DB 连接
         │
         ▼
Recovery defer 看 rec == nil → no-op
         │
         ▼
Logging: c.Next() 返回后读 c.Request.Context().Err() = context.Canceled / DeadlineExceeded
         → ctx_done = true 追加到 http_request 日志
         → http_request 日志: {msg:"http_request", status:500, error_code:1009, ctx_done:true, ...}
         → 运维聚合 count(ctx_done=true) by api_path 识别"被 cancel 的请求"
```

**关键点**：链路上任一环漏接 ctx / 用 `context.Background()`，都会让 cancel 丢失：

- handler 漏传 → service/repo 用到的 ctx 是 Background → MySQL driver 不感知 cancel → SQL 跑到底
- repo 漏 `WithContext` → GORM 不传 ctx 到 driver → 同上
- tx fn 用外层 ctx 而非 txCtx → repo 调用不走 tx 连接 → tx 语义错乱（§2.4）

Story 1.9 的代码改动只有 Logging middleware 末尾读 ctx.Err()，其余约定靠 Epic 4+ 落地时按本 ADR §7 迁移清单逐条检查。

---

## 4. 与 Story 1.3 Logging `ctx_done` 字段的契约

### 4.1 字段位置与时机

`server/internal/app/http/middleware/logging.go` 的 `Logging()` 函数，在 `c.Next()` **之后**、`error_code` 追加块**之后**、`reqLogger.LogAttrs` 调用**之前**：

```go
if err := c.Request.Context().Err(); err != nil {
    attrs = append(attrs, slog.Bool("ctx_done", true))
}
```

**时机**：`c.Next()` 之后读 ctx.Err() 是**唯一稳定读取点** —— handler 执行过程中 ctx 状态可能在 goroutine 之间波动；只有 handler 返回后，net/http server 才对 ctx 做"最终状态"读取。

### 4.2 字段语义

- `ctx_done` 字段类型：JSON boolean
- `ctx_done: true` —— 请求被 cancel（client 断开 / deadline exceeded / 显式 cancel）
- 字段**省略**（不写 `ctx_done: false`） —— 请求正常完成

### 4.3 为什么缺省即 false（不显式写 false）

- 与 `error_code` 字段（成功请求省略）惯例一致，日志体积不膨胀
- 监控聚合 `count(ctx_done=true) by api_path` 不受字段缺失影响（PromQL / Grafana 原生支持"字段存在"判断）
- 若未来要按"比例"做 dashboard（`cancel_rate = count(ctx_done=true) / count(*)`），分母照样能从 `msg="http_request"` 筛出，不需要每条日志都写 `ctx_done: false`

### 4.4 为什么不区分 `context.Canceled` 与 `context.DeadlineExceeded`

- 运维视角：两者都属"客户端侧导致的请求提前终止"，聚合信号一致
- 应用代码视角：Go stdlib 约定两者都让 `ctx.Err() != nil`，区分需要 `errors.Is(err, context.Canceled)` 与 `errors.Is(err, context.DeadlineExceeded)` 两次判断
- **预留扩展位**：若未来监控需区分，追加 `ctx_done_reason: "canceled" | "deadline_exceeded"` 字段即可（不破坏 `ctx_done` 的 boolean 语义）

### 4.5 为什么 Logging 独自 derive ctx.Err()，不走 `c.Keys` 广播

- `ctx.Err()` 是 Gin ctx 的**原生状态**，不是其他中间件的"推断结果"
- 两个中间件（假设未来有 metrics middleware 也读 ctx_done）各自读 `ctx.Err()` **不会**漂移 —— ctx 本身就是 canonical source
- 对比 `error_code` 字段：`error_code` 需要 `c.Keys` 广播是因为 ErrorMappingMiddleware **wrap** 了非 AppError 为 1009（经过 wrap 推断），而 ctx.Err() 没有 wrap 层
- 见 `docs/lessons/2026-04-24-middleware-canonical-decision-key.md` Lesson 1：需要 c.Keys 广播的场景是"上游对原始数据做过推断/转换"；ctx.Err() 不是那种场景

---

## 5. sample ctx cancel 测试范例

`server/internal/service/sample/service_ctx_test.go` 提供两条 case：

### 5.1 `slowCtxAwareRepo` —— 响应 ctx 的 fake repo

```go
type slowCtxAwareRepo struct {
    delay time.Duration // 正常路径 sleep 多久
    dto   *sample.SampleDTO
}

func (r *slowCtxAwareRepo) FindByID(ctx context.Context, id string) (*sample.SampleDTO, error) {
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    case <-time.After(r.delay):
        return r.dto, nil
    }
}
```

**为什么不用 testify/mock**：testify 的 `Mock.Called` 在 mock 侧不感知 ctx.Done() channel，无法模拟"sleep 里面被 cancel 唤醒"这种场景。一次性的 struct 比 mock 更贴近真实意图，也更贴近 Epic 4+ 真 GORM repo 的 cancel 语义（GORM 底层调 MySQL driver 的 `QueryContext` 也是 ctx + socket read 的 select）。

### 5.2 Case 1：ctx cancel 提前返回

`TestSampleService_CtxCancelPropagates`：100ms timeout ctx + 5s slow repo → service 必须 < 500ms 返回 + err 满足 `errors.Is(err, context.DeadlineExceeded)`。

### 5.3 Case 2：ctx 正常时不提前返回

`TestSampleService_CtxNormalPath_ReturnsNormally`：不 cancel ctx + 10ms fast repo → service < 100ms 返回 + value == 42。

这条 happy case 是为防 Case 1 假阴性（证明 slowCtxAwareRepo 在 ctx 正常时确实会 sleep 到 delay 结束）。

### 5.4 Epic 4+ 的复制模板

每个 Epic 4+ 的新 service 至少必须有一条 ctx cancel case（见 §7 迁移清单第 6 条）。典型写法：

```go
func TestAuthService_GuestLogin_CtxCancel(t *testing.T) {
    // 复制 slowCtxAwareRepo 模式到 slowCtxAwareUserRepo
    repo := &slowCtxAwareUserRepo{ delay: 5*time.Second, user: &User{ID: 1} }
    svc := NewAuthService(repo)

    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()

    start := time.Now()
    _, err := svc.GuestLogin(ctx, "uid-123")
    elapsed := time.Since(start)

    assert.ErrorIs(t, err, context.DeadlineExceeded)
    assert.Less(t, elapsed, 500*time.Millisecond)
}
```

---

## 6. 业务错误码对接策略（informational）

### 6.1 ctx.Err() 不是业务错误

`context.Canceled` / `context.DeadlineExceeded` 是 stdlib sentinel error，**不是**业务错误码（V1接口设计 §3 的 32 码与 ctx 无关）。MVP 阶段 service 层拿到 repo 返回的这两类错误：

**推荐**：原样 `return err` —— 最简单。

```go
user, err := s.repo.FindByID(ctx, id)
if err != nil {
    // ctx.Err() / DB 错误都原样向上
    return nil, err
}
```

**允许**：`apperror.Wrap(err, apperror.ErrServiceBusy, "请求已取消")` —— 让 handler 层 / ErrorMappingMiddleware 看到统一的 AppError 类型；AppError.Unwrap 保留 Cause，`errors.Is(wrappedErr, context.Canceled)` 仍可穿透。

```go
user, err := s.repo.FindByID(ctx, id)
if err != nil {
    if stderrors.Is(err, context.Canceled) || stderrors.Is(err, context.DeadlineExceeded) {
        return nil, apperror.Wrap(err, apperror.ErrServiceBusy, "请求已取消")
    }
    return nil, apperror.Wrap(err, apperror.ErrServiceBusy, "服务繁忙")
}
```

**禁止**：`apperror.New(apperror.ErrServiceBusy, "...")` 丢掉 Cause —— `errors.Is(err, context.Canceled)` 返回 false，监控层无法区分"cancel 5xx" vs "真故障 5xx"。

### 6.2 ErrorMappingMiddleware 兜底

handler 抛 `c.Error(ctxErr)` / `c.Error(apperror.Wrap(ctxErr, ...))` 时：

- AppError 形式 → ErrServiceBusy → HTTP 500 + envelope `{code:1009, message:"服务繁忙"}`（见 ADR-0006 §4.5）
- 非 AppError 形式（service 原样上传） → middleware 兜底 wrap 为 ErrServiceBusy → 同上

两者对客户端的响应一致；**差别**只在 log 里是否能 `errors.Is(..., context.Canceled)` 穿透（原样 / apperror.Wrap 可穿透；apperror.New 不可）。

### 6.3 监控告警组合

运维按 `error_code=1009` + `ctx_done=true` 组合识别"cancel 导致的 5xx"：

- `error_code=1009 ∧ ctx_done=true` —— 正常 cancel（客户端断开 / timeout），不告警
- `error_code=1009 ∧ ctx_done=false` —— 真系统故障（panic / DB down），告警
- `error_code=1009 ∧ ctx_done` 字段缺失 —— 等同于 `ctx_done=false`（缺省即 false 惯例）

这也解释了为什么 §4.3 不显式写 `ctx_done: false`：运维规则天然把"缺失"等同于"false"，不需要字段存在性区别。

---

## 7. Future Migration 检查清单（Epic 4+ 每个新 service / repo 走一遍）

Epic 4 (auth) / Epic 7 (step) / Epic 11 (room) / Epic 20 (chest) / Epic 26 (cosmetic) / Epic 32 (compose) 每次新 service / repo / tx 落地时，挨条勾：

- [ ] **C1**：repo 所有导出方法第一参数 `ctx context.Context`（变量名 `ctx`）
- [ ] **C2**：repo 调 DB / Redis / Elasticsearch / 第三方 HTTP 用 `*WithContext` 方法（GORM `db.WithContext(ctx)`；redis/v9 `client.Cmd(ctx, ...)`）
- [ ] **C3**：service 所有导出方法第一参数 `ctx context.Context`；原样下传到 repo；**不**用 `context.Background()` / `context.TODO()` 截断
- [ ] **C4**：handler 从 `c.Request.Context()` 取 ctx；**不**把 `c *gin.Context` 直接当 ctx 传（它的 Done() 是 nil channel）
- [ ] **C5**：长事务用 `txManager.WithTx(ctx, func(txCtx context.Context) error { ... })`；fn 内**所有** repo 调用用 `txCtx`（不是外层 ctx）
- [ ] **C6**：service 单测至少 1 条 ctx cancel case（100ms timeout + 5s slow repo，断言 `elapsed < 500ms` + `errors.Is(err, context.DeadlineExceeded)`）；模板见 §5.4
- [ ] **C7**：如果 service 启了 goroutine（Epic 10+ ws 广播、Epic 20 异步开箱动效等），ctx cancel 必须让 goroutine 退出（通过 `select { case <-ctx.Done(): return ... case <-任务 channel: ... }`）

**review 执行方式**：story 的 Tasks/Subtasks 里建议复制本清单作为 checklist subtask，dev 实装完挨条勾。code-review 阶段 reviewer 按本清单逐条核对。

---

## 8. Change Log

| Date | Change | By |
|---|---|---|
| 2026-04-24 | 初稿（Story 1.9 落地）：四条 ctx 约定（签名 / Handler / Repo / Tx）+ cancel 协作图 + `ctx_done` 字段契约 + Epic 4+ 迁移清单 7 条 | Developer |
