# Story 0.4: 多维健康检查 endpoint — 实现总结

将 Story 0.2 的简单 `{"status":"ok"}` healthz 升级为多维健康探测，同时新增 readyz 就绪探针，让运维在凌晨 3 点被告警时能直接定位是 Mongo、Redis 还是 WS Hub 出了问题。

## 做了什么

### 多维 `/healthz` endpoint
- 并行执行 Mongo ping + Redis ping（`sync.WaitGroup`），读取 WS Hub goroutine 数量，读取 `cron:last_tick` Redis key
- 所有维度健康时返回 200 + `{"status":"ok","mongo":"ok","redis":"ok","wsHub":"ok","lastCronTick":"<RFC3339>"}`
- 任一维度异常返回 503，异常字段显示具体错误原因（如 `"error: connection refused"`），其余字段保持真实值
- 3 秒超时上界（`context.WithTimeout`），防止依赖卡住拖死探针

### `/readyz` 就绪探针
- 使用 `sync/atomic.Bool` 标记进程启动完成状态
- 不触发任何外部 I/O，纯内存检查
- 200 表示就绪，503 表示未就绪

### ReadySignaler 机制
- `httpServer` 改用 `net.Listen` 先绑端口，再 `close(ready)` channel 通知，最后 `Serve(ln)` 开始处理请求
- `App` 新增 `ReadySignaler` 接口，在所有实现该接口的 Runnable 就绪后才调用 `onReady()` 回调
- 保证 readyz 返回 200 时 HTTP 端口已真正 bind

### WS Hub 占位
- `internal/ws/hub_stub.go`：`HubStub` 实现 `WSHubChecker` 接口，`GoroutineCount()` 返回 0
- Story 0.9 实现真实 WS Hub 后替换

### Config 扩展
- 新增 `WSCfg.MaxConnections`（default.toml 默认 10000，对应 NFR-SCALE-4）

## 怎么实现的

**接口设计（accept interfaces, return structs）：**
- `InfraChecker` 接口：`HealthCheck(ctx) error` — `mongox.Client` 和 `redisx.Client` 自动满足
- `WSHubChecker` 接口：`GoroutineCount() int` — 占位 stub 返回 0，真实 Hub 后续替换
- HealthHandler 通过构造函数注入这些接口 + `redis.Cmdable`（读 cron key）+ `maxConn`

**并行执行：** Mongo ping 和 Redis ping 通过两个 goroutine 并行，`WaitGroup` 等待完成。WS hub count 和 Redis GET 是本地/快速操作，串行执行。

**readyz 时序保证：** Review 中发现原始实现在 Start goroutine 启动后立刻调用 onReady，HTTP 端口可能尚未 bind。修复为 httpServer 使用 `net.Listen` 先绑端口再 Serve，通过 `ReadySignaler` 接口让 App 等待。

**超时保护：** Review 中发现 healthz 未设超时，依赖卡住时探针跟着卡。添加 3 秒 `context.WithTimeout` 包裹所有依赖检查。

**WrapClient 辅助：** `mongox.WrapClient` 和 `redisx.WrapClient` 供集成测试从 Testcontainers 原始客户端创建 wrapper。

## 怎么验证的

- `bash scripts/build.sh --test` — 编译 + 全部单元测试通过
- `go vet -tags=integration ./internal/handler/...` — 集成测试编译通过
- 单元测试 8 个用例：全健康、Mongo 故障、Redis 故障、WS Hub 超限、cron tick 缺失、cron tick 存在、readyz ready、readyz not ready
- 集成测试 5 个用例（Testcontainers）：全健康、Mongo 容器停止后 503 不 panic、p95 ≤ 50ms 性能断言、readyz、cron tick 写入后读取

## 后续 story 怎么用

- **Story 0.5**（结构化日志）：healthz 日志会自动获得 requestId 字段
- **Story 0.8**（cron 调度）：heartbeat cron job 每分钟更新 `cron:last_tick` Redis key，healthz 的 lastCronTick 字段从空字符串变为真实时间戳
- **Story 0.9**（WS Hub 骨架）：替换 `ws.HubStub` 为真实 Hub 实现，在 `initialize.go` 中注入，healthz 的 wsHub 维度开始反映真实 goroutine 数量
- `InfraChecker` 和 `WSHubChecker` 接口可被其他需要健康探测的组件复用
