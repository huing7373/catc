// PingStubURLProtocol.swift
// Story 2.5 AC10：按 URL path 路由响应的 URLProtocol stub，用于 PingUseCase 集成测试。
//
// 与 Story 2.4 的 StubURLProtocol 区别：
// - StubURLProtocol 是**单一 stub**模式（一组 static 字段，所有请求返回同一响应）
// - PingStubURLProtocol 支持**按 path 分支**（同一 session 内多次请求可返回不同响应）
//
// 隔离原则：本 stub 是 process-global static 状态，与 StubURLProtocol 同样有并发风险。
// 严格继承 Story 2.4 两条 lesson：
//   - 2026-04-26-urlprotocol-stub-global-state.md：static 字段读写均加 NSLock；snapshot 原子读
//   - 2026-04-26-urlprotocol-session-local-vs-global.md：仅 session-local 注入
//     （URLSessionConfiguration.protocolClasses），**禁止** URLProtocol.registerClass(_:)
//
// 测试作者必须保证：
//   A. 任一时刻进程内只允许一个 testcase 用本工具
//   B. 每个 testcase 在 setUp 和 tearDown 中调用 reset()
//   C. URLSession 必须 per-case 自建（用 URLSessionConfiguration），不要污染 URLSession.shared

import Foundation

final class PingStubURLProtocol: URLProtocol {
    // MARK: - Global stub state（见文件头并发风险注释）

    private static let lock = NSLock()
    private static var _routes: [String: (statusCode: Int, data: Data)] = [:]

    static func setRoute(_ path: String, statusCode: Int, data: Data) {
        lock.lock()
        defer { lock.unlock() }
        _routes[path] = (statusCode, data)
    }

    static func reset() {
        lock.lock()
        defer { lock.unlock() }
        _routes = [:]
    }

    /// 一次性原子读取目标路由，避免在 startLoading 中途被并发写入打断。
    private static func snapshot(for path: String) -> (statusCode: Int, data: Data)? {
        lock.lock()
        defer { lock.unlock() }
        return _routes[path]
    }

    // MARK: - URLProtocol overrides

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let path = request.url?.path ?? ""
        guard let route = Self.snapshot(for: path) else {
            client?.urlProtocol(self, didFailWithError: URLError(.fileDoesNotExist))
            return
        }
        let response = HTTPURLResponse(
            url: request.url!,
            statusCode: route.statusCode,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        )!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: route.data)
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}
