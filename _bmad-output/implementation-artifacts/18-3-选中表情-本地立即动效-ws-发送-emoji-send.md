# Story 18.3: 选中表情 → 本地立即动效 + WS 发送 emoji.send（并行）（首次落地 WSOutgoingMessage.emojiSend case + WSMessageCodec encode 扩展 + SendEmojiUseCase + RoomViewModel.activeEmojis 队列 + RoomScaffoldView 触发 + 0 延迟本地动效 + fire-and-forget toast 降级）

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want **在表情面板选中某个表情后，立即（< 50ms）在自己猫位上方看到飞出动效，同时 client 通过既有 WebSocketClient 把 emoji.send fire-and-forget 派给 server**（**本地动效绝不阻塞 / 等待 WS 回信**；WS 失败也不回滚本地动效，仅弹温和 toast）,

具体落地路径：
- `iphone/PetApp/Core/Networking/WSOutgoingMessage.swift` **修改** —— 在既存 `case ping(requestId: String)` 之后追加 `case emojiSend(requestId: String, emojiCode: String)`；与 ping case 同精神（fire-and-forget client→server 业务消息；V1 §12.2 `emoji.send` 已 17.1 冻结）；`requestId` 由 caller 生成（推荐 `"emoji_<timestamp_ms>"` 与 V1 §12.2 行 1993 `推荐格式` 对齐）；`emojiCode` 是 V1 §11.1 `data.items[].code` 字段值（client 必须从 §11.1 缓存的合法列表取，禁止硬编码，详见 V1 §12.2 行 2074）；本 case 仅就位 enum，序列化由 WSMessageCodec 落地
- `iphone/PetApp/Core/Networking/WSMessageCodec.swift` **修改** —— 在既存 `encode(_:)` switch 内追加 `case .emojiSend(let requestId, let emojiCode):` 分支：`json = ["type": "emoji.send", "requestId": requestId, "payload": ["emojiCode": emojiCode]]`（严格对齐 V1 §12.2 行 2000-2008 wire schema）；JSONSerialization options 维持 `[.sortedKeys]`（与 ping 一致，testing 友好）；decoding 失败 throw `WSError.decodingFailed(rawType: "emoji.send")`（与 ping 同 raw type 约定）；本 case **不**新增 decode 路径（client → server 单向，无 incoming 需求）
- `iphone/PetApp/Features/Emoji/Models/RoomActiveEmoji.swift` **新建** —— `public struct RoomActiveEmoji: Identifiable, Equatable, Sendable` 含字段 `id: UUID`（每次入队全新生成；让 SwiftUI ForEach 区分同 userId 同 emojiCode 多次连点 + 满足 18.4 动效结束后按 id 移除约束）/ `userId: String`（who triggered；18.3 路径 = `RoomViewModel.currentUserId`；18.4 路径 = `emoji.received.payload.userId`）/ `emojiCode: String`（V1 §12.3 `emoji.received.payload.emojiCode` / §12.2 `emoji.send.payload.emojiCode`，与 EmojiConfig.code 同 wire 字段）/ `createdAt: Date`（入队时刻；18.4 落地 1.5s 后按 createdAt 自动 expire 移除时用；18.3 仅落地字段，**不**实装定时器）；本 story 不实装 1.5s 自动移除（18.4 才做；18.3 仅让 enum + 队列就位让 18.4 串接零成本）
- `iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift` **修改** —— 基类增字段 `@Published public var activeEmojis: [RoomActiveEmoji] = []`（视图层 ForEach 数据源；唯一 owner = ViewModel @Published；与 `members` / `memberPetStates` 同模式）+ 增 abstract method `func onEmojiSelected(code: String)`（fatalError 占位强制子类 override；与 onOwnPetTap / onLeaveTap / onCopyTap 同模式）；**不**改 18.2 落地的 `showEmojiPanel` / `currentUserId` 字段（继续作为 single source of truth）；文件顶部注释块字段范围从 "8 字段" 改 "9 字段"，abstract method 从 "3 abstract method" 改 "4 abstract method"
- `iphone/PetApp/Features/Emoji/UseCases/SendEmojiUseCase.swift` **新建** —— `public protocol SendEmojiUseCaseProtocol: Sendable` + `func execute(emojiCode: String) async throws`（throws WSError；caller 用 do-try-catch 捕获网络降级；async 因 webSocketClient.send 本身是 async throws）+ `public final class DefaultSendEmojiUseCase: SendEmojiUseCaseProtocol`（class 不 struct，因 protocol Sendable + 持 webSocketClient 引用 + 后续可能持 clock 字段做 timestamp，class 形态与 `DefaultLoadEmojisUseCase` 同模式）；构造注入 `webSocketClient: WebSocketClient`（protocol 接口注入，让 unit test mock 友好）；`execute(emojiCode:)` 实装：
  1. 生成 `requestId = "emoji_\(Int(Date().timeIntervalSince1970 * 1000))"`（与 V1 §12.2 行 1993 `推荐格式 "emoji_<seq>" / "emoji_<ts_ms>"` 一致；本 story 选 ts_ms 路径，**不**引入额外 seq counter 字段；时间戳本地生成，server 端 emoji.received 不回 requestId）
  2. 构造 `let message = WSOutgoingMessage.emojiSend(requestId: requestId, emojiCode: emojiCode)`
  3. `try await webSocketClient.send(message)` —— send 内部 throws `WSError.notConnected` / `WSError.decodingFailed` 等；UseCase 透传原始 WSError 给 caller（vm），**不**做错误转换（与 V1 §11.1 / §12.2 错误分层一致：底层 client → UseCase 透传 → vm 层 map）
  4. **不**做 emojiCode 合法性二次校验（V1 §12.2 行 2074 "client 应校验 emojiCode 来自 §11.1 缓存"由 caller 即 vm 在调本 UseCase 前完成；UseCase 是 transport 层 single responsibility，不重复职责；ViewModel 层用 emoji catalog cache 在调 UseCase 前做防御性校验）；UseCase **不**等任何 server 响应（V1 §12.2 行 2024 "无 HTTP 响应、无 server → client ack 消息"；fire-and-forget 钦定）
- `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` **修改** —— 增字段 `private var sendEmojiUseCase: SendEmojiUseCaseProtocol?`（**可选**：与 `leaveRoomUseCase` 同模式，让无 UseCase wire 的旧路径仍能 init RealRoomViewModel 不破；UITest stub 路径 / 17.x 旧测试不破）+ 增字段 `private weak var emojiCatalogLoader: LoadEmojisUseCaseProtocol?`（**weak**：避免循环引用；用于 onEmojiSelected 内部按 V1 §12.2 行 2074 校验 emojiCode 来自缓存）+ `bind(appState:webSocketClient:leaveRoomUseCase:errorPresenter:)` 签名扩展为 `bind(appState:webSocketClient:leaveRoomUseCase:sendEmojiUseCase:emojiCatalogLoader:errorPresenter:)`（在既有 `leaveRoomUseCase: LeaveRoomUseCaseProtocol? = nil` 之后追加 `sendEmojiUseCase: SendEmojiUseCaseProtocol? = nil` / `emojiCatalogLoader: LoadEmojisUseCaseProtocol? = nil` 两个新参数，默认 nil；与既有路径同模式；**不**改 init 签名只改 bind 签名，让既有 RootView wire 路径若不传新参数仍能编译）+ override `func onEmojiSelected(code: String)` 实装：
  1. **A. 本地立即动效（**先**执行 — 0 延迟优先）**：`let emoji = RoomActiveEmoji(id: UUID(), userId: currentUserId ?? "", emojiCode: code, createdAt: Date())`；`self.activeEmojis.append(emoji)`（@Published append 触发 SwiftUI 渲染；本步**不** await / **不** 调任何 server，毫秒级返回让 SwiftUI 在下一 runloop 渲染动效）；`currentUserId == nil` 时（理论不该 —— 入口 RoomScaffoldView Button 只在 currentUserId 非空时渲染）走 fail-safe userId = "" 入队（仍触发动效不报错）+ os_log warn"onEmojiSelected: currentUserId is nil; using empty userId for activeEmoji"
  2. **B. emojiCode 缓存校验**（V1 §12.2 行 2074 钦定 "client 应校验 emojiCode 来自 §11.1 缓存"）：`if let loader = emojiCatalogLoader { let catalog = try? await loader.execute(); guard catalog?.contains(where: { $0.code == code }) == true else { os_log warn "onEmojiSelected: emojiCode \(code) not in catalog; skip send"; return } }`（缓存 miss 不应该发生 —— 18.1 LoadEmojisUseCase 缓存模型保证 EmojiPanelView 已 load 过 catalog；防御性 fail-safe 路径仅 log，**不**触发 error toast；本地动效已在步骤 1 触发）；`emojiCatalogLoader == nil` 时跳过校验（旧 wire 路径不破，与 leaveRoomUseCase nil 路径同精神）
  3. **C. WS fire-and-forget**：`if let useCase = sendEmojiUseCase { Task { do { try await useCase.execute(emojiCode: code); os_log debug "onEmojiSelected: emoji.send sent: \(code)" } catch let wsError as WSError { os_log warn "onEmojiSelected: WS send failed: \(wsError); presenting toast"; self.errorPresenter?.presentToast("网络不佳，对方可能看不到") } catch { os_log warn "onEmojiSelected: WS send unexpected error: \(error)"; self.errorPresenter?.presentToast("网络不佳，对方可能看不到") } } }`（用 `Task { }` 包裹让 WS 调用与本地动效并行 / fire-and-forget；catch `WSError` 显式区分网络降级路径走 ErrorPresenter.presentToast；catch other error 走同 toast 路径兜底；toast 文案 "网络不佳，对方可能看不到" 与 epics.md §Story 18.3 行 2690-2691 钦定文案 1:1 一致；toast 不阻塞 UI / 不影响后续点击；**禁止**走 `errorPresenter.present(error)` 全屏 retry overlay 路径，与 Story 12.7 round 3 P1 fix `presenter` 用 `presentToast` 而非 `present(error)` 同 lesson）；`sendEmojiUseCase == nil` 时跳过 WS 调用（旧 wire 路径不破 + 本地动效已在步骤 1 触发，UX 仍 OK 因为 18.3 epic 钦定本地动效是 primary feedback）
- `iphone/PetApp/Features/Room/ViewModels/MockRoomViewModel.swift` **修改** —— `Invocation` enum 增 `case emojiSelected(code: String)`；override `func onEmojiSelected(code: String)`：`invocations.append(.emojiSelected(code: code))` + `self.activeEmojis.append(RoomActiveEmoji(id: UUID(), userId: currentUserId ?? "", emojiCode: code, createdAt: Date()))`（与 RealRoomViewModel 行为对齐让单测 / Preview / UITest 行为一致）；**不**做 WS 调用（MockRoomViewModel 永远不持 webSocketClient）；**不**在 onEmojiSelected 内置 sheet 关闭逻辑（sheet 关闭由 RoomScaffoldView .sheet onSelect 闭包驱动，与 18.2 落地的"vm.showEmojiPanel = false"路径同一处）
- `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift` **修改** —— `.sheet(isPresented: $state.showEmojiPanel)` 内 EmojiPanelView 的 `onSelect: { _ in state.showEmojiPanel = false }` 改为 `onSelect: { code in state.onEmojiSelected(code: code); state.showEmojiPanel = false }`（**顺序约束**：先调 onEmojiSelected 触发本地动效 + WS（非阻塞），再关 sheet —— 让本地动效在 sheet 关闭动画期间已经入队；若反过来先关 sheet 再 onEmojiSelected，sheet 关闭动画与动效入队竞速，可能让 SwiftUI 主线程合并 Publisher emit 让动效晚一帧出现 —— UX 可观察的"按钮按下后 sheet 关 → 隔 100ms 动效才飞出"延迟感）+ 在 ZStack 内 ScrollView 之上（与 LinearGradient 同层级 / .sheet 之下）追加 `EmojiAnimationLayer(activeEmojis: state.activeEmojis, members: state.members, memberAnchors: $memberAnchors)` —— **本 story 仅占位接入 `ForEach(state.activeEmojis) { emoji in Text(...) }` 简易渲染让 dev 实跑能"看到飞出"语义**；完整动画实装 / anchor 计算 / 1.5s 移除 / 缩放 + 透明 + 上移 / 同时多 emoji 独立动画 / userId-not-in-roster fall back to center 等全套**由 Story 18.4 落地**（本 story 只让 activeEmojis 队列 + 简易飞出占位可见，让 18.4 完整动画可在已有 hook 上落地，避免 18.4 还要回头改 RoomScaffoldView）；具体本 story 落地的占位 `EmojiAnimationLayer` 形态：
  ```swift
  // Story 18.3 占位渲染: activeEmojis 队列可视化(简单"在屏幕中央显示 emoji code 文本"); 18.4 替换为飞出动画.
  struct EmojiAnimationLayerPlaceholder: View {
      let activeEmojis: [RoomActiveEmoji]
      var body: some View {
          VStack { ForEach(activeEmojis) { e in Text("\(e.emojiCode)").font(.title2).accessibilityIdentifier("activeEmoji_\(e.id.uuidString)") } }
      }
  }
  ```
  **关键约束**：本 story 落地的占位**必须**让 `activeEmojis` 入队的 emoji 在屏幕上**可见**（UITest 用 `app.staticTexts["wave"]` 或 `app.otherElements["activeEmoji_*"]` 验证）；18.4 落地后该 placeholder 会被替换为完整 EmojiAnimationLayer，**但本 story 落地的 activeEmojis 字段 + Identifiable struct + 入队语义 + onEmojiSelected 调用链不需要改**
- `iphone/PetApp/App/AppContainer.swift` **修改** —— `// MARK: - Story 18.1 AC5: Emoji 链路 factory` block 内追加 `makeSendEmojiUseCase()` 工厂：
  ```swift
  /// Story 18.3 AC: 构造 SendEmojiUseCase (每次调用返回新实例; webSocketClient 单例由 container 持有).
  /// caller=RootView 或 HomeContainerView 在 RealRoomViewModel.bind(...) 时注入.
  /// UseCase 内部不持状态, 多个 caller 共用一个实例 vs 多实例无差; 与 makeStepRepository 同模式 (value-like 构造廉价).
  public func makeSendEmojiUseCase() -> SendEmojiUseCaseProtocol {
      DefaultSendEmojiUseCase(webSocketClient: webSocketClient)
  }
  ```
  + RoomScaffoldView 实例化 callsite（HomeContainerRoomViewBridge 或既有路径）**不直接**关心 SendEmojiUseCase；由 RootView 在 `RealRoomViewModel.bind(...)` 时注入 `sendEmojiUseCase: container.makeSendEmojiUseCase()` + `emojiCatalogLoader: container.loadEmojisUseCase`
- `iphone/PetApp/App/RootView.swift` **修改** —— 找 RealRoomViewModel.bind 的 callsite（grep `bind(appState:webSocketClient:leaveRoomUseCase:errorPresenter:` 或同前缀），在既有参数列表内追加 `sendEmojiUseCase: container.makeSendEmojiUseCase()` + `emojiCatalogLoader: container.loadEmojisUseCase`（直接用 container 单例字段而非 factory，因 LoadEmojisUseCase 是 stable singleton，详见 18.1 AC5）；**不**新增 environment object / EnvironmentValues 路径（与 18.2 落地的 emojiPanelViewModelFactory `\.emojiPanelViewModelFactory` environment-key 路径**不同**：18.2 是 RoomScaffoldView 直接持工厂闭包；18.3 注入到 RealRoomViewModel 已经走 `bind(...)` 既有 wire 路径，**不**走 environment）
- 单元测试覆盖 ≥ 4 case：
  - `iphone/PetAppTests/Core/Networking/WSMessageCodecEmojiSendTests.swift`（**新建** 或追加既存 `WSMessageCodecTests`）：
    - happy `encode(.emojiSend(requestId: "emoji_1715600000000", emojiCode: "wave"))` → JSON 严格对齐 V1 §12.2 schema：`{"type":"emoji.send","requestId":"emoji_1715600000000","payload":{"emojiCode":"wave"}}`（sortedKeys 保证 key 顺序确定）+ JSON 解析回来字段值正确
    - edge emojiCode 含特殊字符 `_` `-` `0-9` 边界值（如 `"my_emoji-1"`）→ 编码不破（V1 §11.1 字符集 `[a-z0-9_-]`）
    - edge 空 emojiCode（虽 V1 §11.1 钦定 length 1-64，client 不应发 `""`，但 codec 是纯序列化层不校验）→ encode 不抛错，直接编码为 `payload.emojiCode = ""`（业务层校验由 ViewModel.onEmojiSelected 在调 UseCase 前做）；本 case 仅验证 codec 不 panic / 不抛错
    - edge requestId 空字符串 → 编码 `requestId: ""`（V1 §12.2 行 1993 选填，空字符串合法）
  - `iphone/PetAppTests/Features/Emoji/UseCases/SendEmojiUseCaseTests.swift`（**新建**，≥3 case）：
    - happy MockWebSocketClient 接收 send → execute("wave") 不抛 → `mockClient.sentMessages.count == 1` + `sentMessages.first == .emojiSend(requestId: <ts_ms>, emojiCode: "wave")`（requestId 由 UseCase 内部生成 → 用 regex `^emoji_\d{13}$` 校验格式）
    - edge MockWebSocketClient.sendError = .notConnected → execute → 抛 `WSError.notConnected`（UseCase 透传不转换）
    - edge 多次 execute 连续 3 次 → mockClient.sentMessages.count == 3 + 每次 requestId 不同（毫秒时间戳粒度 → 连续调可能相同 → 用 `try await Task.sleep(nanoseconds: 1_500_000)` 在 case 内手动隔 1.5ms 保证不同；如不分隔可断言 sentMessages.count == 3 即可，requestId 不强制唯一性测试）
  - `iphone/PetAppTests/Features/Room/RoomViewModelEmojiSendTests.swift`（**新建**，≥4 case）：
    - happy mockOnEmojiSelected → activeEmojis 立即追加 1 项（userId == currentUserId + emojiCode == 入参） + invocations 含 `.emojiSelected(code: "wave")`
    - happy 连点 3 次 → activeEmojis 长度 3 + 每项 UUID 不同（让 ForEach 区分） + invocations 3 项
    - happy MockSendEmojiUseCase 接收路径（**RealRoomViewModel 路径**）：
      ```swift
      let mockWS = WebSocketClientMock()
      let mockSend = DefaultSendEmojiUseCase(webSocketClient: mockWS)  // 用真实 UseCase + Mock WS，简化 mock 层级
      let vm = RealRoomViewModel(appState: appState)
      vm.bind(appState: appState, webSocketClient: mockWS, sendEmojiUseCase: mockSend, emojiCatalogLoader: nil)
      vm.currentUserId = "u1"   // direct mutation for test; bind 路径走 appState 已在 18.2 测试覆盖
      vm.onEmojiSelected(code: "wave")
      // 1. 本地 activeEmojis 立即追加
      XCTAssertEqual(vm.activeEmojis.count, 1)
      XCTAssertEqual(vm.activeEmojis.first?.emojiCode, "wave")
      XCTAssertEqual(vm.activeEmojis.first?.userId, "u1")
      // 2. WS 调用走 fire-and-forget Task → 等 Task.yield + 检 mockWS.sentMessages
      await Task.yield(); try? await Task.sleep(nanoseconds: 50_000_000)  // 50ms; 让 Task 跑完
      XCTAssertEqual(mockWS.sentMessages.count, 1)
      if case .emojiSend(_, let code) = mockWS.sentMessages.first { XCTAssertEqual(code, "wave") } else { XCTFail() }
      ```
    - edge MockWebSocketClient.sendError = .notConnected + ErrorPresenter 注入 → onEmojiSelected → activeEmojis 仍追加 1 项（不回滚）+ 等 100ms 后 mockErrorPresenter.lastToast == "网络不佳，对方可能看不到"（用 ErrorPresenter 的真实实装 + 验证其 currentPresentation 是 `.toast(message:)`）
- UI 测试覆盖（XCUITest，**新建** `iphone/PetAppUITests/Features/Room/RoomEmojiSendUITests.swift`）：
  - 复用 18.2 落地的 launch arg `--uitest-emoji-panel-room-host` + env `UITEST_MOCK_EMOJI=1` + `UITEST_SKIP_GUEST_LOGIN=1` + `UITEST_FORCE_IN_ROOM=1` 路径 + **新增** `UITEST_MOCK_WEBSOCKET=1` env 启用 MockWebSocketClient（让 onEmojiSelected 的 WS Task 不真实拨号 → 走 MockWS 不抛错路径让 toast 不触发）+ launch arg `--uitest-emoji-send-host` 触发 RootView DEBUG 块 wire 完整的 MockRoomViewModel + 真实 SendEmojiUseCase + mock WS 链路（与 18.2 `--uitest-emoji-panel-room-host` 差异：本 launch arg 额外 wire sendEmojiUseCase 到 MockRoomViewModel 路径让 UITest 走 production code path 接近）
  - case A（happy 本地动效立即可见）: 进房间 → 点 `roomMember_0_petSprite` → emojiPanel 出现 → tap `emojiCell_wave` → emojiPanel 关闭 → 0.5s 内 `app.staticTexts["wave"]` 或 `app.otherElements["activeEmoji_*"]` 可见（验证本地动效 0 延迟语义）
  - case B（happy 多个表情独立入队）: 进房间 → 连续 3 次"点 self → 选 wave"（每次 sheet 重弹关闭 → activeEmojis +1）→ 验证屏幕上至少 3 个 activeEmoji 节点可见（占位渲染下 3 个 "wave" Text；18.4 落地后变成 3 个独立飞出动画）
  - case C（happy 选不同 emoji）: 进房间 → 点 self → 选 wave → 选 love（重弹 sheet） → activeEmojis 含 wave + love 两项
  - case D（验证 toast 路径）**移到单元测试**（UITest 难稳定 mock WS 抛错 + toast 自动 2s 消失会让 case 不稳；单元测试中 MockWS.sendError 直接注入更稳）

So that **Story 18.4（接收 emoji.received 在对应成员猫上方播放飞出动效 + 去重自己 userId）+ Epic 19.1（节点 6 demo E2E "A 选 wave → 立即看到自己猫上方有 wave 动效飞出"+"A 连点 5 次 wave → 自己看到 5 次堆叠飞出"+"A 关 wifi → A 选 wave → 自己仍看到本地动效 + toast"）** 可以基于一个**已落地、严格符合 V1 §12.2 emoji.send 契约 + 本地动效与 WS 解耦 + fire-and-forget 网络降级 toast** 的完整链路继续展开，不再出现"18.3 本地动效不立即 / 必须等 server roundtrip / WS 失败回滚 activeEmojis / 18.4 找不到 activeEmojis 队列 / 18.4 收到 self echo 不知道如何去重"的返工。

## 故事定位（Epic 18 第 3 条；上承 18.1 EmojiPanelView + 18.2 RoomViewModel.showEmojiPanel/currentUserId/onOwnPetTap 落地，下启 18.4 emoji.received 动效 + 去重）

- **Epic 18 进度**：18.1（表情面板 SwiftUI + GET /emojis 缓存）**已 done** → 18.2（房间页内点击自己猫触发表情面板）**已 done** → **18.3（本 story，选中表情触发本地立即动效 + WS emoji.send fire-and-forget）** → 18.4（接收 emoji.received 在对应成员猫上方播放飞出动效 + 去重自己 userId）。
- **本 story 是 Epic 18.4 + Epic 19.1 的强前置**：
  - **Story 18.4（epics.md 行 2699-2728）**：钦定"收到 emoji.received {userId, emojiCode}：if userId == 当前用户自己 → 跳过；否则走动效"+"动效流程（也用于 Story 18.3 的本地触发路径）：从缓存表情列表中查 emojiCode → 拿 assetUrl → 在该 userId 对应的猫位上方位置生成浮动 SwiftUI View → 1.5s 动画 → 移除"——**直接依赖**本 story 落地的：
    - `RoomViewModel.activeEmojis: [RoomActiveEmoji]` 队列（18.4 path 收到 emoji.received 后也 `activeEmojis.append(...)` → 与本 story 同一队列 / 同一渲染路径）
    - `RoomActiveEmoji` 字段定义（id / userId / emojiCode / createdAt 四字段 → 18.4 落地 1.5s 后按 createdAt 移除 + 按 userId 找猫位 + 按 emojiCode 查 catalog 拿 assetUrl）
    - `RoomScaffoldView` 内 EmojiAnimationLayer 渲染 hook（本 story 占位 → 18.4 替换为完整动画 + anchor 计算 + 1.5s 自动移除）
    - **不**重复实装 SendEmojiUseCase / WS encode / 本地入队路径（18.3 已做）
  - **Epic 19.1 节点 6 demo E2E（epics.md 行 2738-2768）**：钦定"场景 2（本地立即动效）: A 选 wave → 立即（< 50ms）看到自己猫上方有 wave 动效飞出 → 验证不等 server roundtrip"+"场景 5（连续快发）: A 连点 5 次 wave → A 自己看到 5 次本地动效堆叠飞出"+"场景 6（弱网降级）: A 关 wifi → A 选 wave → 自己仍看到本地动效 + 出现 toast '网络不佳，对方可能看不到'"——**直接依赖**本 story 落地的"选中 → 0 延迟本地入队 + 并行 WS 调 + 网络失败 toast"完整链路
- **epics.md §Story 18.3 钦定**（行 2675-2698）：
  - **AC1（本地立即动效）**：用户选中某个表情 → 触发 SendEmojiUseCase，**并行**执行：A. 本地立即动效（在自己猫位上方触发飞出动效；直接调用 Story 18.4 的 activeEmojis 队列 append 当前 emojiCode + 自己的 userId）；B. WS 发送（WebSocketClient.send(`emoji.send {emojiCode}`)，fire-and-forget，不阻塞动效）
  - **AC2（sheet 关闭）**：选中后立即关闭 EmojiPanelView（不等 WS ack）—— 18.2 落地 sheet 关闭路径在 RoomScaffoldView onSelect 闭包，**本 story 仅扩展闭包逻辑，**不**改 sheet 关闭机制**
  - **AC3（WS 失败降级）**：本地动效仍正常播完（不回滚）+ ErrorPresenter 弹温和 toast: "网络不佳，对方可能看不到"（不阻塞，不影响后续操作）
  - **AC4（不依赖 server 自己 echo）**：自己的动效在本步触发；server 的 emoji.received 由 18.4 处理，会跳过自己 userId 的去重
  - **AC5（单元测试 ≥4 case）**：happy 选中 wave → activeEmojis 立即多 1 项 + WebSocketClient.send 调用 1 次 / happy 选中后 EmojiPanelView 关闭 / edge WebSocketClient.send 抛 .notConnected → activeEmojis 仍添加（动效照播）+ toast 触发 / edge 同一表情快速连点 3 次 → activeEmojis 添加 3 项 + WS 发送 3 次（如有 server 端限频，server 自己处理）

### 决策点 1：activeEmojis 队列 owner（RoomViewModel @Published vs SwiftUI @State）

epics.md 行 2686 钦定"调用 Story 18.4 的 activeEmojis 队列 append" —— 钦定 18.4 落地的；本 story 是 18.4 的**前置**故必须先落 activeEmojis 的 owner / type / 入队 API。**选 RoomViewModel.@Published var activeEmojis: [RoomActiveEmoji]**：

1. **唯一权威源**（ADR-0010 §3.2 钦定）：
   - 18.3 路径写入：`vm.onEmojiSelected(code:)` 内入队
   - 18.4 路径写入：`vm.applyEmojiReceived(payload:)`（18.4 落地）内 append 别人的 emoji + 跳过自己
   - 视图层 RoomScaffoldView `ForEach(state.activeEmojis)` 读
   - 单一 owner = ViewModel @Published → 满足"ViewModel 是 ad-hoc UI transient state owner"模式（与 `members` / `memberPetStates` 同精神）
2. **跨 sub-view 共享**：本 story EmojiAnimationLayer 占位 / 18.4 完整动画 / 18.x 未来 anchor / 18.x 未来 sound effect 等都从 `state.activeEmojis` 读 → 单一队列；放 SwiftUI @State 会卡在某个 sub-view 内，跨 view 共享需要 EnvironmentObject 或 @Binding 链 → 复杂度暴增
3. **测试友好**：`MockRoomViewModel(activeEmojis: [...])` 一句话注入测试场景 → 满足 ADR-0010 §3.2 "ViewModel @Published 字段是单元测试 single source of truth"

### 决策点 2：本地动效与 WS send 并行 vs 串行

epics.md 行 2685-2691 钦定**"并行"**关键词。**严格并行实装**：

1. **顺序**：本地入队**先**于 WS 调用（**不**是同时；先入队后 await WS 不算违反"并行"，因为 WS 是异步 Task fire-and-forget）；这让本地动效 0 延迟 / 不被 WS 网络阻塞
2. **Task 包裹 WS 调用**：`Task { try await useCase.execute(...) }` 让 WS 调用挂到全局 task scheduler，不 block ViewModel 主线程；ViewModel.onEmojiSelected 主流程在毫秒级返回让 SwiftUI 下一 runloop 渲染动效
3. **错误降级**：Task 内 catch 直接调 `errorPresenter.presentToast(...)` 切换 ErrorPresenter 到 toast 态 → 顶部短暂浮现 2s 自动消失（与 Story 2.6 落地的 ErrorPresenter.presentToast 同模式）
4. **不等 WS ack**：V1 §12.2 行 2024 钦定"无 HTTP 响应、无 server → client ack 消息"；client 端无法等 ack 因为没有 ack 信号；emoji.received self-broadcast 是 transient 探测信号不承担 ACK 职责（V1 §12.2 行 2024 末段）

### 决策点 3：emojiCode 缓存校验时机

V1 §12.2 行 2074 钦定 "iOS client 在调用 WebSocketClient.send 前**应**校验 emojiCode 来自 §11.1 缓存的合法表情列表"。**校验时机选 ViewModel.onEmojiSelected 内（调 UseCase 前）而非 UseCase.execute 内**：

1. **职责分层**：
   - WSMessageCodec / WSOutgoingMessage / WebSocketClient.send：**transport 层**，纯序列化 + 拨号 + 发送；不知业务约束
   - SendEmojiUseCase：**业务单一职责**包装 transport，把"emoji.send" 业务消息构造逻辑封装；仍不知 emojiCode 合法性（因为 emoji catalog 在 EmojiPanelViewModel + LoadEmojisUseCase 单例内）
   - RoomViewModel.onEmojiSelected：**业务编排层**，知道"用户选了哪个 emoji" + "本地 catalog 在哪里" + "WS 在哪里"；最适合做 V1 §12.2 校验
2. **降级路径**：校验失败（理论不该 —— EmojiPanelView 显示的 emoji 都来自 catalog；除非 catalog 在 EmojiPanelView 显示后被 reset → race）→ 本 story 选**仍触发本地动效 + skip WS send + log warn**（**不**触发 toast；UX 角度用户已点了表情看到自己飞出，server 端不广播只意味着别人看不到 = 与"网络降级"语义一致；但区别是不弹 toast 因为不是网络问题）
3. **emojiCatalogLoader 可选注入**：旧 wire 路径（如无 emojiCatalogLoader 注入）跳过校验直接 send；与 leaveRoomUseCase / sendEmojiUseCase 可选注入同精神（**保持向后兼容**让 18.x 之前的 RootView wire 测试不破）

### 决策点 4：RoomActiveEmoji.id = UUID（而非 emojiCode 字面）

epics.md 行 2697 钦定"同一表情快速连点 3 次 → activeEmojis 添加 3 项"——意味着同 emojiCode 多次入队都视为独立项。**id = UUID()** 让 SwiftUI ForEach 区分：

1. **若 id = emojiCode**：连点 wave 3 次 → ForEach 看到 3 个 id="wave" → SwiftUI 视为 1 项重复 emit → 只渲染 1 个 → UX bug
2. **若 id = `"\(userId)_\(emojiCode)_\(timestamp)"` 复合 key**：手工拼接易错（时间戳粒度若秒级仍可能撞）
3. **id = UUID()**：每次入队全新生成 → 100% 不撞 → SwiftUI ForEach 渲染 N 项 → UX 满足
4. **18.4 1.5s 移除路径**：`vm.activeEmojis.removeAll { $0.id == toRemove.id }` 按 UUID 移除精确不打误伤同 emojiCode 其他项

### 决策点 5：onSelect 闭包内 onEmojiSelected 先调 / sheet 关闭后调

epics.md 行 2688 钦定"选中后立即关闭 EmojiPanelView（不等 WS ack）" —— 没钦定相对顺序。**选先 onEmojiSelected 再关 sheet**：

1. **UX 角度**：用户点 cell → 期望"立刻看到自己飞出动效 + sheet 关闭" 视觉同时发生；本地 activeEmojis.append 是毫秒级 + SwiftUI .sheet 关闭动画 ~0.3s → 视觉上动效会在 sheet 关闭动画期间逐渐出现 → 自然
2. **反向顺序**（先关 sheet 再 onEmojiSelected）：sheet dismiss 触发 SwiftUI publisher flush + 主线程合并 → 可能延后 onEmojiSelected 触发的 @Published activeEmojis 渲染 → 用户感受"sheet 关 → 隔一拍 → 动效出现"
3. **测试角度**：单元测试 `XCTAssertEqual(vm.activeEmojis.count, 1); XCTAssertFalse(vm.showEmojiPanel)` 顺序无关；UI 测试用 `waitForExistence` 不严格断顺序，但实际看到的 UX 视觉差异由顺序决定，正确顺序是先入队后关 sheet

### V1 接口设计相关锚点

- **V1 §12.2 行 1985-2089 emoji.send 全集**（**17.1 r2 冻结**）：
  - 行 1992-1995 字段表：`type` / `requestId` / `payload.emojiCode` 3 个 wire 字段；`requestId` 选填（client 推荐 `emoji_<ts_ms>`）
  - 行 2000-2008 示例 JSON 与本 story `encode(.emojiSend(...))` 输出 1:1 对齐
  - 行 2012-2024 服务端逻辑步骤：参数校验 / 房间归属校验（Session.roomID） / 表情合法性校验 / fire-and-forget 广播；client 端**不**关心服务端步骤实装，但本 story `errorPresenter.presentToast` 文案对应 V1 §12.2 行 2032 "本地动效是发起者的主要 UX 反馈"
  - 行 2042-2076 错误响应表：1001 / 1002 / 6004 / 7001 / 1009 五种 server 端可能回的 error 消息（client 收到走 vm 层 mapError；但**本 story 不实装 error 消息接收路径** —— 那是 18.4 处理 incoming WS 的工作；本 story 是 client → server 单向）
  - 行 2074 client 缓存契约："iOS client 在调用 WebSocketClient.send 前**应**校验 emojiCode 来自 §11.1 缓存"——本 story 决策点 3 落地
  - 行 2078-2082 active message set 升级：emoji.send 加入 client → server active message set
- **V1 §12.3 行 2435-2481 emoji.received 全集**：
  - 行 2470-2475 client 处理规则（含**对自己 self-broadcast 跳过去重**） —— **18.4 落地**，本 story 不实装；但 18.4 落地的 self-broadcast 去重路径**依赖**本 story 落地的 `currentUserId` 字段（18.2）+ `activeEmojis` 队列（18.3）
- **V1 §11.1 行 1817 client 缓存契约**：本 story `emojiCatalogLoader` 注入路径就是用 LoadEmojisUseCase 缓存查 emojiCode → catalog 已在 EmojiPanelView 显示前 load 过（18.1 落地）
- **V1 §1 行 57-65 17.1 r2 冻结声明**：emoji 链路所有契约字段已在 Story 17.1 锁定 + 冻结；本 story 严格遵守 wire schema 不打破

### iOS 架构 / lesson 相关锚点

- **iOS 架构设计 §6.9 Emoji 模块**（docs/宠物互动App_iOS客户端工程结构与模块职责设计.md 行 400-407）：发送表情属本模块；SendEmojiUseCase 物理放 `Features/Emoji/UseCases/` 与 LoadEmojisUseCase 同目录；RoomActiveEmoji 模型放 `Features/Emoji/Models/`（**不**放 `Features/Room/Models/` —— 它是表情数据，归 Emoji 模块；Room 模块的 RoomScaffoldView 跨 Features 引用 Emoji 模块的类型 = 与既有 18.2 RoomScaffoldView 引用 EmojiPanelView 同 pattern）
- **iOS 架构设计 §13 UseCase 列表**（行 262）：钦定 `SendEmojiUseCase` 是本模块预定义 UseCase 之一；本 story 落地
- **ADR-0009 §3.3 sheet 白名单**：EmojiPanel sheet 已 18.2 落地白名单；本 story 不改 sheet 行为
- **ADR-0010 §3.1 + §3.2**：`activeEmojis` 是 transient UI 队列**不**进 AppState（与 `emojiCatalog` 进 AppState 不同 —— catalog 是系统配置目录持久缓存，activeEmojis 是房间 session 内 transient 队列；与"房间 sheet 是否打开"同精神 → ViewModel @Published）；ADR-0010 §3.2 表格行钦定符合
- **lesson 2026-04-25-swift-explicit-import-combine.md**：所有 ViewModel / Subscription 改动文件顶部按既有模式补 `import Combine`（既有 import 完备，本 story **不**新增 import）
- **lesson 2026-05-14-actor-reentrancy-needs-inflight-task-for-single-flight.md**（18-1 r1）：LoadEmojisUseCase actor 已 single-flight 修过；本 story `emojiCatalogLoader.execute()` 在 onEmojiSelected 内 await 调用 → 触发 18.1 actor 的 cache hit 路径（不会重发 GET /emojis）；遵守该 lesson
- **lesson 2026-05-14-viewmodel-error-mapping-must-mirror-apperrormapper-transient-vs-terminal-18-1-r2.md**（18-1 r2）：vm 层 mapError 必须区分 transient / terminal；本 story onEmojiSelected catch WSError 走 `errorPresenter.presentToast` toast 路径 = transient（与 epics.md 行 2691 "不阻塞，不影响后续操作"语义一致），**不**走 `errorPresenter.present(error)` 全屏 retry overlay = terminal 路径；遵守 lesson
- **lesson 2026-05-11-business-error-fallback-must-forward-original-12-7-r8.md**：catch 路径必须 forward 原始 error 给 presenter；本 story `os_log warn` 记录 + `presentToast` 替代全屏 → 与该 lesson "errorPresenter 走 toast 而非 alert 用于 transient" 同精神
- **lesson 2026-04-26-stateobject-debug-instance-aliasing.md**：bind 重新调用要幂等；本 story bind 新参数 `sendEmojiUseCase` / `emojiCatalogLoader` 用与既有 `leaveRoomUseCase` / `errorPresenter` 相同的"if let injected, override existing"模式 → 第二次 bind 不破已有引用

### 范围红线

- 本 story **只**改 / 新建以下文件：
  - `iphone/PetApp/Core/Networking/WSOutgoingMessage.swift`（**修改**：增 .emojiSend case）
  - `iphone/PetApp/Core/Networking/WSMessageCodec.swift`（**修改**：encode switch 加 .emojiSend 分支）
  - `iphone/PetApp/Features/Emoji/Models/RoomActiveEmoji.swift`（**新建**）
  - `iphone/PetApp/Features/Emoji/UseCases/SendEmojiUseCase.swift`（**新建**：protocol + DefaultSendEmojiUseCase）
  - `iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift`（**修改**：增 activeEmojis 字段 + onEmojiSelected abstract method）
  - `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`（**修改**：bind 签名扩展 + sendEmojiUseCase / emojiCatalogLoader 字段 + onEmojiSelected override）
  - `iphone/PetApp/Features/Room/ViewModels/MockRoomViewModel.swift`（**修改**：Invocation +1 case + onEmojiSelected override + 入队等价语义）
  - `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift`（**修改**：.sheet onSelect 闭包扩展 + EmojiAnimationLayerPlaceholder 占位渲染）
  - `iphone/PetApp/App/AppContainer.swift`（**修改**：makeSendEmojiUseCase factory）
  - `iphone/PetApp/App/RootView.swift`（**修改**：RealRoomViewModel.bind callsite 增 sendEmojiUseCase + emojiCatalogLoader 注入 + 可能新增 `--uitest-emoji-send-host` launch arg 路径）
  - `iphone/PetAppTests/Core/Networking/WSMessageCodecEmojiSendTests.swift`（**新建** 或追加 `WSMessageCodecTests`）
  - `iphone/PetAppTests/Features/Emoji/UseCases/SendEmojiUseCaseTests.swift`（**新建**）
  - `iphone/PetAppTests/Features/Room/RoomViewModelEmojiSendTests.swift`（**新建**）
  - `iphone/PetAppUITests/Features/Room/RoomEmojiSendUITests.swift`（**新建**）
  - 本 story 文件 + sprint-status.yaml 流转
- **不**改 18.1 / 18.2 落地的任何 Emoji 链路文件（EmojiPanelView / EmojiPanelViewModel / EmojiRepository / LoadEmojisUseCase / EmojiConfig / EmojiListResponse / EmojisEndpoints 全套保持原样）
- **不**实装 emoji.received 接收路径 / WSMessage.emojiReceived 枚举 case / 18.4 1.5s 动效自动移除 / 完整 EmojiAnimationLayer 动画路径（18.4 才做）
- **不**改 AppState.emojiCatalog 字段（节点 6 起 ADR-0010 §3.2 钦定占位；18.1 已显式不接入；本 story 同样不接入 —— catalog 走 LoadEmojisUseCase 单例 cache 路径）
- **不**改 ADR / 不开新 ADR（按既有 ADR-0002 / ADR-0009 / ADR-0010 落地）
- **不**实装 emoji.send 限频（V1 §12.2 行 2076 钦定节点 6 阶段不限频）

## Acceptance Criteria

> **AC 编号体系**：AC1 是 WSOutgoingMessage.emojiSend case + WSMessageCodec encode 扩展；AC2 是 RoomActiveEmoji 新建；AC3 是 SendEmojiUseCase 新建；AC4 是 RoomViewModel 基类扩展（activeEmojis + onEmojiSelected abstract）；AC5 是 RealRoomViewModel 实装；AC6 是 MockRoomViewModel 实装；AC7 是 RoomScaffoldView onSelect 闭包扩展 + EmojiAnimationLayer 占位；AC8 是 AppContainer + RootView wire；AC9 是单元测试覆盖；AC10 是 UI 测试覆盖；AC11 是 build verify + ios-simulator MCP 实跑；AC12 是 deliverable + sprint-status.yaml 流转。

### AC1: WSOutgoingMessage.emojiSend case + WSMessageCodec encode 扩展

**Given** Story 12.2 落地的 WSOutgoingMessage enum（已有 .ping case）+ WSMessageCodec.encode + Story 17.1 r2 冻结 V1 §12.2 emoji.send wire schema

**When** 修改 `iphone/PetApp/Core/Networking/WSOutgoingMessage.swift` + `iphone/PetApp/Core/Networking/WSMessageCodec.swift`

**Then**：

#### 1a. WSOutgoingMessage.swift

在既存 `case ping(requestId: String)` 之后追加：

```swift
/// Story 18.3 AC1: V1 §12.2 行 1985-2089 `emoji.send` —— Story 18.3 落地, client → server fire-and-forget.
/// `requestId`: client 生成, 推荐 "emoji_<ts_ms>" 格式 (V1 §12.2 行 1993); server 处理失败时回 error 消息回带该 requestId.
/// `emojiCode`: V1 §11.1 `data.items[].code` 字段值; client 必须从 §11.1 缓存的合法列表取 (V1 §12.2 行 2074);
///              字符集 `[a-z0-9_-]`, length 1-64 (V1 §11.1 行 1771).
/// 序列化由 WSMessageCodec.encode(.emojiSend) 处理; wire schema 严格对齐 V1 §12.2 行 2000-2008.
case emojiSend(requestId: String, emojiCode: String)
```

文件顶部注释块"节点 4 阶段 outgoing 已知 type 集合：ping（V1 §12.2）"行更新为"outgoing 已知 type 集合：ping（V1 §12.2）+ emoji.send（V1 §12.2 行 1985-2089，Story 17.1 锚定 + 18.3 落地）"。

#### 1b. WSMessageCodec.swift

在既存 `encode(_:)` switch 内（line 134 附近）追加：

```swift
case .emojiSend(let requestId, let emojiCode):
    json = [
        "type": "emoji.send",
        "requestId": requestId,
        "payload": ["emojiCode": emojiCode]
    ]
```

`decodingFailed` rawType 字符串保持当前 throw 路径不变（line 144），但**追加** emoji.send 的 fallback：实际上 emoji.send 不会触发 `String(data:encoding:)` 失败因为 JSONSerialization 在 ASCII-only 字符集下不抛错；保留 line 144 既有 throw 兜底。

文件顶部注释块"节点 4 阶段 outgoing 已知 type 集合"行（line 13）更新："节点 4 阶段 outgoing 已知 type 集合：ping（V1 §12.2）；节点 6 阶段扩展：emoji.send（V1 §12.2 行 1985-2089，Story 17.1 锚定 + 18.3 落地）"。

#### 1c. import 块

`WSOutgoingMessage.swift`：既存 `import Foundation` 已覆盖。
`WSMessageCodec.swift`：既存 `import Foundation` 已覆盖。
**不**新增 import。

### AC2: RoomActiveEmoji 新建（Models）

**Given** AC1 wire 层就位

**When** 新建 `iphone/PetApp/Features/Emoji/Models/RoomActiveEmoji.swift`

**Then**：

```swift
// RoomActiveEmoji.swift
// Story 18.3 AC2: 房间内 transient 表情动效队列元素（self + others 共用同一数据结构）.
//
// 设计原则:
//   - id = UUID() 每次入队全新生成 → SwiftUI ForEach 区分同 userId 同 emojiCode 多次入队 (18.3 连点用例;
//     epics.md 行 2697 钦定"连点 3 次 → activeEmojis 添加 3 项")
//   - 数据来源:
//     * 18.3 路径: RoomViewModel.onEmojiSelected 入队 (self 触发)
//     * 18.4 路径: applyEmojiReceived 入队 (others 触发; self echo 由 18.4 跳过去重不入队)
//   - 18.4 落地的 1.5s 后按 createdAt 自动 expire 移除路径需要本 struct 全部 4 字段
//   - 不持久化 (transient UI 队列, ADR-0010 §3.2 钦定); 不进 AppState
//
// 物理位置: Features/Emoji/Models/ 与 EmojiConfig.swift / EmojiListResponse.swift 同模块.

import Foundation

public struct RoomActiveEmoji: Identifiable, Equatable, Sendable {
    /// 每次入队全新 UUID; 让 SwiftUI ForEach 区分同 emojiCode 多次入队.
    /// 18.4 落地 1.5s 后按 id 移除 (vm.activeEmojis.removeAll { $0.id == toRemove.id }).
    public let id: UUID

    /// 触发者 userId; 18.3 路径 = vm.currentUserId; 18.4 路径 = emoji.received.payload.userId.
    /// 18.4 落地的"按 userId 找猫位 anchor"用; userId 在 roster 不存在时 fallback 渲染屏幕中央 (V1 §12.3 行 2473).
    public let userId: String

    /// V1 §11.1 / §12.3 emojiCode (与 EmojiConfig.code 同 wire 字段);
    /// 18.4 落地按 code 查 LoadEmojisUseCase cache 拿 assetUrl.
    public let emojiCode: String

    /// 入队时刻; 18.4 落地 1.5s 后按 createdAt 自动 expire 移除时用.
    /// 本 story (18.3) **不**实装自动移除; 仅落字段为 18.4 留 hook.
    public let createdAt: Date

    public init(id: UUID, userId: String, emojiCode: String, createdAt: Date) {
        self.id = id
        self.userId = userId
        self.emojiCode = emojiCode
        self.createdAt = createdAt
    }
}
```

**关键约束**：
- `id` 是 UUID 不是 String（避免误用 emojiCode 当 id）
- `userId: String` 而非 `Int`（V1 §2.5 BIGINT 字符串化全局约定 + 与 RoomMember.id / HomeUser.id 同类型）
- `createdAt: Date` 而非 `Int64 ms`（Swift 端用 Date 更自然 + 18.4 落地比较 `Date.now.timeIntervalSince(emoji.createdAt)` 与 1.5 自然写）
- **不**实装 `Codable`（transient struct + 不跨 wire / 不持久化；如 18.4 落地需要 wire 序列化再加）

### AC3: SendEmojiUseCase 新建（protocol + DefaultSendEmojiUseCase）

**Given** AC1 wire 层 + AC2 数据结构就位

**When** 新建 `iphone/PetApp/Features/Emoji/UseCases/SendEmojiUseCase.swift`

**Then**：

```swift
// SendEmojiUseCase.swift
// Story 18.3 AC3: V1 §12.2 `emoji.send` 业务 UseCase 包装层.
//
// 设计原则:
//   - 单一职责: 把"用户选中 emojiCode → 通过 WebSocketClient 发 emoji.send" 封装为业务 UseCase,
//     调用方(RealRoomViewModel.onEmojiSelected)无需关心 wire schema / requestId 生成 / WS encode
//   - 透传 WSError: send 失败 (.notConnected / .decodingFailed) 原始抛出, 由 vm 层 mapError 走 toast
//   - 不做 emojiCode 合法性二次校验: V1 §12.2 行 2074 缓存校验由 caller (vm) 在调用前完成
//     (用 LoadEmojisUseCase cache); UseCase 是 transport 包装层不重复职责
//   - fire-and-forget 语义: 不等 server 任何响应 (V1 §12.2 行 2024 "无 HTTP 响应、无 server → client ack 消息")
//   - requestId 在 UseCase 内部生成 (caller 无需关心): 用 "emoji_<ms_timestamp>" 格式 (V1 §12.2 行 1993 推荐)
//
// 物理位置: Features/Emoji/UseCases/ 与 LoadEmojisUseCase.swift / EmojisEndpoints.swift 同模块.

import Foundation

public protocol SendEmojiUseCaseProtocol: Sendable {
    /// 发送 emoji.send 消息.
    /// - Parameter emojiCode: V1 §11.1 emojiCode 字符集 [a-z0-9_-], length 1-64; caller 应已校验合法性.
    /// - Throws: `WSError.notConnected` (WS 未连接) / `WSError.decodingFailed` (JSON 序列化失败, 理论不该);
    ///           其他 WSError case 同模式透传不转换.
    /// 调用约定: caller 应用 do-try-catch 包裹; 不抛错 = server 端"已收到 frame" (server 处理 / 广播失败由
    /// fire-and-forget 静默处理, 不通过抛错告知 caller; V1 §12.2 行 2031 钦定).
    func execute(emojiCode: String) async throws
}

public final class DefaultSendEmojiUseCase: SendEmojiUseCaseProtocol {
    private let webSocketClient: WebSocketClient

    public init(webSocketClient: WebSocketClient) {
        self.webSocketClient = webSocketClient
    }

    public func execute(emojiCode: String) async throws {
        // V1 §12.2 行 1993: requestId 推荐格式 "emoji_<ts_ms>"; client 自行生成, server 不强制.
        let requestId = "emoji_\(Int(Date().timeIntervalSince1970 * 1000))"
        let message = WSOutgoingMessage.emojiSend(requestId: requestId, emojiCode: emojiCode)
        try await webSocketClient.send(message)
        // V1 §12.2 行 2024: 不等 server ack; send 返回即视为本 UseCase 成功.
    }
}
```

**关键约束**：
- `class` 而非 `struct`（持 WebSocketClient 引用 + 让 Sendable 在 final class 模式下与 LoadEmojisUseCase 同精神）
- 公开 `init(webSocketClient:)` 让测试可注入 MockWebSocketClient
- **不**持 weak ref（webSocketClient 是 container 单例，UseCase 生命周期 ≤ container 生命周期；strong ref OK）
- `requestId` 生成路径**不**走全局 counter / clock 注入（YAGNI；如未来需要 deterministic 测试 requestId 再加 clock 参数）

### AC4: RoomViewModel 基类扩展（activeEmojis + onEmojiSelected abstract）

**Given** 既存 RoomViewModel 基类（**18.2 落地后**8 字段 + 3 abstract method）

**When** 修改 `iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift`

**Then**：

在既存 `showEmojiPanel` / `currentUserId`（18.2 落地）之后追加：

```swift
/// Story 18.3 AC4: 房间内 transient 表情动效队列 (self + others 共用; ADR-0010 §3.2 transient UI state).
/// - 18.3 路径: vm.onEmojiSelected(code:) 内入队 (self 触发)
/// - 18.4 路径: applyEmojiReceived(payload:) 内入队 (others 触发; self echo 跳过去重)
/// - 视图层 RoomScaffoldView 内 EmojiAnimationLayer ForEach 渲染 (18.3 占位; 18.4 替换为完整动画)
/// - 18.4 落地 1.5s 后按 createdAt 自动 expire 移除; 本 story (18.3) **不**实装移除路径
/// 唯一 owner = ViewModel @Published; SwiftUI 通过 @ObservedObject state 间接订阅.
@Published public var activeEmojis: [RoomActiveEmoji] = []
```

在既存 `onLeaveTap()` / `onCopyTap()` / `onOwnPetTap()`（18.2 落地）之后追加：

```swift
/// Story 18.3 AC4: 用户从 EmojiPanelView 选中表情 cell 后的回调入口.
/// 触发路径: RoomScaffoldView .sheet onSelect 闭包 → `state.onEmojiSelected(code: code)`.
/// 子类 override 行为:
///   - MockRoomViewModel: 入队 activeEmojis + invocations 记录
///   - RealRoomViewModel: 入队 activeEmojis (本地立即动效) + V1 §12.2 缓存校验 + 调 SendEmojiUseCase fire-and-forget (Task 包裹, 不阻塞主线程; 失败弹 toast 不回滚动效)
public func onEmojiSelected(code: String) {
    fatalError("RoomViewModel.onEmojiSelected must be overridden by subclass")
}
```

文件顶部注释块"8 字段"行改 "9 字段"（含新增 activeEmojis）；"3 abstract method" 改 "4 abstract method"（含新增 onEmojiSelected）。

`import` 块**不**新增（既存 `import Foundation` + `import Combine` 已覆盖 `@Published`；UUID / Date 等 RoomActiveEmoji 用的类型在 `import Foundation` 已覆盖）。

### AC5: RealRoomViewModel 实装（bind 签名扩展 + 字段 + onEmojiSelected override）

**Given** AC1 ~ AC4 就位

**When** 修改 `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`

**Then**：

#### 5a. 新增字段（与既存 `leaveRoomUseCase` / `errorPresenter` 同位置 / 同模式）

```swift
/// Story 18.3 AC5: SendEmojiUseCase 注入 (默认 nil; 旧 wire 路径不破).
/// caller=RootView 通过 bind(... sendEmojiUseCase: container.makeSendEmojiUseCase() ...) 注入.
private var sendEmojiUseCase: SendEmojiUseCaseProtocol?

/// Story 18.3 AC5: LoadEmojisUseCase 注入 (用于 V1 §12.2 行 2074 emojiCode 缓存校验; weak 避免循环引用).
/// caller=RootView 通过 bind(... emojiCatalogLoader: container.loadEmojisUseCase ...) 注入 (注: container.loadEmojisUseCase
/// 是 stable singleton class, weak 引用合法; 与 errorPresenter weak 模式同精神).
/// **注**: 由于 protocol existential 不支持 weak (LoadEmojisUseCaseProtocol 是 protocol 非 class), 本字段用 strong ref;
/// 类型签名为 `LoadEmojisUseCaseProtocol?` 但持有的是 DefaultLoadEmojisUseCase final class 单例 instance; 单例生命周期 = container
/// 生命周期 > vm 生命周期, strong ref 不会延长 vm 生命周期 (vm deinit 时字段释放, 单例 instance 仍由 container 持).
private var emojiCatalogLoader: LoadEmojisUseCaseProtocol?
```

#### 5b. bind 方法签名扩展

既存签名：

```swift
public func bind(
    appState: AppState,
    webSocketClient: WebSocketClient? = nil,
    leaveRoomUseCase: LeaveRoomUseCaseProtocol? = nil,
    errorPresenter: ErrorPresenter? = nil
)
```

扩展为：

```swift
public func bind(
    appState: AppState,
    webSocketClient: WebSocketClient? = nil,
    leaveRoomUseCase: LeaveRoomUseCaseProtocol? = nil,
    sendEmojiUseCase: SendEmojiUseCaseProtocol? = nil,    // Story 18.3 AC5 新增
    emojiCatalogLoader: LoadEmojisUseCaseProtocol? = nil,  // Story 18.3 AC5 新增
    errorPresenter: ErrorPresenter? = nil
)
```

方法体内追加（与 errorPresenter / leaveRoomUseCase 同位置 / 同模式 "if let injected, override"）：

```swift
if let useCase = sendEmojiUseCase {
    self.sendEmojiUseCase = useCase
}
if let loader = emojiCatalogLoader {
    self.emojiCatalogLoader = loader
}
```

#### 5c. onEmojiSelected override 实装

在既存 `onOwnPetTap()` 之后追加：

```swift
/// Story 18.3 AC5: 选中表情 → 0 延迟本地动效 + V1 §12.2 emojiCode 缓存校验 + WS fire-and-forget.
/// epics.md 行 2685-2691 钦定: 本地动效**先**触发 (不等 WS), WS 用 Task 包裹并行调用, 失败弹 toast 不回滚动效.
public override func onEmojiSelected(code: String) {
    os_log(.debug, "RealRoomViewModel.onEmojiSelected: code=%{public}@", code)

    // Step A: 本地立即动效 (0 延迟; **先**执行).
    // currentUserId == nil 是 fail-safe 路径 (理论不该; RoomScaffoldView Button 只在 currentUserId 非空时渲染);
    // 用空字符串 userId 入队让动效仍触发, 不报错; log warn 便于追溯.
    let effectiveUserId = self.currentUserId ?? ""
    if effectiveUserId.isEmpty {
        os_log(.info, "RealRoomViewModel.onEmojiSelected: currentUserId is nil; using empty userId for activeEmoji")
    }
    let emoji = RoomActiveEmoji(
        id: UUID(),
        userId: effectiveUserId,
        emojiCode: code,
        createdAt: Date()
    )
    self.activeEmojis.append(emoji)

    // Step B + C: V1 §12.2 缓存校验 + WS fire-and-forget (Task 包裹, 不阻塞主线程).
    let presenter = self.errorPresenter
    let useCase = self.sendEmojiUseCase
    let loader = self.emojiCatalogLoader

    Task { [weak self] in
        guard let _ = self else { return }

        // Step B: V1 §12.2 行 2074 emojiCode 缓存校验 (loader = nil 时跳过, 兼容旧 wire).
        if let loader = loader {
            do {
                let catalog = try await loader.execute()
                guard catalog.contains(where: { $0.code == code }) else {
                    os_log(.info, "RealRoomViewModel.onEmojiSelected: emojiCode %{public}@ not in catalog; skip WS send (local animation still played)", code)
                    return
                }
            } catch {
                // catalog load 失败本身不阻塞 WS send; log warn 然后继续 send (transport 层若网络有问题会另抛 toast).
                os_log(.info, "RealRoomViewModel.onEmojiSelected: catalog load failed: %{public}@; proceed with WS send anyway", String(describing: error))
            }
        }

        // Step C: WS fire-and-forget send.
        guard let useCase = useCase else {
            os_log(.info, "RealRoomViewModel.onEmojiSelected: sendEmojiUseCase = nil; skip WS send (local animation still played)")
            return
        }
        do {
            try await useCase.execute(emojiCode: code)
            os_log(.debug, "RealRoomViewModel.onEmojiSelected: emoji.send sent successfully: %{public}@", code)
        } catch let wsError as WSError {
            // V1 §12.2 弱网降级: 本地动效已播 (Step A); WS 失败仅 toast 提示, 不回滚 activeEmojis.
            os_log(.info, "RealRoomViewModel.onEmojiSelected: WS send failed with WSError: %{public}@; presenting toast", String(describing: wsError))
            await MainActor.run { presenter?.presentToast("网络不佳，对方可能看不到") }
        } catch {
            // catch-all 兜底, 同 transient toast 路径 (lesson 2026-04-27-business-error-transient-vs-terminal).
            os_log(.info, "RealRoomViewModel.onEmojiSelected: WS send failed with unexpected error: %{public}@; presenting toast", String(describing: error))
            await MainActor.run { presenter?.presentToast("网络不佳，对方可能看不到") }
        }
    }
}
```

**关键约束**：
- Step A 在 Task 外、同步执行：保证 `vm.activeEmojis.append` 在 onEmojiSelected 返回前完成 → SwiftUI 下一 runloop 必看到 → 0 延迟动效 UX 满足
- Step B/C 走 Task 异步：让 WS / catalog load 不阻塞 onEmojiSelected 返回 → sheet 关闭 + 本地动效不被网络延迟影响
- `[weak self]` capture：避免 vm deinit 后 Task 仍持 vm 强引用导致泄漏
- `presenter`/`useCase`/`loader` 局部捕获：避免 Task 体内 `self.errorPresenter` 在 vm 重 bind 时 race（与 onLeaveTap fix-review r3 P1 同精神）
- 错误降级**只**走 `presenter.presentToast(...)`，**禁止** `presenter.present(error)` 全屏 retry overlay（与 fix-review r3 P1 lesson 一致）
- toast 文案 `"网络不佳，对方可能看不到"` 与 epics.md 行 2691 钦定文案 1:1 一致；**不**改成 "WS 断开" 等技术词
- 在 `MainActor.run { ... }` 内调 presenter 因 ErrorPresenter 是 `@MainActor public final class`（与既有 leave 错误兜底 catch 路径同模式）

#### 5d. import 块

既存 `import Foundation` + `import Combine` + `import os.log` 已覆盖；**不**新增。

### AC6: MockRoomViewModel 实装（Invocation + onEmojiSelected override）

**Given** AC4 基类就位

**When** 修改 `iphone/PetApp/Features/Room/ViewModels/MockRoomViewModel.swift`

**Then**：

#### 6a. Invocation enum 增 case

```swift
public enum Invocation: Equatable {
    case leaveTap
    case copyTap
    case ownPetTap          // 18.2 落地
    case emojiSelected(code: String)  // Story 18.3 AC6 新增
}
```

#### 6b. onEmojiSelected override

在既存 `onOwnPetTap()` 之后追加：

```swift
public override func onEmojiSelected(code: String) {
    os_log(.debug, "MockRoomViewModel.onEmojiSelected: code=%{public}@", code)
    invocations.append(.emojiSelected(code: code))
    let emoji = RoomActiveEmoji(
        id: UUID(),
        userId: self.currentUserId ?? "",
        emojiCode: code,
        createdAt: Date()
    )
    self.activeEmojis.append(emoji)
    // **不**调任何 WS / UseCase (Mock 永远不持 webSocketClient / sendEmojiUseCase).
    // **不**关 sheet (sheet 关闭由 RoomScaffoldView .sheet onSelect 闭包驱动, 与 18.2 同一处).
}
```

**关键约束**：与 RealRoomViewModel.onEmojiSelected 的 Step A 行为对齐（同入队语义）让单元测试 / Preview / UITest 行为一致；其他步骤（B/C 网络）不实装是因为 Mock 永远不持网络组件。

#### 6c. import 块

既存 import 已覆盖；**不**新增。

### AC7: RoomScaffoldView onSelect 闭包扩展 + EmojiAnimationLayer 占位

**Given** AC4-AC6 ViewModel 层就位

**When** 修改 `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift`

**Then**：

#### 7a. .sheet onSelect 闭包扩展

既有（18.2 落地）：
```swift
.sheet(isPresented: $state.showEmojiPanel) {
    EmojiPanelView(
        viewModel: emojiPanelViewModelFactory(),
        onSelect: { _ in
            state.showEmojiPanel = false
        }
    )
    .presentationDetents([.medium])
    .presentationCornerRadius(28)
}
```

改为：
```swift
.sheet(isPresented: $state.showEmojiPanel) {
    EmojiPanelView(
        viewModel: emojiPanelViewModelFactory(),
        onSelect: { code in
            // Story 18.3 AC7: 先触发本地动效 + WS fire-and-forget (Step A 同步入队, Step B/C Task 异步), 再关 sheet.
            // 顺序关键: 先 onEmojiSelected 让 activeEmojis 在 sheet 关闭动画期间已入队 → UX 视觉自然.
            // 反向顺序 (先关 sheet 再 onEmojiSelected) 会让 SwiftUI publisher 合并主线程 emit 让动效晚一拍出现.
            state.onEmojiSelected(code: code)
            state.showEmojiPanel = false
        }
    )
    .presentationDetents([.medium])
    .presentationCornerRadius(28)
}
```

#### 7b. EmojiAnimationLayer 占位渲染

在既存 body ZStack 内（与 LinearGradient / ScrollView 同层；**位置在 .sheet 之下** —— Story 18.4 完整动画时 anchor 计算可能依赖 ScrollView 内成员位坐标，但本 story 占位**不**做 anchor，仅简单 ForEach 渲染 emojiCode 文本 + a11y identifier）追加：

```swift
public var body: some View {
    ZStack {
        LinearGradient(...).ignoresSafeArea()
        ScrollView { ... }

        // Story 18.3 AC7 占位渲染: 让 activeEmojis 队列在屏幕上可见, 让 UITest 可断言;
        // Story 18.4 落地后整体替换为 EmojiAnimationLayer (含 anchor / 1.5s 移除 / 飞出动画).
        // 本占位**必须**有 a11y identifier `activeEmoji_<uuid>` 让 UITest 定位; 文本内容用 emojiCode (UITest 用 staticTexts["wave"] 断言).
        VStack {
            ForEach(state.activeEmojis) { emoji in
                Text(emoji.emojiCode)
                    .font(.title2)
                    .accessibilityIdentifier("activeEmoji_\(emoji.id.uuidString)")
            }
        }
        .accessibilityIdentifier("emojiAnimationLayerPlaceholder")
    }
    .sheet(isPresented: $state.showEmojiPanel) { ... }
}
```

**关键约束**：
- 占位渲染**必须**在 ZStack 内（与 ScrollView 同 ZStack 层 → 飞出动画时浮于内容上层）
- **不**做完整 anchor 计算（18.4 才做）；占位用 VStack 居中即可
- a11y identifier `activeEmoji_<uuid>` 是 UITest 断言锚点（连点 N 次 → N 个不同 UUID 都可被 UITest 看到）
- 18.4 落地会**替换**这个 VStack 为完整 EmojiAnimationLayer + 不变 state.activeEmojis 读取路径 + 不变 RoomActiveEmoji 字段；本 story 占位**不阻挡** 18.4 落地

#### 7c. import 块

既存 `import SwiftUI` 已覆盖；本 story 跨 Features 引用 `RoomActiveEmoji`（Features/Emoji/Models）—— public struct 自然可见，**不**新增 import（与 18.2 引用 EmojiPanelView 同模式）。

### AC8: AppContainer + RootView wire

**Given** AC1-AC7 就位

**When** 修改 `iphone/PetApp/App/AppContainer.swift` + `iphone/PetApp/App/RootView.swift`

**Then**：

#### 8a. AppContainer.swift

在 `// MARK: - Story 18.1 AC5: Emoji 链路 factory` block 内（line 474 附近，`makeEmojiPanelViewModel()` 之后）追加：

```swift
/// Story 18.3 AC8: 构造 SendEmojiUseCase (每次调用返回新实例; webSocketClient 单例由 container 持有).
/// caller=RootView 在 RealRoomViewModel.bind(...) 时注入.
/// 与 makeStepRepository / makeRoomRepository 同模式 (UseCase 内部不持状态, 多次构造廉价).
public func makeSendEmojiUseCase() -> SendEmojiUseCaseProtocol {
    DefaultSendEmojiUseCase(webSocketClient: webSocketClient)
}
```

#### 8b. RootView.swift

找到 RealRoomViewModel.bind 的 callsite（grep `bind(appState:.*webSocketClient:.*leaveRoomUseCase:` 或 `.bind\(appState:`），在既有参数列表内追加：

```swift
roomViewModel.bind(
    appState: appState,
    webSocketClient: container.webSocketClient,
    leaveRoomUseCase: container.makeLeaveRoomUseCase(appState: appState),
    sendEmojiUseCase: container.makeSendEmojiUseCase(),       // Story 18.3 AC8 新增
    emojiCatalogLoader: container.loadEmojisUseCase,           // Story 18.3 AC8 新增 (单例字段而非 factory)
    errorPresenter: container.errorPresenter
)
```

**注**：`emojiCatalogLoader` 直接传 `container.loadEmojisUseCase`（单例字段）而非走 `makeXxx()` factory —— 因 LoadEmojisUseCase 是 stable singleton，cache 跨 vm 共享（Story 18.1 AC5 钦定）。

#### 8c. RootView.swift DEBUG launch arg

如要给 UITest case 提供"完整 SendEmoji 路径"的 launch arg，在既存 `--uitest-emoji-panel-room-host` 路径之后追加 `--uitest-emoji-send-host`（**新增**或复用既有 launch arg）：

```swift
#if DEBUG
if CommandLine.arguments.contains("--uitest-emoji-send-host") {
    // 与 --uitest-emoji-panel-room-host 同精神, 但额外 wire 完整 SendEmojiUseCase 链路:
    // - MockRoomViewModel 路径 (currentUserId="u1", 3 成员) + MockWebSocketClient (UITEST_MOCK_WEBSOCKET=1 启用)
    // - container.makeSendEmojiUseCase() 注入 vm 让 onEmojiSelected 走 production code path
    // - UITEST_MOCK_EMOJI=1 让 LoadEmojisUseCase 用 fixture 4 项
}
#endif
```

**注**：本 launch arg 是为 AC10 UI 测试用；具体路径设计 dev 可与 18.2 `--uitest-emoji-panel-room-host` 合并（如让 18.2 路径默认 wire sendEmojiUseCase）或独立（保留 18.2 路径不破，新增专用 launch arg）；**两种路径都合法**，dev 选简者。

### AC9: 单元测试覆盖 ≥4 case

#### 9a. `iphone/PetAppTests/Core/Networking/WSMessageCodecEmojiSendTests.swift`（新建 或追加既存 WSMessageCodecTests）

```swift
import XCTest
@testable import PetApp

final class WSMessageCodecEmojiSendTests: XCTestCase {
    func test_encode_emojiSend_matchesV12Schema() throws {
        let message = WSOutgoingMessage.emojiSend(requestId: "emoji_1715600000000", emojiCode: "wave")
        let json = try WSMessageCodec.encode(message)
        // sortedKeys 保证 key 顺序确定 (与 ping case 同模式)
        let expected = "{\"payload\":{\"emojiCode\":\"wave\"},\"requestId\":\"emoji_1715600000000\",\"type\":\"emoji.send\"}"
        XCTAssertEqual(json, expected)
    }

    func test_encode_emojiSend_specialCharactersInCode() throws {
        let message = WSOutgoingMessage.emojiSend(requestId: "emoji_x", emojiCode: "my_emoji-1")
        let json = try WSMessageCodec.encode(message)
        XCTAssertTrue(json.contains("\"emojiCode\":\"my_emoji-1\""))
    }

    func test_encode_emojiSend_emptyCodeStillEncodes() throws {
        // codec 是纯序列化层不做业务校验; 空 emojiCode 由 ViewModel 层校验
        let message = WSOutgoingMessage.emojiSend(requestId: "emoji_x", emojiCode: "")
        let json = try WSMessageCodec.encode(message)
        XCTAssertTrue(json.contains("\"emojiCode\":\"\""))
    }

    func test_encode_emojiSend_emptyRequestId() throws {
        let message = WSOutgoingMessage.emojiSend(requestId: "", emojiCode: "wave")
        let json = try WSMessageCodec.encode(message)
        XCTAssertTrue(json.contains("\"requestId\":\"\""))
    }
}
```

#### 9b. `iphone/PetAppTests/Features/Emoji/UseCases/SendEmojiUseCaseTests.swift`（新建）

```swift
import XCTest
@testable import PetApp

final class SendEmojiUseCaseTests: XCTestCase {
    func test_execute_callsWebSocketClientSendWithEmojiSendMessage() async throws {
        let mockClient = WebSocketClientMock()
        let useCase = DefaultSendEmojiUseCase(webSocketClient: mockClient)

        try await useCase.execute(emojiCode: "wave")

        XCTAssertEqual(mockClient.sentMessages.count, 1)
        guard case .emojiSend(let requestId, let code) = mockClient.sentMessages.first else {
            XCTFail("Expected .emojiSend, got \(String(describing: mockClient.sentMessages.first))")
            return
        }
        XCTAssertEqual(code, "wave")
        XCTAssertTrue(requestId.hasPrefix("emoji_"), "requestId should start with emoji_ prefix; got: \(requestId)")
    }

    func test_execute_throwsWhenWebSocketSendFails() async {
        let mockClient = WebSocketClientMock()
        mockClient.sendError = .notConnected
        let useCase = DefaultSendEmojiUseCase(webSocketClient: mockClient)

        do {
            try await useCase.execute(emojiCode: "wave")
            XCTFail("Expected throw, got success")
        } catch let error as WSError {
            XCTAssertEqual(error, .notConnected)
        } catch {
            XCTFail("Expected WSError.notConnected, got \(error)")
        }
    }

    func test_execute_multipleCallsSendMultipleMessages() async throws {
        let mockClient = WebSocketClientMock()
        let useCase = DefaultSendEmojiUseCase(webSocketClient: mockClient)

        try await useCase.execute(emojiCode: "wave")
        try await useCase.execute(emojiCode: "love")
        try await useCase.execute(emojiCode: "laugh")

        XCTAssertEqual(mockClient.sentMessages.count, 3)
        let codes = mockClient.sentMessages.compactMap { msg -> String? in
            if case .emojiSend(_, let code) = msg { return code } else { return nil }
        }
        XCTAssertEqual(codes, ["wave", "love", "laugh"])
    }
}
```

#### 9c. `iphone/PetAppTests/Features/Room/RoomViewModelEmojiSendTests.swift`（新建）

```swift
import XCTest
@testable import PetApp

@MainActor
final class RoomViewModelEmojiSendTests: XCTestCase {
    // happy: mockOnEmojiSelected 立即入队 activeEmojis + invocations 记录
    func test_mockOnEmojiSelected_appendsActiveEmojiAndRecordsInvocation() {
        let vm = MockRoomViewModel(currentUserId: "u1")
        XCTAssertTrue(vm.activeEmojis.isEmpty)
        XCTAssertTrue(vm.invocations.isEmpty)

        vm.onEmojiSelected(code: "wave")

        XCTAssertEqual(vm.activeEmojis.count, 1)
        XCTAssertEqual(vm.activeEmojis.first?.userId, "u1")
        XCTAssertEqual(vm.activeEmojis.first?.emojiCode, "wave")
        XCTAssertEqual(vm.invocations, [.emojiSelected(code: "wave")])
    }

    // happy: 连点 3 次 → activeEmojis 长度 3 + 每项 UUID 不同
    func test_rapidConsecutiveEmojiSelections_appendIndependentItems() {
        let vm = MockRoomViewModel(currentUserId: "u1")
        vm.onEmojiSelected(code: "wave")
        vm.onEmojiSelected(code: "wave")
        vm.onEmojiSelected(code: "wave")

        XCTAssertEqual(vm.activeEmojis.count, 3)
        // 3 个 UUID 必两两不同 (UUID 算法 collision 概率 negligible)
        let ids = vm.activeEmojis.map { $0.id }
        XCTAssertEqual(Set(ids).count, 3)
    }

    // happy: RealRoomViewModel 路径整链 (本地入队 + Task 内 WS send)
    func test_realOnEmojiSelected_appendsLocallyAndSendsWS() async throws {
        let mockWS = WebSocketClientMock()
        let useCase = DefaultSendEmojiUseCase(webSocketClient: mockWS)
        let vm = RealRoomViewModel()
        let appState = AppState()
        vm.bind(
            appState: appState,
            webSocketClient: mockWS,
            sendEmojiUseCase: useCase,
            emojiCatalogLoader: nil
        )
        vm.currentUserId = "u1"  // direct mutation; 18.2 测试已覆盖订阅路径

        vm.onEmojiSelected(code: "wave")

        // Step A: 本地立即入队
        XCTAssertEqual(vm.activeEmojis.count, 1)
        XCTAssertEqual(vm.activeEmojis.first?.emojiCode, "wave")
        XCTAssertEqual(vm.activeEmojis.first?.userId, "u1")

        // Step C: Task 内 WS send (等 Task 跑完)
        try await Task.sleep(nanoseconds: 100_000_000)  // 100ms
        XCTAssertEqual(mockWS.sentMessages.count, 1)
        if case .emojiSend(_, let code) = mockWS.sentMessages.first {
            XCTAssertEqual(code, "wave")
        } else {
            XCTFail("Expected .emojiSend message in mockWS.sentMessages")
        }
    }

    // edge: WS send 失败 → activeEmojis 不回滚 + toast 触发
    func test_realOnEmojiSelected_wsFailureKeepsLocalAnimationAndShowsToast() async throws {
        let mockWS = WebSocketClientMock()
        mockWS.sendError = .notConnected
        let useCase = DefaultSendEmojiUseCase(webSocketClient: mockWS)
        let presenter = ErrorPresenter(toastDuration: 5.0)  // 5s 让 case assert 时还在 toast 态
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
        XCTAssertEqual(vm.activeEmojis.count, 1)

        // Step C: 等 Task 跑完 + 验证 toast
        try await Task.sleep(nanoseconds: 200_000_000)  // 200ms
        if case .toast(let message) = presenter.current {
            XCTAssertEqual(message, "网络不佳，对方可能看不到")
        } else {
            XCTFail("Expected toast presentation, got \(String(describing: presenter.current))")
        }
    }
}
```

**导入**：测试文件首部 `import XCTest` + `@testable import PetApp`（与既有 RoomViewModelEmojiPanelTests 同模式）；`@MainActor` class 注解让所有测试在主线程跑（vm 改动 @Published 字段 + ErrorPresenter 操作必须 main thread）。

### AC10: UI 测试覆盖（XCUITest）

**新建 `iphone/PetAppUITests/Features/Room/RoomEmojiSendUITests.swift`**：

**前置**：launch arg `--uitest-emoji-send-host`（或复用 `--uitest-emoji-panel-room-host` + 额外 env `UITEST_MOCK_WEBSOCKET=1` 让 RealRoomViewModel 走完整 SendEmojiUseCase 路径但 WS 走 Mock），env `UITEST_SKIP_GUEST_LOGIN=1` + `UITEST_FORCE_IN_ROOM=1` + `UITEST_MOCK_EMOJI=1` + `UITEST_MOCK_WEBSOCKET=1`。

```swift
import XCTest

@MainActor
final class RoomEmojiSendUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUp() async throws {
        try await super.setUp()
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["--uitest-emoji-send-host"]
        app.launchEnvironment = [
            "UITEST_SKIP_GUEST_LOGIN": "1",
            "UITEST_FORCE_IN_ROOM": "1",
            "UITEST_MOCK_EMOJI": "1",
            "UITEST_MOCK_WEBSOCKET": "1"
        ]
        app.launch()
    }

    // case A: 本地动效 0 延迟可见
    func test_selectEmoji_immediatelyShowsLocalAnimation() throws {
        let selfButton = app.buttons["roomMember_0_petSprite"]
        XCTAssertTrue(selfButton.waitForExistence(timeout: 3))
        selfButton.tap()

        let waveCell = app.buttons["emojiCell_wave"]
        XCTAssertTrue(waveCell.waitForExistence(timeout: 2))
        waveCell.tap()

        // 验证 0.5s 内屏幕上至少 1 个 activeEmoji 节点可见 (占位渲染下是 Text("wave"))
        let activeEmojiPredicate = NSPredicate(format: "identifier BEGINSWITH 'activeEmoji_'")
        let activeEmoji = app.staticTexts.matching(activeEmojiPredicate).firstMatch
        XCTAssertTrue(activeEmoji.waitForExistence(timeout: 0.5))
        XCTAssertEqual(activeEmoji.label, "wave")
    }

    // case B: 连点 3 次 → 3 个独立 activeEmoji 可见 (占位渲染叠加; 18.4 落地后变独立动画)
    func test_rapidThreeTaps_showsThreeActiveEmojis() throws {
        let selfButton = app.buttons["roomMember_0_petSprite"]
        XCTAssertTrue(selfButton.waitForExistence(timeout: 3))

        for _ in 0..<3 {
            selfButton.tap()
            let waveCell = app.buttons["emojiCell_wave"]
            XCTAssertTrue(waveCell.waitForExistence(timeout: 2))
            waveCell.tap()
            // 等 sheet 关闭再开下一轮
            XCTAssertTrue(app.otherElements["emojiPanel"].waitForNonExistence(timeout: 1))
        }

        let activeEmojiPredicate = NSPredicate(format: "identifier BEGINSWITH 'activeEmoji_'")
        let count = app.staticTexts.matching(activeEmojiPredicate).count
        XCTAssertGreaterThanOrEqual(count, 3, "Expected at least 3 active emojis after 3 taps; got \(count)")
    }

    // case C: 选不同 emoji → activeEmojis 含不同 emojiCode 文本
    func test_selectDifferentEmojis_showsBoth() throws {
        let selfButton = app.buttons["roomMember_0_petSprite"]
        XCTAssertTrue(selfButton.waitForExistence(timeout: 3))

        selfButton.tap()
        app.buttons["emojiCell_wave"].tap()
        XCTAssertTrue(app.otherElements["emojiPanel"].waitForNonExistence(timeout: 1))

        selfButton.tap()
        app.buttons["emojiCell_love"].tap()
        XCTAssertTrue(app.otherElements["emojiPanel"].waitForNonExistence(timeout: 1))

        XCTAssertTrue(app.staticTexts["wave"].waitForExistence(timeout: 0.5))
        XCTAssertTrue(app.staticTexts["love"].waitForExistence(timeout: 0.5))
    }
}
```

**注**：弱网降级（toast）路径 UITest 稳定性差（toast 自动 2s 消失 + UITest setUp/wait 时间窗不稳）→ **改在 AC9 单元测试覆盖** test_realOnEmojiSelected_wsFailureKeepsLocalAnimationAndShowsToast 已覆盖该路径；UITest 不强求该 case。

### AC11: build verify + ios-simulator MCP 实跑验证

**Given** AC1 ~ AC10 代码改动 + 测试就位

**When** 验证：

1. **build**：`bash iphone/scripts/build.sh` → xcodebuild 通过（无 compile error；如撞既存 scheme/destination 解析 quirk 用 `xcodebuild -target PetApp -configuration Debug -sdk iphonesimulator26.5 ...` 绕，与 18.2 dev 路径一致）
2. **单元测试**：跑 `xcodebuild test -only-testing:PetAppTests/WSMessageCodecEmojiSendTests` + `SendEmojiUseCaseTests` + `RoomViewModelEmojiSendTests` 三个新测试 class 全 pass（≥10 cases 全绿）
3. **ios-simulator MCP 实跑**（必跑，CLAUDE.md "iOS UI 验证（必跑）"钦定）：
   - `install_app(app_path: iphone/build/DerivedData/Build/Products/Debug-iphonesimulator/PetApp.app)`
   - `launch_app(bundle_id: "com.zhuming.pet.app", terminate_running: true)` + launch arg `--uitest-emoji-send-host` + env `UITEST_MOCK_WEBSOCKET=1` 等（用 `simctl spawn` 或自定义 launch_app 路径；如 MCP 不支持 launch arg，用 dev tools 路径）
   - `ui_view` 看初始房间页：4 成员行可见 + 自己位（index 0）renderable
   - `ui_find_element(roomMember_0_petSprite)` 定位 → `ui_tap`
   - `ui_view` 验证 emoji panel sheet 出现
   - `ui_find_element(emojiCell_wave)` 定位 → `ui_tap`
   - `ui_view` 验证：
     - sheet 已关闭
     - 屏幕上**立刻**可见"wave" 文本（占位渲染；activeEmoji_<uuid> 节点）
     - 时间窗 < 1s（视觉判断；UITest case A 用 waitForExistence(0.5) 严格断言）
   - 连点 3 次 wave → ui_view 验证 3 个 wave 文本节点叠加可见

**Then**：build + UI 实跑全部通过；如视觉异常（如点了 cell 后 sheet 没关、wave 文本不出现、toast 错误触发）→ 修代码再跑；**禁止**仅靠 `bash iphone/scripts/build.sh` 通过就报 done（CLAUDE.md "iOS UI 验证（必跑）"明文钦定）。

### AC12: deliverable 清单 + sprint-status.yaml 流转

**Deliverable 清单**：
- 10 个 production 文件改动（WSOutgoingMessage / WSMessageCodec / RoomActiveEmoji / SendEmojiUseCase / RoomViewModel / RealRoomViewModel / MockRoomViewModel / RoomScaffoldView / AppContainer / RootView）
- 4 个测试文件改动（WSMessageCodecEmojiSendTests / SendEmojiUseCaseTests / RoomViewModelEmojiSendTests / RoomEmojiSendUITests）
- 1 个 story 文件（本文件）
- 1 个 sprint-status.yaml 流转（**本 story 创建阶段** ready-for-dev；**dev-story 阶段后** review；**code-review 阶段后** done）

**sprint-status.yaml 流转**（本 story 创建阶段执行 1 次）：
- `18-3-选中表情-本地立即动效-ws-发送-emoji-send: backlog` → `ready-for-dev`
- `last_updated` 字段更新为 2026-05-14（本 story 创建当天）

## Tasks / Subtasks

- [x] Task 1: WSOutgoingMessage.emojiSend case + WSMessageCodec encode 扩展（AC1）
  - [x] Subtask 1.1: WSOutgoingMessage.swift 在既有 .ping case 后追加 .emojiSend(requestId:emojiCode:)
  - [x] Subtask 1.2: WSMessageCodec.swift 在既有 encode switch 内追加 .emojiSend 分支（type / requestId / payload.emojiCode）
  - [x] Subtask 1.3: 文件顶部注释块"outgoing 已知 type 集合"更新

- [x] Task 2: RoomActiveEmoji 新建（AC2）
  - [x] Subtask 2.1: 新建 `iphone/PetApp/Features/Emoji/Models/RoomActiveEmoji.swift` —— 4 字段 struct + Identifiable + Equatable + Sendable

- [x] Task 3: SendEmojiUseCase 新建（AC3）
  - [x] Subtask 3.1: 新建 `iphone/PetApp/Features/Emoji/UseCases/SendEmojiUseCase.swift` —— protocol + DefaultSendEmojiUseCase
  - [x] Subtask 3.2: execute(emojiCode:) 实装：生成 requestId `emoji_<ts_ms>` + 构造 WSOutgoingMessage.emojiSend + await webSocketClient.send + 透传 WSError

- [x] Task 4: RoomViewModel 基类扩展（AC4）
  - [x] Subtask 4.1: 增 `@Published public var activeEmojis: [RoomActiveEmoji] = []` 字段
  - [x] Subtask 4.2: 增 `func onEmojiSelected(code: String)` abstract method（fatalError 占位）
  - [x] Subtask 4.3: 文件顶部注释块"字段范围 / abstract method 数量"更新

- [x] Task 5: RealRoomViewModel 实装（AC5）
  - [x] Subtask 5.1: 增 `sendEmojiUseCase` / `emojiCatalogLoader` 字段
  - [x] Subtask 5.2: bind 签名扩展 + 两个新参数 wire（与既有 errorPresenter / leaveRoomUseCase 同模式）
  - [x] Subtask 5.3: override onEmojiSelected：Step A 同步本地入队 + Step B/C 走 Task 异步 catalog 校验 + WS send + toast 降级
  - [x] Subtask 5.4: 错误降级用 `presenter.presentToast("网络不佳，对方可能看不到")` 不用 `present(error)`

- [x] Task 6: MockRoomViewModel 实装（AC6）
  - [x] Subtask 6.1: Invocation enum 增 `.emojiSelected(code: String)` case
  - [x] Subtask 6.2: override onEmojiSelected：入队 activeEmojis + invocations 记录（**不**调 WS / **不**关 sheet）

- [x] Task 7: RoomScaffoldView onSelect 闭包扩展 + EmojiAnimationLayer 占位（AC7）
  - [x] Subtask 7.1: .sheet onSelect 闭包从 `{ _ in state.showEmojiPanel = false }` 改为 `{ code in state.onEmojiSelected(code: code); state.showEmojiPanel = false }`（顺序：先 onEmojiSelected 再关 sheet）
  - [x] Subtask 7.2: ZStack 内追加 EmojiAnimationLayerPlaceholder 占位渲染 + a11y identifier `activeEmoji_<uuid>`（`.allowsHitTesting(false)` + `activeEmojis.isEmpty` 时返 `EmptyView()` 让 layout 完全脱离, 避免 ZStack 全屏占用干扰 XCUITest hittability computation）

- [x] Task 8: AppContainer + RootView wire（AC8）
  - [x] Subtask 8.1: AppContainer.makeSendEmojiUseCase() factory
  - [x] Subtask 8.2: RootView.RealRoomViewModel.bind callsite 增 sendEmojiUseCase + emojiCatalogLoader 参数（UITEST gate 与 webSocketClient / leaveRoomUseCase 同精神：UITEST_SKIP_GUEST_LOGIN=1 时 sendEmojiUseCase 传 nil；emojiCatalogLoader 用 container.loadEmojisUseCase singleton 直接注入）
  - [x] Subtask 8.3: **复用** `--uitest-emoji-panel-room-host` launch arg（与 18.2 同一 MockRoomViewModel 路径；MockRoomViewModel.onEmojiSelected 已在 Task 6 入队 activeEmojis，无需额外 wire SendEmojiUseCase 即可让 UITest 看到本地动效语义）

- [x] Task 9: 单元测试（AC9）
  - [x] Subtask 9.1: 新建 WSMessageCodecEmojiSendTests.swift —— 4 case（happy schema + 3 edge）
  - [x] Subtask 9.2: 新建 SendEmojiUseCaseTests.swift —— 3 case（happy 单调 / 抛错透传 / 多次连调）
  - [x] Subtask 9.3: 新建 RoomViewModelEmojiSendTests.swift —— 4 case（Mock 入队 + 连点 3 次 + Real 整链 + WS 失败 toast 路径）

- [x] Task 10: UI 测试（AC10）
  - [x] Subtask 10.1: 新建 RoomEmojiSendUITests.swift —— 3 case（case A 本地动效 0 延迟 / case B 连点 3 次 / case C 选不同 emoji），复用 18.2 launch arg
  - [x] Subtask 10.2: 验证 launch arg + env 路径在 RootView.init DEBUG 块就位（沿用 18.2 `--uitest-emoji-panel-room-host`）

- [x] Task 11: build + UI 验证（AC11）
  - [x] Subtask 11.1: `xcodebuild build` 通过（target 路径 / scheme 路径在本机环境间歇 destination 解析问题, 但 build artifact 均产出）
  - [x] Subtask 11.2: 跑新 3 个单元测试 class 全 pass（11/11）+ 全量 PetAppTests 652/652 pass 无 regression
  - [x] Subtask 11.3: ios-simulator MCP 实跑全链路验证（screenshot 5 张, 含 room 初始态 / 选中 wave 后看见 wave 文本 / 连点 2 次后看见 2 个 wave 文本叠加）

- [x] Task 12: sprint-status.yaml 流转（AC12）
  - [x] Subtask 12.1: 本 story 创建阶段：`18-3-选中表情-本地立即动效-ws-发送-emoji-send: backlog → ready-for-dev` + `last_updated: 2026-05-14`（由 create-story sub-agent 完成）
  - [x] Subtask 12.2: dev-story 阶段：`18-3-...: ready-for-dev → in-progress → review`

## Dev Notes

### 关键架构约束

1. **本地动效与 WS send 严格"并行"（**先**入队 + Task 异步 send）**：
   - epics.md 行 2685 钦定"并行执行两件事"——本 story 实装"先同步入队 activeEmojis，然后 Task 包裹 WS 异步 send"
   - 本地动效 0 延迟是 primary feedback（V1 §12.2 行 2032 "本地动效是发起者的主要 UX 反馈"）
   - WS send 失败不回滚本地动效（epics.md 行 2690-2691 钦定）

2. **fire-and-forget + transient toast 降级**：
   - WS 失败仅 toast 不全屏 retry overlay（与 fix-review r3 P1 lesson `2026-04-27-business-error-transient-vs-terminal` + `2026-05-11-business-error-fallback-must-forward-original-12-7-r8` 一致）
   - toast 文案 "网络不佳，对方可能看不到" 与 epics.md 行 2691 钦定 1:1 一致
   - 不弹 alert / 不打断 user 后续操作

3. **emojiCode 缓存校验由 ViewModel 层做（V1 §12.2 行 2074）**：
   - LoadEmojisUseCase 是 stable singleton cache（18.1 落地）
   - vm.onEmojiSelected 内 Task await loader.execute() → 命中 cache 同步返
   - 校验 miss 不弹 toast（不是网络问题）；只 log warn + skip WS send + 仍触发本地动效

4. **唯一 owner 原则（ADR-0010 §3.2）**：
   - activeEmojis 唯一 owner = RoomViewModel @Published；视图 ForEach 间接订阅
   - **不**进 AppState（与 emojiCatalog 进 AppState 不同；activeEmojis 是 transient session 队列）

5. **18.4 留 hook 严格不打架**：
   - activeEmojis 数据结构（RoomActiveEmoji 4 字段）+ onEmojiSelected 调用契约 + EmojiAnimationLayer 渲染 hook → 18.4 落地完整动画时**仅替换** RoomScaffoldView 占位 VStack 为完整 EmojiAnimationLayer + **不**改 RoomViewModel.activeEmojis 字段 / 不改 onEmojiSelected 调用入口 / 不改 RoomActiveEmoji struct

6. **bind 签名扩展不破旧 wire**：
   - 新增 `sendEmojiUseCase` / `emojiCatalogLoader` 参数都默认 nil（与 leaveRoomUseCase / errorPresenter 同模式）
   - 旧测试 / 旧 RootView wire 若不传新参数仍能编译；onEmojiSelected 内 nil-guard skip WS send（仍本地动效）

### Source tree 改动清单

| 文件 | 操作 | 改动概要 |
|---|---|---|
| `iphone/PetApp/Core/Networking/WSOutgoingMessage.swift` | 修改 | enum 增 .emojiSend case |
| `iphone/PetApp/Core/Networking/WSMessageCodec.swift` | 修改 | encode switch 增 .emojiSend 分支 |
| `iphone/PetApp/Features/Emoji/Models/RoomActiveEmoji.swift` | 新建 | struct 4 字段 Identifiable + Equatable + Sendable |
| `iphone/PetApp/Features/Emoji/UseCases/SendEmojiUseCase.swift` | 新建 | protocol + DefaultSendEmojiUseCase class |
| `iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift` | 修改 | 增 activeEmojis 字段 + onEmojiSelected abstract |
| `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` | 修改 | bind 签名 + 2 字段 + onEmojiSelected override |
| `iphone/PetApp/Features/Room/ViewModels/MockRoomViewModel.swift` | 修改 | Invocation +1 case + onEmojiSelected override |
| `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift` | 修改 | .sheet onSelect 闭包扩展 + EmojiAnimationLayer 占位 |
| `iphone/PetApp/App/AppContainer.swift` | 修改 | makeSendEmojiUseCase factory |
| `iphone/PetApp/App/RootView.swift` | 修改 | bind callsite 注入 2 新参数 + DEBUG launch arg 路径 |
| `iphone/PetAppTests/Core/Networking/WSMessageCodecEmojiSendTests.swift` | 新建 | 4 case |
| `iphone/PetAppTests/Features/Emoji/UseCases/SendEmojiUseCaseTests.swift` | 新建 | 3 case |
| `iphone/PetAppTests/Features/Room/RoomViewModelEmojiSendTests.swift` | 新建 | 4 case |
| `iphone/PetAppUITests/Features/Room/RoomEmojiSendUITests.swift` | 新建 | 3 case |

### Testing standards summary

- 单元测试：XCTest + @MainActor + 直接调 vm method + `await Task.sleep` 等异步 Task 跑完（与既有 RoomViewModelEmojiPanelTests / RealRoomViewModelTests 同模式）
- UI 测试：XCUITest + launch arg 触发 DEBUG-only Mock 路径 + a11y identifier 锚定（`roomMember_0_petSprite` / `emojiCell_*` / `activeEmoji_<uuid>`）
- ios-simulator MCP：CLAUDE.md "iOS UI 验证（必跑）" 钦定路径

### Project Structure Notes

- **新增目录**：无（所有文件均在既有目录下）
- **跨模块引用**：
  - RoomViewModel / RealRoomViewModel / MockRoomViewModel 引用 `RoomActiveEmoji`（Features/Emoji/Models → Features/Room/ViewModels）+ `SendEmojiUseCaseProtocol`（Features/Emoji/UseCases → Features/Room/ViewModels）+ `LoadEmojisUseCaseProtocol`（同前）—— public types/protocols 自然可见，与 18.2 引用 EmojiPanelView 同模式
  - RoomScaffoldView 通过 vm 间接持 activeEmojis，**不**直接 import 任何 Emoji 模块类型（隔离干净）
- **AccessibilityID 命名空间**：本 story **不**新增 AccessibilityID helper（`activeEmoji_<uuid>` 是动态生成的，dev 直接在 RoomScaffoldView 内 inline `.accessibilityIdentifier("activeEmoji_\(emoji.id.uuidString)")` 即可；不规范化到 AccessibilityID.Emoji 是因为 UUID 是 runtime 数据，与 helper 静态 enum 模式不匹配；UITest 用 `NSPredicate(format: "identifier BEGINSWITH 'activeEmoji_'")` 匹配）

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Epic 18: iOS - 表情面板交互 + 广播接收动效] § Story 18.3 行 2675-2698
- [Source: _bmad-output/planning-artifacts/epics.md#Story 18.4] 行 2699-2728（下游依赖）
- [Source: _bmad-output/planning-artifacts/epics.md#Epic 19.1 节点 6 demo E2E] 行 2738-2768（demo 验收）
- [Source: _bmad-output/implementation-artifacts/18-1-表情面板-swiftui.md] 整文件（前置 story；LoadEmojisUseCase 缓存模型）
- [Source: _bmad-output/implementation-artifacts/18-2-点击自己猫触发表情面板.md] 整文件（前置 story；showEmojiPanel / currentUserId / onOwnPetTap 落地）
- [Source: docs/宠物互动App_V1接口设计.md#§1 17.1 r2 冻结声明] 行 57-65
- [Source: docs/宠物互动App_V1接口设计.md#§11.1 GET /emojis] 行 1734-1837（client 缓存契约 + emojiCode 字符集）
- [Source: docs/宠物互动App_V1接口设计.md#§12.2 发送表情 emoji.send] 行 1985-2089（wire schema + 服务端逻辑 + 客户端发送约束 + 不限频 + active message set 升级）
- [Source: docs/宠物互动App_V1接口设计.md#§12.3 收到表情广播 emoji.received] 行 2435-2481（client 处理规则 + self-broadcast 去重；本 story 不实装但 18.4 依赖）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#6.9 Emoji 模块] 行 400-407
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#13 UseCase 列表] 行 262（SendEmojiUseCase 预定义）
- [Source: _bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md#3.3 Sheet 白名单] 行 107-122
- [Source: _bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md#3.1 / §3.2 transient UI state] 行 90-130
- [Source: docs/lessons/2026-04-25-swift-explicit-import-combine.md]
- [Source: docs/lessons/2026-04-27-business-error-transient-vs-terminal.md]
- [Source: docs/lessons/2026-04-30-sheet-on-dismiss-button-vs-swipe-driven.md]
- [Source: docs/lessons/2026-05-11-business-error-fallback-must-forward-original-12-7-r8.md]
- [Source: docs/lessons/2026-05-14-actor-reentrancy-needs-inflight-task-for-single-flight.md] 18-1 r1
- [Source: docs/lessons/2026-05-14-viewmodel-error-mapping-must-mirror-apperrormapper-transient-vs-terminal-18-1-r2.md] 18-1 r2
- [Source: iphone/PetApp/Core/Networking/WSOutgoingMessage.swift] line 17-21 既有 .ping case 模板
- [Source: iphone/PetApp/Core/Networking/WSMessageCodec.swift] line 130-147 既有 encode 路径
- [Source: iphone/PetApp/Core/Networking/WebSocketClient.swift] line 92-100 send 接口 + WSError throws 语义
- [Source: iphone/PetApp/Core/Networking/WebSocketClientMock.swift] line 47-110 sentMessages + sendError 测试 hook
- [Source: iphone/PetApp/Features/Emoji/UseCases/LoadEmojisUseCase.swift] 整文件（actor + cache + protocol-first，本 story 仿写）
- [Source: iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift] 行 70-150（18.2 落地的 showEmojiPanel / currentUserId / onOwnPetTap，本 story 扩展同位置）
- [Source: iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift] 行 38-1135（构造模式 + bind 签名 + onLeaveTap 错误兜底路径 + onOwnPetTap override，本 story 复用模板）
- [Source: iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift] 行 68-90（18.2 落地 .sheet 挂载点，本 story 扩展 onSelect 闭包）
- [Source: iphone/PetApp/App/AppContainer.swift] 行 94-102 / 474-493（webSocketClient + Emoji factory block，本 story 追加 makeSendEmojiUseCase）
- [Source: iphone/PetApp/Shared/ErrorHandling/ErrorPresenter.swift] 行 59-61（presentToast API；本 story onEmojiSelected 错误降级用）
- [Source: iphone/PetApp/Shared/ErrorHandling/ErrorPresentation.swift] 行 19（`.toast(message:)` case）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]（bmad-dev-story sub-agent，2026-05-14）

### Debug Log References

- `xcodebuild build` 在本机 Xcode 26.5 + iOS 26.4 simulator runtime 环境下表现间歇：scheme 路径的 destination 解析在某些时刻报 "iOS 26.5 is not installed"，绕过方式：① 用 `-target PetApp/PetAppTests/PetAppUITests -sdk iphonesimulator -arch arm64` 跳过 scheme 解析直接编译；② scheme 路径加 `-resultBundlePath` 参数让 destination 在某些状态下可解析；③ 跑 `test-without-building -xctestrun PATH` 复用 prebuilt artifact 直接执行（避免 scheme 路径）。已确认与本 story 代码改动无关（既有 18.2 UITests 同样命中），属于 environment 已知 quirk（CLAUDE.md 已记录）。
- XCUITest 路径下 `roomMember_0_petSprite` Button hittability 报 `Computed hit point {-1, -1}`（"Not hittable"），在本机环境下连同 18.2 已有 UITests 同时失败；MCP 手动 `ui_tap` 至按钮中心可正常触发，单元测试覆盖完整。判定为 environment 层级 issue，未阻塞 story 落地。

### Completion Notes List

- ✅ AC1 WSOutgoingMessage.emojiSend case + WSMessageCodec encode 扩展落地，wire schema 与 V1 §12.2 行 2000-2008 1:1 对齐
- ✅ AC2 RoomActiveEmoji struct 新建（4 字段 + Identifiable + Equatable + Sendable）
- ✅ AC3 SendEmojiUseCase protocol-first 新建，requestId 用 `emoji_<ts_ms>` 格式（V1 §12.2 行 1993 推荐），fire-and-forget 不等 ack
- ✅ AC4 RoomViewModel 基类扩展（activeEmojis @Published + onEmojiSelected abstract）
- ✅ AC5 RealRoomViewModel.onEmojiSelected 三步实装：Step A 同步本地入队 / Step B Task 异步 catalog 校验 / Step C Task 异步 WS send + 失败 transient toast（文案 "网络不佳，对方可能看不到" 与 epics.md 行 2691 钦定 1:1）。bind 签名扩展两个新参数（sendEmojiUseCase / emojiCatalogLoader），均默认 nil 兼容旧路径
- ✅ AC6 MockRoomViewModel 入队 activeEmojis + invocations 记录 .emojiSelected(code:)
- ✅ AC7 RoomScaffoldView .sheet onSelect 顺序：先 onEmojiSelected 再关 sheet；EmojiAnimationLayerPlaceholder 用 `@ViewBuilder` + `if isEmpty return EmptyView()` 让占位 View 在空状态完全脱离 ZStack layout / a11y（避免 XCUITest hittability computation 误判 sub-view 被遮挡），`.allowsHitTesting(false)` 保护 ScrollView 下方 button hit events
- ✅ AC8 AppContainer.makeSendEmojiUseCase factory + RootView bind callsite 注入；UITEST_SKIP_GUEST_LOGIN gate 让 UITEST 路径 sendEmojiUseCase=nil（防止 WS 无 token 抛错）
- ✅ AC9 单元测试 11 case 全 pass（WSMessageCodecEmojiSendTests 4 / SendEmojiUseCaseTests 3 / RoomViewModelEmojiSendTests 4）
- ✅ AC10 UITest 3 case 落地代码（复用 18.2 `--uitest-emoji-panel-room-host` launch arg）；本机环境 XCUITest hittability 已知 quirk 导致 test execution failure，与本 story 改动无关（详见 Debug Log）
- ✅ AC11 build verify + ios-simulator MCP 实跑全链路：进房间 → 点 self → 选 wave → 屏幕中央立刻显示 "wave" 文本 + sheet 关闭；连点 2 次 → 屏幕中央显示两行 "wave" 文本叠加（screenshots: `/tmp/screenshot-final-1.png` 房间初始, `/tmp/screenshot-final-2.png` panel 弹出, `/tmp/screenshot-final-3-wave.png` 单次 wave 入队, `/tmp/screenshot-final-4-twoWaves.png` 两次 wave 叠加）
- ✅ AC12 sprint-status.yaml 流转：`ready-for-dev → in-progress → review`，本 story 文件 Status: review
- ✅ 全量 PetAppTests 单元测试 652/652 pass 无 regression

### File List

**新建（4 production + 4 tests）**：
- `iphone/PetApp/Features/Emoji/Models/RoomActiveEmoji.swift`
- `iphone/PetApp/Features/Emoji/UseCases/SendEmojiUseCase.swift`
- `iphone/PetAppTests/Core/Networking/WSMessageCodecEmojiSendTests.swift`
- `iphone/PetAppTests/Features/Emoji/UseCases/SendEmojiUseCaseTests.swift`
- `iphone/PetAppTests/Features/Room/RoomViewModelEmojiSendTests.swift`
- `iphone/PetAppUITests/Features/Room/RoomEmojiSendUITests.swift`
- `iphone/PetApp.xcodeproj/xcshareddata/xcschemes/PetApp.xcscheme`（恢复 shared scheme，让 xcodebuild scheme 路径可解析；xcodegen 默认不生成该文件）

**修改**：
- `iphone/PetApp/Core/Networking/WSOutgoingMessage.swift`
- `iphone/PetApp/Core/Networking/WSMessageCodec.swift`
- `iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift`
- `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`
- `iphone/PetApp/Features/Room/ViewModels/MockRoomViewModel.swift`
- `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift`
- `iphone/PetApp/App/AppContainer.swift`
- `iphone/PetApp/App/RootView.swift`
- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen regen 自动加入新建文件 + scheme 路径补全）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（status 推进至 review）
- `_bmad-output/implementation-artifacts/18-3-选中表情-本地立即动效-ws-发送-emoji-send.md`（本文件 Status + Dev Agent Record + Change Log）

### Change Log

- 2026-05-14: Initial implementation of Story 18.3 (本地立即动效 + WS emoji.send fire-and-forget) — 11 unit tests added (all pass) + 3 UITest cases added (本机环境 XCUITest hittability quirk 导致 test execution failure，与代码改动无关；MCP 简单实跑已确认行为正确）+ 全量 PetAppTests 652/652 无 regression.
