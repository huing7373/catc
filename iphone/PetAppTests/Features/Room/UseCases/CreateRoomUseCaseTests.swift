// CreateRoomUseCaseTests.swift
// Story 12.7 AC1: CreateRoomUseCase 单元测试（≥3 case：happy / 6003 透传 / network 透传）.

import XCTest
@testable import PetApp

@MainActor
final class CreateRoomUseCaseTests: XCTestCase {

    // MARK: - case#1 happy: repo 返回 roomId="3001" → execute 返回 + appState 写入

    func testExecuteHappyPathWritesAppStateAndReturnsRoomId() async throws {
        let mock = MockRoomRepository()
        mock.createRoomStub = .success(
            CreateRoomResponse(
                room: CreateRoomRoomDTO(
                    id: "3001",
                    creatorUserId: "10001",
                    maxMembers: 4,
                    memberCount: 1,
                    status: 1
                )
            )
        )
        let appState = AppState()
        let useCase = DefaultCreateRoomUseCase(roomRepository: mock, appState: appState)

        let returned = try await useCase.execute()

        XCTAssertEqual(returned, "3001", "execute() 应返回 response.room.id")
        XCTAssertEqual(appState.currentRoomId, "3001",
                       "execute() 成功后必须 setCurrentRoomId 让 RealRoomViewModel.subscribeRoomIdConnect 触发")
        XCTAssertEqual(mock.callCount(of: "createRoom()"), 1)
    }

    // MARK: - case#2 edge: 6003（用户已在房间）→ rethrow + appState 不变

    func testExecuteRethrowsBusiness6003WithoutMutatingAppState() async {
        let mock = MockRoomRepository()
        mock.createRoomStub = .failure(APIError.business(code: 6003, message: "已在房间", requestId: "req_x"))
        let appState = AppState()
        appState.setCurrentRoomId(nil)
        let useCase = DefaultCreateRoomUseCase(roomRepository: mock, appState: appState)

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.business(6003)")
        } catch let APIError.business(code, _, _) {
            XCTAssertEqual(code, 6003)
        } catch {
            XCTFail("意外错误：\(error)")
        }
        XCTAssertNil(appState.currentRoomId,
                     "createRoom 失败必须保持 appState.currentRoomId 不变（不能误写让 UI 切到 RoomView）")
    }

    // MARK: - case#3 edge: network → rethrow + appState 不变

    func testExecuteRethrowsNetworkErrorWithoutMutatingAppState() async {
        let mock = MockRoomRepository()
        mock.createRoomStub = .failure(APIError.network(underlying: URLError(.notConnectedToInternet)))
        let appState = AppState()
        let useCase = DefaultCreateRoomUseCase(roomRepository: mock, appState: appState)

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.network")
        } catch let APIError.network(underlying) {
            XCTAssertEqual((underlying as? URLError)?.code, .notConnectedToInternet)
        } catch {
            XCTFail("意外错误：\(error)")
        }
        XCTAssertNil(appState.currentRoomId)
    }

    // MARK: - case#4 edge: 1009 服务繁忙 → rethrow + appState 不变

    func testExecuteRethrowsBusiness1009WithoutMutatingAppState() async {
        let mock = MockRoomRepository()
        mock.createRoomStub = .failure(APIError.business(code: 1009, message: "服务繁忙", requestId: "req_y"))
        let appState = AppState()
        let useCase = DefaultCreateRoomUseCase(roomRepository: mock, appState: appState)

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.business(1009)")
        } catch let APIError.business(code, _, _) {
            XCTAssertEqual(code, 1009)
        } catch {
            XCTFail("意外错误：\(error)")
        }
        XCTAssertNil(appState.currentRoomId)
    }

    // MARK: - Story 12.7 r6 [P1] fix: stale create response 不能 wipe newer room selection

    /// 场景：entryRoomId == nil（idle Home 点 Create）→ createRoom await 期间用户切 tab + join room "B" →
    /// create HTTP 200 带回 newRoomId "A". 旧实装无条件 setCurrentRoomId("A") 把 user 强制带回 stale room A;
    /// 修复后 guard entry==current → "B" 必须保留.
    ///
    /// **r14 [P1] 变更**：stale path 现在抛 `RoomNavigationStaleError` 而非 silent success ——
    /// 让 ViewModel 触发 home refresh 拿 authoritative state，避免 server/client desync.
    func testExecuteThrowsStaleErrorAndDoesNotWipeNewerRoomSelectionWhenStaleResponseArrives() async throws {
        let mock = MockRoomRepository()
        mock.createRoomStub = .success(
            CreateRoomResponse(
                room: CreateRoomRoomDTO(
                    id: "A",
                    creatorUserId: "10001",
                    maxMembers: 4,
                    memberCount: 1,
                    status: 1
                )
            )
        )
        let appState = AppState()
        // entryRoomId == nil（idle Home）
        appState.setCurrentRoomId(nil)
        let useCase = DefaultCreateRoomUseCase(roomRepository: mock, appState: appState)

        // 在 createRoom await 期间模拟用户切到新房间 B.
        mock.createRoomBeforeReturn = { @Sendable in
            await MainActor.run { appState.setCurrentRoomId("B") }
        }

        do {
            _ = try await useCase.execute()
            XCTFail("stale path 应抛 RoomNavigationStaleError")
        } catch let staleError as RoomNavigationStaleError {
            XCTAssertEqual(staleError.source, .createRoom,
                           "stale signal 必须标注 source=createRoom")
        } catch {
            XCTFail("意外错误：\(error)")
        }
        XCTAssertEqual(appState.currentRoomId, "B",
                       "stale create response 不得 wipe 用户已切的新房间 \"B\"（防 race）")
    }

    // MARK: - Story 12.7 r10 [P2] fix: ABA race - generation token 不能被 nil→B→nil cycle 骗过

    /// 场景（codex r10 P2）：entry == nil（idle Home, gen G0）→ createRoom await 期间用户 join B (gen G1) →
    /// leave B 回 idle (gen G2, currentRoomId nil) → create A HTTP 200 迟到.
    /// 旧 `currentRoomId == entryRoomId` 判断：liveRoomId == nil == entryRoomId → 校验通过 → setCurrentRoomId("A")
    /// → user 被强制切到 stale 房间 A.
    /// 新 generation 判断：entryGen == G0，liveGen == G2 → mismatch → 拒绝 setCurrentRoomId.
    ///
    /// **r14 [P1] 变更**：stale path 现在抛 `RoomNavigationStaleError` 而非 silent success.
    func testExecuteThrowsStaleErrorAcrossABAcycleViaGeneration() async throws {
        let mock = MockRoomRepository()
        mock.createRoomStub = .success(
            CreateRoomResponse(
                room: CreateRoomRoomDTO(
                    id: "A",
                    creatorUserId: "10001",
                    maxMembers: 4,
                    memberCount: 1,
                    status: 1
                )
            )
        )
        let appState = AppState()
        // entryRoomId == nil, entryGen == 0
        XCTAssertNil(appState.currentRoomId)
        let initialGen = appState.roomNavigationGeneration
        let useCase = DefaultCreateRoomUseCase(roomRepository: mock, appState: appState)

        // 在 createRoom await 期间模拟 ABA cycle: nil → "B" → nil（currentRoomId 回到原值，但 gen 已 +2）.
        mock.createRoomBeforeReturn = { @Sendable in
            await MainActor.run {
                appState.setCurrentRoomId("B")
                appState.setCurrentRoomId(nil)
            }
        }

        do {
            _ = try await useCase.execute()
            XCTFail("ABA cycle 后 stale path 必须抛 RoomNavigationStaleError")
        } catch let staleError as RoomNavigationStaleError {
            XCTAssertEqual(staleError.source, .createRoom)
        } catch {
            XCTFail("意外错误：\(error)")
        }
        XCTAssertNil(appState.currentRoomId,
                     "ABA cycle 后 stale create response 不得把 currentRoomId 切到 stale A（generation token guard）")
        XCTAssertGreaterThan(appState.roomNavigationGeneration, initialGen,
                             "navigation generation 必须严格单调（ABA cycle 后已增长）")
    }

    // MARK: - Story 12.7 r14 [P1] fix: happy path 不抛 RoomNavigationStaleError（防回归）

    /// 验证非 stale 场景的 happy path 不会误抛 stale error —— 与 case#1 互补,
    /// 显式 assert "happy 路径下抛 stale 错误" 不是任何代码可达分支.
    func testExecuteHappyPathDoesNotThrowStaleError() async throws {
        let mock = MockRoomRepository()
        mock.createRoomStub = .success(
            CreateRoomResponse(
                room: CreateRoomRoomDTO(
                    id: "5001",
                    creatorUserId: "10001",
                    maxMembers: 4,
                    memberCount: 1,
                    status: 1
                )
            )
        )
        let appState = AppState()
        let useCase = DefaultCreateRoomUseCase(roomRepository: mock, appState: appState)

        // 没有 createRoomBeforeReturn —— navigation gen 不变.
        do {
            let returned = try await useCase.execute()
            XCTAssertEqual(returned, "5001", "happy path 必须正常 return roomId")
        } catch is RoomNavigationStaleError {
            XCTFail("happy path 不应抛 RoomNavigationStaleError（generation 未变）")
        } catch {
            XCTFail("意外错误：\(error)")
        }
        XCTAssertEqual(appState.currentRoomId, "5001")
    }
}
