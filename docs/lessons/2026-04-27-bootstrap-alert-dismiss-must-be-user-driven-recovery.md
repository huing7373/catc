---
date: 2026-04-27
source_review: codex review (epic-loop round 7) — /tmp/epic-loop-review-5-5-r7.md
story: 5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-27 — Bootstrap alert dismiss 必须 user-driven recovery (禁 exit(0)) & alert 文案 4 轮迭代史防 regress

## 背景

Story 5.5 codex review round 7。本 story 已经历 6 轮 fix-review。本轮 review 仅 1 条 finding（[P1]），但触及一个**已经迭代 4 轮仍走偏的核心点**：bootstrap 路径下 `.alert` presentation 的 dismiss 行为。

历史轨迹：
- **round 0** (dev-story 默认)：`.needsAuth` → 自动 retry → P2 finding（隐式 retry，不可控）
- **round 3** fix：用 `ErrorPresentation` 区分 alert/retry，但 `.alert` dismiss **仍** retry → P1 死循环（mapper 文案 "请重启应用"，dismiss 立即重试 → 仍失败 → 同 alert 弹回 → 死循环）
- **round 4** fix：alert dismiss 改 no-op → P2 卡死（用户点 OK → state 不变 → AlertOverlayView 永久显示，只能 force-quit）
- **round 5** fix：alert dismiss 改 `exit(0)` → P1（**本轮**）iOS HIG 反模式（App Store 审核会拒；用户感觉是 force-quit / crash）
- **round 7** fix（本轮）：alert dismiss 改回调 `stateMachine.retry()`（user-driven recovery） + mapper 文案补 "持续失败时请杀进程重启 App" 让 user 主动决定

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | bootstrap alert dismiss 不能 `exit(0)`（iOS HIG 反模式） | P1 | error-handling / ui | fix | `iphone/PetApp/App/RootView.swift:473-490`, `iphone/PetApp/Shared/ErrorHandling/AppErrorMapper.swift` |
| 2 | meta：alert dismiss 4 轮 fix-review 都引入新 regression 的元根因 | meta | architecture | meta | — |

## Lesson 1: Bootstrap 路径的 terminal-class alert dismiss 必须是 user-driven recovery（禁 `exit(0)` / no-op / 隐式 retry）

- **Severity**: P1
- **Category**: error-handling / ui
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift:473-490` + `iphone/PetApp/Shared/ErrorHandling/AppErrorMapper.swift`

### 症状（Symptom）

冷启动 bootstrap 失败映射到 `.alert(title, message)` 时，AlertOverlayView 唯一的 OK 按钮（"知道了"）调 `exit(0)` 强制终止进程。在 iOS 上这等价于 force-quit / crash：用户不感觉自己点了 "OK 退出"，而是感觉 app 崩溃了。App Store 审核 Guideline 4.0（Design）+ HIG 明确禁止 app 自己终止进程，会拒审。

### 根因（Root cause）

迭代失误链条 —— round 5 选 `exit(0)` 是为了"让 OK 按钮有真实动作"（round 4 no-op 卡死的反弹），但**忽略了一条 iOS 层的硬约束**：iOS app **不能**主动终止进程。`exit(0)` 在 macOS / Linux / 命令行场景是合理的"用户确认退出 → 关闭"，在 iOS UI 流程里**永远**是反模式。

更深层根因：**round 0 → round 5 都把 "alert dismiss 该做什么" 当成 "在 retry / no-op / exit(0) 三选一"，而忽略了第四条路径**：让 user 主动决定该怎么办。三选一里没有正确答案：
- 隐式 retry → 死循环（用户不知道自己被自动重试）
- no-op → 卡死（用户点完无反应）
- exit(0) → iOS HIG 反模式（用户感知 force-quit）

第四条路径是 **"alert dismiss → retry()，但 mapper 文案明确告知 user 多次失败时该 force-quit"**。这把"是否死循环"的判断权交给 user：
- user 点 OK → retry 成功 → 走出去（第一次失败可能是 transient）
- user 点 OK → retry 失败 → 看到同 alert → 知道该自己关 App（文案明示）

这是 iOS HIG 推荐的 **"non-terminating recovery"** 模式：app 永远不主动退出，让 user 通过系统手势（上滑 / Home）退出。

### 修复（Fix）

**两处改动协同，缺一不可**：

1. `RootView.swift` `needsAuthContent(for:)` 的 `.alert` case：
   - **before**：`onDismiss: { exit(0) }`
   - **after**：`onDismiss: { Task { await stateMachine.retry() } }`

2. `AppErrorMapper.swift` 所有 `.alert` 类 mapping 的文案末尾追加 force-quit fallback 提示：
   - `.unauthorized`：`"登录失败，请重新启动应用"` → `"登录失败，请重试。持续失败时请杀进程重启 App"`
   - `.decoding`：`"数据异常，请稍后重试"` → `"数据异常，请重试。持续失败时请杀进程重启 App"`
   - `.business(permanent code)`：`"{userMessage}"` → `"{userMessage}。持续失败时请杀进程重启 App"`
   - `.missingCredentials`：保持 `"登录信息丢失，请重启 App"`（这条 retry 救不回，文案直接钦定 force-quit，不加 "请重试" 前缀）

**对应测试更新**：
- `AppErrorMapperTests.swift`：所有 `.alert` 文案断言更新为带 suffix 的新文案
- `AppLaunchStateMachineTests.swift` case#15：从 "alert dismiss 不能调 retry" 翻转为 "alert dismiss 必须调 retry，且 retry 失败后 state 仍稳定回 .alert（同文案）"
- `ErrorPresenterTests.swift`：alert 文案断言更新

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **iOS app 任何 UI 路径** 中处理 unrecoverable 错误时，**禁止**调用 `exit(0)` / `abort()` / `fatalError()` 之类终止进程的 API；必须用 **user-driven recovery 模式**（dismiss → retry，文案明示 "持续失败时请杀进程重启 App"）。
>
> **展开**：
> - **iOS HIG 硬约束**：iOS app 不能自己终止进程。`exit(0)` 在 iOS 上的语义是 force-quit（用户感知 = crash），App Store 审核 Guideline 4.0 会拒审。这条约束在 macOS / 命令行场景**不**适用 —— Swift 跨平台代码 review 时要分平台判断。
> - **terminal-class 错误的标准 UX 模板**：用户看到 alert → 点 OK → retry 一次 → 成功就走出去 / 失败就看到同 alert + 文案 → 知道该自己 force-quit（系统手势）。"是否死循环" 的判断权交给 user，**不要** app 替 user 决定。
> - **配套契约**：alert dismiss 调 retry()，则 mapper 钦定的 alert 文案**必须**显式告知 user "持续失败时请杀进程重启 App" —— 否则 user 不知道多次重试无效时该怎么办，又退化成 round 3 的死循环（自动隐式重试，user 蒙圈）。
> - **特例**：如果错误是 "retry 也救不回" 类（如 `.missingCredentials` —— keychain 真没 token，repo 仍抛同样错误），文案直接钦定 force-quit（"请重启 App"），不加 "请重试" 前缀；alert dismiss 调 retry() 是无害的（也只是再次落到同 state，AlertOverlayView 仍可见）。
> - **反例 1**：`onDismiss: { exit(0) }` —— iOS HIG 反模式（round 5 反例）。
> - **反例 2**：`onDismiss: { /* no-op */ }` —— OK 按钮无反应，user 卡死，体验比死循环还差（round 4 反例）。
> - **反例 3**：`onDismiss: { Task { await stateMachine.retry() } }` 但 mapper 文案是 "请重启应用" —— 文案与行为矛盾，user 看到 "请重启" 但点 OK 后又自动重试（round 3 反例）。
> - **正例**：`onDismiss: { Task { await stateMachine.retry() } }` + mapper 文案 "请重试。持续失败时请杀进程重启 App"（round 7 正解）。
> - **平台检查清单**：写 SwiftUI / UIKit 代码时，看到 `exit(`、`abort(`、`fatalError(` 关键字必须立即 review —— 这些在 iOS 用户路径上几乎永远是反模式。仅 `fatalError` 在"程序员意图保证不可达 + 触发即代表逻辑 bug"的内部断言场景可用。

## Lesson 2 (Meta): 同一争议点 4 轮 fix-review 都引入新 regression —— 决策树没穷举导致

- **Severity**: meta
- **Category**: architecture / process
- **分诊**: meta
- **位置**: 跨多 round fix history（见背景）

### 症状（Symptom）

Story 5.5 的 "alert dismiss 行为" 争议点经历了 4 轮 fix-review 后才收敛到正解。每轮都解决了上轮的 regression 但引入新 regression：
- round 0 → round 3：发现"自动 retry 死循环" → 改成"alert dismiss 仍 retry"（**没解决问题**，只是把决策从 dev-story 默认搬到 fix-review，行为相同）
- round 3 → round 4：发现"alert dismiss retry 死循环" → 改 no-op（**矫枉过正**：从"自动循环"摆到"完全卡死"）
- round 4 → round 5：发现"no-op 卡死" → 改 exit(0)（**触发新约束**：不知道 iOS HIG 禁 exit）
- round 5 → round 7（本轮）：发现"exit(0) iOS HIG 反模式" → 改 user-driven retry + 文案补 fallback 指令（**收敛**）

### 根因（Root cause）

每轮 fix 只看了"上一轮坏在哪"+ "怎么消除上一轮的坏"，**没穷举该决策点的全部可能选项 + 各自的 trade-off**。"alert dismiss 该调什么" 实际只有 4 个选项：

| 选项 | 行为 | 结果 |
|---|---|---|
| 隐式 retry | 自动重试 | 用户被动死循环（round 0/3 反例） |
| no-op | 无反应 | 用户卡死（round 4 反例） |
| exit(0) | 终止进程 | iOS HIG 反模式（round 5 反例） |
| user-driven retry + 文案明示 fallback | retry，文案告知多次失败时杀进程 | **正解**（round 7） |

**前 3 轮都把第 4 个选项遗漏了**，因为：
- 没把 mapper 文案当成"决策的一部分"（只调代码，不调文案）
- 没意识到 "把判断权交给 user" 是合法第三态（不是 retry / 不是不 retry）
- 决策时没列穷举表格

### 修复（Fix）

不修代码，修流程。

**预防规则（Rule for future Claude）⚡**：

> **一句话**：当一个争议点经历 ≥2 轮 fix-review 仍走偏时，**必须停下来**列出该决策点的**穷举选项表格**（含每个选项的预期行为 + trade-off + 已知反例），从表格里挑解，**不要**只针对"上一轮坏在哪"做局部修补。
>
> **展开**：
> - **触发条件**：同一文件 / 同一 finding 主题在 fix-review history 出现 ≥2 次（review 文件名带 `r3` / `r4` 等递增后缀都是 signal）
> - **必做动作**：写一个 markdown 表格，列出该决策点的全部技术选项（不只是"修法"，还包括"完全不修"、"换路径修"、"改契约修"），每个选项标注：
>   - 预期行为（具体到 UI / API 层面）
>   - 已知 trade-off（性能 / 安全 / UX / 平台约束）
>   - 哪轮 review 已尝试过 + 结果
> - **挑选标准**：选**没被尝试过 + 全部 trade-off 可接受**的选项；如果所有选项都试过，重新审视上一层假设（mapper 输出语义？AC 文案？平台约束理解错了？）
> - **本次反例归档**：alert dismiss 4 个选项里前 3 个都尝试了才到第 4 个，因为第 4 个需要"同时改代码 + 改文案"（跨两个文件 + 两类 artifact），decision space 没被列出来 → Claude 默认按 "只改代码" 范围搜索 → 永远找不到第 4 个选项。
> - **反例**：round 5 fix-review 只看 round 4 留下的"no-op 卡死"，没问"还有没有其他选项"，直接跳到 exit(0) —— 触发未知的平台约束（iOS HIG 禁 exit）。

## Meta: 本次 review 的宏观教训

本次 review 单条 P1 finding 把 "iOS HIG 知识" 与 "决策迭代失控" 两个根因同时暴露。前者是**领域知识缺口**（写 iOS 代码必须知道 exit(0) 是禁忌），后者是**流程漏洞**（同一争议点反复迭代时缺乏穷举决策机制）。

未来在 iOS 路径写错误处理代码时，触发这两条 lesson 同时复习：
1. 看到 `exit(` / `abort(` / `fatalError(` → 红灯，replace 成 user-driven recovery
2. 看到同一文件 ≥2 轮 fix-review review 报告 → 强制列穷举表格再挑
