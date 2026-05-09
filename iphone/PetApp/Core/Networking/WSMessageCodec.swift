// WSMessageCodec.swift
// Story 12.2 AC4: WS 消息编解码层 —— incoming JSON text frame → WSMessage enum；outgoing WSOutgoingMessage → JSON text frame.
//
// 设计原则：
//   - 与 APIClient 内部 JSONDecoder / JSONEncoder 同精神：每次新建 instance（避免 Sendable 歧义）
//   - incoming 路径走"按 type 字段分发"策略：先解一个 envelope 拿 type，再按 type 解 payload
//   - V1 §12.3 通用信封：{ "type": str, "requestId": str, "payload": {...}, "ts": int64 }
//   - V1 §12.2 通用信封：{ "type": str, "requestId": str, "payload": {...} }（无 ts）
//   - 未识别 type → return .unknown(rawType: ...) + log warn（不抛错；不破坏 stream）
//   - payload 解码失败 → return .unknown(rawType: type) + log warn（同样不破坏 stream；防 server malformed payload 把房间页搞崩）
//
// 节点 4 阶段 incoming 已知 type 集合：room.snapshot / pong / error（Epic 10 钦定）
// 节点 4 阶段 outgoing 已知 type 集合：ping（V1 §12.2）

import Foundation
import os.log

public enum WSMessageCodec {

    private static let logger = OSLog(subsystem: "com.zhuming.pet.app", category: "WSMessageCodec")

    // MARK: - Incoming

    /// 解析 server → client 消息 text frame（UTF-8 JSON string）.
    /// - 已知 type 解码成功 → 返回对应 WSMessage case
    /// - 已知 type 解码失败（payload schema mismatch）→ 返回 .unknown(rawType: type) + log warn
    /// - 未知 type → 返回 .unknown(rawType: rawType) + log warn
    /// - 信封自身解码失败（非 JSON / 缺 type 字段）→ 返回 .unknown(rawType: "") + log error
    public static func decode(_ text: String) -> WSMessage {
        guard let data = text.data(using: .utf8) else {
            os_log(.error, log: logger, "text frame UTF-8 conversion failed")
            return .unknown(rawType: "")
        }
        let envelope: WSEnvelope
        do {
            envelope = try makeDecoder().decode(WSEnvelope.self, from: data)
        } catch {
            os_log(.error, log: logger, "envelope decode failed: %{public}@", String(describing: error))
            return .unknown(rawType: "")
        }
        switch envelope.type {
        case "room.snapshot":
            do {
                let payload = try makeDecoder().decode(RoomSnapshotEnvelope.self, from: data).payload.toDomain()
                return .roomSnapshot(payload)
            } catch {
                os_log(.error, log: logger, "room.snapshot payload decode failed: %{public}@", String(describing: error))
                return .unknown(rawType: "room.snapshot")
            }
        case "pong":
            return .pong(requestId: envelope.requestId)
        case "error":
            do {
                let payload = try makeDecoder().decode(ErrorEnvelope.self, from: data).payload
                return .error(code: payload.code, message: payload.message, requestId: envelope.requestId)
            } catch {
                os_log(.error, log: logger, "error payload decode failed: %{public}@", String(describing: error))
                return .unknown(rawType: "error")
            }
        default:
            os_log(.info, log: logger, "unknown server type: %{public}@", envelope.type)
            return .unknown(rawType: envelope.type)
        }
    }

    // MARK: - Outgoing

    /// 编码 client → server 消息 → text frame（UTF-8 JSON string）.
    /// 节点 4 阶段仅 ping case；future 扩展加 case 即可.
    public static func encode(_ message: WSOutgoingMessage) throws -> String {
        let json: [String: Any]
        switch message {
        case .ping(let requestId):
            json = [
                "type": "ping",
                "requestId": requestId,
                "payload": [String: Any]()  // V1 §12.2 ping payload 固定空对象
            ]
        }
        let data = try JSONSerialization.data(withJSONObject: json, options: [.sortedKeys])
        guard let text = String(data: data, encoding: .utf8) else {
            throw WSError.decodingFailed(rawType: "ping")
        }
        return text
    }

    // MARK: - Internal envelope DTOs

    /// V1 §12.3 通用信封最小投影 —— 仅取分发用 type / requestId 两字段（payload / ts 由后续 envelope 各自解）.
    /// `requestId` 字段在某些 server 推送（如 server-active error）可能为 ""，用 default value 兜底缺失.
    private struct WSEnvelope: Decodable {
        let type: String
        let requestId: String

        enum CodingKeys: String, CodingKey { case type, requestId }

        init(from decoder: Decoder) throws {
            let container = try decoder.container(keyedBy: CodingKeys.self)
            self.type = try container.decode(String.self, forKey: .type)
            self.requestId = (try? container.decodeIfPresent(String.self, forKey: .requestId)) ?? ""
        }
    }

    /// room.snapshot 整体信封 —— 与 V1 §12.3 schema 严格对齐.
    private struct RoomSnapshotEnvelope: Decodable {
        let payload: RoomSnapshotPayloadDTO
    }

    /// payload 层 DTO —— 与 Story 12.1 落地的 RoomSnapshotPayload 字段对齐.
    /// 这里**不**直接 conform Codable 在 RoomSnapshotPayload 上，避免 Story 12.1 既有类型被 codec 实装耦合.
    private struct RoomSnapshotPayloadDTO: Decodable {
        let room: RoomInfoDTO
        let members: [MemberDTO]

        struct RoomInfoDTO: Decodable {
            let id: String
            let maxMembers: Int
            let memberCount: Int
        }

        struct MemberDTO: Decodable {
            let userId: String
            let nickname: String
            let pet: PetDTO?  // V1 §12.3：null = pet-less authoritative 信号

            struct PetDTO: Decodable {
                let petId: String
                let currentState: Int
            }
        }

        func toDomain() -> RoomSnapshotPayload {
            RoomSnapshotPayload(
                room: RoomSnapshotRoomInfo(id: room.id, maxMembers: room.maxMembers, memberCount: room.memberCount),
                members: members.map { dto in
                    RoomSnapshotMember(
                        userId: dto.userId,
                        nickname: dto.nickname,
                        pet: dto.pet.map { p in
                            RoomSnapshotPet(petId: p.petId, currentState: p.currentState)
                        }
                    )
                }
            )
        }
    }

    /// error 整体信封.
    private struct ErrorEnvelope: Decodable {
        let payload: ErrorPayloadDTO

        struct ErrorPayloadDTO: Decodable {
            let code: Int
            let message: String
        }
    }

    private static func makeDecoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        // 节点 4 阶段 WS 信封无 Date 字段；保持默认策略
        return decoder
    }
}
