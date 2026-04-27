// SilentReloginCoordinator.swift
// Story 5.4 AC4: 静默重登协调器 —— "同一时刻只跑一次重登 + 多并发等待复用结果".
//
// 设计选择（actor）：
//   - 用 actor 而非 class + NSLock：actor 的 isolation 自动保证 inFlight 字段 read-modify-write 原子
//   - actor method 内的 `await` 让出 actor —— 第二个进入的 relogin() 调用看到非 nil inFlight 就 await
//     既存 task 的 .value（不重新建 task / 不重复调 useCase.execute）
//
// 关键约束：
//   - inFlight 是 Task<String, Error>?：Task 已 cancelable / can-await-value，是 Swift 标准 future
//   - inFlight 清理绑定在 spawned Task 自身生命周期（非 caller defer）—— 这样 caller 被 cancel 时,
//     inFlight 仍然指向正在跑的 task,后续并发 relogin 会 coalesce 到同一 task,不会重复发 guest-login.
//     清理由 spawned Task 自己完成（hop 回 actor 调 clearInFlight(matching:)）.
//   - 失败的 task 被多并发等待者拿到的是同一个 throw —— 协调器 caller 一致看到同一 error，避免
//     "5 个请求 5 个不同 error" 的诡异行为
//
// 不职责：
//   - 不做 retry（让 caller 决定是否重试 —— 如 AuthRetryingAPIClient 内的 catch 即"重试 1 次"逻辑）
//   - 不做指数退避 / circuit breaker（MVP 不做；server /auth/guest-login 已有 rate_limit 中间件）
//   - 不持有任何 token / session 状态（数据在 useCase 内 + keychain）

import Foundation

public actor SilentReloginCoordinator {
    private let useCase: SilentReloginUseCaseProtocol
    private var inFlight: Task<String, Error>?

    public init(useCase: SilentReloginUseCaseProtocol) {
        self.useCase = useCase
    }

    /// 触发一次静默重登；多并发调用 coalesce 到同一次执行.
    /// - Returns: 新 token
    /// - Throws: useCase.execute 抛的任何错误（KeychainError / APIError）
    public func relogin() async throws -> String {
        // 已有 task 在飞 → 等它（不重启）.
        // actor 的 isolation 保证这个 if-let 与下面 inFlight = task 是原子（无 race）.
        if let existing = inFlight {
            return try await existing.value
        }

        // 没在飞 → 启动新 task.
        // Task { ... } 让 useCase.execute 在 cooperative thread pool 上跑（不阻塞 actor）.
        // 关键：清理动作绑定 spawned Task 的生命周期（不是 caller 的生命周期）.
        //   - caller 被 cancel 时,`await task.value` 抛 CancellationError,但 spawned task
        //     不被 cancel（unstructured Task 不继承 caller 的 cancellation）.inFlight 仍指向它,
        //     后续并发 relogin 仍能 coalesce 到同一 task,不会重复发 guest-login.
        //   - useCase.execute 真正完成（成功 / 失败）后,task body 末尾 hop 回 actor 调
        //     clearInFlight 清空 inFlight,下一轮 relogin 才能开新 task.
        // 用 strong self 而非 weak —— 协调器整个生命周期由 AppContainer 持有,正在跑的重登
        // task 不应该因为弱引用被提前“悬空”；spawned task 完成后 self 自然不被 task 持有了.
        let useCase = self.useCase
        let task = Task { () async throws -> String in
            do {
                let token = try await useCase.execute()
                await self.clearInFlight()
                return token
            } catch {
                await self.clearInFlight()
                throw error
            }
        }
        inFlight = task

        // await 自己启动的 task；其它并发 relogin 调用看到的 inFlight 也指向同一 task → 同一结果.
        // 即使本 caller 被 cancel,spawned task 不被 cancel,inFlight 还活着 → coalesce 不破.
        return try await task.value
    }

    /// 由 spawned Task 在自己完成（成功或失败）时调用,把 inFlight 字段清空.
    /// actor isolation 保证这次写不会和并发 relogin() 的 read 撞 race.
    private func clearInFlight() {
        inFlight = nil
    }
}
