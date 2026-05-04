// HealthProviderImpl.swift
// Story 8.1 AC2: HealthProvider 的生产实装：基于 HKHealthStore + HKStatisticsQuery 读当日累计步数.
//
// 设计要点（详见 story 8-1-healthkit-接入.md AC2 / Dev Notes "HealthKit 接入坑表"）:
// - 用 HKStatisticsQuery + .cumulativeSum 而非 HKSampleQuery + 手动累加（前者 Apple 钦定 efficient）
// - 时区按 Calendar.current（本机本地时区）切日；与 V1 §6.1 钦定 syncDate 时区契约对齐
// - **每次 read 都重新查询 HK**——不缓存（codex r1 [P2] fix）；HKStatisticsQuery 本身廉价，
//   而"同日读两次走 cache"会让用户当天后续累计步数永远拿不到，是更严重的正确性 bug
//   （详见 docs/lessons/2026-05-04-debug-seed-vs-probe-await-coupling.md Lesson 2）.
// - 失败映射严格对齐 HealthProviderError 三态

import Foundation
import HealthKit  // 必须 import；framework 由 project.yml 显式声明（AC3）

/// HealthProvider 的生产实装：基于 HKHealthStore + HKStatisticsQuery 读当日累计步数.
public final class HealthProviderImpl: HealthProvider, @unchecked Sendable {
    private let healthStore = HKHealthStore()
    private let stepCountType = HKQuantityType(.stepCount)

    public init() {}

    public func requestPermission() async throws -> Bool {
        guard HKHealthStore.isHealthDataAvailable() else {
            throw HealthProviderError.healthDataNotAvailable
        }
        return try await withCheckedThrowingContinuation { continuation in
            // requestAuthorization 即使用户拒绝也返回 success=true（Apple 故意防探测）；
            // 真实拒绝在后续 readDailyTotalSteps 时通过 errorAuthorizationDenied 暴露.
            // 故此处仅检测 system error（如 HealthKit 服务不可达），不视 success=true 为"已授权".
            healthStore.requestAuthorization(toShare: [], read: [stepCountType]) { success, error in
                if let nsError = error as NSError? {
                    continuation.resume(throwing: HealthProviderError.systemFailure(underlying: nsError))
                } else {
                    continuation.resume(returning: success)
                }
            }
        }
    }

    public func readDailyTotalSteps(date: Date) async throws -> Int {
        guard HKHealthStore.isHealthDataAvailable() else {
            throw HealthProviderError.healthDataNotAvailable
        }

        // 本地时区当日 00:00 / 次日 00:00 区间.
        let calendar = Calendar.current
        let dayStart = calendar.startOfDay(for: date)
        guard let dayEnd = calendar.date(byAdding: .day, value: 1, to: dayStart) else {
            // 极不可能；保险走 systemFailure.
            throw HealthProviderError.systemFailure(
                underlying: NSError(domain: "HealthProviderImpl", code: -1, userInfo: nil)
            )
        }

        // 每次都重新 query HK（不缓存）——HK 本身廉价，缓存会让同日 sync 持续读到陈旧值.
        // 详见 docs/lessons/2026-05-04-debug-seed-vs-probe-await-coupling.md Lesson 2.
        let predicate = HKQuery.predicateForSamples(withStart: dayStart, end: dayEnd, options: .strictStartDate)
        let value: Int = try await withCheckedThrowingContinuation { continuation in
            let query = HKStatisticsQuery(
                quantityType: stepCountType,
                quantitySamplePredicate: predicate,
                options: .cumulativeSum
            ) { _, statistics, error in
                if let nsError = error as NSError? {
                    if nsError.domain == HKErrorDomain
                        && nsError.code == HKError.errorAuthorizationDenied.rawValue {
                        continuation.resume(throwing: HealthProviderError.permissionDenied)
                    } else {
                        continuation.resume(throwing: HealthProviderError.systemFailure(underlying: nsError))
                    }
                    return
                }
                let count: Int
                if let sum = statistics?.sumQuantity() {
                    count = Int(sum.doubleValue(for: .count()))
                } else {
                    count = 0
                }
                continuation.resume(returning: max(0, count))  // 防御负数（理论不会发生）
            }
            healthStore.execute(query)
        }

        return value
    }
}
