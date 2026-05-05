---
date: 2026-05-04
source_review: codex review round 1 — /tmp/epic-loop-review-8-4-r1.md
story: 8-4-主界面猫-sprite-三态动画切换
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-04 — MotionProvider.bind 必须先 gate authorizationStatus 再 startUpdates（first-launch 不弹权限红线）

## 背景

Story 8.4（主界面猫 sprite 三态动画切换）在 RootView `.onAppear` 内调 `homeViewModel.bind(motionProvider: container.motionProvider)`，bind 内部直接 `motionProvider.startUpdates(...)` 注册 handler. 在 first install / 模拟器 reset 后，CMMotionActivity 授权态为 `.notDetermined` —— `CMMotionActivityManager.startActivityUpdates` 在该态下会**触发系统 NSMotionUsageDescription 权限弹窗**，违背 Story 8.4 红线"权限申请由 8.5 / 8.6 同步触发器统一处理，HomeViewModel 不主动请求权限"，并阻塞 launch-time UITest（系统弹窗拦住 a11y query）.

codex round 1 P1 抓到这条；本 lesson 记录修复路径与未来防御规则.

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | bind 路径在 .notDetermined 下直接 startUpdates 触发系统权限弹窗 | high | architecture | fix | `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift` + `iphone/PetApp/Core/Motion/MotionProvider.swift` + `MotionProviderImpl.swift` + `MotionProviderMock.swift` |

## Lesson 1: bind(motionProvider:) 必须先 gate authorizationStatus 再决定是否 startUpdates

- **Severity**: high
- **Category**: architecture（System Adapter 协议契约 / 权限链路职责边界）
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift:353` + `iphone/PetApp/App/RootView.swift:201`

### 症状（Symptom）

First install / "重置位置与隐私" 后 launch App，HomeView 第一次 paint 之前在 RootView `.onAppear` 调 `homeViewModel.bind(motionProvider:)` → bind 内部无条件 `motionProvider.startUpdates(...)` → CoreMotion 检测到 `.notDetermined` 态 → 触发 NSMotionUsageDescription 系统权限 sheet. 后果：① 用户在主界面渲染前先看到权限弹窗，与"权限按需"产品节奏冲突 ② launch-time UITest 在 clean simulator 上被弹窗拦住 a11y query → flaky / 全失败.

### 根因（Root cause）

Story 8.4 红线写"HomeViewModel 不调 requestPermission（权限按 AR17 / 8.5 / 8.6 处理）"，但作者只 literally 不调 `requestPermission()`，没意识到 `startActivityUpdates` 本身在 `.notDetermined` 下也会触发权限弹窗（CoreMotion 钦定行为，与 `requestPermission` 是两条独立的"权限触发"路径）.

更深的思维漏洞：**System Adapter 协议层不暴露"纯查询授权状态"入口** —— `MotionProvider` 只有 `requestPermission` (主动申请，会弹窗) + `startUpdates` (订阅，会弹窗) + `stopUpdates`，没有"我现在能不能 startUpdates 而不弹窗"的 idempotent 查询. 调用方（HomeViewModel）想做"未授权时只持引用不订阅"也无 API 可用.

类比：Story 8.1 HealthProvider 也有同款风险，但因为 HealthKit 的 `authorizationStatus` 不可被信赖（Apple 文档明确），8.1 用了 probe-read 兜底；这个特殊性让 8.4 作者错把"协议层不暴露 authorizationStatus"当成默认范式，没意识到 CoreMotion 的 `CMMotionActivityManager.authorizationStatus()` (iOS 11+) **是**可信赖的 lightweight 静态查询.

### 修复（Fix）

**方案 D**：协议层加 `authorizationStatus()` 纯查询入口 + HomeViewModel.bind 内部 gate.

- `MotionProvider.swift`：
  - 新增 `enum MotionAuthorizationStatus { case notDetermined / authorized / denied / restricted }`（与 `CMAuthorizationStatus` 1:1 映射，但不让协议耦合 CoreMotion 类型，未来切后端零 protocol 改动）.
  - 协议加 `func authorizationStatus() -> MotionAuthorizationStatus`，文档明确"纯查询，不发起任何 OS 调用 / 不触发权限弹窗".

- `MotionProviderImpl.swift`：
  - 实装 `authorizationStatus()` bridge `CMMotionActivityManager.authorizationStatus()` (class func, lightweight) → `MotionAuthorizationStatus` enum，`@unknown default` 保守视作 `.notDetermined`.

- `MotionProviderMock.swift`：
  - 加 `var authorizationStatusStub: MotionAuthorizationStatus = .authorized` (默认 .authorized 与既有测试用例兼容)
  - 加 `authorizationStatusCallCount: Int` 让单测可断言 bind 路径真的查询了权限状态而非直接 startUpdates.
  - `reset()` 也清新 stub + counter.

- `HomeViewModel.swift`：
  - `bind(motionProvider:)` 改为 idempotent rebind 模式：
    - 第一次进 bind：`if self.motionProvider == nil { self.motionProvider = motionProvider }` —— 仍只存一次引用（防多 instance 混入）.
    - 拆分 guard：新增 `hasStartedMotionUpdates: Bool` 字段，`guard !hasStartedMotionUpdates else { return }` 短路"已订阅"路径（防双倍事件）.
    - **加 gate**：`guard motionProvider.authorizationStatus() == .authorized else { return }` —— 未授权三态只持引用不订阅.
    - `.authorized` 才设 `hasStartedMotionUpdates = true` + 调 `motionProvider.startUpdates(...)`.
  - 这让 Story 8.6 / 8.5 授权 flow 走完后调用方再调一次 `bind(motionProvider:)` 能**升级**到 startUpdates（不被原 `guard self.motionProvider == nil` 短路掉），而 deinit `motionProvider?.stopUpdates()` 路径不变（持引用即可幂等 stop）.

- `HomeViewModelMotionTests.swift`：新增 5 case 覆盖 gate 矩阵（`.notDetermined` / `.denied` / `.restricted` 不订阅；`.authorized` 订阅；未授权 → 授权 rebind 升级；未授权 bind + deinit 仍调 stopUpdates）.

before:
```swift
public func bind(motionProvider: MotionProvider) {
    guard self.motionProvider == nil else { return }
    self.motionProvider = motionProvider
    motionProvider.startUpdates { ... }   // ⚠ first-launch .notDetermined 下触发权限弹窗
}
```

after:
```swift
public func bind(motionProvider: MotionProvider) {
    if self.motionProvider == nil { self.motionProvider = motionProvider }
    guard !hasStartedMotionUpdates else { return }
    guard motionProvider.authorizationStatus() == .authorized else { return }
    hasStartedMotionUpdates = true
    motionProvider.startUpdates { ... }
}
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：在 iOS / macOS System Adapter 协议（HealthKit / CoreMotion / Location / Camera / Microphone 等）层设计"订阅式 API"（`startUpdates` / `subscribe` / `observe` 等）时，**必须**配套提供 `authorizationStatus()` 纯查询入口，让上层调用方能在不触发权限弹窗的前提下决定是否 subscribe；上层 bind / wire 路径**禁止**在未确认 `.authorized` 之前直接 startSubscription.
>
> **展开**：
> - "权限按需"红线的具体含义不是"不调 `requestPermission()`"，而是"任何会触发系统权限弹窗的 API 调用都得等到产品节奏允许的时机"——`startActivityUpdates` / `HKHealthStore.execute(query)` / `CLLocationManager.startUpdatingLocation` 这些**订阅入口**在未授权下**默认会触发系统权限弹窗**，与 `requestPermission` 同等危险.
> - 协议层暴露 `authorizationStatus()` 时，签名约束：**同步**（async/throws 都不允许）+ **idempotent**（调多次无副作用）+ **不耦合系统类型**（用自定义 enum 与 CMAuthorizationStatus / HKAuthorizationStatus / CLAuthorizationStatus 1:1 映射，未来切后端不改 protocol）.
> - bind / wire 路径设计**必须**支持 idempotent rebind 升级模式：first launch 未授权时只持引用、不 subscribe；权限 flow 走完后再调 bind 一次升级到 subscribe. 用单独的 `hasStarted*` flag 短路"已订阅"路径，与"已存引用"分离，让"未存"/"已存未订阅"/"已订阅"三态可被独立短路.
> - **反例 1**：见 Story 8.4 round 1 原实装——`guard self.motionProvider == nil` 同时短路了"持引用"和"订阅"，导致授权后 rebind 永远进不来，配合"无条件 startUpdates"双重错误.
> - **反例 2**：在 RootView 端 gate（`if container.motionProvider.authorizationStatus() == .authorized { homeViewModel.bind(...) }`）—— 把权限 gate 推到 caller，每个新 caller 都得记住这条契约，而 ViewModel.bind **必须**自我保护，不依赖外部调用顺序.
> - **反例 3**：协议层暴露 `requestPermission() -> Bool` 但不暴露 `authorizationStatus()`，调用方想"纯查询"只能调 requestPermission（带 OS 弹窗副作用）—— 这是协议设计漏洞.
> - 配套测试：System Adapter Mock 必须暴露 `authorizationStatusStub`（默认 `.authorized` 与现有测试兼容；显式设 `.notDetermined`/`.denied`/`.restricted` 时测 gate 路径）+ `authorizationStatusCallCount`（让单测可断言 bind 真的查询了权限状态而非"碰巧 mock 不弹窗就过了").

---

## Meta: 本次 review 的宏观教训

Story 8.x 节点 3（权限链路）的红线"权限按需 by 8.5 / 8.6"很容易被字面理解为"不调 `requestPermission()`"，导致作者忽视"订阅式 API 在 `.notDetermined` 下也会触发权限弹窗"这个 OS 默认行为. 未来涉及 iOS 系统权限的协议（HealthKit / CoreMotion / Location / Camera / Microphone / Photos / Contacts 等）的 System Adapter 设计，**所有人** review 时都得 explicitly 检查 ① 协议是否有 authorizationStatus 纯查询入口 ② 上层 bind / wire 路径是否 gate 在 status check 之后 —— 这两条没满足任意一条就会埋 first-launch UX 雷.
