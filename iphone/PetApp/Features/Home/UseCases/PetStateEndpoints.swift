// PetStateEndpoints.swift
// Story 15.4 AC1: V1 §5.2 POST /api/v1/pets/current/state-sync endpoint 工厂；
// 与 sibling StepsEndpoints / HomeEndpoints / AuthEndpoints 同模式.
//
// **位置**：`Features/Home/UseCases/`（与 sibling StepsEndpoints.swift 同目录）.
// 注：story 15.4 spec AC1 段曾写"Services/"，但既有仓库内所有 *Endpoints.swift 均在 UseCases/
// （PingEndpoints / HomeEndpoints / StepsEndpoints / AuthEndpoints 都在此目录），
// 故按实际 sibling 收敛位置；spec 的目录引用 outdated.
//
// path 必须**含** `/api/v1` 前缀（host-only baseURL 契约，与 sibling 同模式）.
// requiresAuth=true：自动经过 APIClient 的 token 注入 + AuthBoundaryAPIClient 装饰器.
//
// **不**接受 `idempotencyKey` header（V1 §5.2 line 500：state-sync 不消耗资产）.
// **不**带额外 query / 自定义 header.

import Foundation

public enum PetStateEndpoints {
    /// POST /api/v1/pets/current/state-sync —— 当前用户当前 pet 状态同步（V1 §5.2）.
    /// server 自查 default pet（请求体不带 petId）；返回 echoed state 用作 ack 信号.
    public static func sync(_ request: PetStateSyncRequest) -> Endpoint {
        Endpoint(
            path: "/api/v1/pets/current/state-sync",
            method: .post,
            body: AnyEncodable(request),
            requiresAuth: true
        )
    }
}
