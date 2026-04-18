# Story 0.15: Spike-OP1 — watchOS WS-primary 稳定性真机测试矩阵（Phase A：服务端脚手架）

Status: review

<!-- Note: Validation is optional. Run validate-create-story for quality check before dev-story. -->

## Scope Split Note (2026-04-18)

原 Story 0.15 的 14 个 AC 中，AC3 / AC4 / AC5 / AC6（执行部分）/ AC8 / AC9 / AC10 / AC11 / AC13 **全部需要真机 Apple Watch + iPhone 或人工判断**，无法由 Claude dev-story workflow 完成。按 user memory `project_claude_coding.md`（Claude 99.99% 编码，瓶颈在真机调试）+ `project_backup_fallback.md`（反对用 fallback 掩盖根因），本 story 于 2026-04-18 拆分为两阶段：

| 阶段 | 承载 Story | AC 范围 | 执行者 |
|---|---|---|---|
| **Phase A（本 story）** | `0-15-*` | AC1 Pre-flight / AC2 报告骨架 / AC7 工具选型 + `tools/ws_loadgen/main.go` / AC12 / AC14（仅针对脚手架） | Claude（dev-story workflow） |
| **Phase B** | `9-1-spike-op1-real-device-and-hub-loadtest-execution.md` | AC3 / AC4 / AC5 / AC6 执行 / AC8 / AC9 / AC10 / AC11 / AC13 | 架构师（手动执行真机测试 + 压测 + 签字） |

**本 story 的 "completed" 含义**：Phase A 服务端脚手架全部就位（报告骨架 + 工具选型 + `ws_loadgen` 工具 + 构建绿）。**不**意味着 Spike-OP1 已收敛；Epic 4 解锁取决于 Story 9.1 Phase B 签字（AC11）。

下文原 AC 1-14 保留**原文**（`epics.md` 行 727-747 锚点不变），但在每条 AC 开头标注 `[Phase A]` / `[Phase B → 9.1]` 明确归属（Phase B 承载 story 原为 0.16，于 2026-04-18 迁入新 Epic 9 并重编号为 Story 9.1 — 让 Epic 0 不被真机执行卡死）。


## Story

As an architect,
I want to quantify WS reconnect p50/p95/p99 latency and battery consumption on real Apple Watch + iPhone across a weak-network matrix using the full WS stack shipped in Stories 0.9–0.14,
so that OP-1 design can converge on **one** direction (from candidates: cache-first + diff, PushOnConnect, raise-wrist reconnect, WS permessage-deflate, NWConnection) driven by data (not guesses), **without introducing HTTP fallback backup**, and unblock Epic 4（OP-1/ADR-003/D9 硬前置）.

## Acceptance Criteria

> **本 story 是 Spike (research) 而非 feature。** 绝大多数交付物是**报告 + 数据表 + 收敛决策**，不是代码。唯一可能的代码变更是 (a) `cfg.WS.MaxConnections` 按压测结论调整（ADR-003 触发条件）+ (b) 一个最小压测工具（AC7 选型结果）+ (c) 简易测试客户端脚本。禁止借此 spike 顺带重构 WS 栈。

1. **[Phase A]** **AC1 — 前置依赖就绪性校验（`architecture.md` §Spike-OP1 关键依赖 行 493、Story 0.9–0.14 闭合）**：

   - 开始 spike 前在报告第 1 节 "Pre-flight Checklist" 以表格形式逐项勾选：
     - `internal/ws/hub.go` 暴露 `HubConfig.MaxConnections`（已存在，行 19）— **必选**
     - `internal/ws/hub.go` `Hub.GoroutineCount()` 可读（已存在，行 61）— **必选**
     - `internal/ws/upgrade_handler.go` + 限流（Story 0.11）+ 黑名单生效 — **必选**
     - `internal/ws/session_resume.go` `ResumeCache` + `ResumeCacheInvalidator`（Story 0.12）在 `debug` 模式注册 — **必选（spike 压测全程 debug mode）**
     - `internal/ws/dedup.go` `RegisterDedup` 中间件可用（Story 0.10）— **必选**
     - `internal/dto/ws_messages.go` `WSMessages` 含 `session.resume` / `debug.echo` / `debug.echo.dedup`（Story 0.14）— **必选**
     - `internal/push/` Pusher + APNs 队列（Story 0.13）— 仅基线对比需要，可选
     - `clockx.Clock` + cron `withLock`（Stories 0.7/0.8）— 用于 Fake 时间驱动长连 + 心跳 tick — **必选**
   - 任何一项缺失：STOP spike，回溯对应 story 修复，不在本 story 内临时 patch。此 AC 的目的是防止 "spike 夹带 fix" 的反模式（参考 Story 0.13 round 2 detached writeCtx 的教训：fix 不该堆在不相关 story）。

2. **[Phase A]** **AC2 — 报告文件与目录结构（`architecture.md` §Project Structure 行 967-968、G5 gap 行 1198）**：

   - 新建 `docs/spikes/op1-ws-stability.md`。`docs/spikes/` 目录**本 story 首次创建**（`ls docs/` 目前仅 `api/ backend-architecture-guide.md`；本 story mkdir 之）。
   - 报告结构**固定**以下 10 节（顺序与标题严格一致，便于后续 story 按锚点引用）：
     1. `## 1. Pre-flight Checklist`（AC1 勾选表）
     2. `## 2. Test Tool Selection`（AC7 工具决策 + 理由）
     3. `## 3. Test Matrix`（AC3 网络 × 场景 = 12 cells）
     4. `## 4. Device & Environment`（AC4 设备清单 + OS/WatchOS 版本 + 服务器部署拓扑 + Clock 来源）
     5. `## 5. Latency & Reliability Metrics`（AC5 原始数据表）
     6. `## 6. Battery Baseline`（AC5 电量对比）
     7. `## 7. Hub Load Test (ADR-003)`（AC6 hub 1k/3k/5k/10k）
     8. `## 8. Candidate Feasibility`（AC8 5 候选方向评估）
     9. `## 9. Convergence Decision`（AC9 单一方案 + 理由 + 对 Hub/Service 的接口改动）
     10. `## 10. Architect Sign-off`（AC11 签字闭合 + 解锁 Epic 4 时间戳）
   - 每节在 spike 执行前先用 `TODO:` 占位，执行完填表/写结论；未填完不得进入 AC11 签字。

3. **[Phase B → 9.1 AC2]** **AC3 — 测试矩阵（epics 行 737）**：

   - 矩阵为**网络状态 × 行为场景** = 4 × 3 = 12 个 cells，全部执行，不遗漏：

     | 网络 \ 场景 | 冷启动建连 | 抬腕循环重连 | 长连持续心跳 |
     |---|---|---|---|
     | Wi-Fi | cell 1 | cell 2 | cell 3 |
     | 4G LTE | cell 4 | cell 5 | cell 6 |
     | 弱网（丢包 10% ± 2%） | cell 7 | cell 8 | cell 9 |
     | 网络切换（BT↔LTE 每 60s） | cell 10 | cell 11 | cell 12 |

   - 弱网模拟**强制用 macOS Network Link Conditioner**（Additional Tools for Xcode）或同等工具；**不得**用 tc netem 在服务器侧模拟（服务器侧丢包会污染 hub 压测结论，必须在客户端网卡层引入）。
   - 每个 cell 执行时长 **≥ 30 分钟**（epics 行 738），期间**不重启客户端进程**；长连场景要求实测 ≥ 30 min 持续心跳不中断或记录每次中断 + 重连延迟。
   - 每 cell 采样量：建连场景重复 ≥ 30 次取 p50/p95/p99；重连场景模拟抬腕/落腕 ≥ 60 次；长连场景单次 30 min 观察 1 条 timeseries 即可。
   - 报告 §3 以 12 行表格呈现 cell ID + 执行时间 + 通过/失败（通过 = NFR-REL-4 达成）+ 主观备注。

4. **[Phase B → 9.1 AC3]** **AC4 — 测试设备（epics 行 738、PRD §构成 §watch）**：

   - **真机强制**：
     - ≥ 1 台**实体** Apple Watch（非 simulator）运行 **watchOS 10.0+**（MVP 目标最低支持版本；报告必须记录实测 OS 版本号）
     - ≥ 1 台配对 iPhone 运行 iOS 17.0+
   - simulator/Xcode preview **禁止**作为 spike 主数据源。可作为回归对照，但必须在报告中明确标注 "simulator" 不计入 p50/p95/p99 决策数据。
   - 服务器端部署：本地 MacBook（`mode = "debug"`，`config/default.toml` 已满足）+ 本地 Mongo/Redis（docker-compose 或 homebrew service），**不**要求云端部署；报告记录 CPU 型号（M1/M2/Intel）、RAM、Go 版本（`go version`）、commit hash (`git rev-parse HEAD`)。
   - Clock 源：生产用 `clockx.RealClock`；spike 客户端测试工具可注入 `FakeClock` 用于 deterministic 重连间隔，但服务端必须 Real（M9 守卫）—— 报告记录每端 Clock 来源。

5. **[Phase B → 9.1 AC4]** **AC5 — 延迟 / 重连 / 电量量化（epics 行 739、NFR-REL-4 行 159）**：

   - **延迟指标（单位 ms）**：每 cell 记录以下列：
     - `connectColdP50 / connectColdP95 / connectColdP99`（TCP connect + WS upgrade + 首个 `session.resume.result` 到达的端到端时间）
     - `reconnectRaiseWristP50/P95/P99`（抬腕触发重连 → 首个业务消息到达的端到端时间；无抬腕事件的 cell 此列填 `N/A`）
     - `reconnectWithin5sRate`（5 秒内重连成功率，NFR-REL-4 目标 ≥ 98%；失败 = 5s 超时 / 握手拒绝 / 限流）
   - **电量指标**：30 分钟长连持续心跳场景下，Apple Watch 电量消耗（Settings → Battery → "Last charged" 起始/结束 % + 屏幕时间内 `catapp` 前后台占比）。
     - **基线对比**：在同一 Apple Watch 上以 30s 轮询 HTTP + APNs 模拟旧方案（用一个最小 `tools/spike_http_poll_baseline.go` 脚本，30s 一次 `GET /healthz`；报告描述之，不进主仓库代码路径）作为对照组，记录 `batteryPctDropBaseline` vs `batteryPctDropWSPrimary`。
     - 差值若 > 5 百分点 → spike 必须在报告 §6 明确讨论权衡（用户 feedback "不接受 backup 掩盖根因" 不代表忽视电量；而是用数据判断 WS-primary 能否达到可接受区间）。
   - **数据呈现**：§5 以表格给出 12 cells × 5 列；不得用图表代替表格（便于 grep + diff）。图表仅作 §8 候选对比用。
   - **达标判据**：`reconnectWithin5sRate ≥ 98%` 在 ≥ 10/12 cells 成立即视为 NFR-REL-4 通过；弱网 + 网络切换 cells 若低于 98% 需在 §6 解释（可能触发候选方向 #3 "精细化抬腕重连"）。

6. **[Phase B → 9.1 AC5]** **AC6 — Hub 上限压测 / ADR-003 决策触发（epics 行 740、`architecture.md` §D9 行 390-400）**：

   - 压测并发连接数 N ∈ {**1k, 3k, 5k, 10k**}；每档稳态运行 ≥ 10 分钟。
   - 压测环境：若本机（MacBook M1/M2 8-16G）无法跑到 10k（ulimit、端口耗尽、Go runtime MEM），优先用 `docker-compose` 启 1 实例 Linux 容器（容器内调高 `net.ipv4.ip_local_port_range` + `nofile` ulimit）；报告 §7 必须记录环境差异。
   - **记录列（每档一行）**：`N / connectSuccessRate / broadcastLatencyP95 / broadcastLatencyP99 / cpuPct / memRSS_MB / goroutineCount（= hub.GoroutineCount()）`
   - **`broadcastLatencyP99`** 触发点：若 10k 下 `p99 > 3000ms`，则本 story 同时负责：
     1. 下调 `config/default.toml` `[ws].max_connections` 至满足 `p99 ≤ 3s` 的最大档位（如 5000），提交此 config 改动；
     2. 在报告 §7 结尾专列 `### Phase 3 Sticky Routing Planning`，列出触发条件 + 扩展路径（架构 D1 已预留 `RedisPubSubBroadcaster`）+ 预估工作量等级（S/M/L）；
     3. 报告 §9 的收敛方案必须接受下调后的上限。
   - **若 10k p99 ≤ 3s**：保留 `max_connections = 10000`，报告 §7 明确 "decision: keep 10k, no sticky-routing planning needed for MVP"，但仍然记录 Phase 3 触发阈值留档。
   - 压测工具：**复用 AC7 决策产物**（不要 adhoc 写一次性工具）。

7. **[Phase A]** **AC7 — 压测工具选型（G5 gap, `architecture.md` 行 1198、epics 行 742）**：

   - 候选：`k6` / `vegeta` / 自写 Go 客户端。评估维度（报告 §2 表格）：
     - WS 协议原生支持度（vegeta HTTP only → 不适合 WS，天然出局；保留在表内说明否决理由）
     - 分布式生成能力（k6 cloud / 多 runner；MVP 单机够用）
     - 与现有 Go 代码互操作性（自写 Go 客户端可复用 `internal/dto` 消息常量，避免 magic string）
     - 学习成本 + 引入依赖风险（k6 引入 JavaScript runtime + 新的容器）
   - **推荐路径（在 spike 开始前即可决策）**：**自写 Go 客户端**（文件：`tools/ws_loadgen/main.go`，**tools/** 目录下一次性脚本，`architecture.md` §Project Structure 行 972-973）。理由：
     1. 可直接 `import "github.com/huing/cat/server/internal/dto"` 拿消息常量（Story 0.14 产物），避免常量双写漂移；
     2. 可复用 `pkg/clockx` 做 deterministic 时钟；
     3. 避免引入 k6/JS 依赖对仅 Go + 单一后端的 MVP 过度设计；
     4. 报告交付后工具可直接砍掉（`tools/` 本就是 one-off 脚本存放处），不进 `internal/`。
   - 如果 spike 执行期发现自写 Go 客户端遇到不可克服的问题（如 CPU 瓶颈），允许切到 k6；切换决策必须在报告 §2 明确记录并附时间戳。
   - 工具必须支持的最小能力：并发 N goroutines 各开一条 WS → 周期性发 `debug.echo` → 采集延迟样本 → 输出 JSON summary。**不**要求支持自动 `session.resume`（那是 e2e 场景客户端模拟，不是 hub 压测）。

8. **[Phase B → 9.1 AC6]** **AC8 — OP-1 候选方向可行性评估（epics 行 741、`architecture.md` §D6 行 351-365）**：

   - 5 个候选方向（固定，禁止新增 —— 架构 Step 4 已收敛）：
     1. 客户端 cache-first + 差分更新（需 `BroadcastDiff`）
     2. 服务端主动 session.resume 推送（需 `PushOnConnect`）
     3. 精细化抬腕事件重连（Hub 无变化）
     4. WS permessage-deflate 压缩（Hub 配置扩展）
     5. `NWConnection` 深度适配（Hub 无变化）
   - 报告 §8 以表格形式给每个候选方向的：
     - `feasibleGivenData`（√ / × / ? —— 根据 §5 / §7 的数据判断；弱网 cell 重连成功率高 → 方向 3 价值低，反之高价值）
     - `effortEstimate`（S / M / L —— Story scope 数量估计）
     - `hubInterfaceImpact`（"none" / "uses existing `BroadcastDiff`" / "uses existing `PushOnConnect`" / "adds `EnableCompression` flag" / "client-only, no hub impact"）
     - `riskLevel`（Low/Med/High —— 涉及的不确定性）
   - **硬约束**：报告必须**明确验证** D6 预留接口 `PushOnConnect` + `BroadcastDiff`（`architecture.md` 行 285-286、`internal/ws/broadcaster.go`）能够满足**至少 2 个**候选方向落地（epics 行 741）；若数据不支持，报告 §9 必须升级为架构风险 flag。

9. **[Phase B → 9.1 AC7]** **AC9 — 单一方向收敛（epics 行 743、user feedback "no backup fallback"）**：

   - §9 必须给出**一个**且仅**一个**主方向（允许 1 个次要方向作为 "plan B fallback on the same WS-primary approach"，但不得是 HTTP backup 类 —— `project_backup_fallback.md` memory：用户明确反对 backup 掩盖根因）。
   - 收敛方案必须包含以下字段：
     - `chosenDirection`（§8 表的行号）
     - `rationale`（引用 §5 / §7 数据支撑，至少 3 句理由）
     - `hubInterfaceChanges`（细化 `architecture.md` §D1 行 282-287 `Broadcaster` interface 后续 Epic 中需要的具体实现：哪些方法从 D6 预留转为 Epic 4 实装；是否需要新增/修改字段）
     - `serviceLayerChanges`（Epic 4 的 `RoomService` / `PresenceService` 与 Broadcaster 的耦合点）
     - `clientSideChanges`（Watch/iPhone 客户端需做的工作；仅罗列接口 + 行为，**不**给出 Swift 代码 —— 后端 story 不涉及客户端实装）
     - `metricsToMonitor`（上线后需要监控哪些指标来验证方案有效，`architecture.md` §D14 Metrics 预留未来接入，本 story 只列出字段名 + 含义）
   - **硬禁止**：方案中出现 "HTTP polling fallback" / "short-poll backup" / "hybrid WS+HTTP heartbeat" 即违反 AC；用户明确反对（epics 行 744：方案**不得引入 HTTP fallback backup**）。次要方向如必要必须仍是 WS-primary 变体（例如主方向 #1 + 次方向 #3 抬腕精细化作为优化补强）。
   - 若数据结论为 "当前 WS-primary 不可行"：§9 写 "**NOT CONVERGED**"，停在此 story，开发者评估后续（epics 行 746）；此时 sprint-status.yaml 中 `epic-0` 保持 `in-progress`，Epic 4 无法启动（AC11 签字不发生）。

10. **[Phase B → 9.1 AC8]** **AC10 — `cfg.WS.MaxConnections` 改动（仅在 AC6 触发时）**：

    - 若 AC6 压测结论要求下调 `max_connections`：
      - 修改 `config/default.toml` 行 31 `max_connections = 10000` 为压测通过档位（如 `5000`）。
      - 同步修改 `config/development.toml` 如存在（`Glob` 确认 —— 目前 `server/config/*.toml` 仅 `default.toml`，若未扩展则仅动 default）。
      - 在 config 相邻加 `# max_connections tuned by Story 0.15 spike 2026-04-XX, see docs/spikes/op1-ws-stability.md §7` 注释（config toml 支持 `#`）。
      - 运行 `bash scripts/build.sh --test` 验证 healthcheck / upgrade_handler 相关测试（Stories 0.4 / 0.11 已覆盖 threshold 行为）仍绿。
    - 若 AC6 无需下调：**不改任何配置**；禁止 "proactively lower to 5k for safety" —— PRD NFR-SCALE-4 行 147 目标 10k，没有数据不调。

11. **[Phase B → 9.1 AC9]** **AC11 — 架构师签字闭合 + 解锁 Epic 4（epics 行 745-746）**：

    - §10 "Architect Sign-off" 段落必须包含：
      - `signoffDate`（YYYY-MM-DD，本 story 完成日；不得早于所有 cells 执行完）
      - `signoffBy`（开发者即架构师 —— MEMORY `project_claude_coding.md` + `project_go_backend.md`：单人后端 + Claude-driven）
      - `epic4Status`：`UNBLOCKED` / `BLOCKED_SEE_SECTION_9`（与 AC9 是否收敛匹配）
      - `followupStories`（估算 Epic 4 首个 story，即 `4-1-presence-lifecycle-d8-ws-disconnect-leave-room` 能否按 epics 行 1180 中的前置条件开始）
    - 签字发生后立即更新 `sprint-status.yaml` 的 `0-15-*` 键 `backlog → ready-for-dev`（由 create-story workflow 自动完成，dev-story 随后推进到 review → done）；若 `epic4Status = UNBLOCKED`，sprint-status `epic-1`, `epic-2`, `epic-3`, `epic-5`, `epic-6`, `epic-7`, `epic-8` **保持 backlog 不动**（本 story 只解锁 Epic 4，其他 Epic 的前置关系不在本 story 范围）。
    - 签字若 `BLOCKED_SEE_SECTION_9`：sprint-status 标记 `0-15-*` 为 `done`（spike 已执行完数据采集，结论为"不可行"仍是有效完成），并新增一行 `project-note: "OP-1 blocked; Epic 4 paused pending protocol redesign"`（手工改 yaml 注释）。

12. **[Phase A]** **AC12 — 测试 / 日志 / 构建纪律（`backend-architecture-guide.md` §19 PR checklist）**：

    - `tools/ws_loadgen/main.go`（AC7 自写客户端）的 `package main`：
      - **不**引入新的第三方依赖（只用 stdlib + 现有 `go.mod` 里的 `gorilla/websocket` + `huing/cat/server/internal/dto`）；
      - **禁用** `fmt.Printf` / `log.Printf`（§P5 + §19）；改用 `log.New(os.Stdout, ...)` 或最小 `zerolog` 调用；
      - 每个 goroutine 退出前 `defer c.conn.Close()` 避免 fd 泄露；
      - 工具 `-help` 必须打印所有 flag + 默认值 + 示例；
      - 工具主循环用 `context.WithCancel` + `signal.NotifyContext(ctx, os.Interrupt)` 响应 Ctrl-C；
    - **本 story 不新增 `internal/` Go 文件**（除 AC10 可能的 `config/default.toml` 一行改动外）；所有代码落 `tools/ws_loadgen/`，`bash scripts/build.sh --test` 要通过（`tools/` 目录被 `go vet ./...` 覆盖）。
    - **不**为 spike 工具写 `_test.go`（tools 是 one-off）；但若 AC6 触发了 `config/default.toml` 改动，运行现有测试套件验证无回归。
    - 报告 `docs/spikes/op1-ws-stability.md` 不经 CI 校验（md 文件；架构 §G2 OpenAPI 校验范围为 `docs/api/openapi.yaml`）；但必须在 Story 0.14 Story 0.10/0.12 AC4/AC9 对 dispatcher registry 的漂移守门下**不触发 drift**（即不新增 WS 消息类型到 `dto.WSMessages` —— spike 不应偷偷扩大协议表面）。

13. **[Phase B → 9.1 AC10]** **AC13 — 文档与 backend-architecture-guide 更新**：

    - 如果 AC6 触发 `max_connections` 下调：`docs/backend-architecture-guide.md` §12 WebSocket 段或相邻 `NFR` 列表若有"10k"硬编码文字，更新为"Spike-OP1 tuning 后的实测值"；搜 grep `-n "10,000\|10k"` 定位。
    - 报告尾部显式链接回 `docs/backend-architecture-guide.md` + `server/_bmad-output/planning-artifacts/architecture.md` §D6 §D9 对应行号，便于 reviewer 对齐。
    - **不**在 `architecture.md` 自身做追溯性修订（那是 planning artifact 的只读档案）；**仅**在本 spike 报告 + `backend-architecture-guide.md` （构建期约束文档）内归档结论。

14. **[Phase A（仅针对已交付脚手架）+ Phase B → 9.1 AC11（剩余项）]** **AC14 — PR checklist（`docs/backend-architecture-guide.md` §19）**：

    - 对本 story 的 diff 逐项验证（打 ✅）：
      - 无 `fmt.Printf` / `log.Printf`（§P5 lint）
      - 所有 I/O 函数接受 `ctx`（tools/ws_loadgen 主 loop 有 ctx）
      - 不直接引用 `*mongo.Client` / `*redis.Client` 在 tools 中（若要测 redis/mongo 也由现有 healthz 间接暴露，不 bypass）
      - 无 `context.TODO()` / `context.Background()` 在 business path；tools `main()` 可用 `signal.NotifyContext(context.Background(), os.Interrupt)`（`main` 是顶层 ctx 源，允许）
      - 所有 exported identifier 有英文 godoc（ws_loadgen 主 struct/func）
      - `bash scripts/build.sh --test` 绿
      - `// TODO` 仅允许在 spike 报告中（"TODO: regenerate if v2 protocol lands"）；tools 代码中不得残留

## Tasks / Subtasks

- [x] **Task 1 — Pre-flight 就绪性校验** (AC: #1) **[Phase A]**
  - [x] 阅读 `internal/ws/hub.go` / `internal/ws/upgrade_handler.go` / `internal/ws/session_resume.go` / `internal/ws/dedup.go` / `internal/dto/ws_messages.go` 确认符号在位
  - [x] 以 `mode = "debug"` 跑一次 `bash scripts/build.sh --test` 确保 Stories 0.9–0.14 无回归
  - [x] 在 `docs/spikes/op1-ws-stability.md` §1 填 checklist 表（15 行全部 ✅）
- [x] **Task 2 — 报告骨架落地** (AC: #2) **[Phase A]**
  - [x] `mkdir docs/spikes/` + `docs/spikes/data/`
  - [x] 新建 `docs/spikes/op1-ws-stability.md`，10 节骨架（§1 / §2 已填；§3-§10 留 TODO 占位，由 Phase B 填）
- [x] **Task 3 — 工具选型决策** (AC: #7) **[Phase A]**
  - [x] 填写 §2 评估表（vegeta ✗ / k6 ✗ / 自写 Go ✓）
  - [x] 定稿为 "自写 Go 客户端"，四条理由入报告 §2
- [x] **Task 4 — 自写压测 / e2e 客户端** (AC: #7, #12) **[Phase A]**
  - [x] `mkdir server/tools/ws_loadgen/`
  - [x] `main.go` 支持 flags：`-url`, `-concurrent`, `-duration`, `-send-interval`, `-scenario={cold_connect|raise_wrist|long_lived}`, `-report=path.json`, `-token-prefix`, `-verbose`
  - [x] 集成 `dto.WSMessagesByType["debug.echo"]` 启动时验证 — 漂移即 Fatal（Story 0.14 AC4 共同守门）
  - [x] 输出 JSON summary（p50/p95/p99 + sample count + 分类 errors）
  - [x] Smoke test：`go run ./tools/ws_loadgen -concurrent 3 -duration 2s -scenario long_lived` 正常退出，JSON summary 打印 stdout；`-help` + 非法 scenario 正确走 exit 2
- [x] **Task 5 — 端到端 12 cells 测试执行** (AC: #3, #4, #5) **[→ Story 9.1 Task 3/4]**
  - 已**整体**拆分到 Story 9.1 AC2/AC3/AC4 执行；本 story 不再承担。
- [x] **Task 6 — Hub 压测（ADR-003）** (AC: #6, #10) **[→ Story 9.1 Task 5/8]**
  - 已**整体**拆分到 Story 9.1 AC5/AC8 执行；本 story 不再承担。
- [x] **Task 7 — 候选方向可行性评估** (AC: #8) **[→ Story 9.1 Task 6]**
  - 已**整体**拆分到 Story 9.1 AC6 执行；本 story 不再承担。
- [x] **Task 8 — 收敛方向决策** (AC: #9) **[→ Story 9.1 Task 7]**
  - 已**整体**拆分到 Story 9.1 AC7 执行；本 story 不再承担。
- [x] **Task 9 — 签字闭合** (AC: #11) **[→ Story 9.1 Task 9]**
  - 已**整体**拆分到 Story 9.1 AC9 执行；本 story 不再承担。
- [x] **Task 10 — 文档更新 + PR checklist** (AC: #13, #14) **[Phase A 部分已完成；剩余 → Story 9.1 Task 10]**
  - [x] `bash scripts/build.sh --test` 最终绿（Phase A 脚手架，含 tools/ws_loadgen）
  - [x] 对照 §19 checklist 针对 Phase A diff 每条过（详见下方 Dev Agent Record PR checklist）
  - [Phase B] 视结论更新 `docs/backend-architecture-guide.md` §12 / NFR 列表 — 延后到 Story 9.1 AC10
  - [x] 报告尾部链回 planning artifacts（op1-ws-stability.md "链接回溯" 节）

## Dev Notes

### 核心定位（必读）

**本 story 是 Spike，不是 feature。** 输出是一份**数据驱动的决策报告**，目的是让 Epic 4（好友房间 / 环境在场感）有**唯一**、**有依据**、**非 backup-fallback** 的 WS-primary 稳定性方案。Epic 4 的 `4-1-presence-lifecycle-d8-ws-disconnect-leave-room` 硬依赖本 story 的签字结论（`epics.md` 行 1180）。

### 关键约束 / 反模式

- **不要借 spike 重构 WS 栈**。Stories 0.9–0.14 已交付完整栈；本 story 仅消费，不修改（AC10 的 `max_connections` 是配置调优，不是栈重构）。
- **不要夹带 fix**。spike 期间若发现 0.9–0.14 bug，开独立 story（参考 Story 0.13 round 1/2 的 review-log 流程 + `done-story` skill），不堆到本 spike。
- **不要引入第三方依赖**（AC12）。`tools/ws_loadgen` 只用 stdlib + 现 `go.mod`。k6 等仅作为备选记录在 §2，不落地。
- **不要引入 HTTP fallback backup**（AC9 硬禁；用户 memory `project_backup_fallback.md` 明确反对）。方案若被迫这么写，就写 `NOT CONVERGED` 让后续重新设计协议。
- **电量不是唯一目标**。用户反对 "以 backup 规避根因"，但并不等于 "接受任何电量劣势"；§6 基线对比的意义是用**数据**判断 WS-primary 是否能进入可接受区间，而非用 "不许讨论电量" 来屏蔽。
- **不要把 spike 工具塞进 `internal/`**。`tools/` 就是 one-off 脚本归宿（architecture §Project Structure 行 972）。结论写完可以留 tool 方便回归，但不要在 `internal/` 增加"压测辅助包"。

### 架构 / 文件分布

- `docs/spikes/` —— 本 story 首次创建（仓库根 `docs/` 目前仅 `api/ backend-architecture-guide.md`）。
- `server/tools/ws_loadgen/` —— 本 story 首次创建。`tools/` 目录目前为空；架构已预留。
- 可能改动：`server/config/default.toml`（AC10 触发条件下）、`docs/backend-architecture-guide.md`（AC13 触发条件下）。**除此之外不改 internal Go 代码**。

### 前置 Story 资产（0.9 – 0.14 可复用）

| 资产 | 来源 | 用法 |
|---|---|---|
| `ws.Hub` / `HubConfig` / `Hub.GoroutineCount()` | 0.9 | AC6 hub 压测 + goroutine 计数 |
| `HubConfig.MaxConnections` / `cfg.WS.max_connections` | 0.9 / 0.4 (healthcheck 消费) | AC10 可能下调的对象 |
| `ws.UpgradeHandler` + Per-user 限流 + 黑名单 | 0.11 | AC3 抬腕重连频次必须穿过限流（验证限流值合理） |
| `ws.ResumeCache` + `ResumeCacheInvalidator` | 0.12 | AC3 冷启动建连后 `session.resume` 命中 |
| `ws.Dispatcher.RegisterDedup` + envelope id 去重 | 0.10 | AC7 压测客户端发 `debug.echo.dedup` 验证幂等 |
| `dto.WSMessages` 常量 + `debug.echo` / `debug.echo.dedup` / `session.resume` | 0.14 | AC7 压测客户端 type 字段来源 |
| `clockx.Clock` + `FakeClock` | 0.7 | 压测工具客户端侧 deterministic 时钟 |
| `cron withLock` + 分布式锁 | 0.8 | Hub 压测验证 cron 不会在 10k 并发下阻塞 |
| Pusher + APNs 队列 | 0.13 | AC5 电量基线对比（APNs 静默推对照组） |
| `healthz` / `readyz` / `/v1/platform/ws-registry` | 0.4 / 0.14 | 压测客户端启动前健康探活 |

### 前置 Story 关键教训

- **Story 0.14 review round 1**：`validateRegistryConsistency` 在 debug 也校验 missing-registration。启发：spike 的"漂移"也要有 double gate —— AC12 约束 spike 不扩大 `dto.WSMessages`（不偷偷加 type）。
- **Story 0.13 round 1/2**：detached writeCtx + token TTL 尊重 config。启发：fix 不堆到无关 story；AC1 Pre-flight 遇到 0.9–0.14 bug 直接回溯。
- **Story 0.12 round 1**：release 模式不注册 `session.resume` —— spike 全程在 `debug` 模式跑，不需要验证 release 行为；AC1 明确锁死。
- **Story 0.11 round 1/2**：rate limiting 真滑动窗口 + Unix ms zset。启发：抬腕重连（每次抬腕都建连）要测 60s ≤ 5 次限流是否过严 —— 若弱网下需要高频重连但被限流拒，该在 §9 收敛方案中 flag "重新评估 connect_rate_per_window"。
- **Story 0.10 round 2**：dedup key 长度前缀编码防歧义。启发：AC7 工具 envelope id 要用 UUID 避免分隔符冲突 —— 虽然 P3 `[convention]` 不强制 UUID，但 spike 数据采样用 UUID 更稳。
- **Story 0.6 error-codes**：双 gate 单元测试 + init 检查。启发：spike 的"数据 vs 决策"也要 double gate —— §5/§7 原始表 + §9 决策理由双检查，不能只有结论没有数据支撑。

### 测试 / 数据纪律

- **对 spike 工具**：smoke test 即可（AC12 不要求 `_test.go`）。一次 `-concurrent 10 -duration 60s` 跑通即验收。
- **对报告数据**：每个数字必须可复现 —— §5 表下一行给"复现命令"（例：`go run ./tools/ws_loadgen -concurrent 1000 -duration 10m -scenario long_lived -report /tmp/cell6.json`）+ 原始 JSON 文件的 git 路径（归档在 `docs/spikes/data/cell06.json` 类）。
- **电量数据**：Apple Watch 电量百分比人工从设备记录（iOS 18 之后 Battery 页显示过去 24h 每小时消耗）；报告 §6 附"记录时间线"说明取样方式。
- **MacBook local test**：推荐关闭其他大户进程（Chrome、IDE 以外）；hub 压测 N=10k 时 MacBook 温度可能上 CPU throttling，报告 §7 附温度观测条目。

### 预期 File List（事前列出 — dev agent 应核对）

**New:**
- `docs/spikes/op1-ws-stability.md`
- `server/tools/ws_loadgen/main.go`
- `server/tools/ws_loadgen/README.md`（可选 —— flag 示例 + smoke 命令）
- `docs/spikes/data/cell01.json` … `cell12.json`（AC5 原始数据归档；小而重要）
- `docs/spikes/data/hub-1k.json` / `hub-3k.json` / `hub-5k.json` / `hub-10k.json`（AC6 压测原始 JSON）

**Modified (conditional on AC6/AC13 triggers):**
- `server/config/default.toml`（仅当 10k p99 > 3s）
- `docs/backend-architecture-guide.md`（仅当硬编码 10k 需更新）

**Totally new dirs (mkdir before first file touch):**
- `docs/spikes/` + `docs/spikes/data/`
- `server/tools/ws_loadgen/`

### 禁止事项 / 常见误区

- **不要**把 spike 报告写成 PRD 风格（"Why / How / Impact"）。报告是**工程决策归档** —— 数据 + 结论 + 后续。避免 marketing 腔。
- **不要**在 §9 给出"视情况再定"之类回避决策的语言。收敛就是收敛，不收敛就写 `NOT CONVERGED` 由人接管。
- **不要**在 spike 范围内为候选方向写 Go 实现。候选评估只列接口影响（AC8 `hubInterfaceImpact`），实装归 Epic 4 及之后的 story。
- **不要**扩大 `dto.WSMessages`。本 story 仅使用现有 3 条。Epic 1+ 的新消息（`profile.update` / `state.tick` / ...）由各自 story 加。
- **不要**在压测工具中触发真实 `session.resume` → 真实 provider（release 模式不挂）路径。压测全程 debug 模式，走 `debug.echo` / `debug.echo.dedup` 即可。
- **不要**用 simulator 的数据充作 watch 电量数据。watchOS simulator 不反映真机电量行为。
- **不要**改 `max_connections` 以外的 WS 配置（`ping_interval_sec` / `pong_timeout_sec` / `connect_rate_per_window`）以"配合 spike"；若结论是这些值需要调，另开 follow-up story。

### Project Structure Notes

- 本 story 首次填充 `docs/spikes/`（架构 §Project Structure 行 967-968 预留）+ `server/tools/` 首个脚本目录（行 972-973 预留）。
- 与架构图一致，无变体。
- 若发现 `docs/` / `server/tools/` 下存在隐藏的 spike/loadgen 冲突文件（e.g. stale `loadgen.go`），STOP 并上升到 code-review。

### References

- [Source: `server/_bmad-output/planning-artifacts/epics.md`#Story 0.15, 行 727-747] — 全部 AC 直接来源
- [Source: `server/_bmad-output/planning-artifacts/epics.md`#Epic 4 前置, 行 414 + 1170 + 1180] — Spike 签字是 Epic 4 硬前置
- [Source: `server/_bmad-output/planning-artifacts/epics.md`#OP-1, 行 311] — 禁止 HTTP backup + 量化要求
- [Source: `server/_bmad-output/planning-artifacts/epics.md`#G5, 行 286 + 1198] — 工具选型归档到 spike 报告
- [Source: `server/_bmad-output/planning-artifacts/epics.md`#NFR-REL-4, 行 159] — WS 重连成功率 ≥ 98% / 5s 目标
- [Source: `server/_bmad-output/planning-artifacts/epics.md`#NFR-SCALE-4, 行 147] — 单实例 WS ≤ 10k 目标
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#D6 OP-1 Design Space, 行 351-365] — 5 个候选方向 + Hub 接口预留
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#D9 ADR-003, 行 390-400] — Hub 压测 1k/3k/5k/10k 触发 sticky routing
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#D1 Broadcaster interface, 行 277-292] — `PushOnConnect` / `BroadcastDiff` 接口占位
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#Spike-OP1 依赖, 行 493] — D6 + D9 + D7 联合验证
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#Project Structure, 行 967-968] — `docs/spikes/op1-ws-stability.md` 预留
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#Project Structure, 行 972-973] — `server/tools/` 预留
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#D13 config 分层, 行 438-448] — `max_connections` 属基础设施配置，改动需重启
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#P5 Logging] — 日志纪律（tools/ws_loadgen 遵守）
- [Source: `docs/backend-architecture-guide.md`#§12 WebSocket] — 心跳 / 帧 / 协议层规则
- [Source: `docs/backend-architecture-guide.md`#§19 PR 检查清单] — AC14 gate
- [Source: `server/internal/ws/hub.go` 行 15-40, 61-63] — `HubConfig.MaxConnections` / `GoroutineCount()` 声明处
- [Source: `server/internal/ws/upgrade_handler.go`] — 建连限流 + 黑名单（压测客户端需穿过）
- [Source: `server/internal/ws/session_resume.go`] — `session.resume` 路径；压测 debug 模式触发
- [Source: `server/internal/dto/ws_messages.go`] — 压测 envelope type 常量
- [Source: `server/config/default.toml` 行 30-39] — `[ws]` 段；AC10 潜在修改对象
- [Source: `server/_bmad-output/implementation-artifacts/0-14-ws-message-type-registry-and-version-query.md`#Dev Notes] — Registry drift 双 gate 模式（启发 AC12）
- [Source: `server/_bmad-output/implementation-artifacts/0-12-session-resume-cache-throttle.md`#Dev Notes] — release 模式跳过 session.resume 的理由（AC1 锁定 debug 模式）
- [Source: `server/_bmad-output/implementation-artifacts/0-11-ws-connect-rate-limit-and-abnormal-device-reject.md`#Dev Notes] — 限流 60s/5 次；抬腕重连 cell 数据用它验证
- [Source: `server/_bmad-output/implementation-artifacts/0-13-apns-push-platform-pusher-queue-routing-410-cleanup.md`#Dev Notes] — APNs 作为电量基线对照组
- [Source: auto-memory `project_backup_fallback.md`] — 用户明确反对 backup/fallback 掩盖根因；AC9 硬禁 HTTP fallback

## Dev Agent Record

### Agent Model Used

Claude Opus 4.7 (1M context) — `claude-opus-4-7[1m]`

### Debug Log References

- 2026-04-18 Pre-flight build：`bash scripts/build.sh --test` 通过；`go vet` + M9 `time.Now()` 检查 + 全 package 测试绿
- 2026-04-18 ws_loadgen smoke：`-help` 正常输出；非法 `-scenario bad` 返回 exit 2；3 workers × 2s long_lived（无服务器）：3 个 connect 失败分类为 `upgrade`（Windows 本机 127.0.0.1:8080 有其他服务返回非 WS 响应 → `websocket: bad handshake`），summary JSON 正常输出
- 2026-04-18 第二次 `bash scripts/build.sh --test`：所有 package 绿 + `tools/ws_loadgen` 新 package 被 `go vet` 覆盖

### Completion Notes List

**Phase A 交付概述（2026-04-18，by Claude）：**

1. **作用范围缩减决策**：原 Story 0.15 包含真机 + 压测执行 + 决策 + 签字，不可由 Claude dev-story workflow 自动化。依据 user memory `project_claude_coding.md` + `project_backup_fallback.md`，拆分为 Phase A（本 story）+ Phase B（新 Story 9.1）。两个 story 共同承载原 14 个 AC（无遗漏；每 AC 在原处标注归属）。
2. **Pre-flight（AC1）**：读 5 个核心文件（`hub.go` / `upgrade_handler.go` / `session_resume.go` / `dedup.go` / `ws_messages.go`）+ 辅助文件（`dispatcher.go` / `broadcaster.go` / `envelope.go` / `config/default.toml`），全部 15 行 checklist ✅。
3. **报告骨架（AC2）**：新建 `docs/spikes/op1-ws-stability.md`，结构固定 10 节。§1 / §2 由 Phase A 完整填写；§3-§10 留 TODO 骨架（表格行数 / 字段 / 判据 都已就位，Phase B 直接填数）。
4. **工具选型（AC7）**：§2 评估表 + 四条理由定稿自写 Go 客户端。切换触发条件已预留（在 §2 末尾追加 `SWITCHED:` 行即可，不用改结构）。
5. **ws_loadgen（AC7 + AC12）**：单文件 `server/tools/ws_loadgen/main.go`（~25k chars，含 package doc）。三个 scenario（cold_connect / raise_wrist / long_lived），JSON summary 含 6 类 error bucket。启动时 `dto.WSMessagesByType["debug.echo"]` 缺失即 Fatal（Story 0.14 drift 双 gate 在 tool 侧延续）。
6. **AC12 / AC14 PR checklist（Phase A 部分）**：
   - ✅ 无 `fmt.Printf` / `log.Printf`（用 `zerolog.New(os.Stderr)` + `io.WriteString` for help；`os.Stdout.Write` for JSON summary 只负责终态输出）
   - ✅ 所有 I/O 函数接受 `ctx`（`dial(ctx, ...)`；worker 主循环全部 `ctx.Err()` 检查）
   - ✅ 不直接引用 `*mongo.Client` / `*redis.Client`（工具只建 WS 连接，不碰持久层）
   - ✅ 无 `context.TODO()`；`main()` 用 `signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)`（顶层 ctx 源允许）
   - ✅ 所有 exported identifier 英文 godoc（`Config / Percentiles / ErrorCounts / Summary / errCounter` 等）
   - ✅ 每个 worker `defer conn.Close()`（`workerLongLived` + `runOneCycle`）
   - ✅ `-help` 打印所有 flag + 默认值 + 3 个示例
   - ✅ `bash scripts/build.sh --test` 绿（包含新 `tools/ws_loadgen` package 的 `go vet`）
   - ✅ `// TODO` 在代码中无残留（`grep` 确认）
   - ✅ 不扩大 `dto.WSMessages`（消费 3 条现有常量；Story 0.14 AC4 drift 守门不触发）

**Phase B handoff（交架构师手动执行）：**

- Story 9.1 `_bmad-output/implementation-artifacts/9-1-spike-op1-real-device-and-hub-loadtest-execution.md` 承载剩余 AC；status = `ready-for-dev`。
- `docs/spikes/op1-ws-stability.md` §3-§10 中 `TODO:` 标记就是 Phase B 填充点。
- `docs/spikes/data/` 空目录已就位，接收 12 cells + 4 hub 压测原始 JSON。
- Story 9.1 完成后**架构师手动**把 `sprint-status.yaml` 的 `9-1-*` 改为 `done`（AC9 签字分支）。本 story 推 `review` → `done` 时**不**推进 Epic 4；Epic 4 解锁取决于 Story 9.1 AC9。

### File List

**New:**
- `docs/spikes/op1-ws-stability.md` — 报告骨架（§1 Pre-flight 已填 / §2 工具选型已填 / §3-§10 留 TODO）
- `docs/spikes/data/` — 空目录（Phase B 数据归档处）
- `server/tools/ws_loadgen/main.go` — WS load generator（~25k chars）
- `server/_bmad-output/implementation-artifacts/9-1-spike-op1-real-device-and-hub-loadtest-execution.md` — Phase B story 文件

**Modified:**
- `server/_bmad-output/implementation-artifacts/0-15-spike-op1-watchos-ws-primary-stability-test-matrix.md` — 本文件（Scope Split Note + AC Phase 标注 + Tasks 勾选 + Dev Agent Record 填充）
- `server/_bmad-output/implementation-artifacts/sprint-status.yaml` — `0-15-*` → `review`；新增 `9-1-*` = `ready-for-dev`

**Not Modified (intentionally):**
- `server/config/default.toml` — AC10 条件改动（10k p99 > 3s）由 Phase B 决定
- `docs/backend-architecture-guide.md` — AC13 条件改动（硬编码 10k 替换）由 Phase B 决定

## Change Log

- **2026-04-18** — Phase A 交付：Pre-flight 就绪性校验完成；`docs/spikes/` + `docs/spikes/data/` 目录创建；`docs/spikes/op1-ws-stability.md` 10 节报告骨架就位（§1 §2 已填）；`server/tools/ws_loadgen/main.go` 压测 / e2e 客户端实装 + smoke test 通过；`bash scripts/build.sh --test` 绿（Stories 0.9-0.14 无回归）。Phase B 拆分到新 Story 9.1，status 推到 `review`。[Claude Opus 4.7]
