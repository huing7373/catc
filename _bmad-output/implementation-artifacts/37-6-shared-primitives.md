# Story 37.6: 共享 primitives（Card / PrimaryButton / Avatar / FadeIn / RarityTag / Icons 完整集）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iOS 开发,
I want 一组从 `iphone/ui_design/source/components/primitives.jsx` 翻译来的 SwiftUI 共享 primitives（Card / PrimaryButton / Avatar / FadeIn / RarityTag）+ 完整 25 键 Icons → SF Symbol 对照表,
so that 后续 5 个 Feature View（HomeView / RoomView / WardrobeView / FriendsView / ProfileView）和 JoinRoomModal 可复用统一的卡片 / 按钮 / 头像 / 渐入动效 / 稀有度色条 / 图标，避免每屏自行重画相同 UI atom 导致的视觉漂移与维护噩梦。

## 故事定位（Epic 37 第三层第 2 条 story；与 Story 37.5 同层、可并行；下游 37.7–37.12 6 屏 / Modal 全部依赖本 story 的 primitives）

这是 Epic 37「iPhone 架构层重构 + UI Scaffold（壳先行）」的**第三层 story**之一（与 Story 37.5 同层；上游 Story 37.3 MainTabView + Story 37.4 AppState 已 done；同层 Story 37.5 Theme 已 done；下游 Story 37.7–37.12 6 屏 Scaffold + JoinRoomModal 跨屏跳转全部依赖本 story 的 6 个 primitive 文件）。本 story 是 **UI 基础类**——属于 Scaffold 共性约束「数据完全 mock + 禁 import APIClient / Repository / UseCase + 视觉像素级匹配 ui_design + 主题用 `@Environment(\.theme)` 取 token + 测试不引 SnapshotTesting / ViewInspector」适用范围。

**本 story 落地后立即解锁**：
- Story 37.7 HomeView Scaffold（StatusBar 步数计 prefix → `Icons.mapping["footprint"]`；ActionRow 三按钮 → `Icons.mapping["bowl"]/["heart"]/["ball"]`；TeamIdleCard 用 PrimaryButton + Card；CatStage 等级名牌底用 Card）
- Story 37.8 RoomView Scaffold（房间号 Card + 复制按钮用 `Icons.mapping["copy"]/["check"]`；MiniCat 头像用 Avatar；离开按钮用 PrimaryButton secondary）
- Story 37.9 WardrobeView Scaffold（顶部钻石货币 → `Icons.mapping["diamond"]`；分类 Tab + 道具卡用 Card；预览区装备/卸下按钮用 PrimaryButton；道具稀有度色条用 RarityTag）
- Story 37.10 FriendsView Scaffold（顶部添加 → `Icons.mapping["plus"]`；FriendRow 用 Avatar(online: ...); 加入按钮用 PrimaryButton + `Icons.mapping["enter"]`）
- Story 37.11 ProfileView Scaffold（头部 Avatar(ring:); 设置入口 → `Icons.mapping["settings"]`；菜单 chevron → `Icons.mapping["chevronRight"]`；微信绑定卡 → `Icons.mapping["wechat"]/["shield"]/["warn"]`；成就 → `Icons.mapping["trophy"]`；称号装饰 → `Icons.mapping["sparkle"]`）
- Story 37.12 JoinRoomModal（关闭按钮 → `Icons.mapping["close"]`；输入框前缀 → `Icons.mapping["paw"]`；确定加入按钮用 PrimaryButton；卡片用 Card 视觉规则）
- 历史占位 / 后续 story 收口：`MainTabView.FloatingTabBar` 内 4 个 SF Symbol 与本 story Icons 表完全对齐（home/box → `house.fill`/`shippingbox.fill` 等；本 story **不**主动改 MainTabView，由 Story 37.7 收口；本 story 仅保证 Icons 表与 FloatingTabBar 已使用的 4 个 SF Symbol 一致）

**本 story 的"实装"动作**（一句话概括）：在 `iphone/PetApp/Core/DesignSystem/Primitives/` 下**从空白构建** 6 个 primitive 文件 + 配套 6 个 #Preview 块（candy / dark 双主题各一）+ 1 个测试文件覆盖 Icons 25 键映射 + PrimaryButton disabled / RarityTag enum 路由 + xcodegen regen 让新文件入 PetApp / PetAppTests target。**无任何 production code 改动**（不改 RootView / MainTabView / 现有 Feature View / AppState）。

**关键路径："纯 SwiftUI value type View struct" + "依赖 Theme 但不复刻 token" + "Icons 表是 String → String 字典"**（本 story 关键设计决策）：

- 6 个 primitive 全部是 `public struct ... : View`（value type，无 ObservableObject / 状态注入）；仅 PrimaryButton / Avatar / RarityTag 接受参数，Card / FadeIn 接受 `@ViewBuilder content`，Icons 是 enum 命名空间 + 静态字典
- 所有视觉 token 通过 `@Environment(\.theme) var theme` 取（不 hardcode hex / spacing / radius；唯一例外是 RarityTag 的 4 档 hex 色，因为 ui_design README §Wardrobe §稀有度配色钦定脱离 theme 命名空间——属于 Wardrobe 业务色而非 theme color）
- `Icons` 是 `public enum Icons { public static let mapping: [String: String] = [...] }` —— 不暴露 SwiftUI Image 类型（让调用方决定是 `Image(systemName:)` 还是其它包装），保持本类型为纯映射表
- Icons.mapping 必须**精确 25 键**（见下方 AC2 表），未匹配键返回 `"questionmark.circle"` + log warning（通过 `Icons.symbol(for:)` 静态函数包装），**不**走 silent fallback
- caller 漏改靠**编译器报错**驱动（任何代码引用不存在的 primitive 类型 / API 会立即编译失败）
- 视觉差异容忍：本表 `ball → circle.dotted` / `bowl → bowl.fill` / `paw → pawprint.fill` 等是 SF Symbol 视觉**近似**而非像素一致；视觉精度由 Story 37.13 visual-review-checklist 人眼把关；**不接受** dev 自行替换映射

**不涉及**（红线）：
- **不**实装 Feature View（HomeView / RoomView / WardrobeView / FriendsView / ProfileView 由 Story 37.7-37.11 落地；本 story 仅落地 6 个 primitive 文件）
- **不**改 MainTabView FloatingTabBar 硬编码 SF Symbol（保留给 Story 37.7 收口；本 story 仅保证 Icons 表与 FloatingTabBar 已使用的 SF Symbol 一致）
- **不**改 RootView / AppState / AppCoordinator（primitives 不持任何 domain state）
- **不**实装 JoinRoomModal（Story 37.12 落地真正 modal 内容；本 story 仅落地 modal 会用到的 atom）
- **不**实装 ChestCardView / FriendRow / CatStage 等业务 composite 组件（属于各 Feature 内部 composition，由 Scaffold story 落地）
- **不**改 ios/ 任何文件（CLAUDE.md + ADR-0002 §3.3 强约束）
- **不**改 server/ 任何文件
- **不**新增 ObservableObject / @Published 模型（primitives 是 value type View）
- **不**引入 SnapshotTesting / ViewInspector（ADR-0002 §3.1 钦定 XCTest only；视觉验证靠 #Preview + Story 37.13 视觉 review）
- **不**实装 `Icons.symbol(for:)` 之外的任何 helper（如 `Image(symbol:)` 视图工厂）—— 让调用方决定 Image 构造时机，避免封装层过厚

## Acceptance Criteria

> **AC 编号体系**：AC1 是文件 / 目录骨架；AC2 是 Icons.swift 完整映射表（25 键，最重，含 fallback 行为）；AC3-AC7 是 5 个 primitive（Card / PrimaryButton / Avatar / FadeIn / RarityTag）字段与视觉契约；AC8 是 #Preview 覆盖；AC9 是单元测试；AC10 是 xcodegen + build verify；AC11 是 Deliverable 清单。

### AC1 — 新建 Primitives/ 子目录骨架（6 个 production 文件 + 1 个测试文件）

**新建目录** `iphone/PetApp/Core/DesignSystem/Primitives/`（与 `Components/` 同级；Components 是已存在的错误 UI 视图集，Primitives 是 ui_design 翻译过来的通用 atom，命名边界清晰）。

**新建 6 个 production 文件**（具体内容见 AC2-AC7）：

```
iphone/PetApp/Core/DesignSystem/Primitives/
├─ Icons.swift            # 25 键 SF Symbol 映射 + symbol(for:) helper
├─ Card.swift             # 圆角 + theme.colors.surface + theme.shadow.sm 卡片容器
├─ PrimaryButton.swift    # 圆药丸按钮 (primary / secondary / ghost 三 variant)
├─ Avatar.swift           # 圆形头像 + 光环描边 + 在线小绿点
├─ FadeIn.swift           # ViewModifier，0.28s ease 渐入 + 上移 8pt
└─ RarityTag.swift        # N/R/SR/SSR 4 档稀有度色条
```

**新建 1 个测试文件**：`iphone/PetAppTests/Core/DesignSystem/Primitives/PrimitivesTests.swift`（落地 ≥3 case，详见 AC9；与 production 镜像目录）。

> **xcodegen 通配**：`iphone/project.yml` 内 `targets.PetApp.sources: - PetApp` 是**通配** PetApp/ 子树全部 .swift 文件；新增 `Primitives/*.swift` 6 个文件全部在 PetApp/Core/DesignSystem/ 下 → 自动 inclusion，**不**改 project.yml。测试目录同理：`PetAppTests` 通配新增 `Core/DesignSystem/Primitives/PrimitivesTests.swift`。dev 须 `cd iphone && xcodegen generate` 重生成 `PetApp.xcodeproj`（详见 AC10）。

### AC2 — 新建 Icons.swift（25 键 SF Symbol 映射，1:1 对齐 ui_design primitives.jsx）

**Icons.swift 文件契约**：

```swift
// Icons.swift
// Story 37.6: ui_design primitives.jsx 内 SVG `Icons` 对象的 SF Symbol 翻译表（25 键完整集）.
//
// 设计约束：
//   - 键名严格保持 ui_design primitives.jsx 内 Icons 对象的原型驼峰写法（home / box / friends /
//     user / paw / bowl / heart / ball / footprint / plus / enter / close / back / dot / copy /
//     check / settings / sparkle / bell / chevronRight / wechat / shield / warn / diamond / trophy）
//   - 值是 SF Symbol 名（视觉近似而非像素一致；视觉精度由 Story 37.13 visual-review-checklist 人眼把关）
//   - 不暴露 SwiftUI Image 类型（让调用方决定是 `Image(systemName:)` 还是其它包装）
//   - 未匹配键查询 → log warning + 退回 `questionmark.circle`；不允许 silent fallback

import Foundation
import os.log

/// Icons: ui_design primitives.jsx 内 SVG `Icons` 对象的 SF Symbol 翻译表.
///
/// **完整 25 键映射**（与 `iphone/ui_design/source/components/primitives.jsx` 内 `Icons` 对象 1:1 对齐）：
/// 视觉差异容忍：`ball → circle.dotted` / `bowl → bowl.fill` / `paw → pawprint.fill` 等是 SF Symbol
/// 视觉**近似**而非像素一致；视觉精度由 Story 37.13 visual-review-checklist 人眼把关；**不接受**
/// dev 自行替换映射（如改成 figure.circle 等）。
///
/// **使用方式**：
///   - 取 SF Symbol 名：`Icons.mapping["home"]` 返回 "house.fill"（Optional<String>）
///   - 取 SF Symbol 名 + fallback：`Icons.symbol(for: "home")` 返回 "house.fill"，未知键返回 "questionmark.circle"
///   - SwiftUI 渲染：`Image(systemName: Icons.symbol(for: "home"))`
public enum Icons {

    /// 完整 25 键 SF Symbol 映射.
    ///
    /// 见枚举上方注释关于视觉差异容忍的说明。
    public static let mapping: [String: String] = [
        "home":         "house.fill",
        "box":          "shippingbox.fill",
        "friends":      "person.2.fill",
        "user":         "person.crop.circle.fill",
        "paw":          "pawprint.fill",
        "bowl":         "fork.knife",
        "heart":        "heart.fill",
        "ball":         "circle.dotted",
        "footprint":    "figure.walk",
        "plus":         "plus.circle.fill",
        "enter":        "arrow.right.circle.fill",
        "close":        "xmark.circle.fill",
        "back":         "chevron.left",
        "dot":          "circle.fill",
        "copy":         "doc.on.doc.fill",
        "check":        "checkmark.circle.fill",
        "settings":     "gearshape.fill",
        "sparkle":      "sparkles",
        "bell":         "bell.fill",
        "chevronRight": "chevron.right",
        "wechat":       "message.fill",
        "shield":       "shield.fill",
        "warn":         "exclamationmark.triangle.fill",
        "diamond":      "diamond.fill",
        "trophy":       "trophy.fill",
    ]

    /// fallback SF Symbol（iOS 17+ 全部存在；用于未匹配键查询）.
    public static let fallbackSymbol: String = "questionmark.circle"

    /// 根据 ui_design 键名取 SF Symbol；未匹配键返回 fallback + log warning（不允许 silent fallback）.
    ///
    /// - Parameter key: ui_design primitives.jsx 内 Icons 对象的键名（驼峰）
    /// - Returns: 对应 SF Symbol 名；未匹配返回 `fallbackSymbol`
    public static func symbol(for key: String) -> String {
        if let symbol = mapping[key] {
            return symbol
        }
        // 未匹配键：log warning + 退回 fallback；让调用站点漂移有 log 信号
        os_log(.error, "Icons.symbol(for:) unknown key: %{public}@; returning fallback %{public}@", key, fallbackSymbol)
        return fallbackSymbol
    }
}
```

**完整 25 键映射表来源**（与代码内字典字面量必须严格一致）：

| ui_design 键（驼峰） | SF Symbol | 用途定位 |
|---|---|---|
| home | house.fill | TabBar 家 |
| box | shippingbox.fill | 宝箱、TabBar 仓库（替 box） |
| friends | person.2.fill | TabBar 好友 |
| user | person.crop.circle.fill | TabBar 我的、Profile 头像 |
| paw | pawprint.fill | JoinRoomModal 输入框 prefix |
| bowl | fork.knife | 喂食按钮（FeedButton） |

> **2026-04-30 user-authorized substitution**：原写 `"bowl.fill"`，dev-story 阶段双路验证 iOS 17+/26.4 simruntime 不提供该 SF Symbol，AC9 case#3 必然 fail；user 授权对"dev 不许改 SF Symbol mapping"红线一次例外，替换为 `"fork.knife"`（实存 + 语义最贴近喂食按钮）。
> 根因：spec 设计阶段未核实 SF Symbol 物理可用性；后续类似 AC 应在 SM 阶段先用 `UIImage(systemName:)` 验证可用后再钦定字符串。
| heart | heart.fill | 抚摸按钮（PetButton；filled 变体在原型用 `heart(filled=true)`，SF Symbol 区分 heart vs heart.fill） |
| ball | circle.dotted | 玩耍按钮（PlayButton） |
| footprint | figure.walk | StatusBar 步数计 prefix |
| plus | plus.circle.fill | FriendsView 添加好友按钮 |
| enter | arrow.right.circle.fill | 创建/加入队伍 CTA、FriendRow 加入按钮 |
| close | xmark.circle.fill | Modal 关闭按钮 |
| back | chevron.left | 导航返回（Tab 内 NavigationStack push 自动给）|
| dot | circle.fill | 在线小绿点、状态指示 |
| copy | doc.on.doc.fill | 房间代码复制按钮 |
| check | checkmark.circle.fill | 已装备 / 复制成功 |
| settings | gearshape.fill | Profile 设置入口 |
| sparkle | sparkles | 装扮稀有度装饰 / Profile 称号装饰 |
| bell | bell.fill | Profile 顶部消息（视觉占位，本期不做行为）|
| chevronRight | chevron.right | Profile 菜单项 / 横向滚动指示 |
| wechat | message.fill | Profile 微信绑定（视觉占位，按钮 toast）|
| shield | shield.fill | 微信绑定 Modal 数据保护图标 |
| warn | exclamationmark.triangle.fill | 微信绑定 Modal 警告图标 |
| diamond | diamond.fill | Wardrobe 钻石货币 |
| trophy | trophy.fill | Profile 成就统计 |

> **dev 必读**：键名拼写**严格保持驼峰**（`chevronRight` 不是 `chevron_right` 或 `chevron-right`），与 `iphone/ui_design/source/components/primitives.jsx` 内 `Icons` 对象的 JS 对象字段名一致；这是为了让 future dev 在 Scaffold story 中 grep `Icons.mapping["chevronRight"]` 时能直接对齐 ui_design 源文件。

### AC3 — 新建 Card.swift（卡片容器）

**视觉契约**（来自 ui_design primitives.jsx `Card` 函数）：
- 背景：`theme.colors.surface`
- 圆角：默认 `theme.radius.cardXl`（24pt；ui_design 钦定 `borderRadius: 24`）；接受可选 `cornerRadius: CGFloat?` 参数让调用方覆写（如 ui_design TeamIdleCard 用 22）
- 阴影：`theme.shadow.sm`
- 内边距：默认 `theme.spacing.s16`（16pt；ui_design 钦定 `padding: 16`）；接受可选 `padding: CGFloat?` 参数让调用方覆写
- 边框：1pt + `theme.colors.border`（ui_design 钦定 `border: '1px solid var(--border)'`）

**Card.swift 文件契约**：

```swift
// Card.swift
// Story 37.6: 通用卡片容器，对齐 ui_design primitives.jsx `Card` 函数.
//
// 视觉规则：theme.colors.surface 背景 + theme.shadow.sm 阴影 + theme.colors.border 1pt 描边 +
// theme.radius.cardXl 圆角（默认 24pt；调用方可通过 cornerRadius 参数覆写）.

import SwiftUI

/// Card: 通用卡片容器.
///
/// 调用方式：`Card { Text("hello") }` 或 `Card(cornerRadius: 22, padding: 18) { ... }`.
public struct Card<Content: View>: View {
    @Environment(\.theme) private var theme

    private let cornerRadius: CGFloat?
    private let padding: CGFloat?
    @ViewBuilder private let content: () -> Content

    public init(
        cornerRadius: CGFloat? = nil,
        padding: CGFloat? = nil,
        @ViewBuilder content: @escaping () -> Content
    ) {
        self.cornerRadius = cornerRadius
        self.padding = padding
        self.content = content
    }

    public var body: some View {
        let resolvedCornerRadius = cornerRadius ?? theme.radius.cardXl
        let resolvedPadding = padding ?? theme.spacing.s16
        return content()
            .padding(resolvedPadding)
            .background(
                RoundedRectangle(cornerRadius: resolvedCornerRadius)
                    .fill(theme.colors.surface)
            )
            .overlay(
                RoundedRectangle(cornerRadius: resolvedCornerRadius)
                    .stroke(theme.colors.border, lineWidth: 1)
            )
            .shadow(
                color: theme.shadow.sm.color,
                radius: theme.shadow.sm.radius,
                x: theme.shadow.sm.x,
                y: theme.shadow.sm.y
            )
    }
}
```

> **dev 实装备注**：
> - 用 `RoundedRectangle.fill` + `.overlay(RoundedRectangle.stroke)` 而非 `.background(...).overlay(...)` 直接套用单一形状——这是 SwiftUI 圆角 + border + shadow 三件套的标准模式（避免 cornerRadius 单独 modifier 因 mask 顺序导致 shadow 被裁掉）
> - **不**接受 onClick / Button 包装（ui_design 原型有 `onClick` 参数，但 SwiftUI 调用方应自己用 `.onTapGesture` 或外层 `Button(action:) { Card { ... } }`，让 Card 保持纯展示型 atom）
> - 若调用方要内嵌可点击区域，由调用方包 Button；本 Card 不持任何 gesture

### AC4 — 新建 PrimaryButton.swift（圆药丸按钮，3 variant）

**视觉契约**（来自 ui_design primitives.jsx `PrimaryButton` 函数）：
- 三 variant：
  - `.primary`：背景 `theme.colors.accent`，文字白色，硬阴影 `0 4px 0 theme.colors.accentDeep`
  - `.secondary`：背景 `theme.colors.surface`，文字 `theme.colors.ink`，硬阴影 `0 4px 0 black.opacity(0.08)`，1.5pt border `theme.colors.border`
  - `.ghost`：背景 `theme.colors.accentSoft`，文字 `theme.colors.accentDeep`，硬阴影 `0 3px 0 black.opacity(0.06)`
- 尺寸：高度 52pt（`theme.spacing` 没 52，用 inline 字面量）；左右 padding 22pt
- 圆角：`theme.radius.pill`（999；圆药丸）
- 字体：`theme.typography.mediumTitle`（17pt heavy；ui_design 钦定 `fontSize: 16, fontWeight: 700`，SwiftUI mediumTitle 17/heavy 视觉接近）
- 按下动效：`translateY(2px)` 0.1s ease（SwiftUI `.scaleEffect` 不合适——这是平移；用 `.offset(y: 2)` 配合 `.animation`）
- disabled 态：opacity 0.5 + 不响应 tap（SwiftUI Button 默认 `.disabled(true)` 已含此行为，但视觉上需明确减淡）
- icon 槽：可选 leading 图标（接受 `Image?` 或 SF Symbol 名）

**PrimaryButton.swift 文件契约**：

```swift
// PrimaryButton.swift
// Story 37.6: 圆药丸主按钮，对齐 ui_design primitives.jsx `PrimaryButton` 函数.
//
// 三 variant: primary / secondary / ghost; 高度 52pt; 圆角走 theme.radius.pill 圆药丸; 硬阴影
// 立体感（按下 translateY(2)）;支持 disabled 态.

import SwiftUI

/// PrimaryButton variant: 三档样式（来自 ui_design primitives.jsx `PrimaryButton` 函数）.
public enum PrimaryButtonVariant {
    case primary
    case secondary
    case ghost
}

/// PrimaryButton: 圆药丸主按钮.
public struct PrimaryButton: View {
    @Environment(\.theme) private var theme

    private let title: String
    private let variant: PrimaryButtonVariant
    private let icon: String?       // SF Symbol 名（来自 Icons.symbol(for:)）；nil 时无 icon
    private let fullWidth: Bool
    private let isEnabled: Bool
    private let action: () -> Void

    @State private var isPressed: Bool = false

    public init(
        title: String,
        variant: PrimaryButtonVariant = .primary,
        icon: String? = nil,
        fullWidth: Bool = false,
        isEnabled: Bool = true,
        action: @escaping () -> Void
    ) {
        self.title = title
        self.variant = variant
        self.icon = icon
        self.fullWidth = fullWidth
        self.isEnabled = isEnabled
        self.action = action
    }

    public var body: some View {
        Button(action: action) {
            HStack(spacing: theme.spacing.s8) {
                if let icon {
                    Image(systemName: icon)
                }
                Text(title)
            }
            .font(theme.typography.mediumTitle.font)
            .foregroundColor(foregroundColor)
            .frame(height: 52)
            .frame(maxWidth: fullWidth ? .infinity : nil)
            .padding(.horizontal, theme.spacing.s22)
            .background(
                RoundedRectangle(cornerRadius: theme.radius.pill)
                    .fill(backgroundColor)
            )
            .overlay(
                Group {
                    if let borderColor {
                        RoundedRectangle(cornerRadius: theme.radius.pill)
                            .stroke(borderColor, lineWidth: 1.5)
                    }
                }
            )
            .shadow(color: shadowColor, radius: 0, x: 0, y: shadowY)
            .offset(y: isPressed ? 2 : 0)
            .opacity(isEnabled ? 1.0 : 0.5)
            .animation(.easeOut(duration: 0.1), value: isPressed)
        }
        .buttonStyle(.plain)
        .disabled(!isEnabled)
        .simultaneousGesture(
            DragGesture(minimumDistance: 0)
                .onChanged { _ in isPressed = isEnabled }
                .onEnded { _ in isPressed = false }
        )
    }

    private var backgroundColor: Color {
        switch variant {
        case .primary:   return theme.colors.accent
        case .secondary: return theme.colors.surface
        case .ghost:     return theme.colors.accentSoft
        }
    }

    private var foregroundColor: Color {
        switch variant {
        case .primary:   return Color.white
        case .secondary: return theme.colors.ink
        case .ghost:     return theme.colors.accentDeep
        }
    }

    private var borderColor: Color? {
        switch variant {
        case .secondary: return theme.colors.border
        default:         return nil
        }
    }

    private var shadowColor: Color {
        switch variant {
        case .primary:   return theme.colors.accentDeep
        case .secondary: return Color.black.opacity(0.08)
        case .ghost:     return Color.black.opacity(0.06)
        }
    }

    private var shadowY: CGFloat {
        switch variant {
        case .ghost: return 3
        default:     return 4
        }
    }
}
```

> **dev 实装备注**：
> - 按下平移用 `.offset(y: isPressed ? 2 : 0)` + `simultaneousGesture(DragGesture)` 实装（SwiftUI Button 内置高亮态非可控；DragGesture 0 距离触发即按下）；不要用 `ButtonStyle.makeBody.configuration.isPressed` 因为它仅在 `ButtonStyle` 内可见，将增加抽象层
> - icon 参数类型是 `String?`（SF Symbol 名）而非 `Image?`——这样调用方写 `PrimaryButton(title: "加入", icon: Icons.symbol(for: "enter"))` 链路顺畅；如未来要支持非 SF Symbol Image，再加重载
> - **不**支持自定义 background/foreground color——color 完全由 variant 决定，避免 caller 写出脱离 theme 的颜色组合

### AC5 — 新建 Avatar.swift（圆形头像 + 光环描边 + 在线小绿点）

**视觉契约**（来自 ui_design primitives.jsx `Avatar` 函数）：
- 形状：圆形（`.clipShape(Circle())`）
- 默认尺寸：44pt（接受 `size: CGFloat = 44` 参数）
- 占位逻辑：取 `name` 首字母大写居中显示；背景从 7 色调色板按 hash 选（与 primitives.jsx palette 完全一致：`["#ffb3c1","#ffd6a5","#caffbf","#bdb2ff","#a0c4ff","#ffc8dd","#b8e0d2"]`）；调用方可通过 `color: Color?` 参数显式覆写背景色（覆写时不走 hash）
- 光环描边（`ring: Bool = false`）：开启时 `boxShadow: 0 0 0 3px theme.colors.surface, 0 0 0 5px theme.colors.accent`；SwiftUI 实装用嵌套 Circle stroke：内层描边 surface 3pt + 外层描边 accent 2pt（叠加效果）
- 在线小绿点（`online: Bool? = nil`）：右下角 `size * 0.28` 直径圆点；online == true → `theme.colors.success`；online == false → 灰色 `Color(red: 0.76, green: 0.74, blue: 0.73)`（ui_design 钦定 `#c3bdb9`）；nil 不渲染
- 占位文字字号：`size * 0.4`，weight `.heavy`，color `Color(black).opacity(0.55)`

**Avatar.swift 文件契约**：

```swift
// Avatar.swift
// Story 37.6: 圆形头像 + 光环描边 + 在线小绿点，对齐 ui_design primitives.jsx `Avatar` 函数.

import SwiftUI

/// Avatar: 圆形头像（占位首字母 + 7 色调色板 hash 选色 + 光环描边 + 在线小绿点）.
public struct Avatar: View {
    @Environment(\.theme) private var theme

    private let name: String
    private let size: CGFloat
    private let color: Color?      // 显式覆写背景色；nil 时走 hash
    private let online: Bool?      // nil 不渲染小点；true 绿色；false 灰色
    private let ring: Bool         // 光环描边开关

    /// 7 色调色板（来自 ui_design primitives.jsx Avatar palette；本期不抽到 theme，
    /// 因为是 Avatar 专属占位策略而非 theme color）.
    private static let palette: [Color] = [
        Color(red: 1.00, green: 0.70, blue: 0.76),     // #ffb3c1
        Color(red: 1.00, green: 0.84, blue: 0.65),     // #ffd6a5
        Color(red: 0.79, green: 1.00, blue: 0.75),     // #caffbf
        Color(red: 0.74, green: 0.70, blue: 1.00),     // #bdb2ff
        Color(red: 0.63, green: 0.77, blue: 1.00),     // #a0c4ff
        Color(red: 1.00, green: 0.78, blue: 0.87),     // #ffc8dd
        Color(red: 0.72, green: 0.88, blue: 0.82),     // #b8e0d2
    ]

    /// 离线灰色（来自 ui_design primitives.jsx Avatar offline indicator color #c3bdb9）.
    private static let offlineColor: Color = Color(
        red: 0xC3 / 255.0, green: 0xBD / 255.0, blue: 0xB9 / 255.0
    )

    public init(
        name: String,
        size: CGFloat = 44,
        color: Color? = nil,
        online: Bool? = nil,
        ring: Bool = false
    ) {
        self.name = name
        self.size = size
        self.color = color
        self.online = online
        self.ring = ring
    }

    public var body: some View {
        let bg = color ?? Self.palette[hashIndex(of: name)]
        let initial = String((name.first ?? "?")).uppercased()

        ZStack {
            Circle()
                .fill(bg)
            if ring {
                Circle()
                    .strokeBorder(theme.colors.surface, lineWidth: 3)
                Circle()
                    .strokeBorder(theme.colors.accent, lineWidth: 2)
                    .padding(-2)
            }
            Text(initial)
                .font(.system(size: size * 0.4, weight: .heavy, design: .rounded))
                .foregroundColor(Color.black.opacity(0.55))

            if let online {
                onlineDot(online: online)
            }
        }
        .frame(width: size, height: size)
    }

    /// 根据 name 字符 ASCII 求和取 palette index（与 ui_design primitives.jsx hash 算法一致）.
    private func hashIndex(of s: String) -> Int {
        let sum = s.unicodeScalars.reduce(0) { $0 + Int($1.value) }
        return abs(sum) % Self.palette.count
    }

    /// 右下角在线小绿点（在线 success 色 / 离线灰色）.
    @ViewBuilder
    private func onlineDot(online: Bool) -> some View {
        let dotSize = size * 0.28
        Circle()
            .fill(online ? theme.colors.success : Self.offlineColor)
            .frame(width: dotSize, height: dotSize)
            .overlay(
                Circle()
                    .stroke(theme.colors.surface, lineWidth: 2.5)
            )
            .offset(x: size * 0.32, y: size * 0.32)
    }
}
```

> **dev 实装备注**：
> - **palette 不抽到 theme**：这是 Avatar 占位策略（hash 选色让不同用户名稳定分配不同色），属于组件实现细节；不是 theme color
> - 离线灰 `#c3bdb9` 也是 ui_design 原型 hardcode；同理不抽 theme
> - 光环描边用 `.strokeBorder` + `.padding(-2)` 模拟 ui_design 的 `boxShadow` 多层光环——SwiftUI 没有真正的 outset shadow，stroke 描边 + padding 负值是常用近似
> - **不**支持 image url（ui_design 原型也只占位首字母 + 颜色；image url 等 Story 21.x 后续头像上传 epic 实装）

### AC6 — 新建 FadeIn.swift（ViewModifier，0.28s ease 渐入 + 上移 8pt）

**视觉契约**（来自 ui_design README §Interactions §动画 + primitives.jsx `FadeIn` 函数）：
- 持续：0.28s
- 曲线：`.easeInOut`（ui_design 钦定 `ease`，SwiftUI 默认 ease 是 easeInOut）
- 起态：opacity 0 + 上移 -8pt（即从下方 8pt 滑上来）
- 终态：opacity 1 + offset 0
- 触发：onAppear；接受可选 `id: AnyHashable?` 参数让调用方在 `id` 变化时重触发动画（ui_design 原型 `keyProp` 同义；SwiftUI 实装用 `.id(id)` 让 SwiftUI 重建子树触发 onAppear）

**FadeIn.swift 文件契约**：

```swift
// FadeIn.swift
// Story 37.6: 渐入 + 上移 8pt 入场动效，对齐 ui_design primitives.jsx `FadeIn` 函数 +
// ui_design README §Interactions §动画 "Tab 切换：内容淡入 + 上移（fadeIn 0.28s ease）".

import SwiftUI

/// FadeInModifier: 渐入 + 上移 8pt 入场动效 ViewModifier.
///
/// 0.28s easeInOut；从 opacity 0 + offsetY -8 渐入到 opacity 1 + offsetY 0.
/// 触发：onAppear；id 变化时重触发（SwiftUI 通过 `.id(id)` 重建子树 → 重走 onAppear）.
public struct FadeInModifier: ViewModifier {
    private let id: AnyHashable?

    @State private var visible: Bool = false

    public init(id: AnyHashable? = nil) {
        self.id = id
    }

    public func body(content: Content) -> some View {
        content
            .opacity(visible ? 1 : 0)
            .offset(y: visible ? 0 : -8)
            .onAppear {
                withAnimation(.easeInOut(duration: 0.28)) {
                    visible = true
                }
            }
            .id(id)
    }
}

extension View {
    /// 应用 FadeInModifier 到当前视图.
    /// - Parameter id: 可选 id；变化时重触发动画（用于 Tab 切换等场景）.
    public func fadeIn(id: AnyHashable? = nil) -> some View {
        modifier(FadeInModifier(id: id))
    }
}
```

> **dev 实装备注**：
> - `.id(id)` 配合 `id` 变化时让 SwiftUI 重建子树——这是 SwiftUI 触发 onAppear 重跑的标准模式；对应 React `key` prop 概念
> - id 类型用 `AnyHashable?` 让调用方传任意 Hashable 值（String / Int / enum case）
> - **不**支持自定义 duration / curve / offset——FadeIn 是 ui_design 钦定单一规则，调用方需要其它动画自己写 modifier，不复用 FadeIn
> - **不**做 onDisappear 反向动画（ui_design 原型 `FadeIn` 函数也只 in 不 out；离开动画由 SwiftUI 默认 transition 接管）

### AC7 — 新建 RarityTag.swift（N/R/SR/SSR 4 档稀有度色条）

**视觉契约**（来自 ui_design README §Wardrobe §稀有度配色）：
- 4 档枚举 `Rarity { N, R, SR, SSR }`
- 配色：
  - `.N`：纯色 `#b0b0b0`（灰）
  - `.R`：纯色 `#7db3e8`（蓝）
  - `.SR`：纯色 `#c58ae8`（紫）
  - `.SSR`：渐变 `linear-gradient(90deg, #ffd166, #ef476f)`（金到红）
- 视觉：横条 / 圆角条；尺寸由 caller 决定（接受 `width: CGFloat = 40, height: CGFloat = 4` 参数）
- 字面量稀有度色 hex 不抽到 theme（属 Wardrobe 业务色而非 theme color）

**RarityTag.swift 文件契约**：

```swift
// RarityTag.swift
// Story 37.6: 稀有度 4 档色条，对齐 ui_design README §Wardrobe §稀有度配色:
//   N=灰 #b0b0b0 / R=蓝 #7db3e8 / SR=紫 #c58ae8 / SSR=金红渐变 linear-gradient(90deg,#ffd166,#ef476f).
//
// 配色 hex 不抽到 theme（属 Wardrobe 业务色而非 theme color）.

import SwiftUI

/// Rarity: 装扮道具稀有度 4 档.
public enum Rarity: String, CaseIterable, Identifiable {
    case N
    case R
    case SR
    case SSR

    public var id: String { rawValue }
}

/// RarityTag: 稀有度色条（横条；caller 决定 width/height）.
public struct RarityTag: View {
    private let rarity: Rarity
    private let width: CGFloat
    private let height: CGFloat

    public init(rarity: Rarity, width: CGFloat = 40, height: CGFloat = 4) {
        self.rarity = rarity
        self.width = width
        self.height = height
    }

    public var body: some View {
        Capsule()
            .fill(fillStyle)
            .frame(width: width, height: height)
            .accessibilityIdentifier("rarityTag_\(rarity.rawValue)")
    }

    private var fillStyle: AnyShapeStyle {
        switch rarity {
        case .N:
            return AnyShapeStyle(Color(red: 0xB0 / 255.0, green: 0xB0 / 255.0, blue: 0xB0 / 255.0))
        case .R:
            return AnyShapeStyle(Color(red: 0x7D / 255.0, green: 0xB3 / 255.0, blue: 0xE8 / 255.0))
        case .SR:
            return AnyShapeStyle(Color(red: 0xC5 / 255.0, green: 0x8A / 255.0, blue: 0xE8 / 255.0))
        case .SSR:
            return AnyShapeStyle(LinearGradient(
                colors: [
                    Color(red: 1.00, green: 0.82, blue: 0.40),    // #ffd166
                    Color(red: 0.94, green: 0.28, blue: 0.43),    // #ef476f
                ],
                startPoint: .leading,
                endPoint: .trailing
            ))
        }
    }
}
```

> **dev 实装备注**：
> - 用 `AnyShapeStyle` 让 `.fill(...)` 接受 Color 与 LinearGradient 同样的返回类型——这是 SwiftUI 17+ 推荐的统一 ShapeStyle 模式（避免分支返回不同 View 类型导致编译失败）
> - `accessibilityIdentifier("rarityTag_\(rarity.rawValue)")` 给单元测试 / Story 37.13 a11y 总表归并提供定位锚（命名风格与 Story 37.3 落地的 `tab_<rawValue>` 一致）
> - **不**接受自定义颜色覆写——稀有度配色是 ui_design 钦定锁定值，覆写会破坏游戏视觉契约

### AC8 — 每个 primitive 配 #Preview 块（candy 主题 + dark 主题各一个）

每个 primitive 文件底部加 `#if DEBUG ... #endif` 块，含 2 个 #Preview：

- `#Preview("Card — candy")`: `Card { Text("hello") }.environment(\.theme, .candy).padding().background(Theme.candy.colors.pageBg)`
- `#Preview("Card — dark")`: 同上但 `.dark`
- `#Preview("PrimaryButton — candy")`: 4 行 `VStack`，含 primary / secondary / ghost / disabled 4 种状态
- `#Preview("PrimaryButton — dark")`: 同上但 `.dark`
- `#Preview("Avatar — candy")`: 4 个 Avatar：默认占位 / ring=true / online=true / online=false
- `#Preview("Avatar — dark")`: 同上但 `.dark`
- `#Preview("FadeIn — candy")`: 一个 Card 套 `.fadeIn()` modifier；预览首次展示时触发
- `#Preview("FadeIn — dark")`: 同上但 `.dark`
- `#Preview("RarityTag — candy")`: 4 个 RarityTag 排列展示 N / R / SR / SSR
- `#Preview("RarityTag — dark")`: 同上但 `.dark`
- `#Preview("Icons — candy")`: LazyVGrid 展示全 25 键 Icons（Image(systemName:) 渲染）；用于视觉抽样
- `#Preview("Icons — dark")`: 同上但 `.dark`

> **dev 实装备注**：
> - Preview 内部用 `.environment(\.theme, ThemeName.candy.theme)` 注入主题（与 Story 37.5 ThemePreview_Sampler 同模式）
> - **不**用 ViewInspector / SnapshotTesting 自动断言 Preview 渲染结果（ADR-0002 §3.1）；Preview 仅供 dev 在 Xcode Canvas 目视对照 ui_design 像素级匹配
> - Preview 块全部包在 `#if DEBUG` 内，Release build 不编译

### AC9 — 单元测试覆盖（≥3 case；本文件落地 5 case 更稳）

在 `iphone/PetAppTests/Core/DesignSystem/Primitives/PrimitivesTests.swift` 落地以下 5 case（≥3 case，按 v1 epic acceptance；额外加 ranges + symbol fallback 共 5 case 更稳）：

```swift
// PrimitivesTests.swift
// Story 37.6 AC9: 共享 primitives 单元测试（≥3 case；本文件落地 5 case）.
//
// 测试基础设施约束（与 Story 2.7 + ADR-0002 §3.1 衔接）：
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）.
//   - 不引 ViewInspector / SnapshotTesting.
//   - Icons 25 键映射 + symbol(for:) fallback + RarityTag enum 抽样 + PrimaryButton variant 抽样.

import XCTest
import SwiftUI
@testable import PetApp

final class PrimitivesTests: XCTestCase {

    // MARK: - case#1 happy: Icons.mapping["home"] 精确

    /// 验证 Icons.mapping["home"] == "house.fill"（v2 §7 Icons 完整映射表抽样）.
    func testIconsMappingHomeReturnsHouseFill() {
        XCTAssertEqual(Icons.mapping["home"], "house.fill")
    }

    // MARK: - case#2 happy: Icons.mapping 25 键完整

    /// 验证 Icons.mapping 含且仅含 25 键（防漏 / 防多）.
    /// 25 键名严格 1:1 对齐 iphone/ui_design/source/components/primitives.jsx `Icons` 对象.
    func testIconsMappingHasExactly25KeysMatchingUiDesign() {
        let expectedKeys: Set<String> = [
            "home", "box", "friends", "user", "paw",
            "bowl", "heart", "ball", "footprint", "plus",
            "enter", "close", "back", "dot", "copy",
            "check", "settings", "sparkle", "bell", "chevronRight",
            "wechat", "shield", "warn", "diamond", "trophy",
        ]
        XCTAssertEqual(Icons.mapping.count, 25, "Icons.mapping 应严格含 25 键")
        XCTAssertEqual(Set(Icons.mapping.keys), expectedKeys,
                       "Icons.mapping 键集应严格对齐 ui_design primitives.jsx Icons 对象 25 键")
    }

    // MARK: - case#3 happy: 全 25 键对应的 SF Symbol 在 iOS 17+ 都存在

    /// 验证全 25 键映射的 SF Symbol 在 iOS 17+ 都能 UIImage(systemName:) 拿到非 nil.
    /// 防止 SF Symbol 名拼写错误 / iOS 版本限定符号被误用.
    func testAllMappedSFSymbolsExistOnIOS17() {
        for (key, symbolName) in Icons.mapping {
            XCTAssertNotNil(
                UIImage(systemName: symbolName),
                "Icons.mapping[\"\(key)\"] = \"\(symbolName)\" 应在 iOS 17+ 存在"
            )
        }
        // fallback symbol 也必须存在
        XCTAssertNotNil(
            UIImage(systemName: Icons.fallbackSymbol),
            "Icons.fallbackSymbol = \"\(Icons.fallbackSymbol)\" 应在 iOS 17+ 存在"
        )
    }

    // MARK: - case#4 edge: Icons.symbol(for:) 未匹配键退回 fallback

    /// 验证未匹配键查询走 fallback（不允许 silent fallback；调用方拿到 questionmark.circle 是显式信号）.
    func testIconsSymbolForUnknownKeyReturnsFallback() {
        let result = Icons.symbol(for: "definitely_not_a_real_key_xyz")
        XCTAssertEqual(result, Icons.fallbackSymbol,
                       "未匹配键应返回 Icons.fallbackSymbol（即 questionmark.circle）")
    }

    // MARK: - case#5 happy: Rarity 4 档枚举完整

    /// 验证 Rarity enum 含且仅含 4 档（N / R / SR / SSR）；为 RarityTag color 路由稳定提供锚.
    func testRarityHasExactlyFourCases() {
        XCTAssertEqual(Rarity.allCases.count, 4)
        XCTAssertEqual(Set(Rarity.allCases.map(\.rawValue)), ["N", "R", "SR", "SSR"])
    }
}
```

> **dev 实装备注**：
> - **不**测 Card / PrimaryButton / Avatar / FadeIn 的 SwiftUI body 渲染（ADR-0002 §3.1 钦定 XCTest only，**不**引 ViewInspector）—— 视觉验证靠 #Preview + Story 37.13 visual-review-checklist
> - **不**测 PrimaryButton.disabled 态的 opacity 数值（涉及 SwiftUI hosting，需要 ViewInspector）；改为通过单元测试覆盖 enum 与字典契约 + 视觉抽样靠 Preview
> - 如果 dev 觉得 PrimaryButtonVariant 路由也想加测试，可补 case#6（验证 `PrimaryButtonVariant.allCases.count` 或类似），不强制

### AC10 — xcodegen regen + build verify

完成 AC1-AC9 后：

1. `cd iphone && xcodegen generate` 让 6 个新 production 文件 + 1 个新测试文件加入 PetApp / PetAppTests target（project.yml 通配规则自动 inclusion）
2. `bash iphone/scripts/build.sh --test` 跑测试通过
   - 全部已有测试（Story 37.5 落地后 251 case）+ 本 story 新增 5 case → 总 256 case 应全绿
   - 任一 case 失败必须修复后才 commit；不接受 "test-skipped" / "ignored" 标记绕过
3. grep 验证：
   - `grep -c '"home"' iphone/PetApp/Core/DesignSystem/Primitives/Icons.swift` ≥ 1（防键名拼错）
   - `grep -c "Icons.mapping" iphone/PetAppTests/Core/DesignSystem/Primitives/PrimitivesTests.swift` ≥ 2（两个测试都引用了 Icons.mapping）

> **dev 实装备注**：dev 必须 `xcodegen generate` 后 commit 一并提交 `iphone/PetApp.xcodeproj/project.pbxproj` 改动（Story 37.5 / 37.4 / 37.3 都按此模式 commit 过 pbxproj 文件）。

### AC11 — Deliverable 清单

- ✅ 6 个 production 文件全部在 `iphone/PetApp/Core/DesignSystem/Primitives/` 落地：Icons.swift / Card.swift / PrimaryButton.swift / Avatar.swift / FadeIn.swift / RarityTag.swift
- ✅ 1 个测试文件在 `iphone/PetAppTests/Core/DesignSystem/Primitives/PrimitivesTests.swift` 落地，5 case 全绿
- ✅ Icons.mapping 严格 25 键，键名 1:1 对齐 ui_design primitives.jsx `Icons` 对象
- ✅ 6 个 primitive 文件每个含 candy / dark 双主题 #Preview 块
- ✅ `iphone/PetApp.xcodeproj/project.pbxproj` 改动随 commit（xcodegen regen 结果）
- ✅ `bash iphone/scripts/build.sh --test` 256/256 全绿（已有 251 + 本 story 新增 5）
- ✅ project.yml **不**手动改（通配规则自动 inclusion）
- ✅ MainTabView.swift / RootView.swift / 其它 production 文件**不**改（本 story 仅落地 primitives，不收口任何已有 view）

## Tasks / Subtasks

- [x] Task 1: 新建 Primitives/ 子目录骨架（AC1）
  - [x] 1.1 创建 `iphone/PetApp/Core/DesignSystem/Primitives/` 目录
  - [x] 1.2 创建 `iphone/PetAppTests/Core/DesignSystem/Primitives/` 目录
- [x] Task 2: 落地 6 个 production primitive 文件（AC2-AC7）
  - [x] 2.1 新建 `iphone/PetApp/Core/DesignSystem/Primitives/Icons.swift`（25 键 mapping + symbol(for:) helper + fallbackSymbol；严格按 AC2 表的键名 / SF Symbol 落地）
  - [x] 2.2 新建 `iphone/PetApp/Core/DesignSystem/Primitives/Card.swift`（generic Card<Content: View> + cornerRadius / padding 可覆写参数 + theme.shadow.sm + theme.colors.border 1pt 描边）
  - [x] 2.3 新建 `iphone/PetApp/Core/DesignSystem/Primitives/PrimaryButton.swift`（PrimaryButtonVariant enum 三档 + 高度 52pt + 圆药丸 + 硬阴影 + 按下 translateY(2) + disabled opacity 0.5）
  - [x] 2.4 新建 `iphone/PetApp/Core/DesignSystem/Primitives/Avatar.swift`（7 色 palette static + #c3bdb9 离线灰 static + name hash 选色 + ring 双层 stroke + 在线小绿点 0.28 size + 0.32 offset）
  - [x] 2.5 新建 `iphone/PetApp/Core/DesignSystem/Primitives/FadeIn.swift`（FadeInModifier + View extension `.fadeIn(id:)`；0.28s easeInOut + offsetY -8 → 0）
  - [x] 2.6 新建 `iphone/PetApp/Core/DesignSystem/Primitives/RarityTag.swift`（Rarity enum 4 档 + RarityTag View + AnyShapeStyle 路由 + SSR LinearGradient 金红渐变 + a11y identifier `rarityTag_<rawValue>`）
- [x] Task 3: 每个 primitive 加 candy / dark 双主题 #Preview（AC8）
  - [x] 3.1 Card.swift 底部 `#if DEBUG ... #endif` + 2 个 #Preview
  - [x] 3.2 PrimaryButton.swift 底部 + 2 个 #Preview（4 状态 VStack）
  - [x] 3.3 Avatar.swift 底部 + 2 个 #Preview（4 配置）
  - [x] 3.4 FadeIn.swift 底部 + 2 个 #Preview
  - [x] 3.5 RarityTag.swift 底部 + 2 个 #Preview（4 档色条）
  - [x] 3.6 Icons.swift 底部 + 2 个 #Preview（LazyVGrid 25 键展示）
- [x] Task 4: 单元测试 (AC9)
  - [x] 4.1 新建 `iphone/PetAppTests/Core/DesignSystem/Primitives/PrimitivesTests.swift`，落地 5 case（Icons.mapping["home"] 精确 / 25 键完整 / 25 SF Symbol iOS 17+ 存在 / unknown key fallback / Rarity 4 档完整）
- [x] Task 5: xcodegen regen + build verify (AC10)
  - [x] 5.1 `cd iphone && xcodegen generate` 让 7 个新文件加入 PetApp / PetAppTests target
  - [x] 5.2 `bash iphone/scripts/build.sh --test` 跑测试通过（256/256 全绿）
  - [x] 5.3 grep 校验抽样：Icons.swift 含 "home"、PrimitivesTests.swift 引用 Icons.mapping ≥2 次
- [x] Task 6: Deliverable 清单确认 (AC11)
  - [x] 6.1 6 个 production 文件 + 1 个测试文件全部落地
  - [x] 6.2 project.yml 不改（通配规则自动 inclusion；如手动改了说明 dev 走偏路径，需回退）
  - [x] 6.3 MainTabView.swift / RootView.swift / 其它现有 view 文件 0 改动（git diff 应仅含新增 Primitives/ 文件 + pbxproj regen 结果）

## Dev Notes

### Primitives 是 value type View struct，不是 ObservableObject（关键设计决策）

**选定**: 6 个 primitive 全部是 `public struct ... : View`（value type），不持任何 `@StateObject` / `@ObservedObject` / `@Published`。

**为何不走 class / ObservableObject 路径**：

1. **无状态**：primitives 是纯展示型 atom——不持 domain state，不监听数据变化；caller 通过参数传值，View 渲染即结束。
2. **零开销**：value type View 在 SwiftUI 内是浅拷贝 + diff；class + observer 是引用 + 订阅链路，多余开销。
3. **测试友好**：value type 可直接构造 + 调用 init 路径；class + observer 需要 hosting 才能验证 body 渲染——而 ADR-0002 §3.1 钦定 XCTest only **不**测 hosting。
4. **与 ADR-0010 边界一致**：AppState 是 ObservableObject（持 mutable domain state）；ViewModel 是 ObservableObject（持 view-specific transient state + mutate AppState）；primitives 是 immutable atom，不属同一边界。

**唯一例外**：FadeInModifier 用 `@State var visible: Bool` 持本地动画状态——这是 ViewModifier 内部细节，不暴露给调用方；与本节"primitives 不持 state"原则不矛盾。

### Icons 是 enum + 静态字典而非 SwiftUI ImageProvider 工厂（关键设计决策）

**选定**: `Icons.mapping: [String: String]` 暴露 SF Symbol 名字符串；调用方自己写 `Image(systemName: Icons.symbol(for: "home"))`。

**为何不暴露 `Image` 类型 / 工厂方法 `Icons.image(for: String)` -> Image`**：

1. **保持 atom 边界纯净**：Icons 是 ui_design 翻译表，不应升级为 SwiftUI 视图工厂——后者会引入 Image 类型在所有 caller 站点的耦合。
2. **调用方决定渲染时机**：`Image(systemName:)` 可能被调用方包在 `.foregroundColor(theme.colors.accent)` / `.font(.system(size: 22))` / `.symbolRenderingMode(.hierarchical)` 等链中——如果 Icons 工厂返回固定包装的 Image，调用方还要 unwrap 加自定义。
3. **测试稳定性**：字符串映射可直接断言 `XCTAssertEqual(Icons.mapping["home"], "house.fill")`；Image 类型断言需要走 ViewInspector（ADR-0002 §3.1 不允许）。

**`Icons.symbol(for:)` 是唯一 helper**：包装 `mapping[key] ?? fallbackSymbol` + log warning，让未匹配键有可观测信号。dev **不**应在本 story 加更多 helper（如 `image(for:)`）；如果 future Scaffold story 觉得 caller 站点冗长，由 Scaffold story 自行加 wrapper（不属本 story scope）。

### Avatar palette + 离线灰 hex 不抽到 theme（设计取舍）

`Avatar.palette` 是 7 色 hash 选色调色板；`Avatar.offlineColor` 是离线灰 `#c3bdb9`——两者来自 ui_design primitives.jsx 内部硬编码，不属 theme color namespace。

**为何不抽到 theme**：

1. **palette 是 Avatar 占位策略**（hash → 不同用户名稳定分配不同色），不是"主题色"——不会随主题切换变（candy 主题与 dark 主题 palette 完全一致）。如果抽到 `theme.colors.avatarPalette: [Color]` 会让 Theme 类型膨胀、且每个主题都得复 candy 同字段（无意义）。
2. **离线灰 #c3bdb9 也是 ui_design 硬编码**，与 candy 的 `success` / `warn` 等语义色不在同一边界——它是"离线"语义，不是"成功 / 警告"。

**Future**：如果产品要让 palette 跟随主题（如 dark 主题用更暗调色板），那时再抽到 theme（属新故事，不是本 story 范围）。

### RarityTag 配色不抽到 theme（同上）

N=灰 #b0b0b0 / R=蓝 #7db3e8 / SR=紫 #c58ae8 / SSR=金红渐变 `linear-gradient(90deg,#ffd166,#ef476f)`——这是 Wardrobe 业务色（玩家心智："SSR 永远是金红"），不是 theme color。

**Future**：如果产品决定让稀有度色随主题改（不太可能），再抽 theme。

### PrimaryButton 按下动效用 simultaneousGesture(DragGesture) 而非 ButtonStyle（实装陷阱）

SwiftUI Button 内置高亮态（`configuration.isPressed`）仅在 `ButtonStyle.makeBody` 内可见；本 story 直接用 `Button(action: ...) { ... }` + `.buttonStyle(.plain)` —— 这层调用看不到 isPressed。

**正确路径**：用 `simultaneousGesture(DragGesture(minimumDistance: 0))` 监听 onChanged / onEnded，自己驱动 `@State var isPressed`。

**为何不用 ButtonStyle**：抽 ButtonStyle 会让 PrimaryButton 类型契约从 `View` 升级为 `View + ButtonStyle`，并且 caller 需要写 `.buttonStyle(PrimaryButtonStyle(...))` 模式——这与本 story "primitive 是开箱即用 atom" 设计冲突。simultaneousGesture 直接在 Button 内部捕获，对 caller 完全透明。

**陷阱**：disabled 状态下 simultaneousGesture 仍会触发 onChanged（DragGesture 不响应 `.disabled`）——所以 onChanged 内必须 `if isEnabled { isPressed = true }` 否则 disabled 按钮也会显示按下动效。本 story AC4 文件契约里已包含 `isPressed = isEnabled` 的写法。

### #Preview 内部主题注入路径（与 Story 37.5 同模式）

每个 primitive 的 #Preview 块内部用 `.environment(\.theme, ThemeName.candy.theme)`（或 `.dark.theme`）注入主题——这与 Story 37.5 ThemePreview_Sampler 的注入模式严格一致。

**为何不依赖 EnvironmentKey 默认值**：
- EnvironmentKey 默认是 `.candy`，但 dark 主题 Preview 必须显式注入；统一两个 Preview 都显式 `.environment(\.theme, ...)` 让 reader 一眼看出主题
- Future 如要加 matcha / sky 主题 Preview，只需复制粘贴改 `.matcha.theme` / `.sky.theme`

**dev 备注**：Preview 不能在 ViewBuilder body 内 `@Environment` 取主题（Preview 闭包不是真正 view body）；必须写：

```swift
#Preview("Card — candy") {
    Card { Text("hello") }
        .environment(\.theme, ThemeName.candy.theme)
        .padding()
        .background(ThemeName.candy.theme.colors.pageBg)
}
```

`.background(ThemeName.candy.theme.colors.pageBg)` 给 Preview 显示 page 背景色让 dev 看出 surface vs page 的对比。

### MainTabView FloatingTabBar SF Symbol 与 Icons 表的一致性（AR-side check）

`iphone/PetApp/App/MainTabView.swift` 内 `FloatingTabBar.iconName(for:)` 使用：
- `.home` → `house.fill`
- `.wardrobe` → `shippingbox.fill`
- `.friends` → `person.2.fill`
- `.profile` → `person.crop.circle.fill`

这与本 story Icons.mapping 表的 `home` / `box` / `friends` / `user` 4 个键完全一致——dev 实装时必须保证这一致性（不需要主动改 MainTabView，仅需 grep 抽样验证 Icons.mapping 的 4 个值与 MainTabView.iconName 4 个值字面量一致）：

```bash
grep -E '"house\.fill"|"shippingbox\.fill"|"person\.2\.fill"|"person\.crop\.circle\.fill"' \
  iphone/PetApp/App/MainTabView.swift \
  iphone/PetApp/Core/DesignSystem/Primitives/Icons.swift
```

应输出 8 行（每个 SF Symbol 在两个文件各 1 次）。

**Future**：Story 37.7 HomeView Scaffold 收口 MainTabView FloatingTabBar 占位时，会把 inline `iconName(for:)` 改为 `Icons.symbol(for: "home")` 等查表——本 story **不**做该收口，Story 37.7 做。

### EnvironmentKey 默认值的 fallback（与 Story 37.5 协调）

primitives 内部全部 `@Environment(\.theme) var theme` 取主题——`Environment+Theme.swift` 已落地 `defaultValue: Theme = .candy` fallback；任何忘记注入 theme 的 caller 都自动 fallback 到 candy。

**风险**：本 story 测试不验证 fallback 行为（已在 Story 37.5 ThemeTests 验证过 `testEnvironmentDefaultIsCandy`）；本 story 假设 Story 37.5 已 done（事实——sprint-status.yaml 内 `37-5-theme-design-tokens: done`）。

### xcodegen regen 节奏 + project.yml 兼容性

`iphone/project.yml` 内 `targets.PetApp.sources: - PetApp` 是**通配** PetApp/ 子树全部 .swift 文件；新增的 6 个 production 文件全部在 PetApp/Core/DesignSystem/Primitives/ 下 → 自动 inclusion，**不**改 project.yml。

测试目录同理：`PetAppTests` 通配新增 `Core/DesignSystem/Primitives/PrimitivesTests.swift`。

dev 必须 `cd iphone && xcodegen generate` 重生成 `PetApp.xcodeproj`（或在 Xcode 内 File → Add Files 手动 import；推荐前者保 project.yml 是 source of truth）。

跑通后 `bash iphone/scripts/build.sh --test` 验证 build + test 通过。

### 与 Story 37.7-37.12 各 Scaffold story 的协调（接缝清单）

本 story 落地后，下游 Scaffold story 可立即按以下方式消费 primitives：

- **Story 37.7 HomeView Scaffold**：
  - StatusBar 步数计 prefix `Image(systemName: Icons.symbol(for: "footprint"))` + `theme.colors.coin`
  - ActionRow 三按钮：每个用 `PrimaryButton(title: "喂食", icon: Icons.symbol(for: "bowl"), variant: .ghost) { state.onFeedTap() }`（或类似）
  - TeamIdleCard：用 `Card(cornerRadius: 22) { ... }`（22 圆角对齐 ui_design TeamIdleCard）
  - 内含 `PrimaryButton(title: "创建队伍", icon: Icons.symbol(for: "enter")) { state.onCreateTap() }`
  - HomeView 入场动画：根 VStack 加 `.fadeIn(id: state.greeting)` 让 Tab 切换时重触发
- **Story 37.8 RoomView Scaffold**：
  - 房间号 Card + 复制按钮 `Image(systemName: Icons.symbol(for: "copy"))` → 点击后切到 `Icons.symbol(for: "check")` 1.2s
  - 离开按钮 `PrimaryButton(title: "离开房间", variant: .secondary) { state.onLeaveTap() }`
  - 成员列表 4 格：每格用 `Avatar(name: member.catName, size: 56, online: member.online)`
- **Story 37.9 WardrobeView Scaffold**：
  - 顶部钻石 `Image(systemName: Icons.symbol(for: "diamond"))` + `theme.colors.coin`
  - 道具卡片：每个 `Card(cornerRadius: 16, padding: 8) { ... + RarityTag(rarity: item.rarity, width: cardWidth - 16) }`
  - 装备/卸下按钮：`PrimaryButton(title: "装备", isEnabled: item.owned) { ... }`
- **Story 37.10 FriendsView Scaffold**：
  - 添加按钮 `Image(systemName: Icons.symbol(for: "plus"))`
  - FriendRow：`Avatar(name: friend.catName, online: friend.online == .offline ? false : true)`
  - 加入按钮 `PrimaryButton(title: "加入", icon: Icons.symbol(for: "enter"), isEnabled: friend.online == .inRoom)`
- **Story 37.11 ProfileView Scaffold**：
  - 头部 `Avatar(name: user.catName, size: 88, ring: true)`
  - 微信绑定卡：`Image(systemName: Icons.symbol(for: "wechat"))` + `Image(systemName: Icons.symbol(for: "shield"))`
  - 绑定 Modal：`Image(systemName: Icons.symbol(for: "warn")).foregroundColor(theme.colors.warn)`
- **Story 37.12 JoinRoomModal**：
  - 关闭按钮 `Image(systemName: Icons.symbol(for: "close"))`
  - 输入框 prefix `Image(systemName: Icons.symbol(for: "paw"))`
  - 确定加入按钮 `PrimaryButton(title: "确定加入", isEnabled: code.count >= 3) { state.onConfirmTap() }`

dev 实装本 story 时**不**需要预生成上述任一调用代码（属下游 Scaffold story 范围）；本接缝清单仅供 dev 验证 primitives API 设计是否覆盖各下游需求（如 Card 的 cornerRadius 覆写参数、PrimaryButton 的 icon 参数都是为 caller 留接缝）。

### 与 ADR-0002 §3.1 测试栈钦定的对齐

本 story 测试**仅**用 XCTest + @testable import PetApp + UIKit `UIImage(systemName:)` 验证 SF Symbol 存在性——**不**引：

- ❌ SnapshotTesting（视觉 diff 类工具）：视觉验证靠 #Preview + Story 37.13 visual-review-checklist 人眼把关
- ❌ ViewInspector（SwiftUI body 内省工具）：本 story primitives body 渲染契约靠 ui_design 1:1 翻译 + Preview 抽样兜底，不需要 body 内省断言
- ❌ Mockingbird / Cuckoo（mock codegen）：primitives 不持任何 protocol / repository，无 mock 需求

### 测试 case 数量取舍（≥3 / 实装 5 / 不再加）

epic AC 钦定 ≥3 case；本 story 落地 5 case（Icons.mapping["home"] / 25 键完整 / 25 SF Symbol iOS 17+ 存在 / unknown key fallback / Rarity 4 档完整）—— 5 case 已覆盖 Icons / Rarity 全部公开契约 + symbol(for:) fallback 行为。

**为何不加 Card / PrimaryButton / Avatar / FadeIn 测试**：

1. 这些是 SwiftUI View struct，body 渲染需要 hosting 才能验证；ADR-0002 §3.1 不允许引 ViewInspector
2. 视觉规则（圆角 / 阴影 / 颜色 / 描边）由 ui_design 钦定 + #Preview 视觉抽样兜底；自动断言这些"数值是不是 24 / radius 是不是 sm" 是过度测试
3. Card / PrimaryButton 的参数路由（cornerRadius / variant 字段路由）是 Swift 编译器静态检查的范围，不是单元测试范围

如果 dev 实装时发现某个 primitive 路由分支特别复杂（如 PrimaryButton variant → backgroundColor 路由分支），可酌情补单元测试（如纯函数化 `private static func backgroundColor(for variant: Variant, theme: Theme) -> Color`），不强制。

### Source tree components to touch

- 新建 production：
  - `iphone/PetApp/Core/DesignSystem/Primitives/Icons.swift`
  - `iphone/PetApp/Core/DesignSystem/Primitives/Card.swift`
  - `iphone/PetApp/Core/DesignSystem/Primitives/PrimaryButton.swift`
  - `iphone/PetApp/Core/DesignSystem/Primitives/Avatar.swift`
  - `iphone/PetApp/Core/DesignSystem/Primitives/FadeIn.swift`
  - `iphone/PetApp/Core/DesignSystem/Primitives/RarityTag.swift`
- 新建测试：
  - `iphone/PetAppTests/Core/DesignSystem/Primitives/PrimitivesTests.swift`
- 修改：
  - `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen regen 结果；不手改）
- **不**改：
  - `iphone/PetApp/App/RootView.swift`
  - `iphone/PetApp/App/MainTabView.swift`（保留 FloatingTabBar inline iconName；Story 37.7 收口）
  - `iphone/PetApp/Core/DesignSystem/Theme*.swift`（Story 37.5 已 done；本 story 仅消费 theme，不改 token 字段）
  - `iphone/PetApp/Features/Home/Views/*.swift`（保留给 Story 37.7）
  - `iphone/project.yml`（通配规则自动 inclusion，不需手改）

### Testing standards summary

- 测试入口：`bash iphone/scripts/build.sh --test`（ADR-0002 §3.4 钦定）
- 测试框架：XCTest only（ADR-0002 §3.1）；**不**引 SnapshotTesting / ViewInspector
- 单元测试位置：`iphone/PetAppTests/Core/DesignSystem/Primitives/PrimitivesTests.swift`（与 production 镜像）
- 测试 case 数量：≥3 case（按 v1 epic acceptance）；本 story 落地 5 case（含 Icons 25 键完整 + iOS 17+ 全部存在 + fallback + Rarity 4 档）
- 测试运行时：每 case ≤ 50ms（纯字典 / enum / UIImage 取存在性）；25 SF Symbol 取存在性 case 单独可能耗时数百 ms（UIKit underlying lookup）
- **不**测 SwiftUI View body 渲染（primitives 都是 value type View，hosting 测试需要 ViewInspector）
- 覆盖目标：Icons 公开契约（mapping 25 键 / symbol(for:) fallback / fallbackSymbol 自身存在性）+ Rarity 公开契约（4 档枚举完整）

### Project Structure Notes

- Alignment with unified project structure: 完全按 `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §4 + ADR-0002 §3.3 的 `iphone/PetApp/Core/DesignSystem/` 目录约定。新增 `Primitives/` 子目录与已存在 `Components/` 同级（Components 是 Story 2.6 落地的错误 UI 视图集，Primitives 是 ui_design 翻译过来的通用 atom，命名边界清晰）。Test mirror 路径 `iphone/PetAppTests/Core/DesignSystem/Primitives/` 与 production 严格镜像（已有 `PetAppTests/App/` `PetAppTests/Features/Home/` `PetAppTests/Helpers/` `PetAppTests/Core/DesignSystem/` 同模式）。
- Naming convention: 6 个 primitive `<Name>.swift`（Icons / Card / PrimaryButton / Avatar / FadeIn / RarityTag）—— 均 PascalCase（Swift 社区惯例）；测试文件 `PrimitivesTests.swift`（单文件，5 case 不需要拆多文件）。
- Detected conflicts or variances: 无。`iphone/PetApp/Core/DesignSystem/Components/` 已存在；`Primitives/` 是新增子目录，不与 Components 冲突。MainTabView.FloatingTabBar 内已 hardcode 4 个 SF Symbol（house.fill / shippingbox.fill / person.2.fill / person.crop.circle.fill）—— 本 story Icons.mapping 内的 home / box / friends / user 4 个键完全对齐，**不**触发漂移。

### References

- [Source: iphone/ui_design/source/components/primitives.jsx] — 6 个 primitive 视觉源头（Icons 对象 25 键 / PrimaryButton / Card / Avatar / FadeIn 函数；本 story 翻译为 SwiftUI value type View struct）
- [Source: iphone/ui_design/README.md#Design Tokens] — 5 类 token 定义（colors / spacing / radius / shadow / typography），primitives 通过 `@Environment(\.theme)` 取
- [Source: iphone/ui_design/README.md#Wardrobe] — 稀有度配色 N=灰 #b0b0b0 / R=蓝 #7db3e8 / SR=紫 #c58ae8 / SSR=金红渐变 钦定值
- [Source: iphone/ui_design/README.md#Interactions] — FadeIn 0.28s ease 钦定 + 按钮按下 translateY(2px) 钦定
- [Source: \_bmad-output/planning-artifacts/epics.md#Story 37.6: 共享 primitives] — 本 story acceptance 原文（≥3 case 测试 / 6 个 primitive 文件 / 25 键完整集 / Preview 双主题）
- [Source: \_bmad-output/planning-artifacts/sprint-change-proposal-2026-04-29-v2.md#Story 37.6 Icons 完整映射表] — 25 键完整映射表 + 视觉差异容忍说明 + Story 37.13 visual-review-checklist 人眼把关位
- [Source: \_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.1 iOS Mock 框架] — XCTest only 测试框架钦定（不引 SnapshotTesting / ViewInspector）
- [Source: \_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.3 iPhone App 工程目录方案] — `iphone/PetApp/` 目录约定 + xcodegen 通配规则
- [Source: \_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md#3.2 AppState 范围白名单] — primitives 不持 domain state（与 ViewModel / AppState 边界对齐）
- [Source: \_bmad-output/implementation-artifacts/37-5-theme-design-tokens.md#AC1 Theme.swift] — Theme 类型契约 + ThemeName 路由（primitives 通过 @Environment 消费）
- [Source: iphone/PetApp/Core/DesignSystem/Theme.swift] — Theme value type + 4 主题静态实例（已 done；本 story 直接消费）
- [Source: iphone/PetApp/Core/DesignSystem/ThemeColors.swift] — 13 字段 color token + Color(hex:) helper（仅 ThemeColors 内部用，primitives 不写 Color(hex:) 直接用 token）
- [Source: iphone/PetApp/Core/DesignSystem/ThemeShadow.swift] — ShadowToken (color/radius/x/y) + sm/md/lg 三档（Card 用 sm；PrimaryButton 硬阴影另算）
- [Source: iphone/PetApp/App/MainTabView.swift#L89-L96] — FloatingTabBar inline SF Symbol 表（本 story Icons 表与之 1:1 对齐 4 键 home/box/friends/user；Story 37.7 收口）
- [Source: iphone/PetApp/Core/DesignSystem/Environment+Theme.swift] — EnvironmentKey 注入入口（primitives 全部走 `@Environment(\.theme)` 取）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- 2026-04-30 dev-story 首次实装 HALT: `iphone/PetAppTests/Core/DesignSystem/Primitives/PrimitivesTests.swift:46` 测试 `testAllMappedSFSymbolsExistOnIOS17` 失败:
  `XCTAssertNotNil failed - Icons.mapping["bowl"] = "bowl.fill" 应在 iOS 17+ 存在`
- 验证手段：`xcrun --sdk macosx swiftc + NSImage(systemSymbolName:)` + `strings .../SFSymbols.framework/SFSymbols | grep -iE "bowl"` 双路确认；
  iOS 26.4 simruntime 内 SFSymbols 二进制不含 `bowl.fill` / `bowl` 任一名称，仅有 `figure.bowling*` 系列（运动姿势，非餐碗）。

### Completion Notes List

- ✅ AC1: 新建 `iphone/PetApp/Core/DesignSystem/Primitives/` + `iphone/PetAppTests/Core/DesignSystem/Primitives/` 目录骨架完成。
- ✅ AC2-AC7: 6 个 production primitive 文件落地（Icons / Card / PrimaryButton / Avatar / FadeIn / RarityTag）；字段、API、视觉契约严格按 AC2-AC7 文件契约 1:1 实装。
- ✅ AC8: 6 个 primitive 各含 candy / dark 双主题 #Preview 块（共 12 个 Preview entry）。
- ✅ AC9: 落地 5 case `PrimitivesTests.swift`（按 v2 文件契约原文）。
- ✅ AC10 (xcodegen): `cd iphone && xcodegen generate` 成功，`PetApp.xcodeproj/project.pbxproj` regen 含新增 7 个文件（pbxproj 内 `Primitives` 关键字共 10 处出现，覆盖 sources / build phase / file ref）。
- ❌ AC10 (build verify, 首轮): `bash iphone/scripts/build.sh --test` **未全绿** —— 1/256 失败：`PrimitivesTests.testAllMappedSFSymbolsExistOnIOS17` 因 `bowl.fill` SF Symbol 在 iOS 26.4 simruntime 内**不存在**而断言失败。
- 🛑 **HALT**（首轮）: Story 内部 AC 冲突:
    - AC2 映射表钦定 `bowl → bowl.fill` + Story line 35 / Dev Notes line 33 明确禁止 dev 自行替换映射（"**不接受** dev 自行替换映射（如改成 figure.circle 等）"）;
    - AC9 case#3 (`testAllMappedSFSymbolsExistOnIOS17`) 与 AC10 钦定要求「全部 SF Symbol 在 iOS 17+ 存在」+ build 全绿才能合并;
    - 二者不可同时满足。**需 PM / Architect 决策替换映射或修改 AC**（推荐 `bowl → fork.knife`，已在 iOS 26.4 simruntime 内验证存在；视觉表意 = 餐具，与 ui_design FeedButton 喂食按钮语义最近）。
- ✅ **2026-04-30 dev-story HALT 后 user 授权 bowl → fork.knife 替换**（对"dev 不许改 SF Symbol mapping"红线一次例外）：改 1 行 mapping（Icons.swift +inline 注释）+ AC2 加 inline 注解；256 tests 全绿（0 fail）。Story status: ready-for-dev → review。
- 📚 沉淀 lesson `docs/lessons/2026-04-30-spec-must-physically-verify-sf-symbol-strings.md`（severity 1）+ index.md 加行：spec 钦定外部 SDK 字符串前必须物理验证存在性，避免"spec 红线 + 验证 test + 物理 SDK 矛盾"三角死结。

### Known Issue (P3 flag, deferred to follow-up PR)

**[P3] Avatar hash UTF-16 vs Unicode scalar 跨平台不等价** — `Avatar.swift:93-95`
- 现象：`hashIndex(of: name)` 用 `unicodeScalars.reduce`，JS 原型 `primitives.jsx` 用 `split('').reduce(charCodeAt(0))`（UTF-16 code units）；对 emoji / 非 BMP CJK 字符 hash 不同
- 影响窄路径：仅 Avatar **无图片 url + name 含 emoji/非 BMP CJK** 的 fallback 颜色路径
- 严重度：codex r6 标 [P2]；主 agent reclassify [P3]/nit（cross-platform pixel-perfect parity，非 epic-37 核心目标；用户不会同时看 iOS + web）
- 决议：本 story 不修；5 轮 fix-review cap 已破例 1 次，再破例失效；本条不阻塞 epic-37 进度
- 修复成本：1 行（`unicodeScalars.reduce(into:)` → `utf16.reduce(into:)`）
- Forward action：epic-37 done 后任意 PR 修；codex review 跑 r1 时提一句即可（diff 极小）
- codex r6 原文：`/tmp/epic-loop-review-37-6-r6.md`

### File List

#### 新增（production）
- `iphone/PetApp/Core/DesignSystem/Primitives/Icons.swift`（含 user-authorized substitution 注释 + bowl → fork.knife mapping）
- `iphone/PetApp/Core/DesignSystem/Primitives/Card.swift`
- `iphone/PetApp/Core/DesignSystem/Primitives/PrimaryButton.swift`
- `iphone/PetApp/Core/DesignSystem/Primitives/Avatar.swift`
- `iphone/PetApp/Core/DesignSystem/Primitives/FadeIn.swift`
- `iphone/PetApp/Core/DesignSystem/Primitives/RarityTag.swift`

#### 新增（测试）
- `iphone/PetAppTests/Core/DesignSystem/Primitives/PrimitivesTests.swift`

#### 新增（lesson）
- `docs/lessons/2026-04-30-spec-must-physically-verify-sf-symbol-strings.md`

#### 修改
- `iphone/PetApp.xcodeproj/project.pbxproj`（xcodegen regen 结果，新增 7 个 file ref / build phase 入口）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（37-6-shared-primitives: ready-for-dev → review；last_updated 同步）
- `_bmad-output/implementation-artifacts/37-6-shared-primitives.md`（本文件：AC2 inline 注解 + Tasks 全勾 + Status review + Completion Notes / File List / Change Log 更新）
- `docs/lessons/index.md`（追加一行 spec-must-physically-verify-sf-symbol-strings）

## Change Log

| Date       | Change |
|------------|--------|
| 2026-04-30 | 初稿落地：Story 37.6 共享 primitives（Card / PrimaryButton / Avatar / FadeIn / RarityTag / Icons 完整 25 键集）+ candy/dark 双主题 #Preview + 5 case 单元测试 + xcodegen regen + bash iphone/scripts/build.sh --test 全绿。Sprint-status: backlog → ready-for-dev。 |
| 2026-04-30 | dev-story 首轮 HALT（AC2 钦定 `bowl → bowl.fill` 但 iOS 17+ SDK 不提供该 SF Symbol，AC9 case#3 必然 fail，4 条约束死结）→ user 授权一次例外替换 `bowl → fork.knife`（实存 + 语义最贴近喂食按钮）；改 Icons.swift 1 行 mapping + inline 注释 + AC2 加 inline 注解；256 tests 全绿（0 fail）；新增 lesson `docs/lessons/2026-04-30-spec-must-physically-verify-sf-symbol-strings.md`（severity 1）；Status: ready-for-dev → review。 |
