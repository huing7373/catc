// HomeViewModelPingTests.swift
// Story 2.5 AC9：HomeViewModel.start() / bind() / applyPingResult 三态文案投影 + idempotency。
//
// 测试基础设施约束（与 Story 2.7 衔接）：
// - 仅依赖 stdlib（XCTest + @testable import PetApp），不引入第三方 mock 库（ADR-0002 §3.1）。
// - StubPingUseCase 是手写 mock（本文件底部），实现 PingUseCaseProtocol 并记录 executeCallCount。
//
// Story 2.5 Dev Note #7 提示：测试不直接断言 viewModel.appVersion 的值（避免依赖测试 host bundle）；
// 只断言 viewModel.serverInfo（PingUseCase 驱动，确定可控）。

import XCTest
@testable import PetApp

@MainActor
final class HomeViewModelPingTests: XCTestCase {

    /// case#1 (happy)：注入返回 reachable=true + commit 的 mock UseCase → start() 后 serverInfo == commit
    func testStartUpdatesServerInfoWithCommitOnSuccess() async {
        let stub = StubPingUseCase(stubResult: PingResult(reachable: true, serverCommit: "abc1234"))
        let viewModel = HomeViewModel(pingUseCase: stub)

        await viewModel.start()

        XCTAssertEqual(viewModel.serverInfo, "abc1234")
    }

    /// case#2 (edge)：reachable=false → serverInfo == "offline"
    func testStartUpdatesServerInfoToOfflineOnPingFailure() async {
        let stub = StubPingUseCase(stubResult: PingResult(reachable: false, serverCommit: nil))
        let viewModel = HomeViewModel(pingUseCase: stub)

        await viewModel.start()

        XCTAssertEqual(viewModel.serverInfo, "offline")
    }

    /// case#3 (edge)：reachable=true + commit nil → serverInfo == "v?"（部分降级）
    func testStartUpdatesServerInfoToVUnknownOnPartialDegrade() async {
        let stub = StubPingUseCase(stubResult: PingResult(reachable: true, serverCommit: nil))
        let viewModel = HomeViewModel(pingUseCase: stub)

        await viewModel.start()

        XCTAssertEqual(viewModel.serverInfo, "v?")
    }

    /// case#4 (edge)：reachable=true + commit 空字符串 → 走 "v?" 分支（兜底）
    func testStartTreatsEmptyCommitAsPartialDegrade() async {
        let stub = StubPingUseCase(stubResult: PingResult(reachable: true, serverCommit: ""))
        let viewModel = HomeViewModel(pingUseCase: stub)

        await viewModel.start()

        XCTAssertEqual(viewModel.serverInfo, "v?")
    }

    /// case#5 (happy)：重复调用 start() 不应触发重复请求
    func testStartIsIdempotentWhenCalledMultipleTimesConcurrently() async {
        let stub = StubPingUseCase(stubResult: PingResult(reachable: true, serverCommit: "abc1234"))
        let viewModel = HomeViewModel(pingUseCase: stub)

        // 并发触发两次 start；第一次跑完前第二次应短路
        async let first: Void = viewModel.start()
        async let second: Void = viewModel.start()
        _ = await (first, second)

        XCTAssertEqual(stub.executeCallCount, 1, "并发 start() 调用，UseCase.execute() 应只被调用 1 次")
        XCTAssertEqual(viewModel.serverInfo, "abc1234")
    }

    /// case#6 (edge)：未注入 pingUseCase（老路径） → start() 是 no-op，serverInfo 保持初始值
    func testStartIsNoOpWhenPingUseCaseNotInjected() async {
        let viewModel = HomeViewModel()  // 老 init，pingUseCase=nil 且 boundPingUseCase=nil

        await viewModel.start()

        XCTAssertEqual(viewModel.serverInfo, "----")
    }

    /// case#7 (happy)：bind(pingUseCase:) 后调 start() 应正常 ping
    func testBindThenStartUpdatesServerInfo() async {
        let stub = StubPingUseCase(stubResult: PingResult(reachable: true, serverCommit: "deadbee"))
        let viewModel = HomeViewModel()
        viewModel.bind(pingUseCase: stub)

        await viewModel.start()

        XCTAssertEqual(viewModel.serverInfo, "deadbee")
    }

    /// case#8 (edge)：bind 多次只第一次生效（防重复绑定）
    func testBindIsIdempotent() async {
        let stub1 = StubPingUseCase(stubResult: PingResult(reachable: true, serverCommit: "first"))
        let stub2 = StubPingUseCase(stubResult: PingResult(reachable: true, serverCommit: "second"))
        let viewModel = HomeViewModel()
        viewModel.bind(pingUseCase: stub1)
        viewModel.bind(pingUseCase: stub2)  // 应 no-op

        await viewModel.start()

        XCTAssertEqual(viewModel.serverInfo, "first", "bind 多次应只第一次生效")
        XCTAssertEqual(stub1.executeCallCount, 1)
        XCTAssertEqual(stub2.executeCallCount, 0)
    }

    /// case#9 (review fix round 1)：bind() 注入时同步刷新 appVersion = readAppVersion()。
    /// 防止 RootView 走老 init 路径时 appVersion 永远停在 "0.0.0" hardcode 默认值。
    /// 测试 host bundle 的 CFBundleShortVersionString 不可控，所以只断言：
    ///   - bind 前 appVersion == "0.0.0"（老 init 默认）
    ///   - bind 后 appVersion 与 HomeViewModel.readAppVersion() 一致（即从 Bundle 读到的值）
    func testBindUpdatesAppVersionFromBundle() async {
        let stub = StubPingUseCase(stubResult: PingResult(reachable: true, serverCommit: "abc1234"))
        let viewModel = HomeViewModel()
        XCTAssertEqual(viewModel.appVersion, "0.0.0", "老 init 默认 appVersion 应为 0.0.0")

        viewModel.bind(pingUseCase: stub)

        XCTAssertEqual(viewModel.appVersion, HomeViewModel.readAppVersion(),
                       "bind() 应同步刷新 appVersion 为 Bundle 读出的版本")
    }
}

/// 手写 mock：实现 PingUseCaseProtocol，记录调用次数，按 stub 返回受控 result。
final class StubPingUseCase: PingUseCaseProtocol, @unchecked Sendable {
    let stubResult: PingResult
    private(set) var executeCallCount = 0

    init(stubResult: PingResult) {
        self.stubResult = stubResult
    }

    func execute() async -> PingResult {
        executeCallCount += 1
        return stubResult
    }
}
