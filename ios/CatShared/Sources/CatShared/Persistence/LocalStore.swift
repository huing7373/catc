import Foundation

/// 本地持久化封装（纯本地原则：只做本地读写，不触发网络请求）
public final class LocalStore: @unchecked Sendable {

    private let defaults: UserDefaults
    private let encoder = JSONEncoder()
    private let decoder = JSONDecoder()

    public init(defaults: UserDefaults = .standard) {
        self.defaults = defaults
    }

    // MARK: - Cat State Machine Persistence

    private enum Keys {
        static let lastState = "cat.stateMachine.lastState"
        static let lastStateTimestamp = "cat.stateMachine.lastStateTimestamp"
        static let blindBoxStatus = "cat.watch.blindBoxStatus"
        static let blindBoxLastDropTimestamp = "cat.watch.blindBoxLastDropTimestamp"
        static let blindBoxSpendableSteps = "cat.watch.blindBoxSpendableSteps"
        static let blindBoxObservedTodaySteps = "cat.watch.blindBoxObservedTodaySteps"
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

    // MARK: - Blind Box Persistence

    public func saveBlindBoxStatus(_ status: BlindBoxStatus?) {
        guard let status else {
            defaults.removeObject(forKey: Keys.blindBoxStatus)
            return
        }

        if let data = try? encoder.encode(status) {
            defaults.set(data, forKey: Keys.blindBoxStatus)
        }
    }

    public func loadBlindBoxStatus() -> BlindBoxStatus? {
        guard let data = defaults.data(forKey: Keys.blindBoxStatus) else { return nil }
        return try? decoder.decode(BlindBoxStatus.self, from: data)
    }

    public func saveBlindBoxLastDropDate(_ date: Date) {
        defaults.set(date.timeIntervalSince1970, forKey: Keys.blindBoxLastDropTimestamp)
    }

    public func loadBlindBoxLastDropDate() -> Date? {
        let ts = defaults.double(forKey: Keys.blindBoxLastDropTimestamp)
        guard ts > 0 else { return nil }
        return Date(timeIntervalSince1970: ts)
    }

    public func saveBlindBoxSpendableSteps(_ steps: Int) {
        defaults.set(steps, forKey: Keys.blindBoxSpendableSteps)
    }

    public func loadBlindBoxSpendableSteps() -> Int {
        defaults.integer(forKey: Keys.blindBoxSpendableSteps)
    }

    public func saveBlindBoxObservedTodaySteps(_ steps: Int) {
        defaults.set(steps, forKey: Keys.blindBoxObservedTodaySteps)
    }

    public func loadBlindBoxObservedTodaySteps() -> Int {
        defaults.integer(forKey: Keys.blindBoxObservedTodaySteps)
    }
}
