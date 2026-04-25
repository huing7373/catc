// APIError.swift
// Story 2.4 AC1：APIClient 抛出的统一错误类型。
//
// 四态对应 V1接口设计 §2.4 envelope 解析的四种失败路径。
// 范围红线：纯数据类型，仅 import Foundation；不涉及 UI / Combine。
// UI 文案策略归 Story 2.6 ErrorPresenter，本文件 errorDescription 仅用于 dev / log。

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

    /// 自定义 Equatable：underlying error 比较只对比 NSError domain/code 或 String(describing:)。
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
