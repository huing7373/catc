// SessionStore.swift
// Story 5.2 AC4: in-memory observable session state holder.
//
// 设计：
// - @MainActor + ObservableObject + @Published session: SessionState? —— SwiftUI 可订阅
// - 初值 nil（未登录 / 启动中）
// - updateSession(_:) 写入；clear() 清空（Story 5.4 静默重登 / dev 重置身份用）
// - **不**直接读 / 写 Keychain（解耦：keychain 持久化层，SessionStore 内存表征）
//
// 命名 SessionStore（不是 SessionManager）：
// - iOS 架构 §5.4 列示的 "SessionRepository" 偏 fetch/persist 语义
// - 本类仅 in-memory observable state holder，更接近 "Store" 命名
// - 与 ErrorPresenter / AppCoordinator 同模式（container 持有的 stable singleton）
//
// import 备注（继承 lesson 2026-04-25-swift-explicit-import-combine.md）：
// ObservableObject / @Published 来自 Combine，必须显式 import Combine.

import Foundation
import Combine

@MainActor
public final class SessionStore: ObservableObject {
    /// 当前会话；nil 表示未登录 / 启动中。SwiftUI view 通过 @ObservedObject / @EnvironmentObject 订阅。
    @Published public private(set) var session: SessionState?

    public init() {}

    /// 写入新会话（GuestLoginUseCase / SilentReloginUseCase 成功后调）。
    /// `@MainActor` 保证调用方必须从 main thread 调（编译器强制）。
    public func updateSession(_ state: SessionState) {
        self.session = state
    }

    /// 清空会话（dev 重置身份按钮 / Story 5.4 静默重登失败 兜底）。
    /// **不**触发 keychain 删除 —— 那是 ResetKeychainUseCase / 5.4 SilentRelogin 的责任；
    /// 本方法仅清内存表征，调用方负责协调 keychain.
    public func clear() {
        self.session = nil
    }
}
