// HomeViewModel.swift
// Story 2.2 占位 ViewModel：
// - 暴露 nickname / appVersion / serverInfo 三个 @Published（hardcode）
// - 暴露 onRoomTap / onInventoryTap / onComposeTap 三个 closure（init 默认空函数）
//
// Story 2.3 把三个 closure 替换为 coordinator.present(...) 路由跳转（已落地）。
//
// Story 2.5 扩展（**追加，不删除老接口**）：
// - 新增 init(pingUseCase:) 重载，按需注入 PingUseCaseProtocol
// - 新增 bind(pingUseCase:) 单次绑定（RootView .task 路径用，规避 @StateObject init 注入限制）
// - 新增 start() async 触发 ping，重复调用通过 pingTask + hasFetched 双层短路
// - 新增 applyPingResult(_:) 三态文案投影（offline / v? / commit）
// - 新增 nonisolated static func readAppVersion() 从 Bundle 读 CFBundleShortVersionString
//
// Story 2.5 review fix round 2（codex P2）：
// - start() 加 `hasFetched` flag，跨"任务完成 → view 重新出现"边界 short-circuit。
//   原因：SwiftUI `.task` 在 view 重新出现时会重启；只用 pingTask 短路无法防 sheet 关闭后重跑。
//   详见 docs/lessons/2026-04-26-swiftui-task-modifier-reentrancy.md。
//
// 设计选择（参照 Story 2.5 Dev Note #2 / #3 / #5）：
// - appVersion 仅存数字部分（如 "1.0.0" 或 "0.0.0"），View 层拼接 "v\(appVersion) · \(serverInfo)"
// - "offline" / "v?" 字符串归 ViewModel（业务投影），"v" 前缀 / "·" 分隔符归 HomeView 模板（UI）
// - 老 init（无 pingUseCase 参数）保留：Preview / Story 2.2/2.3 老测试零改动

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

    /// 注入的 PingUseCase（init 路径）；新 init 设非 nil，老 init 设 nil。
    private let pingUseCase: PingUseCaseProtocol?

    /// 通过 `bind(pingUseCase:)` 注入的 PingUseCase（运行时注入路径，RootView .task 用）。
    /// 与 `pingUseCase` 互斥使用：start() 优先用 init 路径，未注入时回退到 bind 路径。
    private var boundPingUseCase: PingUseCaseProtocol?

    /// 当前是否有进行中的 ping 任务。再次调用 start() 时短路（防 SwiftUI .task 多次触发引发重复请求）。
    private var pingTask: Task<Void, Never>?

    /// 是否已经成功跑完一次 ping（含失败也置 true）。用于跨"task 完成 → view 重新出现"边界 short-circuit。
    ///
    /// 背景（review fix round 2）：SwiftUI 的 `.task` modifier 在 view 重新出现时（如 `.fullScreenCover`
    /// 关闭后回到 RootView）会重启 task。此时 `pingTask` 已经完成被置 nil，原有的"并发短路"防御不生效，
    /// 每次 sheet 关闭都会重发一次 ping —— 业务上 ping/version 是"App 启动一次性探针"，不应在普通导航时重跑。
    /// 加 `hasFetched` flag 让 start() 在已完成一次（无论成功/失败）后直接 return。
    /// 失败也置 true 的理由：避免 server 不可达时反复重试；错误恢复 UI 是 Story 2.6 的责任。
    /// 详见 docs/lessons/2026-04-26-swiftui-task-modifier-reentrancy.md。
    private var hasFetched: Bool = false

    /// 老 init（Story 2.2 / 2.3 路径）：保留 hardcode 默认值，pingUseCase = nil；不破坏老调用方 / Preview。
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
        self.pingUseCase = nil
    }

    /// Story 2.5 新增 init：注入 PingUseCaseProtocol；appVersion 默认从 Bundle 读取。
    /// 在 AppContainer wire 时调用此 init；测试也用此 init 注入 mock UseCase。
    public init(
        nickname: String = "用户1001",
        pingUseCase: PingUseCaseProtocol,
        appVersion: String = HomeViewModel.readAppVersion(),
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
        self.pingUseCase = pingUseCase
    }

    /// 单次绑定 PingUseCase。多次调用时仅第一次生效（后续静默 noop）。
    /// 与 init(pingUseCase:) 互斥使用：测试场景用 init 注入，production 路径（RootView.task）用 bind 注入。
    ///
    /// 防重入理由：SwiftUI `.task` 在 view 重新出现时可能多次触发；防止第二次调用覆盖第一次的注入实例。
    ///
    /// 副作用：第一次注入时也同步更新 `appVersion` = `readAppVersion()`。原因：RootView 走 `HomeViewModel()`
    /// 老 init（@StateObject 不能在 init 阶段读 Bundle，避免初始化期注入陷阱），appVersion 会停在 "0.0.0"
    /// 默认值；只有 bind() 时才真正能拿到 PingUseCase 与 production 上下文，此时同步刷一遍 bundle version
    /// 才能让 production 显示 v1.0.0 而不是 v0.0.0。详见 docs/lessons/2026-04-26-stateobject-init-vs-bind-injection.md。
    public func bind(pingUseCase: PingUseCaseProtocol) {
        guard self.boundPingUseCase == nil else { return }
        self.boundPingUseCase = pingUseCase
        self.appVersion = HomeViewModel.readAppVersion()
    }

    /// 触发 ping。三层短路：
    ///   1. 已成功跑完一次（hasFetched=true）→ 直接 return（防 SwiftUI .task 在 view 重新出现时重跑）。
    ///   2. 进行中的任务（pingTask 非 nil）→ 直接 return（防并发触发同时调两次）。
    ///   3. 未注入 UseCase → no-op。
    /// RootView 在 `.task { await viewModel.start() }` 中调用，App 启动时执行一次（成功/失败都只一次）。
    public func start() async {
        // 取注入实例（init 路径优先，回退 bind 路径）。
        let useCase = pingUseCase ?? boundPingUseCase
        guard let useCase = useCase else { return }   // 未注入：no-op
        guard !hasFetched else { return }              // 已完成一轮：短路（跨 task 边界）
        guard pingTask == nil else { return }          // 已有进行中任务：短路（同一 task 边界内并发）

        let task = Task { [weak self] in
            let result = await useCase.execute()
            await self?.applyPingResult(result)
        }
        pingTask = task
        await task.value
        pingTask = nil
        hasFetched = true   // 失败也置 true：避免不可达 server 时反复重试；错误恢复 UI 见 Story 2.6
    }

    /// 把 PingResult 投影成 serverInfo 文案。
    /// 三态：
    ///   - reachable=false → "offline"
    ///   - reachable=true + commit 非空 → commit 短哈希
    ///   - reachable=true + commit nil/空 → "v?"（部分降级）
    private func applyPingResult(_ result: PingResult) {
        if !result.reachable {
            self.serverInfo = "offline"
        } else if let commit = result.serverCommit, !commit.isEmpty {
            self.serverInfo = commit
        } else {
            self.serverInfo = "v?"
        }
    }

    /// 从 main bundle 读 `CFBundleShortVersionString`；缺省 `"0.0.0"`（与现有 hardcode 默认一致）。
    /// nonisolated 让它能在 `MainActor` init 默认参数位置安全调用（默认参数表达式在 actor isolation 之外评估）。
    public nonisolated static func readAppVersion() -> String {
        (Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String) ?? "0.0.0"
    }
}
