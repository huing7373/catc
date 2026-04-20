import Foundation

struct WSRegistryResponse: Decodable, Sendable, Equatable {
    let apiVersion: String
    let serverTime: String
    let messages: [WSRegistryMessage]
}

struct WSRegistryMessage: Decodable, Sendable, Equatable {
    let type: String
    let version: String
    let direction: String
    let requiresAuth: Bool
    let requiresDedup: Bool
}

enum RegistryClientError: LocalizedError {
    case invalidResponse
    case badStatusCode(Int)
    case unsupportedRegistryVersion(String)

    var errorDescription: String? {
        switch self {
        case .invalidResponse:
            return "后端返回了无效的 registry 响应"
        case .badStatusCode(let code):
            return "请求 ws-registry 失败，HTTP 状态码 \(code)"
        case .unsupportedRegistryVersion(let version):
            return "当前 watch 端暂不支持 registry 版本 \(version)"
        }
    }
}

final class RegistryClient: @unchecked Sendable {
    private let config: BackendConfig
    private let session: URLSession
    private let decoder: JSONDecoder

    init(
        config: BackendConfig,
        session: URLSession = .shared,
        decoder: JSONDecoder = JSONDecoder()
    ) {
        self.config = config
        self.session = session
        self.decoder = decoder
    }

    func fetchRegistry() async throws -> WSRegistryResponse {
        var request = URLRequest(url: config.wsRegistryURL)
        request.httpMethod = "GET"
        request.timeoutInterval = 8

        let (data, response) = try await session.data(for: request)

        guard let httpResponse = response as? HTTPURLResponse else {
            throw RegistryClientError.invalidResponse
        }

        guard (200...299).contains(httpResponse.statusCode) else {
            throw RegistryClientError.badStatusCode(httpResponse.statusCode)
        }

        let registry = try decoder.decode(WSRegistryResponse.self, from: data)
        guard registry.apiVersion == "v1" else {
            throw RegistryClientError.unsupportedRegistryVersion(registry.apiVersion)
        }

        return registry
    }

    func supportsMessageTypes(_ expectedTypes: Set<String>) async throws -> Bool {
        let registry = try await fetchRegistry()
        let supportedTypes = Set(registry.messages.map(\.type))
        return expectedTypes.isSubset(of: supportedTypes)
    }

    func fetchRequiredMVPRegistryStatus() async throws -> RegistryStatus {
        let registry = try await fetchRegistry()
        let supportedTypes = Set(registry.messages.map(\.type))
        let requiredTypes = Set([
            "debug.echo",
            "room.join",
            "action.update",
            "action.broadcast"
        ])

        return RegistryStatus(
            environmentName: config.name,
            apiVersion: registry.apiVersion,
            serverTime: registry.serverTime,
            supportedTypes: supportedTypes,
            requiredTypes: requiredTypes
        )
    }
}

struct RegistryStatus: Sendable, Equatable {
    let environmentName: String
    let apiVersion: String
    let serverTime: String
    let supportedTypes: Set<String>
    let requiredTypes: Set<String>

    var isReadyForDebugEcho: Bool {
        supportedTypes.contains("debug.echo")
    }

    var isReadyForRoomMVP: Bool {
        requiredTypes.isSubset(of: supportedTypes)
    }

    var missingRequiredTypes: [String] {
        requiredTypes.subtracting(supportedTypes).sorted()
    }
}
