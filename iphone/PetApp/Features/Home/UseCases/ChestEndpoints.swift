// ChestEndpoints.swift
// Story 21.2 AC1: GET /api/v1/chest/current endpoint 工厂；与 HomeEndpoints / PetStateEndpoints 同模式.
//
// 提为独立 enum：避免 ChestRepository 内 inline `Endpoint(...)` 字面量散落；
// 当 V1 §7.1 path / requiresAuth 改时（理论上不会，已冻结），仅改本文件一处.
//
// path 必须**含** `/api/v1` 前缀（与 HomeEndpoints 同模式 —— APIClient 用 host-only baseURL,
// 拼出的 URL 是 baseURL + endpoint.path = "http://localhost:8080" + "/api/v1/chest/current"）.

import Foundation

public enum ChestEndpoints {
    /// GET /api/v1/chest/current —— 当前宝箱状态查询（V1 §7.1）.
    /// requiresAuth=true：自动经过 APIClient 的 token 注入（Story 5.3）+ AuthBoundaryAPIClient
    /// 装饰器（ADR-0008 v2 → 401 自动触发 cold-start 重跑 bootstrap）.
    public static func current() -> Endpoint {
        Endpoint(path: "/api/v1/chest/current", method: .get, body: nil, requiresAuth: true)
    }
}
