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

    /// Story 12.7 AC6: LeaveRoom UseCase 注入（默认 nil 让既有 caller / UITest 走 fallback 老 mock 行为）.
    private var leaveRoomUseCase: LeaveRoomUseCaseProtocol?

    /// Story 12.7 AC6: ErrorPresenter 注入（默认 nil；用于 onLeaveTap 错误兜底；caller=RootView 注入 container.errorPresenter）.
    private weak var errorPresenter: ErrorPresenter?

    /// Story 12.7 r14 [P1] fix（codex review）：home refresh hook（同 RealHomeViewModel 模式）.
    /// 触发条件：LeaveRoomUseCase 抛 RoomNavigationStaleError —— server 端已让用户离开 room 但
    /// client UI 因 navigation race 没写 currentRoomId=nil（且用户可能已 rejoin）.
    /// 调 closure 让 RootView 重拉 /home 拿 authoritative state，client/server 收敛.
    private var refreshHomeOnStaleNavigation: (@MainActor @Sendable () -> Void)?

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
    ///
    /// **fix-review round 5 P2**：bind 被以**不同的** webSocketClient instance 重新调用时
    /// （vm 已 bound 且在房间中），必须先对旧 client 调 disconnect() + cancel 旧 messageConsumerTask
    /// 再 swap，否则旧 socket 仍 subscribed → 资源泄漏 + 旧 client deliver duplicate room traffic.
    /// 同 instance 重 bind 时 no-op（避免 redundant disconnect 把好 client 关掉）.
    /// 用 `===` identity 比较（WebSocketClient protocol 已是 `: AnyObject, Sendable`，class-only）.
    ///
    /// **fix-review round 6 P2**：consumer 重启必须 gated on 实际 client swap / first injection,
    /// 不能在 same-instance rebind 时无条件 restart。三种语义：
    ///   - a) `oldClient === newClient`（同 instance rebind）→ true no-op：**不**调 startConsumingMessages
    ///        （否则 cancel 当前 consumer + 在同一已运行的 AsyncStream 上 start new iterator,
    ///         没调 prepareForReconnect → in-flight `room.snapshot` 可能被丢）
    ///   - b) `oldClient != nil && newClient != oldClient`（swap）→ disconnect 旧 + prepareForReconnect 新
    ///        + startConsumingMessages（新 client 的新 stream 上起新 consumer）
    ///   - c) `oldClient == nil && newClient != nil`（first injection）→ startConsumingMessages
    ///        （**不**调 prepareForReconnect：mock 构造的是 fresh stream；production WebSocketClientImpl
    ///        首次也无需 reset）
    public func bind(
        appState: AppState,
        webSocketClient: WebSocketClient? = nil,
        leaveRoomUseCase: LeaveRoomUseCaseProtocol? = nil,
        errorPresenter: ErrorPresenter? = nil,
        refreshHomeOnStaleNavigation: (@MainActor @Sendable () -> Void)? = nil
    ) {
        let codeAlreadySubscribed = roomCodeSubscription != nil
        let connectAlreadySubscribed = roomIdConnectSubscription != nil
        self.appState = appState

        // Story 12.7 AC6: 若 caller 注入了 leaveRoomUseCase / errorPresenter，覆盖既有引用
        // （RootView 只调一次 bind；老 caller 不传保留 nil 走 fallback 老 mock 行为）.
        if let useCase = leaveRoomUseCase {
            self.leaveRoomUseCase = useCase
        }
        if let presenter = errorPresenter {
            self.errorPresenter = presenter
        }
        // r14 [P1] fix: 注入 refresh hook（caller 不传时保留 nil 让 UITEST / preview 路径不触发）.
        if let refresh = refreshHomeOnStaleNavigation {
            self.refreshHomeOnStaleNavigation = refresh
        }

        // fix-review round 5 P2 + round 6 P2：替换 client instance 时分三种语义分类处理.
        // 跟踪是否为「真实的 client swap / first injection」—— 仅此情况才需重启 consumer task.
        var clientChanged = false
        if let newClient = webSocketClient {
            if let oldClient = self.webSocketClient, oldClient === newClient {
                // (a) 同一 instance：true no-op（既不 disconnect、也不 cancel task、也不 restart consumer）
                clientChanged = false
            } else if let oldClient = self.webSocketClient {
                // (b) 不同 instance 替换：disconnect 旧 + cancel 旧 task + swap + prepareForReconnect 新 client
                // prepareForReconnect 关键性：A→B / leave-rejoin 同样语义 —— 新 client 的 stream 必须是 fresh
                // （否则 consumer 接已 finish stream，永远收不到消息）.
                oldClient.disconnect()
                self.messageConsumerTask?.cancel()
                self.messageConsumerTask = nil
                self.webSocketClient = newClient
                newClient.prepareForReconnect()
                clientChanged = true
            } else {
                // (c) 旧 = nil 首次注入：仅 swap（无旧 client 可 disconnect；newClient 的 stream 是 fresh）
                self.webSocketClient = newClient
                clientChanged = true
            }
        }

        if !codeAlreadySubscribed {
            subscribeRoomCode(to: appState)
        }
        if !connectAlreadySubscribed {
            // 首次订阅：sink 同步 emit 会按 (nil, currentRoomId) 决定是否启 task.
            subscribeRoomIdConnect(to: appState)
        } else if clientChanged && webSocketClient != nil && lastObservedRoomId != nil {
            // 已订阅 + 已在房间内 + **client 实际发生变更**（swap 或 first injection） → 主动起 task 接上 messages stream
            // （否则 task 永远等不到下一次 currentRoomId 切换才起）.
            //
            // **fix-review round 6 P2 关键 gate**：`clientChanged` 才进；same-instance rebind 不进
            // —— 否则 cancel 当前 consumer + 同一 stream 上 start new iterator → 丢 in-flight snapshot.
            //
            // 注意：上面 swap 分支已 cancel 旧 task；这里起的是新 client 的 task.
            if self.webSocketClient != nil {
                // bind first-injection / swap 路径占位 .connected（与 sink 路径对称）.
                self.wsState = .connected
            }
            startConsumingMessages()
            // Story 12.7 AC6: bind first-injection / swap 路径也追加 connect 触发（与 sink 路径对称）.
            // 让 RootView .onAppear 同步 bind 路径下既有 in-room session（restored 或 UITEST_FORCE_IN_ROOM）
            // 也能真实拨号 WS（sink 路径 lastObservedRoomId 还没切，不会进 connect 分支）.
            //
            // **fix-review round 1 P2（Story 12.7）**：旧 `try?` 路径吞掉 sync failure（token 空 / URL
            // 构造失败抛 WSError 不会 emit .connectionStateChanged → UI 卡死在"占位 .connected"）.
            // 修复：do/catch await connect → 失败时纠正为 .disconnected.
            //
            // **fix-review round 3 P1（Story 12.7）**：catch 路径**不**调 `errorPresenter.present(error)`.
            // WS connect failure 是后台 transient 状态（server down / network flap / handshake error），
            // **不**是用户主动操作的同步错误；通过 `errorPresenter` 走全屏 `.retry` overlay 会 block 整个 app
            // 的 hit-testing 而不是仅反映 room 的 disconnected/reconnecting 状态.
            // 正确语义：wsState=.disconnected 由 RoomScaffoldView 内的 wsStateLabel 反映给用户；
            // 重连状态机后续自动尝试 reconnect（Story 12.6 已实装）.
            // 只保留日志 / wsState=.disconnected，不弹 modal overlay.
            // 详见 docs/lessons/2026-05-11-ws-connect-failure-must-not-use-error-presenter.md.
            if let roomId = self.lastObservedRoomId, !roomId.isEmpty {
                let client = self.webSocketClient
                let connectingRoomId = roomId
                let connectingClient = client
                Task { @MainActor [weak self, weak client] in
                    guard let client else { return }
                    do {
                        try await client.connect(roomId: connectingRoomId)
                        // 成功路径：占位 .connected 已 set，不重写.
                    } catch {
                        // **fix-review round 4 P2（Story 12.7）**：stale connect failure 守护.
                        // 旧实装在 catch 内**无条件** set `wsState = .disconnected`. 但用户在 connect
                        // 还 await 时切换房间 / 离开（A→B / A→nil）会先调 disconnect()/prepareForReconnect()
                        // 让旧 connect throw later —— 此 throw 属于 stale room A 的失败，**不**应覆盖
                        // 当前 room B 的 wsState（可能 .connected / .reconnecting）.
                        // 守护语义：捕获 connect 调用时的 connectingRoomId，await 返回后比对
                        //   `lastObservedRoomId == connectingRoomId`，不匹配则丢弃信号 + log debug.
                        // 与 12.4 r1 P1 `streamRoomId` 守护同精神：stale event 不能覆盖当前 room state.
                        guard self?.lastObservedRoomId == connectingRoomId else {
                            os_log(.debug,
                                   "RealRoomViewModel.bind: discard stale connect failure (connectingRoomId=%{public}@, current=%{public}@)",
                                   connectingRoomId,
                                   self?.lastObservedRoomId ?? "<nil>")
                            return
                        }
                        // **fix-review round 9 P2（Story 12.7）**：client-identity 守护.
                        // 仅 lastObservedRoomId 校验不够 —— 若 bind 在同 roomId 下 swap 出新 client
                        // instance（同房间内 token 刷新 / 测试场景），旧 client 的 in-flight connect
                        // 因 swap-path 触发 disconnect 而 throw later. 此时 lastObservedRoomId 仍是
                        // 同 roomId（match），但 wsState 已被新 client 切到 .connected → 老 catch 路径
                        // 会错误 flip 回 .disconnected. 增加 client === capturedClient identity 校验,
                        // mismatch 时丢弃失败信号.
                        // 详见 docs/lessons/2026-05-11-async-error-handler-must-stale-guard-room-id-and-client-identity-12-7-r9.md.
                        guard self?.webSocketClient === connectingClient else {
                            os_log(.debug,
                                   "RealRoomViewModel.bind: discard stale connect failure from replaced client (connectingRoomId=%{public}@)",
                                   connectingRoomId)
                            return
                        }
                        os_log(.error,
                               "RealRoomViewModel.bind: connect(roomId:%{public}@) failed: %{public}@",
                               connectingRoomId, String(describing: error))
                        self?.wsState = .disconnected
                    }
                }
            }
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
                    // **fix-review round 11 P2（Story 12.7）**：先 reset roster（与 A→B 分支对齐）.
                    // 旧实装在 nil→A 切换时**不**清空 members / memberPetStates → UI 切到 RoomView 后
                    // 仍展示 RoomScaffoldDefaults seed 的 4 个假成员，直到第一个 `room.snapshot` 到达
                    // 才被 applySnapshot 覆盖；若 connect(roomId:) 抛错（token 缺失 / server down /
                    // handshake error）→ snapshot 永远不到 → 假成员**永久残留**.
                    // A→B 分支已 reset roster（line 407-408），nil→A 分支必须同语义对齐.
                    //
                    // **fix-review round 13 P1（Story 12.7）**：reset 必须**条件化** —— 仅当
                    // `webSocketClient != nil` 时清，nil-client 路径下保留 RoomScaffoldDefaults seed.
                    // 理由：UITEST_SKIP_GUEST_LOGIN=1 + UITEST_FORCE_IN_ROOM=1（r3 P1 引入的 launch path）
                    //   下 RootView 显式把 `webSocketClient` 传 nil（见 RootView.swift:293）；vm 既不会
                    //   `prepareForReconnect()`、也不会拨号 connect、永远没有 `room.snapshot` 到达 ——
                    //   此时 unconditional clear 会让 `RoomScaffoldView` 永久失去 seeded 4 成员 →
                    //   `testJoinRoomModalCrossScreenJoinFlow` 等断言 `roomMember_0/1/2` 出现的 UITest 全 break.
                    // production path（webSocketClient ≠ nil）保持 r11 语义：snapshot 抵达前不残留假成员.
                    // 详见 docs/lessons/2026-05-11-room-entry-must-clear-scaffold-roster-before-connect.md
                    // 与 docs/lessons/2026-05-11-uitest-nil-ws-client-must-preserve-scaffold-roster.md.
                    if self.webSocketClient != nil {
                        self.members = []
                        self.memberPetStates = [:]
                    }
                    //
                    // **fix-review round 2 P1**：若 client 之前被 disconnect 过（leave-rejoin 路径：
                    //   A → nil → A'），其 `messages` stream 已被 finish；必须先调 `prepareForReconnect()`
                    //   重置 stream，否则新 consumer task 接到的是已 finish 的 stream，subsequent
                    //   `room.snapshot` 永远收不到。首次 nil→A（构造后第一次进房间）调 `prepareForReconnect()`
                    //   也是安全的（mock no-cost；Story 12.2 后 production impl 是 idempotent）.
                    self.webSocketClient?.prepareForReconnect()
                    if self.webSocketClient != nil {
                        // 占位 .connected（in-room scaffold UI 同步反映"用户已进房间"）；
                        // 真实握手由后续 spawn Task 完成；失败 catch 路径会立即把它纠正为 .disconnected.
                        self.wsState = .connected
                    }
                    self.startConsumingMessages()
                    // Story 12.7 AC6 关键改动：追加真实 WS 拨号触发（nil → A 进入房间）.
                    // wrap in Task 让 sink 闭包不阻塞.
                    // 空字符串路径（HomeRoomDispatcher 把 "" 当 in-room）下不真实拨号 ——
                    //   server 端 roomId 路径校验对空字符串会 close 4002；这里 guard 让本地占位 "" 不打 server.
                    //
                    // **fix-review round 1 P2（Story 12.7）**：旧实装 `try?` 静默吞 sync failure（token
                    // 空 / URL 构造失败抛 WSError 不会 emit .connectionStateChanged → UI 卡死在
                    // "占位 .connected" 状态实际无 socket）.
                    // 修复：do/catch await connect → 失败时把占位 `.connected` 纠正为 `.disconnected`.
                    //
                    // **fix-review round 3 P1（Story 12.7）**：catch 路径**不**调 `errorPresenter.present(error)`.
                    // WS connect failure 是后台 transient 状态（server down / network flap / handshake error），
                    // **不**是用户主动操作的同步错误；通过 `errorPresenter` 走全屏 `.retry` overlay 会 block
                    // 整个 app 的 hit-testing 而不是仅反映 room 的 disconnected/reconnecting 状态.
                    // 正确语义：wsState=.disconnected 由 RoomScaffoldView 内的 wsStateLabel 反映给用户；
                    // 重连状态机后续自动尝试 reconnect.
                    // 详见 docs/lessons/2026-05-11-ws-connect-failure-must-not-use-error-presenter.md.
                    //
                    // 成功路径维持上面 sync-set 的 `.connected`（无需重设）；后续 server 推
                    // `.connectionStateChanged(.connected)` reactive 路径也对齐（`handle(message:)` line 433-464）.
                    if !roomId.isEmpty {
                        let client = self.webSocketClient
                        let connectingRoomId = roomId
                        let connectingClient = client
                        Task { @MainActor [weak self, weak client] in
                            guard let client else { return }
                            do {
                                try await client.connect(roomId: connectingRoomId)
                                // 成功路径：占位 .connected 已 set；不重复写避免与 .connectionStateChanged
                                // reactive 路径时序竞争（receive loop emit .connected 时已是真实信号）.
                            } catch {
                                // **fix-review round 4 P2（Story 12.7）**：stale connect failure 守护.
                                // 详见 docs/lessons/2026-05-11-ws-stale-connect-failure-must-be-gated-on-room-id.md.
                                // 用户在 connect await 中切换房间（A→B）/ 离开（A→nil）会先调
                                // disconnect()/prepareForReconnect() 让旧 connect throw later —— 此
                                // throw 属于 stale room A 的失败，**不**应覆盖当前 room B 的 wsState.
                                // 守护：比对 `lastObservedRoomId == connectingRoomId`，不匹配则丢弃.
                                guard self?.lastObservedRoomId == connectingRoomId else {
                                    os_log(.debug,
                                           "RealRoomViewModel: discard stale nil→A connect failure (connectingRoomId=%{public}@, current=%{public}@)",
                                           connectingRoomId,
                                           self?.lastObservedRoomId ?? "<nil>")
                                    return
                                }
                                // **fix-review round 9 P2（Story 12.7）**：client-identity 守护.
                                // 若 bind swap webSocketClient instance 但保持同 roomId, 旧 client 的
                                // in-flight connect 因 swap-path disconnect() 而 throw later. 仅 roomId
                                // 校验过不掉这种 race —— 增加 `webSocketClient === connectingClient`
                                // identity 校验. 详见 docs/lessons/2026-05-11-async-error-handler-must-stale-guard-room-id-and-client-identity-12-7-r9.md.
                                guard self?.webSocketClient === connectingClient else {
                                    os_log(.debug,
                                           "RealRoomViewModel: discard stale nil→A connect failure from replaced client (connectingRoomId=%{public}@)",
                                           connectingRoomId)
                                    return
                                }
                                os_log(.error,
                                       "RealRoomViewModel: nil→A connect(roomId:%{public}@) failed: %{public}@",
                                       connectingRoomId, String(describing: error))
                                // **关键纠错**：旧实装 `try?` 在此处吞掉信号；新实装把占位
                                // `.connected` 还原成真实 `.disconnected`，UI 不再卡在"假 connected".
                                self?.wsState = .disconnected
                            }
                        }
                    }
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
                    //
                    // **fix-review round 13 P1（Story 12.7）**：roster reset 与 nil→A 分支同精神，
                    // 仅当 `webSocketClient != nil` 时清；nil-client 路径下保留 scaffold seed roster.
                    // 虽然 A→B 在当前 UITEST_FORCE_IN_ROOM 路径下不易触发（UITEST 只写一次 currentRoomId），
                    // 但语义对齐避免未来 UITest 扩展时再次破坏 scaffold 兜底.
                    // 详见 docs/lessons/2026-05-11-uitest-nil-ws-client-must-preserve-scaffold-roster.md.
                    self.webSocketClient?.disconnect()
                    if self.webSocketClient != nil {
                        self.members = []
                        self.memberPetStates = [:]
                    }
                    self.webSocketClient?.prepareForReconnect()
                    // **fix-review round 1 P2（Story 12.7）**：保持 wsState 上一态 `.connected`（room A
                    // 阶段已 set；A→B 切换"用户仍在房间内"语义维持）—— 失败 catch 路径会把它纠正为
                    // `.disconnected`，所以不存在"connect 失败仍卡 .connected"的 review 担心.
                    if self.webSocketClient != nil {
                        self.wsState = .connected
                    }
                    self.startConsumingMessages()
                    // Story 12.7 AC6 关键改动：A → B 切换也追加 connect 触发新 room.
                    // 与 (nil, A) 分支同精神；同样 wrap in Task；空字符串守护.
                    // **fix-review round 1 P2（Story 12.7）**：旧 `try?` 路径吞掉 sync failure
                    // → A→B 切换 connect 抛错时 wsState 卡 .connected 但实际无 socket；
                    // 修复：do/catch → 失败时纠正为 .disconnected.
                    //
                    // **fix-review round 3 P1（Story 12.7）**：catch 路径**不**调 `errorPresenter.present(error)`.
                    // 同 nil→A 分支注释；WS connect failure 是后台 transient 状态，不应走全屏 retry overlay
                    // block 整个 app hit-testing.
                    // 详见 docs/lessons/2026-05-11-ws-connect-failure-must-not-use-error-presenter.md.
                    if !next.isEmpty {
                        let client = self.webSocketClient
                        let connectingRoomId = next
                        let connectingClient = client
                        Task { @MainActor [weak self, weak client] in
                            guard let client else { return }
                            do {
                                try await client.connect(roomId: connectingRoomId)
                                // 成功路径：占位 .connected 已 set，不重写.
                            } catch {
                                // **fix-review round 4 P2（Story 12.7）**：stale connect failure 守护.
                                // 详见 docs/lessons/2026-05-11-ws-stale-connect-failure-must-be-gated-on-room-id.md.
                                // A→B 切换 connect 还在 await 时用户再切到 C 或离开 → 旧 disconnect()
                                // 让本 connect throw later. 此 throw 属于 stale room B 的失败，
                                // 不应覆盖当前 room C 的 wsState.
                                guard self?.lastObservedRoomId == connectingRoomId else {
                                    os_log(.debug,
                                           "RealRoomViewModel: discard stale A→B connect failure (connectingRoomId=%{public}@, current=%{public}@)",
                                           connectingRoomId,
                                           self?.lastObservedRoomId ?? "<nil>")
                                    return
                                }
                                // **fix-review round 9 P2（Story 12.7）**：client-identity 守护.
                                // 同 nil→A 分支：bind swap webSocketClient instance 但保持同 roomId 时,
                                // 旧 client connect throw later 仍会通过 roomId 校验; 必须额外校验
                                // `webSocketClient === connectingClient`. 详见 docs/lessons/2026-05-11-async-error-handler-must-stale-guard-room-id-and-client-identity-12-7-r9.md.
                                guard self?.webSocketClient === connectingClient else {
                                    os_log(.debug,
                                           "RealRoomViewModel: discard stale A→B connect failure from replaced client (connectingRoomId=%{public}@)",
                                           connectingRoomId)
                                    return
                                }
                                os_log(.error,
                                       "RealRoomViewModel: A→B connect(roomId:%{public}@) failed: %{public}@",
                                       connectingRoomId, String(describing: error))
                                self?.wsState = .disconnected
                            }
                        }
                    }
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
    ///
    /// **fix-review round 2 P1（Story 12.4）**：在启动 task 时**捕获**当时 `lastObservedRoomId` 作为
    /// 局部 `streamRoomId`，传给 `handle(message:streamRoomId:)`。这是 cross-room race 的关键防御：
    ///
    /// 背景：A→B 切换路径下 `subscribeRoomIdConnect` 会 cancel 旧 consumer task + restart 新 task。
    /// 但 `Task.cancel()` 不会立即中断已经被 `for await` dequeue 但还没 `await MainActor.run` 投递
    /// 的 message —— 旧 task 在 cancel 前可能已 dequeue 一条 room A 的 `member.joined` / `member.left`，
    /// 仍会被 handle 应用到 room B 的 members。
    ///
    /// V1 §12.3 钦定 `member.joined` / `member.left` payload **不**含 room.id
    /// （`member.joined`：userId / nickname / avatarUrl / pet；`member.left`：userId）→
    /// 无法做 per-event payload-level 的 room.id 校验。改用"启动 task 时捕获 lastObservedRoomId
    /// 作为 streamRoomId，handle 时校验 streamRoomId == lastObservedRoomId"。
    /// 守护语义统一覆盖三类 stale：
    ///   - 已离开房间（streamRoomId=A, lastObservedRoomId=nil）→ 丢
    ///   - A→B 切换（streamRoomId=A, lastObservedRoomId=B）→ 丢
    ///   - 正常匹配 → 通过
    ///
    /// `room.snapshot` 自带 payload.room.id 校验（12.1 r3 落地，更精确），保留原校验路径不依赖
    /// streamRoomId —— 但 streamRoomId 也作为防御层一并传入（不增加错检风险，因为 payload 校验更严格）.
    private func startConsumingMessages() {
        messageConsumerTask?.cancel()
        guard let client = webSocketClient else { return }
        // 捕获启动时刻的 lastObservedRoomId 作为本 stream 的"语义所属房间"。
        // 跨 room 场景下 sink 切换 lastObservedRoomId 后 restart 新 task，新 task 捕获新 streamRoomId；
        // 旧 task 的 streamRoomId 已是旧值，handle 时与 lastObservedRoomId 不匹配 → 守护层挡住。
        let streamRoomId = self.lastObservedRoomId
        messageConsumerTask = Task { [weak self] in
            for await message in client.messages {
                guard let self else { return }
                await MainActor.run {
                    self.handle(message: message, streamRoomId: streamRoomId)
                }
            }
        }
    }

    /// §12.3 client merge contract 实装：snapshot 是 enrich/correct 而非 wipe-out.
    /// 节点 4 阶段（Story 12.1）落地最小路径；自 Story 15.1 起 memberPetStates 切真实值（Epic 14.3 server 端
    /// 已下发真实 `pet.currentState` 1/2/3 → 本 path 解析 + 写入 memberPetStates）：
    ///   - roster 集合层：以 snapshot 的 userId 集合为权威（缺失则移除、新增则 append）
    ///   - 字段级：非空值覆盖、空字符串保留 client 已有值、null 直接覆盖
    ///   - memberPetStates（Story 15.1 解禁）：
    ///       · pet ≠ nil + currentState ∈ {1,2,3} → 写入 `HomePetState(rawValue:)` 映射后的状态
    ///       · pet ≠ nil + currentState 未知值（如 99）→ fallback `.rest` + log warning（防御性，不应发生）
    ///       · pet == nil（pet-less 账号）→ 写入 `.rest` 默认值，让 RoomScaffoldView PetSpriteView 有默认渲染
    ///       · snapshot 中不存在的 userId（已从 roster 移除的成员）→ 从 memberPetStates 中 removeValue
    ///
    /// **fix-review round 3 P1（Story 12.1）**：`.roomSnapshot` 前先校验 `payload.room.id` 与
    /// `lastObservedRoomId` 匹配，不匹配则丢弃 + log debug。
    /// 防 race：用户 leave / room A → B 切换瞬间，前一个 stream 上排队的 `room.snapshot` 可能在
    /// `currentRoomId` 已经变更后才被 deliver，导致 late snapshot for room A repopulate `members`
    /// 而 UI 已经展示 room B（或 no room）.
    /// 校验源用 `lastObservedRoomId`（sink 切换瞬间已经更新成新值）而非现读 `roomId` —— 同一队列上
    /// publisher 通知顺序通常已切到新值，但 lastObservedRoomId 在 sink 内是字段级写入，比 computed
    /// getter 的 appState 读取更稳定（appState 字段也可能在中途被外部 mutate）.
    /// `""` 与 `""` 匹配，`""` 与 `nil` 不匹配 —— 与 HomeRoomDispatcher 把 "" 当 in-room 的语义对齐.
    ///
    /// **fix-review round 2 P1（Story 12.4）**：`member.joined` / `member.left` 改用 `streamRoomId`
    /// 守护防 cross-room race。`streamRoomId` 由 `startConsumingMessages` 在启动 task 时捕获当时
    /// `lastObservedRoomId` 注入；A→B 切换路径 cancel 旧 task + restart 新 task 时旧 task 已
    /// dequeue 但未投递的 message（pending main-actor await）仍会进 handle，但其 `streamRoomId`
    /// 还是旧 room 的值，与新 `lastObservedRoomId` 不匹配 → 丢弃。
    /// 协议层 V1 §12.3 钦定 `member.joined` / `member.left` payload 不含 room.id，无法做 per-event
    /// payload-level 校验，必须用此 stream-lifecycle 层守护.
    ///
    /// **可见性**：`internal` 而非 `private` —— 让 `@testable import PetApp` 测试能直接调用
    /// 验证 streamRoomId 守护契约（cross-room race 在 mock 上 finish-stream 模型下不易构造端到端
    /// 时序，最 robust 的回归是直接调 handle(streamRoomId: <旧值>) + 断言 members 未被 mutate）.
    func handle(message: WSMessage, streamRoomId: String?) {
        switch message {
        case .roomSnapshot(let payload):
            // fix-review round 3 P1：丢弃不属于当前房间的 stale snapshot.
            // 注意：lastObservedRoomId == nil 时（已离开房间）任何 snapshot 都属于 stale.
            // payload-level 校验比 streamRoomId 校验更精确（payload 自带 room.id），保留原路径.
            guard let currentRoomId = lastObservedRoomId, payload.room.id == currentRoomId else {
                os_log(.debug,
                       "RealRoomViewModel: discard stale room.snapshot (payload.room.id=%{public}@, current=%{public}@)",
                       payload.room.id,
                       lastObservedRoomId ?? "<nil>")
                return
            }
            applySnapshot(payload)
        case .memberJoined(let payload):
            // Story 12.4 fix-review round 2 P1：streamRoomId 守护防 cross-room race.
            // V1 §12.3 钦定 `member.joined` payload 不带 room.id（仅 userId / nickname / avatarUrl / pet），
            // server 端按 fanout 范围保证只投递到该房间的 sessions —— 但 client 在 A→B 切换时
            // 旧 consumer task 已 dequeue 的 room A late message 仍可能在 cancel 前 deliver 到 main actor，
            // 此时 lastObservedRoomId 已是 B 但 message 来自 room A 的 stream → streamRoomId（启动时捕获 = A）
            // 与 lastObservedRoomId（= B）不匹配 → 丢弃 + log debug。
            // streamRoomId == nil（已离开房间起的 task；不应发生但防御性兜底）也通过本守护被正确丢弃.
            guard streamRoomId != nil, streamRoomId == lastObservedRoomId else {
                os_log(.debug,
                       "RealRoomViewModel: discard stale member.joined (userId=%{public}@, streamRoomId=%{public}@, current=%{public}@)",
                       payload.userId,
                       streamRoomId ?? "<nil>",
                       lastObservedRoomId ?? "<nil>")
                return
            }
            applyMemberJoined(payload)
        case .memberLeft(let payload):
            // 同 .memberJoined：streamRoomId 守护防 cross-room race.
            guard streamRoomId != nil, streamRoomId == lastObservedRoomId else {
                os_log(.debug,
                       "RealRoomViewModel: discard stale member.left (userId=%{public}@, streamRoomId=%{public}@, current=%{public}@)",
                       payload.userId,
                       streamRoomId ?? "<nil>",
                       lastObservedRoomId ?? "<nil>")
                return
            }
            applyMemberLeft(payload)
        case .connectionStateChanged(let state):
            // Story 12.5：client-internal 状态变更.
            // **不**调 `setCurrentRoomId(_:)` —— wsState 变更不影响 currentRoomId（房间归属仅由
            // HTTP join/leave 改变；V1 §10.5 r3 锁定）.
            // **不**反向命令 client（例如 webSocketClient?.disconnect()）—— 本 case 仅是状态通知.
            //
            // **fix-review round 2 P2**：必须用 streamRoomId 守护防 cross-room race（推翻 dev 阶段开放
            // 问题 §6"不守护"决定）.
            // 触发场景：A→B 房间切换 / leave-rejoin 期间，旧 stream 的 consumer task 可能已 dequeue 一个
            // `.reconnecting` / `.disconnected` 事件，在 lastObservedRoomId 已变更后 apply →
            // 覆盖当前 room 的 status banner（显示前一个连接的 stale 状态）.
            // 与 .memberJoined / .memberLeft 同精神：connection state 也"绑定 specific socket / stream",
            // 跨 stream apply 没有协议上的合理性 —— 必须丢弃.
            guard streamRoomId != nil, streamRoomId == lastObservedRoomId else {
                os_log(.debug,
                       "RealRoomViewModel: discard stale .connectionStateChanged (state=%{public}@, streamRoomId=%{public}@, current=%{public}@)",
                       String(describing: state),
                       streamRoomId ?? "<nil>",
                       lastObservedRoomId ?? "<nil>")
                return
            }
            switch state {
            case .connected:
                self.wsState = .connected
            case .reconnecting:
                self.wsState = .reconnecting
            case .disconnected:
                self.wsState = .disconnected
            }
            os_log(.debug,
                   "RealRoomViewModel: wsState updated from connectionStateChanged → %{public}@",
                   String(describing: self.wsState))
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
    /// level=8 占位, status="在玩耍" 占位, **isHost=false**）—— `level` / `status` / `isHost`
    /// 由 Epic 14 / Epic 8 / 后续 host 字段下发后真实派生；本 story 仅保证 `id` / `name` 与 snapshot 一致.
    /// **节点 4 placeholder 阶段允许 nickname 为空字符串**——按 §12.3 "client merge contract" 空字符串 =
    /// "server 不知道"，应保留 client 已有值；本 story 实装策略（最小路径）：snapshot member.nickname 为空字符串时,
    /// **保留** client 已有同 userId 的 RoomMember.name；新成员（client 没有的 userId）首次出现 nickname 为空字符串时
    /// 降级为 placeholder "成员"（与 ui_design 占位一致；Story 11.7 真实 nickname 落地后即被覆盖）.
    ///
    /// **fix-review round 4 P2**：snapshot path 下 `RoomMember.isHost` 一律置 `false`（"未知 host"占位语义）.
    /// 旧实装用 `isHost = index == 0` 在合法 server state 下产生错误"队长"徽章：
    ///   - 房主离开后房间可继续存在（协议钦定）→ "剩下的第一个成员"会被错误标 isHost
    ///   - 协议明文 client **不能**依赖 member 顺序
    /// 即使作为占位也不能用 index == 0 启发式。等后续 epic snapshot 真带 host 字段时再接.
    /// vm 自身的 `userIsHost`（"我是不是房主"，与 RoomMember.isHost 是两个独立字段）保留构造时
    /// seed 的 RoomScaffoldDefaults.userIsHost 占位 —— 不被 applySnapshot 触碰.
    private func applySnapshot(_ payload: RoomSnapshotPayload) {
        let snapshotUserIds = Set(payload.members.map { $0.userId })
        // step 1: 按 userId 集合做"roster 权威"裁剪 + 增量
        var newMembers: [RoomMember] = []
        for snapshotMember in payload.members {
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
            // isHost 严格 false：snapshot 不带 host 字段时不做位置启发式（详见上方 fix-review round 4 P2 注释）.
            let merged = RoomMember(
                id: snapshotMember.userId,
                name: mergedName,
                level: existing?.level ?? 8,
                status: existing?.status ?? "在玩耍",
                isHost: false
            )
            newMembers.append(merged)
        }
        self.members = newMembers

        // Story 15.1 AC1：snapshot pet.currentState 解析 + memberPetStates 真实写入.
        // 自 Story 14.3 起 server snapshot 真实下发 pet.currentState 1/2/3（Epic 14 落地后 server
        // 端权威等价桶四处全就绪）→ 本 story 在 iOS 端完成解析 + 映射 + 写入 memberPetStates.
        // 规则（与 AC1 钦定一致）：
        //   - pet ≠ nil + HomePetState(rawValue:) 映射成功 → 写入对应状态
        //   - pet ≠ nil + currentState 未知值 → fallback `.rest` + os_log(.error) warning（防御性）
        //   - pet == nil（pet-less 账号）→ 写入 `.rest` 默认值（让 PetSpriteView 有默认渲染）
        //   - snapshot 中不存在的 userId（已从 roster 移除的成员）→ 同步 removeValue
        var newPetStates: [String: HomePetState] = [:]
        for snapshotMember in payload.members {
            let mappedState: HomePetState
            if let pet = snapshotMember.pet {
                if let parsed = HomePetState(rawValue: pet.currentState) {
                    mappedState = parsed
                } else {
                    os_log(.error,
                           "RealRoomViewModel.applySnapshot: unknown pet.currentState=%{public}d for userId=%{public}@; fallback .rest",
                           pet.currentState, snapshotMember.userId)
                    mappedState = .rest
                }
            } else {
                // pet-less 账号兜底（让 PetSpriteView 有默认 .rest 渲染；与 AC1 钦定推荐策略一致）.
                mappedState = .rest
            }
            newPetStates[snapshotMember.userId] = mappedState
        }
        // 用整体替换语义同步 removeValue：snapshot 中不存在的 userId 不会出现在 newPetStates 里 → 自然移除.
        self.memberPetStates = newPetStates
        os_log(.debug,
               "RealRoomViewModel: applied snapshot (members.count = %{public}d, memberPetStates.count = %{public}d)",
               newMembers.count, newPetStates.count)
        _ = snapshotUserIds  // for future use（Story 12.4 增量 mutate 时需要做 set diff）
    }

    // MARK: - Story 12.4 incremental mutate (member.joined / member.left)

    /// V1 §12.3 client merge contract `member.joined` 处理路径（行 2061）.
    ///
    /// **增量 mutate 语义**（**不是** full snapshot replacement）：
    ///   - 分支 (a) roster 中已存在该 userId entry → 字段级 enrich（dedup by userId 防"4 人房间显示 5 个成员"）：
    ///     nickname 非空覆盖，空字符串保留 existing.name；level / status 沿用 existing；isHost 严格 false
    ///   - 分支 (b) roster 中不存在该 userId entry → 新增完整 entry（占位 level=8 / status="在玩耍" / isHost=false）
    ///
    /// **节点 4 阶段实装策略**（与 applySnapshot 主干保持一致）：
    ///   - `level / status / isHost` 沿用 applySnapshot 的占位（默认 8 / "在玩耍" / false）；
    ///     `isHost` 严格 false（fix-review r4 lesson 同精神：不做位置启发式）
    ///   - `nickname` 空字符串场景理论不会发生（V1 §12.3 行 2008 钦定 member.joined nickname 必非空），
    ///     但防御性写：empty string fallback "成员"（与 applySnapshot 同精神）
    ///   - `avatarUrl` 字段当前 RoomMember struct 无对应 field —— 节点 4 阶段不挂入 RoomMember；
    ///     server payload 已有 avatarUrl 但 client 还未渲染头像图（Story 37.13 a11y 表 + Story 30.x 真实
    ///     渲染时挂入），本 story 仅 codec / payload 层透传.
    ///
    /// **Story 15.1 AC1 解禁**：自 Story 14.3 起 server 端 `member.joined.pet.currentState` 真实下发
    /// 1/2/3 → 本路径同 applySnapshot 规则写入 memberPetStates：
    ///   - `payload.pet ≠ nil` + currentState 已知 → 写入对应 HomePetState
    ///   - `payload.pet ≠ nil` + currentState 未知值（如 99）→ fallback `.rest` + log warning
    ///   - `payload.pet == nil` → 写入 `.rest` 默认值（让 PetSpriteView 有默认渲染）
    private func applyMemberJoined(_ payload: MemberJoinedPayload) {
        // 分支 (a)：dedup by userId —— 已存在 entry → 字段级 enrich（不重复 append）
        if let existingIndex = members.firstIndex(where: { $0.id == payload.userId }) {
            let existing = members[existingIndex]
            // nickname 非空覆盖；空字符串保留 existing.name（防御性，理论不应发生）
            let mergedName: String = payload.nickname.isEmpty ? existing.name : payload.nickname
            // level / status 沿用 existing（applyMemberJoined 不应回退已有 level / status）；isHost 严格 false
            let merged = RoomMember(
                id: existing.id,
                name: mergedName,
                level: existing.level,
                status: existing.status,
                isHost: false
            )
            members[existingIndex] = merged
            // Story 15.1 AC1：同步写入 memberPetStates（即便 entry 已存在；pet.currentState 可能变化）.
            applyJoinedPetState(userId: payload.userId, pet: payload.pet)
            os_log(.debug,
                   "RealRoomViewModel: applyMemberJoined enriched existing entry (userId=%{public}@)",
                   payload.userId)
            return
        }
        // 分支 (b)：新增完整 entry（占位 level=8 / status="在玩耍" / isHost=false）
        let newName: String = payload.nickname.isEmpty ? "成员" : payload.nickname
        let newMember = RoomMember(
            id: payload.userId,
            name: newName,
            level: 8,                  // 节点 4 阶段占位（与 applySnapshot 一致）
            status: "在玩耍",           // 节点 4 阶段占位（与 applySnapshot 一致）
            isHost: false              // 严格 false（fix-review r4 lesson 同精神）
        )
        members.append(newMember)
        // Story 15.1 AC1：同步写入 memberPetStates（与 applySnapshot 同语义；pet-less / 未知值兜底 .rest）.
        applyJoinedPetState(userId: payload.userId, pet: payload.pet)
        os_log(.debug,
               "RealRoomViewModel: applyMemberJoined appended new entry (userId=%{public}@, members.count=%{public}d)",
               payload.userId, members.count)
    }

    /// Story 15.1 AC1: applyMemberJoined 内部 helper —— 解析 MemberJoinedPet.currentState 写入
    /// memberPetStates；与 applySnapshot 同规则（pet-less 与未知值都兜底 `.rest`）.
    private func applyJoinedPetState(userId: String, pet: MemberJoinedPet?) {
        let mappedState: HomePetState
        if let pet = pet {
            if let parsed = HomePetState(rawValue: pet.currentState) {
                mappedState = parsed
            } else {
                os_log(.error,
                       "RealRoomViewModel.applyMemberJoined: unknown pet.currentState=%{public}d for userId=%{public}@; fallback .rest",
                       pet.currentState, userId)
                mappedState = .rest
            }
        } else {
            mappedState = .rest
        }
        memberPetStates[userId] = mappedState
    }

    /// V1 §12.3 client merge contract `member.left` 处理路径（行 2099）.
    ///
    /// **增量 mutate 语义**：从 roster 中**移除** payload.userId 对应 entry.
    ///
    /// **关键约束**：roster 中不存在该 userId 时 → ignore + log warning；
    /// **禁止**抛错 / **禁止**因找不到 entry 把 vm.members 整体清空（防御性"诡异"server state 下不至于把
    /// 整个房间清空；server bug 路径下 client 不应放大破坏）.
    /// 必须先 firstIndex 校验存在性 + log warning，**禁止**用 `members.removeAll(where:)` 吞掉信号.
    private func applyMemberLeft(_ payload: MemberLeftPayload) {
        guard let index = members.firstIndex(where: { $0.id == payload.userId }) else {
            os_log(.error,
                   "RealRoomViewModel: applyMemberLeft userId not in members (userId=%{public}@, members.count=%{public}d)",
                   payload.userId, members.count)
            return
        }
        members.remove(at: index)
        // memberPetStates 节点 4 阶段保持空 map；本 story 不动主干语义。
        // 但若未来 Epic 14 落地时该 user 的 memberPetStates entry 已写入，**应**同步 remove —— 本 story
        // 提前预埋安全语义：从 memberPetStates 也 remove 该 userId（节点 4 阶段空 map 下 no-op，但
        // Epic 14 后真实驱动后该路径自动正确）.
        memberPetStates.removeValue(forKey: payload.userId)
        os_log(.debug,
               "RealRoomViewModel: applyMemberLeft removed entry (userId=%{public}@, members.count=%{public}d)",
               payload.userId, members.count)
    }

    // MARK: - override abstract methods

    public override func onLeaveTap() {
        // Story 12.7 AC6: 调 LeaveRoomUseCase（HTTP 200 / 6004 视同成功 → setCurrentRoomId(nil)）.
        // fallback 路径：若 useCase == nil（RootView 老 wire / UITest 不注入），保留旧 mock 行为
        //   `appState?.setCurrentRoomId(nil)` —— 让 onLeaveTap 在没有 server 的场景下仍能让 UI 切回 idle.
        guard let useCase = self.leaveRoomUseCase else {
            os_log(.debug, "RealRoomViewModel.onLeaveTap (fallback: no LeaveRoomUseCase, set currentRoomId nil directly)")
            self.appState?.setCurrentRoomId(nil)
            return
        }
        let presenter = self.errorPresenter
        // Story 12.7 r10 [P2] fix（codex review）：升级到 `roomNavigationGeneration` token ——
        // r5 旧 `currentRoomId == target` guard 无法区分 ABA cycle（leave A in-flight → re-join A → A 的
        // leave error 迟到 → liveRoomId == "A" == target → stale alert 弹在刚 rejoin 的 A 上）.
        //
        // 场景可达：tab bar 仍可用 → 用户 leave A in-flight 时切 tab → join B → leave B → re-join A →
        // 原 leave A 的 1009/network 抛错迟到 → 老实装 catch 块**无条件** present global error → stale
        // error overlay 出现在新 A session 上.
        //
        // `DefaultLeaveRoomUseCase` 已经 guard 200 / 6004 stale-response（r10 P2 generation fix）；
        // thrown-error 路径同语义补齐 —— generation mismatch 时静默 skip + log debug，不抛错也不弹 UI.
        // 详见 docs/lessons/2026-05-11-room-navigation-generation-token-not-room-id-equality.md.
        let entryGen = self.appState?.roomNavigationGeneration ?? 0
        let refreshHome = self.refreshHomeOnStaleNavigation
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                try await useCase.execute()
                // 成功路径 / 6004 视同成功：UseCase 已写 setCurrentRoomId(nil) → subscribeRoomIdConnect
                // 的 A → nil 分支自动触发 disconnect + 清 roster + wsState = .disconnected →
                // HomeContainerView 自动切回 HomeView.
            } catch is RoomNavigationStaleError {
                // r14 [P1] fix（codex review）：UseCase 检测到 navigation race（server 端已让用户离开 room 但
                // client UI 因 stale guard 没写 nil；用户可能已 rejoin 同房或切到别处）→ silent skip +
                // 触发 home refresh 拿 authoritative state.
                os_log(.info,
                       "RealRoomViewModel.onLeaveTap: caught RoomNavigationStaleError; trigger home refresh to reconcile authoritative state")
                refreshHome?()
            } catch {
                // r10 [P2] fix: guard generation 一致 防 stale 错误 overlay 弹到新 navigation cycle 之上.
                let liveGen = self.appState?.roomNavigationGeneration ?? 0
                guard liveGen == entryGen else {
                    os_log(.info,
                           "RealRoomViewModel.onLeaveTap: stale leave error (entryGen=%{public}d, currentGen=%{public}d); skip errorPresenter to avoid overlay on unrelated navigation cycle",
                           entryGen, liveGen)
                    return
                }
                // 其他 APIError 透传给 ErrorPresenter（默认 mapper 路径，alert / retry）.
                // appState 保留原值（用户仍在 RoomView 内可重试）.
                os_log(.error, "RealRoomViewModel.onLeaveTap LeaveRoomUseCase error: %{public}@",
                       String(describing: error))
                presenter?.present(error)
            }
        }
    }

    public override func onCopyTap() {
        os_log(.debug, "RealRoomViewModel.onCopyTap")
        // 实际 UIPasteboard 复制由 RoomScaffoldView 内 SwiftUI @State + 调用本方法时一起触发（Story 37.8 落地）.
    }

    deinit {
        messageConsumerTask?.cancel()
    }
}
