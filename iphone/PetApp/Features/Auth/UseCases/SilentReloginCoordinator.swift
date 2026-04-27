// SilentReloginCoordinator.swift
// Story 5.4 AC4: 静默重登协调器 —— "同一时刻只跑一次重登 + 多并发等待复用结果".
//
// 设计选择（actor）：
//   - 用 actor 而非 class + NSLock：actor 的 isolation 自动保证 inFlight / generation 字段
//     read-modify-write 原子
//   - actor method 内的 `await` 让出 actor —— 第二个进入的 relogin() 调用看到非 nil inFlight 就 await
//     既存 task 的 .value（不重新建 task / 不重复调 useCase.execute）
//
// Coalesce 的两条独立路径（必须同时存在 —— 缺一就漏 case）：
//   (a) **inFlight 路径**：两个 caller 在 spawned task 还活着时进入 relogin → 第二个 caller 看到
//       inFlight 非 nil → await 同一 task → useCase.execute 仅 1 次.
//       覆盖：5 个并发请求都几乎同时收到 401 的常见场景.
//   (b) **generation 路径**：caller B 的 401 是基于 pre-refresh stale token,但 B 进入 relogin() 的
//       时机晚于 caller A 完成（A 已经 clearInFlight + 写新 token） → inFlight 重新为 nil,但
//       coordinator 内 generation 已 ++ → B 在调 relogin() **之前** snapshot 的 generation
//       小于当前 generation → 直接返回 lastIssuedToken,不再启第二次 useCase.
//       覆盖：fix-review round 3 [P2] codex finding —— "stale concurrent 401 不应触发第二次 relogin".
//
// 关键约束：
//   - inFlight 是 Task<String, Error>?：Task 已 cancelable / can-await-value，是 Swift 标准 future
//   - inFlight 清理绑定在 spawned Task 自身生命周期（非 caller defer）—— 这样 caller 被 cancel 时,
//     inFlight 仍然指向正在跑的 task,后续并发 relogin 会 coalesce 到同一 task,不会重复发 guest-login.
//     清理由 spawned Task 自己完成（hop 回 actor 调 finishInFlight(token:)）.
//   - 失败的 task 被多并发等待者拿到的是同一个 throw —— 协调器 caller 一致看到同一 error，避免
//     "5 个请求 5 个不同 error" 的诡异行为
//   - generation 仅在**成功**完成时 ++ —— 失败不算"已经帮你 refresh 过",B 不能拿到 lastIssuedToken
//     而跳过自己的重登
//
// 不职责：
//   - 不做 retry（让 caller 决定是否重试 —— 如 AuthRetryingAPIClient 内的 catch 即"重试 1 次"逻辑）
//   - 不做指数退避 / circuit breaker（MVP 不做；server /auth/guest-login 已有 rate_limit 中间件）
//   - 不持有任何 long-lived token 状态做业务用 —— lastIssuedToken 只是 dedup 缓存,业务读 token
//     仍走 keychain（保持单一来源）

import Foundation

public actor SilentReloginCoordinator {
    private let useCase: SilentReloginUseCaseProtocol
    private var inFlight: Task<String, Error>?

    /// 单调递增的"成功完成 generation"。每次 spawned task 成功完成 → +1.
    /// caller 在调 relogin(callerGeneration:) **之前** snapshot 当前值；
    /// 进入 relogin 时若 generation 已超过 snapshot → 表示已有别的 caller 完成了一次成功 refresh,
    /// 当前 caller 应直接复用 lastIssuedToken 不再启新 useCase.execute.
    private var generation: UInt64 = 0

    /// 上一次成功 refresh 拿到的 token —— 仅供 generation 路径快速返回.
    /// 业务层读 token 仍走 keychain（保持单一来源）.
    private var lastIssuedToken: String?

    public init(useCase: SilentReloginUseCaseProtocol) {
        self.useCase = useCase
    }

    /// 给 caller 在调 relogin 前 snapshot 当前 generation —— 用于 stale-401 dedup 路径（见文件头注释 (b)）.
    public func currentGeneration() -> UInt64 {
        return generation
    }

    /// 触发一次静默重登；多并发调用 coalesce 到同一次执行（见文件头注释 (a) (b) 两条 coalesce 路径）.
    /// - Parameter callerGeneration: caller 在调本方法**之前** 通过 currentGeneration() 拿到的 snapshot.
    ///                               若进入本方法时 generation > callerGeneration,说明在 caller "决定要 relogin"
    ///                               与"实际进入 relogin"之间已有另一 caller 完成成功 refresh —— 直接返回
    ///                               lastIssuedToken,不启动第二次 useCase.
    /// - Returns: 新 token
    /// - Throws: useCase.execute 抛的任何错误（KeychainError / APIError）
    public func relogin(callerGeneration: UInt64) async throws -> String {
        // (b) generation 路径：A 已成功完成,B 进入时 generation 已 ++ → 直接复用 A 的结果,不启新 useCase.
        // 这是 fix-review round 3 [P2] codex finding 的核心修复.
        if generation > callerGeneration, let cached = lastIssuedToken {
            return cached
        }

        // (a) inFlight 路径：A 仍在跑,B 进入时 inFlight 非 nil → await 同一 task.
        // actor 的 isolation 保证这个 if-let 与下面 inFlight = task 是原子（无 race）.
        if let existing = inFlight {
            return try await existing.value
        }

        // 没在飞 + generation 也没超过 → 启动新 task.
        // Task { ... } 让 useCase.execute 在 cooperative thread pool 上跑（不阻塞 actor）.
        // 关键：清理动作绑定 spawned Task 的生命周期（不是 caller 的生命周期）.
        //   - caller 被 cancel 时,`await task.value` 抛 CancellationError,但 spawned task
        //     不被 cancel（unstructured Task 不继承 caller 的 cancellation）.inFlight 仍指向它,
        //     后续并发 relogin 仍能 coalesce 到同一 task,不会重复发 guest-login.
        //   - useCase.execute 真正完成（成功 / 失败）后,task body 末尾 hop 回 actor 调
        //     finishInFlight,清空 inFlight 同时（仅成功时） ++ generation 写入 lastIssuedToken.
        // 用 strong self 而非 weak —— 协调器整个生命周期由 AppContainer 持有,正在跑的重登
        // task 不应该因为弱引用被提前“悬空”；spawned task 完成后 self 自然不被 task 持有了.
        let useCase = self.useCase
        let task = Task { () async throws -> String in
            do {
                let token = try await useCase.execute()
                await self.finishInFlight(success: token)
                return token
            } catch {
                await self.finishInFlight(failure: ())
                throw error
            }
        }
        inFlight = task

        // await 自己启动的 task；其它并发 relogin 调用看到的 inFlight 也指向同一 task → 同一结果.
        // 即使本 caller 被 cancel,spawned task 不被 cancel,inFlight 还活着 → coalesce 不破.
        return try await task.value
    }

    /// 由 spawned Task 在自己**成功**完成时调用：清空 inFlight + 推进 generation + 缓存新 token.
    /// actor isolation 保证这次写不会和并发 relogin() 的 read 撞 race.
    private func finishInFlight(success token: String) {
        inFlight = nil
        generation &+= 1  // wrapping add；UInt64 实际运行年限够长,但用 &+= 防御性避免溢出 trap
        lastIssuedToken = token
    }

    /// 由 spawned Task 在自己**失败**完成时调用：清空 inFlight,但**不**推进 generation 也**不**写
    /// lastIssuedToken —— 失败不算"已经帮你 refresh 过",后续 caller 仍需自己尝试.
    private func finishInFlight(failure: Void) {
        inFlight = nil
    }
}
