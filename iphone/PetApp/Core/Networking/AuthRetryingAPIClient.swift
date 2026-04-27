// AuthRetryingAPIClient.swift
// Story 5.4 AC2: APIClient decorator —— 拦 APIError.unauthorized 触发静默重登 + 重试一次.
//
// 设计选择（decorator pattern）：
//   - 不修改 APIClient 主体（保留 Story 5.3 落地的 token 注入决策树）
//   - 在 APIClientProtocol 之外加一层 wrap —— 业务层（DefaultAuthRepository / 未来 DefaultHomeRepository 等）
//     拿到的是包装后的 APIClientProtocol，自动获得静默重登能力，零改动
//   - 让单测可以独立验证装饰器逻辑（mock inner APIClient + mock coordinator）
//
// 拦截契约：
//   1. inner.request(endpoint) success → return（不触发重登）
//   2. inner.request(endpoint) throw .unauthorized + endpoint.requiresAuth == true →
//      a. coordinator.relogin() 拿新 token（多并发请求 coalesce 到同一次重登）
//      b. inner.request(endpoint) 重试一次
//      c. 重试结果 success → return；重试结果 throw → 直接抛上去（**不**再二次重登）
//   3. inner.request(endpoint) throw .unauthorized + endpoint.requiresAuth == false →
//      直接抛上去（如 /auth/guest-login 自己 401 → 不能用自己救自己）
//   4. inner.request(endpoint) throw 其它 APIError（.network / .business / .decoding）→
//      直接抛上去（不在重登职责内）
//
// 与 Story 5.3 buildURLRequest 阶段抛 .unauthorized 的关系：
//   - buildURLRequest 阶段抛 .unauthorized = 本地无 token 或 keychain 配置错（key 拼错 / 沙箱权限）
//     → 本装饰器**会**把它当 .unauthorized 处理，触发一次 coordinator.relogin()
//     → 重登成功后重试一次 → 重试时 buildURLRequest 又会读 keychain（刚被重登写过新 token），通常成功
//     → 例外：如果 inject 给 APIClient 的 keychainStore 是 nil（配置错）→ 重登成功后重试仍会
//       buildURLRequest 阶段抛 .unauthorized → 直接透传上层（合理：配置错不是重登能修的）
//   - 即"buildURLRequest 抛 .unauthorized" 跟 "server 返 401" 都走同一恢复路径，行为统一；
//     但每个原请求最多重登 1 次（防无限循环）

import Foundation

public final class AuthRetryingAPIClient: APIClientProtocol {
    private let inner: APIClientProtocol
    private let coordinator: SilentReloginCoordinator

    public init(inner: APIClientProtocol, coordinator: SilentReloginCoordinator) {
        self.inner = inner
        self.coordinator = coordinator
    }

    public func request<T: Decodable>(_ endpoint: Endpoint) async throws -> T {
        do {
            return try await inner.request(endpoint)
        } catch APIError.unauthorized where endpoint.requiresAuth {
            // 触发静默重登（多并发 coalesce）。失败直接抛上去 —— 业务层走自己的错误恢复.
            _ = try await coordinator.relogin()

            // 重试一次（**仅一次**；重试失败直接抛，不再二次重登）.
            // 重试时 inner.request → buildURLRequest → 读 keychain → 拿新 token → 注入 header → 发请求.
            return try await inner.request(endpoint)
        }
        // catch APIError.unauthorized where !endpoint.requiresAuth: 不拦，let it propagate
        // 其它 APIError（.network / .business / .decoding）: 不拦，let it propagate（Swift do-catch 默认行为）
    }
}
