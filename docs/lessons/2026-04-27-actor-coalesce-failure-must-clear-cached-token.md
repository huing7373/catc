---
date: 2026-04-27
source_review: codex review round 4 (file: /tmp/epic-loop-review-5-4-r4.md)
story: 5-4-无效-token-静默重新登录
commit: 83f8292
lesson_count: 1
---

# Review Lessons — 2026-04-27 — actor coalesce 失败路径必须连带清空 cached result，否则 generation 短路会返回已被 invalidate 的旧 token

## 背景

Story 5.4 fix-review round 3 引入了 generation snapshot 机制（`callerGeneration < generation` 短路返回 `lastIssuedToken`），用来 dedup "stale 401 caller 在 A 完成成功 refresh 之后才进 relogin" 的场景。round 4 codex 发现新引入的 generation 机制有一个 corner case：失败的 finishInFlight 只清 `inFlight` 不清 `lastIssuedToken`，导致一个**之后**再发生的 refresh 失败之后，旧 generation 的 stale caller 仍会被短路到一个**已经被 server invalidate** 的旧 token。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Clear cached token after a later relogin failure | P2 (medium) | error-handling | fix | `iphone/PetApp/Features/Auth/UseCases/SilentReloginCoordinator.swift:117-120` |

## Lesson 1: actor coalesce 协调器的失败 finish 必须把 cached result 一起清掉

- **Severity**: P2 (medium)
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Auth/UseCases/SilentReloginCoordinator.swift:117-120`

### 症状（Symptom）

下面这条 4 步时序在新协调器下能复现 caller 拿到已被 invalidate 的 token：

1. caller B 携 stale token T0（snapshot `gen=0`）发 HTTP 请求；server 401 响应到达 client 较慢
2. 与此同时 caller A 完成成功 refresh → `generation=1, lastIssuedToken=T1`
3. 之后某个新 caller 触发了**又一次** refresh 尝试，但失败了（比如 keychain miss / 网络瞬断）→ `inFlight` 被清，但 `lastIssuedToken` 仍然 = T1
4. caller B 的 401 现在才进 `relogin(callerGeneration=0)` → `generation(1) > 0` 且 `lastIssuedToken=T1` 非 nil → 短路返回 T1
5. AuthRetryingAPIClient 拿 T1 重试 → server 又 401（因为 T1 早就被 server invalidate）→ 抛 `.unauthorized`，**跳过了本应启动的真正 relogin**

### 根因（Root cause）

generation 短路的隐含前提是"`lastIssuedToken` 一定还是当前最新且有效的"。这个前提只在 happy path（每次都成功，generation 单调推进）成立。一旦中间夹一次失败，**最新的 generation `+ 1` 没发生（失败不推进）**，但 `lastIssuedToken` 仍指向**比当前 server 端认知更老的**那个 token —— 它已经被 invalidate（server 端通常一次成功 refresh 就会 invalidate 老 token；这里 T1 在第 3 步失败之后实际已被取代为某个不存在的更新版本意图）。短路条件 `generation > callerGeneration && let cached = lastIssuedToken` 看不到这个失效语义，把"旧成功"当成"当前最新"返回。

更抽象的根因是 **coalesce 协调器的两个状态字段（"是否在跑" + "上次成功结果"）之间存在一个隐含 invariant —— "成功结果只在它仍然代表 server 端最新认知时可被复用"** —— 失败路径如果只清"在跑"标志而不清"上次成功结果"，就破坏了这个 invariant。

### 修复（Fix）

最小改动：`finishInFlight(failure:)` 清空 `inFlight` 时同时清空 `lastIssuedToken`。

```diff
 private func finishInFlight(failure: Void) {
     inFlight = nil
+    lastIssuedToken = nil
 }
```

效果：B 的 401 此时再进 `relogin(callerGeneration=0)` → `generation(1) > 0` 仍成立，但 `let cached = lastIssuedToken` (`= nil`) if-let 失败 → 短路条件**整体不命中** → 走 (a)/(c) 正常路径启新 useCase.execute → 拿真正的新 token。

新增回归测试 `SilentReloginCoordinatorTests.testFailureClearsCachedTokenSoStaleCallerWontGetInvalidatedToken`：编排 "A 成功 → 一次失败 → stale caller 进入" 三步时序，断言 stale caller 拿到的是"失败后启的第三次 useCase"产生的新 token，而不是已被 invalidate 的 T1，且 useCase 共被调 3 次（不是被短路成 2 次）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写"actor / mutex 包裹的 coalesce 协调器（in-flight task + last-result cache）"时，**失败路径必须把 last-result cache 也一起清空**，且每个**短路返回 cached 结果的条件**必须显式包含 "cache != nil"，不能只依赖 generation / version 字段。
>
> **展开**：
> - **两个状态字段的 invariant 要写在代码注释里**：例如 "lastIssuedToken 只在 success finish 写入；failure finish 必须连带清空"。这个 invariant 一旦只在脑子里，下一次改 finish 路径就会漏掉。
> - **generation / version 字段不是 cache freshness 的充分条件**：generation 只表达"成功完成过几次"，不表达"最新 cached 结果当前是否还代表 server 端认知"。失败 + 不推进 generation 会让两者脱节。
> - **短路条件必须 if-let 包住 cache，而不是先判 generation 再返 cache!**：写法 `if generation > callerGeneration, let cached = lastIssuedToken { return cached }` 是对的；写法 `if generation > callerGeneration { return lastIssuedToken! }` 既会 crash 也会绕过这条防线。
> - **新增回归测试必须编排"成功 → 失败 → stale caller 进入"三步时序**：只测"失败后能否再启 useCase"不够，必须验证"stale caller（用 pre-成功的 callerGeneration）进入时，cache 被清空导致它走正常路径而不是被短路到失效 token"。
> - **反例**：在 `finishInFlight(failure:)` 里只写 `inFlight = nil` 然后留个注释说"失败不推进 generation 也不写 lastIssuedToken"。注释里只说"不写"是不够的——失败路径**必须主动清掉之前成功路径写入的 cache**，否则之前的写入会跨过这次失败"飘"到后续 caller。
