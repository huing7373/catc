---
date: 2026-05-14
source_review: codex review round 8 — /tmp/epic-loop-review-20-1-r8.md
story: 20-1-接口契约最终化
commit: b5a6c45
lesson_count: 1
---

# Review Lessons — 2026-05-14 — 设计文档跨章节 summary 必须随 canonical 章节同步迭代

## 背景

Story 20.1（开箱契约最终化）经过 r4 → r5 → r6 → r7 多轮 review 不断迭代幂等设计：
canonical 章节 §5.16（chest_open_idempotency_records 表设计）与 V1 §7.2（开箱时序）每轮
都同步更新到最新钦定。但 §7.1 高优先级 UNIQUE 索引清单里那一段 summary 是 r5 时
写的，从 r5 → r6 → r7 一路被各轮 review 漏掉 —— r5 文案（`id = id` + "只有一个
并发请求能拿到 `affected_rows = 1` 进入业务事务"）一直保留到 r8 才被 codex 抓出。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | §7.1 chest_open_idempotency_records 索引说明 stale 文案与 §5.16/§7.2 钦定不一致 | medium | docs | fix | `docs/宠物互动App_数据库设计.md:924` |

## Lesson 1: 设计文档跨章节 summary 是 stale 重灾区，迭代 canonical 时必须 grep 全文

- **Severity**: medium
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_数据库设计.md:924`（§7.1 高优先级 UNIQUE 索引清单 → chest_open_idempotency_records 条目）

### 症状（Symptom）

§5.16（表 schema canonical）和 V1 §7.2（开箱时序 canonical）已经迭代到 r7：

- SQL：`INSERT ... ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`
- 并发语义：unique-key X-lock 阻塞排队，首事务结束后第二事务按 commit/rollback
  分支走 `affected_rows = 0` 短路 或 `affected_rows = 1` 走全流程

但同一文档的 §7.1（"UNIQUE 索引清单"）summary 段还停留在 r5 文案：

- SQL：`INSERT ... ON DUPLICATE KEY UPDATE id = id` ← 旧 placeholder UPDATE
- 并发语义：「只有一个并发请求能拿到 `affected_rows = 1` 进入业务事务」← 完全
  错（r5 之前 client 同 key 重试在第二事务也能拿 1，因为首事务 rollback 后行
  消失；r7 简化二态机后这条更不成立）

后果：未来 Claude / 新人读 §7.1 找契约依据时会被 stale summary 误导，可能把
旧 placeholder SQL 写进 Go 实装；review 报告引用 §7.1 行号时也会把 r8 已废
弃的论据当作论据。

### 根因（Root cause）

**跨章节 summary 是 stale 重灾区的结构性原因**：

1. **canonical 章节有"激情迭代"信号**（review 报告里被反复点名 → 每轮必读必改）
2. **summary 章节没有信号**（review 报告聚焦 canonical，summary 不在 critical
   path，作者改完 canonical 就 commit）
3. **r5 → r6 → r7 迭代时只搜索 canonical 关键词**（如 "post-rollback failed"
   "Redis sentinel"），不搜索 summary 里用的同义表达（如 "只有一个并发请求"
   "id = id"）
4. **同一份决策在文档里有多处复述**（§5.16 / §7.1 / §8.3 / Story spec）—— 改
   canonical 时如果不 grep 全文，summary 一定漏

迭代 N 轮 review 后，所有 canonical 章节都对齐，但散落在文档各处的 summary
段会形成一个"考古地层"—— 每层 summary 对应一轮 review 之前的旧钦定。

### 修复（Fix）

把 `docs/宠物互动App_数据库设计.md:924` 的 §7.1 summary 改为与 §5.16 line 756
（canonical 索引说明）完全对齐的表述：

before（r5 stale）：
```
兼任原子声明依据：V1接口设计 §7.2 步骤 3 用 `INSERT ... ON DUPLICATE KEY UPDATE id = id`
借此 UNIQUE 做 single-statement 原子 claim；同 `(user_id, idempotency_key)` 的并发请求
只有一个能拿到 `affected_rows = 1` 进入业务事务（r5 review 锁定）。
```

after（与 §5.16 / §7.2 对齐 + 标 review 轮次链）：
```
兼任原子声明依据 + 并发阻塞排队依据：V1接口设计 §7.2 步骤 3a 在业务事务内首条语句用
`INSERT ... ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)` 借此 UNIQUE 做 single-statement
原子 claim；同 `(user_id, idempotency_key)` 的并发请求被 InnoDB unique-key X-lock 阻塞
排队，首个事务结束（commit / rollback）后其他事务再继续 —— commit → 行已存在 →
`affected_rows = 0` 短路；rollback → 行已消失 → `affected_rows = 1` 走全流程
（r5 review 锁定 / r6 review 修订 / r7 review 简化；canonical 文案见 §5.16 索引说明）。
```

附加 grep 验证：
- `rg "ON DUPLICATE KEY UPDATE id = id" docs/` → 0 命中
- `rg "只有一个能拿|只有一个并发请求" docs/宠物互动App_数据库设计.md` → 0 命中

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **迭代设计文档的 canonical 章节（如 §5.x 表设计 /
> §7.x 时序）时**，**必须 grep 全文搜索同一决策的所有跨章节复述**（summary /
> 索引清单 / 关键约束段 / Story spec 引用），**禁止**只改 canonical 章节就
> commit。
>
> **展开**：
> - 每轮 review 修订 canonical 后，先列出本轮**改动涉及的关键词清单**（SQL 片段、
>   状态机字段、约束名、决策一句话摘要），再用这些关键词全仓库 grep；命中的
>   每一处都要判断是否需要同步修订
> - 关键词清单要同时包含**新文案的关键词**（验证 canonical 已改）和**旧文案的
>   关键词**（找出未同步的 summary 残留）。例：r8 这次要同时 grep `id = LAST_INSERT_ID`
>   （新）和 `id = id`（旧）；grep `unique-key X-lock` 和 `只有一个能拿`
> - 设计文档里出现"详见 §X.Y"链接的章节，要**特别警惕**：被链接到的 canonical
>   一旦迭代，链接源段落极易 stale；按"链接源 → canonical"反向走一遍
> - 同一文档内多处复述同一决策时，**显式标 canonical 锚点**（如本次修复加了
>   "canonical 文案见 §5.16 索引说明"），让未来读者立刻知道哪段是源、哪段是
>   摘抄；摘抄段简短即可，不要复制完整 SQL / 完整论证
> - **反例**：
>   - r6 review 时改了 §5.16 的状态机字段说明 + §7.2 步骤，没 grep 全文 →
>     §7.1 summary 段保留 r5 "只有一个能拿 affected_rows = 1" 的旧文案
>   - r7 review 时移除了 `failed` 状态 + 删除 post-rollback 补偿描述，仍然没
>     grep `failed` 关键词 → §7.1 summary 又错过一轮
>   - r8 codex review 全文细扫才发现 r5 → r7 三轮迭代里 §7.1 一直没动 ——
>     文档迭代速度远快于"全文 grep 习惯"养成的速度
> - **正例**：每轮 review fix commit message 里附一段 "本次同步 grep 验证：
>   `<旧关键词>` 0 命中 / `<新关键词>` 命中 N 处全部正确"，强制建立 grep
>   动作 → commit 的因果链
