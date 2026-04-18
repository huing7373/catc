# Story 9.1: Spike-OP1 — 真机测试执行 + Hub 压测执行 + 收敛决策 + 签字（Phase B）

Status: ready-for-dev

Epic: **Epic 9 — Spike 真机与物理执行**（横向 epic，承载所有需要真机 / 人工执行的 spike 类工作；不阻塞业务 epic 0-8 的推进）

<!-- Note: 本 story 原为 Story 0.16（由 Story 0.15 Phase A 拆分）。为了让 Epic 0（服务端骨架与平台基线）不被真机执行依赖卡死，于 2026-04-18 由用户决策迁至新 **Epic 9**，重命名为 Story 9.1。迁移后 Epic 0 在 0.15 进 done 即可 done；Epic 9 独立推进，Epic 4 解锁条件（Spike-OP1 签字）改为"Story 9.1 AC9 签字"。Phase A 交付物（报告骨架 / 工具选型 / ws_loadgen）已由 Claude 在 Story 0.15 完成并 Merge；Phase B 的所有 AC 都依赖**真实设备执行**或**人工判断**，不能被 AI 代理完成。 -->

## Story

As an architect,
I want to execute the 12-cell watchOS real-device stability matrix + 1k/3k/5k/10k Hub load-test + OP-1 convergence decision against the Story 0.15 Phase A scaffold,
so that `docs/spikes/op1-ws-stability.md` is fully filled, **ADR-003** (Hub sticky-routing trigger) is resolved, and Epic 4 (好友房间 / 环境在场感) is **data-driven** unblocked (or formally `NOT CONVERGED` if the matrix fails).

## 背景：为什么拆分为独立 Story

Story 0.15 原本是一个单体 spike，涵盖 14 个 AC。其中：

| 类别 | AC | 可否自动化 |
|---|---|---|
| **代码 / 文档脚手架** | AC1 Pre-flight / AC2 报告骨架 / AC7 工具选型 + `tools/ws_loadgen/main.go` / AC12 / AC14（仅针对脚手架） | ✅ 已由 Story 0.15 Phase A 完成 |
| **真实设备执行** | AC3 12 cells / AC4 设备清单 / AC5 延迟+电量 | ✗ 需要物理 Apple Watch + iPhone + macOS Network Link Conditioner |
| **压测执行** | AC6 1k/3k/5k/10k Hub 压测 | ✗ 需要 docker-compose Linux 容器或 MacBook；由人工跑 `tools/ws_loadgen` |
| **数据驱动决策** | AC8 候选评估 / AC9 收敛决策 | ✗ 依赖 AC5/AC6 数据 |
| **条件配置改动** | AC10 `max_connections` 下调 | ✗ 条件触发（仅当 10k p99 > 3s） |
| **签字 / 文档同步** | AC11 签字闭合 / AC13 `backend-architecture-guide.md` 同步 | ✗ 依赖决策 |

User memory `project_claude_coding.md`：**Claude 完成 99.99% 编码，瓶颈在美术和真机调试。** 本 story 正是该瓶颈具体化 —— 需要人工在物理设备上花 10-15 小时执行 + 判断。

**拆分遵循反模式约束（user memory `project_backup_fallback.md`）**：不用"后端先出假数据 + 客户端后补"之类 fallback 掩盖真机调试瓶颈；而是明确把人工执行作为独立 story 承载，以数据为唯一依据。

## Acceptance Criteria

> **执行规程：** 本 story 的 ACs 是 Story 0.15 AC3 / AC4 / AC5 / AC6 / AC8 / AC9 / AC10 / AC11 / AC13 的**原文抄录 + 执行指引**。原 AC 编号保留以便回溯（`epics.md` 行 727-747 锚点不变）。

### AC1 — 前置：Phase A 脚手架已可用（自查）

开始执行前快速自查：

- [ ] `docs/spikes/op1-ws-stability.md` 存在且 §1 Pre-flight Checklist 全部 ✅（Story 0.15 Phase A 落档）
- [ ] `docs/spikes/data/` 目录存在（用于归档原始 JSON）
- [ ] `server/tools/ws_loadgen/main.go` 可 `go run`（`-help` 正常打印）
- [ ] 当前分支 commit 包含 0.15 Phase A 的改动（`git log --oneline -5` 核对）
- [ ] `bash scripts/build.sh --test` 绿

若任一失败：STOP，回溯 Story 0.15 Phase A 完成度，不在本 story 修复。

### AC2 — 12 cells 测试矩阵执行（原 Story 0.15 AC3）

完整抄录 `server/_bmad-output/planning-artifacts/epics.md` 行 737 / Story 0.15 AC3：

- 矩阵 = 网络 × 行为场景 = 4 × 3 = 12 cells，**全部执行**。
- 弱网模拟强制用 **macOS Network Link Conditioner**（Additional Tools for Xcode）；**不得**用 tc netem 在服务器侧。
- 每 cell 执行时长 ≥ 30 分钟，期间**不重启客户端进程**。
- 采样量：建连 ≥ 30 次、重连 ≥ 60 次、长连 30 min 单条 timeseries。
- 数据填入 `docs/spikes/op1-ws-stability.md` §3（表格已由 Phase A 留好 12 行 TODO）。

### AC3 — 测试设备真机强制（原 Story 0.15 AC4）

- ≥ 1 台实体 Apple Watch 运行 watchOS 10.0+（MVP 最低支持；记录实测版本号）
- ≥ 1 台配对 iPhone 运行 iOS 17.0+
- simulator / Xcode preview **禁止**作为主数据源
- 服务端：本地 MacBook（`mode = "debug"`）+ 本地 Mongo/Redis
- 记录 CPU 型号 / RAM / Go 版本 / commit hash
- Clock 源：服务端 `clockx.RealClock`；客户端测试工具可选 `FakeClock`（M9 守卫）
- 数据填入 `docs/spikes/op1-ws-stability.md` §4。

### AC4 — 延迟 / 重连 / 电量量化（原 Story 0.15 AC5）

每 cell 记录以下列（单位 ms / %）：
- `connectColdP50 / P95 / P99`
- `reconnectRaiseWristP50 / P95 / P99`（无抬腕的 cell 填 `N/A`）
- `reconnectWithin5sRate`（NFR-REL-4 目标 ≥ 98%）

电量：
- 30 min 长连 WS-primary → 记录 Apple Watch 电量下降
- 基线对比：30s 轮询 HTTP + APNs（最小脚本，不进主仓库）→ 记录同样时段的电量下降
- 差值 > 5pp → `docs/spikes/op1-ws-stability.md` §6 明确讨论权衡

达标判据：`reconnectWithin5sRate ≥ 98%` 在 ≥ 10/12 cells 成立 = NFR-REL-4 通过。弱网 + 网络切换 cells 若低于 98% 需在 §6 解释。

**原始 JSON 归档：** 每 cell 一个 `docs/spikes/data/cell01.json` … `cell12.json`，**由真机 Watch 客户端遥测写出**（字段见报告 §5）。**不得**用 `tools/ws_loadgen` 输出填 cell JSON —— 工具度量的是 hub 侧 `debug.echo` RTT，不是 AC4 所定义的"首个 `session.resume.result`"端到端时间；参见 Story 0.15 round 1 review 的第二条发现（工具 `cold_connect` / `raise_wrist` 输出会系统性低估真实重连延迟）。

### AC5 — Hub 上限压测 / ADR-003 决策触发（原 Story 0.15 AC6）

压测档位：N ∈ {1k, 3k, 5k, 10k}，每档稳态运行 ≥ 10 min。

环境：
- 优先 docker-compose Linux 容器（调高 `net.ipv4.ip_local_port_range` + `nofile` ulimit）
- 次选 MacBook M1/M2 8-16G 本机
- Windows 本机不推荐（TIME_WAIT 回收慢污染数据）

记录列：
- `N / connectSuccessRate / broadcastLatencyP95 / broadcastLatencyP99 / cpuPct / memRSS_MB / goroutineCount`
- `goroutineCount` 来源：`Hub.GoroutineCount()`（internal/ws/hub.go 行 61-63）= `ConnectionCount() × 2`

**触发点（AC10 联动）**：若 10k 下 `broadcastLatencyP99 > 3000ms`：
1. 下调 `config/default.toml` `[ws].max_connections` 至满足 `p99 ≤ 3s` 的最大档位（如 5000），相邻加注释 `# max_connections tuned by Story 9.1 Phase B YYYY-MM-DD, see docs/spikes/op1-ws-stability.md §7`
2. 在报告 §7 末专列 `### Phase 3 Sticky Routing Planning`（触发条件 + 扩展路径 + 工作量 S/M/L）
3. §9 收敛方案必须接受下调后的上限
4. 跑 `bash scripts/build.sh --test` 验证回归绿

**若 10k p99 ≤ 3s：** 保留 `max_connections = 10000`，§7 明确 `decision: keep 10k, no sticky-routing planning needed for MVP`。

**压测工具：复用 Story 0.15 Phase A 的 `server/tools/ws_loadgen`。** 不重写；不引第三方依赖。

复现命令示例：

```bash
# 1k 档（本机 MacBook 可行）
go run ./tools/ws_loadgen \
  -url ws://127.0.0.1:8080/ws \
  -concurrent 1000 \
  -duration 10m \
  -scenario long_lived \
  -send-interval 2s \
  -token-prefix "loadgen-" \
  -report docs/spikes/data/hub-1k.json

# 10k 档（docker-compose Linux 容器推荐）
# 先 docker-compose up 后在容器内执行同上命令（路径调整）
```

**原始 JSON 归档：** `docs/spikes/data/hub-1k.json` / `hub-3k.json` / `hub-5k.json` / `hub-10k.json`。

### AC6 — OP-1 候选方向可行性评估（原 Story 0.15 AC8）

5 个候选方向（固定，禁止新增 — 架构 §D6 已收敛）：

1. 客户端 cache-first + 差分更新（需 `BroadcastDiff`）
2. 服务端主动 session.resume 推送（需 `PushOnConnect`）
3. 精细化抬腕事件重连（Hub 无变化）
4. WS permessage-deflate 压缩（Hub 配置扩展）
5. `NWConnection` 深度适配（Hub 无变化）

数据填入 `docs/spikes/op1-ws-stability.md` §8：
- `feasibleGivenData`（√ / × / ?）
- `effortEstimate`（S / M / L）
- `hubInterfaceImpact`（"none" / "uses existing `BroadcastDiff`" / "uses existing `PushOnConnect`" / "adds `EnableCompression` flag" / "client-only, no hub impact"）
- `riskLevel`（Low / Med / High）

**硬约束：** 必须明确验证 D6 预留接口 `PushOnConnect` + `BroadcastDiff`（`internal/ws/broadcaster.go` 行 16-17）能够满足**至少 2 个**候选方向落地。若数据不支持，§9 必须升级为架构风险 flag。

### AC7 — 单一方向收敛（原 Story 0.15 AC9）

§9 必须给出**一个**且仅**一个**主方向（允许 1 个次要方向作为 "plan B fallback on the same WS-primary approach"，但**不得**是 HTTP backup 类）。

必须字段：
- `chosenDirection`（§8 表行号 1-5）
- `rationale`（引用 §5 / §7 数据，≥ 3 句理由）
- `hubInterfaceChanges`（D1 Broadcaster interface 后续 Epic 4 实装映射）
- `serviceLayerChanges`（RoomService / PresenceService 与 Broadcaster 耦合点）
- `clientSideChanges`（仅接口 + 行为，不写 Swift）
- `metricsToMonitor`（D14 Metrics 预留字段名 + 含义）

**硬禁止**（user memory `project_backup_fallback.md`）：
- "HTTP polling fallback" / "short-poll backup" / "hybrid WS+HTTP heartbeat" 即违反 AC。

**NOT CONVERGED 分支**：若数据结论为"当前 WS-primary 不可行"，§9 明确写 "NOT CONVERGED"，本 story 仍可推进到 done（采数据 = 有效完成），但 Epic 4 无法启动（AC9 签字走 `BLOCKED_SEE_SECTION_9`）。

### AC8 — `cfg.WS.MaxConnections` 改动（条件触发；原 Story 0.15 AC10）

仅在 AC5 压测结论要求下调时：
- 改 `config/default.toml` 行 31 `max_connections = 10000` 为通过档位
- 同步修改 `config/development.toml` 若存在（当前仅 `default.toml`）
- 加注释 `# max_connections tuned by Story 9.1 Phase B YYYY-MM-DD, see docs/spikes/op1-ws-stability.md §7`
- 跑 `bash scripts/build.sh --test` 验证 healthcheck / upgrade_handler 测试（Stories 0.4 / 0.11）仍绿

若 AC5 无需下调：**不改任何配置**；禁止"proactively lower for safety"。

### AC9 — 架构师签字闭合 + 解锁 Epic 4（原 Story 0.15 AC11）

§10 "Architect Sign-off" 必须包含：
- `signoffDate`（不得早于所有 cells 执行完）
- `signoffBy`（架构师即开发者本人，user memory `project_claude_coding.md`）
- `epic4Status`（`UNBLOCKED` / `BLOCKED_SEE_SECTION_9`，与 §9 匹配）
- `followupStories`（Epic 4 首个 story `4-1-presence-lifecycle-d8-ws-disconnect-leave-room` 是否可按 `epics.md` 行 1180 前置开始）

**签字后 sprint-status 动作（由架构师手动执行）**：

| epic4Status | sprint-status.yaml 动作 |
|---|---|
| `UNBLOCKED` | `9-1-*` 标记 `done`；若 `epic-9` 所有 story 都 `done` 则 `epic-9` 自身也标 `done`；`epic-1` … `epic-8` **保持 `backlog`**（本 story 只解锁 Epic 4，不触发其他业务 epic） |
| `BLOCKED_SEE_SECTION_9` | `9-1-*` 标记 `done`（spike 数据采集完成 = 有效完成，结论不可行仍算 done）；新增注释 `# project-note: OP-1 blocked; Epic 4 paused pending protocol redesign`；`epic-9` 若仅本 story 则同样标 `done` |

### AC10 — 文档与 backend-architecture-guide 同步（条件触发；原 Story 0.15 AC13）

- 若 AC5 触发 `max_connections` 下调：`docs/backend-architecture-guide.md` §12 WebSocket 段 / NFR 列表若有"10k"硬编码，更新为"Spike-OP1 tuning 后的实测值"。定位：`grep -n "10,000\|10k" docs/backend-architecture-guide.md`
- 报告尾部显式链回 `backend-architecture-guide.md` + `architecture.md` §D6 §D9
- **不**在 `architecture.md`（planning artifact 只读档案）追溯性修订；**仅**改 `backend-architecture-guide.md`（构建期约束文档）。

### AC11 — PR checklist 最终自查（原 Story 0.15 AC14 剩余项）

对本 story 最终 diff 逐项自查：

- [ ] 无 `fmt.Printf` / `log.Printf`（§P5 lint；仅新增改动）
- [ ] 若动了 `config/default.toml`（AC8）：`bash scripts/build.sh --test` 绿
- [ ] 若动了 `docs/backend-architecture-guide.md`（AC10）：语法无 broken links
- [ ] `docs/spikes/op1-ws-stability.md` 全部 `TODO:` 已填（§3 / §4 / §5 / §6 / §7 / §8 / §9 / §10）
- [ ] `docs/spikes/data/` 内 12 cells + 4 hub 压测 JSON 文件全部就位
- [ ] Story 0.15 Phase A 的 `// TODO` 残留检查：`grep -n "TODO" server/tools/ws_loadgen/` 应为空
- [ ] 不新增 `dto.WSMessages` 条目（AC12 跨 story 共同守门）

## Tasks / Subtasks

- [ ] **Task 1 — Phase A 脚手架自查** (AC: #1)
  - [ ] 读 `docs/spikes/op1-ws-stability.md` §1 确认所有 ✅
  - [ ] `ls docs/spikes/data/`（应为空目录，待填）
  - [ ] `go run ./tools/ws_loadgen/ -help` 正常打印
  - [ ] `bash scripts/build.sh --test` 绿
- [ ] **Task 2 — Watch/iPhone 客户端原型准备**（本 story 外前置，但必须存在）
  - [ ] 确认存在可连接 `ws://<server>/ws` 的 watchOS app（能发 `debug.echo` + 记录延迟；若没有，先开 follow-up "watchOS 最小 WS 测试客户端" story）
  - [ ] Network Link Conditioner profile 装好（10% 丢包）
  - [ ] Apple Watch + iPhone 电量都充满 100%
- [ ] **Task 3 — 执行 12 cells 测试矩阵** (AC: #2, #3, #4)
  - [ ] cell 1-3（Wi-Fi）：冷启动 / 抬腕 / 长连；每 cell ≥ 30min
  - [ ] cell 4-6（4G LTE）
  - [ ] cell 7-9（弱网 10% 丢包）
  - [ ] cell 10-12（BT↔LTE 切换）
  - [ ] 每 cell 原始 JSON 入 `docs/spikes/data/cellNN.json`
  - [ ] 报告 §3 / §5 填表
- [ ] **Task 4 — 电量基线对比** (AC: #4)
  - [ ] Watch 长连 30min 电量
  - [ ] Watch 30s HTTP 轮询 30min 电量
  - [ ] 报告 §6 填表 + 取样时间线
- [ ] **Task 5 — Hub 压测 1k/3k/5k/10k** (AC: #5)
  - [ ] 选环境：docker-compose 容器 / MacBook
  - [ ] 4 档 × 10min 跑完
  - [ ] 原始 JSON `docs/spikes/data/hub-Nk.json`
  - [ ] 报告 §7 填表
- [ ] **Task 6 — 候选方向可行性评估** (AC: #6)
  - [ ] §8 表逐行填 `feasibleGivenData / effortEstimate / hubInterfaceImpact / riskLevel`
  - [ ] 验证 D6 预留接口足以承载 ≥ 2 候选
- [ ] **Task 7 — 收敛方向决策** (AC: #7)
  - [ ] §9 单一方向 + 次要方向（如必要，且仍 WS-primary）
  - [ ] 数据驱动 `rationale` ≥ 3 句
  - [ ] 列全 6 个必须字段
  - [ ] grep "HTTP polling\|short-poll\|hybrid.*heartbeat" 自查无
- [ ] **Task 8 — 条件配置改动**（仅在 AC5 触发）(AC: #8)
  - [ ] 改 `config/default.toml` `[ws].max_connections`
  - [ ] 加注释
  - [ ] `bash scripts/build.sh --test` 验证回归
- [ ] **Task 9 — 签字闭合** (AC: #9)
  - [ ] §10 `signoffDate / signoffBy / epic4Status / followupStories`
  - [ ] 若 UNBLOCKED：手动更新 `sprint-status.yaml` `9-1-*` → `done`
  - [ ] 若 BLOCKED：手动更新 `sprint-status.yaml` + 加 project-note 注释
- [ ] **Task 10 — 文档同步 + PR checklist** (AC: #10, #11)
  - [ ] 视结论改 `docs/backend-architecture-guide.md`（硬编码 10k 替换）
  - [ ] 报告尾部链回
  - [ ] `bash scripts/build.sh --test` 最终绿
  - [ ] §19 checklist 每条过

## Dev Notes

### 核心定位

**本 story 是 Story 0.15 Phase B，承载所有需要真机执行 / 人工判断的工作。**

- **触发条件：** Phase A（Story 0.15）已 Merge 到 main；`docs/spikes/op1-ws-stability.md` + `server/tools/ws_loadgen` 已可用。
- **完成方式：** 架构师（开发者本人）手动执行真机测试 + 压测 + 填数据 + 签字 + **手动**改 sprint-status。
- **不是** 由 Claude dev-story workflow 自动完成 —— workflow 对本 story 作用仅限于脚手架化 tasks / 引导自查。

### 关键约束 / 反模式

- **不修改 Phase A 已交付代码**（`tools/ws_loadgen/main.go`）除非发现 bug。若修 bug：开独立 follow-up story（参考 Story 0.13 round 1/2 的 review-log 流程），不堆到本 story。
- **不引入新的第三方依赖**（与 Phase A AC12 延续）。
- **不引入 HTTP fallback backup**（user memory `project_backup_fallback.md`；AC7 硬禁）。
- **不把 spike 工具塞进 `internal/`**（`tools/` 本就是 one-off 归宿）。
- **不把 spike 结论写成 "视情况再定"**。收敛就是收敛，不收敛就写 `NOT CONVERGED`。

### 预期 File List（事前列出 — 架构师执行过程核对）

**Filled (not new — 仅填 Phase A 留的 TODO)**:
- `docs/spikes/op1-ws-stability.md` §3 / §4 / §5 / §6 / §7 / §8 / §9 / §10 所有 TODO

**New（数据归档）**:
- `docs/spikes/data/cell01.json` … `cell12.json`（12 个）
- `docs/spikes/data/hub-1k.json` / `hub-3k.json` / `hub-5k.json` / `hub-10k.json`（4 个）

**Modified (conditional on AC5/AC10 triggers)**:
- `server/config/default.toml`（仅当 10k p99 > 3s）
- `docs/backend-architecture-guide.md`（仅当硬编码 10k 需更新）
- `server/_bmad-output/implementation-artifacts/sprint-status.yaml`（签字后手动）

### Story 0.15 Phase A 已交付资产（可直接复用）

| 资产 | 位置 | 用法 |
|---|---|---|
| 报告骨架（10 节 TODO） | `docs/spikes/op1-ws-stability.md` | 填 §3 / §5 / §6 / §7 / §8 / §9 / §10 数据 |
| 数据归档目录 | `docs/spikes/data/` | 原始 JSON 直存 |
| ws_loadgen 工具 | `server/tools/ws_loadgen/main.go` | **仅用于 AC5 Hub 压测**（推荐 `-scenario long_lived`）；不适用于 AC2-AC4 cell 矩阵（工具只度量 `debug.echo` RTT，不走 session.resume 链路，见 Story 0.15 review round 1） |
| Pre-flight 已勾选 | 报告 §1 | 本 story 只需自查不需重填 |
| §2 工具选型已定 | 报告 §2 | 直接用自写 Go 客户端 |

### References

- [Source: `server/_bmad-output/implementation-artifacts/0-15-spike-op1-watchos-ws-primary-stability-test-matrix.md`] — Phase A 交付物
- [Source: `docs/spikes/op1-ws-stability.md`] — Phase A 报告骨架（本 story 填 TODO）
- [Source: `server/_bmad-output/planning-artifacts/epics.md`#Story 0.15, 行 727-747] — 所有 AC 原文来源
- [Source: `server/_bmad-output/planning-artifacts/epics.md`#Epic 4 前置, 行 414 + 1170 + 1180] — Spike 签字是 Epic 4 硬前置
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#D6, 行 351-365 / #D9 ADR-003, 行 390-400] — 候选方向 + Hub 压测触发
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#D1 Broadcaster, 行 277-292] — `PushOnConnect` / `BroadcastDiff` D6 预留
- [Source: `server/internal/ws/broadcaster.go` 行 13-18] — 4 个 Broadcaster 方法签名
- [Source: `server/internal/ws/hub.go` 行 19, 行 61-63] — `HubConfig.MaxConnections` / `GoroutineCount()` 供 AC5 压测
- [Source: `server/config/default.toml` 行 31] — AC8 潜在修改对象
- [Source: `docs/backend-architecture-guide.md`#§12 WebSocket / §19 PR checklist] — AC10 / AC11 gate
- [Source: auto-memory `project_backup_fallback.md`] — 反对 backup 掩盖根因；AC7 硬禁 HTTP fallback
- [Source: auto-memory `project_claude_coding.md`] — Claude 99.99% 编码；真机调试是人工瓶颈（本 story 拆分前提）

## Dev Agent Record

### Agent Model Used

N/A — 本 story 由架构师手动执行，非 AI 代理驱动。

### Debug Log References

_由架构师填：执行过程遇到的问题 + 重要决策轨迹。_

### Completion Notes List

_由架构师填：每个 Task 完成时写一句话总结 + 关键结论。_

### File List

_由架构师填：最终落盘的所有数据 JSON + 修改的 config / guide 文件。_
