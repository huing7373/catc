# Story 37.9: WardrobeView Scaffold + WardrobeViewModel class 层次 + Mock/Real 两子类

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want Wardrobe Tab 显示 ui_design 高保真试衣间式装扮浏览界面（顶部 Card + 预览区 + 5 分类 Tab + 3 列网格）+ 接缝设计支持 Story 24.1（真实 RealWardrobeViewModel + LoadInventoryUseCase 接入）后续注入,
so that 既有视觉壳又有可持续接缝（WardrobeScaffoldView 内部代码 zero edit 让 Epic 24.x / 27.1 / 33.1 链路打开），同时把 Story 37.3 落地的 `WardrobeView` 占位 stub 替换为 ui_design `wardrobe.jsx` 像素级匹配的高保真 Scaffold。

## 故事定位（Epic 37 第四层第 3 条 story；Scaffold 主体 6 屏并行链路第三条，与 37.7 / 37.8 同模式）

这是 Epic 37「iPhone 架构层重构 + UI Scaffold（壳先行）」的**第四层 story**——上游 37.3（MainTabView 已挂 WardrobeView 占位 stub）/ 37.4（AppState.currentInventory / currentEquips 字段就绪）/ 37.5（Theme）/ 37.6（primitives 含 RarityTag）全部 done；37.7（HomeView）/ 37.8（RoomView）已用「class 层次 + Mock/Real 两子类 + ScaffoldDefaults seed + sink 派生 + 同步 onAppear bind」模式落地，**本 story 1:1 复刻该模式于 WardrobeView**。本 story 是 **UI Scaffold 主体** 类——属于 Epic 37 §AC 红线的「数据完全 mock + 禁 import APIClient/Repository/UseCase + 视觉像素级匹配 ui_design + 主题用 `@Environment(\.theme)` 取 token + 测试不引 SnapshotTesting/ViewInspector + 通过 `bash iphone/scripts/build.sh --test`」适用范围。

**本 story 落地后立即解锁**：
- Story 24.1（WardrobeView 真实 ViewModel 注入）—— 缩窄后范围（[sprint-change-proposal-2026-04-29-v2.md §5.4]）：「在本 story 已交付的 WardrobeScaffoldView 上把 `MockWardrobeViewModel` 替换为 `RealWardrobeViewModel: WardrobeViewModel`；inventory 来自 AppState（hydrate 来自 LoadInventoryUseCase）；selectedCategory / selectedCosmeticId 仍为 view-specific transient（@State 或 ViewModel transient @Published 视设计选择）」
- Story 24.2（LoadInventoryUseCase）+ Story 24.3（GET /cosmetics/inventory）/ Story 23.4（server）—— 在 RealWardrobeViewModel 上接 UseCase 驱动 `@Published var inventory`；WardrobeScaffoldView 视图内部 zero edit
- Story 27.1（激活穿戴按钮）—— WardrobeScaffoldView 装备/卸下按钮的 `onEquipTap` 回调本期是占位（Mock：appendInvocation；Real：log only），Story 27.1 改 EquipUseCase / UnequipUseCase 调用并写 `appState.currentEquips`
- Story 33.1（合成页 Wardrobe Tab 内 push）—— 顶部"合成"按钮（accessibilityIdentifier `wardrobeComposeEntry`）本期是占位（点击 print log）；Story 33.1 改 NavigationLink push 真实合成页
- Story 37.13（accessibility identifier 总表）—— WardrobeScaffoldView 全部 a11y identifier 来源；本 story 在 WardrobeScaffoldView 内 inline 字符串（`wardrobeView` / `wardrobeCategory_<id>` / `wardrobeEquipButton` / `wardrobeComposeEntry` / `wardrobeItem_<id>` / `wardrobeDiamondCount`），Story 37.13 收口归并到 `AccessibilityID.Wardrobe`

**本 story 的"实装"动作**（一句话概括）：在 `iphone/PetApp/Features/Wardrobe/Views/WardrobeScaffoldView.swift`（新建文件，**不**改 `WardrobeView.swift` —— 见 Dev Notes "WardrobeView 占位 stub 不删保 git history"）落地 struct `WardrobeScaffoldView` + `@ObservedObject var state: WardrobeViewModel`（基类直接，**非泛型 state**）；新建基类 `class WardrobeViewModel: ObservableObject`（class 而非 final 让子类可继承）+ 5 个 `@Published` 字段（`catName: String / inventory: [CosmeticItem] / equipped: [CosmeticCategory: String] / selectedCategory: CosmeticCategory / selectedCosmeticId: String?`）+ 1 个 abstract method（`onEquipTap(item:)`）+ 2 个 view-action transient sink 入口（`selectCategory(_:)` / `selectItem(_:)` 都是 concrete 在基类，**不是** abstract，理由见 AC1）；新建 `MockWardrobeViewModel: WardrobeViewModel` 子类（硬编码 mock inventory + override `onEquipTap` 仅 print + invocations 数组）+ `RealWardrobeViewModel: WardrobeViewModel` 子类骨架（构造注入 AppState + parameterless init + bind(appState:) + sink 订阅 appState.$currentInventory / $currentPet 派生 inventory / catName；override `onEquipTap` 占位 stub，Story 24.1 / 27.1 实装真实 UseCase 调用）；新建 `CosmeticItem` value type（id / name / category / rarity / owned / iconEmoji）+ `CosmeticCategory` enum（hat / bow / scarf / outfit / bg）+ `WardrobeScaffoldDefaults` 共享 enum（按 Story 37.8 round 1 P2 lesson 钦定路径，Mock / Real 双 init 都用它 seed）。`WardrobeView` 内 `body` 的占位 Text 替换为 `WardrobeScaffoldView(state: wardrobeViewModel)` 真实 Scaffold（caller 漏改靠编译器报错驱动；与 Story 37.3 / 37.7 / 37.8 同精神）；RootView 加 `@StateObject roomViewModel` 同级 `@StateObject wardrobeViewModel: WardrobeViewModel = RealWardrobeViewModel()` + `.environmentObject(wardrobeViewModel)`；`.onAppear` 内同步 bind appState（防 launch-time race，按 Story 37.8 round 2 P2 lesson 钦定路径）。

**关键路径："新建" + caller 替换（与 Story 37.8 同精神：本 story 是新建 + 替换占位）**：

- `WardrobeView.swift` **不删除**（保 Story 37.3 git history 可读 + 让人对比演进足迹；与 Story 37.8 RoomViewPlaceholder 不删同精神）；仅在 `WardrobeView.swift` 内 `body` 的 `Text("Wardrobe Tab Placeholder")` 替换为 `WardrobeScaffoldView(state: wardrobeViewModel)` —— `WardrobeView` 类型本身保留作为 MainTabView 直接 instantiate 的入口 view（Story 37.13 a11y 总表归并时再决定是否一并清理）
- `wardrobeViewModel: WardrobeViewModel` 注入路径走与 HomeView / RoomView 相同模式：RootView 内 `@StateObject private var wardrobeViewModel: WardrobeViewModel = RealWardrobeViewModel()`（与 `homeViewModel` / `roomViewModel` 同模式；用 RealWardrobeViewModel 而非裸 WardrobeViewModel 防生产 fatalError 路径，按 Story 37.7 round 1 P1 lesson 钦定）+ `.environmentObject(wardrobeViewModel)`；`WardrobeView` 内 `@EnvironmentObject var wardrobeViewModel: WardrobeViewModel` 取出后传给 `WardrobeScaffoldView(state:)` 子视图
- `RootView` 同步 `.onAppear { ... }` 内追加 `if let realWardrobeVM = wardrobeViewModel as? RealWardrobeViewModel { realWardrobeVM.bind(appState: appState) }`（防 launch-time race；Story 37.8 round 2 P2 lesson 钦定 `.onAppear` 而非 `.task`）
- `LaunchedContentView` 透传 `wardrobeViewModel: WardrobeViewModel` 字段（与已有 `homeViewModel` / `roomViewModel` 同模式），`.environmentObject(wardrobeViewModel)` 注入 ready 子树

**不涉及**（红线）：
- **不**实装 `LoadInventoryUseCase` / `EquipUseCase` / `UnequipUseCase` 真实 UseCase（Story 24.2 / 27.1 落地；本 story 占位 print）
- **不**接 `appState.applyHomeData` 之外的真实 inventory hydrate（hydrate 路径目前只通过 `applyHomeData` 写空 `currentInventory = []`；Story 24.1 / 24.2 落地后由 LoadInventoryUseCase 写真实 inventory；本 story RealWardrobeViewModel 通过 sink 订阅 `appState.$currentInventory` —— 启动后看到的是 `[]`，但 sink 路径已 hookup 让 Story 24.x 落地后零 edit）
- **不**改 RootView `@StateObject` wire 切到基类 WardrobeViewModel（与 RealHomeViewModel / RealRoomViewModel 一致都用 Real 子类避免 fatalError）
- **不**改 AppState / HomeData / HomePet / HomeUser / HomeEquip 类型（Story 37.4 已锁定）
- **不**实装合成页真实 NavigationLink push（Story 33.1 落地；本 story 顶部"合成"按钮点击仅 print log + invocations 记录）
- **不**新增 `ComposePlaceholderView` 等其它占位 view（合成入口仅按钮占位，不创建合成页 stub）
- **不**引 SnapshotTesting / ViewInspector（ADR-0002 §3.1 钦定 XCTest only）
- **不**改 ios/ 任何文件（CLAUDE.md + ADR-0002 §3.3）
- **不**改 server/ 任何文件
- **不**收口 `AccessibilityID.Wardrobe` 常量（本 story inline 字符串；Story 37.13 一次性归并所有 7 屏 a11y identifier）
- **不**删除 `WardrobeView.swift`（保 git history；下游 Story 37.13 / 24.1 决定）
- **不**预先生成 `CosmeticItem` 之外的额外 helper / mapping 类型
- **不**渲染钻石货币真实数值更新（mock "248"；epics.md line 4906 钦定本 MVP 不含商城）
- **不**引入 3D 猫预览真实 sprite（mock 走占位 emoji + 灰底圆角矩形；Story 30.x 接真实 sprite 时再升级）

## Acceptance Criteria

> **AC 编号体系**：AC1 是 WardrobeViewModel class 层次基类（class + 5 字段 + 1 abstract method + 2 concrete view-action method）；AC2 是 MockWardrobeViewModel / RealWardrobeViewModel 两子类；AC3 是 CosmeticItem / CosmeticCategory 值类型 + WardrobeScaffoldDefaults 共享 enum；AC4 是 WardrobeScaffoldView struct + 4 区块视觉（顶部 Card / 预览区 / 分类 Tab / 网格）；AC5 是 WardrobeView caller 替换 + RootView wire + LaunchedContentView 透传；AC6 是 #Preview 双主题 × 多场景；AC7 是单元测试 ≥6 case（≥4 epic AC line 4789-4792 钦定 + 2 守护 case）；AC8 是 UITest a11y 定位关键锚；AC9 是 build verify；AC10 是 Deliverable 清单。

### AC1 — 新建 WardrobeViewModel 基类（class 层次 + 5 字段 + 1 abstract method + 2 concrete view-action method）

**新建文件**：`iphone/PetApp/Features/Wardrobe/ViewModels/WardrobeViewModel.swift`

**类签名**（class 而非 final，让 Mock/Real 子类可继承；与 HomeViewModel Story 37.7 / RoomViewModel Story 37.8 同精神）：

```swift
// WardrobeViewModel.swift
// Story 37.9 AC1: WardrobeScaffoldView 基类 ViewModel（class 层次 + 5 字段 + 1 abstract method + 2 concrete method）.
//
// 设计：与 HomeViewModel / RoomViewModel 同精神（class 而非 final + abstract method 用 fatalError 强制 override）.
// 字段范围：5 字段（catName / inventory / equipped / selectedCategory / selectedCosmeticId）.
// 节点 8/9 后 Story 24.1 RealWardrobeViewModel 子类扩 LoadInventoryUseCase / EquipUseCase 接 sink（不在本 story 范围）.

import Foundation
import Combine

@MainActor
public class WardrobeViewModel: ObservableObject {
    /// 顶部 Card 标题用的猫名（mock "小花"；RealWardrobeViewModel 从 appState.$currentPet sink 派生）.
    /// **关键约束**：catName 在 Wardrobe 域是合法派生源 = 本地用户**自己的**猫的名字（Wardrobe 是"看自己衣柜"
    /// 单一视角；与 Story 37.8 RoomViewModel.hostCatName **不可** 派生自 currentPet 的 lesson 不冲突 ——
    /// Wardrobe 域的"猫名"语义就是 appState.currentPet.name 钦定；详见 Dev Notes "catName 派生源 vs Story 37.8 hostCatName 反例"）.
    @Published public var catName: String = ""

    /// 当前用户的 inventory（mock 多分类共 30+ 件；RealWardrobeViewModel 从 appState.$currentInventory sink 派生）.
    /// 类型 [CosmeticItem]（本 story 新建 value type；不复用 HomeEquip —— HomeEquip 只描述"已装备"非"全 inventory"）.
    @Published public var inventory: [CosmeticItem] = []

    /// 已装备道具映射（key=分类 / value=cosmeticId）；mock 默认 hat=h3 / bow=b1 / scarf=s2.
    /// MockWardrobeViewModel.onEquipTap 改本字段；RealWardrobeViewModel.onEquipTap 占位（Story 27.1 改调 EquipUseCase 写 appState.currentEquips）.
    @Published public var equipped: [CosmeticCategory: String] = [:]

    /// 当前选中分类 Tab（默认 .hat；用户点 Tab 切换）.
    /// 这是 view-specific transient state（按 ADR-0010 §3.2 表格"表单输入 / 当前选中" → ViewModel @Published 或 SwiftUI @State；
    /// 本 story 选 ViewModel @Published 让 sink 派生路径统一 + Story 24.1 落地 Real 时 selectedCategory 仍归 ViewModel transient 不进 AppState）.
    @Published public var selectedCategory: CosmeticCategory = .hat

    /// 当前选中道具 cosmeticId（用户点 grid cell 切换；nil = 当前未选中任何道具，预览区走 fallback "未选择"）.
    /// 与 selectedCategory 同精神：view-specific transient @Published（ADR-0010 §3.2）.
    @Published public var selectedCosmeticId: String?

    public init() {}

    // MARK: - abstract method（基类 fatalError 占位，子类必 override）

    /// 装备/卸下按钮回调（预览区"装备"按钮调；判断已装备时切换为"卸下"语义）.
    /// MockWardrobeViewModel: 改本地 equipped 映射 + 记录 invocation + print log.
    /// RealWardrobeViewModel（Story 27.1+）: 调 EquipUseCase / UnequipUseCase + appState.updateCurrentEquips.
    public func onEquipTap(item: CosmeticItem) {
        fatalError("WardrobeViewModel.onEquipTap must be overridden by subclass")
    }

    // MARK: - concrete view-action method（基类直接实装，子类不 override）

    /// 切换分类 Tab（用户点 5 个 Tab 之一调）.
    /// **不是** abstract —— 切换分类是纯 view-state 行为，没有"Mock vs Real"分化需求；放基类避免子类重复实装.
    /// 副作用：清空 selectedCosmeticId（切换分类后老选中 id 不在新分类 inventory 内，让预览区 fallback 到 "未选择"）.
    public func selectCategory(_ category: CosmeticCategory) {
        self.selectedCategory = category
        self.selectedCosmeticId = nil
    }

    /// 切换选中道具（用户点 grid cell 调）.
    /// **不是** abstract —— 选中是纯 view-state 行为（与 selectCategory 同精神）.
    public func selectItem(_ cosmeticId: String) {
        self.selectedCosmeticId = cosmeticId
    }

    // MARK: - derived helper（view 层方便用，子类不 override）

    /// 当前选中分类的 inventory（按 selectedCategory 过滤；grid 渲染数据源）.
    public var currentCategoryItems: [CosmeticItem] {
        inventory.filter { $0.category == selectedCategory }
    }

    /// 当前选中的 active item（selectedCosmeticId → CosmeticItem 查找；nil 时 fallback 到当前分类已装备 item 或第一个 item）.
    /// 与 ui_design wardrobe.jsx:25 `activeItem` 派生逻辑等价：selected || items.find(i => i.equip === cat) || items[0]
    public var activeItem: CosmeticItem? {
        let items = currentCategoryItems
        if let selectedId = selectedCosmeticId,
           let selected = items.first(where: { $0.id == selectedId }) {
            return selected
        }
        if let equippedId = equipped[selectedCategory],
           let equippedItem = items.first(where: { $0.id == equippedId }) {
            return equippedItem
        }
        return items.first
    }

    /// 判断 item 是否已装备（grid cell 右上对勾 + 预览区按钮文案"装备 / ✓ 已装备(点击卸下)" 用）.
    public func isEquipped(_ item: CosmeticItem) -> Bool {
        equipped[item.category] == item.id
    }
}
```

> **关键决策 1**：abstract method 用 `fatalError` 而非 default empty body —— 与 HomeViewModel / RoomViewModel 同精神（让漏 override 立刻 crash + 测试覆盖逻辑路径）。

> **关键决策 2**：`selectCategory` / `selectItem` 是 **concrete** 在基类（不是 abstract）—— 切换分类 / 切换选中是纯 view state 行为，没有 Mock vs Real 分化需求（Mock 和 Real 都是同一行为：写 @Published 字段）；abstract 只用于"未来真实业务路径有 Mock vs Real 行为分化"的方法（onEquipTap 是分化点：Mock 改本地映射 / Real 调 UseCase）。这是 abstract method 边界的关键判断标准。

> **关键决策 3**：`activeItem` / `isEquipped` / `currentCategoryItems` 是 **derived computed property** 而非 @Published 字段 —— 它们是 inventory + selectedCategory + selectedCosmeticId 的纯函数派生，每次 SwiftUI body 求值时重新算。@Published 字段会让"派生 state 跟手动 mutate"漂移（与 ADR-0010 §3.5 派生 state 单源真理同精神）。

> **基类无参 init 兼容路径**：与 HomeViewModel / RoomViewModel 同精神 —— RootView 走 RealWardrobeViewModel 子类，**不**走基类无参 init；基类 onEquipTap 在生产 wire 路径下不会被调；Preview / UITest 走 MockWardrobeViewModel。

**对应 Tasks**: Task 1.1, 1.2

### AC2 — 新建 MockWardrobeViewModel / RealWardrobeViewModel 两子类（独立文件）

**新建文件**: `iphone/PetApp/Features/Wardrobe/ViewModels/MockWardrobeViewModel.swift`

```swift
// MockWardrobeViewModel.swift
// Story 37.9 AC2: WardrobeViewModel mock 子类，用于 #Preview / UITest skip-guest-login / Scaffold 单元测试.
//
// 设计：
//   - 硬编码 mock 数据（catName / inventory / equipped 全量；走 WardrobeScaffoldDefaults seed）
//   - override 1 个 abstract method（onEquipTap）改本地 equipped 映射 + 记录 invocations 数组
//   - **不**依赖 AppState（Mock 路径走纯 ViewModel-only 数据）
//
// 与 MockHomeViewModel Story 37.7 / MockRoomViewModel Story 37.8 同模式（invocations 数组而非 closure spy）.

import Foundation
import Combine
import os.log

@MainActor
public final class MockWardrobeViewModel: WardrobeViewModel {
    /// 单元测试用：记录所有方法调用
    public enum Invocation: Equatable {
        case equipTap(itemId: String)
    }

    @Published public var invocations: [Invocation] = []

    /// 默认构造 — 走 WardrobeScaffoldDefaults seed 全量字段.
    public override init() {
        super.init()
        self.catName = WardrobeScaffoldDefaults.catName
        self.inventory = WardrobeScaffoldDefaults.inventory
        self.equipped = WardrobeScaffoldDefaults.equipped
        self.selectedCategory = WardrobeScaffoldDefaults.selectedCategory
        self.selectedCosmeticId = nil
    }

    /// 测试 / Preview 灵活构造 — 可注入任意 inventory / equipped / selectedCategory.
    public init(
        catName: String = WardrobeScaffoldDefaults.catName,
        inventory: [CosmeticItem] = WardrobeScaffoldDefaults.inventory,
        equipped: [CosmeticCategory: String] = WardrobeScaffoldDefaults.equipped,
        selectedCategory: CosmeticCategory = WardrobeScaffoldDefaults.selectedCategory
    ) {
        super.init()
        self.catName = catName
        self.inventory = inventory
        self.equipped = equipped
        self.selectedCategory = selectedCategory
        self.selectedCosmeticId = nil
    }

    // MARK: - override abstract methods

    public override func onEquipTap(item: CosmeticItem) {
        os_log(.debug, "MockWardrobeViewModel.onEquipTap %{public}@", item.id)
        invocations.append(.equipTap(itemId: item.id))
        // Mock 路径：本地切换 equipped 映射（owned == false 不改；与 ui_design wardrobe.jsx:27-32 toggleEquip 等价）.
        guard item.owned else { return }
        if equipped[item.category] == item.id {
            equipped[item.category] = nil  // 卸下
        } else {
            equipped[item.category] = item.id  // 装备
        }
    }
}
```

**新建文件**: `iphone/PetApp/Features/Wardrobe/ViewModels/RealWardrobeViewModel.swift`

```swift
// RealWardrobeViewModel.swift
// Story 37.9 AC2: WardrobeViewModel 生产实装子类（构造注入 AppState；override 1 个 abstract method 占位 stub）.
//
// 范围（本 story 占位；Story 24.1 / 24.2 / 27.1 等填充真实 UseCase 调用）：
//   - 构造注入 AppState（按 ADR-0010 §3.1 ViewModel 注入规则）+ parameterless init() 走 bind(appState:)
//   - override onEquipTap 为占位 stub（仅 print log；本期不写 appState.currentEquips —— Story 27.1 落地）
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
//     （Wardrobe 是"看自己衣柜"单一视角；与 Room 域 host 可能是别人的语境不同；详见 Dev Notes
//     "catName 派生源 vs Story 37.8 hostCatName 反例" 表格）.

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
    /// 不写 `override`：基类没有显式 no-arg init（Swift 通过默认参数合成无参调用，不形成 override 关系）.
    public init() {
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
    /// **关键**：catName 派生源是合法的 —— Wardrobe 域语义就是"看自己衣柜"，appState.currentPet 是真理源
    /// （与 Story 37.8 hostCatName 反例对比详见文件头注释 + Dev Notes "catName 派生源 vs Story 37.8 hostCatName 反例"）.
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
    /// 详见 Dev Notes "RealWardrobeViewModel inventory sink 占位策略".
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

    public override func onEquipTap(item: CosmeticItem) {
        os_log(.debug, "RealWardrobeViewModel.onEquipTap (Story 27.1 will wire EquipUseCase) %{public}@", item.id)
        // 本 story 占位：仅 log，不写 appState.currentEquips（Story 27.1 落地）.
        // **不**像 Mock 那样改本地 equipped 映射 —— Real 路径下 equipped 应当由 server EquipUseCase 成功后写入,
        // 本期 appState.currentEquips 是 [] 占位; equipped 字段在 Real 路径保持 WardrobeScaffoldDefaults.equipped seed.
    }
}
```

> **关键决策 1**：MockWardrobeViewModel / RealWardrobeViewModel 都 `final` —— 子类不可再被继承（与 ADR-0010 §3.1 mock 模式钦定 + Story 37.7 / 37.8 同精神）；只有基类 `WardrobeViewModel` 是 `class`（非 final）。

> **关键决策 2**：MockWardrobeViewModel 用 invocations 数组而非 closure spy —— 与 MockHomeViewModel / MockRoomViewModel 同精神。

> **关键决策 3**：MockWardrobeViewModel.onEquipTap **改本地 equipped 映射**（Mock 路径），RealWardrobeViewModel.onEquipTap **不改 equipped**（Real 路径仅 log；Story 27.1 改写 appState.currentEquips → equipped 通过未来 sink 派生）—— 这是 Mock vs Real 行为分化的合理点（spec line 4790 epic AC "happy: 点选 owned 道具 → 装备按钮可点击 + 状态变化" 由 Mock 路径满足；Real 路径在本 story 范围内不需要"立刻视觉切换"语义，因为 Story 27.1 后 server 写真）。

> **关键决策 4**：RealWardrobeViewModel.subscribeInventory 当前是占位 sink —— `appState.$currentInventory` 类型是 `[HomeEquip]`（Story 37.4 锁定），而本 story 用的 inventory 类型是 `[CosmeticItem]`。**不**在本 story 引入 HomeEquip → CosmeticItem mapping —— 让 Story 24.1 dev 根据真实 LoadInventoryUseCase 输出（可能是 cosmeticItem 类型而非 HomeEquip）决定 mapping shape；本 story 仅 hookup sink 让"reset 后 inventory 回 defaults"路径已工作。详见 Dev Notes "RealWardrobeViewModel inventory sink 占位策略"。

**对应 Tasks**: Task 2.1, 2.2

### AC3 — 新建 CosmeticItem / CosmeticCategory 值类型 + WardrobeScaffoldDefaults 共享 enum

**新建文件**: `iphone/PetApp/Features/Wardrobe/Models/CosmeticItem.swift`

```swift
// CosmeticItem.swift
// Story 37.9 AC3: WardrobeScaffoldView 道具数据模型.
//
// 设计：value type + Equatable + Sendable + Identifiable，纯展示数据；mock 值在 WardrobeScaffoldDefaults.
// Story 24.1 接 LoadInventoryUseCase 后由 RealWardrobeViewModel 内 mapping 写入（HomeEquip → CosmeticItem
// 或 server CosmeticInventoryDTO → CosmeticItem，由 Story 24.1 决定）.
//
// 字段名对齐 ui_design wardrobe.jsx items array shape（id / name / rarity / owned），
// 加 `category` 让按分类过滤时不依赖 dictionary key（顶层数组更易 Story 24.1 mapping）+ `iconEmoji` 给 grid cell 渲染.

import Foundation

public struct CosmeticItem: Equatable, Identifiable, Sendable {
    public let id: String              // cosmeticId / itemId（Story 24.1 后对齐 server cosmeticItemId）
    public let name: String            // 道具名（如"贝雷帽"）
    public let category: CosmeticCategory   // 分类（hat / bow / scarf / outfit / bg）
    public let rarity: Rarity          // Story 37.6 落地的 Rarity enum (N / R / SR / SSR)
    public let owned: Bool             // 是否已拥有（false → grid 半透明 + 🔒 锁标）
    public let iconEmoji: String       // grid cell 占位图标（Story 30.x 接真实 sprite 时升级；mock "🎩" / "🎀" 等）

    public init(
        id: String,
        name: String,
        category: CosmeticCategory,
        rarity: Rarity,
        owned: Bool,
        iconEmoji: String
    ) {
        self.id = id
        self.name = name
        self.category = category
        self.rarity = rarity
        self.owned = owned
        self.iconEmoji = iconEmoji
    }
}
```

**新建文件**: `iphone/PetApp/Features/Wardrobe/Models/CosmeticCategory.swift`

```swift
// CosmeticCategory.swift
// Story 37.9 AC3: 5 分类 enum（对齐 ui_design wardrobe.jsx categories 数组）.
//
// CaseIterable + Identifiable: 让 ForEach + a11y identifier 自动衍生（accessibilityIdentifier "wardrobeCategory_\(rawValue)"）.
// rawValue 与 ui_design wardrobe.jsx:9-13 categories[].id 严格对齐（hat / bow / scarf / outfit / bg）—— 不动 raw,
// 让本 story 与 ui_design CSS 视觉源 1:1 翻译时可直接 `state.selectedCategory.rawValue` 拼 a11y id 字符串.

import Foundation

public enum CosmeticCategory: String, CaseIterable, Identifiable, Sendable {
    case hat
    case bow
    case scarf
    case outfit
    case bg

    public var id: String { rawValue }

    /// Tab label 显示名（ui_design wardrobe.jsx:9-13 钦定）.
    public var label: String {
        switch self {
        case .hat:    return "帽子"
        case .bow:    return "饰品"
        case .scarf:  return "围巾"
        case .outfit: return "服装"
        case .bg:     return "背景"
        }
    }

    /// Tab icon emoji（ui_design wardrobe.jsx:9-13 钦定）.
    public var iconEmoji: String {
        switch self {
        case .hat:    return "🎩"
        case .bow:    return "🎀"
        case .scarf:  return "🧣"
        case .outfit: return "👘"
        case .bg:     return "🏞️"
        }
    }
}
```

**新建文件**: `iphone/PetApp/Features/Wardrobe/ViewModels/WardrobeScaffoldDefaults.swift`

```swift
// WardrobeScaffoldDefaults.swift
// Story 37.9 AC3: Mock 与 Real WardrobeViewModel 共享 scaffold 占位数据.
//
// 背景（Story 37.8 round 1 P2 lesson 预防性应用）：
//   原始 RealRoomViewModel.init() 仅 set roomCodeForCopy / hostCatName 占位，**不** seed members / userIsHost,
//   导致 in-room state Real path 渲染近乎空房间. 本 story 直接采用 lesson 钦定 option A：抽 shared defaults 而非 hardcode.
//
// 设计决议（与 RoomScaffoldDefaults 同精神）：
//   抽 shared defaults 而非 hardcode 在两个 ViewModel —— 避免 Mock/Real 重复定义 mock 数据，
//   未来 Story 24.1 接 LoadInventoryUseCase 时只需在 RealWardrobeViewModel sink 内覆盖即可，**不**动 Mock.

import Foundation

/// Mock 与 Real WardrobeViewModel 启动占位数据（wardrobe state UI scaffold defaults）.
///
/// **使用规则**（务必读）：
/// - Mock：直接用 WardrobeScaffoldDefaults 字段初始化 5 个 @Published（参见 MockWardrobeViewModel.init()）.
/// - Real：init() / init(appState:) 都用 WardrobeScaffoldDefaults seed 起手；sink 路径
///   （subscribeCatName / subscribeInventory）作为 override —— currentPet 来 → 派生 catName；
///   currentInventory 来 → 派生 inventory（Story 24.1 落地后写真）；都 fallback 到 WardrobeScaffoldDefaults 占位.
/// - Story 24.1 / 24.2 后：RealWardrobeViewModel 接 LoadInventoryUseCase → 写入时覆盖 inventory；
///   覆盖前仍用 WardrobeScaffoldDefaults 不让 WardrobeScaffoldView 渲染空衣柜.
public enum WardrobeScaffoldDefaults {
    /// 顶部 Card 标题用的猫名占位（mock "小花"；RealWardrobeViewModel sink 派生覆盖）.
    public static let catName: String = "小花"

    /// 默认选中分类（mock .hat —— ui_design wardrobe.jsx:4 useState('hat') 钦定）.
    public static let selectedCategory: CosmeticCategory = .hat

    /// 默认已装备映射（mock hat=h3 皇冠 / bow=b1 粉色蝴蝶结 / scarf=s2 骑士披风；与 ui_design wardrobe.jsx items 内 equip 字段对齐）.
    public static let equipped: [CosmeticCategory: String] = [
        .hat: "h3",
        .bow: "b1",
        .scarf: "s2",
    ]

    /// 完整 mock inventory（5 分类共 22 件；与 ui_design wardrobe.jsx:16-22 items 字段值 1:1 对齐）.
    public static let inventory: [CosmeticItem] = [
        // hat（6 件）
        CosmeticItem(id: "h1", name: "贝雷帽", category: .hat, rarity: .R, owned: true, iconEmoji: "🎩"),
        CosmeticItem(id: "h2", name: "草帽", category: .hat, rarity: .N, owned: true, iconEmoji: "🎩"),
        CosmeticItem(id: "h3", name: "皇冠", category: .hat, rarity: .SR, owned: true, iconEmoji: "🎩"),
        CosmeticItem(id: "h4", name: "魔法帽", category: .hat, rarity: .SSR, owned: false, iconEmoji: "🎩"),
        CosmeticItem(id: "h5", name: "蝴蝶帽", category: .hat, rarity: .R, owned: true, iconEmoji: "🎩"),
        CosmeticItem(id: "h6", name: "警官帽", category: .hat, rarity: .R, owned: false, iconEmoji: "🎩"),
        // bow（4 件）
        CosmeticItem(id: "b1", name: "粉色蝴蝶结", category: .bow, rarity: .N, owned: true, iconEmoji: "🎀"),
        CosmeticItem(id: "b2", name: "星星发夹", category: .bow, rarity: .R, owned: true, iconEmoji: "🎀"),
        CosmeticItem(id: "b3", name: "樱花发饰", category: .bow, rarity: .SR, owned: true, iconEmoji: "🎀"),
        CosmeticItem(id: "b4", name: "彩虹丝带", category: .bow, rarity: .SSR, owned: false, iconEmoji: "🎀"),
        // scarf（3 件）
        CosmeticItem(id: "s1", name: "毛线围巾", category: .scarf, rarity: .N, owned: true, iconEmoji: "🧣"),
        CosmeticItem(id: "s2", name: "骑士披风", category: .scarf, rarity: .SR, owned: true, iconEmoji: "🧣"),
        CosmeticItem(id: "s3", name: "太空斗篷", category: .scarf, rarity: .SSR, owned: false, iconEmoji: "🧣"),
        // outfit（2 件）
        CosmeticItem(id: "o1", name: "水手服", category: .outfit, rarity: .R, owned: true, iconEmoji: "👘"),
        CosmeticItem(id: "o2", name: "和服", category: .outfit, rarity: .SR, owned: false, iconEmoji: "👘"),
        // bg（3 件）
        CosmeticItem(id: "g1", name: "粉色房间", category: .bg, rarity: .N, owned: true, iconEmoji: "🏞️"),
        CosmeticItem(id: "g2", name: "樱花树下", category: .bg, rarity: .SR, owned: true, iconEmoji: "🏞️"),
        CosmeticItem(id: "g3", name: "星空", category: .bg, rarity: .SSR, owned: false, iconEmoji: "🏞️"),
    ]
}
```

> **关键决策**：inventory 共 18 件（5 分类共覆盖 N/R/SR/SSR + owned/unowned 混合）；与 epic AC line 4786「mock Inventory（每分类 6-8 件，N/R/SR/SSR 各几个，部分 unowned）」精神等价但严格 1:1 对齐 ui_design wardrobe.jsx items 数组（不另加件数让 mock 与 ui_design 视觉源同步）。

**对应 Tasks**: Task 3.1, 3.2, 3.3

### AC4 — 新建 WardrobeScaffoldView struct + 4 区块视觉

**新建文件**: `iphone/PetApp/Features/Wardrobe/Views/WardrobeScaffoldView.swift`

**关键签名**（与 HomeView Story 37.7 / RoomScaffoldView Story 37.8 同模式：`@ObservedObject var state: WardrobeViewModel` 基类直接，**非泛型 state**）：

```swift
public struct WardrobeScaffoldView: View {
    @ObservedObject public var state: WardrobeViewModel

    /// Story 37.5: 主题 token 取值入口；RootView 注入 `.environment(\.theme, currentTheme.theme)`.
    @Environment(\.theme) private var theme

    public init(state: WardrobeViewModel) {
        self.state = state
    }

    public var body: some View {
        VStack(spacing: 0) {
            topCard               // 区块 1: 顶部 Card（收藏数 + "{猫名}的衣柜" + 钻石货币 + 合成按钮）
            previewCard           // 区块 2: 预览区 Card（左 cat 占位 + 右 active item 详情 + 装备按钮）
            categoryTabs          // 区块 3: 5 分类 Tab 横向滚动
            grid                  // 区块 4: 3 列 LazyVGrid 道具网格
        }
        .background(theme.colors.pageBg.ignoresSafeArea())
        .accessibilityIdentifier("wardrobeView")
    }
    // ... 4 区块子视图实现略（Dev Notes "4 区块视觉契约"详述每块视觉 + a11y + 颜色 / spacing 规则）
}
```

**4 区块要点**（详细视觉规则见 Dev Notes "4 区块视觉契约"；这里给关键定位锚）：

- **topCard**（wardrobe.jsx:38-51）：HStack 左 VStack 12pt 700 "收藏 · 36/53" + 22pt 800 "{state.catName} 的衣柜"；右 HStack 含钻石 IconButton 风格小 pill（surface 背景 + 16pt 圆角 + sm shadow + border 1pt + 6pt vertical / 12pt horizontal padding，包 `Icons.symbol(for: "diamond")` accent 色 + "248" 13pt 800 ink，accessibilityIdentifier `wardrobeDiamondCount`）+ 间隔 + "合成" 小按钮（PrimaryButton variant: .secondary，title "合成" + sparkle icon，accessibilityIdentifier `wardrobeComposeEntry`，点击触发 invocations 记录 + Story 33.1 实装 NavigationLink push）；padding 68pt top（状态栏占位）/ 20pt horizontal / 8pt bottom
- **previewCard**（wardrobe.jsx:54-104）：Card 24pt 圆角 + accent-soft → surface 渐变 background + sm shadow + border 1pt + 14pt padding + radial-gradient 衬底装饰（径向 accent-soft → transparent 60% 边缘衰减）；HStack 左 1/2 cat 占位（140x140 灰底圆角矩形 + emoji `cat.fill` 占位 SF Symbol，Story 30.x 接真实 sprite 时升级），右 1/2 VStack 11pt 700 "当前预览" + 17pt 800 `state.activeItem?.name ?? "未选择"` + HStack 6pt（RarityTag(rarity: activeItem.rarity) + "已拥有"绿底 / "未解锁"灰底 10pt 800 white text small pill）+ PrimaryButton（fullWidth，title 走 isEquipped(activeItem) ? "✓ 已装备 (点击卸下)" : "装备"；variant: isEquipped ? .secondary : .primary；isEnabled: activeItem.owned；点击调 `state.onEquipTap(item: activeItem)`；accessibilityIdentifier `wardrobeEquipButton`）
- **categoryTabs**（wardrobe.jsx:107-124）：ScrollView(.horizontal) 内 HStack 6pt + 5 个 Button（按 CosmeticCategory.allCases 渲染；每个 Button 含 emoji + label + count "8" 小字，圆角 16，selected = accent 背景 + white 文字 + sm shadow，unselected = surface 背景 + ink 文字 + border 1pt；padding 8pt vertical / 14pt horizontal；accessibilityIdentifier `wardrobeCategory_\(category.rawValue)`；点击调 `state.selectCategory(category)`）；padding 6pt vertical / 20pt horizontal
- **grid**（wardrobe.jsx:127-164）：ScrollView 内 LazyVGrid 3 列（spacing 10pt） + ForEach `state.currentCategoryItems` Button 每个 cell（10pt padding + surface 背景 + 16pt 圆角 + sm shadow + border 1pt + selected ring 2.5pt accent stroke when `state.selectedCosmeticId == item.id`；VStack 6pt 含 60x60 灰底圆角矩形（surface-2 + 45° 斜条纹 backgroundImage 占位 —— 与 ui_design wardrobe.jsx:140-146 等价）+ 中心 28pt emoji（item.iconEmoji）+ 右上角 🔒 if !item.owned + 右上角 -4pt 偏移 success 圆标 ✓ if state.isEquipped(item) + 11pt 800 item.name 居中 + RarityTag(rarity: item.rarity, width: 24, height: 3) small 版；opacity: item.owned ? 1.0 : 0.55；accessibilityIdentifier `wardrobeItem_\(item.id)`；点击调 `state.selectItem(item.id)`）；padding 8pt top / 20pt horizontal / 100pt bottom（让出浮动 TabBar 空间）

> **关键决策 1**：4 区块布局用 `VStack(spacing: 0)` 全塞主 body —— ui_design wardrobe.jsx:37 `display: flex / flexDirection: column`，预览区 + 分类 Tab + 顶部 Card 都不滚动，仅最底层 grid 滚动（与 ui_design 钦定一致）。

> **关键决策 2**：合成按钮（accessibilityIdentifier `wardrobeComposeEntry`）本期是占位 —— **不**用 NavigationLink push（Story 33.1 落地真实 push）；点击仅 print log + invocations 记录。Mock 路径 `MockWardrobeViewModel.invocations` 不**直接**捕获 compose tap（compose 不是 onEquipTap 范围）；本 story 范围内"合成"按钮的点击行为是 **fileprivate 闭包 / @State 简单 print** —— 真实 dev 实装时建议直接用 SwiftUI `Button(action: { os_log(.debug, "wardrobeComposeEntry tap, Story 33.1 will wire") })` 内联在 WardrobeScaffoldView 的 topCard 子视图里，**不**进 ViewModel（与 RoomScaffoldView 复制按钮 1.2s feedback 走 @State 同精神 —— 临时 transient 不进 ViewModel）。

> **关键决策 3**：grid cell 的 selected ring 用 `RoundedRectangle.stroke(theme.colors.accent, lineWidth: 2.5)` overlay；**shadow 不挂在最外层**（按 Story 37.6 round 5 lesson `2026-04-30-swiftui-state-survives-id-and-shadow-over-children.md` 钦定路径：shadow 必须挂在 `RoundedRectangle.fill(...).shadow(...)` 那一层，避免 children Text/Icon 被 alpha 投影）—— 即用 `.background(RoundedRectangle.fill(theme.colors.surface).shadow(...))` 而非外层 chain `.shadow(...)`。

> **关键决策 4**：本 story **不**实装"装备成功后猫预览自动切换装扮"的视觉动画 —— 节点 9 / Story 27.1 / 30.x 接真实 sprite 时再做；本 story preview 区 cat 占位是固定灰底圆角矩形 + 占位 SF Symbol "cat.fill"。

**对应 Tasks**: Task 4.1, 4.2, 4.3, 4.4, 4.5, 4.6

### AC5 — WardrobeView caller 替换 + RootView wire + LaunchedContentView 透传

**改动文件 1**: `iphone/PetApp/Features/Wardrobe/Views/WardrobeView.swift`

**关键改动**（替换 body 占位 Text 为 WardrobeScaffoldView 真实内容）：

```swift
// 旧（Story 37.3 落地）
public struct WardrobeView: View {
    public init() {}

    public var body: some View {
        NavigationStack {
            Text("Wardrobe Tab Placeholder")
                .accessibilityIdentifier("wardrobeView")
        }
    }
}

// 新（Story 37.9 落地）
public struct WardrobeView: View {
    @EnvironmentObject var wardrobeViewModel: WardrobeViewModel

    public init() {}

    public var body: some View {
        NavigationStack {
            WardrobeScaffoldView(state: wardrobeViewModel)
        }
    }
}
```

> **关键决策**：保留 `NavigationStack` 包裹层 —— 让 Story 33.1 实装 NavigationLink push 合成页时无须再改 WardrobeView 类型签名（NavigationStack 已挂；只需在 WardrobeScaffoldView topCard 内的合成按钮改 NavigationLink 即可）。

**改动文件 2**: `iphone/PetApp/App/RootView.swift`

**关键改动**：在 `@StateObject roomViewModel` 同级新增 `@StateObject wardrobeViewModel` + 同级 `.environmentObject(wardrobeViewModel)` + `.onAppear` 内同步 bind appState：

```swift
// 旧（Story 37.8 落地）
@StateObject private var homeViewModel: HomeViewModel = RealHomeViewModel()
@StateObject private var roomViewModel: RoomViewModel = RealRoomViewModel()
@StateObject private var appState = AppState()
// ...
.onAppear {
    homeViewModel.bind(appState: appState)
    if let realRoomVM = roomViewModel as? RealRoomViewModel {
        realRoomVM.bind(appState: appState)
    }
    ensureLaunchStateMachineWired()
}

// 新（Story 37.9 追加；homeViewModel / roomViewModel 不动）
@StateObject private var homeViewModel: HomeViewModel = RealHomeViewModel()
@StateObject private var roomViewModel: RoomViewModel = RealRoomViewModel()
@StateObject private var wardrobeViewModel: WardrobeViewModel = RealWardrobeViewModel()
@StateObject private var appState = AppState()
// ...
.onAppear {
    homeViewModel.bind(appState: appState)
    if let realRoomVM = roomViewModel as? RealRoomViewModel {
        realRoomVM.bind(appState: appState)
    }
    if let realWardrobeVM = wardrobeViewModel as? RealWardrobeViewModel {
        realWardrobeVM.bind(appState: appState)
    }
    ensureLaunchStateMachineWired()
}
```

LaunchedContentView 透传：

```swift
// LaunchedContentView 加 wardrobeViewModel: WardrobeViewModel 字段（与 homeViewModel / roomViewModel 同模式）
// + body 内 .environmentObject(wardrobeViewModel) 注入 ready 子树
```

> **关键决策**：Story 37.7 round 1 P1 lesson 预防性应用 —— `@StateObject private var wardrobeViewModel: WardrobeViewModel = RealWardrobeViewModel()`（**不是**裸基类 `WardrobeViewModel()` —— 基类 onEquipTap 是 fatalError，用户点装备按钮即 crash）。

> **关键决策**：Story 37.8 round 2 P2 lesson 预防性应用 —— `bind(appState:)` 调用放在 `.onAppear` 而非 `.task`（防 launch-time race，让 ViewModel 在第一次 paint 之前就持有 AppState 引用）。

> **caller 漏改靠编译器报错驱动**：WardrobeView 内 `Text("Wardrobe Tab Placeholder")` 替换为 `WardrobeScaffoldView(state: wardrobeViewModel)` —— 旧 body 替换前编译过；替换后若漏挂 `@EnvironmentObject` 或漏注入 `.environmentObject(wardrobeViewModel)` 会 runtime crash（SwiftUI 找不到 environmentObject）—— **不依赖 grep 兜底**。MainTabView 在 Story 37.3 落地时已有 `WardrobeView().tag(AppTab.wardrobe)`，调用站不变。

**对应 Tasks**: Task 5.1, 5.2, 5.3, 5.4

### AC6 — #Preview 双主题（candy / dark）+ 多场景 mock

WardrobeScaffoldView 文件底部 `#if DEBUG ... #endif` 块含 4 个 Preview（双主题 × 默认/空 inventory 场景）：

```swift
#if DEBUG
#Preview("WardrobeScaffoldView — full mock / candy") {
    WardrobeScaffoldView(state: MockWardrobeViewModel())
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("WardrobeScaffoldView — full mock / dark") {
    WardrobeScaffoldView(state: MockWardrobeViewModel())
        .environment(\.theme, ThemeName.dark.theme)
}

#Preview("WardrobeScaffoldView — bow category / candy") {
    let vm = MockWardrobeViewModel()
    vm.selectCategory(.bow)
    return WardrobeScaffoldView(state: vm)
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("WardrobeScaffoldView — empty inventory / candy") {
    WardrobeScaffoldView(state: MockWardrobeViewModel(inventory: []))
        .environment(\.theme, ThemeName.candy.theme)
}
#endif
```

> **关键决策**：4 个 Preview 覆盖 默认（hat 已选 + 满 inventory）/ 切到 bow（验证分类切换）/ 空 inventory（验证 grid 空态）/ dark 主题（验证 Theme token 适配）。

**对应 Tasks**: Task 6.1

### AC7 — 单元测试覆盖（≥6 case，纯 XCTest + MockWardrobeViewModel + AppState）

**新建文件**: `iphone/PetAppTests/Features/Wardrobe/WardrobeViewScaffoldTests.swift`

落地以下 ≥6 case（≥4 epic AC line 4789-4792 + 守护 case 防 lesson 反例）：

```swift
// WardrobeViewScaffoldTests.swift
// Story 37.9 AC7: WardrobeScaffoldView + WardrobeViewModel class 层次单元测试.
//
// 测试基础设施约束（与 Story 2.7 + ADR-0002 §3.1 衔接）：
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）.
//   - 不引 ViewInspector / SnapshotTesting.
//   - 走 ViewModel 行为 + invocations 数组断言；不走 SwiftUI body 内省.

import XCTest
@testable import PetApp

@MainActor
final class WardrobeViewScaffoldTests: XCTestCase {

    // MARK: - case#1 happy: 切到饰品 Tab → currentCategoryItems 仅 bow 道具（epic AC line 4789）

    func testSelectCategoryFiltersInventoryByCategory() {
        let vm = MockWardrobeViewModel()
        // 默认 hat
        XCTAssertEqual(vm.selectedCategory, .hat)
        XCTAssertTrue(vm.currentCategoryItems.allSatisfy { $0.category == .hat })

        vm.selectCategory(.bow)
        XCTAssertEqual(vm.selectedCategory, .bow)
        XCTAssertTrue(vm.currentCategoryItems.allSatisfy { $0.category == .bow })
        XCTAssertFalse(vm.currentCategoryItems.isEmpty, "bow 分类至少 1 件 mock")
    }

    // MARK: - case#2 happy: 点选 owned 道具 → onEquipTap 触发 + Mock 改 equipped 映射（epic AC line 4790）

    func testOnEquipTapWithOwnedItemTogglesEquipped() {
        let vm = MockWardrobeViewModel()
        let bowItem = vm.inventory.first { $0.category == .bow && $0.id == "b2" }!
        XCTAssertTrue(bowItem.owned)
        XCTAssertNotEqual(vm.equipped[.bow], bowItem.id, "默认 b1 装备，b2 未装备")

        vm.onEquipTap(item: bowItem)
        XCTAssertEqual(vm.equipped[.bow], bowItem.id, "装备后 equipped[.bow] = b2")
        XCTAssertEqual(vm.invocations, [.equipTap(itemId: bowItem.id)])

        // 再点一次 → 卸下
        vm.onEquipTap(item: bowItem)
        XCTAssertNil(vm.equipped[.bow], "再点同 item 卸下 → equipped[.bow] = nil")
    }

    // MARK: - case#3 happy: 点选 unowned 道具 → onEquipTap 触发但 equipped 不动（epic AC line 4791）

    /// 验证 epic AC "点选 unowned 道具 → 装备按钮 disabled" —— 按钮 disabled 由 view 层 isEnabled 判断；
    /// 但即便 ViewModel.onEquipTap 被绕过 disabled 调用（如 UITest），equipped 映射也不变（Mock 路径内置 owned check）.
    func testOnEquipTapWithUnownedItemDoesNotMutateEquipped() {
        let vm = MockWardrobeViewModel()
        let unownedHat = vm.inventory.first { $0.category == .hat && !$0.owned }!
        let originalEquipped = vm.equipped

        vm.onEquipTap(item: unownedHat)
        XCTAssertEqual(vm.equipped, originalEquipped, "unowned 道具点击 equipped 不变（Mock 路径内置 owned check）")
        XCTAssertEqual(vm.invocations, [.equipTap(itemId: unownedHat.id)])
    }

    // MARK: - case#4 edge: items 空数组 → currentCategoryItems / activeItem 空（epic AC line 4792）

    func testEmptyInventoryProducesEmptyCurrentCategoryItemsAndNilActiveItem() {
        let vm = MockWardrobeViewModel(inventory: [])
        XCTAssertTrue(vm.currentCategoryItems.isEmpty)
        XCTAssertNil(vm.activeItem)
    }

    // MARK: - case#5 happy: selectItem 切换 selectedCosmeticId → activeItem 反映新选中

    func testSelectItemUpdatesActiveItem() {
        let vm = MockWardrobeViewModel()
        vm.selectCategory(.hat)
        let originalActive = vm.activeItem
        let candidate = vm.currentCategoryItems.first { $0.id != originalActive?.id }!

        vm.selectItem(candidate.id)
        XCTAssertEqual(vm.selectedCosmeticId, candidate.id)
        XCTAssertEqual(vm.activeItem?.id, candidate.id)
    }

    // MARK: - case#6 happy: selectCategory 副作用清空 selectedCosmeticId

    /// 验证 selectCategory 切分类后 selectedCosmeticId 被置 nil（避免老分类的 id 在新分类查不到导致 activeItem 为 nil 但 selected 仍残留）.
    func testSelectCategoryClearsSelectedCosmeticId() {
        let vm = MockWardrobeViewModel()
        vm.selectItem("h1")
        XCTAssertEqual(vm.selectedCosmeticId, "h1")

        vm.selectCategory(.bow)
        XCTAssertNil(vm.selectedCosmeticId, "切分类后 selectedCosmeticId 必清空")
    }

    // MARK: - case#7 守护: RealWardrobeViewModel 构造注入 AppState 不 crash + onEquipTap override 不 fatalError

    /// 防 RealWardrobeViewModel 漏 override onEquipTap 时本测试立刻 fail（fatalError 在测试中 → trap）.
    /// 与 Story 37.8 RoomViewScaffoldTests case#5 同模式.
    func testRealWardrobeViewModelConstructionAndAbstractMethodDoesNotCrash() {
        let appState = AppState()
        let vm = RealWardrobeViewModel(appState: appState)
        XCTAssertEqual(vm.catName, WardrobeScaffoldDefaults.catName, "init 走 defaults seed")
        XCTAssertFalse(vm.inventory.isEmpty, "init 走 defaults seed inventory")

        let testItem = vm.inventory[0]
        // 调用 onEquipTap override 验证不进入基类 fatalError 路径（仅 log + 不改 equipped Real 行为）.
        vm.onEquipTap(item: testItem)
        // Real 路径不改 equipped（与 Mock 不同）
    }

    // MARK: - case#8 守护: Real init 必 seed scaffold defaults（lesson 预防性应用）

    /// 与 Story 37.8 testRealRoomViewModelInitSeedsRoomScaffoldDefaults 同模式 ——
    /// 防未来 Claude 重构 init 时漏 seed inventory / equipped / catName 让 RealWardrobeViewModel 渲染空衣柜.
    /// lesson: 2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md
    func testRealWardrobeViewModelInitSeedsScaffoldDefaults() {
        // parameterless init 路径
        let vm1 = RealWardrobeViewModel()
        XCTAssertFalse(vm1.inventory.isEmpty)
        XCTAssertEqual(vm1.catName, WardrobeScaffoldDefaults.catName)
        XCTAssertEqual(vm1.equipped, WardrobeScaffoldDefaults.equipped)
        XCTAssertEqual(vm1.selectedCategory, WardrobeScaffoldDefaults.selectedCategory)

        // init(appState:) 路径
        let vm2 = RealWardrobeViewModel(appState: AppState())
        XCTAssertFalse(vm2.inventory.isEmpty)
        XCTAssertEqual(vm2.equipped, WardrobeScaffoldDefaults.equipped)
    }

    // MARK: - case#9 守护: catName 派生自 appState.currentPet（hydrate + reset 路径）

    /// 与 Story 37.7 testRealHomeViewModelGreetingFallsBackOnReset 同模式 ——
    /// 防未来 Claude 重构时把 catName sink 改一次性 hydrate 让 reset 后残留旧 pet 名.
    /// lesson: 2026-04-30-published-derived-state-needs-publisher-subscription.md
    ///
    /// **关键说明**：本测试与 Story 37.8 testRealRoomViewModelHostCatNameDoesNotDeriveFromCurrentPet 看似相反 ——
    /// Wardrobe 域 catName 派生自 currentPet 是合法（"看自己衣柜"语义），Room 域 hostCatName 派生自 currentPet 是错误（"看别人房间"语境）.
    /// 详见 Dev Notes "catName 派生源 vs Story 37.8 hostCatName 反例" 表格.
    func testRealWardrobeViewModelCatNameDerivesFromCurrentPet() {
        let appState = AppState()
        let vm = RealWardrobeViewModel(appState: appState)
        XCTAssertEqual(vm.catName, WardrobeScaffoldDefaults.catName, "appState.currentPet nil → fallback 占位")

        // hydrate 路径：写入 currentPet → catName 同步派生
        let testPet = HomePet(id: "p1", petType: 1, name: "测试猫", currentState: .rest, equips: [])
        appState.currentPet = testPet
        XCTAssertEqual(vm.catName, "测试猫")

        // reset 路径：appState.reset() 把 currentPet 置 nil → catName 即时 fallback 占位（不残留旧值）
        appState.reset()
        XCTAssertEqual(vm.catName, WardrobeScaffoldDefaults.catName, "reset 后 catName 必回占位（防 stale）")
    }

    // MARK: - case#10 守护: bind(appState:) 是同步入口（lesson 2026-04-30-onappear-vs-task-sync-bind 预防性应用）

    /// 防未来 Claude 把 bind 改成 async 路径让 RootView .onAppear 触发后第一帧 ViewModel 仍未连上 AppState.
    /// 与 Story 37.8 testRealRoomViewModelBindAppStateIsSynchronous 同模式.
    func testRealWardrobeViewModelBindAppStateIsSynchronous() {
        let appState = AppState()
        let testPet = HomePet(id: "p1", petType: 1, name: "测试猫", currentState: .rest, equips: [])
        appState.currentPet = testPet

        let vm = RealWardrobeViewModel()  // parameterless init 路径
        XCTAssertEqual(vm.catName, WardrobeScaffoldDefaults.catName, "bind 前 catName = defaults")

        vm.bind(appState: appState)  // 同步路径
        XCTAssertEqual(vm.catName, "测试猫", "bind 后立即派生（无 RunLoop tick 等待）")
    }
}
```

> **关键决策**：≥6 case（epic AC 钦定 ≥4 case；本 story 落 10 case 含 4 个守护 case 预防 Story 37.7 / 37.8 lesson 反例）—— 与 Story 37.8 5 case 相比扩到 10 case 是**预防性应用 lesson**的成本兑现（守护 case 让未来 Claude 重构时立刻发现破坏）。

> **关键决策**：不测 fatalError 路径（基类 abstract method 覆盖在 case#7 间接证明 override 已生效；显式 fatalError trap 测试 ADR-0002 §3.1 不强制）。

> **关键决策**：不测 WardrobeScaffoldView body 渲染含 a11y identifier（属 UITest 范围；详见 AC8）。

**对应 Tasks**: Task 7.1

### AC8 — UITest a11y identifier 关键锚可定位

**改动文件**: `iphone/PetAppUITests/HomeUITests.swift`（与 Story 37.7 / 37.8 同模式：本 story 加一个新 test case 在 HomeUITests.swift 内；Story 37.13 a11y 总表归并时统一移走）

```swift
// Story 37.9: WardrobeScaffoldView 关键 a11y identifier 可定位验证.
// 切到 Wardrobe Tab 后验证主结构 + 5 个分类 Tab + 装备按钮 + 合成按钮可见.
// 与 Story 37.7 testHomeScaffoldShowsAllSevenAnchors / Story 37.8 testRoomScaffoldShowsAllSevenAnchors 同模式.
func testWardrobeScaffoldShowsAllAnchors() throws {
    let app = XCUIApplication()
    app.launchEnvironment["UITEST_SKIP_GUEST_LOGIN"] = "1"
    app.launch()

    // 切到 Wardrobe Tab
    let wardrobeTab = app.buttons["tab_wardrobe"]
    XCTAssertTrue(wardrobeTab.waitForExistence(timeout: 5))
    wardrobeTab.tap()

    // 验证主容器 + 钻石 + 合成入口
    XCTAssertTrue(app.otherElements["wardrobeView"].waitForExistence(timeout: 3))
    XCTAssertTrue(app.staticTexts["wardrobeDiamondCount"].exists || app.otherElements["wardrobeDiamondCount"].exists)
    XCTAssertTrue(app.buttons["wardrobeComposeEntry"].exists)

    // 验证 5 个分类 Tab
    XCTAssertTrue(app.buttons["wardrobeCategory_hat"].exists)
    XCTAssertTrue(app.buttons["wardrobeCategory_bow"].exists)
    XCTAssertTrue(app.buttons["wardrobeCategory_scarf"].exists)
    XCTAssertTrue(app.buttons["wardrobeCategory_outfit"].exists)
    XCTAssertTrue(app.buttons["wardrobeCategory_bg"].exists)

    // 验证装备按钮
    XCTAssertTrue(app.buttons["wardrobeEquipButton"].exists)

    // 验证 grid 至少有一个 wardrobeItem_*（具体 id 由 mock data 决定，不写死 id）
    let firstHatItem = app.buttons["wardrobeItem_h1"]
    XCTAssertTrue(firstHatItem.exists, "默认 hat 分类应显示 h1 贝雷帽")
}
```

> **关键决策**：UITest 不主动验证装备/卸下完整链路 / 切换分类后 grid 内容变化（属"完整流程"测试 —— 节点 8 / 9 范围；本 story 仅验证视觉锚存在，让 Story 37.13 a11y 总表归并时有 baseline）。

> **关键决策**：UITest 路径不需要 `UITEST_FORCE_IN_ROOM` 类似 env flag —— Wardrobe Tab 不依赖任何 inRoom / inX state，启动后切 tab 即可见。

> **现有 testHomeScaffoldShowsAllSevenAnchors / testRoomScaffoldShowsAllSevenAnchors**（Story 37.7 / 37.8）**不动** —— 本 story 范围是 Wardrobe Tab，不影响 Home / Room Tab UITest。

**对应 Tasks**: Task 8.1

### AC9 — xcodegen regen + build verify

完成 AC1-AC8 后：

1. `cd iphone && xcodegen generate` 让新文件加入 PetApp / PetAppTests target（project.yml `sources: - PetApp` / `- PetAppTests` 通配规则自动 inclusion；新增文件全部在 `iphone/PetApp/Features/Wardrobe/` + `iphone/PetAppTests/Features/Wardrobe/` 下）
2. `bash iphone/scripts/build.sh --test` 跑测试通过
   - 总 case 数：~280（Story 37.8 落地后基线 ~280）+ 本 story 新增 10 unit case + 1 UITest case → ~291 case 全绿
   - 不删除任何老 case
3. grep 验证：
   - `grep -c "class WardrobeViewModel" iphone/PetApp/Features/Wardrobe/ViewModels/WardrobeViewModel.swift` ≥ 1（防漏建基类）
   - `grep "fatalError" iphone/PetApp/Features/Wardrobe/ViewModels/WardrobeViewModel.swift` 输出至少 1 次（onEquipTap abstract method）
   - `grep "final class WardrobeViewModel" iphone/PetApp/Features/Wardrobe/ViewModels/WardrobeViewModel.swift` 输出空（基类不能 final）
   - `grep -c "override func" iphone/PetApp/Features/Wardrobe/ViewModels/MockWardrobeViewModel.swift` ≥ 1（onEquipTap override）
   - `grep -c "override func" iphone/PetApp/Features/Wardrobe/ViewModels/RealWardrobeViewModel.swift` ≥ 1
   - `grep -c "WardrobeScaffoldView" iphone/PetApp/Features/Wardrobe/Views/WardrobeView.swift` ≥ 1（caller 替换已生效）
   - `grep "Text(\"Wardrobe Tab Placeholder\")" iphone/PetApp/Features/Wardrobe/Views/WardrobeView.swift` 输出空（旧占位 Text 已替换）

> **dev 实装备注**：dev 必须 `xcodegen generate` 后 commit 一并提交 `iphone/PetApp.xcodeproj/project.pbxproj` 改动（与 Story 37.5 / 37.6 / 37.7 / 37.8 同模式）。

**对应 Tasks**: Task 9.1, 9.2, 9.3

### AC10 — Deliverable 清单

- ✅ `iphone/PetApp/Features/Wardrobe/ViewModels/WardrobeViewModel.swift` 新建（class + 5 字段 + 1 abstract method fatalError 占位 + 2 concrete view-action method + 3 derived computed property + parameterless init）
- ✅ `iphone/PetApp/Features/Wardrobe/ViewModels/MockWardrobeViewModel.swift` 新建（final + invocations + 默认 ScaffoldDefaults seed + 可注入构造）
- ✅ `iphone/PetApp/Features/Wardrobe/ViewModels/RealWardrobeViewModel.swift` 新建（final + appState 构造注入 + parameterless init + bind(appState:) + 2 sink + 1 override 占位 stub）
- ✅ `iphone/PetApp/Features/Wardrobe/ViewModels/WardrobeScaffoldDefaults.swift` 新建（catName / inventory / equipped / selectedCategory 4 字段共享）
- ✅ `iphone/PetApp/Features/Wardrobe/Models/CosmeticItem.swift` 新建（id/name/category/rarity/owned/iconEmoji + Equatable + Identifiable + Sendable）
- ✅ `iphone/PetApp/Features/Wardrobe/Models/CosmeticCategory.swift` 新建（5 case + label + iconEmoji + CaseIterable + Identifiable + Sendable）
- ✅ `iphone/PetApp/Features/Wardrobe/Views/WardrobeScaffoldView.swift` 新建（struct + 4 区块视觉按 ui_design wardrobe.jsx 像素级翻译 + #Preview 4 配置 candy/dark × 默认/bow/empty 场景）
- ✅ `iphone/PetAppTests/Features/Wardrobe/WardrobeViewScaffoldTests.swift` 新建（10 case：6 epic AC + 4 守护 case 预防 lesson 反例）
- ✅ `iphone/PetApp/Features/Wardrobe/Views/WardrobeView.swift` 修改（占位 Text 替换为 WardrobeScaffoldView + 加 @EnvironmentObject 取出 wardrobeViewModel）
- ✅ `iphone/PetApp/App/RootView.swift` 修改（追加 `@StateObject wardrobeViewModel: WardrobeViewModel = RealWardrobeViewModel()` + `.environmentObject(wardrobeViewModel)` + `.onAppear` 内同步 bind(appState:) + LaunchedContentView 接收 wardrobeViewModel 透传）
- ✅ `iphone/PetAppUITests/HomeUITests.swift` 加 `testWardrobeScaffoldShowsAllAnchors`
- ✅ `iphone/PetApp.xcodeproj/project.pbxproj` 改动随 commit（xcodegen regen 结果）
- ✅ `bash iphone/scripts/build.sh --test` 全绿（~291 case）
- ✅ project.yml **不**手动改（通配规则自动 inclusion）
- ✅ RootView wire 用 RealWardrobeViewModel 而非裸基类（防 fatalError 生产 crash 路径，按 Story 37.7 lesson 钦定）
- ✅ `WardrobeView.swift` **不**删除（保 git history；Story 37.13 决定）
- ✅ MainTabView 内 `WardrobeView()` 调用站不变（caller 漏改靠编译器报错驱动 —— WardrobeView 类型签名不变；body 内部改）

## Tasks / Subtasks

- [x] Task 1: WardrobeViewModel 基类（AC1）
  - [x] 1.1 新建 `iphone/PetApp/Features/Wardrobe/ViewModels/WardrobeViewModel.swift`：`@MainActor public class WardrobeViewModel: ObservableObject` + 5 个 @Published 字段（catName / inventory / equipped / selectedCategory / selectedCosmeticId）+ 1 abstract method (onEquipTap) fatalError 占位 + 2 concrete method（selectCategory / selectItem）+ 3 derived computed property（currentCategoryItems / activeItem / isEquipped）+ parameterless init()
  - [x] 1.2 显式 `import Foundation` + `import Combine`（防 transitive @Published；与 MockHomeViewModel round 4 [P0] hardening 同精神）
- [x] Task 2: Mock/Real 子类（AC2）
  - [x] 2.1 新建 `iphone/PetApp/Features/Wardrobe/ViewModels/MockWardrobeViewModel.swift`（final class + invocations 数组 + 1 override + 默认 ScaffoldDefaults seed + 可配 init）
  - [x] 2.2 新建 `iphone/PetApp/Features/Wardrobe/ViewModels/RealWardrobeViewModel.swift`（final class + appState 构造注入 + parameterless init() / init(appState:) 双路径 + bind(appState:) idempotent + 2 sink (subscribeCatName / subscribeInventory) + 1 override 占位 stub）
- [x] Task 3: 数据模型 + ScaffoldDefaults（AC3）
  - [x] 3.1 新建 `iphone/PetApp/Features/Wardrobe/Models/CosmeticItem.swift`（struct value type + 6 字段 + Equatable + Identifiable + Sendable）
  - [x] 3.2 新建 `iphone/PetApp/Features/Wardrobe/Models/CosmeticCategory.swift`（enum 5 case + label + iconEmoji + CaseIterable + Identifiable + Sendable）
  - [x] 3.3 新建 `iphone/PetApp/Features/Wardrobe/ViewModels/WardrobeScaffoldDefaults.swift`（4 字段：catName / selectedCategory / equipped / inventory；inventory 18 件按 ui_design wardrobe.jsx 1:1 翻译）
- [x] Task 4: WardrobeScaffoldView struct + 4 区块（AC4）
  - [x] 4.1 新建 `iphone/PetApp/Features/Wardrobe/Views/WardrobeScaffoldView.swift`，含 VStack(spacing: 0) 4 区块结构 + accessibilityIdentifier "wardrobeView"
  - [x] 4.2 落地 topCard 子视图（左 VStack 收藏 + 标题 / 右 HStack 钻石 pill + 合成按钮；合成按钮内联 print log + accessibilityIdentifier `wardrobeComposeEntry`）
  - [x] 4.3 落地 previewCard 子视图（Card + accent-soft → surface 渐变 + radial-gradient 衬底 + 左 cat 占位 + 右 active item 详情 + PrimaryButton 装备按钮调 onEquipTap + accessibilityIdentifier `wardrobeEquipButton`）
  - [x] 4.4 落地 categoryTabs 子视图（ScrollView .horizontal + ForEach CosmeticCategory.allCases 5 个 Button + 选中态视觉 + accessibilityIdentifier `wardrobeCategory_<rawValue>`）
  - [x] 4.5 落地 grid 子视图（LazyVGrid 3 列 + ForEach state.currentCategoryItems Button + cell 视觉（emoji / name / RarityTag / 锁标 / ✓ 对勾 / 选中 ring） + accessibilityIdentifier `wardrobeItem_<id>` + 调 state.selectItem(id)）
  - [x] 4.6 关键锚 a11y identifier inline 字符串：wardrobeView / wardrobeDiamondCount / wardrobeComposeEntry / wardrobeEquipButton / wardrobeCategory_hat..bg / wardrobeItem_<id>
  - [x] 4.7 **lesson 预防性应用**：所有 shadow 必须挂在 `RoundedRectangle.fill(...).shadow(...)` 那一层（不挂最外层 chain），按 Story 37.6 round 5 lesson 钦定路径
- [x] Task 5: WardrobeView caller 替换 + RootView wire + LaunchedContentView 透传（AC5）
  - [x] 5.1 改 `WardrobeView.swift`：body 占位 Text 替换为 `WardrobeScaffoldView(state: wardrobeViewModel)`；加 `@EnvironmentObject var wardrobeViewModel: WardrobeViewModel`；保留 NavigationStack 包裹层（让 Story 33.1 NavigationLink push 零改）
  - [x] 5.2 改 `RootView.swift`：在 `@StateObject roomViewModel` 同级追加 `@StateObject private var wardrobeViewModel: WardrobeViewModel = RealWardrobeViewModel()`（用 Real 子类避免基类 fatalError 生产路径）+ LaunchedContentView 调用站追加 `wardrobeViewModel: wardrobeViewModel`
  - [x] 5.3 改 LaunchedContentView 内部签名：追加 `wardrobeViewModel: WardrobeViewModel` 字段 + init 参数 + body 内 `.environmentObject(wardrobeViewModel)`（与 homeViewModel / roomViewModel 同模式）
  - [x] 5.4 改 `RootView.swift` `.onAppear`：在已有 `if let realRoomVM = roomViewModel as? RealRoomViewModel { realRoomVM.bind(appState: appState) }` 后追加 `if let realWardrobeVM = wardrobeViewModel as? RealWardrobeViewModel { realWardrobeVM.bind(appState: appState) }`（按 Story 37.8 round 2 P2 lesson `2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md` 钦定路径，**不**放 `.task`）
- [x] Task 6: #Preview 4 配置（AC6）
  - [x] 6.1 WardrobeScaffoldView 文件底部 `#if DEBUG` 块加 4 个 `#Preview`（candy 默认 / dark 默认 / candy bow / candy empty）
- [x] Task 7: 单元测试（AC7）
  - [x] 7.1 新建 `iphone/PetAppTests/Features/Wardrobe/WardrobeViewScaffoldTests.swift`，落地 10 case（6 epic AC + 4 守护 case 预防 lesson 反例）
- [x] Task 8: UITest（AC8）
  - [x] 8.1 在 `HomeUITests.swift` 加 `testWardrobeScaffoldShowsAllAnchors`（不需要 env flag；切 tab 即可见）
  - [x] 8.2 验证现有 `testHomeScaffoldShowsAllSevenAnchors` / `testRoomScaffoldShowsAllSevenAnchors` 不受影响（不动）
- [x] Task 9: xcodegen regen + build verify（AC9）
  - [x] 9.1 `cd iphone && xcodegen generate` 让新文件加入 target
  - [x] 9.2 `bash iphone/scripts/build.sh --test` 跑测试通过（~291 case 全绿）
  - [x] 9.3 grep 校验：WardrobeViewModel 含 `class WardrobeViewModel`（去 final）+ ≥1 个 fatalError；MockWardrobeViewModel / RealWardrobeViewModel 各含 ≥1 个 override func；WardrobeView 含 WardrobeScaffoldView 调用 + 不含 `Text("Wardrobe Tab Placeholder")` 调用
- [x] Task 10: Deliverable 清单确认（AC10）
  - [x] 10.1 7 个新文件 + 修改 3 个老文件（WardrobeView.swift / RootView.swift / HomeUITests.swift）+ pbxproj regen 全部待 commit（不在本 dev-story 范围）

## Dev Notes

### Story 37.7 / 37.8 沉淀 lesson 预防性应用清单（关键约束）

本 story 落地前必读 6 条 lesson + 1 条 ADR-0010 决议；**不重蹈覆辙**清单：

| Lesson 文件 | 预防点 | 本 story 落地动作 |
|---|---|---|
| `2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md` | abstract method base class 注入点全部要换 concrete subclass | RootView `@StateObject wardrobeViewModel: WardrobeViewModel = RealWardrobeViewModel()` 而非裸基类（AC5） |
| `2026-04-30-published-derived-state-needs-publisher-subscription.md` | 派生 state 必须订阅 publisher，禁止 hardcode（避免 reset 后 stale） | RealWardrobeViewModel 用 sink 订阅 appState.$currentPet 派生 catName + 订阅 appState.$currentInventory 派生 inventory；**不**在 init / bind 入口一次性 hydrate（AC2 + 守护 case#9） |
| `2026-04-30-room-host-name-must-not-derive-from-local-current-pet.md` | 不要从 currentPet 派生 host name 类信息（localPet ≠ host） | **本 story 反向应用 lesson** —— Wardrobe 域 catName **是合法**派生自 currentPet（"看自己衣柜"语义）；Story 37.8 RoomViewModel hostCatName 派生自 currentPet 是错误（"看别人房间"语境）；详见下方"catName 派生源 vs Story 37.8 hostCatName 反例"表格 |
| `2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md` | RealViewModel.init 必须 seed scaffold defaults（避免 in-room/in-X state 出现空内容） | 新建 `WardrobeScaffoldDefaults` 共享 enum；Mock / Real 双子类 init 都用它 seed 全 5 字段（AC2 + AC3 + 守护 case#8） |
| `2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md` | `.onAppear` 同步 bind appState（避免 launch-time race） | RootView `.onAppear` 内追加 `realWardrobeVM.bind(appState: appState)`，**不**放 `.task`（AC5 Task 5.4 + 守护 case#10） |
| `2026-04-30-swiftui-onchange-equatable-and-stale-task-cancel.md` | SwiftUI .onChange iOS 17+ 双参签名 | 本 story Wardrobe Scaffold 不用 .onChange（无 timer 类视觉 transient）—— lesson 不直接命中；但若未来加 selectedCategory/selectedCosmeticId 联动动画时必须按 iOS 17+ 双参签名 |
| `2026-04-30-swiftui-explicit-id-nil-shared-identity.md` | .id() 不挂 nil（避免 sibling 共享 explicit identity） | 本 story 无 ViewModifier 默认 nil id 用法；grid cell 使用 `id: \.id` 走 ForEach 自带 id，不会触发该 lesson |
| `2026-04-30-swiftui-state-survives-id-and-shadow-over-children.md` | shadow 挂 RoundedRectangle.fill 那层（避免 children 被投影） | 所有 Card / grid cell shadow 必须挂在 `RoundedRectangle.fill(...).shadow(...)` 那一层（AC4 Task 4.7） |
| `2026-04-30-swiftui-floating-emoji-needs-state-driven-position.md` | @State 驱动浮动动画（不要常量 offset 装动画） | 本 story Wardrobe Scaffold 无浮动动画 —— lesson 不直接命中；preview 区 cat 占位是静态图，无运动动画 |

### catName 派生源 vs Story 37.8 hostCatName 反例（关键澄清表）

Story 37.8 round 3 lesson `2026-04-30-room-host-name-must-not-derive-from-local-current-pet.md` 指出：**Room 域 hostCatName 不可派生自 appState.currentPet（"本地用户的猫" ≠ "房间 host 的猫"）**。但**本 story Wardrobe 域 catName 派生自 appState.currentPet 是合法的**。两者看似冲突，但语义独立：

| 维度 | Story 37.8 RoomViewModel.hostCatName | Story 37.9 WardrobeViewModel.catName |
|---|---|---|
| 域语义 | "看 room host 的小屋"——host 可能是别人 | "看自己衣柜"——猫永远是本地用户自己的 |
| 真理源 | WS room.snapshot（Story 12.1 后到来） | appState.currentPet（"本地用户的猫"语义钦定） |
| pre-feature 占位 | RoomScaffoldDefaults.hostCatName（永远占位直到 WS 接通） | WardrobeScaffoldDefaults.catName（hydrate 前占位；hydrate 后即派生） |
| 是否 sink 订阅 currentPet | ❌ 错（用户加入别人房间时显示"我的猫的小屋"是 user-visible bug） | ✅ 对（衣柜永远是本地用户自己的，currentPet 就是真理源） |
| ADR-0010 §3.2 字段语义 | currentPet = "本地用户的猫"（Wardrobe 视角下就是真理源；Room 视角下不是） | 同上 |

> **关键判断标准**：lesson 不是"禁止从 currentPet 派生任何 X"，而是"判断目标 X 的真理源是不是'本地用户的某条信息'，是 → 可派生；否 → 用 placeholder"。Wardrobe 域的"猫名"无歧义就是本地用户的猫；Room 域的"host 猫名"有歧义（host 可能是别人）。

### WardrobeViewModel 改 class 而非 protocol any 模式（关键设计决策）

**选定**: 基类 `class WardrobeViewModel: ObservableObject`（非 final）+ 子类 `MockWardrobeViewModel: WardrobeViewModel` / `RealWardrobeViewModel: WardrobeViewModel` 各自 final。

**为何不走 `protocol WardrobeViewModelProtocol + any P`**：与 Story 37.7 / 37.8 同精神（v2.1 BLOCKER 7）—— SwiftUI `@ObservedObject` 不接受 `any P`；让 caller 端类型膨胀。

**为何 WardrobeScaffoldView 选择非泛型 struct**（不像 HomeView<ChestSlot: View> 是泛型）：Wardrobe 没有"chestSlot 接缝点"这种泛型必要场景；若未来 Story 33.1 加合成页 NavigationLink slot，再走泛型 ViewBuilder 路径。

### WardrobeView 占位 stub 不删保 git history（关键约束）

`WardrobeView.swift` 在 Story 37.3 落地时作为 Wardrobe Tab 占位 stub；本 story **不删除**该文件，理由：
1. **保 git history 可读**：dev 阅读 git log 能看到 `WardrobeView` 是 Story 37.3 临时方案 → Story 37.9 替换 body 占位 Text 为 WardrobeScaffoldView 的演进足迹；删除会让 git blame 失去线索。
2. **MainTabView 调用站类型不变**：`MainTabView` 的 `WardrobeView().tag(AppTab.wardrobe)` 调用站签名不变；本 story 仅改 body 内部结构。这与 Story 37.8 RoomViewPlaceholder 不删（HomeContainerView 改 caller 替换）路径不同，但同精神（保占位 stub 文件 git history）。
3. **Story 37.13 a11y 总表归并时统一清理**：Story 37.13 决定是否一并清理 / 重命名（可能改名 `WardrobeRootView` 等）；本 story 不收口。

### state owner 边界：selectedCategory / selectedCosmeticId 走 ViewModel @Published 而非 @State（与 RoomScaffoldView copiedFeedback 反向决策）

ADR-0010 §3.2 表格"表单输入 / 当前选中 → ViewModel 或 SwiftUI @State"二选一；判断标准是**是否需要跨 View 触发 / 单元测试需要断言**：

| 场景 | 选择 | 理由 |
|---|---|---|
| `copiedFeedback`（Story 37.8 RoomScaffold） | View @State | 仅 RoomScaffoldView 内"复制按钮 → 1.2s 视觉切到绿对勾"——纯本地视觉 transient，无跨 View 触发 + 1.2s timer 不需要测试断言 |
| `selectedCategory` / `selectedCosmeticId`（本 story） | ViewModel @Published | 单元测试需要断言（case#1 切分类后 currentCategoryItems 过滤；case#5 / case#6 切 item / 切分类后 selectedCosmeticId 行为）；放 @State 让单元测试无法直接断言（必须走 ViewInspector 等 SwiftUI body 内省，违反 ADR-0002 §3.1 红线） |

**反例**：若把 selectedCategory / selectedCosmeticId 放 SwiftUI @State，单元测试只能断言 ViewModel 入参（onEquipTap），不能验证派生 currentCategoryItems / activeItem 行为；放 @Published 让 derived computed property 也能断言（case#1 / case#5）。

### RealWardrobeViewModel inventory sink 占位策略（关键决策 + Story 24.1 接续点）

`appState.currentInventory` 类型是 `[HomeEquip]`（Story 37.4 锁定），但本 story `WardrobeViewModel.inventory` 类型是 `[CosmeticItem]`。两者 mapping 不在本 story 范围 —— 让 Story 24.1 dev 根据真实 LoadInventoryUseCase 输出（可能直接是 cosmeticItem 类型而非 HomeEquip）决定 mapping shape。

本 story `subscribeInventory` sink 的策略：

```swift
inventorySubscription = appState.$currentInventory
    .sink { [weak self] homeEquips in
        guard let self else { return }
        // 占位策略：currentInventory 当前永远是 [] —— LoadInventoryUseCase 未实装
        if homeEquips.isEmpty {
            self.inventory = WardrobeScaffoldDefaults.inventory   // hydrate 前 / reset 后用 mock 占位
        } else {
            // Story 24.1 落地真 mapping；本 story 占位（不改 inventory，保持已有 defaults seed 不漂移）
        }
    }
```

**为何不在本 story 引入 mapping**：① Story 24.1 真实 server 接口 GET /cosmetics/inventory 可能返回 `[CosmeticInventoryDTO]` 而非 `[HomeEquip]`，那时再决定 ViewModel.inventory 与什么 mapping 才合理；② 提前 mapping 是 over-engineer，让 Story 24.1 dev 重写工作量；③ 本 story 范围内 currentInventory 永远是 `[]`（LoadInventoryUseCase 不存在），sink 不会触发 user-visible bug。

> **接续点**：Story 24.1 落地时改 `subscribeInventory` 内部 sink closure 把 `homeEquips` 或新数据源 → `[CosmeticItem]` mapping；其它 ViewModel / View 代码 zero edit。

### 测试边界（XCTest only）

本 story 测试**仅**用 XCTest + @testable import PetApp + UITest（XCUIApplication）—— **不**引：

- ❌ SnapshotTesting（视觉 diff）：视觉验证靠 #Preview + Story 37.13 visual-review-checklist
- ❌ ViewInspector（SwiftUI body 内省）：WardrobeScaffoldView body 渲染契约靠 ui_design 1:1 翻译 + Preview 抽样兜底
- ❌ Mockingbird / Cuckoo（mock codegen）：MockWardrobeViewModel 是手写 final class subclass

### Story 37.7 / 37.8 衔接：与 HomeView / RoomScaffoldView 同 patterns 全表

| 维度 | HomeView (Story 37.7) | RoomScaffoldView (Story 37.8) | WardrobeScaffoldView (本 story) |
|---|---|---|---|
| 文件命名 | `HomeView.swift` (改写) | `RoomScaffoldView.swift` (新建)（旧 `RoomViewPlaceholder.swift` 不动） | `WardrobeScaffoldView.swift` (新建)（旧 `WardrobeView.swift` body 改写但保文件 + 改 NavigationStack 包裹层 + 加 @EnvironmentObject） |
| struct 签名 | `HomeView<ChestSlot: View>` 泛型 | `RoomScaffoldView` 非泛型 | `WardrobeScaffoldView` 非泛型 |
| state owner | `@ObservedObject var state: HomeViewModel` 基类 | `@ObservedObject var state: RoomViewModel` 基类 | `@ObservedObject var state: WardrobeViewModel` 基类 |
| ViewModel 基类 | `class HomeViewModel`（去 final）+ 5 字段 + 5 abstract method | `class RoomViewModel`（class）+ 4 字段 + 2 abstract method | `class WardrobeViewModel`（class）+ 5 字段 + 1 abstract method + 2 concrete view-action method + 3 derived computed property |
| Mock 子类 | `MockHomeViewModel`（final）+ invocations | `MockRoomViewModel`（final）+ invocations | `MockWardrobeViewModel`（final）+ invocations |
| Real 子类 | `RealHomeViewModel`（final）+ appState 注入 + sink greeting | `RealRoomViewModel`（final）+ appState 注入 + sink roomCodeForCopy | `RealWardrobeViewModel`（final）+ appState 注入 + sink catName + sink inventory（占位） |
| Defaults 共享 enum | （未抽，Story 37.7 不需要） | `RoomScaffoldDefaults`（4 字段） | `WardrobeScaffoldDefaults`（4 字段） |
| 数据模型 | `PetStats` / `AnimationState` 新建 | `RoomMember` 新建 | `CosmeticItem` / `CosmeticCategory` 新建 |
| 区块 | 5 区块 | 5 区块 | 4 区块 |
| State (transient) | `@State resetTask` | `@State copiedFeedback` + `@State copyFeedbackTask` | (无 SwiftUI @State；selectedCategory / selectedCosmeticId 走 ViewModel @Published) |
| 老占位文件处理 | 无（HomeView 改写） | RoomViewPlaceholder.swift 保留不删 | WardrobeView.swift 保留不删（改 body 内部不删文件） |
| caller 改动 | HomeContainerHomeViewBridge 改新 init 签名 | HomeContainerView inRoom 分支改 caller + 新增 HomeContainerRoomViewBridge | WardrobeView body 直接改（占位 Text → WardrobeScaffoldView；保 NavigationStack） |
| RootView wire | `@StateObject homeViewModel: HomeViewModel = RealHomeViewModel()` | 追加 `@StateObject roomViewModel: RoomViewModel = RealRoomViewModel()` + `.environmentObject` | 追加 `@StateObject wardrobeViewModel: WardrobeViewModel = RealWardrobeViewModel()` + `.environmentObject` |
| `.onAppear` bind | bind appState（已有） | 追加 `realRoomVM.bind(appState:)` | 追加 `realWardrobeVM.bind(appState:)` |
| #Preview 数 | 2（candy / dark） | 4（candy 4/2/1 + dark 4） | 4（candy default / dark default / candy bow / candy empty） |
| 单元测试 case 数 | 6（≥4 epic AC） | 5（≥4 epic AC） | 10（≥4 epic AC + 6 守护 case 预防 lesson 反例） |
| UITest case | `testHomeScaffoldShowsAllSevenAnchors` | `testRoomScaffoldShowsAllSevenAnchors` + 新 env `UITEST_FORCE_IN_ROOM` | `testWardrobeScaffoldShowsAllAnchors` + 切 tab 路径（无需 env flag） |
| a11y identifier | inline 7 锚 | inline 8 锚（含 sharedStage extra） | inline 12+ 锚（wardrobeView / wardrobeDiamondCount / wardrobeComposeEntry / wardrobeEquipButton / wardrobeCategory_*5 / wardrobeItem_*N） |
| 老 a11y 常量 | 保留 AccessibilityID.Home.* | 不引入 AccessibilityID.Room（Story 37.13 归并） | 不引入 AccessibilityID.Wardrobe（Story 37.13 归并） |

### 4 区块视觉契约（详细 ui_design 翻译表）

按 `iphone/ui_design/source/screens/wardrobe.jsx` + `iphone/ui_design/README.md` §WardrobeScreen 像素级翻译：

#### topCard（wardrobe.jsx:38-51）

```swift
HStack(alignment: .center) {
    // 左：VStack 收藏数 + 标题
    VStack(alignment: .leading, spacing: 0) {
        Text("收藏 · 36/53")
            .font(.system(size: 12, weight: .bold))
            .foregroundColor(theme.colors.inkSoft)
        Text("\(state.catName) 的衣柜")
            .font(.system(size: 22, weight: .heavy))
            .foregroundColor(theme.colors.ink)
    }

    Spacer()

    // 右：HStack 钻石 pill + 合成按钮
    HStack(spacing: 8) {
        // 钻石 pill
        HStack(spacing: 4) {
            Image(systemName: Icons.symbol(for: "diamond"))
                .font(.system(size: 16))
                .foregroundColor(theme.colors.accent)
            Text("248")
                .font(.system(size: 13, weight: .heavy))
                .foregroundColor(theme.colors.ink)
        }
        .padding(.vertical, 6)
        .padding(.horizontal, 12)
        .background(
            RoundedRectangle(cornerRadius: 16)
                .fill(theme.colors.surface)
                .shadow(color: theme.shadow.sm.color, radius: theme.shadow.sm.radius, x: theme.shadow.sm.x, y: theme.shadow.sm.y)
        )
        .overlay(RoundedRectangle(cornerRadius: 16).stroke(theme.colors.border, lineWidth: 1))
        .accessibilityIdentifier("wardrobeDiamondCount")

        // 合成按钮
        Button(action: {
            os_log(.debug, "wardrobeComposeEntry tap (Story 33.1 will wire NavigationLink push)")
        }) {
            HStack(spacing: 4) {
                Image(systemName: Icons.symbol(for: "sparkle"))
                    .font(.system(size: 14, weight: .semibold))
                Text("合成")
                    .font(.system(size: 12, weight: .heavy))
            }
            .padding(.vertical, 6)
            .padding(.horizontal, 12)
            .foregroundColor(.white)
            .background(
                RoundedRectangle(cornerRadius: 16)
                    .fill(theme.colors.accent)
                    .shadow(color: theme.shadow.sm.color, radius: theme.shadow.sm.radius, x: theme.shadow.sm.x, y: theme.shadow.sm.y)
            )
        }
        .accessibilityIdentifier("wardrobeComposeEntry")
    }
}
.padding(.top, 68)
.padding(.horizontal, 20)
.padding(.bottom, 8)
```

#### previewCard（wardrobe.jsx:54-104）

```swift
HStack(spacing: 12) {
    // 左：cat 占位
    ZStack {
        RoundedRectangle(cornerRadius: 16)
            .fill(theme.colors.surface)
            .frame(width: 140, height: 140)
        Image(systemName: "cat.fill")
            .font(.system(size: 56))
            .foregroundColor(theme.colors.inkSoft.opacity(0.4))
    }

    // 右：active item 详情
    VStack(alignment: .leading, spacing: 4) {
        Text("当前预览")
            .font(.system(size: 11, weight: .bold))
            .foregroundColor(theme.colors.inkSoft)
            .tracking(0.5)
        Text(state.activeItem?.name ?? "未选择")
            .font(.system(size: 17, weight: .heavy))
            .foregroundColor(theme.colors.ink)
            .padding(.bottom, 4)
        HStack(spacing: 6) {
            if let active = state.activeItem {
                RarityTag(rarity: active.rarity)
                Text(active.owned ? "已拥有" : "未解锁")
                    .font(.system(size: 10, weight: .heavy))
                    .foregroundColor(.white)
                    .padding(.vertical, 3)
                    .padding(.horizontal, 8)
                    .background(RoundedRectangle(cornerRadius: 8).fill(active.owned ? theme.colors.success : theme.colors.inkSoft))
            }
        }
        PrimaryButton(
            title: state.isEquipped(state.activeItem ?? CosmeticItem.placeholder) ? "✓ 已装备 (点击卸下)" : "装备",
            variant: state.isEquipped(state.activeItem ?? CosmeticItem.placeholder) ? .secondary : .primary,
            fullWidth: true,
            isEnabled: state.activeItem?.owned ?? false
        ) {
            if let active = state.activeItem {
                state.onEquipTap(item: active)
            }
        }
        .accessibilityIdentifier("wardrobeEquipButton")
    }
}
.padding(14)
.background(
    RoundedRectangle(cornerRadius: 24)
        .fill(LinearGradient(
            colors: [theme.colors.accentSoft, theme.colors.surface],
            startPoint: .top, endPoint: .bottom
        ))
        .shadow(color: theme.shadow.sm.color, radius: theme.shadow.sm.radius, x: theme.shadow.sm.x, y: theme.shadow.sm.y)
)
.overlay(RoundedRectangle(cornerRadius: 24).stroke(theme.colors.border, lineWidth: 1))
.padding(.horizontal, 20)
.padding(.vertical, 4)
```

> **关键决策**：`CosmeticItem.placeholder` 是占位空对象（`CosmeticItem(id: "", name: "未选择", category: .hat, rarity: .N, owned: false, iconEmoji: "")`）—— 让 PrimaryButton 在 activeItem nil 时仍能渲染（按钮 isEnabled = false 不可点击）；不抽到 ScaffoldDefaults，仅 inline 在 WardrobeScaffoldView 文件内 fileprivate 静态属性。

#### categoryTabs（wardrobe.jsx:107-124）

```swift
ScrollView(.horizontal, showsIndicators: false) {
    HStack(spacing: 6) {
        ForEach(CosmeticCategory.allCases) { category in
            categoryTabButton(category)
        }
    }
    .padding(.horizontal, 20)
}
.padding(.vertical, 6)

private func categoryTabButton(_ category: CosmeticCategory) -> some View {
    let isSelected = state.selectedCategory == category
    let count = state.inventory.filter { $0.category == category }.count
    return Button(action: { state.selectCategory(category) }) {
        HStack(spacing: 6) {
            Text(category.iconEmoji)
            Text(category.label)
                .font(.system(size: 12, weight: .heavy))
            Text("\(count)")
                .font(.system(size: 10, weight: .bold))
                .opacity(0.7)
        }
        .padding(.vertical, 8)
        .padding(.horizontal, 14)
        .foregroundColor(isSelected ? .white : theme.colors.ink)
        .background(
            RoundedRectangle(cornerRadius: 16)
                .fill(isSelected ? theme.colors.accent : theme.colors.surface)
                .shadow(color: isSelected ? theme.shadow.sm.color : .clear, radius: theme.shadow.sm.radius, x: theme.shadow.sm.x, y: theme.shadow.sm.y)
        )
        .overlay(RoundedRectangle(cornerRadius: 16).stroke(isSelected ? .clear : theme.colors.border, lineWidth: 1))
    }
    .accessibilityIdentifier("wardrobeCategory_\(category.rawValue)")
}
```

#### grid（wardrobe.jsx:127-164）

```swift
ScrollView {
    LazyVGrid(columns: Array(repeating: GridItem(.flexible(), spacing: 10), count: 3), spacing: 10) {
        ForEach(state.currentCategoryItems) { item in
            gridCell(item: item)
        }
    }
    .padding(.horizontal, 20)
    .padding(.top, 8)
    .padding(.bottom, 100)  // 让出浮动 TabBar 空间
}

private func gridCell(item: CosmeticItem) -> some View {
    let isSelected = state.selectedCosmeticId == item.id
    let isEquippedNow = state.isEquipped(item)
    return Button(action: { state.selectItem(item.id) }) {
        VStack(spacing: 6) {
            ZStack {
                // 占位灰底圆角矩形 + 45° 斜条纹（仅装饰；ui_design 钦定）
                RoundedRectangle(cornerRadius: 12)
                    .fill(theme.colors.surface)
                    .frame(width: 60, height: 60)
                Text(item.iconEmoji)
                    .font(.system(size: 28))
                if !item.owned {
                    Text("🔒")
                        .font(.system(size: 12))
                        .position(x: 50, y: 8)
                }
                if isEquippedNow {
                    Image(systemName: Icons.symbol(for: "check"))
                        .font(.system(size: 12, weight: .bold))
                        .foregroundColor(.white)
                        .frame(width: 20, height: 20)
                        .background(Circle().fill(theme.colors.success))
                        .overlay(Circle().stroke(theme.colors.surface, lineWidth: 2))
                        .offset(x: 26, y: -26)
                }
            }
            Text(item.name)
                .font(.system(size: 11, weight: .heavy))
                .foregroundColor(theme.colors.ink)
                .lineLimit(1)
            RarityTag(rarity: item.rarity, width: 24, height: 3)
        }
        .padding(10)
        .background(
            RoundedRectangle(cornerRadius: 16)
                .fill(theme.colors.surface)
                .shadow(color: theme.shadow.sm.color, radius: theme.shadow.sm.radius, x: theme.shadow.sm.x, y: theme.shadow.sm.y)
        )
        .overlay(
            RoundedRectangle(cornerRadius: 16)
                .stroke(isSelected ? theme.colors.accent : theme.colors.border, lineWidth: isSelected ? 2.5 : 1)
        )
        .opacity(item.owned ? 1.0 : 0.55)
    }
    .accessibilityIdentifier("wardrobeItem_\(item.id)")
}
```

> **关键**：grid cell selected ring 用 `RoundedRectangle.stroke` overlay（lineWidth 2.5 时显示 ring；lineWidth 1 时显示普通 border）；shadow 挂在 `RoundedRectangle.fill().shadow(...)` 那层（按 Story 37.6 round 5 lesson 钦定路径）。

### EnvironmentKey 默认值的 fallback（与 Story 37.5 协调）

WardrobeScaffoldView 内全部 `@Environment(\.theme) var theme` 取主题；`Environment+Theme.swift` 已落地 `defaultValue: Theme = .candy` fallback。Preview 显式 `.environment(\.theme, ThemeName.candy.theme)` 注入；Production RootView 注入 currentTheme.theme（Story 37.5 落地）。

### xcodegen regen 节奏 + project.yml 兼容性

`iphone/project.yml` 内 `targets.PetApp.sources: - PetApp` + `targets.PetAppTests.sources: - PetAppTests` 通配；新增 7 个文件全部在 `PetApp/Features/Wardrobe/` + `PetAppTests/Features/Wardrobe/` 下 → 自动 inclusion，**不**改 project.yml。dev 必须 `cd iphone && xcodegen generate` 重生成 `PetApp.xcodeproj`，commit pbxproj diff。

### 与 ADR-0002 §3.1 测试栈钦定的对齐

本 story 测试**仅**用 XCTest + @testable import PetApp + UITest（XCUIApplication）—— 见"测试边界"段。

### 测试 case 数量取舍（≥6 / 实装 10 / 守护 lesson 反例）

epic AC line 4789-4792 钦定 ≥4 case；本 story 落地 10 case：

1. 切到饰品 Tab → currentCategoryItems 仅 bow 道具（epic AC line 4789）
2. 点选 owned 道具 → onEquipTap 触发 + Mock 改 equipped 映射（epic AC line 4790）
3. 点选 unowned 道具 → onEquipTap 触发但 equipped 不动（epic AC line 4791）
4. items 空数组 → currentCategoryItems / activeItem 空（epic AC line 4792）
5. selectItem 切换 selectedCosmeticId → activeItem 反映新选中
6. selectCategory 副作用清空 selectedCosmeticId
7. RealWardrobeViewModel 构造注入 AppState 不 crash + onEquipTap override 不 fatalError（守护）
8. Real init 必 seed scaffold defaults（守护，lesson 预防性应用）
9. catName 派生自 appState.currentPet（hydrate + reset 路径，守护 lesson 预防）
10. bind(appState:) 是同步入口（守护 lesson 预防）

### 顶部"合成"按钮入口契约（与 Story 33.1 协调）

wardrobe.jsx 钦定顶部右侧有"合成"小按钮（与钻石 pill 同侧）。本 story `wardrobeComposeEntry` 实装为占位 Button（点击仅 print log），**不**实装 NavigationLink push（Story 33.1 范围）。

> **关键决策**：`wardrobeComposeEntry` accessibilityIdentifier 在本 story 已落地 —— Story 33.1 实装 NavigationLink push 时只需在 Button action 内改路由调用，accessibilityIdentifier 不变（让 UITest case 在 Story 33.1 落地后无须改 UITest）。

### 与 Story 24.1 衔接的红线（关键约束）

Story 24.1 缩窄后的范围（sprint-change-proposal-2026-04-29-v2.md §5.4）：
- 把 RealWardrobeViewModel 替换 MockWardrobeViewModel（RootView wire 切到 Real —— **本 story 已落地**：用 `RealWardrobeViewModel()` 而非裸基类）
- 给 RealWardrobeViewModel 接 LoadInventoryUseCase 真实调用（本 story RealWardrobeViewModel.subscribeInventory 是占位 sink；Story 24.1 改 sink closure 走真实 mapping）
- inventory 来自 AppState（hydrate 来自 LoadInventoryUseCase）（**本 story 已 hookup sink**；Story 24.1 改 closure 实装 mapping）
- selectedCategory / selectedCosmeticId 仍为 view-specific transient（**本 story 已落地为 ViewModel @Published**；Story 24.1 不动）

> **关键决策**：本 story 不预先加 LoadInventoryUseCase 接口字段 —— Story 24.1 实装时根据真实 UseCase shape 决定字段；预 over-design 反而让 Story 24.1 dev 在 mapping 路径上重写浪费工作量（参考 ADR-0010 §4.4 缓解策略）。

### Project Structure Notes

- 新建目录 `iphone/PetApp/Features/Wardrobe/ViewModels/` + `iphone/PetApp/Features/Wardrobe/Models/`（已有 `iphone/PetApp/Features/Wardrobe/Views/`）
- 新建目录 `iphone/PetAppTests/Features/Wardrobe/`
- 全部走 `iphone/project.yml` 通配 inclusion；不改 project.yml

### References

- [Source: docs/宠物互动App_总体架构设计.md] —— 总体架构与产品规则（仓库 / 装扮概念）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md] —— iOS 工程目录结构（Features/Wardrobe/ViewModels|Models|Views/ 三层）
- [Source: docs/宠物互动App_V1接口设计.md §仓库] —— Story 24.x 后接的 server 接口契约（本 story 不依赖）
- [Source: _bmad-output/planning-artifacts/epics.md §Story 37.9] —— 本 story epic AC（line 4771-4793）
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md §3.1] —— ADR-0002 测试栈钦定（XCTest only）
- [Source: _bmad-output/implementation-artifacts/decisions/0009-ios-navigation.md §3.5] —— ADR-0009 主入口 4 Tab（Wardrobe Tab 直接路由）
- [Source: _bmad-output/implementation-artifacts/decisions/0010-app-state.md §3.1 §3.2 §3.5] —— ADR-0010 ViewModel 注入规则 + AppState 范围白名单 + state owner 边界
- [Source: _bmad-output/planning-artifacts/sprint-change-proposal-2026-04-29-v2.md §5.4] —— Story 24.1 缩窄范围 + Wardrobe Tab 直接路由（不再 Sheet）
- [Source: iphone/ui_design/source/screens/wardrobe.jsx] —— 4 区块视觉源（line 1-186 全文）
- [Source: iphone/ui_design/README.md §WardrobeScreen] —— WardrobeScreen 概述（line 60+）
- [Source: iphone/PetApp/Features/Home/Views/HomeView.swift] —— Story 37.7 落地的 HomeView，本 story 1:1 复刻 class 层次模式
- [Source: iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift / MockHomeViewModel.swift / RealHomeViewModel.swift] —— class 层次 + Mock/Real 三件套参考实现
- [Source: iphone/PetApp/Features/Room/Views/RoomScaffoldView.swift] —— Story 37.8 落地的 RoomScaffoldView，本 story 1:1 复刻 class 层次模式 + sink + Defaults seed 模式
- [Source: iphone/PetApp/Features/Room/ViewModels/RoomViewModel.swift / MockRoomViewModel.swift / RealRoomViewModel.swift / RoomScaffoldDefaults.swift] —— RoomViewModel class 层次 + ScaffoldDefaults 共享 enum 参考实现
- [Source: iphone/PetApp/Features/Room/Models/RoomMember.swift] —— 数据模型 value type 参考实现
- [Source: iphone/PetApp/Features/Wardrobe/Views/WardrobeView.swift] —— Story 37.3 落地的占位 stub（本 story 改 body 内部）
- [Source: iphone/PetApp/Core/DesignSystem/Primitives/Card.swift / PrimaryButton.swift / Avatar.swift / Icons.swift / RarityTag.swift] —— Story 37.6 落地的 primitives，本 story 复用
- [Source: iphone/PetApp/Core/DesignSystem/Theme.swift] —— Story 37.5 Theme tokens
- [Source: iphone/PetApp/App/RootView.swift / MainTabView.swift] —— RootView wire 模式 + MainTabView 4 Tab 路由
- [Source: iphone/PetApp/App/AppState.swift] —— Story 37.4 AppState 7 字段（含 currentInventory / currentEquips / currentPet）
- [Source: docs/lessons/2026-04-30-real-home-viewmodel-injection-must-not-leave-base-fatalerror.md] —— abstract method base class 注入点必须 concrete subclass
- [Source: docs/lessons/2026-04-30-published-derived-state-needs-publisher-subscription.md] —— 派生 state 必须订阅 publisher（避免 reset 后 stale）
- [Source: docs/lessons/2026-04-30-room-host-name-must-not-derive-from-local-current-pet.md] —— 不要从 currentPet 派生 host name 类信息（localPet ≠ host）
- [Source: docs/lessons/2026-04-30-real-viewmodel-init-must-seed-scaffold-defaults.md] —— RealViewModel.init 必须 seed scaffold defaults
- [Source: docs/lessons/2026-04-30-onappear-vs-task-sync-bind-before-first-paint.md] —— `.onAppear` 同步 bind appState（避免 launch-time race）
- [Source: docs/lessons/2026-04-30-swiftui-state-survives-id-and-shadow-over-children.md] —— shadow 挂 RoundedRectangle.fill 那层
- [Source: docs/lessons/2026-04-30-swiftui-onchange-equatable-and-stale-task-cancel.md] —— SwiftUI .onChange iOS 17+ 双参签名

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m] (Claude Code · Opus 4.7 1M context, 2026-05-01)

### Debug Log References

- `bash iphone/scripts/build.sh --test` 全绿，PetAppTests 共 293 unit cases 0 failures（Story 37.8 baseline 283 + 本 story 新增 10 = 293）.
- 编译期 fix：`RealWardrobeViewModel.init()` 必须写 `override`（基类 `WardrobeViewModel` 有显式 `public init() {}`，与 RoomViewModel 同精神；与 spec 文字注释"不写 override"相反 —— 实测编译报 "overriding declaration requires an 'override' keyword"，按真实 Swift 语义保留 `override` 关键字）.

### Completion Notes List

- AC1（WardrobeViewModel 基类）：`@MainActor public class WardrobeViewModel: ObservableObject` 含 5 个 @Published（catName / inventory / equipped / selectedCategory / selectedCosmeticId）+ 1 abstract method `onEquipTap(item:)` fatalError 占位 + 2 concrete view-action method（selectCategory / selectItem）+ 3 derived computed property（currentCategoryItems / activeItem / isEquipped）.
- AC2（Mock/Real 子类）：MockWardrobeViewModel final + invocations 数组 + 2 init（无参 + 可注入）+ override onEquipTap 改本地 equipped 映射（owned check 内置）；RealWardrobeViewModel final + 2 init（parameterless / appState 注入）+ bind(appState:) idempotent + 2 sink（subscribeCatName ← appState.$currentPet / subscribeInventory ← appState.$currentInventory）+ override onEquipTap 占位 stub（仅 log）.
- AC3（数据模型）：CosmeticItem struct + Equatable + Identifiable + Sendable；CosmeticCategory enum 5 case + label / iconEmoji + CaseIterable + Identifiable + Sendable；WardrobeScaffoldDefaults 共享 enum（catName / selectedCategory / equipped / inventory 4 字段，inventory 18 件按 ui_design wardrobe.jsx 1:1 翻译）.
- AC4（WardrobeScaffoldView）：VStack(spacing: 0) 4 区块 topCard / previewCard / categoryTabs / grid + 12+ a11y identifier inline 字符串；shadow 全部挂在 `RoundedRectangle.fill(...).shadow(...)` 那一层（按 Story 37.6 round 5 lesson 钦定路径）；selected ring 用 stroke overlay 而非外层 chain `.shadow`；预览区 cat 占位走 cat.fill SF Symbol（Story 30.x 接真实 sprite 时升级）.
- AC5（caller 替换 + RootView wire + LaunchedContentView 透传）：WardrobeView 内 body 占位 Text 替换为 WardrobeScaffoldView(state: wardrobeViewModel) + 加 @EnvironmentObject + 保留 NavigationStack；RootView 同级 @StateObject wardrobeViewModel: WardrobeViewModel = RealWardrobeViewModel()（防基类 fatalError 生产 crash）+ LaunchedContentView 字段/init/.environmentObject 透传 + .onAppear 内追加 realWardrobeVM.bind(appState:)（按 Story 37.8 round 2 [P2] lesson 钦定路径 `.onAppear` 同步 bind）.
- AC6（#Preview）：4 个 Preview（candy 默认 / dark 默认 / candy bow / candy empty）+ MainTabView Preview 同步注入 MockWardrobeViewModel.
- AC7（单元测试 10 case）：6 epic AC + 4 守护 case（case#7 Real 构造 + abstract method 不 crash / case#8 Real init seed defaults / case#9 catName 派生 hydrate+reset / case#10 bind 同步）；全部通过.
- AC8（UITest）：HomeUITests.swift 加 testWardrobeScaffoldShowsAllAnchors —— 切到 wardrobe Tab 后断言 wardrobeView / wardrobeDiamondCount / wardrobeComposeEntry / wardrobeCategory_*5 / wardrobeEquipButton / wardrobeItem_h1 锚可见；不需要 env flag.
- AC9（build verify）：xcodegen 重新生成 PetApp.xcodeproj；`bash iphone/scripts/build.sh --test` 全绿 293 case；grep 校验全部满足（class WardrobeViewModel = 1 / fatalError = 3 / final class WardrobeViewModel = 0 / Mock override func = 1 / Real override func = 1 / WardrobeView 含 WardrobeScaffoldView = 2 / 旧 placeholder Text = 0）.
- AC10（Deliverable）：7 个新文件 + 3 个老文件改动 + pbxproj 自动 regen 待 commit.

### File List

新建：
- `iphone/PetApp/Features/Wardrobe/Models/CosmeticCategory.swift`
- `iphone/PetApp/Features/Wardrobe/Models/CosmeticItem.swift`
- `iphone/PetApp/Features/Wardrobe/ViewModels/WardrobeViewModel.swift`
- `iphone/PetApp/Features/Wardrobe/ViewModels/MockWardrobeViewModel.swift`
- `iphone/PetApp/Features/Wardrobe/ViewModels/RealWardrobeViewModel.swift`
- `iphone/PetApp/Features/Wardrobe/ViewModels/WardrobeScaffoldDefaults.swift`
- `iphone/PetApp/Features/Wardrobe/Views/WardrobeScaffoldView.swift`
- `iphone/PetAppTests/Features/Wardrobe/WardrobeViewScaffoldTests.swift`

修改：
- `iphone/PetApp/Features/Wardrobe/Views/WardrobeView.swift`（body 占位 Text → WardrobeScaffoldView + @EnvironmentObject）
- `iphone/PetApp/App/RootView.swift`（@StateObject wardrobeViewModel + LaunchedContentView 字段/init/.environmentObject + .onAppear bind）
- `iphone/PetApp/App/MainTabView.swift`（Preview 注入 MockWardrobeViewModel）
- `iphone/PetAppUITests/HomeUITests.swift`（加 testWardrobeScaffoldShowsAllAnchors）
- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen 自动 regen）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（37-9-wardrobeview-scaffold: ready-for-dev → review）

### Change Log

| Date | Change | Author |
|---|---|---|
| 2026-05-01 | Story 37.9 实装：WardrobeView Scaffold（base class + Mock/Real + Defaults + 2 sink + UITest 锚）落地，10 unit case + 1 UITest case 全绿；7 个新文件 + 3 个老文件改 + xcodegen pbxproj regen | dev (Claude Opus 4.7) |
