// EmojiListResponse.swift
// Story 18.1 AC1: GET /api/v1/emojis envelope.data wire DTO.
//
// 对应 V1 §11.1 行 1764-1771 response data 结构：`data: { items: [EmojiConfig] }`.
//
// `items` non-optional `[EmojiConfig]`：server 17.4 钦定 `nil slice 强制 []` —— client 永远收到非 null
// array (V1 §11.1 行 1817 "**禁止** `items: null`"). 若实际 server 漏发或返 null，APIClient 直接抛
// APIError.decoding，符合 fail-fast (lesson 2026-04-27-home-data-fail-fast-on-unknown-enum.md).
//
// import 仅 Foundation.

import Foundation

public struct EmojiListResponse: Decodable, Equatable, Sendable {
    public let items: [EmojiConfig]

    public init(items: [EmojiConfig]) {
        self.items = items
    }
}
