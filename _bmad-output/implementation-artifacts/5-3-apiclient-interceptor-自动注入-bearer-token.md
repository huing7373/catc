# Story 5.3: APIClient interceptor 自动注入 Bearer token

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iOS 开发,
I want APIClient 在 `request<T>(_ endpoint:)` 内置一个 token 注入步骤 —— 按 `endpoint.requiresAuth` 决策：`true` 时从注入的 `KeychainStoreProtocol` 读 `KeychainKey.authToken.rawValue`，存在则给 `URLRequest` header 写 `Authorization: Bearer <token>`，不存在则**直接抛 `APIError.unauthorized`**（不发请求，不浪费一次网络往返）；`false` 时（如 `/auth/guest-login` / `/ping` / `/version`）跳过注入，保持现有行为零回归,
so that 业务层（`AuthRepository` / 未来的 `HomeRepository` / `ChestRepository` 等）调 API 不必手动管理 token，符合 V1接口设计 §2.3 钦定的 `Authorization: Bearer <token>` 鉴权约定，且 Story 5.4 的 401 静默重登有一个稳定的 token 注入点可以挂钩.

## 故事定位（Epic 5 第三条 = 节点 2 iOS 端鉴权基础设施；上承 5.1 / 5.2，下启 5.4 / 5.5）

这是 Epic 5 的**鉴权底座**实装 story —— 把 Story 2.4 / 5.2 留下的 `Endpoint.requiresAuth: Bool` 字段从"占位"激活成"真实生效"。**直接前置**全部 done：

- **Story 2.4 (`done`)** 已落地 `APIClient` + `Endpoint`：
  - `iphone/PetApp/Core/Networking/APIClient.swift`：`final class APIClient: APIClientProtocol`，`request<T>(_ endpoint: Endpoint) async throws -> T`；内部 `buildURLRequest(_:)` 已在 line 191-192 注释 "requiresAuth 暂不处理（Epic 5 AuthInterceptor 落地时插入）"，本 story **激活**该位
  - `iphone/PetApp/Core/Networking/Endpoint.swift`：`Endpoint.requiresAuth: Bool` 字段，所有现有调用方都已显式传 `requiresAuth: false`
  - `iphone/PetApp/Core/Networking/APIError.swift`：`APIError.unauthorized` case，本 story **复用**该 case 表达"requiresAuth=true 但本地无 token"
- **Story 5.1 (`done`)** 已落地 `KeychainStoreProtocol` + `KeychainServicesStore` + `KeychainKey.authToken`：
  - `iphone/PetApp/Core/Storage/KeychainStore.swift`：`KeychainStoreProtocol.get(forKey: String) throws -> String?` 协议方法 —— 本 story **直接调** `keychain.get(forKey: KeychainKey.authToken.rawValue)`
  - `iphone/PetApp/Core/Storage/KeychainServicesStore.swift`：生产实装；不存在 key 时 `get` 返回 `nil`（不抛 error）
- **Story 5.2 (`done`)** 已落地 `GuestLoginUseCase` + `AuthEndpoints.guestLogin` + `SessionStore`：
  - `iphone/PetApp/Features/Auth/UseCases/GuestLoginUseCase.swift`：登录成功后 `try keychainStore.set(response.token, forKey: KeychainKey.authToken.rawValue)` 写 token —— 本 story 的 interceptor 即从此读
  - `iphone/PetApp/Features/Auth/UseCases/AuthEndpoints.swift`：`AuthEndpoints.guestLogin(...).requiresAuth == false`（登录接口本身免 auth）—— 本 story **保留**该值，证明 false 路径行为零回归
  - `iphone/PetApp/App/AppContainer.swift`：`apiClient: APIClientProtocol` + `keychainStore: KeychainStoreProtocol` 已就绪 —— 本 story **改 init 注入路径**：`APIClient(baseURL:, session:, keychainStore:)` 让 keychain 同时提供给 APIClient 用作 token 源

**本 story 的核心动作**（顺序无关，可分批落地）：

1. **修改** `iphone/PetApp/Core/Networking/APIClient.swift`：
   - 给 `init` 加 `keychainStore: KeychainStoreProtocol?` 参数（**Optional 默认 `nil`**）：
     - `nil` 时：interceptor 完全跳过 —— 任何 `requiresAuth: true` 的 endpoint 都抛 `APIError.unauthorized`（防御性 —— 等同于"有 endpoint 要 auth 但 client 没接 keychain，配置错误"）
     - 非 nil 时：`buildURLRequest(_:)` 内按 `endpoint.requiresAuth` 决策注入
   - 修改 `buildURLRequest(_:)`：在现有 body 编码完之后、`return request` 之前插入新代码块：
     ```swift
     if endpoint.requiresAuth {
         guard let keychainStore else { throw APIError.unauthorized }
         let token = try? keychainStore.get(forKey: KeychainKey.authToken.rawValue)
         guard let token, !token.isEmpty else { throw APIError.unauthorized }
         request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
     }
     ```
   - **保留**既有 `request<T>` 方法签名 / 决策树 / `makeDecoder()` / `makeEncoder()` / `normalize()` 全部既有逻辑，**仅追加** keychain 注入 5 行
2. **修改** `iphone/PetApp/App/AppContainer.swift`：
   - 把 `convenience init()` 内 `APIClient(baseURL:)` 改为 `APIClient(baseURL:, keychainStore: KeychainServicesStore())` —— 这样生产链路 APIClient 默认带 keychain
   - 优化（可选）：让 `APIContainer` 把自己持有的 `keychainStore` 传给 APIClient（共享同一 instance），而不是 APIClient 自己 `new KeychainServicesStore()` —— **优先选这个**，避免双 instance 持有带来的 namespace 不一致风险（`KeychainServicesStore(service:)` 默认 namespace 一致，但显式共享更稳）
   - 具体改法：`AppContainer.convenience init()` 先 `let store = KeychainServicesStore()`，然后 `APIClient(baseURL: ..., keychainStore: store)` + `self.init(apiClient: ..., keychainStore: store)`
3. **不**新建 `AuthInterceptor.swift` 文件实体（虽然 iOS 架构 §4 列出了 `Core/Networking/AuthInterceptor.swift`）：
   - 当前阶段 token 注入逻辑只有 5 行，独立成 protocol + 实装会引入 indirection 但收益为零（**YAGNI**）
   - 单元测试已能完整覆盖 token 注入路径（mock keychain → APIClient.request → 验 URLRequest header）
   - 保留 future Story 用 —— 当 token 注入逻辑变复杂（如 retry / cache / 多 token 类型），再 spike 抽出 `AuthInterceptor` 协议（与本 story 的"5 行内联"零冲突 —— 协议化只是 refactor，行为契约不变）
   - **如果 dev 实装时强烈倾向独立 AuthInterceptor 类**：允许，但必须把所有"Story 5.3 落地"的引用改成"Story 5.3 落地（含 `AuthInterceptor.swift` 实体）"，并解释收益 / 维护成本权衡（≥ 100 字 doc comment）。否则 **默认走"5 行内联"路径**
4. **新建** `iphone/PetAppTests/Core/Networking/APIClientAuthInjectionTests.swift`：
   - ≥ 4 case：happy（requiresAuth=true + token 存在 → header 写）/ happy（requiresAuth=false → header 无注入）/ edge（requiresAuth=true + token 不存在 → 抛 unauthorized 不发请求）/ edge（并发 100 请求都正确注入）
   - 复用 Story 2.4 落地的 `MockURLSession` + Story 2.8 落地的 `MockKeychainStore`（两个都已经存在，**不**新建）
5. **新建测试**：单测同一文件内补一个 case 验"requiresAuth=true + APIClient.init 时未注 keychain → 抛 unauthorized 不发请求"（防御性测 keychainStore Optional nil 分支）
6. **不动**：
   - `Endpoint.swift`（`requiresAuth: Bool` 字段已存在；本 story 仅消费）
   - `APIError.swift`（`APIError.unauthorized` 已存在；本 story 复用）
   - `Endpoint.requiresAuth: false` 的全部既有调用方：`PingEndpoints.ping` / `PingEndpoints.version` / `AuthEndpoints.guestLogin`（**保留** false 不改 —— 证明"false 路径行为零回归"）
   - `APIClientProtocol` 协议签名（`request<T>(_:) async throws -> T` 不动；keychain 是构造期注入，不进协议）
   - `AppLaunchStateMachine` / `RootView` / `SessionStore` / `GuestLoginUseCase` / `AuthRepository`（本 story 是网络层底座激活，业务层零改动）
   - `KeychainStoreProtocol` 协议（仅消费 `get(forKey:)` 一个方法）
   - `iphone/scripts/` / `iphone/project.yml` / `iphone/.gitignore`（所有新增文件靠 Story 2.2 既有 `sources: - PetApp` / `sources: - PetAppTests` glob 自动纳入）
7. **测试范围（基础设施）**：
   - **不**新增第三方 mock 库（与 ADR-0002 §3.1 一致）
   - 复用 Story 2.4 `MockURLSession`（已含 `invocations: [URLRequest]`，可断言 header）
   - 复用 Story 2.8 `MockKeychainStore`（继承 `MockBase` + `KeychainStoreProtocol` + `getStubResult` 字段）
   - **不**新建 `MockAuthInterceptor` —— interceptor 是 APIClient 内联逻辑，没有独立协议

**不涉及**：

- **token 失效（401）静默重登**：归 Story 5.4（`SilentReloginUseCase`）。本 story 仅"读 keychain → 注入 header"单向流；server 返 401 时 APIClient 已经抛 `APIError.unauthorized`（Story 2.4 既有逻辑），但本 story **不**在抛错之后做任何"自动重登"动作 —— 那是 Story 5.4 的责任
- **token 刷新 / refresh token**：MVP 不实装（V1 §4.1 钦定 `/auth/guest-login` 是幂等的，过期直接重新 guest-login；refresh token 是 Post-MVP / FR3）
- **服务端任何改动**：本 story 是纯客户端实装；`server/` 全程零改动
- **`/auth/guest-login` 调用方式改变**：本 story **保留** `requiresAuth: false`，登录接口仍走旧路径（直接发请求，无 header 注入）—— 证明"false 路径完全零回归"
- **业务层 endpoint 的 requiresAuth 翻成 true**：节点 2 阶段 Story 5.5 落地 `LoadHomeUseCase` 时会建 `HomeEndpoints.home(...).requiresAuth = true` —— 那时本 story 的 interceptor 即自动生效；本 story **不**预建 `HomeEndpoints` 实体
- **WebSocket 连接的 token 注入**：归 Epic 10（`WebSocketClient`）；REST 与 WS 是不同协议，本 story 不预设 WS interceptor
- **iOS UITest**：本 story 是 APIClient 内部行为改动 + 单测可完整覆盖 + RootView 调用面零改动 —— **不**新建任何 UITest；Story 5.2 落地的 `UITEST_SKIP_GUEST_LOGIN` 旁路继续保护既有 UITest
- **`ios/` 旧产物目录**：CLAUDE.md "Repo Separation" + ADR-0002 §3.3 既有约束。最终 `git status` 须确认 `ios/` 下零改动

**范围红线**：

- **不动 `ios/`**：CLAUDE.md "Repo Separation" + ADR-0002 §3.3。最终 `git status` 须 `ios/` 下零改动
- **不动 `server/`**：本 story 是纯 iPhone 端网络层实装
- **不动 `iphone/scripts/` / `iphone/project.yml` / `iphone/.gitignore`**：新文件靠既有 sources glob 自动纳入；**0 yml 改动**
- **不引入第三方依赖**：`Foundation` 即可；keychain access 已通过 Story 5.1 落地 `KeychainServicesStore` 完成 → 本 story 仅调协议方法
- **不动 `APIClientProtocol` 协议签名**：仅 `APIClient`（具体类）的 init 签名追加 `keychainStore` Optional 参数；协议方法零改动 —— 让所有持 `APIClientProtocol` 引用的代码（如 `DefaultAuthRepository.init(apiClient: APIClientProtocol)`）零改动
- **不动 `Endpoint.requiresAuth: Bool` 字段定义**：仅消费既有字段，**不**改字段类型 / 默认值 / 命名
- **不动既有 endpoint 实例的 `requiresAuth` 值**：`PingEndpoints.ping/version` / `AuthEndpoints.guestLogin` 全部保持 `requiresAuth: false`（证明 false 路径零回归）
- **`APIClient.init` 签名向后兼容**：`keychainStore: KeychainStoreProtocol? = nil` 必须有默认值 nil —— 让 Story 2.5 落地的 `APIClient(baseURL: baseURL)` / Story 2.4 落地的 `APIClient(baseURL: baseURL, session: session)` 调用方零改动；测试中既有 `APIClient(baseURL:, session:)` 调用全部继续工作（`keychainStore` 走默认 nil → requiresAuth=true 时仍抛 unauthorized 但**不**用上 keychain；既有测试 endpoint 全部 `requiresAuth: false` 因此不触发 → 零回归）
- **token 不存在时一定不发请求**：`requiresAuth: true` + Keychain 无 token → 抛 `APIError.unauthorized` 必须发生在 `buildURLRequest(_:)` 内 / `session.data(for:)` 调用**之前**；测试要断言 `MockURLSession.invocations.count == 0`（不浪费一次网络往返、不让 server 看到伪造请求）
- **header 名 / 格式严格按 V1 §2.3**：`"Authorization"` header name + `"Bearer <token>"` value（其中 `<token>` 直接拼 keychain 读到的字符串，**不**额外 base64 编码 / 加引号 / 加 trailing space）
- **空字符串 token 视同不存在**：`token.isEmpty` 时与 nil 同分支抛 `unauthorized` —— 防御性，避免 keychain 里某个 bug 写入空串导致发出 `Authorization: Bearer ` 这种畸形 header
- **keychain.get 抛错时降级为"无 token"**：`try? keychainStore.get(...)` 而非 `try` —— 即使底层 Keychain access 出错（极少见，如沙箱权限问题），也按 unauthorized 处理；**不**把底层 KeychainError 透传给上层（业务层只关心"有 token / 没 token"，KeychainError 是基础设施细节）
- **token 不缓存到 APIClient 内**：每次请求都从 keychain 重新读 —— 让 Story 5.4 的"重登后立刻刷新 token"立即对所有后续请求生效（**不**需要 invalidate cache 操作）；keychain.get 在 simulator + 真机都是 ~0.1ms 开销，可忽略
- **不修改既有 `APIClient.normalize` / `makeDecoder` / `makeEncoder` / `request<T>` 决策树**：仅 `init` + `buildURLRequest` 两点局部改动

## Acceptance Criteria

**AC1 — `APIClient.init` 追加 `keychainStore: KeychainStoreProtocol?` 参数（默认 nil，向后兼容）**

修改 `iphone/PetApp/Core/Networking/APIClient.swift`：

```swift
public final class APIClient: APIClientProtocol {
    private let baseURL: URL
    private let session: URLSessionProtocol
    private let keychainStore: KeychainStoreProtocol?  // ← 新增字段：Optional，默认 nil

    public init(
        baseURL: URL,
        session: URLSessionProtocol = URLSession.shared,
        keychainStore: KeychainStoreProtocol? = nil   // ← 新增参数：默认 nil（向后兼容）
    ) {
        self.baseURL = Self.normalize(baseURL)
        self.session = session
        self.keychainStore = keychainStore
    }

    // makeDecoder / makeEncoder / request<T> / normalize 全部不动 ↓ ↓ ↓
}
```

**具体行为要求**：
- 字段名 `keychainStore` 与 Story 5.2 落地的 `AppContainer.keychainStore` 同名；语义一致（`KeychainStoreProtocol` 实例）
- 字段是 `let` + `private` —— APIClient 持有期间不变；外部不可读 / 不可改
- `keychainStore: KeychainStoreProtocol? = nil` 默认 nil：让 Story 2.4 / 2.5 既有测试中 `APIClient(baseURL:)` / `APIClient(baseURL:, session:)` 调用方零改动 —— Swift 默认参数语义保证
- **不**改 `APIClientProtocol`：keychain 是构造期注入，运行期协议方法签名 `request<T>(_:) async throws -> T` 不变 —— 让所有持 `APIClientProtocol` 引用的下游（如 `DefaultAuthRepository(apiClient: APIClientProtocol)`）零改动
- **不**用 `keychainStore: KeychainStoreProtocol`（非 Optional + 必填）：那会破坏既有 `APIClient(baseURL:)` 调用方；Optional + 默认 nil 是 backward-compatible 唯一路径
- 也**不**给字段加 `setter` 或 mutating method：keychain 是构造期注入；运行时换 keychain 无业务场景需求

**AC2 — `buildURLRequest(_:)` 内插入 token 注入步骤**

修改 `iphone/PetApp/Core/Networking/APIClient.swift` 的 `buildURLRequest(_:)` 方法：

```swift
private func buildURLRequest(_ endpoint: Endpoint) throws -> URLRequest {
    // baseURL / path 拼接 / URL 校验：保持 Story 2.4 既有逻辑不变 ↓ ↓ ↓
    guard let url = URL(string: baseURL.absoluteString + endpoint.path) else {
        throw APIError.network(
            underlying: NSError(
                domain: "APIClient",
                code: -3,
                userInfo: [NSLocalizedDescriptionKey: "Invalid URL: \(baseURL)\(endpoint.path)"]
            )
        )
    }

    var request = URLRequest(url: url)
    request.httpMethod = endpoint.method.rawValue
    request.setValue("application/json", forHTTPHeaderField: "Accept")

    if let body = endpoint.body {
        do {
            request.httpBody = try makeEncoder().encode(body)
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        } catch {
            throw APIError.decoding(underlying: error)
        }
    }

    // ↓ ↓ ↓ Story 5.3 新增：按 requiresAuth 决策注入 Authorization header ↓ ↓ ↓
    //
    // 决策树：
    //   1. requiresAuth == false → 跳过（保持 false 路径行为零回归）
    //   2. requiresAuth == true + keychainStore == nil → throw .unauthorized（防御性）
    //   3. requiresAuth == true + keychainStore.get 抛错 → 降级为"无 token" → throw .unauthorized
    //   4. requiresAuth == true + token == nil 或空串 → throw .unauthorized
    //   5. requiresAuth == true + token 非空 → 写 "Authorization: Bearer <token>" header
    if endpoint.requiresAuth {
        guard let keychainStore else {
            throw APIError.unauthorized
        }
        // try? 而非 try：keychain access 失败（极少见沙箱问题）一律降级为"无 token"
        // → 抛 unauthorized；不把基础设施细节 KeychainError 透传给上层业务
        let token = try? keychainStore.get(forKey: KeychainKey.authToken.rawValue)
        guard let token, !token.isEmpty else {
            throw APIError.unauthorized
        }
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
    }

    return request
}
```

**具体行为要求**：
- 注入逻辑在 body 编码**之后**、`return request` **之前**：让 body 编码失败优先抛 `APIError.decoding`，符合 Story 2.4 既有决策树语义（编码错误优先于鉴权错误）
- header 名 **严格** `"Authorization"`（大写 A，与 V1 §2.3 + HTTP/1.1 spec 一致）；header value **严格** `"Bearer <token>"`（中间一个空格，无尾随空格）
- 字符串拼接用 `"Bearer \(token)"`：Swift 标准插值；不需要 `String(format:)` 或 `+` 运算
- `try? keychainStore.get(...)` 降级语义：让 Story 5.1 落地的 `KeychainError.osStatus(...)` / `unexpectedDataFormat` 不直接透传给业务层 —— 业务层只关心"鉴权失败"（用 `APIError.unauthorized` 表达），底层 keychain 错的 detail 已经超出业务关切
- `token.isEmpty` 视同 nil：防御性 —— 即使 keychain 里某个 bug 写入空串，也不会拼出 `Authorization: Bearer ` 畸形 header；与 Story 5.2 `GuestLoginUseCase` 内 `existing.isEmpty` 视同不存在的设计同精神
- **不**写入 token 到 `URLCache` / `URLSessionConfiguration.httpAdditionalHeaders`：让每次请求重新读 keychain → Story 5.4 重登后无需 invalidate cache → 行为简单
- **不**做 token 格式校验（如 JWT 三段式 base64 检查）：客户端不解析 token；server 才校验签名，不重复劳动
- 注入逻辑必须发生在 `session.data(for:)` 调用**之前**：抛错时 MockURLSession.invocations.count == 0（保证不浪费网络往返）

**AC3 — `Endpoint.requiresAuth: false` 路径完全零回归**

测试与运行时双重验证：
- `AuthEndpoints.guestLogin(...).requiresAuth == false` —— 保持 Story 5.2 落地不变；本 story 不修改 `AuthEndpoints.swift`
- `PingEndpoints.ping().requiresAuth == false` / `PingEndpoints.version().requiresAuth == false` —— 保持 Story 2.5 落地不变；本 story 不修改 `PingEndpoints.swift`
- 所有 Story 2.4 / 2.5 / 5.2 既有的 APIClient 单元测试 / 集成测试全部继续 0 失败（既有 endpoint 全部 `requiresAuth: false` → AC2 决策树第 1 步直接跳过 → 行为完全等同 Story 2.4 落地版）

具体验证方式：
- 跑 `bash iphone/scripts/build.sh --test`：既有 APIClientTests / APIClientIntegrationTests / PingUseCaseTests / GuestLoginUseCaseTests 全部 0 失败
- 跑 `bash iphone/scripts/build.sh --uitest`：既有 HomeUITests / NavigationUITests / KeychainPersistenceUITests 全部 0 失败（Story 5.2 落地的 `UITEST_SKIP_GUEST_LOGIN` 旁路继续保护这些测试）

**具体行为要求**：
- **不**修改 `AuthEndpoints.swift` / `PingEndpoints.swift` / 任何 Story 2.5 / 5.2 既有 production 文件（除 `APIClient.swift` + `AppContainer.swift` 两点局部改动）
- 既有测试 `APIClient(baseURL:)` / `APIClient(baseURL:, session:)` 不传 keychainStore → 默认 nil → false 路径 = 完全等同 Story 2.4 落地版

**AC4 — `AppContainer` 把 keychainStore 共享给 APIClient（避免双 instance）**

修改 `iphone/PetApp/App/AppContainer.swift` 的 `convenience init()`：

```swift
public convenience init() {
    let baseURL = AppContainer.resolveDefaultBaseURL(from: Bundle.main)
    // Story 5.3 改动：先建一个 KeychainServicesStore 实例，让 AppContainer 与 APIClient
    // 共享同一 keychain（namespace 一致；避免 APIClient 内 new 一个新 instance 引发未来
    // 万一调整 namespace 时双源不一致的风险）。
    let keychainStore = KeychainServicesStore()
    self.init(
        apiClient: APIClient(baseURL: baseURL, keychainStore: keychainStore),
        keychainStore: keychainStore
    )
}
```

**具体行为要求**：
- `convenience init()` 内先 `let keychainStore = KeychainServicesStore()` 一次 —— 然后 `APIClient(baseURL:, keychainStore:)` 与 `self.init(apiClient:, keychainStore:)` 都用同一 instance
- **保留** `init(apiClient:keychainStore:)` 主 init 既有签名（Story 2.8 / 5.1 落地），只是 `convenience init()` 内部走新流程
- 测试场景（`AppContainerTests`）走 `init(apiClient:keychainStore:)` 注入式入口 —— 测试方可自定义 mock APIClient + mock keychain；本 story **不**改既有 `AppContainerTests` 中任何调用（向后兼容）
- 默认 `KeychainServicesStore()` 实例化与 Story 5.1 落地一致 —— 节点 2 真实运行：APIClient 拿到的 keychain 即是 GuestLoginUseCase 写 token 的同一个 keychain
- **不**改 `init(apiClient:keychainStore:)` 主 init 内部逻辑（不在 init 内强制把 keychain 二次注入到 apiClient —— 测试场景下传入 mock APIClient 可能本身已经持有 mock keychain 或不需要）

**AC5 — 单元测试覆盖（≥ 4 case）**

新建 `iphone/PetAppTests/Core/Networking/APIClientAuthInjectionTests.swift`：

```swift
// APIClientAuthInjectionTests.swift
// Story 5.3 AC5: APIClient interceptor 自动注入 Bearer token 单元测试.
//
// 与 Story 2.4 APIClientTests 的区别：
// - APIClientTests 覆盖 envelope decode / status code / business code 决策；本文件
//   专注 token 注入路径（requiresAuth=true/false × token 存在/不存在 × 并发安全）.
//
// 复用：
// - MockURLSession（Story 2.4 落地，含 invocations: [URLRequest]）
// - MockKeychainStore（Story 2.8 落地，#if DEBUG，继承 MockBase）

import XCTest
@testable import PetApp

#if DEBUG

@MainActor
final class APIClientAuthInjectionTests: XCTestCase {

    private let baseURL = URL(string: "http://localhost:8080")!

    // 解码目标（mimic Story 2.4 测试中 PingResponseMock 模式）
    private struct EmptyDataMock: Decodable, Equatable {
        // 解码空对象 {} 的占位
    }

    private func makeHTTPResponse(statusCode: Int) -> HTTPURLResponse {
        HTTPURLResponse(
            url: baseURL,
            statusCode: statusCode,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        )!
    }

    private func makeStubResponseBody() -> Data {
        // 成功 envelope，data = {}
        """
        {"code":0,"message":"ok","data":{},"requestId":"req_abc"}
        """.data(using: .utf8)!
    }

    // MARK: - case#1 (happy)：requiresAuth=true + Keychain 有 token → 请求 URL header 含 Authorization: Bearer xxx
    func testInjectsAuthorizationHeaderWhenRequiresAuthAndTokenExists() async throws {
        let session = MockURLSession()
        session.stubbedResponse = (makeStubResponseBody(), makeHTTPResponse(statusCode: 200))

        let keychain = MockKeychainStore()
        keychain.getStubResult = .success("test-jwt-token-abc")

        let client = APIClient(baseURL: baseURL, session: session, keychainStore: keychain)
        let endpoint = Endpoint(path: "/api/v1/home", method: .get, requiresAuth: true)

        let _: EmptyDataMock = try await client.request(endpoint)

        // 1. session.data(for:) 被调过一次
        XCTAssertEqual(session.invocations.count, 1)
        // 2. 该请求的 Authorization header 严格等于 "Bearer test-jwt-token-abc"
        let request = session.invocations.first!
        XCTAssertEqual(request.value(forHTTPHeaderField: "Authorization"), "Bearer test-jwt-token-abc")
        // 3. keychain.get(forKey:) 被调过 1 次（精确次数）
        XCTAssertEqual(keychain.callCount(of: "get(forKey:)"), 1)
    }

    // MARK: - case#2 (happy)：requiresAuth=false → header 无 Authorization（即使 keychain 有 token）
    func testDoesNotInjectAuthorizationHeaderWhenRequiresAuthFalse() async throws {
        let session = MockURLSession()
        session.stubbedResponse = (makeStubResponseBody(), makeHTTPResponse(statusCode: 200))

        let keychain = MockKeychainStore()
        keychain.getStubResult = .success("test-jwt-token-abc")  // 即使有 token

        let client = APIClient(baseURL: baseURL, session: session, keychainStore: keychain)
        let endpoint = Endpoint(path: "/api/v1/auth/guest-login", method: .post, requiresAuth: false)

        let _: EmptyDataMock = try await client.request(endpoint)

        // 1. session.data(for:) 被调过一次
        XCTAssertEqual(session.invocations.count, 1)
        // 2. 该请求 Authorization header 不存在（nil）
        let request = session.invocations.first!
        XCTAssertNil(request.value(forHTTPHeaderField: "Authorization"))
        // 3. keychain.get(forKey:) 一次都不调（false 路径完全跳过 keychain access）
        XCTAssertEqual(keychain.callCount(of: "get(forKey:)"), 0)
    }

    // MARK: - case#3 (edge)：requiresAuth=true 但 Keychain 无 token → 抛 APIError.unauthorized + 不发请求
    func testThrowsUnauthorizedAndDoesNotSendRequestWhenTokenMissing() async {
        let session = MockURLSession()
        // 故意不配 stubbedResponse —— 如果误发请求会抛 .badServerResponse 而非 .unauthorized

        let keychain = MockKeychainStore()
        keychain.getStubResult = .success(nil)  // keychain 无 token

        let client = APIClient(baseURL: baseURL, session: session, keychainStore: keychain)
        let endpoint = Endpoint(path: "/api/v1/home", method: .get, requiresAuth: true)

        do {
            let _: EmptyDataMock = try await client.request(endpoint)
            XCTFail("应抛 APIError.unauthorized")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized)
        } catch {
            XCTFail("应抛 APIError.unauthorized，实际抛 \(error)")
        }

        // 关键断言：session.data(for:) 一次都不调（不浪费网络往返、不让 server 看到伪造请求）
        XCTAssertEqual(session.invocations.count, 0)
        // keychain.get(forKey:) 调过一次（确认 token 状态）
        XCTAssertEqual(keychain.callCount(of: "get(forKey:)"), 1)
    }

    // MARK: - case#4 (edge)：requiresAuth=true 但 keychain 返空字符串 → 视同不存在 → 抛 unauthorized
    func testThrowsUnauthorizedWhenTokenIsEmptyString() async {
        let session = MockURLSession()
        let keychain = MockKeychainStore()
        keychain.getStubResult = .success("")  // 空字符串视同不存在

        let client = APIClient(baseURL: baseURL, session: session, keychainStore: keychain)
        let endpoint = Endpoint(path: "/api/v1/home", method: .get, requiresAuth: true)

        do {
            let _: EmptyDataMock = try await client.request(endpoint)
            XCTFail("应抛 APIError.unauthorized")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized)
        } catch {
            XCTFail("应抛 APIError.unauthorized，实际抛 \(error)")
        }

        XCTAssertEqual(session.invocations.count, 0)
    }

    // MARK: - case#5 (edge)：requiresAuth=true 但 keychain.get 抛错 → 降级为"无 token" → 抛 unauthorized
    func testThrowsUnauthorizedWhenKeychainGetFails() async {
        let session = MockURLSession()
        let keychain = MockKeychainStore()
        keychain.getStubResult = .failure(KeychainError.osStatus(-25300, operation: "get"))

        let client = APIClient(baseURL: baseURL, session: session, keychainStore: keychain)
        let endpoint = Endpoint(path: "/api/v1/home", method: .get, requiresAuth: true)

        do {
            let _: EmptyDataMock = try await client.request(endpoint)
            XCTFail("应抛 APIError.unauthorized")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized, "keychain 错误应降级为 unauthorized，不透传 KeychainError")
        } catch {
            XCTFail("应抛 APIError.unauthorized，实际抛 \(error)")
        }

        XCTAssertEqual(session.invocations.count, 0)
    }

    // MARK: - case#6 (edge)：requiresAuth=true 但 APIClient 构造时未注入 keychain → 抛 unauthorized
    func testThrowsUnauthorizedWhenKeychainStoreNotInjected() async {
        let session = MockURLSession()

        // APIClient 构造时不传 keychainStore（走默认 nil） —— 模拟"配置错误"
        let client = APIClient(baseURL: baseURL, session: session)
        let endpoint = Endpoint(path: "/api/v1/home", method: .get, requiresAuth: true)

        do {
            let _: EmptyDataMock = try await client.request(endpoint)
            XCTFail("应抛 APIError.unauthorized")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized)
        } catch {
            XCTFail("应抛 APIError.unauthorized，实际抛 \(error)")
        }

        XCTAssertEqual(session.invocations.count, 0)
    }

    // MARK: - case#7 (edge)：同一 APIClient 实例并发 100 个请求 → 都正确注入 header（验证线程安全）
    func testConcurrent100RequestsAllInjectAuthorizationHeaderCorrectly() async throws {
        let session = MockURLSession()
        session.stubbedResponse = (makeStubResponseBody(), makeHTTPResponse(statusCode: 200))

        let keychain = MockKeychainStore()
        keychain.getStubResult = .success("concurrent-test-token")

        let client = APIClient(baseURL: baseURL, session: session, keychainStore: keychain)
        let endpoint = Endpoint(path: "/api/v1/home", method: .get, requiresAuth: true)

        // 并发发 100 个请求
        try await withThrowingTaskGroup(of: Void.self) { group in
            for _ in 0..<100 {
                group.addTask {
                    let _: EmptyDataMock = try await client.request(endpoint)
                }
            }
            try await group.waitForAll()
        }

        // 1. session 总共被调 100 次
        XCTAssertEqual(session.invocations.count, 100)
        // 2. 每次请求 header 都正确注入
        for request in session.invocations {
            XCTAssertEqual(
                request.value(forHTTPHeaderField: "Authorization"),
                "Bearer concurrent-test-token",
                "并发请求应全部注入相同 token；MockURLSession + MockKeychainStore 都是 thread-safe"
            )
        }
        // 3. keychain.get 调用次数 == 100（每次请求都从 keychain 重新读，不缓存）
        XCTAssertEqual(keychain.callCount(of: "get(forKey:)"), 100)
    }
}

#endif
```

**具体行为要求**：
- ≥ 4 case，本 AC 给 7 case 覆盖 happy + edge：
  - case#1 happy：requiresAuth=true + token 存在 → header 写
  - case#2 happy：requiresAuth=false → header 无（即使 keychain 有 token 也不读）
  - case#3 edge：requiresAuth=true + token nil → unauthorized + 不发请求
  - case#4 edge：requiresAuth=true + token 空串 → unauthorized + 不发请求（防御性）
  - case#5 edge：requiresAuth=true + keychain.get 抛错 → unauthorized + 不发请求（降级语义）
  - case#6 edge：requiresAuth=true + APIClient.init 未注 keychain → unauthorized + 不发请求（防御性 nil 分支）
  - case#7 edge：同一 APIClient 并发 100 请求 → 全部正确注入（验线程安全）
- 全部 case 走 `MockURLSession`（Story 2.4 落地）+ `MockKeychainStore`（Story 2.8 落地）—— **不**新建 mock
- `@MainActor` test class：与 Story 2.4 `APIClientTests` 同上下文
- `#if DEBUG` 包裹整个 test class：与 Story 5.2 `GuestLoginUseCaseTests` 同模式（`MockKeychainStore` 本身是 #if DEBUG）
- case#7 用 `withThrowingTaskGroup`：Swift 标准并发 API，不引第三方
- case#7 断言 `keychain.callCount(of: "get(forKey:)") == 100`：证明"每次请求都重新读 keychain"（不缓存到 APIClient 内）—— Story 5.4 重登后无需 invalidate cache 的依据
- case#1 / case#2 共用 baseURL（无尾随斜杠）+ endpoint path 含 `/api/v1` 前缀：与 Story 5.2 `AuthEndpoints` 同模式（`AppContainer.resolveDefaultBaseURL` 返回 host-only baseURL，endpoint 必须含 `/api/v1`）
- **不**测 `APIError.unauthorized` 的 envelope 抛错（envelope code=1001）—— 那是 Story 2.4 已覆盖；本 story 只覆盖 token 注入路径

**AC6 — `AppContainerTests` 新增 case 验"convenience init() 后 APIClient 与 AppContainer 共享同一 keychain"**

修改 `iphone/PetAppTests/App/AppContainerTests.swift`（既有文件）追加 1 case：

```swift
// 在 AppContainerTests 既有 testcase 之后追加：

// MARK: - Story 5.3: APIClient 与 AppContainer 共享同一 keychain instance
//
// 验证 convenience init() 走 Story 5.3 改动后的路径 —— APIClient 内的 keychainStore
// 与 AppContainer 暴露的 keychainStore 是同一对象引用.
// 这保证 GuestLoginUseCase 写 token 后，下一次 APIClient 调 requiresAuth=true 接口时
// 立即能从同一 keychain 读到（不会因 namespace 不一致或双 instance 而漏读）.
//
// 注：本测试不能直接读 APIClient 内的 keychain（private 字段）；改用行为断言：
// 1. 通过 container.keychainStore 写 token
// 2. 通过 container.apiClient 调 requiresAuth=true 接口（mock URLSession 不返）
//    → 期望 session 收到带 Authorization header 的请求
//
// 由于 convenience init() 内部走真实 KeychainServicesStore + 真实 APIClient（无 mock URLSession），
// 本 case 用 init(apiClient:keychainStore:) 注入式入口验证"同一 keychain 注入"的语义即可，
// 而 convenience init() 的真实组合关系靠 Story 5.3 集成测试覆盖（如有 / 或 dev manual 联调）.

@MainActor
final class APIClientKeychainSharingTests: XCTestCase {  // 新建独立 class 或加入 AppContainerTests
    func testApiClientReadsSameKeychainAsContainer() async throws {
        // 共享 keychain instance（用 Story 2.8 MockKeychainStore）
        let sharedKeychain = MockKeychainStore()
        try sharedKeychain.set("shared-token-xyz", forKey: KeychainKey.authToken.rawValue)
        sharedKeychain.getStubResult = .success("shared-token-xyz")

        let mockSession = MockURLSession()
        mockSession.stubbedResponse = (
            "{\"code\":0,\"message\":\"ok\",\"data\":{},\"requestId\":\"r\"}"
                .data(using: .utf8)!,
            HTTPURLResponse(url: URL(string: "http://x")!, statusCode: 200,
                            httpVersion: "HTTP/1.1",
                            headerFields: ["Content-Type": "application/json"])!
        )

        let apiClient = APIClient(
            baseURL: URL(string: "http://localhost:8080")!,
            session: mockSession,
            keychainStore: sharedKeychain  // ← 同一个 keychain 也注入到 APIClient
        )

        let container = AppContainer(apiClient: apiClient, keychainStore: sharedKeychain)

        // 通过 container 暴露的接口读 keychain → 与 APIClient 内部读到的应是同一值
        XCTAssertEqual(
            try container.keychainStore.get(forKey: KeychainKey.authToken.rawValue),
            "shared-token-xyz"
        )

        // 通过 APIClient 发 requiresAuth=true 请求 → 验证 header 带上 token
        struct Empty: Decodable {}
        let endpoint = Endpoint(path: "/api/v1/home", method: .get, requiresAuth: true)
        let _: Empty = try await apiClient.request(endpoint)

        let request = mockSession.invocations.first!
        XCTAssertEqual(
            request.value(forHTTPHeaderField: "Authorization"),
            "Bearer shared-token-xyz"
        )
    }
}
```

**具体行为要求**：
- ≥ 1 case，本 AC 给 1：通过 mock keychain 验"同一 instance 注入到 APIClient + AppContainer 两个对象后，APIClient 读到的 token 与 container 暴露的 keychain 写入的 token 一致"
- 本测试**不**直接读 `APIClient.keychainStore`（private 字段无法读）；改用行为断言：通过 mock URLSession 拦截请求 + 验 header
- 用 `init(apiClient:keychainStore:)` 注入式入口（**不**通过 `convenience init()`）—— 后者会真实 new `KeychainServicesStore`，测试无法注入 mock；testing convenience init() 需要做 namespace 隔离的真实 keychain（与 Story 5.2 `GuestLoginUseCaseIntegrationTests` 同精神，但本 story 不增加这种集成测试 —— manual 联调即可）
- 本 case 可以**独立成 `APIClientKeychainSharingTests` 文件**或**追加到 `AppContainerTests` 既有文件**：dev 自行决定；推荐独立文件 `iphone/PetAppTests/App/APIClientKeychainSharingTests.swift`，避免 `AppContainerTests` 文件膨胀
- `@MainActor` test class：与 `AppContainerTests` 同上下文

**AC7 — `bash iphone/scripts/build.sh --test` 全绿 + UI 测试零回归**

```bash
bash iphone/scripts/build.sh             # 普通 build（无 warning）
bash iphone/scripts/build.sh --test      # 单元测试全绿（含 APIClientAuthInjectionTests + APIClientKeychainSharingTests + 既有全部）
bash iphone/scripts/build.sh --uitest    # UI 测试全绿（既有 HomeUITests / NavigationUITests / KeychainPersistenceUITests + Story 5.2 落地的 UITEST_SKIP_GUEST_LOGIN 旁路保持）
```

**具体行为要求**：
- 既有 `bash iphone/scripts/build.sh --test` 全绿（不引入回归 —— Story 2.4 / 2.5 / 5.1 / 5.2 既有 unit tests 不变）
- 既有 `bash iphone/scripts/build.sh --uitest` 全绿（HomeUITests / NavigationUITests / KeychainPersistenceUITests 不受 APIClient 内部改动影响）
- 新增测试全过：
  - `APIClientAuthInjectionTests`（≥ 4 case，本 story 给 7）
  - `APIClientKeychainSharingTests`（1 case）
- 普通 build（`bash iphone/scripts/build.sh`）无 warning

**AC8 — `git status` 范围红线验证**

```bash
git status
# 期望（新增）：
#   iphone/PetAppTests/Core/Networking/APIClientAuthInjectionTests.swift
#   iphone/PetAppTests/App/APIClientKeychainSharingTests.swift（或追加到 AppContainerTests.swift）
# 期望（修改）：
#   iphone/PetApp/Core/Networking/APIClient.swift（init 加 keychainStore Optional 字段 + buildURLRequest 加注入逻辑）
#   iphone/PetApp/App/AppContainer.swift（convenience init() 把 keychain 共享给 APIClient）
#   iphone/PetApp.xcodeproj/project.pbxproj（xcodegen 自动 regen）
# 不期望：
#   ios/ / server/ / iphone/scripts/ / iphone/project.yml / iphone/.gitignore / .gitignore 任何改动
#   docs/ / CLAUDE.md / 任何 production endpoint 的 requiresAuth 值改动（false → true 是 Story 5.5 的事）
```

```bash
# 范围红线验证命令：
git status -- ios/ server/ iphone/scripts/ iphone/project.yml iphone/.gitignore .gitignore docs/ CLAUDE.md
# 必须 "nothing to commit, working tree clean"
```

**具体行为要求**：
- `git status` 仅显示 `APIClient.swift` + `AppContainer.swift` + 新测试 + xcodegen 自动 regen 的 `project.pbxproj`
- `iphone/project.yml` 零改动（新文件靠既有 `sources: - PetApp` / `sources: - PetAppTests` glob 自动纳入）
- `ios/` / `server/` / `docs/` / `CLAUDE.md` 全部零改动

## Tasks / Subtasks

- [x] Task 1: 修改 `APIClient.init` 加 `keychainStore: KeychainStoreProtocol?` Optional 参数（AC1）
  - [x] 1.1 改 `iphone/PetApp/Core/Networking/APIClient.swift`：加 `private let keychainStore: KeychainStoreProtocol?` 字段
  - [x] 1.2 改 `init` 签名加 `keychainStore: KeychainStoreProtocol? = nil` 参数（默认 nil 保证向后兼容）
  - [x] 1.3 init 内 `self.keychainStore = keychainStore`
  - [x] 1.4 验证 `APIClientProtocol` 协议签名零改动（仅具体类的 init 改）
- [x] Task 2: 在 `buildURLRequest(_:)` 内插入 token 注入步骤（AC2）
  - [x] 2.1 在 body 编码块之后、`return request` 之前追加 `if endpoint.requiresAuth { ... }` 注入块（5 行核心逻辑）
  - [x] 2.2 实装决策树：keychainStore nil → unauthorized；token 空 / nil → unauthorized；keychain.get 抛错 → unauthorized（用 try?）；token 存在 → header 写
  - [x] 2.3 header 名严格 `"Authorization"`；header value 严格 `"Bearer \(token)"`（中间一个空格）
  - [x] 2.4 注入逻辑必须在 `session.data(for:)` 之前 —— 抛错时不发请求
- [x] Task 3: 验 `Endpoint.requiresAuth: false` 路径完全零回归（AC3）
  - [x] 3.1 跑 `bash iphone/scripts/build.sh --test` 验既有 APIClientTests / APIClientIntegrationTests / PingUseCaseTests / GuestLoginUseCaseTests 全部 0 失败
  - [x] 3.2 跑 `bash iphone/scripts/build.sh --uitest` 验既有 HomeUITests / NavigationUITests / KeychainPersistenceUITests 全部 0 失败
  - [x] 3.3 确认 `AuthEndpoints.guestLogin(...).requiresAuth == false` / `PingEndpoints.ping().requiresAuth == false` / `PingEndpoints.version().requiresAuth == false` 全部保持 false 不动
- [x] Task 4: 修改 `AppContainer.swift` 让 keychain 共享给 APIClient（AC4）
  - [x] 4.1 改 `convenience init()`：先 `let keychainStore = KeychainServicesStore()` 一次
  - [x] 4.2 `APIClient(baseURL: baseURL, keychainStore: keychainStore)` 显式注入
  - [x] 4.3 `self.init(apiClient: apiClient, keychainStore: keychainStore)` 复用同一 instance
  - [x] 4.4 **保留** `init(apiClient:keychainStore:)` 主 init 既有签名 / 内部逻辑零改动
  - [x] 4.5 既有 `AppContainerTests` 全部 0 失败（`init(apiClient:keychainStore:)` 注入式入口的所有 testcase 不动）
- [x] Task 5: 单元测试 `APIClientAuthInjectionTests`（AC5）
  - [x] 5.1 新建 `iphone/PetAppTests/Core/Networking/APIClientAuthInjectionTests.swift`
  - [x] 5.2 ≥ 4 case，建议写 7：happy/true+token / happy/false / edge/no-token / edge/empty / edge/keychain-error / edge/no-keychain / edge/concurrent-100
  - [x] 5.3 复用 `MockURLSession`（Story 2.4）+ `MockKeychainStore`（Story 2.8），不新建 mock
  - [x] 5.4 `@MainActor` class + `#if DEBUG` 包裹（与 Story 5.2 `GuestLoginUseCaseTests` 同模式）
  - [x] 5.5 case#7 并发用 `withThrowingTaskGroup`，断言 100 次请求 / 100 次 keychain read / 100 次 header 注入正确
- [x] Task 6: 单元测试 `APIClientKeychainSharingTests`（AC6）
  - [x] 6.1 新建 `iphone/PetAppTests/App/APIClientKeychainSharingTests.swift`（独立文件，避免 AppContainerTests 膨胀）
  - [x] 6.2 1 case：通过 mock keychain + mock URLSession 验"AppContainer 与 APIClient 共享同一 keychain instance 时，APIClient 读 token 一致"
  - [x] 6.3 `@MainActor` class
- [x] Task 7: 全套 build / test 验证（AC7）
  - [x] 7.1 `bash iphone/scripts/build.sh` 普通 build 无 warning
  - [x] 7.2 `bash iphone/scripts/build.sh --test` 单元测试全绿（既有 + 新增 ≥ 8）
  - [x] 7.3 `bash iphone/scripts/build.sh --uitest` UI 测试全绿
- [x] Task 8: `git status` 范围红线验证（AC8）
  - [x] 8.1 `git status` 确认仅 `iphone/PetApp/Core/Networking/APIClient.swift` + `iphone/PetApp/App/AppContainer.swift` + `iphone/PetAppTests/Core/Networking/APIClientAuthInjectionTests.swift` + `iphone/PetAppTests/App/APIClientKeychainSharingTests.swift` + `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen 自动 regen）+ 顺手修复 `iphone/PetAppTests/Core/Networking/MockURLSession.swift`（NSLock）改动
  - [x] 8.2 `git status -- ios/ server/ iphone/scripts/ iphone/project.yml iphone/.gitignore .gitignore docs/ CLAUDE.md` → "nothing to commit"

## Dev Notes

### 关键技术约束

1. **`APIClient.init` 必须向后兼容**：`keychainStore: KeychainStoreProtocol? = nil` 默认 nil —— 让 Story 2.4 / 2.5 既有所有 `APIClient(baseURL:)` / `APIClient(baseURL:, session:)` 调用方零改动（含测试 mock）。Optional + 默认值是唯一不破坏既有调用方的路径
2. **`APIClientProtocol` 协议零改动**：`request<T>(_:) async throws -> T` 方法签名不变；keychain 是构造期注入，不进协议。让 `DefaultAuthRepository(apiClient: APIClientProtocol)` 等所有持协议引用的下游零改动
3. **Token 注入逻辑放 `buildURLRequest(_:)` 而非独立 `AuthInterceptor` 类**：5 行内联 vs 独立 protocol + 实装的成本权衡 —— 当前阶段独立类是 over-engineering（YAGNI）。若未来注入逻辑变复杂（retry / cache / 多 token），再 spike 抽出 `AuthInterceptor` 协议 —— 那时是 refactor，行为契约不变
4. **`try?` 而非 `try` 读 keychain**：底层 `KeychainError.osStatus(...)` / `unexpectedDataFormat` 一律降级为"无 token" → 抛 `unauthorized`。业务层只关心"鉴权失败"，不暴露基础设施细节
5. **token 空字符串 `.isEmpty` 视同 nil**：防御性 —— 即使 keychain 有 bug 写入空串，也不会拼出 `Authorization: Bearer ` 畸形 header；与 Story 5.2 `GuestLoginUseCase.existing.isEmpty` 视同不存在的设计同精神
6. **Token 不缓存到 APIClient 内**：每次请求重新调 `keychainStore.get(...)` —— Story 5.4 重登后立即对所有后续请求生效，无需 invalidate cache 操作；keychain.get 在 simulator + 真机都是 ~0.1ms 开销，可忽略。case#7 并发 100 请求验 `keychain.callCount(of: "get(forKey:)") == 100` 即此规约的测试钉子
7. **抛 `unauthorized` 必须发生在 `session.data(for:)` 调用之前**：保证不浪费一次网络往返、不让 server 看到伪造请求；测试要断言 `session.invocations.count == 0`
8. **`AppContainer.convenience init()` 共享 keychain 给 APIClient**：先 `let store = KeychainServicesStore()` 然后两路注入 —— 避免 APIClient 内自己 new 一个新 instance 引发未来万一调整 namespace 时双源不一致风险
9. **`init(apiClient:keychainStore:)` 主 init 不动**：既有 `AppContainerTests` 全部走该入口；本 story 仅改 `convenience init()` 内部走法。测试场景下传入 mock APIClient（已自带 mock keychain 或不需要）—— 主 init 内部**不**强制把 keychain 二次注入到 apiClient
10. **Endpoint requiresAuth 既有值不动**：`PingEndpoints.ping/version` 与 `AuthEndpoints.guestLogin` 全部保持 `requiresAuth: false` —— 证明"false 路径完全零回归"；business endpoint 的 `requiresAuth: true` 由 Story 5.5（`HomeEndpoints.home`）/ 后续 epics 各自落地，本 story 只激活基础设施

### Source tree components to touch

```
iphone/
├─ PetApp/
│  ├─ App/
│  │  └─ AppContainer.swift           # 改：convenience init() 共享 keychain 给 APIClient（AC4）
│  └─ Core/
│     └─ Networking/
│        └─ APIClient.swift           # 改：init 加 keychainStore Optional 字段 + buildURLRequest 加注入逻辑（AC1+AC2）
├─ PetAppTests/
│  ├─ App/
│  │  └─ APIClientKeychainSharingTests.swift  # 新建（AC6）
│  └─ Core/
│     └─ Networking/
│        └─ APIClientAuthInjectionTests.swift  # 新建（AC5）
└─ project.yml                       # 不动（sources glob 自动纳入新文件）

# 不动：
# iphone/PetApp/Core/Networking/{Endpoint,APIError,APIResponse,URLSessionProtocol}.swift（Story 2.4 全套）
# iphone/PetApp/Core/Storage/*（Story 2.8 / 5.1 全套）
# iphone/PetApp/Features/Auth/*（Story 5.2 全套）
# iphone/PetApp/Features/Home/UseCases/PingEndpoints.swift（Story 2.5）
# iphone/PetApp/App/{RootView,AppCoordinator,AppLaunchState,AppLaunchStateMachine,KeychainUITestHookView,PetAppApp}.swift（Story 2.x / 5.2）
# iphone/PetAppTests/Helpers/MockBase.swift（Story 2.7）
# iphone/PetAppTests/Core/Networking/{APIClientTests,APIClientIntegrationTests,MockURLSession,StubURLProtocol}.swift（Story 2.4）
# iphone/PetAppTests/Features/DevTools/MockKeychainStore.swift（Story 2.8 —— 本 story 直接复用）
# iphone/PetAppTests/Features/Home/UseCases/MockAPIClient.swift（Story 2.5 —— APIClientProtocol mock，本 story 不影响）
# iphone/PetAppTests/Features/Auth/*（Story 5.2 全套，本 story 不影响）
# iphone/PetAppUITests/*（Story 2.7 / 5.1 全套；UITEST_SKIP_GUEST_LOGIN 旁路继续工作）
# iphone/project.yml / iphone/scripts/* / iphone/.gitignore
# ios/* / server/*
```

### Testing standards summary（继承 ADR-0002 + Story 2.4 / 2.7 / 2.8 / 5.1 / 5.2）

- **单元测试**：XCTest only（手写 mock）；ADR-0002 §3.1
- **Mock 模式**：复用 Story 2.4 `MockURLSession`（已含 `invocations: [URLRequest]` 可断言 header）+ Story 2.8 `MockKeychainStore`（继承 `MockBase` + `getStubResult` 字段）；**不**新建 mock
- **并发测试**：用 Swift 标准 `withThrowingTaskGroup` —— 不引第三方
- **测试隔离**：本 story 全部用 mock keychain（`MockKeychainStore`）—— 不接触真实 `KeychainServicesStore`，不需要 namespace 隔离
- **跑命令**：`bash iphone/scripts/build.sh --test`（单元）/ `--uitest`（UI）

### Project Structure Notes

- 完全对齐 iOS 架构设计 §4 目录结构（`Core/Networking/{APIClient.swift, APIRequest.swift, APIError.swift, Endpoint.swift, AuthInterceptor.swift}` 列出 5 个文件）
- 本 story **不创建** `AuthInterceptor.swift` 文件实体（YAGNI —— 5 行内联逻辑独立成 protocol + 实装收益为零）；架构 §4 列出但允许内联实现，与 Story 2.4 落地决策（`APIRequest.swift` 也未单独建，由 `Endpoint.swift` 覆盖该语义）同精神
- 测试目录 `iphone/PetAppTests/Core/Networking/` 与 `iphone/PetAppTests/App/` 已存在（Story 2.4 / 2.7 落地），本 story 仅追加新文件
- `Detected conflicts or variances`：未严格遵循架构 §4 "AuthInterceptor.swift" 文件列示 —— 但 §4 是建议性目录，5 行内联在不损害可读性的前提下符合 KISS 原则；如未来 dev 实装时强烈倾向独立 `AuthInterceptor` 类，AC1+AC2 改动可重组到 `AuthInterceptor.swift` 内（行为契约 / 测试断言完全相同），需要在 Completion Notes 解释收益 / 维护成本权衡

### References

- iOS 架构 §4 Core/Networking 目录列示（含 `AuthInterceptor.swift`）/ §5.4 Repository 层 / §6.1 Auth 模块：[Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md]
- V1 §2.3 鉴权方式（`Authorization: Bearer <token>`）/ V1 §2.4 envelope / V1 §3 错误码 1001 = 未登录 / token 无效：[Source: docs/宠物互动App_V1接口设计.md]
- ADR-0002 §3.1 iOS Mock 框架 XCTest only / §3.3 iPhone App 工程目录方案 D / §3.4 CI 命令：[Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md]
- Epic 5 / Story 5.3 完整 AC：[Source: _bmad-output/planning-artifacts/epics.md#Story-5.3-APIClient-interceptor-自动注入-Bearer-token]
- Story 2.4 实装记录（APIClient + Endpoint.requiresAuth + APIError.unauthorized + MockURLSession + StubURLProtocol）：[Source: _bmad-output/implementation-artifacts/2-4-apiclient-封装.md]
- Story 2.5 实装记录（AppContainer 首次落地 + APIClient init 模式 + PingEndpoints.requiresAuth=false）：[Source: _bmad-output/implementation-artifacts/2-5-ping-调用-主界面显示-server-version-信息.md]
- Story 2.8 实装记录（MockKeychainStore 模板 + KeychainStoreProtocol 完整四方法）：[Source: _bmad-output/implementation-artifacts/2-8-dev-重置-keychain-按钮.md]
- Story 5.1 实装记录（KeychainServicesStore + KeychainKey.authToken + KeychainError）：[Source: _bmad-output/implementation-artifacts/5-1-keychain-封装.md]
- Story 5.2 实装记录（GuestLoginUseCase 写 token + AuthEndpoints.guestLogin requiresAuth=false + AppContainer.makeGuestLoginUseCase）：[Source: _bmad-output/implementation-artifacts/5-2-启动自动登录-usecase.md]

## Previous Story Intelligence（来自 Story 5.2 + 2.4 + 2.8）

Story 5.2 是本 story 的**直接前置**，留下以下 IOU + 经验，本 story 必须吸收：

1. **`KeychainKey.authToken.rawValue == "auth.token"`**：Story 5.1 / 5.2 已锁；本 story interceptor 用同一字符串 key 读 token —— 与 GuestLoginUseCase 写 token 配套
2. **GuestLoginUseCase 写 token 时机**：在 `repo.guestLogin(...)` 成功后立即 `keychain.set(response.token, forKey: KeychainKey.authToken.rawValue)`（Step 5）—— 本 story interceptor 即从此读，**保证**首次启动后第一个 `requiresAuth=true` 接口调用就能拿到 token
3. **AuthEndpoints.guestLogin requiresAuth=false 钦定**：登录接口本身免 auth；本 story 不改这个值，证明 false 路径零回归
4. **`@MainActor` test class + `#if DEBUG` 包裹**：与 Story 5.2 `GuestLoginUseCaseTests` 同模式（`MockKeychainStore` 是 #if DEBUG）
5. **`MockKeychainStore` 使用模式**：`getStubResult: Result<String?, Error>` 字段直接配 stub；继承 `MockBase` 自带 `callCount(of:)` 断言

Story 2.4 是本 story 的**间接前置**，留下以下经验：

6. **APIClient.init 设计哲学**：所有协作对象（`session / decoder / encoder`）都通过 init 暴露 + 默认参数 —— 本 story `keychainStore: KeychainStoreProtocol? = nil` 完全沿用此模式
7. **APIClient 决策树语义**：编码错误优先于鉴权错误（body 编码失败先抛 `.decoding`，再到 keychain 注入）—— 本 story 注入逻辑放在 body 编码之后
8. **MockURLSession 用法**：`stubbedResponse` 配响应；`invocations: [URLRequest]` 断言"是否发请求 + 发了什么 header" —— 本 story case#3-#6 都靠 `invocations.count == 0` 断言"未发请求"
9. **APIError.unauthorized 复用**：本 story 复用 Story 2.4 已建的 case，不新增；让 Story 5.4 静默重登能用同一个错误类型识别"鉴权失败"（无论是本地无 token 还是 server 返 401）

Story 2.8 是本 story 的**间接前置**，留下以下经验：

10. **MockBase snapshot-only 读模式**：mock 的 `invocations` / `lastArguments` / `callCounts` 必须通过 snapshot helper 读 —— 本 story case#7 并发 100 请求时，断言 `keychain.callCount(of: "get(forKey:)") == 100` 即走 `callCountsSnapshot()` 加锁路径，TSAN 不报 race（lesson `2026-04-26-mockbase-snapshot-only-reads.md`）

### Story 5.2 / 2.4 lessons 关联（review 阶段已 distill 到 docs/lessons/）

本 story 实装期间值得重读的 lessons：

- `docs/lessons/2026-04-26-mockbase-snapshot-only-reads.md`：本 story case#7 并发 100 验 `keychain.callCount(of:)` 走 snapshot helper —— 必须经过 lock，否则 TSAN 报 race
- `docs/lessons/2026-04-26-jsondecoder-encoder-thread-safety.md`：本 story 不改 APIClient 编解码器策略（`makeDecoder` / `makeEncoder` 仍每请求新建）—— 沿用 Story 2.4 既有"每请求新建" 模式；与本 story"每请求重新读 keychain"同精神（无共享可变状态）
- `docs/lessons/2026-04-26-baseurl-host-only-contract.md`：本 story 测试 baseURL 用 `http://localhost:8080`（host-only），endpoint path 含 `/api/v1` 前缀 —— 与 `AppContainer.resolveDefaultBaseURL` + `AuthEndpoints.guestLogin` 同模式
- `docs/lessons/2026-04-25-swift-explicit-import-combine.md`：本 story 不引入 Combine / SwiftUI（仅 `import Foundation` + `XCTest` + `@testable import PetApp`）

## Git Intelligence Summary

最近 5 个 commit 解析（截至 `99299d5`，story 5-2 收官时点）：

```
99299d5 chore(story-5-2): 收官 Story 5.2 + 归档 story 文件
... feat(iphone): Epic5/5.2 GuestLoginUseCase + SessionStore + AppContainer wire ...
a982f68 chore(story-4-7): 收官 Story 4.7 + 归档 story 文件
6833085 test(server): Epic4/4.7 Layer 2 集成测试 — 游客登录初始化事务全流程
335cf88 chore(story-4-8): 收官 Story 4.8 + 归档 story 文件
```

**对本 story 的指引**：
- Server 端 Epic 4 已全 done（`/auth/guest-login` 接口完整 + `/home` 聚合接口）→ 本 story 完成后立即可在 simulator 真实联调（启 server → 启 App → 走 GuestLoginUseCase 拿 token → 任何 `requiresAuth=true` 接口自动注入 header）
- iPhone 端 Story 5.2 刚 done（GuestLoginUseCase 已写 token 到 keychain）→ 本 story 在 5.2 基础上让 keychain 中的 token 真正"自动用上"
- commit 风格：`feat(iphone): Epic5/5.3 APIClient interceptor 自动注入 Bearer token` / `chore(story-5-3): 收官 Story 5.3 + 归档 story 文件`
- review 阶段如有 lesson 产出，`docs/lessons/index.md` 追加行 + 后续 commit 回填 commit hash

## Latest Tech Information（Apple Foundation / Swift 关键参考）

iOS 17+ / Swift 5.9（注：iphone/project.yml `SWIFT_VERSION: "5.9"`，**不是** Swift 6）当前阶段（2026-04 实测）以下 API 与策略稳定：

- `URLRequest.setValue(_:forHTTPHeaderField:)`：自 iOS 7 起稳定，本 story 写 `Authorization` header 用此 API
- `URLRequest.value(forHTTPHeaderField:)`：自 iOS 7 起稳定，本 story 测试断言 header 值用此 API
- `withThrowingTaskGroup(of:)`：Swift 5.5 起稳定，case#7 并发 100 请求用此 API
- `Optional.flatMap` / `guard let` / `try?`：Swift 标准模式

**已知 Apple 文档建议**：
- `URLRequest.setValue(_:forHTTPHeaderField:)` 设置 header 是覆写式（同名 header 多次 set 会覆盖前次）；本 story 注入 `Authorization` 只调用一次，无重复风险
- `URLRequest.value(forHTTPHeaderField:)` 返回 Optional<String>；header 不存在时返回 nil（不抛错）—— case#2 断言 `XCTAssertNil(...)` 即走此路径

**已知坑预警**：
- `URLRequest` 是 value type；`var request = URLRequest(url:)` 后修改 header / httpMethod 等不会影响外部任何引用 —— 本 story 内多步修改 request 后 return 是安全的
- `MockURLSession.invocations` 字段（Story 2.4 落地）是 `private(set) var invocations: [URLRequest] = []` —— 测试可直接读，无 lock；并发 100 请求场景下需要靠 NSLock 保护（Story 2.4 `MockURLSession` 内部已加 lock）。**dev 实装时务必检查** `MockURLSession.data(for:)` 内对 `invocations.append(...)` 是否在 lock 内 —— 如不在，case#7 会偶发失败。如检查发现 `MockURLSession` 未加 lock，本 story 需顺手加上（属基础设施修复，非 scope creep）

## Project Context Reference

无独立 `project-context.md`；项目背景信息全部从 `CLAUDE.md` + `docs/宠物互动App_*.md` + `_bmad-output/implementation-artifacts/decisions/*.md` 取。**本 story 实装前必读**：

1. `CLAUDE.md` — 项目顶层约束（节点顺序、Repo Separation、Build & Test 命令）
2. `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §4 + §5.4 + §6.1 — 目录结构 + Repository 层 + Auth 模块
3. `docs/宠物互动App_V1接口设计.md` §2.3（鉴权方式）+ §2.4（envelope）+ §3（错误码 1001）
4. `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md` — iOS 工程 / 测试 / CI 决策
5. `_bmad-output/implementation-artifacts/2-4-apiclient-封装.md` — APIClient 主体；本 story 在其上局部追加 token 注入
6. `_bmad-output/implementation-artifacts/5-2-启动自动登录-usecase.md` — 直接前置 story；本 story interceptor 即从其写入的 keychain 读 token
7. `_bmad-output/implementation-artifacts/5-1-keychain-封装.md` — KeychainStoreProtocol + KeychainServicesStore + KeychainKey.authToken
8. `_bmad-output/planning-artifacts/epics.md` Epic 5 §Story 5.3 — 本 story AC 定义

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context)（创建 story 时 + dev-story 实装时）

### Debug Log References

- 2026-04-27 build：`bash iphone/scripts/build.sh` → BUILD SUCCESS（普通 build 无 warning）
- 2026-04-27 unit tests：`bash iphone/scripts/build.sh --test` → 166 tests passed, 0 failures
- 2026-04-27 ui tests：`bash iphone/scripts/build.sh --uitest` → 8 tests passed, 0 failures

### Completion Notes List

1. **AC1 / AC2 完成**：`APIClient.init` 加 Optional `keychainStore` 参数（默认 nil）；`buildURLRequest(_:)` 内追加按 `endpoint.requiresAuth` 决策的注入块（5 行核心逻辑），决策树严格按 story Dev Notes 钦定：keychainStore nil → unauthorized；token 空 / nil → unauthorized；keychain.get 抛错（`try?`）→ unauthorized；token 非空 → 写 `Authorization: Bearer <token>` header。注入位置在 body 编码之后、`session.data(for:)` 之前 —— 测试断言 `MockURLSession.invocations.count == 0` 验证抛错路径不发请求。
2. **AC3 完成**：未修改 `AuthEndpoints.swift` / `PingEndpoints.swift` / 任何 Story 2.5 / 5.2 既有 production 文件；既有 166 个 unit test + 8 个 UI test 全部 0 失败；既有 `APIClient(baseURL:)` / `APIClient(baseURL:, session:)` 调用方零改动（默认参数 nil 保证向后兼容）。
3. **AC4 完成**：`AppContainer.convenience init()` 改为先 `let keychainStore = KeychainServicesStore()` 一次，然后 APIClient 与 self.init 共享同一 instance。主 init `init(apiClient:keychainStore:)` 签名 / 内部逻辑零改动。`AppContainerTests` 全部 0 失败。
4. **AC5 完成**：新建 `APIClientAuthInjectionTests.swift`（7 case）—— happy×2 (true+token / false) + edge×5 (no-token / empty / keychain-error / no-keychain / concurrent-100)；全部用 `MockURLSession` + `MockKeychainStore`，未新建 mock；`@MainActor` + `#if DEBUG` 包裹与 Story 5.2 同模式；case#7 用 `withThrowingTaskGroup` 并发 100 请求，断言 `keychain.callCount(of: "get(forKey:)") == 100` 验"每次请求都从 keychain 重新读不缓存"，走 MockBase `callCountsSnapshot()` 加锁路径（lesson 2026-04-26-mockbase-snapshot-only-reads.md）。
5. **AC6 完成**：新建 `APIClientKeychainSharingTests.swift`（1 case）—— 通过共享 `MockKeychainStore` 注入到 APIClient + AppContainer 两边，断言 APIClient 读到的 token 与 container 暴露的 keychain 写入的 token 一致；走 `init(apiClient:keychainStore:)` 注入式入口（不通过 `convenience init()` 真实 KeychainServicesStore）。
6. **AC7 完成**：build.sh / --test / --uitest 三套全绿。
7. **AC8 完成**：`git status` 仅显示 5 个预期文件改动 + 2 个新文件 + xcodegen 自动 regen 的 project.pbxproj；`ios/` / `server/` / `iphone/scripts/` / `iphone/project.yml` / `iphone/.gitignore` / `docs/` / `CLAUDE.md` 全部零改动。
8. **基础设施修复（顺手）**：`MockURLSession.swift` 用 NSLock 包了 `invocations` / `stubbedResponse` / `stubbedError` —— story Latest Tech Information 已知坑预警钉子：原版 `invocations.append(...)` 不在 lock 内，case#7 并发 100 测试场景下会 race（TSAN 必报，偶发 crash）。改后 `invocations` 通过 getter 返回快照（与 MockBase 同精神），既有 `mock.invocations.count` / `.first` 调用零改动。
9. **YAGNI 选择**：未新建 `AuthInterceptor.swift` 文件实体（虽然 iOS 架构 §4 列出）—— 5 行内联逻辑独立成 protocol + 实装收益为零；future Story 若注入逻辑变复杂（retry / cache / 多 token），再 spike 抽出协议 —— 那时是 refactor，行为契约不变。

### File List

**修改（production code）**：
- iphone/PetApp/Core/Networking/APIClient.swift — 加 `keychainStore: KeychainStoreProtocol?` Optional 字段 + buildURLRequest 内追加 token 注入决策树（AC1+AC2）
- iphone/PetApp/App/AppContainer.swift — convenience init() 共享 keychain 给 APIClient（AC4）

**修改（test infrastructure）**：
- iphone/PetAppTests/Core/Networking/MockURLSession.swift — 用 NSLock 保护 invocations / stubbedResponse / stubbedError（AC5 case#7 并发 100 测试要求）

**新建（unit tests）**：
- iphone/PetAppTests/Core/Networking/APIClientAuthInjectionTests.swift — token 注入路径 7 case 单元测试（AC5）
- iphone/PetAppTests/App/APIClientKeychainSharingTests.swift — APIClient 与 AppContainer 共享同一 keychain 1 case 单元测试（AC6）

**自动 regen（xcodegen）**：
- iphone/PetApp.xcodeproj/project.pbxproj — 新文件靠 sources glob 自动纳入

**不动**：
- ios/ / server/ / iphone/scripts/ / iphone/project.yml / iphone/.gitignore / docs/ / CLAUDE.md
- iphone/PetApp/Core/Networking/{Endpoint,APIError,APIResponse,URLSessionProtocol}.swift
- iphone/PetApp/Core/Storage/{KeychainStore,KeychainKey,KeychainError,KeychainServicesStore}.swift
- iphone/PetApp/Features/Auth/* / Features/Home/* / App/{RootView,AppCoordinator,...}
- iphone/PetAppTests/Helpers/MockBase.swift / Features/DevTools/MockKeychainStore.swift / Core/Networking/{APIClientTests,APIClientIntegrationTests,StubURLProtocol}.swift

## Change Log

| 日期 | 改动 | 来源 |
|---|---|---|
| 2026-04-27 | Story 5.3 实装：APIClient.buildURLRequest 内激活 token 注入决策树（按 endpoint.requiresAuth + Keychain 注入 Authorization Bearer header）；APIClient.init 加 Optional keychainStore 参数（默认 nil 向后兼容）；AppContainer.convenience init() 共享 keychain 给 APIClient；新增 APIClientAuthInjectionTests（7 case）+ APIClientKeychainSharingTests（1 case）；MockURLSession 加 NSLock 保护 invocations / stubbed* 字段；166 unit + 8 UI tests 全绿；status → review | dev-story execution |
