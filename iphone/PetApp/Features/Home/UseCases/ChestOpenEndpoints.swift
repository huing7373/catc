// ChestOpenEndpoints.swift
// Story 21.3 AC1: V1 §7.2 POST /api/v1/chest/open endpoint 工厂；与 StepsEndpoints / PetStateEndpoints 同模式.
//
// path 必须**含** `/api/v1` 前缀（与 ChestEndpoints / StepsEndpoints 同模式 —— APIClient 用
// host-only baseURL，拼出的 URL 是 baseURL + endpoint.path）.
//
// 拆独立 enum 而非合到 ChestEndpoints：
// - ChestEndpoints.current() 是 GET（读型；Story 21.2 落地）
// - ChestOpenEndpoints.open() 是 POST（动作型，带 body）
// - 拆开让"读型 endpoint 工厂"与"动作型 endpoint 工厂"职责清晰；
//   与 既有 PingEndpoints / HomeEndpoints（读）vs StepsEndpoints / PetStateEndpoints（动作）拆分同精神.

import Foundation

public enum ChestOpenEndpoints {
    /// POST /api/v1/chest/open —— 开启宝箱（V1 §7.2）.
    /// requiresAuth=true：自动经过 APIClient 的 token 注入（Story 5.3）+ AuthBoundaryAPIClient 装饰器.
    public static func open(_ request: ChestOpenRequest) -> Endpoint {
        Endpoint(
            path: "/api/v1/chest/open",
            method: .post,
            body: AnyEncodable(request),
            requiresAuth: true
        )
    }
}
