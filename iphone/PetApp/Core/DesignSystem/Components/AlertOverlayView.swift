// AlertOverlayView.swift
// Story 2.6 AC5：全屏阻塞 Alert 组件（半透明遮罩 + 居中卡片 + 单"知道了"按钮）。
//
// 视觉规范：
// - 遮罩：黑色 0.4 透明，吃掉点击不传播但**不**触发 onDismiss（必须按"知道了"才退场，对齐 SwiftUI 原生 Alert）。
// - 卡片：白色背景，圆角 16pt，padding 24，最大宽 280pt
// - 标题：headline 字号
// - 消息：body 字号，行高自适应
// - OK 按钮：填充蓝色（accentColor），白字，圆角 8pt，宽撑满
//
// a11y：卡片外层 `.accessibilityElement(children: .contain)` —— 让 VoiceOver 把卡片内子元素纳入容器，
// 但**不覆盖**子元素自己的 a11y identifier（参考 RoomPlaceholderView 已落地的模式 + Story 2.3 lesson）。

import SwiftUI

public struct AlertOverlayView: View {
    public let title: String
    public let message: String
    public let onDismiss: () -> Void

    public init(title: String, message: String, onDismiss: @escaping () -> Void) {
        self.title = title
        self.message = message
        self.onDismiss = onDismiss
    }

    public var body: some View {
        ZStack {
            Color.black.opacity(0.4)
                .ignoresSafeArea()
                // 阻断点击穿透：遮罩本身吃事件但不调 onDismiss（强制走"知道了"按钮）
                .contentShape(Rectangle())
                .onTapGesture { /* swallow */ }

            VStack(spacing: 16) {
                Text(title)
                    .font(.headline)
                    .accessibilityIdentifier(AccessibilityID.ErrorUI.alertTitle)

                Text(message)
                    .font(.body)
                    .multilineTextAlignment(.center)
                    .accessibilityIdentifier(AccessibilityID.ErrorUI.alertMessage)

                Button(action: onDismiss) {
                    Text("知道了")
                        .font(.headline)
                        .foregroundStyle(.white)
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, 12)
                        .background(
                            RoundedRectangle(cornerRadius: 8)
                                .fill(Color.accentColor)
                        )
                }
                .accessibilityIdentifier(AccessibilityID.ErrorUI.alertOKButton)
            }
            .padding(24)
            .frame(maxWidth: 280)
            .background(
                RoundedRectangle(cornerRadius: 16)
                    .fill(.background)
            )
            .accessibilityElement(children: .contain)
            .accessibilityIdentifier(AccessibilityID.ErrorUI.alertOverlay)
        }
    }
}

#if DEBUG
struct AlertOverlayView_Previews: PreviewProvider {
    static var previews: some View {
        AlertOverlayView(
            title: "提示",
            message: "数据异常，请稍后重试",
            onDismiss: {}
        )
    }
}
#endif
