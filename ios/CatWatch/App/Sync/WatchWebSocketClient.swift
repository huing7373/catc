import Foundation

enum WatchWebSocketConnectionState: Sendable, Equatable {
    case disconnected
    case connecting
    case connected
    case reconnecting(attempt: Int)
}

enum WatchWebSocketClientError: LocalizedError {
    case notConnected
    case unsupportedBinaryMessage
    case failedToEncodePayload

    var errorDescription: String? {
        switch self {
        case .notConnected:
            return "当前 WebSocket 尚未连接"
        case .unsupportedBinaryMessage:
            return "当前客户端只支持文本帧，不支持二进制消息"
        case .failedToEncodePayload:
            return "WebSocket 请求编码失败"
        }
    }
}

@MainActor
final class WatchWebSocketClient: ObservableObject {
    @Published private(set) var connectionState: WatchWebSocketConnectionState = .disconnected
    @Published private(set) var lastErrorMessage: String?

    var onTextMessage: ((String) -> Void)?
    var onStateChanged: ((WatchWebSocketConnectionState) -> Void)?

    private let config: BackendConfig
    private let session: URLSession
    private let encoder: JSONEncoder

    private var task: URLSessionWebSocketTask?
    private var receiveLoopTask: Task<Void, Never>?
    private var reconnectTask: Task<Void, Never>?
    private var isManualDisconnect = false
    private var reconnectAttempt = 0

    private enum ReconnectPolicy {
        static let delays: [TimeInterval] = [0, 1, 2, 5, 10]
        static let maxAttempt = 5
    }

    init(
        config: BackendConfig,
        session: URLSession = .shared,
        encoder: JSONEncoder = JSONEncoder()
    ) {
        self.config = config
        self.session = session
        self.encoder = encoder
    }

    func connect() {
        isManualDisconnect = false
        reconnectTask?.cancel()
        reconnectTask = nil
        reconnectAttempt = 0
        openConnection(asReconnect: false)
    }

    func disconnect() {
        isManualDisconnect = true
        reconnectTask?.cancel()
        reconnectTask = nil
        stopReceiveLoop()

        task?.cancel(with: .normalClosure, reason: nil)
        task = nil
        updateState(.disconnected)
    }

    func sendText(_ text: String) async throws {
        guard let task else {
            throw WatchWebSocketClientError.notConnected
        }

        try await task.send(.string(text))
    }

    func sendJSON<T: Encodable>(_ payload: T) async throws {
        let data = try encoder.encode(payload)
        guard let text = String(data: data, encoding: .utf8) else {
            throw WatchWebSocketClientError.failedToEncodePayload
        }
        try await sendText(text)
    }

    private func openConnection(asReconnect: Bool) {
        stopReceiveLoop()
        task?.cancel(with: .goingAway, reason: nil)
        task = nil

        var request = URLRequest(url: config.webSocketURL)
        request.timeoutInterval = 8
        request.setValue(config.authorizationHeaderValue, forHTTPHeaderField: "Authorization")

        let webSocketTask = session.webSocketTask(with: request)
        task = webSocketTask

        updateState(asReconnect ? .reconnecting(attempt: reconnectAttempt) : .connecting)

        webSocketTask.resume()
        updateState(.connected)
        startReceiveLoop(for: webSocketTask)
    }

    private func startReceiveLoop(for webSocketTask: URLSessionWebSocketTask) {
        receiveLoopTask = Task { [weak self] in
            guard let self else { return }

            while !Task.isCancelled {
                do {
                    let message = try await webSocketTask.receive()
                    try await self.handle(message)
                } catch {
                    await self.handleConnectionFailure(error)
                    return
                }
            }
        }
    }

    private func stopReceiveLoop() {
        receiveLoopTask?.cancel()
        receiveLoopTask = nil
    }

    private func handle(_ message: URLSessionWebSocketTask.Message) async throws {
        switch message {
        case .string(let text):
            lastErrorMessage = nil
            onTextMessage?(text)
        case .data(let data):
            guard let text = String(data: data, encoding: .utf8) else {
                throw WatchWebSocketClientError.unsupportedBinaryMessage
            }
            lastErrorMessage = nil
            onTextMessage?(text)
        @unknown default:
            throw WatchWebSocketClientError.unsupportedBinaryMessage
        }
    }

    private func handleConnectionFailure(_ error: Error) async {
        guard !isManualDisconnect else { return }

        lastErrorMessage = error.localizedDescription
        stopReceiveLoop()
        task = nil
        scheduleReconnect()
    }

    private func scheduleReconnect() {
        guard reconnectAttempt < ReconnectPolicy.maxAttempt else {
            updateState(.disconnected)
            return
        }

        reconnectAttempt += 1
        let delay = ReconnectPolicy.delays[min(reconnectAttempt - 1, ReconnectPolicy.delays.count - 1)]
        updateState(.reconnecting(attempt: reconnectAttempt))

        reconnectTask?.cancel()
        reconnectTask = Task { [weak self] in
            guard let self else { return }

            if delay > 0 {
                try? await Task.sleep(for: .seconds(delay))
            }

            guard !Task.isCancelled else { return }
            self.openConnection(asReconnect: true)
        }
    }

    private func updateState(_ newState: WatchWebSocketConnectionState) {
        connectionState = newState
        onStateChanged?(newState)
    }
}
