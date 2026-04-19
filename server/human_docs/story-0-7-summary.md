# Story 0.7: Clock interface 与虚拟时钟 — 实现总结

为所有业务代码提供可注入的时间抽象，使状态衰减、冷启动召回、token 过期等时间敏感逻辑可以在单元测试中确定性验证，无需 `time.Sleep`。

## 做了什么

### Clock 抽象层（`pkg/clockx/clock.go`）

- 定义 `Clock` interface，唯一方法 `Now() time.Time`
- `RealClock` 实现：返回 `time.Now().UTC()`，遵循 D12 "服务端时间戳 UTC 权威"
- `FakeClock` 实现：构造时指定初始时间，`Advance(d)` 可任意推进/回退时间，供测试使用
- 提供 `NewRealClock()` 和 `NewFakeClock(initial)` 构造函数，保持项目"显式构造"风格

### CI 守卫（`scripts/check_time_now.sh`）

- 扫描 `internal/{service,cron,push,ws,handler}/` 下所有非测试 `.go` 文件
- 检测 `time.Now()` 直接调用，发现即 exit 1，阻止构建
- 不扫描 `middleware/`（logger.go 中计算请求耗时合法使用 `time.Now()`）
- 已集成到 `scripts/build.sh`，每次构建自动执行

### 应用装配（`cmd/cat/initialize.go`）

- 在启动流程中创建 `clk := clockx.NewRealClock()`
- 当前 `_ = clk`（暂无 Service 消费者），后续 Story 0.8+ 会移除 `_` 并传给 Service 构造函数

### 示范代码（`docs/code-examples/`）

- `service_example.go` 重写为含 `clockx.Clock` 字段的 `ExampleService`，展示标准 Service 持 Clock 依赖的写法
- `service_example_test.go` 展示 FakeClock 注入 → Advance → 断言的测试模式

## 怎么实现的

整个 `clockx` 包只有 26 行代码，极其简单：

```go
type Clock interface { Now() time.Time }

type RealClock struct{}
func (RealClock) Now() time.Time { return time.Now().UTC() }

type FakeClock struct{ now time.Time }
func (c *FakeClock) Advance(d time.Duration) { c.now = c.now.Add(d) }
```

**为什么不用 mutex：** FakeClock 只在测试中使用，测试函数内 Advance 和 Now 是串行调用的，不存在并发竞争。如果后续出现并行测试需求再加锁。

**为什么用独立脚本而非 forbidigo：** `forbidigo` 是全局 linter 规则，无法按目录筛选。业务代码禁 `time.Now()` 但 middleware 层允许，用 shell 脚本精确控制扫描范围更合适。

**为什么 initialize.go 中 `_ = clk`：** Go 编译器不允许 unused variable。当前没有 Service 消费 Clock，但 story 要求在启动流程中装配 RealClock 证明基础设施就绪。后续 story（如 0.8 cron scheduler）会把 `clk` 传给 Service 构造函数。

## 怎么验证的

```bash
bash scripts/build.sh --test
```

- `go vet` 通过
- `check_time_now.sh` 通过（无业务代码直接调 `time.Now()`）
- `go build` 成功
- 全量测试通过（14 个包，含 clockx 新增的 7 个测试用例）

clockx 测试覆盖：
- 5 个 FakeClock table-driven 子测试（初始时间、前进、累加、零值、负值回退）
- 1 个 RealClock 冒烟测试（返回 UTC、时间在合理范围内）
- 2 个编译期接口兼容性检查（`var _ Clock = (*RealClock)(nil)`）

## 后续 story 怎么用

- **Story 0.8（cron scheduler）**：`cron/scheduler.go` 中的 job 将持有 `clockx.Clock`，用于心跳 tick 等时间相关逻辑。`initialize.go` 中的 `_ = clk` 会改为传入 Scheduler 构造函数
- **Story 2.5（decay engine）**：衰减引擎 cron job 使用 `clock.Now()` 判断状态过期，FakeClock 可在测试中模拟"15 秒无活动"场景
- **Story 0.14（WS 消息类型版本查询）**：endpoint 返回的时间戳使用 Clock，验证 Clock 真的被业务代码使用（AC8 的延迟验证点）
- **所有新 Service**：构造签名必须包含 `clock clockx.Clock` 参数（架构约定 M9）
