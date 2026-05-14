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
// 节点 6 阶段 incoming 扩展：emoji.received（V1 §12.3 行 2435-2481，Story 17.1 锚定 + 18.4 client 落地）
// 节点 4 阶段 outgoing 已知 type 集合：ping（V1 §12.2）；节点 6 阶段扩展：emoji.send（V1 §12.2 行 1985-2089，Story 17.1 锚定 + 18.3 落地）

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
        case "member.joined":
            // Story 12.4：member.joined 路由（V1 §12.3 行 2003-2013 字段表）.
            // fix-review r2 P2：required 字段语义校验 —— Decodable 只能挡 absent / type-mismatch；
            // server 若推送 `userId == ""` 或 V1 §12.3 钦定非空的 `nickname == ""`，Decodable 仍会
            // 成功解码出空字符串。codec 必须把这类语义无效 payload fallback 为 .unknown，
            // 否则 RealRoomViewModel.applyMemberJoined 会用空 entry 污染 roster.
            do {
                let dto = try makeDecoder().decode(MemberJoinedEnvelope.self, from: data).payload
                guard !dto.userId.isEmpty else {
                    os_log(.error, log: logger, "member.joined rejected: empty userId")
                    return .unknown(rawType: "member.joined")
                }
                guard !dto.nickname.isEmpty else {
                    os_log(.error, log: logger, "member.joined rejected: empty nickname (V1 §12.3 钦定非空)")
                    return .unknown(rawType: "member.joined")
                }
                return .memberJoined(dto.toDomain())
            } catch {
                os_log(.error, log: logger, "member.joined payload decode failed: %{public}@", String(describing: error))
                return .unknown(rawType: "member.joined")
            }
        case "member.left":
            // Story 12.4：member.left 路由（V1 §12.3 行 2073-2080 字段表）.
            // fix-review r2 P2：同 member.joined 精神 —— `userId == ""` 是语义无效 payload，
            // 即便 ViewModel.applyMemberLeft 因 userId 不存在会 ignore，codec 层先 fallback 更稳.
            do {
                let dto = try makeDecoder().decode(MemberLeftEnvelope.self, from: data).payload
                guard !dto.userId.isEmpty else {
                    os_log(.error, log: logger, "member.left rejected: empty userId")
                    return .unknown(rawType: "member.left")
                }
                return .memberLeft(dto.toDomain())
            } catch {
                os_log(.error, log: logger, "member.left payload decode failed: %{public}@", String(describing: error))
                return .unknown(rawType: "member.left")
            }
        case "pet.state.changed":
            // Story 15.2：pet.state.changed 路由（V1 §12.3 行 2223-2230 字段表）.
            // 同 member.joined / member.left 精神：Decodable 只能挡 absent / type-mismatch；
            // server 若推送语义无效 payload（如 `userId == ""` / `petId == ""`）仍会成功解码 ——
            // 但 V1 §12.3 行 2250 钦定三字段必填且非空，codec 必须把这类语义无效 payload fallback 为
            // `.unknown(rawType: "pet.state.changed")` 走 Story 10.1 钦定"安全忽略未识别 type" + log warn 路径,
            // 避免 ViewModel.applyPetStateChanged 用空字段污染 memberPetStates.
            //
            // currentState 值域校验：codec **不**强校验 1/2/3 —— 容忍 server 未来扩展新状态值（如 sleep=4），
            // ViewModel.applyPetStateChanged 层做 HomePetState(rawValue:) 映射 + 未知值 fallback `.rest` + log warn
            // （与 applySnapshot 同语义；Story 15.1 AC1 已落地 fallback 模式）.
            do {
                let dto = try makeDecoder().decode(PetStateChangedEnvelope.self, from: data).payload
                guard !dto.userId.isEmpty else {
                    os_log(.error, log: logger, "pet.state.changed rejected: empty userId")
                    return .unknown(rawType: "pet.state.changed")
                }
                guard !dto.petId.isEmpty else {
                    os_log(.error, log: logger, "pet.state.changed rejected: empty petId")
                    return .unknown(rawType: "pet.state.changed")
                }
                return .petStateChanged(dto.toDomain())
            } catch {
                os_log(.error, log: logger, "pet.state.changed payload decode failed: %{public}@", String(describing: error))
                return .unknown(rawType: "pet.state.changed")
            }
        case "emoji.received":
            // Story 18.4: emoji.received 路由 (V1 §12.3 行 2435-2481 字段表).
            // 同 member.joined / member.left / pet.state.changed 精神:
            //   Decodable 只能挡 absent / type-mismatch; server 若推送语义无效 payload (如 userId == "" / emojiCode == "")
            //   仍会成功解码 —— V1 §12.3 行 2469 钦定两字段必填且非空 (缺字段视为契约违反; client 解析层走"安全忽略 + log warn"),
            //   codec 必须把这类语义无效 payload fallback 为 .unknown(rawType: "emoji.received") 走 Story 10.1 钦定
            //   "安全忽略未识别 type" + log error 路径, 避免 ViewModel.applyEmojiReceived 用空字段污染 activeEmojis.
            //
            // payload.emojiCode 字符集校验 ([a-z0-9_-] + length 1-64, V1 §11.1 行 1771) codec **不**做 —— 由 server 在
            // §12.2 服务端逻辑步骤 4 校验过 (single source of truth); client 信任 server 输出.
            // catalog miss (emojiCode 不在 §11.1 client 缓存) 不由 codec 层处理 —— V1 §12.3 行 2474 (d) 钦定渲染层 fallback.
            do {
                let dto = try makeDecoder().decode(EmojiReceivedEnvelope.self, from: data).payload
                guard !dto.userId.isEmpty else {
                    os_log(.error, log: logger, "emoji.received rejected: empty userId")
                    return .unknown(rawType: "emoji.received")
                }
                guard !dto.emojiCode.isEmpty else {
                    os_log(.error, log: logger, "emoji.received rejected: empty emojiCode")
                    return .unknown(rawType: "emoji.received")
                }
                return .emojiReceived(dto.toDomain())
            } catch {
                os_log(.error, log: logger, "emoji.received payload decode failed: %{public}@", String(describing: error))
                return .unknown(rawType: "emoji.received")
            }
        default:
            os_log(.info, log: logger, "unknown server type: %{public}@", envelope.type)
            return .unknown(rawType: envelope.type)
        }
    }

    // MARK: - Outgoing

    /// 编码 client → server 消息 → text frame（UTF-8 JSON string）.
    /// 节点 4 阶段仅 ping case；节点 6 阶段扩展 emoji.send（Story 17.1 锚定 + 18.3 落地）.
    public static func encode(_ message: WSOutgoingMessage) throws -> String {
        let json: [String: Any]
        let rawType: String
        switch message {
        case .ping(let requestId):
            rawType = "ping"
            json = [
                "type": "ping",
                "requestId": requestId,
                "payload": [String: Any]()  // V1 §12.2 ping payload 固定空对象
            ]
        case .emojiSend(let requestId, let emojiCode):
            // Story 18.3 AC1: V1 §12.2 行 2000-2008 wire schema 严格对齐.
            // sortedKeys 让 JSON 输出 key 顺序确定（与 ping 同精神, testing 友好）.
            rawType = "emoji.send"
            json = [
                "type": "emoji.send",
                "requestId": requestId,
                "payload": ["emojiCode": emojiCode]
            ]
        }
        let data = try JSONSerialization.data(withJSONObject: json, options: [.sortedKeys])
        guard let text = String(data: data, encoding: .utf8) else {
            throw WSError.decodingFailed(rawType: rawType)
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

    // MARK: - Story 12.4 member.joined / member.left envelope DTOs

    /// member.joined 整体信封 —— 与 V1 §12.3 行 2003-2013 字段表 1:1 对齐.
    private struct MemberJoinedEnvelope: Decodable {
        let payload: MemberJoinedPayloadDTO

        struct MemberJoinedPayloadDTO: Decodable {
            let userId: String
            let nickname: String
            let avatarUrl: String
            let pet: PetDTO?  // V1 §12.3：null = pet-less authoritative 信号

            struct PetDTO: Decodable {
                let petId: String
                let currentState: Int
            }

            func toDomain() -> MemberJoinedPayload {
                MemberJoinedPayload(
                    userId: userId,
                    nickname: nickname,
                    avatarUrl: avatarUrl,
                    pet: pet.map { MemberJoinedPet(petId: $0.petId, currentState: $0.currentState) }
                )
            }
        }
    }

    /// member.left 整体信封 —— 与 V1 §12.3 行 2073-2080 字段表 1:1 对齐.
    private struct MemberLeftEnvelope: Decodable {
        let payload: MemberLeftPayloadDTO

        struct MemberLeftPayloadDTO: Decodable {
            let userId: String

            func toDomain() -> MemberLeftPayload {
                MemberLeftPayload(userId: userId)
            }
        }
    }

    // MARK: - Story 15.2 pet.state.changed envelope DTO

    /// pet.state.changed 整体信封 —— 与 V1 §12.3 行 2223-2230 字段表 1:1 对齐.
    /// 三字段（userId / petId / currentState）全部 required —— Decodable 缺字段会 throw，
    /// 走外层 do-catch 的 `.unknown(rawType: "pet.state.changed")` fallback.
    private struct PetStateChangedEnvelope: Decodable {
        let payload: PetStateChangedPayloadDTO

        struct PetStateChangedPayloadDTO: Decodable {
            let userId: String
            let petId: String
            let currentState: Int

            func toDomain() -> PetStateChangedPayload {
                PetStateChangedPayload(userId: userId, petId: petId, currentState: currentState)
            }
        }
    }

    // MARK: - Story 18.4 emoji.received envelope DTO

    /// emoji.received 整体信封 —— 与 V1 §12.3 行 2446-2450 字段表 1:1 对齐.
    /// 两字段 (userId / emojiCode) 全部 required —— Decodable 缺字段会 throw,
    /// 走外层 do-catch 的 .unknown(rawType: "emoji.received") fallback.
    private struct EmojiReceivedEnvelope: Decodable {
        let payload: EmojiReceivedPayloadDTO

        struct EmojiReceivedPayloadDTO: Decodable {
            let userId: String
            let emojiCode: String

            func toDomain() -> EmojiReceivedPayload {
                EmojiReceivedPayload(userId: userId, emojiCode: emojiCode)
            }
        }
    }

    private static func makeDecoder() -> JSONDecoder {
        let decoder = JSONDecoder()
        // 节点 4 阶段 WS 信封无 Date 字段；保持默认策略
        return decoder
    }
}
