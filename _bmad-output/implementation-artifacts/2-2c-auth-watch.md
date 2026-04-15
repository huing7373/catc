# Story 2.2c: Sign in with Apple 认证——Apple Watch 客户端

Status: ready-for-dev

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Story

As a Apple Watch 用户,
I want 当 iPhone 登录成功后，手表自动拿到 token 并持久化到 Watch Keychain; 手表端 API 请求能正常带 token / 自动 refresh; 未登录时显示清晰引导,
so that 我不用在 watchOS 上操作 SIWA（watchOS 无原生 SIWA UI），但体验上手表跟 iPhone 一样"登着"。

> **拆分背景**：原 Story 2-2 是 full-stack，按 backend / iPhone / Watch 三拆。本故事是**第三拆：Watch**；不包含任何后端代码（由 **2-2a**）或 iPhone 端 SIWA UI / APIClient / Keychain（由 **2-2b** 交付到 CatShared）。
>
> **基线与依赖**：
> - **硬依赖 2-2b**：`CatShared` 已提供 `APIClient / TokenStore / KeychainTokenStore / TokenPair / AuthAPI / WatchTokenBridging protocol`。
> - 本故事工作：(a) iPhone 端实现 `WatchConnectivityBridge: WatchTokenBridging`（WCSession 发送）；(b) Watch 端实现 `WatchTokenReceiver`（WCSession 接收）；(c) Watch 端登录状态 gating UI（`CatWatchApp` 根据 Keychain 分视图）；(d) 测试。
> - watchOS 端不发 SIWA：Apple Watch 无独立 SIWA 流程（WWDC 明确），token 必须从 iPhone 通过 WCSession 推送过来。

## Acceptance Criteria

1. **Given** 新文件 `ios/CatShared/Sources/CatShared/Networking/WatchConnectivityBridge.swift`（或同等位置）**When** iPhone 端 `SignInViewModel` 调 `bridge.push(tokenPair)` **Then** 在 iPhone 侧用 `WCSession.default.updateApplicationContext(["auth.tokens.v1": data])` 推送（`data = JSONEncoder().encode(TokenPair)`）：
   - 选 `updateApplicationContext` 而非 `sendMessage`（后者要求 watch 前台）；
   - 单例 `WatchConnectivityBridge: NSObject, WCSessionDelegate, WatchTokenBridging`；`init` 里 `WCSession.default.delegate = self` + `activate()`；
   - `push(_:)` 失败（WCSession 未激活 / 未配对 / iCloud 问题）**不抛错**：log warn 后静默返回（登录流程不能被 WCSession 阻塞，下次 refresh 会重推）；
   - **仅 iOS**：整个文件用 `#if os(iOS)` 包裹，避免 Watch target 误编译。

2. **Given** 新文件 `ios/CatWatch/App/Auth/WatchTokenReceiver.swift` **When** 手表 App 启动 **Then**：
   - 单例 `WatchTokenReceiver: NSObject, WCSessionDelegate`；`activate()` 在 `CatWatchApp.init()` 调一次；
   - 实现 `session(_: WCSession, didReceiveApplicationContext: [String: Any])`：取 `"auth.tokens.v1"` 对应的 `Data` → `JSONDecoder().decode(TokenPair.self, ...)` → 用 CatShared 的 `KeychainTokenStore` 落盘；
   - 解码失败 / key 不存在 → log warn 忽略（不崩）；
   - 保存成功后向 `CatWatchApp` 的状态发个通知（`NotificationCenter` + 自定义 Notification.Name，或 `@Observable` 类的更新）使 UI 从"待登录"状态切换到"已登录"；
   - **仅 watchOS**：整个文件用 `#if os(watchOS)` 包裹。

3. **Given** `ios/CatWatch/App/CatWatchApp.swift` **When** App 启动 **Then**：
   - 在 `init()` / `.task` 里：激活 `WatchTokenReceiver`、读一次 `KeychainTokenStore.loadTokens()`；
   - 引入 `WatchAuthState` `@Observable` 单例：`state: .checking, .signedOut, .signedIn(userID)`；
   - 启动时 `state = .checking`，读 Keychain：
     - 非空 + access 未过期 → `.signedIn`；
     - 非空 + access 过期 → 后台调 `APIClient.refresh(...)`，成功 → `.signedIn`；失败 → `KeychainTokenStore.clear()` + `.signedOut`；
     - 空 → `.signedOut`；
   - `body` 根据 state 分视图：`.checking` → `ProgressView`；`.signedOut` → `WatchWaitingForPhoneView`（见 AC #4）；`.signedIn` → 原 `CatSpineView` 或现有主视图；
   - `WatchTokenReceiver` 收到新 token 保存成功后，`WatchAuthState` 切到 `.signedIn(userID)`（无缝刷新）。

4. **Given** 新文件 `ios/CatWatch/App/Auth/WatchWaitingForPhoneView.swift` **When** 手表端未登录 **Then** 显示静默引导：
   - 图标（`systemImage: "iphone.and.arrow.forward"` 或类似 SF Symbol）；
   - 文字 "请在 iPhone 上登录"（简体中文，对齐 UX 风格）；
   - 字体 `.title3` + `.multilineTextAlignment(.center)` + 适配 Apple Watch 各尺寸（40mm/44mm/45mm/49mm）；
   - **不** 提供任何 SIWA 按钮 / 登录入口（watchOS 不支持 native SIWA）；
   - 可选：底部小字 "iPhone 登录后手表会自动同步"。

5. **Given** `WatchTokenReceiver` 在 Watch App 处于**后台**时 **When** iPhone 推送 `updateApplicationContext` **Then** watchOS 会唤醒 App（系统行为，不需要代码特殊处理），`didReceiveApplicationContext` 被调用；**And** 即使 App 进入后台前是 `.signedOut`，唤醒后下一次前台 App 会走 `CatWatchApp.bootstrap()` 发现 Keychain 非空自动切 `.signedIn`。

6. **Given** 手表端 API 请求 **When** access 过期 **Then** 复用 CatShared 的 `APIClient` 自动 refresh 逻辑（2-2b 已交付）；**And** refresh 失败（如 refresh 过期、死号）→ `KeychainTokenStore.clear()` + `WatchAuthState` 切 `.signedOut` + 显示 `WatchWaitingForPhoneView`（不主动联系 iPhone，下次 iPhone 前台登录后 `WatchTokenBridge` 会推新 token）；**And** 本故事**不要求** Watch 触发 iPhone 重新登录（太复杂，Growth 再考虑）。

7. **Given** iPhone 侧新文件 `ios/CatPhone/App/Auth/WatchConnectivityActivation.swift` **When** `CatPhoneApp` 启动 **Then** 在 App 的 `init()` 或 `.task` 里激活 `WatchConnectivityBridge.shared`（让 iOS WCSession delegate 就位）；**And** 如果没配对 watch（`WCSession.isPaired == false` 或 `isWatchAppInstalled == false`），bridge.push 仍然调用但内部静默 no-op，不影响 iPhone 登录流程；**And** 2-2b 里 `SignInViewModel.bridge` 在本故事**替换**为 `WatchConnectivityBridge.shared`（2-2b 用的是 `NoopWatchTokenBridge`）。

8. **Given** `WatchTokenReceiverTests` **When** 运行 **Then** 用 mock `WCSession` protocol 抽象（自定义 `WCSessionProtocol` 让依赖可注入）覆盖：
   - `didReceiveApplicationContext` 拿到合法 TokenPair → Keychain 保存成功 + WatchAuthState 切 signedIn；
   - 拿到非法 JSON → warn + 不崩 + state 不变；
   - key 缺失 → warn + 不崩。

9. **Given** `WatchConnectivityBridgeTests` **When** 运行 **Then** 用 mock `WCSessionProtocol` 覆盖：
   - `push(tokenPair)` 正常调用触发 `updateApplicationContext` 一次，payload 含 `"auth.tokens.v1"`；
   - WCSession 未激活 → push 不抛错（实际 updateApplicationContext 抛了就捕获 + log warn）；
   - TokenPair JSON 编码后 payload 可被 WatchTokenReceiver 同一份代码解开（round-trip 测试）。

10. **Given** `CatWatchApp.bootstrap` 的 Keychain 读/refresh 逻辑 **When** 测试 **Then** 抽离成独立函数 `func resolveWatchAuthState(tokenStore:, apiClient:, now:) async -> WatchAuthState`，单测覆盖四分支（空 / 未过期 / 过期 refresh 成功 / 过期 refresh 失败）；**And** mock `APIClient` + mock `TokenStore`。

11. **Given** Watch target 的 Info.plist / project.yml **When** 本故事交付 **Then**：
    - `WCSession` 依赖（`WatchConnectivity.framework`）已在 `ios/project.yml` 的 `CatWatch` target 声明（现状已有）；
    - **不** 给 `CatWatch` 加 `com.apple.developer.applesignin` entitlement（关键：手表不直接 SIWA）；
    - `WatchWaitingForPhoneView` 在 watchOS 10.0+ 可用（最低部署目标已对齐）。

12. **Given** 本 PR **When** 提交 **Then**：
    - macOS 上 `cd ios && xcodegen generate` 工程可生成；
    - `xcodebuild -scheme CatWatch -destination 'platform=watchOS Simulator,name=Apple Watch Series 9 (45mm)' build` 通过；
    - `xcodebuild test -scheme CatShared -destination 'platform=watchOS Simulator,name=Apple Watch Series 9 (45mm)'` 通过（CatShared 在 watchOS 也要能跑，APIClient / Keychain 测试都在这里）；
    - Pair Simulator 人工回归：(a) iPhone 未登录 → Watch 打开显示 "请在 iPhone 上登录"；(b) iPhone 登录成功 → Watch 在 10 秒内自动切到主视图；(c) iPhone Keychain 手动清空 → 下次 iPhone 前台 bootstrap 失败后 Watch 的 refresh 失败 → 回到引导页；
    - Windows 本地无 macOS 时在 Completion Notes 注明"iOS/watchOS 编译/测试待 CI"。

## Tasks / Subtasks

- [ ] **Task 1: iPhone 侧 WatchConnectivityBridge** (AC: #1, #7, #9)
  - [ ] 1.1 `ios/CatShared/Sources/CatShared/Networking/WatchConnectivityBridge.swift`（`#if os(iOS)`；实现 `WatchTokenBridging` + `WCSessionDelegate`；单例 `.shared`）
  - [ ] 1.2 引入 `WCSessionProtocol` 让 `updateApplicationContext` 可注入 mock
  - [ ] 1.3 `ios/CatPhone/App/Auth/WatchConnectivityActivation.swift` — 在 `CatPhoneApp.init()` 激活 bridge
  - [ ] 1.4 `ios/CatPhone/App/CatPhoneApp.swift` — `SignInViewModel` 的 `bridge` 从 Noop 换成 `WatchConnectivityBridge.shared`
  - [ ] 1.5 `Tests/CatSharedTests/WatchConnectivityBridgeTests.swift`（iOS only 用 `#if os(iOS)` 包）

- [ ] **Task 2: Watch 侧 WatchTokenReceiver** (AC: #2, #5, #8)
  - [ ] 2.1 `ios/CatWatch/App/Auth/WatchTokenReceiver.swift`（`#if os(watchOS)`）
  - [ ] 2.2 `WCSessionProtocol` 复用 Task 1 的抽象
  - [ ] 2.3 `Tests/CatSharedTests/WatchTokenReceiverTests.swift` 三分支

- [ ] **Task 3: Watch 登录状态 UI + 启动逻辑** (AC: #3, #4, #6, #10)
  - [ ] 3.1 `ios/CatWatch/App/Auth/WatchAuthState.swift`（`@Observable`）
  - [ ] 3.2 `ios/CatWatch/App/Auth/WatchWaitingForPhoneView.swift`
  - [ ] 3.3 `ios/CatWatch/App/Auth/WatchAuthBootstrap.swift` — `func resolveWatchAuthState(...) async -> WatchAuthState` + 四分支测试
  - [ ] 3.4 修改 `ios/CatWatch/App/CatWatchApp.swift` — state-driven 分视图
  - [ ] 3.5 `Tests/CatSharedTests/WatchAuthBootstrapTests.swift`

- [ ] **Task 4: macOS 验证 + Pair Simulator 回归** (AC: #11, #12)
  - [ ] 4.1 `cd ios && xcodegen generate`
  - [ ] 4.2 `xcodebuild -scheme CatWatch` build
  - [ ] 4.3 `xcodebuild test -scheme CatShared` watchOS 目标
  - [ ] 4.4 Pair Simulator 三条手工回归路径
  - [ ] 4.5 Completion Notes 写清环境 + 若无 macOS 声明"待 CI"

## Dev Notes

### WCSession 基本事实（不要踩的坑）

- `updateApplicationContext(_:)` 会**覆盖**上一次的 applicationContext（只保留最新），系统保证一定能送达（可能延迟到 Watch 唤醒）。适合 token 这种"最新状态即真理"的场景。
- `sendMessage(_:replyHandler:)` 要求**双方都在前台**，不适合 token 推送。
- `transferUserInfo(_:)` 是队列式，会按序送达，适合事件流，不适合 token（用这个反而容易让旧 token 覆盖新 token）。
- WCSession 在 iPhone 端单例，Watch 端也是单例；两端都要**显式** `activate()` 并设置 delegate。
- `didReceiveApplicationContext` 在 watch 后台时也会被唤醒触发（系统行为），这也是选它的理由。

### 手表端不做 SIWA 的理由

- watchOS 没有原生 `SignInWithAppleButton` 控件，`ASAuthorizationController` 在 watchOS 上的表现不稳定，且 Apple 官方文档要求 SIWA 在 iPhone / iPad / Mac 发起。
- 本项目定位"Apple Watch 伴侣应用"，iPhone 必存在 → 强约束用户在 iPhone 登录是合理的。
- Growth 阶段如果出现纯 Watch 场景（如 Cellular 独立 watch 首次启动无 iPhone），再考虑 QR 扫码或其它跨设备登录方案——当前阶段拒绝。

### WCSessionProtocol 设计

为了让两端代码可测，自定义一个最小 protocol：

```swift
protocol WCSessionProtocol: AnyObject, Sendable {
    var isReachable: Bool { get }
    var isPaired: Bool { get }           // iOS only，watchOS 返回 true
    var isWatchAppInstalled: Bool { get } // iOS only
    var activationState: WCSessionActivationState { get }
    func activate()
    func updateApplicationContext(_ context: [String: Any]) throws
}

extension WCSession: WCSessionProtocol {}
```

Mock：`final class FakeWCSession: WCSessionProtocol` 存 `lastContext`、`activateCount`、可注入 `throwOnUpdate` 开关。

### Watch 端登录状态时序

```text
CatWatch boot
  → CatWatchApp.init()
    → WatchTokenReceiver.shared.activate()
    → Task { await WatchAuthState.shared.bootstrap() }
      → KeychainTokenStore.loadTokens()
        ├── 非空 + access 未过期 → state=.signedIn
        ├── 非空 + access 过期 → APIClient.refresh(...)
        │     ├── 成功 → save + state=.signedIn
        │     └── 失败 → clear + state=.signedOut
        └── 空 → state=.signedOut
  → state==.signedOut → WatchWaitingForPhoneView
     ...
     → WatchTokenReceiver 收到 applicationContext
       → decode TokenPair → KeychainTokenStore.save
       → WatchAuthState.shared.onTokenReceived(pair) → state=.signedIn
  → state==.signedIn → 原主视图（CatSpineView 等）
```

### 潜在踩坑 Top 5

1. **WCSession activation 时机**：必须在 App 很早（`init` 或 `.onAppear`）激活一次，否则 `updateApplicationContext` 送的值在 delegate 未设置时会丢。一律在 `CatWatchApp.init()` 里 `_ = WatchTokenReceiver.shared`（单例副作用激活）。
2. **TokenPair 编码一致**：iPhone 端编码用的 `JSONEncoder` 配置和 Watch 端解码用的 `JSONDecoder` 必须配对（两端都用 CatShared 里同一套 `APIClient` 的 encoder/decoder 设置，或者在 CatShared 提供 `TokenPair.encode()` / `decode()` 静态方法）。
3. **applicationContext 单条目限制**：只保留最新一次，key `"auth.tokens.v1"` 就固定一个。版本号 `v1` 方便未来升级。
4. **配对但未安装 watch App**：iPhone 端 `isPaired=true, isWatchAppInstalled=false` 时 `updateApplicationContext` 不抛错但也不生效，不要把这当作失败——log info 一下即可。
5. **watchOS target 不能 import AuthenticationServices**：确认 CatShared 里的 `SignInViewModel`（由 2-2b 交付）不依赖 `AuthenticationServices`，UI 层依赖留在 CatPhone target（2-2b 已按此拆）。本故事工作不应触碰。

### Project Structure Notes

```
ios/
├── CatShared/Sources/CatShared/
│   └── Networking/
│       └── WatchConnectivityBridge.swift     # 新增（iOS only）
├── CatShared/Tests/CatSharedTests/
│   ├── WatchConnectivityBridgeTests.swift    # 新增（iOS only）
│   ├── WatchTokenReceiverTests.swift         # 新增（watchOS only）
│   └── WatchAuthBootstrapTests.swift         # 新增（共享）
├── CatPhone/App/Auth/
│   └── WatchConnectivityActivation.swift     # 新增
├── CatPhone/App/
│   └── CatPhoneApp.swift                     # 修改（bridge 从 Noop 换真实）
├── CatWatch/App/
│   ├── CatWatchApp.swift                     # 修改（state-driven 分视图）
│   └── Auth/
│       ├── WatchAuthState.swift              # 新增
│       ├── WatchAuthBootstrap.swift          # 新增
│       ├── WatchTokenReceiver.swift          # 新增
│       └── WatchWaitingForPhoneView.swift    # 新增
└── project.yml                               # 无需改（WatchConnectivity.framework 现状已有）
```

### PR 自检清单

- [ ] iPhone bridge 的 `push` 失败不阻塞登录流程
- [ ] Watch bootstrap 四分支全有测试
- [ ] `WCSessionProtocol` 抽象让测试无需真 WCSession
- [ ] Pair Simulator 人工回归三条路径全过
- [ ] 无 SIWA 按钮误入 Watch target
- [ ] Watch 端 `print(...)` / `NSLog` 零
- [ ] TokenPair 跨 iPhone↔Watch JSON round-trip 单测
- [ ] `applicationContext` key 版本化（`auth.tokens.v1`）

### References

- [Source: _bmad-output/implementation-artifacts/2-2a-auth-backend.md — TokenPair DTO 字段 / 错误码]
- [Source: _bmad-output/implementation-artifacts/2-2b-auth-iphone.md — WatchTokenBridging protocol / CatShared 网络 & Keychain 契约]
- [Source: _bmad-output/planning-artifacts/epics.md §Epic 2 Story 2.2c lines TBD（本拆分落地后）]
- [Source: _bmad-output/planning-artifacts/architecture.md §Frontend Architecture §SpriteKit ↔ SwiftUI 通信 / §Data Boundaries]
- [Source: _bmad-output/planning-artifacts/prd.md §FR40（watchOS 版本提示）/ §FR58（换机恢复）]
- [Source: Apple Developer — WCSessionDelegate / updateApplicationContext(_:) 官方文档]
- [Source: ios/project.yml — CatWatch target 依赖 / bundle id `com.zhuming.cat.phone.watchapp`]

### 衔接 Story

- **硬依赖**：Story 2-2a + Story 2-2b — 必须先 done / review。
- **后续复用**：Epic 3.3 (手表皮肤缓存 + 5 层渲染) / Epic 4.2+ (盲盒解锁) 的 Watch 端 API 调用直接复用 CatShared 的 `APIClient` + `KeychainTokenStore`。
- **Epic 2.3 手表兼容性检测**：iPhone 端工作，与本故事解耦。

## Dev Agent Record

### Agent Model Used

{{agent_model_name_version}}

### Debug Log References

### Completion Notes List

### File List
