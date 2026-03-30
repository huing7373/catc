import Foundation
@testable import CatShared

/// 可控时间提供者，用于测试时间相关逻辑
final class MockTimeProvider: TimeProvider, @unchecked Sendable {
    var now: Date

    init(date: Date = Date()) {
        self.now = date
    }

    func advance(by seconds: TimeInterval) {
        now = now.addingTimeInterval(seconds)
    }

    /// 设置到指定小时（当天）
    func setHour(_ hour: Int, minute: Int = 0) {
        var calendar = Calendar.current
        calendar.timeZone = .current
        var components = calendar.dateComponents([.year, .month, .day], from: now)
        components.hour = hour
        components.minute = minute
        components.second = 0
        now = calendar.date(from: components)!
    }
}
