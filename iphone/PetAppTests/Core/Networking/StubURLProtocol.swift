// StubURLProtocol.swift
// Story 2.4 AC8：URLProtocol-based fake server，集成测试拦截真 URLSession 网络层。
//
// ⚠️ 并发风险与使用约束（必须遵守）⚠️
// ---------------------------------------------------------------------------
// 本类的 stub 字段是 process-global static 状态。这意味着：
//
//   1. 同一进程内**任意**测试同时使用 StubURLProtocol 都会互相覆写 stub 字段。
//   2. XCTest 默认 same-class scope 串行执行，但跨 class 并行 / swift-testing
//      / `swift test --parallel` 会暴露竞态。
//
// 因此本文件级硬约定（**测试作者必须保证**）：
//
//   A. **任一时刻进程内只允许一个 testcase 在使用 StubURLProtocol**。
//      不要在 Swift Testing 的 `@Test` 套件里用，不要把 stub 注入挪到
//      `setUp(with:)` 之外。如果未来要并行化，**必须**把 static 字段改成
//      per-instance 或 per-Task local（见 TODO）。
//   B. **每个 testcase 都必须在 setUp 里 `StubURLProtocol.reset()`**，
//      tearDown 里再 reset 一次（防止异常路径残留）。
//   C. startLoading 内部对 static 字段的读取已用 `lock` 保护，避免
//      "另一个 testcase 在 reset() / 写入" 与 "本 testcase 的请求正在解析"
//      的最坏情况下读出一半新一半旧的 stub。但**这不能解决**约束 A 的语义
//      冲突（一个 case 期望 200，另一个期望 401，是测试设计问题不是锁能解的）。
//
// TODO（Story 2.7+ 可考虑）：用 `URLProtocol.setProperty(_:forKey:in:)` 把
// stub instance 绑到 URLRequest 上，彻底消除 static 状态。MVP 阶段不做。
//
// 不引第三方 mock server（如 swifter / Mockingjay）—— URLProtocol 是 Apple
// 官方推荐手段。

import Foundation

final class StubURLProtocol: URLProtocol {
    // MARK: - Global stub state（见文件头并发风险注释）

    private static let lock = NSLock()

    private static var _stubData: Data?
    private static var _stubStatusCode: Int = 200
    private static var _stubError: Error?

    static var stubData: Data? {
        get { lock.lock(); defer { lock.unlock() }; return _stubData }
        set { lock.lock(); defer { lock.unlock() }; _stubData = newValue }
    }

    static var stubStatusCode: Int {
        get { lock.lock(); defer { lock.unlock() }; return _stubStatusCode }
        set { lock.lock(); defer { lock.unlock() }; _stubStatusCode = newValue }
    }

    static var stubError: Error? {
        get { lock.lock(); defer { lock.unlock() }; return _stubError }
        set { lock.lock(); defer { lock.unlock() }; _stubError = newValue }
    }

    /// 一次性原子读取所有 stub 字段，避免在 startLoading 中途被并发写入打断。
    private static func snapshot() -> (data: Data?, statusCode: Int, error: Error?) {
        lock.lock()
        defer { lock.unlock() }
        return (_stubData, _stubStatusCode, _stubError)
    }

    /// 测试用例必须在 setUp / tearDown 调用，避免跨 case 污染。
    static func reset() {
        lock.lock()
        defer { lock.unlock() }
        _stubData = nil
        _stubStatusCode = 200
        _stubError = nil
    }

    // MARK: - URLProtocol overrides

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let snap = Self.snapshot()
        if let error = snap.error {
            client?.urlProtocol(self, didFailWithError: error)
            return
        }
        let response = HTTPURLResponse(
            url: request.url!,
            statusCode: snap.statusCode,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Type": "application/json"]
        )!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        if let data = snap.data {
            client?.urlProtocol(self, didLoad: data)
        }
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}
