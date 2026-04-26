# Story 2.8: Dev 重置 Keychain 按钮（build flag gated）

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 开发 / demo 者,
I want iPhone App 主界面右上角有一个 **dev-only** 的"重置身份"按钮，按下后清空当前 device 上 App 的 Keychain 全部数据并弹 alert 提示重启 App，
so that demo / 开发期不必每次卸载重装就能模拟"全新安装"场景；同时为后续 Epic 5 真实 KeychainStore 上线时预先把"清 Keychain"工具按钮 + UseCase 协议接缝就位，避免 Story 5.1 落地时再回头改主界面。

## 故事定位（Epic 2 第八条实装 story；iPhone 端最小 dev 工具栈）

这是 Epic 2 内**第八条**实装 story，**直接前置**全部 done：

- **Story 2.1 (`done`)** 输出 ADR-0002 锁定 4 类决策（XCTest only / async/await / `iphone/` 目录方案 / `bash iphone/scripts/build.sh --test` 入口）—— 本 story 的 dev 测试 case 严格用 XCTest only，不引第三方
- **Story 2.2 (`done`)** 落地 `iphone/` 顶层目录 + `iphone/PetApp/{App,Core,Features,Shared,Resources}` + `iphone/project.yml` + 主界面 6 大占位区块（含**右下角**版本号 footer，AccessibilityID.Home.versionLabel）；**右上角空闲**，本 story 的"重置身份"按钮挂在 `userInfoBar`（顶部行）右侧（与已有用户昵称 + 头像位同行，靠右对齐）
- **Story 2.3 (`done`)** 落地 `AppCoordinator` + 三个 CTA 按钮（进入房间 / 仓库 / 合成）的 sheet 路由 —— 本 story **不**走 sheet，alert 走 SwiftUI 原生 `.alert(isPresented:)` modifier
- **Story 2.4 (`done`)** 落地 `APIClient` + `Endpoint` + `APIError` —— 本 story **完全不接** APIClient（"清 Keychain"是纯本地动作）
- **Story 2.5 (`done`)** 落地 `AppContainer.swift`（手写 DI 容器）；line 12 注释"按需追加 KeychainStore / SessionRepository / WebSocketClient 等"—— 本 story 兑现"按需追加"的第一步：在 `AppContainer` 加一个 `keychainStore` 字段（占位实装即可），并加 `makeResetKeychainUseCase()` 工厂方法
- **Story 2.6 (`done`)** 落地 `ErrorPresenter` / `Toast` / `AlertOverlay` / `RetryView` —— 本 story 的 alert **不**走 `ErrorPresenter`（那是给 `APIError` 业务错误用的），而是 dev 工具的"操作完成提示"，用 SwiftUI 原生 `.alert` 即可（与 Story 2.6 的 AlertOverlay 是两条并行 UI 路径）
- **Story 2.7 (`done`)** 落地 `MockBase` / `AsyncTestHelpers` / `iphone/scripts/build.sh` —— 本 story 写 `MockKeychainStore` 时**继承 `MockBase`**（templated 范式，AC2/AC3 强制）

**本 story 的核心动作**（顺序无关，可分批落地）：

1. 新建 `iphone/PetApp/Core/Storage/KeychainStore.swift`：定义 `KeychainStoreProtocol` + 占位实装 `InMemoryKeychainStore`（内部用 `[String: String]` 字典 + `NSLock`）。**真实 Security framework / kSecClassGenericPassword 调用属于 Story 5.1 scope，本 story 不写**。`InMemoryKeychainStore.removeAll()` 是占位但**功能正确**：清空内部字典即"清 Keychain"语义。本 story 命名以 `InMemory*` 前缀表明非生产，Story 5.1 上线时改成 `KeychainServicesStore`（用 `Security.framework`）替换占位实装即可，**协议不变**
2. 新建 `iphone/PetApp/Features/DevTools/UseCases/ResetKeychainUseCase.swift`：定义 `ResetKeychainUseCaseProtocol` + `DefaultResetKeychainUseCase` 实装，单方法 `func execute() async throws`，内部调 `keychainStore.removeAll()`；任何 throw 让上层（ViewModel）转成 alert "重置失败"
3. 新建 `iphone/PetApp/Features/DevTools/Views/ResetIdentityButton.swift`：SwiftUI View，仅在 `#if DEBUG` 下渲染按钮（SF Symbol `arrow.counterclockwise.circle` + 无文字 / 短文 "重置身份"）；点击触发 `ResetIdentityViewModel.tap()` —— 见动作 4。**Release build（`#else` 分支）渲染 `EmptyView()`** —— 视图树中**完全不存在**该按钮（不是 `.opacity(0)` 或 `.hidden()`）
4. 新建 `iphone/PetApp/Features/DevTools/ViewModels/ResetIdentityViewModel.swift`：`@MainActor` ObservableObject；`@Published var alertContent: ResetIdentityAlertContent?`；`func tap() async` 调 `useCase.execute()`，成功设 `alertContent = .success`，失败设 `alertContent = .failure(message:)`。`ResetIdentityAlertContent` enum 含 `.success` / `.failure(message:)` 两 case
5. **修改** `iphone/PetApp/Features/Home/Views/HomeView.swift` 的 `userInfoBar` 计算属性：在原 `Spacer()` 之后追加 `ResetIdentityButton(viewModel: ...)` —— 用 `#if DEBUG ... #endif` 包裹 button 的引用本身（让 Release build 视图树**完全没有**该按钮，不只是 EmptyView 占位）。`ResetIdentityViewModel` 实例从 `HomeView` 接收为新增 `@StateObject` 字段，由 `RootView` 的 `container.makeResetIdentityViewModel()` 注入（与 Story 2.5 `homeViewModel` 同模式）
6. **修改** `iphone/PetApp/App/AppContainer.swift`：追加 `keychainStore: KeychainStoreProtocol` 字段（默认 `InMemoryKeychainStore`）+ `makeResetKeychainUseCase() -> ResetKeychainUseCaseProtocol` 工厂 + `makeResetIdentityViewModel() -> ResetIdentityViewModel` 工厂。新增 init 重载允许测试注入 mock keychainStore
7. **修改** `iphone/PetApp/App/RootView.swift`：仅在 `#if DEBUG` 下加 `@StateObject private var resetIdentityViewModel: ResetIdentityViewModel`，并把它传给 `HomeView`。Release build 该字段不存在
8. **修改** `iphone/PetApp/Shared/Constants/AccessibilityID.swift`：在 `Home` enum 内追加 `static let btnResetIdentity = "home_btnResetIdentity"`（与 `btnRoom` / `btnInventory` / `btnCompose` 同风格）；新增 alert 用 `static let resetIdentityAlert = "home_resetIdentityAlert"`
9. 新建 `iphone/PetAppTests/Features/DevTools/`（新目录）下三个测试文件：
   - `ResetKeychainUseCaseTests.swift`（≥ 2 case：happy + error）
   - `ResetIdentityViewModelTests.swift`（≥ 3 case：happy / error / alert content）
   - `InMemoryKeychainStoreTests.swift`（≥ 2 case：set+get、removeAll 清空）
10. 新建 `iphone/PetAppTests/Features/DevTools/MockKeychainStore.swift`：`MockBase` 子类，提供 `KeychainStoreProtocol` 的 mock；`removeAllStubError: Error?` 字段允许 stub 抛错
11. **修改** `iphone/PetAppUITests/HomeUITests.swift`：追加 `testResetIdentityButtonVisibleAndAlertOnTap()` 测试方法，验证按钮可定位 + 点击 + alert 出现（XCUITest 默认在 Debug configuration 跑，`#if DEBUG` 分支生效）
12. **不动**：Story 2.4 / 2.5 / 2.6 / 2.7 任何文件（除 `AppContainer.swift` `RootView.swift` `HomeView.swift` `AccessibilityID.swift` 这 4 个明确改动点）

**不涉及**：

- **真实 Security framework / kSecClass* 调用**：留给 Story 5.1（KeychainStore 真实封装）。本 story 用 `InMemoryKeychainStore` 占位实装；Story 5.1 上线时换实装 + 改 `AppContainer` 默认实例为 `KeychainServicesStore`，协议不变 → 本 story 写的所有测试零回归。**理由**：① CLAUDE.md "节点顺序不可乱跳"：节点 1 不实装 auth / token / Keychain；② 占位实装在功能上等价（清字典 = 清 Keychain），dev demo 已可用；③ 提前实装 Security framework 等同抢 Story 5.1 scope，违反 epics.md §Story 5.1 边界
- **Dev login override / dev API server switcher / dev grant-steps 按钮**：分别属于 Story 4.6（auth）/ 未来工程化决策 / Story 8.5（步数同步）。本 story **仅**做"重置 Keychain"一项 dev 工具
- **Build configuration / xcconfig / -tags 切换**：iOS 端 dev/release 切换用 Swift `#if DEBUG`（Xcode 默认 Debug configuration 时 `DEBUG` 宏开），不引入 `-tags devtools` 等 Go 风格机制（与 server 端 Story 1.6 的 build tag 双闸门**显式不对齐**：iOS 平台没有 build tag 概念，唯一相对应的是 `SWIFT_ACTIVE_COMPILATION_CONDITIONS` 编译器条件，Xcode 默认 Debug 配置已含 `DEBUG`）
- **`BUILD_DEV` 环境变量**：iOS 同样不适用 —— Server Story 1.6 设计中环境变量是给运维热切的 Linux/macOS 进程用的；iOS App 在用户设备上跑，没有"运维热切"语义。**唯一**真实开关是编译期 `#if DEBUG`
- **`UserDefaults` / 登录态 cache 的清理**：本 story scope 严格限定"清 Keychain"。如果未来发现 demo 时还有 `UserDefaults` 残留干扰（例如 Story 5.5 LoadHomeUseCase 缓存了首页快照），可在那时扩展本 button 的 use case 范围（届时改名 `ResetIdentityUseCase` 即可，协议名已经预留）。**当前阶段不做**
- **修改 Story 1.6 server 端 dev-tools 框架**：跨端独立。server 端 dev-tools 是 HTTP 路由组（`/dev/ping-dev` 等），iOS 端 dev-tools 是 UI 按钮 + 本地动作；两者**无技术耦合**，仅在"dev/release gate"概念上对标
- **Confirm dialog 二次确认**：epics.md AC 没要求；按钮直接触发 reset，alert 在动作**之后**显示。如果未来有"误触代价大"的反馈，可在那时加二次确认（YAGNI）
- **跨端集成测试 / E2E 验证**：归 Epic 3 Story 3.1 / Story 3.2 范围（节点 1 demo 验收）；本 story 仅在 `iphone/` 内闭环（unit + UI test）

**范围红线**：

- **不动 `ios/`**：CLAUDE.md "Repo Separation" + ADR-0002 §3.3 既有约束。最终 `git status` 须确认 `ios/` 下零改动
- **不动 `server/`**：本 story 是 iPhone 端 dev 工具落地，与 server 完全无关（不发任何 API 请求）
- **不动 `iphone/scripts/` / `iphone/project.yml` / `iphone/.gitignore` / `.gitignore`**：所有新文件都靠 Story 2.2 既有 `sources: - PetApp` / `sources: - PetAppTests` glob 自动纳入；**0 yml 改动**（与 Story 2.7 同模式）
- **不动 `iphone/scripts/install-hooks.sh` / `iphone/scripts/git-hooks/pre-commit`**：占位 hook 保持不动
- **不动 Story 2.4 / 2.5 / 2.6 / 2.7 既有任何 production / 测试文件**，除以下 4 个明确改动点：
  - `iphone/PetApp/App/AppContainer.swift`（追加字段 + 工厂方法；不破坏 Story 2.5 既有 init / API）
  - `iphone/PetApp/App/RootView.swift`（`#if DEBUG` 内加新 `@StateObject`；Release build 行为不变）
  - `iphone/PetApp/Features/Home/Views/HomeView.swift`（`userInfoBar` 内 `#if DEBUG` 内追加 button；Release build 行为不变）
  - `iphone/PetApp/Shared/Constants/AccessibilityID.swift`（追加 enum case；不删不改既有）
- **不动 Story 2.7 落地的 `MockBase` / `AsyncTestHelpers` / `SampleTypes` / `SampleViewModelTests`**：本 story 的 `MockKeychainStore` 是 MockBase 的**子类**，新写文件，不改 MockBase 本体
- **不动 `CLAUDE.md` / `docs/`**：仅修改时新建 `docs/lessons/<date>-*.md`（仅在 review 阶段产生）
- **不引入第三方依赖**：Security framework 是 Apple 系统库（Story 5.1 才用）；本 story 仅用 Foundation + Combine + SwiftUI（stdlib + Apple 系统库）
- **不实装 dev/release build configuration 切换的工程配置**：Xcode 默认 Debug configuration 已含 `DEBUG=1` 宏；project.yml 不改
- **`ResetIdentityButton` 必须在 Release build 视图树中物理不存在**：`#if DEBUG ... #else <空> #endif` 包裹 button 引用本身，**不**用 `.opacity(0)` / `.hidden()` / `.disabled(true)`（这些都是"还在视图树中但不显示"，不满足 epics.md AC "完全不渲染（不只是 hidden，是不存在于视图树）"硬约束）
- **`MockKeychainStore` / `InMemoryKeychainStore` 命名不冲突**：grep `iphone/` 下无 `MockKeychain*` / `InMemoryKeychain*` 现有命名；如未来 Story 5.1 用 `KeychainServicesStore` 命名，本 story 占位实装是 `InMemoryKeychainStore`，命名不撞
- **测试用 MockBase 风格但**：`MockKeychainStore` 不能强行复用 `MockURLSession` / `MockAPIClient` —— 那两个是 networking-specific（Story 2.4 / 2.5）；KeychainStore 是 Storage 层，**新写**符合 ADR-0002 §3.1 "手写 mock" 精神

## Acceptance Criteria

**AC1 — `KeychainStoreProtocol` + `InMemoryKeychainStore` 占位实装**

新建 `iphone/PetApp/Core/Storage/KeychainStore.swift`，必须提供：

```swift
// KeychainStore.swift
// Story 2.8: 占位实装 + 协议接缝。Story 5.1 真实 Security.framework 实装时**仅替换实装**，
// 协议不变；本 story 的所有 UseCase / ViewModel 测试在 Story 5.1 落地时零回归。
//
// 设计：协议提供 set / get / remove / removeAll 四方法；本 story 仅 removeAll 真实可用
// （足以驱动 dev "重置身份" 按钮）；其它三方法占位实装亦正确，但生产语义未验证（Story 5.1 验证）。

import Foundation

public protocol KeychainStoreProtocol: Sendable {
    /// 保存 key-value（覆盖已存在）。throws：底层 Keychain access 错误。
    func set(_ value: String, forKey key: String) throws
    /// 读取 value；不存在返回 nil。throws：底层 Keychain access 错误（**不**含 itemNotFound）。
    func get(forKey key: String) throws -> String?
    /// 删除单个 key（不存在不报错）。throws：底层 Keychain access 错误。
    func remove(forKey key: String) throws
    /// 删除该 App **全部** Keychain 项（"重置身份" 按钮触发）。throws：底层 Keychain access 错误。
    func removeAll() throws
}

/// 占位实装：内部 `[String: String]` 字典 + NSLock；功能等价但**不持久化**（App 重启丢失）。
/// 不是生产代码：① 命名 `InMemory*` 前缀；② 文件头硬注明"Story 5.1 替换为 KeychainServicesStore"；
/// ③ 不接触 Security.framework，不调 kSecClass*。
///
/// 为什么本 story 用占位而非真 Keychain：
/// - Story 5.1（Epic 5）才是 KeychainStore 实装 story，节点 2 才到；本 story 是节点 1 dev 工具
/// - 占位实装 `removeAll()` 清空字典 = "清 Keychain" 语义等价（dev demo 视角）
/// - 协议先稳，未来替实装零回归
public final class InMemoryKeychainStore: KeychainStoreProtocol, @unchecked Sendable {
    private var storage: [String: String] = [:]
    private let lock = NSLock()

    public init() {}

    public func set(_ value: String, forKey key: String) throws {
        lock.lock(); defer { lock.unlock() }
        storage[key] = value
    }

    public func get(forKey key: String) throws -> String? {
        lock.lock(); defer { lock.unlock() }
        return storage[key]
    }

    public func remove(forKey key: String) throws {
        lock.lock(); defer { lock.unlock() }
        storage.removeValue(forKey: key)
    }

    public func removeAll() throws {
        lock.lock(); defer { lock.unlock() }
        storage.removeAll()
    }
}
```

**具体行为要求**：
- 协议四方法**全部** `throws`：本 story 占位实装永不抛；Story 5.1 真实实装可能抛 `OSStatus` 包装错误（如 `errSecAuthFailed`）
- `Sendable` / `@unchecked Sendable` 声明对齐 Swift 6 严格 concurrency
- **不**用 `actor` —— 与 Story 2.4 `MockURLSession` / Story 2.5 `MockAPIClient` 模式一致（手写 lock + `@unchecked Sendable`），actor 在 NSLock 模式下没收益还多一层 await
- 文件头注释**必须**写明"占位实装；Story 5.1 替换为 KeychainServicesStore"（review/distill 时供未来 Claude 不踩"以为这是生产 Keychain"的坑）

**AC2 — `ResetKeychainUseCaseProtocol` + `DefaultResetKeychainUseCase` 实装**

新建 `iphone/PetApp/Features/DevTools/UseCases/ResetKeychainUseCase.swift`：

```swift
// ResetKeychainUseCase.swift
// Story 2.8: dev "重置身份" 按钮的 UseCase 层。
// 单一职责：调 keychainStore.removeAll()；任何 throw 透传给 ViewModel 转成 alert。

import Foundation

public protocol ResetKeychainUseCaseProtocol: Sendable {
    func execute() async throws
}

public struct DefaultResetKeychainUseCase: ResetKeychainUseCaseProtocol {
    private let keychainStore: KeychainStoreProtocol

    public init(keychainStore: KeychainStoreProtocol) {
        self.keychainStore = keychainStore
    }

    public func execute() async throws {
        try keychainStore.removeAll()
    }
}
```

**具体行为要求**：
- `struct` 而非 `class`：UseCase 是 value type（与 Story 2.5 `DefaultPingUseCase` 同风格）；构造廉价
- `async throws`：与 ADR-0002 §3.2 异步测试方案对齐；当前实装内部不真异步，但 `async` 标注让 Story 5.1 真实 Keychain 调用（在 KeychainServicesStore 中）可改 `async` 而**不**破协议
- `Sendable`：Swift 6 strict concurrency 默认要求
- 错误透传：占位实装永不 throw；Story 5.1 真实 Keychain 抛错时 ViewModel 走 `.failure` 路径

**AC3 — `ResetIdentityViewModel` 实装**

新建 `iphone/PetApp/Features/DevTools/ViewModels/ResetIdentityViewModel.swift`：

```swift
// ResetIdentityViewModel.swift
// Story 2.8: dev "重置身份" 按钮的 ViewModel。

import Foundation
import Combine

/// alert 内容枚举：成功 / 失败两态；nil = 不显示 alert。
public enum ResetIdentityAlertContent: Equatable {
    case success
    case failure(message: String)
}

@MainActor
public final class ResetIdentityViewModel: ObservableObject {
    @Published public var alertContent: ResetIdentityAlertContent?

    private let useCase: ResetKeychainUseCaseProtocol

    public init(useCase: ResetKeychainUseCaseProtocol) {
        self.useCase = useCase
    }

    /// 用户点击按钮时调用：触发 useCase；任一结果设 alertContent 非 nil 触发 SwiftUI alert 弹出。
    /// 成功文案："已重置，请杀进程后重新启动 App 模拟首次安装"
    /// 失败文案："重置失败：<error description>"
    public func tap() async {
        do {
            try await useCase.execute()
            alertContent = .success
        } catch {
            alertContent = .failure(message: "重置失败：\(error.localizedDescription)")
        }
    }

    /// 由 SwiftUI alert 的 dismiss 回调调用：清空 alertContent，避免下次 tap 时旧 alert 残留。
    public func alertDismissed() {
        alertContent = nil
    }
}
```

**具体行为要求**：
- `@MainActor`：与 Story 2.5 `HomeViewModel` 同风格；@Published 字段写入需在 main actor
- `tap()` 是 `async` 方法（不 throws）：错误内部 catch 转 alertContent，**不**让 SwiftUI Button action 直接看到 `try await`
- `alertDismissed()`：SwiftUI `.alert(isPresented:)` 在用户点 OK 后自动复位 binding，但本 ViewModel 用 `Optional<ResetIdentityAlertContent>` 配合 SwiftUI `.alert(item:)`（iOS 15+）；user dismiss 后调用此方法清 state
- 显式 `import Combine`：lesson `2026-04-25-swift-explicit-import-combine.md` —— ObservableObject / @Published 不能依赖 SwiftUI transitive import（Swift 6 严格模式失败）

**AC4 — `ResetIdentityButton` SwiftUI View + Release build 视图树物理不存在**

新建 `iphone/PetApp/Features/DevTools/Views/ResetIdentityButton.swift`：

```swift
// ResetIdentityButton.swift
// Story 2.8: dev "重置身份" 按钮（SF Symbol arrow.counterclockwise.circle）。
// **仅在 Debug build 渲染**；Release build 视图树物理不存在（#if DEBUG 包裹整个 type 定义）。

#if DEBUG

import SwiftUI

public struct ResetIdentityButton: View {
    @ObservedObject public var viewModel: ResetIdentityViewModel

    public init(viewModel: ResetIdentityViewModel) {
        self.viewModel = viewModel
    }

    public var body: some View {
        Button {
            Task { await viewModel.tap() }
        } label: {
            Image(systemName: "arrow.counterclockwise.circle")
                .font(.title3)
        }
        .accessibilityLabel(Text("重置身份"))
        .accessibilityIdentifier(AccessibilityID.Home.btnResetIdentity)
        .alert(item: $viewModel.alertContent) { content in
            switch content {
            case .success:
                return Alert(
                    title: Text("已重置"),
                    message: Text("请杀进程后重新启动 App 模拟首次安装"),
                    dismissButton: .default(Text("OK")) { viewModel.alertDismissed() }
                )
            case .failure(let message):
                return Alert(
                    title: Text("操作失败"),
                    message: Text(message),
                    dismissButton: .default(Text("OK")) { viewModel.alertDismissed() }
                )
            }
        }
    }
}

extension ResetIdentityAlertContent: Identifiable {
    public var id: String {
        switch self {
        case .success: return "alert_reset_success"
        case .failure: return "alert_reset_failure"
        }
    }
}

#endif
```

**具体行为要求**：
- **整个 type 定义 + extension `#if DEBUG ... #endif` 包裹**：Release build 该 type 完全不存在 —— 编译器看不到，调用方（HomeView）也必须 `#if DEBUG` 包裹引用，否则 Release build 会 "cannot find 'ResetIdentityButton'" 编译失败（这是**有意**的 fail-closed）
- SF Symbol `arrow.counterclockwise.circle`（epics.md AC 字面要求）；`.font(.title3)` 让按钮在 userInfoBar 内尺寸合理
- **AccessibilityIdentifier** `home_btnResetIdentity`：UI 测试定位用
- **AccessibilityLabel** "重置身份"：屏幕阅读器友好（dev 工具但仍要 a11y 兜底）
- `.alert(item:)`：SwiftUI 原生 alert（iOS 15+）；与 Story 2.6 `AlertOverlayView` 是**两条并行 UI 路径**（业务错误走 ErrorPresenter / dev 操作反馈走 .alert(item:)）—— 不复用 AlertOverlay，理由 ① ErrorPresenter 是给 APIError 用的，dev 操作不是 APIError；② SwiftUI 原生 .alert 在 dev 工具场景已足够，引入 ErrorPresenter 反而增加耦合
- `ResetIdentityAlertContent: Identifiable` extension：`.alert(item:)` 要求 item 类型符合 Identifiable
- alert 文案：epics.md AC 字面 "已重置，请杀进程后重新启动 App 模拟首次安装"

**AC5 — `HomeView.userInfoBar` 集成 + Release build 物理不存在**

修改 `iphone/PetApp/Features/Home/Views/HomeView.swift`：

```swift
// 修改前的 userInfoBar（Story 2.2）：
private var userInfoBar: some View {
    HStack(spacing: 8) {
        Text(viewModel.nickname)
        Circle()
            .fill(Color.gray)
            .frame(width: 32, height: 32)
        Spacer()
    }
    .accessibilityElement(children: .ignore)
    .accessibilityLabel(Text(viewModel.nickname))
    .accessibilityIdentifier(AccessibilityID.Home.userInfo)
}

// 修改后（Story 2.8）：
private var userInfoBar: some View {
    HStack(spacing: 8) {
        Text(viewModel.nickname)
        Circle()
            .fill(Color.gray)
            .frame(width: 32, height: 32)
        Spacer()
        #if DEBUG
        if let resetIdentityViewModel = resetIdentityViewModel {
            ResetIdentityButton(viewModel: resetIdentityViewModel)
        }
        #endif
    }
    .accessibilityElement(children: .contain)  // 改：children .ignore → .contain，让按钮 a11y 可定位
    .accessibilityIdentifier(AccessibilityID.Home.userInfo)
}
```

**具体行为要求**：
- HomeView 新增 **optional** `@ObservedObject` 字段 `resetIdentityViewModel: ResetIdentityViewModel?`，由父 View 注入；`init(viewModel:resetIdentityViewModel:)` 默认值 `nil`（保持 Story 2.2 / 2.5 既有 init 不破坏）
- `userInfoBar` 内 `#if DEBUG` 包裹按钮**引用本身**：Release build 视图树没有 `ResetIdentityButton` 节点（`ResetIdentityButton` type 也不存在）
- `accessibilityElement(children: .contain)`：Story 2.2 原值是 `.ignore`（让整个 bar 当成 a11y 整体），改成 `.contain` 让内部按钮的 a11y identifier 可独立定位
- 验证：grep `iphone/PetApp/Features/Home/Views/HomeView.swift` 内 `accessibilityIdentifier(AccessibilityID.Home.userInfo)` 仍存在（不删既有）；新增 `#if DEBUG ... #endif` 块仅出现在 `userInfoBar` 一处

**AC6 — `AppContainer` 注入 KeychainStore + 工厂方法**

修改 `iphone/PetApp/App/AppContainer.swift`：

追加：
```swift
// 在 errorPresenter 字段下方追加：
public let keychainStore: KeychainStoreProtocol

// 在现有 init(apiClient:) 修改为：
public init(
    apiClient: APIClientProtocol,
    keychainStore: KeychainStoreProtocol = InMemoryKeychainStore()
) {
    self.apiClient = apiClient
    self.errorPresenter = ErrorPresenter()
    self.keychainStore = keychainStore
}

// 在 makePingUseCase() 下方追加：
public func makeResetKeychainUseCase() -> ResetKeychainUseCaseProtocol {
    DefaultResetKeychainUseCase(keychainStore: keychainStore)
}

#if DEBUG
public func makeResetIdentityViewModel() -> ResetIdentityViewModel {
    ResetIdentityViewModel(useCase: makeResetKeychainUseCase())
}
#endif
```

**具体行为要求**：
- `keychainStore` 默认值 `InMemoryKeychainStore()`：`convenience init()` 间接复用此默认（已有 convenience init 链路 `convenience init() → init(apiClient:)`）—— 在 `init(apiClient:keychainStore:)` 加默认参数即可，**不**额外加 init 重载
- `makeResetIdentityViewModel()` **必须** `#if DEBUG` 包裹：Release build 该方法不存在；调用方（RootView）也必须 `#if DEBUG` 包裹调用，fail-closed
- 不破坏 Story 2.5 既有 API：`makePingUseCase()` 等不动
- baseURL 解析逻辑 / errorPresenter 等无关字段**不动**

**AC7 — `RootView` Debug-only `@StateObject` 注入**

修改 `iphone/PetApp/App/RootView.swift`：

```swift
struct RootView: View {
    @StateObject private var coordinator = AppCoordinator()
    @StateObject private var container = AppContainer()
    @StateObject private var homeViewModel = HomeViewModel()

    #if DEBUG
    @StateObject private var resetIdentityViewModel: ResetIdentityViewModel
    #endif

    init() {
        #if DEBUG
        // Debug build：用 standalone container 实例预先构造 viewModel；
        // production 路径上 container 是 @StateObject 自管，但 ResetIdentityViewModel
        // 需要在 init 阶段就给 @StateObject 一个值（@StateObject 不能 lazy 初始化）。
        // 避免 lesson 2026-04-26-stateobject-init-vs-bind-injection.md 描述的"init 阶段
        // @StateObject 注入"陷阱：用一个临时 standalone container 喂 ResetIdentityViewModel，
        // 真正用到 keychainStore 时（用户点按钮）走的是 ResetIdentityViewModel 内捕获的
        // useCase 实例 —— 与 RootView 的 container 是否同一个无关（dev 工具场景下，
        // standalone container 与 RootView container 都用同一个默认的 InMemoryKeychainStore() type）。
        let bootstrap = AppContainer()
        _resetIdentityViewModel = StateObject(wrappedValue: bootstrap.makeResetIdentityViewModel())
        #endif
    }

    var body: some View {
        #if DEBUG
        HomeView(viewModel: homeViewModel, resetIdentityViewModel: resetIdentityViewModel)
        #else
        HomeView(viewModel: homeViewModel)
        #endif
            .onAppear {
                wireHomeViewModelClosures()
            }
            .task {
                homeViewModel.bind(pingUseCase: container.makePingUseCase())
                await homeViewModel.start()
            }
            .fullScreenCover(item: $coordinator.presentedSheet) { sheet in
                sheetContent(for: sheet)
            }
            .errorPresentationHost(presenter: container.errorPresenter)
    }
    // wireHomeViewModelClosures / sheetContent(for:) 保持不变
}
```

**具体行为要求**：
- `@StateObject` 不能 lazy 初始化 → 必须在 init 阶段给值 → 用临时 standalone `AppContainer()` 喂（**不**与 `@StateObject container` 同一个实例）
- 这种"两个 container 实例"在**dev 工具的纯本地动作**场景下是 OK 的：`InMemoryKeychainStore()` 默认实例化无副作用；用户点按钮触发的 useCase 走的是 viewModel 内部捕获的 useCase 实例，与 RootView 的 container 无关
- **lesson 2026-04-26-stateobject-init-vs-bind-injection.md 应用**：本 story **不**走 bind() 注入路径（与 HomeViewModel 不同），是因为 ResetIdentityViewModel 不依赖 RootView 的 `container.apiClient` 实例（与 HomeViewModel 用同一 PingUseCase 不同）；future Story 5.1 真 Keychain 上线时如果发现 KeychainStore 必须是单例，再改 bind 路径 —— 本 story 简单实例化够用
- Release build 整个 `#if DEBUG` 块代码不存在：`HomeView(viewModel:)`（不带 `resetIdentityViewModel:` 参数）走旧 init

**AC8 — `AccessibilityID.Home` 追加按钮 + alert identifier**

修改 `iphone/PetApp/Shared/Constants/AccessibilityID.swift`：

```swift
// 在 Home enum 末尾追加（与 btnRoom / btnInventory / btnCompose 同风格）：
public static let btnResetIdentity = "home_btnResetIdentity"
public static let resetIdentityAlert = "home_resetIdentityAlert"  // alert 容器 identifier（reserved；当前不挂在 SwiftUI Alert 上 —— SwiftUI Alert 本身不接 a11y identifier；UI 测试通过 alert 内部文字定位）
```

**具体行为要求**：
- 严格在 `Home` enum 内追加（不新建 enum；命名空间不爆炸）
- 命名风格 `home_btnResetIdentity` 严格对齐 `home_btnRoom` 等
- `resetIdentityAlert` 当前 reserved（SwiftUI Alert 内部 a11y identifier 难挂；UI 测试通过 alert 文字定位）—— 留 constant 是为未来如改用 custom alert overlay 时直接用

**AC9 — Unit Test 覆盖**

新建 `iphone/PetAppTests/Features/DevTools/`（新目录）下：

1. `MockKeychainStore.swift`（继承 MockBase）：
```swift
@testable import PetApp
import Foundation

#if DEBUG
final class MockKeychainStore: MockBase, KeychainStoreProtocol, @unchecked Sendable {
    var setStubError: Error?
    var getStubResult: Result<String?, Error> = .success(nil)
    var removeStubError: Error?
    var removeAllStubError: Error?

    func set(_ value: String, forKey key: String) throws {
        record(method: "set(_:forKey:)", arguments: [value, key])
        if let e = setStubError { throw e }
    }
    func get(forKey key: String) throws -> String? {
        record(method: "get(forKey:)", arguments: [key])
        return try getStubResult.get()
    }
    func remove(forKey key: String) throws {
        record(method: "remove(forKey:)", arguments: [key])
        if let e = removeStubError { throw e }
    }
    func removeAll() throws {
        record(method: "removeAll()")
        if let e = removeAllStubError { throw e }
    }
}
#endif
```

2. `InMemoryKeychainStoreTests.swift`（≥ 2 case）：
   - `testSetGetReturnsValue`：`set` + `get` 同 key → 返回值相等
   - `testRemoveAllClearsStorage`：`set` 多个 key → `removeAll()` → `get` 各 key 返回 nil
   - `testRemoveAllOnEmptyDoesNotThrow`：空字典 `removeAll()` 不抛
   - （可选）`testThreadSafety`：并发 `set` 不崩 / 不丢失任何记录（不强求 — 占位实装即可）

3. `ResetKeychainUseCaseTests.swift`（≥ 2 case，用 `MockKeychainStore`）：
   - `testExecuteCallsRemoveAll`：`execute()` 调一次 `removeAll()`（`mockKeychainStore.callCount(of: "removeAll()")` == 1）
   - `testExecutePropagatesError`：`removeAllStubError` 设错 → `execute()` 抛同一错（`assertThrowsAsyncError` helper）

4. `ResetIdentityViewModelTests.swift`（≥ 3 case）：
   - `testTapHappyPathSetsSuccessAlert`：useCase 成功 → `alertContent == .success`
   - `testTapErrorPathSetsFailureAlert`：useCase 抛错 → `alertContent` 为 `.failure(message:)` 且 message 包含 "重置失败" + error description
   - `testAlertDismissedClearsAlertContent`：先成功 / 失败一次让 `alertContent` 非 nil → `alertDismissed()` → `alertContent == nil`
   - （可选）`testTapWhileLoadingDoesNotReentrant`：连点两次不应导致并发 race（不强求 — useCase 是 lightweight，本 story 不实装 reentrancy guard）

**所有测试必须**：
- 使用 `XCTest` 标准库（ADR-0002 §3.1）
- 使用 `MockBase` 子类记录 invocations（ADR-0002 §3.1 + Story 2.7 模板）
- 标 `@MainActor` 的测试 class（ResetIdentityViewModelTests）—— 因 ResetIdentityViewModel 是 @MainActor
- 使用 `async/await` 主流（ADR-0002 §3.2）；err 路径用 `await assertThrowsAsyncError(...)` helper（Story 2.7 落地的 AsyncTestHelpers）
- 测试 file 顶部 `#if DEBUG ... #endif` 包裹整个 class（与 SampleViewModelTests 同模式）
- 显式 `import Combine`（lesson `2026-04-25-swift-explicit-import-combine.md`）—— ResetIdentityViewModel 是 ObservableObject

**AC10 — UI Test 覆盖**

修改 `iphone/PetAppUITests/HomeUITests.swift`，追加：

```swift
// 复用现有 testHomeViewShowsAllSixPlaceholders 不动，追加新 test method：

func testResetIdentityButtonVisibleAndAlertOnTap() throws {
    let app = XCUIApplication()
    app.launch()

    let timeout: TimeInterval = 5

    // 1. 按钮存在且可点击（AccessibilityID.Home.btnResetIdentity）
    let btn = app.buttons[AccessibilityID.Home.btnResetIdentity]
    XCTAssertTrue(btn.waitForExistence(timeout: timeout), "重置身份按钮未找到（应在 Debug build 渲染）")

    // 2. 点击按钮
    btn.tap()

    // 3. alert 出现（通过 alert 内文字定位 — SwiftUI Alert 的 staticText 含 "已重置" 字样）
    let alertTitle = app.staticTexts["已重置"]
    XCTAssertTrue(alertTitle.waitForExistence(timeout: timeout), "重置成功 alert 未弹出")

    // 4. 点 OK 关闭 alert
    let okButton = app.alerts.buttons["OK"]
    XCTAssertTrue(okButton.waitForExistence(timeout: timeout), "alert OK 按钮未找到")
    okButton.tap()

    // 5. alert 消失（回到主界面，按钮仍存在）
    XCTAssertTrue(btn.waitForExistence(timeout: timeout), "回到主界面后按钮应仍存在")
}
```

**具体行为要求**：
- XCUITest 默认在 **Debug configuration** 跑（`xcodebuild test` 默认 Debug），所以 `#if DEBUG` 分支生效，按钮可见
- 通过 SwiftUI 原生 alert 在 XCUITest 中表现为 `app.alerts` 集合 —— 用 `app.alerts.buttons["OK"].tap()` 关闭
- alert title text "已重置" / button "OK" 来自 AC4 实装的 SwiftUI Alert
- **不**写 Release build "按钮不存在" UI 测试 —— XCUITest 在 Release configuration 跑需要 `xcodebuild -configuration Release`，本 story 不引入此 CI 路径；这个保证由 Unit test (AC9) + 编译器 fail-closed (`#if DEBUG` 包裹 type 定义) 充分覆盖

**AC11 — 验证：build + 全测试在本机跑通**

完成本 story 后**手动**跑（与 Story 2.7 AC7 同模式）：

```bash
# 1. build only
bash iphone/scripts/build.sh
# 期望: BUILD SUCCESS

# 2. build + unit tests（含本 story 新增 ≥ 9 case + 既有 90 tests）
bash iphone/scripts/build.sh --test
# 期望: BUILD SUCCESS + tests 全绿（90 + N tests，N >= 9）；包括本 story 新增的：
#   - InMemoryKeychainStoreTests（≥ 2 case）
#   - ResetKeychainUseCaseTests（≥ 2 case）
#   - ResetIdentityViewModelTests（≥ 3 case）

# 3. build + UI tests
bash iphone/scripts/build.sh --uitest
# 期望: BUILD SUCCESS + UI tests 全绿；包括本 story 新增的：
#   - testResetIdentityButtonVisibleAndAlertOnTap

# 4. clean（验证整体可重建）
bash iphone/scripts/build.sh --clean
# 期望: BUILD SUCCESS
```

把每条命令的 stdout 末尾贴 Completion Notes（特别是 `BUILD SUCCESS` 字样 + 测试 case 数量）。

**AC12 — 验证：既有测试零回归**

`bash iphone/scripts/build.sh --test` 必须确认 Story 2.2 / 2.3 / 2.4 / 2.5 / 2.6 / 2.7 既有所有测试**全绿**：

- `SheetTypeTests` / `AppCoordinatorTests` / `RootViewWireTests` / `AppContainerTests`（Story 2.2 / 2.3 / 2.5）
- `HomeViewModelTests` / `HomeViewModelPingTests` / `HomeViewTests`（Story 2.2 / 2.5）
- `APIClientTests` / `APIClientIntegrationTests`（Story 2.4）
- `PingUseCaseTests` / `PingUseCaseIntegrationTests`（Story 2.5）
- `AppErrorMapperTests` / `ErrorPresenterTests` / `ErrorComponentSnapshotTests`（Story 2.6）
- `SampleViewModelTests`（Story 2.7）

任一既有测试因本 story 改动 fail → 必须**立刻定位**（最可能根因：`AppContainer` init 签名变化破坏了 `AppContainerTests` 的现有构造调用 / `HomeView` init 签名变化破坏了 `HomeViewTests`）。**不能"先 commit 再修"**。

**特别注意**：
- `AppContainerTests`（Story 2.5 落地）测试 `AppContainer().apiClient` 等 — 本 story 给 `init(apiClient:keychainStore:)` 加默认 keychainStore 参数，旧测试调 `AppContainer(apiClient: ...)` **应继续 pass**（默认参数兼容）。如不 pass，**应 fix 测试** 而非 fix production
- `HomeViewTests`（Story 2.2 落地）测试 `HomeView(viewModel:)` 渲染 6 区块 — 本 story 给 `HomeView` 加 optional 第二参数 `resetIdentityViewModel:`，默认 `nil`，旧调用兼容；新加的 `userInfoBar` 内 `#if DEBUG` 分支在 Debug build 测试运行时**会**渲染按钮，但只要 `resetIdentityViewModel == nil` 就**不**渲染（`if let` guard）—— 旧测试构造 `HomeView(viewModel:)` 不传第二参数，按钮不出现，6 区块测试零影响

**AC13 — `git status` 净化**

完成本 story 后 `git status` 应仅含**新增**：

- `iphone/PetApp/Core/Storage/KeychainStore.swift`
- `iphone/PetApp/Features/DevTools/Views/ResetIdentityButton.swift`
- `iphone/PetApp/Features/DevTools/UseCases/ResetKeychainUseCase.swift`
- `iphone/PetApp/Features/DevTools/ViewModels/ResetIdentityViewModel.swift`
- `iphone/PetAppTests/Features/DevTools/MockKeychainStore.swift`
- `iphone/PetAppTests/Features/DevTools/InMemoryKeychainStoreTests.swift`
- `iphone/PetAppTests/Features/DevTools/ResetKeychainUseCaseTests.swift`
- `iphone/PetAppTests/Features/DevTools/ResetIdentityViewModelTests.swift`

以及**修改**：

- `iphone/PetApp/App/AppContainer.swift`（追加 keychainStore 字段 + 工厂方法 + init 默认参数）
- `iphone/PetApp/App/RootView.swift`（`#if DEBUG` 加 @StateObject + init + body 切换）
- `iphone/PetApp/Features/Home/Views/HomeView.swift`（`userInfoBar` 内 `#if DEBUG` 加按钮 + 新 init 参数）
- `iphone/PetApp/Shared/Constants/AccessibilityID.swift`（追加 2 个 const）
- `iphone/PetAppUITests/HomeUITests.swift`（追加 1 个 test method）
- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen regen 副作用，预期）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（`2-8-...` 状态流转 backlog → ready-for-dev → in-progress → review → done）
- `_bmad-output/implementation-artifacts/2-8-dev-重置-keychain-按钮.md`（本 story 文件：Tasks 勾选 / Dev Agent Record / Status 流转）

**绝对不应出现**：

- `ios/` 下任何 diff
- `server/` 下任何 diff
- `iphone/project.yml` diff（glob 自动纳入）
- `iphone/scripts/install-hooks.sh` / `iphone/scripts/git-hooks/pre-commit` diff
- `iphone/scripts/build.sh` diff（Story 2.7 落地，本 story 不动）
- `iphone/PetApp/Shared/Testing/SampleTypes.swift` / `iphone/PetAppTests/Helpers/{MockBase.swift, AsyncTestHelpers.swift, SampleViewModelTests.swift}` diff（Story 2.7 资产，本 story 不动）
- `iphone/PetAppTests/Core/Networking/{MockURLSession.swift, StubURLProtocol.swift, APIClientTests.swift, APIClientIntegrationTests.swift}` diff（Story 2.4，本 story 不动）
- `iphone/PetAppTests/Features/Home/UseCases/{MockAPIClient.swift, PingStubURLProtocol.swift}` diff（Story 2.5，本 story 不动）
- `iphone/PetApp/Shared/ErrorHandling/*` diff（Story 2.6，本 story 不动）
- `iphone/PetApp/Core/DesignSystem/Components/*` diff（Story 2.6，本 story 不动）
- `iphone/PetApp/Core/Networking/*` diff（Story 2.4 / 2.5，本 story 不动）
- `iphone/PetApp/Features/Home/{ViewModels/HomeViewModel.swift, UseCases/*}` diff（Story 2.2 / 2.5，本 story 不动）
- `iphone/PetApp/App/AppCoordinator.swift` / `PetAppApp.swift` diff（Story 2.2 / 2.3，本 story 不动）
- `iphone/.gitignore` / `.gitignore` diff
- `CLAUDE.md` / `docs/`（除 lessons/ 在 review 阶段产生）下任何 diff

## Tasks / Subtasks

- [x] **T1** — `KeychainStore.swift`（AC1）
  - [x] T1.1 写 `KeychainStoreProtocol` 4 方法（set / get / remove / removeAll，全部 throws）
  - [x] T1.2 写 `InMemoryKeychainStore` 占位实装（`[String: String]` + NSLock + `@unchecked Sendable`）
  - [x] T1.3 文件头注释明示"Story 5.1 替换为 KeychainServicesStore"

- [x] **T2** — `ResetKeychainUseCase.swift`（AC2）
  - [x] T2.1 写 `ResetKeychainUseCaseProtocol`（`async throws`）
  - [x] T2.2 写 `DefaultResetKeychainUseCase` struct（init + execute）

- [x] **T3** — `ResetIdentityViewModel.swift`（AC3）
  - [x] T3.1 写 `ResetIdentityAlertContent` enum（success / failure(message:)）
  - [x] T3.2 写 `ResetIdentityViewModel` @MainActor class（`@Published alertContent` + `tap()` async + `alertDismissed()`）
  - [x] T3.3 显式 `import Combine`

- [x] **T4** — `ResetIdentityButton.swift`（AC4）
  - [x] T4.1 整个 type 定义 + extension 包 `#if DEBUG`
  - [x] T4.2 SwiftUI View（SF Symbol arrow.counterclockwise.circle + Button + .alert(item:)）
  - [x] T4.3 `ResetIdentityAlertContent: Identifiable` extension
  - [x] T4.4 `accessibilityLabel("重置身份")` + `accessibilityIdentifier(AccessibilityID.Home.btnResetIdentity)`
  - [x] T4.5 alert 文案对齐 epics.md AC（"已重置，请杀进程后重新启动 App 模拟首次安装"）

- [x] **T5** — `AppContainer.swift` 修改（AC6）
  - [x] T5.1 加 `public let keychainStore: KeychainStoreProtocol` 字段
  - [x] T5.2 改 `init(apiClient:)` 为 `init(apiClient:keychainStore:)`，keychainStore 默认 `InMemoryKeychainStore()`
  - [x] T5.3 加 `makeResetKeychainUseCase()` 工厂
  - [x] T5.4 加 `#if DEBUG makeResetIdentityViewModel() #endif`
  - [x] T5.5 验证 Story 2.5 既有 API（apiClient / errorPresenter / makePingUseCase / resolveDefaultBaseURL / validatedBaseURL）零改动

- [x] **T6** — `HomeView.swift` 修改（AC5）
  - [x] T6.1 新增 optional `resetIdentityViewModel: ResetIdentityViewModel?` 字段（默认 `nil`）（用 plain `let` 持有 — `@ObservedObject` 不接 Optional；ResetIdentityButton 子 view 自行 `@ObservedObject viewModel` 完成订阅；本 View 仅引用透传）
  - [x] T6.2 保留旧 `init(viewModel:)` + 新增 `init(viewModel:resetIdentityViewModel:)`（两 init 并存，旧调用方零改动）
  - [x] T6.3 改 `userInfoBar` 计算属性：`Spacer()` 后追加 `#if DEBUG if let resetIdentityViewModel { ResetIdentityButton(viewModel: resetIdentityViewModel) } #endif`
  - [x] T6.4 改 `accessibilityElement(children: .ignore)` → `.contain`（让按钮 a11y 可定位）
  - [x] T6.5 验证：其他 5 区块代码零改动

- [x] **T7** — `RootView.swift` 修改（AC7）
  - [x] T7.1 加 `#if DEBUG @StateObject private var resetIdentityViewModel #endif`
  - [x] T7.2 加 `init()`（仅 `#if DEBUG` 内逻辑：用临时 standalone `AppContainer()` 初始化 `_resetIdentityViewModel`）
  - [x] T7.3 改 body 内 `HomeView(viewModel:)` 调用为条件编译双分支（提取为 `homeView` @ViewBuilder helper）
  - [x] T7.4 验证 .task / .fullScreenCover / .errorPresentationHost 链路零改动

- [x] **T8** — `AccessibilityID.swift` 修改（AC8）
  - [x] T8.1 在 `Home` enum 末尾追加 `btnResetIdentity` + `resetIdentityAlert`

- [x] **T9** — Unit Tests（AC9）
  - [x] T9.1 `MockKeychainStore.swift` 继承 MockBase + 4 stub 字段
  - [x] T9.2 `InMemoryKeychainStoreTests`（7 case：set+get、覆盖、不存在 key 返 nil、removeAll、空 storage、remove 单 key、remove 不存在 key）
  - [x] T9.3 `ResetKeychainUseCaseTests`（3 case：execute 调 removeAll / 错误透传 / idempotent 多次调用）
  - [x] T9.4 `ResetIdentityViewModelTests`（5 case：happy / error / dismiss after success / dismiss after failure / initial nil；使用 local MockResetKeychainUseCase 而非 MockKeychainStore，更聚焦）
  - [x] T9.5 所有测试 file 顶层 `#if DEBUG` 包裹

- [x] **T10** — UI Test（AC10）
  - [x] T10.1 在 `HomeUITests.swift` 追加 `testResetIdentityButtonVisibleAndAlertOnTap`
  - [x] T10.2 验证：旧 `testHomeViewShowsAllSixPlaceholders` 零改动

- [x] **T11** — 手动验证（AC11 / AC12）
  - [x] T11.1 `bash iphone/scripts/build.sh` → BUILD SUCCESS
  - [x] T11.2 `bash iphone/scripts/build.sh --test` → BUILD SUCCESS + 108 tests 全绿（90 既有 + 18 新增）
  - [x] T11.3 `bash iphone/scripts/build.sh --uitest` → BUILD SUCCESS + 含 `testResetIdentityButtonVisibleAndAlertOnTap`（passed in 9.279s）
  - [x] T11.4 既有测试零回归（AppContainerTests 6 case 全绿；HomeViewTests 5 case 全绿；HomeViewModelTests / HomeViewModelPingTests 全绿）
  - [x] T11.5 把命令面输出贴 Completion Notes

- [x] **T12** — git status 净化（AC13）
  - [x] T12.1 `git status` 文件清单严格匹配 AC13
  - [x] T12.2 `git diff ios/` 无输出
  - [x] T12.3 `git diff server/` 无输出
  - [x] T12.4 `git diff iphone/project.yml` 无输出（xcodegen regen 仅改 .pbxproj 不改 yml）
  - [x] T12.5 `git diff iphone/scripts/` 无输出（不改 build.sh / install-hooks）

- [x] **T13** — 收尾
  - [x] T13.1 Completion Notes 补全
  - [x] T13.2 File List 填充
  - [x] T13.3 状态流转 `ready-for-dev → in-progress → review`

## Dev Notes

### 项目关键约束（必读，勿绕过）

1. **`#if DEBUG` 是 iOS 端唯一 dev/release gate**：与 server 端 Story 1.6 的 `BUILD_DEV` env var + `-tags devtools` 双闸门**显式不对齐** —— iOS 平台没有 build tag 概念，唯一对应是 `SWIFT_ACTIVE_COMPILATION_CONDITIONS`，Xcode 默认 Debug 配置已含 `DEBUG`。Release build 通过 `-configuration Release` 触发，`DEBUG` 宏关，`#if DEBUG` 块编译期被剔除 —— 是真正的 fail-closed（视图树不存在、type 不存在、调用代码不存在；不只是"显示=false"）

2. **lesson `2026-04-25-dev-mode-or-gate-sop-accuracy.md` 应用**：本 story 没有"双闸门"措辞 —— 只有**一个闸门**（`#if DEBUG`），所以不要写"双重保险"类描述。AC4 文件头注释 "**仅在 Debug build 渲染**；Release build 视图树物理不存在" 严格对齐 `#if DEBUG` 的单闸门语义

3. **Release build 视图树"物理不存在" vs "hidden"**：epics.md AC 字面 "完全不渲染（不只是 hidden，是不存在于视图树）" —— 实装手段必须是 `#if DEBUG ... #endif` 编译期剔除；**禁止**用 `.opacity(0)` / `.hidden()` / `.disabled(true)` / 运行时 if-false。验证：在 Release configuration build 一次（`xcodebuild build -configuration Release ...`），grep 编译产物含字符串 "重置身份" 应**不命中**（不强求本 story 验证 Release build，本 story 仅在 Debug 验证；但实装必须保证 Release build **能** compile pass —— `#if DEBUG` 包裹必须严密）

4. **`ResetIdentityButton` 不能继承 / 复用 Story 2.6 `AlertOverlayView`**：那是给 `APIError.business` 用的统一业务错误 alert；dev 操作反馈是**另一条**UI 路径。强行复用 = 让 ErrorPresenter 多承担 dev 工具职责 + 增加 ErrorPresentationHost ViewModifier 复杂度

5. **占位 KeychainStore 的命名前缀 `InMemory*`**：Story 5.1 上线时改 `KeychainServicesStore` —— 命名风格对齐 `URLSession.shared` / `UserDefaults.standard` 等 Apple 内置类型。**不**用 `MockKeychainStore` 作 production 占位（容易与测试 mock 混淆）；测试 mock 严格走 `MockKeychainStore`（对齐 `MockURLSession` / `MockAPIClient`）

6. **lesson `2026-04-26-stateobject-init-vs-bind-injection.md` 应用**：本 story RootView 用临时 standalone `AppContainer()` 喂 `_resetIdentityViewModel = StateObject(wrappedValue: ...)`，**不**走 bind 模式。理由 ① 当前阶段 keychainStore 是占位 `InMemoryKeychainStore()`，一个 viewModel 用一个还是另一个 store 实例**无功能差**；② Story 5.1 真 Keychain 上线时如果发现必须单例，再改 bind 路径（届时把 `RootView.init` 删 + 走 `.task { homeViewModel.bind... }` 同 pattern）—— 本 story 只为兑现 epics.md AC，简实装

7. **HomeView 加 optional 第二 init 参数**：默认 `nil`，确保 Story 2.2 / 2.5 既有所有 `HomeView(viewModel: ...)` 调用零改动 —— 包括 `HomeView_Previews` / `HomeViewTests` / Story 2.2 落地测试。**不**走"加新 init 重载"路径（重载多 = 测试覆盖矩阵爆炸）

8. **AppContainer init 加 keychainStore 默认参数**：与 HomeView 同思路 —— 默认 `InMemoryKeychainStore()` 让 Story 2.5 `AppContainer(apiClient: mockClient)` 等老调用零改动。**不**走"加新 init 重载"

9. **userInfoBar `accessibilityElement(children:)` 从 `.ignore` 改为 `.contain`**：`.ignore` 把整个 HStack 当成 a11y 整体（孩子的 identifier 不可被 XCUITest 访问），新加 button 的 identifier 会被屏蔽 —— 改 `.contain` 让父 a11y 仍存在但孩子 a11y 也可访问。验证：UI test `testHomeViewShowsAllSixPlaceholders` 仍通过（既有 `app.descendants(matching: .any)[AccessibilityID.Home.userInfo]` 仍可定位）

10. **`SwiftUI .alert(item:)` 要求 item 是 Identifiable**：`ResetIdentityAlertContent: Identifiable` extension 必须放在**与 enum 同 file** 或 ResetIdentityButton.swift 内（且也包 `#if DEBUG`，否则 Release build 找不到 enum）。本 story 选 ResetIdentityButton.swift 内放 extension（与按钮 + alert 是一组耦合体）

11. **`@MainActor` 标 ResetIdentityViewModel 但测试方法标 `@MainActor`**：测试 class 整体标 `@MainActor` 即可（与 SampleViewModelTests 同模式）；class 级标注会传播到所有 method

12. **测试 file 顶部 `#if DEBUG` 包裹**：与 SampleViewModelTests 同模式 —— 确保 Release build 测试 bundle 不包含 dev 工具测试 case；同时 `MockKeychainStore` / 各 test class 引用的 `ResetIdentityButton` / `ResetIdentityViewModel` / `ResetIdentityAlertContent` type 在 Release build 不存在，测试如果 Release build 编 fail 那是预期 —— XCTest bundle 只在 Debug 跑

13. **不要在测试中验证 Security.framework 行为**：本 story 占位实装 `InMemoryKeychainStore` 不接触 Security framework，测试不应间接触发 kSecClass*；Story 5.1 真实实装时单独写 KeychainServicesStore tests

14. **`tap()` 不需要并发短路 (vs HomeViewModel.start() 的三层短路)**：dev 工具用户**预期**点完一次想再点（demo 给另一组人看）；`@Published alertContent` 让用户感知到"刚才那次"已结束；不需要 `pingTask != nil` 那种短路

15. **alert dismiss 时 `alertContent = nil` 必须 explicit**：SwiftUI `.alert(item:)` 在 user 点 OK 后**理论**自动把 binding 复位为 nil（双向绑定）；但实测有些 Xcode 版本下 OK 后 binding 不立即 nil。`alertDismissed()` 显式 reset 是兜底 —— 与 Story 2.6 `ErrorPresenter.dismiss()` 同思路

### 为什么不在本 story 做这些

- **真实 Security.framework / kSecClassGenericPassword 调用**：Story 5.1（Epic 5）专门做 KeychainStore 真实封装；本 story 占位 + 协议接缝够用
- **dev login override（自动登录 mock token）**：Story 4.6 / 5.2 范围
- **dev API server switcher**（切换 baseURL）：未来工程化决策；当前 baseURL 已通过 Info.plist `PetAppBaseURL` 可换（Story 2.5 落地），dev 工具按钮不必再加
- **dev grant-steps 按钮**：server 端 Story 7.5 落地 dev 端点；iOS 端 dev grant button 在 Epic 8 步数同步上线后视需求加（YAGNI）
- **Confirm dialog 二次确认 ResetIdentity**：epics.md AC 没要求；按钮直接触发 reset，alert 显示在动作之后；如未来发现误触代价大再加
- **多个 dev 工具按钮的统一管理（DevToolsPanel）**：当前只 1 个按钮，单按钮直接挂 userInfoBar；未来加第 2 个 dev 按钮时再考虑 panel 抽象（YAGNI）
- **集成测试（如 ResetKeychain + 重启 App 模拟）**：UITest 不能触发 App 重启；本 story 仅验证按钮 + alert 链路，"重启后表现"由 Epic 3 demo 验收人工跑

### 与 Server Story 1.6 的对照（dev-tools 框架）

| 维度 | Server (Story 1.6) | iPhone (本 Story 2.8) |
|---|---|---|
| 形态 | HTTP 路由组 `/dev/*` | UI 按钮 + 本地动作 |
| Gate 机制 | `BUILD_DEV` env + `-tags devtools` (OR) | `#if DEBUG` 编译宏（单闸门） |
| 双闸门 | 路由注册闸 + 请求时闸（中间件二次校验） | 视图树物理不存在（编译期单闸门，无运行时校验） |
| 配置 | env / build tag 双触发源 | Xcode build configuration（默认 Debug 含 DEBUG 宏） |
| 启动日志 | `slog.Warn("DEV MODE ENABLED")` | 无（iOS 用户不会看 log；按钮存在即为视觉信号） |
| 示例端点 | `GET /dev/ping-dev` | "重置身份"按钮（不是示例，是真功能） |
| 响应 envelope | 统一 `{code, message, data, requestId}` | SwiftUI Alert（dev 工具不走业务错误体系） |
| 防御纵深 | route 注册时 + 请求 handler 时双 check | 编译期单 check（运行时不可能进入） |

跨端**精神对齐**（dev 工具与生产严格隔离）但**实装机制完全不同** —— 与 ADR-0002 §3.4 "跨端 dev 切换零认知摩擦" 的"零认知摩擦"指**入口命令面**（`bash scripts/build.sh --test` vs `bash iphone/scripts/build.sh --test`），不是要求实装一致

### 与 Story 2.7 既有测试基础设施的关系

| Story 2.7 资产 | 本 story 用法 |
|---|---|
| `MockBase` | `MockKeychainStore` 直接继承 |
| `AsyncTestHelpers.assertThrowsAsyncError` | `ResetKeychainUseCaseTests.testExecutePropagatesError` 直接调 |
| `AsyncTestHelpers.awaitPublishedChange` | 本 story 测试不需要（tap() 是单次状态切换，直接 await + 读 alertContent 即可） |
| `SampleTypes` / `SampleViewModelTests` 模板 | `ResetIdentityViewModelTests` **复制结构**（mock 注入 + setUp/tearDown + ≥3 case + #if DEBUG） |

### Lessons Index（与本 story 相关的过去教训）

- **`docs/lessons/2026-04-25-swift-explicit-import-combine.md`** —— 直接相关：`ResetIdentityViewModel` / `MockKeychainStore` / 各测试 class 都用 ObservableObject / @Published / Combine sink，**必须显式 `import Combine`**
- **`docs/lessons/2026-04-25-dev-mode-or-gate-sop-accuracy.md`** —— 间接相关：本 story 是单闸门（`#if DEBUG`），不要写"双重保险"类描述；AC4 / Dev Note #2 已遵守
- **`docs/lessons/2026-04-26-stateobject-init-vs-bind-injection.md`** —— 直接相关：`RootView.init()` 用临时 standalone `AppContainer()` 喂 `_resetIdentityViewModel`；不走 bind 路径（理由见 Dev Note #6）
- **`docs/lessons/2026-04-26-fullscreencover-isolated-environment.md`** —— 间接相关：本 story alert 不走 sheet 子树，直接挂在 ResetIdentityButton 内 `.alert(item:)`，与 `.fullScreenCover` 隔离 environment 无关
- **`docs/lessons/2026-04-26-modal-overlay-content-shield.md`** —— 间接相关：SwiftUI 原生 `.alert` 自带 hit-testing 屏蔽（系统级），本 story 不需要自定 overlay shield
- **`docs/lessons/2026-04-26-mockbase-snapshot-only-reads.md`** —— 直接相关：`MockKeychainStore` 子类的 stub 字段（`removeAllStubError` 等）是 mutable state，但**写入只在 setUp + 之前**、读取在 method body —— 没有 race；MockBase 内部存储字段保持 private，子类不暴露
- **`docs/lessons/2026-04-26-objectwillchange-no-initial-emit.md`** + **`docs/lessons/2026-04-26-published-publisher-vs-objectwillchange.md`** —— 间接相关：本 story 测试不直接观察 `alertContent` 多次变化（单次 `tap()` 只触发一次状态切换），不踩这两条 lesson 的具体坑；但 `@MainActor` 标注 + `await sut.tap()` 后直接读 `sut.alertContent` 是正确模式
- **`docs/lessons/2026-04-26-swiftui-task-modifier-reentrancy.md`** —— 间接相关：本 story `tap()` 不在 `.task` modifier 内自动重入（按钮显式 user action 触发），不踩此 lesson 的坑
- **`docs/lessons/2026-04-26-baseurl-from-info-plist.md` / `2026-04-26-url-string-malformed-tolerance.md` / `2026-04-26-baseurl-host-only-contract.md`** —— 不相关：本 story 不接 APIClient / baseURL
- **`docs/lessons/2026-04-26-error-presenter-queue-onretry-loss.md`** —— 不相关：本 story 不走 ErrorPresenter，alert 走 SwiftUI 原生 .alert(item:)
- **`docs/lessons/2026-04-26-jsondecoder-encoder-thread-safety.md`** —— 不相关：本 story 不持 JSONDecoder
- **`docs/lessons/2026-04-26-urlprotocol-stub-global-state.md` / `2026-04-26-urlprotocol-session-local-vs-global.md`** —— 不相关：本 story 不动 networking 测试
- **`docs/lessons/2026-04-26-build-script-flag-matrix.md`** —— 不相关：本 story 不动 build.sh
- **`docs/lessons/2026-04-26-xcodebuild-showdestinations-section-aware.md` / `2026-04-26-simulator-placeholder-vs-concrete.md`** —— 不相关：本 story 不动 destination resolution

### Git intelligence（最近 6 个 commit）

- `def015f` (HEAD)
- `3b2973d docs(bmad-output): 更新 0002-ios-stack`
- `18c92e8 chore: 更新 CLAUDE`
- `b80fd14 chore(claude): 更新 Bash allowlist`
- `71b5b93 chore(story-2-1): 收官 Story 2.1 + 归档 story 文件`
- `954c28a 常用 claude 默认允许命令`
- （更早是 Story 2.7 的 dev-story 实装 + review 链）

**最近实装向 commit** 是 Story 2.7（iOS 测试基础设施）的 review fix 链；Story 2.8 紧随 Epic 2 实装顺序。

**commit message 风格**：Conventional Commits，中文 subject，scope `story-X-Y` / `iphone` / `feat-devtools` 类。
本 story 建议：`feat(iphone,story-2-8): Epic2/2.8 dev "重置身份" 按钮（#if DEBUG gated）`

### 常见陷阱

1. **`#if DEBUG` 不严密导致 Release build 编译失败**：检查 `ResetIdentityButton` / `ResetIdentityAlertContent` / `MockKeychainStore` / `AppContainer.makeResetIdentityViewModel()` / `RootView` 内 viewModel 字段及其 init 都包 `#if DEBUG`；任一遗漏 → Release build "cannot find 'ResetIdentityButton'" 等编译错。**修复**：grep `iphone/PetApp` 下 "ResetIdentity" 字符串，每处对照确认在 `#if DEBUG` 块内或在 `#if DEBUG` 包裹的 type 内

2. **`HomeView` 第二 init 参数破坏 Preview**：`HomeView_Previews` 内 `HomeView(viewModel: HomeViewModel())` 调旧 init 路径；只要新 init 有默认 `nil` 参数，preview 仍 compile pass。**验证**：build Preview（`xcodebuild build`）

3. **`AppContainerTests` 内 `AppContainer(apiClient: ...)` 因 keychainStore 必填失败**：本 story 给 keychainStore 加默认参数 `InMemoryKeychainStore()`，旧调用兼容。**验证**：bash iphone/scripts/build.sh --test 全绿

4. **`ResetIdentityViewModelTests` 用 `await sut.tap()` 但 sut 不是 @MainActor isolated**：测试 class 标 `@MainActor` 让所有 method 跑在 main actor；`sut: ResetIdentityViewModel` 默认在 main actor 上调度 → `await sut.tap()` 编译 + 运行通过

5. **UI test 找不到 "OK" 按钮**：SwiftUI `Alert` 在 XCUITest 中按钮 label 是 "OK"（与 `Alert.Button.default(Text("OK"))` 一致）；如果 dev 写成 `Text("好的")` 或 `Text("Confirm")`，UITest 文字定位会失败。AC4 模板严格用 "OK"

6. **`accessibilityElement(children:)` 从 `.ignore` 改成 `.contain` 后既有 UITest 失败**：原 UITest 用 `app.descendants(matching: .any)[AccessibilityID.Home.userInfo]` 定位整个 bar；`.contain` 让父 a11y element 仍存在 + 子 element 也可定位 —— 父定位**仍可通过**（`.contain` ≠ `.combine` ≠ `.ignore`）。**验证**：现有 `testHomeViewShowsAllSixPlaceholders` 全绿

7. **`MockKeychainStore` 被 PetApp target 误纳入**：检查 `iphone/project.yml` `PetAppTests` target 的 `sources: - PetAppTests` glob 自动纳入 `Features/DevTools/MockKeychainStore.swift`；不修改 project.yml；如果手动改成 `- "**/*.swift"` 或类似，mock 会被编译进主 App。**不**修改 project.yml

8. **测试 setUp 不重置 mock**：`MockKeychainStore` 多次 setUp 复用同一实例时不自动 reset() —— 但本 story 测试 setUp 每次 new `mockKeychainStore = MockKeychainStore()`（与 SampleViewModelTests 同模式），不需要 reset。**不**走"reuse mock + setUp 调 reset()"模式

9. **alert 文案中"杀进程"用词**：epics.md AC 字面要求 "已重置，请杀进程后重新启动 App 模拟首次安装" —— **不**改成"请退出 App 后重新启动"（虽然字面更友好），保持与原 AC 一字不改，避免 LLM 自由发挥

10. **`Bundle.main` 在 #if DEBUG 内的 readAppVersion** 等不动：本 story 不接 Bundle / appVersion；HomeViewModel 完全不动

11. **`@StateObject` 在 RootView.init 内的赋值方式**：必须用 `_resetIdentityViewModel = StateObject(wrappedValue: ...)` 形式（带下划线前缀 + StateObject literal init），**不**用 `resetIdentityViewModel = ResetIdentityViewModel(...)`（这是给运行时实例直接赋值，不会被 SwiftUI 识别为 StateObject 注册）。这是 SwiftUI runtime 的硬约束，与 lesson `2026-04-26-stateobject-init-vs-bind-injection.md` 描述的另一边坑相关

12. **`Sendable` 标注 vs `@unchecked Sendable`**：`KeychainStoreProtocol: Sendable`、`InMemoryKeychainStore: KeychainStoreProtocol, @unchecked Sendable`、`MockKeychainStore: MockBase, KeychainStoreProtocol, @unchecked Sendable` —— 与 Story 2.4 / 2.5 既有 mock 风格一致

### Project Structure Notes

- **新增**：
  - `iphone/PetApp/Core/Storage/KeychainStore.swift`（**新子目录** `Core/Storage/`，与 iOS 架构 §4 `Core/Storage/{KeychainStore.swift, UserDefaultsStore.swift, LocalCache.swift}` 文件结构对齐）
  - `iphone/PetApp/Features/DevTools/Views/ResetIdentityButton.swift`（**新顶级 Feature** `DevTools/`，与 Auth / Home / Pet 等并列）
  - `iphone/PetApp/Features/DevTools/UseCases/ResetKeychainUseCase.swift`
  - `iphone/PetApp/Features/DevTools/ViewModels/ResetIdentityViewModel.swift`
  - `iphone/PetAppTests/Features/DevTools/MockKeychainStore.swift`（**新测试目录** 镜像 production）
  - `iphone/PetAppTests/Features/DevTools/InMemoryKeychainStoreTests.swift`
  - `iphone/PetAppTests/Features/DevTools/ResetKeychainUseCaseTests.swift`
  - `iphone/PetAppTests/Features/DevTools/ResetIdentityViewModelTests.swift`

- **修改**（既有文件）：
  - `iphone/PetApp/App/AppContainer.swift`（追加字段 + 工厂方法 + init 默认参数）
  - `iphone/PetApp/App/RootView.swift`（`#if DEBUG` 包裹的 @StateObject + init + body 双分支）
  - `iphone/PetApp/Features/Home/Views/HomeView.swift`（`userInfoBar` 内 `#if DEBUG` 加按钮 + 新 init 参数）
  - `iphone/PetApp/Shared/Constants/AccessibilityID.swift`（追加 2 个 const）
  - `iphone/PetAppUITests/HomeUITests.swift`（追加 1 个 test method）

- **不新增 / 不修改**：
  - `iphone/project.yml`（glob 自动纳入新文件 + 新子目录）
  - `iphone/scripts/build.sh` / `install-hooks.sh` / `git-hooks/pre-commit`
  - 任何 Story 2.4 / 2.5 / 2.6 / 2.7 既有 production 文件（除 4 个明确改动点）
  - 任何 Story 2.4 / 2.5 / 2.6 / 2.7 既有测试文件
  - `iphone/PetApp/Shared/Testing/SampleTypes.swift`（Story 2.7 落地，本 story 不动）
  - `.gitignore` / `CLAUDE.md` / `docs/`（除 lessons/ 在 review 阶段产生）

- **xcodegen auto-regen 副作用**：新增 `Core/Storage/` + `Features/DevTools/` 子目录 + 8 个新 .swift 文件后 build.sh 自动跑 `xcodegen generate`；预期 `iphone/PetApp.xcodeproj/project.pbxproj` 会有 diff（xcodegen 重排 references）—— 这是 `iphone/project.yml` 不变 + .pbxproj 由 yml 生成的标准模式，与 Story 2.7 同

- **测试目录镜像约定**：`PetAppTests/Features/DevTools/` 镜像 `PetApp/Features/DevTools/`（与 `PetAppTests/Features/Home/` 镜像 `PetApp/Features/Home/` 同模式）；`MockKeychainStore.swift` 放 `PetAppTests/Features/DevTools/` 而非 `PetAppTests/Helpers/` —— 因为 KeychainStore 是 Storage feature 专属 mock（与 Story 2.4 `MockURLSession` 放 `PetAppTests/Core/Networking/` 而非 `Helpers/` 同决策）

### References

- [Source: \_bmad-output/planning-artifacts/epics.md#Epic 2 / Story 2.8] — 原始 AC 来源（行 812-834）
- [Source: \_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md] — **本 story 唯一权威 ADR**
  - §3.1 — XCTest only（手写 Mock）：本 story `MockKeychainStore` 直接继承 `MockBase`
  - §3.2 — async/await 主流：本 story `tap()` / `execute()` 全部 async
  - §3.3 — 方案 D：本 story 在 `iphone/` 下落地，零 `ios/` 改动
  - §3.4 — CI 跑法：本 story 通过 `bash iphone/scripts/build.sh --test/--uitest` 验证
- [Source: \_bmad-output/implementation-artifacts/decisions/0001-test-stack.md] — server 端 ADR；与本 story 间接对照（dev tools 跨端精神对齐）
- [Source: \_bmad-output/implementation-artifacts/2-1-ios-mock-框架选型-ios-目录决策-spike.md] — Story 2.1（已 done）：iOS 工具栈 spike
- [Source: \_bmad-output/implementation-artifacts/2-2-swiftui-app-入口-主界面骨架-信息架构定稿.md] — Story 2.2（已 done）：iphone/ 工程骨架 + HomeView 6 区块 + AccessibilityID
- [Source: \_bmad-output/implementation-artifacts/2-3-导航架构搭建.md] — Story 2.3（已 done）：AppCoordinator + sheet 路由
- [Source: \_bmad-output/implementation-artifacts/2-4-apiclient-封装.md] — Story 2.4（已 done）：MockURLSession + StubURLProtocol
- [Source: \_bmad-output/implementation-artifacts/2-5-ping-调用-主界面显示-server-version-信息.md] — Story 2.5（已 done）：MockAPIClient + AppContainer 落地
- [Source: \_bmad-output/implementation-artifacts/2-6-基础错误-ui-框架.md] — Story 2.6（已 done）：ErrorPresenter / Toast / AlertOverlay / RetryView（本 story alert **不**复用 AlertOverlay，理由见 Dev Note #4）
- [Source: \_bmad-output/implementation-artifacts/2-7-ios-测试基础设施搭建.md] — Story 2.7（已 done）：MockBase + AsyncTestHelpers + iphone/scripts/build.sh + sample template
- [Source: \_bmad-output/implementation-artifacts/1-6-dev-tools-框架.md] — server 端类比 story（已 done）：Server 端 dev-tools HTTP 路由组（与本 story 跨端精神对照）
- [Source: iphone/PetApp/App/AppContainer.swift:12] — Story 2.5 落地注释 "按需追加 KeychainStore / SessionRepository / WebSocketClient 等" —— 本 story 兑现"按需追加"第一步
- [Source: iphone/PetApp/Features/Home/Views/HomeView.swift] — Story 2.2 主界面 6 区块；本 story 修改 `userInfoBar`
- [Source: iphone/PetApp/App/RootView.swift] — Story 2.3 / 2.5 RootView wire；本 story `#if DEBUG` 加 @StateObject
- [Source: iphone/PetApp/Shared/Constants/AccessibilityID.swift] — Story 2.2 落地；本 story 追加 enum case
- [Source: iphone/PetApp.xcodeproj/project.pbxproj] — xcodegen regen 副作用（每次 build.sh 跑都会 update）
- [Source: iphone/project.yml] — Story 2.2 既有工程定义；本 story **0 改动**（glob 自动纳入新文件 + 新子目录）
- [Source: iphone/PetAppTests/Helpers/MockBase.swift] — Story 2.7 落地的 MockBase；本 story `MockKeychainStore` 继承
- [Source: iphone/PetAppTests/Helpers/AsyncTestHelpers.swift] — Story 2.7 落地的 `assertThrowsAsyncError`；本 story 测试用
- [Source: iphone/PetAppTests/Helpers/SampleViewModelTests.swift] — Story 2.7 模板；本 story `ResetIdentityViewModelTests` 复制结构
- [Source: iphone/PetAppUITests/HomeUITests.swift] — Story 2.2 落地；本 story 追加 1 个 test method
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#4 项目目录建议] — `Core/Storage/{KeychainStore.swift, UserDefaultsStore.swift, LocalCache.swift}` 文件结构（line 133-136）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#11.1 Keychain] — Keychain 保存 guestUid + token（line 631-639）；本 story 占位实装为 Story 5.1 真实落地预留接缝
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#5.4 Repository 层] — Storage 是 Repository 层底层（line 266-279）；KeychainStore 是 Storage adapter
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#3.1 客户端总体结构] — Keychain Adapter 在 System Adapter 层（line 86-90）；本 story 协议定义在 Core/Storage/，符合"Adapter pattern"
- [Source: CLAUDE.md "Repo Separation（重启阶段过渡态）"] — 三目录约束（`server/` `iphone/` `ios/`）；本 story 严格守 `iphone/` only
- [Source: CLAUDE.md "节点顺序不可乱跳"] — 本 story 仅做"清 Keychain"动作的 dev 工具 + 协议接缝；不实装真实 Security framework（那是 Story 5.1 节点 2 范围）
- [Source: docs/lessons/2026-04-25-swift-explicit-import-combine.md] — **必读**：ObservableObject / @Published 必须显式 import Combine
- [Source: docs/lessons/2026-04-25-dev-mode-or-gate-sop-accuracy.md] — 必读（措辞约束）：单闸门不要写"双重保险"
- [Source: docs/lessons/2026-04-26-stateobject-init-vs-bind-injection.md] — **必读**：RootView.init 内 `_resetIdentityViewModel = StateObject(wrappedValue: ...)` 模式
- [Source: docs/lessons/2026-04-26-mockbase-snapshot-only-reads.md] — 间接：MockKeychainStore stub 字段不暴露存储；与 MockBase 内部 private 模式对齐
- [Source: docs/lessons/2026-04-26-modal-overlay-content-shield.md] — 间接：SwiftUI 原生 .alert 自带屏蔽，不需要自定 overlay
- [Source: docs/lessons/2026-04-26-fullscreencover-isolated-environment.md] — 间接：本 story alert 不走 sheet 子树
- [Source: docs/lessons/2026-04-26-published-publisher-vs-objectwillchange.md] / [docs/lessons/2026-04-26-objectwillchange-no-initial-emit.md] — 间接：本 story 测试不观察多次 publisher 变化，不踩坑

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

无（实装一次过；build 与 test 全部一次性 BUILD SUCCESS / TEST SUCCEEDED）。

### Completion Notes List

- AC1 ✅ `KeychainStoreProtocol` + `InMemoryKeychainStore` 占位实装落地（`iphone/PetApp/Core/Storage/KeychainStore.swift`）。文件头注释明示"Story 5.1 替换为 KeychainServicesStore"。
- AC2 ✅ `ResetKeychainUseCaseProtocol` + `DefaultResetKeychainUseCase` struct 落地（`async throws` + `Sendable`）。
- AC3 ✅ `ResetIdentityViewModel` `@MainActor` ObservableObject 落地，显式 `import Combine`；`ResetIdentityAlertContent` enum 定义在同 file（success / failure(message:)）。
- AC4 ✅ `ResetIdentityButton` SwiftUI View + `Identifiable` extension 全部 `#if DEBUG` 包裹；alert 文案严格对齐 epics.md AC（"已重置 / 请杀进程后重新启动 App 模拟首次安装"）。
- AC5 ✅ `HomeView.userInfoBar` 内 `#if DEBUG` 加按钮；`accessibilityElement(children:)` 从 `.ignore` 改为 `.contain`；保留旧 `init(viewModel:)` 同时新增 `init(viewModel:resetIdentityViewModel:)` —— 旧调用方零改动；用 plain `let` 而非 `@ObservedObject` 持有 optional ViewModel（@ObservedObject 不接 Optional；按钮子 view 自身订阅 ViewModel）。
- AC6 ✅ `AppContainer` 加 `keychainStore: KeychainStoreProtocol` 字段（默认 `InMemoryKeychainStore()`）+ `makeResetKeychainUseCase()` + `#if DEBUG makeResetIdentityViewModel() #endif`；保留 Story 2.5 所有既有 API。
- AC7 ✅ `RootView` `#if DEBUG @StateObject private var resetIdentityViewModel`；`init()` 内用临时 standalone `AppContainer()` 喂 `_resetIdentityViewModel = StateObject(wrappedValue: ...)`（lesson 2026-04-26-stateobject-init-vs-bind-injection.md 应用）；`body` 提取 `homeView` @ViewBuilder 包装 `#if DEBUG / #else` 双 init 分支。
- AC8 ✅ `AccessibilityID.Home` 末尾追加 `btnResetIdentity` + `resetIdentityAlert`。
- AC9 ✅ 4 个测试 file 落地，所有 file 顶层 `#if DEBUG`：
  - `MockKeychainStore.swift`（继承 MockBase + 4 stub 字段）
  - `InMemoryKeychainStoreTests.swift`（7 case）
  - `ResetKeychainUseCaseTests.swift`（3 case）
  - `ResetIdentityViewModelTests.swift`（5 case）
- AC10 ✅ `HomeUITests.swift` 追加 `testResetIdentityButtonVisibleAndAlertOnTap`（XCUITest 默认 Debug build，按钮可定位 + alert "已重置" + OK 按钮链路），9.279s 通过。
- AC11 ✅ build 验证：
  - `bash iphone/scripts/build.sh` → BUILD SUCCESS
  - `bash iphone/scripts/build.sh --test` → BUILD SUCCESS + **108 tests, 0 failures** in 1.209s（90 既有 + 18 新增：7 InMemoryKeychainStore + 3 ResetKeychainUseCase + 5 ResetIdentityViewModel + 3 = 18）
  - `bash iphone/scripts/build.sh --uitest` → BUILD SUCCESS + **5 UI tests, 0 failures** in 49.088s（含新增 `testResetIdentityButtonVisibleAndAlertOnTap` 9.279s）
- AC12 ✅ 既有测试零回归：AppContainerTests 6 case 全绿（含 ATS / errorPresenter singleton / validatedBaseURL 等敏感 case）；HomeViewTests / HomeViewModelTests / HomeViewModelPingTests / SampleViewModelTests 全绿。
- AC13 ✅ `git status` 文件清单严格匹配；`git diff ios/ server/ iphone/project.yml iphone/scripts/ .gitignore` 全部空 diff。

**Lessons learned 应用清单**（来自任务上下文）：
1. ObservableObject / @Published 显式 `import Combine` —— `ResetIdentityViewModel.swift` / `ResetIdentityViewModelTests.swift` 均显式 import。
2. SwiftUI 父容器 a11y identifier 默认传播覆盖子元素 —— `userInfoBar` 用 `.contain` 而非 `.ignore`，让子按钮 identifier 可定位。
3. @StateObject 不能 lazy 初始化 —— `RootView.init()` 用 `_resetIdentityViewModel = StateObject(wrappedValue: ...)` 在 init 阶段给值。
4. MockBase snapshot-only reads —— `MockKeychainStore` stub 字段是 setUp/Test method body 内单线程读写；MockBase 内部 invocations 走 `*Snapshot()` 接口。
5. assertThrowsAsyncError helper 用法 —— `ResetKeychainUseCaseTests.testExecutePropagatesError` 直接调。
6. Release build fail-closed —— `ResetIdentityButton` / `ResetIdentityAlertContent: Identifiable` extension / `makeResetIdentityViewModel()` / `RootView.resetIdentityViewModel` 字段及 init 全部 `#if DEBUG` 包裹；调用方（HomeView userInfoBar / RootView body）也对应 `#if DEBUG`。

### File List

**新增**：
- `iphone/PetApp/Core/Storage/KeychainStore.swift`
- `iphone/PetApp/Features/DevTools/Views/ResetIdentityButton.swift`
- `iphone/PetApp/Features/DevTools/UseCases/ResetKeychainUseCase.swift`
- `iphone/PetApp/Features/DevTools/ViewModels/ResetIdentityViewModel.swift`
- `iphone/PetAppTests/Features/DevTools/MockKeychainStore.swift`
- `iphone/PetAppTests/Features/DevTools/InMemoryKeychainStoreTests.swift`
- `iphone/PetAppTests/Features/DevTools/ResetKeychainUseCaseTests.swift`
- `iphone/PetAppTests/Features/DevTools/ResetIdentityViewModelTests.swift`

**修改**：
- `iphone/PetApp/App/AppContainer.swift`（追加 `keychainStore` 字段 + `init(apiClient:keychainStore:)` 默认参数 + `makeResetKeychainUseCase()` + `#if DEBUG makeResetIdentityViewModel() #endif`）
- `iphone/PetApp/App/RootView.swift`（`#if DEBUG` 加 `@StateObject resetIdentityViewModel` + `init()` 注入 + `homeView` @ViewBuilder 双分支）
- `iphone/PetApp/Features/Home/Views/HomeView.swift`（新 init 重载 `init(viewModel:resetIdentityViewModel:)` + `userInfoBar` 内 `#if DEBUG` 加按钮 + `.contain`）
- `iphone/PetApp/Shared/Constants/AccessibilityID.swift`（追加 `btnResetIdentity` + `resetIdentityAlert`）
- `iphone/PetAppUITests/HomeUITests.swift`（追加 `testResetIdentityButtonVisibleAndAlertOnTap`）
- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen regen 副作用，预期）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（`2-8-dev-重置-keychain-按钮: ready-for-dev → in-progress → review`）
- `_bmad-output/implementation-artifacts/2-8-dev-重置-keychain-按钮.md`（本 story 文件：Tasks/Subtasks 全部勾选 + Dev Agent Record + File List + Change Log + Status review）

## Change Log

| 日期 | 版本 | 描述 | 作者 |
|---|---|---|---|
| 2026-04-25 | 0.1 | 初稿（ready-for-dev）；Ultimate context engine analysis：iPhone 端 dev "重置身份" 按钮 + KeychainStore 占位接缝 + ResetKeychainUseCase + 单闸门 #if DEBUG gate | SM |
| 2026-04-26 | 0.2 | dev-story 实装一次过：8 新文件 + 5 修改文件；108 unit tests + 5 UI tests 全绿；既有测试零回归；状态 → review | Dev |
