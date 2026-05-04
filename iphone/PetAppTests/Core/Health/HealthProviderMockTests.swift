// HealthProviderMockTests.swift
// Story 8.1 AC6: HealthProviderMock 路径单元测试.
//
// 5 个 case（4 个 epics.md AC 钦定 + 1 个加分 reset）:
//   1. happy: requestPermission 返回 true → readDailyTotalSteps 返回设定值
//   2. edge: requestPermission 返回 false + readDailyTotalStepsError → readDailyTotalSteps 抛 .permissionDenied
//   3. happy: 同一天读两次 → mock stub 一致返回（mock 不实现 cache，但断言 stub 一致性）
//   4. edge: 跨自然日两读 → 不同 dayStart key 返回不同值
//   5. 加分项: reset() 清空 stub + invocations + callCount
//
// 设计基线（详见 story 8-1-healthkit-接入.md AC6 段）:
// - 仅覆盖 HealthProviderMock 行为；不测 HealthProviderImpl 真 HK 路径（那是 AC7 集成测试范围）
// - @MainActor 测试 class（与 ADR-0002 §3.2 已知坑"跨 MainActor 边界 + async test"对齐）
// - 不引第三方断言 lib（XCTest only，ADR-0002 §3.1）

import XCTest
@testable import PetApp

@MainActor
final class HealthProviderMockTests: XCTestCase {
    // case 1 - happy: requestPermission 返回 true → readDailyTotalSteps 返回设定值
    func testRequestPermissionGranted_thenReadStepsReturnsStubbedValue() async throws {
        let mock = HealthProviderMock()
        let today = Date()
        let dayStart = Calendar.current.startOfDay(for: today)
        mock.requestPermissionStub = .success(true)
        mock.readDailyTotalStepsStub[dayStart] = 5000

        let granted = try await mock.requestPermission()
        XCTAssertTrue(granted)
        XCTAssertEqual(mock.requestPermissionCallCount, 1)

        let steps = try await mock.readDailyTotalSteps(date: today)
        XCTAssertEqual(steps, 5000)
        XCTAssertEqual(mock.readDailyTotalStepsCallCount, 1)
    }

    // case 2 - edge: requestPermission 返回 false → readDailyTotalSteps 抛 .permissionDenied
    // 注：epics.md AC 原文是 "permission false → readSteps throw"；mock 用 readDailyTotalStepsError 显式 stub.
    func testPermissionDenied_thenReadStepsThrows() async throws {
        let mock = HealthProviderMock()
        let today = Date()
        mock.requestPermissionStub = .success(false)  // mock 允许显式 false
        mock.readDailyTotalStepsError = HealthProviderError.permissionDenied

        let granted = try await mock.requestPermission()
        XCTAssertFalse(granted)

        do {
            _ = try await mock.readDailyTotalSteps(date: today)
            XCTFail("expected throw")
        } catch let error as HealthProviderError {
            XCTAssertEqual(error, .permissionDenied)
        }
    }

    // case 3 - happy: 同一天读两次 → 第二次仍走 mock stub（mock 不实现 cache，但断言 stub 一致性）
    func testSameDayTwoReads_returnsConsistentValue() async throws {
        let mock = HealthProviderMock()
        let today = Date()
        let dayStart = Calendar.current.startOfDay(for: today)
        mock.readDailyTotalStepsStub[dayStart] = 3000

        let first = try await mock.readDailyTotalSteps(date: today)
        let second = try await mock.readDailyTotalSteps(date: today)
        XCTAssertEqual(first, 3000)
        XCTAssertEqual(second, 3000)
        // mock 不 cache，记录 2 次调用（与 HealthProviderImpl 行为差异；测试用 mock 不模拟 cache）
        XCTAssertEqual(mock.readDailyTotalStepsCallCount, 2)
    }

    // case 4 - edge: 跨自然日（不同 dayStart key）→ 返回不同 stub 值
    func testCrossDayReads_returnsDifferentDayValues() async throws {
        let mock = HealthProviderMock()
        let yesterday = Date(timeIntervalSinceNow: -86_400)
        let yesterdayStart = Calendar.current.startOfDay(for: yesterday)
        let todayStart = Calendar.current.startOfDay(for: Date())
        mock.readDailyTotalStepsStub[yesterdayStart] = 4000
        mock.readDailyTotalStepsStub[todayStart] = 1500

        let yesterdaySteps = try await mock.readDailyTotalSteps(date: yesterday)
        let todaySteps = try await mock.readDailyTotalSteps(date: Date())

        XCTAssertEqual(yesterdaySteps, 4000)
        XCTAssertEqual(todaySteps, 1500)
        XCTAssertEqual(mock.readDailyTotalStepsCallCount, 2)
    }

    // case 5 - 加分项: reset() 清空 stub + invocations
    func testReset_clearsAllStubsAndInvocations() async throws {
        let mock = HealthProviderMock()
        mock.requestPermissionStub = .success(false)
        mock.readDailyTotalStepsStub[Calendar.current.startOfDay(for: Date())] = 100
        _ = try await mock.requestPermission()
        _ = try await mock.readDailyTotalSteps(date: Date())

        mock.reset()

        XCTAssertEqual(mock.invocations, [])
        XCTAssertEqual(mock.requestPermissionCallCount, 0)
        XCTAssertEqual(mock.readDailyTotalStepsCallCount, 0)
        // reset 后默认 success(true) + 空表
        let granted = try await mock.requestPermission()
        XCTAssertTrue(granted)
        let steps = try await mock.readDailyTotalSteps(date: Date())
        XCTAssertEqual(steps, 0)
    }
}
