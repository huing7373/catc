// MockChestRepository.swift
// Story 21.2 AC7: MockChestRepository 测试 helper（与 sibling MockPetRepository / MockStepRepository
// 同模式：scripted Result + invocations 数组）.
// Story 21.3 AC9: 扩 openChest stub + lastOpenChestRequest 让 OpenChestUseCase 测试断言 request 体.
//
// 用法（LoadChestUseCaseTests / ChestRefreshTriggerServiceTests / OpenChestUseCaseTests）:
//   let repo = MockChestRepository()
//   repo.stubResponse = .success(ChestCurrentResponse(...))
//   repo.openChestStub = .success(ChestOpenResponse(...))
//   ...
//   XCTAssertEqual(repo.invocations, 1)
//   XCTAssertEqual(repo.openChestInvocations, 1)
//   XCTAssertEqual(repo.lastOpenChestRequest?.idempotencyKey, "test-key-1")
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

    /// 已发出的 fetchCurrent 调用数（无入参，仅记 count）.
    private(set) var invocations: Int = 0

    /// Story 21.3 AC9: scripted result for openChest（success / failure）.
    /// 默认 nil 时调用 XCTFail —— openChest 测试必须显式 set stub.
    var openChestStub: Result<ChestOpenResponse, Error>?

    /// Story 21.3 AC9: openChest 调用数（与 fetchCurrent invocations 区分）.
    private(set) var openChestInvocations: Int = 0

    /// Story 21.3 AC9: 最近一次 openChest 调用传入的 request（让测试断言 idempotencyKey）.
    private(set) var lastOpenChestRequest: ChestOpenRequest?

    /// Story 21.3 AC9: 所有 openChest 调用传入的 request 序列（让测试断言连续两次复用同 key 或新 key）.
    private(set) var openChestRequests: [ChestOpenRequest] = []

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

    func openChest(_ request: ChestOpenRequest) async throws -> ChestOpenResponse {
        openChestInvocations += 1
        lastOpenChestRequest = request
        openChestRequests.append(request)
        guard let stub = openChestStub else {
            XCTFail("MockChestRepository.openChestStub 未设置；test 必须显式 set stub")
            throw APIError.decoding(underlying: NSError(domain: "MockChestRepository", code: -101,
                userInfo: [NSLocalizedDescriptionKey: "MockChestRepository.openChestStub 未设置"]))
        }
        switch stub {
        case .success(let response): return response
        case .failure(let error): throw error
        }
    }
}
