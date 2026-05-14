// WSOutgoingMessage.swift
// Story 12.2 AC2: client → server WS 消息 enum（与 incoming WSMessage 严格分离）.
//
// 设计原则：
//   - 节点 4 阶段 V1 §12.2 钦定 client → server 仅 `ping` 一种合法消息（emoji.send 由 Story 17.1 锚定加入）
//   - 与 incoming WSMessage 分离：incoming 是 server-controlled（含 unknown fallback），outgoing 是 client-controlled（必须严格符合协议）
//   - case 数固定 → enum 而非 protocol；Codable 由 WSMessageCodec 处理而**不**让 enum 自身 conform Codable
//     （让 codec 集中控制 wire JSON 形态 —— payload 始终为 `{}`，requestId 字段层固定写出）
//
// V1 §12.2 ping 信封：
//   { "type": "ping", "requestId": "<optional>", "payload": {} }
//
// 心跳间隔 / 触发由 Story 12.6 决定，本 story 仅提供消息构造 + 编码.
//
// outgoing 已知 type 集合：ping（V1 §12.2）+ emoji.send（V1 §12.2 行 1985-2089，Story 17.1 锚定 + 18.3 落地）.

import Foundation

public enum WSOutgoingMessage: Equatable, Sendable {
    /// V1 §12.2 ping —— Story 12.6 心跳框架消费.
    /// `requestId`：客户端可生成（推荐 `"ping_<seq>"` 或 `"ping_<ts_ms>"`）；省略时用空字符串 `""`（server 回带空 requestId）.
    case ping(requestId: String)

    /// Story 18.3 AC1: V1 §12.2 行 1985-2089 `emoji.send` —— Story 18.3 落地, client → server fire-and-forget.
    /// `requestId`: client 生成, 推荐 "emoji_<ts_ms>" 格式 (V1 §12.2 行 1993); server 处理失败时回 error 消息回带该 requestId.
    /// `emojiCode`: V1 §11.1 `data.items[].code` 字段值; client 必须从 §11.1 缓存的合法列表取 (V1 §12.2 行 2074);
    ///              字符集 `[a-z0-9_-]`, length 1-64 (V1 §11.1 行 1771).
    /// 序列化由 WSMessageCodec.encode(.emojiSend) 处理; wire schema 严格对齐 V1 §12.2 行 2000-2008.
    case emojiSend(requestId: String, emojiCode: String)
}
