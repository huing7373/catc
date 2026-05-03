---
date: 2026-05-02
source_review: codex review of Story 7.3 r2 (`/tmp/epic-loop-review-7-3-r2.md`)
story: 7-3-post-steps-sync-接口-累计差值入账-service
commit: b7a342a
lesson_count: 3
---

# Review Lessons — 2026-05-02 — 步数 sync 基线必须单调 + required 字段必须用指针 + DATE 列必须 string 透传

## 背景

Story 7.3（`POST /api/v1/steps/sync` service）r1 codex review 已修了 MySQL DATE 时区漂移（用 `ParseInLocation(time.Local)`）+ int64→int32 narrowing。本轮 r2 codex 抓出**三条递进**问题：

1. 基线查询 `ORDER BY id DESC` 在乱序到达场景下让旧 sync 反成基线，重复入账步数（**P1 正确性**）
2. handler `clientTotalSteps` 用 `int64` 值类型，无法区分"未传字段"与"显式 0"（**P2 输入校验漏洞**）
3. r1 的 `time.Local` 修复仍耦合 DSN `loc` 配置项，DSN `loc=UTC` 时仍漂日；`syncDate` 全程不该走 `time.Time`（**P2 配置耦合根治**）

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | step_sync_log_repo 基线按 `id DESC` 取最新让旧 sync 成新基线 → 重复入账 | high (P1) | architecture | fix | `server/internal/repo/mysql/step_sync_log_repo.go` |
| 2 | handler `clientTotalSteps` 用值类型 int64 → 缺失字段被默认为 0 静默通过 | medium (P2) | error-handling | fix | `server/internal/app/http/handler/steps_handler.go` |
| 3 | handler `time.ParseInLocation(time.Local)` 仍耦合 DSN `loc` 配置 → 改 string 全程穿透 | medium (P2) | architecture | fix | `server/internal/app/http/handler/steps_handler.go` + repo + service |

## Lesson 1: append-only 日志表的"基线"查询必须按业务单调字段取，不按 INSERT 时序

- **Severity**: high (P1)
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/repo/mysql/step_sync_log_repo.go:113`

### 症状（Symptom）

`StepSyncLogRepo.FindLatestByUserAndDate` 用 `ORDER BY id DESC LIMIT 1` 取"最近"sync log 作为下一次 delta 计算的基线。手机端因网络重试 / 串行错乱可能让"旧 client_total_steps"INSERT 在"新 client_total_steps"之后，导致：

```
sync A: client_total=250 → INSERT id=1, accepted=250
sync B (旧报告延迟): client_total=200 → INSERT id=2, accepted=0
sync C: client_total=260
  - 基线按 id DESC 取 = id=2 (200)
  - delta = 260 - 200 = 60  ← 错！多入账 50 步
  - 正确：基线应取 max(client_total_steps)=250 → delta = 10
```

每次乱序到达都会产生 50 步"白送"，攻击者只需故意把旧 total 重发就能套利。

### 根因（Root cause）

把"PK 自增 ID 时序"等同于"业务时序"。两者只在**严格按时间顺序到达**的场景下同义；append-only 日志表的本质是"事件流"，事件可能乱序到达（重试 / 并发 / 网络抖动）。**业务时序应由业务字段定义**：

- 步数累计本身**单调非降**（健康源永远递增；倒退只可能是源系统重置，倒退场景独立处理）
- "基线"语义 = "迄今为止报告过的最大值"
- 用 `ORDER BY client_total_steps DESC, id DESC LIMIT 1` 直接对齐业务语义，乱序到达不影响

### 修复（Fix）

```go
// before
err := db.WithContext(ctx).
    Where("user_id = ? AND sync_date = ?", userID, syncDate).
    Order("id DESC").
    First(&log).Error

// after
err := db.WithContext(ctx).
    Where("user_id = ? AND sync_date = ?", userID, syncDate).
    Order("client_total_steps DESC, id DESC").
    First(&log).Error
```

`id DESC` 保留作第二排序键：两次同 `client_total_steps` 写入时取后一次（业务无影响，避免 GORM `First` 在 ties 上的非确定行为）。

新增**集成测试** `TestStepServiceIntegration_OutOfOrderSync_BaselineUsesMaxClientTotalSteps` 专门覆盖乱序场景：sync A (250) → sync B (200, 旧延迟) → sync C (260)，断言 `outC.AcceptedDeltaSteps == 10`，并加 regression sentinel 注释（"若 = 60 → repo 退化为 id DESC"）。

新增**单测** `TestStepSyncLogRepo_FindLatestByUserAndDate_OrderByClientTotalSteps_BaselineSQLAssertion` 用 sqlmock 严格匹 SQL 文本片段 `ORDER BY client_total_steps DESC,`，编译时即拦截 regression。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在为 **append-only 日志表**写 `FindLatest*` 类查询时，**必须**用**业务单调字段**（如累计计数 / 序列号 / 业务时间戳）作为 ORDER BY 主排序，**禁止**直接用 `id DESC` 当"业务最近"。
>
> **展开**：
> - "PK 自增 ID = 业务时序"只在**严格顺序到达**场景成立；移动端 sync / 异步任务 / 跨节点写入都可能乱序，结论不成立
> - "基线"类查询（差值计算 / 当前最高分 / 库存最大值）的语义**直接对应业务字段**，应该用业务字段而非代理字段（id / created_at）
> - PK 自增 ID 适合作**第二排序键**做 ties 决断（避免非确定行为），但**不能**作主排序键
> - 差值入账（delta = curr - baseline）类业务必须配套**乱序到达集成测试**：插入 max 行 → 插入更小行 → 断言下一次 delta 算的是 max 而非最近 INSERT 的小值
>
> **反例**：
> - `ORDER BY id DESC LIMIT 1`（基线 = 最近 INSERT 的行）→ 乱序攻击 / 网络重试 → 重复入账
> - `ORDER BY created_at DESC LIMIT 1`（基线 = 最近写入的行）→ 同上，时间戳和 id 同语义
> - **正例**：`ORDER BY client_total_steps DESC, id DESC LIMIT 1`（基线 = 业务最大值 + ties 用 id 决断）

## Lesson 2: required 数值字段必须用指针 + 显式 nil 校验，区分"未传"与"显式零值"

- **Severity**: medium (P2)
- **Category**: error-handling
- **分诊**: fix
- **位置**: `server/internal/app/http/handler/steps_handler.go:39-43`

### 症状（Symptom）

`PostSyncRequest.ClientTotalSteps` 是 `int64`（值类型），handler 只校验 `< 0`。Client 完全不传该字段时，`ShouldBindJSON` 把它绑定为 `0`（int64 zero value），与"显式传 0"（合法首次同步当日 0 步）**不可区分**。malformed client 可创建 sync log + 递增 step_account version 而不报告任何 total。

### 根因（Root cause）

Go 数值类型的 zero value 与"未设置"在静态类型层面合一；JSON 解析也按值类型默认零填充。要区分"缺失"与"显式零"必须借助 **`*T` 指针字段**（`nil` = 缺失，`*T == 0` = 显式零）。

不能用 `binding:"required"` 兜：validator/v10 在数值字段上**把 0 视为缺失**，会误拒首次同步当日 0 步的合法场景（这点 r1 lesson 已锁定）。

### 修复（Fix）

```go
// before
type PostSyncRequest struct {
    SyncDate         string `json:"syncDate" binding:"required"`
    ClientTotalSteps int64  `json:"clientTotalSteps"`
    MotionState      int8   `json:"motionState"`
    ClientTimestamp  int64  `json:"clientTimestamp"`
}

// after
type PostSyncRequest struct {
    SyncDate         string `json:"syncDate" binding:"required"`
    ClientTotalSteps *int64 `json:"clientTotalSteps"` // 指针：区分缺失 vs 显式 0
    MotionState      *int8  `json:"motionState"`
    ClientTimestamp  *int64 `json:"clientTimestamp"`
}

// handler 显式 nil 校验（在 ShouldBindJSON 之后、范围校验之前）
if req.ClientTotalSteps == nil {
    _ = c.Error(apperror.New(apperror.ErrInvalidParam, "clientTotalSteps 必填"))
    return
}
// ...类似处理 MotionState / ClientTimestamp
```

新增 4 个单测：缺 `clientTotalSteps` / 缺 `motionState` / 缺 `clientTimestamp` 各一个 → 断言 1002 + 错误信息含字段名；显式 0 通过 → 断言 service 被调用。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 handler DTO 里声明 **required 数值字段**（V1 spec 标 required 的 int / float）时，**必须**用 `*T` 指针类型 + handler `if x == nil` 显式校验，**禁止**用值类型 + `binding:"required"`（validator/v10 把 0 视为缺失）也**禁止**裸值类型（缺失与零值无法区分）。
>
> **展开**：
> - 字符串 required 用 `string + binding:"required"` OK（空串 ""能被拦）
> - 布尔 required 同样需要 `*bool`（false 与缺失无法区分）
> - 切片 / map required 用 `binding:"required"` OK（nil ≠ 空切片，binding 能区分）
> - 单测必须**双向**覆盖：① 缺字段 → 1002；② 显式零值 → 通过（防修复时把"零值合法"也一起拒了）
>
> **反例**：
> - `int64 + binding:"required"` → 显式 0 被误拒（首次 0 步同步合法但被拦）
> - 裸 `int64` 无任何校验 → 缺字段静默通过为 0
> - **正例**：`*int64` + handler `if req.X == nil { ... }` + 单测覆盖"缺字段 1002"和"显式 0 通过"

## Lesson 3: MySQL DATE 列必须用 string 全程穿透，禁止任何 time.Time + loc 中转

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/app/http/handler/steps_handler.go:86`（r1）→ 全链路 string（r2）

### 症状（Symptom）

r1 用 `time.ParseInLocation("2006-01-02", req.SyncDate, time.Local)` 修了"DATE 列时区漂移"，但**只对 DSN `loc=Local` 有效**。DSN `loc` 是配置项（`configs/local.yaml` 钦定 `loc=Local`，但 prod 常见 `loc=UTC`），`time.Local` 与 DSN `loc` 错配时 driver 把 `time.Time` 转换成连接 loc 再 format DATE 字面值，仍可能漂日。

```
prod TZ=UTC, DSN loc=UTC, time.Local=UTC（无服务器 TZ override）→ 巧合 OK
prod TZ=Asia/Shanghai (+08), DSN loc=UTC, time.Local=+08
  → ParseInLocation(Local) 得 2026-05-01 00:00 +08
  → driver 转 UTC: 2026-04-30 16:00 UTC
  → format DATE: '2026-04-30'  ← bug，少一天
```

### 根因（Root cause）

DATE 列**没有时区语义**（MySQL DATE 就是 'YYYY-MM-DD' 字面值），但 GORM 用 `time.Time` 映射 DATE 强行引入"两次 loc 转换"（Go time.Time loc → DSN loc → DATE 字符串）。任意一环错配就漂日；想"压锚点"必须显式同步 `time.Local` 与 DSN `loc`，但 DSN 是配置项，跨环境不可控。

**根治方案**：syncDate 全程不走 `time.Time`。client 传 `'YYYY-MM-DD'` 字符串 → handler 校验合法性 → service / repo / DB 全程 string → driver 走"VARCHAR → DATE"隐式转换（MySQL 严格按字面值解释，无 loc 介入）。

### 修复（Fix）

四层联动改 string：

1. **Domain struct**：`StepSyncLog.SyncDate` 从 `time.Time` 改 `string`（gorm tag `column:sync_date;type:date` 不变）
2. **Repo interface**：`FindLatestByUserAndDate / SumAcceptedDeltaByUserAndDate` 的 `syncDate` 参数 `time.Time` → `string`
3. **Service input**：`SyncStepsInput.SyncDate` `time.Time` → `string`；service 内 `slog` 字段直接用 `in.SyncDate`（不再 `.Format`）
4. **Handler**：`PostSyncRequest.SyncDate` 仍 `string`（不变），调 `time.Parse` 仅做**纯字符串合法性校验**（不把返回 time 实例往下传，loc 用 `time.UTC` 也行，无副作用）

```go
// handler 校验合法性但不传 time.Time
func isValidYYYYMMDD(s string) bool {
    _, err := time.Parse("2006-01-02", s)
    return err == nil
}

// service 调用直接传 string
out, err := h.svc.SyncSteps(ctx, service.SyncStepsInput{
    SyncDate: req.SyncDate, // string 直传
    ...
})
```

集成 + 单测全部跟着改 string fixture（约 8 个 case）。Integration helper `latestSyncLogAcceptedDelta` 同步把 ORDER BY 改成 `client_total_steps DESC, id DESC`，与 repo 基线查询同序，断言"基线行"的 accepted_delta_steps 而非"最近 INSERT 行"。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在为 MySQL **DATE / TIME / YEAR**（无时区语义的"日历"列）选 Go 字段类型时，**必须**用 `string` 而非 `time.Time`，整链路（DTO / service / repo / DB）保持 string 透传，**禁止**任何环节做 `time.Time` ⇄ string 转换。
>
> **展开**：
> - `time.Time` 适合 `DATETIME / TIMESTAMP`（这两类 MySQL 列**有时区语义**，driver 的 loc 转换是必要的）
> - DATE 字面值就是 'YYYY-MM-DD'，VARCHAR 直传走 driver 隐式转换，MySQL 严格按字面值解释，**永不漂日**
> - 校验日期合法性（闰年 / 月日范围）用 `time.Parse` OK，但**只看 err**，不要把返回的 `time.Time` 实例往下传
> - 跨层 API（repo interface / service input）的字段类型变更牵动测试 fixture / integration helper / sqlmock SQL 断言；**任何"局部修复"都不算彻底** —— 必须四层（domain / repo / service / handler）+ 测试 fixture 全部统一
>
> **反例**：
> - `SyncDate time.Time` + `time.ParseInLocation(time.Local)`（r1 修复）→ 与 DSN `loc=UTC` 配置错配仍漂日
> - "局部 ParseInLocation 用 UTC 锚点 + 其他层不动" → driver 仍把 UTC time.Time 转 DSN loc 再 format
> - **正例**：全链路 string；driver 走 VARCHAR→DATE 隐式转换；handler `time.Parse` 仅做合法性校验不传 time 实例

## Meta: 本次 review 的宏观教训（可选）

三条 finding 共享一个递进主题：**"局部最小修复"在跨层语义问题上往往不够彻底**。

- L1（基线 bug）：把 ID 时序当业务时序，是"最简单的代理"导致的；正确做法是引入业务单调字段，而不是给 id DESC 加个 hack
- L2（指针字段）：值类型 + binding 是 Go HTTP DTO 的"默认套路"，但碰到"缺失 vs 零值"二义性必须升级到 pointer + 显式 nil
- L3（DATE 时区）：r1 已修过一次（ParseInLocation Local），但只是"压锚点"；codex r2 戳穿"锚点本身依赖配置"才推到根治（全 string 透传）

**未来 Claude 在 review 阶段拿到"看似简单的局部修复"建议时**：
- 先问"这个修复能否被另一个配置项 / 上游字段 / 上游调用顺序破坏"
- 若答案是"是"，**必须**升级修复到根治层级（改类型 / 改 schema / 改契约），而不是叠 hack
- 跨层类型变更虽然 diff 大，但比"靠注释和约定"维护跨层一致性可靠得多
