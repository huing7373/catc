---
date: 2026-05-18
source_review: "file: /tmp/epic-loop-review-26-5-r2.md (codex review, epic-loop r2)"
story: 26-5-layer-2-集成测试-穿戴事务全流程
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-18 — 并发断言强弱 ping-pong 的根因在"测试初始状态/并发结构"层而非"断言"层（26-5 r2）

## 背景

Story 26.5（Layer 2 集成测试 — 穿戴事务全流程）第 2 轮 fix-review。这是一条**已确诊的 over-correction chain**（assertion 强弱 ping-pong，与 Epic 20.9 testing chain、23.5 dev-grant chain 同型）：

- **r1 codex**：「并发1 同槽断言 `==1` 错误，因 `Equip` 是 swap 语义、串行化可多个成功 → 放松断言」→ r1 fix-review 把断言放松成 `successCount >= 1` + 补终态一致性矩阵。
- **r2 codex（本轮）**：「放松成 `>=1` 后不再验证 `uk_pet_slot` 冲突回滚路径，epics.md §26.5 AC 行 3608 钦定"只 1 成功其余 99 error" → 恢复强断言」。

两轮 codex 指导**直接矛盾**。若简单把断言翻回 `==1` 就会再触发 r1 型 finding → 无限 ping-pong。本轮做**根因层解决**：用 setup（空槽）+ 同步屏障把被测路径钉死成确定性的唯一一条，再写与该确定路径精确匹配的强断言 + 守门注释。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 并发1 同槽并发断言被 r1 放松成 `>=1`，不再验证 uk_pet_slot 冲突回滚，违反 §26.5 AC 行 3608 | high (P1) | testing | fix（根因层） | `server/internal/service/cosmetic_equip_service_integration_test.go:771-862` |
| — | 并发2 r1 的 `is_default=i`(0..99) setup 合法性 | — | testing | verified-ok（codex r2 未再 flag，构造合法且 r1 已在码内文档化） | `server/internal/service/cosmetic_equip_service_integration_test.go:889-945` |

## Lesson 1: 并发测试断言强弱之争的根因常在"测试初始状态/并发结构"层，不在"断言"层

- **Severity**: high (P1)
- **Category**: testing
- **分诊**: fix（根因层解决，**非**表层翻转断言）
- **位置**: `server/internal/service/cosmetic_equip_service_integration_test.go:771-862`（函数头守门注释 + 计数断言）

### 症状（Symptom）

同一个并发 case 的成功计数断言在两轮 review 间被反复横跳：r0 写 `==1` → r1 codex 说"swap 语义会多个成功"→ r1 放松成 `>=1` → r2 codex 说"`>=1` 抓不住 uk_pet_slot 回归，AC 钦定恰 1 成功"。两轮 codex 指导互相矛盾，表层翻转断言只会触发对面 reviewer 的同型 finding，进入无限 ping-pong。

### 根因（Root cause）

矛盾的根因**不在断言强弱层**，而在**测试初始状态 + 并发结构层**：

- r1 codex 担心的"串行化 swap 让多个 goroutine 成功"，前提是 `runEquipTx` 步骤 8 能看到一条**已提交的旧 `user_pet_equips` 行**才走"删旧→插新"swap 路径。
- 关键交叉核对：`runEquipTx` 步骤 8 的 `FindByPetSlot`（`user_pet_equip_repo.go:214`）是**普通 `First()` SELECT，无 `FOR UPDATE`**；FOR UPDATE 变体 `FindUserCosmeticItemIDByPetSlotForUpdate` **仅** `runUnequipTx` 步骤 5 用，equip **不**用。
- 本 case 的 setup 把被测路径钉死成确定性的唯一一条：① slot **初始为空**（insertPet 后无任何 prior equip）；② `<-start` 屏障强制 100 goroutine **同时释放（真并发、无错峰）**。① + ② ⟹ 绝大多数 tx 的 MVCC 快照看到"无旧行"⟹ 全部跳过 swap 分支、直奔步骤 9 `InsertInTx` ⟹ `uk_pet_slot` UNIQUE **确定性恰放 1 个 INSERT 过、其余 99 撞 1062** → `ErrUserPetEquipPetSlotDuplicate` → 步骤 9 `Wrap` 成 `ErrServiceBusy(1009)` → 整事务回滚。
- 所以 swap 路径在此 case 结构上**不可达**（只在 slot 非空 或 goroutine 错峰 时可达，两者都被 setup 专门排除）。`successCount == 1` 在服务正确时**不会误失败**（化解 r1 合法担忧）又**能抓 uk_pet_slot 回滚回归**（满足 AC + r2）。r1 把它放松成 `>=1` 是过度修正——它把根因（结构层路径不确定的担忧）错当成断言层问题来"放松"，反而丢了 AC 钦定的回归保护。

思维漏洞：收到"断言太强/太弱"的 review 时，本能地在断言强弱之间调参，而没有先问"被测路径在这个 setup 下是否确定？如果不确定，能否用 setup + 同步屏障把它钉死成确定的那一条？"。断言强弱可调是**结果**，路径不确定才是**根因**。

### 修复（Fix）

根因层一次到位修复（`cosmetic_equip_service_integration_test.go`）：

1. 计数断言恢复为精确 `successCount != 1` → `t.Fatalf`（恰 1 成功）。
2. **额外**断言那 99 个失败是**确切的 uk_pet_slot 重复键回滚错**：复用既有 `requireEquipAppError(t, err, apperror.ErrServiceBusy, ...)` 逐个断言 AppError code == 1009，**不**只数 `err != nil`（证明确实走 uk_pet_slot 回滚路径，而非偶发别的错）。
3. **保留** r1 加的终态一致性矩阵（纯增量价值；仅把"至少 1 行因 slot 空时至少一个成功"措辞收紧为"恰 1 行因唯一赢家 INSERT commit、99 个失败 tx 全回滚"）。
4. 函数头写**守门注释**：列出 (1) slot 初始必空、(2) `<-start` 屏障必保留、(3) FindByPetSlot 无 FOR UPDATE ⟹ swap 路径结构不可达 ⟹ uk_pet_slot 确定性恰 1 成功 / 99 撞 1009 回滚；并明示"谁前置一条 equip 让 slot 非空、或移除屏障让 goroutine 错峰，恰-1 不变量就被 swap 路径破坏——那是改坏测试结构，不是放松断言；想再把 `==1` 放松回 `>=1` 前先反驳上面结构论证"。
5. 加 `fmt` import（断言 ctx 字符串需要）。

并发2 r1 setup：交叉核对 `0003_init_pets.up.sql` —— `is_default TINYINT NOT NULL` **无 CHECK**，仅 `UNIQUE (user_id, is_default)`。r1 的 `is_default=i`(0..99) 同 user 100 pet 物理合法（TINYINT 范围够、复合 UNIQUE 不同值不冲突），r1 已在码内（行 878-888）文档化此推理，codex r2 未再 flag。按约束**不动一个 codex 已不 flag 的稳定构造**（避免新 chain），标 verified-ok。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **并发集成测试遭遇"断言太强/太弱"且该 review 与上一轮 review 的指导方向相反（ping-pong）** 时，**禁止**在断言强弱之间表层翻转，**必须**先交叉核对被测代码的锁/可见性语义（有无 FOR UPDATE、是否 swap/upsert），再用 **setup 初始状态 + 同步屏障** 把被测路径钉死成确定性的唯一一条，然后写**与该确定路径精确匹配的强断言**（含错误码精确断言，非 `err != nil`）+ **守门注释**锁死维持该不变量的结构前提。
>
> **展开**：
> - 识别 over-correction chain 信号：本轮 review 的修复建议与上一轮 fix-review 的 lesson **直接相反**（如 r1 "放松"、r2 "恢复强"）。出现此信号立即升级到根因层，不在表层调断言。
> - 根因定位三问：(a) 被测路径在此 setup 下是否确定？(b) 路径分叉由什么决定（已提交行可见性？锁？错峰？）？(c) 能否用空 setup / 同步屏障把分叉消掉、钉死成确定的那一条？
> - 强断言必须**精确匹配确定路径**：恰 N 成功 + 失败用 `apperror.As` / 既有 `requireXxxAppError` helper 断言确切业务码（如 uk_pet_slot 冲突恒 1009），不接受"只要 err != nil"——那会被任意错误满足，回归抓不住根因。
> - 守门注释是 chain 的终结器：把"维持恰-N 不变量的结构前提"显式写进函数头，并预判未来想再放松的人会怎么改坏（前置 equip / 移屏障），要求他们先反驳结构论证。让下一个 reviewer / Claude 撞上论证而非空白。
> - **反例**：① 收到 r2 "恢复 `==1`" 就直接把 `>=1` 改回 `==1` 了事（必再触发 r1 型 finding → 无限 ping-pong）。② 断言"99 个 err != nil"而不断言确切错误码（一个偶发的 1009-无关错误也能让测试绿，uk_pet_slot 回归漏网）。③ 为消除 r1 已文档化、codex r2 未 flag 的并发2 `is_default=i` 理论瑕疵又去改它（开启新 chain）。④ 删掉 r1 补的终态一致性矩阵当"清理"（那是正交增量价值，不是 chain 的一环）。

---

## Meta: 本次 review 的宏观教训

本轮只 1 条 finding 但它是 chain 第 2 环，宏观教训独立成段：**ping-pong review 的本质是两个 reviewer 各看到根因的一个侧面（r1 看到"路径可能不确定→断言会误判"，r2 看到"放松后丢了 AC 回归")，都对，但都只到断言层。Claude 作为 fix-review 执行者的职责是看见整张事实链、把矛盾下推到根因层（此处=测试结构层）一次解决，并用守门注释固化，让 chain 在此终止。** 横向参考：Epic 20.9 testing chain、23.5 dev-grant-count chain（`2026-05-17-dev-grant-count-is-instance-count-not-distinct-over-correction-chain-23-5-r2.md`）、本 story r1 lesson（`2026-05-17-concurrent-test-assertions-must-match-system-semantics-26-5-r1.md`，over-correction 反例本身）。识别 chain → 根因下推 → 守门注释，是这类 review 的标准三步。
