// RetryView.swift
// Story 2.6 AC6：全屏 Retry placeholder 组件。
//
// 用于网络 / 严重错误后的全屏阻塞提示，含 "出错了" 标题 + 错误描述 + "重试" 按钮。
//
// 视觉规范：
// - 背景：系统 background（明暗自适应）
// - 居中布局：图标（SF Symbol exclamationmark.triangle）+ 标题 + 副标题 + 重试按钮
// - 重试按钮：边框样式（与 AlertOverlay 实心按钮区分；retry 是"用户主动选择"，alert 是"被动确认"）

import SwiftUI

public struct RetryView: View {
    public let message: String
    public let onRetry: () -> Void

    public init(message: String, onRetry: @escaping () -> Void) {
        self.message = message
        self.onRetry = onRetry
    }

    public var body: some View {
        ZStack {
            Color(.systemBackground)
                .ignoresSafeArea()

            VStack(spacing: 16) {
                Image(systemName: "exclamationmark.triangle")
                    .font(.system(size: 56))
                    .foregroundStyle(.orange)

                Text("出错了")
                    .font(.title2)
                    .fontWeight(.semibold)

                Text(message)
                    .font(.body)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.center)
                    .padding(.horizontal, 24)
                    .accessibilityIdentifier(AccessibilityID.ErrorUI.retryMessage)

                Button(action: onRetry) {
                    Text("重试")
                        .font(.headline)
                        .padding(.horizontal, 32)
                        .padding(.vertical, 12)
                        .overlay(
                            RoundedRectangle(cornerRadius: 8)
                                .stroke(Color.accentColor, lineWidth: 1.5)
                        )
                }
                .accessibilityIdentifier(AccessibilityID.ErrorUI.retryButton)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityIdentifier(AccessibilityID.ErrorUI.retryView)
    }
}

#if DEBUG
struct RetryView_Previews: PreviewProvider {
    static var previews: some View {
        RetryView(message: "网络异常，请检查后重试", onRetry: {})
    }
}
#endif
