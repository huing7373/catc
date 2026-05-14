// RoomActiveEmoji.swift
// Story 18.3 AC2: 房间内 transient 表情动效队列元素（self + others 共用同一数据结构）.
//
// 设计原则:
//   - id = UUID() 每次入队全新生成 → SwiftUI ForEach 区分同 userId 同 emojiCode 多次入队 (18.3 连点用例;
//     epics.md 行 2697 钦定"连点 3 次 → activeEmojis 添加 3 项")
//   - 数据来源:
//     * 18.3 路径: RoomViewModel.onEmojiSelected 入队 (self 触发)
//     * 18.4 路径: applyEmojiReceived 入队 (others 触发; self echo 由 18.4 跳过去重不入队)
//   - 18.4 落地的 1.5s 后按 createdAt 自动 expire 移除路径需要本 struct 全部 4 字段
//   - 不持久化 (transient UI 队列, ADR-0010 §3.2 钦定); 不进 AppState
//
// 物理位置: Features/Emoji/Models/ 与 EmojiConfig.swift / EmojiListResponse.swift 同模块.

import Foundation

public struct RoomActiveEmoji: Identifiable, Equatable, Sendable {
    /// 每次入队全新 UUID; 让 SwiftUI ForEach 区分同 emojiCode 多次入队.
    /// 18.4 落地 1.5s 后按 id 移除 (vm.activeEmojis.removeAll { $0.id == toRemove.id }).
    public let id: UUID

    /// 触发者 userId; 18.3 路径 = vm.currentUserId; 18.4 路径 = emoji.received.payload.userId.
    /// 18.4 落地的"按 userId 找猫位 anchor"用; userId 在 roster 不存在时 fallback 渲染屏幕中央 (V1 §12.3 行 2473).
    public let userId: String

    /// V1 §11.1 / §12.3 emojiCode (与 EmojiConfig.code 同 wire 字段);
    /// 18.4 落地按 code 查 LoadEmojisUseCase cache 拿 assetUrl.
    public let emojiCode: String

    /// 入队时刻; 18.4 落地 1.5s 后按 createdAt 自动 expire 移除时用.
    /// 本 story (18.3) **不**实装自动移除; 仅落字段为 18.4 留 hook.
    public let createdAt: Date

    public init(id: UUID, userId: String, emojiCode: String, createdAt: Date) {
        self.id = id
        self.userId = userId
        self.emojiCode = emojiCode
        self.createdAt = createdAt
    }
}
