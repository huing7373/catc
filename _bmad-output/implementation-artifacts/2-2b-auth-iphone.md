# Story 2.2b: Sign in with Apple 认证——iPhone 客户端

Status: ready-for-dev

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a iPhone 用户,
I want 打开 CatPhone 时用 Sign in with Apple 一键登录, App 自动保存 token 并在后续访问中透明地刷新 / 重新登录,
so that 我不用记密码，token 安全存在 Keychain，换机或签退后回来也能无缝恢复。

> **拆分背景**：原 Story 2-2 是 full-stack，按 backend / iPhone / Watch 三拆。本故事是**第二拆：iPhone**；不包含任何后端代码（契约由 **2-2a** 定义），也不包含 Watch 侧接收逻辑（由 **2-2c** 承接）。
>
> **基线与依赖**：
> - **硬依赖 2-2a**：`/v1/auth/login` 和 `/v1/auth/refresh` 必须可用；契约见 2-2a DTO 骨架。
> - iPhone 侧现状：`ios/CatShared/Sources/CatShared/Networking/` 是**空目录**，`CatPhoneApp.body` 直接进 `SpineboyPreviewView`。本故事首次建立 API 客户端层 + Keychain + 登录门槛。
> - 本故事交付的 `CatShared.APIClient` / `TokenStore` / `KeychainTokenStore` / `AuthAPI` / `TokenPair` 会被 **2-2c（Watch）** 直接复用，所以它们必须写在 **CatShared** 包（跨 iOS/watchOS 可用），**不能** 写死 `#if os(iOS)`。

## Acceptance Criteria

1. **Given** 新文件 `ios/CatShared/Sources/CatShared/Networking/APIClient.swift` **When** `CatPhone` / `CatWatch` 发起网络请求 **Then** `APIClient` 是 `actor`（Swift Concurrency），封装 `URLSession` + `JSONEncoder/Decoder`：
   - `keyEncodingStrategy = .useDefaultKeys`（Swift 属性名保持 `snake_case` 与后端一致，**不**依赖 `.convertFromSnakeCase` 避免双向转换坑）；
   - `dateDecodingStrategy = .iso8601`；
   - `init(baseURL: URL, tokenStore: any TokenStore)`；
   - 方法 `send<Req: Encodable, Resp: Decodable>(path: String, method: HTTPMethod, body: Req?, authenticated: Bool) async throws -> Resp`；
   - `authenticated=true` 时从 `tokenStore.currentAccessToken()` 注入 `Authorization: Bearer ...`。

2. **Given** `APIClient` 收到 **401 + `code=AUTH_EXPIRED`** **When** `authenticated=true` **Then** 执行**一次** refresh 流程：调 `POST /v1/auth/refresh`（不带 Authorization 头）→ 成功则持久化新 token 对 + 重放原请求；refresh 返回 `AUTH_INVALID` / `UNAUTHORIZED` / `APPLE_AUTH_FAIL` 或网络错 → 清空 Keychain + 抛 `APIError.reauthRequired`；**And** 并发 refresh 去重：actor 内持有 `refreshTask: Task<TokenPair, Error>?`，多请求同时命中 401 时只发一次 refresh，其它 `await` 同一 task；**And** 非 `AUTH_EXPIRED` 的 401（如 `VALIDATION_ERROR`、`RATE_LIMITED` 429）**不** 触发 refresh。

3. **Given** `APIError` 类型 **When** 调用方捕获 **Then** 包含分支：`network(Error) / decode(Error) / server(code: String, message: String, httpStatus: Int) / reauthRequired`；**And** `server(code:)` 里的 `code` 原封对应后端 `error.code`，UI 可以据此做枚举匹配。

4. **Given** 新文件 `ios/CatShared/Sources/CatShared/Networking/AuthAPI.swift` + `TokenPair.swift` **When** 登录流程调用 **Then** 提供：
   - `TokenPair: Codable, Sendable`（属性名 snake_case：`user_id / access_token / refresh_token / access_expires_at: Date / refresh_expires_at: Date / login_outcome: String?`，`?` 是因 refresh 响应没 outcome）；
   - `LoginRequest: Encodable, Sendable { let apple_jwt: String; let nonce: String; let device_id: String }`；
   - `RefreshRequest: Encodable, Sendable { let refresh_token: String }`；
   - `extension APIClient { func login(_: LoginRequest) async throws -> TokenPair; func refresh(_: RefreshRequest) async throws -> TokenPair }`，两者都走 `authenticated: false`。

5. **Given** 新文件 `ios/CatShared/Sources/CatShared/Persistence/KeychainStore.swift` **When** 需要持久化 token **Then** 提供：
   - `protocol TokenStore: Sendable { func saveTokens(_: TokenPair) async throws; func loadTokens() async throws -> TokenPair?; func clear() async throws; func currentAccessToken() async -> String? }`；
   - `final actor KeychainTokenStore: TokenStore`，用 `Security.framework` 的 `SecItemAdd / SecItemCopyMatching / SecItemUpdate / SecItemDelete`；单条目 service `"com.zhuming.cat.auth"` + account `"tokens"`，value = `JSONEncoder().encode(TokenPair)`；
   - `kSecAttrAccessible = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`（不上 iCloud）；
   - 错误 `KeychainError.osStatus(OSStatus) / .decode(Error)`；
   **And** 第二次 `saveTokens` 走 `SecItemUpdate` 而非 `SecItemAdd`（单元测试覆盖）。

6. **Given** `KeychainStoreTests` **When** 运行（iOS Simulator） **Then** 覆盖：save → load round-trip / clear 后 load 返回 nil / update（第二次 save）/ load 空返回 nil 且不抛错。

7. **Given** `APIClientTests` **When** 运行 **Then** 用 `URLProtocol` stub 响应覆盖：
   - 200 成功反序列化；
   - `authenticated=false` 时不注入 Authorization；
   - `authenticated=true` 时注入 Bearer；
   - 401 `AUTH_EXPIRED` → refresh 成功 → 重放 → 成功；
   - 401 `AUTH_EXPIRED` → refresh 失败（`AUTH_INVALID`）→ 抛 `reauthRequired` + Keychain 被清；
   - 两请求并发 401 → 只发一次 refresh（stub 计数断言）；
   - 429 `RATE_LIMITED` 直接抛 `server(...)` 不触发 refresh。

8. **Given** 新文件 `ios/CatPhone/App/Auth/SignInViewModel.swift` **When** `CatPhoneApp` 启动 **Then**：
   - `@Observable` 类持有 `apiClient: APIClient`、`tokenStore: TokenStore`、`bridge: WatchTokenBridging`（protocol，**2-2c** 会提供 `WatchConnectivityBridge` 的真实实现；本故事用 `NoopWatchTokenBridge` 占位不阻塞开发）；
   - `state: SignInState { .checking, .signedOut, .signingIn, .signedIn(userID: String), .error(APIError) }`；
   - `bootstrap()`：`tokenStore.loadTokens()` → 非空且 `access_expires_at > now` → `.signedIn`；非空但 access 过期 → 调 `apiClient.refresh(...)`，成功 → 保存 + bridge 推送 + `.signedIn`；失败 → 清 Keychain + `.signedOut`；空 → `.signedOut`；
   - `handleAppleSignIn(_: ASAuthorization)`：从 `ASAuthorizationAppleIDCredential` 取 `identityToken` (Data → UTF-8)；调 `apiClient.login(LoginRequest(apple_jwt: token, nonce: rawNonce, device_id: deviceID))`；成功 → save + bridge.push + `.signedIn(userID)`；失败 → `.error(err)`；
   - `deviceID` 来源：`await UIDevice.current.identifierForVendor?.uuidString ?? ""`（注意 `UIDevice` 必须在 MainActor 访问，`SignInViewModel` 相应标 `@MainActor` 或用 actor hop）。

9. **Given** `handleAppleSignIn` 的 nonce 约定 **When** 发起 SIWA **Then** 视图层生成 **raw nonce**：`SecRandomCopyBytes(_, 32, &bytes)` → 转 hex string（64 字符）；`request.nonce = sha256(rawNonce hex)` 的 hex（即给 Apple 的是 raw nonce 的 SHA256 hex）；**提交后端的 `LoginRequest.nonce` 是 raw nonce 的 hex（不是 SHA256）**；此约定在 `SignInViewModel` 相应位置加注释固化。

10. **Given** 新文件 `ios/CatPhone/App/Auth/SignInView.swift` **When** 渲染 **Then** 用原生 `SignInWithAppleButton`（AuthenticationServices + SwiftUI），`onRequest` 回调里：设置 `request.requestedScopes = [.email]` + `request.nonce = sha256(rawNonce)`；`onCompletion` 回调：成功把 `ASAuthorization` 交给 VM 的 `handleAppleSignIn`；失败 → VM 切 `.error`；**And** 按钮样式用 `.black` + 圆角 12；**And** 登录失败时显示错误 banner + "重试"按钮（触发一次 sign in 请求，不清 Keychain）；**And** `.reauthRequired` 路径显示"会话已过期，请重新登录"。

11. **Given** 修改 `ios/CatPhone/App/CatPhoneApp.swift` **When** App 启动 **Then** 根据 `SignInViewModel.state` 分视图：`.checking` → `ProgressView()`；`.signedOut / .signingIn / .error` → `SignInView`；`.signedIn` → 原 `SpineboyPreviewView`；**And** VM 在 App 构造时 `Task { await vm.bootstrap() }`；**And** 保留 `#if DEBUG` + 环境变量 `CAT_SKIP_AUTH=1` 的调试旁路（跳过登录直接进主视图），便于开发调试 UI；`#if !DEBUG` 必须走登录门。

12. **Given** `ios/project.yml` + 新文件 `ios/CatPhone/CatPhone.entitlements` **When** 重新跑 `xcodegen generate` **Then**：
   - `ios/CatPhone/CatPhone.entitlements` 是 XML plist，含 `com.apple.developer.applesignin = [Default]`；
   - `project.yml` `CatPhone.settings.base` 加 `CODE_SIGN_ENTITLEMENTS: CatPhone/CatPhone.entitlements`；
   - **不** 给 `CatWatch` target 加 SIWA entitlement（watch 不直接 SIWA，见 2-2c）；
   - iOS 部署目标保持 17.0，无需 Info.plist 改动。

13. **Given** `SignInViewModelTests`（放 `ios/CatShared/Tests/CatSharedTests/` 因 ViewModel 在 CatPhone target 无法用 SPM 测——见 Dev Notes 备选方案）**When** 运行 **Then** 用 mock `APIClient` / `TokenStore` / `WatchTokenBridging` + 可注入 `nowFn` 覆盖：bootstrap 四分支（空 / access 未过期 / access 过期 refresh 成功 / refresh 失败）、handleAppleSignIn 成功 + 失败、reauthRequired 路径自动 clear。

14. **Given** `WatchTokenBridging` protocol **When** 2-2c 接入 **Then** 本故事交付的 protocol 契约：`func push(_ pair: TokenPair) async throws`（iPhone 登录成功 / refresh 成功都调）；**And** 本故事内**不**实现 `WCSession` 发送（那是 2-2c 的职责）；**And** 提供 `NoopWatchTokenBridge: WatchTokenBridging`（空实现）给本故事用，让本故事可独立编译运行 + 测试。

15. **Given** 本 PR **When** 提交 **Then**：
   - macOS 上跑 `cd ios && xcodegen generate` 工程可生成；
   - `xcodebuild -scheme CatPhone -destination 'platform=iOS Simulator,name=iPhone 15' build` 通过；
   - `xcodebuild test -scheme CatShared -destination 'platform=iOS Simulator,name=iPhone 15'` 通过；
   - `SignInViewModelTests` + `KeychainStoreTests` + `APIClientTests` 全绿；
   - Windows 本地无 macOS 时必须在 Completion Notes 注明"iOS 编译/测试待 CI"。

## Tasks / Subtasks

- [ ] **Task 1: CatShared — APIClient + TokenPair + AuthAPI** (AC: #1–#4, #7)
  - [ ] 1.1 `Networking/HTTPMethod.swift` + `Networking/APIError.swift` + `Networking/TokenPair.swift`
  - [ ] 1.2 `Networking/APIClient.swift` — actor + baseURL + send + 401 refresh + 并发去重
  - [ ] 1.3 `Networking/AuthAPI.swift` — `LoginRequest` / `RefreshRequest` + `extension APIClient { login / refresh }`
  - [ ] 1.4 `Networking/WatchTokenBridging.swift` — protocol + `NoopWatchTokenBridge`
  - [ ] 1.5 `Tests/CatSharedTests/APIClientTests.swift` — URLProtocol stub + 7 场景

- [ ] **Task 2: CatShared — KeychainTokenStore** (AC: #5, #6)
  - [ ] 2.1 `Persistence/TokenStore.swift`（protocol）
  - [ ] 2.2 `Persistence/KeychainStore.swift`（actor KeychainTokenStore + KeychainError）
  - [ ] 2.3 `Tests/CatSharedTests/KeychainStoreTests.swift` — 4 场景

- [ ] **Task 3: CatPhone — SignInView + ViewModel** (AC: #8–#11, #13)
  - [ ] 3.1 `App/Auth/SignInViewModel.swift` — state machine + bootstrap + handleAppleSignIn
  - [ ] 3.2 `App/Auth/SignInView.swift` — SignInWithAppleButton + nonce 生成 + 错误 banner
  - [ ] 3.3 `App/Auth/Nonce.swift` — `rawNonce()` + `sha256Hex(_:)` 工具
  - [ ] 3.4 `App/CatPhoneApp.swift` 改写为 state-driven 分视图 + DEBUG skip flag
  - [ ] 3.5 `Tests/CatSharedTests/SignInViewModelTests.swift`（或放 CatPhoneTests，取决于 target 可测性，见 Dev Notes）

- [ ] **Task 4: project.yml + entitlements** (AC: #12)
  - [ ] 4.1 新增 `ios/CatPhone/CatPhone.entitlements`
  - [ ] 4.2 `ios/project.yml` `CatPhone.settings.base` 加 `CODE_SIGN_ENTITLEMENTS`
  - [ ] 4.3 本地 `xcodegen generate` 确认工程能生成 + 打开

- [ ] **Task 5: macOS 验证** (AC: #15)
  - [ ] 5.1 `xcodebuild -scheme CatPhone -destination 'platform=iOS Simulator,name=iPhone 15' build`
  - [ ] 5.2 `xcodebuild test -scheme CatShared -destination 'platform=iOS Simulator,name=iPhone 15'`
  - [ ] 5.3 Simulator 人工回归：首次登录 / 重启后 bootstrap 直接进主视图 / Keychain 清空后回登录
  - [ ] 5.4 Completion Notes 写清 iOS 验证环境（macOS 版本 / Xcode 版本 / Simulator 机型）

## Dev Notes

### 与 2-2a / 2-2c 的契约边界

- **从 2-2a 消费**：DTO 字段名 + 错误码 + access/refresh TTL（access 7 天的 `access_expires_at` 是权威）。API 路径 `/v1/auth/login` `/v1/auth/refresh`。
- **给 2-2c 暴露**：CatShared 里 `APIClient / TokenStore / KeychainTokenStore / TokenPair / AuthAPI / WatchTokenBridging protocol`。2-2c 实现 `WatchConnectivityBridge: WatchTokenBridging` + `WatchTokenReceiver`，并在 CatWatch 端复用 `KeychainTokenStore`（同一份代码）。
- 因此 **CatShared 里的所有新代码必须 `@available(iOS 17.0, watchOS 10.0, *)` 兼容**，`Security.framework` 在两平台都可用，`URLSession` 在两平台都可用（但**禁止** `URLSessionConfiguration.background`，watchOS 受限）。

### 最容易翻车的 Top 7

1. **Nonce 两份不能搞混**：iOS 端生成 raw nonce → 给 Apple 的是 `sha256(raw).hex`，给后端的是 `raw.hex`。在 `Nonce.swift` 集中做 + 注释 + 单测。
2. **并发 refresh 去重**：别用 `NSLock`，用 actor 内部 `refreshTask: Task<TokenPair, Error>?`。多请求看同一个 task 就够了。单测要用两个 `async let` 并行触发 401 验证。
3. **JSON snake_case 一致性**：`keyEncodingStrategy = .useDefaultKeys` + 属性名直接 `access_token`（保留下划线 Swift 属性名，牺牲美观换零转换错误）。如果坚持 camelCase，两个方向都要加 `.convertFromSnakeCase / .convertToSnakeCase`，一致性容易出错。
4. **Keychain 第二次写必须 update**：`SecItemAdd` 会返回 `errSecDuplicateItem`；实现时先 `SecItemCopyMatching` → 存在则 `SecItemUpdate`，否则 `SecItemAdd`。单测覆盖。
5. **`kSecAttrAccessible`**：用 `AfterFirstUnlockThisDeviceOnly`，不上 iCloud。
6. **UIDevice MainActor**：`identifierForVendor` 必须在 MainActor 访问。ViewModel 整体 `@MainActor` 最省心。
7. **`@Observable` vs `ObservableObject`**：iOS 17 用 `@Observable`（`Observation` 框架）；如果 watchOS target 有 10.0 及以上，`@Observable` 两平台都行。

### ViewModel 测试位置

SignInViewModel 放 `CatPhone` target 下会被 `CatPhoneTests` 测；但 CatPhone 是 app target，很多团队踩坑发现 SPM 包 `CatShared` 无法直接测 app-target 里的类。**建议**：
- Option A（推荐）：把 `SignInViewModel` 移到 `CatShared/Sources/CatShared/Auth/`（SPM 包），`SignInView` 留在 CatPhone（因为要 import `AuthenticationServices` 的 UI 组件）。测试跟 SPM 走。
- Option B：ViewModel 留 CatPhone，测试写 `CatPhoneTests`（需 `project.yml` 有 CatPhoneTests target，当前已配置）。

Task 3.5 默认按 Option A；如果项目 target 约束让 A 不可行（如 `AuthenticationServices` 在 watchOS 无法 import 导致 CatShared 编译失败），退化到 Option B 并在 Completion Notes 声明。

### 登录流程时序

```text
CatPhone boot
  → SignInViewModel.bootstrap()
    → TokenStore.loadTokens()
      ├── 非空 + access 未过期 → .signedIn → SpineboyPreviewView
      ├── 非空 + access 过期 → APIClient.refresh(...)
      │     ├── 成功 → save + bridge.push → .signedIn
      │     └── 失败 → TokenStore.clear() → .signedOut
      └── 空              → .signedOut
  → .signedOut → SignInView
     → 点 SignInWithAppleButton
     → Apple 回调 → handleAppleSignIn
       → APIClient.login(apple_jwt, raw_nonce_hex, device_id)
         → TokenStore.save + WatchTokenBridging.push
         → .signedIn
```

### TokenPair 字段约定（与 2-2a 完全对齐）

```swift
public struct TokenPair: Codable, Sendable {
    public let user_id: String
    public let access_token: String
    public let refresh_token: String
    public let access_expires_at: Date    // iso8601 → Date
    public let refresh_expires_at: Date
    public var login_outcome: String?     // nil for refresh
    public init(...) { ... }
}
```

### Library 版本

| 库 | 版本 | 理由 |
|---|---|---|
| `AuthenticationServices` | iOS 17 SDK | SignInWithAppleButton |
| `Security` | iOS/watchOS SDK | Keychain |
| `CryptoKit` | iOS/watchOS SDK | SHA256 |
| 其它 | — | **不引入** 三方 Keychain / OAuth 包 |

### Project Structure Notes

```
ios/
├── CatShared/Sources/CatShared/
│   ├── Networking/
│   │   ├── APIClient.swift
│   │   ├── APIError.swift
│   │   ├── AuthAPI.swift
│   │   ├── HTTPMethod.swift
│   │   ├── TokenPair.swift
│   │   └── WatchTokenBridging.swift
│   ├── Persistence/
│   │   ├── TokenStore.swift
│   │   └── KeychainStore.swift
│   └── Auth/                          # 若采用 Option A
│       └── SignInViewModel.swift
├── CatShared/Tests/CatSharedTests/
│   ├── APIClientTests.swift
│   ├── KeychainStoreTests.swift
│   └── SignInViewModelTests.swift     # 若采用 Option A
├── CatPhone/
│   ├── CatPhone.entitlements          # 新增
│   └── App/
│       ├── CatPhoneApp.swift          # 修改
│       └── Auth/
│           ├── SignInView.swift
│           └── Nonce.swift
└── project.yml                        # 修改（entitlements 路径）
```

### PR 自检清单

- [ ] `xcodegen generate` 成功
- [ ] `xcodebuild` CatPhone + CatShared 全通过
- [ ] `APIClientTests` / `KeychainStoreTests` / `SignInViewModelTests` 全绿
- [ ] Simulator 人工回归三条路径（首次登录 / bootstrap 直进 / Keychain 清空后回登录）
- [ ] 无三方 OAuth / Keychain 库引入
- [ ] 无 `print(...)` / `NSLog` 调试语句
- [ ] `#if DEBUG` 的 CAT_SKIP_AUTH 标志只影响 DEBUG build
- [ ] CatShared 所有新类型 `@available(iOS 17.0, watchOS 10.0, *)` 兼容

### References

- [Source: _bmad-output/implementation-artifacts/2-2a-auth-backend.md — API 契约 / DTO / 错误码]
- [Source: _bmad-output/planning-artifacts/epics.md §Epic 2 Story 2.2b lines TBD（本拆分落地后）]
- [Source: _bmad-output/planning-artifacts/architecture.md §Frontend Architecture / §Data Boundaries（JWT Keychain）]
- [Source: _bmad-output/planning-artifacts/ux-design-specification.md — 登录 SwiftUI Form / Apple HIG 原生 line 359]
- [Source: _bmad-output/planning-artifacts/prd.md §FR36 / §FR58]
- [Source: ios/project.yml — CatPhone bundle id `com.zhuming.cat.phone`]
- [Source: Apple HIG — Sign in with Apple 按钮样式要求]
- [Source: Apple Developer — ASAuthorizationAppleIDProvider / SignInWithAppleButton / nonce 处理流程]

### 衔接 Story

- **硬依赖**：Story 2-2a（后端 API）— 必须先 done / review。
- **阻塞**：Story 2-2c（Watch 接收端）— 消费本故事的 `WatchTokenBridging` protocol 和 `CatShared` 的网络 / Keychain 代码。
- **后续复用**：Epic 2.3+ 所有需要 API 访问的 iPhone 端 Story 直接注入 `APIClient`。

## Dev Agent Record

### Agent Model Used

{{agent_model_name_version}}

### Debug Log References

### Completion Notes List

### File List
