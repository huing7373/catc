// MockHomeRepository.swift
// Story 5.5 测试基础设施: HomeRepositoryProtocol mock；继承 MockBase（Story 2.7 落地）.
//
// 与 MockAuthRepository 同模式：stub 字段（loadHomeStub）由 setUp 写入；method body 读取一次.
// 符合 MockBase snapshot-only 精神（lesson 2026-04-26-mockbase-snapshot-only-reads.md）.

@testable import PetApp
import Foundation

#if DEBUG

final class MockHomeRepository: MockBase, HomeRepositoryProtocol, @unchecked Sendable {
    var loadHomeStub: Result<HomeResponse, Error> = .failure(MockError.notStubbed)

    func loadHome() async throws -> HomeResponse {
        record(method: "loadHome()")
        return try loadHomeStub.get()
    }
}

#endif
