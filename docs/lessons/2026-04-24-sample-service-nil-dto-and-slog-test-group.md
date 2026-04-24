---
date: 2026-04-24
source_review: manual code review on Story 1.5 implementation (2 × P2 findings)
story: 1-5-测试基础设施搭建
commit: 4519274
lesson_count: 2
---

# Review Lessons — 2026-04-24 — Sample 模板的 nil DTO 兜底 & slog 测试 fixture 的 WithGroup 语义

## 背景

Story 1.5 交付的测试基础设施包含两件**被未来反复复制**的工件：
1. `internal/service/sample/` — Epic 4+ 所有 service 的实装模板
2. `internal/pkg/testing/slogtest/` — Epic 4+ 所有结构化日志断言的 fixture

两件工件都有"**被复制 N 次才暴露问题**"的风险剖面 —— review 这次提前一轮把两处潜在陷阱捕获了。两条 finding 都是 P2，都分诊为 **fix**，因为：
- 对于 sample：若带 bug 流出，会把 panic path 复制进 Epic 4/7/10/20/26/32 每一条真 service
- 对于 slogtest：若语义漂移，业务 log 断言测试与生产 logger 行为不一致，Story 1.8（AppError `error_code`）落地时会静默绕过

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Sample GetValue 对 repo 返回 `(nil, nil)` 不做兜底，会 panic | medium (P2) | architecture / error-handling | fix | `server/internal/service/sample/service.go` |
| 2 | slogtest.Handler.WithGroup 只存 group 名，Handle 不应用 prefix，与 JSONHandler 行为分叉 | medium (P2) | testing | fix | `server/internal/pkg/testing/slogtest/handler.go` |

---

## Lesson 1: Sample 模板的 nil DTO 兜底

- **Severity**: medium (P2)
- **Category**: architecture / error-handling
- **分诊**: fix
- **位置**: `server/internal/service/sample/service.go:66-70`

### 症状（Symptom）

`SampleService.GetValue` 在 repo 调用后只检查 `err != nil`，没检查 `dto == nil`。若 `SampleRepo` 实装返回 `(nil, nil)`（"正常的不存在"常见翻译法），下一行 `dto.Value` 直接解引用 nil → panic。

### 根因（Root cause）

复制模板（template）型代码的设计盲点 —— 写模板时脑中想的是"happy path 怎么教 dev 看懂"，把 error handling 简化到最小以突出教学主线。但模板的**实际用途**不是"教 dev 读"，而是"dev 复制后微调字段名"。哪一个 check 在模板里缺席，就会在所有下游真 service 里一起缺席。模板 = 多处坑的**最大风险点**，对它的 error handling 苛刻度必须**等于**真 service，不能更松。

此外 Go 生态里 `(nil, nil)` 约定极常见：
- `database/sql` 没直接返回 `(nil, nil)`，但用户代码把 `sql.ErrNoRows` 翻译成 `(nil, nil)` 是 idiomatic（GORM 的 `errors.Is(err, gorm.ErrRecordNotFound)` → 返回 nil 对象）
- `redis.Nil` 同理
- MongoDB "no document" 同理

这是**跨数据源的共通模式**，模板不可能靠"让 dev 记得修"来防御 —— 必须在模板里内置。

### 修复（Fix）

**before**:
```go
dto, err := s.repo.FindByID(ctx, id)
if err != nil {
    return 0, err
}
return dto.Value, nil
```

**after**:
```go
dto, err := s.repo.FindByID(ctx, id)
if err != nil {
    return 0, err
}
if dto == nil {
    return 0, ErrSampleNotFound
}
return dto.Value, nil
```

同时补一条 table-driven test case：repo 返回 `(nil, nil)` → 断言 service 返回 `ErrSampleNotFound`。service.go 顶部注释把"`(nil, nil)` 约定"列入方法合同的第三条。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写**模板 / sample / 参考实现**类代码时，**必须**把 error handling 完整度拉到"真 service 级别"，不能以"教学突出主线"为由省略。

> **展开**：
> - 任何返回 `(*T, error)` 的 service 方法调用，检查 `err != nil` 之后**必须**紧跟 `if ptr == nil { return ..., <sentinel-not-found> }`。`(nil, nil)` 是 Go 生态数据层（sql / redis / mongo）"正常的不存在"的 idiomatic 翻译，service 层必须兜住
> - 如果**有意**允许 nil 对象进入下游（少见），必须在方法注释第一行说明，让复制模板的人能看见并主动删除兜底
> - 模板文件里的兜底**同时**要配对 test case —— 不 test 的兜底下游会觉得"可删"，test 驱动保留
> - **反例**：方法注释里写"repo 返回 dto → 返回 dto.Value" 但未说"repo 返回 nil 怎么办" —— 下游 dev 复制时假定永远非 nil，真 repo 一旦返回 nil 就炸。正确注释把 `(nil, nil)` 情形与 `err != nil` 并列为 case

## Lesson 2: slog 测试 fixture 必须保留 WithGroup 命名空间

- **Severity**: medium (P2)
- **Category**: testing
- **分诊**: fix
- **位置**: `server/internal/pkg/testing/slogtest/handler.go:95-104` (WithGroup + Handle)

### 症状（Symptom）

`slogtest.Handler.WithGroup(name)` 只把 name 存进字段，**never** 在 `Handle` 里应用。结果：
- 被测代码调 `logger.WithGroup("error").Info("e", slog.Int("code", 1001))`
- slogtest 捕获 attr 为裸 `code` = 1001
- 生产 `slog.JSONHandler` 实际输出 `{"error":{"code":1001}}`（扁平化后的 key 是 `error.code`）
- 测试用 `AttrValue(rec, "code")` **能**查到，用 `AttrValue(rec, "error.code")` **查不到** —— 和生产**正好相反**

`AttrValue` 断言能通过不代表生产 JSON 结构正确。

### 根因（Root cause）

"MVP 够用"原则误用。handler.go 原文甚至**明示**了"MVP 实现不深入 group 渲染"作为已知局限 —— 但这种**已知但不修**的局限会：
1. 被读文档的 dev 忽略（读 API 不读 package doc）
2. 被 sample 测试自然回避（首版 smoke test 不用 WithGroup）
3. 在 Story 1.8（AppError 落地，用 `slog.Group("error", ...)` 是常见风格）第一次真正触发时，测试通过但生产 JSON 不对

"已知局限"落在公共 fixture 上的代价 = N 个下游 story 每次都要警惕这个局限；修它的代价 = 30 行代码。ROI 非常清晰：**公共 fixture 的语义不允许偏离 stdlib 对应组件**。这是 fixture 的本质工作（代替真 handler 做断言）—— 语义偏离 = 工作失效。

### 修复（Fix）

Handler 增加 `groupPrefix string` 字段，`prefixedAttrs` 存"key 已带前缀"的 snapshot。三个核心操作的语义：

- `WithGroup(name)` → 返回新 Handler，`groupPrefix` dot-join name（空 name no-op，与 stdlib 一致）
- `WithAttrs(as)` → snapshot 当下的 prefix 应用到每个 attr key，存进 `prefixedAttrs`
- `Handle(r)` → 组合 `prefixedAttrs` + record.Attrs（用 currentPrefix 包装 key），形成最终 flat-keyed record

断言侧：`AttrValue(rec, "error.code")` 用 dot-joined 完整 key 查（与生产 JSONHandler 扁平化后的 key 等价）；**不**做"裸 key 兜底"—— 那会静默掩盖 group 语义漂移，正是这条 lesson 要防的错误类型。

测试补 `TestHandler_WithGroupNamespacing`：单层 group / 嵌套 group / WithAttrs 在 group 前后的 namespace 差异 / 空 name no-op，4 个子断言覆盖常见用法。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写**测试 fixture / mock / in-memory 替身**时，**必须**让语义与被替换的 stdlib / 生产组件等价，不允许以"MVP 够用"为由留语义漂移。

> **展开**：
> - Fixture 的本质工作 = "代替真实组件参与测试断言"；语义偏离 = 工作失效。语义保真比实现简单**优先**，哪怕多写 30 行
> - 如果真的必须留局限（比如依赖深栈反射不划算实装），**必须**在公共 API 注释第一行 + README **同时**标红，让调用方一定能在"写到相关用法"之前看到
> - 公共 fixture 的每个接口方法都要有**至少一条测试**验证该方法的语义与对应 stdlib 组件一致；不测的接口 = 语义未定义 = 下游没法依赖
> - **反例**：`WithGroup` 接口存在但 `Handle` 不处理 prefix —— 接口形式满足 `slog.Handler`，但被测代码用 group 后 fixture 捕获结构与生产 JSONHandler 不一致，测试蒙对蒙错完全看运气。正确做法是要么**实装完整语义**，要么**让 WithGroup panic** 迫使调用方面对不支持

---

## Meta: 本次 review 的宏观教训

两条 finding 触及同一类思维漏洞：**"公共 artifact 的局限会被 N 倍放大"**。

- Lesson 1 的 sample 是 service 复制模板 → 任何缺陷 × N 个 Epic
- Lesson 2 的 slogtest 是测试断言 fixture → 任何语义漂移 × N 个 story 的 log 断言

这类代码的质量标准不能按"自己这一次能跑通"评估，必须按"假设 10 个下游 story 分别复用，最差的那个能不能跑通"评估。写公共 artifact 时：

1. **测试完整度**：不是"能跑通"就够，要能覆盖**未来下游最可能用到的用法**（sample 的错误路径 / fixture 的每个 slog.Handler 接口方法）
2. **注释完整度**：方法合同注释要把"容易被下游省略的 edge case"作为 first-class case 列出（Lesson 1 的 `(nil, nil)`）
3. **语义等价承诺**：fixture 必须与被替换组件语义等价；若有偏离，放弃"安静局限"，直接 panic 或文档顶部标红（Lesson 2）

这些不是代码风格，是**公共 artifact 的质量门槛**。下次写任何 `internal/pkg/` / `internal/service/<sample|template>/` 时提前过一遍这 3 条 checklist。
