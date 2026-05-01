---
date: 2026-04-30
source_review: codex review (round 2) — file: /tmp/epic-loop-review-37-7-r2.md
story: 37-7-homeview-scaffold
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-30 — SwiftUI `onChange(of:)` Equatable 重放契约 + `Task.sleep` 重置 timer 必须 cancel

## 背景

Story 37.7（HomeView Scaffold）round 2 review。Round 1 的 [P2] fix 已落，整体 scaffold + a11y 都过，但 codex 发现两个**真用户可见**的交互 bug：都在 `interactionAnimation: AnimationState` 这条状态链上，分别是 enum Equatable 设计没考虑"同 emoji 连点"语义、以及 `Task.sleep` 异步 timer 没 cancel 导致 rapid tap race。两条都触及 SwiftUI 的两个反直觉默认行为（onChange 只看 Equatable diff、Task cancel 不中断 sleep）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 同 emoji 连点不重放动画（onChange 看不到 value 变） | P2 / medium | architecture | fix | `iphone/PetApp/Features/Home/Models/AnimationState.swift` + 4 caller |
| 2 | rapid tap stale reset timer 提前清动画 | P2 / medium | architecture | fix | `iphone/PetApp/Features/Home/Views/HomeView.swift:87-95` |

## Lesson 1: SwiftUI `onChange(of:)` 只看 Equatable diff —— "同值" 触发不会被感知

- **Severity**: medium（P2，user-visible 但非 crash）
- **Category**: architecture（SwiftUI state shape 设计契约）
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/Models/AnimationState.swift`（enum 定义）+ `RealHomeViewModel.swift:63-75` / `MockHomeViewModel.swift:53-69`（caller）

### 症状（Symptom）

`AnimationState` 是 `enum { idle; flying(String) }` + Equatable 默认实装。用户连点 Feed → 第二次 `interactionAnimation = .flying("🍥")` 与第一次相同 emoji 字符串 → SwiftUI Equatable 比较两次 value 相等 → `onChange(of: state.interactionAnimation)` 不触发 → 第二次点击没有视觉反馈，UI 看起来"卡住"或"按钮坏了"。

### 根因（Root cause）

SwiftUI `onChange(of:)` 的契约**仅基于 Equatable diff**：新值 != 旧值才触发闭包。设计 `AnimationState` 时只考虑了"状态语义"维度（idle vs flying；emoji 是哪个），没考虑"事件维度"（同语义事件再次发生应当被识别为新事件）。这是个**类别错误**：把"持续状态"（state）和"瞬时事件"（event）塞进同一 enum 但只用 state 语义做 Equatable，导致事件维度丢失。

类似陷阱：
- `@Published var loadingState: LoadingState` 设 `.error("network")` 两次相同字符串 → onChange 静默
- `@Published var toastMessage: String?` 同一文案触发两次 toast → 第二次不显示
- 任何 `onChange(of: someEnum)` 驱动"动画 / 副作用 / 事件回放"语义但 enum 用 default Equatable

### 修复（Fix）

option A（推荐，最干净）：给 `.flying` case 加 `id: UUID` 字段，每次 onTap 用 `UUID()` 新实例。default Equatable 自动比较所有关联值 → id 不同就视为不等，无须自定义 Equatable。

before:
```swift
public enum AnimationState: Equatable {
    case idle
    case flying(String)
}
// caller: self.interactionAnimation = .flying("🍥")
```

after:
```swift
public enum AnimationState: Equatable {
    case idle
    case flying(emoji: String, id: UUID)
}
// caller: self.interactionAnimation = .flying(emoji: "🍥", id: UUID())
```

影响范围（grep `\.flying\(` 全仓库）：4 个 caller（Real / Mock 各 3 个 + tests 解构）+ 1 个 view 解构（`if case let .flying(emoji, _)`）。新增 2 个守护测试：
1. `testRapidSameEmojiTapsProduceDistinctAnimationStates` —— 连点同 emoji 两次断言两次 state 不 Equatable
2. `testAnimationStateFlyingEquatabilityRequiresMatchingId` —— 直接构造两个固定不同 UUID 的 .flying 断言不等

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在设计**驱动动画 / 副作用 / 事件重放**的 SwiftUI state（enum 或 struct）时，**必须**让"同语义事件再次发生"在 Equatable 维度上**被感知为不等**（要么加 `UUID` token，要么加单调递增 tick / timestamp）。
>
> **展开**：
> - 触发条件：`onChange(of: <state>) { ... }` 闭包里做的事是 "spawn animation / show toast / play sound / replay event"，而非纯"状态切换响应"。
> - 必做动作：给 state 加一个 per-event 唯一 token（推荐 `id: UUID`，每次 mutate 时新 `UUID()`）。**不要**指望 caller 在事件间手动 `state = .idle` 来制造 diff —— 同 RunLoop tick 内的连续赋值会被 SwiftUI 合并，只看到最后一个值。
> - 写测试守护这个契约：构造两个**值语义相同但 token 不同**的 state，断言 Equatable 不等。这条测试是 option A 修法的活契约 —— 防止未来重构者"简化"成默认 Equatable + 删 token。
> - **反例 1**（事件错当 state）：`enum LoadingState { case idle; case error(String) }`，连续两次 `.error("network")` → onChange 不触发；toast 只显示一次。
> - **反例 2**（依赖 nil-then-value 制造 diff）：`self.x = nil; self.x = .flying("🍥")` —— 两次赋值同 RunLoop tick 内合并，SwiftUI 只 publish 最后一个值。**这条不行**。
> - **反例 3**（用 hash 当 id）：`id = emoji.hashValue` —— 同 emoji 同 hash → 还是踩坑。必须 per-event 全新随机 token。

## Lesson 2: `Task.sleep` 重置 timer 必须 cancel 上一个 —— rapid action 会 race

- **Severity**: medium（P2，user-visible 时序错乱但非 crash）
- **Category**: architecture（async timer lifecycle）
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/Views/HomeView.swift:87-95`（onChange 闭包）

### 症状（Symptom）

`onChange(of: state.interactionAnimation)` 里每次 `.flying` 都 spawn 一个 `Task { try? await Task.sleep(1.4s); state.interactionAnimation = .idle }`。如果用户 t=0 点 Feed → t=0.5s 点 Play：
- t=1.4s 第一个 Task 醒来 → 把 `.idle` 写入 → **第二个 emoji 提前消失**（应当持续到 t=1.9s）
- 旧 task 句柄从未保存 → 没法 cancel → race 不可避免

### 根因（Root cause）

两个嵌套陷阱：
1. **Task 句柄丢弃**：`Task { ... }` 表达式返回值被忽略 → 没有句柄就没法 cancel → "fire and forget" 看起来简洁但语义错。
2. **`Task.cancel()` 不中断 `Task.sleep`**：cancel 只是**标记** `Task.isCancelled = true`，sleep 仍跑完。所以即使保存了句柄并 cancel，task body 里也必须主动 `if Task.isCancelled { return }` 才能拒写 stale state。
3. SwiftUI `.animation(.easeOut(duration: 1.4), value:)` 看起来"自带 1.4s 时长"会让人误以为 SwiftUI 会自动管理重置，但**它只管视觉过渡**，state 重置必须 ViewModel 或 View 显式做。

### 修复（Fix）

before:
```swift
.onChange(of: state.interactionAnimation) { _, newValue in
    guard case .flying = newValue else { return }
    Task { @MainActor [weak state] in
        try? await Task.sleep(nanoseconds: 1_400_000_000)
        state?.interactionAnimation = .idle
    }
}
```

after:
```swift
@State private var resetTask: Task<Void, Never>?

.onChange(of: state.interactionAnimation) { _, newValue in
    guard case .flying = newValue else { return }
    resetTask?.cancel()                    // 取消上一个 stale timer
    resetTask = Task { @MainActor [weak state] in
        try? await Task.sleep(nanoseconds: 1_400_000_000)
        if Task.isCancelled { return }     // sleep 期间被 cancel 的 race 防护
        state?.interactionAnimation = .idle
    }
}
```

要点：
- 句柄存在 `@State` 而非 ViewModel —— 这是 View 层的视觉 timer，VM 不该管
- `if Task.isCancelled { return }` 必须放在 sleep **之后**，因为 cancel 不打断 sleep
- 使用 `[weak state]` 避免 View 销毁后的 retain cycle / dangling write（如果 ViewModel 已被释放）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 SwiftUI 里用 `Task { try? await Task.sleep(...); state.x = ... }` 做"延时重置 / debounce / one-shot timer"时，**必须**把 task 句柄存在 `@State`、新触发前 `cancel()` 旧的、task body 里 sleep **之后**用 `if Task.isCancelled { return }` 拒写。
>
> **展开**：
> - 触发条件：在 `onChange` / `onAppear` / 按钮闭包等 reactive 闭包里 spawn 一个会写状态的 sleep task，且**同一回调可能被快速重复触发**（rapid tap / 连续状态切换）。
> - 必做动作三件套：①`@State var t: Task<Void, Never>?` 持句柄 ②新触发前 `t?.cancel()` ③task body 在 sleep 之后 `if Task.isCancelled { return }` 才 mutate state。三件**缺一不可**——只缺①没法 cancel；只缺②每次都漏；只缺③ stale write 仍发生。
> - **反例 1**（fire-and-forget）：`Task { try? await Task.sleep(...); state.x = .idle }` —— 没句柄，rapid trigger 必 race。
> - **反例 2**（cancel 但忘 isCancelled check）：`t?.cancel(); t = Task { try? await Task.sleep(...); state.x = .idle }` —— cancel 不打断 sleep，stale task 仍写入。
> - **反例 3**（用 `.animation(...)` 误以为 state 自动重置）：SwiftUI 的 `.animation(value:)` 只管 view 层视觉过渡，state 字段不会自动 reset —— 需要显式 ViewModel 或 onChange timer。
> - 替代方案：如果 timer 语义是"取消最新一个" (debounce) 而非"取消上一个" (latest-wins) → 也是同样三件套；只是 cancel 时机相反（新触发**不**新建 task，只重置截止时间）。这种场景考虑用 Combine `.debounce(for:)` 而非裸 `Task.sleep` —— Combine operator 内置 cancel 语义。

---

## Meta: 本次 review 的宏观教训

两条 finding 都不是"代码写错"，而是**SwiftUI / Swift Concurrency 的两个反直觉默认行为**让看起来"显然正确"的代码在 rapid input 下崩坏：
- `onChange` 只看 Equatable —— 同值不触发（Lesson 1）
- `Task.cancel` 不打断 sleep —— 必须 `isCancelled` 主动 check（Lesson 2）

共同根：**reactive UI + async task 的语义边界要写测试守护**。本次新增的两个守护 case（`testRapidSameEmojiTaps...` + `testAnimationStateFlyingEquatabilityRequiresMatchingId`）就是把"option A 设计契约"固化成测试。Lesson 2 的句柄/cancel 三件套虽然**没**直接加 unit test（涉及时间 & SwiftUI @State，单测代价高），但 lesson 文档本身就是 future Claude 的 "回归测试"。

未来在 Story 14.x（WS pet.state.changed 真实状态切换）实装时，会再次面对同样的语义判断 —— 服务端推一次"喂食成功"事件，客户端是否要重放动画？答案是 **要**，所以 .flying(emoji, id) 设计已经为那个 story 准备好了：让 service 层把 server event id 当作 token 直接传进来即可（不需要客户端 UUID()）。
