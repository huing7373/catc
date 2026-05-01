// MockHomeViewModel.swift
// Story 37.7 AC2: HomeViewModel mock 子类，用于 #Preview / UITest skip-guest-login / Scaffold 单元测试.
//
// 设计：
//   - 硬编码 mock 数据（greeting / weather / stats / nickname / chestRemaining 等）
//   - override 5 个 abstract method 仅 print log（不调任何 UseCase / AppState mutation）
//   - 暴露 `invocations: [Invocation]` 数组让单元测试断言点击触发
//   - **不**依赖 AppState（Mock 路径走纯 ViewModel-only 数据）
//
// Story 37.7 codex round 4 [P0-hardening] fix：显式 `import Combine` —— 本文件用 `@Published`.
// 当前 iOS SDK transitive import 让 `@Published` 编译 OK（基线 build 271/271 pass，[P0] codex 报误），
// 但跨 SDK / 跨 module 不保证 transitive；显式 import 更 future-proof,
// 与 Story 2.2 lesson docs/lessons/2026-04-25-swift-explicit-import-combine.md 同精神.

import Foundation
import Combine
import os.log

@MainActor
public final class MockHomeViewModel: HomeViewModel {
    /// 单元测试用：记录所有方法调用
    public enum Invocation: Equatable {
        case createTap
        case joinTap
        case feedTap
        case petTap
        case playTap
    }

    @Published public var invocations: [Invocation] = []

    public init() {
        super.init(
            nickname: "小花",
            appVersion: "0.0.0",
            serverInfo: "mock"
        )
        // 重置默认值为更"展示用" mock 数据
        self.greeting = "小花想你啦 ♥"
        self.weather = "今天 · 晴"
        self.stats = .mockHappy
        self.interactionAnimation = .idle
        self.showJoinModal = false
    }

    // MARK: - override abstract methods

    public override func onCreateTap() {
        os_log(.debug, "MockHomeViewModel.onCreateTap")
        invocations.append(.createTap)
    }

    public override func onJoinTap() {
        os_log(.debug, "MockHomeViewModel.onJoinTap")
        invocations.append(.joinTap)
        self.showJoinModal = true
    }

    // Story 37.7 codex round 2 [P2] fix：同 RealHomeViewModel 一样，每次 onTap 用 UUID() 新实例
    // 保证 AnimationState Equatable 不等（连点同 emoji 也重放动画）.
    public override func onFeedTap() {
        os_log(.debug, "MockHomeViewModel.onFeedTap")
        invocations.append(.feedTap)
        self.interactionAnimation = .flying(emoji: "🍥", id: UUID())
    }

    public override func onPetTap() {
        os_log(.debug, "MockHomeViewModel.onPetTap")
        invocations.append(.petTap)
        self.interactionAnimation = .flying(emoji: "💕", id: UUID())
    }

    public override func onPlayTap() {
        os_log(.debug, "MockHomeViewModel.onPlayTap")
        invocations.append(.playTap)
        self.interactionAnimation = .flying(emoji: "⭐", id: UUID())
    }
}
