---
date: 2026-05-17
source_review: "file: /tmp/epic-loop-review-26-5-r1.md (codex review, epic-loop r1)"
story: 26-5-layer-2-集成测试-穿戴事务全流程
commit: 93a1d6e
lesson_count: 2
---

# Review Lessons — 2026-05-17 — 并发集成测试断言必须匹配被测系统真实语义（swap≠互斥）& setup 构造必须满足相关表全部 UNIQUE（26-5 r1）

## 背景

Story 26.5（Layer 2 集成测试 — 穿戴事务全流程）dev-story 在 `cosmetic_equip_service_integration_test.go` 追加 10 个集成 case。codex review r1 命中 2 条 P1：(1) 并发 1 case 的"恰 1 成功 99 失败"断言与 `Equip` 的**同槽换装(swap)**语义冲突，串行化执行时服务正确也会误失败；(2) 并发 2 case 的 setup 循环给同一 user 插 100 个 `is_default=0` 的 pet，违反 `0003_init_pets.up.sql` 的 `uk_user_default_pet (user_id, is_default)` UNIQUE，第 2 条 insert 即 `t.Fatalf`，case 根本到不了被测代码。本轮是 26-5 第 1 轮 fix-review。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 并发 1 断言与同槽换装(swap)语义冲突，串行化时服务正确也误失败 | high (P1) | testing | fix | `server/internal/service/cosmetic_equip_service_integration_test.go:819-820` |
| 2 | 并发 2 setup 违反 `uk_user_default_pet` UNIQUE，case 在被测代码前 t.Fatalf | high (P1) | testing | fix | `server/internal/service/cosmetic_equip_service_integration_test.go:854-856` |

## Lesson 1: 并发集成测试断言"成功计数"前必须先确认被测路径不是 swap/upsert 语义

- **Severity**: high (P1)
- **Category**: testing
- **分诊**: fix
- **位置**: `server/internal/service/cosmetic_equip_service_integration_test.go:819-820`

### 症状（Symptom）

并发 1 case（1 user + 1 pet + 100 件不同实例全 slot=1 → 100 goroutine 各 Equip 不同实例）断言 `successCount == 1`，理由写"DB uk_pet_slot UNIQUE 兜底 → 只 1 成功 99 失败"。该断言在并发请求被串行化执行的环境下，即使服务行为完全正确也会失败。

### 根因（Root cause）

把"DB 有 UNIQUE 约束"直接等同于"N 个并发请求只有 1 个成功"，没有去读被测 service 在命中既存行时走的是哪条路径。`runEquipTx`（`cosmetic_equip_service.go` 步骤 8）是**同槽换装(swap)**：`FindByPetSlot` 是普通 `SELECT ... First`（**无 FOR UPDATE 行锁**），查到既存行就「删旧 user_pet_equips 行 + 旧实例 status 回 1 → INSERT 新行 + 新实例 status=2」。`uk_pet_slot (pet_id, slot)` 只保证**同一时刻该 (pet,slot) 至多 1 行**，**不**保证 99 个请求失败 —— 第一个 tx commit 后，被串行化的后续 goroutine 的 `FindByPetSlot` 会读到既存行 → 走换装路径删旧装自己 → **成功 commit**。所以"成功计数 == 1"是与 swap 语义冲突的弱命题：只有在 100 goroutine 真撞在"slot 仍空"窗口、全部走 INSERT-then-1062 路径时才成立；一旦调度退化为串行（连接池上限 10、慢调度都会触发），多个换装会依次成功，断言误失败。UNIQUE 约束只对"同时只能存在 1 行"负责，对"调用成功次数"不负责；二者是不同命题。

### 修复（Fix）

把 case 改名 `_Concurrent100SamePetSlot_OnlyOneEquips` → `_Concurrent100SamePetSlot_FinalStateConsistent`（名字反映真不变量），断言从"成功计数 == 1"换成**终态一致性矩阵**（epics.md §26.5 行 3613 NFR2 钦定的并发正确性属性）：

- 成功数 **>= 1**（slot 空 → 至少一个 equip 必占住 slot；不断言 == 1）
- `user_pet_equips WHERE pet_id=?` 恰 **1 行** + `(pet,slot=1)` 恰 1 行（uk_pet_slot 兜底，无脏写/无多行）
- 恰 **1 个实例 status=2** + 其余 N-1 个 `status=1` + 全部 N 个 `status IN (1,2)`（无实例卡 consumed/invalid 中间态 → 无部分提交）
- 现存装备行指向的实例**正是**那个唯一 status=2 实例（行↔状态对齐 JOIN 断言）
- `assertEquipStateConsistency` 双向一致（NFR2）

同步更新 story 文件 AC7 / 映射表 / 完成清单 / 文件头表格 4 处函数名与不变量描述。**并发 2** case 的"恰 1 成功"断言**不**改（保持 fix）—— 不同 pet 各自 slot 空，`FindByPetSlot(petᵢ, slot)` 全 NotFound → 全走 INSERT，撞的是单列 `uk_user_cosmetic_item_id`，无任何 swap-by-delete 路径，故串行化的后续 goroutine INSERT 同一 user_cosmetic_item_id 必撞 1062 → "恰 1 成功"在此 case 真成立。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写**并发集成测试断言**时，**禁止**直接断言"恰 1 个调用成功 / N-1 个失败"，**必须**先读被测 service 在"目标资源已存在"分支走的是 INSERT-only 还是 swap/upsert/delete-then-insert；只有确认是 INSERT-only 且约束在被并发写的那一列上才可断言成功计数，否则一律断言**终态一致性矩阵**。
>
> **展开**：
> - "DB 有 UNIQUE 约束" ⟹ "同时刻至多 1 行"，**不** ⟹ "N 个并发请求只 1 个成功"。后者还取决于 service 命中既存行时是报错还是换装/覆盖。
> - 并发正确性的稳健不变量是**任意串行化顺序下的终态一致性**（行数 / status 分布 / 无中间态 / 行↔状态对齐 / 双向无孤儿），不是调用成功计数 —— 成功计数依赖调度时序，是 flaky 命题。
> - 测试函数名要反映被断言的不变量（`_FinalStateConsistent` 而非 `_OnlyOneEquips`）；改名后必须全仓同步 story AC 引用 + 文件头映射表，保持名实一致。
> - **反例**：看到 `uk_pet_slot UNIQUE(pet_id,slot)` 就写 `if successCount != 1 { t.Fatalf(...) }`，却没注意 `runEquipTx` 步骤 8 是"查到旧行→删旧→装新"的 swap —— 这会在连接池受限/慢调度（CI 常态）下，服务完全正确时随机红，触发 over-correction：下一轮有人把断言再改强/改弱来回横跳（参照 20.9 testing chain 7 轮收敛史）。

## Lesson 2: 并发/批量 setup 循环造数据前必须枚举目标表全部 UNIQUE/CHECK，确认每条 INSERT 合法

- **Severity**: high (P1)
- **Category**: testing
- **分诊**: fix
- **位置**: `server/internal/service/cosmetic_equip_service_integration_test.go:854-856`

### 症状（Symptom）

并发 2 case 的 setup `for i:=0;i<100;i++ { insertPet(t, rawDB, petIDs[i], userID, 1, "并发猫", 1, 0) }` —— 同一 `userID` 插 100 个 `is_default=0` 的 pet。`0003_init_pets.up.sql` 有 `UNIQUE KEY uk_user_default_pet (user_id, is_default)`，第 2 条 insertPet 即撞 1062，`insertPet` 内 `t.Fatalf`，整个 case 在跑到被测 `svc.Equip` 之前就挂，AC8 的 DB UNIQUE 兜底从未被实际验证。

### 根因（Root cause）

写"造 N 行测并发"的 setup 时只关注被测的那个 UNIQUE（`uk_user_cosmetic_item_id`），没有枚举 setup 所写表（pets）自身的全部约束。`uk_user_default_pet (user_id, is_default)` 限制了"同一 user + 同一 is_default 值"只能有 1 行；硬编码 `is_default=0` 让 100 行全落同一组合 → 必撞。同时也没意识到：本 case 因 `runEquipTx` 步骤 4/6 校验"实例归属 + pet 归属均须 == in.UserID"，单实例单 owner ⟹ 100 个 pet **必属同一 user**，所以不能简单"改 100 个 user"绕过（那会让 100 个 equip 全 5002，case 同样跑不到 UNIQUE 兜底）。

### 修复（Fix）

保持**同一 user**（满足 equip 归属校验），把 `is_default` 改成每个 pet 各不相同：`insertPet(t, rawDB, petIDs[i], userID, 1, "并发猫", 1, i)`（i = 0..99）。schema 合法性依据：`pets.is_default` 是 `TINYINT NOT NULL` **无 CHECK 约束**（DDL 注释"MVP 阶段取值 0/1"是业务约定，非 DB 约束，物理可存 0..99），UNIQUE 在复合列 `(user_id, is_default)` 上，同 user 不同 is_default 值不冲突。100 个 pet 因此全部合法存在，case 得以跑到 `svc.Equip` 真正验证 `uk_user_cosmetic_item_id` 兜底。这与既有测试文件自身注释"pet 表 1 user N pet 物理可建，与节点 9 业务约束正交"一脉相承。同步更新 story 文件 setup 描述 2 处。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 写"循环造 N 行做并发/批量测试"的 setup 时，**必须**先打开目标表的 migration DDL 把它的**每一个** UNIQUE / 复合 UNIQUE / CHECK / NOT NULL 列出来，逐条确认这 N 行**每一行**都不会撞任何约束；并且**禁止**用"减少循环次数 / 改归属 user"这类绕过手段，要找出**符合所有约束的合法构造**。
>
> **展开**：
> - 造数据前 `Read` 目标表 `migrations/*.up.sql`（不止被测的那个约束），枚举全部 UNIQUE/复合 UNIQUE/CHECK。复合 UNIQUE `(a, b)` 意味着"同 a 同 b"才冲突 —— 可以靠让 b 各异来合法造多行（前提是 b 列无 CHECK 限值域）。
> - DDL 注释里的"MVP 阶段取值 0/1"这类**业务约定**不是 DB 约束；只要列类型 + 真实约束允许，测试可用业务约定外的合法值构造数据（且要在测试注释里写清依据，避免后人误判为越界）。
> - setup 修复必须让构造**真合法**（满足所有相关表约束 + 不破坏被测路径的前置如归属校验），不是"少造几行""换个 user"把症状盖住。换 user 前先想清楚被测 service 有没有 owner 校验会因此走进别的错误分支。
> - **反例**：`for i:=0;i<100;i++ { insertPet(..., userID, ..., is_default=0) }` 在有 `UNIQUE(user_id, is_default)` 的表上 —— 第 2 行即 `t.Fatalf`，case 永不可达被测代码，且 review 不细看会以为测试通过。

---

## Meta: 本次 review 的宏观教训

两条 finding 同源：**测试断言/构造没有以"被测系统的真实语义 + 真实 schema 约束"为锚，而是凭对约束作用的直觉**。Finding 1 把"DB UNIQUE"直觉成"只 1 成功"却没读 service 的 swap 路径；Finding 2 把"造 100 行"直觉成"循环 insert"却没读 pets 表的复合 UNIQUE。集成测试的价值正在于"贴真实现 + 真 DB"，所以写集成测试断言/ setup 前的强制动作是：(a) 读被测函数命中既存资源的分支语义；(b) 读所写表全部 migration 约束。跳过这两步写出的"成功计数断言"和"循环 setup"在 CI 真 DB + 受限连接池下会随机红或永不可达，正是触发 testing over-correction chain 的高发源（参照 Epic 20.9 跑了 7 轮断言强弱横跳才靠"责任分离 + 锚定真语义"收敛）。本轮一次性根因解决（终态一致性矩阵 + 合法 setup 构造），不留弱断言/绕过手段给后续 chain。
