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
}
