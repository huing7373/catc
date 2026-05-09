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

import Foundation

public enum WSOutgoingMessage: Equatable, Sendable {
    /// V1 §12.2 ping —— Story 12.6 心跳框架消费.
    /// `requestId`：客户端可生成（推荐 `"ping_<seq>"` 或 `"ping_<ts_ms>"`）；省略时用空字符串 `""`（server 回带空 requestId）.
    case ping(requestId: String)
}
