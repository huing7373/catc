// MockLoadHomeUseCase.swift
// Story 5.5 测试基础设施: LoadHomeUseCaseProtocol mock；让 HomeViewModel 测试不必构造 repo.
//
// stub 字段（executeStub）由 setUp 写入；method body 读取一次.

@testable import PetApp
import Foundation

#if DEBUG

final class MockLoadHomeUseCase: MockBase, LoadHomeUseCaseProtocol, @unchecked Sendable {
    var executeStub: Result<HomeData, Error> = .failure(MockError.notStubbed)

    func execute() async throws -> HomeData {
        record(method: "execute()")
        return try executeStub.get()
    }
}

#endif
