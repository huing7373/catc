// HealthProvider.swift
// Story 8.1 AC1: HealthKit 步数读取的抽象边界（System Adapter 层）.
//
// 业务层（SyncStepsUseCase / StepSyncTriggerService，Story 8.5 落地）只依赖此协议；
// 测试用 HealthProviderMock 替换；生产用 HealthProviderImpl 真接入 HKHealthStore.
//
// 设计基线（详见 story 8-1-healthkit-接入.md AC1 段）:
// - 不暴露 HealthKit 具体类型（HKHealthStore / HKQuantityType / NSError）泄漏到调用方
// - 错误三态：healthDataNotAvailable / permissionDenied / systemFailure(NSError)
// - 用 async throws 主流（与 Story 5.5 LoadHomeUseCase / Story 4.6 GuestLoginUseCase 同 ADR-0002 §3.2 钦定）
// - Sendable 必标（Swift 6 strict concurrency 跨 actor 调 protocol 必须）

import Foundation

/// HealthKit 步数读取的抽象边界（System Adapter 层）.
/// 业务层只依赖此协议；测试用 HealthProviderMock 替换；生产用 HealthProviderImpl 真接入 HKHealthStore.
public protocol HealthProvider: Sendable {
    /// 申请步数读取权限（HKQuantityType.stepCount）.
    /// - Returns: true 表示用户授权 / 已授权；false 表示用户拒绝.
    /// - Throws: `HealthProviderError.healthDataNotAvailable` 当设备不支持 HealthKit
    ///           （iPad 在某些版本 / 模拟器某些机型）；其他 OS 错误 wrap 为 `.systemFailure(underlying:)`.
    /// - Note: HealthKit read 隐私契约——`HKHealthStore.requestAuthorization` 即使用户拒绝也返
    ///         `success == true`，`authorizationStatus(for:)` 对 read 类型亦故意返 `.sharingDenied`
    ///         防外部探测；故 HealthProviderImpl 实装在 requestAuthorization 完成后**额外做一次
    ///         probe-read**（试读当日步数）：HK 抛 `errorAuthorizationDenied` → 返 false；
    ///         任何整数（含 0）成功返回 → 返 true. 0 在已授权但当日无 sample 时合法，
    ///         与 read deny 数值不可区分，但 HK 钦定 deny 走"抛错"而非"返 0"，所以可靠.
    ///         详见 docs/lessons/2026-05-04-healthkit-read-auth-must-probe-read.md.
    func requestPermission() async throws -> Bool

    /// 读指定日期（按本机本地时区起止）的累计步数总和.
    /// - Parameter date: 任意日期（实装内取该日期所在的"本地时区当日 00:00 → 24:00"区间）.
    /// - Returns: 累计步数（非负整数；HKStatisticsQuery sumQuantity → Int 转换；为 nil 时返 0）.
    /// - Throws: `HealthProviderError.permissionDenied` 当 HK 查询返回 `errorAuthorizationDenied`；
    ///           `.healthDataNotAvailable` 当 `HKHealthStore.isHealthDataAvailable() == false`；
    ///           `.systemFailure(underlying:)` 包装其他 NSError.
    func readDailyTotalSteps(date: Date) async throws -> Int
}

/// HealthProvider 错误集合（Swift Error + LocalizedError，errorDescription 给上层 ErrorPresenter 用）.
public enum HealthProviderError: Error, Equatable, LocalizedError {
    /// 设备 / 模拟器不支持 HealthKit（如某些 iPad；模拟器在 Xcode 26 默认支持但某些 runtime 缺数据）.
    case healthDataNotAvailable
    /// 用户在系统弹窗拒绝步数读取权限（或 HK 查询返回 errorAuthorizationDenied）.
    case permissionDenied
    /// 其他系统错误（NSError code 非上述两类）；保留 underlying 给日志 / dev 调试.
    case systemFailure(underlying: NSError)

    public static func == (lhs: HealthProviderError, rhs: HealthProviderError) -> Bool {
        switch (lhs, rhs) {
        case (.healthDataNotAvailable, .healthDataNotAvailable): return true
        case (.permissionDenied, .permissionDenied): return true
        case (.systemFailure(let l), .systemFailure(let r)):
            return l.domain == r.domain && l.code == r.code
        default: return false
        }
    }

    public var errorDescription: String? {
        switch self {
        case .healthDataNotAvailable: return "当前设备不支持步数读取"
        case .permissionDenied: return "请在系统设置中允许 PetApp 读取步数"
        case .systemFailure: return "步数读取失败，请稍后重试"
        }
    }
}
