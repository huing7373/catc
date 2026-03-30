import Foundation

/// 时间抽象协议，支持测试时注入 mock
public protocol TimeProvider: Sendable {
    var now: Date { get }
}

/// 系统时间实现
public struct SystemTimeProvider: TimeProvider {
    public init() {}
    public var now: Date { Date() }
}
