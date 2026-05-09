// RealRoomViewModelTests.swift
// Story 12.1 AC7: RealRoomViewModel 升级版（WS-driven）单元测试.
//
// 测试基础设施约束（与 Story 37.8 / ADR-0002 §3.1 衔接）：
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）
//   - 不引 ViewInspector / SnapshotTesting
//   - 直接断言 RealRoomViewModel 的 @Published 字段 + WebSocketClientMock.emit(_:) 驱动 stream
//
// 测试 case 设计（与 sprint-change-proposal §5.1 锚定）：
//   - case#1 happy: appState.currentRoomId = "room_xxx" → roomId computed getter 返回同值
//   - case#2 happy: WS 推 room.snapshot 含 3 成员 → members.count == 3 + members[0].name 与 snapshot 一致
//   - case#3 happy: appState.currentRoomId nil ↔ non-nil 切换 → wsState 切 + members 清空
//   - case#4 edge: 推 unknown 消息 → members 不破坏（保持现有 3 成员）
//   - case#5 happy: webSocketClient = nil 路径 → wsState 永远保持 .disconnected
//
// AsyncStream consumer task 跑在 Task { @MainActor.run { ... } } 中；测试用 await Task.yield()
// 让事件循环跑一轮（与 RealHomeViewModel Story 37.7 round 4 lesson `published-derived-state-needs-publisher-subscription` 同精神）.

import XCTest
import Combine
@testable import PetApp

@MainActor
final class RealRoomViewModelTests: XCTestCase {

    // MARK: - case#1 happy: roomId computed getter

    /// 验证 `roomId` computed getter 直接派生自 `appState.currentRoomId`（AR21 字符串 ID 约定）.
    func testRoomIdGetterReadsFromAppState() {
        let appState = AppState()
        appState.setCurrentRoomId("room_1234567")
        let vm = RealRoomViewModel(appState: appState)

        XCTAssertEqual(vm.roomId, "room_1234567",
                       "roomId computed getter 应该直接返回 appState.currentRoomId")

        // 切换 → getter 立即反映（无本地副本）
        appState.setCurrentRoomId("room_abcdefg")
        XCTAssertEqual(vm.roomId, "room_abcdefg",
                       "appState.currentRoomId 切换后 roomId getter 必须同步反映新值")

        appState.setCurrentRoomId(nil)
        XCTAssertNil(vm.roomId, "appState.currentRoomId 置 nil 后 roomId 应为 nil")
    }

    // MARK: - case#2 happy: WS 推 room.snapshot → members 正确派生

    /// 验证 `applySnapshot` 路径：3 成员 snapshot → members.count == 3 + 字段映射正确.
    /// 涵盖 §12.3 client merge contract 最小路径：roster 集合裁剪 + nickname 非空覆盖 + **isHost 严格 false**
    /// （fix-review round 4 P2：snapshot path 不依赖位置启发式推断 host）.
    func testRoomSnapshotMessagePopulatesMembers() async {
        let appState = AppState.makeHydrated(currentRoomId: "room_1234567")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        // 等订阅 / consumer task 起动
        await Task.yield()
        await Task.yield()

        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_1234567", maxMembers: 4, memberCount: 3),
            members: [
                RoomSnapshotMember(userId: "u_alice", nickname: "小花",
                                   pet: RoomSnapshotPet(petId: "p_a", currentState: 1)),
                RoomSnapshotMember(userId: "u_bob", nickname: "Mocha",
                                   pet: RoomSnapshotPet(petId: "p_b", currentState: 1)),
                RoomSnapshotMember(userId: "u_charlie", nickname: "Latte",
                                   pet: nil),
            ]
        )

        mockWS.emit(.roomSnapshot(payload))

        // 让 Task consumer + MainActor.run 跑掉
        try? await waitForMembersCount(vm: vm, expected: 3)

        XCTAssertEqual(vm.members.count, 3, "snapshot 含 3 成员应当映射成 vm.members.count == 3")
        XCTAssertEqual(vm.members[0].id, "u_alice")
        XCTAssertEqual(vm.members[0].name, "小花", "snapshot 非空 nickname 应当直接覆盖")
        // fix-review round 4 P2：snapshot path 下所有 RoomMember.isHost 一律 false
        // （旧实装 `isHost = index == 0` 在房主已离开的合法 server state 下会错误标"队长"）.
        XCTAssertFalse(vm.members[0].isHost, "snapshot path 下 isHost 应严格 false（不依赖位置启发式）")
        XCTAssertEqual(vm.members[1].name, "Mocha")
        XCTAssertFalse(vm.members[1].isHost)
        XCTAssertEqual(vm.members[2].name, "Latte")
        XCTAssertFalse(vm.members[2].isHost)

        // memberPetStates 节点 4 阶段保持空 map（snapshot currentState 固定 1，不写入）.
        XCTAssertEqual(vm.memberPetStates, [:],
                       "节点 4 阶段 memberPetStates 应保持空 map（待 Epic 14 后真实驱动）")
    }

    // MARK: - case#3 happy: currentRoomId nil ↔ non-nil 切换驱动 wsState + 清空 members

    /// 验证 subscribeRoomIdConnect 关键路径：roomId nil → non-nil 切换时 wsState 切 .connected
    /// （webSocketClient ≠ nil 路径）；nil 切换时 wsState 切 .disconnected + members 清空.
    func testCurrentRoomIdSwitchTogglesWsStateAndClearsMembers() async {
        let appState = AppState()
        appState.setCurrentRoomId(nil)
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 初始：currentRoomId = nil → wsState .disconnected（subscribe sink 在订阅时同步 emit 当前值）
        XCTAssertEqual(vm.wsState, .disconnected, "初始 currentRoomId = nil → wsState 应为 .disconnected")

        // 进入房间：currentRoomId 切 non-nil → wsState .connected（webSocketClient ≠ nil 路径）
        appState.setCurrentRoomId("room_xxx")
        await Task.yield()
        XCTAssertEqual(vm.wsState, .connected,
                       "currentRoomId 切非空且 webSocketClient ≠ nil → wsState 应切 .connected")

        // 先注入 1 成员让后续清空有可观测信号
        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 1),
            members: [
                RoomSnapshotMember(userId: "u_solo", nickname: "Solo", pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(payload))
        try? await waitForMembersCount(vm: vm, expected: 1)
        XCTAssertEqual(vm.members.count, 1)

        // 离开房间：currentRoomId 置 nil → wsState .disconnected + members 清空
        appState.setCurrentRoomId(nil)
        await Task.yield()
        XCTAssertEqual(vm.wsState, .disconnected, "currentRoomId 置 nil → wsState 应切 .disconnected")
        XCTAssertEqual(vm.members, [], "currentRoomId 置 nil → members 应被清空")
        XCTAssertEqual(vm.memberPetStates, [:], "currentRoomId 置 nil → memberPetStates 应被清空")
        XCTAssertTrue(mockWS.didDisconnect, "currentRoomId 置 nil → webSocketClient.disconnect() 应被调")
    }

    // MARK: - case#4 edge: 推 unknown 消息 → members 不破坏 + stream 不被破坏

    /// 验证 unknown 消息走 fallback 不污染现有 members（stream 不被破坏）.
    func testUnknownMessageDoesNotCorruptMembers() async {
        let appState = AppState.makeHydrated(currentRoomId: "room_xxx")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 先推一个 3 成员 snapshot 让 vm.members 有可观测基线
        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 3),
            members: [
                RoomSnapshotMember(userId: "u1", nickname: "A", pet: nil),
                RoomSnapshotMember(userId: "u2", nickname: "B", pet: nil),
                RoomSnapshotMember(userId: "u3", nickname: "C", pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(payload))
        try? await waitForMembersCount(vm: vm, expected: 3)
        XCTAssertEqual(vm.members.count, 3)

        // 再推 unknown 消息
        mockWS.emit(.unknown(rawType: "garbage_type"))
        await Task.yield()
        await Task.yield()

        // members 不应被破坏
        XCTAssertEqual(vm.members.count, 3,
                       "unknown 消息不应清空 members（stream 走 fallback 不破坏现有数据）")
        XCTAssertEqual(vm.members[0].name, "A")
        XCTAssertEqual(vm.members[2].name, "C")

        // 后续仍可继续接收消息（stream 未被破坏）—— 推一个 pong 也应被 discard 而不破坏 members
        mockWS.emit(.pong(requestId: "req1"))
        await Task.yield()
        XCTAssertEqual(vm.members.count, 3, "pong 消息应被 discard，不影响 members")
    }

    // MARK: - case#5 happy: webSocketClient = nil 路径 → wsState 永远保持 .disconnected

    /// 验证半完成语义（AC4 关键决策 3）：webSocketClient = nil 时即使进入房间 wsState 也保持 .disconnected.
    func testWebSocketClientNilKeepsWsStateDisconnected() async {
        let appState = AppState()
        appState.setCurrentRoomId(nil)
        let vm = RealRoomViewModel(appState: appState, webSocketClient: nil)

        await Task.yield()
        await Task.yield()
        XCTAssertEqual(vm.wsState, .disconnected, "初始应为 .disconnected")

        // 进入房间但 webSocketClient = nil → wsState 仍保持 .disconnected（"半完成"语义）
        appState.setCurrentRoomId("room_xxx")
        await Task.yield()
        await Task.yield()
        XCTAssertEqual(vm.wsState, .disconnected,
                       "webSocketClient = nil 时无论 currentRoomId 是否非空 wsState 都应为 .disconnected（AC4 关键决策 3）")
    }

    // MARK: - case#6 fix-review round 1 P1: restored in-room session（appState.currentRoomId 已非 nil 时构造）

    /// 验证 fix-review round 1 P1#1：`/home` restored in-room session 路径下 ViewModel 在
    /// `appState.currentRoomId` 已经非 nil 时构造，wsState 必须切到 .connected（不能停在 .disconnected）.
    /// 旧实装用 `.dropFirst()` 抑制订阅时同步 emit → restored session 永远停 .disconnected.
    /// 新实装用 `lastObservedRoomId` 区分 (nil, A) connect 分支 + (nil, nil) no-op 分支.
    func testRestoredInRoomSessionTriggersConnect() async {
        // 模拟 AppState.applyHomeData 在 ViewModel 订阅前已写非 nil currentRoomId 的场景.
        let appState = AppState.makeHydrated(currentRoomId: "room_restored")
        let mockWS = WebSocketClientMock()

        // 关键：构造 ViewModel 时 appState.currentRoomId 已经是非 nil 值（restored session）.
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        // 让 sink 同步 emit + consumer task 起步.
        await Task.yield()
        await Task.yield()

        XCTAssertEqual(vm.wsState, .connected,
                       "restored in-room session（currentRoomId 已非 nil 时构造）必须把 wsState 切到 .connected；旧实装 dropFirst 会让此值永远停在 .disconnected")
        XCTAssertEqual(vm.roomId, "room_restored")

        // 验证 stream consumer 确实活跃：emit snapshot 应当能路由到 vm.members.
        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_restored", maxMembers: 4, memberCount: 1),
            members: [
                RoomSnapshotMember(userId: "u_solo", nickname: "Solo", pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(payload))
        try? await waitForMembersCount(vm: vm, expected: 1)
        XCTAssertEqual(vm.members.count, 1, "restored session 路径 stream consumer 应当活跃接收消息")
        XCTAssertEqual(vm.members[0].name, "Solo")
    }

    // MARK: - case#7 fix-review round 1 P1: room A → room B 直接切换重置 roster

    /// 验证 fix-review round 1 P1#2：用户从 room A 直接切到 room B（中间不经 nil）时
    /// 必须清空 members / memberPetStates + tear down 旧 stream + wsState 保持 .connected.
    /// 旧实装只切 wsState（保持 .connected）但保留旧 roster + 旧 stream → room B 渲染 room A 的成员.
    func testDirectRoomToRoomSwitchResetsRosterAndStream() async {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. 进入 room A
        appState.setCurrentRoomId("room_A")
        await Task.yield()

        // 2. room A 推 snapshot 含 2 成员，建立 roster baseline
        let payloadA = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_A", maxMembers: 4, memberCount: 2),
            members: [
                RoomSnapshotMember(userId: "u_alice_A", nickname: "AliceA", pet: nil),
                RoomSnapshotMember(userId: "u_bob_A",   nickname: "BobA",   pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(payloadA))
        try? await waitForMembersCount(vm: vm, expected: 2)
        XCTAssertEqual(vm.members.count, 2)
        XCTAssertEqual(vm.members[0].name, "AliceA")
        XCTAssertFalse(mockWS.didDisconnect, "room A 阶段 disconnect 不应被调")

        // 3. 直接切到 room B（不先置 nil）—— 这是 review P1#2 担心的路径.
        appState.setCurrentRoomId("room_B")
        // 让 sink + cancel/restart stream 跑掉.
        await Task.yield()
        await Task.yield()

        // 4. 验证 A→B 切换语义：
        //    - members / memberPetStates 必须清空（不能让 room B 渲染 room A 的 roster）
        //    - 旧 stream 已被 tear down（mockWS.didDisconnect == true）
        //    - wsState 保持 .connected（room B 概念上仍连着；Story 12.2 后真实重连）
        XCTAssertEqual(vm.members, [],
                       "A→B 直接切换必须清空 members（旧实装只切 wsState 不清 roster → room B 渲染 room A 成员）")
        XCTAssertEqual(vm.memberPetStates, [:],
                       "A→B 直接切换必须清空 memberPetStates")
        XCTAssertTrue(mockWS.didDisconnect,
                      "A→B 直接切换必须 tear down 旧 stream（旧实装保留旧 stream → room A late messages 污染 room B）")
        XCTAssertEqual(vm.wsState, .connected,
                       "A→B 直接切换 wsState 应保持 .connected（仍在房间内；Story 12.2 后重连真实 socket）")
        XCTAssertEqual(vm.roomId, "room_B")

        // 5. fix-review round 2 P1 关键断言：A→B 切换必须调 `prepareForReconnect()`，
        //    否则旧 stream 被 disconnect finish 后，新 consumer task 接到的还是已 finish stream，
        //    后续 room B 的 `room.snapshot` 永远收不到 → UI 永远空房间.
        XCTAssertGreaterThanOrEqual(mockWS.prepareForReconnectCallCount, 1,
                                    "A→B 切换必须调 prepareForReconnect() 让 client 准备新 stream（否则 consumer 接已 finish stream → room B 永远收不到消息）")

        // 6. 关键回归断言：A→B 切换后向 client 推 room B 的 snapshot，vm.members 必须能更新
        //    （旧实装在已 finish 的 stream 上等永远收不到；本断言锁住 stream 重启可观测后果）.
        let payloadB = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_B", maxMembers: 4, memberCount: 1),
            members: [
                RoomSnapshotMember(userId: "u_charlie_B", nickname: "CharlieB", pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(payloadB))
        try? await waitForMembersCount(vm: vm, expected: 1)
        XCTAssertEqual(vm.members.count, 1,
                       "A→B 切换后 room B 的 snapshot 必须能驱动 vm.members（旧实装 stream 已 finish → 永远收不到 → 永远 0 成员）")
        XCTAssertEqual(vm.members[0].name, "CharlieB",
                       "A→B 切换后 vm.members 应反映 room B 的成员")
    }

    // MARK: - case#8 fix-review round 2 P1: leave-rejoin（A → nil → A'）路径必须能再收消息

    /// 验证 fix-review round 2 P1 的另一面：用户在 room A 后离开（→ nil）再进 room A'，
    /// 同一 webSocketClient 实例被复用 → A→nil 已 finish stream → 必须能再次收到 room A' 消息.
    /// 旧实装 nil→A 分支不调 `prepareForReconnect()` → A' 的 stream 永远停在已 finish 状态 → 永远收不到消息.
    func testLeaveRejoinReusesSameClientAndReceivesMessages() async {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. 进入 room A
        appState.setCurrentRoomId("room_A")
        await Task.yield()
        let payloadA = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_A", maxMembers: 4, memberCount: 1),
            members: [RoomSnapshotMember(userId: "u_a", nickname: "A", pet: nil)]
        )
        mockWS.emit(.roomSnapshot(payloadA))
        try? await waitForMembersCount(vm: vm, expected: 1)
        XCTAssertEqual(vm.members.count, 1)

        // 2. 离开 room A → nil（disconnect 被调 + stream 被 finish）
        appState.setCurrentRoomId(nil)
        await Task.yield()
        XCTAssertTrue(mockWS.didDisconnect)
        XCTAssertEqual(vm.wsState, .disconnected)

        // 3. 重新进入 room A'（复用同一 mockWS 实例）
        appState.setCurrentRoomId("room_Aprime")
        await Task.yield()
        XCTAssertEqual(vm.wsState, .connected,
                       "leave-rejoin 后 wsState 应恢复 .connected")
        XCTAssertGreaterThanOrEqual(mockWS.prepareForReconnectCallCount, 1,
                                    "leave-rejoin 路径 nil→A' 分支必须调 prepareForReconnect() 重置 stream")

        // 4. 关键断言：room A' 的 snapshot 必须能驱动 vm.members（旧实装：stream 已 finish → 永远 0）
        let payloadAprime = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_Aprime", maxMembers: 4, memberCount: 1),
            members: [RoomSnapshotMember(userId: "u_aprime", nickname: "APrime", pet: nil)]
        )
        mockWS.emit(.roomSnapshot(payloadAprime))
        try? await waitForMembersCount(vm: vm, expected: 1)
        XCTAssertEqual(vm.members.count, 1,
                       "leave-rejoin 后必须能再收到新 room 的 snapshot（旧实装 stream 永远 finish → 永远收不到）")
        XCTAssertEqual(vm.members[0].name, "APrime")
    }

    // MARK: - case#9 fix-review round 2 P2: 空字符串 currentRoomId 与 HomeRoomDispatcher 对齐

    /// 验证 fix-review round 2 P2：`""` currentRoomId 与 `HomeRoomDispatcher.shouldShowRoom("") == true` 对齐.
    /// 即：`""` 应被本 vm 当 in-room 处理（走 connect 分支），而**不**走 disconnect/clear-members 分支.
    /// 否则 UI 渲染 RoomScaffoldView（dispatcher 把 "" 当 in-room）但 vm 走 disconnect → 状态机不一致.
    func testEmptyStringRoomIdTreatedAsInRoomAlignsWithDispatcher() async {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()
        XCTAssertEqual(vm.wsState, .disconnected, "初始 nil → .disconnected")

        // 设置 "" —— HomeRoomDispatcher.shouldShowRoom("") == true（HomeContainerViewTests:41 锚定）
        appState.setCurrentRoomId("")
        await Task.yield()
        await Task.yield()

        // 关键断言：vm 必须把 "" 当 in-room 处理（与 dispatcher 对齐），走 connect 分支 → wsState .connected.
        // 旧实装把 "" normalize 成 nil → 走 (nil, nil) no-op → wsState 永远 .disconnected → 与 UI 不一致.
        XCTAssertEqual(vm.wsState, .connected,
                       "currentRoomId == \"\" 应被当 in-room 处理（与 HomeRoomDispatcher.shouldShowRoom(\"\") == true 对齐），走 connect 分支")
        XCTAssertFalse(mockWS.didDisconnect,
                       "currentRoomId == \"\" 不应触发 disconnect（旧实装 normalize 成 nil 后会走 disconnect 分支）")
    }

    // MARK: - case#10 fix-review round 3 P1: stale snapshot for room A 不能污染 room B

    /// 验证 fix-review round 3 P1：A→B 切换瞬间，前一个 stream 上排队的 `room.snapshot` for room A
    /// 在 `currentRoomId` 已经变成 room B 之后被 deliver，必须**忽略**而非 repopulate `members`.
    ///
    /// 旧实装 `handle(message:)` 处理 `.roomSnapshot` 时无条件 `applySnapshot(_:)`：late snapshot for
    /// room A 会把已经清空的 `members` 写回 room A 的成员名单，UI 渲染 room B 但 roster 是 room A 的 → bug.
    ///
    /// 新实装：先校验 `payload.room.id == lastObservedRoomId`；不匹配则丢弃 + log debug.
    /// 用 `lastObservedRoomId` 而非现读 `roomId`（同一队列上 publisher 通知顺序通常已切；但 sink 内
    /// 字段写入比 computed getter 读取 appState 更稳定）.
    func testStaleSnapshotForOldRoomDoesNotOverwriteCurrentRoster() async {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. 进入 room_A 并 baseline 成员
        appState.setCurrentRoomId("room_A")
        await Task.yield()
        let payloadA = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_A", maxMembers: 4, memberCount: 1),
            members: [RoomSnapshotMember(userId: "u_alice", nickname: "AliceA", pet: nil)]
        )
        mockWS.emit(.roomSnapshot(payloadA))
        try? await waitForMembersCount(vm: vm, expected: 1)
        XCTAssertEqual(vm.members.count, 1)
        XCTAssertEqual(vm.members[0].name, "AliceA")

        // 2. 直接切到 room_B（subscribeRoomIdConnect A→B 分支会清空 members + tear down 旧 stream
        //    + prepareForReconnect()）
        appState.setCurrentRoomId("room_B")
        await Task.yield()
        await Task.yield()
        XCTAssertEqual(vm.members, [], "A→B 切换瞬间 members 应被清空")
        XCTAssertEqual(vm.roomId, "room_B")

        // 3. 推一个 stale snapshot（room.id = "room_A"）—— 模拟前一个 stream 上排队后到 / 别处 race 路径.
        //    新 consumer task 是从 prepareForReconnect 后的新 stream 拿消息，所以这条要 emit 到新 stream.
        //    （prepareForReconnect 已 swap 过 stream；mock 的 emit 走最新 currentContinuation）.
        let stalePayload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_A", maxMembers: 4, memberCount: 1),
            members: [RoomSnapshotMember(userId: "u_ghost", nickname: "Ghost", pet: nil)]
        )
        mockWS.emit(.roomSnapshot(stalePayload))
        // 给 consumer task 充分时间处理（不能用 waitForMembersCount——预期就是不应被改）
        try? await Task.sleep(nanoseconds: 50_000_000) // 50ms
        await Task.yield()
        await Task.yield()

        // 4. 关键断言：stale room.id == "room_A" 不匹配当前 lastObservedRoomId == "room_B" → 应被丢弃,
        //    members 必须保持空（旧实装会写成 ["Ghost"]）.
        XCTAssertEqual(vm.members, [],
                       "stale snapshot for room A 必须被丢弃；旧实装会把 members 写成 [\"Ghost\"]（room B 的 UI 显示 room A 的 ghost 成员）")

        // 5. 反向断言：room B 的 fresh snapshot 仍能正常 apply（校验只挡 stale 不误伤当前房间）.
        let payloadB = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_B", maxMembers: 4, memberCount: 1),
            members: [RoomSnapshotMember(userId: "u_charlie", nickname: "CharlieB", pet: nil)]
        )
        mockWS.emit(.roomSnapshot(payloadB))
        try? await waitForMembersCount(vm: vm, expected: 1)
        XCTAssertEqual(vm.members.count, 1, "room B 的 fresh snapshot 必须能 apply（校验只挡 stale）")
        XCTAssertEqual(vm.members[0].name, "CharlieB")
    }

    /// 验证 fix-review round 3 P1 的离开场景：A→nil 后 stale snapshot for room A 仍可能投递,
    /// 此时 lastObservedRoomId == nil → 任何 snapshot 都视为 stale，不应 repopulate 已清空的 roster.
    func testStaleSnapshotAfterLeaveDoesNotRepopulateMembers() async {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. 进入 room_A + baseline 成员
        appState.setCurrentRoomId("room_A")
        await Task.yield()
        let payloadA = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_A", maxMembers: 4, memberCount: 1),
            members: [RoomSnapshotMember(userId: "u_alice", nickname: "AliceA", pet: nil)]
        )
        mockWS.emit(.roomSnapshot(payloadA))
        try? await waitForMembersCount(vm: vm, expected: 1)
        XCTAssertEqual(vm.members.count, 1)

        // 2. 离开（A → nil）：disconnect + members 清空 + lastObservedRoomId 现在是 nil
        appState.setCurrentRoomId(nil)
        await Task.yield()
        XCTAssertEqual(vm.members, [])
        XCTAssertEqual(vm.wsState, .disconnected)

        // 3. 此时排队的 stale snapshot 投递（A→nil 已 disconnect 把旧 stream finish；本测试模拟若有路径
        //    让消息从某个旁路进来——保守起见我们用同一 mockWS，但其 stream 已 finish；emit 会被 finish-stream
        //    drop，到不了 consumer。所以此 case 真正测的是：若**某条**路径让 stale snapshot 进入
        //    `handle(message:)`（如 round 4 后某 inline path / unit-test 直接调），守卫仍要挡住）.
        //
        //    本 case 用一种最简单的可验证路径：直接构造另一个 mockWS + 用 `bind` 注入。但 bind 的语义不同；
        //    所以更直接的方式：再起一个房间然后离开制造干净环境，但不必要——guard 的核心 invariant 是
        //    "lastObservedRoomId == nil 时任何 snapshot 都 stale"；用一个**新** consumer 起步前 emit 不可行
        //    （AsyncStream finish 后无法复活）.
        //
        //    保留本 case 作为 case#10 的语义文档：核心校验由 case#10 覆盖；本 case 留下"A→nil 后 members
        //    保持 []"的回归断言（防回归把 disconnect 分支里 members = [] 删掉）.
        XCTAssertEqual(vm.members, [], "A→nil 后 members 应保持空（且任何 stale snapshot 都不应让其复活）")
    }

    // MARK: - case#11 fix-review round 4 P2: snapshot path 下所有 isHost 严格 false（不依赖位置启发式）

    /// 验证 fix-review round 4 P2：snapshot path 下不论 N 个成员，RoomMember.isHost 全员应当为 false.
    ///
    /// 旧实装 `isHost = index == 0` 在合法 server state 下产生错误"队长"徽章：
    ///   - 房主离开后房间继续存在（协议钦定）→ 剩下的"第一个成员"被错误标 isHost
    ///   - 协议明文 client **不能**依赖 member 顺序 → 即使作为占位也不能用 index == 0 启发式
    ///
    /// 本 case 用一个 4 成员 snapshot（覆盖典型房间满员场景）回归：所有成员 isHost == false.
    /// 等后续 epic snapshot 真带 host 字段时，再单独写"snapshot 带 hostUserId → 该成员 isHost == true"的 case.
    ///
    /// 注：vm 自身的 `userIsHost`（"我是不是房主"，与 RoomMember.isHost 是两个独立字段）
    /// 由 RoomScaffoldDefaults.userIsHost 在 init 中 seed，applySnapshot **不**触碰它 ——
    /// 等真实 host 字段下发后由 vm 单独从 `appState.currentUserId == hostUserId` 派生.
    func testSnapshotPathDoesNotInferHostFromMemberOrder() async {
        let appState = AppState.makeHydrated(currentRoomId: "room_full")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 4 成员 snapshot（典型房间满员；含覆盖 index 0/1/2/3 全部位置）
        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_full", maxMembers: 4, memberCount: 4),
            members: [
                RoomSnapshotMember(userId: "u_first", nickname: "First", pet: nil),
                RoomSnapshotMember(userId: "u_second", nickname: "Second", pet: nil),
                RoomSnapshotMember(userId: "u_third", nickname: "Third", pet: nil),
                RoomSnapshotMember(userId: "u_fourth", nickname: "Fourth", pet: nil),
            ]
        )
        // 注意：vm 构造时会被 RoomScaffoldDefaults.members（u1/u2/u3/u4）seed —— 4 成员满员占位.
        // waitForMembersCount(expected: 4) 不能区分"种子默认 4 成员"与"snapshot 已 apply 后的 4 成员",
        // 直接用 waitForFirstMemberId 等待 members[0].id 切到 snapshot 第一项 u_first.
        mockWS.emit(.roomSnapshot(payload))
        try? await waitForFirstMemberId(vm: vm, expected: "u_first")

        // 关键回归断言：不论位置，所有成员 isHost == false（无位置启发式）
        XCTAssertEqual(vm.members.count, 4)
        XCTAssertEqual(vm.members.map { $0.id }, ["u_first", "u_second", "u_third", "u_fourth"],
                       "snapshot 必须已 apply（roster 替换为 snapshot 的 4 成员；旧种子 u1/u2/u3/u4 应被替换）")
        for (index, member) in vm.members.enumerated() {
            XCTAssertFalse(member.isHost,
                           "snapshot path 下成员 #\(index) (\(member.id)) 的 isHost 应为 false；"
                           + "旧实装 `isHost = index == 0` 在房主已离开的合法 server state 下会错误标"
                           + "\"队长\"徽章（协议钦定 client 不能依赖 member 顺序）")
        }

        // 反向断言：vm 自身的 userIsHost（独立字段）保留 RoomScaffoldDefaults seed，未被 applySnapshot 触碰.
        XCTAssertEqual(vm.userIsHost, RoomScaffoldDefaults.userIsHost,
                       "vm.userIsHost 应保留 init 时 seed 的 RoomScaffoldDefaults 占位值，"
                       + "applySnapshot 不应触碰它（host 字段下发后由 vm 单独派生）")
    }

    // MARK: - case#12 fix-review round 5 P2: bind() 替换 client instance 必须 disconnect 旧 client

    /// 验证 fix-review round 5 P2：vm 已 bound 且在房间中，再次调用 `bind(appState:webSocketClient:)` 传入
    /// **不同的** WebSocketClient instance 时，旧 client 必须收到 `disconnect()`，旧 messageConsumerTask
    /// 必须被 cancel —— 否则旧 socket 仍 subscribed → 资源泄漏 + 旧 stream 上的消息仍会被路由到 vm（duplicate traffic）.
    /// 同 instance 重 bind 必须 no-op（不能误调 disconnect 把好 client 关掉）.
    func testBindWithDifferentClientDisconnectsOldClient() async {
        let appState = AppState()
        let oldMockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: oldMockWS)

        await Task.yield()
        await Task.yield()

        // 1. 进入房间，让 vm 进入 bound + in-room 状态
        appState.setCurrentRoomId("room_xxx")
        await Task.yield()
        XCTAssertEqual(vm.wsState, .connected)
        XCTAssertFalse(oldMockWS.didDisconnect, "进入房间阶段不应触发 oldMockWS.disconnect()")

        // 2. baseline：oldMockWS 推 snapshot 让 members 有可观测值
        let oldPayload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 1),
            members: [RoomSnapshotMember(userId: "u_old", nickname: "OldRoom", pet: nil)]
        )
        oldMockWS.emit(.roomSnapshot(oldPayload))
        try? await waitForMembersCount(vm: vm, expected: 1)
        XCTAssertEqual(vm.members[0].name, "OldRoom")

        // 3. 关键：rebind 传入**不同的** WebSocketClient instance（vm 已 bound + 在房间内）
        let newMockWS = WebSocketClientMock()
        vm.bind(appState: appState, webSocketClient: newMockWS)
        await Task.yield()
        await Task.yield()

        // 4. 关键断言：旧 client 必须被 disconnect（避免 stream 仍 active deliver duplicate traffic）
        XCTAssertTrue(oldMockWS.didDisconnect,
                      "rebind 传入不同 client instance → 旧 client 必须收到 disconnect()（否则旧 socket 仍 subscribed → 资源泄漏 + duplicate traffic）")

        // 5. 反向验证：旧 client 推消息**不**应再被路由到 vm（task 已被 cancel）
        //    把 oldMockWS 上的 stream prepare 一下让 emit 不被 finish drop
        oldMockWS.prepareForReconnect()
        let staleOldPayload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 1),
            members: [RoomSnapshotMember(userId: "u_ghost_old", nickname: "GhostOld", pet: nil)]
        )
        oldMockWS.emit(.roomSnapshot(staleOldPayload))
        // 给一点时间防 stale task 复活
        try? await Task.sleep(nanoseconds: 50_000_000) // 50ms
        await Task.yield()
        await Task.yield()
        XCTAssertNotEqual(vm.members.first?.name, "GhostOld",
                          "旧 client 已被 disconnect + 旧 task 已 cancel → 旧 stream 消息不应再被处理")

        // 6. 反向验证：新 client 推消息**应当**被路由到 vm（新 task 应已起来 / 经下次 roomId 切换起）
        //    rebind 时 connectAlreadySubscribed=true + lastObservedRoomId != nil → bind 内主动起 task.
        let newPayload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 1),
            members: [RoomSnapshotMember(userId: "u_new", nickname: "NewClient", pet: nil)]
        )
        newMockWS.emit(.roomSnapshot(newPayload))
        try? await waitForMembersCount(vm: vm, expected: 1)
        // 等到 members 包含的 id 切到新 client 的 u_new
        let deadline = Date().addingTimeInterval(1.0)
        while Date() < deadline {
            if vm.members.first?.id == "u_new" { break }
            try? await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertEqual(vm.members.first?.id, "u_new",
                       "新 client 推的 snapshot 应被新 consumer task 路由到 vm.members")
    }

    /// 验证 fix-review round 5 P2 同 instance 路径：rebind 传**同一** client instance 时
    /// 不应误调 disconnect()（既无副作用也保留状态机）.
    func testBindWithSameClientInstanceIsNoop() async {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 进入房间
        appState.setCurrentRoomId("room_xxx")
        await Task.yield()
        XCTAssertEqual(vm.wsState, .connected)
        XCTAssertFalse(mockWS.didDisconnect)

        // 关键：rebind 传入**同一** instance
        vm.bind(appState: appState, webSocketClient: mockWS)
        await Task.yield()
        await Task.yield()

        // 不应误调 disconnect
        XCTAssertFalse(mockWS.didDisconnect,
                       "rebind 传同一 client instance → no-op（不应误调 disconnect 把好 client 关掉）")
        XCTAssertEqual(vm.wsState, .connected, "同 instance rebind 不应改变 wsState")
    }

    // MARK: - helpers

    /// 等待 vm.members.count 达到预期值（最多等 1 秒；防 Task consumer 调度时机不确定）.
    private func waitForMembersCount(vm: RealRoomViewModel, expected: Int) async throws {
        let deadline = Date().addingTimeInterval(1.0)
        while Date() < deadline {
            if vm.members.count == expected { return }
            try await Task.sleep(nanoseconds: 10_000_000) // 10ms
        }
    }

    /// 等待 vm.members 首个成员 id 切到预期值（用于 vm.members 已被 seed defaults 占位时,
    /// 区分"种子 4 成员"与"snapshot 已 apply 后的 4 成员"——单纯 count==4 无法区分）.
    private func waitForFirstMemberId(vm: RealRoomViewModel, expected: String) async throws {
        let deadline = Date().addingTimeInterval(1.0)
        while Date() < deadline {
            if vm.members.first?.id == expected { return }
            try await Task.sleep(nanoseconds: 10_000_000) // 10ms
        }
    }
}
