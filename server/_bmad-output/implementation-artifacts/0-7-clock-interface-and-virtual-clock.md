# Story 0.7: Clock interface 与虚拟时钟

Status: done

## Story

As a developer,
I want all business code to obtain time via an injectable Clock, with FakeClock for deterministic decay testing,
So that time-sensitive logic (state decay, cold-start recall, token expiry) can be unit-tested without sleeping (M9, FR60).

## Acceptance Criteria

1. **AC1 — Clock interface**：`pkg/clockx/clock.go` 定义 `Clock interface { Now() time.Time }`
2. **AC2 — RealClock**：`RealClock{}` 实现 `Now() time.Time { return time.Now().UTC() }`（D12 时间戳 UTC 权威）
3. **AC3 — FakeClock**：`FakeClock` 含 `Advance(d time.Duration)` 方法；构造支持指定初始时间
4. **AC4 — Lint 规则**：Lint / grep 规则禁止 `internal/{service, cron, push, ws, handler}` 下的业务代码直接调 `time.Now()`（CI 扫描脚本断言）
5. **AC5 — initialize.go 装配**：`initialize.go` 装配一个 `RealClock` 实例并传给所有 Service 构造函数；后续 story 的 Service 构造签名必须包含 `clock clockx.Clock` 参数
6. **AC6 — 示范代码**：`docs/code-examples/service_example.go` 展示 service 持 Clock 依赖（含 FakeClock 单元测试示范）
7. **AC7 — FakeClock 单元测试**：初始时间正确 / `Advance(15 * time.Second)` 后 `Now()` 返回正确值 / 多次 Advance 累加
8. **AC8 — 业务使用验证**：注意 — AC 原文提到 "Story 0.14 的 WS 消息类型版本查询 endpoint 返回的时间戳使用 Clock" 验证真的被使用。当前 Story 0.7 范围内暂无业务 endpoint 可接入 Clock，此验证推迟到 Story 0.14。本 story 通过 initialize.go 装配 + service_example.go 示范 + 单元测试三层证明 Clock 可用。

## Tasks / Subtasks

- [x] Task 1: Clock interface + RealClock + FakeClock (AC: #1, #2, #3)
  - [x] 1.1 删除 `pkg/clockx/doc.go` 占位文件
  - [x] 1.2 创建 `pkg/clockx/clock.go`：定义 `Clock` interface（`Now() time.Time`）+ `RealClock` struct（`Now()` 返回 `time.Now().UTC()`）
  - [x] 1.3 在同文件定义 `FakeClock` struct：字段 `now time.Time`（非导出），构造函数 `NewFakeClock(initial time.Time) *FakeClock`，方法 `Now() time.Time` + `Advance(d time.Duration)`
  - [x] 1.4 `FakeClock.Advance` 实现：`c.now = c.now.Add(d)`（不需要并发安全，测试场景单 goroutine）
  - [x] 1.5 `NewRealClock() *RealClock` 构造函数（供 initialize.go 使用，保持显式构造风格）

- [x] Task 2: FakeClock 单元测试 (AC: #7)
  - [x] 2.1 创建 `pkg/clockx/clock_test.go`
  - [x] 2.2 Table-driven 测试用例：
    - 初始时间 = 构造参数
    - `Advance(15 * time.Second)` 后 Now() = initial + 15s
    - 多次 Advance 累加：`Advance(10s)` + `Advance(5s)` → initial + 15s
    - `Advance(0)` 不变
    - 负值 `Advance(-5s)` 也应正常工作（时间回退）
  - [x] 2.3 RealClock 冒烟测试：`NewRealClock().Now()` 与 `time.Now().UTC()` 差 < 1 秒
  - [x] 2.4 接口兼容性测试：`var _ clockx.Clock = (*RealClock)(nil)` + `var _ clockx.Clock = (*FakeClock)(nil)` 编译期检查

- [x] Task 3: Lint 扫描规则 — 禁止业务代码直接调 time.Now() (AC: #4)
  - [x] 3.1 在 `scripts/check_time_now.sh` 创建 CI 扫描脚本：用 `grep -rn 'time\.Now()' server/internal/{service,cron,push,ws,handler}/` 检测直接调用（排除 `_test.go` 文件）
  - [x] 3.2 脚本若发现匹配则 exit 1 + 输出违规行号
  - [x] 3.3 注意：`internal/middleware/logger.go` 中的 `time.Now()` 是合法的（中间件层不属于业务逻辑，且不可注入 Clock），脚本不扫描 middleware 目录
  - [x] 3.4 考虑将 `forbidigo` 规则扩展为包含 `time.Now`（但 forbidigo 无法按目录筛选，所以用独立脚本更精准）
  - [x] 3.5 在 CI workflow 或 `scripts/build.sh` 中添加对此脚本的调用（可选：如果 build.sh 修改影响面大，先作为独立脚本存在，CI yml 中调用）

- [x] Task 4: initialize.go 装配 RealClock (AC: #5)
  - [x] 4.1 在 `cmd/cat/initialize.go` 中 import `pkg/clockx`
  - [x] 4.2 在 `initialize()` 函数中创建 `clk := clockx.NewRealClock()`
  - [x] 4.3 将 `clk` 赋值到一个变量备用（当前无 Service 消费者，但为后续 story 预留传参位置）
  - [x] 4.4 添加启动日志 `log.Info().Str("component", "clock").Msg("real clock initialized")`（可选，与其他组件初始化日志风格一致）

- [x] Task 5: service_example.go 升级 — Clock 依赖示范 (AC: #6)
  - [x] 5.1 重写 `docs/code-examples/service_example.go`：展示一个持有 `clockx.Clock` 字段的 ExampleService struct
  - [x] 5.2 构造函数 `NewExampleService(clock clockx.Clock, ...) *ExampleService`
  - [x] 5.3 方法中使用 `s.clock.Now()` 而非 `time.Now()`
  - [x] 5.4 在同文件或 `docs/code-examples/service_example_test.go` 添加 FakeClock 单元测试示范：创建 FakeClock → 注入 → Advance → 断言时间相关行为

- [x] Task 6: 清理 + 集成验证
  - [x] 6.1 确认无 `internal/{service,cron,push,ws,handler}/` 下的 `time.Now()` 直接调用（当前 service/doc.go 只是占位，无 time.Now）
  - [x] 6.2 `bash scripts/build.sh --test` 编译 + 所有测试通过（含回归）
  - [x] 6.3 运行 `bash scripts/check_time_now.sh`（如果创建了的话）验证通过

## Dev Notes

### Architecture Constraints (MANDATORY)

- **宪法 §1（显式胜于隐式）**：无 DI 框架；所有依赖在 `initialize()` 手动构造。`RealClock` 在 `initialize.go` 中 `clockx.NewRealClock()` 创建
- **M9 Clock interface `[convention]`**：所有 service / cron 持 Clock 依赖；**禁直接调 `time.Now()`** 在业务代码中
- **D12 时间戳权威源**：服务端 `updatedAt` 权威，`RealClock.Now()` 返回 UTC（`time.Now().UTC()`）
- **M1 包命名**：单数小写短词 → `clockx`（已存在占位目录）
- **M10 测试 helper 命名**：`setupXxx(t)` / `assertXxx(t)`，必须调 `t.Helper()`
- **宪法 §6（Context 贯穿到底）**：Clock 本身不需要 context（`Now()` 无副作用），但使用 Clock 的 service 方法签名仍须 `ctx context.Context` 第一参数

### 关键实现细节

**Clock interface 签名（架构指南 M9 原文）：**
```go
type Clock interface { Now() time.Time }
type RealClock struct{}
func (RealClock) Now() time.Time { return time.Now().UTC() }

type FakeClock struct{ now time.Time }
func NewFakeClock(initial time.Time) *FakeClock { return &FakeClock{now: initial} }
func (c *FakeClock) Now() time.Time { return c.now }
func (c *FakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }
```

**FakeClock 不需要 mutex**：测试场景下 FakeClock 在单 goroutine 中使用（test 函数串行执行 Advance + Now），无并发安全需求。如果后续有并行测试需求再加。

**middleware/logger.go 中的 `time.Now()` 是合法的**：中间件层不属于业务逻辑范畴（M9 约束的是 `internal/{service, cron, push, ws, handler}`），且 Gin 请求延迟计算不适合注入 Clock。

**initialize.go 当前无 Service 消费 Clock**：Story 0.7 只建立 Clock 基础设施。第一个真正消费 Clock 的 Service 将在 Story 0.8（cron scheduler）或更后面的 story 中出现。initialize.go 中创建 `clk` 变量但暂时无下游传递是正常的（Go 编译器会报 unused variable，需要用 `_ = clk` 或赋值给一个后续可用的结构）。

**处理 unused variable 的策略**：
- 方案 A：initialize.go 中 `clk := clockx.NewRealClock()` + `_ = clk`（显式忽略，后续 story 移除 `_`）
- 方案 B：将 clk 存入一个 deps struct 或直接传给 handlers struct（即使暂时不用）
- 方案 C：先不在 initialize.go 中创建 clk，只确保 clockx 包可用 + 示范代码正确，留给第一个消费 story 来添加
- **推荐方案 A**：最简单，符合 "先建基础设施" 的 story 意图

### Source Tree — 要创建/修改的文件

**新建：**
- `pkg/clockx/clock.go` — Clock interface + RealClock + FakeClock + NewFakeClock + NewRealClock
- `pkg/clockx/clock_test.go` — FakeClock 单元测试 + RealClock 冒烟测试 + 接口编译检查
- `scripts/check_time_now.sh` — CI 扫描脚本：禁止业务代码直接调 time.Now()

**修改：**
- `cmd/cat/initialize.go` — 添加 `clockx.NewRealClock()` 装配（当前 `_ = clk`）
- `docs/code-examples/service_example.go` — 重写为含 Clock 依赖的 Service 示范 + FakeClock 测试示范

**删除：**
- `pkg/clockx/doc.go` — 占位文件，有真实文件后删除

### Testing Standards

- 文件命名：`xxx_test.go` 同目录
- 多场景测试必须 table-driven（宪法）
- 单元测试使用 `t.Parallel()`
- testify：`require.NoError` / `assert.Equal`
- 测试 helper：`setupXxx(t)` / `assertXxx(t)`，必须调 `t.Helper()`
- Clock 测试不需要 Testcontainers（纯内存操作）

### Previous Story Intelligence (Story 0.6)

- **AppError + RespondAppError 模式**：`internal/dto/error.go` 已建立，后续 handler 可用 `dto.RespondAppError(c, err)` 统一响应错误
- **init() 启动期校验模式**：`error_codes.go` 的 `init()` 遍历 sentinel 检查 registry 完整性 → Clock 包无需此模式（无 registry 概念）
- **占位文件删除模式**：Story 0.6 删除了 `internal/dto/doc.go`，本 story 同理删除 `pkg/clockx/doc.go`
- **RespondAppError 用 logx.Ctx**：所有 error 响应用 `logx.Ctx(c.Request.Context())` 获取 logger，非 `log.Ctx`
- **Go module path**：`github.com/huing/cat/server`
- **现有依赖**：zerolog v1.35.0, gin, testify 已在 go.mod 中
- **中间件顺序**：Logger → Recover → RequestID（wire.go buildRouter）

### Git Intelligence

最近 5 个 commit 全部属于 Story 0.6（AppError + 错误分类注册表）。关键模式：
- Review round 反馈驱动迭代修正（3 轮 review round）
- Story 文档与实际实现保持对齐（文档即 source of truth）
- 每个 story 最终通过 `bash scripts/build.sh --test` 全量回归

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 0.7 — AC 完整定义, Lines 564-581]
- [Source: _bmad-output/planning-artifacts/architecture.md#M9 — Clock interface 定义, Lines 647-659]
- [Source: _bmad-output/planning-artifacts/architecture.md#D12 — 时间戳权威源, Lines 430-436]
- [Source: _bmad-output/planning-artifacts/architecture.md#目录结构 — pkg/clockx/clock.go, Lines 939-940]
- [Source: _bmad-output/planning-artifacts/architecture.md#Bad/Good 示范 — time.Now 禁止模式, Lines 780-796]
- [Source: docs/backend-architecture-guide.md — 架构宪法全文]
- [Source: _bmad-output/implementation-artifacts/0-6-apperror-and-error-category-registry.md — 前序 story 模式参考]

## Dev Agent Record

### Agent Model Used

Claude Opus 4.6 (1M context)

### Debug Log References

### Completion Notes List

- Clock interface 定义 `Now() time.Time`，RealClock 返回 `time.Now().UTC()`（D12 UTC 权威）
- FakeClock 支持 `NewFakeClock(initial)` + `Advance(d)` 累加，不需要 mutex（测试单 goroutine）
- CI 扫描脚本 `scripts/check_time_now.sh` 检查 `internal/{service,cron,push,ws,handler}/` 下非测试文件的 `time.Now()` 调用
- 脚本已集成到 `scripts/build.sh`（vet 之后、build 之前执行）
- initialize.go 装配 `clk := clockx.NewRealClock()` + `_ = clk`（后续 story 移除 `_` 并传给 Service）
- service_example.go 展示 Service 持 Clock 依赖的标准模式 + FakeClock 单元测试示范
- 全量回归：所有已有测试通过（cmd/cat, config, dto, handler, middleware, jwtx, logx, mongox, redisx, clockx, code-examples）
- clockx 包 7 个测试用例全部通过（5 FakeClock table-driven + 1 RealClock UTC 冒烟 + 1 接口编译检查）

### Change Log

- 2026-04-17: Story 0.7 实现完成 — Clock interface + RealClock + FakeClock + CI 扫描脚本 + initialize.go 装配 + service_example.go 升级

### File List

**新建：**
- pkg/clockx/clock.go
- pkg/clockx/clock_test.go
- scripts/check_time_now.sh
- docs/code-examples/service_example_test.go

**修改：**
- cmd/cat/initialize.go (添加 clockx.NewRealClock() 装配)
- docs/code-examples/service_example.go (重写为含 Clock 依赖的 Service 示范)
- scripts/build.sh (集成 check_time_now.sh 调用)

**删除：**
- pkg/clockx/doc.go (占位文件)
