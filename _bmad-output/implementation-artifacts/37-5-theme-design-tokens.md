# Story 37.5: Theme & Design Tokens（candy 完整 + 三主题 stub）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iOS 开发,
I want 一套 Theme 系统能注入 colors / spacing / radius / shadow / typography 五类 token 到 SwiftUI 视图树,
so that 所有 Feature View 通过 `@Environment(\.theme)` 取色取间距取字号，candy 主题像素级对齐 ui_design，三主题（matcha / sky / dark）切换零代码改动.

## 故事定位（Epic 37 第三层第 1 条 story；与 Story 37.6 同层、可并行；下游 37.7–37.12 5 屏 Scaffold + 37.3 MainTabView 后续优化全部依赖本 story 的 Theme）

这是 Epic 37「iPhone 架构层重构 + UI Scaffold（壳先行）」的**第三层 story**之一（与 Story 37.6 同层；上游 Story 37.3 MainTabView + Story 37.4 AppState 已 done）。本 story 是 **UI 基础类**——属于 Scaffold 共性约束「数据完全 mock + 禁 import APIClient + 视觉像素级匹配 ui_design」适用范围。

**本 story 落地后立即解锁**：
- Story 37.6 共享 primitives（Card / PrimaryButton / Avatar / FadeIn / RarityTag / Icons 全部依赖 theme.colors / theme.spacing / theme.radius / theme.shadow / theme.typography）
- Story 37.7–37.11 5 屏 Scaffold（HomeView / RoomView / WardrobeView / FriendsView / ProfileView）每屏视觉都靠 `@Environment(\.theme).colors.X` 取 token
- Story 37.12 JoinRoomModal 卡片 / 输入框样式依赖 theme.colors.surface / theme.radius
- 历史占位收口：Story 37.3 MainTabView FloatingTabBar 内硬编码 `Color(.systemBackground)` / `Color.black.opacity(0.14)` / `cornerRadius(20)`（详见 `iphone/PetApp/App/MainTabView.swift:68-70` 注释「Story 37.5 落地后 Color 改用 theme.colors / shadow 改用 theme.shadow」）—— 本 story **不强制** 在本期同步把 MainTabView 改为 theme（保留给 Story 37.6 + 37.7 一并做更安全），但本 story 的 Theme 类型契约必须**支撑**该改写（即 colors.surface / shadow.md / radius.lg 在 Theme 内必须就位）。

**本 story 的"实装"动作**（一句话概括）：在 `iphone/PetApp/Core/DesignSystem/` 下**从空白构建** Theme 系统：6 个 token 容器结构（Theme + 5 类 sub-struct）+ 1 个 EnvironmentKey 入口 + 4 个 ThemeName 实例（candy 完整、matcha / sky / dark stub）+ RootView 注入 `@State var currentTheme: ThemeName = .candy` + `.environment(\.theme, currentTheme.theme)` modifier。Theme 系统**纯类型 + 纯静态值**——无 ObservableObject、无 @Published、无运行时切换 UI（v2.4 钦定本期仅 Theme stub，UI 切换面板留给后续 mini-epic）。

**关键路径："纯静态值类型" vs "可观察 source"**（本 story 关键设计决策）：
- Theme 是 **`struct`**（非 class、非 ObservableObject）——@Environment 注入 value type，每次 currentTheme 切换 RootView 重建子树即可；**不**走 @StateObject / @Published 路径
- ThemeColors / ThemeSpacing / ThemeRadius / ThemeShadow / ThemeTypography 都是 **`struct`** + **`let` 字段**（不可变），通过 init 构造完整字段
- 每个 ThemeName 对应一个 **静态 const Theme 实例**（如 `Theme.candy` / `Theme.matcha` / `Theme.sky` / `Theme.dark`），通过 `static let` 暴露
- Theme 切换通过 `RootView` 的 `@State var currentTheme: ThemeName = .candy` + `.environment(\.theme, currentTheme.theme)` 手动改 `currentTheme` 的值触发 SwiftUI 重渲染；本期**不**做 UI 切换面板（按 ui-design-scope-whitelist Story 37.14）
- caller 漏改靠**编译器报错**驱动（任何代码读 `theme.colors.accent` 但 Theme 没有 `accent` 字段会立即编译失败）

**不涉及**（红线）：
- **不**实装三主题切换 UI（按 Story 37.14 白名单：「三主题切换 UI → 不做（仅 Theme stub）→ 后续 mini-epic」）；本期 RootView 硬编码 `currentTheme = .candy`
- **不**实装 matcha / sky / dark 主题的**完整** token 表；按 Sprint Change Proposal v2 acceptance 「matcha / sky / dark 主题：enum case 抽象齐全 + token 表 stub（每个字段 TODO 注释，给值用 candy 同字段 placeholder）」——除 ui_design README §Design Tokens 内已显式列的 candy 不同字段外，其余字段全部复用 candy 同字段 placeholder
- **不**改 MainTabView（保留 Story 37.3 落地的硬编码 Color/shadow；本 story 不强制收口该占位）—— 但为防止"双源"漂移，dev 实装时检查 MainTabView 注释「Story 37.5 落地后 Color 改用 theme.colors」是否仍指向后续 story（Story 37.6/37.7 收口），如指向当前 story 则在本 story 同步改为指向 Story 37.7（Sprint Change Proposal v2.4 钦定 Story 37.7 HomeView 落地时同步把 FloatingTabBar 改 theme）
- **不**改 ios/ 任何文件（CLAUDE.md + ADR-0002 §3.3 强约束）
- **不**改 server/ 任何文件
- **不**实装共享 primitives（Story 37.6 负责）
- **不**实装 Color asset catalog 化（本 story 用 hex literal 在 ThemeColors 内构造 Color；将来如需 Asset Catalog 化另起 spike）
- **不**新建 ObservableObject / @Published 模型（Theme 是 value type）

## Acceptance Criteria

> **AC 编号体系**：AC1–AC6 严格映射 Sprint Change Proposal v2 + epics §Story 37.5 acceptance；AC7 是单元测试；AC8 是 deliverable + commit。

### AC1 — 新建 Theme.swift（顶层入口 + ThemeName enum）

**新建文件 `iphone/PetApp/Core/DesignSystem/Theme.swift`**：

```swift
import SwiftUI

/// ThemeName: 主题名空间 enum.
///
/// CaseIterable 让未来主题切换 UI（白名单 Story 37.14 后续 mini-epic）能 ForEach.
/// raw value 是字符串以便 a11y identifier / 持久化（如 UserDefaults 存当前主题）.
public enum ThemeName: String, CaseIterable, Identifiable {
    case candy
    case matcha
    case sky
    case dark

    public var id: String { rawValue }

    /// 返回对应的 Theme 静态实例.
    /// 设计：用 switch 分发到 Theme.candy / .matcha / .sky / .dark，让 caller 写
    /// `currentTheme.theme.colors.accent` 风格的链式访问.
    public var theme: Theme {
        switch self {
        case .candy: return .candy
        case .matcha: return .matcha
        case .sky: return .sky
        case .dark: return .dark
        }
    }
}

/// Theme: 设计 token 顶层容器.
///
/// **类型选择**: `struct` + `let` 字段（不可变）→ value type，@Environment 注入零开销，
/// 切换主题靠重建（RootView `@State currentTheme` 改值 → SwiftUI 重渲染子树）.
///
/// **范围**: 5 类 sub-token 容器（colors / spacing / radius / shadow / typography），
/// 严格对齐 `iphone/ui_design/README.md` §Design Tokens.
///
/// **不含**: 主题元数据（name / description）—— 那归 ThemeName enum；
/// 不含 ObservableObject / @Published—— 那是 Theme 切换 UI 的实现细节，本期不做.
public struct Theme: Equatable {
    public let colors: ThemeColors
    public let spacing: ThemeSpacing
    public let radius: ThemeRadius
    public let shadow: ThemeShadow
    public let typography: ThemeTypography

    public init(
        colors: ThemeColors,
        spacing: ThemeSpacing,
        radius: ThemeRadius,
        shadow: ThemeShadow,
        typography: ThemeTypography
    ) {
        self.colors = colors
        self.spacing = spacing
        self.radius = radius
        self.shadow = shadow
        self.typography = typography
    }
}

// MARK: - 静态实例

extension Theme {
    /// candy（糖果粉，默认浅色）—— 完整实装；token 全部对齐 ui_design/README §Design Tokens.
    public static let candy = Theme(
        colors: .candy,
        spacing: .standard,
        radius: .standard,
        shadow: .candy,
        typography: .standard
    )

    /// matcha（抹茶）—— stub: 仅 colors.accent / accent-soft / accent-deep 来自 ui_design;
    /// 其余字段全部复用 candy 同字段 placeholder（每个字段须有 TODO 注释指向后续 mini-epic）.
    public static let matcha = Theme(
        colors: .matcha,
        spacing: .standard,
        radius: .standard,
        shadow: .candy, // TODO(Story-Future): matcha shadow 设计待定，当前复用 candy
        typography: .standard
    )

    /// sky（天空）—— stub.
    public static let sky = Theme(
        colors: .sky,
        spacing: .standard,
        radius: .standard,
        shadow: .candy, // TODO(Story-Future): sky shadow 设计待定，当前复用 candy
        typography: .standard
    )

    /// dark（深色模式）—— stub: 仅 colors.pageBg / surface / ink 来自 ui_design 显式表述;
    /// 其余字段以 candy 字段做 placeholder（每个 placeholder 须有 TODO 注释）.
    public static let dark = Theme(
        colors: .dark,
        spacing: .standard,
        radius: .standard,
        shadow: .candy, // TODO(Story-Future): dark shadow 设计待定，当前复用 candy
        typography: .standard
    )
}
```

**关键约束**：
- `Theme` / `ThemeColors` / `ThemeSpacing` / `ThemeRadius` / `ThemeShadow` / `ThemeTypography` 全部 `public struct` + `let` 字段（不可变 value type）
- `Theme: Equatable` 让单元测试可写 `XCTAssertEqual(theme, .candy)` 风格断言
- 4 个 ThemeName 全部走 `static let` 暴露 Theme 实例（init 一次、运行时不变）
- TODO 注释格式：`// TODO(Story-Future): <主题名> <token 类> 设计待定，当前复用 candy`（统一格式让 grep `TODO(Story-Future)` 可一次性列出所有 stub）

### AC2 — 新建 ThemeColors.swift（13 字段，candy 完整 + 三主题 stub）

**新建文件 `iphone/PetApp/Core/DesignSystem/ThemeColors.swift`**：

```swift
import SwiftUI

/// ThemeColors: 13 个语义 color token. 字段名 1:1 对齐 ui_design/README §Design Tokens.
///
/// **命名转换** (CSS 变量 → Swift property):
///   --page-bg     → pageBg
///   --accent      → accent
///   --accent-soft → accentSoft
///   --accent-deep → accentDeep
///   --surface     → surface
///   --surface-2   → surface2
///   --ink         → ink
///   --ink-soft    → inkSoft
///   --ink-mute    → inkMute
///   --success     → success
///   --warn        → warn
///   --coin        → coin
///   --border      → border
///
/// **dark 主题字段语义反转** (ui_design README §Design Tokens 钦定):
///   "深色模式: page-bg #2a1c22, surface #3a2831, ink #fbe5ec（其余配色自动反转）"
///   本期 stub: 仅显式落地 README 列出的 3 字段；其余字段复用 candy placeholder + TODO 注释.
public struct ThemeColors: Equatable {
    public let pageBg: Color
    public let accent: Color
    public let accentSoft: Color
    public let accentDeep: Color
    public let surface: Color
    public let surface2: Color
    public let ink: Color
    public let inkSoft: Color
    public let inkMute: Color
    public let success: Color
    public let warn: Color
    public let coin: Color
    public let border: Color

    public init(
        pageBg: Color,
        accent: Color,
        accentSoft: Color,
        accentDeep: Color,
        surface: Color,
        surface2: Color,
        ink: Color,
        inkSoft: Color,
        inkMute: Color,
        success: Color,
        warn: Color,
        coin: Color,
        border: Color
    ) {
        self.pageBg = pageBg
        self.accent = accent
        self.accentSoft = accentSoft
        self.accentDeep = accentDeep
        self.surface = surface
        self.surface2 = surface2
        self.ink = ink
        self.inkSoft = inkSoft
        self.inkMute = inkMute
        self.success = success
        self.warn = warn
        self.coin = coin
        self.border = border
    }
}

// MARK: - 静态实例

extension ThemeColors {
    /// candy（糖果粉）—— 完整实装；hex 值 1:1 对齐 ui_design/README §Design Tokens.
    public static let candy = ThemeColors(
        pageBg:     Color(hex: 0xF7E9E0),
        accent:     Color(hex: 0xFF8FA3),
        accentSoft: Color(hex: 0xFFD6DF),
        accentDeep: Color(hex: 0xE15F7C),
        surface:    Color(hex: 0xFFF9F5),
        surface2:   Color(hex: 0xFFF1E8),
        ink:        Color(hex: 0x4A2C36),
        inkSoft:    Color(hex: 0x8B6B75),
        inkMute:    Color(hex: 0xB99BA5),
        success:    Color(hex: 0x7BC47F),
        warn:       Color(hex: 0xFFB26B),
        coin:       Color(hex: 0xFFB84D),
        // border: rgba(74,44,54,0.08) → Color(red:0x4A/255, green:0x2C/255, blue:0x36/255).opacity(0.08)
        border:     Color(red: 74.0/255, green: 44.0/255, blue: 54.0/255).opacity(0.08)
    )

    /// matcha（抹茶）—— stub: accent / accentSoft / accentDeep 来自 ui_design;
    /// 其余字段以 candy 同字段 placeholder + TODO 注释.
    public static let matcha = ThemeColors(
        pageBg:     ThemeColors.candy.pageBg,         // TODO(Story-Future): matcha pageBg 待定
        accent:     Color(hex: 0x94B97C),             // ui_design 钦定
        accentSoft: Color(hex: 0xDFE8C8),             // ui_design 钦定
        accentDeep: Color(hex: 0x63894A),             // ui_design 钦定
        surface:    ThemeColors.candy.surface,        // TODO(Story-Future): matcha surface 待定
        surface2:   ThemeColors.candy.surface2,       // TODO(Story-Future): matcha surface2 待定
        ink:        ThemeColors.candy.ink,            // TODO(Story-Future): matcha ink 待定
        inkSoft:    ThemeColors.candy.inkSoft,        // TODO(Story-Future): matcha inkSoft 待定
        inkMute:    ThemeColors.candy.inkMute,        // TODO(Story-Future): matcha inkMute 待定
        success:    ThemeColors.candy.success,        // TODO(Story-Future): matcha success 待定
        warn:       ThemeColors.candy.warn,           // TODO(Story-Future): matcha warn 待定
        coin:       ThemeColors.candy.coin,           // TODO(Story-Future): matcha coin 待定
        border:     ThemeColors.candy.border          // TODO(Story-Future): matcha border 待定
    )

    /// sky（天空）—— stub.
    public static let sky = ThemeColors(
        pageBg:     ThemeColors.candy.pageBg,         // TODO(Story-Future): sky pageBg 待定
        accent:     Color(hex: 0x7BB3E0),             // ui_design 钦定
        accentSoft: Color(hex: 0xCFE2F2),             // ui_design 钦定
        accentDeep: Color(hex: 0x4E86B6),             // ui_design 钦定
        surface:    ThemeColors.candy.surface,        // TODO(Story-Future): sky surface 待定
        surface2:   ThemeColors.candy.surface2,       // TODO(Story-Future): sky surface2 待定
        ink:        ThemeColors.candy.ink,            // TODO(Story-Future): sky ink 待定
        inkSoft:    ThemeColors.candy.inkSoft,        // TODO(Story-Future): sky inkSoft 待定
        inkMute:    ThemeColors.candy.inkMute,        // TODO(Story-Future): sky inkMute 待定
        success:    ThemeColors.candy.success,        // TODO(Story-Future): sky success 待定
        warn:       ThemeColors.candy.warn,           // TODO(Story-Future): sky warn 待定
        coin:       ThemeColors.candy.coin,           // TODO(Story-Future): sky coin 待定
        border:     ThemeColors.candy.border          // TODO(Story-Future): sky border 待定
    )

    /// dark（深色模式）—— stub: pageBg / surface / ink 来自 ui_design 显式表述;
    /// 其余字段以 candy 同字段 placeholder + TODO 注释.
    public static let dark = ThemeColors(
        pageBg:     Color(hex: 0x2A1C22),             // ui_design 钦定
        accent:     ThemeColors.candy.accent,         // TODO(Story-Future): dark accent 待定
        accentSoft: ThemeColors.candy.accentSoft,     // TODO(Story-Future): dark accentSoft 待定
        accentDeep: ThemeColors.candy.accentDeep,     // TODO(Story-Future): dark accentDeep 待定
        surface:    Color(hex: 0x3A2831),             // ui_design 钦定
        surface2:   ThemeColors.candy.surface2,       // TODO(Story-Future): dark surface2 待定
        ink:        Color(hex: 0xFBE5EC),             // ui_design 钦定
        inkSoft:    ThemeColors.candy.inkSoft,        // TODO(Story-Future): dark inkSoft 待定
        inkMute:    ThemeColors.candy.inkMute,        // TODO(Story-Future): dark inkMute 待定
        success:    ThemeColors.candy.success,        // TODO(Story-Future): dark success 待定
        warn:       ThemeColors.candy.warn,           // TODO(Story-Future): dark warn 待定
        coin:       ThemeColors.candy.coin,           // TODO(Story-Future): dark coin 待定
        border:     ThemeColors.candy.border          // TODO(Story-Future): dark border 待定
    )
}

// MARK: - Color hex 辅助 init

extension Color {
    /// 从 24-bit RGB hex literal 构造 Color（如 0xFF8FA3）.
    /// alpha 默认 1.0；如需 alpha 通过 `.opacity(_:)` 修饰.
    /// 仅供 ThemeColors 内部使用；外部代码应取 token 而非自己 hex.
    init(hex: UInt32, alpha: Double = 1.0) {
        let r = Double((hex >> 16) & 0xFF) / 255.0
        let g = Double((hex >> 8) & 0xFF) / 255.0
        let b = Double(hex & 0xFF) / 255.0
        self.init(.sRGB, red: r, green: g, blue: b, opacity: alpha)
    }
}
```

**关键约束**：
- `Color(hex:)` 辅助 init 是 **internal** scope（Swift 默认 internal），只让 ThemeColors 内部用；如外部代码（Feature View）想读色值必须经 `theme.colors.accent` 路径——禁止 `Color(hex: 0xFF8FA3)` 在 Features/ 内出现（grep 校验靠 Story 37.13 静态校验脚本兜底）
- Equatable 默认合成（结构体所有字段都 Equatable）—— Color 在 SwiftUI 是 Equatable
- 13 字段命名严格 camelCase（pageBg / accentSoft / accentDeep / inkSoft / inkMute / surface2）；**不**用 snake_case 或下划线
- `ThemeColors.candy.X` 在 stub 里被引用：dev 必须确保 ThemeColors.candy 先于 ThemeColors.matcha / .sky / .dark 在文件中出现（`static let` 初始化顺序由编译器决定，但 Swift `static let` 是 lazy 一次初始化，相互引用安全；**但** 若 dev 用 `extension` 多个文件分散写则需注意编译顺序）

### AC3 — 新建 ThemeSpacing.swift（9 档间距 token）

**新建文件 `iphone/PetApp/Core/DesignSystem/ThemeSpacing.swift`**：

```swift
import SwiftUI

/// ThemeSpacing: 间距 token. 9 档值对齐 ui_design/README §Spacing.
///
/// 命名: 用 t-shirt size + 数值后缀避免歧义（s / m / l 等命名在 9 档下不够清晰）.
/// 字段值即 SwiftUI CGFloat point 值.
public struct ThemeSpacing: Equatable {
    public let s8: CGFloat    // 8
    public let s10: CGFloat   // 10
    public let s12: CGFloat   // 12
    public let s14: CGFloat   // 14
    public let s16: CGFloat   // 16
    public let s18: CGFloat   // 18
    public let s20: CGFloat   // 20
    public let s22: CGFloat   // 22
    public let s28: CGFloat   // 28

    public init(
        s8: CGFloat = 8,
        s10: CGFloat = 10,
        s12: CGFloat = 12,
        s14: CGFloat = 14,
        s16: CGFloat = 16,
        s18: CGFloat = 18,
        s20: CGFloat = 20,
        s22: CGFloat = 22,
        s28: CGFloat = 28
    ) {
        self.s8 = s8
        self.s10 = s10
        self.s12 = s12
        self.s14 = s14
        self.s16 = s16
        self.s18 = s18
        self.s20 = s20
        self.s22 = s22
        self.s28 = s28
    }
}

extension ThemeSpacing {
    /// standard: 9 档 8/10/12/14/16/18/20/22/28（ui_design 钦定）.
    /// 全部主题共用一份 spacing scale—— 主题切换不改间距（与 ui_design 设计一致）.
    public static let standard = ThemeSpacing()
}
```

**关键约束**：
- 9 档全部覆盖 ui_design `Spacing` 段值；命名 `s<value>` 让 caller 写 `theme.spacing.s16` 一目了然
- 全部主题共用 `ThemeSpacing.standard`（ui_design 不区分主题间距）；保留 init 接受参数让未来某个新主题定制间距时能扩展，不破坏类型契约

### AC4 — 新建 ThemeRadius.swift（圆角 token，5 类语义命名）

**新建文件 `iphone/PetApp/Core/DesignSystem/ThemeRadius.swift`**：

```swift
import SwiftUI

/// ThemeRadius: 圆角 token. 按 ui_design/README §Border Radius 的 5 类语义命名.
///
/// **命名映射**:
///   - tag (小标签):   6 / 8                 → tagSm = 6, tagMd = 8
///   - control (中等元素):  12 / 14 / 16     → controlSm = 12, controlMd = 14, controlLg = 16
///   - card (卡片): 18 / 20 / 22 / 24       → cardSm = 18, cardMd = 20, cardLg = 22, cardXl = 24
///   - modal (大卡片 / Modal): 26 / 28        → modalSm = 26, modalLg = 28
///   - pill (按钮 / 圆药丸): 高度的一半     → pill = 999（"足够大让 SwiftUI 取最小半径，等价圆药丸"）
///
/// **不含**: 头像 / 圆点 50% —— 那靠 .clipShape(Circle()) 实现，无圆角值.
public struct ThemeRadius: Equatable {
    public let tagSm: CGFloat
    public let tagMd: CGFloat
    public let controlSm: CGFloat
    public let controlMd: CGFloat
    public let controlLg: CGFloat
    public let cardSm: CGFloat
    public let cardMd: CGFloat
    public let cardLg: CGFloat
    public let cardXl: CGFloat
    public let modalSm: CGFloat
    public let modalLg: CGFloat
    public let pill: CGFloat

    public init(
        tagSm: CGFloat = 6,
        tagMd: CGFloat = 8,
        controlSm: CGFloat = 12,
        controlMd: CGFloat = 14,
        controlLg: CGFloat = 16,
        cardSm: CGFloat = 18,
        cardMd: CGFloat = 20,
        cardLg: CGFloat = 22,
        cardXl: CGFloat = 24,
        modalSm: CGFloat = 26,
        modalLg: CGFloat = 28,
        pill: CGFloat = 999
    ) {
        self.tagSm = tagSm
        self.tagMd = tagMd
        self.controlSm = controlSm
        self.controlMd = controlMd
        self.controlLg = controlLg
        self.cardSm = cardSm
        self.cardMd = cardMd
        self.cardLg = cardLg
        self.cardXl = cardXl
        self.modalSm = modalSm
        self.modalLg = modalLg
        self.pill = pill
    }
}

extension ThemeRadius {
    /// standard: 全部主题共用 radius scale.
    public static let standard = ThemeRadius()
}
```

**关键约束**：
- `pill = 999`：caller 用 `.cornerRadius(theme.radius.pill)` 实现圆药丸（SwiftUI 自动 clamp 到 height/2）；**不**直接写 `.frame(height: H).cornerRadius(H/2)`（避免 caller 重复算 H/2）
- 11 + 1 = 12 个 token 完整覆盖 ui_design 5 类 + 边界场景（pill）

### AC5 — 新建 ThemeShadow.swift（3 档阴影 + 按钮硬阴影）

**新建文件 `iphone/PetApp/Core/DesignSystem/ThemeShadow.swift`**：

```swift
import SwiftUI

/// ThemeShadow: 阴影 token.
///
/// **3 档语义阴影** (ui_design/README §Shadows):
///   - sm: 0 2px 0 rgba(180,100,120,0.08)  → 卡片
///   - md: 0 6px 16px rgba(180,100,120,0.14) → Tab Bar / 主要卡片
///   - lg: 0 14px 38px rgba(180,100,120,0.18) → Modal
///
/// **按钮硬阴影** (ui_design "立体感硬阴影 0 4px 0 var(--accent-deep)"):
///   - 不在本 struct 内—— 按钮硬阴影与 accent-deep color 强绑定，应在 PrimaryButton (Story 37.6)
///     内部用 `theme.colors.accentDeep` + offset 4 直接组合，不抽象为独立 token.
///
/// **类型选择**: ShadowToken struct (color + radius + x + y) 取代 SwiftUI Color
///   →  让 Card 通过 `.shadow(color: t.color, radius: t.radius, x: t.x, y: t.y)` 一次取齐.
public struct ShadowToken: Equatable {
    public let color: Color
    public let radius: CGFloat
    public let x: CGFloat
    public let y: CGFloat

    public init(color: Color, radius: CGFloat, x: CGFloat = 0, y: CGFloat = 0) {
        self.color = color
        self.radius = radius
        self.x = x
        self.y = y
    }
}

public struct ThemeShadow: Equatable {
    public let sm: ShadowToken
    public let md: ShadowToken
    public let lg: ShadowToken

    public init(sm: ShadowToken, md: ShadowToken, lg: ShadowToken) {
        self.sm = sm
        self.md = md
        self.lg = lg
    }
}

extension ThemeShadow {
    /// candy: shadow 色基 rgba(180,100,120,X).
    public static let candy = ThemeShadow(
        sm: ShadowToken(
            // 0 2px 0 rgba(180,100,120,0.08) → SwiftUI shadow radius=0 → 硬边阴影
            color: Color(red: 180.0/255, green: 100.0/255, blue: 120.0/255).opacity(0.08),
            radius: 0,
            x: 0,
            y: 2
        ),
        md: ShadowToken(
            // 0 6px 16px rgba(180,100,120,0.14)
            color: Color(red: 180.0/255, green: 100.0/255, blue: 120.0/255).opacity(0.14),
            radius: 16,
            x: 0,
            y: 6
        ),
        lg: ShadowToken(
            // 0 14px 38px rgba(180,100,120,0.18)
            color: Color(red: 180.0/255, green: 100.0/255, blue: 120.0/255).opacity(0.18),
            radius: 38,
            x: 0,
            y: 14
        )
    )
}
```

**关键约束**：
- `ShadowToken` 是 helper struct——让 caller 一次性 `.shadow(color:radius:x:y:)` 取齐 4 个参数，不重复算
- 本期仅 candy ThemeShadow 完整实装；matcha / sky / dark 通过 `Theme.candy.shadow` 复用（Theme.matcha = Theme(..., shadow: .candy, ...)）
- CSS `0 2px 0` 中第三个值为 0（blur radius=0）→ SwiftUI `.shadow(radius: 0)` 等价；这是"硬边阴影"特性，不是 bug

### AC6 — 新建 ThemeTypography.swift（6 档字号语义命名）

**新建文件 `iphone/PetApp/Core/DesignSystem/ThemeTypography.swift`**：

```swift
import SwiftUI

/// ThemeTypography: 字号 / 字重 token. 按 ui_design/README §Typography 6 档语义命名.
///
/// **6 档命名映射**:
///   - largeTitle:    22 / 800        → 大标题
///   - mediumTitle:   17-18 / 800     → 中标题（取 17 作为 default）
///   - cardTitle:     14-15 / 800     → 卡片标题（取 14）
///   - body:          13 / 600-700    → 正文（取 13 / 700）
///   - caption:       11-12 / 700     → 辅助文字（取 11 / 700）
///   - microLabel:    9-10 / 800      → 微小标签（取 10）
///
/// **字重映射**:
///   ui_design 的 800 ≈ SwiftUI .heavy (Font.Weight.heavy = ~800)
///   ui_design 的 700 ≈ SwiftUI .bold  (Font.Weight.bold ≈ 700)
///   ui_design 的 600 ≈ SwiftUI .semibold (Font.Weight.semibold ≈ 600)
///
/// **字体家族**:
///   ui_design 钦定 SF Pro Rounded + PingFang SC; SwiftUI 通过
///   `.font(.system(size: t.size, weight: t.weight, design: .rounded))` 取
///   SF Pro Rounded; PingFang SC 由 iOS fallback 链自动接管中文字符.
public struct TypographyToken: Equatable {
    public let size: CGFloat
    public let weight: Font.Weight

    public init(size: CGFloat, weight: Font.Weight) {
        self.size = size
        self.weight = weight
    }

    /// 转 SwiftUI Font (rounded design).
    public var font: Font {
        .system(size: size, weight: weight, design: .rounded)
    }
}

public struct ThemeTypography: Equatable {
    public let largeTitle: TypographyToken
    public let mediumTitle: TypographyToken
    public let cardTitle: TypographyToken
    public let body: TypographyToken
    public let caption: TypographyToken
    public let microLabel: TypographyToken

    public init(
        largeTitle: TypographyToken = TypographyToken(size: 22, weight: .heavy),
        mediumTitle: TypographyToken = TypographyToken(size: 17, weight: .heavy),
        cardTitle: TypographyToken = TypographyToken(size: 14, weight: .heavy),
        body: TypographyToken = TypographyToken(size: 13, weight: .bold),
        caption: TypographyToken = TypographyToken(size: 11, weight: .bold),
        microLabel: TypographyToken = TypographyToken(size: 10, weight: .heavy)
    ) {
        self.largeTitle = largeTitle
        self.mediumTitle = mediumTitle
        self.cardTitle = cardTitle
        self.body = body
        self.caption = caption
        self.microLabel = microLabel
    }
}

extension ThemeTypography {
    /// standard: 全部主题共用 typography scale.
    public static let standard = ThemeTypography()
}
```

**关键约束**：
- `TypographyToken.font` computed property 让 caller 直接 `.font(theme.typography.body.font)` 一行写完
- design: .rounded 钦定取 SF Pro Rounded（iOS 17+ 系统字体）；中文字符靠系统 fallback 链自动用 PingFang SC——**不**显式 .custom("PingFang SC", size:) 避免英文字符也被强切到 PingFang
- ui_design 字号区间（如 17-18 / 800）取低端值作为 default（17 / 14 / 13 / 11 / 10）—— caller 如需 18 可临时 override，保持 default 紧凑

### AC7 — 新建 Environment+Theme.swift（EnvironmentKey 注入入口）

**新建文件 `iphone/PetApp/Core/DesignSystem/Environment+Theme.swift`**：

```swift
import SwiftUI

/// EnvironmentKey for Theme: 让子视图通过 `@Environment(\.theme) var theme` 取主题.
///
/// **default value**: `.candy`（与 RootView `@State currentTheme = .candy` 默认一致）.
/// 这意味着即使父视图忘了写 `.environment(\.theme, ...)` modifier，子视图取 theme 也不 crash,
/// 而是取 candy 默认值——与 SwiftUI 其它 EnvironmentKey 默认值习惯一致.
///
/// **注入路径**:
///   RootView 内 `MainTabView().environment(\.theme, currentTheme.theme)` 把 currentTheme 对应的
///   Theme 实例（`.candy` / `.matcha` / `.sky` / `.dark`）写入子树 environment.
///
/// **取值路径**:
///   Feature View 写 `@Environment(\.theme) var theme`; Sample 调用如
///   `RoundedRectangle(cornerRadius: theme.radius.cardLg).fill(theme.colors.surface)`.
private struct ThemeEnvironmentKey: EnvironmentKey {
    static let defaultValue: Theme = .candy
}

extension EnvironmentValues {
    /// `@Environment(\.theme) var theme` 的取值入口.
    public var theme: Theme {
        get { self[ThemeEnvironmentKey.self] }
        set { self[ThemeEnvironmentKey.self] = newValue }
    }
}
```

**关键约束**：
- `defaultValue: Theme = .candy`（**不是** `Theme.candy` 引用尚未定义的类型）—— Swift 编译器检查 default value 必须是 const 表达式，`Theme.candy` 静态属性合法
- `EnvironmentValues.theme` 是 `public`（让任何 Feature View 都能 `@Environment(\.theme)`）
- 文件名 `Environment+Theme.swift` 走 Swift 社区惯例 `<TypeName>+<Feature>.swift`（这里给 EnvironmentValues 加 theme 字段）

### AC8 — 修改 RootView.swift：注入 Theme 到子树

**修改 `iphone/PetApp/App/RootView.swift`**：

- **新增** `@State private var currentTheme: ThemeName = .candy` 字段（与 `coordinator` / `appState` 同级；**用 `@State` 而非 `@StateObject`** 因为 Theme 是 value type、ThemeName 是 enum）
- **修改** `LaunchedContentView` 的 `.ready` 分支：在 `.environmentObject(coordinator)` / `.environmentObject(homeViewModel)` / `.environmentObject(appState)` 之后**追加** `.environment(\.theme, currentTheme.theme)` modifier
- **关键决策**：本期 currentTheme 始终是 `.candy`（无 UI 切换面板，按 Story 37.14 白名单）；保留 `@State` 而非 `let` 是为了**未来**主题切换 UI 落地时能直接 `@Binding currentTheme: ThemeName`，不破坏 RootView 类型契约

**示例修改片段**（dev 实装时按 git diff 风格落地）：

```swift
struct RootView: View {
    @StateObject private var coordinator = AppCoordinator()
    @StateObject private var container = AppContainer()
    @StateObject private var homeViewModel = HomeViewModel()
    @StateObject private var appState = AppState()

    /// Story 37.5 AC8: Theme 注入 source-of-truth.
    /// 当前固定 .candy（Story 37.14 白名单：本期不做 UI 切换面板）.
    /// 用 @State 而非 let 是为了未来主题切换 UI（mini-epic）落地时能直接改 @Binding 不破坏 RootView 类型契约.
    @State private var currentTheme: ThemeName = .candy

    // ... 其余字段保留不变 ...

    var body: some View {
        ZStack {
            if let stateMachine = launchStateMachine {
                LaunchedContentView(
                    stateMachine: stateMachine,
                    coordinator: coordinator,
                    homeViewModel: homeViewModel,
                    appState: appState,
                    currentTheme: currentTheme,    // 新增传参
                    sessionStore: container.sessionStore,
                    // ... 其余参数保留 ...
                )
            }
            // ...
        }
    }
}

private struct LaunchedContentView: View {
    @ObservedObject var stateMachine: AppLaunchStateMachine
    @ObservedObject var coordinator: AppCoordinator
    let homeViewModel: HomeViewModel
    let appState: AppState
    let currentTheme: ThemeName  // 新增字段
    // ... 其余字段保留 ...

    var body: some View {
        ZStack {
            switch stateMachine.state {
            case .ready:
                MainTabView()
                    .environmentObject(coordinator)
                    .environmentObject(homeViewModel)
                    .environmentObject(appState)
                    .environment(\.theme, currentTheme.theme)   // 新增 modifier
                    // ... 其余 modifier 保留 ...
                    .transition(.opacity)
            // ... 其余 case 保留 ...
            }
        }
    }
}
```

**关键约束**：
- 注入入口是 `MainTabView()`（即 `.ready` 分支根视图）；**不**注入到外层 ZStack（外层含 LaunchingView / KeychainUITestHookView，那些不需要 theme）
- LaunchingView / RetryView / TerminalErrorView 等 `.launching` / `.needsAuth` 分支视图**不**通过本 modifier 拿 theme—— 它们靠 EnvironmentKey 默认值 `.candy` 自动 fallback；如未来这些视图也要用 theme，dev 在专门 spike 决定是否把 modifier 提到 ZStack 外层
- **不**改 `ResetIdentityViewModel` / `KeychainUITestHookView` / 三态机；本 story 仅触碰 RootView 与 LaunchedContentView 两处

### AC9 — 单元测试覆盖（≥3 case）

**新建文件 `iphone/PetAppTests/Core/DesignSystem/ThemeTests.swift`**（**不**改任何现有测试文件）：

测试需求（≥3 case，纯 XCTest，不引第三方）：

- **happy: candy 颜色精确**：`XCTAssertEqual(Theme.candy.colors.accent.uiColorHex, 0xFF8FA3)` —— 验证 candy.accent 的 hex 是 ui_design 钦定值
- **happy: dark 颜色显式表述字段精确**：`XCTAssertEqual(Theme.dark.colors.pageBg.uiColorHex, 0x2A1C22)` —— 验证 dark.pageBg
- **edge: EnvironmentKey 默认值是 candy**：构造一个没注入 .environment(\.theme) 的视图，`@Environment(\.theme)` 取出来等于 `.candy`
- **happy: ThemeName.theme 路由正确**：`XCTAssertEqual(ThemeName.candy.theme, Theme.candy)` + `XCTAssertEqual(ThemeName.dark.theme, Theme.dark)`（依赖 Theme: Equatable）
- **happy: 9 档 spacing 全字段值正确**：`XCTAssertEqual(Theme.candy.spacing.s8, 8)` + `XCTAssertEqual(Theme.candy.spacing.s28, 28)` 等

**Color hex 提取 helper**（测试用）—— **新增**到 `iphone/PetAppTests/Core/DesignSystem/ThemeTests.swift` 文件内**测试 fixture 段**（**不**加到 production code）：

```swift
/// 测试用 helper: 从 SwiftUI Color 提取 24-bit RGB hex (忽略 alpha).
/// **仅供测试断言**——production code 取色应走 `theme.colors.X`.
extension Color {
    var uiColorHex: UInt32 {
        let uiColor = UIColor(self)
        var r: CGFloat = 0, g: CGFloat = 0, b: CGFloat = 0, a: CGFloat = 0
        uiColor.getRed(&r, green: &g, blue: &b, alpha: &a)
        let ri = UInt32((r * 255).rounded())
        let gi = UInt32((g * 255).rounded())
        let bi = UInt32((b * 255).rounded())
        return (ri << 16) | (gi << 8) | bi
    }
}
```

**EnvironmentKey 默认值测试**：用 SwiftUI hosting view 间接触发或直接读 `EnvironmentValues()` 默认实例：

```swift
@MainActor
func testEnvironmentDefaultIsCandy() {
    let env = EnvironmentValues()
    XCTAssertEqual(env.theme, Theme.candy)
}
```

**关键约束**：
- 测试文件路径：`iphone/PetAppTests/Core/DesignSystem/ThemeTests.swift`（新目录 `Core/DesignSystem/` 在 PetAppTests/ 下，与 production `iphone/PetApp/Core/DesignSystem/` 镜像）
- **必须** `@testable import PetApp` 让测试访问 `internal` `Color(hex:)` init（如该 init 是 internal）；如 Color(hex:) 是 public 则普通 `import` 即可
- UIColor 转换 helper 路径 `Color → UIColor → CGFloat (r,g,b,a) → UInt32`；rounded() 规避 floating-point drift（如 0xFF / 255 = 1.0 但 0xCC / 255 = 0.7999... ≠ 0.8 严格相等）
- 全部测试 case **不**依赖 RootView / SwiftUI hosting；纯 token 类型 + EnvironmentValues 默认值；testWeight ≤ 50ms

### AC10 — 视觉验收（preview 抽样）

**Preview 块**：每个 ThemeColors 静态实例（candy / matcha / sky / dark）**至少抽样**一个 SwiftUI Preview，用一个简单卡片渲染（如 RoundedRectangle 取 colors.surface 作底 + Text 取 colors.ink）让 dev 在 Xcode Canvas 里目视确认色值无误。

**示例 Preview** (放在 `Theme.swift` 文件底部 `#if DEBUG`)：

```swift
#if DEBUG
struct ThemePreview_Sampler: View {
    let theme: Theme
    let label: String
    var body: some View {
        VStack(spacing: 8) {
            Text(label)
                .font(theme.typography.cardTitle.font)
                .foregroundColor(theme.colors.ink)
            HStack(spacing: 8) {
                colorSwatch(theme.colors.accent, "accent")
                colorSwatch(theme.colors.accentSoft, "accentSoft")
                colorSwatch(theme.colors.accentDeep, "accentDeep")
            }
            Text("Surface")
                .font(theme.typography.body.font)
                .foregroundColor(theme.colors.ink)
                .padding(theme.spacing.s14)
                .background(theme.colors.surface)
                .cornerRadius(theme.radius.cardMd)
        }
        .padding(theme.spacing.s16)
        .background(theme.colors.pageBg)
    }
    private func colorSwatch(_ c: Color, _ name: String) -> some View {
        VStack {
            RoundedRectangle(cornerRadius: theme.radius.tagMd).fill(c).frame(width: 48, height: 48)
            Text(name).font(theme.typography.caption.font)
        }
    }
}

#Preview("Theme Sampler — candy") { ThemePreview_Sampler(theme: .candy, label: "candy") }
#Preview("Theme Sampler — matcha") { ThemePreview_Sampler(theme: .matcha, label: "matcha (stub)") }
#Preview("Theme Sampler — sky") { ThemePreview_Sampler(theme: .sky, label: "sky (stub)") }
#Preview("Theme Sampler — dark") { ThemePreview_Sampler(theme: .dark, label: "dark (stub)") }
#endif
```

**关键约束**：
- Preview 仅供 dev 在 Canvas 目视确认；**不**作为单元测试断言（视觉差异容忍）
- Preview block 在 `Theme.swift` 文件底部，与 production Theme 静态实例同文件（避免新建 PreviewSupport 文件）
- 4 个 #Preview 块覆盖 4 个 ThemeName，让 dev 一眼看到 candy 完整、stub 三主题部分字段确实复用 candy

### AC11 — Deliverable 清单

- 新建：
  - `iphone/PetApp/Core/DesignSystem/Theme.swift`
  - `iphone/PetApp/Core/DesignSystem/ThemeColors.swift`（含 `Color(hex:)` 辅助 init）
  - `iphone/PetApp/Core/DesignSystem/ThemeSpacing.swift`
  - `iphone/PetApp/Core/DesignSystem/ThemeRadius.swift`
  - `iphone/PetApp/Core/DesignSystem/ThemeShadow.swift`（含 `ShadowToken`）
  - `iphone/PetApp/Core/DesignSystem/ThemeTypography.swift`（含 `TypographyToken`）
  - `iphone/PetApp/Core/DesignSystem/Environment+Theme.swift`
  - `iphone/PetAppTests/Core/DesignSystem/ThemeTests.swift`（≥3 测试 case + Color uiColorHex helper）
- 修改：
  - `iphone/PetApp/App/RootView.swift`（新增 `@State currentTheme` + LaunchedContentView 透传 + `.environment(\.theme, ...)` modifier）
- **不动**：
  - `iphone/PetApp/App/MainTabView.swift`（保留 Story 37.3 硬编码 Color/shadow；本 story 不收口该占位）
  - `iphone/PetApp/Features/Home/Views/HomeView.swift` / `HomeContainerView.swift`（保留 Story 2.5 hardcoded color；Story 37.7 HomeView Scaffold 时统一收口）
  - `ios/` 任何文件（CLAUDE.md 强约束）
  - `server/` 任何文件
- **xcodegen regen**：本 story 新建 8 个文件，全部在 `iphone/PetApp/Core/DesignSystem/` + `iphone/PetAppTests/Core/DesignSystem/` 子目录下；按 `iphone/project.yml` `sources: - PetApp` / `- PetAppTests` 通配规则**不需要**手动改 project.yml；但**必须** `cd iphone && xcodegen generate` 重新生成 `iphone/PetApp.xcodeproj` 让 8 个新文件加入 build target（dev 实装时记得跑此命令；Story 2.7 测试基础设施 / Story 37.3 / 37.4 落地都依赖此步骤）
- **commit 粒度**：建议单 commit；如 dev 拆分 commit 必须保证每个 commit 独立可 build（如 Theme.swift 与其它 sub-token 文件因 `.candy` 静态实例引用关系**必须同 commit**）

## Tasks / Subtasks

- [x] Task 1: 新建 6 个 Theme token 容器结构 (AC1-6)
  - [x] 1.1 新建 `iphone/PetApp/Core/DesignSystem/Theme.swift`（顶层 `Theme` struct + `ThemeName` enum + 4 个 `static let` 实例）
  - [x] 1.2 新建 `iphone/PetApp/Core/DesignSystem/ThemeColors.swift`（13 字段 + 4 主题 static let + `Color(hex:)` 辅助 init；matcha / sky / dark stub 字段必须有 `// TODO(Story-Future):` 注释）
  - [x] 1.3 新建 `iphone/PetApp/Core/DesignSystem/ThemeSpacing.swift`（9 档 + standard）
  - [x] 1.4 新建 `iphone/PetApp/Core/DesignSystem/ThemeRadius.swift`（12 字段 + standard）
  - [x] 1.5 新建 `iphone/PetApp/Core/DesignSystem/ThemeShadow.swift`（`ShadowToken` + 3 档 + candy）
  - [x] 1.6 新建 `iphone/PetApp/Core/DesignSystem/ThemeTypography.swift`（`TypographyToken` + 6 档 + standard）
- [x] Task 2: 新建 EnvironmentKey 注入入口 (AC7)
  - [x] 2.1 新建 `iphone/PetApp/Core/DesignSystem/Environment+Theme.swift`（`ThemeEnvironmentKey` + `EnvironmentValues.theme`）
- [x] Task 3: 修改 RootView 注入 Theme (AC8)
  - [x] 3.1 在 `RootView` 加 `@State private var currentTheme: ThemeName = .candy`
  - [x] 3.2 在 `LaunchedContentView` init / 字段加 `currentTheme: ThemeName` 透传
  - [x] 3.3 在 `.ready` 分支 MainTabView 链上加 `.environment(\.theme, currentTheme.theme)` modifier（紧跟现有 `.environmentObject(appState)` 之后；与其它 environment modifier 同样位置）
- [x] Task 4: 单元测试 (AC9)
  - [x] 4.1 新建 `iphone/PetAppTests/Core/DesignSystem/ThemeTests.swift`，落地 6 case（candy.accent hex / dark.pageBg hex / EnvironmentKey 默认 / ThemeName.theme 路由 / spacing 抽样 / radius 抽样）
  - [x] 4.2 在该文件内加 `Color.uiColorHex` 测试 helper（仅供测试 fixture，不进 production）
- [x] Task 5: 视觉 Preview (AC10)
  - [x] 5.1 在 `Theme.swift` 底部加 `#if DEBUG ... ThemePreview_Sampler` + 4 个 `#Preview` 块覆盖 4 主题
- [x] Task 6: xcodegen regen + build verify (AC11)
  - [x] 6.1 `cd iphone && xcodegen generate` 让 8 个新文件加入 PetApp / PetAppTests target
  - [x] 6.2 `bash iphone/scripts/build.sh --test` 跑测试通过（251/251 tests passed）
  - [x] 6.3 grep 验证 `TODO(Story-Future)` 出现次数 = 34（Theme.swift 3 + ThemeColors.swift 31）；stub 注释覆盖完整

## Dev Notes

### Theme value type 选择 vs ObservableObject 选择（关键设计决策）

**选定**: `struct Theme` (value type), 切换主题靠 RootView `@State currentTheme` 改值 + SwiftUI 重渲染子树。

**为何不走 `class Theme: ObservableObject` + `@StateObject` 路径**：

1. **不变性**：Theme 是一组**纯静态 token**——一旦 ThemeName 选定，Theme 实例永不再变。@Published / objectWillChange 这些"变化通知"机制在此场景没必要；value type + 重渲染语义更直接。
2. **零开销**：value type 在 SwiftUI Environment 内传播是浅拷贝；class 走引用 + 订阅更耗 SwiftUI runtime。
3. **测试友好**：value type 直接 `XCTAssertEqual(Theme.candy, theme)`；class 要写 `XCTAssertTrue(Theme.candy === theme)` 或自定义 == 比较，更繁琐。
4. **与 ADR-0010 边界一致**：AppState 是 ObservableObject 因为含 mutable `@Published` domain state；Theme 是 immutable token，不属同一边界。

**为何 RootView 用 `@State` 而非 `let`**：让未来 mini-epic（主题切换 UI）能直接换成 `@Binding currentTheme`，不破坏 RootView 类型契约（`let` 转 `@State` 编译期 breaking）。本期 currentTheme 始终 `.candy`，但接缝先留好。

### Color(hex:) 辅助 init 的 scope（internal 而非 public）

`Color(hex:)` 设计为**仅 ThemeColors 内部使用**——外部 Feature View 取色应走 `theme.colors.accent` 路径（让 token 成为唯一色值入口）。Swift 默认 internal，已满足；**不**显式标 `public` 以防 Feature View 写 `Color(hex: 0xFF8FA3)` 绕过 theme。

后续 Story 37.13 静态校验脚本 `check_no_apiclient_in_features.sh` 可考虑扩展加一条 `grep -E 'Color\(hex:'`（不在本 story scope；登记到 Story 37.13 backlog）。

### TODO(Story-Future) 标记 + grep 校验

stub 字段（matcha / sky / dark 复用 candy 占位）**全部**带 `// TODO(Story-Future): <主题名> <字段> 设计待定，当前复用 candy` 注释——格式严格统一让后续 mini-epic dev 一次性 `grep -rn 'TODO(Story-Future)' iphone/PetApp/Core/DesignSystem/` 列出全部 stub。

总数大致估算：
- matcha: 9 字段 stub（accent / accentSoft / accentDeep 已显式定值 → 13 - 3 = 10；但 ThemeShadow 整个走 candy 引用 → 加 1 整组）
- sky: 同 matcha = 10
- dark: 9 字段 stub（pageBg / surface / ink 已显式定值 → 13 - 3 = 10；shadow 整组复用 → 加 1）
- ThemeShadow.matcha / .sky / .dark 复用注释：3 条
- 估算总数 30+ 条

dev 实装后 Task 6.3 跑 grep 抽样确认即可，**不**要求精确数字断言（数字会随未来 stub 收口变化）。

### EnvironmentKey 默认值的 fallback 行为

`ThemeEnvironmentKey.defaultValue: Theme = .candy` 让任何忘了写 `.environment(\.theme, ...)` 的视图自动 fallback 到 candy——这是 SwiftUI 标准模式。

**风险**：dev 在 LaunchingView / RetryView / TerminalErrorView 引用 theme 时，因外层 RootView 的 modifier 仅作用于 `.ready` 子树，这些 `.launching` / `.needsAuth` 视图会拿 candy 默认值——**当前阶段无害**（candy 是默认主题）；如未来引入主题切换 UI、user 已切到 dark，这些视图仍展示 candy 风格——产品决策（是否要让 launching / needsAuth 也跟随主题）留给后续 mini-epic 处理。本 story **不**因此把 modifier 提到外层 ZStack（提前优化）。

### 与 Story 37.6（共享 primitives）协调

Story 37.6 的 Card / PrimaryButton / Avatar / FadeIn / RarityTag / Icons 全部依赖 theme.colors / theme.spacing / theme.radius / theme.shadow / theme.typography——本 story 必须**完整**落地 5 个 sub-token struct，不能延后任何字段。

**接缝验证**：本 story 落地后，dev 可在 Xcode Canvas 临时建一个示例 Card：

```swift
RoundedRectangle(cornerRadius: theme.radius.cardLg)
    .fill(theme.colors.surface)
    .frame(height: 200)
    .shadow(
        color: theme.shadow.sm.color,
        radius: theme.shadow.sm.radius,
        x: theme.shadow.sm.x,
        y: theme.shadow.sm.y
    )
```

如该 5 行能编译通过 + 在 Canvas 显示卡片样式，则证明 token 全部就位。

### 与 Story 37.3 MainTabView 占位的协调

Story 37.3 落地的 `MainTabView.swift:68-70` 注释 "Story 37.5 落地后 Color 改用 theme.colors / shadow 改用 theme.shadow"——dev 实装本 story 时**检查并更新**该注释指向 Story 37.7（Sprint Change Proposal v2.4 §661 钦定 Story 37.7 HomeView 落地时同步把 FloatingTabBar 改 theme）；本 story **不**主动改 MainTabView 的硬编码值（避免在 UI Scaffold 未就绪时同步改 TabBar 视觉触发回归）。

**示例注释更新**（dev 实装时落地）：

```swift
// 旧：// Story 37.5 落地后 Color 改用 theme.colors / shadow 改用 theme.shadow
// 新：// Story 37.7 HomeView Scaffold 落地时同步把 Color 改用 theme.colors / shadow 改用 theme.shadow（Story 37.5 已落地 Theme 类型契约，可直接消费）
```

### xcodegen regen 节奏 + project.yml 兼容性

`iphone/project.yml` 内 `targets.PetApp.sources: - PetApp` 是**通配** PetApp/ 子树全部 .swift 文件；新增的 `Core/DesignSystem/{Theme,ThemeColors,ThemeSpacing,ThemeRadius,ThemeShadow,ThemeTypography,Environment+Theme}.swift` 7 个文件全部在 PetApp/ 下 → 自动 inclusion，**不**改 project.yml。

测试目录同理：`PetAppTests` 通配新增 `Core/DesignSystem/ThemeTests.swift`。

dev 必须 `cd iphone && xcodegen generate` 重生成 `PetApp.xcodeproj`（或在 Xcode 内 File → Add Files 手动 import；推荐前者保 project.yml 是 source of truth）。

跑通后 `bash iphone/scripts/build.sh --test` 验证 build + test 通过。

### Source tree components to touch

- 新建：
  - `iphone/PetApp/Core/DesignSystem/Theme.swift`
  - `iphone/PetApp/Core/DesignSystem/ThemeColors.swift`
  - `iphone/PetApp/Core/DesignSystem/ThemeSpacing.swift`
  - `iphone/PetApp/Core/DesignSystem/ThemeRadius.swift`
  - `iphone/PetApp/Core/DesignSystem/ThemeShadow.swift`
  - `iphone/PetApp/Core/DesignSystem/ThemeTypography.swift`
  - `iphone/PetApp/Core/DesignSystem/Environment+Theme.swift`
  - `iphone/PetAppTests/Core/DesignSystem/ThemeTests.swift`
- 修改：
  - `iphone/PetApp/App/RootView.swift`（新增 `@State currentTheme` + 透传 + `.environment(\.theme, ...)` modifier）
- 维护：
  - `iphone/PetApp/App/MainTabView.swift`（仅更新行 68 / 70 注释指向 Story 37.7；不改硬编码值）

### Testing standards summary

- 测试入口：`bash iphone/scripts/build.sh --test`（ADR-0002 §3.4 钦定）
- 测试框架：XCTest only（ADR-0002 §3.1）；**不**引 SnapshotTesting / ViewInspector
- 单元测试位置：`iphone/PetAppTests/Core/DesignSystem/ThemeTests.swift`（与 production 镜像）
- 测试 case 数量：≥3 case（按 v1 epic acceptance）+ 建议加 ThemeName.theme 路由 + spacing 抽样 + EnvironmentKey 默认值共 ≥5 case 更稳
- 测试运行时：每 case ≤ 50ms（纯类型 token + UIColor extraction）
- **不**测 SwiftUI hosting / View body 渲染（Theme 是 token，不需要 View 测试）
- 覆盖目标：candy 完整字段抽样 + stub 主题（matcha / sky / dark）字段不需测（Equatable + static let 由编译器保证存在；TODO 注释由 grep Task 6.3 兜底）

### Project Structure Notes

- Alignment with unified project structure: 完全按 `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §4 + ADR-0002 §3.3 的 `iphone/PetApp/Core/DesignSystem/` 目录约定。Test mirror 路径 `iphone/PetAppTests/Core/DesignSystem/` 与 production 严格镜像（已有 `PetAppTests/App/` `PetAppTests/Features/Home/` `PetAppTests/Helpers/` 同模式）。
- Naming convention: 6 个 token 容器 `Theme<Aspect>.swift`（ThemeColors / ThemeSpacing / ThemeRadius / ThemeShadow / ThemeTypography）+ 顶层 `Theme.swift` + 入口 `Environment+Theme.swift`。命名严格 PascalCase（Swift 社区惯例）。
- Detected conflicts or variances: 无。`iphone/PetApp/Core/DesignSystem/Components/` 已存在（含 AlertOverlayView / ToastView 等 Story 2.6 落地组件）—— 本 story 在 `Components/` **同级**新建 7 个 Theme 文件（不进 Components 子目录），与 ui_design 设计 token vs primitive 边界对齐。

### References

- [Source: iphone/ui_design/README.md#Design Tokens] — 5 类 token 字段定义（colors 13 / spacing 9 / radius 5 类 / shadows 3 + 按钮硬阴影 / typography 6 档 + SF Pro Rounded 钦定）
- [Source: \_bmad-output/planning-artifacts/epics.md#Story 37.5: Theme & Design Tokens（candy 完整 + 三主题 stub）] — 本 story acceptance 原文
- [Source: \_bmad-output/planning-artifacts/sprint-change-proposal-2026-04-29-v2.md#Story 37.5: Theme & Design Tokens] — Sprint Change Proposal v2.1 / v2.4 钦定的 Theme 系统范围 + Story 37.6 协调
- [Source: \_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.1 iOS Mock 框架] — XCTest only 测试框架钦定（不引 SnapshotTesting）
- [Source: \_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md#3.3 iPhone App 工程目录方案] — `iphone/PetApp/` 目录约定 + xcodegen 通配规则
- [Source: \_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md#3.1] — ViewModel 注入规则边界（与 Theme 注入路径对偶：Theme 走 @Environment value type；AppState 走 @EnvironmentObject reference type）
- [Source: iphone/PetApp/App/RootView.swift] — 现有 RootView + LaunchedContentView 结构（Story 37.4 落地的 .environmentObject(appState) 链式注入模式）
- [Source: iphone/PetApp/App/MainTabView.swift#L68-L70] — Story 37.3 占位硬编码 Color/shadow + 注释指向待收口 story（本 story 仅更新注释指向 Story 37.7，不收口）
- [Source: iphone/PetApp/Features/Home/Views/HomeContainerView.swift#L77-L104] — 现有 EnvironmentKey 模式参考（ResetIdentityViewModelKey / SessionStoreKey；本 story Environment+Theme.swift 沿用同模式）
- [Source: iphone/ui_design/source/components/primitives.jsx] — CSS 变量 → Swift token 映射参考（如 `var(--accent)` → `theme.colors.accent`；按钮硬阴影 `0 4px 0 var(--accent-deep)` → Story 37.6 PrimaryButton 内组合，不抽到 ThemeShadow）
- [Source: docs/lessons/2026-04-30-strong-vs-weak-for-constructor-injected-state.md] — 构造注入语义参考（Theme 是 value type 不涉及，但 LaunchedContentView 的 currentTheme 透传走 `let` 不是 `weak` 与该 lesson 同精神）

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) — claude-opus-4-7[1m]

### Debug Log References

- `bash iphone/scripts/build.sh --test`：BUILD SUCCESS + Executed 251 tests, with 0 failures（含新增 ThemeTests 6 case 全绿）
- `grep -c "TODO(Story-Future)" iphone/PetApp/Core/DesignSystem/*.swift`：Theme.swift 3 条（matcha/sky/dark 整组 shadow 复用注释）+ ThemeColors.swift 31 条（matcha 10 + sky 10 + dark 10 + 1 个累计；具体逐字段 placeholder 注释）= 34 条，覆盖 stub 字段完整

### Completion Notes List

- AC1-AC6：6 个 token 容器结构全部就位（Theme / ThemeColors / ThemeSpacing / ThemeRadius / ThemeShadow / ThemeTypography），全部 `public struct` + `let` 字段（不可变 value type）+ Equatable，candy 完整、matcha / sky / dark 三主题 stub 仅显式落值 ui_design 指定字段，其余字段以 `ThemeColors.candy.X` placeholder + `// TODO(Story-Future): <主题名> <字段> 设计待定，当前复用 candy` 统一注释
- AC7：Environment+Theme.swift 落 `ThemeEnvironmentKey` (defaultValue = `.candy`) + `EnvironmentValues.theme` getter/setter（public scope 让 Feature View 能取）
- AC8：RootView 加 `@State private var currentTheme: ThemeName = .candy`；LaunchedContentView init / 字段同步加 `currentTheme: ThemeName`；`.ready` 分支 MainTabView 链上挂 `.environment(\.theme, currentTheme.theme)`，紧跟 `.environmentObject(appState)` 之后；其余三态机分支（launching / needsAuth）保持不挂，依赖 EnvironmentKey 默认值 fallback
- AC9：6 case 全绿（candy.accent / dark.pageBg hex 精确 + EnvironmentKey 默认 candy + ThemeName.theme 4 主题路由 + spacing 9 档抽样 + radius 关键 token 抽样）；`Color.uiColorHex` helper 用 UIColor → CGFloat → UInt32 round 规避 floating-point drift
- AC10：Theme.swift 底部 `#if DEBUG` 块加 `ThemePreview_Sampler` 私有 view + 4 个 `#Preview` 覆盖 candy / matcha / sky / dark
- AC11：xcodegen 通配规则自动 inclusion 8 个新文件（PetApp/Core/DesignSystem 7 个 + PetAppTests/Core/DesignSystem 1 个）；project.yml 不需手动改动；MainTabView.swift FloatingTabBar 注释从「Story 37.5 落地后改用 theme.colors」更新为「Story 37.7 HomeView Scaffold 落地时改用 theme.colors」（按 Sprint Change Proposal v2.4 §661 钦定 Story 37.7 收口路径）；硬编码 Color/shadow 不动

### File List

**新建**：
- iphone/PetApp/Core/DesignSystem/Theme.swift
- iphone/PetApp/Core/DesignSystem/ThemeColors.swift
- iphone/PetApp/Core/DesignSystem/ThemeSpacing.swift
- iphone/PetApp/Core/DesignSystem/ThemeRadius.swift
- iphone/PetApp/Core/DesignSystem/ThemeShadow.swift
- iphone/PetApp/Core/DesignSystem/ThemeTypography.swift
- iphone/PetApp/Core/DesignSystem/Environment+Theme.swift
- iphone/PetAppTests/Core/DesignSystem/ThemeTests.swift

**修改**：
- iphone/PetApp/App/RootView.swift（新增 `@State currentTheme` + LaunchedContentView 透传 + `.environment(\.theme, ...)` modifier）
- iphone/PetApp/App/MainTabView.swift（仅更新 FloatingTabBar 注释指向 Story 37.7；硬编码值不改）
- iphone/PetApp.xcodeproj/project.pbxproj（xcodegen regen 结果）

**sprint / story 元数据**：
- _bmad-output/implementation-artifacts/sprint-status.yaml（37-5-theme-design-tokens 状态 ready-for-dev → review）
- _bmad-output/implementation-artifacts/37-5-theme-design-tokens.md（Status / Tasks 勾选 / Dev Agent Record）

## Change Log

| Date       | Change                                                                                       |
|------------|----------------------------------------------------------------------------------------------|
| 2026-04-30 | 初稿落地：Theme 系统 6 个 token 容器（candy 完整 + matcha/sky/dark stub）+ EnvironmentKey 注入入口 + RootView `@State currentTheme` 注入路径 + 6 case 单元测试 + 4 主题 Preview + MainTabView 注释指向 Story 37.7。`bash iphone/scripts/build.sh --test` 251/251 全绿。 |
