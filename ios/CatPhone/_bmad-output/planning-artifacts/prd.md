---
stepsCompleted:
  - step-01-init
  - step-02-discovery
  - step-02b-vision
  - step-02c-executive-summary
  - step-03-success
  - step-04-journeys
  - step-05-domain-skipped
  - step-06-innovation
  - step-07-project-type
  - step-08-scoping
  - step-09-functional
  - step-10-nonfunctional
  - step-11-polish
  - step-12-complete
completedAt: 2026-04-20
workflowStatus: complete
version: 2
lastRevision: 2026-04-20
revisionNotes:
  - "v2 · 2026-04-20 · UX Step 10 Party Mode 5 处修订已回填至各章节原位置（FR / 表格 / Journey / Capability / NFR）；S-SRV-15..18 已列入 Server-Driven Stories"
  - "变更历史见 sprint-change-proposal-2026-04-20.md + git log；changelog 行见 Executive Summary 前"
inputDocuments:
  - /Users/zhuming/fork/catc/ios/CatPhone/_bmad-output/project-context.md
  - /Users/zhuming/fork/catc/CLAUDE.md
  - /Users/zhuming/fork/catc/docs/backend-architecture-guide.md
  - /Users/zhuming/fork/catc/docs/api/openapi.yaml
  - /Users/zhuming/fork/catc/docs/api/ws-message-registry.md
  - /Users/zhuming/fork/catc/docs/api/integration-mvp-client-guide.md
  - /Users/zhuming/fork/catc/server/_bmad-output/planning-artifacts/prd.md
  - /Users/zhuming/fork/catc/server/_bmad-output/planning-artifacts/epics.md
  - /Users/zhuming/fork/catc/server/_bmad-output/planning-artifacts/architecture.md
  - /Users/zhuming/fork/catc/裤衩猫.md
documentCounts:
  briefs: 0
  research: 0
  brainstorming: 0
  projectDocs: 10
workflowType: 'prd'
classification:
  projectType: mobile_app
  projectSubtype: companion-pet-watch-first-with-iphone-accessory
  domain: general
  domainTags:
    - companion-pet
    - fitness-driven
    - virtual-economy
    - light-social
  complexity: high
  complexityDrivers:
    - cross-device state sync (iPhone ↔ Watch ↔ server, three versioned contracts)
    - dual truth-source for steps (HealthKit for display / server authoritative for unlock)
    - shared WS envelope + strict Swift 6 concurrency day 1
    - iPhone observer-mode = read-only real-time (two-archway architectural complexity)
    - Spine/SpriteKit rendering layer
  projectContext: brownfield
productDecisions:
  positioning: "挂机 + 情感陪伴 + 轻社交"
  northStar: "最重点要做好：反馈"
  slogan: "你的猫在家等你走到它身边"
  sloganHistory: "v1 原文『你的猫在等你去接它回家』——Round 4 Caravaggio 评 7/10（动词『接』模糊），改为 v2 更具象"
  ownershipModel: "每人一只自己的猫（非共享）"
  socialLoop: "在线挂机 30 分钟解锁宝箱 → 消耗 1000 步打开宝箱 → 装扮自己的猫 → 进入房间被朋友观测（房间 = 异步观测层）"
  blindBoxFlow: "在线 30 min（挂机/戴表）掉落宝箱（待解锁态）→ 消耗 1000 步打开（"接回家"）→ 开出皮肤组件 → 进仓库 → 可装扮"
  blindBoxNote: "1000 步 ≈ 1/5 日健康目标（5000 步），每盒对齐"日健康配额 1/5"设计；裤衩猫.md 原文 200 步已作废"
  removedFeatures:
    - "日历签到（撕日历获取皮肤）：从 MVP / 产品中移除；皮肤获取路径收敛为单一盲盒链路"
  iPhoneRole: "Watch 主体验的门户 + 管理 + 深度内容 + 观察者降级端"
  noWatchPolicy: "iPhone-only 观察者模式：能安装、能看朋友猫状态、能点自己的猫发表情（与 Watch 用户权限对等，fire-and-forget），但不可领奖/解锁盲盒（server 权威判定依赖 Watch 数据链）"
  emoteMechanic: "点击自己的猫 → 弹可发表情选单 → 选 → 自己猫旁浮独立 emoji（非身体动作，实现简化 + 节省 Spine 资产）。在房间：server fan-out 给房间成员，他人 Watch 轻震 + 看见 emoji 浮在发起者猫旁。不在房间：仅本地呈现（自娱自乐 / 陪伴兜底）。iPhone 发起入口 = 账号 tab 首屏'我的猫'卡片（静态 avatar + 发表情按钮，无动画）。Watch / iPhone 对称。Fire-and-forget（发送方不等任何回执）。"
  skinRarityGrading: "颜色分级（候选：白 < 灰 < 蓝 < 绿 < 紫 < 橙）。盲盒初掉落为低级；5 个低级合成 1 个高级（比例候选，待经济平衡 epic）。重复掉落 UI 永远框定为'材料入库'，不写'抱歉重复'（防挫败文案）"
  idleFeedback: "久坐召唤 = 北极星'反馈'本体。**无情绪 layer**——猫只做动作同步（走路它跑 / 睡觉它睡 / 挂机它敲桌子，跟随 Watch 物理状态）。2h 无活动（纯 idle timer 触发）→ Watch 主动 haptic 召唤（震感不同于盲盒通知）+ 猫做召唤动作。起身 30s 后猫做欢迎动作（无盲盒奖励，无情绪恢复语义，仅交互闭环）。iPhone 端不展示情绪文案"
  emotionLayerRemoved: "Round 5 决定移除情绪机（原 idle/bored/hungry/happy 状态机 + extreme 离家出走扩展）。理由：情绪 layer 引入 Tamagotchi 式游戏化 guilt，与温柔北极星冲突；久坐召唤作为纯交互触发（idle timer → haptic → 起身动画）已足够承载反馈回路，无需情绪机包装。原 FR37/FR39/FR40 删除；C-IDLE-03 删除；C-IDLE-01 改为'猫动作同步'。"
  noWatchPolicyRationale: "Round 2 Victor 论点：强制 Watch 触 App Store 2.4.2/3.1.1 审核线 + 砍 85% TAM；观察者模式留门不留体验"
  roomTabIphone: "成员列表 + 在线/挂机状态 + 一句话叙事文案（如『张三的猫刚刚睡着了 · 2min 前』），无动画、无猫渲染、无同屏"
  iphoneMVPModules:
    - 账号（Sign in with Apple + Watch 配对可选但鼓励）
    - 好友（添加/管理/邀请）
    - 仓库（皮肤浏览，收集回顾）
    - 装扮（大屏编辑，换装 payoff 在 Watch）
    - 步数回顾（历史图表）
    - 房间（成员名单 + 叙事状态文案）
  acceptedTradeoffs:
    - "装扮编辑空间换乘：iPhone 编辑 → Watch 看 payoff，已知体验断裂但保留（用户决定）"
    - "仓库大屏浏览稀释 Watch 抬腕偶遇魔法：已知但保留（用户决定）"
    - "观察者模式 = iPhone 复杂度上升一档，换 TAM + 冷启可跑性"
  openQuestionsForVision:
    - "观察者 → 完整用户的转化节点（买表激励文案 / 时机）"
    - "盲盒概率披露文案（loot-box 合规最小面）"
    - "iPhone 推送分级策略（避免剧透 Watch 『抬表揭晓』的仪式感）"
    - "WatchTransport 抽象层（Murat）——Dev Notes 层面已有技术方案，PRD 仅需 AC 约束"
    - "步数漂移窗口的 UI 文案标准（Murat 提议『已达成·等待确认』）"
    - "Validation evidence debt（Round 4 Mary）：'市场无先例' claim 基于海外竞品推断（Pokémon Sleep / Walkr / Pikmin Bloom / Fitness+），未扫国内小游戏市场（抖音/微信小游戏），pre-MVP 原型测试未做。PRD 接受此声明风险，debt 登记，由后续 epic 补证据"
    - "Loaded-gun 规则表 5 张（Round 6 Amelia）——Story 前必补：(1) FR22 叙事文案规则表（trigger → template_id）；(2) FR28 盲盒掉盒容错阈值（重复挂机/跨天/重启计时基准）；(3) FR40 craft 材料消耗表 + 合成成功率配方；(4) FR63 PrivacyInfo.xcprivacy 字段清单（API reason + tracking domains）；(5) S-SRV-11 emote broadcast 扇出上限 + 节流配置"
    - "Spike owner 指派（Round 6 Amelia）：**Spine SwiftPM + Swift 6 严格并发 Sendable 兼容**归 Epic 9（真机 / 工具类），非业务 epic 领；Spike 结论前拒绝领 Spine 相关 story"
    - "ClockProtocol / IdleTimerProtocol mock 契约细化（Round 6 Amelia）：`advance(by:)` 语义 / timer 重入 / 跨 actor 线程模型——AC 层 Story 展开时定义"
vision:
  statement: "为久坐却拒绝被『自律』KPI 审判的年轻人造一只腕上的触觉猫——用挂机、盲盒、共享挂机仪式（同屏 + 震动）把朋友之间『不打扰的亲密』固化成一个设备、一只猫、一条触觉频道"
  whatMakesItSpecial:
    - "『负反馈调节』替代『正反馈刺激』——盲盒掉了得走 1000 步才能打开（『接猫回家』）"
    - "触觉社交（广播式身体语言）——点自己的猫 → 旁边浮 emoji → 房间里他人 Watch 同步轻震+看见；fire-and-forget 不等已读，完美契合『不打扰的亲密』；2026 companion-pet 赛道几乎无人深耕"
    - "Watch-first 反转配件关系——其他 companion-pet 把手表做镜像，裤衩猫反过来"
  coreInsight: "反馈 = 即时性 × 体感 × 在场性。Apple Watch 在这三维满分，而整个 companion-pet 赛道没人把它当首要载体"
  moat: "社交网络效应 + 共享挂机仪式（同屏 + 震动）——猫不只属于个人，也属于关系；共享的不是目标而是此刻（iPhone-only 观察者模式是护城河的燃料通道，不是退让）"
  whyNow2026:
    - "Apple Watch 触觉引擎已精细到可做『语言化震动』"
    - "后疫情 WFH 让『不打扰的亲密』成稀缺情绪商品"
    - "Bongo Cat / Duolingo 猫头鹰验证了陪伴型吉祥物的商业潜力"
  futureState:
    - "α：入眠前的最后一瞥——3000 组亲密关系（情侣/室友/异地密友）把裤衩猫当日常仪式"
    - "β：办公室/共享空间的隐秘仪式——同事猫状态像 Slack presence 但温柔十倍"
    - "γ：50+ 种震动表情形成只有彼此懂的手腕暗号——一种新亲密语言"
---

# Product Requirements Document - CatPhone

**Author:** Developer
**Date:** 2026-04-19

> **Changelog**: 2026-04-20 · v2 · 回填 UX Step 10 Party Mode 5 处修订 + S-SRV-15..18 至正文 FR / 表格 / Journey / Capability / NFR。详细 diff 见 `sprint-change-proposal-2026-04-20.md`。

## Executive Summary

**裤衩猫**（CatPhone 为其 iOS 端）是一款 **Watch-first 的挂机式陪伴宠物 app**，为"**久坐却拒绝被自律 KPI 审判**"的年轻人（核心：程序员/设计师/文字工作者/学生；增长：萌系电子陪伴爱好者；爆火：异地情侣与密友）造一只**腕上的触觉猫**。在线挂机 30 分钟触发盲盒掉落，消耗 1000 步"接猫回家"解锁皮肤（1000 步 ≈ 日健康目标 5000 步的 1/5，每盒对齐"日配额 1/5"）；朋友之间通过 2–4 人同屏挂机 + 震动表情形成一条"不打扰的亲密"频道。

本 PRD 聚焦 **iPhone 端**。iPhone 的产品定位为 Watch 主体验的**门户 + 管理 + 深度内容 + 观察者降级端**——而非自成一体的独立 app。解决的核心问题：在"自律类 app 让人疲惫"与"电子宠物赛道始终以手机为中心"两个行业死结之间，开辟一条**反转配件关系**的路径——把手表作为陪伴载体的情感主场，把 iPhone 作为思念的口袋窗户，让走 300 步从"健康压力"变成"接猫回家"的浪漫。

### What Makes This Special

- **负反馈调节替代正反馈刺激**：盲盒掉了必须走 1000 步才能打开——把步数从"奖励门槛"反转为"温柔门槛"。市场上无对应品。
- **触觉社交（广播式身体语言）**：在微信（重）与电话（累）之外建立"我点自己的猫 → 旁边浮 emoji → 房间里大家的 Watch 同步轻震+看见"的轻提醒强感知通路；fire-and-forget 不等已读，完美契合"不打扰的亲密"；2026 companion-pet 赛道几乎无人深耕。
- **Watch-first 反转配件关系**：Pokémon Sleep / Walkr / Pikmin Bloom 均以手机为中心；裤衩猫把 Apple Watch 定义为**电子宠物的真正栖身之所**，iPhone 退为配件端。
- **观察者模式即护城河燃料**：iPhone-only 用户可看朋友猫状态、**与 Watch 用户权限对等地点自己的猫发表情**（fire-and-forget 语义消除"你发的是降级版"的暗示），但不可领奖/解锁——让社交网络密度从装机门槛开始生长，而非终结于装机门槛。
- **Core Insight**：反馈强度 = 即时性 × 体感 × 在场性。Apple Watch 三维满分，整个 companion-pet 赛道却无人将其定位为首要载体。
- **护城河**：社交网络效应 + 共享挂机仪式（同屏 + 震动）——一人的猫是宠物，一群朋友的猫是社群；共享的不是目标而是此刻。单一用户复制容易，关系网络复制不易。

## How to Read This PRD

> **"这不是一只需要你照顾的猫。是一只，在你走到它身边时，刚好抬头看你的猫。"**
>
> ——Story 作者写任何 AC 前，先问：这条 FR 让那一眼**更近**，还是更远？

本 PRD 为 iPhone 端（CatPhone）落地文档，服务于多角色读者。按角色推荐入口：

- **PM / 决策者**：Executive Summary → Success Criteria → Project Scoping → Innovation & Novel Patterns（TRIZ 评估）
- **UX Designer**：User Journeys（7 条带 Story Point IDs）→ Functional Requirements（能力契约）→ NFR · Accessibility
- **Architect**：Cross-Device Messaging Contract → Capability ID Index → Mobile App Specific Requirements → NFR · Reliability/Security
- **Dev（Story 领取）**：Functional Requirements（每条带 `[U/I/E9]` 测试标签 + Capability ID）→ Cross-Device Messaging Contract → Server-Driven New Stories
- **QA**：User Journeys · 失败态（J4）→ FR 测试标签 → NFR · Reliability → Cross-Device Messaging Contract · fail-closed → `CLAUDE-21.4` / `CLAUDE-21.7` 纪律
- **External Reviewer（投资人 / 合作方）**：Executive Summary → Success Criteria → Project Scoping → Innovation & Novel Patterns；**不进入 FR / NFR / Messaging Contract 细节**

**引用纪律**（遵 `CLAUDE-21.8`）：后续 Epic / Story 的 AC **只能 cite** `C-XXX-NN`（Capability ID）或 `FR-NN` 或 `Jn-Sn`（Story Point），**禁止 cite 散文段落**。

**外部文档锚点**：见 "External References" 节；源文档漂移以 server / `PCT-arch` 为准，不得在 PRD 内私自复制。

**PRD 建造史（Party Mode 6 轮关键决策）**：
- Round 1 Victor → 观察者模式 = 护城河燃料
- Round 2 Winston → "iPhone = 配件端" 战略定位
- Round 3 Murat → `WatchTransport` / `ClockProtocol` 测试基础设施约束
- Round 4 Caravaggio / Mary → Slogan v2（"你的猫在家等你走到它身边"）+ Validation evidence debt 登记
- Round 5 → 表情改为 "点自己的猫广播"（fire-and-forget）+ **移除情绪机**（久坐召唤降为纯交互）
- Round 6 Winston / Amelia / Paige / Sophia → 多设备 idle 聚合 + C-OPS 拆分 MVP/Growth + 追溯链修缮 + 灵魂句入扉页

## Project Classification

- **Project Type**：`mobile_app`（sub-type：`companion-pet-watch-first-with-iphone-accessory`）
- **Domain**：`general`（tags：`companion-pet / fitness-driven / virtual-economy / light-social`）
- **Complexity**：`high`
  - 驱动：三端版本化契约（iPhone ↔ Watch ↔ server）、HealthKit 双事实源（显示 vs 权威解锁）、共享 WS envelope + Swift 6 严格并发、iPhone 观察者模式引入 read-only 实时通道、Spine/SpriteKit 渲染层
- **Project Context**：`brownfield`
  - Server Epic 0 已完成、跨端契约（OpenAPI / WS registry）已锁、§21 工程纪律已沉淀；iOS 业务代码处于骨架期（`project.yml` + `CatShared/CatCore/CatWatch` target 就绪，feature 目录空）

## Success Criteria

### User Success —— "Aha" 时刻链

| 时刻 | 语义 | 目标 |
|---|---|---|
| 首次发现盲盒 | 戴表/打开 app 后 30 min 的第一次盲盒掉落 + 用户抬腕或打开 app 主动发现（最早 aha） | 安装后 **24h 内 ≥ 85%** |
| 首次"接猫回家" | 走满 1000 步打开第一个盲盒 | 安装后 **48h 内 ≥ 60%** |
| 首次收到好友震动表情 | 触觉社交首次落地 | 安装后 **7 日内 ≥ 40%** |
| 首次 2–4 人同屏挂机 | 社交网络密度开始形成 | 安装后 **14 日内 ≥ 30%** |
| 日均抬表互动 | 挂机陪伴不追 session 时长，追 glance 频次 | D30 ≥ **6 次/日** |
| 日均开盒数 | 游戏经济稳态指标（最多 5，满额 = 5000 步日健康目标达成） | D30 ≥ **3 盒/日** |

### Business Success

- **网络效应（护城河直测）**：D30 用户平均好友数 ≥ **3**（低于 3 = 护城河未起）
- **观察者→完整用户转化**：iPhone-only 观察者 30 日内配对 Watch 比例 ≥ **25%**
- **Onboarding Watch 配对完成率**：有 Watch 的新用户首次打开后 5 min 内完成配对 ≥ **80%**
- **留存**：D7 ≥ **40%** / D30 ≥ **25%** / D90 ≥ **15%**
- **付费**：**MVP 不引入付费**；Growth 阶段考虑皮肤/盲盒扩展礼包内购（非抽奖型，规避 loot-box 监管）

### Technical Success

- **三端状态同步 P95 延迟**（步数 / 装扮 / 盲盒事件）：iPhone ↔ server ≤ **2s**；server ↔ Watch ≤ **3s**
- **WS 异常恢复**：网络切换后 10s 内重连成功率 ≥ **95%**
- **HealthKit 授权通过率**（新用户首次引导）≥ **85%**
- **崩溃率** ≤ **0.2%**（Crashlytics 口径）
- **§21.4 语义正确性守门**：步数漂移 UI（"已达成·等待确认"文案）必须在 MVP AC 里显式测出；iPhone 本地不做解锁判定（防作弊 + 反事实源漂移）

### Measurable Outcomes

- **北极星指标（复合）**：**D30 用户日均抬表互动次数 × D30 平均好友数**（两项单独做不出数据 = 护城河失败）
- **次级**：观察者→完整用户转化率、D7/D30/D90 留存、Onboarding 配对完成率、日均开盒数

## Product Scope

### MVP —— 证明"反转配件关系 + 触觉社交"假设的最小集

**iPhone 端 MVP**（本 PRD 落地范围）：
- 账号（Sign in with Apple + Watch 配对 + iPhone-only 观察者分支 + **首屏"我的猫"卡片入口**：静态 avatar + 发表情按钮，无动画）
- 好友（添加 / 管理 / 邀请）
- 仓库（皮肤浏览 + **合成材料分栏**）
- **皮肤合成**（颜色分级系统 + 5 低级 → 1 高级合成规则）
- 装扮（大屏编辑，payoff 在 Watch 端显现）
- 步数回顾（历史图表）
- 房间（成员名单 + 在线/挂机状态 + 一句话叙事文案；**无猫渲染、无同屏**）
- **表情广播发送**（与 Watch 用户权限对等，点自己的猫 → 弹选单 → emoji 浮出 + 条件广播，fire-and-forget）
- 表情广播接收（他人猫旁 emoji 浮动 + 轻震）
- **久坐召唤 iPhone 侧**（纯 push 通知，分频道开关；无情绪文案，无触发能力——Watch 独占触发与 haptic）
- 观察者模式（看朋友猫状态 / 发表情对等参与，不可领奖/解锁盲盒）
- **PrivacyInfo.xcprivacy 隐私清单**（App Store 2024/03 起硬要求，必进 MVP；`C-OPS-05`）
- **账户删除 + 数据导出**（SIWA + GDPR 合规红线，必进 MVP；`C-OPS-06`）

**明确不在 iPhone MVP**（归 Watch 独占或后置）：
- 挂机核心动画（Watch 独占，防止"通知剧透仪式感"）
- 盲盒"接回家"解锁交互（Watch 独占，1000 步对齐 Watch 佩戴状态）
- 2–4 人同屏渲染（Watch 独占）
- 久坐召唤的**触发与 haptic**（Watch 独占；iPhone 仅被动接收状态文案）
- ~~日历签到~~（**已从产品移除**——皮肤获取路径收敛为单一盲盒+合成链路）
- ~~共同步数目标金礼盒~~（**已从产品移除**——护城河收敛为"共享挂机仪式"）

### Growth Features (Post-MVP)

- 表情选单扩展（从 3 种基础到 10+ 种，含动态组合暗号——如"比心 + 困意 = 'miss u'"情景）
- 付费内容：皮肤礼包（非抽奖型）、装扮配件市场
- iPhone 端"一句话状态流" → 升级为富媒体信息流（仍不渲染猫，但含截图 / 徽章 / 成就）
- 盲盒皮肤组件扩展（基础款外扩出稀有款 + 合成更高稀有度）
- **Memory Feed**（只读时间线）——表情 & 盲盒历史回看；fire-and-forget 不存历史，但服务端本身有日志，只需 iPhone 只读 view（Round 5 Carson 提议）
- **崩溃上报升级**（`C-OPS-01`）——MVP 阶段用 Xcode Organizer 基础能力，Growth 引入 Crashlytics / Sentry（Round 6 Winston 拆分）
- **远程配置 / Feature Flag**（`C-OPS-03`）——MVP 硬编码够用，Growth 引入（Round 6 Winston 拆分）
- **版本检查强制更新**（`C-OPS-02`）——MVP 用 App Store 原生弹窗，Growth 引入 in-app 强更（Round 6 Winston 拆分）

### Vision (Future)

- **α · 入眠仪式**：情侣/密友间猫挨着睡觉的夜间模式，成为日常入眠前的最后一瞥
- **β · 办公室文化**：Slack presence 的温柔版，企业/共享空间 SaaS 授权
- **γ · 腕上亲密语言**：50+ 种震动暗号可自定义、可分享，成为一种新社交符号（房间广播语义）

## External References

本 PRD 依赖以下既有文档作为上下文锚点。所有引用使用稳定文档 ID，不复制粘贴内容，后续文档漂移以源文档为准。

| 锚点 ID | 引自 | PRD 使用位置 |
|---|---|---|
| `KWM-product` | `裤衩猫.md`（产品愿景原始文档） | Vision / Core Loop / Persona |
| `KWM-idle` | `裤衩猫.md §核心功能·挂机系统` | J1 / J6 盲盒 + 久坐召唤 |
| `KWM-social` | `裤衩猫.md §社交联机` | J3 同屏 / 表情机制 |
| `PCT-modules` | `project-context.md ios_modules_in_scope` | MVP 模块清单 |
| `PCT-arch` | `project-context.md Critical Implementation Rules` | 技术约束（Swift 6 / WS / HealthKit） |
| `CLAUDE-21.4` | `CLAUDE.md §21.4 语义正确性 AC review 早启` | 步数漂移文案 / 语义测试 |
| `CLAUDE-21.7` | `CLAUDE.md §21.7 测试自包含` | xcodebuild test 约束 |
| `SRV-PRD` | `server/_bmad-output/planning-artifacts/prd.md` | server 契约基线 |
| `WSR-v1` | `docs/api/ws-message-registry.md (apiVersion: v1)` | WS 消息类型基线 |
| `OAPI-0.14.0` | `docs/api/openapi.yaml (0.14.0-epic0)` | HTTP 契约基线 |

## Capability ID Index

PRD 后续章节（FR/AC/Story）**只能 cite 本索引的 C-XXX-NN ID，禁止 cite 散文段落**（遵 `CLAUDE-21.8` 可追溯纪律）。

### Onboarding（C-ONB-*）
- **C-ONB-01**：Sign in with Apple 登录
- **C-ONB-02**：Watch 配对（含 iPhone-only 观察者降级分支；分支选择本身也是能力）
- **C-ONB-03**：HealthKit 首次授权（可拒绝；拒绝后降级 UX 见 C-REC-02）

### 步数（C-STEPS-*）
- **C-STEPS-01**：HealthKit 本地步数读取（仅用于展示 SSoT）
- **C-STEPS-02**：Server 权威步数 ACK（盲盒解锁判定唯一依据，防作弊）
- **C-STEPS-03**：步数漂移窗口 UI（"已达成·等待确认"文案，遵 `CLAUDE-21.4`）

### 盲盒（C-BOX-*）
- **C-BOX-01**：在线挂机计时 30 min → server 掉落事件入**待领取队列**（server 权威 · 无主动推送 · 用户主动 GET 或自然打开 app 发现）
- **C-BOX-02**：盲盒待解锁状态 UI（1000 步进度条）
- **C-BOX-03**：Watch 端开箱动画（iPhone 不渲染，防通知剧透）
- **C-BOX-04**：仓库队列（iPhone + Watch 同步；待解锁 / 已解锁两态）
- **C-BOX-05**：盲盒→材料路径（重复皮肤自动归入合成材料，见 C-SKIN-02）

### 皮肤 + 合成（C-SKIN-*）
- **C-SKIN-01**：皮肤颜色分级系统（候选：白 < 灰 < 蓝 < 绿 < 紫 < 橙）
- **C-SKIN-02**：合成规则引擎（5 低级 → 1 高级，比例候选；材料计数 + 合成操作）

### 房间（C-ROOM-*）
- **C-ROOM-01**：成员列表 + 在线/挂机状态（iPhone 纯文字，无猫渲染）
- **C-ROOM-02**：叙事文案合成引擎（如"小林的猫刚刚睡着了 · 2min 前"）
- **C-ROOM-03**：2-4 人同屏渲染（Watch 独占）
- **C-ROOM-04**：环绕跑酷等同屏特效（**server 判定触发**，fail-closed）
- **C-ROOM-05**：房间邀请链接（深链接 / 扫码）

### 社交 / 表情（C-SOC-*）
- **C-SOC-01**：**房间表情广播发起**（点自己的猫 → 弹选单 → 选 emoji → 本地浮 emoji + 有房间则 server fan-out；fire-and-forget）
- **C-SOC-02**：房间表情广播接收（emoji 图标浮在 emitter 猫旁 + Watch 轻震）
- **C-SOC-03**：好友关系管理（添加 / 屏蔽 / 解除）

### "我的猫"入口（C-ME-*）
- **C-ME-01**：iPhone 账号 tab 首屏"我的猫"卡片（静态 avatar + 发表情按钮；无动画）

### 装扮（C-DRESS-*）
- **C-DRESS-01**：装扮编辑（iPhone 大屏）
- **C-DRESS-02**：装扮 payoff 渲染（Watch 端动画呈现）

### 挂机 + 久坐召唤（C-IDLE-*）
- **C-IDLE-01**：猫动作同步（跟随 Watch 物理状态：走路它跑 / 睡觉它睡 / 挂机它敲桌子；**无情绪 layer**）
- **C-IDLE-02**：Watch 主动 haptic 召唤（`IdleTimerProtocol` 抽象，2h 无活动阈值触发；震感不同于盲盒通知；Watch 独占；纯 idle timer 触发，非情绪因果）
- ~~**C-IDLE-03** 已删除~~（原"猫情绪状态"——Round 5 决定移除情绪机）

### 容错（C-REC-*）
- **C-REC-01**：WS 断线重连 + 事件补发（server 按 last_ack_seq 回放）
- **C-REC-02**：HealthKit 授权拒绝降级 UX（步数回退 "—" + 引导条）
- **C-REC-03**：fail-closed 兜底文案（遵 `CLAUDE-21.3`）

### 跨端同步（C-SYNC-*）
- **C-SYNC-01**：三端状态同步（iPhone ↔ server ↔ Watch；P95 延迟目标见 Success Criteria）
- **C-SYNC-02**：msg_id + seq 幂等语义（详见 Cross-Device Messaging Contract）

### UX 生存层（C-UX-*，Round 5 新增）
- **C-UX-01**：叙事 Onboarding（3 屏卡片含 Slogan，SIWA 前插入）
- **C-UX-02**：撕包装首开仪式（盒子震动 → 猫跳出 → 命名猫）
- **C-UX-03**：全局空状态（仓库空 / 好友空 / 房间无广播 各自引导态 CTA）
- **C-UX-04**：用户 identity 可编辑（昵称 / 头像 / 个性签名 + 设置入口）
- **C-UX-05**：负面社交闭环（举报 / 静音好友 / 体面退房/转让房主）

### Dev 可观测性 + 合规（C-OPS-*，Round 5 新增）
- **C-OPS-01**：崩溃上报（Crashlytics 或对等）
- **C-OPS-02**：版本检查 + 强制更新提醒（App Store reject 规避）
- **C-OPS-03**：远程配置 / 功能开关 / kill switch
- **C-OPS-04**：客户端结构化日志上报（对齐 `Log.*` facade per `PCT-arch`）
- **C-OPS-05**：`PrivacyInfo.xcprivacy` 隐私清单（iOS 17 硬要求，2024/03 起 App Store 强制）
- **C-OPS-06**：账户删除 + 数据导出（SIWA 强制 + GDPR right to erasure）

## User Journeys

### Journey 1 · 小林 —— 久坐办公族 · 有 Watch · 核心 Happy Path

**Opening**：小林 28 岁程序员，昨天新买了 Apple Watch SE。连续三天手表 "Stand" 提醒红圈没合拢，他已经麻木。朋友圈刷到"裤衩猫"——"不打你健康牌，不开合拢圆环，只让猫等你回家"。他下载了。

**Rising Action**：Sign in with Apple 一键登录（J1-S1）。Onboarding 引导他打开 iPhone 旁边的 Watch，10 秒完成配对（J1-S2）。HealthKit 授权弹窗一次通过（J1-S3）。Watch 表盘出现一只 Bongo Cat 风格的小猫，跟着他在键盘上敲下动作——**他敲代码，它敲桌子**。他笑了出来。下午 3:10，他抬腕看时间——**屏幕右上角出现一个小礼物图标，猫蹲在它旁边好奇地打量**（J1-S4），点进去显示"待解锁 · 需 1000 步"。

**Climax**：他站起来走到茶水间（J1-S5）。10 分钟后，手表从口袋震动，他抬腕——**看到礼物从天上飞下来，猫扑过去用爪子把它拍开**，一块新皮肤（"条纹裤衩"）滑进仓库（J1-S6）。

**Resolution**：下午他又接了 2 只猫回家。收工前在 iPhone 仓库里慢慢欣赏今天的 3 块皮肤（J1-S7），挑了一条给明天的猫穿上（J1-S8）。他打开账号 tab 看到"我的猫"卡片——**点了一下**，弹出表情选单，选了"开心"——猫旁边浮出一颗星星（J1-S9）。他此刻没在任何房间，没人看见，但他看见了。起身走的理由从"健康"变成了"它在等我"。

**Capabilities-Touched**：`C-ONB-01..03`, `C-STEPS-01..02`, `C-BOX-01..04`, `C-DRESS-01..02`, `C-ME-01`, `C-SOC-01`

---

### Journey 2 · 小雨 —— 异地情侣 · iPhone-only 观察者 · 护城河燃料路径

**Opening**：小雨 24 岁在外地读研，没 Apple Watch。男友小林发来一条 App Store 链接和一句话："下一个，我的猫在里面等你来看。"她点开下载（J2-S1），打开 app，onboarding 弹窗说："没检测到配对的 Apple Watch——你可以等有表了再体验完整版，或者先进来看看。" 她选了"先进来看看"（J2-S2）。

**Rising Action**：SIWA 登录（J2-S3）后进入观察者模式。账号 tab 首屏——**她自己的猫**（一只灰色条纹的，默认装扮）（J2-S4）。她点进去，一个"发表情"按钮。她按了一下，弹出 3 个表情：比心 / 惊讶 / 困意。她点了比心——**猫旁边浮出一颗心**，然后慢慢消散（J2-S5）。

她此刻没在任何房间（小林还没邀请她）。没有任何人看见这一颗心。但她自己看见了，**她知道刚才那一秒猫听到她了**。

**Climax**：三天后小林邀请她进房间。好友 tab 出现邀请（J2-S6），她接受，进入房间——文字流显示"**小林的猫刚刚醒了** · 3min 前"。她再次点"我的猫"按钮发比心——**这一次，server 把那颗心广播到小林的 Watch**（J2-S7）。

**Resolution**：小林午休抬表，看见**一只灰色条纹的猫**漂浮在房间里，旁边一颗心慢慢消散。他不知道小雨刚才是哪一秒发的，小雨也永远不会知道他哪一秒看见的——**这正是这个 app 教会他们的**：有一些表达不需要被即时确认。

两周后小雨在购物车里加了一块 Apple Watch SE（J2-S8）——不是因为 iPhone 发心不够，**是因为她想被小林也这么悄悄地想起**。

**Capabilities-Touched**：`C-ONB-01..02`（观察者分支）, `C-ME-01`, `C-SOC-01`（无房间 + 有房间两态）, `C-ROOM-01..02`, `C-ROOM-05`

---

### Journey 3 · 小林 & 小雨 —— 双方都有 Watch · 亲密社交深用户

**Opening**：小雨拿到新 Watch 已经两周（J3-S1）。周三午休，小林在北京办公室，小雨在上海图书馆，两地时差 0 但节奏不同。

**Rising Action**：小林挂机 30 min，盲盒掉了（J3-S2）。他起身往茶水间走。同一时刻小雨也起身走路（J3-S3）——**两人的 Watch 房间都进入"同屏 2 人"模式**：小林的猫和小雨的猫在屏幕上并排走动（J3-S4）。两个都在走路，屏幕上触发**环绕跑酷特效**——两只猫追着一颗星星绕屏幕跑一圈（J3-S5，server 判定下发）。

**Climax**：小林走满 1000 步，接猫回家，当场开出一块"宇航员装扮"（J3-S6）。小雨抬手点自己的猫——**一颗惊讶的星星从她猫头上蹦出来**（J3-S7），小林的 Watch 同步显示了这颗星星浮在小雨的猫旁。小林也回点了一下自己的猫，比心浮出（J3-S8）。两人都没说话。

**Resolution**：午休结束，两人回到工位。**下午三点多，小林突然手腕痒了一下，下意识抬起来看**——屏幕上猫趴在新宇航员皮肤里打哈欠，小雨的猫头顶还留着一颗慢慢消散的星星（J3-S9）。他笑了笑，放下手。他不知道为什么那一下会抬腕，**但那是今天最好的瞬间**。

**Capabilities-Touched**：`C-BOX-01..04`, `C-DRESS-02`, `C-ROOM-03..04`, `C-SOC-01..02`, `C-SYNC-01..02`

---

### Journey 4 · 小林 —— 边缘恢复 · 网络 / 授权失败

**Opening**：周一早高峰，小林地铁通勤。手表在腕上，iPhone 在口袋。地铁进入长隧道，WS 断连（J4-S1）。

**Rising Action**：挂机 30 min 时间到，但 server 的盲盒掉落事件发不出来——Watch 端出现一行小字："**已达成·等待确认**"（J4-S2，遵 `CLAUDE-21.4` 文案规范）。iPhone 端仓库 tab 显示相同标记。小林出站，WS 重连成功（J4-S3）。server 按 `last_ack_seq` 补推 2 个盲盒掉落事件（J4-S4），Watch 立即弹出"你有 2 个宝箱等你回家"通知。

**Climax**：小林当天午休想进仓库看看，iPhone 首次请求 HealthKit 授权——他手滑点了"拒绝"（J4-S5）。**步数展示回退到 "—"，并出现引导条**："未开启 HealthKit 授权，无法接猫回家。去设置开启 >"（J4-S6）。他点进去授权。

**Resolution**：授权完成回到 app，步数恢复显示 3247（J4-S7）。队列里 2 个盲盒等着——他笑了笑："下班路上接你们。"

**Capabilities-Touched**：`C-REC-01..03`, `C-STEPS-01..03`, `C-BOX-04`, `C-ONB-03`, `C-SYNC-01..02`

---

### Journey 5 · 小林 —— 孤独首开（陪伴单机兜底）

**Opening**：小林新装好 app（J5-S1），通讯录里没人玩裤衩猫。onboarding 完成（J5-S2），配对完 Watch。

**Rising Action**：打开主 tab——他的猫在屏幕上，穿着默认条纹衫（J5-S3），**看见他打开 app 就伸了个懒腰**。他没有加好友的冲动。他起身去接水，Watch 震动——猫在敲桌子。他抬腕看了一眼，猫做了个惊讶表情（J5-S4）。他笑了。

**Climax**：他回座位，点了一下账号 tab 的"我的猫"卡片——弹出表情选单，他选了"哈欠"。**猫打了个哈欠，旁边浮出一个 zzz emoji**（J5-S5）。没人看见，但他看见了。这一瞬间他明白：**陪伴不需要观众**。

**Resolution**：一周过去，他还没加任何好友，但每天打开 app 看一眼猫（J5-S6）。**这是他第一次觉得 app 没有推他去社交**——它给了他一个人陪自己的方式。

**Capabilities-Touched**：`C-ONB-01..03`, `C-ME-01`, `C-SOC-01`（无房间态）, `C-IDLE-01`, `C-SKIN-01`（默认皮肤）

---

### Journey 6 · 小林 —— 久坐召唤（北极星"反馈"本体）

**Opening**：下午 3 点，小林已经坐了 2 小时 40 分钟没起身（J6-S1）。Watch 端 idle timer 越过 2h 阈值——**猫在屏幕里扒拉屏幕边缘，发出小小的喵的动作**（J6-S2，纯动作不走情绪机）。

**Rising Action**：3:07 分，手表轻震——**不同于盲盒通知的震感，更轻更短**（J6-S3）。他抬腕，猫正看着他，做了一个"来嘛"的小手势（J6-S4）。

**Climax**：他笑了，站起来走到窗边（J6-S5）。走路 30 秒后 Watch 检测到活动——猫跑起来做了个欢迎动作，旁边浮一下"很高兴你来了"的小 emoji 气泡（J6-S6，纯交互触发，不涉及情绪状态转移）。

**Resolution**：等他回到工位坐下，这一次没有盲盒奖励，**但他感觉到反馈回路已经完整**——这就是这个 app 和其他步数 app 的区别。起身本身就是奖励。

**Capabilities-Touched**：`C-IDLE-01..02`（动作同步 + 久坐 haptic；**无情绪机**）, `C-STEPS-01..02`

---

### Journey 7 · 小林 —— 皮肤合成（重复转化 / 防挫败）

**Opening**：他走完 1000 步开盲盒（J7-S1），里面是**一条蓝色条纹裤衩**——他已经有了（J7-S2）。

**Rising Action**：UI 没有"抱歉重复"这种沮丧文案，而是说：「**已收入合成材料 · 蓝色条纹 × 3 / 5 可合成绿色条纹**」（J7-S3）。他点开仓库，看到"材料"分栏，蓝色条纹的计数从 2 变成 3（J7-S4）。他笑了一下："**还差两条就能换一条绿色的**"。

**Climax**：三天后他凑够 5 条蓝色条纹，点"合成"——材料燃烧出一条**绿色条纹裤衩**（稀有度提升一级）（J7-S5）。

**Resolution**：他立刻给猫换上——Watch 端的猫穿着新绿色裤衩做了一个炫耀的转圈（J7-S6）。

**Capabilities-Touched**：`C-BOX-05`, `C-SKIN-01..02`, `C-DRESS-01..02`

---

### Journey Requirements Summary

**追溯矩阵（Round 6 Paige 补 Jn-Sn → FR 映射列）**：

| 能力类 | Primary（主属 journey） | Also-touches | 代表性 Story Point → FR 映射 |
|---|---|---|---|
| Onboarding `C-ONB-*` | J1, J2, J5 | `C-UX-01/02` | J1-S1→FR1 / J1-S2→FR2 / J1-S3→FR4 / J2-S2→FR3 / J2-S3→FR1 |
| 步数 `C-STEPS-*` | J1, J4, J6 | `C-BOX`, `C-IDLE` | J1-S5→FR31 / J4-S2→FR34 / J4-S7→FR31 |
| 盲盒 `C-BOX-*` | J1, J4, J7 | `C-SKIN`, `C-STEPS` | J1-S4→FR35a / J4-S4→FR47 / J7-S1→FR35b |
| 皮肤 + 合成 `C-SKIN-*` | J7 | `C-BOX`, `C-DRESS` | J7-S3→FR38 / J7-S4→FR39 / J7-S5→FR40 |
| 房间 `C-ROOM-*` | J2, J3 | `C-SOC` | J2-S5→FR21 / J2-S6→FR20 / J3-S4→FR23 / J3-S5→FR24 |
| 社交 / 表情 `C-SOC-*` | J2, J3, J5 | `C-ROOM`, `C-ME` | J2-S7→FR28 / J3-S7→FR27 / J5-S5→FR29 |
| 我的猫入口 `C-ME-*` | J1, J2, J5 | `C-SOC` | J1-S9→FR26 / J2-S4→FR26 / J5-S5→FR26 |
| 装扮 `C-DRESS-*` | J1, J7 | `C-SKIN` | J1-S8→FR41 / J7-S6→FR42 |
| 挂机 + 久坐 `C-IDLE-*` | J5, J6 | `C-STEPS`, `C-SOC` | J5-S4→FR44 / J6-S3→FR45a / J6-S5→FR46 |
| 容错 `C-REC-*` | J4 | 所有 | J4-S2→FR34 / J4-S3→FR47 / J4-S5→FR50 / J4-S6→FR50 |
| 跨端同步 `C-SYNC-*` | J3, J4 | 所有 | J4-S4→FR47（`last_ack_seq` 补发）|
| UX 生存层 `C-UX-*` | J5, J6, J7 | `C-ONB`, `C-SOC` | 撕包装 J5-S3→FR7 / 空状态场景→FR51 |
| OPS 合规层 `C-OPS-*` | — | 所有 | （基础设施层，不直接 Journey 承载；MVP 仅 `C-OPS-05/06`）|

## Cross-Device Messaging Contract

所有跨端消息共享本节约束，FR/AC 层 **禁止**在 story 内重新定义这些语义（遵 `PCT-arch` + `WSR-v1`）。

### Envelope

- 格式继承 `server/internal/ws/envelope.go`（iPhone / Watch 同协议）：
  - 请求：`{id: String, type: String, payload?: JSON}`
  - 响应：`{id?: String, ok: Bool, type: String, payload?: JSON, error?: {code, message}}`
- 错误 code 一律 `UPPER_SNAKE_CASE`

### Client Msg ID

- 所有**客户端发起**的 msg 必带 `client_msg_id`（UUID v4）
- Server 去重窗口 = **60s**；同 `client_msg_id` 重放直接丢弃不下发

### ACK 语义（两档）

| 档位 | 场景 | ACK 行为 |
|---|---|---|
| **事务性** | 盲盒解锁 / 装扮同步 / 好友关系变更 / 皮肤合成 | 双向 ack 强一致；失败必须显式告知 UI |
| **轻社交** | 表情广播 (`pet.emote.broadcast`) / 未来暗号 | **fire-and-forget** —— 发送方不等 ack，对端自治呈现，UI 不显示"已送达/已读" |

### Seq / last_ack_seq

- 订阅式 msg（`room.state.*` 等）用 `seq` 单调递增
- 重连后 server 按 `last_ack_seq` 回放遗漏事件

### TTL

- Server 待推事件队列 TTL = **24h**
- 超过 24h 未拉取 → GC + 落审计日志 + 下次重连 fail-closed 提示"部分事件已过期，请刷新"
- **AC**：iPhone / Watch fixture **必须覆盖**"断网 25h 后重连 → 事件 GC → fail-closed 提示"分支

### 隐私 Gate（`pet.state` / 观察者）

房间内 `pet.state.broadcast` 字段分级：

- **观察者（iPhone-only）可见**：`current_activity`（**idle / walking / sleeping** 物理动作态，不含任何情绪词）、`minutes_since_last_state_change`
- **观察者不可见**：`exact_step_count`、`heart_rate`、任何 `health_*` 原始字段
- Server 端在 fan-out 时按订阅者类型裁字段

### Fail-Closed 总则（遵 `CLAUDE-21.3`）

任何未知 msg type / registry 漂移 / 队列 GC 后重连 → **强制 UI 提示**"网络异常，请稍后重试"，**禁止**用本地缓存猜测式兜底。可观测信号：`Log.ws.error(...)` 结构化记录。

### Idle Timer 聚合（多设备 · Round 6 Winston）

久坐召唤的 `last_active_ts` **server 权威聚合**，不依赖客户端各算各的：

- **聚合规则**：`last_active_ts = max(iPhone_last_active, Watch_last_active)`
- **iPhone 心跳源**：`/v1/platform/heartbeat` 前台每 60s 一次（可容忍偏差）
- **Watch 心跳源**：`HKObserverQuery` 活动事件 + Watch app 前台时间
- **App 冷启动后**：`last_active_ts` 从 **server 拉取**，客户端不假设 background task 可持续计时
- **召唤触发**：server 在 `now - last_active_ts > 2h` 且用户无其他设备活跃时，push event 至 Watch 触发 haptic
- **边界**：若 Watch 未配对（观察者模式）→ 仅以 iPhone 心跳计时；观察者本来就不参与久坐召唤（Watch 独占）

## Innovation & Novel Patterns

### Detected Innovation Areas

PRD 在 Round 4 Party Mode 挑战后保留 3 项声称创新，**每项附诚实 TRIZ 级别 + 救命语义**：

1. **负反馈温柔门槛**：盲盒先掉落 → 必须走 1000 步才能领取（延迟交付而非惩罚驱动）。与 Duolingo 失败连胜 / Zombies, Run! 的负反馈"惩罚"流派**情绪正交**——这里是"温柔等你来"，不是"恐惧你失去"。
2. **触觉广播（定位为『宠物情绪溢出』）**：点自己的猫 → 猫旁边浮 emoji → 房间成员 Watch 同步轻震。关键 positioning：**发起者不是"我给朋友发心"，是"我的猫做了一个情绪，朋友们看见"**。Fire-and-forget 是实现形式（遵 Cross-Device Messaging Contract），内核是"社交信号无身份/无历史/无义务"——把**信号**从"消息队列+已读+回复债"的 IM 范式中解耦。
3. **Watch-主角反转（iPhone 为配件）**：否定"屏大=主角"的技术最优默认。陪伴场景下"屏小=亲密"成立——TRIZ Inversion + Ideality 组合。iPhone 在此是"不同载体"，不是"阉割版"。

### Market Context & Competitive Landscape

- **直接竞品 phone-centric**：Pokémon Sleep / Walkr / Pikmin Bloom / 旅行青蛙 / Finch（均手机为主，Watch 为附件）
- **触觉社交空档**：Apple Fitness+ 好友分享是异步"完成通知"，非情绪广播；Apple Digital Touch（发心跳给朋友）已下线（市场证伪"高频人对人触觉"）——裤衩猫通过"**宠物代言人**"语义转换规避此坟墓
- **走路解锁叙事前例**：Zombies, Run!（2012，步数解锁叙事）；Finch（延迟满足）——与 #1 部分重叠，但情绪定位正交
- **Fire-and-forget 先例链**：Unix signal (1970s) / UDP broadcast (1980s) / WeChat 摇一摇 (2011) / Snapchat Poke (2012) / Yo (2014) —— "不等已读"是古招式，在"宠物情绪溢出"场景下获得新语义
- **Validation evidence debt**：未扫国内小游戏市场（抖音/微信小游戏"走路解锁扭蛋"类目），pre-MVP 原型测试未做。**"市场无先例"声明基于海外竞品推断**，PRD 接受此风险，后续 epic 补证据（见 frontmatter `openQuestionsForVision`）

### Innovation Rigor（TRIZ Level 自评）

为避免"innovation theater"，每项创新做 TRIZ 级别自评：

| 创新项 | TRIZ Level | 原理说明 | 救命措辞（规避市场证伪） |
|---|---|---|---|
| #1 负反馈温柔门槛 | **2.5（已知重组）** | 延迟奖励 + 运动门槛 + 盲盒包装的新组合；原理不新，**情绪定位新**（温柔而非惩罚） | 叙事对齐"接猫回家"，避开"你欠猫的债"暗示 |
| #2 触觉广播 + 宠物情绪溢出 | **3.5（范式反叛）** | "社交信号"从"消息队列+已读+回复债"范式解耦；fire-and-forget 是形式，**信号无身份/无历史/无义务**是新原理 | 定位为**"宠物情绪溢出"**（猫代言人表达），**不是**"人对人触觉消息"——规避 Apple Digital Touch 下线覆辙 |
| #3 Watch-主角反转 | **4（假设反转）** | 否定"屏大=主角"技术最优默认；陪伴场景下"屏小=亲密"成立——TRIZ Inversion + Ideality | iPhone 明示为"不同载体"，不是"阉割版"，避免被用户当作残次品 |

**TRIZ Level 参考**：1=老套 / 2=重组 / 3=新应用 / **4=假设反转** / 5=全新原理。**不声称 Level 5**。

### Validation Approach

由 Success Criteria 北极星 + Aha 时刻链驱动；D30 稳态数据若三项都绿 → 创新假设验证成立：

- **#1 验证**：D30 日均开盒 ≥ 3 盒 + 留存 D30 ≥ 25%——证明门槛没让人弃坑
- **#2 验证**：首次收到好友震动表情 7 日内 ≥ 40% + 表情推送关闭率 < 30%——证明触觉广播被接收而非被嫌烦
- **#3 验证**：D30 日均抬表互动 ≥ 6 次 + 观察者→完整用户 30 日 ≥ 25%——证明 Watch 是情感主场

### Innovation Failure Signals（早期撤退信号）

- **#3 失败信号**：D30 用户在 iPhone 端的互动次数/时长**反超** Watch → "Watch 主场"假设崩塌，产品哲学级失败，触发策略重估
- **#2 失败信号**：首次收到好友震动表情的用户里 >60% 关闭推送 → 触觉广播不讨喜，**回落文字通知路径**
- **#1 失败信号**：48h 内首次"接猫回家"完成率 <35%（vs 目标 60%）→ 门槛过刻薄，**降到 500 步**（预案见 Success Criteria Risk Mitigation）

### Risk Mitigation

- **#1 风险**：负反馈门槛被用户感知为惩罚 → **缓解**：UI 文案永远强调"接回家"而非"解锁费"；配合"温柔"美学（猫等待的动画、门口的灯）
- **#2 风险**：Digital Touch 覆辙（高频广播触觉被证伪） → **缓解**："宠物情绪溢出"语义 + 频次硬控（下一 epic：每用户每房间每小时最多 N 次 emote，防骚扰）+ 接收端静音选项
- **#3 风险**：Watch 渗透率低导致 TAM 受限 → **缓解**：观察者模式作为护城河燃料通道（Round 2 Victor 论点）；30 日观察者→买表转化率 ≥ 25% 为健康指标

## Mobile App Specific Requirements

### Platform Requirements

- **Native iOS**（Swift 5.9+ / SwiftUI / iOS 17.0+ 硬要求），非跨平台
- Watch 是**独立 watchOS target**（不在本 PRD 范围）
- XcodeGen 驱动（`project.yml` = SSoT，禁直接改 `Cat.xcodeproj/`）
- **Swift 6 严格并发 day 1**（`-strict-concurrency=complete`）

### Device Permissions & Capabilities

**必需授权（Info.plist keys）**：

- `NSHealthShareUsageDescription` — HealthKit 步数读取（`C-STEPS-01`）
- `NSCameraUsageDescription` — 扫描朋友邀请码（`C-ROOM-05`）

**不使用**：Location / Microphone / Contacts / Photos / Calendar / IDFA（遵 `PCT-arch` 安全规则）

**系统框架依赖**：

- HealthKit — stepCount 本地读取 SSoT for display
- WatchConnectivity — iPhone ↔ Watch 通道，抽象为 `WatchTransport` protocol（Round 3 Murat 方案）
- SpriteKit + `esoteric-software/spine-runtimes`（SwiftPM）— Spine 猫动画（Watch 独占渲染；iPhone 仅静态 avatar）
- Keychain — JWT / refresh token 存储（`kSecClassGenericPassword`）
- APNs — push 投递
- `UINotificationFeedbackGenerator` — iPhone 轻震（补 Watch haptic）

### Offline Mode Strategy

| 网络状态 | 能做 | 不能做 |
|---|---|---|
| 完全离线 | HealthKit 步数显示、仓库已下载皮肤浏览、默认装扮查看、"我的猫"卡片表情触发（本地呈现） | — |
| WS 断线但 HTTP 可达 | 挂机本地计时 UI、`C-STEPS-03` "已达成·等待确认" 文案 | 盲盒解锁事件 ACK、表情广播 fan-out、房间状态推送、好友关系变更 |
| 启动 `/v1/platform/ws-registry` 失败 | — | **主功能全屏蔽 + "网络异常，请稍后重试"**（fail-closed，遵 `CLAUDE-21.3`） |

### Push Notification Strategy

**推送分级**（接 `openQuestionsForVision` "iPhone 推送分级策略"）：

| 场景 | Push 行为 | 剧透规避 |
|---|---|---|
| 盲盒掉落（挂机 30min 成熟） | **无通知**（旅行青蛙式纯发现制 · 无 Watch 震动 / 无 APNs / 无 badge） | 用户下次抬腕或打开 app 自然发现 |
| 朋友邀请房间 | 标准通知 | — |
| 观察者收到他人表情广播 | 降级通知（替代 Watch 轻震） | 文案："[朋友的猫] 给房间发了 [emoji]" |
| 久坐召唤 | **不发 iPhone push**，only Watch haptic | 避免 iPhone + Watch 双震淹没 |

用户可**分频道关闭**推送（盲盒 / 好友 / 表情 三档独立开关）。

### Store Compliance

- **Privacy Policy**：明示 HealthKit 用途 / 推送通知用途 / 好友图暴露范围 / **不上传 raw step count**（server 仅收 unlock ACK；遵 Cross-Device Messaging Contract 隐私 gate）
- **Age Rating**：**12+**（无暴力色情，但有用户互动）
- **HealthKit Compliance**（App Store Guideline 2.4.2）：健康数据仅用于"接猫回家"机制，不共享不二次用途
- **Loot-Box Compliance**：盲盒为**步数解锁非付费抽取**，但仍披露皮肤概率（见 `S-SRV-7`），对齐国内《盲盒经营活动规范指引》+ 海外 loot-box 监管最小面
- **Sign in with Apple**：MVP 仅 SIWA（符合 App Store 4.8；若后续加第三方 OAuth 则 SIWA 仍需并列）
- **Export Compliance**：WS 必须 `wss://`（TLS + ATS 合规；`ws://` 仅限 debug build 特定联调域名豁免）

### Implementation Considerations

- **测试自包含硬规则**（`CLAUDE-21.7`）：`xcodebuild test` 一命令绿，禁依赖真机 / 真 Watch 配对 / 特定账号 / 外网
- **Onboarding 测试旁路**（Round 3 Murat 方案）：`AppEnvironment.pairingMode: .real | .mockPaired | .mockUnpaired`，仅通过 `TESTING_HOOKS` compile flag 注入；release build `#if !TESTING_HOOKS` 物理剔除 Mock 分支；双 CI gate（release binary 不含 `MockPairingAdapter` 符号 + `PairingGateTests` 断言 release 不可达）
- **真机 / Watch 配对 / Spine 真机校验归 Epic 9**（`CLAUDE-21.6`，不塞业务 epic 关键路径）
- **依赖管理**：只引 Apple 框架 + `esoteric-software/spine-runtimes`（遵 `PCT-arch` "不引过度依赖"）
- **XcodeGen 工作流**：新增 target / SDK / 资源都写在 `project.yml`，禁止直接修改 `Cat.xcodeproj/`

## Project Scoping & Phased Development

### MVP Strategy & Philosophy

**MVP Approach**：**Experience MVP** —— 不是 revenue/platform/problem-solving 导向，是**验证 3 项创新假设的用户体验**：

1. 负反馈温柔门槛是否被用户感知为温柔（而非惩罚）
2. 触觉广播（宠物情绪溢出）是否感动而非惊扰
3. Watch-主角反转是否真能成为情感主场（反超 iPhone 使用）

**MVP 成功判据**：引用 Success Criteria 北极星指标 + 3 个 Aha 时刻链 + Innovation Validation Approach（见 Innovation & Novel Patterns 节）全绿。

**Resource Requirements（候选估算）**：

- iOS（本 PRD）：1–2 SwiftUI engineer
- watchOS：1 engineer（独立 PRD）
- Server：Go engineer（独立 PRD，Epic 0 完成）
- Spine animator：1 part-time（**关键资源 / 稀缺**）
- UX 设计：1 part-time
- 跨端协调：PM 自承担

### MVP Feature Set (Phase 1)

**Core User Journeys Supported**（引用 User Journeys 节）：

- J1 小林核心 Happy Path（有 Watch）
- J2 小雨 iPhone-only 观察者（护城河燃料）
- J3 小林 & 小雨 同屏深社交（双方有 Watch）
- J4 小林 边缘恢复（网络 / 授权失败）
- J5 小林 孤独首开（陪伴单机兜底）
- J6 小林 久坐召唤（北极星"反馈"本体）
- J7 小林 皮肤合成（重复转化）

**Must-Have Capabilities**（引用 Capability ID Index；后续 FR/Story 必须 cite 这些 ID）：

- Onboarding：`C-ONB-01..03`
- 步数：`C-STEPS-01..03`
- 盲盒：`C-BOX-01..05`
- 皮肤 + 合成：`C-SKIN-01..02`
- 房间：`C-ROOM-01..02`, `C-ROOM-05`（不含 `C-ROOM-03..04` Watch 独占）
- 社交 / 表情：`C-SOC-01..03`
- 我的猫入口：`C-ME-01`
- 装扮：`C-DRESS-01..02`
- 挂机 + 久坐召唤：`C-IDLE-01`（猫动作同步 iPhone 侧被动展示）；`C-IDLE-02` Watch 独占（**无情绪机**）
- 容错：`C-REC-01..03`
- 跨端同步：`C-SYNC-01..02`
- UX 生存层（Round 5 新增）：`C-UX-01..05`
- Dev 可观测性 + 合规（Round 5/6 新增，**拆分 MVP/Growth**）：
  - **MVP**：`C-OPS-05`（PrivacyInfo.xcprivacy）+ `C-OPS-06`（账户删除 + 数据导出）
  - **Growth**（Round 6 Winston 拆分）：`C-OPS-01..04`（Crashlytics / 版本检查 / 远程配置 / 结构化日志）——MVP 用原生能力兜底

### Post-MVP Features

**Phase 2 (Growth)** —— 引用 Product Scope · Growth Features 节：

- 表情选单扩展（3 → 10+ 种 + 组合暗号）
- 付费内容（皮肤礼包 / 装扮配件市场，非抽奖型）
- iPhone "一句话状态流" → 富媒体信息流
- 合成更高稀有度皮肤
- 久坐召唤 iPhone 侧主动触发（若数据显示 iPhone 场景有价值）

**Phase 3 (Vision)** —— 引用 Vision (Future) 节：

- α · 入眠仪式（情侣/密友猫挨着睡觉的夜间模式）
- β · 办公室文化（Slack presence 温柔版，企业 SaaS）
- γ · 腕上亲密语言（50+ 暗号自定义 / 分享）

### Risk Mitigation Strategy

| 维度 | 风险 | 缓解 |
|---|---|---|
| **Technical** | Spine SwiftPM + Swift 6 严格并发兼容性（第三方 runtime 可能非 Sendable） | 早期 Spike 验证（归 Epic 9 或独立 Spike story） |
| **Technical** | Watch 2–4 人同屏 60fps 续航 | FPS 自适应 + 性能埋点（前台 60 / 后台 15） |
| **Technical** | 单 WS 共享信封的 iPhone/Watch 主从仲裁（Round 2 Winston 警告） | Server 侧 session 接管策略写入 Cross-Device Messaging Contract（部分已落，后续 Story 钉死状态机） |
| **Market** | Apple Watch 渗透率 <15%（中国） | 观察者模式 = 护城河燃料通道（Round 2 Victor） |
| **Market** | Apple Digital Touch 覆辙（高频人对人触觉被证伪） | "宠物情绪溢出" positioning（Round 4 Victor） |
| **Market** | Validation evidence debt（"市场无先例"未扫国内小游戏市场） | Pre-MVP 100 人问卷 + 5 人原型测试（Round 4 Mary 建议，后续 epic 补；frontmatter `openQuestionsForVision` 已登记） |
| **Resource** | Spine animator 稀缺 | 基础皮肤套装 + 关键情绪动画先到位，其余渐进交付 |
| **Resource** | 真机测试归 Epic 9 延后 | MVP 发布前 Epic 9 真机 smoke 为硬门（`CLAUDE-21.6` + Round 3 Murat） |
| **Resource** | 跨端契约漂移（server/iOS/Watch 三仓） | 三仓双 gate 漂移守门（`CLAUDE-21.1`）；fixture ↔ openapi/registry 对比脚本归后续 epic |
| **Technical** | **Server 权威解锁服务单点**（Round 6 Winston）——server 解锁挂 = 全量用户无法领盒 | 解锁结果 **Redis + Mongo 双写**，读降级到 Mongo；fail-closed UI "盲盒暂时无法打开，稍后重试"；对应 `S-SRV-14`（server-driven 反馈） |
| **Technical** | **多设备 idle 聚合漂移**（Round 6 Winston）——iPhone/Watch 各算各的 idle 会误触召唤 | Server 权威聚合 `max(iPhone_last_active, Watch_last_active)`（见 Cross-Device Messaging Contract · Idle Timer 聚合 + `S-SRV-13`） |

## Functional Requirements

**能力契约（binding）**：PRD 后续 Epic / Story 只能 cite `FR-*` 或 `C-*` ID，不能 cite 散文。UX 只设计这里列出的，Architect 只支撑这里列出的。

**Test 标签约定**（Round 5 Murat）：
- `[U]` = 单测覆盖（`xcodebuild test` 一命令绿）
- `[I]` = 集成 fake 覆盖（`WatchTransport` / mock HealthKit / fake WS 等）
- `[E9]` = Epic 9 真机 smoke（真机 / 真 Watch 配对 / 视觉 / 感官验证）
- 未标记 = story review 必退回

### 1. 账号 & Onboarding（C-ONB-* + C-UX-01/02 + C-OPS-06）

- **FR1**：User 可通过 Sign in with Apple 登录 `[U, I]`
- **FR2**：有 Apple Watch 的 User 可在 onboarding **引导去 App Store 安装 CatWatch app**，Watch 端独立完成 Sign in with Apple，Server 根据 SIWA user_id 自动关联双端 `[I (FakeWatchTransport as fast-path only), E9]`
- **FR3**：无 Apple Watch 的 User 可选观察者模式继续 onboarding，不被阻断 `[U, I]`
- **FR4**：User 可授权 HealthKit 读取步数 `[I (mock HealthKit), E9]`
- **FR5**：User 可拒绝 HealthKit 授权，app 仍可进入（功能降级，见 FR44）`[U, I]`
- **FR6**：User 在 SIWA 登录前可浏览 3 屏叙事 Onboarding 卡片（含 Slogan "你的猫在家等你走到它身边"）`[U, I]`（Round 5 Sally+Carson 合并 → `C-UX-01`）
- **FR7**：User 首次完成 Watch 配对（或选观察者模式）后进入**撕包装首开仪式**（盒子震动 → 猫跳出 → User 为猫命名）`[I, E9]`（Round 5 Carson → `C-UX-02`）
- **FR8**：User 可请求删除账号（SIWA + GDPR 合规，联动 server `S-SRV-12`）`[I, E9]`（Round 5 Sally → `C-OPS-06`）
- **FR9**：User 可导出个人数据（GDPR right to data portability，联动 `S-SRV-12`）`[I]`
- **FR10**：User 可主动 sign out 并清除本地 token（Keychain 清理）`[U, I]`
- **FR10a**：System 将账号级里程碑（`onboarding_completed` / `first_emote_sent` / `first_room_entered[room_id]` / `first_craft_completed`）存储在 **server `user_milestones` collection**（联动 `S-SRV-15`），**禁止使用 UserDefaults 作为权威源**（保证换机/重装无缝恢复）`[U, I]` → `C-ONB-01`

### 2. 好友（C-SOC-03 + C-UX-05）

- **FR11**：User 可发送好友请求 `[U, I]`
- **FR12**：User 可接受 / 拒绝收到的好友请求 `[U, I]`
- **FR13**：User 可查看好友列表并管理关系 `[U, I]`
- **FR14**：User 可屏蔽 / 移除一个好友 `[U, I]`
- **FR15**：User 可**静音**某位好友的表情广播（不收 push / 震动，但保留好友关系）`[U, I]`（Round 5 Sally → `C-UX-05`）
- **FR16**：User 可举报某位好友或其表情内容 `[I, E9]`（Round 5 Sally → `C-UX-05`）

### 3. 房间 + 社交广播（C-ROOM-* + C-SOC-01/02 + C-ME-01）

- **FR17a**：User 可创建房间 `[U, I]`（FR10 拆分之一，Round 5 Amelia）
- **FR17b**：User 可加入 / 离开房间 `[U, I]`
- **FR17c**：User 是房主时离开需**转让或解散**（体面退房，Sally `C-UX-05`）`[U, I]`
- **FR18**：User 可通过邀请链接 / 二维码邀请好友入房 `[I, E9]`
- **FR19**：邀请链接含失败分支：过期 / 已是好友 / 已在房间 / 房间满 / 自己邀自己——UI 显式提示 `[U, I]`（Round 5 Amelia）
- **FR20**：User 可通过邀请链接加入房间 `[I]`
- **FR21**：iPhone 端 User 可查看房间成员名单（姓名 / 在线或挂机状态 / 一句话叙事文案）`[U, I]`
- **FR22**：叙事文案合成引擎存在**规则表**（trigger → template_id）供 AC 精确断言 `[U]`（Round 5 Murat + Amelia）
- **FR23**：Watch User 可在房间内看到其他成员的猫同屏渲染（2–4 人）`[I (fake assets), E9]`（FR16 拆分）
- **FR24**：房间 2+ 成员同时走路时，Watch 端触发环绕跑酷特效（**server 判定下发**，Watch 禁本地计时触发）`[U (fake server events), I, E9]`
- **FR25**：观察者 User 隐私 gate——只见 `current_activity` + `minutes_since_last_state_change`，不见 `exact_step_count` / `health_*` `[U, I]`
- **FR26**：User 可点击自己的猫（iPhone "我的猫"卡片 / Watch 端猫本体）弹出表情选单 `[U, I]`
- **FR27**：User 选表情后，自己的猫旁浮动对应 emoji（本地立即呈现）`[I, E9]`
- **FR28**：User 在房间内发表情时，server 广播给房间成员；对端 Watch 轻震 + emoji 浮在 emitter 猫旁；iPhone 观察者走 push 通道 `[U, I, E9]`
- **FR29**：User 不在房间时发表情仅本地呈现，不发 server fan-out（但仍走 server 去重落日志）`[U, I]`
- **FR30**：表情广播为 fire-and-forget——发送方无"已读/已送达"UI 状态，对端自治呈现 `[U]`

### 4. 步数（C-STEPS-*）

- **FR31**：System 从 HealthKit 本地读取步数用于展示 `[I (mock HealthKit), E9]`
- **FR32**：System 使用 server 权威步数判定盲盒解锁（**iPhone 本地不做解锁判定**，防作弊 + 防事实源漂移）`[U, I]`（遵 `CLAUDE-21.4`）
- **FR33**：User 可在 iPhone 查看步数历史图表（联动 `S-SRV-9`）`[U, I]`
- **FR34**：WS 断线时，盲盒解锁 UI 显示"已达成·等待确认"占位，不 pre-empt server `[U, I]`

### 5. 经济：盲盒 + 合成 + 装扮（C-BOX-* + C-SKIN-* + C-DRESS-*）

- **FR35a**：System 在 User 在线挂机满 30 min 后掉落盲盒（server 权威判定 + `ClockProtocol` 抽象供单测 fast-forward）`[U (mock clock), I, E9]`（FR27 拆分，Round 5 Amelia+Murat）
  - **ClockProtocol mock 契约**：Story 层需定义 `advance(by:) / setNow(_:)` 语义、timer 重入、跨 actor 线程模型（Round 6 Amelia 登记，详情见 frontmatter `openQuestionsForVision`）
- **FR35b**：User 可消耗 1000 步打开已掉落的盲盒 `[U, I]`
- **FR35c**：Watch User 解锁盲盒时看到开箱动画 + 皮肤揭晓 `[I (fake assets), E9 (视觉)]`
- **FR35d**：Watch 不可达时盲盒走满 1000 步进入 **`unlocked_pending_reveal` 中间态**（联动 `S-SRV-16`）；iPhone 仓库 UI 显示"已解锁·待揭晓（等待 Watch 上线）"占位；Watch 下次可达时 server 推送 `box.unlock.revealed` 事件触发开箱动画 `[U (fake Watch reachability), I]`
- **FR36**：~~原文：iPhone Push 不剧透具体皮肤名~~ —— **已作废**（v2 修订 1：盲盒完全无通知，此 FR 前提不成立）
- **FR37**：System 为每个皮肤标注颜色分级（候选：白 < 灰 < 蓝 < 绿 < 紫 < 橙）`[U]`
- **FR38**：盲盒开出重复皮肤时，System 自动归为合成材料（文案永远"材料入库"）`[U, I]`
- **FR39**：User 可在 iPhone 仓库查看全部皮肤 + 合成材料计数 `[U, I]`
- **FR40**：User 可在材料满足合成规则（候选 5 低级 → 1 高级）时触发合成（联动 `S-SRV-10`，含中途断网 / 材料不足 / 并发合成幂等）`[U, I]`
- **FR41**：User 可在 iPhone 大屏选择装扮给自己的猫 `[U, I]`
- **FR42**：装扮 payoff 由 Watch 端渲染 `[I, E9]`
- **FR43**：System 跨天 / 跨时区正确重置挂机计时基准（`ClockProtocol` 供测试）`[U]`（Round 5 Carson）

### 6. 陪伴反馈：久坐召唤（C-IDLE-01..02，**无情绪机**）

- **FR44**：System 在 Watch 端维护**猫动作同步**（跟随 Watch 物理状态：走路 / 睡觉 / 挂机各自动画；**无情绪 layer**）`[I, E9]`
- **FR45a**：User 无活动 2h 后，Watch 主动 haptic 召唤 User 起身（`UIImpactFeedbackGenerator` 参数异于盲盒通知）`[U]`（Round 5 Murat 拆分，参数断言）
- **FR45b**：真机上人类可辨识久坐召唤与盲盒震感差异 `[E9]`
- **FR46**：User 起身走动 30s 后，Watch 端猫做欢迎动作（**纯交互触发**，非情绪状态转移）`[I, E9]`
- （原 FR37 情绪状态机 / FR40 iPhone 情绪文案 **已删除**——Round 5 决定移除情绪机，猫只做物理动作同步 + 久坐召唤纯交互）

### 7. 容错（C-REC-*）

- **FR47**：WS 断线后 System 重连并按 `last_ack_seq` 补发遗漏事件 `[U (fake WS), I]`
- **FR48**：Server 待推事件 TTL = 24h；超时 GC 后重连，System 显示 fail-closed 提示"部分事件已过期，请刷新"`[U (fake WS + time jump), I]`
- **FR49**：`/v1/platform/ws-registry` 启动失败时，System 屏蔽主功能 + 显示"网络异常，请稍后重试"（fail-closed）`[U, I]`
- **FR50**：HealthKit 授权被拒时，步数 UI 回退为 "—" + 引导条"去设置开启权限" `[U, I]`

### 8. UX 生存层（C-UX-03..04）

- **FR51**：全局空状态——仓库空（"开第一个盲盒认识你的猫"CTA）/ 好友空（"扫码把 TA 拉回家"）/ 房间无广播（引导发第一条表情）`[U, I]`（Round 5 Sally → `C-UX-03`）
- **FR52**：User 可编辑昵称 / 头像 / 个性签名 `[U, I]`（Round 5 Sally → `C-UX-04`）
- **FR53**：User 可在"设置"入口查看隐私政策 / 使用条款 / 账号信息 `[U]`

### 9. 跨端消息 + 推送 + 隐私 + 可观测性（C-SYNC-* + C-OPS-*）

- **FR54**：所有跨端消息带 `client_msg_id`（UUID v4）供 server 60s 去重 `[U]`
- **FR55**：User 可分频道开/关 push（好友 / 表情）`[U, I]`（盲盒频道已移除——v2 修订 1：盲盒全程无通知）
- **FR56**：System 通过 `wss://`（TLS）加密 WS 流量；`ws://` 仅限 debug build 豁免 `[U (config check), E9]`
- **FR57**：System 将 JWT / refresh token 存储于 Keychain（`kSecClassGenericPassword`） `[U]`
- **FR58**：System 不上传 raw step count 至 server（仅发送解锁 ACK / 状态） `[U, I]`
- **FR59**：System 带崩溃上报（MVP：Xcode Organizer 基础；Growth：升级 Crashlytics/Sentry）`[I, E9]` → `C-OPS-01`（**Round 6 Winston：拆分后 MVP 只做基础能力，升级推 Growth**）
- **FR60**：System 启动时做版本检查（MVP：App Store 原生弹窗；Growth：in-app 强更）`[U, I]` → `C-OPS-02`（**Round 6 推 Growth 升级**）
- **FR61**：System 带远程配置 / 功能开关 / kill switch（MVP：硬编码配置；Growth：引入远程配置平台）`[U, I]` → `C-OPS-03`（**Round 6 推 Growth 升级**）
- **FR62**：System 以 `Log.*` facade 做结构化日志上报（对齐 `PCT-arch` `Log.network/ui/spine/health/watch/ws`）`[U]` → `C-OPS-04`
- **FR63**：App Bundle 带 `PrivacyInfo.xcprivacy` 隐私清单（iOS 17 硬要求，2024/03 起 App Store 强制；**必进 MVP**）`[U (build artifact check), E9 (上架审核)]` → `C-OPS-05`

---

### Self-Validation（Round 5 四 agent 挑战后）

- ✅ **Amelia 糅合 FR 已拆**：FR10→FR17a/b/c；FR16→FR23/24/22；FR27→FR35a/b/c
- ✅ **Amelia server 契约空洞**：新增 `S-SRV-8..12`（box.drop / steps/history / craft / emote / account-delete）
- ✅ **Amelia 边界态**：FR19（邀请链接失败分支）/ FR40（合成幂等）/ FR50（HealthKit 拒绝）已显式
- ✅ **Amelia dev 必备**：C-OPS-01..06 全部进 MVP
- ✅ **Sally UX 生存**：FR6/7（叙事 + 撕包装）/ FR15/16/17c（负面社交）/ FR51（空状态）/ FR52（identity）/ FR8/9（删除/导出）
- ✅ **Carson everyday**：FR7 撕包装 / FR43 跨时区 / Memory Feed 推 Growth；**极端情绪态已移除**（Round 5 用户决策）
- ✅ **Murat testability 标签制**：每条 FR 带 `[U/I/E9]`；`ClockProtocol` / `IdleTimerProtocol` / `FakeWatchTransport` 写入 AC 约束；感官验证（FR45b / FR35c）显式归 E9
- ⚠️ **仍未覆盖**：AC 层每条 FR 的具体断言脚本——待 Step 10+ NFR / Story 层做

## Non-Functional Requirements

**原则**：只列**未被其他章节覆盖**的新 NFR；Success Criteria · Technical Success / Cross-Device Messaging Contract / Mobile App Specific Requirements 已覆盖部分本节以引用形式简述。

### Performance（补 Success Criteria 未覆盖）

- **UI 响应**：点猫弹表情选单 < **200ms**；tab 切换 < **100ms**（iPhone 14/15/16 基线）
- **冷启动**：App 首次启动到首屏可交互 < **2s**
- **Spine 动画**：Watch 前台 **60fps** / 后台 **15fps**（`KWM-product` 原文）；帧率不达自动降级，不崩溃
- **HealthKit 查询缓存**：按天聚合缓存 **15min**，禁秒级重查（遵 `PCT-arch`）
- **列表性能**：仓库皮肤数 > 200 用 `LazyVStack / List` with stable `id:`，禁 `ForEach(array)` 暴力展开

### Security

- **传输加密**：WS `wss://` + ATS 合规；`ws://` 仅限 debug build 特定联调域名豁免（FR56）
- **Token 存储**：JWT / refresh token 存 Keychain `kSecClassGenericPassword`，**禁 UserDefaults**（FR57）
- **隐私最小化**：Server 不收 raw step count，仅拿解锁 ACK + 状态（FR58）
- **观察者隐私 gate**：`pet.state.broadcast` 字段按订阅者类型裁剪（FR25 + Cross-Device Messaging Contract）
- **SIWA nonce**：请求前随机生成，回调校验，防 replay
- **敏感日志**：release build 禁明文 token / email；ID 可打，token 需 hash（遵 `PCT-arch`）
- **设备标识**：`identifierForVendor`，**禁 IDFA**
- **Deep link**：严格校验 scheme + 参数；禁直接执行 URL 参数

### Reliability

- **崩溃率** ≤ **0.2%**（Crashlytics 口径；已见 Success Criteria · Technical Success）
- **WS 重连**：网络切换后 10s 内 ≥ **95%** 成功率（同上）
- **事件 TTL**：server 待推事件 TTL = **24h**（FR48 + Cross-Device Messaging Contract）
- **启动 fail-closed**：`/v1/platform/ws-registry` 失败屏蔽主功能（FR49）
- **数据一致性**：盲盒解锁 server 权威 + `client_msg_id` 60s 去重（FR32 + FR54）
- **离线降级**：三档离线策略文档化（完全离线 / WS 断 / 启动失败），见 Mobile App Requirements · Offline Mode

### Accessibility

- **VoiceOver**：全 interactive UI 元素带无障碍标签（Apple 审核高频要求）
- **Dynamic Type**：文字尺寸支持系统动态字体
- **色盲友好**：皮肤颜色分级**必须配合形状或文字标签**（`C-SKIN-01` 不能仅靠颜色区分稀有度）
- **Haptic 作为补充而非替代**：所有触觉信号同时有视觉信号兜底（听障 / 视障用户可用）
- **Onboarding 可跳过**：3 屏叙事卡片（FR6）和撕包装仪式（FR7）允许 skip，不强制

### Scalability（MVP 预期 + 客户端约束）

- **MVP 装机量预期**：内测 100 人 → TestFlight 公测 1 万 → 正式版 10 万（客户端对 server 契约无特殊压力；scalability 主责在 server 侧）
- **客户端并发约束**：单用户最多同时在 1 个房间；单房间 max **4 人**；MVP 好友数上限 **50**（Growth 阶段放开）
- **WS 负载**：单连接共享 envelope；iPhone / Watch 任一端在线都使用同一 session（server 会话接管策略见 Cross-Device Messaging Contract）
- **冷启动网络**：仅 `/v1/platform/ws-registry`（必）+ 可选 health check；**不做**全量 state 拉取（按需加载）

### Integration

- **HealthKit**（读 only）：`HKStatisticsCollectionQuery` 按天聚合 stepCount；增量走 `HKObserverQuery + HKAnchoredObjectQuery`；**禁** 高频 `HKSampleQuery`（耗电 + 权限可能被吊销）
- **WatchConnectivity**（抽象 `WatchTransport` · **架构哲学 B：Server 为主，WC 为辅**）：**MVP 不使用 WC 做核心交互**——所有跨端状态同步（盲盒、装扮、表情、房间、步数）走 `server ↔ iPhone` + `server ↔ Watch` 两条独立 WSS 链路；WC 仅作为 **fast-path 优化层**（局域网双端都前台时的加速通道，可选），非 MVP 必需。三档通道若启用则用途钉死——
  - `updateApplicationContext`：最新状态覆盖（当前皮肤 / 当前房间 ID），仅做加速
  - `transferUserInfo`：队列化事件补偿，仅做加速
  - `sendMessage`：实时双工，仅双方前台可用，非关键路径
  - **禁** `transferFile` 走 JSON
  - **禁** 把 WC 当作核心通路（核心交互必须 server 权威 + WS 双链路）
- **APNs**：标准 iOS 推送，分频道开关（FR55）
- **SpriteKit + Spine**：`esoteric-software/spine-runtimes` 通过 SwiftPM 拉取并锁版本；仅 Watch 侧密集使用，iPhone 端静态 avatar 不依赖 Spine 动画
- **SIWA**：标准接入；若未来加第三方 OAuth，SIWA 仍需并列（App Store 4.8）
- **跨端契约锚点**（遵 `OAPI-0.14.0` + `WSR-v1`）：OpenAPI 0.14.0-epic0 + WS registry apiVersion v1；漂移以 server 为准（`PCT-arch`）

## Server-Driven New Stories（对 server PRD 的反馈清单）

本 iPhone PRD 暴露了 server 侧需要新增的 stories，列于此以便跨仓联调。**不在本 PRD 范围实现**，但需要跨仓沟通锁定契约。

- **S-SRV-1**：`pet.state.broadcast` 消息（含他人状态 + 隐私 gate 字段分级）—— derived from `C-ROOM-01/02`, `FR21`, `FR25`
- **S-SRV-2**：`pet.emote.broadcast` fan-out + fire-and-forget 语义（60s 去重 + 房间订阅者 fan-out，不回发送方 ack）—— derived from `C-SOC-01/02`, `FR28/30`
- **S-SRV-3**：`room.effect.parkour` 等同屏特效 server 判定下发（Watch 禁本地计时触发）—— derived from `C-ROOM-04`, `FR24`
- **S-SRV-4**：待推队列 TTL=24h + GC 审计日志 —— derived from `C-SYNC-*`, Cross-Device Messaging Contract TTL, `FR48`
- **S-SRV-5**：皮肤合成系统（材料计数 + 合成规则引擎 + 事务性 ack）—— derived from `C-SKIN-02`, `FR40`
- **S-SRV-6**：久坐召唤触发事件（`time-since-last-activity` 阈值 + haptic trigger event；**无情绪状态机**，纯 idle timer）—— derived from `C-IDLE-02`, `FR45a`
- **S-SRV-7**：盲盒颜色分级 + 初级掉落概率披露（loot-box 合规最小面，即使非付费也披露）—— derived from `C-SKIN-01`, `FR37`, Store Compliance
- **S-SRV-8**：`box.drop` WS event + `/boxes/pending` API（盲盒 pending 状态查询）—— derived from `C-BOX-01/04`, `FR35a`
- **S-SRV-9**：`/steps/history?range=` 历史查询 API —— derived from `C-STEPS-01`, `FR33`
- **S-SRV-10**：合成 `/craft` 幂等端点 + `materials` 字段（含中途断网 / 材料不足 / 并发双端合成幂等 + 扇出上限 / 节流配置）—— derived from `C-SKIN-02`, `FR38/40`
- **S-SRV-11**：`cat.tap` → `pet.emote.broadcast` WS schema 明确化（payload 字段 + rate limit + 无房间时 drop 行为）—— derived from `C-SOC-01`, `FR26/28/29`
- **S-SRV-12**：账户删除 + 数据导出 API（SIWA + GDPR 合规）—— derived from `C-OPS-06`, `FR8/9`
- **S-SRV-13**（Round 6 Winston 新增）：**Idle Timer 权威聚合** —— server 聚合 `max(iPhone_last_active, Watch_last_active)`；冷启动后客户端从 server 拉 `last_active_ts`，不依赖 background task。Derived from `C-IDLE-02`, Cross-Device Messaging Contract · Idle Timer 聚合
- **S-SRV-14**（Round 6 Winston 新增）：**解锁结果持久化降级** —— Redis（热路径） + Mongo（降级路径）双写；Redis 挂时读 Mongo，UI 提示"盲盒暂时无法打开，稍后重试"。Derived from `C-BOX-01/04`, Risk Mitigation · Technical
- **S-SRV-15**（v2 · UX Step 10 Party Mode 新增）：**`user_milestones` collection + API** —— 承载账号级里程碑（onboarding_completed / first_emote_sent / first_room_entered[room_id] / first_craft_completed），替代客户端 UserDefaults，保证换机无缝恢复。详细契约见 `server-handoff-ux-step10-2026-04-20.md`。Derived from UX §Party Mode A3
- **S-SRV-16**（v2 · UX Step 10 Party Mode 新增）：**`box.state` 新增 `unlocked_pending_reveal` 态 + Watch 重连触发揭晓** —— Watch 不可达时盲盒已走满 1000 步但未揭晓，进入中间态；Watch 下次可达时 server 主动推送 `box.unlock.revealed` 事件触发 Watch 开箱动画。Derived from UX §Party Mode A5'
- **S-SRV-17**（v2 · UX Step 10 Party Mode 新增）：**取消 `emote.delivered` 发送者 ack 推送** —— fire-and-forget 对称性硬约束，发送者 POST `/emote` 返回 202 即终结，server 不向发送者推送 delivery ack（仅 fan-out 给接收方）。Derived from UX §Party Mode A4
- **S-SRV-18**（v2 · UX Step 10 Party Mode 新增）：**所有 fail 节点 Prometheus metric 打点** —— `ws_ack_timeout_total` / `box_unlock_timeout_total` / `craft_fail_total` / `registry_fetch_failed_total` / `room_invite_expired_total` / `milestone_update_conflict_total` / `box_unlocked_pending_reveal_count` / `watch_reachability_false_ratio` 等。Derived from UX §Party Mode B1
