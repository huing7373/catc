---
date: 2026-05-14
source_review: codex review round 2 (/tmp/epic-loop-review-18-1-r2.md) — story 18-1-表情面板-swiftui
story: 18-1-表情面板-swiftui
commit: e323183
lesson_count: 1
---

# Review Lessons — 2026-05-14 — Feature ViewModel 自建 mapError 必须镜像 AppErrorMapper 的 transient / terminal 二分（.localStoreFailure ≠ .missingCredentials）

## 背景

Story 18.1 落地 EmojiPanelViewModel 时，开发者为 RetryView 写了局部 `mapError` 而非走全局 `AppErrorMapper`（因为表情面板是局部 RetryView，不进 toast/alert）。在写 switch 穷举时把 `APIError.missingCredentials` 与 `.localStoreFailure` 合并到了同一个 `case .missingCredentials, .localStoreFailure: return "登录已失效，请重启 App"` 分支 —— **把 transient 错误归到 terminal 文案**。

Codex review round 2 [P2]：`AppErrorMapper` 已经在 ADR-0008 v2 §4.2 钦定了二分（`.localStoreFailure` → `.retry`，`.missingCredentials` → `.alert`），feature 层自建 mapper 必须与之镜像，不能合并。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | EmojiPanelViewModel.mapError 把 `.localStoreFailure` 错合并到 `.missingCredentials` 的 terminal 分支 | medium (P2) | error-handling | fix | `iphone/PetApp/Features/Emoji/ViewModels/EmojiPanelViewModel.swift:87-88` |

## Lesson 1: Feature ViewModel 自建 mapError 必须镜像 AppErrorMapper 的 transient / terminal 二分

- **Severity**: medium (P2)
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Emoji/ViewModels/EmojiPanelViewModel.swift:87-88`

### 症状（Symptom）

EmojiPanelViewModel 的 mapError：

```swift
case .missingCredentials, .localStoreFailure:
    return "登录已失效，请重启 App"
```

当 LoadEmojisUseCase 抛出 `APIError.localStoreFailure(underlying: keychainError)`（典型场景：keychain `SecItemCopyMatching` 返 OSStatus -25291 errSecNotAvailable / iOS sandbox 权限抽风 / 进程刚启动 keychain 还没 ready）时，用户看到的是"登录已失效，请重启 App"——一个 terminal 类文案，建议 user **force-quit App**。

但 `.localStoreFailure` 在协议层被明确定义为 **transient**（`APIError.swift` §line 73-92 与 `AppErrorMapper.swift` §line 90-93 双重锚定）—— retry 可能自愈，强制重启 App 是错误的恢复路径（甚至更糟：cold-start 后 keychain 可能仍处于抽风状态，导致 GuestLoginUseCase 同样失败）。

### 根因（Root cause）

写局部 mapper 时把 `APIError` 的 enum case 当成"名字看起来像就归一组"——`.missingCredentials` 和 `.localStoreFailure` 都含"credential / store"字眼，下意识 OR 起来共用文案。**忽略了 case 名字背后的语义二分**：

- `.missingCredentials` = 本地 keychain 读成功 + 确认无 token = **terminal**（cold-start 仍读不到 → retry 无意义）
- `.localStoreFailure` = 本地 keychain.get 抛错 + 未触达 server = **transient**（retry 可能恢复）

`APIError.swift` 在 enum 注释里写了 30+ 行解释这两态的语义差异（甚至特意标注"mapper 把 .missingCredentials 映射到 .alert，把 .localStoreFailure 映射到 .retry"）；`AppErrorMapper.swift` 也是分两个 `case` 给两份不同文案 + 不同 ErrorPresentation。但开发者在 feature ViewModel 局部 mapper 里**没回头读这两份锚定文档**，凭"看起来差不多"合并了 case。

第二层根因：feature ViewModel 局部 mapper 是**手写镜像**，没有 contract test 或编译期校验保证它和 AppErrorMapper 行为一致；只要写错就会 drift。

### 修复（Fix）

`EmojiPanelViewModel.swift` mapError switch 拆出独立分支，与 AppErrorMapper §line 85-93 文案 1:1 对齐：

```swift
case .missingCredentials:
    // terminal: 本地 keychain 确认无 token，重启 App cold-start 走同一份 store 仍读不到，
    // retry 无意义 —— 与 AppErrorMapper §line 85-88 ".alert(登录信息丢失)" 同语义.
    return "登录已失效，请重启 App"
case .localStoreFailure:
    // **transient**: keychain.get 抛错 (sandbox 抽风 / OSStatus -25291 等)，retry 可能自愈.
    // 与 AppErrorMapper §line 90-93 ".retry(登录信息读取异常)" 同语义 —— 不与 .missingCredentials 合并.
    // 依据：APIError.swift §.localStoreFailure 钦定 transient + AppErrorMapper 分支已成定例.
    return "登录信息读取异常，请重试"
```

同时新增两个单测 (`EmojiPanelViewModelTests.swift`)：

- `test_friendlyMessage_localStoreFailure_returnsRetryableMessage` — 防 transient 回归
- `test_friendlyMessage_missingCredentials_returnsTerminalMessage` — 配对锁住 terminal 语义

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **feature 层写局部 errorMapper / friendlyMessage / mapError 时**，**必须** **先打开 `iphone/PetApp/Shared/ErrorHandling/AppErrorMapper.swift` 看每个 `APIError` case 的现有 `ErrorPresentation` 映射，并把每条文案 1:1 镜像（terminal case → 重启文案；transient case → 重试文案）；禁止把任何两条不同 `ErrorPresentation` 类型的 case 用 `case A, B:` 合并到同一返回值**。
>
> **展开**：
> - `APIError` 是六态枚举（`.business / .unauthorized / .missingCredentials / .localStoreFailure / .network / .decoding`），其中 `.missingCredentials` 与 `.localStoreFailure` 是 Story 5.5 round 11 [P2] 专门拆出来的语义对偶（terminal vs transient），名字像但**语义相反**。
> - `AppErrorMapper.swift` 是 ADR-0008 v2 §4.2 钦定的**单一权威 mapper**，任何 feature 层局部 mapper 都是它的"局部投影"——投影的 ErrorPresentation 类型（`.alert` / `.retry` / `.toast`）必须保持，文案可以为 feature 上下文做润色但**不能跨类型合并**。
> - 局部 mapper 写完，必须 grep `AppErrorMapper.presentation(for:)` 的每一条分支，逐条对照自己的 switch；任何一条 case 在 AppErrorMapper 是 `.retry` 而你给了"请重启 App" / "登录已失效"等 terminal 文案 → 立刻是 bug。
> - 单测覆盖**必须包括 transient 类的 case**（典型遗漏：开发者只测了 `.network` / `.decoding`，没测 `.localStoreFailure`，于是写错了也没人发现）。最佳实践是**给每个 `APIError` case 一条专门的 test_friendlyMessage_*\_returns* 测试**，命名时直接把语义意图（retryable / terminal）写在测试名里，让以后 review 一眼能扫到漏案。
> - **反例**：
>   - `case .missingCredentials, .localStoreFailure: return "登录已失效，请重启 App"` — 用 case 合并屏蔽掉了 transient / terminal 的语义差异。
>   - `case .unauthorized, .missingCredentials: return "请重登录"` — 这两条在 AppErrorMapper 都是 `.alert`，看似可合，但 `.unauthorized` 在 AuthBoundaryAPIClient 装饰器下会被静默重登截走，`.missingCredentials` 不会；feature 层文案差异化（前者"会话过期"vs 后者"凭证丢失"）有助于用户诊断。能不合就不合。
>   - 写局部 mapper 时只读 `APIError.swift` 的 case 名而**不读 AppErrorMapper 的映射表** — 默认会按"名字感觉"分组，下游用户被错误文案误导。

---

## Meta: 本次 review 的宏观教训

Story 18.1 这次 round 2 [P2] 与一系列历史 lesson 同根（参见 `2026-04-28-local-store-transient-vs-terminal-must-distinguish.md` / `2026-04-28-decoding-and-unauthorized-must-be-transient-retry.md` / `2026-05-06-ws-table-probe-misconfig-vs-transient-error-classification-r8.md`）：**transient / terminal 二分是 ADR-0008 v2 钦定的 client 端 presentation 契约，但每个 feature 写局部 mapper 时都重复踩同一个坑**——把 transient case 误归到 terminal 文案。

更深层的反思：**手写镜像 mapper 不可靠**。下一次 feature 局部 mapper 出现时，应当考虑提供一个 protocol-shaped helper（如 `AppErrorMapper.userFacingMessage(for:)` 已经做了一半的事）让 feature 层只override "feature 上下文文案"而非重新 switch case；或者用 lint / 单测模板强制每个新 ViewModel 的 mapper 必须对每条 APIError case 都有显式 case（禁止 `case A, B:` 合并）。这是一个跨 epic 的 architectural debt，不在本轮 fix 范围。
