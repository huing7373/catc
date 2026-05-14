---
date: 2026-05-15
source_review: /tmp/epic-loop-review-20-9-r5.md (codex review round 5)
story: 20-9-layer-2-集成测试-开箱事务全流程
commit: 5238763
lesson_count: 3
---

# Review Lessons — 2026-05-15 — 并发测试 + bypass-resistant 真随机断言：结果断言必须真区分目标行为

## 背景

Story 20.9（Layer 2 开箱事务集成测试）r5 轮 codex review 指出 3 个 P2 false-positive 断言问题：

- AC8（同 key 并发 cached replay）和 AC9（不同 key 并发 FOR UPDATE）的最终结果断言（`1 success + 99 cached` / `1 success + 99 × 4002`）和 100 次顺序串行执行**完全等价** —— 即使 race 路径根本没真触发（goroutine 串行启动 / 连接池阻塞 / spawn 循环退化），断言仍能 pass。
- AC14b（real picker smoke test）的 `total==100 + rarity ∈ {1..4} + common >= 50` 断言可被"picker 退化为返第一个 enabled item"绕过 —— seed 表前 8 个 enabled item 都是 common，所以一直返第一个 common 也满足全部断言。

3 条都是"结果断言只能 verify 最终态，不能 verify 目标行为有没有真发生"的同类陷阱。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Same-key concurrent test cannot distinguish race vs serial | P2 | testing | fix | `server/internal/service/chest_open_service_integration_test.go:686-770` |
| 2 | Different-key concurrent test cannot distinguish race vs serial | P2 | testing | fix | `server/internal/service/chest_open_service_integration_test.go:788-895` |
| 3 | Real-picker smoke test allows "return first enabled item" bypass | P2 | testing | fix | `server/internal/service/chest_open_service_integration_test.go:1178-1290` |

## Lesson 1: 并发测试必须用 timing 断言区分 race vs serial

- **Severity**: P2 (medium)
- **Category**: testing
- **分诊**: fix
- **位置**: `server/internal/service/chest_open_service_integration_test.go:686-770`（同 key 路径）+ `:788-895`（不同 key 路径）

### 症状（Symptom）

100-goroutine 并发开箱测试的最终态断言（"全部 success + 同 reward" 或 "1 success + 99 × 4002"）和 100 次**顺序循环调用**的最终态**完全等价**。即使并发 goroutine 实际上被某种原因串行执行（spawn 循环慢于业务调用 / 连接池阻塞 / chunk 化排队），断言仍 pass —— 测试根本无法证明目标 race 路径（同 key cached replay / FOR UPDATE 行锁排队）真触发过。这是典型 false-positive coverage。

### 根因（Root cause）

写并发测试时只盯着"最终业务结果"做断言，忘了**并发不只是看结果，更是看"有没有真并发"**。从 single-goroutine 视角看，"100 次同 key 顺序调"和"100 个 goroutine 同 key 并发调"得到的应用层最终态本来就该一样（idempotency 语义要求）——这正是 idempotency 设计的好处，但也正是它让结果断言失去区分力。

要 verify race 真触发，必须 verify**只有在真并发下才能产生的副作用**——最直接的就是 wall-clock：

- serial 执行: `totalElapsed ≈ sumDuration`（每个 call 串行排队）→ ratio ≈ 1.0
- 真并发执行: `totalElapsed ≈ sumDuration / parallelism_cap`（受 MaxOpenConns 限制）→ ratio ≈ 1/N_conn

把 ratio 写进断言，serial 必 fail，race 必 pass。

### 修复（Fix）

每个 goroutine 自报业务调用 wall-clock（`t0 := time.Now()` / `times[i] = time.Since(t0)`），主测试体记录 `totalElapsed`，断言 `totalElapsed / sumDuration < 0.5`（MaxOpenConns=10 下 race ratio 应远小于 0.5，留 5x 安全裕量）：

```go
times := make([]time.Duration, N)
// 每个 goroutine 内: t0 := time.Now(); call(); times[i] = time.Since(t0)
beforeRelease := time.Now()
close(start)
wg.Wait()
totalElapsed := time.Since(beforeRelease)

var sumDuration, maxDuration time.Duration
for _, d := range times {
    sumDuration += d
    if d > maxDuration { maxDuration = d }
}
serialRatio := float64(totalElapsed) / float64(sumDuration)
if serialRatio >= 0.5 {
    t.Errorf("并发未触发 race contention: totalElapsed=%v, sumDuration=%v, ratio=%.3f", totalElapsed, sumDuration, serialRatio)
}
```

阈值 0.5 来自 `MaxOpenConns=10` → 真并发下 ratio 约 0.1（10x speedup），留 5x 安全裕量。同方案应用到 AC8 + AC9 两个 case。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写并发测试时，**必须**断言"只有真并发才能产生的可观测信号"（最优先是 wall-clock 比 ratio），**禁止**只用应用层最终态断言。
>
> **展开**：
> - 并发测试的天然陷阱：idempotency / 串行化设计本来就让"并发"和"顺序"的最终业务态相同 → 结果断言对"race 没真触发"完全盲。
> - 必加的 timing 断言模板：
>   ```go
>   times := make([]time.Duration, N)
>   // goroutine 内自报 t0..t1
>   beforeRelease := time.Now(); close(start); wg.Wait()
>   totalElapsed := time.Since(beforeRelease)
>   ratio := float64(totalElapsed) / float64(sum(times))
>   if ratio >= 0.5 { t.Errorf("serial execution detected") }
>   ```
> - 阈值校准方法：先估算 `parallelism_cap = min(N_goroutine, MaxOpenConns, GOMAXPROCS)`，race 下 ratio 期望 ≈ 1/parallelism_cap，阈值取 0.5 / parallelism_cap 之间的中点（留 5-10x 安全裕量给 CI 噪声）。
> - 替代信号（同样接受）：用 `sync.Mutex + atomic` 计数 inflight 峰值，断言 `maxInFlight >= 2`（race 真发生的 minimum 必要条件）。
> - **反例**：
>   ```go
>   // 100-goroutine 并发开箱，只断言最终结果
>   wg.Wait()
>   var succeeded, failed int
>   for _, r := range results { ... }
>   if succeeded != 1 { t.Errorf(...) }
>   if failed != 99 { t.Errorf(...) }
>   // 没有任何"真并发"的可观测断言 → 即使 goroutine 串行启动也 pass
>   ```

## Lesson 2: 真随机测试必须 bypass-resistant，不能被"返固定 index"退化绕过

- **Severity**: P2 (medium)
- **Category**: testing
- **分诊**: fix
- **位置**: `server/internal/service/chest_open_service_integration_test.go:1178-1290`

### 症状（Symptom）

real-picker smoke test（100 次跑真 `random.NewCryptoWeightedPicker`）的断言是 `total==100 + rarity ∈ {1..4} + common >= 50`。本意是验证 production picker 真被 wire 进 service。但 seed 表前 8 个 enabled item 都是 common rarity → 如果 picker 发生 regression 退化为"总返第一个 enabled item"（最常见的 picker bug 形态），100 次都返 common rarity 1，total==100、rarity ∈ {1..4}、common >= 50 三条都满足 → 测试 pass，bypass detection 失败。

### 根因（Root cause）

写真随机测试时，断言只取了"任何 rarity 都满足"或"common 下界"——但 seed 数据的物理顺序让"common 下界"和"返第一个 item"在 enabled 列表 ORDER 默认下完全一致 → 断言**和最最简单的退化场景同解**。

要 bypass-resistant，断言必须聚焦"真随机产生但 picker 退化不产生"的信号：

- **多样性**：真随机 100 样本几乎必抽到 ≥ 2 种 rarity（P(only 1 rarity) ≈ 3e-5，比 σ=5 还要 tail）；返固定 index → 只产 1 种 rarity → 必挂。
- **小概率桶必出现**：rare bucket p=80/889≈0.09，100 样本里 P(rare==0) = 0.91^100 ≈ 7e-5；返固定 common index → rare 永远 0 → 必挂。

两条断言独立，false-positive 累积 ~1e-4，但 bypass detection 100% 保证。

### 修复（Fix）

real-picker case 在原 `common >= 50` 后追加两条断言：

```go
distinctRarities := len(counts)
if distinctRarities < 2 {
    t.Errorf("real-picker only produced %d distinct rarity bucket(s) in N=%d (seen=%v); P(only 1 rarity) ≈ 3e-5 → picker bypass detected",
        distinctRarities, N, counts)
}
rareCount := counts[2]
if rareCount < 1 {
    t.Errorf("real-picker produced 0 rare items in N=%d; P(rare == 0 | p_rare=80/889) ≈ 7e-5 → picker bypass detected",
        N)
}
```

P(false positive on production picker) ≈ 1e-4（CI 跑 10000 次约挂 1 次），但"返第一个 enabled item"必挂、"weight 算法漏算 rare bucket"必挂、"weight 算法把 common 当 weight=0 当成抽不到"必挂。bypass detection 价值 >>> 1e-4 偶发挂掉的维护成本。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写真随机测试时，**必须**至少加一条"小概率桶必出现"或"多桶多样性"断言，**禁止**仅用主桶下界（如 `common >= 50`）—— 主桶下界和"返第一个 enabled item"在 seed ORDER 下同解，无 bypass 防护价值。
>
> **展开**：
> - 真随机断言的 anti-pattern：只断言"占比最大的桶有最小份额"（如 `common >= 50` / `winner >= 60%`），因为这条断言对"返固定 highest-prob index"的退化场景完全失明。
> - **必须**补的两类 bypass-resistant 断言：
>   1. **多样性**：`len(distinctBuckets) >= K`，K 由 N + 各桶概率 + 期望 P(false-positive) ≤ 1e-4 反推。一般 N=100、桶数 ≥ 3 时 K=2 足够；N=1000 时 K=3 都能保证 1e-4 量级。
>   2. **小概率桶必出现**：选 p ≈ 0.05~0.10 的桶，断言 count ≥ 1。100 样本下 P(false-positive) ≈ (1-p)^100，p=0.05 → 0.6%（偏高），p=0.09 → 7e-5（OK），p=0.10 → 3e-5（OK）。如目标桶 p < 0.05，要么扩大 N，要么改用多样性断言。
> - bypass scenario inventory（必须能 detect 的退化形态）：
>   - "返第一个 enabled item"
>   - "返固定 index k"（任何 k）
>   - "总返最大 weight item"
>   - "weight 算法漏算某桶 → 该桶概率塌缩"
>   - "weight 算法计算成等概率 → 各桶比例失真"
> - **反例**：
>   ```go
>   // 真 picker + 100 次 + 只断主桶下界
>   if counts[1] < 50 { t.Errorf("common too low") }
>   // 缺多样性 + 小概率桶断言 → "返第一个 common item" 测试仍 pass
>   ```

## Lesson 3: false-positive 测试的根因 — 断言聚焦"目标行为产生的独有信号"而非"业务最终态"

- **Severity**: P2 (meta lesson)
- **Category**: testing
- **分诊**: fix（meta，融合 #1+#2 共性）
- **位置**: 跨 case

### 症状（Symptom）

3 个 r5 finding 看似无关（并发 + 真随机两类完全不同），但本质都是**同一个**思维漏洞：写测试时只断言"业务最终态"，没断言"目标行为有没有真触发"。

- 并发 case 的"目标行为"是 race contention（同 key 串行化 / FOR UPDATE 排队），但断言只看"业务最终态"（1 success + 99 cached / 4002）→ 串行执行同解。
- 真随机 case 的"目标行为"是 weighted draw 算法被调用，但断言只看"业务最终态"（reward 落库、rarity 范围合法）→ "返固定 index"同解。

### 根因（Root cause）

写测试时优先级颠倒：先写"业务对不对"（最终态），后忘了写"测试覆盖的目标路径有没有真执行"（行为信号）。两者**不是同一回事**——好的 service 设计（idempotency / 错误码合并）会让多条路径产生同一种最终态，恰恰让"业务最终态断言"对路径区分完全盲。

### 修复（Fix）

写并发 / 真随机 / 任何"路径覆盖"目标的集成测试时，**先列 inventory**：

1. **目标行为是什么**（race contention / weighted draw / FOR UPDATE 锁等待 / cached replay 短路）
2. **该行为产生什么"独有"信号**（wall-clock ratio / 多桶分布 / 锁等待时间 / inflight 峰值）
3. **退化场景列表**（如果路径根本没走会得到什么最终态？如果走的是错误路径会得到什么？）
4. **断言公式 = 业务最终态 + 至少一条独有信号 + 至少一条退化场景必挂的反例断言**

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写任何"路径覆盖"类测试（并发 / 真随机 / fault injection / 锁路径 / cache miss），**必须**列出"目标行为产生的独有可观测信号"并加为断言，**禁止**只断言业务最终态。
>
> **展开**：
> - 测试设计公式：
>   ```
>   断言 = 业务最终态断言（必需，保留）
>        + 目标行为独有信号断言（新增，本 lesson 重点）
>        + 退化场景反例断言（新增，proof by contradiction）
>   ```
> - 路径类别 vs 独有信号对应表：
>   | 路径 | 独有信号 |
>   |---|---|
>   | 并发 race | wall-clock ratio < 1/N_cap / inflight 峰值 ≥ 2 |
>   | 真随机 | 多桶分布 + 小概率桶 count ≥ 1 |
>   | FOR UPDATE 行锁 | lock wait time ≥ threshold / 失败事务的 errCode 反映锁排队顺序 |
>   | cached replay 短路 | DB 读次数（用 fault repo 计数）/ response payload byte-for-byte 一致 |
>   | fault injection | error code + ROLLBACK 后副表 count 都 == 0 |
>   | 缓存命中 | DB 调用次数（用 repo wrapper 计 N） |
> - **反例**（典型 false-positive 模式）：
>   ```go
>   // 测试名声称覆盖 race，但只断结果态
>   func TestRace(t *testing.T) {
>       runConcurrent(N)
>       if successCount != expected { t.Errorf(...) }
>       // 没有任何"真并发"独有信号 → serial 执行也 pass
>   }
>   ```

---

## Meta: 本次 review 的宏观教训

r5 codex review 三条 finding 在"语义层"看是 3 个独立问题（并发 same-key / 并发 diff-key / 真随机），但在"思维漏洞层"看是**同 1 个**：测试设计者只想"业务对不对"，忘了"路径走没走对"。

这条 meta 教训在 epic-20 节点 7 是反复出现的：r1 修错误码顺序（路径走对 → 测试错码）、r2 修 random flakiness（路径走对 → 测试不稳）、r3 修 stub 路径完全覆盖 production picker 这件事（路径根本没走 → 测试假绿）、r4 修 spawn 循环慢导致 goroutine 串行（路径没真并发 → 断言假绿）、r5 修同上 + 真随机 bypass detection（断言不能反推路径有没走）。

整个链条都是同一个问题的不同投影：**集成测试 = 验证路径，单测 = 验证逻辑；路径验证必须证明"路径走过"而不是只证明"业务结果对"。**

后续未来 Claude 写集成测试时直接用本 lesson 的"目标行为独有信号断言"作为模板，能一次避开 r1-r5 全部 5 轮 over-correction。
