---
date: 2026-05-16
source_review: "file: /tmp/epic-loop-review-23-1-r2.md (codex review --base, 末尾 ^codex$ 段为真实结论)"
story: 23-1-接口契约最终化
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-16 — 跨接口共享数据源的字段约束必须自洽：catalog 空 URL vs chest-open 非空冻结的矛盾（23-1 r2）

## 背景

Story 23.1（接口契约最终化，纯文档）round 1 已修复 §8.1 catalog 排序 tie-breaker、§8.2 inventory 已拥有实例可见性 + missing-item 降级占位（commit a4ba50e）。round 2 codex review 指出一个 round-1 未触及的契约面：§8.1/§8.2 把 `iconUrl`/`assetUrl` 冻结为"enabled cosmetic 也允许空字符串 `""`"，而 §7.2 `POST /chest/open` 已冻结 `reward.iconUrl`/`reward.assetUrl` **非空** —— 两个接口从**同一批 `is_enabled = 1` 行**取数（catalog 列全量 enabled 行，chest-open 从 enabled 行加权抽奖），却给同源数据定义了互相矛盾的字段约束。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | catalog 允许 enabled cosmetic 空 URL，与 chest-open 冻结的 reward.* 非空契约矛盾 | medium (P2) | architecture / docs | fix | `docs/宠物互动App_V1接口设计.md` (§8.1 :1267-1268 / §8.1 关键约束 :1307 / §8.2 :1361-1362 / §8.2.3 降级占位 :1345 / §8.2 关键约束 / §1 冻结边界 :79) |

## Lesson 1: 跨接口共享同一数据源时，字段约束不能各自为政

- **Severity**: medium (P2)
- **Category**: architecture / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1267-1268`（§8.1 catalog 响应体字段表，连带 §1/§8.2/关键约束 4 处）

### 症状（Symptom）

§8.1 GET /cosmetics/catalog 与 §8.2 GET /cosmetics/inventory 把 `iconUrl`/`assetUrl` 冻结为 `0 ≤ length ≤ 255`「允许空字符串」，理由写的是"catalog 是全量配置可能含未配图 placeholder"。但 §7.2 POST /chest/open 已冻结 `reward.iconUrl`/`reward.assetUrl` 为 `1 ≤ length ≤ 255`「不允许空字符串」。chest-open 的 reward 正是从 catalog 同源的 `is_enabled = 1` 行加权抽出的 —— 一旦 admin 给某 enabled 行配了 `icon_url=''`，catalog 契约说"原样返回空串合法"，chest-open 契约却说"reward 必须非空"。server 实装时被迫二选一：要么违反 chest-open 冻结契约下发空 URL，要么特判一批"永远不能被抽中"的脏配置数据 —— 两条路都破坏冻结契约的可实装性。

### 根因（Root cause）

把字段约束**按单接口的本地视角**定义，没有沿数据流向上溯源到"这批数据还会被哪些接口消费、那些接口对同一字段已经冻结了什么"。catalog 的设计者只想到"配置表可能没配图"，于是放宽成允许空串；却没意识到 chest-open 的 reward 抽奖池 = catalog 的 enabled 行池，是同一份数据的两个出口。共享数据源的两个出口给同一字段定义互斥约束，无论实装怎么写都会违反其中一个冻结契约。差异理由"catalog 是全量配置"听起来自洽，实际是把"DB 列 `DEFAULT ''` 的 DDL 兜底"误当成"业务允许 enabled 行留空"。

### 修复（Fix）

把"enabled 行允许空 URL"收紧为"enabled 正常行必须非空 + 空串仅限 deleted/missing-item 降级路径"，与 §7.2 reward.* 非空冻结对齐。5 处同步改动（保持 §1/§8.1/§8.2 自洽）：

- **§8.1 :1267-1268**：`iconUrl`/`assetUrl` 约束 `0 ≤ length` → `1 ≤ length ≤ 255` 非空；理由改为"与 §7.2 reward.* 一致"，并标注 `DEFAULT ''` 仅 DDL 兜底、seed/admin 写入层必须保证 enabled 行非空，空串仅出现在 §8.2 missing-item 降级路径
- **§8.1 关键约束 :1307**：「允许空字符串契约」→「非空字符串契约」，显式写明消除"catalog 空 URL vs chest-open 非空冻结"跨接口矛盾
- **§8.2 :1361-1362**：正常组 `1 ≤ length` 非空；**仅**配置缺失降级占位组允许 `""`
- **§8.2.3 降级占位 :1345**：标注"这是空 URL 的唯一合法出现路径"，正常组（含 catalog 全量 + inventory 正常组）必须非空
- **§8.2 关键约束**：新增 `iconUrl/assetUrl 非空字符串` 契约 bullet，与 §8.1 :1307 对仗
- **§1 冻结边界 :79**：「允许空字符串契约」→「非空字符串契约（仅 missing-item fallback 可空）」，避免 §1 与 §8.1/§8.2 自相矛盾

注：未反向修改 §7.2 reward.*（本就非空，是被对齐的基准）；§7.2:1215 "reward.* 与 §8.1 字段名/类型一致"仍成立（字段名/类型未变，仅长度下限收紧到与 reward 一致，反而更一致）。空串语义与 round-1 §8.2.3 引入的 missing-item fallback **完全一致**（空串只在 admin 删配置后的已拥有实例降级路径出现），不是 round-1 症状反弹。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **冻结/锚定一个接口字段的取值约束（尤其放宽到允许空值/默认值）** 时，**必须** 先追溯该字段的数据源还被哪些其他接口消费、那些接口对同名同源字段已冻结了什么约束，确保所有出口对同源数据的约束**取交集后非空可实装**。
>
> **展开**：
> - 共享数据源（同一张表 / 同一批 `WHERE` 行）的多个接口出口，对同一字段的约束必须**相容**：若接口 A 放宽为"允许空"、接口 B 冻结为"必须非空"，而 B 的数据是从 A 的同源行派生的，则无论实装怎么写都会违反 B 的冻结契约 —— 这是契约级矛盾，不是措辞分歧
> - DB 列的 `NOT NULL DEFAULT ''` 是 **DDL 兜底**，**不**等于"业务允许该行留空"。enabled / 正常业务行的字段非空性应由 seed 层 + admin 写入层保证，契约层应按"业务有效行非空"冻结，把空值/占位严格限制在已显式定义的降级 fallback 路径（如 deleted/missing-item placeholder）
> - 放宽约束（允许空、允许默认值、允许 null）时，差异理由听起来自洽（"配置表可能没配图"）不代表真的自洽 —— 必须验证下游每个消费方能否接受这个放宽
> - **反例**：在 §8.1 catalog 把 `iconUrl` 冻结为"允许空字符串，因为 catalog 是全量配置可能没配图"，却没检查 §7.2 chest-open 的 reward 是从同一批 enabled 行抽的且已冻结 reward.iconUrl 非空 —— 制造了"server 要么违反 chest-open 冻结要么特判永不可奖励数据"的死结。正确做法：enabled 行强制非空与最严格的下游消费方（chest-open reward）对齐，空串仅留给已定义的 missing-item 降级路径
