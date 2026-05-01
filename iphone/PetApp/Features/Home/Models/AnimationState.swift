// AnimationState.swift
// Story 37.7 AC4: HomeView CatStage interactionAnimation 状态枚举（floatUp emoji 触发用）.
//
// 设计：enum + Equatable，关联值是 emoji 字符串（"🍥" / "💕" / "⭐" 三种触发；未来扩展加 case）；
// idle 不渲染，flying 触发 1.4s ease 上移消失.

import Foundation

public enum AnimationState: Equatable {
    /// 静止态，不渲染浮动 emoji.
    case idle

    /// 触发 floatUp 动画，关联值是 emoji 字符串（"🍥" / "💕" / "⭐"）.
    /// 1.4s 后 ViewModel 自动重置回 idle（HomeView 内 SwiftUI animation 完成 callback 触发）.
    case flying(String)
}
