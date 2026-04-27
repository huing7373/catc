// AuthRetryingAPIClient.swift
// Story 5.4 AC2: APIClient decorator —— 拦 APIError.unauthorized（**仅 server 拒绝 token 的那种**）
//   触发静默重登 + 重试一次.
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
//   4. inner.request(endpoint) throw .missingCredentials（任意 requiresAuth 值）→
//      直接抛上去（**不**重登 —— 见下"为何不拦 missingCredentials"）
//   5. inner.request(endpoint) throw 其它 APIError（.network / .business / .decoding）→
//      直接抛上去（不在重登职责内）
//
// 为何**不**拦 .missingCredentials（Story 5.4 round 2 [P2] codex finding 修正）：
//   .missingCredentials = APIClient.buildURLRequest 阶段抛的"本地无 token / keychain 配置错"，
//   语义是"请求**未发出** + 本地端没有可用凭证"，跟 .unauthorized（"server 拒绝当前 token"）
//   完全不同：
//   - **dev-story 5-4 非范围 §3 钦定**：本 story 只处理 server 401，不接管"本地无 token"路径
//     —— 后者归 cold-start GuestLoginUseCase 管（首次启动 / 卸载重装 / 用户 reset）
//   - 配置错（DI 没注 keychain / key 拼错）→ 应当 fail-fast 让开发者立刻看到，**不**该被
//     静默重登屏蔽到下一次复现才发现
//   - 用户主动 reset（清空 keychain）但 guestUid 还在某些异常路径残留 → 不应当被隐式
//     re-login 把"已退出"状态偷偷恢复成"已登录"（违反 reset 语义）
//   - 连本地 token 都没有时调 coordinator.relogin → SilentReloginUseCase 内部读 guestUid，
//     如果同样缺失也会抛 .missingCredentials → 把"本来 1 次报错"放大成"重登 + 再报错"
//     的 N+1 调用浪费
//
// 与 round 1 注释（"buildURLRequest 抛 .unauthorized 跟 server 返 401 都走同一恢复路径"）
// 的关系：那段说法**已废弃**。round 1 说法忽略了 dev-story 钦定的 scope 边界，导致本地态
// 被错误归并到静默重登路径。本轮把"本地态"切成新 case `.missingCredentials`，让 catch
// 的 case 模式机械保证不再误拦。
//
// Stale-401 dedup（fix-review round 3 [P2] codex finding）：
//   inner.request 之**前** snapshot coordinator.currentGeneration() —— 把它当成"我这次请求基于
//   哪一代 token"的标识. 收到 401 后调 relogin(callerGeneration: snapshot)：
//   - 若 inFlight 路径触发（A 还在跑）→ B coalesce 到同一 task（这条路径靠 inFlight 字段）
//   - 若 generation 路径触发（A 已完成,B 才进 relogin）→ coordinator 看 generation > snapshot →
//     直接返回 lastIssuedToken,不再启第二次 useCase
//   两条路径合起来才能保证 "concurrent 401s trigger one relogin" 的契约在所有时序下成立.

import Foundation

public final class AuthRetryingAPIClient: APIClientProtocol {
    private let inner: APIClientProtocol
    private let coordinator: SilentReloginCoordinator

    public init(inner: APIClientProtocol, coordinator: SilentReloginCoordinator) {
        self.inner = inner
        self.coordinator = coordinator
    }

    public func request<T: Decodable>(_ endpoint: Endpoint) async throws -> T {
        // 进入 inner.request **之前** snapshot generation —— 这就是"我这次请求所基于的 token 那一代".
        // 必须放在 inner.request 之前；放在 catch 内会丢失"caller 的 401 是 stale 还是 fresh"的信息.
        // 只在 requiresAuth=true 时需要这个 snapshot；不过提前做也无副作用（性能可忽略,纯 actor read）.
        let preReloginGeneration = await coordinator.currentGeneration()

        do {
            return try await inner.request(endpoint)
        } catch APIError.unauthorized where endpoint.requiresAuth {
            // 仅 server 401 / envelope 1001 的 .unauthorized 走到这里（本地态走 .missingCredentials，
            // 由下方默认 propagate 行为透传）。
            // 触发静默重登（多并发 coalesce + stale-401 dedup）。失败直接抛上去 —— 业务层走自己的错误恢复.
            _ = try await coordinator.relogin(callerGeneration: preReloginGeneration)

            // 重试一次（**仅一次**；重试失败直接抛，不再二次重登）.
            // 重试时 inner.request → buildURLRequest → 读 keychain → 拿新 token → 注入 header → 发请求.
            return try await inner.request(endpoint)
        }
        // catch APIError.unauthorized where !endpoint.requiresAuth: 不拦，let it propagate
        // catch APIError.missingCredentials: 不拦，let it propagate（见上"为何不拦"）
        // 其它 APIError（.network / .business / .decoding）: 不拦，let it propagate（Swift do-catch 默认行为）
    }
}
