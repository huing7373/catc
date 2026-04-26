// LaunchingView.swift
// Story 2.9: App 启动过场页（替代空白屏）。
//
// 渲染：
//   - 上方：SF Symbol "cat.fill" 大号 + 强调色
//   - 中间：文字 "正在唤醒小猫…"
//   - 下方：ProgressView() 圆形转圈
//   - 背景：浅强调色背景（区别于 HomeView 默认白）
//
// 视觉约定（epics.md AC 钦定）：
//   - 用大号 SF Symbol（不引第三方 asset）
//   - 文字 "正在唤醒小猫…"（精确字符串，UI 测试也按此定位）
//   - 圆形进度条（ProgressView() 的默认 .circular 风格）
//
// 状态：纯无状态 View（无 @State / @StateObject / @ObservedObject）。
// 由 RootView 在 launchStateMachine.state == .launching 时渲染；不订阅状态机。

import SwiftUI

public struct LaunchingView: View {

    /// epics.md AC 钦定的字面字符串（UI 测试也按此定位）。
    public static let titleText = "正在唤醒小猫…"

    public init() {}

    public var body: some View {
        ZStack {
            Color.accentColor.opacity(0.05)
                .ignoresSafeArea()

            VStack(spacing: 24) {
                Image(systemName: "cat.fill")
                    .font(.system(size: 80))
                    .foregroundStyle(.tint)
                    .accessibilityIdentifier(AccessibilityID.Launching.logo)
                    .accessibilityLabel(Text("应用 logo"))

                Text(LaunchingView.titleText)
                    .font(.body)
                    .foregroundStyle(.primary)
                    .accessibilityIdentifier(AccessibilityID.Launching.text)

                ProgressView()
                    .progressViewStyle(.circular)
                    .accessibilityIdentifier(AccessibilityID.Launching.progressIndicator)
                    .accessibilityLabel(Text("正在加载"))
            }
        }
        .accessibilityIdentifier(AccessibilityID.Launching.container)
        .accessibilityElement(children: .contain)
    }
}

#if DEBUG
struct LaunchingView_Previews: PreviewProvider {
    static var previews: some View {
        LaunchingView()
    }
}
#endif
