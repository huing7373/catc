// MockInventoryRepository.swift
// Story 24.2 AC「单元测试覆盖」: MockInventoryRepository 测试 helper（与 sibling
// MockChestRepository 同模式：scripted Result 队列 + invocations）.
//
// 用法（LoadInventoryUseCaseTests）:
//   let repo = MockInventoryRepository()
//   repo.stubResponses = [.success(InventoryResponse(groups: [...]))]   // 单次
//   // 或多次（验证「失败后重试」）：
//   repo.stubResponses = [.failure(APIError.network(...)), .success(InventoryResponse(groups: [...]))]
//   ...
//   XCTAssertEqual(repo.invocations, 2)
//
// scripted 队列语义：每次 fetchInventory() 取队首 stub（FIFO 消费）；队列空时复用最后一个
// （让「重复 execute 都成功」不必塞多份）；从未 set 时抛 decoding（防误用 —— test 必须显式 set）.
//
// 注：本 mock **不**继承 MockBase（与 MockChestRepository 同模式 —— scripted Result + 自管
// invocations 数组就够，保持与既有 Wardrobe / chest 链路单测 mock 风格一致）.

import XCTest
@testable import PetApp

final class MockInventoryRepository: InventoryRepositoryProtocol, @unchecked Sendable {
    /// scripted result 队列：每次 fetchInventory() 按 FIFO 取一个；
    /// 队列耗尽后复用最后一个 stub（让「重复 execute 都成功」简洁）.
    var stubResponses: [Result<InventoryResponse, Error>] = []

    /// 已发出的 fetchInventory 调用数（无入参，仅记 count）.
    private(set) var invocations: Int = 0

    private var cursor: Int = 0

    func fetchInventory() async throws -> InventoryResponse {
        invocations += 1
        guard !stubResponses.isEmpty else {
            throw APIError.decoding(underlying: NSError(
                domain: "MockInventoryRepository", code: -100,
                userInfo: [NSLocalizedDescriptionKey: "MockInventoryRepository.stubResponses 未设置"]))
        }
        let idx = min(cursor, stubResponses.count - 1)
        cursor += 1
        switch stubResponses[idx] {
        case .success(let response): return response
        case .failure(let error): throw error
        }
    }
}
