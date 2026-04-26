// MockKeychainStore.swift
// Story 2.8: KeychainStoreProtocol 的测试 mock；继承 MockBase（Story 2.7 落地）。
//
// stub 字段（setStubError / getStubResult / removeStubError / removeAllStubError）
// 由测试 setUp 阶段写入；method body 读取一次后立即用，符合 MockBase snapshot-only 精神
// （lesson 2026-04-26-mockbase-snapshot-only-reads.md）。
//
// import Combine 不需要：本 mock 不持 ObservableObject / @Published 字段。

@testable import PetApp
import Foundation

#if DEBUG

final class MockKeychainStore: MockBase, KeychainStoreProtocol, @unchecked Sendable {
    var setStubError: Error?
    var getStubResult: Result<String?, Error> = .success(nil)
    var removeStubError: Error?
    var removeAllStubError: Error?

    func set(_ value: String, forKey key: String) throws {
        record(method: "set(_:forKey:)", arguments: [value, key])
        if let e = setStubError { throw e }
    }

    func get(forKey key: String) throws -> String? {
        record(method: "get(forKey:)", arguments: [key])
        return try getStubResult.get()
    }

    func remove(forKey key: String) throws {
        record(method: "remove(forKey:)", arguments: [key])
        if let e = removeStubError { throw e }
    }

    func removeAll() throws {
        record(method: "removeAll()")
        if let e = removeAllStubError { throw e }
    }
}

#endif
