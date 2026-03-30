import Foundation

/// 本地持久化封装（纯本地原则：只做本地读写，不触发网络请求）
public final class LocalStore: @unchecked Sendable {

    private let defaults: UserDefaults

    public init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    // MARK: - Cat State Machine Persistence

    private enum Keys {
        static let lastState = "cat.stateMachine.lastState"
        static let lastStateTimestamp = "cat.stateMachine.lastStateTimestamp"
    }

    /// 保存猫状态机的最后状态
    public func saveCatState(_ state: CatState) {
        defaults.set(state.rawValue, forKey: Keys.lastState)
        defaults.set(Date().timeIntervalSince1970, forKey: Keys.lastStateTimestamp)
    }

    /// 恢复猫状态机的最后状态，不存在则返回 nil
    public func loadCatState() -> CatState? {
        guard let raw = defaults.string(forKey: Keys.lastState) else { return nil }
        return CatState(rawValue: raw)
    }

    /// 最后状态保存时间
    public func loadCatStateTimestamp() -> Date? {
        let ts = defaults.double(forKey: Keys.lastStateTimestamp)
        guard ts > 0 else { return nil }
        return Date(timeIntervalSince1970: ts)
    }
}
