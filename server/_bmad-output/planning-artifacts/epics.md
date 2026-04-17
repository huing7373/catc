---
stepsCompleted:
  - step-01-validate-prerequisites
  - step-02-design-epics
  - step-03-create-stories
  - step-04-final-validation
status: complete
completedAt: 2026-04-17
inputDocuments:
  - C:/fork/cat/server/_bmad-output/planning-artifacts/prd.md
  - C:/fork/cat/server/_bmad-output/planning-artifacts/architecture.md
  - C:/fork/cat/server/_bmad-output/planning-artifacts/implementation-readiness-report-2026-04-16.md
  - C:/fork/cat/docs/backend-architecture-guide.md
workflowType: create-epics-and-stories
project_name: server
user_name: 开发者
date: 2026-04-16
---

# server - Epic Breakdown

## Overview

This document provides the complete epic and story breakdown for server（裤衩猫后端），decomposing the requirements from the PRD and Architecture decisions into implementable stories.

## Requirements Inventory

### Functional Requirements

**身份与鉴权（Identity & Authentication）**

- **FR1**: 用户可以使用 Sign in with Apple 登录，服务端签发 JWT access + refresh token
- **FR2**: 用户可以使用 refresh token 换取新的 access token（无需重新登录）
- **FR3**: 服务端可以将 refresh token 加入黑名单以吊销
- **FR4**: 用户可以注册 APNs device token（每台设备独立，与用户关联）
- **FR5**: 用户可以在 Watch 和 iPhone 上各自独立登录，服务端以 per-device 方式管理连接与 token
- **FR6**: 用户可以通过 WebSocket 升级请求在 header 携带 JWT 完成鉴权（FR1 鉴权能力的延伸）

**猫状态与运动（Cat State & Activity）**

- **FR7**: 用户可以上报自己猫的当前状态（idle / walking / running / sleeping），服务端持久化快照
- **FR8**: 用户可以在状态上报时附带当日累计步数和本次同步增量
- **FR9**: 服务端可以根据用户长时间无状态上报自动推断状态为 idle（服务端 FSM）
- **FR10**: 服务端可以按时间衰减规则将陈旧状态自动降级至 idle 或标记离线
- **FR11**: 服务端可以对异常步数增量（单次 > 10,000 步）打标记以便人工审查
- **FR12**: 服务端可以通过 HTTP `POST /state` 接受 iPhone 后台 HealthKit 的一次性状态上报

**好友关系（Friend Graph）**

- **FR13**: 用户可以生成好友邀请 token（24 小时过期、单次使用）
- **FR14**: 用户可以使用邀请 token 接受好友关系（双向建立）
- **FR15**: 用户可以解除与指定好友的关系
- **FR16**: 用户可以查看当前好友列表
- **FR17**: 用户可以屏蔽指定好友（双向不可见）
- **FR18**: 用户可以取消屏蔽
- **FR19**: 服务端可以限制每用户好友上限为 20 人
- **FR20**: 服务端可以限制每用户 24 小时内最多生成 10 个邀请 token
- **FR55**: 服务端可以生成基于 Universal Link 格式的好友邀请 URL

**房间与实时在场（Room & Real-time Presence）**

- **FR21**: 用户可以加入好友房间（最多 4 人同屏）并建立 WebSocket 长连接
- **FR22**: 用户加入房间时可以立即收到房间全量快照（所有成员当前猫状态 + 皮肤）
- **FR23**: 用户可以接收到房间内好友状态变化的实时广播（p99 ≤ 3 秒）
- **FR24**: 用户可以收到"好友上线"和"好友离线"事件通知
- **FR25**: 用户可以通过 WebSocket 断线后携带 `lastEventId` 执行 `session.resume` 恢复会话
- **FR51**: 用户可以离开房间（主动 / App 退到后台）且服务端可感知
- **FR52**: 服务端可以在房间人数已满（4 人）时拒绝新成员加入并返回明确错误码

**触碰 / 触觉社交（Touch / Haptic Social）**

- **FR26**: 用户可以向任意好友发送触碰事件（指定表情类型）
- **FR27**: 服务端可以在接收方 WebSocket 在线时通过 WS 送达触碰，否则降级为 APNs 送达
- **FR28**: 服务端可以强制执行 per-sender → per-receiver 60 秒 ≤ 3 次的触碰频次限流（超限静默丢弃）
- **FR29**: 服务端可以拦截发送方已被接收方屏蔽的触碰（不送达、不通知发送方）
- **FR30**: 服务端可以在接收方处于本地时区免打扰时段（默认 23:00-07:00）时将推送降级为静默推送

**盲盒与奖励（Blind Box & Rewards）**

- **FR31**: 服务端可以在用户挂机满 30 分钟**且当前无未领取盲盒**时为其投放一个新的盲盒（cron 触发；单槽位设计）
- **FR32**: 用户可以在累计步数达到盲盒解锁阈值后领取盲盒
- **FR33**: 服务端可以验证盲盒领取请求的合法性并保证每个盲盒最多被成功领取一次
- **FR34**: 用户可以查询当前盲盒库存（待解锁 / 已解锁 / 已领取）
- **FR35**: 用户可以在盲盒领取成功后收到奖励详情（皮肤 ID + 稀有度）
- **FR54**: 服务端可以在抽到已拥有皮肤时返回明确的结果（如折算点数）

**皮肤与定制（Skin & Customization）**

- **FR36**: 用户可以查询全部皮肤目录（含默认猫 + 可解锁皮肤）
- **FR37**: 用户可以查询自己已解锁的皮肤列表
- **FR38**: 用户可以将一款已解锁的皮肤装配给自己的猫
- **FR39**: 用户可以让好友看到自己装配的皮肤（随状态广播）

**账户管理（Account Management）**

- **FR47**: 用户可以请求注销账号（MVP 阶段标记 `deletion_requested`）
- **FR48**: 用户可以修改自己的 displayName
- **FR49**: 用户可以修改自己的 timezone 和免打扰时段
- **FR50**: 客户端可以主动上报设备时区变更

**运维与可靠性（Operations & Reliability）**

- **FR40**: 运维方可以通过 `GET /healthz` 获取服务端健康状态（含 Mongo / Redis / WS hub 存活）
- **FR41**: 服务端可以对同一用户的 WS 建连频率做限流
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

### NonFunctional Requirements

**Performance**

- **NFR-PERF-1**: 房间内好友状态广播延迟 p99 ≤ 3 秒
- **NFR-PERF-2**: HTTP bootstrap endpoint 响应时间 p95 ≤ 200ms
- **NFR-PERF-3**: WebSocket `session.resume` 响应时间 p95 ≤ 500ms
- **NFR-PERF-4**: WebSocket RPC 消息往返 p95 ≤ 200ms
- **NFR-PERF-5**: 状态衰减四档阈值 —— 0-15s 真实 / 15-60s 显示但 UI 弱化 / 1-5min 回落 idle / >5min 标记离线
- **NFR-PERF-6**: session.resume 缓存窗口 60 秒
- **NFR-PERF-7**: Cron 任务触发时延 —— 盲盒投放 ≤ 60s、衰减扫描周期 30s、冷启动检测周期 24h

**Security**

- **NFR-SEC-1**: 传输层加密 TLS 1.3 强制（HTTP + WS），无明文端口
- **NFR-SEC-2**: JWT 签名算法 RS256，支持双密钥轮换
- **NFR-SEC-3**: Access token 有效期 ≤ 15 分钟
- **NFR-SEC-4**: Refresh token 有效期 ≤ 30 天
- **NFR-SEC-5**: 密码管理：不存储（Sign in with Apple 唯一来源）
- **NFR-SEC-6**: Apple userId 哈希存储（不可逆），不回溯原始
- **NFR-SEC-7**: APNs device token 加密存储（Mongo field-level 加密或 KMS）
- **NFR-SEC-8**: 限流覆盖所有写入 endpoint（touch、invite、blindbox.redeem、WS connect）
- **NFR-SEC-9**: 幂等性去重 —— 所有权威写 RPC 按 eventId 去重（5 分钟窗口）
- **NFR-SEC-10**: 审计日志 —— 所有 token 签发 / 吊销 / 屏蔽 / 盲盒领取事件留痕（zerolog）

**Scalability**

- **NFR-SCALE-1**: MVP 部署目标单 binary、单实例
- **NFR-SCALE-2**: 代码无进程级全局状态 —— 所有共享状态入 Redis；禁用 `sync.Map` 存连接 / 计数器
- **NFR-SCALE-3**: 多副本就绪性 —— 仅改部署拓扑即可横向扩展，不改业务代码
- **NFR-SCALE-4**: 单实例 WS 并发连接上限 ≤ 10,000（超过即告警）
- **NFR-SCALE-5**: Per-user WS 建连限流 60 秒 ≤ 5 次
- **NFR-SCALE-6**: Per-sender-receiver 触碰限流 60 秒 ≤ 3 次
- **NFR-SCALE-7**: Per-user 邀请 token 限流 24 小时 ≤ 10 个
- **NFR-SCALE-8**: HTTP per-IP 全局限流 60 秒 ≤ 60 次
- **NFR-SCALE-9**: 演进路径 —— 10k+ WS → WS sticky routing；100k+ DAU → cron 持久化队列；push QPS > 100 → push 队列持久化

**Reliability & Availability**

- **NFR-REL-1**: 服务可用性月度 ≥ 99.5%
- **NFR-REL-2**: 触碰送达率 ≥ 99%（WS + APNs 联合）
- **NFR-REL-3**: 盲盒强一致 —— 零重复领取（一票否决）
- **NFR-REL-4**: WS 重连成功率 ≥ 98% 在 5 秒内
- **NFR-REL-5**: 数据持久化 —— 所有权威写必入 Mongo，禁止 fire-and-forget
- **NFR-REL-6**: 优雅停机 SIGTERM 后 30 秒内完成
- **NFR-REL-7**: 灾备 —— Mongo 每日快照 + point-in-time recovery；Redis 重启可从 Mongo 重建
- **NFR-REL-8**: APNs 失败重试 —— 指数退避 3 次，最终失败记日志；HTTP 410 自动清理 token

**Observability**

- **NFR-OBS-1**: 结构化日志 —— 所有日志输出为 JSON，由 zerolog 驱动
- **NFR-OBS-2**: 请求关联 ID —— HTTP 请求带 `requestId`、WS 消息带 `connId + eventId`
- **NFR-OBS-3**: 必含字段 —— `userId / connId / event / duration / build_version / config_hash`
- **NFR-OBS-4**: 健康检查多维（Mongo ping / Redis ping / WS hub goroutine count / last cron tick）
- **NFR-OBS-5**: 核心指标 —— 活跃 WS 连接数、`session.resume` QPS、触碰送达率、盲盒领取率、APNs 成功率、HTTP 5xx 率
- **NFR-OBS-6**: 告警阈值 —— HTTP 5xx > 1% / 5min、WS 错误率 > 2% / 5min、WS 连接数 > 8,000
- **NFR-OBS-7**: 监控工具（MVP）—— Uptime Robot `/healthz` + zerolog JSON；无 Grafana 全套

**Compliance**

- **NFR-COMP-1**: PIPL 健康数据同意记录 —— `user.consents.stepData = { acceptedAt, version, ipCountry }`（中国区）
- **NFR-COMP-2**: HealthKit 用途合规 —— 步数仅用于状态推断 / 盲盒兑换 / 点数换算 / 反作弊；禁用于广告 / 第三方共享 / 排行榜
- **NFR-COMP-3**: APNs 推送规范 —— 符合 Apple Push Guidelines：无垃圾推送、尊重免打扰时段、用户可拒绝
- **NFR-COMP-4**: 数据最小化 —— 服务端仅存当日累计步数 + 本次增量；不存秒级明细
- **NFR-COMP-5**: 注销请求 SLA —— MVP：30 天内人工处理；Growth：自动级联清理
- **NFR-COMP-6**: 未成年人规避（MVP）—— App Store 年龄分级标记 17+

**Integration**

- **NFR-INT-1**: Sign in with Apple —— 符合 Apple ID 登录规范；JWK 公钥动态拉取验签
- **NFR-INT-2**: APNs Provider API —— 使用 HTTP/2（`sideshow/apns2`）；token-based authentication
- **NFR-INT-3**: Universal Links —— `.well-known/apple-app-site-association` 由 web 层正确服务（content-type application/json）
- **NFR-INT-4**: CDN 资源 —— 皮肤 PNG / manifest 托管 CDN；合理 `Cache-Control` header（皮肤 ≥ 1 周、manifest ≤ 1 天）
- **NFR-INT-5**: 未来 App Store 内购（Vision）—— `verifyReceipt` / App Store Server API 接入点

### Additional Requirements

**架构宪法强制（`docs/backend-architecture-guide.md`）**

- 技术栈必须为：Go 1.25+ / Gin / gorilla/websocket / MongoDB (driver v2) / Redis (go-redis/v9) / zerolog / BurntSushi/toml / golang-jwt/v5 / sideshow/apns2 / robfig/cron/v3
- **禁用**：GORM、Postgres、golang-migrate、env+`.env.local`、glog、protobuf、自定义 TCP RPC、route_server/registry
- 目录结构必须为 `cmd/cat/` + `internal/{config,domain,service,repository,handler,dto,middleware,ws,cron,push}` + `pkg/{logx,mongox,redisx,jwtx,fsm,ids,clockx}`
- `cmd/cat/main.go` ≤ 15 行；`initialize.go` ≤ 200 行，是唯一的显式 DI 装配点
- 无 DI 框架、无 IoC 容器、无全局单例、无 `func init()` 做业务 I/O
- 分层单向依赖：handler → service → repository → infra；禁反向或跨层
- 接口消费方定义（"accept interfaces, return structs"），禁 `I` 前缀
- 对外错误格式：`{"error": {"code": "...", "message": "..."}}`（架构宪法 §7）；成功响应直接返回 payload 无 wrapper
- 所有 I/O 函数必传 `ctx context.Context`，禁 `context.TODO()`；`context.Background()` 仅用于启动期或 goroutine 根节点
- `sync.Map` / 全局变量 / singleton 禁存连接或计数器；所有共享状态入 Redis
- 每个 Redis `Set` 必须显式带 TTL（或注释原因）；key 字面量禁散落 → 集中在 `redis_keys.go`
- 每个 repository 实现 `EnsureIndexes(ctx) error`，由 `initialize()` 启动期调用一次
- typed IDs 强制：`UserID` / `SkinID` / `BlindboxID` / `FriendID` / `InviteTokenID` / `ConnID` / `RoomID` 全部走 `pkg/ids`
- Redis key 命名集中 `redis_keys.go`；Mongo domain ↔ BSON 转换在 repo 内部（repo 返回 `*domain.X`）
- 生产代码注释**统一英文**（P2 中英文混用是反例）
- 公开成员必须英文 godoc
- `// TODO(#123): ...` 必带 issue 号
- 测试模式：repository 必须 Testcontainers 集成测试，禁 DB mock；service 用 mock repo；handler 用 httptest + mock service；e2e 在 `test/e2e/`
- 测试覆盖率目标：service 层 ≥ 80%、repo / handler ≥ 60%
- 集成测试 `//go:build integration` tag；集成测试禁 `t.Parallel()`（Testcontainers 资源冲突）
- CI 门禁：`bash scripts/build.sh --test` 必过
- Runnable 接口规范：`Name() / Start(ctx) / Final(ctx)`；`Final` 必须幂等；注册顺序 Start，逆序 Final
- Graceful shutdown 顺序固定：HTTP Shutdown → WS Hub Stop → WS 现有连接发 close frame → cron.Stop → APNs worker 处理完 → repo 关闭 → zerolog flush；预算 ≤ 30s

**Architecture 决策（D1-D16 + ADR-001~003）**

- **D1**: MVP 单进程内存 WS Hub + `Broadcaster` 接口（Phase 3 Redis Pub/Sub 预留替换点）；接口签名必须含 `BroadcastToUser / BroadcastToRoom / PushOnConnect / BroadcastDiff`
- **D2**: 状态冲突解决采用 per-source 优先级 + 同 source 内 LWW；优先级 `watch_foreground > iphone_foreground > iphone_background_healthkit > server_inference`；`cat_states` 文档新增 `source` 字段
- **D3**: APNs 推送队列必须用 Redis Streams（`apns:queue`）+ worker goroutine 消费组；发推 `XADD`，消费 `XREADGROUP`，失败指数退避 3 次，终态失败入 `apns:dlq`
- **D4**: MVP 不使用 Mongo Change Streams，Service 层显式手动调用 `Broadcaster.BroadcastToRoom()`；docker-compose 保留 1 节点 replica set 为未来留门
- **D5**: Cron 调度采用 `robfig/cron/v3` + Redis SETNX 锁（key `lock:cron:{name}`、TTL 55s、持有人 instanceID）；所有 cron job 必须用 `withLock` 包裹
- **D6**: OP-1 设计方向反向约束 Hub 接口预留 `PushOnConnect` + `BroadcastDiff`；最终 OP-1 方案延至 Spike-OP1 后收敛
- **D7/ADR-001**: 服务端单方面衰减引擎 —— Cron `state_decay` 每 30s 扫描 Redis 热缓存 `state:*`，按 NFR-PERF-5 四档分别广播 `stale: weak` / `state.serverPatch` / `friend.offline`；拒绝客户端本地衰减
- **D8/ADR-002**: Presence key 结构为 `presence:{userId}` Redis Set，成员格式 `{deviceId}:{connId}:{instanceId}`；`presence:{userId}:meta` Hash 每成员记 `lastPing`；60s 未心跳由 cron 清理
- **D9/ADR-003**: WS Hub goroutine 目标上限 10,000；Spike-OP1 真机压测（1k/3k/5k/10k）验证；p99 > 3s 时降低上限 + 启动 Phase 3 sticky routing 规划；`ws.Hub` 配置暴露 `MaxConnections`
- **D10**: 事务边界规则 —— 盲盒领取（FR33）必须用 Mongo 事务（四 collection 原子：blindbox 状态、reward、点数、user_skins）；好友建立（FR14）必须用 Mongo 事务（双向 friends + friendCount 原子）；其他写单文档 upsert + Redis SETNX 5min 去重
- **D11**: 数据保留策略 —— `touch_logs` 90 天 TTL index；`invite_tokens` 24h TTL index；`event:{eventId}` Redis 5min EXPIRE；`presence` 成员 60s 心跳续期；`cat_states` 永久（账号未注销）
- **D12**: 时间戳权威源 —— 服务端 `updatedAt = time.Now()` UTC 为权威；客户端 `clientUpdatedAt` 仅做 D2 优先级冲突输入；多副本 NTP 同步由部署层保障
- **D13**: 配置分层 —— 基础设施（DB URL / 端口 / 密钥）走 TOML + 环境变量覆盖，重启生效；业务阈值（触碰限流、衰减档位、盲盒步数要求）走 Redis Hash `config:runtime`，动态生效
- **D14**: 可观测性栈 MVP —— zerolog JSON → stdout → Docker log driver → 本地文件滚动；metrics 预留 `/metrics` hook（Phase 3 Prometheus）；告警 Uptime Robot；无分布式追踪
- **D15**: Feature flag 双层 —— 代码分层（Growth / Vision 功能物理隔离）+ 有限运行时 flag（Redis `config:features`）；禁外部 flag 服务（LaunchDarkly 等）
- **D16**: 事件驱动只用 cron；异步事件复用 Redis Streams（D3 基础设施）；禁专用 task queue（asynq / machinery / NATS）

**实现模式（P1-P7 + M1-M16）—— 强度 tag 标记执行强度**

- **P1** MongoDB 规范 `[convention]`：collection `snake_case` 复数；BSON 字段 `snake_case`；聚合根 `_id` 业务 ID，关联表 `_id` Mongo ObjectId
- **P2** HTTP API 格式：路径 kebab-case 复数；成功响应直接 payload 无 wrapper；错误响应 `{"error": {"code": "...", "message": "..."}}` + 合适 HTTP status；时间 RFC 3339 UTC；分页 MVP 用 limit-only + hasMore；JSON camelCase
- **P3** WebSocket 消息命名 `[convention]`：type 格式 `domain.action`；响应 type 加 `.result`；推送 type 无 `.result`；心跳 `ping/pong`；错误码复用 HTTP error code 注册表；envelope id 为客户端唯一 string（非 UUID 强制）
- **P4** Error 5 档分类 `[compiler]`：`retryable / client_error / silent_drop / retry_after / fatal`；`AppError` 必须含 `Category` 字段；启动时校验每个 error code 都归档到一个 category
- **P5** Logging 规则：每条日志必含 `time/level/msg`；Context 字段 `requestId (HTTP) / connId + eventId (WS) / userId`；字段 camelCase；禁 `fmt.Printf / log.Printf / log.Println` → forbidigo 拦截
- **P6** 测试模式：xxx_test.go 同目录；集成测试 `//go:build integration` tag；table-driven 多场景必须；DB mock 禁用 → 必须 Testcontainers；errors.Is/As 不用字符串比较 → errorlint 拦截
- **P7** Request DTO 校验：位于 handler 层；`go-playground/validator/v10` 通过 Gin binding；validation error 转 `AppError{Code: "VALIDATION_ERROR", Category: client_error}`；service 层不重复校验，domain 层做业务不变量校验
- **M1** Package 命名 `[convention]`：单数小写短词（`user`、`blindbox`、`ws`）
- **M2** Interface 命名：Repository 接口 `<Entity>Repository`；单方法 utility 接口 `-er` 后缀
- **M3** Constructor 命名：默认 `NewXxx(deps...) *Xxx`；启动期失败即 panic 用 `MustXxx`；工厂方法 `<Adj><Type>` 如 `NewInMemoryBroadcaster`
- **M4** Context cancellation `[lint]`（revive）：长循环 / 阻塞调用必须 `select case <-ctx.Done()`
- **M5** Goroutine 生命周期：任何启动的 goroutine 必须有明确退出机制；禁 fire-and-forget；必须由 Runnable 管理（`sync.WaitGroup`，graceful shutdown 等待）
- **M6** Enum 命名：业务 string（JSON/BSON/WS payload）全小写（`idle / walking / running`）；Go 内部 typed const `<Domain><Value>` 模式
- **M7** Repository ↔ Service 边界：Repository 返回 `*domain.X`，不返 raw `bson.M`；BSON ↔ Domain 转换在 repo 内部
- **M8** DTO 转换位置：handler 层做 `DTO ↔ Domain Entity`；service 只接受 / 返回 domain entity
- **M9** Clock interface `[convention]`：所有时间获取通过可注入 Clock；禁直接调 `time.Now()` 在业务代码中；测试用 `FakeClock.Advance(d)` —— 这是 FR60 的实现契约
- **M10** 测试 helper 命名：`setupXxx(t) / teardownXxx(t) / assertXxx(t)`，强制 `t.Helper()`
- **M11** 集成测试禁并行 `[convention]`：集成测试禁用 `t.Parallel()`；单元测试默认开
- **M12** errors.Is/As `[lint]`：错误判断只能用 `errors.Is` 或 `errors.As`；禁字符串比较
- **M13** PII 日志规则：`userId` 可记；`displayName` / 邮箱必须 `[REDACTED]`；`pkg/logx` 提供 `MaskPII` helper
- **M14** APNs token 脱敏：日志中只显示前 8 字符 + `...`；仅 DEBUG level；INFO+ 不出现完整 token
- **M15** WS per-connection rate limit：每条 WS 连接 100 msg/s（token bucket）；超限 close conn + log
- **M16** defer close 紧邻原则：所有 `Open / Connect / Acquire` 后下一行写 `defer close`

**Multi-replica code invariants（5 条必守）**

1. 无 `sync.Map` / 全局变量 / singleton 存连接或状态
2. 所有共享状态在 Redis（`presence:*` / `ratelimit:*` / `event:*` / `blacklist:*`）
3. Cron 必须有分布式锁（Redis SETNX + TTL）
4. WS 广播不能假设目标连接在本实例（需 Pub/Sub fan-out 机制或 sticky routing 预留）
5. APNs 发送必须有持久化队列（非 goroutine chan）

**平台基线与开发工具链**

- 无外部 starter —— Epic 0 Walking Skeleton 手工交付（~15 Go 文件 + 配置 + Testcontainers 种子）
- CI 必须跑 2 副本 docker-compose 对关键路径（cron 去重、presence 多副本一致、广播 fan-out、APNs 队列去重）
- 开发工具链配置文件必须齐全：`.golangci.yml` / `.editorconfig` / `.gitignore` / `.dockerignore` / `Makefile` / `.github/workflows/ci.yml` / pre-commit hook / `deploy/docker-compose.yml`（Mongo 1 节点 replica set + Redis + app）
- `docs/code-examples/` 必须有 8 个标准模板（handler / ws_handler / service / repository / cron_job / error_codes / test_unit / test_integration）作为 Claude 编码 reference source of truth

**已知实施阶段 Gap（来自 readiness report G1-G6）**

- **G1** Universal Links 静态文件 + Nginx/Gin 配置 —— `deploy/well-known/apple-app-site-association` + 路由（Epic 0 / E3 阶段处理）
- **G2** OpenAPI / WS Registry 文档 CI 校验 —— `swagger validate docs/api/openapi.yaml`；WS registry 通过 unit test 扫 `dto/ws_messages.go` 自动核对
- **G3** APNs 证书（`.p8` 私钥 + key id + team id）存放方式 —— `.p8` 挂载 Docker volume；环境变量传 keyId/teamId/topic；`config/` 不含密钥
- **G4** Mongo replica set 初始化脚本 —— `deploy/mongo-init.js` 执行 `rs.initiate()`
- **G5** Spike-OP1 测试矩阵工具选型 —— k6 / vegeta / 自写 Go 客户端（在 Spike 内决策并归档 `docs/spikes/op1-ws-stability.md`）
- **G6** Go 1.25 + Gin 兼容性验证 —— Walking Skeleton 完成后 `go vet` 验证

**合规与用户安全落地（PRD §Domain-Specific Requirements）**

- Apple HealthKit 用途限定：状态推断 / 盲盒兑换 / 点数换算 / 反作弊；禁广告 / 第三方转卖 / 公开排行榜
- PIPL：步数属敏感个人信息，明示告知 + 单独同意；服务端记录同意凭证 `user.consents.stepData`
- 触碰反骚扰：per-sender → per-receiver 60s ≤ 3 次；超限静默丢弃（不告知发送方）
- 显式屏蔽双向不可见：B 的 `touch.send` 被拦截；B 看不到 A 的猫；A 看不到 B 的猫
- 跨时区免打扰：服务端维护每用户 timezone + 默认 23:00-07:00 本地时间；APNs 推送降级静默；WS 触碰消息打 `quietMode: true` 标记
- 好友配对防滥用：邀请 token 24h 过期 + 单次使用；同用户 24h 最多 10 个邀请 token；好友上限 20 人
- 盲盒客户端篡改防御：服务端校验步数增量，单次增量 > 10,000 步触发人工审查标记
- WS hub 内存泄漏缓解：30s 心跳超时自动断开 + goroutine 全局上限 + per-user 连接频率限流
- APNs token 失效：HTTP 410 → 删除 token + 定期清理 cron

**Apple 平台技术约束（影响实现策略）**

- watchOS 后台进程几秒内冻结 → WS 连接沦为僵尸；服务端状态衰减引擎必须单方面运行，不依赖客户端心跳
- APNs 静默推送配额 ~2-3 次/小时/设备 → 好友状态刷新不能依赖静默推送
- WatchConnectivity Watch↔iPhone 延迟可达 30s+ → 服务端不能假设两端状态同步实时，以各自直接上报为准
- iPhone 后台 HealthKit observer 窗口约 30s → HTTP `POST /state` 必须单次完成，不走 WS 握手
- 步数数据模型：`dailySteps` + `stepDelta` + `lastSyncAt`；不存逐秒步数明细

**Open Problems 追踪（设计阶段阻塞项）**

- **OP-1**（open）watchOS WS-primary 稳定性 —— 禁止 HTTP backup fallback（用户 feedback：不接受 backup 掩盖根因）；Spike-OP1 建立真机 + 弱网测试矩阵，量化 WS 重连 p50/p95/p99 延迟与电量消耗；E4 开发硬前置条件
- **OP-3** WS 广播 fan-out 策略（写 Mongo 前/后 trigger broadcast）—— Step 5 Patterns 决策（MVP Service 手动）
- **OP-4** `session.resume` 增量协议 shape（全量快照 vs 带 lastSeq 增量）—— Step 5 Patterns 决策（MVP 先全量）
- **OP-7** i18n 时序（错误 message 国际化、APNs 推送文案）—— Vision 阶段

### UX Design Requirements

**Not Applicable** —— 本 PRD 范围显式限定 `server-only`，客户端 UX 设计（Apple Watch + iPhone 的 SwiftUI 视图、SpriteKit 场景、触觉模式）不在本后端项目的责任范围。

服务端仅暴露与客户端 UX 约定一致的契约：

- `friend.state` 广播包含 `stale` 标记供 UI 分层展示（`presence_decay_truthfulness`）
- 触碰表情类型 `emoteType` 枚举由客户端 UX 阶段定义后回填服务端
- 衰减四档阈值（NFR-PERF-5）为 UI "活着 / 休眠 / 离线" 分层的数据契约

未来客户端 UX 工作流（如 `/bmad-create-ux-design` 或 `/wds-4-ux-design`）独立启动时需做 UX ↔ 本 epics 的对齐检查，回填 emoteType 等共享枚举。

### FR Coverage Map

60/60 FR = 100% 覆盖（FR44 拆分为 FR44a/b，FR53 已废弃）。

| FR | Epic | 一句话说明 |
|---|---|---|
| FR1 | Epic 1 | Sign in with Apple 登录签发 JWT |
| FR2 | Epic 1 | refresh token 换 access token |
| FR3 | Epic 1 | refresh token 黑名单吊销 |
| FR4 | Epic 1 | APNs device token 注册 endpoint（路由执行在 Epic 0） |
| FR5 | Epic 1 | Watch + iPhone per-device 独立登录 |
| FR6 | Epic 1 | WS upgrade header 携带 JWT 鉴权 |
| FR7 | Epic 2 | 上报猫状态（idle/walking/running/sleeping）持久化 |
| FR8 | Epic 2 | 状态上报附带 dailySteps + stepDelta |
| FR9 | Epic 2 | 长时间无上报自动推断 idle（服务端 FSM） |
| FR10 | Epic 2 | 按时间衰减降级陈旧状态 |
| FR11 | Epic 2 | 异常步数增量 > 10k 打标审查 |
| FR12 | Epic 2 | HTTP `POST /state`（HealthKit 后台 30s 窗口） |
| FR13 | Epic 3 | 生成邀请 token（24h 过期 + 单次） |
| FR14 | Epic 3 | 接受邀请 token 建双向好友 |
| FR15 | Epic 3 | 解除好友 |
| FR16 | Epic 3 | 好友列表 |
| FR17 | Epic 3 | 屏蔽好友（双向不可见） |
| FR18 | Epic 3 | 取消屏蔽 |
| FR19 | Epic 3 | 好友上限 20 人 |
| FR20 | Epic 3 | 邀请 token 24h ≤ 10 个 |
| FR21 | Epic 4 | 加入好友房间（≤ 4 人）建 WS 长连 |
| FR22 | Epic 4 | 进房立即收 `room.snapshot` 全量 |
| FR23 | Epic 4 | 房间内好友状态实时广播 p99 ≤ 3s |
| FR24 | Epic 4 | 好友上线/离线事件 |
| FR25 | Epic 4 | `session.resume` + `lastEventId` 恢复 |
| FR26 | Epic 5 | 向好友发送触碰（emoteType） |
| FR27 | Epic 5 | WS 在线送达，离线降级 APNs |
| FR28 | Epic 5 | 触碰限流 60s ≤ 3 次静默丢弃 |
| FR29 | Epic 5 | 屏蔽场景拦截触碰 |
| FR30 | Epic 5 | 免打扰时段降级静默推送 |
| FR31 | Epic 6 | 挂机 30 分钟 + 单槽位盲盒投放 |
| FR32 | Epic 6 | 步数达标领取盲盒 |
| FR33 | Epic 6 | 盲盒强一致领取（零重复，Mongo 事务 + Redis 幂等） |
| FR34 | Epic 6 | 盲盒库存查询 |
| FR35 | Epic 6 | 领取后返回奖励详情 |
| FR36 | Epic 7 | 皮肤目录 |
| FR37 | Epic 7 | 已解锁皮肤列表 |
| FR38 | Epic 7 | 装配皮肤 |
| FR39 | Epic 7 | 好友看到装配的皮肤（随状态广播） |
| FR40 | Epic 0 | `/healthz` 多维探针 |
| FR41 | Epic 0 | WS 建连频率限流 |
| FR42 | Epic 0 | `session.resume` 节流缓存 |
| FR43 | Epic 0 | APNs 410 → 自动删除 token |
| FR44a | Epic 8 🟡 | 识别冷启动用户（48h + 0 好友）— 待评估 |
| FR44b | Epic 8 🟡 | 冷启动召回推送 — 待评估 |
| FR45 | Epic 0 | 异常设备拒绝 WS 连接 |
| FR46 | Epic 0 | 结构化 JSON 日志（含必含字段） |
| FR47 | Epic 1 | 注销标记 |
| FR48 | Epic 1 | 修改 displayName |
| FR49 | Epic 1 | 修改 timezone + 免打扰时段 |
| FR50 | Epic 1 | 客户端上报时区变更 |
| FR51 | Epic 4 | 离开房间服务端感知 |
| FR52 | Epic 4 | 房间已满拒绝新成员 |
| FR54 | Epic 6 | 抽到已拥有皮肤返回折算结果 |
| FR55 | Epic 3 | Universal Link 邀请 URL 生成 |
| FR56 | Epic 0 | Cron 分布式锁单一执行 |
| FR57 | Epic 0 | WS 上行 eventId 去重 |
| FR58 | Epic 0 | APNs 按 platform 路由 |
| FR59 | Epic 0 | 客户端查询支持的 WS 消息类型与版本 |
| FR60 | Epic 0 | 虚拟时钟（Clock interface）测试钩子 |

## Epic List

### Epic 0: 服务端骨架与平台基线
**Platform epic（显式不交付 user value，但所有其他 epic 的地基）。** 交付 Walking Skeleton（~15 Go 文件 + 工具链 + Testcontainers 种子）+ 横切平台能力：健康检查、WS 建连限流、session.resume 缓存节流、APNs 队列路由（Redis Streams + platform 路由 + 410 token 清理）、异常设备拒连、结构化日志、cron 分布式锁、WS eventId 去重、WS 消息类型版本查询、虚拟时钟测试钩子。含 Spike-OP1 watchOS WS 稳定性验证（Epic 4 硬前置）。
**FRs covered:** FR40, FR41, FR42, FR43, FR45, FR46, FR56, FR57, FR58, FR59, FR60

### Epic 1: 身份与账户
用户可以通过 Sign in with Apple 登录、在 Watch + iPhone 各自独立保持会话、使用 JWT（含 refresh）维持 WS 鉴权、注册 APNs device token；用户可管理 displayName / timezone / 免打扰时段、上报设备时区变更、发起账号注销。
**FRs covered:** FR1, FR2, FR3, FR4, FR5, FR6, FR47, FR48, FR49, FR50

### Epic 2: 你的猫活着（状态映射 + 步数）
用户的猫映射本人真实运动（idle / walking / running / sleeping），步数（dailySteps + stepDelta）随状态上报被安全存储；陈旧状态在服务端按四档衰减规则自动降级至 idle 或标记离线；异常步数增量（单次 > 10k）被标记人工审查；iPhone 后台 HealthKit 通过 HTTP `POST /state`（30s 特例窗口）单次上报。
**FRs covered:** FR7, FR8, FR9, FR10, FR11, FR12

### Epic 3: 好友圈
用户可以生成 / 接受好友邀请 token（24h 过期、单次使用、Universal Link 格式）、查看好友列表、解除好友、屏蔽 / 取消屏蔽（双向不可见）；系统强制执行好友上限 20 人、邀请 token 24h ≤ 10 个的防滥用限制。
**FRs covered:** FR13, FR14, FR15, FR16, FR17, FR18, FR19, FR20, FR55

### Epic 4: 好友房间 / 环境在场感
用户加入好友房间（最多 4 人同屏）时立即收到房间全量快照；房间内好友的状态变化以 p99 ≤ 3s 实时广播（产品心脏）；用户可收到好友上下线事件；WS 断线后通过 `session.resume` + `lastEventId` 恢复会话；用户主动或退到后台时服务端感知离开；房间已满时拒绝新成员。**硬前置：Epic 0 的 Spike-OP1 story 必须收敛出 OP-1 watchOS WS-primary 设计方案后才能启动。**
**FRs covered:** FR21, FR22, FR23, FR24, FR25, FR51, FR52

### Epic 5: 触碰 / 触觉社交
用户可以向好友发送触碰事件（指定表情类型），接收方在线时 WS 送达 `friend.touch`，离线时降级 APNs；服务端强制执行 per-sender→receiver 60s ≤ 3 次限流（超限静默丢弃、不告知发送方）、屏蔽拦截、接收方本地免打扰时段降级为静默推送。
**FRs covered:** FR26, FR27, FR28, FR29, FR30

### Epic 6: 盲盒（掉落 + 领取）
用户挂机满 30 分钟（且当前无未领取盲盒时）获得新盲盒（单槽位 cron 投放）；步数达到解锁阈值后可领取盲盒，领取走 Mongo 事务 + Redis SETNX 幂等去重，保证每盒最多成功领取一次（零重复 - 一票否决）；领取成功返回奖励详情（皮肤 ID + 稀有度）；抽到已拥有皮肤时返回折算点数；用户可查询盲盒库存（待解锁 / 已解锁 / 已领取）。
**FRs covered:** FR31, FR32, FR33, FR34, FR35, FR54

### Epic 7: 皮肤与定制
用户可以查询全部皮肤目录（含默认猫 + 可解锁皮肤）、自己已解锁的皮肤列表、装配一款已解锁的皮肤给自己的猫；装配结果随房间状态广播被好友看到（`cat_state.skinId` 在 `friend.state` payload 内下发）。FR36-38 可独立交付用户价值；FR39 在 Epic 4 完成后自动点亮。
**FRs covered:** FR36, FR37, FR38, FR39

### Epic 8: 冷启动召回 🟡 Optional / Deferred
**⚠️ 待评估是否实现。不阻塞任何其他 Epic。** 服务端通过 cron（24h 周期）识别注册 ≥ 48h 且好友数 = 0 的冷启动用户，通过 APNs 召回推送（"邀请一个朋友，解锁好友猫功能"+ 深度链接）引导其生成邀请链接，缓解 J3 冷启动旅程的流失漏斗。评估维度：MVP 30 天 D3 自然留存是否健康、推送骚扰对卸载率的反向影响、NFR-COMP-3 APNs Guidelines 合规。
**FRs covered:** FR44a, FR44b

---

## Epic 0: 服务端骨架与平台基线

**Platform epic（显式不交付 user value，但所有其他 epic 的地基）。** 交付 Walking Skeleton（~15 Go 文件 + 工具链 + Testcontainers 种子）+ 横切平台能力：健康检查、WS 建连限流、session.resume 缓存节流、APNs 队列路由、异常设备拒连、结构化日志、cron 分布式锁、WS eventId 去重、WS 消息类型版本查询、虚拟时钟测试钩子。含 Spike-OP1 watchOS WS 稳定性验证（Epic 4 硬前置）。

### Story 0.1: 项目骨架与开发工具链

As a maintainer,
I want to establish the repo skeleton and dev toolchain per backend-architecture-guide §3,
So that every subsequent story lands on the same foundation and `bash scripts/build.sh --test` gates every PR.

**Acceptance Criteria:**

**Given** repo 当前仅含 `scripts/build.sh` 和 `docs/backend-architecture-guide.md`，无 Go 源码
**When** 按架构宪法 §3 创建目录结构并初始化工具链
**Then** `server/go.mod` 存在，模块路径符合宪法命名（如 `github.com/huing/cat/server`）
**And** `server/` 下按宪法 §3 含 `cmd/cat/` + `internal/{config,domain,service,repository,handler,dto,middleware,ws,cron,push}` + `pkg/{logx,mongox,redisx,jwtx,ids,fsm,clockx}` 目录（每个目录至少含一个 `.gitkeep` 或占位文件）
**And** `config/` 含 `default.toml` + `local.toml.example`（仅字段骨架，具体字段由后续 story 填充）
**And** `deploy/docker-compose.yml` 启动 Mongo（1-node replica set，为事务 / Change Streams 预留）+ Redis + app 三个服务；`deploy/Dockerfile` 基于 `golang:1.25-alpine` 多阶段构建
**And** 根目录含 `.golangci.yml` 启用 `errcheck / errorlint / forbidigo / gocritic / revive / unconvert / unparam / misspell / bodyclose / gofmt / goimports / govet / unused / ineffassign`；forbidigo 规则拦截 `^fmt\.(Printf|Println)$` 与 `^log\.(Print|Println|Printf)$`
**And** 根目录含 `.editorconfig`（tab 缩进）+ `.gitignore`（`build/`、`*.out`、`.env.local`、IDE 目录）+ `.dockerignore`（`.git`、`build/`、`*.md`、`*_test.go`）
**And** `Makefile` 提供 `build / test / lint / docker-up / docker-down / ci` 目标
**And** `.github/workflows/ci.yml` 执行顺序：`golangci-lint run` → `bash scripts/build.sh` → `bash scripts/build.sh --test` → `bash scripts/build.sh --race --test`
**And** pre-commit hook（lefthook 配置或 shell 脚本）在 commit 前自动运行 `gofmt` + `goimports` + `go vet`
**And** `cmd/cat/main.go` 含 placeholder `func main() {}`，`bash scripts/build.sh` 成功产出 `build/catserver`
**And** `go vet ./...` 通过无 warning
**And** CI workflow 在本 story 合入后第一次绿灯

### Story 0.2: 应用入口与 Runnable 生命周期

As a maintainer,
I want the app entry point wired with explicit DI and graceful shutdown per constitution §4-§5,
So that all future Runnable components (HTTP server, WS hub, cron scheduler, APNs worker) plug into one lifecycle with deterministic startup and shutdown order.

**Acceptance Criteria:**

**Given** Story 0.1 交付的 repo 骨架
**When** 实现 `cmd/cat/{main.go, initialize.go, app.go, wire.go}` + `internal/config/config.go`
**Then** `main.go` 行数 ≤ 15（严格，宪法 §4），仅做 `flag.Parse → config.MustLoad → initialize → app.Run`
**And** `initialize.go` 是唯一的显式 DI 装配点，行数 ≤ 200，无 DI 框架、无 IoC 容器
**And** 定义 `Runnable` 接口含 `Name() string / Start(ctx context.Context) error / Final(ctx context.Context) error`；`Final` 幂等
**And** `App` 容器按注册顺序并发 Start，收到 SIGTERM/SIGINT 后逆序 Final
**And** `signal.Notify` 监听 `os.Interrupt, syscall.SIGTERM`；shutdown 总耗时预算 ≤ 30 秒（宪法 §5）
**And** 无 `func init()` 做业务 I/O（code review 人工校验 + CI 辅助 grep）
**And** 无 `sync.Map` / 全局变量 / singleton 存连接或计数器（CI 跑 `grep -rn "var .*sync.Map" internal/ pkg/` 断言只在 `internal/ws/hub.go` 白名单内出现）
**And** `internal/config/config.go` 使用 `BurntSushi/toml` 实现 `MustLoad(path)` + 基础字段空值校验；配置仅在 `initialize()` 读取一次
**And** 启动日志首行 info 级输出 `build_version`（go build -ldflags）+ `config_hash`（config 文件 SHA256 前 8 位）
**And** 集成测试：`./build/catserver -config config/local.toml` 启动后 `kill -TERM <pid>` 能在 30s 内优雅退出，退出码 0
**And** `docs/code-examples/` 占位（后续 story 填充内容）

### Story 0.3: 基础设施连通性与客户端

As a maintainer,
I want Mongo + Redis + JWT clients with `MustConnect` + `HealthCheck` + transaction helper,
So that all repos and services route I/O through controlled clients and integration tests boot via Testcontainers.

**Acceptance Criteria:**

**Given** Story 0.2 交付的 Runnable + config 加载能力
**When** 实现 `pkg/mongox/{client.go, transaction.go}` + `pkg/redisx/client.go` + `pkg/jwtx/manager.go` + `internal/testutil/{mongo_setup.go, redis_setup.go}`
**Then** `mongox.MustConnect(cfg) *mongo.Client` 内部完成 Connect + Ping + timeout 控制，失败 `log.Fatal`
**And** `redisx.MustConnect(cfg) *redis.Client` 内部完成 Ping，失败 `log.Fatal`
**And** 两客户端暴露 `HealthCheck(ctx context.Context) error` 方法供 Story 0.4 的 healthz 复用
**And** `mongox.WithTransaction(ctx, cli, fn func(mongo.SessionContext) error) error` 封装 `StartSession` + `WithTransaction`（D10 事务辅助）
**And** `jwtx.New(cfg)` 初始化 RS256 双密钥（含 kid 轮换字段），实现 `Issue(claims) (string, error)` 和 `Verify(token) (*Claims, error)`
**And** `internal/testutil/` 提供 `SetupMongo(t *testing.T) (*mongo.Client, func())` + `SetupRedis(t *testing.T) (*redis.Client, func())` 通过 Testcontainers 启动一次性容器（M10 强制 `t.Helper()`；M11 禁 `t.Parallel()`）
**And** 示范集成测试 `pkg/mongox/client_test.go` + `pkg/redisx/client_test.go` 文件顶部含 `//go:build integration` tag，通过 Testcontainers 完成 Connect + Ping 验证
**And** `bash scripts/build.sh --test --integration` 能触发集成测试且通过
**And** 任何 handler / service 代码禁止直接持有 `*mongo.Client` / `*redis.Client`（宪法 §6，code review 强制）

### Story 0.4: 多维健康检查 endpoint

As an operator,
I want `GET /healthz` + `/readyz` exposing multi-dimensional health state,
So that Uptime Robot and I can instantly diagnose the root cause when paged at 3 AM (J4 ops journey).

**Acceptance Criteria:**

**Given** Story 0.3 交付的 Mongo/Redis 客户端 + Story 0.9 将交付的 WS Hub（占位，本 story 只校验 interface 可达）
**When** 实现 `internal/handler/health_handler.go`
**Then** `GET /healthz` 返回 200 + JSON `{"status":"ok","mongo":"ok","redis":"ok","wsHub":"ok","lastCronTick":"<RFC3339>"}` 当所有维度健康
**And** Mongo ping 失败时 `/healthz` 返回 503 + JSON 对应字段为 `"error: <reason>"`，其余字段保持真实值
**And** Redis ping 失败 → 返回 503；WS hub goroutine count 超过 `cfg.WS.MaxConnections`（目标 10k，NFR-SCALE-4）→ 返回 503
**And** `GET /readyz` 仅检查"进程是否 ready 接受流量"（内存中的启动完成标志），不触发外部依赖调用
**And** 两 endpoint 不需要鉴权（NFR-OBS-4）
**And** 两 endpoint 响应时间 p95 ≤ 50ms（内部探针，不含网络延迟）（集成测试断言）
**And** 单元测试覆盖四种状态：全健康 / Mongo 故障 / Redis 故障 / wsHub goroutine 超限
**And** 集成测试（Testcontainers）停掉 Mongo 容器后 `/healthz` 返回 503 且服务不 panic（宪法 §19）

### Story 0.5: 结构化日志与请求关联 ID

As an operator,
I want all logs in zerolog JSON with `requestId / userId / connId / event / duration / build_version / config_hash` fields and PII auto-masked,
So that I can grep-and-jq through 3 AM WS hub incident logs to locate users and requests (NFR-OBS-1~3, FR46).

**Acceptance Criteria:**

**Given** Story 0.1 & 0.2 交付的目录和入口
**When** 实现 `pkg/logx/{logx.go, pii.go}` + `internal/middleware/{request_id.go, logger.go, recover.go}`
**Then** `logx.Init(cfg.Log)` 配置 zerolog 为 JSON output（生产）或 `ConsoleWriter`（`cfg.Log.Format == "console"`，仅 dev）
**And** `logx.Ctx(ctx) *zerolog.Logger` 自动带出 context 中的 `requestId / userId / connId` 字段
**And** `logx.MaskPII(s string) string` 将 displayName / 邮箱等替换为 `[REDACTED]`（M13）
**And** `logx.MaskAPNsToken(s string) string` 只保留前 8 字符 + `...`；INFO+ level 日志不得出现完整 token（M14）
**And** `middleware.RequestID` 从 `X-Request-ID` header 读取或生成 UUID，注入 context + 响应 header
**And** `middleware.Logger` 记录每次 HTTP 请求一条 info 日志：`method / path / status / duration_ms / userId（如有）`
**And** `middleware.Recover` 捕获 panic → zerolog `error` 级 + stack + 返回 `AppError{Code: "INTERNAL_ERROR"}` 500
**And** 所有日志含 `build_version` + `config_hash`（启动期通过 zerolog global context 注入）
**And** Lint 规则 forbidigo 拦截 `fmt.Printf / fmt.Println / log.Printf / log.Println`（CI 失败则阻断合并）
**And** `docs/code-examples/` 提供 `handler_example.go` 和 `service_example.go` 展示 `logx.Ctx(ctx).Info().Str().Msg()` field API 用法
**And** 单元测试：PII mask 正确 / APNs token mask 仅 DEBUG 暴露完整 token / requestId 从 header 继承 / requestId 缺失时生成 UUID

### Story 0.6: AppError + 错误分类注册表

As a developer,
I want a compiler-enforced AppError type with 5-tier Category classification and a complete error code registry,
So that the client can branch on Category (retry / prompt / logout / silent) and server audit logs stay complete (P4, NFR-SEC-10).

**Acceptance Criteria:**

**Given** Story 0.5 交付的 logx
**When** 实现 `internal/dto/{error.go, error_codes.go, error_codes_test.go}`
**Then** `AppError` 结构体字段 `Code string / Message string / HTTPStatus int / Category ErrCategory / Cause error`，Category 必填（编译期约束 —— 构造函数强制参数）
**And** `ErrCategory` 枚举含 5 档：`retryable / client_error / silent_drop / retry_after / fatal`
**And** `AppError.Error() / Unwrap()` 实现符合 `errors.Is/As` 语义（M12）
**And** `dto/error_codes.go` 注册 MVP 所有错误码至少含：`AUTH_INVALID_IDENTITY_TOKEN / AUTH_TOKEN_EXPIRED / AUTH_REFRESH_TOKEN_REVOKED / FRIEND_ALREADY_EXISTS / FRIEND_LIMIT_REACHED / FRIEND_INVITE_EXPIRED / FRIEND_INVITE_USED / FRIEND_BLOCKED / BLINDBOX_ALREADY_REDEEMED / BLINDBOX_INSUFFICIENT_STEPS / BLINDBOX_NOT_FOUND / SKIN_NOT_OWNED / RATE_LIMIT_EXCEEDED / DEVICE_BLACKLISTED / INTERNAL_ERROR / VALIDATION_ERROR / UNKNOWN_MESSAGE_TYPE`（后续 story 按需追加）
**And** 每个注册错误码必须关联一个 Category；启动期 `init()` 检查遗漏时 `log.Fatal`；单元测试扫描 registry 断言无漏档（P4 `[test]` 强度）
**And** `dto.RespondAppError(c *gin.Context, err error)` 响应格式 `{"error": {"code": "...", "message": "..."}}`（宪法 §7）+ 映射 `HTTPStatus` + `zerolog.Ctx(ctx).Error().Err(cause).Str("code", ...).Msg("app error")`；`Cause` 不回客户端
**And** `docs/error-codes.md` 人类可读注册表；CI 单元测试校验与 `dto/error_codes.go` 常量一致（G2 gap 修复）
**And** 单元测试覆盖：每个 Category → HTTP status 映射正确 / `errors.Is(err, sentinel)` 判断正确 / `Cause` 不泄露给客户端 JSON / `errors.As` 解包正确

### Story 0.7: Clock interface 与虚拟时钟

As a developer,
I want all business code to obtain time via an injectable Clock, with FakeClock for deterministic decay testing,
So that time-sensitive logic (state decay, cold-start recall, token expiry) can be unit-tested without sleeping (M9, FR60).

**Acceptance Criteria:**

**Given** Story 0.1 交付的 `pkg/` 目录
**When** 实现 `pkg/clockx/clock.go`
**Then** 定义 `Clock interface { Now() time.Time }`
**And** `RealClock{}` 实现 `Now() time.Time { return time.Now().UTC() }`（D12 时间戳 UTC 权威）
**And** `FakeClock` 含 `Advance(d time.Duration)` 方法；构造支持指定初始时间
**And** Lint / grep 规则禁止 `internal/{service, cron, push, ws, handler}` 下的业务代码直接调 `time.Now()`（CI 扫描脚本断言）
**And** `initialize.go` 装配一个 `RealClock` 实例并传给所有 Service 构造函数；后续 story 的 Service 构造签名必须包含 `clock clockx.Clock` 参数
**And** 示范代码 `docs/code-examples/service_example.go` 展示 service 持 Clock 依赖（含 FakeClock 单元测试示范）
**And** 单元测试：`FakeClock` 初始时间正确 / `Advance(15 * time.Second)` 后 `Now()` 返回正确值 / 多次 Advance 累加
**And** Story 0.14 的 WS 消息类型版本查询 endpoint 返回的时间戳使用 Clock（验证 Clock 真的被业务代码使用）

### Story 0.8: Cron 调度 + 分布式锁

As a platform engineer,
I want all cron jobs protected by a Redis SETNX lock so that multi-replica deployments trigger each job exactly once,
So that blindbox drops / state decay / cold-start checks don't double-fire and corrupt state (NFR-SCALE-2/3, D5, FR56).

**Acceptance Criteria:**

**Given** Story 0.3 的 Redis 客户端 + Story 0.7 的 Clock
**When** 实现 `internal/cron/scheduler.go` + `pkg/redisx/locker.go`
**Then** `redisx.Locker` 暴露 `WithLock(ctx, name string, ttl time.Duration, fn func() error) error`
**And** 锁 key 格式 `lock:cron:{name}`，默认 TTL 55 秒（留 5s 边际），value 为 instanceID（启动期随机生成 UUID）
**And** 加锁实现 `SET key value NX EX 55`；未抢到锁直接返回 `nil`（不报错，静默跳过）
**And** 释放用 Lua 脚本 CAS（`if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) end`）避免误删其他实例的锁
**And** `cron/scheduler.go` 基于 `robfig/cron/v3`；`RegisterJobs(sch, deps)` 辅助函数自动用 `WithLock` 包装 job
**And** Scheduler 实现 Runnable（`Name()="cron_scheduler" / Start / Final`）；Final 调用 `cron.Stop()` 并等待当前任务完成
**And** 示范 cron job `heartbeat_tick`（每 1 分钟记一条 info 日志，更新 `cron:last_tick` Redis key），被 Story 0.4 的 `/healthz` 消费
**And** CI 跑 2 副本 docker-compose 对 `heartbeat_tick` 验证：2 个实例同时跑 3 分钟，触发次数 = 3（不是 6）
**And** 单元测试：锁冲突场景 / TTL 过期自动释放 / CAS 释放正确 / 长任务 panic 时锁最终 TTL 自然释放（不阻塞下次调度）
**And** 示范代码 `docs/code-examples/cron_job_example.go` 展示 `withLock` 包裹模式 + ctx 检查（M4）

### Story 0.9: WS Hub 骨架 + Envelope + Broadcaster 接口

As a platform engineer,
I want the full WS stack (upgrade → envelope → dispatcher → hub → broadcaster interface) in place with an InMemory implementation and Phase 3 Pub/Sub replacement point,
So that all business WS handlers (FR21-FR39) can plug into one unified routing and broadcast layer (D1, D6, M15).

**Acceptance Criteria:**

**Given** Story 0.2 的 Runnable + Story 0.3 的 Redis + Story 0.5 的 logx + Story 0.6 的 AppError
**When** 实现 `internal/ws/{hub.go, envelope.go, dispatcher.go, upgrade_handler.go, broadcaster.go, rate_limit.go}`
**Then** `/ws` endpoint 走 HTTP upgrade；upgrade 前校验 `Authorization: Bearer <JWT>` header，jwtx 验证失败返回 401 + `AppError{Code: "AUTH_TOKEN_EXPIRED / AUTH_INVALID_IDENTITY_TOKEN"}`
**And** Envelope 类型定义：上行 `{id: string, type: string, payload: json.RawMessage}`；下行响应 `{id, ok: bool, type: "<req>.result", payload, error?: {code, message}}`；推送 `{type, payload}`；心跳 `{type: "ping" | "pong"}`
**And** Envelope `id` 是客户端唯一 string（P3 不强制 UUID）
**And** `Broadcaster` 接口签名：`BroadcastToUser(ctx, userID, msg) / BroadcastToRoom(ctx, roomID, msg) / PushOnConnect(ctx, connID, userID) / BroadcastDiff(ctx, userID, diff)`（D6 OP-1 接口预留）
**And** `InMemoryBroadcaster` 实现 Broadcaster，基于 Hub 内部 `sync.Map` 存 `connID → *Client`（白名单：仅 `internal/ws/hub.go` 允许 `sync.Map`）
**And** Hub 实现 Runnable；`Final` 对所有在线连接发 close frame，5 秒内完成
**And** 每 WS 连接两 goroutine（readPump / writePump）+ 带缓冲 channel；backpressure：`send` chan 满则关闭该连接（宪法 §12）
**And** 心跳：服务端每 30s 向每 client 发 ping；60s 无 pong 关闭连接
**And** Per-conn rate limit 100 msg/s token bucket（M15）；超限 close + zerolog warn
**And** `Dispatcher` 按 `envelope.type` 路由到注册的 handler；未知 type 返回 `{ok: false, error: {code: "UNKNOWN_MESSAGE_TYPE"}}`
**And** Echo 验证：提供 `debug.echo` 消息类型（仅 `cfg.Server.Mode == "debug"` 注册），Hub 原样返回 payload
**And** 集成测试：建连 → JWT 校验通过 → 发 `debug.echo` → 收到相同 payload；kill server 时客户端收到 close frame
**And** `docs/code-examples/ws_handler_example.go` 展示标准 WS handler 模式（envelope 解析 → service → ack/error）

### Story 0.10: WS 上行 eventId 幂等去重

As a developer,
I want every authoritative WS write to be deduplicated via Redis SETNX on eventId with result caching,
So that retries and client replay bugs don't cause double-redeemed blindboxes or double-delivered touches (NFR-REL-3, NFR-SEC-9, FR57).

**Acceptance Criteria:**

**Given** Story 0.9 交付的 Envelope + Dispatcher
**When** 扩展 Dispatcher 接入幂等中间件
**Then** Dispatcher 支持注册 handler 时声明 `requiresDedup: true/false`
**And** 对权威写 RPC（后续 stories 中的 `blindbox.redeem / touch.send / friend.accept / friend.delete / friend.block / friend.unblock / skin.equip / profile.update`）强制开启 dedup
**And** 去重逻辑：`SET event:{eventId} "processing" NX EX 300`（5 分钟窗口，NFR-SEC-9）；若返回 false，从 `event_result:{eventId}` Redis Hash 读取上次响应直接返回（含成功和失败）
**And** 首次执行成功/失败后，result 写入 `event_result:{eventId}` Redis Hash（TTL 5min），并 `SET event:{eventId} "done"`
**And** 非权威写消息（`users.me / friends.list / friends.state / skins.catalog / blindbox.inventory / session.resume / ping`）不走 dedup
**And** eventId 空值情况：客户端未带 id 时 dispatcher 返回 `{ok: false, error: {code: "VALIDATION_ERROR", message: "envelope.id required"}}`
**And** 单元测试：首次执行成功缓存结果 / 重复执行返回缓存结果 / 不同 eventId 独立 / 未带 id 拒绝
**And** 集成测试（Testcontainers + 真实 Redis）：模拟客户端以相同 eventId 连发 3 次 `debug.echo` 的 dedup 版本（test-only），验证只 1 次业务执行，3 次响应一致

### Story 0.11: WS 建连频率限流 + 异常设备拒连

As a platform engineer,
I want per-user WS connect rate limit 60s ≤ 5 and device blacklist blocking upgrade,
So that the J4 3 AM WS hub explosion (single user client bug reconnecting 100 times/sec) doesn't repeat (FR41, FR45, NFR-SCALE-5).

**Acceptance Criteria:**

**Given** Story 0.9 的 WS Upgrade handler + Story 0.3 的 Redis + Story 0.6 的 AppError
**When** 扩展 `upgrade_handler.go` 加入限流与黑名单检查
**Then** upgrade 前先检查 `blacklist:device:{userId}` 存在则返回 403 + `AppError{Code: "DEVICE_BLACKLISTED", Category: fatal}`
**And** 通过黑名单后检查 `ratelimit:ws:{userId}` 计数器：`INCR key; EXPIRE key 60`（pipeline 原子）；计数 > 5 返回 429 + `AppError{Code: "RATE_LIMIT_EXCEEDED", Category: retry_after}` 含 `Retry-After` header
**And** 黑名单机制：支持 TTL；运维通过 `tools/blacklist_user/main.go` 一次性脚本添加/移除；blacklist 可配置自动 TTL（默认 24h）
**And** 限流计数器 + blacklist 检查 + 正常 upgrade 流程原子（Redis pipeline + 单次 WS upgrade 分步执行无副作用）
**And** 单元测试：6 次连续连接第 6 次被拒 / blacklist 立即生效 / blacklist TTL 过期自动解除 / 不同 userId 独立计数
**And** 集成测试（Testcontainers）：模拟客户端 1 分钟内建连 6 次，第 6 次收到 429 + 正确 error code
**And** zerolog 审计日志：每次拒连记录 `userId / reason (blacklist | ratelimit) / action=rejected`（NFR-SEC-10）

### Story 0.12: session.resume 缓存节流

As a platform engineer,
I want same-user repeat `session.resume` within 60s to return cached result,
So that the J4 session.resume storm can't exhaust Mongo / Redis connection pools (FR42, NFR-PERF-3, NFR-PERF-6).

**Acceptance Criteria:**

**Given** Story 0.9 的 WS + Story 0.10 的去重 + Story 0.11 的限流
**When** 实现 `internal/ws/session_resume.go` 作为 dispatcher 注册的 handler
**Then** 首次 resume：查 Mongo + Redis 组装 `{user, friends, catState, skins, blindboxes, roomSnapshot?}` → 响应客户端 → 结果缓存到 `resume_cache:{userId}` Redis Hash，TTL 60 秒
**And** 60 秒内后续 resume（无论同一 connId 还是重连产生的新 connId）：直接读缓存返回，不触发 Mongo query
**And** 缓存失效场景：权威写入（state.tick / friend.accept / blindbox.redeem / skin.equip / profile.update 等）成功后，对应 Service 层显式调用 `InvalidateResumeCache(ctx, userId)` `DEL resume_cache:{userId}`
**And** 响应时间 SLA：首次 p95 ≤ 500ms（NFR-PERF-3）、缓存命中 p95 ≤ 20ms（集成测试断言）
**And** `session.resume` 本身不走 eventId dedup（Story 0.10）—— 它是读操作
**And** 单元测试：首次查询走 Mongo mock / 缓存命中不走 Mongo / 显式失效后重查走 Mongo
**And** 集成测试 benchmark：连续 10 次 resume 中仅第 1 次触发 Mongo query（通过 `mongotest` 或中间件计数验证）
**And** 提供 `service.ResumeCacheInvalidator` 接口供后续 Service story 消费（接口消费方定义，定义在本 story 的 ws 包）

### Story 0.13: APNs 推送平台（Pusher 接口 + 队列 + 路由 + 410 清理）

As a platform engineer,
I want a unified Pusher interface with Redis Streams queuing, platform routing, retry, DLQ, and 410 token cleanup,
So that touch fallback (FR27) / blindbox notifications / cold-start recall (FR44b) / quiet-hours silent push (FR30) all use one reliable push platform (D3, D16, FR43, FR58, NFR-REL-8, NFR-COMP-3).

**Acceptance Criteria:**

**Given** Story 0.3 的 Redis + Story 0.8 的 cron + Story 0.5 的 logx + Story 0.7 的 Clock
**When** 实现 `internal/push/{pusher.go, apns_client.go, apns_router.go, apns_worker.go}` + `pkg/redisx/stream.go` + `internal/cron/apns_token_cleanup_job.go`
**Then** `Pusher` 接口暴露 `Enqueue(ctx context.Context, userID ids.UserID, p PushPayload) error`
**And** `PushPayload` 含 `Kind (alert|silent) / Title string / Body string / DeepLink string / RespectsQuietHours bool / IdempotencyKey string`
**And** `Enqueue` 实现 `XADD apns:queue * userId <id> kind <alert|silent> payload <json>` 并立即返回；若 `IdempotencyKey` 非空则 `SET apns:idem:{key} "1" NX EX 300` 去重（NFR-SEC-9）
**And** `APNsWorker` 实现 Runnable；`Start` 以 `XREADGROUP GROUP apns_workers <instanceID> COUNT 10 BLOCK 1000` 持续消费；消费组 `apns_workers` 在 Runnable Start 时 `XGROUP CREATE MKSTREAM`
**And** 消费流程：读 `apns_tokens` collection 获取 userId 对应 token 列表（按 platform 分组） → 按 FR58 分发（Watch token 发 Watch topic bundle、iPhone token 发 iPhone topic bundle） → `sideshow/apns2` HTTP/2 调用 → 成功 `XACK`
**And** 失败重试指数退避 3 次（基准 1s / 3s / 9s）；终态失败 `XADD apns:dlq * ...` + zerolog error 含 `userId / deviceToken (masked) / reason`（M14）
**And** HTTP 410 响应：删除对应 `apns_tokens` 文档（FR43） + `XACK`（不重试）
**And** 免打扰降级：若 `RespectsQuietHours == true && 接收方当前时区 ∈ quietWindow`（D7 读取 `user.preferences.quietHours`） → 把 `Kind` 强制改为 `silent`，`apns-push-type` 设为 `background`（NFR-COMP-3）
**And** `apns_token_cleanup_job` cron（每 24h，Story 0.8 `withLock`）：扫描 `apns_tokens` 中 `updatedAt > 30 天` 的过期 token 删除
**And** 证书存放（G3 修复）：`.p8` 私钥路径从环境变量读取（挂载 Docker volume）；keyId / teamId / topic 从环境变量；`config/` 不含密钥
**And** 单元测试：Pusher.Enqueue 入队成功 / IdempotencyKey 去重 / worker 消费单条 / 410 清理 / 免打扰降级正确改 Kind / 重试退避时间准确（用 FakeClock）
**And** 集成测试（Testcontainers + mock APNs HTTP/2 server）：端到端送达 + 重试 + DLQ
**And** `docs/code-examples/` 提供 `pusher_usage_example.go` 展示业务代码如何调 `pusher.Enqueue`

### Story 0.14: WS 消息类型注册表与版本查询

As a client developer,
I want to query the server's currently supported WS message type list with versions,
So that Watch / iPhone clients can validate compatibility on upgrade and avoid sending unknown types (FR59).

**Acceptance Criteria:**

**Given** Story 0.9 WS Hub + Dispatcher
**When** 实现 `internal/dto/ws_messages.go` + `internal/handler/platform_handler.go`
**Then** `dto/ws_messages.go` 定义所有 WS message type 常量（后续 stories 追加）：`MsgTypeBlindboxRedeem = "blindbox.redeem"` 等；每个常量关联 `version`（MVP 统一 `v1`）和 `direction`（`up / down / bi`）
**And** 单元测试 `dto/ws_messages_test.go` 扫描 dispatcher 实际注册的 handler 列表与 `dto/ws_messages.go` 常量集对比，不一致则 fail（G2 gap 修复）
**And** `docs/api/ws-message-registry.md` 人类可读注册表；CI 单元测试校验与代码常量一致
**And** 提供 HTTP `GET /v1/platform/ws-registry` 返回 `{messages: [{type, version, direction, requiresAuth, requiresDedup}, ...], apiVersion: "v1", serverTime: "<RFC3339>"}`（serverTime 走 Clock）
**And** Response 可不鉴权（平台 metadata，不含敏感）
**And** 版本字段 MVP 统一 `v1`；首次破坏性变更时升 `v2` 并保留旧版本 30 天过渡（PRD §Versioning Strategy）
**And** CI 追加 `swagger validate docs/api/openapi.yaml`（G2 gap 修复）；OpenAPI YAML 占位文件本 story 创建，后续 story 填充具体 endpoint
**And** 单元测试：endpoint 响应格式 / 常量与 dispatcher 一致 / Clock 注入的 serverTime

### Story 0.15: Spike-OP1 — watchOS WS-primary 稳定性真机测试矩阵

As an architect,
I want to quantify WS reconnect p50/p95/p99 latency and battery consumption on real Apple Watch + weak-network matrix using the full WS stack from 0.9-0.14,
So that OP-1 design can converge on one approach (from candidates: cache-first + diff, PushOnConnect, raise-wrist reconnect, WS compression, NWConnection) based on data and unblock Epic 4 (OP-1, ADR-003, D9).

**Acceptance Criteria:**

**Given** Story 0.9-0.14 已交付完整 WS 栈（upgrade / envelope / dispatcher / broadcaster / 限流 / 去重 / resume cache / 推送 / 消息注册表）
**When** 建立真机 + 弱网测试矩阵并执行
**Then** 测试矩阵覆盖：`{Wi-Fi / 4G / 弱网（丢包率 10%） / 网络切换（BT↔LTE）} × {冷启动建连 / 抬腕循环重连 / 长连持续心跳}` 共 12 个场景
**And** 测试设备：至少 1 台真实 Apple Watch（非模拟器）+ 1 台 iPhone；每场景测试时长 ≥ 30 分钟
**And** 量化指标：WS 冷建连延迟 p50/p95/p99；抬腕重连延迟 p50/p95/p99；5 秒内重连成功率（NFR-REL-4 目标 ≥ 98%）；30 分钟电量消耗 %（对比 HTTP 轮询 + APNs 基线）
**And** Hub 上限压测（ADR-003）：本机或等效环境并发 WS 连接 `{1k, 3k, 5k, 10k}`，记录 CPU% / MEM（RSS）/ 广播延迟 p95/p99；若 10k 下 p99 > 3s 则触发降低 `cfg.WS.MaxConnections` + 在报告中启动 Phase 3 sticky routing 规划
**And** 评估候选设计方向对 Hub 接口约束：验证 D6 预留的 `PushOnConnect` + `BroadcastDiff` 可满足至少 2 个候选方向
**And** 选型工具（G5 gap 修复）：评估 `k6 / vegeta / 自写 Go 客户端`，选一款作为标准压测工具并归档到报告
**And** 交付 `docs/spikes/op1-ws-stability.md` 报告含：测试矩阵结果表 + 数据图表 + 候选方向可行性分析 + **单一收敛的设计方案 + 理由 + 对 Hub/Service 接口的预期改动**
**And** 方案**不得引入 HTTP fallback backup**（用户明确反对 backup 掩盖根因）
**And** 方案被架构师（开发者）签字认可后，解锁 Epic 4 —— 本 story 是 Epic 4 的硬前置依赖
**And** 若 Spike 结论为"当前 WS-primary 不可行，需重新设计协议"，则 Epic 4 暂停，开发者评估后续路径（不在本 epic 范围）

---

## Epic 1: 身份与账户

用户可以通过 Sign in with Apple 登录、在 Watch + iPhone 各自独立保持会话、使用 JWT（含 refresh）维持 WS 鉴权、注册 APNs device token；用户可管理 displayName / timezone / 免打扰时段、上报设备时区变更、发起账号注销。

### Story 1.1: User 领域 + Sign in with Apple 登录 + JWT 签发

As a new or returning user,
I want to sign in with my Apple ID and receive access / refresh JWTs tagged to my specific device,
So that I can start using the app on Watch or iPhone without creating a password, with independent sessions per device.

**Acceptance Criteria:**

**Given** 客户端完成 Sign in with Apple 本地流程获得 `identityToken` 与可选 `authorizationCode`，客户端生成并持久化设备唯一 `deviceId`（Keychain 存储，client-generated UUID）
**When** POST `/auth/apple` body `{identityToken: string, authorizationCode?: string, deviceId: string, platform: "watch" | "iphone"}`
**Then** 服务端从 `appleid.apple.com/auth/keys` 拉取 JWK（Redis 缓存 `apple_jwk:cache` TTL 24h，NFR-INT-1）
**And** 验证 identityToken 签名 + claims（`aud` == bundle_id / `iss` == `https://appleid.apple.com` / `exp` 未过期 / `nonce` 匹配）；失败返回 401 + `AUTH_INVALID_IDENTITY_TOKEN`（Category: fatal）
**And** 提取 `sub`（Apple userId）后做 SHA-256 哈希得到 `appleUserIdHash`（NFR-SEC-6 不可逆，不存原始）
**And** 首次登录：在 `users` collection 创建文档 `{_id: UserID (uuid), appleUserIdHash, displayName: nil, timezone: nil, preferences: {quietHours: {start: "23:00", end: "07:00"}}, friendCount: 0, createdAt: <Clock.Now>, consents: {stepData: nil}, deletionRequested: false}`
**And** 老用户（`appleUserIdHash` 命中）：查找并返回，若 `deletionRequested == true` 则清空标记允许重新登录（MVP 简化）
**And** 签发 access token（JWT RS256，TTL ≤ 15min，NFR-SEC-3，claim 含 `userId / deviceId / platform / jti / exp / iat`）
**And** 签发 refresh token（JWT RS256，TTL ≤ 30 天，NFR-SEC-4，claim 含 `userId / deviceId / jti / exp / iat`）
**And** 响应 body `{accessToken, refreshToken, user: {id, displayName?, timezone?}}`，响应时间 p95 ≤ 200ms（NFR-PERF-2）
**And** `users` repository 实现 `EnsureIndexes(ctx)` 在 `appleUserIdHash` 建 unique index，由 `initialize()` 启动期调用一次
**And** zerolog 审计日志：`{action: "sign_in_with_apple", userId, deviceId, platform, isNewUser}`（NFR-SEC-10）；不含原始 identityToken / sub
**And** 单元测试：JWK 缓存命中 / identityToken 签名错误 / aud 不匹配 / exp 过期 / 新用户创建 / 老用户查找 / deletionRequested 复活
**And** 集成测试（Testcontainers）：端到端登录 + JWT 可被 `jwtx.Verify` 通过 + users 文档正确落库
**And** `deviceId` 由客户端生成 UUID 服务端直接信任（Story 0.11 WS 建连限流 + FR45 blacklist 为防御纵深）

### Story 1.2: Refresh token 刷新 + 吊销 + per-device session 隔离

As a signed-in user on multiple devices,
I want each device's access token to refresh independently and be revocable without affecting my other devices,
So that I can stay logged in on both Watch and iPhone, and logout-one-device doesn't kick me out everywhere.

**Acceptance Criteria:**

**Given** Story 1.1 已签发 `accessToken + refreshToken`
**When** access token 过期后，客户端 POST `/auth/refresh` body `{refreshToken}`
**Then** 服务端用 `jwtx.Verify` 验证 refresh token 签名 + claims（`exp / jti / userId / deviceId`）
**And** 检查 refresh token `jti` 不在 `refresh_blacklist:{jti}` Redis SET 黑名单；否则返回 401 + `AUTH_REFRESH_TOKEN_REVOKED`（Category: fatal）
**And** 验证通过：签发**新的** access token（新 jti）+ **新的** refresh token（新 jti，TTL 30 天，rolling rotation）
**And** 旧 refresh token `jti` 加入 `refresh_blacklist:{jti}`，TTL = 旧 token 剩余有效期（最长 30 天）（NFR-SEC-4 rolling rotation 安全增强）
**And** Per-device 隔离：Watch 的 refresh 操作只影响 Watch 的 session；同一 userId 可同时持有多个 deviceId 对应的 refresh token（FR5）
**And** 提供 `service.AuthService.RevokeRefreshToken(ctx, userID, deviceID)` 辅助方法把指定 (userID, deviceID) 对应的当前有效 jti 加入黑名单（供 Story 1.6 账户注销调用）
**And** 提供 `service.AuthService.RevokeAllUserTokens(ctx, userID)` 把该 userId 所有 device 的 refresh jti 加入黑名单（供 1.6 账户注销调用）
**And** Refresh token jti 本身需要知道"当前有效 jti per (userId, deviceId)"才能吊销 —— 在 `users.sessions[deviceId] = {currentJti, issuedAt}` 文档字段记录（单用户≤ 2-3 个 device 的数据量可接受）
**And** 响应时间 p95 ≤ 200ms（NFR-PERF-2）
**And** zerolog 审计：`{action: "refresh_token / revoke_token / revoke_all", userId, deviceId, oldJti, newJti?}`（NFR-SEC-10）
**And** 单元测试：正常刷新 / 黑名单拒绝 / 过期拒绝 / per-device 隔离（设备 A 刷新不吊销设备 B 的 jti）/ RevokeAll 后所有 device 均被吊销
**And** 集成测试（Testcontainers）：端到端 refresh + 黑名单 Redis 验证

### Story 1.3: JWT 鉴权中间件 + userId context 注入

As a developer of authenticated endpoints,
I want a single JWT middleware that verifies the Bearer token and injects userId + deviceId into context for both HTTP and WS paths,
So that all downstream handlers can trust `middleware.UserIDFrom(c)` without re-verifying, matching constitution §13 layering.

**Acceptance Criteria:**

**Given** Story 1.1 的 `pkg/jwtx` 能力
**When** 实现 `internal/middleware/jwt_auth.go` 与增强 `internal/ws/upgrade_handler.go` 的 Client 构造
**Then** `middleware.JWTAuth(mgr *jwtx.Manager) gin.HandlerFunc` 从 `Authorization: Bearer <token>` 读取并 `jwtx.Verify`
**And** Token 缺失 → 401 + `AUTH_TOKEN_EXPIRED`（Category: fatal）；签名/claims 错 → 401 + `AUTH_INVALID_IDENTITY_TOKEN`
**And** 验证通过：注入 gin context key `middleware.CtxUserID` 和 `middleware.CtxDeviceID`；提供辅助函数 `UserIDFrom(c *gin.Context) ids.UserID` 和 `DeviceIDFrom(c *gin.Context) string`
**And** 中间件挂载策略：`/v1/*` 路由组强制鉴权；bootstrap endpoints（`/auth/apple, /auth/refresh, /healthz, /readyz, /v1/platform/ws-registry`）不挂（宪法 §13）
**And** WS upgrade 路径（Story 0.9 基础 JWT check）升级：`jwtx.Verify` 成功后构造 `ws.Client{userID, deviceID, platform, connID}`，存入 Hub 供 Broadcaster / rate limiter 查询
**And** WS 鉴权错误返回 WS 升级失败状态码 401（不进入后续 message loop）
**And** access token 在 WS 连接存续期间过期**不中断连接**（PRD §Endpoint Specifications：WS 不做 token 刷新，服务端内部判断允许延期）—— 实现为：连接建立时记录初次验证成功，后续 message 不重复校验 exp；token 被显式吊销（Story 1.2 RevokeAll）时 Hub 显式关闭该连接
**And** 提供 `ws.Hub.DisconnectUser(userID) error` 辅助方法供账户注销（Story 1.6）消费
**And** 单元测试：token 缺失 / 签名错 / exp 过期 / 有效 token + context 注入成功 / WS 升级鉴权
**And** 集成测试：真实 JWT + HTTP endpoint + WS upgrade 端到端

### Story 1.4: APNs device token 注册 endpoint

As a signed-in user,
I want my Watch or iPhone APNs device token to be registered against my account,
So that the server can route pushes (touch fallback / blindbox drop / cold-start recall) to the right device using Epic 0's Pusher platform (FR4, NFR-INT-2).

**Acceptance Criteria:**

**Given** Story 1.3 的 JWT 中间件 + Story 0.13 的 Pusher 基础设施
**When** POST `/v1/devices/apns-token` body `{deviceToken: string, platform: "watch" | "iphone"}`
**Then** validator 校验 deviceToken 非空 + 长度符合 APNs 规范（hex 字符串，典型 64 字符）；platform 枚举
**And** 写入 `apns_tokens` collection upsert by `(userId, platform)`：`{_id, userId, platform, deviceToken: <encrypted>, updatedAt: Clock.Now}`（每个 platform 仅保留最新 token，避免同 Platform 冗余）
**And** `apns_tokens` repository `EnsureIndexes(ctx)` 建 `(userId, platform)` unique compound index，由 `initialize()` 启动期调用
**And** deviceToken 在 Mongo 中加密存储（NFR-SEC-7）—— MVP 使用 `pkg/cryptox` 包装的 AES-GCM，key 来自 config / 环境变量；读出时在 repository 内解密，Service / Handler 看到明文
**And** zerolog 日志对 deviceToken 做 `logx.MaskAPNsToken`（M14 前 8 字符 + `...`，仅 DEBUG level 暴露）
**And** Per-userId 限流：60 秒 ≤ 5 次注册（NFR-SCALE 余量；APNs token 变化应很低频）
**And** 响应 `{ok: true}`，时间 p95 ≤ 200ms（NFR-PERF-2）
**And** `users.sessions[deviceId]` 可额外冗余 `hasApnsToken: bool` 便于查询（可选）
**And** 单元测试：正常注册 / 同 platform 覆盖 / 加密读出一致 / 限流拒绝 / validator 拒绝非法 deviceToken
**And** 集成测试（Testcontainers）：端到端注册 + Pusher（Epic 0）能查到 token

### Story 1.5: Profile 与偏好设置（displayName / timezone / quietHours）

As a signed-in user,
I want to set my display name and local timezone + quiet-hours window, and have my client auto-report timezone changes,
So that friends see my cat labeled correctly and nighttime touches are silenced per my local time (FR48, FR49, FR50).

**Acceptance Criteria:**

**Given** Story 1.3 的 JWT 中间件 + Story 0.12 的 ResumeCacheInvalidator + Story 0.10 的 eventId dedup
**When** 客户端通过 WS RPC `profile.update` 发送 envelope `{id, type: "profile.update", payload: {displayName?: string, timezone?: string, quietHours?: {start: string, end: string}}}`
**Then** dispatcher 路由到 `ws/handlers/profile_handlers.go`，走 eventId dedup（Story 0.10）
**And** validator 校验：
  - `displayName` 若提供：长度 1-32 字符，去除前后空白后非空，不含 ASCII 控制字符；否则 `VALIDATION_ERROR`
  - `timezone` 若提供：有效 IANA 时区字符串（如 `Asia/Shanghai, America/New_York`），用 `time.LoadLocation` 验证；否则 `VALIDATION_ERROR`
  - `quietHours` 若提供：`start` 和 `end` 均符合 `HH:MM` 24 小时格式；否则 `VALIDATION_ERROR`
**And** Payload 至少 1 个字段提供；全空返回 `VALIDATION_ERROR`
**And** 写入 `users` collection 对应字段（按提供的字段 partial update）；`updatedAt = Clock.Now`
**And** 响应 `{user: {id, displayName, timezone, preferences: {quietHours}}}`；WS envelope `ok: true`
**And** 成功后显式调用 `ws.InvalidateResumeCache(ctx, userID)` 失效 Story 0.12 的缓存
**And** `displayName` 变更时 zerolog 日志只记 `userId / action="profile_update" / fields=["displayName"]`，**不记 displayName 本身**（M13 PII）
**And** FR50 实现：客户端发现 `TimeZone.current` 变化时自动触发本 RPC（客户端实现，服务端复用本 endpoint）
**And** 默认值：用户未设置 `timezone` 时，Pusher（Epic 0）默认 UTC；`quietHours` 默认 `{start: "23:00", end: "07:00"}`（Story 1.1 初始化）
**And** 单元测试：各字段校验 / partial update / 跨字段组合 / resume 缓存被失效 / PII mask 正确
**And** 集成测试：WS 端到端 + users collection 字段正确落库

### Story 1.6: 账户注销请求

As a signed-in user,
I want to request account deletion and have my sessions terminated immediately,
So that I can stop using the app while the server handles cascade cleanup per MVP compliance (FR47, NFR-COMP-5 MVP：30 天内人工处理).

**Acceptance Criteria:**

**Given** Story 1.3 的 JWT 中间件 + Story 1.2 的 RevokeAllUserTokens + Story 1.3 的 `ws.Hub.DisconnectUser`
**When** HTTP DELETE `/v1/users/me`（或 POST `/v1/users/me/deletion-request`；二选一在实现时定稿并更新 OpenAPI）
**Then** 在 `users` 文档设置 `deletionRequested = true, deletionRequestedAt = Clock.Now`；**不做级联清理**（MVP，NFR-COMP-5 Growth 再做）
**And** 调用 `AuthService.RevokeAllUserTokens(ctx, userID)` 吊销该用户所有 device 的 refresh token（Story 1.2）
**And** 调用 `ws.Hub.DisconnectUser(userID)` 立即关闭该用户所有 WS 连接（Story 1.3）
**And** 失效 `resume_cache:{userId}`
**And** 响应 `{status: "deletion_requested", requestedAt, note: "30 days manual cleanup per MVP policy"}`，HTTP 202 Accepted
**And** zerolog 审计日志：`{action: "account_deletion_request", userId, deletedAt}`（NFR-SEC-10）
**And** 后续 Sign in with Apple 登录（Story 1.1）若发现 `deletionRequested == true` 则清空该标记允许复活（MVP 简化；Growth 阶段需确认是否允许）
**And** 提供 `tools/process_deletion_queue/main.go` 一次性脚本（本 story 的 tool，30 天后运维手动执行）：扫描 `deletionRequested == true && deletionRequestedAt > 30 天前` 的用户，级联清理（好友 / 状态快照 / 盲盒 / 皮肤 / APNs token），此脚本不纳入自动化 cron
**And** 单元测试：注销后 users 标记正确 / refresh token 全被吊销 / WS 连接被断开 / resume cache 被失效 / 登录复活场景
**And** 集成测试（Testcontainers）：端到端注销 + 重新登录 + 状态正确

---

## Epic 2: 你的猫活着（状态映射 + 步数）

用户的猫映射本人真实运动（idle / walking / running / sleeping），步数（dailySteps + stepDelta）随状态上报被安全存储；陈旧状态在服务端按四档衰减规则自动降级至 idle 或标记离线；异常步数增量（单次 > 10k）被标记人工审查；iPhone 后台 HealthKit 通过 HTTP `POST /state`（30s 特例窗口）单次上报。

### Story 2.1: CatState 领域 + Source 优先级 + FSM 状态机

As a backend developer,
I want a pure domain layer defining cat states, state sources with priority, and snapshot override rules,
So that all upload / decay / conflict logic in this epic and Epic 4 builds on a consistent, testable foundation (D2, M6, M7).

**Acceptance Criteria:**

**Given** Epic 0 交付的 `pkg/ids` + `pkg/fsm` + `pkg/clockx`
**When** 实现 `internal/domain/{cat_state.go, source.go, enums.go}`
**Then** 定义 `type CatState string` 枚举 + typed constants：`CatStateIdle = "idle"` / `CatStateWalking = "walking"` / `CatStateRunning = "running"` / `CatStateSleeping = "sleeping"`（M6 业务 string 小写，Go typed const `<Domain><Value>` 模式）
**And** 定义 `type StateSource string` 枚举 + typed constants：`SourceWatchForeground = "watch_foreground"` / `SourceIphoneForeground = "iphone_foreground"` / `SourceIphoneBackgroundHealthkit = "iphone_background_healthkit"` / `SourceServerInference = "server_inference"`
**And** 提供函数 `SourcePriority(s StateSource) int` 返回 int（高值高优先级），按 D2 顺序：`watch_foreground=40 / iphone_foreground=30 / iphone_background_healthkit=20 / server_inference=10`
**And** 定义 `CatStateSnapshot` 值对象 `{UserID ids.UserID, State CatState, Source StateSource, DailySteps int64, StepDelta int64, UpdatedAt time.Time, Points int64, SkinID *ids.SkinID}`
**And** 实现 `CatStateSnapshot.ShouldOverride(existing *CatStateSnapshot) bool`（D2 per-source priority + 同 source 内 LWW）：
  - `existing == nil` → 必然覆盖（返回 true）
  - `SourcePriority(new.Source) > SourcePriority(existing.Source)` → 覆盖
  - `SourcePriority(new.Source) == SourcePriority(existing.Source) && new.UpdatedAt.After(existing.UpdatedAt)` → 覆盖（LWW）
  - 否则不覆盖
**And** 提供 `ValidateState(s CatState) error` 校验枚举合法性；非法返回 sentinel error `ErrInvalidCatState`
**And** FSM 模块（`pkg/fsm`）不强制 state 间转换规则（PRD/Architecture 未定义禁止转换）；仅提供 state 合法性校验 + transition logging hook
**And** `MapPlatformSourceToStateSource(platform string, isForeground bool) StateSource` 辅助函数：`(watch, true) → SourceWatchForeground / (iphone, true) → SourceIphoneForeground / (iphone, false) → SourceIphoneBackgroundHealthkit`；其他组合返回 error
**And** 单元测试 table-driven：ShouldOverride 全矩阵（每对 source 组合，含同 source LWW 时间前后）/ ValidateState 合法与非法 / MapPlatformSourceToStateSource 所有 branch

### Story 2.2: WS state.tick 上报 + 持久化 + 源优先级冲突解决

As a signed-in user on Apple Watch or iPhone foreground,
I want my cat state and step counts uploaded via WebSocket persisted durably with source-priority conflict resolution,
So that my cat's state reflects the most trustworthy source (Watch foreground beats iPhone background) even when multiple devices report simultaneously (FR7, FR8, D2, D10, NFR-REL-5).

**Acceptance Criteria:**

**Given** Story 2.1 的 domain + Epic 0 的 WS dispatcher / Broadcaster / Clock / logx / eventId dedup + Epic 1 的 WS Client（userID/deviceID/platform 已解析）
**When** 实现 `internal/service/state_service.go` + `internal/repository/cat_state_repo.go` + `internal/ws/handlers/state_handlers.go` + `internal/dto/state_dto.go`
**Then** 客户端通过 WS envelope `{id, type: "state.tick", payload: {catState: "walking", dailySteps: 3245, stepDelta: 120, clientUpdatedAt?: "<RFC3339>"}}`
**And** dispatcher 路由到 `state_handlers.HandleStateTick`；走 Story 0.10 eventId dedup（避免重放）
**And** validator 校验：`catState` 调用 `domain.ValidateState`；`dailySteps >= 0`；`stepDelta >= 0`；`clientUpdatedAt` 若提供须为 RFC3339 解析
**And** handler 从 WS Client context 提取 `userID / deviceID / platform`
**And** handler 组装 `CatStateSnapshot{UserID, State: catState, Source: MapPlatformSourceToStateSource(platform, isForeground=true), DailySteps, StepDelta, UpdatedAt: Clock.Now()}` 调用 `StateService.UpdateCatState(ctx, snapshot)`
**And** StateService 流程：
  1. `existing, err := repo.Get(ctx, userID)` —— 读 Redis 热缓存 miss 则读 Mongo
  2. 若 `!snapshot.ShouldOverride(existing)` → 静默跳过（返回 success，不更新 DB，不广播）
  3. 若覆盖：`repo.Upsert(ctx, snapshot)` —— Mongo upsert `cat_states._id == userID` + Redis Hash `state:{userId}` write-through（单次 Mongo update + Redis HSET 原子，无事务，NFR-REL-5）
  4. 调用 `stateBroadcaster.OnStateChange(ctx, userID, snapshot)` —— `StateBroadcaster` 接口在 `state_service.go` 定义（消费方定义）；Epic 2 提供 `NoOpStateBroadcaster{}`（zerolog debug 日志 only）；Epic 4 将替换为 `RoomStateBroadcaster`
  5. 调用 `resumeCacheInvalidator.InvalidateResumeCache(ctx, userID)`（Story 0.12）
**And** `cat_state_repo` 内部 `catStateDoc` 私有结构与 `*domain.CatStateSnapshot` 互转（M7 repo 内部 BSON 转换）
**And** `cat_state_repo.EnsureIndexes(ctx)` 在 `updated_at` 建 index（Story 2.5 衰减扫描用）；由 `initialize()` 启动期调用
**And** Redis `state:{userId}` Hash 字段：`catState, dailySteps, stepDelta, source, updatedAt, points, skinId`（write-through）
**And** Handler 响应 WS envelope `{id, ok: true, type: "state.tick.result", payload: {accepted: true|false}}`（accepted false 表示源优先级低被跳过）
**And** 响应时间 p95 ≤ 200ms（NFR-PERF-4 WS RPC 往返）
**And** dailySteps 语义：客户端上报当日累计值（monotonic），服务端直接写入；如客户端上报比服务端 existing 低（意外），仅当 ShouldOverride == true 时写入，否则保留 existing（简化，避免 max 合并逻辑）
**And** 单元测试：不同 source 优先级组合 / 同 source LWW / 低优先级跳过 / 高优先级覆盖 / Redis write-through / StateBroadcaster.OnStateChange 被调用 / resume cache invalidate 被调用 / validator 各边界
**And** 集成测试（Testcontainers）：端到端 WS `state.tick` → Mongo + Redis 双写 → NoOpStateBroadcaster 日志输出

### Story 2.3: HTTP POST /state（HealthKit 后台 30s 窗口）

As an iPhone user whose HealthKit observer woke the app in background for 30 seconds,
I want my cat state uploaded via single-shot HTTP (not WS) since I have no time to establish a WebSocket handshake,
So that my state stays reasonably fresh even when the app is suspended (FR12, PRD §Apple 平台特有约束).

**Acceptance Criteria:**

**Given** Story 2.2 的 `StateService.UpdateCatState` + Epic 1 的 JWT 中间件 + Epic 0 的 `middleware.Recover / Logger / RequestID`
**When** 实现 `internal/handler/state_handler.go` 挂在 `/v1/state`
**Then** HTTP POST `/v1/state` body `{catState: "walking", dailySteps: 3245, stepDelta: 120}`
**And** JWT 中间件（Story 1.3）注入 userID / deviceID / platform（platform 应为 "iphone"，其他 platform 返回 `VALIDATION_ERROR`）
**And** handler 强制 `Source = SourceIphoneBackgroundHealthkit`（不信任客户端传入 source 字段）
**And** 调用同一 `StateService.UpdateCatState(ctx, snapshot)`；由于 source 优先级最低，被 foreground 覆盖是预期行为（不 error）
**And** validator 复用 WS 路径的 DTO 校验（M8 DTO 转换在 handler 层）
**And** 响应 HTTP 200 + `{ok: true, accepted: true|false}`；响应时间 p95 ≤ 200ms（NFR-PERF-2）
**And** 单次请求必须在 HealthKit 30s 窗口内完成，因此 endpoint 内部不应做任何阻塞 I/O 超过 2s（超时返回 503 + `INTERNAL_ERROR`，客户端下次 observer 唤醒再试）
**And** 限流：per-userId 60s ≤ 10 次（HealthKit observer 唤醒频率上限，防止客户端 bug 狂打）；超限返回 429 + `RATE_LIMIT_EXCEEDED`
**And** zerolog 审计：`{action: "state_http", userId, source: "iphone_background_healthkit", latencyMs}`（NFR-OBS-3）
**And** 单元测试：正常上报 / platform != iphone 拒绝 / 限流拒绝 / 超时行为
**And** 集成测试（Testcontainers + httptest）：端到端 HTTP → Mongo/Redis 更新

### Story 2.4: 异常步数增量打标审查

As a compliance engineer,
I want single-shot step deltas over 10,000 steps to be flagged for manual review without blocking the upload,
So that blindbox cheating via HealthKit tampering is auditable, while false positives don't break legitimate users (FR11, PRD §Risk Mitigation).

**Acceptance Criteria:**

**Given** Story 2.2 已落 `stepDelta` 字段写入路径
**When** 扩展 `StateService.UpdateCatState` 加入异常检测 + 实现 `internal/repository/step_anomaly_repo.go`
**Then** 当 `snapshot.StepDelta > 10_000`（阈值从 `cfg.Business.StepAnomalyThreshold` 读取，默认 10_000；D13 配置分层）时触发打标
**And** 打标写入 `step_anomalies` collection 文档 `{_id, userId, stepDelta, totalStepsBefore, totalStepsAfter, detectedAt: Clock.Now(), source, deviceId, status: "pending_review"}`
**And** `step_anomalies` repository `EnsureIndexes(ctx)` 在 `(userId, detectedAt)` 建 index + `status` 建 index；由 `initialize()` 启动期调用
**And** 打标**不阻止**当前 `UpdateCatState` 写入 cat_states（PRD §Risk Mitigation：仅 flag 不拒绝，误杀率优先低于检测率）
**And** zerolog warn：`{action: "step_anomaly_flagged", userId, stepDelta, threshold, source}`（NFR-SEC-10 审计）
**And** 提供一次性脚本 `tools/list_step_anomalies/main.go`：扫描 `step_anomalies` `status == "pending_review"` 记录输出 TSV，运维手动审查
**And** 提供一次性脚本 `tools/mark_step_anomaly/main.go`：接收 `--id <_id> --status <reviewed_ok | confirmed_cheat>` 更新文档状态（MVP 无 API，仅 tool）
**And** 阈值可通过 Redis Hash `config:runtime.step_anomaly_threshold` 动态覆盖（D13）
**And** 单元测试 table-driven 边界：`stepDelta = 9999 / 10000 / 10001 / 20000`；每种 source 都检测；连续两次 delta 累积（例如两次 8000）不触发
**And** 集成测试（Testcontainers）：端到端 state.tick delta=15000 → cat_states 正确写入 + step_anomalies 记一条 + zerolog warn

### Story 2.5: 衰减引擎 cron + FSM idle 推断 + 四档广播

As a product owner,
I want stale cat states automatically decayed to idle / offline by a server-side cron according to the four-tier NFR-PERF-5 schedule,
So that `presence_decay_truthfulness` holds — no friend sees a stale state labeled "alive" after the user's Watch has been dark for 10 minutes (FR9, FR10, NFR-PERF-5, NFR-PERF-7, D7/ADR-001).

**Acceptance Criteria:**

**Given** Story 2.2 的 cat_state_repo + StateBroadcaster 接口 + Epic 0 的 cron + lock + Clock + logx
**When** 实现 `internal/cron/state_decay_job.go` + `StateService.Decay(ctx)`
**Then** cron `state_decay` 每 30 秒运行一次（NFR-PERF-7）；包 `redisx.Locker.WithLock("state_decay", 25 * time.Second, fn)`（D5）
**And** Decay 流程：
  1. `SCAN` Redis `state:*` keys（批次 100，避免阻塞）
  2. 对每条读 `state:{userId}` Hash 取 `updatedAt`、`catState`、`offline?`
  3. 计算 `age = Clock.Now().Sub(updatedAt)`
  4. 按 NFR-PERF-5 四档分发：
     - `age <= 15s`：真实，不操作
     - `15s < age <= 60s`：调 `stateBroadcaster.OnDecayTier(userID, "weak", currentState)` —— 后续 Epic 4 的 RoomStateBroadcaster 会广播 `friend.state` 带 `stale: "weak"` 标记
     - `60s < age <= 5min`：若 currentState != idle 则 `repo.UpdateToServerInference(userId, CatStateIdle, Clock.Now())` 且广播 `stateBroadcaster.OnDecayTier(userID, "inferred_idle", CatStateIdle)`（Epic 4 对应 `state.serverPatch` push payload `{catState: "idle", reason: "inactivity_15_min"}`）；若 already idle 则跳过
     - `age > 5min`：若未 offline 则 `repo.MarkOffline(userId)` 把 Redis Hash 添加 `offline: true` 字段（幂等避免重复广播）+ 广播 `stateBroadcaster.OnUserOffline(userID)`（Epic 4 对应 `friend.offline`）
**And** `StateBroadcaster` 接口扩展方法：`OnStateChange / OnDecayTier(userID, tier, state) / OnUserOffline(userID)`；Epic 2 的 NoOpStateBroadcaster 对三者记 debug 日志；Epic 4 实现真实房间广播
**And** FR9（长时间无上报自动推断 idle）由 60s-5min 档实现
**And** Clock 通过依赖注入到 Service 与 cron；单元测试用 FakeClock 确定性跨档（FR60）
**And** 幂等保证：offline 标记后重复扫描不再广播；`inferred_idle` 档只在 state != idle 时触发（避免把 idle 再广播一次）
**And** cron 扫描不触发 Mongo 全扫（仅 Redis SCAN）；如 Redis 与 Mongo 不一致（灾备重启场景），以 Redis 为准（NFR-REL-7 Redis 可从 Mongo 重建）
**And** zerolog：每次 tier 变化记 info `{userId, fromState, toState, tier, reason, ageSec}`；扫描开始/结束记一条 debug 含批次数量 / 耗时（NFR-OBS-5 核心指标）
**And** 单元测试 table-driven：FakeClock 跨四档边界（14s / 16s / 59s / 61s / 4:59 / 5:01 / 已 offline 再扫描）/ source priority（ServerInference 会被下次 ForegroundSource 上报覆盖）/ 并发扫描时锁保护
**And** 集成测试（Testcontainers + FakeClock）：
  - 端到端 state.tick 写入 → FakeClock.Advance(30s) → decay cron 触发 → weak 广播
  - FakeClock.Advance(2m) → inferred_idle 广播 + cat_states.source == server_inference
  - FakeClock.Advance(6m) → offline 广播 + Redis offline 标记

---

## Epic 3: 好友圈

用户可以生成 / 接受好友邀请 token（24h 过期、单次使用、Universal Link 格式）、查看好友列表、解除好友、屏蔽 / 取消屏蔽（双向不可见）；系统强制执行好友上限 20 人、邀请 token 24h ≤ 10 个的防滥用限制。

### Story 3.1: 邀请 token 生成 + Universal Link 邀请 URL

As a signed-in user who wants to invite a friend,
I want to generate a time-limited, single-use invite token wrapped in an Apple Universal Link,
So that my friend can tap the link on Watch or iPhone and be routed directly to the accept flow without any copy-paste (FR13, FR20, FR55, G1).

**Acceptance Criteria:**

**Given** Epic 0（Clock / Redis / logx / WS dispatcher / eventId dedup）+ Epic 1（JWT 中间件）
**When** 客户端发送 WS envelope `{id, type: "friend.invite", payload: {}}`
**Then** dispatcher 路由到 `internal/ws/handlers/friend_handlers.go:HandleFriendInvite`；走 Story 0.10 eventId dedup
**And** 限流：`ratelimit:invite:{userId}` Redis counter `INCR + EXPIRE 86400`；计数 > 10 → 返回 `{ok: false, error: {code: "RATE_LIMIT_EXCEEDED"}}`（Category: retry_after，NFR-SCALE-7）
**And** 生成 invite token：`crypto/rand` 32 字节 → `base64.RawURLEncoding` 字符串（43 字符）
**And** 写入 `invite_tokens` collection `{_id: <token>, creatorId: userID, expiresAt: Clock.Now + 24h, used: false, usedBy: nil, usedAt: nil, createdAt: Clock.Now}`；`_id` 唯一索引保证 token 不冲突（撞库概率 ≈ 2^-256）
**And** `invite_tokens` repository `EnsureIndexes(ctx)` 建 `expiresAt` TTL index（D11 Mongo 自动清理过期 token）+ `creatorId` index（为运维排查便利）；由 `initialize()` 启动期调用
**And** 组装 Universal Link URL：`https://{cfg.UniversalLinks.Domain}/invite?token={token}`（domain 从 `config.toml` 读取，如 `catapp.example.com`）
**And** 响应 envelope `{id, ok: true, type: "friend.invite.result", payload: {inviteToken: "<token>", universalLinkUrl: "https://...", expiresAt: "<RFC3339>"}}`；响应时间 p95 ≤ 200ms（NFR-PERF-4）
**And** `deploy/well-known/apple-app-site-association` 部署文件创建，JSON 内容含 `{"applinks": {"apps": [], "details": [{"appID": "<teamId>.<bundleId>", "paths": ["/invite*"]}]}}`；`teamId / bundleId` 来自环境变量（G3 一致化）
**And** `initialize.go` 注册 Gin 静态路由 `GET /.well-known/apple-app-site-association` 返回该文件，响应头 `Content-Type: application/json`（NFR-INT-3）
**And** zerolog 审计：`{action: "friend_invite_created", userId, tokenIdPrefix: <token前8字符>}`（token 本体不入日志，避免日志泄露邀请权限）
**And** 单元测试：token 随机性 / TTL expiresAt 计算正确 / Universal Link 格式 / 限流拒绝
**And** 集成测试（Testcontainers）：端到端生成 + TTL index 验证 + `GET /.well-known/apple-app-site-association` 返回正确 JSON

### Story 3.2: 接受好友邀请 + 上限检查（Mongo 事务）

As an invitee,
I want to accept an invite token and atomically establish a two-way friendship, subject to both sides' friend limits and block state,
So that both people's friend graphs stay consistent even under retry or concurrent accepts, enforcing the 20-friend cap and explicit-block filters (FR14, FR19, D10, NFR-REL-3-style atomicity).

**Acceptance Criteria:**

**Given** Story 3.1 + Epic 0 的 `mongox.WithTransaction` + Story 0.12 的 ResumeCacheInvalidator + Epic 1 的 JWT 中间件
**When** 客户端发送 WS envelope `{id, type: "friend.accept", payload: {inviteToken: "<token>"}}`
**Then** dispatcher 走 eventId dedup
**And** 查询 `invite_tokens._id == inviteToken`：
  - 不存在 → `{ok: false, error: {code: "FRIEND_INVITE_EXPIRED"}}`（Category: fatal）
  - `used == true` → `FRIEND_INVITE_USED`
  - `expiresAt < Clock.Now()` → `FRIEND_INVITE_EXPIRED`
**And** 解析 `creatorId`；若 `creatorId == requesterUserId` → `{error: {code: "VALIDATION_ERROR", message: "cannot self-invite"}}`（client_error）
**And** 检查屏蔽：任一方向 `blocks` 匹配 `(creatorId, requesterUserId)` 或 `(requesterUserId, creatorId)` → `FRIEND_BLOCKED`（fatal）
**And** 检查是否已是好友：`friends` 查 `(userA=min(creatorId,requesterUserId), userB=max(...))` → 存在则 `FRIEND_ALREADY_EXISTS`（client_error）
**And** 读取双方 `users.friendCount`；任一 ≥ 20 → `FRIEND_LIMIT_REACHED`（client_error，NFR-SCALE 好友上限 20）
**And** 通过所有校验后进入 `mongox.WithTransaction(ctx, cli, fn)` 事务（D10）：
  1. 插入 `friends` 文档 `{_id: ObjectId(), userA: min(creatorId, requesterUserId), userB: max(...), status: "active", createdAt: Clock.Now}`；`(userA, userB)` unique compound 保护并发重复
  2. 更新 `invite_tokens._id == inviteToken` 设 `used: true, usedAt: Clock.Now, usedBy: requesterUserId`
  3. `users.friendCount` creatorId 和 requesterUserId 各 `$inc: 1`
  4. 若任一步骤失败整体 rollback
**And** `friends` repository `EnsureIndexes(ctx)`：`(userA, userB)` unique compound + `userA` + `userB` 单独 index（好友列表正反向查询）；由 `initialize()` 启动期调用
**And** 事务成功后：调 `resumeCacheInvalidator.Invalidate(ctx, creatorId)` + `Invalidate(ctx, requesterUserId)`（Story 0.12）
**And** 响应 envelope `{ok: true, type: "friend.accept.result", payload: {friend: {id: creatorId, displayName, timezone}}}`
**And** 响应时间 p95 ≤ 200ms（NFR-PERF-4）
**And** zerolog 审计：`{action: "friend_accept", userAId, userBId, tokenIdPrefix}`（NFR-SEC-10）
**And** 单元测试：每种 error code / 自加自拒绝 / 上限 20 触发 / 重复加好友 / 屏蔽场景 / 事务中途 `invite_tokens` 更新失败的 rollback
**And** 集成测试（Testcontainers）：真实 Mongo 事务 + `friendCount` 并发 +1 一致性；race 场景（两人同时接受对方邀请）友好失败

### Story 3.3: 解除好友

As a signed-in user,
I want to remove a friend from my list and have both sides' friend counts updated atomically,
So that I can clean up my graph when relationships end, without leaving orphan records (FR15, D10).

**Acceptance Criteria:**

**Given** Story 3.2 的 friends collection + Epic 0 的 `mongox.WithTransaction`
**When** 客户端发送 WS envelope `{id, type: "friend.delete", payload: {friendId}}`
**Then** dispatcher 走 eventId dedup
**And** 规范化 userA/userB：`min(requesterUserId, friendId), max(...)`
**And** 查询 `friends` 文档 `(userA, userB)`：
  - 不存在 → 返回 `{ok: true}`（幂等，silent_drop 语义；避免客户端重试报错）
  - 存在则进入事务
**And** 进入 `mongox.WithTransaction`（D10）：
  1. 删除 `friends` 文档
  2. `users.friendCount` 两侧 `$inc: -1`；使用 `$max: {friendCount: 0}` 保护下限（防止意外负值）
**And** 事务成功后：`resumeCacheInvalidator.Invalidate` 两侧
**And** 响应 `{ok: true, type: "friend.delete.result", payload: {deleted: true|false (false表示本来就不存在)}}`
**And** 响应时间 p95 ≤ 200ms（NFR-PERF-4）
**And** zerolog 审计：`{action: "friend_delete", userAId, userBId}`（NFR-SEC-10）
**And** 注意：解除好友**不自动清理** `blocks` 记录（屏蔽独立于好友关系）
**And** 单元测试：正常删除 / 不存在的幂等 / friendCount 下限保护 / 事务 rollback
**And** 集成测试（Testcontainers）：端到端 + 并发删除相同 pair 仅有一次 friendCount 扣减

### Story 3.4: 好友列表查询（含屏蔽过滤）

As a signed-in user opening the friends list,
I want to see all my active friends with their basic profile and current cat state, with anyone I've blocked or been blocked by filtered out,
So that my UI accurately reflects my social graph and honors explicit-block双向不可见 (FR16, FR17 双向不可见 summary).

**Acceptance Criteria:**

**Given** Story 3.2 的 friends collection + Story 3.5 的 blocks collection + Epic 2 的 cat_states
**When** 客户端发送 WS envelope `{id, type: "friends.list", payload: {}}`
**Then** dispatcher **不走** eventId dedup（读操作）
**And** 查询 `friends` collection where `(userA == requesterUserId OR userB == requesterUserId) AND status == "active"`
**And** 对每条 friend 解出对方 userId（非 requester 那方）
**And** 过滤 blocks：查询 `blocks` 任一方向存在 `(blocker=requesterUserId, blocked=friendId)` 或 `(blocker=friendId, blocked=requesterUserId)`，匹配则从结果中移除（FR17 双向不可见）
**And** 对过滤后的好友列表并发获取（`errgroup` 或等价）：
  - `users` 取 `displayName, timezone`
  - `cat_states`（Redis 热缓存 miss 则 Mongo）取 `catState, skinId, updatedAt`
**And** 响应 envelope `{ok: true, type: "friends.list.result", payload: {friends: [{id, displayName, timezone, catState, skinId, updatedAt}, ...]}}`
**And** 响应时间 p95 ≤ 200ms（NFR-PERF-4）；即使 20 好友全满也要达标（并发查询实现）
**And** `displayName` 在服务端响应中返回完整值（供客户端 UI 显示），但 zerolog 日志内的 displayName 必须 mask（M13）
**And** 单元测试：空列表 / 20 人满列表 / 屏蔽过滤（单向/双向）/ 被屏蔽 + 好友两种并发场景
**And** 集成测试（Testcontainers）：20 人好友 benchmark p95 ≤ 200ms

### Story 3.5: 屏蔽 / 取消屏蔽

As a signed-in user who wants to cut off contact with a specific friend without deleting the friendship,
I want bilateral invisibility (both parties can't see each other's cat nor send touches) via a dedicated block record,
So that harassment or awkward situations are immediately and silently handled without notifying the blocked party (FR17, FR18, PRD §Domain-Specific Requirements 显式屏蔽执行).

**Acceptance Criteria:**

**Given** Story 3.2 的 friends collection + Epic 0 的 Redis / logx / eventId dedup
**When** 客户端发送 WS envelope `{id, type: "friend.block", payload: {friendId}}` 或 `{type: "friend.unblock", payload: {friendId}}`
**Then** dispatcher 走 eventId dedup
**And** `friend.block` 流程：
  1. 校验 `friendId != requesterUserId` → `VALIDATION_ERROR`
  2. 写入 `blocks` collection upsert `{_id: ObjectId(), blocker: requesterUserId, blocked: friendId, createdAt: Clock.Now}`
  3. `(blocker, blocked)` unique compound 保证幂等（重复 block 不创建重复记录）
**And** `friend.unblock` 流程：
  1. 删除 `blocks` 文档 where `blocker == requesterUserId && blocked == friendId`
  2. 不存在则返回 `{ok: true}`（幂等 silent_drop）
**And** `blocks` repository `EnsureIndexes(ctx)`：`(blocker, blocked)` unique compound + `blocker` + `blocked` 各自 index（双向查询）；由 `initialize()` 启动期调用
**And** 屏蔽**不自动删除**好友关系（好友表保留；屏蔽层在下游各 service 独立过滤）
**And** 屏蔽效果（由消费端实现，本 story 仅保证数据存在）：
  - Story 3.4 好友列表过滤
  - Epic 4 房间可见性过滤（将在 Epic 4 实现）
  - Epic 5 触碰拦截（将在 Epic 5 实现）
**And** `resumeCacheInvalidator.Invalidate` 两侧（屏蔽状态影响 resume payload 的好友可见性）
**And** 响应 `{ok: true, type: "friend.block.result" | "friend.unblock.result", payload: {}}`
**And** 响应时间 p95 ≤ 200ms
**And** zerolog 审计：`{action: "friend_block" | "friend_unblock", blocker, blocked}`（NFR-SEC-10）
**And** 屏蔽对方**不应通知被屏蔽方**（PRD §Domain-Specific）—— 响应、日志、推送均不向 `blocked` 方泄露
**And** 单元测试：正常 block / 重复 block 幂等 / 自屏蔽拒绝 / unblock / unblock 不存在 / 双向 block（A block B 且 B block A）两条独立记录
**And** 集成测试（Testcontainers）：端到端 block + Story 3.4 列表过滤验证生效

---

## Epic 4: 好友房间 / 环境在场感

用户加入好友房间（最多 4 人同屏）时立即收到房间全量快照；房间内好友的状态变化以 p99 ≤ 3s 实时广播（产品心脏）；用户可收到好友上下线事件；WS 断线后通过 `session.resume` + `lastEventId` 恢复会话；用户主动或退到后台时服务端感知离开；房间已满时拒绝新成员。**硬前置：Story 0.15 Spike-OP1 必须收敛出 OP-1 watchOS WS-primary 设计方案后才能启动本 Epic。**

### Story 4.1: Presence 生命周期（D8）+ WS 断连即离房

As a platform engineer,
I want Redis-backed presence tracking per D8/ADR-002 (Set with member format `{deviceId}:{connId}:{instanceId}`) with heartbeat-driven expiry and cron cleanup,
So that any WS disconnect (explicit close / app background / network drop / heartbeat timeout) is server-perceivable and drives downstream room/broadcast cleanup (FR51, D8, NFR-SCALE-2 multi-replica ready).

**Acceptance Criteria:**

**Given** Epic 0 Story 0.9 的 WS Hub + Story 0.15 Spike-OP1 已签字闭合 + Redis + Clock + cron with lock
**When** 实现 `internal/service/presence_service.go` + `pkg/redisx/presence.go` + `internal/cron/presence_cleanup_job.go`
**Then** Presence Key 结构（D8 / ADR-002）：`presence:{userId}` Redis Set，成员格式 `{deviceId}:{connId}:{instanceId}`
**And** `presence:{userId}:meta` Redis Hash 每成员键值 `{member: lastPingRFC3339}`
**And** WS 上线回调（Hub.onUpgradeSuccess）调 `PresenceService.Register(ctx, userID, deviceID, connID, instanceID)`：
  1. `SADD presence:{userId} <member>`
  2. `HSET presence:{userId}:meta <member> <Clock.Now RFC3339>`（pipeline 原子）
**And** WS readPump 每收到 pong / 任何 message 调 `PresenceService.Heartbeat(ctx, userID, member)` 刷新 `lastPing`
**And** WS 断连回调（Hub.onDisconnect —— 包括 close frame / readPump error / Hub.DisconnectUser 显式断开）调 `PresenceService.Deregister(ctx, userID, member)`：
  1. `SREM presence:{userId} <member>`
  2. `HDEL presence:{userId}:meta <member>`
  3. 若 `SCARD presence:{userId} == 0` 则 `DEL presence:{userId}` + `DEL presence:{userId}:meta`（避免空 Set 悬挂）
  4. 返回 `(wasLastDevice bool)`（供 Story 4.4 判断是否触发 offline 广播）
**And** `PresenceService.IsUserOnline(ctx, userID) (bool, error)`：基于 `EXISTS presence:{userId}`
**And** `PresenceService.GetActiveDevices(ctx, userID) ([]DeviceConn, error)`：`SMEMBERS + HGETALL`，返回 `[{deviceID, connID, instanceID, lastPing}]`
**And** `internal/cron/presence_cleanup_job.go` 每 2 分钟（NFR-PERF-7 范围）：
  1. `SCAN presence:*:meta` 批次 100
  2. 对每个成员检查 `Clock.Now - lastPing > 60s`（M9 Clock 注入可测）
  3. 过期成员执行与 Deregister 相同的清理（SREM + HDEL）；若是该用户最后一个成员则额外回调 `RoomService.OnUserFullyDisconnect(userID)`（Story 4.4）
  4. 走 `redisx.Locker.WithLock("presence_cleanup", 110s, fn)`（D5）
**And** FR51 实现路径覆盖：
  - 客户端主动 close（发 WS close frame）→ readPump 结束 → Hub.onDisconnect → Deregister 同步清理
  - App 退后台导致 OS 杀 WS 连接 → readPump 检测 close → 同上
  - 网络静默断（没有 close frame）→ 30s 心跳不收 pong → Hub.onHeartbeatTimeout → Deregister
  - 极端场景（Hub 崩溃未触发 Deregister）→ cron 60s 后清理
**And** Hub 本身提供 `Hub.DisconnectUser(userID) error` 方法供 Story 1.6 账户注销消费；内部遍历 `presence.GetActiveDevices` + 关闭对应 conn + 调 Deregister
**And** 单元测试：上线 / 多 device 同用户 / 心跳续期 / 60s 超时 cron 清理 / 全 device 下线清理 key / wasLastDevice 返回正确 / Clock 注入确定性
**And** 集成测试（Testcontainers Redis）：真实 Set + Hash 操作；cron 超时清理 + wasLastDevice 回调链路

### Story 4.2: RoomService + room.snapshot + 4 人 cap

As a signed-in user who just connected WS,
I want an immediate room snapshot containing myself + up to 3 most-recently-active online friends with their cat state + skin,
So that I see "my ambient room" within the first seconds of opening the app, and the 4-人-cap viewport respects Apple Watch's 2-inch screen (FR21, FR22, FR52, NFR-PERF-3).

**Acceptance Criteria:**

**Given** Story 4.1 PresenceService + Epic 2 cat_state_repo + Epic 3 friend_repo + block_repo + Epic 1 WS Client context
**When** 实现 `internal/service/room_service.go` 定义 `RoomManager` interface + `internal/ws/handlers/room_handlers.go`
**Then** `RoomManager` interface 暴露：
  - `OnUserConnect(ctx, userID, connID) error` —— WS 上线触发
  - `OnUserFullyDisconnect(userID) error` —— Story 4.4 消费
  - `GetRoomSnapshot(ctx, userID) (*RoomSnapshot, error)` —— Story 4.5 session.resume 消费
**And** WS Hub 在 upgrade 成功 + Presence.Register 完成后调 `RoomManager.OnUserConnect(ctx, userID, connID)`（Broadcaster D6 的 `PushOnConnect` hook 在此实现）
**And** `OnUserConnect` 流程：
  1. 组装 `RoomSnapshot`（调 `GetRoomSnapshot`）
  2. `broadcaster.BroadcastToUser(userID, {type: "room.snapshot", payload: snapshot})`（只推送给当事人，不广播）
**And** `GetRoomSnapshot(ctx, userID)` 流程：
  1. `friend_repo.ListFriendIDs(userID)` → 好友 ID 列表
  2. `block_repo.FilterOutBlocked(userID, friendIDs)` → 过滤双向屏蔽（Epic 3 API，若未提供则本 story 补充 `FilterOutBlocked`）
  3. `PresenceService.IsUserOnline` 并发过滤只保留在线好友（errgroup 或类似）
  4. 对在线好友并发查 `cat_state_repo.Get`（Redis 优先）
  5. 按 `updatedAt DESC` 排序，**服务端硬截断** cap = 3（加上自己的 state 共 4，FR52）
  6. 组装 self 的 catState 放在 members[0]
  7. 返回 `{members: [{userId, catState, skinId, updatedAt}, ...], selfPinnedFirst: true}`
**And** `room.snapshot` WS payload 格式与 PRD §WS Push Registry 一致：`{members: [{userId, catState, skinId, updatedAt}]}`
**And** `RoomManager` 的 `OnUserFullyDisconnect` 方法仅作为 hook 暂留空实现（Story 4.4 填充）
**And** FR52 实现：
  - `room.snapshot` members 数组硬 cap 4
  - MVP 不支持显式 `room.join` RPC；未来若加入，本 story 的 `RoomManager` 已预留 `JoinRoom(ctx, joinerID, targetUserID) error`（接口 stub），实现返回 `AppError{Code: "ROOM_FULL"}` 当目标已 4 人
  - `ROOM_FULL` error code 已在 Story 0.6 注册表（Category: client_error）
**And** `GetRoomSnapshot` 响应时间 p95 ≤ 500ms（NFR-PERF-3，即使 20 好友全在线也达标 —— 并发查询）
**And** 单元测试：0 / 1 / 3 / 4 / 20 好友场景；在线筛选；屏蔽过滤；自己位于 members[0]；cap 截断后按 updatedAt 排序；`ROOM_FULL` stub
**And** 集成测试（Testcontainers）：WS 上线后 100ms 内收到 `room.snapshot` payload + members cap 正确

### Story 4.3: RoomStateBroadcaster 实现 + 替换 Epic 2 NoOp

As a friend of an active user,
I want to receive real-time `friend.state` + `state.serverPatch` + `friend.offline` broadcasts for their cat state changes and decay tiers within p99 ≤ 3s,
So that my cat room feels genuinely "alive" with ambient co-presence — the product heart (FR23, NFR-PERF-1, D1 Broadcaster, Epic 2 StateBroadcaster integration).

**Acceptance Criteria:**

**Given** Story 4.2 RoomService + Epic 2 StateBroadcaster 接口 + Epic 3 friend_repo / block_repo + Epic 0 Broadcaster
**When** 实现 `internal/service/room_state_broadcaster.go`（实现 Epic 2 `state_service.StateBroadcaster` 接口）+ 更新 `cmd/cat/initialize.go` 替换 NoOp 实现
**Then** `RoomStateBroadcaster.OnStateChange(ctx, userID, snapshot)` 流程：
  1. `friend_repo.ListFriendIDs(userID)` → 好友列表
  2. `block_repo.FilterOutBlocked` 过滤双向屏蔽
  3. `presence.GetOnlineFriends(friendIDs)` 只保留在线
  4. 对每个在线好友并发 `broadcaster.BroadcastToUser(friendID, {type: "friend.state", payload: {friendId: userID, catState: snapshot.State, skinId: snapshot.SkinID, updatedAt: snapshot.UpdatedAt}})`（errgroup）
**And** `RoomStateBroadcaster.OnDecayTier(ctx, userID, tier, state)` 流程：
  - `tier == "weak"`：广播 `friend.state` payload 额外 `stale: "weak"` 字段（好友 UI 弱化展示）
  - `tier == "inferred_idle"`：广播 `state.serverPatch` payload `{friendId: userID, catState: "idle", reason: "inactivity_15_min"}`（PRD §WS Push Registry，我们在 payload 中加 `friendId` 以便客户端定位）
**And** `RoomStateBroadcaster.OnUserOffline(ctx, userID)` 流程：
  - `friend_repo.ListFriendIDs(userID)` → 过滤屏蔽 + 在线 → `broadcaster.BroadcastToUser(friendID, {type: "friend.offline", payload: {friendId: userID}})`
**And** `initialize.go` Service 容器注入 `RoomStateBroadcaster` 实例替换 `NoOpStateBroadcaster`（Epic 2 留的 placeholder）
**And** NFR-PERF-1 房间广播延迟 p99 ≤ 3 秒（集成测试 benchmark，见下）
**And** 性能优化：
  - 单次广播对 N 好友使用 goroutine 并发发送；总耗时 ≈ max(个人 BroadcastToUser 延迟)
  - BroadcastToUser 内部对目标 userId 的所有 device（多 connID）也并发发送
**And** 屏蔽过滤必须同时检查双方向（A 屏蔽 B 或 B 屏蔽 A 都不广播）
**And** 若 `broadcaster.BroadcastToUser` 对某个好友失败（例如 connID 刚好关闭）静默忽略（不阻塞其他好友），记 debug 日志
**And** zerolog：`{action: "broadcast_state_change", from: userID, targetCount, tier?, durationMs}`（NFR-OBS-5 核心指标）
**And** 单元测试：OnStateChange 对 N 好友并发广播 / 屏蔽过滤正确 / 离线好友不广播 / decay 三档分别触发正确 push type / BroadcastToUser 失败不阻塞
**And** 集成测试（Testcontainers + 真实 WS 客户端 + FakeClock）：
  - Benchmark：20 好友房间，1 用户 state.tick → 所有在线好友端收到 friend.state 延迟 p99 ≤ 3s
  - decay 四档端到端验证

### Story 4.4: friend.online / friend.offline 事件广播

As a signed-in user with friends,
I want to receive `friend.online` when a friend's first device connects and `friend.offline` when their last device disconnects (including heartbeat timeout),
So that my UI can show friends lighting up / dimming with accurate presence (FR24).

**Acceptance Criteria:**

**Given** Story 4.1 PresenceService `wasLastDevice` 返回值 + Story 4.2 RoomService `OnUserFullyDisconnect` hook + Story 4.3 RoomStateBroadcaster + Epic 3 friend_repo / block_repo
**When** 扩展 `PresenceService.Register` / `Deregister` 与 `RoomService` 触发广播
**Then** 用户 A **首次上线**（`SCARD presence:{A}` 从 0 → 1 即首个 device 上线）：
  1. Presence.Register 内部检测 wasFirstDevice = true
  2. 调 `RoomService.OnUserFirstConnect(ctx, A)`
  3. OnUserFirstConnect 流程：`friend_repo.ListFriendIDs(A)` → 过滤屏蔽 → 对在线好友并发 `broadcaster.BroadcastToUser(friendID, {type: "friend.online", payload: {friendId: A}})`
**And** 用户 A **全部 device 下线**（`SCARD` 从 1 → 0）：
  1. Presence.Deregister 返回 wasLastDevice = true
  2. 调 `RoomService.OnUserFullyDisconnect(ctx, A)`
  3. OnUserFullyDisconnect 流程：查好友 + 过滤屏蔽 + 在线判断 → 并发广播 `friend.offline` `{friendId: A}`
**And** Presence cleanup cron（Story 4.1）在清理 60s 超时成员且该成员是该用户最后一个时，同样回调 `RoomService.OnUserFullyDisconnect(A)` —— 保证"Hub 崩溃错过 Deregister"场景也能广播 offline
**And** 幂等保证：
  - 非首个 device 上线不触发 online（只有 wasFirstDevice == true 时触发）
  - 非最后 device 下线不触发 offline（只有 wasLastDevice == true 时触发）
  - 多副本下 cron 清理 race：通过 Story 4.1 的 WithLock + Redis 操作原子性保护（Presence 清理操作在 Lua 脚本内或 MULTI/EXEC 内，保证 wasLastDevice 判断与 SREM 原子）
**And** 单元测试：首 device 上线触发 / 第 2 device 上线不触发 / 中间 device 下线不触发 / 最后 device 下线触发 / cron 超时路径触发 / 屏蔽好友不收到 / 离线好友不发送
**And** 集成测试（Testcontainers + 真实 WS）：
  - 用户 A 单 device 上下线：B 先后收到 online / offline
  - 用户 A Watch + iPhone 两 device 上线（B 收到一次 online）→ Watch 断（B 无感知）→ iPhone 断（B 收到 offline）

### Story 4.5: session.resume 全量快照 + lastEventId 恢复

As a user whose WS connection dropped and reconnected,
I want to issue `session.resume` with my `lastEventId` and get back a complete snapshot of my state: user profile, friends, my cat, skins, blindboxes, room members,
So that my UI is fully reconstructed within 500ms without piecing together individual queries (FR25, NFR-PERF-3, D6 PushOnConnect integration, OP-4 defer: MVP returns full snapshot, not diff).

**Acceptance Criteria:**

**Given** Story 0.12 session.resume 缓存节流 + Story 4.2 RoomService.GetRoomSnapshot + Epic 1（user） + Epic 2（cat_state） + Epic 3（friends，blocks 过滤）
**When** 扩展 Story 0.12 骨架为 `internal/ws/session_resume.go` 完整 handler
**Then** 客户端发送 envelope `{id, type: "session.resume", payload: {lastEventId?: string}}`
**And** validator：`lastEventId` 若提供须为非空 string 且 ≤ 128 字符；否则 `VALIDATION_ERROR`（MVP 接受但不处理，见下）
**And** 首次 resume 流程（cache miss）：
  1. 并发组装 payload（errgroup）：
     - `user: user_repo.Get(userID) -> {id, displayName, timezone, preferences: {quietHours}}`
     - `friends: friend_repo.ListFriends + block_repo.FilterOutBlocked -> [{id, displayName, timezone, catState, skinId, updatedAt}]`（含好友在线/离线、状态）
     - `catState: cat_state_repo.Get(userID) -> {state, dailySteps, stepDelta, points, skinId, updatedAt}`
     - `skins: user_skins_repo.ListUnlocked(userID)` —— Epic 7 数据源；Epic 4 完成时该 repo 尚未实现，**本 story 的 handler 接收 `SkinsProvider` interface 参数，MVP 使用 EmptyProvider 返回 `[]`，Epic 7 接入后替换**
     - `blindboxes: blindbox_repo.ListActive(userID)` —— 同上，本 story 用 `BlindboxesProvider` interface + EmptyProvider
     - `roomSnapshot: RoomService.GetRoomSnapshot(userID)` —— 4 人 cap
  2. 响应 envelope `{id, ok: true, type: "session.resume.result", payload: {user, friends, catState, skins, blindboxes, roomSnapshot, serverTime: Clock.Now}}`
  3. 写入 `resume_cache:{userId}` Redis Hash（TTL 60s，复用 Story 0.12）
**And** 缓存命中：60s 内重复 resume 直接读缓存返回（Story 0.12 已实现）
**And** 响应时间：首次 p95 ≤ 500ms（NFR-PERF-3），缓存命中 p95 ≤ 20ms
**And** FR25 `lastEventId` MVP 处理：**接收但忽略**，始终返回全量 snapshot；OP-4 增量协议延至 Post-MVP（D6 已预留 `BroadcastDiff` 接口）
**And** 响应 payload 包含 `serverTime`（Clock 注入），供客户端计算时钟偏差
**And** WS 断连重连场景：客户端重连建立新 WS → 调 `session.resume` → 完整状态在单一响应内恢复，无需多轮 RPC
**And** 屏蔽过滤：`friends` 和 `roomSnapshot` 均要过滤双向屏蔽
**And** Service 层 `SessionResumeService` 接受 `SkinsProvider` / `BlindboxesProvider` 两个 interface 参数（消费方定义）；Epic 7/6 实现时 `initialize.go` 替换真实实现
**And** 单元测试：首次 / 缓存命中 / 空好友 / 空 skins / 空 blindboxes / 屏蔽过滤 / lastEventId 字段忽略 / Clock 注入的 serverTime
**And** 集成测试（Testcontainers + 真实 WS）：
  - WS 断连 → 重连 → session.resume → 响应 payload 完整
  - Benchmark：20 好友 p95 ≤ 500ms

---

## Epic 5: 触碰 / 触觉社交

用户可以向好友发送触碰事件（指定表情类型），接收方在线时 WS 送达 `friend.touch`，离线时降级 APNs；服务端强制执行 per-sender→receiver 60s ≤ 3 次限流（超限静默丢弃、不告知发送方）、屏蔽拦截、接收方本地免打扰时段降级为静默推送。

### Story 5.1: Touch 领域 + touch_logs repo + emoteType 枚举

As a backend developer,
I want a pure domain layer defining EmoteType enum and touch_logs repository, with async write semantics,
So that all touch delivery logic in this epic layers on a consistent type-safe foundation without blocking the main send path (M6, M7, D11).

**Acceptance Criteria:**

**Given** Epic 0 + Epic 1 + Epic 2 + Epic 3 + Epic 4 交付
**When** 实现 `internal/domain/touch.go` + `internal/repository/touch_log_repo.go` + `internal/dto/touch_dto.go`
**Then** 定义 `type EmoteType string` 枚举 + typed constants：MVP 至少含 `EmoteTypeHeart = "heart"`（"比心哦"，J1 场景）；其他表情在产品 UX 阶段定义后追加
**And** `ValidateEmoteType(e EmoteType) error` 返回 sentinel error `ErrInvalidEmoteType` 若未注册
**And** **NOTE**（PRD §UX Warnings）：客户端 UX 阶段定义 `emoteType ↔ 触觉 pattern` 映射；服务端仅转发枚举值，不关心触觉实现
**And** 定义 `Touch` 值对象 `{FromUser ids.UserID, ToUser ids.UserID, EmoteType EmoteType, CreatedAt time.Time, DeliveryPath DeliveryPath, QuietMode bool}`；`DeliveryPath` 枚举 `PathWS = "ws" / PathAPNs = "apns" / PathSilentDrop = "silent_drop"`
**And** `touch_logs` collection 字段：`{_id: ObjectId, fromUser, toUser, emoteType, deliveryPath, quietMode, createdAt}`
**And** `touch_log_repo` 提供 `Create(ctx, log *Touch) error` 方法；**异步写模式**：调用方 `go repo.Create(...)` 或通过 `errgroup.Go`（不阻塞主业务响应）
**And** Mongo 写入失败仅 zerolog warn `{action: "touch_log_write_failed", from, to, err}`，不影响发送方 ack（touch_log 是审计/分析用，非关键路径，NFR-REL-5 例外）
**And** `touch_log_repo.EnsureIndexes(ctx)`：`(fromUser, createdAt)` compound index（排查发送历史）+ `createdAt` TTL 90 天 index（D11 数据保留）；由 `initialize()` 启动期调用
**And** 单元测试：EmoteType 校验 / 合法 Touch 构造 / repo Create 成功 / Create 失败不 panic / Index 创建
**And** 集成测试（Testcontainers）：TTL index 生效（插入后超 TTL 自动删除）

### Story 5.2: touch.send WS RPC + 在线 WS 送达 + 离线 APNs 降级

As a signed-in user who wants to send a touch (e.g., "比心哦") to a friend,
I want my touch delivered via WebSocket if the friend is online, with automatic APNs fallback otherwise,
So that the friend receives the touch within seconds in foreground or as a push in background (FR26, FR27, NFR-REL-2 送达率 ≥ 99%).

**Acceptance Criteria:**

**Given** Story 5.1 Touch 领域 + Epic 4 PresenceService / Broadcaster + Epic 0 Pusher + Epic 3 friend_repo + Story 0.10 eventId dedup
**When** 实现 `internal/service/touch_service.go` + `internal/ws/handlers/touch_handlers.go`
**Then** 客户端发送 WS envelope `{id, type: "touch.send", payload: {friendId: string, emoteType: string}}`
**And** dispatcher 走 eventId dedup（Story 0.10，避免重放）
**And** validator：`friendId` 非空 + 符合 UserID 格式；`emoteType` 走 `domain.ValidateEmoteType`；失败 `VALIDATION_ERROR`
**And** handler 从 WS Client context 解出 `fromUserID`，调 `TouchService.SendTouch(ctx, fromUserID, toUserID, emoteType)`
**And** `TouchService.SendTouch` MVP baseline 流程（本 story 不含 block / ratelimit / quietHours —— 后续 5.3/5.4/5.5 追加）：
  1. **Friend check**：`friend_repo.AreFriends(fromUserID, toUserID)`；不是好友 → 返回 `AppError{Code: "FRIEND_NOT_FOUND"}`（Category: client_error；新错误码在 Story 0.6 error_codes.go 追加）
  2. **接收方 displayName 查询**：`user_repo.Get(toUserID).displayName`（for APNs body）
  3. **Online check**：`PresenceService.IsUserOnline(toUserID)`
     - 在线：调 `broadcaster.BroadcastToUser(toUserID, {type: "friend.touch", payload: {friendId: fromUserID, emoteType, quietMode: false}})`；`DeliveryPath = PathWS`
     - 离线：调 `pusher.Enqueue(ctx, toUserID, PushPayload{Kind: PushKindAlert, Title: "<senderDisplayName>", Body: mapEmoteTypeToBody(emoteType), DeepLink: "cat://touch?from=<fromUserID>", RespectsQuietHours: true, IdempotencyKey: envelope.id})`；`DeliveryPath = PathAPNs`
  4. **touch_log 异步写**：`go touchLogRepo.Create(ctx, &Touch{FromUser, ToUser, EmoteType, CreatedAt: Clock.Now, DeliveryPath, QuietMode: false})`
  5. 返回 `{ok: true, payload: {delivered: true, path: "ws" | "apns"}}`
**And** `friend.touch` push payload 格式与 PRD §WS Push Registry 一致：`{friendId, emoteType}` + MVP 追加 `quietMode: bool`
**And** APNs body 模板：`mapEmoteTypeToBody(emoteType)` 返回本地化字符串（MVP i18n 延后 —— 统一中文 "XXX 给你发了一个 <emote>"；OP-7 Vision 阶段做 i18n）
**And** 响应时间 p95 ≤ 200ms（NFR-PERF-4 WS RPC 往返）
**And** 送达率监控：touch_logs `deliveryPath` 分布 + Pusher DLQ 计数 → NFR-OBS-5 核心指标 "触碰送达率"（NFR-REL-2 ≥ 99%）
**And** zerolog info：`{action: "touch_send", from, to, emoteType, path, durationMs}`（NFR-OBS-5）
**And** `deviceId` 相关字段在 Touch 中不存储（PRD 未要求，按用户级聚合）
**And** Pusher.Enqueue `IdempotencyKey = envelope.id`：跨层去重（Story 0.10 WS 层 + Epic 0 Pusher 层）保证重试安全
**And** 单元测试：好友 online → WS path / 好友 offline → APNs path / 非好友拒绝 / 无效 emoteType 拒绝 / touch_log 异步写入 / Pusher.Enqueue 被调用并含正确 payload
**And** 集成测试（Testcontainers + mock APNs）：端到端 WS 送达 + 离线 APNs 入队验证

### Story 5.3: 触碰频次限流（per-pair 60s ≤ 3 silent drop）

As a product owner committed to anti-harassment,
I want per-sender → per-receiver rate limit 60s ≤ 3 with SILENT drop (sender sees success, receiver never sees it),
So that harassers can't flood a receiver, but also can't game the system by observing which sends "fail" — the "避免对抗" design principle (FR28, NFR-SCALE-6, PRD §Domain-Specific).

**Acceptance Criteria:**

**Given** Story 5.2 baseline + Epic 0 Redis
**When** 扩展 `TouchService.SendTouch` 加入 rate limit preflight（在 friend check 之后）
**Then** 限流规则：`ratelimit:touch:{fromUserId}:{toUserId}` Redis counter；`INCR key; EXPIRE key 60`（pipeline 原子）
**And** 阈值 max = 3（60s 内 ≤ 3 次）；阈值可通过 Redis Hash `config:runtime.touch_rate_limit_max` 动态覆盖（D13）
**And** 计数 > 3 触发**静默丢弃**（silent drop）语义：
  - **不返回 error**给发送方（不返回 `RATE_LIMIT_EXCEEDED`）
  - **不送达**接收方（不调 broadcaster / pusher）
  - **不写 touch_log**（避免污染送达率指标；silent drop 视为未发生）
  - 响应发送方 `{ok: true, payload: {delivered: true, path: "silent_drop"}}`（欺骗性响应，发送方无法辨别）
  - 仅 zerolog **debug** `{action: "touch_rate_limited", from, to, counter}`（不 info 级避免日志泄露限流逻辑）
**And** **Rationale**: PRD §Domain-Specific Requirements：「超限静默丢弃（不告知发送方被限流，避免对抗）」—— 核心反骚扰设计
**And** SendTouch 流程调整（rate limit 在 friend check 之后、送达之前）：
  ```
  0. [未来 5.4 的 block check 放这里]
  1. Rate limit check → 超限 silent drop return
  2. Friend check → 非好友 FRIEND_NOT_FOUND
  3. Online check → WS | APNs
  4. touch_log write
  5. ack
  ```
**And** 单元测试 table-driven：1/2/3/4/5 次发送（前 3 按路径正常送达；第 4/5 silent drop）；TTL 60s 自动重置后再 3 次成功；不同 (from, to) pair 独立；同一 from 对不同 to 独立
**And** 集成测试（Testcontainers）：真实 Redis counter；60s 内 5 次 → 前 3 送达，后 2 silent；touch_logs 仅 3 条记录（silent drop 不写）；发送方响应全部 `ok: true`

### Story 5.4: 屏蔽拦截（接收方屏蔽发送方）silent drop

As a user who has blocked a harasser,
I want the harasser's touches to be silently dropped server-side (never delivered, never notified to harasser),
So that blocking provides real protection without escalating harassment — the sender never knows they've been blocked (FR29, PRD §Domain-Specific 显式屏蔽执行).

**Acceptance Criteria:**

**Given** Story 5.2 baseline + Story 5.3 rate limit + Epic 3 block_repo
**When** 扩展 `TouchService.SendTouch` 加入 block preflight（在 rate limit 之前）
**Then** 屏蔽规则：查 `blocks` collection `(blocker == toUserId, blocked == fromUserId)` —— **单向查询：接收方是否屏蔽发送方**
**And** 匹配 → 触发**静默丢弃**（与 5.3 同样语义）：
  - 不返回 error 给发送方
  - 不送达接收方
  - 不写 touch_log
  - 不消耗 rate limit counter（block 优先于 limit，不让屏蔽方的发送耗掉限流额度）
  - 响应 `{ok: true, payload: {delivered: true, path: "silent_drop"}}`（与 rate limit 返回无法区分）
  - zerolog debug `{action: "touch_blocked", from, to}`（不 info）
**And** **不告知发送方被屏蔽**（PRD §Domain-Specific "B 的 touch.send 被服务端拦截（不送达、不通知 B 被屏蔽）"）
**And** 注意：本 check 是 **receiver blocks sender**；**不**处理 sender blocks receiver 场景（单向屏蔽的对称性：若发送方屏蔽了接收方，但接收方未屏蔽发送方，发送方仍可发送 —— 但实际 UI 上发送方可能看不到被屏蔽的好友所以发不出，客户端层拦截；服务端不做 sender-side block 检查）
**And** SendTouch 最终流程：
  ```
  0. Block check (本 story)        → silent drop
  1. Rate limit check (5.3)        → silent drop
  2. Friend check                  → FRIEND_NOT_FOUND error
  3. Online check                  → WS | APNs + quietMode (5.5)
  4. touch_log write
  5. ack
  ```
**And** 单元测试：接收方屏蔽发送方 → silent drop / 发送方屏蔽接收方（单向，接收方未屏蔽）→ 正常送达 / 双向屏蔽 → silent drop（被 receiver blocks sender 捕获）/ 无屏蔽 → 正常 / block 场景不消耗 rate counter
**And** 集成测试（Testcontainers）：端到端 block + touch → silent；touch_logs 验证无记录

### Story 5.5: 跨时区免打扰降级（quietMode 标记 + APNs silent）

As a user asleep at midnight local time,
I want touches from friends in other timezones silenced (no vibration, no screen wake) per my quiet-hours preference,
So that my sleep isn't disrupted by someone's daytime 比心哦 from 10 hours away (FR30, NFR-COMP-3 APNs Guidelines, PRD §Domain-Specific 跨时区免打扰).

**Acceptance Criteria:**

**Given** Story 5.2 baseline + Story 5.3/5.4 preflight + Epic 1 users.preferences.quietHours / timezone + Epic 0 Pusher + Clock
**When** 扩展 `TouchService.SendTouch` 的送达阶段加入 quiet-hours 判断
**Then** 定义辅助 `domain.IsInQuietHours(user *User, now time.Time) bool`：
  1. 读 `user.timezone`（默认 UTC 若未设置）+ `user.preferences.quietHours{start, end}`（默认 `{start: "23:00", end: "07:00"}`）
  2. `loc, err := time.LoadLocation(user.timezone)`；err 则 fallback UTC + zerolog warn
  3. `local := now.In(loc)`；格式化为 `HH:MM`
  4. 判断是否在 `[start, end)` 区间；跨午夜（`start > end`，如 `23:00-07:00`）需特殊处理：`hm >= start || hm < end`
**And** SendTouch 送达阶段修改（step 3）：
  - 查接收方 User（`user_repo.Get(toUserID)`）（可与 5.2 的 displayName 查询合并一次）
  - `quietMode := IsInQuietHours(receiverUser, Clock.Now())`
  - **若在线 WS 送达**：payload 附加 `quietMode: true` 标记 → `{friendId, emoteType, quietMode: true | false}`（PRD §Domain-Specific "WS 触碰消息打 quietMode: true 标记供客户端判断"）
  - **若离线 APNs 送达**：`PushPayload.RespectsQuietHours = true`（已在 5.2 设置）；Pusher 内部（Story 0.13 实现）检测 quietMode 后强制 `Kind = silent`（`apns-push-type: background`，无震动无亮屏）
**And** `touch_logs.quietMode` 字段记录实际应用的 quiet 状态
**And** 默认 `quietHours = {start: "23:00", end: "07:00"}` 在 Story 1.1 用户创建时设置
**And** Clock 通过依赖注入使可测（FakeClock 模拟不同时刻）
**And** NFR-COMP-3 APNs Guidelines：quietHours 期间 APNs 必须为 silent / background，不得出现 alert / sound / badge
**And** 单元测试 table-driven：
  - 跨午夜边界：22:59 非 quiet / 23:00 quiet / 06:59 quiet / 07:00 非 quiet
  - 非跨午夜：`quietHours = {09:00-17:00}` 白天免打扰场景（边界 08:59 / 09:00 / 16:59 / 17:00）
  - 不同时区：上海（UTC+8）凌晨 2 点 vs 纽约（UTC-5）下午 1 点 → 上海用户 quiet，纽约用户非 quiet
  - 未设置 timezone → UTC fallback
  - quietHours 未设置 → 默认 23:00-07:00
**And** 集成测试（Testcontainers + FakeClock + mock APNs）：
  - 接收方在线 + 在 quiet 时段 → WS 送达含 `quietMode: true`
  - 接收方离线 + 在 quiet 时段 → APNs Pusher 被调用且 RespectsQuietHours=true；mock APNs 验证收到 `apns-push-type: background`

---

## Epic 6: 盲盒（掉落 + 领取）

用户挂机满 30 分钟（且当前无未领取盲盒时）获得新盲盒（单槽位 cron 投放）；步数达到解锁阈值后可领取盲盒，领取走 Mongo 事务 + Redis SETNX 幂等去重，保证每盒最多成功领取一次（零重复 - 一票否决）；领取成功返回奖励详情（皮肤 ID + 稀有度）；抽到已拥有皮肤时返回折算点数；用户可查询盲盒库存（待解锁 / 已解锁 / 已领取）。

### Story 6.1: Blindbox + Skin 领域 + skins seed + reward 权重表

As a backend developer,
I want domain types for blindbox / skin rarity + a weighted reward roll + 5 MVP skins seeded into the catalog,
So that the blindbox roll logic in 6.4 and the skin catalog queries in Epic 7 share the same type-safe foundation with deterministic random sampling (D10, FR31-39 基础, Epic 7 seed).

**Acceptance Criteria:**

**Given** Epic 0 + Epic 1 + Epic 2 cat_states（points 字段已存在）
**When** 实现 `internal/domain/{blindbox.go, skin.go}` + `internal/repository/{blindbox_repo.go, skin_repo.go, user_skin_repo.go}` + `tools/seed_skins/main.go`
**Then** Domain types：
  - `BlindboxStatus` enum：`BlindboxStatusPending = "pending"`（掉落后待领取）/ `BlindboxStatusRedeemed = "redeemed"`（已领取，终态）—— MVP 简化为两状态；步数达标判断在 redeem preflight 不入 status
  - `SkinRarity` enum：`SkinRarityCommon = "common" / SkinRarityRare = "rare" / SkinRarityEpic = "epic" / SkinRarityLegendary = "legendary"`
  - `Blindbox` 值对象：`{ID: ids.BlindboxID, UserID, Status, StepsRequired int64, RewardSkinID *ids.SkinID, RewardRarity *SkinRarity, PointsAwarded *int64, IsNewSkin *bool, CreatedAt, RedeemedAt *time.Time}`
  - `Skin` 值对象：`{ID: ids.SkinID, Name string, Rarity SkinRarity, Layer int, AssetPath string, ReleasedAt time.Time}`（`Layer` 为未来分层换装预留）
**And** 权重表定义在 `config/default.toml` 的 `[blindbox]` section（D13 基础设施级配置，重启生效）：
  ```toml
  [blindbox]
  default_steps = 200
  [blindbox.reward_weights]
  common = 0.70
  rare = 0.20
  epic = 0.09
  legendary = 0.01
  [blindbox.points_conversion]
  common = 10
  rare = 50
  epic = 200
  legendary = 1000
  ```
  - `config.MustLoad` 解析为 `cfg.Blindbox.RewardWeights` 和 `cfg.Blindbox.PointsConversion` 结构体
  - 权重和必须 == 1.0（`config.mustValidate()` 校验，否则 `log.Fatal` 启动失败）
  - 新皮肤与已有皮肤都奖励点数（MVP 简化；FR54 的"折算"由 IsNewSkin=false 触发无皮肤解锁仅获点数实现）
**And** 函数 `RollReward(skins []Skin, weights RewardWeightTable, rng io.Reader) (*Skin, SkinRarity, error)`：
  1. 按权重抽稀有度（累积分布 + 统一随机数）
  2. 从 skins 中 `rarity == 抽中值` 的子集均匀抽一个
  3. 子集为空 → 降级到 `common` 稀有度子集；再空则返回 `ErrNoSkinsAvailable`
  4. `rng` 参数支持依赖注入（测试用 `rand.NewSource(seed)` 确定性）
**And** Collections schema：
  - `blindboxes`：`{_id, userId, status, stepsRequired, rewardSkinId, rewardRarity, pointsAwarded, isNewSkin, createdAt, redeemedAt}`
  - `skins`：`{_id, name, rarity, layer, assetPath, releasedAt}`
  - `user_skins`：`{_id, userId, skinId, unlockedAt}`
**And** Repositories 实现 `EnsureIndexes(ctx)`（由 `initialize()` 启动期调用）：
  - `blindbox_repo`：`(userId, status)` compound index（FR31 单槽位判定 + FR34 inventory 查询）+ `createdAt` single（未来分析）
  - `skin_repo`：`rarity` single index（FR31 reward roll 按稀有度筛选）
  - `user_skin_repo`：`(userId, skinId)` unique compound（防重；6.4 redeem 兜底）+ `userId` single（FR37 已解锁列表）
**And** Seed data（MVP 至少 5 个皮肤覆盖 4 稀有度）：
  - `{id: "default_cat", name: "默认猫", rarity: common, layer: 0, assetPath: "cdn://skins/default.png"}`
  - `{id: "chef_hat", name: "厨师帽", rarity: common, layer: 1, assetPath: "cdn://skins/chef_hat.png"}`（J2 场景）
  - `{id: "red_scarf", name: "红围巾", rarity: rare, layer: 1, assetPath: "..."}`
  - `{id: "tuxedo", name: "燕尾服", rarity: epic, layer: 0, assetPath: "..."}`
  - `{id: "golden_aura", name: "金光", rarity: legendary, layer: 2, assetPath: "..."}`
**And** `tools/seed_skins/main.go` 一次性脚本：从 embed JSON（或 TOML）读 seed 数据，`InsertMany` 到 skins collection；已存在则 upsert（按 `_id`）
**And** `initialize()` 启动期在 dev mode（`cfg.Server.Mode == "debug"`）检测 `skins` collection 为空 → 自动调用 seed 逻辑（生产环境要求运维先跑 `tools/seed_skins`）
**And** 单元测试 table-driven：
  - `RollReward` 1e6 次采样：稀有度分布 ± 1% 符合权重（确定性 seed + 统计断言）
  - 同稀有度内均匀分布
  - 空子集降级到 common
  - 全空返回 `ErrNoSkinsAvailable`
  - Blindbox 状态枚举合法性
**And** 集成测试（Testcontainers）：
  - skins seed 插入 + 查询返回全部 5 个
  - blindboxes + user_skins index 生效（唯一性冲突正确报错）

### Story 6.2: Blindbox drop cron（30 min 挂机 + 单槽位）+ 通知

As an idle user whose cat has been sitting for 30 minutes,
I want a blindbox automatically dropped into my inventory and notified via WS (online) or APNs (offline),
So that the J2 sedentary office worker journey triggers — "你的猫捡到了一个盲盒，走 200 步打开它" without any explicit action (FR31, NFR-PERF-7).

**Acceptance Criteria:**

**Given** Story 6.1 domain + Epic 0（cron + Lock + Pusher + Clock + Broadcaster + logx）+ Epic 2 cat_states
**When** 实现 `internal/cron/blindbox_drop_job.go` + `internal/service/blindbox_service.go::DropForIdleUsers`
**Then** cron `blindbox_drop` 每 5 分钟运行；走 `redisx.Locker.WithLock("blindbox_drop", 280s, fn)`（D5, FR56）
**And** 扫描条件（挂机 ≥ 30 分钟 + 单槽位）：
  1. `SCAN Redis state:*` 批次 100（避免阻塞）
  2. 对每个 userId 读 `state:{userId}.updatedAt`；若 `Clock.Now - updatedAt >= 30min` 视为候选
  3. **仅扫描活跃用户**：`user.lastLoginAt > Clock.Now - 30d`（避免给 churned 用户 drop 浪费资源）
  4. 对候选查 `blindbox_repo.HasPending(userId)` —— `blindboxes where userId == X && status == "pending"`；若存在则跳过（FR31 单槽位）
  5. 通过的候选 → 创建 blindbox
**And** Blindbox 创建：
  - `stepsRequired` 从 `cfg.Blindbox.DefaultSteps`（默认 200，可 200/300/500 随机 —— MVP 简化固定 200）
  - `blindbox_repo.Create(ctx, &Blindbox{UserID, Status: Pending, StepsRequired: 200, CreatedAt: Clock.Now})`
  - 返回新 blindboxId
**And** 通知路径：
  - `PresenceService.IsUserOnline(userID)` → 在线：`broadcaster.BroadcastToUser(userID, {type: "blindbox.drop", payload: {blindboxId, stepsRequired}})`
  - 离线：`pusher.Enqueue(ctx, userID, PushPayload{Kind: alert, Title: "裤衩猫", Body: "你的猫捡到了一个盲盒！", DeepLink: "cat://blindbox/<id>", RespectsQuietHours: true, IdempotencyKey: "blindbox_drop_<blindboxId>"})`
  - 当用户之后上线 `session.resume` 时，`blindboxes` 会出现在 payload（Epic 4 Story 4.5 通过 `BlindboxesProvider`，本 story 接入 `blindbox_repo.ListActive(userID)` 作为 provider 实现）
**And** `stepsRequired` 可通过 `cfg.Blindbox.DefaultSteps` 动态覆盖（D13）
**And** Clock 注入：cron job + Service 均持 Clock；测试用 FakeClock.Advance 可确定性跨 30min 边界
**And** 扫描性能：MVP 目标 1000 活跃用户 → 单次 cron < 10s；可接受 5 分钟频率（NFR-PERF-7 盲盒投放 ≤ 60s 的"60s"理解为"单次投放耗时"，cron 频率是 5 分钟）
**And** zerolog info：`{action: "blindbox_dropped", userId, blindboxId, stepsRequired}`；扫描总结记一条 debug `{action: "blindbox_drop_scan_done", scanned, dropped, skipped}`
**And** 失败隔离：某用户 drop 失败（例如 Mongo 写入冲突）不影响其他用户；zerolog warn 记录
**And** 单元测试：
  - FakeClock 跨 30min 边界（29min/30min/31min）
  - 已有 pending 跳过
  - churned 用户（lastLogin > 30d）不扫
  - 在线 → WS path / 离线 → APNs path
  - Lock 保护验证（并发 cron 只 1 个执行）
**And** 集成测试（Testcontainers + FakeClock）：
  - 插入用户 A cat_states.updatedAt = -35min + 无 pending → cron → blindbox 创建 + blindbox.drop push 触发
  - 用户 B 已有 pending → cron 不再 drop B
  - `FakeClock.Advance(5min)` 再跑 cron → A 仍单槽位（因为已有 pending）

### Story 6.3: Blindbox inventory 查询

As a signed-in user curious about my blindbox collection,
I want to query all my blindboxes (pending + redeemed) with current step progress per pending box,
So that my UI shows "盲盒 x2 待开启 (180/200 步)" live without me guessing (FR34, NFR-PERF-4).

**Acceptance Criteria:**

**Given** Story 6.1 + Epic 1 JWT + Epic 2 cat_states + Story 0.9 WS dispatcher
**When** 实现 `internal/ws/handlers/blindbox_handlers.go::HandleBlindboxInventory`
**Then** 客户端 WS envelope `{id, type: "blindbox.inventory", payload: {statusFilter?: "pending" | "redeemed" | "all"}}`；默认 `statusFilter == "all"`
**And** **不走** eventId dedup（读操作）
**And** 查询 `blindbox_repo.ListByUser(ctx, userID, statusFilter)` 返回按 `createdAt DESC` 排序
**And** 对每个 pending blindbox 附加 `stepsCurrent`：
  1. 查 `cat_states.dailySteps`
  2. `stepsCurrent = min(dailySteps, stepsRequired)`（防超显示）
  3. `redeemable = stepsCurrent >= stepsRequired`
**And** 响应 envelope `{id, ok: true, type: "blindbox.inventory.result", payload: {blindboxes: [{id, status, stepsRequired, stepsCurrent, redeemable, rewardSkinId?, rewardRarity?, pointsAwarded?, isNewSkin?, createdAt, redeemedAt?}, ...]}}`
**And** 响应时间 p95 ≤ 200ms（NFR-PERF-4）
**And** 已 redeemed 的 blindbox 返回完整 reward 信息（`rewardSkinId / rewardRarity / pointsAwarded / isNewSkin`）
**And** 单元测试：空列表 / 仅 pending / 仅 redeemed / 混合 / filter / stepsCurrent 上限 clamping
**And** 集成测试（Testcontainers）：插入多个 blindboxes + cat_states → 查询返回正确 payload

### Story 6.4: Blindbox redeem 强一致事务 + 抽奖 + 折算点数 + friend.blindbox 广播

As a signed-in user who has walked 200 steps,
I want to redeem a pending blindbox and receive my reward (new skin + points, OR points-only if I already own that skin) atomically,
So that no retry / client bug / concurrent tap can ever cause double-redemption or inconsistent state — the one-票-否决 guarantee (FR32, FR33, FR35, FR54, NFR-REL-3, D10).

**Acceptance Criteria:**

**Given** Story 6.1-6.3 + Epic 0（eventId dedup / Mongo WithTransaction / Broadcaster / Clock）+ Epic 2 cat_state_repo + Epic 3 friend_repo + Epic 4 RoomStateBroadcaster + Story 0.12 ResumeCacheInvalidator
**When** 实现 `BlindboxService.Redeem(ctx, userID, blindboxID, envelopeID)` + `RoomStateBroadcaster.OnBlindboxRedeemed` 扩展方法 + handler
**Then** 客户端 WS envelope `{id, type: "blindbox.redeem", payload: {blindboxId: string}}`
**And** dispatcher 走 Story 0.10 eventId dedup（第一层：客户端重试 5min 窗口内返回缓存结果）
**And** validator：`blindboxId` 非空 + 有效 ID 格式
**And** `BlindboxService.Redeem` 流程：
  - **Preflight（事务外，可变量预计算）**：
    1. `blindbox_repo.Get(blindboxID)` → 不存在 → `BLINDBOX_NOT_FOUND`（Category: client_error）
    2. Ownership：`blindbox.userId != requesterUserId` → `BLINDBOX_NOT_FOUND`（不泄露是否存在）
    3. `blindbox.status == "redeemed"` → `BLINDBOX_ALREADY_REDEEMED`（Category: client_error）
    4. 读 `cat_state_repo.Get(userID).dailySteps`；若 `< stepsRequired` → `BLINDBOX_INSUFFICIENT_STEPS`（Category: client_error）
    5. 加载 skins catalog（Redis `skins:catalog` Hash 5min TTL 缓存 miss 则 `skin_repo.ListAll` 回填）
    6. 调 `domain.RollReward(skins, weights, crypto/rand.Reader)` 得到 `rewardSkin + rewardRarity`
    7. `user_skin_repo.Exists(userID, rewardSkin.ID)` → `alreadyOwned bool`
    8. 读 `pointsConversionTable[rewardRarity]` → `pointsAwarded`
  - **Transaction（第二层防重 + 原子性，D10）**：
    ```
    err := mongox.WithTransaction(ctx, cli, func(sc mongo.SessionContext) error {
      // Step A: Conditional update (TOCTOU race protection)
      //   UPDATE blindboxes SET status=redeemed, rewardSkinId=..., rewardRarity=..., pointsAwarded=..., isNewSkin=..., redeemedAt=Clock.Now
      //   WHERE _id=blindboxID AND status=pending
      res, err := blindbox_repo.MarkRedeemed(sc, blindboxID, rewardSkin.ID, rewardRarity, pointsAwarded, !alreadyOwned, Clock.Now)
      if err != nil { return err }
      if res.Modified == 0 { return ErrAlreadyRedeemed }  // 并发竞争 → 转 AppError{BLINDBOX_ALREADY_REDEEMED}
      
      // Step B: 新皮肤时写 user_skins (unique index 第三层保障)
      if !alreadyOwned {
        if err := user_skin_repo.Insert(sc, userID, rewardSkin.ID, Clock.Now); err != nil { return err }
      }
      
      // Step C: 增加点数
      if err := cat_state_repo.IncrementPoints(sc, userID, pointsAwarded); err != nil { return err }
      
      return nil
    })
    ```
  - **NFR-REL-3 三层保障**：
    1. Story 0.10 eventId dedup（客户端重试）
    2. Conditional update `WHERE status=pending`（并发 TOCTOU race）
    3. `user_skins (userID, skinID)` unique index（最后兜底 —— 理论上第 2 层已阻止，但防御纵深）
**And** 事务成功后：
  - 响应 envelope `{id, ok: true, type: "blindbox.redeem.result", payload: {reward: {skinId: rewardSkin.ID, rarity: rewardRarity, skinName: rewardSkin.Name, isNew: !alreadyOwned, pointsAwarded}}}`
  - FR54 体现：`isNew == false` 时前端显示"已拥有，折算 N 点"；`isNew == true` 时前端显示"新皮肤解锁"+点数也奖励（MVP 简化：都给点数）
**And** 事务成功后触发广播：调 `RoomStateBroadcaster.OnBlindboxRedeemed(ctx, userID, rewardSkin.ID, rewardRarity)`（本 story 在 Epic 4 的 broadcaster 上扩展此方法）
  - `OnBlindboxRedeemed` 流程：查好友 + 过滤屏蔽 + 在线 → 并发 `broadcaster.BroadcastToUser(friendID, {type: "friend.blindbox", payload: {friendId: userID, skinId, rarity}})`（PRD §WS Push Registry）
**And** 事务成功后：`resumeCacheInvalidator.Invalidate(ctx, userID)`（Story 0.12，blindboxes + skins + points 均变动）
**And** 响应时间 p95 ≤ 200ms（NFR-PERF-4）
**And** zerolog 审计：`{action: "blindbox_redeem", userId, blindboxId, rewardSkinId, rarity, isNew, pointsAwarded, durationMs}`（NFR-SEC-10 + NFR-OBS-5 盲盒领取率核心指标）
**And** 事务失败的 error mapping：
  - `ErrAlreadyRedeemed`（并发竞争）→ `AppError{Code: "BLINDBOX_ALREADY_REDEEMED"}`
  - `user_skins` unique 冲突（理论不应发生）→ 事务 rollback，记 zerolog error（保障触发证明第 2 层失败，需调查）
  - 其他（网络 / 事务超时）→ `INTERNAL_ERROR`（retryable）
**And** 单元测试：
  - Preflight 各 error：not found / wrong user / already redeemed / insufficient steps
  - 事务 happy path：新皮肤 + 点数 + user_skins 插入 + cat_states.points +N + blindboxes.status → redeemed + friend.blindbox 广播
  - FR54 已有皮肤折算：alreadyOwned=true → isNew=false + pointsAwarded>0 + user_skins 不插入
  - 并发 race 模拟：两个 goroutines 同时 redeem 同 blindboxId → 1 成功 1 收到 `BLINDBOX_ALREADY_REDEEMED`
  - RollReward 确定性 seed 下可断言结果
  - resume cache 被失效
  - friend.blindbox 广播被调用（屏蔽过滤正确）
**And** 集成测试（Testcontainers + 真实 Mongo 事务）：
  - Happy path：setup user + blindbox (stepsRequired=200) + cat_states.dailySteps=250 → redeem → 各 collection 正确
  - **并发 redeem 验证零重复**：10 个 goroutines 同时 redeem 同 blindboxId → 1 成功 9 失败；最终状态 user_skins 仅 1 记录、cat_states.points 仅增 1 次、blindboxes.status=redeemed（NFR-REL-3 强证明）
  - FR54 已有皮肤场景
  - eventId dedup 重试验证

---

## Epic 7: 皮肤与定制

用户可以查询全部皮肤目录（含默认猫 + 可解锁皮肤）、自己已解锁的皮肤列表、装配一款已解锁的皮肤给自己的猫；装配结果随房间状态广播被好友看到（`cat_state.skinId` 在 `friend.state` payload 内下发）。FR36-38 可独立交付用户价值；FR39 在 Epic 4 完成后自动点亮。

### Story 7.1: 皮肤目录查询

As a signed-in user browsing available skins,
I want to see the full skin catalog with name / rarity / asset path for every skin including the default cat,
So that I know what's possible to collect and can preview skins before opening blindboxes (FR36, PRD §皮肤与定制).

**Acceptance Criteria:**

**Given** Epic 6 Story 6.1 的 skins collection + seed 数据 + Epic 1 JWT + Story 0.9 WS dispatcher
**When** 实现 `internal/ws/handlers/skin_handlers.go::HandleSkinsCatalog`
**Then** 客户端 WS envelope `{id, type: "skins.catalog", payload: {}}`
**And** **不走** eventId dedup（读操作）
**And** 查询 `skin_repo.ListAll(ctx)` → 按 `rarity` 分组或按 `releasedAt ASC` 排序（MVP 简化：按 rarity 排 common → legendary）
**And** 缓存策略：skins catalog 极少变化 → 在 Service 层做进程内缓存 + Clock TTL 5 分钟刷新（不走 Redis，避免 catalog 变更后 5 分钟延迟超预期时增大 TTL 即可）；MVP 5 个皮肤，查 Mongo 也 < 5ms
**And** 响应 envelope `{id, ok: true, type: "skins.catalog.result", payload: {skins: [{id, name, rarity, layer, assetPath}, ...]}}`
**And** 响应时间 p95 ≤ 200ms（NFR-PERF-4）；catalog 5 个皮肤远低于此
**And** `assetPath` 含完整 CDN 前缀（`cfg.CDN.BaseURL + skin.AssetPath`）或直接返回相对路径由客户端拼接（MVP 由客户端拼接更灵活；`assetPath` 是相对路径如 `skins/chef_hat.png`）
**And** 包含"默认猫"皮肤（`default_cat`），即使它不需要解锁 —— 客户端用此区分"已装配 vs 可解锁"
**And** 单元测试：空 catalog / 5 个 seed / 排序正确 / 缓存命中不查 Mongo / 缓存过期重查
**And** 集成测试（Testcontainers）：seed 数据 → skins.catalog → 返回 5 个 + 字段完整

### Story 7.2: 已解锁皮肤列表 + session.resume SkinsProvider 接入

As a signed-in user checking my skin collection,
I want to see which skins I've unlocked (from blindbox redemptions) with unlock timestamps,
So that I know what I can equip and what's still locked in the catalog (FR37, Epic 4 Story 4.5 SkinsProvider 桥接).

**Acceptance Criteria:**

**Given** Epic 6 Story 6.1 的 user_skins collection + Epic 1 JWT + Story 0.9 WS dispatcher
**When** 实现 `internal/ws/handlers/skin_handlers.go::HandleSkinsOwned` + 扩展 `skin_repo` + 更新 `initialize.go` 接入 SkinsProvider
**Then** 客户端 WS envelope `{id, type: "skins.owned", payload: {}}`（PRD WS Message Type Registry 未单独列此 type，本 story 作为 `skins.catalog` 的补充追加注册）
**And** **不走** eventId dedup（读操作）
**And** 查询 `user_skin_repo.ListByUser(ctx, userID)` → 返回 `[{skinId, unlockedAt}]`
**And** 对每条 join `skin_repo`（进程内 catalog 缓存）获取 `name, rarity, assetPath`
**And** 默认猫（`default_cat`）始终出现在结果中（即使 user_skins 无记录 —— 所有用户天生拥有默认猫）
**And** 响应 envelope `{id, ok: true, type: "skins.owned.result", payload: {skins: [{id, name, rarity, assetPath, unlockedAt}, ...]}}`
**And** 按 `unlockedAt DESC` 排序（最新获得在前）
**And** 响应时间 p95 ≤ 200ms
**And** **session.resume SkinsProvider 桥接**：
  - 实现 `SkinsProvider` interface（Epic 4 Story 4.5 定义的消费方接口）
  - 实现类 `RealSkinsProvider{userSkinRepo, skinRepo}` 暴露 `ListUnlocked(ctx, userID) ([]SkinInfo, error)`
  - 更新 `initialize.go`：替换 `EmptySkinsProvider` 为 `RealSkinsProvider`（Epic 4 Story 4.5 之后 session.resume payload 中的 `skins` 字段自动从空数组变为真实数据）
**And** 单元测试：空列表（仅默认猫）/ 有 3 个解锁 + 默认猫 / join skin_repo 正确 / 排序
**And** 集成测试（Testcontainers）：seed skins + 插入 user_skins → 查询返回 + session.resume 验证 skins 字段非空

### Story 7.3: 皮肤装配 + 好友可见

As a signed-in user who unlocked a new skin,
I want to equip it on my cat and have friends see the new skin in their room via the existing state broadcast,
So that collecting skins has visible social value — friends notice when I've got something new (FR38, FR39, NFR-PERF-1 via Epic 4 broadcast).

**Acceptance Criteria:**

**Given** Story 7.2 + Epic 2 cat_state_repo + Epic 4 RoomStateBroadcaster + Story 0.10 eventId dedup + Story 0.12 ResumeCacheInvalidator
**When** 实现 `internal/service/skin_service.go` + `internal/ws/handlers/skin_handlers.go::HandleSkinEquip`
**Then** 客户端 WS envelope `{id, type: "skin.equip", payload: {skinId: string}}`
**And** dispatcher 走 eventId dedup（Story 0.10，避免重复装配触发多次广播）
**And** validator：`skinId` 非空 + 有效 SkinID 格式
**And** `SkinService.Equip(ctx, userID, skinID)` 流程：
  1. 校验用户拥有该皮肤：`user_skin_repo.Exists(userID, skinID)` —— 未拥有 → `SKIN_NOT_OWNED`（Category: client_error，FR38）
  2. 特例：`skinId == "default_cat"` 始终允许（所有用户天生拥有）
  3. 更新 `cat_state_repo.UpdateSkinID(ctx, userID, skinID)` → Mongo 更新 `cat_states.skinId` 字段 + Redis write-through `state:{userId}` Hash 更新 `skinId`
  4. 调用 `stateBroadcaster.OnStateChange(ctx, userID, updatedSnapshot)` —— 这会触发 Epic 4 Story 4.3 的 `RoomStateBroadcaster` 对好友广播 `friend.state`，payload 含新 `skinId`（FR39 自然实现：装配后下一次 `friend.state` 广播自动携带新 skinId）
  5. 失效 `resume_cache:{userId}`（Story 0.12）
**And** 响应 envelope `{id, ok: true, type: "skin.equip.result", payload: {skinId, equipped: true}}`
**And** FR39 "好友看到装配的皮肤" 实现路径：
  - `cat_states.skinId` 已在 Epic 2 Story 2.2 的 `CatStateSnapshot` 值对象中
  - Epic 4 Story 4.3 的 `RoomStateBroadcaster.OnStateChange` 已在 `friend.state` payload 中包含 `skinId`
  - Epic 4 Story 4.2 的 `room.snapshot` 已包含 `skinId`
  - 因此 FR39 无需额外实现 —— skin.equip 更新 skinId 后，好友端自动通过现有广播路径看到新皮肤
**And** 若 Epic 4 尚未完成：skin.equip 仍可独立工作（自己可见、skinId 写入 cat_states），好友端在 Epic 4 完成后自动点亮（FR39）
**And** 响应时间 p95 ≤ 200ms（NFR-PERF-4）
**And** zerolog 审计：`{action: "skin_equip", userId, skinId, previousSkinId}`（NFR-SEC-10）
**And** 单元测试：
  - 拥有皮肤装配成功 + cat_states.skinId 更新 + Redis write-through + broadcaster.OnStateChange 调用 + resume cache 失效
  - 未拥有 → `SKIN_NOT_OWNED`
  - `default_cat` 始终允许（无需 user_skins 记录）
  - 重复装配同一皮肤（幂等，返回 ok）
  - eventId dedup 验证
**And** 集成测试（Testcontainers + 真实 WS）：
  - 端到端 equip → cat_states.skinId = 新值 → 另一 WS 客户端收到 friend.state 含新 skinId（FR39 端到端验证）
  - `SKIN_NOT_OWNED` 场景

---

## Epic 8: 冷启动召回 🟡 Optional / Deferred

**⚠️ 待评估是否实现。不阻塞任何其他 Epic。** 服务端通过 cron（24h 周期）识别注册 ≥ 48h 且好友数 = 0 的冷启动用户，通过 APNs 召回推送引导其生成邀请链接，缓解 J3 冷启动旅程的流失漏斗。评估维度：MVP 30 天 D3 自然留存是否健康、推送骚扰对卸载率的反向影响、NFR-COMP-3 APNs Guidelines 合规。

### Story 8.1: 冷启动用户识别 cron

As a product analyst,
I want the server to automatically identify users who registered ≥ 48 hours ago and still have zero friends,
So that we can target the J3 cold-start cohort — the users most likely to churn because they never reached their "aha moment" (seeing a friend's cat alive) (FR44a, PRD §User Journeys J3).

**Acceptance Criteria:**

**Given** Epic 0（cron + Lock + Clock + logx）+ Epic 1（users collection with `friendCount` and `createdAt` fields）
**When** 实现 `internal/cron/cold_start_recall_job.go` + `internal/service/recall_service.go`
**Then** cron `cold_start_recall` 每 24 小时运行一次（NFR-PERF-7 冷启动检测周期 24h）；走 `redisx.Locker.WithLock("cold_start_recall", 3500s, fn)`（D5, FR56）
**And** 识别条件（FR44a）：
  ```
  users WHERE friendCount == 0
         AND createdAt < Clock.Now - 48h
         AND deletionRequested == false
  ```
**And** 额外过滤：
  - 排除已注销标记的用户（`deletionRequested == true`）
  - 排除 30 天内未登录的 churned 用户（`lastLoginAt < Clock.Now - 30d`，如有此字段；否则用 `createdAt < Clock.Now - 30d` 作为代理）
**And** `RecallService.IdentifyColdStartUsers(ctx) ([]ids.UserID, error)` 方法封装查询逻辑
**And** 查询使用 Mongo 聚合（`$match` + `$project`）；结果批量返回（不逐条查询）
**And** 结果暂存到 Service 内存 slice（不持久化中间状态 —— MVP 简化），直接传给 Story 8.2 的推送逻辑
**And** zerolog info：`{action: "cold_start_scan", identifiedCount, scannedCount, durationMs}`
**And** Clock 注入确定性测试（FakeClock 跨 48h 边界）
**And** 单元测试 table-driven：
  - 注册 47h（不识别）/ 48h（识别）/ 72h（识别）
  - friendCount == 0（识别）/ friendCount == 1（不识别）
  - deletionRequested == true（排除）
  - 30d 未登录（排除）
  - 混合场景 1000 用户，仅 N 个符合条件
**And** 集成测试（Testcontainers + FakeClock）：
  - 插入 5 个用户（3 个冷启动 + 1 个有好友 + 1 个已注销）→ cron → 识别正好 3 个

### Story 8.2: 召回推送发送 + 节流

As a cold-start user who registered 3 days ago but never added a friend,
I want to receive a gentle APNs push with a deep link to the invite flow,
So that I'm reminded the app has a social core I haven't tried yet — without being spammed if I choose to ignore it (FR44b, NFR-COMP-3 APNs Guidelines, PRD §Domain-Specific).

**Acceptance Criteria:**

**Given** Story 8.1 识别出的冷启动用户列表 + Epic 0 Pusher + Epic 1 users.preferences.quietHours / timezone + Clock
**When** 扩展 `RecallService` 加入 `SendRecallPush(ctx, userIDs []ids.UserID)` 方法
**Then** 对每个冷启动用户执行：
  1. **节流检查**：`recall:sent:{userId}` Redis key 存在 → 跳过（7 天内已推送过）
  2. 通过节流：调 `pusher.Enqueue(ctx, userID, PushPayload{...})`
  3. 推送 payload：
     - `Kind: alert`（非 silent —— 召回需要用户注意，但受免打扰约束）
     - `Title: "裤衩猫"`
     - `Body: "邀请一个朋友，一起看对方的猫在干嘛 🐱"`（MVP 中文固定文案，OP-7 i18n 延后）
     - `DeepLink: "cat://invite"`（客户端解析后跳到"生成邀请 token"页面）
     - `RespectsQuietHours: true`（Epic 0 Pusher 内部处理免打扰降级 silent）
     - `IdempotencyKey: "recall_<userId>_<dateYYYYMMDD>"`（每天同用户最多一条，Pusher 内部 NX 去重）
  4. 推送成功入队后：`SET recall:sent:{userId} "1" EX 604800`（7 天 TTL）—— 7 天内不再推送该用户
  5. zerolog info：`{action: "cold_start_recall_sent", userId}`
**And** 节流规则：同一用户 7 天内最多收到 1 次冷启动召回推送（`recall:sent:{userId}` TTL 7 天）
**And** 节流 TTL 从 `cfg.Recall.CooldownDays`（TOML 配置，默认 7 天）读取，重启生效
**And** 批量推送性能：1000 个冷启动用户场景 → 并发 Enqueue（`errgroup` MaxConcurrency=10）→ 单次 cron 完成 < 30s
**And** 失败隔离：某用户 Enqueue 失败（如 APNs token 不存在）不影响其他用户；zerolog warn 记录 `{action: "cold_start_recall_failed", userId, err}`
**And** NFR-COMP-3 APNs Guidelines 合规：
  - 推送非垃圾：仅针对确实 48h+ 零好友的用户
  - 尊重免打扰：`RespectsQuietHours: true`
  - 用户可拒绝：客户端可关闭推送通知（OS 层控制；服务端不强推）
  - 频率受控：7 天 cooldown
**And** cron 流程总览（Story 8.1 + 8.2 串联）：
  ```
  cold_start_recall cron (每 24h, withLock):
    1. IdentifyColdStartUsers → [userIDs]
    2. SendRecallPush(userIDs)
       for each userID:
         a. recall:sent:{userId} exists? → skip
         b. pusher.Enqueue(recallPayload)
         c. SET recall:sent:{userId} EX 7d
    3. zerolog summary: {identified, sent, skipped_cooldown, failed}
  ```
**And** 单元测试：
  - 首次推送成功 + Redis cooldown 写入
  - 7 天内第二次跳过（cooldown 生效）
  - 7 天后 cooldown 过期 → 再次推送
  - Pusher.Enqueue 被调用且 payload 正确（Title / Body / DeepLink / RespectsQuietHours / IdempotencyKey）
  - 批量 1000 用户 + MaxConcurrency=10 并发控制
  - Enqueue 失败隔离（不影响后续用户）
**And** 集成测试（Testcontainers + mock APNs）：
  - 端到端 cron → 识别 → 推送入队 → Redis cooldown 写入
  - cooldown 7 天 → 第二次 cron → 跳过
