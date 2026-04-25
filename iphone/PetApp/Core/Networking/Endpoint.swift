// Endpoint.swift
// Story 2.4 AC2：REST 请求元信息容器。
//
// 设计选择：用 struct 而非 enum，让 Repository 按需构造 Endpoint，
// 不强求所有接口聚合在一个全局 enum 中（V1 §4-§9 ~30 个接口太多）。
// body 用 AnyEncodable 类型擦除，避免 Endpoint 染上 generic 签名。
// requiresAuth 字段为 Epic 5 AuthInterceptor 预留位，本 story 不消费。

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
/// - body 用 `Encodable` 包装的存在性容器（AnyEncodable），让 struct 是 value type 友好的。
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
