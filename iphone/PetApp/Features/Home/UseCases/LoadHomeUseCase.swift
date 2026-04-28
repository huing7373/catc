// LoadHomeUseCase.swift
// Story 5.5 AC4: 首屏数据加载 UseCase（Epic 5 收尾 + iOS 架构 §6.2 钦定的 LoadHomeUseCase 落地点）.
//
// 流程：
//   1. 调 repo.loadHome() 拿 HomeResponse
//   2. HomeData(from: response) 转 domain 数据
//   3. 返回 HomeData（含 user / pet / stepAccount / chest / room）
//
// 错误处理：所有错误**原样**透传 throw；不在 UseCase 内吞错或转码.
//   - APIError.business(1001) → 装饰器层已 catch（Story 5.4 wrap）；理论不应抵达 UseCase
//   - APIError.business(1009) → server 任一聚合查询失败 → 透传 → ViewModel 走 ErrorPresenter 显示 RetryView
//   - APIError.unauthorized → 装饰器已重试过仍 401 → 透传 → ViewModel 同上
//   - APIError.missingCredentials → cold-start 路径未走通（keychain 空 / token 未写）→ 透传
//   - APIError.network → 透传 → ViewModel 同上
//   - APIError.decoding → server 返了不符合 schema 的数据（理论不应发生 —— V1 §4.1 行 16 schema 已冻结）
//                         → 透传 → ViewModel 同上（让用户重试或提示"App 需要更新"）
//   - APIError.decoding(HomeDataDecodingError) → 客户端解 wire DTO 时遇未知 enum 值（同 schema drift 信号）
//                         → 透传 → AlertOverlay "数据异常，请稍后重试"（mapper 钦定）
//                         详见 docs/lessons/2026-04-27-home-data-fail-fast-on-unknown-enum.md
//
// 不在本 story 范围（设计选择）：
//   - 不直接接 ErrorPresenter（让 UseCase 可被未来非 UI 场景 / 后台刷新复用）
//   - 不做 retry / 指数退避（401 已被装饰器一次性 retry；其它错误由用户走 RetryView 主动重试）
//   - 不缓存上次结果（节点 2 阶段每次启动都拉新；节点 4 后引入缓存归未来 RefreshHomeUseCase）

import Foundation

public protocol LoadHomeUseCaseProtocol: Sendable {
    /// 调 GET /api/v1/home 拿首屏数据并转 domain.
    /// - Returns: HomeData（含 user / pet / stepAccount / chest / room）
    /// - Throws: APIError（全部 case 原样透传）
    func execute() async throws -> HomeData
}

public struct DefaultLoadHomeUseCase: LoadHomeUseCaseProtocol {
    private let repository: HomeRepositoryProtocol

    public init(repository: HomeRepositoryProtocol) {
        self.repository = repository
    }

    public func execute() async throws -> HomeData {
        let response = try await repository.loadHome()
        // round 6 [P2] fix: HomeData(from:) 改 throws —— 未知 enum 值会抛 APIError.decoding,
        // 由本 UseCase 透传给 ViewModel / RootView bootstrapStep1, 触发 AlertOverlay fail-fast 而非
        // 把未知 pet.currentState / chest.status silently coerce 成 .rest / .counting.
        return try HomeData(from: response)
    }
}
