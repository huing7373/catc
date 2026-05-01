---
date: 2026-04-30
source_review: epic-loop codex review for Story 37.7 round 4 (/tmp/epic-loop-review-37-7-r4.md)
story: 37-7-homeview-scaffold
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-04-30 — `@Published` 派生字段必须订阅 publisher（hydrate 入口 override 不够覆盖 reset 路径）+ `@Published` 用 import 必须显式

## 背景

Story 37.7 round 3 修了一个派生字段问题：`RealHomeViewModel.greeting` 老 hardcode placeholder，没在 hydrate 后反映 `pet.name`。当时 round 3 实装是 override `applyHomeData(_:)` 入口在 super 写完 AppState 后派生 greeting。round 4 codex 复审指出这个修法**只覆盖了 hydrate 路径**，没覆盖 `appState.reset()` 路径——`ResetIdentityViewModel.tap()` 把 `currentPet` 置 nil 但不经过 `applyHomeData`，header 仍显示旧 pet 名。同时 codex 报了一条 [P0] 说 `MockHomeViewModel` 缺 `import Combine`，实跑 build 验证发现是当前 SDK transitive import 误报，但作为 hardening 仍补上显式 import。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | MockHomeViewModel 缺 `import Combine`（codex [P0] 实为误报，但作 hardening 落地） | high(误报)→hardening | dependency | fix | `iphone/PetApp/Features/Home/ViewModels/MockHomeViewModel.swift` |
| 2 | reset 后 greeting 残留旧 pet 名（round 3 只覆盖 hydrate 入口） | low(P3) | architecture | fix | `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift` + `iphone/PetAppTests/Features/Home/HomeViewScaffoldTests.swift` |

## Lesson 1: `@Published` 误报模式 —— 当前 iOS SDK transitive import 让 `@Published` 编译 OK，仍要显式 `import Combine` 防 future SDK regression

- **Severity**: high(codex 报)→hardening (实测 误报)
- **Category**: dependency
- **分诊**: fix（防御性 hardening）
- **位置**: `iphone/PetApp/Features/Home/ViewModels/MockHomeViewModel.swift:10`

### 症状（Symptom）

codex review 报 [P0]：`MockHomeViewModel.swift` 用 `@Published var invocations: [Invocation] = []` 但只 `import Foundation` 与 `import os.log`，预测 iPhone target 编译会挂在 `unknown attribute 'Published'`。

### 根因（Root cause）

实跑 `bash iphone/scripts/build.sh --test` 基线（commit `0b2df22`）已 BUILD SUCCESS + 271/271 通过——说明 codex 的预测在**当前 iOS SDK 上不成立**：

- `@Published` / `ObservableObject` 来自 Combine，但现代 iOS SDK 通过 SwiftUI / Foundation transitive import 让 `Combine` 在 view-related context 默认可见
- codex 模型可能用 macOS swiftc 单文件 `-typecheck` 推断 import 是否齐全；这种推断不等于 iOS app target 的真实编译路径
- 类似情况：Story 2.2 `2026-04-25-swift-explicit-import-combine.md` lesson 已记录过同一精神——transitive import 不可依赖

但**误报不等于不需要修**：codex 的判断方向正确，只是触发条件估错了 SDK 行为。补上 `import Combine` 是 future-proof hardening——未来 SDK 收紧 transitive 时不至于 regression。

### 修复（Fix）

`MockHomeViewModel.swift` 加 `import Combine`，与 `Foundation` / `os.log` 并列：

```swift
import Foundation
import Combine
import os.log
```

并在文件头注释里点明「当前 SDK 编译 OK，仅作 hardening；codex 报 [P0] 实为误报」。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **review 报"缺 import X 会编译挂"** 时，**必须先实跑 build 验证**，再决定是否落地修复——不盲信 LLM review 的预测，但**误报也要 hardening**（显式 import 不增加成本，防 future SDK regression）。
>
> **展开**：
> - **第一步必须 reproduce**：跑 `bash iphone/scripts/build.sh --test`（或 server 端 `bash scripts/build.sh --test`）。BUILD SUCCESS + 测试 pass = 当前真实状态；review 预测可能基于不同 toolchain
> - **transitive import 不可依赖**：即便当前 SDK 编译 OK，跨 SDK / 跨 Swift Package / 跨 module 不保证；显式 import 是 cheap hardening
> - **承认误报 ≠ wontfix**：误报但 hardening 仍可做（commit message 标"防御性 hardening"），lesson 里写清楚「当前 SDK 不挂，但显式 import 防 regression」
> - **反例**：盲信 codex 把 [P0] 当真问题修了 → 修法对，但 lesson 里如果不点明「实测当前 SDK 不挂」，未来读者会以为这是 hard error；同样反例：盲信 codex 把误报当 wontfix 不修 → 错过 hardening 机会
> - **关联 lesson**：`docs/lessons/2026-04-25-swift-explicit-import-combine.md`（Story 2.2）已记录过 ObservableObject / @Published 必须显式 import Combine，本 lesson 是其延伸（codex review 误报 + hardening 决策）

## Lesson 2: `@Published` 派生字段必须订阅 publisher，不能只在 hydrate 入口一次性 override（reset 路径会漏）

- **Severity**: low(P3)
- **Category**: architecture
- **分诊**: fix（option A：bind 时订阅 `appState.$currentPet`）
- **位置**: `iphone/PetApp/Features/Home/ViewModels/RealHomeViewModel.swift:94-100`（round 3 落地的 `applyHomeData` override）

### 症状（Symptom）

`RealHomeViewModel.greeting` 是从 `AppState.currentPet?.name` 派生的字段。round 3 修法是 override `applyHomeData(_:)`：先调 super 写 AppState，再读 `data.pet?.name` 拼 greeting。表面看 hydrate 路径 OK——实跑生产场景里有第二条改 `currentPet` 的路径：

- `ResetIdentityViewModel.tap()` → `appState.reset()` → `currentPet = nil`（同步路径，不经过 `HomeViewModel.applyHomeData`）

reset 后 `appState.currentPet` 已 nil，`AppState` 自身的 chest-badge / 用户头像等订阅 `@EnvironmentObject` 的 SwiftUI 字段都会同步刷，但 `RealHomeViewModel.greeting` 仍是上一次 hydrate 时拼的「测试猫，想你啦 ♥」——header 显示残留旧 pet 名直到下一次 hydrate。

### 根因（Root cause）

派生字段 `greeting` 跨过了 `AppState`（生产源） 和 `ViewModel`（消费层）的边界。当 ViewModel 不订阅源的 publisher、只在「写源」的入口（如 `applyHomeData`）一次性派生，就形成了**单入口耦合**：

- 该入口被调时派生正确
- 任何**绕过该入口直接修改源**的路径（reset / 单字段 mutate / 测试 stub）都让派生字段 stale

正确模式：**派生字段必须订阅源 publisher**。`AppState.$currentPet`（`@Published` projected publisher）任何变化（hydrate / reset / 单独 mutate）都会自动触发订阅 closure，subscriber 在 closure 内重派 greeting，覆盖所有写源路径。

### 修复（Fix）

`RealHomeViewModel` 改为订阅 `appState.$currentPet`：

```swift
private var greetingSubscription: AnyCancellable?

public init(appState: AppState) {
    super.init(appState: appState)
    configureMockDefaults()
    subscribeGreeting(to: appState)  // 构造路径
}

public override func bind(appState: AppState) {
    let alreadySubscribed = greetingSubscription != nil
    super.bind(appState: appState)
    guard !alreadySubscribed else { return }
    subscribeGreeting(to: appState)  // 异步注入路径
}

private func subscribeGreeting(to appState: AppState) {
    greetingSubscription = appState.$currentPet
        .sink { [weak self] pet in
            guard let self else { return }
            if let petName = pet?.name, !petName.isEmpty {
                self.greeting = "\(petName)，想你啦 ♥"
            } else {
                self.greeting = "想你啦 ♥"
            }
        }
}
```

并删除 round 3 的 `applyHomeData` override——sink 已自动覆盖（`super.applyHomeData` 写 `currentPet` → sink 触发）。

新增测试 case `testRealHomeViewModelGreetingFallsBackOnReset` 守护：hydrate 后 greeting 反映 pet name；`appState.reset()` 后 greeting 必须立即回 placeholder。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **ViewModel 派生 ObservableObject `@Published` 字段时**，**必须订阅源 publisher（`source.$field.sink`）而不是只在写入口（`applyXxx` / `bind`）派生**。
>
> **展开**：
> - **派生字段三问**：① 源是不是 `@Published`？② 源是否被多条路径写（hydrate / reset / 单独 mutate / 测试 stub）？③ 我的派生入口是否覆盖所有写路径？任一答「不」就用 sink 订阅
> - **`@Published` projected publisher**：用 `source.$field.sink { newValue in ... }`；closure 参数是即将赋上去的 newValue（不是 oldValue）；不需要 `receive(on:)`（同 thread 同 runloop tick 内更新）
> - **生命周期 + 内存**：`AnyCancellable` 字段保活；closure 用 `[weak self]` 防 ViewModel ↔ AppState 循环引用
> - **防重订阅**：异步注入路径（如 `bind` override）要 `guard greetingSubscription == nil` 短路防 SwiftUI .task 多次触发
> - **可订阅 vs 一次性派生选择**：源是 `@Published` 字段且被多路径写 → 订阅；源是构造时一次性传入的不可变 value type → 派生即可
> - **反例 1**（本次踩坑）：在 `applyHomeData(_:)` override 里派生 → reset 路径漏，header stale 直到下一次 hydrate
> - **反例 2**：用 `assign(to: \.greeting, on: self)` 而非 `sink`——会 strong 持 self，要么循环引用要么字段必须是非 weak。能用 sink + weak self 就用 sink
> - **反例 3**：`receive(on: RunLoop.main)` + 闭包内重读 `self.appState.currentPet`——增加 dispatch 缝隙，unit test 难同步断言；`@Published` projected publisher 已在主线程 fire，直接用 closure 参数即可
>
> **关联架构 ADR**：ADR-0010 §3.1 ViewModel 注入规则 + §3.5 派生 state 落点（`AppState` 持源 / `ViewModel` 派生 → 必须订阅）

---

## Meta: 本次 review 的宏观教训

round 4 两条 finding 都呼应同一个反复出现的 anti-pattern：**派生字段绑死在单一 mutation 入口上**。round 3 lesson `2026-04-30-realhomeviewmodel-greeting-and-empty-text-overlay.md` 已强调「Real ViewModel 派生字段必须 override hydrate 入口」——但**「override hydrate 入口」是 派生方案的最弱形态**：它只在「写主入口」时派生。round 4 把这个推论补完：

- 派生字段的强形态 = **订阅源 publisher**（subscriber pattern）
- 派生字段的弱形态 = override 单一 mutation 入口（hydrate-only / write-only）
- 弱形态在以下场景失效：① 源被多路径写（reset / 单字段 mutate / 测试 stub）② 入口是同步路径，源还有异步写路径

**升级路径**：以后派生 `@Published` 字段，先评估源的写入路径数；多于 1 条 → 直接上 sink；只有 1 条且不会扩展 → 可以临时用 override 入口，但留 TODO「未来如果新增写路径要升级订阅」。

下一个写 `RealHomeViewModel` 的 Story（如 12.7 / 14.x WS pet.state.changed 真实派生）可以直接基于本 lesson 选 sink 模式，跳过 round 3 → round 4 这种返工。
