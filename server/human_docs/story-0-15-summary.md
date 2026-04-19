# Story 0.15: Spike-OP1 — watchOS WS-primary 稳定性真机测试矩阵（Phase A 服务端脚手架）— 实现总结

本 story 原本是一次 spike —— 要在真实 Apple Watch + iPhone 上跑 12 个 cell × 30 min 弱网矩阵 + 1k/3k/5k/10k Hub 压测，用数据驱动 OP-1（好友在场感协议）的收敛决策，解锁 Epic 4。但 **spike 的绝大多数 AC 需要物理设备 + macOS Network Link Conditioner + 10–15 小时真机测试**，user memory `project_claude_coding.md` 明确这是 Claude 编码触达的瓶颈。于是本 story 按 user 指示拆为两段：

- **Phase A（本 story）= 服务端可自动化脚手架**：`docs/spikes/op1-ws-stability.md` 10 节报告骨架 + `tools/ws_loadgen` 自写 Go 压测客户端 + §2 工具选型 §1 Pre-flight 填完 + §3-§10 留 TODO 给真机数据
- **Phase B（拆到新 Epic 9 的 Story 9.1）= 真机 + Hub 压测执行 + 收敛决策 + 签字**：由架构师手动完成；`epic-9: in-progress` 独立推进，**不阻塞 Epic 0 完结**

本 story 最终 `done`，Epic 0（0-1..0-15）整体 `done`。Phase B 签字（Story 9.1 AC9）仍是 Epic 4 解锁的硬前置，但这个依赖关系已经在独立 epic 里被合理地隔离。

**这是一个"拆分也是交付"的 story** —— 除了代码，最有价值的是把"哪些 AC 能由 AI 完成 / 哪些必须人类在真机上做"这件事用一套 `[Phase A]` / `[Phase B → 9.1]` 标注逐 AC 显式化，后续 reviewer / 未来 Claude 不用再猜。

## 做了什么

### `docs/spikes/op1-ws-stability.md` — 报告骨架（AC2）

- 目录 `docs/spikes/` 本 story 首次创建（架构 §Project Structure 预留位），同步建空目录 `docs/spikes/data/` 附 `.gitkeep` 说明未来填什么
- 报告结构**固定 10 节**，顺序与标题严格锁死便于后续锚点引用：
  1. Pre-flight Checklist（本 story 填完 ✅）
  2. Test Tool Selection（本 story 填完）
  3. Test Matrix（12 cells 表留 TODO）
  4. Device & Environment（真机设备清单留 TODO）
  5. Latency & Reliability Metrics（AC5 p50/p95/p99 表留 TODO）
  6. Battery Baseline（30min 电量对比留 TODO）
  7. Hub Load Test (ADR-003)（1k/3k/5k/10k 表留 TODO）
  8. Candidate Feasibility（5 候选方向评估表留 TODO）
  9. Convergence Decision（收敛主方向 + 次方向留 TODO）
  10. Architect Sign-off（签字块留 TODO；解锁 Epic 4 的唯一动作）
- §5 顶部加**大写警告**："`connectColdMs*` / `reconnectRaiseWristMs*` 只能由真机 Watch 客户端遥测写出；**不得**用 `ws_loadgen` 输出填本节" —— 这是 round 1 review 才显式化的约束，填坑的是 Claude 自己之前注释里的模糊话术
- §7 Hub 压测表加 `reconnectRatio` 列 + 稳态判据（round 1 引入，round 2 修正分母）
- "链接回溯"段尾部显式链回 `backend-architecture-guide.md` + `architecture.md` §D6 §D9，便于 Phase B reviewer 快速对齐

### `server/tools/ws_loadgen/main.go` — 自写 Go 压测客户端（AC7 / AC12）

- 目录 `server/tools/ws_loadgen/` 本 story 首次创建（架构预留）；**一个文件 `main.go`**，无 `_test.go`（AC12：tools 是 one-off）
- 唯一依赖：已在 `go.mod` 的 `gorilla/websocket` + `google/uuid` + `rs/zerolog` + `internal/dto`；**不引任何新第三方依赖**（AC12 硬约束）
- Flags：`-url`, `-concurrent`, `-duration`, `-send-interval`, `-scenario={cold_connect|raise_wrist|long_lived}`, `-report`, `-token-prefix`, `-verbose`
- 三个 scenario，语义**严格是 hub 侧压测**：
  - `cold_connect`：每次开 WS → 发 1 个 `debug.echo` → 关；repeat until duration
  - `raise_wrist`：`cold_connect` + 随机 1–5 s sleep；撞 Story 0.11 rate limiter（5 req/60s）是**设计**
  - `long_lived`：开一次 WS → 按 `-send-interval` 发 `debug.echo` → read/write 错误自动重连（round 1 修）；这是 AC6 Hub 压测的正确 scenario
- 启动时 `dto.WSMessagesByType["debug.echo"]` 缺失即 Fatal —— **Story 0.14 drift 双 gate 在工具侧延续**，未来哪天 `dto.WSMessages` 删了这条、工具立刻炸，不会静默发错误 envelope
- JSON Summary 字段：`config / connectSuccess / connectFailures / reconnectAttempts / echoSamples / connectLatencyMs{p50,p95,p99} / echoRttMs{p50,p95,p99} / errors{dial,upgrade,write,read,parse,mismatch}`
- 日志：`zerolog` 到 stderr；最终 Summary JSON 直写 stdout 或 `-report` 文件
- Ctrl-C 响应：`signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)` —— 即使中断也产出 partial summary
- Percentiles 用 nearest-rank + `sort.Float64s` 本地计算（无统计库依赖）；样本缓冲 `sampleBuffer` 加锁（goroutine-safe append）
- AC12 / §19 PR checklist 逐项过：无 `fmt.Printf` / `log.Printf`；每 worker `defer conn.Close()`；exported 全英文 godoc；`-help` 含 3 个示例

### Story 拆分：本 story 与新建 Story 9.1（Epic 9）

- 本 story 文件头部加 "Scope Split Note (2026-04-18)"，表格形式逐 AC 标 `[Phase A]` 还是 `[Phase B → 9.1 ACx]`
- 新建 `0-16-spike-op1-real-device-and-hub-loadtest-execution.md`（首次命名）
- **第二次用户指示："把这个 story 放到独立 epic，让 Epic 0 能完结"** → 重命名 `0-16-*.md` 为 `9-1-*.md`，新增 `sprint-status.yaml` 的 `epic-9` 段：
  - `epic-9: in-progress`（横向 epic，承载所有需要真机/人工执行的 spike 类工作）
  - `9-1-spike-op1-real-device-and-hub-loadtest-execution: ready-for-dev`
  - `epic-9-retrospective: optional`
- Story 9.1 内头部明确 `Epic: Epic 9 — Spike 真机与物理执行`；`sprint-status 动作` 表改成"签字后 `9-1-*` done；若本 epic 仅此 story 则 `epic-9` 同步 done；`epic-1..epic-8` 保持 backlog 不触发"
- 跨文件引用（`0-15-*.md` / `docs/spikes/op1-ws-stability.md` / `9-1-*.md`）里的 "Story 0.16" / "0-16-*" 全部 replace 为 "Story 9.1" / "9-1-*"；保留少量 "原为 0.16" 的历史面包屑，方便未来 git archaeology

### sprint-status.yaml 演化（4 次编辑）

1. 创建 Phase A 时：`0-15-*` 从 `ready-for-dev` → `in-progress`，加 `0-16-*: ready-for-dev` 在 epic-0 段
2. 拆 epic 时：`0-16-*` 从 epic-0 段移出，在文末新增完整 epic-9 块
3. 走 review：`0-15-*` → `review`
4. 走 done：`0-15-*` → `done`；**`epic-0: in-progress` → `done`**（0-1..0-15 全绿，retrospective optional 不阻塞）

### review round 1 修复（commit `254165b`）

**两条 P1 来自 `/code-review`**：

- **`workerLongLived` 首次 read/write/echo 错误就 `return` 永久退出** —— 10 min ADR-003 压测中任何瞬时错误都让该 worker 永久离场，N 静默塌陷，`broadcastLatencyP95/P99` 比配置档位乐观得多。修复：外层循环 + `reconnectBackoff = 250ms` + 新增 `Summary.ReconnectAttempts` 字段
- **`cold_connect` / `raise_wrist` scenario 误标 "Models NFR-REL-4 measures"**，但代码只发 `debug.echo`，不走 `session.resume` 链路。AC5 的 `connectColdMs*` 定义是"首个 `session.resume.result` 端到端时间"，语义完全不同。修复：package doc + scenario godoc 明确声明 "hub-side only, NOT AC5"；报告 §5 顶部加"数据来源必须是真机 Watch 客户端遥测"；§7 Hub 压测表加 `reconnectRatio` 列

### review round 2 修复（commit `66442fa`）

round 1 的 reconnect 指标本身还有两个度量偏差：

- **`workerLongLived` 在第二次循环起无条件 `reconnects.Add(1)`**，包括"启动期 dial 反复失败的初次建连重试"。Summary godoc 说的是"after the first successful connect"，代码与 godoc 不符。修复：加 `everSucceeded` 闸门；`runLongLivedSession` 改为返回 `bool`；只在 `everSucceeded == true` 时才累加 reconnects。smoke 验证：服务器未起时 16 次 dial-fail 现 `reconnectAttempts=0`（round 1 版会错报 16）
- **`reconnectRatio = reconnectAttempts / connectSuccess`** 分母把"所有成功 dial（含重连成功）"都算进来，系统性稀释 ratio 约 `R/(N+R)`。边界反例：1000 worker × 50 reconnects → 旧公式 `50/1050=4.76%`（判稳态）；按文档语义应 `50/1000=5.0%`（触发非稳态标签）。修复：分母改 `Config.Concurrent`；语义改 "average reconnects per worker"；godoc + 报告 §2 §7 同步

## 怎么实现的

**为什么 Phase A / Phase B 拆分不是"偷懒"而是纪律**：最初 story 0.15 的 Tasks 5-9 要求"30 min × 12 cells 真机执行"、"每 cell ≥ 30 次抬腕样本"、"Apple Watch 电量百分比人工从 Settings → Battery 读"。这些步骤无法被任何 AI agent 代替 —— user 自己的 memory 已经承认"瓶颈在真机调试"。继续在 Phase A 里假装能做，或者用"mock 数据填 §5 先过 AC"都是 user memory `project_backup_fallback.md` 明确反对的"用 fallback 掩盖根因"。正确答案是把不能做的部分**显式**留到一个独立 story，而不是用沉默把它吸收掉。

**为什么独立一个 Epic 9 而不是让 9.1 留在 Epic 0**：Epic 0（服务端骨架与平台基线）的"完成"语义是"骨架可用、可以开始写业务"。如果 9.1 留在 Epic 0，user 想推进 Epic 1-8 任何业务 story 时，"Epic 0 还 in-progress"会是一个误导信号——真实情况是服务端 15 个骨架 story 全 done 了，只差架构师自己去做真机测试。把 9.1 放进独立横向 epic（"Spike 真机与物理执行"）让两件事各自有清晰状态：Epic 0 已完结；Epic 9 里的一个 story 等真机。Epic 4（好友房间）解锁仍硬依赖 9.1 AC9 签字，这个依赖关系**没变**，只是在跨 epic 而不是 intra-epic 表达。

**为什么 `ws_loadgen` 不自动走 `session.resume`**：Story 0.15 Dev Notes 明文禁"在压测工具中触发真实 session.resume → 真实 provider 路径"。表面原因是 release 模式 session.resume 未注册（Story 0.12 决策），深层原因是 hub 压测和 e2e 会话恢复是**两条独立测量轴**：
- Hub 压测关心"Hub 在 N 并发下 broadcast 能不能 p99 < 3s"——与 provider 链路无关
- E2E session.resume 关心"Mongo / provider fan-out 在重连风暴下响应如何"——与 hub 并发规模无关
把两件事塞进一个工具会产生"CPU 瓶颈到底来自 hub 还是 provider"的不可归因性。分开度量才对。

**为什么 `reconnectBackoff = 250ms` 是常量而不是 flag**：flag 的每个选项都需要一条语义理由（why 250 vs 500 vs 1000）。250ms 的选择理由：Story 0.11 rate limiter 是 5 req/60s（12s 一次 token）；250ms 快到能让"真瞬时错误"的重连几乎不丢数据点，但不至于把被 rate-limit 的 worker 连到另一个 token 补发之前又撞一次（rate-limit 拒绝也是一次 dial attempt，会无限打爆 Redis）。250ms 是介于"spin 和 throttle"的合理中间点，改成 flag 反而让 reviewer / 未来使用者不得不思考——一个 one-off spike tool 不值得这个 cognitive load。

**为什么 `everSucceeded` 闸门要在外层而不是在 `runLongLivedSession`**：如果在 inner 函数里判断"这次 dial 是不是 reconnect"，需要把 `everSucceeded` 这个状态作为参数传进去，或者让 inner 函数带副作用（`everSucceeded = true`）。更干净的 layering 是 inner 函数**只做单次 dial→run→teardown**，返回"dial 是否成功"的 bool；outer 函数持有 worker 级状态（`everSucceeded`）。这样 `runLongLivedSession` 可以独立推理：单次会话、错误上报到外部计数器、返回 bool。未来如果要抽第四种 scenario（比如"多重连后退出而非无限重连"），outer 层再加一个计数上限，inner 不需改动。

**为什么 `reconnectRatio` 分母用 `Config.Concurrent` 而不是 `ConnectSuccess - ReconnectAttempts`（"initial connects"）**：后者看起来更直观（只算"首次建连"），但有两个问题：
1. 并非每个 worker 都能完成首次建连（服务器已经达到上限、rate-limit、网络失败）。用"成功的初次建连"当分母，会把"没建上的 workers"从 ratio 里一起排除——掩盖了"有些 worker 根本没参与"的事实
2. `Config.Concurrent` 是**固定值**（N = 1000 / 3000 / ...），一眼可知、一目了然。任何 `reconnects / ?` 的读数都能在几秒内在脑子里换算——0.05 = 每 20 个 worker 1 次重连。换成动态分母反而需要先找出当时 ConnectSuccess 是多少

**为什么 review round 1 / 2 都是"指标正确性"而不是"功能正确性"**：Story 0.15 的 ws_loadgen 功能上很简单（连 WS、发消息、计时）。真正的风险是**度量被错误解读**——spike 本身就是"生产数据以驱动决策"，如果数据含糊，整个 spike 的价值归零。两轮 review 刚好都命中这条：round 1 命中"指标的正确性"（worker 不重连 / 不是 AC5），round 2 命中"指标的分母"（何时算 reconnect / 该除什么）。这对未来做类似 spike 的人是一个警示：代码 diff 小不等于 review 压力小；度量工具的 review 重点是**语义**而非**语法**。

## 怎么验证的

**单元测试与集成测试**：本 story 无新增 `_test.go`（AC12 明确 tools/ 不要求）。`tools/ws_loadgen` 被 `go vet ./...` 覆盖（`scripts/build.sh` 扫全仓库）。

**Smoke 手测（三次，贯穿 Phase A + 两轮 review）**：

- Phase A smoke（commit `254165b`）：`go run ./tools/ws_loadgen -concurrent 10 -duration 60s -scenario cold_connect` 以及 3 workers × 2s `long_lived`，summary JSON 落盘正常
- round 1 smoke：同命令后 `long_lived` 不再于首次错误退出；服务器未起时 `reconnectAttempts=16 connectFailures=16`（每 250ms 一轮）
- round 2 smoke：`reconnectAttempts` 从 16 改回 `0`（启动期 dial-fail 不再误记为 reconnect）—— 这是"测试是否修好"的关键反例

**构建验证**：

- `bash scripts/build.sh --test` —— 三次（Phase A、round 1、round 2）全绿；覆盖 `go vet`、`check_time_now` M9 检查、`go build`、全仓库 `go test`
- 所有已落地的 Stories 0.9-0.14 回归测试 100% 通过（没被 `tools/ws_loadgen` 新增 package 影响）

**PR checklist §19 自查**（Phase A diff）：

- ✅ 无 `fmt.Printf` / `log.Printf`（`zerolog.New(os.Stderr)` + `os.Stdout.Write` 分流）
- ✅ 所有 I/O 接 `ctx`（`dial(ctx,...)`；worker 主循环 `ctx.Err()` / `<-ctx.Done()`）
- ✅ 无 `*mongo.Client` / `*redis.Client` 直引（工具只建 WS）
- ✅ 无 `context.TODO()`；`main()` 用 `signal.NotifyContext(context.Background(), ...)`（顶层 ctx 源允许）
- ✅ 所有 exported identifier（`Config / Percentiles / ErrorCounts / Summary / errCounter`）英文 godoc
- ✅ 每 worker `defer conn.Close()`
- ✅ `-help` 含 flag 默认值 + 3 示例
- ✅ `// TODO` 在 `tools/ws_loadgen/` 无残留（`grep` 确认）
- ✅ 不扩 `dto.WSMessages`（Story 0.14 AC4 drift 守门不触发）

## 后续 story 怎么用

- **Story 9.1（Phase B 执行）**——架构师接手真机工作的全部入口，需要按顺序完成：
  1. 准备 Apple Watch（watchOS ≥ 10）+ iPhone（iOS ≥ 17）+ macOS Network Link Conditioner + 能连 `ws://server/ws` 的 watchOS 最小原型（本 story 外前置；若没有，先开 follow-up）
  2. 按 `docs/spikes/op1-ws-stability.md` §3 / §4 / §5 / §6 的 TODO 表逐 cell 填数据；12 cells × 30 min 真机采样 + 电量对比
  3. `go run ./tools/ws_loadgen -concurrent {1000,3000,5000,10000} -duration 10m -scenario long_lived -report docs/spikes/data/hub-Nk.json` 跑 Hub 压测（§7 AC5）
  4. §8 / §9 填候选评估 + 收敛决策（**硬禁** HTTP polling / short-poll / hybrid WS+HTTP heartbeat 字样）
  5. §10 架构师签字（`signoffDate / signoffBy / epic4Status / followupStories`）
  6. 手动改 `sprint-status.yaml`：`9-1-* → done`；若 `epic4Status = UNBLOCKED`，则 `epic-9 → done`（此 epic 仅此一条 story）
- **Epic 4（好友房间）**——Story 4.1 的前置条件已从"Story 0.15 签字"改为"**Story 9.1 AC9 签字**"。SM 创建 4.1 时必须先确认 9.1 = done 且 §9 收敛方向 ≠ "NOT CONVERGED"。`PushOnConnect` / `BroadcastDiff`（`internal/ws/broadcaster.go` 行 16-17）的 D6 预留从 no-op 转为实装，具体接口改动（增字段 / 新方法）由 §9 `hubInterfaceChanges` 字段指定
- **未来 Spike 类 story 的模板**——本 story 的"Scope Split Note"模式是未来 spike 的参考：AC 先分"可自动化 / 必须人工"，把"必须人工"显式留到独立 story（最好是独立 epic），而不是让它卡主线 epic
- **AC10 可能触发的 `max_connections` 下调**（若 Phase B 10k p99 > 3s）：改 `config/default.toml` 行 31 + `docs/backend-architecture-guide.md` §12 硬编码 10k 的文字 + Story 9.1 报告 §7 末尾写 `### Phase 3 Sticky Routing Planning`。`architecture.md` §D1 已预留 `RedisPubSubBroadcaster`（行 282-292）——真需要 sticky routing 时从这里接入
- **`tools/ws_loadgen` 是否保留**：`tools/` 目录的约定是 one-off 脚本。Spike 完成后工具可以直接删除，也可以留作未来回归（比如 Epic 1-8 大型 feature 合并前临时验证一下 hub 没退化）。**不要**以任何理由把它移到 `internal/` —— 那会错误暗示"这是长期依赖"
