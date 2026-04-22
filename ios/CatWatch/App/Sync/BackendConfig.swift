import Foundation

/// Apple Watch 端后端接入的统一配置入口。
/// 后续 RegistryClient / WatchWebSocketClient / WatchRoomSession 都依赖它，
/// 避免把 host、token、roomId 散落到 UI 或业务层里。
struct BackendConfig: Sendable, Equatable {
    let name: String
    let baseHTTPURL: URL
    let webSocketURL: URL
    let debugToken: String
    let roomID: String

    var authorizationHeaderValue: String {
        "Bearer \(debugToken)"
    }

    var wsRegistryURL: URL {
        baseHTTPURL.appending(path: "v1/platform/ws-registry")
    }

    init(
        name: String,
        baseHTTPURL: URL,
        webSocketURL: URL,
        debugToken: String,
        roomID: String
    ) {
        precondition(!name.isEmpty, "BackendConfig.name must not be empty")
        precondition(["http", "https"].contains(baseHTTPURL.scheme?.lowercased() ?? ""), "BackendConfig.baseHTTPURL must use http or https")
        precondition(["ws", "wss"].contains(webSocketURL.scheme?.lowercased() ?? ""), "BackendConfig.webSocketURL must use ws or wss")
        precondition(!debugToken.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty, "BackendConfig.debugToken must not be empty")
        precondition(!roomID.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty, "BackendConfig.roomID must not be empty")

        self.name = name
        self.baseHTTPURL = baseHTTPURL
        self.webSocketURL = webSocketURL
        self.debugToken = debugToken
        self.roomID = roomID
    }
}

extension BackendConfig {
    /// 默认联调配置。
    /// 可以通过 Scheme 环境变量覆盖，避免每次切环境都改代码。
    static var current: BackendConfig {
        fromEnvironment(ProcessInfo.processInfo.environment) ?? localDebug
    }

    static let localDebug = BackendConfig(
        name: "local-debug",
        baseHTTPURL: URL(string: "http://127.0.0.1:18080")!,
        webSocketURL: URL(string: "ws://127.0.0.1:18080/ws")!,
        debugToken: "watch-alice",
        roomID: "test-room"
    )

    static func fromEnvironment(_ environment: [String: String]) -> BackendConfig? {
        guard
            let httpURLString = environment["CAT_BACKEND_HTTP_URL"],
            let wsURLString = environment["CAT_BACKEND_WS_URL"],
            let baseHTTPURL = URL(string: httpURLString),
            let webSocketURL = URL(string: wsURLString)
        else {
            return nil
        }

        let name = environment["CAT_BACKEND_NAME"] ?? "scheme-env"
        let debugToken = environment["CAT_BACKEND_TOKEN"] ?? "watch-alice"
        let roomID = environment["CAT_BACKEND_ROOM_ID"] ?? "test-room"

        return BackendConfig(
            name: name,
            baseHTTPURL: baseHTTPURL,
            webSocketURL: webSocketURL,
            debugToken: debugToken,
            roomID: roomID
        )
    }
}
