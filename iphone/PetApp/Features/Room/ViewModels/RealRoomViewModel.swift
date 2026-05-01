// RealRoomViewModel.swift
// Story 37.8 AC2: RoomViewModel 生产实装子类（构造注入 AppState；override 2 个 abstract method 占位 stub）.
//
// 范围（本 story 占位；Story 12.1 / 12.7 / 12.2-12.6 等填充真实 WS / UseCase 调用）：
//   - 构造注入 AppState（按 ADR-0010 §3.1 ViewModel 注入规则）+ parameterless init() 走 bind(appState:)
//   - override onLeaveTap / onCopyTap 为占位行为：
//     · onLeaveTap: 调 appState.setCurrentRoomId(nil) 让 HomeContainerView 互斥状态机切回 idle
//       （依赖 Story 37.3 数据流；不调 LeaveRoomUseCase 真实 — Story 12.7 落地）
//     · onCopyTap: print log（实际复制行为靠 RoomScaffoldView 内 SwiftUI UIPasteboard 调用 + @State 1.2s feedback）
//
// **不**调用任何 UseCase / Repository / APIClient（Epic 37 红线：UI Scaffold 数据完全 mock）.
// **不**订阅 WS / appState.$currentRoomId（Story 12.1 落地；本 story RealRoomViewModel 仅占位骨架）.
//
// Story 37.8 设计（与 RealHomeViewModel Story 37.7 round 1 P1 fix 同模式）：
//   parameterless `init()` 路径让 RootView `@StateObject` 老模式可用 —— AppState 也是同级 @StateObject,
//   不能在属性初始化器内交叉引用（编译期不允许 self 提前求值）；AppState 通过 `bind(appState:)` 异步注入.
// 派生 state 用 sink 路径而非一次性 hydrate（与 RealHomeViewModel codex round 4 [P3] lesson 同精神）：
//   roomCodeForCopy / hostCatName 订阅 appState.$currentRoomId / appState.$currentPet，
//   reset 路径（appState.reset() 把 currentRoomId 置 nil）也能即时反映到字段（不残留旧值）.
//
// round 1 P2 fix（codex review）：
//   两条 init 路径都用 RoomScaffoldDefaults seed 起始 members / userIsHost / hostCatName / roomCodeForCopy；
//   sink 路径作为 override（currentRoomId 来 → 派生 roomCodeForCopy；currentPet 来 → 派生 hostCatName），
//   不让 RoomScaffoldView 在 Real path 渲染空房间（4 个 mock 占位先在；Story 12.1 接 WS 后被 snapshot 覆盖）.

import Foundation
import Combine
import os.log

@MainActor
public final class RealRoomViewModel: RoomViewModel {
    /// 构造注入 AppState（与 RealHomeViewModel Story 37.7 round 1 P1 fix 同模式）.
    /// `bind(appState:)` 路径让 RootView `@StateObject` 老模式可用 — AppState 也是同级 @StateObject,
    /// 不能在属性初始化器内交叉引用（编译期不允许 self 提前求值）.
    private var appState: AppState?

    /// Story 37.8（同 RealHomeViewModel codex round 4 [P3] lesson 精神）：派生 state 必须订阅
    /// publisher，而不是一次性 hydrate；reset 路径（appState.reset() 把 currentRoomId 置 nil）才能
    /// 即时反映到 roomCodeForCopy / hostCatName。两个 sink 句柄；hookup 时机：
    ///   - init(appState:) 路径：构造完成后立即订阅
    ///   - bind(appState:) 路径：override 内首次 bind 时订阅（防多次 .task 重订阅）.
    private var roomCodeSubscription: AnyCancellable?
    private var hostCatNameSubscription: AnyCancellable?

    /// parameterless init —— RootView `@StateObject` 老模式可用; AppState 通过 bind 异步注入.
    /// 不写 `override`：基类没有显式 no-arg init（Swift 通过默认参数合成无参调用，不形成 override 关系）.
    /// round 1 P2 fix：seed `members` / `userIsHost` / `hostCatName` / `roomCodeForCopy` 全部走
    /// RoomScaffoldDefaults，让 UITEST_FORCE_IN_ROOM / 手动 debug mutation / Story 37.12 后 JoinRoomModal
    /// 落地等任何走 in-room state 的 Real path 渲染时立刻有 4 个 mock member 占位，不再渲染空房间.
    public override init() {
        super.init()
        self.appState = nil
        // 视觉初值（hydrate 前 placeholder）；bind(appState:) 后 sink 派生覆盖 roomCodeForCopy / hostCatName.
        self.roomCodeForCopy = RoomScaffoldDefaults.roomCodeForCopy
        self.hostCatName = RoomScaffoldDefaults.hostCatName
        self.members = RoomScaffoldDefaults.members
        self.userIsHost = RoomScaffoldDefaults.userIsHost
    }

    public init(appState: AppState) {
        super.init()
        self.appState = appState
        // round 1 P2 fix：先 seed scaffold defaults（让 sink 还没派发前 RoomScaffoldView 有数据可渲染）.
        self.roomCodeForCopy = RoomScaffoldDefaults.roomCodeForCopy
        self.hostCatName = RoomScaffoldDefaults.hostCatName
        self.members = RoomScaffoldDefaults.members
        self.userIsHost = RoomScaffoldDefaults.userIsHost
        // 构造路径已注入 AppState；立即订阅 currentRoomId / currentPet 变化派生 roomCodeForCopy / hostCatName,
        // 一旦 publisher 同步发首值即覆盖上面的 default seed（reset 路径置 nil 时会 fallback 回 default）.
        subscribeRoomCode(to: appState)
        subscribeHostCatName(to: appState)
        // members / userIsHost 在 Story 12.1 接 WS room.snapshot 后被覆盖；当前保留 RoomScaffoldDefaults seed.
    }

    /// AppState 异步注入入口（与 HomeViewModel.bind(appState:) 同模式 + RealHomeViewModel.bind 双路 sink）.
    public func bind(appState: AppState) {
        let alreadySubscribed = roomCodeSubscription != nil
        self.appState = appState
        guard !alreadySubscribed else { return }
        subscribeRoomCode(to: appState)
        subscribeHostCatName(to: appState)
    }

    /// 订阅 appState.$currentRoomId —— hydrate / reset / 单独 mutate 都派生 roomCodeForCopy.
    /// nil → fallback 到 RoomScaffoldDefaults.roomCodeForCopy 占位（避免 in-room scaffold 显示空房间号）；
    /// non-nil → 直接用 roomId 值（Story 12.1 后接 WS room.snapshot 时 server 返回的房间号会
    /// 写入 currentRoomId，本期 mock 同样读 currentRoomId 派生展示用号码）.
    private func subscribeRoomCode(to appState: AppState) {
        roomCodeSubscription = appState.$currentRoomId
            .sink { [weak self] roomId in
                guard let self else { return }
                self.roomCodeForCopy = roomId ?? RoomScaffoldDefaults.roomCodeForCopy
            }
    }

    /// 订阅 appState.$currentPet —— pet 名字派生 hostCatName.
    /// pet 有名字 → 用 name；pet=nil → fallback 到 RoomScaffoldDefaults.hostCatName 占位.
    private func subscribeHostCatName(to appState: AppState) {
        hostCatNameSubscription = appState.$currentPet
            .sink { [weak self] pet in
                guard let self else { return }
                if let petName = pet?.name, !petName.isEmpty {
                    self.hostCatName = petName
                } else {
                    self.hostCatName = RoomScaffoldDefaults.hostCatName
                }
            }
    }

    // MARK: - override abstract methods（本 story 占位；Story 12.7 实装真实 LeaveRoomUseCase）

    public override func onLeaveTap() {
        os_log(.debug, "RealRoomViewModel.onLeaveTap (Story 12.7 will wire LeaveRoomUseCase)")
        // 节点 1 占位：直接置 currentRoomId = nil 让 HomeContainerView 切回 idle
        // （依赖 Story 37.3 互斥状态机数据流；不调 server LeaveRoom API — Story 12.7 落地）.
        self.appState?.setCurrentRoomId(nil)
    }

    public override func onCopyTap() {
        os_log(.debug, "RealRoomViewModel.onCopyTap")
        // 实际 UIPasteboard 复制 + 1.2s 视觉反馈由 RoomScaffoldView 内 SwiftUI @State 持有 + 调用此方法即触发.
        // 本 ViewModel 层仅记录 / 透传；不直接操作 UIKit（Epic 37 红线：ViewModel 层不依赖 UIKit）.
    }
}
