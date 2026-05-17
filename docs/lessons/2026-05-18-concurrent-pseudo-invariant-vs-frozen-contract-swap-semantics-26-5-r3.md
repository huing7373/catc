---
date: 2026-05-18
source_review: "file: /tmp/epic-loop-review-26-5-r3.md (codex review, epic-loop r3)"
story: 26-5-layer-2-集成测试-穿戴事务全流程
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-18 — 并发断言 over-correction chain 的终结根因：断言编码的不变量与契约语义（swap≠互斥）不符，靠实证 + 契约语义层收敛（26-5 r3）

## 背景

Story 26.5（Layer 2 集成测试 — 穿戴事务全流程）第 **3 轮** fix-review，是一条**已跑满 3 轮、本轮靠 codex 决定性实证收敛**的 over-correction chain（assertion 强弱 ping-pong，与 Epic 20.9 testing chain、23.5 dev-grant chain 同型，但本条是 chain 的**终结环 + 反例集大成**）。完整链条：

- **r0**（dev-story）：并发1 同 pet 同 slot 100 并发 equip → 写 `successCount == 1`，理由「DB UNIQUE(pet,slot) 兜底 → 只 1 成功 99 失败」（直接抄 epics.md §26.5 AC 行 3608 措辞）。
- **r1 codex**：「`==1` 错，`Equip` 是 swap 语义可串行多成功 → 放松」→ r1 fix-review 放松成 `successCount >= 1` + 加终态一致性矩阵（**方向本来对**）。
- **r2 codex**：「放松成 `>=1` 丢失 uk_pet_slot 回滚回归 → 恢复强断言」→ r2 fix-review（被一个**错误的根因指令**误导）强制恢复 `successCount == 1` + 加 99 个「失败必为 1009」逐个断言 + 写一段**事后被证伪的守门注释**（声称"slot 空 + `<-start` 屏障 ⟹ swap 路径结构不可达 ⟹ 确定性恰 1"）。
- **r3 codex（本轮）**：**实跑复现**——`go test ./internal/service -tags=integration -count=3 -run ...Concurrent100SamePetSlot...` **报告 91 个成功 equip**（不是 1）。实证证明：屏障只同步 goroutine **启动**、不同步各事务读 `user_pet_equips` 的**时刻**；首个 tx commit 后其余 ~99 个 goroutine 的 `FindByPetSlot`（普通 `First`，**无 FOR UPDATE**）渐次看到那条已提交行 → 走 swap 删旧插新 → 串行化合法成功。**r2 的"swap 结构不可达"模型被实证证伪。**

本轮按 codex r3 实证 + 26-1 冻结契约 §8.3 swap 语义一次性收敛终结 chain。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 并发1 `successCount == 1` 是 swap 语义下的伪不变量；codex r3 实跑复现 91/100 合法成功，r2 强断言 + 99 个 1009 逐个断言会在 CI 误红/flaky | high (P1) | testing | **fix**（根因层收敛，终结 3 轮 chain） | `server/internal/service/cosmetic_equip_service_integration_test.go:772-918` |
| — | epics.md §26.5 AC 并发1（行 3608）"只 1 成功其余 99 error" 措辞 vs 26-1 冻结契约 §8.3 swap 语义冲突 | high (P1) | docs | **wontfix-with-rationale**（AC 文本被 26-1 冻结契约 supersede；不改 AC/不改冻结契约/不改服务，story Debug Log 登记留痕） | `epics.md:3608` / story 26-5 Debug Log |

## Lesson 1: 并发断言 over-correction chain 的终结根因不是断言强弱、也不是测试结构，而是"断言所编码的不变量与被测系统的契约语义不符"

- **Severity**: high (P1)
- **Category**: testing
- **分诊**: fix（根因层一次收敛，**非**第 4 跳 ping-pong）+ AC 偏差 wontfix-with-rationale 登记
- **位置**: `server/internal/service/cosmetic_equip_service_integration_test.go:772-918`（函数头守门注释 + 成功数断言 + 终态矩阵注释）

### 症状（Symptom）

同一并发 case 的成功计数断言跨 3 轮 review 强弱横跳：`==1`(r0) → `>=1`(r1) → `==1`+99 个 1009 逐个断言+"swap 不可达"守门注释(r2) → codex r3 实跑 `-count=3` 报告 **91/100 成功**，证伪 r2 的"结构不可达"论证。r2 的强断言在服务**完全正确**时也会让 CI 随机红（91 ≠ 1），且 99 个「失败必为 1009」逐个断言与 91 个 `err==nil` 实证直接冲突、必 flaky。

### 根因（Root cause）

前两轮（含 r2 的"测试结构层"诊断）都没触到最深的根因。chain 的真根因是**断言所编码的不变量与被测系统的契约语义不符**：

- `successCount == 1` 编码的命题是「100 个 equip 调用里恰 1 个成功」≈「equip 调用之间互斥」。
- 但 26-1 冻结契约 §8.3 + 26-3 实装把 equip 钦定为**同槽自动换装（swap）**：查旧 `user_pet_equips` 行 → 删旧行 + 旧实例回 `in_bag` → 插新行 + 新实例 `equipped`（client **无需**先 unequip）。`runEquipTx` 步骤 8 `FindByPetSlot` 是普通 `First()` SELECT（**无 FOR UPDATE** —— r2 已核实但据此推出了错误结论）。
- swap 语义下，**串行化的后续 equip 设计上就该成功**：首个 tx commit 后，其余 goroutine 的事务陆续读到那条已提交旧行 → 走 swap 删旧装自己 → 合法 commit。`<-start` 屏障只同步 goroutine **启动**，**不**同步各事务执行 `FindByPetSlot` 的**时刻**（连接池上限/调度抖动让读时刻渐次错开）—— 这是 r2 "屏障 ⟹ swap 不可达"模型的致命漏洞，被 codex r3 实证（91/100）击穿。
- epics.md §26.5 AC 并发1（行 3608）"只 1 成功其他 99 error"措辞，是基于 **26-1 冻结契约之前的过时心智模型**（误以为 equip 是 insert-only、靠 uk_pet_slot 拒绝重复 INSERT）。AC 括注"DB UNIQUE(pet,slot) 兜底"真正保证的是「**任意时刻至多 1 行 / 终态恰 1 行**」（= 终态一致性矩阵在断言的），**不是**"99 个调用失败"。AC 文本被 26-1 冻结契约 supersede。

思维漏洞链（逐轮加深）：r0「把 DB UNIQUE 直觉成调用互斥」→ r1/r2「收到'断言太强/太弱'就在断言强弱层调参（r1）或往测试结构层归因再造确定性论证（r2），但论证未经实证就当成 ground truth 写进守门注释强制断言方向」→ 共同的最深漏洞：**没有回到契约/AC 语义层确认"被测系统设计上保证什么"，并识别 AC 文本本身可能基于过时心智模型**。r2 的"结构不可达"是一个**未经实证的理论模型**，把它写进守门注释强制 `==1` 是 chain 第 3 跳的直接成因。

### 修复（Fix）

根因层一次收敛（`cosmetic_equip_service_integration_test.go`，**仅测试，不碰生产代码/冻结契约/AC 文档** —— 26-5 范围红线）：

1. 成功数断言 `successCount != 1` → **`successCount < 1`**（slot 空 ⟹ 至少一个 equip 必成功占住 slot；swap 语义下成功数 ∈ [1, N] 均合法，**不**设上界）。
2. **删除** r2 的 99 个 `requireEquipAppError(t, err, apperror.ErrServiceBusy, ...)` 逐个断言（实证表明绝大多数后续调用是 swap **成功**，不是 1009；保留必与 91/100 冲突、必 flaky）。失败 goroutine（若有）不再断言具体错误码（swap 竞争下失败原因可能是行锁等待超时/重复键 等多种合法竞争结果，不构成回归信号）。
3. 随之 `fmt` 包变为 unused（r2 仅为 `fmt.Sprintf` 断言 ctx 引入）→ 同步删 `"fmt"` import（否则 `go vet -tags=integration` 编译失败）。
4. **保留**终态一致性矩阵（r1 加 / r2 留的 4 类不变量：(pet,slot) 终态恰 1 行 / 恰 1 status=2 / 其余 N-1 status=1 / status∈{1,2} 无中间态 / 行↔状态对齐 JOIN / `assertEquipStateConsistency` 双向一致）—— **这才是 swap 语义下真正的并发正确性核心不变量**。uk_pet_slot 兜底由"终态恰 1 行 + 无脏写 + 无孤儿"覆盖，不需"99 调用失败"来证。仅把矩阵头注释里 r2 残留的"99 个失败 tx 全回滚 / 唯一赢家 INSERT"insert-only 措辞改写为 swap 语义表述。
5. **删除** r2 那段被证伪的守门注释（"空槽+屏障⟹swap 不可达⟹确定性恰 1"整段），**替换为**引用 codex r3 实证（`-count=3` 复现 91/100）的正确版：讲清 swap 语义 + 屏障只同步启动不同步事务读时刻 + 本 case 真不变量是终态矩阵不是成功计数 + 完整 3 轮 chain 复盘 + **明确警告未来维护者不要把成功数断言再收紧回 `==1`（伪不变量，已被实证 3 次否定，再设回 = 第 4 跳）**。
6. **AC 偏差正式登记**（wontfix-with-rationale，不静默改测试糊弄）：story 26-5 文件 Debug Log References 补一条 —— epics.md §26.5 AC 行 3608"只 1 成功其余 99 error"与 26-1 冻结契约 §8.3 swap 语义冲突（codex r3 实证 91/100 合法成功），本 story 按冻结契约语义实现并发1 断言（终态一致性而非调用计数互斥），AC 措辞视为被 26-1 冻结契约 supersede；不改 AC 文档/不改冻结契约/不改服务，如实留痕。

验证（本机无 Docker 不实跑集成执行，codex 环境已实证 91/100）：`go vet -tags=integration ./internal/service/` exit 0 + `go test -tags=integration -list 'TestCosmeticEquipServiceIntegration.*'` 仍 12 case 注册 + `bash scripts/build.sh --test` BUILD SUCCESS。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 遇到**并发测试断言强弱反复横跳（over-correction chain，本轮 review 指导与上轮 fix-review lesson 相反）**时，**禁止**在断言强弱层调参、**也禁止**仅停在"测试结构层"造一个**未经实证的确定性论证**就强制断言方向；**必须**先回到**契约/AC 语义层**确认"被测系统设计上保证什么"（如 swap ⟹ 终态至多 1 行 ✓ / 调用级互斥 ✗），用**与契约语义精确匹配**的不变量断言，并主动识别 **AC/spec 文本本身可能基于过时心智模型**（被后续冻结契约 supersede）。能实跑就**先实跑取实证**再下结论。
>
> **展开**：
> - chain 收敛的正确顺序：(a) 看本轮 review 是否带**实证**（codex r3 给了 `-count=3` → 91/100，实证 > 任何理论模型，立即以实证为最高 ground truth）；(b) 回契约/冻结契约/AC 找"系统设计上保证什么"的**语义**（26-1 §8.3 swap）；(c) 比对当前断言编码的命题与该语义是否一致（`==1`≈互斥 vs swap≈终态至多 1 行 → 不一致，断言错）；(d) 用与语义精确匹配的不变量替换（终态一致性矩阵），成功计数只断言契约真保证的下界（`>= 1`）。
> - **"未经实证的理论确定性论证"是 chain 的燃料，不是灭火器**。r2 写"屏障 ⟹ swap 结构不可达 ⟹ 确定性恰 1"并把它当守门注释强制 `==1`，正是第 3 跳成因。任何"我推断这个 setup 会确定性收敛到唯一路径"的论证，**写进守门注释强制断言前必须有实跑实证或并发原语级证明**（屏障同步的是 goroutine 启动 ≠ 事务内某条 SQL 的执行时刻 —— 这个区分 r2 漏了）。
> - **AC/spec 文本不是不可错的 ground truth**。当 AC 措辞与同 Epic 的冻结契约/已实装行为冲突时，识别 AC 可能基于**过时心智模型**（此处：26-1 冻结契约把 equip 从 insert-only 改判为 swap，§26.5 AC 行 3608"99 失败"措辞写于此之前）。处置是 **wontfix-with-rationale + 在 story 文件正式登记 AC 偏差**（不盲改 AC 文档、不改冻结契约、不改服务、不静默改测试糊弄），如实留痕给后人。
> - 终态一致性矩阵（任意串行化顺序后 DB 行数/状态分布/无孤儿/双向一致）是 swap/upsert 语义下的**核心真不变量**；调用成功计数是依赖调度时序的 flaky 命题。chain 里每一轮"保留终态矩阵"的决定都对，"调成功计数强弱"的决定都错。
> - **反例**（本 chain 反例集，蒸馏负例）：① r0 看到 `UNIQUE(pet,slot)` 就写 `successCount == 1`（把 DB 约束直觉成调用互斥，没读 §8.3 swap）。② r2 收到"恢复强断言"指令，造一个"屏障⟹swap 不可达"的**未实证理论模型**写进守门注释强制 `==1` + 加 99 个 1009 逐个断言（理论模型未经实证就当 ground truth；被 codex r3 `-count=3`=91/100 当场击穿）。③ 任何后续把 `>=1` 再收紧回 `==1`（伪不变量，已被实证 3 次否定，= 第 4 跳）。④ 删掉终态一致性矩阵当"清理"（那是正交核心真不变量，不是 chain 的一环）。⑤ 静默改测试断言"过关"而不在 story 文件登记 AC 与冻结契约的偏差（掩盖矛盾，后人重踩）。

---

## Meta: 本次 review 的宏观教训

本轮是 26-5 chain 的**终结环**，也是 testing over-correction chain 的范式总结：**ping-pong 的本质不是"断言该强还是该弱"，而是"断言编码的不变量与被测系统的契约语义错位"——每一轮 reviewer 各看到错位的一个侧面（r1：路径不确定→断言会误判；r2：放松后丢 AC 回归），都对一半，但都没把矛盾下推到"契约语义层 + AC 文本可信度"这一最深层；r2 更进一步犯了"用未经实证的理论模型强制断言方向"的错。终结 chain 的三件武器：(1) 实证优先（codex r3 的 91/100 实跑 > 任何理论推导）；(2) 回契约/冻结契约语义层确认系统设计保证什么，并审视 AC 文本是否被后续冻结契约 supersede；(3) 用 wontfix-with-rationale 在 story 文件如实登记 AC 偏差，不盲改文档/契约/服务、不静默改测试。横向参考：Epic 20.9 testing chain（7 轮断言强弱横跳）、23.5 dev-grant-count chain（`2026-05-17-dev-grant-count-is-instance-count-not-distinct-over-correction-chain-23-5-r2.md`）、本 story r1 lesson（`2026-05-17-concurrent-test-assertions-must-match-system-semantics-26-5-r1.md`，方向本对的一环）、本 story r2 lesson（`2026-05-18-concurrent-assertion-strength-pingpong-root-cause-is-test-structure-26-5-r2.md`，**特别点名为"理论模型未经实证就强制断言方向"的反例本身**——其"swap 结构不可达"守门注释被本轮 codex r3 实证证伪）。识别 chain → 取实证 → 回契约语义层 + 审 AC 可信度 → wontfix-with-rationale 登记，是这类已跑多轮 chain 的标准收敛四步。
