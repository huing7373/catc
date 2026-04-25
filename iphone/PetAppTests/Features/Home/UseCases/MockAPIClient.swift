// MockAPIClient.swift
// Story 2.5 AC9：PingUseCase 单元测试用的 APIClientProtocol 手写 mock。
// 与 Story 2.4 的 MockURLSession 同风格，遵守 ADR-0002 §3.1（XCTest only，手写 Mock）。
//
// 用法：
//   let mock = MockAPIClient()
//   mock.stubResponse["/ping"] = .success(Empty())
//   mock.stubResponse["/version"] = .success(VersionResponse(commit: "abc", builtAt: "..."))
//   let useCase = DefaultPingUseCase(client: mock)
//
// 接受按 endpoint.path 字符串索引的 stub map。`request<T>` 实现：
//   - 找到 path 的 stub → 按 .success(value) / .failure(error) 行为返回 / 抛错
//   - 未找到 → 抛 APIError.decoding（StubMissingError）暴露测试遗漏
//
// 隔离设计：每个 testcase 自己 `let mock = MockAPIClient()`，无 static state，天然防跨测试污染
// （继承 Story 2.4 lesson 2026-04-26-urlprotocol-stub-global-state.md 的精神）。

import Foundation
@testable import PetApp

final class MockAPIClient: APIClientProtocol, @unchecked Sendable {
    enum Stub {
        case success(any Decodable & Sendable)
        case failure(APIError)
    }

    var stubResponse: [String: Stub] = [:]
    private(set) var invocations: [Endpoint] = []

    func request<T: Decodable>(_ endpoint: Endpoint) async throws -> T {
        invocations.append(endpoint)
        guard let stub = stubResponse[endpoint.path] else {
            throw APIError.decoding(underlying: NSError(
                domain: "MockAPIClient",
                code: -100,
                userInfo: [NSLocalizedDescriptionKey: "Stub missing for path: \(endpoint.path)"]
            ))
        }
        switch stub {
        case .success(let value):
            guard let typed = value as? T else {
                throw APIError.decoding(underlying: NSError(
                    domain: "MockAPIClient",
                    code: -101,
                    userInfo: [NSLocalizedDescriptionKey:
                        "Stub type mismatch: expected \(T.self), got \(type(of: value))"]
                ))
            }
            return typed
        case .failure(let error):
            throw error
        }
    }
}
