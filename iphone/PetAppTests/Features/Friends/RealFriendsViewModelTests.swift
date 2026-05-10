// RealFriendsViewModelTests.swift
// Story 12.7 AC7: RealFriendsViewModel.onJoinFriendTap 升级版（接 JoinRoomUseCase）单测.
//
// 覆盖：
//   - onJoinFriendTap(friend.currentRoomId="3001") → mock JoinRoomUseCase.execute("3001") 调一次
//   - onJoinFriendTap(friend.currentRoomId=nil) → 不调 UseCase + lastToastMessage 提示
//   - JoinRoomUseCase throw 6002 → ErrorPresenter 收到 alert "房间已满"

import XCTest
import SwiftUI
@testable import PetApp

@MainActor
final class RealFriendsViewModelTests: XCTestCase {

    // MARK: - case#1 happy: friend 含 currentRoomId → mock UseCase 调一次

    func testOnJoinFriendTapWithCurrentRoomIdInvokesJoinRoomUseCase() async throws {
        let appState = AppState()
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let mockJoin = MockJoinRoomUseCaseFriends()
        mockJoin.executeStub = .success(())

        let vm = RealFriendsViewModel(appState: appState)
        vm.bind(
            appState: appState,
            joinRoomUseCase: mockJoin,
            errorPresenter: presenter
        )

        let friend = Friend(
            id: "f1",
            name: "夏夏",
            online: true,
            status: .inRoom,
            statusText: "在房间 3001",
            currentRoomId: "3001",
            color: nil
        )

        vm.onJoinFriendTap(friend: friend)
        try? await waitForCallCount(mock: mockJoin, method: "execute(roomId:)", expected: 1)

        XCTAssertEqual(mockJoin.callCount(of: "execute(roomId:)"), 1)
        XCTAssertEqual(mockJoin.lastArgumentsSnapshot().first as? String, "3001")
    }

    // MARK: - case#2 edge: friend.currentRoomId nil → 不调 UseCase + 写 lastToastMessage

    func testOnJoinFriendTapWithNilCurrentRoomIdShowsToast() {
        let appState = AppState()
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let mockJoin = MockJoinRoomUseCaseFriends()
        let vm = RealFriendsViewModel(appState: appState)
        vm.bind(
            appState: appState,
            joinRoomUseCase: mockJoin,
            errorPresenter: presenter
        )

        let friend = Friend(
            id: "f2",
            name: "小米",
            online: true,
            status: .online,
            statusText: "刚刚活跃",
            currentRoomId: nil,
            color: nil
        )

        vm.onJoinFriendTap(friend: friend)

        XCTAssertEqual(mockJoin.callCount(of: "execute(roomId:)"), 0,
                       "currentRoomId nil 时不应调 UseCase（防御性兜底）")
        XCTAssertEqual(vm.lastToastMessage, "好友不在房间中")
    }

    // MARK: - case#3 edge: JoinRoomUseCase throw 6002 → ErrorPresenter 收到 alert "房间已满"

    func testOnJoinFriendTap6002PresentsAlertRoomFull() async throws {
        let appState = AppState()
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let mockJoin = MockJoinRoomUseCaseFriends()
        mockJoin.executeStub = .failure(APIError.business(code: 6002, message: "房间已满", requestId: "req_x"))
        let vm = RealFriendsViewModel(appState: appState)
        vm.bind(
            appState: appState,
            joinRoomUseCase: mockJoin,
            errorPresenter: presenter
        )

        let friend = Friend(
            id: "f3",
            name: "Mocha",
            online: true,
            status: .inRoom,
            statusText: "在房间 3001",
            currentRoomId: "3001",
            color: nil
        )

        vm.onJoinFriendTap(friend: friend)
        try? await waitForPresenterAlert(presenter: presenter)

        guard case let .alert(_, message) = presenter.current else {
            XCTFail("应弹 alert，实际 \(String(describing: presenter.current))")
            return
        }
        XCTAssertTrue(message.contains("房间已满"), "6002 alert message 应含'房间已满'，实际 \(message)")
    }

    // MARK: - helpers

    private func waitForCallCount(mock: MockBase, method: String, expected: Int) async throws {
        let deadline = Date().addingTimeInterval(1.0)
        while Date() < deadline {
            if mock.callCount(of: method) >= expected { return }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
    }

    private func waitForPresenterAlert(presenter: ErrorPresenter) async throws {
        let deadline = Date().addingTimeInterval(1.0)
        while Date() < deadline {
            if presenter.current != nil { return }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
    }
}

// MARK: - Inline mock

#if DEBUG
/// 与 RealHomeViewModelTests.MockJoinRoomUseCase 同精神 —— 但因 Swift 编译期把测试文件视作同一 module,
/// 类名需在 target 内全局唯一；用 `*Friends` 后缀避免 collision.
final class MockJoinRoomUseCaseFriends: MockBase, JoinRoomUseCaseProtocol, @unchecked Sendable {
    var executeStub: Result<Void, Error> = .failure(MockError.notStubbed)

    func execute(roomId: String) async throws {
        record(method: "execute(roomId:)", arguments: [roomId])
        try executeStub.get()
    }
}
#endif
