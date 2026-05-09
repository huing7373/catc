// WSError.swift
// Story 12.2 AC3: WebSocketClient 错误类型 —— 与 APIError 同精神（终态 case 集 + Equatable + Sendable）.
//
// 设计原则：
//   - 错误来源分类：tokenMissing / invalidURL（构造期），connectionFailed（拨号期），closedByServer（连接后被 close），notConnected（未拨号或已断开），decodingFailed（incoming frame 解码失败）
//   - 与 V1 §12.1 close code 表对齐：closedByServer.code 直接存 server emit 的 close code（4001 / 4003 / 4004 / 4005 / 4006 / 4007 / 1011 / 1006），让 Story 12.5 reconnect 状态机按 code 分类决策
//   - **不**做"按 close code 自动重连"决策（**Story 12.5 范围**）；本 story 仅暴露 code 让上层决策
//   - underlying error 用 String description 而非 Error existential —— Equatable 易实现 + Sendable 易满足

import Foundation

public enum WSError: Error, Equatable, Sendable {
    /// `WebSocketClientImpl.connect(roomId:)` 时 tokenProvider() 返回 nil / 空字符串.
    /// caller（Story 12.7）应触发"无效 token 静默重新登录"流程（参考 ADR-0008 v2 + Story 5.4）.
    case tokenMissing

    /// connect URL 构造失败（baseURL invalid / roomId 含非法字符 —— 节点 4 阶段不应发生，防御性 case）.
    case invalidURL

    /// 拨号失败（DNS 解析失败 / TLS 握手失败 / connection refused / network unreachable 等）.
    /// underlying 用 String description（Equatable 简单）.
    case connectionFailed(underlyingDescription: String)

    /// 连接成功后被 server 主动 close（V1 §12.1 close code 表 4001 / 4003 / 4004 / 4005 / 4006 / 4007 / 1000 / 1001 / 1011）.
    /// caller 按 code 决策：4001 → 重新登录；4003 / 4004 / 4006 / 4007 → 业务级拒绝不重连；4005 / 1006 / 1011 → transient 应重连（指数退避，Story 12.5）；1000 / 1001 → 主动关闭路径.
    case closedByServer(code: Int, reason: String)

    /// `send(_:)` / 复用同 client 时 underlying URLSessionWebSocketTask 已 closed / nil.
    case notConnected

    /// incoming frame 解码失败（非法 JSON / 信封字段缺失）—— 仅在 codec 抛错时出现；正常 case 已被 `.unknown(rawType:)` 兜底.
    case decodingFailed(rawType: String?)
}
