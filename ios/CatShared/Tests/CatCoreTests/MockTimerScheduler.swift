import Foundation
@testable import CatShared

/// 可控 Timer 调度器，测试中手动触发 Timer
final class MockTimerScheduler: TimerScheduler {

    private(set) var scheduledTokens: [MockTimerToken] = []

    func scheduleOnce(after interval: TimeInterval, action: @escaping () -> Void) -> TimerToken {
        let token = MockTimerToken(interval: interval, repeats: false, action: action)
        scheduledTokens.append(token)
        return token
    }

    func scheduleRepeating(interval: TimeInterval, action: @escaping () -> Void) -> TimerToken {
        let token = MockTimerToken(interval: interval, repeats: true, action: action)
        scheduledTokens.append(token)
        return token
    }

    /// 手动触发所有未取消的 Timer
    func fireAll() {
        for token in scheduledTokens where !token.isCancelled {
            token.fire()
        }
    }

    /// 触发所有匹配指定间隔的 Timer
    func fire(interval: TimeInterval, tolerance: TimeInterval = 0.1) {
        for token in scheduledTokens where !token.isCancelled {
            if abs(token.interval - interval) < tolerance {
                token.fire()
            }
        }
    }

    /// 触发最后一个添加的 Timer
    func fireLast() {
        if let token = scheduledTokens.last, !token.isCancelled {
            token.fire()
        }
    }

    /// 清理已取消的 token
    func cleanup() {
        scheduledTokens.removeAll { $0.isCancelled }
    }

    /// 活跃（未取消）的 token 数量
    var activeCount: Int {
        scheduledTokens.filter { !$0.isCancelled }.count
    }
}

final class MockTimerToken: TimerToken {
    let interval: TimeInterval
    let repeats: Bool
    private let action: () -> Void
    private(set) var isCancelled = false
    private(set) var fireCount = 0

    init(interval: TimeInterval, repeats: Bool, action: @escaping () -> Void) {
        self.interval = interval
        self.repeats = repeats
        self.action = action
    }

    func cancel() {
        isCancelled = true
    }

    func fire() {
        guard !isCancelled else { return }
        fireCount += 1
        action()
        if !repeats {
            isCancelled = true
        }
    }
}
