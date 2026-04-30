// Environment+Theme.swift
// Story 37.5 AC7: EnvironmentKey 注入入口 —— 让子视图通过 `@Environment(\.theme) var theme` 取主题.

import SwiftUI

/// EnvironmentKey for Theme: 让子视图通过 `@Environment(\.theme) var theme` 取主题.
///
/// **default value**: `.candy`（与 RootView `@State currentTheme = .candy` 默认一致）.
/// 这意味着即使父视图忘了写 `.environment(\.theme, ...)` modifier，子视图取 theme 也不 crash,
/// 而是取 candy 默认值——与 SwiftUI 其它 EnvironmentKey 默认值习惯一致.
///
/// **注入路径**:
///   RootView 内 `MainTabView().environment(\.theme, currentTheme.theme)` 把 currentTheme 对应的
///   Theme 实例（`.candy` / `.matcha` / `.sky` / `.dark`）写入子树 environment.
///
/// **取值路径**:
///   Feature View 写 `@Environment(\.theme) var theme`; Sample 调用如
///   `RoundedRectangle(cornerRadius: theme.radius.cardLg).fill(theme.colors.surface)`.
private struct ThemeEnvironmentKey: EnvironmentKey {
    static let defaultValue: Theme = .candy
}

extension EnvironmentValues {
    /// `@Environment(\.theme) var theme` 的取值入口.
    public var theme: Theme {
        get { self[ThemeEnvironmentKey.self] }
        set { self[ThemeEnvironmentKey.self] = newValue }
    }
}
