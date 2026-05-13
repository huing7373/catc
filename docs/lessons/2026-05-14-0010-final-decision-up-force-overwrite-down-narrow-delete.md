---
date: 2026-05-14
source_review: codex round 3 review of Story 17.3 (file: /tmp/epic-loop-review-17-3-r3.md)
story: 17-3-emoji_configs-seed
commit: <pending>
lesson_count: 2
supersedes_partial: 2026-05-14-down-must-undo-up-invariant-over-admin-data.md (注释级补充，决策方向一致；本 lesson 在 r2 决策上**进一步 lock down** up 路径语义)
final_decision_lock: true
---

> 🔒 **FINAL DECISION on 0010 up/down semantics (post r3 review)**
>
> 0010_seed_emoji_configs 经过 r1 / r2 / r3 三轮 codex review 反复打架，本 lesson 是
> **最终决断**。**不要**在 r4+ 因为同一类 finding 再次重开决策（除非引入 r3 没有的全新 context）：
>
> - **up**: `INSERT ... ON DUPLICATE KEY UPDATE` 强制覆盖 name / asset_url / sort_order / is_enabled 4 字段
> - **down**: `DELETE FROM emoji_configs WHERE code IN ('wave', 'love', 'laugh', 'cry')`（narrow，与 r2 一致）
> - **核心语义**: wave / love / laugh / cry 这 4 个 code **由 0010 完全占用 / 强制覆盖**；admin / 运维**禁止**在这 4 个 code 上 customize；新增 emoji 走新 migration 0011+
> - **invariant 保证**: Story 17.4 / 17.5 / 18.1 依赖的"4 个 enabled emoji 配置正确"invariant **100% 强保证**，不依赖任何 admin 自律
>
> 历史 r1（`2026-05-14-insert-ignore-symmetric-down-and-test.md`）+ r2（`2026-05-14-down-must-undo-up-invariant-over-admin-data.md`）lesson 仍然是有价值的历史推演脉络，不删；但**最终落地以 r3 为准**。

# Review Lessons — 2026-05-14 — 0010 emoji_configs seed 最终决断：up 强制覆盖 + down narrow DELETE

## 背景

Story 17.3 落地 `0010_seed_emoji_configs.up/down.sql`，经过 r1 / r2 三轮 review 反复打架后已定稿为：

- **up**: `INSERT IGNORE`（容忍预存行）
- **down**: `DELETE FROM emoji_configs WHERE code IN (...)`（narrow DELETE）

r3 review 抓出**剩余的核心 invariant 漏洞**：r2 决策下 `INSERT IGNORE` 让 admin 预存的"坏行"（is_enabled=0 / asset_url='' / sort_order 乱序）幸存 → Story 17.4 (GET /emojis) / 17.5 (WebSocket emoji event) / 18.1 (emoji 链路 e2e 测试) 依赖的"4 个 enabled emoji 配置正确"invariant **无法保证**。

r3 同时**重提了 r1 否决过的"down 不应删 admin 数据"finding**。

本轮 fix 做最终决断：

- finding #1（up 漏洞）→ **fix**：把 up 改成 `INSERT ... ON DUPLICATE KEY UPDATE` 强制覆盖 4 字段
- finding #2（down 重提）→ **wontfix**：与 r2 决策直接冲突，且 finding #1 让 admin customization 在 up 时就会被覆盖 → admin 数据保留在 0010 owned codes 上**无论如何无法存活**，down DELETE 是 up 强制覆盖的对称延续

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | up INSERT IGNORE 让预存坏行幸存 → 改 INSERT ... ON DUPLICATE KEY UPDATE 强制覆盖 4 字段 | high (P1) | architecture / migration-invariant | fix | `server/migrations/0010_seed_emoji_configs.up.sql` |
| 2 | down DELETE 删 admin 数据 → 建议保留预存（**重提 r1 已被 r2 否决的方案**） | high (P1) | architecture / migration-invariant | wontfix | `server/migrations/0010_seed_emoji_configs.down.sql` |

## Lesson 1: seed migration 的 up 路径必须**强制覆盖**自己所有 owned codes 的字段（防止预存坏行幸存导致下游 invariant 失效）

- **Severity**: high (P1)
- **Category**: architecture / migration-invariant
- **分诊**: fix
- **位置**: `server/migrations/0010_seed_emoji_configs.up.sql`

### 症状（Symptom）

r2 决策下 0010.up 用 `INSERT IGNORE INTO emoji_configs (...) VALUES (4 行 seed)`：

```sql
INSERT IGNORE INTO emoji_configs (code, name, asset_url, sort_order, is_enabled) VALUES
    ('wave', '挥手', 'https://placehold.co/64x64?text=Wave', 1, 1),
    ...
```

当 DB 在 0010 跑之前已经存在同 code 的"坏行"时（例如 `('wave', '挥手-旧', '', 99, 0)`），`INSERT IGNORE` 撞 uk_code 静默丢弃 → 坏行幸存 → 0010 跑完后 emoji_configs 表里 wave 行的 is_enabled=0 / asset_url='' / sort_order=99。

下游 Story 17.4 (GET /emojis where is_enabled=1) 拿不到 4 个 emoji（wave 缺失）→ 17.5 (WebSocket emoji event) 验证 code 合法性时 wave 不在 enabled set 里 → 18.1 e2e 测试挂。

这种预存坏行的来源：
- **r2 注释自己列出的 3 个场景**：(a) golang-migrate force(9) 重跑 (b) dev/admin 手工 mysql import 0010 (c) migrate down 到 0 后再 up
- **admin 误操作**：违反 r2 lesson 里的"约定"在 0010 owned codes 上 INSERT/UPDATE
- **历史 schema 残留**：测试环境 / staging 上次回滚未清干净

r2 决策依赖**"admin 自律 + 约定 + 新 migration 兜底"软约束**保证 invariant；但软约束不等于**强保证**。

### 根因（Root cause）

r2 决策时把"seed 是初始默认值，不应覆盖式重置 admin 调整"当成 design intent，所以选了 `INSERT IGNORE` 而非 `ON DUPLICATE KEY UPDATE`。但这个 design intent 与 Story 17.4/17.5/18.1 依赖的**"4 个 enabled emoji 配置 100% 正确"invariant** 互相冲突 —— 后者要求 seed 必须能"治愈"任何已存在的坏行。

r2 lesson 文档里其实已经清楚地阐述了"migration framework invariant 是 hard constraint；admin 数据保留是 soft constraint"，但只把这条铁律应用到了 down 路径（down 必须真正 undo up），没**对称地**应用到 up 路径（up 必须真正 establish 钦定状态）。r3 finding #1 抓的就是这个对称性盲点。

更深的根因：r2 选 `INSERT IGNORE` 时把"INSERT IGNORE 的容忍语义" + "admin 数据保留约定"耦合在了一起，没意识到只要**显式声明** "wave/love/laugh/cry 这 4 个 code 由 0010 完全占用、admin 不应 customize"，就可以**同时**：

- 用 `ON DUPLICATE KEY UPDATE` 强制治愈所有路径下的预存坏行（→ invariant 100% 强保证）
- 让 admin 数据保留约束**自然消失**（这 4 个 code 上的 admin 数据本就不应该存在 / 不应该被尊重）

### 修复（Fix）

把 0010.up.sql 从 `INSERT IGNORE` 改成 `INSERT ... ON DUPLICATE KEY UPDATE`，强制覆盖 name / asset_url / sort_order / is_enabled 4 字段：

```sql
-- after（r3 定稿）
INSERT INTO emoji_configs (code, name, asset_url, sort_order, is_enabled) VALUES
    ('wave',  '挥手', 'https://placehold.co/64x64?text=Wave',  1, 1),
    ('love',  '爱心', 'https://placehold.co/64x64?text=Love',  2, 1),
    ('laugh', '大笑', 'https://placehold.co/64x64?text=Laugh', 3, 1),
    ('cry',   '哭',   'https://placehold.co/64x64?text=Cry',   4, 1)
ON DUPLICATE KEY UPDATE
    name       = VALUES(name),
    asset_url  = VALUES(asset_url),
    sort_order = VALUES(sort_order),
    is_enabled = VALUES(is_enabled);
```

**关键约束（同时落到 up 和 down 头部注释，作为强约定）**：

1. wave / love / laugh / cry 这 4 个 code **由 0010 完全占用 / 强制覆盖**
2. admin / 运维 **禁止**在这 4 个 code 上做 customization（无论如何无法存活：up 重跑会被覆盖；down 会被删除）
3. 需要新增表情（angry / surprised 等）→ 通过**新 migration**（0011+）添加
4. up 强制覆盖 + down narrow DELETE 这一对决策共同保证 Story 17.4/17.5/18.1 依赖的"4 个 enabled emoji 配置 invariant"**100% 强保证**，不依赖任何 admin 自律

**测试调整**（`migrate_integration_test.go::TestMigrateIntegration_EmojiConfigs_SeedIdempotent`）：

- 测试名保留（含义偏移成"0010 owned codes 最终态强保证"）
- setup 步骤 2 让 admin INSERT 的"坏行"在 4 个字段上**全部**与 seed 钦定值不同：
  - name: `挥手-admin` 等（与 seed `挥手` 不同）
  - asset_url: `https://admin-cdn.example.com/wave.png` 等（与 seed placehold.co URL 不同）
  - sort_order: 91 / 92 / 93 / 94（与 seed 1/2/3/4 不同）
  - is_enabled: 0（与 seed 1 不同 —— 模拟 admin "下架"了这 4 个 emoji）
- 断言步骤 5b **反转**：从 r2 的"name / asset_url 保留 admin 值" → r3 的"name / asset_url / sort_order / is_enabled 全部强制覆盖回 0010 seed 钦定值"

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写 **seed migration 的 up 路径**时，若该 migration 钦定了"X 个 owned keys 完全占用"语义（下游 Story / 业务依赖这些 keys 配置正确），**必须**用 `INSERT ... ON DUPLICATE KEY UPDATE` 强制覆盖所有 owned 字段；**禁止**用 `INSERT IGNORE` 妥协 invariant 强保证。
>
> **展开**：
>
> - **优先级铁律（**完整版**，对称应用到 up 和 down）**：
>   - **强约束（hard，无法妥协）**：
>     - down 必须真正 undo up（version ↔ data 一致）
>     - up 必须真正 establish 钦定状态（owned keys 字段值 = seed 钦定值，不论预存坏行）
>     - 下游业务依赖的 data invariant
>   - **弱约束（soft，可通过其他机制兜底）**：
>     - admin 数据保留 / customization 保留
>
> - **决策树**（写 seed migration 的 up 路径时）：
>   1. 该 migration 是否钦定了"X 个 owned keys 完全占用"？
>      - 是 → 步骤 2
>      - 否（seed 只是初始默认值，admin 可以自由调整）→ 用 `INSERT IGNORE`；
>        但**必须**接受"down 路径无法对称还原" → 下游业务不能依赖这些 keys 配置正确
>   2. 下游 Story 是否依赖这些 owned keys 配置正确？
>      - 是 → 用 `INSERT ... ON DUPLICATE KEY UPDATE` 强制覆盖；显式声明"admin 禁止 customize"
>      - 否 → 回到步骤 1 否分支
>
> - **判断启发式**：写 seed migration 时如果你正在想"INSERT IGNORE 友好一些，admin 改过的就留着"→ 先停下检查下游有没有依赖这些 keys 配置正确；只要有一个下游业务依赖，**必须**走 ON DUPLICATE KEY UPDATE 强制覆盖路径。
>
> - **配对规则（最终版，r3 lock down，替代 r1 / r2 lesson 的早期版本）**：
>   - **owned + 下游强依赖**: up 用 `ON DUPLICATE KEY UPDATE` 4 字段全覆盖 + down 用 narrow DELETE
>   - **owned + 下游无强依赖**: up 用 `INSERT IGNORE` + down 用 narrow DELETE（接受 admin 数据在 up 时不被覆盖，但在 down 时会被删 —— 见 r2 lesson）
>   - **非 owned（admin 主导）**: 不应该用 migration 做 seed；走 admin 后台 / 业务 API
>
> - **反例**：
>   - 下游 Story 依赖"4 个 enabled emoji 配置正确"，up 用 `INSERT IGNORE` → 预存坏行幸存 → 下游 invariant 失效（**本 lesson 抓的就是这条**）
>   - 下游 Story 不依赖 seed 配置正确，up 用 `ON DUPLICATE KEY UPDATE` → 把 admin 友善调整的 customization 强制覆盖回 seed 默认值 → 业务侧抱怨"我改的配置又被 migration 抹了"
>   - 用 `REPLACE INTO`（DELETE + INSERT 语义）做 seed → 看似类似 ON DUPLICATE KEY UPDATE，但触发外键级联 / id 重排 / 触发器 → 危险，**不**用
>
> - **必须配 dockertest 集成测试**：用 duplicate-code 路径（预填坏行 → 回退 schema_migrations.version → 重跑 up）验证 ON DUPLICATE KEY UPDATE 真的把 4 字段都覆盖回了 seed 钦定值。**不**走 Up → Down → Up（Down 把表 DROP 后第二次 Up 跑空表，测不到 ON DUPLICATE KEY 路径）。

## Lesson 2: 当 LLM review 在两个互相冲突的设计目标间反复横跳时，必须做**显式权衡**并 **lock down**

- **Severity**: high (P1)
- **Category**: architecture / review-meta
- **分诊**: fix（写入 r3 lesson 顶部 FINAL DECISION 标记 + 修改注释 + 文档归档）

### 症状（Symptom）

Story 17.3 在 codex review 三轮之间反复打架：

| 轮次 | finding | 建议 | 实际落地 |
|---|---|---|---|
| r1 [P2] | down DELETE 删 admin 数据 | down 改 no-op | down = `SELECT 1;` no-op |
| r2 [P2] | no-op down 违反 golang-migrate invariant | down 改回 DELETE | down = `DELETE WHERE code IN (...)` |
| r3 [P1 #1] | up INSERT IGNORE 让坏行幸存 | up 改 ON DUPLICATE KEY UPDATE | up = `INSERT ... ON DUPLICATE KEY UPDATE` |
| r3 [P1 #2] | down DELETE 删 admin 数据（**重提 r1**） | down 改保留 admin 行 | **wontfix**（与 r2 + 本轮 up 强制覆盖决策冲突） |

每一轮 review 都站在"自己关注的设计目标"视角看代码，结果在"migration invariant"和"admin 数据保留"两个互相冲突的目标间反复横跳。如果继续盲从最新一轮，进入死循环。

### 根因（Root cause）

LLM code review 模型（即使是 codex / claude / gpt-5+ 级别）有以下系统性缺陷：

1. **缺乏长期 review history 记忆**：r3 不知道 "down 不应删 admin 数据" 已经在 r1 提出过、r2 已经否决过，所以 r3 [P1 #2] 把同一个论点重新包装提了出来。
2. **单轮关注单一价值取向**：每一轮 review 都倾向于把当前看到的代码与单一价值取向（最近 prompt 强化的那个）对齐。r1 关注"业务数据安全"，r2 关注"框架契约一致性"，r3 同时关注两者但仍可能反复横跳。
3. **不会主动识别"两个建议背后是冲突的设计目标"**：除非 prompt 明确告诉它，否则 LLM review 不会自动做 meta-level "两轮 review 是否在两个互斥目标间打架" 的识别。

### 修复（Fix）

显式做出最终权衡 + lock down + 写入代码注释 + lesson 文档：

1. **代码注释**（up.sql / down.sql 头部）写清"三轮 review 演化简史"+ "FINAL DECISION"+ 关键决策点的 rationale + 反例（哪些方案被 r4+ 重新提起时应该被拒绝）
2. **lesson 文档**顶部加 `🔒 FINAL DECISION` 标记 + `final_decision_lock: true` frontmatter（这是给未来 Claude / epic-loop 主 agent 的元信号）
3. **不删 r1 / r2 lesson**：r1 / r2 是有价值的历史推演脉络，且 superseded_by / supersedes_partial 链已经建立；r3 lesson 是终点
4. **r2 lesson 注释**：r2 lesson 里"INSERT IGNORE 仍然容忍预存行"的细节在 r3 后**不再成立**；但 r2 的整体决策方向（migration invariant 优先）r3 进一步加固。r2 lesson 不需要全改，只需在跨引用层（本 r3 lesson 顶部 supersedes_partial 字段 + r2 lesson 在 r3 后保留为有效历史）说明：r3 在 r2 基础上把 up 路径**也**收紧成 invariant 强保证

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 遇到**同一个文件 / 决策点经过 N 轮 review 反复打架**的场景时，**禁止**盲从最新一轮 review；**必须**识别冲突的设计目标 → 按约束强弱排序做显式权衡 → 在代码注释和 lesson 文档里**显式 lock down**，并标记"不要在 r(N+1)+ 因为同一类 finding 重开决策"。
>
> **展开**：
>
> - **触发条件**：以下任一即触发本规则
>   - 同一个文件 / 函数 / 决策点经过 ≥3 轮 review
>   - 任一轮 review 的 finding 与前一轮 fix 直接冲突
>   - lesson 文档之间出现 superseded / supersedes_partial 链
>
> - **处理流程**（按顺序）：
>   1. **不要盲从最新一轮 review**；先读最近 N 轮的 lesson 文档（沿 supersedes_partial 链上溯）
>   2. **识别冲突的设计目标**：把不同轮 review 的核心关切抽取出来（"业务数据安全" vs "框架契约一致" vs "下游 invariant 强保证" 等）
>   3. **按约束强弱排序**：framework invariant > 下游业务依赖的 data invariant > 业务数据安全（admin 数据保留 / customization 保留）> 性能 > ergonomics
>   4. **做显式权衡**：明确选哪个、为什么、其他被牺牲的目标如何在其他路径兜底
>   5. **lock down**：
>      - 代码注释：写"三轮 review 演化简史" + "FINAL DECISION" + rationale
>      - lesson 文档顶部加 `🔒 FINAL DECISION` + `final_decision_lock: true` frontmatter
>      - 标记"不要在 r(N+1)+ 因为同一类 finding 重开决策（除非引入全新 context）"
>   6. **不删历史 lesson**：r1 / r2 / ... 都是有价值的推演脉络；通过 supersedes_partial 链建立可追溯性
>
> - **判断启发式**：
>   - 如果第 4 轮 review 再提同一类 finding，回头看 r3 lesson 顶部的 FINAL DECISION 标记 → 默认 wontfix；只有当 r4 引入了 r3 未考虑到的全新 context（如下游 Story 改了依赖契约 / 新增了 r3 时不存在的需求），才能重开决策
>   - "全新 context" 的标准：不是 reviewer 角度不同（reviewer 角度永远不同），而是**事实层面**的变化 —— 新代码引入 / 新需求 / 新外部约束
>
> - **反例**：
>   - r3 看到 "down DELETE 删 admin 数据" 的 finding，没读 r1 / r2 lesson 链，按 r3 字面建议改 down → 又回到 r1 状态 → r4 必然重提 "down 必须真正 undo up" → 死循环
>   - r3 把"admin 数据保留"和"migration invariant"两个目标当成可以折中 → 最终方案两边都不到位
>   - r1 / r2 / r3 三个 lesson 全删，只留 r3 最终方案 → 未来 Claude 不知道为什么这么决策，r4 review 把 r3 当成"看起来武断的设计选择"重新挑战 → 死循环

---

## Meta: 0010 决策三轮演化的元教训

| 轮次 | 决策点 | 选择 | 取舍 |
|---|---|---|---|
| r1 | down DELETE 删 admin 数据 | 改成 no-op | 业务数据安全 > 框架契约 |
| r2 | no-op 破 invariant | 改回 narrow DELETE | 框架契约 > 业务数据安全；admin 数据通过约定 + 新 migration 兜底 |
| r3 | up INSERT IGNORE 让坏行幸存 | 改成 ON DUPLICATE KEY UPDATE 强制覆盖 | 下游业务 invariant > admin 数据保留；admin 不应在 owned codes 上 customize（显式声明） |

**核心元教训**：seed migration 的 up / down 不是孤立的 SQL 决策，而是和"该 migration 钦定占用的 keys 是否被下游业务依赖"耦合的一组决策。三轮 review 把这层耦合一步步暴露出来：

- r1 只看到 up / down 字面对称性问题
- r2 修了 down 但没回头看 up
- r3 同时看 up + down + 下游依赖，做最终决断

**给未来 Claude 的元元规则**：写 seed migration 时**一次性**回答以下 3 个问题，避免分多轮 review 才把所有维度暴露完：

1. 该 migration 钦定的 owned keys 是哪些？
2. 哪些下游 Story / 业务依赖这些 keys 配置正确？依赖的强度是 "100% 强保证"还是 "best effort"？
3. admin / 运维是否会在这些 keys 上做 customization？如果会，谁来定 admin vs seed 的优先级？

3 个问题答案确定后，up / down 的 SQL 选择就**机械导出**了 —— 不需要后续 review 反复横跳。
