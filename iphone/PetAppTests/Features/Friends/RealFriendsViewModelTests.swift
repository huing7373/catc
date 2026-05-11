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

    // MARK: - case#3b r8 P2 regression: unrecognized business code 必须 forward 原 error
    //
    // 与 RealHomeViewModelTests.testOnJoinRoomConfirmUnknownBusinessCodeForwardsServerMessage 同精神 —
    // r8 P2 钦定 onJoinFriendTap 也犯同样 lossy rewrap，修复后必须把 server message 透传给 ErrorPresenter.
    func testOnJoinFriendTapUnknownBusinessCodeForwardsServerMessage() async throws {
        let appState = AppState()
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let mockJoin = MockJoinRoomUseCaseFriends()
        mockJoin.executeStub = .failure(APIError.business(
            code: 9999,
            message: "Server-defined message",
            requestId: "req-abc"
        ))
        let vm = RealFriendsViewModel(appState: appState)
        vm.bind(
            appState: appState,
            joinRoomUseCase: mockJoin,
            errorPresenter: presenter
        )

        let friend = Friend(
            id: "fX",
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
            XCTFail("9999 应走 alert（permanent class），实际 \(String(describing: presenter.current))")
            return
        }
        XCTAssertEqual(message, "Server-defined message",
                       "unrecognized business code 必须 forward server-provided message（不能 rewrap 成空串走 generic fallback '操作失败，请稍后重试'）")
    }

    // MARK: - case#r9-P2 regression: onJoinFriendTap catch 必须 stale-guard 不能 present alert 到新 room

    /// fix-review round 9 P2: user 在 friends tab 点 join friend A 的 room "A" → join HTTP in-flight
    /// 期间用户通过其他路径切到 room "B" → join "A" 路径抛 6002 错 → 老实装会无条件 presentAlert("房间已满")
    /// 弹在 room B 上. 新实装 entry==current guard → mismatch 不 present.
    /// 对应 lesson 2026-05-11-async-error-handler-must-stale-guard-room-id-and-client-identity-12-7-r9.md.
    func testOnJoinFriendTapStaleErrorAfterRoomSwitchSkipsPresenter() async throws {
        let appState = AppState()
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let mockJoin = MockJoinRoomUseCaseFriends()
        // 让 mock 在 execute 中把 currentRoomId 切到 "B"（mid-await 模拟用户切换），再抛 6002 错
        mockJoin.executeStub = .failure(APIError.business(code: 6002, message: "房间已满", requestId: "req_stale_friend"))
        mockJoin.onExecuteAsync = { @Sendable _ in
            await MainActor.run { appState.setCurrentRoomId("B") }
        }
        let vm = RealFriendsViewModel(appState: appState)
        vm.bind(
            appState: appState,
            joinRoomUseCase: mockJoin,
            errorPresenter: presenter
        )

        XCTAssertNil(appState.currentRoomId, "前置：entry currentRoomId 必须 nil")

        let friend = Friend(
            id: "f_stale",
            name: "Stale",
            online: true,
            status: .inRoom,
            statusText: "在房间 A",
            currentRoomId: "A",
            color: nil
        )
        vm.onJoinFriendTap(friend: friend)
        try? await waitForCallCount(mock: mockJoin, method: "execute(roomId:)", expected: 1)
        try? await Task.sleep(nanoseconds: 60_000_000)

        XCTAssertEqual(appState.currentRoomId, "B",
                       "执行过程中应已切到 'B' 模拟用户切换")
        XCTAssertNil(presenter.current,
                     "currentRoomId 已切到 B（entry=nil → current=B），stale 6002 错误不应弹到新 room B")
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
    /// fix-review round 9 P2 新增：async hook，让 stale-guard 回归测试能在 stub.get throw **之前**
    /// 确定性 await 完成 setCurrentRoomId（与 RealHomeViewModelTests.MockJoinRoomUseCase 同模式）.
    var onExecuteAsync: (@Sendable (String) async -> Void)?

    func execute(roomId: String) async throws {
        record(method: "execute(roomId:)", arguments: [roomId])
        if let onExecuteAsync = onExecuteAsync { await onExecuteAsync(roomId) }
        try executeStub.get()
    }
}
#endif
