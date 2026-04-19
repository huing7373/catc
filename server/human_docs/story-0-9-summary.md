# Story 0.9: WS Hub 骨架 + Envelope + Broadcaster 接口 — 实现总结

搭建完整的 WebSocket 栈（upgrade → envelope → dispatcher → hub → broadcaster），为所有后续业务 WS handler 提供统一的消息路由和广播层。

## 做了什么

### 消息协议（Envelope）
- `internal/ws/envelope.go` 定义了三种消息格式：
  - **上行 Envelope**：`{id, type, payload}` — 客户端发给服务端的请求
  - **下行 Response**：`{id, ok, type, payload, error}` — 服务端对请求的应答，type 自动加 `.result` 后缀
  - **推送 Push**：`{type, payload}` — 服务端主动推送，无 id
- 心跳使用 WebSocket 协议层 ping/pong（不是应用层 envelope）

### 连接管理（Hub + Client）
- `internal/ws/hub.go` 是连接注册中心，用 `sync.Map` 存 `connID → *Client`
- 每个 WS 连接启动两个 goroutine：
  - **readPump**：循环读消息 → rate limit 检查 → 交给 Dispatcher 路由
  - **writePump**：从 send channel 读消息写出 + 定时发 ping
- Hub 实现 Runnable 接口，`Final()` 给所有在线连接发 CloseGoingAway close frame
- Hub 实现 `WSHubChecker` 接口（替换了 Story 0.4 的 HubStub），`GoroutineCount()` 返回连接数 × 2

### 消息路由（Dispatcher）
- `internal/ws/dispatcher.go` 按 `envelope.type` 路由到注册的 handler
- 未知 type 返回 `UNKNOWN_MESSAGE_TYPE` 错误
- 无效 JSON 返回 `VALIDATION_ERROR` 错误
- `debug.echo` handler 仅在 `cfg.Server.Mode == "debug"` 时注册，原样返回 payload

### 广播层（Broadcaster）
- `internal/ws/broadcaster.go` 定义 `Broadcaster` 接口，4 个方法：
  - `BroadcastToUser` — 发送到用户的所有连接（已实现）
  - `BroadcastToRoom` — MVP 阶段 no-op，Story 4.x 实现
  - `PushOnConnect` / `BroadcastDiff` — D6 OP-1 预留，no-op
- `InMemoryBroadcaster` 基于 Hub 的 `FindByUser()` 实现单进程广播，Phase 3 可替换为 RedisPubSubBroadcaster

### WS Upgrade 入口
- `internal/ws/upgrade_handler.go` 处理 `/ws` endpoint 的 HTTP → WS 升级
- 升级前校验 `Authorization: Bearer <token>` header
- `TokenValidator` 接口：debug 模式用 `debugValidator`（token 即 userID），非 debug 模式用 `stubValidator`（始终拒绝），Story 1.3 注入真实 jwtx 实现
- connID 用 `uuid.New()` 生成

### Per-conn Rate Limit
- `internal/ws/rate_limit.go` 使用 `golang.org/x/time/rate`，每连接 100 msg/s token bucket
- 超限直接关闭连接 + zerolog warn

### 配置扩展
- `ServerCfg` 新增 `Mode` 字段（release/debug）
- `WSCfg` 新增 `PingIntervalSec`(30)、`PongTimeoutSec`(60)、`SendBufSize`(256)

## 怎么实现的

**并发安全的关键设计：** `send` channel 永不关闭。Client 用一个 `done` channel（`sync.Once` 守卫）通知 writePump 退出。`trySend()` 用三路 select（done/send/default）发送消息——因为 send 从不关闭，所以不可能 panic。这个设计是经过两轮 review 迭代后确定的：
- Round 1 发现 `close(send)` 在 Unregister 路径上与并发 publisher 冲突
- Round 2 发现 `atomic.Bool` + channel send 的 check-then-act 仍有竞态窗口

**Backpressure（宪法 §12）：** send channel 满时，`trySend()` 返回 false，调用方（InMemoryBroadcaster / Dispatcher.sendResponse）调用 `hub.unregisterClient()` 关闭该连接。

**Graceful Shutdown 顺序：** Runnable 注册顺序为 `mongoCli, redisCli, cronSch, wsHub, httpSrv`，Final 逆序执行：先停 HTTP → 关 WS 连接 → 停 cron → 关 DB。

**M9 合规：** Hub 注入 `clockx.Clock`，所有 `time.Now()` 替换为 `clock.Now()`，CI `check_time_now.sh` 扫描通过。

**healthz 集成：** `GoroutineCount()` 返回 `连接数 × 2`，initialize.go 传 `MaxConnections * 2` 作为健康检查阈值，与 goroutine 计数语义匹配。

## 怎么验证的

- `bash scripts/build.sh --test` 全量通过
- 18 个 WS 单元测试：envelope 序列化、rate limit、dispatcher 路由、hub 注册/注销、broadcaster 广播
- 3 个集成测试（`//go:build integration`）：echo round-trip、无 token 拒绝、shutdown close frame
- `check_time_now.sh` 通过（ws/ 无直接 `time.Now()` 调用）
- 新增依赖：gorilla/websocket v1.5.3, golang.org/x/time v0.15.0

## 后续 story 怎么用

- **Story 0.10**（WS eventId 去重）：在 Dispatcher 上添加 middleware，基于 `envelope.id` 做幂等去重
- **Story 0.11**（WS 连接限流 + 异常设备拒连）：扩展 `upgrade_handler.go`，在 upgrade 前做全局连接数限制和设备黑名单检查
- **Story 0.12**（session.resume）：消费 Hub + Broadcaster，实现断线重连时的状态恢复
- **Story 1.3**（JWT auth 中间件）：替换 `TokenValidator` 的 stubValidator 为真实 jwtx 验证
- **Epic 2-6 业务 handler**：在 `internal/ws/handlers/` 下按 domain 分组，每个 handler 实现 `HandlerFunc` 签名，通过 `dispatcher.Register()` 注册
- **Phase 3 多副本**：将 `InMemoryBroadcaster` 替换为 `RedisPubSubBroadcaster`，Hub 接口不变
