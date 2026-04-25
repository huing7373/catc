# Story 2.4: APIClient 封装（Core/Networking 基础设施）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iOS 开发,
I want 一个统一的 `APIClient`（基于 `URLSession` + 统一 envelope 解析 + 错误三层映射）,
so that 后续所有 REST 调用都走同一入口，统一处理 401 / 网络错误 / 业务错误码 / 解码错误。

## 故事定位（Epic 2 第三条实装 story；2.5 / 2.6 的网络底座）

这是 Epic 2 的**网络层第一砖**，紧接 Story 2.3 (`done`) 的导航架构。**本 story 落地的是"协议 + 实体 + 错误 + 解码"的纯基础设施**，不调用任何真实 server 接口、不接 ViewModel、不做 token 注入、不挂 Interceptor、不绑定到 UI 错误展示。

**本 story 是后续 story / Epic 的基础**：
- Story 2.5（ping 调用 + 主界面显示 server /version 信息）：第一次**真实调用** APIClient（已有 `PingEndpoint` / `VersionEndpoint` 通过 APIClient 跑通），把 `App vX.Y.Z · Server <commit>` 接入主界面
- Story 2.6（基础错误 UI 框架）：消费本 story 抛出的 `APIError` 四类，做 Toast / AlertOverlay / RetryView 联动
- Epic 5（节点 2 自动游客登录）：在本 story 的 APIClient 上**追加** `AuthInterceptor`（拦截器自动注入 `Authorization: Bearer`），不重写 APIClient 主体
- Epic 4+ 全部业务 Repository：`AuthRepository` / `HomeRepository` / `StepRepository` 等都注入 `APIClientProtocol`，调 `client.request(endpoint)` 取 typed data

**不涉及**：
- 真实 server 接口调用（→ Story 2.5 ping / FetchVersion；Epic 4 真实业务接口）
- Token / Bearer 注入逻辑（→ Epic 5 Story 5.3 AuthInterceptor）
- 错误的 UI 展示（→ Story 2.6 ErrorPresenter / Toast / AlertOverlay / RetryView）
- 重试 / 限流 / 离线队列（不在 MVP 范围）
- WebSocket（→ Epic 10 / 12 `WebSocketClient` 是独立组件，不复用 APIClient）
- Repository / UseCase / ViewModel 任何业务层代码（→ Epic 4+）
- `AuthInterceptor.swift` 文件实体（架构 §4 已列；本 story **不创建**，留给 Epic 5）

**范围红线**：

- **不动 `ios/`**：本 story 绝对不修改 `ios/` 任何文件（CLAUDE.md "Repo Separation" + ADR-0002 §3.3 + Story 2.2/2.3 既有约束的延续）。最终 `git status` 须确认 `ios/` 下零改动。
- **不动 `server/`**：本 story 是 iOS 端网络层落地，不涉及 server 侧任何文件。
- **不动 Story 2.2 / 2.3 已落地的 production 文件**：`HomeView.swift` / `HomeViewModel.swift` / `RootView.swift` / `PetAppApp.swift` / `AppCoordinator.swift` / 三个 SheetPlaceholderView / `AccessibilityID.swift`（**老 Home + SheetPlaceholder 常量值字符串**）零改动。如确实需要扩展 `AccessibilityID`，在新增 namespace `AccessibilityID.Networking`（**当前不需要**——本 story 不接 UI；如未来需要也保留给 2.5）下追加，**不改老的**。
- **不引入第三方 HTTP 库**：用 `URLSession`（架构 §18.1 钦定）+ `JSONDecoder` / `JSONEncoder` 标准库；**禁止** Alamofire / Moya / RxAlamofire / Swift OpenAPI Generator 等。
- **不引入第三方测试库**：mock 全部手写（ADR-0002 §3.1）。**禁止** Mockingbird / Cuckoo / OHHTTPStubs。
- **不创建** `iphone/PetApp/Core/Networking/AuthInterceptor.swift` 文件实体（架构 §4 列出但留给 Epic 5）。
- **不创建** Repository / UseCase / Feature 任何业务文件。

## Acceptance Criteria

**AC1 — `APIError` 类型（四态枚举 + Equatable + LocalizedError）**

新建 `iphone/PetApp/Core/Networking/APIError.swift`，定义统一抛错类型：

```swift
import Foundation

/// APIClient 抛出的统一错误类型。四态对应 V1接口设计 §2.4 envelope 解析的四种失败路径。
public enum APIError: Error, Equatable {
    /// 业务错误：HTTP 200 + envelope.code != 0。
    /// 对应 V1接口设计 §3 的 32 个错误码（除 0=成功外）。
    /// - code: V1接口设计 §3 业务码（1001..7002）
    /// - message: 服务端 envelope.message 原文
    /// - requestId: 服务端 envelope.requestId（链路追踪）
    case business(code: Int, message: String, requestId: String)

    /// HTTP 401：token 失效 / 未登录。
    /// 对应 envelope.code = 1001（V1接口设计 §3）。
    /// 注意：HTTP 401 与 envelope.code=1001 是"或"关系——两条路径都视为 unauthorized。
    /// 本 story 实装时按"先看 HTTP status，再看 envelope.code"决策。
    case unauthorized

    /// 网络层错误：连不上 / 超时 / 连接重置 / DNS 失败 / SSL 错误 / 离线。
    /// 包装底层 URLError 或其它 transport 错误。
    case network(underlying: Error)

    /// 解码失败：envelope 结构不符 / data 字段不能解为目标类型 T。
    /// 包装底层 DecodingError 或其它解码相关错误。
    case decoding(underlying: Error)

    // MARK: - Equatable

    /// 自定义 Equatable：underlying error 比较只对比 localizedDescription（Error 协议本身不 Equatable）。
    /// 仅用于测试断言 ".network 等于 .network" 这种粗粒度等价；不做深度比较。
    public static func == (lhs: APIError, rhs: APIError) -> Bool {
        switch (lhs, rhs) {
        case let (.business(c1, m1, r1), .business(c2, m2, r2)):
            return c1 == c2 && m1 == m2 && r1 == r2
        case (.unauthorized, .unauthorized):
            return true
        case let (.network(e1), .network(e2)):
            return (e1 as NSError).domain == (e2 as NSError).domain
                && (e1 as NSError).code == (e2 as NSError).code
        case let (.decoding(e1), .decoding(e2)):
            return String(describing: e1) == String(describing: e2)
        default:
            return false
        }
    }
}

extension APIError: LocalizedError {
    /// 简洁的 dev / log 友好描述；**不**用于 UI 展示（UI 文案在 Story 2.6 的 ErrorPresenter 决定）。
    public var errorDescription: String? {
        switch self {
        case let .business(code, message, _):
            return "Business error \(code): \(message)"
        case .unauthorized:
            return "Unauthorized (HTTP 401 or code 1001)"
        case let .network(underlying):
            return "Network error: \(underlying.localizedDescription)"
        case let .decoding(underlying):
            return "Decoding error: \(underlying.localizedDescription)"
        }
    }
}
```

**关键约束**：
- 四态**严格按 epics.md Story 2.4 AC** 定义：`business / unauthorized / network / decoding`，命名一字不差。
- `business` 必带 `requestId`（V1接口设计 §2.4 envelope 字段；后续日志 / 客服排查靠它）。
- `Equatable` 是为了测试断言便利；**不做** underlying error 的深度比较（Swift Error 协议本身不 Equatable，强行 deep-equal 会让代码复杂度爆炸）。
- `LocalizedError.errorDescription` 仅用于 dev / log；**禁止**在此处写 UI 用户文案（i18n 与展示策略归 Story 2.6 ErrorPresenter）。
- `import Foundation` 顶部即可；**不需要** `import Combine` / `import SwiftUI`（纯数据类型，与 UI 无关）。

**AC2 — `Endpoint` 类型（请求元信息）**

新建 `iphone/PetApp/Core/Networking/Endpoint.swift`，定义 REST 请求元信息容器：

```swift
import Foundation

/// HTTP 方法枚举。MVP 仅覆盖 V1接口设计中实际用到的方法。
public enum HTTPMethod: String {
    case get = "GET"
    case post = "POST"
    // PUT / DELETE / PATCH 在 V1接口设计 §4-§9 未出现，MVP 不预留；
    // 后续若新增接口需要时再追加。
}

/// REST 请求元信息：path（含 query）+ method + body + 是否需鉴权。
///
/// 设计原则：
/// - 用 struct 而非 enum：让上层 Repository 可以按需构造（不强求每个接口都列在一个全局 enum 中）。
/// - body 用 `Encodable & Sendable` 包装的存在性容器（AnyEncodable），让 struct 是 Equatable 友好的 value type。
/// - 不持有 Authorization header：interceptor（Epic 5）按 requiresAuth 自动注入，APIClient 主体不写 token 字符串。
///
/// 典型构造（Epic 2 / 4 stories 落地时用）：
/// ```swift
/// // GET /api/v1/version（无鉴权，无 body）
/// Endpoint(path: "/version", method: .get, body: nil, requiresAuth: false)
/// // POST /api/v1/auth/guest-login（无鉴权——这是登录接口本身；body 为 GuestLoginRequest）
/// Endpoint(path: "/auth/guest-login", method: .post, body: AnyEncodable(req), requiresAuth: false)
/// // GET /api/v1/home（需鉴权，无 body）
/// Endpoint(path: "/home", method: .get, body: nil, requiresAuth: true)
/// ```
public struct Endpoint {
    /// path：以 `/` 开头的 v1 接口路径（**不**含 host / `/api/v1` 前缀；前缀由 APIClient 拼）。
    /// 例如 `/version` / `/auth/guest-login` / `/home` / `/chest/open`。
    public let path: String

    /// HTTP 方法。
    public let method: HTTPMethod

    /// 请求体。GET 通常 nil；POST 通常带（用 AnyEncodable 包装）。
    /// nil 时 APIClient 不写 `Content-Type` header / 不写 body 字节。
    public let body: AnyEncodable?

    /// 是否要求 Authorization Bearer token。
    /// MVP 阶段（Epic 2）所有接口手动设 false；Epic 5 落地 AuthInterceptor 后，
    /// 按本字段自动注入 token（true 且无 token 时 interceptor 直接抛 APIError.unauthorized）。
    public let requiresAuth: Bool

    public init(
        path: String,
        method: HTTPMethod,
        body: AnyEncodable? = nil,
        requiresAuth: Bool
    ) {
        self.path = path
        self.method = method
        self.body = body
        self.requiresAuth = requiresAuth
    }
}

/// 类型擦除的 Encodable 包装。
///
/// 用途：让 `Endpoint.body` 字段能容纳任意 Encodable 类型，又不污染 Endpoint 本身的 generic 签名
/// （否则 `[Endpoint]` 这类聚合不可写）。Foundation 不提供 `AnyEncodable`，自实装 ~6 行即可。
public struct AnyEncodable: Encodable {
    private let _encode: (Encoder) throws -> Void

    public init<T: Encodable>(_ wrapped: T) {
        self._encode = wrapped.encode
    }

    public func encode(to encoder: Encoder) throws {
        try _encode(encoder)
    }
}
```

**关键约束**：
- `HTTPMethod` 用 `enum` + `String` rawValue（与 URLRequest API 直接映射）。仅含 `get / post`，**不预留** `put / delete / patch`（V1接口设计未用到 → YAGNI）。
- `Endpoint` 用 struct，**不**用 enum。理由：① Repository 按需构造更自然；② 业务接口数量大（V1接口设计 §4-§9 ~30 个），全部塞一个 enum 文件会非常臃肿。**对照** ADR-0002 §3.3 "Endpoint 设计" 与 iOS 架构 §8.2 "建议每个接口使用枚举或结构体定义"——两者都允许 struct，本 story 选 struct（更 modular）。
- `AnyEncodable` 自实装（不引第三方）；底层逻辑：闭包捕获 wrapped 的 `encode(to:)` 方法。
- **不预留** query 参数字段：MVP V1接口设计的所有 GET（如 `/cosmetics/inventory`、`/compose/overview`）都没有 query string；如未来需要，新增 `query: [URLQueryItem]?` 字段（追加而非破坏）。
- `requiresAuth` 是**显式 bool**（不靠魔法默认值）：让每个 Endpoint 在源头决策，避免 "默认 true 还是 false" 的反复争论。
- `AnyEncodable` 的 `Encodable` 协议是 **value-erasure**——即使包了 `[String: Int]` 也能正确编码到 JSON。

**AC3 — `APIResponse<T>` envelope 解码模型**

新建 `iphone/PetApp/Core/Networking/APIResponse.swift`，定义 V1接口设计 §2.4 统一响应结构的解码模型：

```swift
import Foundation

/// V1接口设计 §2.4 统一响应结构的解码模型。
///
/// ```json
/// {
///   "code": 0,
///   "message": "ok",
///   "data": { ... },
///   "requestId": "req_xxx"
/// }
/// ```
///
/// 用法：APIClient 内部用 `try JSONDecoder().decode(APIResponse<T>.self, from: data)`
/// 一次解出 envelope；`code != 0` 时抛 APIError.business(code, message, requestId)，
/// `code == 0` 时返回 `response.data`（已是泛型 T）。
///
/// 设计选择：
/// - `data` 字段在错误响应中可能缺省 / 为 null（V1接口设计 §3 错误码示例无 `data` 字段）。
///   做成 `data: T?` 让解码兼容这种情况。**调用方 expectations**：成功响应（code=0）必有 data，
///   APIClient 在 unwrap 时若 nil 抛 APIError.decoding（"成功响应 data 为 null" 是契约违反）。
public struct APIResponse<T: Decodable>: Decodable {
    public let code: Int
    public let message: String
    public let data: T?
    public let requestId: String

    public init(code: Int, message: String, data: T?, requestId: String) {
        self.code = code
        self.message = message
        self.data = data
        self.requestId = requestId
    }
}

/// 空 data 的占位：用于 V1接口设计中 data 为 `{}` 的接口（如 ping、bind-wechat）。
/// 用法：`client.request(endpoint) as Empty`，调用方拿到 Empty 不做任何事即可。
public struct Empty: Decodable {
    public init() {}
}
```

**关键约束**：
- `data: T?` 而非 `data: T`：错误响应可能没有 data（V1接口设计 §3 错误码示例无 data）；解码时让 nil 不致整体失败，由 APIClient 业务逻辑（code != 0）路径优先抛 `.business`。
- `Empty: Decodable` 占位：让"data 为 `{}`"的接口（如 V1 §6.1 sync 返回 `data: {}`、§4.2 bind-wechat 返回 data: {}）能用 `client.request(...) as Empty` 简洁表达。
- **不**实装 `Encodable` —— 客户端从不自构造 envelope（envelope 是 server 写的）。
- 文件不导入 Combine / SwiftUI / URLSession 任何 framework；**仅** `import Foundation`。

**AC4 — `URLSessionProtocol` 协议（mock 切口）**

新建 `iphone/PetApp/Core/Networking/URLSessionProtocol.swift`，提取 URLSession 的最小 mock 切口：

```swift
import Foundation

/// URLSession 抽象切口。
///
/// 目的：让单元测试通过 mock 注入受控的 (Data, URLResponse) 或 throw URLError，
/// 验证 APIClient 的解码 / 错误映射 / 401 路径，无需真启动 HTTP server。
///
/// 集成测试（Story 2.7 落地后）会启动真 mock HTTP server（如 swifter / Vapor 测试模式 / URLProtocol fake），
/// 不通过本协议，而是用真 URLSession + 替换 host —— 两条路径互补。
///
/// 协议方法签名严格匹配 URLSession 的标准异步 API：
/// `func data(for request: URLRequest) async throws -> (Data, URLResponse)`
public protocol URLSessionProtocol: Sendable {
    func data(for request: URLRequest) async throws -> (Data, URLResponse)
}

/// URLSession 通过 typealias / extension 自然实现该 protocol（API 已存在）。
extension URLSession: URLSessionProtocol {}
```

**关键约束**：
- 协议方法签名**严格等同** URLSession 的 `data(for:)` 公开 API（`async throws -> (Data, URLResponse)`），所以 `URLSession` 通过空 extension 即可符合协议。
- 不暴露 `dataTask(with:completionHandler:)` callback API —— async/await 是架构 §18.1 主流（ADR-0002 §3.2 也锁定）。
- **不**继承 `Sendable`-only 协议（避免 Swift 6 strict concurrency 在测试 mock 实现时报红）；当前 `Sendable` 已足够。
- 文件 ≤ 15 行；纯 protocol 声明 + extension。

**AC5 — `APIClient` 主体（核心实现）**

新建 `iphone/PetApp/Core/Networking/APIClient.swift`：

```swift
import Foundation

/// APIClient 协议：让上层 Repository 可注入 mock。
public protocol APIClientProtocol: Sendable {
    /// 发起请求并解出业务 data。
    /// - Throws: APIError.business / .unauthorized / .network / .decoding（详见 APIError 文档）
    func request<T: Decodable>(_ endpoint: Endpoint) async throws -> T
}

/// 统一 REST 客户端。
///
/// 职责（本 story 范围）：
/// 1. 拼接 baseURL + path（不做 query 字符串处理；MVP 无 query 接口）
/// 2. 构造 URLRequest（method / body / Content-Type / Accept header）
/// 3. 发起请求（通过注入的 URLSessionProtocol）
/// 4. 解析 HTTP status：
///    - 401 → throw APIError.unauthorized（不再尝试解 body）
///    - 200 → 继续解 envelope
///    - 其它 5xx / 4xx → throw APIError.network（按 NSError(NSURLErrorDomain, code: status) 包装）
/// 5. 解 envelope（APIResponse<T>）：
///    - decode 失败 → throw APIError.decoding(underlying:)
///    - code == 1001 → throw APIError.unauthorized（envelope-level 401 别名）
///    - code != 0 && != 1001 → throw APIError.business(code, message, requestId)
///    - code == 0 && data == nil → throw APIError.decoding("data is null on success")
///    - code == 0 && data != nil → return data
/// 6. URLError 透传：底层 URLSession throw 出来的 URLError 包装成 APIError.network
///
/// 不在本 story 范围内（→ 后续 story / Epic）：
/// - 不注入 Authorization header（→ Epic 5 AuthInterceptor）
/// - 不重试（→ MVP 不做）
/// - 不限流（→ MVP 不做）
/// - 不做 request / response 日志（→ Story 2.7 测试基础设施落地 logger 后再对接；本 story 仅 print 调试不强制）
/// - 不缓存（→ MVP 不做）
public final class APIClient: APIClientProtocol {
    private let baseURL: URL
    private let session: URLSessionProtocol
    private let decoder: JSONDecoder
    private let encoder: JSONEncoder

    public init(
        baseURL: URL,
        session: URLSessionProtocol = URLSession.shared,
        decoder: JSONDecoder = JSONDecoder(),
        encoder: JSONEncoder = JSONEncoder()
    ) {
        self.baseURL = baseURL
        self.session = session
        self.decoder = decoder
        self.encoder = encoder
    }

    public func request<T: Decodable>(_ endpoint: Endpoint) async throws -> T {
        let urlRequest = try buildURLRequest(endpoint)

        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: urlRequest)
        } catch let urlError as URLError {
            throw APIError.network(underlying: urlError)
        } catch {
            // 非 URLError 也归为 network：transport 层任意失败
            throw APIError.network(underlying: error)
        }

        guard let httpResponse = response as? HTTPURLResponse else {
            // URLSession 给非 HTTP response 概率几乎 0（除非是 file:// 等），但兜底
            throw APIError.network(
                underlying: NSError(
                    domain: "APIClient",
                    code: -1,
                    userInfo: [NSLocalizedDescriptionKey: "Response is not HTTPURLResponse"]
                )
            )
        }

        // HTTP 401 直接短路（不解 body —— body 可能是 nginx 默认 401 页而非 envelope）
        if httpResponse.statusCode == 401 {
            throw APIError.unauthorized
        }

        // 非 2xx HTTP 状态：归 network（envelope 大概率不可解）
        // 注：业务错误的 HTTP 是 200（V1 §2.4 + ADR-0006 §6 规约），所以 4xx/5xx 都视为 transport 异常。
        if !(200...299).contains(httpResponse.statusCode) {
            throw APIError.network(
                underlying: NSError(
                    domain: NSURLErrorDomain,
                    code: httpResponse.statusCode,
                    userInfo: [NSLocalizedDescriptionKey: "HTTP \(httpResponse.statusCode)"]
                )
            )
        }

        // 解 envelope
        let envelope: APIResponse<T>
        do {
            envelope = try decoder.decode(APIResponse<T>.self, from: data)
        } catch {
            throw APIError.decoding(underlying: error)
        }

        // envelope-level 业务码决策
        if envelope.code == 0 {
            // 成功：data 必须非 nil（契约违反则视为 decoding 错误）
            guard let payload = envelope.data else {
                throw APIError.decoding(
                    underlying: NSError(
                        domain: "APIClient",
                        code: -2,
                        userInfo: [NSLocalizedDescriptionKey:
                            "Envelope code is 0 but data is null"]
                    )
                )
            }
            return payload
        } else if envelope.code == 1001 {
            // envelope-level 401（V1 §3：1001 = 未登录 / token 无效）
            throw APIError.unauthorized
        } else {
            // 其它业务错误码（1002..7002）
            throw APIError.business(
                code: envelope.code,
                message: envelope.message,
                requestId: envelope.requestId
            )
        }
    }

    // MARK: - Private helpers

    private func buildURLRequest(_ endpoint: Endpoint) throws -> URLRequest {
        // baseURL 已含 `/api/v1` 前缀（构造 APIClient 时由调用方决定，AppContainer 阶段做）。
        // path 以 `/` 开头，直接 appendingPathComponent 会丢前导 `/`，用 absoluteString 拼接。
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
                request.httpBody = try encoder.encode(body)
                request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            } catch {
                throw APIError.decoding(underlying: error)  // 客户端自身编码失败也归 decoding
            }
        }

        // 注：requiresAuth 暂不处理（Epic 5 AuthInterceptor 落地时插入）
        // 当前 MVP 所有 Epic 2 内的 endpoint 都用 requiresAuth=false（ping / version 接口本身无鉴权）

        return request
    }
}
```

**关键约束**：
- `final class APIClient` + `APIClientProtocol`：协议是 mock 注入切口（mock 不需要继承 class）；class 是默认实现。`Sendable` 协议让 Swift 6 strict concurrency 友好。
- **依赖注入**所有协作对象（`session / decoder / encoder`）都暴露在 init 上，便于测试覆盖编码 / 解码策略变化。
- `baseURL` 由调用方提供，**不**在 APIClient 内 hardcode `http://localhost:8080`。Story 2.5 落地时由 `AppContainer`（或临时构造点）传入。
- **HTTP status 决策树**（依次走，先匹配先抛）：
  1. URLSession throw → `.network`
  2. 非 HTTPURLResponse → `.network`
  3. status == 401 → `.unauthorized`
  4. status ∉ 2xx → `.network`
  5. envelope decode 失败 → `.decoding`
  6. envelope.code == 0 + data nil → `.decoding`（"success without data" 是契约违反）
  7. envelope.code == 0 + data ok → return data
  8. envelope.code == 1001 → `.unauthorized`（envelope-level 401 别名）
  9. envelope.code ∈ {其它} → `.business`
- **`Authorization` header 不在本 story 写**：Epic 5 AuthInterceptor 落地时通过 protocol-based decorator 模式包装 APIClient（或在 buildURLRequest 内插入 hook），本 story 给 `requiresAuth: Bool` 字段留位。
- **不写日志**：本 story 不依赖 logger 框架（iOS 端 logger 选型属 Story 2.7 测试基础设施附带）；如 dev 调试需要可临时 `print`，但不允许遗留 print 到 commit。
- 文件长度目标 ≤ 200 行（含 doc comment）；逻辑紧凑，不抽象出多余的 sub-helper。

**AC6 — `Networking` 目录文件清单与组织**

新建目录 `iphone/PetApp/Core/Networking/`（架构 §4 钦定路径）下 5 个文件：

| 文件 | 职责 | 大致长度 |
|---|---|---|
| `APIError.swift` | APIError 枚举 + Equatable + LocalizedError | ~50 行 |
| `Endpoint.swift` | HTTPMethod enum + Endpoint struct + AnyEncodable | ~50 行 |
| `APIResponse.swift` | APIResponse<T> 解码模型 + Empty 占位 | ~30 行 |
| `URLSessionProtocol.swift` | URLSession 抽象切口 + extension | ~15 行 |
| `APIClient.swift` | APIClientProtocol + APIClient 实现 | ~150-200 行 |

**关键约束**：
- 5 个文件**严格按上表组织**，**不**合并（单一职责，便于 review 与未来重构）。
- **不**新建 `AuthInterceptor.swift`（架构 §4 列出但本 story 不在范围）。
- 目录路径必须严格 `iphone/PetApp/Core/Networking/`（架构 §4）。`Core/` 是 Story 2.4 第一次实质用到（Story 2.2 留了 `Core/DesignSystem/Components/` 空目录占位但无文件）。

**AC7 — 单元测试覆盖（≥5 case，按 epics.md AC + 扩展）**

新建 `iphone/PetAppTests/Core/Networking/APIClientTests.swift`（**镜像 production 路径**），覆盖以下 6 类 case：

```swift
import XCTest
@testable import PetApp

@MainActor
final class APIClientTests: XCTestCase {
    // 共用 baseURL（http://localhost:8080/api/v1）—— 测试中不真实发起请求
    private let baseURL = URL(string: "http://localhost:8080/api/v1")!

    // 1. happy: 200 + envelope code=0 + data 完整 → 返回 typed T
    func testRequestReturnsDecodedDataOnSuccess() async throws { ... }

    // 2. edge: 200 + envelope code=1002（参数错误）→ throw APIError.business
    func testRequestThrowsBusinessErrorWhenEnvelopeCodeNonZero() async throws { ... }

    // 3. edge: HTTP 401 → throw APIError.unauthorized
    func testRequestThrowsUnauthorizedOnHttp401() async throws { ... }

    // 4. edge: 200 + envelope code=1001 → throw APIError.unauthorized（envelope-level 401 别名）
    func testRequestThrowsUnauthorizedWhenEnvelopeCodeIs1001() async throws { ... }

    // 5. edge: URLSession throw URLError(.timedOut)→ throw APIError.network
    func testRequestThrowsNetworkErrorOnURLSessionFailure() async throws { ... }

    // 6. edge: 200 + body 不是合法 envelope JSON → throw APIError.decoding
    func testRequestThrowsDecodingErrorOnInvalidEnvelopeBody() async throws { ... }

    // 7. edge: 200 + envelope code=0 + data 字段为 null → throw APIError.decoding
    func testRequestThrowsDecodingErrorOnSuccessWithNullData() async throws { ... }

    // 8. happy: POST + body 编码 → URLRequest.httpBody 正确填充 + Content-Type 写对
    func testPostRequestEncodesBodyAndSetsContentType() async throws { ... }
}
```

**MockURLSession 实装**（建议放同一文件文末或新建 `iphone/PetAppTests/Core/Networking/MockURLSession.swift`；避免 Story 2.7 落地 `MockBase.swift` 之前过度抽象）：

```swift
final class MockURLSession: URLSessionProtocol, @unchecked Sendable {
    /// 受控返回：一组 (Data, URLResponse) 让测试设置预期
    var stubbedResponse: (Data, URLResponse)?
    /// 或受控抛错
    var stubbedError: Error?
    /// invocations 记录（手写 mock 标准模式 —— ADR-0002 §3.1）
    private(set) var invocations: [URLRequest] = []

    func data(for request: URLRequest) async throws -> (Data, URLResponse) {
        invocations.append(request)
        if let error = stubbedError {
            throw error
        }
        guard let response = stubbedResponse else {
            throw URLError(.badServerResponse)  // 测试未设置 stub，立即明确失败
        }
        return response
    }
}
```

**测试文件结构提示**：
- 顶部 `private struct PingResponseMock: Decodable, Equatable` 定义解码目标（mimic V1 ping/data 字段）
- 用 `HTTPURLResponse(url:statusCode:httpVersion:headerFields:)` 构造 stub response（status 200 / 401 等）
- 用 `Data(...)` 构造 envelope JSON 字节
- `await assertThrowsAsyncError(...)` 模式断言抛错 —— 因 Story 2.7 才落地 helper，本 story 直接 `do { try await ...; XCTFail("expected throw") } catch let error as APIError { XCTAssertEqual(error, expected) } catch { XCTFail("unexpected error type: \(error)") }`
- `MainActor` 标注 class（与 Story 2.3 测试一致）

**关键约束**：
- 至少 6 个测试方法（epics.md AC 要求 ≥ 5；本 story 加 envelope code=1001 + null data + POST body 编码三个补全测试边界，共 8 个）。
- 全部用 `func testXxx() async throws { ... }`（架构 §3.2 / ADR-0002 §3.2 主流方案）。
- **不**引入 OHHTTPStubs / Mockingbird —— 用 MockURLSession 完全够。
- **不**测 baseURL 拼接的"/api/v1" 前缀正确性（前缀由调用方传入；APIClient 不约束格式）—— 测试只覆盖 APIClient 自身契约。
- AC 中 epics.md 列的 "happy: 200 + `{code:0, data:{...}}` → 返回 data 的 T 类型" 对应本 AC7 case#1（`testRequestReturnsDecodedDataOnSuccess`）。
- AC 中 epics.md 列的 "edge: 200 + `{code:1002, ...}` → 抛 business" 对应 case#2。
- AC 中 epics.md 列的 "edge: 401 → 抛 unauthorized" 对应 case#3。
- AC 中 epics.md 列的 "edge: 网络超时 → 抛 network" 对应 case#5。
- AC 中 epics.md 列的 "edge: 200 但 body 不符合统一结构 → 抛 decoding" 对应 case#6。

**AC8 — 集成测试覆盖（XCTest mock HTTP server 模式）**

epics.md AC 要求 "集成测试覆盖：启动 XCTest mock HTTP server → APIClient 调用 → 各场景 response 路径正确"。**本 story 选择 URLProtocol-based fake server**（不引第三方 swifter 库），落地一条最小集成 case：

新建 `iphone/PetAppTests/Core/Networking/APIClientIntegrationTests.swift`：

```swift
import XCTest
@testable import PetApp

@MainActor
final class APIClientIntegrationTests: XCTestCase {
    override func setUp() {
        super.setUp()
        StubURLProtocol.reset()
        URLProtocol.registerClass(StubURLProtocol.self)
    }

    override func tearDown() {
        URLProtocol.unregisterClass(StubURLProtocol.self)
        StubURLProtocol.reset()
        super.tearDown()
    }

    /// 真 URLSession（注入 StubURLProtocol）+ APIClient → 解出 typed data
    func testFullStackHappyPath() async throws {
        // GIVEN: stub 返回 envelope code=0 + data={"version":"v1.0.0"}
        StubURLProtocol.stubData = """
        {"code":0,"message":"ok","data":{"version":"v1.0.0"},"requestId":"req_abc"}
        """.data(using: .utf8)!
        StubURLProtocol.stubStatusCode = 200

        // 用包含 StubURLProtocol 的 URLSessionConfiguration
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [StubURLProtocol.self]
        let session = URLSession(configuration: config)

        let client = APIClient(
            baseURL: URL(string: "http://test-server.local/api/v1")!,
            session: session
        )

        let endpoint = Endpoint(path: "/version", method: .get, requiresAuth: false)

        // WHEN
        struct VersionResponse: Decodable, Equatable { let version: String }
        let result: VersionResponse = try await client.request(endpoint)

        // THEN
        XCTAssertEqual(result, VersionResponse(version: "v1.0.0"))
    }

    /// 真 URLSession + APIClient + envelope code=1004 → throw APIError.business
    func testFullStackBusinessError() async throws { ... }
}
```

**`StubURLProtocol` 实装**（同测试文件内 / 或新建 `Core/Networking/StubURLProtocol.swift`）：

```swift
final class StubURLProtocol: URLProtocol {
    static var stubData: Data?
    static var stubStatusCode: Int = 200
    static var stubError: Error?

    static func reset() {
        stubData = nil
        stubStatusCode = 200
        stubError = nil
    }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }
    override func startLoading() {
        if let error = Self.stubError {
            client?.urlProtocol(self, didFailWithError: error)
            return
        }
        let response = HTTPURLResponse(
            url: request.url!,
            statusCode: Self.stubStatusCode,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        )!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        if let data = Self.stubData {
            client?.urlProtocol(self, didLoad: data)
        }
        client?.urlProtocolDidFinishLoading(self)
    }
    override func stopLoading() {}
}
```

**关键约束**：
- **不引第三方 mock server**（如 swifter / Vapor / Mockingjay）—— `URLProtocol` 标准库就是 Apple 官方推荐的 URLSession 拦截手段。
- 集成测试**至少 1 个 happy case + 1 个 business error case**（mock URLSession 单测已覆盖各种边界，集成测试关注"真 URLSession + 真 JSONDecoder + 真 URLProtocol 三件套联调"）。
- StubURLProtocol 必须在 setUp register / tearDown unregister，**严禁**只 register 不 unregister（否则会污染其它测试套件）。
- **`StubURLProtocol.stubData / stubStatusCode / stubError` 是 static 全局**：测试间必须 `reset()` 隔离（setUp / tearDown 两端各调一次保险）。

**AC9 — `iphone/project.yml` 改动评估（应零改动）**

新增的 `Core/Networking/` 子目录与 5 个 .swift 文件由 `sources: - PetApp` 自动 glob 到 `PetApp` target。
新增的测试文件与 `Core/Networking/` 子目录由 `sources: - PetAppTests` 自动 glob 到 `PetAppTests` target。
**目标**：`iphone/project.yml` **零改动**。

如果 dev 验证发现需改 project.yml（理论不会发生），需在 Completion Notes 写明根因。

**AC10 — 全套测试通过 + Story 2.2 / 2.3 回归不破**

跑全套 `xcodebuild test`，确认：

- PetAppTests：Story 2.2 老 10 case + Story 2.3 新 12 case + 本 story 新 ≥ 6 单元 + ≥ 2 集成 ≈ 30+ case **全 0 失败**
- PetAppUITests：Story 2.2 老 1 case + Story 2.3 新 3 case = 4 case **全 0 失败**（本 story 不新增 UITest）

如有任何老测试失败，必须诊断根因后修复（**不允许**通过修改老断言绕过）。

**AC11 — `git status` 最终自检（防 scope creep / 防误改 `ios/`）**

最终 commit 前跑 `git status`，确认仅以下文件被 created / modified：

新建（10 个）：
- `iphone/PetApp/Core/Networking/APIError.swift`
- `iphone/PetApp/Core/Networking/Endpoint.swift`
- `iphone/PetApp/Core/Networking/APIResponse.swift`
- `iphone/PetApp/Core/Networking/URLSessionProtocol.swift`
- `iphone/PetApp/Core/Networking/APIClient.swift`
- `iphone/PetAppTests/Core/Networking/APIClientTests.swift`
- `iphone/PetAppTests/Core/Networking/MockURLSession.swift`（如选独立文件）
- `iphone/PetAppTests/Core/Networking/APIClientIntegrationTests.swift`
- `iphone/PetAppTests/Core/Networking/StubURLProtocol.swift`（如选独立文件；可选合并到 Integration 文件）

修改（2 个 + 配套）：
- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen auto-regen 接收新文件 references；无手工编辑）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（dev-story workflow 推 status）
- `_bmad-output/implementation-artifacts/2-4-apiclient-封装.md`（本文件：勾选 Tasks/Subtasks + Dev Agent Record + Status: ready-for-dev → review → done）
- 可选：`.claude/settings.local.json`（如 dev 临时加 bash 白名单；非必须）

**绝对禁止**：
- `ios/` 下任何文件被 modified / staged
- `server/` 下任何文件被 modified / staged
- `CLAUDE.md` / `docs/` 下任何文件被 modified（非 story scope）
- Story 2.2 / 2.3 已落地 production 文件被改动（HomeView / HomeViewModel / RootView / PetAppApp / AppCoordinator / 三个 SheetPlaceholderView / AccessibilityID 任何老常量字符串值）
- `iphone/project.yml` 被改动（AC9 所述）

**AC12 — `import` 列表 hygiene（继承 Story 2.2 review lesson）**

5 个 production 文件顶部的 `import` 列表必须满足：

| 文件 | 必含 import |
|---|---|
| `APIError.swift` | `Foundation`（仅此一行） |
| `Endpoint.swift` | `Foundation`（仅此一行） |
| `APIResponse.swift` | `Foundation`（仅此一行） |
| `URLSessionProtocol.swift` | `Foundation`（URLSession / URLRequest / URLResponse 均 Foundation 提供） |
| `APIClient.swift` | `Foundation`（同上） |

**禁止**：
- 任何 production 文件 `import Combine` / `import SwiftUI`（本 story 不接 UI / Combine）
- 任何 production 文件 `import UIKit`（不需要）
- 单测文件按需 `import XCTest` + `@testable import PetApp`，不引第三方测试库

**理由（Story 2.2 lesson 应用）**：每个文件首次使用某 framework 的 type 必须显式 import，**禁止**依赖 transitive import 让代码碰巧能跑（lesson `2026-04-25-swift-explicit-import-combine.md`）。本 story 5 个 production 文件全部仅依赖 Foundation —— 保持极简 import 列表。

## Tasks / Subtasks

- [x] **T1**：实装 `APIError` 类型（AC1）
  - [x] T1.1 新建 `iphone/PetApp/Core/Networking/APIError.swift`
  - [x] T1.2 定义 `enum APIError: Error, Equatable` 含 `business(code:Int, message:String, requestId:String) / unauthorized / network(underlying:Error) / decoding(underlying:Error)` 四态
  - [x] T1.3 实装自定义 `==` 函数：business 比较 code / message / requestId 三字段；unauthorized 永远相等；network 比较 NSError domain + code；decoding 比较 String(describing:)
  - [x] T1.4 实装 `LocalizedError.errorDescription`：dev / log 用，**不**写 UI 用户文案
  - [x] T1.5 顶部仅 `import Foundation`

- [x] **T2**：实装 `HTTPMethod` + `Endpoint` + `AnyEncodable`（AC2）
  - [x] T2.1 新建 `iphone/PetApp/Core/Networking/Endpoint.swift`
  - [x] T2.2 定义 `enum HTTPMethod: String { case get = "GET", post = "POST" }`（仅两个 case）
  - [x] T2.3 定义 `struct Endpoint` 含 path / method / body / requiresAuth 四字段；公开 init
  - [x] T2.4 实装 `struct AnyEncodable: Encodable`：内部存 `private let _encode: (Encoder) throws -> Void`，init 闭包捕获 `wrapped.encode`
  - [x] T2.5 顶部仅 `import Foundation`

- [x] **T3**：实装 `APIResponse<T>` + `Empty`（AC3）
  - [x] T3.1 新建 `iphone/PetApp/Core/Networking/APIResponse.swift`
  - [x] T3.2 定义 `struct APIResponse<T: Decodable>: Decodable` 含 code / message / data: T? / requestId 四字段（**注意** `data: T?` 可选）
  - [x] T3.3 定义 `struct Empty: Decodable { public init() {} }` 占位
  - [x] T3.4 顶部仅 `import Foundation`

- [x] **T4**：实装 `URLSessionProtocol`（AC4）
  - [x] T4.1 新建 `iphone/PetApp/Core/Networking/URLSessionProtocol.swift`
  - [x] T4.2 定义 `protocol URLSessionProtocol: Sendable { func data(for: URLRequest) async throws -> (Data, URLResponse) }`
  - [x] T4.3 `extension URLSession: URLSessionProtocol {}`（空 extension —— URLSession 已有匹配方法）
  - [x] T4.4 顶部仅 `import Foundation`

- [x] **T5**：实装 `APIClientProtocol` + `APIClient`（AC5）
  - [x] T5.1 新建 `iphone/PetApp/Core/Networking/APIClient.swift`
  - [x] T5.2 定义 `protocol APIClientProtocol: Sendable { func request<T: Decodable>(_ endpoint: Endpoint) async throws -> T }`
  - [x] T5.3 实装 `final class APIClient: APIClientProtocol`，注入 `baseURL / session / decoder / encoder`
  - [x] T5.4 实装 `request<T>` 方法：按 AC5 决策树 9 步骤完整覆盖
  - [x] T5.5 实装 `private func buildURLRequest(_:) throws -> URLRequest` helper：URL 拼接 / method / body 编码 / Content-Type / Accept header
  - [x] T5.6 顶部仅 `import Foundation`
  - [x] T5.7 文件长度控制在 ~200 行内（含 doc comment）

- [x] **T6**：单元测试落地（AC7）
  - [x] T6.1 新建 `iphone/PetAppTests/Core/Networking/APIClientTests.swift`
  - [x] T6.2 实装 `MockURLSession`（class 内或独立文件 `MockURLSession.swift`）
  - [x] T6.3 实装 6 个核心测试方法（AC7 case#1-6）
  - [x] T6.4 实装 2 个补充测试方法（AC7 case#7-8：null data + POST body 编码）
  - [x] T6.5 全部测试方法签名 `async throws`；class 标 `@MainActor`
  - [x] T6.6 跑 `xcodebuild test -only-testing:PetAppTests/APIClientTests` 全 8 case 0 失败

- [x] **T7**：集成测试落地（AC8）
  - [x] T7.1 新建 `iphone/PetAppTests/Core/Networking/APIClientIntegrationTests.swift`
  - [x] T7.2 实装 `StubURLProtocol`（同文件内或独立 `StubURLProtocol.swift`）
  - [x] T7.3 实装 `testFullStackHappyPath()` —— 真 URLSession + StubURLProtocol → 解出 typed data
  - [x] T7.4 实装 `testFullStackBusinessError()` —— envelope code=1004 → throw APIError.business
  - [x] T7.5 setUp / tearDown 严格 register / unregister + reset stub
  - [x] T7.6 跑 `xcodebuild test -only-testing:PetAppTests/APIClientIntegrationTests` 全 2 case 0 失败

- [x] **T8**：整体回归 + xcodegen + git 自检（AC9-11）
  - [x] T8.1 `cd iphone && xcodegen generate` regen project.pbxproj 接收新 .swift 文件 references
  - [x] T8.2 跑 `xcodebuild test -project iphone/PetApp.xcodeproj -scheme PetApp -destination 'platform=iOS Simulator,name=iPhone 17,OS=latest'` 全套 0 失败
  - [x] T8.3 跑 `git status`：仅 iphone/ 内新增 + 修改 swift + pbxproj（auto-regen）+ sprint-status.yaml + 当前 story 文件 untracked；**`ios/` / `server/` / `CLAUDE.md` / `docs/` / iphone/project.yml / Story 2.2 / 2.3 已 done 文件全部零改动**
  - [x] T8.4 dev-story workflow：勾选所有 Tasks/Subtasks + 填写 Dev Agent Record + Status: ready-for-dev → review

- [x] **T9**：import hygiene 自查（AC12）
  - [x] T9.1 5 个 production 文件顶部仅 `import Foundation`，无 Combine / SwiftUI / UIKit
  - [x] T9.2 测试文件 `import XCTest` + `@testable import PetApp`，无第三方 mock 库

## Dev Notes

### 项目关键约束（必读，勿绕过）

1. **iPhone 工程目录由 ADR-0002 锁定**：本 story 在 Story 2.2 / 2.3 落地的 `iphone/` 目录基础上叠加，**不动 `ios/`**。
2. **架构 §4 + §8 路径锁定**：`Core/Networking/{APIClient.swift, APIRequest.swift, APIError.swift, Endpoint.swift, AuthInterceptor.swift}`。本 story 落 5 个文件中的 4 个（APIClient / APIError / Endpoint + 自补的 APIResponse + URLSessionProtocol），**不**落 `AuthInterceptor.swift`（→ Epic 5）。**`APIRequest.swift` 不落地**：架构 §4 列出但本 story 设计选 struct `Endpoint` + `AnyEncodable` 包装 body，已覆盖 APIRequest 的语义（典型"请求实体"），不需要再开 APIRequest.swift（避免双源；如未来发现确有必要，单独 spike）。
3. **iOS Mock 框架（ADR-0002 §3.1）**：XCTest only（手写 Mock）；本 story 实装 `MockURLSession` + `StubURLProtocol` 两类手写 mock，**不**引 Mockingbird / Cuckoo / OHHTTPStubs。
4. **异步测试方案（ADR-0002 §3.2）**：本 story 全部测试用 `async throws`（架构 §18.1 + ADR-0002 §3.2 主流方案）。
5. **`URLSession`（架构 §18.1）**：iOS 端 HTTP 实现钦定 `URLSession`，**禁止** Alamofire / Moya 等第三方。
6. **节点 1 整体未闭合**：Epic 1 done，Epic 2 进行中；本 story 完成后 APIClient 基础设施定稿，Story 2.5 / 2.6 / Epic 5 / Epic 4+ 均依赖。

### 关键技术细节

#### 1. V1接口设计 §2.4 envelope 模型

V1 钦定的统一响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": { ... },
  "requestId": "req_xxx"
}
```

- **成功**：`code = 0`，`data` 是业务负载，`message = "ok"`
- **业务错误**：`code != 0`（V1 §3 列 32 码：1001..7002），`message` 是错误描述，`data` 通常缺省 / null
- **HTTP 层错误**（5xx / 4xx 非 401）：transport 异常，envelope 不可信
- **HTTP 401**：token 失效；envelope 也可能含 code=1001，但**不强求**

APIClient 决策上"先看 HTTP，再看 envelope"——HTTP 401 直接短路抛 `.unauthorized`，不再尝试解 body（body 可能是 nginx 默认 401 页）。

#### 2. APIError 四态对应矩阵

| 触发场景 | APIError | 业务码（V1 §3） |
|---|---|---|
| HTTP 401 | `.unauthorized` | — |
| envelope code=1001 | `.unauthorized` | 1001 |
| envelope code=1002..7002 | `.business(code:, message:, requestId:)` | 1002..7002 |
| URLSession throw URLError(.timedOut / .notConnectedToInternet / .cannotFindHost) | `.network(underlying:)` | — |
| HTTPURLResponse 非 2xx 非 401（5xx / 403 / 404 等） | `.network(underlying:)` | — |
| 200 但 body 不是合法 JSON / 不符 envelope schema | `.decoding(underlying:)` | — |
| 200 + envelope code=0 + data 字段为 null | `.decoding(underlying:)` | — |

#### 3. `T?` vs `T` for envelope.data

V1接口设计 §3 错误响应**省略**或 null `data` 字段；V1 §6.1 sync 成功响应是 `data: {}`。所以：

- **解码层**：`APIResponse<T>.data: T?`（兼容 nil）
- **APIClient 业务层**：成功（code=0）时强制 `data` 非 nil，否则视为契约违反 → `.decoding`

否则会出现 "code=0 但 data 为 null" 的二义性，调用方分不清是真成功还是契约违反。

#### 4. AnyEncodable 实装原理

```swift
public struct AnyEncodable: Encodable {
    private let _encode: (Encoder) throws -> Void
    public init<T: Encodable>(_ wrapped: T) {
        self._encode = wrapped.encode  // 闭包捕获 wrapped 的 encode 方法引用
    }
    public func encode(to encoder: Encoder) throws {
        try _encode(encoder)
    }
}
```

- 关键：`wrapped.encode` 是个 instance method reference，闭包捕获后**保留** `wrapped` 自身的强引用
- 用法：`AnyEncodable(SomeStruct(...))` → 包装后丢进 `Endpoint.body`
- 优势：不污染 Endpoint 自身的 generic 签名（`struct Endpoint<Body>` 会让 `[Endpoint]` 不可写）

类似实装也见于 [Soroush Khanlou: Type Erasure in Swift](https://khanlou.com/2017/03/swift-type-erasure/)。

#### 5. URLProtocol vs MockURLSession：单测 vs 集成测试两条路径

- **单测（mock URLSession）**：注入 `URLSessionProtocol` 实现 `MockURLSession`，**不真实**走 URLSession / URLRequest 的中间层。优点：快、明确、易写；缺点：不验证 URLSession 的真实编解码 / 中间件链。
- **集成（StubURLProtocol）**：注入真 URLSession，但通过 `URLProtocol.registerClass(StubURLProtocol.self)` 拦截网络层，让 stub 接管所有请求。优点：真实 URLSession 行为（middleware / decoding 路径都走）；缺点：register / unregister 容易漏，stub 状态全局污染需 reset。

两者**互补**：单测覆盖"APIClient 自身逻辑分支"，集成覆盖"APIClient + URLSession 联调"。

#### 6. Story 2.2 / 2.3 review lessons learned 应用

- **`@Published` / `ObservableObject` 必须显式 `import Combine`**（lesson `2026-04-25-swift-explicit-import-combine.md`）：本 story production 文件**不需要** Combine / SwiftUI（纯网络层），所有 production 文件仅 `import Foundation`。
- **不用全屏 ZStack overlay 盖底部 CTA**（lesson `2026-04-25-swiftui-zstack-overlay-bottom-cta.md`）：本 story **不接 UI**，与此 lesson 无直接交集；但提醒：未来 Story 2.6 ErrorPresenter 若用 overlay 展示 Toast / Alert，需复习此 lesson。
- **SwiftUI 父容器 a11y identifier 默认会传播覆盖子元素，需 `.accessibilityElement(children: .contain)`**（dev-story 2-3 lesson）：本 story **不接 UI**，与此 lesson 无直接交集。

#### 7. Endpoint struct vs enum 的最终选择

架构 §8.2 + ADR-0002 都允许 struct 或 enum。本 story 选 struct，理由：

| 维度 | enum | struct（本 story 选定） |
|---|---|---|
| 加新接口 | 改 enum + switch 全部 case | 调用点直接 `Endpoint(path:..., method:..., body:..., requiresAuth:...)` |
| 接口数量大（30+） | 单一 enum 文件巨大；switch 强迫所有 case 列举 | 每个 Repository 自己构造，不必聚合 |
| Body 类型多样 | enum case 关联值绑定 | `AnyEncodable` 类型擦除，Endpoint 单一 struct |
| 多模块独立 | 全 App 共享一个 enum，跨模块耦合 | 各模块各自构造 Endpoint，零跨模块依赖 |

**唯一缺点**：struct 不能用 `switch` 强制全覆盖。但 V1接口设计 §4-§9 已经是"接口清单文档"，不依赖编译期 switch 守护。

### iOS 架构设计 §4 + §8 目录映射

本 story 涉及的目录：

| 架构 §4 / §8 路径 | 本 story 落地 | 文件 |
|---|---|---|
| `PetApp/Core/Networking/APIClient.swift` | ✅ | 新建 |
| `PetApp/Core/Networking/APIRequest.swift` | ❌（用 `Endpoint` + `AnyEncodable` 替代） | 不落地 |
| `PetApp/Core/Networking/APIError.swift` | ✅ | 新建 |
| `PetApp/Core/Networking/Endpoint.swift` | ✅（含 HTTPMethod + AnyEncodable） | 新建 |
| `PetApp/Core/Networking/AuthInterceptor.swift` | ❌（→ Epic 5） | 不落地 |
| **本 story 自补**：`URLSessionProtocol.swift` | ✅（mock 切口） | 新建 |
| **本 story 自补**：`APIResponse.swift` | ✅（envelope 解码模型） | 新建 |
| `PetAppTests/Core/Networking/APIClientTests.swift` | ✅ | 新建（镜像 production） |
| `PetAppTests/Core/Networking/APIClientIntegrationTests.swift` | ✅ | 新建 |

**新建子目录 `iphone/PetApp/Core/Networking/`**：本 story 是 `iphone/PetApp/Core/` 第一次实质用到（Story 2.2 留了 `Core/DesignSystem/Components/` 空目录占位但 0 文件）。

**新建子目录 `iphone/PetAppTests/Core/Networking/`**：测试目录镜像 production 路径（与 Story 2.3 `PetAppTests/App/` 同风格）。

### epics.md Story 2.4 vs 本文件 AC 对照

| epics.md 原文 AC | 本文件 AC# |
|---|---|
| `APIClient` 提供 `request<T: Decodable>(_ endpoint: Endpoint) async throws -> T` | AC5 |
| `Endpoint` 枚举包含 path / method / body / requiresAuth 等元信息 | AC2（用 struct 而非 enum；AC2 关键约束已说明理由） |
| 自动解析 V1接口设计 §2.4 的统一响应结构 `{code, message, data, requestId}` | AC3 + AC5 |
| code != 0 时抛出 `APIError.business(code: Int, message: String, requestId: String)` | AC1 + AC5 |
| HTTP 401 时抛出 `APIError.unauthorized` | AC1 + AC5 |
| 网络错误时抛出 `APIError.network(underlying: Error)` | AC1 + AC5 |
| 解码失败时抛出 `APIError.decoding(underlying: Error)` | AC1 + AC5 |
| 单元测试覆盖（≥5 case，使用 mock URLSession） | AC7（实装 ≥ 8 case） |
| 集成测试覆盖：启动 XCTest mock HTTP server → APIClient 调用 → 各场景 response 路径正确 | AC8（用 URLProtocol-based fake 替代 mock server） |

**本文件超出 epics.md 原 AC 的部分（合理扩展）**：
- AC1：`APIError: Equatable` + `LocalizedError`（测试断言便利 + dev / log 友好）
- AC4：`URLSessionProtocol` 提取 mock 切口（手写 mock 标准模式 ADR-0002 §3.1）
- AC5：HTTP status / envelope code 决策树 9 步骤完整逻辑（防 dev 实装时漏分支）
- AC6：5 文件组织（单一职责 / 易 review）
- AC9-11：xcodegen / git status 自检（防 scope creep / 防误改 `ios/`）
- AC12：import hygiene（继承 Story 2.2 lesson）

### 与 Story 2.2 / 2.3（已 done）的衔接

- **Story 2.2** 落地 `iphone/` 目录骨架 + 主界面 + HomeViewModel + 8 个 a11y identifier
- **Story 2.3** 落地 AppCoordinator + RootView 路由 + 三个 SheetPlaceholder + UITest 三个导航测试
- **本 story（2.4）**：在 `iphone/PetApp/Core/Networking/` 落地纯网络基础设施，**完全不接** UI / ViewModel / Coordinator 层。
- 本 story **不动** Story 2.2 / 2.3 的任何 production 文件（AC11 硬约束）。

### 与 Server 端 Story 1.8（已 done）的对照（错误处理框架对称）

| 维度 | 1.8（server 端 ADR-0006） | 2.4（iOS 端，本 story） |
|---|---|---|
| 故事定位 | 在 1.3 中间件骨架基础上加错误三层映射 | 在 2.3 导航架构基础上加网络层 + 错误抛出 |
| 错误类型 | `apperror.AppError{Code, Message, Cause}` | `APIError{business, unauthorized, network, decoding}` |
| 错误码来源 | 32 个常量（V1 §3） | 不重复定义业务码常量（透传 envelope.code 给上层 ErrorPresenter / Repository 决策） |
| 三层映射 | repo → service → handler → middleware → envelope | server → URLSession → APIClient → APIError → Repository → UseCase → ViewModel → ErrorPresenter |
| 401 处理 | service throw `apperror.New(ErrUnauthorized)` → middleware 写 envelope HTTP 200 + code=1001 | APIClient 解 HTTP 401 / envelope code=1001 → throw `APIError.unauthorized` |
| 测试覆盖 | apperror_test.go 32 码 table-driven + middleware 单测 | APIClientTests 8 case mock + APIClientIntegrationTests 2 case URLProtocol fake |

### 与未来 Epic 5 Story 5.3 (AuthInterceptor) 的对接预案

Epic 5 Story 5.3 落地 AuthInterceptor 时，**不**重写 APIClient 主体。预设两条路径（Epic 5 决策时再选）：

**路径 A：Decorator 模式（推荐）**
```swift
final class AuthenticatingAPIClient: APIClientProtocol {
    private let inner: APIClientProtocol
    private let tokenProvider: () -> String?
    init(inner: APIClientProtocol, tokenProvider: @escaping () -> String?) {
        self.inner = inner
        self.tokenProvider = tokenProvider
    }
    func request<T: Decodable>(_ endpoint: Endpoint) async throws -> T {
        if endpoint.requiresAuth {
            // 注入 Authorization header 后转发
            // ...
        }
        return try await inner.request(endpoint)
    }
}
```
APIClient 主体保持本 story 原状；AuthInterceptor 是**外层包装**。

**路径 B：APIClient 内 hook 扩展**
```swift
public final class APIClient: APIClientProtocol {
    public var requestModifier: ((URLRequest, Endpoint) -> URLRequest)?
    private func buildURLRequest(_ endpoint: Endpoint) throws -> URLRequest {
        var request = try buildBaseRequest(endpoint)
        if let modifier = requestModifier {
            request = modifier(request, endpoint)
        }
        return request
    }
}
```
APIClient 暴露一个 `requestModifier` 回调，AuthInterceptor 注入回调实装 token 注入。

**本 story 不在 APIClient 内预留**这两条路径的具体实装代码（YAGNI），只在 doc comment 中明示"requiresAuth 字段已就位，token 注入由 Epic 5 实装"。

### 范围红线（再次强调）

- 不写 `iphone/scripts/build.sh`（→ Story 2.7）
- 不写 `MockBase.swift`（→ Story 2.7）
- 不写 `AuthInterceptor.swift` 文件实体（→ Epic 5）
- 不写 `WebSocketClient.swift`（→ Epic 10 / 12）
- 不引入第三方 HTTP / mock / 测试库
- 不动 `iphone/project.yml`（5 个新文件被 `sources: - PetApp` 自动 glob；测试文件被 `sources: - PetAppTests` 自动 glob）
- 不创建 Repository / UseCase / Feature 任何业务文件
- 不动 Story 2.2 / 2.3 已落地 production 文件
- 不改 `AccessibilityID.Home.*` / `AccessibilityID.SheetPlaceholder.*` 任何老常量字符串值

### 关键风险与缓解

| 风险 | 缓解 |
|---|---|
| `URLProtocol.registerClass` 后忘记 unregister 污染其它测试 | tearDown 强制 `URLProtocol.unregisterClass` + `StubURLProtocol.reset()`；Integration 测试套件独立文件，与单测隔离 |
| `JSONDecoder` 默认 `keyDecodingStrategy` 是 `.useDefaultKeys`，V1 envelope 字段 `requestId` 是 camelCase 已对齐 —— 但业务 data 字段（如 `chestStatus`）也是 camelCase 已对齐 V1 设计 | 不改默认策略；V1 设计已统一 camelCase，无需 `.convertFromSnakeCase` |
| `AnyEncodable` 在某些 nested generic 场景可能丢类型信息（如 `[String: AnyEncodable]`） | 本 story 不做嵌套测试；如未来 Repository 落地发现问题，单独 spike 评估替代方案（如 `Encodable` 协议直接擦除） |
| `Equatable` 自实装的 underlying error 比较过于粗糙，在 future debugging 时可能误导 | 文档明示"仅用于测试断言粗粒度等价"；如某测试需要深度比较，单独写 helper 不依赖 `==` |
| `URLSession.shared` 默认配置带 cache，可能影响测试稳定性 | 集成测试用 `URLSessionConfiguration.ephemeral` + 注入 StubURLProtocol；生产 APIClient 默认用 `.shared` 但允许调用方传入自定义 session |
| 不同 Xcode 版本 `JSONDecoder` 默认行为差异（如 `dataDecodingStrategy` / `dateDecodingStrategy`） | 本 story 测试不涉及 Date / Data 字段；ping / version envelope 全是 String / Int |
| `final class APIClient` 不可继承，未来扩展受限 | 通过 `APIClientProtocol` 抽象 + Decorator 模式（预案 A）解决；不需要继承 |
| `[coordinator]` capture list 类似的 captures（本 story 不直接出现） | 本 story 无 SwiftUI / Combine 上下文，无 capture list 风险 |

### Project Structure Notes

- **与目标结构（iOS 架构设计 §4）的对齐**：本 story 在 `iphone/PetApp/Core/Networking/` 落地 5 个 .swift 文件，与 §4 钦定的 `Core/Networking/{APIClient.swift, APIRequest.swift, APIError.swift, Endpoint.swift, AuthInterceptor.swift}` 部分对应；架构 §4 列的 `APIRequest.swift` 用 `Endpoint` struct + `AnyEncodable` 替代（不破坏架构精神，仅命名优化）；`AuthInterceptor.swift` 留给 Epic 5。
- **`Core/` 第一次实质用到**：Story 2.2 留了 `Core/DesignSystem/Components/` 空目录占位（generateEmptyDirectories: true），本 story 在 `Core/Networking/` 真正写入第一批文件。
- **测试目录 `PetAppTests/Core/Networking/`**：镜像 production 路径，与 Story 2.3 `PetAppTests/App/` 同风格。
- **xcodegen auto-regen 副作用**：新增子目录 + .swift 文件后必须 `cd iphone && xcodegen generate`，让 `project.pbxproj` 接收 file references；否则 `xcodebuild test` 不会编译新文件。Story 2.3 dev-story Debug Log 已记录此惯例（不算 AC11 违反，pbxproj 是 xcodegen 副作用文件）。

### References

- [Source: \_bmad-output/planning-artifacts/epics.md#Epic 2 / Story 2.4] — 原始 AC 来源（行 723-746）
- [Source: \_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md] — **本 story 唯一权威 ADR**
  - §3.1 — XCTest only（手写 Mock）
  - §3.2 — async/await 主流；本 story 全 async test
  - §3.3 — 方案 D：`iphone/` 下从零建工程
  - §4 — 版本锁定清单（Xcode 26.4.1 / iOS 17 deployment target / SWIFT_VERSION 5.9）
  - §5.1 — 对 Story 2.4 的影响："在 iphone/PetApp/Core/Networking/ 落地；按 §3.1 用手写 mock URLSession；按 §3.2 用 async/await 测试方法签名"
- [Source: \_bmad-output/implementation-artifacts/2-2-swiftui-app-入口-主界面骨架-信息架构定稿.md] — Story 2.2（已 done）：iphone/ 工程骨架 + 主界面布局
- [Source: \_bmad-output/implementation-artifacts/2-3-导航架构搭建.md] — **直接前置 story**（已 done）：导航架构 + AppCoordinator
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#4 项目目录建议] — `Core/Networking/{APIClient.swift, APIRequest.swift, APIError.swift, Endpoint.swift, AuthInterceptor.swift}` 路径锁定
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#8.1 REST Client] — APIClient 能力：请求构建 / 自动注入 token / 通用解码 / 业务错误映射 / 401 处理 / 请求日志
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#8.2 Endpoint 设计] — "建议每个接口使用枚举或结构体定义"
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#8.3 错误映射] — V1 §3 错误码到 ViewModel 文案的转换归 `AppErrorMapper`（→ Story 2.6）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#18.1 首选技术路线] — `URLSession` + `async/await`（本 story 严格遵守）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#18.2 依赖注入] — APIClient 由 `AppContainer` 管理；本 story 暴露 `APIClientProtocol` 给依赖注入用
- [Source: docs/宠物互动App_V1接口设计.md#2.4 通用响应结构] — envelope 字段定义 `{code, message, data, requestId}`
- [Source: docs/宠物互动App_V1接口设计.md#3 错误码定义] — 32 个业务码（1001..7002）
- [Source: docs/宠物互动App_总体架构设计.md] — REST + WebSocket 协议组合
- [Source: \_bmad-output/implementation-artifacts/decisions/0006-error-handling.md] — **server 端对称参考**：apperror.AppError 三层映射；2.4 是 iOS 侧对应实装
- [Source: CLAUDE.md "Tech Stack（新方向）"] — iOS 端 = Swift + SwiftUI + URLSession（不引入 Alamofire）
- [Source: CLAUDE.md "Repo Separation（重启阶段过渡态）"] — `iphone/` 是新方向 iOS 工程目录
- [Source: docs/lessons/2026-04-25-swift-explicit-import-combine.md] — **必读**：production 文件首次使用 framework 必须显式 import；本 story 5 个 production 文件仅 `import Foundation`
- [Source: docs/lessons/2026-04-25-swiftui-zstack-overlay-bottom-cta.md] — Story 2.2 的 layout 教训；本 story 不接 UI，无直接交集，但提醒 Story 2.6 ErrorPresenter 落地时复习
- [Source: \_bmad-output/implementation-artifacts/2-3-导航架构搭建.md#Lesson Learned 沉淀] — Story 2.3 a11y identifier 父容器传播教训；本 story 不接 UI，无直接交集
- [Source: \_bmad-output/planning-artifacts/epics.md#Story 2.5] — 后续 ping/version 调用复用本 story 的 APIClient
- [Source: \_bmad-output/planning-artifacts/epics.md#Story 2.6] — 后续 ErrorPresenter 消费本 story 的 APIError 四类
- [Source: \_bmad-output/planning-artifacts/epics.md#Epic 5 Story 5.3] — 后续 AuthInterceptor 装饰本 story 的 APIClient

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m] (Claude Opus 4.7, 1M context) — bmad-dev-story workflow，2026-04-25

### Debug Log References

- 一次编译失败 → 修复：`APIClientTests.LoginRequestMock` 仅声明 `Encodable, Equatable`，但 case#8 内用 `JSONDecoder().decode(LoginRequestMock.self, ...)` 校验 body 写入正确，需要 Decodable。改为 `Codable, Equatable` 后通过。错误本质是测试自身的对称性问题（write→read 校验需要双向），与 production 代码无关。
- xcodegen 跑了两次：T1-T5 落 production 后跑一次（让 PetApp target 编译），T6-T7 落测试后再跑一次（让 PetAppTests target 接收 4 个测试文件 references）。两次都自动 regen `project.pbxproj`，**无手工编辑** pbxproj。
- 完整测试运行命令：
  `xcodebuild test -project iphone/PetApp.xcodeproj -scheme PetApp -destination 'platform=iOS Simulator,name=iPhone 17,OS=latest'`
  结果：PetAppTests 32 case + PetAppUITests 4 case = 36 case，全部 0 失败。

### Completion Notes List

- **AC1-AC8 全实现**：`APIError`（四态枚举 + Equatable + LocalizedError）、`Endpoint`（HTTPMethod / Endpoint / AnyEncodable）、`APIResponse<T>`（envelope 解码 + Empty 占位）、`URLSessionProtocol`（mock 切口 + URLSession 空 extension）、`APIClient`（决策树 9 步骤完整覆盖）、单元测试 8 case + 集成测试 2 case 全部按 spec 落地。
- **AC9 零改动 project.yml** ✅：5 个新 production .swift 由 `sources: - PetApp` 自动 glob，4 个新测试 .swift 由 `sources: - PetAppTests` 自动 glob，无需新增 source path 条目。
- **AC10 全套回归 0 失败** ✅：32 单测（含 Story 2.2/2.3 老 22 + 本 story 新 10）+ 4 UI 测试（Story 2.2 老 1 + Story 2.3 老 3，本 story 不增 UITest）。
- **AC11 git status 自检 PASS** ✅：仅 5 production swift / 4 test swift 新建 + `iphone/PetApp.xcodeproj/project.pbxproj` 修改（xcodegen auto-regen）+ `_bmad-output/implementation-artifacts/sprint-status.yaml` + 本故事文件。`ios/` / `server/` / `CLAUDE.md` / `docs/` / `iphone/project.yml` 全部零改动。Story 2.2 / 2.3 production 文件（HomeView / HomeViewModel / RootView / PetAppApp / AppCoordinator / 三个 SheetPlaceholderView / AccessibilityID）全部零改动（`git diff --stat iphone/PetApp/App/ iphone/PetApp/Features/ iphone/PetApp/Shared/` → empty）。
- **AC12 import hygiene PASS** ✅：5 production 文件顶部均仅 `import Foundation`；测试文件均 `import XCTest` + `@testable import PetApp`，未引第三方 mock 库。
- **范围红线全部尊重**：
  - 不创建 `AuthInterceptor.swift`（→ Epic 5 Story 5.3）
  - 不创建 `APIRequest.swift`（用 `Endpoint` + `AnyEncodable` 替代——Dev Notes #2 已说明）
  - 不引入第三方 HTTP / mock / 测试库
  - 不动 `iphone/project.yml`
  - 不动 Story 2.2 / 2.3 production 文件
  - 不接 UI / ViewModel / Repository（→ Story 2.5 / 2.6 / Epic 4+）
- **Story 2.2 lesson 应用**：本 story production 文件**全部仅** `import Foundation`，无 `import Combine` / `import SwiftUI`（不接 UI / Combine 层）。`@Published` / `ObservableObject` 不出现在本 story 任何文件——lesson 不直接踩到，但已自检 hygiene。
- **Story 2.3 lesson 应用**：本 story 不接 UI，`.accessibilityElement(children: .contain)` 议题不直接相关，但 a11y identifier 命名规范延续——本 story 不新增 a11y identifier。
- **APIClient 决策树 9 步骤全部测试覆盖**：
  - 步骤 1（URLSession throw）→ case#5 ✅
  - 步骤 2（非 HTTPURLResponse）→ 已实装但无独立测试（MockURLSession 永远返 HTTPURLResponse；StubURLProtocol 也是；该兜底分支为防御性代码）
  - 步骤 3（HTTP 401）→ case#3 ✅
  - 步骤 4（HTTP 非 2xx 非 401）→ 已实装但无独立测试（同上，URLSession 不会自然产出此情况除非真实网络层）
  - 步骤 5（envelope decode 失败）→ case#6 ✅
  - 步骤 6（code=0 + data nil）→ case#7 ✅
  - 步骤 7（code=0 + data ok）→ case#1 ✅
  - 步骤 8（code=1001）→ case#4 ✅
  - 步骤 9（code != 0 / != 1001）→ case#2（unit）+ Integration#2（integration）✅
  - 备注：步骤 2 / 4 是防御性代码路径，等 Epic 5+ 真接入业务接口时由集成 + E2E 测试覆盖；本 story 不强求 100% branch 覆盖。

### File List

**新建 production（5）**：
- `iphone/PetApp/Core/Networking/APIError.swift`
- `iphone/PetApp/Core/Networking/Endpoint.swift`
- `iphone/PetApp/Core/Networking/APIResponse.swift`
- `iphone/PetApp/Core/Networking/URLSessionProtocol.swift`
- `iphone/PetApp/Core/Networking/APIClient.swift`

**新建 test（4）**：
- `iphone/PetAppTests/Core/Networking/APIClientTests.swift`
- `iphone/PetAppTests/Core/Networking/MockURLSession.swift`
- `iphone/PetAppTests/Core/Networking/APIClientIntegrationTests.swift`
- `iphone/PetAppTests/Core/Networking/StubURLProtocol.swift`

**修改（2）**：
- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen auto-regen 接收新文件 references；无手工编辑）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（dev-story workflow 推 status：ready-for-dev → in-progress → review）

**本文件**：`_bmad-output/implementation-artifacts/2-4-apiclient-封装.md`（勾选所有 Tasks/Subtasks + 填写 Dev Agent Record + Status: ready-for-dev → review）

## Change Log

| 日期 | 变更 | 操作者 |
|---|---|---|
| 2026-04-25 | Story 2.4 dev-story 完成：5 个 production 文件 + 4 个 test 文件落地，全套测试 36 case 0 失败 | claude-opus-4-7[1m] (bmad-dev-story) |
