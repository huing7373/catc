// AppLaunchStateMachine.swift
// Story 2.9: App 启动状态机骨架。
//
// 职责：
//   - 持有当前 launch state（@Published，RootView 订阅渲染）
//   - bootstrap() 串行跑两个占位 step；任一抛错 → state = .needsAuth(message:)
//   - 全部 step 成功且经历至少 0.3 秒后 → state = .ready
//   - retry() 重置 state = .launching 并重跑 bootstrap()
//
// 不职责：
//   - 不调真实 GuestLoginUseCase / LoadHomeUseCase / KeychainStore（Epic 5 / Story 5.2 / 5.5 落地）
//   - 不调 APIClient（占位 closure 默认 no-op）
//   - 不持久化 state
//
// Epic 5 接入说明：当 Story 5.2 / 5.5 落地时，RootView 通过初始化器把真实 closure 注入：
//   AppLaunchStateMachine(
//     bootstrapStep1: { try await guestLoginUseCase.execute() },
//     bootstrapStep2: { try await loadHomeUseCase.execute() }
//   )
// 本 story 默认 closure 是 `{ }`（立即成功 no-op），**让 LaunchingView 骨架可独立验证 + 集成测试可控**。
//
// import 备注（继承 lesson 2026-04-25-swift-explicit-import-combine.md）：
// `ObservableObject` / `@Published` 来自 Combine，必须显式 `import Combine`。

import Foundation
import Combine

@MainActor
public final class AppLaunchStateMachine: ObservableObject {

    /// 当前 launch state；初值 `.launching`。RootView 订阅此字段做路由。
    @Published public private(set) var state: AppLaunchState = .launching

    /// LaunchingView 至少显示时长（epics.md AC 钦定 0.3 秒）。
    /// 防止极快 bootstrap（占位 no-op 几乎瞬时完成）让 LaunchingView 闪一下就消失，造成视觉跳动。
    public static let minimumDuration: TimeInterval = 0.3

    /// 失败默认文案（不携带具体错误时使用）。
    public static let defaultFailureMessage = "登录失败，请重试"

    /// Step 1：epics.md 内对应 GuestLoginUseCase（Epic 5 Story 5.2 接入）。
    /// 默认 `{ }`（立即成功），Epic 5 落地时由 RootView 注入真实闭包。
    private let bootstrapStep1: () async throws -> Void

    /// Step 2：epics.md 内对应 LoadHomeUseCase（Epic 5 Story 5.5 接入）。
    /// 默认 `{ }`（立即成功），Epic 5 落地时由 RootView 注入真实闭包。
    private let bootstrapStep2: () async throws -> Void

    /// `bootstrap()` 是否已被调过一次（含成功 / 失败）。防 .task 重启时重复跑 step
    /// （lesson 2026-04-26-swiftui-task-modifier-reentrancy.md：SwiftUI .task 在 view 重新出现时会重启）。
    /// **失败也置 true**：避免 server 不可达时反复重试；用户重试走 retry() 显式入口。
    private var hasBootstrapped: Bool = false

    /// `retry()` 是否正在跑中。用户连点重试按钮时第二次调用应直接丢弃，
    /// 否则会清掉 hasBootstrapped 让在飞中的第一次 bootstrap closure 与第二次并发跑，
    /// 重复发起请求 + race 最终 state 写入（codex round 1 [P2] finding）。
    /// 详见 docs/lessons/2026-04-26-user-triggered-action-reentrancy.md。
    private var isRetrying: Bool = false

    /// 注入式 init：让测试 / Epic 5 真实落地都能传自己的 step closure。
    /// 默认参数 `{ }` 让本 story 测试 + LaunchingView 骨架验证可独立 work，无需 wire 真实 UseCase。
    public init(
        bootstrapStep1: @escaping () async throws -> Void = { },
        bootstrapStep2: @escaping () async throws -> Void = { }
    ) {
        self.bootstrapStep1 = bootstrapStep1
        self.bootstrapStep2 = bootstrapStep2
    }

    /// App 启动时由 RootView `.task` 调一次。串行跑两个 step；
    /// 任一抛错 → state = .needsAuth(message:)，message 取错误描述（默认 fallback "登录失败，请重试"）；
    /// 全成功 → 等"经过至少 0.3 秒"后 state = .ready。
    ///
    /// 防重入：跨 .task 边界用 hasBootstrapped flag 短路（与 HomeViewModel.start() 同模式）。
    public func bootstrap() async {
        guard !hasBootstrapped else { return }
        hasBootstrapped = true

        let startTime = Date()

        do {
            try await bootstrapStep1()
            try await bootstrapStep2()
            await ensureMinimumDuration(elapsedSince: startTime)
            state = .ready
        } catch {
            // 失败路径**不**等 minimumDuration：进入 .needsAuth 越快越好（用户能立即看到 RetryView）。
            state = .needsAuth(message: messageFor(error: error))
        }
    }

    /// 用户在 .needsAuth 状态下点 RetryView 重试按钮 → 调此方法。
    /// 重置 state = .launching + 重跑 bootstrap（清 hasBootstrapped flag 让 bootstrap 可再跑一次）。
    ///
    /// **并发短路**：用户连点两次 retry 时第二次直接丢弃 —— 避免第二次 reset hasBootstrapped
    /// 让在飞中的第一次 bootstrap 与第二次并发跑（codex round 1 [P2] finding）。
    /// 详见 docs/lessons/2026-04-26-user-triggered-action-reentrancy.md。
    public func retry() async {
        guard !isRetrying else { return }
        isRetrying = true
        defer { isRetrying = false }

        state = .launching
        hasBootstrapped = false
        await bootstrap()
    }

    /// 把任意 Error 转成 .needsAuth 的 message。
    ///
    /// **关键**：必须用 `as? LocalizedError` 检查 + 自定义 fallback，**不**依赖 `error.localizedDescription`。
    /// 因为 Swift `Error.localizedDescription` 对非 `LocalizedError` 类型返回 generic 系统串
    /// （`"The operation couldn't be completed (PetApp.Foo error 1.)"`），而非空串 ——
    /// 所以"raw.isEmpty 兜底" 永远不触发，用户看到的是实现细节而非设计文档钦定的"登录失败，请重试"
    /// （codex round 1 [P2] finding）。
    /// 详见 docs/lessons/2026-04-26-error-localizeddescription-system-fallback.md。
    ///
    /// Epic 5 接入真实 APIError 时可让 APIError 实现 LocalizedError → errorDescription 直接走第一分支。
    private func messageFor(error: Error) -> String {
        if let localized = error as? LocalizedError, let desc = localized.errorDescription, !desc.isEmpty {
            return desc
        }
        return AppLaunchStateMachine.defaultFailureMessage
    }

    /// 等待"自 startTime 起至少 minimumDuration 秒"已经流逝。已经超过则立即 return。
    /// 实现关键：取实际 elapsed 与 minimumDuration 的差值 → 仅 sleep 缺口部分。
    /// **不**用 hardcode `Task.sleep(nanoseconds: 0.3 * 1e9)`（那样会把 0.3s 加在每次启动上，
    /// 即使真实工作要 5 秒）—— 这种 max(0, gap) 模式让 LaunchingView 在快网络下保 0.3 秒、
    /// 慢网络下立即过渡，不强加额外延迟。
    private func ensureMinimumDuration(elapsedSince startTime: Date) async {
        let elapsed = Date().timeIntervalSince(startTime)
        let remaining = AppLaunchStateMachine.minimumDuration - elapsed
        guard remaining > 0 else { return }
        try? await Task.sleep(nanoseconds: UInt64(remaining * 1_000_000_000))
    }
}
