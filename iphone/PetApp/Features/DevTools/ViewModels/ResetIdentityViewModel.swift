// ResetIdentityViewModel.swift
// Story 2.8: dev "重置身份" 按钮的 ViewModel。
//
// 设计：@MainActor + ObservableObject + @Published alertContent；按钮点击触发
// useCase.execute()，结果（成功 / 失败）转写为 alertContent 让 SwiftUI .alert(item:) 弹出。
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

    public init(useCase: ResetKeychainUseCaseProtocol) {
        self.useCase = useCase
    }

    /// 用户点击按钮时调用：触发 useCase；任一结果设 alertContent 非 nil 触发 SwiftUI alert 弹出。
    /// 成功文案："已重置，请杀进程后重新启动 App 模拟首次安装"
    /// 失败文案："重置失败：<error description>"
    public func tap() async {
        do {
            try await useCase.execute()
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
