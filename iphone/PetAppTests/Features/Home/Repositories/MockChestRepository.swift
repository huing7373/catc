// MockChestRepository.swift
// Story 21.2 AC7: MockChestRepository 测试 helper（与 sibling MockPetRepository / MockStepRepository
// 同模式：scripted Result + invocations 数组）.
//
// 用法（LoadChestUseCaseTests / ChestRefreshTriggerServiceTests）:
//   let repo = MockChestRepository()
//   repo.stubResponse = .success(ChestCurrentResponse(...))
//   ...
//   XCTAssertEqual(repo.invocations.count, 1)
//
// 注：本 mock **不**继承 MockBase（与 MockPetRepository 同模式 —— scripted Result + 自管 invocations
// 数组就够，无需 MockBase 提供的 method name 字符串化 + lastArguments 通用 API；保持与既有 Pet
// state-sync 链路单测 mock 风格一致，让本 story 测试体感无新概念）.

import XCTest
@testable import PetApp

final class MockChestRepository: ChestRepositoryProtocol, @unchecked Sendable {
    /// scripted result：调 fetchCurrent 时返回该 stub（success / failure）.
    /// 默认 nil 时调用直接抛 APIError.decoding（防误用 —— test 必须显式 set stub）.
    var stubResponse: Result<ChestCurrentResponse, Error>?

    /// 已发出的 request 调用数（fetchCurrent 无入参，仅记 count）.
    private(set) var invocations: Int = 0

    func fetchCurrent() async throws -> ChestCurrentResponse {
        invocations += 1
        guard let stub = stubResponse else {
            throw APIError.decoding(underlying: NSError(domain: "MockChestRepository", code: -100,
                userInfo: [NSLocalizedDescriptionKey: "MockChestRepository.stubResponse 未设置"]))
        }
        switch stub {
        case .success(let response): return response
        case .failure(let error): throw error
        }
    }
}
