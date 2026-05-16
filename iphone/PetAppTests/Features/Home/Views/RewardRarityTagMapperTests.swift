// RewardRarityTagMapperTests.swift
// Story 21.4 AC6 / AC1: RewardRarityTagMapper 纯函数单测（4 case 覆盖 RewardRarity 4 档映射）.
//
// 测试目标:
//   - .common    → .N
//   - .rare      → .R
//   - .epic      → .SR
//   - .legendary → .SSR
//
// 测试边界（ADR-0002 §3.1 钦定 XCTest only）:
//   - 不依赖 SwiftUI 渲染; 仅断言 helper 函数 input → output 的纯映射规则.
//   - switch exhaustive 设计契约（mapper 不写 default）: RewardRarity 新增 case 编译期强制更新 mapper,
//     测试也需要新增一条 case → 双层保证不漏档.

import XCTest
@testable import PetApp

final class RewardRarityTagMapperTests: XCTestCase {

    // MARK: - case#1: .common → .N

    func testMapCommonReturnsN() {
        XCTAssertEqual(
            RewardRarityTagMapper.map(.common),
            .N,
            "RewardRarity.common 必须映射到 Rarity.N（灰色 #b0b0b0），与 V1 §6.9 + RarityTag 配色钦定一致"
        )
    }

    // MARK: - case#2: .rare → .R

    func testMapRareReturnsR() {
        XCTAssertEqual(
            RewardRarityTagMapper.map(.rare),
            .R,
            "RewardRarity.rare 必须映射到 Rarity.R（蓝色 #7db3e8）"
        )
    }

    // MARK: - case#3: .epic → .SR

    func testMapEpicReturnsSR() {
        XCTAssertEqual(
            RewardRarityTagMapper.map(.epic),
            .SR,
            "RewardRarity.epic 必须映射到 Rarity.SR（紫色 #c58ae8）"
        )
    }

    // MARK: - case#4: .legendary → .SSR

    func testMapLegendaryReturnsSSR() {
        XCTAssertEqual(
            RewardRarityTagMapper.map(.legendary),
            .SSR,
            "RewardRarity.legendary 必须映射到 Rarity.SSR（金红渐变 #ffd166 → #ef476f）"
        )
    }
}
