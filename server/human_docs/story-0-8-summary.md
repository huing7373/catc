# Story 0.8: Cron 调度 + 分布式锁 — 实现总结

为多副本部署提供 cron job 去重能力：每个 job 通过 Redis SETNX 抢锁，同一时刻只有一个实例执行，避免盲盒投放、状态衰减等定时任务重复触发。

## 做了什么

### Redis 分布式锁（`pkg/redisx/locker.go`）

- `Locker` 持有 `redis.Cmdable` + 启动期生成的唯一 `instanceID`（UUID）
- `WithLock(ctx, name, ttl, fn)` 核心流程：
  1. `SET lock:cron:{name} {instanceID} NX EX {ttl}` 尝试加锁
  2. 未抢到 → 返回 nil（静默跳过，不是错误）
  3. 抢到 → 执行 fn()
  4. defer Lua CAS 释放：只有 value 匹配 instanceID 才删 key
- Lua 脚本用 `redis.NewScript` 预编译，避免每次传输脚本体

### Cron 调度器（`internal/cron/scheduler.go`）

- 基于 `robfig/cron/v3`，实现 Runnable 接口（`Name/Start/Final`）
- `cron.New(cron.WithChain(cron.Recover(...)))` 确保 job panic 不会崩溃进程
- Start 从 App.Run 继承 context，shutdown 信号可达所有运行中的 job
- Final 调用 `cron.Stop()` 并等待所有 running job 完成
- `addLockedJob` 辅助方法统一包装 WithLock + error 日志，后续新 job 只需调此方法

### heartbeat_tick 示范 job（`internal/cron/heartbeat_tick_job.go`）

- 每 1 分钟执行，`SET cron:last_tick {RFC3339 UTC}`
- Story 0.4 的 `/healthz` 已经读取此 key，无需修改 health_handler
- 使用 `clock.Now()` 而非 `time.Now()`（M9 约定）

### 应用装配（`cmd/cat/initialize.go`）

- `locker := redisx.NewLocker(redisCli.Cmdable())`
- `cronSch := cron.NewScheduler(locker, redisCli.Cmdable(), clk)` — Clock 从 `_ = clk` 升级为实际传参
- Runnable 注册顺序 `mongoCli, redisCli, cronSch, httpSrv`，Final 逆序确保先停 HTTP → 停 cron → 关 DB

## 怎么实现的

**为什么用 Lua CAS 而非 `defer redis.Del`：** 架构指南 D5 的伪代码用 `defer Del`，但这不安全。如果 fn 执行时间超过 TTL，锁已过期被另一个实例获取，此时 Del 会误删别人的锁。Lua 脚本 `GET + DEL` 原子执行，只有 value 匹配时才删除。

**为什么 TTL 55 秒：** cron 默认最小间隔 1 分钟。55s TTL 留 5s 边际：如果持锁实例崩溃，锁在 55s 后自动释放，下一轮 60s 后的调度能正常获取。

**为什么 Start 继承 parent context：** 经 review 修正。初版用 `context.Background()`，shutdown 信号在 `Final()` 被调用前无法到达运行中的 job。改为 `context.WithCancel(ctx)` 后，App.Run 的 cancel 立即传播到 WithLock 和 job body，阻塞在 Redis I/O 的 job 可以及时退出。

**为什么需要 panic recovery：** 经 review 修正。robfig/cron/v3 默认不恢复 panic，需要 `cron.WithChain(cron.Recover(...))` 显式启用。否则 job panic 会穿透 cron worker 终止进程。

## 怎么验证的

```bash
bash scripts/build.sh --test
```

- `go vet` 通过
- `check_time_now.sh` 通过（cron/ 目录无直接 `time.Now()`）
- 全量测试通过（20 个包）

Locker 测试（miniredis，6 个）：
- 获取锁成功 → fn 执行 → 锁释放
- 锁冲突 → fn 不执行 → 返回 nil
- CAS 安全释放：模拟锁被另一实例抢走，CAS 不误删
- TTL 过期后可重新获取
- fn 返回 error → 传播且锁仍释放

Scheduler 测试（miniredis，3 个）：
- Runnable 接口编译期检查
- heartbeat_tick 写入 `cron:last_tick` 验证
- Start/Final 生命周期

## 后续 story 怎么用

- **添加新 cron job**：在 `scheduler.go` 的 `registerJobs()` 中调用 `s.addLockedJob(spec, name, fn)` 即可，WithLock 自动包装
- **Story 2.5（decay engine）**：`state_decay_job.go` 用 `@every 30s` 调度，通过 addLockedJob 注册
- **Story 6.2（blindbox drop）**：`blindbox_drop_job.go` 用 `@every 30m` 调度
- **Story 8.1（cold start detection）**：`cold_start_recall_job.go` 用 `@every 24h` 调度
- **Locker 复用**：虽然 key 前缀是 `lock:cron:`，但 WithLock 本身是通用的，非 cron 场景也可使用
