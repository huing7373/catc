---
date: 2026-05-14
source_review: file:/tmp/epic-loop-review-20-1-r2.md (codex round 2)
story: 20-1-接口契约最终化
commit: dfd9fbe
lesson_count: 4
---

# Review Lessons — 2026-05-14 — 契约 finalize story 必须做跨文档一致性 & SQL PK / 列名 / 字段类型字面级校验（20-1 r2）

## 背景

Story 20.1（接口契约最终化）r2 由 codex 复审 §7.2 `POST /api/v1/chest/open` 冻结后的契约稳定性。本轮指出 4 条问题：1 条跨文档"语义矛盾"（其实是分阶段契约未显式标注成预期）、1 条 SQL PK 字面错误（`WHERE id = ?` 而表 PK 是 `user_id`）、1 条 SQL SELECT 列缺失（缺 `drop_weight` 列），1 条字段类型不一致（`int` / `2^31-1` 与同义字段的 `int64` / `BIGINT` 冲突）。本质：**契约 finalize story 的可信度取决于"实装者照抄就能跑"——任何字面级错误都是 blocking。**

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | reward instance creation 跨文档矛盾（§7.2 placeholder vs §14.1 / DB §8.3 创建实例） | high | docs | fix | `docs/宠物互动App_V1接口设计.md` §7.2.4f / §14.1 + `docs/宠物互动App_数据库设计.md` §8.3 |
| 2 | user_step_accounts UPDATE SQL `WHERE id = ?` 错（表 PK 是 user_id） | high | docs | fix | `docs/宠物互动App_V1接口设计.md:966` |
| 3 | SELECT 子句缺 drop_weight，按 drop_weight 加权无权重输入 | medium | docs | fix | `docs/宠物互动App_V1接口设计.md:968` |
| 4 | stepAccount 字段类型 `int` / `2^31-1` 与 §6.2 + BIGINT 冲突 | medium | docs | fix | `docs/宠物互动App_V1接口设计.md:1006-1008` |

## Lesson 1: 跨文档"渐进式契约"必须在两端都显式标注分阶段差异

- **Severity**: high
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §7.2.4f / §14.1 + `docs/宠物互动App_数据库设计.md` §8.3

### 症状（Symptom）

§7.2 节点 7 钦定"开箱不创建 `user_cosmetic_items` 实例，`reward.userCosmeticItemId` 返回占位 `"0"`"；同 repo 中 §14.1「开箱事务」+ DB §8.3 仍写"插入一条 `user_cosmetic_items`"。reviewer（不带"节点 7 vs 节点 8 分阶段"的上下文）读到时看到"两份权威契约互相矛盾，server / iOS 实装者各自挑一份照抄"。

### 根因（Root cause）

"渐进式契约 / 分节点切片"是本项目特有的契约组织方式 —— §7.2 写**当前节点切片**（节点 7 阶段实装路径），§14.1 / DB §8.3 写**最终契约**（节点 8 稳态）。但**没有任何一端显式说明"两段语义是分阶段的，不是矛盾"**，所以外部 reviewer 无法判断哪份是"权威"。契约 finalize story 的可信度建立在**单一权威**之上，任何看似矛盾的两段都必须 disambiguate。

### 修复（Fix）

三个位置都加显式分阶段说明：

1. §7.2.4f 步骤后追加 `> 跨文档分阶段契约说明（重要）` 块，明确「§14.1 / DB §8.3 是最终契约 / §7.2 是节点 7 切片 / 节点 8 切回 §14.1 语义」
2. §14.1 末尾追加 `> 节点 7 vs 节点 8 阶段差异` 引用，指回 §7.2.4f
3. DB §8.3 末尾追加同样的引用，指回 V1接口设计 §7.2.4f

实装者读到 §14.1 / §8.3 时一定能看到节点 7 切片的存在，不会孤立地按"创建实例"实装。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写契约 finalize story 涉及"节点 X 当前切片"vs"节点 Y 最终契约"差异时**，**必须** **在两端都显式标注"渐进式契约"差异并相互交叉引用，禁止只在一端写而留另一端"看似矛盾"**。
>
> **展开**：
> - 节点 N 钦定与"最终契约"语义不同时，**在最终契约段也加 callout**「本节为最终契约；节点 N 阶段差异详见 §X.Y」—— 不能假设读者会从节点 N 段倒查回来
> - callout 必须包含：① 本段是最终契约 / 当前切片的明示；② 切换时机（哪个 Story / Epic 完成后切回）；③ 反向链接到另一端
> - **反例（本次踩坑形态）**：只在 §7.2.4f 写"节点 7 占位 / 节点 8 切回 Story 23.5"，但 §14.1 仍按最终契约写"发放装扮实例"，**没有任何反向引用** —— reviewer 读到 §14.1 时无法识别"这是预期的分阶段差异"，只能判定为"两段矛盾"
> - **反例（更隐蔽形态）**：跨文档（接口设计 + 数据库设计）的渐进式差异 —— 只在接口设计端标注，DB 端没标注，跨文档矛盾形态更隐蔽（reviewer 切换文档时丢失上下文）

## Lesson 2: 写 SQL 模板时必须现场对照表 schema 验证 PK / 列名

- **Severity**: high
- **Category**: docs
- **位置**: `docs/宠物互动App_V1接口设计.md:966`

### 症状（Symptom）

§7.2.4d 把扣步数 UPDATE 写成 `UPDATE user_step_accounts ... WHERE id = ? AND version = ?`，但 `user_step_accounts` 表 schema（DB §5.4）的主键是 `user_id BIGINT UNSIGNED PRIMARY KEY` —— **没有 `id` 列**。照抄会导致 SQL 解析失败（unknown column 'id'）。

### 根因（Root cause）

写 SQL 模板时凭"惯性"假定每张表都有 `id` PK 列（`users` / `pets` / `user_cosmetic_items` / `user_chests` 等多数表确实是 `id BIGINT AUTO_INCREMENT`），但 `user_step_accounts` 是「per-user 单行账户」语义，PK 直接用 `user_id`（一对一）。这种"per-user 单例表"的 PK 形态在本项目并不罕见 —— 还有 `pets` 实际 PK 也是 user_id 关联（虽然 `pets.id` 也存在），所以写跨表 UPDATE 时**必须现场对照 DB schema**，不能凭印象。

### 修复（Fix）

```diff
- UPDATE user_step_accounts SET ... WHERE id = ? AND version = ?
+ UPDATE user_step_accounts SET ... WHERE user_id = ? AND version = ?
```

并在备注里加「`user_step_accounts` 主键是 `user_id` 不是 `id` —— 见数据库设计 §5.4」，让后续修改者也能看到提醒。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写接口契约 / 实装文档中的 SQL 模板**（UPDATE / DELETE / SELECT FOR UPDATE 任意类型）时，**必须** **现场 Grep / Read 数据库设计文档的对应 `CREATE TABLE` 段，逐列对照 SQL 引用的列名 + PK 形态**。
>
> **展开**：
> - 任何 `WHERE id = ?` 写法前 → 先确认目标表的 PK 是不是 `id` 列；很多业务表（账户 / 设备绑定 / per-user 单例）的 PK 形态是业务键（`user_id` / `device_id` / `room_id`）而非自增 `id`
> - 任何 SELECT 列后会被业务逻辑用的（如 `按 drop_weight 加权抽取`）→ 确认 SELECT 子句**包含**该列
> - 写完 SQL 模板 → **以 reviewer 视角再读一遍**：如果实装者照抄这段 SQL 到 Go 代码里，能不能跑通？跑不通的话契约 finalize story 失去意义
> - **反例（本次踩坑形态）**：写 `UPDATE user_step_accounts ... WHERE id = ? AND version = ?` 时没去对照 DB §5.4 的 `CREATE TABLE user_step_accounts (user_id BIGINT UNSIGNED PRIMARY KEY, ...)`，凭"每张表都有 id PK"的印象写
> - **反例（更微妙形态）**：SELECT 抽数据用作"加权抽取算法"输入时，忘了把权重列本身写到 SELECT 子句里 —— SELECT 只列了"展示用字段"（name / slot / rarity），漏了"算法输入字段"（drop_weight）；这种"列 vs 用途"匹配漏洞需要在写完 SQL 后**用业务逻辑回读一遍**

## Lesson 3: 同义字段在多接口 / 多文档中必须保持字段类型 + 范围一致

- **Severity**: medium
- **Category**: docs
- **位置**: `docs/宠物互动App_V1接口设计.md:1006-1008`

### 症状（Symptom）

§7.2 响应体 `data.stepAccount.{totalSteps, availableSteps, consumedSteps}` 字段表写成 `number (int)` + 上限 `2^31 - 1`；但 §6.2 `GET /steps/account` 同名字段写 `number (int64)`、DB §5.4 `user_step_accounts.{total_steps, available_steps, consumed_steps}` 是 `BIGINT UNSIGNED`（可达 2^63 - 1）。说明里还写"与 §6.2 同义 + 数据库同义"—— 字段类型不一致 **+ 自称同义**，是契约矛盾。

### 根因（Root cause）

写 §7.2 字段表时，模仿了同文件里其他 step / count 字段的"`number (int)` + `2^31 - 1`"模板（这些字段确实是 int 范围），没回头校对**该字段在 §6.2 / DB schema 里的类型**。"同义"声明加重了矛盾权重——客户端 codegen 工具读到"同义"会假定两端可互换，但实际类型不同会导致 Codable / Decoder 在大值场景下溢出。

### 修复（Fix）

```diff
- | `data.stepAccount.totalSteps`     | number (int) | 必填 | `0 ≤ value ≤ 2^31 - 1` | ... |
+ | `data.stepAccount.totalSteps`     | number (int64) | 必填 | `0 ≤ value ≤ 2^63 - 1` | ... 数据库 user_step_accounts.total_steps (BIGINT UNSIGNED) 同义 ... |
```

同理修 `availableSteps` / `consumedSteps`。注释里点明"BIGINT UNSIGNED"让实装者明确底层存储。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **响应体字段表自称"与 §X / 数据库 Y 同义"时**，**必须** **同步把该字段的"类型 / 范围 / 单位"三项都拉去匹配 §X 和 Y，不允许字段名 + 自称同义 + 类型/范围不同的组合**。
>
> **展开**：
> - 字段表加"同义"标注 → 顺手 Grep 同名字段在 repo 其他位置的表，对齐 type / range / unit
> - DB 层 `BIGINT` / `BIGINT UNSIGNED` → API 层必须是 `int64`（Go int64 / Swift Int64 / TS string），**不能**写 `int` / `int32`（`int` 在 32 位平台是 32-bit）
> - 字段如果可能超过 2^53（JS Number 安全整数上限）→ 需要钦定"字符串化主键"路径（本项目对 BIGINT PK 已用此约定）；纯计数 / 步数字段一般不会超 2^53，可保留 `number (int64)` 不字符串化，但**必须明示**
> - **反例（本次踩坑形态）**：字段表写 `number (int)` + `2^31 - 1` + 注释"与 §X int64 同义"—— 字段名匹配但类型 / 范围矛盾；codegen 工具或实装者会选择某一份，丢失契约权威

## Lesson 4: 契约 finalize story 的本质是"实装者照抄能跑"——必须做字面级回读

- **Severity**: high
- **Category**: docs
- **分诊**: meta（本次 4 条 finding 的共同教训）

### 症状（Symptom）

本轮 r2 review 出的 4 条 finding 中，3 条（lesson 2 SQL PK / lesson 3 字段类型 / 部分 lesson 1 跨文档矛盾）属于"字面级错误"——实装者照抄会直接跑不通或与其他文档冲突。Story 20.1 的产出是"接口契约最终化"，验收点本就是"实装者能照抄"，但 r1 review pass 的版本仍漏了这些字面错误。

### 根因（Root cause）

写"接口契约最终化"story 时，专注点在**新增段落 / 新加冻结声明 / 新增 callout** 这些"显性产出"上，**没有以"实装者照抄"视角逐字段 / 逐 SQL 回读**。r1 codex review 抓的是"幂等 / 限频 / 中间件层级"等架构层语义问题，r2 codex review 切换到"字面级语法 / 列名 / 类型"维度才暴露这批问题——说明 LLM reviewer 也是分轮次抓不同层级，第一轮通过不代表 finalize。

### 修复（Fix）

不修代码，写规则。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写"契约 finalize" / "API 冻结" / "实装锚定文档"类 story 完成前**，**必须** **以"实装者视角"逐字段 / 逐 SQL 回读一遍：每个 SQL 模板能跑吗？每个字段类型对得上 DB schema 吗？每个'同义/一致'声明都校对过吗？**
>
> **展开**：
> - 契约 finalize story 的 done 标准不是"段落写完了"，是"实装者照抄就能跑"
> - 写完 SQL 模板 → 模拟执行：表名 / 列名 / PK 形态 / 索引利用 全部对照 schema
> - 写完字段表 → 模拟 codegen：每个字段都能映射到目标语言的合法类型；自称"同义"的字段必须在所有引用点 type 一致
> - 写完"跨文档 callout" → 模拟外部 reviewer 阅读：两端段落能否相互引用 + disambiguate？没有相互引用就是看似矛盾
> - LLM review 单轮 pass 不代表 finalize → **故意请 reviewer 切换"视角"再来一轮**（架构层 → 字面层 → 跨文档层 → 边界场景层），每一轮抓不同维度的问题
> - **反例（本次踩坑形态）**：r1 LLM review 只抓"幂等 / 限频架构层"问题，pass 后 Claude 视为 finalize 就绪；r2 切换字面层视角才暴露 4 条 blocking 问题。如果 r1 → done 一气呵成发版，server / iOS 实装阶段会直接卡在 SQL 跑不通

---

## Meta: 本次 review 的宏观教训

**Story 20.1 r1 → r2 暴露的不是单点 bug，而是"契约 finalize story 的 done 阈值偏低"**：r1 pass 后已被默认"finalize 完成"，但 r2 用字面层视角抓出 4 条 blocking。教训：

1. **契约类 story 必须多轮 LLM review 切换视角抓不同维度** —— 单轮 pass 不能视为 finalize
2. **"跨文档分阶段契约"是高风险模式** —— 任何渐进式契约（节点 N 当前切片 vs 节点 N+k 最终态）必须**在两端都显式 callout 并交叉引用**，否则外部 reviewer 会判为"矛盾"
3. **SQL 模板必须字面级对照 DB schema** —— 不能凭"每张表都有 id PK"的印象写
4. **"自称同义"是契约层强声明** —— 任何"同义/一致"标注必须同步 type / range / unit，否则反而加重矛盾

这条 meta 适合在未来"契约 finalize" / "API 冻结" / "数据契约升级" 类 story 启动时**主动调用**作为 checklist。
