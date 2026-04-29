# ADR-0008: 错误协议与错误恢复边界（Codex 独立草稿）

- **Status**: Draft
- **Date**: 2026-04-29
- **Decider**: Codex
- **Supersedes**: N/A
- **Related Stories**: 1.8, 2.6, 2.8, 2.9, 5.2, 5.4, 5.5

---

## 1. Context

### 1.1 输入材料与边界

- 本草稿只基于以下材料：
  - `CLAUDE.md`
  - `docs/宠物互动App_V1接口设计.md` §3
  - `_bmad-output/implementation-artifacts/decisions/0006-error-handling.md`
  - `docs/lessons/` 指定 22 条 lesson
  - 当前 iOS 核心实现：`AppErrorMapper.swift`、`AppLaunchStateMachine.swift`、`RootView.swift`、`AuthRetryingAPIClient.swift`、`SilentReloginCoordinator.swift`
- 明确未读取：现有 `_bmad-output/implementation-artifacts/decisions/0008-error-protocol.md`

### 1.2 现状摘要

- server 侧 ADR-0006 已经把“32 个业务码 + ErrorMappingMiddleware 单一产出 envelope”定成 Accepted。
- iOS 侧没有同等级的稳定错误协议，只有一组在 Story 5.4 / 5.5 的 11 轮 fix-review 中被反复修补出来的局部规则。
- 当前代码把大量错误策略直接写进注释，尤其 `AppErrorMapper.swift` 与 `RootView.swift` 已接近“判例集 + 迭代史”，说明规则没有沉淀成足够强的类型边界。

### 1.3 迭代史必须保留

- Story 5.5 共 11 轮 fix-review。
- 其中 bootstrap `.alert` dismiss 行为单点翻盘 5 轮：
  - round 0：默认自动 retry
  - round 3：alert dismiss 仍 retry
  - round 4：改 no-op
  - round 5：改 `exit(0)`
  - round 7：改 user-driven retry + 文案指引
  - round 8：最终放弃 dismiss-able overlay，改静态 `TerminalErrorView`
- 这不是实现细节噪音，而是协议语义不稳定的证据。

### 1.4 本 ADR 的工作假设

- 本文不把“transient / terminal 二分”直接视为已拍板真理。
- 本文只承认它是当前 iOS 呈现层从 11 轮修补中提炼出来的**工作性启发式**。
- 如果该二分与协议层、恢复层、产品层的真实状态空间不一致，本 ADR 要保留分歧，而不是替它补一层神学正当性。

---

## 2. Problem

### 2.1 22 条 lesson 共同暴露的不是一个 bug，而是四类系统性错位

- **错位 1：语义在边界被压扁**
  - `ErrorPresentation` 被压成 `String`
  - local-store throw 与 local-store empty 被压进同一 `.missingCredentials`
  - Logging 自己从 `c.Errors` 推断 canonical decision
  - DTO 未知 enum 被 silent fallback 吃掉
- **错位 2：恢复责任边界不清**
  - server 401 与本地缺凭证被错误放进同一 silent relogin
  - bootstrap 错误路径有的经 mapper，有的漏 developer 串
  - reset 只清持久层不清内存镜像
- **错位 3：UI mode 与错误语义不匹配**
  - dismiss-able overlay 被拿来承载 terminal bootstrap error
  - toast/retry/alert 的语义被下游 patch 不断反转
- **错位 4：规则靠注释和 lesson 维持，不靠类型和编译期约束维持**
  - `AppErrorMapper.swift` 大量 round-by-round 注释
  - `RootView.swift` 直接内嵌 dismiss 行为迭代史
  - 说明规则没有被压进更硬的接口

### 2.2 当前架构的真正风险

- 风险不只是“错误文案不统一”。
- 真风险是：
  - 同一错误在不同层被赋予不同恢复语义
  - 同一语义在不同 UI mode 中被迫用不同补丁实现
  - 同一个判则改对一层后，旁边一层继续按旧世界工作
- 这正是 Story 5.5 11 轮 fix-review 的结构性原因。

---

## 3. Draft Decision

### 3.1 暂定方向

- **D1. server 继续坚持“错误 envelope 单一生产者”**
  - `ErrorMappingMiddleware` 是 canonical producer。
  - 任何 middleware / handler 直接写 `response.Error(...)` 都是反模式。
- **D2. iOS 侧禁止在错误边界做 lossy projection**
  - 错误类型必须保留恢复所需语义。
  - state machine 必须携带 `ErrorPresentation` 这类完整决策，不再退化成 `message`。
- **D3. “transient / terminal” 暂定为 presentation heuristic，不升级为协议真理**
  - 当前只承认它指导 `.retry` vs `.alert` / `TerminalErrorView`。
  - 不承认它已经等于 server/client 错误协议的唯一主轴。
- **D4. bootstrap terminal error 不再使用 dismiss-able overlay**
  - 如果保留 terminal 概念，bootstrap 路径只能走静态 fallback page。
- **D5. silent relogin 若保留，必须建立更硬的不变量**
  - 区分 local vs server unauthorized
  - spawned task 生命周期绑定清理
  - generation snapshot dedup
  - 失败清缓存
  - reset 同步清内存 session

### 3.2 本文刻意不拍板的部分

- 不拍板“transient / terminal 应不应该进 APIError / server 错误码元数据”。
- 不拍板 silent relogin 是否值得继续保留。
- 不拍板 `.missingCredentials` 是否真是“重启也救不了”的 terminal，而不是“应回 cold-start guest login”的 product flow 问题。

---

## 4. Anti-Pattern Register

### 4.1 `2026-04-27-business-error-transient-vs-terminal`

- 反模式名：**所有 business 一律 alert**
- 具体踩坑：
  - 1009 服务繁忙被卡进 alert，无 RetryView 入口
  - 1005/1007/1008/1009 这类“可重试自愈”的业务码被强制当 terminal
- 迭代史：
  - round 5 才明确把 1005/1007/1008/1009 列为 transient business codes
- 保留结论：
  - 这是当前最实用的 UI 启发式
- 保留分歧：
  - 它仍可能只是现阶段局部最优，不等于协议层正确维度

### 4.2 `2026-04-28-decoding-and-unauthorized-must-be-transient-retry`

- 反模式名：**按 APIError case 名字直觉硬绑 presentation**
- 具体踩坑：
  - `.unauthorized` exhausted 后被当绝对 terminal
  - `.decoding` 被当 schema drift 必然 terminal
  - bootstrap 路径进 `TerminalErrorView`，用户只有 force-quit
- 迭代史：
  - round 9 把 `.unauthorized` / `.decoding` 从 `.alert` 翻到 `.retry`
- 本 ADR 的判断：
  - 这说明“case 名字”不是恢复语义
  - 也说明 exhausted decorator 不等于 product-level terminal

### 4.3 `2026-04-28-non-api-error-fallback-must-be-transient-retry`

- 反模式名：**fallback 分支漂出主判则**
- 具体踩坑：
  - non-APIError fallback 一直是 `.alert("操作失败，请稍后重试")`
  - bootstrap 遇到 KeychainError 时被送进 TerminalErrorView
- 迭代史：
  - round 10 才把 fallback 改成 `.retry("操作失败，请重试")`
- 关键陷阱：
  - fallback 也是分类决策点，不是“安全 dump”

### 4.4 `2026-04-28-local-store-transient-vs-terminal-must-distinguish`

- 反模式名：**try? bridging 把本地抛错与本地空值压进一个 case**
- 具体踩坑：
  - `buildURLRequest` 用 `try?` 读 keychain
  - keychain 抛错与 keychain 读成功但 nil/空串都变成 `.missingCredentials`
  - mapper 再严谨也无法恢复丢失语义
- 修补结果：
  - round 11 新增 `.localStoreFailure`
  - `.missingCredentials` 收窄到“本地确认无 token”
- 这是本 ADR 认为最重要的 lesson 之一：
  - 说明 mapper 不是信息恢复器
  - 必须先修 case 设计，再谈 presentation 判则

### 4.5 `2026-04-27-bootstrap-all-error-paths-route-via-mapper`

- 反模式名：**bootstrap closure 内只修一条 throw 路径**
- 具体踩坑：
  - LoadHome 失败经 mapper
  - GuestLogin 失败漏包装，直接把 raw APIError 扔给状态机
  - 用户看到 developer-facing 字符串
- 约束：
  - bootstrap 抛给状态机的任何错误都必须先变成 `BootstrapMappedError`

### 4.6 `2026-04-27-bootstrap-alert-dismiss-must-be-user-driven-recovery`

- 反模式名：**把 terminal bootstrap error 放进 dismiss-able overlay 后，在 retry / no-op / exit 之间乱选**
- 具体踩坑：
  - no-op 死锁
  - `exit(0)` 违反 iOS HIG
  - dismiss→retry 与“请重启应用”文案冲突
- 迭代史：
  - 这一点翻了 4 轮以上
- 本 ADR 结论：
  - 它最终被 round 8 的静态 fallback page 推翻
  - 这条 lesson 重要，但它后来被更高层 framing 否决

### 4.7 `2026-04-27-bootstrap-terminal-error-static-fallback-page`

- 反模式名：**dismiss-able overlay × terminal bootstrap error**
- 具体踩坑：
  - retry/no-op/exit/user-driven retry 全部被证明不自洽
  - `TerminalErrorView` 通过“无按钮”把错误语义硬编码到 UI mode
- 关键陷阱：
  - 这不是一个 closure 怎么写的问题
  - 是选错 UI mode

### 4.8 `2026-04-27-bootstrap-error-and-optional-pet-must-route-via-mapper`

- 反模式名 1：**bootstrap 错误绕过 mapper**
- 反模式名 2：**`optional?.optional?.field ?? placeholder` 把 loading 与 server 明确无值压成一类**
- 具体踩坑：
  - `pet: null` 被渲染成“默认小猫”
  - developer copy 经 `errorDescription` 泄漏到 RetryView
- 特别保留：
  - “写一个 `??` 很省事” 本身就是架构味道，不只是 UI 小 bug

### 4.9 `2026-04-27-cold-start-http-budget-and-bootstrap-retry-fail-safe`

- 反模式名 1：**HTTP 预算只盯新请求，不清旧请求**
- 反模式名 2：**错误恢复路径上用“曾经成功”短路 token 协商**
- 具体踩坑：
  - `/ping` 让 cold-start 超过 2 HTTP budget
  - `GuestLoginCompletionGate` 让 retry 跳过重新 auth，卡死在坏 token
- 本 ADR 强调：
  - retry 路径里的 auth 重做，不是浪费；它就是恢复机制

### 4.10 `2026-04-27-launch-state-machine-must-carry-presentation`

- 反模式名：**state machine 只携带 `message: String`**
- 具体踩坑：
  - mapper 已做 `.alert` / `.retry` 决策
  - 状态机把它降成字符串
  - RootView 只能一律渲染 RetryView
- 这是第二个关键 lesson：
  - state 不承载完整 UI 决策语义，就会逼 view 层猜

### 4.11 `2026-04-27-retry-decorator-changes-unauthorized-presentation-semantics`

- 反模式名：**引入 decorator 后不审计 user-visible mapping**
- 具体踩坑：
  - `.unauthorized` 以前等于“后台正在重登”
  - 引入 `AuthRetryingAPIClient` 后，能冒泡上来的 `.unauthorized` 已经是“静默自救失败”
  - toast 文案继续写“正在重新登录...”
- 这个 lesson 的价值：
  - 装饰器不是纯网络层重构，它会反转上层语义

### 4.12 `2026-04-27-home-data-fail-fast-on-unknown-enum`

- 反模式名：**frozen schema 的 enum 解码 silent fallback**
- 具体踩坑：
  - `HomePetState(rawValue:) ?? .rest`
  - `HomeChestStatus(rawValue:) ?? .counting`
  - schema drift 被静默吃掉
- 当前修法：
  - fail-fast 抛 `.decoding(underlying: HomeDataDecodingError)`
- 关键陷阱：
  - forward-compatible 与 frozen-schema 不能混用一套 fallback 心智

### 4.13 `2026-04-26-error-presenter-queue-onretry-loss`

- 反模式名：**队列只存 presentation，不存 callback**
- 具体踩坑：
  - `.retry` 入队时丢 `onRetry`
  - 用户点“重试”只 dismiss，不重发请求
- 这是纯结构性 bug：
  - 队列元素必须是 `(presentation, callback)` 复合单元

### 4.14 `2026-04-26-error-localizeddescription-system-fallback`

- 反模式名：**把 `error.localizedDescription.isEmpty` 当 fallback 判断**
- 具体踩坑：
  - 非 `LocalizedError` 会 bridge 成 NSError 系统串
  - `isEmpty` 分支永远不走
  - 用户看到英文系统实现细节
- 保留教训：
  - Swift `localizedDescription` 默认值不是“空”，而是“错误地有内容”

### 4.15 `2026-04-27-actor-coalesce-cleanup-must-bind-resource-not-caller`

- 反模式名：**single-flight 的清理绑定 caller defer，不绑定 spawned task 生命周期**
- 具体踩坑：
  - `defer { inFlight = nil }`
  - caller 与资源生命周期错位
- 特别保留：
  - 即使 reviewer 给出的具体 cancellation 触发链未必精确，底层 smell 仍成立

### 4.16 `2026-04-27-actor-coalesce-failure-must-clear-cached-token`

- 反模式名：**coalesce 失败只清 inFlight，不清 cached token**
- 具体踩坑：
  - “成功一次 → 再失败一次 → stale caller 进入” 时，旧 token 被 generation 短路复用
  - caller 错失真正 relogin
- 关键陷阱：
  - inFlight 与 lastIssuedToken 之间有隐含 invariant
  - 失败路径不维护 invariant，就会把旧成功结果漂进未来调用

### 4.17 `2026-04-27-silent-relogin-must-distinguish-local-vs-server-unauthorized`

- 反模式名：**同一个 `.unauthorized` case 混合“请求未发出”和“server 已拒绝”**
- 具体踩坑：
  - 本地无 token / keychain 配置错被错误送进 silent relogin
  - reset 语义被偷偷恢复
  - DI 配置错被静默掩盖
- 修补结果：
  - `.missingCredentials` 从 `.unauthorized` 拆出
- 本 ADR 判断：
  - 这是 silent relogin 能否存在的前提条件，不是优化项

### 4.18 `2026-04-27-silent-relogin-stale-401-needs-generation-dedup`

- 反模式名：**只靠 inFlight，拦不住 post-refresh stale caller**
- 具体踩坑：
  - A 已成功刷新并清空 inFlight
  - B 基于旧 token 发出的 401 晚到
  - B 进入 relogin 时误触第二次 guest-login
- 修补结果：
  - `currentGeneration()` + `callerGeneration` snapshot + `lastIssuedToken`
- 特别保留：
  - snapshot 必须在 `inner.request` 之前取，不是 catch 内取

### 4.19 `2026-04-27-reset-identity-must-clear-in-memory-session`

- 反模式名：**reset 只清 Keychain，不清 SessionStore**
- 具体踩坑：
  - HomeView 继续显示 reset 前昵称
  - 直到 kill app 才消失
- 关键陷阱：
  - 引入 in-memory mirror 后，清理路径必须成对补齐

### 4.20 `2026-04-27-sessionstore-home-nickname-source-of-truth`

- 反模式名：**状态写入了，但视图没订阅**
- 具体踩坑：
  - `sessionStore.updateSession(...)` 已发生
  - HomeView 继续读 `HomeViewModel.nickname`
  - 用户看到“用户1001”而非真实身份
- 当前修法：
  - HomeView 通过子视图 `@ObservedObject` 订阅 `SessionStore`

### 4.21 `2026-04-24-error-envelope-single-producer`

- 反模式名：**中间件自己写 envelope，绕过 canonical 管道**
- 具体踩坑：
  - `DevOnlyMiddleware` 直接 `response.Error(...)`
  - 客户端看到 `code=1003`
  - Logging 读不到 `ResponseErrorCodeKey`
- 约束：
  - error envelope 只能由 `ErrorMappingMiddleware` 生产

### 4.22 `2026-04-24-middleware-canonical-decision-key`

- 反模式名：**多个 middleware 各自从原始状态推断同一 canonical decision**
- 具体踩坑：
  - non-AppError fallback：响应是 1009，日志无 `error_code`
  - double-write：响应成功，日志仍打 `error_code`
- 修补结果：
  - `ResponseErrorCodeKey`
- 本 ADR 判断：
  - 这条 lesson 是 server 侧最成熟的错误协议实践
  - iOS 侧反而还没有对应等级的 canonical channel

---

## 5. Current Architecture Reflection

### 5.1 server 侧

- 相对稳定。
- `ErrorMappingMiddleware` + `ResponseErrorCodeKey` 已接近真正的 protocol layer：
  - 单一生产者
  - 多消费者只读 canonical decision
  - double-write / non-AppError fallback 都有明确语义

### 5.2 iOS 侧

- 仍不稳定。
- 当前架构实际分成 5 层：
  - `APIError`
  - `AuthRetryingAPIClient` / `SilentReloginCoordinator`
  - `AppErrorMapper`
  - `AppLaunchStateMachine`
  - `RootView` UI mode dispatch
- 22 条 lesson 显示：
  - 这 5 层没有共享一个硬协议
  - 只是在用注释和 fix-review 追认彼此应该怎么配合

### 5.3 对 `AppErrorMapper` 的独立判断

- 现在的 `AppErrorMapper.swift` 已经承担了过多职责：
  - 文案表
  - 恢复分类器
  - 迭代史档案
  - lesson 索引
- 这不是“注释写得认真”，而是设计压力被挤压到了 mapper。
- 一旦规则靠 mapper 注释维持，说明上游类型系统还不够强。

### 5.4 对 `RootView` / `AppLaunchStateMachine` 的独立判断

- 当前 bootstrap 错误分发已经比前几轮稳定，但仍有两个架构味道：
  - `RootView` 知道太多错误史与恢复哲学
  - `AppLaunchStateMachine` 的 fallback 仍混杂了产品展示与兼容逻辑

### 5.5 对 silent relogin 的独立判断

- 当前实现已经把 single-flight、stale 401、失败清缓存、本地凭证缺失区分到了细节级别。
- 这在工程上是严谨的。
- 但它也暴露出另一个事实：
  - 为了保住 silent relogin，系统被迫新增 generation、snapshot、cached token invariant、dual local/server credential semantics。
- 这未必错，但成本已经明显高于“MVP 自然复杂度”。

---

## 6. Open Questions

- **Q1. `transient / terminal` 到底是协议层概念，还是当前 iOS 呈现层的局部启发式？**
- **Q2. `.missingCredentials` 真的是“terminal”，还是“应该直接切回 cold-start guest-login”的导航问题？**
- **Q3. bootstrap terminal error 的最终产品语义是什么？**
  - force-quit only
  - 回引导态
  - 清 session 后自动重建游客身份
- **Q4. silent relogin 是否值得保留到 MVP？**
  - 还是应直接改成“401 surface 到启动流 / 登录流重建身份”
- **Q5. business code 是否需要显式 retryability metadata？**
  - 当前靠 `transientBusinessCodes` 手工枚举
  - 这属于协议外推断，不是契约字段
- **Q6. iOS 是否需要一个类似 server `ResponseErrorCodeKey` 的 canonical error decision channel？**
  - 例如把“恢复语义”与“展示语义”从 mapper 注释迁移成更硬的结构
- **Q7. lesson 里的“TerminalErrorView = 终极方案”是否只是当时 fix-review 上下文下的终极方案，而不是产品终局？**

---

## 7. Consequences

### 7.1 如果沿当前方向继续推进

- 好处：
  - server 侧可继续稳定扩展
  - iOS 侧现有 bootstrap 恢复链路已有可用行为
  - 大部分已知 regression 有回归测试守护
- 代价：
  - iOS 错误协议继续依赖注释与 lesson 驯化
  - 新成员改 `AppErrorMapper` / `RootView` 风险极高
  - silent relogin 继续放大局部复杂度

### 7.2 如果后续决定重构

- 优先顺序不应是“继续补 mapper 注释”。
- 优先顺序应是：
  - 先确认 silent relogin 是否保留
  - 再确认 bootstrap terminal 的产品出口
  - 再考虑是否把 retryability / recoverability 元数据抬升到更硬的协议层

---

## 8. Change Log

| Date | Change | By |
|---|---|---|
| 2026-04-29 | 初稿：基于 22 条 lesson、ADR-0006、V1 错误码与当前 iOS 实现的独立蒸馏 | Codex |
