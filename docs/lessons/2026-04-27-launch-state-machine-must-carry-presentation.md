---
date: 2026-04-27
source_review: codex review on epic-loop Story 5.5 round 2 (file: /tmp/epic-loop-review-5-5-r2.md)
story: 5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据
commit: b39e7a5
lesson_count: 2
---

# Review Lessons — 2026-04-27 — Launch state machine 必须携带完整 ErrorPresentation 语义 + bootstrap 重试不能重发已成功的 guest-login

## 背景

Story 5.5 codex round 2 review 针对 round 1 fix-review 引入的 bootstrap closure。round 1 fix 解决了"bootstrap 失败展示 developer 串"的问题，把 `loadHomeUseCase` 失败包成 `BootstrapMappedError` 并经 `AppErrorMapper.userFacingMessage` 派出 user copy。但这一层包装把所有失败都降级为 `String message` 塞进 `AppLaunchState.needsAuth(message:)`，导致 RootView 永远渲染 `RetryView` —— `AppErrorMapper` 把 `.unauthorized` / `.missingCredentials` / `.decoding` 钦定为 `.alert`（"请重启应用" guidance），却被状态机 collapse 成 retry 屏，用户无法逃脱 retry loop。同时 `retry()` 重跑整个 step1 closure → 已经成功的 guest-login 也被重发，浪费一次 `/auth/guest-login` 请求。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Preserve error presentation type for `/home` bootstrap failures | high (P1) | error-handling, architecture | fix | `iphone/PetApp/App/AppLaunchState.swift`, `iphone/PetApp/App/AppLaunchStateMachine.swift`, `iphone/PetApp/App/RootView.swift` |
| 2 | Avoid rerunning guest login when only `/home` failed | medium (P2) | perf, error-handling | fix | `iphone/PetApp/App/RootView.swift` |

## Lesson 1: Launch state machine 状态枚举必须承载完整的 UI 决策语义（presentation 而非 message）

- **Severity**: high (P1)
- **Category**: error-handling / architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/App/AppLaunchState.swift`, `iphone/PetApp/App/AppLaunchStateMachine.swift:96`, `iphone/PetApp/App/RootView.swift:204-207, 380-432`

### 症状（Symptom）

`bootstrapStep1` closure 把 `LoadHomeUseCase.execute()` 抛出的任意错误（含 `.unauthorized` / `.missingCredentials` / `.decoding` 这些被 `AppErrorMapper` 钦定为 `.alert` 的错误）一律包装成 `BootstrapMappedError(userFacingMessage:)` → 状态机塞进 `.needsAuth(message:)` → RootView 永远渲染 `RetryView`。用户在 `.unauthorized` 场景下点重试 → `AuthRetryingAPIClient` 已 exhaust 重登 → 仍 401 → 再次走到这里 → 反复弹 retry view → unrecoverable retry loop。

### 根因（Root cause）

把 UI 呈现样式的决策权（`.alert` vs `.retry` vs `.toast`）压缩成单一 `String message` 是 lossy 转换。`AppErrorMapper` 已经做过分类决策（mapping switch 在 mapper 唯一 source-of-truth 处），但 bootstrap 边界把 mapping 的"样式"维度丢了，只留了"文案"维度。RootView `LaunchedContentView` 的 `.needsAuth` 分支没有任何信息可以做 alert vs retry 路由 —— 它只知道"出错了 + 一句话"，于是只能走默认 `RetryView`。这种"在边界上做 lossy projection"的反模式在网络错误处理里特别常见：mapper 做了好的分类，但边界压扁后 callsite 永远走最宽容的兜底（这里是 retry）。

### 修复（Fix）

**方案 A**（state 携带 ErrorPresentation 枚举，与 ErrorPresenter 已有基础设施对齐）。

1. `AppLaunchState`: `case needsAuth(message: String)` → `case needsAuth(presentation: ErrorPresentation)`
2. `BootstrapMappedError`: 字段从 `userFacingMessage: String` 升级为 `presentation: ErrorPresentation`；caller 在 closure catch block 内调 `AppErrorMapper.presentation(for:)` 而非 `userFacingMessage(for:)`，让样式 + 文案一起传递。
3. `AppLaunchStateMachine`: `messageFor(error:)` → `presentationFor(error:)`，优先识别 `BootstrapMappedError` 直接用其 `presentation`；非 `BootstrapMappedError` 的 LocalizedError 用 errorDescription 包成 `.retry(message:)`（fallback 优先 retry —— 给用户重试入口的宽容兜底）；plain Error 走 `defaultFailurePresentation`（也是 `.retry`）。
4. `RootView.LaunchedContentView`: `.needsAuth(let presentation)` 新增 `needsAuthContent(for:)` helper, 三态分发 → `.retry` 渲染 `RetryView`、`.alert` 渲染 `AlertOverlayView`（onDismiss 调 retry 让用户有重试入口避免死锁）、`.toast` 兜底为 alert（bootstrap 阶段不该用非 modal 提示）。

测试新增 4 个 case：`.unauthorized`、`.decoding`、`.missingCredentials` → `.alert` presentation；`.network` → `.retry` presentation。原有 3 个 case 升级为 `.needsAuth(presentation:)` 形态。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计或修复"启动/路由级 state machine 错误状态"时**，**必须** **让 state 携带完整的 UI 决策类型（如 ErrorPresentation 枚举），而不是把样式压缩成 String message 让 view 层去重新决定**。
>
> **展开**：
> - 一旦项目里已经存在 `ErrorPresentation` / `AppErrorMapper` 这种"统一映射 + 分类样式"的基础设施，所有错误边界（bootstrap closure / coordinator catch / global error handler）必须直接产出 `ErrorPresentation`，**不**做 `presentation → String → presentation` 的 lossy round-trip。
> - state machine 的状态枚举设计原则：**承载下游决策所需的全部信息**，而不是只承载"标签"。`.needsAuth(message: String)` 是反模式，因为它逼 view 层做"alert vs retry"的二次判断 —— 而那个判断应当属于 mapper 层。
> - 错误边界的 wrapper 类型（如 `BootstrapMappedError`）应当**预计算并携带 presentation**，而不是延迟到 view 层 re-classify。LocalizedError conformance 仅作为 log/debug 兜底，**不**用于 UI 决策。
> - **反例**：
>   - `enum LaunchState { case error(message: String) }` —— message-only 永远只能 retry。
>   - bootstrap closure 内 `throw error` 不包装 → state machine 拿到原始 APIError 后，要么取 errorDescription 给 developer 串，要么再调一次 mapper（重复决策点）。
>   - state machine 内部走 `AppErrorMapper.presentation(for:)` 兜底任意 plain Error → 任意 plain Error 都会变成 `.alert("操作失败", "请稍后重试")`（mapper 默认 fallback），但启动 closure 的 plain Error 大概率是 mock 测试 / 配置异常，给用户 retry 比给 alert 更宽容；判断逻辑应当在 caller（closure）层显式做。

## Lesson 2: Bootstrap 多 step 串行链路重试时，已经成功的 step 必须用 idempotent flag 短路，避免无谓重发

- **Severity**: medium (P2)
- **Category**: perf / error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift:172-216, 296-310`

### 症状（Symptom）

`bootstrapStep1` closure 内串行跑 `guestLoginUseCase.execute()` + `loadHomeUseCase.execute()`。当 guest-login 成功但 load-home 失败 → 进入 `.needsAuth` → 用户点重试 → `AppLaunchStateMachine.retry()` 重置 `hasBootstrapped` flag 重跑整个 step1 closure → guest-login 又发一次 `/auth/guest-login`。后果：（1）每次 home 重试浪费一次 auth 往返；（2）让原本只是 `/home` 临时失败的恢复链路也得 auth endpoint 健康才能成功；（3）token 已存在的情况下重新生成 token 是反语义的资源浪费。

### 根因（Root cause）

把 "原子启动事务" 的语义边界设错了。事务边界应该是"用户感知层面的成功/失败"（startup 是否成功），而不是"代码执行层面的整个 closure 重跑"。`AppLaunchStateMachine.retry()` 是无脑"重置状态 + 重跑 step1 closure"，没有区分 step 内部已经完成的子动作。closure 内的 sub-steps 没有自己的 idempotency 概念，导致重试粒度过粗 —— 把"重试整个启动序列"等同于"全量重发所有底层请求"。

### 修复（Fix）

closure 内引入 `GuestLoginCompletionGate` actor 作为 idempotent flag。第一次成功后 `markCompleted()`；第二次进 closure 时先检查 `hasCompleted`，已完成则跳过 `useCase.execute()` 直接走 load-home。actor 隔离 mutable state，避免 `@Sendable` closure 直接捕获 `var` 的 Swift 6 strict concurrency 警告。

```swift
let guestLoginGate = GuestLoginCompletionGate()
launchStateMachine = AppLaunchStateMachine(
    bootstrapStep1: { @Sendable in
        let alreadyLoggedIn = await guestLoginGate.hasCompleted
        if !alreadyLoggedIn {
            let output = try await useCase.execute()
            await MainActor.run { sessionStore.updateSession(...) }
            await guestLoginGate.markCompleted()
        }
        // load-home 一直会重跑（它才是真正失败的那步）
        let homeData = try await loadHomeUseCase.execute()
        ...
    }
)
```

测试 `RootViewWireTests.testBootstrapClosureSkipsGuestLoginAfterSuccessfulCompletion` 复刻 closure 模式 → 首次 bootstrap guest-login 跑 1 次 + load-home 失败 → retry 后 guest-login 仍是 1 次（**断言守护**）+ load-home 重试到 ready。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写"多步串行 + 整体 retry" 的启动 / coordinator closure 时**，**必须** **为已经成功的 sub-step 设置 idempotent gate（actor flag / hasCompleted bool），让 retry 只重跑真正失败的那段，而不是整个 closure**。
>
> **展开**：
> - state machine 的 `retry()` 通常是"重置 flag + 重跑 closure"的粗粒度操作 —— 这是 state machine 的合理职责，但 closure 内必须自己处理 sub-step 粒度的 idempotency。
> - "重试 = 重发所有请求" 是新手陷阱：很多时候只有最后一步失败，前面的 token 写入 / session 注入 / 缓存预热都已经完成且具备幂等性。
> - 选 actor 持有 flag（而不是 closure 捕获 `var`）是 Swift 6 strict concurrency 时代的标准做法 —— `@Sendable` closure 拒绝捕获可变 var；用 actor 既隔离又类型安全。
> - 测试必须**直接断言 sub-step 调用次数**（用 `CallCounter` actor），不能只断言"最终 state 是 ready" —— ready 只能证明"功能恢复"，不能证明"恢复路径没有浪费请求"。
> - **反例**：
>   - `bootstrapStep1: { @Sendable in try await guestLogin(); try await loadHome() }` 不带 gate → retry 全量重发。
>   - 把 idempotency 推到 UseCase 内部（"反正 GuestLoginUseCase 是幂等的，重发也无害"）—— 浪费往返 + 模糊了"什么算成功的边界"，且 UseCase 不应感知调用方的重试语义。
>   - 把 step 拆成 `bootstrapStep1` / `bootstrapStep2`（state machine API）—— 现有 state machine 串行 step 的 retry 仍是全量重跑（它没有 per-step retry 概念），改 API 成本高于改 closure。

---

## Meta: 本次 review 的宏观教训（可选）

两条 finding 共享同一个根因：**"上一轮 fix 仓促，把语义压缩成更窄的形态以求 minimal diff，但语义压缩的代价是把决策权下推到不该承担的层（view 层 / 重试机制）"**。

- Lesson 1：把 `ErrorPresentation` 压成 `String` —— 丢了样式维度，view 层无能为力，只能走最宽容的 retry。
- Lesson 2：把 "GuestLogin + LoadHome 整体" 压成 "step1 closure" —— 丢了 sub-step 完成度维度，retry 机制无法区分要重跑哪段，只能全量重发。

**通用规则**：**fix-review 修一个错误时，留意"我引入的 wrapper / 新边界"是否会丢失下游需要的语义维度**。如果新边界的输出形态比 caller 期望的形态窄，下一轮 review 会立刻暴露这个 lossy projection。修复 round 1 的"developer 串漏到 UI"时，应当一开始就让 BootstrapMappedError 携带 ErrorPresentation 而非 String —— round 2 fix 实际是把 round 1 的"压成 String"逆转回"携带 presentation"。
