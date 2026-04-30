// Avatar.swift
// Story 37.6: 圆形头像 + 光环描边 + 在线小绿点，对齐 ui_design primitives.jsx `Avatar` 函数.

import SwiftUI

/// Avatar: 圆形头像（占位首字母 + 7 色调色板 hash 选色 + 光环描边 + 在线小绿点）.
public struct Avatar: View {
    @Environment(\.theme) private var theme

    private let name: String
    private let size: CGFloat
    private let color: Color?      // 显式覆写背景色；nil 时走 hash
    private let online: Bool?      // nil 不渲染小点；true 绿色；false 灰色
    private let ring: Bool         // 光环描边开关

    /// 7 色调色板（来自 ui_design primitives.jsx Avatar palette；本期不抽到 theme，
    /// 因为是 Avatar 专属占位策略而非 theme color）.
    private static let palette: [Color] = [
        Color(red: 1.00, green: 0.70, blue: 0.76),     // #ffb3c1
        Color(red: 1.00, green: 0.84, blue: 0.65),     // #ffd6a5
        Color(red: 0.79, green: 1.00, blue: 0.75),     // #caffbf
        Color(red: 0.74, green: 0.70, blue: 1.00),     // #bdb2ff
        Color(red: 0.63, green: 0.77, blue: 1.00),     // #a0c4ff
        Color(red: 1.00, green: 0.78, blue: 0.87),     // #ffc8dd
        Color(red: 0.72, green: 0.88, blue: 0.82),     // #b8e0d2
    ]

    /// 离线灰色（来自 ui_design primitives.jsx Avatar offline indicator color #c3bdb9）.
    private static let offlineColor: Color = Color(
        red: 0xC3 / 255.0, green: 0xBD / 255.0, blue: 0xB9 / 255.0
    )

    public init(
        name: String,
        size: CGFloat = 44,
        color: Color? = nil,
        online: Bool? = nil,
        ring: Bool = false
    ) {
        self.name = name
        self.size = size
        self.color = color
        self.online = online
        self.ring = ring
    }

    public var body: some View {
        let bg = color ?? Self.palette[hashIndex(of: name)]
        let initial = String((name.first ?? "?")).uppercased()

        ZStack {
            Circle()
                .fill(bg)
            if ring {
                Circle()
                    .strokeBorder(theme.colors.surface, lineWidth: 3)
                Circle()
                    .strokeBorder(theme.colors.accent, lineWidth: 2)
                    .padding(-2)
            } else {
                // ui_design `primitives.jsx:196` 非 ring 分支：
                //   boxShadow: 'inset 0 -2px 0 rgba(0,0,0,0.08)'
                // SwiftUI 无原生 inset shadow（iOS 18 才有 `.innerShadow`），
                // 用底部 2pt 内向渐变模拟：从中心透明 → 底部 black.opacity(0.08).
                Circle()
                    .fill(
                        LinearGradient(
                            stops: [
                                .init(color: .clear, location: 0.85),
                                .init(color: Color.black.opacity(0.08), location: 1.0),
                            ],
                            startPoint: .top,
                            endPoint: .bottom
                        )
                    )
            }
            Text(initial)
                .font(.system(size: size * 0.4, weight: .heavy, design: .rounded))
                .foregroundColor(Color.black.opacity(0.55))

            if let online {
                onlineDot(online: online)
            }
        }
        .frame(width: size, height: size)
    }

    /// 根据 name 字符 ASCII 求和取 palette index（与 ui_design primitives.jsx hash 算法一致）.
    private func hashIndex(of s: String) -> Int {
        let sum = s.unicodeScalars.reduce(0) { $0 + Int($1.value) }
        return abs(sum) % Self.palette.count
    }

    /// 右下角在线小绿点（在线 success 色 / 离线灰色）.
    @ViewBuilder
    private func onlineDot(online: Bool) -> some View {
        let dotSize = size * 0.28
        Circle()
            .fill(online ? theme.colors.success : Self.offlineColor)
            .frame(width: dotSize, height: dotSize)
            .overlay(
                Circle()
                    .stroke(theme.colors.surface, lineWidth: 2.5)
            )
            .offset(x: size * 0.32, y: size * 0.32)
    }
}

// MARK: - Preview (AC8: 双主题视觉抽样 + 4 配置)

#if DEBUG
private struct AvatarPreview_Sample: View {
    @Environment(\.theme) private var theme

    var body: some View {
        VStack(spacing: 24) {
            HStack(spacing: 16) {
                VStack {
                    Avatar(name: "Mocha", size: 56)
                    Text("default")
                        .font(theme.typography.caption.font)
                        .foregroundColor(theme.colors.inkSoft)
                }
                VStack {
                    Avatar(name: "Latte", size: 56, ring: true)
                    Text("ring")
                        .font(theme.typography.caption.font)
                        .foregroundColor(theme.colors.inkSoft)
                }
                VStack {
                    Avatar(name: "Espresso", size: 56, online: true)
                    Text("online")
                        .font(theme.typography.caption.font)
                        .foregroundColor(theme.colors.inkSoft)
                }
                VStack {
                    Avatar(name: "Brew", size: 56, online: false)
                    Text("offline")
                        .font(theme.typography.caption.font)
                        .foregroundColor(theme.colors.inkSoft)
                }
            }
            HStack(spacing: 12) {
                Avatar(name: "A", size: 36)
                Avatar(name: "B", size: 44)
                Avatar(name: "Cookie", size: 88, online: nil, ring: true)
            }
        }
        .padding(20)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(theme.colors.pageBg)
    }
}

#Preview("Avatar — candy") {
    AvatarPreview_Sample()
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("Avatar — dark") {
    AvatarPreview_Sample()
        .environment(\.theme, ThemeName.dark.theme)
}
#endif
