// HealthKitDevSeedUseCase.swift
// Story 8.1 AC7: 仅 DEBUG / UITest 路径用——把 N 步预置到 HKHealthStore 当日.
//
// 不挂 production code 路径（PetAppApp.swift 仅在 ProcessInfo.arguments 含 "-PetAppPreseedHealthKitSteps"
// 时调）；不在 Release build 编译，避免 binary 体积增加 + 误触发风险.
//
// 实现思路（Apple HKHealthStore.save API）：
// 1. 检查 isHealthDataAvailable（模拟器 / 真机）
// 2. requestAuthorization toShare: [stepCountType], read: [stepCountType]（模拟器自动授予）
// 3. 构造 HKQuantitySample（quantity = N steps，end = now - 60s, start = end - 1min；
//    "60 秒前"窗口确保永远落在过去，与读端 endDate clamp(min(now, dayEnd)) 配套——
//    避免 11:00 之前启动时 future-dated sample 被读端 clamp 排除导致 preseed flag 失效）
// 4. healthStore.save(...) 异步桥接.

#if DEBUG
import Foundation
import HealthKit

/// 仅 DEBUG / UITest 用：把 N 步预置到 HKHealthStore 当日.
public enum HealthKitDevSeedUseCase {
    /// 把 N 步预置到 HKHealthStore 当日（窗口 = [now-120s, now-60s]，确保落在过去且与读端 endDate clamp 兼容）.
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

        // 构造一个 HKQuantitySample，end = now - 60s（"60 秒前"），start = end - 60s.
        //
        // ⚠️ 必须落在过去——HealthProviderImpl.readDailyTotalSteps 把当日 endDate clamp 到 min(now, dayEnd)
        // （codex r4 [P2] fix），任何 future-timestamp sample 会被排除。原 11:00 固定时戳在早晨（11:00 前）
        // 启动时是 future，导致 preseed flag 失效（probe/non-probe 都读 0）—— codex r5 [P2] fix.
        //
        // 边界承认：刚跨过午夜的极短窗口（00:00:00 ~ 00:01:00）下，now-60 会落在前一天，sample 会归到
        // 前一天而非今天。UITest 在凌晨 0 点附近运行属极罕见场景，且 probe view 读"今天"仍可能为 0——
        // 此 trade-off 接受，理由是用 "60 秒前固定 offset" 比 magic clock-hour 常数更鲁棒.
        let now = Date()
        let sampleEnd = now.addingTimeInterval(-60)
        let sampleStart = sampleEnd.addingTimeInterval(-60)

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
