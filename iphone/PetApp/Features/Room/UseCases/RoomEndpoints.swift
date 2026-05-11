// RoomEndpoints.swift
// Story 12.7 AC4: 房间 REST 接口 endpoint 工厂（POST /rooms / POST /rooms/{id}/join / POST /rooms/{id}/leave）.
//
// 与 HomeEndpoints / AuthEndpoints 同模式：path 必含 `/api/v1` 前缀；body 走 `Data("{}".utf8)` 包成 AnyEncodable
// （V1 §10.1 / §10.4 / §10.5 钦定 request body 是空对象 `{}`；不能 nil 或省略，否则 server 解 JSON 失败）.
//
// 所有 3 个 endpoint requiresAuth=true：调用方携带 Bearer token；AuthBoundaryAPIClient 装饰器在 401 时
// 自动触发全局 cold-start（ADR-0008 v2）.

import Foundation

public enum RoomEndpoints {
    /// POST /api/v1/rooms —— 创建房间（V1 §10.1）.
    /// requiresAuth=true：服务端依据 token 解析创建者 userId，无 body 字段（空对象 `{}`）.
    public static func createRoom() -> Endpoint {
        Endpoint(
            path: "/api/v1/rooms",
            method: .post,
            body: AnyEncodable(EmptyObjectBody()),
            requiresAuth: true
        )
    }

    /// POST /api/v1/rooms/{roomId}/join —— 加入房间（V1 §10.4）.
    /// roomId 由 caller 传入（BIGINT 字符串化；长度 1..20）.
    ///
    /// **fix-review round 1 + 7 P2（Story 12.7）**：roomId 必须 percent-encode 后再插入 path.
    /// join flow 容许任意输入依赖 server 返回 1002 "房间号格式不合法"；但若 raw input 含 URL reserved
    /// 字符（`/` `?` `#` `..`）或 pre-escaped 序列（如 `%2F`）直接 string interpolate 会改变 request
    /// path/query，server-side 校验永远拿不到该输入 → 1002 业务错误处理路径走不通（client 看到的是
    /// transport 错误 / 路由到错误 endpoint）.
    /// 用 `roomIdPathAllowed` set（URLPathAllowed 减去 `/?#%`）让所有 URL-meaningful 字符被 escape.
    public static func joinRoom(roomId: String) -> Endpoint {
        Endpoint(
            path: "/api/v1/rooms/\(escapePathSegment(roomId))/join",
            method: .post,
            body: AnyEncodable(EmptyObjectBody()),
            requiresAuth: true
        )
    }

    /// POST /api/v1/rooms/{roomId}/leave —— 退出房间（V1 §10.5）.
    /// roomId 由 caller 传入（通常是 appState.currentRoomId）；body 仍是空对象.
    ///
    /// **fix-review round 1 P2（Story 12.7）**：同 joinRoom，roomId percent-encode 后插入 path
    /// （leave 路径同样可能被攻击 / proxy 改写后承接非法 input；统一 escape 不引入回归）.
    public static func leaveRoom(roomId: String) -> Endpoint {
        Endpoint(
            path: "/api/v1/rooms/\(escapePathSegment(roomId))/leave",
            method: .post,
            body: AnyEncodable(EmptyObjectBody()),
            requiresAuth: true
        )
    }

    // MARK: - URL path encoding helper

    /// percent-encode 单个 path segment：用 `urlPathAllowed` 减去 `/?#%`（前三个在 path 内会改变
    /// URL 语义 —— `/` 切 path、`?` 起 query、`#` 起 fragment；`%` 是 percent-encoding 引导符 ——
    /// 若 raw input 已含 `%2F` / `%3F` 等 pre-escaped 序列，**不**显式 escape `%` 会让该序列原样
    /// 透到 server → server decode 后得到 `/` `?` 等字符 → 路由错位绕过 1002 校验.
    /// 把 `%` 一起 subtract，client 会把每个 `%` 转成 `%25`，server decode 后看到字面 `%2F` 字符串,
    /// 转交 server-side 1002 校验.
    ///
    /// reserved 字符 `..` 是合法 path char 但语义上做 path traversal ——
    /// `addingPercentEncoding` 不会自动处理 `..`，server 端必须自行 reject path traversal 输入；
    /// client 这里只负责保证 raw input 不被分隔字符 hijack.
    ///
    /// fallback: encoding 失败（极小概率：surrogate pair 异常等）返回原值 ——
    /// 让 server 端正常返回 1002 business error 而不是 client 自己 panic.
    private static let roomIdPathAllowed: CharacterSet = {
        var allowed = CharacterSet.urlPathAllowed
        // urlPathAllowed 内**不**含 `?` `#`（这两个本就是 query/fragment 起始符 → percent-encoding
        // 已生效），**含** `/` 和 `%` —— 必须显式 subtract：
        //   - `/` 不 subtract → raw input 含 `/` 时仍直通 path.
        //   - `%` 不 subtract → pre-escaped 输入（如 `AA%2FBB`）原样透到 server, decode 后路由错位.
        // fix-review round 7 P2（Story 12.7）锁定该行为.
        allowed.remove(charactersIn: "/%")
        return allowed
    }()

    /// 单元测试可见性：让 `RoomEndpointsTests` 直接断言 escape 行为而无需通过整个 endpoint 反推 path.
    static func escapePathSegment(_ segment: String) -> String {
        segment.addingPercentEncoding(withAllowedCharacters: roomIdPathAllowed) ?? segment
    }
}

/// 空对象 body marker —— Encodable 序列化为 `{}`.
/// V1 §10.1 / §10.4 / §10.5 钦定 request body 是空对象（不能 nil；server 端 JSONDecoder 拒绝缺 body 的 POST）.
private struct EmptyObjectBody: Encodable {}
