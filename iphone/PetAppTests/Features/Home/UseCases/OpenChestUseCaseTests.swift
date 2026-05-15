// OpenChestUseCaseTests.swift
// Story 21.3 AC9: OpenChestUseCase 单测覆盖（≥ 6 case；本文件给 9 case 全覆盖
// happy + idempotencyKey 复用 + error 透传 + fail-fast unknown enum）.
//
// 测试目标：验证 UseCase = (optional triggerManual) → generate idempotencyKey → repo.openChest
//   → DTO 转 domain → MainActor.run { appState.applyCurrentChest + applySyncedStepAccount }
//   → return ChestRewardSnapshot；错误透传严格；未知 enum fail-fast 抛 .decoding.

import XCTest
@testable import PetApp

@MainActor
final class OpenChestUseCaseTests: XCTestCase {

    // MARK: - Helpers

    private static let testUnlockAt = Date(timeIntervalSince1970: 1_800_000_000)

    /// 默认成功响应：counting 态新宝箱 + 步数账户 + common rarity reward.
    private func makeResponse(
        rewardCosmeticItemId: String = "cos_001",
        rewardName: String = "星星围巾",
        rewardSlot: Int = 1,
        rewardRarity: Int = 1,
        rewardAssetUrl: String = "https://placehold.co/64x64?text=Reward",
        rewardIconUrl: String = "https://placehold.co/32x32?text=Icon",
        rewardUserCosmeticItemId: String = "0",
        totalSteps: Int = 12000,
        availableSteps: Int = 11160,
        consumedSteps: Int = 840,
        nextChestId: String = "30002",
        nextChestStatus: Int = 1,
        nextChestUnlockAt: Date = testUnlockAt,
        nextChestOpenCostSteps: Int = 1000,
        nextChestRemainingSeconds: Int = 600
    ) -> ChestOpenResponse {
        ChestOpenResponse(
            reward: ChestRewardDTO(
                userCosmeticItemId: rewardUserCosmeticItemId,
                cosmeticItemId: rewardCosmeticItemId,
                name: rewardName,
                slot: rewardSlot,
                rarity: rewardRarity,
                assetUrl: rewardAssetUrl,
                iconUrl: rewardIconUrl
            ),
            stepAccount: StepAccountInOpenResponse(
                totalSteps: totalSteps,
                availableSteps: availableSteps,
                consumedSteps: consumedSteps
            ),
            nextChest: ChestSnapshotInOpenResponse(
                id: nextChestId,
                status: nextChestStatus,
                unlockAt: nextChestUnlockAt,
                openCostSteps: nextChestOpenCostSteps,
                remainingSeconds: nextChestRemainingSeconds
            )
        )
    }

    /// 给 AppState 预填一个初始 chest + stepAccount，让测试断言"AppState 上次值保留"语义可表达.
    private func makeAppStateWithInitial() -> AppState {
        let appState = AppState()
        appState.currentChest = HomeChest(
            id: "initial-c1",
            status: .counting,
            unlockAt: Date(timeIntervalSince1970: 1_700_000_000),
            openCostSteps: 1000,
            remainingSeconds: 100
        )
        appState.currentStepAccount = HomeStepAccount(
            totalSteps: 5000,
            availableSteps: 4000,
            consumedSteps: 1000
        )
        return appState
    }

    // MARK: - case#1 happy: status=1 counting → AppState 双字段写入 + 返回 snapshot

    func testExecuteHappyCountingWritesBothAppStateFieldsAndReturnsSnapshot() async throws {
        let mock = MockChestRepository()
        mock.openChestStub = .success(makeResponse(rewardRarity: 1, nextChestStatus: 1))
        let appState = AppState()
        let fakeGen = FakeIdempotencyKeyGenerator()
        let useCase = DefaultOpenChestUseCase(
            repository: mock,
            appState: appState,
            keyGenerator: fakeGen,
            stepSyncTriggerService: nil
        )

        let snapshot = try await useCase.execute()

        // AppState.currentChest 写入正确
        XCTAssertEqual(appState.currentChest?.id, "30002")
        XCTAssertEqual(appState.currentChest?.status, .counting)
        XCTAssertEqual(appState.currentChest?.unlockAt, Self.testUnlockAt)
        XCTAssertEqual(appState.currentChest?.openCostSteps, 1000)
        XCTAssertEqual(appState.currentChest?.remainingSeconds, 600)
        // AppState.currentStepAccount 写入正确
        XCTAssertEqual(appState.currentStepAccount?.totalSteps, 12000)
        XCTAssertEqual(appState.currentStepAccount?.availableSteps, 11160)
        XCTAssertEqual(appState.currentStepAccount?.consumedSteps, 840)
        // snapshot 字段对齐 mock DTO
        XCTAssertEqual(snapshot.cosmeticItemId, "cos_001")
        XCTAssertEqual(snapshot.name, "星星围巾")
        XCTAssertEqual(snapshot.slot, 1)
        XCTAssertEqual(snapshot.rarity, .common)
        XCTAssertEqual(snapshot.assetUrl, "https://placehold.co/64x64?text=Reward")
        XCTAssertEqual(snapshot.iconUrl, "https://placehold.co/32x32?text=Icon")
        // repo 被调一次
        XCTAssertEqual(mock.openChestInvocations, 1)
    }

    // MARK: - case#2 happy: status=2 unlockable（边界）→ AppState.currentChest.status = .unlockable

    func testExecuteHappyUnlockableNextChestWritesUnlockableStatus() async throws {
        let mock = MockChestRepository()
        mock.openChestStub = .success(makeResponse(nextChestStatus: 2, nextChestRemainingSeconds: 0))
        let appState = AppState()
        let useCase = DefaultOpenChestUseCase(
            repository: mock,
            appState: appState,
            keyGenerator: FakeIdempotencyKeyGenerator()
        )

        _ = try await useCase.execute()

        XCTAssertEqual(appState.currentChest?.status, .unlockable)
        XCTAssertEqual(appState.currentChest?.remainingSeconds, 0)
    }

    // MARK: - case#3 happy: 同一 execute 调用内 keyGenerator.generate() 仅被调 **1** 次

    func testExecuteCallsGenerateExactlyOnce() async throws {
        let mock = MockChestRepository()
        mock.openChestStub = .success(makeResponse())
        let appState = AppState()
        let fakeGen = FakeIdempotencyKeyGenerator()
        fakeGen.keys = ["only-key-1"]   // 仅一个 key
        let useCase = DefaultOpenChestUseCase(
            repository: mock,
            appState: appState,
            keyGenerator: fakeGen
        )

        _ = try await useCase.execute()

        XCTAssertEqual(fakeGen.callCount, 1, "同一 execute 仅 generate 一次")
        XCTAssertEqual(mock.lastOpenChestRequest?.idempotencyKey, "only-key-1",
                       "request 内 idempotencyKey 应等于 fakeGen 提供的 key")
    }

    // MARK: - case#4 happy: 连续两次 execute（caller 重试）→ 两次 generate 各拿不同 key

    func testRepeatedExecuteGeneratesDistinctKeysForEachCall() async throws {
        let mock = MockChestRepository()
        mock.openChestStub = .success(makeResponse())
        let appState = AppState()
        let fakeGen = FakeIdempotencyKeyGenerator()
        fakeGen.keys = ["call-1-key", "call-2-key"]
        let useCase = DefaultOpenChestUseCase(
            repository: mock,
            appState: appState,
            keyGenerator: fakeGen
        )

        _ = try await useCase.execute()
        _ = try await useCase.execute()

        XCTAssertEqual(fakeGen.callCount, 2)
        XCTAssertEqual(mock.openChestInvocations, 2)
        XCTAssertEqual(mock.openChestRequests.count, 2)
        XCTAssertEqual(mock.openChestRequests[0].idempotencyKey, "call-1-key")
        XCTAssertEqual(mock.openChestRequests[1].idempotencyKey, "call-2-key")
        XCTAssertNotEqual(mock.openChestRequests[0].idempotencyKey,
                          mock.openChestRequests[1].idempotencyKey,
                          "连续两次 execute 应拿不同 key")
    }

    // MARK: - case#5 edge: APIError.business(4002 宝箱未解锁) → throw + AppState 保留旧值

    func testExecuteThrowsBusinessError4002PreservesAppState() async {
        let mock = MockChestRepository()
        mock.openChestStub = .failure(APIError.business(code: 4002, message: "宝箱未解锁", requestId: "req_4002"))
        let appState = makeAppStateWithInitial()
        let useCase = DefaultOpenChestUseCase(
            repository: mock,
            appState: appState,
            keyGenerator: FakeIdempotencyKeyGenerator()
        )

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.business(4002)")
        } catch let APIError.business(code, message, requestId) {
            XCTAssertEqual(code, 4002)
            XCTAssertEqual(message, "宝箱未解锁")
            XCTAssertEqual(requestId, "req_4002")
        } catch {
            XCTFail("意外错误类型：\(error)")
        }

        // AppState 双字段保留 setUp 初始值（未被覆盖）
        XCTAssertEqual(appState.currentChest?.id, "initial-c1")
        XCTAssertEqual(appState.currentStepAccount?.availableSteps, 4000)
    }

    // MARK: - case#6 edge: APIError.business(3002 步数不足) → throw + AppState 保留旧值

    func testExecuteThrowsBusinessError3002PreservesAppState() async {
        let mock = MockChestRepository()
        mock.openChestStub = .failure(APIError.business(code: 3002, message: "步数不足", requestId: "req_3002"))
        let appState = makeAppStateWithInitial()
        let useCase = DefaultOpenChestUseCase(
            repository: mock,
            appState: appState,
            keyGenerator: FakeIdempotencyKeyGenerator()
        )

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.business(3002)")
        } catch let APIError.business(code, _, _) {
            XCTAssertEqual(code, 3002)
        } catch {
            XCTFail("意外错误类型：\(error)")
        }

        XCTAssertEqual(appState.currentChest?.id, "initial-c1")
        XCTAssertEqual(appState.currentStepAccount?.availableSteps, 4000)
    }

    // MARK: - case#7 edge: APIError.network → throw + AppState 保留旧值

    func testExecuteThrowsNetworkErrorPreservesAppState() async {
        let mock = MockChestRepository()
        mock.openChestStub = .failure(APIError.network(underlying: URLError(.timedOut)))
        let appState = makeAppStateWithInitial()
        let useCase = DefaultOpenChestUseCase(
            repository: mock,
            appState: appState,
            keyGenerator: FakeIdempotencyKeyGenerator()
        )

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.network")
        } catch let APIError.network(underlying) {
            XCTAssertEqual((underlying as? URLError)?.code, .timedOut)
        } catch {
            XCTFail("意外错误类型：\(error)")
        }

        XCTAssertEqual(appState.currentChest?.id, "initial-c1")
        XCTAssertEqual(appState.currentStepAccount?.availableSteps, 4000)
    }

    // MARK: - case#8 edge: 未知 nextChest.status=99 → APIError.decoding(unknownNextChestStatus(99))
    //
    // 与 LoadChestUseCaseTests.testExecuteUnknownChestStatusThrowsDecoding 同精神（未知 enum fail-fast）.

    func testExecuteUnknownNextChestStatusThrowsDecoding() async {
        let mock = MockChestRepository()
        mock.openChestStub = .success(makeResponse(nextChestStatus: 99))
        let appState = makeAppStateWithInitial()
        let useCase = DefaultOpenChestUseCase(
            repository: mock,
            appState: appState,
            keyGenerator: FakeIdempotencyKeyGenerator()
        )

        do {
            _ = try await useCase.execute()
            XCTFail("未识别 nextChest.status 应抛 APIError.decoding，不应静默 fallback")
        } catch let APIError.decoding(underlying) {
            guard let chestErr = underlying as? ChestOpenDecodingError else {
                XCTFail("underlying 应是 ChestOpenDecodingError，实得 \(underlying)")
                return
            }
            XCTAssertEqual(chestErr, .unknownNextChestStatus(99))
        } catch {
            XCTFail("意外错误类型：\(error)")
        }

        // fail-fast 抛错时 AppState 双字段保留上次值
        XCTAssertEqual(appState.currentChest?.id, "initial-c1")
        XCTAssertEqual(appState.currentStepAccount?.availableSteps, 4000)
    }

    // MARK: - case#9 edge: 未知 reward.rarity=99 → APIError.decoding(unknownRewardRarity(99))

    func testExecuteUnknownRewardRarityThrowsDecoding() async {
        let mock = MockChestRepository()
        mock.openChestStub = .success(makeResponse(rewardRarity: 99))
        let appState = makeAppStateWithInitial()
        let useCase = DefaultOpenChestUseCase(
            repository: mock,
            appState: appState,
            keyGenerator: FakeIdempotencyKeyGenerator()
        )

        do {
            _ = try await useCase.execute()
            XCTFail("未识别 reward.rarity 应抛 APIError.decoding，不应静默 fallback")
        } catch let APIError.decoding(underlying) {
            guard let chestErr = underlying as? ChestOpenDecodingError else {
                XCTFail("underlying 应是 ChestOpenDecodingError，实得 \(underlying)")
                return
            }
            XCTAssertEqual(chestErr, .unknownRewardRarity(99))
        } catch {
            XCTFail("意外错误类型：\(error)")
        }

        XCTAssertEqual(appState.currentChest?.id, "initial-c1")
        XCTAssertEqual(appState.currentStepAccount?.availableSteps, 4000)
    }

    // MARK: - case#10 happy: rarity 转 enum 正确（legendary 边界值）

    func testExecuteRewardRarityLegendaryDecodedCorrectly() async throws {
        let mock = MockChestRepository()
        mock.openChestStub = .success(makeResponse(rewardRarity: 4))
        let appState = AppState()
        let useCase = DefaultOpenChestUseCase(
            repository: mock,
            appState: appState,
            keyGenerator: FakeIdempotencyKeyGenerator()
        )

        let snapshot = try await useCase.execute()

        XCTAssertEqual(snapshot.rarity, .legendary)
    }
}
