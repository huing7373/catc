// RealWardrobeViewModel.swift
// Story 37.9 AC2: WardrobeViewModel 生产实装子类（构造注入 AppState；override 1 个 abstract method 占位 stub）.
//
// 范围（本 story 占位；Story 24.1 / 24.2 / 27.1 等填充真实 UseCase 调用）：
//   - 构造注入 AppState（按 ADR-0010 §3.1 ViewModel 注入规则）+ parameterless init() 走 bind(appState:)
//   - override onEquipTap：本地 toggle equipped 映射 + log（占位）；Story 27.1 后改调 EquipUseCase 写 appState.currentEquips
//     round 1 P1 fix（codex review）：本 override 必须 mutate equipped — 仅 log 会让 production "装备/已装备" 按钮 no-op
//     （lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`）
//
// **不**调用任何 UseCase / Repository / APIClient（Epic 37 红线：UI Scaffold 数据完全 mock）.
// **不**订阅真实 LoadInventoryUseCase（Story 24.2 落地；本 story RealWardrobeViewModel 仅占位骨架）.
//
// Story 37.7 / 37.8 沉淀 lesson 预防性应用（**不重蹈覆辙**）：
//   - lesson `2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md`：
//     RootView `@StateObject wardrobeViewModel` 用 `RealWardrobeViewModel()` 而非基类 `WardrobeViewModel()` —
//     基类 onEquipTap 是 fatalError 占位，用户点装备按钮即 crash.
//   - lesson `2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md`：
//     两条 init 路径都走 `WardrobeScaffoldDefaults` seed —— 让 launch 后 / hydrate 前 / reset 后任何
//     Real path 都立刻有 mock inventory 占位（不让 WardrobeScaffoldView 渲染空衣柜）.
//   - lesson `2026-04-30-published-derived-state-needs-publisher-subscription.md`：
//     派生 state 用 sink 路径而非一次性 hydrate —— catName 订阅 appState.$currentPet,
//     inventory 订阅 appState.$currentInventory；reset 路径（appState.reset() 把 currentPet / currentInventory 置空）
//     也能即时反映到字段（不残留旧值；fallback 回 WardrobeScaffoldDefaults 占位）.
//   - lesson `2026-04-30-room-host-name-must-not-derive-from-local-current-pet.md`：
//     **本 story 反向应用 lesson** —— Wardrobe 域的 catName 语义合法派生自 appState.currentPet
//     （Wardrobe 是"看自己衣柜"单一视角；与 Room 域 host 可能是别人的语境不同）.

import Foundation
import Combine
import os.log

@MainActor
public final class RealWardrobeViewModel: WardrobeViewModel {
    /// 构造注入 AppState（与 RealHomeViewModel / RealRoomViewModel 同模式）.
    private var appState: AppState?

    /// 派生 state sink 句柄（防多次 bind 重订阅 + 持有 cancellable 让 sink 存活）.
    /// 两个独立 subscription：catName ← currentPet / inventory ← currentInventory；不合并 .combineLatest（语义独立 + 让 fallback 行为独立）.
    private var catNameSubscription: AnyCancellable?
    private var inventorySubscription: AnyCancellable?

    /// parameterless init —— RootView `@StateObject` 老模式可用; AppState 通过 bind 异步注入.
    /// round 1 P2 fix（Story 37.8 lesson 预防性应用）：seed `catName` / `inventory` / `equipped` / `selectedCategory`
    /// 全部走 WardrobeScaffoldDefaults，让 launch / hydrate 前 / reset 后任何走 Real path 都立刻有 mock 占位.
    ///
    /// 注：必写 `override` —— 基类 WardrobeViewModel 有显式 `public init() {}`（与 RoomViewModel 同模式
    /// 但与 spec 注释相反；保留 override 让编译通过；与 MockWardrobeViewModel.init() 行为一致）.
    public override init() {
        super.init()
        self.appState = nil
        // 视觉初值（hydrate 前 placeholder）；bind(appState:) 后 sink 派生覆盖 catName / inventory.
        self.catName = WardrobeScaffoldDefaults.catName
        self.inventory = WardrobeScaffoldDefaults.inventory
        self.equipped = WardrobeScaffoldDefaults.equipped
        self.selectedCategory = WardrobeScaffoldDefaults.selectedCategory
        self.selectedCosmeticId = nil
    }

    public init(appState: AppState) {
        super.init()
        self.appState = appState
        // round 1 P2 fix：先 seed scaffold defaults（让 sink 还没派发前 WardrobeScaffoldView 有数据可渲染）.
        self.catName = WardrobeScaffoldDefaults.catName
        self.inventory = WardrobeScaffoldDefaults.inventory
        self.equipped = WardrobeScaffoldDefaults.equipped
        self.selectedCategory = WardrobeScaffoldDefaults.selectedCategory
        self.selectedCosmeticId = nil
        // 构造路径已注入 AppState；立即订阅 currentPet / currentInventory 派生 catName / inventory,
        // 一旦 publisher 同步发首值即覆盖上面的 default seed（reset 路径置 nil 时会 fallback 回 default）.
        subscribeCatName(to: appState)
        subscribeInventory(to: appState)
    }

    /// AppState 异步注入入口（与 RealHomeViewModel / RealRoomViewModel.bind(appState:) 同模式 + RealHomeViewModel.bind 单路 sink）.
    public func bind(appState: AppState) {
        let alreadySubscribed = catNameSubscription != nil
        self.appState = appState
        guard !alreadySubscribed else { return }
        subscribeCatName(to: appState)
        subscribeInventory(to: appState)
    }

    /// 订阅 appState.$currentPet —— hydrate / reset / 单独 mutate 都派生 catName.
    /// nil → fallback 到 WardrobeScaffoldDefaults.catName 占位（避免 reset 后 catName 残留旧值）；
    /// non-nil 且 name 非空 → 用 pet.name；否则 fallback 占位.
    /// **关键**：catName 派生源是合法的 —— Wardrobe 域语义就是"看自己衣柜"，appState.currentPet 是真理源.
    private func subscribeCatName(to appState: AppState) {
        catNameSubscription = appState.$currentPet
            .sink { [weak self] pet in
                guard let self else { return }
                if let petName = pet?.name, !petName.isEmpty {
                    self.catName = petName
                } else {
                    self.catName = WardrobeScaffoldDefaults.catName
                }
            }
    }

    /// 订阅 appState.$currentInventory —— Story 24.2 LoadInventoryUseCase 落地后会写入此字段；本期 sink 路径已 hookup,
    /// 让 Story 24.1 落地时零 edit RealWardrobeViewModel.
    /// 当前 appState.currentInventory 是 [HomeEquip]，本 story 不引入 HomeEquip → CosmeticItem mapping
    /// （Story 24.1 决定 mapping shape；本 story 仅订阅了，但 sink closure 内只让 inventory fallback 到 defaults
    /// 当前 currentInventory 为空时 —— 真有数据时 mapping 留 Story 24.1）.
    private func subscribeInventory(to appState: AppState) {
        inventorySubscription = appState.$currentInventory
            .sink { [weak self] homeEquips in
                guard let self else { return }
                // 本期占位策略（Story 24.1 落地时改 mapping）：
                //   - currentInventory 为空（启动后 / reset 后）→ 用 WardrobeScaffoldDefaults.inventory 占位
                //   - currentInventory 非空（Story 24.1 落地后才会非空）→ TODO: HomeEquip → CosmeticItem mapping
                //     当前留占位 mock（避免本 story 引入 mapping 让 Story 24.1 dev 重写）
                if homeEquips.isEmpty {
                    self.inventory = WardrobeScaffoldDefaults.inventory
                } else {
                    // 占位：保持当前 inventory 不动（Story 24.1 落地真 mapping）
                    // 行为等价于"Real 看到 hydrate 进来就用最新 mock data"；不会触发 user-visible bug
                    // 因为本 story 范围内 currentInventory 永远是 [].
                }
            }
    }

    // MARK: - override abstract methods（本 story 占位；Story 27.1 实装真实 EquipUseCase / UnequipUseCase）

    /// round 1 P1 fix（codex review 2026-04-30）：override 必须**本地 mutate** equipped 让 UI 立刻反馈,
    /// 不能只 log（否则 production 走 RealWardrobeViewModel 时"装备/已装备"按钮 + 对勾 badge + 预览区按钮 label
    /// 全部 no-op，主交互失效；单测 / Preview 走 Mock 路径覆盖不到本 bug）.
    ///
    /// 复用 Story 37.7 RealHomeViewModel.onJoinTap / onFeedTap 同模式 ——
    /// lesson `2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md`：
    ///   把 abstract method 在 Real 子类的 override 当成 "本 story 范围内的 placeholder 行为"（不只是 log）,
    ///   让交互在 production 路径下视觉上工作；server 真实写入留给后续 Story 实装时再替换.
    ///
    /// 行为与 MockWardrobeViewModel.onEquipTap 同语义（owned guard + 同 id toggle 卸下 / 不同 id 装备）,
    /// 让 Mock 单测 / Preview 与 Real 生产观感一致.
    /// Story 27.1 落地 EquipUseCase 后改为：
    ///   1) 调 EquipUseCase / UnequipUseCase
    ///   2) 成功后写 appState.currentEquips
    ///   3) 通过新 sink 派生 equipped 字段（不再本地直接写）
    public override func onEquipTap(item: CosmeticItem) {
        os_log(.debug, "RealWardrobeViewModel.onEquipTap (Story 27.1 will wire EquipUseCase) %{public}@", item.id)
        // owned check：unowned 道具点击不变 equipped（与 Mock 一致；ui_design wardrobe.jsx:27-32 toggleEquip 等价）.
        guard item.owned else { return }
        if equipped[item.category] == item.id {
            equipped[item.category] = nil  // 卸下
        } else {
            equipped[item.category] = item.id  // 装备
        }
    }
}
