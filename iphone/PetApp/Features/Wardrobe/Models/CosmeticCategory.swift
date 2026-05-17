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

    // MARK: - Story 24.1: server slot int → CosmeticCategory 映射

    /// 由 server `HomeEquip.slot`（V1 §6.8 枚举 `{1,2,3,4,5,6,7,99}`，语义
    /// `1=hat / 2=gloves / 3=glasses / 4=neck / 5=back / 6=body / 7=tail / 99=other`）
    /// 映射到 client 5 分类 enum（`hat / bow / scarf / outfit / bg`）.
    ///
    /// 映射规则（client 5 桶 < server 8 slot，需归并；规则取"语义最近"原则）：
    ///   - slot 1 (hat)   → .hat   （帽子）
    ///   - slot 4 (neck)  → .scarf （颈部 ≈ 围巾）
    ///   - slot 6 (body)  → .outfit（身体 ≈ 服装）
    ///   - slot 5 (back)  → .bg    （背饰归入背景桶 —— client 仅 5 桶下的最近落点）
    ///   - slot 2/3/7/99 + 任何未知值 → .bow（饰品 = 配饰兜底桶）
    ///
    /// **fallback 不丢实例**（V1 §8.2「已拥有不得静默丢失」精神的 client 侧延续）：
    /// 未知 slot 归 `.bow` 兜底桶（宁可错分类也不静默丢；与 server 态 C `slot=99` 降级精神一致）.
    public static func category(forSlot slot: Int) -> CosmeticCategory {
        switch slot {
        case 1:  return .hat
        case 4:  return .scarf
        case 6:  return .outfit
        case 5:  return .bg
        default: return .bow  // slot 2/3/7/99 + 未知值兜底（不丢实例）
        }
    }

    // MARK: - Story 24.1: 分类 badge count clamp（可单测纯函数）

    /// 分类 Tab badge / grid 计数文案（`count > 99` → "99+"；否则 `"\(count)"`）.
    ///
    /// 抽为纯函数让 XCTest 直接断言边界值（ADR-0002 §3.1 测试栈 XCTest only，
    /// 禁 ViewInspector —— 不在 `WardrobeScaffoldView` 内联三元后用 view 内省测）.
    /// `WardrobeScaffoldView.categoryTabButton` badge 文案改调本函数.
    public static func badgeText(forCount count: Int) -> String {
        count > 99 ? "99+" : "\(count)"
    }
}
