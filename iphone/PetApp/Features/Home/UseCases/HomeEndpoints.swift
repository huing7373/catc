// HomeEndpoints.swift
// Story 5.5 AC3: GET /api/v1/home endpoint 工厂；与 AuthEndpoints 同模式.
//
// 提为独立 enum：避免 HomeRepository 内 inline `Endpoint(...)` 字面量散落；
// 当 V1 §5.1 path / requiresAuth 改时（理论上不会，已冻结），仅改本文件一处.
//
// path 必须**含** `/api/v1` 前缀（与 AuthEndpoints 同模式 —— APIClient 用 host-only baseURL，
// 拼出的 URL 是 baseURL + endpoint.path = "http://localhost:8080" + "/api/v1/home"）.

import Foundation

public enum HomeEndpoints {
    /// GET /api/v1/home —— 首屏聚合数据（user + pet + stepAccount + chest + room）.
    /// requiresAuth=true：自动经过 APIClient 的 token 注入（Story 5.3）+ AuthBoundaryAPIClient
    /// 装饰器（ADR-0008 v2 → 401 自动触发 cold-start 重跑 bootstrap）.
    public static func loadHome() -> Endpoint {
        Endpoint(path: "/api/v1/home", method: .get, body: nil, requiresAuth: true)
    }
}
