// ErrorPresentationHostModifier.swift
// Story 2.6 AC7：把 ErrorPresenter.current 投影成对应 SwiftUI 组件的 ViewModifier。
//
// 用法：在 RootView body 最外层 `.errorPresentationHost(presenter: container.errorPresenter)`。
//
// 渲染策略（按 ErrorPresentation case）：
// - `.toast(message)` → ToastView 顶部浮现，叠在主内容之上（zIndex 4，allowsHitTesting=false）
// - `.alert(title, message)` → AlertOverlayView 居中弹出，遮罩全屏阻塞（zIndex 3）
// - `.retry(message)` → RetryView 全屏覆盖（zIndex 2）
//
// **Modal blocking 语义**（alert / retry 都是 modal）：
// - 下层 content 应用 `.accessibilityHidden(true)`：VoiceOver 不能 focus 后面控件
// - 下层 content 应用 `.allowsHitTesting(false)`：tap / touch 不会穿透到后面
// - retry 额外 opacity=0（视觉全覆盖）；alert 视觉上不全覆盖（半透明遮罩在 AlertOverlayView 内自带），但访问性/交互必须屏蔽
// 单纯靠 overlay zIndex 不足以实现 modal blocking——VoiceOver / Hit-testing 默认仍能穿透。
//
// 动画（hardcoded MVP）：
// - toast：`.transition(.move(edge: .top).combined(with: .opacity))`
// - alert / retry：`.transition(.opacity)`
//
// 用 ZStack + 显式 zIndex 而非 overlay 链：层级一目了然（详见 Dev Note #3）。
// Toast 加 .allowsHitTesting(false)：让 toast 浮在内容之上但**不吃**点击——避免 lesson
// 2026-04-25-swiftui-zstack-overlay-bottom-cta.md 提到的"装饰元素吃了交互按钮的点击"问题。
//
// import 备注：`@ObservedObject` 来自 SwiftUI（再 export Combine），但为遵守 lesson
// 2026-04-25-swift-explicit-import-combine.md 的"显式 import"原则，仍 import Combine。

import SwiftUI
import Combine

public struct ErrorPresentationHostModifier: ViewModifier {
    @ObservedObject var presenter: ErrorPresenter

    public func body(content: Content) -> some View {
        ZStack {
            // 主内容；alert / retry 出现时同步屏蔽访问性 + hit-testing（modal blocking 语义）
            // retry 额外 opacity=0（视觉全覆盖）；alert 视觉上保留下层（AlertOverlayView 自带半透明遮罩）
            content
                .accessibilityHidden(modalActive)
                .allowsHitTesting(!modalActive)
                .opacity(retryActive ? 0 : 1)

            // Retry 全屏覆盖
            if case let .retry(message) = presenter.current {
                RetryView(message: message, onRetry: { presenter.dismiss(triggerOnRetry: true) })
                    .transition(.opacity)
                    .zIndex(2)
            }

            // Alert 居中弹层
            if case let .alert(title, message) = presenter.current {
                AlertOverlayView(title: title, message: message, onDismiss: { presenter.dismiss() })
                    .transition(.opacity)
                    .zIndex(3)
            }

            // Toast 顶部浮现
            if case let .toast(message) = presenter.current {
                VStack {
                    ToastView(message: message)
                        .padding(.top, 12)
                        .transition(.move(edge: .top).combined(with: .opacity))
                    Spacer()
                }
                .frame(maxWidth: .infinity)
                .allowsHitTesting(false)   // toast 不吃点击：让用户继续操作下方内容
                .zIndex(4)
            }
        }
        .animation(.easeInOut(duration: 0.2), value: presenter.current)
    }

    /// retry 状态（视觉上需要把下层 opacity 置 0）
    var retryActive: Bool {
        if case .retry = presenter.current { return true }
        return false
    }

    /// modal 状态（alert / retry 都需要屏蔽下层访问性 + hit-testing）。
    /// toast 不是 modal，下层 content 保持可交互；nil 同理（无错误展示）。
    var modalActive: Bool {
        if case .alert = presenter.current { return true }
        if case .retry = presenter.current { return true }
        return false
    }
}

extension View {
    /// 把 ErrorPresenter 挂到 view 上：根据 presenter.current 自动渲染对应错误组件。
    /// 应在 RootView body 最外层调用一次（**不**在每个子页重复挂）。
    public func errorPresentationHost(presenter: ErrorPresenter) -> some View {
        modifier(ErrorPresentationHostModifier(presenter: presenter))
    }
}
