---
date: 2026-04-28
source_review: codex review (epic-loop round 10) — /tmp/epic-loop-review-5-5-r10.md
story: 5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-28 — `AppErrorMapper` 非 APIError fallback 必须按 transient 二分原则归 `.retry`，不是 `.alert`

## 背景

Story 5.5 codex review **round 10**。round 9 刚把 `.decoding` / `.unauthorized` 按 transient/terminal 二分判则从 `.alert` 改成 `.retry`，落定"transient possible → `.retry`，permanent guaranteed → `.alert`"通用判则。round 10 review 抓到这条判则没贯彻到底 —— `AppErrorMapper.presentation(for:)` 的 **non-APIError fallback** 仍然返 `.alert("操作失败", "请稍后重试")`，让 `GuestLoginUseCase.execute()` 抛出的 `KeychainError`（典型 transient 来源：sandbox 临时不可用 / osStatus -25300 item 暂时找不到）走 bootstrap `.alert` 路径 → 渲染 `TerminalErrorView` → user 卡 force-quit only 屏幕。

review 引用：

> [P2] Preserve retry path for non-API bootstrap errors — RootView.swift:229-232
> If `GuestLoginUseCase.execute()` throws a non-`APIError` such as the existing `KeychainError` from guest UID/token reads or writes, this catch now wraps it with `AppErrorMapper.presentation(for:)`. The mapper falls back to `.alert` for every non-`APIError`, and bootstrap `.alert` is rendered as `TerminalErrorView`, so affected users are stuck on a force-quit-only screen even though `retry()` used to be available for these failures. This is a regression for transient keychain issues during launch.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | mapper 非 APIError fallback 必须 `.retry`，不是 `.alert` | P2 | error-handling | fix | `iphone/PetApp/Shared/ErrorHandling/AppErrorMapper.swift`, `iphone/PetAppTests/Shared/ErrorHandling/AppErrorMapperTests.swift`, `iphone/PetApp/App/AppLaunchStateMachine.swift` |

## Lesson 1: round 9 二分判则必须**全量**贯彻 — fallback 也是分类决策点，不是"反正 fallback 走 alert"

- **Severity**: P2
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/Shared/ErrorHandling/AppErrorMapper.swift:96-99` (`presentation(for:)` 的 `guard let apiError = error as? APIError else { return .alert(...) }`)

### 症状（Symptom）

bootstrap closure (`RootView.launchStateMachine.bootstrapStep1`) 调 `useCase.execute()`（`GuestLoginUseCase`）失败时：

- 抛 `APIError` → 走 mapper 具体分支 → round 9 二分后分到 `.retry` (network/decoding/unauthorized) 或 `.alert` (missingCredentials/permanent business)。
- 抛 `KeychainError`（如 `case .osStatus(-25300, "get")` —— keychain 里真没找到 token，或 sandbox 临时不可用）→ 走 mapper fallback `.alert("操作失败", "请稍后重试")` → `BootstrapMappedError(presentation: .alert, underlying: KeychainError)` 抛回状态机 → RootView 把 bootstrap `.alert` 渲染成 `TerminalErrorView`（无按钮，user 必须 force-quit）。

实际 `KeychainError` 的故障域**大多 transient**（sandbox 抽风 / item 暂时找不到 / Apple 内部 Security framework 偶尔抽风），跟 `.network` / `.decoding` 同档 —— 走 `.alert` force-quit 是过度悲观。

### 根因（Root cause）

round 9 钦定 transient/terminal 二分判则时，把 mapper 的具体 case 都按二分判则审了（`.unauthorized` / `.decoding` 改 `.retry`，`.missingCredentials` 保 `.alert`），**但漏审 fallback 分支**。fallback 分支被当成"non-classification 的 dump"——"非 APIError 我们也不知道是什么，反正 fallback 走 alert 安全"。

但"fallback 走 alert 安全"这条 mental model 跟 round 9 二分判则**直接冲突**：判则的核心是"transient possible → `.retry`，因为误判 `.alert` 的代价 (force-quit) 远大于误判 `.retry` 的代价 (多发一次请求)"。fallback 也是"无法判定子集"的场景，按判则**应**默认 `.retry` 而非 `.alert`。

更具体：bootstrap closure 抛出的 non-APIError 来源**有限且可枚举**：
- `KeychainError`（`GuestLoginUseCase` 读/写 keychain 失败）—— 大多 transient
- 其他可能抛的 LocalizedError —— 大多 transient

落地端没有"non-APIError 是 permanent" 的 case，但 fallback 默认 `.alert` 让所有这些场景都被 force-quit，是把 round 9 判则只贯彻到一半。

### 修复（Fix）

```swift
// before (round 9)
public static func presentation(for error: Error) -> ErrorPresentation {
    guard let apiError = error as? APIError else {
        return ErrorPresentation.alert(title: "操作失败", message: "请稍后重试")
    }
    ...

// after (round 10)
public static func presentation(for error: Error) -> ErrorPresentation {
    guard let apiError = error as? APIError else {
        return ErrorPresentation.retry(message: "操作失败，请重试")
    }
    ...
```

同步：
- 测试 `testGenericErrorMapsToFallbackAlert` → 改名 `testGenericErrorMapsToFallbackRetry`，断言从 `.alert(title: "操作失败", message: "请稍后重试")` 改 `.retry(message: "操作失败，请重试")`。
- 测试 `testUserFacingMessageForGenericErrorMatchesFallbackCopy` 文案断言从 `"请稍后重试"` 改 `"操作失败，请重试"`。
- mapper 文件头 + `presentation(for:)` 的 doc comment 加 round 10 fix 注释 + lesson 链接，更新二分判则表 `**fallback (.retry)**`。
- `AppLaunchStateMachine.presentationFor(error:)` 注释里"那会把任意 plain Error 都判成 `.alert(...)`" 旧描述更新成"哪怕 round 10 后 mapper fallback 已统一 .retry, 本函数仍优先 LocalizedError 路径以保留 errorDescription 可读性"。

修复后的 mapper 完整分类表：

| Error 类 | round 9 | round 10 |
|---|---|---|
| `.unauthorized` | `.retry` | `.retry` |
| `.decoding` | `.retry` | `.retry` |
| `.missingCredentials` | `.alert` | `.alert` |
| `.business(transient: 1005/1007/1008/1009)` | `.retry` | `.retry` |
| `.business(其他 permanent)` | `.alert` | `.alert` |
| `.network` | `.retry` | `.retry` |
| **非 APIError fallback** | **`.alert`** | **`.retry`** |

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**为某个 mapper / classifier 钦定"transient/terminal 通用判则"时**，**必须**把判则**全量**贯彻到所有分支（包括 fallback / default / unknown 分支），不能让 fallback 漂出判则之外当"安全 dump"。
>
> **展开**：
> - **fallback 是分类决策点，不是"non-classification dump"**：fallback 处理的是"无法判定子集"的输入，但"无法判定"≠"按 alert 处理是安全的"。按 round 9 判则，"无法判定"应该默认 `.retry`（误判代价低）—— 跟 `.network` / `.decoding` 同精神。
> - **判则迭代时必须建全量决策表**：round 9 改完 `.unauthorized` / `.decoding` 后，本应顺手出一份"所有 mapper 分支按二分判则的归类表"对照审计；漏 fallback 是因为 review 没把 fallback 框进 framing 里。规则：每次钦定判则改动，列**包含 fallback / default / unknown 在内**的全量分类表。
> - **non-APIError 来源是有限可枚举的**：bootstrap 路径下能抛 non-APIError 的 use case 数量有限（本 story 是 `GuestLoginUseCase` 抛 `KeychainError`），可以**逐一审 transient/terminal 子集**，不必"反正不知道就走 alert 兜底"。
> - **bootstrap mapper 路径 + force-quit-only TerminalErrorView 是放大镜**：mapper 的 `.alert` 在 bootstrap 路径会被 RootView 渲染成"无按钮静态全屏"，错判一格代价立即放大成"user 不能继续用 App"。所以**任何**会进 bootstrap 路径的 error 都必须按"force-quit only 是否真合理" 检查 —— fallback 也不例外。
> - **反例**：mapper 改 `.unauthorized` / `.decoding` 时只改 review framing 框定的两个 case，不顺手审 fallback / default —— 下一轮 review 必抓出 fallback regression（本轮 round 10 就是这条反例的实证）。
> - **反例 (mental model)**：把"fallback 走 alert"当成"安全保守"—— 在"force-quit only" 渲染层下这恰恰是激进选择。"transient possible → retry" 才是真保守（误判代价低）。

---

## Meta: 本次 review 的宏观教训

round 10 是 round 9 二分判则的"贯彻校验"。round 9 lesson 已经写明"判则的文档化是必需品"和"每次新增 APIError case 按此判则定归类"，但**未提示 fallback 也是分类决策点**。本轮把这条补齐：判则贯彻必须 cover 所有分支，包括 fallback / default / unknown 分支 —— 否则 fallback 就是"没被判则覆盖的死角"，下一轮 review 几乎必然抓出。

跟 round 9 lesson 2 (meta) 的延伸：判则要"全量审计 + 全量贯彻"，单看"review 框定的 case"是 framing trap。本轮 fix 用一条短 patch + 一条 lesson 把 round 9 判则的最后一格补齐。
