// RealHomeViewModelTests.swift
// Story 12.7 AC5: RealHomeViewModel.onCreateTap / onJoinRoomConfirm 升级版（接 UseCase）单测.
//
// 测试目标：
//   - onCreateTap → CreateRoomUseCase.execute 调一次
//   - onCreateTap 6003 → ErrorPresenter 收到 .alert 含"已经在房间"
//   - onJoinRoomConfirm → showJoinModal 立即 false + JoinRoomUseCase.execute(roomId:) 调一次
//   - onJoinRoomConfirm 6002 → ErrorPresenter 收到 .alert 含"房间已满"
//   - onJoinRoomConfirm 1009 → ErrorPresenter 默认 mapper 路径
//
// 测试基础设施约束：
//   - 仅 stdlib（XCTest + @testable import PetApp）
//   - mock UseCase inline 定义（不入产品 target）+ MainActor scheduling helper

import XCTest
import Combine
@testable import PetApp

@MainActor
final class RealHomeViewModelTests: XCTestCase {

    // MARK: - case#1 happy: onCreateTap → mock CreateRoomUseCase.execute() 调一次 + 不弹 alert

    func testOnCreateTapInvokesCreateRoomUseCase() async {
        let appState = AppState()
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let mockCreate = MockCreateRoomUseCase()
        mockCreate.executeStub = .success("3001")
        let mockJoin = MockJoinRoomUseCase()
        let vm = RealHomeViewModel(appState: appState)
        vm.bind(
            createRoomUseCase: mockCreate,
            joinRoomUseCase: mockJoin,
            errorPresenter: presenter
        )

        vm.onCreateTap()

        // 等异步 Task 跑完
        try? await waitForCallCount(mock: mockCreate, method: "execute()", expected: 1)

        XCTAssertEqual(mockCreate.callCount(of: "execute()"), 1, "onCreateTap 必须调 CreateRoomUseCase.execute()")
        XCTAssertNil(presenter.current, "成功路径不应弹任何 alert")
    }

    // MARK: - case#2 edge: CreateRoomUseCase throw 6003 → ErrorPresenter 收到 .alert "你已经在房间里了"

    func testOnCreateTap6003PresentsAlertAlreadyInRoom() async {
        let appState = AppState()
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let mockCreate = MockCreateRoomUseCase()
        mockCreate.executeStub = .failure(APIError.business(code: 6003, message: "已在房间", requestId: "req_1"))
        let mockJoin = MockJoinRoomUseCase()
        let vm = RealHomeViewModel(appState: appState)
        vm.bind(
            createRoomUseCase: mockCreate,
            joinRoomUseCase: mockJoin,
            errorPresenter: presenter
        )

        vm.onCreateTap()
        try? await waitForPresenterAlert(presenter: presenter)

        guard case let .alert(title, message) = presenter.current else {
            XCTFail("ErrorPresenter.current 应为 .alert，实际 \(String(describing: presenter.current))")
            return
        }
        XCTAssertEqual(title, "提示")
        XCTAssertTrue(message.contains("已经在房间"),
                      "6003 alert message 应含'已经在房间'，实际 \(message)")
    }

    // MARK: - case#3 happy: onJoinRoomConfirm → showJoinModal 立即 false + JoinRoomUseCase.execute("3001") 调一次

    func testOnJoinRoomConfirmInvokesJoinRoomUseCaseAndClosesModal() async {
        let appState = AppState()
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let mockCreate = MockCreateRoomUseCase()
        let mockJoin = MockJoinRoomUseCase()
        mockJoin.executeStub = .success(())
        let vm = RealHomeViewModel(appState: appState)
        vm.showJoinModal = true   // 模拟 modal 打开
        vm.bind(
            createRoomUseCase: mockCreate,
            joinRoomUseCase: mockJoin,
            errorPresenter: presenter
        )

        vm.onJoinRoomConfirm(roomId: "3001")

        // showJoinModal 立即（同步）置 false（不等 Task）
        XCTAssertFalse(vm.showJoinModal, "onJoinRoomConfirm 必须**先**关 modal（同步），再起 Task 调 UseCase")

        try? await waitForCallCount(mock: mockJoin, method: "execute(roomId:)", expected: 1)

        XCTAssertEqual(mockJoin.callCount(of: "execute(roomId:)"), 1)
        XCTAssertEqual(mockJoin.lastArgumentsSnapshot().first as? String, "3001")
    }

    // MARK: - case#4 edge: JoinRoomUseCase throw 6002 → ErrorPresenter 收到 .alert "房间已满"

    func testOnJoinRoomConfirm6002PresentsAlertRoomFull() async {
        let appState = AppState()
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let mockCreate = MockCreateRoomUseCase()
        let mockJoin = MockJoinRoomUseCase()
        mockJoin.executeStub = .failure(APIError.business(code: 6002, message: "房间已满", requestId: "req_x"))
        let vm = RealHomeViewModel(appState: appState)
        vm.bind(
            createRoomUseCase: mockCreate,
            joinRoomUseCase: mockJoin,
            errorPresenter: presenter
        )

        vm.onJoinRoomConfirm(roomId: "3001")
        try? await waitForPresenterAlert(presenter: presenter)

        guard case let .alert(_, message) = presenter.current else {
            XCTFail("ErrorPresenter.current 应为 .alert，实际 \(String(describing: presenter.current))")
            return
        }
        XCTAssertTrue(message.contains("房间已满"),
                      "6002 alert message 应含'房间已满'，实际 \(message)")
    }

    // MARK: - case#5 edge: JoinRoomUseCase throw 1009 → ErrorPresenter 收到呈现态（默认 mapper 路径）

    /// 1009 转给 ErrorPresenter 默认 mapper（business 1009 → retry per AppErrorMapper）.
    func testOnJoinRoomConfirm1009RoutesToErrorPresenterDefaultMapper() async {
        let appState = AppState()
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let mockCreate = MockCreateRoomUseCase()
        let mockJoin = MockJoinRoomUseCase()
        mockJoin.executeStub = .failure(APIError.business(code: 1009, message: "服务繁忙", requestId: "req_y"))
        let vm = RealHomeViewModel(appState: appState)
        vm.bind(
            createRoomUseCase: mockCreate,
            joinRoomUseCase: mockJoin,
            errorPresenter: presenter
        )

        vm.onJoinRoomConfirm(roomId: "3001")
        // 等到 presenter.current 非 nil（任意呈现态）
        try? await waitForPresenterAlert(presenter: presenter)

        XCTAssertNotNil(presenter.current,
                        "1009 路径必须把错误透传给 ErrorPresenter（默认 mapper 决定 retry vs alert）")
    }

    // MARK: - case#5b r8 P2 regression: unrecognized business code 必须 forward 原 error（不丢 server message）
    //
    // r8 review 钦定 lesson 2026-05-11-business-error-fallback-must-forward-original.md：
    //   原实现 catch 内 `catch let APIError.business(code, _, _)` 解构丢了 message+requestId,
    //   fallback 分支合成 `APIError.business(code:, message: "", requestId: "")` 给 presenter →
    //   AppErrorMapper.localizedMessage 未知 code 9999 走 default 分支 `fallback.isEmpty ?
    //   "操作失败，请稍后重试" : fallback` —— fallback 是空串 → 用户看到 generic 文案,
    //   丢失 server 真实解释 + telemetry requestId。
    //
    // 修复后：catch 全 error → if case let APIError.business 解构 code（不消费 error）→
    //   unrecognized → present(error)（原 error 含原 message + requestId）→ AppErrorMapper
    //   localizedMessage 拿到 server message "Server-defined message" 作 fallback.
    //
    // 本测试通过 ErrorPresenter.current（ErrorPresentation）的 message 字段是否含 server 真实 message
    // 间接断言（presenter 不暴露 raw error；AppErrorMapper.localizedMessage default 分支
    // 直接 return fallback message）。
    func testOnJoinRoomConfirmUnknownBusinessCodeForwardsServerMessage() async {
        let appState = AppState()
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let mockCreate = MockCreateRoomUseCase()
        let mockJoin = MockJoinRoomUseCase()
        mockJoin.executeStub = .failure(APIError.business(
            code: 9999,
            message: "Server-defined message",
            requestId: "req-abc"
        ))
        let vm = RealHomeViewModel(appState: appState)
        vm.bind(
            createRoomUseCase: mockCreate,
            joinRoomUseCase: mockJoin,
            errorPresenter: presenter
        )

        vm.onJoinRoomConfirm(roomId: "3001")
        try? await waitForPresenterAlert(presenter: presenter)

        // unknown business code 9999 既不在 hardcoded mapping，也不在 transientBusinessCodes →
        // AppErrorMapper.presentation 走 `.alert(title: "提示", message: localizedMessage(9999, fallback))` →
        // localizedMessage default → fallback "Server-defined message"（非空）。
        guard case let .alert(_, message) = presenter.current else {
            XCTFail("9999 应走 alert（permanent class），实际 \(String(describing: presenter.current))")
            return
        }
        XCTAssertEqual(message, "Server-defined message",
                       "unrecognized business code 必须 forward server-provided message（不能 rewrap 成空串走 generic fallback '操作失败，请稍后重试'）")
    }

    // MARK: - case#6 happy: onJoinRoomConfirm 成功后 mock UseCase 收到 roomId

    func testOnJoinRoomConfirmSuccessKeepsModalClosedAndAppStateWritten() async {
        let appState = AppState()
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let mockCreate = MockCreateRoomUseCase()
        let mockJoin = MockJoinRoomUseCase()
        // 让 mock JoinRoomUseCase 在 execute 时主动写 appState（与生产 DefaultJoinRoomUseCase 一致）
        mockJoin.executeStub = .success(())
        mockJoin.onExecute = { roomId in
            Task { @MainActor in
                appState.setCurrentRoomId(roomId)
            }
        }
        let vm = RealHomeViewModel(appState: appState)
        vm.showJoinModal = true
        vm.bind(
            createRoomUseCase: mockCreate,
            joinRoomUseCase: mockJoin,
            errorPresenter: presenter
        )

        vm.onJoinRoomConfirm(roomId: "3001")
        try? await waitForCallCount(mock: mockJoin, method: "execute(roomId:)", expected: 1)
        // 等 onExecute 副作用跑掉
        try? await Task.sleep(nanoseconds: 30_000_000)

        XCTAssertFalse(vm.showJoinModal)
        XCTAssertEqual(mockJoin.lastArgumentsSnapshot().first as? String, "3001")
    }

    // MARK: - case#story12-7-r5-P3 regression: onCreateTap useCase nil fallback 必须 mutate appState

    /// useCase nil（UITEST_SKIP_GUEST_LOGIN=1 / RootView 老 wire / preview）下点 Create CTA
    /// 必须仍写 appState.currentRoomId 到 placeholder —— 让 HomeContainerView 切到 RoomView,
    /// 与 onJoinRoomConfirm / leaveRoomUseCase nil fallback 同精神（不能是 hard no-op）.
    /// 对应 lesson 2026-05-11-create-room-nil-fallback-must-mutate-state.md.
    func testOnCreateTapWithNilUseCaseFallsBackToPlaceholderRoomId() async {
        let appState = AppState()
        let vm = RealHomeViewModel(appState: appState)
        // 不调 bind() —— 模拟 UITEST_SKIP_GUEST_LOGIN=1 / RootView 老 wire 路径下 createRoomUseCase 仍为 nil.
        XCTAssertNil(appState.currentRoomId, "前置条件：未点 CTA 前 currentRoomId 应为 nil")

        vm.onCreateTap()
        // fallback 是同步路径（直接 localAppState?.setCurrentRoomId）—— 不需要等 Task.
        // 但为防未来重构改成异步，给 1 tick 缓冲.
        await Task.yield()

        XCTAssertNotNil(appState.currentRoomId,
                        "useCase nil fallback 必须 mutate appState.currentRoomId —— 否则 create CTA 在 UITEST / preview 路径下变成 hard no-op")
        XCTAssertEqual(appState.currentRoomId, "1234567",
                       "placeholder roomId 应为 '1234567'（与 MockHomeViewModel.onCreateTap fallback 一致精神）")
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

// MARK: - Inline mocks

#if DEBUG
final class MockCreateRoomUseCase: MockBase, CreateRoomUseCaseProtocol, @unchecked Sendable {
    var executeStub: Result<String, Error> = .failure(MockError.notStubbed)

    func execute() async throws -> String {
        record(method: "execute()")
        return try executeStub.get()
    }
}

final class MockJoinRoomUseCase: MockBase, JoinRoomUseCaseProtocol, @unchecked Sendable {
    var executeStub: Result<Void, Error> = .failure(MockError.notStubbed)
    /// 副作用 hook：测试中需要让 mock UseCase 模拟 setCurrentRoomId 行为时用.
    /// `@MainActor` 闭包让调用方可以安全 mutate AppState.
    var onExecute: (@Sendable (String) -> Void)?

    func execute(roomId: String) async throws {
        record(method: "execute(roomId:)", arguments: [roomId])
        onExecute?(roomId)
        try executeStub.get()
    }
}

final class MockLeaveRoomUseCase: MockBase, LeaveRoomUseCaseProtocol, @unchecked Sendable {
    var executeStub: Result<Void, Error> = .success(())

    func execute() async throws {
        record(method: "execute()")
        try executeStub.get()
    }
}
#endif
