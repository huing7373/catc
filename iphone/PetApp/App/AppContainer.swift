// AppContainer.swift
// Story 2.5 AC6：App 全局依赖注入容器（首次落地）。
//
// 职责（本 story 范围）：
// - 持有 APIClient 单例（baseURL 由 init 时注入）
// - 暴露按需构造 UseCase 的工厂方法（如 makePingUseCase()）
//
// 生命周期：由 RootView 通过 `@StateObject private var container = AppContainer()` 持有，
// 与 App scene 同生命周期。当前 MVP 不引入 SceneStorage / AppDelegate 钩子；container 重启 = App 重启。
//
// 后续扩展（→ Epic 4 / 5 / 12+）：
// - 按需追加 KeychainStore / SessionRepository / WebSocketClient 等
// - 业务 UseCase（GuestLoginUseCase / LoadHomeUseCase / OpenChestUseCase 等）按
//   Repository → UseCase 分层在此 wire；本 story 只 wire PingUseCase 作为模板示范。
//
// 不引入第三方 DI 框架（Swinject / Resolver）：
// MVP 阶段 wire 量小（< 20 个对象），手写 init / factory method 就够，避免 DSL 学习成本
// （与 ADR-0002 §3.1 "手写 mock 优于 codegen" 的精神同源）。
//
// import 备注（继承 Story 2.2 lesson 2026-04-25-swift-explicit-import-combine.md）：
// `ObservableObject` 来自 Combine，必须显式 `import Combine`。

import Foundation
import Combine

@MainActor
public final class AppContainer: ObservableObject {
    public let apiClient: APIClientProtocol

    /// Info.plist 中存放 baseURL 的 key（约定：`PetAppBaseURL`，避免与 Apple 系统 key 冲突）。
    /// 通过 build configuration / xcconfig 覆盖；缺省时回退到 `localhost` fallback。
    public static let baseURLInfoKey = "PetAppBaseURL"

    /// localhost fallback：仅当 Info.plist 既没配置也读不到时启用。
    /// 注意：真机运行时 localhost 解析为设备自身，仅 simulator 上才能命中 Mac 上的 server；
    /// 真机联调请通过 Info.plist `PetAppBaseURL` 覆盖（详见 lesson 2026-04-26-baseurl-from-info-plist）。
    public static let fallbackBaseURLString = "http://localhost:8080"

    /// 默认 init：用 `APIClient(baseURL:)` 构造默认 client。
    /// baseURL 来源优先级：Info.plist[`PetAppBaseURL`] → fallback `http://localhost:8080`。
    /// 不含 `/api/v1` 前缀（host-only baseURL 决策，见 Story 2.5 Dev Note #1）。
    /// 测试 / 未来环境切换通过 `init(apiClient:)` 重载注入自定义 client。
    public convenience init() {
        let baseURL = AppContainer.resolveDefaultBaseURL(from: Bundle.main)
        self.init(apiClient: APIClient(baseURL: baseURL))
    }

    /// 注入式 init：测试中传 mock APIClient；未来 release build 切到 production baseURL 时也走此入口。
    public init(apiClient: APIClientProtocol) {
        self.apiClient = apiClient
    }

    /// 解析默认 baseURL：从给定 bundle 的 Info.plist 读 `PetAppBaseURL`，否则回退到 fallback。
    /// 提取为 static + 接受 bundle 参数：方便测试通过 mock bundle / fixture plist 验证读取逻辑。
    ///
    /// 解析失败（key 不存在 / 类型错 / URL 格式错 / scheme 非 http(s) / host 缺失）一律静默回退到
    /// fallback；不抛、不打 log（MVP 阶段保持 init 路径无副作用；future 改进可加 #if DEBUG print）。
    ///
    /// **关于 `URL(string:)` 的宽容性**（codex round 4 [P2] finding）：
    /// `URL(string: "localhost:8080")` 返回 non-nil（Apple URL parser 把它解析成
    /// `scheme=localhost, path=8080`）；`URL(string: "http://")` 也 non-nil 但 host 为 nil。
    /// 这类 malformed 输入若直接进 `APIClient`，`URLRequest` 会构造出无效请求，所有 ping/version
    /// 调用都落到 offline 路径——表现是"App 看似 OK 但 server 永远 offline"。
    /// 为兑现注释承诺的"malformed 一律 fallback"，必须显式校验 scheme + host。
    /// 详见 docs/lessons/2026-04-26-url-string-malformed-tolerance.md。
    public static func resolveDefaultBaseURL(from bundle: Bundle) -> URL {
        if let raw = bundle.object(forInfoDictionaryKey: baseURLInfoKey) as? String,
           let url = validatedBaseURL(fromString: raw) {
            return url
        }
        // swiftlint:disable:next force_unwrapping
        return URL(string: fallbackBaseURLString)!
    }

    /// 校验字符串能否构成合法的 baseURL：必须能被 `URL(string:)` 解析，且 scheme 是 http/https，
    /// host 非空。任一条件不满足返回 nil，由调用方决定 fallback 策略。
    /// 提为独立 static 方法：让测试可直接覆盖各种 malformed 输入而无需构造 mock Bundle。
    /// 详见 docs/lessons/2026-04-26-url-string-malformed-tolerance.md。
    public static func validatedBaseURL(fromString raw: String) -> URL? {
        guard let url = URL(string: raw),
              let scheme = url.scheme?.lowercased(),
              scheme == "http" || scheme == "https",
              let host = url.host,
              !host.isEmpty
        else {
            return nil
        }
        return url
    }

    /// 工厂：构造 PingUseCase。每次调用返回新实例（UseCase 是 value type，构造廉价）。
    public func makePingUseCase() -> PingUseCaseProtocol {
        DefaultPingUseCase(client: apiClient)
    }
}
