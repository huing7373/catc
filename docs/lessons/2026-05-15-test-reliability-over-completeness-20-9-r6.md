---
date: 2026-05-15
source_review: /tmp/epic-loop-review-20-9-r6.md (codex review round 6) — chain 终结
story: 20-9-layer-2-集成测试-开箱事务全流程
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-15 — 集成测试 reliability over completeness：r2-r6 over-correction chain 终结

## 背景

Story 20.9（Layer 2 开箱事务集成测试）从 r1 开始进入一条长达 5 轮的 review fix chain，每轮 codex review 都能找到上一轮"修复"引入的新问题，形成 over-correction chain。本 lesson 沉淀 r2-r6 全程回顾 + r6 的**终结决策**：承认集成测试层无法同时实现"完美 race detection + 完美 bypass detection + 0 flakiness"三角，改用**责任分离 / reliability over completeness** 终结迭代。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | r5 timing assertion (`serialRatio < 0.5`) 引入 CI flaky | P2 | testing | fix | `server/internal/service/chest_open_service_integration_test.go:765-777, 923-935` |
| 2 | r5 real-picker case 概率断言 (`counts[2] >= 1` + `len(seen) >= 2`) ~7e-5 false-failure 月度 ~0.7% 红灯率 | P2 | testing | fix（选项 A：删除整个 real picker case） | `server/internal/service/chest_open_service_integration_test.go:1239-1354` |

## r2-r6 chain 全程回顾

| 轮次 | 引入修复 | 下轮 finding |
|---|---|---|
| **r1** | 错误码断言修正（race 后失败码 4002 而非 3002，对齐 OpenChest tx 步骤顺序 5d-5f） | r2 指出真随机/真时钟在边界断言上的 flaky |
| **r2** | deterministic stub picker + fixed clock → 消除真随机 flaky | r3 指出 stub 把 production picker 从集成测试完全移除 → 算法 + wiring 失去 production-path regression 兜底 |
| **r3** | 双轨方案（DeterministicWiring stub + RealCryptoPicker_SmokeTest 真 picker 小样本） | r4 指出 100 goroutine 并发缺 start barrier → spawn 循环慢于业务调用 → race 完全不触发（false-positive coverage） |
| **r4** | 加 `close(start)` barrier 同步 goroutine 启动 | r5 指出"最终结果断言"（"100 全 success + 同 reward" / "1 success + 99 × 4002"）和"100 次顺序串行"完全等价 → race vs serial 不可区分 |
| **r5** | 加 wall-clock `serialRatio < 0.5` 硬阈值断言 + bypass-resistant 真随机断言（`distinctRarities >= 2` + `counts[2] >= 1`） | **r6（本轮）**：timing 断言依赖 scheduler/Docker latency → loaded CI 上 healthy implementation 也可能 fail；真随机断言 P(false-failure) ≈ 7e-5 → 月度 ~0.7% CI 红灯 |
| **r6（终结）** | 删除 r5 引入的 timing 断言 + 删除整个 RealCryptoPicker_SmokeTest case；保留 r4 start barrier + r2 stub picker；用责任分离替代"集成测试一站式 detection" | **chain 终结** |

## Lesson: 集成测试 reliability over completeness — 完美三角不可达，必须责任分离

- **Severity**: P2 (medium，但属于"测试纪律"类长期教训，影响所有 Layer 2 集成测试设计)
- **Category**: testing
- **分诊**: fix（删 r5 timing 断言 + 删整个 real-picker case）+ 哲学规则沉淀

### 症状（Symptom）

Story 20.9 的集成测试经历 r2-r6 五轮 fix-review 迭代仍未稳定。每轮"修复"都引入下一轮 review 能找到的新问题：

- r2 stub 消除 flaky → r3 失去 production 算法 coverage
- r3 双轨补 production coverage → r4 发现并发本身没真触发
- r4 加 barrier 让并发真触发 → r5 发现结果断言不能区分 race vs serial
- r5 加 timing + bypass 断言 → r6 发现两者在真实 CI 环境带 flaky

**模式识别**：每条"修复"都是为了让集成测试**同时**做到 (a) 验证最终业务正确 (b) 验证目标 race 路径真触发 (c) 验证 picker 算法正确 (d) 0 flakiness — 这是不可达的"完美三角"。

### 根因（Root cause）

集成测试 = "黑盒事务行为验证 + 真实 MySQL 端到端"，本质上是 **timing-based + statistical** 的环境，叠加：

1. **Docker/MySQL latency 不可预测**：CI runner 共享 host / 资源紧张 / GC 暂停 / 容器冷启动 → wall-clock 测量必然带方差，任何硬阈值 ratio assertion 都会在 tail case 上 false-failure。
2. **真随机不可注入 seed**：`crypto/rand` 设计上不接受 seed（安全前提），导致集成层真 picker case 必然是 statistical assertion；P(false-failure) 即使小到 1e-4，CI 跑 100 次/月 → 月度红灯率 ≈ 1%（不可接受）。
3. **"目标行为有没有真触发"≠"业务结果对不对"**：100 次串行循环和 100 goroutine 并发可以产生**完全相同**的最终态。要直接验证"race 真触发"必须 instrument timing / scheduler state，但这两者在集成测试层都不稳定。

要在**一个测试**里同时做到 (a) (b) (c) (d) → 不可达；任何添加的"目标行为 detection 断言"都把测试变 flaky。

**思维漏洞**：fix-review chain 的每一轮 reviewer 都在追求"再补一条断言就完美了"，没人在元层面问"集成测试到底应该承担哪些验证职责、把哪些职责丢给单测 / runtime / production telemetry"。

### 修复（Fix）

**删除 r5 引入的 flaky 断言；保留 r4 之前的所有功能性 / 正确性断言；用注释 + 测试 doc 把责任分离讲清楚**。

**改动 A**：删除 `serialRatio < 0.5` 硬阈值断言（两处：同 key + 不同 key 并发 case）

```diff
- serialRatio := float64(totalElapsed) / float64(sumDuration)
- if serialRatio >= 0.5 {
-     t.Errorf("并发未触发 race contention: ...ratio=%.3f", ...)
- }
- t.Logf("same-key concurrent timing: total=%v sum=%v max=%v ratio=%.3f", ...)
+ // r6 codex 修正：删除 r5 引入的 `serialRatio < 0.5` 硬阈值断言。
+ // 替代方案（责任分离）：
+ //   - 并发本身的 timing 验证 → 由 `go test -race` runtime 兜底 + production observability
+ //   - 集成测试只保证"在 race scenario 下结果业务正确"
+ // 保留 r4 start barrier（功能正确性前提，无 flaky 风险）。
+ t.Logf("same-key concurrent timing (informational, no assertion): total=%v sum=%v max=%v ratio=%.3f", ...)
```

**改动 B**：删除整个 `TestChestOpenServiceIntegration_WeightedPickDistribution_RealCryptoPicker_SmokeTest` case（选项 A 终极简化）

理由：weighted_test.go 已用 `N=10000 + mathrand.NewSource(123) deterministic seed + ±5% 容差` 覆盖 single / multi-distribution / empty / zero-weight 全分支 → 算法层 0 flakiness 完整覆盖。原 SmokeTest 在集成层只剩"production picker 是否被 wire 进 service"这一职责，但这能由 DeterministicWiring case 反向覆盖（stub picker 与 production picker 共用 `WeightedPicker` interface + service 装配代码完全相同；如果 wiring broken，stub case 也会挂在更早阶段；其他全部 case 也都用 `buildChestOpenServiceIntegration` 装配真 picker，任一 case fail 即说明 wiring broken）。

**改动 C**：在测试文件头补一段"测试哲学"注释（reliability over completeness）

```go
// **20.9 r6 chain 终结 — 测试哲学（reliability over completeness）**：
// 集成测试只对"业务结果正确"做硬断言，**不**对"并发本身真触发"或"picker 真随机"
// 做 timing / 概率断言（这两类断言在集成测试层无法同时实现完美检测 + 0 flakiness）。
//   - 并发本身的 timing 验证 → 由 `go test -race` runtime 兜底 + production
//     metrics（idempotency 表 pending rollback 计数、FOR UPDATE 等待时长直方图）
//   - picker 算法的分布正确性 → 由 weighted_test.go 单测覆盖
//   - 集成测试保留 start barrier（功能正确性前提）+ 业务结果断言
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**集成测试中要验证"目标 race / 真随机 / scheduler timing 真发生"**时，**必须**优先用**责任分离**（单测 + `-race` + production telemetry 三层兜底），**禁止**在集成测试断言里硬编 wall-clock 阈值 / 概率下界（任何 P(false-failure) > 1e-9 的统计断言都会在 CI 上月度红灯）。
>
> **展开**：
>
> 1. **集成测试只做"业务结果断言"** —— 终态字段 / 行数 / 错误码 / DB 状态最终一致性。这些都是 binary（等 vs 不等），0 flakiness。
> 2. **race / scheduler / timing 检测责任分离**：
>    - **算法 / data race 检测** → `go test -race` runtime（不需要 assertion）
>    - **production race 监控** → metrics counter（idempotency pending rollback / FOR UPDATE 等待时长 P99 / connection pool 排队）
>    - **集成测试**只保留 start barrier（goroutine 同时起跑是制造 race scenario 的功能前提，本身不带 flaky 风险）+ 终态业务结果断言
> 3. **真随机算法检测责任分离**：
>    - **算法正确性**（分布精度 / mod bias / weight 计算）→ 单测 + deterministic seed（`mathrand.NewSource(N)`）+ 大样本（N=10000）+ 宽容差（±5%）→ P(failure) ≈ 0
>    - **service wiring 正确性**（picker 被真调进 fn / index→domain 字段映射）→ 集成测试的 deterministic stub case（stub 与 production picker 共用 interface + 装配代码相同）反向覆盖
>    - **集成层不直接断言真随机分布** —— `crypto/rand` 设计上不接受 seed，任何小样本断言都带 statistical tail，必 flaky
> 4. **任何 P(false-failure) > 1e-9 的统计 / timing 断言在集成测试里禁止落地** —— CI 跑 100 次/月 → 任何 1e-7 量级 tail 都会变 ~1e-5 月度红灯率，1e-5 量级 tail 会变 1‰ 红灯率，1e-4 量级会变 1%。集成测试 flaky 比覆盖度差更难诊断（reviewer / future Claude 会怀疑 production 而非 test，浪费数小时调查）。
> 5. **fix-review chain 出现 over-correction 模式**（每轮修复都引入下轮的新 finding）→ 立即在元层面提问："我在追求一个不可达的完美三角吗？哪些职责应该丢给其他层？" 而不是继续在同一层加新断言。
> 6. **反例（不要这样写）**：
>    ```go
>    // 反例 1：并发 wall-clock 比较硬阈值
>    if float64(totalElapsed)/float64(sumDuration) >= 0.5 {
>        t.Errorf("并发未触发 race...")  // CI loaded → false-failure
>    }
>
>    // 反例 2：集成层真随机小样本概率断言
>    if counts[2] < 1 { // P(rare == 0) ≈ 7e-5
>        t.Errorf("picker bypass...")  // 月度 ~0.7% 红灯
>    }
>
>    // 反例 3：scheduler / goroutine 起跑顺序硬性 timing 假设
>    time.Sleep(100 * time.Millisecond)  // CI 慢 → barrier 失效；CI 快 → 浪费时间
>    ```
> 7. **正例**：
>    ```go
>    // 正例 1：start barrier（功能前提，无 flaky）
>    start := make(chan struct{})
>    for i := 0; i < N; i++ {
>        go func() { <-start; svc.OpenChest(...) }()
>    }
>    close(start) // 所有 goroutine 同时释放
>
>    // 正例 2：业务结果硬断言（binary、0 flakiness）
>    if succeededCount != 1 || chestNotUnlockedCount != N-1 {
>        t.Errorf("...")
>    }
>
>    // 正例 3：timing 仅 informational log，不断言
>    t.Logf("concurrent timing: total=%v sum=%v ratio=%.3f", total, sum, ratio)
>
>    // 正例 4：picker 算法单测 + deterministic seed
>    func TestPicker_Distribution(t *testing.T) {
>        picker := random.NewCryptoWeightedPicker(mathrand.New(mathrand.NewSource(123)))
>        // N=10000 + ±5% 容差 → P(failure) ≈ 0
>    }
>    ```

## Meta: chain 终结模式 — fix-review iteration 的元规则

**当 review 进入 N ≥ 3 轮、每轮新增 finding 都来自上轮修复时**，停下来在元层面问 4 个问题：

1. **我在追求的目标是单层达成的吗？** — 集成测试一站式做 race + bypass + 业务正确 → 不是；必须责任分离。
2. **每轮修复都是"补一条断言"吗？** — 是 → over-correction chain 信号；考虑"反过来：删除某条断言 + 把验证职责丢给其他层"。
3. **新增断言的 false-failure 概率乘以 CI 频次能接受吗？** — 1e-4 P × 100 runs/月 ≈ 1% 月度红灯 → 不可接受。
4. **目标行为是"binary 业务正确" 还是 "statistical / timing 检测"？** — 后者**不**应该在集成测试层断言。

**chain 终结 = 撤掉过度修复 + 写元规则 lesson**。本 lesson 是 chain 终结的产物：未来 Claude 遇到类似 over-correction chain（不限于测试，extends to 任何"修复 → 新 finding → 修复"循环），先检索本 lesson 的元规则，再决定是继续修还是终结迭代。

**对照**：参见 `docs/lessons/2026-05-15-domain-aware-rowsaffected-and-over-correction-chain-20-7-r3.md` —— 20-7 r3 已经记录过 over-correction chain 模式（不同领域：业务正确性 vs API 美感），本次是测试 reliability 维度的同模式应用。两条 lesson 一起构成"over-correction chain 终结"的双 case 实证。
