// PingUseCaseIntegrationTests.swift
// Story 2.5 AC10：PingUseCase 端到端集成测试，用 PingStubURLProtocol fake 替代 server。
//
// 验证链路：DefaultPingUseCase → APIClient → URLSession → PingStubURLProtocol → 受控响应
//
// 设计选择：
// - PingStubURLProtocol 只 session-local 注入（URLSessionConfiguration.protocolClasses），
//   不调 URLProtocol.registerClass —— 严格遵守 Story 2.4 lesson 2026-04-26-urlprotocol-session-local-vs-global.md
// - setUp / tearDown 严格 reset 全局状态 —— 严格遵守 Story 2.4 lesson 2026-04-26-urlprotocol-stub-global-state.md
// - 每个 case 自建 URLSession + APIClient + UseCase，不共享 fixture（防跨测试污染）
// - 用 host-only baseURL（http://test-server.local），与 AppContainer 默认一致
//
// 不测真实 server（→ Epic 3 Story 3.1 / 3.2 跨端 E2E）；本文件 100% offline 跑。

import XCTest
@testable import PetApp

@MainActor
final class PingUseCaseIntegrationTests: XCTestCase {

    override func setUp() {
        super.setUp()
        PingStubURLProtocol.reset()
    }

    override func tearDown() {
        PingStubURLProtocol.reset()
        super.tearDown()
    }

    /// case#1 (happy)：ping + version 双成功 → 端到端解出 commit
    /// 验证：APIClient + JSONDecoder + Endpoint + PingUseCase 串联无误
    func testFullStackPingAndVersionHappyPath() async {
        // GIVEN：按 path 设两条路由
        PingStubURLProtocol.setRoute("/ping", statusCode: 200, data: """
        {"code":0,"message":"pong","data":{},"requestId":"req_ping_int"}
        """.data(using: .utf8)!)
        PingStubURLProtocol.setRoute("/version", statusCode: 200, data: """
        {"code":0,"message":"ok","data":{"commit":"abc1234","builtAt":"2026-04-26T08:00:00Z"},"requestId":"req_version_int"}
        """.data(using: .utf8)!)

        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [PingStubURLProtocol.self]
        let session = URLSession(configuration: config)
        let client = APIClient(baseURL: URL(string: "http://test-server.local")!, session: session)
        let useCase = DefaultPingUseCase(client: client)

        // WHEN
        let result = await useCase.execute()

        // THEN
        XCTAssertEqual(result, PingResult(reachable: true, serverCommit: "abc1234"))
    }

    /// case#2 (edge)：ping 路由返回 5xx → APIClient 抛 .network → PingUseCase 转 reachable=false
    func testFullStackPingFailureReturnsOffline() async {
        // GIVEN：ping 返回 500，version 故意不设路由（即使被调到也会 fail，但 ping 失败应短路）
        PingStubURLProtocol.setRoute("/ping", statusCode: 500, data: Data())

        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [PingStubURLProtocol.self]
        let session = URLSession(configuration: config)
        let client = APIClient(baseURL: URL(string: "http://test-server.local")!, session: session)
        let useCase = DefaultPingUseCase(client: client)

        // WHEN
        let result = await useCase.execute()

        // THEN
        XCTAssertEqual(result, PingResult(reachable: false, serverCommit: nil))
    }
}
