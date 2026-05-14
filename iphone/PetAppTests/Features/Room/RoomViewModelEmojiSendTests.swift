// RoomViewModelEmojiSendTests.swift
// Story 18.3 AC9 9c: RoomViewModel / RealRoomViewModel / MockRoomViewModel onEmojiSelected 链路单测.
//
// 测试范围（与 story file AC9 钦定一致，≥4 case）：
//   - case#1 happy: MockRoomViewModel.onEmojiSelected → activeEmojis 立即追加 1 项
//                   + invocations 含 .emojiSelected(code:)
//   - case#2 happy: 连点 3 次 → activeEmojis 长度 3 + 3 个 UUID 两两不同
//   - case#3 happy: RealRoomViewModel 整链 (本地入队 + Task 内 WS send)
//                   + 等 Task 跑完后 mockWS.sentMessages.count == 1
//   - case#4 edge: WS send 失败 (.notConnected) → activeEmojis 仍追加 1 项 (不回滚)
//                  + ErrorPresenter 接收到 toast "网络不佳，对方可能看不到"
//
// 测试基础设施 (与既有 RoomViewModelEmojiPanelTests / RealRoomViewModelTests 同源):
//   - 仅依赖 stdlib (XCTest + @testable import PetApp)
//   - @MainActor class 注解让所有测试在主线程跑 (vm 改 @Published 字段 + ErrorPresenter 必须 main thread)
//   - 直接断言 vm @Published 字段 + invocations 数组 + ErrorPresenter.current

import XCTest
@testable import PetApp

@MainActor
final class RoomViewModelEmojiSendTests: XCTestCase {

    // MARK: - case#1 happy: MockRoomViewModel.onEmojiSelected → 入队 + invocations 记录

    /// Story 18.3 AC6 mock 路径: 入队 activeEmojis (userId / emojiCode 都正确)
    /// + invocations 记录 .emojiSelected(code:).
    func test_mockOnEmojiSelected_appendsActiveEmojiAndRecordsInvocation() {
        let vm = MockRoomViewModel(currentUserId: "u1")
        XCTAssertTrue(vm.activeEmojis.isEmpty,
                      "初始 activeEmojis 必须为空")
        XCTAssertTrue(vm.invocations.isEmpty,
                      "初始 invocations 必须为空")

        vm.onEmojiSelected(code: "wave")

        XCTAssertEqual(vm.activeEmojis.count, 1,
                       "onEmojiSelected 后 activeEmojis 必须追加 1 项")
        XCTAssertEqual(vm.activeEmojis.first?.userId, "u1",
                       "activeEmoji.userId 必须等于 vm.currentUserId")
        XCTAssertEqual(vm.activeEmojis.first?.emojiCode, "wave",
                       "activeEmoji.emojiCode 必须等于入参 code")
        XCTAssertEqual(vm.invocations, [.emojiSelected(code: "wave")],
                       "invocations 必须记录 .emojiSelected(code: 'wave')")
    }

    // MARK: - case#2 happy: 连点 3 次 → activeEmojis 长度 3 + UUID 两两不同

    /// Story 18.3 决策点 4 钦定 id = UUID() 让 SwiftUI ForEach 区分同 emojiCode 多次入队
    /// (epics.md 行 2697 "连点 3 次 → activeEmojis 添加 3 项").
    func test_rapidConsecutiveEmojiSelections_appendIndependentItems() {
        let vm = MockRoomViewModel(currentUserId: "u1")

        vm.onEmojiSelected(code: "wave")
        vm.onEmojiSelected(code: "wave")
        vm.onEmojiSelected(code: "wave")

        XCTAssertEqual(vm.activeEmojis.count, 3,
                       "连点 3 次相同 code → activeEmojis 必须追加 3 项独立 entries")

        // 3 个 UUID 必两两不同 (UUID 算法 collision 概率 negligible)
        let ids = vm.activeEmojis.map { $0.id }
        XCTAssertEqual(Set(ids).count, 3,
                       "3 项 UUID 必须两两不同 (让 SwiftUI ForEach 区分)")

        // invocations 也累计 3 项
        XCTAssertEqual(vm.invocations.count, 3,
                       "invocations 必须累计 3 项 .emojiSelected")
        XCTAssertEqual(vm.invocations,
                       [.emojiSelected(code: "wave"),
                        .emojiSelected(code: "wave"),
                        .emojiSelected(code: "wave")])
    }

    // MARK: - case#3 happy: RealRoomViewModel 整链 (本地入队 + Task 内 WS send)

    /// Story 18.3 AC5 path: 本地立即入队 (Step A 同步) + Task 内 WS send (Step C 异步).
    /// 用真实 DefaultSendEmojiUseCase + Mock WebSocketClient 简化 mock 层级.
    func test_realOnEmojiSelected_appendsLocallyAndSendsWS() async throws {
        let mockWS = WebSocketClientMock()
        let useCase = DefaultSendEmojiUseCase(webSocketClient: mockWS)
        let vm = RealRoomViewModel()
        let appState = AppState()
        vm.bind(
            appState: appState,
            webSocketClient: mockWS,
            sendEmojiUseCase: useCase,
            emojiCatalogLoader: nil   // 跳过 catalog 校验 (与 nil-fallback 路径一致)
        )
        vm.currentUserId = "u1"   // direct mutation; bind 路径走 appState 由 18.2 测试覆盖

        vm.onEmojiSelected(code: "wave")

        // Step A: 本地立即入队 (同步完成, 不需 await)
        XCTAssertEqual(vm.activeEmojis.count, 1,
                       "本地动效必须立即入队 (Step A 同步)")
        XCTAssertEqual(vm.activeEmojis.first?.emojiCode, "wave")
        XCTAssertEqual(vm.activeEmojis.first?.userId, "u1")

        // Step C: Task 内 WS send (等 Task 跑完)
        try await Task.sleep(nanoseconds: 100_000_000)   // 100ms
        XCTAssertEqual(mockWS.sentMessages.count, 1,
                       "WS send 必须在 Task 内执行 1 次")
        guard case .emojiSend(_, let code) = mockWS.sentMessages.first else {
            XCTFail("Expected .emojiSend in mockWS.sentMessages, got \(String(describing: mockWS.sentMessages.first))")
            return
        }
        XCTAssertEqual(code, "wave")
    }

    // MARK: - case#3b edge (r2 fix-review P2): cross-room race —— catalog await 期间切 room → skip WS send

    /// Story 18.3 fix-review r2 [P2] —— V1 §12.2 emoji.send payload 不含 roomId,
    /// 必须 client 端守门: 入口 capture snapshotRoomId, catalog await 之后比对; 不等则 skip useCase.execute.
    ///
    /// 场景: 用户在 room A 选 emoji → Step A 本地动效入队 + Task 启动 + catalog await 挂起
    ///       → 期间 appState.setCurrentRoomId("room_B") (模拟用户离开 A 加入 B + WS 重连到 B)
    ///       → resume catalog → Step B.5 recheck 发现 roomId 已变 → skip WS send.
    ///
    /// 断言: GatedMockLoadEmojisUseCase 触发后 useCase.execute 调用次数仍为 0
    ///       (mockWS.sentMessages 也是 0; activeEmojis 仍 1 项不回滚).
    func test_realOnEmojiSelected_roomSwitchDuringCatalogAwait_skipsWSSend() async throws {
        let mockWS = WebSocketClientMock()
        let recordingUseCase = RecordingSendEmojiUseCase()
        let gatedLoader = GatedMockLoadEmojisUseCase(
            result: .success([EmojiConfig(code: "wave", name: "挥手", assetUrl: "https://example.test/wave.png", sortOrder: 1)])
        )
        let appState = AppState()
        appState.setCurrentRoomId("room_A")   // 选 emoji 时所在 room

        let vm = RealRoomViewModel()
        vm.bind(
            appState: appState,
            webSocketClient: mockWS,
            sendEmojiUseCase: recordingUseCase,
            emojiCatalogLoader: gatedLoader
        )
        vm.currentUserId = "u1"

        vm.onEmojiSelected(code: "wave")

        // Step A 同步入队 (验证基线; 与 case#3 相同断言).
        // **必须在 setCurrentRoomId 切 room 之前** assert —— r1 fix (subscribeRoomIdConnect 的 A→B 分支)
        // 会清空 activeEmojis (room-scoped state reset; lesson `2026-05-14-room-transient-state-must-reset-on-room-transition.md`).
        // 本测试关注的是 r2 fix (WS send skip) 不是 r1 fix (state reset), 所以本地动效断言放 room 切换前.
        XCTAssertEqual(vm.activeEmojis.count, 1,
                       "本地动效必须立即入队 (Step A 同步, 在 room 切换前)")

        // 等 Task 跑到 catalog await suspend 点 (gated loader 卡住).
        try await Task.sleep(nanoseconds: 50_000_000)   // 50ms
        let executeCountBeforeSwitch = await recordingUseCase.executeCount
        XCTAssertEqual(executeCountBeforeSwitch, 0,
                       "catalog await 挂起期间 useCase.execute 必不能跑")

        // 关键模拟: catalog await 挂起期间用户切到 room B (setCurrentRoomId 写 currentRoomId)
        // 注: r1 fix 会同步清空 activeEmojis (符合产品语义 —— A→B 不应残留 A 的 emoji).
        appState.setCurrentRoomId("room_B")

        // resume catalog → Task 继续: Step B.5 recheck 发现 currentRoomId != snapshotRoomId → skip
        await gatedLoader.resume()
        try await Task.sleep(nanoseconds: 100_000_000)   // 100ms 让 Task 完成 recheck + return

        // 验证 (r2 fix 核心断言): useCase.execute **没被调用** (recheck 命中 skip 分支)
        let executeCountAfter = await recordingUseCase.executeCount
        XCTAssertEqual(executeCountAfter, 0,
                       "cross-room race fix: catalog await 期间 roomId 切换后必跳过 WS send (snapshot=A → now=B)")
        XCTAssertEqual(mockWS.sentMessages.count, 0,
                       "WS sentMessages 也应为 0 (useCase 未跑则 WS frame 未发)")
    }

    // MARK: - case#3c edge (r2 fix-review P2): leave to nil during catalog await → skip WS send

    /// Story 18.3 fix-review r2 [P2] 镜像 case: 用户在 catalog await 期间**完全 leave room** (currentRoomId → nil),
    /// 同样必须 skip WS send —— 否则会发到旧 session 的 WS 上（如果还连着）.
    func test_realOnEmojiSelected_leaveRoomToNilDuringCatalogAwait_skipsWSSend() async throws {
        let mockWS = WebSocketClientMock()
        let recordingUseCase = RecordingSendEmojiUseCase()
        let gatedLoader = GatedMockLoadEmojisUseCase(
            result: .success([EmojiConfig(code: "wave", name: "挥手", assetUrl: "https://example.test/wave.png", sortOrder: 1)])
        )
        let appState = AppState()
        appState.setCurrentRoomId("room_A")

        let vm = RealRoomViewModel()
        vm.bind(
            appState: appState,
            webSocketClient: mockWS,
            sendEmojiUseCase: recordingUseCase,
            emojiCatalogLoader: gatedLoader
        )
        vm.currentUserId = "u1"

        vm.onEmojiSelected(code: "wave")
        XCTAssertEqual(vm.activeEmojis.count, 1,
                       "Step A 同步入队 (在 leave room 之前; r1 fix 会在 A→nil 分支清空)")

        try await Task.sleep(nanoseconds: 50_000_000)
        appState.setCurrentRoomId(nil)   // 完全 leave room (snapshot=A → now=nil); r1 fix 会清 activeEmojis

        await gatedLoader.resume()
        try await Task.sleep(nanoseconds: 100_000_000)

        let executeCount = await recordingUseCase.executeCount
        XCTAssertEqual(executeCount, 0,
                       "leave room (currentRoomId → nil) 期间 catalog await 完成后也必跳过 WS send")
        XCTAssertEqual(mockWS.sentMessages.count, 0)
    }

    // MARK: - case#4 edge: WS 失败 → activeEmojis 不回滚 + toast "网络不佳..."

    /// Story 18.3 AC5 弱网降级: WS send 失败 (.notConnected) → 本地动效仍播 (Step A 已完成不回滚)
    /// + ErrorPresenter 接收 transient toast.
    /// 验证 epics.md 行 2691 钦定文案 "网络不佳，对方可能看不到" 1:1 一致.
    func test_realOnEmojiSelected_wsFailureKeepsLocalAnimationAndShowsToast() async throws {
        let mockWS = WebSocketClientMock()
        mockWS.sendError = .notConnected
        let useCase = DefaultSendEmojiUseCase(webSocketClient: mockWS)
        let presenter = ErrorPresenter(toastDuration: 5.0)   // 5s 让 assert 时仍在 toast 态
        let vm = RealRoomViewModel()
        let appState = AppState()
        vm.bind(
            appState: appState,
            webSocketClient: mockWS,
            sendEmojiUseCase: useCase,
            emojiCatalogLoader: nil,
            errorPresenter: presenter
        )
        vm.currentUserId = "u1"

        vm.onEmojiSelected(code: "wave")

        // Step A: 本地动效仍入队 (不回滚)
        XCTAssertEqual(vm.activeEmojis.count, 1,
                       "WS 失败时本地动效仍必须保留 (不回滚)")
        XCTAssertEqual(vm.activeEmojis.first?.emojiCode, "wave")

        // Step C: 等 Task 跑完 + 验证 toast 出现
        try await Task.sleep(nanoseconds: 200_000_000)   // 200ms 让 Task catch + presenter.presentToast 完成
        if case .toast(let message) = presenter.current {
            XCTAssertEqual(message, "网络不佳，对方可能看不到",
                           "toast 文案必须与 epics.md 行 2691 钦定 1:1 一致")
        } else {
            XCTFail("Expected .toast presentation, got \(String(describing: presenter.current))")
        }
    }
}

// MARK: - Test-private mocks for r2 fix-review cross-room race coverage

/// GatedMockLoadEmojisUseCase: `execute()` 挂起在 continuation 上，直到外部调 `resume()` 才返结果.
///
/// 与 `LoadEmojisUseCaseTests` 内的 `GatedMockEmojiRepository` 同精神（lesson
/// `2026-05-14-actor-reentrancy-needs-inflight-task-for-single-flight.md`），
/// 但本 mock 是 UseCase 层 (`LoadEmojisUseCaseProtocol`)，让 vm 路径可以让 `catalog await` 卡住,
/// 测试在 await 挂起期间改 `appState.currentRoomId` 模拟 cross-room race.
private actor GatedMockLoadEmojisUseCase: LoadEmojisUseCaseProtocol {
    private let result: Result<[EmojiConfig], Error>
    private var continuation: CheckedContinuation<Void, Never>?

    init(result: Result<[EmojiConfig], Error>) {
        self.result = result
    }

    func resume() {
        continuation?.resume()
        continuation = nil
    }

    func execute() async throws -> [EmojiConfig] {
        await withCheckedContinuation { (cont: CheckedContinuation<Void, Never>) in
            self.continuation = cont
        }
        return try result.get()
    }
}

/// RecordingSendEmojiUseCase: 记录 `execute(emojiCode:)` 被调用次数 + 入参.
///
/// 用法: 验证 cross-room race fix 命中 skip 分支时 `executeCount == 0`. actor 让 `executeCount`
/// 读写跨任务安全 (`Sendable` 协议要求).
private actor RecordingSendEmojiUseCase: SendEmojiUseCaseProtocol {
    private(set) var executeCount: Int = 0
    private(set) var lastEmojiCode: String?

    func execute(emojiCode: String) async throws {
        executeCount += 1
        lastEmojiCode = emojiCode
    }
}
