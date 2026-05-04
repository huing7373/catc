// MotionStateMapperTests.swift
// Story 8.3 AC3: MotionStateMapper.map(_:previous:) pure function 单元测试.
//
// 测试用 Story 8.2 MotionProviderMock.makeActivity(...) 便利构造 CMMotionActivity 实例
// （MotionProviderMock 在 production target，PetAppTests 通过 @testable import PetApp 直接复用）.
// 不引第三方断言 lib（XCTest only；ADR-0002 §3.1）.

import XCTest
import CoreMotion
@testable import PetApp

@MainActor
final class MotionStateMapperTests: XCTestCase {
    // MARK: - happy path（epics.md AC 行 1510-1513）

    /// case 1：stationary=true → .rest（epics.md AC 行 1510）
    func testMap_stationaryActivity_returnsRest() {
        let activity = MotionProviderMock.makeActivity(stationary: true)
        let result = MotionStateMapper.map(activity)
        XCTAssertEqual(result, .rest)
    }

    /// case 2：walking=true → .walk（epics.md AC 行 1511）
    func testMap_walkingActivity_returnsWalk() {
        let activity = MotionProviderMock.makeActivity(walking: true)
        let result = MotionStateMapper.map(activity)
        XCTAssertEqual(result, .walk)
    }

    /// case 3：running=true → .run（epics.md AC 行 1512）
    func testMap_runningActivity_returnsRun() {
        let activity = MotionProviderMock.makeActivity(running: true)
        let result = MotionStateMapper.map(activity)
        XCTAssertEqual(result, .run)
    }

    /// case 4：cycling=true（其他类型）→ .rest（epics.md AC 行 1513）
    func testMap_cyclingActivity_returnsRest() {
        let activity = MotionProviderMock.makeActivity(cycling: true)
        let result = MotionStateMapper.map(activity)
        XCTAssertEqual(result, .rest)
    }

    // MARK: - 优先级裁决（epics.md AC 行 1514）

    /// case 5：walking + stationary 同时为 true → 优先级 .walk（walking 优先于 stationary）
    func testMap_walkingPlusStationary_returnsWalkByPriority() {
        let activity = MotionProviderMock.makeActivity(stationary: true, walking: true)
        let result = MotionStateMapper.map(activity)
        XCTAssertEqual(result, .walk, "walking + stationary 同时为 true 时应按优先级返回 .walk")
    }

    /// case 6（加分）：running + walking + stationary 三 flag 同时为 true → 优先级 .run（running 优先级最高）
    func testMap_runningPlusWalkingPlusStationary_returnsRunByPriority() {
        let activity = MotionProviderMock.makeActivity(stationary: true, walking: true, running: true)
        let result = MotionStateMapper.map(activity)
        XCTAssertEqual(result, .run, "三 flag 同时为 true 时应按优先级返回 .run")
    }

    // MARK: - confidence 不参与裁决（fix-review r1：移除 .low debounce；所有等级按 type 映射）

    /// case 7：confidence=.low + stationary=true → .rest（不再保持 previous；下游不会卡在 stale walk）
    /// fix-review r1（codex P2）：低置信度 stationary 必须能切回 .rest，避免 8.4/8.5 stuck on .walk/.run.
    func testMap_lowConfidence_stationary_returnsRest() {
        let activity = MotionProviderMock.makeActivity(stationary: true, confidence: .low)
        let result = MotionStateMapper.map(activity, previous: .walk)
        XCTAssertEqual(result, .rest, "confidence=.low + stationary 应按 type 返 .rest（不再用 previous 防抖）")
    }

    /// case 8：confidence=.low + walking=true → .walk（不再依赖 previous；按 type 映射）
    func testMap_lowConfidence_walking_returnsWalk() {
        let activity = MotionProviderMock.makeActivity(walking: true, confidence: .low)
        let result = MotionStateMapper.map(activity, previous: .rest)
        XCTAssertEqual(result, .walk, "confidence=.low + walking 应按 type 返 .walk（previous=.rest 不再覆盖）")
    }

    // MARK: - 兜底分支（规则 4）

    /// case 9：unknown=true → .rest（覆盖规则 4：非 stationary/walking/running 全 false → .rest）
    func testMap_unknownActivity_returnsRest() {
        let activity = MotionProviderMock.makeActivity(unknown: true)
        let result = MotionStateMapper.map(activity)
        XCTAssertEqual(result, .rest)
    }

    /// case 10：confidence=.high + previous 非 nil → 仍按 flag 优先级裁决（previous 任何值都不影响）
    func testMap_highConfidence_withPreviousIgnored_returnsByFlagPriority() {
        let activity = MotionProviderMock.makeActivity(running: true, confidence: .high)
        let result = MotionStateMapper.map(activity, previous: .rest)
        XCTAssertEqual(result, .run, "confidence=.high 时 previous 应被忽略，按 flag 返回 .run")
    }
}
