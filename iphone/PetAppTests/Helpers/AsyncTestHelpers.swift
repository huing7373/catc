// AsyncTestHelpers.swift
// Story 2.7 · ADR-0002 §3.2 落地：async/await 测试 helper。
//
// 提供两个 helper：
// 1. assertThrowsAsyncError(_:_:matcher:): 断言一段 async throws 表达式必抛错；
//    ADR-0002 §3.2 已知坑第 3 条要求落地
// 2. awaitPublishedChange(on:publisher:count:timeout:): 等待 @Published 字段变化 N 次；
//    ADR-0002 §3.2 "场景 1 多次值变化" 标准模式
//
// lesson 2026-04-25-swift-explicit-import-combine.md：用 ObservableObject / @Published / sink
// 必须显式 `import Combine`，本文件已显式 import 避免 implicit re-export 风险

import Combine
import Foundation
import XCTest

/// 断言一段 async throws 表达式抛出错误。
///
/// - Parameters:
///   - expression: 异步表达式（可抛错）
///   - message: 失败时的描述（XCTFail message）
///   - matcher: 可选的错误匹配闭包；返回 false 时断言失败（即"抛错了，但不是期望的错"）
///
/// 用法：
/// ```
/// await assertThrowsAsyncError(try await sut.doSomething()) { error in
///     guard case APIError.unauthorized = error else { return false }
///     return true
/// }
/// ```
///
/// 实装参考 ADR-0002 §3.2 已知坑第 3 条："`await assertThrowsAsyncError(...)` helper（Story 2.7
/// 落地一个 helper 函数，包装 `do { try await ...; XCTFail(...) } catch { ... }` 样板）"。
public func assertThrowsAsyncError<T>(
    _ expression: @autoclosure () async throws -> T,
    _ message: @autoclosure () -> String = "expected throw, got value",
    file: StaticString = #filePath,
    line: UInt = #line,
    matcher: ((Error) -> Bool)? = nil
) async {
    do {
        _ = try await expression()
        XCTFail(message(), file: file, line: line)
    } catch {
        if let matcher = matcher, !matcher(error) {
            XCTFail("error did not match: \(error)", file: file, line: line)
        }
    }
}

/// 等待某个 `@Published` 字段变化 `count` 次后返回收集到的值数组。
///
/// **Contract**: `count` 表示**变化次数**，**不含初始值**。
/// 调用方若需要 initial，请在调用前自己 `let initial = sut.status` 读出。
///
/// **同 run loop turn 内多次 mutation 的处理**：本 helper 用 `Published.Publisher` 订阅
/// （即 `\.$status` 这种 KeyPath），其语义是**每次 mutation 之前**同步 emit 即将赋的 NEW value。
/// 这意味着：
/// - 同一 run loop turn 内连续两次写 `.loading` 立即 `.ready`，会**同步**触发两次 emit，
///   collector 同步 append 两次，拿到 `[.loading, .ready]`；
/// - 而老的 `objectWillChange` + `DispatchQueue.main.async` 实现会让两次 sink 回调都跑在
///   final state 之后，错读成 `[.ready, .ready]`（lesson 2026-04-26-published-publisher-vs-objectwillchange.md）
///
/// - Parameters:
///   - object: 持有 @Published 字段的对象（不限于 ObservableObject）
///   - publisher: 指向 `Published<V>.Publisher` 的 KeyPath（用 `\.$fieldName` 写法 — 注意 `$`）
///   - count: 期望观察到的**变化**次数（不含初始值，默认 1 次）
///   - timeout: 超时秒数（默认 1 秒）
///
/// 用法（示意 .idle → .loading → .ready 的状态机）：
/// ```
/// let viewModel = SampleViewModel(useCase: mockUseCase)
/// let initial = viewModel.status  // .idle —— 调用方自取，helper 不返回
/// async let trigger: Void = viewModel.load()
/// // 期望 2 次变化：.loading（第 1 次）→ .ready（第 2 次）
/// let changes = try await awaitPublishedChange(
///     on: viewModel,
///     publisher: \.$status,
///     count: 2
/// )
/// XCTAssertEqual([initial] + changes, [.idle, .loading, .ready])
/// _ = await trigger
/// ```
///
/// 实装参考 ADR-0002 §3.2 "场景 1: 观察 @Published / Combine publisher 的多次值变化"。
/// Lesson 2026-04-26-objectwillchange-no-initial-emit.md 详述 contract 设计动机。
/// Lesson 2026-04-26-published-publisher-vs-objectwillchange.md 详述为何用 Published.Publisher
/// 而非 objectWillChange + dispatch async（避免 same-run-loop mutation 的 race）。
///
/// 实装注意：
/// - `Published<V>.Publisher` 在每次字段 mutation 之前同步 emit NEW value（与 objectWillChange
///   是变更通知不同 — 它直接 emit 即将赋的值），collector 同步 append，**不**经过 dispatch async
/// - 该 publisher 订阅时**会**emit 当前值（initial sink），helper 内部 drop 掉首条以保持
///   "不含初始值" 的 contract
/// - 用 `.prefix(count)` 在收到 count 个值后让 publisher 自动 completion，避免 publisher
///   emit 多于 count 时 sink 继续 fulfill 导致 XCTest over-fulfillment failure
///   （round 3 修复，lesson 2026-04-26-combine-prefix-vs-manual-fulfill.md）
/// - 内部 NSLock 保护 collected 数组（短临界区，不持锁调外部回调）
public func awaitPublishedChange<O: AnyObject, V>(
    on object: O,
    publisher keyPath: KeyPath<O, Published<V>.Publisher>,
    count: Int = 1,
    timeout: TimeInterval = 1.0,
    file: StaticString = #filePath,
    line: UInt = #line
) async throws -> [V] {
    // XCTestExpectation.expectedFulfillmentCount 不接受 0；调用方若想断言 "无变化"
    // 应该用别的手段（如 sleep + 直接读 @Published 当前值，或 sink 一段时间不 fulfill）。
    // 这里直接 precondition fail，给出明确报错而不是从 XCTest 内部抛出 API violation
    // 让调用方在 helper 调用栈外迷糊。
    // （lesson 2026-04-26-simulator-placeholder-vs-concrete.md / TODO 段：count==0 防御）
    precondition(count > 0, "awaitPublishedChange requires count > 0; to assert no changes, sample @Published value directly after a settled delay")

    // 用 _AsyncTestCollector 收集 + 加锁，避免在 generic function 内嵌套 class
    // （Swift 不允许在 generic function 体内定义 class）
    let collector = _AsyncTestCollector<V>()
    let expectation = XCTestExpectation(description: "awaitPublishedChange(\(keyPath))")
    expectation.expectedFulfillmentCount = count

    // Published.Publisher 在订阅时会同步 emit 当前值；用 dropFirst() 屏蔽 initial，
    // 保持 contract "不含初始值"。后续每次 mutation 之前 publisher 同步 emit NEW value，
    // collector 同步 append，拿到完整变化序列（即使同 run loop turn 内连发多次）。
    //
    // **`.prefix(count)` 是关键**：让 publisher 在 emit count 个值后自动发送 completion，
    // sink 上游被切断，不会再调 fulfill。否则 publisher emit > count 时（如 SampleViewModel.load
    // 同 run loop 内连发 .loading + .ready 而调用方只要 count: 1），sink 会继续 fulfill，
    // 触发 XCTest over-fulfillment failure（lesson 2026-04-26-combine-prefix-vs-manual-fulfill.md）
    let cancellable = object[keyPath: keyPath]
        .dropFirst()
        .prefix(count)
        .sink { value in
            collector.append(value)
            expectation.fulfill()
        }

    let result = await XCTWaiter.fulfillment(of: [expectation], timeout: timeout)
    cancellable.cancel()

    let snapshot = collector.snapshot()
    if result != .completed {
        XCTFail(
            "awaitPublishedChange timed out after \(timeout)s; got "
                + "\(snapshot.count)/\(count) changes",
            file: file,
            line: line
        )
    }

    return snapshot
}

/// Internal helper for `awaitPublishedChange`：用 NSLock 保护值数组，短临界区不持锁调外部回调
/// （与 lesson 2026-04-26-jsondecoder-encoder-thread-safety.md 第 36 行原则一致）。
final class _AsyncTestCollector<V>: @unchecked Sendable {
    private let lock = NSLock()
    private var values: [V] = []

    func append(_ value: V) {
        lock.lock()
        defer { lock.unlock() }
        values.append(value)
    }

    func snapshot() -> [V] {
        lock.lock()
        defer { lock.unlock() }
        return values
    }
}
