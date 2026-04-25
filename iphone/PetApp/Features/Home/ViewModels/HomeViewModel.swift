// HomeViewModel.swift
// Story 2.2 占位 ViewModel：
// - 暴露 nickname / appVersion / serverInfo 三个 @Published（hardcode）
// - 暴露 onRoomTap / onInventoryTap / onComposeTap 三个 closure（init 默认空函数）
//
// Story 2.5 会把 hardcode 替换为 PingUseCase / FetchVersionUseCase 驱动；
// Story 2.3 会把三个 closure 替换为 coordinator.present(...) 路由跳转。
//
// 设计选择（参照 story Dev Note #2）：appVersion 仅存数字部分（如 "0.0.0"），
// View 层拼接 "v\(appVersion) · \(serverInfo)" → 输出 "v0.0.0 · ----"，避免双 v 前缀。

import Foundation
import Combine

@MainActor
public final class HomeViewModel: ObservableObject {
    @Published public var nickname: String
    @Published public var appVersion: String
    @Published public var serverInfo: String

    public var onRoomTap: () -> Void
    public var onInventoryTap: () -> Void
    public var onComposeTap: () -> Void

    public init(
        nickname: String = "用户1001",
        appVersion: String = "0.0.0",
        serverInfo: String = "----",
        onRoomTap: @escaping () -> Void = {},
        onInventoryTap: @escaping () -> Void = {},
        onComposeTap: @escaping () -> Void = {}
    ) {
        self.nickname = nickname
        self.appVersion = appVersion
        self.serverInfo = serverInfo
        self.onRoomTap = onRoomTap
        self.onInventoryTap = onInventoryTap
        self.onComposeTap = onComposeTap
    }
}
