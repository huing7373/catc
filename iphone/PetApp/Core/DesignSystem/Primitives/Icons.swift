// Icons.swift
// Story 37.6: ui_design primitives.jsx 内 SVG `Icons` 对象的 SF Symbol 翻译表（25 键完整集）.
//
// 设计约束：
//   - 键名严格保持 ui_design primitives.jsx 内 Icons 对象的原型驼峰写法（home / box / friends /
//     user / paw / bowl / heart / ball / footprint / plus / enter / close / back / dot / copy /
//     check / settings / sparkle / bell / chevronRight / wechat / shield / warn / diamond / trophy）
//   - 值是 SF Symbol 名（视觉近似而非像素一致；视觉精度由 Story 37.13 visual-review-checklist 人眼把关）
//   - 不暴露 SwiftUI Image 类型（让调用方决定是 `Image(systemName:)` 还是其它包装）
//   - 未匹配键查询 → log warning + 退回 `questionmark.circle`；不允许 silent fallback

import Foundation
import os.log
import SwiftUI

/// Icons: ui_design primitives.jsx 内 SVG `Icons` 对象的 SF Symbol 翻译表.
///
/// **完整 25 键映射**（与 `iphone/ui_design/source/components/primitives.jsx` 内 `Icons` 对象 1:1 对齐）：
/// 视觉差异容忍：`ball → circle.dotted` / `bowl → bowl.fill` / `paw → pawprint.fill` 等是 SF Symbol
/// 视觉**近似**而非像素一致；视觉精度由 Story 37.13 visual-review-checklist 人眼把关；**不接受**
/// dev 自行替换映射（如改成 figure.circle 等）。
///
/// **使用方式**：
///   - 取 SF Symbol 名：`Icons.mapping["home"]` 返回 "house.fill"（Optional<String>）
///   - 取 SF Symbol 名 + fallback：`Icons.symbol(for: "home")` 返回 "house.fill"，未知键返回 "questionmark.circle"
///   - SwiftUI 渲染：`Image(systemName: Icons.symbol(for: "home"))`
public enum Icons {

    /// 完整 25 键 SF Symbol 映射.
    ///
    /// 见枚举上方注释关于视觉差异容忍的说明。
    public static let mapping: [String: String] = [
        "home":         "house.fill",
        "box":          "shippingbox.fill",
        "friends":      "person.2.fill",
        "user":         "person.crop.circle.fill",
        "paw":          "pawprint.fill",
        // User-authorized substitution (2026-04-30): 原 spec AC2 钦定 "bowl.fill"，
        // 但 iOS 17+ Apple SF Symbols SDK 不提供该 symbol（dev-story HALT 双路验证）；
        // "fork.knife" 实存且语义最贴近喂食按钮；详见 docs/lessons/ + Story 37.6 AC2 注解。
        "bowl":         "fork.knife",
        "heart":        "heart.fill",
        "ball":         "circle.dotted",
        "footprint":    "figure.walk",
        "plus":         "plus.circle.fill",
        "enter":        "arrow.right.circle.fill",
        "close":        "xmark.circle.fill",
        "back":         "chevron.left",
        "dot":          "circle.fill",
        "copy":         "doc.on.doc.fill",
        "check":        "checkmark.circle.fill",
        "settings":     "gearshape.fill",
        "sparkle":      "sparkles",
        "bell":         "bell.fill",
        "chevronRight": "chevron.right",
        "wechat":       "message.fill",
        "shield":       "shield.fill",
        "warn":         "exclamationmark.triangle.fill",
        "diamond":      "diamond.fill",
        "trophy":       "trophy.fill",
    ]

    /// fallback SF Symbol（iOS 17+ 全部存在；用于未匹配键查询）.
    public static let fallbackSymbol: String = "questionmark.circle"

    /// 根据 ui_design 键名取 SF Symbol；未匹配键返回 fallback + log warning（不允许 silent fallback）.
    ///
    /// - Parameter key: ui_design primitives.jsx 内 Icons 对象的键名（驼峰）
    /// - Returns: 对应 SF Symbol 名；未匹配返回 `fallbackSymbol`
    public static func symbol(for key: String) -> String {
        if let symbol = mapping[key] {
            return symbol
        }
        // 未匹配键：log warning + 退回 fallback；让调用站点漂移有 log 信号
        os_log(
            .error,
            "Icons.symbol(for:) unknown key: %{public}@; returning fallback %{public}@",
            key,
            fallbackSymbol
        )
        return fallbackSymbol
    }
}

// MARK: - Preview (AC8: 双主题视觉抽样)

#if DEBUG
private struct IconsPreview_Grid: View {
    @Environment(\.theme) private var theme

    private let columns: [GridItem] = Array(repeating: GridItem(.flexible(), spacing: 12), count: 5)

    var body: some View {
        ScrollView {
            LazyVGrid(columns: columns, spacing: 16) {
                ForEach(Array(Icons.mapping.keys.sorted()), id: \.self) { key in
                    VStack(spacing: 6) {
                        Image(systemName: Icons.symbol(for: key))
                            .font(.system(size: 22, weight: .semibold))
                            .foregroundColor(theme.colors.accent)
                            .frame(height: 28)
                        Text(key)
                            .font(theme.typography.caption.font)
                            .foregroundColor(theme.colors.inkSoft)
                            .lineLimit(1)
                            .minimumScaleFactor(0.6)
                    }
                    .padding(8)
                    .frame(maxWidth: .infinity)
                    .background(
                        RoundedRectangle(cornerRadius: 12)
                            .fill(theme.colors.surface)
                    )
                }
            }
            .padding(16)
        }
        .background(theme.colors.pageBg)
    }
}

#Preview("Icons — candy") {
    IconsPreview_Grid()
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("Icons — dark") {
    IconsPreview_Grid()
        .environment(\.theme, ThemeName.dark.theme)
}
#endif
