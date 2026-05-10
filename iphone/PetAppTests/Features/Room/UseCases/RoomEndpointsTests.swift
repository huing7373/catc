// RoomEndpointsTests.swift
// Story 12.7 fix-review round 1 P2 回归：roomId percent-encoding（防 URL reserved 字符 hijack path/query）.
//
// 锚定问题：旧实装 `path: "/api/v1/rooms/\(roomId)/join"` 直 string interpolate raw input；
// 输入含 `/` `?` `#` 等 reserved 字符时，request URL 会改变 path/query → server-side 1002
// "房间号格式不合法" 业务错误处理路径走不通（client 端看到的是 transport error / 路由到错误 endpoint）.
//
// 修复：用 `urlPathAllowed` 减去 `/?#` 字符集做 percent-encoding；本 tests 锁住该行为.

import XCTest
@testable import PetApp

final class RoomEndpointsTests: XCTestCase {

    // MARK: - happy path: 普通 roomId 不应被改动

    func testJoinRoomNormalRoomIdNotEscaped() {
        let endpoint = RoomEndpoints.joinRoom(roomId: "3001")
        XCTAssertEqual(endpoint.path, "/api/v1/rooms/3001/join",
                       "纯数字 roomId 不应被 percent-encode")
    }

    func testLeaveRoomNormalRoomIdNotEscaped() {
        let endpoint = RoomEndpoints.leaveRoom(roomId: "9876543210")
        XCTAssertEqual(endpoint.path, "/api/v1/rooms/9876543210/leave",
                       "纯数字 roomId 不应被 percent-encode")
    }

    // MARK: - escape path: URL reserved 字符必须被 percent-encode

    /// `/` 是 path 分隔符 —— 不 escape 会让 input "AA/BB" 变成 path component 之一 →
    /// server 收到的 path = `/api/v1/rooms/AA/BB/join` 完全错位.
    func testJoinRoomSlashIsEscaped() {
        let endpoint = RoomEndpoints.joinRoom(roomId: "AA/BB")
        XCTAssertEqual(endpoint.path, "/api/v1/rooms/AA%2FBB/join",
                       "raw `/` 必须被 escape 成 %2F；旧实装会让 server 收到 /api/v1/rooms/AA/BB/join 错位")
    }

    /// `?` 起 query —— 不 escape 会让 input "1234?evil=1" 触发 server 收到 query 参数（path 变短）.
    func testJoinRoomQuestionMarkIsEscaped() {
        let endpoint = RoomEndpoints.joinRoom(roomId: "1234?evil=1")
        XCTAssertEqual(endpoint.path, "/api/v1/rooms/1234%3Fevil=1/join",
                       "raw `?` 必须被 escape 成 %3F；旧实装会让 server 收到 query string `evil=1` + path 变成 /api/v1/rooms/1234")
    }

    /// `#` 起 fragment —— fragment 在 HTTP 客户端通常不发到 server，整个 ?#xxx 都会被切掉.
    func testJoinRoomHashIsEscaped() {
        let endpoint = RoomEndpoints.joinRoom(roomId: "1234#frag")
        XCTAssertEqual(endpoint.path, "/api/v1/rooms/1234%23frag/join",
                       "raw `#` 必须被 escape 成 %23；旧实装会让 client URL 把整个 #frag 视为 fragment 不发送")
    }

    /// leave 路径同样需要 escape（roomId 来源是 appState.currentRoomId，正常情况不会含 reserved 字符,
    /// 但防御性 escape 不引入回归 + server 端 leave 的 6004 路径同样依赖 path 正确）.
    func testLeaveRoomSlashIsEscaped() {
        let endpoint = RoomEndpoints.leaveRoom(roomId: "AA/BB")
        XCTAssertEqual(endpoint.path, "/api/v1/rooms/AA%2FBB/leave",
                       "leave 路径 raw `/` 必须被 escape；与 join 同精神")
    }

    // MARK: - escape helper: 直接断言 helper 行为（让回归测试更显式）

    func testEscapePathSegmentSlashHashQuestionMark() {
        XCTAssertEqual(RoomEndpoints.escapePathSegment("a/b"), "a%2Fb", "/ → %2F")
        XCTAssertEqual(RoomEndpoints.escapePathSegment("a?b"), "a%3Fb", "? → %3F")
        XCTAssertEqual(RoomEndpoints.escapePathSegment("a#b"), "a%23b", "# → %23")
        XCTAssertEqual(RoomEndpoints.escapePathSegment("normal"), "normal", "纯字母 ASCII 不变")
        XCTAssertEqual(RoomEndpoints.escapePathSegment("3001"), "3001", "纯数字不变")
    }

    /// `..` 是合法 path char，addingPercentEncoding 不会处理 path traversal 语义层面的攻击；
    /// 这是 server 端职责（spec 已约束）—— client 这里只保证 raw input 不被 reserved 字符 hijack.
    /// 本 case 锁定预期：`..` 不会被 client escape，让 server 端拿到原文做合法性判定.
    func testEscapePathSegmentDotDotPassesThrough() {
        XCTAssertEqual(RoomEndpoints.escapePathSegment(".."), "..",
                       "`..` 不被 client escape；server 端做 path traversal 校验（spec 约束）")
    }
}
