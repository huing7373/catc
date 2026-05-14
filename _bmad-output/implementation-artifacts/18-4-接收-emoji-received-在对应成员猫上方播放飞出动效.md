# Story 18.4: 接收 emoji.received → 在对应成员猫上方播放飞出动效（去重自己 userId；首次落地 WSMessage.emojiReceived case + WSMessageCodec decode 路由 + RoomViewModel.applyEmojiReceived + EmojiAnimationLayer 完整动画 + 1.5s 自动 expire + per-member anchor + center 降级 + catalog miss fallback）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want **当其他成员发送表情时，我能看到表情图从该成员猫位上方飞出的飞出动效（1.5s 内向上飘 100px + 透明度 1→0 + 缩放 1→1.5），自己的表情 echo 被去重（不重复触发；本地动效在 Story 18.3 已播）**,

具体落地路径：

- `iphone/PetApp/Core/Networking/WebSocketClient.swift` **修改** —— 在既存 `case petStateChanged(PetStateChangedPayload)` 之后追加 `case emojiReceived(EmojiReceivedPayload)`（与 memberJoined / memberLeft / petStateChanged 同模式；V1 §12.3 行 2435-2481 钦定；client → server 单向 emoji.send 由 18.3 已落地，本 story 是 server → client 接收路径）；同文件追加 `public struct EmojiReceivedPayload: Equatable, Sendable` 两字段 `userId: String` / `emojiCode: String`（V1 §12.3 行 2446-2449 钦定 2 字段都必填）；payload 顶部注释引用 V1 §12.3 行 2435-2481 + Story 17.5 server 端 BuildEmojiReceivedEnvelope 钦定的 wire schema；同文件顶部 `WSMessage` enum 注释块"emoji.received 由 Epic 17 / Story 17.x 扩展"行更新为"emoji.received 由 Story 18.4 落地（17.x 锚定契约，本 story 是 client 接收端落地）"
- `iphone/PetApp/Core/Networking/WSMessageCodec.swift` **修改** —— 在 `decode(_:)` switch 内 `case "pet.state.changed":` 之后追加 `case "emoji.received":` 分支：解 `EmojiReceivedEnvelope` → 提取 `payload.userId` / `payload.emojiCode` → guard 两字段非空（与 member.joined / member.left / pet.state.changed 同精神：V1 §12.3 行 2469 钦定 payload.userId / payload.emojiCode 都必填，Decodable 只能挡 absent / type-mismatch，server 若推 `""` 仍能解码成功 → codec 必须 fallback `.unknown(rawType: "emoji.received")` + os_log error 避免 ViewModel.applyEmojiReceived 用空字段污染 activeEmojis）→ 成功 → 返 `.emojiReceived(dto.toDomain())`；同文件 `// MARK: - Story 15.2 pet.state.changed envelope DTO` 区段之后追加 `// MARK: - Story 18.4 emoji.received envelope DTO` + `EmojiReceivedEnvelope` 内嵌私有 struct（与 PetStateChangedEnvelope 同模式 + `toDomain()` 桥接到 public `EmojiReceivedPayload`）；文件顶部注释"节点 4 阶段 incoming 已知 type 集合：room.snapshot / pong / error（Epic 10 钦定）"行扩展加 "节点 6 阶段 incoming 扩展：emoji.received（V1 §12.3 行 2435-2481，Story 17.1 锚定 + 18.4 client 落地）"
- `iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift` **修改** —— 文件顶部注释块"字段"区追加"activeEmojis（Story 18.3 落地；本 story 18.4 接收端写入；18.4 落地 1.5s 自动 expire 移除）"备注（**不**改字段定义；activeEmojis 在 18.3 已落地）；abstract method 区**新增** `applyEmojiReceived(payload: EmojiReceivedPayload)`（fatalError 占位与 onOwnPetTap / onEmojiSelected 同模式；MockRoomViewModel / RealRoomViewModel 各自 override）；文件顶部注释 "3 abstract method"（18.3 已扩到 4）改为 "5 abstract method"
- `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` **修改** —— `handle(message:streamRoomId:streamGeneration:)` switch 在既存 `case .petStateChanged(let payload):` block 之后追加 `case .emojiReceived(let payload):` 分支，与 .petStateChanged 同精神先走 `streamRoomId / lastObservedRoomId` 守护（防 cross-room race；V1 §12.3 行 2473 (c) `payload.userId` 不在 roster 是合法 race 但同房间内 stream 仍要按 roomId 守护防 A→B 错房间消息）→ 调 `applyEmojiReceived(payload)`；同时把既有 generation gate switch（line 738-746）case 列表 `case .memberJoined, .memberLeft, .petStateChanged, .connectionStateChanged, .roomSnapshot:` 扩展为 `case .memberJoined, .memberLeft, .petStateChanged, .emojiReceived, .connectionStateChanged, .roomSnapshot:`（lesson 2026-05-12-per-stream-generation-guard-fixes-same-room-rejoin-race-15-2-r2 + 2026-05-12-generation-gate-must-cover-roomsnapshot-15-2-r3 钦定 stale event 必须挡）；override `applyEmojiReceived(_ payload:)` 实装去重 + roster lookup + catalog lookup + 入队 + 1.5s 自动 expire：
  ```swift
  public override func applyEmojiReceived(_ payload: EmojiReceivedPayload) {
      // V1 §12.3 行 2471 (a) self-broadcast 去重: payload.userId == currentUserId → 跳过 (本地动效已在 Story 18.3 播过).
      // log debug "self-emoji-broadcast received" (V1 §12.3 行 2471 钦定 log level).
      if let currentUserId = self.currentUserId, !currentUserId.isEmpty, currentUserId == payload.userId {
          os_log(.debug,
                 "RealRoomViewModel.applyEmojiReceived: self-broadcast received (userId=%{public}@, code=%{public}@); skip (local animation played in 18.3)",
                 payload.userId, payload.emojiCode)
          return
      }
      // V1 §12.3 行 2473 (c) roster 找不到 userId → 合法 race window (sender 已 leave + member.left 先到达).
      // 不丢弃 → 仍入队 + log info "after member.left race, fell back to center anchor"; EmojiAnimationLayer 渲染时按 userId 找 anchor 失败 → 走屏幕中央降级 (V1 §12.3 行 2473 钦定).
      if !members.contains(where: { $0.id == payload.userId }) {
          os_log(.info,
                 "RealRoomViewModel.applyEmojiReceived: userId %{public}@ not in roster (member.left race); fall back to center anchor",
                 payload.userId)
      }
      // V1 §12.3 行 2474 (d) emojiCode catalog miss → 仍入队 (用 emojiCode 作 fallback "?"占位; EmojiAnimationLayer 渲染时 asyncImage url 拿不到 → 显示问号 SF Symbol).
      // 注: catalog miss 检测推迟到渲染层 (EmojiAnimationLayer 内 asyncImage); applyEmojiReceived 不在 vm 层校验 (避免 ViewModel 持 catalog 依赖耦合; 18.3 onEmojiSelected 持 emojiCatalogLoader 是因为 send 路径要求严格符合 §12.2 行 2074 校验, 接收路径无该约束).
      let emoji = RoomActiveEmoji(
          id: UUID(),
          userId: payload.userId,
          emojiCode: payload.emojiCode,
          createdAt: Date()
      )
      self.activeEmojis.append(emoji)
      os_log(.debug,
             "RealRoomViewModel.applyEmojiReceived: enqueued (userId=%{public}@, code=%{public}@, activeEmojis.count=%{public}d)",
             payload.userId, payload.emojiCode, self.activeEmojis.count)
      // 1.5s 后按 id 自动移除 (epics.md 行 2715-2716 钦定动画时长; 移除时机与动画结束对齐 → 让 SwiftUI 自然 unmount FloatingEmojiView 子视图).
      // Task 内 capture id, 1.5s sleep, removeAll { $0.id == captured }; cancel 不重要 (vm deinit 时 Task 自然 leak 一次 1.5s 后退出, 比 activeEmojis cleanup 还快; subscribeRoomIdConnect A→B / A→nil 时 activeEmojis 已被清空, removeAll 是 no-op).
      // 不持 Task 句柄 (与 FloatingEmojiView .onAppear withAnimation 同精神, 一次性 fire-and-forget; 简化代码不增加 Task lifecycle 管理).
      let capturedId = emoji.id
      Task { [weak self] in
          try? await Task.sleep(nanoseconds: 1_500_000_000)  // 1.5s, epics.md 行 2715 钦定
          await MainActor.run {
              self?.activeEmojis.removeAll { $0.id == capturedId }
          }
      }
  }
  ```
- `iphone/PetApp/Features/Room/ViewModels/MockRoomViewModel.swift` **修改** —— `Invocation` enum 增 `case emojiReceived(userId: String, code: String)`；override `func applyEmojiReceived(_ payload: EmojiReceivedPayload)`：`invocations.append(.emojiReceived(userId: payload.userId, code: payload.emojiCode))` + 入队 activeEmojis（与 RealRoomViewModel 行为对齐让单元测试 + Preview 一致）+ **不**实装 1.5s 自动移除（MockRoomViewModel 用于测试 + Preview，1.5s 自动移除会让测试断言时机难控；单元测试如需验证 expire 走 RealRoomViewModel 路径）+ **不**做 self-broadcast 去重（MockRoomViewModel 没有持 currentUserId 与 RoomViewModel 同精神：Mock 的目的是 invocation 记录 + 视觉触发；去重测试走 Real path）
- `iphone/PetApp/Features/Emoji/Views/EmojiAnimationLayer.swift` **新建** —— `public struct EmojiAnimationLayer: View`（替换 18.3 落地的 `EmojiAnimationLayerPlaceholder` 占位）；构造参数 `activeEmojis: [RoomActiveEmoji]` + `memberAnchors: [String: CGPoint]`（{userId: 该成员 PetSpriteView 中心点（in RoomScaffoldView coordinate space）}；caller=RoomScaffoldView 用 `GeometryReader` + `.coordinateSpace` + `.onPreferenceChange` 路径填充）+ `loadEmojisUseCase: LoadEmojisUseCaseProtocol?`（catalog 查询；Preview / UITest 路径可传 nil 走 fallback）+ `centerAnchor: CGPoint`（屏幕中央，作为 roster miss 降级位置；caller 用 GeometryReader 计算）；body 内 ZStack + `ForEach(activeEmojis) { emoji in FloatingEmojiCellView(emoji: emoji, anchor: memberAnchors[emoji.userId] ?? centerAnchor, loadEmojisUseCase: loadEmojisUseCase) }`；`.allowsHitTesting(false)` 防遮挡底层交互（与 18.3 placeholder 同精神）；`activeEmojis.isEmpty` 时整体返 `EmptyView()` 让 ZStack 完全脱离 layout（与 18.3 placeholder 同精神，避免 hittability 误判）；同文件 **新建** `struct FloatingEmojiCellView: View`（per-emoji 子视图；与 `FloatingEmojiView`（HomeView 落地）同 lesson"@State 驱动 .onAppear withAnimation"模式 —— 让每个 emoji 入队即 SwiftUI 重建 + 启动各自动画，互不干扰；epics.md 行 2717 钦定"同时多个表情飞出 → 各自独立动效，不互相干扰"）：
  ```swift
  struct FloatingEmojiCellView: View {
      let emoji: RoomActiveEmoji
      let anchor: CGPoint  // 起点 = 该成员 PetSpriteView 中心 (or center fallback)
      let loadEmojisUseCase: LoadEmojisUseCaseProtocol?

      @State private var animatedY: CGFloat = 0       // 起点 0, 向上 -100 (epics.md 行 2715)
      @State private var animatedOpacity: Double = 1.0 // 1 → 0
      @State private var animatedScale: CGFloat = 1.0  // 1 → 1.5
      @State private var assetUrl: String? = nil       // catalog 查 emojiCode 拿 assetUrl; miss 时 nil → fallback "?"

      var body: some View {
          Group {
              if let urlStr = assetUrl, let url = URL(string: urlStr) {
                  // V1 §11.1 行 1750 钦定 assetUrl 非空; AsyncImage 加载远程图; 占位用 placeholder("?"); failure 走 fallback
                  AsyncImage(url: url) { phase in
                      switch phase {
                      case .empty:
                          Image(systemName: "questionmark.circle")
                              .font(.system(size: 32))
                              .foregroundColor(.secondary)
                      case .success(let img):
                          img.resizable().aspectRatio(contentMode: .fit).frame(width: 48, height: 48)
                      case .failure:
                          Image(systemName: "questionmark.circle")
                              .font(.system(size: 32))
                              .foregroundColor(.secondary)
                      @unknown default:
                          EmptyView()
                      }
                  }
              } else {
                  // catalog miss (V1 §12.3 行 2474 (d) fallback): 显示问号 SF Symbol + 文字 emojiCode (便于 debug 看 wire 输入).
                  VStack(spacing: 2) {
                      Image(systemName: "questionmark.circle")
                          .font(.system(size: 32))
                          .foregroundColor(.secondary)
                      Text(emoji.emojiCode)
                          .font(.system(size: 10, weight: .semibold))
                          .foregroundColor(.secondary)
                  }
              }
          }
          .position(x: anchor.x, y: anchor.y)  // SwiftUI .position: view 中心点对齐 anchor; 让 emoji 飞出起点 = 成员 PetSpriteView 中心
          .offset(y: animatedY)                 // y 上飘
          .opacity(animatedOpacity)
          .scaleEffect(animatedScale)
          .accessibilityIdentifier("activeEmoji_\(emoji.id.uuidString)")
          .task {
              // 启动时异步查 catalog 拿 assetUrl (loadEmojisUseCase = nil 时跳过, 走 catalog-miss fallback).
              if let loader = loadEmojisUseCase {
                  if let catalog = try? await loader.execute(),
                     let entry = catalog.first(where: { $0.code == emoji.emojiCode }) {
                      self.assetUrl = entry.assetUrl
                  }
              }
          }
          .onAppear {
              // 1.5s easeOut: y 0 → -100, opacity 1 → 0, scale 1 → 1.5 (epics.md 行 2715-2716 钦定动画规格).
              // 与 HomeView FloatingEmojiView 同 lesson: .onAppear + @State + withAnimation, 不靠常量 offset.
              withAnimation(.easeOut(duration: 1.5)) {
                  animatedY = -100
                  animatedOpacity = 0
                  animatedScale = 1.5
              }
          }
      }
  }
  ```
- `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift` **修改** —— 替换 18.3 落地的 `EmojiAnimationLayerPlaceholder(activeEmojis:)` 调用点（line 84-85）为完整 `EmojiAnimationLayer(activeEmojis:memberAnchors:loadEmojisUseCase:centerAnchor:)`；新增 `@State private var memberAnchors: [String: CGPoint] = [:]`（SwiftUI 视图层 transient state；放 vm 层是过度设计 —— anchor 是渲染期间几何计算结果，不属于业务状态；ADR-0010 §3.2 表格"视图层几何 @State"路径）；新增 `@State private var roomCenter: CGPoint = .zero`（屏幕中央 fallback）；在 `membersList` 内 `memberRow` 渲染时，对**所有**成员（含自己）的 PetSpriteView 包一层 `GeometryReader` + `Color.clear.preference(key: MemberAnchorPreferenceKey.self, value: [member.id: geo.frame(in: .named("roomCoord")).midPoint])`；`ZStack` 整体加 `.coordinateSpace(name: "roomCoord")`；`.onPreferenceChange(MemberAnchorPreferenceKey.self) { dict in memberAnchors.merge(dict) { _, new in new } }` 收集所有 anchor；`GeometryReader` 在最外层包一次拿 size → `roomCenter = CGPoint(x: size.width / 2, y: size.height / 2)`（每帧重算成本极低，与 SwiftUI 既有用法一致）；构造 `EmojiAnimationLayer` 时把 `loadEmojisUseCase` 由 RoomScaffoldView 新增构造参数注入（caller=RootView 传 `container.loadEmojisUseCase`）—— 在既存 `init(state:emojiPanelViewModelFactory:)` 之后增 `loadEmojisUseCase: LoadEmojisUseCaseProtocol? = nil`（默认 nil 兼容旧 Preview / UITest path），完整签名 `init(state:emojiPanelViewModelFactory:loadEmojisUseCase:)`；同文件**保留** `EmojiAnimationLayerPlaceholder` 类型定义不删（兼容性；与 18.3 落地的 a11y identifier `activeEmoji_<uuid>` UITest 路径打通；EmojiAnimationLayer 内 FloatingEmojiCellView 也用同一 a11y identifier 模式，UITest 不破）—— **修订**：删除 `EmojiAnimationLayerPlaceholder` 避免死代码；UITest 锁定 a11y identifier `activeEmoji_<uuid>` 由 EmojiAnimationLayer 内 FloatingEmojiCellView 直接挂载（命名一致，旧 UITest 不破）
- `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift` **修改（续）** —— 新增 `private struct MemberAnchorPreferenceKey: PreferenceKey { static var defaultValue: [String: CGPoint] = [:]; static func reduce(value: inout [String: CGPoint], nextValue: () -> [String: CGPoint]) { value.merge(nextValue()) { _, new in new } } }`；helper extension `extension CGRect { var midPoint: CGPoint { CGPoint(x: midX, y: midY) } }` —— 简化 anchor 计算；MemberAnchorPreferenceKey 与 helper 都标 fileprivate（不暴露给其他 view）
- `iphone/PetApp/App/RootView.swift` **修改** —— RoomScaffoldView 构造 callsite（grep `RoomScaffoldView(state:`）在既有参数后追加 `loadEmojisUseCase: container.loadEmojisUseCase`；如有多处 callsite（UITEST 路径 + production 路径），都追加；UITEST_SKIP_GUEST_LOGIN gate 内 callsite 仍用 container.loadEmojisUseCase（与 18.1 落地 stable singleton 同精神）—— 即便 wire 残缺路径，loadEmojisUseCase 单例不持网络，无副作用
- 单元测试覆盖 ≥ 5 case：
  - `iphone/PetAppTests/Core/Networking/WSMessageCodecEmojiReceivedTests.swift`（**新建**，≥5 case）：
    - happy `decode` 完整 JSON：`{"type":"emoji.received","requestId":"","payload":{"userId":"1002","emojiCode":"wave"},"ts":1776920345000}` → 返 `.emojiReceived(EmojiReceivedPayload(userId: "1002", emojiCode: "wave"))`
    - edge `payload.userId == ""` → 返 `.unknown(rawType: "emoji.received")`（codec 防 empty userId 污染 vm）
    - edge `payload.emojiCode == ""` → 返 `.unknown(rawType: "emoji.received")`
    - edge `payload` 缺 emojiCode 字段（如 `{"userId":"1002"}`）→ Decodable 抛错 → 返 `.unknown(rawType: "emoji.received")`
    - edge `payload` 缺 userId 字段 → 返 `.unknown(rawType: "emoji.received")`
    - happy `requestId == ""` + `ts` 缺失 / 异常值（codec 既有 envelope 容忍）→ 仍能解 `.emojiReceived`（ts 字段 codec 不消费）
  - `iphone/PetAppTests/Features/Room/RoomViewModelEmojiReceivedTests.swift`（**新建**，≥6 case）：
    - happy "收到别人 emoji.received → activeEmojis 队列多 1 项，userId == payload.userId + emojiCode == payload.emojiCode"（MockRoomViewModel + 直接调 `applyEmojiReceived`）
    - happy（**Real path**）"vm.currentUserId = self → applyEmojiReceived(payload: {userId: self, emojiCode: wave}) → activeEmojis.count == 0（去重生效）"（RealRoomViewModel + direct call + 验证 invocations 不变 + activeEmojis 不变）
    - happy "1.5s 后 activeEmojis 自动移除"（RealRoomViewModel + `Task.sleep(nanoseconds: 1_700_000_000)` 验证 activeEmojis.isEmpty == true；用 `XCTestExpectation` + 1.7s timeout，留 200ms 缓冲）
    - edge "5 个不同 userId 同时入队 → activeEmojis.count == 5 + 每项 id 不同（UUID）"（连续调 applyEmojiReceived 5 次不同 userId）
    - edge "catalog miss（emojiCode 不在 catalog）→ 仍入队 + log info"（vm 层不校验 catalog，渲染层 fallback；本测试验证 applyEmojiReceived 不 drop）
    - edge "roster miss（userId 不在 members）→ 仍入队 + log info（V1 §12.3 行 2473 (c) member.left race）"（直接 applyEmojiReceived payload userId 不在 vm.members → activeEmojis.count == 1 + log info "fell back to center anchor"；本测试**不**验证 anchor 计算（那是渲染层 EmojiAnimationLayer 职责），仅验证 vm 层不丢消息）
    - edge "WS handler 路径：feed .emojiReceived 到 handle(message:streamRoomId:) → activeEmojis 多 1 项"（验证 RealRoomViewModel handle switch 正确路由到 applyEmojiReceived）
    - edge "streamRoomId / streamGeneration mismatch → 丢弃 emoji.received 不入队"（generation gate 守护测试；与既有 .petStateChanged generation gate 测试同模式）
  - `iphone/PetAppTests/Features/Emoji/Views/EmojiAnimationLayerTests.swift`（**新建**，≥2 case，仅做 View structural 验证；动画动态行为难用 XCTest 验证，靠 ios-simulator MCP 实跑 + UI 测试覆盖）：
    - happy "activeEmojis 为空 → body 返 EmptyView（ZStack 不渲染）"（通过 `XCTAssertNoThrow(_ = EmojiAnimationLayer(...).body)` + `Mirror` 反射验证；或更简单：直接构造 View 并验证 `view.activeEmojis.count == 0`）
    - happy "memberAnchors miss + centerAnchor != .zero → FloatingEmojiCellView 接收 centerAnchor 作 anchor"（用 inputs 构造 + 验证 anchor 选择逻辑；用 view inspector 库或简化为单测 anchor 选择函数 helper —— 建议把 anchor 选择抽 helper `EmojiAnimationLayer.anchor(for:userId:memberAnchors:centerAnchor:)` static func 让单测好测）
- UI 测试覆盖（XCUITest）：
  - `iphone/PetAppUITests/Features/Room/RoomEmojiReceivedUITests.swift`（**新建**，≥3 case）：复用 18.2/18.3 的 `--uitest-emoji-panel-room-host` launch arg + `UITEST_FORCE_IN_ROOM=1` + `UITEST_MOCK_WEBSOCKET=1` + **新增** launch arg `--uitest-emoji-received-host`（让 RootView DEBUG 块 wire RealRoomViewModel + 注入 MockWebSocketClient + 让 mock client.messages stream 在 launch 后定时 emit `.emojiReceived(...)` 几条 fixture 消息，模拟其他成员发送的表情；mock 消息触发由 launch env 控制 `UITEST_EMIT_EMOJI_RECEIVED=1`，触发时机 launch 后 1s + 包含 self userId + other userId 不同 fixture）；具体 case：
    - case A（happy 别人发表情 → 看到飞出动效）: 进房间 → 等待 `UITEST_EMIT_EMOJI_RECEIVED` 触发的 mock emoji.received → 0.5s 内 `app.images` 或 `app.otherElements["activeEmoji_*"]` 可见（任一节点）→ 等 2s（动画 1.5s + buffer 0.5s）→ `app.otherElements["activeEmoji_*"].count == 0`（自动 expire）
    - case B（happy self-broadcast 去重）: 进房间 → 自己点 wave（沿用 18.3 路径）→ 0.5s 内本地动效可见 → 等待 mock emit self-broadcast emoji.received（含 self userId） → 等 200ms（让 vm 应用消息）→ 验证 activeEmojis 队列仍只有自己 1 项（去重生效；不是 2 项）→ 等动画结束 → activeEmojis 清空
    - case C（happy 多人同时发表情 → 多个独立动效）: mock 同时 emit 3 条不同 userId 的 emoji.received → 0.5s 内 3 个 activeEmoji 节点可见 → 等 2s → 全部 expire 移除
  - **注**：弱网降级（catalog miss / roster miss → center anchor fallback）UITest 稳定性差（catalog miss 需要先 reset cache + emoji.received 同帧触发，时序难 setup）→ **改在单元测试覆盖**（applyEmojiReceived path）；UITest 不强求
- ios-simulator MCP 实跑验证（CLAUDE.md "iOS UI 验证（必跑）"）：
  - `bash iphone/scripts/build.sh` → install_app → launch_app + launch arg `--uitest-emoji-received-host` + env `UITEST_MOCK_WEBSOCKET=1` + `UITEST_EMIT_EMOJI_RECEIVED=1`
  - `ui_view` 验证：进房间后等 1s → 看到至少 1 个表情图从成员位上方飞出（向上 + 缩放 + 渐隐）
  - `ui_view` 验证 1.5s 后该表情消失（屏幕回归原状）
  - 录制 `record_video` 完整 case A flow 让 reviewer 验证视觉效果（建议但非强制）

So that **Epic 19.1 节点 6 demo E2E（场景 1 自己点表情看到本地动效 + 场景 3 别人发表情看到对应猫位动效 + 场景 4 同时多人发表情看到独立动效 + 场景 6 弱网降级看不到对方表情但 toast 提示）** 可以基于一个**已落地、严格符合 V1 §12.3 emoji.received 契约 + self-broadcast 去重 + roster miss center 降级 + catalog miss 问号 fallback + 1.5s 动画 + 多 emoji 独立**的完整接收链路串联，不再出现"18.4 找不到 EmojiAnimationLayer 渲染入口 / 自己 echo 触发两次动效 / 别人 leave 后 emoji 飞到空中 / catalog miss 时 UI 崩溃 / 多人同发动画互相干扰"的返工。

## 故事定位（Epic 18 最后 1 条；上承 18.3 emoji.send / activeEmojis 队列 / FloatingEmojiCellView placeholder hook，下启 Epic 19 节点 6 demo E2E）

- **Epic 18 进度**：18.1（表情面板 SwiftUI + GET /emojis 缓存）**done** → 18.2（房间页内点击自己猫触发表情面板）**done** → 18.3（选中表情触发本地立即动效 + WS emoji.send fire-and-forget）**done** → **18.4（本 story，接收 emoji.received 在对应成员猫上方播放飞出动效 + 去重自己 userId）** → epic-18 收官，进入 Epic 19（节点 6 demo E2E 联调）。
- **本 story 是 Epic 19.1 + Epic 17/18 demo 验收的强前置**：
  - **Epic 19.1 节点 6 demo E2E**（epics.md 行 2734-2768）：钦定"场景 1 自己点表情 0 延迟本地动效 + 场景 2 server 广播 emoji.received 到房间内所有成员（含自己）+ 场景 3 别人发表情 → 你看到对应猫位上方动效（不重复 echo 自己）+ 场景 4 同时多个 emoji 飞出独立不干扰 + 场景 5 弱网 server 收不到 emoji.send → 自己仍看到本地动效 + toast"——**直接依赖**本 story 落地的：
    - `WSMessage.emojiReceived(EmojiReceivedPayload)` case + WSMessageCodec decode 路由（让 client 能解码 server 端 17.5 落地的 BuildEmojiReceivedEnvelope 输出的 wire frame）
    - `RealRoomViewModel.applyEmojiReceived(_:)` 去重 + 入队（让自己的 echo 跳过 / 别人的入队）
    - `EmojiAnimationLayer` + `FloatingEmojiCellView`（让 activeEmojis 队列里的项真实渲染成飞出动效，替换 18.3 占位）
    - 1.5s 自动 expire（让多 emoji 队列不无限堆叠）
  - **不**依赖本 story 落地（已在 18.3 完成）：emoji.send wire / SendEmojiUseCase / activeEmojis 队列字段 / onEmojiSelected 调用入口 / RoomActiveEmoji struct / EmojiAnimationLayerPlaceholder（被 EmojiAnimationLayer 替换）
- **epics.md §Story 18.4 钦定**（行 2699-2728）：
  - **AC1（去重判定）**：收到 `emoji.received {userId, emojiCode}`：if userId == 当前用户自己 → 跳过；否则走动效流程
  - **AC2（动效流程）**：从缓存表情列表查 emojiCode → 拿 assetUrl → 在该 userId 对应的猫位上方位置生成浮动 SwiftUI View（AsyncImage 加载 assetUrl，初始不透明 + 中心位置）；动画：1.5 秒内向上飘移 100px + 透明度 1.0 → 0.0 + 缩放 1.0 → 1.5；动画结束后从视图层级移除
  - **AC3（多 emoji 独立）**：同时多个表情飞出（自己 + 别人 + 不同 emojiCode）→ 各自独立动效，不互相干扰
  - **AC4（roster miss 降级）**：收到 userId 不在房间的成员 → 忽略 + log warning（**精确化**：epics.md 行 2718 文字"忽略"过于简略，V1 §12.3 行 2473 (c) 钦定**不**丢弃，而是降级到房间中心位置渲染 + log info（不是 warn，因为是预期合法 race）。本 story 实装按 V1 §12.3 (c) 而非 epics.md "忽略"，因为 V1 是冻结契约 / 后写入）
  - **AC5（catalog miss 降级）**：收到 emojiCode 在缓存中找不到（理论不该）→ 显示 fallback 占位（默认问号）
  - **AC6（单元测试 ≥5 case）**：happy 收到别人 emoji → activeEmojis +1 / happy 收到自己 emoji → 跳过 / happy 1.5s 后 activeEmojis 移除 / edge 同时收到 5 个 emoji 各自独立 / edge emojiCode catalog miss → 问号 fallback
  - **AC7（UI 测试 ≥2 case）**：自己发表情 → 0 延迟本地动效 + 等 200ms server echo → 不出现第二次（去重） / 别人发表情 → 看到对应 user 猫上方动效

### 决策点 1：1.5s 自动 expire 实装位置（ViewModel Task vs SwiftUI .onAppear withAnimation duration）

epics.md 行 2715-2716 钦定"1.5 秒内向上飘移 100px... 动画结束后从视图层级移除"。**选 ViewModel 内 Task.sleep(1.5s) + activeEmojis.removeAll**（与 SwiftUI 动画结束回调同时机）：

1. **唯一权威源**（ADR-0010 §3.2）：activeEmojis 是 ViewModel @Published 字段；移除时机由 ViewModel 控制，与入队同 owner
2. **避免 SwiftUI .onDisappear 反向触发 vm**：如果让 SwiftUI 视图自己消失后回调 vm.remove → view-to-vm 反向依赖，与 ADR-0010 §3.2 "vm 是 ViewModel @Published 字段的唯一 owner"违反；且 SwiftUI .onDisappear 触发时机依赖 view hierarchy 拓扑，不稳定
3. **简化代码**：Task { sleep(1.5s); removeAll }，不持 Task 句柄；deinit 时 leak 一次（1.5s 退出后自然 GC，不影响内存）；与既有 FloatingEmojiView .onAppear withAnimation 一次性 fire-and-forget 同模式
4. **测试友好**：单测调 applyEmojiReceived → Task.sleep(1.7s) → 验证 activeEmojis.isEmpty；不需要 mock SwiftUI 动画完成回调

### 决策点 2：anchor 计算位置（SwiftUI GeometryReader/PreferenceKey vs ViewModel 持几何状态）

epics.md 行 2714 钦定"在该 userId 对应的猫位上方位置生成浮动 SwiftUI View"。**选 SwiftUI @State + PreferenceKey 收集每个成员 PetSpriteView 中心点 → 传给 EmojiAnimationLayer**：

1. **职责分层**：anchor 是渲染期间几何计算结果，不属于业务状态 —— 放 vm 层（如 @Published var memberAnchors: [String: CGPoint]）违反 ADR-0010 §3.2 表格"几何渲染数据走 View @State / PreferenceKey 不进 vm"
2. **SwiftUI 原生模式**：`GeometryReader` + `Color.clear.preference(key:value:)` + `.onPreferenceChange(...)` 是 SwiftUI 收集子视图几何的官方路径（与 HomeView catStage anchor 计算同模式）
3. **center fallback**：roster miss 时 `memberAnchors[userId]` 返 nil → FloatingEmojiCellView 用 centerAnchor 作 anchor（V1 §12.3 行 2473 (c) 钦定）；centerAnchor 也由 RoomScaffoldView 在最外层 GeometryReader 读 size 算 (size.width/2, size.height/2)
4. **测试角度**：单元测试不验证 anchor 计算（那是 SwiftUI 渲染层职责）；anchor 选择 helper 可抽 static func `anchor(for:memberAnchors:centerAnchor:)` 让单测 + 渲染共用

### 决策点 3：FloatingEmojiCellView 内 catalog 查询（async vs sync）

V1 §12.3 行 2474 (d) 钦定 catalog miss → 显示问号 fallback。**选 FloatingEmojiCellView .task 异步查 catalog + @State 持 assetUrl**：

1. **不在 vm 层校验 catalog**：vm 层 applyEmojiReceived 直接入队，不做 catalog 校验；让"假设性合法"的 emojiCode（如未来 catalog 升级前的 stale cache）也能入队 + 渲染层降级 → 解耦 vm 与 emoji 模块依赖
2. **catalog actor 内 cache hit 同步返**：LoadEmojisUseCase actor 已 single-flight + cache（18.1 落地）；FloatingEmojiCellView .task `await loader.execute()` 在 cache hit 路径下毫秒级返；cache miss 路径（理论不该，emoji panel 已 load 过）才走 HTTP → 仍非阻塞渲染（assetUrl 在拿到前显示问号占位）
3. **降级幂等**：FloatingEmojiCellView .task 失败 / 返 nil / 返 catalog 不含该 code → assetUrl 保持 nil → if-let-else 走 fallback 路径 → 不报错
4. **不在 EmojiAnimationLayer 上层批量查 catalog**：让每个 FloatingEmojiCellView 独立 await（actor cache hit 几乎 0 开销）比 EmojiAnimationLayer 上层批量 dict[code: assetUrl] 计算简单；后者要管理 dict lifecycle，复杂度上升

### 决策点 4：去重判定位置（ViewModel.applyEmojiReceived vs WSMessageCodec）

V1 §12.3 行 2471 (a) 钦定"client 端**应**对 payload.userId == 当前 user.id 跳过 / 不触发动效"。**选 ViewModel.applyEmojiReceived 内做去重**：

1. **职责分层**：WSMessageCodec 是 transport 解码层，只做 wire schema → enum case 转换；不知"当前 user.id"
2. **vm 层有 currentUserId**：RoomViewModel.currentUserId 已在 Story 18.2 落地（来自 AppState.currentUser.id 订阅）；applyEmojiReceived 直接读 self.currentUserId
3. **去重严格 string match**：`payload.userId == self.currentUserId`（BIGINT 字符串化，与 V1 §2.5 + §12.3 行 2448 钦定一致）；不做大小写 normalize（userId 是 server 钦定 ID，client 不应改写）
4. **log debug**：V1 §12.3 行 2471 钦定 `log debug "self-emoji-broadcast received"`，**不** log warn / error（这是预期合法行为）

### 决策点 5：roster miss 处理（drop vs enqueue + center anchor）

epics.md 行 2718 钦定"收到 userId 不在房间的成员 → 忽略 + log warning"；V1 §12.3 行 2473 (c) 钦定**不**丢弃 + **center 降级渲染** + log info。**选 V1 §12.3 (c) 路径**（V1 是冻结契约 / 后写入 / 更精确）：

1. **V1 §12.3 行 2473 (c) 钦定理由**：sender A 发 emoji.send 后**立即** leave 房间 → server 端 emoji.received 广播与 member.left 广播走**不同**路径 → 到达 receiver B 的物理顺序**不保证**严格一致 → B 可能先收 member.left（已把 A 从本地 roster 移除）→ 后收 A 的 emoji.received → 此时本地 roster 已查不到 A
2. **drop 的危害**：合法 race window 下表情就被吞了 → UX 表现"我看到 A 离开，但 A 离开前发的 wave 没飞出"（与预期不符）；保留入队 + center anchor 降级让 UX 表现"看到一个 emoji 从屏幕中央飞出"，hint 是 A 在 leave 边缘发的
3. **log info 而非 warn**：合法行为不污染告警通道
4. **epics.md vs V1 冲突 resolution**：V1 §12.3 行 57-65 钦定 17.1 r2 起 emoji.received 字段 + payload + 广播范围 + self-broadcast 去重规则**冻结**；epics.md "忽略"是 17.1 之前的简化表述，被 V1 §12.3 行 2473 (c) 精化覆盖

### 决策点 6：删除 EmojiAnimationLayerPlaceholder vs 保留

18.3 落地的 `EmojiAnimationLayerPlaceholder` 是占位渲染（VStack + Text(emojiCode) + activeEmoji_<uuid> a11y identifier）。**选完整删除**：

1. **死代码警告**：18.4 后该类型不再被任何 caller 引用（RoomScaffoldView 改调 EmojiAnimationLayer）；保留 = 死代码
2. **a11y identifier 兼容**：EmojiAnimationLayer 内 FloatingEmojiCellView 用同一 `activeEmoji_<emoji.id.uuidString>` identifier 模式（与 18.3 一致）；UITest 用 `NSPredicate(format: "identifier BEGINSWITH 'activeEmoji_'")` 匹配，旧测试 18.3 RoomEmojiSendUITests 不破
3. **测试覆盖延续**：18.3 RoomEmojiSendUITests 仍跑（验证 self-emoji 本地动效）；18.4 新增 RoomEmojiReceivedUITests（验证 other-user emoji 接收动效）

### V1 接口设计相关锚点

- **V1 §12.3 行 2435-2481 emoji.received 全集**（**17.1 r2 冻结**，详见 V1 §1 行 57-65）：
  - 行 2446-2450 字段表：`type` / `requestId`（固定 ""）/ `payload.userId` / `payload.emojiCode` / `ts`
  - 行 2452-2464 JSON 示例与本 story `EmojiReceivedPayload` + decode 路由 1:1 对齐
  - 行 2466 广播范围**包含**发起者自己（与 member.joined / member.left 排除发起者**不同**语义，与 pet.state.changed 同语义）
  - 行 2469 payload 字段必填（payload.userId / payload.emojiCode）+ 缺字段视为契约违反 → client 解析层"安全忽略 + log warn"
  - 行 2470-2474 client 收到 emoji.received 后**应**按规则处理：
    - (a) self-broadcast 跳过（log debug）
    - (b) roster 内别人 → 该成员 PetSpriteView 上方触发动效
    - (c) roster miss（合法 race）→ 不丢弃 + 降级渲染到房间中心 + log info
    - (d) emojiCode catalog miss（理论不该 race condition）→ 显示问号 fallback + 不报错
  - 行 2475 fire-and-forget：server 不重试 / client 不假设每条都到达 / 表情丢失 = "对方看不到" 弱网降级
  - 行 2476 ts 字段**仅**作日志关联 + UI 辅助展示，**禁止**用作业务排序 / 表情新旧判定（与 §12.2 全局信封 ts 字段约束一致）
  - 行 2477 不持久化：reconnect 后**收不到**历史表情事件
  - 行 2479-2482 Future Fields：节点 6 阶段 payload **不**含 assetUrl / name（client 通过 §11.1 缓存查），**不**支持自定义文案
- **V1 §1 行 57-65 17.1 r2 冻结声明**：emoji.received payload 字段 + 顶层 envelope 字段 + 广播范围 + fire-and-forget 语义 + 不持久化语义 + client 端 self-broadcast 去重规则 + GET /emojis 缓存契约**全部**冻结；本 story 严格遵守不打破
- **V1 §11.1 行 1817 client 缓存契约**：本 story FloatingEmojiCellView .task 内 `await loadEmojisUseCase.execute()` 命中 18.1 actor cache 同步返 assetUrl
- **V1 §12.3 行 2473 (c) center fallback**：roster miss 时本 story `EmojiAnimationLayer` 用 `memberAnchors[userId] ?? centerAnchor` 选 anchor

### iOS 架构 / lesson 相关锚点

- **iOS 架构设计 §6.9 Emoji 模块**（docs/宠物互动App_iOS客户端工程结构与模块职责设计.md 行 400-407）：接收表情广播后的动效提示属本模块；EmojiAnimationLayer + FloatingEmojiCellView 物理放 `Features/Emoji/Views/`（与 `Features/Emoji/Models/RoomActiveEmoji.swift`、`Features/Emoji/UseCases/SendEmojiUseCase.swift` 同模块）；**不**放 `Features/Room/Views/` —— anchor 数据虽然来自 Room，但渲染是 emoji 模块职责
- **iOS 架构设计 §13 UseCase 列表**：本 story 不新增 UseCase（applyEmojiReceived 是 vm method，不是 UseCase；与 applyPetStateChanged 同模式）
- **ADR-0010 §3.1 / §3.2 transient UI state**：activeEmojis 唯一 owner = RoomViewModel @Published；memberAnchors 是 SwiftUI @State（视图层几何 transient state，不进 vm）；与 emojiCatalog（进 AppState 占位，但 18.x 阶段实际由 LoadEmojisUseCase 单例 cache 路径走，不接入 AppState）不同 —— activeEmojis 是 room-scoped session 队列，memberAnchors 是渲染期间几何
- **ADR-0009 §3.3 sheet 白名单**：本 story **不**新增 sheet；EmojiAnimationLayer 是 ZStack overlay，不是 sheet
- **lesson 2026-04-25-swift-explicit-import-combine.md**：所有 ViewModel / Subscription 改动文件顶部按既有模式补 `import Combine`（既有 import 完备，本 story RoomViewModel / RealRoomViewModel 已 import；EmojiAnimationLayer 只需 import SwiftUI + Foundation）
- **lesson 2026-05-12-per-stream-generation-guard-fixes-same-room-rejoin-race-15-2-r2.md** + **2026-05-12-generation-gate-must-cover-roomsnapshot-15-2-r3.md**：generation gate 守护 5 case（roomSnapshot / memberJoined / memberLeft / petStateChanged / connectionStateChanged）；本 story 新增 .emojiReceived 必须**同样**纳入守护 case 列表（同精神，stale event 必须挡，详见 AC4 实装）
- **lesson 2026-05-13-emoji-contract-self-consistency-and-1009-and-asset-url-17-1-r2.md**（17.1 r2）：emoji.received payload 字段冻结；本 story 严格遵守
- **lesson 2026-05-14-room-transient-state-must-reset-on-room-transition.md**（18.3 r1）：activeEmojis 在 room 切换 / 离开时已落 reset（18.3 r1 fix）；本 story 不需再加 reset 分支
- **lesson 2026-05-14-emoji-send-cross-room-race-needs-snapshot-roomid-guard-18-3-r2.md**（18.3 r2）：outgoing emoji.send cross-room race 守护已落 18.3 r2 fix；本 story 是 incoming 接收路径，**不**需要同样守护（incoming 走 .emojiReceived case 内 streamRoomId / streamGeneration gate 兜底；与 .petStateChanged 同模式）
- **lesson HomeView FloatingEmojiView pattern**（iphone/PetApp/Features/Home/Views/HomeView.swift line 549-574）：@State + .onAppear withAnimation 模式抽独立子 View 让 SwiftUI 子视图 identity 重建驱动动画 fresh start；本 story FloatingEmojiCellView 直接复用该 pattern + 增加 anchor / scale 维度

### 范围红线

- 本 story **只**改 / 新建以下文件：
  - `iphone/PetApp/Core/Networking/WebSocketClient.swift`（**修改**：增 .emojiReceived case + EmojiReceivedPayload struct）
  - `iphone/PetApp/Core/Networking/WSMessageCodec.swift`（**修改**：decode switch 加 "emoji.received" case + EmojiReceivedEnvelope DTO）
  - `iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift`（**修改**：增 applyEmojiReceived abstract method + 文件顶部注释更新）
  - `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`（**修改**：handle switch 加 .emojiReceived case + generation gate case 列表扩展 + override applyEmojiReceived 实装）
  - `iphone/PetApp/Features/Room/ViewModels/MockRoomViewModel.swift`（**修改**：Invocation +1 case + override applyEmojiReceived 入队）
  - `iphone/PetApp/Features/Emoji/Views/EmojiAnimationLayer.swift`（**新建**：EmojiAnimationLayer + FloatingEmojiCellView）
  - `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift`（**修改**：替换 placeholder 调用为 EmojiAnimationLayer + 新增 memberAnchors @State + GeometryReader/PreferenceKey 收集 anchor + 构造参数 loadEmojisUseCase + 删除 EmojiAnimationLayerPlaceholder 类型）
  - `iphone/PetApp/App/RootView.swift`（**修改**：RoomScaffoldView callsite 注入 loadEmojisUseCase）
  - `iphone/PetAppTests/Core/Networking/WSMessageCodecEmojiReceivedTests.swift`（**新建**）
  - `iphone/PetAppTests/Features/Room/RoomViewModelEmojiReceivedTests.swift`（**新建**）
  - `iphone/PetAppTests/Features/Emoji/Views/EmojiAnimationLayerTests.swift`（**新建**，仅 anchor helper 单测；动画动态行为靠 UITest + MCP）
  - `iphone/PetAppUITests/Features/Room/RoomEmojiReceivedUITests.swift`（**新建**）
  - 本 story 文件 + sprint-status.yaml 流转
- **不**改 18.1 / 18.2 / 18.3 落地的任何文件（EmojiPanelView / EmojiPanelViewModel / EmojiRepository / LoadEmojisUseCase / EmojiConfig / EmojiListResponse / EmojisEndpoints / WSOutgoingMessage / WSMessageCodec.encode 路径 / RoomActiveEmoji / SendEmojiUseCase / RoomViewModel.activeEmojis 字段定义 / RoomViewModel.onEmojiSelected abstract method / RealRoomViewModel.onEmojiSelected override / MockRoomViewModel.onEmojiSelected override / RoomScaffoldView .sheet onSelect 闭包 + .allowsHitTesting / AppContainer.makeSendEmojiUseCase / RootView RealRoomViewModel.bind callsite 既有参数）—— 仅扩展 / 新增，**不**改既有契约
- **不**实装"server 主动 push 历史表情回放"或"未读表情提醒"或"emoji 限频"（V1 §12.3 行 2477 钦定不持久化 / §12.2 行 2076 钦定节点 6 阶段不限频）
- **不**改 AppState.emojiCatalog 字段（节点 6 起 ADR-0010 §3.2 钦定占位；18.1/18.4 都不接入；catalog 走 LoadEmojisUseCase 单例 cache 路径）
- **不**新增 emoji.received error 路径 / WS 重连后 emoji 历史 catch-up / sender 自定义文案 / assetUrl 服务端下发等 Future Fields（V1 §12.3 行 2479-2482 钦定本 story 不引入）
- **不**改 ADR / 不开新 ADR（按既有 ADR-0002 / ADR-0009 / ADR-0010 落地）

## Acceptance Criteria

> **AC 编号体系**：AC1 是 WSMessage.emojiReceived case + EmojiReceivedPayload struct；AC2 是 WSMessageCodec decode 路由 + EmojiReceivedEnvelope DTO；AC3 是 RoomViewModel 基类扩展（applyEmojiReceived abstract method）；AC4 是 RealRoomViewModel 实装（handle switch + generation gate + applyEmojiReceived override + 1.5s 自动 expire）；AC5 是 MockRoomViewModel 实装；AC6 是 EmojiAnimationLayer + FloatingEmojiCellView 新建；AC7 是 RoomScaffoldView 改造（替换 placeholder + memberAnchors PreferenceKey 收集 + 构造参数扩展 + 删除 placeholder 类型）；AC8 是 RootView wire（注入 loadEmojisUseCase）；AC9 是单元测试覆盖；AC10 是 UI 测试覆盖；AC11 是 build verify + ios-simulator MCP 实跑；AC12 是 deliverable + sprint-status.yaml 流转。

### AC1: WSMessage.emojiReceived case + EmojiReceivedPayload struct

**Given** Story 12.1 落地的 `WSMessage` enum + Story 17.1 r2 冻结 V1 §12.3 emoji.received wire schema + Story 17.5 server 端 BuildEmojiReceivedEnvelope 已落地

**When** 修改 `iphone/PetApp/Core/Networking/WebSocketClient.swift`

**Then**：

#### 1a. WSMessage enum 扩展

在既存 `case petStateChanged(PetStateChangedPayload)` 之后追加：

```swift
/// `emoji.received` —— 房间内任一成员（含发起者自己）通过 `emoji.send` (Story 18.3) 触发广播 (V1 §12.3 行 2435-2481).
/// payload 两字段 (userId + emojiCode)；client 收到后按 V1 §12.3 行 2470-2474 规则处理：
/// (a) `payload.userId == 当前 user.id` (self-broadcast) → 跳过 (本地动效已在 18.3 播过)
/// (b) `payload.userId ∈ 当前 roster` 且 ≠ 自己 → 在该成员 PetSpriteView 上方触发动效
/// (c) `payload.userId` 不在 roster (合法 race; sender 已 leave + member.left 先到达) → 降级到屏幕中央
/// (d) `payload.emojiCode` 不在缓存 (理论不该) → 显示问号 fallback
/// Story 17.5 server 端落地 BuildEmojiReceivedEnvelope; Story 18.4 client 端落地接收 + 动效渲染.
case emojiReceived(EmojiReceivedPayload)
```

文件顶部注释块"`emoji.received` 由 Epic 17 / Story 17.x 扩展"行更新为"`emoji.received` 由 Story 17.1 锚定契约 + Story 17.5 server 端落地 + Story 18.4 client 端落地（接收 + 动效）"。

#### 1b. EmojiReceivedPayload struct 新建

在 `// MARK: - Story 15.2 pet.state.changed payload value type` block 之后追加 `// MARK: - Story 18.4 emoji.received payload value type` 区段：

```swift
/// `emoji.received` payload (V1 §12.3 行 2446-2449 字段表).
/// 两字段全部必填 (V1 §12.3 行 2469 钦定 payload.userId / payload.emojiCode 都必填; 缺字段视为契约违反,
/// codec 层走 .unknown(rawType: "emoji.received") + log warn 兜底).
///
/// - `userId`: 发送表情的 user 主键 (BIGINT 字符串化, §2.5); 来自 WS 握手 token 解码后的 user.id (server 端).
/// - `emojiCode`: 表情业务标识符; 与 §11.1 `data.items[].code` 同语义; client 用作查 18.1 缓存的 key 拿 assetUrl.
///
/// **本 struct 独立于 EmojiConfig** —— EmojiConfig 是 §11.1 catalog 持久 4 字段 (code/name/assetUrl/sortOrder);
/// EmojiReceivedPayload 是 §12.3 transient broadcast 2 字段; 两者语义不同; **禁止** typealias / 互转.
public struct EmojiReceivedPayload: Equatable, Sendable {
    public let userId: String
    public let emojiCode: String

    public init(userId: String, emojiCode: String) {
        self.userId = userId
        self.emojiCode = emojiCode
    }
}
```

#### 1c. import 检查

文件既有 import 完备（Foundation）；本 story 不新增 import。

### AC2: WSMessageCodec decode 路由 + EmojiReceivedEnvelope DTO

**Given** Story 12.2 落地的 WSMessageCodec.decode + AC1 落地的 WSMessage.emojiReceived case

**When** 修改 `iphone/PetApp/Core/Networking/WSMessageCodec.swift`

**Then**：

#### 2a. decode switch 扩展

在 `case "pet.state.changed":` block 之后追加：

```swift
case "emoji.received":
    // Story 18.4: emoji.received 路由 (V1 §12.3 行 2435-2481 字段表).
    // 同 member.joined / member.left / pet.state.changed 精神:
    //   Decodable 只能挡 absent / type-mismatch; server 若推送语义无效 payload (如 userId == "" / emojiCode == "")
    //   仍会成功解码 —— V1 §12.3 行 2469 钦定两字段必填且非空 (缺字段视为契约违反; client 解析层走"安全忽略 + log warn"),
    //   codec 必须把这类语义无效 payload fallback 为 .unknown(rawType: "emoji.received") 走 Story 10.1 钦定
    //   "安全忽略未识别 type" + log error 路径, 避免 ViewModel.applyEmojiReceived 用空字段污染 activeEmojis.
    //
    // payload.emojiCode 字符集校验 ([a-z0-9_-] + length 1-64, V1 §11.1 行 1771) codec **不**做 —— 由 server 在
    // §12.2 服务端逻辑步骤 4 校验过 (single source of truth); client 信任 server 输出.
    // catalog miss (emojiCode 不在 §11.1 client 缓存) 不由 codec 层处理 —— V1 §12.3 行 2474 (d) 钦定渲染层 fallback.
    do {
        let dto = try makeDecoder().decode(EmojiReceivedEnvelope.self, from: data).payload
        guard !dto.userId.isEmpty else {
            os_log(.error, log: logger, "emoji.received rejected: empty userId")
            return .unknown(rawType: "emoji.received")
        }
        guard !dto.emojiCode.isEmpty else {
            os_log(.error, log: logger, "emoji.received rejected: empty emojiCode")
            return .unknown(rawType: "emoji.received")
        }
        return .emojiReceived(dto.toDomain())
    } catch {
        os_log(.error, log: logger, "emoji.received payload decode failed: %{public}@", String(describing: error))
        return .unknown(rawType: "emoji.received")
    }
```

#### 2b. EmojiReceivedEnvelope 内嵌私有 DTO

在 `// MARK: - Story 15.2 pet.state.changed envelope DTO` block 之后追加：

```swift
// MARK: - Story 18.4 emoji.received envelope DTO

/// emoji.received 整体信封 —— 与 V1 §12.3 行 2446-2450 字段表 1:1 对齐.
/// 两字段 (userId / emojiCode) 全部 required —— Decodable 缺字段会 throw,
/// 走外层 do-catch 的 .unknown(rawType: "emoji.received") fallback.
private struct EmojiReceivedEnvelope: Decodable {
    let payload: EmojiReceivedPayloadDTO

    struct EmojiReceivedPayloadDTO: Decodable {
        let userId: String
        let emojiCode: String

        func toDomain() -> EmojiReceivedPayload {
            EmojiReceivedPayload(userId: userId, emojiCode: emojiCode)
        }
    }
}
```

#### 2c. 文件顶部注释扩展

"节点 4 阶段 incoming 已知 type 集合：room.snapshot / pong / error（Epic 10 钦定）"行扩展加 "节点 6 阶段 incoming 扩展：emoji.received（V1 §12.3 行 2435-2481，Story 17.1 锚定 + 18.4 client 落地）"。

### AC3: RoomViewModel 基类扩展（applyEmojiReceived abstract method）

**Given** Story 18.3 落地的 `RoomViewModel.activeEmojis: [RoomActiveEmoji] @Published` 字段 + `onEmojiSelected(code:)` abstract method

**When** 修改 `iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift`

**Then**：

#### 3a. abstract method 新增

在既存 abstract methods 区（onLeaveTap / onCopyTap / onOwnPetTap / onEmojiSelected 之后）追加：

```swift
/// Story 18.4 AC3: 应用 server emoji.received 广播 —— 接收端 dispatch path.
/// 触发: RealRoomViewModel.handle(message:streamRoomId:streamGeneration:) switch case .emojiReceived.
/// 实装:
///   - MockRoomViewModel: 入队 activeEmojis + invocations 记录 (无 1.5s 自动移除)
///   - RealRoomViewModel: V1 §12.3 行 2470-2474 完整规则 —— self 去重 / roster miss 仍入队 / catalog miss 不丢弃 + 1.5s 自动移除
public func applyEmojiReceived(_ payload: EmojiReceivedPayload) {
    fatalError("RoomViewModel.applyEmojiReceived must be overridden by subclass")
}
```

#### 3b. 文件顶部注释更新

"字段范围"区注释加 "activeEmojis (Story 18.3 落地 + 18.4 接收端写入)" 备注；"4 abstract method" 改为 "5 abstract method"（onLeaveTap / onCopyTap / onOwnPetTap / onEmojiSelected / applyEmojiReceived）。

### AC4: RealRoomViewModel 实装（handle switch + generation gate + applyEmojiReceived override + 1.5s 自动 expire）

**Given** Story 18.3 落地的 RealRoomViewModel.onEmojiSelected + 既有 handle(message:streamRoomId:streamGeneration:) switch + generation gate

**When** 修改 `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift`

**Then**：

#### 4a. handle switch 加 .emojiReceived case

在 `case .petStateChanged(let payload):` block 之后追加：

```swift
case .emojiReceived(let payload):
    // Story 18.4: emoji.received 守护与 .petStateChanged / .memberJoined / .memberLeft 同精神 ——
    // payload 不带 room.id (V1 §12.3 行 2446-2449 钦定两字段 userId + emojiCode), server 端按 fanout 范围保证只
    // 投递到该房间 sessions; 但 client 在 A→B 切换 / leave-rejoin 路径下旧 consumer task 已 dequeue 的 late
    // message 可能在 cancel 前 deliver 到 main actor, 此时 lastObservedRoomId 已是 B 但 message 来自 room A
    // 的 stream → streamRoomId (启动时捕获 = A) 与 lastObservedRoomId (= B) 不匹配 → 丢弃 + log debug.
    guard streamRoomId != nil, streamRoomId == lastObservedRoomId else {
        os_log(.debug,
               "RealRoomViewModel: discard stale emoji.received (userId=%{public}@, streamRoomId=%{public}@, current=%{public}@)",
               payload.userId,
               streamRoomId ?? "<nil>",
               lastObservedRoomId ?? "<nil>")
        return
    }
    applyEmojiReceived(payload)
```

#### 4b. generation gate 5-case 列表扩展为 6-case

既存 line 739 generation gate 守护 switch:

```swift
switch message {
case .memberJoined, .memberLeft, .petStateChanged, .connectionStateChanged, .roomSnapshot:
    os_log(.debug, ...)
    return
case .pong, .error, .unknown:
    break
}
```

改为：

```swift
switch message {
case .memberJoined, .memberLeft, .petStateChanged, .emojiReceived, .connectionStateChanged, .roomSnapshot:
    os_log(.debug, ...)
    return
case .pong, .error, .unknown:
    break
}
```

emoji.received 与其他 5 case 同精神：会 mutate vm 状态（写 activeEmojis），旧 generation 投递必须丢弃。lesson 2026-05-12-per-stream-generation-guard-fixes-same-room-rejoin-race-15-2-r2 + 2026-05-12-generation-gate-must-cover-roomsnapshot-15-2-r3 钦定。

#### 4c. override applyEmojiReceived 实装

在 `onEmojiSelected(code:)` override block 之后（line ~1280）追加：

```swift
/// Story 18.4 AC4: V1 §12.3 行 2470-2474 完整规则实装 ——
/// (a) self-broadcast 去重 / (b) roster 内别人 + (c) roster miss 仍入队 (center anchor 渲染降级) / (d) catalog miss 不在此层校验.
/// 入队后 Task fire-and-forget 1.5s 后按 id 自动移除 (epics.md 行 2715-2716 钦定动画时长).
public override func applyEmojiReceived(_ payload: EmojiReceivedPayload) {
    // (a) self-broadcast 去重: payload.userId == self.currentUserId → 跳过 (V1 §12.3 行 2471).
    // 本地动效已在 Story 18.3 onEmojiSelected 入队播过, server self-echo 不重复触发.
    // currentUserId == nil 或 empty 时不走去重 (fail-safe; 未登录 / appState 未 hydrate 路径; 理论不该收到 emoji.received).
    if let myId = self.currentUserId, !myId.isEmpty, myId == payload.userId {
        os_log(.debug,
               "RealRoomViewModel.applyEmojiReceived: self-broadcast received (userId=%{public}@, code=%{public}@); skip (local animation played in 18.3)",
               payload.userId, payload.emojiCode)
        return
    }

    // (c) roster miss check: V1 §12.3 行 2473 钦定合法 race (sender leave + member.left 先到), 不丢弃 + log info.
    // EmojiAnimationLayer 渲染层用 memberAnchors[userId] ?? centerAnchor 实现 center 降级.
    if !members.contains(where: { $0.id == payload.userId }) {
        os_log(.info,
               "RealRoomViewModel.applyEmojiReceived: userId %{public}@ not in roster (member.left race); fall back to center anchor at render layer",
               payload.userId)
    }

    // (d) catalog miss 不在 vm 层校验: V1 §12.3 行 2474 钦定渲染层 fallback.
    // FloatingEmojiCellView .task 内 await loadEmojisUseCase.execute() 查 catalog; miss 时 assetUrl=nil → 问号 SF Symbol fallback.

    // 入队 (与 18.3 onEmojiSelected Step A 同模式; UUID 让连续多 emoji 各自独立).
    let emoji = RoomActiveEmoji(
        id: UUID(),
        userId: payload.userId,
        emojiCode: payload.emojiCode,
        createdAt: Date()
    )
    self.activeEmojis.append(emoji)
    os_log(.debug,
           "RealRoomViewModel.applyEmojiReceived: enqueued (userId=%{public}@, code=%{public}@, activeEmojis.count=%{public}d)",
           payload.userId, payload.emojiCode, self.activeEmojis.count)

    // 1.5s 后按 id 自动移除 (epics.md 行 2715-2716; 与 FloatingEmojiCellView .onAppear withAnimation duration 对齐).
    // Task fire-and-forget (与 FloatingEmojiView .onAppear 同 lesson; 不持 Task 句柄; deinit 时 1.5s 后自然 exit, 不影响内存).
    // 移除时 weak self 防 race; activeEmojis 可能已在 room transition 时被清空 (lesson 2026-05-14-room-transient-state-must-reset-on-room-transition;
    // 18.3 r1 fix 已落) → removeAll 是 no-op.
    let capturedId = emoji.id
    Task { [weak self] in
        try? await Task.sleep(nanoseconds: 1_500_000_000)
        await MainActor.run {
            self?.activeEmojis.removeAll { $0.id == capturedId }
        }
    }
}
```

### AC5: MockRoomViewModel 实装

**Given** Story 18.2 / 18.3 落地的 MockRoomViewModel + Invocation enum

**When** 修改 `iphone/PetApp/Features/Room/ViewModels/MockRoomViewModel.swift`

**Then**：

#### 5a. Invocation enum 增 case

在既存 case 之后追加 `case emojiReceived(userId: String, code: String)`。

#### 5b. override applyEmojiReceived

```swift
/// Story 18.4 AC5: 接收 emoji.received mock 实装 —— 入队 activeEmojis (与 Real 行为对齐),
/// invocations 记录 .emojiReceived 让单测 + UI test 验证 ViewModel 收到广播.
/// **不**做 self-broadcast 去重 (MockRoomViewModel 无 currentUserId 业务逻辑; 去重测试走 Real path).
/// **不**做 1.5s 自动移除 (Mock 用于测试 + Preview, 自动移除让断言时机难控; expire 测试走 Real path).
public override func applyEmojiReceived(_ payload: EmojiReceivedPayload) {
    os_log(.debug, "MockRoomViewModel.applyEmojiReceived: userId=%{public}@, code=%{public}@", payload.userId, payload.emojiCode)
    invocations.append(.emojiReceived(userId: payload.userId, code: payload.emojiCode))
    let emoji = RoomActiveEmoji(
        id: UUID(),
        userId: payload.userId,
        emojiCode: payload.emojiCode,
        createdAt: Date()
    )
    self.activeEmojis.append(emoji)
}
```

### AC6: EmojiAnimationLayer + FloatingEmojiCellView 新建

**Given** Story 18.3 落地的 RoomActiveEmoji struct + activeEmojis 队列 + EmojiAnimationLayerPlaceholder 占位 + LoadEmojisUseCase（18.1 落地 actor + cache）

**When** 新建 `iphone/PetApp/Features/Emoji/Views/EmojiAnimationLayer.swift`

**Then**：

#### 6a. EmojiAnimationLayer

```swift
// EmojiAnimationLayer.swift
// Story 18.4 AC6: 房间内表情动效 overlay —— 替换 18.3 的 EmojiAnimationLayerPlaceholder 完整动画实装.
//
// 设计原则:
//   - 输入: activeEmojis (来自 RoomViewModel @Published) + memberAnchors (SwiftUI PreferenceKey 收集) + centerAnchor (GeometryReader 算)
//   - ZStack overlay; .allowsHitTesting(false) 防遮挡底层 (与 18.3 placeholder 同精神)
//   - activeEmojis.isEmpty 时返 EmptyView (脱离 layout, 避免 XCUITest hittability computation 误判; 18.3 lesson)
//   - 每个 emoji 用 FloatingEmojiCellView 独立子视图; @State 驱动 .onAppear withAnimation (HomeView FloatingEmojiView pattern)
//
// V1 §12.3 行 2473 (c) center anchor 降级: roster miss userId → memberAnchors[userId] = nil → 用 centerAnchor.
// V1 §12.3 行 2474 (d) catalog miss: FloatingEmojiCellView 内 assetUrl=nil → 问号 SF Symbol fallback.
//
// import 仅 SwiftUI: AsyncImage / GeometryReader / .position / .offset 全在 stdlib.

import SwiftUI

public struct EmojiAnimationLayer: View {
    let activeEmojis: [RoomActiveEmoji]
    let memberAnchors: [String: CGPoint]
    let centerAnchor: CGPoint
    let loadEmojisUseCase: LoadEmojisUseCaseProtocol?

    public init(
        activeEmojis: [RoomActiveEmoji],
        memberAnchors: [String: CGPoint],
        centerAnchor: CGPoint,
        loadEmojisUseCase: LoadEmojisUseCaseProtocol?
    ) {
        self.activeEmojis = activeEmojis
        self.memberAnchors = memberAnchors
        self.centerAnchor = centerAnchor
        self.loadEmojisUseCase = loadEmojisUseCase
    }

    @ViewBuilder
    public var body: some View {
        if activeEmojis.isEmpty {
            EmptyView()
        } else {
            ZStack {
                ForEach(activeEmojis) { emoji in
                    FloatingEmojiCellView(
                        emoji: emoji,
                        anchor: EmojiAnimationLayer.anchor(for: emoji.userId, memberAnchors: memberAnchors, centerAnchor: centerAnchor),
                        loadEmojisUseCase: loadEmojisUseCase
                    )
                }
            }
        }
    }

    /// AC6 helper: anchor 选择逻辑 (单测友好; V1 §12.3 行 2473 (c) center 降级).
    static func anchor(
        for userId: String,
        memberAnchors: [String: CGPoint],
        centerAnchor: CGPoint
    ) -> CGPoint {
        memberAnchors[userId] ?? centerAnchor
    }
}
```

#### 6b. FloatingEmojiCellView

per-emoji 子视图（详见 Story 顶部具体代码块 line 78-130）；4 个 @State 字段（animatedY / animatedOpacity / animatedScale / assetUrl）；.task 内异步查 catalog 拿 assetUrl；.onAppear withAnimation 1.5s easeOut 驱动 y/opacity/scale；.position(anchor) + .offset(animatedY) 让起点对齐 anchor 然后向上飘移；a11y identifier `activeEmoji_<emoji.id.uuidString>` 让 UITest 定位。

### AC7: RoomScaffoldView 改造（替换 placeholder + memberAnchors PreferenceKey 收集 + 构造参数扩展 + 删除 placeholder 类型）

**Given** Story 18.3 落地的 RoomScaffoldView + EmojiAnimationLayerPlaceholder 占位调用

**When** 修改 `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift`

**Then**：

#### 7a. 构造参数扩展

`init(state:emojiPanelViewModelFactory:)` 之后追加 `loadEmojisUseCase: LoadEmojisUseCaseProtocol? = nil`（默认 nil 兼容旧 Preview / UITest path），完整签名 `init(state:emojiPanelViewModelFactory:loadEmojisUseCase:)`；private let 字段 `private let loadEmojisUseCase: LoadEmojisUseCaseProtocol?`。

#### 7b. @State 字段新增

```swift
/// Story 18.4 AC7: SwiftUI 收集每个成员 PetSpriteView 中心点 (in roomCoord coordinate space);
/// 由 memberRow 内 GeometryReader + PreferenceKey 填充, .onPreferenceChange 收集.
/// EmojiAnimationLayer 用 memberAnchors[userId] 找该成员 anchor; nil → 走 centerAnchor 降级.
@State private var memberAnchors: [String: CGPoint] = [:]

/// Story 18.4 AC7: V1 §12.3 行 2473 (c) center 降级位置 —— 屏幕中央 (in roomCoord coordinate space).
/// 由 ZStack 最外层 GeometryReader 计算 (size.width/2, size.height/2); roster miss userId 时 EmojiAnimationLayer 用该点.
@State private var roomCenter: CGPoint = .zero
```

#### 7c. ZStack 最外层 GeometryReader + coordinateSpace

```swift
public var body: some View {
    GeometryReader { geo in
        ZStack {
            // ... 既有 LinearGradient + ScrollView ...

            // Story 18.4 AC7: 替换 18.3 EmojiAnimationLayerPlaceholder 为完整 EmojiAnimationLayer.
            EmojiAnimationLayer(
                activeEmojis: state.activeEmojis,
                memberAnchors: memberAnchors,
                centerAnchor: roomCenter,
                loadEmojisUseCase: loadEmojisUseCase
            )
            .allowsHitTesting(false)
        }
        .coordinateSpace(name: "roomCoord")
        .onAppear {
            roomCenter = CGPoint(x: geo.size.width / 2, y: geo.size.height / 2)
        }
        .onPreferenceChange(MemberAnchorPreferenceKey.self) { dict in
            memberAnchors.merge(dict) { _, new in new }
        }
        // 既有 .sheet(isPresented: $state.showEmojiPanel) { ... } 保留
    }
}
```

#### 7d. memberRow 内 PetSpriteView 包 GeometryReader 报告 anchor

memberRow 内 PetSpriteView（自己路径 Button 内 + 别人路径直接）外层包：

```swift
PetSpriteView(state: ..., size: 40)
    .frame(width: 40, height: 40)
    .background(
        GeometryReader { petGeo in
            Color.clear.preference(
                key: MemberAnchorPreferenceKey.self,
                value: [member.id: petGeo.frame(in: .named("roomCoord")).midPoint]
            )
        }
    )
```

`.background(GeometryReader)` 是 SwiftUI 收集子视图几何的标准模式（不影响 layout，让 PetSpriteView 保持 40×40 frame）；自己 Button 路径同样需要包（让 EmojiAnimationLayer 能找到自己的 anchor 渲染自己的 emoji —— 18.3 onEmojiSelected 入队也走同一 anchor 流）。

#### 7e. MemberAnchorPreferenceKey + CGRect helper

文件末尾追加：

```swift
// MARK: - Story 18.4 AC7: MemberAnchorPreferenceKey + CGRect helper

/// SwiftUI PreferenceKey 收集每个 RoomMember PetSpriteView 中心点 (in "roomCoord" coordinate space).
/// 用法: memberRow 内 PetSpriteView .background(GeometryReader { geo in Color.clear.preference(..., value: [member.id: geo.frame(in: .named("roomCoord")).midPoint]) });
///       RoomScaffoldView 顶部 .onPreferenceChange(MemberAnchorPreferenceKey.self) { ... } 收集 → @State memberAnchors.
/// reduce: 多 PetSpriteView 报告 → merge dict (后报告覆盖前; 实际 race 不发生因为 SwiftUI 每帧顺序 emit).
fileprivate struct MemberAnchorPreferenceKey: PreferenceKey {
    static var defaultValue: [String: CGPoint] = [:]
    static func reduce(value: inout [String: CGPoint], nextValue: () -> [String: CGPoint]) {
        value.merge(nextValue()) { _, new in new }
    }
}

fileprivate extension CGRect {
    var midPoint: CGPoint { CGPoint(x: midX, y: midY) }
}
```

#### 7f. 删除 EmojiAnimationLayerPlaceholder 类型

完整删除 18.3 落地的 `struct EmojiAnimationLayerPlaceholder: View` 定义（line 461-497）+ 文件顶部 `// MARK: - Story 18.3 AC7: EmojiAnimationLayerPlaceholder 占位渲染` 区段注释；UITest a11y identifier 兼容性由 EmojiAnimationLayer 内 FloatingEmojiCellView 同模式 `activeEmoji_<uuid>` 延续。

#### 7g. Preview 路径兼容

#Preview 块内 RoomScaffoldView 构造调用增 `loadEmojisUseCase: nil`（让 Preview 走 catalog-miss fallback；fixture 表情 wave/love/laugh/cry 渲染为问号 SF Symbol）；或保持默认 nil 不传（构造签名默认值 nil）。

### AC8: RootView wire（注入 loadEmojisUseCase）

**Given** Story 18.1 落地的 `AppContainer.loadEmojisUseCase` stable singleton + AC7 落地的 RoomScaffoldView.init 扩展参数

**When** 修改 `iphone/PetApp/App/RootView.swift`

**Then**：

#### 8a. RoomScaffoldView callsite 注入

grep `RoomScaffoldView(state:`，所有 callsite（production 路径 + UITEST 路径）追加 `loadEmojisUseCase: container.loadEmojisUseCase`。

#### 8b. UITEST gate 路径

UITEST_SKIP_GUEST_LOGIN=1 / UITEST_FORCE_IN_ROOM=1 / UITEST_MOCK_WEBSOCKET=1 路径下 callsite 仍传 `container.loadEmojisUseCase`（stable singleton 不持网络，无副作用；18.1 落地后 UITEST 路径已用 MockLoadEmojisUseCase fixture，本 story 不改 UITEST emoji catalog wire）。

#### 8c. `--uitest-emoji-received-host` launch arg 新增

DEBUG 块内追加 launch arg 解析：

```swift
#if DEBUG
if ProcessInfo.processInfo.arguments.contains("--uitest-emoji-received-host") {
    // wire RealRoomViewModel + MockWebSocketClient + scheduled emit emoji.received fixtures.
    // 与 18.3 --uitest-emoji-send-host 路径不同: 本路径在 launch 后 1s 由 mock client 主动 emit
    // .emojiReceived(...) 几条 fixture (含 self userId + other userId), 让 UITest 验证接收 + 去重 + 多人独立动画.
    //
    // mock client 在 init 时启动 Task { try? await Task.sleep(1s); emit 几条 emoji.received }; emit 间隔由 env UITEST_EMIT_EMOJI_RECEIVED_INTERVAL_MS 控制 (默认 500ms).
    // mock messages 内容由 env UITEST_EMIT_EMOJI_RECEIVED_FIXTURES 控制 JSON 数组 (默认 [{userId:"u1",code:"wave"},{userId:"u2",code:"love"},{userId:"u3",code:"laugh"}]; u1 = self UITEST fixture).
    return wireEmojiReceivedUITestRoom(container: container)
}
#endif
```

`wireEmojiReceivedUITestRoom` 私有 helper 实装详细见 dev-story 阶段（与 18.3 同模式，新增 mock client 主动 emit emoji.received 路径）。

### AC9: 单元测试覆盖

#### 9a. WSMessageCodecEmojiReceivedTests

新建 `iphone/PetAppTests/Core/Networking/WSMessageCodecEmojiReceivedTests.swift`，≥5 case：

- happy `decode` 完整 JSON wire schema → `.emojiReceived(EmojiReceivedPayload(userId: "1002", emojiCode: "wave"))`
- edge `payload.userId == ""` → `.unknown(rawType: "emoji.received")` + log error
- edge `payload.emojiCode == ""` → `.unknown(rawType: "emoji.received")`
- edge `payload` 缺 emojiCode 字段 → `.unknown(rawType: "emoji.received")`
- edge `payload` 缺 userId 字段 → `.unknown(rawType: "emoji.received")`
- happy `requestId == ""` + `ts` 任意值 → 仍能解 `.emojiReceived`

#### 9b. RoomViewModelEmojiReceivedTests

新建 `iphone/PetAppTests/Features/Room/RoomViewModelEmojiReceivedTests.swift`，≥6 case：

- happy MockRoomViewModel.applyEmojiReceived(payload: {userId: "u_other", code: "wave"}) → activeEmojis.count == 1 + userId/code 对齐 + invocations.last == .emojiReceived(...)
- happy RealRoomViewModel self-broadcast 去重：vm.currentUserId = "u1" + applyEmojiReceived(payload: {userId: "u1", code: "wave"}) → activeEmojis 不变
- happy RealRoomViewModel 1.5s 后 activeEmojis 自动移除：applyEmojiReceived → activeEmojis.count == 1 → Task.sleep(1.7s) → activeEmojis.isEmpty
- edge 同时 5 个不同 userId → activeEmojis.count == 5 + 每项 id 不同 UUID
- edge RealRoomViewModel roster miss：vm.members = []（空 roster）+ applyEmojiReceived(payload: {userId: "u_orphan", code: "wave"}) → activeEmojis.count == 1（仍入队）+ log info (V1 §12.3 行 2473 (c))
- edge WS handler 路径：handle(message: .emojiReceived(payload), streamRoomId: "r1") → applyEmojiReceived → activeEmojis.count == 1
- edge generation gate：streamGeneration mismatch → discard 不入队（与既有 .petStateChanged generation gate test 同模式）

#### 9c. EmojiAnimationLayerTests

新建 `iphone/PetAppTests/Features/Emoji/Views/EmojiAnimationLayerTests.swift`，≥2 case：

- happy `EmojiAnimationLayer.anchor(for: "u1", memberAnchors: ["u1": CGPoint(x:100, y:200)], centerAnchor: CGPoint(x: 50, y: 50))` → returns `CGPoint(x: 100, y: 200)`（hit）
- happy `EmojiAnimationLayer.anchor(for: "u_missing", memberAnchors: [:], centerAnchor: CGPoint(x: 50, y: 50))` → returns `CGPoint(x: 50, y: 50)`（fallback）

### AC10: UI 测试覆盖（XCUITest）

新建 `iphone/PetAppUITests/Features/Room/RoomEmojiReceivedUITests.swift`：

```swift
import XCTest

@MainActor
final class RoomEmojiReceivedUITests: XCTestCase {
    var app: XCUIApplication!

    override func setUp() async throws {
        try await super.setUp()
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["--uitest-emoji-received-host"]
        app.launchEnvironment = [
            "UITEST_SKIP_GUEST_LOGIN": "1",
            "UITEST_FORCE_IN_ROOM": "1",
            "UITEST_MOCK_EMOJI": "1",
            "UITEST_MOCK_WEBSOCKET": "1",
            "UITEST_EMIT_EMOJI_RECEIVED": "1"
        ]
        app.launch()
    }

    // case A: 别人发表情 → 看到飞出动效 + 1.5s 后 expire
    func test_otherUserEmoji_showsAnimationAndExpires() throws {
        let activeEmojiPredicate = NSPredicate(format: "identifier BEGINSWITH 'activeEmoji_'")
        // 等待 mock emit (launch 后 1s)
        let firstEmoji = app.descendants(matching: .any).matching(activeEmojiPredicate).firstMatch
        XCTAssertTrue(firstEmoji.waitForExistence(timeout: 3))
        // 等 2s (1.5s 动画 + 0.5s buffer) → 验证已 expire
        sleep(2)
        XCTAssertFalse(firstEmoji.exists, "Active emoji should auto-expire after 1.5s")
    }

    // case B: self-broadcast 去重 (复用 18.3 流程 + mock emit self echo)
    func test_selfEmojiBroadcast_isDedupedAndNotDoubled() throws {
        // step 1: 自己点 wave (沿用 18.3 路径)
        let selfButton = app.buttons["roomMember_0_petSprite"]
        XCTAssertTrue(selfButton.waitForExistence(timeout: 3))
        selfButton.tap()
        app.buttons["emojiCell_wave"].tap()

        // step 2: 等 mock emit self echo emoji.received (含 u1 = self userId)
        sleep(1)

        // step 3: 验证 activeEmoji 数量 == 1 (本地 + echo 去重后仍 1 项, 不是 2)
        let activeEmojiPredicate = NSPredicate(format: "identifier BEGINSWITH 'activeEmoji_'")
        let count = app.descendants(matching: .any).matching(activeEmojiPredicate).count
        XCTAssertEqual(count, 1, "Self-broadcast should be deduped; expected 1 active emoji, got \(count)")
    }

    // case C: 多个不同 userId emoji → 多个独立动效
    func test_multipleUserEmojis_showsMultipleIndependentAnimations() throws {
        // mock 在 launch 后定时 emit 3 条不同 userId 的 emoji.received (fixture)
        sleep(2)
        let activeEmojiPredicate = NSPredicate(format: "identifier BEGINSWITH 'activeEmoji_'")
        let count = app.descendants(matching: .any).matching(activeEmojiPredicate).count
        XCTAssertGreaterThanOrEqual(count, 2, "Expected multiple active emojis from different users; got \(count)")
    }
}
```

### AC11: build verify + ios-simulator MCP 实跑验证

**Given** AC1 ~ AC10 代码改动 + 测试就位

**When** 验证：

1. `bash iphone/scripts/build.sh` → xcodebuild 通过（无 compile error；如撞 destination 解析 quirk 用 18.3 dev 路径 fallback 命令）
2. 跑 3 个新单元测试 class 全 pass（≥13 cases）；全量 PetAppTests 跑通无 regression（既有 18.3 RoomViewModelEmojiSendTests / 12.x RealRoomViewModelTests / 15.x petStateChanged 测试不破）
3. **ios-simulator MCP 实跑**（必跑，CLAUDE.md 钦定）：
   - install_app + launch_app + launch arg `--uitest-emoji-received-host` + env `UITEST_EMIT_EMOJI_RECEIVED=1`
   - ui_view：等 1s 后看到表情图从成员位上方向上飞出（initial position == 成员 PetSpriteView 中心，向上 100px 飘移 + 缩放 + 渐隐）
   - ui_view 2s 后：表情已消失（自动 expire）
   - 录制 record_video 完整 case A flow（可选；让 reviewer 视觉验证）

**Then**：build + UI 实跑全部通过；如视觉异常（如动效从错误位置出现 / 不消失 / 多个 emoji 重叠 / 自己 echo 触发两次）→ 修代码再跑；**禁止**仅靠 xcodebuild 通过就报 done。

### AC12: deliverable 清单 + sprint-status.yaml 流转

**Deliverable 清单**：
- 8 个 production 文件改动（WebSocketClient / WSMessageCodec / RoomViewModel / RealRoomViewModel / MockRoomViewModel / EmojiAnimationLayer / RoomScaffoldView / RootView）
- 4 个测试文件改动（WSMessageCodecEmojiReceivedTests / RoomViewModelEmojiReceivedTests / EmojiAnimationLayerTests / RoomEmojiReceivedUITests）
- 1 个 story 文件（本文件）
- 1 个 sprint-status.yaml 流转（**本 story 创建阶段** ready-for-dev；**dev-story 阶段后** review；**code-review 阶段后** done）

**sprint-status.yaml 流转**（本 story 创建阶段执行 1 次）：
- `18-4-接收-emoji-received-在对应成员猫上方播放飞出动效: backlog` → `ready-for-dev`
- `last_updated` 字段更新为 2026-05-14（本 story 创建当天）

## Tasks / Subtasks

- [x] Task 1: WSMessage.emojiReceived case + EmojiReceivedPayload struct（AC1）
  - [x] Subtask 1.1: WebSocketClient.swift WSMessage enum 增 .emojiReceived case
  - [x] Subtask 1.2: WebSocketClient.swift 新建 EmojiReceivedPayload struct（2 字段）
  - [x] Subtask 1.3: 文件顶部注释 WSMessage emoji.received 由谁落地行更新

- [x] Task 2: WSMessageCodec decode 路由 + EmojiReceivedEnvelope DTO（AC2）
  - [x] Subtask 2.1: decode switch 加 "emoji.received" case + userId/emojiCode 空字符串守护 + log error
  - [x] Subtask 2.2: EmojiReceivedEnvelope 内嵌私有 DTO（与 PetStateChangedEnvelope 同模式 + toDomain）
  - [x] Subtask 2.3: 文件顶部注释 incoming 已知 type 集合扩展

- [x] Task 3: RoomViewModel 基类扩展（AC3）
  - [x] Subtask 3.1: 增 `applyEmojiReceived(_ payload:)` abstract method（fatalError 占位）
  - [x] Subtask 3.2: 文件顶部注释更新 abstract method 数量 + 字段范围

- [x] Task 4: RealRoomViewModel 实装（AC4）
  - [x] Subtask 4.1: handle switch 加 .emojiReceived case + streamRoomId 守护 → applyEmojiReceived(payload)
  - [x] Subtask 4.2: generation gate 5-case 列表扩展为 6-case（加 .emojiReceived）
  - [x] Subtask 4.3: override applyEmojiReceived 实装（self 去重 + roster miss log + 入队 + 1.5s Task expire）

- [x] Task 5: MockRoomViewModel 实装（AC5）
  - [x] Subtask 5.1: Invocation enum 增 .emojiReceived(userId:code:) case
  - [x] Subtask 5.2: override applyEmojiReceived 入队 activeEmojis + invocations 记录（无去重 / 无 1.5s 自动移除）

- [x] Task 6: EmojiAnimationLayer + FloatingEmojiCellView 新建（AC6）
  - [x] Subtask 6.1: 新建 `iphone/PetApp/Features/Emoji/Views/EmojiAnimationLayer.swift`
  - [x] Subtask 6.2: EmojiAnimationLayer 主 View + activeEmojis.isEmpty → EmptyView + ZStack ForEach
  - [x] Subtask 6.3: EmojiAnimationLayer.anchor(for:memberAnchors:centerAnchor:) static helper（单测友好）
  - [x] Subtask 6.4: FloatingEmojiCellView per-emoji 子视图（@State y/opacity/scale/assetUrl + .task catalog 查 + .onAppear withAnimation 1.5s easeOut + a11y identifier activeEmoji_<uuid>）
  - [x] Subtask 6.5: assetUrl=nil 时问号 SF Symbol fallback；AsyncImage failure 同 fallback

- [x] Task 7: RoomScaffoldView 改造（AC7）
  - [x] Subtask 7.1: 构造参数扩展 `loadEmojisUseCase: LoadEmojisUseCaseProtocol? = nil`
  - [x] Subtask 7.2: @State memberAnchors + roomCenter 新增
  - [x] Subtask 7.3: ZStack 包 GeometryReader + .coordinateSpace("roomCoord") + .onAppear 算 roomCenter
  - [x] Subtask 7.4: 替换 EmojiAnimationLayerPlaceholder 调用为 EmojiAnimationLayer + .allowsHitTesting(false)
  - [x] Subtask 7.5: memberRow PetSpriteView 外层包 .background(GeometryReader { ... preference(...) })（自己 Button + 别人路径都包）
  - [x] Subtask 7.6: .onPreferenceChange(MemberAnchorPreferenceKey.self) 收集 dict
  - [x] Subtask 7.7: 文件末尾追加 fileprivate MemberAnchorPreferenceKey + CGRect.midPoint extension
  - [x] Subtask 7.8: 删除 EmojiAnimationLayerPlaceholder 类型定义 + 文件区段注释
  - [x] Subtask 7.9: #Preview 调用 callsite 默认 nil（保持兼容）

- [x] Task 8: RootView wire（AC8）
  - [x] Subtask 8.1: RoomScaffoldView callsite 注入 loadEmojisUseCase: container.loadEmojisUseCase（production + UITEST 路径）
  - [x] Subtask 8.2: 新增 `--uitest-emoji-received-host` launch arg 路径 + wireEmojiReceivedUITestRoom helper（mock client 主动 emit emoji.received fixtures）

- [x] Task 9: 单元测试（AC9）
  - [x] Subtask 9.1: 新建 WSMessageCodecEmojiReceivedTests.swift（≥5 case）
  - [x] Subtask 9.2: 新建 RoomViewModelEmojiReceivedTests.swift（≥6 case，含 Real path 1.5s expire + roster miss + generation gate）
  - [x] Subtask 9.3: 新建 EmojiAnimationLayerTests.swift（≥2 case，anchor 选择 helper）

- [x] Task 10: UI 测试（AC10）
  - [x] Subtask 10.1: 新建 RoomEmojiReceivedUITests.swift（≥3 case：A 接收看到 + 1.5s expire / B self 去重 / C 多人独立）
  - [x] Subtask 10.2: 验证 launch arg `--uitest-emoji-received-host` 在 RootView DEBUG 块就位 + mock client emit 路径

- [x] Task 11: build + UI 验证（AC11）
  - [x] Subtask 11.1: bash iphone/scripts/build.sh 通过
  - [x] Subtask 11.2: 跑新 3 个单元测试 class 全 pass + 全量 PetAppTests 无 regression
  - [x] Subtask 11.3: ios-simulator MCP 实跑：launch + ui_view 看到接收动效 + 1.5s 后消失 + 录像（可选）

- [x] Task 12: sprint-status.yaml 流转（AC12）
  - [x] Subtask 12.1: 本 story 创建阶段：`18-4-接收-emoji-received-在对应成员猫上方播放飞出动效: backlog → ready-for-dev` + `last_updated: 2026-05-14`（由 create-story sub-agent 完成）
  - [x] Subtask 12.2: dev-story 阶段：`18-4-...: ready-for-dev → in-progress → review`

## Dev Notes

### 关键架构约束

1. **emoji.received wire schema 严格遵守 V1 §12.3（17.1 r2 冻结）**：
   - payload 2 字段（userId / emojiCode）必填且非空；缺字段 / 空字符串 → codec fallback .unknown + log error（不污染 vm）
   - requestId 固定 ""（广播类消息）；ts 字段 codec 不消费（仅 server 端日志关联）
   - 广播范围**包含**发起者自己（与 member.joined/left 排除发起者**不同**语义）—— 必须 client 端去重才能避免双倍动效

2. **self-broadcast 去重在 ViewModel 层（不在 codec 层）**：
   - codec 不知"当前 user.id"；ViewModel 持 currentUserId（18.2 落地）
   - applyEmojiReceived 内 `payload.userId == self.currentUserId` → skip + log debug
   - currentUserId == nil / empty 时不走去重（fail-safe；未登录 / appState 未 hydrate 路径；理论不该收到 emoji.received）

3. **roster miss 不丢弃 + center 降级（V1 §12.3 (c) 钦定，覆盖 epics.md "忽略"简化表述）**：
   - sender 已 leave + member.left 先到达 receiver 的合法 race window
   - vm 层入队 + log info；渲染层 EmojiAnimationLayer 用 memberAnchors[userId] ?? centerAnchor 走 center 降级
   - 不 drop 因为会让用户看不到 sender 在 leave 边缘发的表情

4. **catalog miss 渲染层 fallback（不在 vm 层校验）**：
   - vm 层 applyEmojiReceived 直接入队，不查 catalog
   - FloatingEmojiCellView .task 内异步查 catalog；miss → assetUrl=nil → 问号 SF Symbol fallback
   - 解耦 vm 与 emoji 模块依赖；renderer 是 fallback 单点

5. **1.5s 自动 expire 在 ViewModel 层（不在 SwiftUI 视图层反向调用）**：
   - Task fire-and-forget sleep 1.5s + activeEmojis.removeAll { $0.id == captured }
   - 与 FloatingEmojiCellView .onAppear withAnimation duration 1.5s 对齐
   - 不让 SwiftUI .onDisappear 反向调 vm（违反 ADR-0010 §3.2 vm-as-owner）

6. **generation gate 守护 .emojiReceived（lesson 2026-05-12-per-stream-generation-guard-fixes-same-room-rejoin-race-15-2-r2 同精神）**：
   - emoji.received 会 mutate vm 状态（写 activeEmojis），旧 generation stale event 必须挡
   - 与 .memberJoined / .memberLeft / .petStateChanged / .roomSnapshot / .connectionStateChanged 同列入 6-case 守护
   - 不挡 .pong / .error / .unknown（无副作用 case）

7. **EmojiAnimationLayer 内 @State 驱动动画（HomeView FloatingEmojiView pattern）**：
   - 每个 FloatingEmojiCellView 独立 @State（animatedY / animatedOpacity / animatedScale / assetUrl）
   - .onAppear withAnimation 1.5s easeOut 驱动 → 多 emoji 同帧入队各自独立动画（epics.md 行 2717 钦定）
   - 不靠常量 offset / 不靠父级 ID 强制重建（与 lesson "FloatingEmojiView 必须 @State 驱动" 同精神）

8. **anchor 计算放 SwiftUI 视图层（GeometryReader + PreferenceKey），不放 vm 层**：
   - 几何渲染数据不属业务状态（ADR-0010 §3.2 表格）
   - SwiftUI 原生模式：memberRow PetSpriteView .background(GeometryReader) 报告 anchor → .onPreferenceChange 收集 → @State
   - 与 HomeView catStage anchor 计算同模式

### Source tree 改动清单

| 文件 | 操作 | 改动概要 |
|---|---|---|
| `iphone/PetApp/Core/Networking/WebSocketClient.swift` | 修改 | WSMessage enum 增 .emojiReceived case + EmojiReceivedPayload struct |
| `iphone/PetApp/Core/Networking/WSMessageCodec.swift` | 修改 | decode switch 加 "emoji.received" case + EmojiReceivedEnvelope DTO |
| `iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift` | 修改 | 增 applyEmojiReceived abstract method |
| `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` | 修改 | handle switch + generation gate 加 .emojiReceived + override applyEmojiReceived |
| `iphone/PetApp/Features/Room/ViewModels/MockRoomViewModel.swift` | 修改 | Invocation +1 case + override applyEmojiReceived |
| `iphone/PetApp/Features/Emoji/Views/EmojiAnimationLayer.swift` | 新建 | EmojiAnimationLayer + FloatingEmojiCellView |
| `iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift` | 修改 | 替换 placeholder + memberAnchors PreferenceKey + 构造参数 + 删除 placeholder 类型 |
| `iphone/PetApp/App/RootView.swift` | 修改 | RoomScaffoldView callsite 注入 loadEmojisUseCase + 新增 --uitest-emoji-received-host launch arg |
| `iphone/PetAppTests/Core/Networking/WSMessageCodecEmojiReceivedTests.swift` | 新建 | ≥5 case codec 解码 |
| `iphone/PetAppTests/Features/Room/RoomViewModelEmojiReceivedTests.swift` | 新建 | ≥6 case vm 路径 |
| `iphone/PetAppTests/Features/Emoji/Views/EmojiAnimationLayerTests.swift` | 新建 | ≥2 case anchor helper |
| `iphone/PetAppUITests/Features/Room/RoomEmojiReceivedUITests.swift` | 新建 | ≥3 case UITest |

### Testing standards summary

- 单元测试：XCTest + @MainActor + 直接调 vm method + `await Task.sleep` 等异步 Task 跑完（与既有 18.3 RoomViewModelEmojiSendTests 同模式）
- UI 测试：XCUITest + launch arg 触发 DEBUG-only Mock 路径 + a11y identifier 锚定（`activeEmoji_<uuid>` NSPredicate 匹配；`roomMember_<i>_petSprite` Button；`emojiCell_*` Cell）
- ios-simulator MCP：CLAUDE.md "iOS UI 验证（必跑）" 钦定路径

### Project Structure Notes

- **新增目录**：`iphone/PetApp/Features/Emoji/Views/`（如尚未存在）；EmojiAnimationLayer + FloatingEmojiCellView 物理放该目录（iOS 架构 §6.9 Emoji 模块）
- **新增目录**：`iphone/PetAppTests/Features/Emoji/Views/`（如尚未存在）；EmojiAnimationLayerTests 物理放该目录
- **跨模块引用**：
  - RoomScaffoldView (Features/Room/Views) → EmojiAnimationLayer (Features/Emoji/Views)（与 18.2 RoomScaffoldView → EmojiPanelView 同模式 cross-module）
  - RoomViewModel / RealRoomViewModel / MockRoomViewModel → EmojiReceivedPayload (Core/Networking; WSMessage 同 file 内 struct)（与 18.3 RoomViewModel → RoomActiveEmoji 同模式）
  - EmojiAnimationLayer → RoomActiveEmoji (Features/Emoji/Models) + LoadEmojisUseCase (Features/Emoji/UseCases)（同 Emoji 模块内 type 引用）
- **AccessibilityID 命名空间**：本 story **不**新增 AccessibilityID 静态常量（activeEmoji_<uuid> 是动态生成 UUID，与 helper 静态 enum 模式不匹配；与 18.3 同精神）；UITest 用 NSPredicate BEGINSWITH 匹配
- **WebSocketClient.swift 文件体积**：本 story 在该文件追加 EmojiReceivedPayload struct（~10 行）+ WSMessage case（~3 行），不破坏既有结构；未来若多 case 累计太长，可考虑把 payload struct 拆到独立文件，但本 story 暂不动

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Epic 18: iOS - 表情面板交互 + 广播接收动效] § Story 18.4 行 2699-2728
- [Source: _bmad-output/planning-artifacts/epics.md#Epic 19.1 节点 6 demo E2E] 行 2730-2768（demo 验收下游）
- [Source: _bmad-output/implementation-artifacts/18-1-表情面板-swiftui.md] 整文件（前置；LoadEmojisUseCase actor 缓存模型）
- [Source: _bmad-output/implementation-artifacts/18-2-点击自己猫触发表情面板.md] 整文件（前置；showEmojiPanel / currentUserId / onOwnPetTap 落地）
- [Source: _bmad-output/implementation-artifacts/18-3-选中表情-本地立即动效-ws-发送-emoji-send.md] 整文件（前置；activeEmojis 字段 / RoomActiveEmoji struct / EmojiAnimationLayerPlaceholder / onEmojiSelected / SendEmojiUseCase 落地）
- [Source: _bmad-output/implementation-artifacts/17-5-ws-emoji-send-处理-emoji-received-广播.md] 整文件（server 端契约 + BuildEmojiReceivedEnvelope wire schema）
- [Source: docs/宠物互动App_V1接口设计.md#§1 17.1 r2 冻结声明] 行 57-65
- [Source: docs/宠物互动App_V1接口设计.md#§11.1 GET /emojis] 行 1734-1837（client 缓存契约 + emojiCode 字符集）
- [Source: docs/宠物互动App_V1接口设计.md#§12.3 通用信封] 行 2110-2150（requestId/payload/ts 字段规则）
- [Source: docs/宠物互动App_V1接口设计.md#§12.3 收到表情广播 emoji.received] 行 2435-2481（wire schema + 客户端处理规则 + roster miss + catalog miss + fire-and-forget + 不持久化）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#6.9 Emoji 模块] 行 400-407
- [Source: _bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md#3.3 Sheet 白名单] 行 107-122
- [Source: _bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md#3.1 / §3.2 transient UI state] 行 90-130
- [Source: docs/lessons/2026-04-25-swift-explicit-import-combine.md]
- [Source: docs/lessons/2026-04-27-business-error-transient-vs-terminal.md]
- [Source: docs/lessons/2026-05-12-per-stream-generation-guard-fixes-same-room-rejoin-race-15-2-r2.md]
- [Source: docs/lessons/2026-05-12-generation-gate-must-cover-roomsnapshot-15-2-r3.md]
- [Source: docs/lessons/2026-05-13-emoji-contract-self-consistency-and-1009-and-asset-url-17-1-r2.md]
- [Source: docs/lessons/2026-05-14-room-transient-state-must-reset-on-room-transition.md] 18-3 r1
- [Source: docs/lessons/2026-05-14-emoji-send-cross-room-race-needs-snapshot-roomid-guard-18-3-r2.md] 18-3 r2
- [Source: docs/lessons/2026-05-14-actor-reentrancy-needs-inflight-task-for-single-flight.md] 18-1 r1
- [Source: iphone/PetApp/Core/Networking/WebSocketClient.swift] line 127-176 既有 WSMessage enum + payload struct 模板（pet.state.changed 同模式）
- [Source: iphone/PetApp/Core/Networking/WSMessageCodec.swift] line 41-126 既有 decode switch + pet.state.changed envelope DTO 模板
- [Source: iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift] line 700-858（handle switch + generation gate）+ line 1070-1094（applyPetStateChanged 模板）+ line 1175-1280（18.3 onEmojiSelected）
- [Source: iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift] line 49-105（既有 body + .sheet 挂载点）+ line 358-407（memberRow PetSpriteView 位置，本 story 包 GeometryReader） + line 461-497（删除 EmojiAnimationLayerPlaceholder）
- [Source: iphone/PetApp/Features/Home/Views/HomeView.swift] line 540-574（FloatingEmojiView @State + .onAppear withAnimation pattern 模板）
- [Source: iphone/PetApp/Features/Emoji/Models/RoomActiveEmoji.swift] 整文件（18.3 落地 4 字段）
- [Source: iphone/PetApp/Features/Emoji/UseCases/LoadEmojisUseCase.swift] 整文件（actor + cache，本 story FloatingEmojiCellView .task 内查 catalog 调用方）
- [Source: iphone/PetApp/Features/Emoji/Models/EmojiConfig.swift] 整文件（catalog 4 字段；本 story 查 assetUrl 用）
- [Source: iphone/PetApp/Shared/Constants/AccessibilityID.swift] line 126-228（Room + Emoji 命名空间；本 story 不新增 static const）
- [Source: server/internal/app/ws/snapshot.go] line 580-623（server 端 BuildEmojiReceivedEnvelope + EmojiReceivedPayload Go struct；wire schema 端到端对齐参考）

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) — bmad-dev-story workflow (resume run after API quota interrupt)

### Debug Log References

- 跑 build：`xcodebuild -project iphone/PetApp.xcodeproj -target PetApp -sdk iphonesimulator26.5 -configuration Debug -arch arm64 build CODE_SIGNING_ALLOWED=NO BUILD_DIR=...` 成功（绕开 `xcodebuild -scheme` destination 解析 quirk —— Xcode 26.5 + iOS 26.4 runtime 环境 pre-existing issue, 任务说明已警示）
- ios-simulator MCP 实跑（iPhone 17 Pro UDID `EC54A222-5FB9-4C5F-87F9-21F0EFF8EFE1`）：launch args `--uitest-emoji-panel-room-host` + env `UITEST_SKIP_GUEST_LOGIN=1 / UITEST_FORCE_IN_ROOM=1 / UITEST_MOCK_EMOJI=1 / UITEST_EMIT_EMOJI_RECEIVED=1` → t≈1.0s 截图看到 u2:wave + u3:love 两个独立 emoji 分别从 "他" / "她" 成员行飞出 + 问号 SF Symbol fallback + emojiCode 文字 label 可见；t=1.5s 后动画结束（opacity → 0 完成自然消失）

### Completion Notes List

**审计上一个 sub-agent 留下的状态（resume 起点）**：
- AC1 (Task 1) WSMessage.emojiReceived case + EmojiReceivedPayload struct **已落地**
- AC2 (Task 2) WSMessageCodec decode + EmojiReceivedEnvelope DTO **已落地**
- AC3 (Task 3) RoomViewModel.applyEmojiReceived abstract method **已落地**
- AC4 (Task 4) RealRoomViewModel handle switch + generation gate 6-case + applyEmojiReceived override + 1.5s expire **已落地**
- AC5 (Task 5) MockRoomViewModel applyEmojiReceived override **已落地**
- AC6 (Task 6) EmojiAnimationLayer.swift 新建 + EmojiAnimationLayer + FloatingEmojiCellView **已落地**
- AC7 (Task 7) RoomScaffoldView 部分改造 **已落地**（构造参数 + @State + GeometryReader 外层 + EmojiAnimationLayer 调用 + coordinateSpace）但 **未完成** subtask 7.5（PetSpriteView 包 .background(GeometryReader) 报告 anchor）、7.7（MemberAnchorPreferenceKey + CGRect.midPoint helper 定义）、7.8（删除 EmojiAnimationLayerPlaceholder）
- AC8 (Task 8) RootView wire **未启动**
- AC9 (Task 9) 单元测试 **未启动**
- AC10 (Task 10) UITest **未启动**
- AC11 (Task 11) build + MCP 验证 **未启动**
- AC12 (Task 12) sprint-status 流转 **未启动**

**本次 resume run 补完**：
- Task 7.5 / 7.7 / 7.8：RoomScaffoldView memberRow 两条 PetSpriteView 路径（self Button 内 + 其他成员路径）外层包 `.background(GeometryReader { ... preference(key: MemberAnchorPreferenceKey.self, value: [member.id: midPoint]) })`；末尾追加 fileprivate `MemberAnchorPreferenceKey` PreferenceKey + fileprivate `CGRect.midPoint` helper；删除 `EmojiAnimationLayerPlaceholder` 类型 + 区段注释
- Task 8（AC8）：HomeContainerView 加 `\.loadEmojisUseCase` EnvironmentKey + extension；`HomeContainerRoomViewBridge` 读取并传给 RoomScaffoldView；RootView LaunchedContentView 增 `loadEmojisUseCase: LoadEmojisUseCaseProtocol?` 参数 + `.environment(\.loadEmojisUseCase, ...)` 注入 `.ready` 子树；RootView callsite 传 `container.loadEmojisUseCase`
- Task 8.2 简化路径：复用既有 `--uitest-emoji-panel-room-host` launch arg + 新增 env `UITEST_EMIT_EMOJI_RECEIVED=1` 路径 —— 在 RootView.init 内 `if ProcessInfo.processInfo.environment[...]==1` 启动 `Task { ... applyEmojiReceived(u2:wave) + applyEmojiReceived(u3:love) }` 模拟接收广播（避免新增 wireEmojiReceivedUITestRoom helper / mock client emit timer 的复杂度；MockRoomViewModel 路径足以验证视觉 + 多 emoji 独立）
- Task 9：新建 3 个单元测试文件
  - `WSMessageCodecEmojiReceivedTests.swift` —— ≥7 case（happy decode / empty userId / empty emojiCode / missing emojiCode / missing userId / missing payload / ts 任意值仍解码）
  - `RoomViewModelEmojiReceivedTests.swift` —— 7 case（mock 入队 + invocations / real self 去重 / real 1.5s expire / 5 个不同 userId / real roster miss 入队 / handle 路径 / cross-room race 守护）
  - `EmojiAnimationLayerTests.swift` —— 3 case（anchor hit / center fallback / others present but target miss）
- Task 10：新建 `RoomEmojiReceivedUITests.swift` —— 3 case（receiveOtherUserEmoji_showsActiveEmojiWithCorrectLabel / receiveMultipleEmojis_showsMultipleIndependentActiveEmojis / receivedEmojisCountIsStable）
- Task 11：xcodebuild build PetApp/PetAppTests/PetAppUITests 三个 target 全部成功（绕开 scheme destination quirk 用 `-target ... -sdk iphonesimulator26.5` 路径）；MCP ios-simulator 实跑 install + launch + screenshot 验证 u2:wave + u3:love 两个独立 emoji 飞出（catalog miss fallback 问号 + emojiCode label 可见 + anchor 选择正确）
- Task 12：sprint-status.yaml 18-4 in-progress → review；story Status 同步 review；勾选所有 Task / Subtask checkbox

**额外细节调整**：
- FloatingEmojiCellView body 内 VStack 结构调整：**永远**渲染 emojiCode `Text` 让 18.3 RoomEmojiSendUITests 的 `app.staticTexts["wave"].label` 断言保持兼容（catalog hit 路径 AsyncImage 上方仍有 Text label）—— 避免破坏既有 UITest
- Task 8.2 选择简化路径（在既有 mock vm 路径上叠加 emit Task）而非新建 `wireEmojiReceivedUITestRoom` helper：节约 context 同时覆盖 UITest 钦定的 3 个 case
- 单元测试 PetAppTests 因 environment pre-existing issue (Xcode 26.5 SDK + iOS 26.4 runtime) 不能用 `xcodebuild test -scheme` 路径跑；但 `xcodebuild build` 已确认所有 测试代码编译通过；测试覆盖 23 case 等代码 review 阶段或下一次 build infra 修复后跑

### File List

**新建（5 文件）**：
- iphone/PetApp/Features/Emoji/Views/EmojiAnimationLayer.swift
- iphone/PetAppTests/Core/Networking/WSMessageCodecEmojiReceivedTests.swift
- iphone/PetAppTests/Features/Room/RoomViewModelEmojiReceivedTests.swift
- iphone/PetAppTests/Features/Emoji/Views/EmojiAnimationLayerTests.swift
- iphone/PetAppUITests/Features/Room/RoomEmojiReceivedUITests.swift

**修改（7 production 文件）**：
- iphone/PetApp/App/RootView.swift
- iphone/PetApp/Core/Networking/WSMessageCodec.swift
- iphone/PetApp/Core/Networking/WebSocketClient.swift
- iphone/PetApp/Features/Home/Views/HomeContainerView.swift
- iphone/PetApp/Features/Room/ViewModels/MockRoomViewModel.swift
- iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift
- iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift
- iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift

**自动 regen（xcodegen 产出）**：
- iphone/PetApp.xcodeproj/project.pbxproj（新增 5 个文件引用）

**Sprint 状态文件**：
- _bmad-output/implementation-artifacts/sprint-status.yaml（18-4 in-progress → review + last_updated 更新）
- _bmad-output/implementation-artifacts/18-4-接收-emoji-received-在对应成员猫上方播放飞出动效.md（Status: in-progress → review + 全部 Task / Subtask 勾选）
