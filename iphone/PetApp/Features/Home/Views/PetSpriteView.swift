// PetSpriteView.swift
// Story 8.4 AC2: 主界面猫 sprite 三态动画显示组件（rest / walk / run）.
//
// 设计基线：
// - generic struct + @Binding-free（接 MotionState 直接 read-only 入参）
// - 三态分支：rest / walk / run 各自独立的 SwiftUI 子视图（占位 SF Symbol 或简单几何形状）
// - 250ms 平滑过渡（淡入淡出 + 微缩放）：用 `.animation(.easeInOut(duration: 0.25), value: state)`
//   + `.transition(.opacity.combined(with: .scale))` —— Story 15.3 把过渡从纯 opacity 升级为
//   opacity+scale 组合，duration 上调到 250ms（仍处于 epics.md §15.3 行 2418 钦定 200-300ms 区间）.
// - accessibility identifier：state 切换时同步换 "petSprite_rest" / "petSprite_walk" / "petSprite_run"
//   ── UITest 通过 identifier 切换断言判定状态机是否真的驱动 sprite 渲染.
// - 占位 sprite 资产（节点 3 阶段美术资产不阻塞）：rest / walk / run 各用一个 SF Symbol + 颜色区分.
//
// 与 Story 37.7 chestSlot 接缝模式同精神：本 view 是独立 component；caller（HomeContainerHomeViewBridge
// 通过 petSlot ViewBuilder closure 注入）传入 PetSpriteView(state: viewModel.petState).
//
// **不**接 ViewModel：本 view 是 stateless representation —— state 由 caller 通过参数提供；
// 单元测试场景可 PetSpriteView(state: .walk) 直接构造验证视觉路径，不需要构造完整 ViewModel.

import SwiftUI

public struct PetSpriteView: View {
    public let state: MotionState

    /// 渲染尺寸（pt）；默认 180 保持 HomeView catStage 视觉基线不变.
    /// Story 15.1 review r1：RoomScaffoldView 成员行需要 40pt 缩略版 sprite —— 用
    /// `.frame(width: 40, height: 40).clipped()` 包裹只会裁切 180×180 内容，不会缩放，
    /// 视觉表现为"被裁切的猫头"。把 size 参数化让调用方真正决定渲染尺寸.
    /// 详见 docs/lessons/2026-05-12-swiftui-frame-clipped-does-not-scale.md.
    public let size: CGFloat

    /// Story 37.5: 主题 token 取值入口；本 view 嵌入 HomeView catStage 内 → 由父级 .environment(\.theme, ...) 透传.
    @Environment(\.theme) private var theme

    public init(state: MotionState, size: CGFloat = 180) {
        self.state = state
        self.size = size
    }

    public var body: some View {
        // Story 8.4 AC2 + Story 8.4 review fix: 整个 PetSpriteView 通过 `.accessibilityElement(children: .ignore)`
        //   收成单一 a11y leaf，防止被父级 catStage `.accessibilityElement(children: .contain)` 把
        //   "homeCatStage" identifier 继承覆盖子节点 identifier（iOS 26 简化处子级时 parent identifier 会优先 win）.
        //   identifier 与 label 挂在 outer 容器层；switch state 内部 image 仅做视觉渲染，不再各自挂 a11y modifier.
        //
        // 单一 sprite image 渲染（state 决定 SF Symbol + tint）—— 用 `.id(state)` 强制 SwiftUI 把
        //   state 切换识别为 view 替换 → `.transition(.opacity)` 才能生效；否则 `.animation(value:)`
        //   单独修饰一个常驻 view 的 modifier 链不会让 SF Symbol 内容做 fade（review round 2 P2 fix）.
        //
        // 设计要点（review round 2 P2 fix；详见 docs/lessons/2026-05-04-swiftui-content-swap-needs-id-and-transition.md）：
        //   · `.id(state)`：让 SwiftUI 在 state 改变时把当前 view tree 视为新 view（旧 view 移除 / 新 view 插入），
        //     而不是仅 mutate modifier；这是 .transition() 生效的前提.
        //   · `.transition(.opacity.combined(with: .scale))`：声明 view 加入 / 移除时走 opacity + scale 组合过渡
        //     —— Story 15.3 在 8.4 原始纯 opacity 基础上加 scale 维度（epics.md §15.3 行 2418 钦定）.
        //   · `.animation(.easeInOut(duration: 0.25), value: state)`：声明 transition 的 timing curve
        //     与 250ms duration —— Story 15.3 把 duration 从 8.4 的 200ms 上调到 250ms，
        //     仍处于 epics.md §15.3 行 2418 钦定 200-300ms 区间，给 scale 动效留可感知时间.
        //   · 三者缺一不可；.animation(value:) 单独使用只会动画化"已存在 modifier"的 value 变化，
        //     不会让 view body 内 switch 分支 swap 触发 fade.
        spriteImage(
            symbol: spriteSymbolName(for: state),
            tintColor: spriteTintColor(for: state)
        )
        .id(state)
        .transition(.opacity.combined(with: .scale))
        .animation(.easeInOut(duration: 0.25), value: state)
        // 把 PetSpriteView 整体收成 a11y 叶子节点；children: .ignore 让内部 SF Symbol 不被另算成 a11y 子节点.
        .accessibilityElement(children: .ignore)
        .accessibilityLabel(Text(accessibilityLabel))
        .accessibilityIdentifier(currentIdentifier)
    }

    /// state → SF Symbol 名映射（占位 sprite；节点 3 阶段美术资产不阻塞）.
    private func spriteSymbolName(for state: MotionState) -> String {
        switch state {
        case .rest: return "cat.fill"
        case .walk: return "figure.walk"
        case .run:  return "figure.run"
        }
    }

    /// state → tint color 映射（从 theme tokens 取值，遵循 Story 37.5 主题约定）.
    private func spriteTintColor(for state: MotionState) -> Color {
        switch state {
        case .rest: return theme.colors.inkSoft
        case .walk: return theme.colors.accent
        case .run:  return theme.colors.success
        }
    }

    /// 当前状态对应的 accessibility identifier（UITest 通过此值定位状态变化）.
    private var currentIdentifier: String {
        switch state {
        case .rest: return AccessibilityID.Home.petSpriteRest
        case .walk: return AccessibilityID.Home.petSpriteWalk
        case .run:  return AccessibilityID.Home.petSpriteRun
        }
    }

    /// 当前状态对应的 VoiceOver 中文 label.
    private var accessibilityLabel: String {
        switch state {
        case .rest: return "猫静止"
        case .walk: return "猫行走"
        case .run:  return "猫跑步"
        }
    }

    /// 单一占位 sprite image 渲染（SF Symbol + 半透明 tint，尺寸由 `size` 入参决定）.
    /// 节点 3 阶段美术资产不阻塞；后续 Story 30.x 落地真实 sprite render 时替换此 helper.
    private func spriteImage(symbol: String, tintColor: Color) -> some View {
        Image(systemName: symbol)
            .resizable()
            .scaledToFit()
            .frame(width: size, height: size)
            .foregroundColor(tintColor.opacity(0.7))
    }
}

#if DEBUG
#Preview("PetSprite — rest") {
    PetSpriteView(state: .rest)
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("PetSprite — walk") {
    PetSpriteView(state: .walk)
        .environment(\.theme, ThemeName.candy.theme)
}

#Preview("PetSprite — run") {
    PetSpriteView(state: .run)
        .environment(\.theme, ThemeName.candy.theme)
}
#endif
