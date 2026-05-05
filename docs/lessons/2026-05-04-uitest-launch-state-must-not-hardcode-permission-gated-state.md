---
date: 2026-05-04
source_review: codex review round 3 — /tmp/epic-loop-review-8-4-r3.md (Story 8.4)
story: 8-4-主界面猫-sprite-三态动画切换
commit: ece7aa0
lesson_count: 1
---

# Review Lessons — 2026-05-04 — UITest 不应钦定权限/异步事件依赖的 launch-time state

## 背景

Story 8.4 落地"主界面猫 sprite 三态动画切换"：PetSpriteView 根据 viewModel.petState（rest/walk/run）渲染不同 a11y identifier。round 2 fix-review 之后，UITest `testHomeViewShowsAllPlaceholders` 与新增 `testPetSpriteShowsRestStateOnLaunch` 都硬编码断言 `petSprite_rest`，假设启动时 pet 一定停在 rest。codex round 3 指出：已授权的 sim/device 上 RootView 启动即订阅 CoreMotion → pet 可瞬切到 walk/run，断言 nondeterministic fail。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | UITest 硬编码 launch-time `petSprite_rest` 断言；motion 已授权环境下会 nondeterministic fail | medium (P2) | testing | fix | `iphone/PetAppUITests/HomeUITests.swift:34-39, 104-121` |

## Lesson 1: UITest 不应钦定"权限/异步事件之前"的 default state

- **Severity**: medium (P2)
- **Category**: testing
- **分诊**: fix
- **位置**: `iphone/PetAppUITests/HomeUITests.swift:34-39`（既有 case） + `:104-121`（新增 case）

### 症状（Symptom）

UITest 在 launch 后立刻断言 `app.descendants(...)[AccessibilityID.Home.petSpriteRest].waitForExistence(...)`：

- 未授权环境（首次启动 sim 或 motion 未授权）：pet 卡 .rest，PASS。
- 已授权环境（持久化 sim 上 sticky 授权 / 真机已授权）：RootView 启动即 bind motion → MotionProvider 立刻派发 activity → pet 可能在断言前已切到 .walk / .run，断言 FAIL。
- 同一份代码在不同 CI runner / 开发机的 sim 状态下结果分裂 → flaky。

### 根因（Root cause）

把"启动后第一帧的 pet state"当作稳定契约写进了 UITest assertion。但这个 state 实际由：
1. Motion 授权状态（外部 system permission）
2. ScenePhase / RootView bind 时序
3. CoreMotion 派发延迟

三者共同决定 —— 都不是 UITest 能控制的。结果是把一个**不稳定的运行时 state** 当成了**稳定的视觉契约**。这种坑往往在"绿色 CI"环境上看不出（CI sim 通常 fresh / 未授权 → pet 卡 rest），等到开发机已授权 sim 上跑才暴露。

更深层：UITest 本应覆盖"wiring 是否正确"（PetSpriteView 是否渲染、a11y identifier 是否正确挂载），而不是覆盖"业务初始 state 是哪个"（那是单测/集成测试的领域）。

### 修复（Fix）

两个 case 都改为"三态任一存在"断言（fix 方案 A）：

before:
```swift
let petSpriteRest = app.descendants(matching: .any)[AccessibilityID.Home.petSpriteRest]
XCTAssertTrue(petSpriteRest.waitForExistence(timeout: timeout), "petSprite_rest 区块未找到")
```

after:
```swift
let petSpriteRest = app.descendants(matching: .any)[AccessibilityID.Home.petSpriteRest]
let petSpriteWalk = app.descendants(matching: .any)[AccessibilityID.Home.petSpriteWalk]
let petSpriteRun  = app.descendants(matching: .any)[AccessibilityID.Home.petSpriteRun]
_ = petSpriteRest.waitForExistence(timeout: timeout)  // 给 launch race 一个 wait window
XCTAssertTrue(
    petSpriteRest.exists || petSpriteWalk.exists || petSpriteRun.exists,
    "PetSpriteView 三态 a11y identifier 都未找到（不应 dead wiring）"
)
```

并把 `testPetSpriteShowsRestStateOnLaunch` 改名为 `testPetSpriteRendersAtLaunch`（命名不再钦定 .rest）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写 UITest 时，**禁止**把"依赖 system permission / 异步外部事件"的 view state 当成 launch-time 稳定断言；改为断言"任一可能 state 存在"或"稳定 container 存在"。
>
> **展开**：
> - **触发条件**：当 view 的渲染 state 由（a）OS 权限授权状态、（b）CoreMotion / HealthKit / Location 异步事件、（c）网络回调 决定时，UITest assertion 不能钦定其中某个具体 state 为 launch 默认值。
> - **检查清单**：写 UITest 前问自己——"这个断言的 expected state 是由哪个事件源决定的？这个事件源在 sim 不同 baseline 状态下行为是否一致？" 不一致 → 改用三态/多态任一断言。
> - **正确策略**：
>   - **三态任一**（推荐，覆盖 wiring）：`A.exists || B.exists || C.exists`
>   - **稳定 container**：断言上层 container（如 `catStage`）存在 —— 但纯 container 假绿风险高（child wiring 断了 container 还在）。
>   - **组合**：container + 三态任一 双重覆盖（最严但最啰嗦）。
> - **反例**：
>   - `XCTAssertTrue(petSprite_rest.exists)` ——  motion 授权后启动直接切 walk，FAIL。
>   - `XCTAssertTrue(syncedAt.staticTexts["从未同步"].exists)` —— 已登录 sim 上 sticky token 直接触发 sync，FAIL（同类坑）。
>   - `XCTAssertTrue(loginButton.exists)` 在已登录 sim 上 —— 同类。
> - **命名提示**：UITest case 名字带 "ShowsXxxStateOnLaunch" / "InitialState" 时立刻警惕 —— 这种命名常常隐含对不稳定 launch state 的钦定。改为 "RendersAtLaunch" / "MountsCorrectly" 等只覆盖 wiring 的中性命名。

---

## Meta: 本次 review 的宏观教训

UITest 的 expected behavior 必须仅依赖**测试工程能控制的状态**（launch env vars、launch arguments、mock 注入）。任何由"外部 system 事件 / 权限 sticky state / 网络异步"决定的运行时 state 都不能写进 UITest 断言条件 —— 写进去就是把 flakiness 永久埋进 CI。覆盖 wiring（identifier 是否正确挂载）和覆盖 business logic（初始 state 是哪个）是两件事：UITest 应该只做前者，后者交给单测 + 集成测试。
