// URLSessionProtocol.swift
// Story 2.4 AC4：URLSession 抽象切口（mock 注入用）。
//
// 单测通过 mock URLSessionProtocol 注入受控 (Data, URLResponse) 或 throw URLError；
// 集成测试（同 story AC8）则用真 URLSession + StubURLProtocol 拦截。
// 协议方法签名与 URLSession.data(for:) 完全一致——空 extension 即可让 URLSession 符合协议。

import Foundation

/// URLSession 抽象切口。
///
/// 目的：让单元测试通过 mock 注入受控的 (Data, URLResponse) 或 throw URLError，
/// 验证 APIClient 的解码 / 错误映射 / 401 路径，无需真启动 HTTP server。
///
/// 集成测试（同 story AC8）会启动真 URLSession + StubURLProtocol 拦截，
/// 不通过本协议——两条路径互补。
public protocol URLSessionProtocol: Sendable {
    func data(for request: URLRequest) async throws -> (Data, URLResponse)
}

/// URLSession 通过空 extension 自然实现该 protocol（API 已存在）。
extension URLSession: URLSessionProtocol {}
