---
date: 2026-05-16
source_review: "file: /tmp/epic-loop-review-23-1-r3.md (codex review --base, 末尾 ^codex$ 段为真实结论)"
story: 23-1-接口契约最终化
commit: 36fe5fa
lesson_count: 2
---

# Review Lessons — 2026-05-16 — 冻结的列表契约必须一次性枚举全部边界 case（两级全序排序 + config 三态完整矩阵）

## 背景

Story 23.1（节点 8 装扮查询接口契约最终化）在 `docs/宠物互动App_V1接口设计.md` §8.2 锚定 `GET /cosmetics/inventory`。round 1 已加「配置缺失（无 row）保留组 + 降级占位 + log error」防静默丢失；round 2 已修「enabled 行 URL 非空 vs chest-open 非空冻结」跨接口矛盾。round 3 codex review 又发现 §8.2 同根因下两个 round 1/2 **未覆盖的遗留缺口**：(1) `groups[]` / `instances[]` 排序未钉死 → Story 24 渲染会跨请求重排抖动；(2) round 1 fallback 只覆盖"无 row"，未定义"row 存在但 `is_enabled=0`"中间态 → Story 23.4 可能 join `is_enabled=1` 又隐藏已拥有项。两条均为真实 under-spec（非症状反弹），按"从契约定义根因一次性消除全部歧义"原则全量修复。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | §8.2 inventory `groups[]` / `instances[]` 未定义确定性排序 | P2 (medium) | docs / architecture | fix | `docs/宠物互动App_V1接口设计.md` §8.2 |
| 2 | §8.2 config 缺失态只覆盖"无 row"，未定义 `is_enabled=0` 中间态 | P2 (medium) | docs / architecture | fix | `docs/宠物互动App_V1接口设计.md` §8.2 |

## Lesson 1: 冻结的列表契约必须钉死两级（含嵌套数组）确定性全序排序

- **Severity**: medium (P2)
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §8.2（服务端逻辑步骤 5 + 关键约束 + §1 冻结边界）

### 症状（Symptom）

§8.2 冻结了 inventory 的 payload 形状（`groups[]` + 每组 `instances[]`），但从未定义两级数组的排序规则。Story 24（Story 24.1 ~ 24.5）每次打开 Wardrobe Tab 都重新 GET inventory 渲染聚合 grid + 实例列表（Story 24.2 不缓存），若排序交给 SQL / JOIN 迭代顺序，相同库存数据会在不同请求间重排，client UI 抖动。round 1 已给 §8.1 catalog 加了 `rarity ASC, slot ASC, id ASC` 全序，但 §8.2 inventory 是**两级嵌套结构**，round 1 没顺带覆盖 —— 单级列表的排序教训没有自动外推到嵌套列表。

### 根因（Root cause）

"列表契约必须全序确定"这条规则在 round 1 只被应用到**单级列表**（catalog `items[]`）。inventory 是 `groups[]` 里再嵌 `instances[]` 的**两级结构**，需要"每一级都独立钉死全序"。思维漏洞：把"已经给 catalog 加了 tie-breaker"误当作"列表抖动问题已系统性解决"，没有对契约里**所有**返回数组（含嵌套子数组）逐一审视是否全序。MySQL 不带 `ORDER BY` 时返回顺序不保证稳定，嵌套数组每一层都有这个问题，少钉任何一层都会抖动。

### 修复（Fix）

§8.2 服务端逻辑新增步骤 5「确定性两级全序排序（契约必需，非可选优化）」：

- `groups[]`：`ORDER BY rarity ASC, slot ASC, cosmeticItemId ASC`（与 §8.1 catalog `rarity ASC, slot ASC, id ASC` 同根因风格对齐，client 复用排序心智模型；`cosmeticItemId ASC` 决定性 tie-breaker；态 C 占位组用 `rarity=1, slot=99` 归位仍全序确定）
- `instances[]`：`ORDER BY userCosmeticItemId ASC`（`user_cosmetic_items.id` §5.9 `BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT` 全局唯一，单值即决定性全序）
- 两级 tie-breaker 均到唯一键级别保证**全序唯一**（不存在排序相等的两元素）
- 同步：新增「确定性两级全序排序」关键约束 bullet + §1 冻结边界声明把两级排序纳入冻结范围 + 步骤 7 事务边界注明"无论 JOIN 还是分步查，步骤 5 排序是契约必需，不依赖 DB 天然顺序" + story 23-1 文件 §8.2 关键约束同步

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **冻结任何返回列表 / 嵌套列表的 API 契约** 时，**必须**对契约里**每一层**数组（含 `parent[].child[]` 的子数组）**独立钉死确定性全序排序，tie-breaker 必须落到该层唯一键级别**。
>
> **展开**：
> - "已经给某个列表加了排序"**不**代表"列表抖动问题已系统性解决" —— 嵌套结构的每一层都是独立的抖动源，逐层审视，不靠外推。
> - tie-breaker 必须到**唯一键**（主键 / 唯一索引列），不能停在"业务排序键"（如 rarity/slot）—— 业务键往往多行同值，缺唯一 tie-breaker 时 DB 顺序不保证稳定。
> - 嵌套子数组的排序键优先选该子表的**主键**（如 `user_cosmetic_items.id`），单列即全序无需再补键。
> - 顶层数组排序风格尽量与同语义的**兄弟接口**对齐（inventory `groups[]` 与 catalog `items[]` 都用 `rarity, slot, <唯一键>`），让 client 复用心智模型。
> - **反例**：契约只写"返回 `groups[]`，每组含 `instances[]`"但不写任何 `ORDER BY`，或只钉了 `groups[]` 排序却漏了 `instances[]`，或 tie-breaker 停在 `slot ASC`（同 slot 多行仍可抖动）。这些都会让实装者被迫挑一个 client 无法依赖的任意顺序，UI 跨请求抖动。

## Lesson 2: 引用"可被运维改动的外部配置"的契约，必须穷举配置的全部中间态（不能只覆盖"存在/不存在"两态，漏掉"存在但 disabled"）

- **Severity**: medium (P2)
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §8.2（服务端逻辑步骤 3 config 三态矩阵 + 关键约束 + §1 冻结边界）

### 症状（Symptom）

round 1 给 §8.2 加的 fallback 只覆盖了"`cosmetic_item_id` 在 `cosmetic_items` 表**无匹配 row**"（admin 物理删 row）→ 保留组 + 降级占位 + log error。但 `cosmetic_items` 有 `is_enabled TINYINT`，存在第三态："row 存在但 `is_enabled = 0`"（admin 下架但没删）。§8.1 catalog / §7.2 chest 都显式 `WHERE is_enabled = 1` 过滤，已发放的 `user_cosmetic_items` 完全可能引用一个 disabled 配置。契约没定义这一态 → Story 23.4 实装者很可能"自然地"在关联 `cosmetic_items` 时加 `is_enabled = 1`（与 catalog/chest 一致的直觉），结果把 disabled 配置下用户**已拥有**的项静默隐藏，正好违背 round 1 才锁定的"不得静默丢失已拥有数据"契约。

### 根因（Root cause）

round 1 把配置状态建模成了**二态**（存在 / 不存在），但底层 `cosmetic_items` 实际是**三态**（enabled 存在 / disabled 存在 / 不存在）。思维漏洞：处理"引用的外部配置可能缺失"时只想到"配置被删"，忽略了配置有 `is_enabled` 字段、admin 更常用的操作是"禁用"而非"物理删除"。当一个接口要 join 一张"可被运维独立增删改的配置表"时，配置的状态空间 = 行存在性 × 所有状态布尔字段，必须**全矩阵**枚举，否则实装者会按其它接口的过滤直觉（catalog/chest 都 `is_enabled=1`）默认补一个会导致数据丢失的 WHERE。

### 修复（Fix）

§8.2 服务端逻辑步骤 3 从"二态 fallback"升级为 **config 三态完整矩阵**（表格形式，标注"互斥且穷尽，禁止只处理其中两态"）：

- **态 A enabled**（row 存在 + `is_enabled=1`）→ 出现在 groups[] + row 真实 metadata + 不 log
- **态 B disabled-but-exists**（row 存在 + `is_enabled=0`）→ **仍出现 + row 真实 metadata（与态 A 一致，不 placeholder、不 log）**；理由：inventory 是"已拥有清单"语义，row 物理存在故 metadata 数据可得，用真实值无信息降级；态 B 的 row 受 §8.1 / Story 20.3 非空约束故 URL 仍非空，与 round 2 锁定的"enabled 行 URL 非空"契约不冲突
- **态 C missing-no-row**（无 row）→ 仍出现 + round 1 降级占位（`name="未知装扮"`/`slot=99`/`rarity=1`/空 URL）+ log error；理由：物理删 row 数据**不可得**才被迫降级，与态 B 数据可得有本质区别
- 显式写入"**Story 23.4 关联 `cosmetic_items` 禁止加 `is_enabled = 1` 过滤**"
- 同步：字段表 `iconUrl`/`assetUrl` 约束改为"态 A+态 B 非空、仅态 C 空串" + 错误码注覆盖三态 + 关键约束「已拥有实例可见性」升级为「+ config 三态完整矩阵」+ URL 非空契约把范围明确扩展到态 B + §1 冻结边界声明纳入三态矩阵 + story 23-1 文件 §8.2 关键约束 + Change Log 同步

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **定义"引用一张可被运维独立增删改的配置表"的接口契约** 时，**必须**把被引用配置的状态空间按 `行存在性 × 每个状态布尔字段` 展开成**完整矩阵**逐态定义，**禁止**只覆盖"存在 / 不存在"二态而漏掉"存在但 disabled / soft-deleted / 下架"等中间态。
>
> **展开**：
> - 配置表有 `is_enabled` / `status` / `deleted_at` 之类状态字段时，"配置缺失"**不止**"行不存在"一种 —— "行存在但被禁用"是更常见的运维动作，必须单独定义。
> - 对每个中间态明确三件事：**该数据是否仍返回**（涉及"已拥有/历史数据不得静默丢失"时通常必须返回）、**用真实 metadata 还是 placeholder**（数据**可得**就用真实值，不可得才降级 —— placeholder 是信息损失，能不降级就不降级）、**是否 log error**（运维常规动作如"下架"不告警，数据治理事故如"物理删仍被引用的行"才 log error）。
> - 用**表格 + "互斥且穷尽，禁止只处理其中两态"**的措辞写进契约，逼实装者三态全覆盖。
> - 当兄弟接口（catalog/chest）对同一张表用了 `is_enabled = 1` 过滤时，**必须**在本接口契约里显式写"禁止加 `is_enabled = 1` 过滤"，否则实装者会按兄弟接口直觉默认补一个导致数据丢失的 WHERE。
> - **反例**：fallback 只写"配置在表里查不到时降级占位"，没说"查到了但 `is_enabled=0`"怎么办；或对 disabled 也用 placeholder（数据其实可得，无谓降级）；或对 admin 常规下架也 log error（噪声告警）；或没禁止实装层加 `is_enabled=1` 过滤，导致 Story 23.4 join 时静默隐藏已拥有项。

---

## Meta: 本次 review 的宏观教训

round 1 → round 2 → round 3 三轮 [P2] 都指向 §8.2 同一根因簇：「冻结的列表契约必须全序确定」+「不得对已拥有数据静默丢失」+「跨接口/跨态约束必须自洽」。每轮只修了字面位置而没有**一次性枚举该接口的全部边界 case**，导致下一轮 review 又啃出同源缺口（round 1 修"无 row 缺失"→ round 3 发现"disabled 中间态"；round 1 修"catalog 单级排序"→ round 3 发现"inventory 两级排序"）—— 这正是 review 轮次线性膨胀的失败模式。

**宏观规则**：定稿（finalize / freeze）一个契约 section 时，**不要逐条修 review 点的字面位置**，而要从根因出发**一次性把该 section 涉及的所有边界 case 全矩阵枚举钉死**：列表 → 每一层数组的全序唯一排序；引用外部可变配置 → 配置状态全矩阵 + 每态的"返回性 / metadata 来源 / 日志级别"；跨接口同名字段 → 同义性 + 各节点取值矩阵。修完后让下游实装者读了**不可能**写出抖动排序 / 静默丢数据 / 多态处理不一致的实现，才算真正 finalize。否则每轮 review 都只是把同一个根因的下一个症状暴露出来。
