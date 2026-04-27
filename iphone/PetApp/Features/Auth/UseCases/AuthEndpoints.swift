// AuthEndpoints.swift
// Story 5.2 AC2: /auth/* 子组的 Endpoint 工厂。本 story 仅 guestLogin；
// 后续 future epic 加 BindWechat 等，沿用同模式。
//
// 关键约束：
// - path: "/api/v1/auth/guest-login" —— 必须**含** `/api/v1` 前缀（与 PingEndpoints 的 host-only baseURL
//   契约配套；APIClient 拼出的 URL 是 baseURL + endpoint.path = "http://localhost:8080" + "/api/v1/auth/guest-login"）
// - method: .post
// - body: AnyEncodable(request)（GuestLoginRequest 包装；APIClient 用 JSONEncoder 编码）
// - requiresAuth: false —— 登录接口本身不需要 token；Story 5.3 interceptor 落地后按本字段决策

import Foundation

public enum AuthEndpoints {
    public static func guestLogin(request: GuestLoginRequest) -> Endpoint {
        Endpoint(
            path: "/api/v1/auth/guest-login",
            method: .post,
            body: AnyEncodable(request),
            requiresAuth: false
        )
    }
}
