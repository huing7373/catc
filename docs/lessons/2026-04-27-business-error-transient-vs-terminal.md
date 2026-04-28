---
date: 2026-04-27
source_review: codex review (epic-loop round 5) — /tmp/epic-loop-review-5-5-r5.md
story: 5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据
commit: <pending>
lesson_count: 3
---

# Review Lessons — 2026-04-27 — Business 错误必须区分 transient/terminal & alert OK 按钮必须有真实动作 & 4 轮 fix-review 单点 patch 反模式

## 背景

Story 5.5 codex review round 5。本 story 已经历 4 轮 fix-review，每一轮都解决了 review 提出的问题但**引入新 regression**：

- round 1：bootstrap LoadHome 失败弹 developer 串 → 改成走 mapper 拿用户文案
- round 2：mapper 输出 `userFacingMessage: String` 字段，状态机仍走 RetryView，把 `.unauthorized`/`.decoding` 等 unrecoverable 错误塞进 retry → 改成传 `presentation: ErrorPresentation` 三态
- round 3：retry-after-failure 死循环 + ping 探针超 HTTP 预算 → 删 ping，retry 时 fail-safe 重跑 guest-login
- round 4：guest-login 失败也走 mapper；alert dismiss 触发 retry 死循环 → 改成 `onDismiss = { /* no-op */ }`

round 5 review 发现 round 4 留下的两条 regression：

1. mapper 把所有 business 一律映射 `.alert`，但 1009 (服务繁忙) 是 transient 类，bootstrap 路径下应该可重试 → 卡在 alert 而非 retry
2. AlertOverlayView 唯一 OK 按钮 `知道了` 调 `onDismiss` → no-op → state 不变 → AlertOverlayView 仍渲染 → 永久卡死

两条 finding **互相牵扯** —— 单独修任一会引入新 regression（修 1 不修 2 → permanent error 仍卡死；修 2 不修 1 → 1009 仍走 alert，exit App 太重）。本轮强制综合修复。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | bootstrap `/home` 1009 失败必须保留重试入口 | P1 | error-handling | fix | `iphone/PetApp/Shared/ErrorHandling/AppErrorMapper.swift`, `iphone/PetApp/App/RootView.swift` |
| 2 | bootstrap alert 的 OK 按钮不能 no-op | P2 | ui / error-handling | fix | `iphone/PetApp/App/RootView.swift` |
| 3 | meta：5-5 4 轮 fix-review 都引入新 regression 的根因 | meta | architecture | meta | — |

## Lesson 1: business 错误必须区分 transient（client 重试可自愈）vs terminal（必须重启 App）

- **Severity**: P1
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/Shared/ErrorHandling/AppErrorMapper.swift:55-86`

### 症状（Symptom）

冷启动 `/home` 返 `APIError.business(code: 1009, ...)` 时，bootstrap 路径下用户被困在 AlertOverlayView，没有 RetryView 入口。1009 是"服务繁忙，请稍后重试"，文案承诺可重试但 UI 不给重试按钮——用户唯一出路是 force-quit。

### 根因（Root cause）

`AppErrorMapper.presentation(for:)` 早期把 `.business(code, message, _)` 一律映射成 `.alert(title:, message:)`，没区分业务码语义。但 V1 §3 错误码字典里**两类共存**：

- **terminal**：`.business(2001 账号不存在)` / `.business(5002 道具不属于你)` / `.business(6001 房间不存在)` —— client 重试不会改变结果，必须由 server 状态变化或用户切换上下文才能恢复
- **transient**：`.business(1005 频繁)` / `.business(1007 数据冲突)` / `.business(1008 重复)` / `.business(1009 服务繁忙)` —— client 间隔几秒重试大概率自愈（server 容量恢复 / 乐观锁 race 错峰 / 幂等键过期）

把这两类塞进同一个 `.alert` 决策意味着：
- transient 错误失去重试入口（用户只能 force-quit + 冷启动）
- alert 的"重启 App"语义被稀释（既包含真正终端错误，也包含可重试错误）

更深层根因：`ErrorPresentation` 三态（`.alert` / `.retry` / `.toast`）的语义没在文档中钉死，每次 fix 都按当下场景理解，导致 mapper 输出与 UI 层（AlertOverlayView OK 按钮、RetryView 重试按钮）的预期漂移。

### 修复（Fix）

1. 在 `AppErrorMapper` 加 `transientBusinessCodes: Set<Int> = [1005, 1007, 1008, 1009]` 显式枚举。
2. `presentation(for:)` 内 `.business` 分支：transient code → `.retry(message:)`；其他 → `.alert(title:, message:)`。
3. 文档化 `.alert` = terminal（重启 App）/ `.retry` = transient（in-app 重试）/ `.toast` = info-level。
4. 同步 mapper 的 V1 错误码字典文案保持不变（`localizedMessage(forBusinessCode:fallback:)` 不动）；只改 presentation case 选择。
5. 测试：
   - 加 4 个 transient code → `.retry` 单测（1005/1007/1008/1009）
   - 加 1 个 permanent code 反例 → `.alert`（1004 权限不足）
   - 修复 `HomeViewModelLoadHomeTests.testLoadHomeBusinessFailureUpdatesStateAndPresentsAlert` 改用 4002（permanent），新增 `case#2c` 覆盖 transient → retry
   - 修复 `AppLaunchStateMachineTests.testBootstrapWithMappedBusinessErrorRoutesToAlertPresentation` 改名为 `...RoutesToRetryPresentation`，并新增 `case#11b` 覆盖 permanent business → alert
   - 修复 `RootViewWireTests.testBootstrapClosureWrapsGuestLoginFailureViaAppErrorMapper` 把断言从 `.alert` 改 `.retry`

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**新增/修改 mapper 把 server error code 映射成 UI presentation 时**，**必须**先按"client 重试是否可能自愈"二分错误码，transient 类（限流 / 容量 / 乐观锁 / 幂等键冲突）走 `.retry`，terminal 类（鉴权 / 资源不存在 / 业务规则不满足）走 `.alert`。
>
> **展开**：
> - **`.alert` 语义钉死 = "用户必须重启 App 才能恢复"**：UI 层（AlertOverlayView OK 按钮在 bootstrap 路径调 `exit(0)`）已按此语义实现。把可重试错误塞进 `.alert` = 让用户被迫 force-quit。
> - **`.retry` 语义钉死 = "用户可在 App 内点重试自愈"**：UI 层（RetryView）有重试按钮，会触发 closure 重跑。把 unrecoverable 错误塞进 `.retry` = 让用户卡在 retry loop（每次重试同样失败）。
> - **`.toast` 语义钉死 = "info-level 短提示，不阻塞"**：mapper 当前不主动派 toast；ViewModel 自定义场景才用。
> - **改 mapper 文案 / 增删 code 时必须搜全所有 mapping 测试**：`grep -r "code: <N>" PetAppTests` + 检查每条对应的 presentation 断言。本次回归的 root cause 之一就是没在 round 4 fix 时同步审计 `HomeViewModelLoadHomeTests` / `AppLaunchStateMachineTests` 里硬编码 1009 → .alert 的旧断言。
> - **反例**：`presentation(for:) { case .business: return .alert(...) }` —— "所有 business 一律 alert" 是反模式，必须按 code 语义二分。同样反例：mapper 加新 code 时只改 `localizedMessage` 字典不审 `presentation` 分支选择。

## Lesson 2: alert 类 modal 的"OK / 知道了"按钮必须有真实动作，no-op 是死锁

- **Severity**: P2
- **Category**: ui / error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift:468-486`

### 症状（Symptom）

bootstrap 路径下任一 mapper 钦定为 `.alert` 的错误（`.unauthorized` exhausted / `.decoding` schema 错 / `.missingCredentials`）触发后，AlertOverlayView 显示，用户点 `知道了` → onDismiss closure 是 `{ /* no-op */ }` → state 不变（仍 `.needsAuth(presentation: .alert)`）→ AlertOverlayView 仍渲染 → 用户唯一出路是 force-quit。OK 按钮变成纯装饰品，且与 alert 文案 `请重启应用` 矛盾（按钮无法实现"重启"动作）。

### 根因（Root cause）

round 3 fix 让 alert OK → retry() 触发 → 死循环（每次重试同失败）→ round 4 fix 改 onDismiss 为 no-op，但只解决了"不要 retry"，没解决"OK 按钮要做什么"。SwiftUI 的 modal overlay UI 一旦渲染，**只能由 state 变化触发卸载**——onDismiss no-op 等于"按钮永远不解除 modal"。

更深层根因：`.alert` 在 `ErrorPresentation` 里被设计成"通用 alert"（同样供非 bootstrap 路径用，由 ErrorPresenter 调 `presenter.dismiss()` 卸载）。但 bootstrap 路径下没有 ErrorPresenter 介入，AppLaunchStateMachine 的 state 也不能"从 alert 转回 ready"（用户没解决根本问题）。这种"两条路径共用一个 view，但 dismiss 语义不同"的设计漏洞导致每次修一边漏另一边。

### 修复（Fix）

bootstrap 路径下 AlertOverlayView 的 onDismiss 改为 `{ exit(0) }`：

- 与 mapper 钦定的"请重启应用 / 请重新启动应用"文案对齐：按钮真的实现"重启" 动作（exit → 用户重新冷启动 App）
- 不再 no-op：解决死锁（用户点 OK 有动作发生）
- 不再隐式 retry：解决 round 3 死循环
- exit(0) 是 unrecoverable error 的合理 UX：与系统级 fatal alert 行为对齐

`.toast` 兜底分支（理论上 mapper 不派 toast 到 bootstrap）同样改 `exit(0)`：mapper 配置异常算 unrecoverable，让用户重启。

非 bootstrap 路径的 AlertOverlayView（由 `ErrorPresenter` 调用，见 `ErrorPresentationHostModifier.swift:52`）保持 `onDismiss: { presenter.dismiss() }` 不动 —— 那条路径有 ErrorPresenter 管理 state 卸载，不能 exit App。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**给 modal overlay UI（alert / sheet / fullScreenCover）写 dismiss 闭包时**，**必须**保证闭包要么改 state 让 view 卸载，要么执行外部副作用（如 exit / 跳页），**禁止** no-op —— 否则按钮等于死锁。
>
> **展开**：
> - **modal 的本质是"一旦显示就独占整个交互层"**：用户唯一可点的就是 modal 自己的按钮。按钮 onDismiss no-op = view 永不卸载 = force-quit。
> - **dismiss 闭包的"该做什么"由 modal 的语义类决定**：transient（请重试）→ 必须能触发重试或清 state；terminal（请重启）→ 必须 exit 或跳到引导页；info（仅通知）→ 必须改 state 让 modal 自卸载。
> - **如果同一个 view 在多条路径里被复用**（如 AlertOverlayView 在 bootstrap + ErrorPresenter 两条路径），**dismiss 闭包必须由 caller 注入**而不是 view 内部硬编码 —— caller 负责按路径语义决定动作。本次 fix 就是 caller 显式传 `exit(0)`（bootstrap）vs `presenter.dismiss()`（ErrorPresenter）。
> - **反例**：`onDismiss: { /* TODO 或 no-op 或仅注释为何不做事 */ }` —— 这种"故意空"几乎一定是死锁。如果想让 modal 保留显示，应该在 view 层面隐藏按钮（如 `Button` 不渲染）而不是按钮存在但按了不动。
> - **测试反例**：snapshot 测试或 unit 测试不验证 onDismiss 行为是常态，导致 no-op closure 通过 CI。规则：所有 modal 类 UI 的 dismiss 闭包必须有手动测试（即"在模拟器里点一下 OK 看会发生什么"）作为最后一道关。

## Lesson 3 (meta): epic 4 轮 fix-review 都引入新 regression 的根因 + 综合修复策略

- **Severity**: meta
- **Category**: architecture / process
- **分诊**: meta
- **位置**: 跨 review 流程，针对 Story 5.5 整段 fix history

### 症状（Symptom）

Story 5.5 经历 4 轮 codex review fix（commits ac03578 → b39e7a5 → e32184f → 5dcfa4b），每一轮都成功解决 review 提出的 finding，但每一轮都**引入新 regression**让下一轮 review 抓到。round 5 看到 round 4 留下的两条 finding：

- round 4 P1 fix：guest-login 失败也走 mapper（之前只 LoadHome 走 mapper）—— **副作用**：1009 等 transient 业务码现在被 mapper 派成 `.alert`，进入 AlertOverlayView 死路
- round 4 P2 fix：alert dismiss 改 no-op 防 retry 死循环 —— **副作用**：alert OK 按钮永久死锁

每一轮都是"单点 patch + 局部测试通过"，没做"整条 error 路径行为审计"。

### 根因（Root cause）

5-5 涉及的 error path 拓扑横跨 5 个组件：

```
APIClient (server response)
  → APIError (4 cases)
  → AppErrorMapper.presentation(for:)
    → ErrorPresentation (3 cases)
      → BootstrapMappedError 包装
        → AppLaunchStateMachine.state = .needsAuth(presentation:)
          → RootView LaunchedContentView.needsAuthContent(for:)
            → RetryView / AlertOverlayView / ToastView (UI)
              → onDismiss / onRetry closure
                → exit / retry() / no-op
```

每条 error 类型（`.business(code)` × 9 种 / `.unauthorized` / `.network` / `.decoding` / `.missingCredentials`）经过这条 pipeline 都会得到一个 (presentation, UI, button-action) 三元组。**任一层改动都可能让某条 error type 的最终行为反转**：

- mapper 改 `.business` → `.alert`：影响所有 `.business` 类的 UI（包括应该 retry 的 transient 类）
- AlertOverlayView onDismiss 改 retry：影响所有 mapper 派 `.alert` 的错误（包括应该 exit 的 terminal 类）
- AlertOverlayView onDismiss 改 no-op：影响所有 mapper 派 `.alert` 的错误（包括应该有出口的 transient 类）

单点 patch 思维（"只修当前 finding 提到的那一条"）必然漏掉横向的 error path 矩阵。

### 修复（Fix）

引入 **error path 行为表**作为 mapping 改动的强制 review checkpoint。改 mapper / state-machine / UI dispatch 任一层时必须输出/更新如下表：

| error 类型 | mapper 输出 | bootstrap UI | bootstrap onDismiss | 非 bootstrap UI | 非 bootstrap onDismiss |
|---|---|---|---|---|---|
| `.business(1005/1007/1008/1009)` (transient) | `.retry(localized)` | RetryView | retry() 重跑 closure | RetryView | presenter.dismiss(triggerOnRetry: true) |
| `.business(其他 permanent)` | `.alert(localized)` | AlertOverlayView | exit(0) | AlertOverlayView | presenter.dismiss() |
| `.unauthorized` (exhausted) | `.alert("登录失败，请重新启动应用")` | AlertOverlayView | exit(0) | AlertOverlayView | presenter.dismiss() |
| `.missingCredentials` | `.alert("登录信息丢失，请重启应用")` | AlertOverlayView | exit(0) | AlertOverlayView | presenter.dismiss() |
| `.network` | `.retry("网络异常，请检查后重试")` | RetryView | retry() 重跑 closure | RetryView | presenter.dismiss(triggerOnRetry: true) |
| `.decoding` | `.alert("数据异常，请稍后重试")` | AlertOverlayView | exit(0) | AlertOverlayView | presenter.dismiss() |
| 非 APIError generic | `.alert("操作失败", "请稍后重试")` | AlertOverlayView | exit(0) | AlertOverlayView | presenter.dismiss() |

注：bootstrap UI 与非 bootstrap UI 复用同一个 AlertOverlayView/RetryView component，但 onDismiss/onRetry 由 caller（RootView vs ErrorPresentationHostModifier）按路径语义注入不同 closure。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**修 review 找到的 error-handling regression 时**，**必须**输出"完整 error path 行为表"覆盖所有 error 类型 × 路径，而不是只 patch review 提到的那一条 case；尤其连续多轮 fix-review 同一 story 时，**禁止**单点 patch（必为综合方案）。
>
> **展开**：
> - **多轮 fix 同一 story 是危险信号**：第 3 轮起每轮都应当输出 error path 表，对照前一轮的 fix 是否破坏了表里其他 case。本次 round 5 finding 明显是 round 4 改 mapper 时没对照 transient business 行（如果对照，1009 应当从 retry 行掉到 alert 行，触发"alert OK 按钮要做什么"二次思考）。
> - **review 找到的 finding 永远是"已暴露的冰山"**：往往同根因还藏着未爆雷。改 mapper 一行代码，要 mental model 跑过表里**每一行**确认行为没退化。
> - **复用同一 view component 多条调用路径必须 caller 注入 closure**：view 内部硬编码 dismiss 行为 = 一处改动影响所有 caller，是 regression 温床。
> - **测试覆盖不能只测改的那一行**：mapper 改了 `.business` 分支 → 必须 grep 所有 `code: 1009` / `code: <transient code>` 的硬编码断言一并审。本次 fix 修了 4 个测试文件（mapper / state-machine / view-model / wire test）才把所有断言对齐。
> - **反例**：`只修 RootView 那一行 onDismiss = exit(0)，不改 mapper` → permanent business 还是死锁但 transient 出口 ok。`只修 mapper 把 1009 → retry，不改 onDismiss` → transient ok 但 permanent 仍死锁。两条必须同时改才完整。

---

## Meta: 本次 review 的宏观教训

5-5 这条 story 把"error handling 是单点 patch 累计而成的横切关注点"暴露得淋漓尽致。lesson 1 + 2 是表层 fix，lesson 3 才是根本预防 —— 未来 Claude 在 epic-loop / fix-review 跑同一 story ≥ 3 轮时，**必须**主动跳出"逐条改 review finding"的 mental model，写出 error path 行为表，对照矩阵全量审计。否则下一轮 review 几乎必然挖出新 regression。
