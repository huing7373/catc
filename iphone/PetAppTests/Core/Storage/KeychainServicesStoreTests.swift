// KeychainServicesStoreTests.swift
// Story 5.1 AC4: KeychainServicesStore 真实类单元测试。
// 不用 MockKeychainStore —— 那是给上层用的；本 story 测真实 Security.framework 调用。
//
// 测试隔离强约束：setUp / tearDown 都必须 try? sut.removeAll()，避免：
// 1. 上一轮测试残留干扰本轮（同一 simulator 跨 test bundle 共享 keychain namespace）
// 2. 本轮测试残留泄漏到 simulator keychain 影响 dev 后续运行
//
// 在 simulator 上跑（CI 与本地都是 simulator）：iOS simulator keychain 与 macOS 系统
// keychain 隔离，写入不会污染 dev 主机的 keychain。

import XCTest
@testable import PetApp

final class KeychainServicesStoreTests: XCTestCase {

    var sut: KeychainServicesStore!

    override func setUp() {
        super.setUp()
        sut = KeychainServicesStore()
        // 测试隔离：每个 test 开始前确保 keychain 干净
        try? sut.removeAll()
    }

    override func tearDown() {
        // 测试隔离：每个 test 结束后清理，不泄漏
        try? sut?.removeAll()
        sut = nil
        super.tearDown()
    }

    // happy: set + get 同 key 返回相等值
    func testSetThenGetReturnsValue() throws {
        try sut.set("test-token-abc", forKey: KeychainKey.authToken.rawValue)
        let got = try sut.get(forKey: KeychainKey.authToken.rawValue)
        XCTAssertEqual(got, "test-token-abc")
    }

    // edge: get 不存在的 key 返回 nil（不抛错）
    func testGetNonExistentKeyReturnsNil() throws {
        let got = try sut.get(forKey: KeychainKey.guestUid.rawValue)
        XCTAssertNil(got)
    }

    // happy: set 同一 key 两次，get 返回最新值（upsert 行为）
    func testSetOverwritesExistingValue() throws {
        try sut.set("v1", forKey: KeychainKey.guestUid.rawValue)
        try sut.set("v2", forKey: KeychainKey.guestUid.rawValue)
        let got = try sut.get(forKey: KeychainKey.guestUid.rawValue)
        XCTAssertEqual(got, "v2")
    }

    // happy: remove 单个 key 后 get 返回 nil；其他 key 不受影响
    func testRemoveSingleKeyOnly() throws {
        try sut.set("uid-1", forKey: KeychainKey.guestUid.rawValue)
        try sut.set("token-1", forKey: KeychainKey.authToken.rawValue)
        try sut.remove(forKey: KeychainKey.guestUid.rawValue)
        XCTAssertNil(try sut.get(forKey: KeychainKey.guestUid.rawValue))
        XCTAssertEqual(try sut.get(forKey: KeychainKey.authToken.rawValue), "token-1")
    }

    // edge: remove 不存在的 key 不报错
    func testRemoveNonExistentKeyDoesNotThrow() {
        XCTAssertNoThrow(try sut.remove(forKey: KeychainKey.guestUid.rawValue))
    }

    // happy: removeAll 清空所有 key
    func testRemoveAllClearsAllKeys() throws {
        try sut.set("uid", forKey: KeychainKey.guestUid.rawValue)
        try sut.set("token", forKey: KeychainKey.authToken.rawValue)
        try sut.removeAll()
        XCTAssertNil(try sut.get(forKey: KeychainKey.guestUid.rawValue))
        XCTAssertNil(try sut.get(forKey: KeychainKey.authToken.rawValue))
    }

    // edge: removeAll 在空 keychain 上不报错
    func testRemoveAllOnEmptyDoesNotThrow() {
        XCTAssertNoThrow(try sut.removeAll())
    }

    // 持久化跨实例（同 process 内）：写入 sut1 的 value 能被 sut2 读到
    // 验证 keychain 真实持久化语义（不是某个 sut 实例的内部 state）
    func testPersistenceAcrossInstances() throws {
        let sut1 = KeychainServicesStore()
        try sut1.set("persist-test", forKey: KeychainKey.guestUid.rawValue)

        let sut2 = KeychainServicesStore()
        let got = try sut2.get(forKey: KeychainKey.guestUid.rawValue)
        XCTAssertEqual(got, "persist-test")
    }

    // edge: 协议 forKey 接受任意 String（不只 KeychainKey enum case），ad-hoc key 也能存取
    // 这保证 dev / 测试场景需要临时 key 时不被 enum 卡住
    func testArbitraryStringKeyWorks() throws {
        let adHocKey = "ad.hoc.key.\(UUID().uuidString)"
        XCTAssertNoThrow(try sut.set("ad-hoc-value", forKey: adHocKey))
        let got = try sut.get(forKey: adHocKey)
        XCTAssertEqual(got, "ad-hoc-value")
    }

    // KeychainKey enum AC1 验证：raw value 即真实 keychain account
    func testKeychainKeyRawValuesMatchExpectedNamespace() {
        XCTAssertEqual(KeychainKey.guestUid.rawValue, "auth.guestUid")
        XCTAssertEqual(KeychainKey.authToken.rawValue, "auth.token")
        XCTAssertEqual(KeychainKey.allCases.count, 2)
    }

    // KeychainError AC3 验证：errorDescription 包含 OSStatus 数字 + operation 名
    func testKeychainErrorDescriptionIncludesOperationAndStatus() {
        let error = KeychainError.osStatus(-25300, operation: "get")
        let desc = error.errorDescription ?? ""
        XCTAssertTrue(desc.contains("-25300"), "应包含 OSStatus 数字")
        XCTAssertTrue(desc.contains("get"), "应包含 operation 名")
    }

    // KeychainError AC3 验证：unexpectedDataFormat 描述含 operation
    func testKeychainErrorUnexpectedDataFormatDescription() {
        let error = KeychainError.unexpectedDataFormat(operation: "set")
        let desc = error.errorDescription ?? ""
        XCTAssertTrue(desc.contains("set"))
        XCTAssertTrue(desc.contains("UTF-8"))
    }
}
