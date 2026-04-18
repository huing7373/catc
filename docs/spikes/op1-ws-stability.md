# Spike-OP1 — watchOS WS-primary 稳定性报告

> **Status:** Phase A（服务端脚手架）完成 @ 2026-04-18；Phase B（真机执行 + Hub 压测执行 + 收敛决策 + 签字）由 **Story 9.1** 承载，待架构师手动执行。
>
> **本报告是工程决策归档，不是产品文档。** 以数据为先，结论为后；避免 marketing 腔。
>
> **源头锚点：**
> - `server/_bmad-output/planning-artifacts/epics.md` §Story 0.15 行 727-747（AC 原文）
> - `server/_bmad-output/planning-artifacts/architecture.md` §D6（候选方向）§D9 ADR-003（Hub 压测触发）§Spike-OP1 依赖 行 493
> - `docs/backend-architecture-guide.md` §12 WebSocket / §19 PR checklist
> - `server/_bmad-output/implementation-artifacts/0-15-spike-op1-watchos-ws-primary-stability-test-matrix.md`（Story 文件 Phase A）
> - `server/_bmad-output/implementation-artifacts/9-1-spike-op1-real-device-and-hub-loadtest-execution.md`（Story 文件 Phase B）

---

## 1. Pre-flight Checklist

**执行日期：** 2026-04-18
**执行人：** Claude Opus 4.7（dev-story workflow）
**验证方式：** 读 `internal/ws/*.go` + `internal/dto/ws_messages.go` + 跑 `bash scripts/build.sh --test` 确认无回归。

| # | 依赖项 | 来源文件 | 符号/锚点 | 状态 | 备注 |
|---|---|---|---|---|---|
| 1 | `HubConfig.MaxConnections` | `internal/ws/hub.go` | 行 19 | ✅ | Story 0.9；已在 cfg `[ws].max_connections = 10000` 消费 |
| 2 | `Hub.GoroutineCount()` | `internal/ws/hub.go` | 行 61-63 | ✅ | Story 0.9；healthcheck + AC6 压测需要 |
| 3 | `UpgradeHandler` 建连路径 | `internal/ws/upgrade_handler.go` | 行 73-158 | ✅ | Story 0.11；含限流 + 黑名单 fail-closed 语义 |
| 4 | `ConnectRateLimiter` 滑动窗口 | `internal/ws/conn_guard.go` + `pkg/redisx` | 行 `AcquireConnectSlot` | ✅ | Story 0.11；`connect_rate_per_window=5 / 60s` |
| 5 | `Blacklist` 设备黑名单 | `internal/ws/conn_guard.go` + `pkg/redisx` | `IsBlacklisted` | ✅ | Story 0.11；fail-closed on Redis error |
| 6 | `ResumeCache` + `ResumeCacheInvalidator` | `internal/ws/session_resume.go` | 行 64-82 | ✅ | Story 0.12；60s Hash cache + singleflight |
| 7 | `session.resume` handler 在 debug 模式注册 | `cmd/cat/initialize.go` 行 106-115 | `sessionResumeHandler` | ✅ | Story 0.12；debug-only（AC1 要求 spike 全程 debug） |
| 8 | `Dispatcher.RegisterDedup` 中间件 | `internal/ws/dedup.go` 行 60-193 + `dispatcher.go` 行 56-65 | `dedupMiddleware` | ✅ | Story 0.10；长度前缀 key `scopedDedupKey` 防歧义 |
| 9 | `dto.WSMessages` registry | `internal/dto/ws_messages.go` 行 49-77 | 3 条：`session.resume` / `debug.echo` / `debug.echo.dedup` | ✅ | Story 0.14；`validateRegistryConsistency` 双 gate 生效 |
| 10 | `Dispatcher.RegisteredTypes()` drift 守门 | `internal/ws/dispatcher.go` 行 79-86 | `RegisteredTypes` | ✅ | Story 0.14；`cmd/cat/initialize.go` 行 158 fail-fast |
| 11 | APNs Pusher + 队列（电量基线对照可选） | `internal/push/` | `apnsWorker` | ✅（可选） | Story 0.13；AC5 电量基线用 |
| 12 | `clockx.Clock` + `FakeClock` | `pkg/clockx/` | `RealClock` / `FakeClock` | ✅ | Story 0.7；客户端侧可注入 Fake；服务端 Real |
| 13 | cron `withLock` 分布式锁 | `internal/cron/` | `withLock` | ✅ | Story 0.8；Hub 压测验证 cron 不阻塞 |
| 14 | `/ws` 路由 | `cmd/cat/wire.go` 行 35 | `r.GET("/ws", h.wsUpgrade.Handle)` | ✅ | Story 0.9 落点 |
| 15 | `/v1/platform/ws-registry` 路由 | `cmd/cat/wire.go` 行 34 | `h.platform.WSRegistry` | ✅ | Story 0.14；压测客户端可用它探活 |

**Pre-flight 结论：** 全部 ✅，Stories 0.9–0.14 闭合，可进入 Phase B 真机测试 + 压测执行。

**构建状态：** `bash scripts/build.sh --test` 于 2026-04-18 通过（`go vet` + `time.Now()` M9 检查 + 全部 package 测试绿；commit: see `git rev-parse HEAD`）。

---

## 2. Test Tool Selection

**决策日期：** 2026-04-18
**决策人：** Claude Opus 4.7（ADR 候选者依据 AC7 列出；复盘人：架构师 on Phase B）
**结论：** **自写 Go 客户端**（`server/tools/ws_loadgen/main.go`）。

### 评估表

| 候选 | WS 协议原生支持 | 分布式生成能力（MVP） | 与现有 Go 代码互操作 | 学习成本 / 依赖风险 | 决策 |
|---|---|---|---|---|---|
| **vegeta** | ✗ 仅 HTTP | N/A | N/A | 低 | **否决** — HTTP-only，不适合 WS |
| **k6** | ✅ `k6/ws` 模块 | ✅ k6 cloud / 多 runner | ⚠ JS 侧代码，要手工抄常量 | 中（引入 JS runtime + 新容器） | **否决** — 依赖引入风险 > 收益 |
| **自写 Go 客户端** | ✅ `gorilla/websocket`（已在 `go.mod`） | MVP 单机足够 | ✅ 直接 `import "github.com/huing/cat/server/internal/dto"` 拿消息常量 | 低（不引新依赖） | **采纳** |

### 采纳自写 Go 客户端的四条理由

1. **常量来源统一**：`import dto.WSMessagesByType["debug.echo"]` 直接复用 Story 0.14 registry。避免 JS/外部工具手抄 `"debug.echo"` 字符串 → Story 0.14 AC4 drift 保护失效。
2. **时钟可控**：需要 deterministic 重连间隔时，可注入 `pkg/clockx.FakeClock`（M9：服务端依然 RealClock）。
3. **无新依赖**：MVP 只有 Go 后端 + Watch/iPhone 客户端，引入 JS runtime 属过度设计（`backend-architecture-guide.md` §依赖边界）。
4. **可抛弃**：`tools/` 目录本就是 one-off 脚本归宿（architecture §Project Structure 行 972-973）。Spike 完成后工具可留作回归、也可删除，不进 `internal/`。

### 切换触发条件

若 Phase B 执行期发现自写 Go 客户端在 N ≥ 5k 并发下遇到不可克服的 CPU 瓶颈，允许切到 k6。切换决策**必须**在本节末尾追加：

```
SWITCHED: YYYY-MM-DD <HH:MM> — reason: <具体瓶颈 + 复现命令>
```

未触发即留白。

### 工具交付

- **文件：** `server/tools/ws_loadgen/main.go`（可选 `server/tools/ws_loadgen/README.md`）
- **flags：** `-url`, `-concurrent`, `-duration`, `-send-interval`, `-scenario={cold_connect|raise_wrist|long_lived}`, `-report=path.json`, `-token-prefix`, `-verbose`
- **smoke 验收：** `go run ./tools/ws_loadgen -url ws://127.0.0.1:8080/ws -concurrent 10 -duration 60s -scenario long_lived -token-prefix loadgen-`
- **输出 JSON summary 字段：** `scenario / concurrent / durationMs / connectLatencyMs{p50,p95,p99} / echoRttMs{p50,p95,p99} / connectSuccess / connectFailures / reconnectAttempts / echoSamples / errors`

### 工具度量范围（重要）

本工具度量的是"dial + WS upgrade + `debug.echo` RTT"—— **hub 侧**的连接握手 + 回显延迟。这**不是** §5 所定义的 `connectColdMs*` / `reconnectRaiseWristMs*`（后者需要真机 Watch 发 `session.resume`，走 provider 链路）。因此：

- ✅ **适用场景：** §7 Hub 压测（AC6 / ADR-003）—— `long_lived` scenario 的 `connectLatencyMs` + `echoRttMs` 是 hub 稳态容量的直接度量
- ❌ **不适用场景：** §5 cell 表的 `connectColdMs*` / `reconnectRaiseWristMs*` —— 这些列只能由真机 Watch 客户端遥测写出

**`long_lived` 的 `reconnectAttempts` 指标（round 2 review 修正后口径）：** 工具在 read/write/echo 错误时会自动重连并保持 N 不塌陷（round 1 review 修复）。`reconnectAttempts` 只在 worker 已经有过一次成功会话之后才累加（启动期反复 dial 失败的初次建连重试**不**算 reconnect；见 `Summary.ReconnectAttempts` godoc）。**稳态判据（正确分母）：**

```
reconnectRatio = ReconnectAttempts / Config.Concurrent
```

- 语义："每 worker 在本次运行中的平均重连次数"
- 阈值：`≤ 0.05`（平均不到每 20 个 worker 一次重连）→ 稳态；`> 0.05` → 非稳态 ⚠，p95/p99 读数不能作为 `broadcastLatencyP99 ≤ 3s` gate 的直接输入
- **不要**用 `ReconnectAttempts / ConnectSuccess` —— 分母 `ConnectSuccess` 自身包含了所有成功的重连，会系统性稀释 ratio 约 `R/(N+R)`（round 1 原文档的缺陷，round 2 修正）

---

## 3. Test Matrix

> **Phase B TODO**（Story 9.1 承载）— 架构师执行真机测试后填本节。

**矩阵：** 4 网络 × 3 场景 = 12 cells，全部执行。弱网强制 macOS Network Link Conditioner（**不得**用 tc netem 服务器侧）。每 cell ≥ 30 min 持续，不重启客户端。

| cell ID | 网络 | 场景 | 执行时间 | 通过/失败 | 主观备注 |
|---|---|---|---|---|---|
| cell 1 | Wi-Fi | 冷启动建连 | TODO | TODO | TODO |
| cell 2 | Wi-Fi | 抬腕循环重连 | TODO | TODO | TODO |
| cell 3 | Wi-Fi | 长连持续心跳 | TODO | TODO | TODO |
| cell 4 | 4G LTE | 冷启动建连 | TODO | TODO | TODO |
| cell 5 | 4G LTE | 抬腕循环重连 | TODO | TODO | TODO |
| cell 6 | 4G LTE | 长连持续心跳 | TODO | TODO | TODO |
| cell 7 | 弱网（丢包 10% ± 2%） | 冷启动建连 | TODO | TODO | TODO |
| cell 8 | 弱网（丢包 10% ± 2%） | 抬腕循环重连 | TODO | TODO | TODO |
| cell 9 | 弱网（丢包 10% ± 2%） | 长连持续心跳 | TODO | TODO | TODO |
| cell 10 | 网络切换（BT↔LTE 每 60s） | 冷启动建连 | TODO | TODO | TODO |
| cell 11 | 网络切换（BT↔LTE 每 60s） | 抬腕循环重连 | TODO | TODO | TODO |
| cell 12 | 网络切换（BT↔LTE 每 60s） | 长连持续心跳 | TODO | TODO | TODO |

**采样量：**
- 建连场景：重复 ≥ 30 次取 p50/p95/p99
- 重连场景：模拟抬腕/落腕 ≥ 60 次
- 长连场景：单次 30 min 观察 1 条 timeseries

**通过判据：** `NFR-REL-4` 达成 — `reconnectWithin5sRate ≥ 98%` 在 ≥ 10/12 cells 成立。弱网 + 网络切换 cells 若低于 98% 需在 §6 解释。

---

## 4. Device & Environment

> **Phase B TODO** — 架构师填本节。

**设备清单（真机强制）：**

| 项 | 要求 | 实测值 |
|---|---|---|
| Apple Watch 型号 + 代数 | 实体（禁 simulator） | TODO |
| watchOS 版本 | ≥ 10.0 | TODO |
| 配对 iPhone 型号 | 实体 | TODO |
| iOS 版本 | ≥ 17.0 | TODO |
| 服务器 CPU 型号 | M1/M2/Intel 任一 | TODO（`sysctl -n machdep.cpu.brand_string` 或 WMI） |
| 服务器 RAM | ≥ 8 GB | TODO |
| Go 版本 | `go version` | TODO |
| 服务器 commit hash | `git rev-parse HEAD` | TODO |
| Network Link Conditioner profile | "3G / 100% Loss" 或自定义 10% loss | TODO |

**Clock 来源（M9 守门）：**
- 服务端：`clockx.RealClock`（必须）
- 压测客户端（ws_loadgen）：默认 `clockx.RealClock`；可选 `-fake-clock` 注入 `FakeClock` 用于 deterministic 重连间隔
- Watch/iPhone 客户端：设备系统时钟（不受 spike 控制）

---

## 5. Latency & Reliability Metrics

> **Phase B TODO** — 架构师执行 12 cells 后填表。
>
> **⚠️ 数据来源必须是真机 Watch 客户端遥测**：`connectColdMs*` / `reconnectRaiseWristMs*` 的定义涉及"首个 `session.resume.result` / 首个业务消息"的到达时间，只有真机 Watch 上运行的客户端能发起该语义的请求并采样。**不得**用 `tools/ws_loadgen` 的 `cold_connect` / `raise_wrist` 输出填本节 —— 该工具只度量"dial + `debug.echo` RTT"，不走 session.resume 提供者链路（Story 0.15 Dev Notes 禁止事项 + AC7 工具边界），用其数据会**系统性低估**真实重连延迟并让 cell 假通过。`tools/ws_loadgen` 的输出只适用于 §7（AC6 Hub 压测）。

**列说明：**
- `connectColdMs*`：TCP connect + WS upgrade + 首个 `session.resume.result` 到达的端到端时间（p50/p95/p99，单位 ms）
- `reconnectRaiseWristMs*`：抬腕触发重连 → 首个业务消息到达的端到端时间（p50/p95/p99；无抬腕的 cell 填 `N/A`）
- `reconnectWithin5sRate`：5 秒内重连成功率（NFR-REL-4 目标 ≥ 98%；失败 = 5s 超时 / 握手拒绝 / 限流）

| cell | connectColdP50 | connectColdP95 | connectColdP99 | reconnectRaiseWristP50 | reconnectRaiseWristP95 | reconnectRaiseWristP99 | reconnectWithin5sRate |
|---|---|---|---|---|---|---|---|
| cell 1 | TODO | TODO | TODO | N/A | N/A | N/A | TODO |
| cell 2 | N/A | N/A | N/A | TODO | TODO | TODO | TODO |
| cell 3 | N/A | N/A | N/A | N/A | N/A | N/A | TODO（长连不重连则 100% 或 N/A） |
| cell 4 | TODO | TODO | TODO | N/A | N/A | N/A | TODO |
| cell 5 | N/A | N/A | N/A | TODO | TODO | TODO | TODO |
| cell 6 | N/A | N/A | N/A | N/A | N/A | N/A | TODO |
| cell 7 | TODO | TODO | TODO | N/A | N/A | N/A | TODO |
| cell 8 | N/A | N/A | N/A | TODO | TODO | TODO | TODO |
| cell 9 | N/A | N/A | N/A | N/A | N/A | N/A | TODO |
| cell 10 | TODO | TODO | TODO | N/A | N/A | N/A | TODO |
| cell 11 | N/A | N/A | N/A | TODO | TODO | TODO | TODO |
| cell 12 | N/A | N/A | N/A | N/A | N/A | N/A | TODO |

**原始 JSON 归档：** 每 cell 一个 `docs/spikes/data/cell01.json` … `cell12.json`，由**真机 Watch 客户端遥测**直接写出。字段建议包含：`cellId / network / scenario / watchOSVersion / startAt / endAt / connectColdMs{p50,p95,p99} / reconnectRaiseWristMs{p50,p95,p99} / reconnectWithin5sRate / sampleCount`。**注意**：此格式**不等同**于 `tools/ws_loadgen -report` 的 Summary 结构（后者是 hub 侧 `debug.echo` RTT，语义不同）。ws_loadgen 的 JSON 只写 `hub-Nk.json`，不写 `cellNN.json`。

**复现命令模板（每 cell 命令不同，示例）：**
```bash
# cell 3 (Wi-Fi, long_lived 30min, single connection from Watch)
# — 由 Watch app 直连；服务端 debug 模式启动
bash scripts/build.sh && ./build/catserver --config config/default.toml
# Watch 端连 ws://<server>/ws，记录心跳稳定性 30min
```

**每 cell 须附：** 实际命令 + 起止时间戳 + 原始 JSON git 路径。

---

## 6. Battery Baseline

> **Phase B TODO** — 架构师执行后填本节。

**测量方法：**
1. 同一 Apple Watch 在**电量充至 100%** 后起始。
2. **实验组（WS-primary）：** Watch app 长连 WS，以 `ping_interval_sec = 30` 保活，持续 30 min。
3. **对照组（baseline HTTP polling）：** 同一 Watch 运行一个最小 Shortcut 或辅助 app，每 30s 一次 `GET /healthz`（经由 `tools/spike_http_poll_baseline.go` 模拟服务端侧；实际电量在 Watch 端测量），持续 30 min。
4. 记录起止 Battery %（Settings → Battery → Last charged 时间线）。

| 实验 | 起始电量 | 结束电量 | 下降 pp | 平均 pp/min |
|---|---|---|---|---|
| WS-primary (30min 长连) | TODO | TODO | TODO | TODO |
| Baseline (30min HTTP 30s 轮询) | TODO | TODO | TODO | TODO |
| **差值 `ΔBattery` = WS - Baseline** | — | — | TODO | — |

**判据：**
- `ΔBattery ≤ 5pp` → WS-primary 电量可接受，直接进入 §8 / §9 决策。
- `ΔBattery > 5pp` → §9 必须明确讨论权衡（用户 memory `project_backup_fallback.md`：反对 backup 规避根因，但不等于忽视电量；以数据判断 WS-primary 能否进入可接受区间）。

**取样时间线（供核查）：** TODO — 列每次取样的 UTC/本地时间戳 + 电量百分比。

---

## 7. Hub Load Test (ADR-003)

> **Phase B TODO** — 架构师用 `tools/ws_loadgen` 在本机（MacBook）或 docker-compose Linux 容器执行后填本节。

**压测档位：** N ∈ {**1k, 3k, 5k, 10k**}，每档稳态运行 ≥ 10 min。

**环境注意：**
- Windows 本机 socket 上限与 TIME_WAIT 回收慢，不推荐。
- MacBook M1/M2 8-16G 可能在 10k 档遇到 ulimit / 端口耗尽。
- 优先 docker-compose Linux 容器（调高 `net.ipv4.ip_local_port_range` + `nofile` ulimit），报告记录环境差异。

| N | connectSuccessRate | broadcastLatencyP95 (ms) | broadcastLatencyP99 (ms) | cpuPct | memRSS_MB | goroutineCount | reconnectRatio | 备注 |
|---|---|---|---|---|---|---|---|---|
| 1000 | TODO | TODO | TODO | TODO | TODO | TODO | TODO | TODO |
| 3000 | TODO | TODO | TODO | TODO | TODO | TODO | TODO | TODO |
| 5000 | TODO | TODO | TODO | TODO | TODO | TODO | TODO | TODO |
| 10000 | TODO | TODO | TODO | TODO | TODO | TODO | TODO | TODO |

**`reconnectRatio` 判据（round 2 review 修正后口径）：**

```
reconnectRatio = reconnectAttempts / concurrent
```

- 分母是**配置的 worker 总数**（固定 = N，本表的 1k/3k/5k/10k），**不是** `connectSuccess`（后者包含了重连成功次数，会稀释分母约 `R/(N+R)`，系统性低估 churn）
- 分子 `reconnectAttempts` 仅在 worker 已有一次成功会话之后才累加（启动期 dial-fail 的初次建连重试不算；见 `tools/ws_loadgen` `Summary.ReconnectAttempts` godoc）
- 阈值：`> 0.05` → 该档位 hub 未能维持稳态，本行 `broadcastLatencyP95/P99` 读数须加 "⚠️ 非稳态" 标签，**不**可用作 ADR-003 `broadcastLatencyP99 ≤ 3000ms` gate 的直接输入
- 反例说明 round 1 的缺陷：若 1000 workers 中 50 个各重连 1 次，旧公式 `50/(1000+50)=4.76%` 判"稳态"；新公式 `50/1000=5.0%` 触发"非稳态"，符合文档原意

**goroutineCount 测量：** 从 healthcheck 路由或直接 `Hub.GoroutineCount()` 读（= `ConnectionCount() × 2`，readPump + writePump）。

**CPU / RSS 测量：** 容器内 `docker stats <container>` 取稳态平均值；MacBook 本机 `top -pid $(pgrep catserver)`。

**触发点（AC6 / AC10）：** 若 10k 档 `broadcastLatencyP99 > 3000ms`：
1. **下调 `config/default.toml` `[ws].max_connections`** 至满足 `p99 ≤ 3s` 的最大档位（如 5000），并在相邻加注释 `# max_connections tuned by Story 0.15 spike YYYY-MM-DD, see docs/spikes/op1-ws-stability.md §7`。
2. 跑 `bash scripts/build.sh --test` 验证 Stories 0.4 / 0.11 测试仍绿。
3. §9 收敛方案必须接受下调后的上限。
4. 本节末尾追加 `### Phase 3 Sticky Routing Planning` 段落（见下）。

**若 10k p99 ≤ 3s：** 保留 `max_connections = 10000`，本节末尾明确 `decision: keep 10k, no sticky-routing planning needed for MVP`，仍留档 Phase 3 触发阈值。

**原始 JSON 归档：** `docs/spikes/data/hub-1k.json` / `hub-3k.json` / `hub-5k.json` / `hub-10k.json`。

### Phase 3 Sticky Routing Planning

> **仅在 10k p99 > 3s 触发时填本节；否则空节留档触发阈值。**

- 触发条件：TODO（在 §D9 ADR-003 锚点基础上写实测数据驱动的具体阈值）
- 扩展路径：`architecture.md` §D1 已预留 `RedisPubSubBroadcaster`（行 282-292）→ 具体接口从 D6 预留转为 Epic 4 实装时的映射
- 预估工作量等级：TODO（S / M / L）
- 阻塞关系：TODO

---

## 8. Candidate Feasibility

> **Phase B TODO** — 架构师在 §5 / §7 数据落地后填本节。

**候选方向（固定 5 个，禁止新增 — 架构 §D6 已收敛）：**

| # | 方向 | feasibleGivenData | effortEstimate | hubInterfaceImpact | riskLevel |
|---|---|---|---|---|---|
| 1 | 客户端 cache-first + 差分更新 | TODO (√/×/?) | TODO (S/M/L) | uses existing `BroadcastDiff`（D6 预留） | TODO (Low/Med/High) |
| 2 | 服务端主动 session.resume 推送 | TODO | TODO | uses existing `PushOnConnect`（D6 预留） | TODO |
| 3 | 精细化抬腕事件重连 | TODO | TODO | client-only, no hub impact | TODO |
| 4 | WS permessage-deflate 压缩 | TODO | TODO | adds `EnableCompression` flag（Hub 配置扩展） | TODO |
| 5 | `NWConnection` 深度适配 | TODO | TODO | client-only, no hub impact | TODO |

**硬约束（AC8）：** 本节必须**明确验证** D6 预留接口 `PushOnConnect` + `BroadcastDiff`（`architecture.md` 行 285-286、`internal/ws/broadcaster.go` 行 16-17）能够满足**至少 2 个**候选方向落地。若数据不支持，§9 必须升级为架构风险 flag。

**D6 接口验证结论：** TODO（是 / 否 + 3 句理由）

---

## 9. Convergence Decision

> **Phase B TODO** — 架构师在 §5 / §7 / §8 数据落地后填本节。

**`chosenDirection`：** TODO（§8 表的行号 1-5）
**`secondaryDirection`：** TODO（可选；必须仍是 WS-primary 变体；**禁止 HTTP polling / short-poll / hybrid WS+HTTP heartbeat**）

**`rationale`：**
TODO — 引用 §5 / §7 数据支撑，至少 3 句理由。

**`hubInterfaceChanges`：**
TODO — 细化 `architecture.md` §D1 行 282-287 `Broadcaster` interface 后续 Epic 中需要的具体实现：哪些方法从 D6 预留转为 Epic 4 实装；是否需要新增/修改字段。

**`serviceLayerChanges`：**
TODO — Epic 4 的 `RoomService` / `PresenceService` 与 Broadcaster 的耦合点。

**`clientSideChanges`：**
TODO — Watch/iPhone 客户端需做的工作；仅罗列接口 + 行为，**不**给出 Swift 代码（后端 story 不涉及客户端实装）。

**`metricsToMonitor`：**
TODO — 上线后需要监控哪些指标来验证方案有效（`architecture.md` §D14 Metrics 预留，本 story 只列字段名 + 含义）。

### 硬禁止（AC9 / user memory `project_backup_fallback.md`）

本节出现以下任一字样即违反 AC：
- "HTTP polling fallback"
- "short-poll backup"
- "hybrid WS+HTTP heartbeat"

次要方向如必要必须仍是 WS-primary 变体（例如主方向 #1 + 次方向 #3 抬腕精细化作为优化补强）。

### Not-Converged 分支

若数据结论为"当前 WS-primary 不可行"：在此写 **NOT CONVERGED**，详述根因，并停在本 story。`sprint-status.yaml` 中 `epic-0` 保持 `in-progress`，Epic 4 无法启动（AC11 签字 `BLOCKED_SEE_SECTION_9`）。

---

## 10. Architect Sign-off

> **Phase B TODO** — 架构师在所有 12 cells 执行完 + §9 决策落地后填本节。

| 字段 | 值 |
|---|---|
| `signoffDate` (YYYY-MM-DD) | TODO（不得早于所有 cells 执行完） |
| `signoffBy` | 架构师（开发者本人，user memory `project_claude_coding.md`） |
| `epic4Status` | TODO（`UNBLOCKED` / `BLOCKED_SEE_SECTION_9`，与 §9 匹配） |
| `followupStories` | TODO（估算 Epic 4 首个 story `4-1-presence-lifecycle-d8-ws-disconnect-leave-room` 是否可按 `epics.md` 行 1180 前置条件开始） |

### 签字后 sprint-status 动作

| epic4Status | sprint-status 动作 |
|---|---|
| `UNBLOCKED` | 立即更新 `sprint-status.yaml` 的 `9-1-*` 键为 `done`；`epic-1` … `epic-8` **保持 `backlog`**（本 story 只解锁 Epic 4，其他 Epic 前置不在本 story 范围）。 |
| `BLOCKED_SEE_SECTION_9` | `sprint-status.yaml` 标记 `9-1-*` 为 `done`（spike 数据采集完成；结论"不可行"仍是有效完成）；新增注释 `# project-note: OP-1 blocked; Epic 4 paused pending protocol redesign`。 |

---

## 链接回溯

- 构建期约束：[`docs/backend-architecture-guide.md`](../backend-architecture-guide.md) §12 WebSocket / §19 PR checklist
- 设计候选方向与 Hub 压测触发：[`server/_bmad-output/planning-artifacts/architecture.md`](../../server/_bmad-output/planning-artifacts/architecture.md) §D6 行 351-365 / §D9 ADR-003 行 390-400 / §Spike-OP1 依赖 行 493
- Story 文件（Phase A）：[`server/_bmad-output/implementation-artifacts/0-15-spike-op1-watchos-ws-primary-stability-test-matrix.md`](../../server/_bmad-output/implementation-artifacts/0-15-spike-op1-watchos-ws-primary-stability-test-matrix.md)
- Story 文件（Phase B）：[`server/_bmad-output/implementation-artifacts/9-1-spike-op1-real-device-and-hub-loadtest-execution.md`](../../server/_bmad-output/implementation-artifacts/9-1-spike-op1-real-device-and-hub-loadtest-execution.md)
- Epics 行 727-747（AC 原文）：[`server/_bmad-output/planning-artifacts/epics.md`](../../server/_bmad-output/planning-artifacts/epics.md)

### 本报告不经 CI 校验（markdown）

`docs/spikes/` 不在 `docs/api/openapi.yaml` 校验范围内；AC12 要求 spike **不扩大 `dto.WSMessages`**（本报告与 `tools/ws_loadgen` 仅使用现有 `session.resume` / `debug.echo` / `debug.echo.dedup` 三条消息），Story 0.14 AC15 `validateRegistryConsistency` 双 gate 不会误报。
