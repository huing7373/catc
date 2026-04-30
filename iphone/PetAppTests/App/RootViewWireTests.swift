// RootViewWireTests.swift
// Story 2.3 AC6：HomeViewModel ↔ AppCoordinator 闭包注入链路验证.
//
// Story 37.3 修改（ADR-0009 §3.5 步骤 1 + 4 + 5）：
//   - 删除原 onRoomTap / onInventoryTap / onComposeTap 闭包接到 coordinator.present(.room/.inventory/.compose)
//     的 3 个用例（这些 closure / SheetType case 都已删除；主入口 IA 改 4 Tab + HomeContainerView 互斥状态机）.
//   - 保留 bootstrap closure 相关 retry / error mapping 用例（Story 5.5 round 3-4 fix 的 regression guard，
//     与 Story 37.3 主入口改造正交）.
//
// Story 5.5 round 3 [P1] fix 追加：bootstrap closure 在 retry 时**必须**重跑 guest-login.
// 详见 docs/lessons/2026-04-27-bootstrap-retry-must-not-skip-auth.md.
//
// Story 5.5 round 4 [P2] fix 追加：guest-login 失败也必须经 AppErrorMapper → BootstrapMappedError.
// 详见 docs/lessons/2026-04-27-bootstrap-all-error-paths-route-via-mapper.md.

import XCTest
@testable import PetApp

@MainActor
final class RootViewWireTests: XCTestCase {

    // MARK: - Story 5.5 round 3 [P1] fix: bootstrap closure retry 时**必须**重跑 guest-login

    /// retry 路径下, guest-login 必须重跑一次 (复刻 ensureLaunchStateMachineWired 的 closure 模式).
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
        }

        let sm = AppLaunchStateMachine(bootstrapStep1: bootstrapStep1)
        await sm.bootstrap()

        XCTAssertEqual(
            sm.state,
            .needsAuth(presentation: .retry(message: "服务繁忙，请稍后重试")),
            "transient business 1009 必须经 mapper → .retry; 之前 round 4 fallback 会派 .alert 死锁. " +
            "当前 state: \(sm.state)"
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
    nonisolated private static func failingGuestLogin(error: APIError) async throws -> GuestLoginOutput {
        throw error
    }

    // MARK: - Story 37.4 AC5 + AC8: bootstrap 必须把 /home 的 room.currentRoomId 传播进 AppState
    //
    // 旧 (Story 37.3) 测试 testBootstrapPropagates...ToCoordinator 直接断言 coordinator.currentRoomId,
    // Story 37.4 落地 AppState 后改为断言 appState.currentRoomId（数据源切换；intent 完全保留：
    // bootstrap 完成后 currentRoomId 必须传播；不传播就会让"已在房间"用户被错误落到 idle home screen）.
    // 详见 docs/lessons/2026-04-30-coordinator-must-mirror-loaded-home-room-state.md（lesson 仍有效，
    // 仅 source of truth 从 coordinator 切换到 AppState）.

    /// 已在房间用户 (`/home` 返回 `room.currentRoomId != nil`) 在 bootstrap 完成后,
    /// `appState.currentRoomId` 必须等于 server 返回值 —— HomeContainerView 互斥状态机
    /// 以此为决策入参；不写就会让用户被错误落到 idle home screen.
    func testBootstrapPropagatesLoadedHomeRoomIdToAppState() async {
        let appState = AppState()
        XCTAssertNil(appState.currentRoomId, "前置：appState.currentRoomId 默认 nil")

        let inRoomData = makeHomeData(currentRoomId: "room_abc123")

        // 复刻 RootView.ensureLaunchStateMachineWired step1 closure 的 await MainActor.run 内
        // 单写模式 (appState.applyHomeData(homeData) —— Story 37.4 收口后取代 coordinator.currentRoomId 双写).
        let bootstrapStep1: @Sendable () async throws -> Void = {
            await MainActor.run {
                appState.applyHomeData(inRoomData)
            }
        }

        let sm = AppLaunchStateMachine(bootstrapStep1: bootstrapStep1)
        await sm.bootstrap()

        XCTAssertEqual(sm.state, .ready, "bootstrap 成功 → state 进入 .ready")
        XCTAssertEqual(
            appState.currentRoomId,
            "room_abc123",
            "bootstrap 完成后 appState.currentRoomId 必须 = homeData.room.currentRoomId; " +
            "否则 HomeContainerView 会把已在房间用户错误渲染成 idle home screen."
        )
        XCTAssertTrue(
            HomeRoomDispatcher.shouldShowRoom(currentRoomId: appState.currentRoomId),
            "已在房间用户 bootstrap 完后, HomeContainerView 必须切到 RoomViewPlaceholder 态."
        )
    }

    /// 镜像用例: `/home` 返回 `room.currentRoomId == nil` 时, appState.currentRoomId 仍为 nil
    /// (HomeContainerView 渲染 idle home screen).
    func testBootstrapKeepsAppStateCurrentRoomIdNilWhenHomeRoomIsEmpty() async {
        let appState = AppState()
        let idleData = makeHomeData(currentRoomId: nil)

        let bootstrapStep1: @Sendable () async throws -> Void = {
            await MainActor.run {
                appState.applyHomeData(idleData)
            }
        }

        let sm = AppLaunchStateMachine(bootstrapStep1: bootstrapStep1)
        await sm.bootstrap()

        XCTAssertEqual(sm.state, .ready)
        XCTAssertNil(appState.currentRoomId, "/home 返回 currentRoomId=nil → appState 应保持 nil.")
        XCTAssertFalse(
            HomeRoomDispatcher.shouldShowRoom(currentRoomId: appState.currentRoomId),
            "未在房间 → HomeContainerView 应渲染 HomeView 而非 RoomViewPlaceholder."
        )
    }

    /// 测试 helper: 构造最小可用 HomeData 用于 currentRoomId 传播验证 (其它字段填占位值).
    private func makeHomeData(currentRoomId: String?) -> HomeData {
        HomeData(
            user: HomeUser(id: "u_test", nickname: "tester", avatarUrl: ""),
            pet: nil,
            stepAccount: HomeStepAccount(totalSteps: 0, availableSteps: 0, consumedSteps: 0),
            chest: HomeChest(
                id: "c_test",
                status: .counting,
                unlockAt: Date(timeIntervalSince1970: 0),
                openCostSteps: 0,
                remainingSeconds: 0
            ),
            room: HomeRoom(currentRoomId: currentRoomId)
        )
    }
}
