---
date: 2026-05-09
source_review: codex review --uncommitted (epic-loop round 1, /tmp/epic-loop-review-12-2-r1.md)
story: 12-2-websocketclient-封装
commit: e9bcf77
lesson_count: 2
---

# Review Lessons — 2026-05-09 — WebSocket connect() 必须 await 握手结果 & DI 容器 fallback 必须跟随注入的 baseURL（12-2 r1）

## 背景

Story 12.2 用 `URLSessionWebSocketTask` 落地 `WebSocketClient` 真实实装。codex round 1 review 提出两条 finding：

1. P1：`WebSocketClientImpl.connect(roomId:)` 在 socket handshake 失败时仍返回成功 —— 4001 / 4004 / DNS / TLS 等失败被吞，调用方没法决定 retry / relogin。
2. P2：`AppContainer(apiClient: ...)` 注入非默认 backend 但不传 `webSocketClient` 时，fallback 还是用 `Bundle.main` 默认 `PetAppBaseURL` 派生 WS —— REST 打注入 backend，WS 打 default host，split-brain。

两条都是真实 bug，按 fix 处理。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `connect(roomId:)` 在握手期失败仍返回成功 | high (P1) | error-handling | fix | `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:172-178` |
| 2 | `AppContainer` 默认 WS fallback 不跟随注入的 `apiClient.baseURL` | medium (P2) | architecture | fix | `iphone/PetApp/App/AppContainer.swift:209-214` |

## Lesson 1: `connect()` 必须阻塞 await 握手第一帧 / 第一次 error

- **Severity**: high (P1)
- **Category**: error-handling
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift`

### 症状（Symptom）

`connect(roomId:)` 内只调 `task.resume()` + `startReceiveLoop`，立即 return。caller `try await connect(...)` 永远成功。真实失败（DNS 解析失败 / TLS 握手失败 / server 4001 token 过期 / 4004 房间满 / 1011 internal error）只在 receive 长任务里观察到 —— 当时只 `os_log .error` + `continuation.finish()` 让 messages stream 终结，**connect 调用方拿不到任何错误信号**，无法决定是 retry 还是 relogin。

### 根因（Root cause）

`URLSessionWebSocketTask.resume()` 是同步方法，调它**不**等于建连完成。WebSocket 握手（HTTP upgrade + server 端 token 校验 + room_members 校验）发生在 `resume()` 之后；失败信号通过 `task.receive()` 抛 URLError 暴露。原实装没在 connect 内部架"等握手结果" 的桥 —— 把 `task.resume()` 立即 return 误当成"建连完成"。

iOS WebSocket API 没有显式的 `didOpenWithProtocol` callback 强制 await（除非走 URLSessionWebSocketDelegate）；现有非-delegate 实装只能用"first receive 成功 / 第一次 error" 作为握手结果代理信号 —— V1 §12.1 钦定 server 握手成功后**必发** `room.snapshot` 作为第一条消息，所以这个代理在协议层是稳的。

### 修复（Fix）

引入一次性 latch `connectGate: CheckedContinuation<Void, Error>?`：

- `connect()` 在起 receive loop 前 `lock + connectGate = cont`，然后 `try await withCheckedThrowingContinuation { ... }`
- receive loop 第一帧到达 → `resolveConnectGate(success: true)` → connect() return
- receive loop 第一次 catch error 之前未收过帧 → 由 `defer { resolveConnectGate(success: false, error: connectionFailed) }` 兜底 → connect() throw `WSError.connectionFailed`
- `disconnect()` / `prepareForReconnect()` / `deinit` 期间也兜底 `resolveConnectGate(success: false)`，防 caller 在 connect await 期间被遗弃

`resolveConnectGate` 用 lock + nil-out 模式保证最多 resume 一次。

receive loop 出口必须用 `defer { continuation.finish() }` 兜底所有路径（cancel via `while !Task.isCancelled` 退出、catch、return）—— 没有 defer，cancel 路径会让 stream 永远 hang 在消费方 for-await 上。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **包装异步 handshake 协议（WS / SSE / TCP / 自定义协议）的 connect/dial 方法** 时，**必须** **让 connect 阻塞 await 第一个握手 ACK 信号（成功 ACK 或失败 ACK），不能拿"已 enqueue dial 任务"代替"建连完成"**。
>
> **展开**：
> - `URLSessionWebSocketTask.resume()` 这类同步 enqueue 方法**不**等于握手成功；caller 拿不到失败信号就没法做 retry / relogin
> - 协议层有"server 必发首条 message"约定（如 `room.snapshot`）的，可用 first-frame 代理 handshake-OK
> - 协议层无该约定的，必须用 URLSessionWebSocketDelegate 的 `didOpenWithProtocol:` / `didCloseWith:` callback 接 success / failure
> - 引入 `CheckedContinuation` 一次性 latch 时 ：lock + nil-out 字段 + `resolve` helper 防双 resume
> - `disconnect()` / `cancel()` / `deinit` 必须兜底 resume gate（`resume(throwing:)`），否则 caller 在 connect await 期间永久 hang
> - receive 长任务退出路径**必须**用 `defer { continuation.finish() }`，覆盖 normal cancel + catch error + return 全部三条路径；只在 catch 里 finish 会漏掉 `while !Task.isCancelled` 退出
> - **反例**：调 `task.resume(); startReceiveLoop(); return`（没 await 握手结果）→ 失败被吞；调用方做 `try await connect(); start_business_action()`，business_action 在已 closed 的 task 上跑导致整链 transparent fail

## Lesson 2: DI 容器 fallback 必须跟随**已注入**依赖的配置，不能退回 `Bundle.main`

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/App/AppContainer.swift`

### 症状（Symptom）

`AppContainer.init(apiClient:keychainStore:...)` 提供测试 / Preview / alt-env 注入入口。注入 `apiClient = APIClient(baseURL: stagingURL)` 但**不**注入 `webSocketClient` 时，fallback 路径硬写：

```swift
self.webSocketClient = webSocketClient ?? WebSocketClientImpl(
    baseURL: AppContainer.resolveDefaultBaseURL(from: Bundle.main), ...)
```

→ REST 打 `staging.example.com`，WS 打 `localhost:8080`（split-brain）。任何 test / preview / alt-env build 一旦触发 `container.webSocketClient.connect(...)` 就会拨到错误 host，且不报错（看起来"连成功"，实际打的根本不是注入的 backend）。

### 根因（Root cause）

DI 容器内多个被注入依赖共享同一配置维度（这里是 `baseURL`），但容器只暴露按依赖类型粒度的 init 参数（`apiClient`、`webSocketClient` 各一个）。当 caller 只覆盖一部分时，剩下那部分的 fallback 路径没有"跟随已注入依赖"的契约，硬写 default value source（`Bundle.main`）—— **fallback 默认值的来源**和**注入路径**走两条不同源数据，注入端只能改一半，另一半 silent 走 default。

旧代码作者把"baseURL 默认值"的 single-source-of-truth 设在 `Bundle.main / fallbackBaseURLString`，没考虑"caller 注入 apiClient 隐含也注入了 baseURL"的语义传递。`APIClientProtocol` 没暴露 `baseURL` getter 进一步加固了"注入 apiClient 时无法读它的 baseURL"的盲点。

### 修复（Fix）

1. `APIClientProtocol` 加 `var baseURL: URL { get }` 协议要求；`APIClient` 把 `private let baseURL` 升级为 `public let baseURL` 满足约束；`AuthBoundaryAPIClient` decorator 透传 `inner.baseURL`
2. 两个测试 mock（`MockAPIClient` / `StatefulMockAPIClient`）加 `var baseURL = URL(string: "http://mock.test")!` 占位字段
3. `AppContainer` 两个 public init 把 fallback 路径从 `resolveDefaultBaseURL(from: Bundle.main)` 改为 `apiClient.baseURL` —— 注入的 apiClient 是什么 baseURL，fallback 派生出的 WebSocketClient 就用什么 baseURL
4. 加回归测试 `testWebSocketClientFallbackUsesInjectedAPIClientBaseURL`：注入 `APIClient(baseURL: "https://staging.example.com")` + 不注入 webSocketClient → 验证 `container.webSocketClient.makeWSURL(...)` 派生的 host 与 stagingURL.host 同源

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **DI 容器 init 提供"部分注入 / 部分 fallback" 形态** 时，**必须** **让 fallback 路径从已注入依赖派生剩余配置（pull from injected sibling），而不是回退到 `Bundle.main` / global-default 这种与注入路径无关的来源**。
>
> **展开**：
> - 容器的多依赖共享同一配置维度（baseURL、namespace、tokenProvider）时，把这个维度暴露在最关键依赖的协议上（`APIClientProtocol.baseURL`），别藏在 private 字段
> - decorator pattern 必须透传被装饰协议的所有 getter（`AuthBoundaryAPIClient.baseURL { inner.baseURL }`），否则 decorator 一包 split-brain 就出现
> - 测试 mock 新加的协议字段需要给占位默认值（`var baseURL = URL(string: "http://mock.test")!`），避免 mock conformance break；占位 URL 用清晰可识别的域名（`mock.test` 比 `localhost` 好，肉眼可见是 mock）
> - `??` 默认值必须 derive from injected sibling，**不**写"回到 global Bundle.main"。`x ?? FromBundle()` 是反面教材
> - **反例**：`webSocketClient ?? WebSocketClientImpl(baseURL: resolveDefaultBaseURL(from: Bundle.main))`（写了 fallback 但 fallback 来源与注入路径脱节，注入 apiClient 后 WS 仍打 default host —— split-brain container）

---

## Meta：本次 review 的宏观教训

两条 finding 都属于 **"看起来工作但 silent fail"** 的家族：

- P1：connect 看起来成功，实际握手挂了（caller 没机会 retry）
- P2：container 看起来正常（apiClient 工作正常），实际 WS 打错地方（业务层接到 connect 成功后调用业务 action，但全打到 default host —— transparent failure）

防御策略一致：**把"信号丢失"的接缝补上**。connect 加 latch 让握手失败有 throw 路径；DI 容器把 fallback 链路从 `Bundle.main` 改成 `apiClient.baseURL` 让"注入路径"和"fallback 路径"共享 single-source-of-truth。

未来设计协议 / 容器时，凡是出现"看起来 OK 但其实没真正完成 / 没真正落到该去的地方"的可能，都需要在协议层加显式 ACK / 显式 single-source-of-truth 字段，提前杜绝 silent fail。
