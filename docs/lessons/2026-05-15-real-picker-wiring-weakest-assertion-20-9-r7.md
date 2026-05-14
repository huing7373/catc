---
date: 2026-05-15
source_review: /tmp/epic-loop-review-20-9-r7.md (codex review round 7)
story: 20-9-layer-2-集成测试-开箱事务全流程
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-15 — real picker wiring 弱断言：r6 整 case 删除 → r7 弱化断言反弹

## 背景

Story 20.9 r6 收尾时为了让 r5 引入的"概率断言 flaky"问题彻底归零，选择了**选项 A：删除整个 `RealCryptoPicker_SmokeTest` case**。r7 codex review 指出这是**过度修复**：集成测试不再 exercise `random.NewCryptoWeightedPicker` 路径 → 如果 `buildChestOpenServiceIntegration` / `NewChestService` 在装配阶段把 picker 错 wire 成固定 picker / nil，无任何集成 case 兜底。本 lesson 沉淀**责任三角的最弱版本** + r6 反弹学习。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | r6 选项 A 整 case 删除 → production picker wiring 在集成层失兜底（应改用选项 B 弱化断言） | P2 | testing | fix | `server/internal/service/chest_open_service_integration_test.go:1205-1320` |

## Lesson 1: real picker wiring 的最弱断言 — wiring + algorithm + flakiness 三选二的集成测试最优解

- **Severity**: P2（影响 Layer 2 集成测试整体可信度 + 长期"如何处理 review 概率断言 flaky"决策范式）
- **Category**: testing
- **分诊**: fix（新增 `TestChestOpenServiceIntegration_WeightedPickDistribution_RealCryptoPicker_WiringOnly` case + 保留 `DeterministicWiring_1000Opens` case 不动）
- **位置**: `server/internal/service/chest_open_service_integration_test.go:1205-1320`

### 症状（Symptom）

r6 收尾时为彻底归零 r5 引入的"`counts[2] >= 1` 在 N=100 真随机下 P ≈ 7e-5 flaky"问题，整段删除 `RealCryptoPicker_SmokeTest` case，仅保留 `DeterministicWiring_1000Opens`（stub picker）。codex r7 指出：

> "TestChestOpenServiceIntegration_WeightedPickDistribution_DeterministicWiring_1000Opens now builds the service with a stub picker, and the accompanying comments explicitly remove the old real-picker smoke test. That means this file no longer exercises the production random.NewCryptoWeightedPicker path at all: if buildChestOpenServiceIntegration or NewChestService starts wiring a fixed/incorrect picker, every case here can still stay green because they either ignore rarity entirely or inject their own picker."

集成测试**所有 case** 要么用 stub picker（DeterministicWiring）要么不关心 rarity（happy path / 边界 / 并发 / 幂等）→ 把 `random.NewCryptoWeightedPicker(rand.Reader)` 错配成 nil / 错 wire 时**无 case 会挂**。

### 根因（Root cause）

r6 "选项 A 删除整 case" 触发了**核弹级修复**：把"概率断言 flaky"和"production picker wiring 兜底"两个不同维度的责任绑在了同一段代码上，删 case 就同时丢两份。

**思维漏洞**：r6 在评估"如何让 case 0 flakiness"时，没在元层面问"这个 case 的**核心责任**是什么、能不能只删 flaky 断言保留核心责任"。三个可能的修复梯度：

| 选项 | 操作 | flakiness 改善 | wiring 兜底 | 评价 |
|---|---|---|---|---|
| A（r6 选）| 删整 case | 0 flakiness | **丢失** | 过度修复 |
| **B（r7 选）**| **保留 case + 弱化断言** | 0 flakiness | **保留** | 正解 |
| C | 加 χ² 检验代替概率断言 | 0 flakiness | 保留 | 引入额外复杂度（χ² 计算函数），MVP 阶段不必要 |

r6 跳过 B/C 直接选 A 的反思：**"完美 0 flakiness" 不应以"放弃 wiring 兜底"为代价；正确做法是把概率断言降级到"必然成立的最弱断言"（`total == N` + `rarity ∈ enum`）而非删除整 case**。

更深层次：r2-r6 over-correction chain 让 r6 处于"宁可删过头也要终结 chain"的心态，于是把 case 整删而非精确修；r7 反弹证明 chain 终结**不是**"砍掉所有有 flaky 风险的 case"，而是"在每个 case 上精确分离 flaky 维度 vs 核心责任维度"。

### 修复（Fix）

**采用 r7 选项 B（保留 case + 退到最弱断言）**：

1. **新增** `TestChestOpenServiceIntegration_WeightedPickDistribution_RealCryptoPicker_WiringOnly` case：
   - 用 `buildChestOpenServiceIntegration` 真 picker 装配（与其他 case 同 helper → 一旦装配链 broken，本 case + 其他 case 同步挂；同 helper 单点失败覆盖最大化）
   - N=100 次开箱
   - **极简断言**：
     - `total == N`（picker 真被调度 + 全部 success；wiring broken / picker nil panic → 必挂）
     - 每次 `out.Reward.Rarity ∈ {1, 2, 3, 4}`（picker 返合法 index + service index→rarity 字段映射正确）
     - DB `chest_open_logs.reward_rarity ∈ {1, 2, 3, 4}`（落库 rarity 正确，防 service 返合法值但落库错值）
   - **不**断言 distribution（不 `common >= X` / `rare >= Y` 等 probabilistic 断言）
   - **不**断言 "至少 2 distinct rarity"（避免 r5/r6 真随机 flaky 重演）
   - case 注释明确："本 case 仅验证 wiring（生产 picker 真被 wire + 返合法 rarity），分布正确性由 `weighted_test.go` 单测覆盖"

2. **保留** `DeterministicWiring_1000Opens` case 不动：
   - stub picker 严格 900/90/9/1 精确断言 → 验证 service 调度 + index→rarity 映射

3. **AC14 story 描述同步更新**：
   - `_bmad-output/implementation-artifacts/20-9-layer-2-集成测试-开箱事务全流程.md` AC14 节增加 "r2-r7 chain 终稿"三 case 责任分离表 + r6→r7 反弹学习段

**最终责任三角（r7 锁定）**：

| 验证维度 | 覆盖层 | helper / picker | 样本 / 断言 | flakiness |
|---|---|---|---|---|
| 算法分布正确性（weight 计算 / 概率分布） | 单测 `weighted_test.go` | 真 picker + `mathrand.Source` deterministic seed | N=10000 + ±5% 容差 | 0 |
| service wiring（调度 + index→rarity 映射 + 1000 事务串行） | 集成 `DeterministicWiring_1000Opens` | stub `raritySequencePicker` | N=1000 + 精确 900/90/9/1 | 0 |
| **production picker injection（装配链真把 `random.NewCryptoWeightedPicker` wire 进 service）** | 集成 **`RealCryptoPicker_WiringOnly`** | 真 `random.NewCryptoWeightedPicker` | N=100 + `total==N` + rarity∈{1..4} | **0** |

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **接到 review 报"集成测试某 case 概率断言 flaky"** 时，**必须**先尝试**弱化断言到 0-flakiness 最弱形态**（如 `total == N` / 枚举合法性 / `value > 0` 类必然成立的断言），**禁止**首先选择"删除整个 case" —— 删除 case 是核弹级修复，会同时丢失该 case 的**所有**职责（包括与 flaky 维度无关的 wiring 兜底 / smoke test 等）。
>
> **展开**：
> - 接到"case X 的断言 Y flaky"反馈时，按以下优先级评估：
>   1. **降级 Y 到 0-flakiness 必然成立形态**（最优 — wiring 兜底完整保留，flakiness 归零）
>   2. **替换 Y 为更精确算法**（如 probabilistic → χ² 检验；增加复杂度，MVP 阶段慎用）
>   3. **拆分 case：Y 独立成新 case + 给宽松断言；原 case 删 Y**（次优 — 多 case 但责任清晰）
>   4. **删除整个 case Y 所在 X**（最后手段 — 只在 case X 的**全部职责**都已被其他 case 覆盖时才允许）
> - 0-flakiness 最弱断言模板（按场景）：
>   - 真随机抽样 → `total == N` + 每个 value ∈ 合法枚举集 + DB 写入 value 一致性
>   - 真时钟 → 固定时刻 fixedClock 注入 + 边界值 ± 1ms / 1ns 精确断言（避免 wall-clock 测量）
>   - 真并发 → 最终状态等价 + barrier 同步 + race detector（`-race`）兜底；**不**断言 wall-clock timing ratio
> - **责任三角的最弱版本（核心规则）**：集成测试 = wiring + algorithm + flakiness，三选二（不能三全）。集成层必选 **wiring + 无 flakiness**，algorithm distribution 留单测层。这是 Story 20.9 r2-r7 七轮迭代最终收敛的硬约束。
> - **反例（r6 形态）**：为消除 r5 概率断言 flaky，**整段删除** `RealCryptoPicker_SmokeTest` case → flakiness 归零但 production picker wiring 失兜底 → r7 反弹。
> - **反例（r5 形态）**：为补足 r3 的"真 picker wiring 缺失"，加 `counts[2] >= 1` + `len(seen) >= 2` 双概率断言 → P(false-failure) ≈ 1e-4 → CI 月度 ~1% 红灯。
> - **反例（r2 形态）**：为消除真随机 flaky，**完全用 stub 替换** 真 picker → algorithm distribution 留单测 ✓ 但 production picker wiring 也丢了 ✗ → r3 反弹。
> - **正例（r7 终稿）**：
>   - 集成层：DeterministicWiring（stub，验调度 + 映射 + 0 flakiness）+ RealCryptoPicker_WiringOnly（真 picker，仅 total + rarity enum 断言，0 flakiness）
>   - 单测层：`weighted_test.go` deterministic seed + 10000 样本 + ±5% 容差（algorithm distribution，0 flakiness）

## Meta: r2-r7 over-correction chain 的最终形态（Story 20.9 七轮收敛）

Story 20.9 自 r1 起经历 7 轮 review，每轮发现上轮"修复"的副作用：

| 轮 | codex finding | 修复方向 | 副作用 / 下轮 finding |
|---|---|---|---|
| r1 | 错误码断言与事务步骤顺序不符（4002 vs 3002）| 校准 5d unlock_at 检查先于 5e steps 检查 | r2 指出真随机/真时钟在边界断言上 flaky |
| r2 | distribution case 真 picker N=1000 仍有 ~5% tail flakiness | 注入 deterministic stub picker | r3 指出 stub 把 production picker 路径完全移除 |
| r3 | r2 stub 失去 production picker wiring 兜底 | 双轨方案（stub DeterministicWiring + real picker smoke）| r4 指出并发 100 goroutine 缺 start barrier |
| r4 | 并发 spawn 循环慢于业务调用 → race 不真触发 | 加 `close(start)` barrier | r5 指出 race vs serial 结果断言等价不可区分 |
| r5 | race vs serial 不可区分 | 加 wall-clock `serialRatio < 0.5` + 真随机 `counts[2] >= 1` 双断言 | r6 指出两者均 flaky（timing 依赖 CI 负载，概率 ~7e-5）|
| r6 | r5 timing + 真随机断言 flaky | 选项 A：**删 timing + 整 case 删除** | **r7 指出整 case 删除是过度修复 → wiring 失兜底**|
| r7（本轮）| r6 选项 A 过度修复 | **选项 B**：保留 real picker case + 弱化到最弱断言（`total==N` + `rarity ∈ enum`）| **chain 真正终结**|

**蒸馏：r6 → r7 反弹的核心学习**：

- **chain 终结 ≠ 砍掉所有有 flaky 风险的 case**；chain 终结 = "**每个 case 上精确分离 flaky 维度 vs 核心责任维度**"
- r6 用"删整 case"换"chain 终结"，但 r7 找到更精确的解：**弱化断言**保留核心责任 + 去掉 flaky 风险
- **修复梯度黄金法则**：从最低梯度（弱化断言）逐步上升到最高梯度（删 case），不要跳过中间梯度
- **review chain 长度 ≠ 团队失败**；每一轮都精化测试责任分离的认知边界，r7 是 r3 责任分离的**完整版**（r3 提出双轨但 real picker case 仍带概率断言；r7 把 real picker case 退到"仅 wiring 验证"的纯净形态）

这次七轮收敛把"集成测试如何在 wiring + algorithm + flakiness 三角中精确取舍"的边界条件全部刻画到位 —— 后续凡涉及"真随机 / 真时钟 / 真并发"的集成测试都可直接套用本 lesson 的责任分离矩阵 + 修复梯度黄金法则。
