import Foundation

/// 猫的 6 种状态：4 个主状态 + 2 个微行为
public enum CatState: String, Codable, CaseIterable, Sendable {
    case idle
    case walking
    case running
    case sleeping
    case microYawn
    case microStretch

    /// 是否为主状态（idle/walking/running/sleeping）
    public var isMainState: Bool {
        switch self {
        case .idle, .walking, .running, .sleeping:
            return true
        case .microYawn, .microStretch:
            return false
        }
    }

    /// 是否为微行为状态
    public var isMicroBehavior: Bool {
        !isMainState
    }

    /// 微行为持续时长（秒）。非微行为状态返回 0
    public var microBehaviorDuration: TimeInterval {
        switch self {
        case .microYawn: return 2.0
        case .microStretch: return 3.0
        default: return 0
        }
    }
}
