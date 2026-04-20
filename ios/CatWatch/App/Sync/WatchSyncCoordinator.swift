import Foundation
import CatShared

@MainActor
final class WatchSyncCoordinator: ObservableObject {
    @Published private(set) var statusText = "联机同步未开始"
    @Published private(set) var isHealthy = false
    @Published private(set) var connectionState: WatchWebSocketConnectionState = .disconnected

    let friendStore: FriendCatStore

    private let config: BackendConfig
    private let registryClient: RegistryClient
    private let webSocketClient: WatchWebSocketClient
    private let roomSession: WatchRoomSession
    private let isoFormatter = ISO8601DateFormatter()

    private var hasStarted = false
    private var pendingDebugEchoRequestID: String?
    private var latestPendingAction: String?
    private let decoder = JSONDecoder()

    init(
        config: BackendConfig = .current,
        registryClient: RegistryClient? = nil,
        webSocketClient: WatchWebSocketClient? = nil,
        friendStore: FriendCatStore? = nil
    ) {
        self.config = config
        self.registryClient = registryClient ?? RegistryClient(config: config)
        self.webSocketClient = webSocketClient ?? WatchWebSocketClient(config: config)
        self.friendStore = friendStore ?? FriendCatStore(localUserID: config.debugToken)
        self.roomSession = WatchRoomSession(
            config: config,
            webSocketClient: self.webSocketClient,
            friendStore: self.friendStore
        )

        self.webSocketClient.onStateChanged = { [weak self] state in
            self?.handleConnectionStateChange(state)
        }
        self.webSocketClient.onTextMessage = { [weak self] text in
            self?.handleIncomingText(text)
        }
        self.roomSession.onStatusTextChanged = { [weak self] text in
            self?.statusText = text
        }
        self.roomSession.onJoined = { [weak self] in
            self?.flushLatestPendingActionIfNeeded()
        }
    }

    func start() {
        guard !hasStarted else { return }
        hasStarted = true

        Task {
            await bootstrap()
        }
    }

    func handleLocalStateChange(_ state: CatState) {
        let action = SyncActionCodec.encode(state: state)
        latestPendingAction = action

        guard roomSession.isJoined else { return }
        Task {
            await roomSession.sendActionUpdate(action)
        }
    }

    private func bootstrap() async {
        statusText = "正在检查联机环境"
        isHealthy = false

        do {
            let registryStatus = try await registryClient.fetchRequiredMVPRegistryStatus()
            guard registryStatus.isReadyForRoomMVP else {
                statusText = "后端缺少消息：\(registryStatus.missingRequiredTypes.joined(separator: ", "))"
                return
            }

            statusText = "已发现联机环境，正在连接 WS"
            webSocketClient.connect()
        } catch {
            statusText = "registry 检查失败：\(error.localizedDescription)"
        }
    }

    private func handleConnectionStateChange(_ state: WatchWebSocketConnectionState) {
        connectionState = state

        switch state {
        case .disconnected:
            if !isHealthy {
                statusText = "WS 已断开"
            }
        case .connecting:
            statusText = "WS 连接中"
        case .connected:
            roomSession.resetForReconnect()
            statusText = "WS 已连接，发送 debug.echo"
            Task {
                await sendDebugEcho()
            }
        case .reconnecting(let attempt):
            isHealthy = false
            roomSession.resetForReconnect()
            statusText = "WS 重连中 (\(attempt))"
        }
    }

    private func sendDebugEcho() async {
        let requestID = UUID().uuidString
        pendingDebugEchoRequestID = requestID

        let payload = DebugEchoPayload(
            source: config.debugToken,
            message: "watch-debug-echo",
            sentAt: isoFormatter.string(from: Date())
        )
        let envelope = WSRequestEnvelope(
            id: requestID,
            type: "debug.echo",
            payload: payload
        )

        do {
            try await webSocketClient.sendJSON(envelope)
            statusText = "debug.echo 已发送"
        } catch {
            isHealthy = false
            statusText = "debug.echo 发送失败：\(error.localizedDescription)"
        }
    }

    private func handleIncomingText(_ text: String) {
        if handleDebugEchoResponse(text) {
            return
        }

        _ = roomSession.handleIncomingText(text)
    }

    private func handleDebugEchoResponse(_ text: String) -> Bool {
        guard let data = text.data(using: .utf8) else { return false }
        guard let response = try? decoder.decode(WSResponseEnvelope<DebugEchoPayload>.self, from: data),
              response.type == "debug.echo.result",
              response.id == pendingDebugEchoRequestID else {
            return false
        }

        if response.ok, let payload = response.payload {
            isHealthy = true
            statusText = "debug.echo 已跑通：\(payload.message)"
            Task {
                await roomSession.joinCurrentRoom()
            }
        } else {
            isHealthy = false
            let message = response.error?.message ?? "未知错误"
            statusText = "debug.echo 失败：\(message)"
        }

        return true
    }

    private func flushLatestPendingActionIfNeeded() {
        guard let latestPendingAction else { return }

        Task {
            await roomSession.sendActionUpdate(latestPendingAction)
        }
    }
}
