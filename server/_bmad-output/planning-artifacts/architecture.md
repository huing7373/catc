---
stepsCompleted:
  - step-01-init
  - step-02-context
  - step-03-starter
  - step-04-decisions
  - step-05-patterns
  - step-06-structure
  - step-07-validation
  - step-08-complete
status: complete
completedAt: 2026-04-16
inputDocuments:
  - C:/fork/cat/server/_bmad-output/planning-artifacts/prd.md
  - C:/fork/cat/docs/backend-architecture-guide.md
  - C:/fork/cat/裤衩猫.md
  - C:/fork/cat/README.md
  - C:/fork/cat/document/联机同步设计稿.md
  - C:/fork/cat/document/Spine到AppleWatch资源导出方案.md
  - C:/fork/cat/server/_bmad-output/planning-artifacts/implementation-readiness-report-2026-04-16.md
workflowType: architecture
project_name: server
user_name: 开发者
date: 2026-04-16
---

# Architecture Decision Document

_This document builds collaboratively through step-by-step discovery. Sections are appended as we work through each architectural decision together._

## Project Context Analysis

### Requirements Overview

**8 子系统**：状态镜像、房间与实时在场、触碰中继、盲盒权属、好友图、账户、皮肤、平台基础
**6 横切关注点**：鉴权、限流、幂等去重、分布式锁、结构化日志、APNs 推送路由

**Primary domain**：实时社交后端（HTTP bootstrap + WebSocket 主通道 + APNs 后台唤醒）
**Complexity**：medium-high（7 项驱动因素：multi_device_sync、websocket_framework、apns_tiered_push、fsm、anti_cheat、snapshot_decay_engine、session_resume）

### Technical Constraints（从架构宪法）

- Go 1.25+，单体，无微服务
- 显式依赖注入（无 DI 框架），`initialize.go` 唯一装配点
- HTTP Gin + WebSocket gorilla；MongoDB + Redis；zerolog；TOML
- 接口消费方定义（"accept interfaces, return structs"）；repository 是 DB 唯一入口
- AppError（code + message + httpStatus）+ sentinel error + `errors.Is/As`
- Context 贯穿所有 I/O；禁止 `context.TODO()`

**外部依赖：**
- Apple APNs（HTTP/2 Provider API）
- Apple `appleid.apple.com/auth/keys`（JWK 动态拉取）
- MongoDB 4.0+（事务依赖）
- Redis 6+（SETNX / TTL / Hash / Stream）
- CDN（皮肤资源；不在服务端范围）

### 关键 NFR 架构驱动

- 房间广播 p99 ≤ 3s、`session.resume` p95 ≤ 500ms、WS RPC 往返 p95 ≤ 200ms
- 触碰送达率 ≥ 99%（WS + APNs 联合）
- 盲盒零重复领取（一票否决）
- WS 重连 5s 内成功率 ≥ 98%
- 服务可用性月度 ≥ 99.5%
- 代码无进程级全局状态；单实例 WS 并发 ≤ 10,000

### Multi-Replica Invariants（Code-Level）

代码按多副本假设设计 —— **code-level multi-replica，not deployment multi-replica**。MVP 部署单实例，但代码必须满足 5 条不变量：

1. 无 `sync.Map` / 全局变量 / singleton 存连接或状态
2. 所有共享状态在 Redis（`presence:*` / `ratelimit:*` / `event:*` / `blacklist:*`）
3. Cron 必须有分布式锁（Redis SETNX + TTL）
4. WS 广播不能假设目标连接在本实例（需 Pub/Sub fan-out 机制或 sticky routing 预留）
5. APNs 发送必须有持久化队列（非 goroutine chan）

**验证**：CI 跑 2 副本 docker-compose 对关键路径（cron 去重、presence 多副本一致、广播 fan-out、APNs 队列去重）。

### Hidden Assumptions — 需显式化为独立 ADR

以下 3 条在 PRD 和早期讨论中被当作事实陈述，实际是决策，需后续独立 ADR 分析：

- **ADR-001**：服务端单方面衰减引擎 vs 客户端本地衰减 + 服务端仅做超时判定（权衡：服务端压力 vs 客户端复杂度）
- **ADR-002**：per-device WS 多路复用的 presence key 结构（`presence:{userId}:{deviceId}` 单键 vs `presence:{userId}` Set of deviceId）
- **ADR-003**：WS hub 单实例 goroutine 上限（PRD 假设 10k，需 Spike 压测基准为依据）

### 额外架构关切（10 条）

| # | 关切 | 归属 |
|---|---|---|
| 1 | 事务边界规则（Mongo 事务 vs Redis 幂等） | Step 4 或独立 ADR |
| 2 | 数据保留策略（TTL / 归档 / 注销后留存） | Step 4 |
| 3 | 时间戳权威源 + 时钟同步（客户端上报 vs 服务端接收；多副本 NTP） | Step 4 |
| 4 | 配置变更生效策略（静态重启 / 动态热更 / 分层） | Step 4 |
| 5 | 测试环境架构（Testcontainers；禁用 DB mock） | Step 6 Structure |
| 6 | 可观测性栈拓扑（log 去向、metrics 是否有、告警渠道） | Step 4 |
| 7 | Feature flag 策略（Growth/Vision 代码分层 vs 运行时 flag） | Step 4 |
| 8 | 部署升级 + 兼容性策略（蓝绿 / 金丝雀 / 版本协商） | Step 7 Validation |
| 9 | 错误分类 + 客户端期望行为矩阵（retry / fail / silent） | Step 5 Patterns |
| 10 | 事件驱动 vs cron（worker queue 需求） | Step 4 |

### Cross-Cutting Concerns（最终列表 6+）

1. **鉴权中间件（JWT verify）** —— HTTP Gin 中间件 + WS upgrade hook
2. **限流中间件 + 服务** —— Redis Counter；多维度（per-user WS connect / per-pair touch / per-user invite / per-IP HTTP）
3. **幂等去重服务** —— Redis SETNX eventId；WS 上行 + 推送出站
4. **分布式锁服务** —— Redis SETNX + TTL；cron 单一触发、异常设备标记
5. **结构化日志（zerolog）+ 请求关联 ID** —— HTTP `requestId` / WS `connId + eventId`
6. **APNs 推送路由** —— platform 区分 + 静默/普通分层 + 失败重试 + token 清理 + 持久化队列

### Open Items —— 从 3 扩充到 10

**T1 — Step 4 必决（不决策无法写代码骨架）：**
- WS hub 结构（单进程内 hub goroutine vs Redis Pub/Sub fan-out）
- **OP-2**：状态冲突解决（last-write-wins vs per-source priority）
- **OP-5**：APNs 推送队列化（Redis Stream / Mongo capped / 外部 MQ）
- **OP-6**：Mongo Change Streams 是否使用
- cron 分布式锁策略（`robfig/cron` + Redis SETNX vs 外部 asynq）
- **OP-1 design space exploration**：候选方向对 hub 结构的约束分析（提前到 Step 4，Step 5 不再盲决）

**T2 — Step 5 Patterns 必决：**
- **OP-1 final**：watchOS WS-primary 稳定性方案（Spike 收敛，禁止 HTTP backup）
- **OP-3**：WS 广播 fan-out 策略（写 Mongo 前/后 trigger broadcast；Change Streams vs 手动）
- **OP-4**：`session.resume` 增量协议 shape（全量快照 vs 带 lastSeq 增量）

**T3 — 可延后但架构文档需留 hook：**
- **OP-7**：i18n 时序（错误 message 国际化、APNs 推送文案 i18n）
- 配置热更新实现（见关切 #4）
- 灰度升级策略（见关切 #8）

### 项目特殊约束

- 客户端 Apple Watch + iPhone 双端独立登录（per-device WS 多路复用）
- watchOS 后台 WS 不可假设存活 → 服务端衰减引擎单方面运行（待 ADR-001 确认）
- iPhone HealthKit 30s 后台窗口 → HTTP `/state` 必须单次完成、不走 WS
- 美术和真机调试是瓶颈（非代码量）；Claude 承担 99.99% 编码
- MVP 目标 6 个月上线；Spike-OP1 前置 E4（WS 房间）开发

## Walking Skeleton

### Primary Technology Domain

Go 1.25+ 后端 API + WebSocket 服务，按架构宪法（`docs/backend-architecture-guide.md`）约束。

### Decision: No External Starter; Manual Walking Skeleton

**Rationale：**
- 架构宪法已钉死目录结构、初始化、Runnable 接口、错误与日志规范
- Go 生态无符合宪法的公共 starter；任何外部 starter 都会与宪法冲突或冗余
- 1-2 天走出 walking skeleton，覆盖从 HTTP/WS 入口 → Service → Repository → Mongo/Redis 的全栈最薄路径
- 6 个月 MVP 下 ≤ 1% 时间预算，回报比极高

### Alternatives Rejected

| 方案 | 拒绝理由 |
|---|---|
| `gonew` + 模板 repo | 无符合宪法的公共模板 |
| `golang-standards/project-layout` | 仅文档；与宪法目录有出入 |
| 第三方 Go 后端模板 | 陈旧或选型冲突 |
| go-kit / go-micro | 微服务框架，违反宪法"单体 no microservices" |

### Epic 0 Walking Skeleton 拆解（4 个 Stories）

| Story | 交付物 | 验收 |
|---|---|---|
| **S0.1 Repo 骨架** | go.mod + 目录结构 + 全部配置文件 | `bash scripts/build.sh` 过；`go vet ./...` 过 |
| **S0.2 最小运行时** | `cmd/cat/{main.go, initialize.go, app.go}` + Runnable + `/healthz` | `./catserver` 起来；`curl /healthz` 返回 JSON；SIGTERM 30s 内优雅停机 |
| **S0.3 DB 连通性** | `pkg/{mongox, redisx}` 连接 + ping + Testcontainers 集成测试 | `bash scripts/build.sh --test` 过；`/healthz` 正确反映 Mongo/Redis 状态 |
| **S0.4 WS 骨架** | `internal/ws` 最小 Hub + `/ws` upgrade + echo + 优雅关闭 | WS 建连 → 发消息 → 收到 echo；SIGTERM 时连接按规范关闭 |

### Initialization Artifacts（S0.1 交付）

**目录结构：** 按宪法 §3 创建

```
server/
├── cmd/cat/
├── internal/{config,domain,service,repository,handler,dto,middleware,ws,cron,push}
├── pkg/{logx,mongox,redisx,jwtx,ids,fsm}
├── config/{default.toml, local.toml}
├── tools/
├── scripts/ (build.sh 已存在)
├── deploy/{Dockerfile, docker-compose.yml}
└── go.mod
```

**Go modules + 核心依赖：**

```bash
go mod init github.com/<owner>/cat/server

# 宪法强制依赖
go get github.com/gin-gonic/gin
go get github.com/gorilla/websocket
go get go.mongodb.org/mongo-driver/v2/mongo
go get github.com/redis/go-redis/v9
go get github.com/BurntSushi/toml
go get github.com/rs/zerolog
go get github.com/golang-jwt/jwt/v5
go get github.com/go-playground/validator/v10
go get github.com/sideshow/apns2
go get github.com/robfig/cron/v3
go get github.com/google/uuid
go get github.com/stretchr/testify

# Testcontainers（集成测试）
go get github.com/testcontainers/testcontainers-go
```

**开发工具链配置文件：**

| 文件 | 作用 |
|---|---|
| `.golangci.yml` | lint 规则（enable: gofmt/goimports/govet/errcheck/unused/ineffassign；disable: lll） |
| `.editorconfig` | tab 缩进（Go 强制） |
| `.gitignore` | `build/`、`*.out`、`.env.local`、IDE 目录 |
| `.dockerignore` | `.git`、`build/`、`*.md`、`*_test.go` |
| `Makefile` | `build` / `test` / `lint` / `docker-up` / `docker-down` |
| `.github/workflows/ci.yml` | lint + test（含 race）+ build，on push/PR |
| Pre-commit hook（lefthook 或手写） | gofmt + goimports + 快速测试 |
| `config/default.toml` | 默认配置骨架 |
| `config/local.toml` | 本地开发覆盖（不提交） |
| `deploy/docker-compose.yml` | Mongo **replica set**（1 节点，为 Change Streams 预留）+ Redis + app |

### Baseline 代码文件清单

**S0.1 / S0.2 交付：**

| # | 文件 | 核心契约 |
|---|---|---|
| 1 | `pkg/logx/logx.go` | `Init(cfg)` + `Ctx(ctx) *zerolog.Logger` + `WithRequestID(ctx, id)` |
| 2 | `internal/dto/error.go` | `AppError{Code, Message, HTTPStatus, Cause}` + 常用构造器 + Gin middleware |
| 3 | `pkg/ids/ids.go` | typed IDs（UserID/SkinID/BlindboxID/FriendID/InviteTokenID）+ `New()` / `Parse()` |
| 4 | `cmd/cat/app.go` | `Runnable` 接口 + `App` 容器 + signal handling + 30s graceful shutdown |
| 5 | `internal/config/config.go` | `MustLoad(path)` via BurntSushi/toml，支持 default + override 合并 |
| 6 | `internal/middleware/request_id.go` | 从 `X-Request-ID` 读取或生成；注入 context + 响应 header |
| 7 | `internal/middleware/recover.go` | panic recovery → zerolog → AppError 500 |
| 8 | `internal/middleware/logger.go` | 结构化访问日志（method/path/status/duration/userId） |
| 9 | `internal/handler/health_handler.go` | `GET /healthz` + `GET /readyz` |

**S0.3 交付：**

| # | 文件 | 核心契约 |
|---|---|---|
| 10 | `pkg/mongox/client.go` | `MustConnect(cfg)` + ping + `HealthCheck(ctx)` |
| 11 | `pkg/redisx/client.go` | `MustConnect(cfg)` + ping + `HealthCheck(ctx)` |
| 12 | `internal/testutil/testutil.go` | Testcontainers Mongo/Redis 启动辅助；`SetupMongo(t)` / `SetupRedis(t)` |
| 13 | `internal/repository/sample_repo_test.go` | 示范集成测试 seed（用 Testcontainers，**不 mock DB**） |

**S0.4 交付：**

| # | 文件 | 核心契约 |
|---|---|---|
| 14 | `internal/ws/hub.go` | Hub 最小骨架 + connection registry（Redis-backed） |
| 15 | `internal/ws/upgrade_handler.go` | `/ws` endpoint + JWT 校验 + upgrade + echo message |
| 16 | `internal/ws/envelope.go` | envelope 类型 `{id, type, payload, ok, error}` + JSON 序列化 |

### Cookie-Cutter Generator —— 不做（YAGNI）

当前只有一个项目。生成器是"为重用"的投资；没重用就是负债。

**替代方案**：写一份 `docs/skeleton-spec.md` 描述"骨架包含什么、为什么"，充当设计意图档案；未来有第二个 Go 后端时再抽 template repo（或 tag 本 commit 为 `v0.0.0-skeleton`）。

### Starter 决策一句话总结

**不用外部 starter；由架构宪法驱动，Epic 0 用 4 个 stories 交付 walking skeleton（~15 个 Go 文件 + 完整工具链 + Testcontainers 集成测试种子）。**

**Note：** 项目初始化和 walking skeleton 是 Epic 0（服务端骨架与平台基线）的交付物，是后续所有 Epic 的前置条件。

## Core Architectural Decisions

### Decision Priority Analysis

**Critical（阻塞实现）：** D1 WS Hub 结构、D2 状态冲突、D3 APNs 队列、D4 Change Streams、D5 Cron 锁、D6 OP-1 hub 接口预留
**Important（塑造架构）：** D7 ADR-001 衰减、D8 ADR-002 presence、D9 ADR-003 goroutine 上限、D10 事务边界、D11 保留、D12 时间戳、D13 配置、D14 观测、D15 flag、D16 事件驱动
**Deferred（Post-MVP / Step 5 Patterns）：** OP-3 fan-out 模式、OP-4 session.resume 增量 shape、OP-7 i18n、部署升级、Prometheus、OpenTelemetry

### D1. WebSocket Hub 结构

**决策：** MVP 单进程内存 hub + 抽象 `Broadcaster` 接口（为 Phase 3 Redis Pub/Sub 预留替换点）

```go
type Broadcaster interface {
    BroadcastToUser(ctx context.Context, userID UserID, msg []byte) error
    BroadcastToRoom(ctx context.Context, roomID RoomID, msg []byte) error
    PushOnConnect(ctx context.Context, connID ConnID, userID UserID) error  // D6 预留
    BroadcastDiff(ctx context.Context, userID UserID, diff []byte) error    // D6 预留
}
// MVP: InMemoryBroadcaster（本进程 broadcast）
// Phase 3: RedisPubSubBroadcaster（跨实例 fan-out）
```

**影响：** `internal/ws/` 目录；`Service` 层只依赖 `Broadcaster` 接口。

### D2. 状态冲突解决（OP-2）

**决策：** Per-source 优先级 + 同 source 内 LWW

**优先级（高→低）：**
1. `watch_foreground`
2. `iphone_foreground`
3. `iphone_background_healthkit`
4. `server_inference`

**规则：** 新上报仅在 `priority(new) ≥ priority(current)` 时覆盖 `cat_states`；`priority(new) > priority(current)` 立即覆盖；同优先级 last-write-wins。

**影响：** `cat_states` 文档新增 `source` 字段；`StateService.Upload` 实现优先级比较。

### D3. APNs 推送队列化（OP-5）

**决策：** Redis Streams（`apns:queue`）+ worker goroutine 消费组

- 发推：`XADD apns:queue * userId <id> type <alert|silent> payload <json>`
- 消费：`XREADGROUP` with `consumer_group=apns_workers, consumer=<instanceID>`
- 成功：`XACK`；失败指数退避 3 次；终态失败 → `XADD apns:dlq` + log
- 多副本就绪：consumer group 自动分片

**影响：** `internal/push/` 目录；独立 `APNsWorker` Runnable。

### D4. Mongo Change Streams（OP-6）

**决策：** MVP 不使用；Service 层手动触发广播

**路径：**
```
WS state.tick → Service.UpdateCatState()
  → Repository.Save() (Mongo + Redis write-through)
  → Broadcaster.BroadcastToRoom()（service 显式调用）
```

**保留条件：** `docker-compose.yml` 配 1 节点 replica set（为未来 Phase 3 评估 Change Streams 或 Mongo 事务留门）。

**影响：** Service 层显式依赖 Broadcaster。

### D5. Cron 分布式锁

**决策：** `robfig/cron/v3` + Redis SETNX 锁 + 55s TTL

```go
func withLock(r *redis.Client, name string, fn func()) cron.Job {
    return cron.FuncJob(func() {
        ok, _ := r.SetNX(ctx, "lock:cron:"+name, instanceID, 55*time.Second).Result()
        if !ok { return }
        defer r.Del(ctx, "lock:cron:"+name)
        fn()
    })
}
```

**影响：** `internal/cron/` 所有 job 包裹 `withLock`；CI 测试 2 副本场景验证单次触发。

### D6. OP-1 Design Space Exploration（Hub 接口反向约束）

**候选方向与约束：**

| # | 候选方向 | Hub 接口约束 |
|---|---|---|
| 1 | 客户端 cache-first + 差分更新协议 | 需 `BroadcastDiff`；维护 `lastKnownSeq` |
| 2 | 服务端主动 session.resume 推送 | 需 `PushOnConnect` |
| 3 | 精细化抬腕事件重连（客户端驱动） | Hub 无变化 |
| 4 | WS permessage-deflate 压缩 | Hub 配置压缩扩展 |
| 5 | `NWConnection` 深度适配 | Hub 无变化 |

**Step 4 决策：** Hub 接口纳入 `PushOnConnect` + `BroadcastDiff`（方向 #1, #2 预留）。

**Final 决策延至 Step 5 / Spike-OP1 后。**

### D7. ADR-001 状态衰减实现

**决策：** 服务端单方面衰减引擎

- Cron `state_decay` 每 30 秒扫描 Redis 热缓存 `state:*`
- 按四档处理：0-15s 真实 / 15-60s 广播打 `stale: weak` 标记 / 1-5min 推断 idle → `state.serverPatch` 广播 / >5min 标记 `offline` → `friend.offline` 广播

**拒绝方案：** 客户端本地衰减（watchOS 后台无法保证执行；多好友广播一致性差）

**影响：** `internal/cron/state_decay.go`；`StateService.Decay(ctx)` 方法。

### D8. ADR-002 Presence Key 结构

**决策：** `presence:{userId}` Redis Set，成员格式 `{deviceId}:{connId}:{instanceId}`

- `SADD presence:u_123 watch-xxx:conn-aaa:inst-01`
- 心跳每 30s 用 `presence:{userId}:meta` Hash 记录每成员的 `lastPing`
- 过期清理 cron：扫描 meta Hash，移除 60s 未心跳的成员

**拒绝方案：** `presence:{userId}:{deviceId}` 单键 —— 同设备多 WS 并发时覆盖

**影响：** `pkg/redisx/presence.go`；fan-out 广播解析 Set 成员按 instanceId 决定本地/远程。

### D9. ADR-003 WS Hub Goroutine 上限

**决策：** 目标 10,000；Spike-OP1 期间真机压测验证

**压测方案：**
- 环境：MacBook M1 / 8G 或类似
- 变量：并发连接数 N ∈ {1k, 3k, 5k, 10k}
- 指标：建连成功率、广播延迟 p95/p99、CPU%/MEM
- 决策触发：若 10k 下 p99 > 3s → 调低上限 + 启动 Phase 3 sticky routing 规划

**影响：** `ws.Hub` 初始化配置暴露 `MaxConnections`；Spike 报告归档到 `docs/spikes/op1-ws-stability.md`。

### D10. 事务边界规则

**决策：** 幂等默认，事务仅在必要时

| 场景 | 策略 |
|---|---|
| 盲盒领取（FR33） | **Mongo 事务** —— 盲盒状态检查 + reward 写 + 点数更新 + user_skins 写 四 collection 原子 |
| 好友建立（FR14） | **Mongo 事务** —— 双向 `friends` 文档 + friendCount 原子 |
| 账户注销（FR47） | MVP 仅标记 `deletion_requested`，无事务；Growth 级联时用事务 |
| 状态上报（FR7） | 单文档 upsert + Redis write-through，无事务 |
| 触碰上报（FR26） | 异步 `touch_logs` 写，无事务 |
| 其他单 collection 写 | 无事务；Redis SETNX `event:{eventId}` 5min 去重（FR57） |

**影响：** `pkg/mongox/` 提供事务辅助 `WithTransaction(ctx, fn)`。

### D11. 数据保留策略

**决策：**

| 数据 | 保留时长 | 机制 |
|---|---|---|
| `cat_states` | 永久（账号未注销） | 无 TTL |
| `touch_logs` | 90 天 | Mongo TTL index `createdAt` |
| `invite_tokens` | 24 小时 | Mongo TTL index `expiresAt` |
| `event:{eventId}` Redis | 5 分钟 | Redis EXPIRE |
| `presence:{userId}` 成员 | 60s 心跳续期 | Redis + cron 清理 |
| 账户注销数据 | MVP 保留标记；Growth 30 天级联清理 | 标记 + cron |

### D12. 时间戳权威源

**决策：** 服务端 `updatedAt` 权威；客户端 `clientUpdatedAt` 仅做冲突检测输入

- Service 接收上报时记 `updatedAt = time.Now()` UTC
- 客户端可选附带 `clientUpdatedAt`；仅参与 D2 优先级比较
- 多副本 NTP 同步由部署层保障（chrony / systemd-timesyncd）

### D13. 配置变更生效策略

**决策：** 分层

| 参数类别 | 存储 | 生效 |
|---|---|---|
| 基础设施（DB URL / 端口 / 密钥） | TOML + 环境变量覆盖 | 重启生效 |
| 业务规则阈值（触碰限流、衰减档位、盲盒步数要求） | Redis Hash `config:runtime` | 动态生效（Service 每次读） |
| Feature flags | Redis Hash `config:features` | 动态生效 |

**影响：** `internal/config/` 同时读 TOML + Redis；运行时参数变更由运维直接修改 Redis。

### D14. 可观测性栈拓扑（MVP 最小）

**决策：**
- **日志**：zerolog JSON → stdout → Docker log driver → 本地文件滚动；Phase 2 可接入 Loki
- **Metrics**：无（架构代码预留 `/metrics` hook，Phase 3 引入 Prometheus）
- **告警**：Uptime Robot `/healthz` + 邮件/SMS
- **分布式追踪**：无（Phase 3 引入 OpenTelemetry）

### D15. Feature Flag 策略

**决策：** 双层
1. **代码分层**：Growth / Vision 功能代码不在 MVP 交付（物理隔离）
2. **有限运行时 flag**：Redis `config:features`（仅用于 MVP 内可调参数）

**拒绝**：外部 flag 服务（LaunchDarkly / flipt）—— YAGNI。

### D16. 事件驱动 vs Cron

**决策：** MVP 明确只用 cron；异步事件复用 Redis Streams（D3 基础设施）

**MVP 事件驱动任务场景：**
- APNs 推送 → `apns:queue`
- 好友接受欢迎推送 → 直接 APNs
- 账户注销级联 → Growth 再引入

**拒绝**：专用 task queue（asynq / machinery / NATS）—— 一致性优先，不引入新组件。

### Decision Impact Analysis

**实现依赖顺序：**
1. D13（config 分层） → D14（可观测性）
2. D1（Broadcaster 接口）+ D6（hub 接口扩展）→ D4（service 手动广播）
3. D8（presence key）→ D1（hub 消费 presence）
4. D5（cron 锁）→ D7（衰减引擎）+ D16（事件驱动基础设施）
5. D3（APNs 队列）→ D16（复用 Redis Streams）
6. D10（事务边界）→ D2（状态冲突处理）
7. D12（时间戳权威）→ D2 + D7

**跨组件依赖：**
- Hub（D1）⟷ Presence（D8）⟷ APNs 路由（D3）形成"触碰送达"核心链路
- 衰减引擎（D7）⟷ 状态冲突（D2）⟷ 时间戳（D12）形成"状态真实性"核心链路
- 事务（D10）⟷ 幂等（D16 via Redis）⟷ 盲盒确权（FR33）形成"权威写"核心链路

**Spike-OP1 关键依赖：** D6 预留接口 + D9 上限 + D7 衰减 —— 三者在 Spike 期间联合验证。

## Implementation Patterns & Consistency Rules

> **强度 tag**：`[compiler]` 编译器强制 / `[lint]` Lint 强制 / `[test]` 测试验证 / `[convention]` 约定（依赖 review + Claude 文档输入）

### P1 · MongoDB 规范

- **Collection 命名** `[convention]`：`snake_case` 复数（`users` / `cat_states` / `touch_logs` / `invite_tokens`）
- **BSON 字段命名** `[convention]`：`snake_case`（`user_id`、`updated_at`、`cat_state`）
- **Index 命名** `[convention]`：手动 index 用 `{field}_{order}` 格式（`user_id_1`、`created_at_-1`）
- **主键策略**：聚合根用业务 ID 作 `_id`（如 `UserID` 字符串）；关联表用 Mongo 自动 `ObjectId`

### P2 · HTTP API 格式

- **路径命名** `[convention]`：复数 kebab-case（`/auth/apple`、`/devices/apns-token`）
- **成功响应** `[convention]`：直接返回 payload，**无 wrapper**
- **错误响应** `[compiler]`：`{ "code": "...", "message": "..." }` + 合适 HTTP status
- **时间格式** `[test]`：RFC 3339 UTC（`"2026-04-16T13:00:00Z"`）
- **分页（修订）**`[convention]`：MVP 用 **limit-only + `hasMore` 标志**：
  ```
  GET /skins?limit=50 → { items: [...], hasMore: true }
  ```
  Phase 2 真有大数据量时再上 cursor-based。
- **JSON 字段** `[convention]`：camelCase（与 BSON snake_case 对应通过 struct tag）
- **DTO 双 tag 强制对齐** `[convention]`：每个字段 json + bson tag 都写
  ```go
  type UserDTO struct {
      ID        UserID    `json:"id"          bson:"_id"`
      Name      string    `json:"displayName" bson:"display_name"`
      CreatedAt time.Time `json:"createdAt"   bson:"created_at"`
  }
  ```

### P3 · WebSocket Message 命名

- **Type 格式** `[convention]`：`domain.action` 点分（`blindbox.redeem`、`friend.state`）
- **响应 type** `[convention]`：`domain.action.result`
- **推送 type** `[convention]`：无 `.result` 后缀
- **心跳** `[convention]`：`ping` / `pong`（无 domain 前缀）
- **错误码复用 HTTP error code 注册表** `[convention]`
- **Envelope id（修订）** `[convention]`：客户端唯一 string 即可（不强制 UUID 格式）

### P4 · Error Classification（修订为 5 档）

| 分类 | HTTP 码 | 客户端期望行为 | 示例 |
|---|---|---|---|
| `retryable` | 500 / 502 / 503 / 504 | 指数退避自动重试 | `INTERNAL_ERROR`、`UPSTREAM_TIMEOUT` |
| `client_error` | 400 / 404 / 409 / 422 | 不重试，提示用户或调整请求 | `FRIEND_ALREADY_EXISTS`、`BLINDBOX_ALREADY_REDEEMED` |
| `silent_drop` | 200 / 业务静默 | 客户端无任何反应（fire-and-forget 类） | `TOUCH_RATE_LIMITED`（触碰限流场景） |
| `retry_after` | 429 | 等待 retry-after header 后重试 | `RATE_LIMIT_EXCEEDED`（业务级 429） |
| `fatal` | 401 / 403 (鉴权) | 清理 token → 强制登出 | `AUTH_TOKEN_EXPIRED`、`AUTH_REFRESH_TOKEN_REVOKED`、`DEVICE_BLACKLISTED` |

**编译期强制** `[compiler]`：
```go
type AppError struct {
    Code       string
    Message    string
    HTTPStatus int
    Category   ErrCategory  // 必填
    Cause      error
}
```

**测试强制** `[test]`：扫描所有 `error_codes.go` 确保每个 code 都归档到一个 category（启动时 init 检查 + 单元测试）。

**Error wrap 原则**：内部用 sentinel + `errors.Is/As`；**仅在最外层（handler / WS dispatcher）wrap 为 AppError**，避免重复 wrap 丢失 stack。

### P5 · Logging 强制规则

- **Level** `[convention]`：`debug` / `info` / `warn` / `error` / `fatal`
- **每条日志必含**：`time` + `level` + `msg`
- **Context 字段（如可获得）**：`requestId` (HTTP) / `connId + eventId` (WS) / `userId`
- **字段命名（修订）** `[convention]`：全 camelCase（`eventId` 而非 `event_id`）
- **userId 非强制**：bootstrap 流程（`/auth/apple`）尚无 userId 时可省略
- **禁** `[lint]`：`fmt.Printf`、`log.Printf`、`log.Println`、字符串拼接 → `forbidigo` 拦截
- **要求** `[convention]`：用 zerolog field API：
  ```go
  logx.Ctx(ctx).Info().
      Str("action", "blindbox_redeem").
      Str("blindboxId", string(id)).
      Int64("stepsAvailable", steps).
      Msg("blindbox redeemed")
  ```

### P6 · 测试模式

- **文件命名** `[compiler]`：`xxx_test.go` 同目录
- **集成测试** `[compiler]`：文件顶部 `//go:build integration` tag；CI 跑 `go test -tags=integration`
- **Table-driven** `[convention]`：多场景测试必须用 table-driven（宪法）
- **DB mock 禁用** `[convention]`（per memory）：Mongo / Redis 必须用 Testcontainers
- **集成测试禁并行** `[convention]`：禁用 `t.Parallel()`，避免 Testcontainers 资源冲突；单元测试默认开
- **错误测试** `[lint]`：用 `errors.Is/As`，不用字符串比较 → `errorlint` 拦截
- **testify**：优先 `require.NoError` / `assert.Equal`

### P7 · Request DTO 校验

- **位置** `[convention]`：handler 层调 service 之前
- **工具** `[compiler]`：`go-playground/validator/v10` 通过 Gin binding 自动触发
- **Validation error 转换** `[convention]`：→ `AppError{Code: "VALIDATION_ERROR", Category: client_error, HTTPStatus: 400}`
- **service 层不重复校验**：信任 handler；domain 层做**业务不变量**校验（如"盲盒状态必须为 unlocked"）

### M · 新增模式

#### M1 Package 命名 `[convention]`
单数小写短词（`user`、`blindbox`、`ws`）。**反例**：`services` ❌、`userManagement` ❌

#### M2 Interface 命名 `[convention]`
- Repository 接口直接 `<Entity>Repository`（`UserRepository`、`SkinRepository`）
- 单方法 service / utility 接口用 `-er` 后缀（`Broadcaster`、`Closer`）

#### M3 Constructor 命名 `[convention]`
- `NewXxx(deps...) *Xxx` 默认（return error 时用 `(*Xxx, error)`）
- `MustXxx(deps...) *Xxx` 用于"启动期失败即 panic"（如 `mongox.MustConnect`）
- 工厂方法 `<Adj><Type>`（如 `NewInMemoryBroadcaster`）

#### M4 Context cancellation 处理 `[lint]`（revive）
长循环 / 阻塞调用必须 `select case <-ctx.Done()`：
```go
for {
    select {
    case <-ctx.Done(): return ctx.Err()
    case msg := <-ch: handleMsg(msg)
    }
}
```
短同步函数依靠外层 ctx，不重复检查。

#### M5 Goroutine 生命周期 `[convention]`
- 任何启动的 goroutine 必须有明确退出机制
- 禁 fire-and-forget
- 必须由 Runnable 管理（持 `sync.WaitGroup`，graceful shutdown 等待）

#### M6 Enum / 常量命名 `[convention]`
- 业务 string（出现在 JSON / BSON / WS payload）：全小写（`idle` / `walking` / `running`）
- Go 内部 typed const：`<Domain><Value>` 模式
  ```go
  type CatState string
  const (
      CatStateIdle    CatState = "idle"
      CatStateWalking CatState = "walking"
      CatStateRunning CatState = "running"
      CatStateSleeping CatState = "sleeping"
  )
  ```

#### M7 Repository ↔ Service 边界 `[convention]`
Repository 返回 **domain entity**（`*domain.CatState`），不返 raw `bson.M`。所有 BSON ↔ Domain 转换在 repo 内部完成。

#### M8 DTO 转换位置 `[convention]`
- handler 层做 `DTO ↔ Domain Entity` 转换
- service 只接受 / 返回 domain entity
- **反模式**：`service.Foo(req *FooRequest)` —— service 知道 HTTP 形态

#### M9 Clock interface（FR60 测试钩子） `[convention]`
所有时间获取通过可注入 Clock：
```go
type Clock interface { Now() time.Time }
type RealClock struct{}
func (RealClock) Now() time.Time { return time.Now() }

// 测试用
type FakeClock struct{ now time.Time }
func (c *FakeClock) Now() time.Time { return c.now }
func (c *FakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }
```
所有 service / cron 持 Clock 依赖；**禁直接调 `time.Now()`** 在业务代码中。

#### M10 测试 helper 命名 `[convention]`
- `setupXxx(t)` / `teardownXxx(t)` / `assertXxx(t)`
- 强制调 `t.Helper()`：
  ```go
  func setupMongo(t *testing.T) *mongo.Client {
      t.Helper()
      // ...
  }
  ```

#### M11 集成测试禁并行 `[convention]`
集成测试（`//go:build integration`）禁用 `t.Parallel()`；单元测试默认 `t.Parallel()`。

#### M12 errors.Is/As `[lint]`（errorlint）
错误判断只能用 `errors.Is(err, sentinel)` 或 `errors.As(err, &target)`。禁字符串比较。

#### M13 PII 日志规则 `[convention]`
- `userId` 可记
- `displayName`、邮箱（如有）必须 `[REDACTED]`
- `pkg/logx/` 提供 `MaskPII(s string) string` helper
- 步数等敏感数据可记（合规 NFR-COMP-2 范围内）

#### M14 APNs token 脱敏 `[convention]`
- Device token 在日志中只显示**前 8 字符 + `...`**（如 `"abcd1234..."`）
- 仅 DEBUG level；INFO+ 不出现完整 token

#### M15 WS per-connection rate limit `[convention]`
每条 WS 连接限速：100 msg/s（进入 dispatcher 前的 token bucket）。超限 close conn + log。

#### M16 defer close 紧邻原则 `[convention]`
所有 `Open` / `Connect` / `Acquire` 后**下一行**写 `defer close`：
```go
file, err := os.Open(path)
if err != nil { return err }
defer file.Close()  // 紧邻
```

### Graceful Shutdown 顺序

由 `App.Run()` 按 Runnable 注册的**逆序** Final() 实现：

1. 停止接受新 HTTP / WS 连接（Gin server `Shutdown(ctx)`）
2. WS Hub 不再接受新消息（`Hub.Stop()`）
3. WS 现有连接发 close frame；5s 内允许消费完待处理消息
4. 停止 cron scheduler（`cron.Stop()`）
5. 等 APNs worker 处理完已 `XREADGROUP` 的消息（最多 10s；超时则丢弃，下次启动从 Redis Stream 续）
6. 关闭 Repository（Mongo `Disconnect`、Redis `Close`）
7. 刷 zerolog buffer（`os.Stdout.Sync()`）

**总耗时预算 ~20s**（宪法 §5 限 30s 内）。

### .golangci.yml 配置示例

```yaml
linters:
  enable:
    - errcheck      # 未处理的 error
    - errorlint     # M12: 强制 errors.Is/As
    - forbidigo     # P5: 禁 fmt.Printf / log.Printf
    - gocritic
    - revive        # M4: ctx.Done() 提示
    - unconvert
    - unparam
    - misspell
    - bodyclose     # M16: HTTP body 必须 close
    - gofmt
    - goimports
    - govet
    - unused
    - ineffassign
linters-settings:
  forbidigo:
    forbid:
      - '^fmt\.Printf$'
      - '^fmt\.Println$'
      - '^log\.(Print|Println|Printf)$'
```

### 标准代码示例（Claude Reference Source）

`docs/code-examples/` 提供 5-10 个标准代码示例作为 Claude 编码时的 reference：

- `handler_example.go` — 标准 HTTP handler（DTO 校验 → service → DTO 转换 → 响应）
- `ws_handler_example.go` — 标准 WS message handler（envelope 解析 → service → ack/error）
- `service_example.go` — 标准 Service（Clock 依赖、事务边界、broadcaster 调用）
- `repository_example.go` — 标准 Repository（domain ↔ BSON 转换、index 创建）
- `cron_job_example.go` — 标准 Cron job（withLock 包裹、ctx 检查）
- `error_codes_example.go` — Error code 注册（含 Category 字段）
- `test_unit_example.go` — Table-driven 单元测试
- `test_integration_example.go` — Testcontainers 集成测试

**这些示例是 source of truth**：写新代码时 Claude 必须参照对应模板，不凭"惯性"决策。

### Pattern 反例集

```go
// ❌ BAD: 违反 P2（wrapper）+ P4（无 category）
c.JSON(500, gin.H{"data": nil, "error": err.Error()})

// ✅ GOOD:
c.JSON(500, dto.AppError{
    Code:       "INTERNAL_ERROR",
    Message:    "state update failed",
    Category:   dto.ErrRetryable,
    HTTPStatus: 500,
})
```

```go
// ❌ BAD: 违反 P5（字符串拼接）+ M13（PII 暴露）
log.Printf("user %s (name: %s) redeemed blindbox", userID, displayName)

// ✅ GOOD:
logx.Ctx(ctx).Info().
    Str("userId", string(userID)).
    Str("displayName", logx.MaskPII(displayName)).
    Str("blindboxId", string(blindboxID)).
    Msg("blindbox redeemed")
```

```go
// ❌ BAD: 违反 M9（直接调 time.Now）
func (s *StateService) ApplyDecay(ctx context.Context) error {
    cutoff := time.Now().Add(-15 * time.Second)
    // ...
}

// ✅ GOOD:
type StateService struct {
    clock Clock
    // ...
}
func (s *StateService) ApplyDecay(ctx context.Context) error {
    cutoff := s.clock.Now().Add(-15 * time.Second)
    // ...
}
```

## Project Structure & Boundaries

### Complete Project Directory Structure

```
server/
├── README.md
├── go.mod
├── go.sum
├── Makefile
├── .editorconfig
├── .gitignore
├── .dockerignore
├── .golangci.yml
├── .github/
│   └── workflows/ci.yml
│
├── cmd/
│   └── cat/
│       ├── main.go            # ≤ 15 行：flag.Parse → config.Load → initialize → app.Run
│       ├── initialize.go      # 显式 DI 装配（≤ 200 行）
│       ├── app.go             # App 容器 + Runnable 接口 + signal handling
│       └── wire.go            # handler 聚合 + router build
│
├── internal/
│   ├── config/
│   │   ├── config.go          # TOML + Redis runtime 双层加载（D13）
│   │   └── config_test.go
│   ├── domain/
│   │   ├── cat_state.go       # CatState typed string + FSM + Source 优先级（D2）
│   │   ├── blindbox.go        # 状态机 + reward 权重表
│   │   ├── friend.go
│   │   ├── invite_token.go
│   │   ├── skin.go
│   │   ├── user.go
│   │   ├── enums.go           # 全部 typed const（M6）
│   │   └── *_test.go
│   ├── service/
│   │   ├── auth_service.go        # FR1-3
│   │   ├── state_service.go       # FR7-12, D2 优先级
│   │   ├── friend_service.go      # FR13-20, FR55
│   │   ├── touch_service.go       # FR26-30
│   │   ├── blindbox_service.go    # FR31-35, FR54
│   │   ├── skin_service.go        # FR36-39
│   │   ├── account_service.go     # FR47-50
│   │   ├── room_service.go        # FR21-22, FR51-52
│   │   ├── presence_service.go    # D8 presence Redis 操作
│   │   └── *_test.go
│   ├── repository/
│   │   ├── user_repo.go              # FR1, FR47-50
│   │   ├── cat_state_repo.go         # FR7-12（Mongo + Redis write-through）
│   │   ├── friend_repo.go            # FR13-19
│   │   ├── block_repo.go             # FR17-18, FR29
│   │   ├── invite_token_repo.go      # FR13-14, FR20
│   │   ├── blindbox_repo.go          # FR31-35
│   │   ├── skin_repo.go              # FR36-39
│   │   ├── touch_log_repo.go         # FR26 异步写
│   │   ├── apns_token_repo.go        # FR4, FR43, FR58
│   │   └── *_test.go                 # Testcontainers 集成测试
│   ├── handler/
│   │   ├── auth_handler.go           # POST /auth/apple, /auth/refresh
│   │   ├── device_handler.go         # POST /devices/apns-token
│   │   ├── state_handler.go          # POST /state（HealthKit 后台）
│   │   ├── health_handler.go         # GET /healthz, /readyz
│   │   └── *_test.go
│   ├── dto/
│   │   ├── error.go                  # AppError + Category 枚举（P4, D10）
│   │   ├── auth_dto.go
│   │   ├── state_dto.go
│   │   ├── ws_envelope.go
│   │   ├── ws_messages.go            # 全部 WS message type 常量 + payload
│   │   ├── error_codes.go            # 错误码注册表（含 Category）
│   │   └── error_codes_test.go       # 启动时校验 + 单元测试每码必有 category
│   ├── middleware/
│   │   ├── jwt_auth.go               # JWT 校验 → context.userId
│   │   ├── rate_limit.go             # Redis Counter（D5, M15）
│   │   ├── request_id.go             # X-Request-ID
│   │   ├── recover.go                # panic recovery → AppError 500
│   │   ├── logger.go                 # zerolog 结构化访问日志（P5）
│   │   ├── cors.go                   # CORS（仅 dev）
│   │   └── *_test.go
│   ├── ws/
│   │   ├── hub.go                    # Hub + connection registry（D1）
│   │   ├── broadcaster.go            # Broadcaster 接口 + InMemoryBroadcaster
│   │   ├── envelope.go               # envelope 解析 + ack 生成
│   │   ├── dispatcher.go             # message type → handler 路由
│   │   ├── upgrade_handler.go        # /ws + JWT + connection setup
│   │   ├── session_resume.go         # session.resume RPC（D6, FR42）
│   │   ├── rate_limit.go             # per-conn rate limit（M15）
│   │   ├── handlers/
│   │   │   ├── blindbox_handlers.go     # blindbox.redeem / inventory
│   │   │   ├── friend_handlers.go       # invite/accept/delete/block/unblock
│   │   │   ├── touch_handlers.go        # touch.send
│   │   │   ├── skin_handlers.go         # skin.equip / skins.catalog
│   │   │   ├── state_handlers.go        # state.tick
│   │   │   ├── room_handlers.go         # room join/leave + snapshot
│   │   │   └── system_handlers.go       # ping/pong / users.me / friends.list
│   │   └── *_test.go
│   ├── cron/
│   │   ├── scheduler.go              # robfig/cron + withLock（D5）
│   │   ├── blindbox_drop_job.go      # FR31 单槽位投放
│   │   ├── state_decay_job.go        # FR10, D7 衰减引擎（30s）
│   │   ├── cold_start_recall_job.go  # FR44a 冷启动检测（24h）
│   │   ├── apns_token_cleanup_job.go # FR43
│   │   ├── presence_cleanup_job.go   # D8 过期成员
│   │   └── *_test.go
│   ├── push/
│   │   ├── apns_client.go            # APNs HTTP/2 客户端
│   │   ├── apns_router.go            # platform 区分 + tier（D3, FR58）
│   │   ├── apns_worker.go            # Redis Stream consumer goroutine（D3）
│   │   └── *_test.go
│   └── testutil/
│       ├── mongo_setup.go            # Testcontainers Mongo（M10）
│       ├── redis_setup.go            # Testcontainers Redis
│       ├── fixture_loader.go
│       └── fake_clock.go             # M9 测试钩子
│
├── pkg/
│   ├── logx/
│   │   ├── logx.go                   # Init + Ctx + WithRequestID
│   │   ├── pii.go                    # MaskPII helper（M13）
│   │   └── logx_test.go
│   ├── mongox/
│   │   ├── client.go                 # MustConnect + ping + HealthCheck
│   │   ├── transaction.go            # WithTransaction(ctx, fn)（D10）
│   │   └── client_test.go
│   ├── redisx/
│   │   ├── client.go                 # MustConnect + ping + HealthCheck
│   │   ├── locker.go                 # SETNX 分布式锁（D5）
│   │   ├── stream.go                 # Stream consumer group helper（D3）
│   │   ├── presence.go               # Set + Hash 操作（D8）
│   │   └── *_test.go
│   ├── jwtx/
│   │   ├── manager.go                # RS256 签发 + 校验
│   │   ├── apple_jwk.go              # Apple JWK 拉取 + 缓存
│   │   └── *_test.go
│   ├── ids/
│   │   └── ids.go                    # UserID/SkinID/BlindboxID/FriendID/InviteTokenID/ConnID/RoomID
│   ├── fsm/
│   │   └── fsm.go                    # 通用状态机（CatState 用）
│   └── clockx/
│       └── clock.go                  # Clock interface + RealClock（M9）
│
├── config/
│   ├── default.toml
│   ├── production.toml
│   └── local.toml.example
│
├── deploy/
│   ├── Dockerfile
│   ├── docker-compose.yml            # Mongo（1 节点 replica set）+ Redis + app
│   └── nginx.conf                    # Phase 2 反向代理（可选）
│
├── docs/
│   ├── code-examples/                # 标准代码示例（Claude reference）
│   │   ├── handler_example.go
│   │   ├── ws_handler_example.go
│   │   ├── service_example.go
│   │   ├── repository_example.go
│   │   ├── cron_job_example.go
│   │   ├── error_codes_example.go
│   │   ├── test_unit_example.go
│   │   └── test_integration_example.go
│   ├── api/
│   │   ├── openapi.yaml              # HTTP API OpenAPI 3.0
│   │   └── ws-message-registry.md    # WS message type 注册表
│   ├── error-codes.md                # 错误码注册表（人类可读）
│   ├── skeleton-spec.md              # Walking Skeleton 设计意图
│   └── spikes/
│       └── op1-ws-stability.md       # Spike-OP1 报告
│
├── scripts/
│   └── build.sh
│
├── tools/                            # 一次性脚本
│
├── build/                            # gitignored
│   └── catserver
│
└── _bmad-output/
    └── planning-artifacts/
        ├── prd.md
        ├── architecture.md
        ├── implementation-readiness-report-2026-04-16.md
        └── epics.md (待 /bmad-create-epics-and-stories 生成)
```

**统计：** ~75 个 Go 文件 + ~15 个配置/文档文件 + 测试文件（~75 个）

### Architectural Boundaries

#### HTTP API 边界（仅 5 endpoint）

| Endpoint | 文件 | 责任 |
|---|---|---|
| `POST /auth/apple` | `handler/auth_handler.go` | bootstrap 鉴权 |
| `POST /auth/refresh` | `handler/auth_handler.go` | token 刷新 |
| `POST /devices/apns-token` | `handler/device_handler.go` | APNs token 注册 |
| `POST /state` | `handler/state_handler.go` | iPhone 后台 HealthKit（30s 特例） |
| `GET /healthz, /readyz` | `handler/health_handler.go` | 基础设施 |

#### WebSocket 边界

- 入口：`internal/ws/upgrade_handler.go` 单一 `/ws`
- Hub：`internal/ws/hub.go` 单进程 + `Broadcaster` 接口（多副本预留）
- Dispatcher：`internal/ws/dispatcher.go` 路由 type → handler
- Handlers：`internal/ws/handlers/` 按 domain 分组
- Service 调用：dispatcher 调 service；service 不直接持 WS 连接

#### Service ↔ Repository 边界

```
Handler ──▶ Service ──▶ Repository ──▶ Mongo/Redis
                │
                ├──▶ Broadcaster (WS)
                ├──▶ APNs Router (Push)
                └──▶ Clock (M9)
```

- Repository 接口在 service 包定义（"accept interfaces"）
- Repository 实现在 repository 包
- Service 不直接持 `*mongo.Client` / `*redis.Client`

#### Data 边界

| 持久层 | 责任 | 主要 Collection / Key |
|---|---|---|
| MongoDB | 权威持久化 | users, cat_states, friends, blocks, invite_tokens, blindboxes, skins, user_skins, touch_logs, apns_tokens |
| Redis | 热缓存 + 横切 | state:*, presence:*, event:*, ratelimit:*, blacklist:*, refresh_blacklist:*, lock:cron:*, apns:queue, config:runtime, config:features |

#### 横切关注点边界

- **JWT 鉴权**：`middleware/jwt_auth.go`（HTTP）+ `ws/upgrade_handler.go`（WS）共用 `pkg/jwtx`
- **限流**：`middleware/rate_limit.go`（HTTP per-IP）+ `ws/rate_limit.go`（per-conn）+ service 内 Redis Counter（per-pair touch / per-user invite）
- **结构化日志**：`pkg/logx` + `middleware/logger.go` 注入 requestId
- **分布式锁**：`pkg/redisx/locker.go` 由 cron / 异常设备标记消费
- **APNs 路由**：`internal/push/` 由 service 通过 `apns:queue` Stream 异步消费

### Requirements → Structure Mapping（按 Epic）

| Epic | 主要文件 | FR |
|---|---|---|
| **Epic 0: Walking Skeleton** | `cmd/cat/`, `pkg/{logx,mongox,redisx,clockx}`, `internal/{config, middleware}`, `internal/handler/health_handler.go`, `internal/ws/{hub,upgrade_handler,envelope,dispatcher}`, `docs/code-examples/`, `.golangci.yml`, `Makefile`, `deploy/` | FR40-46, FR56-57, FR59-60 |
| **E1: 身份与账户** | `handler/{auth, device}`, `service/{auth, account}`, `repository/{user, apns_token}`, `pkg/jwtx`, `dto/auth_dto.go` | FR1-6, FR47-50, FR58 |
| **E2: 你的猫活着** | `service/state_service.go`, `repository/cat_state_repo.go`, `handler/state_handler.go`, `cron/state_decay_job.go`, `domain/cat_state.go`, `pkg/fsm` | FR7-12 |
| **E3: 好友圈 + 冷启动召回** | `service/friend_service.go`, `repository/{friend, block, invite_token}`, `ws/handlers/friend_handlers.go`, `cron/cold_start_recall_job.go` | FR13-20, FR55, FR44a-b |
| **E4: 好友房间 / 在场感** | `ws/{hub, broadcaster, session_resume}`, `ws/handlers/room_handlers.go`, `service/{room, presence}`, `pkg/redisx/presence.go` | FR21-25, FR51-52 |
| **E5: 触碰 / 触觉社交** | `service/touch_service.go`, `repository/touch_log_repo.go`, `ws/handlers/touch_handlers.go` | FR26-30 |
| **E6: 盲盒 + 皮肤** | `service/{blindbox, skin}`, `repository/{blindbox, skin}`, `ws/handlers/{blindbox, skin}_handlers.go`, `cron/blindbox_drop_job.go`, `domain/blindbox.go` | FR31-39, FR54 |
| **APNs Push（横切）** | `internal/push/`, `cron/apns_token_cleanup_job.go`, `pkg/redisx/stream.go` | FR4, FR27, FR30, FR43, FR44b, FR58 |

### Integration Points

#### Internal Communication

```
HTTP Request → middleware → handler → service → repo → Mongo/Redis
                                          ├→ broadcaster → 本实例 WS conns
                                          ├→ apns router → Redis Stream apns:queue
                                          └→ clock / rate_limiter / dedup

WS Message in → upgrade → dispatcher → handler → service → repo + broadcaster
WS Message out → service → broadcaster → presence lookup → 本实例 / Pub/Sub remote
```

#### External Integrations

| 外部系统 | 通讯 | 文件 | 触发 |
|---|---|---|---|
| Apple Sign in with Apple | HTTPS GET (JWK) + identity token verify | `pkg/jwtx/apple_jwk.go` | 登录 |
| Apple APNs Provider API | HTTP/2 + token-based auth | `internal/push/apns_client.go` | apns_worker 消费 Stream |
| MongoDB | Mongo Driver v2 | `pkg/mongox/client.go` | 启动 + repo |
| Redis | go-redis/v9 | `pkg/redisx/client.go` | 启动 + 横切 + Stream |
| CDN（皮肤资源） | 不通讯 | — | 客户端直接 fetch |
| Uptime Robot | 反向探测 `/healthz` | `handler/health_handler.go` | 周期 |

#### Data Flow（核心路径）

**状态广播（产品心脏）：**
```
Watch state.tick (WS)
  → ws/dispatcher → ws/handlers/state_handlers
  → service/state_service.UpdateCatState(userId, state, source)
  → 优先级比较（D2）→ repo/cat_state_repo.Save (Mongo + Redis write-through)
  → broadcaster.BroadcastToRoom(roomId, friend.state)
  → presence.GetRoomMembers → 本实例 conn 直发 / 跨实例 Pub/Sub（Phase 3）
```

**触碰送达：**
```
WS touch.send → ws/handlers/touch_handlers
  → service/touch_service.Send(from, to, emoteType)
  → 限流 + 屏蔽检查 → presence.Lookup(to)
    ├─ 在线 → broadcaster.BroadcastToUser(to, friend.touch)
    └─ 离线 → push_router.Enqueue → Redis Stream apns:queue
              → apns_worker → APNs HTTP/2
```

**盲盒领取（强一致）：**
```
WS blindbox.redeem → handlers
  → service/blindbox_service.Redeem(userId, blindboxId, eventId)
  → Redis SETNX event:{eventId}（去重）
  → mongox.WithTransaction:
       blindbox_repo.MarkRedeemed
       cat_state_repo.IncrementPoints
       skin_repo.UnlockSkin
       blindbox_repo.LogAudit
  → broadcaster.BroadcastToRoom(friend.blindbox 视觉提示)
  → ack（payload: reward）
```

### File Organization Patterns

- **Configuration**：`config/default.toml` + `production.toml` 覆盖；运行时业务参数走 Redis `config:runtime`
- **Source**：layer 顶层（cmd / internal / pkg）；`internal/` 内 layer 二次拆分；`internal/ws/handlers/` 按 domain 横切
- **Test**：`*_test.go` 同目录；集成测试 `//go:build integration` tag
- **Asset**：服务端无静态资源；`docs/code-examples/` 是开发参考

### Development Workflow Integration

- **本地开发**：`make docker-up` → `go run ./cmd/cat -config config/local.toml`
- **测试**：`make test`（单元）/ `bash scripts/build.sh --test --integration`（集成 + race）
- **构建**：`bash scripts/build.sh` → `build/catserver`
- **Docker**：`docker build -f deploy/Dockerfile .`
- **CI**：`.github/workflows/ci.yml` lint + 单元 + 集成（Mongo/Redis services）+ build

## Architecture Validation Results

### Coherence Validation ✅

**Decision Compatibility（D1-D16 内部一致性）：** 7 对关键决策对验证全部 ✅
- D1 Broadcaster 接口 ↔ D6 OP-1 hub 接口预留：`PushOnConnect` + `BroadcastDiff` 已纳入
- D3 APNs Stream ↔ D16 事件驱动：复用同一 Redis Streams 基础设施
- D5 cron 锁 ↔ multi-replica invariant #3：`withLock` 包裹所有 cron job
- D8 presence Set ↔ D1 Broadcaster：InMemoryBroadcaster 解析 Set 成员的 instanceId
- D10 事务 + D2 状态冲突 + D12 时间戳：upsert + per-source 优先级
- D7 服务端衰减 ↔ D5 cron 锁 ↔ D11 数据保留：state_decay job 在锁保护下运行
- D13 配置分层 ↔ D14 可观测性：`config:runtime` Redis 不影响日志输出

**Pattern Consistency（P1-P7 + M1-M16）：**
- P2 JSON camelCase ↔ P1 BSON snake_case ↔ P3 WS msg type：通过 struct tag 隔离 ✅
- P4 5 档错误分类 ↔ M12 errors.Is/As ↔ AppError.Category：编译期 + lint + test 三层保障 ✅
- P5 zerolog field API ↔ M13 PII mask ↔ M14 APNs token mask：共用 `pkg/logx` ✅
- M9 Clock ↔ FR60 虚拟时钟 ↔ D7 衰减引擎：FakeClock 可控 ✅

**Structure Alignment：** 所有目录结构支持决策 ✅

### Requirements Coverage Validation ✅

#### FR Coverage：60/60（100%）

按 Step 6 Epic 映射表全部覆盖。关键 FR 已二次验证（鉴权、状态、好友、房间、触碰、盲盒、皮肤、运维、冷启动、账户、平台横切）。

#### NFR Coverage：52/52（100%）

| NFR 类别 | 实现机制 | 状态 |
|---|---|---|
| Performance PERF-1~7 | D1 Broadcaster + D7 衰减 + D2 优先级 + P5 字段日志 | ✅ |
| Security SEC-1~10 | TLS 部署层 + jwtx RS256 + middleware/rate_limit + Redis SETNX dedup + zerolog audit | ✅ |
| Scalability SCALE-1~9 | multi-replica 5 invariants + D5 + D8 + D3 Streams 全就绪 | ✅ |
| Reliability REL-1~8 | D10 事务 + D3 APNs 重试 + Runnable graceful shutdown + Mongo replica set | ✅ |
| Observability OBS-1~7 | logx + middleware/{logger, request_id} + health_handler 多维 + Uptime Robot | ✅ |
| Compliance COMP-1~6 | M13 PII + 步数模型 + APNs guideline + 数据最小化 + cron retention | ✅ |
| Integration INT-1~5 | jwtx/apple_jwk + push/apns_client + (Universal Links → 见 Gap G1) + CDN | ⚠️ 见 G1 |

#### Open Items 收敛状态

**已收敛（6/10）：** WS Hub 结构（D1）、OP-2 状态冲突（D2）、OP-5 APNs 队列（D3）、OP-6 Change Streams（D4）、cron 锁（D5）、OP-1 hub 接口约束（D6）、ADR-001 衰减（D7）、ADR-002 presence（D8）

**按设计延后（4/10）：** OP-1 final 稳定性（Spike-OP1 后）、OP-3 fan-out（Patterns 阶段）、OP-4 session.resume 增量（Patterns 阶段）、OP-7 i18n（Vision）、ADR-003 hub 上限（Spike）

### Implementation Readiness Validation ✅

- **Decision Completeness**：16 D + 3 ADR 全部有理由 + 影响 + 实现指引；决策依赖图清晰
- **Structure Completeness**：完整 75+ Go 文件树；FR → Epic → 文件三级映射；5 类边界 + 3 条核心 data flow 端到端
- **Pattern Completeness**：7 P + 16 M 模式 + 强度 tag + Anti-pattern 反例 + 标准代码示例

### Gap Analysis Results

#### 🔴 Critical Gaps：无

#### 🟠 Important Gaps（Epic 0 / E1 阶段必处理）

**G1：Universal Links 静态文件 + Nginx 配置**
- NFR-INT-3 要求 `.well-known/apple-app-site-association` 由 web 层服务
- 修复：`deploy/well-known/apple-app-site-association` + `deploy/nginx.conf` 路由；或 Gin 直接服务

**G2：OpenAPI / WS Registry 文档维护**
- 已写路径 `docs/api/`，但更新责任和 CI 校验未明
- 修复：CI 加 `swagger validate docs/api/openapi.yaml`；WS registry 通过 unit test 扫 `dto/ws_messages.go` 自动核对

**G3：APNs 证书 / Token 配置存放**
- `.p8` 私钥 + key id + team id 未明确存放方式
- 修复：`config/` 不含密钥；`.p8` 挂载 Docker volume；环境变量传 keyId/teamId/topic

#### 🟡 Minor Gaps

**G4：Mongo replica set 初始化脚本** —— `deploy/mongo-init.js` 执行 `rs.initiate()`
**G5：Spike-OP1 测试矩阵工具未定** —— `docs/spikes/op1-ws-stability.md` 模板预留章节（k6 / vegeta / 自写 Go 客户端）
**G6：Go 1.25 + Gin 兼容性验证** —— Walking Skeleton S0.1 完成后 `go vet` 验证

### Architecture Completeness Checklist

**✅ Requirements Analysis** — context 深度分析、scale + complexity 评估、constraints 罗列、cross-cutting concerns 明确
**✅ Architectural Decisions** — 16 D + 3 ADR + 决策依赖图、技术栈版本、集成模式、performance 考量
**✅ Implementation Patterns** — 命名规范、结构模式、通讯模式、流程模式
**✅ Project Structure** — 完整目录、组件边界、集成点、FR → Epic → 文件映射

### Architecture Readiness Assessment

**Overall Status：READY FOR IMPLEMENTATION**（含 G1-G6 实现期 follow-up）

**Confidence Level：High**
- PRD 60 FR + 52 NFR 100% 架构支持
- 7 个 Open Items 中 6 个 closed；3 个 Open 是按设计延后到 Step 5 Patterns / Spike-OP1
- 实现就绪度报告的 4 个 Critical 问题在本 Architecture + Step 6 Structure 中全部得到响应

**Key Strengths：**
1. 架构宪法 + PRD + Architecture 三层契合 —— 上层钉死方向，下层补具体决策
2. multi-replica invariants 提前内化到代码层面 —— 单实例 MVP 部署，但代码不绑定单实例
3. OP-1 没有掩盖 —— 明确为 open problem + Spike-OP1 + hub 接口预留三招应对，而非加 backup
4. 横切关注点显式提取 —— 6 项 cross-cutting concerns 都有归档位置
5. Pattern 强度 tag 让 Lint 可执行的部分明确 —— 不是"愿景式"约束

**Areas for Future Enhancement：**
- Phase 2 引入 Loki / Prometheus 完整观测栈
- Phase 3 引入 WS sticky routing + 多副本部署 + Pub/Sub broadcaster
- 未来 PRD 扩展（日历签到、共同目标等）需评估对当前 architecture 的影响

### Implementation Handoff

**AI Agent Guidelines：**
1. 遵循所有 D1-D16 决策与 P1-P7 / M1-M16 模式
2. 写新代码前对照 `docs/code-examples/` 标准模板
3. 任何与本 Architecture 冲突的实现需先更新本文档
4. Open Items（OP-1 / OP-3 / OP-4）实现时回写收敛结果到 architecture.md

**First Implementation Priority：Epic 0 Walking Skeleton**（4 个 stories：S0.1 → S0.2 → S0.3 → S0.4）

**Critical Path：**
```
Epic 0 (Walking Skeleton)
  → E1 (身份与账户)
  ├→ E2 (你的猫活着) ─→ E6 (盲盒+皮肤)
  └→ E3 (好友圈+冷启动)
                ↓
       [Spike-OP1 完成 + OP-1 收敛]
                ↓
       E4 (好友房间) → E5 (触碰)
```

E1/E2/E3/E6 可**并行启动**，不阻塞 Spike-OP1。E4/E5 等 Spike-OP1 收敛后再开始。
