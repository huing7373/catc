import Foundation

struct RoomJoinRequestPayload: Codable, Sendable, Equatable {
    let roomId: String
}

struct RoomJoinResponsePayload: Codable, Sendable, Equatable {
    let roomId: String
    let members: [RoomMemberSnapshot]
}

struct RoomMemberSnapshot: Codable, Sendable, Equatable {
    let userId: String
    let action: String
    let tsMs: Int64
}

struct ActionUpdateRequestPayload: Codable, Sendable, Equatable {
    let action: String
}

struct ActionBroadcastPayload: Codable, Sendable, Equatable {
    let userId: String
    let action: String
    let tsMs: Int64
}

@MainActor
final class WatchRoomSession {
    private let config: BackendConfig
    private let webSocketClient: WatchWebSocketClient
    private let friendStore: FriendCatStore
    private let decoder = JSONDecoder()

    private(set) var isJoined = false
    private var pendingJoinRequestID: String?
    private var lastSentActionRequestID: String?

    var onJoined: (() -> Void)?
    var onStatusTextChanged: ((String) -> Void)?

    init(
        config: BackendConfig,
        webSocketClient: WatchWebSocketClient,
        friendStore: FriendCatStore
    ) {
        self.config = config
        self.webSocketClient = webSocketClient
        self.friendStore = friendStore
    }

    func resetForReconnect() {
        isJoined = false
        pendingJoinRequestID = nil
        lastSentActionRequestID = nil
    }

    func joinCurrentRoom() async {
        let requestID = UUID().uuidString
        pendingJoinRequestID = requestID

        let envelope = WSRequestEnvelope(
            id: requestID,
            type: "room.join",
            payload: RoomJoinRequestPayload(roomId: config.roomID)
        )

        do {
            try await webSocketClient.sendJSON(envelope)
            onStatusTextChanged?("room.join 已发送")
        } catch {
            onStatusTextChanged?("room.join 发送失败：\(error.localizedDescription)")
        }
    }

    func sendActionUpdate(_ action: String) async {
        guard isJoined else {
            onStatusTextChanged?("等待 room.join 成功后再发动作")
            return
        }

        let requestID = UUID().uuidString
        lastSentActionRequestID = requestID

        let envelope = WSRequestEnvelope(
            id: requestID,
            type: "action.update",
            payload: ActionUpdateRequestPayload(action: action)
        )

        do {
            try await webSocketClient.sendJSON(envelope)
        } catch {
            onStatusTextChanged?("action.update 发送失败：\(error.localizedDescription)")
        }
    }

    @discardableResult
    func handleIncomingText(_ text: String) -> Bool {
        guard let data = text.data(using: .utf8) else { return false }

        if let joinResponse = try? decoder.decode(WSResponseEnvelope<RoomJoinResponsePayload>.self, from: data),
           joinResponse.type == "room.join.result" {
            handleRoomJoinResponse(joinResponse)
            return true
        }

        if let updateResponse = try? decoder.decode(WSResponseEnvelope<EmptyPayload>.self, from: data),
           updateResponse.type == "action.update.result" {
            handleActionUpdateResponse(updateResponse)
            return true
        }

        if let broadcast = try? decoder.decode(WSDownstreamEnvelope<ActionBroadcastPayload>.self, from: data),
           broadcast.type == "action.broadcast" {
            friendStore.applyBroadcast(broadcast.payload)
            return true
        }

        return false
    }

    private func handleRoomJoinResponse(_ response: WSResponseEnvelope<RoomJoinResponsePayload>) {
        guard response.id == pendingJoinRequestID else { return }

        if response.ok, let payload = response.payload {
            isJoined = true
            friendStore.replace(with: payload.members)
            onStatusTextChanged?("room.join 成功：\(payload.members.count) 位好友")
            onJoined?()
        } else {
            isJoined = false
            let message = response.error?.message ?? "未知错误"
            onStatusTextChanged?("room.join 失败：\(message)")
        }
    }

    private func handleActionUpdateResponse(_ response: WSResponseEnvelope<EmptyPayload>) {
        guard response.id == lastSentActionRequestID else { return }

        if !response.ok {
            let message = response.error?.message ?? "未知错误"
            onStatusTextChanged?("action.update 失败：\(message)")
        }
    }
}
