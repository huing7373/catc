// APIResponse.swift
// Story 2.4 AC3：V1接口设计 §2.4 统一响应结构的解码模型。
//
// data 字段做成 T? 可选——错误响应中可能缺省 / 为 null（V1 §3 错误码示例无 data）。
// 调用方（APIClient）在 code==0 时再强制 unwrap data，否则视为契约违反 → .decoding。
// Empty 占位用于 data: {} 的接口（如 ping / sync / bind-wechat）。

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
