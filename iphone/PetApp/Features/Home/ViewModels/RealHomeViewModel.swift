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

import Foundation
import os.log

@MainActor
public final class RealHomeViewModel: HomeViewModel {

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
        os_log(.debug, "RealHomeViewModel.onCreateTap (Story 12.7 will wire CreateRoomUseCase)")
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
}
