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

        // Step 1: 走原始 HKHealthStore.requestAuthorization 完成系统弹窗（或模拟器自动授权）.
        //
        // ⚠️ Apple 钦定的隐私契约：requestAuthorization 即使用户拒绝 read 权限也返回 `success == true`，
        // `authorizationStatus(for:)` 对 read 类型同样故意返回 `.sharingDenied` 防外部探测真实授权.
        // 详见 https://developer.apple.com/documentation/healthkit/protecting_user_privacy
        // 因此 success 自身**不**是"已授权"信号——必须用 probe-read heuristic 推断（Step 2）.
        let _: Void = try await withCheckedThrowingContinuation { continuation in
            healthStore.requestAuthorization(toShare: [], read: [stepCountType]) { _, error in
                if let nsError = error as NSError? {
                    continuation.resume(throwing: HealthProviderError.systemFailure(underlying: nsError))
                } else {
                    continuation.resume(returning: ())
                }
            }
        }

        // Step 2: probe-read 当日步数 → 拿真实授权信号.
        //   - HK 抛 `errorAuthorizationDenied` → readDailyTotalSteps throw `.permissionDenied` → 返 false
        //   - 其他错误（systemFailure / healthDataNotAvailable）→ 视为真错往上抛
        //   - 成功（任意 Int 返回值，包括 0）→ 视为已授权 → 返 true
        //     （0 在已授权但当日无 sample 时合法；这与 read deny 在数值上无法区分，
        //     但 HK 在 read deny 时**抛错**而非返 0，所以可靠）
        do {
            _ = try await readDailyTotalSteps(date: Date())
            return true
        } catch HealthProviderError.permissionDenied {
            return false
        } catch {
            throw error
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
        //
        // ⚠️ 关于 `.strictStartDate` 单独使用而非 `[.strictStartDate, .strictEndDate]`（codex r3 [P2] defer）:
        // - 跨午夜 sample（罕见的 vendor batched 写入，如 23:59→00:01）会**整段归到起始日**，
        //   理论上前一日 overcount + 后一日 undercount.
        // - 但 codex 建议的 `[.strictStartDate, .strictEndDate]` 反而**完全丢弃**跨午夜 sample，更糟.
        // - Apple 钦定的 prorate 方案是 `HKStatisticsCollectionQuery` + daily anchored interval（按时间比例自动拆分），
        //   但那是 Story 8.3/8.5 步数同步业务级别的重写，超出本 story（probe-level read API）scope.
        // - HKQuantitySample stepCount 在实践中通常分钟级 sample 且不跨午夜；trade-off 接受.
        // - 详见 docs/lessons/2026-05-04-healthkit-strictstartdate-cross-midnight-tradeoff.md.
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
