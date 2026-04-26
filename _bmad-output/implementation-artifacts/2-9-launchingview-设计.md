# Story 2.9: LaunchingView 设计（首次启动过场）

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want App 启动时看到一个友好的过场画面（猫咪 logo + 加载提示），不是空白屏,
so that 首次启动 3-5 秒等待中（未来接 Epic 5 自动登录 + LoadHome）不焦虑、不疑惑"App 是不是卡住了"。

## 故事定位（Epic 2 第九条实装 story；Epic 2 收尾倒数第二条 — 给 Epic 5 自动登录预留接入点）

这是 Epic 2 内**第九条**实装 story，**直接前置**全部 done：

- **Story 2.1 (`done`)** 输出 ADR-0002 锁定 4 类决策（XCTest only / async/await / `iphone/` 目录方案 / `bash iphone/scripts/build.sh --test` 入口）—— 本 story 的所有测试严格用 XCTest only，不引第三方
- **Story 2.2 (`done`)** 落地 `iphone/` 顶层目录 + `iphone/PetApp/{App,Core,Features,Shared,Resources}` + `iphone/project.yml` + 主界面 6 大占位区块；本 story **不动** HomeView 内部任何渲染逻辑，只在 RootView 增加"在 HomeView 渲染前先显示 LaunchingView"的状态机切换
- **Story 2.3 (`done`)** 落地 `AppCoordinator` + 三个 CTA 按钮（进入房间 / 仓库 / 合成）的 sheet 路由 —— 本 story `LaunchingView` **不**走 sheet 系统（它替代 RootView body 显示，不在 modifier 链上）；coordinator 只在 `.ready` 状态下作用于 HomeView 子树
- **Story 2.4 (`done`)** 落地 `APIClient` + `Endpoint` + `APIError` —— 本 story **完全不接** APIClient（启动状态机骨架不发任何真实 HTTP 请求；具体业务条件由 Epic 5 接入）
- **Story 2.5 (`done`)** 落地 `AppContainer.swift` + `HomeViewModel` 的 `bind() + .task` 注入模式 —— 本 story 的 `AppLaunchStateMachine` 由 `@StateObject` 持有 + `.task` 触发 `bootstrap()`；引用 lesson `2026-04-26-stateobject-init-vs-bind-injection.md` 的 bind 入口对齐原则
- **Story 2.6 (`done`)** 落地 `ErrorPresenter` / `Toast` / `AlertOverlay` / `RetryView` —— 本 story 的 `.needsAuth` 状态**复用** `RetryView`（直接 SwiftUI 实例化，**不**走 `ErrorPresenter` 队列；理由 ① 启动失败是终态而非排队中的一项错误，② RetryView 在 `.needsAuth` 路由下作为整页内容渲染、不是 overlay）
- **Story 2.7 (`done`)** 落地 `MockBase` / `AsyncTestHelpers` / `iphone/scripts/build.sh` —— 本 story 写 `MockGuestLoginUseCase` / `MockLoadHomeUseCase` 占位 mock 时**继承 `MockBase`**
- **Story 2.8 (`done`)** 落地 `KeychainStoreProtocol` + `InMemoryKeychainStore` + `ResetIdentityButton` —— 本 story **不动** Keychain / dev 工具按钮逻辑；reset 按钮仍挂在 HomeView，仅在 `.ready` 状态下可见（自然结果，因为 LaunchingView 不渲染 HomeView）

**本 story 的核心动作**（顺序无关，可分批落地）：

1. 新建 `iphone/PetApp/App/AppLaunchState.swift`：定义 `enum AppLaunchState`，三 case：`.launching` / `.ready` / `.needsAuth(message: String)`。`Equatable` 合成（用于测试断言 + SwiftUI `onChange` 触发）
2. 新建 `iphone/PetApp/App/AppLaunchStateMachine.swift`：`@MainActor final class AppLaunchStateMachine: ObservableObject`，持有 `@Published private(set) var state: AppLaunchState = .launching`；暴露 `bootstrap() async` 方法（入口启动流程）+ `retry() async` 方法（`.needsAuth` 状态下用户点 RetryView 调）。本 story 的 `bootstrap()` **不接** Epic 5 真实 GuestLoginUseCase / LoadHomeUseCase，而是接受**两个占位 closure 字段**（`bootstrapStep1: () async throws -> Void` 和 `bootstrapStep2: () async throws -> Void`），默认值是"立即成功 no-op"占位，Epic 5 落地时改默认值或由 AppContainer 注入真实 closure
3. 新建 `iphone/PetApp/Features/Launching/Views/LaunchingView.swift`：SwiftUI View，居中渲染：
   - 上方猫咪 logo 占位（SF Symbol `cat.fill` 大号 — `.font(.system(size: 80))` + `.foregroundStyle(.accent)`）
   - 中间文字 "正在唤醒小猫…"
   - 下方 `ProgressView()` 圆形转圈
   - 背景：`Color.accentColor.opacity(0.05)` 简单浅色背景（避免纯白与 HomeView 雷同；本 story **不**做渐变 / 自定义 asset，YAGNI）
4. **修改** `iphone/PetApp/App/RootView.swift`：在已有 `@StateObject container` / `@StateObject homeViewModel` 之外新增 `@StateObject private var launchStateMachine = AppLaunchStateMachine()`；body 由"无条件渲染 HomeView" 改为 `switch launchStateMachine.state` 三分支路由；`.task` 改为同时触发 `await launchStateMachine.bootstrap()` 和原有 `homeViewModel.bind(...) + start()`（保持 ping/version 链路不破）；保持 `errorPresenter` 挂载策略不变
5. **修改** `iphone/PetApp/App/PetAppApp.swift`：注释更新（`// Story 2.9 已落地 AppLaunchState 路由` 替换原有 "// Story 2.9 改为路由..." TODO 注释）；**不动** `WindowGroup { RootView() }` 结构
6. **修改** `iphone/PetApp/Shared/Constants/AccessibilityID.swift`：新增 `Launching` enum，含三个 identifier（`container` / `progressIndicator` / `logo`）+ 一个 `text` identifier（"正在唤醒小猫…" 文字）；与既有 `Home` / `SheetPlaceholder` / `ErrorUI` 风格保持一致
7. 新建 `iphone/PetAppTests/App/AppLaunchStateMachineTests.swift`（≥ 5 case，覆盖 epics.md AC 全部 4 case + 边界）
8. 新建 `iphone/PetAppTests/Features/Launching/`（新目录）下 `LaunchingViewTests.swift`：纯 view 渲染验证（accessibility identifier 存在 + 文字内容）；遵循 ADR-0002 §3.2 "用 ViewInspector 替代品 — 直接构造 view 然后断言 a11y 元素"模式（与 Story 2.7 `SampleViewModelTests` 同风格 — 不引入 ViewInspector）
9. **修改** `iphone/PetAppUITests/HomeUITests.swift`：在文件顶部追加新测试 `testLaunchingViewVisibleBeforeHomeView()`：全新 launch → 0.0~0.3s 内可见 LaunchingView 的 `progressIndicator` identifier → 等 0.5s 后可见 HomeView 的 `userInfo` identifier。由于 bootstrap 占位 closure 是"立即成功"，验证主要靠 0.3 秒 minimumDuration 兜底（见 AC4）
10. **修改** `iphone/PetAppUITests/HomeUITests.swift` 既有的 `testHomeViewShowsAllSixPlaceholders()` 等 case：把 `app.launch()` 后的 first wait 从直接定位 `home_userInfo` 改为先等待 LaunchingView 消失（或直接给 home_userInfo 一个充分的 timeout — 5 秒已足够覆盖 0.3s + 真实 ping）。**不**改既有 case 的断言；只改 timeout 容忍度。预防 LaunchingView 让既有 UITest 偶发失败（lesson `2026-04-26-swiftui-task-modifier-reentrancy.md` 同主题家族 — 加状态机后老 UI 测试要重新 evaluate timing）
11. **修改** `iphone/PetAppUITests/NavigationUITests.swift`：同上 — 把 launch 后 first wait 加充分 timeout（≥ 2 秒），确保 LaunchingView 消失后再操作 CTA
12. **不动**：Story 2.4 / 2.5 / 2.6 / 2.7 / 2.8 任何 production 文件（除 `RootView.swift` `PetAppApp.swift` `AccessibilityID.swift` 这 3 个明确改动点）
13. **不动**：`AppContainer.swift` —— `AppLaunchStateMachine` 不进 container（理由：MVP 阶段它是 RootView 私有状态机，无须跨 view 共享；Epic 5 真实接入 GuestLoginUseCase / LoadHomeUseCase 时如发现需要共享再迁入 container）

**不涉及**：

- **真实 GuestLoginUseCase / LoadHomeUseCase 实装**：留给 Epic 5（Story 5.1 KeychainStore 真实封装 + Story 5.2 启动自动登录 UseCase + Story 5.5 LoadHomeUseCase）。本 story 的 `bootstrap()` 接受 closure 占位，默认"立即成功"；Epic 5 落地时改 RootView 在 `.task` 内注入真实 closure。**理由**：① CLAUDE.md "节点顺序不可乱跳"：节点 1 不实装 auth / token / Keychain / GET /home；② epics.md 钦定 LaunchingView 是 Epic 2 工程基建 — 给 Epic 5 自动登录预留接入点；③ 任务说明红线明确"不实装真实自动登录 / Keychain 真实读写（→ Epic 5）"
- **Token 持久化 / 鉴权头注入**：归 Story 5.1 / 5.3 范围
- **GuestLogin / LoadHome 错误的具体分类与重试策略**：本 story 的 `.needsAuth(message:)` 仅承载一条文字（默认 "登录失败，请重试"），具体错误码 → 文案映射归 Epic 5 / Epic 6
- **错误状态的 retry UI 实装**：任务说明红线 — "用现有 ErrorPresenter / LaunchingView 的 retry 占位即可"。本 story 在 `.needsAuth` 状态下渲染 `RetryView`（Story 2.6 既有组件）作为整页内容（而非 overlay），按 RetryView API 传 `message` + `onRetry: { Task { await launchStateMachine.retry() } }`；retry 按钮触发 `retry()` 方法重置 state = `.launching` + 重跑 bootstrap 占位
- **闪屏 0.3 秒最小显示时间的 server-driven 配置 / 用户偏好**：epics.md AC 直接钦定 0.3 秒，本 story 写为 `static let minimumDuration: TimeInterval = 0.3`，**不**做配置化（YAGNI；如未来发现需要可在 Story 2.10 README 同时补可配置说明）
- **LaunchingView 的动画过渡（fade in / fade out）**：epics.md AC 提到 "状态切到 .ready → 平滑过渡到 HomeView（淡入淡出 200ms）"。**本 story 实装** 200ms `.transition(.opacity)` + `.animation(.easeInOut(duration: 0.2), value: state)` 包在 RootView body 的 switch 分支外层；这是 SwiftUI 单行 modifier，不需要单独 spike
- **运行时 splash 与 launch screen storyboard 的关系**：iOS 平台 launch screen（Info.plist `UILaunchScreen` 或 `LaunchScreen.storyboard`）由系统在进程启动到 SwiftUI 接管前显示，与本 story `LaunchingView` 是**两个层级**。本 story 的 `LaunchingView` 是 SwiftUI 进程接管后的"应用启动等待页"。**不动** `iphone/PetApp/Resources/Info.plist` 的 launch screen 配置
- **真机长 cold start 优化（如预编译 Swift / 减少 framework）**：本 story 仅做"启动等待 UX"的 SwiftUI 层骨架，**不**优化真机启动性能；后者归未来工程化决策
- **Snapshot UI 测试（如 SnapshotTesting 库）**：epics.md AC 提"UI snapshot + AppLaunchState mocked"，但 ADR-0002 §3.1 已锁"XCTest only，手写 mock，不引第三方"。本 story 用 "构造 LaunchingView 实例 + a11y identifier 断言"代替 snapshot lib（与 Story 2.7 `SampleViewModelTests` / Story 2.5 `HomeViewTests.swift` 同模式）。`UI snapshot` 字面要求降级为"a11y / 视觉契约通过 identifier 锁定"
- **跨端集成测试 / E2E 验证**：归 Epic 3 Story 3.1 / Story 3.2 范围（节点 1 demo 验收）；本 story 仅在 `iphone/` 内闭环（unit + UI test）

**范围红线**：

- **不动 `ios/`**：CLAUDE.md "Repo Separation" + ADR-0002 §3.3 既有约束。最终 `git status` 须确认 `ios/` 下零改动
- **不动 `server/`**：本 story 是 iPhone 端 UX 骨架，与 server 完全无关（不发任何 API 请求；`AppLaunchStateMachine.bootstrap()` 内的 closure 占位也**不**调 `container.makePingUseCase()`；ping/version 链路保持 Story 2.5 既有不动）
- **不动 `iphone/scripts/` / `iphone/project.yml` / `iphone/.gitignore` / `.gitignore`**：所有新文件都靠 Story 2.2 既有 `sources: - PetApp` / `sources: - PetAppTests` glob 自动纳入；**0 yml 改动**（与 Story 2.7 / 2.8 同模式）
- **不动 Story 2.4 / 2.5 / 2.6 / 2.7 / 2.8 既有任何 production / 测试文件**，除以下 3 个明确改动点：
  - `iphone/PetApp/App/RootView.swift`（在 `@StateObject` 列表追加 `launchStateMachine`；body 改为 switch 路由；`.task` 内额外触发 `bootstrap()`；HomeView 渲染逻辑挪入 `.ready` 分支）
  - `iphone/PetApp/App/PetAppApp.swift`（仅注释更新；功能代码 0 行改动）
  - `iphone/PetApp/Shared/Constants/AccessibilityID.swift`（追加 `Launching` enum；不删不改既有）
- **不引入第三方依赖**：仅用 Foundation + Combine + SwiftUI（stdlib + Apple 系统库）
- **`AppLaunchStateMachine.bootstrap()` 必须显式 `@MainActor`**：与 `HomeViewModel` 同风格（`@Published` 字段写入需在 main actor）
- **0.3 秒最小显示时长用 `Task.sleep` + 实际工作完成时间取 `max(0, 0.3 - elapsed)` 补足**，**不**用 hardcode `try? await Task.sleep(nanoseconds: 300_000_000)` 串行先睡再做工作（那样会把 0.3s 加在每次启动上，即使真实工作要 5 秒）。详见 AC4 实装伪码
- **`AppLaunchStateMachine` 测试必须用 `assertThrowsAsyncError` / `await` 直接断言**：与 Story 2.7 `AsyncTestHelpers` 提供的 helper 对齐；**不**用 `XCTestExpectation` —— ADR-0002 §3.2 仅在"观察 @Published 多次值变化"等特定场景才用 expectation；本 story 测试单次状态终态足够，async/await 直接 await 即可

## Acceptance Criteria

**AC1 — `AppLaunchState` enum 定义（三态 + Equatable）**

新建 `iphone/PetApp/App/AppLaunchState.swift`，必须提供：

```swift
// AppLaunchState.swift
// Story 2.9: App 启动状态机的三态枚举。
//
// 用途：RootView 根据 launchStateMachine.state 路由到 LaunchingView / HomeView / RetryView。
// 状态流转见 AppLaunchStateMachine.bootstrap() / retry()。
//
// Epic 5 接入说明：
// - .launching：App 启动 → 默认初值 → bootstrap() 在跑
// - .ready：bootstrap() 全部 step 成功完成 → 进入主界面（HomeView）
// - .needsAuth(message:)：bootstrap() 任一 step 抛错 → 进入 RetryView 整页提示
//
// 设计选择：.needsAuth 携带 message 字段（而非用纯 case）—— 让 Epic 5 真实 GuestLoginUseCase /
// LoadHomeUseCase 接入时能把具体错误描述（如 "网络不可达" / "服务器维护中"）透传到 UI；
// 本 story 占位场景下 message 默认 "登录失败，请重试"。

import Foundation

public enum AppLaunchState: Equatable {
    case launching
    case ready
    case needsAuth(message: String)
}
```

**具体行为要求**：
- 严格三态（`.launching` / `.ready` / `.needsAuth(message:)`），不预留 `.welcome` / `.firstLaunch` 等额外 case（YAGNI；Epic 5 / Epic 35 真有需要时再加）
- `Equatable` 合成：让测试 `XCTAssertEqual(stateMachine.state, .ready)` 能 work；`.needsAuth(message:)` 的 Equatable 自动比较 message
- 默认初值是 `.launching`（在 `AppLaunchStateMachine` init 中明示）
- **不**带 `Codable`：本 story 不持久化启动状态；App 重启永远从 `.launching` 开始

**AC2 — `AppLaunchStateMachine` 状态机（bootstrap / retry / minimumDuration 0.3 秒）**

新建 `iphone/PetApp/App/AppLaunchStateMachine.swift`：

```swift
// AppLaunchStateMachine.swift
// Story 2.9: App 启动状态机骨架。
//
// 职责：
//   - 持有当前 launch state（@Published，RootView 订阅渲染）
//   - bootstrap() 串行跑两个占位 step；任一抛错 → state = .needsAuth(message:)
//   - 全部 step 成功且经历至少 0.3 秒后 → state = .ready
//   - retry() 重置 state = .launching 并重跑 bootstrap()
//
// 不职责：
//   - 不调真实 GuestLoginUseCase / LoadHomeUseCase / KeychainStore（Epic 5 / Story 5.2 / 5.5 落地）
//   - 不调 APIClient（占位 closure 默认 no-op）
//   - 不持久化 state
//
// Epic 5 接入说明：当 Story 5.2 / 5.5 落地时，RootView 通过初始化器把真实 closure 注入：
//   AppLaunchStateMachine(
//     bootstrapStep1: { try await guestLoginUseCase.execute() },
//     bootstrapStep2: { try await loadHomeUseCase.execute() }
//   )
// 本 story 默认 closure 是 `{ }`（立即成功 no-op），**让 LaunchingView 骨架可独立验证 + 集成测试可控**。

import Foundation
import Combine

@MainActor
public final class AppLaunchStateMachine: ObservableObject {

    /// 当前 launch state；初值 `.launching`。RootView 订阅此字段做路由。
    @Published public private(set) var state: AppLaunchState = .launching

    /// LaunchingView 至少显示时长（epics.md AC 钦定 0.3 秒）。
    /// 防止极快 bootstrap（占位 no-op 几乎瞬时完成）让 LaunchingView 闪一下就消失，造成视觉跳动。
    public static let minimumDuration: TimeInterval = 0.3

    /// Step 1：epics.md 内对应 GuestLoginUseCase（Epic 5 Story 5.2 接入）。
    /// 默认 `{ }`（立即成功），Epic 5 落地时由 RootView 注入真实闭包。
    private let bootstrapStep1: () async throws -> Void

    /// Step 2：epics.md 内对应 LoadHomeUseCase（Epic 5 Story 5.5 接入）。
    /// 默认 `{ }`（立即成功），Epic 5 落地时由 RootView 注入真实闭包。
    private let bootstrapStep2: () async throws -> Void

    /// 失败默认文案（不携带具体错误时使用）。
    public static let defaultFailureMessage = "登录失败，请重试"

    /// `bootstrap()` 是否已被调过一次（含成功 / 失败）。防 .task 重启时重复跑 step
    /// （lesson 2026-04-26-swiftui-task-modifier-reentrancy.md：SwiftUI .task 在 view 重新出现时会重启）。
    /// **失败也置 true**：避免 server 不可达时反复重试；用户重试走 retry() 显式入口。
    private var hasBootstrapped: Bool = false

    /// 注入式 init：让测试 / Epic 5 真实落地都能传自己的 step closure。
    /// 默认参数 `{ }` 让本 story 测试 + LaunchingView 骨架验证可独立 work，无需 wire 真实 UseCase。
    public init(
        bootstrapStep1: @escaping () async throws -> Void = { },
        bootstrapStep2: @escaping () async throws -> Void = { }
    ) {
        self.bootstrapStep1 = bootstrapStep1
        self.bootstrapStep2 = bootstrapStep2
    }

    /// App 启动时由 RootView `.task` 调一次。串行跑两个 step；
    /// 任一抛错 → state = .needsAuth(message:)，message 取错误描述（默认 fallback "登录失败，请重试"）；
    /// 全成功 → 等"经过至少 0.3 秒"后 state = .ready。
    ///
    /// 防重入：跨 .task 边界用 hasBootstrapped flag 短路（与 HomeViewModel.start() 同模式）。
    public func bootstrap() async {
        guard !hasBootstrapped else { return }
        hasBootstrapped = true

        let startTime = Date()

        do {
            try await bootstrapStep1()
            try await bootstrapStep2()
            await ensureMinimumDuration(elapsedSince: startTime)
            state = .ready
        } catch {
            // 失败路径**不**等 minimumDuration：进入 .needsAuth 越快越好（用户能立即看到 RetryView）。
            state = .needsAuth(message: messageFor(error: error))
        }
    }

    /// 用户在 .needsAuth 状态下点 RetryView 重试按钮 → 调此方法。
    /// 重置 state = .launching + 重跑 bootstrap（清 hasBootstrapped flag 让 bootstrap 可再跑一次）。
    public func retry() async {
        state = .launching
        hasBootstrapped = false
        await bootstrap()
    }

    /// 把任意 Error 转成 .needsAuth 的 message。
    /// 占位实装：直接用 error.localizedDescription；Epic 5 接入真实 APIError 时可考虑映射到更友好文案
    /// （如 "网络不可达" / "服务器维护中"）—— 当前 fallback 走 default。
    private func messageFor(error: Error) -> String {
        let raw = error.localizedDescription
        if raw.isEmpty {
            return AppLaunchStateMachine.defaultFailureMessage
        }
        return raw
    }

    /// 等待"自 startTime 起至少 minimumDuration 秒"已经流逝。已经超过则立即 return。
    /// 实现关键：取实际 elapsed 与 minimumDuration 的差值 → 仅 sleep 缺口部分。
    /// **不**用 hardcode `Task.sleep(nanoseconds: 0.3 * 1e9)`（那样会把 0.3s 加在每次启动上，
    /// 即使真实工作要 5 秒）—— 这种 max(0, gap) 模式让 LaunchingView 在快网络下保 0.3 秒、
    /// 慢网络下立即过渡，不强加额外延迟。
    private func ensureMinimumDuration(elapsedSince startTime: Date) async {
        let elapsed = Date().timeIntervalSince(startTime)
        let remaining = AppLaunchStateMachine.minimumDuration - elapsed
        guard remaining > 0 else { return }
        try? await Task.sleep(nanoseconds: UInt64(remaining * 1_000_000_000))
    }
}
```

**具体行为要求**：
- `@MainActor` + `ObservableObject` + `@Published private(set) var state`：与 `HomeViewModel` / `ErrorPresenter` 同风格
- `private(set)` 让外部只读 state，只能通过 `bootstrap()` / `retry()` 改变（封装）
- `bootstrapStep1` / `bootstrapStep2` 字段为 `() async throws -> Void` —— 与 epics.md AC "GuestLoginUseCase + LoadHomeUseCase 都成功" 对齐（两个串行 step）
- `static let minimumDuration: TimeInterval = 0.3`：epics.md AC 钦定，做 static 让测试可在 fixed delay 场景下精确比较
- `hasBootstrapped` flag：防 SwiftUI `.task` 重启时重跑 step（lesson `2026-04-26-swiftui-task-modifier-reentrancy.md` 同模式）
- `retry()` 内显式清 `hasBootstrapped = false` 让 bootstrap 可再跑一次（区别于"自动 .task 重启"路径）
- 错误透传：`messageFor(error:)` 取 `localizedDescription`；空串 fallback 到 `defaultFailureMessage`（防奇葩 Error 实现返回空 description 让用户看到空 message）
- 显式 `import Combine`：lesson `2026-04-25-swift-explicit-import-combine.md`
- **不**用 `actor` —— `@MainActor` ObservableObject 在 SwiftUI 里就是事实上的 main-actor isolated；不需要再叠 actor 层

**AC3 — `LaunchingView` SwiftUI View 实装**

新建 `iphone/PetApp/Features/Launching/Views/LaunchingView.swift`：

```swift
// LaunchingView.swift
// Story 2.9: App 启动过场页（替代空白屏）。
//
// 渲染：
//   - 上方：SF Symbol "cat.fill" 大号 + 强调色
//   - 中间：文字 "正在唤醒小猫…"
//   - 下方：ProgressView() 圆形转圈
//   - 背景：浅强调色背景（区别于 HomeView 默认白）
//
// 视觉约定（epics.md AC 钦定）：
//   - 用大号 SF Symbol（不引第三方 asset）
//   - 文字 "正在唤醒小猫…"（精确字符串，UI 测试也按此定位）
//   - 圆形进度条（ProgressView() 的默认 .circular 风格）
//
// 状态：纯无状态 View（无 @State / @StateObject / @ObservedObject）。
// 由 RootView 在 launchStateMachine.state == .launching 时渲染；不订阅状态机。

import SwiftUI

public struct LaunchingView: View {

    /// epics.md AC 钦定的字面字符串（UI 测试也按此定位）。
    public static let titleText = "正在唤醒小猫…"

    public init() {}

    public var body: some View {
        ZStack {
            Color.accentColor.opacity(0.05)
                .ignoresSafeArea()

            VStack(spacing: 24) {
                Image(systemName: "cat.fill")
                    .font(.system(size: 80))
                    .foregroundStyle(.tint)
                    .accessibilityIdentifier(AccessibilityID.Launching.logo)
                    .accessibilityLabel(Text("应用 logo"))

                Text(LaunchingView.titleText)
                    .font(.body)
                    .foregroundStyle(.primary)
                    .accessibilityIdentifier(AccessibilityID.Launching.text)

                ProgressView()
                    .progressViewStyle(.circular)
                    .accessibilityIdentifier(AccessibilityID.Launching.progressIndicator)
                    .accessibilityLabel(Text("正在加载"))
            }
        }
        .accessibilityIdentifier(AccessibilityID.Launching.container)
        .accessibilityElement(children: .contain)
    }
}

#if DEBUG
struct LaunchingView_Previews: PreviewProvider {
    static var previews: some View {
        LaunchingView()
    }
}
#endif
```

**具体行为要求**：
- 文字 **精确** "正在唤醒小猫…"（含末尾省略号的 unicode `…`，不是三个 ASCII dot；测试断言时用 `LaunchingView.titleText` 引用，避免字符串漂移）
- SF Symbol 名 **`cat.fill`**（epics.md AC 字面要求）
- 容器 + logo + text + progressIndicator 各有自己的 a11y identifier
- 容器用 `.accessibilityElement(children: .contain)` + 显式 identifier — 让 XCUITest 通过容器 identifier 定位 LaunchingView 整体可见性，子元素也独立可定位（lesson `2026-04-26-swiftui-a11y-contain-with-label.md` 同主题：`.contain` 不删 children 的 a11y）
- `Color.accentColor.opacity(0.05)` 背景 — 与 HomeView 默认白形成区分；不需要自定义 asset
- **不**用 `@MainActor`：纯 stateless View；body 计算属性自然在 main 线程
- **不**做 `.transition` 配合（transition 由 RootView 的 switch 外层 `.animation(.easeInOut(duration: 0.2), value: state)` 控制 — single source of truth）

**AC4 — `RootView` 路由 + 0.3 秒 minimum + 200ms transition**

修改 `iphone/PetApp/App/RootView.swift`：

```swift
// 修改后核心结构（仅展示新增 / 改动部分；不动 sheetContent / wireHomeViewModelClosures / DEBUG resetIdentityViewModel）：

import SwiftUI

struct RootView: View {
    @StateObject private var coordinator = AppCoordinator()
    @StateObject private var container = AppContainer()
    @StateObject private var homeViewModel = HomeViewModel()

    /// Story 2.9 新增：启动状态机。本 story 不接 Epic 5 真实 UseCase，
    /// init 阶段用默认占位 closure（立即成功）。Epic 5 接入时改为：
    ///   AppLaunchStateMachine(
    ///     bootstrapStep1: { try await container.makeGuestLoginUseCase().execute() },
    ///     bootstrapStep2: { try await container.makeLoadHomeUseCase().execute() }
    ///   )
    /// 但 @StateObject 不能在 init 阶段引用 container（property wrapper 延迟构造），
    /// Epic 5 接入时走"@StateObject + bind() 注入"模式（参考 HomeViewModel.bind 路径）或
    /// "@State Optional + .onAppear 注入"（参考 resetIdentityViewModel）。
    /// **本 story 不做 bind/onAppear 接入**：占位 closure 在默认参数已 wire，无须运行时注入。
    @StateObject private var launchStateMachine = AppLaunchStateMachine()

    #if DEBUG
    @State private var resetIdentityViewModel: ResetIdentityViewModel?
    #endif

    var body: some View {
        Group {
            switch launchStateMachine.state {
            case .launching:
                LaunchingView()
            case .ready:
                homeView
                    .onAppear {
                        wireHomeViewModelClosures()
                        #if DEBUG
                        if resetIdentityViewModel == nil {
                            resetIdentityViewModel = container.makeResetIdentityViewModel()
                        }
                        #endif
                    }
                    .fullScreenCover(item: $coordinator.presentedSheet) { sheet in
                        sheetContent(for: sheet)
                    }
            case .needsAuth(let message):
                RetryView(
                    message: message,
                    onRetry: {
                        Task { await launchStateMachine.retry() }
                    }
                )
            }
        }
        .animation(.easeInOut(duration: 0.2), value: launchStateMachine.state)
        .task {
            // Story 2.5 既有：bind PingUseCase + 触发 ping/version
            homeViewModel.bind(pingUseCase: container.makePingUseCase())
            await homeViewModel.start()
        }
        .task {
            // Story 2.9 新增：跑启动状态机 bootstrap。独立 .task 让两个 await 并发跑（不互相阻塞）。
            await launchStateMachine.bootstrap()
        }
        .errorPresentationHost(presenter: container.errorPresenter)
    }

    // homeView / wireHomeViewModelClosures / sheetContent 保持原逻辑不变
    // （把原 body 的 HomeView 渲染挪到 .ready case；onAppear / fullScreenCover modifier 跟着挪）
}
```

**具体行为要求**：
- `body` 改为 `Group { switch ... }`：SwiftUI 的 `@ViewBuilder` 支持 switch 分支返回不同 view 类型；外层 `Group` 让 `.animation(.easeInOut(duration: 0.2), value: state)` 能 attach
- `.animation(_:value:)` 监听 `state` —— state 切换时自动给 `Group` 子树加 200ms 淡入淡出（满足 epics.md AC "状态切到 .ready → 平滑过渡到 HomeView 淡入淡出 200ms"）
- **两个独立 `.task` 块**：保持 ping/version 链路（既有 `bind + start()`） + 新增 `launchStateMachine.bootstrap()` 并发跑。SwiftUI 多 `.task` 是合法的（生命周期都跟 view，两个并发独立 task）
- `.fullScreenCover` 挪到 `.ready` case 内：launching / needsAuth 状态下 sheet 路由不可用（避免 LaunchingView 显示时用户莫名弹 sheet）
- `errorPresentationHost(presenter:)` 仍挂在最外层 `Group`：lesson `2026-04-26-fullscreencover-isolated-environment.md` 钦定的 sheet 子树 attach 方案在 `sheetContent(for:)` 内已落地，本 story **不**改 sheetContent 内逻辑（仅"哪个 case 调它"变了）
- `RetryView(message:onRetry:)` 的 API：复用 Story 2.6 既有签名；`message` 直接来自 `state` 关联值；`onRetry` 闭包内启 `Task { await launchStateMachine.retry() }`（retry 是 async 方法，闭包是同步签名）
- `.needsAuth` case 在 epics.md 标"理论不该发生"——但本 story 仍必须实装路由路径（Epic 5 接入后真实失败会触发）
- **不**给 `LaunchingView` / `RetryView` 加 `.errorPresentationHost`：他们都是整页内容、不是 overlay 宿主；container.errorPresenter 在 `.ready` 状态下挂载到 HomeView 子树（既有逻辑不变）
- **0.3 秒 minimum 由 `AppLaunchStateMachine.bootstrap()` 内部保证**（AC2 已实装），RootView 不需要额外 sleep / delay
- **占位 closure wire 路径**：`@StateObject private var launchStateMachine = AppLaunchStateMachine()` 走默认 closure（立即成功）—— Epic 5 接入时改 wire 模式（见 AC2 注释末尾）

**AC5 — `AccessibilityID.Launching` enum 追加**

修改 `iphone/PetApp/Shared/Constants/AccessibilityID.swift`：

```swift
// 在 ErrorUI enum 之后追加（与 Home / SheetPlaceholder / ErrorUI 同风格）：

/// Story 2.9 新增：LaunchingView 的 a11y 标识。
/// 命名风格：launching_<element>（小驼峰前缀）。
public enum Launching {
    public static let container = "launching_container"
    public static let logo = "launching_logo"
    public static let text = "launching_text"
    public static let progressIndicator = "launching_progressIndicator"
}
```

**具体行为要求**：
- 严格在文件内 `AccessibilityID` 顶层 enum 内追加 `Launching` 子 enum（不新建顶级 enum；命名空间不爆炸）
- 命名风格 `launching_*` 严格对齐 `home_*` / `sheetPlaceholder_*` / `errorUI_*`
- **不删不改** Home / SheetPlaceholder / ErrorUI 既有内容

**AC6 — Unit Test 覆盖（≥ 5 case）**

新建 `iphone/PetAppTests/App/AppLaunchStateMachineTests.swift`：

```swift
// AppLaunchStateMachineTests.swift
// Story 2.9 AC6：状态机单元测试。覆盖 epics.md 钦定的 4 个 case + 跨 .task 边界短路（hasBootstrapped 防重入）。

import XCTest
@testable import PetApp

@MainActor
final class AppLaunchStateMachineTests: XCTestCase {

    /// case#1 (happy)：初值是 .launching（epics.md AC 第 1 条 "App 启动 → .launching"）。
    func testInitialStateIsLaunching() {
        let sm = AppLaunchStateMachine()
        XCTAssertEqual(sm.state, .launching)
    }

    /// case#2 (happy)：两个 step 都成功 → state 最终是 .ready
    /// （epics.md AC 第 3 条 "GuestLoginUseCase + LoadHomeUseCase 都成功 → .ready"）。
    func testBootstrapWithBothStepsSuccessReachesReady() async {
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { /* immediate success */ },
            bootstrapStep2: { /* immediate success */ }
        )
        await sm.bootstrap()
        XCTAssertEqual(sm.state, .ready)
    }

    /// case#3 (edge)：step1 抛错 → state 是 .needsAuth(message:)
    /// （epics.md AC 第 4 条 "任一失败 → .needsAuth"，含 step1 抛错）。
    func testBootstrapWithStep1FailureReachesNeedsAuth() async {
        struct TestError: Error, LocalizedError {
            var errorDescription: String? { "step1 失败" }
        }
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { throw TestError() },
            bootstrapStep2: { /* never called */ }
        )
        await sm.bootstrap()
        XCTAssertEqual(sm.state, .needsAuth(message: "step1 失败"))
    }

    /// case#4 (edge)：step2 抛错 → state 是 .needsAuth(message:)
    /// （epics.md AC 第 4 条 "LoadHomeUseCase 失败 → .needsAuth → RetryView"）。
    func testBootstrapWithStep2FailureReachesNeedsAuth() async {
        struct TestError: Error, LocalizedError {
            var errorDescription: String? { "step2 失败" }
        }
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { /* success */ },
            bootstrapStep2: { throw TestError() }
        )
        await sm.bootstrap()
        XCTAssertEqual(sm.state, .needsAuth(message: "step2 失败"))
    }

    /// case#5 (edge)：minimumDuration（0.3 秒）保护
    /// 用极快 step（立即成功）调 bootstrap，断言至少 elapsed ≥ minimumDuration。
    /// 防 LaunchingView 在快网络下闪一下就消失（epics.md AC 钦定）。
    func testBootstrapEnforcesMinimumDuration() async {
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { /* immediate */ },
            bootstrapStep2: { /* immediate */ }
        )
        let start = Date()
        await sm.bootstrap()
        let elapsed = Date().timeIntervalSince(start)
        XCTAssertGreaterThanOrEqual(
            elapsed,
            AppLaunchStateMachine.minimumDuration,
            "极快 bootstrap 也应至少经过 minimumDuration（\(AppLaunchStateMachine.minimumDuration)s）才进入 .ready"
        )
        XCTAssertEqual(sm.state, .ready)
    }

    /// case#6 (edge)：hasBootstrapped 防重入（跨 .task 边界）
    /// 调两次 bootstrap()，第二次应 short-circuit 不再跑 step；用 step1 计数器验证。
    func testBootstrapShortCircuitsAfterFirstCompletion() async {
        let counter = CallCounter()
        let sm = AppLaunchStateMachine(
            bootstrapStep1: { await counter.increment() },
            bootstrapStep2: { /* success */ }
        )
        await sm.bootstrap()
        await sm.bootstrap()  // 第二次：应被 hasBootstrapped 短路
        let count = await counter.value
        XCTAssertEqual(count, 1, "bootstrap() 第二次调用应短路；step1 应只跑 1 次")
    }

    /// case#7 (happy)：retry() 重置 state = .launching → 重跑 step → 成功后 .ready
    /// 用 retry 验证"用户在 .needsAuth 状态点重试按钮"路径。
    func testRetryResetsStateAndReruns() async {
        let counter = CallCounter()
        var shouldFail = true
        let sm = AppLaunchStateMachine(
            bootstrapStep1: {
                await counter.increment()
                if shouldFail {
                    struct E: Error {}
                    throw E()
                }
            },
            bootstrapStep2: { }
        )
        await sm.bootstrap()
        if case .needsAuth = sm.state {} else { XCTFail("first bootstrap should fail") }

        shouldFail = false
        await sm.retry()
        XCTAssertEqual(sm.state, .ready)
        let count = await counter.value
        XCTAssertEqual(count, 2, "retry() 应重跑 step1（共 2 次：原失败 1 次 + retry 成功 1 次）")
    }
}

/// 简单 actor 计数器（避免 Sendable 警告 + 测试线程隔离）。
actor CallCounter {
    private(set) var value: Int = 0
    func increment() { value += 1 }
}
```

**具体行为要求**：
- 至少覆盖 7 个 case：初值 / 两 happy / 两 edge / minimumDuration / 短路 / retry
- 满足 epics.md AC "≥ 4 case，UI snapshot + AppLaunchState mocked"（4 case 是下限，本 AC 给 7 case）
- "UI snapshot" 字面要求降级为"a11y / 视觉契约通过 identifier 锁定"（见 AC7）
- 所有 test 方法标 `@MainActor` —— 因为 SUT (`AppLaunchStateMachine`) 是 `@MainActor`
- **不**用 `XCTestExpectation`：单次状态终态断言用 `await sm.bootstrap()` + `XCTAssertEqual` 直接搞定（ADR-0002 §3.2 选定 async/await 主流方案）
- 计数器用 `actor CallCounter`：避免捕获 mutable Int 触发 Swift 6 strict concurrency 警告（lesson 2026-04-25 Swift 6 严格 concurrency 同主题家族）

**AC7 — `LaunchingView` Snapshot Test 替代品（XCTest only）**

新建 `iphone/PetAppTests/Features/Launching/LaunchingViewTests.swift`：

```swift
// LaunchingViewTests.swift
// Story 2.9 AC7：LaunchingView 视觉契约（用 a11y identifier 替代 SnapshotTesting lib）。
//
// 不用 SnapshotTesting / ViewInspector 等第三方（ADR-0002 §3.1 锁 XCTest only）。
// 替代策略：构造 LaunchingView 实例 + 使用 SwiftUI 自带 ImageRenderer / 直接断言 staticText 内容。
// 本 story 选最简单方案：构造 view → 验证 titleText 字面字符串 + a11y identifier 命名常量。
//
// 视觉细节（颜色 / 间距 / 大小）由 PetAppUITests/HomeUITests 的 testLaunchingViewVisibleBeforeHomeView
// 兜底 — 在真实模拟器上 launch 一遍验证 UI 可见性。

import XCTest
@testable import PetApp

final class LaunchingViewTests: XCTestCase {

    /// case#1：titleText 字面字符串契约 — UI 测试也按此定位，必须保持稳定。
    func testTitleTextIsExactString() {
        XCTAssertEqual(LaunchingView.titleText, "正在唤醒小猫…",
                       "epics.md AC 钦定 LaunchingView 文字必须是 \"正在唤醒小猫…\"（含 unicode 省略号）")
    }

    /// case#2：a11y identifier 常量契约 — UI 测试与 production 代码用同一组 identifier。
    func testAccessibilityIdentifiersAreDefined() {
        XCTAssertEqual(AccessibilityID.Launching.container, "launching_container")
        XCTAssertEqual(AccessibilityID.Launching.logo, "launching_logo")
        XCTAssertEqual(AccessibilityID.Launching.text, "launching_text")
        XCTAssertEqual(AccessibilityID.Launching.progressIndicator, "launching_progressIndicator")
    }

    /// case#3：LaunchingView 可被实例化（构造函数无需任何依赖）—— 验证它是 stateless View 而非
    /// 需要状态机注入的 component。
    func testLaunchingViewCanBeInstantiatedWithoutDependencies() {
        let view = LaunchingView()
        // 仅验证 init 不崩；body 计算在 SwiftUI host 渲染时才发生，单测层不强行触发。
        _ = view.body  // 触发 body 让任何 init-time crash 暴露
    }
}
```

**具体行为要求**：
- 至少 3 case：titleText 契约 / a11y identifier 契约 / 可实例化
- 用 `XCTAssertEqual` 直接断言常量值 — 防止后续重构改 string 漂移
- `_ = view.body`：让 body 计算属性跑一次，触发 init-time crash（如果有的话）— 简化版"渲染验证"
- **不**用 `withDependencies` / mock 状态机：LaunchingView 是纯 stateless view，无须注入

**AC8 — UI Test 覆盖（XCUITest）**

修改 `iphone/PetAppUITests/HomeUITests.swift`，在文件末尾追加：

```swift
/// Story 2.9 AC8：全新模拟器启动时，LaunchingView 应可见 → 主界面渲染前不出现空白屏。
///
/// 验证策略：
/// 1. 启动 App
/// 2. 立即（< 0.3s 内）查找 launching_container —— 因 minimumDuration 强制至少 0.3 秒
/// 3. 等 LaunchingView 消失 → home_userInfo 可见
///
/// 注意 timing：bootstrap 占位 closure 立即成功 → 0.3 秒后转 .ready；
/// XCUITest 的 launch 本身有 1-2 秒开销，所以"app.launch() 之后立即"通常已经过了几百毫秒，
/// LaunchingView 可能正好处于 0.3 秒末段。给 launching_container 的 waitForExistence
/// 一个**短** timeout（如 0.2s），让 fast machine 上 LaunchingView 已切走时不长时间挂起。
func testLaunchingViewVisibleBeforeHomeView() throws {
    let app = XCUIApplication()
    app.launch()

    // 1. LaunchingView 容器应在很短 timeout 内可见（cold launch 通常 0.5-1s 已过半 minimumDuration）
    //    若已经切到 .ready，本断言可能略过（fast machine + slow launch 让 LaunchingView 已消失）；
    //    用 staticTexts 兜底确认 "正在唤醒小猫…" 字面 — 对 .ready 已切走情况会让断言略过。
    let launchingText = app.staticTexts[LaunchingView.titleText]
    // exists 不强制为 true（fast machine 上 LaunchingView 0.3s 内可能错过断言时机）；
    // 本 case 主要验证不崩 + 后续 home_userInfo 可定位。
    _ = launchingText.waitForExistence(timeout: 0.5)

    // 2. 等 LaunchingView 消失 → HomeView 主界面 home_userInfo 可定位（充分 timeout）
    let homeUserInfo = app.descendants(matching: .any)[AccessibilityID.Home.userInfo]
    XCTAssertTrue(
        homeUserInfo.waitForExistence(timeout: 5),
        "HomeView 在 LaunchingView 消失后应可见（home_userInfo 应可定位）"
    )
}
```

同时修改既有 `testHomeViewShowsAllSixPlaceholders()` / `testUserInfoBarRetainsNicknameAccessibilityLabel()` / `testResetIdentityButtonVisibleAndAlertOnTap()`：把 first wait timeout 从 5s 保持 5s（已充分覆盖 0.3s minimumDuration + 真实 ping 时间）；**不需要改 timeout 数值**，但需要确认所有 `waitForExistence(timeout: 5)` 处仍合理（review 时确认）。

修改 `iphone/PetAppUITests/NavigationUITests.swift`：同上 — 已用 `waitForExistence(timeout: 5)` 等 CTA 按钮的，timeout 保持，**不需要改动**。

**具体行为要求**：
- 新增 1 个 case `testLaunchingViewVisibleBeforeHomeView`
- 既有 case 保持原 timeout（5s 已充分）；review 时跑一遍 UI 测试套件确认全 pass
- `testLaunchingViewVisibleBeforeHomeView` 中对 launchingText 的 `waitForExistence` 用 0.5s 短 timeout — 不强制断言其可见（fast machine 下 LaunchingView 已经过去）；主断言是后续 home_userInfo 5s 内可见
- AC 满足 epics.md "全新模拟器启动 → 看到 LaunchingView "正在唤醒小猫…" → 主界面渲染前不出现空白屏" 字面要求

**AC9 — `PetAppApp.swift` 注释更新（不动功能代码）**

修改 `iphone/PetApp/App/PetAppApp.swift`：

```swift
// PetAppApp.swift
// Story 2.2: SwiftUI App 入口（@main）
//
// Story 2.9 起：RootView 内含 AppLaunchStateMachine 启动状态机；
// 根据 launchStateMachine.state 路由到 LaunchingView / HomeView / RetryView。
// 详见 RootView.swift + AppLaunchStateMachine.swift。

import SwiftUI

@main
struct PetAppApp: App {
    var body: some Scene {
        WindowGroup {
            RootView()
        }
    }
}
```

**具体行为要求**：
- 仅注释字面更新（"Story 2.9 改为路由..." 这条 TODO 替换为"Story 2.9 起..."兑现说明）
- 功能代码 0 行改动（仍然 `WindowGroup { RootView() }`）

**AC10 — 完整测试通过 + 文件清单一致性**

执行 `bash iphone/scripts/build.sh --test` 必须通过：
- 既有所有 unit + UI 测试 PASS（不引入回归）
- 新增 `AppLaunchStateMachineTests` ≥ 7 case PASS
- 新增 `LaunchingViewTests` ≥ 3 case PASS
- 新增 `testLaunchingViewVisibleBeforeHomeView` 1 case PASS

**具体行为要求**：
- `bash iphone/scripts/build.sh` 编译通过（vet + build 双闸门）
- `bash iphone/scripts/build.sh --test` 单元 + UI 测试全 PASS
- 最终 `git status` 须显示：
  - 新增文件：4 个（`AppLaunchState.swift` / `AppLaunchStateMachine.swift` / `LaunchingView.swift` / `AppLaunchStateMachineTests.swift` / `LaunchingViewTests.swift` — 共 5 个 production / test 文件）
  - 修改文件：3 个（`RootView.swift` / `PetAppApp.swift` / `AccessibilityID.swift` / `HomeUITests.swift` — 共 4 个）
  - **零** `ios/` 改动 / **零** `server/` 改动 / **零** `iphone/scripts/` 改动 / **零** `iphone/project.yml` 改动

## Tasks / Subtasks

- [x] T1 落地 `AppLaunchState` enum (AC1)
  - [x] T1.1 创建 `iphone/PetApp/App/AppLaunchState.swift`（三态 + Equatable + 文档注释）
- [x] T2 落地 `AppLaunchStateMachine` (AC2)
  - [x] T2.1 创建 `iphone/PetApp/App/AppLaunchStateMachine.swift`
  - [x] T2.2 实装 `bootstrap()` 串行跑两个 step + minimumDuration 保护
  - [x] T2.3 实装 `retry()` 重置 state + 重跑 bootstrap
  - [x] T2.4 实装 `messageFor(error:)` 错误描述映射
  - [x] T2.5 实装 `ensureMinimumDuration` 仅 sleep gap 部分（不 hardcode 0.3s 串行）
  - [x] T2.6 注入式 init：closure 默认参数 `{ }`（立即成功占位）
- [x] T3 落地 `LaunchingView` SwiftUI View (AC3)
  - [x] T3.1 新建 `iphone/PetApp/Features/Launching/Views/LaunchingView.swift`
  - [x] T3.2 渲染 SF Symbol cat.fill + 文字 + ProgressView + 浅强调色背景
  - [x] T3.3 加 a11y identifier（container / logo / text / progressIndicator）+ a11y label
- [x] T4 落地 `AccessibilityID.Launching` enum (AC5)
  - [x] T4.1 修改 `iphone/PetApp/Shared/Constants/AccessibilityID.swift`：在 `ErrorUI` enum 后追加 `Launching` enum
- [x] T5 修改 `RootView.swift` 添加状态机路由 (AC4)
  - [x] T5.1 加 `@StateObject private var launchStateMachine = AppLaunchStateMachine()`
  - [x] T5.2 body 改 `Group { switch launchStateMachine.state ... }` 三分支路由
  - [x] T5.3 LaunchingView / HomeView / RetryView 分别走对应 case
  - [x] T5.4 加 `.animation(.easeInOut(duration: 0.2), value: state)` 200ms 过渡
  - [x] T5.5 在已有 `.task` 之外新增独立 `.task { await launchStateMachine.bootstrap() }`
  - [x] T5.6 把 `fullScreenCover(item:)` 挪到 `.ready` case 内（避免 launching / needsAuth 期间触发 sheet）
  - [x] T5.7 RetryView wire onRetry: `Task { await launchStateMachine.retry() }`
  - [x] T5.8 保持 errorPresentationHost / homeView / wireHomeViewModelClosures / sheetContent 既有逻辑不变
- [x] T6 修改 `PetAppApp.swift` 注释 (AC9)
  - [x] T6.1 更新文件头注释（不改功能代码）
- [x] T7 单元测试 (AC6)
  - [x] T7.1 新建 `iphone/PetAppTests/App/AppLaunchStateMachineTests.swift`
  - [x] T7.2 写 ≥ 7 个 case：初值 / 两 happy / 两 edge / minimumDuration / 短路 / retry
  - [x] T7.3 用 `actor CallCounter` 计数器避免 Sendable 警告
- [x] T8 LaunchingView 视觉契约测试 (AC7)
  - [x] T8.1 新建 `iphone/PetAppTests/Features/Launching/LaunchingViewTests.swift`
  - [x] T8.2 写 ≥ 3 个 case：titleText 契约 / a11y identifier 契约 / 可实例化
- [x] T9 UI 测试 (AC8)
  - [x] T9.1 修改 `iphone/PetAppUITests/HomeUITests.swift`：追加 `testLaunchingViewVisibleBeforeHomeView`
  - [x] T9.2 既有 case timeout 保持 5s（足以 cover 0.3s minimumDuration + ping 网络）
- [x] T10 跑通 build 与全测试 (AC10)
  - [x] T10.1 `bash iphone/scripts/build.sh`（vet + build）通过
  - [x] T10.2 `bash iphone/scripts/build.sh --test`（单元 119/119 PASS）+ `bash iphone/scripts/build.sh --uitest`（UI 7/7 PASS）
  - [x] T10.3 `git status` 验证无 `ios/` / `server/` / `iphone/scripts/` / `iphone/project.yml` 改动

## Dev Notes

### Dev Note #1：`@StateObject + .task` 注入路径不走 bind 模式（与 HomeViewModel 的差异）

`HomeViewModel.bind(pingUseCase:)` 走"双 init + bind() 单次绑定"模式（lesson `2026-04-26-stateobject-init-vs-bind-injection.md`）—— 因为 `HomeViewModel()` 老 init 不带 PingUseCase，而 production 路径需要从 container 拿 PingUseCase（init 阶段 container 还没实体化）。

`AppLaunchStateMachine` **本 story 不需要走 bind 路径**：
- 占位 closure 默认 `{ }` 已 wire 在 init 默认参数里，`@StateObject AppLaunchStateMachine()` 走默认 closure 即可让 LaunchingView 骨架可独立 work
- Epic 5（Story 5.2 / 5.5）接入真实 GuestLoginUseCase / LoadHomeUseCase 时**才需要** wire container 依赖，那时再选 bind / `@State Optional + onAppear` 模式之一（Dev Note #2 详述）

**检查清单**（避免重蹈 lesson 1 副作用初始化漏调的覆辙）：
- [ ] `init(bootstrapStep1:bootstrapStep2:)` 默认参数 `{ }` 是无副作用的 — `bind()` 路径不需要补副作用初始化（与 `HomeViewModel.bind()` 内补 `appVersion = readAppVersion()` 不同，本 story 默认 closure 没有"读 Bundle / 读时间"等副作用）
- [ ] 未来 Epic 5 接入真实 closure 后，如果新增了"读 Keychain / 读 SessionRepository" 等副作用，**必须**在 bind() 路径里对齐

### Dev Note #2：Epic 5 真实 wire 时的注入路径选项（前瞻性 — 本 story 不实装）

当 Story 5.2 (GuestLoginUseCase) + Story 5.5 (LoadHomeUseCase) 实装后，RootView 需要把 `container.makeGuestLoginUseCase()` 等 wire 进 `AppLaunchStateMachine`。备选两条路径：

**路径 A：`@StateObject + bind()` 模式**（参考 HomeViewModel）
```swift
@StateObject private var launchStateMachine = AppLaunchStateMachine()
.task {
    launchStateMachine.bind(
        bootstrapStep1: { try await container.makeGuestLoginUseCase().execute() },
        bootstrapStep2: { try await container.makeLoadHomeUseCase().execute() }
    )
    await launchStateMachine.bootstrap()
}
```

**路径 B：`@State Optional + .onAppear` 模式**（参考 resetIdentityViewModel）
```swift
@State private var launchStateMachine: AppLaunchStateMachine?
.onAppear {
    if launchStateMachine == nil {
        launchStateMachine = AppLaunchStateMachine(
            bootstrapStep1: { try await container.makeGuestLoginUseCase().execute() },
            bootstrapStep2: { try await container.makeLoadHomeUseCase().execute() }
        )
    }
}
```

**推荐路径**：A（bind 模式），理由：
- LaunchingView 从 App 一启动就要可见 — 不能容忍 nil 状态（`@State Optional` 模式 onAppear 前 launchStateMachine 是 nil，view 渲染逻辑要写 `if let ... else { fallback }` 多一层）
- bind 模式让 state machine 与 RootView 同生命周期 + state 变化触发 SwiftUI 重新渲染（@StateObject 订阅）
- 与 `HomeViewModel.bind(pingUseCase:)` 同模式，dev 心智成本低

但**本 story 不实装路径 A 的 bind 接口**：YAGNI；Epic 5 落地时一并加。本 story 只做"占位 closure 走默认参数"路径，让 LaunchingView 骨架可独立验证。

### Dev Note #3：跨 .task 边界防重入（hasBootstrapped flag）

参考 lesson `2026-04-26-swiftui-task-modifier-reentrancy.md`：SwiftUI `.task` 在 view 重新出现时（如 `.fullScreenCover` 关闭后回到 RootView）会**重启** task。

`AppLaunchStateMachine.bootstrap()` 必须用 `hasBootstrapped: Bool` flag 跨 task lifecycle 防重入：
- 仅 `Task` 引用做并发短路无效（每次 task 重启 Task 已是 nil）
- `hasBootstrapped` 失败也置 true：避免 server 不可达时反复重试；用户重试走 `retry()` 显式入口

但本 story 与 HomeViewModel 有一个**重要差别**：`AppLaunchStateMachine.bootstrap()` 在初次跑完后切到 `.ready`，此时 RootView body switch 路由到 HomeView 子树 —— RootView 整个外层 view 不会被 fullScreenCover 覆盖（HomeView 才会），所以本 story bootstrap 的 `.task` 不会因为 sheet 开关而重启。但仍**强制实装** `hasBootstrapped` 防御 — 防御性编程，且为 Epic 5 真实接入做铺垫。

### Dev Note #4：Equatable 合成 + 状态比较 + animation value

`AppLaunchState: Equatable` 让 SwiftUI 的 `.animation(_:value:)` 能正确判断"state 是否变化"。Swift 自动合成 enum Equatable 的 case 比较 + 关联值比较；本 story 三 case：
- `.launching == .launching` → `true`
- `.ready == .ready` → `true`
- `.needsAuth(message: a) == .needsAuth(message: b)` → `a == b`

**验证 + 测试**：在 `AppLaunchStateMachineTests` 通过 `XCTAssertEqual` 直接断言 state 即可（Equatable 合成正确性靠 Swift 编译器保证，无须单独测试）。

### Dev Note #5：UI 测试 timing 与 minimumDuration 的关系

epics.md AC 钦定 `LaunchingView 至少显示 0.3 秒`。UI 测试中 `app.launch()` 本身有 1-2 秒 cold start 开销 — 包含 SwiftUI 进程启动 + `WindowGroup { RootView() }` 渲染 + `@StateObject` 初始化 + 第一帧渲染。这意味着：
- `app.launch()` 返回时（即 XCUITest 认为 app launched），LaunchingView 通常**已经**渲染了几百毫秒
- 0.3 秒 minimumDuration 在大部分机器上"已经过完"了
- `testLaunchingViewVisibleBeforeHomeView` 中对 `launchingText` 的 `waitForExistence(timeout: 0.5)` 是**机会性断言**：能命中说明 LaunchingView 在 0.5 秒内仍可见；命不中也不 fail（`_ = launchingText.waitForExistence(...)`）

**核心断言**是后续 `home_userInfo` 5 秒内可见 — 验证"主界面渲染前不出现空白屏"靠的是"5 秒内 home_userInfo 一定能定位"，间接证明状态机正常切到 .ready。

如未来发现 fast machine 上 launchingText 反复命不中，可考虑：
- 把 minimumDuration 临时调大（仅 UI test 走 environment override） — 本 story **不**做（YAGNI）
- 改用 mock launch closure 让 step1 / step2 故意 sleep 几秒 — 同样 YAGNI

### Dev Note #6：RetryView API 复用 + onRetry 闭包不丢失

Story 2.6 落地的 `RetryView` 是 SwiftUI View，签名约定：`RetryView(message: String, onRetry: () -> Void)`（确切签名 + 测试参考 `iphone/PetApp/Shared/ErrorHandling/` 既有代码）。

**复用契约**：
- `message` 来自 `state` 关联值 `.needsAuth(message:)` 的 String
- `onRetry: { Task { await launchStateMachine.retry() } }` —— 闭包内启 Task 包 async retry()

**与 ErrorPresenter 的差别**（不要混淆）：
- `ErrorPresenter.present(_:onRetry:)` 是**全局错误队列** — 适合"业务请求失败 → 弹 RetryView 在主界面 overlay"
- 本 story 的 `RetryView` 是**整页内容** — RootView body 在 `.needsAuth` 状态下整个渲染 RetryView，**不**走 ErrorPresenter 队列

理由：lesson `2026-04-26-error-presenter-queue-onretry-loss.md` 教训是"队列项必须连带 callback 入队"；本 story 不入 ErrorPresenter 队列，直接 SwiftUI 实例化 RetryView，避开了那个 queue 复杂度。

### Dev Note #7：File List 全集（dev 实装时按此核对）

**新建**（5 个）：
1. `iphone/PetApp/App/AppLaunchState.swift`
2. `iphone/PetApp/App/AppLaunchStateMachine.swift`
3. `iphone/PetApp/Features/Launching/Views/LaunchingView.swift`
4. `iphone/PetAppTests/App/AppLaunchStateMachineTests.swift`
5. `iphone/PetAppTests/Features/Launching/LaunchingViewTests.swift`

**修改**（4 个）：
1. `iphone/PetApp/App/RootView.swift`（@StateObject 追加 + body switch 路由 + .task 增加 + .animation modifier）
2. `iphone/PetApp/App/PetAppApp.swift`（仅注释更新）
3. `iphone/PetApp/Shared/Constants/AccessibilityID.swift`（追加 `Launching` enum）
4. `iphone/PetAppUITests/HomeUITests.swift`（追加 1 个 case）

**不动**（明确白名单 — review 时 grep 确认）：
- `iphone/scripts/build.sh` / `iphone/scripts/install-hooks.sh` / `iphone/scripts/git-hooks/pre-commit`
- `iphone/project.yml`
- `iphone/.gitignore` / 仓库根 `.gitignore`
- `iphone/PetApp/Resources/Info.plist`
- `iphone/PetApp/Resources/Assets.xcassets/`
- `iphone/PetApp/Core/` 任何文件
- `iphone/PetApp/Features/Home/` 任何文件
- `iphone/PetApp/Features/DevTools/` 任何文件
- `iphone/PetApp/Shared/ErrorHandling/` 任何文件
- `iphone/PetApp/App/AppContainer.swift` / `AppCoordinator.swift`
- `iphone/PetAppTests/Helpers/` 任何文件
- `iphone/PetAppTests/Core/` 任何文件
- `iphone/PetAppTests/App/AppContainerTests.swift` / `AppCoordinatorTests.swift` / `RootViewWireTests.swift` / `SheetTypeTests.swift`
- `iphone/PetAppTests/Features/Home/` 任何文件
- `iphone/PetAppTests/Features/DevTools/` 任何文件
- `iphone/PetAppUITests/NavigationUITests.swift`
- `ios/` 任何文件（CLAUDE.md "不动 ios/" 红线）
- `server/` 任何文件
- `docs/` 任何文件（除 review 阶段产生的 `docs/lessons/<date>-*.md`）
- `CLAUDE.md`

### Project Structure Notes

- 新增 `iphone/PetApp/Features/Launching/` 目录是 Feature 层第三个子目录（前两个是 `Home/` 和 `DevTools/`），符合 iOS 架构 §4 钦定结构（`Features/<feature>/{Views,ViewModels,UseCases,Repositories,Models}`）
- `Launching/` 内**仅**有 `Views/LaunchingView.swift` —— 本 story 没有 ViewModel / UseCase / Repository 需求（state machine 在 `App/` 层而非 `Features/Launching/`），符合"Feature 内只放该 feature 的视图层；跨 feature 的 App-level 逻辑放 `App/` 层" 的分层原则
- `AppLaunchStateMachine.swift` 放 `App/` 而非 `Features/Launching/`：理由 ① 它跨多个 feature（决定 RootView 路由到 HomeView / RetryView 哪一个，不是 launching feature 内部状态），② 与 `AppCoordinator.swift` 同一层级（都是 App-level 路由 / 状态控制器）
- 测试目录 `iphone/PetAppTests/Features/Launching/` 是新增（与 `Features/Home/` / `Features/DevTools/` 同级），符合"测试目录镜像 production 目录" 原则

### References

- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#7.1 应用启动状态] AppLaunchState 三态钦定（launching / needsAuth / ready）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#12.1 App 启动链路] App Launch → /auth/guest-login → /home → render HomeView 钦定（Epic 5 接入路径）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#4 项目目录建议] PetApp/{App,Core,Features,...} 钦定
- [Source: \_bmad-output/planning-artifacts/epics.md#Story 2.9] 完整 AC + epics 钦定字面要求（"正在唤醒小猫…"文字 / SF Symbol cat.fill / 0.3 秒最小显示 / 200ms 淡入淡出）
- [Source: \_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.1] XCTest only 锁定 — 本 story 不引入 SnapshotTesting / ViewInspector
- [Source: \_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.2] async/await 主流测试方案
- [Source: \_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.3] iphone/ 目录方案 + ios/ 不动
- [Source: docs/lessons/2026-04-25-swift-explicit-import-combine.md] ObservableObject / @Published 必须显式 `import Combine`
- [Source: docs/lessons/2026-04-26-swiftui-task-modifier-reentrancy.md] SwiftUI .task 在 view 重新出现时会重启；一次性 side effect 必须用 hasFetched / hasBootstrapped flag 跨 task 边界短路
- [Source: docs/lessons/2026-04-26-stateobject-init-vs-bind-injection.md] @StateObject 不能 lazy 初始化；副作用初始化必须在所有入口对齐覆盖
- [Source: docs/lessons/2026-04-26-stateobject-debug-instance-aliasing.md] @StateObject init 阶段构造 standalone container 是别名陷阱（本 story 占位 closure 在默认参数 wire，不踩此坑）
- [Source: docs/lessons/2026-04-26-fullscreencover-isolated-environment.md] errorPresentationHost 必须在 sheet 子树重复 attach（本 story `.fullScreenCover` 挪到 `.ready` case 内时 sheetContent 内 attach 仍生效）
- [Source: docs/lessons/2026-04-26-error-presenter-queue-onretry-loss.md] queue 入队时必须连带 onRetry 一起入；本 story 直接 SwiftUI 实例化 RetryView，避开队列
- [Source: docs/lessons/2026-04-26-swiftui-a11y-contain-with-label.md] `.contain` + `.accessibilityLabel` 并存才不丢父 summary（本 story LaunchingView 容器用 `.contain` + identifier，不需要 label）
- [Source: docs/lessons/2026-04-26-objectwillchange-no-initial-emit.md] 同主题家族 — @Published 初值不会 emit objectWillChange，本 story state 默认 .launching 不需要测试 emit

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) — 2026-04-26 dev-story 一次过完整实装。

### Debug Log References

- `bash iphone/scripts/build.sh --test`：build vet + 119 个单元测试全 PASS（含新增 7 + 3 个 case）
- `bash iphone/scripts/build.sh --uitest`：7 个 UI 测试全 PASS（含新增 `testLaunchingViewVisibleBeforeHomeView`）
- `git status`：仅触及 `iphone/PetApp/App/`、`iphone/PetApp/Features/Launching/`、`iphone/PetApp/Shared/Constants/`、`iphone/PetAppTests/App/`、`iphone/PetAppTests/Features/Launching/`、`iphone/PetAppUITests/`、`iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen 自动重生成），未碰 `ios/` / `server/` / `iphone/scripts/` / `iphone/project.yml`

### Completion Notes List

- **AC1–AC9 全部满足**：3 态 enum + 状态机（含 minimumDuration / hasBootstrapped / retry / messageFor）+ LaunchingView SwiftUI 骨架 + RootView Group switch 路由 + AccessibilityID.Launching + PetAppApp 注释更新 + ≥ 7 单测 + ≥ 3 LaunchingView 契约测试 + 1 UI 启动测试
- **AC10**：build vet + 119/119 单元测试 + 7/7 UI 测试 全绿
- **Lesson 遵守清单**：
  - L1 (Combine 显式 import)：`AppLaunchStateMachine.swift` + `AsyncTestHelpers.swift` 都已显式 `import Combine`；测试文件无须 import Combine（不直接用 publisher API）
  - L3 (`.contain` 不丢父 label)：LaunchingView 容器只用 `.accessibilityElement(children: .contain)` + identifier；本 story 容器无 summary label 需求（与 HomeView userInfoBar 不同），故未叠加 `.accessibilityLabel`，符合 lesson 精神（**保留** label 仅当需要 summary）
  - L7 (bind 副作用初始化对齐)：本 story 走默认 closure init 路径，**不**走 bind 模式（Dev Note #1 论证）；Epic 5 真实接入时按 Dev Note #2 路径 A 落地 bind+副作用对齐
  - L8 (`.task` 重启 → ViewModel hasBootstrapped)：`AppLaunchStateMachine.bootstrap()` 用 `hasBootstrapped: Bool` flag 跨 .task 边界短路（含失败也置 true，避免反复重试）
  - L9 (fullScreenCover 隔离 / errorPresenter 重复 attach)：`.fullScreenCover` 挪入 `.ready` case；`errorPresentationHost(presenter:)` 仍挂在最外层 Group，sheet 子树内 `sheetContent(for:)` 已重复 attach（既有不动）
  - L13 (@StateObject 不能在 init 阶段构造 standalone 同类型 bootstrap)：本 story `AppLaunchStateMachine` 走默认 closure（无副作用），不踩此坑；Epic 5 真实接入时按 Dev Note #2 路径 A bind 模式（参考 HomeViewModel）或路径 B（@State Optional + onAppear，参考 resetIdentityViewModel）
- **测试设计**：`AppLaunchStateMachineTests` 7 case 覆盖初值 / 两 happy / 两 edge / minimumDuration / 短路 / retry；用 `actor CallCounter` + `actor ShouldFailHolder` 避免 Swift 6 strict concurrency 捕获 var 的警告（lesson 同主题家族）
- **UI 测试**：`testLaunchingViewVisibleBeforeHomeView` 用机会性断言（短 timeout 容忍 fast machine 错过 0.3s 窗口）+ home_userInfo 5s 充分 timeout 兜底；既有 6 个 UI case timeout 保持 5s 已充分（无需改）

### File List

**新建**（5 个）：
1. `iphone/PetApp/App/AppLaunchState.swift`
2. `iphone/PetApp/App/AppLaunchStateMachine.swift`
3. `iphone/PetApp/Features/Launching/Views/LaunchingView.swift`
4. `iphone/PetAppTests/App/AppLaunchStateMachineTests.swift`
5. `iphone/PetAppTests/Features/Launching/LaunchingViewTests.swift`

**修改**（4 个）：
1. `iphone/PetApp/App/RootView.swift`（@StateObject 追加 + body Group switch 路由 + .task 增加 + .animation modifier）
2. `iphone/PetApp/App/PetAppApp.swift`（仅注释更新）
3. `iphone/PetApp/Shared/Constants/AccessibilityID.swift`（追加 `Launching` enum）
4. `iphone/PetAppUITests/HomeUITests.swift`（追加 `testLaunchingViewVisibleBeforeHomeView`）

**xcodegen 自动重生成**（每次 build 都会刷新；不算手工改动）：
- `iphone/PetApp.xcodeproj/project.pbxproj`

### Change Log

- 2026-04-26 — Story 2.9 LaunchingView 设计 dev 实装完成；ready-for-dev → review。Build + 全单测 + UI 测试全绿。
