// HealthProviderMock.swift
// Story 8.1 AC4: HealthProvider 的 in-memory 测试 mock；按 ADR-0002 §3.1 手写 invocation 记录.
//
// 用法：
//   let mock = HealthProviderMock()
//   mock.requestPermissionStub = .success(true)
//   mock.readDailyTotalStepsStub[Calendar.current.startOfDay(for: Date())] = 5000
//   let useCase = SyncStepsUseCase(healthProvider: mock, ...)
//
// 设计决策（详见 story 8-1-healthkit-接入.md AC4 段）:
// - 不继承 MockBase（class）：HealthProviderMock 走 production target（PetApp/Core/Health），
//   PetApp target 不能 @testable import test helper.
// - 位置在 PetApp/Core/Health/ 而非 PetAppTests/：架构 §17.1 钦定 mock 在 production target
//   让 Preview / DevTools / 集成测试都能消费.
// - stub 表 + error 互斥优先：readDailyTotalStepsError 优先（模拟"权限拒绝时永远抛错"场景）；
//   表查询为缺省路径.
// - @unchecked Sendable：字段 mutable 但测试串行调用，不会真竞态.

import Foundation

/// HealthProvider 的 in-memory 测试 mock；按 ADR-0002 §3.1 手写 invocation 记录.
public final class HealthProviderMock: HealthProvider, @unchecked Sendable {
    /// requestPermission 返回值 stub.
    public var requestPermissionStub: Result<Bool, Error> = .success(true)

    /// readDailyTotalSteps 按 dayStart（startOfDay 后的 Date）查表；缺省 0.
    /// key = Calendar.current.startOfDay(for: requestDate).
    public var readDailyTotalStepsStub: [Date: Int] = [:]

    /// readDailyTotalSteps 单独 stub 错误（优先于 readDailyTotalStepsStub 表）.
    public var readDailyTotalStepsError: Error?

    /// 调用历史；按 ADR-0002 §3.1 钦定的"至少记录 invocations"模板.
    public private(set) var invocations: [String] = []

    /// 调用次数（独立计数；测试断言"被调 N 次"用）.
    public private(set) var requestPermissionCallCount: Int = 0
    public private(set) var readDailyTotalStepsCallCount: Int = 0

    public init() {}

    public func requestPermission() async throws -> Bool {
        invocations.append("requestPermission()")
        requestPermissionCallCount += 1
        switch requestPermissionStub {
        case .success(let v): return v
        case .failure(let e): throw e
        }
    }

    public func readDailyTotalSteps(date: Date) async throws -> Int {
        let dayStart = Calendar.current.startOfDay(for: date)
        invocations.append("readDailyTotalSteps(date: \(dayStart.timeIntervalSince1970))")
        readDailyTotalStepsCallCount += 1
        if let error = readDailyTotalStepsError {
            throw error
        }
        return readDailyTotalStepsStub[dayStart] ?? 0
    }

    /// 重置全部 stub + 调用历史（测试 setUp / tearDown 用）.
    public func reset() {
        requestPermissionStub = .success(true)
        readDailyTotalStepsStub = [:]
        readDailyTotalStepsError = nil
        invocations = []
        requestPermissionCallCount = 0
        readDailyTotalStepsCallCount = 0
    }
}
