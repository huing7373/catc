# Story 0.9: WS Hub 骨架 + Envelope + Broadcaster 接口

Status: done

## Story

As a platform engineer,
I want the full WS stack (upgrade → envelope → dispatcher → hub → broadcaster interface) in place with an InMemory implementation and Phase 3 Pub/Sub replacement point,
So that all business WS handlers (FR21-FR39) can plug into one unified routing and broadcast layer (D1, D6, M15).

## Acceptance Criteria

1. **AC1 — WS Upgrade**：`/ws` endpoint 走 HTTP upgrade（gorilla/websocket）；upgrade 前校验 `Authorization: Bearer <JWT>` header，jwtx 验证失败返回 401 + `AppError{Code: "AUTH_TOKEN_EXPIRED / AUTH_INVALID_IDENTITY_TOKEN"}`
2. **AC2 — Envelope 定义**：上行 `{id: string, type: string, payload: json.RawMessage}`；下行响应 `{id, ok: bool, type: "<req>.result", payload, error?: {code, message}}`；推送 `{type, payload}`；心跳 `{type: "ping" | "pong"}`
3. **AC3 — Envelope ID**：客户端唯一 string（P3 不强制 UUID）
4. **AC4 — Broadcaster 接口**：`BroadcastToUser(ctx, userID, msg) / BroadcastToRoom(ctx, roomID, msg) / PushOnConnect(ctx, connID, userID) / BroadcastDiff(ctx, userID, diff)`（D6 OP-1 预留）
5. **AC5 — InMemoryBroadcaster**：基于 Hub 内部 `sync.Map` 存 `connID → *Client`（白名单：仅 `internal/ws/hub.go` 允许 `sync.Map`）
6. **AC6 — Hub Runnable**：Hub 实现 Runnable；`Final` 对所有在线连接发 close frame，5 秒内完成
7. **AC7 — 双 goroutine 模型**：每 WS 连接 readPump / writePump + 带缓冲 channel；backpressure：`send` chan 满则关闭该连接（宪法 §12）
8. **AC8 — 心跳**：服务端每 30s 向每 client 发 ping；60s 无 pong 关闭连接
9. **AC9 — Per-conn rate limit**：100 msg/s token bucket（M15）；超限 close + zerolog warn
10. **AC10 — Dispatcher**：按 `envelope.type` 路由到注册的 handler；未知 type 返回 `{ok: false, error: {code: "UNKNOWN_MESSAGE_TYPE"}}`
11. **AC11 — debug.echo**：仅 `cfg.Server.Mode == "debug"` 注册；Hub 原样返回 payload
12. **AC12 — 集成测试**：建连 → JWT 校验通过 → 发 `debug.echo` → 收到相同 payload；kill server 时客户端收到 close frame
13. **AC13 — ws_handler_example.go**：`docs/code-examples/ws_handler_example.go` 展示标准 WS handler 模式（envelope 解析 → service → ack/error）
14. **AC14 — GoroutineCount**：Hub 实现 `WSHubChecker` 接口（替换 HubStub），`GoroutineCount()` 返回实际 WS goroutine 数量

## Tasks / Subtasks

- [x] Task 1: Envelope 定义 (AC: #2, #3)
  - [x] 1.1 创建 `internal/ws/envelope.go`
  - [x] 1.2 定义 `Envelope` struct：`ID string`, `Type string`, `Payload json.RawMessage`
  - [x] 1.3 定义 `Response` struct：`ID string`, `OK bool`, `Type string`, `Payload json.RawMessage`, `Error *ErrorPayload`（ErrorPayload: Code + Message）
  - [x] 1.4 定义 `Push` struct：`Type string`, `Payload json.RawMessage`
  - [x] 1.5 辅助函数：`NewAckResponse(id, reqType string, payload json.RawMessage) Response`（Type = reqType + ".result"，OK = true）
  - [x] 1.6 辅助函数：`NewErrorResponse(id, reqType string, code, message string) Response`（OK = false）
  - [x] 1.7 辅助函数：`NewPush(pushType string, payload json.RawMessage) Push`

- [x] Task 2: Broadcaster 接口 + InMemoryBroadcaster (AC: #4, #5)
  - [x] 2.1 创建 `internal/ws/broadcaster.go`
  - [x] 2.2 定义 `Broadcaster` 接口：4 个方法签名（BroadcastToUser, BroadcastToRoom, PushOnConnect, BroadcastDiff）
  - [x] 2.3 定义 `ConnID`, `UserID`, `RoomID` 类型（string 类型别名）
  - [x] 2.4 `InMemoryBroadcaster` struct 持有 Hub 引用
  - [x] 2.5 `BroadcastToUser`：从 Hub 查找 userID 的所有连接，发送 msg
  - [x] 2.6 `BroadcastToRoom`：MVP 阶段不实现真实 room 逻辑（预留接口，log warn），后续 Story 4.x 实现
  - [x] 2.7 `PushOnConnect` / `BroadcastDiff`：D6 预留，MVP 阶段 log debug + no-op

- [x] Task 3: Rate Limiter (AC: #9)
  - [x] 3.1 创建 `internal/ws/rate_limit.go`
  - [x] 3.2 实现 per-connection token bucket（100 msg/s），纯内存，不依赖 Redis
  - [x] 3.3 `Allow() bool` 方法
  - [x] 3.4 使用 `golang.org/x/time/rate` 标准库（`rate.NewLimiter(100, 100)`）

- [x] Task 4: Hub + Client (AC: #5, #6, #7, #8, #14)
  - [x] 4.1 创建 `internal/ws/hub.go`
  - [x] 4.2 `Hub` struct 持有：`sync.Map`（connID → *Client），atomic 计数器
  - [x] 4.3 `Client` struct：`connID ConnID`, `userID UserID`, `conn *websocket.Conn`, `send chan []byte`（带缓冲），`hub *Hub`, `dispatcher *Dispatcher`
  - [x] 4.4 `Hub` 构造函数 `NewHub(cfg HubConfig, clock clockx.Clock)`：HubConfig 包含 PingInterval(30s), PongTimeout(60s), SendBufSize(256), MaxConnections
  - [x] 4.5 `Hub` 实现 Runnable：`Name()` 返回 `"ws_hub"`；`Start` no-op；`Final` 遍历所有连接发 close frame + 5s 超时
  - [x] 4.6 `Hub` 实现 `WSHubChecker`：`GoroutineCount()` 返回连接数 × 2
  - [x] 4.7 `Register(client *Client)` / `Unregister(connID ConnID)` 方法
  - [x] 4.8 `FindByUser(userID UserID) []*Client` 方法
  - [x] 4.9 `readPump(client *Client)`：循环读 WS 消息 → 检查 rate limit → 交给 Dispatcher
  - [x] 4.10 `writePump(client *Client)`：从 send channel 读 → 写 WS；处理 ping/pong 心跳
  - [x] 4.11 backpressure：send channel 满时 close 该连接并 Unregister

- [x] Task 5: Dispatcher (AC: #10, #11)
  - [x] 5.1 创建 `internal/ws/dispatcher.go`
  - [x] 5.2 `Dispatcher` struct 持有 `handlers map[string]HandlerFunc`
  - [x] 5.3 `HandlerFunc` 类型定义：`func(ctx context.Context, client *Client, env Envelope) (json.RawMessage, error)`
  - [x] 5.4 `Register(msgType string, fn HandlerFunc)` 方法
  - [x] 5.5 `Dispatch(ctx context.Context, client *Client, raw []byte)`：解析 envelope → 路由到 handler；未知 type 返回 UNKNOWN_MESSAGE_TYPE
  - [x] 5.6 handler 返回 error 时返回 error response
  - [x] 5.7 `debug.echo` handler：原样返回 payload（仅 debug 模式注册，在 initialize.go 中）

- [x] Task 6: Upgrade Handler (AC: #1)
  - [x] 6.1 创建 `internal/ws/upgrade_handler.go`
  - [x] 6.2 `UpgradeHandler` struct 持有 Hub, Dispatcher, validator, upgrader
  - [x] 6.3 Gin handler：解析 JWT → 验证失败返回 401 → upgrade → 创建 Client → Register → 启动 readPump/writePump goroutine
  - [x] 6.4 `TokenValidator` 接口 + `debugValidator`（token 即 userID）+ `stubValidator`（始终返回 error）
  - [x] 6.5 connID 使用 `uuid.New().String()` 生成

- [x] Task 7: Config 扩展 (AC: #11)
  - [x] 7.1 `ServerCfg` 添加 `Mode string` 字段（`toml:"mode"`）
  - [x] 7.2 `WSCfg` 添加 `PingIntervalSec int`、`PongTimeoutSec int`、`SendBufSize int`
  - [x] 7.3 `default.toml` 添加 `mode = "release"` 和 WS 新字段
  - [x] 7.4 `local.toml` 添加 `mode = "debug"`

- [x] Task 8: initialize.go + wire.go 装配 (AC: #1, #6)
  - [x] 8.1 创建 Hub、Dispatcher、InMemoryBroadcaster（未注入，但已实现）、UpgradeHandler
  - [x] 8.2 根据 `cfg.Server.Mode == "debug"` 注册 debug.echo handler
  - [x] 8.3 替换 `ws.NewHubStub()` 为真实 Hub；health_handler 接收 Hub（Hub 实现 WSHubChecker）
  - [x] 8.4 `buildRouter` 添加 `/ws` 路由
  - [x] 8.5 Hub 作为 Runnable 加入 App（注册顺序：mongoCli, redisCli, cronSch, wsHub, httpSrv）
  - [x] 8.6 删除 `internal/ws/hub_stub.go`

- [x] Task 9: 单元测试
  - [x] 9.1 `internal/ws/envelope_test.go`：5 个测试（Unmarshal/AckResponse/ErrorResponse/Push/MarshalJSON）
  - [x] 9.2 `internal/ws/rate_limit_test.go`：2 个测试（AllowWithinRate/ExceedRate）
  - [x] 9.3 `internal/ws/dispatcher_test.go`：3 个测试（KnownType/UnknownType/InvalidEnvelope）
  - [x] 9.4 `internal/ws/hub_test.go`：5 个测试（RegisterUnregister/UnregisterNonExistent/FindByUser/GoroutineCount/Name）
  - [x] 9.5 `internal/ws/broadcaster_test.go`：3 个测试（BroadcastToUser/NoConnections/InterfaceCompliance）

- [x] Task 10: 集成测试 (AC: #12)
  - [x] 10.1 创建 `internal/ws/upgrade_handler_integration_test.go`（`//go:build integration` tag）
  - [x] 10.2 启动 httptest.Server + Hub + Dispatcher（debug.echo 已注册）
  - [x] 10.3 测试：gorilla/websocket client 建连 → 发 debug.echo → 验证收到 ack + 相同 payload
  - [x] 10.4 测试：无 token 建连 → 验证 401 拒绝
  - [x] 10.5 测试：shutdown 时客户端收到 close frame（CloseGoingAway）

- [x] Task 11: ws_handler_example.go (AC: #13)
  - [x] 11.1 创建 `docs/code-examples/ws_handler_example.go`
  - [x] 11.2 展示标准模式：HandlerFunc 签名 → 解析 payload → 调用 service → 返回 ack payload 或 error

- [x] Task 12: 清理 + 集成验证
  - [x] 12.1 删除 `internal/ws/doc.go`（占位文件）
  - [x] 12.2 `bash scripts/build.sh --test` 全量通过（18 个测试 ws 包 + 全部回归通过）
  - [x] 12.3 `check_time_now.sh` 通过（hub.go 使用 clock.Now() 而非 time.Now()）
  - [x] 12.4 health_handler 集成测试改用真实 Hub（NewHub(HubConfig{}, clockx.NewRealClock())）

## Dev Notes

### Architecture Constraints (MANDATORY)

- **宪法 §1（显式胜于隐式）**：无 DI 框架；所有组件在 `initialize.go` 手动构造
- **宪法 §12（backpressure）**：send channel 满则关闭连接，不阻塞 writePump
- **D1 WebSocket Hub**：MVP 单进程内存 hub + `Broadcaster` 接口（Phase 3 替换为 RedisPubSubBroadcaster）
- **D6 OP-1 预留**：`PushOnConnect` + `BroadcastDiff` 方法签名纳入接口，MVP 阶段 no-op
- **M4 ctx.Done 检查**：readPump/writePump 循环必须检查 ctx.Done()
- **M5 goroutine 生命周期**：readPump/writePump 由 Hub 管理；Hub.Final 负责关闭所有连接
- **M9 Clock interface**：如需时间戳使用 `clock.Now()`（CI 扫描守卫 `internal/ws/` 下 `time.Now()` 调用）
- **M15 per-conn rate limit**：100 msg/s token bucket，进入 dispatcher 前检查
- **P3 消息命名**：`domain.action` 点分；响应 `.result` 后缀；心跳 `ping`/`pong` 无前缀
- **Graceful Shutdown 顺序**：注册顺序 `mongoCli, redisCli, cronSch, wsHub, httpSrv`；Final 逆序：httpSrv → wsHub → cronSch → redisCli → mongoCli（先停 HTTP，再关 WS 连接，再停 cron，最后关 DB 连接）

### 关键实现细节

**gorilla/websocket 核心 API：**
```go
upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
conn, err := upgrader.Upgrade(w, r, nil)
conn.SetReadDeadline(time.Now().Add(pongWait))
conn.SetPongHandler(func(string) error {
    conn.SetReadDeadline(time.Now().Add(pongWait))
    return nil
})
conn.WriteMessage(websocket.PingMessage, nil) // 服务端 ping
conn.WriteMessage(websocket.TextMessage, data) // 发送文本
_, message, err := conn.ReadMessage() // 读取消息
conn.WriteMessage(websocket.CloseMessage,
    websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
```

**注意：gorilla/websocket 的 ping/pong 是 WebSocket 协议层，不是应用层 envelope。** 服务端用 `conn.WriteMessage(websocket.PingMessage, nil)` 发 ping；客户端浏览器自动回 pong。应用层心跳 envelope `{type: "ping"}` 是另一回事（本 story 只实现协议层 ping/pong）。

**readPump / writePump 标准模式：**
- readPump 运行在自己的 goroutine，循环 `conn.ReadMessage()` → 解析 Envelope → 检查 rate limit → `dispatcher.Dispatch()`
- writePump 运行在自己的 goroutine，select `client.send` channel 或 `ticker.C`（ping interval）
- readPump 退出时关闭 send channel → writePump 检测到 channel 关闭退出
- writePump 退出时调用 `conn.Close()`

**send channel 满的 backpressure 处理：**
```go
select {
case client.send <- msg:
default:
    // channel 满，关闭连接
    hub.Unregister(client.connID)
    client.conn.Close()
}
```

**rate.Limiter 使用（golang.org/x/time/rate）：**
```go
limiter := rate.NewLimiter(100, 100) // 100 events/sec, burst 100
if !limiter.Allow() {
    // 超限，关闭连接
}
```

**Hub.Final close frame 发送（5s 超时）：**
```go
func (h *Hub) Final(ctx context.Context) error {
    deadline := time.Now().Add(5 * time.Second)
    h.clients.Range(func(key, value any) bool {
        c := value.(*Client)
        c.conn.WriteControl(websocket.CloseMessage,
            websocket.FormatCloseMessage(websocket.CloseGoingAway, "server shutdown"),
            deadline)
        c.conn.Close()
        return true
    })
    return nil
}
```

**JWT 校验 — 本 story 的简化方案：**
Story 1.3 才实现完整 jwtx 验证中间件。本 story 需要：
- 定义 `TokenValidator` 接口：`ValidateToken(token string) (userID string, err error)`
- debug 模式提供 `debugValidator`：直接把 token 当 userID 返回（开发测试用）
- 非 debug 模式提供 `stubValidator`：始终返回 `ErrAuthInvalidIdentityToken`（直到 Story 1.3 注入真实实现）
- upgrade_handler 从 `Authorization: Bearer <token>` header 解析 token

**现有 error codes 已注册（`internal/dto/error_codes.go`）：**
- `ErrAuthInvalidIdentityToken` — AUTH_INVALID_IDENTITY_TOKEN, 401, CategoryFatal
- `ErrAuthTokenExpired` — AUTH_TOKEN_EXPIRED, 401, CategoryFatal
- `ErrUnknownMessageType` — UNKNOWN_MESSAGE_TYPE, 400, CategoryClientError

这些 error codes 已存在，直接使用即可，不要重复注册。

**ServerCfg.Mode 添加注意：**
`config.go` 的 `ServerCfg` 目前只有 Host/Port/TLS。添加 `Mode string`。`default.toml` 设为 `"release"`，`local.toml`（如存在）设为 `"debug"`。

**HubStub 替换：**
- `internal/ws/hub_stub.go` 目前被 `initialize.go` 使用创建占位 Hub
- `health_handler.go` 依赖 `WSHubChecker` 接口（`GoroutineCount() int`）
- 真实 Hub 需实现 `GoroutineCount()` — 返回 `连接数 × 2`
- 删除 `hub_stub.go`，`initialize.go` 改用真实 Hub
- health_handler 集成测试目前 import `ws.NewHubStub()`，需改为使用真实 Hub 或保留兼容（推荐：集成测试仍可用 mock；hub_stub 的 mock 在 `health_handler_test.go` 已有 `mockHub`，不受影响）

**新增依赖：**
- `github.com/gorilla/websocket` — WS 协议
- `golang.org/x/time` — rate.Limiter（如果不想引新依赖，可自实现简单 token bucket；但 x/time/rate 是标准做法）

### Source Tree — 要创建/修改的文件

**新建：**
- `internal/ws/envelope.go` — Envelope / Response / Push 类型定义 + 辅助函数
- `internal/ws/broadcaster.go` — Broadcaster 接口 + InMemoryBroadcaster
- `internal/ws/rate_limit.go` — per-conn token bucket rate limiter
- `internal/ws/hub.go` — Hub struct + Client + readPump / writePump + Runnable
- `internal/ws/dispatcher.go` — message type → handler 路由
- `internal/ws/upgrade_handler.go` — /ws endpoint + JWT 校验 + WS upgrade
- `internal/ws/envelope_test.go`
- `internal/ws/rate_limit_test.go`
- `internal/ws/hub_test.go`
- `internal/ws/broadcaster_test.go`
- `internal/ws/dispatcher_test.go`
- `internal/ws/upgrade_handler_integration_test.go`
- `docs/code-examples/ws_handler_example.go`

**修改：**
- `internal/config/config.go` — ServerCfg 添加 Mode；WSCfg 添加 PingIntervalSec, PongTimeoutSec, SendBufSize
- `config/default.toml` — 添加 server.mode, ws 新字段
- `cmd/cat/initialize.go` — Hub/Dispatcher/Broadcaster/UpgradeHandler 创建；替换 HubStub；Hub 加入 Runnable
- `cmd/cat/wire.go` — handlers struct 添加 ws upgrade handler；buildRouter 添加 /ws 路由
- `go.mod` / `go.sum` — gorilla/websocket, golang.org/x/time
- `internal/handler/health_handler_integration_test.go` — NewHubStub 调用改为真实 Hub 或调整 import

**删除：**
- `internal/ws/hub_stub.go` — 被真实 Hub 替换
- `internal/ws/doc.go` — 占位文件

### Testing Standards

- 文件命名：`xxx_test.go` 同目录
- 多场景测试必须 table-driven（宪法）
- 单元测试使用 `t.Parallel()`
- testify：`require.NoError` / `assert.Equal`
- 集成测试：`//go:build integration` tag；使用 `httptest.Server` + gorilla/websocket client
- WS 集成测试需 `net/http/httptest`：启动 server → 拨号连接 → 发消息 → 读响应
- rate_limit 测试：验证连续请求在限速内通过、超限被拒
- Hub 测试用 mock conn 或直接测试 Register/Unregister/FindByUser 逻辑

### Previous Story Intelligence (Story 0.8)

- **Runnable 模式已验证**：Scheduler 实现了完整 Runnable 生命周期（Start/Final），Hub 同理
- **initialize.go 装配模式**：手动构造 → 传入 App Runnable 列表，顺序决定 Final 逆序
- **占位文件删除模式**：Story 0.8 删除 `internal/cron/doc.go` → 替换真实文件；本 story 同样删除 `hub_stub.go` 和 `doc.go`
- **miniredis 用于 Redis 单元测试**：WS 层不直接用 Redis（rate limit 纯内存），但如有需要可参考
- **`build.sh --test` 必须全量通过**：包括已有的 health_handler 集成测试
- **review 教训**：Start 应使用 parent ctx（不是 context.Background），cron panic 需 recovery — Hub 的 readPump/writePump 也需要 panic recovery

### Git Intelligence

最近 commit 属于 Story 0.8（Cron distributed lock）。关键模式：
- 新 Runnable 组件：构造函数 → initialize.go 装配 → App Runnable 列表
- 占位文件删除 → 真实文件替代
- `build.sh --test` 全量回归每个 story 必须通过
- review 反馈修复模式：`fix(review): 0-N round M — 摘要`

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 0.9 — AC 完整定义]
- [Source: _bmad-output/planning-artifacts/architecture.md#D1 — WebSocket Hub 结构]
- [Source: _bmad-output/planning-artifacts/architecture.md#D6 — OP-1 Hub 接口预留]
- [Source: _bmad-output/planning-artifacts/architecture.md#M15 — per-conn rate limit]
- [Source: _bmad-output/planning-artifacts/architecture.md#P3 — WS Message 命名 Convention]
- [Source: _bmad-output/planning-artifacts/architecture.md#Graceful Shutdown 顺序]
- [Source: _bmad-output/planning-artifacts/architecture.md#目录结构 — internal/ws/]
- [Source: internal/ws/hub_stub.go — 当前占位实现，本 story 替换]
- [Source: internal/handler/health_handler.go#Lines 19-21 — WSHubChecker 接口定义]
- [Source: internal/dto/error_codes.go — AUTH_INVALID_IDENTITY_TOKEN / AUTH_TOKEN_EXPIRED / UNKNOWN_MESSAGE_TYPE 已注册]
- [Source: internal/dto/error.go — AppError 类型定义]
- [Source: cmd/cat/app.go#Lines 13-17 — Runnable 接口定义]
- [Source: cmd/cat/wire.go — buildRouter / handlers struct / httpServer]
- [Source: cmd/cat/initialize.go — 当前装配代码]
- [Source: internal/config/config.go — ServerCfg / WSCfg 当前定义]
- [Source: _bmad-output/implementation-artifacts/0-8-cron-scheduler-and-distributed-lock.md — 前序 story 模式参考]

## Dev Agent Record

### Agent Model Used

Claude Opus 4.6 (1M context)

### Debug Log References

- CI `check_time_now.sh` 拦截了 hub.go 中 3 处 `time.Now()` 调用 → 注入 `clockx.Clock` 修复
- middleware flaky test `TestRequestID_InjectsIntoContextLogger` 偶现失败，为已知竞态问题，非本 story 引入

### Completion Notes List

- Envelope 类型完整：上行 Envelope / 下行 Response / 推送 Push / 心跳使用 WebSocket 协议层 ping/pong
- Broadcaster 接口 4 方法：BroadcastToUser 完整实现，BroadcastToRoom/PushOnConnect/BroadcastDiff 为 D6 预留 no-op
- Hub 使用 sync.Map + atomic.Int64 存储连接，readPump/writePump 双 goroutine 模型
- Hub 注入 clockx.Clock 避免 time.Now()，满足 M9 CI 扫描
- readPump/writePump 均有 panic recovery（吸取 Story 0.8 review 教训）
- Per-conn rate limiter 使用 golang.org/x/time/rate，100 msg/s token bucket
- Dispatcher 支持 handler 注册 + 未知 type 返回 UNKNOWN_MESSAGE_TYPE + 无效 envelope 返回 VALIDATION_ERROR
- TokenValidator 接口：debugValidator（token=userID）/ stubValidator（始终拒绝）— Story 1.3 注入真实 jwtx 实现
- Client 导出 ConnID()/UserID() 访问器供外部 handler 使用
- InMemoryBroadcaster 的 backpressure：send channel 满时关闭连接（宪法 §12）
- Graceful shutdown: Hub.Final 发 CloseGoingAway close frame + 关闭连接
- 注册顺序 mongoCli, redisCli, cronSch, wsHub, httpSrv — Final 逆序正确
- 删除 hub_stub.go + doc.go，health_handler 集成测试改用真实 Hub
- 18 个 WS 单元测试 + 3 个集成测试（//go:build integration）
- 新增依赖：gorilla/websocket v1.5.3, golang.org/x/time v0.15.0

### Change Log

- 2026-04-18: Story 0.9 实现完成 — WS Hub 骨架 + Envelope + Broadcaster 接口 + Dispatcher + UpgradeHandler + 单元测试 + 集成测试

### File List

**新建：**
- internal/ws/envelope.go
- internal/ws/broadcaster.go
- internal/ws/rate_limit.go
- internal/ws/hub.go
- internal/ws/dispatcher.go
- internal/ws/upgrade_handler.go
- internal/ws/envelope_test.go
- internal/ws/rate_limit_test.go
- internal/ws/hub_test.go
- internal/ws/broadcaster_test.go
- internal/ws/dispatcher_test.go
- internal/ws/upgrade_handler_integration_test.go
- docs/code-examples/ws_handler_example.go

**修改：**
- internal/config/config.go (ServerCfg +Mode; WSCfg +PingIntervalSec/PongTimeoutSec/SendBufSize)
- config/default.toml (server.mode + ws 新字段)
- config/local.toml (server.mode = "debug")
- cmd/cat/initialize.go (Hub/Dispatcher/Validator/UpgradeHandler 创建；替换 HubStub)
- cmd/cat/wire.go (handlers +wsUpgrade; buildRouter +/ws; +ws import)
- internal/handler/health_handler_integration_test.go (NewHubStub → NewHub + clockx)
- go.mod / go.sum (gorilla/websocket v1.5.3, golang.org/x/time v0.15.0)

**删除：**
- internal/ws/hub_stub.go
- internal/ws/doc.go
