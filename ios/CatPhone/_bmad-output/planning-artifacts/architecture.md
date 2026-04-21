---
stepsCompleted: [1, 2, 3, 4, 5, 6, 7, 8]
lastStep: 8
status: 'complete'
completedAt: '2026-04-21'
inputDocuments:
  - /Users/zhuming/fork/catc/ios/CatPhone/_bmad-output/planning-artifacts/prd.md
  - /Users/zhuming/fork/catc/ios/CatPhone/_bmad-output/planning-artifacts/ux-design-specification.md
  - /Users/zhuming/fork/catc/ios/CatPhone/_bmad-output/planning-artifacts/server-handoff-ux-step10-2026-04-20.md
  - /Users/zhuming/fork/catc/ios/CatPhone/_bmad-output/planning-artifacts/implementation-readiness-report-2026-04-20.md
  - /Users/zhuming/fork/catc/ios/CatPhone/_bmad-output/planning-artifacts/sprint-change-proposal-2026-04-20.md
  - /Users/zhuming/fork/catc/ios/CatPhone/_bmad-output/planning-artifacts/validation-report-2026-04-21.md
  - /Users/zhuming/fork/catc/ios/CatPhone/_bmad-output/project-context.md
  - /Users/zhuming/fork/catc/CLAUDE.md
  - /Users/zhuming/fork/catc/docs/backend-architecture-guide.md
  - /Users/zhuming/fork/catc/docs/api/openapi.yaml
  - /Users/zhuming/fork/catc/docs/api/ws-message-registry.md
  - /Users/zhuming/fork/catc/docs/api/integration-mvp-client-guide.md
serverContractInvestigationDate: '2026-04-21'
serverRefsExamined:
  - /Users/zhuming/fork/catc/server/internal/ws/envelope.go
  - /Users/zhuming/fork/catc/server/internal/ws/dispatcher.go
  - /Users/zhuming/fork/catc/server/internal/ws/dedup.go
  - /Users/zhuming/fork/catc/server/internal/ws/hub.go
  - /Users/zhuming/fork/catc/server/internal/ws/upgrade_handler.go
  - /Users/zhuming/fork/catc/server/internal/dto/ws_messages.go
  - /Users/zhuming/fork/catc/server/internal/dto/error_codes.go
  - /Users/zhuming/fork/catc/server/internal/handler/platform_handler.go
  - /Users/zhuming/fork/catc/server/internal/handler/health_handler.go
  - /Users/zhuming/fork/catc/server/cmd/cat/initialize.go
  - /Users/zhuming/fork/catc/server/cmd/cat/wire.go
  - /Users/zhuming/fork/catc/server/config/default.toml
  - /Users/zhuming/fork/catc/server/docs/error-codes.md
workflowType: 'architecture'
project_name: 'CatPhone'
user_name: 'Developer'
date: '2026-04-21'
scope: 'iPhone 端（ios/CatPhone）'
---

# Architecture Decision Document — CatPhone (iPhone 端)

_本文档通过 step-by-step 协作逐步构建。各章节在对应 step 完成后追加。_

**范围**：仅 iPhone 端（`ios/CatPhone`）——Watch / server 为独立 repo 的架构决策不在本文档范围。

---

## Project Context Analysis

### Requirements Overview

**Functional Requirements**（PRD 63 条 FR + 35 个 Capability ID，9 类能力组）：

| 类目 | FR 范围 | 架构含义 |
|---|---|---|
| Onboarding（`C-ONB-*` + `C-UX-01/02` + `C-OPS-06`） | FR1–10a | SIWA / Watch 配对（**哲学 B**：Watch 独立走 server 账号层关联，**不**走 WCSession probe）/ HealthKit 延迟授权 / 撕包装 + 命名 / 账号删除导出 / **里程碑 server 权威**（`S-SRV-15 user_milestones`） |
| 好友（`C-SOC-03` + `C-UX-05`） | FR11–16 | 关系管理 / 静音 / 举报；标准 CRUD + WS 推送 |
| 房间 + 社交广播（`C-ROOM-*` + `C-SOC-01/02` + `C-ME-01`） | FR17a/b/c–FR30 | 创建 / 加入 / 邀请含失败分支 / iPhone 纯文字 `C-ROOM-02` 叙事流 / 观察者隐私 gate / **"我的猫"卡片 = 核心动作入口** / fire-and-forget emoji 广播 |
| 步数（`C-STEPS-*`） | FR31–34 | HealthKit 本地 SSoT（展示）/ server 权威 SSoT（解锁）/ 漂移窗口"已达成·等待确认"文案 |
| 经济：盲盒 + 合成 + 装扮（`C-BOX-*` + `C-SKIN-*` + `C-DRESS-*`） | FR35a/b/c/d–FR43 | server 权威 30min 计时 / 1000 步解锁 / **A5' Watch 不可达时 `unlocked_pending_reveal` 中间态** / 颜色分级 6 档 + 形状 + 文字标签 / 5→1 合成幂等 / `ClockProtocol` 测试抽象 / **iPhone 端禁 Spine SDK**（帧序列 PNG 或 Lottie） |
| 陪伴反馈：久坐召唤（`C-IDLE-*`） | FR44–46 | 猫动作同步（走 / 睡 / 挂机）· **无情绪机** · Watch 独占 haptic · server 权威 `last_active_ts = max(iPhone, Watch)` |
| 容错（`C-REC-*`） | FR47–50 | WS 重连 + `last_ack_seq` 回放 / TTL 24h fail-closed / `/v1/platform/ws-registry` 启动失败屏蔽主功能 / HealthKit 拒绝降级 — **⚠️ 部分假设与 server 实际漂移，见 Known Contract Drift** |
| UX 生存层（`C-UX-03/04`） | FR51–53 | 5 空状态 / identity 可编辑 / 设置入口 |
| 跨端 + 推送 + 隐私 + OPS（`C-SYNC-*` + `C-OPS-*`） | FR54–63 | `client_msg_id` UUID v4 60s 去重（**实际 server 300s**）/ 分频道推送 / wss + Keychain / 零 raw step 上传 / `Log.*` facade / `PrivacyInfo.xcprivacy` |

**Non-Functional Requirements**（8 类）：

- **Performance**：点猫弹选单 < 200ms / tab 切换 < 100ms / 冷启动 < 2s / 列表 > 200 行 LazyVStack / HealthKit 缓存 15min
- **Security**：wss + ATS / Keychain token / SIWA nonce / release 禁 IDFA + 明文 token / deep link 严校验
- **Reliability**：崩溃 ≤ 0.2% / WS 重连 10s ≥ 95% / TTL fail-closed / `/v1/platform/ws-registry` 启动失败屏蔽
- **Accessibility**：稀有度必配形状 + 文字 / Dynamic Type 全语义字号 / reduce motion 降级 / haptic 必配视觉兜底
- **Scalability**：单 WS 连接共享 envelope / 单房间 ≤ 4 人 / MVP 好友上限 50
- **Integration**：HealthKit SSoT 分工 / WatchConnectivity 降级为 fast-path 可选 / APNs / Spine（Watch）+ Lottie（iPhone）
- **Store Compliance**：`PrivacyInfo.xcprivacy` / SIWA / loot-box 概率披露
- **Offline**：三档（完全离线 / WS 断 / `ws-registry` fail-closed）

**Cross-Repo Server Stories**（S-SRV-1~18，**不**在本架构实现但 iPhone 架构必须钉契约端点）：已锁 `UserMilestones` / `box.state.unlocked_pending_reveal` / emote fire-and-forget 对称 / fail 节点 metric 四项（S-SRV-15~18）。**⚠️ 调研显示 server 端均未实装**，见 Known Contract Drift。

### Scale & Complexity

- **Primary domain**：Native iOS mobile app（SwiftUI + Swift 6 严格并发）
- **Complexity level**：**high**
- **Project Context**：brownfield（server Epic 0 已完成；iOS 骨架期）

**5 条主要 complexity driver**：

1. **三端版本化契约 + 哲学 B**（server 为主 / WC 为辅）：所有跨端状态走 `server ↔ iPhone` + `server ↔ Watch` 两条独立 WSS；WatchConnectivity 降为**可选 fast-path**优化层
2. **双事实源步数**：HealthKit 展示 SSoT / server 权威解锁 SSoT；iPhone 本地**禁**判定解锁
3. **Swift 6 严格并发 day 1**：`-strict-concurrency=complete` 零 warning
4. **共享 WS envelope + fire-and-forget UI 对称性**：单连接共享信封，两档 ACK 语义（事务性 vs fire-and-forget），UI 层**禁**显示"已读 / 已送达 / 发送失败"
5. **iPhone-only 观察者模式 + 隐私 gate**：observer 可见字段 server 端 fan-out 时裁剪；观察者与 Watch 用户权限对等发表情（fire-and-forget 对称）

**架构层次规模**（基于 UX Component Strategy + project-context 目录约定）：

- `CatShared/UI/` 跨端共享组件 ≈ 7（MyCatCard / SkinRarityBadge / StoryLine / EmptyStoryPrompt / GentleFailView / EmojiEmitView / LottieView）
- `CatShared/Sources/CatShared/` 原子能力：Networking / Persistence / Resources（DesignTokens + Animations）/ Configuration（Timeouts）/ Utilities（Log facade / Clock / IdleTimer / FirstTimeExperienceTracker / AccessibilityAnnouncer）
- `CatCore` 业务编排：use-case / service / coordinator（依赖 `CatShared` · 反向禁止）
- `CatPhone/Features/` Feature 模块：Onboarding / Account / Friends / Inventory / Dressup / Room（6 个 feature，每个 ~3–6 组件）

### Technical Constraints & Dependencies（已锁）

| 维度 | 决策 | 来源 |
|---|---|---|
| Platform | iOS 17+ / Swift 5.9+ / Xcode 16 / SwiftUI / 禁跨平台 | `project.yml` + PRD |
| 并发 | Swift 6 严格并发 day 1（`-strict-concurrency=complete`） | `PCT-arch` + PRD |
| 工程构建 | XcodeGen 驱动 · `project.yml` = SSoT · 禁直接改 `Cat.xcodeproj/` | `PCT-arch` |
| 网络栈 | `URLSession` + `URLSessionWebSocketTask` only · **禁** Alamofire / Starscream / Moya | `PCT-arch` |
| 持久化 | SwiftData（iOS 17）+ UserDefaults（轻量）+ Keychain（token）· **禁** Realm / CoreData | `PCT-arch` |
| 渲染 | iPhone 端**禁 link `spine-ios` SDK**；Watch 独占 Spine；iPhone 用帧序列 PNG / Lottie（`lottie-ios` SwiftPM） | UX v0.3 |
| 状态管理 | `@Observable` + `@Environment(StoreType.self)` · **禁** `ObservableObject` / `@EnvironmentObject` / `@StateObject` | `PCT-arch` |
| 导航 | `NavigationStack` + `NavigationPath` · **禁** `NavigationView` | `PCT-arch` |
| JSON 编解码 | 全 camelCase（不 `convertFromSnakeCase`）· `tsMs` 保留 Int64 不转日期 · 宽松 decode（忽略 unknown fields） | `PCT-arch` + server 实证 |
| WatchConnectivity | **哲学 B 降级**：MVP 不做核心通路；三档通道仅作 fast-path 可选优化层 | UX v0.3 |
| 测试 | `xcodebuild test` 一命令绿 · 禁依赖真机 / 真 Watch / 真 server · 真机归 Epic 9 | `CLAUDE-21.7` |
| 本地化 | 中文硬编码 MVP · 禁 `String(localized:)` · 国际化推 Growth | `PCT-arch` |
| 依赖管理 | SwiftPM · 只 Apple + Spine（Watch）+ Lottie（iPhone） | `PCT-arch` + UX v0.3 |

**Server 契约真相源（2026-04-21 调研）**：

- **Envelope 结构 100% 与 iPhone 假设一致**（`server/internal/ws/envelope.go:5-27`）：`{id, type, payload?}` / `{id?, ok, type, payload?, error?}`；无隐藏字段（seq / meta / ts / ack 都不存在）
- **Ping/Pong**：server 30s ping / 60s pong timeout（URLSessionWebSocketTask 自动回 pong；`server/internal/ws/hub.go:45-49`）
- **错误码 24 条**（`server/docs/error-codes.md`）分 4 类：`retryable` / `client_error` / `retry_after`（含 `Retry-After` header）/ `fatal`；iPhone **按类**决定重试策略而非按码
- **roomId 限制**：1–64 **UTF-8 字节**（非字符数）
- **Debug/Release 模式不对称**：Release 模式 `WSMessages` 数组为空；业务消息全 DebugOnly（当前 6 条：`session.resume` / `debug.echo` / `debug.echo.dedup` / `room.join` / `action.update` / `action.broadcast`）
- **Auth 预留未实装**：Debug 模式任意非空 Bearer token = userID；Release 模式 401。SIWA / JWT refresh / nonce 均在 Story 1.1+ 实装
- **SSoT 裁决顺序**：`server internal/dto/` > `docs/api/` > iOS fixture；所有漂移以 server 为准

### Cross-Cutting Concerns

13 条贯穿所有 Feature 的横切关注点：

1. **单 WS 连接 + 信封契约适配**（单连接共享 envelope；单 reconnect 策略；可选 `session.resume` 拉快照；重连后主动 re-`room.join`）
2. **步数双 SSoT 分工**（HealthKit 展示 / server 权威解锁；漂移文案 `C-STEPS-03`；iPhone 本地**禁**判定解锁）
3. **HealthKit 延迟授权 + 降级**（首次进仓库 / 步数展示才请求；拒绝不 crash；步数 "—" + 引导条）
4. **Server 权威里程碑（`UserMilestones`）替代 UserDefaults**（S-SRV-15 · 换机无缝恢复；UserDefaults 仅存纯 UI 级状态如 `emoji_send_count_v1`）
5. **Fire-and-forget UI 对称性纪律**（发送方**永不**显示 ack；本地 emoji + haptic + 猫微回应是心理兜底；`C-ROOM-02` 叙事流 fail-open 加分）
6. **Fail-closed vs Fail-open 区分**（`CLAUDE-21.3`：`ws-registry` fail-closed 屏蔽主功能 · `C-ROOM-02` fail-open 静默降级；每处写入 Dev Notes + `Log.*` 可观测信号）
7. **三元组 Fail AC（B1）**（所有 fail 分支 AC 必含 `(超时, UI 终态, metric)`；无 metric 的 PR reject）
8. **DesignTokens + Timeouts 集中化**（`CatShared/Resources/DesignTokens.swift` + `CatShared/Configuration/Timeouts.swift` 集中 `pair/ack/registry/craft/invite`）
9. **测试基础设施 protocol 抽象**（`WatchTransport` / `ClockProtocol` / `IdleTimerProtocol` / `AccessibilityAnnouncer` / `FirstTimeExperienceTracker` / `KeyValueStore` + Fixture 三件套 `UserDefaultsSandbox` / `WSFakeServer` / `HapticSpy`）
10. **Observability + Privacy 基础设施**（`Log.*` facade 分类 `network/ui/spine/health/watch/ws` + `PrivacyInfo.xcprivacy` + SIWA 账号删除 / 导出联动 `S-SRV-12`）
11. **契约运行时发现 + 可容错消费者**（新增 · 调研驱动）：WSClient 启动时调 `/v1/platform/ws-registry` 获取 `{ type, requiresDedup, requiresAuth }` 元数据表；消息发送前查表而非硬编码；未知消息优雅降级（UI 屏蔽入口 + `Log.ws.warn`）
12. **Dedup 本地窗口对齐 server 配置**（新增 · 调研驱动）：跟随 server `config.dedup_ttl_sec`（default 300s，非 iPhone PRD 假设的 60s）；重连时超过窗口则生成新 `client_msg_id`；防 300s 内重放触发重复扣费 / 广播
13. **断线恢复降级策略**（新增 · 调研驱动）：server 不存在 `seq` / `last_ack_seq` / TTL 过期推送机制；重连后 → 可选 `session.resume` 拉账号快照 → 主动重新 `room.join`（server 断链即从房间移除，无宽限期）；`action.update` 不幂等，禁无脑重发

**承载 §21 工程纪律的 iOS 映射**：

- **§21.1 双 gate 漂移守门** — `MessageType` 枚举 ↔ server `WSMessages` ↔ `ws-message-registry.md` 三处同步；**iPhone 客户端优先走 runtime `ws-registry` 运行时发现**（弱化枚举）
- **§21.2 Empty Provider 逐步填实** — 骨架先立（Networking / LocalStore / StepDataSource / WatchTransport / Clock），Feature 进来再填；空 target / 空 protocol 不许长期存在
- **§21.3 fail-closed vs fail-open** — 每处外部依赖失败处理在 Dev Notes 记录选择 + 可观测信号
- **§21.6 真机归 Epic 9** — Spine + Swift 6 Sendable Spike 归 Epic 9
- **§21.7 测试自包含** — `xcodebuild test` 一命令绿硬规则
- **§21.8 语义正确性思考题** — 每 PR 回答"跑通但结果错会误导谁？"

### Known Contract Drift（iPhone PRD vs Server 实际 · 2026-04-21 调研）

**CRITICAL drift · 架构决策前必须跨仓 sync 锁定**：

| # | 项 | PRD 假设 | Server 实际 | 处置方向 |
|---|---|---|---|---|
| D1 | Dedup TTL | 60s | **300s**（`server/config/default.toml:35` `dedup_ttl_sec = 300`） | PRD 修订 → 跟 server 300s；iPhone 本地缓存窗口同步 |
| D2 | 消息 TTL 24h + `last_ack_seq` 回放 | 存在 | **完全不存在**（无 seq / 无 last_ack_seq / 无过期推送） | PRD Cross-Device Messaging Contract 需整段重写；iPhone 断线恢复降级为"重连 + 可选 resume + 重 join" |
| D3 | 业务 WS 消息（`pet.*` / `box.*` / `room.effect.*` / `craft.*` / `emote.*`） | 已注册 | **Release 模式消息列表为空**；Debug 模式仅 6 条（none 是业务消息） | iPhone 架构按"运行时从 ws-registry 发现消息"设计；Empty Provider 填实节奏与 server epic 对齐；本 PRD 假定的契约调用全部为 **待 server 实装** |
| D4 | 业务 HTTP 端点（SIWA / friends / boxes / steps/history / user/milestones / craft） | 存在 | **仅 3 个** `/healthz` `/readyz` `/v1/platform/ws-registry` | 业务操作全部走 WS；HTTP client 骨架立但业务 endpoints 占位空 |
| D5 | SIWA + JWT 刷新 + nonce | 标准实装 | **未实装**；Debug 模式任意 Bearer token = userID；Release 模式 401 | Story 1.1 前 iPhone 只能用 mock token 联调；Auth 抽象层可插拔 |
| D6 | `S-SRV-15~18`（`user_milestones` / `unlocked_pending_reveal` / `emote.delivered` 取消 / Prometheus metrics） | iPhone PRD 硬依赖 | **零实现**（grep 无结果 · server backlog 也无痕迹） | 跨仓 sync 会议必须锁定交付时间线；架构预留扩展点但**不硬依赖** |
| D7 | `action.update` 幂等性 + 房间脱链宽限期 | — | `action.update` 每次广播，**不幂等**；server 断链即 `RoomManager.OnDisconnect` 移除成员（无 D8 宽限期） | iPhone 重连后**禁无脑重发** action；房间状态客户端自治维护 |

**Cross-Repo Sync Action Items（架构进入 Step 3 前建议 / 但不阻塞当前架构工作）**：

- **[跨仓 sync 会议 60min]** 锁定 D1–D7 七项漂移的 owner + 交付时间线
- **[server 团队]** 明确 S-SRV-15~18 进入哪个 epic / story（目前 backlog 无痕迹）
- **[PRD 团队]** 修订 `Cross-Device Messaging Contract`（dedup 60s → 300s · 删除 `last_ack_seq` / TTL 24h / 过期推送的假设 · 与 server 实际对齐）
- **[docs/api 团队]** 扩充 `ws-message-registry.md` 与 `integration-mvp-client-guide.md`，把业务消息（即使 release 模式未实装）作为"predicted contract"登记

### Open Questions（待 Step 4+ 回答）

1. **模块分层与状态管理**：`@Observable` Store 作用域（Feature 级 vs 全局）；全局 state（`AuthSession` / `WSConnection` / `CurrentRoom`）放 `CatCore` 还是 `CatShared`
2. **WS 客户端架构**：单连接 + envelope codec + 多 subscriber pub/sub 如何组织；可选 `session.resume` 是否作为重连握手标配
3. **HealthKit 抽象形态**：`StepDataSource` protocol + `HKStatisticsCollectionQuery` 按天聚合 + `HKObserverQuery` 增量；15min 缓存层归属
4. **WatchTransport 抽象（哲学 B 降级版）**：MVP 是否仅作 no-op protocol 占位；真实 WC 集成推 Growth
5. **Lottie + 帧序列 PNG 渲染栈**：`LottieView` 封装策略；帧序列资源组织（Asset Catalog vs 独立目录）
6. **Server 契约绑定策略**：手写 DTO vs 未来引入 OpenAPI 代码生成；契约漂移对比脚本（归后续 epic）
7. **Fixture + Test 基础设施位置**：B3 三件套 protocol / 实现；如何跨 `CatShared` / `CatCore` / `CatPhoneTests` 复用
8. **Observability 栈**：MVP OSLog + Xcode Organizer vs Growth Crashlytics/Sentry 的升级路径
9. **Epic 1 切片优先级**：先立哪些骨架 Empty Provider；哪些 Provider 在 Epic 1 内填实 vs 占位到后续 Epic

---

## Starter Template Evaluation

### Primary Technology Domain

Native iOS mobile app（SwiftUI + XcodeGen）· Apple 生态 · brownfield

Apple 生态**没有**与 `create-next-app` / `create-t3-app` 对等的 production starter CLI——iOS 社区 starter 多数会引入三方 lib（Alamofire / R.swift / SwiftyJSON / Quick/Nimble）或替代方案（Tuist vs XcodeGen），与本项目已锁约束冲突。

### Starter Options Considered

| 候选 | 评估 |
|---|---|
| Xcode 默认 "App" template | 无目录分层 / 无测试骨架 / 无 XcodeGen 集成 → 放弃 |
| Tuist 社区 starter | 与已锁的 XcodeGen 互斥（两者竞争） → 放弃 |
| iOS-Starter-Kit / 社区 boilerplate | 多数含 Alamofire / R.swift / SwiftyJSON，违反"不引过度依赖" → 放弃 |
| SwiftUI + TCA（Point-Free）starter | TCA 推翻 `@Observable` + `@Environment` 锁定 → 放弃 |
| **现有 `ios/project.yml` + CatShared/CatCore/CatWatch 骨架** | **与所有约束自洽 · de facto starter** |

### Selected Starter

**现有 `ios/project.yml` + CatShared/CatCore/CatWatch 骨架（brownfield）**

### Rationale

1. **约束自洽性最优**——既有骨架严格遵循 XcodeGen + SwiftPM + "不引过度依赖" + 四目标分层；任何外部 starter 都会撕毁其中至少一条
2. **Brownfield 现实**——项目已定义 bundle ID / XcodeGen 配置 / CatShared Package / Watch sibling target；引入外部 starter 需整体重写
3. **"不引过度依赖"硬纪律**——社区 starter 99% 捆绑三方 lib，与本项目"只引 Apple + Spine(Watch) + Lottie(iPhone)"正面冲突
4. **初始化动作 = XcodeGen 生成 + 首个 Story**——不执行外部 CLI，由 Epic 1 Story 1 做 bootstrap

### Go-with-Conditions（3 条核心 Condition · Party Mode + Advanced Elicitation 合奏裁剪版）

经 Party Mode 四人质询（Winston / Amelia / Murat / Paige）+ Advanced Elicitation（Pre-mortem 5 败因 + First Principles 剥离假设），原 7 条 condition 裁剪为 **3 条核心 condition**。裁剪原则：外部 starter CLI 的本质职责（P1 可编译骨架 / P2 锁技术决策 / P3 防漂移 / P4 30min onboarding）必须被这 3 条完整覆盖，且不制造"Story 0 巨石化"和"跨仓节奏绑架"两大风险。

#### **C1 · Bootstrap 可执行化**（承担 P1 + P4）

合并内容：
- `ios/scripts/build.sh` 填实：`xcodegen generate → xcodebuild build → xcodebuild test → 零 warning 校验（-warningsAsErrors）`
- `ios/scripts/install-hooks.sh` 填实（.git/hooks/pre-push 自动触发 build.sh）
- **破例引入 `swift-format`**（Apple 官方 · 零配置 · 不违"不引过度依赖"精神）：`build.sh` 集成 `swift-format lint --strict --recursive ios/`；Swift 6 严格并发零 warning 的 CI 兜底
- Onboarding 文档三件套：
  - `ios/README.md` · **唯一入口**（新 dev 第一眼看到 · 漏斗结构：Prereqs → Bootstrap 三步 → 跑第一个测试 → 按角色分叉读什么）
  - `ios/docs/dev/troubleshooting.md`（XcodeGen / 签名 / 模拟器常见错误）
  - `_bmad/templates/story-ac-triplet.md`（Story AC 三元组 `(超时, UI 终态, metric)` 模板 · 配 `_bmad/lint/ac-triplet-check.sh` grep 校验）

**判据**：新 dev clone 后 `bash ios/scripts/build.sh --test` **30 min 内绿 · 零 Slack 提问**。

#### **C2 · Provider 骨架 + Fixture 三件套**（承担 P2）

**6 核心 Provider protocol**（Epic 1 Story 1 立齐 · Empty impl 占位）：

```
CatShared/Sources/CatShared/Networking/
  ├─ APIClient.swift            protocol APIClient { func send<T>(_:) async throws -> T }
  ├─ WSClient.swift             protocol WSClient { func connect() async; var messages: AsyncStream<WSMessage> { get } }
  └─ AuthTokenProvider.swift    protocol AuthTokenProvider { func currentToken() async throws -> String }
CatShared/Sources/CatShared/Persistence/
  └─ LocalStore.swift           protocol LocalStore { func get/set/delete }
CatCore/Sources/CatCore/Auth/
  └─ AuthService.swift          protocol AuthService { func signInWithApple() async throws -> Session }
CatCore/Sources/CatCore/Box/
  └─ BoxStateStore.swift        protocol BoxStateStore { var state: AsyncStream<BoxState> { get } }
```

**6 测试抽象 protocol**（UX v0.3 D 组 + Murat 调研汇总）：

- `WatchTransport`（WC 消息进 / 出可替 · 哲学 B 降级版仅作 no-op 占位）
- `ClockProtocol`（`advance(by:)` / `setNow(_:)` · 超时 / 节流测试不依赖真时钟）
- `IdleTimerProtocol`（`UIApplication.shared.isIdleTimerDisabled` 不可测）
- `AccessibilityAnnouncer`（`UIAccessibility.post` 必 stub）
- `FirstTimeExperienceTracker`（server `user_milestones` 账号级里程碑 · UserDefaults 副作用隔离）
- `KeyValueStore`（`emoji_send_count_v1` 类纯 UI 级状态持久化）

**Fixture 三件套**（UX v0.3 B3）：

- `UserDefaultsSandbox`（`UserDefaults(suiteName: UUID)` + tearDown 清理）
- `WSFakeServer`（**envelope-agnostic 设计** · 不硬编码 msg type · 构造函数接 `envelope: Data` + `replay(jsonlFile:)` · fixture jsonl 从 `docs/api/ws-message-registry.md` 生成 · msg type 按字符串处理不做 enum · 测试须覆盖 unknown type default branch）
- `HapticSpy`（`UIImpactFeedbackGenerator` wrap · record-only）

**`DualSSoTScenarioFixture`**（HealthKit stub + WSFakeServer 时序编排器 · 复现"本地步数 =100 / server 权威 =98 / 延迟 2s 后追平"的双 SSoT 漂移窗口 · Murat 指定 starter 阶段必交付，否则 Epic 1/2 每个 story 都在重造）。

**判据**：Feature story 能写 unit test 不依赖真 server / 真机 / 真 Watch。

#### **C3 · 契约漂移防护 = 运行时 + 轻文档**（承担 P3）

- **`WSClient` 运行时从 `/v1/platform/ws-registry` 发现消息**（启动时拉元数据表 · 发送前查表 · 未知消息优雅降级 UI 屏蔽入口 + `Log.ws.warn`）
- **`docs/contract-drift-register.md` 新建**（repo 根 `docs/`，不属于任一端 repo · 承载 Known Contract Drift 7 项 + 未来新增）：

```
| ID | 项 | iOS 值 | Server 值 | 上游权威 | 状态 | Owner | Issue |
|---|---|---|---|---|---|---|---|
| CD-01 | dedup TTL | 60s | 300s | server | open | @who | #xxx |
```

每条附 ≤ 200 字 context；CI 可加"open 超 N 天提醒"软 gate。

- **`ios/scripts/check-ws-fixtures.sh`**（dev-time 手动脚本 · **不进 CI** · 拉 server `ws-message-registry.md` hash 对比本地 fixture snapshot · 下发变更提醒但不阻塞）

**判据**：Epic 2+ 任何 story 引入新 WS 消息类型时能被运行时发现 / 漂移被登记 / CI 不被跨仓节奏绑架。

### 显式删除 / 降级（附 Pre-mortem 教训理由 · 防后人"好心加回来"）

| 原 Condition | 去向 | 教训 |
|---|---|---|
| `swift-openapi-generator` 破例引入 | **删** | server OpenAPI 0.14.0-epic0 尚不稳（半年内可能 0.14 → 0.19 高频 bump）；"防漂移工具"反成漂移重灾区；手写 DTO + `ws-registry` 运行时发现更便宜 |
| CatShared 边界章程 ADR-002 | **删** · 改 `CatShared/README.md` 顶部 5 行硬约束 + `.github/pull_request_template.md` 勾选项 | ADR 易变墙纸（业务压力下 reviewer 懒翻）；**执行纪律 > 文档纪律** |
| `ws-fixtures` SHA gate 进 CI | **降级**为 dev-time 手动 `check-ws-fixtures.sh`（不进 CI） | 跨仓节奏不稳会让 gate 频繁 fail，最终被改 warning-only → 禁用；防"架构被跨仓节奏绑架" |

### Epic 1 Story 1 AC（裁剪后 AC1-5 · 可自动验证不靠 agent 自觉）

- **AC1 [U]**：`bash ios/scripts/build.sh` 在干净 clone 上退出码 0，`-warningsAsErrors` 零 warning
- **AC2 [U]**：`bash ios/scripts/build.sh --test` 运行 `CatPhoneAppTests.SmokeTests.testAppTypeExists` 通过
- **AC3 [U]**：`swift-format lint --strict --recursive ios/` 退出码 0
- **AC4 [U]**：`*.xcodeproj` 在 `.gitignore`，`git check-ignore ios/CatPhone.xcodeproj` 退出码 0
- **AC5 [U]**：C2 清单所有 protocol + `Empty*` impl 存在（`grep -r "protocol APIClient" CatShared/` 等命中）
- **AC6 [E9]**（可延 Epic 9 · 不阻塞 Story 1）：GitHub Actions `.github/workflows/ios.yml` 在 macOS runner 跑 `bash ios/scripts/build.sh --test` 绿

### Initialization Command（Epic 1 Story 1 执行）

```bash
# 1. 确保 XcodeGen 已安装
brew install xcodegen swift-format

# 2. 从 project.yml 生成 Xcode project
cd ios && xcodegen generate

# 3. 安装 git hooks（首次 clone 必做）
bash ios/scripts/install-hooks.sh

# 4. 一键 build + test + lint
bash ios/scripts/build.sh --test
```

### Architectural Decisions Provided by Starter（既有骨架已有的）

| 维度 | 决策 |
|---|---|
| 语言 | Swift 5.9+ · SwiftUI · iOS 17.0 deployment target |
| 工程 | XcodeGen（`project.yml` SSoT）· 四 target（CatPhone / CatWatch / CatShared / CatCore） |
| 依赖管理 | SwiftPM（`CatShared/Package.swift`） |
| 目录布局 | `CatShared/Sources/CatShared/{Models, Networking, Persistence, Resources, UI, Utilities}` + `CatShared/Sources/CatCore/{use-case, service, coordinator}` + `CatPhone/Features/{Account, Friends, Inventory, Dressup, Room, Onboarding}` |
| 测试 | XCTest · CatSharedTests / CatCoreTests / CatPhoneTests · CatPhoneUITests（Epic 1 Story 1 内加入 project.yml） |
| 构建脚本 | `ios/scripts/build.sh`（C1 填实）· `xcodegen generate → xcodebuild build → xcodebuild test → swift-format lint --strict → 零 warning 校验` |
| Git hook | `.git/hooks/pre-push` 自动触发 `build.sh`（C1 `install-hooks.sh` 填实） |
| VCS ignore | `Cat.xcodeproj/` ignored；追踪 `project.yml` + `Package.swift` + `Package.resolved` + `Assets.xcassets` |

**Note**：本项目初始化不执行外部 starter CLI；架构决策由 `project.yml` + `project-context.md` + 本架构文档共同承载。Epic 1 Story 1 做 C1-C3 全部落地 + AC1-5 自动验证。

### Open Questions 追加到 Step 4

Pre-mortem P4 教训：**Provider + Empty 模式 × Swift 6 严格并发 × `@Observable` 的 Sendable 冲突解法** —— `EmptyBoxStateStore` 跨 actor 提供 `AsyncStream<BoxState>` 给 `@Observable` Store 时，Swift 6 报 non-Sendable；`@unchecked Sendable`（留 race 种子） vs `@MainActor` 整包（失去并行）的权衡，**留给 Step 4 Architecture Decisions 回答**。

---

## Core Architectural Decisions

### Decision Priority Analysis

**Already Decided（from project-context + PRD + UX + Step 3 · 不再重讨论）**：

- Platform / Language: iOS 17+ · SwiftUI · Swift 5.9+ · Swift 6 严格并发 day 1
- 工程 / 依赖: XcodeGen · SwiftPM · 禁三方（Spine Watch / Lottie iPhone / swift-format 破例）
- 状态管理: `@Observable` + `@Environment(Type.self)` · 禁 `ObservableObject` / TCA
- 导航: `NavigationStack` + `NavigationPath`
- 网络栈: `URLSession` + `URLSessionWebSocketTask` · 单 WS 连接共享 envelope
- 持久化: SwiftData + UserDefaults（纯 UI）+ Keychain（token）· `user_milestones` server 权威
- HealthKit: 展示 SSoT iPhone / 解锁 SSoT server · iPhone 禁判定
- Auth 传输: wss + ATS · Keychain · SIWA nonce
- 测试: XCTest only · 禁 Quick/Nimble/SnapshotTesting · `xcodebuild test` 一命令绿
- 本地化: 中文硬编码 MVP
- 渲染: iPhone 禁 Spine SDK · Lottie + 帧序列 PNG
- 目录分层: CatShared → CatCore → CatPhone/CatWatch（单向）
- Provider 抽象: 6 核心 + 6 测试 protocol + Empty impl（Step 3 C2）
- Fixture: UserDefaultsSandbox / WSFakeServer / HapticSpy + DualSSoTScenarioFixture
- 契约漂移防护: `ws-registry` 运行时发现 + `docs/contract-drift-register.md` + dev-time `check-ws-fixtures.sh`
- Dedup TTL: **300s**（跟 server · 非 PRD 假设的 60s）
- 断线恢复: 重连 → 可选 `session.resume` → 主动 re-`room.join` · **无 seq/last_ack_seq**

**Critical（阻塞 Epic 1 实施）**：D1 · D2 · D3
**Important（塑造架构）**：D4 · D5 · D6
**Deferred（MVP 后）**：D7

---

### D1 · `@Observable` Store 作用域 + 全局 State 安放

**Decision**：
- **全局 Store 放 `CatCore`**（AppCoordinator 层）· `@Environment` 从根注入 · 4 个：`AuthSession` / `WSConnection` / `CurrentRoom` / `HealthKitAuthorization`
- **Feature 级 Store 放 `CatPhone/Features/<X>/<X>Store.swift`**（`InventoryStore` / `FriendsStore` / `AccountStore` 等）· 由 Feature view 自持 · 跨 Feature 依赖走 `CatCore` service protocol
- **`CatShared` 绝不放 `@Observable` Store**（只放 wire 契约 · 遵 Pre-mortem P3 教训）

**Rationale**：Feature 级内聚高；全局 concern 少而稳（4 个）；`CatCore` 承担跨 Feature 编排定位；`CatShared` 保持 wire-level 纯度。

**Affects**：所有 Epic 的 Store 组织结构。

---

### D2 · WS 客户端架构

**Decision**：

```
CatShared/Networking/
├─ WSEnvelope.swift          // {id, type, payload?} + {id?, ok, type, payload?, error?}
├─ WSClient.swift (protocol)
├─ WSRegistry.swift          // 启动拉 /v1/platform/ws-registry；缓存 {type: {requiresDedup, requiresAuth}}
├─ WSClientEmpty.swift
└─ WSClientURLSession.swift  // Epic 2+ 填
```

- **单连接**：`WSClientURLSession` 持一个 `URLSessionWebSocketTask` · `@Observable class WSConnection` in CatCore 包装生命周期 + 指数退避重连
- **Pub/Sub**：`AsyncStream<WSPush>` 单一 push 通道 · Feature Store `for await` 过滤 `type` · 不自建 NotificationCenter / Combine 桥
- **发送**：`send<T>(type:, payload:, awaitResponse:)` · `awaitResponse=true` 走 dedup 300s · `awaitResponse=false` fire-and-forget
- **运行时发现**：`WSClient` 发送前查 `WSRegistry` · 未知 type 返回 `UNKNOWN_MESSAGE_TYPE` + `Log.ws.warn` + UI 屏蔽入口
- **重连握手**：重发 Authorization → 可选 `session.resume` 拉快照 → 在房间时主动 `room.join` · **禁**试图 replay `last_ack_seq`

**Rationale**：单 `AsyncStream` 语义简单 · Swift concurrency 原生 · 无 Combine 开销 · fire-and-forget 对称性由 API 签名表达。

**Affects**：Epic 2+ 所有 WS 消费 Feature。

---

### D3 · Provider × Swift 6 严格并发 × `@Observable` Sendable 解法（Pre-mortem P4）

**Decision · 3 种解法混用**：

- **A · 全局 `@MainActor`**（默认）：Provider protocol 方法 `@MainActor`；Store 也 `@MainActor`
  - 适用：`AuthTokenProvider` / `LocalStore`（非 SwiftData 部分）/ `AccessibilityAnnouncer` / `FirstTimeExperienceTracker` / `KeyValueStore` / `ClockProtocol` / `IdleTimerProtocol`
- **B · Actor 隔离 Provider**：Provider 实现是 `actor` · `AsyncStream` 由 actor continuation 产生 · `XxxState: Sendable`（struct）
  - 适用：`WSClient`（网络 I/O）/ `StepDataSource`（HealthKit 后台）/ `LocalStore` SwiftData 部分
- **C · `@unchecked Sendable` 逃逸舱**：仅 Apple 框架非 Sendable 限制场景
  - 必须注释 `// FB<issue_id>` 归档；PR review 逐条质询

**Rationale**：A 成本最低覆盖多数；B 只用在真实后台需求；C 严控。

**Affects**：12 个 protocol 的 Sendable 符合性。

---

### D4 · HealthKit 抽象 + 15min 缓存归属

**Decision**：

```swift
// CatCore/Health/StepDataSource.swift
protocol StepDataSource: Sendable {
    func todayStepCount() async throws -> Int
    func stepHistory(range: DateInterval) async throws -> [DailyStepCount]
    var incrementalUpdates: AsyncStream<Int> { get }
}
```

- 实现：`actor HealthKitStepDataSource`（内持 `HKStatisticsCollectionQuery` + 15min cache）
- Empty：`struct EmptyStepDataSource`
- Test：`class InMemoryStepDataSource`（可控 step 值 + 可注入延迟）
- 授权：`HealthKitAuthorization` `@Observable` in CatCore 统一管 · 首次进仓库 / 步数展示 trigger
- 增量：`HKObserverQuery` 回调 → continuation 推 · `InventoryStore` `for await` 更新 UI

**Rationale**：actor 隔离保后台查询并行；cache 封装在 actor 内部 Store 层无感；AsyncStream 与 WS 风格一致。

**Affects**：Epic 2+ 步数展示、盲盒进度。

---

### D5 · WatchTransport 哲学 B 降级版

**Decision**：

- **MVP**：`WatchTransport` protocol + `WatchTransportNoop` 唯一实现（`isReachable = false` · `send = no-op`）
- **Release**：无 `WCSession` 实际代码链接 · binary 纤细
- **Growth**：`WatchTransportWCSession` 实装（三档通道 `updateApplicationContext` / `transferUserInfo` / `sendMessage` 作为 fast-path）
- **测试**：`FakeWatchTransport` spy（对齐 Murat B3）

**Rationale**：Empty Provider 纪律；MVP 无 WC 依赖即可编译 / 跑测试绿；Growth 不改 call-site 只换 impl。

**Affects**：Epic 1（Noop 占位）· Growth Epic（WC 实装）。

---

### D6 · Lottie + 帧序列 PNG 渲染栈

**Decision**：

```
CatShared/UI/LottieView.swift         // SwiftUI wrap LottieAnimationView · {animation, loop, onComplete}
CatShared/Resources/Animations/       // .json Lottie 文件（跨端共用）
CatPhone/Features/<X>/Resources/      // 帧序列 PNG @2x/@3x · feature 局部（非共用）
```

- **Lottie**：`LottieView` 读 `@Environment(\.accessibilityReduceMotion)` · reduce-motion 显首帧静帧停留 UX v0.3 指定时长
- **帧序列 PNG**：自定义 `FrameSequencePlayer` View · `TimelineView` 驱动 · reduce-motion 停最后一帧
- **资产管理**：Lottie `.json` 跨端（CatShared/Resources/Animations/）· 帧序列 PNG feature 局部
- **Spine 绝对禁 iPhone**：PR template 勾选项 · CatPhone target dependencies 硬断言

**Rationale**：Lottie 跨端能力放 CatShared / 帧序列作为 feature 资产放局部 / reduce-motion 统一由 Environment 贯穿。

**Affects**：Epic 1（CatGazeAnimation 2.5s · UX v0.3 C2）· Epic 2（emoji 微回应）· Epic 4（撕包装仪式）。

---

### D7 · Observability MVP → Growth 升级路径（Deferred）

**Decision · MVP**：
- `CatShared/Utilities/Log.swift` · 封装 `OSLog` · 分类 `Log.network/ui/spine/health/watch/ws` · 级别 `debug/info/notice/error/fault`
- 崩溃：Xcode Organizer 基础（零三方）
- 指标：server 端 Prometheus 承担（S-SRV-18）· iPhone 侧不本地上报
- `PrivacyInfo.xcprivacy` 清单（必进 MVP）

**Decision · Growth（Deferred）**：Crashlytics vs Sentry 选型（`C-OPS-01`）· 远程配置（`C-OPS-03`）· in-app 强更（`C-OPS-02`）· 均在 Growth Epic 再评估

**Rationale**：MVP 轻量符合 `CLAUDE-21.3` fail-closed 可观测信号（`Log.ws.error` 承担）· Growth 再评估三方 SDK。

**Affects**：所有 Feature（Log.* facade 贯穿）。

---

### Decision Impact Analysis

**Implementation Sequence（Epic 1 Story 1 之后的 Epic 1 剩余 Story）**：

1. D1 全局 Store 骨架（AuthSession / WSConnection / CurrentRoom / HealthKitAuthorization in CatCore）
2. D2 WSClient + WSRegistry 骨架（Empty + URLSession impl）
3. D3 Sendable 分类注入（12 protocol 标注 A/B/C 方案）
4. D7 Log facade + OSLog wrap
5. D4 StepDataSource 骨架（Empty + HealthKit actor）
6. D6 LottieView 骨架（Epic 1 CatGazeAnimation 需要）
7. D5 WatchTransportNoop（骨架即交付物）

**Cross-Component Dependencies**：
- D2 WS 依赖 D1（AuthSession 提供 token）+ D3（actor B）+ D7（Log）
- D4 HealthKit 依赖 D1（HealthKitAuthorization 状态）+ D3（actor B）
- D6 Lottie 独立
- D5 WatchTransport 独立（Noop）

---

### Open Questions · 待 Step 5 Patterns / Step 7 Validation 再评估

（来自 Step 4 Party Mode 四人挑战 · 本 Step 未合入但保留议题）

- **O1 · D2 AsyncStream 多消费者陷阱**（Winston/Amelia/Murat 共识）：裸 `AsyncStream` 多 `for await` 抢 element（非广播）· 是否改为 `actor WSHub { subscribe(type) -> AsyncStream }` 多路广播 + `WSHubTests` 断言"所有 type 都有订阅者"
- **O2 · D3 默认化方向**（Winston/Amelia）：是否反转为"Store `@MainActor`；Provider 默认非 MainActor（actor / Sendable struct）"防主线程蔓延 · `.swiftlint.yml` 加 `@unchecked Sendable` 必须同行 `// FB\d+` custom_rule
- **O3 · D4 Clock 注入 + `cacheHitCount` 计数器**（Amelia/Murat）：`init(clock: any Clock<Duration> = ContinuousClock(), cacheTTL: Duration)` + cacheHitCount 计数器使 15min cache 边界可断言；`HealthKitSpy` 纳入 C2 fixture
- **O4 · D5 compile-time 硬验证**（Winston）：`#if WATCH_TRANSPORT_ENABLED` flag + CI linker symbol check 确保 release binary 真不含 `WCSession`
- **O5 · D6 三栈分层**（Sally）：emoji 浮起 / 微回应是否走 SwiftUI 原生 `PhaseAnimator`/`KeyframeAnimator` 而非 Lottie；PNG 帧序列仅留特殊 case
- **O6 · D7 崩后证据链**（Murat）：MVP 内补 `LogSink` protocol + 本地环形 JSONL + 崩后首启上报（零三方），Crashlytics 仍推 Growth
- **O7 · UX 补**（Sally）：HealthKit 授权弹窗期间猫"歪头" 0.8s 过渡，避免空白等待

---

## Implementation Patterns & Consistency Rules

### 已由 project-context.md + CLAUDE.md + Step 3/4 覆盖（不重述）

代码命名 / 文件名 / JSON 编码 / 错误传输 / Logging facade / 并发 `@MainActor` / 目录分层 / 测试自包含 · 详见 `_bmad-output/project-context.md`

### Step 5 新增 13 条 Pattern + 2 条基础设施 + Growth 延迟清单

经 Party Mode + Advanced Elicitation 四人合奏裁剪（Amelia / Murat / John / Paige），原 10 条 Pattern 修订为 **13 条 + 2 条基础设施**；部分细粒度规则推延 Growth 防 MVP 过度工程化。

---

### P1 · `@Observable` Store 命名 + 属性可见性

- 全局 Store = 纯名词（`AuthSession` / `WSConnection` / `CurrentRoom` / `HealthKitAuthorization`）· **禁** `Service/Manager/Coordinator` 后缀
- Feature Store = `<Feature>Store`（`InventoryStore` / `FriendsStore` / `RoomStore`）
- 方法 = 动词+名词（`loadSkins()` / `acceptInvite(_:)` / `sendEmote(_:)`）
- State 属性 = `let` 或 `private(set) var` · **禁**公开 `var`

**示例**：
```swift
@Observable
final class InventoryStore {
    private(set) var skins: [Skin] = []
    func loadSkins() async { /* ... */ }
}
```

---

### P2 · Protocol + 3 Impl 命名（MVP 简化 · John 剪）

- Protocol = 职能名词（`StepDataSource` / `AuthTokenProvider` / `WatchTransport`）· 禁 `Protocol` 后缀
- **MVP 三分法**：
  - `Empty<X>` / `Noop<X>` · 骨架占位（`EmptyStepDataSource` / `WatchTransportNoop`）
  - `<X>URLSession` / `<X>HealthKit` · 真实实现（按依赖命名）
  - `Fake<X>` · 测试可控伪实现（`FakeStepDataSource`）
- **Growth 再补**：`InMemory<X>`（可控存储）· `Spy<X>`（记录调用）按需引入

**反模式**：`MockStepDataSource` / `StubWSClient`（Mock/Stub 语义模糊）

---

### P3 · WS 消息 type 字符串常量 + `actor WSHub` 多路广播（整合 O1）

- WS `type` **不** enum（容忍 server 新增 · 漂移由 `WSRegistry` 运行时发现）
- 常量放 `CatShared/Networking/WSMessageTypes.swift`（静态字符串）
- **`actor WSHub` 多路广播**（O1 必纳 · 修订 D2 方案）：

```swift
// CatCore/WS/WSHub.swift
actor WSHub {
    func subscribe(_ type: String) -> AsyncStream<WSPush>  // 每订阅者独立 stream
    func publish(_ push: WSPush) async                      // fan-out 到匹配 type 的所有订阅者
}
```

- Feature Store 通过 `wsHub.subscribe(WSMessageTypes.boxDrop)` 拿独立 stream · **禁**裸 `for await wsClient.messages`
- 测试：`WSHubTests.test_twoSubscribers_bothReceive` + `test_allWSMessageTypes_haveSubscriber`（`allCases` 枚举断言）

**Rationale**：裸 `AsyncStream` 多 `for await` 会抢 element（非广播）· Epic 2 Party Mode 多 Feature 订阅必翻车。

---

### P4 · Fire-and-forget UI 对称性 + SwiftLint 守门

- UI **禁**出现：已读 / 已送达 / 对方已收到 / 发送中 / 发送失败 / 重试发送
- UI **允许**：本地 emoji 浮起 / haptic / 猫微回应 / 房间叙事流 presence
- 封装：`CatCore/WS/FireAndForget.swift` 提供 `func fireAndForget(type:, payload:)`
- **SwiftLint custom rule**：禁 `wsClient.send(.*awaitResponse: false)` 直调 · 必须走 `fireAndForget()` wrapper（`.swiftlint.yml` regex 落地）

**守门**：Murat 20/25 + Paige "最易漂移" · 必须 lint 自动拦截。

---

### P5 · Fail-closed vs Fail-open 决策表（锚点行号引用）

每处外部依赖失败处理**必须在 Dev Notes 引用本表锚点行号 + `Log.*` 可观测信号**（对齐 `CLAUDE-21.3`）：

| L# | 场景 | Policy | UI 行为 | 可观测信号 |
|---|---|---|---|---|
| L1 | `/v1/platform/ws-registry` 启动 | **closed** | 屏蔽主功能 + "网络累了" | `Log.ws.error("registry_fetch_failed")` |
| L2 | SIWA 登录 | **closed** | 阻断 + 人话错误 | `Log.network.error("siwa_failed")` |
| L3 | 盲盒解锁 ACK 超时 | **closed** | "已达成·等待确认" | `Log.ws.error("box_unlock_timeout")` |
| L4 | 合成 `/craft` | **closed** | "合成没成功，材料还在仓库" + 重试 | `Log.ws.error("craft_failed")` |
| L5 | `C-ROOM-02` 叙事流 | **open** | 静默（保 emoji + haptic） | `Log.ws.warn("narrative_unavailable")` |
| L6 | 表情广播 fan-out | **open** | 本地呈现照常 | `Log.ws.warn("emote_broadcast_failed")` |
| L7 | 观察者推送 | **open** | 静默降级 | `Log.ws.warn("observer_push_failed")` |

文件：`docs/fail-closed-policy.md` 落地全表 + 行号锚点 · Story AC `fail_policy` 字段必须引用 `fail-closed-policy.md#Lxx`

**Rationale**：散文决策易漂移（Paige）· 强制锚点引用让 reviewer 机械核对。

---

### P6 · Server Error 2 类重试（MVP 简化 · John 剪）

**MVP 阶段**：按错误**大类**决定策略：

```swift
// CatShared/Networking/ServerErrorCategory.swift
enum ServerErrorCategory {
    case retryable       // 5xx + 429 → 指数退避自动重试（max 3 次 / 500ms-4s · Retry-After 覆盖）
    case failSilent      // 4xx（非 429）→ 不重试 + 人话提示用户
}
```

- 每个错误码在 `ServerError+Category.swift` 注册分类（grep 全量断言无遗漏）
- **禁**按单个错误码硬编码分支（`if code == "RATE_LIMITED"` 反模式）
- 401/403 `fatal` 由 AuthSession 统一处理（清 token + 踢登录），不进 Error category

**Growth 再细化**：`retryable` / `clientError` / `retryAfter` / `fatal` 四分（`C-OPS` Growth epic 补）

---

### P7 · Log facade 3 分类 × 3 级别 + `LogSink` + 环形 JSONL（MVP 简化 · 整合 O6）

**MVP 简化版**（John 剪 6×5 → 3×3）：

- 分类 3 个：`Log.network` / `Log.ui` / `Log.health`
- 级别 3 个：`debug / info / error`
- Release 禁明文 token / email / PII

**整合 O6 · LogSink + 崩后上报**（Amelia+Murat+John 共识必纳）：

```swift
// CatShared/Utilities/Log.swift
protocol LogSink: Sendable {
    func write(_ entry: LogEntry) async
}

final class OSLogSink: LogSink { /* OSLog 直写 */ }
final class RingBufferJSONLSink: LogSink { /* 本地环形 JSONL · Documents/logs/current.jsonl · 大小上限 5MB */ }
final class InMemoryLogSink: LogSink { /* 测试用 */ }
```

- 生产：`Log` 同时写 `OSLogSink` + `RingBufferJSONLSink`
- 崩后首启：检测上次非正常退出 → 读 ring buffer → `POST /v1/platform/crash-reports`（server 端点 S-SRV-*-new · MVP 可先本地存）
- Fail 节点必带 metric tag（对齐 server S-SRV-18）：`Log.network.error("box_unlock_timeout", ["metric": "box_unlock_timeout_total"])`

**Rationale**：John "MVP 上线后唯一能看见用户流失原因的管道" · 零三方依赖（纯 Foundation）

---

### P8 · 三元组 Fail AC 模板（多 fail 面覆盖 · Murat 补）

文件：`_bmad/templates/story-ac-triplet.md`

```yaml
ac_id: AC-<story>-<n>
tag: [U|I|E9]                  # 必填，CI grep 校验
trigger: <前置动作>
fail_modes:                    # 3×N 占位 · 覆盖多 fail 面
  timeout:
    threshold_ms: <int>
    ui_terminal: <失败 UI>
    metric: <prom metric>
  disconnect:
    ui_terminal: <失败 UI>
    metric: <prom metric>
  unauthorized:                # 若 story 涉及认证
    ui_terminal: <失败 UI>
    metric: <prom metric>
success_terminal: <成功 UI>
fail_policy: "fail-closed-policy.md#Lxx"   # 必须引用 P5 锚点
```

- **必填**：至少 `timeout` + 另外 1 个 fail 面
- 配 `_bmad/lint/ac-triplet-check.sh` · grep 三字段齐全 + tag + fail_policy 锚点格式
- Story 0 交付 · Epic 2+ 每条 Story AC 必走

---

### P9 · Feature Store 通信（MVP 手动 · Lint 延迟 Growth）

- Feature Store **禁** `import <Other Feature>Store`（编译期依赖方向单一）
- 跨 Feature 依赖通过 `CatCore` 的 service protocol（如 `UnlockService` / `RoomService`）
- **MVP**：PR checklist 人工勾选核对（Feature 数 < 5）
- **Growth 补 lint**：Feature 数 ≥ 5 时引入 SwiftLint custom rule 自动拦截

**Rationale**：John "现在写 lint 是给未来的自己" · MVP 阶段 5 个 Feature 手工审核够用。

---

### P10 · PR Checklist 5 条（MVP 简化 · John 剪 8→5）

`.github/pull_request_template.md` 必含（MVP 精简版）：

- [ ] **Fail 方向正确**：若涉及外部依赖失败，已引用 `docs/fail-closed-policy.md#Lxx`
- [ ] **AC 三元组齐备**：每个新 Story AC 含 `(timeout, ui_terminal, metric)` + `fail_policy` 锚点
- [ ] **契约同步**：触及 server 契约 → `docs/api/*.md` 已先更新 · iOS fixture 同步
- [ ] **Metric 打点**：Fail 节点 `Log.*.error` 含 `["metric": "xxx_total"]` tag
- [ ] **swift-format + build.sh --test 绿**：零 warning · SwiftLint custom rules 通过

**Growth 补**：敏感字段明文扫描 / 三方 SDK 引入检查 / WatchConnectivity import 检查（目前手动）

---

### P11 · Fixture/Spy 生命周期契约（Murat 新增）

每个 `Fake<X>` / `Spy<X>` / `<X>Sandbox` 必须实现：

```swift
protocol TestDouble: AnyObject {
    func reset()                                      // 清空内部状态
    var unfulfilledExpectations: [String] { get }    // 未兑现期望（如 spy 期望调用次数）
}
```

- `XCTestCase.setUp()` 创建 · `tearDown()` 断言 `unfulfilledExpectations.isEmpty`
- **禁**跨 test 复用 TestDouble 实例（防状态污染 → flaky 温床）

---

### P12 · 测试目录三分镜像（Murat 新增）

目录约定：

```
Tests/
├─ Unit/<Feature>/         ← 与 App/<Feature>/ 一一对应
├─ Fixture/                ← 共用 fixture（UserDefaultsSandbox / WSFakeServer / HapticSpy）
└─ Integration/DualSSoT/   ← 双 SSoT 时序编排（HealthKit + WS）
```

- 新 Feature target 必对应 `Tests/Unit/<Feature>/` 同步创建
- PR gate：`ios/scripts/check-test-mirror.sh`（10 行 shell）· 新 Feature 无测试目录则 reject

---

### P13 · 时间注入（Clock Protocol · O3 部分采纳）

- 所有涉及"时间 / 超时 / 冷却 / 重试" 的 Provider 接受 `any Clock<Duration>` 注入：

```swift
actor HealthKitStepDataSource {
    init(clock: any Clock<Duration> = ContinuousClock(), cacheTTL: Duration = .seconds(900))
}
```

- 生产默认 `ContinuousClock()` · 测试用 Swift 标准库 `TestClock` 做 `advance(by:)` / `setNow(_:)` 可控模拟
- **Rationale**：15min cache 边界 / WS 重连指数退避 / rate limit 冷却，离了 Clock 注入无法稳定测试（Amelia+Murat 共识）
- **未采纳**：`cacheHitCount` 计数器 / `HealthKitSpy` protocol · 按 Story 触发需要再补（Amelia "不预置"立场）

---

### 基础设施 I1 · Pattern 索引表（Paige 新增）

新建 `docs/patterns/README.md` · **单一目录卡**：

| ID | Pattern 名 | 落地代码路径 | 触发场景 | 验证方式 |
|----|----------|------------|---------|---------|
| P1 | Store 命名 | `CatCore/Stores/*.swift` + `CatPhone/Features/*/Store.swift` | 新增 Store | PR review |
| P3 | WSHub 多路广播 | `CatCore/WS/WSHub.swift` | Feature 订阅 WS push | `WSHubTests` |
| P4 | fireAndForget | `CatCore/WS/FireAndForget.swift` | 发 fire-and-forget msg | SwiftLint custom |
| P5 | fail-closed/open | `docs/fail-closed-policy.md` | 外部依赖失败 | AC triplet 行号引用 |
| P6 | Error 2 类重试 | `CatShared/Networking/ServerErrorCategory.swift` | HTTP/WS 错误 | grep 分类注册无遗漏 |
| P7 | LogSink + JSONL | `CatShared/Utilities/Log.swift` | 所有日志 | Crash 首启上报测试 |
| P8 | AC 三元组 | `_bmad/templates/story-ac-triplet.md` | 每 Story AC | `ac-triplet-check.sh` |
| P13 | Clock 注入 | `CatShared/Utilities/Clock.swift` | 超时 / 冷却逻辑 | TestClock fixture |
| ... | ... | ... | ... | ... |

- **关键**：每条 pattern 标代码落地绝对路径 · 无路径的 pattern = 漂移候选
- **更新纪律**：新 pattern 加入 architecture.md 同时必须更新索引表（PR reviewer 守门）

---

### 基础设施 I2 · README 玄关化（Paige 新增）

`ios/README.md` **重构为玄关**（≤ 100 行 · 机场指示牌式，不承载内容）：

```markdown
# CatPhone iPhone App

## 去哪找什么

- **第一次来 → [新 dev onboarding](../docs/dev/onboarding.md)**（15 min 跑通第一个绿测）
- **要写代码 → [Pattern 索引](../docs/patterns/README.md)**（所有实施 pattern 的单一目录卡）
- **找文档 → [文档地图](../docs/map.md)**（按角色分叉：PM / UX / Dev / QA）

## Bootstrap 三步

```bash
brew install xcodegen swift-format
cd ios && xcodegen generate
bash ios/scripts/build.sh --test
```
```

- `docs/dev/onboarding.md` 承载详细 onboarding 流程（含 troubleshooting 链接）
- `docs/patterns/README.md` 承载 pattern 索引（I1）
- `docs/map.md` 新建 · 按角色分叉的文档地图（PM 读 PRD / UX 读 ux-spec / Dev 读 architecture + patterns / QA 读 fail-closed-policy + test-fixture-guide）

**Rationale**：README 三重职责（onboarding + pattern 索引 + 文档地图）必撑爆 · 玄关化分流防单文件失控。

---

### Growth 延迟清单（明确登记 · 防未来"好心补回来"）

Step 5 Party Mode 决定 MVP 阶段**暂不实施**的 pattern / 工具（登记触发条件）：

| 项 | 触发条件 | 理由（Party Mode 来源） |
|---|---|---|
| P6 Error 4 类细分 | Growth epic（C-OPS-*） | John 过度工程化批评 · MVP retry-5xx + fail-silent-4xx 够用 |
| P7 Log 6×5 笛卡尔积 | 数据反馈真有 6 维度需求 | John · 30 维笛卡尔积给 Growth |
| P9 Feature Store 互 import lint | Feature 数 ≥ 5 | John · 现在手工 review 够用 |
| **O2 @unchecked Sendable `// FB\d+` swiftlint custom rule** | 首次发现代码中已有 `@unchecked Sendable` 无 FB 注释 | Amelia 反对（Swift 6 编译期已报）· Murat 支持（16/25）· 按 Amelia 默认延迟 |
| **O4 `#if WATCH_TRANSPORT_ENABLED` + linker symbol CI check** | Growth epic 引入 `WatchTransportWCSession` 实装前 | Amelia/Murat/John 一致 · MVP 无 WC 依赖时无风险 |
| **O5 SwiftUI PhaseAnimator/KeyframeAnimator 三栈分层** | Story 级别判断 | Amelia/Murat/John 一致 · 属 UI 表现层非架构 pattern |
| **O7 HealthKit 授权弹窗猫歪头过渡** | UX spec 补（非 architecture 范围） | Sally · 归 UX v0.4 |

---

### Decision Impact Analysis

**Implementation Sequence（Epic 1 Story 1 bootstrap 之后）**：

1. I1 `docs/patterns/README.md` 骨架立 · I2 `ios/README.md` 玄关化
2. P1 全局 Store 骨架（对齐 D1）
3. P3 `actor WSHub` 骨架 + `WSMessageTypes.swift`
4. P4 `fireAndForget()` wrapper + SwiftLint custom rule
5. P7 `LogSink` + `OSLogSink` + `RingBufferJSONLSink` + 崩后首启上报
6. P5 `docs/fail-closed-policy.md` 初版 + 行号锚点
7. P8 `story-ac-triplet.md` + `ac-triplet-check.sh`
8. P13 `Clock` 协议注入骨架
9. P2/P6/P9/P10 随 Feature Story 落地
10. P11/P12 Test infra 随 Story 0/1 立

**Cross-Component Dependencies**：
- P3 WSHub 依赖 P13 Clock（重连退避）+ P7 Log
- P7 LogSink 依赖无（独立）
- P5 Fail 决策表由 P8 AC 引用
- P4 fireAndForget 依赖 P3 WSHub

---

## Project Structure & Boundaries

### Requirements → Architecture 映射

基于 PRD 35 个 Capability ID + UX v0.3 组件清单 + Step 4/5 决策，映射到 target / 目录：

| Capability | 所属 Feature / Module | 关键组件 |
|---|---|---|
| `C-ONB-*` + `C-UX-01/02` | `Features/Onboarding/` | NarrativeCardCarousel, WatchModePicker, TearCeremonyView, CatGazeAnimation, CatNamingView |
| `C-ME-01` + `C-SOC-01/02` | `Features/Account/` + `CatShared/UI/MyCatCard` | AccountTabView, EmojiPicker (Inline/Sheet), CatMicroReactionView, FirstEmojiTooltip |
| `C-SOC-03` + `C-UX-05` | `Features/Friends/` | FriendListView, FriendRequestBanner, InviteQRSheet |
| `C-BOX-*` + `C-SKIN-*` | `Features/Inventory/` | InventoryTabView, SkinCardGrid/Card/DetailSheet, BoxPendingCard, **BoxUnlockedPendingRevealCard**（A5'）, MaterialCounter, CraftConfirmSheet |
| `C-DRESS-01/02` | `Features/Dressup/` | DressupTabView, SkinGalleryPicker, DressupPreview |
| `C-ROOM-01/02/05` | `Features/Room/` | RoomTabView, RoomMemberList, RoomStoryFeed, RoomInviteButton |
| `C-STEPS-*` | `CatCore/Health/` + Feature 消费 | `StepDataSource`, `HealthKitAuthorization` |
| `C-IDLE-*` | Watch 独占 · iPhone 仅接收 state | — |
| `C-REC-*` | `CatCore` + Feature + `CatShared/UI/GentleFailView` | `docs/fail-closed-policy.md` 锚点 |
| `C-SYNC-01/02` | `CatShared/Networking` + `CatCore/WS` | `WSHub`, `WSRegistry`, `Envelope` |
| `C-UX-03/04` | `CatShared/UI` + Feature 组装 | EmptyStoryPrompt, StoryLine, SettingsView |
| `C-OPS-04/05/06` (MVP) | `CatShared/Utilities/Log.swift` + `CatCore/Milestones` + `Resources/PrivacyInfo.xcprivacy` | LogSink/OSLogSink/RingBufferJSONLSink |
| `C-OPS-01/02/03` (Growth) | 推延 Growth epic | — |

---

### Complete Project Tree（综合 Party Mode 升级版 · 新增 CatTestSupport + EventBus + "故意不做"块）

```
/Users/zhuming/fork/catc/                           # mono-repo root
├── CLAUDE.md                                        # 根工程宪法（共享 3 端）
├── docs/                                            # 跨端共享文档
│   ├── api/                                         # 契约 SSoT
│   │   ├── openapi.yaml                             # 当前 0.14.0-epic0
│   │   ├── ws-message-registry.md                   # v1
│   │   └── integration-mvp-client-guide.md
│   ├── contract-drift-register.md                   # 7 项跨仓漂移登记（Step 3 C3）
│   ├── fail-closed-policy.md                        # Fail 决策表 · 带行号锚点（P5）
│   ├── patterns/
│   │   └── README.md                                # Pattern 索引表（含"我们故意不做什么"块 · Paige I1 + Dr. Quinn 负空间哲学）
│   ├── dev/
│   │   ├── onboarding.md                            # 新 dev 15 分钟跑通
│   │   └── troubleshooting.md                       # XcodeGen / 签名 / 模拟器常见错误
│   └── map.md                                       # 文档地图（按角色分叉）
├── server/                                          # Go 后端（独立 repo · 不在架构范围）
├── ios/                                             # iOS mono-repo 根
│   ├── README.md                                    # 玄关（≤ 100 行 · 含"我们故意不做什么"块）
│   ├── project.yml                                  # XcodeGen SSoT
│   ├── .gitignore                                   # 忽略 Cat.xcodeproj/
│   ├── .swiftlint.yml                               # custom rules（P4 fireAndForget 守门）
│   ├── scripts/
│   │   ├── build.sh                                 # xcodegen → xcodebuild → swift-format → 零 warning
│   │   ├── install-hooks.sh                         # .git/hooks/pre-push 安装
│   │   ├── check-ws-fixtures.sh                     # dev-time WS fixture SHA diff（不进 CI）
│   │   ├── check-test-mirror.sh                     # Tests/Unit/<F>/ ↔ App/<F>/ 镜像校验（P12）
│   │   └── git-hooks/pre-push                       # 触发 build.sh
│   ├── CatShared/                                   # SwiftPM Package · 跨端原子能力
│   │   ├── Package.swift
│   │   ├── Package.resolved
│   │   └── Sources/
│   │       ├── CatShared/                           # 跨端共享（iPhone + Watch）
│   │       │   ├── README.md                        # "只放 wire 契约" 5 行硬约束（取代 ADR-002）
│   │       │   ├── Models/ (Skin/Box/WSPush/...)
│   │       │   ├── Networking/
│   │       │   │   ├── WSEnvelope.swift
│   │       │   │   ├── WSClient.swift (protocol) + WSClientEmpty/URLSession
│   │       │   │   ├── WSRegistry.swift
│   │       │   │   ├── WSMessageTypes.swift         # 静态字符串常量（P3）
│   │       │   │   ├── APIClient.swift + APIClientEmpty/URLSession
│   │       │   │   ├── AuthTokenProvider.swift
│   │       │   │   ├── ServerError.swift
│   │       │   │   └── ServerErrorCategory.swift    # 2 类重试（P6 MVP）
│   │       │   ├── Persistence/
│   │       │   │   ├── LocalStore.swift (+ Empty/SwiftData)
│   │       │   │   ├── KeychainStore.swift
│   │       │   │   └── KeyValueStore.swift
│   │       │   ├── Configuration/Timeouts.swift
│   │       │   ├── Resources/
│   │       │   │   ├── DesignTokens.swift
│   │       │   │   └── Animations/cat_gaze_onboarding.json
│   │       │   ├── UI/                              # 跨端可复用 View
│   │       │   │   ├── MyCatCard.swift
│   │       │   │   ├── SkinRarityBadge.swift
│   │       │   │   ├── StoryLine.swift
│   │       │   │   ├── EmptyStoryPrompt.swift
│   │       │   │   ├── GentleFailView.swift
│   │       │   │   ├── EmojiEmitView.swift
│   │       │   │   └── LottieView.swift
│   │       │   └── Utilities/
│   │       │       ├── Log.swift (3×3 facade)
│   │       │       ├── LogSink.swift + OSLogSink + RingBufferJSONLSink + InMemoryLogSink
│   │       │       ├── Clock.swift                  # P13 · any Clock<Duration>
│   │       │       ├── IdleTimerProtocol.swift
│   │       │       └── AccessibilityAnnouncer.swift
│   │       └── CatCore/                             # 业务编排（依赖 CatShared）
│   │           ├── AppCoordinator.swift             # 根 Environment 注入点
│   │           ├── EventBus.swift                   # ★新增 · 跨 Feature 事件总线（Winston）
│   │           ├── DomainEvent.swift                # ★新增 · 类型化 enum · 禁 Any
│   │           ├── Auth/
│   │           │   ├── AuthSession.swift            # @Observable 全局 Store
│   │           │   └── AuthService.swift (protocol)
│   │           ├── WS/
│   │           │   ├── WSConnection.swift           # @Observable 全局 Store · 重连
│   │           │   ├── WSHub.swift                  # actor 多路广播（P3 + O1）
│   │           │   └── FireAndForget.swift          # wrapper（P4）
│   │           ├── Health/
│   │           │   ├── HealthKitAuthorization.swift # @Observable 全局 Store
│   │           │   ├── StepDataSource.swift (protocol + Empty/HealthKit actor)
│   │           │   └── FakeStepDataSource.swift
│   │           ├── Watch/
│   │           │   ├── WatchTransport.swift (protocol)
│   │           │   ├── WatchTransportNoop.swift     # MVP 唯一实现（D5）
│   │           │   └── FakeWatchTransport.swift
│   │           ├── Room/
│   │           │   ├── CurrentRoom.swift            # @Observable 全局 Store
│   │           │   └── RoomService.swift            # protocol（query/command only · 禁 delegate · Winston）
│   │           ├── Box/
│   │           │   ├── UnlockService.swift
│   │           │   └── BoxStateStore.swift
│   │           └── Milestones/
│   │               ├── FirstTimeExperienceTracker.swift (protocol · S-SRV-15)
│   │               └── UserMilestonesService.swift
│   ├── CatPhone/                                    # iPhone app target
│   │   ├── App/CatPhoneApp.swift                    # @main · 注入全局 Store + EventBus
│   │   ├── Features/                                # Feature 模块化（禁互 import · P9）
│   │   │   ├── Onboarding/ (View + Store + 5 子 View)
│   │   │   ├── Account/ (View + Store + EmojiPicker × 2 + LayoutPolicy + CatMicroReactionView + FirstEmojiTooltip + TooltipCounter)
│   │   │   ├── Friends/ (View + Store + 3 子 View)
│   │   │   ├── Inventory/ (View + Store + 8 子 View 含 BoxUnlockedPendingRevealCard)
│   │   │   ├── Dressup/ (View + Store + 2 子 View)
│   │   │   └── Room/ (View + Store + 3 子 View)
│   │   ├── Common/                                  # Feature 间共用（非跨端）
│   │   │   ├── SkeletonView.swift
│   │   │   ├── HintLabel.swift
│   │   │   ├── GentleConfirmSheet.swift
│   │   │   └── PulseEffect.swift
│   │   └── Resources/
│   │       ├── Info.plist (HealthKit / Camera UsageDescription)
│   │       ├── PrivacyInfo.xcprivacy                # C-OPS-05 · 必进 MVP
│   │       └── Assets.xcassets/                     # 按 Feature 分组
│   ├── CatWatch/                                    # watchOS target（独立实施）
│   ├── CatTestSupport/                              # ★新增 · test-only framework target（Murat）
│   │   ├── UserDefaultsSandbox.swift
│   │   ├── WSFakeServer.swift                       # envelope-agnostic
│   │   ├── HapticSpy.swift
│   │   ├── DualSSoTScenarioFixture.swift
│   │   └── TestClock.swift (Swift 标准库 TestClock 扩展)
│   │   # Build settings 硬约束：只能被 test target 依赖 · 严禁进 App/Core/Shared
│   ├── CatSharedTests/                              # @testable import CatShared + import CatTestSupport
│   │   ├── WSEnvelopeTests.swift
│   │   ├── WSRegistryTests.swift
│   │   ├── ServerErrorCategoryTests.swift
│   │   └── LogSinkTests.swift
│   ├── CatCoreTests/                                # @testable import CatCore + CatTestSupport
│   │   ├── WSHubTests.swift                         # 多路广播 + 所有 type 有订阅者
│   │   ├── EventBusTests.swift                      # ★新增
│   │   ├── AuthSessionTests.swift
│   │   ├── HealthKitStepDataSourceTests.swift       # TestClock 注入
│   │   └── FirstTimeExperienceTrackerTests.swift
│   ├── CatPhoneTests/                               # @testable import CatPhone + CatTestSupport
│   │   ├── Unit/                                    # P12 镜像 Features/（≥ 2 test file 才开子目录）
│   │   │   ├── Onboarding/
│   │   │   ├── Account/
│   │   │   ├── ...
│   │   └── Integration/DualSSoT/
│   │       └── StepUnlockWhenWSArrivesBeforeHealthKitTests.swift (场景化命名)
│   │   # Fixture/ 已搬迁至 CatTestSupport · 此处不再保留
│   └── CatPhoneUITests/                             # E9 · Epic 9 落地 · Epic 1-8 用 Snapshot test 兜底
│       └── SmokeFlowTests.swift
├── _bmad/                                           # BMad workflow 配置
│   ├── templates/story-ac-triplet.md                # Fail AC 三元组（P8）
│   └── lint/ac-triplet-check.sh                     # P8 lint
└── .github/
    ├── workflows/ios.yml                            # Epic 9 CI
    └── pull_request_template.md                     # P10 · 6 勾选项（新增 CatShared/UI 归属检验）
```

---

### Architectural Boundaries

**Target / Package 依赖单向图**：

```
CatPhone ─────▶ CatCore ─────▶ CatShared
CatWatch ─────▶ CatCore ─────▶ CatShared
CatPhoneTests / CatCoreTests / CatSharedTests ─────▶ CatTestSupport（test-only · App 不可依赖）

禁止：
  CatShared     ▶ CatCore       (✗)
  CatCore       ▶ CatPhone/Watch (✗)
  Feature A     ▶ Feature B     (✗)  走 CatCore/<X>Service protocol 或 EventBus
  App/Core/Shared ▶ CatTestSupport (✗)  严禁 fixture 泄漏到生产
```

**网络边界**：

- **HTTP**：MVP 仅 `GET /v1/platform/ws-registry`（启动必调 · fail-closed）· 未来 `/v1/auth/*` / `/v1/user/milestones` / 其他业务端点全部待 server 实装
- **WS**：iPhone ↔ server 单 `URLSessionWebSocketTask` + envelope · 30s/60s ping/pong 自动 · 300s dedup TTL · 无 seq/last_ack_seq
- **WatchConnectivity**：MVP `WatchTransportNoop`（binary 不链 WCSession）· Growth 才有 fast-path

**状态边界**：

- 全局 state（4 `@Observable` Store + EventBus in `CatCore`）→ `AppCoordinator` 根注入
- Feature state 由 Feature `@Observable Store` 自持 · 跨 Feature 通过 `CatCore` service protocol（query/command）或 `EventBus`（事件广播）
- wire-level 契约只放 `CatShared`

**CatCore Service protocol 反腐化约束**（Winston 方案 A）：

- Service protocol 只暴露 `func` 形式 query/command（同步或 async）
- **禁**挂 delegate 回调 · **禁**把 Feature 状态刷新通过 Service 传递
- 跨 Feature 事件走 `CatCore/EventBus.swift` · 类型化 `enum DomainEvent` · enum 变更强制全局 review

---

### Epic 1 Story 切分（升级版 · 9 → 8 Story · 关键路径深度 9 → 6）

基于 Amelia 的拆分提案（1.3 WS 拆 3 子 · 1.6 Clock 并入 1.3b · 原 1.9 合进 1.3c），标注并行组：

| Story | 范围 | 并行组 | 依赖 |
|---|---|---|---|
| **1.1** | XcodeGen Bootstrap（AC1-5 · C1/C2/C3 基础设施 · CatTestSupport target 创建） | 串行起点 | — |
| **1.2** | 全局 Store 骨架（AuthSession / WSConnection / CurrentRoom / HealthKitAuthorization）+ **EventBus + DomainEvent 骨架** + AppCoordinator | 串行 | 1.1 |
| **1.3a** | WSEnvelope + WSMessageTypes（双 gate 数据层 · 4-5 AC） | 串行 | 1.2 |
| **1.4** | FireAndForget wrapper + SwiftLint custom rule | **并行组 A** | 1.3a |
| **1.5** | LogSink + OSLogSink + RingBufferJSONLSink + 崩后首启上报 | **并行组 A** | 1.2 |
| **1.7** | DesignTokens + LottieView 骨架 | **并行组 A** | 1.1 |
| **1.3b** | WSHub（多路广播 · actor · 连接 / 重连 / 心跳）+ Clock + Timeouts（合并原 1.6） | 串行 | 1.3a |
| **1.3c** | WSRegistry fail-closed（合并原 1.9 · 与 Hub dispatch 一体） | 串行 | 1.3b |
| **1.8** | Onboarding UI（C-UX-01/02 + C-ONB-*） | 串行末 | 1.2, 1.3c, 1.7 |

**Hidden bottleneck**（Winston）：1.3b WSHub 必须一人专注做完（FireAndForget 语义踩坑靠连续思考，禁切两人）· 1.3a/1.3b/1.3c 是 Party Mode 命脉的串行干道。

---

### Test 组织细化（P11 + P12 补）

**P12 目录镜像规则**：
- `Tests/Unit/<F>/` ↔ `App/Features/<F>/`（目录层镜像）
- **不**强制源文件 1:1 mapping
- Test class 按"**用户能观察到的能力**"切（非 type 切）· 每个 `test_*` 函数名 = AC 描述
- **豁免**：单文件 Feature 不开子目录，直接 `Tests/Unit/<F>Tests.swift` · **≥ 2 个测试文件**才开子目录

**Integration vs Unit 边界规则**（Murat 二选一判定）：
- 跨 **2 个及以上 SSoT/Provider**（HealthKit + WS / WC + Server / Keychain + UserDefaults）→ `Tests/Integration/`
- 验证 **"时序/顺序"** 而非"单点输出" → `Tests/Integration/`
- 否则归 `Tests/Unit/<F>/`

**命名反模式**：`ScenarioTests.swift`（"Scenario"不是信息）· **推荐**：`StepUnlockWhenWSArrivesBeforeHealthKitTests.swift`（场景化 · 一眼看出在测什么时序）

**示例 · Onboarding Feature 的 Test class 切法**（Amelia）：

```swift
// CatPhoneTests/Unit/Onboarding/OnboardingFlowTests.swift
func test_新用户首次启动_进入引导第一页()
func test_已登录用户_跳过引导直达房间()
func test_引导中途杀进程_重启恢复到同一步()

// CatPhoneTests/Unit/Onboarding/OnboardingAuthTests.swift
func test_SIWA登录成功_token持久化()
func test_SIWA登录失败_展示错误且可重试()

// CatPhoneTests/Unit/Onboarding/OnboardingNetworkFailureTests.swift (§21.3 fail-closed)
func test_注册接口502_不推进到下一步()
func test_WS首连失败_不进入主功能()
```

---

### Epic 1-8 UI Regression 兜底清单（Murat No-Go 转 Go 硬条件）

UITest 推延 Epic 9 期间，Epic 1-8 **必须落地**以下兜底，缺任一项则 Murat 投 No-Go：

| # | 兜底项 | Epic 1-8 覆盖 | 必需度 | 落地 Story |
|---|---|---|---|---|
| 1 | **Snapshot test**（SwiftUI View → PNG diff · flakiness ≈ 0） | UI regression 替代 UITest | **P0** | Story 1.7 尾部引入 |
| 2 | **ViewModel unit test 100% state 转换覆盖** | Feature Store 交互逻辑 | **P0** | 每 Feature Story |
| 3 | **WSHub 合同测**（所有 msg type 必有订阅者 + 反向 assert） | 协议漂移 | **P0** | Story 1.3b |
| 4 | **DualSSoTScenarioFixture 集成测** | HealthKit + WS 时序 | **P0** | Story 1.3b 后立 |
| 5 | **Accessibility smoke**（XCTest `app.snapshot()` 静态抓取） | A11y 底线 | **P1** | Epic 1 尾 |
| 6 | **每 Story DoD 加"手动 regression 脚本 update"** | 人肉兜底可追溯 | **P1** | Story 模板 |

**Snapshot test 工具**：默认 swift-snapshot-testing（Point-Free）**暂不引入**（违反"禁三方"），用 XCTest `XCTAttachment` + `CIImage` diff 自建轻量 snapshot helper in `CatTestSupport/`（Story 1.7 交付）。

---

### 关键领域代码地图（4-box 图 · Amelia · 防新人"跳 4 目录靠猜"）

**HealthKit 领域**（新人要改 HealthKit 相关时，只看这张图就知道跳哪 4 个目录）：

```
┌─ Protocol 层                              ─┐   ┌─ 依赖基础设施                     ─┐
│  CatCore/Health/StepDataSource.swift        │   │  CatShared/Utilities/Clock.swift    │
│  CatCore/Health/HealthKitAuthorization     │   │  CatShared/Configuration/Timeouts   │
│  ↑ 6 Feature 消费 · 全局唯一抽象             │   │  ↑ 10+ 消费者共用（WS / 日志时间戳） │
└──────────────────────────────────────────┘   └────────────────────────────────────┘
             │                                              │
             ▼                                              ▼
┌─ 实现层                                   ─┐   ┌─ 测试基础设施                     ─┐
│  CatCore/Health/HealthKitStepDataSource    │   │  CatTestSupport/FakeStepDataSource  │
│  (actor · 15min cache · HK queries)        │   │  CatTestSupport/TestClock            │
│                                             │   │  CatCoreTests/HealthKitStepDataSource│
└──────────────────────────────────────────┘   └────────────────────────────────────┘
```

**WS 领域**：

```
CatShared/Networking/WSEnvelope + WSClient + WSRegistry + WSMessageTypes
                    │
                    ▼
CatCore/WS/WSHub (actor 多路广播) + FireAndForget + WSConnection (@Observable)
                    │
                    ▼
Feature Store `for await wsHub.subscribe(type)`
                    │
                    ▼
CatTestSupport/WSFakeServer (测试)  +  CatCoreTests/WSHubTests (多路广播断言)
```

**Auth 领域**：

```
CatShared/Networking/AuthTokenProvider + KeychainStore
                    │
                    ▼
CatCore/Auth/AuthService (protocol) + AuthSession (@Observable 全局 Store)
                    │
                    ▼
Features/Onboarding + Features/Account 消费
                    │
                    ▼
CatTestSupport/FakeAuthService (测试)
```

---

### 负空间哲学块（Dr. Quinn · 必须写进 README 玄关 + Pattern 索引）

`ios/README.md` 和 `docs/patterns/README.md` **必须包含**以下"**我们故意不做什么**"块，让新人从"不做清单"反推产品哲学：

```markdown
## 我们故意不做什么（from day 1）

- **我们不显示"已读 / 已送达"** — fire-and-forget 对称性（P4）· 社交信号无身份、无历史、无义务（创新 #2 触觉广播 · 反 IM 范式）
- **我们不镜像 Watch 的 UI 到 iPhone** — 哲学 B（D5）· iPhone 是"慢房间"，Watch 是"剧场"；iPhone 故意不渲染猫动画、不剧透盲盒、不做实时挂机
- **我们不用情绪机（hungry/sad/happy）** — Round 5 决定移除 · 只做 walk/sleep/idle 物理动作同步（创新 #1 温柔门槛 · 反 Tamagotchi guilt）
- **我们不让 iPhone 判定盲盒解锁** — 双 SSoT 分工（D4）· HealthKit 展示 / server 权威解锁 · 防作弊 + 反事实源漂移
- **我们不给观察者 raw step count** — 隐私 gate（`C-SOC-01`）· 对等权限、降级尊重
- **我们不签到、不连胜、不 FOMO** — 违反创新 #1 温柔门槛

**为什么写在这里？** 因为项目结构 tree 讲得清"我们建了什么"，讲不清"我们拒绝了什么"。后者是温柔门槛 / 触觉广播 / Watch 反转这三个创新假设的灵魂。
```

---

### Integration Points

**内部通信**：

- Feature Store → `CatCore` service protocol（query/command）or EventBus → 实际 impl
- CatCore service → CatShared atom（APIClient / WSClient / LocalStore）
- WS push → `WSHub.publish()` → fan-out `AsyncStream<WSPush>` → Feature Store `for await` 过滤 type
- 跨 Feature 事件 → `CatCore/EventBus.swift` 发 `enum DomainEvent` → Feature Store 订阅感兴趣的 case

**外部集成**：

- HealthKit（actor 隔离 · 15min cache）· APNs（分频道推送）· SIWA（nonce 校验）· Keychain · SpriteKit + Spine（仅 Watch）· Lottie（仅 iPhone）

**数据流**：

```
用户动作 → Feature View → Feature Store (@Observable)
  ├─▶ CatCore service protocol → global Store / CatShared atom → server HTTP/WS
  ├─▶ EventBus → 其他 Feature Store 订阅
  └─▶ Local-only UI 反馈（fire-and-forget）
```

---

### Development Workflow Integration

- `bash ios/scripts/build.sh [--test]` 一键 `xcodegen → xcodebuild build → swift-format lint --strict → xcodebuild test → 零 warning`
- `.git/hooks/pre-push` 自动触发；不绿 reject push（`install-hooks.sh` 安装）
- `check-test-mirror.sh` 新 Feature 无对应测试目录 PR reject（P12）
- `check-ws-fixtures.sh` dev-time WS fixture 漂移对比（不进 CI · 防跨仓节奏绑架 · Pre-mortem P5 教训）
- `.github/workflows/ios.yml` macOS runner 跑 CI（Epic 9 落地）
- SwiftLint custom rules 守 P4 fireAndForget 直调；Growth 补 P9 Feature 互 import

---

### Long-term Boundary Stewardship（Winston · CatShared/UI 腐化兜底）

Step 4 Pre-mortem P3 教训：CatShared 易腐化为"垃圾桶"。Epic 3+（Inventory 真正跨端时）重新 review 本边界；Epic 1-2 阶段以 PR checklist 第 6 条守门（P10 新增）：

**每次往 `CatShared/UI/` 加组件的 PR 必须在 description 回答**：
- 这个组件 Watch 会用吗？哪个 Watch Feature / Story？
- 如果只是"将来可能用"（未立 Story），**退回 `CatPhone/Common/`**，等 Watch Feature 要用时再迁移

**checkpoint**：每次 `CatShared/UI/` 新增组件时，reviewer 必须核对 Watch 侧至少有 1 个使用点（或即将有）。无 Watch 使用证据 → reject → 放 `CatPhone/Common/`

---

## Architecture Validation Results

_经 Step 7 Party Mode（Mary / John / Paige / Winston 四位 agent 多维度审视）升级后的版本_

### Coherence Validation ✅

6 条一致性检查全部通过：

| # | 检查项 | 状态 |
|---|---|---|
| C1 | Step 4 决策 D1-D7 与 Step 5 Patterns 互洽 | ✅ |
| C2 | Step 5 Patterns 与 Step 6 Structure 互洽 | ✅ |
| C3 | Party Mode O1/O6 补丁与 D2/D7 原决策无冲突 | ✅ |
| C4 | Step 6 Epic 1 Story 序与 Pattern 依赖图互洽 | ✅ |
| C5 | CatTestSupport 独立 target 与 SwiftPM 依赖方向单向 | ✅ |
| C6 | P5 Fail 决策表锚点行号与 P8 AC triplet 格式互洽 | ✅ |

**已识别决策张力**（不阻塞 · 观察窗口）：D3 默认 `@MainActor` vs Party Mode O2 建议 Provider 默认非 MainActor——用户选 Amelia 立场，保留原 D3。**触发重评条件**：首次出现 `@unchecked Sendable` 未归档 / 主线程阻塞 issue。

### Requirements Coverage Validation

**Functional Requirements（63 FR）**：第一跳映射全部覆盖（见 Step 6 "Requirements → Architecture 映射"表）· **但 Mary 抽样追溯发现 5 条里 3.5 条末端未闭环**（FR → Capability → Structure → Epic → Story → AC → Pattern）。见下方 "FR → AC 末端追溯表（附录）"。

**Capability ID（35 项）**：全部映射 Feature / Module · `C-OPS-01/02/03` 显式推延 Growth。

**Non-Functional Requirements（8 类）**：全部承载于 Already Decided / D1-D7 / P1-P13（见 Step 6 表）。

**User Journeys（7 条）**：由 Capability 组合支持 · J6 Watch 独占显式标注 · **J1/J2 happy path 依赖 server S-SRV-15 实装（G1）**。

### Implementation Readiness Validation

- Decision 完整度：D1-D7 文档化 + 版本明确 + Pre-mortem P4 冲突由 D3 三方案混用解决
- Structure 完整度：Complete tree + Epic 1 8-Story 切分 + 并行组标注
- Pattern 完整度：13 Pattern + 2 基础设施 + Growth 延迟清单
- Test 基础设施：CatTestSupport 独立 target + Fixture 三件套 + DualSSoTScenarioFixture + TestClock + 6 测试抽象 protocol
- 契约消费可容错：WSRegistry 运行时发现 · 未知 type 优雅降级

---

### Critical Gaps（12 条 · G1-G6 原版 + G7-G12 Party Mode 新增）

| # | Gap | 影响 | 处置 | 来源 |
|---|---|---|---|---|
| G1 | server `S-SRV-15~18` 四个需求零实现（backlog 无痕迹） | Epic 1 Onboarding 阻塞（user_milestones 硬依赖） | 跨仓 sync 锁定 epic 归属 + 交付时间线 | Step 2 调研 |
| G2 | SIWA / JWT refresh server 未实装 | Story 1.8 只能用 mock token 联调 | 真实 SIWA 联调推 Epic 9 | Step 2 调研 |
| G3 | 崩后 JSONL 上报端点 `POST /v1/platform/crash-reports` 未定义 | MVP 崩后证据链不完整 | 新增 CD-08 contract drift · MVP 可先本地 RingBufferJSONL · 跨仓协调 | Step 5/7 |
| G4 | O2 `@unchecked Sendable` swiftlint custom rule 未引入 | 跨 actor race bug 难排查 | **观察窗口**：首次出现 race issue 或代码中出现 `@unchecked Sendable` 未归档时重评 | Step 4 |
| G5 | `check-test-mirror.sh` / `check-ws-fixtures.sh` 脚本内容未定义 | PR gate 未落实 | Epic 1 Story 1.1 交付 **v0.1 stub**（`echo "TODO: wire after 1.3" && exit 0`）· Story 1.4 retro 填实 | Step 6 / Paige |
| G6 | `contract-drift-register.md` 跨仓维护权未约定 | 漂移登记形式化易死亡 | **顶部加 `## 维护权（TBD · 见 issue #XX）` 横幅** · 显式化 > 假装解决 · Epic 1 retro 前解决 | Step 7 / Paige |
| **G7** | **降级路径 Structure 所有权 + 跨端版本协商协议**（iPhone/Watch/Server 三端独立发版，WS msg registry 加字段谁兼容谁；server/Watch 不可达 iPhone 降级分支 Structure 所有权未定） | Epic 2+ 必踩 · FR7/FR19 末端未闭环由此而起 | **Epic 1 Sprint 1 补 2 天 spike**（Winston 硬门槛 2） · 不做完不进 Epic 2 | Mary + Winston |
| **G8** | iPhone 端可观测性契约未定义（S-SRV-18 只覆盖 server metric · iPhone "哪些事件必须埋点 / schema 谁 own" 未定） | 用户投诉时无客户端证据 | Epic 1 Story 1.5 `LogSink` 落地时**同步定义 iPhone metric schema** · 补 Pattern P14 | Winston |
| **G9** | Server 延期 contingency plan 缺失（S-SRV-15 不按时 → iPhone Story 回滚路径未定） | iOS Epic 1 critical path 断链风险 | **每条 server 依赖 Story 必须有"server 延期 N 周"降级方案**（如 UserDefaults stub 占位 · 后续迁移 plan 文档化） | Mary |
| **G10** | Legal / Privacy 未过审（`PrivacyInfo.xcprivacy` / `user_milestones` PII / 崩后上报） | 合规风险 · App Store reject | **MVP 发布前必补 Legal sync 会议**（1 次 30 min · 归 Epic 9 前置） | Mary |
| **G11** | 真实 alpha 用户反馈 = 0（7 agent 全 LLM） | J6 Watch 独占 / 降级揭晓情绪体验无真人验证 | **Pre-MVP 2-3 位 alpha tester journey walkthrough**（PRD `openQuestionsForVision` "validation evidence debt" 已登记 · 本处 pin 时间 = Epic 5 收尾前） | Mary |
| **G12** | 跨仓文档可见性（`architecture.md` 住 `ios/CatPhone/_bmad-output/planning-artifacts/` 深层目录 · server/watch dev clone 看不到） | 架构文档对跨端 stakeholder 等于不存在 | **repo 根 `docs/architectures/README.md`** 索引列三端架构链接 · 成本 10 min · 推荐方案（非 symlink · 非 CI 镜像） | Paige |

---

### FR → AC 末端追溯表（附录 · Mary 抽样发现 3.5/5 断链）

修补 4 条随机抽样发现的末端断链：

| FR | Capability | Structure 承载 | Epic 1 Story | AC ID（待 Epic 1 创建时锁定） | Pattern 引用 | 断链状态 |
|---|---|---|---|---|---|---|
| **FR7**（Watch 不可达 iPhone 本地降级揭晓） | `C-BOX-04` + A5' | `Features/Inventory/BoxUnlockedPendingRevealCard.swift` + `CatCore/Box/BoxStateStore` | **待 Epic 3 Inventory Story** · Epic 1 仅提供 protocol 骨架 | TBD | P3（WSHub 订阅 `box.unlock.revealed`）+ P5 fail-closed-policy.md#Lx（降级分支） | **已补锚点** |
| **FR19**（emote fire-and-forget · 发送方无 ack UI） | `C-SOC-01/02` · `S-SRV-17` | `CatPhone/Features/Account/AccountStore.sendEmote()` + `CatCore/WS/FireAndForget.swift` | **待 Epic 2 Account Story** | TBD · **AC 必含 negative assertion**：`test_sendEmote_neverExpectsDeliveredEvent()` | P4 FireAndForget + SwiftLint custom rule | **已补 negative assertion 要求** |
| **FR33**（paired 状态 UI 显示） | `C-ONB-02` | **跨 Feature 所有权**：`CatShared/UI/PairedStateIndicator.swift`（新增）· 消费方 `Features/Onboarding/` + `Features/Account/` | Epic 1 Story 1.8 Onboarding UI | TBD | P1 Store 命名 + P10 PR Checklist "CatShared/UI 归属检验" | **已定所有权** |
| **FR48**（`user_milestones` 账号级持久化） | `C-ONB-01` · S-SRV-15 硬依赖 | `CatCore/Milestones/FirstTimeExperienceTracker.swift` + `UserMilestonesService.swift` | Epic 1 Story 1.8（阻塞 G1） | TBD + **G9 contingency：server 延期 2+ 周则临时降级 UserDefaults stub，后续迁移 plan 记档** | P7 Log facade + P8 AC triplet `fail_policy` | **已补 contingency** |
| FR58（崩溃上报） | `C-OPS-01` | `CatShared/Utilities/RingBufferJSONLSink.swift` + 崩后首启上报 | Story 1.5 | TBD | P7 LogSink + O6 | ✅ 原本就通 |

**结论**：抽样 5 条原断 3.5 条 · 补 4 条锚点后**末端闭环**。**建议 Epic 1 kickoff 前对全 63 FR 跑一次末端追溯表**（batch · 2 小时工作）· 可由架构师或 PM 轮值。

---

### MVP TestFlight 真阻塞清单（产品执行视角 · John 补）

**不是 iPhone 架构 Gap · 是跨端产品执行 P0 阻塞**（iPhone 架构 READY 也不代表 MVP 可发）：

| P0 | 阻塞项 | 影响 | Owner |
|---|---|---|---|
| P0-A | **server 业务消息 Release 模式为空** | iPhone + Watch 做完 Epic 1-5，无真消息可广播 → **3 个创新假设（温柔门槛 / 触觉广播 / Watch 反转）零验证** | server 团队 |
| P0-B | **Spine animator 档期未锁** | 猫动画质量决定"温柔门槛"情感验证成败 · 无 animator 档期 = MVP 是 PPT | PM + 设计团队 |
| P0-C | **Watch 反转交互真机体验未验证**（归 Epic 9） | 假设 3（Watch 反转配件关系）崩塌风险 · Epic 9 太晚暴露 | Watch 团队 + PM |

**建议**：MVP 发布 go/no-go 会议必须把 P0-A/B/C 列为 definition of done，iPhone 架构 READY 不代表 MVP READY。

---

### Cross-Repo Sync Action Items（John 精简版 · 60 min · 3 议题）

| # | 议题 | 时长 | 输出 |
|---|---|---|---|
| 1 | **S-SRV-15 `user_milestones` 排期** | 20 min | server epic 归属 + 交付日期（阻塞 iPhone Story 1.8） |
| 2 | **SIWA + JWT refresh 实装节奏** | 20 min | 定 Story 1.8 unblock 日期（解 G2） |
| 3 | **`contract-drift-register.md` 维护权 + S-SRV-16/17/18 epic 归属** | 20 min | 定 owner + 更新流程 · 解 G6 + 部分 G1 |

**砍掉**（降级为 iPhone 团队内决 / 延 Growth）：
- G3 crash-reports（MVP 可先本地存）
- G4 swiftlint（iPhone 内部）
- G5 build 脚本（Story 1.1 内解决）
- G7-G12 新增项（除 G12 跨仓可见性归本会议 5 min 快决定）
- 其他原 Step 7 10 议题

---

### Winston 3 条硬门槛（Go 的前置条件）

1. **`_bmad/tech-debt-registry.md` 必须建**（Epic 1 Story 1.1 内交付骨架）
   - 8 条延迟项登记：Error 4 类（P6 Growth）· Log 6×5（P7 Growth）· Feature 互 import lint（P9 Growth）· O2 swiftlint · O4 `#if WATCH_TRANSPORT_ENABLED`+linker check · O5 PhaseAnimator 三栈 · C-OPS-01/02/03
   - 每条字段：**触发条件 + owner + 补偿 deadline**
   - **无此表 = No-Go**

2. **Epic 1 Sprint 1 内补 G7 version negotiation spike**（2 天）
   - 产出：三端 WS msg registry 加字段时的 version handshake 草案 + 降级路径 Structure 所有权约定
   - 不做完不进 Epic 2

3. **哲学 B 下重跑 D5 WatchTransport 风险评估**（half-day）
   - 重新验证"server 不可达 + Watch 不可达双故障"下 iPhone UX（Step 4 D5 Go-with-conditions 基于哲学 A 风险模型 · 哲学 B 下未重评）
   - 结论写入 architecture.md §23（待 Epic 1 Sprint 1 追加）

---

### Architecture Completeness Checklist

**✅ Requirements Analysis**
- [x] Project context 深度分析
- [x] Scale and complexity 评估
- [x] Technical constraints 识别
- [x] Cross-cutting concerns 映射

**✅ Architectural Decisions**
- [x] Critical 决策文档化
- [x] Technology stack 完全锁定
- [x] Integration patterns 定义
- [x] Performance 考虑

**✅ Implementation Patterns**
- [x] Naming conventions 建立
- [x] Structure patterns 定义
- [x] Communication patterns 定义
- [x] Process patterns 定义

**✅ Project Structure**
- [x] Complete directory structure
- [x] Component boundaries 建立
- [x] Integration points 映射
- [x] Requirements → Structure 映射

**✅ Validation & Handoff**
- [x] Coherence 验证 6 条通过
- [x] Requirements Coverage 63 FR + 35 Capability + 8 NFR 第一跳全覆盖
- [x] 12 Critical Gaps 显式标注（G1-G12）
- [x] FR → AC 末端追溯表（4 条抽样补闭环）
- [x] Cross-repo sync 3 议题精简
- [x] Winston 3 硬门槛
- [x] MVP TestFlight 真阻塞清单（P0-A/B/C）

---

### Architecture Readiness Assessment（修订版）

**Overall Status**：**CONDITIONALLY READY FOR STORY 1.1 INCEPTION**（非 "READY FOR IMPLEMENTATION"）

**Confidence Level**：**MEDIUM-HIGH**（原自评 HIGH · 经 Party Mode 四人一致判过度乐观后降级）

**Epic 1 启动策略（John 收敛）**：
- **可立即启动**：Story 1.1 XcodeGen Bootstrap · Story 1.2 全局 Store + EventBus · Story 1.4 FireAndForget + SwiftLint custom（均不依赖 server 契约）
- **挂 server gate**：Story 1.3a/b/c WS · Story 1.5 LogSink · Story 1.7 DesignTokens · Story 1.8 Onboarding · 禁止启动直至跨仓 sync 解决 G1/G2/G6

**Key Strengths**：
1. 哲学 B 贯穿 · 简化断线恢复
2. Empty Provider + CatTestSupport 独立 target
3. 契约运行时发现（WSRegistry）容忍 server 快速演进
4. 三层 Party Mode 合奏留痕
5. 负空间哲学块（从"不做清单"反推产品哲学）

**Known Weaknesses（诚实披露）**：
1. FR 末端追溯断链（抽样 3.5/5）· 仅补 4 条 · 全 63 FR 批量闭环待 Epic 1 kickoff 前
2. HIGH → MEDIUM-HIGH confidence 是基于"演进韧性未验证"和"双故障降级未重评"两个 Winston 盲点
3. 真实 alpha 用户反馈 = 0（G11）
4. Legal/Privacy 未过审（G10）
5. 8 条 Growth 延迟累计技术债 · 无登记表则易失控（tech-debt-registry 为硬门槛）
6. iPhone 端可观测性契约未定义（G8）

**Areas for Future Enhancement（Growth · 已登记 tech-debt-registry）**：
- Error 4 类细分 · Log 6×5 细粒度 · Feature 互 import lint · O2 swiftlint · O4 linker check · O5 PhaseAnimator · C-OPS-01/02/03

---

### Implementation Handoff

**AI Agent Guidelines**：
- 实施 Story 前必读：`ios/README.md` 玄关 → `docs/patterns/README.md` → 本架构文档（含 Step 7 Validation）
- 架构决策是约束不是建议——偏离必须 PR description 说明 + reviewer 核查
- 每 PR 过 P10 Checklist 6 条（含 CatShared/UI 组件 Watch 使用检验）
- 每 Story AC 过 P8 三元组（timeout + ui_terminal + metric + fail_policy 锚点）
- 偏离本 Validation 的 G1-G12 任一条**需 PR description 明引 gap ID + 降级方案**

**First Implementation Priority**：
**Epic 1 Story 1.1 · XcodeGen Bootstrap + tech-debt-registry 骨架**（AC1-5 + Winston 硬门槛 1）

**Starter 命令**：
```bash
brew install xcodegen swift-format
cd ios && xcodegen generate
bash ios/scripts/install-hooks.sh
bash ios/scripts/build.sh --test
```

**Pre-Story 1.1 门槛**（必须完成才能启动 Story 1.1）：
- [ ] 60 min 跨仓 sync 会议（3 议题 · 解 G1/G2/G6）· 输出 server S-SRV-15 交付日期
- [ ] `_bmad/tech-debt-registry.md` 骨架立（Winston 硬门槛 1）
- [ ] `docs/architectures/README.md` 跨仓索引（G12 · 10 min 工作）
- [ ] Legal sync 会议排期确认（G10 · Epic 9 前置）
- [ ] Pre-MVP alpha tester 招募开始（G11 · Epic 5 收尾前用上）
