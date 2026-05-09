// WSState.swift
// Story 12.1 AC2: WebSocket 连接态枚举（房间页"已连接 / 正在重连 / 已断开"占位文本派生源）.
//
// 设计：value type + Equatable + Sendable（不引 Hashable，无需作 Dictionary key）.
// 节点 4 阶段三态枚举一次到位 —— Story 12.5 自动重连落地后会真实在三态间切换；
// Story 12.6 心跳超时落地后会从 connected → disconnected 触发 reconnecting；
// 本 story 的 RealRoomViewModel 仅在 connect / disconnect 两路径上切，reconnecting 由 Story 12.5 触发.
//
// 关键决策：枚举值不带 associated value（不附 attempt 数 / lastError 等）—— 节点 4 阶段视觉只需三态文字,
// 无附加信息；Story 12.5 真实重连指数退避落地后如果需要展示"第 N 次重连"，再演进为
// `case reconnecting(attempt: Int)` —— 本 story 不预 over-design.
//
// `disconnected` 默认值：与 Story 12.5 落地后"重连失败超过 5 次 → wsState = .disconnected" 终态语义一致；
// 本 story 没有 reconnect 路径，初始态默认 `disconnected` + WS 连接成功后切 `connected`，无中间态切换.

import Foundation

public enum WSState: Equatable, Sendable {
    case connected
    case reconnecting
    case disconnected
}
