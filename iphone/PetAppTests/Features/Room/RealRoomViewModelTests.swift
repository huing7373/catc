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

        // Story 15.1 AC1：memberPetStates 自本 story 起从 snapshot.pet.currentState 真实写入；
        // 旧 case 全 currentState=1（rest）+ 一个 pet=nil（pet-less）→ 全部映射为 .rest.
        XCTAssertEqual(vm.memberPetStates, [
            "u_alice": .rest,
            "u_bob": .rest,
            "u_charlie": .rest,  // pet=nil（pet-less）→ fallback .rest（Story 15.1 AC1 钦定）
        ], "Story 15.1 起 snapshot 写入 memberPetStates：currentState=1 → .rest，pet=nil 兜底 .rest")
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

    // MARK: - case#13 fix-review round 6 P2: same-instance rebind 必须 true no-op，consumer 不重启

    /// 验证 fix-review round 6 P2：vm 已 bound 且在房间中，`bind(appState:webSocketClient:)`
    /// 传入**同一** WebSocketClient instance 时，**不能** restart consumer task —— 否则
    /// cancel 当前 consumer + 在同一 AsyncStream 上 start new iterator（没调 prepareForReconnect）
    /// → in-flight `room.snapshot` 在 rebind 缝隙间被丢.
    ///
    /// 测试时序（关键）：
    ///   1. init vm + 进房间 → consumer 起在 mockWS.messages 上
    ///   2. 第一次 bind（same instance）
    ///   3. **在 rebind 后**立即 emit snapshot（模拟 in-flight 消息）
    ///   4. 断言 snapshot 被 vm 正确接收（members 包含 snapshot 内的 userId）
    ///
    /// 旧实装 bug：rebind 内 `else if webSocketClient != nil && lastObservedRoomId != nil`
    /// 无条件调 startConsumingMessages → cancel 当前 consumer 然后 start new iterator on same stream.
    /// AsyncStream 不支持多 iterator 重新订阅 —— 后起的 iterator 可能 miss buffered values.
    /// 修复：only call startConsumingMessages 当 client 实际 swap / first injection.
    func testSameInstanceRebindDoesNotDropInFlightSnapshot() async throws {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. 进入房间，让 consumer task 起在 mockWS.messages 上
        appState.setCurrentRoomId("room_xxx")
        await Task.yield()
        await Task.yield()
        XCTAssertEqual(vm.wsState, .connected)

        // 2. baseline: 推一个 snapshot 让 members 有可观测值（验证 consumer task 已活）
        let baselinePayload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 1),
            members: [RoomSnapshotMember(userId: "u_baseline", nickname: "Baseline", pet: nil)]
        )
        mockWS.emit(.roomSnapshot(baselinePayload))
        try await waitForMembersCount(vm: vm, expected: 1)
        XCTAssertEqual(vm.members.first?.id, "u_baseline",
                       "baseline: consumer task 必须能消费 snapshot")

        // 3. 关键：第一次 same-instance rebind（模拟 reappear / dependency refresh 场景）
        let prepareCountBeforeRebind = mockWS.prepareForReconnectCallCount
        vm.bind(appState: appState, webSocketClient: mockWS)
        await Task.yield()
        await Task.yield()

        // 关键断言 A: same-instance rebind 不应触发 prepareForReconnect（这是 swap 路径独有）
        XCTAssertEqual(mockWS.prepareForReconnectCallCount, prepareCountBeforeRebind,
                       "same-instance rebind 不应调 prepareForReconnect（这是 swap 路径独有的语义）")

        // 4. 关键测试：rebind 后立即 emit snapshot（模拟 rebind 过程中 server 推的 in-flight 消息）
        //    旧实装 bug 触发条件：bind 内 startConsumingMessages 在 rebind 期间 cancel 当前 consumer
        //    + 同一 AsyncStream 上 start new iterator → emit 落入 stream buffer 后被新 iterator miss.
        let inFlightPayload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 1),
            members: [RoomSnapshotMember(userId: "u_inflight", nickname: "InFlight", pet: nil)]
        )
        mockWS.emit(.roomSnapshot(inFlightPayload))

        // 5. 关键断言 B: vm 必须能消费到 in-flight snapshot（members.first.id 切到 u_inflight）
        try await waitForFirstMemberId(vm: vm, expected: "u_inflight")
        XCTAssertEqual(vm.members.first?.id, "u_inflight",
                       "same-instance rebind 后 in-flight snapshot 必须被原 consumer 接收（不应被 rebind 误 cancel + restart 路径丢失）")
    }

    // MARK: - case#14 fix-review round 6 P2: same-instance rebind 在两次 bind 之间 enqueue snapshot

    /// 更严格的 fix-review round 6 P2 回归：bind 同 instance **两次** + 在两次 bind 之间
    /// emit snapshot → 断言 vm 收到 snapshot.
    ///
    /// 与 case#13 区别：case#13 测的是「rebind 后 emit 不丢」；本 case 测的是「连续两次 same-instance
    /// bind 之间 emit 不丢」—— 模拟更激进的 rebind 抖动（如 SwiftUI environment 多次 publish）.
    func testRepeatedSameInstanceRebindPreservesInFlightSnapshot() async throws {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 进入房间
        appState.setCurrentRoomId("room_xxx")
        await Task.yield()
        await Task.yield()
        XCTAssertEqual(vm.wsState, .connected)

        // 第一次 same-instance rebind
        vm.bind(appState: appState, webSocketClient: mockWS)
        await Task.yield()

        // 在两次 bind 之间 enqueue snapshot
        let snapshotBetween = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 1),
            members: [RoomSnapshotMember(userId: "u_between", nickname: "Between", pet: nil)]
        )
        mockWS.emit(.roomSnapshot(snapshotBetween))

        // 第二次 same-instance rebind（模拟连续 rebind 抖动）
        vm.bind(appState: appState, webSocketClient: mockWS)
        await Task.yield()
        await Task.yield()

        // 断言：两次 rebind 之间 emit 的 snapshot 不应被丢
        try await waitForFirstMemberId(vm: vm, expected: "u_between")
        XCTAssertEqual(vm.members.first?.id, "u_between",
                       "两次 same-instance rebind 之间 emit 的 snapshot 必须保留（consumer 不应被 rebind 误 restart 丢失消息）")
        XCTAssertFalse(mockWS.didDisconnect,
                       "两次 same-instance rebind 都不应误调 disconnect")
    }

    // MARK: - Story 12.3 case#B happy: 同一 snapshot 推两次 → idempotent，members.count 仍 = 3

    /// Story 12.3 AC4 case#B: snapshot 是 idempotent；同 userId 集合 + 同字段值 → members 数组 stable
    /// （数量不变 + 字段不退化）.
    /// 关键覆盖：snapshot 重复推送（如握手对齐 / 重新拉 snapshot 路径）下 members 不退化、不变化、不重复 append.
    /// 与 Story 12.1 既有 testRoomSnapshotMessagePopulatesMembers 平级累加，不重写 / 不删除既有 case.
    func testRoomSnapshotIsIdempotentOnRepeatedEmit() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_1234567")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_1234567", maxMembers: 4, memberCount: 3),
            members: [
                RoomSnapshotMember(userId: "u_alice", nickname: "Alice",
                                   pet: RoomSnapshotPet(petId: "p_a", currentState: 1)),
                RoomSnapshotMember(userId: "u_bob", nickname: "Bob",
                                   pet: RoomSnapshotPet(petId: "p_b", currentState: 1)),
                RoomSnapshotMember(userId: "u_charlie", nickname: "Charlie", pet: nil),
            ]
        )

        // 第一次 emit
        mockWS.emit(.roomSnapshot(payload))
        try await waitForMembersCount(vm: vm, expected: 3)
        XCTAssertEqual(vm.members.count, 3, "first snapshot 应当映射成 3 成员")
        XCTAssertEqual(vm.members.map { $0.id }, ["u_alice", "u_bob", "u_charlie"])
        XCTAssertEqual(vm.members.map { $0.name }, ["Alice", "Bob", "Charlie"])

        // 第二次 emit 同 payload —— 关键：member 数量不变 + 字段值不退化
        mockWS.emit(.roomSnapshot(payload))
        // 给 consumer task 充分时间处理；count==3 是预期稳定值，不能用 waitForMembersCount 区分
        try? await Task.sleep(nanoseconds: 50_000_000) // 50ms
        await Task.yield()
        await Task.yield()

        XCTAssertEqual(vm.members.count, 3,
                       "重复 emit 同 snapshot 后 members.count 仍 == 3（snapshot 是 idempotent）")
        XCTAssertEqual(vm.members.map { $0.id }, ["u_alice", "u_bob", "u_charlie"],
                       "重复 emit 后 member id 顺序 / 集合不变")
        XCTAssertEqual(vm.members.map { $0.name }, ["Alice", "Bob", "Charlie"],
                       "重复 emit 后 nickname 字段不退化（不被空串覆盖；不被错误 wipe-out）")
        // Story 15.1 AC1：memberPetStates 自本 story 起真实写入；payload 全 currentState=1（rest）+
        // u_charlie pet=nil（pet-less）→ 全部映射 `.rest`. 重复 emit 同 payload 后值不退化、不重复.
        XCTAssertEqual(vm.memberPetStates, [
            "u_alice": .rest,
            "u_bob": .rest,
            "u_charlie": .rest,  // pet=nil → fallback .rest
        ], "Story 15.1 起 idempotent snapshot 路径下 memberPetStates 仍稳定")
    }

    // MARK: - Story 12.3 case#C happy: empty snapshot members → vm.members = []

    /// Story 12.3 AC4 case#C: snapshot members[] 为空数组（不是 nil；server 不可能下发，
    /// 但 contract layer 必须容忍）.
    /// 关键覆盖：先 emit 3 成员 baseline → 再 emit empty snapshot（同 roomId）→ members 数组本身为 [] 非 nil.
    /// 与 testCurrentRoomIdSwitchTogglesWsStateAndClearsMembers（A→nil 路径下清空）区别：
    ///   本 case 测的是 "in-room 状态下 server 下发 empty roster" 路径（applySnapshot 算法本身的空数组路径），
    ///   而非 leave-room transition 路径.
    func testEmptyRoomSnapshotClearsMembers() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_xxx")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. baseline 3 成员
        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 3),
            members: [
                RoomSnapshotMember(userId: "u1", nickname: "A", pet: nil),
                RoomSnapshotMember(userId: "u2", nickname: "B", pet: nil),
                RoomSnapshotMember(userId: "u3", nickname: "C", pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(payload))
        try await waitForMembersCount(vm: vm, expected: 3)
        XCTAssertEqual(vm.members.count, 3)

        // 2. emit empty snapshot（same roomId 保证 stale-discard guard 不挡）
        let emptyPayload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 0),
            members: []
        )
        mockWS.emit(.roomSnapshot(emptyPayload))
        try await waitForMembersCount(vm: vm, expected: 0)

        XCTAssertEqual(vm.members.count, 0,
                       "empty snapshot members[] 必须把 vm.members 清空（applySnapshot 空数组路径）")
        XCTAssertEqual(vm.members, [],
                       "vm.members 应该是空数组 [] 而非 nil")
    }

    // MARK: - Story 12.3 case#D edge: snapshot 解码失败 fallback (.unknown(rawType: "room.snapshot")) 不破坏 members

    /// Story 12.3 AC4 case#D: snapshot 解码失败（payload schema mismatch）→
    /// codec 兜底为 `.unknown(rawType: "room.snapshot")` → ViewModel 不破坏现有 members + log error.
    /// 关键覆盖：现实路径的 codec fallback 语义（与 Story 12.1 既有 case#4 testUnknownMessageDoesNotCorruptMembers
    /// 用 "garbage_type" 不同；本 case 显式用 server type "room.snapshot" + payload schema mismatch 路径）.
    /// 锚定：Story 12.2 WSMessageCodec.decode payload 解码失败时 fallback 为 .unknown(rawType: ...) 不破坏 stream.
    func testRoomSnapshotPayloadDecodeFailureFallbackDoesNotCorruptMembers() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_xxx")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. baseline 3 成员
        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 3),
            members: [
                RoomSnapshotMember(userId: "u1", nickname: "A", pet: nil),
                RoomSnapshotMember(userId: "u2", nickname: "B", pet: nil),
                RoomSnapshotMember(userId: "u3", nickname: "C", pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(payload))
        try await waitForMembersCount(vm: vm, expected: 3)
        XCTAssertEqual(vm.members.count, 3)

        // 2. 模拟 codec 解码 payload 失败 → fallback 为 .unknown(rawType: "room.snapshot")
        // 这是 WSMessageCodec.decode 在拿到 type="room.snapshot" 但 payload schema mismatch 时的兜底路径.
        mockWS.emit(.unknown(rawType: "room.snapshot"))
        try? await Task.sleep(nanoseconds: 50_000_000) // 50ms
        await Task.yield()
        await Task.yield()

        // 3. 关键断言：members 不破坏 + 字段值不退化
        XCTAssertEqual(vm.members.count, 3,
                       ".unknown(rawType: \"room.snapshot\") fallback 不应清空 members（codec 解码失败兜底，stream 不破坏）")
        XCTAssertEqual(vm.members.map { $0.id }, ["u1", "u2", "u3"],
                       "fallback 后 member id 集合不变")
        XCTAssertEqual(vm.members.map { $0.name }, ["A", "B", "C"],
                       "fallback 后 nickname 字段值不退化")

        // 4. 反向断言：stream 仍活，后续 fresh snapshot 仍能 apply
        let freshPayload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 1),
            members: [RoomSnapshotMember(userId: "u_fresh", nickname: "Fresh", pet: nil)]
        )
        mockWS.emit(.roomSnapshot(freshPayload))
        try await waitForMembersCount(vm: vm, expected: 1)
        XCTAssertEqual(vm.members.first?.name, "Fresh",
                       "fallback 不破坏 stream —— 后续 fresh snapshot 仍能 apply")
    }

    // MARK: - Story 12.3 case#F edge: nickname 空字符串保留 existing + pet null 直接覆盖（client merge contract 完整路径）

    /// Story 12.3 AC4 case#F: V1 §12.3 client merge contract 字段级 merge 完整路径守护回归.
    ///
    /// 覆盖两个独立但同时落地的契约：
    ///   - **nickname 空字符串**：保留 client 已有 RoomMember.name（"server 不知道"信号；不是 "请清空"指令）
    ///   - **pet null**：直接覆盖 client 已有值（authoritative pet-less 信号；本 story RoomMember 类型暂不持
    ///     pet 字段，因此本 case 仅断言不会 crash + members 数组依然 stable，与 RoomSnapshotMember 层 pet
    ///     字段 preserve 一致）
    ///
    /// 锚定：本 case 是节点 4 placeholder 阶段（Story 10.7）与真实阶段（Story 11.7）共同 going-forward
    /// 契约的最终守护，详见 V1 §12.3 "client merge contract" 段.
    func testRoomSnapshotPreservesExistingNicknameOnEmptyStringAndOverridesPetWithNull() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_xxx")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. 第一次 emit snapshot 含 1 成员（nickname: "Alice", pet: ≠null）
        let payloadV1 = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 1),
            members: [
                RoomSnapshotMember(
                    userId: "u_alice",
                    nickname: "Alice",
                    pet: RoomSnapshotPet(petId: "p_a", currentState: 1)
                ),
            ]
        )
        mockWS.emit(.roomSnapshot(payloadV1))
        try await waitForMembersCount(vm: vm, expected: 1)
        XCTAssertEqual(vm.members[0].name, "Alice",
                       "first snapshot 非空 nickname 应直接覆盖（authoritative）")

        // 2. 第二次 emit 同 userId 但 nickname 为空字符串 + pet 改为 null
        //    （placeholder 阶段语义：server 不知道这个值 → 保留 client 已有值；pet null 直接覆盖）
        let payloadV2 = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 1),
            members: [
                RoomSnapshotMember(userId: "u_alice", nickname: "", pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(payloadV2))
        try? await Task.sleep(nanoseconds: 50_000_000) // 50ms
        await Task.yield()
        await Task.yield()

        // 3. 关键断言：nickname 空字符串保留 existing.name == "Alice"（不被空串覆盖；不被降级 placeholder "成员"）
        XCTAssertEqual(vm.members.count, 1, "merge contract 路径 members 数量不变")
        XCTAssertEqual(vm.members[0].id, "u_alice", "userId 集合保持一致")
        XCTAssertEqual(vm.members[0].name, "Alice",
                       "nickname 空字符串必须保留 client 已有值（V1 §12.3 client merge contract: 空字符串 = \"server 不知道\"，不是 \"请清空\"）")

        // 4. 反向 case：新 userId（client 没有的 userId）+ nickname 空字符串
        //    → 首次出现降级为 "成员" placeholder（与 ui_design 占位一致；与 applySnapshot 内 mergedName 路径锁住）
        let payloadV3 = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 2),
            members: [
                // 保留已有 alice（带回 nickname "Alice" 也行；这里测的是新成员路径）
                RoomSnapshotMember(userId: "u_alice", nickname: "Alice",
                                   pet: RoomSnapshotPet(petId: "p_a", currentState: 1)),
                // 新成员，nickname 空字符串 → 应降级为 "成员" placeholder
                RoomSnapshotMember(userId: "u_new", nickname: "", pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(payloadV3))
        try await waitForMembersCount(vm: vm, expected: 2)
        XCTAssertEqual(vm.members.count, 2)
        XCTAssertEqual(vm.members[0].name, "Alice",
                       "alice 走非空 nickname 覆盖路径（authoritative）")
        XCTAssertEqual(vm.members[1].id, "u_new")
        XCTAssertEqual(vm.members[1].name, "成员",
                       "新 userId 首次出现 + nickname 空字符串应降级为 \"成员\" placeholder（applySnapshot mergedName 路径）")
    }

    // MARK: - Story 12.4 case#G1 happy: member.joined 增 1 个成员

    /// Story 12.4 AC4 case#G1（epic line 2134）：收到 member.joined → ViewModel.members 多 1 个.
    /// 关键覆盖：
    ///   - append 而非 prepend（vm.members.last == 新加入成员）
    ///   - name 来自 payload.nickname
    ///   - isHost 严格 false（fix-review r4 lesson 同精神）
    ///   - level=8 / status="在玩耍" 节点 4 阶段占位（与 applySnapshot 一致）
    func testMemberJoinedAppendsOneMember() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_xxx")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. baseline 2 成员
        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 2),
            members: [
                RoomSnapshotMember(userId: "u_alice", nickname: "Alice", pet: nil),
                RoomSnapshotMember(userId: "u_bob", nickname: "Bob", pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(payload))
        try await waitForMembersCount(vm: vm, expected: 2)
        XCTAssertEqual(vm.members.count, 2)

        // 2. emit member.joined（新 userId u_charlie + 完整 payload）
        let joined = MemberJoinedPayload(
            userId: "u_charlie",
            nickname: "Charlie",
            avatarUrl: "https://example.com/charlie.png",
            pet: MemberJoinedPet(petId: "p_c", currentState: 1)
        )
        mockWS.emit(.memberJoined(joined))
        try await waitForMembersCount(vm: vm, expected: 3)

        // 3. 关键断言
        XCTAssertEqual(vm.members.count, 3, "member.joined 后应增 1 个成员")
        XCTAssertEqual(vm.members.last?.id, "u_charlie", "新成员应 append 到末尾（不是 prepend）")
        XCTAssertEqual(vm.members.last?.name, "Charlie", "新成员 name 应来自 payload.nickname")
        XCTAssertEqual(vm.members.last?.isHost, false, "applyMemberJoined 必须 isHost 严格 false（fix-review r4 lesson 同精神）")
        XCTAssertEqual(vm.members.last?.level, 8, "节点 4 阶段 level 占位 8（与 applySnapshot 一致）")
        XCTAssertEqual(vm.members.last?.status, "在玩耍", "节点 4 阶段 status 占位（与 applySnapshot 一致）")
    }

    // MARK: - Story 12.4 case#G2 happy: member.left 减 1 个成员

    /// Story 12.4 AC4 case#G2（epic line 2135）：收到 member.left → ViewModel.members 少 1 个.
    /// 关键覆盖：
    ///   - 中间成员被 remove（不是只能 remove last）
    ///   - 其他成员 entry 字段不退化
    func testMemberLeftRemovesOneMember() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_xxx")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. baseline 3 成员
        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 3),
            members: [
                RoomSnapshotMember(userId: "u_alice", nickname: "Alice", pet: nil),
                RoomSnapshotMember(userId: "u_bob", nickname: "Bob", pet: nil),
                RoomSnapshotMember(userId: "u_charlie", nickname: "Charlie", pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(payload))
        try await waitForMembersCount(vm: vm, expected: 3)

        // 2. emit member.left（移除中间成员 u_bob）
        mockWS.emit(.memberLeft(MemberLeftPayload(userId: "u_bob")))
        try await waitForMembersCount(vm: vm, expected: 2)

        // 3. 关键断言
        XCTAssertEqual(vm.members.count, 2, "member.left 后应少 1 个成员")
        XCTAssertNil(vm.members.first(where: { $0.id == "u_bob" }), "u_bob 应被 remove")
        XCTAssertEqual(vm.members.map { $0.id }, ["u_alice", "u_charlie"], "其他 2 个成员 entry 顺序保留")
        XCTAssertEqual(vm.members.first(where: { $0.id == "u_alice" })?.name, "Alice", "其他成员 name 字段不退化")
        XCTAssertEqual(vm.members.first(where: { $0.id == "u_charlie" })?.name, "Charlie", "其他成员 name 字段不退化")
    }

    // MARK: - Story 12.4 case#G3 edge: member.joined dedup（已存在 userId → enrich + count 不变）

    /// Story 12.4 AC4 case#G3（epic line 2136）：收到 member.joined 但 userId 已存在 → 不重复添加 + log.
    /// 关键覆盖：
    ///   - dedup by userId：同一 userId 重复 emit 不重复 append（防"4 人房间显示 5 个成员"）
    ///   - 字段级 enrich：nickname 非空覆盖（"小花" → "新名字"）
    ///   - 其他成员未变化
    func testMemberJoinedDedupsExistingUserAndEnrichesFields() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_xxx")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. baseline 3 成员（含 u_alice 名字 "小花"）
        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 3),
            members: [
                RoomSnapshotMember(userId: "u_alice", nickname: "小花", pet: nil),
                RoomSnapshotMember(userId: "u_bob", nickname: "Bob", pet: nil),
                RoomSnapshotMember(userId: "u_charlie", nickname: "Charlie", pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(payload))
        try await waitForMembersCount(vm: vm, expected: 3)

        // 2. emit member.joined 复用同一 u_alice + 新 nickname "新名字"
        let joined = MemberJoinedPayload(
            userId: "u_alice",
            nickname: "新名字",
            avatarUrl: "",
            pet: MemberJoinedPet(petId: "p_a", currentState: 1)
        )
        mockWS.emit(.memberJoined(joined))

        // 等 enrich 路径生效：u_alice 的 name 切到 "新名字"
        let deadline = Date().addingTimeInterval(1.0)
        while Date() < deadline {
            if vm.members.first(where: { $0.id == "u_alice" })?.name == "新名字" { break }
            try? await Task.sleep(nanoseconds: 10_000_000)
        }

        // 3. 关键断言
        XCTAssertEqual(vm.members.count, 3, "member.joined 但 userId 已存在 → members.count 不变（dedup 防\"4 人房间显示 5 个成员\"）")
        XCTAssertEqual(vm.members.first(where: { $0.id == "u_alice" })?.name, "新名字",
                       "enrich 路径应字段级 merge：nickname 非空覆盖（\"小花\" → \"新名字\"）")
        XCTAssertEqual(vm.members.first(where: { $0.id == "u_alice" })?.isHost, false,
                       "enrich 路径 isHost 严格 false")
        XCTAssertEqual(vm.members.first(where: { $0.id == "u_bob" })?.name, "Bob", "其他成员未变化")
        XCTAssertEqual(vm.members.first(where: { $0.id == "u_charlie" })?.name, "Charlie", "其他成员未变化")
    }

    // MARK: - Story 12.4 case#G4 edge: member.left userId 不存在 → ignore + 不报错 + 不清空

    /// Story 12.4 AC4 case#G4（epic line 2137）：收到 member.left 但 userId 不存在 → 不报错 + log warning.
    /// 关键覆盖：
    ///   - 不抛 exception
    ///   - members.count 仍 == baseline（**不**清空 / **不**意外删除）
    ///   - 所有 baseline 成员 entry 全部保持
    func testMemberLeftIgnoresUnknownUserAndDoesNotClearRoster() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_xxx")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. baseline 3 成员
        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 3),
            members: [
                RoomSnapshotMember(userId: "u_alice", nickname: "Alice", pet: nil),
                RoomSnapshotMember(userId: "u_bob", nickname: "Bob", pet: nil),
                RoomSnapshotMember(userId: "u_charlie", nickname: "Charlie", pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(payload))
        try await waitForMembersCount(vm: vm, expected: 3)

        // 2. emit member.left 不存在的 userId u_unknown
        mockWS.emit(.memberLeft(MemberLeftPayload(userId: "u_unknown")))
        try? await Task.sleep(nanoseconds: 50_000_000) // 50ms 充分等
        await Task.yield()
        await Task.yield()

        // 3. 关键断言
        XCTAssertEqual(vm.members.count, 3, "member.left 不存在 userId → members.count 不变（**不**清空 / **不**意外删除）")
        XCTAssertEqual(vm.members.map { $0.id }, ["u_alice", "u_bob", "u_charlie"],
                       "全部 baseline 成员 entry 保持（**不**因找不到 entry 清空整个 roster）")
        XCTAssertEqual(vm.members.first(where: { $0.id == "u_alice" })?.name, "Alice", "baseline 成员字段不退化")
    }

    // MARK: - Story 12.4 case#G5 edge: 连续 join + leave 同一 user → members 数量正确

    /// Story 12.4 AC4 case#G5（epic line 2138）：连续 join + leave 同一 user → members 数量正确.
    /// 关键覆盖：joined+left 序列下 vm.members 增减一致.
    func testMemberJoinedThenLeftSameUserResultsInOriginalRoster() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_xxx")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. baseline 2 成员（u_alice / u_bob）
        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 2),
            members: [
                RoomSnapshotMember(userId: "u_alice", nickname: "Alice", pet: nil),
                RoomSnapshotMember(userId: "u_bob", nickname: "Bob", pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(payload))
        try await waitForMembersCount(vm: vm, expected: 2)

        // 2. emit member.joined u_charlie → 3 成员
        mockWS.emit(.memberJoined(MemberJoinedPayload(
            userId: "u_charlie", nickname: "Charlie", avatarUrl: "", pet: nil
        )))
        try await waitForMembersCount(vm: vm, expected: 3)
        XCTAssertEqual(vm.members.last?.id, "u_charlie", "joined 后 u_charlie append")

        // 3. emit member.left u_charlie → 2 成员
        mockWS.emit(.memberLeft(MemberLeftPayload(userId: "u_charlie")))
        try await waitForMembersCount(vm: vm, expected: 2)

        // 4. 关键断言：仅含 u_alice / u_bob（u_charlie 已 remove）
        XCTAssertEqual(vm.members.count, 2, "join+leave 同一 user 后 members 数量回到 baseline")
        XCTAssertEqual(vm.members.map { $0.id }, ["u_alice", "u_bob"], "u_charlie 已 remove")
        XCTAssertNil(vm.members.first(where: { $0.id == "u_charlie" }), "u_charlie 不在 roster")
    }

    // MARK: - Story 12.4 case#G6 edge: lastObservedRoomId == nil 时丢弃 stale member.joined

    /// Story 12.4 AC4 case#G6（推荐，覆盖 12.1 r3 lesson 延伸）：lastObservedRoomId == nil 时
    /// （已离开房间）收到 stale member.joined → 必须丢弃 + log debug，不应错误 append.
    /// 关键覆盖：12.1 r3 lesson `2026-05-09-stale-snapshot-discard-by-room-id-12-1-r3.md` 同精神延伸到 member.* 消息层.
    func testStaleMemberJoinedAfterLeaveDoesNotMutateRoster() async throws {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. 进入 room_xxx + baseline 2 成员
        appState.setCurrentRoomId("room_xxx")
        await Task.yield()
        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 2),
            members: [
                RoomSnapshotMember(userId: "u_alice", nickname: "Alice", pet: nil),
                RoomSnapshotMember(userId: "u_bob", nickname: "Bob", pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(payload))
        try await waitForMembersCount(vm: vm, expected: 2)

        // 2. 离开（A → nil；subscribeRoomIdConnect A→nil 分支会 disconnect + clear members）
        appState.setCurrentRoomId(nil)
        await Task.yield()
        XCTAssertEqual(vm.members.count, 0, "leave 后 members 应清空")

        // 3. emit stale member.joined（注意：旧 stream 已 finish；要先 prepareForReconnect 让 emit 不被 drop）
        mockWS.prepareForReconnect()
        mockWS.emit(.memberJoined(MemberJoinedPayload(
            userId: "u_late", nickname: "Late", avatarUrl: "", pet: nil
        )))
        try? await Task.sleep(nanoseconds: 50_000_000) // 50ms
        await Task.yield()
        await Task.yield()

        // 4. 关键断言：lastObservedRoomId == nil 时 stale member.joined 应被丢弃
        //    （即使有路径让 message 投递到 vm —— 旧 stream finish 已是一道防线，handle 内 guard 是第二道）
        XCTAssertEqual(vm.members.count, 0,
                       "lastObservedRoomId == nil（已离开房间）时收到的 member.joined 必须被丢弃；"
                       + "旧实装无 guard 会错误 append → vm.members.count == 1（即使 UI 已不在房间）")
    }

    // MARK: - Story 12.4 case#G6.5 fix-review round 2 P1: cross-room race（A→B 切换时旧 stream member.* 被丢）

    /// Story 12.4 fix-review round 2 P1：用户 A→B 切换路径下，旧 consumer task 在 cancel 前
    /// 已 dequeue 但还没投递到 main actor 的 room A `member.joined` / `member.left` 事件，
    /// 必须**不**被 apply 到 room B 的 members.
    ///
    /// V1 §12.3 钦定 `member.joined` / `member.left` payload 不含 room.id（仅 userId / nickname / pet 等），
    /// 无法做 per-event payload-level room.id 校验。修复策略：`startConsumingMessages` 在启动 task
    /// 时捕获当时 `lastObservedRoomId` 作为局部 `streamRoomId`；`handle(message:streamRoomId:)` 校验
    /// `streamRoomId == lastObservedRoomId`，不匹配则丢弃.
    ///
    /// 测试构造说明：cross-room race 的真实端到端时序在 mock `disconnect()` 同步 `finish()` 旧 stream
    /// 的模型下不易构造（finish 后旧 task 立即退出 `for await`，pending message 只能在 finish 之前的
    /// 极短时间窗口被 dequeue，单测难以稳定复现）。最 robust 的回归是**直接调** vm 暴露的 internal
    /// `handle(message:streamRoomId:)` API，模拟"旧 task 持有旧 streamRoomId、handle 时 lastObservedRoomId
    /// 已切到新 room"的瞬间状态，断言 guard 正确丢弃 stale message.
    func testCrossRoomMemberJoinedFromOldStreamIsDiscarded() async throws {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. 进入 room_A + baseline 1 成员（让 lastObservedRoomId = "room_A"）
        appState.setCurrentRoomId("room_A")
        await Task.yield()
        let payloadA = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_A", maxMembers: 4, memberCount: 1),
            members: [RoomSnapshotMember(userId: "u_alice", nickname: "Alice", pet: nil)]
        )
        mockWS.emit(.roomSnapshot(payloadA))
        try await waitForMembersCount(vm: vm, expected: 1)

        // 2. 直接切到 room_B（subscribeRoomIdConnect A→B 分支会清空 members + restart consumer）
        appState.setCurrentRoomId("room_B")
        await Task.yield()
        await Task.yield()
        XCTAssertEqual(vm.members, [], "A→B 切换瞬间 members 应被清空")

        // 3. 模拟"旧 stream 上 dequeue 但还没 apply 的 room A late member.joined"投递：
        //    旧 task 启动时 streamRoomId 捕获的是 "room_A"，但此刻 lastObservedRoomId 已是 "room_B"。
        //    直接调 handle(streamRoomId: "room_A") 模拟该瞬间状态.
        let staleJoined = MemberJoinedPayload(
            userId: "u_ghost", nickname: "GhostFromA", avatarUrl: "", pet: nil
        )
        vm.handle(message: .memberJoined(staleJoined), streamRoomId: "room_A")

        // 4. 关键断言：streamRoomId="room_A" != lastObservedRoomId="room_B" → guard 丢弃，
        //    members 必须保持空（不被错误 append 一个 room A 的 ghost）.
        XCTAssertEqual(vm.members, [],
                       "cross-room race: 旧 stream 上 dequeue 的 room A member.joined 必须被丢弃；"
                       + "旧实装仅守护 lastObservedRoomId != nil，A→B 切换后 lastObservedRoomId=B、"
                       + "stale event 来自 room A 仍会被错误 apply → members.count == 1")

        // 5. 同样验证 member.left：旧实装会从（已清空的）room B members 找 u_alice 找不到，
        //    走 ignore 路径不破坏 —— 但若 room B 后续真有 u_alice，旧实装会错误 remove.
        //    这里用 mock 一个 room B 的成员让漏洞可观测.
        let payloadB = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_B", maxMembers: 4, memberCount: 1),
            members: [RoomSnapshotMember(userId: "u_alice", nickname: "AliceB", pet: nil)]
        )
        mockWS.emit(.roomSnapshot(payloadB))
        try await waitForMembersCount(vm: vm, expected: 1)
        XCTAssertEqual(vm.members.first?.id, "u_alice")
        XCTAssertEqual(vm.members.first?.name, "AliceB", "room B 的 fresh snapshot 仍能 apply")

        // 6. 模拟旧 stream 投递的 room A `member.left u_alice`（u_alice 在 room A 离开了）：
        //    streamRoomId="room_A" != lastObservedRoomId="room_B" → 守护应丢弃，不 mutate room B 的 u_alice.
        vm.handle(message: .memberLeft(MemberLeftPayload(userId: "u_alice")), streamRoomId: "room_A")

        XCTAssertEqual(vm.members.count, 1,
                       "cross-room race: 旧 stream 上 room A 的 member.left u_alice 必须被丢弃；"
                       + "旧实装无 streamRoomId 守护会错误 remove room B 的 u_alice → members.count == 0")
        XCTAssertEqual(vm.members.first?.id, "u_alice", "room B 的 u_alice 应保留")
        XCTAssertEqual(vm.members.first?.name, "AliceB", "room B 的 u_alice 字段应保留")
    }

    /// fix-review round 2 P1 配套：streamRoomId 与 lastObservedRoomId 都为 nil 的边界
    /// （已离开房间起的 task；不应发生但防御性兜底）—— guard 第一段 `streamRoomId != nil` 同样挡住.
    /// 注意：vm 构造后 members 初值是 RoomScaffoldDefaults seed（4 个占位）；本 case 在进入 room 后再
    /// 离开（A→nil 分支会 clear members）让 baseline 落到空数组，再调 handle 验证 stale 不破坏空 baseline.
    func testHandleWithBothStreamRoomIdAndLastObservedRoomIdNilDiscardsMemberMessages() async throws {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. 进 room A → 离开 → vm.members = []，vm.lastObservedRoomId = nil
        appState.setCurrentRoomId("room_A")
        await Task.yield()
        appState.setCurrentRoomId(nil)
        await Task.yield()
        XCTAssertEqual(vm.members, [], "leave 后 members 清空")

        // 2. 直接调 handle(streamRoomId: nil) —— 模拟"任何 stream 都没起来时收到的 stray message".
        let joined = MemberJoinedPayload(userId: "u_x", nickname: "X", avatarUrl: "", pet: nil)
        vm.handle(message: .memberJoined(joined), streamRoomId: nil)
        XCTAssertEqual(vm.members, [], "streamRoomId=nil 时任何 member.joined 应被丢弃")

        vm.handle(message: .memberLeft(MemberLeftPayload(userId: "u_x")), streamRoomId: nil)
        XCTAssertEqual(vm.members, [], "streamRoomId=nil 时任何 member.left 应被丢弃")
    }

    // MARK: - Story 12.4 case#G7 edge: member.joined payload pet=null（pet-less 账号）

    /// Story 12.4 AC4 case#G7（推荐，契约 1 nullable pet 回归守护）：
    /// member.joined payload pet 为 null（pet-less 账号）→ codec / payload 层正常解析 +
    /// applyMemberJoined 不抛错 + members 正常 append.
    func testMemberJoinedWithNullPetAppendsNormally() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_xxx")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. baseline 2 成员
        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_xxx", maxMembers: 4, memberCount: 2),
            members: [
                RoomSnapshotMember(userId: "u_alice", nickname: "Alice", pet: nil),
                RoomSnapshotMember(userId: "u_bob", nickname: "Bob", pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(payload))
        try await waitForMembersCount(vm: vm, expected: 2)

        // 2. emit member.joined pet=null（pet-less 账号）
        let joined = MemberJoinedPayload(
            userId: "u_petless",
            nickname: "无宠物用户",
            avatarUrl: "",
            pet: nil
        )
        mockWS.emit(.memberJoined(joined))
        try await waitForMembersCount(vm: vm, expected: 3)

        // 3. 关键断言：members 正常 append（节点 4 阶段 RoomMember 不持 pet 字段，所以
        //    pet=null 与 pet=non-null 在 vm.members 层无可见差异）
        XCTAssertEqual(vm.members.count, 3, "member.joined pet=null 仍应正常 append")
        XCTAssertEqual(vm.members.last?.id, "u_petless")
        XCTAssertEqual(vm.members.last?.name, "无宠物用户")
        XCTAssertEqual(vm.members.last?.isHost, false)
    }

    // MARK: - Story 12.4 fix-review r2 P2: WSMessageCodec required-field semantic validation

    /// fix-review r2 P2 baseline: 合法 member.joined payload 应正常 decode 为 .memberJoined.
    /// 与下面 empty-userId / empty-nickname reject case 配对，锚定"reject 路径不会误伤合法 payload".
    func testCodecMemberJoinedValidPayloadDecodesAsMemberJoined() {
        let json = """
        {
          "type": "member.joined",
          "requestId": "req_abc",
          "payload": {
            "userId": "u_alice",
            "nickname": "Alice",
            "avatarUrl": "https://cdn/x.png",
            "pet": null
          }
        }
        """
        let result = WSMessageCodec.decode(json)
        guard case .memberJoined(let payload) = result else {
            return XCTFail("合法 member.joined payload 应 decode 为 .memberJoined，实际: \(result)")
        }
        XCTAssertEqual(payload.userId, "u_alice")
        XCTAssertEqual(payload.nickname, "Alice")
    }

    /// fix-review r2 P2: server 推送 `userId == ""` 时 codec 必须 fallback 为 .unknown，
    /// 不应构造 MemberJoinedPayload 让 RealRoomViewModel.applyMemberJoined 用空 entry 污染 roster.
    func testCodecMemberJoinedEmptyUserIdFallsBackToUnknown() {
        let json = """
        {
          "type": "member.joined",
          "requestId": "req_x",
          "payload": {
            "userId": "",
            "nickname": "SomeName",
            "avatarUrl": "",
            "pet": null
          }
        }
        """
        let result = WSMessageCodec.decode(json)
        guard case .unknown(let rawType) = result else {
            return XCTFail("empty userId 必须 fallback 为 .unknown(rawType: \"member.joined\")，实际: \(result)")
        }
        XCTAssertEqual(rawType, "member.joined",
                       "rawType 必须是 \"member.joined\" 而非 \"\"（区分语义校验失败 vs envelope 解码失败）")
    }

    /// fix-review r2 P2: V1 §12.3 钦定 `nickname` 非空；server 推送 `nickname == ""` 时
    /// codec 必须 fallback —— 防 placeholder "成员" 占位漏到真实 join 路径.
    func testCodecMemberJoinedEmptyNicknameFallsBackToUnknown() {
        let json = """
        {
          "type": "member.joined",
          "requestId": "req_y",
          "payload": {
            "userId": "u_bob",
            "nickname": "",
            "avatarUrl": "",
            "pet": null
          }
        }
        """
        let result = WSMessageCodec.decode(json)
        guard case .unknown(let rawType) = result else {
            return XCTFail("empty nickname 必须 fallback 为 .unknown(rawType: \"member.joined\")，实际: \(result)")
        }
        XCTAssertEqual(rawType, "member.joined")
    }

    /// fix-review r2 P2: member.left 同精神校验 `userId.isEmpty` —— 即便 ViewModel ignore
    /// 路径在 userId 不匹配时不破坏，codec 层先 fallback 更稳（防未来 ViewModel 实装变更踩坑）.
    func testCodecMemberLeftEmptyUserIdFallsBackToUnknown() {
        let json = """
        {
          "type": "member.left",
          "requestId": "req_z",
          "payload": {
            "userId": ""
          }
        }
        """
        let result = WSMessageCodec.decode(json)
        guard case .unknown(let rawType) = result else {
            return XCTFail("empty userId 必须 fallback 为 .unknown(rawType: \"member.left\")，实际: \(result)")
        }
        XCTAssertEqual(rawType, "member.left")
    }

    // MARK: - Story 12.5 case#H1-H5：connectionStateChanged → wsState 三态映射

    /// case#H1 happy: emitConnectionState(.reconnecting(attempt:)) → vm.wsState = .reconnecting.
    func testConnectionStateChangedReconnectingMapsToWsStateReconnecting() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_X")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // connect 路径下 vm.wsState 默认应为 .connected（webSocketClient ≠ nil + currentRoomId 非 nil）
        XCTAssertEqual(vm.wsState, .connected, "in-room + non-nil client → wsState 默认 .connected")

        mockWS.emitConnectionState(.reconnecting(attempt: 1))
        try await waitForWsState(vm: vm, expected: .reconnecting)
        XCTAssertEqual(vm.wsState, .reconnecting,
                       "connectionStateChanged(.reconnecting) → wsState 切 .reconnecting")
    }

    /// case#H2 happy: .disconnected → .connected 三态切换路径.
    func testConnectionStateChangedThreeStateMapping() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_X")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // .reconnecting → .reconnecting
        mockWS.emitConnectionState(.reconnecting(attempt: 2))
        try await waitForWsState(vm: vm, expected: .reconnecting)

        // .disconnected
        mockWS.emitConnectionState(.disconnected)
        try await waitForWsState(vm: vm, expected: .disconnected)

        // .connected
        mockWS.emitConnectionState(.connected)
        try await waitForWsState(vm: vm, expected: .connected)
    }

    /// case#H3 happy: .disconnected → vm.wsState = .disconnected.
    /// 与 case#H2 第一阶段重叠但单独一 case 覆盖不依赖前置状态.
    func testConnectionStateChangedDisconnectedMapsToWsStateDisconnected() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_X")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        mockWS.emitConnectionState(.disconnected)
        try await waitForWsState(vm: vm, expected: .disconnected)
        XCTAssertEqual(vm.wsState, .disconnected)
    }

    /// case#H4 happy: reconnect 后 server 推 room.snapshot → vm 自动 applySnapshot 对齐.
    /// 关键回归 acceptance 行 2153 "重连成功 → 立即重新拉 snapshot 对齐"语义：
    /// vm 不需特殊处理，server 自动重发 snapshot，vm 通过 applySnapshot 自动对齐 roster.
    func testReconnectFollowedBySnapshotRealignsRoster() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_R")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. baseline 2 成员
        let initial = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_R", maxMembers: 4, memberCount: 2),
            members: [
                RoomSnapshotMember(userId: "u_alice", nickname: "Alice", pet: nil),
                RoomSnapshotMember(userId: "u_bob", nickname: "Bob", pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(initial))
        try await waitForMembersCount(vm: vm, expected: 2)

        // 2. 触发重连状态变化
        mockWS.emitConnectionState(.reconnecting(attempt: 1))
        try await waitForWsState(vm: vm, expected: .reconnecting)

        // 3. reconnect 成功 + server 自动重发 snapshot（含新成员）
        mockWS.emitConnectionState(.connected)
        try await waitForWsState(vm: vm, expected: .connected)
        let updated = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_R", maxMembers: 4, memberCount: 3),
            members: [
                RoomSnapshotMember(userId: "u_alice", nickname: "Alice", pet: nil),
                RoomSnapshotMember(userId: "u_bob", nickname: "Bob", pet: nil),
                RoomSnapshotMember(userId: "u_charlie", nickname: "Charlie", pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(updated))
        try await waitForMembersCount(vm: vm, expected: 3)

        XCTAssertEqual(vm.members.count, 3, "reconnect 后 snapshot 自动对齐 roster")
        XCTAssertEqual(vm.members.last?.id, "u_charlie", "新加入成员应在 roster 中")
    }

    /// case#H5 fix-review round 2 P2: connectionStateChanged **必须**受 streamRoomId 守护.
    ///
    /// **推翻** dev 阶段开放问题 §6 "不守护" 决定（fix-review round 2 P2）：
    /// connection state 事件也"绑定 specific socket / stream"，跨 stream 投递的 .reconnecting /
    /// .disconnected 在 lastObservedRoomId 已变更后被 apply 会覆盖当前 room 的 status banner
    /// （显示前一个连接的 stale 状态）→ 与 .memberJoined / .memberLeft 同精神必须守护.
    ///
    /// 校验方式：直接调 vm.handle(message: .connectionStateChanged, streamRoomId: <旧值>) 模拟
    /// 旧 stream 上的 stale 事件投递；wsState 不应被改变.
    func testConnectionStateChangedGuardedByStreamRoomId() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_B")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // baseline：vm 处于 in-room → wsState 默认 .connected
        XCTAssertEqual(vm.wsState, .connected, "in-room + non-nil client → wsState 默认 .connected")

        // 1. 模拟"旧 stream 上 dequeue 的 .disconnected"（streamRoomId="room_A"，lastObservedRoomId="room_B"）
        //    —— A→B 切换瞬间旧 consumer task 投递 stale .disconnected 事件
        vm.handle(
            message: .connectionStateChanged(.disconnected),
            streamRoomId: "room_A"
        )
        XCTAssertEqual(vm.wsState, .connected,
                       "stale streamRoomId 的 .disconnected 应被守护丢弃，wsState 保持当前 room 的 .connected")

        // 2. streamRoomId=nil（极端 stale，已离开房间起的 task）也应被守护丢弃
        vm.handle(
            message: .connectionStateChanged(.reconnecting(attempt: 3)),
            streamRoomId: nil
        )
        XCTAssertEqual(vm.wsState, .connected,
                       "streamRoomId=nil 时 connectionStateChanged 应被守护丢弃（stream 不在任何房间）")

        // 3. streamRoomId 与当前 room 匹配 → 守护通过 → wsState 正常更新
        vm.handle(
            message: .connectionStateChanged(.reconnecting(attempt: 1)),
            streamRoomId: "room_B"
        )
        XCTAssertEqual(vm.wsState, .reconnecting,
                       "streamRoomId 与 lastObservedRoomId 匹配 → 守护通过 → wsState 正常更新")
    }

    /// case#H6 fix-review round 2 P2: A→B 切换路径下旧 stream 的 .reconnecting 不应污染 room B 的 wsState.
    /// 端到端语义验证（与 H5 单元级互补）：vm 在 in-room 状态切到另一房间后，旧 stream 上的 stale
    /// connection state 事件不能覆盖新 room 的 status banner.
    func testStaleConnectionStateFromOldRoomDoesNotPollute() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_A")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()
        XCTAssertEqual(vm.wsState, .connected, "room_A in-room → 默认 .connected")

        // A→B 切换（vm 内部会 cancel 旧 consumer + restart 新 task with streamRoomId="room_B"）
        appState.setCurrentRoomId("room_B")
        await Task.yield()
        await Task.yield()

        // 模拟"旧 stream 仍 enqueue 一个 .reconnecting"（streamRoomId="room_A" 的旧 task 投递）
        vm.handle(
            message: .connectionStateChanged(.reconnecting(attempt: 2)),
            streamRoomId: "room_A"
        )
        XCTAssertNotEqual(vm.wsState, .reconnecting,
                          "stale .reconnecting from room_A stream 不应污染 room_B 的 wsState")
    }

    // MARK: - Story 15.1 AC3: snapshot pet.currentState 解析 + memberPetStates 真实写入

    /// case#A happy（Story 15.1 AC3 epic line 2382）：snapshot 含 3 成员 + currentState 1/2/3 →
    /// memberPetStates 写入正确（.rest / .walk / .run）.
    func testSnapshotPetCurrentStateOneTwoThreeMapsToRestWalkRun() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_15_1_A")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_15_1_A", maxMembers: 4, memberCount: 3),
            members: [
                RoomSnapshotMember(userId: "userId1", nickname: "Alice",
                                   pet: RoomSnapshotPet(petId: "p1", currentState: 1)),
                RoomSnapshotMember(userId: "userId2", nickname: "Bob",
                                   pet: RoomSnapshotPet(petId: "p2", currentState: 2)),
                RoomSnapshotMember(userId: "userId3", nickname: "Carol",
                                   pet: RoomSnapshotPet(petId: "p3", currentState: 3)),
            ]
        )
        mockWS.emit(.roomSnapshot(payload))
        try await waitForMembersCount(vm: vm, expected: 3)

        XCTAssertEqual(vm.memberPetStates["userId1"], .rest,
                       "currentState=1 → HomePetState.rest")
        XCTAssertEqual(vm.memberPetStates["userId2"], .walk,
                       "currentState=2 → HomePetState.walk")
        XCTAssertEqual(vm.memberPetStates["userId3"], .run,
                       "currentState=3 → HomePetState.run")
    }

    /// case#B edge（Story 15.1 AC3 epic line 2383）：snapshot 含未知 currentState 值（如 99）→
    /// 默认按 `.rest` 处理 + log warning（防御性兜底，不应发生但必须有路径覆盖）.
    func testSnapshotUnknownPetCurrentStateFallsBackToRest() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_15_1_B")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_15_1_B", maxMembers: 4, memberCount: 1),
            members: [
                RoomSnapshotMember(userId: "userId_unknown", nickname: "Unknown",
                                   pet: RoomSnapshotPet(petId: "p_x", currentState: 99)),
            ]
        )
        mockWS.emit(.roomSnapshot(payload))
        try await waitForMembersCount(vm: vm, expected: 1)

        XCTAssertEqual(vm.memberPetStates["userId_unknown"], .rest,
                       "未知 currentState 值（99）应 fallback `.rest`（Story 15.1 AC1 钦定防御性兜底）")
    }

    /// case#C happy（Story 15.1 AC3 epic line 2384）：snapshot 缺 pet 字段（pet-less 账号）→
    /// 默认 `.rest`（让 PetSpriteView 有默认渲染；AC1 推荐策略）.
    func testSnapshotPetlessMemberDefaultsToRest() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_15_1_C")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_15_1_C", maxMembers: 4, memberCount: 1),
            members: [
                RoomSnapshotMember(userId: "userId_petless", nickname: "PetLess", pet: nil),
            ]
        )
        mockWS.emit(.roomSnapshot(payload))
        try await waitForMembersCount(vm: vm, expected: 1)

        XCTAssertEqual(vm.memberPetStates["userId_petless"], .rest,
                       "pet=nil（pet-less 账号）应兜底 `.rest`（让 PetSpriteView 有默认渲染）")
    }

    /// case#D happy（Story 15.1 AC3 epic line 2385-2386）：同一房间多次刷新 snapshot →
    /// memberPetStates 正确同步更新（不是仅累加；状态变化时旧值被新值覆盖）.
    func testRepeatedSnapshotUpdatesMemberPetStates() async throws {
        let appState = AppState.makeHydrated(currentRoomId: "room_15_1_D")
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. 第一次 snapshot：2 成员 currentState=1/2
        let snap1 = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_15_1_D", maxMembers: 4, memberCount: 2),
            members: [
                RoomSnapshotMember(userId: "uA", nickname: "A",
                                   pet: RoomSnapshotPet(petId: "p_a", currentState: 1)),
                RoomSnapshotMember(userId: "uB", nickname: "B",
                                   pet: RoomSnapshotPet(petId: "p_b", currentState: 2)),
            ]
        )
        mockWS.emit(.roomSnapshot(snap1))
        try await waitForMembersCount(vm: vm, expected: 2)
        XCTAssertEqual(vm.memberPetStates["uA"], .rest)
        XCTAssertEqual(vm.memberPetStates["uB"], .walk)

        // 2. 第二次 snapshot：同 userId 但 currentState=2/3 → memberPetStates 必须同步更新
        let snap2 = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_15_1_D", maxMembers: 4, memberCount: 2),
            members: [
                RoomSnapshotMember(userId: "uA", nickname: "A",
                                   pet: RoomSnapshotPet(petId: "p_a", currentState: 2)),
                RoomSnapshotMember(userId: "uB", nickname: "B",
                                   pet: RoomSnapshotPet(petId: "p_b", currentState: 3)),
            ]
        )
        mockWS.emit(.roomSnapshot(snap2))
        // 第二次 snapshot 不改变 members.count，无法用 waitForMembersCount 区分；
        // 用循环+短 sleep 等 memberPetStates[uA] 切到新值（Story 15.1 整体替换语义）.
        let deadline = Date().addingTimeInterval(1.0)
        while Date() < deadline {
            if vm.memberPetStates["uA"] == .walk { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }

        XCTAssertEqual(vm.memberPetStates["uA"], .walk,
                       "snapshot 刷新后 memberPetStates[uA] 应从 .rest 更新为 .walk")
        XCTAssertEqual(vm.memberPetStates["uB"], .run,
                       "snapshot 刷新后 memberPetStates[uB] 应从 .walk 更新为 .run")
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

    /// Story 12.5：等 vm.wsState 切到预期值（同 waitForMembersCount 精神；防 Task consumer 调度时机不确定）.
    private func waitForWsState(vm: RealRoomViewModel, expected: WSState) async throws {
        let deadline = Date().addingTimeInterval(1.0)
        while Date() < deadline {
            if vm.wsState == expected { return }
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

    // MARK: - Story 12.7 AC6: onLeaveTap + connect 触发新增 case

    /// case#story12-7-1 happy: onLeaveTap → mock LeaveRoomUseCase.execute() 调一次.
    func testOnLeaveTapInvokesLeaveRoomUseCase() async throws {
        let appState = AppState()
        appState.setCurrentRoomId("3001")
        let mockWS = WebSocketClientMock()
        let mockLeave = MockLeaveRoomUseCaseRoom()
        mockLeave.onExecute = { @MainActor in
            appState.setCurrentRoomId(nil)
        }
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)
        vm.bind(
            appState: appState,
            webSocketClient: mockWS,
            leaveRoomUseCase: mockLeave,
            errorPresenter: presenter
        )

        await Task.yield()
        await Task.yield()

        vm.onLeaveTap()
        try? await waitForCallCountStory12_7(mock: mockLeave, method: "execute()", expected: 1)

        XCTAssertEqual(mockLeave.callCount(of: "execute()"), 1,
                       "onLeaveTap 必须调 LeaveRoomUseCase.execute()")
    }

    /// case#story12-7-2 happy: LeaveRoomUseCase 成功 → A → nil 分支触发 → wsState=.disconnected + members=[] + memberPetStates=[:]
    func testOnLeaveTapSuccessTriggersAToNilBranch() async throws {
        let appState = AppState()
        appState.setCurrentRoomId("3001")
        let mockWS = WebSocketClientMock()
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let mockLeave = MockLeaveRoomUseCaseRoom()
        mockLeave.onExecute = { @MainActor in
            appState.setCurrentRoomId(nil)
        }
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)
        vm.bind(
            appState: appState,
            webSocketClient: mockWS,
            leaveRoomUseCase: mockLeave,
            errorPresenter: presenter
        )

        await Task.yield()
        await Task.yield()

        // 先注入 1 成员让后续清空有可观测信号
        let payload = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "3001", maxMembers: 4, memberCount: 1),
            members: [RoomSnapshotMember(userId: "u_solo", nickname: "Solo", pet: nil)]
        )
        mockWS.emit(.roomSnapshot(payload))
        try await waitForMembersCount(vm: vm, expected: 1)

        vm.onLeaveTap()
        try await waitForWsState(vm: vm, expected: .disconnected)

        XCTAssertEqual(vm.wsState, .disconnected)
        XCTAssertEqual(vm.members, [], "成功路径 A → nil 分支应清空 members")
        XCTAssertEqual(vm.memberPetStates, [:])
        XCTAssertNil(appState.currentRoomId)
    }

    /// case#story12-7-3 edge: LeaveRoomUseCase throw 1009 → ErrorPresenter 收到 + appState 保留
    func testOnLeaveTap1009RoutesToErrorPresenterAndPreservesAppState() async throws {
        let appState = AppState()
        appState.setCurrentRoomId("3001")
        let mockWS = WebSocketClientMock()
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let mockLeave = MockLeaveRoomUseCaseRoom()
        mockLeave.executeStub = .failure(APIError.business(code: 1009, message: "服务繁忙", requestId: "req_x"))
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)
        vm.bind(
            appState: appState,
            webSocketClient: mockWS,
            leaveRoomUseCase: mockLeave,
            errorPresenter: presenter
        )

        await Task.yield()
        await Task.yield()

        vm.onLeaveTap()
        // 等 ErrorPresenter 接收到呈现态
        let deadline = Date().addingTimeInterval(1.0)
        while Date() < deadline {
            if presenter.current != nil { break }
            try? await Task.sleep(nanoseconds: 10_000_000)
        }

        XCTAssertNotNil(presenter.current,
                        "1009 路径必须把错误透传给 ErrorPresenter")
        XCTAssertEqual(appState.currentRoomId, "3001",
                       "1009 透传应保留 appState.currentRoomId（用户在 RoomView 内重试）")
    }

    /// case#story12-7-r5-P2 regression: leave A in-flight 期间用户切到 room B → A 的 useCase 抛 1009 →
    /// **不**应 present errorPresenter（不让 stale error overlay 弹到 room B 之上）.
    /// 与 lesson 2026-05-11-leave-room-thrown-error-also-needs-stale-guard.md 对应.
    func testOnLeaveTapStaleThrownErrorAfterRoomSwitchSkipsErrorPresenter() async throws {
        let appState = AppState()
        appState.setCurrentRoomId("room_A")
        let mockWS = WebSocketClientMock()
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let mockLeave = MockLeaveRoomUseCaseRoom()
        // mock 在 execute() 内**先**模拟用户切到 room_B，再抛 1009 模拟迟到的网络错误.
        // 这与生产场景对齐：HTTP request in-flight 期间 tab 切 + join B → A 的 leave 后抛 1009 → catch 块读 current 已是 B.
        mockLeave.onExecute = { @MainActor in
            appState.setCurrentRoomId("room_B")
        }
        mockLeave.executeStub = .failure(APIError.business(code: 1009, message: "服务繁忙", requestId: "req_stale"))
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)
        vm.bind(
            appState: appState,
            webSocketClient: mockWS,
            leaveRoomUseCase: mockLeave,
            errorPresenter: presenter
        )

        await Task.yield()
        await Task.yield()

        vm.onLeaveTap()
        // 等 useCase 调用完成（execute 被记一次后 catch block 已跑过）.
        try? await waitForCallCountStory12_7(mock: mockLeave, method: "execute()", expected: 1)
        // 多一个 runloop tick 让 catch block 内的 MainActor 调度落地.
        for _ in 0..<5 { await Task.yield() }

        XCTAssertNil(presenter.current,
                     "stale thrown error（appState 已切到 room_B）必须 skip errorPresenter —— 不让 stale overlay 弹到新房间之上")
        XCTAssertEqual(appState.currentRoomId, "room_B",
                       "appState.currentRoomId 应保留为新房间 room_B（不被 stale leave 失败回滚）")
    }

    // MARK: - Story 12.7 r14 [P1] fix: LeaveRoomUseCase 抛 RoomNavigationStaleError → ViewModel 触发 home refresh

    /// 验证 LeaveRoomUseCase 抛 RoomNavigationStaleError 时 ViewModel：
    ///   1) 不调 errorPresenter（silent skip）
    ///   2) 调一次 refreshHomeOnStaleNavigation closure（reconcile authoritative state）.
    func testOnLeaveTapCaughtStaleErrorTriggersHomeRefresh() async throws {
        let appState = AppState()
        appState.setCurrentRoomId("3001")
        let mockWS = WebSocketClientMock()
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let mockLeave = MockLeaveRoomUseCaseRoom()
        mockLeave.executeStub = .failure(RoomNavigationStaleError(source: .leaveRoom))
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        var refreshCallCount = 0
        let refresh: @MainActor @Sendable () -> Void = { refreshCallCount += 1 }

        vm.bind(
            appState: appState,
            webSocketClient: mockWS,
            leaveRoomUseCase: mockLeave,
            errorPresenter: presenter,
            refreshHomeOnStaleNavigation: refresh
        )

        await Task.yield()
        await Task.yield()

        vm.onLeaveTap()
        try? await waitForCallCountStory12_7(mock: mockLeave, method: "execute()", expected: 1)
        for _ in 0..<5 { await Task.yield() }

        XCTAssertEqual(refreshCallCount, 1,
                       "RoomNavigationStaleError 必须触发 home refresh 一次")
        XCTAssertNil(presenter.current,
                     "stale error 是 race 信号，不应走 errorPresenter")
    }

    /// happy path 不应触发 refresh（negative assertion）.
    func testOnLeaveTapHappyPathDoesNotTriggerHomeRefresh() async throws {
        let appState = AppState()
        appState.setCurrentRoomId("3001")
        let mockWS = WebSocketClientMock()
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let mockLeave = MockLeaveRoomUseCaseRoom()
        mockLeave.onExecute = { @MainActor in
            appState.setCurrentRoomId(nil)
        }
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        var refreshCallCount = 0
        let refresh: @MainActor @Sendable () -> Void = { refreshCallCount += 1 }

        vm.bind(
            appState: appState,
            webSocketClient: mockWS,
            leaveRoomUseCase: mockLeave,
            errorPresenter: presenter,
            refreshHomeOnStaleNavigation: refresh
        )

        await Task.yield()
        await Task.yield()

        vm.onLeaveTap()
        try? await waitForCallCountStory12_7(mock: mockLeave, method: "execute()", expected: 1)
        for _ in 0..<5 { await Task.yield() }

        XCTAssertEqual(refreshCallCount, 0,
                       "happy path 不应触发 home refresh（只有 stale error 才 refresh）")
    }

    /// case#story12-7-4 happy: subscribeRoomIdConnect nil → A → mock WebSocketClient.connect(roomId:) 被调用一次
    func testSubscribeRoomIdConnectNilToATriggersConnect() async {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        // 注意：必须 strong 持 vm —— 否则 vm 立即释放后 sink cancellable 也 dealloc，setCurrentRoomId 不再触发 sink.
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        appState.setCurrentRoomId("room_A")
        // 等 connect Task 跑完
        let deadline = Date().addingTimeInterval(1.0)
        while Date() < deadline {
            if !mockWS.connectCallArgs.isEmpty { break }
            try? await Task.sleep(nanoseconds: 10_000_000)
        }

        XCTAssertGreaterThanOrEqual(mockWS.connectCallArgs.count, 1,
                                    "nil → A 分支必须调 webSocketClient.connect(roomId:)")
        XCTAssertEqual(mockWS.connectCallArgs.first, "room_A")
        _ = vm  // 防止编译器优化
    }

    /// case#story12-7-5 happy: A → B → mock WebSocketClient.connect("B") 被调用 + 先 disconnect 旧 + members 清空
    func testSubscribeRoomIdConnectAToBDisconnectsAndConnects() async throws {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        appState.setCurrentRoomId("room_A")
        // 等 connect("room_A") 完成
        let deadline1 = Date().addingTimeInterval(1.0)
        while Date() < deadline1 {
            if mockWS.connectCallArgs.contains("room_A") { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }

        // baseline 1 个成员
        let payloadA = RoomSnapshotPayload(
            room: RoomSnapshotRoomInfo(id: "room_A", maxMembers: 4, memberCount: 1),
            members: [RoomSnapshotMember(userId: "u_a", nickname: "A", pet: nil)]
        )
        mockWS.emit(.roomSnapshot(payloadA))
        try await waitForMembersCount(vm: vm, expected: 1)

        // 直接切 B
        appState.setCurrentRoomId("room_B")
        // 等 connect("room_B") 完成
        let deadline2 = Date().addingTimeInterval(1.0)
        while Date() < deadline2 {
            if mockWS.connectCallArgs.contains("room_B") { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }

        XCTAssertTrue(mockWS.didDisconnect, "A → B 必须先 disconnect 旧 client")
        XCTAssertTrue(mockWS.connectCallArgs.contains("room_B"),
                      "A → B 必须调 connect(roomId: \"room_B\")，实际 connectCallArgs=\(mockWS.connectCallArgs)")
        XCTAssertEqual(vm.members, [], "A → B 切换必须清空 members")
    }

    private func waitForCallCountStory12_7(mock: MockBase, method: String, expected: Int) async throws {
        let deadline = Date().addingTimeInterval(1.0)
        while Date() < deadline {
            if mock.callCount(of: method) >= expected { return }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
    }

    // MARK: - case#story12-7-r1-P2 fix-review round 1: connect failure 必须 NOT 让 wsState 卡在 .connected

    /// 验证 fix-review round 1 P2#1：subscribeRoomIdConnect nil→A 分支当 client.connect(roomId:) 抛错
    /// （token 缺失 / URL 构造失败 / DNS 等 sync failure 路径）—— wsState **不能**留在 .connected.
    ///
    /// 旧实装 bug：sink 同步设置 `wsState = .connected` 后再 `try? await client.connect(roomId:)`
    /// 静默吞错 → 即使 connect 抛了 WSError.tokenMissing，UI 仍显示 connected 状态但实际无 socket.
    ///
    /// 新实装：先 await connect 成功 → wsState=.connected；失败 → wsState=.disconnected + present.
    func testNilToAConnectFailureKeepsWsStateDisconnected() async throws {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        // 关键：让 connect 抛 WSError.tokenMissing（模拟 token 空 / URL 构造失败 sync failure）
        mockWS.connectError = .tokenMissing
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 进入房间 → 触发 nil→A 分支 → spawn Task → await connect 抛错 → wsState 应保持 .disconnected
        appState.setCurrentRoomId("room_failconnect")

        // 等 connect Task 跑完（throw 路径也会立即返回；无需 wait stream）
        let deadline = Date().addingTimeInterval(1.0)
        while Date() < deadline {
            if !mockWS.connectCallArgs.isEmpty { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        // 再多 yield 几次让 catch 分支的 wsState=.disconnected 跑掉
        await Task.yield()
        await Task.yield()
        await Task.yield()

        XCTAssertGreaterThanOrEqual(mockWS.connectCallArgs.count, 1,
                                    "nil→A 分支必须调用 connect(roomId:)（即使会抛错）")
        // **关键断言**：sync connect failure 路径下 wsState 不能卡在 .connected
        XCTAssertNotEqual(vm.wsState, .connected,
                          "connect(roomId:) 抛错（如 WSError.tokenMissing）时 wsState **不能**保持 .connected"
                          + "（旧实装 bug：sync 设置 .connected + try? 吞错 → UI 卡在错误的 connected 状态）")
        XCTAssertEqual(vm.wsState, .disconnected,
                       "connect 失败后 wsState 必须切回 .disconnected 让 UI 展示真实状态")
    }

    // MARK: - case#story12-7-r3-P1 fix-review round 3: WS connect failure 必须 NOT 走 errorPresenter

    /// 验证 fix-review round 3 P1#1：nil→A 分支当 client.connect(roomId:) 抛错时，
    /// **不**应调 `errorPresenter.present(error)`.
    ///
    /// 背景（review 命中）：r1 的修复路径在 catch 内调 `presenter?.present(error)`. `WSError` 在
    /// `AppErrorMapper` 没特殊映射 → fallback 到全屏 `.retry` overlay → `errorPresentationHost`
    /// 禁用整个 app 的 hit-testing. transient WS 故障（server down / network flap / handshake
    /// 错误）变成 block 整个 UI 的 modal，而不是仅反映 room 的 disconnected/reconnecting 状态.
    ///
    /// 正确语义：WS connect failure 通过 `wsState=.disconnected` 让 RoomScaffoldView 的 wsStateLabel
    /// 展示给用户 + 重连状态机后续自动尝试 reconnect；**不**走全屏 retry overlay.
    /// `errorPresenter` 仅用于"用户主动操作"的同步错误（如 onLeaveTap LeaveRoomUseCase 1009 → 仍走）.
    ///
    /// 详见 docs/lessons/2026-05-11-ws-connect-failure-must-not-use-error-presenter.md.
    func testNilToAConnectFailureDoesNotInvokeErrorPresenter() async throws {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        mockWS.connectError = .tokenMissing
        let presenter = ErrorPresenter(toastDuration: 0.05)
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)
        // 显式注入 errorPresenter（与生产 RootView.bind 一致），但 WS connect failure 路径**不**应触达它.
        vm.bind(
            appState: appState,
            webSocketClient: mockWS,
            errorPresenter: presenter
        )

        await Task.yield()
        await Task.yield()

        // 进入房间 → 触发 nil→A 分支 → spawn Task → await connect 抛错 → wsState 应保持 .disconnected
        appState.setCurrentRoomId("room_failconnect_r3")

        let deadline = Date().addingTimeInterval(1.0)
        while Date() < deadline {
            if !mockWS.connectCallArgs.isEmpty { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        await Task.yield()
        await Task.yield()
        await Task.yield()

        XCTAssertGreaterThanOrEqual(mockWS.connectCallArgs.count, 1,
                                    "nil→A 分支必须调用 connect(roomId:)（即使会抛错）")
        XCTAssertEqual(vm.wsState, .disconnected,
                       "connect 失败后 wsState 必须切回 .disconnected（r1 修复，保留）")
        // **关键断言**（r3 P1#1）：errorPresenter **不**应被 WS connect failure 触发
        XCTAssertNil(presenter.current,
                     "WS connect failure 是后台 transient 状态，**禁止**走 errorPresenter 路径"
                     + "（防 regression：r1 误把 presenter.present(error) 加进 catch → AppErrorMapper "
                     + "fallback 到全屏 .retry overlay block 整个 app hit-testing）")
    }

    // MARK: - case#story12-7-r4-P2 fix-review round 4: stale connect failure 不能覆盖当前 room 的 wsState

    /// 验证 fix-review round 4 P2：用户在 connect(roomId:) 还 await 时切换房间（A→B），
    /// 旧 connect 因 disconnect()/prepareForReconnect() throw later —— 此 throw 是 stale
    /// room A 的失败，**不**应覆盖当前 room B 的 wsState（可能是 .connected 占位 / .reconnecting）.
    ///
    /// 旧实装 bug：catch 内无条件 `self?.wsState = .disconnected` → stale failure for room A
    /// 覆盖 current room B 的 .connected → RoomScaffoldView 显示错误的连接状态.
    ///
    /// 新实装：catch 内 guard `lastObservedRoomId == connectingRoomId` 守护，不匹配丢弃 + log.
    /// 与 Story 12.4 r1 P1 `streamRoomId` 守护同精神 —— stale event 不能覆盖当前 room state.
    func testStaleConnectFailureDoesNotOverwriteCurrentRoomWsState() async throws {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        // 关键：开启 gate，让 connect 调用 await 不立即返回，让我们能在 mid-flight 切换 room
        mockWS.connectShouldGate = true
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. 进入 room_A → 触发 nil→A 分支 spawn Task：appState/sink → connect(roomId: "room_A")
        //    本 connect 调用因 gate 而 await 卡住 —— vm 已 sync set wsState = .connected 占位.
        appState.setCurrentRoomId("room_A")

        // 等 connect 被调（gate 卡在 await）
        let deadline1 = Date().addingTimeInterval(1.0)
        while Date() < deadline1 {
            if !mockWS.connectCallArgs.isEmpty { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertEqual(mockWS.connectCallArgs, ["room_A"], "connect 应被调一次 with room_A")
        XCTAssertEqual(vm.wsState, .connected, "nil→A 占位 wsState 应为 .connected")

        // 2. 用户在 room_A connect mid-flight 切到 room_B —— sink 走 A→B 分支：
        //    disconnect(旧) + prepareForReconnect + 新 connect(roomId: "room_B")（也走 gate）.
        //    此时 lastObservedRoomId 已经是 "room_B"；占位 wsState 仍 .connected.
        appState.setCurrentRoomId("room_B")
        await Task.yield()
        await Task.yield()

        // 等 room_B 的 connect 被调
        let deadline2 = Date().addingTimeInterval(1.0)
        while Date() < deadline2 {
            if mockWS.connectCallArgs.count >= 2 { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertEqual(mockWS.connectCallArgs, ["room_A", "room_B"],
                       "A→B 切换后应有 room_A + room_B 两次 connect 调用")
        XCTAssertEqual(vm.wsState, .connected, "A→B 切换占位 wsState 仍应为 .connected")

        // 3. 释放 room_A 的 connect with throwing —— 模拟 disconnect 触发的 stale failure.
        //    旧实装：catch 无条件 `wsState = .disconnected` → 覆盖 room_B 的 .connected → bug.
        //    新实装：guard `lastObservedRoomId == "room_A"` 失败（current = "room_B"）→ 丢弃信号.
        mockWS.releaseConnect(at: 0, throwing: true)

        // 让 catch 跑掉
        for _ in 0..<10 {
            await Task.yield()
        }
        try await Task.sleep(nanoseconds: 50_000_000)
        for _ in 0..<10 {
            await Task.yield()
        }

        // **关键断言**：stale room_A connect 失败**不**应覆盖 room_B 的 wsState.
        XCTAssertEqual(vm.wsState, .connected,
                       "stale room_A connect failure 不能覆盖 current room_B 的 wsState（旧实装 bug：catch 无条件 set .disconnected 让 RoomScaffoldView 显示错误连接状态）")
        XCTAssertEqual(vm.roomId, "room_B", "vm.roomId 仍应是 room_B")

        // 4. 反向断言：room_B 的 connect 失败仍**应**正常更新 wsState（守护只挡 stale 不误伤当前 room）.
        mockWS.releaseConnect(at: 1, throwing: true)

        for _ in 0..<10 {
            await Task.yield()
        }
        try await Task.sleep(nanoseconds: 50_000_000)
        for _ in 0..<10 {
            await Task.yield()
        }

        XCTAssertEqual(vm.wsState, .disconnected,
                       "current room_B 的 connect 失败应正常切到 .disconnected（守护只挡 stale，不误伤）")
    }

    /// 验证 fix-review round 4 P2：用户在 connect mid-flight 直接离开房间（A→nil），
    /// 旧 connect throw later —— stale failure 不应在已离开房间时覆盖 wsState（lastObservedRoomId == nil）.
    /// 离开路径 A→nil 由 sink 直接 set wsState = .disconnected，stale connect failure 不应改 wsState（已经是 .disconnected 了就保持）.
    /// 本断言核心：捕获守护语义在 lastObservedRoomId == nil 时也成立（不抛错、不 race log）.
    func testStaleConnectFailureAfterLeaveDoesNotMutateWsState() async throws {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        mockWS.connectShouldGate = true
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        await Task.yield()
        await Task.yield()

        // 1. 进入 room_A → connect await 卡在 gate
        appState.setCurrentRoomId("room_A")
        let deadline = Date().addingTimeInterval(1.0)
        while Date() < deadline {
            if !mockWS.connectCallArgs.isEmpty { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertEqual(vm.wsState, .connected)

        // 2. 用户离开（A→nil）—— sink 走 A→nil 分支：disconnect + 清空 + wsState = .disconnected
        appState.setCurrentRoomId(nil)
        await Task.yield()
        await Task.yield()
        XCTAssertEqual(vm.wsState, .disconnected, "A→nil 离开应同步 set wsState = .disconnected")

        // 3. 释放 room_A 的 connect with throwing —— stale failure.
        //    守护：lastObservedRoomId (nil) != "room_A" → 丢弃，不改 wsState.
        mockWS.releaseConnect(at: 0, throwing: true)
        for _ in 0..<10 {
            await Task.yield()
        }
        try await Task.sleep(nanoseconds: 50_000_000)
        for _ in 0..<10 {
            await Task.yield()
        }

        XCTAssertEqual(vm.wsState, .disconnected,
                       "离开后 stale connect failure 不应再 mutate wsState（保持 .disconnected；守护层丢弃信号）")
        XCTAssertNil(vm.roomId, "vm.roomId 应仍为 nil")
    }

    // MARK: - case#story12-7-r9-P2 fix-review round 9: stale connect failure from replaced client 守护

    /// 验证 fix-review round 9 P2：bind swap WebSocketClient instance 但保持同 roomId 时,
    /// 旧 client 的 in-flight connect 因 disconnect() 而 throw later —— 此 throw 属于旧 client
    /// 的失败. 老实装仅 gate on `lastObservedRoomId == connectingRoomId`，同 roomId match → flip
    /// wsState 回 .disconnected，即使新 client 已 connect.
    ///
    /// 新实装：catch 内额外校验 `webSocketClient === connectingClient` —— mismatch 丢弃.
    /// 对应 lesson 2026-05-11-async-error-handler-must-stale-guard-room-id-and-client-identity-12-7-r9.md.
    func testStaleConnectFailureFromReplacedClientDoesNotOverwriteWsState() async throws {
        let appState = AppState()
        appState.setCurrentRoomId("room_A")   // 同 roomId 全程不变

        let clientA = WebSocketClientMock()
        clientA.connectShouldGate = true       // clientA 的 connect 卡在 gate，让我们能 mid-flight swap

        let vm = RealRoomViewModel(appState: appState, webSocketClient: clientA)

        // 等 connect(roomId: "room_A") 被调（clientA 卡在 gate）
        await Task.yield()
        await Task.yield()
        let deadline1 = Date().addingTimeInterval(1.0)
        while Date() < deadline1 {
            if !clientA.connectCallArgs.isEmpty { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertEqual(clientA.connectCallArgs, ["room_A"], "clientA.connect 应已被调（gate 卡住）")
        XCTAssertEqual(vm.wsState, .connected, "占位 wsState 应为 .connected")

        // bind swap：注入新 clientB（同 roomId 不变）.
        // bind 路径会：disconnect(clientA) → cancel 旧 consumer task → swap → prepareForReconnect(clientB)
        //   → startConsumingMessages → 因 clientChanged && lastObservedRoomId != nil → 起新 connect(clientB)
        let clientB = WebSocketClientMock()
        vm.bind(appState: appState, webSocketClient: clientB)

        // 等 clientB.connect 被调
        let deadline2 = Date().addingTimeInterval(1.0)
        while Date() < deadline2 {
            if !clientB.connectCallArgs.isEmpty { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertEqual(clientB.connectCallArgs, ["room_A"], "swap 后 clientB.connect 也应被调（同 roomId）")
        XCTAssertTrue(clientA.didDisconnect, "swap 后旧 clientA 应被 disconnect")
        XCTAssertEqual(vm.wsState, .connected, "swap 后占位 wsState 应保持 .connected")

        // 关键：释放 clientA 的 gate with throwing —— stale failure from replaced client.
        // 旧实装 bug：catch 仅 gate on lastObservedRoomId（== "room_A" 匹配）→ flip wsState .disconnected,
        //   即使新 clientB 已 connect.
        // 新实装：catch 额外 `webSocketClient === connectingClient`（clientA）失败（current 是 clientB）→ 丢弃.
        clientA.releaseConnect(at: 0, throwing: true)

        // 等 catch 路径跑掉
        for _ in 0..<10 {
            await Task.yield()
        }
        try await Task.sleep(nanoseconds: 80_000_000)
        for _ in 0..<10 {
            await Task.yield()
        }

        // 关键断言：stale clientA failure 不能覆盖当前 clientB 的 wsState.
        XCTAssertEqual(vm.wsState, .connected,
                       "stale connect failure from replaced clientA 不应覆盖 current clientB 的 wsState（旧实装仅 gate on roomId → 同 roomId match → flip 回 .disconnected）")
    }

    // MARK: - case#story12-7-r11-P2 fix-review round 11: nil→A must clear scaffold roster before connect

    /// 验证 fix-review round 11 P2：subscribeRoomIdConnect 的 `nil → A` 分支必须在 connect 之前
    /// **立即**清空 `members` / `memberPetStates`，与 `A → B` 分支 reset 行为对齐.
    ///
    /// 旧实装 bug：vm.init() 用 RoomScaffoldDefaults seed 了 4 个 mock 成员；当 currentRoomId 从 nil 切到 A
    /// 时 sink 走 nil→A 分支**不**清空 roster → UI 立即切到 RoomView 但 `members` 仍是 scaffold 4 个假成员，
    /// 直到第一个 `room.snapshot` 到达才被 applySnapshot 覆盖.
    ///
    /// 场景影响：
    ///   - 首个 snapshot delayed → 用户短暂看到 4 个假成员（accuracy regression）
    ///   - 若 `connect(roomId:)` 失败（token missing / server down / handshake error）→ snapshot 永远不到 →
    ///     假成员**永久残留**直到用户离开房间
    ///
    /// 新实装：sink 内进入 nil→A 分支后**立即**（在 `prepareForReconnect()` / connect Task 起之前）
    /// `members = []` + `memberPetStates = [:]`，与 A→B 分支同语义.
    ///
    /// 详见 docs/lessons/2026-05-11-room-entry-must-clear-scaffold-roster-before-connect.md.
    func testNilToAClearsScaffoldRosterBeforeConnect() async throws {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        // gate connect 让我们能在 connect 之前 / 期间断言 members 状态（snapshot 一定不会到）
        mockWS.connectShouldGate = true
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        // baseline: vm.members 应被 init() 用 RoomScaffoldDefaults 4 个假成员 seed
        XCTAssertEqual(vm.members.count, RoomScaffoldDefaults.members.count,
                       "vm.init() 应 seed RoomScaffoldDefaults.members 作为占位")
        XCTAssertEqual(vm.members.first?.id, "u1",
                       "baseline 首成员应是 RoomScaffoldDefaults seed 的 u1")

        // 提前往 memberPetStates 写一个 entry（模拟 Epic 14 后真实状态；本 story 不会写入，但防御性测试）
        // —— 让我们能验证 nil→A 分支同时清空了 memberPetStates.
        // 注：vm.memberPetStates 是 @Published 字段，可以从外部 mutate.
        vm.memberPetStates = ["seedKey": .rest]
        XCTAssertEqual(vm.memberPetStates.count, 1, "baseline memberPetStates 应有 1 条")

        await Task.yield()
        await Task.yield()

        // 触发 nil → A 切换
        appState.setCurrentRoomId("room_fresh")

        // 等 connect 被调（confirm sink 已跑完 nil→A 分支同步部分）
        let deadline = Date().addingTimeInterval(1.0)
        while Date() < deadline {
            if !mockWS.connectCallArgs.isEmpty { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertEqual(mockWS.connectCallArgs, ["room_fresh"],
                       "nil→A 分支必须调 connect(roomId:)（gate 卡在 await）")

        // **关键断言**：sink 同步路径已跑完（connect 调用证明此点），members / memberPetStates
        // 必须**立即**清空 —— **不**等 snapshot 到达，**不**等 connect 完成.
        XCTAssertEqual(vm.members, [],
                       "nil→A 分支必须立即清空 members（与 A→B 分支对齐）；"
                       + "否则 connect 失败时 RoomScaffoldDefaults seed 假成员会永久残留")
        XCTAssertEqual(vm.memberPetStates, [:],
                       "nil→A 分支必须立即清空 memberPetStates（与 A→B 分支对齐）")

        // 反向断言：占位 wsState 仍为 .connected（与既有 nil→A 语义一致；nil→A 同步 set .connected）
        XCTAssertEqual(vm.wsState, .connected,
                       "nil→A 同步 set wsState=.connected 不受 roster reset 影响")
        XCTAssertEqual(vm.roomId, "room_fresh",
                       "vm.roomId 派生自 appState.currentRoomId 仍为新房间")
    }

    /// 配套验证：上面 nil→A reset 测试的"connect 失败"等价路径 —— 即使 connect 抛错，
    /// roster 也应在 connect 调用前**已经**被 reset（不依赖 snapshot 到达）.
    /// 验证 review 命中的"connect 失败 → 假成员永久残留"场景被根治.
    func testNilToAClearsRosterEvenWhenConnectFails() async throws {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        // 让 connect 抛错（模拟 token missing / server down）—— snapshot 永远不会到达
        mockWS.connectError = .tokenMissing
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        // baseline: seed scaffold 4 成员
        XCTAssertEqual(vm.members.count, 4, "baseline members 应是 4 个 RoomScaffoldDefaults seed")

        await Task.yield()
        await Task.yield()

        // 触发 nil → A → connect 抛错
        appState.setCurrentRoomId("room_failconnect")

        // 等 connect 被调（即使抛错也会被记入 connectCallArgs）
        let deadline = Date().addingTimeInterval(1.0)
        while Date() < deadline {
            if !mockWS.connectCallArgs.isEmpty { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        // 多 yield 让 catch 路径跑完（wsState=.disconnected）
        for _ in 0..<10 {
            await Task.yield()
        }
        try await Task.sleep(nanoseconds: 50_000_000)
        for _ in 0..<10 {
            await Task.yield()
        }

        // **关键断言**：即使 connect 失败 → snapshot 永远不会到 → members 必须保持空，不能残留 scaffold seed.
        XCTAssertEqual(vm.members, [],
                       "connect 失败时 snapshot 永远不到达；nil→A 分支提前 reset roster 保证假成员不残留")
        XCTAssertEqual(vm.wsState, .disconnected,
                       "connect 抛错时 wsState 应被纠正为 .disconnected（fix-review round 1 P2 不回退）")
    }

    // MARK: - case#fix-review round 13 P1: nil-client 路径必须保留 scaffold roster

    /// **fix-review round 13 P1 回归 #1**：UITEST_SKIP_GUEST_LOGIN=1 + UITEST_FORCE_IN_ROOM=1 路径下,
    /// RootView 显式把 `webSocketClient` 传 nil 给 RealRoomViewModel.bind(...). 在 nil-client + 写入
    /// currentRoomId 触发 nil→A 分支时，**不能**清空 RoomScaffoldDefaults seed 的 4 成员
    /// —— 否则 `RoomScaffoldView` 永久失去 roster，break `testJoinRoomModalCrossScreenJoinFlow`
    /// 等断言 `roomMember_0/1/2` 出现的 UITest.
    ///
    /// 修复语义：nil→A 分支的 `members = []` / `memberPetStates = [:]` 用 `webSocketClient != nil` gate；
    /// nil-client 路径下保留 seed roster.
    ///
    /// 详见 docs/lessons/2026-05-11-uitest-nil-ws-client-must-preserve-scaffold-roster.md.
    func testNilToAPreservesScaffoldRosterWhenClientIsNil() async throws {
        let appState = AppState()
        // **关键**：不注入 webSocketClient（默认 nil）—— 模拟 UITEST_SKIP_GUEST_LOGIN=1 路径
        let vm = RealRoomViewModel(appState: appState, webSocketClient: nil)

        // baseline: vm.init() 应 seed RoomScaffoldDefaults 的 4 成员
        XCTAssertEqual(vm.members.count, RoomScaffoldDefaults.members.count,
                       "baseline：nil-client vm 应 seed RoomScaffoldDefaults.members 作为占位")
        XCTAssertEqual(vm.members.first?.id, "u1",
                       "baseline 首成员应是 RoomScaffoldDefaults seed 的 u1")

        await Task.yield()
        await Task.yield()

        // 触发 nil → A 切换（模拟 UITEST_FORCE_IN_ROOM 路径写 currentRoomId）
        appState.setCurrentRoomId("room_uitest")

        // 多 yield 让 sink 跑完所有同步副作用 + 任何可能起的 Task
        for _ in 0..<10 {
            await Task.yield()
        }
        try await Task.sleep(nanoseconds: 50_000_000)
        for _ in 0..<10 {
            await Task.yield()
        }

        // **关键断言**：nil-client 路径下 RoomScaffoldDefaults seed 必须**保留**（不被清空）
        XCTAssertEqual(vm.members.count, RoomScaffoldDefaults.members.count,
                       "nil-client + nil→A 路径必须保留 RoomScaffoldDefaults seed roster"
                       + "（否则 RoomScaffoldView UITest 断言 roomMember_0/1/2 全失败）")
        XCTAssertEqual(vm.members.first?.id, "u1",
                       "首成员仍是 RoomScaffoldDefaults seed 的 u1")
        XCTAssertEqual(vm.roomId, "room_uitest",
                       "vm.roomId 应反映新 currentRoomId（派生自 appState）")
    }

    /// **fix-review round 13 P1 回归 #2**：production path（webSocketClient ≠ nil）下行为不变 ——
    /// 仍立即清空 roster（与 r11 测试 `testNilToAClearsScaffoldRosterBeforeConnect` 同精神）.
    /// 本测试与 r11 测试是对照组：同一切换，仅 client 是否 nil 决定 reset / preserve.
    func testNilToAClearsRosterWhenClientIsNotNil() async throws {
        let appState = AppState()
        let mockWS = WebSocketClientMock()
        mockWS.connectShouldGate = true  // gate connect 让我们能在 snapshot 之前断言
        let vm = RealRoomViewModel(appState: appState, webSocketClient: mockWS)

        // baseline: seed 4 成员
        XCTAssertEqual(vm.members.count, 4, "baseline members 应是 4 个 RoomScaffoldDefaults seed")

        await Task.yield()
        await Task.yield()

        // 触发 nil → A
        appState.setCurrentRoomId("room_prod")

        // 等 connect 被调（confirm sink 同步路径已跑完）
        let deadline = Date().addingTimeInterval(1.0)
        while Date() < deadline {
            if !mockWS.connectCallArgs.isEmpty { break }
            try await Task.sleep(nanoseconds: 10_000_000)
        }
        XCTAssertEqual(mockWS.connectCallArgs, ["room_prod"],
                       "production path nil→A 仍调 connect(roomId:)")

        // **关键断言**：production path 立即清空 roster（与 r11 语义一致）
        XCTAssertEqual(vm.members, [],
                       "production path（webSocketClient ≠ nil）nil→A 必须立即清空 roster")
        XCTAssertEqual(vm.memberPetStates, [:],
                       "production path 同时清 memberPetStates")
    }
}

// MARK: - Inline mocks for Story 12.7

#if DEBUG
final class MockLeaveRoomUseCaseRoom: MockBase, LeaveRoomUseCaseProtocol, @unchecked Sendable {
    var executeStub: Result<Void, Error> = .success(())
    /// 测试 hook：让 mock UseCase 可以模拟 setCurrentRoomId 副作用（与生产 DefaultLeaveRoomUseCase 一致）.
    var onExecute: (@MainActor @Sendable () -> Void)?

    func execute() async throws {
        record(method: "execute()")
        if let hook = onExecute {
            await MainActor.run {
                hook()
            }
        }
        try executeStub.get()
    }
}
#endif
