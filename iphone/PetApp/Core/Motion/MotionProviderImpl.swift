// MotionProviderImpl.swift
// Story 8.2 AC2: MotionProvider 的生产实装：基于 CMMotionActivityManager.startActivityUpdates 订阅运动状态变化.
//
// 设计要点（详见 story 8-2-coremotion-接入.md AC2 / Dev Notes "CoreMotion 接入坑表"）：
// - 使用 OperationQueue.main 接收 callback（与 SwiftUI / @Published 主线程更新天然对齐）.
// - 同时多次 startUpdates 只生效第一次（NSLock + isUpdating 旗标；epics.md AC 钦定）.
// - stopUpdates 后再 startUpdates 视作全新订阅（NSLock 内重置 isUpdating + 替换 handler）.
// - requestPermission 用 `CMMotionActivityManager.authorizationStatus()` (iOS 11+) 直接判定;
//   如 .notDetermined → queryActivityStarting 触发系统弹窗（探针式）一次后再次读 status.
// - 错误映射严格对齐 MotionProviderError 三态.
//
// 重要（与 Story 8.1 round 1 lesson 同精神）:
// NSLock 区间内**禁止** await——任何 lock.lock()/lock.unlock() 必须在同一同步段内闭合，
// 防 Swift 6 strict concurrency error（actor reentrancy + lock cross-suspension）.
//
// 重要（review round 3 P2 — stop/callback race full close）:
// CoreMotion handler 在 OperationQueue.main 上 dispatch 后，必须在 NSLock 持有期间**同步**调用 user handler；
// 否则 "check generation in lock → unlock → invoke" 之间存在间隙，stopUpdates 在此间隙跑完会让一个本应被丢弃的
// callback 仍 invoke user handler（违反"stopUpdates 返回后保证不再 fire"的契约）.
//
// 约束：user handler **禁止** 在其闭包内再调 startUpdates() / stopUpdates()——NSLock 不可重入，会死锁.
// 调用方需保证 handler 是轻量同步操作（写 @Published / @State / array.append 等），不在 handler 内做长耗时 IO 或锁等待.
// 预期合规调用点（Story 8.4+）：
//  - HomeViewModel.handleMotionUpdate（仅 store latest activity 到 @Published）
//  - debug probe view（仅 print）.

import Foundation
import CoreMotion  // 必须 import；framework 由 project.yml 显式声明（AC3）

/// MotionProvider 的生产实装：基于 CMMotionActivityManager.startActivityUpdates 订阅运动状态变化.
public final class MotionProviderImpl: MotionProvider, @unchecked Sendable {
    private let manager = CMMotionActivityManager()

    /// 同时多次 startUpdates 防御 + handler 替换 + stopUpdates 后清空状态——全用 NSLock 保护.
    private let lock = NSLock()
    private var isUpdating: Bool = false
    private var currentHandler: (@Sendable (CMMotionActivity) -> Void)?

    /// "代际"标记 / generation token——防 stop/restart race 时 stale callback 串到新订阅.
    ///
    /// 场景（review round 1 P2）：stopUpdates() 调完 manager.stopActivityUpdates() 后，
    /// CoreMotion 在调用前已经 enqueue 到 OperationQueue.main 的 callback **未必**取消执行——
    /// 这些 stale callback 跑到我们注入的 closure 里时，如果紧接着 startUpdates() 又设了新 handler，
    /// 它们会读到 fresh handler ref 并把"上一代"事件 forward 给新订阅者. Story 8.4 lifecycle rebind
    /// （HomeViewModel onAppear/onDisappear）会暴露此 race.
    ///
    /// 修法：每次 startUpdates 自增 generation 并把当前 generation 闭包到 callback 里；
    /// callback invoke 前 lock 内 check generation 一致才 forward. stopUpdates 也自增 generation
    /// 让任何已 enqueue 的"上一代"callback 全部失效（generation 不一致 → 直接丢弃）.
    private var generation: UInt64 = 0

    public init() {}

    /// Story 8.4 review round 1 P1 修复：纯查询当前权限状态，不触发任何系统调用.
    /// bridge `CMMotionActivityManager.authorizationStatus()` (iOS 11+ class func) → MotionAuthorizationStatus.
    ///
    /// 关键：本方法**不调** startActivityUpdates / queryActivityStarting，绝不触发系统权限弹窗——
    /// `CMMotionActivityManager.authorizationStatus()` 是 lightweight class-method 静态属性查询，
    /// 在 simulator / device 上读 sandbox 内权限缓存即返回，无 IO 无弹窗.
    /// HomeViewModel.bind(motionProvider:) 在 first paint 前调此 API gate：仅 `.authorized` 走
    /// startUpdates，其他三态保留引用不订阅，让 first launch UX 干净（不在 launch 路径弹权限）.
    public func authorizationStatus() -> MotionAuthorizationStatus {
        switch CMMotionActivityManager.authorizationStatus() {
        case .authorized: return .authorized
        case .denied: return .denied
        case .restricted: return .restricted
        case .notDetermined: return .notDetermined
        @unknown default:
            // future iOS 引入新 case 时保守视作未授权——与 requestPermission 内 @unknown default 同精神.
            return .notDetermined
        }
    }

    public func requestPermission() async throws -> Bool {
        guard CMMotionActivityManager.isActivityAvailable() else {
            throw MotionProviderError.activityDataNotAvailable
        }

        let status = CMMotionActivityManager.authorizationStatus()
        switch status {
        case .authorized:
            return true
        case .denied, .restricted:
            return false
        case .notDetermined:
            // 触发系统弹窗：用 queryActivityStarting 做一个极短探针（now-1s ~ now），
            // 系统会弹出权限弹窗 / iOS 13+ 自动拒绝（受隐私设置）；查询完成后再读一次 authorizationStatus.
            // 注意：如果直接调 startActivityUpdates 也会触发弹窗，但 startActivityUpdates 把 handler 注册了
            // 就改不掉了——此处仅为"探针式触发权限"，必须用 queryActivityStarting 让回调结束后即可释放.
            return try await probePermissionViaQuery()
        @unknown default:
            // future iOS 引入新 case 时保守视作未授权；不抛错（避免上游误以为 systemFailure）.
            return false
        }
    }

    public func startUpdates(handler: @escaping @Sendable (CMMotionActivity) -> Void) {
        lock.lock()
        // epics.md AC 钦定："同时多次 startUpdates → 只生效第一次"——
        // 已 isUpdating 时直接 return，不替换 handler、不抛错、不打 log（避免日志泛滥）.
        guard !isUpdating else {
            lock.unlock()
            return
        }
        isUpdating = true
        currentHandler = handler
        // 每次 startUpdates 自增 generation；callback 闭包捕获本次 myGen，invoke 前 check 不变才 forward.
        generation &+= 1
        let myGen = generation
        lock.unlock()

        // 注意：CMMotionActivityManager.startActivityUpdates 必须在 main thread 调；
        // OperationQueue.main 让 callback 也在 main，与 SwiftUI 状态更新对齐.
        manager.startActivityUpdates(to: OperationQueue.main) { [weak self] activity in
            guard let self, let activity else { return }
            // lock 内 check generation 一致 + **同步** invoke user handler——防 stop/restart race（review round 3 P2）.
            //
            // 关键：generation check 与 user handler invoke **必须**在同一 lock 持有段内完成——
            // 如果 unlock 后再 invoke，stopUpdates 可能在 unlock 与 invoke 之间跑完，导致 "stopUpdates 返回后
            // 仍有一个 callback 抵达 user handler" 的契约违反.
            //
            // 代价：user handler 阻塞 lock 期间所有其他 start/stop——但 handler 文档约束为轻量同步操作
            // （Story 8.4 HomeViewModel store @Published；debug probe 仅 print），sub-microsecond 级别可接受.
            //
            // 死锁规避：user handler **禁止**在闭包内调 startUpdates() / stopUpdates()（NSLock 不可重入）.
            self.lock.lock()
            defer { self.lock.unlock() }
            guard self.generation == myGen, let captured = self.currentHandler else {
                return
            }
            captured(activity)
        }
    }

    public func stopUpdates() {
        lock.lock()
        // 幂等：未 isUpdating 时 stopUpdates 不抛错也不调 manager.stopActivityUpdates
        // （Apple 文档不保证未启动时调 stop 的安全；保守只在 isUpdating 时才调）.
        guard isUpdating else {
            lock.unlock()
            return
        }
        isUpdating = false
        currentHandler = nil
        // generation 自增——让任何已 enqueue 但还没 invoke 的"上一代"callback 在 generation check 时被丢弃.
        // 关键：哪怕 stopUpdates 后立即 startUpdates，stale callback 也不会串到新订阅
        // （新 startUpdates 会再次自增 generation，stale callback 闭包里的 myGen 仍对应"两代之前"，永远不 match）.
        generation &+= 1
        lock.unlock()

        manager.stopActivityUpdates()
    }

    /// 探针：发一个极短窗口的 queryActivityStarting 触发权限弹窗（系统首次会弹），
    /// 完成后再读一次 authorizationStatus 判定真实结果.
    private func probePermissionViaQuery() async throws -> Bool {
        let now = Date()
        let oneSecondAgo = now.addingTimeInterval(-1)
        return try await withCheckedThrowingContinuation { continuation in
            manager.queryActivityStarting(from: oneSecondAgo, to: now, to: OperationQueue.main) { _, error in
                if let nsError = error as NSError? {
                    if nsError.domain == CMErrorDomain,
                       nsError.code == CMErrorMotionActivityNotAuthorized.rawValue {
                        continuation.resume(returning: false)
                        return
                    }
                    continuation.resume(throwing: MotionProviderError.systemFailure(underlying: nsError))
                    return
                }
                // 弹窗结束后再读 status——authorized 才返 true；其他 case 全返 false.
                let final = CMMotionActivityManager.authorizationStatus()
                continuation.resume(returning: final == .authorized)
            }
        }
    }
}
