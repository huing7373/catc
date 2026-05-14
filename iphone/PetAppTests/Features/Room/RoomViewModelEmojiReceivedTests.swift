// RoomViewModelEmojiReceivedTests.swift
// Story 18.4 AC9 9b: applyEmojiReceived 链路单测 (Mock + Real path + 1.5s expire + roster miss + handle path + generation gate).
//
// 测试范围 (story file AC9 钦定, ≥6 case):
//   - happy MockRoomViewModel: 入队 activeEmojis + invocations 记录 .emojiReceived
//   - happy RealRoomViewModel self-broadcast 去重: currentUserId=self + payload.userId=self → 不入队
//   - happy RealRoomViewModel 1.5s expire: 入队 → sleep 1.7s → activeEmojis 空
//   - edge 同时 5 个不同 userId → activeEmojis.count == 5 + 5 个 UUID 不同
//   - edge RealRoomViewModel roster miss: payload.userId 不在 members → 仍入队 + (log info, 渲染层 center 降级)
//   - edge WS handler 路径: handle(.emojiReceived, streamRoomId: r1) → applyEmojiReceived → activeEmojis.count == 1
//
// 与既有 RoomViewModelEmojiSendTests / RealRoomViewModelTests 同精神 (@MainActor + @testable import PetApp + 直接断言 @Published).

import XCTest
@testable import PetApp

@MainActor
final class RoomViewModelEmojiReceivedTests: XCTestCase {

    // MARK: - case#1 happy: MockRoomViewModel.applyEmojiReceived → 入队 + invocations 记录

    /// Story 18.4 AC5 mock 实装: 入队 activeEmojis + invocations 记录 .emojiReceived(userId:code:).
    func test_mockApplyEmojiReceived_appendsActiveEmojiAndRecordsInvocation() {
        let vm = MockRoomViewModel()
        XCTAssertTrue(vm.activeEmojis.isEmpty, "初始 activeEmojis 必须为空")
        XCTAssertTrue(vm.invocations.isEmpty, "初始 invocations 必须为空")

        let payload = EmojiReceivedPayload(userId: "u_other", emojiCode: "wave")
        vm.applyEmojiReceived(payload)

        XCTAssertEqual(vm.activeEmojis.count, 1, "applyEmojiReceived 后 activeEmojis 必须追加 1 项")
        XCTAssertEqual(vm.activeEmojis.first?.userId, "u_other")
        XCTAssertEqual(vm.activeEmojis.first?.emojiCode, "wave")
        XCTAssertEqual(vm.invocations, [.emojiReceived(userId: "u_other", code: "wave")])
    }

    // MARK: - case#2 happy: RealRoomViewModel self-broadcast 去重

    /// Story 18.4 AC4 (a) self 去重: vm.currentUserId == payload.userId → 跳过 (本地动效已在 18.3 播过).
    func test_realApplyEmojiReceived_selfBroadcastIsSkipped() {
        let vm = RealRoomViewModel()
        vm.currentUserId = "u_self"

        let payload = EmojiReceivedPayload(userId: "u_self", emojiCode: "wave")
        vm.applyEmojiReceived(payload)

        XCTAssertTrue(vm.activeEmojis.isEmpty,
                      "self-broadcast 必须被去重 (V1 §12.3 行 2471 钦定); 18.3 本地动效已播过")
    }

    // MARK: - case#3 happy: RealRoomViewModel 1.5s 后自动 expire 移除

    /// Story 18.4 AC4 fire-and-forget 1.5s Task.sleep + removeAll {id == captured} (epics.md 行 2715-2716).
    /// sleep 1.7s 留 200ms 缓冲 (与 story file AC9 9b 钦定 timing 一致).
    func test_realApplyEmojiReceived_autoExpiresAfter15Seconds() async throws {
        let vm = RealRoomViewModel()
        vm.currentUserId = "u_self"

        let payload = EmojiReceivedPayload(userId: "u_other", emojiCode: "wave")
        vm.applyEmojiReceived(payload)
        XCTAssertEqual(vm.activeEmojis.count, 1, "入队后立即 activeEmojis.count == 1")

        try await Task.sleep(nanoseconds: 1_700_000_000)  // 1.7s 缓冲
        XCTAssertTrue(vm.activeEmojis.isEmpty,
                      "1.7s 后 activeEmojis 必须自动 expire 移除 (epics.md 行 2715 钦定)")
    }

    // MARK: - case#4 edge: 5 个不同 userId 同时入队 → 各自独立 entries

    /// Story 18.4 AC4 入队 UUID() 让 SwiftUI ForEach 区分; epics.md 行 2717 "同时多个表情飞出独立动效".
    func test_realApplyEmojiReceived_multipleDifferentUsersAppendIndependently() {
        let vm = RealRoomViewModel()
        vm.currentUserId = "u_self"

        let userIds = ["u_a", "u_b", "u_c", "u_d", "u_e"]
        for uid in userIds {
            vm.applyEmojiReceived(EmojiReceivedPayload(userId: uid, emojiCode: "wave"))
        }

        XCTAssertEqual(vm.activeEmojis.count, 5, "5 个不同 userId 必入队 5 项")
        let ids = vm.activeEmojis.map { $0.id }
        XCTAssertEqual(Set(ids).count, 5, "5 项 UUID 必两两不同 (UUID() 钦定)")
    }

    // MARK: - case#5 edge: RealRoomViewModel roster miss → 仍入队 (V1 §12.3 (c))

    /// Story 18.4 AC4 (c) roster miss: payload.userId 不在 vm.members → 仍入队 + log info "center 降级".
    /// (vm 层不丢弃; 渲染层 EmojiAnimationLayer 用 memberAnchors[userId] ?? centerAnchor 实现 center fallback).
    func test_realApplyEmojiReceived_rosterMissStillEnqueues() {
        let vm = RealRoomViewModel()
        vm.currentUserId = "u_self"
        vm.members = []  // 空 roster

        let payload = EmojiReceivedPayload(userId: "u_orphan", emojiCode: "wave")
        vm.applyEmojiReceived(payload)

        XCTAssertEqual(vm.activeEmojis.count, 1,
                       "V1 §12.3 行 2473 (c) roster miss 必须不丢弃 (sender leave + member.left 先到的合法 race)")
        XCTAssertEqual(vm.activeEmojis.first?.userId, "u_orphan")
        XCTAssertEqual(vm.activeEmojis.first?.emojiCode, "wave")
    }

    // MARK: - case#6 edge: WS handler 路径 → applyEmojiReceived

    /// Story 18.4 AC4 handle switch: handle(.emojiReceived, streamRoomId) → applyEmojiReceived → 入队.
    /// 直接调 vm.handle(...) (与 RealRoomViewModelTests cross-room race 测试同模式; handle 是 package-internal 函数).
    func test_realHandleEmojiReceived_routesToApplyAndEnqueues() {
        let vm = RealRoomViewModel()
        vm.currentUserId = "u_self"

        // bind appState + 触发 currentRoomId → lastObservedRoomId 同步 (sink 路径).
        let appState = AppState()
        vm.bind(appState: appState)
        appState.setCurrentRoomId("room_A")

        // streamRoomId == lastObservedRoomId == "room_A" → guard 通过 → applyEmojiReceived 调用.
        let payload = EmojiReceivedPayload(userId: "u_other", emojiCode: "love")
        vm.handle(message: .emojiReceived(payload), streamRoomId: "room_A")

        XCTAssertEqual(vm.activeEmojis.count, 1, "handle .emojiReceived 必走 applyEmojiReceived 入队")
        XCTAssertEqual(vm.activeEmojis.first?.userId, "u_other")
        XCTAssertEqual(vm.activeEmojis.first?.emojiCode, "love")
    }

    /// Story 18.4 AC4 cross-room race guard: streamRoomId != lastObservedRoomId → 丢弃, 不入队.
    /// 与 .petStateChanged / .memberJoined 同 guard 模式 (V1 §12.3 行 2473 (c) 是 vm 层 race; cross-room 是 stream 层 race).
    func test_realHandleEmojiReceived_crossRoomRaceDiscarded() {
        let vm = RealRoomViewModel()
        vm.currentUserId = "u_self"

        let appState = AppState()
        vm.bind(appState: appState)
        appState.setCurrentRoomId("room_B")  // current room 是 B

        // streamRoomId == "room_A" 但 lastObservedRoomId == "room_B" → guard 丢弃.
        let payload = EmojiReceivedPayload(userId: "u_other", emojiCode: "love")
        vm.handle(message: .emojiReceived(payload), streamRoomId: "room_A")

        XCTAssertTrue(vm.activeEmojis.isEmpty,
                      "cross-room stale event 必丢弃 (streamRoomId='room_A' ≠ lastObservedRoomId='room_B')")
    }
}
