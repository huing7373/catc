---
date: 2026-05-16
source_review: "file: /tmp/epic-loop-review-23-1-r1.md (codex review, Story 23.1 接口契约最终化 round 1)"
story: 23-1-接口契约最终化
commit: a4ba50e
lesson_count: 2
---

# Review Lessons — 2026-05-16 — 冻结契约必须全序确定 & 不得对已拥有数据静默丢失

## 背景

Story 23.1（接口契约最终化）是纯文档定稿 story —— 把 §8.1 `GET /cosmetics/catalog` /
§8.2 `GET /cosmetics/inventory` 两个节点 8 装扮查询接口的 schema 锚定并**冻结**（自
2026-05-16 起进入 §1 契约冻结状态，Epic 23/24 实装者据此 1:1 落地）。codex review
round 1 指出：契约本身冻结了两个会导致下游错误实装的语义 —— catalog 排序对现有 seed
数据非确定、inventory 在 admin 删配置后会让用户已拥有实例静默消失。两条均 [P2]，均为
契约语义缺陷（非措辞问题），按根因优先在契约定义层修复。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | catalog 冻结排序缺 `id ASC` tie-breaker，同 (rarity,slot) 多行致 MySQL 顺序抖动 | medium (P2) | architecture | fix | `docs/宠物互动App_V1接口设计.md` §8.1 / §1；`_bmad-output/implementation-artifacts/23-1-接口契约最终化.md` AC3/AC4 镜像 |
| 2 | inventory 对配置缺失组 skip+warn = admin 删配置后用户已拥有实例可见数据静默丢失 | medium (P2) | architecture | fix | `docs/宠物互动App_V1接口设计.md` §8.2 / 错误码注 / 关键约束；story AC3/AC4/Completion Notes 镜像 |

## Lesson 1: 冻结一个声明"client 可依赖"的排序契约前，排序键必须构成全序（total order）

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §8.1 服务端逻辑步骤 2（约 :1250）+ 关键约束（约 :1306）+ §1 冻结边界（约 :79）

### 症状（Symptom）

契约冻结 `ORDER BY rarity ASC, slot ASC` 并明文写"client 可依赖该顺序做 UI 渲染"，
但同一份 schema 下的 seed（`server/migrations/0012_seed_cosmetic_items.up.sql`）必然
存在多行共享同一 `(rarity, slot)`（如 `hat_yellow`/`hat_red` 同为 rarity=1,slot=1；
`gloves_white`/`gloves_brown` 同为 rarity=1,slot=2 —— 这是 §1 AR18 数量约束 common≥8
的直接后果）。SQL 标准下 ORDER BY 未覆盖的列其相对顺序**不保证**，MySQL 可跨请求对
这些行返回不同顺序 → Epic 24 会针对一个"数据没变也会抖"的 UI 顺序构建。原契约还写了
句"理论可加 `id ASC` 但本 story 不强制 —— client 不应依赖组内顺序"，与同段"client
可依赖该顺序"自相矛盾。

### 根因（Root cause）

把"指定了 ORDER BY"误当成"顺序确定"。ORDER BY 只在排序键能区分所有行时才产生确定
顺序；当排序键存在并列（ties）且数据保证会有并列时，剩余顺序由实现 / 执行计划 / 索引
/ 行存储位置决定，是非确定的。冻结契约时只照抄了 epics.md AC 的"rarity ASC + slot
ASC"字面，没有用本 schema 的 seed 数据去验证该排序是否真的是全序 —— 而本 story 的
AR18 数量约束恰恰保证了并列必然存在。"理论可加但不强制"这种措辞是危险的：契约里任何
"client 可依赖 X"的声明，X 必须是确定的，不能留"理论可加"的不确定尾巴。

### 修复（Fix）

契约层补 `id ASC` 为决定性 tie-breaker，使排序成为全序确定（id 是主键，全表唯一 →
三级排序后无并列）：

- §8.1 服务端逻辑步骤 2 SQL 改为 `ORDER BY rarity ASC, slot ASC, id ASC`，并说明
  `id ASC` 是 AR18"同 (rarity,slot) 必有多行"前提下保证全序的**必需**补充（非可选）。
- §8.1 关键约束：删除自相矛盾的"理论可加但不强制 / client 不应依赖组内顺序"，改为
  "全序确定，client 可依赖**整个列表**顺序；移除 `id ASC` 或改任一排序键视为契约变更"。
- §1 冻结边界 bullet 同步：`catalog 排序 rarity ASC + slot ASC + id ASC（全序确定）`。
- story 文件 AC3/AC4/Completion Notes/Task 依赖说明镜像同步（AC5 跨文档一致性）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **冻结 / 锚定一个声明"client 可依赖顺序"的 ORDER BY
> 契约**时，**必须** **验证排序键在该 schema 的真实 seed/数据约束下构成全序（无并列），
> 否则补主键 `id ASC`（或其他唯一键）作为决定性 tie-breaker**。
>
> **展开**：
> - "指定了 ORDER BY" ≠ "顺序确定"。只有当排序键能唯一区分所有可能行时才是全序。
> - 冻结契约前用本 schema 的 seed / 数量约束（如本例 AR18 common≥8）反推："是否存在
>   两行排序键完全相同？"只要可能存在并列且契约声明 client 可依赖顺序 → 必须加唯一
>   tie-breaker。
> - 契约文本里**禁止**出现"理论可加 X 但本 story 不强制"这类把不确定性留给下游的措辞 ——
>   要么 X 是契约的一部分（写死），要么明确声明该维度不保证（client 不得依赖）。二者
>   不能在同一段同时出现。
> - **反例**：契约写 `ORDER BY rarity ASC, slot ASC` + "client 可依赖此顺序渲染 UI"
>   + "组内顺序 DB 未指定 tie-breaker、理论可加 id ASC 但不强制"。三句并存 = 既让
>   client 依赖又承认顺序不确定，下游必踩抖动。

## Lesson 2: 已拥有的用户资产数据，绝不能因受支持的配置删除而从读接口静默消失

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §8.2 服务端逻辑步骤 3（约 :1343）+ 错误码注（约 :1418）+ 关键约束（新增"已拥有实例可见性"条）

### 症状（Symptom）

§8.2 inventory 契约写"配置不存在的 `cosmetic_item_id`（理论不该有）→ skip 该组 +
log warning"。但 §1 **显式允许** admin 增删 `cosmetic_items` 行（"admin 后台增删
cosmetic_items 行不视为契约变更"）。两条契约组合的后果：admin 删 / disable 一个仍有
用户持有的 cosmetic 配置后，这些用户在 `user_cosmetic_items` 里**真实拥有**的实例会
从 inventory 响应里凭空消失，且只有一行 warning 日志 —— 一个受支持的运维动作被契约
变成了用户可见数据丢失。

### 根因（Root cause）

把"配置不存在"一律当成"不该发生的脏数据"，用 skip 兜底。但没有交叉核对 §1 的冻结
边界声明 —— §1 把 admin 删 cosmetic_items 列为**合法且受支持**的动作。于是"配置缺失"
不是异常脏数据，而是受支持运维动作的正常后果。skip 脏数据的兜底逻辑套用到"受支持
动作产生的合法状态"上，就把数据丢失合法化了。根因是写容错分支时只考虑了"防脏数据
炸 client"，没考虑"这个分支在受支持操作下会被正常触发，且触发时代价是用户资产消失"。
实例的存在性应只取决于 `user_cosmetic_items`（用户真实拥有），与配置表是否还有对应
行解耦 —— 配置是展示元信息来源，不是实例存在性的判据。

### 修复（Fix）

契约层改 skip 语义为"保留可见性 + 降级展示 + 升级日志级别"：

- §8.2 步骤 3：配置缺失组**不 skip**，保留该组，`count`/`instances[]` 照常来自
  `user_cosmetic_items`（与配置是否存在无关）；元信息用降级占位值填充：
  `name="未知装扮"` / `slot=99(other)` / `rarity=1(common)` / `iconUrl=""` /
  `assetUrl=""` —— 全部落在既有枚举值域 + 空字符串契约内，**不**扩展任何 schema。
- 日志从 warning 升级为 **error**（admin 删了仍有用户持有的配置 = 需运维介入的数据
  治理事件，应触发告警）；仍**不**报 client 错误码（单组配置缺失不让只读 inventory
  整体失败）。
- 新增 §8.2 关键约束"已拥有实例可见性"条：实例存在性/数量/id 只取决于
  `user_cosmetic_items`，与配置存在性解耦；"配置缺失则 skip"或"配置缺失则整体报错"
  均视为契约变更。
- 错误码注 + story AC3/AC4/Completion Notes 镜像同步。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **为读接口设计"关联配置/字典行缺失"的兜底分支**时，
> **必须** **先核对该缺失是否由系统受支持的操作（如 §1 允许的 admin 删配置）正常
> 产生；若是，禁止 skip / 隐藏用户已拥有的资产数据，应保留可见性（降级占位元信息）
> + 升级日志级别，绝不静默丢失**。
>
> **展开**：
> - "skip + log warning" 只对"不该发生的脏数据"成立。下笔前必须问：这个分支会被
>   哪些**受支持**的操作触发？查冻结边界 / 权限设计 / admin 能力清单。
> - 用户资产（拥有的道具/实例/余额）的**存在性**应只由其归属表（`user_cosmetic_items`
>   等）决定，与展示用的配置/字典表是否还有对应行**解耦**。配置缺失只影响"怎么展示"，
>   不应影响"是否展示"。
> - 降级展示用既有枚举值域内的占位值（如 other/common/空串），不要为兜底新增 schema。
> - 日志级别要匹配事件严重性：受支持操作产生的需治理状态 → error（触发告警），不是
>   warning（容易被淹没）。
> - **反例**：契约同时写"§1 允许 admin 增删配置行"+"配置不存在的实例 skip 该组 +
>   log warning"。两句并存 = 一个合法运维动作被契约变成用户资产静默蒸发，且只有一行
>   warning，运维和用户都无感知。

---

## Meta: 本次 review 的宏观教训

两条 finding 指向同一思维漏洞：**冻结/锚定契约时只照抄了上游 AC 的字面，没有用本
设计包内的其它已冻结约束（§1 freeze 边界、AR18 seed 数量约束、admin 能力声明）去
反向压力测试这份契约在真实数据/受支持操作下的行为**。纯文档 story 没有编译器/测试
兜底，契约的每一条"client 可依赖 X""异常则 skip Y"都必须主动拿同设计包的其它条款
做交叉验证（"X 在本 seed 下真的确定吗？""触发 skip 的 Y 会被哪个受支持动作正常
产生吗？"）。契约文档的正确性判据不是"自洽读起来通顺"，而是"任一下游实装者按它
1:1 落地后，在所有受支持操作 + 真实 seed 下都不产生抖动/数据丢失"。
