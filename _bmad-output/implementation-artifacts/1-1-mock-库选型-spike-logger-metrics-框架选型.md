# Story 1.1: Mock 库选型 Spike + Logger / Metrics 框架选型

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want 在写第一行 service 代码前确定 mock + test + logger + metrics 工具栈,
so that 后续 Epic 写测试 / 写日志 / 写指标时不用临时拼凑，避免每个 epic 自己 println 风格漂移.

## 故事定位（Spike 性质）

这是**决策型 Spike**，不是实装 story。**唯一交付物是一份决策文档** `_bmad-output/implementation-artifacts/decisions/0001-test-stack.md`。本 story 不写任何 `server/` 下的生产代码。后续 Story 1.3 的 logging 中间件、1.5 的测试基础设施、以及全 Epic 的测试 / 日志 / 指标实装，都直接遵从本 story 选定的工具栈，不再二次讨论。

## Acceptance Criteria

**AC1** — 输出 `_bmad-output/implementation-artifacts/decisions/0001-test-stack.md`，包含下列 6 类决策，每项至少含：**候选清单 / 选定项 / 理由（≥ 3 条）/ 被否候选的否决理由**。

1. **DB Mock 方案**：在 `sqlmock` / 真 MySQL via `dockertest` / `sqlmock` + `miniredis` 组合 中选一。
2. **HTTP Handler 测试方案**：在 `net/http/httptest` 标准库 / Gin 自带 test helpers（`router.ServeHTTP`）/ testify mock 中选一（注意 Gin 本身不提供独立 test helpers 库，通常是 `httptest` + `router.ServeHTTP`，决策文档需澄清此事实）。
3. **断言库**：标准库 `testing` / `testify/assert` + `testify/require` 中选一。
4. **CI 跑法**：`go test -race -cover ./...` 全量 / 按目录拆分 中选一；并注明本地 `bash scripts/build.sh --test` 与 CI 命令的对齐方式（Story 1.7 会重做 build.sh，需预留接口）。
5. **Logger 选型**：`log/slog` (Go 1.21+ stdlib) / `go.uber.org/zap` / `github.com/rs/zerolog` 中选一。
6. **Metrics 选型**：`github.com/prometheus/client_golang` / OpenTelemetry Go SDK / Vector 中选一。

**AC2** — Logger 决策文档必须包含**结构化字段约定**，至少覆盖下列字段的字段名、类型、来源：

| 字段 | 类型 | 来源 | 说明 |
|---|---|---|---|
| `request_id` | string | middleware 注入 | Story 1.3 request_id 中间件 |
| `user_id` | string | auth 中间件上下文 | Epic 4 接入后生效，目前先预留字段 |
| `api_path` | string | `c.FullPath()` | Gin 路由 pattern，非 raw URL |
| `latency_ms` | int64 | logging 中间件计时 | 毫秒 |
| `business_result` | string | service 主动写入 | 如 `chest_opened` / `login_created` |
| `error_code` | int | `*AppError.Code` | Story 1.8 AppError 注入 |

来源：`docs/宠物互动App_总体架构设计.md` §13.1 要求至少输出 `request id / user id / api path / latency / business result / error detail`。

**AC3** — Metrics 决策文档必须**预留 NFR16 要求的 7 类指标位**（给出 metric 名称 / 类型 Counter/Gauge/Histogram / 标签维度 / 更新点）：

| 指标 | 类型 | 典型标签 | 来源要求 |
|---|---|---|---|
| 接口 QPS | Counter | `api_path`, `method`, `code` | 总体架构 §13.3 |
| 接口耗时 | Histogram | `api_path`, `method` | 总体架构 §13.3 |
| 错误率 | 由 Counter 派生 | `api_path`, `code` | 总体架构 §13.3 |
| 开箱成功次数 | Counter | `rarity` | 总体架构 §13.3，Epic 20 接入 |
| 合成成功次数 | Counter | `from_rarity`, `to_rarity` | 总体架构 §13.3，Epic 32 接入 |
| 当前在线房间数 | Gauge | — | 总体架构 §13.3，Epic 10 接入 |
| 当前在线连接数 | Gauge | — | 总体架构 §13.3，Epic 10 接入 |

本节点不要求实际注册这些 metrics，只要求**决策文档把位置 / 名称 / 类型锁死**，避免后续 epic 自由发挥。

**AC4** — 决策文档末尾给出**版本锁定清单**（`go.mod require` 格式），包含：所选 logger 库、metrics 库、断言库、mock 库、dockertest（如选用）、miniredis（如选用）。版本号必须选当前最新稳定版（Dev 实装时需在 go.mod 中 pin 住，不用 `latest`）。

**AC5** — 决策在 Epic 1 后续 story 落地时**直接采用，不再二次讨论**。Story 1.3（中间件 logging）按本 spike 选型实装，Story 1.5（测试基础设施）按本 spike 选型安装依赖、建立示例文件。

**AC6** — 本 story **不产出任何 `.go` 代码**、**不修改 `go.mod`**、**不建立 `server/` 目录**。这些由 Story 1.2 / 1.5 / 1.7 分别承担。

## Tasks / Subtasks

- [x] **T1**：建立决策文档骨架（AC1）
  - [x] T1.1 创建目录 `_bmad-output/implementation-artifacts/decisions/`（若不存在）
  - [x] T1.2 新建 `0001-test-stack.md`，按 ADR 模板（`# 0001 Test Stack / Status / Context / Decision / Consequences`）起手
- [x] **T2**：完成 6 类候选评估与决策（AC1）
  - [x] T2.1 DB mock：对比 sqlmock vs dockertest vs sqlmock+miniredis 组合，决策并记录
  - [x] T2.2 HTTP handler 测试：澄清"Gin 自带 test helpers" 实质 = `router.ServeHTTP` + `httptest.NewRecorder()`
  - [x] T2.3 断言库：testify vs stdlib testing
  - [x] T2.4 CI 命令：确认 `go test -race -cover ./...` 能覆盖全量，记录本地 vs CI 对齐方案
  - [x] T2.5 Logger：slog vs zap vs zerolog，考量 stdlib 兼容性 / 性能 / 结构化字段灵活度
  - [x] T2.6 Metrics：prometheus/client_golang vs OTel vs Vector，考量拉 vs 推 / 与 Gin 中间件集成难度
- [x] **T3**：写入结构化字段约定表（AC2）
  - [x] T3.1 复制本 story §AC2 的 6 行表格到决策文档
  - [x] T3.2 对每个字段补一段"本 epic / 后续 epic 何时生效"的说明
- [x] **T4**：写入 NFR16 指标位预留表（AC3）
  - [x] T4.1 复制本 story §AC3 的 7 行表格到决策文档
  - [x] T4.2 对每个指标补"注册点 / 更新点"说明（如 QPS 在 logging 中间件末尾递增 Counter）
- [x] **T5**：输出版本锁定清单（AC4）
  - [x] T5.1 查询每个选定库 2026-04 当前最新稳定版
  - [x] T5.2 以 `go.mod require ( ... )` 格式写入决策文档
- [x] **T6**：自检并提交（AC5 / AC6）
  - [x] T6.1 检查本 story **未**创建任何 `.go` 文件、**未**创建 `server/go.mod`
  - [x] T6.2 提交：`git add _bmad-output/implementation-artifacts/decisions/0001-test-stack.md`（由 user 执行 commit，dev-story 流程不直接 commit）
  - [x] T6.3 commit message 格式：`docs(decision): 0001 test stack - 选定 xxx / xxx / xxx`（建议值：`docs(decision): 0001 test stack - slog / prometheus / sqlmock+miniredis / testify`）

## Dev Notes

### 项目关键约束（必读，勿绕过）

1. **当前 `scripts/build.sh` 仍引用旧架构残留**：`cmd/cat`、`docs/api/openapi.yaml`、`scripts/check_time_now.sh`。**本 story 不需要修它**（Story 1.7 专职），但选 CI 命令时要意识到 build.sh 即将重做，CI 命令应直接面向 `bash scripts/build.sh --test`。
2. **`server/` 目录当前为空**（CLAUDE.md 确认），go.mod / cmd/server 都不存在。本 story 不建立这些东西。
3. **CLAUDE.md 已锁死技术栈** — HTTP 框架 = Gin，ORM = GORM 或 sqlx，主存储 = MySQL 8.0，缓存 = Redis，配置 = YAML。Logger / Metrics / Mock 在 CLAUDE.md 中**未**硬性指定，故由本 spike 决定。
4. **按需引入原则**（MVP 节点 §2 原则 7）：本节点（节点 1）不接 MySQL / Redis / WS / auth 中间件。所以 DB mock 决策**只决定工具栈**，实际第一条 SQL 测试在 Epic 4 才写。**但工具栈必须现在锁死**，否则 Story 1.5 无法示范 AR27 mock 单测。
5. **Spike 性质不写代码**：本 story **禁止**修改 `server/` 或安装任何 Go 依赖。决策文档以外的任何代码改动都是 scope creep，归 Story 1.5 / 1.7。

### 选型评估轴（建议从这几个维度打分）

| 轴 | 备注 |
|---|---|
| **依赖数量** | 用 stdlib 的能用 stdlib，减少后续升级负担 |
| **性能** | logger 选型要衡量 QPS 场景（本应用 QPS 不会很高，但 WS 广播会产生大量事件日志） |
| **结构化字段灵活度** | logger 必须支持 §AC2 的 6 字段且可扩展 |
| **Gin 集成成熟度** | Gin 中间件生态较完整，优先选有现成 Gin adapter 的库 |
| **测试场景契合** | mock 库选型要考虑 GORM / sqlx 产生的 SQL 形态（sqlmock 对 GORM 的正则匹配有坑） |
| **社区活跃度** | 2026-04 仍在活跃维护的版本 |
| **可观测生态** | metrics 若选 prometheus，要考虑后续是否接 OTel 迁移路径 |

### 候选库关键事实（2026-04 当前生态快照，供 Spike 参考，非已选型）

**Logger**
- `log/slog`：Go 1.21（2023-08）进入 stdlib，`slog.Handler` 接口允许自定义 JSON/Text 输出。无外部依赖，长期稳定。性能次于 zap/zerolog 但差距在可接受范围。
- `go.uber.org/zap`：业界性能标杆，反射-free，field-pool。API 分 `Logger`（强类型高性能）和 `SugaredLogger`（易用）。有成熟 Gin adapter（如 `gin-contrib/zap`）。
- `github.com/rs/zerolog`：零分配，JSON-first，API 简洁。适合需要极致性能的场景。非 JSON 输出支持较弱。

**Metrics**
- `github.com/prometheus/client_golang`：事实标准，拉取式（需暴露 `/metrics` 端点），Gin 有现成中间件（`penglongli/gin-metrics` 或手写）。
- OpenTelemetry Go SDK：统一 traces + metrics + logs，推送式或拉取式都支持。API 相对复杂，实现细节仍在演进。
- Vector：偏部署侧 agent（Rust 写的），应用侧通常不直接引入，故实际上不是同一类候选。

**DB Mock**
- `github.com/DATA-DOG/go-sqlmock`：mock `database/sql` driver，用正则匹配 SQL。对 GORM 自动生成的 SQL 正则易碎，通常需要 `ExpectPrepare` + `WithArgs` 组合。无需启动 MySQL。
- `github.com/ory/dockertest/v3`：启动真实 Docker MySQL，集成测试保真度最高，但依赖 Docker 且启动慢（~5-10s）。适合 Epic 4 layer-2 集成测试。
- `github.com/alicebob/miniredis/v2`：纯 Go Redis 模拟，90%+ Redis 命令覆盖。单元测试友好，不支持 Pub/Sub 复杂场景。

**接口 Mock**
- `go.uber.org/mock`（前 `golang/mock`，2023 转移 Uber 维护）：codegen，`mockgen` 工具。
- `github.com/vektra/mockery`：生成 testify/mock 风格，流行度高。
- `github.com/stretchr/testify/mock`：手写 mock，不需 codegen 但样板代码多。

**HTTP Test**
- `net/http/httptest`：stdlib，Gin 通过 `router.ServeHTTP(w, req)` 与其无缝集成。
- 所谓"Gin test helpers" = Gin 文档推荐的 `httptest` + `router.ServeHTTP` 组合模式，**Gin 没有独立的测试库**。决策文档澄清这一点，避免后续实装者踩坑。

**断言**
- stdlib `testing`：需手写 `if got != want { t.Errorf(...) }`。
- `testify/assert`：continue on fail。
- `testify/require`：fail-fast。两者常混用（前置条件用 require，单元 case 用 assert）。

### 决策文档模板建议

```markdown
# ADR-0001: Server 测试 / 日志 / 指标工具栈

- **Status**: Accepted
- **Date**: 2026-04-XX
- **Decider**: <dev name>
- **Supersedes**: N/A

## Context

<项目阶段、当前约束（按需引入原则）、选型驱动因素>

## Decision Summary

| 领域 | 选定 | 版本 |
|---|---|---|
| DB mock | … | … |
| HTTP handler 测试 | … | … |
| 断言 | … | … |
| CI 跑法 | … | … |
| Logger | … | … |
| Metrics | … | … |

## Decisions

### 1. DB Mock
- **选定**: …
- **理由**: (1) … (2) … (3) …
- **否决候选**: …（否决理由）

（对其他 5 项重复同结构）

## Structured Log Fields

<复制本 story §AC2 表格 + 每字段生效时机>

## Metrics Reserved Slots

<复制本 story §AC3 表格 + 每指标注册点>

## Version Lock

```go.mod
require (
    github.com/… vX.Y.Z
    …
)
```

## Consequences

- **Positive**: …
- **Negative / Accepted trade-offs**: …
- **Follow-ups**: Story 1.3 logging 中间件、Story 1.5 测试基础设施、Epic 4 首次 DB 测试按本决策实装
```

### Project Structure Notes

- 决策文档路径：`_bmad-output/implementation-artifacts/decisions/0001-test-stack.md`（BMAD artifacts 目录下，不进 `server/` 源码树）
- 未来同类决策文档编号递增：Story 1.8 的 `0006-error-handling.md`、Story 1.9 的 `0007-context-propagation.md` 已预留，本 story 占用 0001
- **与统一项目结构无冲突**：本 story 只写 markdown，不触及 `docs/宠物互动App_Go项目结构与模块职责设计.md` §4 定义的 `server/` 目录骨架

### References

- [Source: docs/宠物互动App_总体架构设计.md#6.1-技术选型] — Go / 模块化单体 / Gin|Echo|Chi / GORM|sqlx
- [Source: docs/宠物互动App_总体架构设计.md#13-日志与可观测性建议] — Logger 字段要求、Metrics 7 类指标
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#13.1-日志要求] — request id / user id / api path / latency / business result / error detail
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#13.3-监控指标建议] — 7 类指标预留
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#14-测试建议] — 单元测试 / service 测试 / 集成测试分层
- [Source: docs/宠物互动App_MVP节点规划与里程碑.md#4.1-节点1] — 节点 1 范围（含测试基础设施必须在节点 1 完成）
- [Source: docs/宠物互动App_MVP节点规划与里程碑.md#2-当前MVP节点规划原则] — 原则 7（按需引入）/ 原则 8（测试基础设施节点 1 内建立）
- [Source: docs/宠物互动App_V1接口设计.md#2.4-通用响应结构] — ping response 形态（Story 1.2 会用到，本 story 不触及）
- [Source: docs/宠物互动App_V1接口设计.md#3-错误码定义] — 26 个错误码（Story 1.8 AppError 会用到，本 story 不触及）
- [Source: CLAUDE.md#Tech-Stack新方向] — 锁定 Gin + MySQL + Redis + YAML
- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.1] — 本 story 原始 AC（本文档对齐并细化）
- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.3] — 下游依赖（logging 中间件按本选型实装）
- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.5] — 下游依赖（测试基础设施按本选型安装）
- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.8] — 下游依赖（AppError 框架占用 decision 0006）
- [Source: _bmad-output/planning-artifacts/epics.md#Story-1.9] — 下游依赖（ctx 传播占用 decision 0007）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- 用 WebSearch 确认 2026-04-24 当前各库最新稳定版：
  - Gin v1.12.0（2026-02-28）
  - testify v1.11.1（2025-08-27）
  - zap 最新 v1.27.1（2025-11-19，**否决**，记录备忘）
  - sqlmock v1.5.2（上游已声明稳定态）
  - dockertest/v3 v3.12.0（2025-03-12）
  - miniredis/v2 v2.37.0（2026-02-25）
  - client_golang v1.23.2（2025-09-05）
- 发现并处理 AC2 表格注脚的源文件标注轻微偏差：story AC2 注"来源：总体架构设计.md §13.1"，实际该 §13.1（日志最少字段）位于 `docs/宠物互动App_Go项目结构与模块职责设计.md`（总体架构设计.md §13 是"当前 MVP 范围"）。References 区已给出两份文档的正确引用，决策文档 §4 末尾的来源行直接写 Go项目结构与模块职责设计.md，不误导后续 dev。

### Completion Notes List

- ✅ 交付物：`_bmad-output/implementation-artifacts/decisions/0001-test-stack.md`（1 个新文件，~210 行 markdown）
- ✅ 6 类决策（AC1）全部完成，每项均含"候选清单 / 选定项 / ≥3 条理由 / 否决候选的否决理由"
- ✅ 最终选定：
  - DB/缓存 Mock：**sqlmock v1.5.2 + miniredis v2.37.0** 组合（layer-2 集成测试补 dockertest v3.12.0）
  - HTTP Handler 测试：**`net/http/httptest` + `gin.Engine.ServeHTTP`**（明确澄清 Gin 无独立 test helpers 库）
  - 断言：**testify v1.11.1**（assert + require 混用）
  - 接口 Mock：**testify/mock**（手写，零 codegen）
  - CI：**`go test -race -cover ./...` 全量** + `scripts/build.sh --test[--race][--coverage]` 开关（Story 1.7 实现）
  - Logger：**`log/slog`**（stdlib，Go 1.22+）
  - Metrics：**prometheus/client_golang v1.23.2**（拉取式）
- ✅ AC2 日志 6 字段表已写入决策文档 §4，每字段补"生效 Epic/Story"说明
- ✅ AC3 metrics 7 指标位已写入 §5，每项给出 metric 名（`cat_*` 前缀）/ 类型 / 标签 / 注册点文件路径 / 更新点 / 接入 Epic
- ✅ AC4 go.mod require 清单在 §6，版本全部 pin 到 2026-04 当前稳定版
- ✅ AC5 Follow-ups 表映射到 Story 1.2 / 1.3 / 1.5 / 1.7 / 1.8 / 1.9 / Epic 10 / Epic 20 / Epic 32 的落地位置
- ✅ AC6 自检通过：`find . -name '*.go'` 返回空；`go.mod` 不存在；`server/` 目录为空（仅 `.` `..`）
- 本 spike 不写任何 Go 代码，无需 `scripts/build.sh --test` 验证（无可测代码）；自检依赖对文件系统的审查
- Git 未 commit（dev-story 流程末尾由 user 触发 commit；所有变更均留在工作区供 review）

### File List

**新增**：
- `_bmad-output/implementation-artifacts/decisions/0001-test-stack.md`（唯一交付物）

**修改**：
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（story 1-1 状态 `ready-for-dev` → `in-progress` → `review`；`last_updated` → `2026-04-24`）
- `_bmad-output/implementation-artifacts/1-1-mock-库选型-spike-logger-metrics-框架选型.md`（本 story 文件：Tasks 全部勾选 / Dev Agent Record / Change Log / Status=review）

## Change Log

| Date | Change | By |
|---|---|---|
| 2026-04-24 | Story 1.1 实装：交付决策文档 `0001-test-stack.md`；锁定 slog / prometheus client_golang / sqlmock+miniredis / testify 工具栈；日志 6 字段 + 指标 7 位 + 版本清单全部就位。状态 → review。 | Developer (claude-opus-4-7[1m]) |
