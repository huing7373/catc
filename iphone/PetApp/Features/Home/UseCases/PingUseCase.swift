// PingUseCase.swift
// Story 2.5 AC3：负责一次性拉 `/ping` + `/version` 并组装成 `PingResult`。
//
// 三态语义（与 PingResult 一致）：
//   - ping OK + version OK → (reachable: true, serverCommit: <非空>)
//   - ping OK + version 失败 → (reachable: true, serverCommit: nil)（部分降级）
//   - ping 失败 → (reachable: false, serverCommit: nil)（version 调用被短路跳过）
//
// 注意：execute() **永远不抛错** —— 任何失败都被映射成 PingResult 的某个负态。
//   1. 主界面 versionLabel 是装饰性元素，错误不应阻断渲染（区别于 GuestLoginUseCase 等关键 UseCase）。
//   2. 调用方（HomeViewModel）只关心三态文案，不关心是 401 / 网络 / 解码哪种失败
//      （→ Story 2.6 ErrorPresenter 才关心错误类型）。
//
// 不抛错并不意味着"吞错"：实装中可在未来 Story 2.7 logger 接入后把底层错误透传给 logger；
// 当前 MVP 阶段 logger 还没接，先不在 UseCase 里 print，避免遗留 print 到 commit。

import Foundation

/// PingUseCase 协议：让 HomeViewModel 可以注入 mock 而无需依赖具体实现。
public protocol PingUseCaseProtocol: Sendable {
    func execute() async -> PingResult
}

/// 默认实现：注入 `APIClientProtocol`，按 happy / partial-degrade / offline 三态返回。
public struct DefaultPingUseCase: PingUseCaseProtocol {
    private let client: APIClientProtocol

    public init(client: APIClientProtocol) {
        self.client = client
    }

    public func execute() async -> PingResult {
        // 步骤 1：先调 ping。失败立即返回 offline。
        do {
            let _: PingResponse = try await client.request(PingEndpoints.ping())
        } catch {
            return PingResult(reachable: false, serverCommit: nil)
        }

        // 步骤 2：ping OK，再调 version。失败时 reachable 仍 true（部分降级）。
        do {
            let v: VersionResponse = try await client.request(PingEndpoints.version())
            return PingResult(reachable: true, serverCommit: v.commit)
        } catch {
            return PingResult(reachable: true, serverCommit: nil)
        }
    }
}
