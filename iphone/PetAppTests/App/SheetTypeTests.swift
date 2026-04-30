// SheetTypeTests.swift
// Story 2.3 AC5：SheetType 的 Identifiable / Equatable 一致性.
//
// Story 37.3 修改（ADR-0009 §3.5 步骤 5）：
//   - 删除 `.room` / `.inventory` 用例（SheetType 已删这两个 case；主入口改 4 Tab IA）.
//   - 保留 `.compose` 一致性断言.

import XCTest
@testable import PetApp

final class SheetTypeTests: XCTestCase {

    // MARK: - happy: .compose id 非空 + 命名约定

    func testSheetTypeComposeIdIsNonEmptyAndStable() throws {
        XCTAssertEqual(SheetType.compose.id, "sheet_compose",
                       "SheetType.compose.id 必须等于 sheet_compose（防 id 字符串漂移破坏 .sheet 行为）")
        XCTAssertFalse(SheetType.compose.id.isEmpty)
    }

    // MARK: - happy: Equatable 一致性

    func testSheetTypeEquatableConsistency() throws {
        XCTAssertEqual(SheetType.compose, SheetType.compose)
    }

    // MARK: - Story 37.3 codex round 1 [P2] fix: ComposeSheetPlaceholder presenter 必须可构造

    /// codex round 1 [P2] fix regression guard: `LaunchedContentView .ready` 子树挂的
    /// `.fullScreenCover(item: $coordinator.presentedSheet)` modifier 在 `.compose` 分支
    /// 渲染 `ComposeSheetPlaceholder`. 该类型必须可构造, 否则 present(.compose) 仍会变 silent no-op.
    /// 详见 docs/lessons/2026-04-30-coordinator-must-mirror-loaded-home-room-state.md §Lesson 2.
    @MainActor
    func testComposeSheetPlaceholderIsConstructible() throws {
        let view = ComposeSheetPlaceholder()
        // 构造不抛 + 类型存在即满足回归守护;
        // a11y identifier `compose_placeholder` 由 UITest 验证 sheet 真正弹出.
        _ = view
    }
}
