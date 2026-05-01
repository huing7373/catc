// AnimationState.swift
// Story 37.7 AC4: HomeView CatStage interactionAnimation 状态枚举（floatUp emoji 触发用）.
//
// 设计：enum + Equatable，关联值是 emoji 字符串（"🍥" / "💕" / "⭐" 三种触发；未来扩展加 case）；
// idle 不渲染，flying 触发 1.4s ease 上移消失.
//
// Story 37.7 codex round 2 [P2] fix（"同 emoji 连点不重放动画"）：
//   .flying 关联值除 emoji 外加 `id: UUID` —— 每次 onTap 用 `UUID()` 新实例触发，
//   即使 emoji 相同（如连点 Feed → "🍥" → "🍥"）也保证 Equatable 比较为不等：
//   SwiftUI `onChange(of: state.interactionAnimation)` 必能感知 value 变化，重放动画.
//
// Equatable 实装契约（关键）：
//   - .idle == .idle → true
//   - .flying(e1, id1) == .flying(e2, id2) → e1 == e2 && id1 == id2
//   - 不同 UUID 即视为不同 value（即便 emoji 相同）—— 这是连点重放的核心保证.

import Foundation

public enum AnimationState: Equatable {
    /// 静止态，不渲染浮动 emoji.
    case idle

    /// 触发 floatUp 动画.
    /// - Parameters:
    ///   - emoji: 浮动 emoji 字符串（"🍥" / "💕" / "⭐"）.
    ///   - id: 唯一 trigger id —— 每次 onTap 新生成 `UUID()`，即使 emoji 相同也保证 Equatable 不等
    ///     （SwiftUI onChange 才能感知；连点同 emoji 才能重放动画）.
    /// 1.4s 后 ViewModel 自动重置回 idle（HomeView 内 SwiftUI animation 完成 callback 触发）.
    case flying(emoji: String, id: UUID)
}
