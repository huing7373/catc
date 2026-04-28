---
date: 2026-04-28
source_review: codex review (epic-loop round 11) — /tmp/epic-loop-review-5-5-r11.md
story: 5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-28 — error case 不该 conflate transient (store 读失败) vs terminal (store 读成功但空) 两种语义

## 背景

Story 5.5 codex review **round 11**。round 9 + round 10 把 transient/terminal 二分判则贯彻到 mapper 所有分支后，本轮 review 抓到一个更深的 conflate ：判则在 **mapper** 层做对了，但 **APIError case 本身**仍然把 "本地存储抛错 (transient)" 与 "本地存储读成功但返 nil/空串 (terminal)" conflate 进同一个 `.missingCredentials` case。mapper 看到的输入语义已经丢失，无论 mapper 写得多严谨，都救不回 transient 路径被误判 terminal 的 regression。

review 引用：

> [P2] Keep transient keychain read failures retryable — `iphone/PetApp/Shared/ErrorHandling/AppErrorMapper.swift:148-155`
> `APIClient.buildURLRequest(_:)` already collapses `keychainStore.get(...)` throws into `.missingCredentials`, so this case is not limited to a permanently absent token. When `/home` hits a transient keychain access error right after guest login, the new `.missingCredentials -> .alert` mapping routes bootstrap into `TerminalErrorView` with no retry path, even though rerunning bootstrap would re-execute guest login and can recover. Please either distinguish "token missing" from "keychain read failed", or keep this bootstrap path retryable.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | APIError 必须把 "本地存储抛错 (transient)" 跟 "本地存储读成功但返空 (terminal)" 拆成两个 case | P2 | error-handling, architecture | fix | `iphone/PetApp/Core/Networking/APIError.swift`, `iphone/PetApp/Core/Networking/APIClient.swift`, `iphone/PetApp/Core/Networking/AuthRetryingAPIClient.swift`, `iphone/PetApp/Shared/ErrorHandling/AppErrorMapper.swift`, `iphone/PetAppTests/Core/Networking/APIClientAuthInjectionTests.swift`, `iphone/PetAppTests/Shared/ErrorHandling/AppErrorMapperTests.swift` |

## Lesson 1: 错误分类的"信息保真度"必须从 case 设计层就做对，不能依赖下游 mapper 救场

- **Severity**: P2
- **Category**: error-handling, architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/APIError.swift:43-58` (原 `.missingCredentials` 单 case 设计) + `iphone/PetApp/Core/Networking/APIClient.swift:238-248` (`buildURLRequest` 用 `try?` 把抛错也 collapse 成 missingCredentials)

### 症状（Symptom）

`APIClient.buildURLRequest(_:)` 在注入 Authorization header 时三种本地态被 collapse 成同一个 `.missingCredentials`：

1. `keychainStore == nil` (DI 没配 — terminal)
2. `keychainStore.get(...)` 抛错 (sandbox 抽风 / OSStatus -25291 errSecNotAvailable 等 — **transient**)
3. `keychainStore.get(...)` 返 `nil` 或空串 (token 真没写入 — terminal)

mapper round 9 钦定 `.missingCredentials → .alert (terminal force-quit)` 是基于 case 1 + case 3 的语义；但 case 2 也走 `.missingCredentials` 通道，于是 sandbox 抽风等 transient 错误被错误归为 terminal force-quit。

bootstrap 路径下 RootView 把 `.alert` 渲染成 `TerminalErrorView` (无按钮 force-quit only)，user 即使 sandbox 抢权抽风导致 keychain 暂时读不出来，也只能 kill App。bootstrap closure 重跑明明可以让 cold-start GuestLoginUseCase + LoadHome 自愈 transient 故障，被 `.alert` 路径阻断。

### 根因（Root cause）

round 9/10 lesson 的修复方向都集中在 mapper 层做 transient/terminal 二分判则（"transient possible → `.retry`，permanent guaranteed → `.alert`"），漏审了**判则的输入信号本身是否 lossless**。

具体：APIClient 在 Story 5.4 round 2 [P2] fix 把 `.missingCredentials` 从 `.unauthorized` 拆出时，用 `let token = try? keychainStore.get(...)` 把"keychain 抛错"和"keychain 返 nil/空串"都 collapse 成"无 token"语义，再统一抛 `.missingCredentials`。这条选择当时的 framing 是"client 不需要分辨基础设施细节，统一'本地无 token'对上层最简洁"。

但 round 9 把 `.missingCredentials` 钦定为 terminal `.alert` 后，case 设计的信息丢失立刻**变成 user-facing regression**：

- 原 `.unauthorized` 通道下 sandbox 抽风也走 `.alert` 是 round 5 钦定（"token 失效需重启"），用户体验 acceptable
- round 2 把"本地无 token"拆出来 → 单独走 `.missingCredentials`
- round 9 把 `.missingCredentials` 改 `.alert(force-quit only)` → conflate 进来的 sandbox 抽风也被 force-quit

mapper 层无法从 `.missingCredentials` 反推"这次是抛错还是返空" —— 信息已经在 APIClient 层永久丢失。**判则贯彻得再彻底，也救不回 lossy 输入**。

### 修复（Fix）

**方案 A（精细 — 选用）**：把 transient 子态从 `.missingCredentials` 拆出到新 case `.localStoreFailure(underlying: Error)`，让 mapper 能按二分判则归类：

```swift
// APIError.swift —— 新增 case
case localStoreFailure(underlying: Error)

// APIClient.swift buildURLRequest —— 把 try? 拆开
let token: String?
do {
    token = try keychainStore.get(forKey: KeychainKey.authToken.rawValue)
} catch {
    throw APIError.localStoreFailure(underlying: error)
}
guard let token, !token.isEmpty else {
    throw APIError.missingCredentials   // 真 terminal: 读成功但确认无
}

// AppErrorMapper.swift —— 新增分支
case .localStoreFailure:
    return ErrorPresentation.retry(message: "登录信息读取异常，请重试")
```

同步：

- `APIError.Equatable` 加 `.localStoreFailure` 比较（仅看 case 标签 — underlying Error 不是 Equatable）
- `APIError.errorDescription` 加 dev-facing 串
- `AuthRetryingAPIClient` 注释加 "为何不拦 .localStoreFailure"（同 .missingCredentials 理由：sandbox 抽风也会让 SilentReloginUseCase 内部读 guestUid 失败 → 放大 N+1 调用）
- `APIClientAuthInjectionTests` case#5 (keychain.get 抛错) 断言从 `.missingCredentials` 改 `.localStoreFailure` + 反向断言 `.notEqual(.missingCredentials)` 防 regress
- `AppErrorMapperTests` 新增 case#4c：`testLocalStoreFailureMapsToRetry` + 反向断言 `.notEqual(.alert("登录信息丢失，请重启 App"))`

**为何不选方案 B（mapper 把 .missingCredentials 改 .retry）**：

方案 B 简单但失真：让"永久 token 缺失"也走 retry，user 反复重试永远不会成功（cold-start GuestLoginUseCase 同样读不到 token），UX 负面更大。方案 A 在 case 设计层直接保留语义，mapper 各自走对路径，是 lossless 的拆分。

修复后的 mapper 完整分类表（round 11 后）：

| Error 类 | 子语义 | round 10 | round 11 |
|---|---|---|---|
| `.unauthorized` | server 拒绝 token | `.retry` | `.retry` |
| `.decoding` | server payload 异常 | `.retry` | `.retry` |
| `.missingCredentials` | 本地确认无 token (terminal) | `.alert` | `.alert` |
| `.localStoreFailure` | 本地存储抛错 (transient) | **不存在该 case** | **`.retry`** ✨ |
| `.business(transient: 1005/1007/1008/1009)` | server 限流冲突 | `.retry` | `.retry` |
| `.business(其他 permanent)` | server 永久错 | `.alert` | `.alert` |
| `.network` | transport 失败 | `.retry` | `.retry` |
| 非 APIError fallback | 未知 | `.retry` | `.retry` |

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**为某个错误通道做 mapper 分类决策**之前，**必须**先审 **error case 的"输入信号是否 lossless"** —— 如果上游把"transient (临时 IO 抽风)"和"terminal (确认无资源)"两种**判则反映不同**的子语义 collapse 成同一 case，mapper 无论怎么分类都救不了，必须**先在 case 设计层把它们拆开**。
>
> **展开**：
> - **mapper 是分类决策点，不是信息恢复器**：mapper 只能基于 case 标签 + payload 决策；如果 case 设计已经丢失"transient vs terminal" 的信号区分，mapper 永远是 lossy 输出。下游 fix 救不了上游 collapse。
> - **try? / catch ignore 是潜在的 conflate 信号丢失点**：用 `try?` 把抛错降级为 `nil` 是常见 Swift 习语，但当 "返 nil" 与 "抛错" 在下游分类时**应当走不同 presentation**，就**不能** collapse 进同一通道。回头审 `try?` 是否在某个新增 mapper 分类后变成了 lossy bridge。
> - **新增 mapper 分类决策时，反审上游 case 设计**：例如 round 9 把 `.missingCredentials` 钦定为 terminal `.alert`，应同时反问"这个 case 的所有源头是否都真 terminal" —— 若答案"不是" (本例：keychain 抛错也走这个 case)，则**先拆 case** 再 mapper 分类。
> - **误判 alert 的代价 ≠ 误判 retry 的代价**：force-quit-only 的 `.alert` 误用代价远大于 retry 的多发请求，conflate 时 conservative 的方向是 retry 而不是 alert。case 设计层若难以拆分，临时办法是 mapper 走 retry；正确办法是 case 设计层拆开（本轮选方案 A）。
> - **每一轮 fix-review 改 mapper 分类时建"反审清单"**：① 这个 case 是否还代表它原始 framing 描述的语义？② 它有几个源头？③ 每个源头按新分类是否合理？④ 哪些源头需要拆出新 case？这是 round 9/10/11 三轮迭代沉淀出的方法论。
> - **Equatable underlying Error 的实现选择**：当新 case 的 associated value 是 Error (不 Equatable)，测试断言粒度通常只关心"是否归类正确"，按"仅比较 case 标签"实现 `==` 即可（本例 `case (.localStoreFailure, .localStoreFailure): return true`），不强求深度比较。
> - **反例 1**：在 `.missingCredentials → .alert` 钦定后，不审上游 collapse 链；下一轮 review 必抓 transient 路径被误判 terminal —— 本轮 round 11 就是该反例的实证。
> - **反例 2**：用 `try?` 简化代码 → 把"抛错"和"返 nil"语义合并 → 下游分类需要分别处理时被迫 lossy mapping。规则：**任何 `try?`/`catch _` 处都标注"这两路语义在下游是否需要区分"**，是的话现在就拆。

---

## Meta: 本次 review 的宏观教训

round 9 → round 10 → round 11 是同一思想的三层贯彻：transient/terminal 二分判则。round 9 在 mapper 把具体 case 改对；round 10 把 fallback 也按判则补齐；round 11 把判则**反推到上游 case 设计**，发现 `.missingCredentials` 自己 conflate 了不同语义子集。

每一轮的迭代都暴露同一类盲区：判则只在"被显式审视的层"贯彻，相邻层（fallback 是 mapper 的"未被框定分支"，case 设计是 mapper 的"输入端")会留死角。规则：**每次钦定通用判则，都列三层贯彻清单**：① 显式 case 是否符合判则；② fallback / unknown 分支是否符合判则；③ **case 自身的源头是否都符合该 case 的判则归类** —— 不符合就拆 case。

跟 round 10 lesson "fallback 也是分类决策点" 的延伸：**case 设计也是分类决策点**。判则贯彻必须 cover 三层 (case 内分支 / fallback 分支 / case 自身设计)，否则下一轮 review 几乎必然抓出新的死角。
