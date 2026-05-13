---
date: 2026-05-14
source_review: codex round 2 review of Story 17.3 (file: /tmp/epic-loop-review-17-3-r2.md)
story: 17-3-emoji_configs-seed
commit: d4467f1
lesson_count: 1
supersedes_partial: 2026-05-14-insert-ignore-symmetric-down-and-test.md (Lesson 1)
locked_down_by: 2026-05-14-0010-final-decision-up-force-overwrite-down-narrow-delete.md (r3 在 r2 基础上把 up 路径也收紧成 invariant 强保证)
status: valid-historical-context; up 路径细节被 r3 lesson 进一步收紧
---

> 📎 **r3 update note (2026-05-14 r3 review 后追加)**：
>
> 本 lesson 的核心决策（down 必须真正 undo up）**仍然有效** —— r3 没有推翻 r2 关于 down 路径的决断；narrow DELETE 在 r3 后**继续保留**。
>
> 但本 lesson 里关于 up 路径的注释（"INSERT IGNORE 容忍预存行" / "admin 数据保留通过约定 + 新 migration 兜底"）在 r3 后**部分失效**：
> - r3 抓到 `INSERT IGNORE` 让预存的"坏行"（is_enabled=0 / asset_url='' / sort_order 乱序）幸存，**下游 Story 17.4/17.5/18.1 依赖的"4 个 enabled emoji 配置正确"invariant 无法保证**。
> - **最终决断**：up 改成 `INSERT ... ON DUPLICATE KEY UPDATE` 强制覆盖 4 字段。
> - **admin 数据保留约定**进一步收紧为：admin **禁止**在 0010 owned codes 上做 customization（up 重跑会被覆盖；down 会被删除；customization 无论如何无法存活）。
>
> r2 的方向（migration invariant > admin 数据保留）r3 进一步加固到 up 路径；具体最终决断见 → **[docs/lessons/2026-05-14-0010-final-decision-up-force-overwrite-down-narrow-delete.md](2026-05-14-0010-final-decision-up-force-overwrite-down-narrow-delete.md)**。
>
> **未来 Claude 读到本 lesson 请同时读 r3 lesson；r3 是最终落地版本。**

# Review Lessons — 2026-05-14 — seed migration 的 down 必须真正 undo up（migration invariant 优先于 admin 数据保留）

## 背景

Story 17.3 落地 `0010_seed_emoji_configs.up/down.sql`，r1 review 阶段曾把 `0010.down.sql` 改成 **no-op**（`SELECT 1;` 占位），理由：up 用 `INSERT IGNORE` 故意"容忍预存行"，down DELETE 反向操作会破坏对称性、静默丢失 admin 数据（参考 r1 lesson `2026-05-14-insert-ignore-symmetric-down-and-test.md` Lesson 1）。

r2 review 抓出这条 r1 决策违反 golang-migrate 框架的标准 **invariant** "down 必须真正 undo up"：

- 操作员单步回滚（`migrate down 1` / `goto 9`）→ schema_migrations.version=9，但 wave/love/laugh/cry 行仍存在 → 数据库内容与版本号不一致。
- `INSERT IGNORE` 的"保留预存行"语义不构成借口，因为 0010 自己 INSERT 的行也活过了这次 down → migration 框架视角下这是 functional bug。

r1 与 r2 是两个互相冲突的设计目标的两面：
- **r1 P2**：silent admin data loss（关注业务数据安全）
- **r2 P2**：migration invariant violation（关注框架契约一致性）

本轮 fix 重新做权衡：**优先 migration invariant**，admin 数据保留通过"约定 + 新 migration"兜底，**不**通过 down 路径搞 no-op。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | down 必须真正 undo up（即使 up 用 INSERT IGNORE） | medium (P2) | architecture / migration-invariant | fix | `server/migrations/0010_seed_emoji_configs.down.sql` |

## Lesson 1: seed migration 的 down 必须真正 undo up；INSERT IGNORE 的"容忍预存"语义不是 down no-op 的借口

- **Severity**: medium (P2)
- **Category**: architecture / migration-invariant
- **分诊**: fix
- **位置**: `server/migrations/0010_seed_emoji_configs.down.sql`

### 症状（Symptom）

r1 fix 把 `0010.down.sql` 改成 `SELECT 1;`（no-op），理由是 up 用 `INSERT IGNORE` 容忍 admin 预存行，down DELETE 会反向破坏。但这违反 golang-migrate 框架 invariant：

```
操作员: migrate down 1     # 想回退 17.3 seed
golang-migrate: schema_migrations.version 9 ← 10
DB 实际状态: wave / love / laugh / cry 4 行仍在
→ 版本号与表内容不一致
→ 同事 / CI / staging 看到"v=9 但有 seed 行" → 困惑 + 调试黑洞
```

更糟糕：0010 自己 INSERT 的 4 行也活过了这次 no-op down → 完全没"回退"。这是 migration 框架层面的 bug，不是"业务可接受的代价"。

### 根因（Root cause）

r1 决策把两个互相冲突的设计目标当成"哪个都可以"的二选一，但 migration 工具链里**有强弱之分**：

- **强约束**（无法妥协）：down 必须真正 undo up；版本号 ↔ schema/数据状态 必须一致；这是 golang-migrate / Flyway / Alembic 等所有 migration 框架的核心契约。违反 → migration 状态机不可信。
- **弱约束**（可通过其他机制兜底）：admin 数据保留可以通过"约定 + 新 migration + 备份"等手段在 down 路径**外**解决。

r1 把弱约束（admin 数据保留）凌驾在强约束（migration invariant）之上 → 错位决策。

另一个根因：r1 没意识到"down no-op 让 up 不可逆"在生产 ops 视角下是个 trap —— ops 以为执行了 `migrate down 1` 就回退了，结果数据还在 → 二次操作可能踩雷。

### 修复（Fix）

把 `0010.down.sql` 改回 **narrow DELETE 4 行**：

```sql
-- after（r2 定稿）
DELETE FROM emoji_configs WHERE code IN ('wave', 'love', 'laugh', 'cry');
```

**关键约束**（写入 up / down 头部注释作为强约定）：

1. `wave / love / laugh / cry` 这 4 个 code **由 0010 钦定固定占用**，是系统 seed 表情；admin / 运维 **不应**手工 INSERT 这 4 个 code。
2. 需要新增表情（如 `angry` / `surprised`）→ 通过**新 migration**（0011+）添加，不要在 emoji_configs 表上做 admin 直插。
3. `0010.up` 用 `INSERT IGNORE` 是为了**仅**容忍两类预存行：
   - admin 误操作残留（违反约定 1 的预存行）
   - 测试环境残留 / 上次回滚未清干净的本 seed 行
   —— `0010.down` 会清掉它们；admin 真要保留同 code 行需要手动 stage rollback（`migrate down 1` 前先备份）。
4. `0010.down` 是 **narrow DELETE 4 行**（不是 `TRUNCATE`、不是全表 `DELETE`）：只删 4 个钦定 code，**不会动** 0011+ 加的新表情（如 angry / surprised）的行。

**测试调整**（`migrate_integration_test.go::TestMigrateIntegration_EmojiConfigs_SeedIdempotent`）：
- 测试代码（setup + 断言）**不**改动，仍走 "预填 admin-flavored 行 → 回滚 schema_migrations.version → 重跑 up" 路径。
- 测试**注释**调整：核心结论从 r1 的"down 后预存行保留"收紧成"INSERT IGNORE 在 duplicate code 时 server 端表现：不报错 + 不翻倍 + 不覆盖现有值"。"down 后预存行保留"语义在 r2 后已不成立（down=narrow DELETE 会把这些行删掉）。
- 测试名保留 `SeedIdempotent`（含义偏移成 "up 操作幂等 + 不破坏现有数据"）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写 / 评审 **seed migration 的 down 路径**时，**禁止**用 "up 用了 INSERT IGNORE / ON DUPLICATE KEY UPDATE 等容忍语义" 当成 "down 可以 no-op" 的理由；**必须**让 down 真正 undo up（narrow DELETE / narrow UPDATE 回滚字段），即使这会删掉 admin 误操作的预存行。
>
> **展开**：
> - **优先级铁律**：migration invariant（down 必须真正 undo up；版本号 ↔ schema/数据 一致）是 **强约束**；admin 数据保留是 **弱约束**。两者冲突时**优先**前者。
> - **admin 数据保留的正确兜底路径**（不是 down no-op）：
>   - **强约定**：在 up/down 头部注释里明示"这些 code/key 由本 migration 钦定占用，admin 不应手工 INSERT 同 key 行"。
>   - **新 migration 扩展**：admin 真要新增同表数据 → 新建 migration 0011+ 添加，而不是直接 INSERT。
>   - **运维 SOP**：单步 `migrate down 1` 前 ops 自己负责备份（这是 migration 工具的标准用法约定）。
> - **判断启发式**：写 down migration 时如果你正在想"删掉这些行可能不是我插的，所以让 down no-op 吧"→ 停下；改去想"这些 row 的 key 是不是应该由本 migration 钦定占用？如果是，admin 不应插同 key，down 就该删；如果不是，up 就不应该 INSERT 这些 key"。
> - **narrow vs broad DELETE**：down 必须是 **narrow**（明确列出本 up 插入的 key），**禁止**用 `TRUNCATE` / `DELETE WHERE 1=1` —— 后两者会误删 0011+ 后续 migration 加的同表行。
> - **migration 框架视角的"对称"**：framework 视角下"对称"指 "version=N 状态可逆" —— up 应用后 version=N，down 应用后 version=N-1 且 DB 状态等于"从未应用过 N"。**不是**指 SQL 字面对称（"up INSERT k 行，down DELETE k 行"是巧合的字面对称，本质是 version=N → version=N-1 的状态等价）。INSERT IGNORE 在 up 路径下"撞 key 静默通过"是为应对 dirty / force / 手工跑等**异常路径**的双层兜底，**不**改变 "version=N 状态" 的语义。
> - **反例**：
>   - up 用 INSERT IGNORE 4 行 + down 用 `SELECT 1;` no-op → 单步回滚后 version=N-1 但行仍在 → migration 状态机不可信。
>   - up INSERT 4 行 + down `TRUNCATE TABLE emoji_configs` → 删多了，后续 migration 加的行也被清掉。
>   - up INSERT 4 行 + down `DELETE FROM emoji_configs` 无 WHERE → 同上。
> - **配对规则（修正版，替代 r1 lesson 的 "INSERT IGNORE → down no-op" 错误规则）**：
>   - up 用普通 `INSERT INTO` → down 用 narrow DELETE（明确 4 个 key）
>   - up 用 `INSERT IGNORE` → down **仍**用 narrow DELETE（不是 no-op；通过约定+新 migration 兜底 admin 数据）
>   - up 用 `INSERT ... ON DUPLICATE KEY UPDATE name=VALUES(name)` → down 用 narrow UPDATE（把字段恢复成已知的"pre-up 默认值"；如果不知道 pre-up 值就 narrow DELETE）
>   - up 用 `REPLACE INTO` → 同上，down 用 narrow UPDATE / DELETE

---

## Meta: round 1 vs round 2 互相打脸的设计冲突教训

这次 review 的核心元教训：**当两轮 review 给出互相冲突的建议时（r1 说 A，r2 说 not A），不要简单选最新一轮，而要识别两个建议背后的设计目标是否本来就互相冲突；如果冲突，按约束强弱排序做权衡，不是"听最近一次"也不是"折中"**。

本次冲突的两个目标：
- r1: 防止 admin 数据被 down silent-loss（业务数据安全）
- r2: 保持 migration framework invariant（工具链一致性）

这两个目标在 INSERT IGNORE seed migration 的 down 路径上**先天**互斥（down DELETE 会丢 admin，down no-op 会破 invariant），不存在两全方案。需要按约束强弱选择 —— **migration framework invariant 是 hard constraint**（违反则工具链状态机不可信，影响所有 ops），admin 数据保留是 soft constraint（可通过约定 / 新 migration / 备份等多路径兜底）。

**未来 Claude 遇到此类"两轮 review 互相打脸"场景的处理流程**：

1. **不要假设**最新一轮 review 自动正确；最新一轮可能没看到前一轮的设计权衡上下文。
2. **识别**两条 review finding 背后是不是冲突的设计目标，而不是表面冲突。
3. **按约束强弱排序**：framework invariant > 业务数据安全 > 性能 > ergonomics（一般顺序，具体场景可调）。
4. **把弱约束移到 fallback 路径**：admin 数据保留不靠 down no-op，而靠"约定 + 新 migration + ops 备份 SOP"。
5. **在 lesson 文档里 supersede 旧 lesson**：r1 lesson 顶部加 superseded 警告 + 指向 r2 lesson 的最终决策，避免未来 Claude 翻到 r1 时被误导成 down no-op。
