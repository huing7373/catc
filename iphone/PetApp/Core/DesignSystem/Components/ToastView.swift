// ToastView.swift
// Story 2.6 AC4：顶部短暂浮现的 Toast 组件（纯渲染，不携带状态）。
//
// 显示/隐藏由 caller（ErrorPresentationHostModifier）通过 `if let toast = ...` +
// `.transition(.move(edge: .top))` 控制；本 view 只负责静态渲染。
//
// 视觉规范（hardcoded MVP 默认）：
// - 黑色半透明背景，圆角 capsule，padding 16x12
// - 白色字，subheadline 字号
// - 顶部安全区下方 12pt（由 caller 套外层 padding 控制）

import SwiftUI

public struct ToastView: View {
    public let message: String

    public init(message: String) {
        self.message = message
    }

    public var body: some View {
        Text(message)
            .font(.subheadline)
            .foregroundStyle(.white)
            .padding(.horizontal, 16)
            .padding(.vertical, 12)
            .background(
                Capsule()
                    .fill(Color.black.opacity(0.85))
            )
            .accessibilityIdentifier(AccessibilityID.ErrorUI.toastMessage)
            .accessibilityElement(children: .ignore)
            .accessibilityLabel(message)
    }
}

#if DEBUG
struct ToastView_Previews: PreviewProvider {
    static var previews: some View {
        VStack {
            ToastView(message: "已同步")
            ToastView(message: "网络异常，请检查后重试")
            ToastView(message: "登录已过期，正在重新登录...")
        }
    }
}
#endif
