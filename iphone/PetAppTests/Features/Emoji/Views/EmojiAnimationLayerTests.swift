// EmojiAnimationLayerTests.swift
// Story 18.4 AC9 9c: EmojiAnimationLayer.anchor(for:memberAnchors:centerAnchor:) static helper 单测.
//
// 测试范围 (story file AC9 钦定, ≥2 case):
//   - happy memberAnchors hit → 返该成员 anchor 点
//   - happy memberAnchors miss → 返 centerAnchor (V1 §12.3 行 2473 (c) center 降级)
//
// 设计原则: anchor 选择是纯函数 helper, 单测覆盖 anchor 选择逻辑;
//   动画 / 视图渲染 / @State 驱动行为靠 UITest + ios-simulator MCP 覆盖 (不在本单元测试范围).

import XCTest
import SwiftUI
@testable import PetApp

final class EmojiAnimationLayerTests: XCTestCase {

    // MARK: - case#1 happy: memberAnchors hit → 返该成员 anchor

    /// Story 18.4 AC6 helper: memberAnchors[userId] 命中 → 直接返 (该成员 PetSpriteView 中心).
    func test_anchor_memberAnchorsHit_returnsMemberAnchor() {
        let memberAnchors: [String: CGPoint] = [
            "u1": CGPoint(x: 100, y: 200),
            "u2": CGPoint(x: 50, y: 250)
        ]
        let centerAnchor = CGPoint(x: 200, y: 400)

        let resolved = EmojiAnimationLayer.anchor(
            for: "u1",
            memberAnchors: memberAnchors,
            centerAnchor: centerAnchor
        )

        XCTAssertEqual(resolved, CGPoint(x: 100, y: 200),
                       "memberAnchors hit 必返该成员 anchor, 不走 center fallback")
    }

    // MARK: - case#2 happy: memberAnchors miss → 返 centerAnchor (V1 §12.3 (c) center 降级)

    /// Story 18.4 AC6 helper: memberAnchors[userId] miss → 返 centerAnchor (V1 §12.3 行 2473 钦定).
    /// 场景: sender 已 leave + member.left 先到达 → 本地 roster 查不到 → renderer 用 centerAnchor.
    func test_anchor_memberAnchorsMiss_returnsCenterFallback() {
        let memberAnchors: [String: CGPoint] = [:]  // 空 / 完全 miss
        let centerAnchor = CGPoint(x: 200, y: 400)

        let resolved = EmojiAnimationLayer.anchor(
            for: "u_missing",
            memberAnchors: memberAnchors,
            centerAnchor: centerAnchor
        )

        XCTAssertEqual(resolved, CGPoint(x: 200, y: 400),
                       "memberAnchors miss 必返 centerAnchor (V1 §12.3 行 2473 (c) center 降级)")
    }

    // MARK: - case#3 edge: memberAnchors 有其他用户但目标用户 miss → 仍返 centerAnchor

    /// Story 18.4 AC6 helper: dict 非空但目标 userId miss 仍 fallback (不取任意其他用户的 anchor).
    func test_anchor_otherUsersPresentButTargetMiss_returnsCenterFallback() {
        let memberAnchors: [String: CGPoint] = [
            "u_a": CGPoint(x: 10, y: 20),
            "u_b": CGPoint(x: 30, y: 40)
        ]
        let centerAnchor = CGPoint(x: 200, y: 400)

        let resolved = EmojiAnimationLayer.anchor(
            for: "u_c",   // 不在 dict 中
            memberAnchors: memberAnchors,
            centerAnchor: centerAnchor
        )

        XCTAssertEqual(resolved, centerAnchor,
                       "其他成员存在但目标 userId miss 时, 必返 centerAnchor (不复用 u_a / u_b 的 anchor)")
    }
}
