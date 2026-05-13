---
date: 2026-05-14
source_review: codex round 1 review of Story 17.3 (file: /tmp/epic-loop-review-17-3-r1.md)
story: 17-3-emoji_configs-seed
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-14 — INSERT IGNORE seed migration 的对称 down + duplicate-code 路径测试

## 背景

Story 17.3 落地 `0010_seed_emoji_configs.up/down.sql`，up 用 `INSERT IGNORE INTO emoji_configs (...) VALUES (...)` seed 4 个表情。codex round 1 review 抓到两条 P2：

1. **down 与 up 不对称**：up 的 `INSERT IGNORE` 故意保留预存的 wave/love/laugh/cry 行（admin 手工插入 / 历史残留），但 down 走 `DELETE FROM emoji_configs WHERE code IN (...)` 会把这些预存行也删掉。
2. **SeedIdempotent 测试名不副实**：Up → Down → Up 路径里 Down 把整张表 DROP 掉 → 第二次 Up 跑空表 → 把 `0010.up` 从 `INSERT IGNORE` 改成普通 `INSERT` 也能过。没真正测到 `duplicate-code` 路径。

两条都是真问题，本轮 fix 修。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Preserve pre-existing emoji rows on 0010 rollback | medium (P2) | architecture / migration-symmetry | fix | `server/migrations/0010_seed_emoji_configs.down.sql` |
| 2 | Exercise duplicate-seed path instead of drop-and-recreate | medium (P2) | testing | fix | `server/internal/infra/migrate/migrate_integration_test.go` |

## Lesson 1: INSERT IGNORE seed migration 的 down 不应做 per-row DELETE

- **Severity**: medium (P2)
- **Category**: architecture / migration-symmetry
- **分诊**: fix
- **位置**: `server/migrations/0010_seed_emoji_configs.down.sql:13`

### 症状（Symptom）

`0010.down` 走 `DELETE FROM emoji_configs WHERE code IN ('wave','love','laugh','cry')`。当 DB 在 apply 0010 之前已存在同 code 的 admin / dev / 历史残留行时，up 路径的 `INSERT IGNORE` 故意保留这些预存行不动；但 down 路径会把它们一并删掉，造成 **up / down 不对称** —— 在 duplicate-code 场景下静默丢失 admin 数据。

### 根因（Root cause）

写 down migration 时下意识地"做 up 的反向操作" → "up 是 INSERT 4 行 → down 是 DELETE 这 4 个 code"。这种"机械镜像"忽略了 `INSERT IGNORE` 的契约本质：

> `INSERT IGNORE` 不是"我一定写入了这些 row"的承诺，而是"如果 unique key 没命中就写入，命中就当无事发生"。换句话说，up 完成后**不能假设**这些 row 的来源都是本次 up。

所以 down 也不能假设"这些 row 都是我插入的，删掉是安全的"—— 反向操作的对象集合 != up 操作的对象集合。

### 修复（Fix）

把 `0010.down.sql` 改成 **no-op**（用 `SELECT 1;` 占位，让 golang-migrate 找得到 down 文件不报 dirty）：

- **单跑 0010.down**（仅回退 seed，不回退 0009 schema）→ no-op：再 up 一次会重新跑 0010.up，缺失的行被 `INSERT IGNORE` 补回，预存行保留；语义自洽。
- **跑 0009.down 链式带 0010.down**：0009.down `DROP TABLE emoji_configs` 已经覆盖了整张表的清理，0010.down 不需要 per-row DELETE 重复劳动。

before / after：

```sql
-- before（删除可能不是自己插入的行 → 数据损失）
DELETE FROM emoji_configs WHERE code IN ('wave', 'love', 'laugh', 'cry');

-- after（no-op；整表清理交给 0009.down DROP TABLE）
SELECT 1;
```

up 文件加 cross-reference 注释指向 down 的 no-op 决策 + lesson 路径，便于未来读者理解。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写 **seed migration 的 down** 时，若 up 用了 **`INSERT IGNORE` / `ON DUPLICATE KEY UPDATE`** 这类 **"容忍预存行"** 的语义，down **禁止**做 per-row DELETE / UPDATE 反向操作；整表清理责任**必须**交给 schema migration（前一号 migration 的 DROP TABLE）。
>
> **展开**：
> - `INSERT IGNORE` 的契约是 "row 不一定是 up 插入的"；down 反向 DELETE 会破坏对称性 → 在 duplicate-code 场景下静默丢失预存数据。
> - 同理，`INSERT ... ON DUPLICATE KEY UPDATE` 在 down 时也不应"反向把字段还原"—— admin 可能在 up 之后又改了字段，down 反向操作会覆盖 admin 改动。
> - **判断启发式**：up 文件里出现以下任一关键字 → down 倾向于 no-op：
>   - `INSERT IGNORE`
>   - `ON DUPLICATE KEY UPDATE`
>   - `REPLACE INTO`（虽然语义更激进，但 down 同样难以"反向"）
> - **反例**：把 `INSERT IGNORE` 配对一个 `DELETE WHERE code IN (...)` 的 down → 看似对称、实则破坏 INSERT IGNORE 的"容忍预存"契约。code reviewer 视角下这是典型的 "asymmetric migration pair" 异味。
> - **配对规则**：
>   - up 用普通 `INSERT INTO`（保证行一定是自己插入） → down 可以 per-row DELETE
>   - up 用 `INSERT IGNORE` / `ON DUPLICATE KEY UPDATE` → down 走 no-op（`SELECT 1;` 占位）；整表清理交给前一号 migration 的 DROP TABLE
> - **不要写空文件**：golang-migrate 要求每个 version 的 up / down 文件都存在，空文件可能触发 dirty 状态；用一句 no-op SQL 占位（`SELECT 1;`）。

## Lesson 2: 测 `INSERT IGNORE` 幂等必须走 duplicate-code 路径，不能走 drop-and-recreate

- **Severity**: medium (P2)
- **Category**: testing
- **分诊**: fix
- **位置**: `server/internal/infra/migrate/migrate_integration_test.go:1130-1147`（原版）

### 症状（Symptom）

原版 `TestMigrateIntegration_EmojiConfigs_SeedIdempotent`：

1. `mig.Up()` → 4 行
2. `mig.Down()` → 表被 0009.down DROP，行数 0
3. `mig.Up()` → 重建表 + INSERT seed → 4 行
4. 断言 count == 4

**问题**：步骤 3 跑的是**空表**，没有任何 row 撞 `uk_code`，所以 `INSERT IGNORE` 退化为普通 `INSERT`。把 `0010.up` 里的 `INSERT IGNORE` 改成 `INSERT INTO` —— 这个测试**仍然能过**。

→ 测试名"SeedIdempotent"声称覆盖 INSERT IGNORE 语义，但实际什么也没测。

### 根因（Root cause）

写测试时把"幂等"等同于"重复跑 up 不报错且行数对"。**Up → Down → Up** 看起来像幂等测试，但只测了 golang-migrate 框架的"再跑 up 不挂"，没测 SQL 层 `INSERT IGNORE` 关键字的实际生效路径。

真正要触发 INSERT IGNORE 必须满足两个前提：
1. 表里**已经**有 4 行 wave/love/laugh/cry（duplicate-code 行存在）
2. 跑一次 `INSERT IGNORE INTO ... VALUES (wave, ...), (love, ...), ...`（4 个 unique key 全部命中）

`Down()` 把表 DROP 了 → 前提 1 被破坏 → 测试退化。

### 修复（Fix）

重写测试逻辑，分 5 步：

1. `mig.Up()` 跑全程到 v=10（表 + 4 行 seed 落地）
2. `DELETE FROM emoji_configs WHERE code IN (...)`，然后手动 `INSERT INTO emoji_configs (...) VALUES (...)` 4 行 **admin-flavored** 数据：asset_url 用 `https://admin-cdn.example.com/...`、name 用 `挥手-admin` 等，与 seed 钦定的 `https://placehold.co/...` / `挥手` 故意区分
3. `UPDATE schema_migrations SET version = 9, dirty = 0` 把 golang-migrate 内部记录的版本号回退到 9，使下次 Up 重跑 0010.up
4. 关掉当前 migrator + 新建一个（golang-migrate 内部 cache 版本号，必须重开实例才会重新读 schema_migrations）→ `mig2.Up()`
5. 断言：
   - 行数仍 4（不翻倍到 8）
   - 每行的 name / asset_url 仍是 admin 值（`挥手-admin` / `https://admin-cdn.example.com/wave.png`），**不是** seed 值

断言 5 的字段值检查是关键 —— 把 `INSERT IGNORE` 改成普通 `INSERT INTO` 时，第二次 Up 会撞 `uk_code` 1062 直接报错；把 `INSERT IGNORE` 改成 `INSERT ... ON DUPLICATE KEY UPDATE name=VALUES(name), asset_url=VALUES(asset_url)` 时，行数对但 admin 值被覆盖 → 字段断言炸。两种"伪幂等"实现都能被本测试抓到。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在测 **`INSERT IGNORE` / `ON DUPLICATE KEY UPDATE` 等"重复键容忍"语义** 时，**禁止**走 "drop-and-recreate" 路径（Up → Down → Up）；**必须**走 **duplicate-code 路径**：预填和待 seed 冲突的行 → 再触发 seed → 验证不报错 + 行数 + 预存字段值。
>
> **展开**：
> - **判断启发式**：当被测 SQL 含以下任一关键字时，drop-and-recreate 测试一定无效：
>   - `INSERT IGNORE`
>   - `ON DUPLICATE KEY UPDATE`
>   - `REPLACE INTO`
>   - `INSERT ... WHERE NOT EXISTS (SELECT ...)`
> - **正确测法骨架**：
>   ```
>   1. 用 schema migration 建表
>   2. 手动塞 N 行**与 seed 冲突**的数据（故意改字段值区分 admin vs seed）
>   3. 触发 seed SQL（重跑 migration / 直接 exec）
>   4. 断言 row count 不翻倍 + 预存字段值不被覆盖
>   ```
> - **关键技巧**：让"预存行"的字段值与 seed 钦定值**故意不同**，这样字段级断言能区分 "INSERT IGNORE（保留预存）" vs "ON DUPLICATE KEY UPDATE（覆盖预存）" vs "普通 INSERT（直接撞 1062）" 三种实现。
> - **回滚版本号的技巧**：当被测包没暴露 `Force` API 时，dockertest 里直接 `UPDATE schema_migrations SET version = ?` 是合法且最小的 fixture 手段；golang-migrate 内部 cache 版本号，必须 `Close()` 旧实例 + `New()` 新实例才会重新读。
> - **反例**：
>   - 走 `mig.Up() → mig.Down() → mig.Up()` 然后断言 count 不变。Down 把表 drop 后第二次 Up 跑空表，INSERT IGNORE 跟普通 INSERT 行为一致 → 测了寂寞。
>   - 用 `INSERT IGNORE INTO ... VALUES (相同行)` 直接连跑两次然后断言 row count = N（不是 2N）。表面上对，但只测了同进程内同语句的去重，没测 "已存在不同字段值的预存行" 这个核心场景。
> - **不只断言 count**：count 只能抓 "行数翻倍" / "撞 1062 报错"；要抓 `ON DUPLICATE KEY UPDATE` 这种"行数对但字段被覆盖"的伪实现，**必须**断言关键字段的实际值。

---

## Meta: 本次 review 的宏观教训

这两条 finding 指向同一个深层问题：**写"幂等" / "容忍"语义的 migration 时，up / down 对称性 + 测试覆盖必须三方一致**：

- up 语义：`INSERT IGNORE` 表示"容忍预存行"
- down 语义：必须**不**反向破坏 up 容忍的对象（→ no-op，整表清理交给前一号 schema migration）
- 测试语义：必须真触发"预存行存在"的场景，不能用 drop-and-recreate 绕过

任何一方与另外两方不一致都是 bug：
- up 用 INSERT IGNORE + down 用 per-row DELETE → up 容忍的行被 down 删掉（lesson 1）
- 测试用 drop-and-recreate → INSERT IGNORE 关键字没被实际测到（lesson 2）

**统一启发式**：写 / 改 / 评审任何用了"容忍重复键"语义（`INSERT IGNORE` / `ON DUPLICATE KEY UPDATE` / `REPLACE INTO` / `INSERT ... WHERE NOT EXISTS`）的 SQL 时，**必须**在同一个 review pass 里同时检查 up + down + 测试三处的一致性；缺一会留死角。
