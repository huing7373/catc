// RealWardrobeViewModel.swift
// Story 37.9 AC2: WardrobeViewModel 生产实装子类（构造注入 AppState；override 1 个 abstract method）.
// Story 24.1: subscribeInventory 真实 inventory 派生落地（替换 37.9 占位 sink）.
//
// 范围（Story 24.1 落地点；Story 24.2 / 27.1 等继续填充）：
//   - 构造注入 AppState（按 ADR-0010 §3.1 ViewModel 注入规则）+ parameterless init() 走 bind(appState:)
//   - subscribeInventory：订阅 appState.$currentInventory（类型 [HomeEquip]），closure 内
//     `[HomeEquip] → [CosmeticItem]` 真实 mapping 写 self.inventory；空（启动 / reset）→ self.inventory = []
//     （空仓库走 Story 37.9 已实装空态 placeholder；**不**再退回 WardrobeScaffoldDefaults.inventory mock）.
//   - 上游触发归属：Story 24.2 LoadInventoryUseCase 调 GET /cosmetics/inventory 后写
//     appState.currentInventory —— 写入后零 edit 通过本 sink 反映到 Wardrobe（本 story 实装 closure
//     mapping 即完成「Story 24.2 写 → Wardrobe 显示真实装扮」闭环）；本 story **不**加 .task/.onAppear
//     触发 load（避免与 Story 24.2 UseCase 触发点冲突）.
//   - override onEquipTap：本地 toggle equipped 映射 + log（占位）；Story 27.1 后改调 EquipUseCase 写 appState.currentEquips
//     round 1 P1 fix（codex review）：本 override 必须 mutate equipped — 仅 log 会让 production "装备/已装备" 按钮 no-op
//     （lesson `2026-04-30-real-viewmodel-override-placeholder-must-mutate-state.md`）
//
// **不**调用任何 UseCase / Repository / APIClient（LoadInventoryUseCase 本体属 Story 24.2；本 story 仅 sink mapping）.
//
// HomeEquip → CosmeticItem mapping 形状（Story 24.1 AC2 钦定；mapToCosmeticItems(_:)）：
//   - id       ← HomeEquip.userCosmeticItemId（实例级唯一 id；同 cosmetic 多实例各成独立 CosmeticItem，
//                 不在 ViewModel 层去重 —— count 由 WardrobeScaffoldView filter().count 自然得出，
//                 与 37.9 既有 grid 渲染契约一致）
//   - name     ← HomeEquip.name
//   - category ← CosmeticCategory.category(forSlot: HomeEquip.slot)（V1 §6.8 slot 8 值归并到 client 5 桶；未知 slot fallback .bow 不丢实例）
//   - rarity   ← HomeEquip.rarity int → Rarity enum（{1,2,3,4} → .N/.R/.SR/.SSR；未知值 fallback .N，视觉降级不 crash）
//   - owned    ← 恒 true（inventory 全是「已拥有实例」；空仓库时数组为空，不出现 owned=false）
//   - iconEmoji← category.iconEmoji 派生占位（Story 30.x 接真实 sprite 时升级）
//
// Story 37.7 / 37.8 / 37.9 沉淀 lesson（预防性应用，**不重蹈覆辙**）：
//   - lesson `2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md`：
//     RootView `@StateObject wardrobeViewModel` 用 `RealWardrobeViewModel()` 而非基类（基类 onEquipTap fatalError）.
//   - lesson `2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md`：
//     **本 story 部分偏离此 lesson（有意决策，AC5 钦定）**：catName / equipped / selectedCategory 仍 seed
//     WardrobeScaffoldDefaults（保留 lesson 精神：hydrate 前 UI 不空白崩溃）；但 `inventory` seed 改 `[]`
//     —— 理由：本 story 后 Real 路径语义是「真实 inventory」，hydrate 前显示 mock 18 件假装扮会误导用户
//     （开箱前应显示空仓库 placeholder）；37.9 用 mock seed 是因当时无真实数据源，本 story 接通后 seed
//     应反映真实初值（空）。**非未遵守 seed lesson，是语义正确性优先的有意偏离**.
//   - lesson `2026-04-30-published-derived-state-needs-publisher-subscription.md`：
//     派生 state 用 sink 路径而非一次性 hydrate —— catName 订阅 appState.$currentPet,
//     inventory 订阅 appState.$currentInventory；reset 路径（appState.reset() 把 currentPet / currentInventory 置空）
//     即时反映（不残留旧值；inventory 空 → []，catName → WardrobeScaffoldDefaults 占位）.
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
    /// Story 24.1 调整（AC5）：catName / equipped / selectedCategory 仍 seed WardrobeScaffoldDefaults
    /// （保留 37.8 lesson 精神：hydrate 前 UI 不空白崩溃）；`inventory` seed 改 `[]`
    /// —— 本 story 后 Real 路径语义是「真实 inventory」，hydrate 前空仓库 placeholder 才是正确初值
    /// （文件头注释记录此有意偏离 seed lesson 的理由，防 review 误判）.
    ///
    /// 注：必写 `override` —— 基类 WardrobeViewModel 有显式 `public init() {}`（与 RoomViewModel 同模式
    /// 但与 spec 注释相反；保留 override 让编译通过；与 MockWardrobeViewModel.init() 行为一致）.
    public override init() {
        super.init()
        self.appState = nil
        // 视觉初值（hydrate 前 placeholder）；bind(appState:) 后 sink 派生覆盖 catName / inventory.
        self.catName = WardrobeScaffoldDefaults.catName
        self.inventory = []  // Story 24.1 AC5：真实 inventory 语义下空仓库占位（非 mock 18 件）
        self.equipped = WardrobeScaffoldDefaults.equipped
        self.selectedCategory = WardrobeScaffoldDefaults.selectedCategory
        self.selectedCosmeticId = nil
    }

    public init(appState: AppState) {
        super.init()
        self.appState = appState
        // catName / equipped / selectedCategory seed scaffold defaults（sink 派发前 view 有数据可渲染）；
        // inventory seed [] —— Story 24.1 AC5：真实 inventory 语义；init(appState:) 路径下方 subscribeInventory
        // 立即用 appState.currentInventory（启动时通常为空）覆盖，行为一致.
        self.catName = WardrobeScaffoldDefaults.catName
        self.inventory = []
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

    /// 订阅 appState.$currentInventory（类型 [HomeEquip]）—— Story 24.1 实装真实 inventory 派生.
    /// 上游 Story 24.2 LoadInventoryUseCase 调 GET /cosmetics/inventory 后写 appState.currentInventory,
    /// 写入即通过本 sink 零 edit 反映到 Wardrobe（本 story 实装 closure mapping 完成「24.2 写 → Wardrobe 显示」闭环）.
    ///
    ///   - currentInventory 为空（启动后 / reset 后 / hydrate 前）→ self.inventory = []
    ///     （**不**再退回 WardrobeScaffoldDefaults.inventory；空仓库走 37.9 已实装空态 placeholder）
    ///   - currentInventory 非空 → self.inventory = mapToCosmeticItems(homeEquips)
    ///
    /// **保持 sink 路径不退化为一次性 hydrate**（lesson `2026-04-30-published-derived-state-needs-publisher-subscription`）：
    /// reset() 把 currentInventory = [] 时 sink 派发 [] → self.inventory = []（不残留旧 inventory）.
    private func subscribeInventory(to appState: AppState) {
        inventorySubscription = appState.$currentInventory
            .sink { [weak self] homeEquips in
                guard let self else { return }
                if homeEquips.isEmpty {
                    self.inventory = []  // 空仓库占位（37.9 空态 placeholder 生效；非 mock 18 件）
                } else {
                    self.inventory = self.mapToCosmeticItems(homeEquips)
                }
            }
    }

    /// `[HomeEquip] → [CosmeticItem]` 真实 mapping（Story 24.1 AC2 钦定形状）.
    ///
    /// 抽独立私有方法便于单测直接断言（ADR-0002 §3.1 XCTest only —— 不经 view 内省）.
    /// 逐元素转换（**不去重不聚合**）：同 cosmetic 的多个实例各成独立 CosmeticItem,
    /// 分类 count 由 WardrobeScaffoldView `inventory.filter { $0.category == }.count` 自然得出
    /// （与 Story 37.9 既有 grid 渲染契约一致；V1 §8.2 server 侧 count = instances 数组长度的 client 等价）.
    ///
    /// fallback 不丢实例（V1 §8.2「已拥有不得静默丢失」精神的 client 侧延续）：
    ///   - 未知 slot → CosmeticCategory.category(forSlot:) 内归 .bow 兜底桶
    ///   - 未知 rarity int → .N（最低品质，视觉降级而非 crash）
    private func mapToCosmeticItems(_ equips: [HomeEquip]) -> [CosmeticItem] {
        equips.map { equip in
            let category = CosmeticCategory.category(forSlot: equip.slot)
            return CosmeticItem(
                id: equip.userCosmeticItemId,           // 实例级唯一 id（多实例各独立）
                name: equip.name,
                category: category,
                rarity: Self.rarity(forServerValue: equip.rarity),
                owned: true,                            // inventory 全是已拥有实例
                iconEmoji: category.iconEmoji           // 占位（Story 30.x 接真实 sprite 升级）
            )
        }
    }

    /// server rarity int（V1 §6.9 `{1,2,3,4}`）→ `Rarity` enum（`.N/.R/.SR/.SSR`）.
    /// 未知值 fallback `.N`（最低品质，视觉降级而非 crash —— Rarity enum rawValue 是 String 非 Int,
    /// 不能用 `Rarity(rawValue:)`，故显式 switch 映射）.
    private static func rarity(forServerValue value: Int) -> Rarity {
        switch value {
        case 1:  return .N
        case 2:  return .R
        case 3:  return .SR
        case 4:  return .SSR
        default: return .N  // 未知 rarity int 视觉降级（不丢实例 / 不 crash）
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
