// AsyncTestHelpers.swift
// Story 2.7 · ADR-0002 §3.2 落地：async/await 测试 helper。
//
// 提供两个 helper：
// 1. assertThrowsAsyncError(_:_:matcher:): 断言一段 async throws 表达式必抛错；
//    ADR-0002 §3.2 已知坑第 3 条要求落地
// 2. awaitPublishedChange(on:keyPath:count:timeout:): 等待 ObservableObject 的 @Published
//    字段变化 N 次；ADR-0002 §3.2 "场景 1 多次值变化" 标准模式
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

/// 等待 ObservableObject 上某个 @Published 字段变化 `count` 次后返回收集到的值数组。
///
/// **Contract**: `count` 表示**变化次数**（即 `objectWillChange` 信号次数），**不含初始值**。
/// `ObservableObject.objectWillChange` 是变化通知，订阅时不会 emit 当前 state。
/// 调用方若需要 initial，请在调用前自己 `let initial = sut.status` 读出。
///
/// - Parameters:
///   - object: ObservableObject 实例
///   - keyPath: 指向 @Published 字段的 keyPath（用 `\.fieldName` 写法）
///   - count: 期望观察到的**变化**次数（不含初始值，默认 1 次）
///   - timeout: 超时秒数（默认 1 秒）
///
/// 用法（示意 .idle → .loading → .ready 的状态机）：
/// ```
/// let viewModel = SampleViewModel(useCase: mockUseCase)
/// let initial = viewModel.status  // .idle —— 调用方自取，helper 不返回
/// async let trigger: Void = viewModel.load()
/// // 期望 2 次变化：.idle → .loading（第 1 次）→ .ready（第 2 次）
/// let changes = try await awaitPublishedChange(on: viewModel, keyPath: \.status, count: 2)
/// XCTAssertEqual([initial] + changes, [.idle, .loading, .ready])
/// _ = await trigger
/// ```
///
/// 实装参考 ADR-0002 §3.2 "场景 1: 观察 @Published / Combine publisher 的多次值变化"。
/// Lesson 2026-04-26-objectwillchange-no-initial-emit.md 详述 contract 设计动机。
///
/// 实装注意：
/// - ObservableObject 的 `objectWillChange` 在字段变更**之前**触发，所以 sink 闭包里读
///   `object[keyPath: keyPath]` 会读到旧值；用 `DispatchQueue.main.async` 让出一拍读到新值
///   （SwiftUI 内部观察机制的标准 workaround）
/// - 内部 NSLock 保护 collected 数组（短临界区，不持锁调外部回调）
public func awaitPublishedChange<O: ObservableObject, V>(
    on object: O,
    keyPath: KeyPath<O, V>,
    count: Int = 1,
    timeout: TimeInterval = 1.0,
    file: StaticString = #filePath,
    line: UInt = #line
) async throws -> [V] where O.ObjectWillChangePublisher == ObservableObjectPublisher {
    // ObservableObjectPublisher 在每次 @Published 字段变化前发出值；用它驱动观察。
    // 用 _AsyncTestCollector 收集 + 加锁，避免在 generic function 内嵌套 class
    // （Swift 不允许在 generic function 体内定义 class）
    let collector = _AsyncTestCollector<V>()
    let expectation = XCTestExpectation(description: "awaitPublishedChange(\(keyPath))")
    expectation.expectedFulfillmentCount = count

    let cancellable = object.objectWillChange
        .sink { _ in
            // objectWillChange 发出时字段尚未更新；用 DispatchQueue.main.async 让出一拍读到新值
            DispatchQueue.main.async {
                let value = object[keyPath: keyPath]
                collector.append(value)
                expectation.fulfill()
            }
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
