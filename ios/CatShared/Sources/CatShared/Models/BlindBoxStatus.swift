import Foundation

public struct BlindBoxStatus: Codable, Equatable, Sendable {
    public var droppedAt: Date
    public var stepsRequired: Int
    public var stepsAccumulated: Int
    public var rewardName: String

    public init(
        droppedAt: Date,
        stepsRequired: Int = 300,
        stepsAccumulated: Int = 0,
        rewardName: String
    ) {
        self.droppedAt = droppedAt
        self.stepsRequired = stepsRequired
        self.stepsAccumulated = stepsAccumulated
        self.rewardName = rewardName
    }

    public var isUnlocked: Bool {
        stepsAccumulated >= stepsRequired
    }

    public var stepsRemaining: Int {
        max(stepsRequired - stepsAccumulated, 0)
    }
}
