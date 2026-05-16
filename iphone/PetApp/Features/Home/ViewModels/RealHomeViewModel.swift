// RealHomeViewModel.swift
// Story 37.7 AC2: HomeViewModel 生产实装子类（构造注入 AppState；override 5 个 abstract method 占位 stub）.
//
// 范围（本 story 占位；Story 12.7 / 21.x 等填充真实 UseCase 调用）：
//   - 构造注入 AppState（按 ADR-0010 §3.1 ViewModel 注入规则）
//   - override onCreateTap / onJoinTap / onFeedTap / onPetTap / onPlayTap 为占位行为：
//     · onCreateTap: print log（Story 12.7 实装 CreateRoomUseCase）
//     · onJoinTap: 设 showJoinModal = true（与 Mock 同行为；Story 12.7 / 37.12 落地真实 modal 闭包）
//     · onFeedTap / onPetTap / onPlayTap: 设 interactionAnimation = .flying(emoji)（与 Mock 同行为；
//       未来 Story 14.x WS pet.state.changed 真实状态切换时再分化）
//
// **不**调用任何 UseCase / Repository / APIClient（Epic 37 红线：UI Scaffold 数据完全 mock）.
//
// Story 37.7 codex round 1 [P1] fix：新增 parameterless `init()` 重载.
//   原因：RootView 走 `@StateObject private var homeViewModel = RealHomeViewModel()` 老模式时,
//   AppState 也是同级 @StateObject，不能在属性初始化器内交叉引用（编译期不允许 self 提前求值）.
//   解法：保留 `init(appState:)` 主入口，新增 parameterless `init()` 走基类老 init + 后续 `bind(appState:)`
//   注入（与 pingUseCase / loadHomeUseCase / appState 既有 bind() 模式一致）.
//   注：`onCreateTap` 等 override 不依赖 self.appState 任何字段（仅写 self.showJoinModal /
//   self.interactionAnimation）；bind(appState:) 时机晚也不会让 abstract method crash.
//
// Story 37.7 codex round 3 [P2-A] fix：override applyHomeData(_:) 让 greeting 从 hydrated AppState
//   currentPet.name 派生 —— 老 configureMockDefaults 只设静态 placeholder "想你啦 ♥",
//   bootstrap 注入 HomeData 后 RealHomeViewModel.greeting 仍 hardcode 不 propagate pet name,
//   生产用户永远看不到自己宠物名字. override 链路：先调 super.applyHomeData(data) 写 AppState +
//   置 hasLoadedHome flag,再读 self.appState.currentPet.name 拼 greeting. AppState 在 super
//   调用内已 hydrate,此处读为最新值.
//
// Story 37.7 codex round 4 [P3] fix：把 greeting 派生从 applyHomeData(_:) 一次性回调改成
//   订阅 appState.$currentPet 的 Combine sink —— 解决 round 3 留下的 reset 后 greeting stale 问题.
//   ResetIdentityViewModel.tap() → appState.reset() 把 currentPet 置 nil；老 round 3 实装只在
//   applyHomeData 入口派生 greeting，reset 路径不经过 applyHomeData → header 仍显示旧 pet name 直到
//   下一次 hydrate. round 4 改成 reactive sink：appState.$currentPet 任何变化（hydrate / reset /
//   单独 mutate）都自动重新派生 greeting，不再依赖单一入口.
//   sink 在 init(appState:) 与 bind(appState:) 两路 hookup（与 base bind 一次性短路同步）.

import Foundation
import Combine
import os.log

@MainActor
public final class RealHomeViewModel: HomeViewModel {

    /// Story 37.7 codex round 4 [P3] fix：订阅 appState.$currentPet 的 sink 句柄.
    /// 仅本子类持有；基类 self.appState 是 private 不可见. sink hookup 时机：
    ///   - init(appState:) 路径：构造完成后立即订阅
    ///   - bind(appState:) 路径：override 内 super 注入完后订阅（仅首次生效，防多次 .task 重订阅）
    private var greetingSubscription: AnyCancellable?

    /// Story 37.12: 子类持有 appState 引用（基类的 appState 是 private 不可见 →
    /// onJoinRoomConfirm 需要走 `appState?.setCurrentRoomId(roomId)` 规范入口）.
    /// 与 RealFriendsViewModel.appState 同模式（基类 + 子类双持，子类负责自己使用的入口；
    /// 基类 self.appState 仅 base class 内部 applyHomeData / sink 用）.
    private var localAppState: AppState?

    /// Story 12.7 AC5: CreateRoomUseCase 注入（默认 nil；caller=RootView 通过 bind() 注入 container.makeCreateRoomUseCase）.
    private var createRoomUseCase: CreateRoomUseCaseProtocol?

    /// Story 12.7 AC5: JoinRoomUseCase 注入（默认 nil；caller=RootView 通过 bind() 注入 container.makeJoinRoomUseCase）.
    private var joinRoomUseCase: JoinRoomUseCaseProtocol?

    /// Story 12.7 AC5: ErrorPresenter 注入（weak 引用避免循环）；caller=RootView 注入 container.errorPresenter.
    private weak var localErrorPresenter: ErrorPresenter?

    /// Story 12.7 r14 [P1] fix（codex review）：home refresh hook.
    /// 触发条件：CreateRoom / JoinRoom UseCase 抛 RoomNavigationStaleError —— server 已 commit
    /// 用户进 room 但 client UI 因 navigation race 没写 currentRoomId；需重拉 /home 拿 authoritative
    /// state 让 client/server 收敛.
    /// caller=RootView 注入一个调 `homeViewModel.resetLoadHomeForRetry()` + `await homeViewModel.loadHome()` 的 closure.
    /// 默认 nil：UITEST / fallback / preview 路径下不触发 refresh（保持现状 silent behavior）.
    private var refreshHomeOnStaleNavigation: (@MainActor @Sendable () -> Void)?

    /// Story 21.3 AC6: OpenChestUseCase 注入（默认 nil；caller=RootView 通过 bind() 注入 container.makeOpenChestUseCase）.
    private var openChestUseCase: OpenChestUseCaseProtocol?

    /// Story 21.1 AC5: 宝箱倒计时驱动（订阅 appState.$currentChest → 每秒写 viewModel.chestRemainingSeconds）.
    /// - var Optional 而非 let non-Optional：Swift `self` 在 super.init 前不可用，无法在字段声明初始化器内
    ///   传 `self` 给 ChestTimerDriver；只能 super.init 后再赋值（与 RealHomeViewModel.greetingSubscription 同节奏）.
    /// - 仅 RealHomeViewModel 持（Mock 不需要 driver；Preview / UITest 直接 hardcode chestRemainingSeconds 即可）.
    /// - 生命周期：driver 与 ViewModel 一致（ViewModel dealloc → driver dealloc → subscription / Task 自动 cancel）.
    private var chestTimerDriver: ChestTimerDriver?

    /// Story 37.7 codex round 1 [P1] fix：parameterless init 让 RootView `@StateObject` 老模式可用.
    /// AppState 通过 `bind(appState:)` 在 `.task` 内异步注入（与 pingUseCase / loadHomeUseCase 同模式）.
    /// 不再持 `injectedAppState` 字段（基类已保 self.appState；本类无独立持有需求）.
    /// 不写 `override`：基类没有显式 no-arg init（Swift 通过默认参数合成无参调用，不形成 override 关系）.
    public init() {
        super.init()
        configureMockDefaults()
    }

    public init(appState: AppState) {
        super.init(appState: appState)
        configureMockDefaults()
        self.localAppState = appState   // Story 37.12: 子类持有让 onJoinRoomConfirm 可访问
        // 构造路径已注入 AppState；立即订阅 currentPet 变化派生 greeting.
        subscribeGreeting(to: appState)
        // Story 21.1 AC5: 构造路径已注入 AppState → 立即创建并启动 chest timer driver.
        startChestTimerDriver(with: appState)
    }

    /// Story 37.7 codex round 4 [P3] fix：override base bind(appState:) 在异步注入路径也 hookup sink.
    /// 与 base 一次性 guard 同节奏（greetingSubscription == nil 表 sink 未建立 → 首次 bind 才订阅）.
    /// Story 21.1 AC5: 异步注入路径也 hookup chestTimerDriver（与 greetingSubscription 同节奏，一次性 guard）.
    public override func bind(appState: AppState) {
        let alreadySubscribed = greetingSubscription != nil
        let alreadyHasDriver = chestTimerDriver != nil
        super.bind(appState: appState)
        self.localAppState = appState   // Story 37.12: 异步注入路径也要更新子类持的引用
        if !alreadySubscribed {
            subscribeGreeting(to: appState)
        }
        if !alreadyHasDriver {
            startChestTimerDriver(with: appState)
        }
    }

    /// Story 21.1 AC5: 创建并启动 ChestTimerDriver；driver 持引用让生命周期与 ViewModel 一致.
    /// 两路 init（构造注入 + bind 注入）共享同一入口，避免分支漂移.
    private func startChestTimerDriver(with appState: AppState) {
        let driver = ChestTimerDriver(appState: appState, viewModel: self)
        driver.start()
        self.chestTimerDriver = driver
    }

    /// Story 12.7 AC5: 注入 CreateRoom / JoinRoom UseCase + ErrorPresenter.
    /// 与既有 bind(appState:) / bind(loadHomeUseCase:errorPresenter:) 同模式（独立入口，参数化注入）.
    /// 幂等：caller=RootView .onAppear 只调一次；多次调用覆盖既有引用（生产路径无意义，仅防错重）.
    /// **不**破坏基类 bind 的"first-time-only"约定：本入口不调 super 的任何 bind（不重订阅 publisher）.
    public func bind(
        createRoomUseCase: CreateRoomUseCaseProtocol,
        joinRoomUseCase: JoinRoomUseCaseProtocol,
        errorPresenter: ErrorPresenter,
        refreshHomeOnStaleNavigation: (@MainActor @Sendable () -> Void)? = nil
    ) {
        self.createRoomUseCase = createRoomUseCase
        self.joinRoomUseCase = joinRoomUseCase
        self.localErrorPresenter = errorPresenter
        // r14 [P1] fix: closure 用 default-nil 让 UITEST / preview / 测试不传时保持向后兼容.
        self.refreshHomeOnStaleNavigation = refreshHomeOnStaleNavigation
    }

    /// Story 21.3 AC6: 扩 bind 入口（与既有 bind(createRoomUseCase:joinRoomUseCase:errorPresenter:refreshHomeOnStaleNavigation:) 同模式）.
    /// 幂等：caller=RootView .onAppear 只调一次；多次调用覆盖既有引用（生产路径无意义，仅防错重）.
    /// **不**破坏基类 bind 的"first-time-only"约定：本入口不调 super 的任何 bind（不重订阅 publisher）.
    public func bind(openChestUseCase: OpenChestUseCaseProtocol) {
        self.openChestUseCase = openChestUseCase
    }

    /// 订阅 appState.$currentPet —— 任何 hydrate / reset / 单独 mutate 都派生 greeting.
    /// 派生公式：pet 有名字 → "{petName}，想你啦 ♥"；pet=nil 或空名 → 老 placeholder "想你啦 ♥".
    /// 用 sink 而非 assign(to:on:)：sink closure 持 weak self 避免 AppState ↔ ViewModel 循环引用.
    ///
    /// **关键 timing 细节**：`@Published` projected publisher 发的是 *willSet* 时机的**新值参数**
    /// （Apple docs 写 "The publisher emits before the value is changed." 但 closure 参数是 newValue,
    /// 即将赋上去的值，可直接用. 见 swift-evolution SE-0258 / Combine.Published.publisher 文档）.
    /// 所以这里直接用 `pet` 参数即可，不需要再 `receive(on:)` + 重读 self.appState.
    /// 不加 receive(on:) 让 unit test 在同 runloop tick 内看到 greeting 更新（无 dispatch 异步缝隙）.
    private func subscribeGreeting(to appState: AppState) {
        greetingSubscription = appState.$currentPet
            .sink { [weak self] pet in
                guard let self else { return }
                if let petName = pet?.name, !petName.isEmpty {
                    self.greeting = "\(petName)，想你啦 ♥"
                } else {
                    self.greeting = "想你啦 ♥"
                }
            }
    }

    /// 视觉初值统一入口（两路 init 都调；避免分支漂移）.
    private func configureMockDefaults() {
        // 视觉初值：从 AppState.currentPet?.name 派生 greeting（hydrate 后）；hydrate 前用空 placeholder
        self.greeting = "想你啦 ♥"
        self.weather = "今天 · 晴"
        self.stats = .mockHappy   // Story 8.x / 14.x 后接真实状态
        self.interactionAnimation = .idle
        self.showJoinModal = false
    }

    // MARK: - override abstract methods（本 story 占位；Story 12.7 / 14.x 实装真实 UseCase 调用）

    public override func onCreateTap() {
        // Story 12.7 AC5: 调 CreateRoomUseCase（POST /rooms → 写 appState.currentRoomId → UI 自动切到 RoomView）.
        // 失败路径：6003（已在房间）→ alert "你已经在房间里了"；其他错误走 ErrorPresenter 默认 mapper.
        guard let useCase = self.createRoomUseCase else {
            // Story 12.7 r5 [P3] fix（codex review）：useCase nil fallback —— 保留 UITEST_SKIP_GUEST_LOGIN=1
            // 启动模式下 / RootView 老 wire 路径下点 Create CTA 仍能切到 RoomView 的视觉行为.
            //
            // 与 onJoinRoomConfirm 的 fallback 同精神（直接 `localAppState?.setCurrentRoomId(roomId)`）：
            // 无 backend 的 UI tests / previews 下用户仍应看到 RoomView 进入动画 + RoomScaffoldView,
            // 否则 create CTA 变成 hard no-op（join / leave / friend-join 都还保留 fallback —— 只有 create 漏了）.
            //
            // 占位 roomId 用 "1234567"（与 MockHomeViewModel.onCreateTap 同精神 placeholder).
            // 详见 docs/lessons/2026-05-11-create-room-nil-fallback-must-mutate-state.md.
            os_log(.debug, "RealHomeViewModel.onCreateTap (fallback: no CreateRoomUseCase wired; write appState.currentRoomId placeholder directly)")
            self.localAppState?.setCurrentRoomId("1234567")
            return
        }
        let presenter = self.localErrorPresenter
        // fix-review round 10 P2 (Story 12.7): catch-path stale guard 升级到 generation token.
        // 场景：user 点 Create CTA → createRoom() HTTP in-flight 时切到 friend tab → 通过 friend
        // join room B → leave B 回 idle（currentRoomId nil → "B" → nil; gen 单调递增）→ create
        // 路径 HTTP 抛错迟到.
        // r9 旧实装用 currentRoomId equality 会被 ABA cycle 骗过（liveRoomId == nil == entryRoomId
        // → guard 通过 → stale alert overlay 弹在 idle Home 上）.
        // r10 升级到 `appState.roomNavigationGeneration` —— 单调计数器，A→B→A cycle 也 monotonic,
        // 不会重合. 详见 docs/lessons/2026-05-11-room-navigation-generation-token-not-room-id-equality.md.
        let entryGen = self.localAppState?.roomNavigationGeneration ?? 0
        let refreshHome = self.refreshHomeOnStaleNavigation
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                _ = try await useCase.execute()
                // 成功 → no-op：UseCase 已写 appState.currentRoomId → HomeContainerView 互斥状态机切 RoomView.
            } catch is RoomNavigationStaleError {
                // r14 [P1] fix（codex review）：UseCase 检测到 navigation race（entryGen != liveGen 但 server 已 commit）.
                // server 端用户已进 room → client UI 没切（仍 idle）→ 后续 create/join 会拿 6003.
                // 解：silent skip errorPresenter（用户视角下没出错）+ 触发 home refresh 拿 authoritative state.
                // 详见 docs/lessons/2026-05-11-stale-usecase-success-must-refresh-not-silently-return.md.
                os_log(.info,
                       "RealHomeViewModel.onCreateTap: caught RoomNavigationStaleError; trigger home refresh to reconcile authoritative state")
                refreshHome?()
            } catch let APIError.business(code, _, _) where code == 6003 {
                // 6003 用户已在房间中：明确文案 alert（不附 onRetry —— 用户应主动 leave 再创建）.
                // 但仍需 generation guard：若 navigation cycle 已发生，旧 create 的 6003 alert 不能弹.
                let liveGen = self.localAppState?.roomNavigationGeneration ?? 0
                guard liveGen == entryGen else {
                    os_log(.debug,
                           "RealHomeViewModel.onCreateTap: stale 6003 error (entryGen=%{public}d, currentGen=%{public}d); skip alert to avoid overlay on unrelated navigation cycle",
                           entryGen, liveGen)
                    return
                }
                presenter?.presentAlert(title: "提示", message: "你已经在房间里了")
            } catch {
                // 其他 APIError（network / unauthorized / 1009 / decoding 等）走默认 mapper（retry / alert）.
                let liveGen = self.localAppState?.roomNavigationGeneration ?? 0
                guard liveGen == entryGen else {
                    os_log(.debug,
                           "RealHomeViewModel.onCreateTap: stale error (entryGen=%{public}d, currentGen=%{public}d); skip errorPresenter",
                           entryGen, liveGen)
                    return
                }
                os_log(.error, "RealHomeViewModel.onCreateTap CreateRoomUseCase error: %{public}@",
                       String(describing: error))
                presenter?.present(error)
            }
        }
    }

    public override func onJoinTap() {
        os_log(.debug, "RealHomeViewModel.onJoinTap")
        self.showJoinModal = true
    }

    // Story 37.7 codex round 2 [P2] fix：每次 .flying 用新 `UUID()` —— 同 emoji 连点
    // （如 Feed → 🍥 → 🍥）也保证 AnimationState Equatable 不等，HomeView onChange 重放动画.
    public override func onFeedTap() {
        os_log(.debug, "RealHomeViewModel.onFeedTap (Story 14.x will wire WS pet.state.changed)")
        self.interactionAnimation = .flying(emoji: "🍥", id: UUID())
    }

    public override func onPetTap() {
        os_log(.debug, "RealHomeViewModel.onPetTap")
        self.interactionAnimation = .flying(emoji: "💕", id: UUID())
    }

    public override func onPlayTap() {
        os_log(.debug, "RealHomeViewModel.onPlayTap")
        self.interactionAnimation = .flying(emoji: "⭐", id: UUID())
    }

    /// Story 37.12: JoinRoomModal "确定加入" 按钮 trigger.
    /// round 1 P1 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`
    /// 预防性应用：override 必须**本地 mutate** state 让 UI 立刻反馈，不能只 log.
    /// 行为与 MockHomeViewModel.onJoinRoomConfirm 同语义：
    ///   - 写 showJoinModal = false（关 sheet）
    ///   - 调 appState?.setCurrentRoomId(roomId)（让 sink 派生 RoomScaffoldView /
    ///     FriendsView.currentRoomId 等订阅了 currentRoomId 的兄弟 ViewModel 也同步）
    /// **关键**：通过 appState 入口而非直接写 self —— 与 RealFriendsViewModel.onJoinFriendTap 同
    /// 精神（Story 37.10 落地）；showJoinModal 是 Home 域 ViewModel-only state（关 sheet 不影响兄弟 sink）;
    /// currentRoomId 必须走 appState 入口（兄弟 ViewModel 订阅 appState.$currentRoomId）.
    /// **mutation 顺序**：先 showJoinModal = false（关 sheet），后 appState?.setCurrentRoomId(roomId)
    /// （写 AppState 让兄弟 sink 派生）—— 避免 sheet 还在但底层 view 已切走的视觉错乱（不可反序）.
    /// Story 12.7（节点 4 后）落地 JoinRoomUseCase 后改为：
    ///   1) 调 JoinRoomUseCase.execute(roomId:)
    ///   2) 成功后 server 推送 WS room.snapshot → setCurrentRoomId 由 server 端权威态写入
    public override func onJoinRoomConfirm(roomId: String) {
        // Story 12.7 AC5: mutation 顺序锁定（lesson 2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md）：
        //   1) **先** showJoinModal = false（关 sheet —— 立即视觉反馈让 modal 退场）
        //   2) **后** 起 Task 调 JoinRoomUseCase（异步路径在 sheet 关闭后才把 appState.currentRoomId 写入）
        // 不可反序：若先调 UseCase（成功后才关 sheet），HomeContainerView 已切到 RoomView 但 modal 还在最上层 → 视觉错乱.
        showJoinModal = false

        // 删除老占位行为里 `localAppState?.setCurrentRoomId(roomId)` 那行（避免 UseCase + 直写双触发 sink）.
        guard let useCase = self.joinRoomUseCase else {
            os_log(
                .debug,
                "RealHomeViewModel.onJoinRoomConfirm %{public}@ (no JoinRoomUseCase wired; fallback: write appState directly)",
                roomId
            )
            // fallback: 老 mock 行为兜底（让 RootView 老 wire / UITest 走 onJoinRoomConfirm 也能切到 inRoom）.
            localAppState?.setCurrentRoomId(roomId)
            return
        }
        let presenter = self.localErrorPresenter
        // fix-review round 10 P2 (Story 12.7): catch-path stale guard 升级到 generation token.
        // r9 旧 `currentRoomId` equality guard 会被 ABA cycle 骗（join A in-flight → join B → leave
        // 回 idle → A error 迟到 → liveRoomId == nil == entryRoomId → stale alert 弹在 idle 上）.
        // r10 升级到 `appState.roomNavigationGeneration` —— 单调计数器，不会重合.
        // 详见 docs/lessons/2026-05-11-room-navigation-generation-token-not-room-id-equality.md.
        let entryGen = self.localAppState?.roomNavigationGeneration ?? 0
        let refreshHome = self.refreshHomeOnStaleNavigation
        Task { @MainActor [weak self] in
            guard let self else { return }
            do {
                try await useCase.execute(roomId: roomId)
                // 成功 → no-op：UseCase 已写 appState.currentRoomId.
            } catch is RoomNavigationStaleError {
                // r14 [P1] fix（codex review）：UseCase 检测到 navigation race（server 已让用户加入 room 但
                // client UI 因 stale guard 没写 currentRoomId）→ silent skip + 触发 home refresh.
                os_log(.info,
                       "RealHomeViewModel.onJoinRoomConfirm: caught RoomNavigationStaleError; trigger home refresh to reconcile authoritative state")
                refreshHome?()
            } catch {
                // r10 P2 stale guard: generation mismatch 则静默 skip + log debug，不调 ErrorPresenter.
                let liveGen = self.localAppState?.roomNavigationGeneration ?? 0
                guard liveGen == entryGen else {
                    os_log(.debug,
                           "RealHomeViewModel.onJoinRoomConfirm: stale error (entryGen=%{public}d, currentGen=%{public}d); skip errorPresenter to avoid overlay on unrelated navigation cycle",
                           entryGen, liveGen)
                    return
                }
                // r8 P2 lesson 2026-05-11-business-error-fallback-must-forward-original.md：
                // catch 内对 business 做 case-by-case 文案 mapping；unrecognized code 必须
                // **forward 原 error**（保留 server message + requestId），**不**能合成新的
                // APIError.business(message: "", requestId: "") —— 那会让 ErrorPresenter
                // 默认 mapper 走 fallback `"操作失败，请稍后重试"`，丢失 server 解释 & telemetry。
                if case let APIError.business(code, _, _) = error {
                    // 业务错误码 case-by-case 文案（spec AC2 + V1 §10.4）：
                    let message: String? = {
                        switch code {
                        case 6001: return "房间不存在或已被解散"
                        case 6002: return "房间已满（4/4）"
                        case 6003: return "你已经在房间里了"
                        case 6005: return "房间已关闭"
                        case 1002: return "房间号格式不合法"
                        default: return nil
                        }
                    }()
                    if let message {
                        presenter?.presentAlert(title: "提示", message: message)
                    } else {
                        // 透传**原** error 给 ErrorPresenter 默认 mapper（如 1009 → retry）.
                        // 不 rewrap：原 APIError.business 已含 server message + requestId,
                        // AppErrorMapper.localizedMessage 在未知 code 时 fallback 用 server message.
                        presenter?.present(error)
                    }
                } else {
                    os_log(.error, "RealHomeViewModel.onJoinRoomConfirm JoinRoomUseCase error: %{public}@",
                           String(describing: error))
                    presenter?.present(error)
                }
            }
        }
    }

    // Story 37.7 codex round 4 [P3] fix：移除老 round 3 override applyHomeData(_:).
    // 老路径仅在 hydrate 入口派生 greeting → reset() 把 currentPet 置 nil 不经 applyHomeData →
    // greeting 残留旧 pet 名. 改为 subscribeGreeting(to:) 订阅 currentPet 任何变化（含 reset → nil）,
    // applyHomeData 入口的派生工作已被 sink 自动覆盖（applyHomeData 内 super 写 currentPet → sink 触发）.

    /// Story 21.3 AC6: 用户点击"开宝箱"按钮的真实路径.
    /// 流程:
    ///   1. guard !isOpening（重入防御层 1；与 button .disabled 形成双层防御）.
    ///   2. guard openChestUseCase != nil（fallback: log + return，与 onCreateTap nil fallback 同精神 ——
    ///      开箱事务涉及步数扣减 + 抽奖随机，没有合理的本地占位，所以 fallback 是 noop 而非 silent mutate）.
    ///   3. isOpening = true（同步段，main actor）.
    ///   4. 起 Task:
    ///      - Story 21.5 AC2: 起 syncHintTask（sleep 2s → 若 execute 仍未返回则 isSyncingSteps = true）.
    ///      - do { snapshot = try await useCase.execute() → self.pendingReward = snapshot }
    ///        catch { ErrorPresenter 业务错误码 case-by-case 文案 / 其他错误透传 mapper }.
    ///      - defer { isOpening = false + syncHintTask.cancel() + isSyncingSteps = false
    ///                （必恢复，让按钮重新可点 + 隐藏 "同步步数中…"）}.
    ///   注：execute() 内 Step 0 已 await stepSyncTriggerService.triggerManual()（21.3 落地编排；
    ///      21.5 DI 让该 service 非 nil）—— sync 失败被 triggerManual 静默吞（非 async throws）,
    ///      不阻塞后续 openChest（AC3 不变量；本 ViewModel 不需新增兜底）.
    ///
    /// 业务错误码文案策略（V1 §7.2 错误码表 + spec AC 行 3083）:
    ///   - 4002 宝箱未解锁    → alert "宝箱未解锁"
    ///   - 3002 步数不足      → alert "步数不足，再走走吧"
    ///   - 4001 宝箱不存在    → alert "宝箱数据异常，请重启 App"
    ///   - 1005 操作过于频繁  → alert "操作过于频繁，请稍候"
    ///   - 1002 参数错误      → alert "请求参数错误（idempotencyKey 不合法）"
    ///   - 1009 服务繁忙      → present(error) 透传给默认 mapper（→ RetryView 让用户重试）
    ///   - 未知业务码         → present(error) 透传（保留 server message + requestId）
    ///   - 非 business error → present(error) 透传给默认 mapper
    ///
    /// 与 onJoinRoomConfirm 6001-6005 mapper 同精神（lesson `2026-05-11-business-error-fallback-must-forward-original.md`）.
    public override func onChestOpenTap() {
        guard !isOpening else {
            // 重入防御层 1：UI .disabled 应已挡住；此处 guard 是兜底（lesson 防御性编程 + 防 SwiftUI tap rapid-fire bug）.
            os_log(.debug, "RealHomeViewModel.onChestOpenTap: reentry blocked (isOpening already true)")
            return
        }
        guard let useCase = self.openChestUseCase else {
            // useCase nil fallback：保留 UITEST_SKIP_GUEST_LOGIN=1 启动模式 / 老 wire 路径下点开箱按钮不 crash.
            // 与 onCreateTap useCase nil fallback 同精神（log + return；本路径不能 fallback 直写 AppState
            // 因为开箱事务涉及步数扣减 + 抽奖随机，没有合理的本地占位）.
            os_log(.debug, "RealHomeViewModel.onChestOpenTap: no OpenChestUseCase wired (fallback: log + noop)")
            return
        }
        let presenter = self.localErrorPresenter
        self.isOpening = true
        Task { @MainActor [weak self] in
            // Story 21.5 AC2: 起 2s 延迟 task —— execute()（内含 Step 0 triggerManual sync）
            // 超 2s 仍未返回 → 显 "同步步数中…"（钦定折中：以 execute 整体为基准，不改 21.3 冻结的
            // execute() 内部加回调；spec Dev Notes "实装折中" 钦定 + 防过度工程）.
            let syncHintTask = Task { @MainActor [weak self] in
                try? await Task.sleep(nanoseconds: 2_000_000_000)
                guard !Task.isCancelled else { return }
                self?.isSyncingSteps = true
            }
            defer {
                // 必恢复 isOpening = false（成功 / 失败 / cancel 都走此 defer）.
                self?.isOpening = false
                // Story 21.5 AC2 / AC3: execute() 返回（成功 / 失败 / catch）后必复位 ——
                // syncHintTask.cancel() 防 execute 在 2s 内返回时延迟 task 仍 set true
                // （cancel 后延迟 task 内 `guard !Task.isCancelled` 短路）；isSyncingSteps = false
                // 走 defer 与 isOpening 同精神保证三路径（成功 / 抛 APIError / 抛非 APIError）都复位.
                syncHintTask.cancel()
                self?.isSyncingSteps = false
            }
            guard let self else { return }
            do {
                let snapshot = try await useCase.execute()
                // 成功 → 写 transient pendingReward 字段（Story 21.4 RewardPopupView 通过 .sheet(item:) 订阅触发）.
                self.pendingReward = snapshot
                // 注：AppState.currentChest + currentStepAccount 已由 UseCase 内部写入；
                //    ChestTimerDriver 通过 sink 自动 react；ChestCardView 自动重新渲染 counting 态 + nextChest 倒计时.
            } catch let error as APIError {
                // APIError 三层映射：business code case-by-case 文案；其他 case 透传给默认 mapper.
                if case let .business(code, _, _) = error {
                    let alert: (title: String, message: String)? = {
                        switch code {
                        case 4002: return ("提示", "宝箱未解锁")
                        case 3002: return ("提示", "步数不足，再走走吧")
                        case 4001: return ("提示", "宝箱数据异常，请重启 App")
                        case 1005: return ("提示", "操作过于频繁，请稍候")
                        case 1002: return ("提示", "请求参数错误（idempotencyKey 不合法）")
                        case 1009: return nil   // 透传给默认 mapper → RetryView 让用户重试
                        default:   return nil   // 未知业务码透传给默认 mapper（保留 server message + requestId）
                        }
                    }()
                    if let alert {
                        presenter?.presentAlert(title: alert.title, message: alert.message)
                    } else {
                        // 透传**原** error 给 ErrorPresenter 默认 mapper（保留 server message + requestId）.
                        // 不 rewrap（lesson `2026-05-11-business-error-fallback-must-forward-original.md`）.
                        presenter?.present(error)
                    }
                } else {
                    // 非 business 的 APIError（network / decoding / unauthorized / missingCredentials） → 透传.
                    os_log(.error, "RealHomeViewModel.onChestOpenTap OpenChestUseCase APIError: %{public}@",
                           String(describing: error))
                    presenter?.present(error)
                }
            } catch {
                // 非 APIError 的兜底（UseCase 内部应只抛 APIError；防御性兜底）.
                os_log(.error, "RealHomeViewModel.onChestOpenTap OpenChestUseCase non-APIError: %{public}@",
                       String(describing: error))
                presenter?.present(error)
            }
        }
    }
}
