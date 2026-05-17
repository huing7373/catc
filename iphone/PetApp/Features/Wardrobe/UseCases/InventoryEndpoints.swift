// InventoryEndpoints.swift
// Story 24.2 AC2: GET /api/v1/cosmetics/inventory endpoint 工厂.
//
// 与 EmojisEndpoints / ChestEndpoints 同模式：path 必含 `/api/v1` 前缀
// (lesson 2026-04-26-baseurl-host-only-contract.md —— APIClient 用 host-only baseURL,
// 拼出 URL = baseURL + endpoint.path = "http://localhost:8080" + "/api/v1/cosmetics/inventory").
//
// requiresAuth=true：经 AuthBoundaryAPIClient 自动注 Bearer token + 拦 401
// (V1 §8.2 接口元信息「认证：需要 Bearer token」钦定).
//
// body=nil + 无 query 参数：V1 §8.2「Query 参数：无」「无请求体（GET 接口）」——
// 筛选纯客户端（iOS Story 24.3 在 client 侧做），本接口不接受任何 query string.

import Foundation

public enum InventoryEndpoints {
    /// GET /api/v1/cosmetics/inventory —— 背包聚合 + 实例列表 (V1 §8.2).
    /// response data: `{groups: [{cosmeticItemId, name, slot, rarity, iconUrl, assetUrl, count, instances}]}`
    /// server 端排序：groups `ORDER BY rarity ASC, slot ASC, cosmeticItemId ASC` +
    /// instances `ORDER BY userCosmeticItemId ASC`（V1 §8.2 步骤 5 两级确定性全序）——
    /// client 接收后**不**需要二次排序.
    public static func inventory() -> Endpoint {
        Endpoint(path: "/api/v1/cosmetics/inventory", method: .get, body: nil, requiresAuth: true)
    }
}
