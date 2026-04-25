// MockURLSession.swift
// Story 2.4 AC7：手写 mock URLSession（ADR-0002 §3.1：XCTest only，无第三方 mock 库）。
//
// 单测注入：通过 stubbedResponse / stubbedError 控制返回值或抛错；invocations 记录用于断言。

import Foundation
@testable import PetApp

final class MockURLSession: URLSessionProtocol, @unchecked Sendable {
    /// 受控返回：一组 (Data, URLResponse) 让测试设置预期
    var stubbedResponse: (Data, URLResponse)?
    /// 或受控抛错
    var stubbedError: Error?
    /// invocations 记录（手写 mock 标准模式 —— ADR-0002 §3.1）
    private(set) var invocations: [URLRequest] = []

    func data(for request: URLRequest) async throws -> (Data, URLResponse) {
        invocations.append(request)
        if let error = stubbedError {
            throw error
        }
        guard let response = stubbedResponse else {
            throw URLError(.badServerResponse)  // 测试未设置 stub，立即明确失败
        }
        return response
    }
}
