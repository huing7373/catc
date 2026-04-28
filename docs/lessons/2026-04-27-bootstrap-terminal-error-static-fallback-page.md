---
date: 2026-04-27
source_review: codex review (epic-loop round 8) — /tmp/epic-loop-review-5-5-r8.md
story: 5-5-loadhomeusecase-主界面用-get-home-一次拉取全部数据
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-27 — Bootstrap terminal error 必须用静态全屏 fallback page，禁用任何 dismiss-able overlay & alert dismiss 5 轮迭代史的元根因复盘

## 背景

Story 5.5 codex review round 8。本 story 已经历 7 轮 fix-review（baseline → review 6 → review 7 → review 8）。本轮 review 仅 1 条 finding（[P1]），但触及一个**已经迭代 5 轮仍走偏的核心 UX 决策**：bootstrap 路径下 `.alert` presentation 的 dismiss 行为。

**5 轮 dismiss 行为迭代史**（每轮都被下一轮 review 推翻）：

| Round | Dismiss 行为 | Review 结论 |
|---|---|---|
| 0 (dev-story) | 默认 .needsAuth → 自动 retry | P2: 隐式重试，不可控 |
| 3 fix | 区分 alert/retry，alert dismiss 仍 retry | P1: 死循环（mapper 文案"请重启应用"，dismiss 立即重试 → 仍失败 → 同 alert 弹回） |
| 4 fix | alert dismiss 改 no-op | P2: 卡死（user 点 OK 后 state 不变 → AlertOverlayView 永久显示） |
| 5 fix | alert dismiss 改 `exit(0)` | P1: iOS HIG 反模式（App Store 审核拒；user 感觉是 force-quit / crash） |
| 7 fix | alert dismiss → `stateMachine.retry()` + mapper 文案补"持续失败时请杀进程重启 App" | **P1（本轮 round 8）**: user 仍可被困 retry → fail → retry 循环；`.missingCredentials` 这种 retry 根本无效的 case 文案提示也救不了 |
| **8 fix（本轮 — 终极方案）** | 引入 **TerminalErrorView** 全屏静态 page（**无任何按钮**），bootstrap 路径 `.alert` / `.toast` 不再用 `AlertOverlayView` | — |

本 lesson 包含两条核心教训：(1) bootstrap terminal-class error 必须用静态全屏 fallback page；(2) meta：5 轮迭代史背后的元根因（dismiss-able overlay × terminal error 是个伪命题）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | bootstrap 路径 terminal-class error 必须用静态全屏 fallback page（无按钮 force-quit only） | P1 | error-handling / ui / architecture | fix | `iphone/PetApp/Core/DesignSystem/Components/TerminalErrorView.swift` (新增), `iphone/PetApp/App/RootView.swift`, `iphone/PetApp/Shared/ErrorHandling/AppErrorMapper.swift` |
| 2 | meta：alert dismiss 5 轮 fix-review 都引入新 regression 的元根因（dismiss-able overlay × terminal error 是伪命题） | meta | architecture / process | meta | — |

## Lesson 1: Bootstrap 路径 terminal-class error 必须用静态全屏 fallback page，禁用任何 dismiss-able overlay

- **Severity**: P1
- **Category**: error-handling / ui / architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/App/RootView.swift:434-498`（`needsAuthContent(for:)` 三态分发），`iphone/PetApp/Core/DesignSystem/Components/TerminalErrorView.swift`（新文件），`iphone/PetApp/Shared/ErrorHandling/AppErrorMapper.swift`（文案回归简洁）

### 症状（Symptom）

Round 7 fix 让 bootstrap `.alert` presentation 的 OK 按钮调用 `stateMachine.retry()`，并在 mapper 文案末尾追加"持续失败时请杀进程重启 App"，期望靠"user-driven recovery + 文案指引"打破死循环。Round 8 review 指出该方案仍不安全：

- `.unauthorized` / `.missingCredentials` / `.decoding` / permanent business error 等 mapper 钦定为 `.alert` 的错误都是 **terminal-class**（client 端任何重试都救不回）
- user 点 OK → retry → 同样的 closure → 同样的 error → 同 alert 弹回 → user 又被困 retry/fail loop
- 特别 `.missingCredentials` 是本地 keychain 没 token，retry guest-login 仍走同一路径抛同样 error，文案提示也救不了
- 所谓"user 看到文案知道该 force-quit"是单方面假设：user 在 retry 失败几次后才会读完文案，已经踩过 N 次 server roundtrip

### 根因（Root cause）

**5 轮迭代揭示的根本矛盾**：`AlertOverlayView` 是 dismiss-able overlay 设计 → 必须有 OK 按钮 → 必须有 dismiss closure → closure 选什么动作都跟 terminal 错误的语义冲突：

| Dismiss 动作 | Terminal 语义冲突 |
|---|---|
| 自动 retry | 死循环（重试无效 → 同错误 → 同 alert 弹回） |
| no-op | UI 卡死（user 点 OK 没反应） |
| `exit(0)` | iOS HIG 反模式（App Store 审核拒；user 感觉是 crash） |
| `stateMachine.retry()` + 文案提示 | 仍是死循环，只是把"决策权"伪装给 user |

根因是**"在 dismiss-able overlay 上设计 terminal error 处理"本身就是个伪命题**。Terminal error 的语义是 "in-app 任何操作都救不回"，但 dismiss-able overlay 的语义是 "提供一个 user 可点的退场路径让 UI 流转继续"。两个语义直接冲突，5 轮 patch 都是在试图调和这个根本不可调和的冲突。

唯一的解：**不给 dismiss 入口**。iOS error boundary 模式 = full-screen static page = user 必须 OS 级 force-quit。

### 修复（Fix）

**1. 新增 `TerminalErrorView`（`iphone/PetApp/Core/DesignSystem/Components/TerminalErrorView.swift`）**：

```swift
public struct TerminalErrorView: View {
    public let title: String
    public let message: String
    // 关键: 无 onDismiss / onRetry closure 字段 —— 这条契约就是 "no dismiss = no loop"
    
    public var body: some View {
        VStack(spacing: 24) {
            Image(systemName: "exclamationmark.triangle.fill") ...
            Text(title) ...
            Text(message) ...
            // 底部静态指引文本 (非按钮): 告知 user 唯一退路
            Text("请双击 Home 键 / 上滑底部小条杀进程后重新启动 App")
                .font(.caption).foregroundColor(.gray)
        }
        // 全屏 + systemBackground
    }
}
```

故意不做按钮 —— 一旦做按钮（无论调 retry / no-op / exit / 提示）都会回到 round 3-7 的 dismiss 行为问题域。静态文本避免引诱 user 点击 → 强制 user 走 OS 级 force-quit。

**2. RootView 改路由**：bootstrap 路径 `.alert` / `.toast` 不再用 `AlertOverlayView`，改用 `TerminalErrorView`：

```swift
// before (round 7):
case let .alert(title, message):
    AlertOverlayView(title: title, message: message,
                     onDismiss: { Task { await stateMachine.retry() } })  // ← 死循环

// after (round 8):
case let .alert(title, message):
    TerminalErrorView(title: title, message: message)  // ← 无 closure
```

`AlertOverlayView` **保留**给非 bootstrap 路径用（`ErrorPresenter` 管理的 transient business error 那条线，dismiss-able overlay 在那里语义合适）。

**3. mapper 文案回归 round 5 简洁形态**：去掉 round 7 引入的"持续失败时请杀进程重启 App" suffix（该指引已 move 到 `TerminalErrorView` 底部静态文本，mapper 文案专注表达"什么错了" 而非"怎么办"）：

| Error case | round 7 文案 | round 8 文案 |
|---|---|---|
| `.unauthorized` | "登录失败，请重试。持续失败时请杀进程重启 App" | "登录失败，请重新启动应用" |
| `.decoding` | "数据异常，请重试。持续失败时请杀进程重启 App" | "数据异常，请稍后重试" |
| permanent business | "{userMessage}。持续失败时请杀进程重启 App" | "{userMessage}" |
| `.missingCredentials` | "登录信息丢失，请重启 App"（不变） | 不变 |

**4. 新增测试**：
- `ErrorComponentSnapshotTests.testTerminalErrorViewHoldsTitleAndMessageOnly` — 编译期保证 `TerminalErrorView.init` 签名只有 title + message，未来若有人加 closure 字段编译期 fail
- `ErrorComponentSnapshotTests.testTerminalErrorViewRendersVariousErrorCopy` — 覆盖 .unauthorized / .missingCredentials / .decoding 文案
- `AppLaunchStateMachineTests.testStateMachinePresentsAlertForTerminalErrorAndRetryStaysIdempotent` — 重写 round 7 case#15：state 机仍保留 .alert presentation invariant，retry() API 自身仍 idempotent，但不再 simulate "user 点 OK"（TerminalErrorView 没按钮）
- mapper / presenter 测试同步更新到新简洁文案

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **iOS / macOS bootstrap 路径处理 terminal-class error**（rety 救不回的错误）时，**禁止使用任何 dismiss-able overlay**（`.alert(...)` modal / `Alert` SwiftUI / 自定义 dismiss-able view）；**必须**渲染静态全屏 fallback page，无任何按钮，user 唯一退路是 OS 级 force-quit。
>
> **展开**：
> - **触发条件**：bootstrap / startup / cold-start 路径（user 还没进入主流程，没有"上一屏"可退回）+ terminal-class error（`.unauthorized` / `.missingCredentials` / `.decoding` / 永久 business error / 任何 client 重试无法自愈的错误）
> - **必须**：渲染全屏静态 view，含 (1) 错误图标 (2) 错误标题 (3) 错误正文 (4) 引导 user 杀进程的静态文本（不是按钮）。systemBackground + ZStack 占满整屏。
> - **禁止**：在 terminal error 路径上挂任何 OK / 重试 / 退出 / 知道了 button。一旦有 button 就有 closure，closure 选什么动作都跟 terminal 语义冲突（详见本 lesson 5 轮迭代史）。
> - **禁止**：在 app 内调 `exit(0)` / `abort()` / `fatalError()` 终止进程。iOS HIG 明确禁止 app 自杀，App Store 审核会拒。让 user 主动杀。
> - **禁止**：用 toast/snackbar 兜底 terminal error。Toast 自动消失后留白屏比错误 UI 更糟。
> - **依然允许**：transient error（network / business 1005/1007/1008/1009）走 `RetryView`（user 可点重试自愈）；非 bootstrap 路径的 business error 走 `AlertOverlayView`（dismiss-able overlay 在有"上一屏"可退回的语境下语义合适）
> - **反例 1（round 3 死循环）**：`AlertOverlayView(onDismiss: { stateMachine.retry() })` —— terminal error 的 retry 必死循环
> - **反例 2（round 4 卡死）**：`AlertOverlayView(onDismiss: { /* no-op */ })` —— UI 卡死无法离开
> - **反例 3（round 5 iOS HIG 拒）**：`AlertOverlayView(onDismiss: { exit(0) })` —— App Store 审核拒，用户感觉是 crash
> - **反例 4（round 7 死循环复现）**：`AlertOverlayView(onDismiss: { Task { await stateMachine.retry() } })` + 文案"持续失败时杀进程" —— user 仍被困 retry/fail loop，文案提示是单方面假设
> - **正例（round 8 终极）**：`TerminalErrorView(title:, message:)` 无 closure → user 必须 OS 级 force-quit。文案 + 底部静态指引清晰告知唯一退路。
> - **架构提示**：把 `TerminalErrorView` / `AlertOverlayView` / `RetryView` 三组件视为不同 error severity 的不同 UI 模式，而非"通用 alert 的不同变种"。它们的 init 签名差异（terminal 无 closure / alert 有 onDismiss / retry 有 onRetry）就是各自语义的编译期契约 —— 未来若想给 `TerminalErrorView` 加 closure 字段，编译期单测会 fail（这是有意设计，防 regress）。

## Lesson 2 (Meta): Alert dismiss 5 轮 fix-review 都引入新 regression 的元根因 — dismiss-able overlay × terminal error 是个伪命题

- **Severity**: meta
- **Category**: architecture / process
- **分诊**: meta
- **位置**: 整个 Story 5.5 review 周期（round 0/3/4/5/7/8）

### 症状（Symptom）

同一条 `bootstrap .alert dismiss 行为` 在 5 轮 fix-review 中：
- round 3: dismiss → retry → P1 死循环
- round 4: dismiss → no-op → P2 卡死
- round 5: dismiss → exit(0) → P1 iOS HIG 拒
- round 7: dismiss → retry + 文案提示 → P1 死循环复现

每轮 fix 都解了上一轮 review 提出的具体问题，但又引入了下一个 review 会拒绝的新问题。5 轮迭代下来在四种"dismiss 动作"间循环切换（retry → no-op → exit → retry...），始终找不到一个能同时满足所有 review constraint 的方案。

### 根因（Root cause）

**元根因不在任何具体 dismiss 动作的对错，而在"dismiss-able overlay 用作 terminal error UI"这个 UI mode 选择本身就是错的**。

`AlertOverlayView` 是 dismiss-able overlay 模式 → 必须暴露 dismiss closure。Terminal error 的语义是"in-app 任何操作都救不回"。两者本质冲突：dismiss closure 必须做"某个动作"，但 terminal error 的所有可能动作都是错的：

- 做 in-app 操作（retry）→ 救不回 terminal error → 死循环
- 不做动作（no-op）→ UI 流转停止 → 卡死
- 终止 app（exit）→ iOS HIG 禁止 → 审核拒
- 让 user 选（retry + 文案）→ user 做的"选择"仍是上面三选一 → 仍踩同样坑

5 轮 fix 都是在四种错误动作之间做"哪个最不糟"的选择，没人质疑过 "dismiss-able overlay 这个 UI mode 是不是适合 terminal error" 这个上层问题。直到 round 8 跳出 dismiss 选择题，引入 `TerminalErrorView` 这个**没有 dismiss closure 的全新 UI mode**，问题才消失。

### 修复（Fix）

不修代码 —— 这是 meta lesson，记录给未来 Claude 的元方法论：

**当一个具体 fix 在多轮 review 中反复被否，应该跳出"具体 fix 选择题"，质疑这个问题的整个 framing 本身**。

具体到本次：
- 错的 framing："alert dismiss closure 该做什么动作？"
- 对的 framing："terminal error 该不该用 dismiss-able overlay？"

跳出 framing 后才能看到：dismiss-able overlay × terminal error 是 product/design 层面就该被识别的不匹配，不该让 implementation 层去硬调和。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **同一具体 fix 被 review 推翻 ≥3 次** 时，**必须**停止"在当前 framing 内挑下一个 fix 选项"，**改为质疑当前 framing 本身**：(1) 这个组件 / API / UI mode 是否适合当前问题？ (2) 是否需要引入一个全新的 mode（而非在现有 mode 的参数空间里挑）？
>
> **展开**：
> - **触发条件**：同一 finding（或同一组件的同一行为决策）在 ≥3 轮 review 中反复换方案仍被推翻；每轮 fix 都解了上一轮问题但引入下一轮问题；fix 在几个固定选项之间循环切换（如 retry / no-op / exit / 文案提示）
> - **必须**：写下"当前 framing"是什么（具体到一句话："X 组件的 Y 行为该选 A/B/C/D 哪个？"），然后**主动**质疑这个 framing：是否问题根本不该用 X 组件解？是否 Y 行为本身是个伪命题？
> - **必须**：把 review 的 N 轮 finding 横向放在一起看（不是孤立看每轮）。N 轮否决一起看会暴露元根因（如本案的"5 轮都在 dismiss 动作上打转 → 真问题是 dismiss-able overlay 不该用在 terminal error"）
> - **优先**：引入新 type / 新 component / 新 UI mode 而非在旧 type 上加参数 / case / closure。新 type 可以在编译期就排除旧 type 的反模式（如 `TerminalErrorView` 的 init 签名编译期保证没 closure）
> - **反例**：在 round 5 的 fix 里看到 round 3/4 的 fail 模式后，仍选择"在 dismiss closure 里换个动作"（exit(0)）而不是"换个 UI mode 不要 dismiss closure"。round 7 同模式重蹈覆辙
> - **正例**：在 round 8 fix 里跳出 "dismiss closure 选什么" 的选择题，引入 `TerminalErrorView` 这个**没有 dismiss closure** 的新 UI mode。问题域整个消失，不再有 "dismiss 行为对不对" 的争议
> - **元方法论**：fix-review 循环 ≥3 轮在同一点上打转 = signal "应该 step back，重新 frame 问题"。stack overflow 救不了你；只有跳出当前 framing 才能。

---

## Meta: 本次 review 的宏观教训

本次 review 的核心收获**不是**"alert dismiss 该怎么做"，而是 **"何时该跳出当前 framing"** 的元判断力。

Story 5.5 整个 review 周期共 7 轮，alert dismiss 占了 5 轮。每轮 reviewer（codex）都给出有效 finding，每轮 fixer（previous Claude） 都做了局部正确的 patch，但循环停不下来。直到 round 8 用户在 override 里**直接示范了跳出 framing 的方法**（"5 轮都失败说明 'alert as dismiss-able overlay' 模式根本不适合 terminal error。必须根本重构"）才打破循环。

未来 Claude 应该在 review round 3-4 时就主动做这个 step-back，而不是等 user 来手动喊停。本 lesson 的预防规则 #2 就是把这个元判断力 codify 成"≥3 轮否决 → 必须质疑 framing"的硬规则。

这条 meta lesson 应该是 epic 5 最有价值的设计教训之一 —— 不是"具体怎么写 SwiftUI"，而是"什么时候停止局部 patch、上升到 architectural 决策"。
