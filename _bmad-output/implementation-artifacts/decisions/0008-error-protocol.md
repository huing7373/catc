# ADR-0008: 错误协议与 401 恢复路径 —— Bootstrap 三态分发 + Silent Relogin 退役

- **Status**: **Accepted（v2 实施完成，Mac build/test 全绿验证 236/236）** —— v1 主旨"transient/terminal 是协议层概念" 经 codex 独立 review 反对后**降级**为 presentation heuristic；silent relogin 经用户拍板**砍掉**；Story 0008-impl-1 实施 + Mac 验证收官
- **Date**: 2026-04-29（v2 修订）
- **Decider**: TBD（user + Claude）
- **Supersedes**: 不取代 ADR-0006；本 ADR 是协议契约 + client 行为契约层
- **Related Stories**: 1.3 / 1.5 / 1.8（server 错误三层映射）；2.6（iOS 错误 UI）；2.9（LaunchingView）；5.1 / 5.2（Keychain + 启动自动登录）；**5.3 / 5.4（APIClient interceptor + 静默重登 —— 拟退役，见 §6）**；5.5（LoadHomeUseCase —— 11 轮 fix-review 是本 ADR 主要源头）；Epic 6+

---

## 1. Context

### 1.1 触发本 ADR 的现状

截至 2026-04-29，repo 已积攒 **88 条 lesson** + **265 commit / 39% `fix(review):` 前缀**。Story 5.5 单 story 跑了 **11 轮 fix-review**；`AppErrorMapper.swift` 286 行 / 注释占 70%+；bootstrap + 错误链路共 1676 行 Swift（不含测试）。

### 1.2 五层根因（采用 codex 独立反思）

不只是"协议层缺 metadata"一维。

1. **错误语义在多个边界被主动压扁** —— `ErrorPresentation` 被压成 `String`；`try?` 把 throw 与 nil 压成同一路；unknown enum 被 `?? default` 吃掉；server middleware 自己写 envelope 绕过 canonical producer
2. **恢复责任没有被单点定义** —— 谁负责 retry / 谁负责 cold-start / 谁负责 terminal fallback / 谁负责 canonical error decision，在 5.4 / 5.5 中不断跨层漂移
3. **把 UI mode 当成协议层修复器** —— 多轮 fix 不是先修 error type，而是先修 AlertOverlay / RetryView / TerminalErrorView 的分发，把本该类型层解决的下沉成 UI 行为补丁
4. **实现策略偏 patch-driven 不偏 invariant-driven** —— 修一条破一条
5. **代码没有真正承载规则，注释和 lesson 承载规则** —— 正确性来自"有人记得 round 8 为什么推翻 round 7"，而非更硬的接口

### 1.3 v1 → v2 修订

- **v1 主旨"transient vs terminal 是协议层概念"被推翻** —— codex 反思指出该二分混了 (a) retry 可能有用 (b) 用户值得被给 retry 入口 (c) bootstrap vs feature flow 路由维度，不是干净的协议维度。改钦定它为 **presentation heuristic**（policy heuristic, not protocol ontology）
- **v1 §6 silent relogin actor 双字段 invariant 整段删除** —— 用户拍板砍掉。401 改走全局 catch + cold-start 路径
- 22 lesson 中 4 条 silent relogin / actor coalesce 系列**完全 superseded**

---

## 2. Decision（主旨修订版）

### 2.1 核心论断（v2）

> **transient vs terminal 是 client 呈现层的工作启发式（policy heuristic），不升级为协议真理。** Server 协议契约只钦定错误码字典；client 端按"呈现 + 路由"两个维度分发，**不引入** RetrySemantic 协议层 metadata。

理由（采用 codex 反思）：
- "误判代价不对称"是真的（`.alert` 误判 = force-quit；`.retry` 误判 = 多发一次请求）—— 所以 client 默认偏好 `.retry`
- 但**不应**把这个偏好沉淀成协议契约 —— 它涉及 UX 宽容判断，server 不该决定 client 的弹窗策略

### 2.2 三大子决策

- **D1（生产侧）**：server 端 envelope 单一生产者范式 + canonical key（已实装于 ADR-0006，本 ADR 加固为强制契约 —— 详见 §3）
- **D2（client 呈现侧）**：transient/terminal 二分作为 mapper presentation heuristic；bootstrap 路径三态分发；TerminalErrorView 编译期契约（详见 §4、§5）
- **D3（401 恢复侧）**：silent relogin 退役 → 401 走全局 catch + cold-start 重建游客身份（详见 §6）

### 2.3 一条反思（采用 codex §6 警告）

> **"`AppErrorMapper.swift` 现在已经不是 mapper，而是制度补丁汇编 / 文案表 / 恢复分类器 / 迭代史档案 / lesson 索引。别把 mapper 的膨胀误认成架构收敛。"**

本 ADR **不**通过加固 mapper centrality 来解决问题。本 ADR 通过**砍掉 silent relogin** 让一半的 mapper 注释自然消失（详见 §6）。如果实施后 mapper 仍臃肿，下一步是把 case 设计往上游推（让 APIClient 直接产出更少 case），不是再加 mapper 注释。

---

## 3. Server 端契约：单一生产者 + Canonical Key（D1）

ADR-0006 已实装，本节加固为强制契约。

### 3.1 中间件链

```
input → RequestID → Logging → ErrorMapping → Recovery → handler
                       ↑           ↑                       ↑
                   读 canonical  唯一生产者              c.Error 推事件
                   key 写日志    + Set canonical key
```

### 3.2 责任分割

| 中间件 | 角色 | 不许做 |
|---|---|---|
| RequestID | 链路入口 | — |
| Logging | canonical key **消费者** —— `c.Get(ResponseErrorCodeKey)` | 禁止 `apperror.As(c.Errors[0])` 自推 error_code |
| **ErrorMapping** | error envelope **唯一生产者** + canonical key **唯一发布者** | 不能 c.Error 自己；不能在跳过分支 Set canonical key |
| Recovery | panic 兜底 → c.Error 推回管道 | 不直接写 envelope |
| handler / 业务 middleware | error 事件**生产者**：`c.Error(apperror.New(code, msg))` + `c.Abort()` | **禁止** `response.Error(c, ...)`；**禁止**自决 HTTP status；**禁止**自写 envelope |

### 3.3 双向不变式

- **生产侧**：`response.Error` 调用点必须仅在 ErrorMappingMiddleware 内部
- **消费侧**：所有需要 envelope.code 的下游必须读 `c.Get(ResponseErrorCodeKey)`，不读 `c.Errors[0]` 自推
- **跳过分支**：`c.Writer.Written()==true`（double-write 场景）必须**故意不**Set ResponseErrorCodeKey

---

## 4. Client 呈现侧：transient/terminal heuristic（D2）

### 4.1 状态机契约

```swift
enum AppLaunchState {
    case launching
    case ready
    case needsAuth(presentation: ErrorPresentation)  // ← payload 必须是 presentation
}

enum ErrorPresentation {
    case retry(message: String)
    case alert(title: String, message: String)
    case toast(message: String)
}

struct BootstrapMappedError: Error {
    let presentation: ErrorPresentation  // ← 携带预计算的 presentation
    let underlying: Error
}
```

### 4.2 mapper 的 transient/terminal 二分（heuristic，非协议）

| Error case | RetrySemantic（heuristic） | 文案 | bootstrap UI | 非 bootstrap UI |
|---|---|---|---|---|
| `.business(1005/1007/1008/1009)` 限流/冲突/重复/繁忙 | transient | V1 字典 | RetryView | RetryView |
| `.business(其他)` permanent | terminal | V1 字典 | TerminalErrorView | AlertOverlayView |
| `.unauthorized` | **走 §6 全局 catch（不走 mapper presentation）** | — | — | — |
| `.decoding(underlying:)` | transient | "数据异常，请重试" | RetryView | RetryView |
| `.missingCredentials` | **走 §6 全局 catch（cold-start）** | — | — | — |
| `.localStoreFailure(underlying:)` | transient | "登录信息读取异常，请重试" | RetryView | RetryView |
| `.network` | transient | "网络异常，请检查后重试" | RetryView | RetryView |
| 非 APIError fallback | transient | "操作失败，请重试" | RetryView | RetryView |

**注意**：`.unauthorized` / `.missingCredentials` 两个 case **不再由 mapper 派 ErrorPresentation**。它们在 §6 的全局 catch 层被拦截，触发 cold-start，而不进入 bootstrap state machine 的 `.needsAuth` 分发。

### 4.3 transientBusinessCodes 仍是 client 端硬编码集合

```swift
// client 侧硬编码（不再尝试升级为协议层 metadata）
public static let transientBusinessCodes: Set<Int> = [1005, 1007, 1008, 1009]
```

加新业务码时 client 必须显式审视该码归 transient 还是 terminal —— **审视责任明确归 client**，不延迟到协议层。

### 4.4 mapper 文案钦定

- mapper 文案专注表达"什么错了"，不再表达"怎么办"
- 指引文本（"请双击 Home 键..."）上移到 `TerminalErrorView` view 层静态文本

---

## 5. Bootstrap 错误路径契约

### 5.1 钦定的判则（按 bootstrap 流程时序）

**判则 5.1.1（必经 mapper）**：`bootstrapStep1` closure 内**任何**抛给状态机的 error **必须**经 `BootstrapMappedError(presentation: AppErrorMapper.presentation(for: error), underlying: error)` 包装。
- Why：r1 修了 LoadHome 路径漏 mapper；r4 又抓到 GuestLoginUseCase 路径漏 —— "修一处漏一处"反例

**判则 5.1.2（fail-safe retry，禁止 sub-step 短路）**：closure 内已成功的 sub-step **不要**用永久 idempotent flag 跨 retry 短路。
- Why：r2 引入 `GuestLoginCompletionGate` 让 retry 跳过 guest-login，r3 发现 retry 复用坏掉的鉴权状态永远恢复不了
- 钦定：`GuestLoginUseCase.execute()` 必须幂等；retry 时无条件重跑

**判则 5.1.3（冷启动 HTTP 预算 ≤ 2）**：仅 `/auth/guest-login` + `/home`；`.ready` 前禁所有装饰性请求（如 `/ping`）
- 双层 defense：(a) 启动 `.task` 不调装饰性请求；(b) ping 调用挪到 `LaunchedContentView` 的 `.ready` case 内 `.task`

**判则 5.1.4（presentation 携带，禁 message-only）**：`AppLaunchState.needsAuth(...)` 必须携带 `ErrorPresentation`，不能是 `String`。
- Why：r1 把 payload 设为 `userFacingMessage: String` → r2 发现 RootView 永远渲染 RetryView，`.alert` case 被 collapse

**判则 5.1.5（presentationFor fallback 优先级）**：状态机 helper：
1. `BootstrapMappedError` → 用其 `.presentation`
2. 非 `BootstrapMappedError` 的 `LocalizedError` → `errorDescription` 包成 **`.retry(message:)`**
3. plain Error → `defaultFailurePresentation`（`.retry`）

### 5.2 RootView 三态分发

| ErrorPresentation case | bootstrap 路径 | 非 bootstrap 路径 |
|---|---|---|
| `.retry(message:)` | `RetryView`（onRetry → `stateMachine.retry()`） | `RetryView`（onRetry → `presenter.dismiss(triggerOnRetry: true)`） |
| `.alert(title:, message:)` | **`TerminalErrorView`**（无按钮静态全屏） | `AlertOverlayView`（dismiss-able） |
| `.toast(message:)` | bootstrap 阶段同 `.alert` | `AlertOverlayView` 或自定义 toast |

### 5.3 TerminalErrorView 编译期契约

- 位置：`iphone/PetApp/Core/DesignSystem/Components/TerminalErrorView.swift`
- 签名：`public let title: String; public let message: String` —— **无 closure 字段**
- 测试守护：`ErrorComponentSnapshotTests.testTerminalErrorViewHoldsTitleAndMessageOnly` 编译期保证 init 签名

### 5.4 关于 RootView / AppLaunchStateMachine 的反思（采用 codex §5.4）

> 当前 bootstrap 错误分发已经比前几轮稳定，但仍有两个架构味道：
> - `RootView` 知道太多错误史与恢复哲学
> - `AppLaunchStateMachine` 的 fallback 仍混杂了产品展示与兼容逻辑

本 ADR 不在此次重构范围解决这两条，列入 §13 Open Questions。

---

## 6. 401 恢复路径：Silent Relogin 退役 + Cold-Start 重建（D3）

### 6.1 退役决策

**砍掉**以下文件 + 配套测试：

| 文件 | 行数 |
|---|---|
| `iphone/PetApp/Core/Networking/AuthRetryingAPIClient.swift` | 92 |
| `iphone/PetApp/Features/Auth/UseCases/SilentReloginCoordinator.swift` | 138 |
| `iphone/PetApp/Features/Auth/UseCases/SilentReloginUseCase.swift` | 74 |
| 配套测试 | ~500（估算） |

**理由**：800+ 行代码服务一个 MVP 阶段触发频率 < 1% 的场景（详见 v2 修订决策对话）；6 轮 fix-review 才接近收敛仍未稳定；继续维护成本 > 删除成本；重新引入路径清晰（post-MVP 接 server 标准 OAuth refresh token 时再设计）。

### 6.2 替代路径：401 全局 catch + cold-start

```
某次业务请求 → server 返 401 / envelope code=1001
  ↓
APIClient throw .unauthorized
  ↓
全局 catch（位置见下）
  ↓
sessionStore.clear() + keychain 不动（guestUid 保留）
  ↓
state machine 回 .launching
  ↓
bootstrap 重跑（cold-start GuestLoginUseCase 用已有 guestUid 拿新 token + LoadHome）
  ↓
.ready
```

### 6.3 全局 catch 位置 —— A（已拍板，§13.1 Resolved）

**新建 `AuthBoundaryAPIClient: APIClientProtocol`** 装饰器层 —— 网络层统一拦截。

```swift
final class AuthBoundaryAPIClient: APIClientProtocol {
    private let inner: APIClientProtocol
    private let sessionStore: SessionStore
    private let stateMachine: AppLaunchStateMachine

    func request<T>(_ endpoint: Endpoint) async throws -> T {
        do {
            return try await inner.request(endpoint)
        } catch APIError.unauthorized where endpoint.requiresAuth {
            await sessionStore.clear()           // ← 立即让 UI 切 LaunchingView
            await stateMachine.triggerColdStart() // ← 触发 bootstrap 重跑
            throw APIError.unauthorized           // ← 仍 throw，让 caller 知道这次失败
        }
    }
}
```

**关键约束**：
- 只对 `endpoint.requiresAuth == true` 的请求触发 cold-start（避免 `/auth/guest-login` 自身 401 自救悖论）
- 装饰器持 `sessionStore` + `stateMachine` 引用 —— 由 `AppContainer` 装配时注入
- `triggerColdStart()` 是 `AppLaunchStateMachine` 新增入口（与 `retry()` 类似但带 session.clear 副作用）
- 装饰器**不**做 in-flight 协调（不复刻退役的 generation snapshot）—— 多请求并发触发 cold-start 时由 `stateMachine.triggerColdStart` 内部 idempotent flag 保护（与 `isRetrying` 同模式）

详见 Story `0008-impl-1-砍-silent-relogin.md`。

### 6.4 用户感知

- 失败路径：从主屏闪一下 LaunchingView（< 1 秒）→ 回到主屏
- **没有错误弹窗、没有手动操作**
- 用户数据不丢失（guestUid 在 keychain 持久化，server 端 `auth_service.go#GuestLoginOrReuse` 行为已锁定见 lesson 2026-04-26-multi-table-tx）

### 6.5 错误源识别（保留 case 拆分，但不再服务 silent relogin）

`APIError.unauthorized` 仍严格等于"server 拒绝当前 token"；本地态仍保留细分：
- `.missingCredentials` —— 本地确认无 token（含 DI 没配 keychain）
- `.localStoreFailure(underlying:)` —— keychain 抛错（sandbox / OSStatus 抽风）

但**消费侧变了**：
- `.unauthorized` → §6.2 cold-start 路径
- `.missingCredentials` → §6.2 cold-start 路径（cold-start 内会 throw 同 case 进 alert）
- `.localStoreFailure` → mapper `.retry`（保留 §4.2 表里的判定，因为 keychain transient 抽风 retry 可能恢复）

避免 `try?` 反模式（见 §8.5）的 case 设计仍有效。

### 6.6 实施 Story（待起）

新建 Story `epic-cleanup/砍 silent relogin` 列出：
1. 新增 401 全局 catch 实装（位置见 §13.1）
2. 删除 3 个核心文件 + 配套测试
3. 简化 `APIClient` 调用链（移除 `AuthRetryingAPIClient` 装饰器）
4. 测试矩阵：401 → cold-start → 新数据
5. 回归测试：现有 happy path + cold-start path 不退化

---

## 7. SessionStore 一致性契约

### 7.1 单一 source of truth

**钦定**：`SessionStore.session?.user.nickname` 是 UI 层身份的 single source of truth。

| 组件 | 责任 |
|---|---|
| `KeychainTokenStore` | 持久层身份载体 |
| `SessionStore` (`@Published session: SessionState?`) | UI 层 single source of truth |
| `HomeView` / `SessionAwareUserInfoBar` | 通过 `@ObservedObject` 订阅 |
| `HomeNicknameResolver` (pure enum static) | 决策纯函数 |
| `HomeViewModel.nickname` | **fallback 值** `"用户1001"` |
| `RootView.bootstrapStep1` | login 写入侧协调器 |
| `ResetIdentityViewModel` | 清除侧协调器（fail-open） |

### 7.2 跨 store 三向审计 checklist（每次新增 in-memory mirror）

- **写入侧**：login / refresh / **§6.2 cold-start 后重建（替代 silent relogin 写入）**
- **清除侧**：reset / logout / **§6.2 401 触发的 cold-start 前清空** / token expired
- **失败回退**：fail-open（keychain 没清成功就保留 session）
- **隐性 AC**：至少一个 view 订阅 store.session 并展示某字段

未来 `RoomStore` / `StepStore` / `ChestStore` 引入时强制走相同三向审计。

---

## 8. 反模式登记（**保留所有条目**作为历史警示）

§8.19 / §8.20 / §8.21 是 silent relogin 系列的反模式，**虽然 silent relogin 已退役但反模式登记保留** —— 未来若重新引入并发协调器，仍能受益于这些教训。

### 8.1 单点 patch 反模式（核心 / 元反模式）

- **触发**：fix-review 多轮迭代同一议题时，每轮只改 review framing 框定的那一条 case
- **后果**：每轮都引入新 regression（5-5 round 1→4 全部命中）
- **元规则**：≥3 轮起每轮必须出全量行为表 / 全量分类表

### 8.2 过度悲观 force-quit

- **触发**："按 APIError case 名硬绑 terminal" 思路
- **后果**：transient 子集被误伤为 force-quit
- **反例**：round 5 钦定 `.unauthorized` → `.alert`；round 8 钦定 `.decoding` → `.alert`

### 8.3 fallback 当 "non-classification dump"

- **触发**：把 fallback 想成"不知道怎么分类的 dump"而非"分类决策点"
- **反例**：round 10 抓到 mapper non-APIError fallback 仍 `.alert`

### 8.4 `.alert` 默认安全选择

- **触发**："反正让 user 重启总没错"心智
- **后果**：在 force-quit-only 渲染下这恰是激进选择

### 8.5 `try?` / `catch ignore` lossy bridge

- **触发**：用 `try?` 把"抛错"降级 nil
- **后果**：下游 mapper 永远 lossy，分类不可救
- **反例**：round 11 抓到 `APIClient.buildURLRequest` 用 `try?` collapse 两种语义
- **规则**：任何 `try?` / `catch _` 处都要标注"这两路语义在下游是否需要区分"

### 8.6 modal onDismiss no-op

- **触发**：想"让 modal 保留"就把 onDismiss 写空 → view 永不卸载死锁
- **正解**：view 层不渲染按钮，而非按钮存在但按了不动

### 8.7 alert dismiss 隐式 retry

- **触发**：`AlertOverlayView.onDismiss` 接 retry，但文案是 terminal-class
- **后果**：文案与行为矛盾；用户死循环

### 8.8 exit(0) / abort() / fatalError() 强制退出（iOS HIG 反模式）

- **触发**：iOS app UI 路径调 `exit(0)`
- **后果**：iOS HIG 禁止；App Store 拒审；user 感觉崩溃
- **跨平台注意**：Swift 跨平台时分平台判断（CLI 合理 / iOS UI **永远**反模式）

### 8.9 user-driven retry + 文案提示（伪 user choice）

- **元根因**：`AlertOverlayView` 是 dismiss-able overlay × terminal error 本身就是伪命题

### 8.10 嵌套 optional 链塌陷两种 nil 语义

- **触发**：`a?.b?.c ?? placeholder`，内层 optional 是 schema 允许的合法值
- **修复模板**：抽纯函数 helper 三态分支显式列出
- **跨语言通用**：Go / TypeScript 同样适用

### 8.11 删除调用 + 加短路双重防御

- **正例**：要么完全删除入口，要么保持调用但挪位置 —— 单选其一

### 8.12 `error.localizedDescription` 直接漏到 UI

- **后果**：developer copy（`"Network error: ..."`）漏到用户看
- **审计触发器**：grep `RetryView` / `AlertOverlay` / `Toast` / `.needsAuth(message:`

### 8.13 isEmpty 兜底反模式

- **正解**：`as? LocalizedError + errorDescription Optional 解包`

### 8.14 queue 不携 callback

- **规则**：队列元素 = 立即展示路径所有输入的复合 tuple/struct（presentation + callback）

### 8.15 注释替代行为反模式

- **正解**：要么从 API 删除该能力（编译期可见），要么实装到位

### 8.16 文案表跨组件 drift

- **反例**：`AuthRetryingAPIClient`（Story 5.4）上线反转 `.unauthorized` 语义但 mapper 文案没同步
- **规则**：decorator/wrapper PR 必须把跨 module 文案审计列为 explicit AC checklist
- **注**：silent relogin 退役后此具体场景消失，但反模式本身仍登记（未来引入 decorator 时仍适用）

### 8.17 silent fallback（schema drift）

- **触发**：`enum.init(rawValue: x) ?? .first`
- **正解**：frozen schema 必须抛 `APIError.decoding`；evolving schema 必须显式定义 `.unknown` case

### 8.18 `#if DEBUG assertionFailure(); fallback in production`

- **后果**：prod 仍是 silent，本质仍是 silent fallback 变体

### ~~8.19 inFlight 清理绑定 caller 生命周期~~ （历史 / silent relogin 退役后不再触发，但记录保留）

- **触发**：`defer { inFlight = nil }` 写在 caller 函数体
- **后果**：清理时机绑定 caller 而非资源本身；future Swift 概率破裂
- **保留理由**：未来若引入 single-flight 协调器（如 token refresh / 资源池）仍适用

### ~~8.20 generation snapshot 不连带清空 cached result~~ （历史）

- **触发**：失败 finish 只清 inFlight 不清 lastIssuedToken
- **保留理由**：未来若有 cache + invalidation 模式仍适用

### ~~8.21 本地无凭证误走 relogin~~ （历史 / 但 case 拆分本身保留 —— 见 §6.5）

- **保留**：`.missingCredentials` / `.localStoreFailure` case 拆分本身仍有效（§6.5），避免 `try?` 反模式 8.5

### 8.22 Reset 后 session 残留

- **正解**：在 ViewModel 协调层调 `sessionStore?.clear()`（不在 Sendable struct UseCase 持有 SessionStore）

### 8.23 SessionStore 写入但不订阅

- **正解**：分发到子 view，子 view 用 `@ObservedObject var sessionStore: SessionStore`（非 Optional）

### 8.24 中间件直接写 envelope（绕过单一生产者）

- **review 视角**：PR 里出现 `response.Error(c, ...)` 调用点且不在 ErrorMappingMiddleware 内部 → 直接打回

### 8.25 中间件各自从 c.Errors 推断同一决策

- **正解**：`c.Get(ResponseErrorCodeKey)` 读 canonical key

### 8.26 跳过分支也 Set decision key

- **判则**：**key 的存在是肯定，key 的缺失也是信息**

---

## 9. 约束记录

### 9.1 关键文件路径（v2 后）

**保留**：
- `iphone/PetApp/App/RootView.swift`
- `iphone/PetApp/App/AppLaunchState.swift`
- `iphone/PetApp/App/AppLaunchStateMachine.swift`
- `iphone/PetApp/Shared/ErrorHandling/AppErrorMapper.swift`（v2 后预期 ~200 行，砍 ~80 行迭代史注释）
- `iphone/PetApp/Shared/ErrorHandling/ErrorPresenter.swift`
- `iphone/PetApp/Shared/ErrorHandling/ErrorPresentation.swift`
- `iphone/PetApp/Core/DesignSystem/Components/TerminalErrorView.swift`
- `iphone/PetApp/Core/Networking/APIClient.swift`（v2 后简化，去除 `AuthRetryingAPIClient` 装饰器层）
- `iphone/PetApp/Features/Home/Models/HomeData.swift`
- `iphone/PetApp/Features/Home/Views/HomeView.swift`
- `iphone/PetApp/Features/Auth/Session/SessionStore.swift`
- `server/internal/app/http/middleware/error_mapping.go`
- `server/internal/app/http/middleware/logging.go`

**退役**（§6 实施 Story 内删除）：
- `iphone/PetApp/Core/Networking/AuthRetryingAPIClient.swift`
- `iphone/PetApp/Features/Auth/UseCases/SilentReloginCoordinator.swift`
- `iphone/PetApp/Features/Auth/UseCases/SilentReloginUseCase.swift`

### 9.2 保留的回归测试

- `ErrorComponentSnapshotTests.testTerminalErrorViewHoldsTitleAndMessageOnly`（编译期契约）
- `HomeNicknameResolverTests`（happy / fallback / 空字符串 edge）
- `ResetIdentityViewModelTests`（happy / error fail-open / 老 init 兼容 / container wiring）
- `TestRouter_DevOnlyMiddleware_FallbackPath_LogsCanonicalErrorCode`（server canonical key 链路）

### 9.3 退役的回归测试

- `SilentReloginCoordinatorTests` 全套
- `AuthRetryingAPIClientTests` 全套
- `SilentReloginUseCaseTests` 全套
- `MockSilentReloginUseCase`

### 9.4 跨组件审计触发器

每改 mapper / 引入 decorator / 引入 in-memory mirror，必须 grep 全 PetAppTests：`code: <N>` / case 名硬编码断言 / `RetryView` / `AlertOverlay` / `Toast` / `.needsAuth(message:` / `.failed(message:`

---

## 10. 元方法论反思

### 10.1 五轮 fix-review 元根因复盘（r8 钦定）

**触发信号**：同一具体 fix 在 review 中**反复换方案仍被推翻 ≥3 次**；fix 在固定选项之间循环切换；review 文件名带递增后缀。

**alert dismiss 5 轮迭代切换史**：

| Round | Dismiss 行为 | Review 结论 |
|---|---|---|
| 0 | 默认 → 自动 retry | P2: 隐式重试 |
| r3 | alert dismiss 仍 retry | P1: 死循环 |
| r4 | alert dismiss 改 no-op | P2: 卡死 |
| r5 | alert dismiss 改 `exit(0)` | P1: iOS HIG 反模式 |
| r7 | alert dismiss → retry + 文案补"持续失败时请杀进程" | P1: 仍困 retry/fail loop |
| **r8（终极）** | 引入 `TerminalErrorView` 全屏静态 page（无任何按钮） | 收敛 |

**元根因**："dismiss-able overlay × terminal error" 本身就是个伪命题。

**钦定元规则**：
- ≥3 轮被否，**必须停下来**列穷举决策表格
- 所有选项都试过 → **重新审视上一层假设**
- **优先**引入新 type / 新 component / 新 UI mode（如 `TerminalErrorView`），**而非**在旧 type 上加参数 / case / closure
- 错的 framing："X 组件的 Y 行为该选 A/B/C/D 哪个？" 对的 framing："这个组件 / API / UI mode 是否适合当前问题？"

### 10.2 边界 lossy projection 通用规则

修一个错误时，留意"我引入的 wrapper / 新边界"是否会丢失下游需要的语义维度。检查清单：
1. 这个 fallback 后面的字符串/值在**哪些**输入下会被使用？两种输入的业务语义一样吗？
2. 这个 helper 函数的输出会**漏到**几条不同路径？这些路径对"什么是合理输出"的要求一样吗？

任一答案"不一样" → **不**能共用 fallback / helper，必须显式分支。

### 10.3 30 秒 self-review

每次 fix-review 修完后：
1. 这次修改**新引入**的代码路径在哪些场景会触发？覆盖了 sad path 吗？
2. 这次修改**删除/短路**的代码路径，原本承担什么角色？是否还有其他地方依赖该角色？
3. 这次修改影响了什么**契约**（接口 / spec / 性能 NFR）？是否所有契约都仍满足？

### 10.4 三层贯彻清单

每次钦定 / 迭代通用判则，必须三层贯彻清单审计：
1. **显式 case** —— 已经在判则里命名的 case
2. **fallback / unknown 分支** —— 没命名但分类决策仍生效的 case
3. **case 自身设计的源头** —— 上游边界（wire DTO / ErrorMapping / try? bridging）是否已经在该层 lossy 化

### 10.5 lesson 沉淀机制本身的批判（采用 codex §5）

> **"lesson 机制是有效但危险。短期有用，长期如果不把高频 lesson 反向沉淀成类型、接口、测试模板、lint 规则、代码生成模板，它就会变成'把架构债写成文档'的高级形式。"**

本 ADR 退役 silent relogin 的同时把 4-6 条相关 lesson 标 superseded —— 这是 lesson → ADR 蒸馏的第一次实践。未来若 lesson 累积速度 > 蒸馏速度（当前 22/天 vs 0/月），必须主动暂停 dev-story 节奏来追蒸馏债。

---

## 11. Consequences

### 11.1 实装路径

1. **阶段 1（本 ADR Accepted 前）**：用户拍板 §13 Open Questions 剩余 6 项
2. **阶段 2（退役 silent relogin）**：起 Story `epic-cleanup/砍 silent relogin`（详见 §6.6）
3. **阶段 3（lesson 标 superseded）**：批量更新 `docs/lessons/index.md` + 受影响 lesson 文件头加 "**superseded by ADR-0008**" 注记

### 11.2 对未来 Story 的影响

- **Epic 6+ 业务接口落地**：`AppErrorMapper.transientBusinessCodes` 是 client 端硬编码集合；新增码必须显式审视归类（不延迟到协议层）
- **Epic 7 步数 / Epic 11 房间 / Epic 20 宝箱**：引入 in-memory mirror 必须走 §7.2 三向审计 checklist
- **任何 401 处理路径**：统一走 §6.2 cold-start，不再引入新的 `AuthRetrying*` 装饰器

### 11.3 对 mapper 的预期

- v2 后 `AppErrorMapper.swift` 预期从 286 → ~200 行（砍 ~80 行 silent relogin 相关迭代史注释）
- 如果 v2 实施后 mapper 仍臃肿，下一步**不是**继续加注释，而是把 case 设计往上游推（让 APIClient 直接产出更少 case）

---

## 12. 不在本 ADR 范围

- 具体的 V1 文档同步 PR
- `RootView` / `AppLaunchStateMachine` 的"知道太多错误史"重构（§5.4 反思 → §13 Open Question）
- watch / 旧 `ios/` 目录的错误协议
- post-MVP 重新引入 silent relogin 时的 OAuth refresh token 设计

---

## 13. Open Questions（**待用户拍板**）

### 13.1 401 全局 catch 位置 —— **Resolved: A**（2026-04-29）

**拍板**：A. `APIClient` 装饰器层（新建 `AuthBoundaryAPIClient`）

**否决理由**：
- B（mapper + RootView flag）：直接违反本 ADR §2.3 钦定 "不通过加固 mapper centrality 来解决问题" —— mapper 又多一个 cold-start 触发职责
- C（AppCoordinator + 每个 ViewModel 显式调）：复刻 §8.1 单点 patch 反模式 + §1.2 根因 2 "恢复责任没有被单点定义" —— 漏一个 ViewModel = silent failure 不会编译期发现

**A 的实现形态**：
```swift
final class AuthBoundaryAPIClient: APIClientProtocol {
    private let inner: APIClientProtocol
    private let sessionStore: SessionStore
    private let stateMachine: AppLaunchStateMachine

    func request<T>(_ endpoint: Endpoint) async throws -> T {
        do {
            return try await inner.request(endpoint)
        } catch APIError.unauthorized where endpoint.requiresAuth {
            await sessionStore.clear()
            await stateMachine.triggerColdStart()
            throw APIError.unauthorized  // 仍 throw，让 caller 知道这次失败
        }
    }
}
```

预计 ~50 行新文件 + `AppLaunchStateMachine` 新增 `triggerColdStart()` 入口 ~10 行 = 净增 ~60 行；退役 ~800 行 = **净减 ~740 行**。详见 Story `0008-impl-1-砍-silent-relogin.md`。

### 13.2 client 端 mapper 对未知码的默认 fallback

未知业务码（server 加新码 + client 旧版）默认归 transient 还是 terminal？
- transient 默认：与"误判代价不对称"原则一致（多发一次请求 < force-quit）
- terminal 默认：未知码视为"client 落后"

### 13.3 `RootView` / `AppLaunchStateMachine` 的"知道太多"重构

§5.4 反思指出两个架构味道；是否纳入下一个 epic-cleanup？

### 13.4 mapper 改动的最小测试矩阵 SOP

是否在本 ADR 加一节"mapper 改动 SOP"（固定 N 个 case × 2 路径 = 2N 条断言模板），还是另开 ADR？

### 13.5 OpSec 外观仿真层修复路径

DevOnlyMiddleware 修复后 HTTP status 404 → 200，扫描器仍可识别 dev 路由。是否纳入本 ADR 还是另起 spike？

### 13.6 lesson 蒸馏后的源 lesson 处理

候选：
- 删除（git history 仍可查）
- 归档到 `docs/lessons/superseded/`
- 仅 index 标 superseded（lesson 仍在原地，最低破坏性）

### 13.7 ~~RetrySemantic 协议传递形态~~ ~~silent relogin 是否保留~~ ~~generation 机制是否过度工程化~~

**已拍板** —— 见 v2 §2.1（heuristic 不升级协议）+ §6（silent relogin 砍掉）。

---

## 14. Change Log

| Date | Change | By |
|---|---|---|
| 2026-04-28 | 初稿 v1：从 22 条 lesson 蒸馏；钦定主旨"transient/terminal 是协议层概念" + Open Questions | Claude |
| 2026-04-29 | v2 修订：codex 独立 review 反对主旨 → 降级为 presentation heuristic；用户拍板砍掉 silent relogin → 删 §6 actor 双字段 invariant + 新增 §6 cold-start 替代路径；采用 codex 5 层根因 / mapper 反思 / lesson 沉淀批判 | Claude（用户委托 + codex review） |
| 2026-04-29 | §13.1 Resolved: A（AuthBoundaryAPIClient 装饰器层全局 catch） | Claude |
| 2026-04-29 | Story 0008-impl-1 实施完成（Windows 静态检查通过）：新建 AuthBoundaryAPIClient.swift（92 行）+ UnauthorizedHandlerSink + AppLaunchStateMachine.triggerColdStart() + AuthBoundaryAPIClientTests（8 case）；删除 AuthRetryingAPIClient / SilentReloginCoordinator / SilentReloginUseCase 三件套 + 4 配套测试文件（净减 ~600 行）；AppContainer 装配重写；RootView wire late-bind handler；AppErrorMapper 286 → 170 行（砍 116 行迭代史 + 退役引用） | Claude（用户委托） |
| 2026-04-29 | Mac 验证修复：3 个测试漏改（AppErrorMapperTests / AppLaunchStateMachineTests 仍按旧 round 9 `.unauthorized → .retry` 预期，应改 `.alert` 兜底）+ xcodegen regenerate pbxproj 剔除退役文件引用；`bash iphone/scripts/build.sh --test` 236/236 全绿 → Status: **Accepted** | Claude（Mac 端用户验证） |
