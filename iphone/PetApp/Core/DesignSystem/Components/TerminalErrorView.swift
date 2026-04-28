// TerminalErrorView.swift
// Story 5.5 round 8 [P1] fix：terminal/unrecoverable error 的静态全屏 fallback page。
//
// 跟 RetryView / AlertOverlayView 的关键区别：
// - RetryView：有"重试"按钮，触发 stateMachine.retry()。适合 transient（network / 1009 等）.
// - AlertOverlayView：dismiss-able overlay，按 OK 触发 onDismiss closure。适合非 bootstrap
//   路径的 business error（ErrorPresenter.present(...) 那条线）。
// - **TerminalErrorView**：**无任何按钮**。屏幕静态展示错误 + 引导 user 主动杀进程重启 App。
//
// **为什么要单独引入这个 view**（5 轮 fix-review 反复栽倒同一坑的痛苦教训）：
// bootstrap 阶段的 terminal class 错误（`.unauthorized` / `.missingCredentials` /
// `.decoding` / permanent business error）—— retry **救不回**：
//   - `.missingCredentials`: keychain 真没 token,重试 useCase.execute() 仍走同一条 path 抛同样错
//   - `.unauthorized`: AuthRetryingAPIClient 已 exhaust 唯一一次静默重登,bootstrap retry 仍 401
//   - `.decoding`: server payload 与 client schema 不兼容,client 端 retry 改不了 schema
//   - permanent business code (1004/2001/...): server 拒绝是基于持久状态,retry 拿同样回应
// 把这些 case 路由到任何"dismiss-able overlay"（AlertOverlayView）都会出问题：
//   - dismiss → retry：死循环（round 3 / round 7 / round 8）
//   - dismiss → no-op：卡死（round 4）
//   - dismiss → exit(0)：iOS HIG 反模式 + App Store 审核拒（round 5）
//   - dismiss → 提示 user 杀进程：user 仍可能误点重试（仍可能死循环）
// 唯一可调和的方案：**根本不给 dismiss 入口**，让 user 必须主动 force-quit。
// 这是 iOS error boundary 模式（terminal error = full-screen static page = user 主动退出）.
//
// 详见 docs/lessons/2026-04-27-bootstrap-terminal-error-static-fallback-page.md
//   + docs/lessons/2026-04-27-bootstrap-alert-dismiss-must-be-user-driven-recovery.md（前轮 lesson 的延续）.
//
// 视觉规范：
// - 背景：systemBackground（明暗自适应）
// - 居中布局：大号 SF Symbol exclamationmark.triangle.fill (orange) + 标题 + 正文 + 底部静态指引文本
// - **无任何 Button** —— 关键：user 唯一退路是杀进程
// - a11y：整个视图聚合成单 a11y element 让 VoiceOver 一次性朗读，accessibilityIdentifier
//   暴露给未来 UITest

#if !os(macOS)
import SwiftUI

/// `TerminalErrorView`：terminal/unrecoverable error 的静态全屏 fallback page.
///
/// 跟 `RetryView` 的区别：`RetryView` 有"重试"按钮（用户主动点 → `stateMachine.retry()`）；
/// `TerminalErrorView` **无任何按钮**，user 必须主动杀进程才能退出 App.
///
/// **使用场景**：仅 bootstrap 路径的 terminal-class 错误
/// （`.unauthorized` / `.missingCredentials` / `.decoding` / permanent business error）.
/// 非 bootstrap 路径仍用 `AlertOverlayView` (dismiss-able overlay 适合 transient business error
/// 由 ErrorPresenter 自己管理 dismiss).
///
/// **历史**：`AlertOverlayView` 用作 bootstrap terminal error 已经迭代 5 轮（round 3 / 4 / 5 / 7 / 8）
/// 仍走偏 —— dismiss 行为不可调和（详见文件头注释 / lesson 文档）.
public struct TerminalErrorView: View {
    public let title: String
    public let message: String

    public init(title: String, message: String) {
        self.title = title
        self.message = message
    }

    public var body: some View {
        VStack(spacing: 24) {
            Image(systemName: "exclamationmark.triangle.fill")
                .font(.system(size: 56))
                .foregroundColor(.orange)

            Text(title)
                .font(.title2)
                .bold()
                .multilineTextAlignment(.center)
                .accessibilityIdentifier(AccessibilityID.ErrorUI.terminalTitle)

            Text(message)
                .font(.body)
                .foregroundColor(.secondary)
                .multilineTextAlignment(.center)
                .padding(.horizontal, 16)
                .accessibilityIdentifier(AccessibilityID.ErrorUI.terminalMessage)

            // 底部静态指引：告知 user 唯一退路是杀进程.
            // 故意不做按钮 —— 一旦做按钮 (无论调 retry / no-op / exit(0) / 提示) 都会回到 round 3-7 的
            // dismiss 行为问题域. 静态文本避免引诱 user 点击 → 强制 user 走 OS 级 force-quit.
            Text("请双击 Home 键 / 上滑底部小条杀进程后重新启动 App")
                .font(.caption)
                .foregroundColor(.gray)
                .multilineTextAlignment(.center)
                .padding(.top, 16)
                .accessibilityIdentifier(AccessibilityID.ErrorUI.terminalHelp)
        }
        .padding(32)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(Color(UIColor.systemBackground))
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier(AccessibilityID.ErrorUI.terminalView)
    }
}

#if DEBUG
struct TerminalErrorView_Previews: PreviewProvider {
    static var previews: some View {
        TerminalErrorView(
            title: "提示",
            message: "登录失败，请重新启动应用"
        )
    }
}
#endif

#endif
