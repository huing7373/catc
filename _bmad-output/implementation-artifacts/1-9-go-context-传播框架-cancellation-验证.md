# Story 1.9: Go context 传播框架 + cancellation 验证

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want 把 ctx propagation 约定固化为 ADR（handler → service → repo 第一参数；repo 必用 `*WithContext`；tx 里 txCtx 必须下钻）+ 至少一条 mock 测试证明 ctx cancel 真的在 100ms 内逃出 5s 慢 repo + logging 中间件追加 `ctx_done` 字段,
so that 未来 Epic 4+ 真实 repo / 长事务落地时不靠开发自觉，客户端断开 / 请求超时时 server 能立即释放资源（不 hang 住事务 + 锁泄漏 + goroutine 积压），同时监控能按 `ctx_done=true` 聚合"被 cancel 的请求"这类信号.

## 故事定位（节点 1 第九条实装 story）

- **节点 1 进度**：Story 1.1 (ADR-0001) → 1.2 (cmd/server + ping) → 1.3 (RequestID/Logging/Recovery 中间件) → 1.4 (/version) → 1.5 (测试基础设施 + sample service 模板) → 1.6 (Dev Tools) → 1.7 (build.sh) → 1.8 (AppError + 三层映射) **已完成**；本 story 是节点 1 横切纪律的最后一条（后续 1.10 是 server README，纯文档）
- **NFR 出处**：
  - `docs/宠物互动App_Go项目结构与模块职责设计.md` §10 "事务边界建议" 钦定 `txManager.WithTx(ctx, func(txCtx context.Context) error { ... })` 模式，但**没**明确说"txCtx 必须下钻到 repo 每一次调用"—— 本 ADR 把这条隐含约定固化
  - 同 §5.1 / §5.2 / §5.3 分层职责里没有显式写 "第一个参数必须是 ctx"，实际 Epic 4+ 落地时会按 Go 社区惯例写，但缺一个**钦定文档**兜底；本 ADR 是那个文档
  - ADR-0001 §4 补充约束 "所有日志通过 `slog.InfoContext(ctx, msg, ...)`，**禁止** `fmt.Println` / 裸 `slog.Info`（丢 context）" —— 已落地，但缺 ctx cancel 语义的对应约定
- **三条上游 story 的承接**：
  1. **Story 1.3 Logging 中间件**（`server/internal/app/http/middleware/logging.go`）：Dev Notes 陷阱 #5 明示 "Story 1.9 追加字段 `ctx_done`（epics.md Story 1.9 AC）：logging 中间件末尾读 `c.Request.Context().Err()` 判断 cancel。**本 story 不做**，Story 1.9 会 revisit 本文件。" —— 本 story 兑现
  2. **Story 1.5 sample service**（`server/internal/service/sample/service.go`）：顶部注释明示 "所有导出方法第一个参数是 ctx context.Context —— Story 1.9 即将固化为 ADR 0007，本包先示范该约定。" —— 本 story 固化 ADR；且 Story 1.5 自己把 "**不**在 sample service 里做 ctx cancellation 测试：Story 1.9 AC 明示它会**追加**一条 ctx cancel 测试到 sample 模板；本 story 保留'同步路径'骨架，Story 1.9 自己扩展" 钦定进 scope
  3. **Story 1.8 AppError**：ADR-0006 §1.4 "Story 1.9 立即下游" 明示本 story 依赖 AppError + apperror.Wrap 已就绪（用 ctx.Err() 作为 Cause 透传给 service 的上层调用，通过 `stderrors.Is(err, context.Canceled) / context.DeadlineExceeded` 在未来 handler 层做可选分流）
- **下游 story 立即依赖**：Story 1.10 server README 需列"ctx 传播约定"在"工作纪律"章节；Epic 4 Story 4.2 落地 GORM 时 `db.WithContext(ctx)` 是本 ADR 的首个真实调用方；Epic 4 Story 4.6 游客登录事务落地 `txManager.WithTx(ctx, func(txCtx context.Context) error { ... })` 是 §2.4 txCtx 下钻规则的首个真实调用方

**范围红线（本 story 只做以下六件事）**：

1. 新建 `_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md`：8 章节，含 4 条 ctx 约定 + cancel 传播协作图 + sample 测试范例引用 + Epic 4+ 迁移指南 + Change Log
2. 修改 `server/internal/app/http/middleware/logging.go`：`c.Next()` 之后读 `c.Request.Context().Err()`，仅在 **非 nil** 时追加 `ctx_done=true` 字段到 `http_request` 日志（成功/超时/cancel 三类路径区分；沿用"缺省即 false"的 `error_code` 字段惯例，避免日志体积翻倍）
3. 修改 `server/internal/app/http/middleware/logging_test.go`：追加 ≥2 条 case 覆盖 `ctx_done` 字段（正常请求不出现 / 客户端断连时出现且为 true）
4. 新建 `server/internal/service/sample/service_ctx_test.go`：ctx cancel 测试 case（mock 一个 "sleep 5s 但响应 ctx 的 repo" → service 被 100ms ctx timeout 强制提前返回，全程 < 500ms）
5. 新建 `server/internal/app/http/middleware/ctx_propagation_integration_test.go`（或在 `error_mapping_integration_test.go` 追加 case）：handler 模拟 client 断开场景，通过 `httptest.NewRequest + req.WithContext(cancelableCtx)` 把 cancel 传进去 → 断言 handler / service 链路提前返回
6. 修改 `server/internal/app/http/middleware/logging.go` 顶部注释：追加 `ctx_done` 字段语义说明；同步 `_bmad-output/implementation-artifacts/1-3-中间件-request_id-recover-logging.md` 陷阱 #5 改写为"已在 1.9 落地"的 backfill（**不**改 1-3 story 的 Tasks/AC，那已 done）

**本 story 不做**：

- ❌ **不**修改 `sample/service.go` 的 `GetValue` 源码加 `ctx.Err()` 预检 —— sample 的 ctx 传递已正确（第一参数 + 原样下传到 repo），repo cancel 时错误原样向上传是**标准模式**；pre-check ctx.Err() 在 repo 快速返回场景下纯属额外分支，反而让模板变复杂。ADR §2.3 会明示 "service 层无需主动 `ctx.Err()`，repo 已 WithContext 即可"
- ❌ **不**在 `internal/pkg/testing/helpers.go` 加 `NewCancelableCtx` / `SleepyRepo` 等 helper —— 本 story 只有 1-2 个测试用得上，内联 `context.WithTimeout` + 匿名 `slowRepo{}` struct 足够；future 如 Epic 4+ 普遍需要再抽 helper（遵循 CLAUDE.md "三条相似先于抽象"原则）
- ❌ **不**落地 `repo/tx/manager.go`（`txManager.WithTx` 真实实装）—— MySQL 还没接，没有可注入的 `*sql.Tx` / `*gorm.DB`。ADR 记录契约，Epic 4 Story 4-2 落地
- ❌ **不**接入真实 GORM `db.WithContext(ctx)` —— 同上，GORM 还没进 `go.mod`，测试用 sqlmock 也不到时候在 ctx 维度验证（sqlmock 不模拟 WithContext 语义；真正验证在 Epic 4 Story 4-7 的 layer-2 集成测试）
- ❌ **不**在 service 层/ handler 层引入 `context.Canceled` / `context.DeadlineExceeded` 到业务错误码的映射 —— MVP 阶段 ctx cancel 原样向上，最终由 Gin / ErrorMappingMiddleware 走 500 兜底（AppError Wrap 为 ErrServiceBusy）；若未来监控需要区分 "正常 5xx" vs "cancel 5xx"，单独 ADR 演进
- ❌ **不**引入新依赖（`context` 是 stdlib；测试用 `time.After` + `context.WithTimeout` 都是 stdlib）；`go.mod` 不动
- ❌ **不**改 ErrorMappingMiddleware / Recovery 的 ctx 使用（已正确用 `c.Request.Context()`，见 error_mapping.go:102 / recover.go:51-52）
- ❌ **不**为 `/ping` / `/version` / `/metrics` / `/dev/ping-dev` 这些空 handler 加 ctx cancel 测试 —— 它们没有 IO / 耗时，cancel 无可观测效果；本 story 的 cancel 测试集中在 "service → repo"（sample）和 "handler → service → repo"（集成）两层

## Acceptance Criteria

**AC1 — ADR-0007 文档（`_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md`）**

新建文件，至少 **8 章节**，内容如下（每节都需要写足够判据与禁忌，后续 Epic 4+ 落地时直接对照）：

| 章节 | 内容 |
|---|---|
| **1. Context** | 引用 设计文档 §5 / §10；引用 Story 1.3 `ctx_done` defer；引用 Story 1.5 sample 的"Story 1.9 即将固化"注释；引用 Story 1.8 ADR-0006 §1.4；说明为什么 MVP 阶段必须现在固化（Epic 4+ 的 repo / tx 落地若无文档约束，一定会出现 ctx 丢失 bug） |
| **2. Decision — 四条 ctx 约定** | 2.1 签名约定：service/repo 所有导出函数第一参数必须是 `ctx context.Context`（命名就叫 `ctx`，不允许 `c`/`context`/`ctxt`/`_`）<br>2.2 Handler 约定：handler 层必须从 `c.Request.Context()` 取 ctx 并向下传；**不**接受 `context.Background()` / `context.TODO()` 作为业务 ctx 起点<br>2.3 Repo 约定：repo 层调 DB / Redis 必须用 `*WithContext` 方法（GORM `db.WithContext(ctx).Where(...).First(...)`；redis/v9 `client.Get(ctx, key)` 已 ctx-first；miniredis 同）；**禁止** `db.Where(...).First(...)` 不带 ctx 的调用（会忽略 cancel）<br>2.4 Tx 约定：`txManager.WithTx(ctx, func(txCtx context.Context) error { ... })` 模式下，**fn 内部所有 repo 调用必须用 txCtx（不是外层 ctx）**；若用了外层 ctx，tx 逻辑不会自动绑定到该事务 + cancel 时也可能错过 tx rollback 路径 |
| **3. Cancel 传播协作图** | 文字流程图：`client 断开 → Gin 自动 cancel c.Request.Context() → handler 取 c.Request.Context() → service(ctx) → repo(ctx) → db.WithContext(ctx).First() → MySQL driver 感知 cancel → 立即返回 context.Canceled / context.DeadlineExceeded → repo 原样上传 → service 原样上传 → handler c.Error(err) → ErrorMappingMiddleware wrap 为 1009 envelope（由于 response 写不出去客户端已断，actual side effect 是 goroutine 快速退出）→ Logging 读 ctx.Err() != nil → log ctx_done=true`；**强调**链路任一环漏接 ctx / 用 ctx.Background()，都会让 cancel 丢失 → hang |
| **4. 与 Story 1.3 Logging `ctx_done` 字段的契约** | `ctx_done` 字段语义：`c.Next()` 之后读 `c.Request.Context().Err()`；非 nil 时 log 中追加 `ctx_done=true`；nil 时**省略字段**（与 `error_code` 成功路径"省略字段"行为一致）。**不**区分 `context.Canceled` vs `context.DeadlineExceeded`（监控层用 `count(ctx_done=true) by api_path` 聚合"被 cancel 的请求"信号；若未来需要细分，扩 `ctx_done_reason` 字段） |
| **5. sample ctx cancel 测试范例** | 引用 `server/internal/service/sample/service_ctx_test.go` 的写法：mock 一个"sleep 5s 但响应 ctx" 的 repo（见 AC4 代码骨架）+ service 被 100ms ctx timeout 强制提前返回；断言 `elapsed < 500ms` 且 `errors.Is(err, context.DeadlineExceeded)`。**未来 Epic 4+ 真实 repo 也**必须有至少 1 条同形态 ctx cancel 测试（见 §7 迁移检查清单） |
| **6. 业务错误码对接策略（informational）** | ctx.Err() 不是业务错误 —— MVP 阶段 service 层拿到 `context.Canceled` / `context.DeadlineExceeded` 后**原样向上 return**（或用 `apperror.Wrap(err, apperror.ErrServiceBusy, "请求已取消")` 也可，但**非强制**）。handler / ErrorMappingMiddleware 兜底 1009 + HTTP 500。监控告警按 `error_code=1009` + `ctx_done=true` 的组合识别"cancel 导致的 5xx"而非"系统故障导致的 5xx"，不影响 alert 规则 |
| **7. Future Migration 检查清单（Epic 4+ 每个新 service / repo 走一遍）** | □ repo 所有方法第一参数 `ctx context.Context`<br>□ repo 调 DB / Redis / Elasticsearch / 第三方 HTTP 用 `*WithContext` 方法<br>□ service 所有方法第一参数 `ctx context.Context`；原样下传到 repo；**不**用 `context.Background()` 截断<br>□ handler 从 `c.Request.Context()` 取 ctx；**不**用 `context.TODO()`<br>□ 长事务用 `txManager.WithTx(ctx, fn)`；fn 内 **所有** repo 调用用 `txCtx`<br>□ service 单测至少 1 条 ctx cancel case（100ms timeout + 5s slow repo，断言提前返回）<br>□ 如果 service 启了 goroutine（如 Epic 10+ ws 广播），ctx cancel 必须让 goroutine 退出（通过 `select { case <-ctx.Done(): ... }`） |
| **8. Change Log** | `\| 2026-04-24 \| 初稿（Story 1.9 落地）：四条 ctx 约定 + cancel 协作图 + ctx_done 字段契约 + Epic 4+ 迁移清单 \| Developer \|` |

**AC2 — `logging.go` 追加 `ctx_done` 字段**

修改 `server/internal/app/http/middleware/logging.go`：

```go
// 在 c.Next() 之后、reqLogger.LogAttrs 之前、现有 error_code 读取分支**旁**
// （建议：error_code 追加块之后）加入 ctx_done 追加块：
if err := c.Request.Context().Err(); err != nil {
    // ctx 被 cancel / deadline exceeded → 客户端断连或请求超时
    // 语义：缺省 = 正常完成；存在 = 被 cancel（不再细分 Canceled vs DeadlineExceeded，
    // 聚合成本低；若未来监控需区分，扩 ctx_done_reason 字段，见 ADR-0007 §4）
    attrs = append(attrs, slog.Bool("ctx_done", true))
}
```

**关键约束**：

- 缺省即 false（字段不出现）—— 与 `error_code` 成功路径省略字段的惯例一致，避免 http_request 日志条目体积 ×2
- **只读不写**：`c.Request.Context()` 是 Gin 维护的生命周期 ctx，本中间件只读 `Err()`，不 modify ctx（不调 `context.WithCancel` 等）
- 日志字段名必须是 `ctx_done`（snake_case 与 `error_code` / `request_id` 风格一致；不是 `ctxDone` / `canceled` / `cancelled`）
- 本中间件顶部注释追加一段说明 `ctx_done` 字段语义（与 ADR-0007 §4 对齐）

**AC3 — `logging_test.go` 追加 ≥2 条 ctx_done case**

追加到 `server/internal/app/http/middleware/logging_test.go`：

| # | Case 名 | 类型 | 断言点 |
|---|---|---|---|
| 1 | `TestLogging_NoCtxDoneOnSuccess` | happy | `/ping` 正常完成 → log 中**不含** `ctx_done` 字段（缺省即 false） |
| 2 | `TestLogging_CtxDoneWhenClientDisconnects` | happy | handler 故意调 `ctx.Done()` 前接一个**已 cancel** 的 ctx（通过 `httptest.NewRequest` + `req.WithContext(canceledCtx)` 模拟 client 断开）→ handler 立即 return → log 含 `ctx_done=true` |

**关键约束**：

- 不需要真起 server；用 `httptest.NewRequest(...)`+ `req.WithContext(ctx)` + `engine.ServeHTTP(w, req)` 直接喂 cancelable ctx 给 engine
- Case 2 的 handler 不必真 sleep —— 可以 `c.JSON(200, gin.H{"ok":true})` 成功返回，但因 `req.Context()` 已 canceled，`c.Request.Context().Err()` 非 nil → logging 写 `ctx_done=true`
- **兼容** Gin 在 canceled ctx 下仍允许 handler 跑完的行为（Gin 默认不中断 handler，只是传递 ctx）—— 这是真实 HTTP 场景的 faithful 模拟
- 现有 `TestLogging_HappyPath_HasSixFields` 的 `forbidden` 列表需追加 `ctx_done`（与 `user_id` / `business_result` / `error_code` 同类"不该出现的字段"）

**AC4 — sample service ctx cancel 测试（新文件 `service_ctx_test.go`）**

新建 `server/internal/service/sample/service_ctx_test.go`：

```go
package sample_test

import (
    "context"
    "errors"
    "testing"
    "time"

    "github.com/huing/cat/server/internal/service/sample"
)

// slowCtxAwareRepo 是一个"响应 ctx 的慢 repo" —— 实装 SampleRepo 接口，
// 在 FindByID 里 select ctx.Done() / time.After(delay)，模拟真实 DB driver
// 在 WithContext 下对 cancel 的即时响应（Epic 4 真 GORM repo 同样行为）。
//
// 为什么不用 testify/mock：testify 的 Mock.Called 在 mock 侧不感知 ctx，
// 无法模拟"sleep 里面被 cancel 唤醒"这种需要 ctx.Done() channel 的场景。
// 一次性的 struct 比 mock 更贴近真实意图，也更贴近 ADR-0007 §5 对 Epic 4+
// repo cancel 测试的示范要求。
type slowCtxAwareRepo struct {
    delay time.Duration // 正常路径会 sleep 多久
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

func TestSampleService_CtxCancelPropagates(t *testing.T) {
    // Given: repo 慢 5 秒；ctx 100ms 后超时
    repo := &slowCtxAwareRepo{
        delay: 5 * time.Second,
        dto:   &sample.SampleDTO{ID: "x", Value: 42},
    }
    svc := sample.NewSampleService(repo)

    ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
    defer cancel()

    // When: service 调用（预期 100ms 内提前返回，而非等 5 秒）
    start := time.Now()
    _, err := svc.GetValue(ctx, "x")
    elapsed := time.Since(start)

    // Then: ctx.Err() 向上传 + 耗时远小于 5s
    if err == nil {
        t.Fatalf("expected ctx deadline error, got nil")
    }
    if !errors.Is(err, context.DeadlineExceeded) {
        t.Fatalf("err = %v, want context.DeadlineExceeded (repo 必须响应 ctx.Done())", err)
    }
    if elapsed >= 500*time.Millisecond {
        t.Fatalf("elapsed = %v, want < 500ms (service 必须提前返回，不能 wait repo sleep 结束)", elapsed)
    }
    // 可选断言：elapsed 至少达到 ctx timeout 时间（排除"0ms 假通过"）
    if elapsed < 50*time.Millisecond {
        t.Logf("WARN: elapsed %v < 50ms，可能未真正触发 ctx timeout；检查 slowCtxAwareRepo 的 select", elapsed)
    }
}
```

**关键约束**：

- 测试**不用** testify/mock —— 见上面代码注释里的理由（mock 不感知 ctx channel 语义）
- `500ms` 阈值是 `100ms ctx timeout` + Go runtime 调度 overhead（Windows 下 timer 精度约 ±15ms）+ test runner overhead 的综合考虑；严阈值会出 flake
- 测试**不**加 `t.Parallel()` —— 涉及 100ms 级 timing，并行运行时 CPU 抢占会拉长 elapsed 误伤断言（Story 1.5 Dev Notes §6 已钦定"sample 测试不 parallel"）
- 同时提供 **happy path** case `TestSampleService_CtxNormalPath`（ctx 不 cancel，repo delay 10ms，elapsed < 100ms 返回正常 value）—— 防止 Case 1 的"假阴性"（断言只检查 error 存在 + elapsed 短，若 repo 永远即时返回 err 也能通过；加 happy case 证明 slowCtxAwareRepo 在 ctx 正常时确实会 sleep）

测试文件**总 2 条 case**：`TestSampleService_CtxCancelPropagates` + `TestSampleService_CtxNormalPath_ReturnsNormally`。

**AC5 — handler/middleware 链路 ctx cancel 集成测试**

新建 `server/internal/app/http/middleware/ctx_propagation_integration_test.go`（或扩 `error_mapping_integration_test.go` 新增 case；建议**新文件**，避免与现有 2 条 envelope case 耦合）。

测试场景（≥1 case，**推荐 2 case**）：

| # | Case 名 | 断言点 |
|---|---|---|
| 1 | `TestCtxPropagation_HandlerSeesCancelViaCRequestContext` | handler 内读 `c.Request.Context().Err()` 立即返回 → 整条链路（handler → no service 直接返回）提前结束 → logging 日志含 `ctx_done=true`；耗时 < 200ms |
| 2（推荐） | `TestCtxPropagation_SlowHandlerCanceledByClientDisconnect` | 用 `httptest.NewServer(bootstrap.NewRouter())` + 真 HTTP client 调用；handler 故意 `select { case <-c.Request.Context().Done(): ... case <-time.After(5*time.Second): ... }`；测试侧用 `context.WithTimeout(ctx, 100ms) + client.Do` 主动取消 → 断言 HTTP client 返回 `context.DeadlineExceeded` 且服务器侧 goroutine 在 500ms 内退出（通过 server log 含 ctx_done=true） |

**关键约束**：

- 手段一（推荐）：`req := httptest.NewRequest(...)`; `ctx, cancel := context.WithCancel(...)`; `cancel()` 立即调; `req = req.WithContext(ctx)`; `engine.ServeHTTP(w, req)` —— 完全在 in-process 跑通，无需开 port
- 手段二（Case 2 推荐）：`httptest.NewServer` + `http.Client` + `client.Do(req.WithContext(ctxTimeout))` —— 验证跨真 TCP 层 cancel 传播
- 集成测试**不**要求严阈值 elapsed < N ms（不同机器 HTTP roundtrip 抖动大）；只要求**存在性**：log 含 `ctx_done=true` + 响应在 ctx timeout ≤ 2× 内返回
- 测试 setup 必须挂完整中间件链 `RequestID → Logging → ErrorMappingMiddleware → Recovery → handler`（与 `newLoggingRouter` 复用 helper）

**AC6 — 中间件挂载顺序与 router.go 不变**

router.go **不改**：现有顺序 `RequestID → Logging → ErrorMappingMiddleware → Recovery` 已正确承载 ctx 传播（Logging 在最外层能读到 final ctx state）；本 story 的 `ctx_done` 字段读取点就在 Logging 的 after-c.Next() 位置，与 `error_code` 字段读取点相邻。

**AC7 — Sprint Status 与 CLAUDE.md**

- `_bmad-output/implementation-artifacts/sprint-status.yaml`：`1-9-go-context-传播框架-cancellation-验证` 状态 `backlog → ready-for-dev`（本 SM 步骤完成时）→ 后续 dev 推进 `→ in-progress → review → done`
- `CLAUDE.md` §"工作纪律"：**建议在本 story 完成时** append 一行 "**ctx 必传**：service / repo 所有函数第一参数 ctx；handler 用 `c.Request.Context()`；repo 调 DB / Redis 必用 `*WithContext`；tx fn 用 txCtx。见 ADR-0007。" —— 但不强制（`docs/宠物互动App_Go项目结构与模块职责设计.md` 已隐含，ADR-0007 提供正式出处）。**最终以 dev 判断**
- `docs/lessons/index.md` 不需改动（本 story 不产 lesson；fix-review 阶段如有 finding 才追加）

**AC8 — 手动验证（补强 AC5）**

```bash
# 确认 go test 全绿
$ bash scripts/build.sh --test
# 特别关注：
#   ok   github.com/huing/cat/server/internal/app/http/middleware  <N.Ns>
#   ok   github.com/huing/cat/server/internal/service/sample  <N.Ns>

# 可选手工：启 server → curl 一个正常请求 → 看 server 日志 http_request 不含 ctx_done
# 可选手工：curl with --max-time 0.001 → 预期 curl 报超时；server 侧日志含 ctx_done=true
$ CAT_HTTP_PORT=18092 ./build/catserver -config server/configs/local.yaml &
$ curl --max-time 0.001 http://127.0.0.1:18092/ping
# 通常因 ctx 太早 cancel 看不到 server 响应，但 server stdout 会看到:
# {"level":"INFO","msg":"http_request",...,"ctx_done":true}
```

## Tasks / Subtasks

- [x] **T1** — ADR-0007 文档（AC1）
  - [x] T1.1 新建 `_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md`
  - [x] T1.2 写 8 章节（严格按 AC1 表）；§3 Cancel 传播协作图用 ASCII 流程图
  - [x] T1.3 §5 测试范例引用 AC4 的 `service_ctx_test.go` 文件路径与 case 名
  - [x] T1.4 §7 迁移清单是 7 条 checkbox（C1-C7），Epic 4+ dev 落地时挨条勾

- [x] **T2** — `logging.go` 追加 `ctx_done`（AC2）
  - [x] T2.1 在 `Logging()` 函数 `c.Next()` 之后、`LogAttrs` 之前，`error_code` 追加块**之后**，加 ctx_done 追加块
  - [x] T2.2 文件顶部注释段落追加 `ctx_done` 字段语义说明（snake_case；缺省即 false；不区分 Canceled vs DeadlineExceeded；与 ADR-0007 §4 对齐）
  - [x] T2.3 字段顺序保持稳定：method / status / latency_ms / client_ip / [error_code] / [ctx_done]

- [x] **T3** — `logging_test.go` 追加 ≥2 case（AC3）
  - [x] T3.1 追加 `TestLogging_NoCtxDoneOnSuccess`（正常路径不含字段）
  - [x] T3.2 追加 `TestLogging_CtxDoneWhenClientDisconnects`（已 cancel 的 ctx 通过 `req.WithContext` 注入）
  - [x] T3.3 追加 `TestLogging_CtxDoneOnDeadlineExceeded`（ADR-0007 §4.4 对称验证 —— 不区分 Canceled vs DeadlineExceeded）
  - [x] T3.4 更新 `TestLogging_HappyPath_HasSixFields` 的 forbidden 列表加 `ctx_done`

- [x] **T4** — sample ctx cancel 测试（AC4）
  - [x] T4.1 新建 `server/internal/service/sample/service_ctx_test.go`
  - [x] T4.2 定义 `slowCtxAwareRepo` struct（实装 `sample.SampleRepo` 接口）
  - [x] T4.3 `TestSampleService_CtxCancelPropagates`：100ms timeout + 5s repo delay → assert < 500ms elapsed + `errors.Is(err, context.DeadlineExceeded)` + elapsed > 50ms 软警告
  - [x] T4.4 `TestSampleService_CtxNormalPath_ReturnsNormally`：ctx 不 cancel + repo delay 10ms → assert elapsed < 100ms + value == 42
  - [x] T4.5 跑 `go test ./internal/service/sample/...` → 两条 case 全绿（0.10s + 0.01s）

- [x] **T5** — 集成测试（AC5）
  - [x] T5.1 新建 `server/internal/app/http/middleware/ctx_propagation_integration_test.go`（`_test` 后缀黑盒风格，包名 `middleware_test`）
  - [x] T5.2 Case 1 `TestCtxPropagation_HandlerSeesCancelViaCRequestContext`（in-process + 手动 cancel，断言 handler 看到 ctx.Err() 非 nil + log 含 ctx_done=true + elapsed < 200ms）
  - [x] T5.3 Case 2 `TestCtxPropagation_SlowHandlerCanceledByClientDisconnect`（httptest.NewServer + 真 HTTP roundtrip，client 100ms timeout + server-side handler select ctx.Done()，断言两侧 elapsed < 500ms）
  - [x] T5.4 跑 `bash scripts/build.sh --test` → 全绿（12 个包，含 cmd/server）

- [x] **T6** — 回溯修正 Story 1.3 陷阱注释
  - [x] T6.1 修改 `_bmad-output/implementation-artifacts/1-3-中间件-request_id-recover-logging.md` 陷阱 #5 / TODO 列表 / References 三处：把 "**本 story 不做**，Story 1.9 会 revisit" 改成 "**Story 1.9 已落地**（ADR-0007 §4 + logging.go `ctx_done` 字段）"；AC / Tasks / Dev Agent Record / File List 保持不动（已 done 的 story 正文冻结）

- [x] **T7** — 手动验证（AC8）
  - [x] T7.1 `bash scripts/build.sh --test` 全绿（见 Dev Agent Record 测试结果摘要）
  - [~] T7.2（可选，跳过）启 binary + curl 超时手工验证：AC5 Case 2 已用 httptest.NewServer 跑真 HTTP roundtrip 覆盖等价路径，不重复跑手工验证

- [x] **T8** — 收尾
  - [x] T8.1 Completion Notes 补全（见下方）
  - [x] T8.2 File List 填充（见下方）
  - [x] T8.3 状态流转 `ready-for-dev → in-progress → review`
  - [x] T8.4 sprint-status.yaml 同步
  - [x] T8.5 CLAUDE.md §"工作纪律" 追加"ctx 必传"一行（AC7 可选项采纳，本 story commit 一并带上 —— 让 ctx 约定在 CLAUDE.md + ADR-0007 两处同时可见）

## Dev Notes

### 项目关键约束（必读，勿绕过）

1. **ctx cancel 是 repo 的责任，不是 service 的责任**：service 层无需主动 `if ctx.Err() != nil { return ctx.Err() }` 预检 —— 那是性能微优化，**不是**正确性要求。正确性由 repo 层（`db.WithContext(ctx)` 或本 story 的 `slowCtxAwareRepo` select pattern）保证。sample.GetValue 已正确：`s.repo.FindByID(ctx, id)` 里面 ctx aware，ctx cancel 时 repo 立即返回 ctx.Err()，service 拿到 err 原样向上 return —— 这就够了。ADR-0007 §2.3 / §6 会明示这点

2. **sample service 不做源码改动**：
   - ❌ **不**加 `if err := ctx.Err(); err != nil { return 0, err }` 在 `GetValue` 顶部
   - ❌ **不**加 `if err := ctx.Err(); err != nil { return 0, err }` 在 repo 调用前
   - ✅ 现状的 "ctx 作为第一参数下传，不做任何 ctx 检查" 已是 **Go 社区 idiomatic + ADR-0007 §2 示范**
   - 只**新增**测试文件 `service_ctx_test.go`，测试验证 "ctx cancel 时 service 确实会因为 repo 感知 ctx 而提前返回"

3. **`slowCtxAwareRepo` 不用 testify/mock**：理由见 AC4 代码注释 —— testify mock 不感知 ctx channel。直接定义一个带 `delay time.Duration` 和 `dto *SampleDTO` 字段的小 struct，在 `FindByID` 里 `select { case <-ctx.Done(): ... case <-time.After(r.delay): ... }`。这种"一次性 fake"在 Go 社区比 mock 更推荐用于 timing / channel / ctx 语义验证

4. **`ctx_done` 字段的 bool 选型**：不用 string `ctx_done="canceled"` / `"deadline_exceeded"` / `""`，因为：
   - 聚合查询 `count(ctx_done=true) by api_path` 最简单
   - 监控面板加一条 single bool 字段不污染 cardinality
   - 如果将来真要区分 Canceled / DeadlineExceeded，**扩展**字段 `ctx_done_reason: "canceled" | "deadline_exceeded"`（ADR-0007 §4 已预留），**不**破坏 `ctx_done` 的语义
   - 选 bool 也与 JSON schema 简单性对齐（Grafana / Alertmanager 对 bool 字段支持原生）

5. **缺省即 false 的字段惯例**：`error_code` / `user_id` / `business_result` / `ctx_done` 都遵循"字段缺失 = 负向信号"的契约（见 lessons/2026-04-24-middleware-canonical-decision-key.md Lesson 2）。本 story 的 `ctx_done` **不**写 `slog.Bool("ctx_done", false)` 让 false 值显式出现 —— 那会让每条 log 都多一个字段，且监控聚合不需要 false 出现

6. **Logging 读 ctx.Err() 的时机**：**必须**在 `c.Next()` 之后 —— Gin 在 handler 执行过程中（或 handler 内部 goroutine）不保证 ctx 状态稳定；只有 handler 返回后，Gin 的 engine 才对 ctx 做一次"最终状态"读取。`c.Next()` 之后读 `c.Request.Context().Err()` 是**统一的结算点**

7. **`c.Request.Context()` vs `c.Request.Context().Err()` 的区别**：
   - `c.Request.Context()` 返回当前的 ctx（不阻塞）
   - `c.Request.Context().Err()` 返回 ctx 的终止原因（nil / Canceled / DeadlineExceeded）；**不阻塞**；可以多次调用（幂等）
   - 本 story Logging 用 `.Err()` 即可；**不**需要 `select { case <-ctx.Done(): ... default: ... }`（那是给 handler 用的，不是给 after-c.Next() 的 logging 用的）

8. **测试 flake 防御**：
   - AC4 的 `< 500ms` 上限留了 400ms buffer 避免 CI slow runner 假失败（Windows Defender / macOS Spotlight 扫描偶发拖慢）
   - AC4 的 `> 50ms` **下限**断言不做硬性 fail，只是 `t.Logf("WARN: ...")` —— 防止在极快机器上 100ms timer 提前到 10ms 就触发也误报
   - AC5 集成测试**不**加 elapsed 严阈值 —— 跨 TCP 的时序抖动更大
   - 所有 ctx cancel 测试都**不**加 `t.Parallel()`（Story 1.5 Dev Notes §6 已确认 parallel 会让 testify mock 期望种族 + ctx timing 放大）

9. **Go context 签名的位置约定**（ADR-0007 §2.1 固化）：
   ```go
   // 正确：ctx 第一参数，变量名就叫 ctx
   func (s *AuthService) GuestLogin(ctx context.Context, uid string) (*Token, error)

   // 错误：ctx 不是第一参数
   func (s *AuthService) GuestLogin(uid string, ctx context.Context) (*Token, error)

   // 错误：变量名不叫 ctx
   func (s *AuthService) GuestLogin(c context.Context, uid string) (*Token, error)   // c 易与 *gin.Context 混淆
   func (s *AuthService) GuestLogin(ctxt context.Context, uid string) (*Token, error) // 拼错
   ```
   Go 社区 `gofmt` / `go vet` / `gopls` 都不强检这条，全靠 code review；ADR 固化后，review 看到不合规签名**直接打回**

10. **`context.Background()` / `context.TODO()` 禁用范围**：
    - ✅ 允许：`cmd/server/main.go` 启动时 `ctx := context.Background()` 作为 server 生命周期根 ctx
    - ✅ 允许：测试里 `ctx := context.Background()` 作为测试 ctx 起点
    - ❌ 禁止：handler / service / repo 内部再起 `context.Background()` —— 会截断上游 cancel 链路
    - ❌ 禁止：ADR-0007 §2.2 明示 handler 用 `c.Request.Context()`，**不**是 `context.Background()`

11. **goroutine 里的 ctx**：本 story 不涉及（节点 1 无 goroutine 级业务）；但 ADR-0007 §7 迁移清单**预留**规则："如果 service 启了 goroutine（Epic 10+ ws 广播），ctx cancel 必须让 goroutine 退出（通过 `select { case <-ctx.Done(): ... }`）"。这是给 Epic 10 / Epic 20 落地时用的

12. **与 AppError 的交互**：`context.Canceled` / `context.DeadlineExceeded` 是 stdlib sentinel error。在 service 层拿到 repo 返回的这俩：
    - **推荐**：原样 `return err` —— 最简单
    - **允许**：`return apperror.Wrap(err, apperror.ErrServiceBusy, "请求已取消")` —— 让 errors.Is(wrapErr, context.Canceled) 仍然 true（因为 apperror.Unwrap 保留 Cause）
    - **禁止**：`return apperror.New(apperror.ErrServiceBusy, "...")` 丢掉 Cause —— 会让 `errors.Is(err, context.Canceled)` 返回 false，监控层区分 "cancel 5xx" vs "真故障 5xx" 就失效了
    - ADR-0007 §6 会写清这三条，Epic 4+ dev 按需选

### 与上游 story 的契约兑现表

| 上游 story | 未竟约定 | 本 story 如何兑现 |
|---|---|---|
| 1.3 Logging 中间件 陷阱 #5 "Story 1.9 追加字段 `ctx_done`" | T2：logging.go 追加 ctx_done 字段；T6：回溯修正 1-3 story 文件的陷阱注释为 "已落地" |
| 1.5 sample service 顶部注释 "ctx 第一参数 —— Story 1.9 即将固化为 ADR 0007，本包先示范该约定" | T1：ADR-0007 §2.1 固化 ctx 签名约定；T4：追加 service_ctx_test.go 补全"sample 保留同步路径骨架，Story 1.9 扩展 ctx cancel 测试" |
| 1.5 Tasks "不在 sample service 里做 ctx cancellation 测试：Story 1.9 AC 明示它会追加一条 ctx cancel 测试到 sample 模板" | T4：新文件 service_ctx_test.go，两条 case（cancel + happy） |
| 1.5 TODO "Story 1.9：在 sample 中追加 ctx cancel 测试 case；输出 ADR 0007-context-propagation.md" | T1 + T4 同时兑现 |
| 1.8 ADR-0006 §1.4 "Story 1.9 context 传播框架依赖 AppError + Wrap + Unwrap" | Dev Notes §12 说明 apperror.Wrap 与 ctx.Err 的交互策略（保留 Cause 让 errors.Is 穿透） |
| ADR-0001 §4 "所有日志通过 slog.InfoContext(ctx, msg, ...)" | T2 logging.go `ctx_done` 追加在 `LogAttrs(ctx, ...)` 同一调用里，沿用 ctx-aware 日志风格 |
| ADR-0001 §8 "Story 1.9：context 传播框架占用 decision 文档编号 0007-context-propagation.md" | T1：编号 0007 严格按 Story 1.1 ADR §8 预留 |

### Lessons Index（与本 story 相关的过去教训）

- `docs/lessons/2026-04-24-sample-service-nil-dto-and-slog-test-group.md` **Lesson 1 "公共 artifact 质量门槛"** —— **直接相关**：sample 是"复制模板"，本 story 扩 ctx cancel 测试时**必须**按"假设 Epic 4+ N 个 service 各自复制"评估：
  - 测试完整度：happy + cancel 两条 case；两者缺一都会让复制 dev 误以为"cancel 测试可以省"
  - 注释完整度：`slowCtxAwareRepo` struct 顶部必须解释"为什么不用 testify/mock"（见 AC4 代码注释）
  - 语义等价：`slowCtxAwareRepo` 的 `select { case <-ctx.Done() ... case <-time.After(delay) ... }` 必须与 Epic 4+ 真 GORM `db.WithContext(ctx).First(...)` 的 cancel 语义等价（两者都：ctx 已 cancel 时**立即**返回 ctx.Err()；ctx 未 cancel 时 wait delay 后返回结果）
- `docs/lessons/2026-04-24-middleware-canonical-decision-key.md` **Lesson 1 / Lesson 2** —— **间接相关**：本 story 的 `ctx_done` 字段是 Logging 独自 derive 的（不走 `c.Keys` 广播），**原因**：`ctx.Err()` 是 Gin ctx 的**原生状态**，不是 ErrorMappingMiddleware / 其它中间件的"推断结果"。两个独立中间件读同一个 `ctx.Err()` **不会**漂移（ctx 本身就是 canonical source）。与 lesson 中 "c.Errors 独立 derive 会漂移" 不是同一形态 —— c.Errors 被 middleware wrap 过（Story 1.8 Add 了 wrap 1009 逻辑），ctx.Err() 没有这种 wrap 层。**不需要**引入 `CtxDoneKey`
- `docs/lessons/2026-04-24-error-envelope-single-producer.md` —— **不直接相关**（本 story 不新增 error envelope 生产者）；但提醒：如果 AC5 Case 2 在 handler 里用 `response.Error(c, ...)` **直接**写 envelope，是反模式（见 lesson）；必须 `c.Error(apperror.Wrap(...))` 走 ErrorMappingMiddleware
- `docs/lessons/2026-04-24-config-path-and-bind-banner.md` **Lesson 2 "声明 vs 现实"** —— **间接相关**：本 story 的 ADR-0007 §2 声明 4 条 ctx 约定，但**代码层面**只有 `logging.go` 的 `ctx_done` 字段是真实装；其余约定（service/repo 签名、WithContext、txCtx）是 Epic 4+ 的**未来承诺**。ADR §7 的 7 条迁移清单就是"防声明 vs 现实分裂"的具体手段 —— Epic 4+ 每个 service 落地时挨条勾，把声明转成现实
- `docs/lessons/2026-04-25-slog-init-before-startup-errors.md` —— **不直接相关**（本 story 不动 slog init）；提醒：Logging 中间件的 `reqLogger.LogAttrs(ctx, ...)` 已经用 ctx-aware 日志，本 story 的 `ctx_done` 追加在同一 LogAttrs 调用里，**不**用裸 `slog.Info(...)` / `slog.Default().Info(...)`

### Git intelligence（最近 6 个 commit）

```
28a0d96 chore(story-1-8): 收官 Story 1.8 + 归档 story 文件
8eca52f chore(lessons): backfill commit hash for 2026-04-24-error-envelope-single-producer
26b5692 fix(review): error envelope 必须经 ErrorMappingMiddleware 单一产出
e6546ec chore(commands): /fix-review 不再在 commit 前询问确认
f06bd99 chore(lessons): backfill commit hash for 2026-04-24-middleware-canonical-decision-key
f85064c fix(review): 中间件之间的 canonical decision 必须走显式 c.Keys 而非各自从 c.Errors 推断
```

最近实装向 commit 是 Story 1.8 的 review fixes（`26b5692` / `f85064c`）。本 story 紧随 1.8 done。

**commit message 风格**：Conventional Commits，中文 subject，scope `story-X-Y` / `server` / `scripts`。
本 story 建议：`feat(server): Epic1/1.9 ctx 传播框架 + logging ctx_done + ADR-0007`
（理由：新增 ADR-0007 + logging.go 字段追加 + 2 测试文件，属新能力建立；scope 用 `server` 因为变更跨 middleware/service/docs，`story-1-9` scope 也可）

### 常见陷阱

1. **`c.Request.Context()` 在 c.Next() 前就读**：陷阱 —— handler 里 ctx 仍是初始状态；只有 handler 返回（panic 或 return）后再读才能拿到 "client 是否已断"的 final state。Logging 必须在 `c.Next()` **之后** 读 `Err()`

2. **handler 里 `select { case <-c.Request.Context().Done(): return }` 的陷阱**：Gin 默认不自动 interrupt handler，handler 必须**主动** select ctx.Done() 才能感知 cancel。空 handler（如 `/ping`）没 select 也没关系（反正没耗时）；耗时 handler（Epic 4+ 真业务）就必须 select。**不在本 story scope**，但 ADR-0007 §7 迁移清单第 6/7 条覆盖

3. **`context.WithTimeout` 后忘记 `defer cancel()`**：
   ```go
   // 错误：ctx 泄漏（即使 timeout 自动触发，cancel func 仍要手动释放资源）
   ctx, _ := context.WithTimeout(context.Background(), 100*time.Millisecond)
   result, err := svc.GetValue(ctx, "x")

   // 正确：
   ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
   defer cancel()
   result, err := svc.GetValue(ctx, "x")
   ```
   AC4 的 case 代码里显式写了 `defer cancel()`；`go vet` 会警告 "the cancel function returned by context.WithCancel should be called"，但容易被忽视

4. **`time.After` 在 select 里的资源泄漏**：
   ```go
   // 问题：ctx 提前 cancel 时 time.After 创建的 timer channel 仍在 GC 前占资源
   select {
   case <-ctx.Done():
       return nil, ctx.Err()
   case <-time.After(r.delay):
       return r.dto, nil
   }
   ```
   测试里 `r.delay = 5*time.Second` 导致 timer channel 要 GC 5 秒后才释放。对 1 条 test case 可忽略；对 N 条并发 case 或长期 server 代码是隐患。**本 story 的 AC4 测试只跑 1 条 + 1 次**，容忍这个泄漏；Epic 10+ 的 ws 广播涉及 select ctx.Done + timer 时建议用 `time.NewTimer` + `timer.Stop()` 替代 `time.After`

5. **`errors.Is(err, context.DeadlineExceeded)` vs `errors.Is(err, context.Canceled)`**：
   - `context.DeadlineExceeded`：`context.WithTimeout` / `context.WithDeadline` 到期触发
   - `context.Canceled`：显式调 `cancel()` / `context.WithCancel` 被 cancel 触发
   - 两者都让 `ctx.Err() != nil`，但**不是同一个** error value
   - AC4 测试用 `context.WithTimeout` → 必须 assert `errors.Is(err, context.DeadlineExceeded)`（不是 Canceled）
   - AC5 Case 1 用 `context.WithCancel` + 立即 `cancel()` → 必须 assert `errors.Is(err, context.Canceled)`（若测试需要区分）
   - ADR-0007 §4 / §6 明示：生产环境 `ctx_done=true` 不区分两者（聚合信号足够），但测试代码**必须**用对应的 sentinel 断言，否则测试真阴性变假阳性

6. **`httptest.NewRequest` 里 `req.WithContext(ctx)` 的陷阱**：
   ```go
   ctx, cancel := context.WithCancel(context.Background())
   cancel() // 立即 cancel

   req := httptest.NewRequest("GET", "/path", nil)
   req = req.WithContext(ctx) // 替换 req.Context() 为已 cancel 的 ctx

   engine.ServeHTTP(w, req)
   ```
   **注意**：`req = req.WithContext(ctx)` 返回**新 request**（不修改原 req）；必须赋值回去。Go net/http 文档明示，容易漏写 `req = ...`

7. **Gin v1.12.0 的 ctx 行为**：Gin 从 v1.9 起默认 `c.Request.Context()` 与 HTTP request 的 ctx 一致；client 断开时 net/http server 会 cancel 该 ctx。**不需要**额外配置。但历史上 Gin < 1.9 有 bug（见 gin-gonic/gin#1847），本项目 pin 的 v1.12.0 已 fix

8. **`c.Context` ≠ `c.Request.Context()`**：Gin 的 `c *gin.Context` **不是** `context.Context` —— `c` 实现了 `context.Context` 接口（有 `Deadline / Done / Err / Value` 方法），但**行为与 HTTP request ctx 不同**（`c.Done()` 返回 nil channel，即永不 cancel）。
   ```go
   // 错误：c 没有 cancel 语义
   svc.DoIt(c, ...)

   // 正确：
   svc.DoIt(c.Request.Context(), ...)
   ```
   ADR-0007 §2.2 必须明示 "handler 从 c.Request.Context() 取 ctx，**不**从 c 直接传"

9. **测试里的 `<-time.After(5*time.Second)` block 住整个 test**：
   - AC4 测试**有可能**因 bug（如 `slowCtxAwareRepo` 的 select 写错）真的 sleep 5 秒 → `go test` 看起来 hang
   - 防御：在 top-level `t.Run` 外用 `time.AfterFunc(10*time.Second, func() { panic("test hung") })` 作为兜底超时；或在 `go.mod` 或 build.sh 里 `go test -timeout 30s ./...`（但 build.sh 目前默认超时足够）
   - **本 story 不强求**设 test-level timeout，依赖 go test 默认 10 分钟 timeout 兜底；但 code review 时如果发现测试 hang > 1 分钟，怀疑 `slowCtxAwareRepo.select` 写错

10. **`slog.Bool("ctx_done", true)` JSON 输出**：`slog.JSONHandler` 把 `slog.Bool` 直接序列化为 JSON boolean（`true` / `false`，无引号）。测试解析 log 时 `m["ctx_done"].(bool)` **必须** comma-ok：
    ```go
    got, ok := m["ctx_done"].(bool)
    if !ok {
        t.Fatalf("ctx_done not a bool: %T = %v", m["ctx_done"], m["ctx_done"])
    }
    if !got { t.Errorf("ctx_done = false, want true") }
    ```
    若直接 `m["ctx_done"] == true` 比较：interface{} 相等性 — 值相等+类型相等才 true，Go 原生支持；但 comma-ok 更健壮

11. **ADR-0007 §3 协作图用 mermaid 还是 ASCII？** —— 两者都允许；本 story 推荐 ASCII（简单，不依赖 markdown 渲染器）。若将来文档站需要 mermaid，单独 PR 改即可

12. **CLAUDE.md §"工作纪律" 的改动是否纳入本 story commit**：AC7 给了开放选项。**推荐**纳入 —— 让本 story 的 6 条 red lines 落地完整（包括规则同步）。若 dev 决定不改 CLAUDE.md，需要在 Completion Notes 里记录"CLAUDE.md 未改；ctx 约定以 ADR-0007 为唯一权威"，不然 future dev 可能只看 CLAUDE.md 不看 ADR

### 与节点 1 之后业务 epic 的衔接（informational，非本 story scope）

Epic 4 (auth) / Epic 7 (step) / Epic 11 (room) / Epic 20 (chest) / Epic 26 (cosmetic) / Epic 32 (compose) 的 service + repo + tx 落地时，严格按 ADR-0007 §7 七条迁移清单检查。典型新 service 模板（AC1 §5 引用的 "sample ctx cancel 测试范例" 的 Epic 4 对应版）：

```go
// repo 层（Epic 4 user_repo.go 示例）
func (r *UserRepo) FindByGuestUID(ctx context.Context, uid string) (*User, error) {
    var user User
    // 必须 WithContext(ctx)：GORM 底层会把 ctx 传给 database/sql driver，
    // driver 感知 ctx cancel 时立即返回 context.Canceled / context.DeadlineExceeded
    if err := r.db.WithContext(ctx).Where("guest_uid = ?", uid).First(&user).Error; err != nil {
        return nil, err
    }
    return &user, nil
}

// service 层（Epic 4 auth_service.go 示例）
func (s *AuthService) GuestLogin(ctx context.Context, uid string) (*Token, error) {
    user, err := s.userRepo.FindByGuestUID(ctx, uid)
    if err != nil {
        // ctx.Err() 原样向上 / 或用 apperror.Wrap 保留 Cause（见 Dev Notes §12）
        if stderrors.Is(err, gorm.ErrRecordNotFound) {
            return nil, apperror.Wrap(err, apperror.ErrGuestAccountNotFound, "游客账号不存在")
        }
        return nil, apperror.Wrap(err, apperror.ErrServiceBusy, "服务繁忙")
    }
    return token, nil
}

// tx 模式（Epic 4 Story 4-6 游客登录初始化事务落地示例）
func (s *AuthService) GuestInitialize(ctx context.Context, uid string) (*Token, error) {
    err := s.txManager.WithTx(ctx, func(txCtx context.Context) error {
        // txCtx ≠ 外层 ctx：GORM 把外层 ctx 包了一层 tx-aware 的 txCtx
        // 所有 fn 内 repo 调用必须用 txCtx（不是外层 ctx）
        user, err := s.userRepo.CreateUser(txCtx, uid)
        if err != nil { return err }
        _, err = s.petRepo.CreateDefaultPet(txCtx, user.ID)
        if err != nil { return err }
        // ... 其他 repo 调用都用 txCtx
        return nil
    })
    if err != nil {
        return nil, apperror.Wrap(err, apperror.ErrServiceBusy, "服务繁忙")
    }
    return token, nil
}

// service 单测必备：ctx cancel case（复用本 story 的 slowCtxAwareRepo 模式）
func TestAuthService_GuestLogin_CtxCancel(t *testing.T) {
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

这是 ADR-0007 §7 迁移清单的落地模板。本 story 的 sample ctx 测试是"模板的模板"—— Epic 4+ dev 复制本 story 的 `slowCtxAwareRepo` 模式到各自 domain 即可。

### Project Structure Notes

**新增文件**（3 个）：
- `_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md` — ADR-0007（8 章节）
- `server/internal/service/sample/service_ctx_test.go` — sample ctx cancel 测试（2 case）
- `server/internal/app/http/middleware/ctx_propagation_integration_test.go` — handler/middleware 链路 ctx 传播集成测试（1-2 case）

**修改文件**（5-6 个）：
- `server/internal/app/http/middleware/logging.go` — 追加 `ctx_done` 字段；顶部注释扩段落
- `server/internal/app/http/middleware/logging_test.go` — 追加 ≥2 case（no ctx_done on success + ctx_done when canceled）；更新 `TestLogging_HappyPath_HasSixFields` forbidden 列表加 `ctx_done`
- `_bmad-output/implementation-artifacts/1-3-中间件-request_id-recover-logging.md` — 陷阱 #5 / References 回溯修正为"已落地"
- `_bmad-output/implementation-artifacts/sprint-status.yaml` — 1-9 状态流转
- `CLAUDE.md` §"工作纪律"（可选） — 追加"ctx 必传"一行

**删除文件**：无

**不动文件**（明确范围红线）：
- `server/internal/service/sample/service.go` — **不**加 ctx.Err() 预检（Dev Notes §1-2）
- `server/internal/service/sample/service_test.go` — 现有 5 条 case 不改；ctx cancel case 单独放新文件
- `server/internal/app/http/middleware/error_mapping.go` / `recover.go` / `request_id.go` — 已正确用 `c.Request.Context()`
- `server/internal/app/bootstrap/router.go` — 中间件顺序不变
- `server/internal/pkg/errors/*` — 不改 AppError（Story 1.8 已定稿）
- `server/internal/pkg/testing/helpers.go` — 不加 ctx helper（Dev Notes 范围红线第 2 条）
- `go.mod` / `go.sum` — 不引入新依赖

**对目录结构的影响**：无新增目录；在现有 `server/internal/service/sample/` 与 `server/internal/app/http/middleware/` 各加 1 个测试文件；`_bmad-output/implementation-artifacts/decisions/` 加 1 个 ADR

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.9] — 本 story 原始 AC（ADR-0007 四条约定 + logging ctx_done + sample ctx cancel 测试 + ≥3 单元测试 case）
- [Source: _bmad-output/planning-artifacts/epics.md#Epic-1] — Epic 1 scope "ctx 传播框架" 明示在范围内
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#5.1-5.3] — handler / service / repo 分层职责（ctx 传递的分层契约）
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#10] — 事务边界 + `txManager.WithTx(ctx, fn)` 模式
- [Source: docs/宠物互动App_V1接口设计.md#2.4] — 统一响应结构（ctx cancel 时 ErrorMappingMiddleware 兜底 1009）
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#4] — logger 字段表（`error_code` 字段先例，`ctx_done` 字段沿用同惯例）
- [Source: _bmad-output/implementation-artifacts/decisions/0001-test-stack.md#8] — ADR 编号预留表（0007 归本 story）
- [Source: _bmad-output/implementation-artifacts/decisions/0006-error-handling.md#1.4] — Story 1.8 说明"Story 1.9 立即下游"
- [Source: _bmad-output/implementation-artifacts/decisions/0006-error-handling.md#8.2] — 业务 service 落地标准模板（ctx-first 签名的先例）
- [Source: _bmad-output/implementation-artifacts/1-3-中间件-request_id-recover-logging.md#陷阱] — 陷阱 #5 "Story 1.9 追加字段 `ctx_done`"（本 story 兑现）
- [Source: _bmad-output/implementation-artifacts/1-5-测试基础设施搭建.md#TODO] — "Story 1.9：在 sample 中追加 ctx cancel 测试 case；输出 ADR 0007-context-propagation.md"
- [Source: _bmad-output/implementation-artifacts/1-5-测试基础设施搭建.md#范围红线] — "不在 sample service 里做 ctx cancellation 测试：Story 1.9 AC 明示它会追加一条 ctx cancel 测试到 sample 模板"
- [Source: _bmad-output/implementation-artifacts/1-8-apperror-类型-错误三层映射框架.md#下游story立即依赖] — "Story 1.9 context 传播框架的 AC 第一行就是 Given Story 1.8 AppError 框架已就绪"
- [Source: server/internal/app/http/middleware/logging.go:41-79] — 现有 Logging 实装；T2 追加 `ctx_done` 字段的具体插入点
- [Source: server/internal/app/http/middleware/logging_test.go:58-98] — `TestLogging_HappyPath_HasSixFields` 现有 forbidden 列表；T3.3 追加 `ctx_done`
- [Source: server/internal/service/sample/service.go:16] — "所有导出方法第一个参数是 ctx context.Context —— Story 1.9 即将固化为 ADR 0007"（本 story 兑现）
- [Source: server/internal/service/sample/service.go:70] — `s.repo.FindByID(ctx, id)` 现有 ctx 下传示例；AC4 `slowCtxAwareRepo` 验证该链路
- [Source: docs/lessons/2026-04-24-sample-service-nil-dto-and-slog-test-group.md#Lesson-1] — 公共 artifact 质量门槛（sample 模板扩 ctx cancel 测试时按 N 个下游复用标准评估）
- [Source: docs/lessons/2026-04-24-middleware-canonical-decision-key.md] — canonical decision key 模式（说明为什么 `ctx_done` 不需要走 c.Keys，见 Dev Notes §Lessons Index 第 2 条）
- [Source: docs/lessons/2026-04-24-error-envelope-single-producer.md] — ErrorMappingMiddleware 是 envelope 单一生产者（AC5 集成测试 handler 用 c.Error 而非 response.Error 的契约）
- [Source: CLAUDE.md#工作纪律] — "节点顺序不可乱跳"（本 story 是节点 1 第九条；前序 1.1-1.8 已 done；1.10 是纯文档）
- [Source: CLAUDE.md#Tech-Stack] — Gin / Go / stdlib context 技术栈

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- **T7.2 可选手工验证跳过**：AC5 Case 2 `TestCtxPropagation_SlowHandlerCanceledByClientDisconnect` 已用 `httptest.NewServer` 跑真 HTTP roundtrip 覆盖等价路径（client 侧 ctx timeout + server 侧 handler select ctx.Done()，两侧 elapsed 均 < 500ms）。手工 curl + binary 启动方案在 CI / 无人值守场景下不可重复，本 story 用集成测试替代，与 Story 1.4 review-driven `httptest.NewServer` 测试模式一致。
- **T8.5 CLAUDE.md 采纳**：AC7 给了"dev 自行判断是否纳入 CLAUDE.md"的开放选项；采纳理由是让 ctx 约定在两处权威文档同时可见（CLAUDE.md §"工作纪律" 是新 session 起手读的文件；ADR-0007 是技术细节权威）。若只写 ADR，未来 Claude 新 session 可能只读 CLAUDE.md 漏掉 ctx 约定。
- **未跑 `--race --test`**：Story 1.5 AC7 + Story 1.7 AC4 / T5.3 + Story 1.8 T10.1 已多次确认 Windows 本机 TSAN 内存分配限制（`ERROR: ThreadSanitizer failed to allocate ... error code: 87`）让 race 测试无法在本机跑；归 CI Linux runner 执行。本 story 不重复跑同样会失败的路径。
- **`slowCtxAwareRepo` 的 `id string` 参数用 `_` 丢弃**：原接口签名是 `FindByID(ctx, id)`；本 story 的 slowCtxAwareRepo 作为 fake，`id` 值不影响测试语义（正常路径永远返回预设 dto；cancel 路径永远返回 ctx.Err()），故用 `_` 丢弃。若未来扩展 case 需按 id 分支，改回 `id`。

### Completion Notes List

**实现摘要**

- **T1 ADR-0007**：8 章节落地 —— Context / 四条 ctx 约定（签名 / Handler / Repo / Tx）/ cancel 协作图 / `ctx_done` 字段契约 / sample 测试范例 / 业务错误码对接策略 / Epic 4+ 迁移清单 7 条（C1-C7）/ Change Log。ASCII 流程图清晰展示 "client 断开 → Gin ctx cancel → handler → service → repo → MySQL driver" 完整路径。
- **T2 logging.go**：`c.Next()` 之后读 `c.Request.Context().Err()`，非 nil 时追加 `ctx_done=true`（bool）到 http_request 日志；缺省省略字段（与 error_code 成功路径惯例一致）。顶部注释段落追加 `ctx_done` 字段语义段落（含为什么不走 c.Keys 广播的理由：ctx 本身是 canonical source，对比 error_code 需广播因为 middleware 做过 wrap 推断）。
- **T3 logging_test.go**：新增 3 条 case —— `TestLogging_NoCtxDoneOnSuccess`（正常请求不含字段）/ `TestLogging_CtxDoneWhenClientDisconnects`（WithCancel + 立即 cancel）/ `TestLogging_CtxDoneOnDeadlineExceeded`（WithTimeout 0ns 即到期，对称验证）。更新 `TestLogging_HappyPath_HasSixFields` forbidden 列表加 `ctx_done`。
- **T4 service_ctx_test.go**：新文件 2 case —— 核心 `TestSampleService_CtxCancelPropagates`（100ms ctx timeout + 5s slow repo → elapsed < 500ms + errors.Is DeadlineExceeded；elapsed < 50ms 软警告防假阳）；辅助 `TestSampleService_CtxNormalPath_ReturnsNormally`（防核心 case 假阴：证明 slowCtxAwareRepo 在 ctx 正常时确实 sleep 到 delay 结束）。`slowCtxAwareRepo` 用 struct + select 而非 testify/mock（mock 不感知 ctx channel 语义）。
- **T5 ctx_propagation_integration_test.go**：新文件 2 case —— `TestCtxPropagation_HandlerSeesCancelViaCRequestContext`（in-process req.WithContext(canceledCtx) + engine.ServeHTTP，断言 handler 看到 ctx.Err() 非 nil + log ctx_done=true + elapsed < 200ms）；`TestCtxPropagation_SlowHandlerCanceledByClientDisconnect`（真 httptest.NewServer + http.Client with 100ms ctx timeout + server handler select ctx.Done() vs time.After(5s)，断言两侧 elapsed < 500ms，证明 ctx cancel 跨 TCP 传播）。
- **T6 回溯修正 1-3 story**：陷阱 #5 / TODO 列表 / References 三处 "Story 1.9 会 revisit" 改为 "已于 Story 1.9 落地"；正文 AC / Tasks / Dev Agent Record / File List 保持不动（已 done 的 story 正文冻结）。
- **T8.5 CLAUDE.md**：§"工作纪律" 新增 "ctx 必传" 一行，含 4 条要点（第一参数 ctx / handler 用 c.Request.Context() / repo 用 WithContext / tx fn 用 txCtx）+ 引用 ADR-0007。

**契约兑现**

| 上游约定 | 兑现位置 |
|---|---|
| Story 1.3 陷阱 #5 / TODO / References "Story 1.9 revisit" | T2：logging.go 追加 ctx_done；T6：回溯 1-3 story 三处注释改为 "已落地" |
| Story 1.5 sample 顶部注释 "Story 1.9 即将固化为 ADR 0007" | T1：ADR-0007 §2.1 固化 ctx 签名约定 |
| Story 1.5 Tasks "Story 1.9 追加一条 ctx cancel 测试到 sample 模板" | T4：新文件 service_ctx_test.go 两条 case |
| Story 1.5 TODO "Story 1.9：sample ctx cancel 测试 + ADR 0007" | T1 + T4 同时兑现 |
| Story 1.8 ADR-0006 §1.4 "Story 1.9 立即下游依赖 AppError + Wrap" | ADR-0007 §6 说明 apperror.Wrap 与 ctx.Err 的交互（保留 Cause 让 errors.Is 穿透） |
| ADR-0001 §4 "error_code 字段先例（缺省即 false）" | T2：ctx_done 沿用同惯例（缺省省略字段） |
| ADR-0001 §8 "Story 1.9 占用 0007-context-propagation.md" | T1：编号 0007 严格按预留 |

**测试结果摘要**

```
$ bash scripts/build.sh --test
=== go vet ===

=== go build (commit=28a0d96, builtAt=2026-04-24T14:32:38Z) ===
OK: binary at build/catserver.exe

=== go test ===
?   	github.com/huing/cat/server/cmd/server	[no test files]
ok  	github.com/huing/cat/server/internal/app/bootstrap	0.904s
ok  	github.com/huing/cat/server/internal/app/http/devtools	1.206s
ok  	github.com/huing/cat/server/internal/app/http/handler	0.151s
ok  	github.com/huing/cat/server/internal/app/http/middleware	0.272s
?   	github.com/huing/cat/server/internal/buildinfo	[no test files]
ok  	github.com/huing/cat/server/internal/infra/config	0.316s
ok  	github.com/huing/cat/server/internal/infra/logger	0.326s
ok  	github.com/huing/cat/server/internal/infra/metrics	0.604s
ok  	github.com/huing/cat/server/internal/pkg/errors	0.378s
?   	github.com/huing/cat/server/internal/pkg/response	[no test files]
ok  	github.com/huing/cat/server/internal/pkg/testing	0.478s
ok  	github.com/huing/cat/server/internal/pkg/testing/slogtest	0.392s
ok  	github.com/huing/cat/server/internal/service/sample	0.495s
OK: all tests passed

BUILD SUCCESS
```

**新增 case 独立验证**：

```
$ go test -v -run "TestSampleService_Ctx" ./internal/service/sample/
=== RUN   TestSampleService_CtxCancelPropagates
--- PASS: TestSampleService_CtxCancelPropagates (0.10s)
=== RUN   TestSampleService_CtxNormalPath_ReturnsNormally
--- PASS: TestSampleService_CtxNormalPath_ReturnsNormally (0.01s)
PASS
ok   github.com/huing/cat/server/internal/service/sample	0.469s

$ go test -v -run "TestLogging_NoCtxDoneOnSuccess|TestLogging_CtxDoneWhenClientDisconnects|TestLogging_CtxDoneOnDeadlineExceeded|TestCtxPropagation_" ./internal/app/http/middleware/
=== RUN   TestLogging_NoCtxDoneOnSuccess
--- PASS: TestLogging_NoCtxDoneOnSuccess (0.01s)
=== RUN   TestLogging_CtxDoneWhenClientDisconnects
--- PASS: TestLogging_CtxDoneWhenClientDisconnects (0.00s)
=== RUN   TestLogging_CtxDoneOnDeadlineExceeded
--- PASS: TestLogging_CtxDoneOnDeadlineExceeded (0.00s)
=== RUN   TestCtxPropagation_HandlerSeesCancelViaCRequestContext
--- PASS: TestCtxPropagation_HandlerSeesCancelViaCRequestContext (0.00s)
=== RUN   TestCtxPropagation_SlowHandlerCanceledByClientDisconnect
--- PASS: TestCtxPropagation_SlowHandlerCanceledByClientDisconnect (0.10s)
PASS
ok   github.com/huing/cat/server/internal/app/http/middleware	0.238s
```

5 条新 case 全绿；`TestSampleService_CtxCancelPropagates` 和 `TestCtxPropagation_SlowHandlerCanceledByClientDisconnect` 的 0.10s 耗时符合 100ms ctx timeout 预期（证明 cancel 确实触发，而非"假通过"的 0 耗时）。

**后续延伸**（非本 story scope，留记录）

- Story 1.10 server README 需在"依赖 & 配置"或"目录结构"章节引用 ADR-0007，告知新 dev "ctx 约定以 ADR-0007 为权威"
- Epic 4 Story 4-2 落地 GORM 时，所有 repo 方法**必须**挨条勾 ADR-0007 §7 迁移清单（C1-C7）
- Epic 4 Story 4-6 游客登录初始化事务是 `txManager.WithTx(ctx, func(txCtx context.Context) error { ... })` 模式的首个真实实装；必须对照 ADR-0007 §2.4 + §5.4 迁移模板核对 txCtx 下钻到每个 repo 调用
- Epic 10+ ws 广播涉及 goroutine，按 §7 C7 "ctx cancel 必须让 goroutine 退出" 设计 select pattern

### File List

**新增**
- `_bmad-output/implementation-artifacts/decisions/0007-context-propagation.md`（ADR-0007，8 章节）
- `server/internal/service/sample/service_ctx_test.go`（slowCtxAwareRepo + 2 case：Cancel + NormalPath）
- `server/internal/app/http/middleware/ctx_propagation_integration_test.go`（2 case：in-process + 真 HTTP roundtrip + parseLastLogLineFromBuf helper）

**修改**
- `server/internal/app/http/middleware/logging.go`（c.Next() 之后读 ctx.Err() 追加 ctx_done=true；顶部注释补 ctx_done 字段语义段落）
- `server/internal/app/http/middleware/logging_test.go`（新增 3 case + forbidden 列表加 ctx_done + context import）
- `_bmad-output/implementation-artifacts/1-3-中间件-request_id-recover-logging.md`（陷阱 #5 / TODO / References 三处 "Story 1.9 revisit" 改为 "已于 Story 1.9 落地"）
- `CLAUDE.md`（§工作纪律 追加 "ctx 必传" 一行，引用 ADR-0007）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（1-9 状态 backlog → ready-for-dev → in-progress → review；last_updated 时间戳）
- `_bmad-output/implementation-artifacts/1-9-go-context-传播框架-cancellation-验证.md`（本 story 文件：Tasks 勾选 / Dev Agent Record 填充 / Status 流转 → review）

**删除**
- 无

## Change Log

| 日期 | 版本 | 描述 | 作者 |
|---|---|---|---|
| 2026-04-24 | 0.1 | 初稿（ready-for-dev） | SM |
| 2026-04-24 | 1.0 | 实装完成：ADR-0007 8 章节落地；logging.go 追加 ctx_done 字段；sample + middleware 共 5 条新 ctx 相关 case 全绿；回溯修正 1-3 story 三处注释；CLAUDE.md 追加 ctx 必传一行；状态流转 review | Dev |
