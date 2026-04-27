// MockURLSession.swift
// Story 2.4 AC7：手写 mock URLSession（ADR-0002 §3.1：XCTest only，无第三方 mock 库）。
//
// 单测注入：通过 stubbedResponse / stubbedError 控制返回值或抛错；invocations 记录用于断言。
//
// Story 5.3 升级（基础设施修复）：`invocations` / `stubbedResponse` / `stubbedError` 全部
// 用 NSLock 保护 —— APIClientAuthInjectionTests case#7 并发 100 请求场景下，多 task 同时
// `data(for:)` → 同时 `invocations.append(...)` 不加锁会触发 race（TSAN 必报；偶发 crash）。
// 既有读取方式 `mock.invocations.count` 改为通过 `invocationsSnapshot()` 取快照，避免读 / 写
// 竞争；既有同步测试调用方零业务逻辑改动（snapshot 等价于直接读字段，仅多一层 lock）。
// 详见 lesson 2026-04-26-mockbase-snapshot-only-reads.md（同精神）。

import Foundation
@testable import PetApp

final class MockURLSession: URLSessionProtocol, @unchecked Sendable {
    /// 受控返回：一组 (Data, URLResponse) 让测试设置预期
    private var _stubbedResponse: (Data, URLResponse)?
    /// 或受控抛错
    private var _stubbedError: Error?
    /// invocations 记录（手写 mock 标准模式 —— ADR-0002 §3.1）；私有 + 通过 snapshot 读
    private var _invocations: [URLRequest] = []

    private let lock = NSLock()

    // MARK: - 测试访问入口（写）

    var stubbedResponse: (Data, URLResponse)? {
        get { lock.lock(); defer { lock.unlock() }; return _stubbedResponse }
        set { lock.lock(); defer { lock.unlock() }; _stubbedResponse = newValue }
    }

    var stubbedError: Error? {
        get { lock.lock(); defer { lock.unlock() }; return _stubbedError }
        set { lock.lock(); defer { lock.unlock() }; _stubbedError = newValue }
    }

    /// 测试访问入口（读）：返回 invocations 快照（拷贝 array），与 MockBase snapshot-only 同模式。
    /// 既有 `mock.invocations.count` / `mock.invocations.first` 风格调用零改动（snapshot 是 array 拷贝，
    /// 支持完全相同的 Sequence 操作）。
    var invocations: [URLRequest] {
        lock.lock()
        defer { lock.unlock() }
        return _invocations
    }

    func data(for request: URLRequest) async throws -> (Data, URLResponse) {
        lock.lock()
        _invocations.append(request)
        let stubError = _stubbedError
        let stubResponse = _stubbedResponse
        lock.unlock()

        if let error = stubError {
            throw error
        }
        guard let response = stubResponse else {
            throw URLError(.badServerResponse)  // 测试未设置 stub，立即明确失败
        }
        return response
    }
}
