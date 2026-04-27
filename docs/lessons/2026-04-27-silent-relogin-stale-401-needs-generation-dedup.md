---
date: 2026-04-27
source_review: codex review round 3 of Story 5-4 (file: /tmp/epic-loop-review-5-4-r3.md)
story: 5-4-无效-token-静默重新登录
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-27 — actor coalesce 仅靠 inFlight 字段不足以拦 stale-401，需要 generation snapshot

## 背景

Story 5.4 实现 `SilentReloginCoordinator`（actor），目标契约：**concurrent 401s trigger one relogin**。round 1 / round 2 修复后，inFlight 字段 + spawned-task-bound cleanup 已能 coalesce "时序重叠" 的并发 401。但 round 3 codex 指出还有一个时序窗口未拦：caller A 完成并清空 inFlight 之后，caller B 才进入 `relogin()` —— B 的 401 是基于 pre-refresh 的 stale token，按契约应复用 A 的结果，旧实现却启动了第二次 `/auth/guest-login`。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Stale concurrent 401 触发第二次 relogin | medium (P2) | architecture | fix | `iphone/PetApp/Features/Auth/UseCases/SilentReloginCoordinator.swift:38-39` |

## Lesson 1: actor coalesce 必须区分两条独立路径——"inFlight 时序重叠" 与 "post-refresh stale 调用方"

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Auth/UseCases/SilentReloginCoordinator.swift:38-39`

### 症状（Symptom）

时序：
1. caller A 携 stale token T1 发请求 → server 401
2. A 进 `relogin()` → inFlight=nil → 启 spawned task → useCase.execute → 拿 T2 → finishInFlight → 返 T2
3. caller B 同样基于 T1 发请求，server 401 响应到达 client **晚于** A 完成
4. B 此时进 `relogin()` → 看到 inFlight=nil（A 已清空）→ 启**第二个** spawned task → useCase.execute（多余，违反契约）

inFlight 字段只能 coalesce "B 进入 relogin 时 A 还在跑" 的窗口；B 进入晚于 A 完成的窗口拦不住。

### 根因（Root cause）

把 "concurrent" 仅理解为 "时序重叠"。真实的 concurrent 401 = **多个 caller 都基于同一旧 token 发出了请求**，无论 server 401 响应到达 client 的顺序如何，他们都应共享同一次重登。inFlight 字段只刻画 "spawned task 是否还活着"，不刻画 "B 决定要 relogin 时基于的是哪一代 token"。

后者必须由 **caller 在调 relogin 之前** 提供 snapshot —— actor 内部任何时刻读到的 generation 都已经包含了 A 的影响，无法事后区分 "B 的 401 是 stale 还是 fresh"。

### 修复（Fix）

引入 `generation: UInt64` + `lastIssuedToken: String?` 双字段，新增 `currentGeneration()` 公开方法供 caller 在调 `relogin` 前 snapshot；`relogin(callerGeneration:)` 入口先做 generation 短路检查：

```swift
// SilentReloginCoordinator.swift
public func currentGeneration() -> UInt64 { generation }

public func relogin(callerGeneration: UInt64) async throws -> String {
    // (b) generation 路径：A 已成功完成,B 进入时 generation 已 ++ → 直接复用 A 的结果
    if generation > callerGeneration, let cached = lastIssuedToken {
        return cached
    }
    // (a) inFlight 路径：A 仍在跑,B 进入时 inFlight 非 nil → await 同一 task
    if let existing = inFlight { return try await existing.value }
    // 否则启 spawned task；finishInFlight(success:) 时 generation &+= 1 + lastIssuedToken = token
    ...
}

// 失败路径**不**推进 generation——失败不算"已经帮你 refresh 过"
private func finishInFlight(failure: Void) { inFlight = nil }
```

caller 端（`AuthRetryingAPIClient`）必须在 `inner.request` 之前 snapshot：

```swift
// AuthRetryingAPIClient.swift
let preReloginGeneration = await coordinator.currentGeneration()
do {
    return try await inner.request(endpoint)
} catch APIError.unauthorized where endpoint.requiresAuth {
    _ = try await coordinator.relogin(callerGeneration: preReloginGeneration)
    return try await inner.request(endpoint)
}
```

新增测试：
- `SilentReloginCoordinatorTests.testStaleConcurrent401DoesNotTriggerSecondReloginViaGenerationDedup` —— 直接验证 generation 路径（A 完成清空后 B 进入应复用 lastIssuedToken）
- `SilentReloginCoordinatorTests.testEqualGenerationDoesNotShortCircuit` —— 边界保护：cold start 时 caller snapshot=0、generation=0，不应误中短路（避免 lastIssuedToken=nil 时返回 nil）

测试结果：188 / 188 全绿。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计 "并发请求 coalesce 到同一次远端调用" 的协调器** 时，**必须** **同时考虑 "时序重叠" 与 "stale snapshot" 两条 dedup 路径**——前者用 inFlight Future / Task 字段，后者用 caller 在触发动作**之前** snapshot 的 generation（或 token 本身）。
>
> **展开**：
> - **dedup 不是只做 inFlight**：inFlight 字段只能拦 "B 进入时 A 还活着" 的子集。如果 A 完成后 B 才进入（B 的触发条件**早于** A 完成形成的），靠 inFlight 永远拦不住。
> - **caller 必须在动作之前 snapshot**：snapshot 时机是 "B 决定要 relogin 那一刻的状态"，不是 "B 进入 actor 那一刻"。actor isolation 让进入时已经看到 A 的全部 side effects，事后无法区分 stale / fresh。
>   - 在 `AuthRetryingAPIClient` 里这意味着：`let preGen = await coordinator.currentGeneration()` 必须在 `inner.request(endpoint)` **之前**，不能放在 `catch` 块内。
> - **失败不推进 generation**：generation ++ 必须严格绑定 "成功完成 + 写入 lastIssuedToken"。失败时不能让后续 caller 因为 generation 偏移而短路 —— "A 失败" 不构成 "A 帮 B refresh 过"。
> - **短路条件必须严格大于 + lastIssuedToken 非空**：cold start 时 generation=0 / lastIssuedToken=nil / caller snapshot=0；如果用 `>=` 会让 cold-start caller 拿到 nil，必须 `>` + 显式检查 `lastIssuedToken != nil`。
> - **反例**：
>   - 只用 inFlight 字段做 coalesce —— 漏掉 "post-refresh stale caller" 路径，单测用同步 5 并发能过，真实 race 下重复发请求。
>   - 把 generation snapshot 放在 actor 内部 read（如 `relogin()` 入口第一行）—— snapshot 总是等于当前值，永远不会触发短路，等价于没做。
>   - 用 token 字符串做 generation 但允许 caller 提供任意 token —— 攻击面：caller 可以伪造 "我的 token 是 X" 让 coordinator 永远短路。改用单调 UInt64 generation + actor 控制 ++，caller 只能 snapshot 不能写入。

---

## Meta: 本次 review 的宏观教训

Story 5.4 的 SilentReloginCoordinator 经过 3 轮 codex review，每轮发现一个新的 race window：
- round 1：cleanup 绑定 caller defer → 改成 spawned-task-bound cleanup
- round 2：本地态 `.unauthorized`（buildURLRequest 阶段）被误送进 relogin → 拆出 `.missingCredentials` case
- round 3：inFlight 拦不住 post-refresh stale caller → 加 generation snapshot

共同模式：**actor / 锁 / Future 这类 "顺序串行化" 工具，本身只能保证 read-modify-write 原子性，不能定义 "concurrent 操作" 的语义边界**。语义边界（"哪两个 caller 的 401 应该 dedup"）必须由 caller 主动声明（通过 snapshot / generation token），不能让协调器靠"我看到的状态"自行推断。

设计这类协调器时三问自己：
1. 两个 caller 的"决定要触发动作"那一刻是否同时？（→ inFlight 路径）
2. 一个 caller 的"决定"早于另一个的"完成"，但执行晚于完成？（→ generation 路径）
3. caller 的"决定"基于什么外部状态（旧 token / 旧版本号）？这个状态变化时旧"决定"应否被作废？（→ 是否需要让 caller 携带 snapshot）
