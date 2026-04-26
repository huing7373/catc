// LaunchingViewTests.swift
// Story 2.9 AC7：LaunchingView 视觉契约（用 a11y identifier 替代 SnapshotTesting lib）。
//
// 不用 SnapshotTesting / ViewInspector 等第三方（ADR-0002 §3.1 锁 XCTest only）。
// 替代策略：构造 LaunchingView 实例 + 直接断言 titleText 字面字符串 + a11y identifier 命名常量。
// 本 story 选最简单方案：构造 view → 验证 titleText + identifier，触发 body 让 init-time crash 暴露。
//
// 视觉细节（颜色 / 间距 / 大小）由 PetAppUITests/HomeUITests 的 testLaunchingViewVisibleBeforeHomeView
// 兜底 — 在真实模拟器上 launch 一遍验证 UI 可见性。

import XCTest
import SwiftUI
@testable import PetApp

final class LaunchingViewTests: XCTestCase {

    /// case#1：titleText 字面字符串契约 — UI 测试也按此定位，必须保持稳定。
    func testTitleTextIsExactString() {
        XCTAssertEqual(
            LaunchingView.titleText,
            "正在唤醒小猫…",
            "epics.md AC 钦定 LaunchingView 文字必须是 \"正在唤醒小猫…\"（含 unicode 省略号）"
        )
    }

    /// case#2：a11y identifier 常量契约 — UI 测试与 production 代码用同一组 identifier。
    func testAccessibilityIdentifiersAreDefined() {
        XCTAssertEqual(AccessibilityID.Launching.container, "launching_container")
        XCTAssertEqual(AccessibilityID.Launching.logo, "launching_logo")
        XCTAssertEqual(AccessibilityID.Launching.text, "launching_text")
        XCTAssertEqual(AccessibilityID.Launching.progressIndicator, "launching_progressIndicator")
    }

    /// case#3：LaunchingView 可被实例化（构造函数无需任何依赖）—— 验证它是 stateless View 而非
    /// 需要状态机注入的 component。
    func testLaunchingViewCanBeInstantiatedWithoutDependencies() {
        let view = LaunchingView()
        // 仅验证 init 不崩；body 计算在 SwiftUI host 渲染时才发生，单测层不强行触发。
        _ = view.body  // 触发 body 让任何 init-time crash 暴露
    }
}
