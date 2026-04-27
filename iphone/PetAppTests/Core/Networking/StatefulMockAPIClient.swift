// StatefulMockAPIClient.swift
// Story 5.4 AC10: 按"调用次数序列"返回不同 stub 的 mock APIClient.
//
// 与 MockAPIClient（Story 2.5）的区别：
//   - MockAPIClient: stubResponse: [String: Stub] —— path → 单 stub —— 同一 path 多次调用都返同结果
//   - StatefulMockAPIClient: responseSequence: [String: [Stub]] —— path → stub 队列 —— 按调用顺序 pop
//
// 用途：
//   - AuthRetryingAPIClientTests 需要"第 1 次 401 + 第 2 次 success" 序列
//   - 5 并发场景需要序列共享（线程安全 pop）
//
// 不替换 MockAPIClient：
//   - MockAPIClient 模式简单，覆盖单次调用场景；既有 PingUseCaseTests 等不变
//   - 新建 StatefulMockAPIClient 让"复杂序列" 场景独立，避免污染既有 mock
//
// 线程安全：用 NSLock 保护 responseSequence + invocations（与 MockURLSession 同模式 —— Story 5.3 已加 lock）.

import Foundation
@testable import PetApp

#if DEBUG

final class StatefulMockAPIClient: APIClientProtocol, @unchecked Sendable {
    enum Stub {
        case success(any Decodable & Sendable)
        case failure(APIError)
    }

    private let lock = NSLock()
    private var _responseSequence: [String: [Stub]] = [:]
    private var _invocations: [Endpoint] = []
    private var _callCounts: [String: Int] = [:]

    var responseSequence: [String: [Stub]] {
        get { lock.lock(); defer { lock.unlock() }; return _responseSequence }
        set { lock.lock(); defer { lock.unlock() }; _responseSequence = newValue }
    }

    var invocations: [Endpoint] {
        lock.lock()
        defer { lock.unlock() }
        return _invocations
    }

    func callCount(forPath path: String) -> Int {
        lock.lock()
        defer { lock.unlock() }
        return _callCounts[path, default: 0]
    }

    func request<T: Decodable>(_ endpoint: Endpoint) async throws -> T {
        let stub: Stub? = {
            lock.lock()
            defer { lock.unlock() }
            _invocations.append(endpoint)
            _callCounts[endpoint.path, default: 0] += 1
            // 按 path pop 队首
            if var queue = _responseSequence[endpoint.path], !queue.isEmpty {
                let head = queue.removeFirst()
                _responseSequence[endpoint.path] = queue
                return head
            }
            return nil
        }()

        guard let stub else {
            // 序列耗尽 —— 测试 bug，应失败
            throw APIError.decoding(underlying: NSError(
                domain: "StatefulMockAPIClient",
                code: -200,
                userInfo: [NSLocalizedDescriptionKey:
                    "Stub sequence exhausted for path: \(endpoint.path) (call #\(callCount(forPath: endpoint.path)))"]
            ))
        }

        switch stub {
        case .success(let value):
            guard let typed = value as? T else {
                throw APIError.decoding(underlying: NSError(
                    domain: "StatefulMockAPIClient",
                    code: -201,
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

#endif
