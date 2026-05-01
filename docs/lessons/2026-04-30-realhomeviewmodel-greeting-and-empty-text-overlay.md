---
date: 2026-04-30
source_review: codex round 3 review for Story 37.7 (`/tmp/epic-loop-review-37-7-r3.md`)
story: 37-7-homeview-scaffold
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-30 — Real ViewModel 的派生字段必须 override hydrate 入口 & 空 Text overlay 是 VoiceOver 陷阱

## 背景

Story 37.7（HomeView Scaffold）round 3 codex review 抓到 2 个 [P2]：

1. `RealHomeViewModel.greeting` 永远 hardcode `"想你啦 ♥"`，bootstrap 已注入 HomeData
   并 hydrate 到 AppState，但 ViewModel 不读 → 生产用户看不到自己宠物的名字。
2. `homeStatusBar` a11y identifier 通过空 `Text("")` overlay 注入到父级 HStack 之外，
   SwiftUI 把 zero-sized Text 当 focusable accessibility node → VoiceOver 用户滑过
   statusBar 顶部时会卡在空白元素。

两条都 fix。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | RealHomeViewModel.greeting 永远 hardcode 不从 hydrated AppState 派生 | medium | architecture | fix | `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift` |
| 2 | 空 Text overlay 注入 a11y identifier 引发 VoiceOver focusable 空白节点 | medium | a11y / ui | fix | `iphone/PetApp/Features/Home/Views/HomeView.swift`, `iphone/PetApp/Shared/Constants/AccessibilityID.swift` |

## Lesson 1: Real ViewModel 的派生字段必须 override hydrate 入口，不能只设静态 placeholder

- **Severity**: medium
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift:43-50`

### 症状

`RealHomeViewModel(appState:)` 在 `configureMockDefaults()` 里 hardcode `self.greeting = "想你啦 ♥"`，
之后 bootstrap 调 `appState.applyHomeData(data)` 把宠物 hydrate 到 AppState；但
HomeView 渲染的 `state.greeting` 字段从来不被改写 → 生产用户在主界面永远看到 placeholder
字符串，看不到自己的猫名。

### 根因

设计 abstract method base class 时只想到 user 交互入口（onCreateTap / onJoinTap / ...）需要
override，**忘了"派生字段"也需要 override 注入入口** —— `applyHomeData(_:)` 是 base class
里被 caller 集中调用的钩子函数，但 mock 子类只在 `configureMockDefaults()` 一次性写死字段；
real 子类 copy 这种模式，结果 hydrate 路径下静态默认值从不更新。

视觉默认值（mock）和派生字段（real）走同一个赋值入口 `configureMockDefaults()` 是错的：
mock 不需要 hydrate（测试 / Preview 直接构造完成态），real 需要 hydrate 后重新派生。
两者**不能用同一个一次性入口**。

### 修复

`RealHomeViewModel` override `applyHomeData(_:)`：

```swift
public override func applyHomeData(_ data: HomeData) {
    super.applyHomeData(data)
    if let petName = data.pet?.name, !petName.isEmpty {
        self.greeting = "\(petName)，想你啦 ♥"
    } else {
        self.greeting = "想你啦 ♥"
    }
}
```

加守护测试 `testRealHomeViewModelGreetingDerivesFromHydratedPet`：构造 → 断言 placeholder
→ apply 含 `pet.name = "测试猫"` 的 HomeData → 断言 greeting 反映了 pet name；同时验证
`super.applyHomeData(data)` 链路（loadingState 转 `.loaded`）跑了。

### 预防规则（Rule for future Claude） ⚡

> **一句话**：未来 Claude 设计有 abstract method 的 ViewModel base class 时，**必须把 hydrate
> 入口（如 `applyHomeData` / `applyResponse` / `applySnapshot`）也视作可 override 接缝点**，
> Real 子类对**任何派生字段**（greeting / displayName / formattedDate / ...）都必须在
> hydrate 入口里重新计算，不能只在 `configureDefaults()` 里 hardcode 静态 placeholder。
>
> **展开**：
> - 视觉默认值（"placeholder until hydrate"）和派生字段（"reflect domain state"）是**两类
>   赋值**，不能合并到同一个一次性入口（如 `configureMockDefaults()`）；
>   placeholder 走构造入口一次，派生字段走 hydrate 入口每次；
> - Real 子类 override hydrate 入口必须以 `super.<method>(...)` 开头 —— base class
>   通常承担"写状态 + 设短路 flag"双重职责（如 `loadingState = .loaded` + `hasLoadedHome = true`），
>   漏 super 会让短路 flag 不生效；
> - 加守护测试：assert hydrate 前 placeholder + hydrate 后派生字段值对应 hydrated state +
>   super 链路也跑了（断言短路 flag / loadingState 转移）；
> - **反例**：把 greeting hardcode 在 `configureMockDefaults()` + 不 override
>   `applyHomeData`；或 override 但忘了 `super.applyHomeData(data)` 让短路 flag 静默
>   失效；或 override 但只读 `self.appState?.currentPet`（依赖 super 已写完 AppState 的
>   时序假设）—— 改用 `data.pet` 直接读传入参数更稳。

## Lesson 2: SwiftUI 空 Text overlay 注入 a11y identifier 是 VoiceOver 陷阱

- **Severity**: medium
- **Category**: a11y / ui
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/Views/HomeView.swift:144-150`（旧），
  `iphone/PetApp/Shared/Constants/AccessibilityID.swift:13`

### 症状

为同时满足新旧两条 UITest（一条用 `AccessibilityID.Home.userInfo` = `"home_userInfo"`，
另一条用字面量 `"homeStatusBar"`），代码采用「父级挂老 identifier + overlay 内挂空
`Text("").frame(width: 0, height: 0).accessibilityIdentifier("homeStatusBar")` 注入新
identifier」双锚共存。SwiftUI 把空 Text 视为 focusable accessibility node →
VoiceOver 用户滑过 statusBar 顶部时会卡在零宽零高的空白元素，不能直接跳到子元素。

### 根因

误以为「让 a11y identifier 可定位」 = 「往 accessibility tree 加节点」。实际上：
- SwiftUI 一个 view element 只能有一个 `.accessibilityIdentifier`（同一节点重复调用后写覆盖前写）；
- "为多条 UITest 共存" 不该靠**新建空节点**实现 —— 空节点会污染 a11y tree，对 VoiceOver
  用户产生可感知的卡顿；
- 正确解法是**改 enum 常量的物理值**：让 `AccessibilityID.Home.userInfo` 的值从
  `"home_userInfo"` 改为 `"homeStatusBar"`，老 caller 因为是用 enum 引用，无 source 改动；
  新 caller 因为字面量直接命中 `"homeStatusBar"`，也无改动。

### 修复

1. `AccessibilityID.Home.userInfo` 值从 `"home_userInfo"` 改为 `"homeStatusBar"`
   （所有老 caller 通过 enum 引用零改动迁移）；
2. HomeView statusBar 删除 `.overlay(Text("").accessibilityIdentifier(...))` 整段，
   `.accessibilityIdentifier(AccessibilityID.Home.userInfo)` 直接挂在父 HStack；
3. `HomeViewTests.testAccessibilityIdentifierNamingFollowsFeatureUnderscoreElement`
   命名约定测试：拆分 snakeCase（`home_xxx`，`petArea` 等 7 个）和 camelCase（`homeXxx`，
   `userInfo` 1 个），用两段断言分别验证。

### 预防规则（Rule for future Claude） ⚡

> **一句话**：未来 Claude 在 SwiftUI 中需要给一个 view 挂多个 a11y identifier 时，**禁止**
> 通过空 `Text("")` / 空 `EmptyView()` overlay 注入第二个 identifier；要么改物理值让单
> identifier 同时满足多条测试，要么用单 identifier + a11y label 二元验证身份。
>
> **展开**：
> - SwiftUI accessibility tree 不去重 zero-sized node —— 空 Text 会被 VoiceOver 视为
>   focusable element（除非显式 `.accessibilityHidden(true)`，但这又会让 XCUITest 找不到，
>   两难）；
> - 一个 element 一个 identifier 是 SwiftUI 的硬约定（`.accessibilityIdentifier`
>   后写覆盖前写）；想兼容老 / 新两条 UITest 的字符串差异，**改 enum 常量物理值**是
>   最干净的路径（caller 用 enum 引用 → 自动迁移，caller 用字面量 → 已经命中新值）；
> - 验证身份信息（如 nickname）用 `.accessibilityLabel(Text(value))` + UITest 断
>   `element.label == value`，**不要**通过额外的 a11y identifier 节点承载身份；
> - **反例**：父级挂 identifier A + `.overlay(Text("").accessibilityIdentifier(B))` 注入 B；
>   或父级挂 identifier A + `.background(Color.clear.accessibilityIdentifier(B))` 同病；
>   或 try `.accessibilityHidden(true)` 让空节点不 focusable —— 但这同时让 XCUITest
>   `descendants(matching:)` 找不到，新 UITest 立刻挂。

---

## Meta: 本次 review 的宏观教训

两条 finding 单独看是独立缺陷，但有共同的"**让所有约束在同一个 sink 上汇流**"思维漏洞：

- [P2-A] 把 placeholder 默认值和派生字段写值合并到同一个 sink（`configureMockDefaults()`），
  导致 hydrate 后派生字段不更新；
- [P2-B] 想让一个 view 同时承载两个 identifier 而不是改物理值让单 identifier 兼容多条测试，
  导致空 Text 污染 a11y tree。

**抽象规则**：当多条独立的语义都落在同一个 SwiftUI / ViewModel sink 上时，先问"它们语义
真的同源吗？还是只是凑巧都需要赋值"。前者合并合理，后者拆分到不同入口（构造入口 vs hydrate
入口；enum 常量物理值 vs 多 a11y 节点）。
