---
date: 2026-05-15
source_review: codex review r3 of Story 20.9 (file: /tmp/epic-loop-review-20-9-r3.md)
story: 20-9-layer-2-集成测试-开箱事务全流程
commit: db34c06
lesson_count: 1
---

# Review Lessons — 2026-05-15 — 集成测试 vs 单元测试责任划分（weighted picker 算法 vs wiring）20-9 r3

## 背景

Story 20.9（开箱事务 Layer 2 集成测试）r3 轮 codex review 指出：r2 修复（用 deterministic stub 替换真 `random.NewCryptoWeightedPicker`）虽然消除了集成测试的 random flakiness，但同时把 production weighted picker 路径从集成测试完全移除 —— 若 production picker 发生 regression（miscompute totals / 选错 bucket），1000-opens distribution case 仍会绿。这是 r1-r3 over-correction chain 的典型形态：r1 修了一个 bug，r2 改造太狠摔到对面坑，r3 再校准回平衡点。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | r2 stub 把真 weighted picker 路径从集成测试移除 | medium (P2) | testing | fix (dual-track) | `server/internal/service/chest_open_service_integration_test.go:1016-1120` |

## Lesson 1: 集成测试 vs 单元测试责任划分（weighted 算法的 distribution 验证 vs wiring 验证）

- **Severity**: medium (P2)
- **Category**: testing
- **分诊**: fix（采用 dual-track 双轨方案）
- **位置**: `server/internal/service/chest_open_service_integration_test.go:1016-1120`

### 症状（Symptom）

r2 修复用 `raritySequencePicker` deterministic stub 完全替换 production `random.NewCryptoWeightedPicker`，导致 `WeightedPickDistribution_1000Opens` 集成测试不再执行真正的 weighted picker 算法。codex r3 指出：

> "a regression in the production weighting code (for example, miscomputing totals or choosing the wrong bucket) would still leave this new integration scenario green, even though Story 20.9 is supposed to validate the end-to-end weighted draw behaviour."

### 根因（Root cause）

**集成测试 vs 单元测试的责任划分不清** —— r2 修 flaky 时把"分布算法验证"和"service wiring 验证"混在同一个 case 里思考，于是用 stub 既解决 flakiness 又"反正算法在 weighted_test.go 已覆盖"地默认 production picker 路径不需要在集成层兜底。但 stub 替换后，service.NewChestService 是否真的把 production picker wire 进 fn → 这条"装配 wiring"路径在集成层完全缺失。

更深层次：

1. **"算法分布验证"** 应该在 unit test 层（`weighted_test.go`）—— 用大样本 + 确定性 seed（mathrand.NewSource）+ 紧容差精确验证 distribution 算法（"miscompute totals / wrong bucket" 必挂）；不依赖 docker / MySQL；快速 + 0 flakiness
2. **"集成测试"** 应该聚焦"装配 + 端到端事务行为" —— 验证 service 真的调到 production picker（不是 nil / 不是 mock）、picker index → reward_rarity 映射正确、1000 个事务串行 commit 不丢
3. r2 的错误是用 stub 同时承担"算法"和"wiring"两份责任 → 实际只覆盖了 wiring + 反向把"production picker 真被调用"这层兜底完全丢失

**反例（r2 的形态）**：weighted_test.go 已经用 `mathrand.NewSource(123)` + 10000 样本 + ±5% 容差覆盖了算法分布；集成测试用 stub 也覆盖 wiring + reward 映射；但**没有任何一层**验证"service 在 production 装配时是否真的把 `random.NewCryptoWeightedPicker(rand.Reader)` 注入 fn" —— 若有人改 `buildChestServiceWithRepos` 或 `cmd/server/main.go` 把 picker 错配成 nil 或别的实装，所有现有测试都会绿。

### 修复（Fix）

**双轨方案（dual-track）**：

1. **保留 r2 stub case，改名为 `WeightedPickDistribution_DeterministicWiring_1000Opens`**：
   - 名字明确表达"验证 wiring（service 调度 picker + index→rarity 映射）"而非"验证分布"
   - 保留精确断言 900/90/9/1（0 flakiness）
   - 注释顶部增加 r3 改造决策段，解释为何与新 case 共存
2. **新增 case `WeightedPickDistribution_RealCryptoPicker_SmokeTest`**：
   - 用 `random.NewCryptoWeightedPicker(rand.Reader)` 真 picker
   - 小样本 N=100（节省 CI 时间，smoke test 不验证精度）
   - 极宽松下界断言 `common ≥ 50`：Binomial(100, 0.9) 下 P(X<50) ≈ 6e-29 → 0 flakiness（远低于"flaky 概率 < 0.1%"目标）
   - 同时断言 total==100（picker 调度次数正确）+ rarity ∈ {1,2,3,4}（picker 返回 valid index）
   - `t.Logf` 输出实际分布供 debug
3. **更新文件 header 注释**：把"15. WeightedPickDistribution_1000Opens"拆成"15a. DeterministicWiring + 15b. RealCryptoPicker_SmokeTest"两条 + 说明各自责任

**三层责任分离（最终形态）**：

| 层 | 文件 | 验证内容 | 样本 / 容差 | flakiness |
|---|---|---|---|---|
| 算法层（unit） | `internal/pkg/random/weighted_test.go` | distribution 算法（miscompute totals / wrong bucket 必挂） | 10000 + deterministic seed + ±5% | 0 |
| 集成 wiring 层 | 本文件 `DeterministicWiring_1000Opens` | service 调度 picker + index→rarity 映射 + 1000 事务串行 | 1000 + stub | 0 |
| 集成 production 兜底层 | 本文件 `RealCryptoPicker_SmokeTest` | production picker 真被 wire（不是 nil/错实装） | 100 + 真 crypto + 5σ 下界 | 6e-29（实际 0） |

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写集成测试需要随机算法（weighted picker / 抽签 / 加密 nonce 等）参与**时，**必须**做三层责任分离：
> ① 算法分布正确性在 unit test 层用 deterministic seed + 大样本 + 紧容差兜底；
> ② 集成 wiring 层用 stub 验证调度 + 映射（0 flakiness）；
> ③ 集成 production 兜底层用真随机 + 小样本 + 极宽松下界（5σ 之外，P(失败) < 1e-20）验证 production 装配真被调到。
>
> **展开**：
> - 不要让"算法验证"和"装配 wiring 验证"挤进同一个集成 case —— stub 同时承担两份责任时会偷偷丢失 production 兜底
> - 真随机断言的下界计算必须做：`Binomial(n, p)` 的 X ≥ k 处 P(X<k) < 1e-20（5σ tail）→ 集成测试每天跑 100 次 100 年也不挂；否则就是 flaky CI 隐患
> - 小样本（100）+ 极宽松下界 vs 大样本（10000）+ 紧容差 → smoke test 优先小样本（节省 CI 时间），紧容差留 unit test
> - 在文件 header 注释 / lesson 文档中**显式记录**算法验证落在哪个 unit test，让未来 reviewer 不会再问"为什么集成测试用 stub"
> - **反例**：r2 形态 —— "weighted_test.go 已覆盖算法，集成测试用 stub 就够了" → 漏掉 production picker 是否真被 wire 进 service 的兜底；service 装配阶段把 picker 错配 / 漏注入 / nil 时所有测试仍绿
> - **反例**：用 `mathrand` + 固定 seed 在集成测试里替代真 crypto picker → 虽 deterministic 但和 production 路径（crypto/rand）类型不一样，wiring 兜底仍未做到
> - **反例**：集成测试断言 `common ≥ 80 of 100` 这种"看起来宽松实际紧"的下界 —— Binomial(100, 0.9) 下 P(X<80) ≈ 1e-3 → 每 1000 次 CI 跑挂一次，仍属 flaky；下界必须放到 5σ 之外（≥ 50 而非 ≥ 80）
> - **正例**：本 r3 dual-track —— stub case 名字带 `DeterministicWiring`，新 case 名字带 `RealCryptoPicker_SmokeTest`，header 注释明确两 case 的责任划分，新 case 断言下界 50（5σ 外）

## Meta: r1-r3 over-correction chain 的最终形态（Story 20.9）

Story 20.9 r1 / r2 / r3 三轮 review 各自发现的问题构成了一条典型 over-correction chain：

| 轮 | codex finding | 修复方向 | 副作用 |
|---|---|---|---|
| r1 | 集成测试断言错误码与事务步骤顺序不符（4002 vs 3002） | 校准断言到 5d unlock_at 检查先于 5e steps 检查 | 无 |
| r2 | distribution case 用真 crypto picker → 1000 样本仍有 ~5% tail flakiness | 注入 deterministic stub picker | **副作用**：production picker 路径从集成测试消失 |
| r3 | r2 stub 让 production picker regression 不会被集成测试发现 | 双轨方案：保留 stub + 新增 real picker smoke test | 终止链条 |

**蒸馏教训**：每轮修复都"是对的"，但局部最优拼起来不是全局最优。`bmad-fix-review` 出现"r3 指出 r2 副作用"形态时，**必须**用 dual-track / 责任分离方案而非"再来一次反向 over-correction"（如把 r3 直接退回 r1 真 picker + 宽容差，那 r1 flaky 问题又回来）。这与 Story 20.7 r4 用"事务消除 RowsAffected 语义模糊"终结 r1-r4 链是相同范式：找到第三条路（升维），而非在两个错误之间摆动。
