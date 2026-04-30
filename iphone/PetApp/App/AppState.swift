// AppState.swift
// Story 37.4：iPhone 全局 domain state 单 source of truth（ADR-0010 §3.1 / §3.2 / §3.3 / §3.7）.
//
// 落地依据：
//   - ADR-0010 §3.1：AppState 类型与生命周期（@MainActor + final + ObservableObject + @Published）.
//   - ADR-0010 §3.2：白名单 7 字段（节点 1 阶段相关 5 字段 + 节点 6/8/9 占位 3 字段）.
//   - ADR-0010 §3.3：hydrate 入口 applyHomeData(_:)（启动 / 自动登录后 + 后续 WS / REST mutation 入口）.
//   - ADR-0010 §3.7：Reset 流程 reset()（与 SessionStore.clear() 同精神）.
//
// 注入规则（ADR-0010 §3.1 ADR 级硬规则）：
//   - View 层：通过 RootView `.environmentObject(appState)` 注入；
//     子视图（如 HomeView / HomeContainerView）用 `@EnvironmentObject var appState: AppState` 读.
//   - ViewModel 层：**只允许构造注入**（init 默认参数 / bind(appState:)）；
//     **禁止** ViewModel 内部用 `@EnvironmentObject`（ViewModel 不是 SwiftUI View，没有 environment 依赖注入）.
//   - 测试时通过 extension testing helper 构造已 hydrate / 已 reset 实例
//     （AppState 是 final class 不可继承 → 不走 MockAppState 子类路径；与 SessionStore 同模式）.
//
// 与 ADR-0009 / SessionStore 的边界（ADR-0010 §3.2 表格 + §3.4）：
//   - 当前 Tab → AppCoordinator.currentTab（与 presentedSheet 同级；不进 AppState）.
//   - Sheet 是否打开 / Loading / WS 连接态 / 表单输入 / 倒计时秒数 → ViewModel 或 SwiftUI @State.
//   - SessionStore（认证态：登录/登出 + access token 派生）与 AppState 并行边界；
//     ResetIdentityViewModel.tap() 成功路径调 sessionStore?.clear() + appState?.reset() 双调.
//
// import 备注（继承 Story 2.2 lesson 2026-04-25-swift-explicit-import-combine.md）：
// `ObservableObject` / `@Published` 来自 Combine，不能依赖 SwiftUI transitive import.

import Foundation
import Combine

/// AppState：全局 domain state 单 source of truth（ADR-0010 §3.1 / §3.2）.
///
/// 范围（白名单，节点 1 阶段相关字段就位；其余节点占位）：
///   - currentUser / currentPet / currentStepAccount / currentChest / currentRoomId（节点 2 起）
///   - currentInventory（节点 8 起）/ currentEquips（节点 9 起）/ emojiCatalog（节点 6 起）
///
/// 不含（ADR-0010 §3.2 表格）：
///   - 当前 Tab → AppCoordinator.currentTab
///   - Sheet 是否打开 / Loading / WS 连接态 / 表单输入 / 倒计时秒数 → ViewModel 或 SwiftUI @State
///
/// 类型选择（Story 37.4 Dev Notes "类型选择"）：
///   - 节点 1 阶段直接复用 `Home*` 类型族，避免预创建空类型签名影响测试；
///   - 后续节点接入新 epic 时如发现需要"非 Home* 派生"的领域类型再做演进（ADR-0010 §4.4 缓解策略）.
@MainActor
public final class AppState: ObservableObject {
    @Published public var currentUser: HomeUser?
    @Published public var currentPet: HomePet?
    @Published public var currentStepAccount: HomeStepAccount?
    @Published public var currentChest: HomeChest?
    @Published public var currentRoomId: String?

    /// 占位字段（节点 6 / 8 / 9 起真实使用；本 story 仅类型骨架就位 + 默认值）.
    /// 类型选择：节点 1 阶段直接复用 Home* 类型族，避免预创建空类型签名影响测试；
    /// 后续节点接入新 epic 时如发现需要"非 Home* 派生"的领域类型再做演进（ADR-0010 §4.4 缓解策略）.
    @Published public var currentInventory: [HomeEquip] = []
    @Published public var currentEquips: [HomeEquip] = []
    @Published public var emojiCatalog: [String] = []  // 节点 6 起换 EmojiConfig 类型

    public init() {}

    // MARK: - Hydrate / Mutation 入口（ADR-0010 §3.3）

    /// LoadHomeUseCase 完成后的统一 hydrate 入口（ADR-0010 §3.3 启动/自动登录后流程）.
    /// 命名 `applyHomeData` 与现有 HomeViewModel.applyHomeData(_:) 同名风格,
    /// 让 RootView bootstrap closure 替换前后语义一致（dev 阅读 git diff 时直观）.
    /// 详见 ADR-0010 §3.3 hydrate 流程伪代码 + §3.5 HomeViewModel 关键变化.
    public func applyHomeData(_ data: HomeData) {
        self.currentUser = data.user
        self.currentPet = data.pet
        self.currentStepAccount = data.stepAccount
        self.currentChest = data.chest
        self.currentRoomId = data.room.currentRoomId
    }

    /// Reset 流程（ADR-0010 §3.7）：用户主动登出 / 重置身份时清空全部 domain state.
    /// 由 ResetIdentityViewModel.tap() 成功路径调用（与 SessionStore.clear() 同精神）.
    /// **不**置默认值给 currentUser 等 optional 字段（语义清晰：未登录就是 nil）.
    public func reset() {
        self.currentUser = nil
        self.currentPet = nil
        self.currentStepAccount = nil
        self.currentChest = nil
        self.currentRoomId = nil
        self.currentInventory = []
        self.currentEquips = []
        self.emojiCatalog = []
    }

    /// 显式 setter（节点 4 后用，房间状态 mutation 入口）.
    /// 取消注释当 Story 12.7 落地 CreateRoom/JoinRoom/LeaveRoom UseCase；本 story 仅声明.
    public func setCurrentRoomId(_ roomId: String?) {
        self.currentRoomId = roomId
    }

    /// 显式 setter（节点 5 后 WS pet.state.changed 自身分支用；ADR-0010 §3.3 WS 流程）.
    /// 节点 5 才接 WS；本 story 仅声明类型契约（让 AppStateTests 可写 case），
    /// 不连真实 WS 入口.
    public func updateMyPetState(_ state: HomePetState) {
        guard let pet = currentPet else { return }
        let updated = HomePet(
            id: pet.id,
            petType: pet.petType,
            name: pet.name,
            currentState: state,
            equips: pet.equips
        )
        self.currentPet = updated
    }

    /// 显式 setter（节点 9 后 EquipUseCase / UnequipUseCase 用）.
    public func updateCurrentEquips(_ equips: [HomeEquip]) {
        self.currentEquips = equips
    }
}
