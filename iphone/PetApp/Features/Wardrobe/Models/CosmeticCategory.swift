// CosmeticCategory.swift
// Story 37.9 AC3: 5 分类 enum（对齐 ui_design wardrobe.jsx categories 数组）.
//
// CaseIterable + Identifiable: 让 ForEach + a11y identifier 自动衍生（accessibilityIdentifier "wardrobeCategory_\(rawValue)"）.
// rawValue 与 ui_design wardrobe.jsx:9-13 categories[].id 严格对齐（hat / bow / scarf / outfit / bg）—— 不动 raw,
// 让本 story 与 ui_design CSS 视觉源 1:1 翻译时可直接 `state.selectedCategory.rawValue` 拼 a11y id 字符串.

import Foundation

public enum CosmeticCategory: String, CaseIterable, Identifiable, Sendable {
    case hat
    case bow
    case scarf
    case outfit
    case bg

    public var id: String { rawValue }

    /// Tab label 显示名（ui_design wardrobe.jsx:9-13 钦定）.
    public var label: String {
        switch self {
        case .hat:    return "帽子"
        case .bow:    return "饰品"
        case .scarf:  return "围巾"
        case .outfit: return "服装"
        case .bg:     return "背景"
        }
    }

    /// Tab icon emoji（ui_design wardrobe.jsx:9-13 钦定）.
    public var iconEmoji: String {
        switch self {
        case .hat:    return "🎩"
        case .bow:    return "🎀"
        case .scarf:  return "🧣"
        case .outfit: return "👘"
        case .bg:     return "🏞️"
        }
    }
}
