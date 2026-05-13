// PetStateEndpointsTests.swift
// Story 15.4 AC5: PetStateEndpoints 单元测试.
//
// 锁定 V1 §5.2 钦定的 endpoint 三字段：path / method / requiresAuth.
// 与 sibling RoomEndpointsTests 同模式（XCTest + 直接断言 endpoint 字段）.

import XCTest
@testable import PetApp

final class PetStateEndpointsTests: XCTestCase {

    // case#1 (AC5 Task 5.1.1): endpoint path / method / requiresAuth 字段断言.
    func testSync_endpoint_path_method_requiresAuth() {
        let request = PetStateSyncRequest(state: 2)  // .walk wireValue
        let endpoint = PetStateEndpoints.sync(request)

        XCTAssertEqual(endpoint.path, "/api/v1/pets/current/state-sync",
            "path 必须是 V1 §5.2 钦定的 /api/v1/pets/current/state-sync —— 含 /api/v1 前缀（host-only baseURL 契约）")
        XCTAssertEqual(endpoint.method, .post, "state-sync 必须是 POST（V1 §5.2 line 490）")
        XCTAssertTrue(endpoint.requiresAuth, "state-sync 必须 requiresAuth=true，由 APIClient 注入 token")
        XCTAssertNotNil(endpoint.body, "state-sync 必须带 request body（含 state 字段）")
    }
}
