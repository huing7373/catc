---
stepsCompleted:
  - step-01-init
  - step-01b-continue
  - step-02-discovery
  - step-02b-vision
  - step-02c-executive-summary
  - step-03-success
  - step-04-journeys
  - step-05-domain
  - step-06-innovation
  - step-07-project-type
  - step-08-scoping
  - step-09-functional
  - step-10-nonfunctional
  - step-11-polish
  - step-12-complete
inputDocuments:
  - C:/fork/cat/裤衩猫.md
  - C:/fork/cat/README.md
  - C:/fork/cat/docs/backend-architecture-guide.md
  - C:/fork/cat/document/联机同步设计稿.md
  - C:/fork/cat/document/Spine到AppleWatch资源导出方案.md
documentCounts:
  briefs: 1
  research: 0
  brainstorming: 0
  projectDocs: 3
workflowType: 'prd'
classification:
  projectType: api_backend
  projectSubtype: realtime_social_backend
  projectContext: greenfield
  priorArtifacts:
    - "Story 2-1 Postgres/GORM prototype (discarded 2026-04-15)"
  domain: social
  domainSubtag: ambient_presence_companion
  domainConcerns:
    - friend_pairing_abuse
    - touch_anti_harassment
    - minor_privacy_minimization
    - cross_timezone_quiet_hours
    - presence_decay_truthfulness
    - minor_verification_in_cn
    - health_data_pipl_classification
    - healthkit_data_use_scope_compliance
    - account_deletion_cascade
    - explicit_block_enforcement
  complexity: medium-high
  complexityJustification: |
    (1) WebSocket framework is new infrastructure (~2000 lines protocol glue);
    (2) Multi-device WS multiplexing (Watch + iPhone) needs careful design;
    (3) session_resume reconnect correctness + perf requires dedicated testing;
    (4) APNs fallback path requires cross-protocol event idempotency.
  complexityDrivers:
    - multi_device_sync
    - websocket_framework
    - apns_tiered_push
    - fsm
    - anti_cheat
    - snapshot_decay_engine
    - session_resume
  coreModes:
    state_mirror: 0.40
    rpc_over_ws: 0.40
    event_relay: 0.20
  protocolContract:
    primaryChannel: websocket
    http:
      role: "bootstrap + external_integration + static + infrastructure"
      endpoints:
        - "POST /auth/apple"
        - "POST /auth/refresh"
        - "POST /devices/apns-token"
        - "POST /state  # iPhone background HealthKit, 30s window"
        - "GET /healthz, /readyz"
      outboundHttp:
        - "APNs Provider API (HTTP/2)"
        - "Sign in with Apple JWK"
      cdn: "skin_assets (external CDN)"
    websocket:
      role: all_post_login_user_actions
      endpoint: /ws
      lifecycle: foreground_session
      connection_multiplexing: per_device
      startup_strategy: "cache_first + background_resume"
      envelope: "{id, type, payload | ok/error}"
      reliability: "request_response + ack + retry + idempotency_dedup"
    apns:
      role: background_wake
      tiers: [alert, silent]
  supplementaryConcerns:
    - websocket_framework
    - apns_tiered_push
    - protocol_fallback
    - event_idempotency
    - scheduled_jobs
    - snapshot_decay
    - offline_replayability
    - session_resume
  scalabilityProfile:
    mvpTarget: single_binary_single_instance
    designForMultiReplica: true
    growthPath:
      - horizontal_ws_with_sticky_routing
      - cron_to_persistent_queue
      - push_queue_persistence
    explicitlyNOT: microservices_decomposition
---

# Product Requirements Document - server

**Author:** 开发者
**Date:** 2026-04-15

## Executive Summary

裤衩猫后端是一套服务于 Apple Watch + iPhone 的实时社交伴侣后端（`realtime_social_backend`），定位为产品的**"存在感引擎"** —— 让好友之间的虚拟猫实时映射各自真实运动状态（idle / walking / running / sleeping），构建无需主动打招呼即可感知对方存在的**环境在场感**（ambient co-presence）。

**产品背景**：Apple Watch 陪伴产品赛道当前为空白，裤衩猫抢占首发生态位。核心用户为久坐办公族（"起身的理由"）、萌系收集癖（盲盒皮肤驱动）和异地亲密关系（"不打扰也能感觉你在"）。产品打**治愈牌**而非健康牌，弱化期待、降低心理负担。

**服务端核心职责（按产品价值降序）**：
1. **状态镜像低延迟广播** —— 维护每用户猫状态快照，在好友房间内以 15 秒内时效广播 FSM 状态切换，驱动"共享水族馆"式的环境在场体验
2. **触碰中继与送达保障** —— 处理好友间的触觉社交事件（表情震动），前台走 WebSocket，后台降级 APNs，确保送达
3. **盲盒权属强一致** —— 步数兑换、盲盒领取、皮肤解锁的服务端确权，防多端重复领取和客户端作弊

**协议架构**：登录后以 WebSocket 为主通道（per-device 连接、envelope 化请求-响应 + ACK 重试 + 幂等去重），HTTP 仅服务 bootstrap（Sign in with Apple / token 刷新 / APNs token 注册）、iPhone 后台 HealthKit 状态上报（30 秒窗口特例）和基础设施探针。APNs 承担后台设备唤醒（普通推送 + 静默推送）。

**技术约束**：遵循 `docs/backend-architecture-guide.md` 宪法 —— Go + Gin + MongoDB + Redis + zerolog + TOML，P2 风格分层单体，显式依赖注入，无微服务拆分。代码从第一行起按多副本假设编写（连接状态入 Redis、cron 加分布式锁、推送走持久化队列），为 10k+ 在线 WS 连接的水平扩展预留路径。

### What Makes This Special

1. **平台首发红利** —— Apple Watch 上不存在同类陪伴产品，先发意味着用户心智和社交网络效应先于竞品建立
2. **在场 > 通信** —— 不同于 IM / 视频社交卖"触达"，裤衩猫卖的是"不打招呼也能感觉你在活着" —— 好友的猫在走路，意味着 TA 正在通勤；好友的猫在睡觉，意味着 TA 已经休息了。这是一种零认知负担的亲密感。
3. **运动即陪伴货币** —— 把步数从健康指标转译为社交存在感和盲盒兑换资源，回避"健身 App"心智，拥抱"治愈陪伴"心智

**服务端作为护城河**：好友图、触碰历史、共同目标进度、盲盒收集记录构成的**社交网络效应**随用户使用时长持续加深 —— 这些状态全部由服务端托管，是竞品无法简单复制的壁垒。

### Project Classification

| 维度 | 值 | 备注 |
|---|---|---|
| Project Type | `api_backend` / `realtime_social_backend` | REST bootstrap + WebSocket 主通道 + APNs 后台唤醒 |
| Domain | `social` / `ambient_presence_companion` | 环境在场式轻社交，非 IM |
| Complexity | **medium-high** | 7 项驱动因素：multi_device_sync、websocket_framework、apns_tiered_push、fsm、anti_cheat、snapshot_decay_engine、session_resume |
| Project Context | **greenfield** | Story 2-1 Postgres/GORM 原型已废弃（2026-04-15），按架构宪法从零搭建 |
| Core Modes | state_mirror 40% / rpc_over_ws 40% / event_relay 20% | 状态镜像 + WS RPC + 事件中继三本柱 |
| Scalability | 单体单实例（MVP） → 多副本 sticky routing（成长期） | 代码按多副本设计，显式排除微服务拆分 |
| Domain Concerns | 10 项 | friend_pairing_abuse、touch_anti_harassment、minor_privacy_minimization、cross_timezone_quiet_hours、presence_decay_truthfulness、minor_verification_in_cn、health_data_pipl_classification、healthkit_data_use_scope_compliance、account_deletion_cascade、explicit_block_enforcement |

## Success Criteria

### User Success

**核心成功指标 —— "用户进入房间并留下"：**

| 指标 | 定义 | MVP 目标值 |
|---|---|---|
| 首次入房时间 | 注册到第一次加入好友房间的时长 | ≤ 72 小时（前提：用户需先配对至少 1 个好友） |
| 单次在房时长 | 一次进入房间到离开的中位数 | ≥ 5 分钟 |
| 日均在房时长 | 每日所有在房时间累计中位数 | ≥ 15 分钟 |
| 房间好友数 | 用户进入房间时平均可见好友数 | ≥ 1 人（≥ 2 为健康） |
| 好友配对率 | 注册 7 天内成功配对至少 1 个好友的用户比例 | ≥ 60% |

**"啊哈时刻"定义**：用户第一次进入房间，看到好友的猫处于非 idle 状态（walking / running / sleeping）—— "TA 真的在那边活着"。

### Business Success

| 指标 | 定义 | 3 个月目标 | 12 个月目标 |
|---|---|---|---|
| DAU | 当日打开 App 的独立用户数 | 1,000 | 10,000 |
| 次日留存 | D1 retention | ≥ 35% | ≥ 45% |
| 7 日留存 | D7 retention | ≥ 15% | ≥ 25% |
| DAU/MAU | 粘性比 | ≥ 20% | ≥ 30% |

> 注：以上数值为消费级社交 App 的行业基准估算。首发窗口期内，**DAU 绝对值不如留存率重要** —— 用户少但留得住 > 用户多但流失快。

### Technical Success

| 指标 | SLA | 说明 |
|---|---|---|
| 好友状态广播延迟 | p99 ≤ 3 秒（房间内 WS） | 产品心脏的延迟底线 |
| 状态衰减准确性 | 0-15s 档误判率 < 5% | 保证"刚刚还在走"的真实感 |
| 触碰送达率 | ≥ 99%（WS + APNs 联合） | 跨协议降级后的最终送达 |
| 盲盒重复领取 | 0 次 | 强一致确权，零容忍 |
| WS 重连成功率 | ≥ 98%（5 秒内） | 抬腕到有效画面的体验底线 |
| 服务端可用性 | ≥ 99.5%（月度） | MVP 阶段允许短时维护窗口 |
| API 响应时间（HTTP） | p95 ≤ 200ms | bootstrap + HealthKit 上报 |

### Measurable Outcomes

**MVP 上线 30 天验证清单：**
1. 有用户在房间里看到好友猫的真实状态并发出第一次 touch
2. 次日留存 ≥ 35% 且无持续下滑趋势
3. 盲盒系统零作弊 / 零重复领取事故
4. WS 连接在 Apple Watch dim → 抬腕场景下 5 秒内恢复
5. 好友配对流程端到端完成率 ≥ 80%（从邀请链接到双方确认）

## Product Scope

### MVP - Minimum Viable Product

| 功能模块 | 服务端交付物 |
|---|---|
| **鉴权** | Sign in with Apple 登录 + JWT access/refresh + APNs token 注册 |
| **猫状态系统** | 状态上报（WS `state.tick` + HTTP `/state`）、快照存储（Mongo + Redis）、FSM 服务端推断（idle 超时） |
| **好友房间** | WS 房间 join/leave、`room.snapshot` 初始推送、`friend.state` 实时广播、状态衰减引擎（4 档） |
| **触碰** | WS `touch.send` / `friend.touch` 广播、APNs 降级、频次限流（反骚扰）、屏蔽执行 |
| **盲盒** | 掉落触发（挂机 30 分钟 / cron）、步数兑换确权（`blindbox.redeem`）、库存查询、服务端审计 |
| **好友系统** | 邀请 token 生成 / 接受 / 解除、好友列表、屏蔽 |
| **皮肤基础** | 皮肤目录（默认猫 + MVP 少量皮肤）、解锁记录、装配 |
| **基础设施** | WS envelope 框架（ACK / retry / dedup）、session_resume、健康检查、zerolog 结构化日志 |

### Growth Features (Post-MVP)

| 功能 | 服务端交付物 |
|---|---|
| 日历签到 | `calendar.checkin` WS RPC + 每日奖励逻辑 + 连续签到追踪 |
| 皮肤换装系统（分层贴图） | 换装组合验证、部位冲突检查、CDN manifest 管理 |
| 共同目标 | 好友组队步数累计、阈值结算（cron）、金礼盒奖励 |
| 跨时空表情（多类型） | 表情类型注册表、动画资源关联、表情解锁 |
| 静默推送刷新 | APNs 静默推送策略、配额管理 |
| 账号删除级联 | GDPR / PIPL 合规的好友图 / 快照 / 历史全清理 |

### Vision (Future)

| 功能 | 说明 |
|---|---|
| Complication 表盘 | 服务端推送 complication 更新数据 |
| 未成年人认证流程 | `minor_verification_in_cn` 完整实现 |
| 水平扩展部署 | WS sticky routing + cron 持久化队列 + push 队列 |
| 数据导出（GDPR Art 20） | 用户数据打包下载 |

## User Journeys

### J1：小鱼 & 小周 —— "1000 公里外的猫在跑步"

**人物**：小鱼（25 岁，上海互联网公司设计师）和小周（26 岁，成都游戏公司程序员）。异地恋一年半，每天微信视频太累，但不联系又焦虑。两人都戴 Apple Watch。

**开场**：周五晚上，小鱼在小红书看到裤衩猫的推荐帖："手表上养猫，能看到男朋友的猫在干嘛"。她下载后用 Sign in with Apple 注册（30 秒完成），然后把邀请链接发给小周。

> **服务端需求**：`POST /auth/apple` → JWT 签发 → 首次 WS 连接 → `session.resume` 返回空好友列表 + 默认猫皮肤

**升温**：小周点开 Universal Link，App 自动跳转注册 + 接受好友邀请。两人各自的猫出现在彼此屏幕上。小鱼进入房间，看到小周的猫 —— 状态是 `idle`（他在工位写代码）。她点了一下小周的猫，发送"比心哦" —— 小周手腕震动，抬手看到小鱼的猫比了个心。

> **服务端需求**：好友邀请 token 校验 + `friend.accept` WS RPC → 双向好友关系写入 → `room.snapshot`（2 只猫）→ `touch.send` → 对端 WS 在线 → `friend.touch` 推送 + 震动

**高潮**：周六上午，小鱼去公园散步，打开房间 —— 小周的猫在 `sleeping`（成都早上 10 点他还没起）。她笑了，继续走。半小时后再看，小周的猫变成 `walking` —— "他起来遛狗了"。没有一条消息，但她知道他醒了。

> **服务端需求**：`state.tick`（小周端）→ 快照更新（Mongo + Redis）→ `friend.state` 广播到小鱼的 WS → 状态衰减引擎（15 秒内小鱼看到真实状态）

**结局**：一个月后，两人的猫各自有了不同的皮肤（小鱼收集了 12 个盲盒，小周 8 个）。每天不打视频电话，但在通勤、午休、睡前会打开房间瞥一眼对方的猫。"虽然在不同城市加班，但我们正在一起养猫。"

> **服务端需求**：盲盒掉落（cron 30 分钟触发）→ `blindbox.redeem`（步数确权）→ 皮肤解锁记录 → `skin.equip` → 好友端看到新皮肤

---

### J2：阿铭 —— "200 步接猫回家"

**人物**：阿铭（30 岁，北京后端程序员），每天坐 10 小时，知道久坐有害但讨厌健身 App。买 Apple Watch 本来是为了看时间和刷通知。

**开场**：同事推荐安装裤衩猫。注册后没有好友，但手表上出现了一只猫 —— 和他一样坐着不动（`idle`）。30 分钟后，猫旁边掉出一个闪光的盲盒，手表轻微震动提示。

> **服务端需求**：单人模式也要挂机逻辑 → cron 触发盲盒掉落 → APNs 普通推送（"你的猫捡到了一个盲盒！"）

**升温**：阿铭点开盲盒 → 提示"需要 200 步才能打开"。他翻了个白眼，但走到茶水间倒了杯水 —— 回来一看 180 步了。再走一圈，200 步达标，盲盒打开 → 获得一顶厨师帽皮肤。

> **服务端需求**：`blindbox.redeem`（校验步数 ≥ 200）→ 随机奖励生成（权重表）→ 皮肤解锁写入 → WS 返回奖励详情

**高潮**：一周后阿铭和 3 个同事都装了 App，午休时 4 个人的猫在同一块表上跑酷（都在 `walking` 状态走去食堂）。阿铭说"这是我戴 Apple Watch 以来最有用的 App"。

> **服务端需求**：4 人房间 → `room.snapshot`（4 只猫）→ `friend.state` 广播（4 路 WS 各自发 3 条广播）→ 状态全部 `walking` 时的全员联动视觉触发

**结局**：阿铭现在每天 3000-4000 步（远没到 5000，但比以前的 800 步好太多），收集了 28 个皮肤，没有把它当健身 App —— "只是为了给猫开盲盒顺便走了几步"。

---

### J3：小夏 —— "下载了但没有朋友"（冷启动边缘情况）

**人物**：小夏（22 岁，大学生），在 App Store 搜"Apple Watch 宠物"找到裤衩猫。没有朋友用这个 App。

**开场**：注册完成，猫出现了。但好友列表空空的，房间里只有自己的猫。产品核心体验（看好友猫的状态）完全无法触发。

> **服务端需求**：`session.resume` 返回 `friendCount: 0` → 客户端需要识别冷启动态并引导分享

**困境**：小夏玩了 2 天单机模式 —— 挂机领盲盒、开盲盒、换皮肤。但没有好友，触碰不能用，房间只有自己，"啊哈时刻"根本不会到来。第 3 天留存风险极大。

> **服务端需求**：冷启动用户标记（`user.friendCount == 0 && daysSinceRegister >= 2`）→ APNs 推送"邀请一个朋友，解锁好友猫功能" → 深度链接生成（`friend.invite` WS RPC 预生成邀请 token）

**转机**：小夏把邀请链接发到宿舍群，室友小萌装了 App。两人配对后，小夏第一次在房间里看到小萌的猫在 `walking`（小萌在去图书馆的路上）。"啊哈时刻"终于到来。

> **服务端需求**：邀请 + 配对全链路 → 配对成功后立即触发 `room.snapshot` 推送（不要等小夏下次打开 App）→ APNs 推送"你的好友小萌的猫来了！"

**教训（对服务端的需求）**：
- 好友配对是漏斗最窄的喉咙 — 没配对 = 没核心体验 = 必然流失
- 服务端要主动检测"注册 48 小时仍无好友"并触发召回推送
- 邀请链路的端到端成功率（从链接生成到双方确认）是关键 SLA：目标 ≥ 80%

---

### J4：开发者 —— "凌晨 3 点 WS hub 炸了"（运维旅程）

**人物**：开发者（裤衩猫后端维护者 + Claude 协作），一个人负责服务端运维。

**开场**：周三凌晨 3 点，手机收到 Uptime Robot 告警 —— `/healthz` 返回 503。

> **服务端需求**：`GET /healthz` 检查 Mongo ping + Redis ping + WS hub goroutine 存活 → 任一失败返回 503 + zerolog 记录详情

**排查**：登录服务器，查看 zerolog JSON 日志 → 发现 WS hub 的 goroutine 数从正常的 ~50 飙升到 ~8000 → Redis 连接池耗尽 → 新连接全部阻塞。根因：某用户的 Watch 客户端 bug 导致 1 秒内重连 100 次，每次都 `session.resume` 拉全量快照。

> **服务端需求**：per-user WS 连接频率限流（同一 userID 60 秒内 ≤ 5 次连接）、`session.resume` 幂等 + 节流（60 秒内重复 resume 返回缓存结果）、zerolog 结构化字段（`userId`、`connId`、`event`、`duration`、`goroutineCount`）、Redis 连接池 `MaxActive` 配置 + 超限告警

**恢复**：添加连接频率限流后重启服务，WS hub 恢复正常。排查日志确认该用户 ID，标记为异常设备，推送客户端强制更新。

> **服务端需求**：异常设备/用户标记机制（Redis blacklist + 自动解除 TTL）、优雅重启（接受 WS 连接丢失 + 客户端 `session.resume` 自动恢复）、日志中包含 `build_version`、`config_hash` 便于问题复现

**教训**：一个人运维 = 必须让日志和告警足够清晰。zerolog JSON + 结构化字段是生命线。WS hub 必须有连接级限流和全局 goroutine 上限。MVP 不需要 Grafana 全套，但 `/healthz` + zerolog + Uptime Robot 是最低运维套件。

---

### Journey Requirements Summary

| 旅程 | 揭示的核心服务端能力 |
|---|---|
| **J1 异地恋** | 好友配对全链路、房间状态广播（15s SLA）、触碰 WS + APNs 降级、盲盒确权、皮肤装配广播 |
| **J2 久坐族** | 单人挂机 cron 盲盒掉落、步数兑换确权、多人房间 4 路广播、APNs 盲盒提醒 |
| **J3 冷启动** | 冷启动检测 + 召回推送、邀请链路端到端成功率 SLA、配对即时通知 |
| **J4 运维** | `/healthz` 多维探针、zerolog 结构化日志、WS 连接限流 + goroutine 上限、异常设备标记、优雅重启 |

**MVP 必须覆盖的旅程**：J1（核心体验）+ J2（单人 + 多人）+ J3（冷启动救活）+ J4（运维底线）

## Domain-Specific Requirements

### 合规与隐私

| 要求 | 说明 | MVP / Growth |
|---|---|---|
| **Apple HealthKit 数据用途合规** | 步数由客户端通过 HealthKit 读取后上报服务端存储（每次状态同步附带累计步数）。**允许存储**但限定用途：状态推断、盲盒兑换、点数换算、反作弊。**禁止用于**广告、第三方转卖、公开排行榜（Apple 开发者协议）。 | MVP |
| **中国《个人信息保护法》(PIPL) 健康数据分类** | 步数属于"敏感个人信息"，需明示告知 + 单独同意。服务端需记录同意凭证。 | MVP（如上中国区 App Store） |
| **中国《未成年人保护法》** | 14 岁以下用户需父母同意 + 实名认证。MVP 阶段可通过 App Store 年龄分级（17+）规避，Growth 阶段需服务端认证流程。 | Growth |
| **GDPR 账号删除级联** | 用户删除账号时，必须级联清理：好友关系（双向）、状态快照、触碰历史、盲盒记录、皮肤解锁。好友侧显示为"已注销用户"。 | Growth |
| **GDPR 数据可携带（Art 20）** | 用户可导出个人数据（JSON 打包）。 | Vision |

### 用户安全

| 要求 | 说明 | MVP / Growth |
|---|---|---|
| **触碰反骚扰** | per-user 频次限流：同一发送方 → 同一接收方 60 秒内 ≤ 3 次触碰。超限静默丢弃（不告知发送方被限流，避免对抗）。 | MVP |
| **显式屏蔽执行** | 用户 A 屏蔽 B 后：B 的 `touch.send` 被服务端拦截（不送达、不通知 B 被屏蔽）；B 在房间看不到 A 的猫；A 在房间看不到 B 的猫。双向不可见。 | MVP |
| **跨时区免打扰** | 服务端维护每用户时区 + 免打扰时段（默认 23:00-07:00 本地时间）。免打扰期间：APNs 推送降级为静默推送（不震动不亮屏）；WS 触碰消息打 `quietMode: true` 标记供客户端判断。 | MVP |
| **好友配对防滥用** | 邀请 token 24 小时过期 + 单次使用。同一用户 24 小时内最多生成 10 个邀请 token（防撒网式加好友）。好友上限 20 人（MVP）。 | MVP |

### 技术约束（Apple 平台特有）

| 约束 | 说明 | 影响 |
|---|---|---|
| **watchOS 后台限制** | App 进入后台后 WS 连接不可假设存活。服务端不能依赖"Watch 持续在线"做任何逻辑（如在线计数、实时状态更新）。 | 状态衰减引擎必须在服务端单方面运行，不依赖客户端心跳 |
| **APNs 静默推送配额** | Apple 限制静默推送频率（无官方数字，社区经验 ~2-3 次/小时/设备）。服务端不能把静默推送当实时通道。 | 好友状态刷新不能依赖静默推送，必须靠 WS 前台 + 服务端衰减 |
| **WatchConnectivity 延迟** | Watch ↔ iPhone 通过 WatchConnectivity 中转时延迟可达 30 秒以上。 | 服务端不能假设 Watch 和 iPhone 状态同步是实时的；以各自设备直接上报为准 |
| **HealthKit 后台唤醒窗口** | iPhone 后台 HealthKit observer 唤醒窗口约 30 秒。 | HTTP `POST /state` 必须单次完成，不能走 WS 握手 |
| **步数数据模型** | 服务端存储 `dailySteps`（当日累计）+ `stepDelta`（本次同步增量）+ `lastSyncAt`。不存储逐秒步数明细（数据最小化）。步数随状态快照一起通过 WS `state.tick` 或 HTTP `POST /state` 上报。 | 用于状态推断、盲盒兑换确权、点数换算、反作弊校验 |

### 风险缓解

| 风险 | 影响 | 缓解措施 |
|---|---|---|
| **盲盒客户端篡改步数** | 用户修改 HealthKit 数据骗取盲盒 | 服务端校验步数增量合理性（单次增量 > 10000 步触发人工审查标记） |
| **WS hub 内存泄漏** | 大量僵尸连接耗尽服务器内存 | 30 秒心跳超时自动断开 + goroutine 全局上限 + per-user 连接频率限流 |
| **好友图过度膨胀** | 大 V 加满 20 好友，广播风暴 | MVP 好友上限 20；房间同屏上限 4；广播只发给同房间成员 |
| **状态衰减误判** | 好友猫显示"在线"但实际已离线 | `presence_decay_truthfulness` — 衰减规则透明（UI 显示"X 分钟前"而非伪装实时） |
| **APNs token 失效未清理** | 推送失败累积，浪费服务端资源 | APNs 反馈处理（HTTP 410 → 删除 token）+ 定期清理 cron |

## Innovation & Novel Patterns

### Detected Innovation Areas

**1. 平台生态位创新 —— Apple Watch 陪伴赛道的首发者**

Apple Watch 上现存的社交/陪伴类 App 几乎全是"iPhone 功能的手表缩略版"（微信手表版 = 看消息、Nike Run Club = 看配速）。裤衩猫是第一款把 Apple Watch 作为主交互表面、为手表原生设计的陪伴社交产品。

创新本质：不是"把手机 App 搬到手表"，而是"为手腕这块 2 英寸屏幕设计一种新的社交形态"。

**2. 交互范式创新 —— Ambient Co-Presence（环境在场感）**

现有社交产品的范式：
- **IM（微信/iMessage）**：发消息 → 等回复 → 高注意力负担
- **视频（FaceTime/Zoom）**：同步在线 → 最高注意力负担
- **动态（朋友圈/Instagram）**：发布 → 被动消费 → 异步、无实时感

裤衩猫的范式：
- **看猫 = 感知对方存在** → 无需发送、无需回复、无需在线确认
- "TA 的猫在走路" = "TA 在通勤" → 零消息量、满信息量
- 这是一种第四类社交形态：不发消息、不打电话、不发动态，但持续知道对方在干嘛

服务端角色：状态镜像 + 衰减引擎就是这种交互范式的技术底座。没有服务端精确维护快照 + 15 秒广播，"ambient co-presence"就退化成"看一张旧截图"。

**3. 语义重构创新 —— 步数从健康指标到社交货币**

所有健康 App 把步数定义为"你的健身成绩"（圆环、排行榜、目标达成率）。裤衩猫把步数重新定义为：
- **盲盒钥匙** —— 200 步 = 打开一个盲盒的资格
- **猫的行为驱动** —— 走路 = 猫跑步（不是"你跑了多少"，是"猫在干嘛"）
- **好友可见的生活痕迹** —— 你的步数变成好友看到的猫的状态

服务端角色：步数存储 + 点数换算 + 盲盒确权 → 把步数的"健康语义"完全替换为"社交 + 收集语义"。

**4. 协议创新 —— 资源受限设备上的 WS-primary 架构**

Apple Watch 上主流做法是"HTTP 轮询 + APNs"。裤衩猫选择"登录后 WS 主导 + HTTP 仅限 bootstrap"是对 watchOS 网络模型的非常规运用：
- 前台全程 WS → 房间内秒级状态广播
- cache-first + background-resume → 抬腕即猫（不等连接）
- per-device 多路复用 → Watch + iPhone 各自独立连接

这套方案在 watchOS 上没有先例参考，需要验证。

### Market Context & Competitive Landscape

| 竞品 / 参考 | 平台 | 与裤衩猫的差异 |
|---|---|---|
| **Bongo Cat** | PC（桌面挂件） | 单机、无社交、无手表 |
| **Tamagotchi Uni** | 专有硬件 | 有 Wi-Fi 社交但非手表、无运动映射 |
| **Woebot / Replika** | 手机 | AI 对话陪伴，非环境在场 |
| **Apple 健身圆环** | Watch | 健身压力模型，非治愈 |
| **Widgetsmith** | Watch + Phone | 视觉定制，无社交、无陪伴 |

直接竞品：零。这验证了"Apple Watch 陪伴产品不存在"的论断。

### Validation Approach

| 创新点 | 验证方式 | 失败判据 |
|---|---|---|
| 平台生态位 | App Store 搜索排名 + 下载量 | 上线 30 天搜索关键词（Apple Watch 宠物 / 手表猫）排名 ≤ 3 |
| Ambient Co-Presence | 日均在房时长 + 好友状态查看频次 | 用户在房时间中位数 < 2 分钟 = "ambient"无效，产品退化成"偶尔打开看一眼" |
| 步数语义重构 | 盲盒打开率 + 用户日均步数变化 | 用户安装后日均步数无变化 = 步数激励失效 |
| WS-primary on watchOS | WS 重连成功率 + 抬腕到有效画面延迟 | p99 > 5 秒 → 触发 OP-1 设计方案收敛（见 Open Problems） |

### Innovation Risk Mitigation

| 风险 | 影响 | 缓解 |
|---|---|---|
| **"Ambient Co-Presence 太弱"** | 用户觉得"看猫状态"不如直接微信问 | Growth 阶段加强互动层：触碰多样化、共同目标、日历签到，让"在场感"不止于"看" |
| **步数激励被健康 App 疲劳抵消** | 用户觉得"又一个算步数的 App" | 严格不打健康牌：无圆环、无排行、无目标达成率。只有"猫在跑 + 盲盒可以开" |
| **竞品 3 个月内跟进** | 首发窗口被压缩 | 社交网络效应是护城河（好友图不可迁移）+ 快速迭代皮肤 / 表情差异化 |

> WS-primary on watchOS 稳定性风险独立追踪于 Open Problems OP-1。

## API Backend Specific Requirements

### Project-Type Overview

裤衩猫后端是 `api_backend` 的非典型子类（`realtime_social_backend`）：传统 API backend 以 HTTP REST 为主通道，本项目以 **WebSocket 为主通道**，HTTP 仅服务 bootstrap 和系统集成。技术需求需覆盖两套协议的规范。

### Authentication Model

| 要素 | 设计 |
|---|---|
| 身份提供方 | Sign in with Apple（OAuth 风格） |
| 服务端 token | JWT access token（短效 15 分钟）+ refresh token（长效 30 天） |
| JWT 签发 | `pkg/jwtx` 封装，`golang-jwt/jwt/v5`，RS256 双密钥 |
| 鉴权入口 | HTTP：`Authorization: Bearer <access_token>` header；WS：upgrade 请求带 `Authorization` header |
| Token 刷新 | HTTP `POST /auth/refresh`（WS 不做 token 刷新 — 连接存续期间 token 失效由服务端内部延期判断，不中断连接） |
| Token 吊销 | refresh token 黑名单（Redis SET，TTL = token 剩余有效期） |
| 设备标识 | APNs device token 独立注册（`POST /devices/apns-token`），与 JWT 关联但不耦合 |

### Endpoint Specifications — HTTP (Bootstrap)

| Endpoint | Method | Auth | 请求 | 响应 | 说明 |
|---|---|---|---|---|---|
| `/auth/apple` | POST | 无 | `{ identityToken, authorizationCode }` | `{ accessToken, refreshToken, user }` | Sign in with Apple 登录 |
| `/auth/refresh` | POST | refresh token | `{ refreshToken }` | `{ accessToken, refreshToken }` | 刷新 access token |
| `/devices/apns-token` | POST | access token | `{ deviceToken, platform }` | `{ ok }` | 注册 APNs device token |
| `/state` | POST | access token | `{ catState, dailySteps, stepDelta }` | `{ ok }` | iPhone 后台 HealthKit 状态上报（30s 窗口特例） |
| `/healthz` | GET | 无 | — | `{ status, mongo, redis, wsHub }` | 健康检查 |
| `/readyz` | GET | 无 | — | `{ ready }` | 就绪检查 |
| `/ws` | GET→Upgrade | access token（header） | — | WebSocket 连接 | WS 升级 |

### Endpoint Specifications — WebSocket Message Type Registry

**Envelope 格式：**

```
上行请求:   { "id": "uuid", "type": "xxx.yyy", "payload": {...} }
下行响应:   { "id": "uuid", "ok": true|false, "type": "xxx.yyy.result", "payload": {...}, "error": {...}|null }
下行推送:   { "type": "xxx.yyy", "payload": {...} }
心跳:       { "type": "ping" } / { "type": "pong" }
```

**RPC 类型（请求-响应，有 id）：**

| type | 方向 | payload (request) | payload (response) | 说明 |
|---|---|---|---|---|
| `session.resume` | 上→下 | `{ lastEventId? }` | `{ user, friends, catState, skins, blindboxes, roomSnapshot? }` | 建连后拉全量快照 |
| `blindbox.redeem` | 上→下 | `{ blindboxId }` | `{ reward: { skinId, rarity } }` | 盲盒领取（强一致） |
| `touch.send` | 上→下 | `{ friendId, emoteType }` | `{ delivered: true }` | 发触碰 |
| `friend.invite` | 上→下 | `{}` | `{ inviteToken, expiresAt }` | 生成邀请 token |
| `friend.accept` | 上→下 | `{ inviteToken }` | `{ friend }` | 接受邀请 |
| `friend.delete` | 上→下 | `{ friendId }` | `{ ok }` | 解除好友 |
| `friend.block` | 上→下 | `{ friendId }` | `{ ok }` | 屏蔽好友 |
| `friend.unblock` | 上→下 | `{ friendId }` | `{ ok }` | 取消屏蔽 |
| `skin.equip` | 上→下 | `{ skinId }` | `{ ok }` | 装配皮肤 |
| `profile.update` | 上→下 | `{ displayName? }` | `{ user }` | 更新个人资料 |
| `skins.catalog` | 上→下 | `{}` | `{ skins: [...] }` | 皮肤目录 |
| `users.me` | 上→下 | `{}` | `{ user, points, dailySteps }` | 个人信息 |
| `friends.list` | 上→下 | `{}` | `{ friends: [...] }` | 好友列表 |
| `friends.state` | 上→下 | `{}` | `{ friendStates: [...] }` | 好友快照（含衰减） |
| `blindbox.inventory` | 上→下 | `{}` | `{ blindboxes: [...] }` | 盲盒库存 |

**Push 类型（服务端主动推送，无 id）：**

| type | payload | 说明 |
|---|---|---|
| `friend.state` | `{ friendId, catState, updatedAt }` | 好友状态变化广播 |
| `friend.touch` | `{ friendId, emoteType }` | 收到好友触碰 |
| `friend.blindbox` | `{ friendId, skinId, rarity }` | 好友开盲盒视觉提示 |
| `friend.online` | `{ friendId }` | 好友上线通知 |
| `friend.offline` | `{ friendId }` | 好友离线通知 |
| `room.snapshot` | `{ members: [{ userId, catState, skinId, updatedAt }] }` | 房间当前状态全量 |
| `state.serverPatch` | `{ catState, reason }` | 服务端状态修正（衰减 / 推断） |
| `blindbox.drop` | `{ blindboxId, stepsRequired }` | 新盲盒掉落 |

**Client Push 类型（客户端上报，无响应）：**

| type | payload | 说明 |
|---|---|---|
| `state.tick` | `{ catState, dailySteps, stepDelta }` | 状态 + 步数上报（也触发持久化） |

### Data Schemas

**核心数据模型（MongoDB Collections）：**

| Collection | 主键 | 关键字段 | 索引 |
|---|---|---|---|
| `users` | `_id: UserID` | `appleUserId, displayName, timezone, friendCount, createdAt` | `appleUserId` (unique) |
| `cat_states` | `_id: UserID` | `catState, dailySteps, stepDelta, points, skinId, updatedAt, source` | `updatedAt` (TTL cleanup) |
| `friends` | `_id: auto` | `userA, userB, status, createdAt` | `(userA, userB)` unique compound |
| `blocks` | `_id: auto` | `blocker, blocked, createdAt` | `(blocker, blocked)` unique compound |
| `invite_tokens` | `_id: token` | `creatorId, expiresAt, used` | `expiresAt` (TTL) |
| `blindboxes` | `_id: BlindboxID` | `userId, status, stepsRequired, reward, createdAt, redeemedAt` | `(userId, status)` |
| `skins` | `_id: SkinID` | `name, rarity, layer, assetPath` | — |
| `user_skins` | `_id: auto` | `userId, skinId, unlockedAt` | `(userId, skinId)` unique |
| `touch_logs` | `_id: auto` | `fromUser, toUser, emoteType, createdAt` | `(fromUser, createdAt)` |
| `apns_tokens` | `_id: auto` | `userId, deviceToken, platform, updatedAt` | `(userId, platform)` |

**Redis 热数据：**

| Key Pattern | 类型 | TTL | 用途 |
|---|---|---|---|
| `state:{userId}` | Hash | 无（随写更新） | 猫状态快照热缓存 |
| `presence:{userId}` | String (connId) | 60s（心跳续期） | WS 在线状态 |
| `event:{eventId}` | String ("1") | 5min | 幂等去重 |
| `ratelimit:touch:{from}:{to}` | Counter | 60s | 触碰频次限流 |
| `ratelimit:ws:{userId}` | Counter | 60s | WS 连接频率限流 |
| `ratelimit:invite:{userId}` | Counter | 24h | 邀请生成限流 |
| `blacklist:device:{userId}` | String ("1") | 可配置 | 异常设备标记 |
| `refresh_blacklist:{tokenJti}` | String ("1") | token 剩余有效期 | refresh token 吊销 |

### Error Codes

**HTTP 错误响应格式：**
```json
{ "code": "AUTH_TOKEN_EXPIRED", "message": "Access token has expired", "httpStatus": 401 }
```

**WS 错误响应格式（在 envelope 内）：**
```json
{ "id": "uuid", "ok": false, "type": "blindbox.redeem.result", "error": { "code": "BLINDBOX_ALREADY_REDEEMED", "message": "..." } }
```

**错误码注册表（MVP）：**

| Code | HTTP | 说明 |
|---|---|---|
| `AUTH_INVALID_IDENTITY_TOKEN` | 401 | Sign in with Apple token 无效 |
| `AUTH_TOKEN_EXPIRED` | 401 | access token 过期 |
| `AUTH_REFRESH_TOKEN_REVOKED` | 401 | refresh token 已吊销 |
| `FRIEND_ALREADY_EXISTS` | 409 | 已是好友 |
| `FRIEND_LIMIT_REACHED` | 422 | 好友数达上限（20） |
| `FRIEND_INVITE_EXPIRED` | 410 | 邀请 token 过期 |
| `FRIEND_INVITE_USED` | 410 | 邀请 token 已使用 |
| `FRIEND_BLOCKED` | 403 | 已被屏蔽 |
| `BLINDBOX_ALREADY_REDEEMED` | 409 | 盲盒已领取 |
| `BLINDBOX_INSUFFICIENT_STEPS` | 422 | 步数不足 |
| `BLINDBOX_NOT_FOUND` | 404 | 盲盒不存在 |
| `SKIN_NOT_OWNED` | 403 | 未解锁的皮肤 |
| `RATE_LIMIT_EXCEEDED` | 429 | 频率限流 |
| `DEVICE_BLACKLISTED` | 403 | 异常设备 |
| `INTERNAL_ERROR` | 500 | 服务端内部错误 |

### Rate Limits

| 限流维度 | 阈值 | 窗口 | 超限行为 |
|---|---|---|---|
| 触碰（同一 from → to） | ≤ 3 次 | 60 秒 | 静默丢弃（不通知发送方） |
| WS 连接（同一 userId） | ≤ 5 次 | 60 秒 | 拒绝连接 + 返回 429 |
| 邀请 token 生成 | ≤ 10 次 | 24 小时 | 返回 RATE_LIMIT_EXCEEDED |
| `session.resume`（同一 userId） | ≤ 5 次 | 60 秒 | 返回缓存结果（不重查 DB） |
| HTTP 全局（per IP） | ≤ 60 次 | 60 秒 | 返回 429 |

### API Documentation Strategy

| 协议 | 文档形式 | 内容 |
|---|---|---|
| HTTP（5 endpoints） | **OpenAPI 3.0 YAML** | endpoint / request / response / error codes / auth |
| WS（20+ 消息类型） | **WS Message Type Registry**（Markdown 表格） | type / direction / payload schema / response schema / rate limits |
| 两者共用 | **Error Code Registry**（Markdown 表格） | code / HTTP status / 说明 |

文档随代码维护在 `docs/` 目录，由 CI 校验 OpenAPI YAML 的结构合法性。

### Versioning Strategy

MVP 阶段不做显式版本管理：
- HTTP 路由无 `/v1` 前缀
- WS 消息 type 不带版本后缀
- 客户端和服务端同步发版（自有 App，无第三方消费者）

首次破坏性变更时引入版本：HTTP 加 `/v2` 路由前缀，WS 加 `type.v2` 后缀，旧版本保留 30 天过渡。

### Implementation Considerations

**分层落地指引（按架构宪法）：**

| 层 | 包 | 职责 | 数量估算 |
|---|---|---|---|
| Handler | `internal/handler/` | HTTP 路由处理（仅 bootstrap 5 endpoint） | ~3 文件 |
| WS | `internal/ws/` | Hub + Client + Dispatcher + Envelope + SessionResume | ~8 文件 |
| Service | `internal/service/` | AuthService, StateService, FriendService, TouchService, BlindboxService, SkinService | ~6 文件 |
| Repository | `internal/repository/` | interface + mongo impl（每 collection 一对） | ~10 文件 |
| Domain | `internal/domain/` | CatState FSM, BlindboxReward 权重表, FriendRelation 值对象 | ~4 文件 |
| DTO | `internal/dto/` | AppError + WS envelope types + request/response structs | ~3 文件 |
| Middleware | `internal/middleware/` | JWT auth, rate limiter, CORS, request logger | ~4 文件 |
| Cron | `internal/cron/` | BlindboxDrop, StateDecay, APNsCleanup | ~3 文件 |
| Push | `internal/push/` | APNs client + tier logic (alert/silent) + fallback | ~2 文件 |
| Pkg | `pkg/` | logx, mongox, redisx, jwtx, ids, fsm | ~6 文件 |

## Project Scoping & Phased Development

### MVP Strategy & Philosophy

**MVP 哲学：Experience MVP（体验验证型）**

裤衩猫的核心假设是"Ambient Co-Presence 能成为一种被用户持续消费的社交形态"。这既不是 Platform MVP（拼生态）也不是 Revenue MVP（拼商业模式），而是**体验假设验证**：
- 如果用户打开房间看到好友猫的状态变化 → 感到陪伴 → 留下
- 这个链路不成立，再多功能都白费

**验证闭环（30 天内必须完成）：**
1. 用户能成功配对至少 1 个好友
2. 用户在房间里看到好友猫的真实状态变化
3. 用户自发再次打开 App（次日留存 ≥ 35%）

### Resource Requirements

| 角色 | 人员 | 承担 |
|---|---|---|
| 后端开发 | 开发者 + Claude（99.99% 编码） | 后端全部实现 |
| iOS/watchOS 开发 | 开发者 + Claude | 客户端全部实现 |
| 美术 | 外部资源 / 个人外包（**瓶颈**） | Spine 母资源、默认猫、MVP 少量皮肤 |
| 测试 | 开发者 + Claude + 真机调试（**瓶颈**） | table-driven unit + 关键 E2E 手测 |
| 运维 | 开发者 | 单机部署 + zerolog + Uptime Robot |
| 合规 | 开发者自查 | App Store 年龄分级 17+ 规避未成年人复杂流程 |

**关键约束：美术资源和真机调试是瓶颈，而非代码量**。后端代码尽可能模块化 + 高测试覆盖，让 Claude 能稳定迭代，人工精力集中在美术审美和真机验证上。

### MVP Feature Set (Phase 1) — Must-Have 细化

按用户旅程倒推必备能力（否则 MVP 根本跑不起来）：

| 来自 | Must-Have 能力 | 备注 |
|---|---|---|
| J1 异地恋 | Sign in with Apple、JWT、好友邀请 + 接受、房间 WS、`friend.state` 广播、`touch.send` + APNs 降级 | 核心体验闭环 |
| J2 久坐族 | 单人挂机盲盒掉落（cron）、步数兑换确权、4 人房间广播、盲盒 APNs 提醒 | 盲盒 + 多人 |
| J3 冷启动 | 冷启动检测 cron、召回 APNs 推送、邀请端到端成功率追踪 | 救活无好友用户 |
| J4 运维 | `/healthz` 多维探针、zerolog 结构化、WS 连接限流、goroutine 上限、`session.resume` 节流 | 运维底线 |
| 合规底线 | 触碰反骚扰限流、显式屏蔽执行、跨时区免打扰、邀请 token 限流 + 过期 | 必须上线即有 |

**MVP 显式不做**：详见 §Product Scope 的 *Growth Features (Post-MVP)* 和 *Vision (Future)* 两节。

### Phase Progression Triggers

| Phase 2（Growth）触发条件 | 说明 |
|---|---|
| DAU ≥ 1,000 持续 30 天 | 首发红利验证成功，进入 Growth |
| 次日留存 ≥ 35% 稳定 | Ambient Co-Presence 假设被证实，值得加深 |
| 美术资源稳定产出 | 换装系统依赖美术产能 |

| Phase 3（Vision）触发条件 | 说明 |
|---|---|
| DAU ≥ 10,000 | 单实例开始吃力，启动水平扩展 |
| WS 在线连接峰值 > 5,000 | Redis presence 和 goroutine 压力接近阈值 |
| 中国区运营计划明确 | 触发未成年人认证流程 |

### Risk Mitigation Strategy

**Technical Risks**

| 风险 | 缓解方式 | 触发信号 |
|---|---|---|
| WS-primary on watchOS 不稳定 | 独立追踪于 Open Problems OP-1 | 见 OP-1 验证路径 |
| 状态衰减引擎误判 | 单元测试覆盖所有 4 档衰减边界；真机观察 | 用户反馈"好友猫状态假" |
| 盲盒强一致故障 | MongoDB 4.0+ 事务 + Redis 幂等 eventId | 任一次重复领取 = P0 事故 |
| 步数反作弊误杀 | 阈值从宽（10000 步/次），仅打 flag 不直接拒绝 | 误杀率 > 1% → 重调阈值 |

**Market Risks**

| 风险 | 验证 / 缓解 |
|---|---|
| Ambient Co-Presence 是伪需求 | MVP 30 天留存和在房时长即检验；留存 < 20% → pivot 到触碰驱动 |
| App Store 审核失败 | 提前研究类似品类审核案例（Widgetsmith / Bongo 同类），准备好"陪伴非医疗非成瘾"的申辩 |
| 竞品快速跟进 | 社交网络效应 + 快速皮肤迭代拉开差距 |
| 用户找不到你 | 预热营销：小红书 / 微博 KOL 种草（"Apple Watch 手腕养猫"），不投信息流 |

**Resource Risks**

| 风险 | 缓解 |
|---|---|
| 美术产能跟不上 | MVP 只用默认猫 + 3-5 个皮肤；用开源 / 现成 Spine 资源验证技术链路 |
| 真机调试时间不足 | 优先服务端 + 客户端 unit/integration 覆盖；真机只测关键 E2E（配对、触碰、盲盒、WS 重连） |
| 开发者单点故障 | 所有配置和部署文档化（架构宪法 + 本 PRD）；Claude 协作保障知识不丢 |
| 时间超支 | MVP 目标：**6 个月内上线**（含客户端 + 后端）。超过 8 个月 → 重新评估 Growth 阶段是否延后 |

## Open Problems

### OP-1：watchOS 上 WS-primary 的稳定性

**问题陈述**

Apple Watch 的 App 生命周期（进程冻结 / 网络切换 / 电量敏感 / 抬腕延迟预算 500ms-1s）与"登录后 WS 全程"的前提假设冲突。具体六个层面：

1. watchOS 屏幕熄灭后进程几秒内被冻结 → WS 连接沦为僵尸
2. 抬腕 → 看到画面的心理预算 < 1 秒，但 WS 冷重连良好 Wi-Fi 下 ~250ms、4G 下 ~850ms、弱网 > 4s
3. 网络静默切换（Bluetooth via iPhone ↔ 独立 LTE ↔ 无网）导致 TCP 连接静默死亡
4. `URLSessionWebSocketTask` 不支持后台 session configuration、不支持自动重连
5. 持续 LTE + WS 心跳的电量开销约为 HTTP 轮询 + APNs 的 3-5 倍
6. 抬腕高峰期的服务端 `session.resume` 风暴

**约束**

- **不采用 HTTP 回退作为降级方案**（用户明确：backup 不解决问题，只掩盖问题）
- 必须从设计层解决根因

**待探索的设计方向**（非决定，仅候选）

- 客户端 cache-first + 差分更新协议（WS 传 diff，不传全量）
- 服务端驱动的"最后已知状态"快照协议（客户端带 lastSeq，服务端只回增量）
- 基于 watchOS 抬腕事件的精细化重连触发（而非周期性心跳）
- `session.resume` 服务端主动推送版（客户端建连后服务端立即推，不等客户端请求）
- 预连接 / 连接预热策略
- 协议层的消息压缩（减少弱网下 payload 传输时间）
- 对 watchOS `NWConnection` 的深度适配（而非 `URLSessionWebSocketTask`）
- 客户端网络切换事件订阅 + 立即重连，减少僵尸连接时长
- 服务端 presence 的双向心跳 + 主动探测机制

**验证路径**

MVP 开发过程中建立**真机 + 弱网测试矩阵**（Wi-Fi / 4G / 弱网 / 网络切换 / 抬腕循环），量化 WS 重连 p50/p95/p99 延迟与电量消耗，以此驱动设计决策。

**责任人**：开发者 + Claude（架构 + 实现）
**状态**：`open`，不在 MVP 启动时阻塞，但必须在**灰度测试前**收敛出明确设计方案。

## Functional Requirements

本章节是产品的**能力契约**。下游所有工作（Epic 拆分、架构设计、实现、测试）只能实现这里列出的能力。遗漏意味着产品里不存在。

### 身份与鉴权（Identity & Authentication）

- **FR1**: 用户可以使用 Sign in with Apple 登录，服务端签发 JWT access + refresh token
- **FR2**: 用户可以使用 refresh token 换取新的 access token（无需重新登录）
- **FR3**: 服务端可以将 refresh token 加入黑名单以吊销
- **FR4**: 用户可以注册 APNs device token（每台设备独立，与用户关联）
- **FR5**: 用户可以在 Watch 和 iPhone 上各自独立登录，服务端以 per-device 方式管理连接与 token
- **FR6**: 用户可以通过 WebSocket 升级请求在 header 携带 JWT 完成鉴权（FR1 鉴权能力的延伸）

### 猫状态与运动（Cat State & Activity）

- **FR7**: 用户可以上报自己猫的当前状态（idle / walking / running / sleeping），服务端持久化快照
- **FR8**: 用户可以在状态上报时附带当日累计步数和本次同步增量
- **FR9**: 服务端可以根据用户长时间无状态上报自动推断状态为 idle（服务端 FSM）
- **FR10**: 服务端可以按时间衰减规则将陈旧状态自动降级至 idle 或标记离线（具体阈值见 NFR）
- **FR11**: 服务端可以对异常步数增量（单次 > 10,000 步）打标记以便人工审查
- **FR12**: 服务端可以通过 HTTP `POST /state` 接受 iPhone 后台 HealthKit 的一次性状态上报

### 好友关系（Friend Graph）

- **FR13**: 用户可以生成好友邀请 token（24 小时过期、单次使用）
- **FR14**: 用户可以使用邀请 token 接受好友关系（双向建立）
- **FR15**: 用户可以解除与指定好友的关系
- **FR16**: 用户可以查看当前好友列表
- **FR17**: 用户可以屏蔽指定好友（双向不可见）
- **FR18**: 用户可以取消屏蔽
- **FR19**: 服务端可以限制每用户好友上限为 20 人
- **FR20**: 服务端可以限制每用户 24 小时内最多生成 10 个邀请 token
- **FR55**: 服务端可以生成基于 Universal Link 格式的好友邀请 URL

### 房间与实时在场（Room & Real-time Presence）

- **FR21**: 用户可以加入好友房间（最多 4 人同屏）并建立 WebSocket 长连接
- **FR22**: 用户加入房间时可以立即收到房间全量快照（所有成员当前猫状态 + 皮肤）
- **FR23**: 用户可以接收到房间内好友状态变化的实时广播（p99 ≤ 3 秒）
- **FR24**: 用户可以收到"好友上线"和"好友离线"事件通知
- **FR25**: 用户可以通过 WebSocket 断线后携带 `lastEventId` 执行 `session.resume` 恢复会话
- **FR51**: 用户可以离开房间（主动 / App 退到后台）且服务端可感知
- **FR52**: 服务端可以在房间人数已满（4 人）时拒绝新成员加入并返回明确错误码

### 触碰 / 触觉社交（Touch / Haptic Social）

- **FR26**: 用户可以向任意好友发送触碰事件（指定表情类型）
- **FR27**: 服务端可以在接收方 WebSocket 在线时通过 WS 送达触碰，否则降级为 APNs 送达
- **FR28**: 服务端可以强制执行 per-sender → per-receiver 60 秒 ≤ 3 次的触碰频次限流（超限静默丢弃）
- **FR29**: 服务端可以拦截发送方已被接收方屏蔽的触碰（不送达、不通知发送方）
- **FR30**: 服务端可以在接收方处于本地时区免打扰时段（默认 23:00-07:00）时将推送降级为静默推送

### 盲盒与奖励（Blind Box & Rewards）

- **FR31**: 服务端可以在用户挂机满 30 分钟**且当前无未领取盲盒**时为其投放一个新的盲盒（cron 触发；单槽位设计）
- **FR32**: 用户可以在累计步数达到盲盒解锁阈值后领取盲盒
- **FR33**: 服务端可以验证盲盒领取请求的合法性并保证每个盲盒最多被成功领取一次
- **FR34**: 用户可以查询当前盲盒库存（待解锁 / 已解锁 / 已领取）
- **FR35**: 用户可以在盲盒领取成功后收到奖励详情（皮肤 ID + 稀有度）
- **FR54**: 服务端可以在抽到已拥有皮肤时返回明确的结果（如折算点数）

### 皮肤与定制（Skin & Customization）

- **FR36**: 用户可以查询全部皮肤目录（含默认猫 + 可解锁皮肤）
- **FR37**: 用户可以查询自己已解锁的皮肤列表
- **FR38**: 用户可以将一款已解锁的皮肤装配给自己的猫
- **FR39**: 用户可以让好友看到自己装配的皮肤（随状态广播）

### 账户管理（Account Management）

- **FR47**: 用户可以请求注销账号（MVP 阶段标记 `deletion_requested`，Growth 阶段落实级联清理）
- **FR48**: 用户可以修改自己的 displayName
- **FR49**: 用户可以修改自己的 timezone 和免打扰时段
- **FR50**: 客户端可以主动上报设备时区变更

### 运维与可靠性（Operations & Reliability）

- **FR40**: 运维方可以通过 `GET /healthz` 获取服务端健康状态（含 Mongo / Redis / WS hub 存活）
- **FR41**: 服务端可以对同一用户的 WS 建连频率做限流（具体阈值见 NFR）
- **FR42**: 服务端可以对短时间内重复的 `session.resume` 调用返回缓存结果而非重复查询持久化存储
- **FR43**: 服务端可以在收到 APNs 反馈 HTTP 410 时自动删除失效的 device token
- **FR44a**: 服务端可以识别冷启动用户（注册 ≥ 48h + 好友数 = 0）
- **FR44b**: 服务端可以向识别出的冷启动用户发起召回推送
- **FR45**: 服务端可以对异常设备（通过 userId 标记）拒绝 WebSocket 连接
- **FR46**: 服务端可以输出结构化 JSON 日志并包含 userId / connId / event / duration 字段
- **FR56**: 服务端可以保证分布式锁下的单一 cron 执行（盲盒投放、衰减、冷启动检测）
- **FR57**: 服务端可以对 WS 上行消息按 eventId 去重（窗口时间作为 NFR 配置）
- **FR58**: 服务端可以按 platform 路由 APNs 推送（Watch token 发 Watch 推送，iPhone token 发 iPhone 推送）
- **FR59**: 客户端可以查询服务端支持的 WS 消息类型与版本
- **FR60**: 服务端可以在开发 / 测试环境暴露"虚拟时钟"以便模拟状态衰减测试

### Epic 拆分预览

| Epic | FR 范围 | 依赖 | 预估 Story 数 |
|---|---|---|---|
| E1: 身份与鉴权 | FR1-6, FR47-50 | 无 | ~5 |
| E2: 猫状态 + 步数数据模型 | FR7-12 | E1 | ~5 |
| E3: 好友图 + 邀请系统 | FR13-20, FR55 | E1 | ~6 |
| E4: WS 房间 + 实时在场 | FR21-25, FR51-52 | E1, E2，**+ Spike-OP1 前置** | ~5 |
| E5: 触碰 / 社交 | FR26-30 | E3, E4, E8 | ~4 |
| E6: 盲盒 + 皮肤 | FR31-39, FR54 | E1, E2 | ~6 |
| E7: 基础设施与可靠性 | FR40-46, FR56-60 | 贯穿 | ~6 |
| E8: APNs 推送路由 | FR4, FR27, FR30, FR43, FR44b, FR58 | E1 | ~3 |

### Spike-OP1（MVP 前置）

**Spike-OP1**：在 MVP 开发进入 E4（房间与 WS 实时在场）之前，建立真机 + 弱网测试矩阵，量化 WS 重连 p50/p95/p99 延迟与电量消耗。此 Spike 收敛出 OP-1（watchOS WS-primary 稳定性）的设计方案后才允许进入 E4 开发。

## Non-Functional Requirements

### Performance

| ID | 需求 | 可测判据 |
|---|---|---|
| **NFR-PERF-1** | 房间内好友状态广播延迟 | p99 ≤ 3 秒 |
| **NFR-PERF-2** | HTTP bootstrap endpoint 响应时间 | p95 ≤ 200ms |
| **NFR-PERF-3** | WebSocket `session.resume` 响应时间 | p95 ≤ 500ms |
| **NFR-PERF-4** | WebSocket RPC 消息往返 | p95 ≤ 200ms |
| **NFR-PERF-5** | 状态衰减四档阈值 | 0-15s 真实 / 15-60s 显示但 UI 弱化 / 1-5min 回落 idle / >5min 标记离线 |
| **NFR-PERF-6** | session.resume 缓存窗口 | 60 秒内重复调用返回缓存 |
| **NFR-PERF-7** | Cron 任务触发时延 | 盲盒投放 ≤ 60s、衰减扫描周期 30s、冷启动检测周期 24h |

### Security

| ID | 需求 | 可测判据 |
|---|---|---|
| **NFR-SEC-1** | 传输层加密 | TLS 1.3 强制（HTTP + WS），无明文端口 |
| **NFR-SEC-2** | JWT 签名算法 | RS256，支持双密钥轮换（architecture 宪法 §2） |
| **NFR-SEC-3** | Access token 有效期 | ≤ 15 分钟 |
| **NFR-SEC-4** | Refresh token 有效期 | ≤ 30 天 |
| **NFR-SEC-5** | 密码管理 | 不存储（Sign in with Apple 唯一来源） |
| **NFR-SEC-6** | Apple userId 落库方式 | 哈希存储（不可逆），不回溯原始 |
| **NFR-SEC-7** | APNs device token 存储 | 加密存储（Mongo field-level 加密或 KMS） |
| **NFR-SEC-8** | 限流覆盖 | 所有写入 endpoint 必有限流（touch、invite、blindbox.redeem、WS connect） |
| **NFR-SEC-9** | 幂等性去重 | 所有权威写 RPC 按 eventId 去重（5 分钟窗口） |
| **NFR-SEC-10** | 审计日志 | 所有 token 签发 / 吊销 / 屏蔽 / 盲盒领取事件留痕（zerolog） |

### Scalability

| ID | 需求 | 可测判据 |
|---|---|---|
| **NFR-SCALE-1** | MVP 部署目标 | 单 binary、单实例 |
| **NFR-SCALE-2** | 代码不变量 — 无进程级全局状态 | 所有共享状态入 Redis（presence / ratelimit / event dedup / blacklist）；禁用 `sync.Map` 存连接 / 计数器 |
| **NFR-SCALE-3** | 多副本就绪性 | 仅改部署拓扑即可横向扩展，不改业务代码 |
| **NFR-SCALE-4** | 单实例 WS 并发连接上限 | ≤ 10,000（超过即告警） |
| **NFR-SCALE-5** | Per-user WS 建连限流 | 60 秒 ≤ 5 次 |
| **NFR-SCALE-6** | Per-sender-receiver 触碰限流 | 60 秒 ≤ 3 次 |
| **NFR-SCALE-7** | Per-user 邀请 token 限流 | 24 小时 ≤ 10 个 |
| **NFR-SCALE-8** | HTTP per-IP 全局限流 | 60 秒 ≤ 60 次 |
| **NFR-SCALE-9** | 演进路径 | 10k+ WS → WS sticky routing；100k+ DAU → cron 持久化队列；push QPS > 100 → push 队列持久化 |

### Reliability & Availability

| ID | 需求 | 可测判据 |
|---|---|---|
| **NFR-REL-1** | 服务可用性 | 月度 ≥ 99.5% |
| **NFR-REL-2** | 触碰送达率 | ≥ 99%（WS + APNs 联合；监控端到端） |
| **NFR-REL-3** | 盲盒强一致 | 零重复领取（一票否决） |
| **NFR-REL-4** | WS 重连成功率 | ≥ 98% 在 5 秒内重连 |
| **NFR-REL-5** | 数据持久化 | 所有权威写必入 Mongo，禁止 fire-and-forget |
| **NFR-REL-6** | 优雅停机 | SIGTERM 后 30 秒内完成：停止接受新连接、完成在途 HTTP 请求、WS 连接断开（客户端 `session.resume` 自动恢复） |
| **NFR-REL-7** | 灾备 | Mongo 每日快照 + point-in-time recovery；Redis 重启可从 Mongo 重建（`offline_replayability`） |
| **NFR-REL-8** | APNs 失败重试 | 指数退避 3 次，最终失败记日志；HTTP 410 自动清理 token |

### Observability

| ID | 需求 | 可测判据 |
|---|---|---|
| **NFR-OBS-1** | 结构化日志 | 所有日志输出为 JSON，由 zerolog 驱动 |
| **NFR-OBS-2** | 请求关联 ID | HTTP 请求带 `requestId`、WS 消息带 `connId + eventId` |
| **NFR-OBS-3** | 必含字段 | `userId / connId / event / duration / build_version / config_hash` |
| **NFR-OBS-4** | 健康检查 | `/healthz` 多维（Mongo ping / Redis ping / WS hub goroutine count / last cron tick） |
| **NFR-OBS-5** | 核心指标 | 活跃 WS 连接数、`session.resume` QPS、触碰送达率、盲盒领取率、APNs 成功率、HTTP 5xx 率 |
| **NFR-OBS-6** | 告警阈值 | HTTP 5xx > 1% / 5min、WS 错误率 > 2% / 5min、WS 连接数 > 8,000 |
| **NFR-OBS-7** | 监控工具（MVP） | Uptime Robot（/healthz）+ zerolog JSON 导入日志平台（Loki / 本地）；无 Grafana 全套 |

### Compliance

| ID | 需求 | 可测判据 |
|---|---|---|
| **NFR-COMP-1** | PIPL 健康数据同意记录 | `user.consents.stepData = { acceptedAt, version, ipCountry }`（中国区） |
| **NFR-COMP-2** | HealthKit 用途合规 | 步数仅用于：状态推断、盲盒兑换、点数换算、反作弊；不得用于广告 / 第三方共享 / 排行榜 |
| **NFR-COMP-3** | APNs 推送规范 | 符合 Apple Push Guidelines：无垃圾推送、尊重免打扰时段、用户可拒绝 |
| **NFR-COMP-4** | 数据最小化 | 服务端仅存当日累计步数 + 本次增量；不存秒级明细 |
| **NFR-COMP-5** | 注销请求 SLA | MVP：30 天内人工处理；Growth：自动级联清理 |
| **NFR-COMP-6** | 未成年人规避（MVP） | App Store 年龄分级标记 17+ |

### Integration

| ID | 需求 | 可测判据 |
|---|---|---|
| **NFR-INT-1** | Sign in with Apple | 符合 Apple ID 登录规范；JWK 公钥动态拉取验签 |
| **NFR-INT-2** | APNs Provider API | 使用 HTTP/2（`sideshow/apns2`）；token-based authentication |
| **NFR-INT-3** | Universal Links | `.well-known/apple-app-site-association` 由 web 层正确服务（content-type application/json） |
| **NFR-INT-4** | CDN 资源 | 皮肤 PNG / manifest 托管 CDN；合理 `Cache-Control` header（皮肤 ≥ 1 周、manifest ≤ 1 天） |
| **NFR-INT-5** | 未来 App Store 内购（Vision） | `verifyReceipt` / App Store Server API 接入点 |

### Accessibility（Not Applicable Server-Side）

服务端无直接 accessibility 责任。客户端需支持 VoiceOver（Apple Watch 盲人辅助）；服务端响应字段避免图形化特殊字符以便客户端朗读。
