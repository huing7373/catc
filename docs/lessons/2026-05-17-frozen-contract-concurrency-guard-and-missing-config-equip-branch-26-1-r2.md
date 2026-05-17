---
date: 2026-05-17
source_review: file:/tmp/epic-loop-review-26-1-r2.md（codex review --base round 2，文件末尾最后一个 ^codex$ 段后）
story: 26-1-接口契约最终化
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-17 — 冻结契约里的并发卸下竞态兜底 & 缺失配置 equip 分支必须显式定义（26-1 r2）

## 背景

Story 26.1（接口契约最终化，POST /cosmetics/equip + unequip 节点 9 冻结契约）r2 fix-review。codex round 2 对 `docs/宠物互动App_V1接口设计.md` §8.3 / §8.4 提出 2 条 finding：一条是 §8.4 unequip 服务端逻辑 step 5-6 的 SELECT-then-DELETE 并发竞态会让"已空槽"误返成功而非冻结契约钦定的 5004；一条是 §8.3 equip step 7 对 §8.2 态 C missing-no-row 实例（合法可见可 equip 输入）没有定义错误码分支，落进未定义 / 内部错误路径。两条均为纯契约文档缺口（本 story 无守门测试），不是 r1 修复引入的 over-correction 反弹（r1 改的是 §8.3 step 4 的 5001/5002 强制映射，与本轮 §8.4 step 5-6 / §8.3 step 7 不同章节、不同代码点）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | [P1] §8.4 unequip step 5-6 并发竞态：两并发 unequip 同 petId+slot 都过 step 5，loser DELETE 0 行仍 commit → 空槽误返成功而非冻结 5004 | high | error-handling / architecture | `docs/宠物互动App_V1接口设计.md:1604-1606` | fix |
| 2 | [P2] §8.3 step 7 未定义 missing-no-row 实例 equip 行为：§8.2 态 C 声明该实例可见可 equip，step 7 裸 SELECT slot 无 missing-row 分支 → 合法流落进未定义 / 1009 | medium | error-handling / docs | `docs/宠物互动App_V1接口设计.md:1499` | fix |

修了 2 条 / defer 0 条 / wontfix 0 条。

## Lesson 1: 冻结契约里"SELECT-then-DELETE 然后无条件返成功"是 TOCTOU 竞态，必须用行锁串行化 + affected-rows 兜底双保险写死

- **Severity**: high
- **Category**: error-handling / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §8.4 服务端逻辑 step 5-6

### 症状（Symptom）

§8.4 unequip 原文 step 5 = `SELECT user_cosmetic_item_id FROM user_pet_equips WHERE pet_id=? AND slot=?` → 行不存在则 5004；step 6 = `DELETE ...` + `UPDATE 实例 status=1`；step 7 无条件 commit。两个并发 unequip 同 `pet_id+slot`：T1、T2 都在各自事务里过 step 5（都 SELECT 查到同一行），T1 先 DELETE 1 行 + commit，T2 后 DELETE 命中 **0 行**但 step 7 仍 commit、step 8 仍返回 `unequipped: true`。结果：一个对"已被 T1 卸空的槽位"的请求误返成功，违背 §1 节点 9 冻结声明"unequip 空槽显式报 5004（非幂等 noop）"+ NFR2 一致性。重复点击 / 客户端重试是该 timeline 的现实触发源。

### 根因（Root cause）

契约草稿把"存在性判断"和"删除"拆成 step 5（SELECT 判存在 → 不存在才 5004）+ step 6（DELETE）两个非原子步骤，中间没有任何串行化机制 —— 经典 TOCTOU（Time-Of-Check-To-Time-Of-Use）。SELECT 在事务内**不加锁**时只读到自己事务启动时刻的 MVCC 快照，并发事务的 DELETE 对它不可见，两个事务都能"check 通过"。然后 step 6 的 DELETE 在 loser 侧命中 0 行，但契约没要求检查 affected rows，loser 带着"什么也没删"继续 commit 并返成功。

这是 codebase 已有的同根因模式：数据库设计 §8.6/§8.7 房间 leave 事务早已用 `SELECT ... FOR UPDATE` + `DELETE ... 检查 RowsAffected == 0` 双保险解决"同 user 并发两次 leave 输家走完后续步骤产生重复广播"；Epic 20.7 r4 lesson（`2026-05-15-transaction-eliminates-rowsaffected-ambiguity-20-7-r4.md`）总结过"用事务 + FOR UPDATE 把存在性保证从 driver 层迁到 DB 层"。契约草稿没复用这个团队已沉淀的模式。

### 修复（Fix）

§8.4 step 5-6 改写为行锁串行化 + affected-rows 契约级冗余兜底（与 §8.6/§8.7 leave 事务正交双保险同构）：

- **step 5**：`SELECT ... WHERE pet_id=? AND slot=? FOR UPDATE`。`FOR UPDATE` 对该 `pet_id+slot` 行加排他锁，同 key 并发 unequip 在锁上排队；loser 事务必须等 winner commit（行已 DELETE）后才进入 step 5，此时查不到行 → 直接 5004，**不**会两个请求都越过 step 5。
- **step 6**：`DELETE` **必须**检查 `RowsAffected`；`RowsAffected == 0`（理论上已被 step 5 FOR UPDATE 阻止，本检查为不依赖锁实现细节的契约级冗余兜底）→ **回滚 + 5004**，禁止带 0 affected rows 继续 commit 误返 `unequipped: true`；`RowsAffected == 1` → 继续 UPDATE 实例 status。
- 在 §8.4 "注" 块 + 关键约束补"unequip 非幂等 + 并发对已空槽必返 5004"不变量，并显式引用 §8.6/§8.7 同根因模式。

错误码无新增（复用既有 5004），未扩张 §1 节点 9 冻结的 unequip 错误码集合 `{5002,5004 + 1001,1002,1005,1009}`。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计 / 评审任何"先 SELECT 判存在性 → 再 DELETE/UPDATE → 然后无条件返成功"的事务步骤序列**（尤其冻结契约里"空目标必返某错误码"的非幂等接口）时，**必须**在 SELECT 上加 `FOR UPDATE` 行锁串行化 **且** 在 DELETE/UPDATE 后检查 `RowsAffected == 0 → 回滚 + 返回该错误码` 双保险，**禁止**只写"应避免并发问题"这类无约束描述。

> **展开**：
> - **TOCTOU 识别信号**：契约 / 代码里出现"step N: SELECT 查到才继续；step N+1: DELETE/UPDATE；step N+2: commit + 返成功"且 step N 的 SELECT 没 `FOR UPDATE`、step N+1 没检查 affected rows —— 几乎必然有并发竞态。
> - **双保险正交、缺一不可**：`FOR UPDATE` 解决"跨事务 check-vs-delete 串行化"（让 loser 等 winner commit 后再 check）；`RowsAffected == 0 → 回滚` 解决"不依赖具体锁实现 / 隔离级别的契约级冗余兜底"（即使锁推理有疏漏也不会假成功）。两者锁对象 / 防御层不同，都写上。
> - **优先复用 codebase 已沉淀的同根因模式**：本项目数据库设计 §8.6/§8.7 leave 事务 + Epic 20.7 r4 lesson 早已是 canonical 答案。写新契约前先 grep `FOR UPDATE` / `RowsAffected == 0` 找团队既有模式，对齐措辞与机制，别另起炉灶。
> - **反例 1**：契约只写"step 5 查不到行 → 5004"+"step 6 DELETE"，靠"实装层自己注意并发"——这把竞态正确性踢给实装且没约束，winner/loser 都返成功的 bug 必然漏到联调或线上。
> - **反例 2**：只加 `FOR UPDATE` 不加 affected-rows 检查（或反之）。单一机制在极端隔离级别 / 实装细节下可能失守；冻结契约的并发不变量要双保险写死，不留实装自由度。
> - **反例 3**：finding 说"并发会出问题"，就在契约里加一句"应避免并发返回两次成功"敷衍。这是"换说法引入新歧义"——必须给**确定的**机制（哪个 SELECT 加 FOR UPDATE、哪个 DELETE 检查 RowsAffected、命中 0 行回滚返哪个码），不是无约束描述。

## Lesson 2: 上游接口声明"某状态实例对外可见且可作为下游操作的合法输入"时，下游接口必须为该状态显式定义错误码分支，且复用既有冻结错误码集合内的码

- **Severity**: medium
- **Category**: error-handling / docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md` §8.3 服务端逻辑 step 7

### 症状（Symptom）

§8.2 inventory 的 config 三态矩阵态 C "missing-no-row"（admin 物理删了 `cosmetic_items` 行但用户已拥有的 `user_cosmetic_items` 实例仍在、`status IN (1,2)`）显式声明：该实例**仍出现在 inventory**（已拥有不得静默丢失），即 `status = 1` 的 missing-no-row 实例是 client 能在仓库里看到并合法发起 equip 的输入。但 §8.3 equip step 7 = 裸 `SELECT slot FROM cosmetic_items WHERE id = <实例.cosmetic_item_id>`，对"行不存在"没有任何分支 —— 该合法流落进未定义 / 内部错误（1009）路径，等于"§8.2 声明可见可 equip 但 §8.3 没有对应契约"的跨文档缺口。

### 根因（Root cause）

§8.3 草稿默认"实例的 cosmetic_item_id 必然能 JOIN 到 cosmetic_items 行"，没意识到 §8.2 态 C 已把"配置行被物理删除但实例仍可见可操作"定义为**受支持的常态**（不是脏数据异常）。两个接口由同一份契约文档定义，但 §8.3 step 7 没和 §8.2 态 C 对齐 —— 跨章节不变量没做闭环。

### 修复（Fix）

§8.3 step 7 拆成两分支：行存在 → 拿 slot 继续；行不存在（missing-no-row，与 §8.2 态 C 同源）→ **5003 道具状态不可用**（slot 不可得 → 实例不可穿戴）+ log error（与 §8.2 态 C 一致的数据治理告警）+ 事务回滚。

**错误码选择 = 5003**，理由：
- 实例**存在**于 `user_cosmetic_items`，故不能用 5001（5001 由 fix-review 26-1 r1 [P2] 锁定**仅**对应"实例完全无 row"，复用会与 r1 锁定语义直接冲突）。
- 不属于"不属于当前用户"，排除 5002。
- 语义最贴近 5003"道具状态不可用"——配置已物理删除使该实例当前不可穿戴，与 5003 既有语义（consumed/invalid 不可穿戴）同类。
- 5003 **已在** §1 节点 9 冻结的 equip 错误码集合 `{5001,5002,5003,5008}` 内，复用它**未扩张**冻结集合（override 硬约束：节点 9 equip 错误码集合不能扩张、禁止新造码）。

同步更新 §8.3 错误码表 5003 行触发条件 + 关键约束加"missing-no-row equip → 5003"契约段，跨文档自洽。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **定义 / 评审接口契约时，凡上游接口（同份契约文档的另一节）已声明"某状态的实体对外可见且可作为本接口的合法输入"**，本接口的服务端逻辑**必须**为该状态显式写出错误码分支，且该错误码**必须**取自本接口已冻结的错误码集合（不新造、不扩张），并跨节同步错误码表 + 关键约束。

> **展开**：
> - **跨节闭环自检**：写完一个接口的服务端逻辑，对每个"SELECT 关联表"步骤问"关联可能查不到吗？查不到是脏数据异常还是被另一节定义成了受支持常态？"——若另一节（如 inventory config 三态、status 枚举边界）已声明该缺失是常态且实体仍可见可操作，本接口就必须有对应分支，不能落进 1009/未定义。
> - **错误码选择三原则**：① 不与本文档其他 finding/round 已锁定的同号码语义冲突（本案 5001 被 r1 锁死"无 row"，故 missing-but-exists 不能用 5001）；② 取自本接口**已冻结**的错误码集合，绝不新造、绝不扩张冻结集合；③ 选语义最贴近的既有码（本案 5003"状态不可用"贴合"配置删除致不可穿戴"）。
> - **跨文档一致性是契约 story 的"测试"**：纯契约 story 无守门单测，唯一防回归手段是改完做"本节用到的错误码 ⊆ §3 表"+"冻结集合不多不少"+"上下游不变量闭环"+"不破坏前序 round 锁定语义"四项人工自检。把这四项当作 mandatory gate。
> - **反例 1**：默认"JOIN 必然命中"，不为关联缺失写分支 —— 上游已声明该缺失合法可见时，这就是跨文档契约缺口，下游 client 拿到 1009 无法正确处理。
> - **反例 2**：为新场景新造错误码（如造个 5009）。冻结契约的错误码集合是 iOS/server 共享的硬边界；新造码 = 扩张冻结集合 = 契约破坏，必须先复用既有码。
> - **反例 3**：选了个语义不贴的既有码（如对"配置删除"用 5001"道具不存在"）—— 与同号码在别处被锁定的语义冲突，制造跨节自相矛盾。选码前先 grep 该码在本文档所有出现处，确认语义不打架。

---

## Meta: 本次 review 的宏观教训

26-1 r1（错误码映射不能写成"实装层二选一"）+ r2（并发兜底要双保险写死 / 缺失配置分支要显式定义）指向同一类宏观漏洞：**冻结契约里任何"对外可观测的行为"都不能留实装自由度或未定义分支**。冻结契约是 iOS/server 并行开工的唯一同步面，凡 client 能观测到的输出（错误码、并发下的成功/失败、边界状态的响应）都必须在契约里被**唯一确定**地钉死，并与同文档其他节的不变量做闭环自检 —— 这是纯契约 story 替代单测的核心质量动作。
