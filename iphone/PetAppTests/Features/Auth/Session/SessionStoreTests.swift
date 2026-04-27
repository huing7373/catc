// SessionStoreTests.swift
// Story 5.2 AC10: SessionStore 基础行为测试.
//
// SessionStore 是 in-memory observable state holder，无 mock 价值 —— 直接 new instance 即可.
// `@MainActor` test class：SessionStore 是 @MainActor class，写入 @Published 必须 main actor.

import XCTest
@testable import PetApp

@MainActor
final class SessionStoreTests: XCTestCase {

    private func makeSession(userId: String = "1001") -> SessionState {
        SessionState(
            user: UserProfile(id: userId, nickname: "用户\(userId)", avatarUrl: "", hasBoundWechat: false),
            pet: PetProfile(id: "2001", petType: 1, name: "默认小猫")
        )
    }

    // MARK: - case#1: 初始 session 为 nil

    func testInitialSessionIsNil() {
        let store = SessionStore()
        XCTAssertNil(store.session, "刚 init 的 SessionStore.session 应为 nil（未登录）")
    }

    // MARK: - case#2: updateSession 写入

    func testUpdateSessionStoresValue() {
        let store = SessionStore()
        let state = makeSession()

        store.updateSession(state)

        XCTAssertEqual(store.session, state, "updateSession 后 store.session 应等于传入的 state")
    }

    // MARK: - case#3: clear 清空

    func testClearResetsSessionToNil() {
        let store = SessionStore()
        store.updateSession(makeSession())
        XCTAssertNotNil(store.session, "前置：updateSession 后 session 非 nil")

        store.clear()

        XCTAssertNil(store.session, "clear() 后 session 应为 nil")
    }

    // MARK: - case#4: updateSession 二次覆盖

    func testUpdateSessionOverwritesPrevious() {
        let store = SessionStore()
        store.updateSession(makeSession(userId: "1001"))
        store.updateSession(makeSession(userId: "9999"))

        XCTAssertEqual(store.session?.user.id, "9999", "二次 updateSession 应覆盖前次值")
    }
}
