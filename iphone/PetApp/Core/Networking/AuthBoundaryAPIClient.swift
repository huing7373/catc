// AuthBoundaryAPIClient.swift
// ADR-0008 v2 §6 / Story 0008-impl-1: APIClient decorator —— 拦 APIError.unauthorized
//   + endpoint.requiresAuth 触发**全局 cold-start**（清 SessionStore + state machine 回 .launching → 重跑 bootstrap）.
//
// 关系到 silent relogin 退役（ADR-0008 v2 D3）：
//   - 替代退役的 AuthRetryingAPIClient + SilentReloginCoordinator + SilentReloginUseCase 三件套
//   - 不复刻 generation snapshot / inFlight task / lastIssuedToken cache 三件套并发原语
//   - 多请求并发触发 cold-start 由 AppLaunchStateMachine.triggerColdStart() 内部 isRetrying flag 保护
//
// 拦截契约：
//   1. inner.request(endpoint) success → return（不触发 cold-start）
//   2. inner.request(endpoint) throw .unauthorized + endpoint.requiresAuth == true →
//      a. 触发 sink.trigger() —— RootView 注入的 handler 调 sessionStore.clear() + stateMachine.triggerColdStart()
//      b. throw APIError.unauthorized 让 caller 知道本次请求失败（ViewModel / Repository 层走自己的错误恢复）
//      c. **不**做 in-app retry（caller 看到的是普通失败；下次新请求由 cold-start 后的新 token 处理）
//   3. inner.request(endpoint) throw .unauthorized + endpoint.requiresAuth == false →
//      直接抛上去（如 /auth/guest-login 自己 401 → 不能用自己救自己）
//   4. inner.request(endpoint) throw .missingCredentials / .localStoreFailure / .network / .business / .decoding →
//      直接抛上去（不在 cold-start 职责内）
//
// 为何**不**拦 .missingCredentials / .localStoreFailure（与退役的 AuthRetryingAPIClient 同理由）：
//   .missingCredentials = 本地确认无 token（terminal）→ mapper 钦定 .alert force-quit
//   .localStoreFailure = keychain 抛错（transient）→ mapper 钦定 .retry 让 user 自助
//   两者都不是 server 拒绝当前 token，触发 cold-start 也救不回（cold-start 自己也走同一份 keychain）
//
// 与 AppLaunchStateMachine.triggerColdStart() 的责任分割：
//   - AuthBoundaryAPIClient: 网络层"我看到 401" 信号源
//   - UnauthorizedHandlerSink: late-bind handler 容器（解 chicken-and-egg：container init 时 stateMachine 还不存在）
//   - AppLaunchStateMachine.triggerColdStart(): UI 层"重跑 bootstrap" 动作（含 isRetrying flag 防并发触发）
//   - SessionStore.clear(): in-memory mirror 清除（让订阅 view 立即过渡到 fallback 态）
//
// 详见 _bmad-output/implementation-artifacts/decisions/0008-error-protocol.md §6
//   + _bmad-output/implementation-artifacts/0008-impl-1-砍-silent-relogin.md.

import Foundation

/// 401 cold-start handler 的 late-binding holder.
///
/// 解决 chicken-and-egg：
///   - AuthBoundaryAPIClient 在 AppContainer.init 时构造（需要 sink 引用）
///   - 但触发 cold-start 需要 AppLaunchStateMachine.triggerColdStart()
///   - AppLaunchStateMachine 在 RootView.ensureLaunchStateMachineWired() 时才 lazy 构造（晚于 container init）
///
/// Sink 让 container 先持有空 handler 的实例，RootView 创建 stateMachine 后再调 setHandler 注入。
///
/// **线程安全**：handler 字段用 NSLock 保护读写。AuthBoundaryAPIClient 是 Sendable，
/// trigger() 可能被任意 Task 并发调用；setHandler 由 RootView 在 main thread 设置。
public final class UnauthorizedHandlerSink: @unchecked Sendable {
    private let lock = NSLock()
    private var handler: (@Sendable () async -> Void)?

    public init() {}

    /// RootView 在创建 AppLaunchStateMachine 后调用此方法注入真实 handler.
    /// handler 闭包内一般做：`await sessionStore.clear(); await stateMachine.triggerColdStart()`.
    public func setHandler(_ handler: @escaping @Sendable () async -> Void) {
        lock.lock()
        defer { lock.unlock() }
        self.handler = handler
    }

    /// AuthBoundaryAPIClient 在检测到 401 + requiresAuth 时调用.
    /// handler 未注入时 no-op（RootView 还没 wire 完；理论上不该出现，因 cold-start 链路在 .ready 之后才触发业务请求）.
    public func trigger() async {
        let h: (@Sendable () async -> Void)?
        lock.lock()
        h = handler
        lock.unlock()
        await h?()
    }
}

/// AuthBoundaryAPIClient: 全局 401 cold-start 拦截器（替代退役的 AuthRetryingAPIClient）.
///
/// 设计选择（与 AuthRetryingAPIClient 对比）：
///   - **不**做 in-app retry：caller 收到 .unauthorized throw，由业务层错误处理流接管
///   - **不**调 SilentReloginUseCase（已退役）：直接走 cold-start GuestLogin + LoadHome 重跑
///   - **不**做 single-flight 协调：多请求并发由 AppLaunchStateMachine.isRetrying flag 去重
///   - decorator pattern 与 AuthRetryingAPIClient 一致，零 caller 改动（业务层拿到的仍是 APIClientProtocol）
///
/// 用户感知：从主屏闪一下 LaunchingView（< 1 秒）→ 回到主屏，无错误弹窗、无手动操作.
/// 用户数据不丢失（guestUid 在 keychain 持久化，cold-start 复用同一身份）.
public final class AuthBoundaryAPIClient: APIClientProtocol {
    private let inner: APIClientProtocol
    private let sink: UnauthorizedHandlerSink

    public init(inner: APIClientProtocol, sink: UnauthorizedHandlerSink) {
        self.inner = inner
        self.sink = sink
    }

    /// fix-review round 1 P2（Story 12.2 review）：decorator 透传 inner baseURL.
    /// 让 AppContainer 通过 wrappedAPIClient.baseURL 拿到与 REST 调用同源的 host-only URL,
    /// 派生 WebSocketClient 时不会退回 Bundle.main 默认值（split-brain 隐患）.
    public var baseURL: URL { inner.baseURL }

    public func request<T: Decodable>(_ endpoint: Endpoint) async throws -> T {
        do {
            return try await inner.request(endpoint)
        } catch APIError.unauthorized where endpoint.requiresAuth {
            // 仅 server 401 / envelope 1001 + requiresAuth=true 的请求触发 cold-start.
            // 本地态（.missingCredentials / .localStoreFailure）由默认 propagate 行为透传给 mapper.
            // requiresAuth=false（如 /auth/guest-login 自身）抛 .unauthorized 也透传（不能用自己救自己）.
            await sink.trigger()
            throw APIError.unauthorized
        }
        // catch APIError.unauthorized where !endpoint.requiresAuth: 不拦，let it propagate
        // catch APIError.missingCredentials / .localStoreFailure: 不拦，let it propagate
        // 其它 APIError（.network / .business / .decoding）: 不拦，let it propagate
    }
}
