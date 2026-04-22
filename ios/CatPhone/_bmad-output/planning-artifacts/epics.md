---
stepsCompleted: [step-01-validate-prerequisites, step-02-design-epics, step-03-create-stories, step-04-final-validation]
inputDocuments:
  - /Users/zhuming/fork/catc/ios/CatPhone/_bmad-output/planning-artifacts/prd.md
  - /Users/zhuming/fork/catc/ios/CatPhone/_bmad-output/planning-artifacts/architecture.md
  - /Users/zhuming/fork/catc/ios/CatPhone/_bmad-output/planning-artifacts/ux-design-specification.md
  - /Users/zhuming/fork/catc/ios/CatPhone/_bmad-output/planning-artifacts/server-handoff-ux-step10-2026-04-20.md
  - /Users/zhuming/fork/catc/ios/CatPhone/_bmad-output/planning-artifacts/sprint-change-proposal-2026-04-20.md
  - /Users/zhuming/fork/catc/ios/CatPhone/_bmad-output/planning-artifacts/implementation-readiness-report-2026-04-20.md
  - /Users/zhuming/fork/catc/server/_bmad-output/planning-artifacts/epics.md
  - /Users/zhuming/fork/catc/docs/api/openapi.yaml
  - /Users/zhuming/fork/catc/docs/api/ws-message-registry.md
  - /Users/zhuming/fork/catc/docs/api/integration-mvp-client-guide.md
  - /Users/zhuming/fork/catc/docs/backend-architecture-guide.md
  - /Users/zhuming/fork/catc/CLAUDE.md
  - /Users/zhuming/fork/catc/ios/CatPhone/_bmad/tech-debt-registry.md
author: Claude (bmad-create-epics-and-stories · Step 1)
createdAt: 2026-04-21
---

# CatPhone - Epic Breakdown

## Overview

This document provides the complete epic and story breakdown for **CatPhone (iPhone 端)**, decomposing the requirements from the PRD (v2 · 2026-04-21 validated · PASS 4.5/5), UX Design Specification (v0.3), and Architecture (2026-04-21 · CONDITIONALLY READY) into implementable stories.

## Requirements Inventory

### Functional Requirements

**能力契约（binding · PRD §Functional Requirements）**：Epic / Story 只能 cite `FR-*` 或 `C-*` ID，不能 cite 散文。每条 FR 带测试标签 `[U]`=单测 / `[I]`=集成 fake / `[E9]`=Epic 9 真机。

#### 1. 账号 & Onboarding（C-ONB-* + C-UX-01/02 + C-OPS-06）

- **FR1**：User 可通过 Sign in with Apple 登录 `[U, I]`
- **FR2**：有 Apple Watch 的 User 可在 onboarding 引导去 App Store 安装 CatWatch app，Watch 端独立完成 SIWA，Server 根据 SIWA user_id 自动关联双端 `[I, E9]`
- **FR3**：无 Apple Watch 的 User 可选观察者模式继续 onboarding，不被阻断 `[U, I]`
- **FR4**：User 可授权 HealthKit 读取步数 `[I, E9]`
- **FR5**：User 可拒绝 HealthKit 授权，app 仍可进入（功能降级，见 FR44/FR50）`[U, I]`
- **FR6**：User 在 SIWA 登录前可浏览 3 屏叙事 Onboarding 卡片（含 Slogan "你的猫在家等你走到它身边"）`[U, I]` → `C-UX-01`
- **FR7**：User 首次完成 Watch 配对（或选观察者模式）后进入撕包装首开仪式（盒子震动 → 猫跳出 → 命名）`[I, E9]` → `C-UX-02`
- **FR8**：User 可请求删除账号（SIWA + GDPR · 联动 `S-SRV-12`）`[I, E9]` → `C-OPS-06`
- **FR9**：User 可导出个人数据（GDPR · 联动 `S-SRV-12`）`[I]`
- **FR10**：User 可主动 sign out 并清除本地 token（Keychain 清理）`[U, I]`
- **FR10a**：System 将账号级里程碑（`onboarding_completed` / `first_emote_sent` / `first_room_entered[room_id]` / `first_craft_completed`）存储在 server `user_milestones` collection（`S-SRV-15`），禁止用 UserDefaults 做权威源 `[U, I]` → `C-ONB-01`

#### 2. 好友（C-SOC-03 + C-UX-05）

- **FR11**：User 可发送好友请求 `[U, I]`
- **FR12**：User 可接受 / 拒绝收到的好友请求 `[U, I]`
- **FR13**：User 可查看好友列表并管理关系 `[U, I]`
- **FR14**：User 可屏蔽 / 移除一个好友 `[U, I]`
- **FR15**：User 可静音某位好友的表情广播（不收 push / 震动，保留好友关系）`[U, I]` → `C-UX-05`
- **FR16**：User 可举报某位好友或其表情内容 `[I, E9]` → `C-UX-05`

#### 3. 房间 + 社交广播（C-ROOM-* + C-SOC-01/02 + C-ME-01）

- **FR17a**：User 可创建房间 `[U, I]`
- **FR17b**：User 可加入 / 离开房间 `[U, I]`
- **FR17c**：User 是房主时离开需转让或解散（体面退房）`[U, I]` → `C-UX-05`
- **FR18**：User 可通过邀请链接 / 二维码邀请好友入房 `[I, E9]`
- **FR19**：邀请链接失败分支（过期 / 已是好友 / 已在房间 / 房间满 / 自邀）UI 显式提示 `[U, I]`
- **FR20**：User 可通过邀请链接加入房间 `[I]`
- **FR21**：iPhone User 可查看房间成员名单（姓名 / 在线或挂机状态 / 一句话叙事文案）`[U, I]`
- **FR22**：叙事文案合成引擎存在规则表（trigger → template_id）供 AC 精确断言 `[U]`
- **FR23**：Watch User 可在房间内看到其他成员的猫同屏渲染（2–4 人）`[I, E9]`
- **FR24**：房间 2+ 成员同时走路时，Watch 端触发环绕跑酷特效（server 判定下发，Watch 禁本地计时触发）`[U, I, E9]`
- **FR25**：观察者 User 隐私 gate — 只见 `current_activity` + `minutes_since_last_state_change`，不见 `exact_step_count` / `health_*` `[U, I]`
- **FR26**：User 可点击自己的猫（iPhone "我的猫"卡片 / Watch 端猫本体）弹出表情选单 `[U, I]`
- **FR27**：User 选表情后，自己的猫旁浮动对应 emoji（本地立即呈现）`[I, E9]`
- **FR28**：User 在房间内发表情时，server 广播给房间成员；对端 Watch 轻震 + emoji 浮在 emitter 猫旁；iPhone 观察者走 push 通道 `[U, I, E9]`
- **FR29**：User 不在房间时发表情仅本地呈现，不发 server fan-out（但仍走 server 去重落日志）`[U, I]`
- **FR30**：表情广播为 fire-and-forget — 发送方无"已读/已送达"UI 状态，对端自治呈现 `[U]`

#### 4. 步数（C-STEPS-*）

- **FR31**：System 从 HealthKit 本地读取步数用于展示 `[I, E9]`
- **FR32**：System 使用 server 权威步数判定盲盒解锁（iPhone 本地不做解锁判定，防作弊 + 防事实源漂移）`[U, I]`
- **FR33**：User 可在 iPhone 查看步数历史图表（联动 `S-SRV-9`）`[U, I]`
- **FR34**：WS 断线时，盲盒解锁 UI 显示"已达成·等待确认"占位，不 pre-empt server `[U, I]`

#### 5. 经济：盲盒 + 合成 + 装扮（C-BOX-* + C-SKIN-* + C-DRESS-*）

- **FR35a**：System 在 User 在线挂机满 30 min 后掉落盲盒（server 权威判定 + `ClockProtocol` 抽象）`[U, I, E9]`
- **FR35b**：User 可消耗 1000 步打开已掉落的盲盒 `[U, I]`
- **FR35c**：Watch User 解锁盲盒时看到开箱动画 + 皮肤揭晓 `[I, E9]`
- **FR35d**：Watch 不可达时盲盒走满 1000 步进入 `unlocked_pending_reveal` 中间态（`S-SRV-16`）；iPhone 仓库显示"已解锁·待揭晓"；Watch 下次可达时 server 推 `box.unlock.revealed` `[U, I]`
- **FR36**：~~（v2 已作废：盲盒完全无通知）~~
- **FR37**：System 为每个皮肤标注颜色分级（白 < 灰 < 蓝 < 绿 < 紫 < 橙）`[U]`
- **FR38**：盲盒开出重复皮肤时，System 自动归为合成材料（文案"材料入库"）`[U, I]`
- **FR39**：User 可在 iPhone 仓库查看全部皮肤 + 合成材料计数 `[U, I]`
- **FR40**：User 可在材料满足合成规则（候选 5 低级 → 1 高级）时触发合成（联动 `S-SRV-10`，含断网 / 材料不足 / 并发幂等）`[U, I]`
- **FR41**：User 可在 iPhone 大屏选择装扮给自己的猫 `[U, I]`
- **FR42**：装扮 payoff 由 Watch 端渲染 `[I, E9]`
- **FR43**：System 跨天 / 跨时区正确重置挂机计时基准（`ClockProtocol`）`[U]`

#### 6. 陪伴反馈：久坐召唤（C-IDLE-01..02，无情绪机）

- **FR44**：System 在 Watch 端维护猫动作同步（走路 / 睡觉 / 挂机各自动画；无情绪 layer）`[I, E9]`
- **FR45a**：User 无活动 2h 后，Watch 主动 haptic 召唤起身（`UIImpactFeedbackGenerator` 参数异于盲盒通知）`[U]`
- **FR45b**：真机上人类可辨识久坐召唤与盲盒震感差异 `[E9]`
- **FR46**：User 起身走动 30s 后，Watch 端猫做欢迎动作（纯交互触发，非情绪转移）`[I, E9]`

#### 7. 容错（C-REC-*）

- **FR47**：WS 断线后 System 重连并按 `last_ack_seq` 补发遗漏事件 `[U, I]`（**注**：D2 决策为重发 Authorization → 可选 `session.resume` → 主动 re-`room.join`，禁 replay `last_ack_seq`；CD-drift 已登记）
- **FR48**：Server 待推事件 TTL = 24h；超时 GC 后重连显示 fail-closed 提示"部分事件已过期，请刷新"`[U, I]`
- **FR49**：`/v1/platform/ws-registry` 启动失败时，System 屏蔽主功能 + 显示"网络异常，请稍后重试"（fail-closed）`[U, I]`
- **FR50**：HealthKit 授权被拒时，步数 UI 回退为 "—" + 引导条"去设置开启权限" `[U, I]`

#### 8. UX 生存层（C-UX-03..04）

- **FR51**：全局空状态 — 仓库空 / 好友空 / 房间无广播 `[U, I]` → `C-UX-03`
- **FR52**：User 可编辑昵称 / 头像 / 个性签名 `[U, I]` → `C-UX-04`
- **FR53**：User 可在"设置"入口查看隐私政策 / 使用条款 / 账号信息 `[U]`

#### 9. 跨端消息 + 推送 + 隐私 + 可观测性（C-SYNC-* + C-OPS-*）

- **FR54**：所有跨端消息带 `client_msg_id`（UUID v4）供 server 60s 去重（**注**：D2 决策 dedup TTL = 300s 跟 server；PRD 60s 保留为历史约束描述，CD-drift 登记）`[U]`
- **FR55**：User 可分频道开/关 push（好友 / 表情）`[U, I]`
- **FR56**：System 通过 `wss://`（TLS）加密 WS；`ws://` 仅 debug build 豁免 `[U, E9]`
- **FR57**：System 将 JWT / refresh token 存 Keychain（`kSecClassGenericPassword`）`[U]`
- **FR58**：System 不上传 raw step count，仅发送解锁 ACK / 状态 `[U, I]`
- **FR59**：System 带崩溃上报（MVP：Xcode Organizer；Growth：Crashlytics/Sentry）`[I, E9]` → `C-OPS-01`
- **FR60**：System 启动时做版本检查（MVP：App Store 原生弹窗；Growth：in-app 强更）`[U, I]` → `C-OPS-02`
- **FR61**：System 带远程配置 / 功能开关 / kill switch（MVP：硬编码；Growth：远程）`[U, I]` → `C-OPS-03`
- **FR62**：System 以 `Log.*` facade 做结构化日志（`Log.network/ui/spine/health/watch/ws`）`[U]` → `C-OPS-04`
- **FR63**：App Bundle 带 `PrivacyInfo.xcprivacy`（iOS 17 硬要求 · 必进 MVP）`[U, E9]` → `C-OPS-05`

**FR 总数**：69 条（FR36 作废不计）

### NonFunctional Requirements

#### Performance

- **NFR1**：点猫弹表情选单 **< 200ms**；tab 切换 **< 100ms**（iPhone 14/15/16 基线）
- **NFR2**：冷启动到首屏可交互 **< 2s**
- **NFR3**：Spine 动画 Watch 前台 **60fps** / 后台 **15fps**；帧率不达自动降级，不崩溃
- **NFR4**：HealthKit 按天聚合缓存 **15min**，禁秒级重查
- **NFR5**：仓库皮肤数 > 200 用 `LazyVStack / List` with stable `id:`，禁 `ForEach(array)` 暴力展开

#### Security

- **NFR6**：WS 传输 `wss://` + ATS 合规；`ws://` 仅 debug build 特定联调域名豁免（对应 FR56）
- **NFR7**：Token 存 Keychain `kSecClassGenericPassword`，禁 UserDefaults（对应 FR57）
- **NFR8**：Server 不收 raw step count，仅拿解锁 ACK + 状态（对应 FR58）
- **NFR9**：观察者隐私 gate — `pet.state.broadcast` 字段按订阅者类型裁剪（对应 FR25）
- **NFR10**：SIWA nonce 请求前随机生成，回调校验，防 replay
- **NFR11**：Release build 禁明文 token / email；ID 可打，token 需 hash
- **NFR12**：设备标识用 `identifierForVendor`，禁 IDFA
- **NFR13**：Deep link 严格校验 scheme + 参数；禁直接执行 URL 参数

#### Reliability

- **NFR14**：崩溃率 **≤ 0.2%**（Crashlytics 口径）
- **NFR15**：WS 重连网络切换后 10s 内 **≥ 95%** 成功率
- **NFR16**：Server 待推事件 TTL = **24h**（对应 FR48）
- **NFR17**：启动 `/v1/platform/ws-registry` 失败 fail-closed 屏蔽主功能（对应 FR49）
- **NFR18**：盲盒解锁 server 权威 + `client_msg_id` 300s 去重（对应 FR32 + FR54）
- **NFR19**：离线降级三档文档化（完全离线 / WS 断 / 启动失败）

#### Accessibility

- **NFR20**：VoiceOver 全 interactive UI 元素带无障碍标签（Apple 审核高频）
- **NFR21**：Dynamic Type 文字尺寸支持系统动态字体
- **NFR22**：色盲友好 — 皮肤颜色分级必须配形状或文字标签（C-SKIN-01 不能仅靠颜色区分稀有度）
- **NFR23**：Haptic 作为补充而非替代 — 所有触觉信号同时有视觉兜底
- **NFR24**：Onboarding 3 屏叙事卡片（FR6）和撕包装仪式（FR7）允许 skip

#### Scalability

- **NFR25**：MVP 装机量预期 — 内测 100 / TestFlight 公测 1 万 / 正式版 10 万（客户端对 server 无特殊压力）
- **NFR26**：客户端并发约束 — 单用户最多同时在 1 房间；房间 max **4 人**；MVP 好友数上限 **50**
- **NFR27**：WS 单连接共享 envelope；iPhone / Watch 任一端在线都用同一 session
- **NFR28**：冷启动网络仅 `/v1/platform/ws-registry`（必）+ 可选 health check；不做全量 state 拉取

#### Integration

- **NFR29**：HealthKit 读 only — `HKStatisticsCollectionQuery` 按天；增量走 `HKObserverQuery + HKAnchoredObjectQuery`；禁高频 `HKSampleQuery`
- **NFR30**：WatchConnectivity 抽象 `WatchTransport`（哲学 B）— MVP 不使用 WC 做核心交互；WC 仅 fast-path 可选；核心交互必须 server 权威 + WS 双链路；禁 `transferFile` 走 JSON
- **NFR31**：APNs 标准 iOS 推送，分频道开关（对应 FR55）
- **NFR32**：SpriteKit + Spine — `esoteric-software/spine-runtimes` 通过 SwiftPM 锁版本；仅 Watch 侧密集使用，iPhone 不依赖 Spine
- **NFR33**：SIWA 标准接入；若后续加第三方 OAuth，SIWA 仍需并列（App Store 4.8）
- **NFR34**：跨端契约锚点 — OpenAPI 0.14.0-epic0 + WS registry apiVersion v1；漂移以 server 为准

**NFR 总数**：34 条

### Additional Requirements

来源：Architecture（Starter Evaluation / Core Decisions D1-D7 / Implementation Patterns）+ CLAUDE.md §21 工程纪律 + tech-debt-registry.md。

#### AR-1 · Starter / Bootstrap（Epic 1 Stage A 核心交付）

- **AR1.1**：现有 `ios/project.yml` + CatShared/CatCore/CatWatch 四 target 骨架作为 brownfield starter（不执行外部 CLI）
- **AR1.2**：`ios/scripts/build.sh` 填实链路 — `xcodegen generate → xcodebuild build → xcodebuild test → swift-format lint --strict → 零 warning 校验 (-warningsAsErrors)`
- **AR1.3**：`ios/scripts/install-hooks.sh` 填实（`.git/hooks/pre-push` 自动触发 build.sh）
- **AR1.4**：破例引入 `swift-format`（Apple 官方 · 零配置 · `brew install swift-format`）
- **AR1.5**：Onboarding 文档三件套 — `ios/README.md` 唯一入口 / `ios/docs/dev/troubleshooting.md` / `_bmad/templates/story-ac-triplet.md` + `_bmad/lint/ac-triplet-check.sh`
- **AR1.6**：判据 — 新 dev `bash ios/scripts/build.sh --test` 30 min 内绿 · 零 Slack 提问
- **AR1.7**：Story 1.1 AC1-5 自动验证（build 绿 / test 绿 / lint 绿 / `.xcodeproj` ignored / Stage-A 骨架存在）· 6+6 Provider 存在性验证移到 Stage B/C 对应 Story
- **AR1.8**：AC6（可延 Epic 9）— GitHub Actions `.github/workflows/ios.yml` macOS runner 跑 build.sh --test 绿
- **AR1.9（Party Mode 新增 · Winston + Murat 共识）**：**Spine × Swift 6 严格并发兼容性 Spike** · Epic 1 Stage A 独立 Story · 输出物：Spine Swift 6 兼容性 verdict + actor boundary 草图 + `@MainActor` 隔离边界能否容纳 Spine 渲染循环的结论 · 预算 2-3 天 · **Go/No-Go 门槛**：No-Go 则 Epic 1 范围内决定替代方案（Starscream / 自研 / 降级渲染），避免 Epic 3/4/5 渲染层全部返工

#### AR-2 · Provider 骨架（6 核心 + 6 测试 protocol · Empty impl 占位）

- **AR2.1**：6 核心 Provider 立齐（Epic 1 Story 1 骨架）—
  - `CatShared/Networking/APIClient.swift`（`protocol APIClient { func send<T>(_:) async throws -> T }`）
  - `CatShared/Networking/WSClient.swift`（`func connect()`; `var messages: AsyncStream<WSMessage>`）
  - `CatShared/Networking/AuthTokenProvider.swift`
  - `CatShared/Persistence/LocalStore.swift`
  - `CatCore/Auth/AuthService.swift`
  - `CatCore/Box/BoxStateStore.swift`
- **AR2.2**：6 测试抽象 protocol —
  - `WatchTransport`（哲学 B 降级 · Noop 唯一 MVP impl）
  - `ClockProtocol`（`advance(by:) / setNow(_:)` · Story 层定义 timer 重入 / 跨 actor 线程模型语义）
  - `IdleTimerProtocol`（wrap `UIApplication.shared.isIdleTimerDisabled`）
  - `AccessibilityAnnouncer`（wrap `UIAccessibility.post`）
  - `FirstTimeExperienceTracker`（server `user_milestones` + UserDefaults 副作用隔离）
  - `KeyValueStore`（`emoji_send_count_v1` 类 UI 级状态）

#### AR-3 · Fixture 三件套 + DualSSoTScenarioFixture

- **AR3.1**：`UserDefaultsSandbox`（`UserDefaults(suiteName: UUID)` + tearDown 清理）
- **AR3.2**：`WSFakeServer`（envelope-agnostic · 构造函数接 `envelope: Data` + `replay(jsonlFile:)` · fixture jsonl 从 `docs/api/ws-message-registry.md` 生成 · msg type 按字符串处理 · 覆盖 unknown type default branch）· **Party Mode 追加（Murat）**：WS fixture 接口冻结后打 version tag（`WSFakeServerV1`）· 后续跨 Epic 变更 E1 接口须 bump version · 防止 Epic 3/4/5/6 级联 mock 重写
- **AR3.3**：`HapticSpy`（wrap `UIImpactFeedbackGenerator` · record-only）
- **AR3.4**：`DualSSoTScenarioFixture`（HealthKit stub + WSFakeServer 时序编排器 · 复现"本地步数 =100 / server 权威 =98 / 延迟 2s 后追平"双 SSoT 漂移窗口 · Epic 1 Story 1 必交付）

#### AR-4 · 契约漂移防护

- **AR4.1**：`WSRegistry` 运行时发现（启动拉 `/v1/platform/ws-registry` · 缓存 `{type: {requiresDedup, requiresAuth}}` · 发送前查表 · 未知 type 优雅降级 + `Log.ws.warn` + UI 屏蔽入口）
- **AR4.2**：新建 `docs/contract-drift-register.md`（repo 根 docs/ · 不属任一端 repo · 承载 Known Contract Drift 7 项 + 未来新增 · 每条 ≤ 200 字 context）
- **AR4.3**：`ios/scripts/check-ws-fixtures.sh`（dev-time 手动 · 不进 CI · 拉 server `ws-message-registry.md` hash 对比本地 fixture snapshot · 下发变更提醒不阻塞）

#### AR-5 · D1–D7 架构决策落地（按 Epic 1 剩余 Story 顺序）

- **AR5.1（D1）**：4 全局 Store 置 `CatCore`（`AuthSession` / `WSConnection` / `CurrentRoom` / `HealthKitAuthorization`）· `@Environment` 从根注入 · Feature-level Store 放 `CatPhone/Features/<X>/<X>Store.swift` · `CatShared` 绝不放 `@Observable`
- **AR5.2（D2）**：WS 客户端架构 — `WSEnvelope.swift` / `WSClient.swift` (protocol) / `WSRegistry.swift` / `WSClientEmpty.swift` / `WSClientURLSession.swift`（Epic 2+）· 单 `URLSessionWebSocketTask` + 指数退避重连 · 单 `AsyncStream<WSPush>` · fire-and-forget 由 API 签名表达（`awaitResponse: false`）· 重连握手：重发 Authorization → 可选 `session.resume` → 主动 `room.join` · **禁 replay `last_ack_seq`**
- **AR5.3（D3）**：Provider × Swift 6 Sendable 解法 3 种混用 — A 全 `@MainActor` 默认 / B actor 隔离（WSClient / StepDataSource / SwiftData 部分）/ C `@unchecked Sendable` 逃逸舱（须附 `// FB<issue_id>` 归档 + PR review 质询）
- **AR5.4（D4）**：`StepDataSource` protocol — `actor HealthKitStepDataSource` 内持 `HKStatisticsCollectionQuery` + 15min cache；`EmptyStepDataSource` + `InMemoryStepDataSource`；`HealthKitAuthorization` `@Observable` in CatCore 统一管；增量 `HKObserverQuery` 回调 → continuation
- **AR5.5（D5）**：`WatchTransport` MVP 仅 `WatchTransportNoop` 唯一实现（`isReachable=false` · `send=no-op`）· Release binary 无 `WCSession` 代码链接 · `FakeWatchTransport` spy 测试用
- **AR5.6（D6）**：Lottie + 帧序列 PNG 渲染栈 — `LottieView` in `CatShared/UI/` · Lottie `.json` 跨端放 `CatShared/Resources/Animations/` · 帧序列 PNG feature 局部 · Spine 绝对禁 iPhone（PR template 勾选 + CatPhone target deps 硬断言）· `FrameSequencePlayer` + `TimelineView` · reduce-motion 停最后一帧
- **AR5.7（D7 Deferred）**：Observability MVP — `CatShared/Utilities/Log.swift` 封装 `OSLog` · 分类 `Log.network/ui/spine/health/watch/ws` · 级别 `debug/info/notice/error/fault`；崩溃用 Xcode Organizer（零三方）；指标由 server Prometheus 承担（`S-SRV-18`）；iPhone 不本地上报；`PrivacyInfo.xcprivacy` 必进 MVP

#### AR-6 · 架构 Open Questions O1-O7（待 Step 5/7 或 Story 回答）

- **AR6.1（O1）**：`AsyncStream` 多消费者陷阱 — 是否改 `actor WSHub { subscribe(type) -> AsyncStream }` + `WSHubTests` 断言"所有 type 都有订阅者"
- **AR6.2（O2）**：D3 默认化方向 — 是否反转为"Store `@MainActor`；Provider 默认非 MainActor"+ swiftlint 规则 `@unchecked Sendable` 必须同行 `// FB\d+`
- **AR6.3（O3）**：D4 Clock 注入 + `cacheHitCount` — `init(clock:, cacheTTL:)` + cacheHitCount 使 15min 边界可断言；`HealthKitSpy` 纳入 C2 fixture
- **AR6.4（O4）**：D5 compile-time 硬验证 — `#if WATCH_TRANSPORT_ENABLED` flag + CI linker symbol check 确保 release binary 真不含 `WCSession`
- **AR6.5（O5）**：D6 三栈分层 — emoji 浮起 / 微回应是否走 SwiftUI 原生 `PhaseAnimator`/`KeyframeAnimator` 而非 Lottie
- **AR6.6（O6）**：D7 崩后证据链 — MVP 内补 `LogSink` protocol + 本地环形 JSONL + 崩后首启上报（零三方）
- **AR6.7（O7）**：HealthKit 授权弹窗期间猫"歪头" 0.8s 过渡，避免空白等待

#### AR-7 · 跨端契约（S-SRV-1..18 · 不在 iPhone 实施，但影响 Story gate）

- **AR7.1**：S-SRV-1 `pet.state.broadcast` + 隐私 gate 字段分级（derived from FR21/FR25, C-ROOM-01/02）
- **AR7.2**：S-SRV-2 `pet.emote.broadcast` fan-out + fire-and-forget（无发送方 ack · derived from FR28/30, C-SOC-01/02）
- **AR7.3**：S-SRV-3 `room.effect.parkour` server 判定下发（derived from FR24, C-ROOM-04）
- **AR7.4**：S-SRV-4 待推队列 TTL=24h + GC 审计日志（derived from FR48）
- **AR7.5**：S-SRV-5 皮肤合成系统（材料计数 + 规则引擎 + 事务性 ack · derived from FR40）
- **AR7.6**：S-SRV-6 久坐召唤触发事件（纯 idle timer · derived from FR45a）
- **AR7.7**：S-SRV-7 盲盒颜色分级 + 概率披露（loot-box 合规 · derived from FR37）
- **AR7.8**：S-SRV-8 `box.drop` WS event + `/boxes/pending` API（derived from FR35a）
- **AR7.9**：S-SRV-9 `/steps/history?range=` 历史查询（derived from FR33）
- **AR7.10**：S-SRV-10 `/craft` 幂等端点 + materials 字段 + 扇出上限 / 节流（derived from FR38/40）
- **AR7.11**：S-SRV-11 `cat.tap` → `pet.emote.broadcast` schema 明确化（derived from FR26/28/29）
- **AR7.12**：S-SRV-12 账户删除 + 数据导出 API（SIWA+GDPR · derived from FR8/9）
- **AR7.13**：S-SRV-13 Idle Timer 权威聚合（`max(iPhone_last_active, Watch_last_active)` · 冷启动拉 `last_active_ts`）
- **AR7.14**：S-SRV-14 解锁结果持久化降级（Redis + Mongo 双写 · Redis 挂读 Mongo + UI 提示）
- **AR7.15**：S-SRV-15 `user_milestones` collection + API（替代 UserDefaults · 换机无缝恢复 · **Story 1.8 Onboarding 硬依赖**）
- **AR7.16**：S-SRV-16 `box.state` 新增 `unlocked_pending_reveal` 态 + Watch 重连触发 `box.unlock.revealed`
- **AR7.17**：S-SRV-17 取消 `emote.delivered` 发送者 ack 推送（fire-and-forget 对称性硬约束）
- **AR7.18**：S-SRV-18 所有 fail 节点 Prometheus metric 打点

#### AR-8 · CLAUDE.md §21 工程纪律（Epic 1+ 每 Story 自检）

- **AR8.1（§21.1）**：双 gate 漂移守门（全局常量集 / 错误码 / WS msg type / feature flag / Redis key prefix 新增时）
- **AR8.2（§21.2）**：Empty/Noop Provider 逐步填实（不一次性写满）
- **AR8.3（§21.3）**：fail-closed vs fail-open 决策必须在 Dev Notes 登记 + 留可观测信号
- **AR8.4（§21.4）**：语义正确性 AC review 早启（tool / metric / guard / measurement stories 实施前做）
- **AR8.5（§21.5）**：`tools/*` CLI 上线判据
- **AR8.6（§21.6）**：Spike / 真机 / 人工执行类工作归 Epic 9（不塞业务 epic 关键路径）
- **AR8.7（§21.7）**：server 测试自包含；iPhone 类比 — `xcodebuild test` 一命令绿，禁依赖真机 / 真 Watch / 特定账号 / 外网
- **AR8.8（§21.8）**：PR checklist "语义正确性"思考题 — "who gets misled if this code runs and produces a wrong result but doesn't crash?"

#### AR-9 · Tech Debt Registry（9 条骨架 · Story 触发条件）

tech-debt-registry.md 9 条 TD 条目存在，每条带 trigger / when-to-revisit。Story 实施时必须检查是否触发 TD condition（详见 `_bmad/tech-debt-registry.md`）。

#### AR-10 · Failure Semantics ADR + Fail-closed 一致性 Gate（Party Mode 新增 · Winston + Murat 共识）

**背景**：原 Epic 6 容错解散为 Epic 1 baseline + 各 Epic Story 的 Epic-specific 失败路径后，fail-closed 语义一致性责任分散。Winston 警告"没有单一 owner 发现 drift"；Murat 警告"触发条件不对称 + 没有信号"（who gets misled）。两者互补 → 都做。

- **AR10.1 · `FailureSemantics.md` ADR**（Winston）：Epic 1 Stage A 内交付 · 路径 `docs/architectures/adr/ADR-001-failure-semantics.md` 或 `ios/CatPhone/docs/adr/` · 必写内容：
  - fail-closed vs fail-open 判据表（何时 fail-closed / 何时 fail-open 的决策矩阵 · 遵 `CLAUDE-21.3`）
  - Observable signal 规范（`Log.ws.error` / `Analytics.track` / metric 打点必要性）
  - 每个 Epic-specific 失败路径 Story 必须引用并声明符合哪个判据（AC 强制）
- **AR10.2 · Fail-closed grep gate CI**（Murat）：Epic 1 Stage A 内交付脚本 · 路径 `ios/scripts/check-fail-closed-sites.sh` · 进 CI：
  ```bash
  # 扫 GentleFailView / failClosed / Log.ws.error 所有 site
  # 每个 site 同文件必须配对 Analytics.track("ws_fail_closed") 或 metric 打点
  # 缺失 → PR block
  ```
- **AR10.3 · PR checklist 新增**：`.github/pull_request_template.md` 加勾选项"新增 fail-closed site 已登记 metric 打点 + 引用 ADR-001 判据条目"

### UX Design Requirements

来源：ux-design-specification.md v0.3（Design System / Component Strategy / UX Patterns / Responsive & Accessibility）。**每条 UX-DR 必须足够具体以生成可测试 AC**。

#### UX-DR1 · Design Tokens 单文件（CatShared/Resources/DesignTokens.swift）

6 维度：
- **色系**：奶油/米色温柔主调 + 陶土/蜜桃强调；稀有度 6 级低饱和 60-70%（白/灰/蓝/绿/紫/橙）；功能色 mint/honey/coral
- **字体**：SF Pro + Dynamic Type；不引自定义字体；5 级字号（大标题/标题/正文/小字/叙事）
- **间距**：8pt 栅格（8/12/16/20/24/32/48）；留白偏大
- **圆角**：卡片 16 / 按钮 12 / chip 8 / sheet 24
- **阴影**：极轻（Y=2, blur=8, opacity=6%）或完全无
- **动画**：默认 0.35s easeInOut；仪式最长 1.5s（撕包装）；呼吸循环 4s；抬眼 0.6s 单次
- **图标**：SF Symbols 主导；少量品牌资产（猫 avatar / 撕包装盒 / 皮肤缩略）

#### UX-DR2 · 跨端共享 UI 组件（CatShared/UI/ · 7 个）

- **MyCatCard**（`cat: Cat, breathingEnabled, onTap`）
- **SkinRarityBadge**（`rarity: Rarity, label: String` · 印章 treatment · 6 色 × 形状 × 文字三重冗余）
- **StoryLine**（`text: String, time: Date, category` · Bear 风格时态模糊化）
- **EmptyStoryPrompt**（基于 `ContentUnavailableView` · `icon, title, cta`）
- **GentleFailView**（`reason: FailReason, retry`）
- **EmojiEmitView**（`emoji: Emoji, origin: CGPoint` · 0.25s 浮起 + 2s 缓飘 + 0.35s 淡出 + ±2pt 摆动）
- **LottieView**（wrap `lottie-ios` · 读 `@Environment(\.accessibilityReduceMotion)` · reduce-motion 显首帧静帧）

#### UX-DR3 · Onboarding 独占组件（CatPhone/Features/Onboarding/ · 5 个）

- **NarrativeCardCarousel**（3 屏 · 含 Slogan · 允许 skip）
- **WatchModePicker**（"我有 Watch" / "先进来看看"）
- **TearCeremonyView**（撕包装首开仪式）
- **CatGazeAnimation**（Lottie 2.5s）
- **CatNamingView**（命名输入）

#### UX-DR4 · Account / 我的猫组件（CatPhone/Features/Account/ · 5 个）

- **AccountTabView**（承载 MyCatCard + 设置入口）
- **EmojiPickerInline**（2×2 网格 · 从卡片下方展开）
- **EmojiPickerSheet**（Sheet 降级 · 底部 30% 弹出）
- **CatMicroReactionView**（3 种随机派发）
- **FirstEmojiTooltip**（首次发送后 "它摆了摆尾巴" · 2s 自动淡出）

#### UX-DR5 · Friends 组件（CatPhone/Features/Friends/ · 3 个）

- **FriendListView** / **FriendRequestBanner**（`onAccept, onReject`）/ **InviteQRSheet**

#### UX-DR6 · Inventory 组件（CatPhone/Features/Inventory/ · 8 个）

- **InventoryTabView**（分栏：已入库 / 材料）
- **SkinCardGrid** / **SkinCard** / **SkinDetailSheet**（放大 + 装扮按钮）
- **BoxPendingCard**（进度 347/1000）
- **BoxUnlockedPendingRevealCard**（A5' · 已接回家待揭晓）
- **MaterialCounter**（"蓝色 × 3/5"）
- **CraftConfirmSheet**（"换一条"按钮 · 破坏性操作文案业务语境）

#### UX-DR7 · Dressup 组件（CatPhone/Features/Dressup/ · 3 个）

- **DressupTabView**（大屏编辑）/ **SkinGalleryPicker** / **DressupPreview**（iPhone 静态预览）

#### UX-DR8 · Room 组件（CatPhone/Features/Room/ · 4 个）

- **RoomTabView**（未在房间 / 已在房间两态）
- **RoomMemberList**（iPhone 纯文字 · 无猫渲染）
- **RoomStoryFeed**（使用多个 StoryLine）
- **RoomInviteButton**（含失败分支提示）

#### UX-DR9 · Utility 组件（4 个）

- **SkeletonView**（`.redacted(reason: .placeholder)` · 替代 spinner）
- **HintLabel**（ink/500 + .caption）
- **GentleConfirmSheet**（破坏性操作确认）
- **PulseEffect**（首次引导一次性脉冲）

#### UX-DR10 · UX Patterns（10 个通用模式）

- **Pattern 1 · Empty State**：`EmptyStoryPrompt` + 5 个空状态模板（好友/仓库/材料/房间外/房间内无广播）
- **Pattern 2 · Fail-closed**：`GentleFailView` · 6 触发场景文案（ws-registry 失败 / WS 断 / SIWA 失败 / HealthKit 拒绝 / 房间邀请过期 / 合成失败）· 禁用词（抱歉/失败/错误/无法/请稍候/感叹号）· 永远有去处
- **Pattern 3 · Fail-open**：UI 静默隐藏加分项 · 主功能继续 · 不显示"加载中"或错误态 · 可观测 metric 打点
- **Pattern 4 · Coach Mark / Tooltip**：强制引导（hand pointer）+ Tooltip 情感兜底 · 只出现 1 次 · 状态持久化由 `FirstTimeExperienceTracker`（server `UserMilestones`）· 禁"跳过引导"按钮
- **Pattern 5 · Skeleton vs Spinner**：首选骨架屏；spinner 仅必要同步阻塞；禁 `ProgressView` 旋转
- **Pattern 6 · Destructive Confirmation**：`GentleConfirmSheet` · 破坏性按钮文案业务语境（"换一条"/"解散"/"离开"· 禁"删除/取消"）
- **Pattern 7 · Narrative Copy**：规则表驱动 · 时态模糊化档位（刚刚 / X min 前 / 今天早些时候 / 今天上午）· 禁精确时间戳 · 规则表 server 侧 + 客户端一致副本
- **Pattern 8 · Server 权威状态展示**：所有跨端状态 UI 绑定 server push；客户端不自决策（反模式：本地步数 ≥ 1000 自显"已解锁"）
- **Pattern 9 · Fire-and-forget 对称性**：禁 UI 元素（已读/已送达/对方已收到/正在发送/发送失败/重试发送）· 允许（本地 emoji 浮出 / haptic / 猫微回应 / 房间叙事流 presence）
- **Pattern 10 · 进度显示（数字优先）**：MVP `347 / 1000 步`（不用模糊文字）· Growth 可替换为"天气语言"

#### UX-DR11 · 响应式设备 Matrix

- **iPhone SE (3rd)** 667pt · Inline menu → **自动切 Sheet**（< 700pt 阈值）
- **iPhone 13 mini** 812pt / **15 / 15 Pro** 852pt · Inline menu 正常
- **15 Plus / Pro Max** 932pt · 单手热区：卡片 Y > 60% 时切 Sheet

#### UX-DR12 · Dynamic Type Matrix

- `.xSmall` 到 `.large` · 正常 Inline menu
- `.xLarge` 到 `.xxxLarge` · Inline menu 2×2 改 2 行堆叠（保 54pt tap target）
- `.accessibilityMedium` ~ `.accessibilityXXXLarge` · **自动切 Sheet**；菜单改垂直单列
- 纪律：语义字号（`.body / .caption`）禁硬编码 pt；超长文 `.allowsTightening(true)` + `.minimumScaleFactor(0.9)`；容器 `.frame(minHeight:)`

#### UX-DR13 · VoiceOver 适配

- 组件 accessibilityLabel 清单：MyCatCard / EmojiPickerInline / SkinRarityBadge / BoxPendingCard / StoryLine 等
- VoiceOver announcements 通过 `AccessibilityAnnouncer` protocol 触发（发表情成功 / 盲盒被发现 / 好友邀请到达 / Watch 开箱完成）

#### UX-DR14 · Reduce Motion 降级（7 条）

- MyCatCard 呼吸 → 静止 + 透明度 ±5%
- MyCatCard 抬眼 → 跳过
- CatGazeOnboarding → 静帧 PNG 停留 1.5s
- emoji 浮起 → 0.5s 加速版
- 猫微回应 → 单帧姿态切换
- 撕包装仪式 → 跳过直接显打开后画面
- 合成动画 → 跳过直接显结果
- 全局：`@Environment(\.accessibilityReduceMotion)` 贯穿

#### UX-DR15 · Reduce Transparency + Color-Blind

- 禁 `opacity(0.5)` 作为次级信息标志 · hint 用 `ink/500` 语义色
- 稀有度 6 色永不单独承担信息 · 配形状 + 文字标签
- 功能色（mint/honey/coral）仅装饰层 · 所有状态信号 **色 + icon + 文字** 三重冗余

#### UX-DR16 · 单手可达性（iPhone Pro Max）

- TabBar 底部 5 icon 天然拇指区
- 破坏性 CTA 永远在屏幕下半区（如 CraftConfirmSheet "换一条"按钮）
- Sheet 高度 ≤ 屏幕 50%，保 dismiss 手势可达

#### UX-DR17 · 组件复用纪律

- CatShared/UI/ 下组件必须兼容 watchOS（iOS 17 + watchOS 10 语义字号对齐）
- Feature 组件禁止被其他 Feature 导入（跨 Feature 复用通过 CatShared/UI/）
- DesignTokens 唯一色 / 字 / 间距源 · 禁硬编码 HEX / pt
- 所有 interactive 组件带 `accessibilityLabel + accessibilityIdentifier`
- Lottie 动画资产走 `CatShared/Resources/Animations/` · 命名 `cat_gaze_onboarding.json`

#### UX-DR18 · 组件实施优先级（按 Epic 依赖）

- **Epic 1（Onboarding）**：NarrativeCardCarousel / WatchModePicker / TearCeremonyView / CatGazeAnimation / CatNamingView / LottieView
- **Epic 2（账号 + 我的猫）**：MyCatCard / EmojiPicker{Inline, Sheet} / CatMicroReactionView / EmojiEmitView / FirstEmojiTooltip / AccountTabView
- **Epic 3（仓库 + 盲盒）**：BoxPendingCard / BoxUnlockedPendingRevealCard / SkinCard{Grid, } / SkinRarityBadge / MaterialCounter / CraftConfirmSheet / InventoryTabView
- **Epic 4（房间 + 社交）**：RoomTabView / RoomMemberList / RoomStoryFeed / StoryLine / RoomInviteButton / FriendListView / FriendRequestBanner
- **Epic 5（装扮）**：DressupTabView / SkinGalleryPicker / DressupPreview
- **贯穿所有 Epic**：GentleFailView / EmptyStoryPrompt / SkeletonView / HintLabel / GentleConfirmSheet

**UX-DR 总数**：18 条

### FR Coverage Map

**iPhone-addressable FRs**：69 条中 62 条 iPhone 实施 · 6 条 Watch-only 归 Epic 9 真机 smoke（FR23/24/44/45a/45b/46）· FR36 作废 · FR42 跨 Epic（iPhone 触发→E5 · Watch 渲染→E9）· FR35c 仅 Watch 端开箱归 E9。

| FR | Epic | 说明 |
|---|---|---|
| FR1 | E2 | Sign in with Apple 登录 |
| FR2 | E2 + E9 | 引导安装 CatWatch + 双端关联（真机部分归 E9） |
| FR3 | E2 | 观察者模式继续 onboarding |
| FR4 | E4 | HealthKit 授权 |
| FR5 | E4 | HealthKit 拒绝降级 |
| FR6 | E2 | 3 屏叙事 Onboarding 卡片 |
| FR7 | E2 | 撕包装首开仪式 |
| FR8 | E7 | 账户删除（GDPR） |
| FR9 | E7 | 数据导出（GDPR） |
| FR10 | E7 | Sign out + Keychain 清理 |
| FR10a | E2 | 账号级里程碑 server 权威（user_milestones） |
| FR11 | E6 | 好友请求发送 |
| FR12 | E6 | 好友接受/拒绝 |
| FR13 | E6 | 好友列表 |
| FR14 | E6 | 屏蔽/移除好友 |
| FR15 | E6 | 静音好友表情广播 |
| FR16 | E6 | 举报好友/表情 |
| FR17a | E6 | 创建房间 |
| FR17b | E6 | 加入/离开房间 |
| FR17c | E6 | 房主转让/解散 |
| FR18 | E6 | 邀请链接/二维码 |
| FR19 | E6 | 邀请失败分支 |
| FR20 | E6 | 通过邀请链接加入 |
| FR21 | E6 | 房间成员名单 |
| FR22 | E6 | 叙事文案规则表 |
| FR23 | **E9** | Watch 同屏渲染（iPhone 无实施） |
| FR24 | **E9** | Watch 环绕跑酷（iPhone 无实施） |
| FR25 | E6 | 观察者隐私 gate（iPhone UI） |
| FR26 | E3 | 点猫弹表情选单 |
| FR27 | E3 | 自发表情 emoji 浮起 + 本地呈现 |
| FR28 | E6 | 房间内发表情 server 广播 + iPhone 观察者 push |
| FR29 | E3 | 不在房间时本地呈现 + server 去重落日志 |
| FR30 | E3 | fire-and-forget 无 ack UI |
| FR31 | E4 | HealthKit 步数展示 |
| FR32 | E4 | Server 权威盲盒解锁 |
| FR33 | E4 | 步数历史图表 |
| FR34 | E4 | WS 断线"已达成·等待确认"占位 |
| FR35a | E4 | 挂机 30min 掉落盲盒（pending UI） |
| FR35b | E4 | 1000 步开盒 |
| FR35c | **E9** | Watch 开箱动画（iPhone 无实施） |
| FR35d | E4 | unlocked_pending_reveal 中间态 UI |
| FR36 | ~~作废~~ | v2 修订：盲盒完全无通知 |
| FR37 | E4 | 皮肤颜色分级（6 色） |
| FR38 | E4 | 重复皮肤归合成材料 |
| FR39 | E4 | 仓库查看皮肤 + 材料 |
| FR40 | E5 | 合成触发（断网/不足/并发幂等） |
| FR41 | E5 | iPhone 大屏装扮选择 |
| FR42 | E5 + **E9** | iPhone 触发装扮 → Watch 渲染 payoff |
| FR43 | E5 | 跨天/跨时区挂机计时（ClockProtocol） |
| FR44 | **E9** | Watch 猫 walk/sleep/idle 动作同步（iPhone 无） |
| FR45a | **E9** | Watch 久坐召唤 haptic（iPhone 无） |
| FR45b | **E9** | 真机 haptic 可辨识度（感官验证） |
| FR46 | **E9** | Watch 欢迎动作（iPhone 无） |
| FR47 | E1 | WS 断线重连（baseline · 按 D2 `session.resume` + `room.join`） |
| FR48 | E1 | TTL 24h GC + fail-closed（baseline · 各 Epic 扩 UI 文案） |
| FR49 | E1 | ws-registry 启动失败 fail-closed |
| FR50 | E4 | HealthKit 拒绝 UI 回退 |
| FR51 | E7 | 全局空状态 · 组件在 E1 交付 · 各 Epic 自用 |
| FR52 | E7 | 昵称/头像/签名编辑 |
| FR53 | E7 | 设置页（隐私政策/条款/账号信息） |
| FR54 | E1 | client_msg_id 300s 去重（baseline · CD-drift 登记） |
| FR55 | E7 | Push 分频道开关 |
| FR56 | E1 | wss/ATS 传输加密（Bootstrap 配置） |
| FR57 | E1 | JWT 存 Keychain（Bootstrap 能力） |
| FR58 | E4 | 不上传 raw step count |
| FR59 | E7 | 崩溃上报 MVP（Xcode Organizer） |
| FR60 | E7 | 版本检查 MVP（App Store 原生） |
| FR61 | E7 | 远程配置 MVP（硬编码） |
| FR62 | E1 | Log.* facade（Bootstrap） |
| FR63 | E7 | PrivacyInfo.xcprivacy |

**覆盖完整性**：69 个 active FR 全部映射到 8 个 Epic（FR36 作废）· 6 条 Watch-only FR 归 Epic 9 · 跨端 FR（FR2/FR42）iPhone 触发侧归业务 Epic + Watch 渲染 / 真机侧归 Epic 9。

## Epic List

**8 个 MVP Epic · 7 业务 + 1 硬件**（按交付序号排列 · 每 Epic standalone）

### Epic 1: Foundation（基石：Bootstrap + Provider + WS 稳定性基线 + Log）

**Party Mode 修订（Amelia + John + Winston 共识）**：Epic 1 体量偏重（预估 12-18 Story），Story 按 **Stage A / B / C** 三个 group 组织，Story 粒度细化（每 Provider 独立 Story），避免"Story 1.1 AC 边界模糊巨石化"。三个 Stage 在同一 Epic 下分 section 交付。

**User Outcome**：新 dev `clone → bash ios/scripts/build.sh --test` 30 min 内绿 · 0 Slack 提问 · 6+6 Provider + Fixture 骨架 ready 供后续 Epic 填实；WSClient 重连 + TTL + client_msg_id + fail-closed 机制作为所有业务 Epic 的基线能力；FailureSemantics ADR 冻结后续 fail-closed 语义一致性；Spine × Swift 6 spike Go/No-Go verdict 出炉（防 Epic 3-5 渲染层返工）。

**FRs covered**：FR47, FR48, FR49, FR54, FR56, FR57, FR62

**Stage 拆分**（Story 组织结构 · Step 3 会逐 Story 展开）：

- **Stage 1A · Bootstrap + 骨架 + Spike**（AC 全可机器验证）
  - Story 1A.1 — Bootstrap 脚本链路（AR1.2-1.4 · build.sh / install-hooks.sh / swift-format）
  - Story 1A.2 — XcodeGen 配置 + 四 target 骨架（AR1.1 · CatPhone / CatWatch / CatShared / CatCore）+ `.xcodeproj` ignored
  - Story 1A.3 — D1-D7 目录骨架 + Log facade（AR5.7 · `Log.*` 6 分类 OSLog wrap · FR62）
  - Story 1A.4 — Onboarding 文档三件套 + `_bmad/lint/ac-triplet-check.sh`（AR1.5）
  - Story 1A.5 — **Spine × Swift 6 严格并发兼容性 Spike**（AR1.9 · Party Mode 新增 · 2-3 天预算 · Go/No-Go verdict）
  - Story 1A.6 — **`FailureSemantics.md` ADR**（AR10.1 · Party Mode 新增 · fail-closed 判据矩阵）
  - Story 1A.7 — **Fail-closed grep gate 脚本 + PR template 勾选项**（AR10.2-10.3 · Party Mode 新增）
- **Stage 1B · 核心 Provider**（每 Provider 独立 Story · Sendable + Swift 6 并发隔离随落地）
  - Story 1B.1 — APIClient protocol + Empty impl（AR2.1）
  - Story 1B.2 — WSClient protocol + Empty impl + WSRegistry 运行时发现（AR2.1 + AR4.1）
  - Story 1B.3 — WSClient baseline：重连 + TTL + client_msg_id + wss-ATS + fail-closed（FR47/48/49/54/56）
  - Story 1B.4 — AuthTokenProvider + Keychain integration（FR57）
  - Story 1B.5 — LocalStore protocol + Empty impl（AR2.1）
  - Story 1B.6 — AuthService protocol + Empty impl（AR2.1）
  - Story 1B.7 — BoxStateStore protocol + Empty impl（AR2.1）
  - Story 1B.8 — WatchTransportNoop（AR5.5 · D5 哲学 B）
- **Stage 1C · 测试基础设施**（Epic 2+ gate · 独立交付可验证）
  - Story 1C.1 — 6 测试 protocol 骨架（WatchTransport/ClockProtocol/IdleTimerProtocol/AccessibilityAnnouncer/FirstTimeExperienceTracker/KeyValueStore · AR2.2）
  - Story 1C.2 — Fixture 三件套：UserDefaultsSandbox / WSFakeServer `V1` / HapticSpy（AR3.1-3.3）
  - Story 1C.3 — DualSSoTScenarioFixture（AR3.4 · HealthKit stub + WSFakeServer 时序编排）
  - Story 1C.4 — `docs/contract-drift-register.md` + `ios/scripts/check-ws-fixtures.sh`（AR4.2-4.3）
  - Story 1C.5 — CatShared/UI 基础组件：EmptyStoryPrompt / GentleFailView / SkeletonView / LottieView / HintLabel（UX-DR2+9 部分）

**附加交付**：AR1 Bootstrap · AR2 6+6 Provider Empty · AR3 Fixture 三件套 + DualSSoTScenarioFixture · AR4 WSRegistry + contract-drift-register · AR5.1-5.3/5.5-5.7 D1/D2/D3/D5/D6/D7 骨架 · AR8 §21 工程纪律落地 · **AR10 FailureSemantics ADR + grep gate**（Party Mode 新增）· UX Pattern 2/3/5/7/8/9 骨架

**跨仓依赖**：S-SRV-15（`user_milestones` API schema · 为 Epic 2 准备 · 不实施只对齐契约）· 发送端 `client_msg_id` 300s 去重契约对齐 server

**Standalone 判据**：Stage 1A 纯 iPhone 离线可跑绿；Stage 1B Story 1B.1-1.8 每个独立可 review/rollback；Stage 1C 是 Epic 2+ 硬 gate（Fixture 接口冻结后打 version tag · AR3.2 追加）

---

### Epic 2: Onboarding（首开仪式：叙事 + SIWA + Watch 模式 + 撕包装 + 命名）

**Party Mode 修订（John · S-SRV-15 硬 gate 降级方案）**：Winston 指出 Epic 2 standalone 宣告虚假（依赖 server 团队）；John 提出 **本地 dirty flag + TTL 30 天 + foreground sync 降级方案**，解除硬 gate 为软 gate。UserDefaults 仍非权威源（立场不变），只是有期限的桥接：
- 首开流程本地写 `onboarding_state` 临时 flag（显式标记"尚未同步"草稿态 · 非权威）
- App foreground 时尝试 sync 到 server `user_milestones`
- 30 天内 server 未确认 → 视为未完成 onboarding，重新触发（防长期腐烂）
- 这样 Epic 2 可与 server 团队排期解耦，实际 ship 时再收敛为权威源

**Party Mode 修订（John · Epic 2/3 并行窗口）**：Epic 3（我的猫 & 表情）的实际依赖是 **SIWA 登录 + 用户已有猫实例**（Story 2.X SIWA 完成后即可），**无需等撕包装仪式 + 命名全部完成**。Epic 2 内部 Story 交付到 SIWA + Watch 模式选择（Story 2.1-2.3）后，Epic 3 即可并行启动，不必串行等 Epic 2 整体完工。

**User Outcome**：新用户首次打开 app → 3 屏叙事（含 Slogan "你的猫在家等你走到它身边"）→ SIWA 登录 → Watch 配对引导 / 观察者模式二选一 → 撕包装仪式 → 猫命名，完成"我拥有一只猫"的首次感情投资。

**FRs covered**：FR1, FR2, FR3, FR6, FR7, FR10a

**附加交付**：UX-DR3 Onboarding 组件（NarrativeCardCarousel / WatchModePicker / TearCeremonyView / CatGazeAnimation / CatNamingView）· SIWA nonce 生成 + 回调校验（NFR10）· 观察者模式路径（为 Epic 6 观察者 FR25 铺垫）· **本地 dirty flag 降级方案**（Party Mode 新增 · 30 天 TTL + foreground sync · 解除 S-SRV-15 硬 gate）

**跨仓依赖**：**S-SRV-15 `user_milestones` collection + API**（**软 gate** · 降级为 30 天 TTL dirty flag · server 交付后收敛为权威源；若 server 延迟可继续 Story 开发）

**Standalone 判据**：Onboarding 流程独立可测（降级方案使得即使 server 未交付 S-SRV-15，Story 2.X 仍可 fake 完成里程碑写入 + sync attempt）· Epic 1 Foundation Stage 1A+1B 必须先绿

**Epic 2/3 并行窗口**：Story 2.1（3 屏叙事）+ Story 2.2（SIWA 登录）+ Story 2.3（Watch 模式选择）完成后 → Epic 3 启动条件满足 · Story 2.4（撕包装）+ Story 2.5（命名）可与 Epic 3 并行

---

### Epic 3: 我的猫 & 表情交互（MyCat & Emote）
**User Outcome**：User 每次打开 app，都能和自己的猫有 reciprocal — 点猫 → emoji 浮起 + 猫微回应 + haptic。即使不在房间 / 断网，本地交互仍闭环（fire-and-forget 对称性）。
**FRs covered**：FR26, FR27, FR29, FR30
**附加交付**：UX-DR4 Account 组件（MyCatCard 呼吸+抬眼 · EmojiPickerInline + Sheet 自适应 · CatMicroReactionView 3 随机 · EmojiEmitView · FirstEmojiTooltip · AccountTabView）· UX Pattern 9 fire-and-forget 对称性严格落地（禁 UI: 已读/已送达/发送中/发送失败）· UX Pattern 4 Tooltip 引导（FirstTimeExperienceTracker）· Epic-specific 网络失败路径：server 去重落日志失败本地仍呈现（FR29）
**跨仓依赖**：S-SRV-2（emote fan-out）· S-SRV-11（cat.tap schema）· S-SRV-17（取消 emote.delivered ack · fire-and-forget 对称性硬约束）
**Standalone 判据**：即使 Epic 4/5/6 均未交付，User 也能与自己的猫进行本地 emote 交互（FR29 明确"不在房间时仅本地呈现"）

---

### Epic 4: HealthKit × 盲盒 × 仓库（经济闭环 Part 1）
**User Outcome**：走路 → 看见步数进展 → 挂机 30min 掉盒子 → 1000 步开盒 → 皮肤入仓库 / 重复转材料。核心北极星反馈回路（"走 1000 步接猫回家"）一次跑通。
**FRs covered**：FR4, FR5, FR31, FR32, FR33, FR34, FR35a, FR35b, FR35d, FR37, FR38, FR39, FR50, FR58
**附加交付**：AR5.4（D4 StepDataSource · actor HealthKitStepDataSource + 15min cache · 增量 HKObserverQuery）· HealthKitAuthorization @Observable in CatCore · UX-DR6 Inventory 组件（InventoryTabView / BoxPendingCard / BoxUnlockedPendingRevealCard / SkinCardGrid / SkinCard / SkinRarityBadge / MaterialCounter）· UX Pattern 8 Server 权威状态展示严格落地（客户端不自决策"已解锁"）· UX Pattern 10 数字进度（347/1000）· Epic-specific 失败路径：HealthKit 拒绝（GentleFailView "我们只看步数"）· WS 断线"已达成·等待确认"（FR34）· S-SRV-14 降级（"盲盒暂时无法打开，稍后重试"）
**跨仓依赖**：S-SRV-7（loot-box 合规披露）· S-SRV-8（box.drop WS event + /boxes/pending API）· S-SRV-9（/steps/history API）· S-SRV-14（解锁 Redis+Mongo 双写降级）· S-SRV-16（unlocked_pending_reveal 中间态）
**Standalone 判据**：单用户（无需好友 / 房间 / 装扮）即可跑通"走路 → 盒子 → 皮肤仓库"整条闭环；Watch 侧 FR35c 开箱动画归 Epic 9 真机验证

---

### Epic 5: 合成 & 装扮（经济闭环 Part 2）
**User Outcome**：User 把重复材料合成为更高阶皮肤 → 在 iPhone 大屏选中装扮 → Watch payoff 渲染，完成"收藏升华 + 自我表达"。
**FRs covered**：FR40, FR41, FR42（iPhone 触发侧）, FR43
**附加交付**：UX-DR6 CraftConfirmSheet（破坏性确认 · "换一条"文案 · 禁"删除/取消"等硬词）· UX-DR7 Dressup 组件（DressupTabView / SkinGalleryPicker / DressupPreview · iPhone 静态预览不含 Spine）· UX Pattern 6 Destructive Confirmation 严格落地 · AR5.4 ClockProtocol 跨时区测试 · Epic-specific 失败路径：合成中途断网（GentleFailView "合成没成功，材料还在仓库"）· 并发双端合成幂等
**跨仓依赖**：S-SRV-5（合成系统 + 规则引擎）· S-SRV-10（/craft 幂等 + materials + 扇出上限/节流）
**Standalone 判据**：仓库有 5+ 重复皮肤即可合成（Epic 4 前置）· iPhone 侧 Story 独立可跑 · Watch 渲染 payoff 归 Epic 9

---

### Epic 6: 好友 & 房间（社交核心）
**User Outcome**：User 加好友 → 建房 / 加入房间 → 看见朋友的状态（叙事流 · 观察者文字层）→ 房间内发表情 server 广播，完成社交回路激活。
**FRs covered**：FR11, FR12, FR13, FR14, FR15, FR16, FR17a, FR17b, FR17c, FR18, FR19, FR20, FR21, FR22, FR25, FR28
**附加交付**：UX-DR5 Friends 组件（FriendListView / FriendRequestBanner / InviteQRSheet）· UX-DR8 Room 组件（RoomTabView / RoomMemberList · 纯文字无猫渲染 / RoomStoryFeed + StoryLine / RoomInviteButton）· UX Pattern 7 叙事文案规则表 + 时态模糊化档位 · 观察者隐私 gate 字段裁剪（FR25 · pet.state.broadcast 字段按订阅者类型裁剪）· Epic-specific 失败路径：邀请链接 5 分支（FR19 · 过期/已好友/已在房/房满/自邀）· 房间邀请过期（GentleFailView "这次错过了"）
**跨仓依赖**：S-SRV-1（pet.state.broadcast + 隐私 gate）· S-SRV-2（emote fan-out · Epic 3 同依赖）· S-SRV-3（room.effect.parkour · iPhone 不实施但触发条件影响 Watch 渲染）
**Standalone 判据**：独立好友系统 + 房间创建加入 + 成员展示 + 观察者叙事流可 MVP 交付；Watch 侧同屏渲染 FR23/24 归 Epic 9

---

### Epic 7: 账户 & 隐私合规 & 可观测性 MVP
**User Outcome**：User 对自己的数据有控制权（删除 / 导出 / sign out）· 设置页 identity 编辑 + 隐私政策 + push 分频道 · App Store 合规过审（PrivacyInfo.xcprivacy）· Dev 崩溃可追踪（Xcode Organizer）+ 版本可控（App Store 原生）+ kill switch（硬编码）。
**FRs covered**：FR8, FR9, FR10, FR51, FR52, FR53, FR55, FR59, FR60, FR61, FR63
**附加交付**：UX-DR9 GentleConfirmSheet（账户删除确认 · 破坏性文案业务语境 · 不可恢复警示）· Settings 入口页 + identity 编辑 + push 分频道（好友/表情 2 档 · 盲盒档已废）· EmptyStoryPrompt 5 场景模板固化（FR51 · 组件 Epic 1 已交付 · 这里收敛文案）· PrivacyInfo.xcprivacy build artifact 校验 · Log facade 6 分类补齐（network/ui/spine/health/watch/ws）· 版本检查 App Store 原生弹窗 · 远程配置 MVP 硬编码
**跨仓依赖**：S-SRV-12（账户删除 + 数据导出 API · SIWA + GDPR）· S-SRV-18（iPhone 侧发送端 Prometheus metric 打点 · 由 server 统一收集）
**Standalone 判据**：账户生命周期 + Store 合规清单完全自包含；发布前必须绿（App Store 审核硬门槛）

---

### Epic 9: 真机 × Watch 配对 × Spike 验证（Hardware & Manual · per `CLAUDE-21.6`）
**User Outcome**：Pre-MVP ship gate — 真机环境下核心体验验证通过；感官层（Spine / Lottie / haptic 辨识度）人类确认；上架合规 + alpha tester + legal sync 完成。
**FRs covered**：FR2（真机部分）· FR23 · FR24 · FR35c · FR42（Watch 渲染部分）· FR44 · FR45a · FR45b · FR46
**附加交付**：
- **Watch 配对真机流** + SIWA 真机登录 + HealthKit 真机授权（Story 2.X E9 部分）
- **Watch-only FR 真机 smoke**：FR23 同屏渲染 2-4 人 · FR24 环绕跑酷 · FR35c 开箱动画 · FR42 装扮 payoff · FR44 walk/sleep/idle 三态动作同步 · FR45a+b 久坐召唤 haptic 与盲盒震感差异 · FR46 欢迎动作
- **感官验证**：Spine / Lottie / haptic 参数人类可辨（非自动化覆盖）
- **CI**：AR1.8 GitHub Actions macOS runner（Story 1.X AC6）
- **Pre-MVP alpha tester 招募**（跨仓 sync 前置 · 来自 memory blocking item）+ Legal sync（G10）+ Store 合规审核前检查
- ~~**Spike**：Spine runtime × Swift 6 严格并发兼容性验证~~ → **Party Mode 修订**：已前置到 Epic 1 Stage 1A Story 1A.5（Winston + Murat 共识 · 风险量化：Epic 9 发现返工 3-5x · Epic 1 发现成本仅 2-3 天）
- **残留 spike 项**：AR6.4 O4 compile-time 硬验证（`#if WATCH_TRANSPORT_ENABLED` flag + CI linker symbol check 确保 release binary 真不含 `WCSession`）仍归 Epic 9，因为 release binary linker check 需要真 archive 流程

**Standalone 判据**：Epic 1-7 全部业务 Story 绿之后 Epic 9 启动；不阻塞前序 Epic 关键路径（§21.6 硬纪律）· 真机 smoke 失败不 block 单元测试持续交付

---

## Epic Summary Table

| # | Epic | FR 数 | 跨仓 gate | 依赖 |
|---|---|---|---|---|
| 1 | Foundation（Stage A+B+C）| 7 | S-SRV-15（schema 对齐 · 不实施） | — |
| 2 | Onboarding | 6 | S-SRV-15（**软 gate · 本地 dirty flag + 30 天 TTL 降级**） | E1 Stage 1A+1B |
| 3 | 我的猫 & 表情 | 4 | S-SRV-2/11/17 | E1 + **E2 Story 2.1-2.3（SIWA 完成即可并行）** |
| 4 | HealthKit × 盲盒 × 仓库 | 14 | S-SRV-7/8/9/14/16 | E1 |
| 5 | 合成 & 装扮 | 4 | S-SRV-5/10 | E4 |
| 6 | 好友 & 房间 | 16 | S-SRV-1/2/3 | E1 + E3 |
| 7 | 账户 & 合规 & 可观测 | 11 | S-SRV-12/18 | E1 |
| 9 | 真机 × Watch × 残留 Spike | 8（Watch-only 为主）| — | E1-E7 全绿 |
| **合计** | | **70** | | |

**注**：FR42 在 E5（iPhone 触发）和 E9（Watch 渲染）双归一；FR2 在 E2（SIWA + 引导）和 E9（真机配对）双归一 —— 因此 70 > 69。实际 unique FR = 69（FR36 作废不计）。

**Party Mode 修订生效点**（2026-04-21）：
1. **Epic 1 Stage A/B/C Story 拆分**（Amelia 方案）· 预估 18 Story：Stage A 7 + Stage B 8 + Stage C 5
2. **Spine × Swift 6 Spike 前置** 到 Epic 1 Stage 1A Story 1A.5（Winston + Murat 共识 · 3-5x ROI）
3. **S-SRV-15 硬 gate → 软 gate**（John 降级方案 · 本地 dirty flag + 30 天 TTL + foreground sync）
4. **Epic 2/3 并行窗口**：Story 2.3 SIWA + Watch 模式选择完成 → Epic 3 启动（John 追问确立）
5. **FailureSemantics ADR + grep gate CI**（Winston + Murat 共识 · AR10 · Story 1A.6-1A.7）
6. **WSFakeServer V1 version tag**（Murat 追加 · AR3.2 · 防止跨 Epic 级联重写）

**Growth Epic（post-MVP · 不在本 MVP 范围）**：
- **G1** Crashlytics/Sentry + in-app 强更 + 远程配置平台（FR59/60/61 升级）
- **G2** 表情扩展 3→10+ + 付费皮肤 + 富媒体信息流
- **G3** Memory Feed + 久坐召唤 iPhone 侧
- **G4** Vision（入眠仪式 / 办公室 / 50+ 暗号）

---

## Epic 1: Foundation（基石：Bootstrap + Provider + WS 稳定性基线 + Log）

**Epic Goal**：新 dev `clone → bash ios/scripts/build.sh --test` 30 min 内绿；6+6 Provider 骨架 + Fixture 三件套 ready；WSClient 重连/TTL/client_msg_id/fail-closed baseline；FailureSemantics ADR + Fail-closed grep gate 冻结语义一致性；Spine × Swift 6 spike Go/No-Go verdict 出炉。

**Story 编号约定**：Stage A（1A.1-1A.7）· Stage B（1B.1-1B.8）· Stage C（1C.1-1C.5）· 共 20 Story。

### Stage A · Bootstrap + 骨架 + Spike

### Story 1A.1: 实装 build.sh + install-hooks.sh + swift-format 链路

As an **iOS developer**,
I want **一键构建 + 测试 + lint 脚本 `bash ios/scripts/build.sh --test`**,
So that **clone 仓库后 30 min 内可跑绿并开始写业务 Story，无需逐条研究 XcodeGen / swift-format 配置**.

**Acceptance Criteria:**

**Given** 干净的 clone · 本地已 `brew install xcodegen swift-format`
**When** 运行 `bash ios/scripts/build.sh`（不带 --test）
**Then** 脚本依次执行 `xcodegen generate → xcodebuild build -scheme CatPhone -configuration Debug → swift-format lint --strict --recursive ios/`，全部退出码 0
**And** `xcodebuild build` 命令带 `OTHER_SWIFT_FLAGS=-warningsAsErrors`（零 warning 硬门槛）

**Given** 同上
**When** 运行 `bash ios/scripts/build.sh --test`
**Then** 在 build 成功后追加执行 `xcodebuild test -scheme CatPhone -destination "platform=iOS Simulator,name=iPhone 15"`，退出码 0

**Given** 全新机器未装 xcodegen 或 swift-format
**When** 运行 `bash ios/scripts/build.sh`
**Then** 脚本 fail-fast 输出人话错误（"请先 brew install xcodegen swift-format"）并退出码 ≠ 0

**Given** `ios/scripts/install-hooks.sh` 存在
**When** 运行 `bash ios/scripts/install-hooks.sh`
**Then** `.git/hooks/pre-push` 被软链或写入，执行 pre-push 时自动触发 `bash ios/scripts/build.sh`

**Given** Story 已完成
**When** 检查工程根 `ios/scripts/`
**Then** 目录下存在 `build.sh`（可执行 chmod +x）+ `install-hooks.sh`（可执行）

**FRs**：（本 Story 无直接 FR · 支撑 AR1.2/1.3/1.4）

---

### Story 1A.2: XcodeGen 配置 + 四 target 骨架 + `.xcodeproj` 忽略

As an **iOS developer**,
I want **`ios/project.yml` 承载 SSoT + 从中生成 Cat.xcodeproj 的四 target 骨架（CatPhone / CatWatch / CatShared / CatCore）**,
So that **项目结构变更只改 project.yml 不改生成物，PR diff 可读 + 避免 Cat.xcodeproj 冲突**.

**Acceptance Criteria:**

**Given** 仓库 clone 后
**When** `cd ios && xcodegen generate`
**Then** 生成 `ios/Cat.xcodeproj`，包含 **CatPhone**（iOS app · iOS 17 deployment target · Swift 5.9+）、**CatWatch**（watchOS · 独立 target · 本 PRD 不详）、**CatShared**（Swift Package from `CatShared/Package.swift`）、**CatCore**（Swift Package subfolder）

**Given** `ios/project.yml` 已配置 settings
**When** Xcode 打开生成的 project
**Then** Build Settings 含 `SWIFT_STRICT_CONCURRENCY = complete`（Swift 6 严格并发 day 1 · per Architecture D3）
**And** Deployment Target = iOS 17.0

**Given** `.gitignore` 已更新
**When** `git check-ignore ios/Cat.xcodeproj`
**Then** 退出码 0（`.xcodeproj` 被忽略 · 只追踪 `project.yml` + `Package.swift` + `Package.resolved` + `Assets.xcassets`）

**Given** XcodeGen 配置完成
**When** 运行 `bash ios/scripts/build.sh --test`
**Then** 生成 project + build + 运行占位 smoke test 全部绿

**FRs**：（本 Story 无直接 FR · 支撑 AR1.1）

---

### Story 1A.3: D1-D7 目录骨架 + `Log.*` facade（OSLog wrap · 6 分类）

As an **iOS developer**,
I want **Architecture D1-D7 要求的目录骨架 + `CatShared/Utilities/Log.swift` facade 分 `Log.network/ui/spine/health/watch/ws` 6 类 OSLog wrapper**,
So that **后续 Story 实装 Provider / Feature 时有清晰归属 + 日志输出一致（release build 禁明文敏感信息）**.

**Acceptance Criteria:**

**Given** CatShared Package 结构
**When** 检查 `CatShared/Sources/CatShared/`
**Then** 存在子目录：`Models/` · `Networking/` · `Persistence/` · `Resources/` · `UI/` · `Utilities/`

**Given** CatCore Package 结构
**When** 检查 `CatShared/Sources/CatCore/`（或独立路径）
**Then** 存在子目录：`Auth/` · `Box/` · `Health/` · `Room/` · `WS/`（占位 README.md 说明归属）

**Given** CatPhone target 结构
**When** 检查 `CatPhone/Features/`
**Then** 存在子目录：`Account/` · `Friends/` · `Inventory/` · `Dressup/` · `Room/` · `Onboarding/`（各含占位 README.md）

**Given** `CatShared/Sources/CatShared/Utilities/Log.swift` 实装
**When** 代码调用 `Log.ws.warn("..."), Log.health.info("..."), Log.ui.error("...")`
**Then** 编译通过 · 6 个分类 `network / ui / spine / health / watch / ws` 均可用 · 级别 `debug / info / notice / error / fault`

**Given** Release build（非 DEBUG）
**When** `Log.network.debug("token=\(token)")` 被调用
**Then** 通过 `#if DEBUG` 或条件编译 · release build 对敏感字段（含 "token"/"email" 子串）做 hash/脱敏（PR review 纪律）

**Given** Unit test 目标
**When** 运行 `xcodebuild test`
**Then** `LogFacadeTests.testSixCategoriesExist` 存在且通过（6 分类全 compile + 基本写入）

**FRs**：FR62

---

### Story 1A.4: Onboarding 文档三件套 + AC Triplet Lint 脚本

As a **new iOS developer joining the project**,
I want **`ios/README.md` 漏斗结构入口 + `ios/docs/dev/troubleshooting.md` + `_bmad/templates/story-ac-triplet.md` + `_bmad/lint/ac-triplet-check.sh`**,
So that **新人 Bootstrap 零 Slack 提问即可跑绿**.

**Acceptance Criteria:**

**Given** 新 dev clone 仓库 · 从未见过本项目
**When** 打开 `ios/README.md`
**Then** 文档呈漏斗结构：**Prereqs（brew / Xcode / Command Line Tools）→ Bootstrap 三步（brew install / xcodegen generate / build.sh --test）→ 跑第一个测试 → 按角色分叉（dev / QA / architect 读什么）**

**Given** `ios/docs/dev/troubleshooting.md` 存在
**When** 新 dev 遇到典型错误（XcodeGen not found / 签名失败 / 模拟器启动失败 / swift-format 报错）
**Then** 文档含对应 FAQ 条目 + 解决步骤（至少 5 个常见错误）

**Given** `_bmad/templates/story-ac-triplet.md` 存在
**When** 查看模板内容
**Then** 定义 AC 三元组 `(timeout, UI 终态, metric)` 标准：每个 AC 必须回答"超时如何处理" + "UI 显示什么" + "发何 metric 供可观测"

**Given** `_bmad/lint/ac-triplet-check.sh` 存在 · Story 文件 YAML frontmatter 含 `ac_triplet_covered: true/false`
**When** 运行 `bash _bmad/lint/ac-triplet-check.sh <story-file>`
**Then** grep AC 正文含 `timeout` + `UI` + `metric` 三关键词（或等价中文）· 缺失任一则退出码 ≠ 0

**FRs**：（本 Story 无直接 FR · 支撑 AR1.5/1.6）

---

### Story 1A.5: Spine × Swift 6 严格并发 Spike（Go/No-Go verdict）

As a **tech lead / architect**,
I want **在 Provider 接口固化前验证 `esoteric-software/spine-runtimes` 是否与 Swift 6 `-strict-concurrency=complete` 兼容 + actor boundary 能否容纳 Spine 渲染循环**,
So that **若 No-Go 可在 Epic 1 内决定替代方案（Starscream / 自研 / 降级），避免 Epic 3-5 渲染层 3-5 sprint 返工**.

**Acceptance Criteria:**

**Given** 本 Story 启动 · 预算 2-3 天
**When** 创建 Spike branch `spike/spine-swift6` + 引入 `spine-runtimes` SwiftPM dependency（锁版本）+ 编写最小 Spine 渲染 PoC（CatWatch target · 单猫走路动画）
**Then** `xcodebuild build -scheme CatWatch` 在 `SWIFT_STRICT_CONCURRENCY=complete` 下的 warning/error 数量被记录（分为：Spine SDK 内 warning / 我方代码 warning）

**Given** Spike PoC 运行
**When** 渲染循环接入 `@MainActor` Store（模拟 `WSConnection` push 触发动画切换）
**Then** 记录 actor boundary 冲突点：`@unchecked Sendable` 必要场景 / Sendable-clean 可行场景 / 完全 blocked 场景

**Given** Spike 完成
**When** 产出 `ios/CatPhone/docs/spike/spine-swift6-verdict.md`
**Then** 文档含：**Verdict（Go / No-Go / Go-with-Conditions）** + actor boundary 草图 + 必要的 `@unchecked Sendable` 逃逸舱清单（附 `// FB<issue>` 归档）+ 若 No-Go 的替代方案建议（降级到 Lottie-only / 自研最小 Spine 子集 / 推延 Watch 动画到 Growth）

**Given** Verdict = Go 或 Go-with-Conditions
**When** 后续 Epic 1 Story 1B.8（WatchTransportNoop）+ Epic 3+ 渲染 Story 启动
**Then** 遵循 verdict 文档的 actor boundary 约束

**Given** Verdict = No-Go
**When** Epic 1 计划
**Then** 在 Epic 1 内新增补救 Story（降级 / 替代方案）· 通知 PM / UX 评估对 3 创新假设（触觉广播 / Watch-主角反转）的影响

**FRs**：（本 Story 无直接 FR · Party Mode 新增 · AR1.9）

---

### Story 1A.6: `FailureSemantics.md` ADR（fail-closed vs fail-open 判据矩阵）

As an **architect / tech lead**,
I want **Epic 1 内冻结一份 `FailureSemantics` ADR 定义 fail-closed vs fail-open 判据 + observable signal 规范**,
So that **后续 Epic 2-7 的 Epic-specific 失败路径 Story 必须引用并声明符合哪个判据，防止"触发条件不对称 + 无信号"式 drift**.

**Acceptance Criteria:**

**Given** Story 启动
**When** 创建 `ios/CatPhone/docs/adr/ADR-001-failure-semantics.md`（或 `docs/architectures/adr/` · 路径与 user 确认）
**Then** ADR 含以下章节：
1. **Context**（CLAUDE-21.3 要求 fail-closed vs fail-open 决策显式）
2. **Decision Matrix**（表格 · 行=失败类型 · 列=fail-closed / fail-open / context-dependent · 至少覆盖：ws-registry fetch 失败 / WS 断线 / API 429 / HealthKit 拒绝 / SIWA 失败 / 邀请链接 5 分支 / 盲盒解锁 ACK 超时 / 合成 API 幂等失败 / 账户删除失败）
3. **Observable Signal 规范**（每个 fail-closed site 必备：`Log.<category>.error` + `Analytics.track("<event_name>")` or server metric 打点 · 遵 `S-SRV-18`）
4. **How to Apply in Stories**（AC 模板："本 Story 失败路径引用 ADR-001 §Decision Matrix row X · fail-closed/open · observable = Y"）
5. **Change Log**（ADR 变更史 · 谁 / 何时 / 为何）

**Given** ADR 内 Decision Matrix
**When** 检查 row 数量
**Then** 覆盖 PRD 明示的 9 个失败场景（上述） · 后续 Epic 若发现新场景须 bump ADR version 并 PR review

**Given** ADR 已 merge
**When** 后续 PR 涉及新增 fail-closed UI（`GentleFailView`）或 fail-open 静默降级
**Then** PR description 必须 cite ADR-001 §row · reviewer 按 ADR 验证一致性

**FRs**：（本 Story 无直接 FR · Party Mode 新增 · AR10.1）

---

### Story 1A.7: Fail-closed grep gate CI + PR template 勾选项

As an **architect / tech lead**,
I want **`ios/scripts/check-fail-closed-sites.sh` 自动扫描所有 fail-closed site 是否配对 metric 打点 + PR template 加勾选项**,
So that **新增 fail-closed site 忘记打点的低级漂移能被 CI 自动抓到（而非等 QA 手工 review）**.

**Acceptance Criteria:**

**Given** Story 启动
**When** 创建 `ios/scripts/check-fail-closed-sites.sh`（可执行 · chmod +x）
**Then** 脚本逻辑：
```bash
# 扫 Swift 源文件（含 CatShared / CatCore / CatPhone/Features · 排除 _Tests / _Spec / *UITests）
# 识别 site：包含 "GentleFailView(" 或 "Log.\w+\.error(" 或 "failClosed" 的文件
# 对每个 site · 同文件必须含 "Analytics.track(" 或 "metric." 或 "Log.*.fault("（metric 等价 facade）
# 缺失 → stderr 打印文件路径 + 行号 + 退出码 1
```

**Given** CI workflow `.github/workflows/ios.yml`（可 Epic 9 落地 · 但脚本本身 Story 1A.7 交付）
**When** PR push 触发 CI
**Then** step `check-fail-closed-sites` 运行此脚本 · 失败 block PR merge

**Given** `.github/pull_request_template.md` 已更新
**When** 开 PR
**Then** template 含勾选项：
- [ ] 新增 fail-closed site 已登记 metric 打点（引用 `ADR-001` §Decision Matrix row X）
- [ ] `bash ios/scripts/check-fail-closed-sites.sh` 本地跑绿
- [ ] 若 fail-closed 判据是新场景（ADR 未覆盖）· 已 bump ADR-001

**Given** 脚本首次落地时（项目初期无 `GentleFailView` 调用点）
**When** 运行脚本
**Then** 退出码 0（无 site · 通过）· 后续任何 Story 新增 site 立即被 gate

**FRs**：（本 Story 无直接 FR · Party Mode 新增 · AR10.2-10.3）

---

### Stage B · 核心 Provider（每 Provider 独立 Story · Sendable + Swift 6 并发随落地）

### Story 1B.1: `APIClient` protocol + `EmptyAPIClient` impl

As an **iOS developer**,
I want **`CatShared/Networking/APIClient.swift` 定义 `protocol APIClient` + 可注入的 `EmptyAPIClient` 占位实现**,
So that **Feature Story 可通过 Provider 依赖注入写 unit test 不碰真网络 · 后续 Epic 2+ 用 URLSession 替换 `EmptyAPIClient` 不改 call-site**.

**Acceptance Criteria:**

**Given** `CatShared/Sources/CatShared/Networking/` 目录
**When** 检查 `APIClient.swift`
**Then** 文件定义 `protocol APIClient: Sendable { func send<T: Decodable>(_ request: APIRequest) async throws -> T }` + `struct APIRequest` 承载 method / path / body / headers
**And** `APIClient` protocol method 为 `@MainActor` 或纯 Sendable（依 Story 1A.5 Spike verdict · Story 1A.5 未出 verdict 前默认 `@MainActor` · per Architecture D3 方案 A）

**Given** `EmptyAPIClient` 实装
**When** 调用任意 `send(_:)` 方法
**Then** 抛出 `APIError.notImplemented` 明确错误（非静默 return）· 供测试断言

**Given** APIClient 模块
**When** 运行 `xcodebuild test -scheme CatSharedTests`
**Then** `APIClientTests.testProtocolConformance` 通过（`EmptyAPIClient` 符合 `APIClient & Sendable`）
**And** `APIClientTests.testEmptyImplThrows` 通过（调用 `send` 抛 `.notImplemented`）

**Given** PR 准备 merge
**When** reviewer 检查
**Then** 无 `Alamofire` / `SwiftyJSON` / 任何第三方网络库 import（遵 PRD `PCT-arch` "不引过度依赖"）

**FRs**：（本 Story 无直接 FR · AR2.1）

---

### Story 1B.2: `WSClient` protocol + `EmptyWSClient` impl + `WSRegistry` 运行时发现

As an **iOS developer**,
I want **`CatShared/Networking/WSClient.swift` 定义 `protocol WSClient` + `WSRegistry.swift` 启动拉 `/v1/platform/ws-registry` 缓存 msg type 元数据 + `EmptyWSClient` 占位实现**,
So that **WS 消息收发路径建立 · 未知 type 优雅降级 · 后续 Story 1B.3 实装重连 baseline 前 call-site 已就位**.

**Acceptance Criteria:**

**Given** `CatShared/Sources/CatShared/Networking/` 目录
**When** 检查 `WSClient.swift` + `WSRegistry.swift` + `WSEnvelope.swift`
**Then** 三文件存在 ·
- `WSEnvelope.swift` 定义 `struct WSEnvelope: Codable, Sendable` 含 `id: String / type: String / payload: Data?`（request）+ `WSResponseEnvelope` 含 `id: String? / ok: Bool / type: String / payload: Data? / error: String?`（response）
- `WSClient.swift` 定义 `protocol WSClient: Sendable { func connect() async; func send<T>(type: String, payload: T, awaitResponse: Bool) async throws -> WSResponseEnvelope?; var messages: AsyncStream<WSEnvelope> { get } }`
- `WSRegistry.swift` 定义 `actor WSRegistry { func load() async throws; func metadata(for type: String) -> WSMessageMetadata? }` · `WSMessageMetadata` 含 `requiresDedup: Bool, requiresAuth: Bool`

**Given** `EmptyWSClient` 实装
**When** 调用 `connect()`
**Then** no-op（不抛错）· `messages` AsyncStream 立即 finish（空流）· `send(_:,_:,awaitResponse:true)` 抛 `WSError.notConnected`

**Given** `WSRegistry.load()` 被调用 + 网络成功返回 `/v1/platform/ws-registry` JSON
**When** 后续 `metadata(for: "pet.emote.broadcast")`
**Then** 返回对应 `WSMessageMetadata`（从 registry 解析）

**Given** `WSRegistry` 未 load 或 load 失败
**When** 调用 `metadata(for:)`
**Then** 返回 `nil`（调用方决定降级）· 发送未知 type 的 `send(_:)` 应 `Log.ws.warn` 并抛 `WSError.unknownMessageType(type)`

**Given** WSClient 模块
**When** 运行 `xcodebuild test`
**Then** `WSClientTests.testEmptyImplConforms` + `WSRegistryTests.testLoadAndLookup` + `WSRegistryTests.testUnknownTypeReturnsNil` + `WSRegistryTests.testUnloadedRegistryReturnsNilForAllTypes` 全部通过

**FRs**：（本 Story 无直接 FR · AR2.1 + AR4.1）

---

### Story 1B.3: `WSClient` baseline 实装：重连 + TTL + `client_msg_id` + wss-ATS + fail-closed

As an **iOS developer**,
I want **`WSClientURLSession` 实装 WSClient protocol · 含指数退避重连 + TTL GC + `client_msg_id` 300s 去重 + `wss://` 强制 + fail-closed on registry 失败**,
So that **Epic 2+ 业务 Story 的 WS 通信路径已就位 · 网络抖动/启动失败等场景由 baseline 统一处理（无需各 Epic 重复实现）**.

**Acceptance Criteria:**

**Given** `CatShared/Networking/WSClientURLSession.swift` 存在
**When** 初始化 `WSClientURLSession(url: URL, authTokenProvider: AuthTokenProvider, registry: WSRegistry)`
**Then** 内持单个 `URLSessionWebSocketTask`（per Architecture D2）· URL scheme 必须 `wss://`（release build 强断言）· `ws://` 仅 `#if DEBUG` 且 URL host 匹配配置的 debug 联调域名白名单（FR56）

**Given** WSClient 连接成功
**When** 调用 `send(type:, payload:, awaitResponse: true)`
**Then** envelope 生成 `client_msg_id = UUID v4`（per FR54）· 同 `client_msg_id` 在 300s 内重复发送 server 去重（**注**：PRD FR54 原文 60s · 这里跟 server 权威 300s · CD-drift 已登记 · Story 层以 300s 为准）

**Given** WS 断线
**When** 底层 `URLSessionWebSocketTask` 失败 / 关闭
**Then** WSClient 启动**指数退避重连**（初始 1s · max 30s · jitter ±20%）· `NFR15` 网络切换后 10s 内 ≥ 95% 成功率
**And** 重连成功后 **重发 Authorization**（调 `AuthTokenProvider.currentToken()`）→ 可选 `session.resume` → 若之前在 Room，主动 re-`room.join`（per Architecture D2 · **禁 replay `last_ack_seq`** · CD-drift 登记 与 PRD FR47 描述冲突以 D2 为准）

**Given** Server 待推事件 TTL = 24h（FR48）
**When** WS 断线超过 24h 后重连
**Then** server 返回 `session.expired` 或类似标识 · 客户端显示 fail-closed 提示"部分事件已过期，请刷新"（文案遵 UX Pattern 2 · 具体 UI 由 Epic 7 Settings Story 或相关 Feature Story 调用 `GentleFailView`）
**And** `Log.ws.error("session_ttl_expired")` + `Analytics.track("ws_session_ttl_expired")`（遵 ADR-001 judgment · AR10.1）

**Given** App 启动 · `WSRegistry.load()` 失败（网络 / 5xx / timeout）
**When** 主 App scene 加载
**Then** UI 显示全屏 fail-closed `GentleFailView`（文案"网络累了，先歇一歇"）· 主 tab 屏蔽（`NFR17`）· `Log.ws.error("ws_registry_fetch_failed")` + `Analytics.track("ws_registry_fetch_failed")`
**And** 后台每 30s 重试 · 成功后 UI 解锁主 tab（FR49）

**Given** 单元测试
**When** 运行 `xcodebuild test`
**Then** 以下测试通过（基于 `WSFakeServer` fixture · Story 1C.2 交付后完整跑 · 本 Story 先跑 mock 子集）：
- `WSClientTests.testReconnectExponentialBackoff`
- `WSClientTests.testClientMsgIdDedupWithin300s`
- `WSClientTests.testWssSchemeEnforcedInRelease`
- `WSClientTests.testTTLExpiredShowsFailClosed`
- `WSClientTests.testReconnectReauthorizesAndRejoinsRoom`
- `WSClientTests.testRegistryFailureTriggersFullScreenFailClosed`

**FRs**：FR47, FR48, FR49, FR54, FR56

---

### Story 1B.4: `AuthTokenProvider` protocol + Keychain 集成

As an **iOS developer**,
I want **`CatShared/Networking/AuthTokenProvider.swift` 定义 protocol + `KeychainAuthTokenProvider` 实装 JWT / refresh token 存储于 Keychain**,
So that **Auth token 生命周期可被 WSClient / APIClient 依赖 · 遵 PRD FR57 禁 UserDefaults 存 token**.

**Acceptance Criteria:**

**Given** `AuthTokenProvider.swift` 存在
**When** 检查 protocol 定义
**Then** `protocol AuthTokenProvider: Sendable { func currentToken() async throws -> String?; func setToken(_ token: String, refresh: String?) async throws; func clearTokens() async throws; var tokenDidChange: AsyncStream<AuthToken?> { get } }`
**And** `struct AuthToken: Sendable { let jwt: String; let refresh: String?; let issuedAt: Date }`

**Given** `KeychainAuthTokenProvider` 实装
**When** 调用 `setToken("jwt-xxx", refresh: "refresh-yyy")`
**Then** 通过 `SecItemAdd` / `SecItemUpdate` 写入 Keychain · `kSecClass = kSecClassGenericPassword` · `kSecAttrService = "com.cat.catphone.auth"`（FR57）
**And** `kSecAttrAccessible = kSecAttrAccessibleAfterFirstUnlock`（设备解锁后可访问 · 适配后台 WS 重连）

**Given** Keychain 写入成功
**When** 调用 `currentToken()`
**Then** 从 Keychain 读出 JWT 返回

**Given** 用户 sign out（由 Epic 7 Story 调用）
**When** 调用 `clearTokens()`
**Then** `SecItemDelete` 清除两个 Keychain item · 后续 `currentToken()` 返回 `nil`

**Given** 禁 UserDefaults 纪律
**When** grep 本 Story 产物 `grep -r "UserDefaults.*token\|UserDefaults.*jwt" CatShared/`
**Then** 零命中（PR review 验证）

**Given** 单元测试
**When** 运行 `xcodebuild test -scheme CatSharedTests`
**Then** `KeychainAuthTokenProviderTests` 全套通过（使用 `UserDefaultsSandbox` + Keychain mock · Fixture 来自 Story 1C.2）：
- `testSetAndGetToken`
- `testClearTokens`
- `testTokenDidChangeStreamEmitsOnUpdate`
- `testClearTokensEmitsNilOnStream`

**FRs**：FR57（+ FR10 sign out 场景由 Epic 7 Story 调用本 Provider）

---

### Story 1B.5: `LocalStore` protocol + `EmptyLocalStore` impl + SwiftData 占位

As an **iOS developer**,
I want **`CatShared/Persistence/LocalStore.swift` 定义 protocol + `EmptyLocalStore` 占位 + SwiftData 分层（将来 Epic 4 仓库用）**,
So that **非 token 持久化能力走统一 Provider · UI 级状态（UserDefaults）与业务数据（SwiftData）路径明确分离**.

**Acceptance Criteria:**

**Given** `LocalStore.swift` 存在
**When** 检查 protocol 定义
**Then** `protocol LocalStore: Sendable { func get<T: Codable>(_ key: String, as type: T.Type) async throws -> T?; func set<T: Codable>(_ key: String, value: T) async throws; func delete(_ key: String) async throws }`

**Given** `EmptyLocalStore` 实装
**When** 调用任意方法
**Then** `get` 返回 `nil` · `set` / `delete` no-op（不抛错）

**Given** PRD 定义持久化分工
**When** 检查本 Story 产物
**Then** `LocalStore` 只承载非权威 UI-level 持久化（如 "onboarding_complete_at_local" dirty flag · Epic 2 用）· **绝不**存 token（由 `AuthTokenProvider` 专司）· **绝不**作为业务数据 SSoT（业务数据走 SwiftData · 或 server 权威）

**Given** SwiftData 分层占位
**When** 检查 `CatShared/Persistence/`
**Then** 创建 `SwiftDataContainer.swift` 占位（含 `@ModelActor` 骨架 · 实装待 Epic 4 仓库 Story）

**Given** 单元测试
**When** 运行 `xcodebuild test -scheme CatSharedTests`
**Then** `LocalStoreTests.testEmptyImplReturnsNilForGet` + `testEmptyImplNoOpForSet` 通过

**FRs**：（本 Story 无直接 FR · AR2.1）

---

### Story 1B.6: `AuthService` protocol + `EmptyAuthService` impl

As an **iOS developer**,
I want **`CatCore/Auth/AuthService.swift` 定义 protocol · 承载 SIWA 登录业务逻辑编排（Epic 2 会实装）**,
So that **Auth 业务层与底层 APIClient/TokenProvider 解耦 · Epic 2 SIWA Story 直接实装此 protocol**.

**Acceptance Criteria:**

**Given** `CatShared/Sources/CatCore/Auth/AuthService.swift` 存在
**When** 检查 protocol
**Then** `protocol AuthService: Sendable { func signInWithApple(nonce: String) async throws -> Session; func signOut() async throws; var currentSession: Session? { get async } }` · `struct Session: Sendable { let userId: String; let jwt: String; let refreshToken: String?; let expiresAt: Date }`

**Given** `EmptyAuthService` 实装
**When** 调用 `signInWithApple(nonce:)`
**Then** 抛 `AuthError.notImplemented`

**Given** `@Observable` `AuthSession` in CatCore（AppCoordinator 层 · per Architecture D1）
**When** 检查 `CatShared/Sources/CatCore/Auth/AuthSession.swift`
**Then** 定义 `@Observable @MainActor final class AuthSession { var session: Session? }` · 根 `@Environment` 注入占位就绪

**Given** 单元测试
**When** 运行 `xcodebuild test -scheme CatCoreTests`
**Then** `AuthServiceTests.testEmptyImplThrows` + `AuthSessionTests.testInitialStateNil` 通过

**FRs**：（本 Story 无直接 FR · AR2.1 · 为 FR1/FR10 铺垫）

---

### Story 1B.7: `BoxStateStore` protocol + `EmptyBoxStateStore` impl

As an **iOS developer**,
I want **`CatCore/Box/BoxStateStore.swift` 定义 protocol · 承载盲盒状态流（pending / unlocking / unlocked / unlocked_pending_reveal）**,
So that **Epic 4 盲盒 UI Story 可 `for await state in boxStateStore.state` 消费流状态 · Empty 占位让 Epic 1 不强跑 server**.

**Acceptance Criteria:**

**Given** `CatShared/Sources/CatCore/Box/BoxStateStore.swift` 存在
**When** 检查 protocol
**Then** `protocol BoxStateStore: Sendable { var state: AsyncStream<BoxState> { get }; func refresh() async throws }`
**And** `enum BoxState: Sendable, Equatable { case empty; case pending(progress: Int, total: Int, droppedAt: Date); case unlocking; case unlockedPendingReveal(box: PendingBox); case unlocked(skin: Skin) }` · `PendingBox` / `Skin` 为占位 `Sendable` struct（字段可空）

**Given** `EmptyBoxStateStore` 实装
**When** 订阅 `state` AsyncStream
**Then** 立即 yield `BoxState.empty` 然后 finish（Epic 1 无真盒子状态）· `refresh()` no-op

**Given** Architecture D3 actor isolation 决策
**When** 检查 `EmptyBoxStateStore` 实装
**Then** 若 Spike 1A.5 verdict = Go-with-Conditions 则按 verdict 的 actor/Sendable 方案（常为 `actor EmptyBoxStateStore { }`）· 否则默认 `@MainActor` + continuation（per D3 方案 A）

**Given** 单元测试
**When** 运行 `xcodebuild test -scheme CatCoreTests`
**Then** `BoxStateStoreTests.testEmptyImplYieldsEmptyThenFinishes` 通过

**FRs**：（本 Story 无直接 FR · AR2.1 · 为 FR35a/b/d 铺垫）

---

### Story 1B.8: `WatchTransport` protocol + `WatchTransportNoop` impl（哲学 B 降级版）

As an **iOS developer**,
I want **`CatShared/Networking/WatchTransport.swift` 定义 protocol + `WatchTransportNoop` 唯一 MVP 实现（`isReachable = false` · `send = no-op`）**,
So that **MVP 不引 `WCSession` 依赖（哲学 B · 跨端走 server WS 为主）· binary 纤细 · Growth 阶段可换 `WatchTransportWCSession` impl 不改 call-site**.

**Acceptance Criteria:**

**Given** `CatShared/Sources/CatShared/Networking/WatchTransport.swift` 存在
**When** 检查 protocol
**Then** `protocol WatchTransport: Sendable { var isReachable: Bool { get async }; func send(context: [String: Sendable]) async throws; func sendMessage(_ message: [String: Sendable]) async throws }` · 三档通道 API 预留（MVP 不启用）

**Given** `WatchTransportNoop` 实装
**When** 调用任一方法
**Then** `isReachable` 返回 `false` · `send(_:)` / `sendMessage(_:)` no-op（不抛 · 不实际发送）

**Given** `import WatchConnectivity` 纪律
**When** grep `grep -r "import WatchConnectivity" CatPhone/` （release / non-DEBUG 部分）
**Then** **零命中** · 仅允许在 `#if WATCH_TRANSPORT_ENABLED`（Growth flag · 默认 off）下 import

**Given** Release build binary
**When** Epic 9 Story 执行 linker symbol check（AR6.4 · O4 归 Epic 9）
**Then** binary 不含 `WCSession` 符号（本 Story 的纪律前置保证 · Epic 9 linker check 是最终 gate）

**Given** 单元测试
**When** 运行 `xcodebuild test -scheme CatSharedTests`
**Then** `WatchTransportNoopTests.testIsReachableFalse` + `testSendNoOp` 通过 · `FakeWatchTransport` spy impl 占位在 Story 1C.2

**FRs**：（本 Story 无直接 FR · AR2.2 + AR5.5 · 为 FR23/24/42/44/45a/46 Epic 9 Watch-only FR 铺垫）

---

### Stage C · 测试基础设施（Epic 2+ 硬 gate · 独立交付可验证）

### Story 1C.1: 6 测试 protocol 骨架（ClockProtocol / IdleTimer / Accessibility / FirstTime / KeyValue / WatchTransport 测试面）

As an **iOS developer writing Feature stories**,
I want **6 测试抽象 protocol 全部定义 + Default/Empty impl**,
So that **Feature Story 能 fast-forward clock / stub idle timer / spy accessibility announcements / isolate UserDefaults 副作用 · `xcodebuild test` 一命令绿不依赖真时钟/设备**.

**Acceptance Criteria:**

**Given** `CatShared/Sources/CatShared/Utilities/` 目录（或 `CatCore` · 依 Story 1A.3 目录骨架）
**When** 检查 6 protocol 文件
**Then** 存在：
- `ClockProtocol.swift` · `protocol ClockProtocol: Sendable { var now: Date { get async }; func sleep(for duration: Duration) async throws }` + `struct SystemClock: ClockProtocol`（产线用）
- `IdleTimerProtocol.swift` · `protocol IdleTimerProtocol: Sendable { func setDisabled(_ disabled: Bool) async }` + `struct SystemIdleTimer` wrap `UIApplication.shared.isIdleTimerDisabled`
- `AccessibilityAnnouncer.swift` · `protocol AccessibilityAnnouncer: Sendable { func announce(_ text: String) async }` + `struct UIKitAccessibilityAnnouncer` wrap `UIAccessibility.post(notification: .announcement)`
- `FirstTimeExperienceTracker.swift` · `protocol FirstTimeExperienceTracker: Sendable { func hasSeen(_ key: FirstTimeKey) async -> Bool; func markSeen(_ key: FirstTimeKey) async }` + `enum FirstTimeKey: String, Sendable { case onboardingCompleted, firstEmoteSent, firstRoomEntered(String)... }`
- `KeyValueStore.swift` · `protocol KeyValueStore: Sendable { func int(_ key: String) async -> Int?; func set(_ key: String, value: Int) async; func string(_ key: String) async -> String?; func set(_ key: String, value: String) async }` · UserDefaults 薄封装 · 供 UI 级状态（如 `emoji_send_count_v1`）
- （`WatchTransport` 已在 1B.8 定义 · 此 Story 只补测试面 `FakeWatchTransport` 占位 · 真 impl 在 1C.2 Fixture 中）

**Given** 每个 protocol
**When** 检查默认产线 impl
**Then** `SystemClock` / `SystemIdleTimer` / `UIKitAccessibilityAnnouncer` / `UserDefaultsFirstTimeExperienceTracker`（基于 Story 1B.4 `AuthTokenProvider` 存 token · Tracker 用 UserDefaults 存 milestone local dirty flag · Epic 2 会切 server 权威 `user_milestones`）/ `UserDefaultsKeyValueStore` 实装完成

**Given** `FirstTimeExperienceTracker` 实装
**When** `UserDefaultsFirstTimeExperienceTracker.markSeen(.onboardingCompleted)`
**Then** 同步写入**本地 dirty flag**（带 `onboardingCompletedLocalTimestamp` + `onboardingCompletedSyncedWithServer: false`）· 供 Epic 2 Story 2.X S-SRV-15 软 gate 降级方案使用（AR7.15 + Epic 2 修订）

**Given** 单元测试
**When** 运行 `xcodebuild test -scheme CatSharedTests`
**Then** 6 protocol 的 `ProtocolConformanceTests` 全通过 · `ClockProtocolTests.testSystemClockNowMonotonic` + `FirstTimeExperienceTrackerTests.testMarkSeenWritesDirtyFlag` 通过

**FRs**：（本 Story 无直接 FR · AR2.2 · 为 Epic 2 S-SRV-15 软 gate 降级铺垫）

---

### Story 1C.2: Fixture 三件套 · `UserDefaultsSandbox` + `WSFakeServer V1` + `HapticSpy`

As an **iOS developer writing integration-style unit tests**,
I want **三件套 fixture 可在任意 test 内 setUp/tearDown · WS msg type envelope-agnostic · Haptic 调用 record-only 不触发真振动**,
So that **Epic 2+ Feature Story 的 integration [I] 测试可用同一 fixture · 不污染真 UserDefaults · WSFakeServer 接口冻结 V1 后跨 Epic 不改写**.

**Acceptance Criteria:**

**Given** `CatSharedTests/Fixtures/UserDefaultsSandbox.swift` 存在
**When** Test setUp `let sandbox = UserDefaultsSandbox()` + tearDown `sandbox.clear()`
**Then** sandbox 内部用 `UserDefaults(suiteName: UUID().uuidString)` · 所有 `KeyValueStore` / `FirstTimeExperienceTracker` 注入 sandbox · tearDown 清理不影响其他 test

**Given** `CatSharedTests/Fixtures/WSFakeServer.swift` 存在 · 标注 `/// @Version V1 · 跨 Epic 不改写此接口 · 修改须 bump 到 V2`
**When** 构造 `let server = WSFakeServerV1(url: URL)` + `let client = WSClientURLSession(...)` 连上
**Then** fixture 是 **envelope-agnostic**（构造函数接 `envelope: Data` 不硬编码任何 msg type · per AR3.2 · 防 Murat 警告的跨 Epic 级联重写）
**And** 支持 `server.replay(jsonlFile: URL)` 从 jsonl fixture 文件批量回放 msg（fixture 源于 `docs/api/ws-message-registry.md` 生成 · Story 1C.4 的 `check-ws-fixtures.sh` 保证 fixture 与 registry 同步）
**And** 测试须覆盖 **unknown type default branch**（server 发 "future.unknown.type" · WSClient 应走 `WSError.unknownMessageType` + `Log.ws.warn` 不崩）

**Given** `CatSharedTests/Fixtures/HapticSpy.swift` 存在
**When** Test 注入 `HapticSpy`（替换 `UIImpactFeedbackGenerator` 调用）
**Then** 所有 haptic 调用被 record 到 `spy.events: [HapticEvent]`（含 `style / intensity / timestamp`）· test 可 assert `spy.events.count == 2 && spy.events[0].style == .medium`
**And** HapticSpy 实际**不触发**真振动（record-only）

**Given** `FakeWatchTransport` 在 `CatSharedTests/Fixtures/`
**When** Test 注入 `FakeWatchTransport(reachable: true)` + 调用 `send(context: [:])`
**Then** fake 实现 record 到 `fake.sentContexts: [[String: Sendable]]` · test 可断言 `fake.sentContexts.first?["last_skin_id"] == "blue-01"`

**Given** 单元测试
**When** 运行 `xcodebuild test -scheme CatSharedTests`
**Then** 以下通过：
- `UserDefaultsSandboxTests.testSandboxIsolated`
- `WSFakeServerV1Tests.testReplayJsonl`
- `WSFakeServerV1Tests.testUnknownTypeDefaultBranch`
- `HapticSpyTests.testEventsRecorded`
- `FakeWatchTransportTests.testContextsSpied`

**Given** V1 接口冻结
**When** 后续 Epic PR 要改动 `WSFakeServerV1` public API
**Then** PR description 必须说明 bump V2 理由 + 列出影响的 Epic 数 · reviewer 按 AR3.2 version tag 纪律验证

**FRs**：（本 Story 无直接 FR · AR3.1-3.3）

---

### Story 1C.3: `DualSSoTScenarioFixture`（HealthKit stub + WSFakeServer 时序编排）

As an **iOS developer writing Epic 4 HealthKit × 盲盒 integration 测试**,
I want **`DualSSoTScenarioFixture` 一条 API 即可编排"本地步数 =100 / server 权威 =98 / 延迟 2s 后追平"的双 SSoT 漂移窗口**,
So that **Epic 4 每个 Story 不重造时序编排代码 · 漂移 edge case 在 Epic 1 一次钉死**.

**Acceptance Criteria:**

**Given** `CatSharedTests/Fixtures/DualSSoTScenarioFixture.swift` 存在
**When** Test 调用：
```swift
let scenario = DualSSoTScenarioFixture(
    localSteps: 100,
    serverSteps: 98,
    convergeAfter: .seconds(2),
    clock: mockClock,
    wsServer: wsFake
)
scenario.start()
await mockClock.advance(by: .seconds(3))
```
**Then** fixture 内部协同：
- `HealthKit stub`（wrap `StepDataSource` protocol 的 `InMemoryStepDataSource` · Story 1C.1 相关）yield `100 steps`
- `WSFakeServerV1` 按时序推 `box.progress.update { server_steps: 98 }` 延迟 0s · 然后推 `box.progress.update { server_steps: 100 }` 延迟 2s
- 测试可断言 BoxStateStore state 按序经过 `pending(98)` → `pending(100)`

**Given** Scenario 支持 3 种典型漂移
**When** 检查 fixture API
**Then** 提供 preset：
- `.localAheadServerBehind(localSteps, serverSteps, convergeAfter)`
- `.localBehindServerAhead(localSteps, serverSteps, convergeAfter)`
- `.serverSilent(localSteps, untilTimeout)`（server 完全无响应 · Epic 4 FR34 "已达成·等待确认"验证）

**Given** 单元测试 + mock `ClockProtocol`
**When** 运行 `xcodebuild test -scheme CatSharedTests`
**Then** `DualSSoTScenarioFixtureTests`：
- `testLocalAheadServerBehindConvergence` 通过
- `testServerSilentTriggersPendingAckUI`（断言 FR34 UI 文案 trigger）通过
- `testScenarioTearDownCleansState` 通过

**Given** Epic 4 Story 启动
**When** 查阅本 fixture 文档
**Then** 文档含**典型使用模板**（boilerplate 最少 20 行起可跑 · 避免每 Story 重写编排）

**FRs**：（本 Story 无直接 FR · AR3.4 · 为 Epic 4 FR32/34/35a/b/d 铺垫）

---

### Story 1C.4: `docs/contract-drift-register.md` 登记 + `check-ws-fixtures.sh` dev-time 脚本

As an **architect / tech lead**,
I want **repo 根 `docs/contract-drift-register.md` 承载跨仓契约漂移登记 · `ios/scripts/check-ws-fixtures.sh` dev-time 脚本对比本地 fixture 与 server WS registry hash**,
So that **已知 drift（PRD FR54 dedup 60s vs server 300s / FR47 `last_ack_seq` replay vs D2 `session.resume`）被显式登记 · fixture 漂移能被 dev 手动检测不依赖 CI 跨仓节奏**.

**Acceptance Criteria:**

**Given** Story 启动
**When** 创建 `docs/contract-drift-register.md`（repo 根 docs/ · 不属任一端 repo · 遵 Architecture C3）
**Then** 文档含 markdown 表格：
```
| ID | 项 | iOS 值 | Server 值 | 上游权威 | 状态 | Owner | Issue | 登记日期 |
```
**And** 预填入当前已知 7 项 drift：
- `CD-01` dedup TTL（60s vs 300s）· 上游=server · 状态=open · Owner=@who · 2026-04-21
- `CD-02` WS 重连 replay 策略（`last_ack_seq` vs `session.resume+room.join`）· 上游=Architecture D2 · 状态=accepted · Owner=@Winston · 2026-04-21
- `CD-03..07` 其余从 PRD/Architecture 已知漂移（每条 ≤ 200 字 context）

**Given** 登记的 drift 每条
**When** 检查 ID 下详情链接
**Then** 附 context section ≤ 200 字说明：**影响范围** / **当前处理** / **何时收敛**

**Given** `ios/scripts/check-ws-fixtures.sh` 存在
**When** 运行（dev 手动 · 不进 CI）
**Then** 脚本逻辑：
```bash
# 拉 server repo docs/api/ws-message-registry.md 的 SHA / hash
# 对比本地 CatSharedTests/Fixtures/ws-*.jsonl 的 snapshot
# 若 hash 不匹配 → stdout 打印"WS registry 已变更，请更新 fixture + 检查 msg schema"（dev 提醒不阻塞）
```
**And** 脚本明确标注"**仅 dev-time · 不进 CI**"（AR4.3 · 防跨仓节奏绑架）

**Given** PR 新增 WS msg type 并使用
**When** dev 本地跑脚本
**Then** 若本地 fixture 未同步 server registry 变更，脚本提醒 · dev 决定是否跟进

**FRs**：（本 Story 无直接 FR · AR4.2-4.3）

---

### Story 1C.5: CatShared/UI 基础组件（EmptyStoryPrompt / GentleFailView / SkeletonView / LottieView / HintLabel）

As an **iOS developer building Feature UIs in Epic 2+**,
I want **5 个跨端共用 UI 组件在 Epic 1 就位 · 遵 UX-DR2+9 · 基于 DesignTokens 单文件**,
So that **Feature Story 不重复造空状态 / fail-closed / skeleton · UX Pattern 1/2/5 骨架语义一致**.

**Acceptance Criteria:**

**Given** `CatShared/Sources/CatShared/Resources/DesignTokens.swift` 存在
**When** 检查 tokens
**Then** 含 UX-DR1 的 6 维度：色（奶油/米色主调 + 陶土/蜜桃强调 + 稀有度 6 级 + 功能色 mint/honey/coral）· 字（SF Pro + Dynamic Type · 5 级语义字号 禁硬编码 pt）· 间距（8pt 栅格 8/12/16/20/24/32/48）· 圆角（卡片 16/按钮 12/chip 8/sheet 24）· 阴影（极轻 Y=2 blur=8 opacity=6%）· 动画 token（默认 0.35s easeInOut / 仪式 1.5s / 呼吸 4s / 抬眼 0.6s）

**Given** `CatShared/Sources/CatShared/UI/EmptyStoryPrompt.swift` 存在（UX-DR2）
**When** 使用 `EmptyStoryPrompt(icon: Image(systemName: "person.2"), title: "你还没有朋友", subtitle: "扫码把 TA 拉回家，一起挂机", cta: ("邀请朋友", onTap))`
**Then** 基于 `ContentUnavailableView` · 兼容 iOS 17 + watchOS 10 · VoiceOver label 合并 title+subtitle+cta 读出

**Given** `CatShared/Sources/CatShared/UI/GentleFailView.swift` 存在
**When** 使用 `GentleFailView(reason: .wsRegistryFailed, retry: onRetry)`
**Then** 内置 ADR-001 §Decision Matrix 对应场景文案（6 触发场景 UX Pattern 2）· Lottie 图占位（Story 1A.3 Log facade 报错时调 Lottie）· 按钮文案业务语境（禁"抱歉/失败/错误/无法/请稍候/感叹号"· Pattern 2 铁律）· 永远有去处（重试按钮或说明可稍后）

**Given** `CatShared/Sources/CatShared/UI/SkeletonView.swift` 存在
**When** 使用 `.redacted(reason: .placeholder)` 或 `SkeletonView(shape: .rect(cornerRadius: 16))`
**Then** 渲染 cream/200 色背景 + 隐约 shimmer · Pattern 5 "首选骨架屏" 遵守

**Given** `CatShared/Sources/CatShared/UI/LottieView.swift` 存在（UX-DR2）
**When** 使用 `LottieView(animation: .named("cat_gaze_onboarding"), loop: false, onComplete: ...)`
**Then** 读 `@Environment(\.accessibilityReduceMotion)` · reduce-motion 时显首帧静帧停留 UX v0.3 指定时长（UX-DR14 · 动画 token）
**And** Lottie `.json` 资产放 `CatShared/Sources/CatShared/Resources/Animations/`（Story 1A.3 目录骨架已就位）

**Given** `CatShared/Sources/CatShared/UI/HintLabel.swift` 存在
**When** 使用 `HintLabel("轻点跟它打招呼")`
**Then** 用 `ink/500` 语义色 + `.caption` 语义字号（禁 `opacity(0.5)` · Pattern 2 + UX-DR15）

**Given** 组件单元测试 + 无障碍测试
**When** 运行 `xcodebuild test -scheme CatSharedTests`
**Then** 以下通过：
- `EmptyStoryPromptTests.testAccessibilityLabelMergesAllFields`
- `GentleFailViewTests.testNoForbiddenWordsInCopy`（grep 断言 "抱歉/失败/错误/无法" 零命中）
- `GentleFailViewTests.testAllReasonsHaveCopy`（每个 `FailReason` enum case 有对应文案）
- `LottieViewTests.testReduceMotionRendersStaticFrame`
- `HintLabelTests.testSemanticFontAndColor`

**Given** PR review 纪律
**When** 任何 Feature Story 新增 UI 元素
**Then** 优先复用本 Story 5 组件 · 偏离须在 PR 描述解释（UX-DR17 组件复用纪律）

**FRs**：（本 Story 无直接 FR · UX-DR1+2+9 · 为 FR51 空状态 + UX Pattern 1/2/5 铺垫）

---

## Epic 2: Onboarding（首开仪式：叙事 + SIWA + Watch 模式 + 撕包装 + 命名）

**Epic Goal**：新用户首次打开 app → 3 屏叙事 → SIWA 登录 → Watch 配对 / 观察者模式二选一 → 撕包装仪式 → 猫命名 · 完成"我拥有一只猫"首次感情投资。S-SRV-15 软 gate 降级（本地 dirty flag + 30 天 TTL + foreground sync）保证 server 团队排期解耦。

**Story 编号**：2.1-2.5 共 5 Story · **Epic 2/3 并行窗口**：Story 2.1-2.3 完成后 Epic 3 启动条件满足 · Story 2.4-2.5 可与 Epic 3 并行

### Story 2.1: 3 屏叙事 Onboarding 卡片（NarrativeCardCarousel · 含 Slogan + Skip）

As a **new User opening the app for the first time**,
I want **看 3 屏叙事卡片（含核心 Slogan "你的猫在家等你走到它身边"）· 可向左滑切换 / 跳过**,
So that **未登录前先理解产品定位（"配件端而非阉割版"哲学· 不是又一个游戏化健身 app）· 不被强迫完成漫长 Onboarding（FR6+NFR24）**.

**Acceptance Criteria:**

**Given** 全新用户首次打开 app
**When** 主 Scene 加载且 `AuthSession.session == nil` 且 `FirstTimeExperienceTracker.hasSeen(.onboardingNarrativeCompleted) == false`
**Then** 渲染 `NarrativeCardCarousel`（CatPhone/Features/Onboarding/）· 3 屏 paged TabView · 每屏含图 + 一句叙事文案 + 进度指示器
**And** 第 1 屏 Slogan 显示"你的猫在家等你走到它身边"（FR6）

**Given** 用户在 Onboarding 卡片
**When** 用户向左/右滑动
**Then** 卡片切换 · 进度指示器更新 · 切换动画 0.35s easeInOut（DesignTokens 默认 token）

**Given** 用户在任意 Onboarding 卡片屏
**When** 顶部"跳过"按钮可见 + 点击
**Then** 跳到 Story 2.2 SIWA 登录屏（NFR24 允许 skip 不强制）· 写本地 `KeyValueStore.set("onboarding_narrative_skipped_count", value: getCount + 1)`（埋点 · 后续可观测哪屏跳过率最高）

**Given** 用户滑到第 3 屏并点 "进入"
**When** 切换屏
**Then** 调 `FirstTimeExperienceTracker.markSeen(.onboardingNarrativeCompleted)` · 导航到 Story 2.2 SIWA 屏

**Given** 用户已完成 Onboarding 后再次开 app（已 sign out 但 milestone 已完成）
**When** 主 Scene 加载
**Then** 跳过卡片直接进 Story 2.2 SIWA · 不重复展示叙事卡片（FirstTimeExperienceTracker 防重）

**Given** Reduce Motion accessibility 开启
**When** 切换卡片
**Then** 切换动画降级为瞬切（无 0.35s easeInOut · UX-DR14）

**Given** Dynamic Type `.accessibilityXXXLarge`
**When** 文案渲染
**Then** 文字 `.allowsTightening(true)` + `.minimumScaleFactor(0.9)` · 不裁剪不截断（UX-DR12）

**Given** 单元 / UI 测试
**When** 运行 `xcodebuild test`
**Then** 以下通过：
- `NarrativeCardCarouselTests.testThreeCardsRender`
- `NarrativeCardCarouselTests.testSloganOnFirstCard`
- `NarrativeCardCarouselTests.testSkipNavigatesToSIWA`
- `NarrativeCardCarouselTests.testCompletionMarksFirstTimeKey`
- `NarrativeCardCarouselTests.testReduceMotionSkipsAnimation`

**FRs**：FR6 → C-UX-01

---

### Story 2.2: SIWA 登录（FR1 · NFR10 nonce 防 replay）

As a **User who has finished narrative cards (or skipped)**,
I want **用 Apple ID 一键登录 · 无需创建用户名密码**,
So that **零认知负担进入产品 · 符合 App Store 4.8 SIWA 优先（NFR33）**.

**Acceptance Criteria:**

**Given** Story 2.1 已完成或跳过 · 用户在 SIWA 屏
**When** 屏幕渲染
**Then** 显示 `SignInWithAppleButton`（系统原生 SwiftUI · `.continue` 模式）· 文案"用 Apple 继续"

**Given** 用户点击 SIWA 按钮
**When** SIWA 流程触发
**Then** 客户端**先生成随机 nonce**（`UUID().uuidString` 或 SecureRandom）· 传给 `SignInWithAppleButton` request `request.nonce = nonce`（NFR10 防 replay）
**And** Apple 返回 `ASAuthorizationAppleIDCredential` 含 `identityToken` · 客户端调 `AuthService.signInWithApple(nonce: nonce)` 实装（CatCore 层 · 替换 Story 1B.6 EmptyAuthService）

**Given** `AuthService.signInWithApple` 内部
**When** POST `/auth/apple` 含 `{ identityToken, nonce }`
**Then** server 验证 nonce + identityToken · 返回 JWT + refreshToken + userId · 客户端调 `AuthTokenProvider.setToken(jwt, refresh: refreshToken)` 写 Keychain（FR57）
**And** 更新 `AuthSession.session = Session(...)` · `@Observable` 触发 UI 导航到 Story 2.3 Watch 模式选择

**Given** SIWA 失败（用户取消 / Apple 验证失败 / 网络错误）
**When** 错误抛出
**Then** UI 显示 `GentleFailView(reason: .siwaFailed, retry: ...)`（文案"账号没连上"+ 按钮"再试一次" · UX Pattern 2 · ADR-001 §Decision Matrix）
**And** `Log.network.error("siwa_failed: \(reasonHash)")` + `Analytics.track("siwa_failed")`（fail-closed observable signal · AR10.2 grep gate 验证）

**Given** SIWA 后 Apple 返回 nonce 与客户端原值不匹配
**When** 客户端校验
**Then** 拒绝 session · 显示 `GentleFailView(reason: .siwaNonceInvalid, retry: ...)` · `Log.network.fault("siwa_nonce_mismatch")` + `Analytics.track("siwa_nonce_mismatch")`（高严重度 · 可能 replay 攻击）

**Given** Onboarding 流程后续 milestone（onboardingCompleted）
**When** 用户完成 Story 2.5 命名后
**Then** 调 `FirstTimeExperienceTracker.markSeen(.onboardingCompleted)` 触发 S-SRV-15 软 gate 同步流程（详见 Story 2.5）

**Given** 单元 / UI 测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `SIWAFlowTests.testNonceGeneratedAndPassed`
- `SIWAFlowTests.testSuccessWritesKeychainAndUpdatesSession`
- `SIWAFlowTests.testFailureShowsGentleFailView`
- `SIWAFlowTests.testNonceMismatchShowsFault`
- `AuthServiceTests.testRealImplReplacesEmptyAuthService`

**FRs**：FR1 + NFR10 + FR57（写 Keychain · 复用 Story 1B.4）

---

### Story 2.3: Watch 模式选择（WatchModePicker · FR2 引导 + FR3 观察者模式）

As a **User who has just signed in via SIWA**,
I want **明确二选一：'我有 Apple Watch · 引导我安装 CatWatch app' / '我没有 / 先进来看看（观察者模式）'**,
So that **路径分叉立即清晰 · 无 Watch 用户不被阻断（FR3）· 有 Watch 用户被引导到 CatWatch 安装链路（FR2）**.

**Acceptance Criteria:**

**Given** Story 2.2 SIWA 完成 · `AuthSession.session != nil`
**When** 屏幕渲染
**Then** 显示 `WatchModePicker` 二选一 UI · 上 CTA "我有 Apple Watch" · 下 CTA "先进来看看"
**And** 文案不暗示观察者模式是降级（避免用户内疚 · UX 哲学：iPhone-only 是护城河燃料 J2）

**Given** 用户选 "我有 Apple Watch"
**When** 点击
**Then** 跳转到 CatWatch 引导屏 · 显示 App Store 按钮（深链 `itms-apps://...catwatch_id`）+ 文字"Watch 端独立 SIWA 登录后会自动关联"
**And** 写 `KeyValueStore.set("user_watch_mode", value: "with_watch")` · 不阻塞 · 用户可 "我去装了" 按钮直接进 Story 2.4 撕包装

**Given** 用户选 "先进来看看"
**When** 点击
**Then** 进入观察者模式标识：`KeyValueStore.set("user_watch_mode", value: "observer_only")`
**And** 直接进 Story 2.4 撕包装（撕包装在两种模式都展示 · 是 Tap-into-product 仪式）
**And** 后续 Epic 6 房间 Story 检查此 flag 应用观察者隐私 gate（FR25 联动）

**Given** Server 端 SIWA 自动关联（FR2）
**When** 用户在 Watch 端独立完成 SIWA（Epic 9 真机验证）
**Then** server 根据同一 SIWA user_id 自动关联双端（不需要客户端额外操作）· 客户端只显示 onboarding 提示

**Given** 用户已选择 Watch 模式后 sign out 重登
**When** 再次完成 SIWA
**Then** 不重复显示 Watch 模式选择屏（`KeyValueStore.string("user_watch_mode")` 已存在）· 直接跳过到下一未完成 milestone

**Given** 用户后期想切换 Watch 模式
**When** 进 Settings 页（Epic 7）
**Then** 提供"重新选择 Watch 模式"入口 · 重置 KeyValueStore + 跳转回本屏

**Given** 单元测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `WatchModePickerTests.testBothCtasRender`
- `WatchModePickerTests.testWithWatchOpensAppStoreLink`
- `WatchModePickerTests.testObserverModeWritesFlag`
- `WatchModePickerTests.testCopyDoesNotImplyDegradation`（grep 文案 · 禁"降级 / 仅 / 只能"等贬义词）

**FRs**：FR2（iPhone 端引导部分 · Watch 真机配对归 Epic 9）+ FR3

**🟢 Epic 3 启动条件达成**：本 Story 完成后 Epic 3（我的猫 & 表情）启动门槛满足（用户已 SIWA + 已选 Watch 模式 → 可创建"我的猫"实例）· Story 2.4-2.5 可与 Epic 3 并行

---

### Story 2.4: 撕包装首开仪式（TearCeremonyView · CatGazeAnimation · FR7）

As a **User who has chosen Watch mode**,
I want **看到一个"包装盒震动 → 撕开 → 猫跳出"的 1.5s 仪式**,
So that **从功能性 onboarding 转入"我的猫存在"情感锚点 · 实现 PRD 描述的"仪式感" · J5 孤独首开陪伴单机兜底**.

**Acceptance Criteria:**

**Given** Story 2.3 完成 · `KeyValueStore.string("user_watch_mode")` 非 nil
**When** 进入 Story 2.4 屏
**Then** 渲染 `TearCeremonyView`（CatPhone/Features/Onboarding/）· 初始状态：屏幕中央显示包装盒静态图

**Given** 用户在屏（首次进入）
**When** 0.5s 延迟后
**Then** 包装盒触发**震动动画**（轻 haptic via `HapticSpy` 测试时 record · 真机轻 `UIImpactFeedbackGenerator(style: .light)`）+ 视觉抖动 0.5s
**And** 之后**撕开动画**（Lottie · `cat_tear_ceremony.json` 资产 · Story 1A.3 路径）触发 1.5s 完成（DesignTokens 仪式动画 token）

**Given** 撕开动画完成
**When** 1.5s 后
**Then** `CatGazeAnimation` 触发（Lottie `cat_gaze_onboarding.json` 2.5s · UX v0.3 C2）· 猫从盒子里跳出 · 与镜头对视 2.5s
**And** 完成后调 `FirstTimeExperienceTracker.markSeen(.firstOpenCeremonyCompleted)` 写本地 dirty flag · 触发软 gate sync 流程（Story 2.5 详述）

**Given** 仪式播放中
**When** 用户**点击屏幕**
**Then** 不允许 skip（NFR24 例外 · 撕包装是 Tap-into-product 关键仪式 · 1.5+2.5=4s 已极短）· 视觉反馈"轻晃动"提示用户耐心等待

**Given** Reduce Motion accessibility 开启
**When** 仪式触发
**Then** 跳过震动 + 撕开动画 + CatGazeAnimation · 直接跳到下一 Story（命名）· 显示静帧"猫已跳出"图（UX-DR14）

**Given** 仪式动画 Lottie 资产缺失（异常情况）
**When** Lottie 加载失败
**Then** **静默降级**到帧序列 PNG 回退 · `Log.ui.error("lottie_tear_ceremony_missing")` + `Analytics.track("tear_ceremony_lottie_failed")`（observable signal · ADR-001 fail-open · 仪式不阻断 Onboarding）

**Given** 已完成首开仪式 · 用户 sign out + 重新登录
**When** 再次走 Onboarding
**Then** 跳过本仪式（FirstTimeExperienceTracker 防重）· 直接到 Story 2.5 命名（如已命名也跳过）

**Given** 单元 / UI 测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `TearCeremonyViewTests.testCeremonySequence`（断言震动 → 撕开 → CatGaze 顺序）
- `TearCeremonyViewTests.testHapticTriggered`（HapticSpy 验证）
- `TearCeremonyViewTests.testReduceMotionSkipsCeremony`
- `TearCeremonyViewTests.testLottieMissingFallsBackToPNG`
- `TearCeremonyViewTests.testNoSkipDuringCeremony`

**FRs**：FR7 → C-UX-02

---

### Story 2.5: 猫命名 + S-SRV-15 软 gate 降级方案落地（FR7 cont · FR10a）

As a **User who has just seen my cat jump out of the box**,
I want **给我的猫起一个名字 · 命名后完成 Onboarding 进入主功能**,
So that **完成"我拥有一只猫"的最终情感投资 · 名字成为后续社交 / 房间叙事文案的基础**.

**Acceptance Criteria:**

**Given** Story 2.4 撕包装仪式完成 · `FirstTimeExperienceTracker.hasSeen(.firstOpenCeremonyCompleted) == true`
**When** 屏幕渲染
**Then** 显示 `CatNamingView` · 中央猫静态图（Story 2.4 跳出后的姿态）· 下方 `TextField` 输入猫名（占位"给它起个名字"）· "完成"按钮

**Given** 用户输入猫名
**When** 字符数验证
**Then** 1-12 字符 · 禁纯空格 · 禁 emoji 主导（允许 1 个表情 + 文字）· 实时 disable "完成" 按钮如不符合
**And** 文字超长用 `.allowsTightening(true) + .minimumScaleFactor(0.9)`（UX-DR12）

**Given** 用户点 "完成"
**When** 提交
**Then** POST `/v1/cat/name` `{ catName }` · 成功返回 `{ catId, catName }`
**And** 客户端**本地写**：
- `KeyValueStore.set("cat_name_local", value: catName)`（dirty flag · 标"尚未同步至 milestones"）
- `KeyValueStore.set("onboarding_completed_local_at", value: nowTimestamp)`（dirty flag · TTL 30 天起算）
**And** **触发 S-SRV-15 软 gate sync 流程**（详见 AC 下方）

**Given** S-SRV-15 软 gate 降级方案（Party Mode 修订 · AR7.15 + Epic 2 修订 + Story 1C.1 1A 紧密协作）
**When** App `foreground` 事件触发（包括本 Story 完成时）
**Then** 调 `MilestonesSyncService.attemptSync()`（Epic 7 实装 · 本 Story 内只调用 protocol 接口占位）：
1. 检查所有本地 dirty flag（`onboarding_completed_local_at` 等）
2. 若 server `user_milestones` API（S-SRV-15）已 ready · POST `/v1/users/me/milestones` 同步 · 成功后清 dirty flag · 写 `onboarding_completed_synced: true`
3. 若 server S-SRV-15 未 ready 或 API 调用失败 · 保留 dirty flag · `Log.network.warn("milestone_sync_pending")` + `Analytics.track("milestone_sync_pending")`

**Given** 本地 dirty flag TTL 30 天
**When** 检查 `onboarding_completed_local_at` + 当前时间差 > 30 天 + 仍未 sync
**Then** 视为未完成 onboarding · 重新触发 Onboarding 流程（防长期腐烂 · John 降级方案的硬约束）
**And** 显示用户提示 `GentleFailView(reason: .onboardingResyncRequired, retry: ...)` 文案"我们让你重新打个招呼"（不暗示 server 故障）· `Log.ws.warn("milestone_ttl_expired_resync")`

**Given** 服务器最终 ready 并接收 sync · 用户在 30 天内
**When** sync 成功
**Then** server 端写入 `user_milestones.onboarding_completed = true`（FR10a 权威源）· 清本地 dirty flag · UI 不打扰用户

**Given** 用户完成命名 + 进入主屏
**When** 主 TabBar 渲染
**Then** 显示 5 个 tab（Account / Friends / Inventory / Dressup / Room · 实际 UI 在各 Epic Story · 本 Story 只导航到 Tab Container）
**And** Account tab 显示用户起的猫名（数据从 `KeyValueStore` 或 server 读 · UI 由 Epic 3 Story 实装）

**Given** 命名 API 失败（网络 / 5xx）
**When** 错误抛出
**Then** 显示 `GentleFailView(reason: .catNamingFailed, retry: ...)` 文案"先记下了" + 按钮"再试一次"
**And** **本地仍写** `cat_name_local` dirty flag（**不丢用户输入** · 重试时预填）· `Log.network.error("cat_naming_failed")` + `Analytics.track("cat_naming_failed")`

**Given** 单元 / UI 测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `CatNamingViewTests.testValidationConstraints`（1-12 字符 / 禁纯空格 / 1 emoji 上限）
- `CatNamingViewTests.testSuccessWritesDirtyFlagAndTriggersSync`
- `MilestonesSyncServiceTests.testSyncSuccessClearsDirtyFlag`
- `MilestonesSyncServiceTests.testSyncFailureKeepsDirtyFlag`
- `MilestonesSyncServiceTests.test30DayTTLExpiredTriggersResync`
- `CatNamingViewTests.testFailurePreservesUserInput`

**Given** S-SRV-15 跨仓契约
**When** 任何调 `/v1/users/me/milestones` API 的 client 代码
**Then** 必须遵守 server-handoff-ux-step10-2026-04-20.md 定义的 API contract（payload 字段名 / TTL 约定 / 冲突处理）· 偏离须登记到 `docs/contract-drift-register.md`

**FRs**：FR7（命名部分）+ FR10a（user_milestones server 权威 + 软 gate 降级）

---

## Epic 3: 我的猫 & 表情交互（MyCat & Emote）

**Epic Goal**：User 每次打开 app 都能和自己的猫有 reciprocal — 点猫 → emoji 浮起 + 猫微回应 + haptic。即使断网 / 不在房间，本地交互闭环（fire-and-forget 对称性 · 无"已读已送达"UI）。

**Story 编号**：3.1-3.4 共 4 Story · 启动条件：Epic 1 全 + Epic 2 Story 2.3 完成（用户已 SIWA + 已选 Watch 模式）

### Story 3.1: AccountTabView + MyCatCard（静态 + 呼吸 + 抬眼 + 卡片 button）

As a **User who has just completed Onboarding**,
I want **打开 app 即看到我的猫卡片（静态 avatar + 4s 呼吸循环 + 打开 app 时 0.6s 抬眼）**,
So that **每次开 app 第一眼是"它在家等我"的实在感 · 不是空荡荡的 tab 列表**.

**Acceptance Criteria:**

**Given** Epic 2 已完成 · 用户在主 TabBar
**When** 默认进入 Account tab
**Then** 渲染 `AccountTabView`（CatPhone/Features/Account/）· 顶部猫名（Story 2.5 已存）· 中央 `MyCatCard`（CatShared/UI/）· 底部 `HintLabel("轻点跟它打招呼")`（Story 1C.5 组件）· 设置入口图标（右上角 · 进 Epic 7 Settings）

**Given** `MyCatCard` 渲染（DesignTokens · 卡片圆角 16 + 极轻阴影）
**When** 卡片可见
**Then** 显示猫静态 avatar 图（默认皮肤 · 由 server 装扮状态决定 · 默认皮肤资产在 CatShared/Resources/）
**And** **呼吸动画**触发：4s 循环 · 透明度 ±5% 或缩放 ±2% · `breathingEnabled: true` 默认 · DesignTokens 呼吸 token

**Given** App 从 background → foreground（或首次启动）
**When** `AccountTabView.onAppear` 触发
**Then** **抬眼动画**触发一次：0.6s 单次 · Lottie `cat_gaze_lookup.json` 或 SwiftUI `PhaseAnimator`（依 Story 1A.5 Spike verdict 决定栈）· 完成后回归静态 + 呼吸

**Given** Reduce Motion accessibility 开启
**When** AccountTabView 渲染
**Then** 呼吸 → 静止 + 透明度 ±5% 循环（保最小生命感）· 抬眼 → **跳过**（UX-DR14）

**Given** 用户首次进入 Account tab（`FirstTimeExperienceTracker.hasSeen(.accountTabFirstSeen) == false`）
**When** MyCatCard 完成抬眼
**Then** 触发 **PulseEffect**（一次性脉冲提示卡片可点 · UX-DR9 组件 · 0.8s · 之后永不再现）
**And** 调 `markSeen(.accountTabFirstSeen)`

**Given** `MyCatCard` 是 button（`onTap` callback）
**When** 用户点击
**Then** 触发 Story 3.2 EmojiPicker（本 Story 只 emit `onTap` event · 接收方在 Story 3.2 实装）
**And** 立即触发 light haptic（`UIImpactFeedbackGenerator(style: .light)` · HapticSpy 可断言）

**Given** VoiceOver 开启
**When** focus 到 MyCatCard
**Then** 读出 `accessibilityLabel = "[猫名]，轻点跟它打招呼"`（UX-DR13 · 可测试）

**Given** Dynamic Type `.accessibilityXXXLarge`
**When** 渲染
**Then** 卡片高度自适应（`.frame(minHeight:)`）· 文字不截断（UX-DR12）

**Given** 单元 / UI 测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `AccountTabViewTests.testInitialRender`
- `MyCatCardTests.testBreathingEnabledByDefault`
- `MyCatCardTests.testGazeLookupTriggersOnAppear`
- `MyCatCardTests.testReduceMotionDisablesGaze`
- `MyCatCardTests.testFirstTimePulseOnlyOnce`
- `MyCatCardTests.testTapTriggersOnTapCallbackAndHaptic`
- `MyCatCardTests.testAccessibilityLabelIncludesCatName`

**FRs**：（本 Story 无直接 FR · 为 FR26 铺垫 · 用 UX-DR4 组件）

---

### Story 3.2: EmojiPicker（Inline + Sheet 自适应 · 设备 + Dynamic Type 双轴）

As a **User looking at my cat card**,
I want **点猫卡片弹出 3 个表情选项（❤️ 比心 / ☀️ 晒太阳 / 😴 困意 · MVP）· 小屏 / 大字号自动切 Sheet**,
So that **零思考交互 · 200ms 内响应 · 不被设备尺寸 / 字号失控**.

**Acceptance Criteria:**

**Given** Story 3.1 MyCatCard 渲染
**When** 用户点卡片 + 触发 `onTap`
**Then** **设备 + 字号双轴自适应判定**：
- iPhone 屏高 ≥ 700pt + Dynamic Type ≤ `.xxxLarge` → 渲染 `EmojiPickerInline`（卡片下方 2×2 网格内联展开 · UX-DR4）
- 否则（iPhone SE 3rd 667pt / DT ≥ `.accessibilityMedium`）→ 渲染 `EmojiPickerSheet`（底部 30% Sheet 弹出 · UX-DR4）
- iPhone Pro Max 932pt + 卡片 Y > 60% → 强切 Sheet（单手热区 · UX-DR16）

**Given** `EmojiPickerInline` 渲染
**When** 200ms 内
**Then** 完成展开（NFR1 · "点猫弹表情选单 < 200ms" 硬指标）· DesignTokens 默认 0.35s easeInOut 用 0.2s 加速版

**Given** `EmojiPickerInline` 在 DT `.xLarge - .xxxLarge`
**When** 渲染
**Then** 2×2 网格自动改 2 行垂直堆叠 · 保 54pt tap target（UX-DR12）

**Given** 用户点选某 emoji
**When** 触发 `onPick(emoji)` callback
**Then** picker 关闭（Inline 收回 / Sheet dismiss）· 触发 Story 3.3 emoji 浮起 + 微回应 + server 去重 fan-out
**And** 写 `KeyValueStore.set("emoji_send_count_v1", value: getCount + 1)`（埋点 + Story 3.3 FirstEmojiTooltip 触发条件）

**Given** 用户点 picker 外区域 / 下滑（Sheet）
**When** dismiss 动作
**Then** picker 关闭无表情发送 · 不写 emoji_send_count

**Given** Reduce Motion 开启
**When** picker 弹出
**Then** 弹出动画降级为瞬切（无 0.2s 缓动）

**Given** VoiceOver 开启
**When** focus 到 picker
**Then** 读 `accessibilityLabel = "表情选择，3 个选项"`· 每个 emoji 读 `比心` / `晒太阳` / `困意`（UX-DR13）

**Given** 单元 / UI 测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `EmojiPickerAdaptiveTests.testInlineOnLargeScreen`
- `EmojiPickerAdaptiveTests.testSheetOnSmallScreen`
- `EmojiPickerAdaptiveTests.testSheetOnAccessibilityFontSize`
- `EmojiPickerAdaptiveTests.testProMaxBottomCardForcesSheet`
- `EmojiPickerInlineTests.testRenderUnder200ms`（性能断言 NFR1）
- `EmojiPickerInlineTests.testDTReflowToTwoRowStack`
- `EmojiPickerSheetTests.testDismissDoesNotEmitEmoji`
- `EmojiPickerInlineTests.testEmojiSendCountIncrements`

**FRs**：FR26（点猫弹表情选单）+ NFR1（< 200ms）

---

### Story 3.3: 自发表情 emoji 浮起 + 猫微回应 + FirstEmojiTooltip（本地呈现）

As a **User who has just picked an emoji**,
I want **看到 emoji 从猫旁浮起飘走 + 猫微微反应（3 种随机派发）+ 首次成功发送后看到 tooltip "它摆了摆尾巴"**,
So that **fire-and-forget 立即有本地反馈闭环 · 即使没在房间也不孤独 · 首次有情感 payoff**.

**Acceptance Criteria:**

**Given** Story 3.2 用户选了某 emoji（callback `onPick(emoji)` 触发）
**When** 200ms 内
**Then** **EmojiEmitView** 触发（CatShared/UI/ · UX-DR2 组件 · Story 1C.5 已立组件骨架 · 本 Story 实装动画细节）：
- 0.25s 浮起（猫旁起点 · DesignTokens 动画 token）
- 2s 缓飘 + ±2pt 摆动
- 0.35s 淡出
- 总时长 ~2.6s

**Given** EmojiEmitView 触发的同时
**When** 0ms 延迟
**Then** **CatMicroReactionView** 触发（UX-DR4 · CatPhone/Features/Account/ · 本 Story 实装）：
- 3 种随机派发：`tailFlick`（0.3s 尾巴摆）/ `earTwitch`（0.4s 耳朵动）/ `slowBlink`（0.5s 慢眨眼）
- 每次随机 1/3 概率（不连续重复）
- Lottie 或 SwiftUI 原生 PhaseAnimator（依 Story 1A.5 Spike verdict）
- HapticSpy 可断言伴随的 light haptic

**Given** Reduce Motion 开启
**When** 微回应触发
**Then** 简化为单帧姿态切换（0.1s · UX-DR14）

**Given** 首次成功发送 emoji（`emoji_send_count_v1 == 1`）
**When** 微回应完成 + 200ms 延迟
**Then** **FirstEmojiTooltip** 弹出（UX-DR4）：
- 文案"它摆了摆尾巴"
- 2s 自动淡出（UX Pattern 4 Tooltip）
- 之后**永不再出现**（`FirstTimeExperienceTracker.markSeen(.firstEmoteSent)`）
- 调 `MilestonesSyncService.attemptSync()` 触发 S-SRV-15 软 gate sync 流程

**Given** Tooltip 已显示过（`hasSeen(.firstEmoteSent) == true`）
**When** 后续点 emoji
**Then** 不再显示 tooltip · 只走 emoji 浮起 + 微回应

**Given** Reduce Motion + 首次 emoji
**When** 微回应跳过
**Then** Tooltip 仍按时弹出（信息层不省略 · 仅动画降级）

**Given** 单元 / UI 测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `EmojiEmitViewTests.testThreePhasesAnimation`（0.25s 浮起 + 2s 缓飘 + 0.35s 淡出）
- `CatMicroReactionViewTests.testThreeReactionsRandomDispatch`
- `CatMicroReactionViewTests.testNoConsecutiveDuplicates`
- `CatMicroReactionViewTests.testHapticTriggered`
- `FirstEmojiTooltipTests.testShowsOnFirstEmoteOnly`
- `FirstEmojiTooltipTests.testAutoDismissAfter2Seconds`
- `FirstEmojiTooltipTests.testMarksFirstTimeKey`
- `FirstEmojiTooltipTests.testReduceMotionStillShowsTooltip`

**FRs**：FR27（emoji 浮起 + 本地呈现）

---

### Story 3.4: Fire-and-forget 对称性 + server 去重落日志（FR29 + FR30 + S-SRV-2/11/17）

As a **User sending emojis when not in a room (or in a room)**,
I want **emoji 走 server 去重落日志（如果在房间则 fan-out · 不在房间则纯本地 + server 去重）· UI 永不显示"已读 / 已送达 / 发送中"**,
So that **fire-and-forget 对称性硬约束 · 防触觉广播感受被破坏（创新假设 #2）**.

**Acceptance Criteria:**

**Given** Story 3.3 emoji 浮起 + 微回应触发的同时
**When** 0ms 延迟（fire-and-forget · 不等任何 ack）
**Then** 客户端调 `EmoteService.send(emoji: catTap, roomId: currentRoomId?)`：
- `EmoteService` protocol 在 CatCore/Emote/ · 本 Story 实装
- 内部调 `WSClient.send(type: "cat.tap", payload: { emoji, room_id?, client_msg_id }, awaitResponse: false)`（FR54 client_msg_id · 300s server 去重 · awaitResponse=false 关键 · 由 Story 1B.3 baseline 提供）
- **绝不等 ack** · 调用立即返回

**Given** 用户**不在房间**（`currentRoomId == nil`）
**When** WS 发送
**Then** server 收到 `cat.tap` 仅做去重落日志（S-SRV-11 schema · 不 fan-out）· 客户端无任何额外 UI 表现（已在 Story 3.3 完成本地呈现 FR29）
**And** `Log.ws.info("emote_sent_no_room", emoji: ..., roomId: nil)`

**Given** 用户**在房间**（`currentRoomId != nil`）
**When** WS 发送
**Then** server 走 `cat.tap` → `pet.emote.broadcast` fan-out 给房间其他成员（S-SRV-2 + S-SRV-11 schema）· **server 不向发送方发任何 ack 或 delivered 推送**（S-SRV-17 硬约束 · server-handoff-ux-step10-2026-04-20.md 定义）

**Given** WS 发送失败（断线 / 5xx / 超时）
**When** WSClient 抛错
**Then** **客户端 UI 无任何错误显示**（fire-and-forget · 用户已在 Story 3.3 看到本地浮起 + 猫回应 · 不破坏沉浸）
**And** `Log.ws.warn("emote_send_failed_silent", emoji: ..., reason: ...)` + `Analytics.track("emote_send_failed_silent")`（observable signal · ADR-001 §Decision Matrix · fail-open 静默降级 · UX Pattern 3）

**Given** UI 设计禁用清单（fire-and-forget 对称性硬约束 · UX Pattern 9）
**When** grep 本 Story + Story 3.1-3.3 所有 Swift 源文件
**Then** **零命中**以下字符串（grep gate · 可在 PR 自动检查）：
- "已读" · "已送达" · "对方已收到" · "正在发送" · "发送中" · "发送失败" · "重试发送" · "delivered" · "read by" · "sending..."

**Given** 服务端 envelope schema（S-SRV-11 cat.tap）
**When** 客户端发送
**Then** payload 字段严格遵 `docs/api/ws-message-registry.md` 定义 · 字段命名漂移登记 `docs/contract-drift-register.md`

**Given** 客户端收到来自 server 的 `emote.delivered`（违反 S-SRV-17）
**When** WSClient `messages` AsyncStream yield
**Then** **客户端忽略不渲染**（防 server 误推破坏对称性）· `Log.ws.error("unexpected_emote_delivered_from_server")` + `Analytics.track("server_violated_fire_and_forget")`（高严重度 · 通知 server 团队）

**Given** 单元 / 集成测试
**When** 运行 `xcodebuild test`（用 WSFakeServerV1 · Story 1C.2）
**Then** 通过：
- `EmoteServiceTests.testSendDoesNotAwaitResponse`
- `EmoteServiceTests.testSendIncludesClientMsgId`
- `EmoteServiceTests.testNoRoomIdSendsAnyway`
- `EmoteServiceTests.testFailureDoesNotShowUIError`
- `EmoteServiceTests.testFailureLogsObservableSignal`
- `FireAndForgetGrepTests.testForbiddenStringsAbsent`（grep 断言 9 禁用词）
- `EmoteServiceTests.testIgnoresUnexpectedDeliveredFromServer`

**FRs**：FR29（不在房间本地呈现 + server 去重落日志）+ FR30（fire-and-forget 无 ack UI）+ FR54（client_msg_id 复用 baseline）

---

## Epic 4: HealthKit × 盲盒 × 仓库（经济闭环 Part 1）

**Epic Goal**：走路 → 看见步数进展 → 挂机 30min 掉盒子 → 1000 步开盒 → 皮肤入仓库 / 重复转材料。北极星反馈回路（"走 1000 步接猫回家"）一次跑通。fail-closed 严格遵 ADR-001 · server 权威解锁防作弊（FR32 + AR8.3）。

**Story 编号**：4.1-4.9 共 9 Story · 启动条件：Epic 1 全 + Epic 2 Story 2.3 完成

### Story 4.1: HealthKit 授权流 + 拒绝降级（FR4 + FR5 + FR50）

As a **User newly into the main app**,
I want **首次进 Inventory tab 或步数相关 UI 时被请求 HealthKit 授权 · 拒绝后 UI 优雅降级（步数显示"—" + 引导条"去设置开启权限"）**,
So that **步数功能可选 · 不被授权弹窗强制 · 拒绝路径明确不假死**.

**Acceptance Criteria:**

**Given** 用户首次进入需要步数的页面（Inventory tab / BoxPendingCard 出现）· `HealthKitAuthorization.status == .notDetermined`
**When** 触发授权请求
**Then** 调 `HKHealthStore().requestAuthorization(toShare: [], read: [HKQuantityType(.stepCount)])` · 系统弹 `NSHealthShareUsageDescription`（Info.plist 已含 · 文案"步数用于'走 1000 步接猫回家'机制 · 不会上传到服务器"）

**Given** `CatShared/Sources/CatCore/Health/HealthKitAuthorization.swift` 实装
**When** 检查 `@Observable @MainActor final class HealthKitAuthorization`
**Then** 暴露：`var status: HKAuthorizationStatus`（系统状态）· `func request() async throws`（触发系统弹窗）· `var didChange: AsyncStream<HKAuthorizationStatus>`（状态变化推送）
**And** 根 `@Environment` 注入（per Architecture D1 · CatCore 全局 Store）

**Given** 用户允许授权
**When** 系统回调
**Then** `status = .sharingAuthorized` · 后续 Story 4.2 StepDataSource 可读步数

**Given** 用户拒绝授权
**When** 系统回调
**Then** `status = .sharingDenied` · UI **不再重复弹**（系统已记忆 · 二次 request 无效）
**And** 步数 UI 走 FR50 降级路径：
- BoxPendingCard 进度数字显示"—" 而非 "0"
- 仓库 tab 顶部显示 `GentleFailView(reason: .healthKitDenied)` 文案"我们只看步数，别的不看" + 按钮"去开启"
- 按钮跳转 `UIApplication.shared.open(URL(string: UIApplication.openSettingsURLString))`（FR50 引导条）

**Given** 用户从设置回来允许授权后
**When** App foreground
**Then** `HealthKitAuthorization.refresh()` 重检状态 · 若变 `.sharingAuthorized` 触发 `didChange` AsyncStream · UI 自动恢复（订阅者 reactive 更新）

**Given** Epic 7 Settings 入口（Story 7.x）
**When** 用户主动想开 HealthKit
**Then** Settings 提供 "HealthKit 权限" 行 · 显示当前状态 + "去开启" 按钮（共用本 Story 跳设置逻辑）

**Given** fail-closed observable signal
**When** 拒绝授权 · UI 显示 GentleFailView
**Then** `Log.health.warn("healthkit_denied")` + `Analytics.track("healthkit_denied")`（ADR-001 · AR10.2 grep gate 验证）

**Given** 单元 / UI 测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `HealthKitAuthorizationTests.testRequestUpdatesStatus`
- `HealthKitAuthorizationTests.testDeniedDoesNotRetry`
- `HealthKitAuthorizationTests.testSettingsLinkOpensCorrectURL`
- `HealthKitAuthorizationTests.testForegroundRefreshDetectsChange`
- `HealthKitDenialUITests.testGentleFailViewShownOnDenial`
- `HealthKitDenialUITests.testStepsShowDashOnDenial`

**FRs**：FR4（授权请求）+ FR5（拒绝降级）+ FR50（UI 回退 + 引导条）

---

### Story 4.2: `StepDataSource` 实装 + 步数本地展示（D4 + FR31 + NFR4 cache）

As a **User who granted HealthKit permission**,
I want **App 内看到今日步数 · 数据从 HealthKit 本地读 · 15 min 缓存避免耗电**,
So that **步数实时但不烧 CPU/电池 · 遵 NFR29 禁高频 HKSampleQuery**.

**Acceptance Criteria:**

**Given** Story 1B 时期 BoxStateStore protocol 已存在 · 本 Story 实装 StepDataSource 真 impl（替换 Story 1C 占位）
**When** 检查 `CatShared/Sources/CatCore/Health/StepDataSource.swift`
**Then** 协议定义已在 AR5.4：
```swift
protocol StepDataSource: Sendable {
    func todayStepCount() async throws -> Int
    func stepHistory(range: DateInterval) async throws -> [DailyStepCount]
    var incrementalUpdates: AsyncStream<Int> { get }
}
```
**And** `actor HealthKitStepDataSource: StepDataSource` 实装：
- 内持 `HKStatisticsCollectionQuery`（按天聚合 stepCount · NFR29）
- 15min cache（actor 内部 dict · `lastFetchTimestamp + cachedValue` · NFR4）
- `incrementalUpdates` 由 `HKObserverQuery` + `HKAnchoredObjectQuery` 触发 continuation 推（NFR29 · 不轮询）

**Given** `HealthKitStepDataSource.todayStepCount()` 调用
**When** 距上次 fetch < 15 min
**Then** 返回 cached value · 不发 HKQuery（NFR4 · 节电）

**Given** 距上次 fetch ≥ 15 min
**When** 调用
**Then** 发 `HKStatisticsCollectionQuery` 拉今日聚合 · 更新 cache · 返回新值

**Given** AR6.3 (O3) Clock 注入 + cacheHitCount 计数器
**When** 检查 `HealthKitStepDataSource` 构造函数
**Then** `init(clock: any ClockProtocol = SystemClock(), cacheTTL: Duration = .seconds(900))` · 暴露 `cacheHitCount: Int` · 测试可断言"15min 边界 cache hit/miss"

**Given** HealthKit 权限 `.sharingAuthorized` + Story 4.1 已完成
**When** 用户进 Account/Inventory tab
**Then** UI 渲染 `StepCountView`（CatPhone/Features/Inventory/ 或 Account/）显示"今日步数 1234"
**And** 数字字号大 + 易读 · 用 DesignTokens 大标题字号

**Given** HKObserverQuery 检测到新步数（用户走路新增）
**When** continuation 推 incrementalUpdates AsyncStream
**Then** UI Store `for await` 接收 · 自动更新数字（无 polling · 推送式）
**And** 触发 BoxStateStore.refresh()（间接联动 Story 4.5 解锁判定）

**Given** EmptyStepDataSource（Epic 1 1B.7 占位）vs InMemoryStepDataSource（测试用 · Fixture）vs HealthKitStepDataSource（产线）
**When** Test 注入 `InMemoryStepDataSource(value: 500)`
**Then** UI 显示 500 · 修改 `inMemorySource.value = 1000` + advance clock 15min · UI 自动更新

**Given** 单元测试 + Mock Clock
**When** 运行 `xcodebuild test`
**Then** 通过：
- `HealthKitStepDataSourceTests.testCacheHitWithin15Min`
- `HealthKitStepDataSourceTests.testCacheMissAfter15Min`
- `HealthKitStepDataSourceTests.testCacheHitCountIncrement`
- `HealthKitStepDataSourceTests.testIncrementalUpdatesPushesContinuation`
- `StepCountViewTests.testRendersStepCount`
- `StepCountViewTests.testReactiveUpdate`

**FRs**：FR31（HealthKit 本地读取展示）+ NFR4（15min cache）+ NFR29（HK API 选型）

---

### Story 4.3: 步数历史图表（FR33 + S-SRV-9）

As a **User who wants to see my walking pattern over time**,
I want **iPhone 大屏步数历史图表 · 7/30/90 天切换 · 含 streak 提示**,
So that **iPhone "慢房间"分工的体验 payoff · 让用户感受到走路的累积价值**.

**Acceptance Criteria:**

**Given** Story 4.2 StepDataSource 实装 + 用户 HealthKit 已授权
**When** 用户在 Inventory tab 进 "步数历史" 入口（或 Account tab settings）
**Then** 渲染 `StepHistoryView`（CatPhone/Features/Inventory/）· 顶部 segmented control "7 天 / 30 天 / 90 天"

**Given** 用户切到 7 天
**When** 视图加载
**Then** 调 `StepDataSource.stepHistory(range: 7 days)` · 内部从 HealthKit 拉本地数据（NFR29 按天聚合）+ 调 `/steps/history?range=7d` API（S-SRV-9）确认与 server 一致
**And** UI 用 `Charts` framework（iOS 17+ · 已锁）渲染柱状图 · X 轴=日期 · Y 轴=步数 · 1000 步水平线（"接猫回家"参考线）

**Given** 本地步数与 server `/steps/history` 返回不一致（双 SSoT 漂移）
**When** 检测漂移
**Then** UI **以 server 为准展示**（FR32 server 权威）+ 图表底部小字"以服务端记录为准"
**And** `Log.health.warn("step_history_drift", localCount, serverCount, range)` + `Analytics.track("step_history_drift")`

**Given** 用户当前 streak 计算（连续天数 ≥ 1000 步）
**When** 视图加载完成
**Then** 顶部小字显示"连续走够 N 天" · N 来自 `/steps/history` API streak 字段（server 计算）· 0 时不显示

**Given** API `/steps/history` 调用失败
**When** 网络错误
**Then** 显示 `GentleFailView(reason: .stepsHistoryFailed, retry: ...)` 文案"步数历史暂时拿不到" · 不破坏当日步数显示（FR31 仍可用 · 仅历史图降级）
**And** `Log.network.error("steps_history_failed")` + `Analytics.track("steps_history_failed")`

**Given** Reduce Motion + Charts 默认有动画
**When** 渲染
**Then** Charts 动画走系统 `accessibilityReduceMotion` 自适应（Charts framework 原生支持）

**Given** VoiceOver
**When** focus 到柱状图
**Then** 读出 `[日期]: [步数] 步`（per bar · UX-DR13）

**Given** Dynamic Type `.accessibilityXXXLarge`
**When** 渲染
**Then** Y 轴标签 / 日期标签自适应缩放 · 不堆叠

**Given** 单元测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `StepHistoryViewTests.testRangeSegmentSwitches`
- `StepHistoryViewTests.testDriftShowsServerAuthority`
- `StepHistoryViewTests.testStreakRendersFromAPI`
- `StepHistoryViewTests.testFailureShowsGentleFailView`
- `StepHistoryViewTests.testReferenceLineAt1000`

**FRs**：FR33（步数历史图表）+ S-SRV-9（/steps/history API）

---

### Story 4.4: BoxStateStore 实装 + 盲盒 pending UI（FR35a · 30min 挂机 · ClockProtocol）

As a **User who has been online idle for 30 min**,
I want **server 判定后掉落一个盲盒 · iPhone 仓库显示 BoxPendingCard 包含进度（"已走 0 / 1000 步"）**,
So that **挂机有反馈 · 走路有目标 · 北极星循环开始**.

**Acceptance Criteria:**

**Given** Story 1B.7 BoxStateStore protocol 已存在 · 本 Story 实装 RealBoxStateStore 替换 Empty
**When** 检查 `CatCore/Box/RealBoxStateStore.swift`
**Then** `actor RealBoxStateStore: BoxStateStore` 实装：
- 持 `WSClient + APIClient + ClockProtocol`
- 启动时调 `GET /v1/boxes/pending`（S-SRV-8）拉当前 pending 盒子列表
- 订阅 WS `box.drop` event（S-SRV-8）· server 推 → state yield `.pending(progress: 0, total: 1000, droppedAt: Date)`
- 订阅 WS `box.progress.update` event · server 推进度更新 → state yield `.pending(progress: N, total: 1000, ...)`

**Given** server 判定挂机满 30min（FR35a · server 权威 · iPhone 不本地判定）
**When** server 推 `box.drop` WS event
**Then** RealBoxStateStore 收到 → state AsyncStream yield `.pending(...)`· UI 自动渲染 `BoxPendingCard`

**Given** Inventory tab 渲染
**When** 当前 boxState 为 `.pending(progress, total, droppedAt)`
**Then** 显示 `BoxPendingCard`（UX-DR6 · CatPhone/Features/Inventory/）：
- 包装盒图（暗示未开）
- 进度文字"已走 \(progress) / \(total) 步"（UX Pattern 10 · 数字优先）
- 进度条（线性 · 不打 spinner）
- 文案不剧透皮肤（Story 4.7 颜色分级仅在开盒后揭晓）

**Given** ClockProtocol 注入（per AR5.4 + 测试用）
**When** Test 注入 `MockClock` + 调 `mockClock.advance(by: .minutes(30))`
**Then** WSFakeServerV1 按 fixture 推 `box.drop` · UI 渲染 BoxPendingCard · 测试可 fast-forward 不等真 30 min（[U] 标签 + AC 三元组 timeout 角度）

**Given** 多个 pending 盒子同时存在（罕见 · 例如用户长时间未上线累积）
**When** state yield 多个 `.pending`
**Then** UI 渲染**多张** BoxPendingCard 列表（按 droppedAt 时间倒序）· 用户可同时走步数解锁多个盒子（FR35b 在 Story 4.5 处理多盒解锁顺序）

**Given** iPhone 启动后无 pending 盒子
**When** state yield `.empty`
**Then** UI 显示 `EmptyStoryPrompt(icon: archivebox, title: "仓库还是空的", subtitle: "等第一个盒子到家")`（UX Pattern 1 · UX-DR10 · 无 CTA）

**Given** Reduce Motion
**When** BoxPendingCard 进度条更新
**Then** 进度条变化无补间动画 · 直接跳到新值

**Given** VoiceOver
**When** focus 到 BoxPendingCard
**Then** 读 `accessibilityLabel = "待解锁盲盒，已走 [progress] / 1000 步"`（UX-DR13）

**Given** 单元测试 + WSFakeServerV1 + MockClock
**When** 运行 `xcodebuild test`
**Then** 通过：
- `RealBoxStateStoreTests.testInitialFetchPendingBoxes`
- `RealBoxStateStoreTests.testWSBoxDropYieldsPending`
- `RealBoxStateStoreTests.testWSProgressUpdateYieldsNewProgress`
- `BoxPendingCardTests.testRendersProgressNumerically`
- `BoxPendingCardTests.testNoSpoilerAboutSkin`
- `BoxPendingCardUITests.testReduceMotionNoProgressAnimation`
- `BoxPendingCardTests.testVoiceOverLabel`
- `RealBoxStateStoreTests.testMockClockFastForwardTriggersBoxDrop`

**FRs**：FR35a（30min 挂机掉盒 · server 权威 + ClockProtocol）+ FR51（仓库空状态）

---

### Story 4.5: 1000 步开盒 + server 权威解锁 + 等待确认占位（FR32 + FR35b + FR34 + FR58）

As a **User who has a pending box and is walking**,
I want **走够 1000 步后 server 解锁盲盒 · WS 断线时 UI 显示"已达成·等待确认"占位 · 不假装解锁**,
So that **server 权威防作弊（创新假设）· 网络抖动时不误导用户**.

**Acceptance Criteria:**

**Given** Story 4.4 BoxPendingCard 渲染 · `progress < 1000`
**When** 用户走路 + Story 4.2 StepDataSource 增量推送
**Then** RealBoxStateStore 用本地步数**仅做 UI 进度展示** · **绝不本地判定解锁**（FR32 防作弊 · 遵 AR8.4 §21.4 语义正确性）
**And** 客户端**不上传 raw step count**（FR58 · 仅 server 通过 HKObserver 已收到的 ack 流推算）

**Given** server 端步数 ≥ 1000（server 权威判定 · 由 Watch 端或 iPhone 端的 `/steps/sync` ack 通道汇总 · 跨仓 contract）
**When** server 推 WS `box.unlock.request` event（询问客户端确认解锁动作 · 或自动解锁视产品逻辑）
**Then** RealBoxStateStore state yield `.unlocking` · UI BoxPendingCard 切换为"unlocking 中..."占位 + Skeleton（UX Pattern 5）

**Given** server 解锁成功（推 `box.unlock.complete` 含 skin payload · S-SRV-8）
**When** RealBoxStateStore 收到
**Then** state yield `.unlocked(skin: Skin)` · UI 切换：
- iPhone 仅做**简洁视觉提示**（无开箱仪式 · iPhone 哲学不抢 Watch 剧场 · 仅显示卡片"接到家了 [skin name]"轻动画）
- Watch 端走 FR35c 开箱动画（Epic 9 真机验证）

**Given** WS 断线（Story 1B.3 baseline 重连中） · 客户端本地步数 ≥ 1000 但 server 状态未确认
**When** UI 渲染
**Then** BoxPendingCard 切换为 `BoxAwaitingAckCard` 显示"已达成·等待确认"占位（FR34）· 进度条满 · 不显示"已解锁"
**And** WSClient 重连后 → 调 `RealBoxStateStore.refresh()` → 拉最新 state · UI 自动恢复

**Given** S-SRV-14 server 解锁服务降级（Redis 挂 · 读 Mongo）
**When** server 返回降级响应
**Then** UI 显示 `GentleFailView(reason: .boxUnlockTemporarilyUnavailable, retry: ...)` 文案"盲盒暂时无法打开，稍后重试"（UX Pattern 2）
**And** `Log.ws.error("box_unlock_redis_degraded")` + `Analytics.track("box_unlock_redis_degraded")`（fail-closed observable · ADR-001）
**And** 用户重试时再走 server 路径 · 不本地伪装解锁

**Given** 单元 / 集成测试 + DualSSoTScenarioFixture（Story 1C.3）
**When** 运行 `xcodebuild test`
**Then** 通过：
- `BoxUnlockTests.testLocalStepsAtThresholdDoesNotUnlockLocally`（FR32 防作弊核心）
- `BoxUnlockTests.testServerUnlockRequestTransitionsToUnlocking`
- `BoxUnlockTests.testServerUnlockCompleteYieldsUnlocked`
- `BoxUnlockTests.testWSDisconnectShowsAwaitingAckCard`（FR34）
- `BoxUnlockTests.testReconnectRefreshesState`
- `BoxUnlockTests.testRedisDegradedShowsGentleFailView`（S-SRV-14）
- `BoxUnlockTests.testNoRawStepCountUploaded`（FR58 · grep 网络层 payload）
- `DualSSoTScenarioTests.testLocalAheadServerBehindShowsAwaitingAck`

**FRs**：FR32（server 权威）+ FR34（WS 断线占位）+ FR35b（1000 步开盒）+ FR58（不上传 raw step）+ S-SRV-14（解锁降级）

---

### Story 4.6: `unlocked_pending_reveal` 中间态 UI（FR35d + S-SRV-16）

As a **User whose box was unlocked but Watch was unreachable**,
I want **iPhone 仓库显示 BoxUnlockedPendingRevealCard "已解锁·待揭晓（等待 Watch 上线）"占位 · Watch 重连后 server 推 box.unlock.revealed 触发开箱动画**,
So that **盲盒揭晓仪式归 Watch 剧场（哲学一致）· iPhone 不剧透不冒充**.

**Acceptance Criteria:**

**Given** Story 4.5 box.unlock.complete 收到 + 客户端检测到 `WatchTransport.isReachable == false`
**When** RealBoxStateStore 处理
**Then** state yield `.unlockedPendingReveal(box: PendingBox)` · UI 渲染 `BoxUnlockedPendingRevealCard`（UX-DR6）：
- 卡片图：包装盒"绑了蝴蝶结但还没拆"姿态
- 文案"已解锁·待揭晓（等待 Watch 上线）"
- **不显示皮肤名 / 颜色 / 任何 spoiler**（Watch 剧场未播 iPhone 不剧透）

**Given** server 端 `box.state == "unlocked_pending_reveal"`（S-SRV-16 新增态）
**When** 客户端拉 `/v1/boxes/pending` 包含此态
**Then** state 还原 · UI 渲染 BoxUnlockedPendingRevealCard

**Given** Watch 重新可达（用户戴上 Watch / Watch 重连 server）
**When** server 检测到并推 WS `box.unlock.revealed` event
**Then** server 同时向 Watch 推开箱动画触发 + 向 iPhone 推 state 更新
**And** RealBoxStateStore state yield `.unlocked(skin: Skin)` · iPhone UI 切换为简洁"接到家了 [skin name]"卡片（与 Story 4.5 unlocked 路径一致）

**Given** 用户从未戴 Watch（observer mode）
**When** Story 4.5 server 解锁
**Then** server 直接 yield `.unlocked` 跳过 pending_reveal 中间态（不卡死 observer 用户 · 跨仓约定 · server-handoff doc）
**And** iPhone 用户仍看简洁卡片

**Given** PendingReveal 持续超过 7 天（Watch 一直不可达）
**When** server 检测
**Then** server 自动发 `box.unlock.revealed` 强制揭晓（防永久卡死）· iPhone state 更新

**Given** 单元 / 集成测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `BoxUnlockedPendingRevealTests.testYieldsWhenWatchUnreachable`
- `BoxUnlockedPendingRevealCardTests.testNoSpoilerAboutSkin`
- `BoxUnlockedPendingRevealTests.testWatchReachableTriggersRevealed`
- `BoxUnlockedPendingRevealTests.testObserverModeSkipsIntermediateState`
- `BoxUnlockedPendingRevealTests.testForceRevealAfter7Days`

**FRs**：FR35d（unlocked_pending_reveal 中间态）+ S-SRV-16（server 端中间态）

---

### Story 4.7: 皮肤颜色分级 + SkinRarityBadge（FR37 + S-SRV-7 loot-box 合规）

As a **User browsing my unlocked skins**,
I want **每张皮肤显示稀有度（白/灰/蓝/绿/紫/橙 6 级）· 图形标记 + 文字标签 + 色（三重冗余防色盲）· 合规披露概率**,
So that **稀有度信息无障碍可读 · loot-box 合规（NFR22 + S-SRV-7）**.

**Acceptance Criteria:**

**Given** Skin 数据结构
**When** 检查 `CatShared/Models/Skin.swift`
**Then** `struct Skin: Sendable, Codable { let id, name: String; let rarity: Rarity; let imageName: String }` · `enum Rarity: String, Sendable, Codable { case common, uncommon, rare, epic, legendary, mythic }`（6 级 · 白/灰/蓝/绿/紫/橙对应）

**Given** `CatShared/UI/SkinRarityBadge.swift` 存在（Story 1C.5 占位 · 本 Story 实装细节）
**When** 渲染 `SkinRarityBadge(rarity: .epic, label: "紫·史诗")`
**Then** 渲染**三重冗余**（NFR22 + UX-DR15）：
- **色**：紫色（DesignTokens 稀有度 token · 60-70% 低饱和）
- **形状**：六边形（与 .legendary 圆形 / .common 方形等区分）
- **文字**：标签 "紫·史诗"

**Given** 6 级稀有度形状映射表
**When** 检查实装
**Then**：
- `.common` → 圆角方形 · 白色
- `.uncommon` → 方形 · 灰色
- `.rare` → 三角形 · 蓝色
- `.epic` → 六边形 · 紫色（修正：应分散）
- `.legendary` → 星形 · 橙色
- 准确清单：以 PRD §C-SKIN-01 + UX 规范定义 · 实装时与 UX 团队 final review · 形状-颜色映射稳定不变（防色盲混淆）

**Given** S-SRV-7 概率披露（loot-box 合规）
**When** 用户进 Inventory tab + 长按 SkinRarityBadge 或进 "盲盒说明" 入口
**Then** 显示概率披露 sheet：
- 列表 6 级稀有度 + 各级掉落概率（来自 server `/v1/box/probability` API · S-SRV-7）
- 文案"基础掉落概率 · 实际可能因当前活动调整"
- 国内合规要求（《盲盒经营活动规范指引》）

**Given** VoiceOver
**When** focus 到 SkinRarityBadge
**Then** 读 `accessibilityLabel = "[colorName] · [rarityName]"`（如 "蓝·精良"）（UX-DR13）

**Given** 用户视觉色觉障碍
**When** 仅看图形 + 文字（屏蔽色彩）
**Then** 仍可区分 6 级稀有度（NFR22 · 形状-字标签信息完整）

**Given** 单元测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `SkinRarityBadgeTests.testAllSixRaritiesHaveDistinctShape`
- `SkinRarityBadgeTests.testColorIsRedundantNotPrimary`（断言色失败时形状 + 文字仍可识别）
- `SkinRarityBadgeTests.testVoiceOverLabelComplete`
- `BoxProbabilityDisclosureTests.testShowsSixRarityProbabilities`
- `BoxProbabilityDisclosureTests.testCopyMeetsLootBoxCompliance`

**FRs**：FR37（皮肤颜色分级）+ NFR22（色盲冗余）+ S-SRV-7（概率披露）

---

### Story 4.8: 仓库 UI（InventoryTabView + SkinCardGrid + SkinCard）+ 重复转材料（FR38 + FR39）

As a **User who has unlocked some skins**,
I want **仓库 tab 看到已入库皮肤网格 + 材料计数 · 重复皮肤自动归材料显示"材料入库"**,
So that **收藏可视化 · 材料为后续 Epic 5 合成铺垫**.

**Acceptance Criteria:**

**Given** Story 4.5 box.unlock 完成 · skin 入库
**When** 渲染 `InventoryTabView`（CatPhone/Features/Inventory/）
**Then** 顶部 segmented control "已入库 / 材料"（FR39 二分栏）

**Given** 用户在 "已入库" tab
**When** 视图加载
**Then** 调 `GET /v1/inventory/skins` · 返回 unique skins 列表
**And** 渲染 `SkinCardGrid`（UX-DR6 · `LazyVGrid` · 2 列 · NFR5 性能要求 stable id）
**And** 每个 cell `SkinCard(skin)` 显示 skin 缩略图 + SkinRarityBadge（Story 4.7）

**Given** 用户点 SkinCard
**When** 点击
**Then** 弹 `SkinDetailSheet`（UX-DR6）放大显示 + "装扮" 按钮（跳 Epic 5 装扮 Story · 本 Story 仅按钮存在 · 实际跳转在 Epic 5）

**Given** server 端开盒返回的 skin 已存在于用户 inventory（重复）
**When** Story 4.5 box.unlock.complete 收到 + server 标记 `is_duplicate: true`（S-SRV 协议字段）
**Then** RealBoxStateStore 处理：
- state 走特殊 yield `.unlocked(skin)` 但 UI 文案改"材料入库"（FR38 · 永远不说"重复皮肤"避免负面感受）
- 实际不入"已入库" · 增加对应稀有度的 material count

**Given** 用户切到 "材料" tab
**When** 视图加载
**Then** 调 `GET /v1/inventory/materials` · 返回 6 级材料计数
**And** 渲染 `MaterialCounter` × 6（UX-DR6）显示：
- 每个稀有度的当前材料数 / 合成所需数（候选 5 → 1 升级）
- 材料 ≥ 5 时高亮"可合成"按钮（跳 Epic 5 Story · 本 Story 按钮占位）

**Given** 仓库为空（无 skin 无材料）
**When** 渲染
**Then** "已入库" tab 显示 `EmptyStoryPrompt(icon: archivebox, title: "仓库还是空的", subtitle: "等第一个盒子到家")`
**And** "材料" tab 显示 `EmptyStoryPrompt(icon: sparkles, title: "还没有材料", subtitle: "开出重复的皮肤会留在这里")`

**Given** NFR5 性能要求（皮肤数 > 200）
**When** SkinCardGrid 渲染
**Then** 用 `LazyVGrid` + `id: skin.id` stable id · 滚动 60fps · 不暴力展开 ForEach

**Given** 单元 / UI 测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `InventoryTabViewTests.testSegmentedControlSwitches`
- `SkinCardGridTests.testRendersStableIds`
- `SkinCardGridTests.testEmptyStateShown`
- `SkinDetailSheetTests.testDressupButtonExists`
- `DuplicateSkinTests.testCopyShowsMaterialIncoming`（FR38 文案 · 禁"重复"）
- `MaterialCounterTests.testRendersAllSixRarities`
- `MaterialCounterTests.testHighlightsWhenCraftable`
- `InventoryPerformanceTests.testLazyGridScrolls60fpsWith300Skins`（NFR5）

**FRs**：FR38（重复转材料 + 文案）+ FR39（仓库 UI）+ NFR5（性能）

---

### Story 4.9: 解锁服务 fail-closed 整合 + S-SRV-14 降级路径（容错聚合）

As an **iOS developer ensuring Epic 4 reliability**,
I want **将本 Epic 所有 fail-closed 路径（HealthKit 拒绝 / 解锁 ACK 超时 / S-SRV-14 Redis 降级）统一引用 ADR-001 §Decision Matrix · 全部走 GentleFailView 模板 + observable signal**,
So that **fail-closed 语义在 Epic 4 内一致 · 通过 grep gate（AR10.2）+ PR template 勾选项**.

**Acceptance Criteria:**

**Given** 本 Story 是 Epic 4 收尾整合 · 不引新功能 · 整理前序 Story 的失败路径
**When** 检查 Epic 4 所有 GentleFailView 调用点
**Then** 均符合 ADR-001 §Decision Matrix（Story 1A.6）：
- HealthKit denied → row "HealthKit 拒绝" · fail-closed degrade · `Log.health.warn + Analytics.track("healthkit_denied")`
- box.unlock ACK 超时 → row "盲盒解锁 ACK 超时" · fail-closed UI "已达成·等待确认" · `Log.ws.warn + track`
- S-SRV-14 Redis 降级 → row "Redis 降级 → 解锁不可用" · fail-closed UI · `Log.ws.error + track("box_unlock_redis_degraded")`
- /steps/history 失败 → row "API 网络失败" · fail-closed 仅历史图降级 · `Log.network.error + track`

**Given** 整合后跑 grep gate（AR10.2 Story 1A.7）
**When** `bash ios/scripts/check-fail-closed-sites.sh`
**Then** Epic 4 所有 site 通过 grep gate · 退出码 0
**And** 缺失 metric 打点 → CI block

**Given** PR template 勾选项（AR10.3）
**When** Epic 4 所有 PR 准备 merge
**Then** description 含勾选项 · 引用 ADR-001 specific row

**Given** 跨 Epic fail-closed 一致性
**When** Epic 4 完成 · 整体回归 ADR-001
**Then** 若发现 ADR 未覆盖的新 fail 场景 · bump ADR-001（添加 row）+ 更新 grep gate
**And** 在 epics.md frontmatter `stepsCompleted` 之外另起 changeLog 节记录 ADR bump 历史

**Given** 容错路径的真机验证（Epic 9 归口）
**When** Epic 9 真机 smoke
**Then** 真机网络断开 + 重连 · 真机 HealthKit 拒绝路径手工验证（FR45b 类感官验证 · 自动化补不到）

**Given** 单元测试 + 集成测试
**When** 运行 `xcodebuild test`
**Then** 通过 Epic 4 整合断言：
- `Epic4FailClosedConsistencyTests.testAllFailSitesHaveLogAndAnalytics`（grep all GentleFailView usages 验证）
- `Epic4FailClosedConsistencyTests.testAllFailSitesReferenceADR001Row`（comments 或 metadata 含 ADR-001 row 引用）
- `Epic4ADRConsistencyTests.testNoNewFailureWithoutADRBump`

**FRs**：（本 Story 无新 FR · 整合 Epic 4 各 Story 的 fail-closed 路径 · 实施 AR10 完整闭环）

---

## Epic 5: 合成 & 装扮（经济闭环 Part 2）

**Epic Goal**：User 把重复材料合成为更高阶皮肤 → 在 iPhone 大屏选中装扮 → Watch payoff 渲染。"收藏升华 + 自我表达"闭环。合成幂等防双端 / 断网 / 并发；跨时区挂机计时正确（ClockProtocol 跨时区扩展）。

**Story 编号**：5.1-5.4 共 4 Story · 启动条件：Epic 4 完成（仓库 + 材料 ready）

### Story 5.1: CraftConfirmSheet + 合成触发（FR40 上半 · 用户行为）

As a **User who has 5+ duplicate materials of one rarity**,
I want **在材料 tab 点 "可合成" → 弹出 GentleConfirmSheet "让它们换一条新的（5 条蓝色条纹会变成 1 条绿色条纹）" → 确认后触发合成**,
So that **破坏性操作前明确确认 · 文案业务语境（不用"删除/合并/消耗"等硬词 · UX Pattern 6）**.

**Acceptance Criteria:**

**Given** Epic 4 Story 4.8 用户在 "材料" tab · 某稀有度材料数 ≥ 合成所需（候选 5 → 1 升级）
**When** MaterialCounter 显示 "可合成" 高亮按钮 + 用户点击
**Then** 弹出 `CraftConfirmSheet`（CatPhone/Features/Inventory/ · UX-DR6 + UX-DR9 GentleConfirmSheet 复用）：
- 标题"让它们换一条新的"（业务语境 · 禁"合成/删除/合并"等硬词 · UX Pattern 6）
- Body "5 条蓝色条纹会变成 1 条绿色条纹"（具体描述会发生什么 · 用户能预测后果）
- 取消按钮（左 · 低权重）+ "换一条" 按钮（右 · 强调色 clay/500）
- 单手可达性：按钮在屏幕下半区（NFR + UX-DR16）

**Given** 用户点 "换一条"（确认）
**When** Sheet dismiss
**Then** 调 Story 5.2 `CraftService.craft(rarityFrom: .uncommon, count: 5)` 触发实际 API 流程
**And** UI 进入合成中态：BoxStateStore 类似 → 立即显示 `CraftInProgressSheet`（spinner 仅此场景例外允许 · UX Pattern 5 备注 · 2s 内完成）

**Given** 用户点 "取消"
**When** Sheet dismiss
**Then** 不调 API · 材料数不变 · 无任何副作用

**Given** Sheet 文案 grep 验证（UX Pattern 6 + UX-DR10）
**When** PR 检查
**Then** Sheet 内文案**零命中**：
- "删除" / "消耗" / "合并" / "失去" / "牺牲"（业务语境违规词）
- 任何感叹号
- "不可恢复"（恐吓性文案 · 改"换一条" 暗示新生）

**Given** Reduce Motion
**When** Sheet 弹出
**Then** 弹出动画降级为瞬切

**Given** VoiceOver
**When** focus 到 Sheet
**Then** 读出 标题 + Body + 两按钮 label

**Given** 单元 / UI 测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `CraftConfirmSheetTests.testRendersTitleAndBody`
- `CraftConfirmSheetTests.testCopyContainsNoForbiddenWords`（grep 6 禁词）
- `CraftConfirmSheetTests.testConfirmTriggersCraftService`
- `CraftConfirmSheetTests.testCancelDoesNothing`
- `CraftConfirmSheetTests.testButtonsInBottomHalfForSingleHand`
- `CraftConfirmSheetUITests.testReduceMotionNoSheetAnimation`

**FRs**：FR40（合成触发 UI 层 · 用户行为部分）+ UX Pattern 6

---

### Story 5.2: `CraftService` 实装 · 幂等 / 并发 / 断网（FR40 下半 · S-SRV-10）

As an **iOS developer ensuring craft reliability**,
I want **`CraftService` 实装 POST /v1/craft 幂等端点 · 含中途断网重试 / 材料不足拒绝 / 并发双端合成幂等**,
So that **用户走过 Epic 4 仓库辛苦攒的材料绝不丢失 · server S-SRV-10 幂等契约严格遵守**.

**Acceptance Criteria:**

**Given** `CatCore/Craft/CraftService.swift` 实装
**When** 检查 protocol + impl
**Then** `protocol CraftService: Sendable { func craft(rarityFrom: Rarity, count: Int) async throws -> Skin; var craftHistory: AsyncStream<CraftEvent> { get } }` · `actor RealCraftService` 实装

**Given** Story 5.1 用户确认合成
**When** 调 `craftService.craft(rarityFrom: .uncommon, count: 5)`
**Then** 内部生成 `craft_request_id = UUID v4`（幂等 key · S-SRV-10 contract）+ POST `/v1/craft` 含 `{ rarityFrom, count, craft_request_id }`
**And** 同一 craft_request_id 在 server 端 60s 内幂等去重（S-SRV-10 与 dedup 60s · 注意非 client_msg_id 300s · 单独契约）

**Given** server 验证材料是否足够
**When** server 返回 `200 OK` + `{ resultSkin: { id, name, rarity, ... } }`
**Then** 客户端 craftHistory yield `.success(skin)` · UI 切换为 `CraftSuccessSheet` 显示新皮肤
**And** Inventory 自动 refresh（材料数减 5 + 新皮肤入"已入库" tab · 通过 BoxStateStore / InventoryStore 一致刷新）

**Given** server 返回 `409 Conflict { error: "insufficient_materials" }`（用户走神 · 客户端 cache 与 server 不同步）
**When** 客户端处理
**Then** 显示 `GentleFailView(reason: .craftInsufficient, retry: ...)` 文案"材料正好不够，等下个盒子凑齐"（UX Pattern 2）
**And** UI Inventory force refresh（让客户端材料数与 server 同步）
**And** `Log.network.warn("craft_insufficient_materials")` + `Analytics.track("craft_insufficient_materials")`

**Given** 中途断网（POST /v1/craft 已发送但客户端未收到响应 · 用户重试）
**When** 客户端重发同一 craft_request_id
**Then** server 60s 内幂等 · 返回相同 resultSkin（不重复扣材料）· 用户看到一致结果不丢材料

**Given** 双端并发合成（极罕见 · 用户在 iPhone 同时点合成 + Watch 也触发了合成）
**When** 两个 craft_request_id 都到达 server
**Then** server 按 server-handoff-ux-step10 + S-SRV-10 contract 处理（先到先得 + 第二个 409 insufficient · 客户端按 409 路径走）
**And** 客户端不试图本地"避免重复"· server 权威

**Given** 合成 API 完全超时（≥ 10s 无响应）
**When** WSClient/APIClient 抛 timeout
**Then** 显示 `GentleFailView(reason: .craftTimeout, retry: ...)` 文案"合成没成功，材料还在仓库" + "再试一次"（UX Pattern 2）
**And** 重试时**复用同一 craft_request_id**（幂等 · 防重复扣材料）
**And** `Log.network.error("craft_timeout")` + `Analytics.track("craft_timeout")`

**Given** 单元 / 集成测试 + WSFakeServerV1
**When** 运行 `xcodebuild test`
**Then** 通过：
- `CraftServiceTests.testSuccessReturnsSkin`
- `CraftServiceTests.testInsufficientShowsFailViewAndRefreshesInventory`
- `CraftServiceTests.testRetryReusesSameRequestId`（幂等 · 关键防丢材料）
- `CraftServiceTests.testTimeoutShowsRetryFailView`
- `CraftServiceTests.testConcurrentCraftSecondReturns409`
- `CraftServiceTests.testCraftRequestIdIsUuidV4`

**FRs**：FR40（合成幂等 / 断网 / 并发）+ S-SRV-10（/craft 幂等端点）

---

### Story 5.3: 装扮选择 iPhone 大屏（FR41 · DressupTabView + SkinGalleryPicker + DressupPreview）

As a **User with unlocked skins**,
I want **iPhone 装扮 tab 大屏选中皮肤 · 静态预览猫穿着选中皮肤的样子**,
So that **iPhone "慢房间"分工 payoff（深度编辑 vs Watch 实时反馈）· 不需要 Watch 即可完成装扮决策**.

**Acceptance Criteria:**

**Given** Epic 4 Story 4.8 用户已有 ≥ 1 个皮肤入库
**When** 用户切到 Dressup tab（主 TabBar 5 tab 之一）
**Then** 渲染 `DressupTabView`（CatPhone/Features/Dressup/ · UX-DR7）：
- 上半区：`DressupPreview` 静态预览猫穿着当前选中皮肤（**iPhone 静态 · 不依赖 Spine** · D6 决策）
- 下半区：`SkinGalleryPicker` 已入库皮肤栅格（LazyVGrid · 复用 Story 4.8 SkinCard 风格）

**Given** 用户已有 0 个皮肤
**When** 渲染
**Then** 显示 `EmptyStoryPrompt(icon: tshirt, title: "还没有皮肤可换", subtitle: "等盒子接回家先")`（UX Pattern 1 · 无 CTA）

**Given** 用户在 SkinGalleryPicker 选某 skin
**When** 点击 skin cell
**Then** UI **本地立即更新** DressupPreview 显示新 skin（fire-and-forget 风格 · 不等 server）
**And** 后台调 Story 5.4 装扮 payoff 流程（POST /v1/cat/dressup · 发 server fan-out 给 Watch）

**Given** 当前选中 skin 高亮
**When** 渲染
**Then** SkinGalleryPicker 中当前 skin 有视觉边框 + 选中状态（DesignTokens 强调色 clay/500）

**Given** 用户切换不同皮肤多次
**When** 频繁点击
**Then** 客户端**节流 / 去重**：500ms 内多次切换仅取最后一次发 server（防 spam）
**And** UI 仍立即响应（无 spinner）

**Given** DressupPreview 静态图加载
**When** skin 资产存在 `CatShared/Resources/Skins/`
**Then** 用 `Image("skin_blue_stripe")` 静态图 · 不触发 Lottie / Spine（NFR32 · iPhone 不依赖 Spine）

**Given** Skin 资产缺失（异常）
**When** Image 加载失败
**Then** 静默降级为默认皮肤图 + `Log.ui.error("skin_asset_missing", skinId)` + `Analytics.track("skin_asset_missing")`（fail-open · 不阻断）

**Given** Reduce Motion
**When** 切换 skin
**Then** DressupPreview 切换无淡入淡出 · 直接替换（UX-DR14）

**Given** VoiceOver
**When** focus 到 SkinGalleryPicker
**Then** 每个 cell 读 `[skinName] · [稀有度] · [是否当前装扮]`（UX-DR13）

**Given** 单元 / UI 测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `DressupTabViewTests.testRendersPreviewAndPicker`
- `DressupTabViewTests.testEmptyStateForNoSkins`
- `SkinGalleryPickerTests.testSelectionUpdatesPreviewLocally`
- `SkinGalleryPickerTests.testThrottle500msToServer`
- `DressupPreviewTests.testNoSpineImport`（grep `import Spine` 零命中 · NFR32 硬纪律）
- `DressupPreviewTests.testFallbackOnMissingAsset`
- `SkinGalleryPickerTests.testCurrentSelectionHighlighted`

**FRs**：FR41（iPhone 大屏装扮选择）

---

### Story 5.4: 装扮 payoff 触发 → Watch 渲染（FR42 iPhone 端） + FR43 跨时区挂机计时

As a **User who picks a skin in iPhone Dressup**,
I want **iPhone 触发后 server 转发给 Watch · Watch 端实时看到猫换皮肤（payoff 在 Watch 剧场）· 同时跨天/跨时区挂机计时正确不假死/重置**,
So that **iPhone 决策 / Watch 实时 = 双端分工最强例证 · 跨时区用户挂机计时不被 24 时区差错重置**.

**Acceptance Criteria:**

**Given** Story 5.3 用户在 SkinGalleryPicker 选某 skin（500ms 节流后取最后一次）
**When** 客户端调 POST `/v1/cat/dressup` `{ skin_id, client_msg_id }`
**Then** server 端：
- 验证 skin 是否在用户 inventory · 不在则 403
- 写入 server 端"当前装扮"
- WS fan-out `pet.dressup.update { user_id, skin_id, ts }` 给所有订阅者（含本用户的 Watch · 含房间其他成员）

**Given** server 返回 200 OK
**When** 客户端收到
**Then** 不显示任何"已发送"UI（保持 fire-and-forget 对称性 · 用户已在 Story 5.3 看到本地 preview）
**And** Watch 端（Epic 9 真机）收到 fan-out · 渲染开始换装动画（FR42 Watch 端实施 · 本 Story iPhone 仅触发）

**Given** API 失败（403 / 5xx / 超时）
**When** 客户端处理
**Then** UI **不立即报错**（dressup 是非关键路径）：
- 4s 静默重试 1 次（exponential backoff）
- 仍失败 → 弹 `GentleFailView(reason: .dressupFailed, retry: ...)` 文案"装扮没换上，再点一次"
- DressupPreview 本地仍显示用户选的（不强制 rollback · 避免视觉跳动）
- `Log.network.warn("dressup_failed")` + `Analytics.track("dressup_failed")`

**Given** FR43 跨时区挂机计时基准
**When** 检查 `ClockProtocol`（Story 1C.1）+ 实际使用方
**Then** 所有挂机计时基准（Story 4.4 BoxStateStore 30min idle 触发）使用：
- **server 权威 timestamp**（不依赖客户端本地时区 · server 用 UTC + iPhone 用本地时区显示）
- 客户端 ClockProtocol 仅用作 UI 展示 / 测试 fast-forward · 不参与 idle 判定

**Given** 用户跨时区飞行（手机系统时区从 UTC+8 切到 UTC-5）
**When** 检测时区变化
**Then** 客户端**不重置**任何挂机计时（避免误重置）· server 端按 UTC 时间戳延续判定 · iPhone UI 切换显示新时区（如"5 分钟前"基于本地时区 + UTC 差计算）

**Given** 跨天（00:00 时刻）
**When** 用户挂机跨过午夜
**Then** 挂机计时**不被本地日期切换打断**（server UTC 时间戳是连续的 · 客户端无需特殊处理）
**And** 步数显示按本地日切换（HKStatisticsCollectionQuery 按本地 calendar day 聚合 · NFR29）

**Given** ClockProtocol 单元测试 + MockClock + TimeZone 注入
**When** 运行 `xcodebuild test`
**Then** 通过：
- `DressupServiceTests.testSuccessTriggersWSFanout`
- `DressupServiceTests.testFailureGentleFailViewAfterRetry`
- `DressupServiceTests.testNoUIErrorOnTransientFailure`
- `IdleTimerCrossTimezoneTests.testTimezoneChangeDoesNotResetIdle`
- `IdleTimerCrossTimezoneTests.testMidnightCrossingPreservesIdle`
- `StepCountTimezoneTests.testStepsAggregateByLocalCalendarDay`

**FRs**：FR42（iPhone 触发装扮 · Watch payoff 渲染由 Epic 9 真机验证）+ FR43（跨时区挂机计时）

---

## Epic 6: 好友 & 房间（社交核心）

**Epic Goal**：User 加好友 → 建房 / 加入房间 → 看见朋友的状态（叙事流 · 观察者文字层）→ 房间内发表情 server 广播。社交回路激活 · 观察者隐私 gate 严格执行（字段按订阅者类型裁剪）。

**Story 编号**：6.1-6.9 共 9 Story · 启动条件：Epic 1 全 + Epic 3（MyCatCard + EmojiPicker 基础）

### Story 6.1: 好友请求 + 接受/拒绝（FR11 + FR12）

As a **User who knows a friend's Cat ID or scans their QR**,
I want **发送好友请求 · 对方能接受/拒绝 · 请求状态在 UI 清晰**,
So that **建立好友关系进入房间前置**.

**Acceptance Criteria:**

**Given** `CatPhone/Features/Friends/FriendsTabView.swift` · Friends tab 渲染
**When** 用户点 "加好友" 入口
**Then** 弹出 `AddFriendSheet`（两种输入方式）：
- 输入 Cat ID（UUID 或人类友好 ID）
- 扫 QR（相机权限 · Info.plist `NSCameraUsageDescription` · 扫对方的 InviteQR）

**Given** 用户输入有效 Cat ID
**When** 点击 "发送请求"
**Then** 调 POST `/v1/friends/requests` `{ target_user_id }` · 返回 200 + requestId
**And** UI 显示 toast "请求已送出，等它去看看"（祈使文案 · 不说"等待回复"）· 不阻塞其他操作

**Given** 接收方 User
**When** FriendsTabView 加载或 WS 推 `friend.request.incoming`
**Then** 顶部显示 `FriendRequestBanner`（UX-DR5）含请求方名称 / 头像 + "同意" / "先不" 按钮
**And** 按钮文案业务语境（禁"接受/拒绝" · "同意" = 加好友 · "先不" = 拒绝）

**Given** 接收方点 "同意"
**When** 调 POST `/v1/friends/requests/{id}/accept`
**Then** 成功后双方 friend 列表互相出现（server 端关系建立 · 双端 WS 推 `friend.added`）· Banner 消失

**Given** 接收方点 "先不"
**When** 调 DELETE `/v1/friends/requests/{id}`
**Then** 请求清除 · 发送方**不收到"被拒绝"推送**（保护对方感受 · 发送方只看到"等它去看看"一直不变 · UX 决策）

**Given** 用户重复发请求给同一对象
**When** 调 API
**Then** server 返回 409 Conflict · 客户端显示 toast "已经在等它回复了"（不重新发）· `Log.network.info("friend_request_duplicate")` + `Analytics.track("friend_request_duplicate")`

**Given** 已经是好友 / 对方已屏蔽自己 / 对方不存在 / 自己加自己
**When** API 返回 4xx
**Then** UI 显示 GentleFailView 温柔文案：
- 409 "你们已经是朋友了" · 跳转到好友列表高亮 TA
- 403 (blocked by other) "TA 暂时不想添加新朋友"（不暴露被屏蔽事实）
- 404 "这个 Cat ID 不存在"
- 422 (self) "你不能加自己"（简单 validation · 实际 UI 先 disable send）

**Given** 单元 / UI 测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `FriendRequestTests.testSendRequestSuccess`
- `FriendRequestBannerTests.testAcceptTriggersAPI`
- `FriendRequestBannerTests.testDeclineDoesNotNotifySender`（隐私 + UX）
- `FriendRequestTests.testCopyUsesBusinessLanguage`（grep "接受/拒绝" 零命中）
- `FriendRequestTests.testAllRejectionBranchesHaveCopy`（5 分支都有文案）

**FRs**：FR11（发送请求）+ FR12（接受/拒绝）

---

### Story 6.2: 好友列表 + 屏蔽/移除（FR13 + FR14）

As a **User with established friends**,
I want **查看好友列表（姓名 + 头像 + 最近状态）· 长按可屏蔽 / 移除**,
So that **管理社交关系 · 负面社交工具（屏蔽/移除）随手可得**.

**Acceptance Criteria:**

**Given** Friends tab
**When** 用户好友数 ≥ 1
**Then** 渲染 `FriendListView`（UX-DR5 · List + 长按 context menu）· 每行显示：
- 头像 / 猫 avatar
- 昵称（FR52 由 Epic 7 设置页提供）
- 最近一句叙事文案（如"刚刚在挂机" · 来自 Story 6.8 规则表 · 不精确时间）
- 在线/离线 dot（server 推 `friend.presence` · 本地显示）

**Given** 好友数 = 0
**When** 渲染
**Then** 显示 `EmptyStoryPrompt(icon: person.2, title: "你还没有朋友", subtitle: "扫码把 TA 拉回家，一起挂机", cta: ("邀请朋友", onTap))`（UX Pattern 1）· CTA 跳 Story 6.1 AddFriendSheet

**Given** 用户长按好友行
**When** 触发 context menu
**Then** 显示两项：
- **"静音 TA 的表情"**（Story 6.3 处理）
- **"屏蔽 TA"**（危险操作 · 弹 `GentleConfirmSheet` 二次确认）
- **"从朋友里移除"**（危险操作 · 弹 GentleConfirmSheet）

**Given** 用户点 "屏蔽 TA" + 二次确认
**When** 调 POST `/v1/friends/{userId}/block`
**Then** server 端关系变 blocked · 双方看不到对方的任何内容（FR14）· 客户端 FriendListView 自动 refresh（TA 从列表消失）
**And** 若当前在同一房间 → 客户端自动离开房间（server 强制 kick）

**Given** 用户点 "从朋友里移除" + 二次确认
**When** 调 DELETE `/v1/friends/{userId}`
**Then** 双方好友关系删除（但未屏蔽 · 可重新加）· 列表移除

**Given** 屏蔽/移除的 ConfirmSheet 文案（UX Pattern 6）
**When** 渲染
**Then** 禁"删除/拉黑/举报"硬词 · 改"屏蔽 TA" / "从朋友里移除"（业务语境）· 取消按钮左低权重 + 执行按钮右 clay/500

**Given** 单元 / UI 测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `FriendListViewTests.testRendersFriendsWithPresence`
- `FriendListViewTests.testEmptyStateShown`
- `FriendContextMenuTests.testThreeActionsAvailable`
- `FriendBlockTests.testConfirmSheetRequired`
- `FriendBlockTests.testBlockedFriendRemovedFromRoom`
- `FriendRemoveTests.testFriendshipOnlyNotBlocked`

**FRs**：FR13（好友列表）+ FR14（屏蔽/移除）+ FR51（空状态）

---

### Story 6.3: 静音好友表情 + 举报（FR15 + FR16）

As a **User with specific friend whose emotes are overwhelming / inappropriate**,
I want **静音 TA 的表情广播（不收 push / 震动但保留好友）· 或举报恶意内容**,
So that **负面社交工具精细分级 · 保好友关系但降扰动**.

**Acceptance Criteria:**

**Given** Story 6.2 好友 context menu
**When** 用户点 "静音 TA 的表情"
**Then** 调 POST `/v1/friends/{userId}/mute` `{ type: "emote" }`
**And** 本地 `KeyValueStore.set("muted_emote_from_\(userId)", value: true)` 缓存
**And** UI toast "已静音 TA 的表情 · TA 不会知道"（保护对方感受）

**Given** 已静音好友发表情
**When** WS 推 `pet.emote.broadcast` 含该好友 userId
**Then** 客户端**检查 mute list** → 静音后走"降级分支"：
- 不触发 haptic
- 不弹 push 通知
- 房间 RoomStoryFeed 仍显示叙事文字（保社交存在感 · 仅屏蔽打扰）

**Given** 用户想取消静音
**When** 再次点 context menu 或 Epic 7 Settings 查看静音列表
**Then** "取消静音" · 调 DELETE `/v1/friends/{userId}/mute`

**Given** Story 6.2 好友 context menu
**When** 用户点 "举报 TA"（第四项 · 非 MVP 必需但 App Store 合规要求）
**Then** 弹 `ReportSheet` 含预设理由（4 选项）：
- "内容不合适"
- "骚扰行为"
- "垃圾信息"
- "其他"（含文本输入 · max 140 字）

**Given** 用户提交举报
**When** 调 POST `/v1/reports` `{ targetUserId, reason, detail }`
**Then** 成功后 toast "已收到举报 · 我们会认真看"· 不告知被举报方
**And** 建议用户顺手屏蔽（ReportSheet dismiss 后弹"要不要也屏蔽 TA"二次 prompt · 可选不强制）

**Given** App Store compliance
**When** 检查举报流程
**Then** 必须 24h 内服务端响应（server 承诺 · 跨仓契约）· 客户端 UI 不承诺具体时效避免合规风险

**Given** 单元测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `MuteEmoteTests.testMuteTriggersAPI`
- `MuteEmoteTests.testMutedEmoteSkipsHapticAndPush`
- `MuteEmoteTests.testRoomStoryFeedStillShows`
- `MuteEmoteTests.testMuteIsNotNotifiedToTarget`
- `ReportSheetTests.testFourPresetReasons`
- `ReportSheetTests.testDetailMaxLength140`

**FRs**：FR15（静音）+ FR16（举报）

---

### Story 6.4: 创建房间 + 加入/离开（FR17a + FR17b）

As a **User who wants to share space with friends**,
I want **在 Room tab 创建房间 · 加入已有房间 · 随时离开**,
So that **房间是社交存在感容器 · 多人同屏挂机（Watch 端 payoff）的前提**.

**Acceptance Criteria:**

**Given** 用户在主 TabBar Room tab · 尚未在任何房间
**When** 渲染 `RoomTabView`（CatPhone/Features/Room/ · UX-DR8）
**Then** 显示未在房间态：
- `EmptyStoryPrompt(icon: house, title: "还没进房间", subtitle: "和朋友一起挂机，看对方的猫", cta: ...)` · 双 CTA "创建房间" / "加入房间"

**Given** 用户点 "创建房间"
**When** 触发
**Then** 调 POST `/v1/rooms` · 返回 `{ roomId, inviteCode }`
**And** 客户端 `CurrentRoom.@Observable` 更新 · UI 切换为"已在房间"态
**And** NFR26 `单房间 max 4 人` · 创建时 room 空（1 人）

**Given** 用户点 "加入房间"
**When** 触发
**Then** 弹出输入 inviteCode 或扫 QR 选项（Story 6.6 邀请流程）

**Given** 用户已在房间
**When** RoomTabView 渲染
**Then** 显示已在房间态：
- 顶部房间名 / 邀请码 · "邀请按钮"（Story 6.6）
- 中部 `RoomMemberList`（Story 6.7）
- 下部 `RoomStoryFeed`（Story 6.8）
- 底部 "离开房间" 按钮

**Given** 用户点 "离开房间"
**When** 用户非房主
**Then** 弹 `GentleConfirmSheet` "离开房间" · body "你走了还可以再回来"· 确认后调 POST `/v1/rooms/{id}/leave` · 客户端 CurrentRoom = nil · UI 切回未在房间态

**Given** 用户是房主（Story 6.5 转让/解散）
**When** 用户点 "离开房间"
**Then** 走 Story 6.5 转让/解散流程 · 非简单离开

**Given** NFR26 单用户最多 1 房间
**When** 用户已在 A 房间 · 尝试加入 B 房间
**Then** 客户端先提示 `GentleConfirmSheet` "你要先离开 [A] 房间"· 确认后自动先离开再加入

**Given** 房间 server 端已解散（例如房主解散）
**When** WS 推 `room.dissolved`
**Then** 客户端 CurrentRoom = nil · UI 切回未在房间态 · 显示 toast "[房主] 结束了房间"

**Given** 单元测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `RoomCreateTests.testCreatesAndUpdatesCurrentRoom`
- `RoomJoinTests.testSwitchRoomRequiresLeaveFirst`
- `RoomLeaveTests.testConfirmSheetForNonOwner`
- `RoomDissolutionTests.testWSDissolvedUpdatesUI`
- `RoomTabViewTests.testEmptyVsActiveStateSwitch`
- `RoomCapacityTests.testMaxFourMembers`

**FRs**：FR17a（创建）+ FR17b（加入/离开）

---

### Story 6.5: 房主转让 / 解散房间（FR17c）

As a **Room owner who wants to leave**,
I want **选择转让房主给别人 / 或解散整个房间**,
So that **体面退房 · 不强制其他成员被动失去房间**.

**Acceptance Criteria:**

**Given** Story 6.4 用户是房主（server 记录 `room.owner == userId`）· 点 "离开房间"
**When** 触发
**Then** **不直接离开** · 弹 `OwnerLeaveSheet`（CatPhone/Features/Room/）三选一：
- "转让给 [member name]"（如果房间有其他成员 · 默认选择在线时间最长的）
- "解散房间"（所有人同时离开）
- "取消"（留下）

**Given** 用户选 "转让给 [member name]"
**When** 调 POST `/v1/rooms/{id}/transfer` `{ new_owner_id }`
**Then** server 更新 room.owner · WS 推 `room.owner.changed` 给所有成员
**And** 新房主收到 toast "[原房主] 把房间交给你了"· 原房主自动离开

**Given** 用户选 "解散房间"
**When** 弹第二层 GentleConfirmSheet "房间会消失 · 其他 [N] 人会被带回家"
**Then** 确认后调 DELETE `/v1/rooms/{id}`
**And** server 推 `room.dissolved` 给所有成员（包括房主自己）· 全员 UI 切回未在房间态

**Given** 房间只有房主 1 人
**When** 点 "离开房间"
**Then** 跳过 OwnerLeaveSheet · 直接弹 GentleConfirmSheet "离开后房间消失"· 确认后 POST /leave 等价于解散

**Given** OwnerLeaveSheet 文案（UX Pattern 6）
**When** 渲染
**Then** 禁"删除/销毁/关闭"硬词 · 改"解散 / 转让"业务语境

**Given** 转让成功后房主角色变更的 UI 反馈
**When** 新房主在 Room tab
**Then** 成员列表显示 TA 为 "house.fill" 图标或"房主"徽章（视觉区分）

**Given** 单元测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `OwnerLeaveSheetTests.testThreeOptions`
- `OwnerLeaveSheetTests.testSoloOwnerSkipsSheet`
- `TransferRoomTests.testNewOwnerReceivesToast`
- `DissolveRoomTests.testAllMembersKickedToEmpty`
- `RoomOwnerBadgeTests.testVisualDistinction`
- `OwnerLeaveCopyTests.testBusinessLanguage`

**FRs**：FR17c（房主转让/解散 · 体面退房）

---

### Story 6.6: 邀请链接 / 二维码 + 5 分支失败 + 加入流程（FR18 + FR19 + FR20）

As a **Room member who wants to invite friends**,
I want **生成二维码 / 可分享链接 · 朋友点开或扫码即可加入 · 失败 5 分支明确提示**,
So that **降低邀请摩擦 · 失败路径不假死**.

**Acceptance Criteria:**

**Given** Story 6.4 用户在房间 · RoomTabView 有 RoomInviteButton
**When** 点击
**Then** 弹 `InviteQRSheet`（UX-DR5 · CatPhone/Features/Friends/）：
- QR code 显示（encode `cat://join?room=xxx&code=yyy`）
- 下方"复制链接"按钮（复制 `https://cat.app/join?room=xxx&code=yyy`）
- 分享 "转给朋友" 按钮（iOS `UIActivityViewController`）
- 说明"链接 30 分钟有效"（server 侧 TTL · 跨仓契约）

**Given** 邀请链接 TTL 30 分钟（server 权威）
**When** server 记录 inviteCode expiresAt
**Then** 客户端显示倒计时（30min）· 过期后按钮变灰 + 文案"链接过期了，重新生成"

**Given** 接收方点 universal link 或 cat://scheme
**When** deep link handler（`SceneDelegate` or `UIApplication.handleURL`）
**Then** 解析 roomId + code · 调 POST `/v1/rooms/{id}/join` `{ invite_code }`

**Given** server 返回 200 OK
**When** 成功加入
**Then** 客户端 CurrentRoom 更新 · 导航到 Room tab 已在房间态

**Given** 接收方扫 QR 或点链接 · **FR19 5 失败分支**
**When** server 返回 4xx
**Then** 客户端**每分支独立文案**（GentleFailView · UX Pattern 2）：
- **410 Gone (过期)** → "这次错过了" + 按钮 "让 TA 再邀你" · `Log.network.warn("invite_expired")` + `Analytics.track("invite_expired")`
- **409 Conflict (已是好友在房)** → 不显示错 · 直接加入
- **409 Conflict (已在同一房间)** → toast "你已经在这个房间了"· 跳转到 Room tab
- **409 Conflict (房间满 4 人)** → "房间满了 · 让 TA 下次再叫你"· 无重试按钮
- **400 Bad Request (自邀)** → "你不能邀请自己"· validation 前端应先 disable

**Given** 用户未安装 app 点 universal link
**When** iOS 系统处理
**Then** 跳 App Store 安装页 · 安装后 app 冷启动时 deep link 已记录（`UserActivity` continuation）· 直接进 Story 2 Onboarding · onboarding 完成后自动 join 房间

**Given** 相机权限被拒（扫 QR 场景）
**When** 用户进 AddFriendSheet 或 InviteQRSheet 的扫码入口
**Then** UI 显示 `GentleFailView(reason: .cameraDenied, retry: "去设置开启")` · 跳系统设置

**Given** InviteQR 文案 · 禁 "邀请二维码 / 扫码功能/ 立即加入" 等僵硬词
**When** 渲染
**Then** "让朋友看这个图 · TA 就到了"（温柔文案）

**Given** 单元 / UI 测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `InviteQRSheetTests.testQRAndLinkRendered`
- `InviteQRSheetTests.test30MinCountdown`
- `InviteJoinTests.testUniversalLinkHandled`
- `InviteJoinTests.testDeepLinkSchemeHandled`
- `InviteFailureBranchesTests.testAllFiveBranchesHaveCopy`（FR19 硬要求）
- `InviteFailureBranchesTests.testExpiredShowsRetryViaSender`
- `InviteJoinTests.testAutoJoinAfterOnboarding`（冷启动场景）
- `CameraPermissionTests.testDeniedShowsGentleFail`

**FRs**：FR18（邀请链接/QR）+ FR19（5 失败分支）+ FR20（加入房间）

---

### Story 6.7: 房间成员名单 + 观察者隐私 gate（FR21 + FR25）

As a **User in a room**,
I want **看到成员列表（姓名 + 在线/挂机状态 + 一句话叙事文案）· 观察者只能看 activity + since_last_state_change，不能看具体步数**,
So that **社交存在感明确 · 观察者模式隐私 gate 严格（FR25 防数据泄露）**.

**Acceptance Criteria:**

**Given** Story 6.4 用户已在房间 · WS 推 `room.members.update` 或初次拉 `/v1/rooms/{id}/members`
**When** RoomTabView 渲染
**Then** `RoomMemberList`（UX-DR8 · CatPhone/Features/Room/）· **iPhone 端纯文字列表 · 无猫渲染**（Watch 剧场专属）· 每行：
- 头像
- 昵称
- 在线/挂机状态 dot
- 一句话叙事文案（来自 Story 6.8 规则表 · 如"刚刚在挂机"）

**Given** 当前用户是观察者（Story 2.3 `KeyValueStore.string("user_watch_mode") == "observer_only"`）· 或 server 根据用户 Watch 状态判定
**When** server 推 `pet.state.broadcast` 含其他成员状态
**Then** 客户端收到的 payload 已被 server 侧字段裁剪（**server 权威 gate** · FR25 + S-SRV-1）：
- **可见**：`current_activity`（如 "walking" / "sitting" / "sleeping"）· `minutes_since_last_state_change`（如 "5 min 前"）
- **不可见**：`exact_step_count` · `health_*`（心率 / 卡路里 / 任何 HealthKit 衍生数据）

**Given** 客户端 UI 渲染 payload
**When** 即使客户端代码误访问字段
**Then** 字段**不存在于 payload**（server 不下发 · 非客户端加密隐藏）· 即双重保险：server 权威 gate + 客户端无字段可渲染

**Given** RoomMemberList 渲染观察者与非观察者混合房间
**When** 某成员是观察者
**Then** 成员头像旁小图标标识 "iPhone only" · 点图标显示 tooltip "TA 还没戴 Watch · 先通过手机看大家"
**And** 观察者成员的叙事文案用"可见范围"生成（如"TA 在看"而非"TA 走了 347 步"）

**Given** 房间成员变化（加入 / 离开 / 房主转让）
**When** server 推 `room.members.update`
**Then** RoomMemberList 自动更新 · 人数 / 房主徽章变化（Story 6.5）

**Given** 隐私 gate 测试（关键 · 防数据泄露 · AR8.4 §21.4 语义正确性）
**When** 运行 integration test
**Then** 断言客户端收到的 WS payload 不含 exact_step_count / health_* 字段（即使 WSFakeServerV1 推含这些字段的 test payload · 客户端应 `Log.ws.error("privacy_gate_violation")` 并不渲染）

**Given** 单元 / 集成测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `RoomMemberListTests.testRendersNameAndStatus`
- `RoomMemberListTests.testNoSpineRenderInIPhone`
- `ObserverPrivacyGateTests.testObserverPayloadExcludesStepCount`
- `ObserverPrivacyGateTests.testObserverPayloadExcludesHealthFields`
- `ObserverPrivacyGateTests.testClientLogsOnPrivacyViolation`
- `ObserverBadgeTests.testIphoneOnlyIconShown`

**FRs**：FR21（成员名单）+ FR25（观察者隐私 gate）+ S-SRV-1（server 字段裁剪契约）

---

### Story 6.8: 叙事文案规则表 + RoomStoryFeed（FR22 · UX Pattern 7）

As a **User in a room watching friends' activity**,
I want **看房间叙事流（时态模糊化文案 · "刚刚 / X min 前 / 今天早些时候"）· 不是精确时间戳 / 原始步数**,
So that **温柔陪伴感 · 非数据报告 · 符合"配件端慢浏览" iPhone 哲学**.

**Acceptance Criteria:**

**Given** `CatShared/Sources/CatShared/UI/StoryLine.swift` 实装
**When** 检查
**Then** `struct StoryLine: View { let text: String; let time: Date; let category: StoryCategory }` · `enum StoryCategory: Sendable { case idle, walking, sleeping, emote, unlock, dressup, entered }`

**Given** 叙事文案**规则表**（FR22 · AC 精确断言要求）
**When** 检查 `CatShared/Sources/CatShared/Utilities/NarrativeTemplateEngine.swift`
**Then** 规则表结构：
```swift
struct NarrativeTemplate {
    let trigger: StoryCategory
    let timeRange: TimeRange // .now / .minutesAgo(5-30) / .earlierToday / .yesterday ...
    let templateId: String
    let text: (String /*subject*/) -> String
}

// 示例
.init(trigger: .walking, timeRange: .now, templateId: "walking_now",
      text: { "\($0) 刚刚在挂机" })
.init(trigger: .walking, timeRange: .minutesAgo(5...30), templateId: "walking_recent",
      text: { "\($0) 在走路 · 几分钟前" })
```
**And** 规则表**只在本 Story 集中维护**（后续 Epic 新增文案 bump 表版本 + PR review 一致性）

**Given** 时态模糊化档位（UX Pattern 7）
**When** 规则表实装
**Then** 档位：
- 0-5 min → "刚刚"
- 5-30 min → "X min 前"（最大到 "29 min 前"）
- 30-120 min → "今天早些时候" / "半小时前"
- 120+ min 同日 → "今天上午" / "今天下午"
- 前一日 → "昨晚" / "昨天早上"
- 更早 → "前两天" / "上周"
- **禁用**：精确时间戳（`14:32:18`）· ISO 字符串

**Given** RoomStoryFeed 渲染
**When** 服务端推 `room.story.event` 或客户端聚合状态
**Then** `RoomStoryFeed` 组件（CatPhone/Features/Room/）· List of StoryLine · 倒序显示 · Bear 风格留白（DesignTokens 间距 · 行间 24pt）

**Given** 服务端与客户端一致副本（UX Pattern 7）
**When** server 也有同样的 NarrativeTemplate 规则
**Then** 客户端使用本地规则 · server 规则在 MVP 可偏离但以 server 为权威（跨端契约）· 规则表热更新推 Growth

**Given** 规则表完整性校验
**When** 检查所有 `StoryCategory` case
**Then** 每个 category 至少有 3 个时态模板（`.now` / `.minutesAgo` / `.earlierToday`）· 无 category 缺失

**Given** 精确时间戳 grep 验证
**When** PR 检查
**Then** RoomStoryFeed 相关代码 grep `.formatted(.dateTime)` / `ISO8601` 零命中（UX Pattern 7 硬约束）

**Given** 单元测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `NarrativeTemplateEngineTests.testAllCategoriesHaveThreeTimeRanges`
- `NarrativeTemplateEngineTests.testTimeFuzzinessRules`（断言 0-5 → "刚刚" / 10 min → "10 min 前" / 昨天 → "昨晚" 等）
- `RoomStoryFeedTests.testNoPreciseTimestamps`
- `RoomStoryFeedTests.testBearStyleSpacing`
- `StoryLineTests.testAccessibilityLabelFormat`

**FRs**：FR22（叙事文案规则表）+ UX Pattern 7

---

### Story 6.9: 房间内发表情 server 广播 + iPhone 观察者 push（FR28）

As a **User sending emojis while in a room**,
I want **emoji 触发 server fan-out 给房间其他成员（Watch 轻震 + emoji 浮在 emitter 猫旁 · iPhone 观察者走 push）**,
So that **房间社交广播闭环 · fire-and-forget 对称性（Epic 3 Story 3.4）延伸到房间场景**.

**Acceptance Criteria:**

**Given** Epic 3 Story 3.4 `EmoteService.send` 已实装 · 用户在房间内（`currentRoomId != nil`）
**When** 用户点猫 → 选 emoji → Story 3.4 `WSClient.send(type: "cat.tap", payload: { emoji, room_id: currentRoomId, client_msg_id }, awaitResponse: false)`
**Then** server 处理（S-SRV-2 fan-out）：
- 去重（client_msg_id 300s）
- 记日志
- **向房间其他成员 WS 推 `pet.emote.broadcast` { emitter_user_id, emoji, room_id, ts }**
- **不向 emitter 推 delivered ack**（S-SRV-17 硬约束 · Story 3.4 已覆盖）

**Given** 房间成员 A 收到 fan-out
**When** WSClient messages stream yield `pet.emote.broadcast`
**Then** A 的客户端：
- 在 RoomMemberList 中 emitter 的 row 旁浮起 EmojiEmitView（复用 Story 3.3 组件）
- 触发 light haptic（非 emitter 自己）
- 追加一条 StoryLine 到 RoomStoryFeed "[emitter name] 给房间发了 [emoji]"

**Given** Epic 2 Story 2.3 用户选"观察者模式"（`user_watch_mode == "observer_only"`）
**When** 不在 app 内但有 fan-out 到达
**Then** server 走 APNs push 通道 · 推送文案"[朋友的猫] 给房间发了 [emoji]"（FR28 观察者 push）
**And** push payload 含 `{ type: "observer_emote", emitter_id, emoji, room_id }` · 用户点 push 跳回 Room tab

**Given** 用户在前台 + app 在 Room tab
**When** 收到 fan-out
**Then** **不走 push**（避免本地 + push 双通知）· 只 in-app 本地呈现

**Given** 静音的好友（Story 6.3 `muted_emote_from_<uid> == true`）
**When** 收到该好友的 fan-out
**Then** 客户端本地 gate：
- 不触发 haptic
- 不弹 push
- **RoomStoryFeed 仍显示**（保社交存在感 · Story 6.3 规则）

**Given** 用户是 emitter 自己
**When** 收到自己的 `pet.emote.broadcast`（server echo · 罕见）
**Then** 客户端**忽略**（emitter 已在 Story 3.3 本地呈现 · 防止双显示）· `Log.ws.debug("ignore_own_emote_broadcast")`

**Given** fail-open 静默降级（UX Pattern 3）
**When** fan-out 接收失败（WS 断线中）
**Then** UI 不显示错误（用户不应感知到本应看到的 emote 没到）· 重连后 Story 1B.3 `session.resume` 拉快照补齐（不保证 100% 不丢 · TTL 24h · `NFR16`）

**Given** Epic 3 fire-and-forget 9 禁用词 grep 扩展到本 Story
**When** PR 检查
**Then** RoomStoryFeed + push 文案**零命中** "已读 / 已送达 / 对方已收到"（Story 3.4 grep gate 覆盖扩展）

**Given** Epic 6 fail-closed 整合（像 Story 4.9）
**When** 整 Epic 所有 GentleFailView 调用点
**Then** 均符合 ADR-001 Decision Matrix（邀请 5 分支 / 相机权限拒绝 / 房间已满 / 举报 / 好友请求冲突 · 全部引用对应 row）
**And** grep gate 通过 · PR template 勾选项验证

**Given** 单元 / 集成测试 + WSFakeServerV1
**When** 运行 `xcodebuild test`
**Then** 通过：
- `RoomEmoteFanoutTests.testFanoutToOtherMembers`
- `RoomEmoteFanoutTests.testEmitterNotNotifiedByServer`（S-SRV-17）
- `RoomEmoteFanoutTests.testObserverReceivesViaPush`
- `RoomEmoteFanoutTests.testForegroundInRoomNoPush`
- `RoomEmoteFanoutTests.testMutedFriendHaptsAndPushSuppressed`
- `RoomEmoteFanoutTests.testStoryFeedShowsEvenWhenMuted`
- `RoomEmoteFanoutTests.testIgnoreOwnEchoBroadcast`
- `Epic6FailClosedConsistencyTests.testAllFailSitesReferenceADR001Row`

**FRs**：FR28（房间内发表情 server 广播 + iPhone 观察者 push）+ Epic 6 fail-closed 整合（AR10 闭环延伸）

---

## Epic 7: 账户 & 隐私合规 & 可观测性 MVP

**Epic Goal**：User 对自己的数据有控制权（sign out / 删除 / 导出）· 设置页 identity 编辑 + 隐私政策 + push 分频道 · App Store 合规过审（PrivacyInfo.xcprivacy）· Dev 崩溃可追踪（Xcode Organizer）+ 版本可控 + kill switch（硬编码）· 实装 `MilestonesSyncService` 完成 Epic 2 S-SRV-15 软 gate 闭环。

**Story 编号**：7.1-7.7 共 7 Story · 启动条件：Epic 1 全（Foundation 必须先绿）· 上线发布前必须全绿（App Store 审核硬门槛）

### Story 7.1: SettingsTabView 入口 + 静态信息页（FR53）

As a **User who wants to view privacy policy / terms / account info**,
I want **从 Account tab 进 Settings 页查看 4 类静态信息（隐私政策 / 使用条款 / 账号信息 / HealthKit 状态）**,
So that **App Store 4.8 合规可访问 · 用户随时可读到隐私承诺**.

**Acceptance Criteria:**

**Given** Epic 3 Story 3.1 AccountTabView 右上角 Settings 图标
**When** 用户点击
**Then** 导航到 `SettingsView`（CatPhone/Features/Account/Settings/）· iOS 系统级 NavigationStack push

**Given** SettingsView 渲染
**When** 视图加载
**Then** 显示 List 分组（按 iOS HIG）：
- **账号** section · 含 "猫名" / "Cat ID" / "Apple ID 邮箱" 行（只读 · 来自 Story 1B.6 AuthSession + Story 2.5 cat_name）
- **隐私** section · 含 "HealthKit 权限"（Story 4.1 状态 + 跳转）/ "通知权限"（系统设置跳转）
- **关于** section · 含 "隐私政策" / "使用条款" / "App 版本" 行
- **危险区** section · 含 "Sign out" / "删除账号" 行（Story 7.4 / 7.5 处理）

**Given** 用户点 "隐私政策"
**When** 触发
**Then** 内嵌 `WKWebView` 加载远程 URL `https://cat.app/privacy`（hardcoded MVP · Growth 推 FR61 远程配置）
**And** WebView 顶部 navigation bar 含 "完成" 按钮关闭

**Given** 用户点 "使用条款"
**When** 触发
**Then** 同上 · 加载 `https://cat.app/terms`

**Given** 用户点 "App 版本"
**When** 触发
**Then** 显示 `Bundle.main.infoDictionary?["CFBundleShortVersionString"]` + Build number（如 "1.0.0 (42)"）· 长按可复制

**Given** WebView 加载失败（网络异常）
**When** 错误抛出
**Then** 显示 `GentleFailView(reason: .webContentFailed, retry: ...)` 文案"暂时打不开 · 你可以稍后再来"
**And** `Log.network.warn("settings_webview_failed", url: ...)` + `Analytics.track("settings_webview_failed")`

**Given** Dynamic Type 大字号
**When** 渲染 List
**Then** 行高自适应 · 不截断（UX-DR12）

**Given** VoiceOver
**When** focus 到每行
**Then** 读出 "section name + row name + 当前值（如适用）"

**Given** 单元测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `SettingsViewTests.testFourSectionsRender`
- `SettingsViewTests.testCatNameAndAppleIdShown`
- `SettingsViewTests.testWebViewOpensOnPolicyTap`
- `SettingsViewTests.testAppVersionDisplaysCorrectly`
- `SettingsViewTests.testWebFailureShowsGentleFailView`

**FRs**：FR53（设置页 - 隐私政策 / 条款 / 账号信息）

---

### Story 7.2: Identity 编辑（昵称 / 头像 / 个性签名 · FR52）

As a **User who wants to personalize my profile**,
I want **在 Settings 编辑昵称 / 头像 / 个性签名 · 立即同步到 server**,
So that **identity 表达 · 房间叙事流 / 好友列表显示自己的猫的样子**.

**Acceptance Criteria:**

**Given** Story 7.1 Settings 账号 section
**When** 用户点 "编辑资料" 行
**Then** push `EditProfileView`（CatPhone/Features/Account/Settings/）· 三 field：
- **昵称**：TextField · 1-12 字符 · 同 Story 2.5 验证规则
- **头像**：当前头像 + "更换" 按钮 → PHPickerViewController 选 / 系统相机拍（Info.plist `NSCameraUsageDescription` 已含）
- **个性签名**：TextEditor · max 60 字 · 占位"一句话介绍你和你的猫"

**Given** 用户编辑完点 "保存"
**When** 调 PATCH `/v1/users/me` `{ nickname?, avatar_image_url?, bio? }`
**Then** 头像走两步：
- 先调 POST `/v1/users/me/avatar/upload-url` 获取 pre-signed S3 URL
- PUT 上传图（max 2MB · 客户端预压缩）
- 拿到 final URL 后 PATCH 资料
**And** 成功后客户端本地 `KeyValueStore.set("user_nickname", value: ...)` 缓存 · UI 立即更新

**Given** 头像上传失败（网络 / 文件超限）
**When** 错误抛出
**Then** 显示 `GentleFailView(reason: .avatarUploadFailed, retry: ...)` 文案"头像没传上 · 文字资料已保存"（部分成功 · 友好降级）
**And** `Log.network.warn("avatar_upload_failed")` + `Analytics.track("avatar_upload_failed")`

**Given** 昵称重复（server 返回 409）
**When** 检测
**Then** UI 实时校验 · TextField 下方红字 "TA 在用了 · 试试别的"· save 按钮 disable

**Given** 个性签名含敏感词（server 端 reject · 422）
**When** 检测
**Then** 显示 toast "这句话发不出去 · 换个说法"· 不提示具体哪个词（防绕过）

**Given** 编辑完成 + 取消未保存
**When** 用户点返回
**Then** 弹 `GentleConfirmSheet` "改的还没保存 · 真的不要吗?" · 确认才退（防误退）

**Given** 单元 / UI 测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `EditProfileViewTests.testThreeFieldsRender`
- `EditProfileViewTests.testNicknameValidation`
- `EditProfileViewTests.testBioMaxLength60`
- `EditProfileViewTests.testAvatarTwoStepUpload`
- `EditProfileViewTests.testPartialFailureGracefully`
- `EditProfileViewTests.testUnsavedChangesPrompt`
- `EditProfileViewTests.testNicknameDuplicateRealtimeValidation`

**FRs**：FR52（昵称 / 头像 / 个性签名编辑）

---

### Story 7.3: Push 分频道开关（FR55）

As a **User who wants control over notifications**,
I want **在 Settings 分频道开关 push（好友 / 表情 2 档 · 盲盒档已废）**,
So that **不被无关通知打扰 · 用户对推送有掌控感**.

**Acceptance Criteria:**

**Given** Story 7.1 Settings 隐私 section
**When** 用户点 "通知"
**Then** push `NotificationSettingsView`（CatPhone/Features/Account/Settings/）· 顶部显示系统级通知权限状态 + "去开启" 按钮跳系统设置（如未开）

**Given** 系统级通知已开启
**When** 视图渲染
**Then** 显示 2 档分频道开关 Toggle：
- **"朋友邀请"**（FR55 · 默认 ON）
- **"房间表情广播"**（FR55 · 默认 ON）
- **(已废)**：盲盒掉落档 · v2 修订 1 已移除（FR36 作废）· 不显示

**Given** 用户切换某档 Toggle
**When** 变化
**Then** 调 PATCH `/v1/users/me/notification-preferences` `{ channel: "friend_invite" | "room_emote", enabled: Bool }`
**And** 成功后本地 `KeyValueStore.set("push_pref_<channel>", value: enabled)` 缓存

**Given** server 端按用户偏好 fan-out
**When** 推送即将发出
**Then** server 检查偏好 · 未启用的频道不发 push（**server 权威 gate** · 即使客户端 cache 失效也不漏 push 控制）

**Given** API 失败
**When** 错误抛出
**Then** Toggle 自动回滚到原状态 · toast "保存失败 · 等下再试"
**And** `Log.network.warn("push_pref_save_failed", channel: ...)` + `Analytics.track("push_pref_save_failed")`

**Given** 系统级通知未开
**When** 用户尝试切 channel toggle
**Then** Toggle disable + 提示"先去系统设置开通知"（系统级是上层 gate · 应用级 toggle 失效）

**Given** Reduce Motion
**When** Toggle 切换
**Then** 标准 SwiftUI Toggle 动画（系统级 reduce motion 自适应 · 无需特殊处理）

**Given** 单元测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `NotificationSettingsViewTests.testTwoChannelsRender`
- `NotificationSettingsViewTests.testNoBoxDropChannel`（v2 作废验证）
- `NotificationSettingsViewTests.testToggleCallsAPI`
- `NotificationSettingsViewTests.testFailureRollsBackToggle`
- `NotificationSettingsViewTests.testSystemDisabledDisablesAppToggles`

**FRs**：FR55（push 分频道）

---

### Story 7.4: Sign out + Keychain 清理（FR10）

As a **User who wants to switch accounts or temporarily exit**,
I want **从 Settings 一键 sign out · 清除本地 token + cache · 不删除 server 数据**,
So that **账号切换轻量 · sign out 不等同删除（用户不会误失数据）**.

**Acceptance Criteria:**

**Given** Story 7.1 Settings 危险区
**When** 用户点 "Sign out"
**Then** 弹 `GentleConfirmSheet` "你要先离开吗 · 数据都还在"（**强调 sign out ≠ 删除**）· 取消按钮左低权重 + "离开"按钮右 clay/500

**Given** 用户确认
**When** 触发 sign out 流程
**Then** 顺序执行：
1. 调 POST `/v1/auth/signout` 通知 server 失效 token（best-effort · 失败不阻塞）
2. 调 `AuthTokenProvider.clearTokens()`（Story 1B.4 · `SecItemDelete` Keychain 清理）
3. 调 `KeyValueStore` 清理用户级 cache（昵称 / 头像 URL / push 偏好 · 保留全局非用户级 · 如 `onboarding_narrative_skipped_count`）
4. 调 `WSClient.disconnect()` 断开 WS
5. `AuthSession.session = nil` · `@Observable` 触发根 navigation 回到 Story 2.2 SIWA 屏

**Given** Sign out 完成
**When** UI 切换
**Then** Onboarding flow **跳过 Story 2.1 叙事卡片**（`FirstTimeExperienceTracker.hasSeen(.onboardingNarrativeCompleted) == true` 仍为 true · 不重新看叙事）· 直接进 SIWA 登录屏

**Given** 用户重新登录同一 SIWA
**When** Story 2.2 SIWA 完成
**Then** server 识别为同一 user_id · 数据全部恢复（cat name / 好友 / inventory 等 · server 权威）· 跳过 Story 2.4-2.5（已 onboardingCompleted）· 直接进主 TabBar

**Given** Story 1B.4 Keychain 清理彻底性
**When** sign out 后 grep
**Then** Keychain query `SecItemCopyMatching(... kSecAttrService = "com.cat.catphone.auth")` 返回 errSecItemNotFound（彻底清理）

**Given** Sign out 期间网络异常
**When** server `/auth/signout` 调用失败
**Then** 仍执行本地清理（best-effort · 不依赖 server）· 用户 UX 仍正常 · `Log.network.warn("signout_api_failed_local_cleanup_done")`

**Given** 单元 / UI 测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `SignOutTests.testFiveStepCleanupSequence`
- `SignOutTests.testKeychainEmptyAfterSignOut`
- `SignOutTests.testFirstTimeKeysPreservedAcrossSignOut`
- `SignOutTests.testServerFailureDoesNotBlockLocal`
- `SignOutTests.testReSignInRestoresState`
- `SignOutSheetTests.testCopyEmphasizesNotDeletion`（grep "删除" 在 sheet 文案零命中）

**FRs**：FR10（sign out + Keychain 清理）+ 复用 FR57（Keychain 由 Story 1B.4 实装）

---

### Story 7.5: 账户删除 + 数据导出（FR8 + FR9 + S-SRV-12 · GDPR）

As a **User exercising GDPR rights**,
I want **请求删除账号（不可恢复）或导出个人数据（机器可读）**,
So that **GDPR 合规 · 用户对数据有最终控制权**.

**Acceptance Criteria:**

**Given** Story 7.1 Settings 危险区
**When** 用户点 "删除账号"
**Then** push `DeleteAccountFlowView`（CatPhone/Features/Account/Settings/）· **3 步流程防误删**：
1. 解释影响（cat name / inventory / 好友 / 房间 全部删除 · 30 天后服务端硬删 · 可在 30 天内重新 sign in 撤销）
2. 输入"删除"二字确认（防一键误删 · GDPR 推荐做法）
3. 最终 `GentleConfirmSheet` 二次确认

**Given** 三步全部确认
**When** 调 DELETE `/v1/users/me` `{ confirmation: "delete" }` · S-SRV-12
**Then** server 返回 200 + `delete_scheduled_at`（30 天后硬删时间）
**And** 客户端本地走 Story 7.4 sign out 流程清理 token + cache · UI 跳回 SIWA 屏 + toast "30 天内重新登录可撤销"

**Given** 用户在 30 天内重新 SIWA 登录
**When** Story 2.2 SIWA 完成 + server 识别 pending delete user
**Then** 弹 `GentleConfirmSheet` "你的账号还在等删除 · 要不留下吗?" 三选一：
- "留下" → server 取消删除 · 数据全恢复
- "继续删除" → 直接进 sign out 状态
- "取消"（默认）→ 不变（继续 pending delete）

**Given** Settings 危险区另一项 "导出我的数据"
**When** 用户点击
**Then** push `DataExportView`（同 settings 子 view）· 解释"会通过邮件发给你 [Apple ID 邮箱] · 可能等几分钟"

**Given** 用户点 "请求导出"
**When** 调 POST `/v1/users/me/export-request` · S-SRV-12
**Then** server 异步生成 JSON 包（含 cat_name / friends list / room history / inventory · GDPR right to data portability）· 完成后邮件发 user
**And** 客户端 toast "请求已收到 · 等几分钟看邮箱"· UI 不阻塞

**Given** 导出请求频率限制
**When** 用户 24h 内已请求过
**Then** server 返回 429 · 客户端显示 "今天已经请求过 · 明天再来"（防滥用）

**Given** 删除流程文案（GDPR · UX Pattern 6）
**When** PR 检查
**Then** "删除"作为业务术语**允许出现**（GDPR 合规要求明确告知 · 与 Pattern 6 普通业务文案禁词不同 · 此处显式 exception）· 但不用感叹号 · 不用恐吓性"永久删除！"等

**Given** 单元 / UI 测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `DeleteAccountFlowTests.testThreeStepGate`
- `DeleteAccountFlowTests.testTextConfirmationRequired`
- `DeleteAccountFlowTests.testReSignInWithin30DaysOffersUndo`
- `DataExportFlowTests.testRequestTriggersAsyncEmail`
- `DataExportFlowTests.testRateLimit429ShowsCopy`
- `GDPRComplianceTests.testDeletionIsExplicitNotEuphemism`（合规反例 · 删除流程文案不能用 "离开 / 撤销" 等模糊词）

**FRs**：FR8（账户删除）+ FR9（数据导出）+ S-SRV-12（删除 + 导出 API）

---

### Story 7.6: 崩溃上报 + 版本检查 + 远程配置 MVP（FR59 + FR60 + FR61）

As an **iOS team operator**,
I want **MVP 用最轻量方案：Xcode Organizer 收崩溃 / App Store 原生版本检查 / 硬编码 kill switch · Growth 再升级三方 SDK**,
So that **MVP 不引 Crashlytics/Sentry/远程配置平台依赖 · `PCT-arch` "不引过度依赖" 纪律 · 上线后基础可观测仍有兜底**.

**Acceptance Criteria:**

**Given** FR59 崩溃上报 MVP 方案
**When** 检查 Xcode 工程配置
**Then** Build Settings 含 `DEBUG_INFORMATION_FORMAT = dwarf-with-dsym`（生成 dSYM）· Archive 流程产物含 dSYM
**And** Info.plist `NSAppTransportSecurity` 不破例（Apple Crash Reports 走 Apple 内部通道 · 客户端零代码集成）
**And** `ios/docs/dev/troubleshooting.md`（Story 1A.4）追加 "Xcode Organizer 查崩溃" 步骤

**Given** FR60 版本检查 MVP 方案
**When** App 启动 + 第一次 foreground
**Then** 调 iTunes Search API `https://itunes.apple.com/lookup?bundleId=com.cat.catphone&country=cn` · 解析 `results[0].version`
**And** 比对 `Bundle.main.infoDictionary?["CFBundleShortVersionString"]`
**And** 若 server version > local 且 server 标记为 `.recommended`（硬编码门槛 · 客户端判定）→ 弹 `UpdateAvailableSheet` 含 "去 App Store" 按钮跳应用商店
**And** 若 server version > local 且差距 ≥ 3 个版本 / 标记 `.required` → 弹强制更新（仅推荐 · 不阻塞 app · MVP）· Growth 推 in-app 强更

**Given** iTunes Search API 失败 / 超时
**When** 错误抛出
**Then** 静默吞掉（fail-open · 版本检查非关键路径）· `Log.network.info("version_check_failed")` · 不影响主功能

**Given** FR61 远程配置 MVP 方案
**When** 检查 `CatShared/Sources/CatShared/Utilities/RemoteConfig.swift`
**Then** `enum RemoteConfigKey { case killSwitchInventoryV2; case killSwitchEmoteCount; ... }` · 默认值**硬编码**在 client（MVP）· 后期 Growth 接入远程配置平台时同接口 swap impl
**And** `protocol RemoteConfigProvider: Sendable { func bool(_ key: RemoteConfigKey) -> Bool; func int(_ key: RemoteConfigKey) -> Int }` · `HardcodedRemoteConfigProvider` 唯一 MVP impl

**Given** Kill switch 使用场景
**When** 某 Feature（如 Inventory v2 实验）出问题
**Then** Hot fix 发版本 + 改 `HardcodedRemoteConfigProvider` 中默认值 · 用户更新后立即生效
**And** **不在 MVP 引入远程配置 SDK**（Firebase Remote Config / LaunchDarkly · Growth 推）· 遵 D7 决策 + `PCT-arch`

**Given** S-SRV-18 sender 端 metric 打点
**When** 检查所有 `Analytics.track` 调用点
**Then** 通过 `Story 1A.7` grep gate · 每个 fail-closed site 已配对 metric 打点（前序 Epic 已满足）
**And** 本 Story 验证 `Analytics` 实际实装：MVP 用 `Log.network.info("analytics_event", payload)` 仅 OSLog 写入（不发任何远程 collector · 等 Growth 接入 server-side Prometheus）

**Given** 单元 / 集成测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `VersionCheckTests.testRecommendedShowsSheet`
- `VersionCheckTests.testFailureSilent`
- `VersionCheckTests.testRequiredShowsForceUpdate`（仅推荐 MVP）
- `RemoteConfigTests.testHardcodedDefaultsReturned`
- `RemoteConfigTests.testKillSwitchOverrideWorks`
- `AnalyticsTests.testTrackWritesToOSLogOnly`（MVP 不发远程）

**FRs**：FR59（崩溃 MVP · Xcode Organizer）+ FR60（版本检查 MVP）+ FR61（远程配置 MVP 硬编码）+ S-SRV-18 sender 端打点配套

---

### Story 7.7: `MilestonesSyncService` 实装 + `PrivacyInfo.xcprivacy` + EmptyStoryPrompt 5 场景固化 + Epic 7 收尾

As an **iOS team finalizing MVP for App Store submission**,
I want **PrivacyInfo.xcprivacy 清单 build artifact 校验 + MilestonesSyncService 实装闭环 Epic 2 软 gate + EmptyStoryPrompt 5 场景文案最终固化 + Epic 7 fail-closed 整合**,
So that **App Store 审核硬门槛通过 · S-SRV-15 软 gate 完整闭环 · UX-DR10 Pattern 1 文案统一**.

**Acceptance Criteria:**

**Given** FR63 PrivacyInfo.xcprivacy（iOS 17 硬要求 · 2024/03 起 App Store 强制）
**When** 检查 Xcode 工程
**Then** `CatPhone/Resources/PrivacyInfo.xcprivacy` 存在 · 含必需字段：
- `NSPrivacyAccessedAPITypes`：UserDefaults / FileTimestamp / SystemBootTime（per Apple required reasons API list · 实际使用的标记）
- `NSPrivacyTracking: NO`（不追踪用户）
- `NSPrivacyCollectedDataTypes`：HealthKit step count（与 PRD/server 一致 · 仅本地展示 · 不上传）
- `NSPrivacyTrackingDomains: []`（无追踪域）

**Given** Build artifact 校验
**When** 运行 `bash ios/scripts/build.sh`（Story 1A.1）
**Then** 脚本追加 `xcrun privacy_check` 或等价检查 · 验证 PrivacyInfo.xcprivacy 包含在最终 .ipa
**And** 缺失或字段不全 → 退出码 ≠ 0 · CI block

**Given** Story 2.5 引用的 `MilestonesSyncService`（之前是 protocol 占位）
**When** 检查 `CatCore/Onboarding/MilestonesSyncService.swift`
**Then** `actor RealMilestonesSyncService: MilestonesSyncService` 实装：
- 持 `APIClient + KeyValueStore + ClockProtocol`
- `func attemptSync() async` 实装：
  1. 扫描所有 dirty flag（`onboarding_completed_local_at` / `cat_name_local` / `first_emote_sent_local_at` 等）
  2. 若 server S-SRV-15 已 ready · POST `/v1/users/me/milestones` 同步全部 dirty flag · 成功后清 flag + 写 `_synced` 标记
  3. 若 server 未 ready 或 调用失败 · 保留 dirty flag + log
  4. 检查 30 天 TTL：本地 dirty flag 时间戳 + 30 天 < now → 触发 resync 流程（弹 `GentleFailView(.onboardingResyncRequired)` · Story 2.5 已定义）
- `App` 注册 `UIApplication.willEnterForegroundNotification` 触发 `attemptSync()`

**Given** FR51 全局空状态 5 场景模板（Epic 1 Story 1C.5 已立 EmptyStoryPrompt 组件）
**When** 检查所有 5 场景的最终文案（前序 Epic 已用 · 本 Story 收敛 + grep 验证）
**Then** 5 场景文案与 UX 规范一致：
- 好友 tab 空 → `(person.2, "你还没有朋友", "扫码把 TA 拉回家，一起挂机", CTA "邀请朋友")`
- 仓库空 → `(archivebox, "仓库还是空的", "等第一个盒子到家", 无 CTA)`
- 仓库材料空 → `(sparkles, "还没有材料", "开出重复的皮肤会留在这里", 无 CTA)`
- 房间外 → `(house, "还没进房间", "和朋友一起挂机，看对方的猫", 双 CTA "创建房间" / "加入房间")`
- 房间内无广播 → `(moon.stars, "房间里很安静", "点你的猫打个招呼，朋友能看见", 无 CTA)`

**Given** Epic 7 fail-closed 整合（同 Epic 4 Story 4.9 / Epic 6 Story 6.9 收尾模式）
**When** 检查 Epic 7 所有 GentleFailView 调用点
**Then** 均符合 ADR-001 Decision Matrix · 引用对应 row · `bash ios/scripts/check-fail-closed-sites.sh` 退出码 0
**And** PR template 勾选项验证

**Given** 上线前 final compliance checklist
**When** 检查 `ios/docs/release-checklist.md`（本 Story 创建）
**Then** 文档含：
- PrivacyInfo.xcprivacy 已含必需字段 ✓
- App Store Connect 隐私问卷已填（HealthKit 用途 / 不上传 raw step / 联系方式 = nil）
- Loot-box 合规披露（Story 4.7 概率披露 + S-SRV-7） ✓
- Sign in with Apple 在 SIWA flow 中正确（Story 2.2） ✓
- Export compliance（wss + ATS · FR56 · Story 1B.3） ✓
- Privacy Policy URL `https://cat.app/privacy` 可访问 · 内容含必需 5 节（HealthKit 用途 / push 用途 / 好友图暴露范围 / 不上传 raw step / 数据保留与删除）
- Age Rating 12+

**Given** 单元 / 集成测试 + 整 Epic 整合测试
**When** 运行 `xcodebuild test`
**Then** 通过：
- `PrivacyInfoTests.testRequiredFieldsPresent`（解析 xcprivacy plist 验证）
- `BuildScriptPrivacyCheckTests.testFailsOnMissingFields`
- `MilestonesSyncServiceTests.testSyncSuccessClearsDirtyFlags`
- `MilestonesSyncServiceTests.testSyncFailureKeepsDirtyFlags`
- `MilestonesSyncServiceTests.test30DayTTLTriggersResync`
- `MilestonesSyncServiceTests.testForegroundTriggersAttemptSync`
- `EmptyStoryPromptCopyTests.testFiveScenarioCopyMatchesUXSpec`（grep 5 场景文案精确匹配）
- `Epic7FailClosedConsistencyTests.testAllFailSitesReferenceADR001Row`
- `ReleaseChecklistTests.testAllItemsPresent`（解析 release-checklist.md · 7 必需项验证）

**FRs**：FR51（5 场景空状态文案最终固化）+ FR63（PrivacyInfo.xcprivacy）+ Epic 2 软 gate `MilestonesSyncService` 闭环 + Epic 7 fail-closed 整合 + 上线 compliance checklist

---

## Epic 9: 真机 × Watch 配对 × Spike 验证（Hardware & Manual · per `CLAUDE-21.6`）

**Epic Goal**：Pre-MVP ship gate — 真机环境下核心体验验证通过；感官层（Spine / Lottie / haptic 辨识度）人类确认；上架合规 + alpha tester + legal sync 完成。不阻塞 Epic 1-7 关键路径（§21.6 硬纪律）· 真机 smoke 失败不 block 单元测试持续交付。

**Story 编号**：9.1-9.6 共 6 Story · 启动条件：Epic 1-7 全部 Story 绿（xcodebuild test 一命令绿 · CI 通过）

**特殊性**：本 Epic Story AC 多为**人工验证 / CI 配置 / 操作流程**，而非单元测试。部分 AC 用 checklist 形式 · 需要真人真机执行 · per `[E9]` 测试标签纪律。

### Story 9.1: GitHub Actions CI 工作流（AR1.8 · Story 1A AC6 延后项）

As an **iOS team**,
I want **`.github/workflows/ios.yml` 在 GitHub macOS runner 跑 `bash ios/scripts/build.sh --test` 绿 · PR 合入前 CI block merge**,
So that **CI 绿是 Epic 1-7 每个 PR 的前置条件 · 不依赖单 dev 本地环境**.

**Acceptance Criteria:**

**Given** Epic 1 Story 1A.1-1A.7 已落地（build.sh / install-hooks / swift-format / fail-closed grep gate / ac-triplet-check）
**When** 创建 `.github/workflows/ios.yml`
**Then** workflow 含：
- `on: pull_request, push to main`
- runner: `macos-14`（Xcode 15.x · iOS 17 SDK）
- steps:
  1. `brew install xcodegen swift-format`
  2. `cd ios && xcodegen generate`
  3. `bash ios/scripts/build.sh --test`
  4. `bash ios/scripts/check-fail-closed-sites.sh`（Story 1A.7）
  5. `bash ios/scripts/check-ws-fixtures.sh`（Story 1C.4 · 仅 warning · 不 block）
- 任一 step 非 0 退出码 → job failed → PR merge blocked

**Given** GitHub repo settings
**When** 配置 branch protection rules
**Then** `main` 分支要求 CI 通过才能 merge · Status check 含 `ios / build-and-test` workflow

**Given** CI 运行时间 budget
**When** 单次 PR CI 完成
**Then** 期望 ≤ 15 min（Epic 1-7 total ~55 Story · 每 Story ~20 test · 总 ~1000 test · iPhone simulator 启动 ~3min · 目标总时 ≤ 15min）
**And** 若超 30min → 触发告警 · team 评审是否需要 test 分层 / 并行化

**Given** GitHub Actions runner 偶发 flakiness（模拟器 /brew 网络问题）
**When** CI 失败
**Then** 允许 retry 1 次（workflow 配置 `continue-on-error: false` 但 retry-on-failure 脚本）· 连续 3 次失败 → Slack 告警 team

**Given** CI cache optimization
**When** `brew install` 步骤
**Then** 用 GitHub Actions cache 缓存 homebrew 依赖 + SwiftPM .build/ + Xcode DerivedData 保 CI 速度

**Given** 手工验证
**When** 新 PR 提交测试 workflow
**Then** PR description 自动触发 CI · GitHub UI 显示 check status · 通过后可 merge

**FRs**：（本 Story 无直接 FR · AR1.8 · Story 1A.1 AC6 延后项）

---

### Story 9.2: Release binary linker symbol check（AR6.4 · O4 · WCSession 不在 binary）

As a **tech lead ensuring D5 哲学 B 纪律落地**,
I want **CI 在 release archive 后验证 binary 不含 `WCSession` 相关符号 · 防止某 dev 误 import WatchConnectivity**,
So that **`WatchTransportNoop` 的"binary 纤细"承诺不是口头 · 真实落地 · 确认 iPhone MVP 零 WC 依赖**.

**Acceptance Criteria:**

**Given** Story 1B.8 `WatchTransport` + `WatchTransportNoop`（AR5.5 · 禁 `import WatchConnectivity`）
**When** 创建 `ios/scripts/check-no-wcsession-symbols.sh`
**Then** 脚本逻辑：
```bash
# 对 release archive (.xcarchive/Products/Applications/CatPhone.app/CatPhone) 执行
# nm -U <binary> | grep -i "WCSession\|WatchConnectivity" → 零命中
# otool -L <binary> | grep -i "WatchConnectivity.framework" → 零命中
# 任一命中 → 退出码 1 + stderr 打印命中符号
```

**Given** CI 流程
**When** release build（archive 流程）
**Then** GitHub Actions workflow 新增 `release-verify` job（独立于 PR CI · 仅 release tag / main 分支）：
1. `xcodebuild archive` 生成 .xcarchive
2. `bash ios/scripts/check-no-wcsession-symbols.sh <archive-path>`
3. 失败 → block release tag 创建

**Given** 某 dev 误 `import WatchConnectivity` 或启用 `WATCH_TRANSPORT_ENABLED` flag
**When** CI release-verify 跑
**Then** 脚本退出码 1 · error log 指出文件 + 符号 · team 回到代码修复

**Given** Growth 阶段计划引入 WCSession（PRD `WatchTransport` 三档通道）
**When** 启用 `WATCH_TRANSPORT_ENABLED` build setting
**Then** 脚本升级策略：
- MVP build（无 flag）: 零命中 WCSession
- Growth build（有 flag）: 允许命中 · 但必须 CI 明确声明 `-DWATCH_TRANSPORT_ENABLED=1`
- 通过 `if [ "${WATCH_TRANSPORT_ENABLED}" = "1" ]; then exit 0; fi` 早退

**Given** 手工验证
**When** 本地 `xcodebuild archive` 后跑脚本
**Then** 输出 "✓ No WatchConnectivity symbols found in release binary"

**FRs**：（本 Story 无直接 FR · AR6.4 · O4 compile-time 硬验证）

---

### Story 9.3: Watch 配对真机流 + HealthKit / SIWA 真机 smoke（FR2 E9 部分 + FR4 真机）

As a **QA engineer running pre-MVP smoke test**,
I want **真机 checklist 验证 iPhone + Watch 配对流程 · HealthKit 授权 · SIWA 双端关联**,
So that **模拟器无法覆盖的真机路径（Watch 配对协议 / 真 HealthKit 授权 / SIWA Apple ID 真 flow）在 MVP 发布前人工走通**.

**Acceptance Criteria:**

**Given** 真机环境准备
**When** 创建 `ios/docs/e9-smoke-checklist.md`
**Then** 文档含测试环境要求：
- iPhone（iOS 17+）· 建议 2 台（分别测 with-Watch / observer-only）
- Apple Watch（watchOS 10+）· 与上述 iPhone 1 配对
- 真实 Apple ID（不能用 Xcode-created · 需测 SIWA full flow）
- TestFlight 构建（Epic 1-7 全绿后 archive + 上传）

**Given** Watch 配对 smoke checklist
**When** QA 执行
**Then** 按以下步骤逐项 ✓：
- [ ] iPhone 从 Story 2.1 叙事卡片走到 2.3 Watch 模式选择屏
- [ ] 选 "我有 Apple Watch" · 点击 "去 App Store 装" → 跳 App Store 成功
- [ ] 安装 CatWatch app（Watch target · 假设已独立交付）
- [ ] Watch 独立完成 SIWA 登录（用同一 Apple ID）
- [ ] 在 iPhone 端 ∩ Watch 端 · server 自动关联（FR2 核心验证）· iPhone 显示"你的 Watch 配对好了"
- [ ] 离线后 / 重连后关联状态保持

**Given** HealthKit 真机授权 checklist
**When** QA 执行
**Then**：
- [ ] 首次进 Inventory tab 弹 `NSHealthShareUsageDescription` 原生弹窗 · 文案正确
- [ ] 允许 → 走到 Story 4.2 步数展示 · 显示真实 HealthKit 步数（非模拟器假数据）
- [ ] 拒绝 → 走到 Story 4.1 降级路径 · GentleFailView "去开启"按钮跳真系统设置
- [ ] 从设置返回后 · HealthKitAuthorization.refresh() 正确检测到状态变化

**Given** SIWA 真机 smoke checklist
**When** QA 执行
**Then**：
- [ ] Story 2.2 SIWA 按钮 → iOS 原生 SIWA UI 弹出
- [ ] 允许 → identityToken 正确解析 · server 返回 JWT · Keychain 写入
- [ ] 拒绝 / 取消 → GentleFailView "账号没连上"
- [ ] 重新登录 → Keychain 读回 JWT · 状态恢复

**Given** 每个 checklist 项完成
**When** QA 填写
**Then** 在 Markdown 文件内勾选 [x] · 附简要观察（如"✓ 关联延迟 2s"/ "✗ 关联失败 server 500 · ticket #1234"）

**Given** 任一关键项失败
**When** 记录 issue
**Then** 创建 GitHub issue · label `e9-blocker` · 路由到对应 Epic / team（iPhone / server / watchOS）· MVP ship 阻塞

**Given** Smoke 周期
**When** 每个 RC（release candidate）构建
**Then** 重新跑 checklist（预计 2 人时 · 含 2 台 iPhone + 1 Watch 配对）· 结果附 release notes

**FRs**：FR2（Watch 配对真机 · iPhone 侧引导部分已在 Epic 2 · 真机验证在此）+ FR4 真机授权 smoke

---

### Story 9.4: Watch-only FR 真机 smoke（FR23/24/35c/42/44/45a/45b/46 · 一站式感官验证）

As a **QA engineer + product owner validating sensory-level innovation hypotheses**,
I want **真机执行 Watch-only FR 的感官验证 · 人类确认 Spine 动画 / haptic 参数 / 跨端 fan-out 的真实体感**,
So that **"Watch 主角反转"和"触觉广播"两大创新假设在真机上被人类辨识 · 非自动化能覆盖**.

**Acceptance Criteria:**

**Given** `ios/docs/e9-smoke-checklist.md` 增补 Watch-only FR 节
**When** 按 FR 序号逐项验证
**Then** checklist 条目（每项需人类观察 + 视频录证据 + 打勾 ✓/✗）：

- [ ] **FR23 同屏渲染 2-4 人**：2 人房间 → Watch 同屏可见双猫 · 4 人房间 → 4 只猫不重叠 / 不穿模 · 60fps 无明显卡顿（前台）
- [ ] **FR24 环绕跑酷特效**：2 人同时走路（真实走路 · 步频达阈值）→ Watch 触发环绕跑酷视觉 · **server 判定下发（Watch 禁本地计时）**· 验证方法：1 人单独走路不触发 · 2 人同时走路触发
- [ ] **FR35c 开箱动画**：Epic 4 Story 4.5 盲盒解锁到达 Watch → Watch 开箱 + 皮肤揭晓动画完整 · 1.5s 左右 · 视觉 smooth
- [ ] **FR42 装扮 payoff 渲染**：iPhone Dressup 选中新皮肤 → Watch 端 1-2s 内换皮肤完成 · 渲染清晰
- [ ] **FR44 猫 walk/sleep/idle 动作同步**：用户真实走路 → Watch 猫 walking 动画 · 用户停下 5min → 猫 idle · 用户手腕放平 / 睡觉 → 猫 sleeping（依 watchOS CMMotionActivity）
- [ ] **FR45a 久坐召唤 haptic**：2h 无活动 → Watch 触发 haptic 召唤 · 测试方法：用 server 端 debug 工具 fast-forward idle counter 到 2h 阈值
- [ ] **FR45b haptic 辨识度**：**5 位测试者分别盲测**久坐召唤 vs 盲盒掉落 haptic（注：盲盒掉落已作废 · 改 vs 好友邀请 haptic）· ≥ 4/5 人可辨识差异 → 通过
- [ ] **FR46 欢迎动作**：久坐 2h + haptic → 用户起身走 30s → Watch 猫做欢迎动作（尾巴高抬 / 转头看镜头 · UX 规范定义）· 1s 内反馈

**Given** 测试记录
**When** 每项勾 ✓ 或 ✗
**Then** 附录视频 link（Figma / Notion / 内部 share）· 音频描述（FR45b）· 5 人盲测结果填表（FR45b 可 auditable）

**Given** FR45b 人类感知差异要求
**When** 盲测不通过（< 4/5）
**Then** issue 路由到 watch team · 调整 `UIImpactFeedbackGenerator` 参数或改为 `UINotificationFeedbackGenerator` 不同 style · 重跑

**Given** 多个 FR 依赖同一 Spine 动画资产
**When** 资产缺失或渲染失败
**Then** 整 checklist 暂停 · issue 路由 Spine animator · 资产交付后重跑

**Given** MVP ship 前最后一次 smoke
**When** 全部 ✓
**Then** Epic 9 部分输出：`e9-smoke-report-<date>.md` 归档 · release notes 附链

**FRs**：FR23 + FR24 + FR35c + FR42（Watch 渲染）+ FR44 + FR45a + FR45b + FR46（全部 Watch-only E9 验证）

---

### Story 9.5: Pre-MVP alpha tester 招募 + Legal sync + Store 合规最终检查

As a **PM + legal + ops team running Pre-MVP operational gates**,
I want **完成 alpha tester 招募 / Legal review / App Store 合规最终清单**,
So that **MVP ship 的非技术门槛（人 + 合规 + 法律）全部通过**.

**Acceptance Criteria:**

**Given** Pre-MVP alpha tester 招募目标（来自 memory · `project-memory` G11）
**When** PM / ops 执行
**Then** 完成以下：
- [ ] 制定 alpha tester 画像（参考 J1-J7 用户 · 有 Watch / 无 Watch 混合 · 年龄分布 · 有无健身习惯）
- [ ] 招募 ≥ 30 人（含 ≥ 20 有 Watch · 保证 Watch-主角反转假设可验证）
- [ ] TestFlight 分发（iOS 端 MVP · 附 feedback 渠道 · 如 Apple Feedback Assistant 或问卷）
- [ ] 预计 ≥ 2 周 alpha 测试窗口 · 收 ≥ 50 条反馈

**Given** Legal sync（G10）
**When** 法务 review
**Then** 完成：
- [ ] 隐私政策 `https://cat.app/privacy` 法务确认（HealthKit 用途 / 数据保留期 / 第三方披露）
- [ ] 使用条款 `https://cat.app/terms` 法务确认（用户行为规范 / 账号终止 / 争议解决）
- [ ] Loot-box 合规（中国《盲盒经营活动规范指引》· 海外 loot-box 监管 · 法务签字）
- [ ] GDPR Article 17 删除权 + Article 20 数据可携带权 · 法务确认 S-SRV-12 实装符合
- [ ] COPPA（若 age rating 改 4+ 或 9+ · 须加额外儿童保护 · 当前 12+ 不需要）· 法务确认 12+ rating 足够

**Given** App Store Connect 提交前 final checklist
**When** 团队逐项核对
**Then** 完成 `ios/docs/release-checklist.md`（Story 7.7 创建）全部 ✓：
- [ ] App Store Connect 隐私问卷已填（HealthKit + 好友图曝光 + 分析数据）
- [ ] Age Rating 12+ 已设定
- [ ] Sign in with Apple · 唯一登录方式声明
- [ ] In-App Purchases · 无
- [ ] Export Compliance ENCRYPTION：wss TLS · 属 standard · 用 category 5D992.c 豁免
- [ ] App Review Information：含 demo account + SIWA 绕过（test account）+ 演示 video URL
- [ ] Screenshots（iPhone 15 / 15 Pro Max 必需 · 5 张起）· 翻译中文简体为默认

**Given** 任一合规项 blocker
**When** 无法在 MVP window 解决
**Then** PM 评估：降级 MVP scope（如暂不上架某区域）/ 延后 ship / 临时合规补丁 · 与 stakeholder 对齐

**Given** 所有 checklist ✓
**When** 准备 submit
**Then** Product owner 签字 · release tag 创建 · archive 上传 App Store Connect · 进入"Waiting for Review"

**FRs**：（本 Story 无直接 FR · Pre-MVP operational gates · 来自 memory G10/G11）

---

### Story 9.6: Final release ship gate · 整合所有 Epic 输出（MVP 发布最终 gate）

As a **product owner + tech lead running final ship decision**,
I want **整合 Epic 1-7 + Epic 9 Story 9.1-9.5 的输出 · 最后一道 gate 确认 MVP 可 ship**,
So that **所有非技术 / 技术 / 合规 / 感官 gate 都过了才按发射钮**.

**Acceptance Criteria:**

**Given** Epic 1-7 所有 Story 绿（`xcodebuild test` + CI 全绿 · 每 Epic 收尾 Story fail-closed ADR 整合已通过）
**When** 检查 tracking
**Then** 状态表：
```
Epic 1 Foundation (20 Story · Stage A+B+C) ✓
Epic 2 Onboarding (5 Story) ✓
Epic 3 我的猫 & 表情 (4 Story) ✓
Epic 4 HealthKit × 盲盒 × 仓库 (9 Story · 含收尾 4.9) ✓
Epic 5 合成 & 装扮 (4 Story) ✓
Epic 6 好友 & 房间 (9 Story · 含收尾 6.9) ✓
Epic 7 账户 & 合规 & 可观测 (7 Story · 含收尾 7.7) ✓
```
**And** 总 Story count: 58（Epic 1 20 + E2 5 + E3 4 + E4 9 + E5 4 + E6 9 + E7 7）· 实际开发进度写入 sprint-status

**Given** Epic 9 Story 9.1-9.5 已完成
**When** 核对
**Then**：
- Story 9.1 GitHub Actions CI ✓（持续运行）
- Story 9.2 Binary linker symbol check ✓（release build 跑）
- Story 9.3 Watch 配对 + HealthKit + SIWA 真机 smoke ✓
- Story 9.4 Watch-only FR 真机 smoke ✓（含 FR45b 5 人盲测通过）
- Story 9.5 Alpha tester + Legal + Store 合规 ✓

**Given** 3 创新假设验证（Experience MVP 核心目标）
**When** Alpha tester 反馈收集后评估
**Then** 3 假设各自验证 ✓/✗：
- **假设 1 · 负反馈温柔门槛**：alpha tester 是否认为久坐召唤是"温柔提醒"而非"骚扰"？（问卷 ≥ 70% ✓ → 通过）
- **假设 2 · 触觉广播**：alpha tester 是否感动于"朋友的猫的表情传到手腕"？（问卷 ≥ 60% ✓ · 且 fire-and-forget 对称性无破坏）
- **假设 3 · Watch-主角反转**：alpha tester 是否从 Watch 而非 iPhone 主要体验产品？（行为数据 Watch-session/iPhone-session ratio ≥ 2:1 → 通过）

**Given** MVP 成功判据（PRD §Success Criteria · 引用北极星指标 + 3 Aha 时刻链）
**When** Dashboard 数据拉取
**Then** 检查：
- 首次挂机掉盒 12h 内 ≥ 85%（v2 修订后改为"首次被发现 24h 内 ≥ 85% 或保持 48h 首次接回家 ≥ 85%"）
- 崩溃率 ≤ 0.2%（NFR14）
- WS 重连 10s 内 ≥ 95%（NFR15）
- （其他 Success Criteria 条目 ...）

**Given** Known tech debt（tech-debt-registry.md 9 条）
**When** 检查触发条件
**Then** 每条 TD 标记状态：
- `not-triggered`（未触发 · 不影响 MVP）
- `triggered-but-deferred`（已触发 · 记录到 Growth backlog）
- `blocker`（必须在 MVP 前修 · 若存在则 ship 阻塞）

**Given** Cross-repo gate
**When** 检查跨仓状态
**Then**：
- S-SRV-1..S-SRV-18 全部交付 ✓ 或明确降级方案（如 S-SRV-15 软 gate 已由 Epic 2+7 闭环）
- CD-drift register 所有 open drift 有 owner + issue

**Given** Product owner + tech lead 共同评审
**When** 所有 ✓
**Then** ship 决策会议（1h）· 决议 log 到 `ios/docs/release-<date>-decision.md`
**And** 按"发射钮"：提交 App Store Review

**Given** App Store Review 通过
**When** 用户可下载
**Then** MVP 发布 · 启动 Post-MVP 度量观察期（2 周 · 决定 Growth 进 G1-G4 中的哪一个）

**FRs**：（本 Story 无直接 FR · Final release gate 整合）

---



