// SyncPetStateUseCaseTests.swift
// Story 15.4 AC5: SyncPetStateUseCase 单元测试.
// 不引第三方断言 lib（XCTest only；ADR-0002 §3.1）.

import XCTest
@testable import PetApp

@MainActor
final class SyncPetStateUseCaseTests: XCTestCase {

    // case#A happy: appState.currentRoomId 非 nil → execute(.walk) → repo 收到 state: 2 + 返 .success(echoedState: 2).
    func testExecute_happy_inRoom_callsRepo_returnsSuccess() async throws {
        let repo = MockPetRepository()
        repo.stubResponse = .success(PetStateSyncResponse(state: 2))
        let appState = AppState()
        appState.setCurrentRoomId("room-X")
        let useCase = DefaultSyncPetStateUseCase(repository: repo, appState: appState)

        let outcome = try await useCase.execute(state: .walk)

        XCTAssertEqual(repo.invocations.count, 1, "in-room → repo.syncPetState 应被调一次")
        XCTAssertEqual(repo.invocations.first?.state, 2, "MotionState.walk.wireValue == 2")
        XCTAssertEqual(outcome, .success(echoedState: 2),
            "happy path 应返 .success(echoedState: 2) —— echo 来自 repo response")
    }

    // case#B edge: appState.currentRoomId == nil → execute(.walk) → repo 未被调用 + 返 .skippedNotInRoom.
    func testExecute_edge_notInRoom_skipsRepo_returnsSkipped() async throws {
        let repo = MockPetRepository()
        repo.stubResponse = .success(PetStateSyncResponse(state: 2))  // 防御性 set；不应被消费
        let appState = AppState()
        // 不调 setCurrentRoomId → currentRoomId 默认 nil.
        let useCase = DefaultSyncPetStateUseCase(repository: repo, appState: appState)

        let outcome = try await useCase.execute(state: .walk)

        XCTAssertEqual(repo.invocations.count, 0,
            "not-in-room → 必须不调 repo.syncPetState（节省流量；epics.md §15.4 行 2437-2438 钦定 client preflight）")
        XCTAssertEqual(outcome, .skippedNotInRoom,
            "not-in-room → 必须返 .skippedNotInRoom（caller 据此跳过节流锚点写入）")
    }

    // case#C edge: repo throw APIError.network → execute throw 透传（测 try-await throws）.
    func testExecute_edge_repoThrows_propagatesError() async {
        let repo = MockPetRepository()
        repo.stubResponse = .failure(APIError.network(underlying: NSError(domain: "test", code: -1)))
        let appState = AppState()
        appState.setCurrentRoomId("room-X")
        let useCase = DefaultSyncPetStateUseCase(repository: repo, appState: appState)

        do {
            _ = try await useCase.execute(state: .run)
            XCTFail("repo throw 时 useCase.execute 必须透传 throw")
        } catch let error as APIError {
            switch error {
            case .network: break  // 期望
            default: XCTFail("应抛 APIError.network，实际抛 \(error)")
            }
        } catch {
            XCTFail("应抛 APIError.network，实际抛 \(type(of: error))")
        }

        XCTAssertEqual(repo.invocations.count, 1, "in-room 时即便 repo throw，也是被调用过的")
    }

    // case#D edge: HTTP echo state != request state（如发 .walk 收 .run）→ 仍返 .success(echoedState:)
    //              不做一致性断言（HTTP ack 仅作信号；V1 §5.2 line 610-613 钦定不可作权威信号源）.
    func testExecute_edge_echoStateMismatch_stillReturnsSuccess() async throws {
        let repo = MockPetRepository()
        repo.stubResponse = .success(PetStateSyncResponse(state: 3))  // 发 .walk(2) 收 echo 3
        let appState = AppState()
        appState.setCurrentRoomId("room-X")
        let useCase = DefaultSyncPetStateUseCase(repository: repo, appState: appState)

        let outcome = try await useCase.execute(state: .walk)

        XCTAssertEqual(outcome, .success(echoedState: 3),
            "HTTP echoedState 必须原样透传 —— UseCase **不**做 echo == request 的一致性断言；")
        XCTAssertEqual(repo.invocations.first?.state, 2, "请求体 state 仍是 .walk.wireValue=2")
    }
}
