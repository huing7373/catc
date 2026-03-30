import Foundation
import Combine
import CatShared

/// 猫状态机 — 手表端中央事件总线
/// 纯发布原则：只发布状态变化，不触发任何副作用
@Observable
public final class CatStateMachine {

    public static let shared = CatStateMachine()

    // MARK: - Published State

    public private(set) var currentState: CatState = .idle

    /// Combine Publisher 广播状态变化
    public var statePublisher: AnyPublisher<CatState, Never> {
        stateSubject.eraseToAnyPublisher()
    }

    // MARK: - Dependencies

    @ObservationIgnored
    private let timeProvider: TimeProvider

    @ObservationIgnored
    private let localStore: LocalStore

    @ObservationIgnored
    private let scheduler: TimerScheduler

    // MARK: - Internal State

    @ObservationIgnored
    private let stateSubject = PassthroughSubject<CatState, Never>()

    @ObservationIgnored
    private var pendingInput: MotionInput?
    @ObservationIgnored
    private var pendingInputStart: Date?
    @ObservationIgnored
    private var debounceToken: TimerToken?

    @ObservationIgnored
    private var idleStartTime: Date?

    @ObservationIgnored
    private var selfHealToken: TimerToken?
    @ObservationIgnored
    private var microYawnToken: TimerToken?
    @ObservationIgnored
    private var microStretchToken: TimerToken?
    @ObservationIgnored
    private var microReturnToken: TimerToken?
    @ObservationIgnored
    private var sleepCheckToken: TimerToken?

    // MARK: - Constants

    public enum Timing {
        public static let motionDebounce: TimeInterval = 3.0
        public static let stationaryDebounce: TimeInterval = 10.0
        public static let selfHealInterval: TimeInterval = 60.0
        public static let idleToSleepDuration: TimeInterval = 30 * 60
        public static let sleepCheckInterval: TimeInterval = 60.0
        public static let nightStartHour = 22
        public static let nightEndHour = 7
        public static let meanYawnInterval: TimeInterval = 5 * 60
        public static let meanStretchInterval: TimeInterval = 8 * 60
        public static let debounceCheckInterval: TimeInterval = 0.5
    }

    // MARK: - Init

    public init(timeProvider: TimeProvider = SystemTimeProvider(),
                localStore: LocalStore = LocalStore(),
                scheduler: TimerScheduler = SystemTimerScheduler()) {
        self.timeProvider = timeProvider
        self.localStore = localStore
        self.scheduler = scheduler
        restoreState()
    }

    // MARK: - Public API

    /// 传感器层唯一入口：传入运动输入，状态机内部处理防抖和转换规则
    public func handleMotionInput(_ input: MotionInput) {
        switch input {
        case .wristRaise:
            handleWristRaise()
        case .stationary:
            handleStationaryInput()
        case .walking:
            handleMotionDetected(.walking)
        case .running:
            handleMotionDetected(.running)
        }
    }

    // MARK: - Internal Transition (package-level, not public)

    /// 直接状态转换 — 仅限内部和测试使用，不对外暴露
    func transition(to newState: CatState) {
        guard newState != currentState else { return }

        // 微行为打断：主状态可打断微行为
        if currentState.isMicroBehavior && newState.isMainState {
            cancelMicroBehaviorReturn()
        }

        // 微行为只能从 idle 触发
        if newState.isMicroBehavior && currentState != .idle {
            return
        }

        performTransition(to: newState)
    }

    // MARK: - State Transition Engine

    private func performTransition(to newState: CatState) {
        let oldState = currentState
        currentState = newState

        cleanupTimers(for: oldState)
        setupTimers(for: newState)

        if newState.isMainState {
            localStore.saveCatState(newState)
        }

        stateSubject.send(newState)
        resetSelfHealTimer()
    }

    // MARK: - Input Handlers

    private func handleWristRaise() {
        if currentState == .sleeping {
            transition(to: .idle)
        }
    }

    private func handleStationaryInput() {
        guard currentState == .walking || currentState == .running else {
            cancelPendingInput()
            return
        }

        startDebounce(for: .stationary, requiredDuration: Timing.stationaryDebounce) { [weak self] in
            self?.transition(to: .idle)
        }
    }

    private func handleMotionDetected(_ targetMotion: MotionInput) {
        let targetState: CatState = targetMotion == .walking ? .walking : .running

        // walking ↔ running 直接转换（无防抖）
        if (currentState == .walking && targetState == .running) ||
           (currentState == .running && targetState == .walking) {
            cancelPendingInput()
            transition(to: targetState)
            return
        }

        guard currentState == .idle || currentState.isMicroBehavior else {
            cancelPendingInput()
            return
        }

        startDebounce(for: targetMotion, requiredDuration: Timing.motionDebounce) { [weak self] in
            self?.transition(to: targetState)
        }
    }

    // MARK: - Debounce

    private func startDebounce(for input: MotionInput, requiredDuration: TimeInterval, action: @escaping () -> Void) {
        if pendingInput == input, let start = pendingInputStart {
            if timeProvider.now.timeIntervalSince(start) >= requiredDuration {
                cancelPendingInput()
                action()
            }
            return
        }

        cancelPendingInput()
        pendingInput = input
        pendingInputStart = timeProvider.now

        debounceToken = scheduler.scheduleRepeating(interval: Timing.debounceCheckInterval) { [weak self] in
            guard let self, let start = self.pendingInputStart, self.pendingInput == input else { return }
            if self.timeProvider.now.timeIntervalSince(start) >= requiredDuration {
                self.cancelPendingInput()
                action()
            }
        }
    }

    private func cancelPendingInput() {
        pendingInput = nil
        pendingInputStart = nil
        debounceToken?.cancel()
        debounceToken = nil
    }

    // MARK: - Self-Heal Timer

    private func resetSelfHealTimer() {
        selfHealToken?.cancel()
        selfHealToken = scheduler.scheduleOnce(after: Timing.selfHealInterval) { [weak self] in
            guard let self, self.currentState != .idle else { return }
            self.transition(to: .idle)
        }
    }

    // MARK: - Sleeping Check

    private func startSleepCheck() {
        idleStartTime = timeProvider.now
        sleepCheckToken = scheduler.scheduleRepeating(interval: Timing.sleepCheckInterval) { [weak self] in
            self?.checkSleepCondition()
        }
    }

    private func checkSleepCondition() {
        guard currentState == .idle, let idleStart = idleStartTime else { return }
        let now = timeProvider.now
        let idleDuration = now.timeIntervalSince(idleStart)
        guard idleDuration >= Timing.idleToSleepDuration else { return }

        let calendar = Calendar.current
        let hour = calendar.component(.hour, from: now)
        let isNighttime = hour >= Timing.nightStartHour || hour < Timing.nightEndHour

        if isNighttime {
            transition(to: .sleeping)
        }
    }

    private func stopSleepCheck() {
        sleepCheckToken?.cancel()
        sleepCheckToken = nil
        idleStartTime = nil
    }

    // MARK: - Micro-Behavior Scheduler

    private func startMicroBehaviorScheduler() {
        scheduleMicroYawn()
        scheduleMicroStretch()
    }

    private func stopMicroBehaviorScheduler() {
        microYawnToken?.cancel()
        microYawnToken = nil
        microStretchToken?.cancel()
        microStretchToken = nil
        cancelMicroBehaviorReturn()
    }

    private func scheduleMicroYawn() {
        let delay = randomExponentialDelay(mean: Timing.meanYawnInterval)
        microYawnToken = scheduler.scheduleOnce(after: delay) { [weak self] in
            guard let self, self.currentState == .idle else {
                self?.scheduleMicroYawn()
                return
            }
            self.transition(to: .microYawn)
        }
    }

    private func scheduleMicroStretch() {
        let delay = randomExponentialDelay(mean: Timing.meanStretchInterval)
        microStretchToken = scheduler.scheduleOnce(after: delay) { [weak self] in
            guard let self, self.currentState == .idle else {
                self?.scheduleMicroStretch()
                return
            }
            self.transition(to: .microStretch)
        }
    }

    private func startMicroBehaviorReturn(for state: CatState) {
        let duration = state.microBehaviorDuration
        guard duration > 0 else { return }
        microReturnToken = scheduler.scheduleOnce(after: duration) { [weak self] in
            self?.transition(to: .idle)
        }
    }

    private func cancelMicroBehaviorReturn() {
        microReturnToken?.cancel()
        microReturnToken = nil
    }

    private func randomExponentialDelay(mean: TimeInterval) -> TimeInterval {
        let u = Double.random(in: 0.001...1.0)
        return -mean * log(u)
    }

    // MARK: - Timer Management

    private func cleanupTimers(for oldState: CatState) {
        switch oldState {
        case .idle:
            stopSleepCheck()
            stopMicroBehaviorScheduler()
        case .microYawn, .microStretch:
            cancelMicroBehaviorReturn()
        default:
            break
        }
        cancelPendingInput()
    }

    private func setupTimers(for newState: CatState) {
        switch newState {
        case .idle:
            startSleepCheck()
            startMicroBehaviorScheduler()
        case .microYawn, .microStretch:
            startMicroBehaviorReturn(for: newState)
        default:
            break
        }
    }

    // MARK: - State Persistence

    private func restoreState() {
        if let savedState = localStore.loadCatState(), savedState.isMainState {
            currentState = savedState
            setupTimers(for: savedState)
        } else {
            currentState = .idle
            setupTimers(for: .idle)
        }
        resetSelfHealTimer()
    }
}
