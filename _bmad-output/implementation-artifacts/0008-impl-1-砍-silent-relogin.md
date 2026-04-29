# Story 0008-impl-1: 砍掉 Silent Relogin 改走 Cold-Start 重建（ADR-0008 v2 D3 实施）

Status: done

## Story

As an iPhone 用户,
I want 当 server 返回 401 / envelope `code=1001` 时 App 自动清空 in-memory session、回到 LaunchingView、用 keychain 中已有 `guestUid` 重跑 cold-start `/auth/guest-login` + `/home`，把我送回主屏 —— 整个过程 < 1 秒，没有错误弹窗、没有手动操作；如果 cold-start 本身失败才走 RetryView / TerminalErrorView,
so that **MVP 阶段**鉴权失效场景以最小代码复杂度恢复（替代当前 `AuthRetryingAPIClient` + `SilentReloginCoordinator` + `SilentReloginUseCase` 的 800+ 行实现），同时把 4 条 silent relogin 相关 lesson 标 superseded 推动 lesson 蒸馏。

## 故事定位（ADR-0008 v2 §6 D3 直接实施）

ADR-0008 v2 把 silent relogin 退役改走"401 全局 catch + cold-start 重建"。本 story 是该决策的代码层退役动作。**前置依赖**：

- **ADR-0008 v2 已 Accepted**（含 §13.1 的"401 全局 catch 位置"已拍板：A/B/C 三选一）
- **Story 5.4 (`done`)**：当前 `AuthRetryingAPIClient` / `SilentReloginCoordinator` / `SilentReloginUseCase` 已实装，本 story 退役它们
- **Story 5.5 (`done`)**：当前 `AppErrorMapper` / `AppLaunchStateMachine` / `RootView` bootstrap 路径已稳定（11 轮 fix-review 后），本 story 不动 bootstrap state machine 内部逻辑
- **Story 5.2 (`done`)**：`SessionStore` + `clear()` 方法已就位 —— 本 story 在 401 全局 catch 内调 `sessionStore.clear()`

## 核心动作（按 §13.1 拍板结果决定具体落点）

### 1. 新增 401 全局 catch（位置由 ADR §13.1 拍板）

**A 已拍板**（2026-04-29，ADR §13.1 Resolved）。

**新建** `iphone/PetApp/Core/Networking/AuthBoundaryAPIClient.swift`（替代退役的 `AuthRetryingAPIClient`）：

```swift
final class AuthBoundaryAPIClient: APIClientProtocol {
    private let inner: APIClientProtocol
    private let sessionStore: SessionStore
    private let stateMachine: AppLaunchStateMachine

    init(inner: APIClientProtocol, sessionStore: SessionStore, stateMachine: AppLaunchStateMachine) {
        self.inner = inner
        self.sessionStore = sessionStore
        self.stateMachine = stateMachine
    }

    public func request<T: Decodable>(_ endpoint: Endpoint) async throws -> T {
        do {
            return try await inner.request(endpoint)
        } catch APIError.unauthorized where endpoint.requiresAuth {
            await sessionStore.clear()            // 立即让订阅 SessionStore 的 view 切到 fallback 态
            await stateMachine.triggerColdStart() // 触发 bootstrap 重跑（cold-start GuestLogin + LoadHome）
            throw APIError.unauthorized            // 仍 throw 让 caller 知道本次请求失败
        }
    }
}
```

**关键约束**：
- 只对 `endpoint.requiresAuth == true` 的请求触发 cold-start —— 避免 `/auth/guest-login` 自身 401 自救悖论（`AuthEndpoints.guestLogin(...).requiresAuth == false` 由 Story 5.2 锁定）
- 装饰器**不**做 in-flight 协调（**不**复刻退役的 generation snapshot / inFlight task / lastIssuedToken cache 三件套）—— 多请求并发触发由下方 §1.2 `AppLaunchStateMachine.triggerColdStart()` 内部 idempotent flag 保护
- 装饰器持 `sessionStore` + `stateMachine` 强引用 —— 由 `AppContainer` 装配时注入（详见 §3）

### 1.2 `AppLaunchStateMachine` 新增 `triggerColdStart()` 入口

```swift
extension AppLaunchStateMachine {
    /// 由 AuthBoundaryAPIClient 在 401 时调用 —— 与 retry() 几乎相同，区分语义：
    /// - retry(): user 在 .needsAuth 状态下点 RetryView 重试按钮
    /// - triggerColdStart(): network 层检测到 token 失效自动触发
    public func triggerColdStart() async {
        guard !isRetrying else { return }  // 复用 isRetrying flag 防多请求并发触发（与 retry() 同模式）
        isRetrying = true
        defer { isRetrying = false }

        state = .launching
        hasBootstrapped = false
        await bootstrap()
    }
}
```

**注意**：`triggerColdStart()` 与 `retry()` 实现几乎相同，**不要在本 story 范围内合并**为 private helper —— 两者语义不同（user-initiated vs system-initiated），强行合并属于 ADR §13.3 "RootView/AppLaunchStateMachine 知道太多" 重构范畴，留给后续 epic-cleanup。

### 2. 删除文件 + 配套测试

**删除源文件**（~304 行）：
- `iphone/PetApp/Core/Networking/AuthRetryingAPIClient.swift`（92 行）
- `iphone/PetApp/Features/Auth/UseCases/SilentReloginCoordinator.swift`（138 行）
- `iphone/PetApp/Features/Auth/UseCases/SilentReloginUseCase.swift`（74 行）

**删除测试**（估 ~500 行）：
- `iphone/PetAppTests/Features/Auth/UseCases/SilentReloginCoordinatorTests.swift`
- `iphone/PetAppTests/Core/Networking/AuthRetryingAPIClientTests.swift`
- `iphone/PetAppTests/Features/Auth/UseCases/SilentReloginUseCaseTests.swift`
- `iphone/PetAppTests/Features/Auth/UseCases/MockSilentReloginUseCase.swift`
- 视情况调整 `APIClientAuthInjectionTests` / `AppContainerTests` 中涉及 `AuthRetryingAPIClient` 的 case

### 3. 简化 `AppContainer` 装配

- `AppContainer.swift` 当前 `apiClient: APIClientProtocol` 走 `AuthRetryingAPIClient(inner: APIClient(...), reloginCoordinator: SilentReloginCoordinator(useCase: SilentReloginUseCase(...)))`
- 改装为：
  ```swift
  let bareClient = APIClient(baseURL: ..., session: ..., keychainStore: keychainStore)
  self.apiClient = AuthBoundaryAPIClient(
      inner: bareClient,
      sessionStore: sessionStore,
      stateMachine: launchStateMachine
  )
  ```
- `AppContainer` 现在已经持有 `sessionStore` + `launchStateMachine`，直接传引用即可，无新增字段

### 4. 简化 `AppErrorMapper` 注释

- 移除 `.unauthorized` case 的 round 5 / 9 / 11 迭代史注释（mapper 不再决定 `.unauthorized` 的 presentation —— 由 §6.2 全局 catch 接管）
- 移除 silent relogin 相关跨组件耦合注释（`AuthRetryingAPIClient` / `SilentReloginCoordinator` / generation snapshot 等提及）
- 预期 mapper 从 286 行 → ~200 行
- 保留 `.localStoreFailure` / `.missingCredentials` 区分（ADR §6.5 钦定保留）

### 5. 更新 `APIClient.buildURLRequest`

- 当前 `buildURLRequest` 在三种本地态分别抛 `.missingCredentials` / `.localStoreFailure` —— **保留不变**
- 移除内联注释中"`AuthRetryingAPIClient` 不接管这两个 case"的说明（decorator 已退役）

## Acceptance Criteria

### AC1（功能）：401 触发 cold-start 重建

- Given app 在 `.ready` 状态发任意业务请求（如 `GET /home`）
- When server 返回 401 / envelope `code=1001`
- Then app 自动：(a) 清空 `SessionStore.session`；(b) state machine 回 `.launching`；(c) 重跑 cold-start `GuestLoginUseCase` + `LoadHome`；(d) 成功后回主屏
- And 整个过程用户感知 = 主屏闪一下 LaunchingView（< 1 秒）
- And keychain `guestUid` 不丢失（cold-start 复用同一身份）
- And 用户的步数 / 宠物 / 装扮 / 房间数据全保留

### AC2（功能）：cold-start 失败走 RetryView / TerminalErrorView

- Given 401 触发 cold-start
- When cold-start `GuestLoginUseCase` 或 `LoadHome` 也失败
- Then 走 ADR-0008 §5.2 既有三态分发（`.retry` → RetryView / `.alert` → TerminalErrorView）
- And 反复点 retry → 反复 cold-start，无死循环（`isRetrying` flag 已实装于 Story 2.9）

### AC3（代码退役）：删除 silent relogin 实现

- Given 退役完成
- Then `iphone/PetApp/` 树下不再存在 `AuthRetryingAPIClient.swift` / `SilentReloginCoordinator.swift` / `SilentReloginUseCase.swift`
- And 配套测试文件全部删除
- And `bash iphone/scripts/build.sh --test` 全绿

### AC4（测试矩阵）：新增 cold-start 测试

- 新建 `AuthBoundaryAPIClientTests`（候选 A）/ `AppLaunchStateMachineTests` 扩展（候选 B）：
  - case 1：业务请求 401 → 触发 cold-start → state 转 `.launching` → cold-start 成功 → 回 `.ready`
  - case 2：业务请求 401 → 触发 cold-start → cold-start 失败 → state 转 `.needsAuth(.alert)`
  - case 3：业务请求 401 → 触发 cold-start → 期间用户切回前台再次触发请求 → 不并发 cold-start（防重入 flag）
  - case 4：`requiresAuth: false` 端点（如 `/auth/guest-login` 自身）401 → 不触发 cold-start，直接 throw

### AC5（lesson 标注）：4 条 silent relogin lesson 标 superseded

- 已在 `docs/lessons/index.md` 完成（本 ADR commit 内执行）
- lesson 文件本身**不删除**（保留物理文件 + git history）

### AC6（mapper 简化）：`AppErrorMapper.swift` 行数下降

- v2 实施前 286 行 → v2 实施后 ≤ 220 行（砍 ~80 行 silent relogin 相关迭代史 + decorator 跨组件耦合注释）

## 非范围（必须显式列出）

- **不**删除 `APIError.missingCredentials` / `.localStoreFailure` case（ADR §6.5 钦定保留）
- **不**改动 `AppLaunchStateMachine.bootstrap()` / `retry()` 内部实现（仍是 Story 2.9 + 5.5 收敛后的形态）
- **不**改动 `TerminalErrorView` 编译期契约
- **不**改动 server 端任何代码（401 仍是 server 行为，server 不感知 client 切换 silent relogin → cold-start）
- **不**重构 `RootView.swift` 的"知道太多错误史"问题（ADR §13.3 Open Question 留给后续 epic-cleanup）
- **不**预设 post-MVP 的 OAuth refresh token 设计（ADR §12 不在范围）
- **不**做 `AppErrorMapper.transientBusinessCodes` 协议层化（v2 已钦定保留为 client 端硬编码 §4.3）

## 风险 / 注意

- **风险 1：cold-start 触发频率被低估**。MVP 估算 401 < 1%；若实际生产 server 行为导致频繁 401（如 token TTL 设置过短），用户感知"App 反复闪 LaunchingView" → 需要回头调 server token TTL 而非重新引入 silent relogin
- **风险 2：cold-start 期间 UI 卡顿**。candidate A 路径下，cold-start 期间业务 view 会保留旧数据短暂可见 → 可考虑 candidate A + 立即清 SessionStore 让 UI 立即过渡到 LaunchingView
- **注意**：本 story 退役决策的"重新引入路径清晰"假设依赖于 git history 完整保留 —— `git log --follow` 必须仍能查到删除前的 SilentReloginCoordinator 实现，作为未来 OAuth refresh token spike 的参考

## 实施顺序建议

1. ADR §13.1 拍板（A/B/C 三选一）→ 决定 §1 落点
2. 实装 §1 新 catch 路径 + AC4 测试（**先加新功能验证**）
3. 删除 §2 三个文件 + 配套测试（**再砍旧实现**）
4. §3 简化 AppContainer
5. §4 简化 AppErrorMapper
6. `bash iphone/scripts/build.sh --test` 全绿验收
7. 更新 ADR-0008 Status: Draft v2 → Accepted（如剩余 Open Questions 也已拍板）

## Change Log

| Date | Change | By |
|---|---|---|
| 2026-04-29 | 初稿（draft）：ADR-0008 v2 §6 D3 实施 story | Claude（用户委托） |
| 2026-04-29 | 实施 + Mac 验证完成：commit 2154daf 主体退役 / 565411b 测试断言修复；`bash iphone/scripts/build.sh --test` 236/236 全绿；Status → done | Claude（Mac 端用户验证） |
