---
date: 2026-05-02
source_review: file:/tmp/epic-loop-review-7-1-r3.md (codex review round 3 of Story 7.1)
story: 7-1-接口契约最终化
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-02 — 接口契约 story 必须连同时序图 + 数据库枚举一起锚定，不能只改 V1 接口文档

## 背景

Story 7.1 是 Epic 7 的契约定稿 story，AC1 把"唯一受编辑的文档"限定为 `docs/宠物互动App_V1接口设计.md`，明确"不修改时序图 / 数据库设计 / Go 项目结构"等其他 7 份 docs。r1 / r2 review 通过后，r3 codex review 发现 V1 文档新增的 `clientTimestamp` 必填字段、`source=2 (admin_grant)` 取值在另外两份文档里没有同步，形成两套互相冲突的契约。本轮 fix 把跨文档不一致直接消除，并把"契约 story 的 AC1 范围必须包含跨文档锚定"作为下次 retrospective 的输入。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | clientTimestamp 字段未同步到时序图 syncSteps 调用 | P2 | docs | fix | `docs/宠物互动App_时序图与核心业务流程设计.md:188` |
| 2 | user_step_sync_logs.source 枚举未加 admin_grant | P2 | docs | fix | `docs/宠物互动App_数据库设计.md:765-769` |

## Lesson 1: V1 接口新增必填字段必须同步刷新时序图调用签名

- **Severity**: P2
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_时序图与核心业务流程设计.md:188`

### 症状（Symptom）

V1 接口设计 §6.1 的 `POST /api/v1/steps/sync` 请求体在 r2 已经把 `clientTimestamp` 锚定为必填字段（int64 ms）。但时序图文档 §6.3 的 `Client → API → Service` 调用签名仍然是 `syncSteps(userId, syncDate, clientTotalSteps, motionState)` 4 参数，缺 `clientTimestamp`。Story 7.3 / 8.5 实装阶段，dev 同时读 V1 文档 + 时序图，会看到两份不同的 service 入参列表。

### 根因（Root cause）

Story 7.1 的 AC1 把"编辑范围"切到只动 V1 接口文档一份，但**契约一致性是跨文档的**：V1 接口文档定义客户端可见 schema，时序图文档定义 service 层调用签名，数据库设计文档定义底层 column / enum。三份文档对同一个字段（如 `clientTimestamp`）必须一起描述，否则 implementer / reviewer 读到不同文档会得到不同的"真相"。AC1 的范围红线是从"防止 scope creep"角度写的，但**它把跨文档锚定的必要工作误判成了 scope creep**。

### 修复（Fix）

时序图 §6.3 的 mermaid sequence diagram 里 `API → Service` 调用补一个参数：

```diff
-    API->>Service: syncSteps(userId, syncDate, clientTotalSteps, motionState)
+    API->>Service: syncSteps(userId, syncDate, clientTotalSteps, motionState, clientTimestamp)
```

Story 7.3 实装阶段 service 层签名直接照此对齐。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在做"接口契约定稿 / schema 锚定"类 story 时，**必须**把 V1 接口文档 + 时序图文档 + 数据库设计文档**三份一起列入 AC1 编辑范围**，不能只改其中一份。
>
> **展开**：
> - 任何接口字段（特别是必填项 / enum / 类型）的新增 / 修改，影响面**至少**横跨三处：(1) `docs/宠物互动App_V1接口设计.md` 的 schema 表 + JSON 示例；(2) `docs/宠物互动App_时序图与核心业务流程设计.md` 的 mermaid 调用签名；(3) `docs/宠物互动App_数据库设计.md` 的 column 定义 / enum 取值表
> - 写"契约定稿" story 的 AC 时，编辑范围红线**不能**写成"只改 V1 接口文档"。正确表述："V1 接口文档为主修改方；时序图 + 数据库设计的对应 column / signature **如有差异必须同步刷新**"
> - **反例**：Story 7.1 AC1 line 25 写 "**不**改其他 7 份 docs/宠物互动App_*.md（包括数据库设计 / Go 项目结构 / iOS 项目结构等 —— 它们对应的 schema 已锚定，本 story 只对齐 / 引用，不修改）" —— 这条规则在新增字段（clientTimestamp）+ 扩枚举（source=2 admin_grant）场景下直接漏修，导致 r3 codex 抓到两条 P2 跨文档不一致

## Lesson 2: 数据库 enum 必须穷举所有写入路径，扩枚举不是"实装时的事"

- **Severity**: P2
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_数据库设计.md:765-769`

### 症状（Symptom）

V1 文档 §6.1 服务端逻辑明文说"`source=1` 代表客户端正常上报；dev grant 走 source=2（admin_grant），见数据库设计 §6.6 + Story 7.5"。但数据库设计 §6.6 user_step_sync_logs.source 枚举只列了 `1 = healthkit`，没有 `2 = admin_grant`。Story 7.5（dev grant）实装时按 V1 文档写入 `source=2`，会跑到一个 DB 字典表里没定义的取值，迁移 / 测试 / 审计脚本都会断。

### 根因（Root cause）

数据库 enum 字典通常只在新枚举值"第一次实装"时被想起来扩展，但实际上**只要有任何文档说会写入某个值，字典就必须先扩**。本场景下 dev grant 是 Story 7.5 才实装的功能，但 Story 7.1 在 V1 文档里**已经引用了** `source=2 (admin_grant)` 作为 sync_logs 写入的取值之一，此时数据库设计文档就必须同步加。否则在 7.1 → 7.5 之间任何读这两份文档的人都会读到矛盾结论。

### 修复（Fix）

数据库设计 §6.6 扩枚举：

```diff
 ### 6.6 user_step_sync_logs.source

 ```text
-1 = healthkit
+1 = healthkit       # 客户端正常上报（POST /api/v1/steps/sync, 见 V1接口设计 §6.1）
+2 = admin_grant     # dev / 运营手动发放（POST /api/v1/dev/grant-steps, 见 Story 7.5）
 ```
```

注释里写明"哪个接口写哪个 source 值"，避免下次有人读到孤立的枚举数字猜不出来源。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 V1 接口文档**引用**任何数据库 enum 取值时（如 "source=2 见数据库设计 §6.6"），**必须**当场**反查**该 enum 在数据库设计文档里**已经存在**；不存在就**先扩字典**再写引用。
>
> **展开**：
> - V1 接口文档里写 "field=N（语义见数据库设计 §X.Y）" → grep 数据库设计文档 §X.Y → 该值必须在枚举表里 → 不在就**当前 story 内补齐**，不能延后到 enum 真正使用的 story
> - 数据库 enum 扩枚举的成本**极低**（一行 markdown），但**不扩**的成本**极高**（migration 脚本 / 单测 fixture / dashboard 字典表都得返工）
> - **反例**：Story 7.1 在 V1 文档 §6.1 服务端逻辑写 "`source=1` 代表客户端正常上报；dev grant 走 source=2（admin_grant），见数据库设计 §6.6"，但数据库设计 §6.6 当时还只有 `1 = healthkit` —— 跨文档锚点对不上，r3 codex 当场抓出

---

## Meta: 本次 review 的宏观教训

Story 7.1 的 AC1 范围红线写得过紧（"只改 V1 接口文档"），是 r3 review 抓出两条跨文档不一致的根因。**契约定稿 story 的本质是"把分散在多份 docs 里的字段语义对齐到一处"，不是"只编辑一份 docs"**。下次写类似 story 的 AC1 时，编辑范围应改写为：

> "V1 接口文档为主修改方；任何字段引用到的时序图 mermaid 签名 / 数据库设计 enum 字典 / 数据库设计 column 定义，**如发现与新锚定的字段 schema 不一致，必须在本 story 内同步刷新**。其他 7 份文档不在本 story 范围 = 仅指**结构性 / 模块层** 改动（不引入新模块、不重排目录），**字段层一致性是契约 story 的核心交付物，不算 scope creep**"。

此 meta 教训作为 Epic 7 retrospective 的输入，触发对未来 contract-first story（11.1 / 14.1 / 17.1 等）的 AC1 模板复盘。
