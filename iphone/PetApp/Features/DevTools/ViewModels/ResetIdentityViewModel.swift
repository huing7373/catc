// ResetIdentityViewModel.swift
// Story 2.8: dev "重置身份" 按钮的 ViewModel。
// Story 5.2 round 2 [P2] fix：tap() 成功路径同步清 SessionStore.session，避免 reset 后
// HomeView SessionAwareUserInfoBar 仍渲染旧 nickname/avatar 直到 kill app。
//
// Story 37.4 改造（AC6 / ADR-0010 §3.7 Reset 流程）：tap() 成功路径在 sessionStore?.clear() 之后
// 追加 appState?.reset()，让 AppState domain state（user/pet/stepAccount/chest/currentRoomId）
// 同步清空；HomeView / HomeContainerView 通过 @EnvironmentObject 订阅 → reset 后立即退回空态.
//
// 设计：@MainActor + ObservableObject + @Published alertContent；按钮点击触发
// useCase.execute()，结果（成功 / 失败）转写为 alertContent 让 SwiftUI .alert(item:) 弹出。
//
// 与 GuestLoginUseCase 对称的协调模式：
// - GuestLoginUseCase 只负责 keychain.set + repository.login，**不**调 SessionStore.updateSession
//   —— 由 RootView bootstrapStep1 closure 协调（UseCase 单一职责）。
// - 对称地，ResetKeychainUseCase 只负责 keychain.removeAll，**不**调 SessionStore.clear
//   —— 由本 ViewModel 协调（同样保持 UseCase 单一职责，不让其跨 actor 持有 @MainActor 类型）。
// - sessionStore 注入为 Optional：兼容老测试 init(useCase:) 签名 + Release build 也安全。
//
// import Combine 必显式（lesson 2026-04-25-swift-explicit-import-combine.md）：
// ObservableObject / @Published 来自 Combine，不能依赖 SwiftUI transitive import。

import Combine
import Foundation

/// alert 内容枚举：成功 / 失败两态；nil = 不显示 alert。
public enum ResetIdentityAlertContent: Equatable {
    case success
    case failure(message: String)
}

@MainActor
public final class ResetIdentityViewModel: ObservableObject {
    @Published public var alertContent: ResetIdentityAlertContent?

    private let useCase: ResetKeychainUseCaseProtocol
    /// Story 5.2 round 2 [P2] fix：tap() 成功后调 sessionStore.clear()，
    /// 让 HomeView SessionAwareUserInfoBar 立刻退回 fallback nickname。
    /// Optional：保留 Story 2.8 老测试 init(useCase:) 兼容（在 SessionStore 还不存在的语境下）。
    private let sessionStore: SessionStore?

    /// Story 37.4 AC6：tap() 成功后调 appState.reset()，让 domain state 同步清空.
    /// Optional：保留老测试 init(useCase:) / init(useCase:sessionStore:) 兼容.
    private let appState: AppState?

    public init(
        useCase: ResetKeychainUseCaseProtocol,
        sessionStore: SessionStore? = nil,
        appState: AppState? = nil
    ) {
        self.useCase = useCase
        self.sessionStore = sessionStore
        self.appState = appState
    }

    /// 用户点击按钮时调用：触发 useCase；任一结果设 alertContent 非 nil 触发 SwiftUI alert 弹出。
    /// 成功文案："已重置，请杀进程后重新启动 App 模拟首次安装"
    /// 失败文案："重置失败：<error description>"
    ///
    /// 成功路径**额外**调 `sessionStore?.clear()` + `appState?.reset()` —— 顺序：先 keychain 清完
    /// 再清 in-memory session 再清 domain state（fail-open 原则：keychain 抛错时 session/state
    /// 不会被错误地置 nil/empty）.
    /// Story 37.4 AC6 / ADR-0010 §3.7：appState.reset() 与 sessionStore.clear() 双调,
    /// 边界各自独立（SessionStore 持认证态 / AppState 持 domain state）.
    public func tap() async {
        do {
            try await useCase.execute()
            sessionStore?.clear()
            appState?.reset()
            alertContent = .success
        } catch {
            alertContent = .failure(message: "重置失败：\(error.localizedDescription)")
        }
    }

    /// 由 SwiftUI alert 的 dismiss 回调调用：清空 alertContent，避免下次 tap 时旧 alert 残留。
    /// SwiftUI `.alert(item:)` 在 user 点 OK 后理论自动复位 binding 为 nil；显式 reset 是兜底
    /// （与 Story 2.6 ErrorPresenter.dismiss() 同思路）。
    public func alertDismissed() {
        alertContent = nil
    }
}
