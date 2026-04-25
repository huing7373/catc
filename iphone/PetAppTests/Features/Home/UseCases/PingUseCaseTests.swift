// PingUseCaseTests.swift
// Story 2.5 AC9：DefaultPingUseCase 单元测试，覆盖三态语义 + 短路 + 解码失败 + 业务错误。
//
// 测试基础设施约束（与 Story 2.7 衔接）：
// - 仅依赖 stdlib（XCTest + @testable import PetApp），不引入第三方 mock 库（ADR-0002 §3.1）。
// - MockAPIClient 是手写 mock（同目录 MockAPIClient.swift），按 endpoint.path 路由 stub。

import XCTest
@testable import PetApp

@MainActor
final class PingUseCaseTests: XCTestCase {

    // MARK: - happy / partial-degrade / offline 三态

    /// case#1 (happy)：ping 成功 + version 成功 → reachable=true + commit 非空
    func testExecuteReturnsReachableWithCommitOnFullSuccess() async {
        let mock = MockAPIClient()
        mock.stubResponse[PingEndpoints.ping().path] = .success(Empty())
        mock.stubResponse[PingEndpoints.version().path] = .success(VersionResponse(
            commit: "abc1234",
            builtAt: "2026-04-26T08:00:00Z"
        ))

        let useCase = DefaultPingUseCase(client: mock)
        let result = await useCase.execute()

        XCTAssertEqual(result, PingResult(reachable: true, serverCommit: "abc1234"))
    }

    /// case#2 (edge)：ping 成功 + version network 失败 → reachable=true + commit nil（部分降级）
    func testExecuteReturnsReachableWithoutCommitWhenVersionFails() async {
        let mock = MockAPIClient()
        mock.stubResponse[PingEndpoints.ping().path] = .success(Empty())
        mock.stubResponse[PingEndpoints.version().path] = .failure(.network(underlying: URLError(.timedOut)))

        let useCase = DefaultPingUseCase(client: mock)
        let result = await useCase.execute()

        XCTAssertEqual(result, PingResult(reachable: true, serverCommit: nil))
    }

    /// case#3 (edge)：ping 失败 → reachable=false + commit nil；version 调用被短路
    func testExecuteReturnsOfflineWhenPingFails() async {
        let mock = MockAPIClient()
        mock.stubResponse[PingEndpoints.ping().path] = .failure(.network(underlying: URLError(.notConnectedToInternet)))
        // version stub 不设置：本 case 应该不被调到；如果 stub 缺失被命中会抛 APIError.decoding 暴露问题

        let useCase = DefaultPingUseCase(client: mock)
        let result = await useCase.execute()

        XCTAssertEqual(result, PingResult(reachable: false, serverCommit: nil))
        XCTAssertEqual(mock.invocations.map(\.path), ["/ping"], "ping 失败后 version 不应被调用")
    }

    /// case#4 (edge)：ping 业务错误（envelope code != 0）→ 视为 ping 失败 → offline
    func testExecuteReturnsOfflineWhenPingThrowsBusinessError() async {
        let mock = MockAPIClient()
        mock.stubResponse[PingEndpoints.ping().path] = .failure(.business(
            code: 1009,
            message: "服务繁忙",
            requestId: "req_x"
        ))

        let useCase = DefaultPingUseCase(client: mock)
        let result = await useCase.execute()

        XCTAssertEqual(result, PingResult(reachable: false, serverCommit: nil))
    }

    /// case#5 (edge)：version 解码失败（如 server 返回字段名变动）→ 部分降级
    func testExecuteReturnsPartialDegradeWhenVersionDecodingFails() async {
        let mock = MockAPIClient()
        mock.stubResponse[PingEndpoints.ping().path] = .success(Empty())
        mock.stubResponse[PingEndpoints.version().path] = .failure(.decoding(underlying: URLError(.cannotParseResponse)))

        let useCase = DefaultPingUseCase(client: mock)
        let result = await useCase.execute()

        XCTAssertEqual(result, PingResult(reachable: true, serverCommit: nil))
    }
}
