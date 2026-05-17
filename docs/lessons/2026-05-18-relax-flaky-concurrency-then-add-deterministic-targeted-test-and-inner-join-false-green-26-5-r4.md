---
date: 2026-05-18
source_review: "file: /tmp/epic-loop-review-26-5-r4.md (codex review, epic-loop r4)"
story: 26-5-layer-2-集成测试-穿戴事务全流程
commit: 8c823e0
lesson_count: 2
---

# Review Lessons — 2026-05-18 — 放松 flaky 并发断言后必须新增独立确定性测试补回安全网（而非回退强断言）& 双向一致性 helper 用 INNER JOIN 会把违例悬挂行 join 掉而误绿（26-5 r4）

## 背景

Story 26-5（Layer 2 集成测试 — 穿戴事务全流程）第 4 轮 codex review。前 3 轮（r1→r3）是同一条并发断言的 over-correction ping-pong，已在 r3 用「实证 + 契约语义层」收敛（并发1 case `successCount >= 1` + 终态一致性矩阵 + 守门注释）。r4 codex 提了 2 条 [P2]：① r3 放松并发断言后，并发1 case 在「Equip 假想全串行化回归」下不再确定性 exercise uk_pet_slot 重复键兜底路径（覆盖缺口）；② `assertEquipStateConsistency` 反向检查用 INNER JOIN，会把违反不变量的悬挂装备行自己 join 掉而误绿。本轮是 r1/r2/r3 chain 的**正确收尾**——不开第 4 跳。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 放松并发断言后 uk_pet_slot 兜底覆盖缺口 | medium (P2) | testing | fix（新增独立确定性测试） | `server/internal/service/cosmetic_equip_service_integration_test.go` |
| 2 | 双向一致性 helper INNER JOIN 漏检悬挂行 | medium (P2) | testing | fix（LEFT JOIN 保留违例行） | `server/internal/service/cosmetic_equip_service_integration_test.go:432-435` |

## Lesson 1: 放松 flaky 并发断言后，必须用独立确定性测试补回被放松掉的特定安全网（而非回退到 flaky 强断言）

- **Severity**: medium (P2)
- **Category**: testing
- **分诊**: fix
- **位置**: `server/internal/service/cosmetic_equip_service_integration_test.go`（并发1 case `_Concurrent100SamePetSlot_FinalStateConsistent` 行 ~862-877 不动；新增 `_UkPetSlotDuplicateKey_DeterministicFallback`）

### 症状（Symptom）

并发1 case 的成功计数断言经 r1`==1`→r1 放松`>=1`→r2 错误收回`==1`→r3 实证证伪回`>=1`+终态矩阵。r3 终态对 swap 语义正确，但放松后留下覆盖缺口：若未来 Equip 在 `InsertInTx` 前变**全串行化**（如给 slot lookup 加锁），即使 `uk_pet_slot` 重复键→回滚路径**完全坏掉**，每个 goroutine 也都能作为合法 swap 成功、终态仍 1 行 1 equipped，并发1 case 照样绿——它不再确定性 exercise 这条 DB 兜底路径。

### 根因（Root cause）

并发集成测试天然有两个互斥诉求：(a) 验证真并发竞态在 swap 语义下不破坏终态一致性；(b) 确定性证明 DB UNIQUE 兜底这条**特定路径**被走到。放松到 `>=1`+终态矩阵满足了 (a)，但 (a) 的终态断言在「全串行化合法 swap」假想回归下恒真，**不蕴含** (b)。`uk_pet_slot` 兜底在真实生产中只在并发下触发（goroutine A 的 `FindByPetSlot` 读到空、B 已 commit、A 的 `InsertInTx` 才撞 UNIQUE），而并发本身就是 flaky 来源。把成功计数收回 `==1` 是错的（swap 语义下伪不变量，已被实证 3 次否定）——正确解法是 (a)(b) 用**两个正交测试**分别承载：并发 case 保持放松（承载 a），新增**独立的、不靠并发的确定性测试**承载 (b)。

### 修复（Fix）

并发1 case（成功计数语义 / 守门注释 / 终态矩阵）**保持 r3 现状不动**。新增独立确定性测试 `TestCosmeticEquipServiceIntegration_UkPetSlotDuplicateKey_DeterministicFallback`，无 goroutine / 无 race / 无 flaky，分两段确定性覆盖「uk_pet_slot 重复键 → 回滚 → 错误映射」全链：

- **段 A（repo 直测）**：seed user/pet + 直接 `INSERT` 一条 (pet,slot=1) `user_pet_equips` 行模拟「赢家已提交」→ 在事务内对**同 (pet,slot=1)** 调 `userPetEquipRepo.InsertInTx` → 真实 MySQL `uk_pet_slot` 拒绝 → 断言 `errors.Is(err, mysql.ErrUserPetEquipPetSlotDuplicate)` 哨兵 + 重复行未落库。
- **段 B（service 全链）**：新增 `findByPetSlotNotFoundStub`（`FindByPetSlot` 恒返 `ErrUserPetEquipNotFound` 哨兵，迫 service.Equip 步骤 8 走「slot 无装备 → 跳过 swap」分支，`InsertInTx` 透传真 repo）→ service 步骤 9 `InsertInTx` 撞预 seed 行 → 真实 `uk_pet_slot` 拒绝 → repo `PetSlotDuplicate` 哨兵 → service `errors.Is` → 映射成冻结契约钦定的 1009 `ErrServiceBusy` + 事务 ROLLBACK 验证。

走 service-stub + repo 双段（非纯 service 路径）的原因：service.Equip 步骤 8 **总是先** `FindByPetSlot`，若 slot 已有行就走 swap 分支、永不让 `InsertInTx` 撞 UNIQUE——不靠并发就无法纯 service 命中，故用 stub 把 swap 分支屏蔽掉。与 Story 26-2 `TestMigrateIntegration_UserPetEquips_UniqueConstraints_Rejected`（纯 DDL 约束存在性，raw SQL）**正交互补**：本测试覆盖的是「GORM Create → 1062 → repo 按约束名分流哨兵 → service 1009」这条应用层翻译链，不重复造轮子；测试注释里点明此分工避免 reviewer 误判重复。

case 总数 12 → 13；`go vet -tags=integration`、`-list`、`build.sh --test` 均真实通过（本机无 Docker 不实跑集成执行）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **把一条 flaky 的并发断言放松（如 `==1`→`>=1`）以匹配系统真实语义** 时，**必须** **同时新增一个独立的、不靠并发的确定性测试，专门承载被放松掉的那条特定安全网覆盖**——而**禁止**靠回退到 flaky 强断言来"保住覆盖"。

> **展开**：
> - 放松并发断言只解决「断言与语义不符」，不解决「这条特定 DB/竞态兜底路径还要被确定性证明走到」——后者必须由正交的确定性测试承载。
> - 确定性命中「只在并发下触发的兜底路径」的标准手法：用 stub/fault wrapper 屏蔽掉让路径不可达的前置分支（这里是 `FindByPetSlot` 恒返 NotFound 迫跳 swap），或直接在 repo/DB 层预造「并发赢家已提交」的前置状态，再走一条单线程必然撞约束的路径。
> - 新增确定性测试要与既有同主题覆盖**显式分工**并在注释点明（这里：26-5 应用层翻译链 vs 26-2 纯 DDL 约束），避免 reviewer 误判重复造轮子。
> - **反例**：codex 提「放松后丢了 uk_pet_slot 回归」，就把并发 case 成功计数收回 `==1` 或重新加「99 个必须 1009」逐个断言——这是把已被实证 3 次否定的伪不变量重新塞回去，是 over-correction chain 的第 4 跳，必被下一轮翻案 → HALT。
> - **反例**：新增的"确定性"测试里又用 goroutine + barrier 制造并发去"碰运气"命中 UNIQUE——这只是把 flaky 换个地方，没消除不确定性。

## Lesson 2: 双向一致性断言用 INNER JOIN 会把"违反不变量的悬挂行"自己 join 掉而误绿——必须用保留违例行的连接/计数

- **Severity**: medium (P2)
- **Category**: testing
- **分诊**: fix
- **位置**: `server/internal/service/cosmetic_equip_service_integration_test.go:432-435`（`assertEquipStateConsistency` 反向检查）

### 症状（Symptom）

`assertEquipStateConsistency` 反向不变量（每条 `user_pet_equips` 行 ⟹ 对应实例 `status=2`）用 `user_pet_equips upe JOIN user_cosmetic_items uci ON uci.id = upe.user_cosmetic_item_id WHERE uci.status <> 2` 计数。任何 `user_cosmetic_item_id` 错/缺/指向不存在实例的悬挂装备行，在 INNER JOIN 阶段因无匹配被丢弃，COUNT 计不到它 → helper 误报一致。多个 rollback/matrix case 复用此 helper，悬挂行回归全部漏网。

### 根因（Root cause）

要断言的不变量是「**所有** `user_pet_equips` 行都指向一个 `status=2` 的实例」。INNER JOIN 的语义是「只保留两表都有匹配的行」——它先把「没匹配的行」（恰恰是最严重的违例：悬挂/错指向）默默剔除，再在剩下的"良民"里找 `status<>2`。于是「指向不存在实例」这一档违例对 COUNT 完全隐形。一致性/不变量断言里，**违例行往往正是 JOIN 匹配不上的那些行**，用 INNER JOIN 等于让被测对象自证清白。

### 修复（Fix）

INNER JOIN 改 LEFT JOIN + 条件覆盖两类违例：

```sql
-- before（误绿）
SELECT COUNT(*) FROM user_pet_equips upe
 JOIN user_cosmetic_items uci ON uci.id = upe.user_cosmetic_item_id
 WHERE upe.user_id = ? AND uci.status <> 2
-- after（保留违例行）
SELECT COUNT(*) FROM user_pet_equips upe
 LEFT JOIN user_cosmetic_items uci ON uci.id = upe.user_cosmetic_item_id
 WHERE upe.user_id = ? AND (uci.id IS NULL OR uci.status <> 2)
```

LEFT JOIN 保留所有 `user_pet_equips` 行；`uci.id IS NULL` 抓「缺/错指向不存在实例」悬挂行，`uci.status <> 2` 抓「指向存在但非 equipped 实例」悬挂行。正常一致状态每行都 JOIN 到 status=2 实例，两条件皆 false → 计 0，既有正常 case 不误报（已 `build.sh --test` 验证既有路径仍 pass，集成 case 本机无 Docker 不实跑）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **写"每条 A 行都必须对应一个满足条件的 B 行"这类双向/参照一致性断言** 时，**禁止用 INNER JOIN 计违例数**，**必须** **用 LEFT JOIN（含 `B.key IS NULL` 分支）或 `NOT EXISTS` 子查询**，让无匹配/错指向的悬挂行**计入**违例而非被 join 掉。

> **展开**：
> - 判据：违例行经常就是 JOIN 匹配不上的行；INNER JOIN 会先把它们删掉再统计 → 系统性漏检最严重的一档。
> - 正确写法二选一：① `A LEFT JOIN B ON ... WHERE B.key IS NULL OR <B 不满足条件>`；② `A WHERE NOT EXISTS (SELECT 1 FROM B WHERE <匹配且满足条件>)`。两者都把「无匹配」算违例。
> - 双向不变量要正反两个方向各写一遍，且**两个方向都**用保留违例行的写法（本 helper 正向已用 `NOT EXISTS` 正确，问题只在反向的 INNER JOIN）。
> - **反例**：`SELECT COUNT(*) FROM child c JOIN parent p ON p.id=c.parent_id WHERE p.status<>X` 当成「所有 child 都指向 status=X 的 parent」的断言——`parent_id` 悬空/错的 child 全部隐形，断言对最严重的数据损坏睁眼瞎。

---

## Meta: 本次 review 的宏观教训

r1→r4 四轮指向同一类思维盲区的不同侧面：**测试断言/查询编码的"被验证命题"必须与"想验证的不变量"在逻辑上等价，不能因为放松断言、或因为 JOIN 语义、就让被测系统在某档违例下自动通过**。r4 的两条都是「断言看起来在测 X，实际 X 的某子集恒真/被剔除」。收尾范式记住一句：**放松 flaky 强断言时，被放松掉的覆盖用独立确定性测试补回，绝不靠回退强断言；一致性断言永远用"保留违例行"的连接/计数。** 链条上下文见同 story r1/r2/r3 三篇 lesson（concurrent-test-assertions-must-match-system-semantics-26-5-r1 / concurrent-assertion-strength-pingpong-root-cause-is-test-structure-26-5-r2 / concurrent-pseudo-invariant-vs-frozen-contract-swap-semantics-26-5-r3）。
