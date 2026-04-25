// PingEndpoints.swift
// Story 2.5 AC2：`/ping` 与 `/version` 的 Endpoint 工厂 + VersionResponse 解码模型。
//
// 这两个端点是**运维 / 探活端点**，server 注册在根路径（**不**走 `/api/v1` 前缀）：
//   - GET /ping     → envelope.data = {} (空对象)，envelope.message = "pong"
//   - GET /version  → envelope.data = {commit: String, builtAt: String}
//
// 详情见 server `internal/app/bootstrap/router.go:30-46` 与 server story 1.2 / 1.4。
//
// 关于 baseURL 的拼接约定（本 story Dev Note #1）：
// 本 story 的 APIClient 用 host-only baseURL（如 `http://localhost:8080`，**不**含 `/api/v1` 前缀），
// endpoint.path 自带完整路径（`/ping` / `/version`）。
// 后续 Epic 4+ 的业务 endpoint 同样自带 `/api/v1/...` 前缀。
// 这与 Story 2.4 doc comment 中"baseURL 含 /api/v1"的早期假设有出入；本约定后向覆盖。

import Foundation

/// `/ping` 与 `/version` 的 Endpoint 工厂。
public enum PingEndpoints {
    public static func ping() -> Endpoint {
        Endpoint(path: "/ping", method: .get, body: nil, requiresAuth: false)
    }

    public static func version() -> Endpoint {
        Endpoint(path: "/version", method: .get, body: nil, requiresAuth: false)
    }
}

/// `/ping` 响应的 data 解码模型。data 是空对象 `{}`，复用 Story 2.4 的 `Empty` 占位。
public typealias PingResponse = Empty

/// `/version` 响应的 data 解码模型，对齐 server 端 `VersionResponse` struct（小驼峰，
/// 见 server/internal/app/http/handler/version_handler.go:12-15）。
public struct VersionResponse: Decodable, Equatable {
    public let commit: String
    public let builtAt: String

    public init(commit: String, builtAt: String) {
        self.commit = commit
        self.builtAt = builtAt
    }
}
