// ChestOpenRequest.swift
// Story 21.3 AC1: V1 §7.2 POST /api/v1/chest/open 请求 wire DTO.
//
// 契约源（V1 §7.2 r15 review 已冻结，**禁止**改字段）:
// - idempotencyKey: string，必填，1 ≤ length ≤ 128；字符集 [A-Za-z0-9_:-]
//
// 字段类型选择:
// - idempotencyKey: String（Codable encode 直接 JSON 字符串）；client 用 UUID v4 字面量
//   （形如 "E621E1F8-C36C-495A-93FC-0C247A3E6E5F"，36 字符长度落入 1-128 区间，且全字符
//   `A-F0-9` + `-` 满足字符集，无需额外清洗）.

import Foundation

public struct ChestOpenRequest: Encodable, Sendable, Equatable {
    public let idempotencyKey: String

    public init(idempotencyKey: String) {
        self.idempotencyKey = idempotencyKey
    }
}
