// RootView.swift
// Story 2.2 起占位 RootView：渲染 HomeView.
// Story 2.3 起：注入 AppCoordinator，把 HomeView 三个 CTA 闭包连到 coordinator.present(...)，
// 并通过 .fullScreenCover(item:) 弹出对应 Sheet placeholder.
// Story 2.5 起（追加）：
//   - 新增 @StateObject private var container = AppContainer()
//   - 新增 .task：bind PingUseCase 后触发 start() 拉 /ping + /version
// Story 2.9 起（追加）：
//   - 新增 @StateObject private var launchStateMachine = AppLaunchStateMachine()
//   - body 改为 ZStack { switch launchStateMachine.state } 三分支路由 + 每分支 `.transition(.opacity)`：
//     · .launching → LaunchingView
//     · .ready → HomeView 子树（保留 .onAppear / .fullScreenCover / errorPresenter 既有 wire）
//     · .needsAuth(presentation:) → 根据 presentation 三态分发（Story 5.5 round 2 [P1] fix）.
//
// Story 37.3 改造（ADR-0009 §3.5 步骤 1 + 8）：
//   - 删除 RootView.wireHomeViewModelClosures() 方法（HomeView 不再有 onRoomTap / onInventoryTap /
//     onComposeTap closure 字段；主入口 IA 改 4 Tab + HomeContainerView 互斥状态机）.
//   - `.ready` 分支不再渲染 homeView，改渲染 `MainTabView()` + `.environmentObject(coordinator)`
//     + `.environmentObject(homeViewModel)` + `.environment(\.resetIdentityViewModel, ...)`
//     + `.environment(\.sessionStore, container.sessionStore)`（HomeContainerView 通过 environment
//     读取 HomeView 三参数）.
//   - **保留** `.fullScreenCover(item: $coordinator.presentedSheet)` modifier 仅服务 `.compose`
//     次级路由（ADR-0009 §3.4 钦定 SheetType 白名单仅留 .compose；.room/.inventory 已 supersede
//     为 4 Tab + HomeContainer 互斥状态机）. codex round 1 [P2] fix: 不挂会让 present(.compose)
//     变 silent no-op.
//   - 保留：launching / needsAuth 三态机不变；onReadyTask 内 ping bind / start + 外层 .task 内
//     loadHome bind + errorPresentationHost 全部既有 wire.

import SwiftUI

struct RootView: View {
    @StateObject private var coordinator = AppCoordinator()
    @StateObject private var container = AppContainer()
    /// Story 37.7 codex round 1 [P1] fix：`RealHomeViewModel` 而非裸 `HomeViewModel`.
    ///
    /// 原 (Story 2.5+) 注入裸 `HomeViewModel()` 在 Story 37.7 引入 5 个 abstract method (`onCreateTap`
    /// / `onJoinTap` / `onFeedTap` / `onPetTap` / `onPlayTap`，基类 `fatalError("subclass override")`)
    /// 之后变成生产 crash 路径——用户在 .ready 子树点 actionRow 三按钮 / teamIdleCard "创建队伍" /
    /// "加入队伍" 任一按钮就会 crash. RealHomeViewModel override 5 个方法走占位行为（写 showJoinModal
    /// / interactionAnimation），让 UI 链路活起来.
    ///
    /// 注：用 parameterless `RealHomeViewModel()` 而非 `RealHomeViewModel(appState:)` —— SwiftUI
    /// `@StateObject` 属性初始化器内不能交叉引用同级 `@StateObject appState`（self 未求值）；
    /// AppState 通过下方 `.task` 内 `homeViewModel.bind(appState: appState)` 延迟注入（与 pingUseCase /
    /// loadHomeUseCase 既有 bind 模式一致）.
    /// 详见 docs/lessons/2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md.
    @StateObject private var homeViewModel: HomeViewModel = RealHomeViewModel()

    /// Story 37.8 AC5：RoomScaffoldView 注入入口；与 homeViewModel 同模式 @StateObject 持有 + .environmentObject 注入子树.
    /// 静态类型 `RoomViewModel`（基类）让 SwiftUI `@StateObject` 老模式可用 —— AppState 也是同级 @StateObject,
    /// 不能在属性初始化器内交叉引用（编译期不允许 self 提前求值）.AppState 通过 `.onAppear` 同步注入
    /// （Story 37.8 round 2 [P2] fix；详见下方 `.onAppear` 内注释 + lesson
    /// docs/lessons/2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md）.
    /// 实例类型 `RealRoomViewModel`（生产实装；onLeaveTap / onCopyTap override 完整）—— round 1 P2 fix
    /// 已把基类换 Real，让 inRoom path（UITEST_FORCE_IN_ROOM / 未来 Story 12.1 join flow）走 onLeaveTap
    /// 调 appState.setCurrentRoomId(nil) 切回 idle 不再 silent no-op.
    ///
    /// Story 12.3 AC5（UITest 策略 A）：DEBUG build + `UITEST_ROOM_THREE_MEMBERS=1` env flag 路径下
    /// 切到 `MockRoomViewModel(members: 3 fixed)` —— 让 UITest 直接锚定 `roomMember_0/1/2` + `roomMember_3`
    /// （dashed empty slot）的 a11y identifier，而不依赖真实 WS server.
    /// Production build 此 env 被忽略；UITest 跑完 reset env 即恢复正常.
    @StateObject private var roomViewModel: RoomViewModel

    /// Story 37.9 AC5：WardrobeScaffoldView 注入入口；与 homeViewModel / roomViewModel 同模式
    /// @StateObject 持有 + .environmentObject 注入子树.
    /// 静态类型 `WardrobeViewModel`（基类）让 SwiftUI `@StateObject` 老模式可用；AppState 通过
    /// `.onAppear` 同步注入（与 RealRoomViewModel.bind 同精神，避免 launch-time race）.
    /// 实例类型 `RealWardrobeViewModel`（生产实装）—— Story 37.7 round 1 [P1] lesson 预防性应用,
    /// 不能用裸基类 WardrobeViewModel()：基类 onEquipTap 是 fatalError，用户点装备按钮即 crash.
    @StateObject private var wardrobeViewModel: WardrobeViewModel = RealWardrobeViewModel()

    /// Story 37.10 AC5：FriendsScaffoldView 注入入口；与 homeViewModel / roomViewModel /
    /// wardrobeViewModel 同模式 @StateObject 持有 + .environmentObject 注入子树.
    /// 静态类型 `FriendsViewModel`（基类）让 SwiftUI `@StateObject` 老模式可用；AppState 通过
    /// `.onAppear` 同步注入（与 RealRoomViewModel.bind / RealWardrobeViewModel.bind 同精神，
    /// 避免 launch-time race；Story 37.8 round 2 [P2] lesson 钦定路径）.
    /// 实例类型 `RealFriendsViewModel`（生产实装）—— Story 37.7 round 1 [P1] lesson 预防性应用,
    /// 不能用裸基类 FriendsViewModel()：基类 onInviteFriendTap / onJoinFriendTap 是 fatalError，
    /// 用户点邀请/加入按钮即 crash.
    @StateObject private var friendsViewModel: FriendsViewModel = RealFriendsViewModel()

    /// Story 37.11 AC5：ProfileScaffoldView 注入入口；与 homeViewModel / roomViewModel /
    /// wardrobeViewModel / friendsViewModel 同模式 @StateObject 持有 + .environmentObject 注入子树.
    /// 静态类型 `ProfileViewModel`（基类）让 SwiftUI `@StateObject` 老模式可用；AppState 通过
    /// `.onAppear` 同步注入（与 RealRoomViewModel.bind / RealWardrobeViewModel.bind /
    /// RealFriendsViewModel.bind 同精神，避免 launch-time race；Story 37.8 round 2 [P2] lesson 钦定路径）.
    /// 实例类型 `RealProfileViewModel`（生产实装）—— Story 37.7 round 1 [P1] lesson 预防性应用,
    /// 不能用裸基类 ProfileViewModel()：基类 5 个 abstract method（onWeChatCardTap /
    /// onWeChatBindConfirmTap / onWeChatModalDismissTap / onMenuTap / onCollectionViewAllTap）
    /// 是 fatalError，用户点任一按钮即 crash.
    @StateObject private var profileViewModel: ProfileViewModel = RealProfileViewModel()

    /// Story 37.4 AC3：全局 AppState 单 source of truth；通过 `.environmentObject(appState)` 注入子树.
    /// 与 coordinator / homeViewModel 同级 @StateObject 持有；HomeViewModel.bind(appState:) 在 .task 内调
    /// 形成 weak 反向引用，让 applyHomeData(_:) 写入 AppState（不再写自身 homeData 字段）.
    @StateObject private var appState = AppState()

    /// Story 37.5 AC8: Theme 注入 source-of-truth.
    /// 当前固定 .candy（Story 37.14 白名单：本期不做 UI 切换面板）.
    /// 用 @State 而非 let 是为了未来主题切换 UI（mini-epic）落地时能直接改 @Binding 不破坏
    /// RootView 类型契约.Theme 是 value type → 用 @State (而非 @StateObject) 即可.
    @State private var currentTheme: ThemeName = .candy

    /// Story 2.9 新增 / Story 5.2 升级：启动状态机.
    @State private var launchStateMachine: AppLaunchStateMachine?

    /// Story 8.5 AC9: StepSyncTriggerService（持 4 触发器 + in-flight gate + 5min 定时器）.
    /// 由 RootView @State 持有，避免 body 重建时 service 重启
    /// （rebuild → factory 返回新 service → 旧 timer 仍在跑 → 资源泄漏）.
    /// `nil` 守卫让 `ensureStepSyncWired()` 仅初始化一次（与 ensureLaunchStateMachineWired() 同模式）.
    @State private var stepSyncTriggerService: StepSyncTriggerService?

    /// Story 15.4 AC4: PetStateSyncTriggerService（订阅 HomeViewModel.$petState → 5s 节流 + roomId
    /// preflight → fire-and-forget 触发 SyncPetStateUseCase）.
    /// 由 RootView @State 持有，避免 body 重建时 service 重启 subscription（rebuild → factory 返回新 service →
    /// 旧 subscription 与新 subscription 同时活 → 重复 spawn）.与 stepSyncTriggerService 同模式;
    /// 详见 lesson `2026-04-26-stateobject-debug-instance-aliasing.md`.
    @State private var petStateSyncTriggerService: PetStateSyncTriggerService?

    /// Story 8.4 review round 2 P2 fix: scenePhase listener，让 background → foreground reactivate
    /// 时再 bind 一次 motionProvider —— 覆盖 first-launch 用户去 Settings 改权限再回 app 的真实路径.
    ///
    /// 单纯的 `.onAppear` 仅在 RootView 首次出现时触发；用户在同 session 中授权（去 Settings App
    /// 切完 Motion 权限再回来）时 RootView 不会重新 .onAppear → bind 不会被再调 → first-launch
    /// 未授权 path 持有的引用永远停在 .notDetermined gate 上 → petState 卡 .rest 直到 app relaunch.
    /// scenePhase `.active` 切换覆盖此场景：从 Settings 回来 app 走 background → active 路径，
    /// .onChange 触发 → 调一次 idempotent bind → 此时 authorizationStatus 已变 .authorized → startUpdates.
    ///
    /// 与 Story 5.5 round 8 RetryView / Story 37.x 各 ViewModel bind 模式同精神：bind 入口幂等 + auth-gate，
    /// 多次调用安全（hasStartedMotionUpdates 短路防重复订阅；未授权时仅持引用不订阅，不会触发权限弹窗）.
    /// 详见 docs/lessons/2026-05-04-scenephase-rebind-for-auth-gated-subscriptions.md.
    @Environment(\.scenePhase) private var scenePhase

    #if DEBUG
    /// Story 2.8: dev "重置身份" 按钮 ViewModel.仅 Debug build 存在；Release build 字段不存在.
    @State private var resetIdentityViewModel: ResetIdentityViewModel?
    #endif

    /// Story 12.3 AC5（UITest 策略 A）：根据 `UITEST_ROOM_THREE_MEMBERS` env flag 决定
    /// `roomViewModel` @StateObject 初始值 —— flag = "1" 时切到 `MockRoomViewModel` 持 3 个 fixed members,
    /// 让 UITest 验证 `RoomScaffoldView` 真实渲染 3 个成员行（roomMember_0/1/2）+ 1 个空位（roomMember_3）.
    /// Production build / 无 env flag 路径走 `RealRoomViewModel()` 既有逻辑（Story 37.8 round 1 P2 fix
    /// 钦定路径不动）.
    /// `_roomViewModel = StateObject(wrappedValue:)` 是 SwiftUI 推荐的 @StateObject custom init 写法.
    init() {
        #if DEBUG
        // Story 18.2 AC7: `--uitest-emoji-panel-room-host` launch arg 路径 —— MockRoomViewModel
        // (currentUserId: "u1", members: u1 自己 + u2 他人 + u3 他人) 让 RoomScaffoldView UI 测试
        // 锚定 self vs other PetSpriteView Button 行为. 仅 DEBUG; 与 18.1 stub host view 同精神
        // (但区别: 18.1 是独立全屏 host view 路径, 18.2 走正常 MainTabView/HomeContainer 但用 Mock vm).
        // 同时与 UITEST_SKIP_GUEST_LOGIN=1 + UITEST_FORCE_IN_ROOM=1 配合 (这两个 env 让 launch bootstrap
        // 立即成功并切到 inRoom 态).
        if ProcessInfo.processInfo.arguments.contains("--uitest-emoji-panel-room-host") {
            let mock = MockRoomViewModel(
                roomCodeForCopy: RoomScaffoldDefaults.roomCodeForCopy,
                hostCatName: RoomScaffoldDefaults.hostCatName,
                members: [
                    RoomMember(id: "u1", name: "我", level: 8, status: "在玩耍", isHost: true),
                    RoomMember(id: "u2", name: "他", level: 7, status: "在散步", isHost: false),
                    RoomMember(id: "u3", name: "她", level: 9, status: "在玩耍", isHost: false),
                ],
                userIsHost: true,
                currentUserId: "u1"   // 自己 = u1 (members[0])
            )
            // Story 18.4 AC8 / AC10: UITEST_EMIT_EMOJI_RECEIVED=1 路径 —— launch 后 1s 自动调
            // mock.applyEmojiReceived 模拟"别人发表情"广播; UITest 验证接收端动效渲染.
            // Fixture: u2 + u3 各发一个 wave + love, 触发 EmojiAnimationLayer 多 emoji 独立渲染.
            // self-broadcast 去重路径在 RealRoomViewModel 单测覆盖; UITest 用 Mock path 验证视觉.
            if ProcessInfo.processInfo.environment["UITEST_EMIT_EMOJI_RECEIVED"] == "1" {
                // weak capture mock 防 leak; Task 不持 mock strong ref.
                // 用 0.6s + 0.3s 让 RoomScaffoldView 已挂载第一帧 + emit 都在 1.5s 动画窗口内.
                Task { @MainActor [weak mock] in
                    try? await Task.sleep(nanoseconds: 600_000_000)  // 0.6s 让 RoomScaffoldView 第一帧渲染完
                    mock?.applyEmojiReceived(EmojiReceivedPayload(userId: "u2", emojiCode: "wave"))
                    try? await Task.sleep(nanoseconds: 300_000_000)
                    mock?.applyEmojiReceived(EmojiReceivedPayload(userId: "u3", emojiCode: "love"))
                }
            }
            self._roomViewModel = StateObject(wrappedValue: mock)
            return
        }
        if ProcessInfo.processInfo.environment["UITEST_ROOM_THREE_MEMBERS"] == "1" {
            // 3 个 fixed members（房主 + 2 普通）—— 与 UITest case 对齐：roomMember_0/1/2 可见 + 1 空位 roomMember_3.
            let mock = MockRoomViewModel(
                roomCodeForCopy: RoomScaffoldDefaults.roomCodeForCopy,
                hostCatName: RoomScaffoldDefaults.hostCatName,
                members: [
                    RoomMember(id: "u_alice", name: "Alice", level: 8, status: "在玩耍", isHost: true),
                    RoomMember(id: "u_bob", name: "Bob", level: 7, status: "在散步", isHost: false),
                    RoomMember(id: "u_charlie", name: "Charlie", level: 9, status: "在玩耍", isHost: false),
                ],
                userIsHost: RoomScaffoldDefaults.userIsHost
            )
            // Story 15.1 AC4: 给三成员注入三种不同 pet state → UITest 可定位
            // petSprite_rest / petSprite_walk / petSprite_run 三个 a11y identifier.
            mock.memberPetStates = [
                "u_alice": .rest,
                "u_bob": .walk,
                "u_charlie": .run,
            ]
            self._roomViewModel = StateObject(wrappedValue: mock)
            return
        }
        #endif
        self._roomViewModel = StateObject(wrappedValue: RealRoomViewModel())
    }

    var body: some View {
        #if DEBUG
        // Story 18.1 AC8: UITest stub host view 路径 —— `--uitest-emoji-panel-host` launch arg
        // 触发时, 全屏渲染 EmojiPanelHostView 而非正常 ZStack/MainTabView. 仅 DEBUG 编译.
        // 不依赖 launchStateMachine (UITest 路径下 bootstrap 已被 UITEST_SKIP_GUEST_LOGIN 替代).
        if ProcessInfo.processInfo.arguments.contains("--uitest-emoji-panel-host") {
            return AnyView(EmojiPanelHostView(container: container))
        }
        #endif
        return AnyView(mainBody)
    }

    @ViewBuilder
    private var mainBody: some View {
        ZStack {
            if let stateMachine = launchStateMachine {
                LaunchedContentView(
                    stateMachine: stateMachine,
                    coordinator: coordinator,
                    homeViewModel: homeViewModel,
                    roomViewModel: roomViewModel,
                    wardrobeViewModel: wardrobeViewModel,
                    friendsViewModel: friendsViewModel,
                    profileViewModel: profileViewModel,
                    appState: appState,
                    currentTheme: currentTheme,
                    sessionStore: container.sessionStore,
                    resetIdentityViewModel: currentResetIdentityViewModel(),
                    // Story 18.2 AC5: factory 闭包 wire 入口 —— HomeContainerRoomViewBridge 通过
                    // `\.emojiPanelViewModelFactory` environment value 拿到, 传给 RoomScaffoldView init.
                    // 每次 sheet 弹出 RoomScaffoldView 调一次 factory → new EmojiPanelViewModel (useCase
                    // 是 container.loadEmojisUseCase 单例 → cache 跨 sheet 共享, 18.1 缓存契约一致).
                    emojiPanelViewModelFactory: { [container] in container.makeEmojiPanelViewModel() },
                    // Story 18.4 AC8: 同时把 container.loadEmojisUseCase stable singleton 透传给 LaunchedContentView →
                    // .environment(\.loadEmojisUseCase, ...) → RoomScaffoldView → EmojiAnimationLayer → FloatingEmojiCellView
                    // .task 查 catalog 拿 assetUrl. UITEST 路径下 loadEmojisUseCase 单例不变 (mock fixture 已注入).
                    loadEmojisUseCase: container.loadEmojisUseCase,
                    onReadyAppear: {
                        #if DEBUG
                        // lazy 注入：第一次 .onAppear 时从已稳定的 container 拿 keychainStore，
                        // 保证 reset 按钮调的 removeAll() 清的是 container.keychainStore 这同一份.
                        // nil 守卫让 RootView 重建（如旋转 / 离开返回）时不会重新构造覆盖既有 instance.
                        // Story 37.4：注入 appState 让 reset 按钮成功后清 AppState（reset 流程 ADR-0010 §3.7）.
                        if resetIdentityViewModel == nil {
                            resetIdentityViewModel = container.makeResetIdentityViewModel(appState: appState)
                        }
                        #endif
                    },
                    onReadyTask: {
                        // Story 5.5 round 4 / round 6 钦定 wire：bind ping + start 在同一 .ready
                        // 分支 .task 内串行（避免 SwiftUI 多 .task 顺序 race；详见 lesson
                        // 2026-04-27-swiftui-multi-task-no-ordering.md）.
                        homeViewModel.bind(pingUseCase: container.makePingUseCase())
                        await homeViewModel.start()

                        // Story 8.5 AC9: 启动步数同步触发器（首次同步 + 5min 定时循环）.
                        // 必须在 launchStateMachine bootstrapStep1 完成后（AppState.applyHomeData 已写入
                        // currentUser 等）；ready 子树只在 bootstrap 完成后渲染 → onReadyTask 执行时
                        // SessionStore.token 必已存在；service.start() 调 /steps/sync 不会 401.
                        stepSyncTriggerService?.start()

                        // Story 15.4 AC4: 启动 pet state-sync 触发器（subscribe `homeViewModel.$petState` +
                        // dropFirst 抹掉订阅瞬间 replay）；与 stepSync 同时机；start() 幂等 (subscription != nil 短路).
                        petStateSyncTriggerService?.start()
                    },
                    // Story 8.5 review round 2 [P2] fix: launch state 离开 .ready 时停 step sync timer.
                    //
                    // 触发链路：token expiry 401 → AuthBoundaryAPIClient 调
                    // unauthorizedHandlerSink → AppLaunchStateMachine.triggerColdStart() →
                    // state = .launching → bootstrap 重跑（可能再失败 → .needsAuth）.
                    //
                    // 旧实装漏洞：5min timer 在 .ready 启动后无人 stop —— 离开 .ready 期间仍然
                    // 每 5min 跑一次 sync（用已被 sessionStore.clear() 清掉的 token → 又触发 401 →
                    // 又触发 cold-start，形成自激循环）；scenePhase listener 在下次 .active 仍会
                    // 调 start() 重启 timer.
                    //
                    // 修复：在 LaunchedContentView 内 .onChange(of: stateMachine.state) 监听
                    // state 切换；离开 .ready 时调本回调 → service.stop() cancel timer.
                    // 重新进入 .ready 时由 onReadyTask 重新调 service.start()（既有路径，幂等）.
                    // 详见 docs/lessons/2026-05-04-launch-state-leave-ready-must-stop-feature-services.md.
                    onLeaveReady: {
                        stepSyncTriggerService?.stop()
                        // Story 15.4 AC4: 离开 .ready 时同停 pet state-sync subscription
                        // （与 stepSync 同精神 —— token expiry / cold-start 后避免无效请求 + 释放订阅引用）.
                        // 详见 lesson `2026-05-04-launch-state-leave-ready-must-stop-feature-services.md`.
                        petStateSyncTriggerService?.stop()
                    }
                )
            } else {
                LaunchingView().transition(.opacity)
            }

            #if DEBUG
            // Story 5.1 AC5: UITest hook —— XCUITest 通过 launchEnvironment 触发 keychain 写/读.
            KeychainUITestHookView(container: container)
            #endif
        }
        .onAppear {
            // Story 37.8 round 2 [P2] fix（codex review）：把 `bind(appState:)` 同步搬到 `.onAppear`，
            // 让 RealHomeViewModel / RealRoomViewModel 在第一次 paint 之前就持有 AppState 引用.
            //
            // 旧（round 1）实装在下方 `.task` 内调 bind(appState:) 是异步路径——SwiftUI 在第一次
            // paint 之前 `.task` 还没触发，HomeContainerView 互斥状态机已经按 appState.currentRoomId
            // 决定走 inRoom 分支 → RoomScaffoldView 渲染 → RealRoomViewModel.appState 仍 nil →
            // leave tap silent no-op + room title/code 显示 placeholder. 触发条件：
            //   · `/home` 返回 `room.currentRoomId != nil`（restored in-room session）→ AppLaunchStateMachine
            //     bootstrapStep1 内 appState.applyHomeData 写非 nil currentRoomId → ready 子树第一帧 paint
            //   · UITEST_FORCE_IN_ROOM env flag → ensureLaunchStateMachineWired 内 setCurrentRoomId 立即写
            //     非 nil → ready 子树第一帧 paint
            //
            // `.onAppear` 在 SwiftUI 第一次 paint 之前同步执行，比 `.task` 早 → 任何渲染之前 VM 已绑定.
            // 复用现有 bind(appState:) 入口（idempotent；alreadySubscribed guard），不动 ViewModel 持有结构.
            // 详见 docs/lessons/2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md.
            homeViewModel.bind(appState: appState)
            // Story 12.7 AC8: 注入 CreateRoom / JoinRoom UseCase + ErrorPresenter 到 RealHomeViewModel.
            // 让 onCreateTap / onJoinRoomConfirm override 走真实 server 路径（以前只 log + 直写 appState 占位）.
            //
            // Story 12.7 r2 [P1] fix（codex review）：UITEST_SKIP_GUEST_LOGIN=1 路径下**不**注入 real
            // CreateRoom / JoinRoom / LeaveRoom UseCase —— 让 ViewModel override 走既有 nil-fallback path
            // （直接 `appState.setCurrentRoomId(...)`），不调真实 HTTP. 否则 UITest 路径无 token /
            // 无 backend → HTTP 失败 → `RoomScaffoldView` 永远不出现 → `testJoinRoomModalCrossScreenJoinFlow`
            // 等依赖 fallback 的回归用例持续 broken.
            //
            // **Story 12.7 r3 [P1] fix**（codex review）：扩展上一轮 r2 gate 的精神 —— UITEST_SKIP_GUEST_LOGIN
            // 路径下 `webSocketClient` 与 `errorPresenter` 同样**不**注入：
            //   · `webSocketClient`：UITEST_FORCE_IN_ROOM 写 currentRoomId 后 `subscribeRoomIdConnect`
            //     nil→A 分支会尝试 real `connect(roomId:)`；UITEST 无 token → 抛 `WSError.tokenMissing`.
            //     即使 WS connect failure 现已不走 errorPresenter（r3 P1#1 修），仍会触发额外背景
            //     Task 与日志噪声，且 UITest 验证 RoomScaffoldView 直接渲染时无需真实 WS.
            //   · `errorPresenter`：UITEST 路径下若 onLeaveTap 走 fallback 已不需要 presenter；
            //     业务 onCreateTap / onJoinTap 也走 fallback（无 UseCase）→ 注入 presenter 亦无用途.
            //     保留 nil 让 vm 完全脱离 errorPresentationHost 路径，避免 UITest 期间任何透传
            //     至 modal retry overlay 的可能性（防回归）.
            //
            // 详见 docs/lessons/2026-05-11-uitest-launch-path-must-preserve-usecase-nil-fallback.md
            // 与 docs/lessons/2026-05-11-uitest-skip-real-ws-and-error-presenter-wiring.md.
            let isUITestSkipGuestLogin: Bool = {
                #if DEBUG
                return ProcessInfo.processInfo.environment["UITEST_SKIP_GUEST_LOGIN"] == "1"
                #else
                return false
                #endif
            }()
            // Story 12.7 r14 [P1] fix（codex review）：home refresh closure 共享给 RealHomeViewModel /
            // RealRoomViewModel / RealFriendsViewModel —— UseCase 抛 RoomNavigationStaleError 时让
            // /home 重新 hydrate 拿 authoritative state. UITEST 路径仍传 nil 让 stale path silent skip.
            // 提取为局部 binding 避免在 body 内 inline 三次造成 type-checker 复杂度爆炸（Swift 编译器
            // failed to produce diagnostic）.
            // 详见 docs/lessons/2026-05-11-stale-usecase-success-must-refresh-not-silently-return.md.
            let refreshHomeClosure: @MainActor @Sendable () -> Void = { [weak homeViewModel] in
                homeViewModel?.resetLoadHomeForRetry()
                Task { @MainActor [weak homeViewModel] in
                    await homeViewModel?.loadHome()
                }
            }
            if let realHomeVM = homeViewModel as? RealHomeViewModel, !isUITestSkipGuestLogin {
                realHomeVM.bind(
                    createRoomUseCase: container.makeCreateRoomUseCase(appState: appState),
                    joinRoomUseCase: container.makeJoinRoomUseCase(appState: appState),
                    errorPresenter: container.errorPresenter,
                    refreshHomeOnStaleNavigation: refreshHomeClosure
                )
            }
            if let realRoomVM = roomViewModel as? RealRoomViewModel {
                // Story 12.7 AC8 关键改动：webSocketClient 从 nil 升级为 container.webSocketClient（节点 4 真实 client 接通）.
                // 同时注入 LeaveRoomUseCase + ErrorPresenter 让 onLeaveTap 走真实 server 路径（HTTP 200 / 6004 → setCurrentRoomId(nil)）.
                //
                // Story 12.7 r2 [P1] fix：UITEST 路径下 `leaveRoomUseCase` 传 nil → onLeaveTap 走老 fallback
                // 直写 appState.setCurrentRoomId(nil)（与 UITEST_FORCE_IN_ROOM 路径配套；让 leave 不走 HTTP）.
                //
                // Story 12.7 r3 [P1] fix：UITEST 路径下 `webSocketClient` 与 `errorPresenter` 同样传 nil ——
                // 防止 UITEST_FORCE_IN_ROOM 触发 nil→A `connect(roomId:)` 抛 WSError.tokenMissing,
                // 让 vm 完全走 nil-fallback 路径（wsState 保持 .disconnected；RoomScaffoldView 仍渲染）.
                // 与 RealHomeViewModel.bind / RealFriendsViewModel.bind 的 UITEST gate 同精神.
                // 详见 docs/lessons/2026-05-11-uitest-skip-real-ws-and-error-presenter-wiring.md.
                // Story 12.7 r14 [P1] fix：注入 home refresh closure（onLeaveTap stale path 触发）.
                // UITEST 路径下传 nil（与 leaveRoomUseCase / webSocketClient / errorPresenter 同 gate）.
                // Story 18.3 AC8: 注入 sendEmojiUseCase + emojiCatalogLoader 让 onEmojiSelected 走完整链路
                //   (本地动效 + V1 §12.2 缓存校验 + WS fire-and-forget send + 失败 toast).
                // UITEST 路径下 sendEmojiUseCase 仍 nil (与 webSocketClient / leaveRoomUseCase 同 gate):
                //   - sendEmojiUseCase 内部依赖 webSocketClient, UITEST 路径下 wsClient = nil → 无 client 可注入,
                //     onEmojiSelected 走 nil-fallback 仍触发本地动效但跳过 WS send.
                //   - emojiCatalogLoader 用 container.loadEmojisUseCase (stable singleton, UITEST mock 路径也 OK
                //     因 UITEST_MOCK_EMOJI=1 时 loadEmojisUseCase 已注入 UITestMockEmojiRepository → 校验仍可命中
                //     mock fixture).
                realRoomVM.bind(
                    appState: appState,
                    webSocketClient: isUITestSkipGuestLogin ? nil : container.webSocketClient,
                    leaveRoomUseCase: isUITestSkipGuestLogin ? nil : container.makeLeaveRoomUseCase(appState: appState),
                    sendEmojiUseCase: isUITestSkipGuestLogin ? nil : container.makeSendEmojiUseCase(),
                    emojiCatalogLoader: container.loadEmojisUseCase,
                    errorPresenter: isUITestSkipGuestLogin ? nil : container.errorPresenter,
                    refreshHomeOnStaleNavigation: isUITestSkipGuestLogin ? nil : refreshHomeClosure
                )
            }
            // Story 37.9 AC5 Task 5.4：与 RealRoomViewModel.bind 同精神，
            // `.onAppear` 同步路径让 RealWardrobeViewModel 在第一次 paint 之前持有 AppState 引用
            // （按 Story 37.8 round 2 [P2] lesson 钦定路径，**不**放 `.task`）.
            if let realWardrobeVM = wardrobeViewModel as? RealWardrobeViewModel {
                realWardrobeVM.bind(appState: appState)
            }
            // Story 37.10 AC5 Task 5.4：与 RealRoomViewModel.bind / RealWardrobeViewModel.bind 同精神，
            // `.onAppear` 同步路径让 RealFriendsViewModel 在第一次 paint 之前持有 AppState 引用
            // （按 Story 37.8 round 2 [P2] lesson 钦定路径，**不**放 `.task`）.
            // Story 12.7 AC8: 同时注入 JoinRoomUseCase + ErrorPresenter 让 onJoinFriendTap 走真实 server 路径.
            //
            // Story 12.7 r2 [P1] fix：与 RealHomeViewModel.bind 同精神 —— UITEST 路径下 `joinRoomUseCase`
            // 传 nil → onJoinFriendTap 走老 fallback 直写 appState.setCurrentRoomId(...).
            if let realFriendsVM = friendsViewModel as? RealFriendsViewModel {
                // Story 12.7 r14 [P1] fix：注入 home refresh closure（onJoinFriendTap stale path 触发）.
                realFriendsVM.bind(
                    appState: appState,
                    joinRoomUseCase: isUITestSkipGuestLogin ? nil : container.makeJoinRoomUseCase(appState: appState),
                    errorPresenter: container.errorPresenter,
                    refreshHomeOnStaleNavigation: isUITestSkipGuestLogin ? nil : refreshHomeClosure
                )
            }
            // Story 37.11 AC5 Task 5.4：与 RealRoomViewModel.bind / RealWardrobeViewModel.bind /
            // RealFriendsViewModel.bind 同精神，`.onAppear` 同步路径让 RealProfileViewModel 在
            // 第一次 paint 之前持有 AppState 引用（按 Story 37.8 round 2 [P2] lesson 钦定路径，
            // **不**放 `.task`）.
            if let realProfileVM = profileViewModel as? RealProfileViewModel {
                realProfileVM.bind(appState: appState)
            }

            // Story 8.4 AC4: 同步 wire MotionProvider（与 bind(appState:) 同精神 — 在 first paint 前注入）.
            //
            // 在 .onAppear 而非 .task：与 Story 37.8 round 2 [P2] lesson `onappear-vs-task-sync-bind-before-first-paint.md`
            //   钦定路径同精神 —— ViewModel 在 first paint 前必须持有订阅引用，否则 catStage 第一帧渲染时
            //   petSlot() 调用 PetSpriteView(state: viewModel.petState) 会拿到默认 .rest（视觉本就该如此），
            //   但**第一次** activity event 到达前 petState 不会更新；如果用户在 1s 内（首帧到第一次 motion event 之间）
            //   切走然后回来，subscribe 漏触发的话视觉永远停在 .rest. 同步 bind 让此 race 不存在.
            //
            // **不**调 motionProvider.requestPermission（按 AR17 / 节点 3 设计：权限按需）.
            //   Story 8.5 同步触发器内部会走 8.2 motionProvider.requestPermission 链路；
            //   未授权时 CMMotionActivityManager 默默不发事件 → handler 不被调 → petState 保持 .rest 默认值.
            homeViewModel.bind(motionProvider: container.motionProvider)

            ensureLaunchStateMachineWired()
            // Story 8.5 AC9: lazy 注入 stepSyncTriggerService（与 ensureLaunchStateMachineWired 同模式）.
            // 必须在 ensureLaunchStateMachineWired 之后调（与未来 audit 期望顺序一致）.
            ensureStepSyncWired()
            // Story 15.4 AC4: lazy 注入 petStateSyncTriggerService（与 ensureStepSyncWired 同模式）.
            ensurePetStateSyncWired()
            // Story 15.5 AC3: 把 PetStateSyncTriggerService 作为 reconnect alignment delegate 注入 vm.
            // 时机：vm.bind(appState:webSocketClient:) 之后（vm 字段已就位）+ ensurePetStateSyncWired
            // 之后（service 已构造）. UITEST_SKIP_GUEST_LOGIN=1 路径下 service 仍构造（与 stepSync 同行为）；
            // 弱引用让 wire 残缺时 vm.handle .connected 分支 ?. 短路.
            if let realRoomVM = roomViewModel as? RealRoomViewModel {
                realRoomVM.bindReconnectAlignmentDelegate(petStateSyncTriggerService)
            }
        }
        .task {
            // Story 5.5：bind LoadHomeUseCase + ErrorPresenter，让 ErrorPresenter onRetry 闭包
            // 能驱动 ViewModel 重试. 这两个依赖 container.makeLoadHomeUseCase() / container.errorPresenter
            // 构造（容器初始化），保留 .task 异步路径.
            homeViewModel.bind(
                loadHomeUseCase: container.makeLoadHomeUseCase(),
                errorPresenter: container.errorPresenter
            )
        }
        .task {
            ensureLaunchStateMachineWired()
            await launchStateMachine?.bootstrap()
        }
        .onChange(of: scenePhase) { oldPhase, newPhase in
            // Story 8.4 review round 2 P2 fix: app 从 background / inactive 回到 .active 时
            // 重新 bind 一次 motionProvider —— 覆盖用户去 Settings 改 Motion 权限再回 app 的路径.
            //
            // bind 入口幂等：
            //   · 已 startUpdates → hasStartedMotionUpdates short-circuit 直接 return；
            //   · 仍未授权 → authorization gate 仍 return；
            //   · 首次到达 .authorized（用户刚去 Settings 授权回来）→ 第一次走完 startUpdates 注册 handler.
            // 因此 reactivate 触发 bind 不会重复订阅 / 不会触发权限弹窗 / 不会破坏既有 first-launch 红线.
            //
            // 仅在 `oldPhase != .active && newPhase == .active` 时触发：
            //   · 滤掉 .active → .inactive（如系统通知 banner）的反向边沿；
            //   · 滤掉首次启动时 .background → .inactive → .active 中的 .inactive 边沿（避免重复 bind）.
            // 详见 docs/lessons/2026-05-04-scenephase-rebind-for-auth-gated-subscriptions.md.

            // Story 8.5 AC9: background 进入时 stop step sync timer（节省电；下次 .active 重启）.
            // Story 15.4 AC4: 同时 stop pet state-sync subscription（释放订阅引用；下次 .active 重启幂等）.
            if newPhase == .background {
                stepSyncTriggerService?.stop()
                petStateSyncTriggerService?.stop()
            }

            guard newPhase == .active, oldPhase != .active else { return }
            homeViewModel.bind(motionProvider: container.motionProvider)

            // Story 8.5 AC9 / codex review round 1 [P3] fix: 回前台只调 `start()`（幂等）.
            //
            // 旧实装（round 1 review 命中）同时调 `triggerForeground()` + `start()` —— 两个 Task
            // 各 spawn 一次 performSync；in-flight gate 仅压重叠，第一个完成后第二个即发出 duplicate
            // `/steps/sync`. 修复：start() 自身改成幂等 reactivate 入口（首次启 timer + 立即 sync；
            // 后续仅立即 sync，不重启 timer）.详见 lesson
            // docs/lessons/2026-05-04-scenephase-idempotent-start-no-duplicate-trigger.md.
            //
            // Story 8.5 review round 2 [P2] fix: 加 `.ready` gate —— launch state 不在 .ready
            // 时（如 token expiry 401 后被 cold-start 路径重置回 .launching/.needsAuth），
            // 即便 scenePhase .active 也不应 restart step sync timer（无 valid session token,
            // 调 /steps/sync 必 401 → 又触发 cold-start 自激循环）.重新进入 .ready 时由
            // onReadyTask 路径调 service.start()，本路径不再越权.
            // 详见 docs/lessons/2026-05-04-launch-state-leave-ready-must-stop-feature-services.md.
            if launchStateMachine?.state == .ready {
                stepSyncTriggerService?.start()
                // Story 15.4 AC4: 与 stepSync 同 .ready guard —— 仅当 launch state 在 .ready 时才 start
                // pet state-sync subscription（幂等：subscription != nil 短路；resume 与 first start 同处理）.
                petStateSyncTriggerService?.start()
            }
        }
        .errorPresentationHost(presenter: container.errorPresenter)
    }

    /// 取当前 ResetIdentityViewModel? 给 LaunchedContentView (Debug 注入；Release 永远 nil).
    private func currentResetIdentityViewModel() -> ResetIdentityViewModel? {
        #if DEBUG
        return resetIdentityViewModel
        #else
        return nil
        #endif
    }

    /// Story 8.5 AC9: lazy 注入 `stepSyncTriggerService`（与 ensureLaunchStateMachineWired 同模式）.
    /// nil 守卫确保 RootView 重建（旋转 / 离开返回）时不重新构造覆盖既有 instance —— 否则旧 timer 仍在跑.
    /// option A：service 持 homeViewModel 借用 petState 作为 motionState 来源（不订阅 motionProvider）.
    private func ensureStepSyncWired() {
        guard stepSyncTriggerService == nil else { return }
        stepSyncTriggerService = container.makeStepSyncTriggerService(
            appState: appState,
            homeViewModel: homeViewModel
        )
    }

    /// Story 15.4 AC4: lazy 注入 `petStateSyncTriggerService`（与 ensureStepSyncWired 同模式）.
    /// nil 守卫防 RootView 重建覆盖既有 service instance —— 否则旧 subscription 仍活 + 新 subscription
    /// 也建 → 重复 spawn 同 state-sync.service 持 homeViewModel + appState weak 引用，
    /// 借 `homeViewModel.$petState` publisher 触发 + `appState.currentRoomId` 同步读 preflight.
    private func ensurePetStateSyncWired() {
        guard petStateSyncTriggerService == nil else { return }
        petStateSyncTriggerService = container.makePetStateSyncTriggerService(
            appState: appState,
            homeViewModel: homeViewModel
        )
    }

    /// Story 5.2 AC8 新增：lazy 注入 `launchStateMachine`，把 GuestLoginUseCase + SessionStore wire 到 bootstrapStep1.
    private func ensureLaunchStateMachineWired() {
        guard launchStateMachine == nil else { return }

        #if DEBUG
        if ProcessInfo.processInfo.environment["UITEST_SKIP_GUEST_LOGIN"] == "1" {
            // UITest 路径：bootstrap 立即成功，复用 Story 2.9 默认 closure 行为（让 HomeView 可渲染）
            launchStateMachine = AppLaunchStateMachine()
            // Story 37.8 AC8: UITest 路径强制切到 inRoom 态（让 RoomScaffoldView 7 锚 a11y identifier 可定位）.
            // 仅 Debug build 生效；Production build 此 env 被忽略.
            // 复用 Story 37.4 setCurrentRoomId 入口；不污染生产 wire；UITest 跑完 reset env 即恢复正常.
            // env flag 名 `UITEST_FORCE_IN_ROOM` 与现有 `UITEST_SKIP_GUEST_LOGIN` 同前缀（Story 2.2 落地的命名风格）.
            if ProcessInfo.processInfo.environment["UITEST_FORCE_IN_ROOM"] == "1" {
                appState.setCurrentRoomId("1234567")
            }
            // Story 21.1 AC8: 注入 mock counting 态宝箱让 UITest 可定位 chestCard_counting 锚.
            // 命名 `UITEST_CHEST_COUNTING` 与 UITEST_SKIP_GUEST_LOGIN / UITEST_FORCE_IN_ROOM 同前缀.
            // SKIP_GUEST_LOGIN 是登录态绕过；本 env 是具体 mock 数据精细控制，两个职责正交.
            // 仅 Debug build 生效；Production 路径忽略.
            if ProcessInfo.processInfo.environment["UITEST_CHEST_COUNTING"] == "1" {
                appState.currentChest = HomeChest(
                    id: "uitest-c1",
                    status: .counting,
                    unlockAt: Date().addingTimeInterval(300),
                    openCostSteps: 1000,
                    remainingSeconds: 300
                )
            }
            return
        }
        #endif

        let useCase = container.makeGuestLoginUseCase()
        let loadHomeUseCase = container.makeLoadHomeUseCase()
        let sessionStore = container.sessionStore
        let homeViewModel = self.homeViewModel
        let appState = self.appState
        launchStateMachine = AppLaunchStateMachine(
            bootstrapStep1: { @Sendable in
                let output: GuestLoginOutput
                do {
                    output = try await useCase.execute()
                } catch {
                    throw BootstrapMappedError(
                        presentation: AppErrorMapper.presentation(for: error),
                        underlying: error
                    )
                }
                await MainActor.run {
                    sessionStore.updateSession(SessionState(user: output.user, pet: output.pet))
                }
                let homeData: HomeData
                do {
                    homeData = try await loadHomeUseCase.execute()
                } catch {
                    throw BootstrapMappedError(
                        presentation: AppErrorMapper.presentation(for: error),
                        underlying: error
                    )
                }
                await MainActor.run {
                    // Story 37.4 AC3：直接写 AppState（而非通过 coordinator.currentRoomId 双写）.
                    // 设计决议：保留 homeViewModel.applyHomeData(homeData) 调用 ——
                    // HomeViewModel 内 loadingState / hasLoadedHome 短路 flag 仍归 HomeViewModel
                    // 自己管（ADR-0010 §3.2 钦定 loading 状态归 ViewModel transient，不进 AppState）；
                    // HomeViewModel.applyHomeData 内部既调 self.appState?.applyHomeData(data) 写
                    // AppState，又设 loadingState=.loaded 与 hasLoadedHome=true.
                    // RootView 这里也直接调 appState.applyHomeData(homeData) 是为了：
                    // 让 AppState hydrate 不依赖 HomeViewModel 实例存在（如未来 LaunchingViewModel
                    // 直接调 LoadHomeUseCase 也能写 AppState）；双写**不**是 anti-pattern：内层
                    // HomeViewModel.applyHomeData 写 AppState 会重复，但因为是同一个 AppState
                    // 实例 + idempotent 赋值（同值），测试可断言 currentUser 与 hasLoadedHome 两套语义并存.
                    appState.applyHomeData(homeData)
                    homeViewModel.applyHomeData(homeData)
                }
            }
        )

        // ADR-0008 v2 §6.3 / Story 0008-impl-1: wire 401 cold-start handler.
        if let stateMachine = launchStateMachine {
            container.unauthorizedHandlerSink.setHandler { [weak stateMachine, weak sessionStore] in
                await MainActor.run {
                    sessionStore?.clear()
                }
                await stateMachine?.triggerColdStart()
            }
        }
    }
}

// Story 5.5 round 3 [P1] fix: 移除原 GuestLoginCompletionGate actor.
// 详见 docs/lessons/2026-04-27-bootstrap-retry-must-not-skip-auth.md.

/// Story 5.5 codex round 1 [P2] fix + round 2 [P1] fix: 把 bootstrap step closure 内的失败
/// 包装成携带完整 ErrorPresentation 语义的 LocalizedError, 让状态机决定 retry vs alert vs toast.
struct BootstrapMappedError: LocalizedError {
    let presentation: ErrorPresentation
    let underlying: Error

    var errorDescription: String? {
        switch presentation {
        case let .toast(message):
            return message
        case let .alert(_, message):
            return message
        case let .retry(message):
            return message
        }
    }
}

/// Story 5.2 新增子视图：用 `@ObservedObject` 订阅 `AppLaunchStateMachine.state` 的变化.
///
/// Story 37.3 改造（ADR-0009 §3.5 步骤 1）：
///   - .ready 分支改渲染 MainTabView + 注入 4 个 environment（coordinator / homeViewModel /
///     resetIdentityViewModel / sessionStore），让 HomeContainerView 通过 environment 透传给 HomeView.
///   - homeView 闭包参数删除（不再渲染 HomeView 当根；HomeContainerView 内嵌 HomeView 由 MainTabView
///     的 Home Tab 拿到）.
///   - **保留** `.fullScreenCover(item: $coordinator.presentedSheet)` modifier 服务 `.compose` 次级
///     路由（codex round 1 [P2] fix）. 渲染 ComposePlaceholderView（Story 33.1 落地真实合成 view）.
private struct LaunchedContentView: View {
    @ObservedObject var stateMachine: AppLaunchStateMachine
    @ObservedObject var coordinator: AppCoordinator
    let homeViewModel: HomeViewModel
    /// Story 37.8 AC5：接收 RootView 的 RoomViewModel，注入到 .ready 子树 environmentObject.
    /// HomeContainerRoomViewBridge 通过 @EnvironmentObject var roomViewModel: RoomViewModel 取出.
    let roomViewModel: RoomViewModel
    /// Story 37.9 AC5：接收 RootView 的 WardrobeViewModel，注入到 .ready 子树 environmentObject.
    /// WardrobeView 通过 @EnvironmentObject var wardrobeViewModel: WardrobeViewModel 取出后传给
    /// WardrobeScaffoldView(state:) 子视图.
    let wardrobeViewModel: WardrobeViewModel
    /// Story 37.10 AC5：接收 RootView 的 FriendsViewModel，注入到 .ready 子树 environmentObject.
    /// FriendsView 通过 @EnvironmentObject var friendsViewModel: FriendsViewModel 取出后传给
    /// FriendsScaffoldView(state:) 子视图.
    let friendsViewModel: FriendsViewModel
    /// Story 37.11 AC5：接收 RootView 的 ProfileViewModel，注入到 .ready 子树 environmentObject.
    /// ProfileView 通过 @EnvironmentObject var profileViewModel: ProfileViewModel 取出后传给
    /// ProfileScaffoldView(state:) 子视图.
    let profileViewModel: ProfileViewModel
    /// Story 37.4 AC3：接收 RootView 的 AppState，注入到 .ready 子树 environmentObject.
    let appState: AppState
    /// Story 37.5 AC8: 接收 RootView 的 ThemeName，转 Theme 实例后注入 .ready 子树 environment(\.theme).
    let currentTheme: ThemeName
    let sessionStore: SessionStore?
    let resetIdentityViewModel: ResetIdentityViewModel?
    /// Story 18.2 AC5: EmojiPanelViewModel 工厂闭包；通过 `\.emojiPanelViewModelFactory` environment
    /// value 注入 .ready 子树, HomeContainerRoomViewBridge 读取后传给 RoomScaffoldView init.
    let emojiPanelViewModelFactory: () -> EmojiPanelViewModel
    /// Story 18.4 AC8: LoadEmojisUseCase stable singleton；通过 `\.loadEmojisUseCase` environment value
    /// 注入 .ready 子树, HomeContainerRoomViewBridge 读取后传给 RoomScaffoldView init →
    /// EmojiAnimationLayer 内 FloatingEmojiCellView .task 查 catalog 拿 assetUrl.
    let loadEmojisUseCase: LoadEmojisUseCaseProtocol?
    let onReadyAppear: () -> Void
    let onReadyTask: () async -> Void
    /// Story 8.5 review round 2 [P2] fix: launch state 离开 `.ready` 时回调（同步路径）.
    /// 用于让外层 RootView 停 step sync timer，避免 token expiry 401 → cold-start 重置回
    /// `.launching` / `.needsAuth` 期间 timer 仍 5min 跑一次 sync（用清掉的 token → 又 401 自激循环）.
    /// 详见 docs/lessons/2026-05-04-launch-state-leave-ready-must-stop-feature-services.md.
    let onLeaveReady: () -> Void

    init(
        stateMachine: AppLaunchStateMachine,
        coordinator: AppCoordinator,
        homeViewModel: HomeViewModel,
        roomViewModel: RoomViewModel,
        wardrobeViewModel: WardrobeViewModel,
        friendsViewModel: FriendsViewModel,
        profileViewModel: ProfileViewModel,
        appState: AppState,
        currentTheme: ThemeName,
        sessionStore: SessionStore?,
        resetIdentityViewModel: ResetIdentityViewModel?,
        emojiPanelViewModelFactory: @escaping () -> EmojiPanelViewModel,
        loadEmojisUseCase: LoadEmojisUseCaseProtocol?,
        onReadyAppear: @escaping () -> Void,
        onReadyTask: @escaping () async -> Void = { },
        onLeaveReady: @escaping () -> Void = { }
    ) {
        self.stateMachine = stateMachine
        self.coordinator = coordinator
        self.homeViewModel = homeViewModel
        self.roomViewModel = roomViewModel
        self.wardrobeViewModel = wardrobeViewModel
        self.friendsViewModel = friendsViewModel
        self.profileViewModel = profileViewModel
        self.appState = appState
        self.currentTheme = currentTheme
        self.sessionStore = sessionStore
        self.resetIdentityViewModel = resetIdentityViewModel
        self.emojiPanelViewModelFactory = emojiPanelViewModelFactory
        self.loadEmojisUseCase = loadEmojisUseCase
        self.onReadyAppear = onReadyAppear
        self.onReadyTask = onReadyTask
        self.onLeaveReady = onLeaveReady
    }

    var body: some View {
        ZStack {
            switch stateMachine.state {
            case .launching:
                LaunchingView()
                    .transition(.opacity)
            case .ready:
                MainTabView()
                    .environmentObject(coordinator)
                    .environmentObject(homeViewModel)
                    .environmentObject(roomViewModel)
                    .environmentObject(wardrobeViewModel)
                    .environmentObject(friendsViewModel)
                    .environmentObject(profileViewModel)
                    .environmentObject(appState)
                    .environment(\.theme, currentTheme.theme)
                    .environment(\.sessionStore, sessionStore)
                    .environment(\.resetIdentityViewModel, resetIdentityViewModel)
                    // Story 18.2 AC5: factory 闭包注入 .ready 子树 environment,
                    // HomeContainerRoomViewBridge 通过 `\.emojiPanelViewModelFactory` 读取并传给 RoomScaffoldView init.
                    .environment(\.emojiPanelViewModelFactory, emojiPanelViewModelFactory)
                    // Story 18.4 AC8: LoadEmojisUseCase stable singleton 注入 .ready 子树 environment,
                    // HomeContainerRoomViewBridge 通过 `\.loadEmojisUseCase` 读取并传给 RoomScaffoldView init →
                    // EmojiAnimationLayer 内 FloatingEmojiCellView .task 查 catalog 拿 assetUrl. UITEST 路径下
                    // (UITEST_MOCK_EMOJI=1) container.loadEmojisUseCase 已是 UITestMockEmojiRepository fixture,
                    // 注入路径不变.
                    .environment(\.loadEmojisUseCase, loadEmojisUseCase)
                    .onAppear { onReadyAppear() }
                    .task {
                        await onReadyTask()
                    }
                    // codex round 1 [P2] fix: 重新挂回 .fullScreenCover —— ADR-0009 §3.4 SheetType
                    // 白名单仍含 .compose（Story 33.1 落地真实形式）；删 modifier 会让
                    // coordinator.present(.compose) 变 silent no-op.
                    // 详见 docs/lessons/2026-04-30-coordinator-must-mirror-loaded-home-room-state.md.
                    .fullScreenCover(item: $coordinator.presentedSheet) { sheet in
                        switch sheet {
                        case .compose:
                            ComposeSheetPlaceholder()
                        }
                    }
                    .transition(.opacity)
            case .needsAuth(let presentation):
                needsAuthContent(for: presentation)
                    .transition(.opacity)
            }
        }
        .animation(.easeInOut(duration: 0.2), value: stateMachine.state)
        // Story 8.5 review round 2 [P2] fix: 监听 launch state 切换，离开 `.ready` 时
        // 调 onLeaveReady 让外层停 step sync timer（避免 token expiry 401 后 timer 仍跑）.
        // 重新进入 `.ready` 时由 .ready 分支的 .task → onReadyTask 重启 timer（既有路径，幂等）.
        .onChange(of: stateMachine.state) { oldState, newState in
            let wasReady: Bool = {
                if case .ready = oldState { return true }
                return false
            }()
            let isReady: Bool = {
                if case .ready = newState { return true }
                return false
            }()
            if wasReady && !isReady {
                onLeaveReady()
            }
        }
    }

    /// Story 5.5 round 8 [P1] fix（终极方案）: bootstrap 路径的 `.alert` / `.toast` 改用
    /// 全新的 `TerminalErrorView` (静态全屏 fallback page，无任何按钮，user 必须主动杀进程退出).
    @ViewBuilder
    private func needsAuthContent(for presentation: ErrorPresentation) -> some View {
        switch presentation {
        case let .retry(message):
            RetryView(
                message: message,
                onRetry: { Task { await stateMachine.retry() } }
            )
        case let .alert(title, message):
            TerminalErrorView(title: title, message: message)
        case let .toast(message):
            TerminalErrorView(title: "提示", message: message)
        }
    }
}

/// Story 37.3 codex round 1 [P2] fix: `.compose` 路由的临时占位 view.
///
/// ADR-0009 §3.4 SheetType 白名单仍保留 `.compose`（Story 33.1 决定具体形式 / 落地真实合成 view）.
/// 在此之前，coordinator.present(.compose) 必须有 view 挂载，否则 state 改了但 UI 不渲染.
///
/// a11y identifier `compose_placeholder` 让 UITest 能验证"present(.compose) 后 sheet 真的弹出".
struct ComposeSheetPlaceholder: View {
    var body: some View {
        Text("compose placeholder")
            .accessibilityIdentifier(AccessibilityID.Compose.placeholder)
    }
}
