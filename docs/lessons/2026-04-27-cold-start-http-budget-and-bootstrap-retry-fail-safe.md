---
date: 2026-04-27
source_review: codex round 3 review on Story 5.5 fix-review (file: /tmp/epic-loop-review-5-5-r3.md)
story: 5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-27 — 冷启动 HTTP 预算钦定 ≤2 时不能保留任何 nice-to-have 探针 + bootstrap retry 必须 fail-safe 重跑 auth 不能基于"曾经成功"短路

## 背景

Story 5.5 第三轮 codex review。前两轮修了多个 P1/P2，但 round 2 的 fix 自身又引入两条 round 3 P1：

1. **冷启动 HTTP 预算违约**：spec line 11 钦定"启动 → 主界面" 路径只能发 2 个 HTTP（`/auth/guest-login` + `/home`）。但 Story 2.5 留下的 `homeViewModel.start()` 调用在 RootView 启动 `.task` 内仍然会触发 `/ping` —— 共 3 个 HTTP，违反 spec。
2. **重试死循环**：round 2 P2 的 fix 引入 `GuestLoginCompletionGate` actor，把"guest-login 曾经成功"永久记下，让 retry 跳过重发。但当 `/home` 因 `.unauthorized` / `.missingCredentials` 失败时，retry 复用同一份**坏掉的**鉴权状态，永远恢复不了，用户只能重启 App。

两条都是经典"上一轮 fix 引入下一轮 bug"的迭代陷阱。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `/ping` 留在冷启动链路违反 ≤2 HTTP 钦定 | high | architecture | fix | `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift` `iphone/PetApp/App/RootView.swift` |
| 2 | `GuestLoginCompletionGate` 永久跳过 → `.unauthorized` / `.missingCredentials` 重试死循环 | high | error-handling | fix | `iphone/PetApp/App/RootView.swift` |

## Lesson 1: 冷启动 HTTP 预算钦定 ≤N 时所有非首屏必需的探针都必须移出启动路径

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift:124-138`、`iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift:194-209`

### 症状（Symptom）

Story 5.5 spec 明确写"启动 → 主界面 ≤ 2 HTTP（`/auth/guest-login` + `/home`）"。但 RootView 的启动 `.task` 内仍然有从 Story 2.5 遗留下来的 `await homeViewModel.start()` 调用，会发起第 3 个 HTTP（`/ping`）拿 server commit 显示在版本号 footer。这违反 spec 钦定预算。

### 根因（Root cause）

- Story 2.5 给 ping 设计的角色是"App 启动一次性 server 探针" → 自然挂在启动 `.task`。
- Story 5.5 引入新约束（≤2 HTTP）但没审计**已有**的启动 .task 是否仍合契约。新增 LoadHome 调用挂到独立 `.task` 与 ping `.task` 并发跑，前两轮 review 都聚焦在 LoadHome / 错误处理，没回头质疑 ping 的归属。
- 心智模型陷阱：把"加新功能"和"减老功能"分开思考。预算契约（不超过 N 个 HTTP）是**所有路径**的总和约束，加一个新请求就要审视是否要踢一个旧请求。

### 修复（Fix）

**双层 defense-in-depth**：

1. **在 `HomeViewModel.start()` 内加第 4 层短路**：`if hasLoadedHome { return }`。语义：`/home` 成功本身已证明 server reachable + token 有效，ping 是冗余探针。

2. **从 RootView 启动 `.task` 移除 `await homeViewModel.start()` 调用**。`bind(pingUseCase:)` 仍保留（保持注入语义），但不再触发 ping。即使未来某处错把 `start()` 加回启动路径，第 1 层短路也能兜底。

```swift
// HomeViewModel.swift
public func start() async {
    // Story 5.5 round 3 [P1] fix: 启动期 LoadHome 已成功 → ping 冗余, 短路
    if hasLoadedHome {
        hasFetched = true
        return
    }
    // ... 既有逻辑
}

// RootView.swift
.task {
    homeViewModel.bind(pingUseCase: container.makePingUseCase())
    homeViewModel.bind(loadHomeUseCase: ..., errorPresenter: ...)
    // ⚠️ 移除: await homeViewModel.start() —— 违反 ≤2 HTTP 钦定
}
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：当 spec / story / 性能 NFR 钦定"启动路径 ≤N 个 HTTP" 时，未来 Claude 在引入新 HTTP 调用前**必须**列出该路径上**全部**已有调用，逐一证明每个仍是首屏必需；非必需的（探针 / 缓存预热 / 上报 / 版本检查）必须延迟到首屏 `.ready` 之后或完全移除。
>
> **展开**：
> - "启动路径"包括所有在第一帧渲染前能触发的代码：`@StateObject` init、`.onAppear`、`.task`、跨 `.task` 并发 closure。SwiftUI `.task` 在 view 重新出现时会重启，跨 sheet/cover 关闭也会触发。
> - 心智模型：HTTP 预算契约是**减法约束**，加新请求要先腾位置。新 story 引入 `/home` 时必须同步审视 `/ping` 等旧请求是否仍属"启动 → 首屏"必需。
> - 短路点要有 defense-in-depth：(a) 调用方不调用（移除 .task 内 await）+ (b) 被调用方自防（start() 内查 hasLoadedHome 短路）。前者保证当前路径，后者防御未来回归。
> - **反例**：只在 ViewModel 内加短路 flag 但不删除调用方的 `await ...start()` —— flag 会在 ViewModel 重建时失效（@StateObject 也可能在某些场景重新初始化），单层防御不够。
> - **反例**：把"探针"调用挂到启动 `.task` 但不评估每帧 budget。"App 启动一次性"≠"必须在第一帧前"，可挂到 `.ready` 后的 .task / .onAppear 而不是与首屏 critical path 抢预算。

## Lesson 2: bootstrap retry 必须 fail-safe，不能基于"上次成功"短路 auth 步骤

- **Severity**: high
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift:185-201`（修复前的 `GuestLoginCompletionGate` actor + bootstrapStep1 closure）

### 症状（Symptom）

Round 2 P2 fix 为了避免"重试时重发已成功的 `/auth/guest-login`"，引入 `GuestLoginCompletionGate` actor 持永久 `hasCompleted` flag。但当 `/home` 因 `.unauthorized` / `.missingCredentials` / 静默重登后仍 `.unauthorized` 失败时：

1. 用户点重试 → `retry()` → `bootstrap()` → step1 closure
2. closure 看 `gate.hasCompleted == true` → **跳过** `useCase.execute()`
3. closure 跑到 LoadHome 段，复用同一份**坏掉的**鉴权状态调 `/home` → 同样的 `.unauthorized` 失败
4. 状态机进入 `.needsAuth(.alert(...请重启应用...))`，用户点 dismiss → 又触发 retry → 死循环
5. 用户唯一恢复路径：重启 App

### 根因（Root cause）

- Round 2 修复的是 P2 finding"retry 多发一次 /auth/guest-login"，目的是省一次 ~50ms 往返 + 减少 auth endpoint 依赖。但优化的代价被严重低估：丢掉了"retry 能刷新坏掉鉴权"的语义。
- gate 的语义模型缺陷：`hasCompleted = true` 永久记 "曾经成功"，但**不**记"当前 token 是否仍有效"。两者在大部分时间一致，但一旦 token 因 server 端 revoke / clock skew / keychain 异常失效，retry 必须能重新协商。
- 优化导向陷阱：把"幂等操作的重复调用"当 cost；其实 `useCase.execute()` 是写新 token 的幂等 op，重复调一次产生新鲜 token，这正是 retry 想要的副作用。

### 修复（Fix）

**移除 `GuestLoginCompletionGate` actor 完全**。bootstrapStep1 closure 改回 fail-safe 形态：每次跑都无条件调 `useCase.execute()` 写新 token。

```swift
// 修复前 (round 2 P2)：gate 短路
let alreadyLoggedIn = await guestLoginGate.hasCompleted
if !alreadyLoggedIn {
    let output = try await useCase.execute()
    sessionStore.updateSession(...)
    await guestLoginGate.markCompleted()
}
// 之后跑 LoadHome ...

// 修复后 (round 3 P1)：retry 时 guest-login 也重跑
let output = try await useCase.execute()  // 永远跑, 保证刷新坏掉的 token
sessionStore.updateSession(...)
// 之后跑 LoadHome ...
```

理由：

- 多 ~50ms 一次往返的成本远小于"重试可恢复"的语义价值。
- `GuestLoginUseCase.execute()` 幂等（每次产生新 token 写 keychain），重复调用无副作用。
- Round 2 P2 的"省往返"优化属于过早优化（premature optimization）—— 重试本身已是错误恢复路径，不是高频热路径。

### 预防规则（Rule for future Claude）⚡

> **一句话**：在 retry / 重连 / 错误恢复路径上，未来 Claude **禁止**用"上次成功"flag 短路任何会重新协商外部状态（token、session、连接、租约）的操作 —— 这类操作的"重做"本身就是恢复机制；短路 = 把恢复机制阉了。
>
> **展开**：
> - 区分两种短路语义：
>   - **同次操作内幂等短路**（OK）：同一次 user action 触发的并发请求合并成一个，避免重复发起。如 `start()` 内 `pingTask != nil` short-circuit。
>   - **跨用户重试动作的"曾经成功"短路**（危险）：把"曾经成功"当"当前仍有效"，会丢失重试的恢复语义。如 `GuestLoginCompletionGate.hasCompleted`。
> - 优化"重试时多发一次请求"前必须问：(a) 这个请求是否为恢复机制本身（如重新协商 token）？是 → 不能优化掉。(b) 有没有 alternative 短路条件能区分"已有效"vs"仅曾经有效"？很难写对，往往需要轮询 / 探测 → 比直接重发更贵。
> - fail-safe 默认：在错误恢复路径上"多发一次幂等请求" 是默认行为，需要短路必须给出**反向**理由（如：实测高频导致服务端 throttle）。
> - **反例**：用 `actor SessionGate { var hasLoggedIn: Bool }` 短路 retry 路径上的 login。session 状态本身可能在 retry 前一刻被 server 端 invalidate（如 admin force-logout、clock skew、token revoke），retry 必须能通过重新登录刷新；gate 让 retry 复用 invalidated session 永远恢复不了。
> - **反例**：用 "first-success-only" 模式做幂等键短路。幂等键的语义是"同一逻辑请求最多被处理一次"，不是"未来所有重试都跳过"。前者短路在 server 端（同 idempotencyKey 返回 cached result），后者短路在 client 端（client 不发请求）—— 后者切断 server 端纠错能力。

---

## Meta: 本次 review 的宏观教训

**iterative fix 的 regression 风险**：fix-review 命令本身是好工具，但每一轮 fix 都可能引入下一轮 bug。round 1 改 P2（错误映射）→ round 2 引入 P1（presentation 丢失）+ P2（retry 多发 guest-login）→ round 3 P1（重试死循环 + ping 冗余）。

**反思规则**：每次 fix-review 修完后，做一次 30 秒 self-review：

1. 这次修改**新引入**的代码路径在哪些场景会触发？覆盖了 happy path 之外的 sad path 吗？
2. 这次修改**删除**或**短路**的代码路径，原本承担什么角色？是否还有其他地方依赖该角色？
3. 这次修改影响了什么**契约**（接口 / spec / 性能 NFR）？是否所有契约都仍满足？

特别警惕"为了不再被 review flag 而过度短路"模式（round 2 加 gate 是为了消除 P2 finding 的 review noise）—— review 找出问题不等于必须用最激进方式消除，常常退一步保留 fail-safe 行为反而更对。
