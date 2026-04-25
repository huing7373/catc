// APIClient.swift
// Story 2.4 AC5：统一 REST 客户端，URLSession + JSON envelope 解析。
//
// 决策树（依次走，先匹配先抛）：
//   1. URLSession throw → .network
//   2. 非 HTTPURLResponse → .network
//   3. status == 401 → .unauthorized
//   4. status ∉ 2xx → .network
//   5. envelope decode 失败 → .decoding
//   6. envelope.code == 0 + data nil → .decoding
//   7. envelope.code == 0 + data ok → return data
//   8. envelope.code == 1001 → .unauthorized（envelope-level 401 别名）
//   9. envelope.code ∈ {其它} → .business
//
// 不在本 story 范围：Authorization header（→ Epic 5）、重试 / 限流 / 缓存 / 日志（MVP 不做或 → Story 2.7）。

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
/// 4. 解析 HTTP status：见决策树
/// 5. 解 envelope（APIResponse<T>）
/// 6. URLError 透传：底层 URLSession throw 出来的 URLError 包装成 APIError.network
///
/// 不在本 story 范围内（→ 后续 story / Epic）：
/// - 不注入 Authorization header（→ Epic 5 AuthInterceptor）
/// - 不重试（→ MVP 不做）
/// - 不限流（→ MVP 不做）
/// - 不做 request / response 日志（→ Story 2.7 测试基础设施落地 logger 后再对接）
/// - 不缓存（→ MVP 不做）
public final class APIClient: APIClientProtocol {
    private let baseURL: URL
    private let session: URLSessionProtocol

    public init(
        baseURL: URL,
        session: URLSessionProtocol = URLSession.shared
    ) {
        // 规范化 baseURL：去掉 trailing slash，保证后续 `baseURL + endpoint.path` 拼接
        // 不会产生 `.../api/v1//version` 双斜杠。
        // 调用方传 `https://api.example.com/api/v1` 或 `https://api.example.com/api/v1/`
        // 都被吸收成无 trailing slash 形式；endpoint.path 必须以 `/` 开头（Endpoint 自带契约）。
        self.baseURL = Self.normalize(baseURL)
        self.session = session
    }

    /// 去掉 baseURL 的 trailing slash（保留 scheme / host / path 其余部分）。
    /// 失败回退原 URL（极少见——absoluteString 总是合法 URL string）。
    private static func normalize(_ url: URL) -> URL {
        let s = url.absoluteString
        guard s.hasSuffix("/") else { return url }
        let trimmed = String(s.dropLast())
        return URL(string: trimmed) ?? url
    }

    // MARK: - Coder factory
    //
    // 注：每次请求新建 JSONDecoder / JSONEncoder 而不是共享实例。
    // 理由：APIClientProtocol 标了 `Sendable`，而 Foundation 的 JSONDecoder / JSONEncoder
    // 是 reference type、未标 `Sendable`。虽然现代 Foundation（iOS 15+）的 decode/encode
    // 在实现上是 thread-safe 的，但这点不在 SDK 公开契约里，Swift 6 strict concurrency 不会
    // 自动认可它满足 `Sendable` 语义。每请求新建实例：
    //   - 抹平歧义（不依赖任何 Apple 内部并发保证）
    //   - 构造开销可忽略（< 1µs，远小于一次网络 I/O）
    //   - 未来需要定制 keyDecodingStrategy / dateDecodingStrategy 时改一处即可
    private func makeDecoder() -> JSONDecoder { JSONDecoder() }
    private func makeEncoder() -> JSONEncoder { JSONEncoder() }

    public func request<T: Decodable>(_ endpoint: Endpoint) async throws -> T {
        let urlRequest = try buildURLRequest(endpoint)

        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: urlRequest)
        } catch let urlError as URLError {
            throw APIError.network(underlying: urlError)
        } catch let apiError as APIError {
            // 由 buildURLRequest 等已包装好的错误（不应发生但兜底）
            throw apiError
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

        // 解 envelope（每请求新建 decoder，见上文 makeDecoder 注释）
        let envelope: APIResponse<T>
        do {
            envelope = try makeDecoder().decode(APIResponse<T>.self, from: data)
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
        // baseURL 在 init 里被 normalize 过（无 trailing slash），endpoint.path 必须 `/` 开头，
        // 故 absoluteString 直接拼接结果一定形如 `https://host/api/v1/path`，不会出现双斜杠。
        // 不用 appendingPathComponent：它会丢前导 `/`、还会对 path 内的特殊字符做奇怪转义。
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
                // 每请求新建 encoder（见 makeEncoder 注释）
                request.httpBody = try makeEncoder().encode(body)
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
