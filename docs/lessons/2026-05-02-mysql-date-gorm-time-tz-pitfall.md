---
date: 2026-05-02
source_review: codex review on Story 7-3-post-steps-sync-接口-累计差值入账-service round 1 (file: /tmp/epic-loop-review-7-3-r1.md)
story: 7-3-post-steps-sync-接口-累计差值入账-service
commit: 2d2b84a
lesson_count: 2
---

# Review Lessons — 2026-05-02 — MySQL DATE 列 + GORM time.Time 的时区陷阱 & 配置 int64→int32 narrowing 静默扣款

## 背景

Story 7.3 实装 `POST /api/v1/steps/sync` 累计差值入账 service。codex review r1 在 handler 层发现一个跨服务器时区会写错日期的隐藏 bug，在 service 构造函数发现一个超大 YAML 配置会 wrap 成负数静默扣减用户余额的 narrowing bug。两条都是"测试不会暴露但生产会出事"的隐性问题，必须从设计层面修复并写入规则手册。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | syncDate 时区漏洞：UTC 0:00 + DSN loc=Local → 负偏移服务器写错日期 | high (P1) | architecture | fix | `server/internal/app/http/handler/steps_handler.go:68` |
| 2 | NewStepService int32 narrowing 无范围校验 → 超大配置 wrap 负值扣余额 | medium (P2) | config | fix | `server/internal/service/step_service.go:93-99` |

## Lesson 1: MySQL DATE 列 + GORM `time.Time` 字段 + 非 UTC DSN 的时区陷阱

- **Severity**: high (P1)
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/app/http/handler/steps_handler.go:68`

### 症状（Symptom）

handler 用 `time.Parse("2006-01-02", req.SyncDate)` 把 `"2026-05-01"` 解析成 `time.Time` 传给 service 与 GORM。该返回值是 **`2026-05-01 00:00:00 UTC`**。当 mysql DSN 是 `loc=Local`（项目当前 `configs/local.yaml` 第 15 行 + 集成测试 DSN），go-sql-driver 在序列化 `time.Time` 到 DATE 列时会先 `t.In(loc)` 转到连接 loc 再 format `YYYY-MM-DD`：

- 服务器 TZ = `+08:00`：`2026-05-01 00:00 UTC` → `2026-05-01 08:00 Local` → DATE 写 `2026-05-01`（看似正确，纯属正偏移让日期"飘进"同一天的巧合）
- 服务器 TZ = `-05:00`（如 us-east-1）：`2026-05-01 00:00 UTC` → `2026-04-30 19:00 Local` → DATE 写 **`2026-04-30`**（**bug：少一天**）

后果链：差值入错日期分桶 → `FindLatestByUserAndDate` / `SumAcceptedDeltaByUserAndDate` 找错行 → dailyCap 累计错日 → 跨端审计串日。**dev 机器都在 +08，单测也用 UTC fixture，CI 也是 +08 环境跑——所以这条永远不会被现有测试暴露**，只能在生产部署到非正偏移机房时才炸。

### 根因（Root cause）

`time.Parse(layout, value)`  的"沉默默认"是返回 `time.UTC` Location，**不**问环境与 DSN。当 DSN 是 `loc=Local` 时，序列化路径会做一次"UTC → Local"转换；这次转换在跨日边界上是 lossy（calendar date 改变），但代码层完全感知不到。**Calendar date（日历日，如"今天"）不是 instant（带时区时间点），不应承载时区语义**——但 Go 的 `time.Time` 类型把两者强行混在一起，触发条件极隐蔽。

### 修复（Fix）

handler 改用 `time.ParseInLocation("2006-01-02", req.SyncDate, time.Local)`，让解析出的 `time.Time` 与 DSN `loc=Local` 锁同步：

```go
// before
syncDate, err := time.Parse("2006-01-02", req.SyncDate)

// after
syncDate, err := time.ParseInLocation("2006-01-02", req.SyncDate, time.Local)
```

这样不论服务器 TZ 是 +08 / -05 / 0，driver 转回 Local 都是 no-op（同一 loc），DATE 列写入就是用户传的那个日历日。

新增 handler 单测 `TestStepsHandler_PostSync_SyncDateParsedInLocalLocation_TZSafeRegression` 验证 `in.SyncDate.Location() == time.Local` + Year/Month/Day 与请求字符串一致 + 时分秒 == 0。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **把"日历日"字符串（YYYY-MM-DD）解析成 `time.Time` 准备写入 MySQL DATE 列** 时，**必须** **用 `time.ParseInLocation(layout, s, time.Local)`，并确保 `time.Local` 与 DSN `loc=` 参数对齐**。**禁止**裸用 `time.Parse(layout, s)`（默认 UTC）。
>
> **展开**：
> - 触发条件三要素：①  字段语义是"日历日"（business calendar date，如 syncDate / 生日 / 账期），不是带时区的 instant；②  存储列是 MySQL `DATE` 类型；③  连接 DSN 的 `loc` 不是 UTC。任一缺失则规则不强制，但应保留警觉。
> - DSN 是 `loc=Local` 时：用 `time.ParseInLocation(..., time.Local)` 让 parse 阶段就和 driver 序列化阶段对齐。
> - DSN 是 `loc=UTC` 时：用 `time.ParseInLocation(..., time.UTC)`（或裸 `time.Parse`，等价）。
> - **更稳的做法**（防止未来 DSN 改动）：在 service / repo DTO 层把 calendar date 表达为字符串（`"2026-05-01"`） 或 `civil.Date` 这类无时区类型，在 GORM struct 字段里用 `string` 而非 `time.Time` 映射 DATE 列，从根上消除时区语义。本次未做该重构（最小变更原则），但若未来出现第二处 DATE 列即应一次性迁移。
> - **测试纪律**：handler-level 单测必须断言 `in.SyncDate.Location()` 与预期对齐；不能只验证 Year/Month/Day（dev 机器通常 +08，UTC vs Local 在正偏移下 Year/Month/Day 一致，会让 bug 漏网）。
> - **反例**：
>   - `t, _ := time.Parse("2006-01-02", req.Date)` 然后传给 GORM 写 DATE 列 + DSN `loc=Local`：在负偏移服务器写错日期。
>   - 单测里 fixture `time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)`：dev 机器 +08 跑过，部署到 us-east 出 bug，单测仍然绿。
>   - "我以为 GORM 会自动处理时区"：GORM 把 `time.Time` 直接交给 driver，driver 按 DSN `loc` 做转换；GORM 不感知 calendar date 语义。

## Lesson 2: 配置 int64 → int32 narrowing 必须在边界 fail-fast，否则静默扣款

- **Severity**: medium (P2)
- **Category**: config
- **分诊**: fix
- **位置**: `server/internal/service/step_service.go:93-99`

### 症状（Symptom）

`config.StepsConfig.SingleSyncCap` 是 `int64`（YAML 反序列化天然类型），`NewStepService` 构造函数直接 `int32(cfg.SingleSyncCap)` 无范围校验。如果运维误填 YAML `single_sync_cap: 5000000000`（5e9，超 `math.MaxInt32 = 2147483647`）：

1. `int32(5_000_000_000)` 的 Go 行为是 wrap（取低 32 位有符号）→ 实际值约 `705_032_704` 正数（具体取决于值），但若值再大些就会变负数（如 `int64(math.MaxInt32) + 1 = 2147483648`，cast 后 = `-2147483648`）。
2. service 内 `delta := rawDelta` 是 int64；`delta > int64(s.singleSyncCap)` 把负数当上限 → 任何正向 sync 的 `rawDelta > -2147483648` 永远成立 → 走 truncate 分支 → `delta = int64(s.singleSyncCap) = -2147483648`。
3. `int32(delta) = -2147483648` 写入 `accepted_delta_steps` 列 + 传给 `UpdateBalance(... delta int32 ...)` → **余额被扣减 21 亿步**，而不是按正向 cap 截断。

后果：用户余额被静默清零甚至下溢，无任何错误日志（防作弊截断警告还会"正常"打出来，迷惑性极强）。生产侧只能通过余额诡异下降才发现，且回溯困难（sync_log.accepted_delta_steps 也是负值，看起来像"业务正确"）。

### 根因（Root cause）

两个层面：

1. **类型边界感知缺失**：YAML config 字段普遍用 `int64`（兼容更大数 / 与文档侧"int 即 int64"语义一致），但业务侧字段（如 `accepted_delta_steps` 列是 INT signed = int32）天然是 int32 范围。从 `int64` cast 到 `int32` 是 narrowing conversion，Go 不报警、不 panic、不返回错误，**静默 wrap**。
2. **业务语义边界缺失**：`single_sync_cap` 这种"截断阈值"配置，**负数 / 超限值都没有合法业务语义**（cap < 0 等价于"任何 delta 都被截断为负"，没有用户场景）。但代码没有显式拒绝，等价于把"配置项的取值集合"开放给了无穷大整数集——任何越界都是潜在 bug 触发器。

### 修复（Fix）

把 service 内部 `singleSyncCap` 字段类型从 `int32` 改为 `int64`（与 config 字段一致，避免存储期 narrowing），并在 `NewStepService` 添加 fail-fast 范围校验：

```go
// before
singleSyncCap := int32(defaultStepsSingleSyncCap)
if cfg.SingleSyncCap > 0 {
    singleSyncCap = int32(cfg.SingleSyncCap) // 静默 narrowing，无范围校验
}

// after
if cfg.SingleSyncCap < 0 {
    panic(fmt.Sprintf("step service: single_sync_cap=%d 不能为负数", cfg.SingleSyncCap))
}
if cfg.SingleSyncCap > math.MaxInt32 {
    panic(fmt.Sprintf("step service: single_sync_cap=%d 超过 int32 上限 %d ...", cfg.SingleSyncCap, math.MaxInt32))
}
if cfg.DailyCap < 0 {
    panic(fmt.Sprintf("step service: daily_cap=%d 不能为负数", cfg.DailyCap))
}
singleSyncCap := int64(defaultStepsSingleSyncCap)
if cfg.SingleSyncCap > 0 {
    singleSyncCap = cfg.SingleSyncCap // 已校验 ≤ MaxInt32，存储用 int64 避免后续 narrowing
}
```

实际写入 `accepted_delta_steps` 时仍 `int32(delta)`，但 `delta ≤ singleSyncCap ≤ MaxInt32` 已被构造函数保证，cast 安全。新增 4 个单测（oversized panic / negative panic / daily_cap negative panic / MaxInt32 边界 OK）覆盖。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **把 YAML config 字段（int64）赋给业务侧 int32/int16/int8 字段** 时，**必须** **在构造函数边界 fail-fast 校验范围（`< 0` panic + `> max` panic）**，**禁止**裸 `int32(cfg.X)` cast 后塞给运行期。
>
> **展开**：
> - 触发条件：configs.go 字段是 `int64`（YAML 默认类型），而下游业务字段 / DB 列 / 跨端 API 钦定 < int64 范围（`int32` / `int16` / `int8` / `uint8`）。
> - **校验位置**：构造函数（`New<X>Service`）启动期一次 fail-fast，不在 hot path 反复校验。Panic 比 return error 合适——错配置应阻断启动而非降级运行。
> - **校验内容**：①  `< 0` 是否合法（多数场景不合法，cap / size / count 类配置都不该负）；②  `> 类型 max` 是否合法（永远不合法 if 下游是定宽类型）；③  `== 0` 看业务语义（zero-value 兜底用默认值是常见模式，本次保留）。
> - **存储类型选择**：service 内部状态字段类型应 ≥ config 字段类型，避免存储期 narrowing。只在最终写入定宽列 / 接口边界时才 cast，且 cast 前必须有"已被构造函数验证"的不变量保证。
> - **单测纪律**：fail-fast panic 必须有专用回归 case（`defer recover` 模式）；边界值 `MaxInt32` 必须有"合法不 panic"的 case 防 off-by-one（误用 `>=` 会把合法上限也拒）。
> - **反例**：
>   - `cap := int32(cfg.Cap)`（无校验）：cfg.Cap = 5e9 → wrap 负数 → 业务逻辑全反。
>   - `if cfg.Cap > 0 { cap = int32(cfg.Cap) }`（只防负无防超限）：`cfg.Cap = 1<<40` 仍能 wrap。
>   - "validator/v10 在 config 包加 max tag"：YAML 解组后才校验，但构造函数若不显式调 `Validate()` 就被跳过；fail-fast 应放在构造函数启动期硬编码校验，不依赖外部 validator。
>   - "测试用 `cfg.Cap = 5000`，所以没问题"：单测永远不会传超大值；类型边界 bug 只能被显式范围校验或 fuzzing 暴露，不能被典型 case 覆盖。

---

## Meta: 本次 review 的宏观教训

两条 finding 共享同一个思维漏洞：**"看似业务正确但被默认行为劫持"的隐性 bug**。`time.Parse` 默认 UTC、`int32(int64)` 默认 wrap，都是语言/库的"沉默默认"——**调用代码看不到任何错误信号**，但部署条件改变（服务器换 TZ / 配置换数量级）就引爆。

未来 Claude 接触"穿越类型边界"或"穿越运行时配置边界"的代码时，应**显式问自己**：

1. 这个 conversion / parse 的"沉默默认"是什么？默认值在哪些部署条件下变成 bug？
2. 我能在边界处加一道 fail-fast 检查吗？（panic / return error / static assert）
3. 单测覆盖了"非典型部署条件"吗？（不同 TZ / 不同数量级 / 不同字符集）

不要把"测试通过 + 看起来对"当作"实际对"。
