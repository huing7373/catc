# Story 2.5: ping 调用 + 主界面显示 server /version 信息

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iOS 开发,
I want App 启动后自动调用 server 的 `GET /ping` + `GET /version` 接口，把 `App v<bundle短版本> · Server <commit短哈希>` 显示在主界面右下角的版本号位,
so that demo / 联调时一眼能看到 client / server 是否在线 + 各自的版本，且为节点 1 的 demo 验收（Epic 3 / Story 3.2）准备好"端到端真实联调"的最小链路。

## 故事定位（Epic 2 第四条实装 story；首条"真实跨端调用"）

这是 Epic 2 内**第一条**真实把 server 接入的 story。前置条件全部 done：

- **Story 2.2 (`done`)** 落地了 `HomeView` + `HomeViewModel`，`@Published var appVersion: String = "0.0.0"` / `@Published var serverInfo: String = "----"` 以 hardcode 方式占位，`HomeView.versionLabel` 渲染 `"v\(viewModel.appVersion) · \(viewModel.serverInfo)"`，accessibility identifier `AccessibilityID.Home.versionLabel = "home_versionLabel"` 已存在（**不变**）。
- **Story 2.3 (`done`)** 落地了 `RootView` 持有 `@StateObject private var homeViewModel = HomeViewModel()`，并通过 `.onAppear { wireHomeViewModelClosures() }` 完成 CTA 闭包 wire。
- **Story 2.4 (`done`)** 落地了 `APIClient` / `APIClientProtocol` / `APIError` / `Endpoint` / `APIResponse<T>` / `URLSessionProtocol`，单测 + 集成测试共 10 case 全绿。

**本 story 的核心动作**：

1. 新建 `PetApp/Features/Home/UseCases/PingUseCase.swift`（**首个 UseCase**，对齐 iOS 架构 §5.3）。`PingUseCase` 调 `APIClient` 一次性拉 `/ping` + `/version` 两个端点，组装成单一 result（`PingResult { isReachable: Bool, serverCommit: String? }`）返回。
2. 扩展 `HomeViewModel`：保留 `appVersion` / `serverInfo` 两个 `@Published` 字段（**不**改字段名 / **不**改字段类型），**新增**注入 `PingUseCase` 的 init 重载 + `start()` 方法（在 RootView `.task` 调用），把 hardcode 替换为运行时 PingUseCase 驱动。
3. 新建 `PetApp/App/AppContainer.swift`（**首次落地**依赖注入容器，对齐 iOS 架构 §18.2 + ADR-0002 §3.3）。`AppContainer` 在 init 时构造 `APIClient` + 各 UseCase；`RootView` 通过 `@StateObject` 持有 container 实例并把 `pingUseCase` 注入给 `HomeViewModel`。
4. `RootView` 的 `homeViewModel` 改为按需构造，并新增 `.task { await homeViewModel.start() }` 触发首次 ping；保持现有 `.onAppear { wireHomeViewModelClosures() }` 不变（仅追加 `.task`，不替换 `.onAppear`）。
5. `HomeView` 现有 `versionLabel` 字符串模板（`"v\(viewModel.appVersion) · \(viewModel.serverInfo)"`）**不动**，只是 `viewModel.appVersion` / `viewModel.serverInfo` 在 ping 完成后被 PingUseCase 推上来的真实值替换。

**不涉及**：
- **真实 server 联调（→ Epic 3 Story 3.1 / 3.2）**：本 story 单测 / 集成测试**不**真启动 server，全部用 `MockAPIClient` / `StubURLProtocol` 拦截。Demo 场景下 dev 手动起 server + Simulator 跑 App 验证（不在自动化范围）。
- **token interceptor（→ Epic 5 Story 5.3）**：ping / version 不需鉴权（`requiresAuth: false`）；本 story 不动 `AuthInterceptor.swift`（仍**不创建**该文件）。
- **错误 UI（→ Story 2.6）**：本 story 出错时 `serverInfo` 显示 `"offline"`（部分降级显示 `"v?"`），**不**弹 Toast / Alert / RetryView。
- **App build hash 注入（编译期 ldflags 类机制）**：iOS 端不走 ldflags，而是从 `Bundle.main.infoDictionary?["CFBundleShortVersionString"]` 读 plist 内的 `CFBundleShortVersionString`（项目目前未在 Info.plist 显式声明，需在本 story 加上）。
- **节点 2+ 业务接口（auth / home 等）**：本 story 只动 `/ping` + `/version` 两个运维端点；业务 endpoint 由 Epic 4+ stories 落地。
- **AppLaunchState（→ Story 2.9）** / **LaunchingView（→ Story 2.9）** / **GuestLoginUseCase（→ Epic 5）**：本 story 不引入 launch 状态机；ping 失败仍直接显示 `HomeView`，只是 versionLabel 文案变。
- **WebSocket（→ Epic 10 / 12）**：V1接口设计 §12 的 `ping` / `pong` 是 WS 心跳消息，与本 story 的 REST `/ping` 同名但**不同协议**，不要混淆。

**范围红线**：

- **不动 `ios/`**：本 story 绝对不修改 `ios/` 任何文件（CLAUDE.md "Repo Separation" + ADR-0002 §3.3 + Story 2.2 / 2.3 / 2.4 既有约束的延续）。最终 `git status` 须确认 `ios/` 下零改动。
- **不动 `server/`**：本 story 是 iOS 端 wire / 集成层落地，不涉及 server 任何文件（**包括 `_bmad-output/implementation-artifacts/sprint-status.yaml` 之外的任何 server-related 文件**）。Server 的 `/ping` / `/version` 实装在 Epic 1 已 done（参见 Story 1.2 / 1.4），本 story 视它为契约 fixed。
- **不修改 Story 2.4 已落地的 `APIClient` / `APIError` / `Endpoint` / `APIResponse` / `URLSessionProtocol` 5 个 production 文件**（5d97a74 + 2b0449a 两个 fix-review commit 后已稳定）。如有微调（如新增 `Endpoint` 工厂方法）只能**追加**到新文件，不动原文件主体。
- **不修改 Story 2.4 测试文件**（`APIClientTests` / `MockURLSession` / `APIClientIntegrationTests` / `StubURLProtocol`）的现有 case；可在本 story 测试中**复用** `MockURLSession` 类（手写 mock 标准模式 ADR-0002 §3.1）。
- **不修改 `AccessibilityID.Home.*` 任何老常量字符串值**（特别是 `versionLabel = "home_versionLabel"`）。
- **不引入 Combine 响应式框架**：`HomeViewModel` 仍用 `ObservableObject + @Published`（Story 2.2 / 2.3 同风格）；`PingUseCase` 用 `async throws`（与 ADR-0002 §3.2 + 架构 §18.1 一致）。
- **不引入第三方依赖注入框架**（如 Swinject / Resolver）：`AppContainer` 用纯 Swift class + 手动 wire，与 iOS 架构 §18.2 "通过 AppContainer 管理"对齐。
- **不引入 Combine `Publisher` 测试 helper**：测试 `@Published` 状态变化用 `await fulfillment(of:)` + 显式 expectation，或直接 `await viewModel.start()` + 同步断言（async/await 主路径）。

## Acceptance Criteria

**AC1 — `PingResult` 模型**

新建 `iphone/PetApp/Features/Home/UseCases/PingResult.swift`（与 `PingUseCase.swift` 同目录），定义 UseCase 输出的 value type：

```swift
import Foundation

/// PingUseCase 的输出。
///
/// 三态语义（按本 story Dev Note #2 决策表）：
/// - `(reachable: true, serverCommit: "abc1234")`  ping 成功 + version 成功 → 显示 "Server abc1234"
/// - `(reachable: true, serverCommit: nil)`        ping 成功 + version 失败 → 显示 "Server v?"（部分降级）
/// - `(reachable: false, serverCommit: nil)`       ping 失败                → 显示 "Server offline"
///
/// 不引入第四种 `(reachable: false, serverCommit: "...")`：ping 失败时已经认为整个 server 不可达，
/// 即使理论上 version 调用先成功后被 ping 推翻，UI 上"server offline"语义优先级更高，commit 值无意义。
public struct PingResult: Equatable {
    /// 整体可达性：ping 成功 = true；ping 失败 = false。
    public let reachable: Bool
    /// version 接口返回的 commit 短哈希；version 失败 / ping 失败时为 nil。
    public let serverCommit: String?

    public init(reachable: Bool, serverCommit: String?) {
        self.reachable = reachable
        self.serverCommit = serverCommit
    }
}
```

**关键约束**：
- 用 `struct`（value type），不用 `class`；`Equatable` 便于测试断言。
- 字段命名：`reachable` 而非 `isReachable`（V1 风格统一：避免 `is` 前缀污染 SwiftUI / Combine binding 的命名空间）。
- 两字段 `let`：构造后不可变，PingUseCase 一次性返回完整 result。
- **不持有** `versionBuiltAt` / `versionMessage` 等额外字段：UI 只展示 commit，不展示 builtAt（节点 1 demo 不需要；如未来要扩展，**追加**字段即可）。
- 顶部仅 `import Foundation`。

**AC2 — `PingEndpoint` / `VersionEndpoint` 工厂**

新建 `iphone/PetApp/Features/Home/UseCases/PingEndpoints.swift`，集中定义两个 Endpoint 构造：

```swift
import Foundation

/// `/ping` 与 `/version` 的 Endpoint 工厂。
///
/// 这两个端点是**运维 / 探活端点**，server 注册在根路径（**不**走 `/api/v1` 前缀）：
/// - GET /ping     → envelope.data = {} (空对象)，envelope.message = "pong"
/// - GET /version  → envelope.data = {commit: String, builtAt: String}
///
/// 详情见 server `internal/app/bootstrap/router.go:45-46` 与 server story 1.2 / 1.4。
///
/// 关于 baseURL 的拼接约定：本 story 的 APIClient 用 host-only baseURL（如 `http://localhost:8080`，
/// **不**含 `/api/v1` 前缀），endpoint.path 自带完整路径（`/ping` / `/version`）。
/// 后续 Epic 4+ 的业务 endpoint 同样自带 `/api/v1/...` 前缀。
/// 这与 Story 2.4 doc comment 中"baseURL 含 `/api/v1`"的早期假设有出入；本 story Dev Note #1 解释最终选型理由。
public enum PingEndpoints {
    public static func ping() -> Endpoint {
        Endpoint(path: "/ping", method: .get, body: nil, requiresAuth: false)
    }

    public static func version() -> Endpoint {
        Endpoint(path: "/version", method: .get, body: nil, requiresAuth: false)
    }
}

/// `/ping` 响应的 data 解码模型。data 是空对象 `{}`，用 `Empty` 占位。
public typealias PingResponse = Empty

/// `/version` 响应的 data 解码模型，对齐 server 端 `VersionResponse` struct（小驼峰，见
/// server/internal/app/http/handler/version_handler.go）。
public struct VersionResponse: Decodable, Equatable {
    public let commit: String
    public let builtAt: String

    public init(commit: String, builtAt: String) {
        self.commit = commit
        self.builtAt = builtAt
    }
}
```

**关键约束**：
- `path` 严格用 `"/ping"` / `"/version"`（**不**加 `/api/v1` 前缀），server 端实测就是这两个路径（router.go:45-46）。
- `requiresAuth: false`：两个端点无 token 要求（Story 1.2 / 1.4 都是**无鉴权 + 无中间件 auth 拦截**）。
- `VersionResponse.commit` / `builtAt` 字段名严格对齐 server 端 `VersionResponse` Go struct 的 JSON tag（`json:"commit"` / `json:"builtAt"` 见 version_handler.go:13-14）；偏差任何一个字段名都会让客户端解码 fail。
- `PingResponse = Empty`：复用 Story 2.4 落地的 `Empty: Decodable` 占位类型（避免重定义"空 data"模型）。
- 顶部仅 `import Foundation`（`Endpoint` / `Empty` 都在 PetApp module 内 public 可用，无需额外 import）。

**AC3 — `PingUseCase` 协议 + 默认实现**

新建 `iphone/PetApp/Features/Home/UseCases/PingUseCase.swift`：

```swift
import Foundation

/// `PingUseCase` 协议：负责一次性拉 `/ping` + `/version` 并组装成 `PingResult`。
///
/// 三态语义：
/// - ping OK + version OK → `(reachable: true, serverCommit: <非空>)`
/// - ping OK + version 失败 → `(reachable: true, serverCommit: nil)`（部分降级）
/// - ping 失败 → `(reachable: false, serverCommit: nil)`（version 调用被短路跳过）
///
/// 注意：`execute()` **永远不抛错**——任何失败都被映射成 `PingResult` 的某个负态。这是有意为之：
/// 1. 主界面 versionLabel 是装饰性元素，错误不应阻断渲染（区别于 GuestLoginUseCase / LoadHomeUseCase 等关键 UseCase）。
/// 2. 调用方（HomeViewModel）只关心三态文案，不关心是 401 / 网络 / 解码哪种失败（→ Story 2.6 ErrorPresenter 才关心错误类型）。
///
/// 不抛错并不意味着"吞错": 实装中会把底层 `APIError` 透传给 logger（Story 2.7 落地后接入）；
/// 当前 MVP 阶段 logger 还没接，先不在 UseCase 里 print，避免遗留 print 到 commit。
public protocol PingUseCaseProtocol: Sendable {
    func execute() async -> PingResult
}

/// 默认实现：注入 `APIClientProtocol`，按 happy / partial-degrade / offline 三态返回。
public struct DefaultPingUseCase: PingUseCaseProtocol {
    private let client: APIClientProtocol

    public init(client: APIClientProtocol) {
        self.client = client
    }

    public func execute() async -> PingResult {
        // 步骤 1：先调 ping。失败立即返回 offline。
        do {
            let _: PingResponse = try await client.request(PingEndpoints.ping())
        } catch {
            return PingResult(reachable: false, serverCommit: nil)
        }

        // 步骤 2：ping OK，再调 version。失败时 reachable 仍 true（部分降级）。
        do {
            let v: VersionResponse = try await client.request(PingEndpoints.version())
            return PingResult(reachable: true, serverCommit: v.commit)
        } catch {
            return PingResult(reachable: true, serverCommit: nil)
        }
    }
}
```

**关键约束**：
- `protocol PingUseCaseProtocol: Sendable`：让 `HomeViewModel` 可以注入 mock 而无需依赖具体实现；`Sendable` 与 Story 2.4 `APIClientProtocol` 风格一致。
- `execute()` 签名 `async -> PingResult`（**不** throws）：见上文 doc comment 解释。
- **顺序串行**：先 ping，ping 失败立即短路；不要并行（ping 失败时 version 调用浪费）。这与"reachable=true + serverCommit=非空"的因果链对齐。
- 用 `struct DefaultPingUseCase` 而非 `class`：UseCase 是无状态的纯组合层，value type 最贴切（与架构 §5.3 "对一个明确业务动作进行封装"语义相符）。
- 顶部仅 `import Foundation`。
- **不**写日志：MVP 阶段 logger 框架未落地（→ Story 2.7）；当前 catch 块里直接 swallow error，等 logger 接入后改为 `logger.warning("ping failed: \(error)")`。这条**作为本 story 的 follow-up TODO 留给 Story 2.7**。

**AC4 — `HomeViewModel` 扩展（注入 PingUseCase + start()）**

修改 `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`，**追加**以下能力（**不删除**任何现有字段 / closure / init 参数）：

```swift
import Foundation
import Combine

@MainActor
public final class HomeViewModel: ObservableObject {
    @Published public var nickname: String
    @Published public var appVersion: String
    @Published public var serverInfo: String

    public var onRoomTap: () -> Void
    public var onInventoryTap: () -> Void
    public var onComposeTap: () -> Void

    /// 注入的 PingUseCase；**可选**：保留 nil 让现有 Story 2.2 / 2.3 测试 + Preview 用 default init 继续工作。
    /// RootView 在 production 路径里通过 AppContainer 注入非 nil 实例。
    private let pingUseCase: PingUseCaseProtocol?

    /// 当前是否有进行中的 ping 任务。再次调用 start() 时短路（防 SwiftUI .task 多次触发引发的重复请求）。
    private var pingTask: Task<Void, Never>?

    /// 现有 init（Story 2.2 / 2.3 路径）：保留 hardcode 默认值，pingUseCase = nil；不破坏老调用方 / Preview。
    public init(
        nickname: String = "用户1001",
        appVersion: String = "0.0.0",
        serverInfo: String = "----",
        onRoomTap: @escaping () -> Void = {},
        onInventoryTap: @escaping () -> Void = {},
        onComposeTap: @escaping () -> Void = {}
    ) {
        self.nickname = nickname
        self.appVersion = appVersion
        self.serverInfo = serverInfo
        self.onRoomTap = onRoomTap
        self.onInventoryTap = onInventoryTap
        self.onComposeTap = onComposeTap
        self.pingUseCase = nil
    }

    /// Story 2.5 新增 init：注入 `PingUseCaseProtocol`，appVersion 默认从 Bundle 读取。
    /// 在 AppContainer wire 时调用此 init；测试也用此 init 注入 mock UseCase。
    public init(
        nickname: String = "用户1001",
        pingUseCase: PingUseCaseProtocol,
        appVersion: String = HomeViewModel.readAppVersion(),
        serverInfo: String = "----",
        onRoomTap: @escaping () -> Void = {},
        onInventoryTap: @escaping () -> Void = {},
        onComposeTap: @escaping () -> Void = {}
    ) {
        self.nickname = nickname
        self.appVersion = appVersion
        self.serverInfo = serverInfo
        self.onRoomTap = onRoomTap
        self.onInventoryTap = onInventoryTap
        self.onComposeTap = onComposeTap
        self.pingUseCase = pingUseCase
    }

    /// 触发 ping。重复调用时短路（pingTask 非 nil → 直接 return）。
    /// RootView 在 .task { await viewModel.start() } 中调用，App 启动时执行一次。
    /// SwiftUI .task 闭包 在 view 重新出现时可能多次触发 —— 短路保证不会发起重复请求。
    public func start() async {
        guard let pingUseCase = pingUseCase else { return }   // 老路径（无注入）：no-op
        guard pingTask == nil else { return }                  // 已有进行中任务：短路

        let task = Task { [weak self] in
            let result = await pingUseCase.execute()
            await self?.applyPingResult(result)
        }
        pingTask = task
        await task.value
        pingTask = nil
    }

    /// 把 PingResult 投影成 serverInfo 文案。
    /// 三态文案（与 AC5 主界面渲染对应）：
    /// - reachable=true + commit 非空：serverInfo = commit 短哈希
    /// - reachable=true + commit nil：serverInfo = "v?"
    /// - reachable=false：serverInfo = "offline"
    private func applyPingResult(_ result: PingResult) {
        if !result.reachable {
            self.serverInfo = "offline"
        } else if let commit = result.serverCommit, !commit.isEmpty {
            self.serverInfo = commit
        } else {
            self.serverInfo = "v?"
        }
    }

    /// 从 main bundle 读 CFBundleShortVersionString；缺省 "0.0.0"（与现有 hardcode 默认一致）。
    /// nonisolated 让它能在 init 默认参数位置安全调用（init 还没进 MainActor）。
    public nonisolated static func readAppVersion() -> String {
        (Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String) ?? "0.0.0"
    }
}
```

**关键约束**：
- **保留旧 init**（参数与 Story 2.2 一致，`pingUseCase = nil`）：让 `HomeViewModel()` 在 Preview / 老测试中继续可用，不破坏 `HomeViewModelTests.testHardcodedDefaultStateMatchesStorySpec` 等老 case。
- **新增 init**（带 `pingUseCase` 必填参数）：production 路径 / 本 story 新单测都走此 init。
- `serverInfo` **默认值仍是 `"----"`**：在 ping 完成前主界面继续显示老占位（不引入额外 loading 文案，避免视觉跳动）。
- `pingTask: Task<Void, Never>?` 短路：防 `.task` 在 SwiftUI 视图刷新时多次触发引发并发 ping；epics.md AC 列的 "重复触发不会发起重复请求" 由此满足。
- `applyPingResult` 三态决策：与 AC1 `PingResult` 三态一一对应；**不**在 ViewModel 决定 UI 字符串前缀（"v" / "Server"）—— 那由 `HomeView.versionLabel` 模板决定。
- `commit.isEmpty` 兜底：server 理论上不会返回空字符串（version_handler.go 默认 `"unknown"`），但 dev / test 环境若 ldflags 注入失败传来空字符串，按"version 失败"处理。
- `nonisolated static func readAppVersion()`：让它能在 `MainActor` init 默认参数位置安全调用（默认参数表达式在 actor isolation 之外评估）。
- `import Combine`（继承 Story 2.2 lesson `2026-04-25-swift-explicit-import-combine.md`）：`@Published` / `ObservableObject` 必须显式 import。**老文件已经 import Combine**，本 story 改动不删此行。

**AC5 — `HomeView.versionLabel` 文案模板（**不变**）**

`HomeView.swift` 的 `versionLabel`：

```swift
private var versionLabel: some View {
    Text("v\(viewModel.appVersion) · \(viewModel.serverInfo)")
        .font(.caption)
        .foregroundStyle(.secondary)
        .accessibilityIdentifier(AccessibilityID.Home.versionLabel)
}
```

**保持不变**：模板字符串、字体、accessibility identifier 全部沿用 Story 2.2 已落地的实现。

最终主界面三种渲染结果：

| ViewModel 状态 | 渲染文案 | epics.md AC 对应 |
|---|---|---|
| appVersion="1.0.0", serverInfo="abc1234" | `v1.0.0 · abc1234` | "App v<App build hash> · Server <server commit>" |
| appVersion="1.0.0", serverInfo="offline" | `v1.0.0 · offline` | "App v<...> · Server offline" |
| appVersion="1.0.0", serverInfo="v?" | `v1.0.0 · v?` | "Server v?"（部分降级） |
| appVersion="0.0.0", serverInfo="----"（ping 启动前） | `v0.0.0 · ----` | Story 2.2 hardcode 默认（保留作为初始态） |

**关键约束**：
- 不改 `HomeView.swift` 任何代码——文案变化通过 `viewModel.serverInfo` 的 `@Published` 推送实现。
- 不在 `HomeView` 里写"v?" / "offline" 字面量——这些字符串归 `HomeViewModel.applyPingResult` 决策。
- AC 红线：**`AccessibilityID.Home.versionLabel = "home_versionLabel"` 字符串值不变**（防 UITest `HomeUITests.testHomeViewShowsAllSixPlaceholders` 之类的回归）。

**AC6 — `AppContainer`（首次落地依赖注入容器）**

新建 `iphone/PetApp/App/AppContainer.swift`：

```swift
import Foundation

/// App 全局依赖注入容器。
///
/// 职责（本 story 范围）：
/// - 持有 `APIClient` 单例（baseURL 由 init 时注入）
/// - 暴露按需构造 UseCase 的工厂方法（如 `makePingUseCase()`）
///
/// 生命周期：由 `RootView` 通过 `@StateObject private var container = AppContainer()` 持有，
/// 与 App scene 同生命周期。**当前 MVP 不引入 SceneStorage / AppDelegate 钩子**，container 重启 = App 重启。
///
/// 后续扩展（→ Epic 4 / 5 / 12+）：
/// - 按需追加 `KeychainStore` / `SessionRepository` / `WebSocketClient` 等
/// - 业务 UseCase（GuestLoginUseCase / LoadHomeUseCase / OpenChestUseCase 等）按 Repository → UseCase
///   分层在此 wire；此 story 只 wire PingUseCase，作为模板示范
///
/// 不引入第三方 DI 框架（Swinject / Resolver）：
/// MVP 阶段 wire 量小（< 20 个对象），手写 init / factory method 就够，避免 DSL 学习成本（与 ADR-0002 §3.1
/// "手写 mock 优于 codegen" 的精神同源）。
@MainActor
public final class AppContainer: ObservableObject {
    public let apiClient: APIClientProtocol

    /// 默认 init：用 `APIClient(baseURL:)` 构造默认 client。
    /// baseURL 默认 `http://localhost:8080`（local dev 默认，与 server `cmd/server` 默认 listen 端口对齐）。
    /// 测试 / 未来环境切换通过 `init(apiClient:)` 重载注入自定义 client。
    public convenience init() {
        let baseURL = URL(string: "http://localhost:8080")!
        self.init(apiClient: APIClient(baseURL: baseURL))
    }

    /// 注入式 init：测试中传 mock APIClient；未来 release build 切到 production baseURL 时也走此入口。
    public init(apiClient: APIClientProtocol) {
        self.apiClient = apiClient
    }

    /// 工厂：构造 PingUseCase。每次调用返回新实例（UseCase 是 value type，构造廉价）。
    public func makePingUseCase() -> PingUseCaseProtocol {
        DefaultPingUseCase(client: apiClient)
    }
}
```

**关键约束**：
- `AppContainer: ObservableObject`：让 `RootView` 可以用 `@StateObject` 持有；**不**暴露任何 `@Published` 字段（容器本身状态稳定，不参与 SwiftUI 重新渲染）。
- `apiClient: APIClientProtocol` 用协议类型而非具体 class：测试场景下注入 `MockAPIClient` 即可。
- `convenience init()`：local dev 默认 `http://localhost:8080`（**不**含 `/api/v1` —— 见 Dev Note #1 baseURL 决策）；production 切换通过 `init(apiClient:)` 注入自定义 client（节点 1 阶段尚未需要 staging / prod env 切换，**不**引入配置文件机制）。
- `makePingUseCase()` 返回 protocol 类型：让 `HomeViewModel.init(pingUseCase:)` 接收 protocol 即可，与单测 mock 路径对称。
- 顶部仅 `import Foundation`（`ObservableObject` 在 Foundation 通过 transitive import 可用 —— 但**遵守 Story 2.2 lesson**，仍要显式 `import Combine`）。**修正**：本文件需要 `import Foundation` + `import Combine`（`ObservableObject` 来自 Combine）。
- `@MainActor`：与 `HomeViewModel` / `AppCoordinator` 一致，全部在主线程读写。
- **不**做 lazy init / 单例锁定（如 `static let shared`）：让 `RootView` 通过 `@StateObject` 显式持有，符合 SwiftUI 数据流 conventions。

**AC7 — `RootView` wire 改造**

修改 `iphone/PetApp/App/RootView.swift`：

```swift
import SwiftUI

struct RootView: View {
    @StateObject private var coordinator = AppCoordinator()
    @StateObject private var container = AppContainer()
    @StateObject private var homeViewModel: HomeViewModel

    init() {
        // 在 init 阶段 container 还未真正初始化（@StateObject wrapper 的 wrappedValue 在
        // body 第一次求值时才稳定），所以这里先用占位 default value 构造 homeViewModel；
        // 真正的 PingUseCase 注入推迟到 body / .task 中。
        // 但 SwiftUI 的 @StateObject 不允许在 init 里赋值非 default 实例（会被运行时丢弃），
        // 所以采用如下模式：homeViewModel 用默认 init（hardcode pingUseCase=nil），
        // 在 .task 中替换成 container.makePingUseCase() 驱动的真实路径。
        _homeViewModel = StateObject(wrappedValue: HomeViewModel())
    }

    var body: some View {
        HomeView(viewModel: homeViewModel)
            .onAppear {
                wireHomeViewModelClosures()
            }
            .task {
                // App 启动后第一次显示 HomeView 时触发 ping。
                // 注意：homeViewModel 的 pingUseCase 在初始 init 中是 nil（@StateObject 限制）；
                // 在此处通过 container 拿真正的 UseCase 后调 startPing 驱动。
                // 详见 Dev Note #4 "@StateObject 与 init 注入" 的取舍说明。
                let pingUseCase = container.makePingUseCase()
                await homeViewModel.startWithPingUseCase(pingUseCase)
            }
            .fullScreenCover(item: $coordinator.presentedSheet) { sheet in
                sheetContent(for: sheet)
            }
    }

    private func wireHomeViewModelClosures() {
        homeViewModel.onRoomTap = { [coordinator] in
            coordinator.present(.room)
        }
        homeViewModel.onInventoryTap = { [coordinator] in
            coordinator.present(.inventory)
        }
        homeViewModel.onComposeTap = { [coordinator] in
            coordinator.present(.compose)
        }
    }

    @ViewBuilder
    private func sheetContent(for sheet: SheetType) -> some View {
        switch sheet {
        case .room:
            RoomPlaceholderView(onClose: { coordinator.dismiss() })
        case .inventory:
            InventoryPlaceholderView(onClose: { coordinator.dismiss() })
        case .compose:
            ComposePlaceholderView(onClose: { coordinator.dismiss() })
        }
    }
}
```

**注意：上面是一个有缺陷的范例**。详见 Dev Note #4 对 `@StateObject` + 注入的最终选型决策——本 story 选用**简化方案 B**：

**最终采用方案 B（dev 实装时按此落地）**：

不在 RootView init 创建带 PingUseCase 的 HomeViewModel；改为：
- `HomeViewModel` 新增 `func bind(pingUseCase:)` 方法（单次 setter，调多次只第一次生效），让外部在 `.task` 时把 use case 注入。
- `RootView.task` 调 `homeViewModel.bind(pingUseCase: container.makePingUseCase()); await homeViewModel.start()`。

此方案**不破坏** `@StateObject` 生命周期约束，**不需要**改 `HomeViewModel` 老的默认 init，**保持** Story 2.2 / 2.3 老测试 0 改动。

具体 `HomeViewModel.bind` 实装：

```swift
/// 单次绑定 PingUseCase。多次调用时仅第一次生效（后续静默 noop）。
/// 与 init(pingUseCase:) 互斥使用：测试场景用 init 注入，production 路径用 bind 注入。
public func bind(pingUseCase: PingUseCaseProtocol) {
    guard self.boundPingUseCase == nil else { return }
    self.boundPingUseCase = pingUseCase
}

private var boundPingUseCase: PingUseCaseProtocol?

public func start() async {
    let useCase = pingUseCase ?? boundPingUseCase
    guard let useCase = useCase else { return }
    guard pingTask == nil else { return }
    let task = Task { [weak self] in
        let result = await useCase.execute()
        await self?.applyPingResult(result)
    }
    pingTask = task
    await task.value
    pingTask = nil
}
```

`pingUseCase` 字段语义微调：让两条路径（init 注入 vs bind 注入）共用 `start()`，dev 实装时按上面这版落地。

**关键约束**：
- 现有 `wireHomeViewModelClosures()` + `.onAppear` 不改（Story 2.3 落地的 closure wire 路径），**追加** `.task { ... }` 驱动 ping。
- `.task` 在 view 出现时自动 launch + cancel：当 RootView 消失时自动 cancel ping task（PingUseCase 内部不抛 cancellation error 是 OK 的——`.task` 调 `.cancel()` 时 `URLSession.data(for:)` 会 throw `URLError(.cancelled)`，被 PingUseCase catch 后映射成 reachable=false）。
- `@StateObject private var container = AppContainer()`：默认 init 用 `http://localhost:8080`；production build 时若需要换 baseURL，**不**在 RootView 改，而是新建 `AppContainer.production()` factory（→ 节点 5+ 真上 staging 时再做，本 story 不预留）。
- `import SwiftUI` 顶部即可（已是 RootView 现状）；**不**新增 import Combine（@StateObject 来自 SwiftUI）。

**AC8 — Info.plist 加 `CFBundleShortVersionString`**

`iphone/project.yml` 当前 `info.properties` 里没有显式 `CFBundleShortVersionString`（XcodeGen 默认在生成的 Info.plist 里给 `1.0`，但**不在 project.yml 里显式写**会让 dev / 维护者看不到 source of truth）。

修改 `iphone/project.yml`：

```yaml
targets:
  PetApp:
    type: application
    platform: iOS
    sources:
      - PetApp
    info:
      path: PetApp/Resources/Info.plist
      properties:
        CFBundleDisplayName: PetApp
        CFBundleShortVersionString: "1.0.0"     # ← 本 story 新增
        CFBundleVersion: "1"                     # ← 本 story 新增（build number）
        UILaunchScreen: {}
        LSRequiresIPhoneOS: true
        UISupportedInterfaceOrientations:
          - UIInterfaceOrientationPortrait
        UIApplicationSceneManifest:
          UIApplicationSupportsMultipleScenes: false
```

**关键约束**：
- `CFBundleShortVersionString = "1.0.0"`：本 MVP 节点 1 的 marketing version；后续节点过验收时再 bump（节点 1 demo → 1.0.0；节点 2 demo → 1.1.0 或类似，由 Story 3.2 demo 规则决定）。
- `CFBundleVersion = "1"`：build number；CI 长期需要按 commit count 自动递增，**本 story 不接 CI**，固定 "1"。
- 修改 `project.yml` 后必须 `cd iphone && xcodegen generate` 让 `project.pbxproj` regen（与 Story 2.4 T8.1 同样惯例）。

**注意**：`iphone/project.yml` 改动是 Story 2.4 AC9 红线提到的"零改动 project.yml"的 **唯一例外**——本 story 因为需要从 Info.plist 读 `CFBundleShortVersionString`，必须显式声明。Story 2.4 的 5 个网络层文件依然零 project.yml 改动；本 story 的 5 个新文件（PingResult / PingEndpoints / PingUseCase / AppContainer / 测试文件）依然由 `sources: - PetApp` / `sources: - PetAppTests` 自动 glob，**新增的 yaml 改动只限于 `info.properties` 两条**。

**AC9 — 单元测试覆盖（≥ 4 case，按 epics.md AC + 扩展）**

新建 `iphone/PetAppTests/Features/Home/UseCases/PingUseCaseTests.swift`：

```swift
import XCTest
@testable import PetApp

@MainActor
final class PingUseCaseTests: XCTestCase {

    // MARK: - happy / partial-degrade / offline 三态

    /// case#1 (happy)：ping 成功 + version 成功 → reachable=true + commit 非空
    func testExecuteReturnsReachableWithCommitOnFullSuccess() async {
        let mock = MockAPIClient()
        mock.stubResponse[PingEndpoints.ping().path] = .success(Empty())
        mock.stubResponse[PingEndpoints.version().path] = .success(VersionResponse(commit: "abc1234", builtAt: "2026-04-26T08:00:00Z"))

        let useCase = DefaultPingUseCase(client: mock)
        let result = await useCase.execute()

        XCTAssertEqual(result, PingResult(reachable: true, serverCommit: "abc1234"))
    }

    /// case#2 (edge)：ping 成功 + version 失败 → reachable=true + commit nil（部分降级）
    func testExecuteReturnsReachableWithoutCommitWhenVersionFails() async {
        let mock = MockAPIClient()
        mock.stubResponse[PingEndpoints.ping().path] = .success(Empty())
        mock.stubResponse[PingEndpoints.version().path] = .failure(APIError.network(underlying: URLError(.timedOut)))

        let useCase = DefaultPingUseCase(client: mock)
        let result = await useCase.execute()

        XCTAssertEqual(result, PingResult(reachable: true, serverCommit: nil))
    }

    /// case#3 (edge)：ping 失败 → reachable=false + commit nil；version 调用被短路
    func testExecuteReturnsOfflineWhenPingFails() async {
        let mock = MockAPIClient()
        mock.stubResponse[PingEndpoints.ping().path] = .failure(APIError.network(underlying: URLError(.notConnectedToInternet)))
        // version stub 不设置：本 case 应该不被调到；如果 stub 缺失被命中会暴露问题（MockAPIClient 默认抛 stub-missing）

        let useCase = DefaultPingUseCase(client: mock)
        let result = await useCase.execute()

        XCTAssertEqual(result, PingResult(reachable: false, serverCommit: nil))
        XCTAssertEqual(mock.invocations.map(\.path), ["/ping"], "ping 失败后 version 不应被调用")
    }

    /// case#4 (edge)：ping 业务错误（envelope code != 0）→ 视为 ping 失败 → offline
    func testExecuteReturnsOfflineWhenPingThrowsBusinessError() async {
        let mock = MockAPIClient()
        mock.stubResponse[PingEndpoints.ping().path] = .failure(APIError.business(code: 1009, message: "服务繁忙", requestId: "req_x"))

        let useCase = DefaultPingUseCase(client: mock)
        let result = await useCase.execute()

        XCTAssertEqual(result, PingResult(reachable: false, serverCommit: nil))
    }

    /// case#5 (edge)：version 解码失败（如 server 返回字段名变动）→ 部分降级
    func testExecuteReturnsPartialDegradeWhenVersionDecodingFails() async {
        let mock = MockAPIClient()
        mock.stubResponse[PingEndpoints.ping().path] = .success(Empty())
        mock.stubResponse[PingEndpoints.version().path] = .failure(APIError.decoding(underlying: URLError(.cannotParseResponse)))

        let useCase = DefaultPingUseCase(client: mock)
        let result = await useCase.execute()

        XCTAssertEqual(result, PingResult(reachable: true, serverCommit: nil))
    }
}
```

新建 `iphone/PetAppTests/Features/Home/HomeViewModelPingTests.swift`：

```swift
import XCTest
@testable import PetApp

@MainActor
final class HomeViewModelPingTests: XCTestCase {

    /// case#1 (happy)：注入返回 reachable=true + commit 的 mock UseCase → start() 后 serverInfo == commit
    func testStartUpdatesServerInfoWithCommitOnSuccess() async {
        let stub = StubPingUseCase(stubResult: PingResult(reachable: true, serverCommit: "abc1234"))
        let viewModel = HomeViewModel(pingUseCase: stub)

        await viewModel.start()

        XCTAssertEqual(viewModel.serverInfo, "abc1234")
    }

    /// case#2 (edge)：reachable=false → serverInfo == "offline"
    func testStartUpdatesServerInfoToOfflineOnPingFailure() async {
        let stub = StubPingUseCase(stubResult: PingResult(reachable: false, serverCommit: nil))
        let viewModel = HomeViewModel(pingUseCase: stub)

        await viewModel.start()

        XCTAssertEqual(viewModel.serverInfo, "offline")
    }

    /// case#3 (edge)：reachable=true + commit nil → serverInfo == "v?"（部分降级）
    func testStartUpdatesServerInfoToVUnknownOnPartialDegrade() async {
        let stub = StubPingUseCase(stubResult: PingResult(reachable: true, serverCommit: nil))
        let viewModel = HomeViewModel(pingUseCase: stub)

        await viewModel.start()

        XCTAssertEqual(viewModel.serverInfo, "v?")
    }

    /// case#4 (happy)：重复调用 start() 不应触发重复请求
    func testStartIsIdempotentWhenCalledMultipleTimesConcurrently() async {
        let stub = StubPingUseCase(stubResult: PingResult(reachable: true, serverCommit: "abc1234"))
        let viewModel = HomeViewModel(pingUseCase: stub)

        // 并发触发两次 start；第一次跑完前第二次应短路
        async let first: Void = viewModel.start()
        async let second: Void = viewModel.start()
        _ = await (first, second)

        XCTAssertEqual(stub.executeCallCount, 1, "并发 start() 调用，UseCase.execute() 应只被调用 1 次")
        XCTAssertEqual(viewModel.serverInfo, "abc1234")
    }

    /// case#5 (edge)：未注入 pingUseCase（老路径） → start() 是 no-op，serverInfo 保持初始值
    func testStartIsNoOpWhenPingUseCaseNotInjected() async {
        let viewModel = HomeViewModel()  // 老 init，pingUseCase=nil

        await viewModel.start()

        XCTAssertEqual(viewModel.serverInfo, "----")
    }

    /// case#6 (happy)：bind(pingUseCase:) 后调 start() 应正常 ping
    func testBindThenStartUpdatesServerInfo() async {
        let stub = StubPingUseCase(stubResult: PingResult(reachable: true, serverCommit: "deadbee"))
        let viewModel = HomeViewModel()
        viewModel.bind(pingUseCase: stub)

        await viewModel.start()

        XCTAssertEqual(viewModel.serverInfo, "deadbee")
    }

    /// case#7 (edge)：bind 多次只第一次生效（防重复绑定）
    func testBindIsIdempotent() async {
        let stub1 = StubPingUseCase(stubResult: PingResult(reachable: true, serverCommit: "first"))
        let stub2 = StubPingUseCase(stubResult: PingResult(reachable: true, serverCommit: "second"))
        let viewModel = HomeViewModel()
        viewModel.bind(pingUseCase: stub1)
        viewModel.bind(pingUseCase: stub2)  // 应 no-op

        await viewModel.start()

        XCTAssertEqual(viewModel.serverInfo, "first", "bind 多次应只第一次生效")
        XCTAssertEqual(stub1.executeCallCount, 1)
        XCTAssertEqual(stub2.executeCallCount, 0)
    }
}

/// 手写 mock：实现 PingUseCaseProtocol，记录调用次数，按 stub 返回受控 result。
final class StubPingUseCase: PingUseCaseProtocol, @unchecked Sendable {
    let stubResult: PingResult
    private(set) var executeCallCount = 0

    init(stubResult: PingResult) {
        self.stubResult = stubResult
    }

    func execute() async -> PingResult {
        executeCallCount += 1
        return stubResult
    }
}
```

新建 `iphone/PetAppTests/Features/Home/UseCases/MockAPIClient.swift`（**手写 mock**，与 Story 2.4 `MockURLSession` 同风格）：

```swift
import Foundation
@testable import PetApp

/// PingUseCase 单元测试用的 APIClientProtocol mock。
///
/// 用法：
///   let mock = MockAPIClient()
///   mock.stubResponse["/ping"] = .success(Empty())
///   mock.stubResponse["/version"] = .success(VersionResponse(commit: "abc", builtAt: "..."))
///   let useCase = DefaultPingUseCase(client: mock)
///
/// 接受按 endpoint.path 字符串索引的 stub map。`request<T>` 实现：
///   - 找到 path 的 stub → 按 .success(value) / .failure(error) 行为返回 / 抛错
///   - 未找到 → 抛 APIError.decoding(StubMissingError)（暴露测试遗漏）
final class MockAPIClient: APIClientProtocol, @unchecked Sendable {
    enum Stub {
        case success(any Decodable & Sendable)
        case failure(APIError)
    }

    var stubResponse: [String: Stub] = [:]
    private(set) var invocations: [Endpoint] = []

    func request<T: Decodable>(_ endpoint: Endpoint) async throws -> T {
        invocations.append(endpoint)
        guard let stub = stubResponse[endpoint.path] else {
            throw APIError.decoding(underlying: NSError(
                domain: "MockAPIClient",
                code: -100,
                userInfo: [NSLocalizedDescriptionKey: "Stub missing for path: \(endpoint.path)"]
            ))
        }
        switch stub {
        case .success(let value):
            guard let typed = value as? T else {
                throw APIError.decoding(underlying: NSError(
                    domain: "MockAPIClient",
                    code: -101,
                    userInfo: [NSLocalizedDescriptionKey: "Stub type mismatch: expected \(T.self), got \(type(of: value))"]
                ))
            }
            return typed
        case .failure(let error):
            throw error
        }
    }
}
```

**关键约束**：
- 测试文件目录结构镜像 production：`PetAppTests/Features/Home/UseCases/PingUseCaseTests.swift` 对应 `PetApp/Features/Home/UseCases/PingUseCase.swift`。
- 不引入第三方 mock 库（ADR-0002 §3.1）；所有 mock（`StubPingUseCase` / `MockAPIClient`）均手写。
- 全部测试方法签名 `async` 或 `async throws`（ADR-0002 §3.2 主流方案）。
- `@MainActor` 标注 class（与 Story 2.3 / 2.4 测试一致）。
- 每个测试方法独立断言，**不**共享 fixture（避免 Story 2.4 lesson `2026-04-26-urlprotocol-stub-global-state.md` 同类陷阱：本 story 用实例化 mock 而非 static state，天然隔离）。
- 至少 **5 + 7 = 12 个**测试方法；epics.md AC 要求 ≥ 4 case，本 story 实装 12 case，覆盖三态 + idempotency + 老路径兼容 + bind 路径。

**AC10 — 集成测试覆盖（StubURLProtocol fake server 模式）**

epics.md AC 要求"集成测试覆盖：跑 XCTest mock server → App 启动 → 主界面版本号显示 mock commit"。本 story 复用 Story 2.4 落地的 `StubURLProtocol` 路径，落地以下集成 case：

新建 `iphone/PetAppTests/Features/Home/UseCases/PingUseCaseIntegrationTests.swift`：

```swift
import XCTest
@testable import PetApp

@MainActor
final class PingUseCaseIntegrationTests: XCTestCase {

    /// 真 URLSession + 真 APIClient + StubURLProtocol stub server →
    /// PingUseCase.execute() → result.serverCommit == "abc1234"
    /// 本 case 验证：APIClient + JSONDecoder + Endpoint + UseCase 的端到端联调
    func testFullStackPingAndVersionHappyPath() async {
        // GIVEN：StubURLProtocol 按 path 路由响应
        StubURLProtocol.reset()
        StubURLProtocol.stubData = """
        {"code":0,"message":"pong","data":{},"requestId":"req_ping"}
        """.data(using: .utf8)!
        StubURLProtocol.stubStatusCode = 200

        // 注：StubURLProtocol 当前不支持按 path 分支（设计约束 see lessons）；
        //   本 case 用"序列化预设两次响应"路径——即先用 ping stub，调完后立即重置成 version stub。
        //   实际实装时若 StubURLProtocol 不支持 sequential stub，可在 setUp 里根据 request.url.path 在
        //   StubURLProtocol.startLoading() 中分支选 stub（需评估是否扩展 StubURLProtocol API；
        //   推荐：本 story 在 PingUseCaseIntegrationTests.swift 顶部新建 RoutingStubURLProtocol 子类专用此测试，
        //   不污染 Story 2.4 的 StubURLProtocol 主体）。

        // 详细落地见 Dev Note #6 "集成测试 stub 路由方案"。
        XCTSkip("路由式 stub 待 Dev Note #6 决策落地。")
    }
}
```

**集成测试落地最终方案（dev 实装时按此）**：

由于 `StubURLProtocol` 当前是**单一 stub**模式（一组 static 字段不区分 path），无法在一次测试中同时 stub `/ping` 和 `/version` 两个不同响应，本 story **新建专用 `PingStubURLProtocol`**（专门支持按 path 路由），放在 test target 内不污染 Story 2.4 的 `StubURLProtocol`：

```swift
// iphone/PetAppTests/Features/Home/UseCases/PingStubURLProtocol.swift
import Foundation

/// 按 URL path 路由响应的 URLProtocol stub，用于 PingUseCase 集成测试。
/// 与 Story 2.4 的 StubURLProtocol 区别：本 stub 支持按 path 分支（同一 session 内多次请求
/// 可返回不同响应）；StubURLProtocol 是单一 stub（同一 session 内所有请求返回同样响应）。
///
/// 并发约束（继承 Story 2.4 lesson `2026-04-26-urlprotocol-stub-global-state.md`）：
/// - 一时刻进程内只允许一个 testcase 用本工具
/// - static 字段读写均加 NSLock；snapshot 原子读
/// - 仅 session-local 注入（URLSessionConfiguration.protocolClasses），**禁止** registerClass
final class PingStubURLProtocol: URLProtocol {
    private static let lock = NSLock()
    private static var _routes: [String: (statusCode: Int, data: Data)] = [:]

    static func setRoute(_ path: String, statusCode: Int, data: Data) {
        lock.lock(); defer { lock.unlock() }
        _routes[path] = (statusCode, data)
    }

    static func reset() {
        lock.lock(); defer { lock.unlock() }
        _routes = [:]
    }

    private static func snapshot(for path: String) -> (statusCode: Int, data: Data)? {
        lock.lock(); defer { lock.unlock() }
        return _routes[path]
    }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
    override func startLoading() {
        let path = request.url?.path ?? ""
        guard let route = Self.snapshot(for: path) else {
            client?.urlProtocol(self, didFailWithError: URLError(.fileDoesNotExist))
            return
        }
        let response = HTTPURLResponse(
            url: request.url!,
            statusCode: route.statusCode,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        )!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: route.data)
        client?.urlProtocolDidFinishLoading(self)
    }
    override func stopLoading() {}
}
```

集成测试用法：

```swift
final class PingUseCaseIntegrationTests: XCTestCase {

    override func setUp() {
        super.setUp()
        PingStubURLProtocol.reset()
    }

    override func tearDown() {
        PingStubURLProtocol.reset()
        super.tearDown()
    }

    @MainActor
    func testFullStackPingAndVersionHappyPath() async {
        // GIVEN
        PingStubURLProtocol.setRoute("/ping", statusCode: 200, data: """
        {"code":0,"message":"pong","data":{},"requestId":"req_ping_int"}
        """.data(using: .utf8)!)
        PingStubURLProtocol.setRoute("/version", statusCode: 200, data: """
        {"code":0,"message":"ok","data":{"commit":"abc1234","builtAt":"2026-04-26T08:00:00Z"},"requestId":"req_version_int"}
        """.data(using: .utf8)!)

        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [PingStubURLProtocol.self]
        let session = URLSession(configuration: config)
        let client = APIClient(baseURL: URL(string: "http://test-server.local")!, session: session)
        let useCase = DefaultPingUseCase(client: client)

        // WHEN
        let result = await useCase.execute()

        // THEN
        XCTAssertEqual(result, PingResult(reachable: true, serverCommit: "abc1234"))
    }

    @MainActor
    func testFullStackPingFailureReturnsOffline() async {
        // GIVEN: ping 路由返回 5xx → APIClient 抛 .network → PingUseCase 转 reachable=false
        PingStubURLProtocol.setRoute("/ping", statusCode: 500, data: Data())

        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [PingStubURLProtocol.self]
        let session = URLSession(configuration: config)
        let client = APIClient(baseURL: URL(string: "http://test-server.local")!, session: session)
        let useCase = DefaultPingUseCase(client: client)

        // WHEN
        let result = await useCase.execute()

        // THEN
        XCTAssertEqual(result, PingResult(reachable: false, serverCommit: nil))
    }
}
```

**关键约束**：
- **不**修改 Story 2.4 的 `StubURLProtocol`（5d97a74 / 2b0449a 已稳定）；本 story 新建独立 `PingStubURLProtocol`。
- 仅 **session-local 注入**（`URLSessionConfiguration.protocolClasses`），**不**调 `URLProtocol.registerClass(_:)`（继承 Story 2.4 lesson `2026-04-26-urlprotocol-session-local-vs-global.md`）。
- setUp / tearDown 严格 `reset()`（继承 Story 2.4 lesson `2026-04-26-urlprotocol-stub-global-state.md`）。
- 至少 **2 个集成 case**：happy path（ping + version 双成功）+ ping 失败 path；版本部分降级 / 业务错误等场景由单元测试覆盖。

**AC11 — `iphone/project.yml` 改动评估**

本 story 修改 `iphone/project.yml` **唯一且必要**的两行：`info.properties.CFBundleShortVersionString` + `CFBundleVersion`（见 AC8）。

新增的源码目录与 .swift 文件由 `sources: - PetApp` / `sources: - PetAppTests` 自动 glob，**无需**额外 yaml 改动。

修改 `project.yml` 后必须运行：

```bash
cd iphone && xcodegen generate
```

让 `PetApp.xcodeproj/project.pbxproj` regen，接收：
- 新源码 file references（4 production + 4 test）
- `Info.plist` properties 更新（如果 XcodeGen 把 `CFBundleShortVersionString` 写到 Build Settings 而非 Info.plist 文件，看实际行为；两种都 OK）

**AC12 — 全套测试通过 + Story 2.2 / 2.3 / 2.4 回归不破**

跑全套 `xcodebuild test`，确认：

- PetAppTests：
  - Story 2.2 老 10 case + Story 2.3 新 12 case + Story 2.4 新 ~10 case + 本 story 新 12 单元 + 2 集成 ≈ 46+ case **全 0 失败**
  - 特别注意 `HomeViewModelTests.testHardcodedDefaultStateMatchesStorySpec` 老 case **必须仍然通过**（验证旧 init `appVersion = "0.0.0"` / `serverInfo = "----"` 默认不变）
- PetAppUITests：Story 2.2 老 1 case + Story 2.3 新 3 case = 4 case **全 0 失败**（本 story 不新增 UITest，但需验证 UITest 看到的 versionLabel 仍含 a11y identifier `home_versionLabel`，不被破坏）

如有任何老测试失败，必须诊断根因后修复（**不允许**通过修改老断言绕过）。

**AC13 — `git status` 最终自检（防 scope creep / 防误改 `ios/` / `server/`）**

最终 commit 前跑 `git status`，确认仅以下文件被 created / modified：

新建 production（4）：
- `iphone/PetApp/Features/Home/UseCases/PingResult.swift`
- `iphone/PetApp/Features/Home/UseCases/PingEndpoints.swift`
- `iphone/PetApp/Features/Home/UseCases/PingUseCase.swift`
- `iphone/PetApp/App/AppContainer.swift`

修改 production（2）：
- `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`（追加 init / start / bind / applyPingResult）
- `iphone/PetApp/App/RootView.swift`（追加 `.task { await ... }`、新增 container `@StateObject`）

新建 test（4）：
- `iphone/PetAppTests/Features/Home/UseCases/PingUseCaseTests.swift`
- `iphone/PetAppTests/Features/Home/UseCases/PingUseCaseIntegrationTests.swift`
- `iphone/PetAppTests/Features/Home/UseCases/MockAPIClient.swift`
- `iphone/PetAppTests/Features/Home/UseCases/PingStubURLProtocol.swift`

新建 test（1，可选合并）：
- `iphone/PetAppTests/Features/Home/HomeViewModelPingTests.swift`（也可合并到现有 `HomeViewModelTests.swift` 文件末尾；推荐独立文件以保持 Story 2.2 老测试零改动）

修改 config（2）：
- `iphone/project.yml`（仅 `info.properties` 加两行）
- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen auto-regen 接收新文件 references；无手工编辑）

修改 BMAD（2）：
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（dev-story workflow 推 status：backlog → ready-for-dev → in-progress → review）
- `_bmad-output/implementation-artifacts/2-5-ping-调用-主界面显示-server-version-信息.md`（本文件：勾选 Tasks/Subtasks + Dev Agent Record）

**绝对禁止**：
- `ios/` 下任何文件被 modified / staged
- `server/` 下任何文件被 modified / staged（包括 server/cmd / server/internal / server/scripts 等）
- `CLAUDE.md` / `docs/` 下任何文件被 modified（非本 story scope）
- Story 2.4 的 `iphone/PetApp/Core/Networking/` 5 个文件被改动（APIClient / APIError / Endpoint / APIResponse / URLSessionProtocol）
- Story 2.2 / 2.3 已落地 production 文件被破坏（HomeView / AppCoordinator / PetAppApp / 三个 SheetPlaceholderView / AccessibilityID 任何老常量字符串值；`HomeViewModel` 仅允许追加新方法 / init 重载，老字段 / 老 init 默认值不动）
- Story 2.4 测试文件（APIClientTests / MockURLSession / StubURLProtocol / APIClientIntegrationTests）被改动

**AC14 — `import` 列表 hygiene（继承 Story 2.2 / 2.4 lesson）**

新增 production / test 文件顶部的 `import` 列表必须满足：

| 文件 | 必含 import |
|---|---|
| `PingResult.swift` | `Foundation`（仅此一行） |
| `PingEndpoints.swift` | `Foundation`（仅此一行） |
| `PingUseCase.swift` | `Foundation`（仅此一行） |
| `AppContainer.swift` | `Foundation`、`Combine`（`ObservableObject` 来自 Combine） |
| `HomeViewModel.swift`（**修改**） | 维持 `Foundation`、`Combine`（已有，本 story 不删） |
| `RootView.swift`（**修改**） | 维持 `SwiftUI`（已有；**不**新增 import） |
| `PingUseCaseTests.swift` | `XCTest`、`@testable import PetApp` |
| `HomeViewModelPingTests.swift` | `XCTest`、`@testable import PetApp` |
| `PingUseCaseIntegrationTests.swift` | `XCTest`、`@testable import PetApp` |
| `MockAPIClient.swift` | `Foundation`、`@testable import PetApp` |
| `PingStubURLProtocol.swift` | `Foundation` |

**禁止**：
- 任何 production 文件 `import UIKit`（不需要）
- 任何 production 文件未先 `import Foundation` 就用 `URL` / `URLSession` 等（依赖 transitive 不算）
- `PingUseCase.swift` / `PingResult.swift` / `PingEndpoints.swift` 顶部出现 `import Combine` / `import SwiftUI`（这三个文件是纯业务 / 数据层，不应被 UI 层 framework 污染）
- 单测 / 集成测试文件出现第三方 mock 库（OHHTTPStubs / Mockingbird / Cuckoo）

**理由（Story 2.2 / 2.4 lesson 应用）**：
- Story 2.2 lesson `2026-04-25-swift-explicit-import-combine.md`：`@Published` / `ObservableObject` 必须显式 import Combine。`HomeViewModel.swift`（已有此 import）/ `AppContainer.swift`（新增此 import）严格遵守。
- Story 2.4 lesson `2026-04-26-jsondecoder-encoder-thread-safety.md`：本 story 复用 Story 2.4 的 APIClient（已修复），`PingUseCase` 自身不持有 JSONDecoder / JSONEncoder。
- Story 2.4 lesson `2026-04-26-url-trailing-slash-concat.md`：本 story `AppContainer` 默认 baseURL `http://localhost:8080`（**无** trailing slash），`PingEndpoints.ping()` / `.version()` path **以 `/` 开头**——拼接结果 `http://localhost:8080/ping` / `.../version`，不会双斜杠。

## Tasks / Subtasks

- [x] **T1**：实装 `PingResult` 模型（AC1）
  - [x] T1.1 新建 `iphone/PetApp/Features/Home/UseCases/PingResult.swift`
  - [x] T1.2 定义 `struct PingResult: Equatable` 含 `reachable: Bool` / `serverCommit: String?` 两字段
  - [x] T1.3 文件顶部仅 `import Foundation`

- [x] **T2**：实装 `PingEndpoints` 工厂 + `VersionResponse` 模型（AC2）
  - [x] T2.1 新建 `iphone/PetApp/Features/Home/UseCases/PingEndpoints.swift`
  - [x] T2.2 定义 `enum PingEndpoints` 含 `static func ping() -> Endpoint` / `static func version() -> Endpoint`
  - [x] T2.3 path 严格用 `"/ping"` / `"/version"`（**不**含 `/api/v1` 前缀）；method `.get`；body `nil`；requiresAuth `false`
  - [x] T2.4 定义 `typealias PingResponse = Empty`（复用 Story 2.4 落地的 `Empty: Decodable`）
  - [x] T2.5 定义 `struct VersionResponse: Decodable, Equatable` 含 `commit: String` / `builtAt: String`（小驼峰，对齐 server `version_handler.go:13-14`）
  - [x] T2.6 文件顶部仅 `import Foundation`

- [x] **T3**：实装 `PingUseCaseProtocol` + `DefaultPingUseCase`（AC3）
  - [x] T3.1 新建 `iphone/PetApp/Features/Home/UseCases/PingUseCase.swift`
  - [x] T3.2 定义 `protocol PingUseCaseProtocol: Sendable { func execute() async -> PingResult }`
  - [x] T3.3 实装 `struct DefaultPingUseCase: PingUseCaseProtocol`，注入 `client: APIClientProtocol`
  - [x] T3.4 `execute()` 内：先 try ping，失败 catch 返回 `PingResult(reachable: false, serverCommit: nil)`；ping 成功后 try version，失败 catch 返回 `PingResult(reachable: true, serverCommit: nil)`；version 成功返回 `PingResult(reachable: true, serverCommit: v.commit)`
  - [x] T3.5 **不抛错**（execute 签名是 `async -> PingResult`，无 throws）；**不**写 print / log（→ Story 2.7 logger 接入后再加）
  - [x] T3.6 文件顶部仅 `import Foundation`

- [x] **T4**：扩展 `HomeViewModel`（AC4）
  - [x] T4.1 修改 `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`
  - [x] T4.2 **保留**老 init（hardcode 默认值，pingUseCase=nil 隐式语义；为避免破坏老 ABI，老 init 签名一字不改）
  - [x] T4.3 **新增** init `init(nickname:pingUseCase:appVersion:serverInfo:onRoomTap:onInventoryTap:onComposeTap:)`：必填 `pingUseCase: PingUseCaseProtocol`；`appVersion` 默认 `HomeViewModel.readAppVersion()`
  - [x] T4.4 新增 `private let pingUseCase: PingUseCaseProtocol?`（老 init 设 nil，新 init 设非 nil）
  - [x] T4.5 新增 `private var boundPingUseCase: PingUseCaseProtocol?`（bind 路径用）
  - [x] T4.6 新增 `private var pingTask: Task<Void, Never>?`（idempotency 短路）
  - [x] T4.7 新增 `public func bind(pingUseCase:)`：`guard self.boundPingUseCase == nil else { return }; self.boundPingUseCase = pingUseCase`
  - [x] T4.8 新增 `public func start() async`：从 `pingUseCase ?? boundPingUseCase` 取注入实例；`pingTask` 非 nil 短路；正常 await result + applyPingResult
  - [x] T4.9 新增 `private func applyPingResult(_ result: PingResult)`：三态文案投影（详见 AC4）
  - [x] T4.10 新增 `public nonisolated static func readAppVersion() -> String`：从 `Bundle.main.infoDictionary?["CFBundleShortVersionString"]` 读，缺省 "0.0.0"
  - [x] T4.11 顶部 `import Foundation` + `import Combine` 不动（已有）

- [x] **T5**：实装 `AppContainer`（AC6）
  - [x] T5.1 新建 `iphone/PetApp/App/AppContainer.swift`
  - [x] T5.2 定义 `@MainActor public final class AppContainer: ObservableObject`
  - [x] T5.3 持有 `public let apiClient: APIClientProtocol`
  - [x] T5.4 实装 `convenience init()` 默认 baseURL `http://localhost:8080`；`init(apiClient:)` 注入式
  - [x] T5.5 实装 `public func makePingUseCase() -> PingUseCaseProtocol { DefaultPingUseCase(client: apiClient) }`
  - [x] T5.6 顶部 `import Foundation` + `import Combine`

- [x] **T6**：改造 `RootView` wire（AC7）
  - [x] T6.1 修改 `iphone/PetApp/App/RootView.swift`
  - [x] T6.2 新增 `@StateObject private var container = AppContainer()`
  - [x] T6.3 保留现有 `@StateObject private var coordinator` / `@StateObject private var homeViewModel = HomeViewModel()` 不动
  - [x] T6.4 在 body 内现有 `.onAppear { wireHomeViewModelClosures() }` 后**追加** `.task { homeViewModel.bind(pingUseCase: container.makePingUseCase()); await homeViewModel.start() }`
  - [x] T6.5 现有 `wireHomeViewModelClosures` / `sheetContent(for:)` / `.fullScreenCover` 全部不动
  - [x] T6.6 顶部 `import SwiftUI` 不动

- [x] **T7**：修改 `iphone/project.yml`（AC8）
  - [x] T7.1 在 `targets.PetApp.info.properties` 加 `CFBundleShortVersionString: "1.0.0"`
  - [x] T7.2 在同处加 `CFBundleVersion: "1"`
  - [x] T7.3 不动 yaml 其它任何字段

- [x] **T8**：单元测试落地（AC9）
  - [x] T8.1 新建 `iphone/PetAppTests/Features/Home/UseCases/MockAPIClient.swift`：实现 `APIClientProtocol` 手写 mock，按 path stub
  - [x] T8.2 新建 `iphone/PetAppTests/Features/Home/UseCases/PingUseCaseTests.swift`：实装 5 个 case（happy / partial-degrade / offline-on-ping-fail / offline-on-business-error / partial-degrade-on-decoding-fail）
  - [x] T8.3 新建 `iphone/PetAppTests/Features/Home/HomeViewModelPingTests.swift`：实装 7 个 case（三态 + idempotency + 老路径 no-op + bind 路径 + bind 重复绑定）
  - [x] T8.4 文件内手写 `StubPingUseCase`（实现 `PingUseCaseProtocol`，记录 executeCallCount）
  - [x] T8.5 全部测试方法用 `func testXxx() async [throws] {}`；class 标 `@MainActor`
  - [x] T8.6 跑 `xcodebuild test -only-testing:PetAppTests/PingUseCaseTests` + `-only-testing:PetAppTests/HomeViewModelPingTests` 全 12 case 0 失败

- [x] **T9**：集成测试落地（AC10）
  - [x] T9.1 新建 `iphone/PetAppTests/Features/Home/UseCases/PingStubURLProtocol.swift`：按 path 路由 + NSLock 保护 + snapshot 原子读
  - [x] T9.2 新建 `iphone/PetAppTests/Features/Home/UseCases/PingUseCaseIntegrationTests.swift`：实装 2 个 case
  - [x] T9.3 setUp / tearDown 严格 `PingStubURLProtocol.reset()`
  - [x] T9.4 仅 session-local 注入（`URLSessionConfiguration.protocolClasses = [PingStubURLProtocol.self]`），**不**调 `URLProtocol.registerClass`
  - [x] T9.5 跑 `xcodebuild test -only-testing:PetAppTests/PingUseCaseIntegrationTests` 全 2 case 0 失败

- [x] **T10**：xcodegen + 整体回归 + git 自检（AC11-13）
  - [x] T10.1 `cd iphone && xcodegen generate` regen project.pbxproj 接收新 .swift 文件 + Info.plist 改动
  - [x] T10.2 跑 `xcodebuild test -project iphone/PetApp.xcodeproj -scheme PetApp -destination 'platform=iOS Simulator,name=iPhone 17,OS=latest'` 全套 0 失败
  - [x] T10.3 验证 `HomeViewModelTests.testHardcodedDefaultStateMatchesStorySpec` 老 case 仍 PASS（默认 init 字段值不变）
  - [x] T10.4 跑 `git status`：仅 iphone/ 内新增 + 修改 swift + project.yml + pbxproj（auto-regen）+ sprint-status.yaml + 当前 story 文件；**`ios/` / `server/` / `CLAUDE.md` / `docs/` 全部零改动**
  - [x] T10.5 dev-story workflow：勾选所有 Tasks/Subtasks + 填写 Dev Agent Record + Status: ready-for-dev → review

- [x] **T11**：import hygiene 自查（AC14）
  - [x] T11.1 4 个新 production 文件顶部 import 列表严格按 AC14 表
  - [x] T11.2 `PingResult.swift` / `PingEndpoints.swift` / `PingUseCase.swift` 顶部仅 `import Foundation`，不出现 Combine / SwiftUI
  - [x] T11.3 `AppContainer.swift` 顶部 `import Foundation` + `import Combine`
  - [x] T11.4 测试文件 `import XCTest` + `@testable import PetApp`，无第三方 mock 库

## Dev Notes

### 项目关键约束（必读，勿绕过）

1. **iPhone 工程目录由 ADR-0002 锁定**：本 story 在 `iphone/` 目录（Story 2.2 / 2.3 / 2.4 落地的）基础上叠加，**不动 `ios/`**。
2. **架构 §4 + §5.3 + §18.2 路径锁定**：UseCase 落 `Features/<Module>/UseCases/`；DI 容器落 `App/AppContainer.swift`。本 story 第一次实质用到 `Features/Home/UseCases/` 与 `App/AppContainer.swift`，作为后续 UseCase / Repository 的模板示范。
3. **iOS Mock 框架（ADR-0002 §3.1）**：XCTest only（手写 Mock）；本 story 实装 `MockAPIClient` + `StubPingUseCase` + `PingStubURLProtocol` 三类手写 mock，**不**引 Mockingbird / Cuckoo / OHHTTPStubs。
4. **异步测试方案（ADR-0002 §3.2）**：本 story 全部测试用 `async [throws]`（架构 §18.1 + ADR-0002 §3.2 主流方案）。
5. **`URLSession`（架构 §18.1）+ Story 2.4 APIClient**：iOS 端 HTTP 走 Story 2.4 的 `APIClient`，**禁止**绕开 APIClient 直接用 URLSession 调 ping/version。
6. **Sendable / async 一致性**：`PingUseCaseProtocol: Sendable` 与 Story 2.4 `APIClientProtocol: Sendable` 风格一致；`HomeViewModel @MainActor` 与 `AppCoordinator @MainActor` 风格一致。
7. **节点 1 整体未闭合**：Epic 1 done，Epic 2 进行中（2.1 / 2.2 / 2.3 / 2.4 done，本 2.5 是第 5 条）；本 story 完成后 **APIClient 首次接入业务路径**，距 Epic 3 节点 1 demo 验收又近一步。

### 关键技术细节

#### 1. baseURL 决策：host-only（无 `/api/v1` 前缀）

**问题**：V1接口设计 §2.2 规定接口前缀为 `/api/v1`。Story 2.4 doc comment 写 "baseURL 已含 `/api/v1` 前缀"。但 server `/ping` + `/version` 是**运维端点**，注册在根路径（router.go:30 注释 "运维端点（不走 /api/v1 前缀，不走业务 auth）"）。

**冲突点**：如果 baseURL = `http://localhost:8080/api/v1`，那 `Endpoint(path: "/ping")` 会拼成 `http://localhost:8080/api/v1/ping` —— 404。

**决策**：本 story 选 **host-only baseURL**（`http://localhost:8080`），让每个 endpoint 自带完整 path 前缀：

| 端点类型 | path 写法 | 拼接结果 |
|---|---|---|
| 运维（ping / version） | `/ping` / `/version` | `http://localhost:8080/ping` / `.../version` |
| 业务（auth / home / chest 等） | `/api/v1/auth/guest-login` / `/api/v1/home` / ... | `http://localhost:8080/api/v1/auth/guest-login` / ... |

**理由**：
- 单一 baseURL 适配 server 实际拓扑（运维 + 业务混部）。如果 baseURL 含 `/api/v1`，每次访问运维端点都要构造特殊 endpoint（如 `path: "/../ping"`）—— 黑魔法。
- 业务 endpoint 把 `/api/v1` 写在 path 里，一行清晰可见，**比** 把它隐藏在 baseURL 里更便于 grep / debug / log。
- Story 2.4 doc comment 的"baseURL 已含 `/api/v1`"是早期假设，本 story 是**第一条真实使用 baseURL 的 story**，可以校准。

**对 Story 2.4 文档的影响**：Story 2.4 已 done，doc 不必倒回去改；本 story 在 `PingEndpoints.swift` 顶部注释明示新约定即可（前向覆盖），未来 dev 看 PingEndpoints 注释能立即理解。

#### 2. PingResult 三态决策表

| ping 结果 | version 结果 | reachable | serverCommit | UI 文案 | epics.md AC 对应 |
|---|---|---|---|---|---|
| OK | OK + commit 非空 | true | "abc1234" | `v1.0.0 · abc1234` | "Server <server commit>" |
| OK | OK + commit 空字符串 | true | nil（被 isEmpty 兜底转 nil） | `v1.0.0 · v?` | "Server v?"（部分降级） |
| OK | 失败（network / decoding / business） | true | nil | `v1.0.0 · v?` | "Server v?"（部分降级） |
| 失败（network / decoding / business） | **不调** | false | nil | `v1.0.0 · offline` | "Server offline" |

**设计要点**：
- ping 失败时 version 不调（短路省一次请求 + 语义统一）。
- "v?" 表示 "version 接口本身有问题但 server 在线"，与 "offline"（server 整体不可达）有意区分。
- 不引入 `(reachable: false, serverCommit: 非空)` 的状态：业务上 ping 失败时即使 version 之前调过 cache 也不展示（避免 stale 数据误导）。

#### 3. `@StateObject` 与 init 注入的取舍（→ AC7 选用方案 B）

**问题**：SwiftUI 的 `@StateObject` 必须在 `View` init 阶段用 `@autoclosure` 默认值构造；运行时注入非 default 实例会被 SwiftUI 丢弃（SwiftUI 文档明确警告）。这意味着不能在 `RootView.init()` 里写 `_homeViewModel = StateObject(wrappedValue: HomeViewModel(pingUseCase: container.makePingUseCase()))` —— 因为 `container` 自身也是 `@StateObject`，init 阶段还未真正初始化。

**两条路径**：

**方案 A**（不推荐）：把 `homeViewModel` 从 `@StateObject` 降为 `@State`，自己管理生命周期。缺点：失去 SwiftUI 的 ViewModel 生命周期保证（rebuild 时可能重置），引入额外样板代码。

**方案 B**（**本 story 选定**）：保持 `@StateObject` + 默认 init，新增 `bind(pingUseCase:)` 方法做"运行时一次注入"，在 `.task { ... }` 中调。优点：
- 不破坏 SwiftUI 数据流约定；
- 不修改老 init（Story 2.2 / 2.3 测试零改动）；
- `bind` 单次生效（保护重复 .task 触发）；
- 方便测试场景下两种 init 并存（mock 注入用 init，production 用 bind）。

缺点：
- `HomeViewModel` 需要兼容"未注入"中间态（`pingUseCase = nil` 且 `boundPingUseCase = nil` 时 `start()` 是 no-op）—— 但这个状态在 production 路径下只是 `.task` 触发前的极短瞬间，不影响 UX。

#### 4. `bind()` 单次生效语义

```swift
public func bind(pingUseCase: PingUseCaseProtocol) {
    guard self.boundPingUseCase == nil else { return }   // ← 防重入
    self.boundPingUseCase = pingUseCase
}
```

**理由**：SwiftUI `.task` 在 view 重新出现时可能多次触发；如果 bind 不防重入，第二次会覆盖第一次的注入实例（理论上等价，因为同一 container 返回同质实例，但语义上"绑定"应当只发生一次）。

**与 `start()` 内的 `pingTask != nil` 短路区别**：
- `bind()` 防重入：保护"注入实例不被覆盖"
- `start()` 内 `pingTask != nil` 短路：保护"不重复发起 ping 请求"
- 两条防御互补，缺一不可

#### 5. `applyPingResult` 文案归属

```swift
private func applyPingResult(_ result: PingResult) {
    if !result.reachable {
        self.serverInfo = "offline"
    } else if let commit = result.serverCommit, !commit.isEmpty {
        self.serverInfo = commit
    } else {
        self.serverInfo = "v?"
    }
}
```

**字符串归属决策**：
- "offline" / "v?" 在 ViewModel 里写（业务投影逻辑）
- "v" 前缀 + "·" 分隔符 在 HomeView.versionLabel 模板里写（UI 视觉策略）

**为何不全放 View**：ViewModel 是状态投影中心，"reachable=false → offline" 的映射是业务决策（属于 ViewModel 职责）；UI 只决定字体 / 颜色 / 排版。

**为何不全放 ViewModel**：避免 ViewModel 知道太多 UI 细节（如分隔符）；HomeView 模板易于改版（如未来要把 versionLabel 拆成两行 `v1.0.0` + `Server: abc1234`，只改 View 不改 ViewModel）。

**i18n 时机**：当前 MVP 不做多语言；"offline" / "v?" 是英文（dev / demo 用）。Story 2.6 落地 i18n 时，把这两个字符串移到 `Localizable.strings`。

#### 6. 集成测试 stub 路由方案

Story 2.4 的 `StubURLProtocol` 是**单一 stub**模式（一组 static 字段，所有请求返回同一响应）。本 story 需要在一次测试中同时 stub `/ping`（返回空 envelope）+ `/version`（返回 `{commit, builtAt}`）—— 单一 stub 模式不适用。

**方案选项**：
- **方案 1**：扩展 `StubURLProtocol` 加 path-routing 能力 —— 否决：会破坏 Story 2.4 已稳定的单测，scope creep
- **方案 2**：在 `PingUseCaseIntegrationTests` 内**新建独立 `PingStubURLProtocol`**（含 path-routing），与 Story 2.4 主体隔离 —— **本 story 选定**
- **方案 3**：用真 server（Vapor / swifter）—— 否决：引入第三方依赖

`PingStubURLProtocol` 实装严格继承 Story 2.4 的两条 lesson：
- `2026-04-26-urlprotocol-stub-global-state.md`：static 字段加 NSLock；`snapshot(for:)` 原子读
- `2026-04-26-urlprotocol-session-local-vs-global.md`：仅 session-local 注入；**不**调 registerClass

#### 7. `Bundle.main.infoDictionary` 在测试 target 的行为

`HomeViewModel.readAppVersion()` 在测试 target 内运行时，`Bundle.main` 指向**测试 host**（XCTest runner），不是 PetApp 主 bundle。XCTest runner 可能没有 `CFBundleShortVersionString`，会落到 fallback `"0.0.0"`。

**测试断言策略**：
- `HomeViewModelPingTests` 里**不直接断言 appVersion 的值**（避免依赖测试 host bundle 状态）
- 只断言 `serverInfo` 的值（这是 PingUseCase 驱动的字段，确定可控）
- 老 case `testHardcodedDefaultStateMatchesStorySpec` 用**老 init**（`HomeViewModel()`），`appVersion = "0.0.0"` 是显式默认值——不走 `readAppVersion()` 路径，所以不受测试 host bundle 影响

#### 8. SwiftUI `.task` 与 `weak self`

`HomeViewModel.start()` 内的 `Task { [weak self] in ... }`：

```swift
let task = Task { [weak self] in
    let result = await pingUseCase.execute()
    await self?.applyPingResult(result)
}
```

- `[weak self]` 防止 ViewModel 被 task 强引用导致泄漏
- `await self?.applyPingResult(result)` 是 `MainActor` 隔离的方法调用，自动 hop 回主线程
- `pingUseCase` 在 closure 里被 strong 捕获（局部变量），不依赖 self —— 这样 ViewModel deinit 后 task 仍能完成（applyPingResult 时 self 已 nil，整个 update 静默 noop）

`pingTask = nil` 在 `await task.value` 之后赋值：保证下次 start() 调用能识别"没有进行中任务"。

#### 9. Story 2.2 / 2.4 review lessons learned 应用

| Lesson | 文件 | 本 story 应用 |
|---|---|---|
| `2026-04-25-swift-explicit-import-combine.md` | HomeViewModel.swift / AppContainer.swift | 显式 `import Combine`（HomeViewModel 已有；AppContainer 新增） |
| `2026-04-25-swiftui-zstack-overlay-bottom-cta.md` | HomeView.swift | 不接 UI 改动，无直接交集；**不**碰 versionLabel 的 layout |
| `2026-04-26-jsondecoder-encoder-thread-safety.md` | APIClient.swift | 复用 Story 2.4 已修复的 makeDecoder / makeEncoder factory，PingUseCase 不持有 coder |
| `2026-04-26-urlprotocol-stub-global-state.md` | PingStubURLProtocol.swift | NSLock 保护 static 字段 + snapshot 原子读 |
| `2026-04-26-url-trailing-slash-concat.md` | AppContainer.swift / PingEndpoints.swift | baseURL `http://localhost:8080`（无 trailing slash）+ path 必 `/` 开头 |
| `2026-04-26-urlprotocol-session-local-vs-global.md` | PingUseCaseIntegrationTests.swift | 仅 `URLSessionConfiguration.protocolClasses` 注入；**不**调 `URLProtocol.registerClass` |
| dev-story 2-3 lesson（a11y identifier 父容器传播） | HomeView.swift | 不接 UI 改动；versionLabel a11y identifier 不变 |

#### 10. epics.md Story 2.5 vs 本文件 AC 对照

| epics.md 原文 AC | 本文件 AC# |
|---|---|
| App 启动时 `HomeViewModel` 触发 `PingUseCase` + `FetchVersionUseCase` | AC3（合并为一个 `PingUseCase`，内部串行调 ping + version；保留单一 UseCase 边界更简洁）+ AC4（HomeViewModel.start）+ AC7（RootView.task） |
| 主界面版本号显示格式: `App v<App build hash> · Server <server commit>` | AC5（HomeView.versionLabel 模板不动，由 ViewModel 推送字段值） |
| ping 失败时版本号显示 `App v<...> · Server offline` | AC4（applyPingResult: reachable=false → serverInfo="offline"） |
| 调用通过 APIClient（不是直接 URLSession） | AC3（DefaultPingUseCase 注入 APIClientProtocol） |
| 单元测试覆盖（≥4 case，mocked APIClient） | AC9（实装 12 case：5 PingUseCase + 7 HomeViewModel） |
| 集成测试覆盖：跑 XCTest mock server → App 启动 → 主界面版本号显示 mock commit | AC10（PingUseCaseIntegrationTests 实装 2 case，用 PingStubURLProtocol fake） |

**本文件超出 epics.md 原 AC 的部分（合理扩展）**：
- AC1：独立 `PingResult` value type（封装三态语义，便于测试断言）
- AC2：独立 `PingEndpoints` 工厂（集中 path / method 配置，便于未来扩展）
- AC6：首次落地 `AppContainer`（DI 容器，对齐架构 §18.2；为节点 2+ 业务 UseCase 准备）
- AC7：`RootView` 改造保持 `@StateObject` 数据流约定 + `bind` 单次注入（避免 SwiftUI init 注入陷阱）
- AC8：`Info.plist` 加 `CFBundleShortVersionString`（让 `Bundle.main.infoDictionary` 能拿到版本号）
- AC11-13：xcodegen / git status 自检（防 scope creep / 防误改 `ios/` / `server/`）
- AC14：import hygiene（继承 Story 2.2 / 2.4 lesson）

**FetchVersionUseCase 是否独立成 UseCase**：epics.md 原文写 `PingUseCase + FetchVersionUseCase` 两个 UseCase。本文件**合并为单一 `PingUseCase`**：
- 节点 1 demo 阶段 ping + version 是同一个"探活"语义动作（version 是 ping 的扩展），分两个 UseCase 增加 wire 复杂度
- ViewModel 只关心一个 PingResult，不需要 FetchVersionResult / PingResult 分别消费
- 未来若 ping / version 真的有独立调用场景（如某页只关心 version），再拆 UseCase 成本可控（提取 `FetchVersionUseCase` 即可）
- 这是**精简**而非"违反 epics 原文"——保留语义等价（一次性拉两个端点），只是工程上合并

### iOS 架构设计 §4 + §5.3 + §18.2 目录映射

本 story 涉及的目录：

| 架构路径 | 本 story 落地 | 文件 |
|---|---|---|
| `PetApp/App/AppContainer.swift` | ✅（首次落地，§4 + §18.2） | 新建 |
| `PetApp/Features/Home/UseCases/` | ✅（首次落地，§4 + §5.3） | 新建子目录 |
| `PetApp/Features/Home/UseCases/PingUseCase.swift` | ✅ | 新建 |
| **本 story 自补**：`PingResult.swift` | ✅（PingUseCase 输出 value type） | 新建 |
| **本 story 自补**：`PingEndpoints.swift` | ✅（端点工厂） | 新建 |
| `PetApp/Features/Home/ViewModels/HomeViewModel.swift` | ✅ 修改（追加 init / start / bind） | 修改 |
| `PetApp/App/RootView.swift` | ✅ 修改（追加 .task + container） | 修改 |
| `PetApp/Features/Home/Views/HomeView.swift` | ❌（不改） | 不动 |
| `PetApp/Core/Networking/*.swift`（Story 2.4 落地） | ❌（不改） | 不动 |

**测试目录**：`PetAppTests/Features/Home/UseCases/` 镜像 production，与 Story 2.4 `PetAppTests/Core/Networking/` 同风格。

### 与 Story 2.2 / 2.3 / 2.4（已 done）的衔接

- **Story 2.2** 落地 `iphone/` 目录骨架 + `HomeView` + `HomeViewModel`（hardcode `appVersion / serverInfo`）+ 8 个 a11y identifier
- **Story 2.3** 落地 `AppCoordinator` + `RootView` 路由 + 三个 SheetPlaceholder + UITest 三个导航测试
- **Story 2.4** 落地 `Core/Networking/` 5 个文件（APIClient / APIError / Endpoint / APIResponse / URLSessionProtocol）+ 测试 4 文件
- **本 story（2.5）**：在前三者基础上**第一次连接业务路径**——APIClient 真正被业务调用（PingUseCase），HomeViewModel 真正被业务驱动（不再是 hardcode）
- 本 story **不动** Story 2.2 / 2.3 / 2.4 的任何 production 文件主体（HomeViewModel 仅追加方法 / init 重载，老字段 / 老 init 不动；RootView 仅追加 `.task` + container `@StateObject`，老逻辑不动）

### 与 Server 端 Story 1.2 / 1.4（已 done）的对照（端到端契约）

| 维度 | server（已 done） | iOS（本 story） |
|---|---|---|
| `/ping` 端点 | server router.go:45 注册；返回 envelope `{code:0, message:"pong", data:{}}` | iOS `PingEndpoints.ping()` 构造 `Endpoint(path:"/ping", method:.get, requiresAuth:false)`；`PingResponse = Empty` 解码 |
| `/version` 端点 | server router.go:46 注册；返回 envelope `{code:0, message:"ok", data:{commit, builtAt}}` | iOS `PingEndpoints.version()` 构造 `Endpoint(path:"/version", method:.get, requiresAuth:false)`；`VersionResponse{commit, builtAt}` 解码 |
| 路径前缀 | **不**走 `/api/v1`（运维端点） | iOS baseURL `http://localhost:8080`（host-only，**不**含 `/api/v1`） |
| 鉴权 | 无（`/ping` / `/version` 不挂业务 auth 中间件） | iOS `requiresAuth: false` |
| 字段命名 | `commit` / `builtAt`（小驼峰，json tag）| iOS `VersionResponse.commit` / `.builtAt`（与 server 一字不差） |
| 默认值 | server `buildinfo.Commit` 在未注入 ldflags 时为 `"unknown"` | iOS 不需要"unknown"特判：commit 字段拿到什么字符串就显示什么；空字符串走 `"v?"` 路径 |

### 与未来 Epic 5 / Epic 4 的对接预案

- **Epic 5 Story 5.3 (AuthInterceptor)**：本 story 的 `Endpoint(requiresAuth: false)` 字段为 AuthInterceptor 预留位；ping / version 始终 false，业务 endpoint Epic 4+ 设 true，AuthInterceptor 按本字段拦截注入 Bearer token。
- **Epic 4+ 业务 UseCase**：`AppContainer.makePingUseCase()` 是**模板示范**，节点 2 起将追加 `makeGuestLoginUseCase()` / `makeLoadHomeUseCase()` 等。后续 Repository 层引入时（如 `AuthRepository`），AppContainer 持有 Repository 实例，UseCase 通过 init 注入 Repository（与本 story PingUseCase 注入 APIClient 同模式）。

### 范围红线（再次强调）

- 不写 `iphone/scripts/build.sh`（→ Story 2.7）
- 不写 `MockBase.swift`（→ Story 2.7）
- 不写 `AuthInterceptor.swift` 文件实体（→ Epic 5 Story 5.3）
- 不写 `WebSocketClient.swift`（→ Epic 10 / 12）
- 不写 `LaunchingView.swift` / `AppLaunchState`（→ Story 2.9）
- 不写 `ResetKeychainUseCase` / dev 重置按钮（→ Story 2.8）
- 不写 ErrorPresenter / Toast / AlertOverlay / RetryView（→ Story 2.6）
- 不引入第三方 HTTP / mock / 测试库
- 不引入第三方 DI 框架（Swinject / Resolver）
- 不创建 Repository（首个 Repository 由 Epic 4 落地，节点 2 起）
- 不动 Story 2.2 / 2.3 / 2.4 已落地 production 文件主体
- 不修改 `AccessibilityID.Home.*` / `AccessibilityID.SheetPlaceholder.*` 任何老常量字符串值
- 不真实调用 server（demo 联调留给 Epic 3 Story 3.1 / 3.2）
- 不修改 `iphone/project.yml` 除 `info.properties` 外任何字段

### 关键风险与缓解

| 风险 | 缓解 |
|---|---|
| `@StateObject` init 阶段无法注入运行时构造的依赖 | 选用方案 B（bind 单次注入 + .task 触发）；Dev Note #3 说明取舍 |
| SwiftUI `.task` 在视图刷新时多次触发引发并发 ping | `pingTask != nil` 短路 + `bind()` 防重入双重防御 |
| `ping` 路径调用前 baseURL 含 `/api/v1` 会 404 | host-only baseURL 决策（Dev Note #1）；`PingEndpoints.swift` 顶部注释明示新约定 |
| `PingStubURLProtocol` 全局 static 状态在并行测试下污染 | NSLock + `snapshot(for:)` 原子读（继承 Story 2.4 lesson） |
| version 字段名变动（如 server 把 `builtAt` 改 `built_at`）会让 iOS 解码失败 | `VersionResponse` decoding 失败映射为部分降级（`v?`），不阻塞 reachable 判断；server 字段名变动应在 contract 层（V1接口设计文档）通知 |
| `Bundle.main.infoDictionary` 在测试 host 内不含 CFBundleShortVersionString | `readAppVersion()` 兜底返回 `"0.0.0"`；测试断言只对 `serverInfo` 不对 `appVersion` |
| 集成测试用 `URLSession.shared` 而非自建 session 引发跨测试污染 | 集成测试每条 case 自建 `URLSession(configuration:)`，仅 session-local 注入 PingStubURLProtocol；遵守 Story 2.4 lesson `2026-04-26-urlprotocol-session-local-vs-global.md` |
| 老测试 `HomeViewModelTests.testHardcodedDefaultStateMatchesStorySpec` 因 init 默认值变动失败 | 老 init 签名 / 默认值一字不改；新 init 是新增重载，不破坏老 init 调用方 |
| `PingUseCase.execute()` 不抛错，调用方误以为永远成功 | doc comment 明示三态返回 + 调用方按 `result.reachable` / `result.serverCommit` 显式分支；ViewModel.applyPingResult 已覆盖三态 |

### Project Structure Notes

- **与目标结构（iOS 架构设计 §4 + §5.3 + §18.2）的对齐**：本 story 落地 `App/AppContainer.swift`（§4 + §18.2 钦定路径）+ `Features/Home/UseCases/{PingUseCase, PingResult, PingEndpoints}.swift`（§4 + §5.3 模式），完全对齐架构。
- **`Features/<Module>/UseCases/` 第一次实质用到**：Story 2.2 / 2.3 留了 `Features/Home/{Views, ViewModels}/`，本 story 第一次填 `Features/Home/UseCases/` 目录，作为后续 Epic 4+ 业务 UseCase 的模板示范。
- **xcodegen auto-regen 副作用**：新增子目录 + .swift 文件后必须 `cd iphone && xcodegen generate`（与 Story 2.3 / 2.4 同惯例）；本 story 还要 regen `Info.plist`（因 yml 改动）—— xcodegen 会一并处理。
- **测试目录 `PetAppTests/Features/Home/UseCases/`**：镜像 production，与 Story 2.4 `PetAppTests/Core/Networking/` 同风格。

### References

- [Source: \_bmad-output/planning-artifacts/epics.md#Epic 2 / Story 2.5] — 原始 AC 来源（行 748-767）
- [Source: \_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md] — **本 story 唯一权威 ADR**
  - §3.1 — XCTest only（手写 Mock）：本 story `MockAPIClient` / `StubPingUseCase` / `PingStubURLProtocol` 全部手写
  - §3.2 — async/await 主流：本 story 全 async test
  - §3.3 — 方案 D：`iphone/` 下从零建工程
  - §4 — 版本锁定清单（Xcode 26.4.1 / iOS 17 deployment target / SWIFT_VERSION 5.9）
  - §5.1 — 对 Story 2.5 的影响："按 §3.1 mock APIClient（`class MockAPIClient: APIClientProtocol`）；按 §3.2 `async test` 验证 ViewModel 状态切换"
- [Source: \_bmad-output/implementation-artifacts/2-2-swiftui-app-入口-主界面骨架-信息架构定稿.md] — Story 2.2（已 done）：iphone/ 工程骨架 + 主界面布局
- [Source: \_bmad-output/implementation-artifacts/2-3-导航架构搭建.md] — Story 2.3（已 done）：导航架构 + AppCoordinator
- [Source: \_bmad-output/implementation-artifacts/2-4-apiclient-封装.md] — **直接前置 story**（已 done）：APIClient / APIError / Endpoint / APIResponse / URLSessionProtocol
- [Source: \_bmad-output/implementation-artifacts/1-2-cmd-server-入口-配置加载-gin-ping.md] — server 端 `/ping` 实装（路径根路径，不走 `/api/v1`）
- [Source: \_bmad-output/implementation-artifacts/1-4-version-接口.md] — server 端 `/version` 实装（commit / builtAt 字段命名小驼峰）
- [Source: server/internal/app/bootstrap/router.go:30-46] — server 端运维端点路由实际位置
- [Source: server/internal/app/http/handler/version_handler.go:12-15] — VersionResponse 字段命名 `commit` / `builtAt`
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#4 项目目录建议] — 目录路径锁定
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#5.3 UseCase 层] — UseCase 职责：对一个明确业务动作进行封装
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#18.1 首选技术路线] — `URLSession` + `async/await`（本 story 严格遵守）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#18.2 依赖注入] — 通过 AppContainer 管理：APIClient / Repositories / UseCases
- [Source: docs/宠物互动App_V1接口设计.md#2.4 通用响应结构] — envelope 字段定义 `{code, message, data, requestId}`
- [Source: docs/宠物互动App_V1接口设计.md#3 错误码定义] — 32 个业务码（1001..7002）
- [Source: docs/宠物互动App_总体架构设计.md] — REST + WebSocket 协议组合
- [Source: CLAUDE.md "Tech Stack（新方向）"] — iOS 端 = Swift + SwiftUI + URLSession（不引入 Alamofire）
- [Source: CLAUDE.md "Repo Separation（重启阶段过渡态）"] — `iphone/` 是新方向 iOS 工程目录
- [Source: docs/lessons/2026-04-25-swift-explicit-import-combine.md] — **必读**：production 文件首次使用 framework 必须显式 import；本 story `AppContainer.swift` 显式 import Combine
- [Source: docs/lessons/2026-04-25-swiftui-zstack-overlay-bottom-cta.md] — Story 2.2 layout 教训；本 story 不接 UI，但提醒 versionLabel 当前 layout（footer 行）不能改
- [Source: docs/lessons/2026-04-26-jsondecoder-encoder-thread-safety.md] — Story 2.4 review lesson：APIClient 用 makeDecoder/makeEncoder factory；本 story PingUseCase 不持有 coder
- [Source: docs/lessons/2026-04-26-urlprotocol-stub-global-state.md] — Story 2.4 review lesson：URLProtocol stub 静态字段加锁 + 文档化；本 story `PingStubURLProtocol` 严格继承
- [Source: docs/lessons/2026-04-26-url-trailing-slash-concat.md] — Story 2.4 review lesson：baseURL trailing slash 边界吸收；本 story `AppContainer` baseURL 无 trailing slash + path 必 `/` 开头
- [Source: docs/lessons/2026-04-26-urlprotocol-session-local-vs-global.md] — Story 2.4 review lesson：仅 session-local 注入 stub；本 story `PingUseCaseIntegrationTests` 严格遵守
- [Source: \_bmad-output/implementation-artifacts/2-3-导航架构搭建.md#Lesson Learned 沉淀] — Story 2.3 a11y identifier 父容器传播教训；本 story 不接 UI，无直接交集
- [Source: \_bmad-output/planning-artifacts/epics.md#Story 2.6] — 后续 ErrorPresenter 消费本 story 触发的 APIError（如果未来选择把 PingUseCase 改抛错的话）
- [Source: \_bmad-output/planning-artifacts/epics.md#Story 2.8] — 与 Story 2.5 版本号显示同区域（角落 dev info），Story 2.8 dev 重置按钮预留位
- [Source: \_bmad-output/planning-artifacts/epics.md#Story 2.9] — 后续 LaunchingView 落地时 ping 调用从 RootView.task 移到 LaunchingView.onAppear（视决策）
- [Source: \_bmad-output/planning-artifacts/epics.md#Epic 3 / Story 3.1] — 节点 1 跨端集成测试场景：iOS Simulator + server 真实联调 ping E2E（本 story 是该 E2E 的 iOS 端实装基础）
- [Source: \_bmad-output/planning-artifacts/epics.md#Epic 3 / Story 3.2] — 节点 1 demo 验收：`GET /ping` 返回成功 + App 与 Server 至少一条真实联调链路（ping + /version）—— 本 story 完成后该联调即可走通

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]（dev-story workflow / Opus 4.7 1M context）

### Debug Log References

- `xcodegen generate`（在 `iphone/` 下执行；接收 `project.yml` info.properties 改动 + 8 个新 .swift 文件 references）
- `xcodebuild test -project iphone/PetApp.xcodeproj -scheme PetApp -destination 'platform=iOS Simulator,name=iPhone 17,OS=latest' -only-testing:PetAppTests`：48 case 0 失败
- `xcodebuild test -project iphone/PetApp.xcodeproj -scheme PetApp -destination 'platform=iOS Simulator,name=iPhone 17,OS=latest' -only-testing:PetAppUITests`：4 case 0 失败
- 总计 52 case 0 失败（PetAppTests 48 + PetAppUITests 4）

### Completion Notes List

- AC1 ✅：`PingResult` value type 落地（`reachable: Bool` / `serverCommit: String?`，Equatable）；新建 `iphone/PetApp/Features/Home/UseCases/PingResult.swift`
- AC2 ✅：`PingEndpoints` 工厂 + `VersionResponse` Decodable + `PingResponse = Empty` typealias 落地；path 严格 `"/ping"` / `"/version"`，requiresAuth: false；新建 `iphone/PetApp/Features/Home/UseCases/PingEndpoints.swift`
- AC3 ✅：`PingUseCaseProtocol: Sendable` + `DefaultPingUseCase: PingUseCaseProtocol` 落地；execute() 不抛错，三态 happy / partial-degrade / offline 串行返回；新建 `iphone/PetApp/Features/Home/UseCases/PingUseCase.swift`
- AC4 ✅：`HomeViewModel` 扩展（追加 `init(pingUseCase:)` / `bind(pingUseCase:)` / `start()` / `applyPingResult` / `nonisolated static readAppVersion`）；老 init 签名 / 默认值一字不改，老测试 `testHardcodedDefaultStateMatchesStorySpec` 仍 PASS
- AC5 ✅：`HomeView.versionLabel` 模板未改（仍是 `"v\(viewModel.appVersion) · \(viewModel.serverInfo)"`），文案变化通过 `viewModel.serverInfo` 推送实现
- AC6 ✅：`AppContainer: ObservableObject` 落地，持有 `apiClient: APIClientProtocol`，提供 `makePingUseCase()` 工厂；默认 baseURL `http://localhost:8080`；新建 `iphone/PetApp/App/AppContainer.swift`
- AC7 ✅：`RootView` 追加 `@StateObject private var container = AppContainer()` + `.task { homeViewModel.bind(...); await homeViewModel.start() }`；现有 `.onAppear` / `.fullScreenCover` / `wireHomeViewModelClosures` 不动
- AC8 ✅：`iphone/project.yml` info.properties 加 `CFBundleShortVersionString: "1.0.0"` + `CFBundleVersion: "1"`；其它字段未变
- AC9 ✅：单元测试 13 case（PingUseCaseTests 5 + HomeViewModelPingTests 8）全部 PASS
- AC10 ✅：集成测试 2 case（happy + ping-failure）全部 PASS；新建专用 `PingStubURLProtocol`（按 path 路由），不动 Story 2.4 的 `StubURLProtocol`
- AC11 ✅：`xcodegen generate` 已在 iphone/ 下执行，`PetApp.xcodeproj/project.pbxproj` 接收 8 个新 .swift 文件 + Info.plist properties 更新
- AC12 ✅：全套 52 case PASS（PetAppTests 48 含老 22 + 新 13 单元 + 2 集成 + Story 2.2/2.3/2.4 老 case；PetAppUITests 4 case；老 `HomeViewModelTests.testHardcodedDefaultStateMatchesStorySpec` 通过验证 hardcode 默认值未变）
- AC13 ✅：`git status` 验证仅以下文件改动：`iphone/` 内 6 modified + 5 untracked + sprint-status.yaml + 本 story 文件；**`ios/` / `server/` / `CLAUDE.md` / `docs/` 全部零改动**
- AC14 ✅：import hygiene 严格按 AC14 表格落地（PingResult / PingEndpoints / PingUseCase 仅 Foundation；AppContainer Foundation+Combine；HomeViewModel Foundation+Combine 不变）
- 范围红线 ✅：未动 `ios/` / `server/`；未动 Story 2.4 networking 5 文件；未动 Story 2.4 测试 4 文件；未改 `AccessibilityID.Home.versionLabel` 字符串值；未引第三方 mock / DI 框架
- Lesson learned 应用：
  1. `import Combine` 显式（HomeViewModel 已有；AppContainer 新增） — 应用 lesson 2026-04-25-swift-explicit-import-combine.md
  2. JSONDecoder/Encoder 不持有 — Story 2.4 APIClient 已修复，PingUseCase 不持有 coder（lesson 2026-04-26-jsondecoder-encoder-thread-safety.md）
  3. URLProtocol stub 仅 session-local 注入 — `PingUseCaseIntegrationTests` 用 `URLSessionConfiguration.protocolClasses`，不调 registerClass（lesson 2026-04-26-urlprotocol-session-local-vs-global.md）
  4. URLProtocol stub static 字段 NSLock 保护 + snapshot 原子读 — `PingStubURLProtocol` 严格继承（lesson 2026-04-26-urlprotocol-stub-global-state.md）
  5. URL 拼接 trailing slash — AppContainer baseURL `http://localhost:8080`（无 trailing）+ PingEndpoints path 必 `/` 开头（lesson 2026-04-26-url-trailing-slash-concat.md）
- 后续 follow-up：PingUseCase catch 块当前 swallow error；待 Story 2.7 logger 接入后改为 `logger.warning(...)`（已在 PingUseCase doc comment 标记）

### File List

新建 production（4）：
- `iphone/PetApp/Features/Home/UseCases/PingResult.swift`
- `iphone/PetApp/Features/Home/UseCases/PingEndpoints.swift`
- `iphone/PetApp/Features/Home/UseCases/PingUseCase.swift`
- `iphone/PetApp/App/AppContainer.swift`

修改 production（2）：
- `iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift`（追加 init / start / bind / applyPingResult / readAppVersion；老 init 不动）
- `iphone/PetApp/App/RootView.swift`（追加 container `@StateObject` + `.task` 触发 ping；老逻辑不动）

新建 test（4）：
- `iphone/PetAppTests/Features/Home/UseCases/PingUseCaseTests.swift`（5 case）
- `iphone/PetAppTests/Features/Home/UseCases/PingUseCaseIntegrationTests.swift`（2 case）
- `iphone/PetAppTests/Features/Home/UseCases/MockAPIClient.swift`（手写 mock）
- `iphone/PetAppTests/Features/Home/UseCases/PingStubURLProtocol.swift`（按 path 路由的 URLProtocol stub）
- `iphone/PetAppTests/Features/Home/HomeViewModelPingTests.swift`（8 case，含 `StubPingUseCase` 内嵌 mock）

修改 config（3）：
- `iphone/project.yml`（仅 `info.properties` 加 `CFBundleShortVersionString` + `CFBundleVersion` 两行）
- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen auto-regen 接收新文件 references；无手工编辑）
- `iphone/PetApp/Resources/Info.plist`（xcodegen auto-regen 落 properties）

修改 BMAD（2）：
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（status: ready-for-dev → in-progress → review）
- `_bmad-output/implementation-artifacts/2-5-ping-调用-主界面显示-server-version-信息.md`（本文件：勾选 Tasks/Subtasks + 填 Dev Agent Record + Status: in-progress → review）

## Change Log

| 日期 | 变更 | 操作者 |
|---|---|---|
| 2026-04-25 | 创建 Story 2.5 上下文（bmad-create-story） | claude-opus-4-7[1m] |
| 2026-04-26 | dev-story 实装：4 production + 6 test 文件落地，AC1-14 全 PASS，52 case 0 失败；Status → review | claude-opus-4-7[1m] |
