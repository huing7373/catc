---
date: 2026-05-15
source_review: codex review round 2 — /tmp/epic-loop-review-20-9-r2.md
story: 20-9-layer-2-集成测试-开箱事务全流程
commit: f1b3827
lesson_count: 2
---

# Review Lessons — 2026-05-15 — 集成测试不准用真随机/真时钟做边界断言（20-9 r2）

## 背景

Story 20.9 r1 实装在 `chest_open_service_integration_test.go` 加了 12 个新 case。
其中 2 个 case 的断言**绑了 wall clock / 真 RNG**，让 `--integration` CI gate 在
"production 代码无 bug"时也可能误红灯：

- AC14 `WeightedPickDistribution_1000Opens`：用真实 crypto-weighted picker + 1000
  样本 + 固定分布桶断言；
- AC13 `UnlockAtMinus1ms_IsUnlockable`：用 wall clock `time.Now().Add(-1ms)` 后
  再让 service 内 `s.nowFn() = time.Now()` —— 两次取 `time.Now()` 在 busy CI runner
  上间隔可能远大于 1ms，让"边界 1ms"语义实际未被锁住。

r2 修复：注入 deterministic stub picker + 通过 `service.SetChestServiceNowFn`
钩子注入 fixed clock。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | rarity 分布断言依赖真随机 → flaky | P2 | testing | fix | `server/internal/service/chest_open_service_integration_test.go` |
| 2 | unlock_at=-1ms 边界用 wall clock → 实际未锁边界 | P3 | testing | fix | `server/internal/service/chest_open_service_integration_test.go` + 新 `export_test.go` |

## Lesson 1: 集成测试断言"真随机分布"会把 CI gate 拖成 flaky

- **Severity**: P2
- **Category**: testing
- **分诊**: fix
- **位置**: `server/internal/service/chest_open_service_integration_test.go:984-998`（r1 行号）

### 症状（Symptom）

r1 实装：
```go
weightedPicker := random.NewCryptoWeightedPicker(rand.Reader)  // 真 crypto RNG
svc := service.NewChestService(..., weightedPicker)
// ... 1000 次开箱 ...
if counts[1] < 820 || counts[1] > 980 { t.Errorf(...) }  // common
if counts[2] < 50  || counts[2] > 130 { t.Errorf(...) }  // rare
if counts[3] < 0   || counts[3] > 25  { t.Errorf(...) }  // epic
if counts[4] < 0   || counts[4] > 8   { t.Errorf(...) }  // legendary
```

drop_weight 比例 100:20:4:1（common:rare:epic:legendary） → 1000 次开箱期望
分布为 common ≈ 900 / rare ≈ 90 / epic ≈ 9 / legendary ≈ 1.1。

虽然 r1 已用"宽松区间"（±10% common/rare，0-25 epic，0-8 legendary），但：

- rare 期望 90，σ ≈ 9.6，±3σ 区间 [62, 119]；r1 区间 [50, 130] 包住 ±3σ；
- legendary 期望 1.1，Poisson 分布 P(count=0) ≈ e^-1.1 ≈ 33%；r1 区间 [0, 8] 接受 0
  → 这条不再 flaky，但...

更深层问题：**集成测试的关注点是 service 层 + DB 层联调正确性**，不是 picker
算法本身的分布正确性。让 picker 走真 RNG → 引入"统计 flakiness"的不确定性
"血缘"，违反 ADR-0001（测试稳定性优先于真实性）。

### 根因（Root cause）

写集成测试时，下意识"既然是集成测试，就应该用生产路径的所有依赖"——
**包括 RNG**。这是误解："集成测试" = "多模块联调" ≠ "全部生产依赖直连"。
RNG / 时钟 / 外部 API / 文件系统这类**不确定性依赖**应该在测试边界注入
deterministic stub，否则测试就把不确定性的 tail 全部继承了。

### 修复（Fix）

注入 `raritySequencePicker` stub —— 实现 `random.WeightedPicker` 接口，按预定
sequence（common×900 → rare×90 → epic×9 → legendary×1）返回 item index。
service 收到 stub 返回的 index 后照常写 `chest_open_logs.reward_rarity`。

```go
stub := newRaritySequencePicker(t,
    raritySequenceSpec{desiredWeight: 100, count: 900}, // common
    raritySequenceSpec{desiredWeight: 20,  count: 90},  // rare
    raritySequenceSpec{desiredWeight: 4,   count: 9},   // epic
    raritySequenceSpec{desiredWeight: 1,   count: 1},   // legendary
)
svc := service.NewChestService(chestRepo, txMgr, ..., stub)
// ... 1000 次开箱后 ...
if counts[1] != 900 { t.Errorf(...) }   // 精确断言（0 flakiness）
if counts[2] != 90  { t.Errorf(...) }
if counts[3] != 9   { t.Errorf(...) }
if counts[4] != 1   { t.Errorf(...) }
if stub.calls() != 1000 { t.Errorf("picker 被调次数不对") }
```

stub 不依赖 `cosmetic_items` 表的 ORDER BY 顺序 —— 每次 Pick(items) 在 items
里线性扫找第一个 Weight 等于 desiredWeight 的 index（drop_weight ∈ {100, 20, 4, 1}
唯一可区分）。

**Picker 算法自身的分布正确性**：留给 `internal/pkg/random/weighted_test.go` 的
stand-alone unit test 验证（可用大样本 + 确定性 seed `mathrand.NewSource(123)`
检查均匀性，详见现有 `TestWeightedPicker_Pick_MultipleItems_DistributionWithDeterministicSeed`）。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **写集成测试碰到 RNG / 时钟 / 外部 API 依赖** 时，
> **必须** 注入 deterministic stub，**禁止** 把生产 RNG / wall clock 直连进
> 集成测试的断言路径。
>
> **展开**：
> - 集成测试的关注点是"多模块联调路径"，不是"统计/概率/分布算法本身的正确性"
> - RNG 算法的分布正确性归 unit test 验证（用确定性 seed + 大样本 + chi-square
>   或大区间），不归集成测试
> - 注入 stub 的方式：identify domain interface（如 `WeightedPicker`） → 测试
>   写 stub 实现 → 通过 constructor / DI 注入；不要为此动生产 API
> - **反例**：把 `random.NewCryptoWeightedPicker(rand.Reader)` 直接给集成测试 +
>   写"宽松区间"断言来 cover tail outcome。即使 ±3σ 区间也只是把 flaky 概率
>   降到 ~0.3%，CI 跑 1000 次仍会红 3 次；况且"宽松区间"会失去断言敏感性 —— 真
>   出 distribution bug 也容易漏掉。

## Lesson 2: 边界测试必须用 fixed clock，不准走 wall clock

- **Severity**: P3
- **Category**: testing
- **分诊**: fix
- **位置**: `server/internal/service/chest_open_service_integration_test.go:963-970`（r1 行号）

### 症状（Symptom）

r1 实装：
```go
// test 内：
unlockAt := time.Now().UTC().Add(-1 * time.Millisecond)
insertChest(t, sqlDB, 9001, userID, 1, unlockAt, 1000)
svc.OpenChest(...)  // 期望成功
```

service 内：
```go
// chest_open_service.go:224
now := s.nowFn()  // = time.Now().UTC()（默认）
isUnlockable := chest.Status == 2 || (chest.Status == 1 && !chest.UnlockAt.After(now))
```

测试想验证 "unlockAt 比 now 早 1ms 时仍 unlockable"。但：

- test 取 `t1 := time.Now()`，立即 SQL `insertChest`（含 DB RTT、GORM 反射）；
- service 后续取 `t2 := s.nowFn() = time.Now()`，比较 `unlockAt vs t2`；
- `t2 - t1` 实际 delta = `(t1 - 1ms) + RTT + GORM处理时间 ... < t2` 的 gap 可能是
  几十毫秒到几百毫秒（busy CI runner）；
- 即使 service 把 `!After(now)` 错改成 `unlockAt.Add(5*time.Millisecond).Before(now)`
  这种 regression，本测试仍可能误判通过 —— 实际 delta >> 5ms。

**测试没真正锁住 "差 1ms"的边界**，违反了 "AC13 边界 4" 的语义。

### 根因（Root cause）

下意识用 `time.Now()` 表达"当前时刻锚点"，没意识到当 service 内部**再次**调
`time.Now()` 时，两次取值之间的 delta 由 runner 性能决定，对"差 N 毫秒"这种
小尺度边界测试而言**完全不可控**。

边界测试的本质是"在 exact threshold 处验证 == / </ > 的行为"—— 必须让 test
和被测代码看到**完全相同**的 now 值。wall clock 不可能做到。

### 修复（Fix）

新增 `server/internal/service/export_test.go`（_test.go 后缀 → 仅 go test 时编入，
生产 binary 不携带）：

```go
package service
import "time"
// SetChestServiceNowFn 覆盖 chestServiceImpl.nowFn 字段（仅测试用）。
func SetChestServiceNowFn(svc ChestService, fn func() time.Time) {
    impl, ok := svc.(*chestServiceImpl)
    if !ok { panic("SetChestServiceNowFn: svc is not *chestServiceImpl") }
    impl.nowFn = fn
}
```

集成测试改造：

```go
fixedNow := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
service.SetChestServiceNowFn(svc, func() time.Time { return fixedNow })
unlockAt := fixedNow.Add(-1 * time.Millisecond)
insertChest(t, sqlDB, 9001, userID, 1, unlockAt, 1000)
// 现在 service 内 s.nowFn() 必返 fixedNow，
// unlockAt vs s.nowFn() = exact 1ms delta，精确验证边界。
```

如果 service 把 `!After(now)` 错改成 `Before(now)`（即 strict `<`），则
`unlockAt = fixedNow - 1ms` 不再满足 `Before(fixedNow)` 的反义 → 测试会立即 fail
（精确 catch regression）。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 在 **写边界测试涉及时间字段** 时，**必须** 注入
> fixed clock（test 和被测代码共享同一时刻锚点），**禁止** 用 `time.Now()` 做
> 边界锚点。
>
> **展开**：
> - "边界测试" = 在 exact threshold 处验证 `==` / `<` / `<=` / `>=` 行为
> - 用 wall clock 表达 "threshold ± Nms" → test 和被测代码各调一次 `time.Now()`
>   → 实际 delta 由 runner 性能决定，对小尺度阈值完全不可控
> - 注入 fixed clock 的途径（按优先级）：
>   1. 生产代码本就支持 `clock interface` / `nowFn func()` → 直接注入
>   2. 不支持 → 加 `export_test.go` 暴露内部钩子（仅测试编译可见）
>   3. 都不行 → 重构生产代码加 clock 抽象（不要绕过；wall-clock 边界测试不靠谱）
> - **反例**：
>   - `unlockAt := time.Now().Add(-1*time.Millisecond)` 让 service 内 `s.nowFn() =
>     time.Now()` 重新取 → 两次 now 间隔不可控
>   - 用"宽边界"（如 `-1ms` 改成 `-1s`）回避问题 → 失去"差 1ms"的语义精度，
>     service 把 `<=` 错改成 `<` 也 catch 不到
> - **export_test.go 模式**：Go 标准库（`io` / `encoding/json` / `net/http`）的成熟
>   pattern；`_test.go` 后缀让该文件仅在 `go test` 时编入，生产 binary 不携带 →
>   零生产副作用，纯测试钩子

---

## Meta: 本次 review 的宏观教训

两条 finding 共享同一思维漏洞：**"集成测试 = 走全部生产依赖"是误解**。

正确的边界：
- 集成测试关注 "多模块联调路径正确性" —— DB / repo / service / transaction / DTO
  这些**确定性**层之间的契约联调；
- 不确定性依赖（RNG / 时钟 / 外部 API / 文件系统） → 即使在集成测试也要 stub，
  让测试结果完全 deterministic。

未来 Claude 写集成测试，**先识别"不确定性依赖"** → 全部注入 stub →
再写断言。这一步识别遗漏，CI 就会带 flaky tail。
