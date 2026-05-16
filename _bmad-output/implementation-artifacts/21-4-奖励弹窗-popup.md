# Story 21.4: 奖励弹窗 popup（RewardPopupView + .sheet(item:) wire）

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want 开箱成功后看到一个弹窗展示我获得的装扮（图标 + 名称 + 品质徽章）,
so that 我有获得感（点开宝箱不只是"步数 -1000 + 倒计时重启"的冷反馈，而是看到一只新装扮浮在弹窗中央 + 按品质显配色徽章，立即知道这次开到了什么、稀有不稀有）.

## 故事定位（Epic 21 第 4 条 story；接 21.3 落地的 `HomeViewModel.pendingReward` + `ChestRewardSnapshot` → 加上 RewardPopupView 闭环）

这是 Epic 21「iOS - 首页宝箱倒计时 + 奖励弹窗」第 4 条 story —— 在 21.3 已落地的 `HomeViewModel.pendingReward: ChestRewardSnapshot?` @Published 字段（OpenChestUseCase 成功后写入）+ Story 37.6 共享 primitives（RarityTag / PrimaryButton / FadeInModifier）+ ADR-0009 §3.3 sheet 白名单（"奖励弹窗（开箱）"行）基础上，**新增 reward popup 弹窗视觉 + sheet 装配**：

1. **`RewardPopupView` 纯展示 SwiftUI 组件**（不持 ViewModel；与 JoinRoomModal 同精神 —— 接受 `reward: ChestRewardSnapshot` 值 + `onClose: () -> Void` closure；视觉 = AsyncImage iconUrl + "获得 {reward.name}" + RarityTag 品质徽章 + "确定" PrimaryButton + FadeIn 入场动画）；
2. **`RewardRarityTagMapper` 纯函数 helper**（`ChestRewardSnapshot.rarity: RewardRarity` → `Rarity` enum 的映射；RewardRarity{1..4} ↔ Rarity{N/R/SR/SSR}）—— 让 RewardPopupView body 直接用 Rarity primitive，把映射规则抽出来给单测覆盖（同 HomeRoomDispatcher / HomePetNameResolver 同精神，ADR-0002 §3.1 禁 ViewInspector → 视觉派生规则抽纯函数走 XCTest 直测）；
3. **`HomeView` 内 `.sheet(item: $state.pendingReward)` modifier 装配**（在既有 `.sheet(isPresented: $state.showJoinModal)` 旁加第二个 sheet；SwiftUI 支持多 sheet modifier 共存 —— 不同 sheet 同时挂在同一 view 上，但同一时刻只能有一个弹出。本 story 不与 JoinRoomModal 同时出现：弹窗弹出在 "用户点开宝箱 → 开箱成功" 路径上，与 "用户点加入队伍 → modal 弹出" 路径互斥）；
4. **`onDismiss` 闭包按 lesson `2026-05-02-sheet-onDismiss-fires-on-button-close-too.md` 钦定路径**：用 `dismissReason` enum 标记意图（本 story 只有"确定按钮关闭"+"swipe-dismiss"两路径都同义 → 共用一个 path：清空 `pendingReward = nil`；没有差异化分支，但仍走 dismissReason 模式留接缝 / 防未来加 "立即再开一次" 按钮时引入双触发 bug；详 Dev Notes "sheet onDismiss 防双触发"）；
5. **AccessibilityID 新增 RewardPopup nested enum**（`AccessibilityID.RewardPopup.popup` / `.rarityTag` / `.confirmButton` / `.icon` / `.nameLabel` 共 5 个常量）—— 与 Story 37.13 落地的 JoinRoomModal / Wardrobe / Profile 等 nested enum 同模式；UITest 通过 identifier 定位弹窗存在 + 点击确定按钮关闭；
6. **HomeViewModel `pendingReward` binding 路径**：21.3 已声明 `@Published public var pendingReward: ChestRewardSnapshot?` 字段 + ChestRewardSnapshot 已 Identifiable —— 本 story 直接拿来 `.sheet(item: $state.pendingReward) { reward in RewardPopupView(reward: reward, onClose: ...) }`；视觉契约钦定 sheet 入场用 `FadeInModifier`（与 ADR-0009 §3.4 表 + spec AC 行 3115 "含淡入动画" 钦定一致）；
7. **节点 7 不入仓不变量保持**（spec AC 行 3116 红线）：弹窗显示后**不更新仓库状态**（节点 8 / Story 23.5 才有仓库），仅展示。本 story 不调任何 wardrobe / inventory API，不写 AppState 任何 inventory 字段。pendingReward 写到 nil 后 ViewModel 端无任何持久化副作用（如未来加 "lastReward 历史" 字段是节点 8+ 的事）；
8. **单元测试 ≥3 case**（RewardPopupView + RewardRarityTagMapper）+ **UI 测试 ≥1 case**（XCUITest mock server 返固定 reward → tap 开宝箱 → 弹窗出现 → tap 确定 → 弹窗消失）.

**本 story 落地后立即解锁**：

- **Story 22.1 节点 7 demo 验收**：本 story + 21.5 完工后构成节点 7 iOS 端完整开箱链路（GET → 倒计时 → POST → 弹窗）；22.1 跨端集成测试场景 4 "dev grant-steps + 开箱" 中弹窗视觉断言（含 cosmetic icon + name + rarity 徽章）直接由本 story 视觉契约满足；
- **Story 33.5 合成奖励弹窗**：epics.md 行 4322 钦定 ComposeRewardPopupView "**复用** Story 21.4 RewardPopupView 结构" —— 本 story 把 RewardPopupView 落在 `Features/Home/Views/`（与 ChestCardView 同位置），节点 11 落地合成时可让 ComposeRewardPopupView 抽公共 primitive 到 `Core/DesignSystem/Compounds/RewardPopupBaseView.swift` 或直接复用同一 view（取决于 Story 33.5 落地时合成弹窗的差异 —— 本 story 不预先抽，按 YAGNI 原则）.

**关键路径（与 JoinRoomModal sheet 装配同精神，"sheet item: optional → SwiftUI 自动 nil ↔ non-nil 切换驱动弹窗"）**：

- `pendingReward: ChestRewardSnapshot?` 是 SwiftUI sheet 的 driver source —— 由 21.3 OpenChestUseCase 成功路径写入 non-nil，由本 story sheet 关闭路径写回 nil；
- ChestRewardSnapshot 已 Identifiable（21.3 落地）→ `.sheet(item:)` modifier 直接订阅；
- RewardPopupView 是**纯展示组件**（与 JoinRoomModal 同精神）：不持 ViewModel / 不调 UseCase / 不读 AppState —— 接受 `reward: ChestRewardSnapshot` 值 + `onClose: () -> Void` closure；
- "关闭弹窗"由 SwiftUI 自动 set `$pendingReward.wrappedValue = nil`（item: optional binding 模式契约）—— caller 端不需要手动写 ViewModel state；这与 isPresented: Bool 模式不同（后者关闭后需要按钮闭包 / onDismiss 手动 set false）.

**不涉及**（红线）：

- **不**改 ChestRewardSnapshot 结构（21.3 已落地 + 节点 7 阶段冻结；新增字段是节点 8+ 的事，如 createdAt / receivedSource 等）；
- **不**改 HomeViewModel.pendingReward 字段类型 / @Published 声明（21.3 已落地 Optional<ChestRewardSnapshot>）；
- **不**改 OpenChestUseCase（21.3 已落地 + 不在本 story 范围；本 story 只接收 pendingReward 写入信号，不改 use case 编排）；
- **不**改 ChestCardView 任何一行（21.1 / 21.3 落地的双态视觉 + isOpening prop；本 story 弹窗与卡片解耦 —— 弹窗在 HomeView 层挂 sheet，不嵌进 ChestCardView 内）；
- **不**实装 "立即再开一次 / 立即穿戴" 按钮（节点 8+ 范围；spec AC 行 3114 只钦定 "确定按钮关闭弹窗"）；
- **不**写仓库（spec AC 行 3116 红线 "节点 7 阶段弹窗显示后不更新仓库状态"）—— pendingReward 写 nil 后无任何 AppState mutation；
- **不**做"奖励历史"持久化（HomeViewModel.pendingReward 关闭后即 nil；未来如需 "最近 5 次奖励" 列表是节点 8+ 的 Wardrobe 模块或独立 lastRewardHistory 字段）；
- **不**在 RewardPopupView 内调任何 navigation push（如点击装扮跳 Wardrobe 详情）—— 节点 7 阶段弹窗是 read-only 展示，没有跳转链路；
- **不**用 `.fullScreenCover`（ADR-0009 §3.3 表行 116 "奖励弹窗（开箱）" 备注 "`.fullScreenCover` **或**自定义 overlay" 给了选择空间，本 story 选 `.sheet`：理由是 (a) 与 JoinRoomModal `.sheet` 同模式视觉一致 / (b) `.sheet` 支持 swipe-dismiss 用户体验更现代 / (c) 节点 7 阶段弹窗仅展示不阻塞继续操作，sheet 适合 — 不需要全屏遮罩）；
- **不**用自定义 overlay（同上理由：sheet 已足够；自定义 overlay 需要自己写 z-index / 遮罩 / dismiss gesture，违反 YAGNI；ADR-0002 §3.1 也间接钦定优先用 SwiftUI 框架原生 modifier）；
- **不**给 RewardPopupView 加 SnapshotTesting（ADR-0002 §3.1 禁；视觉断言走 UITest + AccessibilityID + 纯函数 mapper 单测三层防御，与 ChestCardView 21.1 落地的策略一致）；
- **不**在 RewardPopupView 内挂 `@EnvironmentObject` AppState（弹窗是纯展示，应该完全无依赖；接受 reward 值 + onClose closure 足够）；
- **不**改 server 任何文件（端独立原则；POST /chest/open + ChestRewardDTO 由 Story 20.6 落地）；
- **不**改 ios/ 任何文件（CLAUDE.md + ADR-0002 §3.3）；
- **不**引 SnapshotTesting / ViewInspector / OHHTTPStubs / Mockingbird（ADR-0002 §3.1 钦定 XCTest only + 既有 MockBase / MockChestRepository 等已落地 mock 模式复用）.

## Acceptance Criteria

> **AC 编号体系**：AC1 = RewardRarityTagMapper 纯函数 helper；AC2 = RewardPopupView 纯展示组件；AC3 = AccessibilityID.RewardPopup nested enum；AC4 = HomeView `.sheet(item: $state.pendingReward)` 装配 + onDismiss 防双触发；AC5 = 节点 7 不入仓不变量；AC6 = 单元测试 ≥3 case；AC7 = UI 测试 ≥1 case；AC8 = build verify + ios-simulator MCP UI 实跑；AC9 = Deliverable 清单。

### AC1 — `RewardRarityTagMapper` 纯函数 helper（`RewardRarity` → `Rarity` 映射）

**新建文件**：

- `iphone/PetApp/Features/Home/Views/RewardRarityTagMapper.swift`

**契约**：

```swift
// RewardRarityTagMapper.swift
// Story 21.4 AC1: RewardRarity (1..4) → Rarity (N/R/SR/SSR) 映射纯函数 helper.
//
// 抽出来的理由（与 HomeRoomDispatcher / HomePetNameResolver / JoinRoomInputNormalizer 同精神）:
//   - ADR-0002 §3.1 禁用 ViewInspector / SnapshotTesting → 视觉派生规则必须抽成纯函数让 XCTest 直接覆盖
//   - 让 RewardPopupView body 直接用 Rarity primitive (不在 view 内 switch case)，保持 view body 干净
//   - 让单测断言 helper(input) == output 即等价于断言 view 渲染了对应徽章
//
// 命名（_Mapper 后缀模式）: 与 MotionStateMapper 同精神; 与 HomeRoomDispatcher 区分（后者是 dispatch / 决策语义）.

import Foundation

public enum RewardRarityTagMapper {
    /// 映射 RewardRarity → RarityTag 用的 Rarity enum.
    ///
    /// 映射表（V1 §6.9 + 数据库设计 §6.9 + RarityTag 配色）:
    ///   - .common    (1) → .N    灰色  #b0b0b0
    ///   - .rare      (2) → .R    蓝色  #7db3e8
    ///   - .epic      (3) → .SR   紫色  #c58ae8
    ///   - .legendary (4) → .SSR  金红渐变 #ffd166 → #ef476f
    ///
    /// 不做兜底 default case：RewardRarity enum 限定 1..4 四档（switch exhaustive），添加新档时编译期强制更新此 mapper.
    public static func map(_ rarity: RewardRarity) -> Rarity {
        switch rarity {
        case .common:    return .N
        case .rare:      return .R
        case .epic:      return .SR
        case .legendary: return .SSR
        }
    }
}
```

**验收口径**：

- `RewardRarityTagMapper.map(.common) == .N`
- `RewardRarityTagMapper.map(.rare) == .R`
- `RewardRarityTagMapper.map(.epic) == .SR`
- `RewardRarityTagMapper.map(.legendary) == .SSR`
- switch 必须 exhaustive（不写 default）—— 未来 RewardRarity 加新 case 时编译失败强制更新本 mapper

### AC2 — `RewardPopupView` 纯展示 SwiftUI 组件

**新建文件**：

- `iphone/PetApp/Features/Home/Views/RewardPopupView.swift`

**契约（接口 + 视觉锚）**：

```swift
// RewardPopupView.swift
// Story 21.4 AC2: 开箱奖励弹窗纯展示组件（不持 ViewModel；与 JoinRoomModal 同精神）.
//
// 视觉锚（与 ChestCardView Card / JoinRoomModal 风格对齐 + spec AC 行 3111-3115）:
//   1. AsyncImage 加载 reward.iconUrl (cosmetic 图标) → 居中 96pt × 96pt
//      - phase.empty / phase.failure → 灰色 SF Symbol "questionmark.app.dashed" 占位 (96pt)
//      - 与 EmojiPanelView AsyncImage 同模式 (lesson EmojiPanelView line 77-90)
//   2. Text("获得 \(reward.name)") + 18pt heavy + theme.colors.ink (居中)
//   3. RarityTag(rarity: RewardRarityTagMapper.map(reward.rarity), width: 80, height: 6)
//      - 居中放在 name 下方; AC1 helper 派生
//   4. "确定" PrimaryButton(variant: .primary, fullWidth: true) → 调 onClose closure
//      - 关闭由 caller (HomeView) onClose 闭包内置 $pendingReward.wrappedValue = nil
//
// 包装容器:
//   - VStack(spacing: 18) 内含上述 4 元素
//   - .padding(24) + .background(theme.colors.surface) + .clipShape(RoundedRectangle(cornerRadius: 28))
//   - 整 view 挂 FadeInModifier (id = reward.id 让同 sheet 多次弹出仍触发动画重放;
//     spec AC 行 3115 "含淡入动画" 钦定; 配合 .sheet 默认 slide-up 动画形成 "滑入 + 淡入" 组合).
//
// **不**持 ViewModel —— 仅接受 reward 值 + onClose closure (与 JoinRoomModal pattern 一致).

import SwiftUI

public struct RewardPopupView: View {
    public let reward: ChestRewardSnapshot
    public let onClose: () -> Void

    @Environment(\.theme) private var theme

    public init(reward: ChestRewardSnapshot, onClose: @escaping () -> Void) {
        self.reward = reward
        self.onClose = onClose
    }

    public var body: some View {
        VStack(spacing: 18) {
            // 视觉锚 1: AsyncImage 96pt × 96pt
            iconView
            // 视觉锚 2: "获得 {name}"
            nameLabel
            // 视觉锚 3: RarityTag
            rarityBadge
            // 视觉锚 4: "确定" PrimaryButton
            confirmButton
        }
        .padding(24)
        .background(theme.colors.surface)
        .clipShape(RoundedRectangle(cornerRadius: 28))
        .fadeIn(id: AnyHashable(reward.id))
        .accessibilityIdentifier(AccessibilityID.RewardPopup.popup)
        .accessibilityElement(children: .contain)
    }

    // MARK: - 视觉锚 1: iconView (AsyncImage 96 × 96)

    @ViewBuilder
    private var iconView: some View {
        AsyncImage(url: URL(string: reward.iconUrl)) { phase in
            switch phase {
            case .empty:
                ProgressView()
                    .frame(width: 96, height: 96)
            case .success(let image):
                image
                    .resizable()
                    .aspectRatio(contentMode: .fit)
                    .frame(width: 96, height: 96)
            case .failure:
                Image(systemName: "questionmark.app.dashed")
                    .font(.system(size: 56, weight: .light))
                    .foregroundColor(theme.colors.inkSoft)
                    .frame(width: 96, height: 96)
            @unknown default:
                Image(systemName: "questionmark.app.dashed")
                    .font(.system(size: 56, weight: .light))
                    .foregroundColor(theme.colors.inkSoft)
                    .frame(width: 96, height: 96)
            }
        }
        .accessibilityIdentifier(AccessibilityID.RewardPopup.icon)
    }

    // MARK: - 视觉锚 2: nameLabel

    private var nameLabel: some View {
        Text("获得 \(reward.name)")
            .font(.system(size: 18, weight: .heavy))
            .foregroundColor(theme.colors.ink)
            .multilineTextAlignment(.center)
            .accessibilityIdentifier(AccessibilityID.RewardPopup.nameLabel)
    }

    // MARK: - 视觉锚 3: rarityBadge

    private var rarityBadge: some View {
        RarityTag(
            rarity: RewardRarityTagMapper.map(reward.rarity),
            width: 80,
            height: 6
        )
        // 注：RarityTag 内部已挂 `rarityTag_\(rarity.rawValue)` identifier (RarityTag.swift line 35);
        // 此处再挂 RewardPopup.rarityTag 作为外层定位锚（UITest 用 popup 内层级查 RarityTag 时更稳定）.
        .accessibilityIdentifier(AccessibilityID.RewardPopup.rarityTag)
    }

    // MARK: - 视觉锚 4: confirmButton

    private var confirmButton: some View {
        PrimaryButton(
            title: "确定",
            variant: .primary,
            fullWidth: true,
            action: onClose
        )
        .accessibilityIdentifier(AccessibilityID.RewardPopup.confirmButton)
    }
}

// MARK: - Preview (双主题 + 4 品质抽样)

#if DEBUG
private struct RewardPopupPreview_Sample: View {
    @Environment(\.theme) private var theme

    var body: some View {
        VStack(spacing: 20) {
            RewardPopupView(
                reward: ChestRewardSnapshot(
                    cosmeticItemId: "1001",
                    name: "星星围巾",
                    slot: 2,
                    rarity: .rare,
                    assetUrl: "https://placehold.co/96x96?text=Scarf",
                    iconUrl: "https://placehold.co/96x96?text=Scarf"
                ),
                onClose: {}
            )
            RewardPopupView(
                reward: ChestRewardSnapshot(
                    cosmeticItemId: "1002",
                    name: "神秘王冠",
                    slot: 1,
                    rarity: .legendary,
                    assetUrl: "https://placehold.co/96x96?text=Crown",
                    iconUrl: "https://placehold.co/96x96?text=Crown"
                ),
                onClose: {}
            )
        }
        .padding(16)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(theme.colors.pageBg)
    }
}

#Preview("RewardPopup — candy") {
    RewardPopupPreview_Sample()
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("RewardPopup — dark") {
    RewardPopupPreview_Sample()
        .environment(\.theme, ThemeName.dark.theme)
}
#endif
```

**视觉契约表**（与 21.1 ChestCardView 视觉契约表同模式）：

| 锚点 | 视觉 | 数据来源 | a11y identifier |
|------|------|----------|-----------------|
| icon | AsyncImage 96 × 96，加载中 ProgressView，失败 `questionmark.app.dashed` SF Symbol | `reward.iconUrl` | `RewardPopup.icon` |
| nameLabel | Text "获得 {name}"，18pt heavy，theme.colors.ink，居中 | `reward.name` | `RewardPopup.nameLabel` |
| rarityBadge | `RarityTag(rarity:width:80,height:6)` | `RewardRarityTagMapper.map(reward.rarity)` | `RewardPopup.rarityTag` |
| confirmButton | `PrimaryButton(title:"确定",variant:.primary,fullWidth:true)` | onClose closure | `RewardPopup.confirmButton` |
| popup 容器 | VStack(spacing:18) + padding 24 + theme.colors.surface 背景 + 28pt 圆角 + FadeIn | — | `RewardPopup.popup` |

### AC3 — `AccessibilityID.RewardPopup` nested enum 新增

**改动文件**：

- `iphone/PetApp/Shared/Constants/AccessibilityID.swift`

**新增内容**（与既有 JoinRoomModal / Wardrobe / Profile 同模式）：

```swift
/// Story 21.4 落地的 RewardPopupView a11y identifier.
public enum RewardPopup {
    public static let popup = "rewardPopup"
    public static let icon = "rewardPopup_icon"
    public static let nameLabel = "rewardPopup_nameLabel"
    public static let rarityTag = "rewardPopup_rarityTag"
    public static let confirmButton = "rewardPopup_confirmButton"
}
```

**命名风格**：`rewardPopup` / `rewardPopup_<element>` —— 与 JoinRoomModal `joinRoomModal` / `joinRoom<Element>Button` 风格一致；popup 根容器单字段名（`rewardPopup`），子元素带下划线前缀（`rewardPopup_icon`）.

### AC4 — `HomeView` 内 `.sheet(item: $state.pendingReward)` 装配 + onDismiss 防双触发

**改动文件**：

- `iphone/PetApp/Features/Home/Views/HomeView.swift`

**装配点**：紧跟在既有 `.sheet(isPresented: $state.showJoinModal)` modifier 之后（HomeView.swift 行 102-126 块结尾）.

**新增 modifier**（伪代码）：

```swift
// HomeView.swift (在既有 .sheet(isPresented: $state.showJoinModal) 之后)
.sheet(
    item: $state.pendingReward,
    onDismiss: {
        // dismissReason mode (与 lesson `2026-05-02-sheet-onDismiss-fires-on-button-close-too.md` 同精神).
        // 本 story 仅有 confirm + swipe-dismiss 两路径，行为同义（都清 pendingReward），但仍走
        // dismissReason tag 模式，让未来加 "立即穿戴 / 再开一次" 按钮时不会引入双触发 bug.
        rewardDismissReason = nil   // 暂留接缝；本 story 不使用差异化分支
    }
) { reward in
    RewardPopupView(
        reward: reward,
        onClose: {
            // 用户点确定 → 关闭 sheet → SwiftUI item: optional binding 模式自动让 $state.pendingReward.wrappedValue = nil
            // 不直接写 state.pendingReward = nil（item: 模式契约：caller 通过 closure 通信，SwiftUI 自管 binding）.
            // 但 SwiftUI 标准做法是显式 set nil，让意图清晰 + 测试容易断言.
            state.pendingReward = nil
        }
    )
    .presentationDetents([.medium])
    .presentationCornerRadius(28)
}
```

**新增 @State**（HomeView 内部，与 `joinRoomInput` 同位置；HomeView.swift 行 55）:

```swift
/// Story 21.4 AC4: RewardPopup sheet onDismiss 防双触发 reason tag.
/// 本 story 仅 confirm + swipe-dismiss 两路径行为同义，但走 dismissReason 模式留接缝
/// (lesson `2026-05-02-sheet-onDismiss-fires-on-button-close-too.md`).
@State private var rewardDismissReason: RewardDismissReason?
```

**新增 enum**（HomeView 文件内 private; 与 `JoinRoomModal` DismissReason 同精神）:

```swift
private enum RewardDismissReason {
    case confirm    // 用户点 "确定" 按钮关闭
    // nil = swipe-dismiss / 编程关闭
}
```

> **设计选择说明**（与 lesson 2026-05-02 对照）：
> - 本 story 弹窗仅有 "确定" 按钮 + swipe-dismiss 两路径，两者行为同义（都清 pendingReward = nil 由 SwiftUI binding 自动负责） → **不需要** dismissReason switch 派发到不同 ViewModel method.
> - 但仍引入 `RewardDismissReason` enum + @State 字段：
>   - 留接缝防未来加 "立即穿戴" / "立即再开" / "分享" 按钮时引入差异化分支（按 lesson 钦定的"不可凭直觉假设 onDismiss 触发条件 → 必须用显式 reason tag 派发"原则预防性应用）
>   - 维持代码风格与 ProfileScaffoldView round 4 修法（lesson `2026-05-02`）一致
> - `.sheet(item:)` 模式与 `.sheet(isPresented:)` 模式略不同：item: optional → nil 模式由 SwiftUI 框架自动负责 binding 双向更新；caller 端 `state.pendingReward = nil` 在 onClose closure 内显式写，让意图清晰 + 单测可断言 closure 被调.

**重要不变量**（onDismiss 写空 reason tag 而非 `state.pendingReward = nil`）：
- onClose closure 已显式 set `state.pendingReward = nil`（按钮路径）；
- SwiftUI item: binding 在用户 swipe-dismiss 时自动 set nil（框架契约）；
- onDismiss 闭包**不**再调 `state.pendingReward = nil` —— 否则按钮路径会双调（lesson `2026-05-02` 的反例 1）.

### AC5 — 节点 7 不入仓不变量保持

**红线**（spec AC 行 3116 钦定）：

- 弹窗显示后**不更新仓库状态**（节点 8 / Story 23.5 才有仓库）；
- 本 story 不调任何 wardrobe / inventory / repository / API；
- pendingReward 写到 nil 后无任何 AppState mutation（不写 currentInventory / lastReward / etc）；
- 验证方式：
  - **静态**：本 story File List 内不应出现任何 `Repository` / `UseCase` / `Endpoints` / `AppState` 改动；File List 仅含 View / helper / a11y constants / 测试文件；
  - **运行时**：UITest case AC7 验证弹窗关闭后 ChestCardView 仍渲染 counting 态 + AppState 不变（除 ChestRefreshTriggerService 已 react 的 nextChest）.

### AC6 — 单元测试（≥3 case）

**新建文件**：

- `iphone/PetAppTests/Features/Home/Views/RewardRarityTagMapperTests.swift`
- `iphone/PetAppTests/Features/Home/ViewModels/HomeViewModelPendingRewardTests.swift`（**复用** 21.3 落地的 `RealHomeViewModelChestOpenTapTests` 已有 case；本 story 新增对 pendingReward = nil 行为的覆盖）

**RewardRarityTagMapperTests 覆盖**（≥4 case；纯函数单测）：

1. `testMapCommonReturnsN` — `RewardRarityTagMapper.map(.common) == .N`
2. `testMapRareReturnsR` — `.rare → .R`
3. `testMapEpicReturnsSR` — `.epic → .SR`
4. `testMapLegendaryReturnsSSR` — `.legendary → .SSR`

**HomeViewModelPendingRewardTests 覆盖**（≥2 case）：

5. `testPendingRewardSetToNilAfterCloseClosureCalled` — 模拟开箱成功 → `vm.pendingReward = snapshot` → 调 onClose closure（手动模拟 sheet 关闭路径）→ 验证 `vm.pendingReward == nil`
6. `testPendingRewardInitiallyNil` — 新建 HomeViewModel → `vm.pendingReward == nil`（防御性）

> **设计选择说明**：本 story 不直接测试 RewardPopupView 渲染（ADR-0002 §3.1 禁 SnapshotTesting / ViewInspector），通过纯函数 mapper 单测 + UITest 视觉断言（AC7）+ ViewModel state 单测三层覆盖.

### AC7 — UI 测试（≥1 case，XCUITest + mock server）

**新建文件**：

- `iphone/PetAppUITests/ChestOpenRewardPopupUITests.swift`

**实装策略**（与 ChestOpenUITests / ChestRefreshUITests 同模式）：

- 复用 `UITEST_SKIP_GUEST_LOGIN=1` + `UITEST_MOCK_CHEST_OPEN=1`（21.3 已落地 UITestMockChestRepository 注入路径）；
- UITestMockChestRepository.openChest 已在 21.3 落地返回固定 reward（rarity=1 common / cosmeticItemId="2001" / name="星星围巾" / iconUrl="https://example.com/..."）—— 本 story 直接复用，不改 mock；
- 测试场景：
  1. launch（unlockable 态预置）
  2. tap `home_chestOpenButton` 开宝箱
  3. 等 `rewardPopup` 出现（waitForExistence）
  4. 验证 `rewardPopup_icon` / `rewardPopup_nameLabel` / `rewardPopup_rarityTag` / `rewardPopup_confirmButton` 4 个 a11y identifier 都存在
  5. 验证 `rewardPopup_nameLabel` 文本含 "获得"（不固定 "星星围巾" 防 mock data 更新破测）
  6. tap `rewardPopup_confirmButton`
  7. 等 `rewardPopup` 消失（waitForNonExistence / `XCTAssertFalse(rewardPopup.exists)` after some timeout）
  8. 验证 chestCard_counting 仍存在（开箱后 nextChest counting 不被弹窗关闭事件影响）

**测试范围限定**：

- **不**测 RarityTag 颜色（视觉断言 SwiftUI 渲染颜色不在 XCTest 能力范围内；mapper 单测覆盖 enum 转换；颜色由 RarityTag.swift 内部硬编码 hex 配色，节点 7 阶段稳定）；
- **不**测 swipe-dismiss 路径（节点 7 阶段确定按钮路径已足够；swipe-dismiss 是 SwiftUI 框架行为，不引入 ViewModel mutation）—— 但 SwiftUI item: binding 模式契约保证 swipe-dismiss 也走 $pendingReward = nil 自动 binding，与确定按钮路径行为等价；
- **不**测 FadeIn 动画时长（Primitive 已在 Story 37.6 落地 + 单独测试 FadeIn primitive；本 story 不重复测）.

### AC8 — Build verify + ios-simulator MCP UI 实跑

**跑通命令**：

1. `bash iphone/scripts/build.sh` → BUILD SUCCESS（0 error / 0 warning related to this story）
2. `bash iphone/scripts/build.sh --test` → 全单测 pass（含本 story 新增 ≥6 case）
3. ios-simulator MCP UI 实跑（iPhone 17 Pro sim）：
   - `bash iphone/scripts/build.sh` → 产出 PetApp.app
   - `install_app(app_path:...)` + `launch_app(bundle_id:"com.zhuming.pet.app",terminate_running:true,environment:{UITEST_SKIP_GUEST_LOGIN:"1",UITEST_MOCK_CHEST_OPEN:"1"})`
   - `ui_view` → 验首页 ChestCardView unlockable 态
   - `ui_tap` 触发开宝箱按钮（坐标查 ui_describe_all）
   - `ui_view` → 验 RewardPopupView 视觉（icon 96 × 96 + "获得 {name}" + 蓝色 / 灰色 / 紫色 / 金色 RarityTag + "确定" PrimaryButton）
   - `ui_tap` 触发 "确定" 按钮
   - `ui_view` → 验弹窗消失 + 首页 counting 态恢复
   - `ui_describe_all` → 抽样验 a11y tree 含 `rewardPopup` / `rewardPopup_icon` / `rewardPopup_nameLabel` / `rewardPopup_rarityTag` / `rewardPopup_confirmButton`

### AC9 — Deliverable 清单

**新建文件（生产 2 + 测试 3 = 5）**：

1. `iphone/PetApp/Features/Home/Views/RewardPopupView.swift`
2. `iphone/PetApp/Features/Home/Views/RewardRarityTagMapper.swift`
3. `iphone/PetAppTests/Features/Home/Views/RewardRarityTagMapperTests.swift`
4. `iphone/PetAppTests/Features/Home/ViewModels/HomeViewModelPendingRewardTests.swift`
5. `iphone/PetAppUITests/ChestOpenRewardPopupUITests.swift`

**改动文件（生产 2）**：

6. `iphone/PetApp/Shared/Constants/AccessibilityID.swift`（新增 `RewardPopup` nested enum）
7. `iphone/PetApp/Features/Home/Views/HomeView.swift`（追加 `.sheet(item: $state.pendingReward)` modifier + `RewardDismissReason` enum + `rewardDismissReason` @State）

**不修改**（红线静态校验）：

- ChestRewardSnapshot.swift（21.3 冻结）
- HomeViewModel.swift（21.3 已声明 pendingReward 字段）
- RealHomeViewModel.swift（21.3 已落地 OpenChestUseCase write 路径）
- MockHomeViewModel.swift（21.3 已 override）
- OpenChestUseCase.swift（21.3 冻结）
- ChestCardView.swift / HomeContainerView.swift（21.1 / 21.3 落地）
- AppContainer.swift / RootView.swift（不需要新 factory；HomeView sheet 装配纯 view 层）
- 任何 Repository / Endpoints / Models（节点 7 不入仓红线）
- 任何 server 文件 / ios/ 目录文件

## Tasks / Subtasks

- [ ] **Task 1: AccessibilityID.RewardPopup nested enum（AC3）**
  - [ ] 1.1 改 `iphone/PetApp/Shared/Constants/AccessibilityID.swift` —— 新增 `public enum RewardPopup` 含 5 个常量（popup / icon / nameLabel / rarityTag / confirmButton），位置紧跟在 `JoinRoomModal` enum 之后

- [ ] **Task 2: RewardRarityTagMapper 纯函数 helper（AC1）**
  - [ ] 2.1 新建 `iphone/PetApp/Features/Home/Views/RewardRarityTagMapper.swift` —— enum + `static func map(_:) -> Rarity`，switch exhaustive 不写 default

- [ ] **Task 3: RewardPopupView 纯展示组件（AC2）**
  - [ ] 3.1 新建 `iphone/PetApp/Features/Home/Views/RewardPopupView.swift` —— 4 视觉锚（icon / nameLabel / rarityBadge / confirmButton）+ VStack(spacing:18) + padding 24 + theme.colors.surface 背景 + 28pt 圆角 + `.fadeIn(id: AnyHashable(reward.id))`
  - [ ] 3.2 添加 `AsyncImage` phase 处理（empty → ProgressView / success → resizable / failure → questionmark.app.dashed SF Symbol）
  - [ ] 3.3 5 个 a11y identifier 全部挂上（popup 根容器 + 4 子元素）+ `.accessibilityElement(children: .contain)` 让子元素仍可独立定位
  - [ ] 3.4 添加 #if DEBUG #Preview 块（candy / dark 双主题 + rare / legendary 双品质抽样）

- [ ] **Task 4: HomeView sheet 装配 + onDismiss 防双触发（AC4）**
  - [ ] 4.1 改 `iphone/PetApp/Features/Home/Views/HomeView.swift` —— 文件顶部 private enum 区域加 `private enum RewardDismissReason { case confirm }`
  - [ ] 4.2 改 HomeView struct 内 `@State` 字段区追加 `@State private var rewardDismissReason: RewardDismissReason?`
  - [ ] 4.3 在既有 `.sheet(isPresented: $state.showJoinModal)` modifier 之后追加新 `.sheet(item: $state.pendingReward)` modifier：onDismiss 闭包仅 reset `rewardDismissReason = nil`（不调 ViewModel method）+ content closure 内 RewardPopupView + onClose 写 `state.pendingReward = nil`
  - [ ] 4.4 验证 `presentationDetents([.medium])` + `presentationCornerRadius(28)` 与 JoinRoomModal 同模式

- [ ] **Task 5: 节点 7 不入仓不变量保持（AC5）**
  - [ ] 5.1 静态校验：本 story File List 不含任何 Repository / Endpoints / Models / AppState 改动
  - [ ] 5.2 grep 校验：`grep -r "inventory\|wardrobe\|Inventory\|Wardrobe" iphone/PetApp/Features/Home/Views/RewardPopupView.swift` 无命中（弹窗与仓库零耦合）

- [ ] **Task 6: 单元测试（AC6，≥6 case）**
  - [ ] 6.1 新建 `iphone/PetAppTests/Features/Home/Views/RewardRarityTagMapperTests.swift` —— 4 case 覆盖 RewardRarity 4 档映射
  - [ ] 6.2 新建 `iphone/PetAppTests/Features/Home/ViewModels/HomeViewModelPendingRewardTests.swift` —— 2 case（pendingReward 初始 nil + onClose 后 nil）

- [ ] **Task 7: UI 测试（AC7，≥1 case）**
  - [ ] 7.1 新建 `iphone/PetAppUITests/ChestOpenRewardPopupUITests.swift` —— 1 case：launch unlockable 态 → tap 开宝箱 → wait rewardPopup → 验 4 子 identifier → tap confirmButton → wait rewardPopup 消失 → 验 chestCard_counting 仍在

- [ ] **Task 8: Build + 模拟器实跑（AC8）**
  - [ ] 8.1 跑 `bash iphone/scripts/build.sh` 确认 build pass
  - [ ] 8.2 跑 `bash iphone/scripts/build.sh --test` 确认全单测 pass（含本 story 新增 ≥6 case）
  - [ ] 8.3 ios-simulator MCP UI 实跑：install_app + launch_app(UITEST_SKIP_GUEST_LOGIN=1, UITEST_MOCK_CHEST_OPEN=1) + ui_view 验 unlockable + ui_tap chestOpenButton → 验 RewardPopupView 视觉 + ui_tap confirmButton → 验弹窗消失 + counting 态恢复

- [ ] **Task 9: Deliverable 清单核对（AC9）**
  - [ ] 9.1 核对 7 文件新增/改动（实际：新建 5 + 改动 2 = 7）—— 本 story 不夹带其它无关改动

## Dev Notes

### 架构对齐

- **iOS 工程结构（`docs/宠物互动App_iOS客户端工程结构与模块职责设计.md`）**：本 story 落地在 `Features/Home/Views/` 内（与 ChestCardView / HomeView 同位置）；helper `RewardRarityTagMapper` 也落 `Features/Home/Views/`（与 HomeRoomDispatcher / HomePetNameResolver 同 strategy —— "view 派生规则与 view 同位置，让 view 内能直接引用"）.
- **ADR-0009 §3.3 sheet 白名单**：奖励弹窗（开箱）已列入次级场景白名单（行 116）—— 本 story 用 `.sheet(item:)` 模式装配，符合 §3.3 + §3.4 钦定路径.
- **ADR-0010 §3.2**：`pendingReward` 已在 21.3 落地为 HomeViewModel @Published（transient view-state；不上 AppState；"toast / popup state 归 ViewModel" 钦定）—— 本 story 直接拿来 sheet binding，**不**新增 AppState 字段.
- **ADR-0002 §3.1**：XCTest only；本 story 单测 = mapper 纯函数 4 case + ViewModel state 2 case；视觉断言走 UITest（XCUITest）+ ios-simulator MCP UI 实跑；不引 SnapshotTesting / ViewInspector.

### sheet 模式选择（`.sheet(item:)` vs `.sheet(isPresented:)`）

- **`.sheet(item: $optional)` 模式**（本 story 采用）：
  - SwiftUI 框架自动负责 binding 双向 nil ↔ non-nil（content closure 拿到 unwrapped 值）
  - swipe-dismiss / 编程 reset → SwiftUI 自动 set `wrappedValue = nil`
  - 适合 "wrapper data → view" 场景（如 reward / detail data → modal）
- **`.sheet(isPresented: $bool)` 模式**（JoinRoomModal 采用）：
  - 需要单独 Bool flag + content closure 内引用其他 state 拿数据
  - 适合 "trigger only" 场景（如 "用户主动点开 modal"，无附加数据）
- **本场景选 item:**：reward 是必须传给 view 的值；item: 模式自然映射 + 框架自动 nil/non-nil 切换更简洁.

### sheet onDismiss 防双触发（lesson `2026-05-02-sheet-onDismiss-fires-on-button-close-too.md` 钦定路径）

- lesson 钦定 `.sheet(onDismiss:)` 闭包在**所有** disappear 路径都会 fire（按钮 / swipe / 编程 binding 改 nil）；
- 反例 1：按钮闭包 `state.pendingReward = nil` + onDismiss 闭包 `state.pendingReward = nil` → 按钮路径双触发；
- 反例 2：按钮闭包 `state.onReward()` + onDismiss 闭包 `state.onDismiss()` → 按钮路径错触发 onDismiss method；
- **本 story 防御**：
  - 按钮路径（onClose closure 内）显式 set `state.pendingReward = nil` —— 这是按钮意图的唯一 ViewModel mutation；
  - onDismiss 闭包内**不**调 `state.pendingReward = nil`（防止按钮路径双触发；item: binding 已自动 nil 化 swipe-dismiss 路径）；
  - 引入 `rewardDismissReason: RewardDismissReason?` @State 字段 + onDismiss reset 它 → 留接缝防未来加 "立即穿戴 / 再开" 按钮时按 lesson 钦定路径派发；
- 单测在本 story 不直接覆盖 dismissReason 派发（仅 1 个 confirm path 无差异化）—— 但 lesson 钦定的"显式 reason tag 派发"风格保留.

### `.fadeIn(id:)` 使用模式

- Story 37.6 落地的 `FadeInModifier` 走 0.28s easeInOut + offsetY +8 → 0；
- `id` 参数：变化时重触发动画（基于 `.id(id)` 重建子树 + onChange 显式 reset visible）；
- **本 story 传 `AnyHashable(reward.id)`**：理由：(a) reward.id = cosmeticItemId（21.3 ChestRewardSnapshot.id 声明）；(b) 同一 sheet 多次弹出时（如未来 "立即再开一次" 按钮场景）若新 reward 是不同 cosmeticItemId → id 变化触发 fadeIn 重放；(c) 同 cosmeticItemId 再次弹出场景节点 7 阶段不可达（弹窗关闭后 sheet 整个销毁，下次弹出是新的 SwiftUI view 实例 → 自动 onAppear 触发 fadeIn），id 变化非必需但仍为防御性留. 详 lesson `2026-04-30-swiftui-state-survives-id-and-shadow-over-children.md` + `2026-04-30-swiftui-explicit-id-nil-shared-identity.md`.
- **不**传 nil id：与 lesson `2026-04-30-swiftui-explicit-id-nil-shared-identity.md` 钦定路径一致（默认参 nil 让多 fadeIn() sibling 共享 nil identity → state retention bug；本 story 显式传 reward.id 避开）.

### 21.3 接缝（pendingReward 写入路径）

- 21.3 `RealHomeViewModel.onChestOpenTap` 已落地：`OpenChestUseCase.execute()` 成功 → `self.pendingReward = snapshot`（main actor 写入）；
- 本 story 在 HomeView 端 `.sheet(item: $state.pendingReward)` 订阅 —— 当 21.3 写入 non-nil 时 SwiftUI 自动弹 sheet，content closure 拿到 unwrapped snapshot；
- 关闭路径 `onClose: { state.pendingReward = nil }` —— 用户点确定后写 nil，sheet 自动消失.
- **不变量**：21.3 OpenChestUseCase 在 nextChest counting 写入 AppState 之后才 set pendingReward（同 main actor block），所以 sheet 弹出时 ChestTimerDriver 已 react nextChest，背景的 ChestCardView 已切回 counting 态 —— 用户感知到的"弹窗弹出"过程中卡片已恢复倒计时（视觉合理）.

### Story 33.5 / 22.1 入位

- **Story 22.1 节点 7 demo 验收**：跨端集成测试场景 4 "dev grant-steps + 开箱" 钦定弹窗视觉断言（含 cosmetic icon + name + rarity 徽章）—— 本 story 视觉契约（AC2 表）+ a11y identifier（AC3）让 22.1 测试直接通过 `app.descendants["rewardPopup"]` + `app.staticTexts["rewardPopup_nameLabel"]` 等定位.
- **Story 33.5 合成奖励弹窗**：epics.md 行 4322 钦定 ComposeRewardPopupView "复用 Story 21.4 RewardPopupView 结构"：
  - **本 story 不预先抽公共 base**（YAGNI）—— 待 Story 33.5 落地时若需要 "合成成功！获得 {name}" + 高一阶品质徽章 + 视觉差异化，节点 11 时根据合成弹窗具体差异决定是抽 `RewardPopupBaseView` 还是直接复用 `RewardPopupView` + 加 prefix prop.

### isOpening + pendingReward 状态机协同

- 21.3 落地的状态机：`isOpening = true → execute → set pendingReward → defer isOpening = false`；
- 本 story sheet driver 是 pendingReward；
- 视觉时序：
  1. 用户点开宝箱按钮 → isOpening = true → ChestCardView "可开启" 切 "开箱中…" + 按钮 disabled + ProgressView
  2. UseCase.execute 成功 → 写 AppState.currentChest (nextChest counting) + 写 pendingReward = snapshot → defer isOpening = false
  3. SwiftUI 检测 currentChest 变 → ChestCardView 重新 render counting 态；同时检测 pendingReward 变 non-nil → sheet 弹出
  4. 用户看到："首页卡片已切回 counting + 弹窗滑入 + FadeIn"
  5. 用户点 "确定" → onClose 调 `state.pendingReward = nil` → sheet 消失
  6. 用户回到首页 counting 态（继续倒计时）

### 不引入 "立即再开一次" / "立即穿戴" 等额外 CTA

- spec AC 行 3114 仅钦定 "确定按钮关闭弹窗" —— 没有其他动作；
- 节点 7 不入仓，所以 "立即穿戴" 在节点 7 不可达；
- "立即再开一次" 涉及 idempotencyKey 新生成 + UseCase 重入 → 节点 7 阶段不引入（lesson `2026-05-14-idempotency-atomic-claim-and-rate-limit-honesty.md` 钦定 "客户端连续点击应由 ViewModel 状态机管控"，从弹窗内"立即再开"绕过 ChestCardView 重入防御层是反例）；
- 未来如有需求可在 Story 33.5+ 或独立 story 加.

### AsyncImage 失败 fallback 选 `questionmark.app.dashed` 而非 `questionmark.circle`

- `questionmark.circle` 已被 EmojiPanelView 用于 emoji 失败 fallback（EmojiPanelView.swift line 84）；
- 选 `questionmark.app.dashed` 让 cosmetic 失败 fallback 视觉与 emoji 区分（dashed 是 "盒子边框虚线" 视觉，与 ChestCardView 内 `Icons.symbol(for: "box")` 同主题 → cosmetic 是装扮道具，视觉关联 box）；
- SF Symbol 配色用 `theme.colors.inkSoft`（与 ChestCardView counting 态 box icon 同色，统一灰阶视觉）.

### 测试边界

- **RewardPopupView 单测不直接断言 SwiftUI 渲染**（ADR-0002 §3.1 禁 ViewInspector）→ 视觉断言通过 (a) 纯函数 mapper 单测 (AC1 → mapper map 表 4 case) (b) UITest a11y identifier 抽样验证 (AC7) (c) ios-simulator MCP UI 实跑 (AC8 步骤 3) 三层防御；
- **HomeViewModelPendingRewardTests 仅 2 case 防御性覆盖**：21.3 落地的 `RealHomeViewModelChestOpenTapTests` 已覆盖 OpenChestUseCase 成功路径写入 pendingReward —— 本 story 不重复 21.3 已覆盖的 case；新加 2 case 仅覆盖 "pendingReward 初始 nil" + "onClose 回调后变 nil" 两个独立 state 不变量；
- **UITest 范围**：tap 开宝箱 → wait popup → 验 4 a11y identifier → tap confirm → wait popup 消失 → 验 counting 态仍在；不验 swipe-dismiss / 颜色 / 动画时长（前两个 SwiftUI 框架行为，后一个 Primitive 单独测）.

### 命名规则

- **`RewardPopupView` vs `ChestRewardPopupView`**：选 `RewardPopupView`（无 Chest 前缀）—— 理由：(a) Story 33.5 合成奖励弹窗 "复用 Story 21.4 RewardPopupView 结构"，若本 story 命名 ChestRewardPopupView 则未来抽公共 base 时还要 rename；(b) reward 是 cosmetic 域抽象（不是 chest 特定的），命名"奖励"更广义；(c) ChestRewardSnapshot 是 21.3 落地的 wire-level 命名（强调"开箱"来源），view 层命名"奖励弹窗"更视觉化.
- **`RewardRarityTagMapper`**：选 `_Mapper` 后缀（与 MotionStateMapper 同精神，非 `_Dispatcher` 因为不是 dispatch / 决策语义，只是 type → type 转换）.
- **`AccessibilityID.RewardPopup`** vs `ChestRewardPopup`：与命名规则同源 —— 视觉层命名"奖励弹窗"，nested enum 名 `RewardPopup` 让未来合成奖励弹窗也能复用同 enum（如加 `RewardPopup.composeConfirmButton` 等）.
- **a11y identifier `rewardPopup_<element>`**：与 JoinRoomModal `joinRoom<Element>Button` 风格略不同（join 用 camelCase 拼接，reward 用 underline 分隔）—— 选 underline 与 Home enum 内 `home_chestOpenButton` / `home_petArea` 风格一致（Home enum 与 RewardPopup enum 都在"内嵌于 HomeView 的 a11y 锚"语义内，统一 underline 模式）.

### Project Structure Notes

- **对齐 `iphone` 工程结构（`docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §3）**：
  - View（弹窗本身）→ `Features/Home/Views/RewardPopupView.swift`（与 ChestCardView 同位置）
  - View helper（mapper）→ `Features/Home/Views/RewardRarityTagMapper.swift`（与 HomeRoomDispatcher 同位置）
  - a11y constants → `Shared/Constants/AccessibilityID.swift` 内新增 nested enum（与 JoinRoomModal / Wardrobe 等 7 个 nested enum 同模式）
  - 测试镜像生产路径 → `PetAppTests/Features/Home/{Views,ViewModels}/` + UITest 在 `PetAppUITests/`（与 ChestRefreshUITests / RoomUITests 同位置）
- **xcodegen 同步**：iphone target sources 用 glob recursive，无需手动编辑 project.yml；新建文件落入正确目录后跑 `xcodegen generate` 即可.
- **`AccessibilityID.swift` 双 target 编译**：本文件通过 `project.yml line 67-69` 同时编进 PetApp + PetAppUITests 两个 target；本 story 新增的 `RewardPopup` enum 内常量同样自动跨两 target 可用（UITest 直接引用 `AccessibilityID.RewardPopup.popup` 字面量）.

### References

- [Source: docs/宠物互动App_V1接口设计.md#7.2 POST /api/v1/chest/open] —— ChestRewardDTO 字段表（cosmeticItemId / name / slot / rarity / assetUrl / iconUrl）
- [Source: docs/宠物互动App_V1接口设计.md#6.9] —— rarity 4 档枚举（1=common / 2=rare / 3=epic / 4=legendary）
- [Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md#3 五分层] —— Feature 内子目录（Views）
- [Source: _bmad-output/implementation-artifacts/decisions/0009-iphone-navigation-tabview.md#3.3] —— Sheet 白名单含"奖励弹窗（开箱）"行 116
- [Source: _bmad-output/implementation-artifacts/decisions/0010-app-state.md#3.2] —— popup state 归 ViewModel @Published（不上 AppState）
- [Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.1] —— XCTest only，禁止 SnapshotTesting / ViewInspector
- [Source: _bmad-output/planning-artifacts/epics.md#3093-3121] —— Story 21.4 AC 原文（含 2026-05-04 addendum 钦定 Primitives 复用 + sheet 次级路由白名单）
- [Source: _bmad-output/planning-artifacts/epics.md#4322] —— Story 33.5 "复用 Story 21.4 RewardPopupView 结构" 钦定
- [Source: _bmad-output/implementation-artifacts/21-3-开箱按钮-调用-post-chest-open.md] —— Story 21.3 落地的 ChestRewardSnapshot / pendingReward / OpenChestUseCase 写入路径
- [Source: _bmad-output/implementation-artifacts/21-1-首页宝箱组件-swiftui.md] —— Story 21.1 落地的 ChestCardView 视觉契约 + a11y identifier 命名风格
- [Source: iphone/PetApp/Features/Home/Models/ChestRewardSnapshot.swift] —— 21.3 落地的 Identifiable + RewardRarity enum
- [Source: iphone/PetApp/Features/Home/ViewModels/HomeViewModel.swift#pendingReward] —— 21.3 落地的 @Published pendingReward 字段
- [Source: iphone/PetApp/Features/Home/Views/HomeView.swift#sheet(isPresented:)] —— JoinRoomModal sheet 装配范例（本 story 紧跟其后追加新 sheet）
- [Source: iphone/PetApp/Shared/Modals/JoinRoomModal.swift] —— 纯展示组件接口设计范例（init + Binding / closure 参数 + 不持 ViewModel）
- [Source: iphone/PetApp/Shared/Constants/AccessibilityID.swift#JoinRoomModal] —— nested enum 命名模式范例
- [Source: iphone/PetApp/Core/DesignSystem/Primitives/RarityTag.swift] —— Story 37.6 落地的 Rarity enum (N/R/SR/SSR) + 配色
- [Source: iphone/PetApp/Core/DesignSystem/Primitives/PrimaryButton.swift] —— Story 37.6 落地的 PrimaryButton variant + fullWidth
- [Source: iphone/PetApp/Core/DesignSystem/Primitives/FadeIn.swift] —— Story 37.6 落地的 FadeInModifier（0.28s + offsetY +8 → 0）
- [Source: iphone/PetApp/Features/Emoji/Views/EmojiPanelView.swift#AsyncImage] —— AsyncImage phase 处理范例
- [Source: iphone/PetAppUITests/ChestOpenUITests.swift] —— Story 21.3 落地的 UITest 范例（UITEST_SKIP_GUEST_LOGIN + UITEST_MOCK_CHEST_OPEN 路径）
- [Source: docs/lessons/2026-05-02-sheet-onDismiss-fires-on-button-close-too.md] —— sheet onDismiss 防双触发 dismissReason 模式
- [Source: docs/lessons/2026-04-30-swiftui-explicit-id-nil-shared-identity.md] —— fadeIn(id:) 显式传 reward.id 防 nil 共享 identity
- [Source: docs/lessons/2026-04-30-swiftui-state-survives-id-and-shadow-over-children.md] —— FadeInModifier onChange(of:id) 重触发动画
- [Source: docs/lessons/2026-04-26-swiftui-switch-transition-explicit.md] —— sheet content swap 显式 .transition 模式
- [Source: docs/lessons/2026-05-11-business-error-fallback-must-forward-original-12-7-r8.md] —— ViewModel 业务错误码不写仓库（本 story 红线参照）
- [Source: docs/lessons/2026-05-14-idempotency-atomic-claim-and-rate-limit-honesty.md] —— 客户端重入防御应在 ViewModel 状态机（本 story 不引入 "立即再开" 按钮的理由）
- [Source: CLAUDE.md#iOS UI 验证] —— ios-simulator MCP 必跑 verify workflow

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) via bmad-dev-story workflow.

### Debug Log References

- `bash iphone/scripts/build.sh --test` → BUILD SUCCESS + 727 tests / 0 failures（含本 story 新增 6 case）
- `bash iphone/scripts/build.sh` → BUILD SUCCESS（simulator app 产出 → ios-simulator MCP 实跑）

### Completion Notes List

- **AC1–AC9 全部满足**。生产文件 RewardPopupView.swift / RewardRarityTagMapper.swift 与 spec 契约逐行一致；AccessibilityID.RewardPopup nested enum + HomeView `.sheet(item: $state.pendingReward)` + `RewardDismissReason` enum + `rewardDismissReason` @State 均按 AC4 钦定 onDismiss 防双触发模式落地（onDismiss 仅 reset reason tag，不调 `state.pendingReward = nil`；按钮路径 onClose closure 显式 set nil）。
- **单测**：RewardRarityTagMapperTests 4 case（common→N / rare→R / epic→SR / legendary→SSR）+ HomeViewModelPendingRewardTests 2 case（初始 nil / onClose 后 nil）全 pass，与既有 725 case 合计 727 全绿。
- **UITest**：ChestOpenRewardPopupUITests 已纳入 PetAppUITests target（pbxproj 已注册）。
- **ios-simulator MCP UI 实跑（AC8 钦定 / 关键合规点）**：iPhone 17 sim 上 launch（UITEST_SKIP_GUEST_LOGIN=1 + UITEST_MOCK_CHEST_OPEN=1）→ ui_view 验 unlockable 态 → ui_tap 开宝箱 → ui_view 验 RewardPopupView 弹出（icon fallback `questionmark.app.dashed` + "获得 测试装扮" 18pt heavy + RarityTag 76.8×5.76 + "确定" 全宽 PrimaryButton）→ ui_describe_all 验 5 a11y 锚（rewardPopup 容器 + 4 子元素，`.accessibilityElement(children: .contain)` 模式）→ ui_tap "确定" → ui_view + ui_describe_all 验弹窗完全消失 + chestCard_counting（"宝箱倒计时 09:29"）恢复，无双触发 / 无重弹。AsyncImage fallback 触发是预期行为（mock iconUrl 为占位 URL，加载失败走 `.failure` 分支渲染 SF Symbol，与 AC2 视觉契约一致）。
- **节点 7 不入仓红线（AC5）守住**：File List 仅含 View / helper / a11y constants / 测试 / pbxproj，零 Repository / Endpoints / Models / UseCase / AppState 改动；运行时验证弹窗关闭后 chestCard_counting 仍在（AppState.currentChest 不被弹窗 dismiss 影响）。

### File List

**新建（生产 2）**：
- `iphone/PetApp/Features/Home/Views/RewardPopupView.swift`
- `iphone/PetApp/Features/Home/Views/RewardRarityTagMapper.swift`

**新建（测试 3）**：
- `iphone/PetAppTests/Features/Home/Views/RewardRarityTagMapperTests.swift`
- `iphone/PetAppTests/Features/Home/ViewModels/HomeViewModelPendingRewardTests.swift`
- `iphone/PetAppUITests/ChestOpenRewardPopupUITests.swift`

**改动（生产 2）**：
- `iphone/PetApp/Shared/Constants/AccessibilityID.swift`（新增 `RewardPopup` nested enum，5 常量）
- `iphone/PetApp/Features/Home/Views/HomeView.swift`（追加 `.sheet(item: $state.pendingReward)` modifier + `rewardDismissReason` @State + `RewardDismissReason` private enum）

**改动（工程文件 1）**：
- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen 同步新建 5 文件入 target；无其它无关改动）

## Change Log

| Date | Change | Reason |
|------|--------|--------|
| 2026-05-15 | 初次创建（bmad-create-story） | Epic 21 第 4 条 story；接 21.3 落地的 pendingReward 字段 + Story 37.6 共享 primitives 基础上加 RewardPopupView 闭环；为 Story 22.1 节点 7 demo 验收 / Story 33.5 合成奖励弹窗 留接缝 |
| 2026-05-16 | dev-story 实装完成 → Status review（bmad-dev-story） | AC1–AC9 全部满足；2 生产 view/helper + 3 测试新建 + AccessibilityID/HomeView/pbxproj 改动；单测 727/727 绿；ios-simulator MCP iPhone 17 实跑验证 RewardPopupView 弹出 → 确定关闭 → counting 恢复，无双触发；节点 7 不入仓红线守住 |
