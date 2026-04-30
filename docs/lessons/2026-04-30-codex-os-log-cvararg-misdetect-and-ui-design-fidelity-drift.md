---
date: 2026-04-30
source_review: file:/tmp/epic-loop-review-37-6-r1.md (codex via /epic-loop fix-review round 1)
story: 37-6-shared-primitives
commit: b18c9d5
lesson_count: 3
---

# Review Lessons — 2026-04-30 — codex `os_log CVarArg` 误报 + ui_design FadeIn 方向反转 + Avatar inset shadow 漏实现

## 背景

Story 37.6（iPhone DesignSystem 共享 primitives：Icons / FadeIn / Avatar / RarityTag / PrimaryButton / Card）经 dev-story 实装 + 方案 A sub-agent 修复后进入 review，codex 反馈 3 条 finding（[P0] / [P2] / [P3]）。其中 [P0] 是 codex 在 sandbox xcodebuild 失败后退化到 macOS swiftc 单文件 -typecheck 的误报；[P2] / [P3] 是真 ui_design 契约偏移。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | os_log variadic 不接受 String → 需 CVarArg/NSString bridge | high | testing | wontfix | `iphone/PetApp/Core/DesignSystem/Primitives/Icons.swift:75-79` |
| 2 | FadeIn 起点 offsetY = -8（从上向下落），与 ui_design `translateY(8px)→0`（从下向上升）反向 | medium | style | fix | `iphone/PetApp/Core/DesignSystem/Primitives/FadeIn.swift:22-23` |
| 3 | Avatar `ring == false` 分支漏 `inset 0 -2px 0 rgba(0,0,0,0.08)`，缺底部内向阴影 | low | style | fix | `iphone/PetApp/Core/DesignSystem/Primitives/Avatar.swift:51-54` |

## Lesson 1: codex 用 macOS `swiftc -typecheck` 单文件验证 iOS `os_log`，结论不适用

- **Severity**: high（codex 标 P0；实际验证为误报）
- **Category**: process / tooling-misdetection
- **分诊**: wontfix
- **位置**: `iphone/PetApp/Core/DesignSystem/Primitives/Icons.swift:75-79`

### 症状（Symptom）

codex 报告 `os_log(.error, "...%{public}@...%{public}@", key, fallbackSymbol)` 因 Swift `String` 不 conform `CVarArg` 而无法编译，故"任何 build 包含 Icons.symbol(for:) 都会挂"。

### 根因（Root cause）

codex 在 sandbox 内 xcodebuild 因 `Couldn't create workspace arena folder ... Operation not permitted` 启不动，退化到 `swiftc -typecheck` 单文件验证。但：

1. **`swiftc -typecheck` 默认 macOS target**，不会触发 iOS overlay 里的 `os_log` macro 重载。
2. iOS 的 `os_log` 有 macro 形式 `os_log(_ message: StaticString, ...)`，并通过 `os_log` overlay + Swift string-interpolation 语义自动接受 `String` 参数（无需 `CVarArg` bridge）。
3. **真 `bash iphone/scripts/build.sh --test` 跑 iOS 17 target，257/257（含本轮新增的 1 个 FadeIn 方向 guard 测试）全绿，BUILD SUCCESS**。

故 codex 这条 P0 的技术依据（macOS swiftc 单文件 -typecheck）和项目实际编译路径（iOS 17 target 经 xcodebuild 走完整 module pipeline）不同，结论不适用。

### 修复（Fix）

不修。`Icons.swift:75-79` 保持原 `os_log(.error, "...%{public}@...%{public}@", key, fallbackSymbol)`，已通过 257/257 iOS 17 测试。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 收到 codex / 任何 sub-agent review 标 [P0] "编译错"时，**必须先跑项目实际 build 命令验证**，而不是看 review 文字描述就盲目改代码。
>
> **展开**：
> - codex sandbox 跑 xcodebuild 失败时（`Operation not permitted` / `Couldn't create workspace arena folder` 等 sandbox 报错），它会退化到 `swiftc -typecheck` 单文件验证 → **target 默认 macOS，结论对 iOS 不适用**。
> - iOS `os_log` macro overload 接受 Swift `String`（通过 string-interpolation overlay），**不**需要 `NSString(string:)` / `CVarArg` bridge。
> - **强制流程**：review 标 [P0] 编译错 → 先跑 `bash iphone/scripts/build.sh --test`（或 server `bash scripts/build.sh --test`）→ 通过 → wontfix + lesson；失败 → 按 review 修。
> - **反例**：盲信 codex review 文字把 `key` / `fallbackSymbol` 改成 `NSString(string: key)` / 切到 `Logger(subsystem:...)` 现代 API → 引入 import / API 切换的额外审查面，且根本没必要（旧 API 在 iOS 17 工作正常）。
> - **元规则**：**review 是输入，不是 ground truth**；ground truth 是项目主测试命令的退出码。

## Lesson 2: ui_design 动画方向必须按原 `@keyframes` from→to 语义直译，不能凭 SwiftUI 经验猜

- **Severity**: medium
- **Category**: style / ui-fidelity
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/DesignSystem/Primitives/FadeIn.swift:22-23`

### 症状（Symptom）

`FadeInModifier.body` 的 `.offset(y: visible ? 0 : -8)` 让 view 从 -8（上方 8pt）滑入到 0（原位），即"从上向下落"。但 ui_design `iphone/ui_design/source/screens/home.jsx:101-102` 钦定：

```js
@keyframes fadeIn {
  from { opacity: 0; transform: translateY(8px); }
  to   { opacity: 1; transform: translateY(0); }
}
```

`translateY(8px) → 0` = 起点 +8（**下方** 8pt）→ 终点 0，即"由下向上升起"。Swift 实现方向反转，每个 `.fadeIn(id:)` 调用站点视觉行为与原型相反。

### 根因（Root cause）

1. CSS `transform: translateY(Npx)` 与 SwiftUI `.offset(y: N)` 同向（都是 +y 向下），但 dev 写 Swift 时凭"渐入感觉"猜起点是负数（"从上方落下来"），没回查 ui_design `@keyframes` 实际值。
2. `from { translateY(8px) }` 的语义是"起点在 0 点下方 8pt"，"to translateY(0)"是"终点回归原位"。即"从下向上"。
3. dev 把"FadeIn"和"内容从上方淡入"的 web/app 通用印象绑定，没**逐字**对照 ui_design 源码。
4. 此前实现注释（"渐入 + 上移 8pt"）描述含糊—— "上移"指"运动方向向上"还是"起点偏移在上方"？没引用 ui_design 数值，导致 codex 第一次 review 才抓到。

### 修复（Fix）

`FadeIn.swift`：
- `.offset(y: visible ? 0 : -8)` → `.offset(y: visible ? 0 : 8)`
- 抽出 `static let offsetStartY: CGFloat = 8` / `offsetEndY: CGFloat = 0` 公开契约值（防回归 + 给测试用）
- 文件头注释引用 ui_design `home.jsx:101-102` 的 `@keyframes fadeIn from→to` 数值
- Preview 里的描述文案 `"0.28s easeInOut + offsetY -8 → 0"` → `"0.28s easeInOut + offsetY +8 → 0（由下向上升起）"`

`PrimitivesTests.swift` 加 case#6 `testFadeInOffsetStartIsPositiveEightFromBelow`：
```swift
XCTAssertEqual(FadeInModifier.offsetStartY, 8, "...")
XCTAssertEqual(FadeInModifier.offsetEndY, 0, "...")
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在把 ui_design `@keyframes` / CSS transform 翻成 SwiftUI `.offset` / `.scaleEffect` / `.rotationEffect` 时，**必须**逐字对照 from/to 数值（含正负号），并把数值连同 ui_design 文件路径 + 行号写进注释，**不**凭"动效感觉"或"web 习惯"猜方向。
>
> **展开**：
> - CSS `transform: translateY(Npx)` 和 SwiftUI `.offset(y: N)` 同向（+y 向下）；CSS `translate(-50%, ...)` 是 transform-origin 偏移，不能直翻成 SwiftUI offset，要走 alignment.
> - `@keyframes` 的 `from` 值是**起点**（动画播放第 0 帧时的状态），`to` 是**终点**（动画结束帧）。SwiftUI `withAnimation { visible = true }` 时，`visible == false` 对应 from，`visible == true` 对应 to。
> - **强制审查项**：每个 `.offset(y:)` 起点值必须能在 ui_design 源码 grep 到对应的 `translateY(...)` 数值；不能 grep 到 → 该实现违反 ui_design 契约。
> - **抽出契约常量**：把动效数值抽 `static let` 暴露，让单测能 anchor（不依赖 ViewInspector），既防回归也给后续 visual-review 提供可比对锚。
> - **反例**：实现注释写"渐入 + 上移 8pt"，但没说"起点偏移 +8（下方）→ 0（原位）"——含糊到正负号都没说，无法对账。"上移"既可读为"运动方向向上"也可读为"起点偏移在上方"，二义就是 bug 温床。

## Lesson 3: ui_design `boxShadow` 的 inset / 多层阴影必须显式实现，SwiftUI 没原生 inset shadow（iOS 17）

- **Severity**: low
- **Category**: style / ui-fidelity
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/DesignSystem/Primitives/Avatar.swift:51-54`

### 症状（Symptom）

ui_design `iphone/ui_design/source/components/primitives.jsx:196`：
```js
boxShadow: ring
  ? '0 0 0 3px var(--surface), 0 0 0 5px var(--accent)'
  : 'inset 0 -2px 0 rgba(0,0,0,0.08)',
```

`ring == true` 分支（外双圈描边）已实现；`ring == false` 分支的 **`inset 0 -2px 0 rgba(0,0,0,0.08)`** 漏了。这是底部 2pt 内向暗色阴影，给圆形头像加深度信号；缺它后，所有 default avatar（friends / room 列表大量出现）都是平面圆，视觉差于原型。

### 根因（Root cause）

1. dev 看 `ring ? A : B` ternary 时只翻了 `ring == true` 分支（外圈描边），把 `ring == false` 分支当"无样式"略过，没注意 `B = 'inset 0 -2px 0 ...'` 是**有样式**的 fallback。
2. SwiftUI iOS 17 没有原生 `.innerShadow`（iOS 18 才加），dev 可能下意识当作"做不出来"就跳了。
3. 实测可用 `Circle().fill(LinearGradient(stops:...))` 在底部 15% 区间从透明渐进到 `black.opacity(0.08)` 模拟 inset bottom shadow，结果近似度足够（视觉精度由 Story 37.13 visual-review-checklist 把关）。

### 修复（Fix）

`Avatar.swift` `ring == false` 分支加：
```swift
} else {
    Circle()
        .fill(
            LinearGradient(
                stops: [
                    .init(color: .clear, location: 0.85),
                    .init(color: Color.black.opacity(0.08), location: 1.0),
                ],
                startPoint: .top, endPoint: .bottom
            )
        )
}
```

inline 注释引用 ui_design `primitives.jsx:196` 的 `inset 0 -2px 0 rgba(0,0,0,0.08)` 原始 spec + 解释为什么用 LinearGradient（iOS 17 无原生 innerShadow）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 翻 ui_design CSS `boxShadow` 到 SwiftUI 时，**必须把 ternary 两分支都翻**；遇到 `inset` 阴影且 iOS 17 target，用 `LinearGradient(stops:...)` 在 Circle/RoundedRectangle 内向 fill 模拟；不要因为"没原生 API"就静默跳过 fallback 分支。
>
> **展开**：
> - `boxShadow: cond ? A : B` 这种 CSS ternary 是**双分支**，必须各写一份 SwiftUI 实现；只翻一边是漏实现 = 视觉 drift.
> - SwiftUI iOS 17 无 `.innerShadow`（iOS 18 加）；模拟方法：`Circle().fill(LinearGradient(stops: [.init(color: .clear, location: ...), .init(color: shadowColor, location: 1)], startPoint: ..., endPoint: ...))`，把 stops location 调到与 inset offset 比例匹配（`-2px` on a 44pt circle ≈ bottom 4-15% 区间）。
> - **强制审查项**：grep ui_design CSS `boxShadow:`，每个 ternary 都点开两边 → 对应 SwiftUI 实现里也得有两个分支体（不能 `if ring { ... } else { /* nothing */ }`，除非 ui_design fallback 真为空字符串）。
> - **反例**：把 `inset 0 -2px 0 rgba(0,0,0,0.08)` 这类内向阴影当"装饰可省"略过；其实它是 ui_design designer 给平面圆形加的关键深度信号，省了 = 整个 friends/room 列表的 avatar 视觉降级。

---

## Meta: 本次 review 的宏观教训

3 条 finding 共一个底层 framing 错位：**review 反馈是输入，不是 ground truth**。

- [P0] codex 在 sandbox 受限下退化的判断方法（macOS swiftc -typecheck）和项目真编译路径（iOS xcodebuild）不一致 → 误报。修复 = 跑项目主测试命令复核，不照单全收。
- [P2] / [P3] codex 翻原型对照 ui_design 源码确实抓到 dev 漏实现 → 真 bug，按建议修。

**元规则给未来 Claude 接 fix-review 任务**：

1. **每条 finding 先复现**：定位到代码 + 跑项目 build/test → 自己判断真假。
2. **review 文字 ≠ 事实**：codex / 任何 LLM reviewer 的 [P0] 编译错断言要拿编译器输出复核，[P2] / [P3] 视觉/逻辑 bug 要拿原型源码或运行行为复核。
3. **wontfix 不是甩锅，是诚实**：在 lesson 里把"为什么 review 不成立"写清楚（具体引用代码 / 编译器版本 / 命令），比无脑改代码更有蒸馏价值——未来 Claude 看到同类 codex sandbox 误报时不再被误导。
