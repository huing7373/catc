// APIError.swift
// Story 2.4 AC1：APIClient 抛出的统一错误类型。
//
// 四态对应 V1接口设计 §2.4 envelope 解析的四种失败路径。
// 范围红线：纯数据类型，仅 import Foundation；不涉及 UI / Combine。
// UI 文案策略归 Story 2.6 ErrorPresenter，本文件 errorDescription 仅用于 dev / log。

import Foundation

/// APIClient 抛出的统一错误类型。
///
/// 六态划分（Story 5.4 round 2 [P2] fix 把 `.unauthorized` 拆成两态；
/// Story 5.5 round 11 [P2] fix 把 `.missingCredentials` 拆出 transient 子态 `.localStoreFailure`）：
/// - 前四态对应 V1接口设计 §2.4 envelope 解析的四条失败路径
/// - `.missingCredentials` 是**本地态-terminal**：请求**未发出** + 本地凭证**确认不存在**（keychain 读成功但返回 nil/空串）
/// - `.localStoreFailure` 是**本地态-transient**：请求**未发出** + 本地存储**临时不可用**（keychain.get 抛错，sandbox 抽风等）
///
/// 分多态的语义动机（重要 —— 关系到静默重登 scope + 错误展示策略）：
/// - `.unauthorized` = **server 拒绝**当前 token：HTTP 401 或 envelope.code=1001。
///   表达"我已发请求 + server 否认了我的身份"。
///   这是 Story 5.4 静默重登的**唯一**触发条件（"复用既有 guestUid 重新拿 token"语义）。
/// - `.missingCredentials` = **本地端确认无凭证**：keychainStore 未注入 / keychain.get 返 nil
///   或空串。表达"我根本没法发请求 + 本地状态确凿无 token"。
///   这是真 terminal —— 重启 App 也救不了（cold-start 同样读不到 token）。
/// - `.localStoreFailure` = **本地存储临时不可用**：keychain.get 抛错（sandbox 权限抽风 /
///   osStatus -25300 暂时找不到 / 进程刚启动 keychain 还没 ready 等 transient 场景）。
///   语义是"我现在读不到本地 token，但下次再读可能就有了"——**transient**，应允许 retry。
///   这种情况**不能**走静默重登（同 .missingCredentials 理由 1/2/3），但**应当**走 retry，
///   而不是被错误归并到 `.missingCredentials` 的 terminal-class 通道。
///
/// `.missingCredentials` 与 `.localStoreFailure` 的共同点：
///   1. 如果 keychain 配置错，重登写出来的新 token 也读不回来 → 无限失败循环
///   2. 用户可能只是丢了本地 token 但还有 guestUid → 不应当被隐式 relogin（违反 Story 5.4
///      "静默重登限定 server 401" 的 intended scope；也违反 dev-story 文档非范围 §3 钦定）
///   3. cold-start 路径（首次启动 / 卸载重装）应当走 `GuestLoginUseCase`，不该被
///      AuthBoundaryAPIClient 静默拦截掉真实的"无身份"信号
/// 不同点：mapper 把 `.missingCredentials` 映射到 `.alert`（terminal force-quit），
/// 把 `.localStoreFailure` 映射到 `.retry`（transient 自助恢复）。
public enum APIError: Error, Equatable {
    /// 业务错误：HTTP 200 + envelope.code != 0。
    /// 对应 V1接口设计 §3 的 32 个错误码（除 0=成功外）。
    /// - code: V1接口设计 §3 业务码（1001..7002）
    /// - message: 服务端 envelope.message 原文
    /// - requestId: 服务端 envelope.requestId（链路追踪）
    case business(code: Int, message: String, requestId: String)

    /// **server 拒绝**当前 token：HTTP 401 或 envelope.code=1001（V1接口设计 §3）。
    /// 注意：HTTP 401 与 envelope.code=1001 是"或"关系——两条路径都视为 unauthorized。
    /// APIClient 实装按"先看 HTTP status，再看 envelope.code"决策。
    ///
    /// **AuthBoundaryAPIClient 仅 catch 这个 case** 触发静默重登；本地态走 `.missingCredentials`。
    case unauthorized

    /// **本地端确认无凭证（terminal）**：请求被 APIClient 在 `buildURLRequest` 阶段直接拒掉，**未触达 server**。
    /// 语义：keychain 读取成功但确认没有 token（nil / 空串），或 DI 完全没配 keychainStore。
    ///
    /// 触发条件（与 APIClient §决策树对齐 — Story 5.5 round 11 [P2] fix 收窄）：
    /// 1. `endpoint.requiresAuth == true` 但 `keychainStore == nil`（DI 配置错）
    /// 2. `keychainStore.get` 返 `nil` 或空字符串（token 未写入 / 已被 reset）
    ///
    /// **不**包括 keychain.get 抛错的 transient 路径 —— 那条路径走 `.localStoreFailure`（见下一 case）。
    ///
    /// **AuthBoundaryAPIClient 不会 catch 这个 case** —— 让上层（如 RootView 冷启动门路 /
    /// ErrorPresenter）能看到真实的"本地无身份"信号，触发 cold-start GuestLoginUseCase
    /// 或显示配置错诊断，而**不是**被静默重登屏蔽掉。
    ///
    /// 当 `.missingCredentials` 抛到业务层时的恢复路径（按 dev-story 5-4 非范围 §3）：
    /// - cold-start 启动序列读 keychain → 决定走 `GuestLoginUseCase` 重新生成 guestUid + token
    /// - 用户主动 reset → ResetIdentityViewModel 已经清空全部本地态，下一次启动重走 cold-start
    /// - 配置错（keychain 没注入）→ Dev / Release fail-fast，让开发者立刻看到错误
    case missingCredentials

    /// **本地存储临时不可用（transient）**：keychainStore.get 抛错，请求**未发出**。
    /// 语义：本地 token 不是"确认不存在"，而是"现在读不到，下次再读可能就有了"。
    ///
    /// 典型触发场景：
    /// - sandbox 权限抽风（iOS 系统状态切换瞬间）
    /// - keychain 项被 SecItemCopyMatching 返回 osStatus -25300 (errSecItemNotFound) 之外的
    ///   transient OSStatus（例如 -25291 errSecNotAvailable / -34018 service not available）
    /// - 进程刚启动 / 设备刚解锁，keychain 还没 ready
    ///
    /// 与 `.missingCredentials` 的关键差异：mapper 把本 case 映射到 `.retry`（让 user 在 App
    /// 内点重试自愈），而非 `.alert`（terminal force-quit）。bootstrap 路径下重跑整个 closure
    /// 会重新 cold-start GuestLoginUseCase + 读 keychain，transient 错误大概率自愈。
    ///
    /// **AuthBoundaryAPIClient 不会 catch 这个 case** —— 同 `.missingCredentials` 理由：
    /// 1. 静默重登也要读 keychain（写新 token 之前先确认 guestUid），同样会被 transient 失败拦
    /// 2. 不该把"本地 IO 抽风"伪装成"server 401"误触 relogin
    ///
    /// 不透传 underlying KeychainError 给上层业务（仅 dev-facing）—— 业务层只关心 transient
    /// 二分判则，不需要知道 OSStatus 细节。
    case localStoreFailure(underlying: Error)

    /// 网络层错误：连不上 / 超时 / 连接重置 / DNS 失败 / SSL 错误 / 离线。
    /// 包装底层 URLError 或其它 transport 错误。
    case network(underlying: Error)

    /// 解码失败：envelope 结构不符 / data 字段不能解为目标类型 T。
    /// 包装底层 DecodingError 或其它解码相关错误。
    case decoding(underlying: Error)

    // MARK: - Equatable

    /// 自定义 Equatable：underlying error 比较只对比 NSError domain/code 或 String(describing:)。
    /// 仅用于测试断言 ".network 等于 .network" 这种粗粒度等价；不做深度比较。
    /// `.localStoreFailure` 比较只看 case 标签（不比较 underlying Error；测试只关心"是否归类正确"）。
    public static func == (lhs: APIError, rhs: APIError) -> Bool {
        switch (lhs, rhs) {
        case let (.business(c1, m1, r1), .business(c2, m2, r2)):
            return c1 == c2 && m1 == m2 && r1 == r2
        case (.unauthorized, .unauthorized):
            return true
        case (.missingCredentials, .missingCredentials):
            return true
        case (.localStoreFailure, .localStoreFailure):
            // underlying Error 不是 Equatable；仅比较 case 标签即可满足测试断言粒度
            return true
        case let (.network(e1), .network(e2)):
            return (e1 as NSError).domain == (e2 as NSError).domain
                && (e1 as NSError).code == (e2 as NSError).code
        case let (.decoding(e1), .decoding(e2)):
            return String(describing: e1) == String(describing: e2)
        default:
            return false
        }
    }
}

extension APIError: LocalizedError {
    /// 简洁的 dev / log 友好描述；**不**用于 UI 展示（UI 文案在 Story 2.6 的 ErrorPresenter 决定）。
    public var errorDescription: String? {
        switch self {
        case let .business(code, message, _):
            return "Business error \(code): \(message)"
        case .unauthorized:
            return "Unauthorized (HTTP 401 or code 1001 — server rejected current token)"
        case .missingCredentials:
            return "Missing credentials (local-terminal: no keychain / token nil or empty) — request not sent"
        case let .localStoreFailure(underlying):
            return "Local store failure (local-transient: keychain read threw): \(underlying.localizedDescription) — request not sent"
        case let .network(underlying):
            return "Network error: \(underlying.localizedDescription)"
        case let .decoding(underlying):
            return "Decoding error: \(underlying.localizedDescription)"
        }
    }
}
