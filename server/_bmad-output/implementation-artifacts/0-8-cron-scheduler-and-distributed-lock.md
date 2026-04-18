# Story 0.8: Cron 调度 + 分布式锁

Status: review

## Story

As a platform engineer,
I want all cron jobs protected by a Redis SETNX lock so that multi-replica deployments trigger each job exactly once,
So that blindbox drops / state decay / cold-start checks don't double-fire and corrupt state (NFR-SCALE-2/3, D5, FR56).

## Acceptance Criteria

1. **AC1 — Locker**：`redisx.Locker` 暴露 `WithLock(ctx, name string, ttl time.Duration, fn func() error) error`
2. **AC2 — 锁 key 格式**：`lock:cron:{name}`，默认 TTL 55 秒（留 5s 边际），value 为 instanceID（启动期随机生成 UUID）
3. **AC3 — 加锁**：`SET key value NX EX 55`；未抢到锁直接返回 `nil`（不报错，静默跳过）
4. **AC4 — Lua CAS 释放**：`if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) end` 避免误删其他实例的锁
5. **AC5 — Scheduler**：`cron/scheduler.go` 基于 `robfig/cron/v3`；`RegisterJobs(sch, deps)` 辅助函数自动用 `WithLock` 包装 job
6. **AC6 — Runnable**：Scheduler 实现 Runnable（`Name()="cron_scheduler" / Start / Final`）；Final 调用 `cron.Stop()` 并等待当前任务完成
7. **AC7 — heartbeat_tick 示范**：每 1 分钟记一条 info 日志，更新 `cron:last_tick` Redis key，被 Story 0.4 的 `/healthz` 消费
8. **AC8 — 2 副本验证**：CI 跑 2 副本 docker-compose 对 `heartbeat_tick` 验证：2 个实例同时跑 3 分钟，触发次数 = 3（不是 6）— 注意：此 AC 需要 docker-compose 集成测试环境，当前 CI 暂不支持多副本。本 story 范围内用单元测试模拟锁冲突场景替代，完整 2 副本测试推迟到 Spike 0.15
9. **AC9 — Locker 单元测试**：锁冲突场景 / TTL 过期自动释放 / CAS 释放正确 / 长任务 panic 时锁最终 TTL 自然释放（不阻塞下次调度）
10. **AC10 — 示范代码**：`docs/code-examples/cron_job_example.go` 展示 `withLock` 包裹模式 + ctx 检查（M4）

## Tasks / Subtasks

- [x] Task 1: redisx.Locker — 分布式锁 (AC: #1, #2, #3, #4)
  - [x] 1.1 创建 `pkg/redisx/locker.go`：定义 `Locker` struct，持有 `redis.Cmdable` + `instanceID string`
  - [x] 1.2 构造函数 `NewLocker(cmd redis.Cmdable) *Locker`：内部生成 `uuid.New().String()` 作为 instanceID
  - [x] 1.3 实现 `WithLock(ctx context.Context, name string, ttl time.Duration, fn func() error) error`
  - [x] 1.4 `InstanceID() string` 导出方法（供测试和日志使用）
  - [x] 1.5 `github.com/google/uuid` 已为间接依赖，直接使用即可

- [x] Task 2: Locker 单元测试 (AC: #9)
  - [x] 2.1 创建 `pkg/redisx/locker_test.go`
  - [x] 2.2 使用 miniredis（`github.com/alicebob/miniredis/v2`）— 新增依赖
  - [x] 2.3 测试用例：获取锁成功/锁冲突/CAS 安全释放/TTL 过期/fn error 传播
  - [x] 2.4 key 格式测试：验证 `lock:cron:{name}`

- [x] Task 3: Cron Scheduler — Runnable 实现 (AC: #5, #6)
  - [x] 3.1 删除 `internal/cron/doc.go` 占位文件
  - [x] 3.2 创建 `internal/cron/scheduler.go`
  - [x] 3.3 构造函数 `NewScheduler(locker, redisCmd, clock)`
  - [x] 3.4 实现 Runnable：`Name()` 返回 `"cron_scheduler"`
  - [x] 3.5 `Start`：registerJobs + cron.Start
  - [x] 3.6 `Final`：cron.Stop() + 等待 Done
  - [x] 3.7 `registerJobs()` 私有方法 + `addLockedJob` 辅助
  - [x] 3.8 添加 `github.com/robfig/cron/v3` 依赖

- [x] Task 4: heartbeat_tick 示范 job (AC: #7)
  - [x] 4.1 创建 `internal/cron/heartbeat_tick_job.go`
  - [x] 4.2 实现：SET cron:last_tick + zerolog info
  - [x] 4.3 使用 `clock.Now()`（M9）
  - [x] 4.4 在 registerJobs 中注册 `@every 1m`

- [x] Task 5: Scheduler 单元测试
  - [x] 5.1 创建 `internal/cron/scheduler_test.go`
  - [x] 5.2 编译期 Runnable 接口检查
  - [x] 5.3 heartbeat_tick 设置 Redis key 测试
  - [x] 5.4 Start/Final 生命周期测试

- [x] Task 6: initialize.go 装配 + cron_job_example.go (AC: #5, #10)
  - [x] 6.1 修改 initialize.go：Locker + Scheduler 创建，clk 传参，cronSch 加入 App Runnable
  - [x] 6.2 创建 `docs/code-examples/cron_job_example.go`

- [x] Task 7: 清理 + 集成验证
  - [x] 7.1 `bash scripts/build.sh --test` 全量通过
  - [x] 7.2 `check_time_now.sh` 通过（cron/ 无直接 time.Now）
  - [x] 7.3 healthz 集成由已有 health_handler.go 代码保证，heartbeat_tick 写 cron:last_tick

## Dev Notes

### Architecture Constraints (MANDATORY)

- **宪法 §1（显式胜于隐式）**：无 DI 框架；`Locker` 和 `Scheduler` 在 `initialize.go` 手动构造
- **D5 Cron 分布式锁**：`robfig/cron/v3` + Redis SETNX 锁 + 55s TTL；所有 job 包裹 `withLock`
- **M4 ctx.Done 检查**：长循环/阻塞调用必须检查 `ctx.Done()`；cron job 内部如有循环须遵循
- **M5 goroutine 生命周期**：Scheduler 启动的 goroutine 必须由 Runnable 管理，Final 时等待完成
- **M9 Clock interface**：heartbeat_tick 使用 `clock.Now()` 而非 `time.Now()`（已有 CI 扫描脚本守卫）
- **Graceful Shutdown 顺序**：App.Run() 按 Runnable 注册逆序调用 Final()。注册顺序应为 `mongoCli, redisCli, cronSch, httpSrv`，这样 Final 顺序为：httpSrv → cronSch → redisCli → mongoCli（先停 HTTP，再停 cron，最后关 DB 连接）

### 关键实现细节

**D5 架构指南 Locker 伪代码（需要改进）：**
```go
// 架构指南原文 — 简化版，缺少 CAS 释放
func withLock(r *redis.Client, name string, fn func()) cron.Job {
    return cron.FuncJob(func() {
        ok, _ := r.SetNX(ctx, "lock:cron:"+name, instanceID, 55*time.Second).Result()
        if !ok { return }
        defer r.Del(ctx, "lock:cron:"+name)  // ← 不安全！可能误删其他实例的锁
        fn()
    })
}
```
实际实现必须用 Lua CAS 替代 `defer r.Del`：
```go
const unlockScript = `if redis.call('GET', KEYS[1]) == ARGV[1] then return redis.call('DEL', KEYS[1]) end return 0`
```
这确保只有持锁实例能释放自己的锁。如果 fn 耗时超过 TTL，锁已过期被其他实例获取，CAS 检查发现 value 不匹配，不会误删。

**robfig/cron/v3 关键 API：**
```go
c := cron.New(cron.WithSeconds()) // 支持秒级精度（可选）
c.AddFunc("@every 1m", func() { ... })
c.Start()
ctx := c.Stop() // 返回 context，Done() 时表示所有 running job 完成
<-ctx.Done()    // 等待当前任务完成
```

**Scheduler.Final 的 Stop 等待**：`cron.Stop()` 返回一个 context，其 `Done()` channel 在所有正在运行的 job 完成后关闭。Final 必须 `<-ctx.Done()` 等待，而非 fire-and-forget。

**instanceID 生成时机**：在 `NewLocker()` 构造时生成一次，整个实例生命周期内不变。多副本场景下每个实例有唯一 instanceID，这是 CAS 释放的正确性基础。

**heartbeat_tick 与 /healthz 集成**：Story 0.4 的 `health_handler.go:84-88` 已经读取 `cron:last_tick` Redis key。heartbeat_tick job 写入该 key 后，/healthz 自动展示。无需修改 health_handler。

**redisx.Client 暴露 `Cmdable()` 方法**：返回 `redis.Cmdable` 接口，Locker 接受 `redis.Cmdable`（而非具体 `*redis.Client`），便于测试注入 miniredis。

### Source Tree — 要创建/修改的文件

**新建：**
- `pkg/redisx/locker.go` — Locker struct + WithLock + Lua CAS 释放
- `pkg/redisx/locker_test.go` — miniredis 测试（锁冲突/CAS/TTL/panic）
- `internal/cron/scheduler.go` — Scheduler Runnable + registerJobs
- `internal/cron/heartbeat_tick_job.go` — 每 1 分钟更新 cron:last_tick
- `internal/cron/scheduler_test.go` — Scheduler 生命周期测试
- `docs/code-examples/cron_job_example.go` — 标准 cron job 示范

**修改：**
- `cmd/cat/initialize.go` — 创建 Locker + Scheduler，移除 `_ = clk`，将 cronSch 加入 App Runnable 列表
- `go.mod` / `go.sum` — 添加 `robfig/cron/v3`、`google/uuid`（如不存在）、`alicebob/miniredis/v2`（如不存在）

**删除：**
- `internal/cron/doc.go` — 占位文件

### Testing Standards

- 文件命名：`xxx_test.go` 同目录
- 多场景测试必须 table-driven（宪法）
- 单元测试使用 `t.Parallel()`
- testify：`require.NoError` / `assert.Equal`
- Redis 测试使用 miniredis（不需要 Testcontainers）
- Locker 测试重点：锁竞争、CAS 安全性、TTL 过期
- Scheduler 测试重点：Runnable 生命周期、job 注册执行

### Previous Story Intelligence (Story 0.7)

- **clockx.Clock 已就绪**：`pkg/clockx/clock.go` 定义 Clock interface + RealClock + FakeClock
- **initialize.go 中 `_ = clk`**：本 story 将移除 `_` 并传给 Scheduler 构造函数
- **CI 扫描脚本已就绪**：`scripts/check_time_now.sh` 扫描 `internal/{service,cron,...}/` 下的 `time.Now()` — cron 目录也在扫描范围内，必须用 `clock.Now()`
- **Go module path**：`github.com/huing/cat/server`
- **现有依赖**：zerolog v1.35.0, gin, testify, go-redis/v9 已在 go.mod 中
- **Runnable 接口**：`app.go` 定义 `Runnable { Name() string; Start(ctx) error; Final(ctx) error }`
- **App.NewApp(runs ...Runnable)**：Runnable 注册顺序决定 Final 逆序
- **health_handler 已读取 cron:last_tick**：`health_handler.go:84-88` `redis.Get(ctx, "cron:last_tick")`，无需修改

### Git Intelligence

最近 commit 属于 Story 0.7（Clock interface）。关键模式：
- 占位文件删除 → 真实文件替代（doc.go 模式）
- `initialize.go` 装配新组件的标准流程
- `build.sh --test` 全量回归每个 story 必须通过

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 0.8 — AC 完整定义, Lines 583-602]
- [Source: _bmad-output/planning-artifacts/architecture.md#D5 — Cron 分布式锁, Lines 334-349]
- [Source: _bmad-output/planning-artifacts/architecture.md#M4 — ctx.Done 检查, Lines 609-619]
- [Source: _bmad-output/planning-artifacts/architecture.md#M5 — goroutine 生命周期, Lines 621-624]
- [Source: _bmad-output/planning-artifacts/architecture.md#Graceful Shutdown 顺序, Lines 698-707]
- [Source: _bmad-output/planning-artifacts/architecture.md#目录结构 — internal/cron/, Lines 897-904]
- [Source: _bmad-output/planning-artifacts/architecture.md#目录结构 — pkg/redisx/locker.go, Lines 927]
- [Source: internal/handler/health_handler.go#Lines 84-88 — cron:last_tick 读取]
- [Source: cmd/cat/app.go#Lines 13-17 — Runnable 接口定义]
- [Source: pkg/redisx/client.go#Lines 47-49 — Cmdable() 方法]
- [Source: _bmad-output/implementation-artifacts/0-7-clock-interface-and-virtual-clock.md — 前序 story 模式参考]

## Dev Agent Record

### Agent Model Used

Claude Opus 4.6 (1M context)

### Debug Log References

### Completion Notes List

- Locker 使用 `redis.NewScript` 预编译 Lua CAS 脚本，`SetNX` 加锁 + Lua CAS 释放确保安全
- instanceID 在 `NewLocker` 时 `uuid.New()` 一次性生成，整个实例生命周期不变
- 未抢到锁返回 nil（静默跳过），fn error 通过 WithLock 传播，Redis error 也传播
- Scheduler 实现 Runnable：Start 注册 jobs + cron.Start，Final 调 cron.Stop() + 等待 Done channel
- `addLockedJob` 辅助方法统一包装 WithLock + error 日志，后续 job 只需调此方法
- heartbeat_tick 用 `clock.Now().Format(time.RFC3339)` 写入 `cron:last_tick`，已有 health_handler 消费
- initialize.go 注册顺序 `mongoCli, redisCli, cronSch, httpSrv`，Final 逆序：httpSrv → cronSch → redisCli → mongoCli
- 新增依赖：robfig/cron/v3 v3.0.1, alicebob/miniredis/v2 v2.37.0
- 全量回归通过（20 个包，含 cron 3 个测试 + redisx locker 6 个测试）

### Change Log

- 2026-04-18: Story 0.8 实现完成 — Redis 分布式锁 + Cron Scheduler Runnable + heartbeat_tick + initialize.go 装配

### File List

**新建：**
- pkg/redisx/locker.go
- pkg/redisx/locker_test.go
- internal/cron/scheduler.go
- internal/cron/heartbeat_tick_job.go
- internal/cron/scheduler_test.go
- docs/code-examples/cron_job_example.go

**修改：**
- cmd/cat/initialize.go (Locker + Scheduler 创建，clk 传参，cronSch 加入 App)
- go.mod / go.sum (robfig/cron/v3, alicebob/miniredis/v2)

**删除：**
- internal/cron/doc.go (占位文件)
