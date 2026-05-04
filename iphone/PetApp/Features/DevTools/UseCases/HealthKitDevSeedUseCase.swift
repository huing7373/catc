// HealthKitDevSeedUseCase.swift
// Story 8.1 AC7: 仅 DEBUG / UITest 路径用——把 N 步预置到 HKHealthStore 当日.
//
// 不挂 production code 路径（PetAppApp.swift 仅在 ProcessInfo.arguments 含 "-PetAppPreseedHealthKitSteps"
// 时调）；不在 Release build 编译，避免 binary 体积增加 + 误触发风险.
//
// 实现思路（Apple HKHealthStore.save API）：
// 1. 检查 isHealthDataAvailable（模拟器 / 真机）
// 2. requestAuthorization toShare: [stepCountType], read: [stepCountType]（模拟器自动授予）
// 3. 构造 HKQuantitySample（quantity = N steps，start = today 11:00, end = today 11:01；
//    fixed time 避开模拟器在午夜附近的边界 race）
// 4. healthStore.save(...) 异步桥接.

#if DEBUG
import Foundation
import HealthKit

/// 仅 DEBUG / UITest 用：把 N 步预置到 HKHealthStore 当日.
public enum HealthKitDevSeedUseCase {
    /// 把 N 步预置到 HKHealthStore 当日（本地时区 11:00～11:01 区间）.
    /// - Parameter steps: 期望预置的步数（必须 > 0）
    /// - Throws: HKHealthStore 操作失败 / authorization 失败 / 设备不支持时抛错；调用方决定是否吞错继续 boot.
    /// - Note: 本方法是幂等且 destructive—— 多次调同一日会累加 sample（不删旧 sample；测试场景下用单次 launch
    ///         路径调用即可，UITest 之间通过新模拟器或 simctl reset 清环境）.
    public static func preseedToday(steps: Int) async throws {
        guard steps > 0 else { return }
        guard HKHealthStore.isHealthDataAvailable() else {
            throw NSError(
                domain: "HealthKitDevSeedUseCase",
                code: -1,
                userInfo: [NSLocalizedDescriptionKey: "isHealthDataAvailable == false"]
            )
        }

        let healthStore = HKHealthStore()
        let stepCountType = HKQuantityType(.stepCount)

        // 申请 share 权限（模拟器自动授予；真机会弹窗，但本 UseCase 仅 DEBUG 路径）
        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
            healthStore.requestAuthorization(
                toShare: [stepCountType],
                read: [stepCountType]
            ) { _, error in
                if let error = error {
                    continuation.resume(throwing: error)
                } else {
                    continuation.resume()
                }
            }
        }

        // 构造一个 HKQuantitySample（11:00～11:01，避开 0 点边界）
        let calendar = Calendar.current
        let dayStart = calendar.startOfDay(for: Date())
        let sampleStart = calendar.date(byAdding: .hour, value: 11, to: dayStart) ?? dayStart
        let sampleEnd = calendar.date(byAdding: .minute, value: 1, to: sampleStart) ?? sampleStart

        let quantity = HKQuantity(unit: .count(), doubleValue: Double(steps))
        let sample = HKQuantitySample(
            type: stepCountType,
            quantity: quantity,
            start: sampleStart,
            end: sampleEnd
        )

        try await withCheckedThrowingContinuation { (continuation: CheckedContinuation<Void, Error>) in
            healthStore.save(sample) { _, error in
                if let error = error {
                    continuation.resume(throwing: error)
                } else {
                    continuation.resume()
                }
            }
        }
    }
}
#endif
