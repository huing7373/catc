// WardrobeScaffoldView.swift
// Story 37.9 AC4: ui_design wardrobe.jsx 高保真衣柜界面 Scaffold（4 区块视觉 + 12+ a11y 锚 + #Preview 4 配置）.
//
// 关键设计：
//   - struct 非泛型（与 RoomScaffoldView 同精神：Wardrobe 无类似 chestSlot 接缝点；
//     未来 Story 33.1 加合成页 NavigationLink slot 时再走泛型 ViewBuilder 路径）
//   - `@ObservedObject var state: WardrobeViewModel` 基类直接（与 HomeView / RoomScaffoldView 同模式）
//   - 4 区块：topCard / previewCard / categoryTabs / grid
//   - selectedCategory / selectedCosmeticId 走 ViewModel @Published（不走 SwiftUI @State）—
//     单元测试需要直接断言派生 currentCategoryItems / activeItem 行为（ADR-0010 §3.2 +
//     story 37.9 Dev Notes "state owner 边界"钦定）.
//   - 12+ a11y identifier inline 字符串：wardrobeView / wardrobeDiamondCount / wardrobeComposeEntry /
//     wardrobeEquipButton / wardrobeCategory_<rawValue> / wardrobeItem_<id>；Story 37.13 a11y 总表归并
//   - 视觉规则：iphone/ui_design/source/screens/wardrobe.jsx + iphone/ui_design/README.md §WardrobeScreen
//
// Story 37.7 / 37.8 沉淀 lesson 预防性应用：
//   - 所有 Card / grid cell shadow 必须挂在 `RoundedRectangle.fill(...).shadow(...)` 那一层（不挂最外层 chain）—
//     按 Story 37.6 round 5 lesson `2026-04-30-swiftui-state-survives-id-and-shadow-over-children.md` 钦定路径.
//   - grid cell selected ring 用 `RoundedRectangle.stroke(theme.colors.accent, lineWidth: 2.5)` overlay,
//     而非 chain `.shadow(...)` 在最外层（避免 children Text/Icon 被 alpha 投影）.

import SwiftUI
import os.log

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

    // MARK: - 区块 1: topCard (wardrobe.jsx:38-51)

    /// 顶部 Card：左 VStack 收藏数 + "{猫名}的衣柜" / 右 HStack 钻石 pill + 合成按钮.
    private var topCard: some View {
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
                        .shadow(
                            color: theme.shadow.sm.color,
                            radius: theme.shadow.sm.radius,
                            x: theme.shadow.sm.x,
                            y: theme.shadow.sm.y
                        )
                )
                .overlay(RoundedRectangle(cornerRadius: 16).stroke(theme.colors.border, lineWidth: 1))
                .accessibilityIdentifier("wardrobeDiamondCount")

                // 合成按钮（占位 — Story 33.1 落地 NavigationLink push）.
                // 按 Story 37.9 spec 关键决策 2：内联 Button + os_log 而非进 ViewModel
                // （与 RoomScaffoldView 复制按钮 1.2s feedback 走 @State 同精神）.
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
                            .shadow(
                                color: theme.shadow.sm.color,
                                radius: theme.shadow.sm.radius,
                                x: theme.shadow.sm.x,
                                y: theme.shadow.sm.y
                            )
                    )
                }
                .accessibilityIdentifier("wardrobeComposeEntry")
            }
        }
        .padding(.top, 68)
        .padding(.horizontal, 20)
        .padding(.bottom, 8)
    }

    // MARK: - 区块 2: previewCard (wardrobe.jsx:54-104)

    /// 预览区 Card：左 cat 占位 / 右 active item 详情 + 装备按钮.
    private var previewCard: some View {
        HStack(spacing: 12) {
            // 左：cat 占位（140x140 灰底圆角矩形 + cat.fill SF Symbol 占位；Story 30.x 接真实 sprite 时升级）
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
                            .background(
                                RoundedRectangle(cornerRadius: 8)
                                    .fill(active.owned ? theme.colors.success : theme.colors.inkSoft)
                            )
                    }
                }
                equipButton
            }
            Spacer(minLength: 0)
        }
        .padding(14)
        .background(
            RoundedRectangle(cornerRadius: 24)
                .fill(LinearGradient(
                    colors: [theme.colors.accentSoft, theme.colors.surface],
                    startPoint: .top,
                    endPoint: .bottom
                ))
                .shadow(
                    color: theme.shadow.sm.color,
                    radius: theme.shadow.sm.radius,
                    x: theme.shadow.sm.x,
                    y: theme.shadow.sm.y
                )
        )
        .overlay(RoundedRectangle(cornerRadius: 24).stroke(theme.colors.border, lineWidth: 1))
        .padding(.horizontal, 20)
        .padding(.vertical, 4)
    }

    /// 装备/卸下按钮（PrimaryButton fullWidth；isEnabled = activeItem.owned；点击调 state.onEquipTap）.
    private var equipButton: some View {
        let active = state.activeItem
        let equippedNow: Bool = {
            guard let active else { return false }
            return state.isEquipped(active)
        }()
        let title: String = equippedNow ? "✓ 已装备 (点击卸下)" : "装备"
        let variant: PrimaryButtonVariant = equippedNow ? .secondary : .primary
        let isEnabled: Bool = active?.owned ?? false

        return PrimaryButton(
            title: title,
            variant: variant,
            fullWidth: true,
            isEnabled: isEnabled,
            action: {
                if let item = state.activeItem {
                    state.onEquipTap(item: item)
                }
            }
        )
        .accessibilityIdentifier("wardrobeEquipButton")
    }

    // MARK: - 区块 3: categoryTabs (wardrobe.jsx:107-124)

    /// 5 分类 Tab 横向滚动条.
    private var categoryTabs: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: 6) {
                ForEach(CosmeticCategory.allCases) { category in
                    categoryTabButton(category)
                }
            }
            .padding(.horizontal, 20)
        }
        .padding(.vertical, 6)
    }

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
                    .shadow(
                        color: isSelected ? theme.shadow.sm.color : .clear,
                        radius: theme.shadow.sm.radius,
                        x: theme.shadow.sm.x,
                        y: theme.shadow.sm.y
                    )
            )
            .overlay(
                RoundedRectangle(cornerRadius: 16)
                    .stroke(isSelected ? .clear : theme.colors.border, lineWidth: 1)
            )
        }
        .accessibilityIdentifier("wardrobeCategory_\(category.rawValue)")
    }

    // MARK: - 区块 4: grid (wardrobe.jsx:127-164)

    /// 道具网格：3 列 LazyVGrid + ForEach state.currentCategoryItems Button cell.
    private var grid: some View {
        ScrollView {
            LazyVGrid(
                columns: Array(repeating: GridItem(.flexible(), spacing: 10), count: 3),
                spacing: 10
            ) {
                ForEach(state.currentCategoryItems) { item in
                    gridCell(item: item)
                }
            }
            .padding(.horizontal, 20)
            .padding(.top, 8)
            .padding(.bottom, 100)  // 让出浮动 TabBar 空间
        }
    }

    private func gridCell(item: CosmeticItem) -> some View {
        let isSelected = state.selectedCosmeticId == item.id
        let isEquippedNow = state.isEquipped(item)
        return Button(action: { state.selectItem(item.id) }) {
            VStack(spacing: 6) {
                ZStack {
                    // 占位灰底圆角矩形（仅装饰；ui_design 钦定 surface-2 + 45° 斜条纹背景；本期简化为 surface 单色）.
                    RoundedRectangle(cornerRadius: 12)
                        .fill(theme.colors.surface2)
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
            .frame(maxWidth: .infinity)
            .background(
                RoundedRectangle(cornerRadius: 16)
                    .fill(theme.colors.surface)
                    .shadow(
                        color: theme.shadow.sm.color,
                        radius: theme.shadow.sm.radius,
                        x: theme.shadow.sm.x,
                        y: theme.shadow.sm.y
                    )
            )
            .overlay(
                RoundedRectangle(cornerRadius: 16)
                    .stroke(
                        isSelected ? theme.colors.accent : theme.colors.border,
                        lineWidth: isSelected ? 2.5 : 1
                    )
            )
            .opacity(item.owned ? 1.0 : 0.55)
        }
        .accessibilityIdentifier("wardrobeItem_\(item.id)")
    }
}

// MARK: - Previews

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
