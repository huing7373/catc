// FadeIn.swift
// Story 37.6: 渐入 + 上移 8pt 入场动效，对齐 ui_design primitives.jsx `FadeIn` 函数 +
// ui_design README §Interactions §动画 "Tab 切换：内容淡入 + 上移（fadeIn 0.28s ease）".
//
// 方向契约（来自 ui_design `screens/home.jsx:101-102` `@keyframes fadeIn`）：
//   from { opacity: 0; transform: translateY(8px); }  // 从下方 +8pt
//   to   { opacity: 1; transform: translateY(0); }    // 升至原位
// 故 SwiftUI 实现：`offset(y:)` 由 `+8 → 0`（视觉上"由下向上升起"）；
// **绝不**反向（offset(y: -8) → 0 是从上向下落，会与 ui_design 契约冲突）.

import SwiftUI

/// FadeInModifier: 渐入 + 上移 8pt 入场动效 ViewModifier.
///
/// 0.28s easeInOut；从 opacity 0 + offsetY +8 渐入到 opacity 1 + offsetY 0
/// （即"由下向上升起"，对齐 ui_design `screens/home.jsx` `@keyframes fadeIn`）.
/// 触发：onAppear；id 变化时重触发（SwiftUI 通过 `.id(id)` 重建子树 → 重走 onAppear）.
public struct FadeInModifier: ViewModifier {
    /// ui_design 契约：起点 offsetY = +8pt（下方）；终点 = 0（原位）.
    /// 来源：`iphone/ui_design/source/screens/home.jsx:101-102` `@keyframes fadeIn`
    /// `from { opacity: 0; transform: translateY(8px); } to { opacity: 1; transform: translateY(0); }`
    /// **绝不可改成负数**（负数 = 从上向下落，反向；ui_design 钦定"由下向上升起"）.
    public static let offsetStartY: CGFloat = 8
    /// ui_design 契约：终点 offsetY = 0.
    public static let offsetEndY: CGFloat = 0

    private let id: AnyHashable?

    @State private var visible: Bool = false

    public init(id: AnyHashable? = nil) {
        self.id = id
    }

    @ViewBuilder
    public func body(content: Content) -> some View {
        // 仅当 id 非 nil 时挂 .id(...)；否则不挂 explicit identity.
        // 守护意图：fix-review round 4 / [P2] — 早先版本对 nil id 也调 .id(nil)，
        // 会让所有 fadeIn() 默认参 sibling views 共享同一 explicit identity（nil），
        // SwiftUI 据此做 diffing 会引发 state retention bug（视图状态被错误重用）.
        // ui_design 契约（home.jsx fadeIn keyframes）只规定 offset/opacity 行为，
        // 不要求 explicit identity；故 nil 路径走 implicit identity（位置/类型）安全.
        let core = content
            .opacity(visible ? 1 : 0)
            .offset(y: visible ? Self.offsetEndY : Self.offsetStartY)
            .onAppear {
                withAnimation(.easeInOut(duration: 0.28)) {
                    visible = true
                }
            }
        if let id = id {
            core.id(id)
        } else {
            core
        }
    }
}

extension View {
    /// 应用 FadeInModifier 到当前视图.
    /// - Parameter id: 可选 id；变化时重触发动画（用于 Tab 切换等场景）.
    public func fadeIn(id: AnyHashable? = nil) -> some View {
        modifier(FadeInModifier(id: id))
    }
}

// MARK: - Preview (AC8: 双主题视觉抽样)

#if DEBUG
private struct FadeInPreview_Sample: View {
    @Environment(\.theme) private var theme
    @State private var rebuildKey: Int = 0

    var body: some View {
        VStack(spacing: 20) {
            Card {
                VStack(alignment: .leading, spacing: 8) {
                    Text("FadeIn 卡片")
                        .font(theme.typography.cardTitle.font)
                        .foregroundColor(theme.colors.ink)
                    Text("0.28s easeInOut + offsetY +8 → 0（由下向上升起）")
                        .font(theme.typography.body.font)
                        .foregroundColor(theme.colors.inkSoft)
                }
            }
            .fadeIn(id: rebuildKey)

            Button("重触发动画") {
                rebuildKey += 1
            }
            .font(theme.typography.body.font)
            .foregroundColor(theme.colors.accentDeep)
        }
        .padding(20)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(theme.colors.pageBg)
    }
}

#Preview("FadeIn — candy") {
    FadeInPreview_Sample()
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("FadeIn — dark") {
    FadeInPreview_Sample()
        .environment(\.theme, ThemeName.dark.theme)
}
#endif
