// LoadEmojisUseCaseTests.swift
// Story 18.1 AC7: LoadEmojisUseCase 单测覆盖 (≥3 case，MockEmojiRepository capture invocation count).
//
// 测试目标：验证 cache 语义 ——
//   - happy 首次调用 → repo.listEmojis 调用 1 次 + 返 4 项
//   - happy 缓存命中：第二次调用 → repo.listEmojis 调用次数仍为 1 (cache 有效)
//   - edge 失败不缓存：先失败 → 再调 repo 仍调用 (失败不污染 cache)
//
// MockEmojiRepository 放在测试文件内部 (private final class) ——
// 与 PetAppTests 内 MockAPIClient / InMemoryKeychainStore 同模式 (Dev Note #7 钦定).

import XCTest
@testable import PetApp

@MainActor
final class LoadEmojisUseCaseTests: XCTestCase {

    private let fixture: [EmojiConfig] = [
        EmojiConfig(code: "wave", name: "挥手", assetUrl: "https://example.com/wave.png", sortOrder: 1),
        EmojiConfig(code: "love", name: "爱心", assetUrl: "https://example.com/love.png", sortOrder: 2),
        EmojiConfig(code: "laugh", name: "大笑", assetUrl: "https://example.com/laugh.png", sortOrder: 3),
        EmojiConfig(code: "cry", name: "哭泣", assetUrl: "https://example.com/cry.png", sortOrder: 4)
    ]

    // MARK: - case#1 happy: 首次调用 → repo 调 1 次 + 返 4 项

    func test_execute_firstCall_hitsRepoOnce() async throws {
        let mockRepo = MockEmojiRepository()
        await mockRepo.setStubResult(.success(fixture))
        let useCase = DefaultLoadEmojisUseCase(repository: mockRepo)

        let emojis = try await useCase.execute()

        XCTAssertEqual(emojis.count, 4)
        XCTAssertEqual(emojis, fixture)
        let count = await mockRepo.callCount
        XCTAssertEqual(count, 1, "首次 execute 应调用 repo.listEmojis 1 次")
    }

    // MARK: - case#2 happy: 第二次调用命中 cache → repo 调用次数仍为 1

    func test_execute_secondCall_hitsCache_repoStillCalledOnce() async throws {
        let mockRepo = MockEmojiRepository()
        await mockRepo.setStubResult(.success(fixture))
        let useCase = DefaultLoadEmojisUseCase(repository: mockRepo)

        let first = try await useCase.execute()
        let second = try await useCase.execute()

        XCTAssertEqual(first, second, "两次返回值应严格相等 (Equatable)")
        let count = await mockRepo.callCount
        XCTAssertEqual(count, 1, "第二次 execute 应命中 cache, repo.listEmojis 调用次数仍为 1")
    }

    // MARK: - case#3 edge: 首次失败 → 抛错 + 不缓存 (再调 repo 仍被调用)

    func test_execute_firstFails_doesNotCache_subsequentSucceeds() async throws {
        let mockRepo = MockEmojiRepository()
        await mockRepo.setStubResult(.failure(APIError.network(underlying: URLError(.notConnectedToInternet))))
        let useCase = DefaultLoadEmojisUseCase(repository: mockRepo)

        // 第一次 execute 抛错
        do {
            _ = try await useCase.execute()
            XCTFail("首次应抛 APIError.network")
        } catch APIError.network {
            // expected
        } catch {
            XCTFail("意外错误: \(error)")
        }
        let countAfterFirst = await mockRepo.callCount
        XCTAssertEqual(countAfterFirst, 1)

        // 切 stub 为成功后再调 execute —— 应触发 repo 第二次调用 (说明失败未污染 cache)
        await mockRepo.setStubResult(.success(fixture))
        let emojis = try await useCase.execute()
        XCTAssertEqual(emojis.count, 4)
        let countAfterSecond = await mockRepo.callCount
        XCTAssertEqual(countAfterSecond, 2, "失败后再调 execute 应重新走 repo (cache 未污染)")
    }

    // MARK: - case#4 happy: 空列表也走 cache (server 永远返 [] 而非 null)

    func test_execute_emptyList_alsoCached() async throws {
        let mockRepo = MockEmojiRepository()
        await mockRepo.setStubResult(.success([]))
        let useCase = DefaultLoadEmojisUseCase(repository: mockRepo)

        let first = try await useCase.execute()
        let second = try await useCase.execute()

        XCTAssertTrue(first.isEmpty)
        XCTAssertTrue(second.isEmpty)
        let count = await mockRepo.callCount
        XCTAssertEqual(count, 1, "空列表也应缓存, 第二次 execute 不应再调 repo")
    }

    // MARK: - case#5 concurrency: 并发 caller miss path 也只调一次 repo (review round 1 P2 fix)
    //
    // Actor reentrancy: `execute()` 在 `await repository.listEmojis()` 处释放 isolation,
    // 两个并发 caller 都可能通过 `cache == nil` 检查后各自发 GET. 修复要求 inflightTask 兜底:
    // 第一个 caller 起 Task 存 inflightTask；后续 caller 看到 inflight 直接 `await task.value`
    // 共享同一次 repo 调用. 用 `GatedMockEmojiRepository` 模拟"慢 repo" —— 第一个 listEmojis 调用
    // 起 continuation 挂起，第二个 caller 进 useCase 后 (此时 inflight 已存)，再 resume continuation,
    // 验证 repo.callCount == 1 (single-flight 生效).

    func test_execute_concurrentMiss_singleFlight_repoCalledOnce() async throws {
        let gatedRepo = GatedMockEmojiRepository(result: .success(fixture))
        let useCase = DefaultLoadEmojisUseCase(repository: gatedRepo)

        // 起两个并发 caller —— 都会进 miss path. 第一个起 inflight Task；
        // 在 first await task.value 时让出，第二个 caller 进 actor 看到 inflight 后 await 同一 Task.
        async let firstTask = useCase.execute()
        async let secondTask = useCase.execute()

        // 等两边都进入 repo.listEmojis 之前的 await (或 first 进 await + second 看到 inflight).
        // 用 polling: gatedRepo.callCount 变 1 表示 first 已经发起 repo 调用 (inflight 已存),
        // 此时 second caller 必然走 inflight 复用路径 (或仍在排队 actor hop —— 同样不会发 repo).
        try await waitFor(timeoutMs: 2000) {
            await gatedRepo.callCount == 1
        }
        // resume repo continuation 让两边都拿到结果.
        await gatedRepo.resume()

        let first = try await firstTask
        let second = try await secondTask

        XCTAssertEqual(first, fixture)
        XCTAssertEqual(second, fixture)
        let callCount = await gatedRepo.callCount
        XCTAssertEqual(callCount, 1, "并发 miss path 应 single-flight, repo 仅被调 1 次")
    }

    // Helper: 异步条件 polling, 避免 sleep 写死时长 (与既有 test 哲学一致).
    private func waitFor(timeoutMs: Int, _ predicate: @Sendable () async -> Bool) async throws {
        let deadline = Date().addingTimeInterval(Double(timeoutMs) / 1000.0)
        while Date() < deadline {
            if await predicate() { return }
            try await Task.sleep(nanoseconds: 10_000_000) // 10ms
        }
        XCTFail("waitFor 超时 \(timeoutMs)ms")
    }
}

// MARK: - MockEmojiRepository (test-private actor)

/// MockEmojiRepository: actor 形态保护 callCount 字段; 模拟 EmojiRepositoryProtocol 行为
/// (Sendable + thread-safe; 与 PetAppTests 内 mock 同精神).
///
/// 用 actor 而非 class+lock: actor 自带串行, 与 LoadEmojisUseCase actor 跨 actor 边界调用时
/// callCount 读写自然 race-free. stub 通过 setStubResult(_:) 异步入口写入.
private actor MockEmojiRepository: EmojiRepositoryProtocol {
    private var stubResult: Result<[EmojiConfig], Error> = .failure(MockError.notStubbed)
    private(set) var callCount: Int = 0

    nonisolated init() {}

    func setStubResult(_ result: Result<[EmojiConfig], Error>) {
        self.stubResult = result
    }

    func listEmojis() async throws -> [EmojiConfig] {
        callCount += 1
        return try stubResult.get()
    }
}

// MARK: - GatedMockEmojiRepository (test-private actor, single-flight 测试用)

/// GatedMockEmojiRepository: `listEmojis()` 挂起在 continuation 上，直到外部调 `resume()` 才返结果.
/// 用于验证 review round 1 P2 fix —— 并发 caller miss path 时 inflightTask single-flight 生效.
///
/// callCount += 1 在 continuation suspend **之前**记入 —— 多并发 listEmojis 调用进来，
/// callCount 反映"真正发了几次 repo 调用". 修复正确时，callCount 应为 1 (两 caller 共享同一 Task);
/// 未修复时 callCount 会等于并发 caller 数 (actor reentrancy 让两边都进 miss path).
private actor GatedMockEmojiRepository: EmojiRepositoryProtocol {
    private let result: Result<[EmojiConfig], Error>
    private(set) var callCount: Int = 0
    private var continuation: CheckedContinuation<Void, Never>?

    init(result: Result<[EmojiConfig], Error>) {
        self.result = result
    }

    func resume() {
        continuation?.resume()
        continuation = nil
    }

    func listEmojis() async throws -> [EmojiConfig] {
        callCount += 1
        // 第一次调用挂起等 resume；后续调用不应发生 (single-flight 生效)，
        // 即使发生也走同一 continuation 路径 (此实现仅记 callCount).
        await withCheckedContinuation { (cont: CheckedContinuation<Void, Never>) in
            // 只存第一个 continuation；并发情况下若进来第二个 listEmojis,
            // 它会覆盖 cont —— 但单 single-flight 修复正确时不应有第二个进来.
            self.continuation = cont
        }
        return try result.get()
    }
}
