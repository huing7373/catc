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

    /// Story 12.7 r10 [P2] fix（codex review）：Per-room-navigation monotonic generation token.
    /// 用于解决 currentRoomId equality 无法区分的 ABA race（detail 见 lesson
    /// `2026-05-11-room-navigation-generation-token-not-room-id-equality.md`）.
    ///
    /// 不变量：`setCurrentRoomId(_:)` 每次调用（无论新值是否等于旧值）都 +1.
    /// 用法（UseCase / ViewModel catch-path）：
    ///   - 入口 capture `let entryGen = appState.roomNavigationGeneration`
    ///   - async await 返回后 guard `appState.roomNavigationGeneration == entryGen`
    ///     → 不匹配 silent skip + log（说明期间发生过任意房间切换 cycle，stale response/error 不可应用）.
    /// **不**走 @Published —— 是 internal race-guard token，不直接显示给 UI；
    /// 让 SwiftUI 不因 generation 自增触发额外 view diff（避免不必要的 invalidation）.
    public private(set) var roomNavigationGeneration: Int = 0

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
        // Story 12.7 r10 [P2] fix: hydrate 也算一次 room navigation cycle —— 任何启动 / 重登陆
        // 后的 in-flight stale create/join/leave response 都必须被新 generation 拒绝.
        self.currentRoomId = data.room.currentRoomId
        self.roomNavigationGeneration &+= 1
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
        // Story 12.7 r10 [P2] fix: reset 也算一次 room navigation cycle —— 用户登出 / 切身份
        // 后任何 in-flight stale response 都必须被新 generation 拒绝.
        self.roomNavigationGeneration &+= 1
    }

    /// 显式 setter（节点 4 后用，房间状态 mutation 入口）.
    /// 取消注释当 Story 12.7 落地 CreateRoom/JoinRoom/LeaveRoom UseCase；本 story 仅声明.
    ///
    /// Story 12.7 r10 [P2] fix（codex review）：每次调用 `roomNavigationGeneration &+= 1`,
    /// 即使新值与旧值相等（如 leave A → re-join A —— currentRoomId 经历 A → nil → A，
    /// 但 generation A1 → A2 → A3 严格单调）.
    /// 用 `&+= 1`（wrapping overflow）保留 monotonic invariant 即使 Int.max（按当前 navigation 频率
    /// 不可达，但语义安全）.
    public func setCurrentRoomId(_ roomId: String?) {
        self.currentRoomId = roomId
        self.roomNavigationGeneration &+= 1
    }

    /// Story 8.5 AC7: 步数同步成功后写入 currentStepAccount 单字段.
    /// 由 SyncStepsUseCase.execute(_:) 在同步成功后调；不动其它 6 字段
    /// （与 applyHomeData 全字段写入区分）.
    ///
    /// 命名 `applySyncedStepAccount` 与 `applyHomeData` 同前缀（apply* 前缀表示"hydrate / mutation 入口"；
    /// 详见 ADR-0010 §3.3）；后缀 `SyncedStepAccount` 表达数据来源
    /// （**同步动作返回**，与 GET /home 全量加载区分）.
    ///
    /// **不**包装 Optional：caller SyncStepsUseCase 同步成功必有 stepAccount（V1 §6.1 响应字段必填；
    /// 不可能为 nil，schema 已冻结）.
    public func applySyncedStepAccount(_ stepAccount: HomeStepAccount) {
        self.currentStepAccount = stepAccount
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
