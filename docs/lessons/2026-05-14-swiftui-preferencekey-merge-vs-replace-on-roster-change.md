---
date: 2026-05-14
source_review: /tmp/epic-loop-review-18-4-r1.md (codex review, round 1)
story: 18-4-接收-emoji-received-在对应成员猫上方播放飞出动效
commit: 682e6de
lesson_count: 2
---

# Review Lessons — 2026-05-14 — SwiftUI PreferenceKey merge vs replace & transient queue owner-side expire

## 背景

Story 18.4 dev-story 实装"接收 emoji.received 在对应成员猫上方播放飞出动效"链路：

- `RoomScaffoldView` 通过 `MemberAnchorPreferenceKey` 收集每个成员 `PetSpriteView` 的中心点，存到 `@State memberAnchors: [String: CGPoint]`
- `EmojiAnimationLayer` 从 `memberAnchors[userId]` 取 anchor；miss → centerAnchor（V1 §12.3 行 2473 (c) 契约）
- `RealRoomViewModel.applyEmojiReceived` 入队 `activeEmojis` 后启动 1.5s Task 自动 `removeAll`（与 `FloatingEmojiCellView` 1.5s `withAnimation` 对齐）

Codex round 1 review 标出 2 个 [P2] 后续路径上的"半生命周期"漏洞 —— 都是"upstream 数据/事件已经走完一段，但 owner 端没把对应资源清掉"的同源问题。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `memberAnchors` PreferenceKey 用 merge 模式保留离开成员的 stale anchor | P2 / medium | architecture (correctness) | fix | `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift:108-110` |
| 2 | `onEmojiSelected` 入队 `activeEmojis` 后**永不**移除（只有 `applyEmojiReceived` 加了 1.5s expire） | P2 / medium | perf / a11y | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:1190+` |

## Lesson 1: SwiftUI PreferenceKey + 多源订阅时, `onPreferenceChange` 必须替换式赋值（不 merge）

- **Severity**: P2 / medium
- **Category**: architecture (correctness)
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift:108-110`

### 症状（Symptom）

`member.left` 事件先于该 user 的某条 `emoji.received` 到达时（V1 §12.3 行 2473 钦定的合法 race），`EmojiAnimationLayer` 仍能从 `memberAnchors[userId]` 取到该成员**离开前**的最后一帧 anchor 坐标，让 emoji 从"幽灵位"飞出，破坏 V1 §12.3 (c) "roster miss → centerAnchor 降级"契约。

### 根因（Root cause）

旧实装：

```swift
.onPreferenceChange(MemberAnchorPreferenceKey.self) { dict in
    memberAnchors.merge(dict) { _, new in new }
}
```

**思维误区**：把"PreferenceKey 合并 reduce"和"`@State` 字典累积"混为一谈。

- **PreferenceKey.reduce** 在**单帧** body 重渲染过程中把多个 child 视图发出的 value 合并成一个 dict —— 在那一帧内 merge 是正确的（reduce 闭包里就该写 merge）。
- **`onPreferenceChange` 回调**接收到的 dict 是**完整的当前帧聚合结果** —— 即"本帧所有还在渲染的 `memberRow` 报告的 anchor 的并集"。已 leave 的成员当帧不会 emit preference，所以**当帧的 dict 不含其 userId**。

旧实装把这两层语义合在一起，用了第二层 merge，结果"上一帧存活的 anchor"被永久保留在 `@State` 里。

### 修复（Fix）

```swift
.onPreferenceChange(MemberAnchorPreferenceKey.self) { dict in
    // 替换式赋值, 不 merge:
    // 每帧 dict = 当前所有 memberRow emit 的并集 (PreferenceKey.reduce 已合并);
    // 直接整 dict 覆盖 @State → 离开成员的 userId 自动消失 → anchor lookup miss → 走 centerAnchor 降级.
    memberAnchors = dict
}
```

PreferenceKey 自身的 `reduce` 保持原状（**那一层** merge 是 SwiftUI per-frame 子视图聚合契约，必须 merge）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：在 SwiftUI 用 `PreferenceKey + @State + onPreferenceChange` 收集**有 lifecycle 的子视图状态**（典型如成员列表、动态可消失的 row）时，`onPreferenceChange` 回调里**必须用替换式赋值**，**绝不**用 merge —— 否则离场子视图的旧值会被永久保留在 `@State` 里。
>
> **展开**：
> - SwiftUI PreferenceKey 有两层 reduce 语义：
>   1. **`PreferenceKey.reduce`**：单帧内多个 child 视图发出的 value 合并 → 这里 merge 是必须的，因为同帧多个子视图各自报告一份 partial value
>   2. **`onPreferenceChange` 回调**：收到的是**当帧完整聚合结果**；child 视图已离场则其 value **不在** dict 中
> - 这两层是叠加关系而非选择关系。`@State` 持久化的语义由你怎么在 callback 内更新决定 —— 想"snapshot of current frame"就**赋值**；想"history union of all frames"才 merge（极少见的场景，比如手动 undo stack）。
> - **反例**：把 PreferenceKey 当作"事件流"而不是"快照流"对待 —— 任何 `@State 旧值.merge(新值) { _, new in new }` 模式都是高度可疑信号，意味着"离场子视图的状态会被永久保留"，需要进一步 audit。
> - 同精神适用于 `AnchorPreferenceKey` / 用 `GeometryReader → preference` 上报几何信息的所有场景。
> - Audit 启发：如果一段 SwiftUI 代码同时出现 `@State 字典` + `.onPreferenceChange { dict in ... }` + `.merge(`，**立刻**看下游会不会读 stale key —— 多半就是 bug。

## Lesson 2: Transient view-state 队列的 expire 路径必须每个**入队点**都挂 expire Task（不依赖 view 端 fade）

- **Severity**: P2 / medium
- **Category**: perf / a11y
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:1190+` (`onEmojiSelected`)

### 症状（Symptom）

`onEmojiSelected` (self path) 入队 `activeEmojis` 但**永不**移除条目；只有 `applyEmojiReceived` (remote path) 加了 1.5s `Task { sleep → removeAll }`。结果：

- 用户连续 send N 次 emoji 后，`activeEmojis.count` 累积到 N（条目永不出队）
- `FloatingEmojiCellView` 走完 1.5s `withAnimation .opacity → 0` 后视觉看不见，但 SwiftUI `ForEach` 仍持有该 view + a11y 节点
- 副作用：a11y leaf 数量随 send 次数线性增长（污染 VoiceOver 导览次序），SwiftUI body diff 工作量也线性增长

### 根因（Root cause）

**思维误区**：以为 `withAnimation(.easeOut(duration: 1.5)) { animatedOpacity = 0 }` "完成了 cleanup" —— 但这只是**视觉**完成，**数据**owner（vm 的 `activeEmojis`）没动。

dev-story 实装时只在新加的 `applyEmojiReceived` 路径里加了 1.5s expire Task；而 Story 18.3 已经存在的 `onEmojiSelected` 路径没碰 —— review 时容易漏掉。但两条路径的**数据归属点**都是 `vm.activeEmojis`，它们的 lifecycle 必须等价。

### 修复（Fix）

在 `onEmojiSelected` Step A 入队后，挂同款 expire Task（与 `applyEmojiReceived` 行为对齐）：

```swift
self.activeEmojis.append(emoji)

// self path 1.5s expire (与 applyEmojiReceived 同款; 数据 owner = vm, 同 owner 同 lifecycle).
let capturedSelfEmojiId = emoji.id
Task { [weak self] in
    try? await Task.sleep(nanoseconds: 1_500_000_000)
    await MainActor.run {
        self?.activeEmojis.removeAll { $0.id == capturedSelfEmojiId }
    }
}
```

测试覆盖：`test_realOnEmojiSelected_autoExpiresLocalEmojiAfter15Seconds`（mirror `test_realApplyEmojiReceived_autoExpiresAfter15Seconds` 的 1.7s 缓冲断言）。

**注**：未抽 helper 是因为两路径上下文不同（self path 后面还要走 Step B/C catalog + WS send，applyEmojiReceived 后面没有），统一抽出反而需要 callback 形参，得不偿失。保留两份近似代码 + 同一 lesson 比抽 helper 更清晰。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 `@Published` 数组承担"transient UI queue"角色（典型如表情飞出、toast 队列、抖动 trigger）时，**每个 append 入队点**都必须**同步**挂 1.5s/动画时长 的 Task 自动 `removeAll`；**禁止**只在新增路径加而旧路径不加 —— **数据**owner 的 lifecycle 与**视觉**fade 是两层。
>
> **展开**：
> - SwiftUI `withAnimation { animatedOpacity = 0 }` 完成后**只是视觉不可见**，`@State` / `@Published` 数据条目仍在 ForEach data source 里（除非数据 owner 显式移除）
> - 视觉 fade 时长（1.5s）= 数据 expire 时长 —— 两者必须 1:1 对齐；任何一边落后都是 bug
> - 入队点 N 个 → expire Task 也得 N 个；不能只挂 1 个全局清理 timer（race + 多窗口入队会乱）
> - 改善结构：可以把"入队 + 1.5s expire"抽 helper `enqueueTransient(_:)`；但只在 owner（vm）层抽，不要拉到 view 层（view 层无法持有 timer state 安全释放）
> - **反例 A**：新增路径加了 expire 但已有路径漏改 —— review 时只看新增 diff 容易遗漏，**必须**全局 grep `activeEmojis.append` (或同类 mutation 关键词) 看每个写入点是否都挂了 expire Task
> - **反例 B**：把 expire 任务塞到 `FloatingEmojiCellView.onAppear` 内的 `Task { sleep; onExpire?() }` —— 视图层不持数据 owner 的引用，需要回调 callback，破坏"数据 owner 是 vm"的 ADR-0010 §3.2 钦定
> - **反例 C**：依赖 `.transition + .animation` 让 SwiftUI 自动从 ForEach 里移除 → 不可靠（需要 view identity 配合 + 不是所有视图都触发）；显式 `vm.activeEmojis.removeAll` 才是 SoT 一致的写法

---

## Meta: 本次 review 的宏观教训

两条 finding 在表面看属于"forgot to clean up"，但根因都是同一思维漏洞：

**"上游事件结束 ≠ 下游 owner 资源回收"。**

- Finding 1：`member.left` 上游事件 → row 离场 → preference 不再 emit；但 `@State memberAnchors` owner 没主动 evict
- Finding 2：`withAnimation.opacity → 0` 上游视觉完成 → fade 不可见；但 `@Published activeEmojis` owner 没主动 removeAll

**Rule of thumb**：在 SwiftUI / Combine / Async 任何"上游 event-driven 下游"链路里，**显式标记每个 stateful 数据的 evict 路径**，**不**默认依赖上游"自然消失"。审查时核对：

- 每个 `@State / @Published` 数据 owner，是否有**显式**清空/移除路径？
- 该路径**每个**写入点都触发了对应清空吗？还是只有"新加路径"做了？
- 视觉层（animation / fade / 离场）的"完成"是否被错当成数据层的"清理完成"？

这条 meta 反过来也解释了为什么 dev-story 容易在已有代码上漏改：实装者注意力放在"新增功能"，"已有路径需要同步加 expire"这种 cross-cutting concern 容易在 review round 1 才被外部视角抓到。
