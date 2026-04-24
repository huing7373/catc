# ADR-0001: Server 测试 / 日志 / 指标工具栈

- **Status**: Accepted
- **Date**: 2026-04-24
- **Decider**: Developer
- **Supersedes**: N/A
- **Related Stories**: 1.1 (本决策), 1.3 (logging 中间件落地), 1.5 (测试基础设施安装依赖), 1.7 (build.sh 重做对齐 CI 命令), 1.8 (AppError 接入 `error_code` 字段), Epic 4+ (首次 DB / Redis 测试)

---

## 1. Context

当前项目处于"重启后节点 1"阶段：

- `server/` 目录为空，`go.mod` / `cmd/server` 均未建立（见 `CLAUDE.md` "重启中"章节）。
- 旧架构（MongoDB + TOML + P2 分层）已整体放弃，新方向锁定在 Gin + MySQL 8.0 + Redis + YAML（`CLAUDE.md` "Tech Stack（新方向）"）。
- 节点 1 的范围是 **"App 与 Server 可运行"** + **测试基础设施搭好**，但不接 MySQL / Redis / auth 中间件（遵循 `docs/宠物互动App_MVP节点规划与里程碑.md` §2 原则 7 "基础设施按需引入"与原则 8 "测试基础设施必须在节点 1 内建立"）。
- 后续 Epic 的业务测试 / logging 中间件 / metrics 埋点都需要一套**预先锁定、不再争议**的工具栈；否则每个 epic 自己拼凑 → 风格漂移。

本 ADR 的目的：**一次性锁定 6 类工具 + 结构化日志字段 + NFR16 指标位 + 版本清单**，覆盖 Epic 1 到 Epic 36 全部测试 / 日志 / 指标相关选型。

**本 ADR 不产出任何 `.go` 代码、不修改 `go.mod`、不建立 `server/` 目录**（Story 1.1 AC6 强制约束）。实际安装与代码落地由下游 story 承担（Story 1.2 建立 `go.mod`，Story 1.5 安装测试依赖，Story 1.3 落 logging 中间件）。

---

## 2. Decision Summary

| 领域 | 选定 | 版本 |
|---|---|---|
| DB / 缓存 Mock | `sqlmock` + `miniredis` 组合（主），`dockertest` 保留给 Epic-4 之后的 layer-2 集成测试 | sqlmock v1.5.2 / miniredis v2.37.0 / dockertest v3.12.0 |
| HTTP Handler 测试 | `net/http/httptest` + `gin.Engine.ServeHTTP`（stdlib，Gin 无独立 test helpers 库） | stdlib |
| 断言库 | `testify/assert` + `testify/require` 混用 | testify v1.11.1 |
| 接口 Mock（service/repo 边界） | `testify/mock`（手写，不引入 codegen） | testify v1.11.1 |
| CI 跑法 | `go test -race -cover ./...` 全量 + coverage 阈值通过 `bash scripts/build.sh --test` 对齐（Story 1.7 重做时预留 `--race` / `--coverage` 开关） | stdlib |
| Logger | `log/slog`（Go 1.21+ stdlib），自定义 JSON Handler | stdlib (Go 1.22+) |
| Metrics | `github.com/prometheus/client_golang`（拉取式，暴露 `/metrics`） | client_golang v1.23.2 |

---

## 3. Decisions

### 3.1 DB / 缓存 Mock 方案

- **选定**：`sqlmock` + `miniredis` 组合（单元 / service 层主力），`dockertest/v3` 仅在 Epic-4 之后的"layer-2 集成测试"场景引入（如 `4-7` / `11-9` / `20-9` / `26-5` / `32-5`）。
- **理由**：
  1. **速度**：sqlmock + miniredis 都是纯 Go in-process，单测用例启动无感（<10ms），`go test -race ./...` 可在开发机上秒级完成。dockertest 每次启动 MySQL/Redis 容器约 5-10s，放进 `go test ./...` 会让 TDD 节奏崩。
  2. **零外部依赖**：miniredis 覆盖 Redis 90%+ 命令（足够 `idem:*` key + TTL 设置 + `presence:*` 用法），sqlmock 模拟 `database/sql` driver；两者都不需要 Docker / CI runner 带 daemon，CI 成本低。
  3. **与新架构契合**：`docs/宠物互动App_Go项目结构与模块职责设计.md` §14 建议分层（单元 / service / 集成），sqlmock+miniredis 正好覆盖前两层；layer-2 用 dockertest 保真度高，让真实 MySQL 8.0 驱动与 GORM SQL 对接跑通。
  4. **按需引入原则**：本 spike 只决定"工具栈"，第一条 SQL 测试要到 Epic 4 Story 4-7 才真正落地；现在 pin 住版本、Story 1.5 装依赖、例子文件在 1.5 示范 AR27 mock 单测即可。
- **否决候选**：
  - **纯 sqlmock（不带 miniredis）**：否决 — 后续 Epic 10 / Epic 17 的 ws 表情广播、Epic 20 的 idempotencyKey 都依赖 Redis，没有 Redis mock 会逼集成测试全部升级到 dockertest，节奏崩。
  - **纯 dockertest（不带 sqlmock）**：否决 — 单测阶段启动容器过慢，TDD 循环痛苦；而且单测本该剥离 IO，用真容器违背测试金字塔。
- **已知坑**：sqlmock 对 GORM 自动生成的 SQL 用正则匹配较脆（复杂 JOIN / UPSERT 容易写错 `ExpectPrepare` regex）。**缓解措施**：Story 1.5 在 `docs/architecture` 下留一份"sqlmock + GORM 正则写法 cheatsheet"；Story 4-7 layer-2 集成测试必须用 dockertest（不是 sqlmock）。

### 3.2 HTTP Handler 测试方案

- **选定**：`net/http/httptest` + `gin.Engine.ServeHTTP(w, req)`。
- **理由**：
  1. **Gin 没有独立 test helpers 库**：Gin 官方文档推荐的 pattern 就是 `httptest.NewRecorder()` + `router.ServeHTTP()`，这是 stdlib + Gin 原生接口组合，不是额外的库。**本决策文档明确澄清这一点**，避免后续 dev 去找"Gin test helpers 库"导致版本选型浪费时间。
  2. **stdlib，零依赖**：`net/http/httptest` 从 Go 1.0 起就稳定，和未来 Go 版本升级完全兼容。
  3. **与 testify/require 天然匹配**：`httptest.ResponseRecorder.Body.Bytes()` + `require.JSONEq` / `assert.Equal` 构成 handler 层测试的标准模式。
- **否决候选**：
  - **testify mock（用作 HTTP 层）**：否决 — testify/mock 是接口 mock 库（service 层 repo mock 用），不是端点测试工具；方向错误。
  - **"Gin 自带 test helpers"作为独立选项**：否决 — 该库不存在，只是 stdlib 的组合模式。

### 3.3 断言库

- **选定**：`github.com/stretchr/testify`（`assert` + `require` 混用）。
- **理由**：
  1. **前置条件用 `require`（fail-fast）**：如果 setup 阶段 `require.NoError(t, err)` 失败，后续断言没有意义，立即失败省掉 noise。
  2. **单元 case 用 `assert`（continue-on-fail）**：一次 `go test -run` 能看到多个断言的失败情况，排障效率高。
  3. **生态配套**：testify 的 mock / suite / assert / require 四件套统一，后续 interface mock（见 §3.4）也复用 testify/mock，依赖收敛。
  4. **全 Go 社区事实标准**：Gin、GORM、sqlmock、dockertest 官方示例都用 testify，新 dev 上手零摩擦。
- **否决候选**：
  - **stdlib `testing`（`if got != want { t.Errorf(...) }`）**：否决 — 样板代码多，失败信息不带 diff，维护期可读性差；本项目 Epic 20/32 等事务测试断言复杂度高，stdlib 不够用。

### 3.4 接口 Mock（service ↔ repo 边界）

- **选定**：`github.com/stretchr/testify/mock`（手写 `MockXxxRepo` struct，实现 repo interface）。
- **理由**：
  1. **零 codegen**：不引入 `mockgen` / `mockery` CLI 工具，不污染 CI 环境、不增加 `go generate` 步骤。
  2. **可读性**：手写 mock 一眼能看出"这个测试期望 repo 被调哪几次、返回什么"，比 codegen 生成的大文件更易 code review。
  3. **依赖收敛**：已经引入 testify 做断言，复用 `testify/mock` 零新增。
- **否决候选**：
  - **`go.uber.org/mock`（原 `golang/mock`）**：否决 — 需要 `mockgen` 二进制 + `//go:generate` 注释，CI 要装 tool，增加维护环节。本项目 repo 接口数量不多（按 `Go项目结构设计.md` §4 约 8-10 个），手写成本可控。
  - **`github.com/vektra/mockery`**：否决 — 同样是 codegen 工具（生成 testify/mock 风格），手写已经能达到等价效果，不需要额外工具。

### 3.5 CI 跑法

- **选定**：`go test -race -cover ./...` 全量 + 通过 `bash scripts/build.sh --test` 作为本地与 CI 的统一入口。
- **理由**：
  1. **全量而非分目录**：当前 `server/` 模块扁平（见 `Go项目结构设计.md` §4：`internal/{app,domain,service,repo,infra,pkg}/`），全量跑 race + cover 在开发机上时间可控（<30s 预期）；分目录拆分会隐藏跨模块 race。
  2. **本地与 CI 一致**：`build.sh --test` 是 `CLAUDE.md` 已约定的统一入口，Story 1.7 重做时必须保留 `--test` / `--race` / `--coverage` 开关 → CI 脚本直接调 `bash scripts/build.sh --test --race --coverage` 即可。
  3. **race 必开**：Epic 10 之后的 ws 广播 / Epic 20 的 idempotencyKey 并发都涉及 goroutine，`-race` 必须开在 CI，尽早暴露 data race。
- **Story 1.7 落地要求**（对 build.sh 重做的 contract）：
  - `bash scripts/build.sh` → vet + build（默认）
  - `bash scripts/build.sh --test` → 加跑 `go test ./...`
  - `bash scripts/build.sh --test --race` → 加 `-race`
  - `bash scripts/build.sh --test --coverage` → 加 `-cover -coverprofile=...`
  - 老的 `cmd/cat` / `docs/api/openapi.yaml` / `check_time_now.sh` 残留必须移除（`CLAUDE.md` TODO 已列）。
- **否决候选**：
  - **按目录拆分跑**：否决 — 早期阶段收益不明显，反而隐藏跨包 race / 跨包 import cycle。等到 epic 30+ 模块多且 CI 时间 > 5 分钟再考虑矩阵拆分。

### 3.6 Logger 选型

- **选定**：`log/slog`（Go 1.21+ stdlib），自定义 `slog.Handler` 输出 JSON。
- **理由**：
  1. **零外部依赖**：`log/slog` 是 stdlib（Go 1.21+ 进入），升级 Go 版本即升级 logger，不存在 "zap 突然停止维护" 的风险。
  2. **结构化字段灵活度够**：§AC2 要求的 `request_id` / `user_id` / `api_path` / `latency_ms` / `business_result` / `error_code` 六字段，`slog.Attr` 完全覆盖，且可用 `logger.With(slog.String("request_id", rid))` 绑定 context logger，Story 1.3 logging 中间件可直接 `ctx = slog.NewContext(ctx, withReqID)` 下沉到 service 层。
  3. **性能可接受**：slog 比 zap 在极端场景下慢约 2-3x，但本应用 QPS 不属于高频场景（节点 10 上线 WS 广播后单实例预计 <1000 msg/s），slog 开销在总延迟中 <1%。
  4. **长期稳定**：Go 团队承诺 stdlib 向后兼容，工具升级路径清晰。
  5. **Handler 可插拔**：`slog.Handler` 接口允许未来换实现（如需要接 OTel 日志通道时加一层 adapter）而不改调用方。
- **否决候选**：
  - **`go.uber.org/zap`**：否决 — 性能最优但需外部依赖；本项目 QPS 场景不需要零分配；zap 的 `SugaredLogger` / `Logger` 双 API 会诱导 dev 选错（Sugared 方便但性能差 2-3x，强类型 API 样板多）。保留迁移路径：slog Handler → zap adapter 如将来性能压测证明瓶颈。
  - **`github.com/rs/zerolog`**：否决 — 零分配 JSON-first，但非 JSON 输出（本地开发 pretty print）支持弱；社区 gin adapter 不如 slog / zap 完整。
- **结构化字段落地**：见本文档 §4，Story 1.3 logging 中间件按该表实装。

### 3.7 Metrics 选型

- **选定**：`github.com/prometheus/client_golang`（拉取式，HTTP `/metrics` endpoint 暴露）。
- **理由**：
  1. **事实标准**：Prometheus 是 K8s 生态默认监控方案，Grafana / Alertmanager 全部原生适配，后续运维无缝接入。
  2. **Gin 集成成熟**：社区有 `gin-contrib/gin-prometheus`（或类似轻量中间件）可直接对 latency / QPS 打点；自行实现也简单（中间件在 `c.Next()` 前后各一次 `time.Now()` + Counter/Histogram Inc/Observe）。
  3. **指标类型完备**：§AC3 的 7 类指标（Counter / Gauge / Histogram）client_golang 原生支持；`promauto.NewCounterVec` / `promauto.NewHistogramVec` / `promauto.NewGaugeFunc` 都能对应。
  4. **拉取式适合单体部署**：本项目是模块化单体，Prometheus server 定期 scrape `/metrics` 即可，不需要 push gateway。
  5. **成熟稳定**：client_golang v1.23.2 已发布，API 锁定多年，无大变动风险。
- **否决候选**：
  - **OpenTelemetry Go SDK**：否决 — 统一 traces+metrics+logs 是未来方向，但 2026-04 当前 Go SDK 的 metrics API 仍在演进（v1 稳定但生态中间件少），MVP 阶段增加学习 + 集成成本不值。保留未来迁移路径：prometheus exporter 可以被 OTel collector 消费，切换是运维侧而非代码侧决策。
  - **Vector**：否决 — 分类错误。Vector 是 Rust 写的部署侧 agent（用于日志 / 指标聚合转发），不是 Go 应用侧 metrics 库。即使最终部署引入 Vector 做 pipeline，应用侧仍需要一个 Go metrics 库，两者不是互斥关系。

---

## 4. Structured Log Fields（§AC2 落地）

> Logger 实装参考：Story 1.3（logging 中间件）按本表注入；Story 1.8（AppError）提供 `error_code` 字段。

| 字段 | 类型 | 来源 | 生效时机 / 说明 |
|---|---|---|---|
| `request_id` | string | middleware 注入 | **Epic 1 Story 1.3 生效**。logging 中间件生成（优先读 `X-Request-Id` header，缺省用 UUIDv7 / ULID 生成），存入 `context.Context`，handler / service / repo 层通过 `slog.With` 携带。 |
| `user_id` | string | auth 中间件上下文 | **Epic 4 Story 4.5 生效**（auth 中间件落地时）。节点 1 到 Epic 4 之前**预留字段**，匿名请求（如 `/ping`）输出 `""` 或省略该字段。 |
| `api_path` | string | `c.FullPath()` | **Epic 1 Story 1.3 生效**。必须用 Gin 路由 pattern（如 `/rooms/:id`）而非 raw URL（如 `/rooms/123`），避免 metric cardinality 爆炸 + 日志聚合失效。 |
| `latency_ms` | int64 | logging 中间件计时 | **Epic 1 Story 1.3 生效**。毫秒单位（`time.Since(start).Milliseconds()`），保留 int64 类型避免浮点精度。 |
| `business_result` | string | service 主动写入 | **按 service 实装逐步生效**。用 snake_case 小写枚举值（如 `chest_opened` / `chest_open_not_ready` / `login_created` / `login_reused` / `compose_success` / `compose_failed` / `room_joined` / `room_full`）。由各 domain service 在关键路径末尾调 `slog.InfoContext(ctx, "business_event", slog.String("business_result", "..."))`。 |
| `error_code` | int | `*AppError.Code` | **Epic 1 Story 1.8 生效**。AppError 框架建立后，logging 中间件在 `c.Errors` 非空时读第一个 AppError 的 `.Code` 字段写入；成功请求省略该字段。 |

**补充约束**：
- 所有日志通过 `slog.InfoContext(ctx, msg, ...)` / `slog.ErrorContext(ctx, msg, ...)` 调用；**禁止** `fmt.Println` / `log.Println` / 裸 `slog.Info`（丢 context）。
- Story 1.3 logging 中间件会把 `request_id` / `user_id` / `api_path` / `latency_ms` 以 `ctx` 形式绑定到 child logger；service 层只需 `logger := slog.FromContext(ctx)` 即可继承。
- Story 1.5 测试基础设施安装 `slog.Handler` fixture（如 `slogtest.TestHandler` 或自定义 in-memory handler）用于断言日志字段。

**来源**：`docs/宠物互动App_Go项目结构与模块职责设计.md` §13.1（日志最少字段要求）+ Story 1.1 AC2（字段命名与类型约定）。

---

## 5. Metrics Reserved Slots（§AC3 落地）

> **本 ADR 只锁定指标位**，**不**注册任何 prometheus Collector。真实注册由下表"接入 Epic"列标明的 story 承担。

| 指标名 | 类型 | 标签维度 | 注册点 | 更新点 | 接入 Epic |
|---|---|---|---|---|---|
| `cat_api_requests_total` | Counter | `api_path`, `method`, `code` | `internal/infra/metrics/registry.go`（Story 1.5 预留骨架） | logging 中间件在 `c.Next()` 后、handler 返回前递增（与 `request_id` 日志同一位置） | Epic 1 Story 1.3 |
| `cat_api_request_duration_seconds` | Histogram | `api_path`, `method` | 同上 | logging 中间件末尾 `Observe(time.Since(start).Seconds())`；buckets 建议 `{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2.5, 5}` 秒 | Epic 1 Story 1.3 |
| 错误率 | 由 Counter 派生 | `api_path`, `code` | — | 错误率不单独注册 metric，用 PromQL `rate(cat_api_requests_total{code=~"5.."}[5m]) / rate(cat_api_requests_total[5m])` 在 Grafana 侧派生 | N/A（Grafana 面板侧） |
| `cat_chest_opened_total` | Counter | `rarity` | `internal/domain/chest/metrics.go`（Story 20-6 落地） | service 层 `OpenCurrentChest` 成功事务提交后递增；`rarity` 取本次开出的 cosmetic rarity 枚举值 | Epic 20 Story 20-6 |
| `cat_compose_success_total` | Counter | `from_rarity`, `to_rarity` | `internal/domain/compose/metrics.go`（Story 32-4 落地） | service 层 `ComposeUpgrade` 成功事务提交后递增 | Epic 32 Story 32-4 |
| `cat_rooms_active` | Gauge | — | `internal/domain/room/metrics.go`（Story 11-3 / 11-4 / 11-5 落地） | 创建房间 `+1`，退出时房间关闭 `-1`；或 Story 10-6 的 Redis presence repo 定期 `SetToCurrentTime`（推荐后者，避免进程重启计数飘） | Epic 10 / Epic 11 |
| `cat_ws_connections_active` | Gauge | — | `internal/app/ws/metrics.go`（Story 10-3 / 10-4 落地） | WS 连接建立 `+1`、断开 `-1`（`defer` 保证断开路径覆盖） | Epic 10 Story 10-3 |

**补充约束**：
- 全部指标以 `cat_` 前缀命名，避免与 Go runtime / process metrics 碰撞。
- 标签基数（cardinality）控制：`api_path` 用 Gin `c.FullPath()`（模式化，基数 = 路由数 ≈ 30），**禁止**用 raw URL 作标签；`user_id` **不进 metric 标签**（爆炸风险）。
- `/metrics` 端点暴露路径：`GET /metrics`，Story 10-3 之后考虑是否独立端口（监控 port 与业务 port 分离），节点 1 阶段共用主 HTTP server 即可。
- Story 1.5 建立 `internal/infra/metrics/` 骨架包（含 `Registry` 全局变量 + `Register(c Collector)` 辅助），Epic 20 / 32 落地时用 `promauto` 或手动 `Registry.MustRegister`。

**来源**：`docs/宠物互动App_Go项目结构与模块职责设计.md` §13.3（7 类指标最小集）+ `docs/宠物互动App_总体架构设计.md` §6（单体架构与部署约定）。

---

## 6. Version Lock

下表是 Story 1.5 安装测试依赖、Story 1.2 建立 `go.mod` 时必须 **pin 住** 的版本（不用 `latest`）。版本号是 2026-04-24 当前稳定版：

```go.mod
require (
    // HTTP 框架（Story 1.2 落地；非本 ADR 决策范围但一并锁定）
    github.com/gin-gonic/gin v1.12.0

    // 断言 + 接口 mock
    github.com/stretchr/testify v1.11.1

    // DB / 缓存 Mock
    github.com/DATA-DOG/go-sqlmock v1.5.2
    github.com/alicebob/miniredis/v2 v2.37.0

    // Layer-2 集成测试（Epic 4+ 才 import；Story 1.5 可先装）
    github.com/ory/dockertest/v3 v3.12.0

    // Metrics
    github.com/prometheus/client_golang v1.23.2
)
```

**说明**：
- **Logger**：`log/slog` 是 stdlib（Go 1.21+），**不占 go.mod require 行**。Go 版本要求 `go 1.22` 及以上（Story 1.2 在 `go.mod` 顶部声明 `go 1.22`）。
- **HTTP handler 测试**：`net/http/httptest` 是 stdlib，不占 require 行。
- **Gin 版本**：虽非本 ADR 6 大轴中的决策项，但 CLAUDE.md Tech Stack 已锁定 Gin，这里统一 pin 住主版本（v1.12.0 是 2026-02-28 发布的稳定版），避免 Story 1.2 落地时再讨论。
- **升级策略**：所有版本每季度评估一次，升级需走新 ADR（最小变更：直接 append 到本 ADR 的 Change Log 小节即可，无需新建文档）。
- **`latest` 禁令**：Story 1.2 / 1.5 的 `go get` 命令必须带 `@vX.Y.Z` 指定版本，`go mod tidy` 后 commit `go.sum`。

---

## 7. Consequences

### Positive

- **选型争议清零**：Epic 1-36 的测试 / 日志 / 指标实装直接照搬本 ADR，不再每个 story 重新讨论。
- **依赖可控**：外部库仅 5 个（gin + testify + sqlmock + miniredis + dockertest + client_golang），加 stdlib logger，总依赖树浅，安全审计成本低。
- **可测试性**：单元 / service 层全部 in-process（sqlmock + miniredis），`go test -race ./...` 秒级反馈；layer-2 集成用 dockertest 有保真度。
- **可观测性**：logger 6 字段 + metrics 7 指标位一次锁死，Story 1.3 / 1.8 / Epic 10-32 落地时照搬，Grafana 面板可提前设计。

### Negative / Accepted Trade-offs

- **slog 性能不如 zap**：接受 — 本项目 QPS 不属高频；future 可以加 adapter 层切到 zap 而不改调用方。
- **sqlmock 正则脆性**：接受 — Story 1.5 留 cheatsheet，复杂 SQL 用 dockertest。
- **手写 testify/mock 样板**：接受 — repo 接口数量可控（<15 个），换来 CI 无 codegen 工具依赖。
- **prometheus 非统一可观测**：接受 — OTel 迁移路径保留（exporter 侧切换），应用代码可以在若干年后无痛迁移。

### Follow-ups（按 story 分工）

- **Story 1.2**：建立 `server/go.mod`，顶部 `go 1.22`，`require` 段按 §6 版本清单初始化（Gin 必装，其他测试/metrics 库可由 Story 1.5 补）。
- **Story 1.3**：logging 中间件按 §4 字段表实装，用 `log/slog` + 自定义 JSON Handler；同步注册 §5 中 Epic 1 阶段的两个 metrics（`cat_api_requests_total` + `cat_api_request_duration_seconds`）。
- **Story 1.5**：测试基础设施目录（`server/internal/infra/testing/` 或类似）安装 testify + sqlmock + miniredis；提供 `NewMockDB()` / `NewMiniRedis()` helper；写一个示例单测（如 `ping_test.go`）证明工具栈跑通。
- **Story 1.7**：重做 `scripts/build.sh`，对齐 §3.5 的 `--test` / `--race` / `--coverage` 开关；移除旧 `cmd/cat` / `docs/api/openapi.yaml` / `check_time_now.sh` 残留。
- **Story 1.8**：AppError 类型建立时，确保 `*AppError.Code` 字段可被 logging 中间件消费（§4 的 `error_code` 来源）；AppError 框架占用 decision 文档编号 `0006-error-handling.md`。
- **Story 1.9**：context 传播框架占用 decision 文档编号 `0007-context-propagation.md`，与本 ADR 的 `ctx` 使用约定（`slog.InfoContext`）对齐。
- **Epic 10 / Epic 20 / Epic 32**：按 §5 实际注册对应 domain metrics。

---

## 8. Change Log

| Date | Change | By |
|---|---|---|
| 2026-04-24 | 初稿，Story 1.1 交付 | Developer |
| 2026-04-24 | Story 1.5 落地：装 testify v1.11.1 / sqlmock v1.5.2 / miniredis v2.37.0（按 §6 版本锁定）；dockertest **延迟到 Epic 4 Story 4-7**（首次 layer-2 集成测试场景引入，避免 `go mod tidy` 剔除未 import 的包）；sqlmock+GORM cheatsheet 延迟到 Epic 4 Story 4-2（GORM 落地后再新建 `0002-sqlmock-gorm-cheatsheet.md`）。本机 Windows Go install 缺 `race_windows_amd64.syso` + 下载的 toolchain 缺 `covdata.exe`，`go test -race -cover` 无法本地跑；按本 ADR §3.5 原则归 CI（Linux runner）执行，Story 1.7 重做 `scripts/build.sh` 时预留 `--race` / `--coverage` 开关。 | Developer |
