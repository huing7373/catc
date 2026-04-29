// AppLaunchStateMachine.swift
// Story 2.9: App 启动状态机骨架。
//
// 职责：
//   - 持有当前 launch state（@Published，RootView 订阅渲染）
//   - bootstrap() 串行跑两个占位 step；任一抛错 → state = .needsAuth(presentation:)
//     （Story 5.5 round 2 [P1] fix：原 .needsAuth(message:) 升级为携带 ErrorPresentation,
//      让 RootView 三态分发 alert/retry/toast 而不是一律渲染 RetryView）
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

    /// 失败默认 presentation（fallback 路径）：unmapped error 时用 .retry —— 优先给用户重试入口
    /// （比卡 alert 更宽容；已知非可重试错误必须由 caller 显式携带 .alert presentation 传入）.
    /// Story 5.5 round 2 [P1] fix.
    public static let defaultFailurePresentation: ErrorPresentation = .retry(message: defaultFailureMessage)

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
    /// 任一抛错 → state = .needsAuth(presentation:)，presentation 由 BootstrapMappedError 携带
    /// 或 LocalizedError fallback 包成 .retry（默认 fallback `defaultFailurePresentation`）；
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
            // 失败路径**不**等 minimumDuration：进入 .needsAuth 越快越好（用户能立即看到错误 UI）.
            // presentation 由 BootstrapMappedError 显式携带（caller 在 closure catch block 内调
            // AppErrorMapper.presentation(for:) 派出对应样式）；非 BootstrapMappedError 走
            // defaultFailurePresentation = .retry —— Story 5.5 round 2 [P1] fix.
            state = .needsAuth(presentation: presentationFor(error: error))
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

    /// ADR-0008 v2 §6.3 / Story 0008-impl-1: 由 AuthBoundaryAPIClient 在 401 时调用 ——
    /// 触发全局 cold-start 重建（与 retry() 几乎相同实现，区分语义）：
    ///   - retry(): user 在 .needsAuth 状态下点 RetryView 重试按钮（user-initiated）
    ///   - triggerColdStart(): network 层检测到 token 失效自动触发（system-initiated）
    ///
    /// **并发短路**：复用 isRetrying flag —— 多个并发业务请求同时拿到 401 时，
    /// 只有第一个触发的会真正重跑 bootstrap，后续都被 short-circuit（与 retry() 同模式）。
    /// 这正是 silent relogin 退役后**不需要**generation snapshot / inFlight 三件套的原因 ——
    /// state machine 自身的 reentrancy guard 就够了。
    ///
    /// **不**与 retry() 合并为 private helper —— 两者语义不同（user-initiated vs system-initiated），
    /// 强行合并属于 ADR §13.3 "RootView/AppLaunchStateMachine 知道太多" 重构范畴，留给后续 epic-cleanup.
    public func triggerColdStart() async {
        guard !isRetrying else { return }
        isRetrying = true
        defer { isRetrying = false }

        state = .launching
        hasBootstrapped = false
        await bootstrap()
    }

    /// 把任意 Error 转成 `.needsAuth` 的 `ErrorPresentation`.
    ///
    /// **优先级**（Story 5.5 round 2 [P1] fix）:
    /// 1. `BootstrapMappedError`: caller 已经在 closure catch block 内调过
    ///    `AppErrorMapper.presentation(for:)` 决定好样式 —— 直接用其 `presentation` 字段.
    ///    这是 production 路径：让 mapper 单一决定 alert vs retry vs toast, 状态机不重做判断.
    /// 2. 其他 LocalizedError: 用其 errorDescription 包成 `.retry(message:)` —— 兼容
    ///    legacy 测试 case + Story 2.9 default closure 行为. `.retry` 是宽容兜底（用户至少
    ///    能点重试触发 cold-start）.
    /// 3. plain Error: 走 `defaultFailurePresentation` = `.retry(message: defaultFailureMessage)`.
    ///    防 `error.localizedDescription` 漏 NSError 系统串到 UI（lesson
    ///    2026-04-26-error-localizeddescription-system-fallback.md, codex round 1 [P2] finding）.
    ///
    /// **不**直接调 `AppErrorMapper.presentation(for:)` 兜底：production 已用 BootstrapMappedError
    /// 路径走 mapper, 落到本函数 fallback 的应是 default closure / 测试 case / Epic 5 之外的
    /// plain Error. 本函数自己保留 LocalizedError-aware fallback (优先用 errorDescription)
    /// 比直接走 mapper fallback 更友好 —— 哪怕 round 10 [P2] fix 后 mapper fallback 已经统一
    /// 用 .retry, 本函数仍优先 LocalizedError 路径以保留 dev 上 errorDescription 的可读性.
    private func presentationFor(error: Error) -> ErrorPresentation {
        if let mapped = error as? BootstrapMappedError {
            return mapped.presentation
        }
        if let localized = error as? LocalizedError, let desc = localized.errorDescription, !desc.isEmpty {
            return .retry(message: desc)
        }
        return AppLaunchStateMachine.defaultFailurePresentation
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
