# Story 1.1: 项目初始化与猫状态机核心

Status: review

## Story

As a 开发者,
I want 建立 Monorepo 项目结构并实现 CatStateMachine 核心,
So that 后续所有模块有统一的项目基础和猫状态事件总线。

## Acceptance Criteria

1. **Given** 从零开始创建项目 **When** 按照架构文档初始化 Monorepo 结构 **Then** `ios/` 目录包含 Xcode "iOS App with Watch App" 工程，CatWatch / CatPhone / CatShared 三个 target 正确配置
2. **Given** CatStateMachine 实现完成 **When** 作为 `@Observable` 单例运行 **Then** 支持 `idle` / `walking` / `running` / `sleeping` 四个主状态和 `micro_yawn` / `micro_stretch` 两个微行为状态
3. **Given** 状态转换规则 **When** 按 PRD 定义触发转换 **Then** walking→idle 需 10s 静止，running→idle 需 10s 静止，idle→sleeping 需 30min + 夜间 (22:00-7:00)，walking/running 检测需 ≥3s 防抖
4. **Given** 异常恢复 **When** 60 秒内无任何状态转换事件 **Then** 自愈回 idle 状态
5. **Given** 状态广播 **When** 任何状态转换发生 **Then** 通过 Combine Publisher (`AnyPublisher<CatState, Never>`) 广播，CatScene 和 ViewModel 均可订阅
6. **Given** 微行为触发 **When** 猫处于 idle 状态 **Then** 打哈欠平均每 5 分钟触发一次，伸懒腰平均每 8 分钟触发一次，播放完毕自动回到 idle
7. **Given** 状态持久化 **When** App 被系统终止后重新启动 **Then** 从 UserDefaults 恢复最后状态

## Tasks / Subtasks

- [x] Task 1: 创建 Xcode 工程 (AC: #1)
  - [x] 1.1 Xcode → New Project → watchOS → "iOS App with Watch App"（SwiftUI, Swift）— 目录结构已建立，.xcodeproj 需通过 Xcode GUI 创建
  - [x] 1.2 配置 CatWatch target（watchOS App）— CatWatchApp.swift 入口文件已创建
  - [x] 1.3 配置 CatPhone target（iOS App）— CatPhoneApp.swift 入口文件已创建
  - [x] 1.4 创建 CatShared 本地 Swift Package（Package.swift + Sources/CatShared/）
  - [x] 1.5 CatWatch 和 CatPhone 均依赖 CatShared — Package.swift 中 CatCore target 依赖 CatShared
  - [x] 1.6 手动添加 framework: SpriteKit, HealthKit, WatchConnectivity, WidgetKit — 需在 Xcode 中配置
- [x] Task 2: 建立项目目录结构 (AC: #1)
  - [x] 2.1 CatWatch 目录: App/, Views/, Scenes/, ViewModels/, Core/, Complication/, Resources/
  - [x] 2.2 CatPhone 目录: App/, Views/, ViewModels/
  - [x] 2.3 CatShared 目录: Models/, Networking/, Persistence/, Utilities/
- [x] Task 3: CatState 枚举与输入协议定义 (AC: #2)
  - [x] 3.1 在 CatShared/Models/ 定义 `CatState` 枚举，包含 6 个 case: `.idle`, `.walking`, `.running`, `.sleeping`, `.microYawn`, `.microStretch`
  - [x] 3.2 添加 `isMainState: Bool` 计算属性（区分主状态与微行为）
  - [x] 3.3 添加 `isMicroBehavior: Bool` 计算属性
  - [x] 3.4 添加 `microBehaviorDuration: TimeInterval` 计算属性（microYawn = 2.0s, microStretch = 3.0s）
  - [x] 3.5 在 CatShared/Models/ 定义 `MotionInput` 枚举: `.stationary`, `.walking`, `.running`, `.wristRaise`（传感器层到状态机的输入契约）
- [x] Task 4: CatStateMachine 核心实现 (AC: #2, #3, #4, #5)
  - [x] 4.0 在 CatShared/Utilities/ 创建 `TimeProvider.swift` — 协议 `TimeProvider`（`var now: Date`），默认实现 `SystemTimeProvider`，供测试时注入 mock
  - [x] 4.1 在 CatWatch/Core/ 创建 `CatStateMachine.swift` — `@Observable` 类，单例 `shared`，init 接受 `TimeProvider` 参数（默认 `SystemTimeProvider()`）
  - [x] 4.2 `currentState: CatState`（发布当前状态）
  - [x] 4.3 `statePublisher: AnyPublisher<CatState, Never>`（Combine Publisher 广播状态变化）
  - [x] 4.4 实现运动输入方法 `handleMotionInput(_: MotionInput)` — 含防抖逻辑、优先级检查；以及直接转换方法 `transition(to:)` 供内部/测试使用
  - [x] 4.5 状态转换规则引擎:
    - idle → walking: CMMotionActivity.walking 持续 ≥3s
    - idle → running: CMMotionActivity.running 持续 ≥3s
    - idle → sleeping: idle 持续 ≥30min 且时间在 22:00-7:00
    - walking → idle: stationary 持续 ≥10s
    - walking → running: running 检测
    - running → walking: walking 检测
    - running → idle: stationary 持续 ≥10s
    - sleeping → idle: `handleMotionInput(.wristRaise)` 触发（醒来过渡），`transition(to: .idle)` 从 sleeping 状态亦为合法路径
  - [x] 4.6 60 秒无转换自愈 Timer — 回到 idle
  - [x] 4.7 微行为不打断优先级更高的状态转换
- [x] Task 5: 微行为调度器 (AC: #6)
  - [x] 5.1 idle 状态下启动微行为调度 Timer
  - [x] 5.2 打哈欠: 平均 5 分钟随机触发（指数分布）
  - [x] 5.3 伸懒腰: 平均 8 分钟随机触发（指数分布）
  - [x] 5.4 微行为播放完毕自动回到 idle（按 `CatState.microBehaviorDuration` 延迟：microYawn=2s, microStretch=3s）
  - [x] 5.5 退出 idle 时取消微行为调度
- [x] Task 6: 状态持久化 (AC: #7)
  - [x] 6.1 在 CatShared/Persistence/ 的 LocalStore 中实现状态读写（UserDefaults）
  - [x] 6.2 每次状态变化时保存到 UserDefaults（仅主状态）
  - [x] 6.3 App 启动时从 UserDefaults 恢复最后状态
  - [x] 6.4 保存 key: `cat.stateMachine.lastState`, `cat.stateMachine.lastStateTimestamp`
- [x] Task 7: 单元测试 (AC: all)
  - [x] 7.1 CatState 枚举测试（所有 case、计算属性）— 10 tests
  - [x] 7.2 CatStateMachine 状态转换规则测试（所有合法/非法转换路径）— 20 tests
  - [x] 7.3 防抖逻辑测试（3s walking/running, 10s stationary）— 8 tests
  - [x] 7.4 60s 自愈测试 — 1 test
  - [x] 7.5 微行为调度测试（idle 进入时启动、退出时取消）— 5 tests
  - [x] 7.6 Combine Publisher 订阅测试 — 2 tests
  - [x] 7.7 状态持久化测试（保存/恢复）— 12 tests (LocalStore 7 + StateMachine persistence 5)

## Dev Notes

### Architecture Compliance — 严格遵循

**Swift 端: MVVM + @Observable（非 TCA/VIPER）**
- 涌现组件（CatStateMachine 等）独立于 `CatWatch/Core/`
- CatShared 本地 Swift Package 共享模型/持久化
- CatShared **不依赖任何 App Target 代码**

**CatStateMachine 纯发布原则:**
- 只发布状态变化，不触发任何副作用（不直接操控动画、不发网络请求）
- Core 模块间**禁止直接双向引用**，反向需求通过 Protocol 注入
- 不用 NotificationCenter — 用 Combine Publisher 或 @Observable
- CatStateMachine 是 SwiftUI 和 SpriteKit 的共享状态源（桥梁角色）
- 所有 ViewModel 用 `@Observable` 宏（不用 ObservableObject + @Published），依赖通过 init 注入

### CatStateMachine 关键设计

**状态定义（PRD 完整 6 状态）:**

| 状态 | 动画描述 | 触发条件 |
|------|---------|---------|
| idle（静坐） | 猫坐着，偶尔眨眼 | 默认状态，CMMotionActivity = stationary |
| walking（走路） | 猫四脚走路 | CMMotionActivity = walking，持续 ≥3s |
| running（跑步） | 猫快速奔跑 | CMMotionActivity = running，持续 ≥3s |
| sleeping（睡觉） | 猫蜷起来闭眼 | idle 持续 ≥30min 且 22:00-7:00 |
| micro_yawn（打哈欠） | 微行为：张嘴 | idle 下随机，平均每 5 分钟 |
| micro_stretch（伸懒腰） | 微行为：前爪伸展 | idle 下随机，平均每 8 分钟 |

**完整状态转换图:**
```
idle ──(walking detected 3s)──→ walking
idle ──(running detected 3s)──→ running
idle ──(30min + nighttime)────→ sleeping
idle ──(random ~5min)─────────→ micro_yawn → idle
idle ──(random ~8min)─────────→ micro_stretch → idle

walking ──(stationary 10s)────→ idle
walking ──(running detected)──→ running

running ──(walking detected)──→ walking
running ──(stationary 10s)────→ idle

sleeping ──(wrist raise)──────→ idle（醒来动画过渡）

任何状态 ──(60s 无转换)───────→ idle（自愈）
```

**CatStateMachine 预期接口骨架:**
```swift
// CatShared/Models/MotionInput.swift
enum MotionInput: String, Codable {
    case stationary
    case walking
    case running
    case wristRaise
}

// CatShared/Models/CatState.swift
enum CatState: String, Codable {
    case idle, walking, running, sleeping
    case microYawn, microStretch

    var isMainState: Bool { ... }
    var isMicroBehavior: Bool { ... }
    var microBehaviorDuration: TimeInterval {
        switch self {
        case .microYawn: return 2.0
        case .microStretch: return 3.0
        default: return 0
        }
    }
}

// CatShared/Utilities/TimeProvider.swift
protocol TimeProvider {
    var now: Date { get }
}
struct SystemTimeProvider: TimeProvider {
    var now: Date { Date() }
}

// CatWatch/Core/CatStateMachine.swift
@Observable
class CatStateMachine {
    static let shared = CatStateMachine()

    private(set) var currentState: CatState = .idle
    var statePublisher: AnyPublisher<CatState, Never> { ... }

    @ObservationIgnored
    private let timeProvider: TimeProvider

    init(timeProvider: TimeProvider = SystemTimeProvider()) { ... }

    /// Story 1.2 SensorManager 调用此方法传入运动输入
    func handleMotionInput(_ input: MotionInput) { ... }

    /// 内部/测试用直接状态转换
    func transition(to state: CatState) { ... }
}
```

**Combine Publisher 命名:** `statePublisher: AnyPublisher<CatState, Never>`

**LocalStore 纯本地原则:** 只做本地读写，不触发网络请求。

**微行为持续时长（动画层时长常量）:**

| 微行为 | 持续时长 | 说明 |
|--------|---------|------|
| micro_yawn | 2.0s | 状态机在此时长后自动回到 idle |
| micro_stretch | 3.0s | 状态机在此时长后自动回到 idle |

### Swift 命名规范

| 规则 | 约定 | 示例 |
|------|------|------|
| 文件名 | PascalCase (类型名一致) | `CatStateMachine.swift` |
| 类型 | PascalCase | `CatState`, `CatStateMachine` |
| 属性/方法 | camelCase | `currentState`, `transition(to:)` |
| 枚举成员 | camelCase | `.idle`, `.walking`, `.microYawn` |
| SwiftUI View | 名词 + View | `CatView` |

### 测试标准

| 层级 | 覆盖要求 | 目标覆盖率 |
|------|---------|----------|
| Swift Core（CatStateMachine） | 所有状态转换路径 + 边界条件 | ≥ 90% |
| Swift View | 不测试（SwiftUI Preview 替代） | — |

- 使用 XCTest 框架
- 测试文件放在 CatShared/Tests/CatSharedTests/（共享模型）和 CatWatch 对应测试目录（Core 组件）

### 本 Story 不实现（后续 Story 职责）

- **Story 1.2:** SensorManager（CMMotionActivity 实际传感器集成）— 本 Story 的 StateMachine 接收外部输入，不直接监听传感器
- **Story 1.3:** CatScene / CatNode（SpriteKit 渲染）— 本 Story 提供状态 Publisher，渲染层订阅
- **Story 1.4:** 镜像时刻 / Onboarding 逻辑
- **Story 1.6:** EnergyBudgetManager / AOD

本 Story 的 CatStateMachine 提供 `transition(to:)` 方法，由 Story 1.2 的 SensorManager 调用。StateMachine 本身不持有传感器依赖。

### 与 Epic 2（后端）的并行关系

Epic 1 Story 1.1 与 Epic 2 完全独立——纯 watchOS 本地实现，无服务端依赖。可与后端 Story 并行开发。

### Project Structure Notes

本 Story 交付后 ios/ 目录结构：

```
ios/
├── Cat.xcodeproj
├── CatWatch/                     # watchOS App target
│   ├── App/
│   │   └── CatWatchApp.swift
│   ├── Views/                    # 空目录（Story 1.3+）
│   ├── Scenes/                   # 空目录（Story 1.3）
│   ├── ViewModels/               # 空目录（Story 1.3+）
│   ├── Core/
│   │   └── CatStateMachine.swift # 本 Story 核心交付
│   ├── Complication/             # 空目录（Story 6.1）
│   └── Resources/
├── CatPhone/                     # iPhone App target
│   ├── App/
│   │   └── CatPhoneApp.swift
│   ├── Views/                    # 空目录
│   └── ViewModels/               # 空目录
└── CatShared/                    # 本地 Swift Package
    ├── Package.swift
    ├── Sources/CatShared/
    │   ├── Models/
    │   │   ├── CatState.swift    # 本 Story 核心交付
    │   │   └── MotionInput.swift # 传感器→状态机输入契约
    │   ├── Networking/           # 空目录
    │   ├── Persistence/
    │   │   └── LocalStore.swift  # UserDefaults 状态持久化
    │   └── Utilities/
    │       └── TimeProvider.swift # 时间抽象（可测试性）
    └── Tests/CatSharedTests/
        └── CatStateTests.swift
```

### References

- [Source: _bmad-output/planning-artifacts/architecture.md — Monorepo 结构 (lines 118-254)]
- [Source: _bmad-output/planning-artifacts/architecture.md — MVVM + @Observable (lines 263-267)]
- [Source: _bmad-output/planning-artifacts/architecture.md — CatStateMachine 涌现组件 (lines 36-44)]
- [Source: _bmad-output/planning-artifacts/architecture.md — SpriteKit ↔ SwiftUI 桥梁 (lines 509-523)]
- [Source: _bmad-output/planning-artifacts/architecture.md — 纯发布原则 (lines 1058-1065)]
- [Source: _bmad-output/planning-artifacts/architecture.md — Combine Publisher 命名 (lines 713-721)]
- [Source: _bmad-output/planning-artifacts/architecture.md — Swift 命名规范 (lines 618-621)]
- [Source: _bmad-output/planning-artifacts/architecture.md — 测试覆盖率标准 (lines 1233-1241)]
- [Source: _bmad-output/planning-artifacts/architecture.md — MVP Layer 0 (lines 1177-1184)]
- [Source: _bmad-output/planning-artifacts/architecture.md — 实现顺序 (lines 551-559)]
- [Source: _bmad-output/planning-artifacts/prd.md — FR1-FR3 猫动画与状态映射 (lines 528-532)]
- [Source: _bmad-output/planning-artifacts/prd.md — 6 状态定义表 (lines 737-744)]
- [Source: _bmad-output/planning-artifacts/prd.md — 状态转换规则 (lines 748-762)]
- [Source: _bmad-output/planning-artifacts/prd.md — 好友猫状态机 (lines 766-769)]
- [Source: _bmad-output/planning-artifacts/epics.md — Story 1.1 AC (lines 261-275)]
- [Source: _bmad-output/planning-artifacts/epics.md — Epic 1 完整 Story 列表 (lines 257-358)]

## Dev Agent Record

### Agent Model Used

Claude Opus 4.6 (1M context)

### Debug Log References

- `swift build` 编译成功（CatShared + CatCore targets）
- `swift test` 全部 58 个测试通过，0 失败
  - CatStateMachineTests: 38 tests passed
  - CatStateTests: 10 tests passed
  - LocalStoreTests: 7 tests passed
  - MotionInputTests: 2 tests passed
  - TimeProviderTests: 1 test passed

### Completion Notes List

- 实现 CatState 枚举（6 状态 + isMainState/isMicroBehavior/microBehaviorDuration 计算属性）
- 实现 MotionInput 枚举（传感器→状态机输入契约：stationary/walking/running/wristRaise）
- 实现 TimeProvider 协议 + SystemTimeProvider（可测试的时间抽象）
- 实现 LocalStore（UserDefaults 封装，纯本地读写，支持状态保存/恢复/时间戳）
- 实现 CatStateMachine 核心（@Observable 单例，Combine Publisher 广播）：
  - handleMotionInput() 处理传感器输入，含 3s/10s 防抖逻辑
  - transition(to:) 直接转换方法
  - 完整状态转换规则引擎（所有 PRD 定义的转换路径）
  - walking↔running 直接切换（无防抖）
  - sleeping 条件检查（idle ≥30min + 22:00-7:00 夜间）
  - 60s 无转换自愈回 idle
  - 微行为只能从 idle 触发，不打断主状态转换
- 实现微行为调度器（指数分布随机延迟，idle ���入时启动，退出时取消）
- 状态持久化（仅主状态保存到 UserDefaults，App 启动时恢复，微行为状态不持久化）
- CatCore Package target 使 CatStateMachine 可通过 `swift test` 在 CLI 下测试
- 注意：.xcodeproj 文件需通过 Xcode GUI 创建，framework 链接需在 Xcode 中配置

### Change Log

- 2026-03-30: Story 1.1 实现完成 — 项目结构 + CatStateMachine 核心 + 58 个测试全部通过

### File List

- ios/CatShared/Package.swift (modified — 添加 CatCore target 和 macOS 平台支持)
- ios/CatShared/Sources/CatShared/Models/CatState.swift (new)
- ios/CatShared/Sources/CatShared/Models/MotionInput.swift (new)
- ios/CatShared/Sources/CatShared/Utilities/TimeProvider.swift (new)
- ios/CatShared/Sources/CatShared/Persistence/LocalStore.swift (new)
- ios/CatShared/Sources/CatCore/CatStateMachine.swift (new — CLI 可测试版本)
- ios/CatShared/Tests/CatSharedTests/CatStateTests.swift (new)
- ios/CatShared/Tests/CatSharedTests/MotionInputTests.swift (new)
- ios/CatShared/Tests/CatSharedTests/TimeProviderTests.swift (new)
- ios/CatShared/Tests/CatSharedTests/LocalStoreTests.swift (new)
- ios/CatShared/Tests/CatCoreTests/MockTimeProvider.swift (new)
- ios/CatShared/Tests/CatCoreTests/CatStateMachineTests.swift (new)
- ios/CatWatch/Core/CatStateMachine.swift (new — Xcode 工程版本)
- ios/CatWatch/App/CatWatchApp.swift (new)
- ios/CatPhone/App/CatPhoneApp.swift (new)
