// MockBase.swift
// Story 2.7 · ADR-0002 §3.1 落地：手写 Mock 通用基类。
//
// 设计目标：让后续业务 mock（MockAuthRepository / MockHomeRepository / MockChestUseCase 等）
// 通过继承（class）或组合（struct/actor）方式复用 invocations 记录 + lastArguments + 线程安全机制。
//
// 现有 networking-specific mock 不强制迁移：
// - MockURLSession (Story 2.4) 已有 invocations: [URLRequest] 模式（手写实装）
// - MockAPIClient (Story 2.5) 已有 invocations: [Endpoint] 模式（手写实装）
// 两者对应 ADR-0002 §3.1 "至少记录 invocations + lastArguments" 精神，不需要改。
// 新写业务 mock 优先继承 MockBase 或包含 MockBase 字段；老 mock 保持原样。
//
// 用法 1：class 继承（推荐，业务 mock 大多是 class）
//
//   final class MockChestRepository: MockBase, ChestRepository, @unchecked Sendable {
//       var openChestStubResult: Result<Reward, Error> = .failure(MockError.notStubbed)
//       func openChest(idempotencyKey: String) async throws -> Reward {
//           record(method: "openChest(idempotencyKey:)", arguments: [idempotencyKey])
//           return try openChestStubResult.get()
//       }
//   }
//
// 用法 2：struct / actor 用组合
//
//   actor MockSomeActor: SomeProtocol {
//       private let mockBase = MockBase()
//       func doStuff(arg: Int) async {
//           mockBase.record(method: "doStuff(arg:)", arguments: [arg])
//       }
//   }
//
// 线程安全：内部 NSLock 保护 invocations / lastArguments / callCounts；多 task 调用同一 mock 不污染。
//
// 设计参考：
// - server 端 `internal/service/sample/MockSampleRepo`（testify/mock 模式 —— 用 m.Called 记录调用）
// - lesson 2026-04-26-urlprotocol-stub-global-state.md（NSLock + snapshot 原子读模式）

import Foundation

/// `MockBase`：手写 mock 通用基类，提供 invocations 记录 + lastArguments 字段 + NSLock 线程安全。
///
/// 子类典型用法（class 继承）：
/// 1. `final class MockXxx: MockBase, XxxProtocol, @unchecked Sendable { }`
/// 2. 在协议方法里 `record(method: "<funcName>", arguments: [...])` 一行
/// 3. stub 字段（如 `var stubResult: Result<...>`）由子类自己声明
///
/// 注意：`MockBase` 本身**不**标 `@unchecked Sendable`。子类持有 stub 字段（mutable 状态）
/// 的 Sendable 性由子类决策；让子类显式 `@unchecked Sendable`，避免 MockBase 替子类做隐式承诺
/// （与 MockURLSession / MockAPIClient 同模式）。
///
/// **线程安全 contract（lesson 2026-04-26-mockbase-snapshot-only-reads.md）**：
/// 内部存储字段（invocations / lastArguments / callCounts）一律 `private`，
/// **唯一**对外读 API 是 `*Snapshot()` / `wasCalled(...)` / `callCount(of:)` 这一组方法 —
/// 它们都在 `lock` 内拷贝再返回，调用者拿到的是不可变 snapshot。**禁止**把存储字段提升为
/// public 让外部直接读（即使加 `private(set)`），那会 bypass 锁形成 race（TSAN 必报）。
public class MockBase {
    /// 调用记录（每次 record() 追加一条）。`private` — 仅通过 `invocationsSnapshot()` 读。
    private var invocations: [String] = []

    /// 最近一次调用的参数（任意类型 array）。`private` — 仅通过 `lastArgumentsSnapshot()` 读。
    private var lastArguments: [Any] = []

    /// 每个方法名 → 调用次数。`private` — 仅通过 `callCountsSnapshot()` 读。
    private var callCounts: [String: Int] = [:]

    private let lock = NSLock()

    public init() {}

    /// 记录一次方法调用：`invocations` 追加方法名；`lastArguments` 覆写为本次参数；
    /// `callCounts[method] += 1`。
    /// - Parameters:
    ///   - method: 方法签名字符串（建议 "funcName(label1:label2:)" 风格便于断言）
    ///   - arguments: 本次实参 array（任意类型；用于断言"上次传的是什么"）
    public func record(method: String, arguments: [Any] = []) {
        lock.lock()
        defer { lock.unlock() }
        invocations.append(method)
        lastArguments = arguments
        callCounts[method, default: 0] += 1
    }

    /// 快照式读 invocations（避免迭代过程中被并发写入影响）。
    public func invocationsSnapshot() -> [String] {
        lock.lock()
        defer { lock.unlock() }
        return invocations
    }

    /// 快照式读 lastArguments（最近一次 record 的实参 array 拷贝）。
    public func lastArgumentsSnapshot() -> [Any] {
        lock.lock()
        defer { lock.unlock() }
        return lastArguments
    }

    /// 快照式读 callCounts。
    public func callCountsSnapshot() -> [String: Int] {
        lock.lock()
        defer { lock.unlock() }
        return callCounts
    }

    /// 重置所有记录（测试 tearDown 用）。线程安全。
    public func reset() {
        lock.lock()
        defer { lock.unlock() }
        invocations.removeAll(keepingCapacity: true)
        lastArguments.removeAll(keepingCapacity: true)
        callCounts.removeAll(keepingCapacity: true)
    }

    /// 断言指定方法是否被调用过（次数 >= 1）。
    public func wasCalled(method: String) -> Bool {
        callCountsSnapshot()[method, default: 0] > 0
    }

    /// 断言指定方法的调用次数。
    public func callCount(of method: String) -> Int {
        callCountsSnapshot()[method, default: 0]
    }
}

/// 通用 mock 错误：用于 stub 未配置时的 sentinel 错误。
public enum MockError: Error, Equatable {
    case notStubbed
    case unexpectedCall(String)
}
