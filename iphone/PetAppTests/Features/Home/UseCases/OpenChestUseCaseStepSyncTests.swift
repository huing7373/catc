// OpenChestUseCaseStepSyncTests.swift
// Story 21.5 AC4（路径 A）: 验证 DefaultOpenChestUseCase 在调 /chest/open 之前先 await
// StepSyncTriggerService.triggerManual()（同步步数）；同步失败也不阻塞开箱.
//
// 测试策略（spec Dev Notes "测试边界" 路径 A）:
//   - StepSyncTriggerService 是 `final class @MainActor`（非 protocol）→ 无法直接 mock.
//     但其依赖 `SyncStepsUseCaseProtocol`（可 mock）+ `homeViewModel`（可用真实 HomeViewModel）.
//   - 构造**真实** StepSyncTriggerService（内核注入 OrderRecordingSyncStepsUseCase）+ mock
//     ChestRepository（OrderRecordingChestRepository）→ 注入 DefaultOpenChestUseCase.
//   - 两个 mock 都把调用 marker append 到同一个 @MainActor CallOrderRecorder → 可精确断言
//     "sync 先 open 后" 顺序（spec AC 行 3146 测试意图由单测精确承担；UITest 守回归）.
//
// 覆盖 spec AC 行 3142-3143:
//   - case#1 sync 成功 → repository.openChest 被调 1 次 + snapshot 正确（同步成功 → 用最新步数判定）
//   - case#2 sync 抛 APIError → triggerManual 吞错 → repository.openChest 仍被调 1 次（同步失败不阻塞）
//   - case#3 调用顺序断言：syncSteps 在 openChest 之前（路径 A 精确顺序锚定）

import XCTest
@testable import PetApp

@MainActor
final class OpenChestUseCaseStepSyncTests: XCTestCase {

    // MARK: - Call-order recorder（两个 mock 共享，断言 sync 先 open 后）

    /// @MainActor 隔离的调用序记录器；test 与 mock 都在 main actor → 无需锁.
    final class CallOrderRecorder {
        private(set) var order: [String] = []
        func record(_ marker: String) { order.append(marker) }
    }

    // MARK: - Order-recording SyncStepsUseCase（注入真实 StepSyncTriggerService 内核）

    final class OrderRecordingSyncStepsUseCase: SyncStepsUseCaseProtocol, @unchecked Sendable {
        let recorder: CallOrderRecorder
        var stubError: Error?
        private(set) var invocations: Int = 0

        init(recorder: CallOrderRecorder) { self.recorder = recorder }

        func execute(motionState: MotionState) async throws {
            invocations += 1
            await MainActor.run { self.recorder.record("sync") }
            if let stubError { throw stubError }
        }
    }

    // MARK: - Order-recording ChestRepository

    final class OrderRecordingChestRepository: ChestRepositoryProtocol, @unchecked Sendable {
        let recorder: CallOrderRecorder
        var openChestStub: Result<ChestOpenResponse, Error>
        private(set) var openChestInvocations: Int = 0

        init(recorder: CallOrderRecorder, openChestStub: Result<ChestOpenResponse, Error>) {
            self.recorder = recorder
            self.openChestStub = openChestStub
        }

        func fetchCurrent() async throws -> ChestCurrentResponse {
            XCTFail("OpenChestUseCase 不应调 fetchCurrent")
            throw APIError.decoding(underlying: NSError(domain: "test", code: -1))
        }

        func openChest(_ request: ChestOpenRequest) async throws -> ChestOpenResponse {
            openChestInvocations += 1
            await MainActor.run { self.recorder.record("open") }
            switch openChestStub {
            case .success(let r): return r
            case .failure(let e): throw e
            }
        }
    }

    // MARK: - Fixtures

    private static let testUnlockAt = Date(timeIntervalSince1970: 1_800_000_000)

    private func makeOpenResponse() -> ChestOpenResponse {
        ChestOpenResponse(
            reward: ChestRewardDTO(
                userCosmeticItemId: "0",
                cosmeticItemId: "cos_001",
                name: "星星围巾",
                slot: 1,
                rarity: 1,
                assetUrl: "https://placehold.co/64x64?text=R",
                iconUrl: "https://placehold.co/32x32?text=I"
            ),
            stepAccount: StepAccountInOpenResponse(
                totalSteps: 12000,
                availableSteps: 11160,
                consumedSteps: 840
            ),
            nextChest: ChestSnapshotInOpenResponse(
                id: "30002",
                status: 1,
                unlockAt: Self.testUnlockAt,
                openCostSteps: 1000,
                remainingSeconds: 600
            )
        )
    }

    /// 构造真实 StepSyncTriggerService（内核注入 order-recording SyncStepsUseCase + 真实 HomeViewModel）.
    private func makeRealTriggerService(
        recorder: CallOrderRecorder,
        syncError: Error? = nil
    ) -> (StepSyncTriggerService, OrderRecordingSyncStepsUseCase) {
        let syncMock = OrderRecordingSyncStepsUseCase(recorder: recorder)
        syncMock.stubError = syncError
        let homeVM = HomeViewModel()   // 真实基类即可（StepSyncTriggerService 仅读 .petState 默认 .rest）
        let service = StepSyncTriggerService(syncStepsUseCase: syncMock, homeViewModel: homeVM)
        return (service, syncMock)
    }

    // MARK: - case#1 sync 成功 → openChest 被调 1 次 + snapshot 正确（spec AC 行 3142）

    func testSyncSucceedsThenOpenChestCalled() async throws {
        let recorder = CallOrderRecorder()
        let (service, syncMock) = makeRealTriggerService(recorder: recorder)
        let repo = OrderRecordingChestRepository(recorder: recorder, openChestStub: .success(makeOpenResponse()))
        let appState = AppState()
        let useCase = DefaultOpenChestUseCase(
            repository: repo,
            appState: appState,
            keyGenerator: FakeIdempotencyKeyGenerator(),
            stepSyncTriggerService: service
        )

        let snapshot = try await useCase.execute()

        XCTAssertEqual(syncMock.invocations, 1, "sync 成功路径 SyncStepsUseCase.execute 应被调 1 次")
        XCTAssertEqual(repo.openChestInvocations, 1, "sync 成功后 repository.openChest 应被调 1 次")
        XCTAssertEqual(snapshot.cosmeticItemId, "cos_001")
        XCTAssertEqual(snapshot.rarity, .common)
        // AppState 双字段被 UseCase 写入（既有 21.3 编排不变）
        XCTAssertEqual(appState.currentChest?.id, "30002")
        XCTAssertEqual(appState.currentStepAccount?.availableSteps, 11160)
    }

    // MARK: - case#2 sync 抛 APIError → triggerManual 吞错 → openChest 仍被调（spec AC 行 3143）

    func testSyncFailsButOpenChestStillCalled() async throws {
        let recorder = CallOrderRecorder()
        let (service, syncMock) = makeRealTriggerService(
            recorder: recorder,
            syncError: APIError.network(underlying: URLError(.timedOut))
        )
        let repo = OrderRecordingChestRepository(recorder: recorder, openChestStub: .success(makeOpenResponse()))
        let appState = AppState()
        let useCase = DefaultOpenChestUseCase(
            repository: repo,
            appState: appState,
            keyGenerator: FakeIdempotencyKeyGenerator(),
            stepSyncTriggerService: service
        )

        // sync 抛错被 StepSyncTriggerService.runSync 的 catch 吞掉；triggerManual 非 async throws →
        // OpenChestUseCase.execute() 不应因 sync 失败而中断 → openChest 仍跑 → 不抛错.
        let snapshot = try await useCase.execute()

        XCTAssertEqual(syncMock.invocations, 1, "sync 失败路径 SyncStepsUseCase.execute 仍被调 1 次")
        XCTAssertEqual(repo.openChestInvocations, 1,
                       "sync 失败被 triggerManual 静默吞 → openChest 仍应被调 1 次（不阻塞开箱）")
        XCTAssertEqual(snapshot.cosmeticItemId, "cos_001",
                       "sync 失败不影响开箱返回的 snapshot")
    }

    // MARK: - case#3 调用顺序：syncSteps 在 openChest 之前（路径 A 精确顺序锚定，spec AC 行 3146 测试意图）

    func testSyncIsCalledBeforeOpenChest() async throws {
        let recorder = CallOrderRecorder()
        let (service, _) = makeRealTriggerService(recorder: recorder)
        let repo = OrderRecordingChestRepository(recorder: recorder, openChestStub: .success(makeOpenResponse()))
        let useCase = DefaultOpenChestUseCase(
            repository: repo,
            appState: AppState(),
            keyGenerator: FakeIdempotencyKeyGenerator(),
            stepSyncTriggerService: service
        )

        _ = try await useCase.execute()

        XCTAssertEqual(recorder.order, ["sync", "open"],
                       "调用顺序必须是 sync 先 open 后（OpenChestUseCase Step 0 await triggerManual → Step 2 openChest）")
    }

    // MARK: - case#3b（补充）: stepSyncTriggerService = nil 时不 sync 直开箱（21.3 短路不回归）

    func testNilStepSyncServiceSkipsSyncAndOpensDirectly() async throws {
        let recorder = CallOrderRecorder()
        let repo = OrderRecordingChestRepository(recorder: recorder, openChestStub: .success(makeOpenResponse()))
        let useCase = DefaultOpenChestUseCase(
            repository: repo,
            appState: AppState(),
            keyGenerator: FakeIdempotencyKeyGenerator(),
            stepSyncTriggerService: nil
        )

        _ = try await useCase.execute()

        XCTAssertEqual(recorder.order, ["open"],
                       "stepSyncTriggerService=nil 时 21.3 hook 短路 —— 只 open 不 sync（不破回归）")
        XCTAssertEqual(repo.openChestInvocations, 1)
    }
}
