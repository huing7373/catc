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
// Story 5.5 round 2 [P2] fix 追加：bootstrap closure 内 `GuestLoginCompletionGate` actor 短路语义.
// 复刻 RootView.ensureLaunchStateMachineWired 的 closure 模式（不依赖 SwiftUI lifecycle）,
// 验证: guest-login 成功后, retry() 重跑 closure 时 guest-login 不再重发.

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

    // MARK: - Story 5.5 round 2 [P2] fix: bootstrap closure 短路 guest-login 重发

    /// retry 路径下, 已经成功的 guest-login 不应再次发起 (复刻 ensureLaunchStateMachineWired 的 closure 模式).
    ///
    /// **regression guard**: round 2 [P2] finding 复现 —— 原 closure 没短路, 第二次跑 step1 closure
    /// 时 guest-login useCase.execute() 又被调用一次, 多走一次 /auth/guest-login 往返,
    /// 让本应只重试 /home 的 transient 失败也得 auth endpoint 健康才能恢复.
    /// 修复后 GuestLoginCompletionGate actor 在第一次成功后置 hasCompleted=true, 第二次跑 closure
    /// 时直接跳过 useCase.execute(), 只重跑 loadHome 失败的下半段.
    func testBootstrapClosureSkipsGuestLoginAfterSuccessfulCompletion() async {
        let guestLoginCounter = CallCounter()
        let loadHomeCounter = CallCounter()
        let loadHomeShouldFail = ShouldFailHolder()
        let gate = GuestLoginCompletionGate()

        // closure 模仿 RootView.ensureLaunchStateMachineWired 的 step1 形态.
        let bootstrapStep1: @Sendable () async throws -> Void = {
            let alreadyLoggedIn = await gate.hasCompleted
            if !alreadyLoggedIn {
                await guestLoginCounter.increment()
                await gate.markCompleted()
            }
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

        // 3) retry: guest-login **不应**再被调用 + load-home 重试一次成功 → state .ready
        await sm.retry()
        let guestCount2 = await guestLoginCounter.value
        let loadCount2 = await loadHomeCounter.value
        XCTAssertEqual(
            guestCount2,
            1,
            "retry 后 guest-login **仍**应是 1 次 —— gate 必须短路重发. 当前 \(guestCount2) 次违反 [P2] fix"
        )
        XCTAssertEqual(loadCount2, 2, "retry 应再跑 load-home 1 次（共 2 次）")
        XCTAssertEqual(sm.state, .ready, "load-home 第二次成功后, state 应进入 .ready")
    }
}
