// MockPetRepository.swift
// Story 15.4 AC5: MockPetRepository 测试 helper（与 sibling MockStepRepository 同模式：
// scripted Result + invocations 数组）.
//
// 用法（SyncPetStateUseCaseTests / PetStateSyncTriggerServiceTests）:
//   let repo = MockPetRepository()
//   repo.stubResponse = .success(PetStateSyncResponse(state: 2))
//   ...
//   XCTAssertEqual(repo.invocations.count, 1)
//   XCTAssertEqual(repo.invocations.first?.state, 2)

import XCTest
@testable import PetApp

final class MockPetRepository: PetRepositoryProtocol, @unchecked Sendable {
    /// scripted result：调 syncPetState 时返回该 stub（success / failure）.
    /// 默认 nil 时调用直接抛 APIError.decoding（防误用 —— test 必须显式 set stub）.
    var stubResponse: Result<PetStateSyncResponse, Error>?

    /// 已发出的 request 列表（让测试断言"发了几次 + 入参是什么"）.
    private(set) var invocations: [PetStateSyncRequest] = []

    func syncPetState(_ request: PetStateSyncRequest) async throws -> PetStateSyncResponse {
        invocations.append(request)
        guard let stub = stubResponse else {
            throw APIError.decoding(underlying: NSError(domain: "MockPetRepository", code: -100,
                userInfo: [NSLocalizedDescriptionKey: "MockPetRepository.stubResponse 未设置"]))
        }
        switch stub {
        case .success(let response): return response
        case .failure(let error): throw error
        }
    }
}
