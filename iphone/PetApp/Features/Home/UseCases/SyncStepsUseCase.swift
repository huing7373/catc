// SyncStepsUseCase.swift
// Story 8.5 AC3: 步数同步 UseCase（业务编排：healthProvider → repository → appState）.
//
// 职责（epics.md AC 行 1572-1576）:
//   1. 读 HealthProvider 当日累计步数（healthProvider.readDailyTotalSteps(date:)）
//   2. 拼请求体（StepsSyncRequest：syncDate / clientTotalSteps / motionState / clientTimestamp）
//   3. 调 repository.syncSteps(_:) 拿响应
//   4. 成功 → 调 appState.applySyncedStepAccount(_:) 写入 AppState.currentStepAccount
//      （addendum 钦定，**不**写 ViewModel）
//   5. 失败 → throw 透传给上层 TriggerService（TriggerService 决定不阻塞 UI / 下次再试）
//
// **不**做的事:
//   - 不接 ErrorPresenter（背景同步失败不弹 toast；与 epics.md AC 行 1576 "不阻塞 UI" 钦定一致）
//   - 不做 retry / 指数退避（TriggerService 层用"下次定时器到达"模式自然兜底；YAGNI）
//   - 不申请 HealthKit 权限（caller 决定是否走 healthProvider.requestPermission()；
//     未授权时 HealthProviderImpl 抛 .permissionDenied → 透传 → TriggerService log warning + 不更新）
//   - 不读 ViewModel.petState（与 8.4 视觉层解耦；motionState 由调用方注入）

import Foundation

public protocol SyncStepsUseCaseProtocol: Sendable {
    /// 执行一次步数同步（4 步流程：read → build → sync → apply）.
    /// - Parameter motionState: 当前活动状态（由 caller 提供）
    /// - Throws: HealthProviderError / APIError（原样透传给 caller）
    func execute(motionState: MotionState) async throws
}

public struct DefaultSyncStepsUseCase: SyncStepsUseCaseProtocol {
    private let healthProvider: HealthProvider
    private let repository: StepRepositoryProtocol
    private let appState: AppState
    private let dateProvider: DateProvider

    public init(
        healthProvider: HealthProvider,
        repository: StepRepositoryProtocol,
        appState: AppState,
        dateProvider: DateProvider
    ) {
        self.healthProvider = healthProvider
        self.repository = repository
        self.appState = appState
        self.dateProvider = dateProvider
    }

    public func execute(motionState: MotionState) async throws {
        // Step 1: 读 HealthKit 当日累计步数（按 dateProvider.now() 取本机时区当日）.
        // healthProvider 内部用 HKStatisticsQuery sumQuantity；与 8.1 协议契约一致.
        //
        // codex review round 1 [P2] fix：**只 capture 一次 `now`**，所有派生字段（syncDate /
        // clientTimestamp / readDailyTotalSteps 入参）从这同一个 Date 推导，避免跨午夜时
        // `now()` / `todayString()` / `nowMillis()` 各自独立 fetch 导致 day1 总数被标 day2 →
        // server 拒绝或错账（详见 docs/lessons/2026-05-04-cross-midnight-single-captured-date.md）.
        // **不**调 dateProvider.todayString() / nowMillis() —— 那两个方法内部仍用 `Date()`，
        // 与本 capture 的 now 可能跨过午夜.use case 自己用 Calendar / timeIntervalSince1970 推.
        let now = dateProvider.now()
        let totalSteps = try await healthProvider.readDailyTotalSteps(date: now)

        // Step 2: 拼请求体.
        // - syncDate: 本机时区当日 YYYY-MM-DD（DateProvider 钦定；不用 ISO8601 避免跨时区漂移）
        // - motionState: MotionState.wireValue（AC4.1 extension）→ 1 / 2 / 3
        // - clientTimestamp: Int64 毫秒（与 V1 §6.1.4 一致）
        // 全部从同一个 `now` 推导（见上方 race fix 注释）.
        let request = StepsSyncRequest(
            syncDate: Self.formatLocalDateString(from: now),
            clientTotalSteps: totalSteps,
            motionState: motionState.wireValue,
            clientTimestamp: Int64(now.timeIntervalSince1970 * 1000)
        )

        // Step 3: 调 server.
        // - 网络成功 + 业务 code=0 → 解 StepsSyncResponse；
        // - 网络失败 / 业务 code != 0 → 抛 APIError（透传给 TriggerService）
        let response = try await repository.syncSteps(request)

        // Step 4: 写 AppState（与 addendum 钦定一致；**不**写 ViewModel）.
        // 用 AppState 提供的 mutation 入口（AC7：applySyncedStepAccount(_:)）.
        // 转 wire DTO → domain HomeStepAccount（与 5.5 HomeData(from:) 同精神，单字段；
        // Story 21.x ChestOpen 后续也按此模式）.
        let domainAccount = HomeStepAccount(
            totalSteps: response.stepAccount.totalSteps,
            availableSteps: response.stepAccount.availableSteps,
            consumedSteps: response.stepAccount.consumedSteps
        )
        await MainActor.run {
            appState.applySyncedStepAccount(domainAccount)
        }
    }

    /// 把 captured `Date` 格式化成本机时区当日 "YYYY-MM-DD".
    ///
    /// codex review round 1 [P2] fix：**use case 不再调 dateProvider.todayString()**（那个方法内部
    /// 自己 `Date()` 重新取一次时间，与 use case 已 capture 的 `now` 可能跨过午夜）；改成 use case
    /// 自己用同一个 `now` Date 派生 dateString，保证 `syncDate` / `clientTimestamp` /
    /// `readDailyTotalSteps(date:)` 三处时间字段全部源自同一瞬间.
    ///
    /// 实现与 `DefaultDateProvider.todayString()` 完全一致（gregorian / en_US_POSIX / TimeZone.current）,
    /// 只是入参从隐式 `Date()` 改成显式 `from: Date`.
    private static func formatLocalDateString(from date: Date) -> String {
        let formatter = DateFormatter()
        formatter.calendar = Calendar(identifier: .gregorian)
        formatter.dateFormat = "yyyy-MM-dd"
        formatter.locale = Locale(identifier: "en_US_POSIX")
        formatter.timeZone = TimeZone.current
        return formatter.string(from: date)
    }
}
