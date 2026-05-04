// MotionProviderMockTests.swift
// Story 8.2 AC6: MotionProviderMock 路径单元测试.
//
// 6 个 case（4 个 epics.md AC 钦定 + 2 个加分 reset / stop-then-start）:
//   1. happy: requestPermission granted → startUpdates 后 handler 收到 inject 的 activity
//   2. edge: requestPermission denied → caller 不 inject 时 handler 不收到事件
//   3. happy: stopUpdates 后 inject 不再触发 handler
//   4. edge: 同时多次 startUpdates → 只生效第一次（防止重复订阅）
//   5. 加分项：stop-then-start 重订阅 → 新 handler 收到事件、老 handler 不再收到
//   6. 加分项：reset() 清空 stub + invocations + handler 注册
//
// 设计基线（详见 story 8-2-coremotion-接入.md AC6 段）:
// - 仅覆盖 MotionProviderMock 行为；不测 MotionProviderImpl 真 CM 路径（那是 AC7 集成测试范围）
// - @MainActor 测试 class（与 ADR-0002 §3.2 已知坑"跨 MainActor 边界 + async test"对齐）
// - 不引第三方断言 lib（XCTest only，ADR-0002 §3.1）

import XCTest
import CoreMotion
@testable import PetApp

@MainActor
final class MotionProviderMockTests: XCTestCase {
    // case 1 - happy: requestPermission 成功 → startUpdates 后 handler 收到注入的事件
    func testRequestPermissionGranted_thenHandlerReceivesInjectedActivity() async throws {
        let mock = MotionProviderMock()
        mock.requestPermissionStub = .success(true)

        let granted = try await mock.requestPermission()
        XCTAssertTrue(granted)
        XCTAssertEqual(mock.requestPermissionCallCount, 1)

        let received = ReceivedActivities()
        mock.startUpdates { activity in received.append(activity) }
        XCTAssertEqual(mock.startUpdatesCallCount, 1)

        let walking = MotionProviderMock.makeActivity(walking: true)
        mock.injectActivity(walking)
        XCTAssertEqual(received.count, 1)
        XCTAssertTrue(received.first?.walking ?? false)
        XCTAssertEqual(mock.handlerInvocationCount, 1)
    }

    // case 2 - edge: requestPermission 失败 → startUpdates 不触发任何回调
    // 注：mock 不强制"requestPermission 失败时 startUpdates 也失败"——caller 决策（与 8.1 同模式）.
    // 此 case 验证：caller 不调 inject 时永远收不到事件；调了 startUpdates 也只是 handler 注册，
    // 没人 inject 就没人收（与"权限失败 → 系统不发事件"现实路径同语义）.
    func testRequestPermissionDenied_thenNoHandlerInvocationWithoutInject() async throws {
        let mock = MotionProviderMock()
        mock.requestPermissionStub = .success(false)

        let granted = try await mock.requestPermission()
        XCTAssertFalse(granted)

        let received = ReceivedActivities()
        mock.startUpdates { activity in received.append(activity) }

        // 没人 inject，handler 不被调
        XCTAssertEqual(received.count, 0)
        XCTAssertEqual(mock.handlerInvocationCount, 0)
    }

    // case 3 - happy: stopUpdates 后 handler 不再收到事件
    func testStopUpdates_thenInjectActivityDoesNotInvokeHandler() async throws {
        let mock = MotionProviderMock()
        let received = ReceivedActivities()
        mock.startUpdates { activity in received.append(activity) }

        // 先 inject 一次确认正常路径
        mock.injectActivity(MotionProviderMock.makeActivity(stationary: true))
        XCTAssertEqual(received.count, 1)

        mock.stopUpdates()
        XCTAssertEqual(mock.stopUpdatesCallCount, 1)

        // stop 之后 inject 不应触发 handler
        mock.injectActivity(MotionProviderMock.makeActivity(walking: true))
        XCTAssertEqual(received.count, 1)  // 仍是 1
    }

    // case 4 - edge: 同时多次 startUpdates → 只生效第一次（防止重复订阅）
    func testMultipleStartUpdates_thenFirstHandlerOnlyReceivesActivity() async throws {
        let mock = MotionProviderMock()

        let firstHandlerReceived = ReceivedActivities()
        let secondHandlerReceived = ReceivedActivities()

        mock.startUpdates { activity in firstHandlerReceived.append(activity) }
        mock.startUpdates { activity in secondHandlerReceived.append(activity) }
        XCTAssertEqual(mock.startUpdatesCallCount, 2)  // 调用了两次

        mock.injectActivity(MotionProviderMock.makeActivity(running: true))

        // 只有第一个 handler 收到（second 被忽略）
        XCTAssertEqual(firstHandlerReceived.count, 1)
        XCTAssertTrue(firstHandlerReceived.first?.running ?? false)
        XCTAssertEqual(secondHandlerReceived.count, 0)
    }

    // case 5 - 加分项：stopUpdates 后再 startUpdates 视作全新订阅（handler 替换为新 closure）
    func testStopThenStartUpdatesAgain_thenNewHandlerReceivesActivity() async throws {
        let mock = MotionProviderMock()
        let firstHandlerReceived = ReceivedActivities()
        let secondHandlerReceived = ReceivedActivities()

        mock.startUpdates { activity in firstHandlerReceived.append(activity) }
        mock.injectActivity(MotionProviderMock.makeActivity(walking: true))
        XCTAssertEqual(firstHandlerReceived.count, 1)

        mock.stopUpdates()

        mock.startUpdates { activity in secondHandlerReceived.append(activity) }
        mock.injectActivity(MotionProviderMock.makeActivity(running: true))

        // 老 handler 不再收到
        XCTAssertEqual(firstHandlerReceived.count, 1)
        // 新 handler 收到了
        XCTAssertEqual(secondHandlerReceived.count, 1)
        XCTAssertTrue(secondHandlerReceived.first?.running ?? false)
    }

    // case 7 - review round 1 P2: stop/restart race——stale callback 必须被 generation token 丢弃
    //
    // 时序模拟（在真 CMMotionActivityManager 上对应"stop 之后系统残留 enqueue 的 callback"）:
    //   1. start handler1，capture generation == G1
    //   2. stop（generation 推进到 G1+1）
    //   3. start handler2（generation 推进到 G1+2）
    //   4. 以"旧 generation G1"inject activity——模拟 handler1 注册期间 enqueue 但延迟到 step 3 后才 invoke 的 callback
    //   → 必须被 generation check 丢弃（handler2 也不能收到，handler1 不能收到）
    func testStopRestartRace_staleCallbackWithOldGenerationIsDiscarded() async throws {
        let mock = MotionProviderMock()

        let handler1Received = ReceivedActivities()
        mock.startUpdates { activity in handler1Received.append(activity) }
        let oldGeneration = mock.captureGeneration()

        mock.stopUpdates()

        let handler2Received = ReceivedActivities()
        mock.startUpdates { activity in handler2Received.append(activity) }

        // 关键断言：以"旧 generation"inject——这模拟 stop 前 enqueue 的 stale callback
        let stale = MotionProviderMock.makeActivity(walking: true)
        mock.injectActivity(stale, expectedGeneration: oldGeneration)

        // 老 handler 不能收到（generation 不 match → 丢弃）
        XCTAssertEqual(handler1Received.count, 0,
                       "stale callback 不能漏给已 stop 的 handler1")
        // 新 handler **也**不能收到——这是 round 1 P2 的核心：stale event 不能串到新订阅
        XCTAssertEqual(handler2Received.count, 0,
                       "stale callback 不能串到新一代 handler2（review round 1 P2 修复）")
        // handlerInvocationCount 也不应增加（因为 generation check 拦截 in 进 forward 之前）
        XCTAssertEqual(mock.handlerInvocationCount, 0)

        // 验证当前 generation 下 inject 仍然 work（避免误伤）
        let fresh = MotionProviderMock.makeActivity(running: true)
        mock.injectActivity(fresh, expectedGeneration: mock.captureGeneration())
        XCTAssertEqual(handler2Received.count, 1)
        XCTAssertTrue(handler2Received.first?.running ?? false)
        XCTAssertEqual(mock.handlerInvocationCount, 1)
    }

    // case 8 - review round 1 P2: 普通 injectActivity（无 generation 参数）保持原行为
    // 防止 case 7 的 generation 机制误伤现有 case 1-6 的 path（"测试通过 inject 触发 handler" 仍然 work）.
    func testInjectActivity_withoutGenerationParam_stillForwardsAfterStopRestart() async throws {
        let mock = MotionProviderMock()
        let received = ReceivedActivities()

        mock.startUpdates { _ in /* drop */ }
        mock.stopUpdates()

        // 全新 startUpdates 后用普通 injectActivity（不带 generation）必须正常 forward——
        // 否则 case 5（stop-then-start 重订阅）会被打破.
        mock.startUpdates { activity in received.append(activity) }
        mock.injectActivity(MotionProviderMock.makeActivity(stationary: true))
        XCTAssertEqual(received.count, 1)
    }

    // case 6 - 加分项：reset() 清空 stub + invocations + handler 注册
    func testReset_clearsAllStubsHandlerAndInvocations() async throws {
        let mock = MotionProviderMock()
        mock.requestPermissionStub = .success(false)
        mock.startUpdates { _ in }
        _ = try await mock.requestPermission()
        mock.injectActivity(MotionProviderMock.makeActivity(walking: true))

        mock.reset()

        XCTAssertEqual(mock.invocations, [])
        XCTAssertEqual(mock.requestPermissionCallCount, 0)
        XCTAssertEqual(mock.startUpdatesCallCount, 0)
        XCTAssertEqual(mock.stopUpdatesCallCount, 0)
        XCTAssertEqual(mock.handlerInvocationCount, 0)

        // reset 后默认 success(true) + 无 handler 注册
        let granted = try await mock.requestPermission()
        XCTAssertTrue(granted)

        // reset 后再 inject 不会触发任何 handler（registeredHandler 已清）
        mock.injectActivity(MotionProviderMock.makeActivity(running: true))
        XCTAssertEqual(mock.handlerInvocationCount, 0)
    }
}

// MARK: - Test Helper

/// 辅助记录 handler 收到的 activity 序列；
/// 用 reference type + 同步 setter 让闭包内"append"可见——
/// closure 是 @Sendable 不能 capture inout var.
/// `@unchecked Sendable`：测试串行调用，无真竞态（与 MotionProviderMock @unchecked Sendable 同精神）.
private final class ReceivedActivities: @unchecked Sendable {
    private let lock = NSLock()
    private var activities: [CMMotionActivity] = []

    var count: Int {
        lock.lock(); defer { lock.unlock() }
        return activities.count
    }
    var first: CMMotionActivity? {
        lock.lock(); defer { lock.unlock() }
        return activities.first
    }

    func append(_ activity: CMMotionActivity) {
        lock.lock(); defer { lock.unlock() }
        activities.append(activity)
    }
}
