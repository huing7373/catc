---
date: 2026-04-30
source_review: codex review --uncommitted (epic-loop round 1) → /tmp/epic-loop-review-37-4-r1.md
story: 37-4-appstate-实装-loadhome-迁移
commit: 8c9d991
lesson_count: 1
---

# Review Lessons — 2026-04-30 — 构造注入参数 + weak 存储字段在 fresh-instance 调用路径下的语义陷阱

## 背景

Story 37.4（HomeViewModel 迁 LoadHome 到 AppState）实装时，`HomeViewModel` 新增 `private weak var appState: AppState?`，并给三个 init 各追加 `appState: AppState? = nil` 默认参数。dev-story 的 design intent 是「生产路径 RootView @StateObject 同时 strong 持 AppState 与 HomeViewModel，HomeViewModel 内部 weak 防循环引用」。codex round 1 review 揭示 init 路径与 weak 存储语义不兼容：caller 用 `HomeViewModel(appState: AppState())` 这种 fresh instance 而无外部 strong owner 时，weak 立刻释放，`applyHomeData` / `loadHome` 静默 fail，"构造注入"路径名存实亡。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | weak appState 与构造注入参数语义冲突 | medium (P2) | architecture | fix (option A：改 strong) | `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift:122` |

## Lesson 1: 构造注入的字段不该 weak —— 注入语义就是「ViewModel 是否对该实例的存活负责」

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix（option A：改 strong）
- **位置**: `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift:122`

### 症状（Symptom）

`HomeViewModel.init(..., appState: AppState? = nil)` 暴露了构造注入入口，但内部 `private weak var appState: AppState?` 让任何 `HomeViewModel(appState: AppState())` 形式的 caller 立刻丢引用 —— 注入参数指向的 fresh AppState 在 init 返回后即被释放，后续 `applyHomeData(_:)` 内 `self.appState?.applyHomeData(data)` 走 `nil` 分支静默 no-op。生产路径（RootView @StateObject 同时 strong 持有 AppState 与 HomeViewModel）下不出问题，但「构造注入」这个公开 API 在生产路径之外形同虚设；尤其测试 / Preview / 其他 owner 都会踩坑。

### 根因（Root cause）

dev-story 把"防循环引用"作为 weak 的唯一论证依据，没区分两种引用语义：
1. **观察者模式 / delegate 反向引用**：A 持 B（owner），B 持 A 是循环 → B → A 用 weak 才对。
2. **构造注入的依赖**：caller 把"我希望 ViewModel 长期使用的实例"传进来 → ViewModel **必须** strong 持有，否则注入语义崩塌。

dev-story 看到「RootView @StateObject 同时持 AppState + HomeViewModel」这个特定生产拓扑后，错把 case 1 的解法套到 case 2 —— 但 caller 拓扑不是 ViewModel 自己能预设的。weak 在生产路径"巧合地"工作，是因为 RootView 给 AppState 留了 strong owner；任何不留的 caller 都会触发静默 bug。

ADR-0010 §3.1 钦定「ViewModel 仅允许构造注入 AppState」明确该字段的语义类别 = case 2。

### 修复（Fix）

option A：删 `weak` 关键字，改为 strong 引用。

before：
```swift
private weak var appState: AppState?
```

after：
```swift
private var appState: AppState?
```

风险审查：HomeViewModel strong 持 AppState 不会形成循环 —— 已在 `iphone/PetApp/State/` 全局 grep 确认 AppState 不反向持 HomeViewModel。生产路径 RootView 拓扑下两条 strong 路径（RootView → AppState、RootView → HomeViewModel → AppState）汇合在同一 AppState 上，正常 ARC 不构成循环。

新增回归测试 `testInitInjectionWithFreshAppStateRetainsReference`（`iphone/PetAppTests/Features/Home/ViewModels/HomeViewModelLoadHomeTests.swift`）：故意不在测试 stack 上保留 `AppState()` 引用，让 `HomeViewModel(appState: AppState())` 的 init 注入路径成为 AppState 的唯一持有者；通过 Mirror 反射断言 `viewModel.appState` 字段在 `applyHomeData` 后仍非 nil。该 case 在旧 weak 实现下必然 fail（appState 在 init 返回后即释放），新 strong 实现下通过。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 ViewModel / Service 写**构造注入**字段时，**必须** strong 持有 —— 注入参数的语义就是「ViewModel 接管该实例的生命周期共享」，不是「我观察一下就行」。
>
> **展开**：
> - 区分两类引用：(a) 反向通知 / delegate 用 weak 防循环；(b) 构造注入 / 依赖注入用 strong（语义层面 ViewModel 就是要把它用到 deinit）。
> - 看到"防循环引用"作为 weak 的唯一论证时停一下问：循环路径是哪两段？反向路径是否真的存在？常见误判：「反向路径不存在但 weak 也无害吧」—— **有害**：weak 的"巧合存活"完全依赖外部 owner 的存在，破坏注入 API 在通用 caller 下的可用性。
> - 写 `private weak var X: Type?` 时如果 X 来自 init 参数 / bind() 单次绑定 → **几乎肯定**应该 strong；除非能明确指出"循环路径在哪两个对象间形成"。
> - 决策若依赖"生产拓扑刚好让 weak 不释放"，等价于"测试 / Preview / 其他 caller 全部踩坑也无所谓"——这违反构造注入作为公开 API 的契约。
> - **反例**：本 lesson 修复前的 HomeViewModel —— init 参数 + weak 存储 + 文档说"防循环"但循环路径其实不存在；dev-story Dev Notes 段甚至记录了 design intent，但 intent 本身错。
> - 加测试时优先用「不在测试 stack 留 strong owner」的形式覆盖回归 —— 用 `viewModel.bind(appState:)` + `let appState = AppState()` 的常见模式无法揭示 weak bug（测试 stack 上的 `let` 给 AppState 留了 strong owner 让旧实现也能过）。

---

## Meta: 本次 review 的宏观教训

dev-story 阶段「写一段 design intent 注释解释自己的选择」很容易把错误的设计理由也固化进文档，让后续 reviewer / Claude 沿用。codex 这次直接对 design intent 段提反例（init + weak + fresh-instance caller）有效打破了「文档自洽 = 设计正确」的伪证。教训：design intent 的合理性必须经得起「caller 拓扑变换」的压测，而不是只被生产路径单点拓扑锚定。
