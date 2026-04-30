// HomeContainerViewTests.swift
// Story 37.3 AC6：HomeContainerView 互斥状态机决策 helper 单元测试.
//
// 测试策略（ADR-0002 §3.1 钦定路径）：
//   - 不引入 ViewInspector / SnapshotTesting.
//   - HomeContainerView 内部互斥决策抽 `HomeRoomDispatcher.shouldShowRoom(currentRoomId:)`
//     纯函数 helper，让 XCTest 直接覆盖三态（nil / non-nil / transition）.
//   - 与 HomePetNameResolver / HomeNicknameResolver 同精神：fileprivate 子视图 body 难直接断言,
//     抽决策逻辑为纯函数后单测 lock 语义即可.

import XCTest
@testable import PetApp

@MainActor
final class HomeContainerViewTests: XCTestCase {

    // MARK: - happy: currentRoomId == nil → 显示 HomeView

    /// idle 态决策：currentRoomId 为 nil 时 shouldShowRoom 返回 false（HomeContainerView 渲染 HomeView）.
    func testIdleStateShouldShowHomeView() throws {
        XCTAssertFalse(
            HomeRoomDispatcher.shouldShowRoom(currentRoomId: nil),
            "currentRoomId == nil 时应返回 false (idle → 显示 HomeView)"
        )
    }

    // MARK: - happy: currentRoomId 非 nil → 显示 RoomView

    /// inRoom 态决策：currentRoomId 非 nil 时 shouldShowRoom 返回 true（HomeContainerView 渲染 RoomViewPlaceholder）.
    func testInRoomStateShouldShowRoomView() throws {
        XCTAssertTrue(
            HomeRoomDispatcher.shouldShowRoom(currentRoomId: "room_1234567"),
            "currentRoomId 非 nil 时应返回 true (inRoom → 显示 RoomView)"
        )
    }

    // MARK: - edge: 空字符串 currentRoomId 仍视为 inRoom（防 caller 漏检；server 契约定义 roomId 非空）

    /// 空字符串视为有效 inRoom 态：HomeRoomDispatcher 仅对 nil 做特例；server 契约保证 roomId 非空.
    /// 该 case 防 caller 把 server 返回的空字符串当作 "已退出房间" 误判.
    func testEmptyStringRoomIdStillTriggersInRoom() throws {
        XCTAssertTrue(
            HomeRoomDispatcher.shouldShowRoom(currentRoomId: ""),
            "空字符串 currentRoomId 仍应返回 true（仅 nil 表示 idle 态）"
        )
    }

    // MARK: - edge: 状态切换 nil → roomId → nil

    /// 状态切换链：idle → inRoom → idle 决策连续性（过渡动画由 SwiftUI .animation 自动接管，不在单测验证）.
    func testTransitionFromIdleToInRoomToIdle() throws {
        XCTAssertFalse(HomeRoomDispatcher.shouldShowRoom(currentRoomId: nil),
                       "初始 idle 态")
        XCTAssertTrue(HomeRoomDispatcher.shouldShowRoom(currentRoomId: "room_1234567"),
                      "切到 inRoom 态")
        XCTAssertFalse(HomeRoomDispatcher.shouldShowRoom(currentRoomId: nil),
                       "退回 idle 态")
    }
}
