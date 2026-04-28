// RootViewWireTests.swift
// Story 2.3 AC6：HomeViewModel ↔ AppCoordinator 闭包注入链路验证（≥3 case）。
//
// 由于 RootView 是 SwiftUI struct，@StateObject 不易在单测中直接构造（需要 view body 渲染才会触发 .onAppear），
// 本 story 选间接验证策略：在测试中模拟 RootView 的 wire 逻辑：
//   viewModel.onRoomTap = { [coordinator] in coordinator.present(.room) }
// 然后调用闭包，验证 coordinator.presentedSheet 被设到对应值。
//
// RootView 的视图渲染由 PetAppUITests/NavigationUITests 兜底（黑盒）。
//
// Story 5.5 round 3 [P1] fix 追加：bootstrap closure 在 retry 时**必须**重跑 guest-login.
// 原 round 2 P2 引入 GuestLoginCompletionGate 短路 retry 重发, 但让 .unauthorized /
// .missingCredentials 失败死循环 —— retry 复用同一份坏掉的鉴权状态. round 3 改回 fail-safe:
// retry 时 guest-login 也重跑一次, 保证坏掉的鉴权可被刷新.
// 详见 docs/lessons/2026-04-27-bootstrap-retry-must-not-skip-auth.md.
//
// Story 5.5 round 4 [P2] fix 追加：guest-login 失败也必须经 AppErrorMapper → BootstrapMappedError.
// 原方案: 只 LoadHome 失败包 BootstrapMappedError, guest-login 失败抛 raw APIError → 状态机
// 走 errorDescription fallback → 弹 "Network error: ..." 等 developer-facing 串.
// 修复后: 两条路径都走 mapper, 用户面文案统一受 mapper 控制.
// 详见 docs/lessons/2026-04-27-bootstrap-all-error-paths-route-via-mapper.md.

import XCTest
@testable import PetApp

@MainActor
final class RootViewWireTests: XCTestCase {

    // MARK: - happy: onRoomTap 闭包接到 coordinator.present(.room)

    func testOnRoomTapClosureRoutesToCoordinatorPresentRoom() throws {
        let coordinator = AppCoordinator()
        let viewModel = HomeViewModel()

        viewModel.onRoomTap = { [coordinator] in coordinator.present(.room) }

        viewModel.onRoomTap()

        XCTAssertEqual(coordinator.presentedSheet, .room,
                       "onRoomTap 闭包未把 coordinator 路由到 .room")
    }

    // MARK: - happy: onInventoryTap 闭包接到 coordinator.present(.inventory)

    func testOnInventoryTapClosureRoutesToCoordinatorPresentInventory() throws {
        let coordinator = AppCoordinator()
        let viewModel = HomeViewModel()

        viewModel.onInventoryTap = { [coordinator] in coordinator.present(.inventory) }

        viewModel.onInventoryTap()

        XCTAssertEqual(coordinator.presentedSheet, .inventory,
                       "onInventoryTap 闭包未把 coordinator 路由到 .inventory")
    }

    // MARK: - happy: onComposeTap 闭包接到 coordinator.present(.compose)

    func testOnComposeTapClosureRoutesToCoordinatorPresentCompose() throws {
        let coordinator = AppCoordinator()
        let viewModel = HomeViewModel()

        viewModel.onComposeTap = { [coordinator] in coordinator.present(.compose) }

        viewModel.onComposeTap()

        XCTAssertEqual(coordinator.presentedSheet, .compose,
                       "onComposeTap 闭包未把 coordinator 路由到 .compose")
    }

    // MARK: - Story 5.5 round 3 [P1] fix: bootstrap closure retry 时**必须**重跑 guest-login

    /// retry 路径下, guest-login 必须重跑一次 (复刻 ensureLaunchStateMachineWired 的 closure 模式).
    ///
    /// **round 3 regression guard**: round 2 [P2] 引入 `GuestLoginCompletionGate` 短路 retry 重发,
    /// 但 gate 永久记录 → retry 复用同一份坏掉的鉴权状态死循环 (`.unauthorized` /
    /// `.missingCredentials` 时永远恢复不了, 用户只能重启 App). round 3 改回 fail-safe:
    /// retry 时 guest-login useCase.execute() 也再调一次, 保证坏掉的 token 可被刷新.
    /// useCase.execute() 幂等, 重复一次无副作用; ~50ms 一次往返成本 << "重试可恢复" 的语义价值.
    func testBootstrapClosureRerunsGuestLoginOnRetryToRecoverBadAuthState() async {
        let guestLoginCounter = CallCounter()
        let loadHomeCounter = CallCounter()
        let loadHomeShouldFail = ShouldFailHolder()

        // closure 模仿 RootView.ensureLaunchStateMachineWired 的 step1 形态 (round 3 后).
        // 关键: 每次 closure 跑都无条件调 guest-login, 不再有 gate 短路.
        let bootstrapStep1: @Sendable () async throws -> Void = {
            await guestLoginCounter.increment()
            await loadHomeCounter.increment()
            if await loadHomeShouldFail.value {
                struct LoadHomeFailure: Error {}
                throw LoadHomeFailure()
            }
        }

        let sm = AppLaunchStateMachine(bootstrapStep1: bootstrapStep1)

        // 1) 首次 bootstrap: guest-login 跑 1 次 + load-home 跑 1 次但失败 → state .needsAuth
        await sm.bootstrap()
        let guestCount1 = await guestLoginCounter.value
        let loadCount1 = await loadHomeCounter.value
        XCTAssertEqual(guestCount1, 1, "首次 bootstrap 应跑 guest-login 1 次")
        XCTAssertEqual(loadCount1, 1, "首次 bootstrap 应跑 load-home 1 次")
        if case .needsAuth = sm.state { /* expected */ } else {
            XCTFail("首次 load-home 失败, state 应为 .needsAuth, 实际 \(sm.state)")
        }

        // 2) 让 load-home 第二次成功
        await loadHomeShouldFail.setValue(false)

        // 3) retry: guest-login **必须**再被调用 (round 3 fail-safe) + load-home 重试一次成功 → state .ready
        await sm.retry()
        let guestCount2 = await guestLoginCounter.value
        let loadCount2 = await loadHomeCounter.value
        XCTAssertEqual(
            guestCount2,
            2,
            "retry 后 guest-login **必须**变成 2 次 —— round 3 [P1] fix: retry 不能跳过 guest-login. " +
            "当前 \(guestCount2) 次违反 fail-safe 原则: 跳过 guest-login 会让 .unauthorized 死循环不可恢复."
        )
        XCTAssertEqual(loadCount2, 2, "retry 应再跑 load-home 1 次（共 2 次）")
        XCTAssertEqual(sm.state, .ready, "load-home 第二次成功后, state 应进入 .ready")
    }

    // MARK: - Story 5.5 round 4 [P2] fix #1: guest-login 失败必须经 AppErrorMapper

    /// 复刻 RootView.ensureLaunchStateMachineWired 中 step1 closure 对 guest-login 失败的包装.
    ///
    /// **round 4 regression guard**: round 1-3 fix 只把 LoadHome 失败包成 BootstrapMappedError,
    /// guest-login 失败仍 raw throw APIError → 状态机走 LocalizedError fallback (errorDescription) →
    /// 弹 "Network error: <URLError 系统串>" 等 developer-facing 文案, 而不是 mapper 钦定的
    /// "网络异常，请检查后重试" 等用户面文案. 这条 fix 保证两条失败路径走同一 mapping pipeline.
    ///
    /// 用 .business(1009) 验证: mapper 派 .alert("提示", "服务繁忙，请稍后重试");
    /// raw APIError fallback 走的话只能拿 LocalizedError errorDescription "Business error 1009: ..."
    /// 是 developer 串. 断言 state 是 .alert 才证明走了 mapper.
    func testBootstrapClosureWrapsGuestLoginFailureViaAppErrorMapper() async {
        // 复刻 RootView step1 closure 对 guest-login 失败的包装模式 (round 4 修复后):
        let guestLoginError: APIError = .business(code: 1009, message: "raw server原文 — should be hidden", requestId: "req_x")
        let bootstrapStep1: @Sendable () async throws -> Void = {
            do {
                _ = try await Self.failingGuestLogin(error: guestLoginError)
            } catch {
                throw BootstrapMappedError(
                    presentation: AppErrorMapper.presentation(for: error),
                    underlying: error
                )
            }
            // 不到这里 (前面 throw 了) —— LoadHome 步骤略
        }

        let sm = AppLaunchStateMachine(bootstrapStep1: bootstrapStep1)
        await sm.bootstrap()

        XCTAssertEqual(
            sm.state,
            .needsAuth(presentation: .alert(title: "提示", message: "服务繁忙，请稍后重试")),
            "guest-login 失败必须经 mapper → .alert; round 4 [P2] fix 之前会 fallback 到 LocalizedError " +
            "errorDescription, 弹 developer-facing 串. 当前 state: \(sm.state)"
        )
    }

    /// 同模式 .network 验证: guest-login 网络失败 → mapper 派 .retry, 不再降级为 errorDescription.
    func testBootstrapClosureWrapsGuestLoginNetworkFailureAsRetry() async {
        let guestLoginError: APIError = .network(underlying: URLError(.timedOut))
        let bootstrapStep1: @Sendable () async throws -> Void = {
            do {
                _ = try await Self.failingGuestLogin(error: guestLoginError)
            } catch {
                throw BootstrapMappedError(
                    presentation: AppErrorMapper.presentation(for: error),
                    underlying: error
                )
            }
        }

        let sm = AppLaunchStateMachine(bootstrapStep1: bootstrapStep1)
        await sm.bootstrap()

        XCTAssertEqual(
            sm.state,
            .needsAuth(presentation: .retry(message: "网络异常，请检查后重试")),
            "guest-login 网络错误必须走 mapper → .retry, 用户能点重试触发 cold-start"
        )
    }

    /// helper: 复刻"调 guestLoginUseCase.execute() 抛 error" 的 stub —— 不依赖真 useCase.
    /// 故意抽成 nonisolated static 让 closure 闭包不捕获测试实例.
    nonisolated private static func failingGuestLogin(error: APIError) async throws -> GuestLoginOutput {
        throw error
    }
}
