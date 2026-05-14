// EmojiPanelViewModelTests.swift
// Story 18.1 AC7: EmojiPanelViewModel 单测覆盖 (≥3 case, MockLoadEmojisUseCase).
//
// 测试目标：验证 state 切换 + mapError 文案 + retry 路径 ——
//   - happy load 成功 → state == .loaded(4 项)
//   - edge load 失败 (APIError.network) → state == .failed("网络异常，请检查后重试")
//   - happy retry 后成功 → state == .loaded
//
// MockLoadEmojisUseCase 放在测试文件内部 (private actor) —— 与 LoadEmojisUseCaseTests 同精神.

import XCTest
@testable import PetApp

@MainActor
final class EmojiPanelViewModelTests: XCTestCase {

    private let fixture: [EmojiConfig] = [
        EmojiConfig(code: "wave", name: "挥手", assetUrl: "https://example.com/wave.png", sortOrder: 1),
        EmojiConfig(code: "love", name: "爱心", assetUrl: "https://example.com/love.png", sortOrder: 2),
        EmojiConfig(code: "laugh", name: "大笑", assetUrl: "https://example.com/laugh.png", sortOrder: 3),
        EmojiConfig(code: "cry", name: "哭泣", assetUrl: "https://example.com/cry.png", sortOrder: 4)
    ]

    // MARK: - case#1 happy: load 成功 → state == .loaded

    func test_load_success_setsLoadedState() async throws {
        let mockUseCase = MockLoadEmojisUseCase()
        await mockUseCase.setStubResult(.success(fixture))
        let vm = EmojiPanelViewModel(useCase: mockUseCase)

        XCTAssertEqual(vm.state, .loading, "init state 应为 .loading")
        await vm.load()
        XCTAssertEqual(vm.state, .loaded(fixture))
    }

    // MARK: - case#2 edge: load 失败 (network) → state == .failed("网络异常...")

    func test_load_networkError_setsFailedWithMessage() async {
        let mockUseCase = MockLoadEmojisUseCase()
        await mockUseCase.setStubResult(.failure(APIError.network(underlying: URLError(.notConnectedToInternet))))
        let vm = EmojiPanelViewModel(useCase: mockUseCase)

        await vm.load()

        XCTAssertEqual(vm.state, .failed("网络异常，请检查后重试"))
    }

    // MARK: - case#3 happy: retry 后成功 → state == .loaded

    func test_retry_afterFailure_setsLoadedState() async throws {
        let mockUseCase = MockLoadEmojisUseCase()
        await mockUseCase.setStubResult(.failure(APIError.network(underlying: URLError(.timedOut))))
        let vm = EmojiPanelViewModel(useCase: mockUseCase)

        // 首次 load 失败
        await vm.load()
        XCTAssertEqual(vm.state, .failed("网络异常，请检查后重试"))

        // 切 stub 为成功后 retry
        await mockUseCase.setStubResult(.success(fixture))
        await vm.retry()
        XCTAssertEqual(vm.state, .loaded(fixture))
    }

    // MARK: - case#4 edge: business 1009 错误 → state == .failed("服务器繁忙...")

    func test_load_business1009_setsServerBusyMessage() async {
        let mockUseCase = MockLoadEmojisUseCase()
        await mockUseCase.setStubResult(.failure(APIError.business(code: 1009, message: "DB 异常", requestId: "req_1")))
        let vm = EmojiPanelViewModel(useCase: mockUseCase)

        await vm.load()

        XCTAssertEqual(vm.state, .failed("服务器繁忙，请稍后再试"))
    }

    // MARK: - case#5 edge: unauthorized → state == .failed("登录已失效...")

    func test_load_unauthorized_setsLoginExpiredMessage() async {
        let mockUseCase = MockLoadEmojisUseCase()
        await mockUseCase.setStubResult(.failure(APIError.unauthorized))
        let vm = EmojiPanelViewModel(useCase: mockUseCase)

        await vm.load()

        XCTAssertEqual(vm.state, .failed("登录已失效，请重启 App"))
    }

    // MARK: - case#6 edge: decoding 错误 → state == .failed("数据解析失败...")

    func test_load_decodingError_setsDecodingFailedMessage() async {
        let mockUseCase = MockLoadEmojisUseCase()
        let underlying = NSError(domain: "test", code: 0)
        await mockUseCase.setStubResult(.failure(APIError.decoding(underlying: underlying)))
        let vm = EmojiPanelViewModel(useCase: mockUseCase)

        await vm.load()

        XCTAssertEqual(vm.state, .failed("数据解析失败，请重试"))
    }
}

// MARK: - MockLoadEmojisUseCase (test-private actor)

private actor MockLoadEmojisUseCase: LoadEmojisUseCaseProtocol {
    private var stubResult: Result<[EmojiConfig], Error> = .failure(MockError.notStubbed)

    nonisolated init() {}

    func setStubResult(_ result: Result<[EmojiConfig], Error>) {
        self.stubResult = result
    }

    func execute() async throws -> [EmojiConfig] {
        return try stubResult.get()
    }
}
