// StepsEndpoints.swift
// Story 8.5 AC2: V1 §6.1 POST /api/v1/steps/sync endpoint 工厂；与 AuthEndpoints / HomeEndpoints 同模式.
//
// 提为独立 enum：避免 StepRepository 内 inline `Endpoint(...)` 字面量散落；
// 当 V1 §6.1 path / requiresAuth 改时（理论上不会，已冻结），仅改本文件一处.
//
// path 必须**含** `/api/v1` 前缀（与 AuthEndpoints / HomeEndpoints 同模式 —— APIClient 用
// host-only baseURL，拼出的 URL 是 baseURL + endpoint.path）.

import Foundation

public enum StepsEndpoints {
    /// POST /api/v1/steps/sync —— 步数同步（V1 §6.1）.
    /// requiresAuth=true：自动经过 APIClient 的 token 注入（Story 5.3）+ AuthBoundaryAPIClient 装饰器.
    public static func sync(_ request: StepsSyncRequest) -> Endpoint {
        Endpoint(
            path: "/api/v1/steps/sync",
            method: .post,
            body: AnyEncodable(request),
            requiresAuth: true
        )
    }
}
