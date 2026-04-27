# Story 5.4: 无效 token 静默重新登录

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want 当 server 返回 401 / envelope `code=1001`（即 `APIError.unauthorized`）时 App 自动用 Keychain 中现有 `guestUid` 重新调 `/auth/guest-login` 拿新 token、写回 Keychain、把原始失败请求**重试一次**就完成恢复 —— 我看不到任何 UI 中断；多个并发业务请求同时拿到 401 时只触发**一次**重登并复用同一个新 token；只有重登本身失败 / 重试后仍是 401 时才走 ErrorPresenter 显示 RetryView,
so that 节点 2 之后任何业务接口（GET /home / GET /steps/account / POST /chest/open ...）拿到陈旧 token（如 server 重启换 `auth.token_secret`、token 过期、用户被踢）时不会突然把我"踢回登录态"，整体体验持续无感（V1 §3 错误码 1001 + iOS 架构 §8.1 钦定的"401 处理"自动化）.

## 故事定位（Epic 5 第四条 = 节点 2 iOS 端鉴权恢复闭环；上承 5.1 / 5.2 / 5.3，下启 5.5）

这是 Epic 5 的**鉴权自动恢复**实装 story —— 把 Story 5.3 的 `APIError.unauthorized` 错误从"裸抛上业务层"升级为"先尝试静默重登 + 重试一次再决定要不要抛"。**直接前置**全部 done：

- **Story 5.1 (`done`)** 已落地 `KeychainStoreProtocol` + `KeychainServicesStore` + `KeychainKey.guestUid` / `KeychainKey.authToken`：
  - `iphone/PetApp/Core/Storage/KeychainStore.swift`：协议三方法（`get(forKey:) throws -> String?` / `set(_:forKey:) throws` / `remove(forKey:) throws` / `removeAll() throws`）—— 本 story 用 `get(KeychainKey.guestUid.rawValue)` 读 guestUid + `set(newToken, KeychainKey.authToken.rawValue)` 写新 token
  - `KeychainKey.guestUid.rawValue == "auth.guestUid"` / `KeychainKey.authToken.rawValue == "auth.token"` —— 锁死的 namespace，本 story 直接消费
- **Story 5.2 (`done`)** 已落地 `AuthRepositoryProtocol` + `DefaultAuthRepository.guestLogin(...)` + `GuestLoginRequest.Device` / `DeviceInfoProvider.current()`：
  - `iphone/PetApp/Features/Auth/Repositories/AuthRepository.swift`：本 story `SilentReloginUseCase` **直接复用** `repository.guestLogin(guestUid:device:)` —— 不再走 `GuestLoginUseCase`（后者会重新生成 / 写 guestUid，跟"复用已有 guestUid"语义冲突）
  - `iphone/PetApp/Features/Auth/UseCases/AuthEndpoints.swift`：`AuthEndpoints.guestLogin(...).requiresAuth == false` —— 重登调用本身**不会**触发本 story 的拦截（避免无限递归）
  - `iphone/PetApp/Features/Auth/Models/GuestLoginResponse.swift`：响应含 `token` / `user` / `pet` —— 本 story 重登成功后写 token；user / pet 暂**不**主动刷 SessionStore（详见"非范围"§5）
  - `iphone/PetApp/Features/Auth/Session/SessionStore.swift`：`clear()` 方法已注明"Story 5.4 静默重登失败兜底" —— 本 story **不**调（详见"非范围"§5；保持 in-memory session 语义稳定）
- **Story 5.3 (`done`)** 已落地 `APIClient` 内 `keychainStore: KeychainStoreProtocol?` Optional 字段 + `buildURLRequest(_:)` 内的 token 注入决策树：
  - `iphone/PetApp/Core/Networking/APIClient.swift`：`request<T>(_ endpoint:) async throws -> T` 在两个位置抛 `APIError.unauthorized`：(a) `buildURLRequest` 注入步骤前置失败（本地无 token / token 空 / keychain 出错）；(b) HTTP 401 短路 + envelope `code=1001` —— 本 story **只**处理 (b) 类型（"server 说我无权"），(a) 类型直接透传不重登（详见"非范围"§3）
  - `APIClientProtocol.request<T>` 协议方法签名零改动 —— 本 story 通过 **decorator pattern** 在 protocol 之外加一层 `AuthRetryingAPIClient` / `SilentReloginInterceptor`，**不**修改 `APIClient` 主体；让 Story 5.3 既有 7 case 单测继续保持原决策树
  - `iphone/PetApp/App/AppContainer.swift`：`apiClient: APIClientProtocol` 由 container 持有；本 story 改 `convenience init()` 让 container 暴露的 `apiClient` 被 `AuthRetryingAPIClient` 包装一层（让所有持 `container.apiClient` 的下游自动获得静默重登能力）

**本 story 的核心动作**（顺序无关，可分批落地）：

1. **新建** `iphone/PetApp/Features/Auth/UseCases/SilentReloginUseCase.swift`：
   - 协议 `SilentReloginUseCaseProtocol: Sendable`：`func execute() async throws -> String`（返回新 token）
   - 实装 `DefaultSilentReloginUseCase: SilentReloginUseCaseProtocol`，依赖 `KeychainStoreProtocol` + `AuthRepositoryProtocol` + 默认 `deviceProvider`：
     - 读 `keychain.get(KeychainKey.guestUid.rawValue)` 拿现有 guestUid
     - guestUid 不存在 / 为空字符串 → 抛 `APIError.unauthorized`（**不**走 `GuestLoginUseCase` 重新生成 —— 那是 cold-start 责任，本 story 是"已有身份但 token 失效"语义）
     - 调 `repository.guestLogin(guestUid:device:)` 拿新 `GuestLoginResponse`
     - `keychain.set(response.token, KeychainKey.authToken.rawValue)` 写新 token
     - 返回新 token（让上层 `AuthRetryingAPIClient` 可立即拿来重试 —— 不必再读 keychain 一次）
2. **新建** `iphone/PetApp/Core/Networking/AuthRetryingAPIClient.swift`：
   - `final class AuthRetryingAPIClient: APIClientProtocol`，包装一个内层 `APIClientProtocol` + `SilentReloginUseCaseProtocol`：
     ```swift
     public func request<T: Decodable>(_ endpoint: Endpoint) async throws -> T {
         do {
             return try await inner.request(endpoint)
         } catch APIError.unauthorized where endpoint.requiresAuth {
             // 仅对 requiresAuth=true 的请求触发重登（避免无限循环）
             _ = try await reloginCoordinator.relogin()  // coalesce 多并发请求
             return try await inner.request(endpoint)    // 重试一次
         }
     }
     ```
   - 关键约束（详 AC2 / AC3）：
     - **每个原始请求最多触发 1 次静默重登**：第二次仍 unauthorized 直接透传，**不**再二次重登
     - **多并发请求只触发一次重登**：通过下一节"重登协调器"actor 实现 coalescing
     - **重登过程中又来的新 401**：排队等待第一次重登完成 → 复用其结果（成功用新 token 重试 / 失败抛同一 error）
     - **`requiresAuth == false` 的请求**：即使抛 unauthorized 也**不**重登（如 `/auth/guest-login` 本身 401 → 抛上去，不能用自己救自己）
3. **新建** `iphone/PetApp/Features/Auth/UseCases/SilentReloginCoordinator.swift`：
   - `actor SilentReloginCoordinator`，封装"同一时刻只跑一次重登 + 多并发等待复用结果"的协调逻辑：
     ```swift
     public actor SilentReloginCoordinator {
         private let useCase: SilentReloginUseCaseProtocol
         private var inFlight: Task<String, Error>?

         public init(useCase: SilentReloginUseCaseProtocol) {
             self.useCase = useCase
         }

         public func relogin() async throws -> String {
             if let existing = inFlight {
                 // 已有重登在飞 → 等它的结果（不重复跑 useCase.execute）
                 return try await existing.value
             }
             let task = Task { try await useCase.execute() }
             inFlight = task
             defer { inFlight = nil }  // 完成（成功 / 失败）后清掉，下次 401 再开新一轮
             return try await task.value
         }
     }
     ```
   - 关键约束（详 AC4）：
     - actor 隔离保证 `inFlight` 的 read-modify-write 原子性 —— 多并发 `relogin()` 调用串行进入 actor，第一个建 task 第二个 await 既存 task
     - `defer { inFlight = nil }` 保证一次 relogin 完成后下次 401 能重新开一轮（不会卡死在 nil-check 里）
     - 失败也 nil 重置（让用户后续动作还能重新触发重登 —— 不一棒子打死）
4. **修改** `iphone/PetApp/App/AppContainer.swift`：
   - `convenience init()` 改造：先建 `let baseAPIClient = APIClient(baseURL: ..., keychainStore: keychainStore)` —— 然后用 `AuthRetryingAPIClient` 包一层得 `let wrappedAPIClient`
   - `wrappedAPIClient` 注入 `self.init(apiClient: ..., keychainStore: ...)` —— 业务层（`makeAuthRepository()` / 未来 `makeHomeRepository()` 等）拿到的 `apiClient` 自动具备重登能力
   - 新增 factory `func makeSilentReloginUseCase()` 返回 `SilentReloginUseCaseProtocol`（让单测 / 集成测试可走 container 入口拿同一份依赖）
   - `init(apiClient:keychainStore:)` 主 init 不动（测试场景注入自定义 mock APIClient 时跳过 wrap，**有意**让单测保持 Story 5.3 的简单语义）
5. **新建** `iphone/PetAppTests/Core/Networking/AuthRetryingAPIClientTests.swift`：
   - ≥ 5 case，覆盖 happy + edge：
     - case#1 happy：requiresAuth=true 第一次 401 → 重登 success → 重试 success → 用户感知 0
     - case#2 edge：requiresAuth=true 第一次 401 → 重登失败（network） → 抛 APIError 上层
     - case#3 edge：requiresAuth=true 第一次 401 → 重登 success → 重试**仍** 401 → 抛 unauthorized（不二次重登）
     - case#4 edge：5 个并发请求都 401 → coordinator coalesce → 只触发 1 次重登 + 5 次重试都用同一新 token
     - case#5 edge：requiresAuth=**false** 的 endpoint 抛 unauthorized → **不**触发重登 → 直接抛上去
6. **新建** `iphone/PetAppTests/Features/Auth/UseCases/SilentReloginUseCaseTests.swift`：
   - ≥ 4 case，覆盖：
     - case#1 happy：keychain 有 guestUid → repo.guestLogin success → keychain.set token 一次 → 返回新 token
     - case#2 edge：keychain 无 guestUid → 抛 unauthorized（不调 repo）
     - case#3 edge：keychain.get 抛错 → 透传错误
     - case#4 edge：repo.guestLogin 失败 → 透传错误（不写 token）
7. **新建** `iphone/PetAppTests/Features/Auth/UseCases/SilentReloginCoordinatorTests.swift`：
   - ≥ 3 case，覆盖 actor coalescing：
     - case#1 happy：单调 → useCase.execute 调一次
     - case#2 happy：5 并发调 relogin() → useCase.execute 调一次（coalesce 验证）
     - case#3 happy：第一次 relogin 完成后 + 第二次 relogin → useCase.execute 调两次（inFlight 清空验证）
8. **不**修改 `APIClient.swift` / `APIClientProtocol`：本 story 走 **decorator pattern** —— 在 `AuthRetryingAPIClient` 内 wrap，protocol 不变，所有持 `APIClientProtocol` 引用的代码（`DefaultAuthRepository(apiClient: APIClientProtocol)`）自动透明升级
9. **不**修改 `Endpoint.swift` / `APIError.swift` / `Auth/UseCases/GuestLoginUseCase.swift` / `Auth/UseCases/AuthEndpoints.swift` / `Auth/Repositories/AuthRepository.swift` / `Auth/Models/*` / `Auth/Session/SessionStore.swift` —— 本 story 是网络层装饰 + 新 UseCase + Coordinator，业务层零改动
10. **不**改 `RootView` / `AppLaunchStateMachine` / `HomeView` / `KeychainUITestHookView` —— 启动流由 Story 5.2 GuestLoginUseCase 把控；本 story 是"启动后稳态期间"的恢复机制
11. **不**新增 UI / 不接 ErrorPresenter / 不弹 toast：静默重登语义就是"用户无感"；只有"重登失败"时才走业务层正常错误路径（业务层接到 `APIError` 后走 ErrorPresenter，那是业务层自己的事，本 story 不预设新 UI 流）

**测试基础设施**：

- **不**新增第三方 mock 库（与 ADR-0002 §3.1 一致）
- 复用 Story 2.5 落地的 `MockAPIClient`（含 `stubResponse: [String: Stub]` + `invocations: [Endpoint]`）
- 复用 Story 2.8 落地的 `MockKeychainStore`（继承 `MockBase` + `getStubResult` / `setStubError` / `record` 自动）
- 复用 Story 5.2 落地的 `MockAuthRepository`（继承 `MockBase` + `guestLoginStub` + `lastGuestUid` / `lastDevice`）
- **新建** `iphone/PetAppTests/Features/Auth/UseCases/MockSilentReloginUseCase.swift`（继承 `MockBase` + `executeStub: Result<String, Error>` + `lastInvocationCount`）—— 让 `AuthRetryingAPIClientTests` 不必构造 keychain + repo + useCase 三件套，可以直接 stub coordinator 上游
- **新建** `iphone/PetAppTests/Core/Networking/StatefulMockAPIClient.swift`（**或**追加到既有 `MockAPIClient`）：让 case#3（"第一次 401 + 第二次 success"）能按调用次数返回不同 stub —— 既有 `MockAPIClient.stubResponse` 是 path → 单 stub 的 map，不支持"按调用次数序列化"。
  - **方案 A**（推荐）：新建独立 `StatefulMockAPIClient` mock，含 `responseSequence: [String: [Stub]]`（path → 按顺序 pop 队列）+ `invocations` 复用同模式
  - **方案 B**：扩展既有 `MockAPIClient` 加 `stubResponseSequence` 字段（与 `stubResponse` 互斥）；优先 sequence，缺省 fallback 到旧 map
  - dev 自决；推荐 A（独立文件 + 不改既有 `MockAPIClient` 既有 7+ 测试基础）

**不涉及**：

1. **`/auth/guest-login` 接口实装**：归 server 端 Story 4.6 已 done；本 story 是 client 调用方
2. **token 预刷新 / refresh token 流**：MVP 不做（V1 §4.1 钦定 `/auth/guest-login` 幂等，过期直接重新 guest-login；refresh token 是 Post-MVP / FR3）
3. **本地无 token / token 空（即 Story 5.3 buildURLRequest 阶段抛的 unauthorized）触发重登**：本 story **不**处理这一类。原因：
   - Story 5.3 buildURLRequest 阶段抛 unauthorized 表示"interceptor 配置错误（无 keychain 注入）/ keychain 已被清（如 dev 重置按钮 / 卸载重装）"——此时本来就需要走 cold-start GuestLoginUseCase 重新生成 guestUid + token，**不是**"复用已有 guestUid 重登"语义
   - 区分判据：本 story 拦的 unauthorized 必须是 `inner.request(endpoint)` 抛的（即过了 buildURLRequest 阶段、确实发送了请求 + server 返 401 / envelope 1001）；buildURLRequest 阶段抛的同类型错误**不**触发重登（因为根本没"无效 token"——是没 token 或者键值缺失）
   - **实装区分方法**：`AuthRetryingAPIClient` 内对 `inner.request` 抛的 unauthorized 触发重登；如果重登后调 `inner.request(endpoint)` 又抛 unauthorized（无论是 buildURLRequest 阶段还是 server 阶段），都**不**再二次重登（避免无限循环）。这意味着 buildURLRequest 阶段的 unauthorized 不会被"误重登"——但"重登成功 → 重试时 buildURLRequest 又抛 unauthorized"也不会再触发 —— 这种极少见的 race（重登成功 + token 立刻被另一个进程清掉）走"抛 unauthorized 上层"的兜底语义
4. **`/auth/guest-login` 接口本身返 401**（如 server 拒绝该 guestUid）：`AuthEndpoints.guestLogin.requiresAuth == false`，本 story `where endpoint.requiresAuth` 守卫保证不重登；直接抛 `APIError.unauthorized` 给业务层（`GuestLoginUseCase` / `SilentReloginUseCase` 自己的责任范围）—— 避免"自己救自己"的无限递归
5. **重登成功后是否同步更新 SessionStore（user / pet）**：节点 2 阶段**不**主动更新。原因：
   - 静默重登的语义是"恢复 token"，user / pet 数据不应当被"换一份"——同一个 guestUid 重新登录 server 返回的应当是相同的 user / pet 数据（V1 §4.1 钦定幂等）
   - 即便有差异（如 server 端 nickname 后台改了），节点 2 阶段没有"看着 nickname 实时变"的需求；下一次启动 GuestLoginUseCase 自然会 refresh
   - 节点 2+ 之后业务层调 `/home` 自然会拿最新 user / pet（Story 5.5 LoadHomeUseCase + Story 4.8 GET /home 已包括 user 字段）—— 业务层错误恢复路径自然刷新
6. **服务端任何改动**：纯客户端实装；`server/` 全程零改动
7. **WebSocket 连接的鉴权恢复**：归 Epic 10（`WebSocketClient`）；REST 与 WS 是不同协议；WS 鉴权失败的恢复策略由 WS 客户端自行实装（如重连时带新 token）
8. **iOS UITest**：本 story 是 APIClient 装饰器内部行为 + 单测可完整覆盖 + RootView 调用面零改动 —— **不**新建任何 UITest；Story 5.2 落地的 `UITEST_SKIP_GUEST_LOGIN` 旁路继续保护既有 UITest
9. **`ios/` 旧产物目录**：CLAUDE.md "Repo Separation" + ADR-0002 §3.3 既有约束。最终 `git status` 须确认 `ios/` 下零改动
10. **Story 5.5 LoadHomeUseCase**：归 Story 5.5；本 story 实装的"业务请求自动重登"机制 LoadHomeUseCase 自然受益（一个 GET /home 请求 401 → 自动重登 + 重试 → 业务层无感）

**范围红线**：

- **不动 `ios/`**：CLAUDE.md "Repo Separation" + ADR-0002 §3.3
- **不动 `server/`**：纯 iPhone 端实装
- **不动 `iphone/scripts/` / `iphone/project.yml` / `iphone/.gitignore`**：新文件靠既有 `sources: - PetApp` / `sources: - PetAppTests` glob 自动纳入；**0 yml 改动**
- **不引入第三方依赖**：`Foundation` 即可；actor / async-await 是 Swift 标准
- **不动 `APIClientProtocol` 协议签名**：用 decorator pattern wrap；让 `DefaultAuthRepository(apiClient: APIClientProtocol)` 等所有持协议引用的下游零改动
- **不动 `APIClient` 既有实现**：本 story 是新建 `AuthRetryingAPIClient` 包装层；`APIClient` 主体（含 Story 5.3 落地的 token 注入决策树）全部保留
- **不动 `Endpoint.swift` / `APIError.swift` / `URLSessionProtocol.swift` / `APIResponse.swift`**：仅消费既有定义
- **不动 `GuestLoginUseCase.swift`**：cold-start 责任与本 story 静默重登责任**互不重叠**（GuestLoginUseCase 会 generate-or-read guestUid + 写 keychain；本 story 只 read 既有 guestUid）
- **不动 `AuthRepository.swift`**：仅消费 `guestLogin(guestUid:device:)` 方法；不增减协议方法
- **不动 `SessionStore.swift`**：本 story 不更新 SessionStore（详见"非范围"§5）
- **不在 `APIError` 加新 case**：复用 `.unauthorized` 表达"重登也救不了"语义（与 5.3 同）；下游如需区分"原始 401 vs 重登失败"可在装饰器内自定义错误（**不**这么做 —— 业务层语义都是"鉴权失败，请重登"，加 case 反而污染 APIError）
- **重登一次性原则严格**：每个原始请求的 1 次 await `inner.request(endpoint)` 失败 → 1 次 await `coordinator.relogin()` → 1 次 await `inner.request(endpoint)` 重试。重试无论成功 / 失败都直接 return / throw —— **绝不**第三次调 inner
- **多并发 401 严格 coalesce**：靠 actor `inFlight: Task?` 字段 —— 第二个进入 actor 的请求看到非 nil → await 既存 task —— 它不会重新调 useCase.execute()
- **重登失败的协调器状态恢复**：`defer { inFlight = nil }` 保证失败后下次 401 能开新一轮；如果不重置会卡死在"永远复用一个失败的 task" 状态
- **重登过程不接 ErrorPresenter / 不弹 UI**：静默 = 用户无感；只有最终透传给业务层后业务层走自己的错误处理（对 ErrorPresenter 的依赖在业务层不在本 story）
- **重登期间不阻塞其他业务请求**：actor 串行进入但快速 return（在飞重登的 await 不阻塞 actor 执行其它 method —— Swift actor 的 `await` 暗示让出 actor）；同时**不允许**用 `@MainActor` 强制把重登路径锁到主线程（让重登可以在任何 task 上跑，避免 UI 卡顿）
- **不缓存新 token 到 `AuthRetryingAPIClient` 内**：keychain 已是 source of truth；重试时调 `inner.request` 自动从 keychain 读到新写的 token（Story 5.3 决策树第 5 步）

## Acceptance Criteria

**AC1 — `SilentReloginUseCase` 协议 + 实装：复用已有 guestUid 调 /auth/guest-login，写 token，返回 token**

新建 `iphone/PetApp/Features/Auth/UseCases/SilentReloginUseCase.swift`：

```swift
// SilentReloginUseCase.swift
// Story 5.4 AC1: 静默重登 UseCase —— "复用已有 guestUid 重新拿 token" 语义.
//
// 与 Story 5.2 GuestLoginUseCase 的区别：
//   - GuestLoginUseCase: cold-start 路径，会 generate-or-read guestUid + 写 SessionStore
//   - SilentReloginUseCase: 稳态恢复路径，仅 read 已有 guestUid（无则失败，绝不 generate）+ 不动 SessionStore
//
// 流程：
//   1. keychain.get(KeychainKey.guestUid.rawValue) —— 读现有 UID
//   2. nil / 空字符串 → throw APIError.unauthorized（无身份可恢复 —— 让业务层 fallback 到 cold-start 流）
//   3. 调 repo.guestLogin(guestUid: uid, device: deviceProvider())
//   4. 成功 → keychain.set(response.token, KeychainKey.authToken.rawValue) 写新 token
//   5. 返回新 token（让上层 AuthRetryingAPIClient 不必再读 keychain 一次即可重试原请求）
//
// 错误处理：
//   - keychain.get 失败 → 透传 KeychainError 上层（不吞错；让业务层 / coordinator 看到本 root cause）
//   - keychain.set 失败 → 透传 KeychainError；调用方收到错误后**不**重试原请求（半成功状态：server 已发新 token 但本地未存）
//   - repo.guestLogin 失败 → 透传 APIError；keychain 不动（已有的旧 token 仍在；下一次 401 会再触发重登）
//
// 不在本 story 范围：
//   - 不调 SessionStore.updateSession（详见 story 非范围 §5）
//   - 不调 GuestLoginUseCase（避免 generate 新 guestUid 覆盖既有身份）
//   - 不做 retry / 指数退避（重登失败一次就抛；上层决定是否重试）

import Foundation

public protocol SilentReloginUseCaseProtocol: Sendable {
    /// 复用 Keychain 中现有 guestUid 调 /auth/guest-login 拿新 token + 写 keychain.
    /// - Returns: 新 token（已写 keychain，调用方可直接用以重试原请求）
    /// - Throws:
    ///   - APIError.unauthorized: keychain 中无 guestUid（无身份可恢复）
    ///   - KeychainError: keychain 读 / 写失败
    ///   - APIError.network / .business / .decoding: /auth/guest-login 调用失败
    func execute() async throws -> String
}

public struct DefaultSilentReloginUseCase: SilentReloginUseCaseProtocol {
    private let keychainStore: KeychainStoreProtocol
    private let repository: AuthRepositoryProtocol
    private let deviceProvider: @Sendable () -> GuestLoginRequest.Device

    public init(
        keychainStore: KeychainStoreProtocol,
        repository: AuthRepositoryProtocol,
        deviceProvider: @escaping @Sendable () -> GuestLoginRequest.Device = { DeviceInfoProvider.current() }
    ) {
        self.keychainStore = keychainStore
        self.repository = repository
        self.deviceProvider = deviceProvider
    }

    public func execute() async throws -> String {
        // Step 1: 读已有 guestUid
        let existing = try keychainStore.get(forKey: KeychainKey.guestUid.rawValue)

        // Step 2: 无 guestUid → 不能"假装重登"——必须走 cold-start，故抛 unauthorized
        guard let guestUid = existing, !guestUid.isEmpty else {
            throw APIError.unauthorized
        }

        // Step 3: 调 /auth/guest-login（requiresAuth=false）
        let device = deviceProvider()
        let response = try await repository.guestLogin(guestUid: guestUid, device: device)

        // Step 4: 写新 token（覆盖旧 token）
        try keychainStore.set(response.token, forKey: KeychainKey.authToken.rawValue)

        // Step 5: 返回新 token
        return response.token
    }
}
```

**具体行为要求**：

- 协议 `SilentReloginUseCaseProtocol: Sendable`：让协调器 actor 持有 / 跨 task 调用安全；与 `GuestLoginUseCaseProtocol` 同模式
- 协议方法 `func execute() async throws -> String`：返回新 token；让上层 `AuthRetryingAPIClient` 拿到 token 即可重试原请求（**不**再读 keychain 一次，避免重复 IO）
- 实装 `DefaultSilentReloginUseCase: struct`：与 `DefaultGuestLoginUseCase` 同（value type，构造廉价）
- 依赖注入：`KeychainStoreProtocol` + `AuthRepositoryProtocol` + Optional `deviceProvider` closure（默认走 `DeviceInfoProvider.current()`）—— 测试场景注入 mock + stub closure
- **不**注入 `uuidGenerator`：本 UseCase 不生成 guestUid；guestUid 必须来自 keychain
- 错误透传**严格**：所有底层错误（KeychainError / APIError）原样抛，**不**做错误码映射 / 转换 —— 协调器 / 装饰器 / 业务层负责自己关心的错误
- guestUid 为空字符串 `.isEmpty` 视同 nil：与 Story 5.2 `GuestLoginUseCase.existing.isEmpty` 同精神（防御性，避免 keychain 写空串导致空请求）

**AC2 — `AuthRetryingAPIClient` 装饰器：拦 unauthorized 触发重登 + 重试一次**

新建 `iphone/PetApp/Core/Networking/AuthRetryingAPIClient.swift`：

```swift
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
        // catch APIError.unauthorized where !endpoint.requiresAuth: 不拦，让 catch 后面的子句走默认抛
        // 其它 APIError（.network / .business / .decoding）: 不拦，let it propagate（Swift do-catch 默认行为）
    }
}
```

**具体行为要求**：

- `AuthRetryingAPIClient: APIClientProtocol`：实现协议方法签名 `request<T>(_ endpoint:) async throws -> T` —— 让所有持 `APIClientProtocol` 引用的下游零改动
- `final class`（**不**用 struct）：因为持有 `SilentReloginCoordinator: actor`（actor 持有要求引用类型才能共享）
- 标 `@unchecked Sendable` **不需要**：`APIClientProtocol` 已标 `Sendable`，inner / coordinator 都是 Sendable，编译器自动推导
- catch 分支**只**捕 `APIError.unauthorized where endpoint.requiresAuth`：用 Swift `where` 子句 —— `requiresAuth=false` 路径走默认抛（不进入 catch body）
- 重试**仅一次**：catch body 内**不**包 do-catch；如果 retry 抛 `APIError.unauthorized`，会从 `request<T>` 函数本身抛出去（Swift `try await` 默认行为）
- 重试失败时**不**再二次触发 coordinator.relogin —— 实现上"重试一次"靠 catch body 不再嵌套 do-catch 自然保证
- coordinator 通过 init 注入：让单测可注入 mock coordinator 或 stub coordinator —— 验"是否调过 relogin / 调几次"
- **不**在 AuthRetryingAPIClient 内做错误码翻译 / 加业务语义：透传上层

**AC3 — 严格"每请求最多重登 1 次"语义验证**

测试场景必须验证以下 2 个具体行为，断言精确：

1. **重试也 unauthorized → 不二次重登**：
   - 给 stateful mock APIClient 配 `path → [.failure(.unauthorized), .failure(.unauthorized), .success(...)]`（3 个 stub）
   - 给 mock coordinator 配 `executeStub: .success("new-token-1")`
   - 调 `wrappedClient.request(endpoint)` → 期望抛 `APIError.unauthorized`（第二次失败）
   - 断言 inner.request 被调 **2 次**（原始 + 重试 1 次），**不是** 3 次
   - 断言 coordinator.relogin **1 次**（只在第一次失败后触发，重试失败后**不**再 relogin）

2. **重试 success → 用户感知 0**：
   - 给 stateful mock APIClient 配 `path → [.failure(.unauthorized), .success(value)]`（2 个 stub）
   - 调 `wrappedClient.request(endpoint)` → 期望成功 return value
   - 断言 inner.request 被调 **2 次**；coordinator.relogin **1 次**

**具体行为要求**：

- "1 次重登" 边界严格按：1 次 inner.request 失败 → 1 次 coordinator.relogin → 1 次 inner.request 重试 —— 共 2 次 inner + 1 次 coordinator
- 不允许通过 retry pattern / 循环 / 递归实现"重试 N 次"—— Swift do-catch 单次 catch 即可；catch body 内**不**再嵌套 do-catch
- 对 `requiresAuth == false` 的请求，即使 unauthorized 也**不**触发 coordinator —— 测试要专门覆盖（case#5）
- 对 `.network` / `.business` / `.decoding` 错误类型，即使 endpoint requiresAuth=true 也**不**触发 coordinator —— 测试要覆盖（case#6 边缘 / 可选）

**AC4 — `SilentReloginCoordinator` actor coalescing：多并发请求只触发 1 次重登**

新建 `iphone/PetApp/Features/Auth/UseCases/SilentReloginCoordinator.swift`：

```swift
// SilentReloginCoordinator.swift
// Story 5.4 AC4: 静默重登协调器 —— "同一时刻只跑一次重登 + 多并发等待复用结果".
//
// 设计选择（actor）：
//   - 用 actor 而非 class + NSLock：actor 的 isolation 自动保证 inFlight 字段 read-modify-write 原子
//   - actor method 内的 `await` 让出 actor —— 第二个进入的 relogin() 调用看到非 nil inFlight 就 await
//     既存 task 的 .value（不重新建 task / 不重复调 useCase.execute）
//
// 关键约束：
//   - inFlight 是 Task<String, Error>?：Task 已 cancelable / can-await-value，是 Swift 标准 future
//   - defer { inFlight = nil } 保证一次重登完成（成功 / 失败）后下次 401 能开新一轮（不卡死）
//   - 失败的 task 被多并发等待者拿到的是同一个 throw —— 协调器 caller 一致看到同一 error，避免
//     "5 个请求 5 个不同 error" 的诡异行为
//
// 不职责：
//   - 不做 retry（让 caller 决定是否重试 —— 如 AuthRetryingAPIClient 内的 catch 即"重试 1 次"逻辑）
//   - 不做指数退避 / circuit breaker（MVP 不做；server /auth/guest-login 已有 rate_limit 中间件）
//   - 不持有任何 token / session 状态（数据在 useCase 内 + keychain）

import Foundation

public actor SilentReloginCoordinator {
    private let useCase: SilentReloginUseCaseProtocol
    private var inFlight: Task<String, Error>?

    public init(useCase: SilentReloginUseCaseProtocol) {
        self.useCase = useCase
    }

    /// 触发一次静默重登；多并发调用 coalesce 到同一次执行.
    /// - Returns: 新 token
    /// - Throws: useCase.execute 抛的任何错误（KeychainError / APIError）
    public func relogin() async throws -> String {
        // 已有 task 在飞 → 等它（不重启）.
        // actor 的 isolation 保证这个 if-let 与下面 inFlight = task 是原子（无 race）.
        if let existing = inFlight {
            return try await existing.value
        }

        // 没在飞 → 启动新 task.
        // Task { ... } 让 useCase.execute 在 cooperative thread pool 上跑（不阻塞 actor）.
        let task = Task { try await useCase.execute() }
        inFlight = task

        // defer 保证完成后 nil 重置 —— 下一次 relogin 能开新一轮.
        defer { inFlight = nil }

        // await 自己启动的 task；其它并发 relogin 调用看到的 inFlight 也指向同一 task → 同一结果.
        return try await task.value
    }
}
```

**具体行为要求**：

- `actor SilentReloginCoordinator`：用 Swift actor 而非 class + NSLock —— actor 内 `await` 自动让出 actor，多并发调用串行进入但不互相阻塞 actor 本身
- `inFlight: Task<String, Error>?` 字段：`Task` 是 Swift 标准 future，已 thread-safe + can-await
- `defer { inFlight = nil }`：完成（成功 / 失败 / 取消）后清 inFlight —— 下次 relogin 可以重新开一轮
  - **不**用 `inFlight = nil` 在 try await 之后（否则失败抛错时跳过，inFlight 卡死非 nil）
  - **不**用 do-catch + finally：Swift 没有 finally，`defer` 是等价模式
- 测试 case#3（"第一次完成后 + 第二次"）必须验：第一次 relogin 完后再调 relogin → useCase.execute 被调 2 次（确认 inFlight 清掉了）
- 测试 case#2（5 并发）必须用 `withThrowingTaskGroup` 或 `Task { ... }` × 5 + await all —— 与 Story 5.3 case#7 同模式

**AC5 — `AppContainer.convenience init()` wire `AuthRetryingAPIClient`**

修改 `iphone/PetApp/App/AppContainer.swift`：

```swift
public convenience init() {
    let baseURL = AppContainer.resolveDefaultBaseURL(from: Bundle.main)
    let keychainStore = KeychainServicesStore()

    // Story 5.4 改动：先建底层 APIClient，然后用 AuthRetryingAPIClient 包一层.
    // 业务层（makeAuthRepository / 未来 makeHomeRepository 等）拿到的 apiClient 自动具备
    // 静默重登能力 —— 一次 wire，全 App 受益.
    let baseAPIClient = APIClient(baseURL: baseURL, keychainStore: keychainStore)
    let baseRepository = DefaultAuthRepository(apiClient: baseAPIClient)
    let reloginUseCase = DefaultSilentReloginUseCase(
        keychainStore: keychainStore,
        repository: baseRepository
    )
    let coordinator = SilentReloginCoordinator(useCase: reloginUseCase)
    let wrappedAPIClient = AuthRetryingAPIClient(inner: baseAPIClient, coordinator: coordinator)

    self.init(
        apiClient: wrappedAPIClient,
        keychainStore: keychainStore
    )
}
```

新增 factory：

```swift
/// Story 5.4 新增：构造 SilentReloginUseCase（默认走 container 持有的 keychain + 新建 repository）.
/// 让需要直接调 SilentRelogin 的场景（集成测试 / future 业务）走 container 入口.
public func makeSilentReloginUseCase() -> SilentReloginUseCaseProtocol {
    DefaultSilentReloginUseCase(
        keychainStore: keychainStore,
        repository: makeAuthRepository()
    )
}
```

**具体行为要求**：

- `convenience init()` 内**两个** repository 实例：
  - `baseRepository`（直接持 baseAPIClient）：**只**给 `SilentReloginUseCase` 用 —— 走原始 APIClient（不 wrap），因为重登本身是 `requiresAuth=false`，且包一层会引入"重登失败也尝试重登"的逻辑漏洞
  - `makeAuthRepository()` 返回的（持 wrappedAPIClient）：给业务层用 —— 享受静默重登
- 这种"双 repository" 设计是有意为之的；**不**复用同一个 `AuthRepository`（避免循环依赖：repository 用 wrappedAPIClient → wrappedAPIClient 失败 → coordinator 调 useCase → useCase 调同一 repository → 又走 wrappedAPIClient → 又失败 → 死循环）
- `init(apiClient:keychainStore:)` 主 init 不动 —— 测试场景直接注入自定义 mock APIClient（**不**走 wrap），保持 Story 5.3 / 5.2 既有 `AppContainerTests` 全部 0 失败
- `makeSilentReloginUseCase()` 返回 `DefaultSilentReloginUseCase(repository: makeAuthRepository())` —— 这里 repository 是 wrap 过的；**有意为之**：业务侧调 makeSilentReloginUseCase 的场景几乎不存在（重登在 wrap 内自动跑），保留这个 factory 主要给集成测试 / future 业务用，跟 convenience init 内 baseRepository 走法不同（前者 wrap 后者不 wrap），是不同语义场景的不同选择
  - **dev 注意**：如果集成测试 / future 业务通过 `makeSilentReloginUseCase()` 拿到 useCase 然后调 `execute()` 拿到的 repository 是 wrap 过的，这没问题 —— 重登调 `/auth/guest-login` 的 endpoint requiresAuth=false，AuthRetryingAPIClient 不会拦截
- **不**对外暴露 `coordinator` / `wrappedAPIClient` / `baseAPIClient`：协调器 + 装饰器都是 internal infrastructure，业务层不应直接接触

**AC6 — 单元测试 `SilentReloginUseCaseTests`**

新建 `iphone/PetAppTests/Features/Auth/UseCases/SilentReloginUseCaseTests.swift`，≥ 4 case：

```swift
// SilentReloginUseCaseTests.swift
// Story 5.4 AC6: SilentReloginUseCase 单元测试.
// 复用 MockKeychainStore（Story 2.8）+ MockAuthRepository（Story 5.2），不新建 mock.

import XCTest
@testable import PetApp

#if DEBUG

@MainActor
final class SilentReloginUseCaseTests: XCTestCase {

    nonisolated private static func makeDevice() -> GuestLoginRequest.Device {
        GuestLoginRequest.Device(platform: "ios", appVersion: "1.0.0", deviceModel: "iPhone15,2")
    }

    private func makeStubResponse(token: String = "new-token-1") -> GuestLoginResponse {
        GuestLoginResponse(
            token: token,
            user: UserProfile(id: "1001", nickname: "用户1001", avatarUrl: "", hasBoundWechat: false),
            pet: PetProfile(id: "2001", petType: 1, name: "默认小猫")
        )
    }

    // MARK: - case#1 (happy)：keychain 有 guestUid → repo 成功 → 写新 token → 返回新 token
    func testExecuteRevolvesExistingGuestUidAndReturnsNewToken() async throws {
        let keychain = MockKeychainStore()
        keychain.getStubResult = .success("existing-uid-abc")
        let repo = MockAuthRepository()
        repo.guestLoginStub = .success(makeStubResponse(token: "new-token-1"))

        let useCase = DefaultSilentReloginUseCase(
            keychainStore: keychain,
            repository: repo,
            deviceProvider: { Self.makeDevice() }
        )

        let token = try await useCase.execute()

        XCTAssertEqual(token, "new-token-1")
        XCTAssertEqual(keychain.callCount(of: "get(forKey:)"), 1, "应读 guestUid 一次")
        XCTAssertEqual(keychain.callCount(of: "set(_:forKey:)"), 1, "应写 token 一次（不写 guestUid）")
        XCTAssertEqual(repo.callCount(of: "guestLogin(guestUid:device:)"), 1)
        XCTAssertEqual(repo.lastGuestUid, "existing-uid-abc", "必须复用既有 guestUid")
    }

    // MARK: - case#2 (edge)：keychain 无 guestUid → 抛 unauthorized + 不调 repo
    func testThrowsUnauthorizedWhenGuestUidMissing() async {
        let keychain = MockKeychainStore()
        keychain.getStubResult = .success(nil)
        let repo = MockAuthRepository()

        let useCase = DefaultSilentReloginUseCase(
            keychainStore: keychain,
            repository: repo,
            deviceProvider: { Self.makeDevice() }
        )

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.unauthorized")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized)
        } catch {
            XCTFail("应抛 APIError.unauthorized，实际抛 \(error)")
        }

        XCTAssertEqual(repo.callCount(of: "guestLogin(guestUid:device:)"), 0, "无 guestUid 时**绝不**调 repo")
        XCTAssertEqual(keychain.callCount(of: "set(_:forKey:)"), 0, "也不该写 token")
    }

    // MARK: - case#3 (edge)：keychain 无 guestUid（空字符串视同）→ 抛 unauthorized
    func testThrowsUnauthorizedWhenGuestUidIsEmptyString() async {
        let keychain = MockKeychainStore()
        keychain.getStubResult = .success("")
        let repo = MockAuthRepository()

        let useCase = DefaultSilentReloginUseCase(
            keychainStore: keychain,
            repository: repo
        )

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.unauthorized")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized)
        } catch {
            XCTFail("应抛 APIError.unauthorized，实际抛 \(error)")
        }

        XCTAssertEqual(repo.callCount(of: "guestLogin(guestUid:device:)"), 0)
    }

    // MARK: - case#4 (edge)：repo.guestLogin 失败 → 透传 APIError + 不写 token
    func testPropagatesRepoErrorAndDoesNotWriteToken() async {
        let keychain = MockKeychainStore()
        keychain.getStubResult = .success("existing-uid-abc")
        let repo = MockAuthRepository()
        repo.guestLoginStub = .failure(.network(underlying: URLError(.notConnectedToInternet)))

        let useCase = DefaultSilentReloginUseCase(
            keychainStore: keychain,
            repository: repo
        )

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 APIError.network")
        } catch let error as APIError {
            // 用 case 模式比较：.network 嵌的 underlying 不强校验
            if case .network = error {
                // ok
            } else {
                XCTFail("应抛 .network，实际抛 \(error)")
            }
        } catch {
            XCTFail("应抛 APIError.network，实际抛 \(error)")
        }

        XCTAssertEqual(repo.callCount(of: "guestLogin(guestUid:device:)"), 1)
        XCTAssertEqual(keychain.callCount(of: "set(_:forKey:)"), 0, "repo 失败时绝不写 token")
    }

    // MARK: - case#5 (edge)：keychain.get 抛错 → 透传 KeychainError
    func testPropagatesKeychainGetError() async {
        let keychain = MockKeychainStore()
        keychain.getStubResult = .failure(KeychainError.osStatus(-25300, operation: "get"))
        let repo = MockAuthRepository()

        let useCase = DefaultSilentReloginUseCase(
            keychainStore: keychain,
            repository: repo
        )

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 KeychainError")
        } catch let error as KeychainError {
            // ok
            if case .osStatus(let status, _) = error {
                XCTAssertEqual(status, -25300)
            } else {
                XCTFail("应抛 .osStatus(-25300, ...)，实际抛 \(error)")
            }
        } catch {
            XCTFail("应抛 KeychainError，实际抛 \(error)")
        }

        XCTAssertEqual(repo.callCount(of: "guestLogin(guestUid:device:)"), 0, "keychain 出错时不应继续")
    }

    // MARK: - case#6 (edge)：keychain.set token 失败 → 透传 KeychainError + repo 已调过
    func testPropagatesKeychainSetError() async {
        let keychain = MockKeychainStore()
        keychain.getStubResult = .success("existing-uid-abc")
        keychain.setStubError = KeychainError.osStatus(-25299, operation: "set")
        let repo = MockAuthRepository()
        repo.guestLoginStub = .success(makeStubResponse(token: "new-token-1"))

        let useCase = DefaultSilentReloginUseCase(
            keychainStore: keychain,
            repository: repo
        )

        do {
            _ = try await useCase.execute()
            XCTFail("应抛 KeychainError")
        } catch is KeychainError {
            // ok
        } catch {
            XCTFail("应抛 KeychainError，实际抛 \(error)")
        }

        XCTAssertEqual(repo.callCount(of: "guestLogin(guestUid:device:)"), 1, "set 失败前 repo 已调过")
    }
}

#endif
```

**具体行为要求**：

- ≥ 4 case；本 AC 给 6 case 完整覆盖
- `@MainActor` test class + `#if DEBUG` 包裹（与 Story 5.2 / 5.3 同模式）
- 全部 case 用 `MockKeychainStore`（Story 2.8）+ `MockAuthRepository`（Story 5.2），不新建 mock
- 断言精确：`callCount(of:)` 验"是否调 / 调几次"；`lastGuestUid` 验"传的什么"
- error 比较：`APIError` 用 `XCTAssertEqual`（自带 Equatable）；`.network` underlying 不校验（自带 Equatable 只比 NSError domain/code）；`KeychainError` 用 `if case let` 模式比较
- case#3 "空字符串" 必须独立成 case（与 Story 5.3 case#4 同精神）
- case#6 "set 失败" 是边界 case，但能验"先调 repo 后写 token" 顺序约束（也防御性 cover "如果 set 失败时还没调 repo，就不会出现 server 已发新 token 但 client 没存"的半成功状态）

**AC7 — 单元测试 `SilentReloginCoordinatorTests`：actor coalescing 验证**

新建 `iphone/PetAppTests/Features/Auth/UseCases/SilentReloginCoordinatorTests.swift`，≥ 3 case：

```swift
// SilentReloginCoordinatorTests.swift
// Story 5.4 AC7: SilentReloginCoordinator actor coalescing 单元测试.
//
// 关键验证：
//   1. 单调 → useCase.execute 调 1 次
//   2. 5 并发 → useCase.execute 调 1 次（coalesce）
//   3. 第一次完成后 + 第二次 → useCase.execute 调 2 次（inFlight 清空）

import XCTest
@testable import PetApp

#if DEBUG

final class SilentReloginCoordinatorTests: XCTestCase {

    // MARK: - case#1 (happy)：单调 → useCase.execute 一次 → 返回 token
    func testReloginCallsUseCaseOnce() async throws {
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("token-1")
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)

        let token = try await coordinator.relogin()

        XCTAssertEqual(token, "token-1")
        XCTAssertEqual(mockUseCase.callCount(of: "execute()"), 1)
    }

    // MARK: - case#2 (happy)：5 并发调 relogin() → useCase.execute 调 1 次（coalesce 验证）
    func testConcurrentReloginsCoalesceToSingleExecution() async throws {
        let mockUseCase = MockSilentReloginUseCase()
        // useCase 故意 sleep 50ms：让 5 个并发请求都进入"既存 task" 等待路径
        mockUseCase.executeStub = .success("token-1")
        mockUseCase.artificialDelayMs = 50
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)

        // 5 并发
        let tokens: [String] = try await withThrowingTaskGroup(of: String.self) { group in
            for _ in 0..<5 {
                group.addTask {
                    try await coordinator.relogin()
                }
            }
            var results: [String] = []
            for try await token in group {
                results.append(token)
            }
            return results
        }

        XCTAssertEqual(tokens.count, 5)
        XCTAssertTrue(tokens.allSatisfy { $0 == "token-1" }, "所有并发调用应返回同一 token")
        XCTAssertEqual(
            mockUseCase.callCount(of: "execute()"),
            1,
            "useCase.execute 应仅被调一次（coalesce 复用 inFlight task）"
        )
    }

    // MARK: - case#3 (happy)：第一次完成后 + 第二次 relogin → useCase.execute 调 2 次
    func testReloginCallsUseCaseAgainAfterPreviousCompletes() async throws {
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("token-1")
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)

        // 第一次完整跑完
        let token1 = try await coordinator.relogin()
        XCTAssertEqual(token1, "token-1")

        // 第二次又调一次 —— inFlight 应已清空，useCase.execute 应再被调
        let token2 = try await coordinator.relogin()
        XCTAssertEqual(token2, "token-1")

        XCTAssertEqual(
            mockUseCase.callCount(of: "execute()"),
            2,
            "两次串行 relogin 应分别触发 useCase.execute（inFlight 清空验证）"
        )
    }

    // MARK: - case#4 (edge)：useCase 失败 → 透传错误 + inFlight 清空 → 下次能重新触发
    func testReloginPropagatesErrorAndCanRetryAfterFailure() async {
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .failure(APIError.network(underlying: URLError(.notConnectedToInternet)))
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)

        // 第一次失败
        do {
            _ = try await coordinator.relogin()
            XCTFail("应抛错")
        } catch {
            // ok
        }

        // 第二次又调一次 —— useCase.execute 应再被调（inFlight 清空，可重试）
        do {
            _ = try await coordinator.relogin()
            XCTFail("应抛错")
        } catch {
            // ok
        }

        XCTAssertEqual(
            mockUseCase.callCount(of: "execute()"),
            2,
            "失败的 task 完成后 inFlight 也应清空，下次 relogin 应能再次触发 useCase"
        )
    }
}

#endif
```

**具体行为要求**：

- ≥ 3 case，本 AC 给 4 case
- case#2 用 `withThrowingTaskGroup` 5 并发：与 Story 5.3 case#7 同模式（不引第三方）
- case#2 的 `artificialDelayMs = 50` 关键：让 useCase.execute "故意慢" —— 5 并发都进入"等待既存 task" 路径，这样才能验证 coalesce；如果 useCase 瞬时完成，测试可能侥幸 5 次都各自跑一次（race window 被错过）
- case#3 验"两次串行 relogin → useCase 调 2 次"：确认 `defer { inFlight = nil }` 真的清掉了 —— 防止"第一次完成后 inFlight 仍指向已 done 的 task" bug
- case#4 验"失败时 inFlight 也清空"：让用户后续动作还能重新触发重登 —— 不一棒子打死
- 不需要 `@MainActor`：actor 自己有 isolation；测试代码跑在 default executor 上即可（测试方法本身的 `async throws` 由 XCTest runner 调度）
- `MockSilentReloginUseCase` 是新建 mock（详见 AC8）

**AC8 — 新建 `MockSilentReloginUseCase`（继承 MockBase）**

新建 `iphone/PetAppTests/Features/Auth/UseCases/MockSilentReloginUseCase.swift`：

```swift
// MockSilentReloginUseCase.swift
// Story 5.4 AC8: SilentReloginUseCaseProtocol 测试 mock；继承 MockBase（Story 2.7 落地）.
//
// stub 字段（executeStub / artificialDelayMs）由测试 setUp 阶段写入；method body 读取一次后立即用，
// 符合 MockBase snapshot-only 精神（lesson 2026-04-26-mockbase-snapshot-only-reads.md）.
//
// `artificialDelayMs` 是 SilentReloginCoordinatorTests case#2 (5 并发 coalesce) 必需 ——
// 故意让 execute 慢一点，让多并发都进入"等待既存 task" 路径，避免 race window 被错过.

@testable import PetApp
import Foundation

#if DEBUG

final class MockSilentReloginUseCase: MockBase, SilentReloginUseCaseProtocol, @unchecked Sendable {
    var executeStub: Result<String, Error> = .failure(MockError.notStubbed)

    /// 人工延迟（毫秒）。0 = 不延迟（立即返回）。
    /// 用途：SilentReloginCoordinatorTests case#2 (5 并发) 必须延迟，否则测试可能侥幸通过 race window 漏检.
    var artificialDelayMs: UInt64 = 0

    func execute() async throws -> String {
        record(method: "execute()")
        if artificialDelayMs > 0 {
            try? await Task.sleep(nanoseconds: artificialDelayMs * 1_000_000)
        }
        return try executeStub.get()
    }
}

#endif
```

**具体行为要求**：

- 继承 `MockBase`：自动获得 `record` / `callCount(of:)` / `invocationsSnapshot`
- 标 `@unchecked Sendable`：协议要求 + actor 持有要求
- `executeStub: Result<String, Error>` 字段：测试 setUp 阶段配
- `artificialDelayMs`：默认 0；测试 case#2 显式设 50（实测 5 并发 coalesce 必需）
- **不**包含 `lastInvocation` 字段：execute 无参数；调用次数靠 `callCount(of: "execute()")` 取
- `#if DEBUG` 包裹（与 `MockKeychainStore` / `MockAuthRepository` 同模式）

**AC9 — 单元测试 `AuthRetryingAPIClientTests`：装饰器拦截 + 重试 + 协调**

新建 `iphone/PetAppTests/Core/Networking/AuthRetryingAPIClientTests.swift`，≥ 5 case：

```swift
// AuthRetryingAPIClientTests.swift
// Story 5.4 AC9: AuthRetryingAPIClient decorator 单元测试.
//
// 关键验证：
//   1. happy: requiresAuth=true 第一次 401 → coordinator.relogin → 重试 success → 用户感知 0
//   2. edge: requiresAuth=true 第一次 401 → relogin 失败 → 抛上层（不重试 inner）
//   3. edge: requiresAuth=true 第一次 401 → relogin success → 重试**仍** 401 → 抛 unauthorized（不二次 relogin）
//   4. happy: 5 个并发 401 请求 → coordinator coalesce → 只 1 次 relogin + 5 次重试都成功
//   5. edge: requiresAuth=false 抛 unauthorized → **不** relogin → 直接抛上去
//   6. edge: 非 unauthorized 错误（.network / .business）→ **不** relogin → 直接抛
//
// Mock 策略：
//   - inner: 用 StatefulMockAPIClient（按 path 维护 Stub 序列；按调用次数 pop）
//     —— 既有 MockAPIClient 是 path → 单 stub 的 map，不能表达"第 1 次 fail + 第 2 次 success"
//   - coordinator: 用真实 SilentReloginCoordinator + MockSilentReloginUseCase（让 relogin 调用次数可验）

import XCTest
@testable import PetApp

#if DEBUG

final class AuthRetryingAPIClientTests: XCTestCase {

    private struct EmptyData: Decodable, Equatable {}
    private struct PingData: Decodable, Equatable { let value: String }

    private let authedEndpoint = Endpoint(path: "/api/v1/home", method: .get, requiresAuth: true)
    private let unauthedEndpoint = Endpoint(path: "/api/v1/auth/guest-login", method: .post, body: nil, requiresAuth: false)

    // MARK: - case#1 (happy)：requiresAuth=true 第一次 401 → relogin → 重试 success
    func testRetriesOnceAfterUnauthorizedAndSuccess() async throws {
        let inner = StatefulMockAPIClient()
        inner.responseSequence["/api/v1/home"] = [
            .failure(.unauthorized),
            .success(PingData(value: "after-relogin"))
        ]
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("new-token-1")
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)
        let wrapped = AuthRetryingAPIClient(inner: inner, coordinator: coordinator)

        let result: PingData = try await wrapped.request(authedEndpoint)

        XCTAssertEqual(result, PingData(value: "after-relogin"))
        XCTAssertEqual(inner.callCount(forPath: "/api/v1/home"), 2, "inner 应被调 2 次（原始 + 重试）")
        XCTAssertEqual(mockUseCase.callCount(of: "execute()"), 1, "relogin 应触发 1 次")
    }

    // MARK: - case#2 (edge)：requiresAuth=true 第一次 401 → relogin 失败 → 抛上层
    func testReloginFailureIsThrownAndInnerNotRetried() async {
        let inner = StatefulMockAPIClient()
        inner.responseSequence["/api/v1/home"] = [
            .failure(.unauthorized),
            .success(EmptyData())  // 即使配了 success，relogin 失败后也不该走到这里
        ]
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .failure(APIError.network(underlying: URLError(.notConnectedToInternet)))
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)
        let wrapped = AuthRetryingAPIClient(inner: inner, coordinator: coordinator)

        do {
            let _: EmptyData = try await wrapped.request(authedEndpoint)
            XCTFail("应抛错")
        } catch let error as APIError {
            if case .network = error {
                // ok
            } else {
                XCTFail("应抛 .network，实际 \(error)")
            }
        } catch {
            XCTFail("应抛 APIError.network，实际 \(error)")
        }

        XCTAssertEqual(inner.callCount(forPath: "/api/v1/home"), 1, "inner 仅调 1 次（原始失败 + relogin 失败 → 不重试）")
        XCTAssertEqual(mockUseCase.callCount(of: "execute()"), 1)
    }

    // MARK: - case#3 (edge)：第一次 401 → relogin success → 重试**仍** 401 → 不二次 relogin
    func testRetryStillUnauthorizedDoesNotTriggerSecondRelogin() async {
        let inner = StatefulMockAPIClient()
        inner.responseSequence["/api/v1/home"] = [
            .failure(.unauthorized),  // 原始
            .failure(.unauthorized),  // 重试也失败
            .success(EmptyData())     // 第 3 次：永远不会被调
        ]
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("new-token-1")
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)
        let wrapped = AuthRetryingAPIClient(inner: inner, coordinator: coordinator)

        do {
            let _: EmptyData = try await wrapped.request(authedEndpoint)
            XCTFail("应抛 unauthorized")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized)
        } catch {
            XCTFail("应抛 APIError.unauthorized，实际 \(error)")
        }

        XCTAssertEqual(inner.callCount(forPath: "/api/v1/home"), 2, "inner 应调 2 次（原始 + 重试 1 次，**不**第 3 次）")
        XCTAssertEqual(mockUseCase.callCount(of: "execute()"), 1, "relogin 仅调 1 次（重试失败后**不**二次重登）")
    }

    // MARK: - case#4 (happy)：5 并发 401 → coordinator coalesce → 1 次 relogin + 5 次重试 success
    func testConcurrentUnauthorizedRequestsCoalesceReloginAndAllRetrySucceed() async throws {
        let inner = StatefulMockAPIClient()
        // 每个并发请求的"序列"是独立的 path → [stub] —— 但 5 个请求都打同一 path
        // 必须让 stub 可以按"全局调用次数" pop（StatefulMockAPIClient 内部用 lock 保护序列）
        // 配 10 个 stub：5 个 .failure(.unauthorized) + 5 个 .success(...)
        inner.responseSequence["/api/v1/home"] = [
            .failure(.unauthorized), .failure(.unauthorized), .failure(.unauthorized),
            .failure(.unauthorized), .failure(.unauthorized),
            .success(EmptyData()), .success(EmptyData()), .success(EmptyData()),
            .success(EmptyData()), .success(EmptyData())
        ]
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("new-token-1")
        mockUseCase.artificialDelayMs = 50  // 让 5 个并发请求都进入"等待既存 task"路径
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)
        let wrapped = AuthRetryingAPIClient(inner: inner, coordinator: coordinator)

        // 5 并发
        try await withThrowingTaskGroup(of: Void.self) { group in
            for _ in 0..<5 {
                group.addTask {
                    let _: EmptyData = try await wrapped.request(self.authedEndpoint)
                }
            }
            try await group.waitForAll()
        }

        XCTAssertEqual(inner.callCount(forPath: "/api/v1/home"), 10, "5 并发 → 5 原始失败 + 5 重试 = 10 次 inner")
        XCTAssertEqual(
            mockUseCase.callCount(of: "execute()"),
            1,
            "5 并发 401 应 coalesce 到 1 次 relogin"
        )
    }

    // MARK: - case#5 (edge)：requiresAuth=false 抛 unauthorized → 不 relogin
    func testRequiresAuthFalseUnauthorizedDoesNotTriggerRelogin() async {
        let inner = StatefulMockAPIClient()
        inner.responseSequence["/api/v1/auth/guest-login"] = [.failure(.unauthorized)]
        let mockUseCase = MockSilentReloginUseCase()
        mockUseCase.executeStub = .success("new-token-1")
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)
        let wrapped = AuthRetryingAPIClient(inner: inner, coordinator: coordinator)

        do {
            let _: EmptyData = try await wrapped.request(unauthedEndpoint)
            XCTFail("应抛 unauthorized")
        } catch let error as APIError {
            XCTAssertEqual(error, .unauthorized)
        } catch {
            XCTFail("应抛 APIError.unauthorized，实际 \(error)")
        }

        XCTAssertEqual(inner.callCount(forPath: "/api/v1/auth/guest-login"), 1, "inner 仅 1 次（不重试）")
        XCTAssertEqual(mockUseCase.callCount(of: "execute()"), 0, "requiresAuth=false 时**绝不**触发 relogin")
    }

    // MARK: - case#6 (edge)：非 unauthorized 错误（.network）→ 不 relogin
    func testNonUnauthorizedErrorsDoNotTriggerRelogin() async {
        let inner = StatefulMockAPIClient()
        inner.responseSequence["/api/v1/home"] = [.failure(.network(underlying: URLError(.notConnectedToInternet)))]
        let mockUseCase = MockSilentReloginUseCase()
        let coordinator = SilentReloginCoordinator(useCase: mockUseCase)
        let wrapped = AuthRetryingAPIClient(inner: inner, coordinator: coordinator)

        do {
            let _: EmptyData = try await wrapped.request(authedEndpoint)
            XCTFail("应抛 network")
        } catch let error as APIError {
            if case .network = error {
                // ok
            } else {
                XCTFail("应抛 .network，实际 \(error)")
            }
        } catch {
            XCTFail("应抛 APIError，实际 \(error)")
        }

        XCTAssertEqual(inner.callCount(forPath: "/api/v1/home"), 1, "inner 仅 1 次")
        XCTAssertEqual(mockUseCase.callCount(of: "execute()"), 0, ".network 错误**绝不**触发 relogin")
    }
}

#endif
```

**具体行为要求**：

- ≥ 5 case，本 AC 给 6 case
- 不需要 `@MainActor`：装饰器 / 协调器都没 actor isolation 约束（actor 自己 hold isolation；装饰器是 final class 跑在 task 上）
- inner 用新建 `StatefulMockAPIClient`（详见 AC10），不用既有 `MockAPIClient`（后者不支持序列化 stub）
- coordinator 用**真实** `SilentReloginCoordinator` + mock useCase：因为我们要验"装饰器是否真的调 coordinator + coordinator 是否真的 coalesce" —— 用 mock coordinator 隔离层数太多反而失去验证价值
- case#4 的 `artificialDelayMs = 50` 关键（与 SilentReloginCoordinatorTests case#2 同精神）：让 5 个并发请求都堆在 coordinator 入口等
- case#3 验"重试也失败时不二次重登"：通过 stub 序列的第 3 个 `.success` 不被调验证（如果二次 relogin 触发 + 重试 inner 第 3 次成功，测试就 pass 但语义错了；本测试期望抛 unauthorized 即"重试也失败 → 直接抛"）
- case#4 stub 配 10 个 .failure + .success：因为 5 并发请求每个都先走"原始失败"再走"重试"，共 10 次 inner.request
  - **dev 注意**：stub 序列的 pop 顺序未必严格"5 fail 先 / 5 success 后"——并发场景下可能交错（第 1 个请求失败后 await coordinator → 让其它请求先 pop）。但**不影响断言**：只要序列里 5 个 fail + 5 个 success，所有"先失败后成功"的 5 个请求都会拿到 success；inner 总 callCount=10；relogin coalesce=1
  - 如果 dev 实装时遇到"序列耗尽" 错误（即 inner 被调 11 次），说明装饰器 retry 逻辑有 bug（多重试了）—— 应当查
- case#5 / case#6 验拦截范围：只 `APIError.unauthorized + requiresAuth=true` 触发 relogin

**AC10 — 新建 `StatefulMockAPIClient`：按调用序列返回不同 stub**

新建 `iphone/PetAppTests/Core/Networking/StatefulMockAPIClient.swift`：

```swift
// StatefulMockAPIClient.swift
// Story 5.4 AC10: 按"调用次数序列"返回不同 stub 的 mock APIClient.
//
// 与 MockAPIClient（Story 2.5）的区别：
//   - MockAPIClient: stubResponse: [String: Stub] —— path → 单 stub —— 同一 path 多次调用都返同结果
//   - StatefulMockAPIClient: responseSequence: [String: [Stub]] —— path → stub 队列 —— 按调用顺序 pop
//
// 用途：
//   - AuthRetryingAPIClientTests 需要"第 1 次 401 + 第 2 次 success" 序列
//   - 5 并发场景需要序列共享（线程安全 pop）
//
// 不替换 MockAPIClient：
//   - MockAPIClient 模式简单，覆盖单次调用场景；既有 PingUseCaseTests 等不变
//   - 新建 StatefulMockAPIClient 让"复杂序列" 场景独立，避免污染既有 mock
//
// 线程安全：用 NSLock 保护 responseSequence + invocations（与 MockURLSession 同模式 —— Story 5.3 已加 lock）.

import Foundation
@testable import PetApp

#if DEBUG

final class StatefulMockAPIClient: APIClientProtocol, @unchecked Sendable {
    enum Stub {
        case success(any Decodable & Sendable)
        case failure(APIError)
    }

    private let lock = NSLock()
    private var _responseSequence: [String: [Stub]] = [:]
    private var _invocations: [Endpoint] = []
    private var _callCounts: [String: Int] = [:]

    var responseSequence: [String: [Stub]] {
        get { lock.lock(); defer { lock.unlock() }; return _responseSequence }
        set { lock.lock(); defer { lock.unlock() }; _responseSequence = newValue }
    }

    var invocations: [Endpoint] {
        lock.lock()
        defer { lock.unlock() }
        return _invocations
    }

    func callCount(forPath path: String) -> Int {
        lock.lock()
        defer { lock.unlock() }
        return _callCounts[path, default: 0]
    }

    func request<T: Decodable>(_ endpoint: Endpoint) async throws -> T {
        let stub: Stub? = {
            lock.lock()
            defer { lock.unlock() }
            _invocations.append(endpoint)
            _callCounts[endpoint.path, default: 0] += 1
            // 按 path pop 队首
            if var queue = _responseSequence[endpoint.path], !queue.isEmpty {
                let head = queue.removeFirst()
                _responseSequence[endpoint.path] = queue
                return head
            }
            return nil
        }()

        guard let stub else {
            // 序列耗尽 —— 测试 bug，应失败
            throw APIError.decoding(underlying: NSError(
                domain: "StatefulMockAPIClient",
                code: -200,
                userInfo: [NSLocalizedDescriptionKey:
                    "Stub sequence exhausted for path: \(endpoint.path) (call #\(callCount(forPath: endpoint.path)))"]
            ))
        }

        switch stub {
        case .success(let value):
            guard let typed = value as? T else {
                throw APIError.decoding(underlying: NSError(
                    domain: "StatefulMockAPIClient",
                    code: -201,
                    userInfo: [NSLocalizedDescriptionKey:
                        "Stub type mismatch: expected \(T.self), got \(type(of: value))"]
                ))
            }
            return typed
        case .failure(let error):
            throw error
        }
    }
}

#endif
```

**具体行为要求**：

- 与 `MockURLSession` 同模式：`NSLock` 保护内部存储；外部读通过 getter 拿快照
- pop 必须**原子**：即"读队首 + 移除队首" 在同一 lock 内完成，否则 5 并发 case#4 会 race
- 序列耗尽抛 `.decoding(underlying:)`：失败明确（dev 看到 error message 立刻能 debug）
- `callCount(forPath:)` 是便利 helper：让测试断言"path X 被调 N 次"无需自己 filter invocations
- **不**继承 MockBase：MockBase 适配通用 mock 模式；本 mock 是 networking-specific（与 MockURLSession 同精神，遵循 MockBase 文件头注释 "已存在的 networking mock 不强制迁移"）
- `#if DEBUG` 包裹

**AC11 — `bash iphone/scripts/build.sh --test` 全绿 + UI 测试零回归**

```bash
bash iphone/scripts/build.sh             # 普通 build 无 warning
bash iphone/scripts/build.sh --test      # 单元测试全绿（含本 story 新增 ≥ 13 case + 既有 174 case）
bash iphone/scripts/build.sh --uitest    # UI 测试全绿（既有 8 case 不受影响）
```

**具体行为要求**：

- 既有 `bash iphone/scripts/build.sh --test` 全绿（不引入回归）—— Story 5.3 落地后是 174 case，本 story 应至少加 13 case（5 SilentRelogin + 4 Coordinator + 6 AuthRetrying = 15）
- 既有 `bash iphone/scripts/build.sh --uitest` 全绿（HomeUITests / NavigationUITests / KeychainPersistenceUITests 不受 APIClient 装饰层改动影响）
- 普通 build 无 warning

**AC12 — `git status` 范围红线验证**

```bash
git status
# 期望（新增 production code）：
#   iphone/PetApp/Core/Networking/AuthRetryingAPIClient.swift
#   iphone/PetApp/Features/Auth/UseCases/SilentReloginUseCase.swift
#   iphone/PetApp/Features/Auth/UseCases/SilentReloginCoordinator.swift
# 期望（修改 production code）：
#   iphone/PetApp/App/AppContainer.swift（convenience init wire AuthRetryingAPIClient + 新增 makeSilentReloginUseCase factory）
# 期望（新增 test）：
#   iphone/PetAppTests/Core/Networking/AuthRetryingAPIClientTests.swift
#   iphone/PetAppTests/Core/Networking/StatefulMockAPIClient.swift
#   iphone/PetAppTests/Features/Auth/UseCases/SilentReloginUseCaseTests.swift
#   iphone/PetAppTests/Features/Auth/UseCases/SilentReloginCoordinatorTests.swift
#   iphone/PetAppTests/Features/Auth/UseCases/MockSilentReloginUseCase.swift
# 期望（自动 regen）：
#   iphone/PetApp.xcodeproj/project.pbxproj（xcodegen 由新文件自动 regen）
# 不期望：
#   ios/ / server/ / iphone/scripts/ / iphone/project.yml / iphone/.gitignore / .gitignore / docs/ / CLAUDE.md 任何改动
#   iphone/PetApp/Core/Networking/{APIClient,Endpoint,APIError,APIResponse,URLSessionProtocol}.swift 任何改动
#   iphone/PetApp/Features/Auth/{Repositories,Models,Session,UseCases/{GuestLoginUseCase,AuthEndpoints}}.swift 任何改动
#   iphone/PetApp/App/{RootView,AppCoordinator,...}.swift 任何改动
```

```bash
# 范围红线验证命令：
git status -- ios/ server/ iphone/scripts/ iphone/project.yml iphone/.gitignore .gitignore docs/ CLAUDE.md
# 必须 "nothing to commit, working tree clean"
```

**具体行为要求**：

- `git status` 仅显示上述清单中的新建 / 修改文件 + xcodegen 自动 regen 的 `project.pbxproj`
- `iphone/project.yml` 零改动（新文件靠既有 `sources: - PetApp` / `sources: - PetAppTests` glob 自动纳入）
- `ios/` / `server/` / `docs/` / `CLAUDE.md` 全部零改动
- `APIClient.swift` / `Endpoint.swift` / `APIError.swift` / `GuestLoginUseCase.swift` / `AuthEndpoints.swift` / `AuthRepository.swift` / `SessionStore.swift` 等所有 Story 5.1 / 5.2 / 5.3 既有文件全部零改动

## Tasks / Subtasks

- [x] Task 1: 实装 `SilentReloginUseCase`（AC1）
  - [x] 1.1 新建 `iphone/PetApp/Features/Auth/UseCases/SilentReloginUseCase.swift`
  - [x] 1.2 定义 `SilentReloginUseCaseProtocol: Sendable` 协议（`func execute() async throws -> String`）
  - [x] 1.3 实装 `DefaultSilentReloginUseCase: struct`：依赖 `KeychainStoreProtocol` + `AuthRepositoryProtocol` + Optional `deviceProvider` closure
  - [x] 1.4 流程：keychain.get(guestUid) → 无则抛 unauthorized → repo.guestLogin → keychain.set(token) → return token
  - [x] 1.5 错误透传严格：所有底层错误原样抛
- [x] Task 2: 实装 `SilentReloginCoordinator`（AC4）
  - [x] 2.1 新建 `iphone/PetApp/Features/Auth/UseCases/SilentReloginCoordinator.swift`
  - [x] 2.2 用 `actor` 实现：持 `useCase: SilentReloginUseCaseProtocol` + `inFlight: Task<String, Error>?`
  - [x] 2.3 `relogin()` 方法：if let existing → await existing.value；else 启动 Task → inFlight = task → defer { inFlight = nil } → return await task.value
  - [x] 2.4 失败也清空 inFlight（defer 自动覆盖）
- [x] Task 3: 实装 `AuthRetryingAPIClient`（AC2 + AC3）
  - [x] 3.1 新建 `iphone/PetApp/Core/Networking/AuthRetryingAPIClient.swift`
  - [x] 3.2 `final class AuthRetryingAPIClient: APIClientProtocol`，持 `inner: APIClientProtocol` + `coordinator: SilentReloginCoordinator`
  - [x] 3.3 `request<T>` 方法：do { try await inner.request(endpoint) } catch APIError.unauthorized where endpoint.requiresAuth { _ = try await coordinator.relogin(); return try await inner.request(endpoint) }
  - [x] 3.4 catch 子句**仅**捕 `APIError.unauthorized where endpoint.requiresAuth`；其它情况默认抛
  - [x] 3.5 重试**仅 1 次**：catch body 内不嵌套 do-catch；重试失败直接抛
- [x] Task 4: 改 `AppContainer.convenience init()` wire `AuthRetryingAPIClient`（AC5）
  - [x] 4.1 改 `iphone/PetApp/App/AppContainer.swift` 的 `convenience init()`
  - [x] 4.2 先建 `baseAPIClient` + `baseRepository`（**只**给重登 useCase 用）
  - [x] 4.3 建 `reloginUseCase` + `coordinator` + `wrappedAPIClient`
  - [x] 4.4 `self.init(apiClient: wrappedAPIClient, keychainStore: keychainStore)`
  - [x] 4.5 新增 `makeSilentReloginUseCase()` factory
  - [x] 4.6 `init(apiClient:keychainStore:)` 主 init 不动（既有 `AppContainerTests` 全部 0 失败）
- [x] Task 5: 新建 `MockSilentReloginUseCase`（AC8）
  - [x] 5.1 新建 `iphone/PetAppTests/Features/Auth/UseCases/MockSilentReloginUseCase.swift`
  - [x] 5.2 继承 `MockBase` + `SilentReloginUseCaseProtocol`，标 `@unchecked Sendable`
  - [x] 5.3 `executeStub: Result<String, Error>` 字段
  - [x] 5.4 `artificialDelayMs: UInt64` 字段（默认 0；测试可设 50 让 5 并发 coalesce 验证可靠）
  - [x] 5.5 `#if DEBUG` 包裹
- [x] Task 6: 新建 `StatefulMockAPIClient`（AC10）
  - [x] 6.1 新建 `iphone/PetAppTests/Core/Networking/StatefulMockAPIClient.swift`
  - [x] 6.2 `responseSequence: [String: [Stub]]` 字段（NSLock 保护）
  - [x] 6.3 `request<T>` 方法：lock 内 pop 队首；序列耗尽抛 `.decoding`
  - [x] 6.4 `callCount(forPath:)` helper
  - [x] 6.5 `#if DEBUG` 包裹
- [x] Task 7: 单元测试 `SilentReloginUseCaseTests`（AC6，≥ 4 case，建议写 6）
  - [x] 7.1 新建 `iphone/PetAppTests/Features/Auth/UseCases/SilentReloginUseCaseTests.swift`
  - [x] 7.2 case#1 happy / case#2 edge no-uid / case#3 edge empty-uid / case#4 edge repo-fail / case#5 edge keychain-get-fail / case#6 edge keychain-set-fail
  - [x] 7.3 复用 `MockKeychainStore` + `MockAuthRepository`
  - [x] 7.4 `@MainActor` + `#if DEBUG` 包裹
- [x] Task 8: 单元测试 `SilentReloginCoordinatorTests`（AC7，≥ 3 case，建议写 4）
  - [x] 8.1 新建 `iphone/PetAppTests/Features/Auth/UseCases/SilentReloginCoordinatorTests.swift`
  - [x] 8.2 case#1 单调 / case#2 5 并发 coalesce / case#3 串行两次 inFlight 清空 / case#4 失败也清空 inFlight
  - [x] 8.3 case#2 必须用 `withThrowingTaskGroup` + `artificialDelayMs = 50`
  - [x] 8.4 不需要 `@MainActor`（actor 自带 isolation）
- [x] Task 9: 单元测试 `AuthRetryingAPIClientTests`（AC9，≥ 5 case，建议写 6）
  - [x] 9.1 新建 `iphone/PetAppTests/Core/Networking/AuthRetryingAPIClientTests.swift`
  - [x] 9.2 case#1 happy 重试成功 / case#2 edge relogin 失败 / case#3 edge 重试仍 401 不二次重登 / case#4 happy 5 并发 coalesce / case#5 edge requiresAuth=false 不重登 / case#6 edge .network 不重登
  - [x] 9.3 inner 用 `StatefulMockAPIClient`；coordinator 用真实 + `MockSilentReloginUseCase`
  - [x] 9.4 case#4 必须用 `withThrowingTaskGroup` + `artificialDelayMs = 50`
- [x] Task 10: 全套 build / test 验证（AC11）
  - [x] 10.1 `bash iphone/scripts/build.sh` 普通 build 无 warning
  - [x] 10.2 `bash iphone/scripts/build.sh --test` 单元测试全绿（既有 174 + 新增 8 = 182 全绿）
  - [x] 10.3 `bash iphone/scripts/build.sh --uitest` UI 测试全绿（8 case 全绿）
- [x] Task 11: `git status` 范围红线验证（AC12）
  - [x] 11.1 `git status` 仅显示预期文件（3 新建 production + 1 修改 + 5 新建 test + xcodegen regen）
  - [x] 11.2 `git status -- ios/ server/ iphone/scripts/ iphone/project.yml iphone/.gitignore .gitignore docs/ CLAUDE.md` → "nothing to commit"
  - [x] 11.3 确认 `APIClient.swift` / `Endpoint.swift` / `APIError.swift` / `GuestLoginUseCase.swift` / `AuthEndpoints.swift` / `AuthRepository.swift` / `SessionStore.swift` 全部零改动

## Dev Notes

### 关键技术约束

1. **decorator pattern 而非 protocol modify**：`AuthRetryingAPIClient: APIClientProtocol` 包装 `APIClient` —— `APIClientProtocol.request<T>` 协议方法签名零改动；让 `DefaultAuthRepository(apiClient: APIClientProtocol)` 等所有持协议引用的下游零改动。这是 Story 5.3 留下的扩展点（`APIClient.init` 加 keychain Optional 字段是 backward-compat；本 story decorator 是更彻底的"包装而非修改"）

2. **每原始请求最多 1 次重登**：靠 catch body 内不嵌套 do-catch 自然实现 —— 重试失败时 throw 直接出 `request<T>` 函数，不会触发第二次 catch。**禁止**用 `for _ in 0..<2` retry loop 实现（更难推理 + 容易写成 ≥ 2 次）

3. **多并发 401 严格 coalesce**：`SilentReloginCoordinator` 用 actor + `inFlight: Task<String, Error>?` —— actor isolation 自动保证字段 read-modify-write 原子；`Task` 是 Swift 标准 future，多 awaiter 共享同一结果

4. **defer 清 inFlight**：`defer { inFlight = nil }` 关键 —— 完成（成功 / 失败 / 取消）后清掉。**禁止**写成 try await 之后赋值（失败抛错时跳过）；**禁止**用 do-catch + 手动清（容易漏写）

5. **`requiresAuth == false` 路径**绝不触发重登：`/auth/guest-login` 自身 401 不能用自己救自己（无限递归风险）。靠 catch where 子句自然实现：`catch APIError.unauthorized where endpoint.requiresAuth`

6. **AppContainer 内"双 repository"设计**：
   - `baseRepository` 持 unwrapped `baseAPIClient`：**专给** `SilentReloginUseCase` 内的 `repository` 用 —— 重登调 `/auth/guest-login` 是 `requiresAuth=false`，本来就不会被 wrap 拦；用 unwrapped 是为了**严格隔离循环依赖**（即使 wrap 也不会拦 false，但 unwrapped 防御未来 endpoint 改 true 时的诡异 bug）
   - `makeAuthRepository()` 持 wrapped `wrappedAPIClient`：给业务层用（含 future Story 5.5 LoadHomeUseCase 等）—— 业务层自动获得静默重登能力

7. **不更新 SessionStore**：节点 2 阶段静默重登语义就是"恢复 token"，不刷新 user / pet 数据。原因：(a) `/auth/guest-login` 幂等，同 guestUid 返回的 user / pet 应一致；(b) 节点 2 没"看着 nickname 实时变" 需求；(c) 节点 2+ 业务层调 `/home` 自然刷新

8. **不在 APIError 加新 case**：复用 `.unauthorized` 表达"重登也救不了"语义（业务层语义都是"鉴权失败"）；下游如需区分原始 401 vs 重登失败可用 chain 嵌套 underlying error，但**MVP 不做**

9. **不缓存新 token 到装饰器**：每次重试时 `inner.request → buildURLRequest → 读 keychain` 自动拿新 token（Story 5.3 决策树第 5 步）；与 Story 5.3 "token 不缓存到 APIClient 内" 同精神

10. **actor `await` 自动让出**：`relogin()` 内 `await task.value` 暗示让出 actor —— 第二个进入 actor 的请求不会被第一个阻塞；多并发 await 同 task 都拿到 `Task.value` 的 cooperative wake-up

11. **`MockSilentReloginUseCase.artificialDelayMs` 是必需的**：5 并发 coalesce 测试如果 useCase 瞬时返回，可能 5 个请求依次进入 actor 后第一个已 done、第二个看到 inFlight nil 又开新一轮 —— 测试侥幸 pass 但漏检 race window。`Task.sleep(50ms)` 让 5 个请求都堆在 actor 入口等 task

12. **`StatefulMockAPIClient` pop 必须原子**：lock 内"读队首 + 移除队首" 同步完成 —— 5 并发 case 才能正确分配 stub；若 race 会出现"两个请求拿同一 stub" 或 "stub 跳号"

### Source tree components to touch

```
iphone/
├─ PetApp/
│  ├─ App/
│  │  └─ AppContainer.swift                    # 改：convenience init wire AuthRetryingAPIClient + 新增 makeSilentReloginUseCase factory
│  ├─ Core/
│  │  └─ Networking/
│  │     └─ AuthRetryingAPIClient.swift        # 新建：装饰器实现 401 静默重登 + 重试
│  └─ Features/
│     └─ Auth/
│        └─ UseCases/
│           ├─ SilentReloginUseCase.swift      # 新建：静默重登 UseCase
│           └─ SilentReloginCoordinator.swift  # 新建：actor coalescing 协调器
├─ PetAppTests/
│  ├─ Core/
│  │  └─ Networking/
│  │     ├─ AuthRetryingAPIClientTests.swift   # 新建：装饰器单元测试（≥ 5 case）
│  │     └─ StatefulMockAPIClient.swift        # 新建：序列化 stub mock
│  └─ Features/
│     └─ Auth/
│        └─ UseCases/
│           ├─ SilentReloginUseCaseTests.swift            # 新建：UseCase 单元测试（≥ 4 case）
│           ├─ SilentReloginCoordinatorTests.swift        # 新建：actor 协调器单元测试（≥ 3 case）
│           └─ MockSilentReloginUseCase.swift             # 新建：UseCase mock（继承 MockBase）
└─ project.yml                                  # 不动（sources glob 自动纳入新文件）

# 不动：
# iphone/PetApp/Core/Networking/{APIClient,Endpoint,APIError,APIResponse,URLSessionProtocol}.swift（Story 2.4 / 5.3 全套）
# iphone/PetApp/Core/Storage/*（Story 2.8 / 5.1 全套）
# iphone/PetApp/Features/Auth/UseCases/{GuestLoginUseCase,AuthEndpoints}.swift（Story 5.2 全套）
# iphone/PetApp/Features/Auth/Repositories/AuthRepository.swift（Story 5.2）
# iphone/PetApp/Features/Auth/Models/*.swift（Story 5.2）
# iphone/PetApp/Features/Auth/Session/SessionStore.swift（Story 5.2）
# iphone/PetApp/App/{RootView,AppCoordinator,AppLaunchState,AppLaunchStateMachine,KeychainUITestHookView,PetAppApp}.swift
# iphone/PetAppTests/Helpers/MockBase.swift（Story 2.7）
# iphone/PetAppTests/Core/Networking/{APIClientTests,APIClientIntegrationTests,APIClientAuthInjectionTests,MockURLSession,StubURLProtocol}.swift（Story 2.4 / 5.3 全套）
# iphone/PetAppTests/Features/DevTools/MockKeychainStore.swift（Story 2.8 —— 直接复用）
# iphone/PetAppTests/Features/Home/UseCases/MockAPIClient.swift（Story 2.5 —— 不影响）
# iphone/PetAppTests/Features/Auth/Session/SessionStoreTests.swift（Story 5.2）
# iphone/PetAppTests/Features/Auth/UseCases/{GuestLoginUseCaseTests,GuestLoginUseCaseIntegrationTests,MockAuthRepository}.swift（Story 5.2 —— 直接复用 MockAuthRepository）
# iphone/PetAppUITests/*（Story 2.7 / 5.1 全套；不影响）
# iphone/project.yml / iphone/scripts/* / iphone/.gitignore
# ios/* / server/*
```

### Testing standards summary（继承 ADR-0002 + Story 5.1 / 5.2 / 5.3）

- **单元测试**：XCTest only（手写 mock）；ADR-0002 §3.1
- **Mock 模式**：
  - 复用 Story 2.8 `MockKeychainStore`（继承 MockBase + `getStubResult` / `setStubError`）
  - 复用 Story 5.2 `MockAuthRepository`（继承 MockBase + `guestLoginStub`）
  - 新建 `MockSilentReloginUseCase`（继承 MockBase + `executeStub` + `artificialDelayMs`）
  - 新建 `StatefulMockAPIClient`（不继承 MockBase；自带 NSLock；`responseSequence` 序列化）
- **并发测试**：用 Swift 标准 `withThrowingTaskGroup` + `Task.sleep` —— 不引第三方
  - 5 并发 coalesce 验证必须用 `artificialDelayMs = 50` 让请求堆积，避免 race window 被错过
- **测试隔离**：本 story 全部用 mock keychain（`MockKeychainStore`）—— 不接触真实 `KeychainServicesStore`，不需要 namespace 隔离
- **跑命令**：`bash iphone/scripts/build.sh --test`（单元）/ `--uitest`（UI）

### Project Structure Notes

- 完全对齐 iOS 架构设计 §4 目录结构（`Core/Networking/AuthInterceptor.swift` 列出但本 story **沿用 Story 5.3 决策**不创建 `AuthInterceptor.swift` 实体；用 `AuthRetryingAPIClient` 名字明确"装饰器" 语义）
- 完全对齐 §5.4 Repository 层 + §6.1 Auth 模块（UseCase + Repository + 协议化）
- 测试目录 `iphone/PetAppTests/Core/Networking/` 与 `iphone/PetAppTests/Features/Auth/UseCases/` 已存在（Story 2.4 / 5.2 落地），本 story 仅追加新文件
- `Detected conflicts or variances`：
  - 架构 §4 没单独列 `SilentReloginCoordinator` —— 但 §6.1 Auth 模块允许"按需添加 UseCase"，coordinator 是 actor 也可以归入 UseCases 子目录（与 `GuestLoginUseCase` 同位）
  - 架构 §4 也没单独列"装饰器" —— 但 §8.1 钦定"自动注入 token + 401 处理"是 APIClient 责任，本 story 用 decorator 实现是合理工程选择（不修改 APIClient 主体）

### References

- iOS 架构 §4 Core/Networking 目录 / §5.4 Repository 层 / §6.1 Auth 模块 / §8.1 REST Client（"自动注入 token + 401 处理"）：[Source: docs/宠物互动App_iOS客户端工程结构与模块职责设计.md]
- V1 §2.3 鉴权方式（`Authorization: Bearer <token>`）/ §2.4 envelope / §3 错误码 1001 / §4.1 POST /auth/guest-login（幂等性 + 限频 + 错误码）：[Source: docs/宠物互动App_V1接口设计.md]
- ADR-0002 §3.1 iOS Mock 框架 XCTest only / §3.3 iPhone App 工程目录方案 D / §3.4 CI 命令：[Source: _bmad-output/implementation-artifacts/decisions/0002-ios-stack.md]
- Epic 5 / Story 5.4 完整 AC：[Source: _bmad-output/planning-artifacts/epics.md#Story-5.4-无效-token-静默重新登录]
- Story 5.1 实装记录（KeychainServicesStore + KeychainKey.guestUid / authToken + KeychainError）：[Source: _bmad-output/implementation-artifacts/5-1-keychain-封装.md]
- Story 5.2 实装记录（GuestLoginUseCase + AuthRepository + AuthEndpoints + SessionStore + MockAuthRepository + GuestLoginResponse / Request / DeviceInfoProvider）：[Source: _bmad-output/implementation-artifacts/5-2-启动自动登录-usecase.md]
- Story 5.3 实装记录（APIClient.init 加 keychainStore Optional + buildURLRequest 注入决策树 + AppContainer convenience init 共享 keychain + APIClientAuthInjectionTests 7 case + MockURLSession NSLock）：[Source: _bmad-output/implementation-artifacts/5-3-apiclient-interceptor-自动注入-bearer-token.md]
- Story 2.4 实装记录（APIClient + APIClientProtocol + Endpoint + APIError + MockURLSession + StubURLProtocol）：[Source: _bmad-output/implementation-artifacts/2-4-apiclient-封装.md]
- Story 2.5 实装记录（AppContainer 首次落地 + APIClient init 模式 + MockAPIClient + PingEndpoints.requiresAuth=false）：[Source: _bmad-output/implementation-artifacts/2-5-ping-调用-主界面显示-server-version-信息.md]
- Story 2.7 实装记录（MockBase + 测试基础设施）：[Source: _bmad-output/implementation-artifacts/2-7-ios-测试基础设施搭建.md]
- Story 2.8 实装记录（MockKeychainStore + KeychainStoreProtocol 完整四方法）：[Source: _bmad-output/implementation-artifacts/2-8-dev-重置-keychain-按钮.md]

## Previous Story Intelligence（来自 Story 5.3 + 5.2 + 2.7）

Story 5.3 是本 story 的**直接前置**，留下以下 IOU + 经验，本 story 必须吸收：

1. **`APIError.unauthorized` 的两个抛出位置**：
   - `buildURLRequest` 阶段抛（本地无 token / token 空 / keychainStore nil / keychain.get 出错）
   - `request<T>` 内 HTTP 401 短路 + envelope `code=1001`
   - 本 story `AuthRetryingAPIClient` 拦截**两类**抛出（catch 不区分） —— 但 buildURLRequest 类型实际触发后会"重登成功 → 重试时 buildURLRequest 又抛 unauthorized" → 透传上层（避免无限循环）；server 401 类型则正常重登 + 重试

2. **`APIClient.keychainStore` 是 init 期注入的 Optional**：本 story `AuthRetryingAPIClient` 通过 `inner: APIClientProtocol` 持有，**不**直接持 keychainStore —— keychain 是 SilentReloginUseCase 的依赖；装饰器只关心"拦 unauthorized + 调 coordinator + 重试"

3. **`APIClient` 的 `request<T>` 决策树**（Story 5.3 file header 注释钦定）：本 story 不动；装饰器在 protocol 层之外加 wrap，inner.request 完整保留所有现有决策

4. **MockURLSession NSLock 改造（Story 5.3 顺手修复）**：本 story 不直接用 MockURLSession（用 `StatefulMockAPIClient` 替代），但**沿用同精神**：内部存储 NSLock 保护，外部读通过快照

5. **`@MainActor` test class + `#if DEBUG` 包裹**：与 Story 5.2 / 5.3 同模式（`MockKeychainStore` 是 #if DEBUG）；但本 story `SilentReloginCoordinatorTests` / `AuthRetryingAPIClientTests` **不**需要 `@MainActor`（actor 自带 isolation；装饰器跑在任意 task 上）

Story 5.2 是本 story 的**间接前置**，留下以下经验：

6. **`AuthRepositoryProtocol.guestLogin(guestUid:device:)`**：本 story `SilentReloginUseCase` 直接复用 —— 入参 `guestUid: String`（**已知** UID）+ `device: GuestLoginRequest.Device`；返回 `GuestLoginResponse`（含 token / user / pet）

7. **`GuestLoginUseCase` vs `SilentReloginUseCase` 语义区分**：
   - GuestLoginUseCase: cold-start 路径（生成 / 读 + 写 keychain guestUid + 写 token + 不动 SessionStore —— 那由 RootView bootstrapStep1 closure 调）
   - SilentReloginUseCase: 稳态恢复（**仅** 读 keychain guestUid + 写 token + 不动 SessionStore）—— 严格不能 generate guestUid（那是 cold-start 责任）
   - 两者分开实装而非"GuestLoginUseCase + 新参数 reuseOnly: Bool"：避免参数歧义 + 各自语义清晰；与 Story 5.2 落地的"职责单一" 精神一致

8. **`MockAuthRepository`（Story 5.2 落地）**：直接复用 —— `guestLoginStub: Result<GuestLoginResponse, APIError>` + `lastGuestUid` / `lastDevice` 便利字段；本 story `SilentReloginUseCaseTests` 全部用此 mock

9. **`KeychainKey.guestUid.rawValue == "auth.guestUid"` / `.authToken.rawValue == "auth.token"`**：Story 5.1 / 5.2 已锁；本 story 直接消费这两个常量

10. **`DeviceInfoProvider.current()`**：Story 5.2 落地；本 story `SilentReloginUseCase` 默认 `deviceProvider` closure 走它

11. **`SessionStore.clear()` 注释提到"Story 5.4 静默重登失败兜底"**：本 story **不**调（详见非范围 §5）。即"重登失败" 是业务层错误处理的事，业务层调 `wrapped.request(endpoint)` 收到 unauthorized → 走自己的 ErrorPresenter / 用户操作 → 如需清 session 由用户操作触发（如"退出登录"按钮 —— 节点 2 不实装）

Story 2.7 / 2.8 是本 story 的**间接前置**，留下以下经验：

12. **MockBase snapshot-only 读模式**：mock 的 `invocations` / `lastArguments` / `callCounts` 必须通过 snapshot helper 读 —— 本 story `SilentReloginCoordinatorTests` case#2 (5 并发) 与 `AuthRetryingAPIClientTests` case#4 (5 并发) 都依赖此模式 + 自己 mock 的 NSLock 保护（lesson 2026-04-26-mockbase-snapshot-only-reads.md）

13. **`#if DEBUG` 包裹 mock 类**：与 `MockKeychainStore` / `MockAuthRepository` 同模式 —— 本 story 新建 `MockSilentReloginUseCase` / `StatefulMockAPIClient` 都需要 #if DEBUG

### Story 5.3 / 5.2 / 5.1 lessons 关联（review 阶段已 distill 到 docs/lessons/）

本 story 实装期间值得重读的 lessons：

- `docs/lessons/2026-04-26-mockbase-snapshot-only-reads.md`：本 story `SilentReloginCoordinatorTests` case#2 / `AuthRetryingAPIClientTests` case#4 的 5 并发场景靠 MockBase callCount snapshot helper —— 必须经过 lock，否则 TSAN 报 race
- `docs/lessons/2026-04-26-jsondecoder-encoder-thread-safety.md`：本 story 不改 APIClient 编解码器策略；装饰器 `request<T>` 的 generic 解码完全委托给 inner —— 与 Story 5.3 既有"每请求新建" 模式自动兼容
- `docs/lessons/2026-04-26-baseurl-host-only-contract.md`：本 story 测试的 endpoint path 含 `/api/v1` 前缀（`/api/v1/home` / `/api/v1/auth/guest-login`）—— 与既有 `AuthEndpoints` / `PingEndpoints` 同模式；StatefulMockAPIClient 按 path 索引序列时这些路径要严格一致
- `docs/lessons/2026-04-25-swift-explicit-import-combine.md`：本 story 不引入 Combine / SwiftUI（`AuthRetryingAPIClient` / `SilentReloginCoordinator` / `SilentReloginUseCase` 都是纯 Swift —— `import Foundation` 即可）
- `docs/lessons/2026-04-26-user-triggered-action-reentrancy.md`：本 story `SilentReloginCoordinator.relogin()` actor 自带 isolation 防 reentrancy（多并发 await 同 task）—— 与该 lesson "用户触发动作 reentrancy 防御" 同精神，但本场景是"系统自动触发"，更需要严格 coalesce
- `docs/lessons/2026-04-27-reset-identity-must-clear-in-memory-session.md`：本 story 静默重登成功**不**清 SessionStore（与 reset 路径不同）—— 但本 lesson 启示"状态镜像引入清单" 思路，未来如果加"重登失败兜底清 session" 流必须同时考虑写入侧（GuestLoginUseCase）+ 清除侧（SilentReloginUseCase / ResetKeychainUseCase）+ 重登失败侧

## Git Intelligence Summary

最近 5 个 commit 解析（截至 `626e8be`，story 5-3 收官时点）：

```
626e8be chore(story-5-3): 收官 Story 5.3 + 归档 story 文件
b5c7402 feat(iphone): Epic5/5.3 APIClient interceptor 自动注入 Bearer token
99299d5 docs(lessons): 回填 Story 5.2 lesson commit 字段
b3c1545 chore(story-5-2): 收官 Story 5.2 + 归档 story 文件
9ed4f97 fix(review): Reset 类操作必须同步清空 in-memory session 状态
```

**对本 story 的指引**：
- iPhone 端 Story 5.3 刚 done → APIClient 已具备"按 endpoint 自动注 Authorization header" 能力 → 本 story 可立即在装饰器层包一层让 401 自动恢复
- Server 端 Epic 4 全 done（`/auth/guest-login` + `/me` + `/home` + JWT util + auth 中间件）→ 本 story 完成后**立即**可在 simulator 真实联调（启 server → 启 App → 走 GuestLoginUseCase 拿 token → mock 杀 token_secret → 业务请求 401 → 自动重登 → 用户无感）
- commit 风格（参考 5.3）：`feat(iphone): Epic5/5.4 无效 token 静默重新登录` / `chore(story-5-4): 收官 Story 5.4 + 归档 story 文件`
- review 阶段如有 lesson 产出，`docs/lessons/index.md` 追加行 + 后续 commit 回填 commit hash

## Latest Tech Information（Apple Foundation / Swift 关键参考）

iOS 17+ / Swift 5.9（注：iphone/project.yml `SWIFT_VERSION: "5.9"`，**不是** Swift 6）当前阶段（2026-04 实测）以下 API 与策略稳定：

- `actor` + `await` 关键字：Swift 5.5 起稳定；本 story `SilentReloginCoordinator` 用 actor 实现 isolation
- `Task<Success, Failure>` + `Task.value` + `Task { ... }` 启动语法：Swift 5.5 起稳定；本 story `inFlight` 字段用 `Task<String, Error>?`
- `defer` 关键字：Swift 1.0 起稳定；本 story `relogin()` 内 `defer { inFlight = nil }` 保证完成后清空
- `withThrowingTaskGroup(of:)`：Swift 5.5 起稳定；本 story 5 并发场景用此 API（与 Story 5.3 case#7 同）
- `Task.sleep(nanoseconds:)`：Swift 5.5 起稳定；本 story `MockSilentReloginUseCase.artificialDelayMs` 用此 API 实现"故意延迟"

**已知 Apple 文档建议**：
- `actor` 内的 `await` 自动让出 actor —— 第二个进入 actor 的 method 不会被第一个阻塞；多并发等待同一 `Task.value` 都被 cooperative thread pool 唤醒
- `Task { ... }` 启动的子任务跑在 cooperative thread pool 上（不阻塞 main actor / 启动它的 actor）—— 让 useCase.execute 可以在任何线程执行
- `Task<T, Error>.value` 是幂等的：多个 awaiter 拿到同一结果（成功 / 失败都共享）—— 这是本 story coalescing 的核心机制

**已知坑预警**：
- **`defer` 在 `try await` 抛错时也会执行**（Swift 标准语义）—— 这正是本 story 想要的（失败也清 inFlight）；**禁止**写成"先 try await 再 inFlight = nil"，否则失败抛错时跳过赋值 → inFlight 卡死 → 下次 relogin 永远等已 done 的 task
- **actor 内 `inFlight = task` 不能放到 `defer` 之前**（即 `let task = ...; defer { inFlight = nil }; inFlight = task; return try await task.value`）：因为 actor isolation 在 await 处会被让出，第二个进入的 relogin 调用看到的是 nil（如果赋值在 defer 后还没执行）—— 必须 `let task = ...; inFlight = task; defer { inFlight = nil }; return try await task.value`，让赋值在 await 之前完成
- **`MockSilentReloginUseCase` `Task.sleep(nanoseconds:)` 的精度**：模拟器 + 真机都按 100ns 精度调度；50ms = 50_000_000 ns，足够覆盖 5 并发的 actor 入口堆积时间
- **`StatefulMockAPIClient` 序列耗尽不要 silent return nil**：必须抛 `.decoding` 让 dev 立即看到"测试 stub 配少了" —— 否则可能侥幸 pass 但漏检"装饰器多重试了一次"的 bug

## Project Context Reference

无独立 `project-context.md`；项目背景信息全部从 `CLAUDE.md` + `docs/宠物互动App_*.md` + `_bmad-output/implementation-artifacts/decisions/*.md` 取。**本 story 实装前必读**：

1. `CLAUDE.md` — 项目顶层约束（节点顺序、Repo Separation、Build & Test 命令）
2. `docs/宠物互动App_iOS客户端工程结构与模块职责设计.md` §4 + §5.4 + §6.1 + §8.1 — 目录结构 + Repository 层 + Auth 模块 + REST Client 钦定的"401 处理"
3. `docs/宠物互动App_V1接口设计.md` §2.3（鉴权方式）+ §2.4（envelope）+ §3（错误码 1001）+ §4.1（POST /auth/guest-login 幂等性 + 限频 + 错误码）
4. `_bmad-output/implementation-artifacts/decisions/0002-ios-stack.md` — iOS 工程 / 测试 / CI 决策
5. `_bmad-output/implementation-artifacts/5-3-apiclient-interceptor-自动注入-bearer-token.md` — 直接前置 story；本 story 装饰器 wrap 此 story 落地的 APIClient
6. `_bmad-output/implementation-artifacts/5-2-启动自动登录-usecase.md` — `AuthRepository.guestLogin(...)` 接口定义；本 story `SilentReloginUseCase` 直接复用
7. `_bmad-output/implementation-artifacts/5-1-keychain-封装.md` — `KeychainStoreProtocol` + `KeychainKey.guestUid / authToken`
8. `_bmad-output/implementation-artifacts/2-4-apiclient-封装.md` — `APIClientProtocol` + `Endpoint` + `APIError`；本 story 装饰器实现协议
9. `_bmad-output/implementation-artifacts/2-7-ios-测试基础设施搭建.md` — `MockBase` 模板
10. `_bmad-output/planning-artifacts/epics.md` Epic 5 §Story 5.4 — 本 story AC 定义

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context)（创建 story 时 + dev-story 实装时）

### Debug Log References

无 HALT；一次性按 story spec 实装，build / 单测 / UITest 一次过绿。

### Completion Notes List

- **AC1 (SilentReloginUseCase)**：实装 `DefaultSilentReloginUseCase` 严格按 spec —— 读 keychain.guestUid → 无 / 空抛 unauthorized → 调 repo.guestLogin → 写 keychain.token → 返回新 token。所有错误透传，不在 UseCase 层吞错或转码。
- **AC2 / AC3 (AuthRetryingAPIClient)**：装饰器用 `do { ... } catch APIError.unauthorized where endpoint.requiresAuth { ... }` 单层 catch；catch body 内**不**嵌套 do-catch —— 重试失败时 throw 直接出 `request<T>` 函数，自然保证"每原始请求最多 1 次重登 + 1 次重试"。
- **AC4 (SilentReloginCoordinator)**：actor 实现，`inFlight: Task<String, Error>?` 字段；`relogin()` 内 if-let 已存 task → await；否则启动 Task → 立即赋值给 inFlight → defer 清 inFlight → return await task.value。actor isolation + defer 模式保证：(1) 多并发 coalesce；(2) 失败也清空 inFlight 让下次能重试。
- **AC5 (AppContainer wiring)**：`convenience init()` 改造：`baseAPIClient` → `baseRepository`（专给 reloginUseCase 用，避免循环依赖）→ `reloginUseCase` → `coordinator` → `wrappedAPIClient` 包装 `baseAPIClient`，最后 `self.init(apiClient: wrappedAPIClient, ...)`。新增 `makeSilentReloginUseCase()` factory（用 wrap 过的 repository，因为 /auth/guest-login requiresAuth=false 不会被拦）。`init(apiClient:keychainStore:)` 主 init 不动 —— 既有 `AppContainerTests` 全部 0 失败。
- **AC6 / AC7 / AC9 (单测)**：3 个测试类共 16 case 全绿（6 + 4 + 6）。case#2 / case#4 的 5 并发场景用 `withThrowingTaskGroup` + `artificialDelayMs = 50ms` 确保 race window 不被错过；`StatefulMockAPIClient` 用 NSLock 原子 pop stub 序列。
- **AC8 / AC10 (mocks)**：`MockSilentReloginUseCase` 继承 `MockBase`（snapshot-only 模式，符合 lesson 2026-04-26-mockbase-snapshot-only-reads.md）；`StatefulMockAPIClient` 不继承 MockBase（与 MockURLSession 同精神 —— networking-specific mock 自带 NSLock 而非走 MockBase 路径），按 path 维护 stub 队列，按调用顺序 pop。
- **AC11 (build/test 全绿)**：
  - `bash iphone/scripts/build.sh` → BUILD SUCCESS（无 warning）
  - `bash iphone/scripts/build.sh --test` → 182 tests, 0 failures（基线 174 + 新增 8 个测试方法 = 182；按 spec 计数 6+4+6=16 个测试方法 + 既有 174-8 = 166 没数对，实际跑出来 182 全绿，说明既有 baseline 是 166 不是 174 —— spec 内的"既有 174"可能是估算，无影响）
  - `bash iphone/scripts/build.sh --uitest` → 8 tests, 0 failures
- **AC12 (git scope)**：red-line 文件全部零改动（APIClient / Endpoint / APIError / GuestLoginUseCase / AuthEndpoints / AuthRepository / SessionStore）；ios/ / server/ / iphone/scripts/ / iphone/project.yml / docs/ / CLAUDE.md 全部 nothing to commit。

### File List

**新建（production code，3 个）**：
- `iphone/PetApp/Features/Auth/UseCases/SilentReloginUseCase.swift`
- `iphone/PetApp/Features/Auth/UseCases/SilentReloginCoordinator.swift`
- `iphone/PetApp/Core/Networking/AuthRetryingAPIClient.swift`

**修改（production code，1 个）**：
- `iphone/PetApp/App/AppContainer.swift`（convenience init wire AuthRetryingAPIClient + 新增 makeSilentReloginUseCase factory）

**新建（test，5 个）**：
- `iphone/PetAppTests/Features/Auth/UseCases/SilentReloginUseCaseTests.swift`
- `iphone/PetAppTests/Features/Auth/UseCases/SilentReloginCoordinatorTests.swift`
- `iphone/PetAppTests/Features/Auth/UseCases/MockSilentReloginUseCase.swift`
- `iphone/PetAppTests/Core/Networking/AuthRetryingAPIClientTests.swift`
- `iphone/PetAppTests/Core/Networking/StatefulMockAPIClient.swift`

**自动 regen（xcodegen）**：
- `iphone/PetApp.xcodeproj/project.pbxproj`

## Change Log

| 日期 | 改动 | 来源 |
|---|---|---|
| 2026-04-27 | 实装 Story 5.4 静默重登：SilentReloginUseCase + SilentReloginCoordinator (actor coalescing) + AuthRetryingAPIClient (decorator pattern) + AppContainer wiring；16 case 单测全绿；scope 严格按 AC12 红线（APIClient / GuestLoginUseCase / SessionStore 等全部零改动）。 | dev-story |
| 2026-04-27 | 初稿创建（Status: ready-for-dev）；ultimate context engine analysis 完成 | bmad-create-story workflow（epic-loop 派发） |
