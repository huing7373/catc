// ThemeTests.swift
// Story 37.5 AC9: Theme 系统单元测试（≥3 case；本文件落地 6 case 覆盖 candy 完整 / dark 显式
// 字段 / EnvironmentKey 默认 / ThemeName 路由 / spacing 抽样 / radius 抽样）.
//
// 测试基础设施约束（与 Story 2.7 + ADR-0002 §3.1 衔接）：
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）.
//   - 不引 ViewInspector / SnapshotTesting.
//   - 全部测试 case 不依赖 SwiftUI hosting / View body 渲染（Theme 是 token 类型，纯值断言）.

import XCTest
import SwiftUI
@testable import PetApp

final class ThemeTests: XCTestCase {

    // MARK: - case#1 happy: candy.accent hex 精确

    /// 验证 candy.accent 的 sRGB hex 是 ui_design 钦定值 0xFF8FA3.
    /// 依赖 `Color.uiColorHex` helper（仅供测试，production 取色应走 `theme.colors.X`）.
    func testCandyAccentHexEqualsDesignSpec() {
        let hex = Theme.candy.colors.accent.uiColorHex
        XCTAssertEqual(hex, 0xFF8FA3, "candy.accent 应严格等于 ui_design §Design Tokens 钦定 0xFF8FA3")
    }

    // MARK: - case#2 happy: dark.pageBg hex 精确（ui_design 显式表述字段）

    /// 验证 dark.pageBg 是 ui_design README §Design Tokens 显式表述的 0x2A1C22.
    /// 这是 dark stub 唯一显式定值的 3 字段（pageBg / surface / ink）之一.
    func testDarkPageBgHexEqualsDesignSpec() {
        let hex = Theme.dark.colors.pageBg.uiColorHex
        XCTAssertEqual(hex, 0x2A1C22, "dark.pageBg 应严格等于 ui_design 钦定 0x2A1C22")
    }

    // MARK: - case#3 edge: EnvironmentKey 默认值是 candy

    /// 验证 EnvironmentValues.theme 默认值是 Theme.candy.
    /// 让任何忘了写 `.environment(\.theme, ...)` 的视图自动 fallback 到 candy（与 RootView
    /// `@State currentTheme = .candy` 默认一致）.
    @MainActor
    func testEnvironmentDefaultIsCandy() {
        let env = EnvironmentValues()
        XCTAssertEqual(env.theme, Theme.candy, "EnvironmentValues.theme 默认值必须 == Theme.candy")
    }

    // MARK: - case#4 happy: ThemeName.theme 路由正确

    /// 验证 4 个 ThemeName.theme 各自路由到对应的 Theme 静态实例.
    /// 依赖 Theme: Equatable.
    func testThemeNameRoutingMatchesStaticInstances() {
        XCTAssertEqual(ThemeName.candy.theme, Theme.candy)
        XCTAssertEqual(ThemeName.matcha.theme, Theme.matcha)
        XCTAssertEqual(ThemeName.sky.theme, Theme.sky)
        XCTAssertEqual(ThemeName.dark.theme, Theme.dark)
    }

    // MARK: - case#5 happy: 9 档 spacing 抽样（boundary + middle）

    /// 验证 ThemeSpacing.standard 9 档 boundary 值正确（防止有人把 default 参数改了）.
    func testSpacingStandardBoundariesMatchUiDesignScale() {
        let spacing = Theme.candy.spacing
        XCTAssertEqual(spacing.s8, 8)
        XCTAssertEqual(spacing.s16, 16)
        XCTAssertEqual(spacing.s28, 28)
    }

    // MARK: - case#6 happy: radius pill / cardLg / tagSm 抽样

    /// 验证 ThemeRadius.standard 关键 token 值（pill 必须 >= 容器最大高度 / 2 才能成圆药丸）.
    func testRadiusStandardKeyTokensMatchUiDesignScale() {
        let radius = Theme.candy.radius
        XCTAssertEqual(radius.tagSm, 6)
        XCTAssertEqual(radius.cardLg, 22)
        XCTAssertEqual(radius.pill, 999, "pill 须足够大让 SwiftUI clamp 到 height/2 实现圆药丸")
    }
}

// MARK: - 测试 fixture: Color → 24-bit RGB hex helper

extension Color {
    /// 测试用 helper: 从 SwiftUI Color 提取 24-bit RGB hex (忽略 alpha).
    /// **仅供测试断言**——production code 取色应走 `theme.colors.X` token 路径.
    ///
    /// 实现：Color → UIColor → CGFloat (r,g,b,a) → UInt32.rounded() 规避 floating-point drift
    /// （如 0xCC / 255 = 0.7999... ≠ 0.8 严格相等）.
    var uiColorHex: UInt32 {
        let uiColor = UIColor(self)
        var r: CGFloat = 0
        var g: CGFloat = 0
        var b: CGFloat = 0
        var a: CGFloat = 0
        uiColor.getRed(&r, green: &g, blue: &b, alpha: &a)
        let ri = UInt32((r * 255).rounded())
        let gi = UInt32((g * 255).rounded())
        let bi = UInt32((b * 255).rounded())
        return (ri << 16) | (gi << 8) | bi
    }
}
