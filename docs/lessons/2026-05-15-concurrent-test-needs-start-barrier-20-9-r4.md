---
date: 2026-05-15
source_review: /tmp/epic-loop-review-20-9-r4.md (codex review round 4)
story: 20-9-layer-2-集成测试-开箱事务全流程
commit: 4436239
lesson_count: 1
---

# Review Lessons — 2026-05-15 — 并发集成测试必须用 start barrier 同步 goroutine 启动

## 背景

Story 20-9 r4 codex review 指出：`chest_open_service_integration_test.go` 里 AC8 / AC9
两个 100-goroutine 并发 case（同 idempotencyKey 全部成功 cached replay / 不同 key + 1500 步
1 成功 + 99 × 4002）虽然写了 `sync.WaitGroup` + 100 个 goroutine，但 spawn 循环本身的耗时
可能比 goroutine 内业务调用还慢 —— fast runner 上前几个 goroutine 已经跑完业务，后面的
goroutine 还没启动，导致并发退化为基本顺序执行，**根本没触发 race contention**。即使生产
代码有 regression，断言仍能 trivially pass → false-positive coverage。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 同 key 并发测试缺 start barrier (line 704-720) | medium (P2) | testing | fix | `server/internal/service/chest_open_service_integration_test.go` |
| 2 | 不同 key 并发测试缺 start barrier (line 799-820) | medium (P2) | testing | fix | `server/internal/service/chest_open_service_integration_test.go` |

## Lesson 1: 并发测试必须用 release barrier 让所有 goroutine 几乎同时进入业务逻辑

- **Severity**: medium (P2)
- **Category**: testing
- **分诊**: fix
- **位置**: `server/internal/service/chest_open_service_integration_test.go:704-720` / `:799-820`

### 症状（Symptom）

100-goroutine 并发集成测试断言（"100 个同 key 全部成功 cached replay" / "1 succeeded + 99 ×
4002"）在某些 runner 上能 trivially pass —— 即使去掉 ClaimPending 的 ON DUPLICATE KEY UPDATE
串行化 / FOR UPDATE 行锁，断言依然通过。原因：

```go
// 反例（旧实装）
var wg sync.WaitGroup
wg.Add(N)
for i := 0; i < N; i++ {
    i := i
    go func() {
        defer wg.Done()
        out, err := svc.OpenChest(ctx, in) // 直接进业务
    }()
}
wg.Wait()
```

- spawn 循环本身有开销（创建 goroutine + 调度 + 进 i := i 闭包捕获）
- fast runner 上：goroutine #0 已经跑完 OpenChest 并 commit；goroutine #50 还没启动
- 结果：100 个事务**不是同时**进入 ClaimPending / FindByUserIDForUpdate，而是几乎串行
- 真实的 UNIQUE 索引锁等待 / FOR UPDATE 行锁排队从未触发
- 如果生产代码有 race regression（如把 INSERT ON DUPLICATE KEY UPDATE 改成 INSERT IGNORE
  + 漏查 affectedRows），这种测试根本检不出来

### 根因（Root cause）

Goroutine 启动的"并发"是个**乐观假设**：编写者下意识以为 `go func() { ... }()` 在循环里
意味着 N 个 goroutine 同时跑业务。但 Go runtime 的调度是**串行 spawn + 抢占式调度**，循环
本身就是顺序执行 N 次 `runtime.newproc`。如果 goroutine 内的"开始干活前的工作"（这里是
直接调 `svc.OpenChest`）足够轻，spawn 循环的耗时 > goroutine 业务耗时 → 没有真并发。

集成测试场景下更糟：业务调用要去真实 MySQL 拿锁 / 跑 SQL —— 但 helper 配置 `MaxOpenConns=10`
（见 `buildChestOpenServiceIntegration`）→ 即使 spawn 是真并发，也最多 10 个事务同时跑；
spawn 慢的话连这 10 个并发都凑不齐。

正确的"几乎同时"需要 **release barrier**：先把所有 goroutine 都起好、都阻塞在同一个 channel
上，循环结束后 `close(channel)` 一次性唤醒所有 goroutine。这样 spawn 耗时被搬到 barrier 前，
真正进入业务调用的时刻被压缩到 channel close 之后的微秒级窗口内。

### 修复（Fix）

两个并发 case 都加 release barrier：

```go
// 正例（新实装）
start := make(chan struct{})
var wg sync.WaitGroup
wg.Add(N)
for i := 0; i < N; i++ {
    i := i
    go func() {
        defer wg.Done()
        <-start // 阻塞，等所有 goroutine 都 ready
        out, err := svc.OpenChest(ctx, in)
        // record result
    }()
}
close(start) // 一次性释放所有 goroutine
wg.Wait()
```

修改点：
- `server/internal/service/chest_open_service_integration_test.go:704-721`（AC8 同 key 100 并发）
- `server/internal/service/chest_open_service_integration_test.go:799-821`（AC9 不同 key 100 并发）
- 同步更新 story 20-9 文件 Task 8.2 / 9.2 描述（说明 start barrier 是钦定设计，不是可选优化）
- 同步更新 story 20-9 文件 AC8 / AC9 scaffold 代码块（避免后续 Claude 抄旧 scaffold）

顺带改动：无。最小改动只动并发循环，断言部分不变。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写并发测试（goroutine 数 > 2、目的是触发 race contention /
> 锁排队 / 幂等串行化）** 时，**必须** **用 `start := make(chan struct{})` release barrier
> 同步 goroutine 启动 —— 每个 goroutine 业务调用前 `<-start` 阻塞，spawn 循环结束后
> `close(start)` 统一释放**。
>
> **展开**：
> - 适用场景：
>   - 并发幂等测试（同 key N 并发 → 期望 1 走全流程 + N-1 cached replay）
>   - 并发锁测试（不同 key + 资源不足 → 期望 1 succeeded + N-1 错误码 X）
>   - 并发预订/抢购类（N 并发 + stock=K → 期望 K succeeded + N-K 失败）
>   - 任何依赖"几乎同时进入业务"才能触发的 race 行为
> - 实现模板（**直接抄**）：
>   ```go
>   start := make(chan struct{})
>   var wg sync.WaitGroup
>   wg.Add(N)
>   for i := 0; i < N; i++ {
>       i := i // 闭包捕获
>       go func() {
>           defer wg.Done()
>           <-start // 等释放
>           // ... 业务调用
>       }()
>   }
>   close(start) // 统一释放
>   wg.Wait()
>   ```
> - **为什么不能用 sync.WaitGroup 替代 channel barrier**：WaitGroup 是"等所有 goroutine
>   结束"的同步原语，不是"等所有 goroutine 都启动好再统一开跑"的同步原语。后者需要 channel
>   close 的广播语义。
> - **反例 1**：goroutine 内直接调业务（无 `<-start`）—— spawn 循环耗时 > 业务耗时时退化为
>   顺序，race 没触发。
> - **反例 2**：用 `time.Sleep(100ms)` 让 goroutine 等齐 —— 时序依赖不可靠，CI 上 jitter 大
>   时一样会退化；并且就算 sleep 够长，所有 goroutine 也是先后跑完 sleep 再陆续进业务，仍
>   有 spread。
> - **反例 3**：用 `sync.Mutex.Lock` 让 goroutine 等齐 —— 解锁后 goroutine 是**串行**拿锁
>   进业务，反而消除了并发，比不加 barrier 还糟。
> - **代码 review checklist**：看到 `go func()` 在循环里 + 没看到 `<-start` / `<-barrier`
>   / `<-ready` 这种 receive，就要质疑"这个测试真的并发吗"。
> - **runtime 验证手段**：在 goroutine 业务调用前 `t.Log(time.Now())`，跑测试看时间戳是否
>   都在微秒内 —— 如果时间戳分散在毫秒级或更大，说明 barrier 没生效或没加。

## Meta: 本次 review 的宏观教训

并发测试的"正确性"比"看起来在跑并发"难得多。`go func() { ... }()` 在循环里只是制造了
N 个 goroutine 句柄，**不**保证它们同时进入业务逻辑。真正想测 race contention（INSERT ...
ON DUPLICATE KEY UPDATE 的 UNIQUE 串行化 / FOR UPDATE 行锁排队 / unique constraint
violation），必须在 goroutine 启动和业务调用之间**人为插一个同步点**。Story 4.7 的 100
guest UID 并发 + Story 11.9 的 100 不同 user 并发也都应该回查是否有这个问题（虽然这两个
case 不依赖 race contention 检出 regression —— 它们断言的是"全部成功"，而不是"某种锁/失败
分布"，所以即使退化为串行也能 pass，但严格意义上是同一个反模式）。

未来写"100 goroutine + N=100 钦定"类测试时，barrier 应作为**默认模板**而非可选优化。
