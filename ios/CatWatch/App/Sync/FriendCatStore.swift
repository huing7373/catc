import Foundation
import CatShared

struct FriendCatPresence: Identifiable, Sendable, Equatable {
    let id: String
    let userID: String
    let state: CatState
    let rawAction: String
    let updatedAtMs: Int64
}

@MainActor
final class FriendCatStore: ObservableObject {
    @Published private(set) var friends: [FriendCatPresence] = []

    private let localUserID: String

    init(localUserID: String) {
        self.localUserID = localUserID
    }

    func replace(with snapshots: [RoomMemberSnapshot]) {
        friends = snapshots
            .filter { $0.userId != localUserID }
            .map {
                FriendCatPresence(
                    id: $0.userId,
                    userID: $0.userId,
                    state: SyncActionCodec.decode(action: $0.action),
                    rawAction: $0.action,
                    updatedAtMs: $0.tsMs
                )
            }
    }

    func applyBroadcast(_ payload: ActionBroadcastPayload) {
        guard payload.userId != localUserID else { return }

        let presence = FriendCatPresence(
            id: payload.userId,
            userID: payload.userId,
            state: SyncActionCodec.decode(action: payload.action),
            rawAction: payload.action,
            updatedAtMs: payload.tsMs
        )

        if let index = friends.firstIndex(where: { $0.userID == payload.userId }) {
            friends[index] = presence
        } else {
            friends.append(presence)
        }
    }

    var topThreeFriends: [FriendCatPresence] {
        Array(friends.prefix(3))
    }
}

enum SyncActionCodec {
    static func encode(state: CatState) -> String {
        switch state {
        case .idle, .sleeping, .walking, .running:
            return state.rawValue
        case .microYawn, .microStretch:
            return CatState.idle.rawValue
        }
    }

    static func decode(action: String) -> CatState {
        switch action {
        case CatState.walking.rawValue:
            return .walking
        case CatState.running.rawValue:
            return .running
        case CatState.sleeping.rawValue:
            return .sleeping
        default:
            return .idle
        }
    }
}
