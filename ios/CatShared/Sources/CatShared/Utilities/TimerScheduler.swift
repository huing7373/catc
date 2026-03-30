import Foundation

/// Timer 调度抽象，支持测试时注入 mock
public protocol TimerScheduler: AnyObject {
    /// 创建一次性 Timer，返回可取消的 token
    @discardableResult
    func scheduleOnce(after interval: TimeInterval, action: @escaping () -> Void) -> TimerToken

    /// 创建重复 Timer，返回可取消的 token
    @discardableResult
    func scheduleRepeating(interval: TimeInterval, action: @escaping () -> Void) -> TimerToken
}

/// Timer 取消令牌
public protocol TimerToken: AnyObject {
    func cancel()
}

// MARK: - System Implementation

/// 真实 Timer 调度器
public final class SystemTimerScheduler: TimerScheduler {
    public init() {}

    public func scheduleOnce(after interval: TimeInterval, action: @escaping () -> Void) -> TimerToken {
        let timer = Timer.scheduledTimer(withTimeInterval: interval, repeats: false) { _ in action() }
        return SystemTimerToken(timer: timer)
    }

    public func scheduleRepeating(interval: TimeInterval, action: @escaping () -> Void) -> TimerToken {
        let timer = Timer.scheduledTimer(withTimeInterval: interval, repeats: true) { _ in action() }
        return SystemTimerToken(timer: timer)
    }
}

private final class SystemTimerToken: TimerToken {
    private let timer: Timer
    init(timer: Timer) { self.timer = timer }
    func cancel() { timer.invalidate() }
}
