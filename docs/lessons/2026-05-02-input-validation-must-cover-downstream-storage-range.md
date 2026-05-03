---
date: 2026-05-02
source_review: codex review r4 of Story 7-3 (POST /steps/sync 累计差值入账 service)
story: 7-3-post-steps-sync-接口-累计差值入账-service
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-02 — 输入校验边界必须考虑下游存储真实约束（time.Parse 接受不代表 MySQL DATE 接受）

## 背景

Story 7.3 review r4 指出 `POST /api/v1/steps/sync` 的 `syncDate` 校验函数 `isValidYYYYMMDD` 只用 `time.Parse("2006-01-02", s)` 校验"Go 能解析"，但 MySQL `DATE` 列只接受 `1000-01-01 ~ 9999-12-31`。Go 接受的 `"0999-12-31"` 这类 pre-1000 字符串会跳过 handler 的 1002 参数校验直接走到 repo `WHERE sync_date = ?` / `INSERT … VALUES (?, …)`，被 mysql driver 拒后 wrap 成 ErrServiceBusy → client 看到 1009（"服务繁忙"）而非预期的 1002（"参数错误"）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | isValidYYYYMMDD 漏 MySQL DATE range 校验（pre-1000 / post-9999 日期混入） | medium (P2) | error-handling / input-validation | fix | `server/internal/app/http/handler/steps_handler.go:171-194` |

## Lesson 1: 输入校验边界必须以"下游存储真实约束"而非"语言解析能力"为准

- **Severity**: medium (P2)
- **Category**: error-handling / input-validation
- **分诊**: fix
- **位置**: `server/internal/app/http/handler/steps_handler.go:171-194`

### 症状（Symptom）

`POST /api/v1/steps/sync` 接收 `syncDate = "0999-12-31"`：

1. handler `len(req.SyncDate) != 10` 通过（"0999-12-31" 正好 10 字符）
2. handler `isValidYYYYMMDD` 旧实装 `time.Parse("2006-01-02", "0999-12-31")` 返回合法 `time.Time` + nil err → 通过
3. service / repo 直传 string 到 `WHERE sync_date = ?` / `INSERT INTO user_step_sync_logs (sync_date, …)`
4. mysql driver 检测到 DATE 字面值 < `1000-01-01` → driver 返 error
5. repo wrap → `apperror.ErrServiceBusy`
6. client 收到 envelope `{"code": 1009, "message": "服务繁忙"}`，**期望**是 `{"code": 1002, "message": "syncDate 格式不符 …"}`

### 根因（Root cause）

写 handler 时把"输入合法"的标准默认成"Go 类型系统能表示"，没有去看**下游存储的真实物理约束**。MySQL `DATE` 列的范围 `1000-01-01 ~ 9999-12-31` 是 [MySQL 文档](https://dev.mysql.com/doc/refman/8.0/en/datetime.html) 钦定的，Go `time.Time` 能表达更广（`time.Time` 可表示公元前到极远未来），两者范围**不**重合。

更本质的反模式：handler 校验函数复用了"借助标准库 parser 做合法性校验"的便宜写法（`_, err := time.Parse(layout, s); return err == nil`），但**没问自己**"parser 接受的子集是不是等于业务能接受的子集"。这是一类"以工具能做什么、而不是以业务/存储能做什么为校验依据"的思维漏洞。

### 修复（Fix）

`isValidYYYYMMDD` 在 `time.Parse` 通过后追加年份范围校验：

```go
func isValidYYYYMMDD(s string) bool {
    parsed, err := time.Parse("2006-01-02", s)
    if err != nil {
        return false
    }
    // MySQL DATE 物理范围 1000-01-01 ~ 9999-12-31
    year := parsed.Year()
    if year < 1000 || year > 9999 {
        return false
    }
    return true
}
```

设计取舍：
- **不**加业务上界（"≤ 当前年" / "≤ 2099"）—— client 时钟漂移 / 跨日 race 偶发"未来日期"，业务侧合理性应由 service 层（rate-limit / 跨日去重）兜底，handler 只做物理范围拦截
- **不**加业务下界（"≥ 1970 Unix epoch" / "≥ 2020 业务上线年"）—— 同样属于业务语义，不属于 handler 入口校验
- 保守只用 MySQL DATE 物理范围；上界 9999 同样写一遍是防御未来 layout 调整（理论上 `time.Parse("2006-01-02")` 已通过 4 字符长度限制隐式拒 5 位年份，但显式上界更安全）

新增 4 个 boundary 单测覆盖：
- `TestStepsHandler_PostSync_SyncDatePre1000_Returns1002` — `"0999-12-31"` → 1002
- `TestStepsHandler_PostSync_SyncDateFiveDigitYear_Returns1002` — `"10000-01-01"` → 1002（被 len 检查拦掉，同时 time.Parse 也会拒）
- `TestStepsHandler_PostSync_SyncDateMinBoundary_Allowed` — `"1000-01-01"` → 通过（边界 OK）
- `TestStepsHandler_PostSync_SyncDateMaxBoundary_Allowed` — `"9999-12-31"` → 通过（边界 OK）

### 预防规则（Rule for future Claude）⚡

> **一句话**：handler 写**输入校验函数**且字段最终落盘时，**必须**把"下游存储的物理约束"（MySQL DATE/INT/VARCHAR 长度 / DATETIME 范围 / DECIMAL 精度等）作为校验下界，不能只校验"Go / Python / 语言运行时能不能解析这个值"。
>
> **展开**：
> - 写"格式校验"函数时，**先列出该字段最终会落到哪个 DB 列**，查该列类型的物理范围（MySQL DATE = `[1000-01-01, 9999-12-31]`，TINYINT = `[-128, 127]` 或 `[0, 255]` UNSIGNED，VARCHAR(N) 严格 N 字符上限……），把这些范围作为 handler 校验的 hard floor / hard ceiling
> - **业务范围**（如"下单金额 ≤ 100 万" / "syncDate ≈ 今天附近"）放 service 层；**物理范围**（DB 列接受集）放 handler 层。两层校验不重复也不缺失
> - 反例 1：`time.Parse("2006-01-02", s); err == nil` —— Go 接受 pre-1000 + 公元前，MySQL DATE 不接受
> - 反例 2：`json.Unmarshal` int64 字段不加上界 —— Go int64 max ≈ 9.2e18，MySQL INT max ≈ 2.1e9 / BIGINT UNSIGNED max ≈ 1.8e19，类型不匹配会 silent overflow / driver error
> - 反例 3：`utf8.ValidString(s)` —— Go 接受 4-byte UTF-8（如 emoji），MySQL `utf8` charset（**非** `utf8mb4`）不接受 → 落盘报 `Incorrect string value`
> - **触发判定**：任何 handler 校验函数命名 `isValidXxx` / `validateXxx`，且 `xxx` 字段会经 service → repo → DB 链路落盘 —— 必须查目标 DB 列物理约束，不能假设"语言能 parse 就行"
> - **测试纪律**：boundary case 必须包含"DB 上界 / DB 下界 / 上界 + 1 / 下界 - 1"四点；不能只测"中间合法值 + 明显格式错"（后者 review 也容易漏）

---

## Meta: 错误码契约的方向性

Handler 入口校验和 service 业务校验都可能产生"参数错误"，但映射的 envelope code 不同：
- handler 入口（格式 / 物理范围 / required 字段）→ `1002 ErrInvalidParam`
- service / repo 因下游约束被拒 → 默认 wrap 成 `1009 ErrServiceBusy`（"服务繁忙"）

**1002 表示"client 自己能改 request 修复"，1009 表示"server 内部异常"**。如果一个本质是"client 输入超范围"的错误被映射成 1009，client 会把它当成 server 故障 → retry 风暴 / 用户看到"服务繁忙"误以为后端挂了。

**原则**：凡是"不修改请求内容就一定还会失败"的错误（参数格式错 / 范围越界 / required 缺失 / 枚举值非法），都必须在 handler 入口拦截并返 1002，绝不让它穿透到 service / repo 让 driver / DB 报错后再 fallback 成 1009。这是 V1 §3 错误码语义的根本约束。
