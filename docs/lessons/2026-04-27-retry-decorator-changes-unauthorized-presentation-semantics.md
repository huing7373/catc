---
date: 2026-04-27
source_review: codex review round 5 (file: /tmp/epic-loop-review-5-4-r5.md)
story: 5-4-无效-token-静默重新登录
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-27 — Retry decorator 上线后，原 `.unauthorized` 文案的语义会反转 — 必须同步审计所有 user-visible mapping

## 背景

Story 2.6 引入 `AppErrorMapper`，当时业务层还没有任何静默重登装饰器，`.unauthorized` 的语义就是"server 第一次返 401，App 此时尚未做任何恢复尝试" —— 配 toast "登录已过期，正在重新登录..." 是合理的（暗示后台正在恢复）。

Story 5.4 落地 `AuthRetryingAPIClient` 后，业务层能接到的 `.unauthorized` 的语义已经反转：装饰器已经在 wrap 层把"第一次 401" 静默吞掉 + 自动 relogin + 自动重试一次。能从 wrap 层逃出来的 `.unauthorized` 必然是 **"已经 exhaust 唯一一次静默重登尝试"** 的场景（relogin 本身失败 / retry-after-relogin 仍是 401）。原 toast 文案两条都不再成立 ——

1. **谎言**：实际没有 relogin 在跑，"正在重新登录..." 是误导
2. **非 recoverable**：toast 2s 自动消失，用户没有任何 action point；下一次同 generation 的 401 又会被 generation dedup 短路返回旧 token，再失败又 toast，形成 user-perceivable loop

但原 `presentation(for:)` 的 `.unauthorized` 分支没有被同步审计，留在了原文案 —— 直到 round 5 codex 才发现。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Surface exhausted auth recovery as a blocking error | P1 | error-handling | fix | `iphone/PetApp/Shared/ErrorHandling/AppErrorMapper.swift:42-46` |

## Lesson 1: Retry decorator 改变 `.unauthorized` 的 user-visible 语义后，所有相关文案必须同步审计

- **Severity**: P1
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/Shared/ErrorHandling/AppErrorMapper.swift:42-46`

### 症状（Symptom）

Story 5.4 完成后业务层接到 `.unauthorized` 的所有路径都会被 mapping 成 toast `"登录已过期，正在重新登录..."`，但此时：

1. 装饰器内已 exhaust 唯一一次静默重登尝试（relogin 失败 / retry 后仍 401）—— 实际并没有"正在重新登录"
2. toast 2s 自动消失，用户没有 action point
3. 用户后续动作再触发同 generation 的 401，会被 `SilentReloginCoordinator` 的 generation dedup 短路返回**旧** token，再失败再 toast —— 形成纯感知层面的死循环

Story 5.4 spec 钦定的预期是："只有重登本身失败 / 重试后仍是 401 时才走 ErrorPresenter 显示 RetryView"。当前 mapping 既不满足"显示 RetryView"，也不满足任何 blocking error 语义。

### 根因（Root cause）

引入 `AuthRetryingAPIClient` 的 PR 只考虑了"装饰器层逻辑是否正确"，没有审计**这个变化对所有调用方的 user-visible 错误分类的语义影响**。具体说：

- 旧契约下：`.unauthorized` = "server 第一次返 401，App 还没尝试自救" → toast "正在重新登录" 是 hint 后台动作
- 新契约下：`.unauthorized` = "App 已自救一次但失败，需要 user-visible 兜底" → 必须是 blocking error

`AppErrorMapper.presentation(for:)` 的所有 case 都是基于"`.unauthorized` 的语义"做的文案选择 —— 装饰器改了语义但没改文案，等于装饰器引入了一个隐式的"语义/文案 mismatch"。这种 cross-layer 文案错配在单文件 diff 里看不出来，必须做全文搜 + 跨 layer 语义审计。

### 修复（Fix）

`AppErrorMapper.presentation(for:)` 的 `.unauthorized` 分支：toast 改 alert，文案改 "登录失败，请重新启动应用"，与 `.missingCredentials` 的处理对齐（两者都是"App 已自救但需要 cold-start 接手"）。

```swift
// Before
case .unauthorized:
    return ErrorPresentation.toast(message: "登录已过期，正在重新登录...")

// After
case .unauthorized:
    // AuthRetryingAPIClient 上线后,业务层接到的 .unauthorized 必然是"已 exhaust
    // 静默重登"的场景 —— blocking alert + "请重启应用" 与 .missingCredentials 对齐.
    return ErrorPresentation.alert(title: "提示", message: "登录失败，请重新启动应用")
```

文档级修订：在 mapper 的 doc-comment 顶部解释新语义，避免下次有人按旧契约改回 toast。同步更新 `AppErrorMapperTests.testUnauthorizedMapsToToast` → `testUnauthorizedMapsToAlertWithRestartHint`。

**为什么不用 `.retry`** —— 虽然 spec line 11 表面上要求 RetryView：

1. `.retry` 在 `ErrorPresenter` 里依赖 caller 注入 `onRetry` 闭包；当前 mapper 调用方（业务层 `presenter.present(error)`）大多数路径**没有**注入 onRetry —— RetryView 的"重试"按钮只会 dismiss 不会真的重发请求，UX 比 alert 更差
2. 即使 caller 注入了 onRetry，重试发起的同 endpoint 请求会被 `SilentReloginCoordinator` 的 generation dedup 短路返回同一个失效 token，再次走到同一 `.unauthorized` —— 不解决问题
3. cold-start 是更"实质性"的恢复路径（重启 → AppContainer 走 GuestLoginUseCase → keychain.guestUid 可能仍能换出新 token；如果 guestUid 也失效，会被 GuestLoginUseCase 接管走完整新建流程）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**给某个 error case 引入 retry decorator / wrapper / 自动恢复中间件**时，**必须**审计所有把这个 error case 映射到 user-visible 文案的代码（grep `case .<errorCase>`），重新评估文案是否仍然成立 —— 因为 decorator 把这个 case 的"用户实际感受到的语义"反转了。
>
> **展开**：
> - decorator 改的是"何时这个 error 还能继续 propagate" —— 凡是能 propagate 上来的，必然属于"decorator 已尝试自救但失败"的子集，文案应当反映"已尝试 + 失败 + 需用户介入"
> - 对应的 mapping 文案至少要满足：(a) 不撒谎（不暗示"系统正在做什么"，因为系统**已**做完了）；(b) recoverable（给用户明确 action：重启 / 重试 / 联系客服 / 等等）
> - 这类审计应当作为 decorator PR 的 **explicit checklist item**，不能依赖 reviewer 自己想到
> - **反例**：本次本来犯的就是反例 —— 引入 `AuthRetryingAPIClient` 时只改了网络层 + Coordinator + 单测，没动 `AppErrorMapper.swift` 的 `.unauthorized` 分支。文案与新语义错配长达 4 轮 review 没被发现，因为 mapper 单测仍然通过（断言匹配的是旧文案）—— 单测没有"语义反转"的检测能力，必须靠人/LLM 显式审计

## Meta: 本次 review 的宏观教训

不只 `.unauthorized` 这种"接口边界附近"的错误，**任何引入 cross-layer recovery / retry / fallback 机制的 PR，都要把"上层文案与新语义是否一致"作为 explicit AC**。这是 SDK / framework 类改动跨 module 文案一致性的通病：单点改动 + 多点引用 = mapping 漂移。
