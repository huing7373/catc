import Foundation

struct WSRequestEnvelope<Payload: Encodable & Sendable>: Encodable, Sendable {
    let id: String
    let type: String
    let payload: Payload
}

struct WSResponseEnvelope<Payload: Decodable & Sendable>: Decodable, Sendable {
    let id: String
    let ok: Bool
    let type: String
    let payload: Payload?
    let error: WSErrorPayload?
}

struct WSDownstreamEnvelope<Payload: Decodable & Sendable>: Decodable, Sendable {
    let type: String
    let payload: Payload
}

struct WSErrorPayload: Decodable, Sendable, Equatable {
    let code: String
    let message: String
}

struct EmptyPayload: Codable, Sendable, Equatable {}

struct DebugEchoPayload: Codable, Sendable, Equatable {
    let source: String
    let message: String
    let sentAt: String
}
