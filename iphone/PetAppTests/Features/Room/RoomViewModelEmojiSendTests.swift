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
