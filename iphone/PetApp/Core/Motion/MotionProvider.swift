// MotionProvider.swift
// Story 8.2 AC1: CoreMotion 运动状态识别的抽象边界（System Adapter 层）.
//
// 业务层（HomeViewModel / Story 8.4 + Story 8.3 MotionStateMapper）只依赖此协议；
// 测试用 MotionProviderMock 替换；生产用 MotionProviderImpl 真接入 CMMotionActivityManager.
//
// 设计基线（详见 story 8-2-coremotion-接入.md AC1 段）:
// - 协议层暴露 CMMotionActivity 原始类型（Story 8.3 mapper 直接吃 CMMotionActivity）
//   - 不预先枚举化（避免 8.3 mapper 二次翻译）
// - 错误三态：activityDataNotAvailable / permissionDenied / systemFailure(NSError)
//   - 与 HealthProviderError 同精神，不细分超时 / 其他 errorDomain
// - requestPermission 是 async throws；startUpdates / stopUpdates 同步签名
//   - CMMotionActivityManager.startActivityUpdates 本身就是同步注册回调（非 await 一次拿值）
// - Sendable 必标 + handler @Sendable（Swift 6 strict concurrency 跨 actor 调 protocol 必须）
// - 不暴露 AsyncStream / AnyPublisher（保持 callback 风格；上层 Story 8.4 自行 wrap @Published）
//
// import 备注（继承 Story 2.2 lesson 2026-04-25-swift-explicit-import-combine.md）：
// 仅 import Foundation + CoreMotion；不引 Combine（async/await 主流，ADR-0002 §3.2）.

import Foundation
import CoreMotion  // 协议层暴露 CMMotionActivity 类型——Story 8.3 mapper 直接吃此类型

/// MotionProvider 权限状态枚举——与 `CMAuthorizationStatus` 1:1 映射，但不让协议层耦合 CoreMotion 类型.
///
/// 设计理由（Story 8.4 review round 1 P1 修复）：
/// - 让 HomeViewModel.bind(motionProvider:) 能在调 `startUpdates` 前**纯查询**当前权限状态——
///   `.authorized` 才发起订阅；`.notDetermined` / `.denied` / `.restricted` 时只持引用不订阅,
///   避免 first launch UX 异常（`startActivityUpdates` 本身在 `.notDetermined` 下会触发系统权限弹窗,
///   破坏"权限申请由 8.5 / 8.6 同步触发器统一处理"的红线）.
/// - 用自定义 enum 而非直接暴露 `CMAuthorizationStatus`：协议层零 CoreMotion 依赖,
///   未来更换 motion 后端（如 SensorKit / 自研 fallback）不需要全链路改 protocol;
///   `MotionProviderImpl` 内部 bridge `CMMotionActivityManager.authorizationStatus()` → 本 enum.
/// - 与 `MotionProviderError` 的 `.permissionDenied` 不重复：error 是"操作失败的原因",
///   status 是"当前授权快照"——两者分别承担"事后失败分类"与"事前查询"职责.
public enum MotionAuthorizationStatus: Equatable {
    /// 用户尚未对此 App 做出 motion 权限决定（首次启动 / 重置授权后）.
    case notDetermined
    /// 用户授权 App 读取运动状态识别数据.
    case authorized
    /// 用户拒绝 App 读取运动状态识别数据.
    case denied
    /// 系统级别 restriction（如 Screen Time / MDM）禁止 App 读取——用户无法自主授予.
    case restricted
}

/// CoreMotion 运动状态识别的抽象边界（System Adapter 层）.
/// 业务层（HomeViewModel / Story 8.4 + Story 8.3 MotionStateMapper）只依赖此协议；
/// 测试用 MotionProviderMock 替换；生产用 MotionProviderImpl 真接入 CMMotionActivityManager.
public protocol MotionProvider: Sendable {
    /// 当前 motion 权限状态——**纯查询，不发起任何系统调用 / 不触发权限弹窗**.
    ///
    /// Story 8.4 review round 1 P1 引入：
    /// HomeViewModel.bind(motionProvider:) 在 .onAppear（first paint 之前）被调；
    /// 如果直接调 `startUpdates`，CoreMotion 在 `.notDetermined` 下会触发系统权限弹窗——
    /// 破坏 Story 8.4 红线"权限按需 by 8.5 / 8.6 统一处理". 加此查询入口让 bind 可 gate：
    ///   - `.authorized` → 调 startUpdates 正常订阅
    ///   - 其他三态 → 仅持 motionProvider 引用 return，不订阅，不弹权限
    ///
    /// 实装契约（详见 MotionProviderImpl）：
    /// - 必须**同步**调用（async/throws 都不允许）——bind 在 SwiftUI .onAppear（同步上下文）调.
    /// - **禁止**在内部触发任何 OS 弹窗 / IO；仅读 CMMotionActivityManager.authorizationStatus() 类的
    ///   "本地静态属性"或等价快照.
    /// - 调用次数语义无副作用（idempotent；可被 caller 反复查询）.
    func authorizationStatus() -> MotionAuthorizationStatus

    /// 申请运动状态识别权限（CMMotionActivity）.
    /// - Returns: true 表示用户授权 / 已授权；false 表示用户拒绝 / 受限.
    /// - Throws: `MotionProviderError.activityDataNotAvailable` 当设备不支持 activity 识别
    ///           （`CMMotionActivityManager.isActivityAvailable() == false`，如部分模拟器机型 / iPad）;
    ///           其他 OS 错误 wrap 为 `.systemFailure(underlying:)`.
    /// - Note: CoreMotion 的权限语义和 HealthKit 不同——CMAuthorizationStatus 可被信赖
    ///         （`CMMotionActivityManager.authorizationStatus()` 在 iOS 11+ 公开 API），
    ///         不需要像 8.1 HealthProvider 那样做 probe-read 兜底.
    func requestPermission() async throws -> Bool

    /// 开始订阅运动状态事件流；handler 在每次系统判定 activity 变化时被调用.
    /// - Parameter handler: 接收 `CMMotionActivity`（带 confidence + walking/running/stationary 等 flag）
    ///                      的 closure；CMMotionActivityManager 的 callback 在 main queue 触发
    ///                      （由实装内 `OperationQueue.main` 钦定，详见 AC2）.
    /// - 实装契约（必须严格遵守，详见 AC2 实装坑表）:
    ///   1. **同时多次 startUpdates 只生效第一次**：第二次起的 startUpdates 调用直接被忽略（**不**抛错、**不**替换 handler）.
    ///      epics.md AC 钦定"防止重复订阅"——这是为了避免 caller（Story 8.4 HomeViewModel）二次 bind 时同时收到双倍事件.
    ///   2. **handler 必须 @Sendable**：跨 actor 调用需保证.
    ///   3. **handler 持有 closure（escaping）**：本协议不规定 weak/strong；调用方决定 retain 周期.
    ///   4. **未授权 + activityAvailable 但 startUpdates 时**：**禁止**抛错；handler 不被调用即可（与系统行为一致——
    ///      未授权时 CMMotionActivityManager 默默不发事件，由 Story 8.5 / 8.4 等 caller 配合超时策略决策 fallback）.
    func startUpdates(handler: @escaping @Sendable (CMMotionActivity) -> Void)

    /// 停止订阅运动状态事件流.
    /// - 幂等：未 startUpdates 时调 stopUpdates 不抛错也不破坏后续 startUpdates 调用（次数计数清零）.
    /// - 调用 stopUpdates 后再次 startUpdates 视作"全新订阅"——handler 替换为新 closure，事件流重新开启.
    func stopUpdates()
}

/// MotionProvider 错误集合（Swift Error + LocalizedError，errorDescription 给 ErrorPresenter / DEBUG 日志用）.
public enum MotionProviderError: Error, Equatable, LocalizedError {
    /// 设备 / 模拟器不支持 activity 识别（`CMMotionActivityManager.isActivityAvailable() == false`）.
    case activityDataNotAvailable
    /// 用户拒绝 / 受限 activity 识别权限.
    /// 与 .activityDataNotAvailable 区分：前者是设备能力缺失，后者是用户授权拒绝.
    case permissionDenied
    /// 其他系统错误（NSError code 非上述两类）；保留 underlying 给日志 / dev 调试.
    case systemFailure(underlying: NSError)

    public static func == (lhs: MotionProviderError, rhs: MotionProviderError) -> Bool {
        switch (lhs, rhs) {
        case (.activityDataNotAvailable, .activityDataNotAvailable): return true
        case (.permissionDenied, .permissionDenied): return true
        case (.systemFailure(let l), .systemFailure(let r)):
            return l.domain == r.domain && l.code == r.code
        default: return false
        }
    }

    public var errorDescription: String? {
        switch self {
        case .activityDataNotAvailable: return "当前设备不支持运动状态识别"
        case .permissionDenied: return "请在系统设置中允许 PetApp 访问运动与健身数据"
        case .systemFailure: return "运动状态识别失败，请稍后重试"
        }
    }
}
