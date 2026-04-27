// HomeNicknameResolverTests.swift
// Story 5.2 codex round 1 [P1] fix 验证：
// HomeView 在 SessionStore 注入路径下，nickname 应来自 SessionStore.session.user.nickname,
// session = nil 时回退到 fallback（viewModel.nickname）.
//
// 详见 docs/lessons/2026-04-27-sessionstore-home-nickname-source-of-truth.md.

import XCTest
@testable import PetApp

@MainActor
final class HomeNicknameResolverTests: XCTestCase {

    // MARK: - happy: session 注入后优先显示 session 中的 nickname

    func testResolveReturnsSessionNicknameWhenSessionPresent() {
        let user = UserProfile(
            id: "1234567890",
            nickname: "用户1234567890",
            avatarUrl: "",
            hasBoundWechat: false
        )
        let pet = PetProfile(id: "p1", petType: 1, name: "默认小猫")
        let session = SessionState(user: user, pet: pet)

        let result = HomeNicknameResolver.resolve(session: session, fallback: "用户1001")

        XCTAssertEqual(
            result,
            "用户1234567890",
            "session 非 nil 时必须返回 session.user.nickname，否则切换 guestUid 时主界面显示错身份"
        )
    }

    // MARK: - fallback: session 为 nil 时回退到 viewModel 默认值

    func testResolveReturnsFallbackWhenSessionIsNil() {
        let result = HomeNicknameResolver.resolve(session: nil, fallback: "用户1001")

        XCTAssertEqual(
            result,
            "用户1001",
            "session 为 nil（启动早期 / Reset 后 / UITest skip-guest-login）时必须 fallback，保持向后兼容"
        )
    }

    // MARK: - edge: session 中 nickname 为空字符串时仍按 session 显示（不再 fallback）

    func testResolveReturnsEmptyNicknameVerbatimWhenSessionPresent() {
        // 服务端契约（V1 §4.1）保证 nickname 非空（自动生成 "用户{id}"），但 client 不假设；
        // 即便 server 极端情况下下发空串，行为应保持"以 session 为准"——
        // 反之若 fallback 到 "用户1001"，反而误导用户以为身份没切换.
        let user = UserProfile(
            id: "9",
            nickname: "",
            avatarUrl: "",
            hasBoundWechat: false
        )
        let pet = PetProfile(id: "p9", petType: 1, name: "默认小猫")
        let session = SessionState(user: user, pet: pet)

        let result = HomeNicknameResolver.resolve(session: session, fallback: "用户1001")

        XCTAssertEqual(
            result,
            "",
            "session 非 nil 时，即便 nickname 是空串也按 session 显示——以 server 为准的纪律"
        )
    }
}
