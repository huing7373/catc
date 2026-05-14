// EmojisEndpoints.swift
// Story 18.1 AC1: GET /api/v1/emojis endpoint 工厂.
//
// 与 HomeEndpoints / RoomEndpoints 同模式：path 必含 `/api/v1` 前缀
// (lesson 2026-04-26-baseurl-host-only-contract.md —— APIClient 用 host-only baseURL,
// 拼出的 URL = baseURL + endpoint.path = "http://localhost:8080" + "/api/v1/emojis").
//
// requiresAuth=true：自动经 APIClient token 注入 + AuthBoundaryAPIClient 装饰器拦 401
// (V1 §11.1 元信息表"认证：需要 Bearer token"钦定).
//
// body=nil：GET 无 body (V1 §11.1 "不分页 / 不接受 query 参数").

import Foundation

public enum EmojisEndpoints {
    /// GET /api/v1/emojis —— 表情列表 (V1 §11.1).
    /// response data: `{items: [{code, name, assetUrl, sortOrder}]}`
    /// server 端排序：`ORDER BY sort_order ASC, id ASC` —— client 接收后**不**需要二次排序.
    public static func listEmojis() -> Endpoint {
        Endpoint(path: "/api/v1/emojis", method: .get, body: nil, requiresAuth: true)
    }
}
