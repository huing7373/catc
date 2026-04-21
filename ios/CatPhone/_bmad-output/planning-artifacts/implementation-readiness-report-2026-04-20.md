---
stepsCompleted: [step-01-document-discovery, step-02-prd-analysis, step-03-epic-coverage-validation-blocked]
filesIncluded:
  - _bmad-output/planning-artifacts/prd.md
  - _bmad-output/planning-artifacts/ux-design-specification.md
  - _bmad-output/planning-artifacts/server-handoff-ux-step10-2026-04-20.md
  - _bmad-output/project-context.md
filesMissing:
  - architecture (none found in planning-artifacts)
  - epics (none found in planning-artifacts)
  - stories (none found in planning-artifacts)
---

# Implementation Readiness Assessment Report

**Date:** 2026-04-20
**Project:** CatPhone (iOS)

## Step 1 — Document Discovery

### PRD
- **Whole:** `_bmad-output/planning-artifacts/prd.md` (78 KB, 修改于 04-20 16:49)
- **Sharded:** none

### Architecture
- **Whole:** _未发现_
- **Sharded:** _未发现_
- 备注：根目录 `CLAUDE.md` 引用的 `docs/backend-architecture-guide.md` 属于 **server** repo（Go 后端），不是 iOS 端架构文档。iOS 端在 `_bmad-output/planning-artifacts/` 下没有独立的 architecture 文档。

### Epics & Stories
- **Whole:** _未发现_
- **Sharded:** _未发现_
- 备注：当前 `_bmad-output/` 没有 epics list 文件，也没有 stories 文件。

### UX Design
- **Whole:** `_bmad-output/planning-artifacts/ux-design-specification.md` (80 KB, 修改于 04-20 15:58)
- **辅助:** `_bmad-output/planning-artifacts/server-handoff-ux-step10-2026-04-20.md` (12 KB, 04-20 16:46) — Step 10 评审产出的跨仓 server 契约 handoff
- **Sharded:** none

### 其它
- `_bmad-output/project-context.md` — 项目上下文 / AI 规则

---

## Step 2 — PRD Analysis

PRD 文档已完整读取（1056 行 / 80 KB / v2 修订版），按 BMad PRD 模板组织：Vision → Success Criteria → Scope → External References → Capability ID Index → User Journeys → Cross-Device Messaging Contract → Innovation → Mobile App Specific → Project Scoping → FR → NFR → Server-Driven Stories → v2 修订注记。

### Functional Requirements（共 63 条 / 拆分后 67 条 sub-ID）

#### §1 账号 & Onboarding（C-ONB-* + C-UX-01/02 + C-OPS-06）
- **FR1**：User 可通过 Sign in with Apple 登录 `[U, I]`
- **FR2**：有 Apple Watch 的 User 可在 onboarding 完成 Watch 配对 `[I (FakeWatchTransport), E9]` ⚠️ **v2 修订 5 影响**：原 WC probing 路径改为 "iPhone 引导用户去装 CatWatch app，Watch 端独立 SIWA + server 关联"
- **FR3**：无 Apple Watch 的 User 可选观察者模式继续 onboarding，不被阻断 `[U, I]`
- **FR4**：User 可授权 HealthKit 读取步数 `[I (mock HealthKit), E9]`
- **FR5**：User 可拒绝 HealthKit 授权，app 仍可进入（功能降级，见 FR50）`[U, I]`
- **FR6**：User 在 SIWA 登录前可浏览 3 屏叙事 Onboarding 卡片（含 Slogan）`[U, I]` → `C-UX-01`
- **FR7**：User 首次完成配对后进入撕包装首开仪式（盒子震动 → 猫跳出 → 命名）`[I, E9]` → `C-UX-02`
- **FR8**：User 可请求删除账号（SIWA + GDPR 合规，联动 `S-SRV-12`）`[I, E9]` → `C-OPS-06`
- **FR9**：User 可导出个人数据（GDPR right to data portability，联动 `S-SRV-12`）`[I]`
- **FR10**：User 可主动 sign out 并清除本地 token（Keychain 清理）`[U, I]`

#### §2 好友（C-SOC-03 + C-UX-05）
- **FR11**：User 可发送好友请求 `[U, I]`
- **FR12**：User 可接受 / 拒绝收到的好友请求 `[U, I]`
- **FR13**：User 可查看好友列表并管理关系 `[U, I]`
- **FR14**：User 可屏蔽 / 移除一个好友 `[U, I]`
- **FR15**：User 可静音某位好友的表情广播 `[U, I]` → `C-UX-05`
- **FR16**：User 可举报某位好友或其表情内容 `[I, E9]` → `C-UX-05`

#### §3 房间 + 社交广播（C-ROOM-* + C-SOC-01/02 + C-ME-01）
- **FR17a/b/c**：User 可创建房间 / 加入离开房间 / 房主转让解散（Sally 体面退房）`[U, I]`
- **FR18**：User 可通过邀请链接 / 二维码邀请好友入房 `[I, E9]`
- **FR19**：邀请链接含失败分支：过期 / 已是好友 / 已在房间 / 房间满 / 自己邀自己 `[U, I]`
- **FR20**：User 可通过邀请链接加入房间 `[I]`
- **FR21**：iPhone 端 User 可查看房间成员名单（姓名 / 状态 / 一句话叙事文案）`[U, I]`
- **FR22**：叙事文案合成引擎存在规则表（trigger → template_id）供 AC 精确断言 `[U]`
- **FR23**：Watch User 可在房间内看到其他成员的猫同屏渲染（2–4 人）`[I (fake assets), E9]`
- **FR24**：房间 2+ 成员同时走路 → Watch 端环绕跑酷特效（**server 判定下发**，Watch 禁本地计时触发）`[U (fake server events), I, E9]`
- **FR25**：观察者隐私 gate——只见 `current_activity` + `minutes_since_last_state_change` `[U, I]`
- **FR26**：User 可点击自己的猫弹出表情选单 `[U, I]`
- **FR27**：User 选表情后，自己猫旁浮 emoji（本地立即呈现）`[I, E9]`
- **FR28**：User 在房间内发表情 → server 广播；对端 Watch 轻震 + emoji；iPhone 观察者走 push `[U, I, E9]`
- **FR29**：User 不在房间时发表情仅本地呈现，不发 fan-out（仍走 server 去重落日志）`[U, I]`
- **FR30**：表情广播为 fire-and-forget——发送方无"已读/已送达"UI 状态 `[U]`

#### §4 步数（C-STEPS-*）
- **FR31**：System 从 HealthKit 本地读取步数用于展示 `[I (mock HealthKit), E9]`
- **FR32**：System 使用 server 权威步数判定盲盒解锁（iPhone 本地不做解锁判定）`[U, I]`（遵 `CLAUDE-21.4`）
- **FR33**：User 可在 iPhone 查看步数历史图表（联动 `S-SRV-9`）`[U, I]`
- **FR34**：WS 断线时，盲盒解锁 UI 显示"已达成·等待确认"占位 `[U, I]`

#### §5 经济：盲盒 + 合成 + 装扮（C-BOX-* + C-SKIN-* + C-DRESS-*）
- **FR35a**：System 在挂机满 30 min 后掉落盲盒（server 权威 + `ClockProtocol`）`[U (mock clock), I, E9]` ⚠️ **v2 修订 1+3 影响**：改为"无通知 / 入待领取队列 / 用户主动发现"
- **FR35b**：User 可消耗 1000 步打开已掉落的盲盒 `[U, I]`
- **FR35c**：Watch User 解锁盲盒时看到开箱动画 + 皮肤揭晓 `[I, E9 (视觉)]`
- **FR36**：iPhone Push 不剧透具体皮肤名 `[U]` ⚠️ **v2 修订 1**：盲盒掉落改为完全无通知
- **FR37**：System 为皮肤标注颜色分级（白 < 灰 < 蓝 < 绿 < 紫 < 橙）`[U]`
- **FR38**：盲盒开出重复皮肤时自动归为合成材料（文案"材料入库"）`[U, I]`
- **FR39**：User 可在 iPhone 仓库查看全部皮肤 + 合成材料计数 `[U, I]`
- **FR40**：User 可触发合成（联动 `S-SRV-10`，含中途断网 / 材料不足 / 并发幂等）`[U, I]`
- **FR41**：User 可在 iPhone 大屏选择装扮 `[U, I]`
- **FR42**：装扮 payoff 由 Watch 端渲染 `[I, E9]`
- **FR43**：System 跨天 / 跨时区正确重置挂机计时基准（`ClockProtocol` 测试）`[U]`

#### §6 陪伴反馈：久坐召唤（C-IDLE-01..02，**无情绪机**）
- **FR44**：System 在 Watch 端维护猫动作同步（跟随物理状态：走路/睡觉/挂机；**无情绪 layer**）`[I, E9]`
- **FR45a**：User 无活动 2h 后，Watch 主动 haptic 召唤（参数异于盲盒通知）`[U]`
- **FR45b**：真机上人类可辨识久坐召唤与盲盒震感差异 `[E9]`
- **FR46**：User 起身走动 30s 后，Watch 端猫做欢迎动作（纯交互触发）`[I, E9]`
- ❌ 原 FR37 情绪状态机 / 原 FR40 iPhone 情绪文案 **已删除**（Round 5 移除情绪机）

#### §7 容错（C-REC-*）
- **FR47**：WS 断线后 System 重连并按 `last_ack_seq` 补发遗漏事件 `[U (fake WS), I]`
- **FR48**：Server 待推事件 TTL = 24h；超时 GC 后 fail-closed 提示"部分事件已过期，请刷新"`[U, I]`
- **FR49**：`/v1/platform/ws-registry` 启动失败时屏蔽主功能 + "网络异常，请稍后重试"`[U, I]`
- **FR50**：HealthKit 授权被拒时，步数 UI 回退 "—" + 引导条 `[U, I]`

#### §8 UX 生存层（C-UX-03..04）
- **FR51**：全局空状态——仓库空 / 好友空 / 房间无广播 各自 CTA `[U, I]` → `C-UX-03`
- **FR52**：User 可编辑昵称 / 头像 / 个性签名 `[U, I]` → `C-UX-04`
- **FR53**：User 可在"设置"入口查看隐私政策 / 使用条款 / 账号信息 `[U]`

#### §9 跨端消息 + 推送 + 隐私 + 可观测性（C-SYNC-* + C-OPS-*）
- **FR54**：所有跨端消息带 `client_msg_id`（UUID v4）供 server 60s 去重 `[U]`
- **FR55**：User 可分频道开/关 push（盲盒 / 好友 / 表情）`[U, I]` ⚠️ **v2 修订 1 影响**：盲盒频道无通知，开关项是否保留待定
- **FR56**：System 通过 `wss://`（TLS）加密 WS；`ws://` 仅 debug build 豁免 `[U, E9]`
- **FR57**：System 将 JWT / refresh token 存 Keychain `[U]`
- **FR58**：System 不上传 raw step count 至 server `[U, I]`
- **FR59**：System 带崩溃上报（MVP：Xcode Organizer；Growth：Crashlytics/Sentry）`[I, E9]` → `C-OPS-01`
- **FR60**：System 启动时做版本检查（MVP：原生弹窗；Growth：in-app 强更）`[U, I]` → `C-OPS-02`
- **FR61**：System 带远程配置 / kill switch（MVP：硬编码；Growth：远程配置平台）`[U, I]` → `C-OPS-03`
- **FR62**：System 以 `Log.*` facade 做结构化日志上报 `[U]` → `C-OPS-04`
- **FR63**：App Bundle 带 `PrivacyInfo.xcprivacy` 隐私清单 `[U, E9]` → `C-OPS-05`

**Total FRs：63 条（含 FR17a/b/c, FR35a/b/c, FR45a/b 拆分 → 67 个 sub-ID）**

### Non-Functional Requirements（共 6 类 ≈ 34 项）

#### Performance（5）
- UI 响应：点猫弹表情选单 < 200ms；tab 切换 < 100ms（iPhone 14/15/16 基线）
- 冷启动：App 首次启动到首屏可交互 < 2s
- Spine 动画：Watch 前台 60fps / 后台 15fps；帧率不达自动降级
- HealthKit 查询缓存：按天聚合缓存 15min，禁秒级重查
- 列表性能：仓库皮肤数 > 200 用 `LazyVStack / List` with stable `id:`

#### Security（8）
- 传输加密：WS `wss://` + ATS 合规（FR56）
- Token 存储：JWT / refresh token 存 Keychain（FR57）
- 隐私最小化：Server 不收 raw step count（FR58）
- 观察者隐私 gate：`pet.state.broadcast` 字段裁剪（FR25）
- SIWA nonce：请求前随机生成，回调校验，防 replay
- 敏感日志：release build 禁明文 token / email；ID 可打，token 需 hash
- 设备标识：`identifierForVendor`，禁 IDFA
- Deep link：严格校验 scheme + 参数；禁直接执行 URL 参数

#### Reliability（6）
- 崩溃率 ≤ 0.2%（Crashlytics 口径）
- WS 重连：网络切换后 10s 内 ≥ 95% 成功率
- 事件 TTL：server 待推事件 TTL = 24h（FR48）
- 启动 fail-closed：`/v1/platform/ws-registry` 失败屏蔽主功能（FR49）
- 数据一致性：盲盒解锁 server 权威 + `client_msg_id` 60s 去重
- 离线降级：三档离线策略文档化（完全离线 / WS 断 / 启动失败）

#### Accessibility（5）
- VoiceOver：全 interactive UI 元素带无障碍标签
- Dynamic Type：文字尺寸支持系统动态字体
- 色盲友好：皮肤颜色分级必须配合形状或文字标签（C-SKIN-01 不能仅靠颜色）
- Haptic 作为补充而非替代：所有触觉信号同时有视觉信号兜底
- Onboarding 可跳过：3 屏叙事卡片（FR6）和撕包装仪式（FR7）允许 skip

#### Scalability（4）
- MVP 装机量预期：内测 100 人 → TestFlight 1 万 → 正式版 10 万
- 客户端并发约束：单用户最多同时在 1 个房间；单房间 max 4 人；MVP 好友数上限 50
- WS 负载：iPhone / Watch 任一端在线都使用同一 session（server 会话接管策略）
- 冷启动网络：仅 `/v1/platform/ws-registry`（必）+ 可选 health check；不做全量 state 拉取

#### Integration（6）
- HealthKit（读 only）：`HKStatisticsCollectionQuery` 按天聚合；增量走 `HKObserverQuery + HKAnchoredObjectQuery`；禁高频 `HKSampleQuery`
- WatchConnectivity（抽象 `WatchTransport`）：三档通道 `updateApplicationContext` / `transferUserInfo` / `sendMessage`；禁 `transferFile` 走 JSON ⚠️ **v2 修订 5**：架构哲学升级为"Server 为主，WC 为辅"，MVP 不使用 WC 做核心交互
- APNs：标准 iOS 推送，分频道开关（FR55）
- SpriteKit + Spine：`esoteric-software/spine-runtimes` SwiftPM 锁版本；仅 Watch 侧密集使用
- SIWA：标准接入；未来加第三方 OAuth 时 SIWA 仍需并列（App Store 4.8）
- 跨端契约锚点：OpenAPI 0.14.0-epic0 + WS registry apiVersion v1；漂移以 server 为准

**Total NFRs：≈ 34 项（不重复计算 FR 已覆盖项）**

### Additional Requirements / Constraints

#### Capability ID Index（13 类，**FR/AC 必须 cite 这些 ID**）
- `C-ONB-01..03` Onboarding
- `C-STEPS-01..03` 步数（含 SSoT/server 权威/漂移文案三层）
- `C-BOX-01..05` 盲盒
- `C-SKIN-01..02` 皮肤分级 + 合成
- `C-ROOM-01..05` 房间
- `C-SOC-01..03` 社交 / 表情
- `C-ME-01` 我的猫入口
- `C-DRESS-01..02` 装扮
- `C-IDLE-01..02` 挂机 + 久坐召唤（C-IDLE-03 已删除）
- `C-REC-01..03` 容错
- `C-SYNC-01..02` 跨端同步
- `C-UX-01..05` UX 生存层（Round 5 新增）
- `C-OPS-01..06` Dev 可观测性 + 合规

#### Cross-Device Messaging Contract（binding）
- Envelope 格式继承 `server/internal/ws/envelope.go`
- Client `client_msg_id`（UUID v4）+ server 60s 去重窗口
- ACK 双档：事务性（盲盒/装扮/合成/好友）vs fire-and-forget（表情广播）
- Seq 单调递增 + 重连按 `last_ack_seq` 回放
- TTL = 24h；超时 GC + 落审计日志 + fail-closed
- 隐私 Gate：观察者只见物理动作态，禁见步数/心率/health_*
- Fail-Closed 总则：未知 msg type / registry 漂移 → 强制 UI 提示
- Idle Timer 聚合：server 权威 `max(iPhone_last_active, Watch_last_active)`

#### Server-Driven Stories（共 18 条 → server PRD 跨仓 backlog）
- **S-SRV-1..14**：v1 已识别（pet.state / pet.emote / room.effect / TTL / 合成系统 / 久坐 / 盲盒概率披露 / box.drop / steps history / craft / cat.tap / 账户删除 / Idle Timer 聚合 / 解锁双写）
- **S-SRV-15..18**（v2 · UX Step 10 Party Mode 新增）：
  - S-SRV-15：`user_milestones` collection + API（替代 UserDefaults）
  - S-SRV-16：`box.state` 新增 `unlocked_pending_reveal` 态
  - S-SRV-17：取消 `emote.delivered` 发送者 ack 推送（fire-and-forget 对称性硬约束）
  - S-SRV-18：所有 fail 节点 Prometheus metric 打点（含 8 项 metric 名）

#### Mobile App Specific Requirements
- Native iOS（Swift 5.9+ / SwiftUI / iOS 17.0+ 硬要求）
- Swift 6 严格并发 day 1（`-strict-concurrency=complete`）
- XcodeGen 驱动（`project.yml` = SSoT）
- 测试自包含（`CLAUDE-21.7`）：`xcodebuild test` 一命令绿
- 真机 / Watch 配对 / Spine 真机校验归 Epic 9（`CLAUDE-21.6`）
- Onboarding 测试旁路：`AppEnvironment.pairingMode`（`TESTING_HOOKS` flag）+ 双 CI gate
- 必需 Info.plist keys：`NSHealthShareUsageDescription` + `NSCameraUsageDescription`

### PRD v2 修订（5 处 + 5 项次要影响，2026-04-20）

| # | 修订对象 | v1 → v2 关键变化 |
|---|---|---|
| 1 | Push Notification Strategy | 盲盒掉落：轻震+push → **完全无通知**（旅行青蛙式纯发现制） |
| 2 | J1-S4 叙事 | "下午 2:30 手表震一下" → "下午 3:10 抬腕看时间发现盒子" |
| 3 | C-BOX-01 | "掉落事件" → "记入待领取队列 + 无主动推送 + 用户主动 GET 或自然打开 app 发现" |
| 4 | Success Criteria · 首次掉盒 | "12h 内 ≥ 85%（被掉落）" → "24h 内 ≥ 85%（被发现）" |
| 5 | WatchConnectivity 使用 | WC 主通道 → **降级为 fast-path 优化层**（哲学 B · Server 为主，WC 为辅）；MVP 不使用 WC 做核心交互；FR2 改为 Watch 端独立 SIWA |

### PRD Completeness Assessment

#### 优点（高完整度）
- ✅ **能力契约纪律完整**：13 类 Capability ID + 67 个 sub-FR，明确"FR/AC 只能 cite C-* / FR-* / Jn-Sn，禁止 cite 散文段落"
- ✅ **测试可执行性高**：每条 FR 带 `[U/I/E9]` 测试标签（Round 5 Murat），未标签 = story review 退回
- ✅ **追溯矩阵齐备**：Journey ↔ Capability ↔ FR 三层映射表
- ✅ **跨端契约具象**：Envelope / msg_id / ACK 双档 / TTL / Seq / 隐私 gate / Idle Timer 聚合 全部展开
- ✅ **Server-Driven Stories 显式**：S-SRV-1..18 列出，便于跨仓 sync
- ✅ **创新假设可证伪**：每条 innovation 有 TRIZ 自评 + 验证指标 + 失败信号 + 撤退预案
- ✅ **§21 工程纪律已挂钩**：`CLAUDE-21.3/4/6/7/8` 散布在 FR / Cross-Device Contract / NFR

#### 风险（待 Epic / Story 层补强）
- ⚠️ **AC 层断言尚未展开**：Self-Validation 显式标注 "AC 层每条 FR 的具体断言脚本——待 Step 10+ NFR / Story 层做"
- ⚠️ **5 张 Loaded-gun 规则表未补**（Round 6 Amelia）：FR22 文案规则表 / FR28 emote 扇出上限 / FR40 合成材料表 / FR63 隐私清单字段 / FR35a 盲盒容错阈值
- ⚠️ **ClockProtocol / IdleTimerProtocol mock 契约**未细化（Round 6 Amelia 登记 to story 层）
- ⚠️ **Spine SwiftPM × Swift 6 严格并发兼容 Spike** 归 Epic 9，结论前不可领 Spine story
- ⚠️ **v2 修订未回填原文**：5 处 v2 修订仍是覆写注记形式，下游 story 写作时需主动 reconcile（不能只 cite 原 FR 文本）
- ⚠️ **Validation evidence debt**：未扫国内小游戏市场（抖音/微信小游戏），pre-MVP 原型测试未做

---

## Step 3 — Epic Coverage Validation 🚫 BLOCKED

### 🛑 关键阻塞缺口

**iOS 端没有 epics 和 stories 文档**。穷举搜索结果：

- `_bmad-output/planning-artifacts/` — 无 `*epic*.md` / `*stor*.md` 文件
- `_bmad-output/implementation-artifacts/` — 空目录
- `_bmad-output/test-artifacts/` — 空目录
- 无 `sprint-status.yaml`（BMad sprint planning 默认输出）

### 既有线索（需用户澄清）

1. **CLAUDE.md 提到 server 端有 Epic 0/1+**：根目录 `CLAUDE.md §21 / §22` 引用 `sprint-status.yaml` 和 "最新 epic retro"。git log 也显示 "Story 10.1 实现总结 + 客户端对接指南（手表+iPhone）" → **server 端**有 epic 流程在跑。
2. **iOS 端 epic 流程未启动**：本次扫描的 `ios/CatPhone/` 无任何 epic / story 文件，PRD 仅在 §Functional Requirements §5.MVP Strategy 列出 "Must-Have Capabilities"，但**没有把 67 个 sub-FR 拆分到 epic / story**。
3. **PRD 有 "下游待办" 占位**：PRD §Self-Validation 显式承认 "AC 层每条 FR 的具体断言脚本——待 Step 10+ NFR / Story 层做" → epic / story 是计划中、尚未生成的文档。

### 影响

| 维度 | 影响 |
|---|---|
| **FR 覆盖矩阵** | 无法生成（没有 epic 行可对照） |
| **Story 质量审查（Step 5）** | 无法生成（没有 story 可审） |
| **UX ↔ FR ↔ Story 对齐（Step 4）** | 部分可做（UX ↔ FR 可比较，但缺 story 验证锚点） |
| **Implementation readiness 总评** | **未达标**——按 BMad 实施准备定义，epic + story 拆分是 Phase 4 实施的硬前置 |

### 建议下一步路径（请选择）

**Path A（推荐）· 先生成 Epic + Story，再回来跑 readiness**
1. 调用 `/bmad-create-epics-and-stories` 把 67 个 sub-FR 拆成 epic/story
2. 期间触发 §21 工程纪律识别（双 gate / Empty Provider / fail-closed/open / AC 早启 / tools / Spike / 测试自包含 / 语义正确性思考题）
3. 完成后再调用 `/bmad-check-implementation-readiness` 重跑（或继续 Step 4 onwards）

**Path B · 仅基于 PRD + UX 做有限 readiness 评估**
- 跳过 Step 3 + Step 5（依赖 epic/story 的检查）
- 仅做 Step 4 UX ↔ PRD 对齐（已有 UX 规范 80 KB）
- 输出"PRD + UX 维度的 readiness 报告"，注明 epic/story 维度未评估

**Path C · 终止 readiness 流程**
- 当前 readiness 报告作为"Phase 3 → Phase 4 过渡前的 gap 清单"归档
- 后续 BMad workflow（create-epics-and-stories）启动时使用本报告作为输入


