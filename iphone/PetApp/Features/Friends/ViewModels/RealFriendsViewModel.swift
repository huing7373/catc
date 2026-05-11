// RealFriendsViewModel.swift
// Story 37.10 AC2: FriendsViewModel 生产实装子类（构造注入 AppState；override 2 个 abstract method 占位 mutate）.
//
// 范围（本 story 占位；Story 12.7 / 37.12 等填充真实 UseCase 调用）：
//   - 构造注入 AppState（按 ADR-0010 §3.1 ViewModel 注入规则）+ parameterless init() 走 bind(appState:)
//   - override onInviteFriendTap / onJoinFriendTap：本地 mutate currentRoomId / lastToastMessage（占位）
//     按 Story 37.9 round 1 P1 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`：
//     Real 子类 override 必须实装"本 story 范围内能让 UI 视觉工作的最小 placeholder 行为"，禁止只 log.
//
// **不**调用任何 UseCase / Repository / APIClient（Epic 37 红线：UI Scaffold 数据完全 mock）.
// **不**订阅真实好友列表 server 接口（后续 epic 落地；本 story RealFriendsViewModel friends 走 ScaffoldDefaults seed）.
//
// Story 37.7 / 37.8 / 37.9 沉淀 lesson 预防性应用（**不重蹈覆辙**）：
//   - lesson `2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md`：
//     RootView `@StateObject friendsViewModel` 用 `RealFriendsViewModel()` 而非基类 `FriendsViewModel()` —
//     基类 onInviteFriendTap / onJoinFriendTap 是 fatalError 占位，用户点按钮即 crash.
//   - lesson `2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md`：
//     两条 init 路径都走 `FriendsScaffoldDefaults` seed —— 让 launch 后 / hydrate 前 / reset 后任何
//     Real path 都立刻有 mock friends 占位（不让 FriendsScaffoldView 渲染空好友列表）.
//   - lesson `2026-04-30-published-derived-state-needs-publisher-subscription.md`：
//     派生 state 用 sink 路径而非一次性 hydrate —— currentRoomId 订阅 appState.$currentRoomId；
//     reset 路径（appState.reset() 把 currentRoomId 置 nil）也能即时反映到字段（不残留旧值）.
//   - lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`（**关键** —— Story 37.9 第一次复犯）：
//     onInviteFriendTap / onJoinFriendTap override **必须本地 mutate state**（与 Mock 同语义），
//     不能只 log（否则 production 路径下用户点按钮 no-op）.

import Foundation
import Combine
import os.log

@MainActor
public final class RealFriendsViewModel: FriendsViewModel {
    /// 构造注入 AppState（与 RealHomeViewModel / RealRoomViewModel / RealWardrobeViewModel 同模式）.
    private var appState: AppState?

    /// 派生 state sink 句柄（防多次 bind 重订阅 + 持有 cancellable 让 sink 存活）.
    private var currentRoomIdSubscription: AnyCancellable?

    /// Story 12.7 AC7: JoinRoomUseCase 注入（默认 nil；caller=RootView 通过 bind() 注入 container.makeJoinRoomUseCase）.
    private var joinRoomUseCase: JoinRoomUseCaseProtocol?

    /// Story 12.7 AC7: ErrorPresenter 注入（weak 引用避免循环）；caller=RootView 注入 container.errorPresenter.
    private weak var errorPresenter: ErrorPresenter?

    /// parameterless init —— RootView `@StateObject` 老模式可用; AppState 通过 bind 异步注入.
    /// 按 Story 37.8 / 37.9 round 1 P2 lesson 预防性应用：seed `friends` / `selectedTab` 全部走 FriendsScaffoldDefaults,
    /// 让 launch / hydrate 前 / reset 后任何走 Real path 都立刻有 mock 好友列表占位.
    /// 注：必写 `override` —— 基类 FriendsViewModel 有显式 `public init() {}`（与 RoomViewModel / WardrobeViewModel 同模式）.
    public override init() {
        super.init()
        self.appState = nil
        self.friends = FriendsScaffoldDefaults.friends
        self.selectedTab = FriendsScaffoldDefaults.selectedTab
        self.currentRoomId = FriendsScaffoldDefaults.currentRoomId  // nil; bind 后 sink 派生
        self.lastToastMessage = nil
    }

    public init(appState: AppState) {
        super.init()
        self.appState = appState
        // round 1 P2 fix：先 seed scaffold defaults（让 sink 还没派发前 FriendsScaffoldView 有数据可渲染）.
        self.friends = FriendsScaffoldDefaults.friends
        self.selectedTab = FriendsScaffoldDefaults.selectedTab
        self.currentRoomId = FriendsScaffoldDefaults.currentRoomId
        self.lastToastMessage = nil
        // 构造路径已注入 AppState；立即订阅 currentRoomId 派生.
        subscribeCurrentRoomId(to: appState)
    }

    /// AppState 异步注入入口（与 RealHomeViewModel / RealRoomViewModel.bind / RealWardrobeViewModel.bind 同模式）.
    /// Story 12.7 AC7 扩展：可选注入 joinRoomUseCase + errorPresenter（默认 nil 让既有 caller / 测试 / Preview 不破）.
    public func bind(
        appState: AppState,
        joinRoomUseCase: JoinRoomUseCaseProtocol? = nil,
        errorPresenter: ErrorPresenter? = nil
    ) {
        let alreadySubscribed = currentRoomIdSubscription != nil
        self.appState = appState
        if let useCase = joinRoomUseCase {
            self.joinRoomUseCase = useCase
        }
        if let presenter = errorPresenter {
            self.errorPresenter = presenter
        }
        guard !alreadySubscribed else { return }
        subscribeCurrentRoomId(to: appState)
    }

    /// 订阅 appState.$currentRoomId —— hydrate / reset / 单独 mutate 都派生 currentRoomId.
    /// **关键**：currentRoomId 派生源是合法的 —— Friends 域语义就是"我的房间"（本地用户自己的房间）,
    /// appState.currentRoomId 是真理源（与 Story 37.9 catName 派生自 currentPet 同合法理由；
    /// 与 Story 37.8 RoomViewModel.hostCatName 反例不冲突 —— 那是"看别人房间"语境）.
    private func subscribeCurrentRoomId(to appState: AppState) {
        currentRoomIdSubscription = appState.$currentRoomId
            .sink { [weak self] roomId in
                guard let self else { return }
                self.currentRoomId = roomId
            }
    }

    // MARK: - override abstract methods（本 story 占位 mutate；Story 12.7 / 37.12 实装真实 UseCase）

    /// round 1 P1 lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md` 预防性应用：
    /// override 必须**本地 mutate** state 让 UI 立刻反馈，不能只 log（否则 production 走 RealFriendsViewModel 时
    /// 邀请按钮 no-op，主交互失效；单测/Preview 走 Mock 路径覆盖不到本 bug）.
    ///
    /// 行为与 MockFriendsViewModel.onInviteFriendTap 同语义（currentRoomId nil → 设占位串 + toast；非 nil → 仅 toast）.
    /// 让 Mock 单测 / Preview 与 Real 生产观感一致.
    /// Story 12.7 落地 CreateRoomUseCase 后改为：
    ///   1) 调 CreateRoomUseCase（若 currentRoomId nil）/ WS invitation
    ///   2) 成功后通过 appState.setCurrentRoomId(...) 写入
    ///   3) 通过 sink 派生 currentRoomId 字段（不再本地直接写）
    public override func onInviteFriendTap(friend: Friend) {
        os_log(.debug, "RealFriendsViewModel.onInviteFriendTap (Story 12.7 will wire CreateRoomUseCase) %{public}@", friend.id)
        if currentRoomId == nil {
            // 占位创建队伍：通过 appState.setCurrentRoomId 走规范入口（让 sink 派生 currentRoomId，与 Story 12.7 落地后路径一致）.
            // **不**直接写 self.currentRoomId —— Real 路径必须走 appState 入口,
            //   让 RealRoomViewModel / RealWardrobeViewModel 等订阅了 currentRoomId 的兄弟 ViewModel 也同步.
            appState?.setCurrentRoomId("1234567")
            lastToastMessage = "已创建队伍并邀请 \(friend.name)"
        } else {
            lastToastMessage = "已邀请 \(friend.name) 加入房间 \(currentRoomId ?? "?")"
        }
    }

    /// round 1 P1 lesson 预防性应用：override 必须本地 mutate state.
    /// 行为与 MockFriendsViewModel.onJoinFriendTap 同语义（mutate currentRoomId 到 friend.currentRoomId）.
    /// Story 37.12 落地 JoinRoomUseCase 后改为：
    ///   1) 调 JoinRoomUseCase(roomId: friend.currentRoomId)（epic AC line 4859 钦定 FriendsScreen 直接调，不弹 modal）
    ///   2) 成功后 server 真实加入 + WS room.snapshot 推送
    ///   3) appState.setCurrentRoomId 由 server 端权威态写入 + sink 派生
    public override func onJoinFriendTap(friend: Friend) {
        // Story 12.7 AC7: 调 JoinRoomUseCase（直接 join，不弹 modal —— spec line 4859 钦定 FriendsScreen 直接调）.
        // 防御性兜底：friend.currentRoomId nil → toast "好友不在房间中"（理论不应发生；UI 仅在 friend.currentRoomId != nil 时显示"加入"按钮）.
        guard let targetRoomId = friend.currentRoomId, !targetRoomId.isEmpty else {
            os_log(.debug, "RealFriendsViewModel.onJoinFriendTap friend.currentRoomId nil (defensive guard)")
            lastToastMessage = "好友不在房间中"
            return
        }
        guard let useCase = self.joinRoomUseCase else {
            // fallback: 老 mock 行为兜底（让 RootView 老 wire / UITest 走 onJoinFriendTap 也能切到 inRoom）.
            os_log(.debug, "RealFriendsViewModel.onJoinFriendTap (no JoinRoomUseCase wired; fallback) %{public}@", friend.id)
            appState?.setCurrentRoomId(targetRoomId)
            lastToastMessage = "加入 \(friend.name) 的房间 \(targetRoomId)"
            return
        }
        let presenter = self.errorPresenter
        Task { @MainActor [weak self] in
            guard self != nil else { return }
            do {
                try await useCase.execute(roomId: targetRoomId)
                // 成功 → no-op（UseCase 已写 appState.currentRoomId → HomeContainerView 自动切到 RoomView）.
            } catch {
                // r8 P2 lesson 2026-05-11-business-error-fallback-must-forward-original.md：
                // unrecognized business code 必须 forward 原 error（保留 server message +
                // requestId），不能合成空 APIError.business（会让 AppErrorMapper fallback 到
                // generic "操作失败，请稍后重试" 并丢失 server 解释）. 与 RealHomeViewModel
                // 同精神。
                if case let APIError.business(code, _, _) = error {
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
                        // 透传**原** error（不 rewrap），让 AppErrorMapper 拿到 server message.
                        presenter?.present(error)
                    }
                } else {
                    os_log(.error, "RealFriendsViewModel.onJoinFriendTap JoinRoomUseCase error: %{public}@",
                           String(describing: error))
                    presenter?.present(error)
                }
            }
        }
    }
}
