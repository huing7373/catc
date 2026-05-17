# Story 26.5: Layer 2 集成测试 — 穿戴事务全流程（dockertest 真实 MySQL 在 26.3/26.4 已落地 service 层集成测试基础上**追加** epics.md §26.5 行 3603-3615 钦定 12 类场景：完整流程 / 同槽换装 / 3 回滚 / 2 并发(100 goroutine) / 3 边界 / 1 状态一致性矩阵；**不**实装新业务功能，仅扩展 integration test 覆盖矩阵）

Status: review

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 资产事务负责人,
I want 一组深度集成测试覆盖**穿戴 / 卸下事务**（POST /cosmetics/equip 同槽换装 + POST /cosmetics/unequip 解绑）的失败回滚 / 100 goroutine 并发 / DB UNIQUE 兜底 / 状态一致性矩阵全部场景，全部用 dockertest 真实 MySQL 跑通，作为节点 9 Server 端 Layer 2 集成测试收尾保障，**追加** 到 Story 26.3 已落地的 `TestCosmeticEquipServiceIntegration_EquipAndSwapSameSlot`（equip happy + 同槽换装）+ Story 26.4 已落地的 `TestCosmeticEquipServiceIntegration_UnequipHappyPath`（unequip happy + 重复 unequip 5004）2 个 service 层集成测试基础上，把覆盖率从局部 happy / swap 路径推到"事务全失败模式 + 高并发 DB 约束兜底 + 状态一致性不变量矩阵"三个维度全绿,
so that NFR1（资产事务原子）/ NFR2（穿戴关系一致性约束：所有 status=2(equipped) 的实例必然在 user_pet_equips 中有对应行，反之亦然）/ NFR11 / `docs/宠物互动App_V1接口设计.md` §8.3 / §8.4（穿戴 / 卸下事务）/ `docs/宠物互动App_数据库设计.md` §5.10（uk_pet_slot + uk_user_cosmetic_item_id 两个 UNIQUE 约束）在节点 9 阶段不只靠 26.3 / 26.4 已有的 2 条 happy / swap / unequip case，而是**穷举** epics.md §Story 26.5 行 3603-3615 钦定的 1 完整流程 + 1 同槽换装 + 3 回滚（equip 删旧装备失败 / equip 更新实例 status 失败 / unequip 最后一步失败）+ 2 并发（同 pet 同 slot 100 并发 equip → DB UNIQUE(pet_id,slot) 兜底只 1 成功 / 同实例 100 并发 equip → DB UNIQUE(user_cosmetic_item_id) 兜底只 1 成功）+ 3 边界（equip consumed 实例 5003 / equip 非本人实例 5002 / unequip 空 slot 5004）+ 1 状态一致性矩阵共 **12 类场景**，把覆盖率从 happy 路径推到事务全失败模式 + 高并发 DB 约束兜底 + status↔user_pet_equips 双向一致性 3 个维度全绿；任何一个场景退化（如某条回滚路径漏 rollback 留下脏的 equipped 实例 / 100 goroutine 同槽 equip 出现 2 行 user_pet_equips / 状态一致性矩阵出现 status=2 但无 user_pet_equips 行的孤儿）→ 立即在 Layer 2 阶段被发现，**不**让节点 9 验收 demo 阶段（Story 28.1 跨端 E2E）才暴露穿戴事务原子性 / DB UNIQUE 兜底 / status 一致性回归。

## 故事定位（Epic 26 第五条 = 节点 9 Server 收尾性 Layer 2 集成测试；上承 26.3 equip 事务 + 26.4 unequip 事务；下启 26.6 GET /home pet.equips 真实数据）

- **Epic 26 进度**：26.1（接口契约最终化 §8.3/§8.4/§1 节点 9 冻结，**done**）→ 26.2（user_pet_equips migration + `UserPetEquip` GORM struct + 0016 schema 含 uk_pet_slot / uk_user_cosmetic_item_id 两 UNIQUE，**done**）→ 26.3（POST /cosmetics/equip 事务，含同槽换装；落地 `CosmeticEquipService.Equip` / `runEquipTx` / `UserPetEquipRepo` `FindByPetSlot`+`DeleteByPetSlotInTx`+`InsertInTx` / `cosmetic_equip_service_integration_test.go` `TestCosmeticEquipServiceIntegration_EquipAndSwapSameSlot`，**done**）→ 26.4（POST /cosmetics/unequip 事务；落地 `CosmeticEquipService.Unequip` / `runUnequipTx` / `UserPetEquipRepo` `FindUserCosmeticItemIDByPetSlotForUpdate`+`DeleteByPetSlotInTxReturningAffected` / `TestCosmeticEquipServiceIntegration_UnequipHappyPath`，**done**）→ **26.5（本 story，Layer 2 集成测试 - 穿戴事务全流程）** → 26.6（GET /home 扩展 - pet.equips 真实数据，之前节点 2 返回 `[]`）。

- **物理执行顺序与逻辑编号一致**：本 story 编号 26.5，物理上**第五**执行（26.1-26.4 done 后立刻做 26.5）。理由：
  - Story 26.5 是 epic-26 的**收尾性 Layer 2 集成测试**，需要 26.3（Equip）+ 26.4（Unequip）两条业务链路 + 26.2（0016 user_pet_equips schema 两 UNIQUE 约束）全部落地后再做整体回归
  - sprint-status.yaml 第 254 行已按此顺序排列（26-5 在 26-4 之后、26-6 之前）
  - 26.5 是测试 story，**不实装新业务功能**，仅扩展 integration test coverage 矩阵；与 4.7（auth_service Layer 2 收尾）/ 11.9（room_service Layer 2 收尾）/ 20.9（chest_open 事务 Layer 2 收尾）/ 32.5（合成事务 Layer 2 收尾）同模式，**Story 20.9 的 4 个 fault injection wrapper 范式（按方法包装真实 mysql repo + 指定方法替换为 injectErr）是本 story 回滚 case 的直接参照**（20.9 §下游依赖行 83 已显式点名 "Story 26.5 穿戴事务集成测试钦定相同 Layer 2 模式"）

- **epics.md §Story 26.5 钦定**（`_bmad-output/planning-artifacts/epics.md` 行 3592-3616，**唯一权威 AC 来源**）：
  - **Given** Story 26.3 / 26.4 happy path 已通过
  - **When** 完成本 story
  - **Then** 输出 / 扩展 `internal/service/cosmetic_equip_service_integration_test.go`（已存在，26.3 落地 `TestCosmeticEquipServiceIntegration_EquipAndSwapSameSlot` + 26.4 落地 `TestCosmeticEquipServiceIntegration_UnequipHappyPath` 共 2 个测试函数 + `buildCosmeticEquipServiceIntegration` helper），**追加** 12 类场景（**不**新建独立测试文件 —— 与 4.7 / 11.9 / 20.9 同模式，同包 `service_test` 同文件内聚，复用既有 `buildCosmeticEquipServiceIntegration` / `startMySQL` / `runMigrations` / `insertUser` / `insertPet` / `insertUserCosmeticItem` / `cosmeticIDByCode` / `assertCount` helper）：

    | epics.md 行 | 场景类别 | 详细要求 |
    |---|---|---|
    | 行 3603 | **完整流程** | 创建 user + pet + 5 件不同 cosmetic 实例（不同 slot）→ 依次穿戴到 5 个槽位 → 验证 user_pet_equips 5 行 + 5 个实例 status=2 |
    | 行 3604 | **同槽换装** | 同 slot 已有 hat A，穿 hat B → 验证 A status 回 1 + B status 变 2 + user_pet_equips 行更新（**不是新增**，净行数不变）|
    | 行 3605 | **回滚 1**（equip 删旧装备失败）| equip 事务 mock 第 2 步（删旧装备 `DeleteByPetSlotInTx`）失败 → 验证旧装备仍 equipped(2) + 新装备仍 in_bag(1) + user_pet_equips 不变（指向旧实例）|
    | 行 3606 | **回滚 2**（equip 更新实例 status 失败）| equip 事务 mock 最后一步（更新当前实例 status=equipped）失败 → 验证 user_pet_equips 行也回滚（INSERT 的新行不存在）|
    | 行 3607 | **回滚 3**（unequip 最后一步失败）| unequip 事务 mock 最后一步（更新实例 status 回 in_bag）失败 → 验证 user_pet_equips 行未删（仍存在 + 实例仍 equipped(2)）|
    | 行 3608 | **并发 1**（同 pet 同 slot 100 并发 equip 不同实例）| 同一 pet 同一 slot 100 个并发 equip 不同实例 → 只 1 个成功，其他 99 个返回错误（DB `uk_pet_slot` UNIQUE(pet_id, slot) 兜底）|
    | 行 3609 | **并发 2**（同一实例 100 并发 equip 到不同 pet）| 同一实例 100 个并发 equip 到不同 pet（理论不发生因 1 user 1 pet，但测一致性约束）→ 只 1 个成功（DB `uk_user_cosmetic_item_id` UNIQUE(user_cosmetic_item_id) 兜底）|
    | 行 3610 | **边界 1**（equip 实例 status=consumed）| equip 实例 status=consumed(3) → 5003 |
    | 行 3611 | **边界 2**（equip 实例不属于当前用户）| equip 实例不属于当前用户 → 5002 |
    | 行 3612 | **边界 3**（unequip 不存在的 slot）| unequip 不存在的 slot → 5004 |
    | 行 3613 | **状态一致性** | 任意操作后，所有 status=2(equipped) 的实例必然在 user_pet_equips 中有对应行；反之亦然（NFR2 一致性约束）—— 在完整流程 + 同槽换装 + 回滚后均断言此双向不变量 |

  - 全部场景用 dockertest 真实 MySQL 跑通（**不**用 sqlmock —— 业务上是 Layer 2 黑盒事务行为验证，不是 SQL 字符串匹配；并发兜底依赖真实 InnoDB UNIQUE 索引 X-lock + 真实事务回滚，sqlmock 无法验证）
  - 集成测试在 CI 标 `//go:build integration` + `// +build integration` 双行 tag（与 26.3 / 26.4 / 20.9 / 11.9 / 4.7 同模式）

- **范围边界**（**关键** —— 与 26.3 / 26.4 已落地集成测试的明确分工）：

  **26.3 / 26.4 service 层集成测试已落地 2 case**（`server/internal/service/cosmetic_equip_service_integration_test.go`，全部 done；通过 `grep -n "func TestCosmeticEquipServiceIntegration_"` 列举）：
  - `TestCosmeticEquipServiceIntegration_EquipAndSwapSameSlot`（26.3 AC6）— equip 第 1 件 hat（slot 空 → 直接装上，user_pet_equips 1 行 + 实例 status=2）+ equip 第 2 件 hat（同 slot=1 → 同槽换装，user_pet_equips 仍 1 行 + 旧 hat status 回 1 + 新 hat status=2）
  - `TestCosmeticEquipServiceIntegration_UnequipHappyPath`（26.4 AC6）— Equip 装 hat → Unequip(petId, slot=1) → user_pet_equips 0 行 + 实例 status=1 + Unequipped=true；再次 Unequip 同空槽 → 5004（非幂等）+ DB 状态不变

  **本 story 任务是扩展上述文件追加 ≥10 个新测试函数**（追加到同一份 `cosmetic_equip_service_integration_test.go`，**不**新建独立测试文件 —— 与 4.7 / 11.9 / 20.9 同模式同包同文件内聚，复用 26.3 落地的 `buildCosmeticEquipServiceIntegration` + startMySQL / runMigrations / insertUser / insertPet / insertUserCosmeticItem / cosmeticIDByCode / assertCount helper）：

  | epics.md 钦定场景 | 测试函数命名 | 与既有 case 关系 |
  |---|---|---|
  | 完整流程（行 3603）| `TestCosmeticEquipServiceIntegration_FullFlow_Equip5SlotsAll` | **新增**（5 slot 全装；含状态一致性矩阵断言）|
  | 同槽换装（行 3604）| 复用 26.3 `EquipAndSwapSameSlot` 场景 2 | **复用 + 文档化对应关系**（不新增；本 story 在文件顶部注释指向已落地 case + 在新增矩阵 case 中复跑同槽换装后断言一致性）|
  | 回滚 1（equip 删旧装备失败）| `TestCosmeticEquipServiceIntegration_EquipDeleteOldEquipFails_AllRollback` | **新增**（fault inject `UserPetEquipRepo.DeleteByPetSlotInTx`，前置 slot 已有旧装备）|
  | 回滚 2（equip 更新实例 status 失败）| `TestCosmeticEquipServiceIntegration_EquipUpdateStatusFails_AllRollback` | **新增**（fault inject `UserCosmeticItemRepo.UpdateStatusInTx` 在最后一步，slot 空 → INSERT 后 status 更新失败 → user_pet_equips 新行回滚）|
  | 回滚 3（unequip 最后一步失败）| `TestCosmeticEquipServiceIntegration_UnequipUpdateStatusFails_AllRollback` | **新增**（fault inject `UserCosmeticItemRepo.UpdateStatusInTx`；前置已装一件 → unequip 最后一步失败 → user_pet_equips 行未删 + 实例仍 equipped）|
  | 并发 1（同 pet 同 slot 100 并发 equip）| `TestCosmeticEquipServiceIntegration_Concurrent100SamePetSlot_FinalStateConsistent` | **新增**（与 20.9 _Concurrent100SameKey / 11.9 _Concurrent100DifferentUsers 同 goroutine 范式；100 实例 → 同 pet 同 slot；equip 是同槽换装 swap 语义 → 断言**终态一致性矩阵**而非"恰 1 成功"，详见 AC7；fix-review 26-5 r1 [P1]）|
  | 并发 2（同实例 100 并发 equip 不同 pet）| `TestCosmeticEquipServiceIntegration_Concurrent100SameInstanceDifferentPets_OnlyOneEquips` | **新增**（构造 1 user + 100 个 pet + 1 实例；验证 DB uk_user_cosmetic_item_id 兜底）|
  | 边界 1（equip consumed 实例 5003）| `TestCosmeticEquipServiceIntegration_EquipConsumedInstance_Returns5003` | **新增**（实例 status=3 consumed → 5003）|
  | 边界 2（equip 非本人实例 5002）| `TestCosmeticEquipServiceIntegration_EquipNotOwnedInstance_Returns5002` | **新增**（实例属于 user B，userA equip → 5002）|
  | 边界 3（unequip 空 slot 5004）| `TestCosmeticEquipServiceIntegration_UnequipEmptySlot_Returns5004` | **新增**（pet 无任何装备 → unequip slot=1 → 5004）|
  | 状态一致性（行 3613）| `TestCosmeticEquipServiceIntegration_StateConsistencyMatrix` + 内联辅助断言 `assertEquipStateConsistency(t, rawDB, userID)` | **新增**（双向不变量断言 helper + 独立矩阵 case；并在完整流程 / 回滚 case 末尾复用 helper）|

  **关键设计约束**：
  - 全部 ≥10 case 必须挂 `//go:build integration` + `// +build integration` 双行 tag（与 26.3 / 26.4 / 20.9 同模式 —— 默认 `bash scripts/build.sh --test` 不触发；只在 `bash scripts/build.sh --integration`（`go test -tags=integration`）触发）
  - **回滚 1/2/3 必须用 fault injection wrapper repo**（不能用 stub repo，理由同 20.9 §关键设计约束：stub 不真开 InnoDB 事务无法验证 rollback 真行为 —— 必须真起 `tx.NewManager(gormDB).WithTx` 让 fault 让 fn return error 触发真实 InnoDB ROLLBACK，再断言 DB 5 表恢复 case setup 状态）；wrapper 模式与 20.9 落地的 `faultStepAccountRepoOnSpend` / `faultCosmeticItemRepoOnList` / `faultChestOpenLogRepoOnCreate` / `faultChestRepoOnCreate` **完全同模式**（按方法包装真实 mysql repo + 在指定方法上替换为 injectErr，其余方法透传委托给被包装的真实 repo）
  - **本 story 需新增 2 个 fault wrapper struct**（仅本文件 `cosmetic_equip_service_integration_test.go` 可见，避免与 20.9 / 4.7 / 11.9 同 package `service_test` 命名冲突 —— 命名带 `Equip` 前缀区分）：
    - `faultUserPetEquipRepoOnDelete` — 回滚 1 用：包装真实 `mysql.UserPetEquipRepo`，`DeleteByPetSlotInTx` 抛 `injectErr`，其余 5 方法（`FindByPetSlot` / `InsertInTx` / `FindUserCosmeticItemIDByPetSlotForUpdate` / `DeleteByPetSlotInTxReturningAffected`）透传真实 repo
    - `faultUserCosmeticItemRepoOnUpdateStatus` — 回滚 2 / 回滚 3 用：包装真实 `mysql.UserCosmeticItemRepo`，`UpdateStatusInTx` 抛 `injectErr`（**注**：回滚 2 是 slot 空场景 equip 走到"步骤 9 INSERT user_pet_equips" 后"最后一步 UpdateStatusInTx(当前实例,equipped)"失败；回滚 3 是 unequip "步骤 6 DELETE 后 UpdateStatusInTx(uciID,in_bag)"失败 —— 同一个 wrapper 服务两个回滚 case，因为两处都是 `userCosmeticRepo.UpdateStatusInTx` 失败语义），其余 `UpdateStatusInTx` 之外方法透传真实 repo
    - **装配方式**：参照 26.3 落地的 `buildCosmeticEquipServiceIntegration`（行 47-87）—— 回滚 case 起容器 + migrate 后**不**直接用 helper 返回的 svc，而是新建一个"装配 helper 内部 repo 集 + 在目标 repo 上套 fault wrapper + `service.NewCosmeticEquipService` 重新装配"的局部构造（与 20.9 `buildChestServiceWithRepos` 行 405-477 "返回原料供 fault case 在原料基础上构造 fault 包装 + svc 装配" 同模式 —— **建议**：本 story 抽一个 `buildCosmeticEquipServiceIntegrationWithRepos(t) (gormDB, userCosmeticItemRepo, cosmeticItemRepo, petRepo, userPetEquipRepo, txMgr, rawDB, cleanup)` 暴露内部原料，fault case 在原料上套 wrapper 再 `service.NewCosmeticEquipService(txMgr, <可能被 wrap 的 repo>...)`；既有 `buildCosmeticEquipServiceIntegration` 保持不变供非 fault case 复用，新 helper 内部可被既有 helper 调用以避免重复 dsn/migrate 代码 —— 实装时以"最小重复 + 不破坏 26.3/26.4 既有 2 case"为准）
  - **回滚 1（equip 删旧装备失败）必须先让 slot 有旧装备**：case setup 先 `svc.Equip(hatA)` 用**正常 svc**（无 fault）把 hatA 装到 slot=1（status=2 + user_pet_equips 1 行指向 hatA）；再切到 **fault svc**（`DeleteByPetSlotInTx` 注入 err）`Equip(hatB)` 同 slot=1 → equip 走到"步骤 8 同槽已有装备 → DeleteByPetSlotInTx 删旧" 失败 → fn return error → InnoDB ROLLBACK；断言：hatA `status==2`（仍 equipped）+ hatB `status==1`（仍 in_bag）+ user_pet_equips 仍 1 行且 `user_cosmetic_item_id == hatA`（指向旧实例未变）+ `assertEquipStateConsistency` 通过
  - **回滚 2（equip 更新实例 status 失败）用 slot 空场景**：case setup user + pet + 1 件 hatA（slot 空，无前置装备）；fault svc（`UserCosmeticItemRepo.UpdateStatusInTx` 注入 err）`Equip(hatA)` → equip slot 空 → 跳过删旧 → "步骤 9 InsertInTx user_pet_equips"成功 → "最后一步 UpdateStatusInTx(hatA,equipped)" 失败 → fn return error → InnoDB ROLLBACK 把刚 INSERT 的 user_pet_equips 行也回滚；断言：user_pet_equips `WHERE pet_id=?` **0 行**（INSERT 的新行被回滚）+ hatA `status==1`（仍 in_bag，未变 equipped）+ `assertEquipStateConsistency` 通过（0 equipped 实例 + 0 user_pet_equips 行，双向空集一致）
  - **回滚 3（unequip 最后一步失败）先正常装一件再 fault unequip**：case setup 用**正常 svc** Equip(hatA) 到 slot=1（status=2 + user_pet_equips 1 行）；切 **fault svc**（`UserCosmeticItemRepo.UpdateStatusInTx` 注入 err）`Unequip(petId, slot=1)` → unequip "步骤 6 DeleteByPetSlotInTxReturningAffected 删行"成功（rowsAffected=1）→ "UpdateStatusInTx(hatA,in_bag)" 失败 → fn return error → InnoDB ROLLBACK 把 DELETE 也回滚；断言：user_pet_equips `WHERE pet_id=? AND slot=1` **仍 1 行**（DELETE 被回滚 → 行未删）+ hatA `status==2`（仍 equipped，未变 in_bag）+ `assertEquipStateConsistency` 通过（1 equipped 实例 ↔ 1 user_pet_equips 行）
  - **并发 1（同 pet 同 slot 100 并发 equip 不同实例）**：case setup 1 user + 1 pet + 100 件不同 hat 实例（全 slot=1，rarity 同，全 status=1 in_bag —— 用 `cosmeticIDByCode(t,rawDB,"hat_yellow")` 等 slot=1 配置 ID 复用即可，100 件 user_cosmetic_items 行 user_cosmetic_item_id 不同但 cosmetic_item_id 可同）；100 goroutine 各 `svc.Equip(petId, instᵢ)` 用 `sync.WaitGroup` + 收集每个 goroutine 的 (err)（与 20.9 `_Concurrent100SameKey` 行 810-940 同 goroutine 收集模式）；**断言**：恰 **1 个 goroutine 成功**（err==nil）+ 99 个失败（err != nil —— DB `uk_pet_slot` UNIQUE(pet_id,slot) X-lock 兜底，输家 INSERT user_pet_equips 撞 1062 → service `runEquipTx` `InsertInTx` 1062 翻译路径返业务错误 → fn return error → ROLLBACK）+ DB `user_pet_equips WHERE pet_id=?` 恰 **1 行** + 恰 **1 个实例 status=2**（赢家实例）+ 其余 99 个实例 `status==1`（in_bag，输家事务回滚未改 status）+ `assertEquipStateConsistency` 通过。**注**：不强求"哪一个实例赢"（真随机调度）；只断言"有且仅 1 个赢 + DB 终态干净 + 一致性不变"
  - **并发 2（同一实例 100 并发 equip 到不同 pet）**：epics.md 行 3609 显式注 "理论不发生因 1 user 1 pet，但测一致性约束"；case setup 1 user + **100 个 pet**（同 user_id，pet 表 1 user N pet 物理可建 —— 用 `insertPet` 循环造 100 行，**`is_default=i` 各不相同避开 `uk_user_default_pet (user_id, is_default)` UNIQUE**，fix-review 26-5 r1 [P1]；同 user 因 equip 步骤 4/6 实例+pet 归属均须 == userID，单实例单 owner 不能改多 user；与节点 9 业务"1 user 1 pet"约束正交，本 case 纯测 DB `uk_user_cosmetic_item_id` UNIQUE 兜底）+ 1 件 hat 实例（status=1）；100 goroutine 各 `svc.Equip(petᵢ, sameInstance)`；**断言**：恰 **1 个成功** + 99 个失败（DB `uk_user_cosmetic_item_id` UNIQUE(user_cosmetic_item_id) X-lock 兜底 —— 输家 INSERT 撞 1062；不同 pet 各空 slot 无 swap 路径，"恰 1 成功"成立）+ DB `user_pet_equips WHERE user_cosmetic_item_id=?` 恰 **1 行** + 该实例 `status==2` + `assertEquipStateConsistency` 通过
  - **`assertEquipStateConsistency(t, rawDB, userID)` 双向不变量断言 helper（NFR2 核心，epics.md 行 3613）**：实装两条 SQL 反向校验（用既有 `assertCount` 模式或裸 `rawDB.QueryRow`）：
    - **正向**（status=2 ⟹ 有 user_pet_equips 行）：`SELECT COUNT(*) FROM user_cosmetic_items uci WHERE uci.user_id=? AND uci.status=2 AND NOT EXISTS (SELECT 1 FROM user_pet_equips upe WHERE upe.user_cosmetic_item_id=uci.id)` 必须 **== 0**（无"equipped 但无装备关系"的孤儿实例）
    - **反向**（user_pet_equips 行 ⟹ 实例 status=2）：`SELECT COUNT(*) FROM user_pet_equips upe JOIN user_cosmetic_items uci ON uci.id=upe.user_cosmetic_item_id WHERE upe.user_id=? AND uci.status<>2` 必须 **== 0**（无"装备关系存在但实例非 equipped"的孤儿行）
    - 任一非 0 → `t.Fatalf` 报具体哪条不变量被破坏 + userID（便于定位）
    - **复用范围**：完整流程末尾 / 同槽换装后 / 3 个回滚 case ROLLBACK 后 / 2 个并发 case 终态 / 独立 `StateConsistencyMatrix` case（先 equip 5 件 → 卸 2 件 → 同槽换 1 件 → 每步后调 helper）均调用，确保任意操作序列后 status↔user_pet_equips 双向一致
  - **完整流程（5 slot 全装）实例 slot 选取**：5 个不同 slot 用 0012 seed cosmetic_items 中不同 slot 的 code（`cosmeticIDByCode` 查 ID）—— slot 枚举 `{1,2,3,4,5,6,7,99}`，从 seed 里挑 5 个分属不同 slot 的 code（实装时 `grep` 0012 migration seed 行确认每个 code 的 slot，如 `hat_yellow`=slot1；其余 slot 的 seed code 由实装阶段从 `migrations/0012_*.sql` 实际读取确定 —— **不**臆造 code，以 0012 seed 实际行为准）；依次 `svc.Equip(petId, instₖ)` 5 次 → 断言 `user_pet_equips WHERE pet_id=?` 恰 **5 行** + 5 个实例全 `status=2` + `assertEquipStateConsistency` 通过

  - **本机 Windows 无 Docker daemon**：集成测试 `bash scripts/build.sh --integration` 本机无法执行（与 26.3 / 26.4 / 既有所有 `*_integration_test.go` 同环境限制，`startMySQL` 内已 `t.Skip` 兜底，CI Linux 跑）；本 story 验收以"`go vet -tags=integration ./internal/service/` 通过 + `go test -tags=integration -list 'TestCosmeticEquipServiceIntegration.*'` 新增 ≥10 个测试函数 + 既有 2 个全部正确注册编译通过"为本机可验证标准（与 26.4 Dev Notes 行 110 + Debug Log 行 225 同模式）

**本 story 不做**（明确范围红线）：

- [skip] **不**修改 `server/internal/service/cosmetic_equip_service.go`（26.3 `Equip`/`runEquipTx` + 26.4 `Unequip`/`runUnequipTx` 已 done；本 story 仅消费 + 通过 fault wrapper 注入 repo 失败做 Layer 2 黑盒回滚验证）
- [skip] **不**修改 `server/internal/repo/mysql/user_pet_equip_repo.go`（26.3 `FindByPetSlot`/`DeleteByPetSlotInTx`/`InsertInTx` + 26.4 `FindUserCosmeticItemIDByPetSlotForUpdate`/`DeleteByPetSlotInTxReturningAffected` 已 done；本 story 仅消费 + 包装做 fault injection）
- [skip] **不**修改 5 个相关 mysql repo（user_pet_equip / user_cosmetic_item / cosmetic_item / pet + tx manager；26.2-26.4 已 done；本 story 仅消费 + wrapper 透传）
- [skip] **不**修改 `server/internal/app/http/handler/cosmetics_handler.go`（26.3 Equip + 26.4 Unequip handler 已 done；本 story **不**测 handler 层 —— epics.md §26.5 全部场景在 service 层验证；HTTP 端到端 envelope schema 由 26.3/26.4 handler 单测 + Story 28.1 跨端 E2E demo 阶段覆盖）
- [skip] **不**修改 0016 / 任何 0001-0016 migration（26.2 已落地 0016 user_pet_equips schema 含 uk_pet_slot + uk_user_cosmetic_item_id；本 story 仅消费这两个 UNIQUE 约束做并发兜底验证）
- [skip] **不**修改 `server/internal/repo/tx/manager.go`（4.2 已 done；本 story 回滚 case 仅消费真实 `tx.NewManager(gormDB).WithTx` 验证真 InnoDB ROLLBACK）
- [skip] **不**修改 26.3 已落地的 `TestCosmeticEquipServiceIntegration_EquipAndSwapSameSlot` + 26.4 已落地的 `TestCosmeticEquipServiceIntegration_UnequipHappyPath`（保持现有 done 状态测试不破坏 —— 仅在同一份 `cosmetic_equip_service_integration_test.go` 文件**追加** ≥10 个新 case + 必要的 helper / fault wrapper）
- [skip] **不**修改 26.3 已落地的 `buildCosmeticEquipServiceIntegration`（行 47-87）签名 / 行为（若需暴露内部原料供 fault case 装配 → **新增**一个 `buildCosmeticEquipServiceIntegrationWithRepos` helper，既有 helper 内部可委托新 helper 但对外签名不变 —— 不破坏 26.3/26.4 两个既有 case 的调用点）
- [skip] **不**新建跨包 testing util（不抽 startMySQL / fault wrapper 到 `internal/testutil/` —— 与 4.7 / 11.9 / 20.9 同模式，复用 helper / 新 wrapper 留在 service 包内即可，避免范围扩散）
- [skip] **不**用 sqlmock（epics.md 行 3614 钦定 "全部场景用 dockertest 真实 MySQL 跑通"；并发兜底依赖真实 InnoDB UNIQUE 索引 X-lock + 真实事务 ROLLBACK undo log，sqlmock 测 SQL 字符串匹配与本 story Layer 2 黑盒行为验证语义不符）
- [skip] **不**改 `docs/宠物互动App_*.md` 任一份（V1 §8.3 / §8.4 / 数据库设计 §5.10 / §8.4 / §6.8 / §6.10 / 时序图 §8.x 是契约**输入**，本 story 严格对齐**不**修改；若发现实装与契约不一致 → 优先改本 story 测试断言对齐契约，**不**反向改 docs / 不改 26.3/26.4 实装）
- [skip] **不**写 README / 部署文档：留 Epic 26 收尾或 Story 28.3 文档同步阶段
- [skip] **不**实装 GET /home pet.equips 真实化（Story 26.6 owner；本 story 不涉及 home_service / GET /home）
- [skip] **不**测 ctx cancel / timeout 路径（ADR-0007 ctx 传播是 4.2-4.6 已建立的范式，26.3/26.4 service 单测已覆盖，本 story 不重复验证）
- [skip] **不**测 deadlock / 隔离级别 anomaly 专项（InnoDB 默认 REPEATABLE READ + 本 story 100 goroutine 同 pet 同 slot equip 已触发 uk_pet_slot UNIQUE X-lock；不深挖隔离级别专项）
- [skip] **不**做 fuzz / property-based testing（dockertest case 已穷举 epics.md 钦定 12 类；fuzz 是 future testing 升级范畴）
- [skip] **不**支持 `go test -short`（dockertest 必跑；本 story ≥10 case 全部 `+build integration`，默认 `bash scripts/build.sh --test` 不触发；只在 `--integration` 触发）
- [skip] **不**实装"测试容器复用"优化（每 case 独立 `startMySQL` 容器，与 26.3 / 20.9 / 11.9 / 4.7 同模式，简单 + 一致性优于性能；优化方向留 future 性能 epic）
- [skip] **不**新造错误码 / 不改 §1 节点 9 冻结的 equip `{1001,1002,1005,5001,5002,5003,5008,1009}` / unequip `{1001,1002,1005,5002,5004,1009}` 错误码集合（本 story 仅断言既有错误码在边界 case 正确返回 —— equip consumed→5003 / equip 非本人→5002 / unequip 空 slot→5004）
- [skip] **不**给 Story 26.5 加 sprint-status.yaml 占位 retrospective（epic-26 retrospective 已在 sprint-status.yaml 第 256 行 optional，本 story done 后 26.6 done + 整 epic done 才推 retrospective）
- [skip] **不**测 user_pet_equips 之外的穿戴衍生表（节点 10 Story 29.x render_config 列尚未加；本 story 阶段 cosmetic_items 无 render_config，equip 不涉及）
- [skip] **不**为并发 case 引入第三方并发测试框架（用标准库 `sync.WaitGroup` + goroutine，与 20.9 `_Concurrent100SameKey` / 11.9 同模式）

## Acceptance Criteria

> 全部源自 epics.md §Story 26.5（行 3592-3616，**唯一权威 AC 来源**）+ NFR1 / NFR2 / NFR11 + V1 §8.3 / §8.4（26.1 冻结契约）+ 数据库设计 §5.10（uk_pet_slot + uk_user_cosmetic_item_id 两 UNIQUE）。本 story 是测试 story —— **不实装新业务功能**，仅扩展 `cosmetic_equip_service_integration_test.go` 集成测试覆盖矩阵。

**AC1 — 测试文件位置 + build tag + helper 复用**

- 本 story 在已有 `server/internal/service/cosmetic_equip_service_integration_test.go`（26.3 落地）**追加** ≥10 个新测试函数 + 必要的 helper / fault wrapper；**不**新建独立测试文件（与 4.7 / 11.9 / 20.9 同模式同包 `service_test` 同文件内聚）。
- 所有新增测试函数 + helper / wrapper 受文件顶部既有 `//go:build integration` + `// +build integration` 双行 tag 覆盖（与既有 2 case 同文件同 tag）—— 默认 `bash scripts/build.sh --test` 不触发，只在 `bash scripts/build.sh --integration`（`go test -tags=integration`）触发。
- 复用 26.3 落地的 `buildCosmeticEquipServiceIntegration`（行 47-87）+ `startMySQL` / `runMigrations`（auth_service_integration_test.go）+ `insertUser` / `insertPet`（home_service_integration_test.go）+ `insertUserCosmeticItem` / `cosmeticIDByCode`（cosmetic_service_integration_test.go）+ `assertCount`（chest_open_service_integration_test.go / 同包共享）helper，**不**重复定义 / **不**抽跨包 util。
- 文件顶部注释**追加** Story 26.5 段落：列出本 story 追加的 ≥10 个 case + 指向既有复用 case（`EquipAndSwapSameSlot` 同槽换装 / `UnequipHappyPath`）+ 12 类场景与 epics.md 行号对应表（与 20.9 文件顶部注释 §52-79 同文档化风格）。

**AC2 — 完整流程（epics.md 行 3603）**

- 新增 `TestCosmeticEquipServiceIntegration_FullFlow_Equip5SlotsAll`：创建 1 user + 1 pet + 5 件不同 cosmetic 实例（分属 5 个**不同 slot**，slot ∈ 枚举 `{1,2,3,4,5,6,7,99}`，code 由实装阶段从 `migrations/0012_*.sql` seed 实际读取确定 —— **不**臆造 code）→ 依次 `svc.Equip(petId, instₖ)` 5 次（全部 err==nil）→ 断言 DB `user_pet_equips WHERE pet_id=?` 恰 **5 行** + 5 个实例全 `status=2` + 每个 `(slot, user_cosmetic_item_id)` 对正确 + 末尾调 `assertEquipStateConsistency(t,rawDB,userID)` 通过。

**AC3 — 同槽换装一致性（epics.md 行 3604）**

- **复用** 26.3 `TestCosmeticEquipServiceIntegration_EquipAndSwapSameSlot` 已验证的"同 slot hat A → hat B：A status 回 1 + B status 变 2 + user_pet_equips 行更新（净不变）"（本 story **不**重复实现该 case，在文件顶部注释 + AC3 文档化此复用关系）。
- 本 story 在新增 `TestCosmeticEquipServiceIntegration_StateConsistencyMatrix` 内**复跑**一次同槽换装序列（equip hatA slot1 → equip hatB slot1）后调 `assertEquipStateConsistency` 断言换装后 status↔user_pet_equips 双向一致（补足 26.3 case 未显式断言的 NFR2 一致性矩阵维度）。

**AC4 — 回滚 1：equip 删旧装备失败 → 整体回滚（epics.md 行 3605）**

- 新增 `TestCosmeticEquipServiceIntegration_EquipDeleteOldEquipFails_AllRollback`：
  - case setup：用**正常 svc**（无 fault）`Equip(hatA)` 到 slot=1（hatA status=2 + user_pet_equips 1 行指向 hatA）。
  - 切 **fault svc**：`faultUserPetEquipRepoOnDelete`（包装真实 `UserPetEquipRepo`，`DeleteByPetSlotInTx` 抛 `injectErr`，其余 4 方法透传）+ `service.NewCosmeticEquipService` 重装配 → `Equip(hatB)` 同 slot=1（同槽换装走"步骤 8 删旧装备" → fault 抛 err → fn return error → 真 InnoDB ROLLBACK）。
  - 断言：`Equip(hatB)` 返非 nil err + hatA `status==2`（仍 equipped）+ hatB `status==1`（仍 in_bag）+ `user_pet_equips WHERE pet_id=?` 仍 **1 行** 且 `user_cosmetic_item_id == hatA`（指向旧实例未变）+ `assertEquipStateConsistency` 通过。

**AC5 — 回滚 2：equip 更新实例 status 失败 → 整体回滚（epics.md 行 3606）**

- 新增 `TestCosmeticEquipServiceIntegration_EquipUpdateStatusFails_AllRollback`：
  - case setup：user + pet + 1 件 hatA（slot=1 空，无前置装备）。
  - 切 **fault svc**：`faultUserCosmeticItemRepoOnUpdateStatus`（包装真实 `UserCosmeticItemRepo`，`UpdateStatusInTx` 抛 `injectErr`，其余方法透传）+ 重装配 → `Equip(hatA)`（slot 空 → 跳过删旧 → "步骤 9 InsertInTx user_pet_equips" 成功 → "最后一步 UpdateStatusInTx(hatA,equipped)" 失败 → fn return error → 真 InnoDB ROLLBACK）。
  - 断言：`Equip(hatA)` 返非 nil err + `user_pet_equips WHERE pet_id=?` **0 行**（INSERT 的新行被回滚）+ hatA `status==1`（仍 in_bag，未变 equipped）+ `assertEquipStateConsistency` 通过（双向空集一致）。

**AC6 — 回滚 3：unequip 最后一步失败 → 整体回滚（epics.md 行 3607）**

- 新增 `TestCosmeticEquipServiceIntegration_UnequipUpdateStatusFails_AllRollback`：
  - case setup：用**正常 svc** `Equip(hatA)` 到 slot=1（hatA status=2 + user_pet_equips 1 行）。
  - 切 **fault svc**：`faultUserCosmeticItemRepoOnUpdateStatus`（同 AC5 wrapper 复用，`UpdateStatusInTx` 抛 injectErr）+ 重装配 → `Unequip(petId, slot=1)`（unequip "步骤 6 DeleteByPetSlotInTxReturningAffected" 成功 rowsAffected=1 → "UpdateStatusInTx(hatA,in_bag)" 失败 → fn return error → 真 InnoDB ROLLBACK 把 DELETE 也回滚）。
  - 断言：`Unequip` 返非 nil err + `user_pet_equips WHERE pet_id=? AND slot=1` 仍 **1 行**（DELETE 被回滚 → 行未删）+ hatA `status==2`（仍 equipped，未变 in_bag）+ `assertEquipStateConsistency` 通过（1↔1 一致）。

**AC7 — 并发 1：同 pet 同 slot 100 并发 equip 不同实例（DB uk_pet_slot 兜底，epics.md 行 3608）**

- 新增 `TestCosmeticEquipServiceIntegration_Concurrent100SamePetSlot_FinalStateConsistent`（fix-review 26-5 r1 [P1] 改名 + 重设计断言）：
  - case setup：1 user + 1 pet + 100 件不同 hat 实例（全 slot=1，全 status=1 in_bag；cosmetic_item_id 可同一个 slot=1 code，user_cosmetic_item_id 100 个不同）。
  - 100 goroutine（`sync.WaitGroup`）各 `svc.Equip(petId, instᵢ)`，收集每个 (err)（与 20.9 `_Concurrent100SameKey` 行 810-940 同 goroutine 收集模式）。
  - **断言（终态一致性矩阵，非成功计数）**：`runEquipTx` 步骤 8 是**同槽换装 swap** 语义（`FindByPetSlot` 普通 SELECT 无 FOR UPDATE；查到既存行就删旧+回退旧 status → INSERT 新行）—— 一个 tx commit 后被串行化的后续 goroutine 读到既存行会走换装路径**成功装自己**，`uk_pet_slot` 只保证同时刻单行**不**保证 99 失败。故**不**断言"恰 1 成功 99 失败"（该弱命题与 swap 语义冲突，服务正确时会误失败），改断言并发正确性真不变量（epics.md 行 3613 NFR2）：① 成功数 **>= 1**（slot 空必有一个 equip 占住）；② `user_pet_equips WHERE pet_id=?` 恰 **1 行** + `(pet,slot=1)` 恰 1 行（uk_pet_slot 兜底，无脏写/无多行）；③ 恰 **1 个实例 status=2** + 其余 N-1 个 `status=1` + 全部 N 个 `status IN (1,2)`（无实例卡 consumed/invalid 中间态 → 无部分提交）；④ 现存装备行指向的实例**正是**那个唯一 status=2 实例（行↔状态对齐 JOIN 断言）；⑤ `assertEquipStateConsistency` 双向一致。

**AC8 — 并发 2：同一实例 100 并发 equip 到不同 pet（DB uk_user_cosmetic_item_id 兜底，epics.md 行 3609）**

- 新增 `TestCosmeticEquipServiceIntegration_Concurrent100SameInstanceDifferentPets_OnlyOneEquips`：
  - case setup：1 user + **100 个 pet**（同 user_id，`insertPet` 循环造 100 行，**`is_default=i` 各异避开 `uk_user_default_pet (user_id, is_default)` UNIQUE** —— fix-review 26-5 r1 [P1]：原 `is_default=0` 硬编码会让第 2 条 insertPet 撞 1062 t.Fatalf，case 到不了被测代码；同 user 因 equip 归属校验须 == userID 不能改多 user —— 与节点 9 业务"1 user 1 pet"约束正交，本 case 纯测 DB UNIQUE 兜底）+ 1 件 hat 实例（status=1 in_bag）。
  - 100 goroutine 各 `svc.Equip(petᵢ, sameInstance)`，收集每个 (err)。
  - 断言：恰 **1 个 err==nil** + 99 个 err != nil（DB `uk_user_cosmetic_item_id` UNIQUE(user_cosmetic_item_id) X-lock + 1062 兜底）+ `user_pet_equips WHERE user_cosmetic_item_id=?` 恰 **1 行** + 该实例 `status==2` + `assertEquipStateConsistency` 通过。

**AC9 — 边界 1/2/3（epics.md 行 3610-3612）**

- 新增 `TestCosmeticEquipServiceIntegration_EquipConsumedInstance_Returns5003`：user + pet + 1 件 hatA 实例 status=**3 consumed**（`insertUserCosmeticItem` 后 raw `UPDATE user_cosmetic_items SET status=3 WHERE id=?` 或直接以 status=3 插入）→ `svc.Equip(petId, hatA)` → 断言返 `apperror.ErrCosmeticItemUnavailable`（**5003**，错误码值以 26.1 落地 `server/internal/pkg/errors/codes.go` 实际常量名为准 —— 实装阶段 `grep` 确认 5003 对应常量；epics.md 行 3509 "5003 道具状态不可用"）+ DB 状态不变（hatA 仍 status=3 + user_pet_equips 0 行）。
- 新增 `TestCosmeticEquipServiceIntegration_EquipNotOwnedInstance_Returns5002`：user A + pet A + 1 件 hatB 实例属于 **user B**（`insertUser` 造 user B + `insertUserCosmeticItem(userB,...)`）→ user A `svc.Equip(petA, hatB实例id)` → 断言返 `apperror.ErrCosmeticNotOwned`（**5002**）+ DB 状态不变（hatB 仍 status=1 + user_pet_equips 0 行）。
- 新增 `TestCosmeticEquipServiceIntegration_UnequipEmptySlot_Returns5004`：user + pet（无任何装备）→ `svc.Unequip(petId, slot=1)` → 断言返 `apperror.ErrCosmeticSlotMismatch`（**5004**）+ DB 状态不变（user_pet_equips 0 行）。
- 错误码常量名以 26.1 落地 `server/internal/pkg/errors/codes.go` 实际为准（5002=`ErrCosmeticNotOwned` / 5004=`ErrCosmeticSlotMismatch` 由 26.4 Dev Notes 行 188 确认；5003 对应常量名实装阶段 `grep "5003" codes.go` 确认 —— **不**臆造常量名，断言用 `errors.Is` 或 `apperror.Code(err)` 取数值码）。

**AC10 — 状态一致性矩阵 helper + 独立 case（NFR2，epics.md 行 3613）**

- 新增 `assertEquipStateConsistency(t *testing.T, rawDB *sql.DB, userID uint64)` helper（本文件可见，受 build tag 覆盖）：实装 §关键设计约束钦定的双向 SQL 不变量校验（正向 status=2⟹有 user_pet_equips 行 == 0 孤儿 + 反向 user_pet_equips 行⟹实例 status=2 == 0 孤儿），任一非 0 → `t.Fatalf` 报具体破坏的不变量 + userID。
- 新增 `TestCosmeticEquipServiceIntegration_StateConsistencyMatrix`：单容器内串行跑一组操作序列（equip 3 件不同 slot → unequip 1 件 → 同槽换装 1 件 → equip 第 4 件 → unequip 全部），**每个操作后**调 `assertEquipStateConsistency` 断言任意操作序列后 status↔user_pet_equips 双向一致。
- AC2 / AC4 / AC5 / AC6 / AC7 / AC8 的对应 case **末尾**均调 `assertEquipStateConsistency`（一致性不变量是全 case 的横切断言，不只独立 case 验）。

**AC11 — 验证脚本通过（本机降级标准）**

- 本机 Windows 无 Docker daemon → `bash scripts/build.sh --integration` 无法执行（`startMySQL` 内 `t.Skip` 兜底，CI Linux 跑）。本 story 本机可验证标准（与 26.4 AC7 同模式）：
  - `bash scripts/build.sh --test` 全绿（vet + build + `go test -count=1 ./...` 单测 —— 本 story **不**新增 unit test，仅确保 integration test 文件 build tag 隔离不影响默认单测编译；既有全包单测保持 BUILD SUCCESS）
  - `go vet -tags=integration ./internal/service/` **通过**（新增 ≥10 case + 2 fault wrapper + helper 编译干净）
  - `go test -tags=integration -list 'TestCosmeticEquipServiceIntegration.*'` 列出**既有 2 个**（`EquipAndSwapSameSlot` / `UnequipHappyPath`）+ 本 story 新增 ≥10 个测试函数全部正确注册（证明编译通过 + 测试函数命名规范，执行需 CI Linux Docker）。

## Tasks / Subtasks

- [x] **Task 1 — fault injection wrapper + helper 基础设施**（AC1, AC4, AC5, AC6, AC10）
  - [x] `server/internal/service/cosmetic_equip_service_integration_test.go` 追加 `faultUserPetEquipRepoOnDelete` struct（持有真实 `mysql.UserPetEquipRepo` + `injectErr error`；`DeleteByPetSlotInTx` 返 injectErr，其余 4 方法 `FindByPetSlot`/`InsertInTx`/`FindUserCosmeticItemIDByPetSlotForUpdate`/`DeleteByPetSlotInTxReturningAffected` 透传委托真实 repo）—— 模式抄 20.9 `faultStepAccountRepoOnSpend`（行 1448-1457，按方法包装真实 repo）
  - [x] 追加 `faultUserCosmeticItemRepoOnUpdateStatus` struct（持有真实 `mysql.UserCosmeticItemRepo` + `injectErr`；`UpdateStatusInTx` 返 injectErr，其余 3 方法 `ListByUserForInventory`/`CreateInTx`/`FindByIDForEquip` 透传）—— 服务 AC5 + AC6 两个回滚 case
  - [x] 追加 `buildCosmeticEquipServiceIntegrationWithRepos(t) (userCosmeticItemRepo, cosmeticItemRepo, petRepo, userPetEquipRepo, txMgr tx.Manager, rawDB *sql.DB, cleanup func())` helper（暴露内部原料供 fault case 在原料上套 wrapper + `service.NewCosmeticEquipService` 重装配；模式抄 20.9 `buildChestServiceWithRepos`）；**既有 `buildCosmeticEquipServiceIntegration` 签名/行为不改**（新增独立 helper，既有 helper 不委托新 helper 以保 26.3/26.4 调用 trace 不变）
  - [x] 追加 `assertEquipStateConsistency(t, rawDB, userID)` helper（双向 SQL 不变量：正向 status=2⟹有 user_pet_equips 行 ==0 孤儿 + 反向 user_pet_equips 行⟹实例 status=2 ==0 孤儿；任一非 0 → t.Fatalf 报具体不变量 + userID）+ `requireEquipAppError` 错误码断言 helper
  - [x] 文件顶部注释追加 Story 26.5 段落（10 case 清单 + 复用 26.3/26.4 既有 case 说明 + 12 类场景↔epics.md 行号对应表）
- [x] **Task 2 — 完整流程 + 同槽换装一致性 + 状态一致性矩阵 case**（AC2, AC3, AC10）
  - [x] `TestCosmeticEquipServiceIntegration_FullFlow_Equip5SlotsAll`（5 不同 slot 实例依次 equip → user_pet_equips 5 行 + 5 实例 status=2 + 末尾 assertEquipStateConsistency）；5 个 slot 的 seed code 从 `migrations/0012_seed_cosmetic_items.up.sql` 实际读取确定：hat_yellow=slot1 / gloves_white=slot2 / glasses_round=slot3 / neck_blue=slot4 / back_bag=slot5
  - [x] `TestCosmeticEquipServiceIntegration_StateConsistencyMatrix`（操作序列 equip×3 → unequip×1 → 同槽换装×1 → equip×1 → unequip×全部；每操作后 assertEquipStateConsistency；含 AC3 复跑同槽换装后一致性断言）
- [x] **Task 3 — 3 个回滚 case**（AC4, AC5, AC6）
  - [x] `TestCosmeticEquipServiceIntegration_EquipDeleteOldEquipFails_AllRollback`（正常 svc Equip hatA slot1 → fault `faultUserPetEquipRepoOnDelete` svc Equip hatB slot1 → 断言 hatA 仍 status=2 + hatB 仍 status=1 + user_pet_equips 仍 1 行指向 hatA + assertEquipStateConsistency）
  - [x] `TestCosmeticEquipServiceIntegration_EquipUpdateStatusFails_AllRollback`（slot 空 → fault `faultUserCosmeticItemRepoOnUpdateStatus` svc Equip hatA → 断言 user_pet_equips 0 行 + hatA status=1 + assertEquipStateConsistency 双向空集）
  - [x] `TestCosmeticEquipServiceIntegration_UnequipUpdateStatusFails_AllRollback`（正常 svc Equip hatA slot1 → fault `faultUserCosmeticItemRepoOnUpdateStatus` svc Unequip slot1 → 断言 user_pet_equips 仍 1 行 + hatA 仍 status=2 + assertEquipStateConsistency 1↔1）
- [x] **Task 4 — 2 个并发 case（100 goroutine）**（AC7, AC8）
  - [x] `TestCosmeticEquipServiceIntegration_Concurrent100SamePetSlot_FinalStateConsistent`（1 user + 1 pet + 100 实例全 slot=1 → 100 goroutine Equip → **终态一致性矩阵**：成功数 >= 1 + user_pet_equips 恰 1 行 + 恰 1 实例 status=2 + 其余 N-1 status=1 + 全 N 个 status IN(1,2) + 行↔状态对齐 + assertEquipStateConsistency；DB uk_pet_slot 兜底；fix-review 26-5 r1 [P1]：swap 语义下不断言"恰 1 成功"）
  - [x] `TestCosmeticEquipServiceIntegration_Concurrent100SameInstanceDifferentPets_OnlyOneEquips`（1 user + 100 pet [is_default=i 各异，避开 uk_user_default_pet] + 1 实例 → 100 goroutine Equip 到不同 pet → 恰 1 成功 + 99 失败 + user_pet_equips 1 行 + 实例 status=2 + assertEquipStateConsistency；DB uk_user_cosmetic_item_id 兜底；fix-review 26-5 r1 [P1]：setup 修正）；goroutine 收集 + start barrier 模式抄 20.9 `_Concurrent100SameKey` 行 810
- [x] **Task 5 — 3 个边界 case**（AC9）
  - [x] `TestCosmeticEquipServiceIntegration_EquipConsumedInstance_Returns5003`（实例 status=3 consumed → Equip → 5003 `apperror.ErrCosmeticInvalidState` + DB 不变）；5003 常量名 `grep` 确认 = `ErrCosmeticInvalidState`（codes.go 行 41）
  - [x] `TestCosmeticEquipServiceIntegration_EquipNotOwnedInstance_Returns5002`（实例属 user B，user A Equip → 5002 `apperror.ErrCosmeticNotOwned` + DB 不变）
  - [x] `TestCosmeticEquipServiceIntegration_UnequipEmptySlot_Returns5004`（pet 无装备 → Unequip slot=1 → 5004 `apperror.ErrCosmeticSlotMismatch` + DB 不变）
- [x] **Task 6 — 验证 + 收尾**（AC11）
  - [x] `bash scripts/build.sh --test` 全绿（vet + build + 全包单测 BUILD SUCCESS —— 本 story 不新增 unit test，integration build tag 隔离不破坏默认单测编译，已实跑确认）
  - [x] `go vet -tags=integration ./internal/service/` 通过（exit 0 无输出；10 新 case + 2 fault wrapper + 3 helper 编译干净）
  - [x] `go test -tags=integration -list 'TestCosmeticEquipServiceIntegration.*'` 列出既有 2 个 + 新增 10 个测试函数全部正确注册（共 12 个；本机 Windows 无 Docker，执行归 CI Linux）
  - [x] 自检：无改 `cosmetic_equip_service.go` / `user_pet_equip_repo.go` / 任何 5 repo / handler / router / 0001-0016 migration / docs/*.md / 26.3/26.4 既有 2 case / `buildCosmeticEquipServiceIntegration` 既有签名；无 sqlmock；无新造错误码；范围红线全部遵守

## Dev Notes

### 关键架构约束（必须遵守）

- **目录形态与 CLAUDE.md target 不同**：实际代码不在 `internal/domain/` —— service 集成测试在 `internal/repo/...` 之上的 `internal/service/`（package `service_test`），与 26.3/26.4/20.9/11.9 同。按**实际既有结构**落地，**不**按 CLAUDE.md §"节点 1 之后的目录形态（target）"的理想树新建目录（与 26.4 Dev Notes 行 148 同结论）。
- **本 story 是测试 story，不实装新业务功能**：仅扩展 `cosmetic_equip_service_integration_test.go` 覆盖矩阵（与 4.7 / 11.9 / 20.9 / 32.5 同模式）。**不**改任何 service / repo / handler / router / migration 生产代码。
- **回滚 case 必须真起 InnoDB 事务**：26.3 落地的 `buildCosmeticEquipServiceIntegration` 用真实 `tx.NewManager(gormDB)`（行 72-74）—— 回滚 case 必须复用真 `tx.Manager.WithTx`（不能用单测 mock txManager 直接调 fn 不真起事务，那只验"fn 返 error"不验"InnoDB undo log 真回滚 DB 行"）。fault wrapper 让 `WithTx` fn 内某 repo 调用 return error → `WithTx` 触发真 ROLLBACK → 断言 DB 表恢复 case setup 态（与 20.9 §关键设计约束行 74 "回滚必须用 fault injection wrapper repo，stub 不真开 InnoDB 事务无法验证 rollback 真行为" 同根因）。
- **ctx 传播（ADR-0007）**：测试中 `svc.Equip(context.Background(), ...)` / `svc.Unequip(context.Background(), ...)` —— service 内部 `txMgr.WithTx(ctx, fn)` 把 txCtx 下传 repo（26.3/26.4 已实装，本 story 仅消费）。fault wrapper 透传方法签名第一参数 `ctx context.Context` 原样转发被包装 repo（**不**改 ctx）。
- **错误码断言用数值码不用字符串**：边界 case 断言 5002/5003/5004 用 `errors.Is(err, apperror.ErrXxx)` 或 `apperror.Code(err) == 500X`（具体取码方式以既有 service 集成测试断言风格为准 —— `grep` 既有 `cosmetic_service_integration_test.go` / `chest_open_service_integration_test.go` 错误断言模式照搬）；常量名以 26.1 落地 `server/internal/pkg/errors/codes.go` 实际为准（**不**臆造）。

### 与 26.3 / 26.4 已落地集成测试的分工（关键边界）

| 维度 | 26.3 `EquipAndSwapSameSlot` | 26.4 `UnequipHappyPath` | **26.5（本 story）** |
|---|---|---|---|
| equip happy | ✅ slot 空直接装 + 同槽换装 | — | 复用（FullFlow 扩 5 slot；StateConsistencyMatrix 复跑换装 + 一致性断言）|
| unequip happy | — | ✅ unequip + 重复 unequip 5004 | 复用（StateConsistencyMatrix 含 unequip 序列）|
| equip 回滚 | ❌ | ❌ | ✅ 删旧装备失败 / 更新实例 status 失败（fault wrapper + 真 ROLLBACK）|
| unequip 回滚 | ❌ | ❌ | ✅ 最后一步 UpdateStatusInTx 失败（fault wrapper + 真 ROLLBACK）|
| 100 goroutine 并发 | ❌ | ❌ | ✅ 同 pet 同 slot（uk_pet_slot 兜底）/ 同实例不同 pet（uk_user_cosmetic_item_id 兜底）|
| 边界错误码 | ❌（26.3 单测覆盖 5001/5002/5003）| ❌（26.4 单测覆盖 5004）| ✅ 集成层真 DB 复验 5003/5002/5004 |
| 状态一致性矩阵 | ❌ | ❌ | ✅ `assertEquipStateConsistency` 双向不变量 + 横切全 case |

26.3/26.4 是"局部 happy / swap / 单 error"，本 story 是"全失败模式 + 高并发 DB 兜底 + 一致性矩阵"——**两层互补不重叠**；本 story **不**重写 26.3/26.4 已验证场景（仅复用 + 在矩阵 case 中复跑做一致性补断言）。

### Layer 2 fault injection 范式（直接参照 20.9）

- 20.9 §下游依赖（`20-9-layer-2-集成测试-开箱事务全流程.md` 行 83）已显式点名："本 story 的 fault injection 模式（4 个 wrapper）成为 future Layer 2 集成测试的范式（如 ... Story 26.5 穿戴事务集成测试 ... 都钦定相同 Layer 2 模式）"。
- 20.9 落地 fault wrapper 实装位置：`server/internal/service/chest_open_service_integration_test.go` 行 1441-1457+（`faultStepAccountRepoOnSpend` 等 —— 按方法包装真实 mysql repo，目标方法返 injectErr 其余透传）。**本 story 新增的 2 个 wrapper（`faultUserPetEquipRepoOnDelete` / `faultUserCosmeticItemRepoOnUpdateStatus`）严格抄此模式**，只换被包装 interface + 注入方法。
- 20.9 `buildChestServiceWithRepos`（行 405-477）"返回完整原料供 fault case 在原料上构造 fault 包装 + svc 装配" → 本 story `buildCosmeticEquipServiceIntegrationWithRepos` 同模式（暴露 gormDB + 4 repo + txMgr + rawDB + cleanup）。
- 20.9 `_Concurrent100SameKey`（行 810-940）100 goroutine + `sync.WaitGroup` + 收集每 goroutine (err) → 统计成功/失败计数断言 → 本 story AC7/AC8 两个并发 case 同 goroutine 收集 + 计数断言模式。

### V1 §8.3 equip / §8.4 unequip 事务步骤（回滚 case 注入点定位依据）

- **equip（26.3 `runEquipTx`，`cosmetic_equip_service.go` 行 197-308）**：步骤 4 查实例归属/状态 → 步骤 6 查 pet 归属 → 步骤 8 查同 slot 旧装备（有 → `DeleteByPetSlotInTx` 删旧 + `UpdateStatusInTx(旧实例,in_bag)`）→ 步骤 9 `InsertInTx` user_pet_equips 新行 → 最后一步 `UpdateStatusInTx(当前实例,equipped)` → commit。**回滚 1 注入点 = 步骤 8 `DeleteByPetSlotInTx`**（须 slot 已有旧装备前置）；**回滚 2 注入点 = 最后一步 `UpdateStatusInTx(当前实例,equipped)`**（slot 空场景，步骤 9 INSERT 已成功 → 验 INSERT 行回滚）。
- **unequip（26.4 `runUnequipTx`，`cosmetic_equip_service.go` 行 364-419）**：步骤 4 查 pet 归属 → 步骤 5 `FindUserCosmeticItemIDByPetSlotForUpdate`（FOR UPDATE 行锁）→ 步骤 6 `DeleteByPetSlotInTxReturningAffected`（rowsAffected==0→5004 回滚）→ `UpdateStatusInTx(uciID,in_bag)` → commit。**回滚 3 注入点 = 步骤 6 之后的 `UpdateStatusInTx(uciID,in_bag)`**（须 slot 已有装备前置，DELETE 已成功 rowsAffected=1 → 验 DELETE 行回滚未删）。
- 上述步骤号 / 行号是**实装阶段定位 fault 注入点的参照**，dev 须 `grep` 实际 `cosmetic_equip_service.go` 确认 26.3/26.4 实装的方法调用顺序（**不**臆造 —— 以实际 done 代码为准）。

### DB UNIQUE 约束（并发兜底验证依据）

- `migrations/0016_*.sql`（26.2 落地，user_pet_equips schema）含两个 UNIQUE（数据库设计 §5.10 + epics.md §26.2 行 3526-3529）：
  - `UNIQUE KEY uk_pet_slot (pet_id, slot)` —— 一个宠物同一部位只能穿一件 → **AC7 并发 1 兜底**：100 goroutine 同 (pet,slot) INSERT，InnoDB UNIQUE 索引 X-lock 串行化，1 个 INSERT 成功，99 个撞 1062 → `runEquipTx` `InsertInTx` 1062 翻译路径返业务 error → fn return → ROLLBACK
  - `UNIQUE KEY uk_user_cosmetic_item_id (user_cosmetic_item_id)` —— 一件实例同时只能装备一次 → **AC8 并发 2 兜底**：100 goroutine 同 user_cosmetic_item_id INSERT，1 成功 99 撞 1062 → ROLLBACK
- 本 story 验证的是"业务事务在 service 层逻辑兜底失效时，DB UNIQUE 作为最后一道防线兜住并发"——这是 NFR2 一致性约束的硬保障（epics.md §26.5 行 3608-3609 显式钦定 "DB UNIQUE(...)兜底"）。

### 范围红线（本 story 明确不做）

- **不**修改任何生产代码（`cosmetic_equip_service.go` / `user_pet_equip_repo.go` / 5 repo / handler / router / 0001-0016 migration / tx manager —— 26.2-26.4 已 done；本 story 仅消费 + fault wrapper 包装）。
- **不**修改 26.3 `EquipAndSwapSameSlot` + 26.4 `UnequipHappyPath` 既有 2 case + `buildCosmeticEquipServiceIntegration` 既有签名（仅追加新 case / 新 helper / 新 wrapper）。
- **不**用 sqlmock（epics.md 行 3614 钦定 dockertest 真实 MySQL；并发兜底 + 真 ROLLBACK 必须真 InnoDB）。
- **不**新建独立测试文件 / 不抽跨包 testutil（同文件同包内聚，与 4.7/11.9/20.9 同）。
- **不**测 handler 层 / HTTP envelope（epics.md §26.5 全在 service 层；handler 由 26.3/26.4 单测 + Story 28.1 E2E demo 覆盖）。
- **不**改 docs/*.md / 不新造错误码 / 不实装 GET /home pet.equips（26.6 owner）。
- **不**做 fuzz / 隔离级别 anomaly 专项 / ctx cancel 专项（已建立范式，不重复）。
- **不**给 Story 26.5 加 sprint-status retrospective 占位（epic-26 retrospective 已 optional，26.6 done + 整 epic done 才推）。

### 已落地可复用资产（避免重复造轮子）

- `server/internal/service/cosmetic_equip_service_integration_test.go`（26.3/26.4 落地）：`buildCosmeticEquipServiceIntegration`（行 47-87，起容器 + migrate + 装配真 svc + 真 tx.Manager + 返 rawDB + cleanup）+ `TestCosmeticEquipServiceIntegration_EquipAndSwapSameSlot`（行 92-162）+ `TestCosmeticEquipServiceIntegration_UnequipHappyPath`（行 175+）+ 文件顶部双行 build tag + import 块。本 story **同文件追加**，复用 helper / 顶部 tag / import（按需补 `sync` / `errors` / `apperror` import）。
- `server/internal/service/chest_open_service_integration_test.go`（20.9 落地）：`faultStepAccountRepoOnSpend` 等 4 个 fault wrapper（行 1441-1457+，按方法包装真实 repo 范式）+ `buildChestServiceWithRepos`（行 405-477，暴露原料供 fault 装配）+ `_Concurrent100SameKey`（行 810-940，100 goroutine 收集断言）+ `assertCount`（同包共享，断言 `SELECT COUNT(*) FROM <传入>`，内部已前缀 `SELECT COUNT(*) FROM `）。本 story 新 wrapper / 新 build helper / 并发 case **直接抄此模式改类型**。
- `server/internal/service/auth_service_integration_test.go`：`startMySQL`（dockertest 起 mysql:8.0 + 本机无 Docker `t.Skip` 兜底）/ `runMigrations`（跑到最新含 0012 seed + 0016 schema）—— 同包共享，本 story 经 `buildCosmeticEquipServiceIntegration` 间接复用。
- `server/internal/service/home_service_integration_test.go`：`insertUser` / `insertPet`（手工 INSERT 测试数据，不调 GuestLogin —— 同包共享）。
- `server/internal/service/cosmetic_service_integration_test.go`：`insertUserCosmeticItem`（INSERT user_cosmetic_items，可指定 status）/ `cosmeticIDByCode`（按 seed code 查 cosmetic_item_id —— 同包共享）。
- `server/internal/service/cosmetic_equip_service.go`（26.3/26.4 落地）：`EquipParams`/`EquipResult`/`UnequipParams`/`UnequipResult` DTO + `NewCosmeticEquipService(txMgr, userCosmeticRepo, cosmeticRepo, petRepo, userPetEquipRepo)` 构造（行 152-166）—— fault case 重装配 svc 用此构造（替换被 wrap 的 repo）。
- `server/internal/repo/mysql/`：`UserPetEquipRepo`（5 方法 interface，26.3/26.4 落地）/ `UserCosmeticItemRepo`（含 `UpdateStatusInTx`）/ `CosmeticItemRepo` / `PetRepo` —— fault wrapper 包装这些 interface（嵌入真实 impl 或持有字段委托透传）。
- `server/internal/pkg/errors/codes.go`（26.1 落地）：5002=`ErrCosmeticNotOwned` / 5004=`ErrCosmeticSlotMismatch`（26.4 Dev Notes 行 188 确认）+ 5003 常量（实装 `grep` 确认名）—— 边界 case 错误码断言用。
- `migrations/0012_*.sql`（cosmetic_items seed）/ `0016_*.sql`（user_pet_equips schema 两 UNIQUE）—— 实装阶段读 0012 确定 5 个不同 slot 的 seed code（**不**臆造）；0016 两 UNIQUE 是 AC7/AC8 并发兜底依据。

### Project Structure Notes

- **无新增文件**（全部追加到既有 `cosmetic_equip_service_integration_test.go`）：≥10 case + 2 fault wrapper + 2 helper（`buildCosmeticEquipServiceIntegrationWithRepos` / `assertEquipStateConsistency`）+ 文件顶部注释扩展 + 按需补 import（`sync` / `errors` / 已有 `apperror`）。
- 改既有文件（仅测试，1 个）：`server/internal/service/cosmetic_equip_service_integration_test.go`（追加，**不**改既有 2 case + `buildCosmeticEquipServiceIntegration` 签名）。
- 无改生产代码 / 无改 migration / 无改 docs。
- 与统一项目结构一致：service 集成测试在 `internal/service/` package `service_test`，build tag `integration` 隔离（与 26.3/26.4/20.9/11.9/4.7 完全同模式）。
- **检测到的变体（已说明）**：CLAUDE.md target 树写 `internal/domain/cosmetic/` 等，项目实际从未采用，按 `internal/service/` 既有结构落地（与 26.4 Dev Notes 行 196 同结论）。

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 26.5 行 3592-3616]（**唯一权威 AC 来源** —— 12 类场景：完整流程 / 同槽换装 / 3 回滚 / 2 并发(100) / 3 边界 / 1 状态一致性 + dockertest + build tag 钦定）
- [Source: _bmad-output/planning-artifacts/epics.md#Story 26.2 行 3526-3529]（0016 user_pet_equips 两 UNIQUE：uk_pet_slot + uk_user_cosmetic_item_id —— AC7/AC8 并发兜底依据）
- [Source: docs/宠物互动App_V1接口设计.md#8.3 穿戴装扮]（equip 服务端逻辑步骤 —— 回滚 1/2 注入点定位；26.1 冻结）
- [Source: docs/宠物互动App_V1接口设计.md#8.4 卸下装扮 行 1569-1661]（unequip 服务端逻辑步骤 —— 回滚 3 注入点定位；26.1 冻结 + fix-review 26-1 r2 [P1] 强化）
- [Source: docs/宠物互动App_数据库设计.md#5.10 user_pet_equips]（schema + uk_pet_slot UNIQUE(pet_id,slot) + uk_user_cosmetic_item_id UNIQUE —— 并发兜底 + 状态一致性矩阵依据）
- [Source: docs/宠物互动App_数据库设计.md#8.4 穿戴事务 行 1009-1018]（事务边界 —— 回滚原子性验证依据）
- [Source: docs/宠物互动App_数据库设计.md#6.8 slot 枚举 / #6.10 user_cosmetic_items.status 枚举]（slot `{1,2,3,4,5,6,7,99}` 完整流程 5 slot 选取 + status 1/2/3 边界 case 依据）
- [Source: _bmad-output/implementation-artifacts/26-3-post-cosmetics-equip-事务.md]（前序：`Equip`/`runEquipTx` + `UserPetEquipRepo` 3 方法 + `cosmetic_equip_service_integration_test.go` `EquipAndSwapSameSlot` + `buildCosmeticEquipServiceIntegration` 落地）
- [Source: _bmad-output/implementation-artifacts/26-4-post-cosmetics-unequip-事务.md]（前序：`Unequip`/`runUnequipTx` + `UserPetEquipRepo` +2 方法 + `UnequipHappyPath` 落地 + 错误码常量名确认行 188 + 范围红线行 165-176）
- [Source: _bmad-output/implementation-artifacts/20-9-layer-2-集成测试-开箱事务全流程.md]（**Layer 2 fault injection 范式直接参照** —— fault wrapper 行 1441-1457 / buildChestServiceWithRepos 行 405-477 / _Concurrent100SameKey 行 810-940 / §下游依赖行 83 显式点名 26.5 同模式）
- [Source: _bmad-output/implementation-artifacts/11-9-layer-2-集成测试-房间生命周期全流程.md]（Layer 2 收尾性集成测试同模式参照 —— 同包同文件追加 + dockertest + build tag + 不实装新业务功能定位）
- [Source: server/internal/service/cosmetic_equip_service_integration_test.go]（本 story 追加目标文件 —— 26.3/26.4 落地 helper + 既有 2 case + 顶部 build tag/import）
- [Source: server/internal/service/chest_open_service_integration_test.go]（20.9 fault wrapper / build helper / 并发 case / assertCount 实装参照）
- [Source: server/internal/service/cosmetic_equip_service.go]（26.3/26.4 `runEquipTx`/`runUnequipTx` 步骤 —— fault 注入点定位）
- [Source: server/internal/pkg/errors/codes.go]（26.1 落地 5002/5003/5004 错误码常量 —— 边界 case 断言）
- [Source: CLAUDE.md#Build & Test]（scripts/build.sh --test / --integration 验证契约 + 本机无 Docker 降级标准）
- [Source: _bmad-output/implementation-artifacts/decisions/0007-context-propagation.md]（ctx/txCtx 传播 §2.4 —— fault wrapper ctx 透传约束）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]（bmad-dev-story workflow，由 /epic-loop 派出的隔离 sub-agent）

### Debug Log References

- `bash scripts/build.sh --test`（repo 根目录运行；Go module 在 `server/`，build.sh 在 repo 根，脚本内部 cd 到 server/）→ vet + build + `go test -count=1 ./...` 全包单测 **BUILD SUCCESS**（所有包 ok / no test files；确认 integration build tag 隔离不破坏默认单测编译，本 story 不新增 unit test）。
- `cd server && go vet -tags=integration ./internal/service/` → **exit 0 无输出**（10 个新 case + 2 fault wrapper + 3 helper 编译干净，import `stderrors`/`sync` 正确引入）。
- `cd server && go test -tags=integration -list 'TestCosmeticEquipServiceIntegration.*' ./internal/service/` → 列出 **12 个**测试函数（既有 2：`EquipAndSwapSameSlot`/`UnequipHappyPath`；新增 10：`FullFlow_Equip5SlotsAll`/`StateConsistencyMatrix`/`EquipDeleteOldEquipFails_AllRollback`/`EquipUpdateStatusFails_AllRollback`/`UnequipUpdateStatusFails_AllRollback`/`Concurrent100SamePetSlot_FinalStateConsistent`/`Concurrent100SameInstanceDifferentPets_OnlyOneEquips`/`EquipConsumedInstance_Returns5003`/`EquipNotOwnedInstance_Returns5002`/`UnequipEmptySlot_Returns5004`）+ `ok` exit 0。
- **本机 Windows 无 Docker daemon → dockertest 集成测试无法实跑**（与 26.2/26.3/26.4 + 既有所有 `*_integration_test.go` 同环境约束；`startMySQL` 内 `t.Skip` 兜底，CI Linux 跑）。本 story 按 AC11 既定降级验收路径执行，本机可验证标准（编译通过 + 测试函数正确注册 + 非 integration 单测全绿）全部满足；集成测试**真实执行**归 CI Linux Docker 环境。这是已知环境约束，**非** HALT 条件（story AC11 已对齐）。

### Completion Notes List

- 全部修改集中在单一测试文件 `server/internal/service/cosmetic_equip_service_integration_test.go`（追加 ≈700 行）；**未改任何生产代码 / migration / docs / 既有 2 case / `buildCosmeticEquipServiceIntegration` 签名**（范围红线全遵守）。
- fault injection 范式严格抄 20.9 `faultStepAccountRepoOnSpend`（按方法包装真实 mysql repo + 目标方法返 injectErr 其余透传）；2 个新 wrapper 命名带 `Equip`/精确动作前缀避免与同包 `service_test` 20.9/4.7/11.9 冲突。
- 回滚 case 用真 `tx.NewManager(gormDB).WithTx` + fault wrapper 让 fn return error 触发真 InnoDB ROLLBACK，断言 DB 表恢复 setup 态（非 stub txManager，符合 §关键设计约束）。
- 5 slot seed code 从 `migrations/0012_seed_cosmetic_items.up.sql` 实读确定（hat_yellow=1/gloves_white=2/glasses_round=3/neck_blue=4/back_bag=5），**未臆造**。
- 错误码常量 `grep server/internal/pkg/errors/codes.go` 实读确认：5002=`ErrCosmeticNotOwned`(行 40)/5003=`ErrCosmeticInvalidState`(行 41)/5004=`ErrCosmeticSlotMismatch`(行 42)；断言用 `apperror.As(err)` + `ae.Code` 数值码。
- `assertEquipStateConsistency` 实装双向 SQL 不变量（正向 NOT EXISTS 子查询 + 反向 JOIN status<>2），横切全 case（完整流程末尾 / 3 回滚 ROLLBACK 后 / 2 并发终态 / 矩阵每步后）。

### File List

- `server/internal/service/cosmetic_equip_service_integration_test.go`（修改 —— 追加 Story 26.5 文件顶部注释段落 + `stderrors`/`sync` import + 2 fault wrapper + `buildCosmeticEquipServiceIntegrationWithRepos` + `assertEquipStateConsistency` + `requireEquipAppError` + 10 个新测试函数；既有 2 case + `buildCosmeticEquipServiceIntegration` 未动）

### Change Log

| 日期 | 变更 | 说明 |
|---|---|---|
| 2026-05-17 | Story 26.5 创建 | bmad-create-story 自动从 sprint-status.yaml 发现 epic-26 首个 backlog story（26-5）；状态 backlog → ready-for-dev；epic-26 已 in-progress 不变 |
| 2026-05-17 | Story 26.5 实装完成 | bmad-dev-story：cosmetic_equip_service_integration_test.go 追加 10 个 Layer 2 集成测试 case（完整流程/状态一致性矩阵/3 回滚/2 并发 100 goroutine/3 边界）+ 2 fault wrapper + 3 helper；build.sh --test BUILD SUCCESS + go vet -tags=integration 通过 + 12 个 test 函数正确注册（本机无 Docker，集成执行归 CI Linux）；状态 ready-for-dev → in-progress → review |
