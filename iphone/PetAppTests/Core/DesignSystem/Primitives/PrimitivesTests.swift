// PrimitivesTests.swift
// Story 37.6 AC9: 共享 primitives 单元测试（≥3 case；本文件落地 5 case）.
//
// 测试基础设施约束（与 Story 2.7 + ADR-0002 §3.1 衔接）：
//   - 仅依赖 stdlib（XCTest + @testable import PetApp）.
//   - 不引 ViewInspector / SnapshotTesting.
//   - Icons 25 键映射 + symbol(for:) fallback + RarityTag enum 抽样 + PrimaryButton variant 抽样.

import XCTest
import SwiftUI
import UIKit
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

    // MARK: - case#6 contract: FadeIn 方向（"由下向上升起"，对齐 ui_design）

    /// 验证 FadeInModifier 起点 offsetY 是 +8（"由下向上升起"），不是 -8（反向）.
    /// 契约来源：`iphone/ui_design/source/screens/home.jsx:101-102` `@keyframes fadeIn`
    /// `from { opacity: 0; transform: translateY(8px); }`.
    /// 守护：fix-review round 1 / [P2]（此前 dev 错写成 -8 = 从上向下落，与 ui_design 反向）.
    func testFadeInOffsetStartIsPositiveEightFromBelow() {
        XCTAssertEqual(FadeInModifier.offsetStartY, 8,
                       "FadeIn 起点必须 +8（下方）→ 0（原位），即'由下向上升起'；"
                       + "若改成 -8 则反向，违反 ui_design home.jsx fadeIn keyframes 契约")
        XCTAssertEqual(FadeInModifier.offsetEndY, 0,
                       "FadeIn 终点必须 0（原位）")
    }
}
