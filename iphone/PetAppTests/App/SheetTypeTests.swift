// SheetTypeTests.swift
// Story 2.3 AC5：SheetType 的 Identifiable / Equatable 一致性（≥2 case）。
//
// 防 SwiftUI .fullScreenCover(item:) 因 id 重复导致 sheet 错乱。

import XCTest
@testable import PetApp

final class SheetTypeTests: XCTestCase {

    // MARK: - happy: 三个 case 的 id 两两不等

    func testSheetTypeIdsArePairwiseDistinct() throws {
        let ids = [
            SheetType.room.id,
            SheetType.inventory.id,
            SheetType.compose.id,
        ]
        let unique = Set(ids)
        XCTAssertEqual(unique.count, ids.count, "SheetType 三个 case 的 id 必须两两不等：\(ids)")
    }

    // MARK: - happy: Equatable 一致性（同 case 相等，跨 case 不等）

    func testSheetTypeEquatableConsistency() throws {
        XCTAssertEqual(SheetType.room, SheetType.room)
        XCTAssertEqual(SheetType.inventory, SheetType.inventory)
        XCTAssertEqual(SheetType.compose, SheetType.compose)

        XCTAssertNotEqual(SheetType.room, SheetType.inventory)
        XCTAssertNotEqual(SheetType.inventory, SheetType.compose)
        XCTAssertNotEqual(SheetType.room, SheetType.compose)
    }

    // MARK: - happy: id 字符串值符合命名约定（防 future rename id 破坏 .fullScreenCover 行为）

    func testSheetTypeIdStringsAreNonEmpty() throws {
        XCTAssertFalse(SheetType.room.id.isEmpty)
        XCTAssertFalse(SheetType.inventory.id.isEmpty)
        XCTAssertFalse(SheetType.compose.id.isEmpty)
    }
}
