// LoadEmojisUseCase.swift
// Story 18.1 AC2: 加载表情列表 UseCase + App 生命周期单例缓存.
//
// 设计选型 (Dev Note #3 钦定):
//   - **首选 `actor DefaultLoadEmojisUseCase`** —— Swift 6 concurrency 友好，cache 字段读写自动串行,
//     无需手动加锁；多 ViewModel 同时调 `execute()` 不会触发 data race.
//   - 与 `LoadHomeUseCase` 的 `struct` 模式区别：`LoadHomeUseCase` 无内部状态 (每次启动重拉);
//     `LoadEmojisUseCase` 持 in-memory cache 字段，必须用 reference / actor 类型保 cache 共享.
//   - **注意 actor reentrancy**：actor 仅保证**单 hop** 串行，遇到 `await` 会释放 isolation,
//     单纯 `if cache == nil { cache = try await repo... }` 模式在并发 caller 下仍会发多次请求
//     (review round 1 P2 修复). 用 `inflightTask: Task<...>?` 做 single-flight,
//     详见 `execute()` 方法注释 + lessons/2026-05-14-actor-reentrancy-needs-inflight-task-for-single-flight.md.
//
// 缓存语义 (V1 §11.1 行 1817 client 缓存契约钦定):
//   - 表情列表是**静态配置**，App 生命周期内首次拉取后缓存，后续不再重复拉取.
//   - server 端表情配置变更需 App 重启 / 主动刷新才能看到新值 —— 节点 6 MVP 无 push 通知机制，可接受.
//   - **不**实装 invalidate / reset 接口 (节点 6 阶段 MVP 不需要).
//
// 缓存写入规则:
//   - **只在 success 路径写**：失败保持 nil 不缓存 error；下次调 `execute()` 时再次走 repo.
//   - 与 Story 5.5 `LoadHomeUseCase` 错误透传精神同源 (失败不污染缓存).
//
// **App 生命周期单例**：caller=AppContainer.loadEmojisUseCase 字段 (stable singleton);
// 与 errorPresenter / sessionStore 同精神，跨 ViewModel 共享同一 instance 保证 cache 共享.
// **禁止**走 `makeLoadEmojisUseCase()` factory 模式 (factory each call new 实例会让 cache 失效).
//
// import 仅 Foundation：actor / async / throws 都在 stdlib.

import Foundation

public protocol LoadEmojisUseCaseProtocol: Sendable {
    /// 拿表情列表 (首次走 repo + 缓存写入；后续命中缓存直接返).
    /// - Returns: `[EmojiConfig]` —— 4 项 fixture / server 配置变更前**永远**返同一 array.
    /// - Throws: APIError 全部 case 原样透传 (repo 抛什么这里抛什么)；
    ///   失败**不**写缓存，下次再调会重新走 repo.
    func execute() async throws -> [EmojiConfig]
}

public actor DefaultLoadEmojisUseCase: LoadEmojisUseCaseProtocol {
    private let repository: EmojiRepositoryProtocol
    private var cache: [EmojiConfig]?
    /// Story 18.1 review round 1 P2 fix: actor 内 single-flight 缓存.
    ///
    /// Swift actor 仅保证**同一 hop** 串行；遇到 `await` 会释放 isolation 让其他 caller 抢进
    /// (actor reentrancy)。在 miss path 上，两个并发 caller 都可能通过 `cache == nil` 检查后
    /// 各自发 GET /emojis —— 破坏 AC2 钦定的"App 生命周期 single-load / cache-once"契约.
    /// 解法：miss path 把 `repository.listEmojis()` 包成 Task 存 `inflightTask`，后续 caller
    /// 先看是否有 inflight；有则 `await` 同一个 Task.value（共享 result）.
    /// 成功 → 写 cache + 清 inflightTask；失败 → 清 inflightTask 让下次 caller 重新走 miss path.
    /// 详见 docs/lessons/2026-05-14-actor-reentrancy-needs-inflight-task-for-single-flight.md.
    private var inflightTask: Task<[EmojiConfig], Error>?

    public init(repository: EmojiRepositoryProtocol) {
        self.repository = repository
        self.cache = nil
        self.inflightTask = nil
    }

    public func execute() async throws -> [EmojiConfig] {
        // cache hit 直接返 (actor 自带串行，cache 字段读写 race-free).
        if let cached = cache {
            return cached
        }
        // 已有 inflight load → 复用同一 Task.value (single-flight 关键步).
        if let inflight = inflightTask {
            return try await inflight.value
        }
        // miss + 无 inflight → 起新 Task；先存 inflightTask 让并发 caller 复用,
        // 再 await 同一 Task 拿结果. 成功写 cache；失败/成功都清 inflightTask (defer).
        let task = Task<[EmojiConfig], Error> { [repository] in
            try await repository.listEmojis()
        }
        self.inflightTask = task
        do {
            let emojis = try await task.value
            self.cache = emojis
            self.inflightTask = nil
            return emojis
        } catch {
            // 失败**不**写缓存 (与原契约一致); 清 inflightTask 让下次 caller 重新走 miss path.
            self.inflightTask = nil
            throw error
        }
    }
}
