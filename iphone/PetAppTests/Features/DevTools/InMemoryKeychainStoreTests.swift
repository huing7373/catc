// InMemoryKeychainStoreTests.swift
// Story 2.8 AC9: KeychainStoreProtocol 占位实装的单元测试。
// 验证 set/get/remove/removeAll 协议契约；并发安全为占位实装的 NSLock 提供基本保障。

import XCTest
@testable import PetApp

#if DEBUG

final class InMemoryKeychainStoreTests: XCTestCase {

    var sut: InMemoryKeychainStore!

    override func setUp() {
        super.setUp()
        sut = InMemoryKeychainStore()
    }

    override func tearDown() {
        sut = nil
        super.tearDown()
    }

    /// happy: set + get 同 key 返回相等值。
    func testSetGetReturnsValue() throws {
        try sut.set("token-abc", forKey: "auth.token")
        let got = try sut.get(forKey: "auth.token")
        XCTAssertEqual(got, "token-abc")
    }

    /// happy: set 覆盖同 key 的旧值。
    func testSetOverwritesExistingValue() throws {
        try sut.set("v1", forKey: "k")
        try sut.set("v2", forKey: "k")
        XCTAssertEqual(try sut.get(forKey: "k"), "v2")
    }

    /// edge: get 不存在 key 返回 nil（不抛错）。
    func testGetNonExistentKeyReturnsNil() throws {
        let got = try sut.get(forKey: "missing")
        XCTAssertNil(got)
    }

    /// happy: removeAll 清空 storage，所有 key 之后 get 返回 nil。
    func testRemoveAllClearsStorage() throws {
        try sut.set("v1", forKey: "k1")
        try sut.set("v2", forKey: "k2")
        try sut.set("v3", forKey: "k3")

        try sut.removeAll()

        XCTAssertNil(try sut.get(forKey: "k1"))
        XCTAssertNil(try sut.get(forKey: "k2"))
        XCTAssertNil(try sut.get(forKey: "k3"))
    }

    /// edge: removeAll 在空 storage 上不抛。
    func testRemoveAllOnEmptyDoesNotThrow() throws {
        XCTAssertNoThrow(try sut.removeAll())
    }

    /// edge: remove 单个 key 不影响其他。
    func testRemoveSingleKeyOnly() throws {
        try sut.set("v1", forKey: "k1")
        try sut.set("v2", forKey: "k2")

        try sut.remove(forKey: "k1")

        XCTAssertNil(try sut.get(forKey: "k1"))
        XCTAssertEqual(try sut.get(forKey: "k2"), "v2")
    }

    /// edge: remove 不存在 key 不抛错。
    func testRemoveNonExistentKeyDoesNotThrow() throws {
        XCTAssertNoThrow(try sut.remove(forKey: "missing"))
    }
}

#endif
