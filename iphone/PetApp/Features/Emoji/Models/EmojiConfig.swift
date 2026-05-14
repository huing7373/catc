// EmojiConfig.swift
// Story 18.1 AC1: Emoji wire DTO + domain model (合一)，对齐 V1 §11.1 `data.items[].*` 4 字段.
//
// 字段名严格小驼峰对齐 server JSON —— server 17.4 落地的 emojis_handler.go 已做 DTO 转换
// (`asset_url` → `assetUrl`、`sort_order` → `sortOrder`)；client 直接小驼峰匹配，**不**走
// `CodingKeys` snake_case 转换路径 (Dev Note #1 钦定).
//
// 4 字段全部 non-optional：
//   - `code`: V1 §11.1 行 1746 钦定 length 1-64 / 字符集 `[a-z0-9_-]`
//   - `name`: V1 §11.1 行 1748 钦定 length 1-64
//   - `assetUrl`: V1 §11.1 行 1750 钦定 length 1-255 / **禁止**空字符串 (17.3 seed 保证非空)
//   - `sortOrder`: V1 §11.1 行 1752 钦定 int / 0 ≤ value ≤ 2^31-1
//
// `Identifiable.id` = `code` (V1 §11.1 `code` UNIQUE KEY 保证全局唯一；SwiftUI `ForEach` 直接绑定).
// `Sendable` 让 actor / Task 边界传递安全 (LoadEmojisUseCase actor 内缓存 + ViewModel @MainActor 边界).
//
// import 仅 Foundation：无 Combine / SwiftUI 依赖
// (lesson 2026-04-25-swift-explicit-import-combine.md 钦定 import 严格按需).

import Foundation

public struct EmojiConfig: Decodable, Equatable, Identifiable, Sendable {
    public let code: String
    public let name: String
    public let assetUrl: String
    public let sortOrder: Int

    /// Identifiable 协议要求；`code` 作为 UNIQUE KEY 直接当 id 用 (V1 §11.1 钦定).
    public var id: String { code }

    public init(code: String, name: String, assetUrl: String, sortOrder: Int) {
        self.code = code
        self.name = name
        self.assetUrl = assetUrl
        self.sortOrder = sortOrder
    }
}
