// RealRoomViewModel.swift（Story 12.1 升级版；保留 Story 37.8 Lessons：
//   round 1 P2 fix - init seed scaffold defaults
//   round 3 P2 fix - 删除 hostCatName 派生自 currentPet
// ）.
//
// 范围（本 story 完整路径）：
//   - 构造注入 AppState + WebSocketClient（webSocketClient 默认 nil 让 RootView 老 wire 不破）
//   - roomId computed getter 来自 appState.currentRoomId（不持本地副本，避免双 source of truth）
//   - 订阅 appState.$currentRoomId：non-nil → connect WS（Story 12.2 后真实拨号；本 story 仅记 wsState = .connected 占位）；
//     nil → disconnect WS（断开 + members 清空）
//   - 订阅 webSocketClient.messages stream：解析 room.snapshot → 按 client merge contract 写 members
//   - onLeaveTap 保持 Story 37.8 行为：调 appState.setCurrentRoomId(nil) 让 HomeContainerView 切回 idle
//
// 本 story 不接 真实 URLSessionWebSocketTask 拨号（Story 12.2 落地）；wsState = .connected 仅由"appState.$currentRoomId
// 切到 non-nil + webSocketClient ≠ nil 之路径"显式切；webSocketClient = nil 时 wsState 保持 .disconnected.
//
// 关键决策：
//   1. `roomId` 是 computed getter（`var roomId: String? { appState?.currentRoomId }`），**不**用 `@Published` 修饰 ——
//      View 层不需要单独 observe `roomId`（已经通过 `roomCodeForCopy` 派生展示；`roomIdDisplay` UITest 锚定的也是
//      `roomCodeForCopy` 文本），同时避免与 AppState.currentRoomId 双 source of truth.
//   2. `subscribeRoomIdConnect` 用 `.removeDuplicates()` —— 防 AppState 多次重复 emit 同值（如 hydrate 两次）
//      触发重复 connect / disconnect.
//   3. `webSocketClient = nil` 时 wsState 保持 `.disconnected` —— RootView 当前 wire `RealRoomViewModel()` 无参 init
//      （无 webSocketClient），即使 user 已 hydrate 进房间，wsState 仍 `.disconnected`；待 Story 12.2 + 12.7
//      落地后由 UseCase 注入真实 client + 调用 `bind(appState:webSocketClient:)`，wsState 切 `.connected`.
//      这是**显式**的"半完成"语义，不是 bug —— 让 UI 在节点 4 阶段就有占位反映 WS 真实态.
//   4. `applySnapshot` 实装"最小 client merge contract" —— 严格按 §12.3 字段级 merge 规则；但**简化**
//      `level` / `status` / `isHost` 节点 4 阶段占位逻辑（`isHost = index == 0`）.
//   5. `memberPetStates` 节点 4 阶段保持空 map —— server `currentState` 固定 1 不携带真实值；待 Epic 14 后真实驱动.
//   6. `startConsumingMessages` 在 `webSocketClient = nil` 时 early return（不启 task）—— 避免空跑 task.

import Foundation
import Combine
import os.log

@MainActor
public final class RealRoomViewModel: RoomViewModel {
    /// 构造注入的 AppState 引用（同 Story 37.8 模式：可经 init(appState:webSocketClient:) 或
    /// bind(appState:webSocketClient:) 注入）.
    private var appState: AppState?

    /// 构造注入的 WebSocketClient（Story 12.1 新增；默认 nil 让 RootView `@StateObject` 老 wire 路径继续工作）.
    /// Story 12.2 / 12.7 后由真实 UseCase 注入 `WebSocketClientImpl` 实例.
    private var webSocketClient: WebSocketClient?

    /// roomId 派生 getter —— 直接来自 appState.currentRoomId，**不**持本地副本.
    /// 避免与 appState 双 source of truth（防 codex BLOCKER 4 重复出现：详见 sprint-change-proposal-2026-04-29-v2.md §3）.
    public var roomId: String? {
        appState?.currentRoomId
    }

    /// Story 37.8 round 3 P2 / Story 12.1 共用的 currentRoomId 订阅（派生 roomCodeForCopy）.
    /// 保留 Story 37.8 lesson "published-derived-state-needs-publisher-subscription"（不用一次性 hydrate）.
    private var roomCodeSubscription: AnyCancellable?

    /// Story 12.1 新增：`appState.$currentRoomId` 订阅（roomId nil ↔ non-nil 切换驱动 WS connect/disconnect + members 清空）.
    private var roomIdConnectSubscription: AnyCancellable?

    /// Story 12.1 新增：WebSocket messages stream consumer task（订阅 webSocketClient.messages → 解析 → 派生 members）.
    private var messageConsumerTask: Task<Void, Never>?

    /// Story 12.1 fix-review round 1：跟踪订阅看到的"上一次 currentRoomId 值"，用于在 sink 闭包内
    /// 区分四种转换：nil→nil（no-op）/ nil→A（connect）/ A→B（reset roster + 重启 stream）/ A→nil（disconnect）.
    /// A→A 同值由 `removeDuplicates()` 在 publisher 层挡掉，不会进 sink.
    ///
    /// **关键**：订阅起步前**不**预设此字段为 `appState.currentRoomId` 当前值 —— 否则
    /// `Published` 订阅时同步 emit 的当前值进入 sink 会被识别为 A→A no-op，restored in-room
    /// session 路径下"nil → 已非 nil"的转换信号丢失. 字段保持默认 nil；
    /// 若 ViewModel 在 appState.currentRoomId 已非 nil 时订阅，第一条 emission 走 `(nil, A)` connect 分支
    /// 把 wsState 切对.
    private var lastObservedRoomId: String?

    public override init() {
        super.init()
        self.appState = nil
        self.webSocketClient = nil
        // Story 37.8 round 1 P2 fix：seed RoomScaffoldDefaults 让首帧渲染不空.
        self.roomCodeForCopy = RoomScaffoldDefaults.roomCodeForCopy
        self.hostCatName = RoomScaffoldDefaults.hostCatName
        self.members = RoomScaffoldDefaults.members
        self.userIsHost = RoomScaffoldDefaults.userIsHost
        // wsState / memberPetStates 走基类默认值（.disconnected / [:]）.
    }

    public init(appState: AppState, webSocketClient: WebSocketClient? = nil) {
        super.init()
        self.appState = appState
        self.webSocketClient = webSocketClient
        // Story 37.8 round 1 P2 fix：seed.
        self.roomCodeForCopy = RoomScaffoldDefaults.roomCodeForCopy
        self.hostCatName = RoomScaffoldDefaults.hostCatName
        self.members = RoomScaffoldDefaults.members
        self.userIsHost = RoomScaffoldDefaults.userIsHost
        subscribeRoomCode(to: appState)
        // fix-review round 1：consumer task 不在 init 起步；由 subscribeRoomIdConnect 在 nil→A / A→B 分支
        // 调用 startConsumingMessages 唯一起 task，避免 init + sink 双起 task 争抢 AsyncStream（同一 stream
        // 多 iterator 是未定义行为；表现为消息丢失 → snapshot 不被 consume）.
        subscribeRoomIdConnect(to: appState)
    }

    /// AppState + WebSocketClient 异步注入入口（与 Story 37.8 bind 同模式扩展两路）.
    public func bind(appState: AppState, webSocketClient: WebSocketClient? = nil) {
        let codeAlreadySubscribed = roomCodeSubscription != nil
        let connectAlreadySubscribed = roomIdConnectSubscription != nil
        self.appState = appState
        if let client = webSocketClient {
            self.webSocketClient = client
            // fix-review round 1：bind 路径由 subscribeRoomIdConnect 起 task；同一 stream 多 iterator 是未定义行为.
            // 但若 connectAlreadySubscribed=true 且当前已经在房间内，subscribeRoomIdConnect 不会再触发 sink；
            // 那种情况下需要主动 startConsumingMessages 接上新 client. 详见下面分支.
            _ = client
        }
        if !codeAlreadySubscribed {
            subscribeRoomCode(to: appState)
        }
        if !connectAlreadySubscribed {
            // 首次订阅：sink 同步 emit 会按 (nil, currentRoomId) 决定是否启 task.
            subscribeRoomIdConnect(to: appState)
        } else if webSocketClient != nil && lastObservedRoomId != nil {
            // 已订阅 + 已在房间内 + 现在补注入 client → 主动起 task 接上 messages stream
            // （否则 task 永远等不到下一次 currentRoomId 切换才起）.
            if self.webSocketClient != nil {
                self.wsState = .connected
            }
            startConsumingMessages()
        }
    }

    // MARK: - subscribe helpers

    /// 订阅 appState.$currentRoomId —— hydrate / reset / 单独 mutate 都派生 roomCodeForCopy.
    /// nil → fallback 到 RoomScaffoldDefaults.roomCodeForCopy 占位（避免 in-room scaffold 显示空房间号）；
    /// non-nil → 直接用 roomId 值.
    private func subscribeRoomCode(to appState: AppState) {
        roomCodeSubscription = appState.$currentRoomId
            .sink { [weak self] roomId in
                guard let self else { return }
                self.roomCodeForCopy = roomId ?? RoomScaffoldDefaults.roomCodeForCopy
            }
    }

    /// Story 12.1 AC4 关键路径：roomId 转换驱动 wsState + members 清空 + stream 重启.
    /// 单元测试 case#3 直接测本订阅触发的副作用.
    ///
    /// 关键决策（fix-review round 1 修订）：
    ///
    /// **不再用 `.dropFirst()`** —— 旧实装抑制了"订阅时的同步 emit"，但 `/home` restored in-room
    /// session 路径（`AppState.applyHomeData(homeData)` 在 ViewModel 订阅前已写非 nil currentRoomId）下
    /// 第一个 emission 就是真实转换信号被 dropFirst 吃掉，wsState 永远停 .disconnected。
    /// 改用 `lastObservedRoomId` 字段在 sink 内识别四种转换，保留同步 emit 但避免 nil→nil 的 no-op
    /// 触发"未拨号即 disconnect"副作用（注释写在原决策里的同问题）.
    ///
    /// 四种转换分支：
    ///   1. nil → nil：订阅时同步 emit 的初始空房间状态 —— **no-op**（不调 disconnect 避免误关刚注入的 mock client）
    ///   2. nil → A：进入房间 —— wsState = .connected（webSocketClient ≠ nil 时）+ 启动 stream consumer
    ///   3. A → B：room 切换 —— **重置 roster** (members / memberPetStates 清空) + tear down 旧 stream
    ///      + 启动新 stream consumer + wsState 保持 .connected（避免旧 room late messages 污染新 room）
    ///   4. A → nil：离开房间 —— disconnect + 清空 roster + wsState = .disconnected
    ///   5. A → A 同值：`removeDuplicates` 已抑制；不会进 sink
    ///
    /// `removeDuplicates` 仍保留以防同值重复 emit（如 hydrate 两次都把 currentRoomId 置为同 roomId）.
    private func subscribeRoomIdConnect(to appState: AppState) {
        // 不预设 lastObservedRoomId（保持默认 nil）：让订阅同步 emit 走 (previous=nil, new=非 nil) connect 分支
        // 把 restored in-room session 的初始 currentRoomId 信号正确处理.
        //
        // **fix-review round 2 P2**：本 sink **不**把 `""` normalize 成 nil。
        // 理由：`HomeRoomDispatcher.shouldShowRoom(currentRoomId:)`（HomeContainerView.swift:98）
        //       钦定空字符串为 in-room（HomeContainerViewTests:41 锚定）；若本 sink 反向把 `""` 当 nil
        //       走 disconnect 分支，会出现 UI 渲染 `RoomScaffoldView` 而 vm 走 disconnect/clear-members
        //       的不一致状态。两边对齐 `""` ⇒ in-room 才能保持 UI ↔ vm 状态机自洽.
        // server 契约保证 roomId 非空（数据库设计.md §room_id 钦定）；空字符串只可能来自 caller bug,
        // 此时按 in-room 处理更安全（dispatcher 一侧已是这个语义）.
        roomIdConnectSubscription = appState.$currentRoomId
            .removeDuplicates()
            .sink { [weak self] newRoomId in
                guard let self else { return }
                let previous = self.lastObservedRoomId
                self.lastObservedRoomId = newRoomId

                switch (previous, newRoomId) {
                case (nil, nil):
                    // 分支 1：订阅起步时同步 emit 的初始空房间状态 —— no-op.
                    // 这是 dropFirst 旧实装真正想避开的场景；用显式分支替代后既能正确处理
                    // restored in-room（previous=nil + newRoomId 已非 nil 走分支 2）也能保留 no-op 语义.
                    break
                case (nil, .some(let roomId)):
                    // 分支 2：nil → A，进入房间.
                    //
                    // **fix-review round 2 P1**：若 client 之前被 disconnect 过（leave-rejoin 路径：
                    //   A → nil → A'），其 `messages` stream 已被 finish；必须先调 `prepareForReconnect()`
                    //   重置 stream，否则新 consumer task 接到的是已 finish 的 stream，subsequent
                    //   `room.snapshot` 永远收不到。首次 nil→A（构造后第一次进房间）调 `prepareForReconnect()`
                    //   也是安全的（mock no-cost；Story 12.2 后 production impl 是 idempotent）.
                    self.webSocketClient?.prepareForReconnect()
                    if self.webSocketClient != nil {
                        self.wsState = .connected
                    }
                    self.startConsumingMessages()
                    os_log(.debug, "RealRoomViewModel: nil → %{public}@ (WS stream started)", roomId)
                case (.some(let prev), .some(let next)):
                    // 分支 3：A → B，房间切换 —— 必须清空旧 roster + tear down 旧 stream + 取消旧 consumer task.
                    // 否则同 @StateObject vm 实例下 room B 会渲染 room A 的 roster，
                    // 旧 stream 的 late messages 也会污染新房间.
                    //
                    // 节点 4 阶段语义：本 story 仅暴露 `disconnect()` + `messages` stream + `prepareForReconnect()`
                    // 接缝；没有 `connect(roomId:)`。A→B 真正"拨号到新 room channel"要等 Story 12.2 落地
                    // `WebSocketClientImpl.connect(roomId:token:)`。本分支当前实装做的是：
                    //   ① disconnect 旧 stream（mock 下 finish stream，真实下关 socket）
                    //   ② 清空 roster
                    //   ③ **fix-review round 2 P1**：调 `prepareForReconnect()` 让 client 准备好新 stream
                    //      —— 否则新 consumer task 接到的是已被 finish 的旧 stream，永远收不到 room B 的
                    //      `room.snapshot`，UI 永远空房间.
                    //   ④ wsState 切 .connected（webSocketClient ≠ nil；占位语义）
                    //   ⑤ cancel 并重启 consumer task（读 `client.messages` 拿新 stream）.
                    // Story 12.2 后此分支在 `prepareForReconnect()` 后再调 `webSocketClient.connect(roomId: next)`
                    // 完成真实重连.
                    self.webSocketClient?.disconnect()
                    self.members = []
                    self.memberPetStates = [:]
                    self.webSocketClient?.prepareForReconnect()
                    if self.webSocketClient != nil {
                        self.wsState = .connected
                    }
                    self.startConsumingMessages()
                    os_log(.debug, "RealRoomViewModel: %{public}@ → %{public}@ (roster reset + stream restarted)", prev, next)
                case (.some(let prev), nil):
                    // 分支 4：A → nil，离开房间.
                    self.webSocketClient?.disconnect()
                    self.members = []
                    self.memberPetStates = [:]
                    self.wsState = .disconnected
                    os_log(.debug, "RealRoomViewModel: %{public}@ → nil (cleared roster + WS disconnected)", prev)
                }
            }
    }

    /// Story 12.1 AC4 关键路径：subscribe webSocketClient.messages → 解析 room.snapshot → 写 members.
    /// for-await 走 detached task；ViewModel deinit / disconnect 时 task cancel + stream finish 自然退出.
    private func startConsumingMessages() {
        messageConsumerTask?.cancel()
        guard let client = webSocketClient else { return }
        messageConsumerTask = Task { [weak self] in
            for await message in client.messages {
                guard let self else { return }
                await MainActor.run {
                    self.handle(message: message)
                }
            }
        }
    }

    /// §12.3 client merge contract 实装：snapshot 是 enrich/correct 而非 wipe-out.
    /// 节点 4 阶段（本 story）实装最小路径：
    ///   - roster 集合层：以 snapshot 的 userId 集合为权威（缺失则移除、新增则 append）
    ///   - 字段级：非空值覆盖、空字符串保留 client 已有值、null 直接覆盖
    ///   - memberPetStates：节点 4 阶段 server 固定 currentState=1 → 本 story 保持空 map（Epic 14 真实驱动后再写入）
    private func handle(message: WSMessage) {
        switch message {
        case .roomSnapshot(let payload):
            applySnapshot(payload)
        case .pong:
            // Story 12.6 心跳框架处理；本 story discard.
            break
        case .error(let code, let message, _):
            os_log(.error, "RealRoomViewModel WS error: code=%{public}d, msg=%{public}@", code, message)
        case .unknown(let rawType):
            os_log(.error, "RealRoomViewModel WS unknown message type: %{public}@", rawType)
        }
    }

    /// snapshot apply（roster 集合 + 字段级 merge）.
    /// 节点 4 阶段：snapshot members[] 直接映射为 RoomMember 数组（id=userId, name=nickname || 占位,
    /// level=8 占位, status="在玩耍" 占位, isHost=index==0 占位）—— `level` / `status` / `isHost`
    /// 由 Epic 14 / Epic 8 / 后续 host 字段下发后真实派生；本 story 仅保证 `id` / `name` 与 snapshot 一致.
    /// **节点 4 placeholder 阶段允许 nickname 为空字符串**——按 §12.3 "client merge contract" 空字符串 =
    /// "server 不知道"，应保留 client 已有值；本 story 实装策略（最小路径）：snapshot member.nickname 为空字符串时,
    /// **保留** client 已有同 userId 的 RoomMember.name；新成员（client 没有的 userId）首次出现 nickname 为空字符串时
    /// 降级为 placeholder "成员"（与 ui_design 占位一致；Story 11.7 真实 nickname 落地后即被覆盖）.
    private func applySnapshot(_ payload: RoomSnapshotPayload) {
        let snapshotUserIds = Set(payload.members.map { $0.userId })
        // step 1: 按 userId 集合做"roster 权威"裁剪 + 增量
        var newMembers: [RoomMember] = []
        for (index, snapshotMember) in payload.members.enumerated() {
            let existing = self.members.first { $0.id == snapshotMember.userId }
            // 字段级 merge: nickname 空字符串保留 existing.name；非空覆盖
            let mergedName: String = {
                if !snapshotMember.nickname.isEmpty {
                    return snapshotMember.nickname
                } else if let existing = existing {
                    return existing.name  // client 已有值（来自上一次 snapshot 或 GET /rooms 响应）
                } else {
                    return "成员"  // placeholder（首次出现 + nickname 空字符串；Story 11.7 后即覆盖）
                }
            }()
            // level / status / isHost 节点 4 阶段保持占位
            let merged = RoomMember(
                id: snapshotMember.userId,
                name: mergedName,
                level: existing?.level ?? 8,
                status: existing?.status ?? "在玩耍",
                isHost: index == 0  // 节点 4 阶段约定：snapshot 第一个成员视为 host（与 ui_design room.jsx 默认渲染一致；
                                    // 后续 epic 引入 host userId 字段后真实派生）
            )
            newMembers.append(merged)
        }
        self.members = newMembers
        // memberPetStates：节点 4 阶段 server 固定 currentState=1，不写入；Epic 14 后真实驱动.
        // 本 story 不动 memberPetStates（保持初始 [:]）.
        os_log(.debug, "RealRoomViewModel: applied snapshot (members.count = %{public}d)", newMembers.count)
        _ = snapshotUserIds  // for future use（Story 12.4 增量 mutate 时需要做 set diff）
    }

    // MARK: - override abstract methods

    public override func onLeaveTap() {
        os_log(.debug, "RealRoomViewModel.onLeaveTap (Story 12.7 will wire LeaveRoomUseCase)")
        // 节点 4 占位：直接置 currentRoomId = nil（subscribeRoomIdConnect 自动触发 disconnect + members
        // 清空 + wsState = .disconnected）.
        // Story 12.7 落地 LeaveRoomUseCase 后改为：调 server POST /rooms/{id}/leave → 成功后再 setCurrentRoomId(nil).
        self.appState?.setCurrentRoomId(nil)
    }

    public override func onCopyTap() {
        os_log(.debug, "RealRoomViewModel.onCopyTap")
        // 实际 UIPasteboard 复制由 RoomScaffoldView 内 SwiftUI @State + 调用本方法时一起触发（Story 37.8 落地）.
    }

    deinit {
        messageConsumerTask?.cancel()
    }
}
