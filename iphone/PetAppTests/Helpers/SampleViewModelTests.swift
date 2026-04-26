// SampleViewModelTests.swift
// Story 2.7 · 业务相关 mock 单元测试模板（epics.md Story 2.7 AC 强制：≥ 1 条）。
//
// 后续业务 story 写 ViewModel 测试时，**直接复制本文件结构**，改 type / mock / case 名即可。
// 文件结构：
//   1. 本地 mock（继承 MockBase）
//   2. setUp / tearDown
//   3. ≥ 3 case：happy + error + state-transition + helper-demo

import Combine
import XCTest
@testable import PetApp

#if DEBUG

@MainActor
final class SampleViewModelTests: XCTestCase {

    // MARK: - Local Mock（继承 MockBase；ADR-0002 §3.1 "至少记录 invocations + lastArguments"）

    final class MockSampleUseCase: MockBase, SampleUseCase, @unchecked Sendable {
        var stubResult: Result<Int, Error> = .failure(MockError.notStubbed)

        func execute(input: String) async throws -> Int {
            record(method: "execute(input:)", arguments: [input])
            return try stubResult.get()
        }
    }

    var sut: SampleViewModel!
    var mockUseCase: MockSampleUseCase!

    override func setUp() {
        super.setUp()
        mockUseCase = MockSampleUseCase()
        sut = SampleViewModel(useCase: mockUseCase)
    }

    override func tearDown() {
        sut = nil
        mockUseCase = nil
        super.tearDown()
    }

    // MARK: - Tests

    /// happy: useCase 返回值 → ViewModel 状态切换 .idle → .loading → .ready
    func testLoadHappyPath() async {
        mockUseCase.stubResult = .success(42)

        await sut.load(input: "hello")

        XCTAssertEqual(sut.status, .ready(value: 42))
        XCTAssertEqual(mockUseCase.callCount(of: "execute(input:)"), 1)
        XCTAssertEqual(mockUseCase.lastArgumentsSnapshot().first as? String, "hello")
    }

    /// edge: useCase 抛错 → ViewModel 状态切到 .failed
    func testLoadErrorPath() async {
        struct DemoError: Error {}
        mockUseCase.stubResult = .failure(DemoError())

        await sut.load(input: "world")

        if case .failed(let message) = sut.status {
            XCTAssertTrue(message.contains("DemoError"))
        } else {
            XCTFail("expected .failed, got \(sut.status)")
        }
        XCTAssertTrue(mockUseCase.wasCalled(method: "execute(input:)"))
    }

    /// happy: 状态转换序列被正确记录（演示 awaitPublishedChange 用法）
    ///
    /// 实装注意：`Published.Publisher` 在订阅时同步 emit initial，helper 内部 dropFirst 屏蔽掉；
    /// 后续每次 mutation 都会同步 emit NEW value。所以 sink 订阅 + 触发 load 的顺序无所谓，
    /// 即使同 run loop turn 内连发多次 mutation 也都能捕获到（不像旧的 objectWillChange +
    /// dispatch async 实现会因 race 错读 final state）。
    ///
    /// Contract（lesson 2026-04-26-objectwillchange-no-initial-emit.md）：
    /// `count` 表示**变化次数**，不含 initial。调用方自己读 `sut.status` 取初值。
    func testStatusTransitionsCaptured() async throws {
        mockUseCase.stubResult = .success(7)
        let initial = sut.status  // .idle —— helper 不返回 initial，调用方自取

        // 用 Task 延迟一拍触发 load，让 awaitPublishedChange 内部 sink 先订阅
        Task { @MainActor in
            try? await Task.sleep(nanoseconds: 50_000_000)  // 50ms
            await self.sut.load(input: "x")
        }
        let captured = try await awaitPublishedChange(
            on: sut,
            publisher: \.$status,
            count: 1,
            timeout: 2.0
        )

        // 至少能看到 1 次 transition（loading 或 ready）；ADR-0002 §3.2 容忍 ±1 漂移
        XCTAssertGreaterThanOrEqual(captured.count, 1)
        // initial 由调用方自取，不在 captured 内
        if case .idle = initial {} else {
            XCTFail("expected initial .idle, got \(initial)")
        }

        // 等待 load 完成（确保 ready 状态已落定）
        try? await Task.sleep(nanoseconds: 200_000_000)  // 200ms
        XCTAssertEqual(sut.status, .ready(value: 7))
    }

    /// contract: 等待 2 次变化拿到 `[loading, ready]`（不含初始 .idle）。
    /// 验证 lesson 2026-04-26-objectwillchange-no-initial-emit.md 规则：
    /// `count` = 变化次数，不含 initial。
    func testAwaitPublishedChangeExcludesInitialValue() async throws {
        mockUseCase.stubResult = .success(99)
        let initial = sut.status
        if case .idle = initial {} else {
            XCTFail("expected initial .idle before load, got \(initial)")
        }

        // 触发延迟一拍，让 sink 先订阅
        Task { @MainActor in
            try? await Task.sleep(nanoseconds: 50_000_000)
            await self.sut.load(input: "ctx")
        }
        let captured = try await awaitPublishedChange(
            on: sut,
            publisher: \.$status,
            count: 2,
            timeout: 2.0
        )

        // captured 应当为 2 次变化（loading + ready），且第一项不是 .idle
        XCTAssertEqual(captured.count, 2, "expected exactly 2 changes (loading + ready)")
        if let first = captured.first, case .idle = first {
            XCTFail("captured[0] must NOT be initial .idle (contract: no initial emit)")
        }
        // 末态确认 ready
        if let last = captured.last, case .ready(let v) = last {
            XCTAssertEqual(v, 99)
        } else {
            XCTFail("captured.last expected .ready(99), got \(String(describing: captured.last))")
        }
    }

    /// contract (round 2): 同 run loop turn 内的多次 mutation 必须**全部**被捕获，
    /// 不能因 dispatch async 让 sink 回调跑在 final state 之后而丢失中间值。
    ///
    /// 验证 lesson 2026-04-26-published-publisher-vs-objectwillchange.md 核心规则：
    /// `Published.Publisher` 在 mutation 之前同步 emit NEW value，按顺序到达。
    /// 旧的 `objectWillChange + DispatchQueue.main.async` 实现在此场景下会读到
    /// `[.ready, .ready]`（两次 callback 都跑在 final state 之后），而新实现拿到
    /// `[.loading, .ready]`。
    func testAwaitPublishedChangeCapturesAllIntermediateValues() async throws {
        // Helper 对象：在单个 run loop turn 内同步连发两次 @Published mutation
        final class Burst: ObservableObject {
            @Published var value: Int = 0
            func bump() {
                value = 1
                value = 2
            }
        }
        let burst = Burst()

        // 50ms 后让 burst 在同一 run loop turn 内连发两次 mutation
        Task { @MainActor in
            try? await Task.sleep(nanoseconds: 50_000_000)
            burst.bump()
        }

        let captured = try await awaitPublishedChange(
            on: burst,
            publisher: \.$value,
            count: 2,
            timeout: 2.0
        )

        // 关键断言：同 run loop turn 内的两次 mutation 都被同步捕获，**不**漏中间值 1
        XCTAssertEqual(
            captured,
            [1, 2],
            "Published.Publisher 必须按 mutation 顺序同步 emit；旧 objectWillChange + dispatch async 在此场景会错读为 [2, 2]"
        )
    }

    /// edge: assertThrowsAsyncError helper 用法演示
    func testAssertThrowsAsyncErrorHelper() async {
        struct StubError: Error, Equatable {}
        mockUseCase.stubResult = .failure(StubError())

        await assertThrowsAsyncError(try await mockUseCase.execute(input: "x")) { error in
            error is StubError
        }
    }

    /// helper: MockBase reset() 行为验证
    func testMockBaseResetClearsState() {
        mockUseCase.record(method: "foo()", arguments: [1, 2])
        XCTAssertEqual(mockUseCase.callCount(of: "foo()"), 1)
        XCTAssertFalse(mockUseCase.invocationsSnapshot().isEmpty)

        mockUseCase.reset()

        XCTAssertEqual(mockUseCase.callCount(of: "foo()"), 0)
        XCTAssertTrue(mockUseCase.invocationsSnapshot().isEmpty)
        XCTAssertFalse(mockUseCase.wasCalled(method: "foo()"))
    }
}

#endif
