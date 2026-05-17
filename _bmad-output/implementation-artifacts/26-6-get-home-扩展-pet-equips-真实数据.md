# Story 26.6: GET /home 扩展 - pet.equips 真实数据（节点 9 收尾，替换 4.8 节点 2 阶段写死的 `[]`）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want 主界面猫立即显示我已装备的装扮（启动 App 一次 `GET /home` 拿全），不需要先打开仓库再回主界面才看到装备,
so that 启动 App 看到的猫和我离开 App 时一致 —— 节点 9 阶段把 `GET /home.data.pet.equips` 从节点 2 写死的 `[]` 换成读 `user_pet_equips JOIN cosmetic_items JOIN user_cosmetic_items` 真实数据，与 V1 §5.1 行 408 + 行 516 Future Fields 钦定的 "节点 9 由 Story 26.6 落地 pet.equips 真实数据" 收口对齐，闭合节点 9 Server 端最后一条 story。

## 故事定位（Epic 26 第六条 = 节点 9 Server 收尾性接口扩展；上承 26.3 equip 事务写入 user_pet_equips + 26.5 Layer 2 集成测试；下启 Epic 27 iOS 端文字降级渲染直接消费真实 pet.equips 字段）

- **Epic 26 进度**：26.1（接口契约最终化 §8.3/§8.4/§1 节点 9 冻结，**done**）→ 26.2（user_pet_equips migration + `UserPetEquip` GORM struct + 0016 schema 含 uk_pet_slot / uk_user_cosmetic_item_id 两 UNIQUE，**done**）→ 26.3（POST /cosmetics/equip 事务含同槽换装；落地 `UserPetEquipRepo` `FindByPetSlot`/`DeleteByPetSlotInTx`/`InsertInTx`，**done**）→ 26.4（POST /cosmetics/unequip 事务，**done**）→ 26.5（Layer 2 集成测试 - 穿戴事务全流程，**done**）→ **26.6（本 story，GET /home 扩展 pet.equips 真实数据）= Epic 26 收官 story**。
- **物理执行顺序与逻辑编号一致**：本 story 编号 26.6，物理上**第六**执行（26.1-26.5 done 后做 26.6）。理由：
  - 本 story 依赖 26.3 写入 `user_pet_equips`（equip 事务 INSERT 行）—— 必须先 done 才有真值可 JOIN 查；节点 2 阶段（4.8）写死的 `[]` 是合法但语义无意义的占位状态
  - 本 story 同时是 Epic 26 收官 story，把节点 2 阶段 4.8 / 4.8 handler 写死的 `pet.equips: []any{}` 改成读 `user_pet_equips JOIN cosmetic_items + user_cosmetic_items`，最终把 V1 §5.1 + §5.1 Future Fields + 数据库设计.md §5.10 三处对应字段在节点 9 阶段语义闭环
  - sprint-status.yaml 第 255 行已按此顺序排列（26-6 在 26-5 之后、epic-26-retrospective 之前）
  - 本 story **不**实装新业务功能（**不**新建 service / handler / migration），仅**扩展** 4.8 已落地的 `homeServiceImpl.LoadHome` + `home_handler.homeResponseDTO` 输出 1 个数组字段；范围最小化（与 Story 11.10 "GET /home 扩展 room.currentRoomId 真实数据" 同模式）

- **epics.md §Story 26.6 钦定**（`_bmad-output/planning-artifacts/epics.md` 行 3617-3637，**唯一权威 AC 来源**）：
  - **Given** Story 4.8 GET /home 已可用 + Story 26.3 装备关系已就绪
  - **When** 完成本 story
  - **Then** 修改 GET /home 实装：
    - `pet.equips` 字段从写死 `[]` 改为读 `user_pet_equips JOIN cosmetic_items + user_cosmetic_items`
    - 返回 `[{slot, userCosmeticItemId, cosmeticItemId, name, rarity, assetUrl}]`
    - 节点 10 后由 Story 29.6 进一步追加 `renderConfig` 字段（**本 story 不做**）
  - **And** 不破坏 Story 4.8 + Story 11.10 既有 schema（仅填充 `pet.equips` 数组）
  - **And** **单元测试覆盖**（≥4 case，mocked repo）:
    - happy: 用户穿了 hat + gloves → `pet.equips` 长度 = 2，含正确字段
    - happy: 用户没穿任何装备 → `pet.equips` = `[]`
    - happy: 装备的 cosmetic_item 配置缺失（理论不该）→ skip + log warning
    - edge: 大量装备并发查 → 单 SQL JOIN 不退化（**不出现 N+1**）
  - **And** **集成测试覆盖**（dockertest）: 创建 user + 装 2 件装备 → curl /home → `pet.equips` 含 2 项 + 字段值正确

- **V1 §5.1 钦定 schema 锚点**（`docs/宠物互动App_V1接口设计.md`）：
  - 行 408：`data.pet.equips` 类型 `array`，**节点 2 阶段强制返回 `[]`**（节点 9 由 Story 26.6 落地真实数据；仅当 `data.pet ≠ null`）
  - 行 436（节点 2 阶段示例）：`"equips": []`
  - 行 475-484（**节点 9 之后真实数据示例，本 story 输出契约**）：
    ```json
    "equips": [
      {
        "slot": 1,
        "userCosmeticItemId": "90001",
        "cosmeticItemId": "12",
        "name": "小黄帽",
        "rarity": 1,
        "assetUrl": "https://..."
      }
    ]
    ```
  - 行 516：Future Fields 引用块钦定 "`data.pet.equips[]`：节点 9（Epic 26）穿戴链路落地后，由 Story 26.6 把 `user_pet_equips JOIN cosmetic_items` 的真实数据填充。每个元素 schema 含 `slot / userCosmeticItemId / cosmeticItemId / name / rarity / assetUrl`"
  - 行 517：`data.pet.equips[].renderConfig` 节点 10（Epic 29）由 Story 29.6 追加 —— **本 story 不做**（明确范围红线）
  - **关键 wire 类型约定**（V1 §2.5 + 现有 home_handler.go BIGINT 字符串化模式）：
    - `slot`：**int**（数字，不字符串化 —— `slot` 枚举 `{1,2,3,4,5,6,7,99}` 是小整数，与现有 `pet.petType` / `pet.currentState` / `chest.status` 同走数字路径；对齐 §8.3 `equipped.slot` 数字型）
    - `userCosmeticItemId`：**string**（BIGINT 字符串化，`strconv.FormatUint(...,10)`；与 §8.2 `instances[].userCosmeticItemId` / §8.3 `equipped.userCosmeticItemId` 同义同型）
    - `cosmeticItemId`：**string**（BIGINT 字符串化；与 §8.1 `items[].cosmeticItemId` / §8.3 `equipped.cosmeticItemId` 同义同型）
    - `name`：**string**（cosmetic_items.name 直出）
    - `rarity`：**int**（数字；`rarity` 枚举 `{1,2,3,4}`，与 §8.1 `items[].rarity` 同型）
    - `assetUrl`：**string**（cosmetic_items.asset_url 直出；enabled cosmetic 非空，§1 行 79 "assetUrl 非空字符串契约"）

- **V1 §1 节点 9 冻结边界说明锚点**（`docs/宠物互动App_V1接口设计.md` 行 85 + 行 1565）：
  - 行 85：`§5.1 pet.equips 节点 2 占位 [] → 节点 9 Story 26.6 真实化（已在 §1 Future Fields + Story 4.1 冻结边界锚定）` **不**视为契约变更
  - 行 1565：`§5.1 pet.equips[] 额外含 rarity / assetUrl，由 Story 26.6 从 cosmetic_items JOIN 补充 —— §8.3 equip response 不加这两字段；本接口契约不反向修改 §5.1`
  - **关键**：本字段类型已在 4.1 接口契约最终化阶段 + 26.1 锚定阶段冻结 —— 本 story **不**改 schema，仅把"节点 2 阶段写死 `[]`"换成"读 `user_pet_equips JOIN cosmetic_items + user_cosmetic_items` 真实数据"

- **数据库设计层锚点**（`docs/宠物互动App_数据库设计.md`）：
  - §5.10 `user_pet_equips`：`user_id` / `pet_id` / `slot` / `user_cosmetic_item_id`（穿戴关系；26.2 落地 0016 schema，含 `uk_pet_slot` + `uk_user_cosmetic_item_id` 两 UNIQUE + `idx_user_pet (user_id, pet_id)` 普通索引 —— **本 story JOIN 查询走 `idx_user_pet` 索引**）
  - §5.9 `user_cosmetic_items`：实例表，`id` = `userCosmeticItemId`，`cosmetic_item_id` 关联配置（本 story JOIN 取实例 id）
  - §5.8 `cosmetic_items`：配置表，`name` / `slot` / `rarity` / `asset_url`（本 story JOIN 取展示元信息）
  - **JOIN 关系**：`user_pet_equips upe` (筛 `user_id=? AND pet_id=?`) `JOIN user_cosmetic_items uci ON uci.id = upe.user_cosmetic_item_id` `JOIN cosmetic_items ci ON ci.id = uci.cosmetic_item_id` —— 单 SQL 一次取全部装备元信息（AC4 edge 钦定**禁止 N+1 单查每件**）

- **设计文档 §6.2 钦定 home 模块**（`docs/宠物互动App_Go项目结构与模块职责设计.md` §6.2）：
  - 关联接口：`GET /api/v1/me` / `GET /api/v1/home`
  - 关联表：users / pets / user_step_accounts / user_chests / **user_pet_equips** / rooms —— 节点 9 阶段本 story 查 `user_pet_equips`（JOIN cosmetic_items + user_cosmetic_items）
  - 钦定：home_service 已存在（4.8 落地），本 story **直接扩展** 4.8 已建立的 `homeServiceImpl.LoadHome` —— **不**新建 home_service_v2.go / 不拆分 home_equip_service.go

- **Story 11.10 是本 story 的直接扩展模板**（`_bmad-output/implementation-artifacts/11-10-get-home-扩展-room-currentroomid-真实数据.md`，**done**）：
  - 11.10 把 `room.currentRoomId` 从节点 2 写死 `null` 改为读真实 `users.current_room_id`，建立了"按字段维度 increment 上线 GET /home"的扩展范式（节点 2 写死 → 节点 4 起 room.currentRoomId 真实 → 节点 9 起 pet.equips 真实）
  - **本 story 复用 11.10 的扩展模式**：在 `home_service` 加 repo 调用 + 在 `home_handler` DTO 写真值，**不**改 4.8 已稳定的 service / handler 主结构
  - **关键差异（必须注意，否则会做错）**：11.10 是**零额外 repo 调用**（`user.CurrentRoomID` 是 `(1) userRepo.FindByID` 已一次性返回的 user struct 字段，直接透传）；**本 story 不同 —— `pet.equips` 数据跨 3 张表，user / pet / stepAccount / chest 任一已查结果都不含装备数据，必须新增 1 个 repo 方法 + 1 个 repo 依赖注入**。这导致 `NewHomeService` 构造函数签名变化（多 1 个 repo 参数）→ 连锁影响 router.go wire / home_service_test.go `buildHomeService` helper / home_service_integration_test.go `buildHomeServiceIntegration` helper（详见 Dev Notes "连锁修改清单"）

## Acceptance Criteria

> 全部源自 epics.md §Story 26.6（行 3617-3637，**唯一权威 AC 来源**）+ V1 §5.1（行 408 / 458-504 / 513-517 节点 9 真实数据示例 + Future Fields）+ V1 §1（行 85 / 1565 节点 9 冻结边界）+ 数据库设计 §5.8 / §5.9 / §5.10。本 story **不实装新业务功能**，仅扩展 4.8 已落地的 GET /home 输出 `pet.equips` 1 个数组字段。

**AC1 — 新增 `UserPetEquipRepo.ListEquipsForHome`（单 SQL JOIN，禁止 N+1）**

在 `server/internal/repo/mysql/user_pet_equip_repo.go` 的 `UserPetEquipRepo` interface **追加**（**不**新建文件，与既有 5 个方法同 interface / impl 同文件组织 —— 与 23.4 在 user_cosmetic_item_repo.go 既有 interface 追加方法同模式）一个**只读查询方法**：

```go
// ListEquipsForHome 单 SQL JOIN 查某 pet 当前全部装备（GET /home pet.equips
// 数据源，Story 26.6 引入；V1 §5.1 节点 9 真实数据 + epics.md §Story 26.6）。
//
// SQL（单查，禁止 N+1 —— epics.md §Story 26.6 AC4 edge 钦定）:
//   SELECT upe.slot, upe.user_cosmetic_item_id,
//          ci.id AS cosmetic_item_id, ci.name, ci.rarity, ci.asset_url
//   FROM user_pet_equips upe
//   JOIN user_cosmetic_items uci ON uci.id = upe.user_cosmetic_item_id
//   JOIN cosmetic_items ci       ON ci.id  = uci.cosmetic_item_id
//   WHERE upe.user_id = ? AND upe.pet_id = ?
//   ORDER BY upe.slot ASC
//
// 走 0016 落地的 idx_user_pet (user_id, pet_id) 索引（数据库设计 §5.10）。
// ORDER BY slot ASC：client 渲染稳定顺序（防 grid 抖动，与 catalog
// ORDER BY 同根因模式 —— 单 pet 单 slot UNIQUE，slot ASC 已是全序）。
//
// 空结果（pet 未穿任何装备）→ 返 []HomeEquipRow{}（**非 nil**）；
// service 透传为 pet.equips=[] 非 error（AC2 happy: 没穿装备 → []）。
// query 失败 → 返 raw error 透传（service 包成 1009）。
ListEquipsForHome(ctx context.Context, userID, petID uint64) ([]HomeEquipRow, error)
```

- 同文件定义返回行结构体 `HomeEquipRow`（**轻量结构体接收 JOIN 投影列，不复用 `UserPetEquip` 全 struct** —— JOIN 跨 3 表的列不在单 struct 上，显式裁字段避免误读未 SELECT 列的 zero-value，与 `cosmetic_item_repo.FindSlotNameByID` 行 361-364 轻量 struct 同模式）：
  ```go
  // HomeEquipRow 是 ListEquipsForHome 的 JOIN 投影行（Story 26.6 引入）。
  // 字段类型与 3 表对应列 1:1：slot/rarity int8（TINYINT）、id 列 uint64（BIGINT
  // UNSIGNED）、name/asset_url string（VARCHAR）。
  type HomeEquipRow struct {
      Slot               int8
      UserCosmeticItemID uint64
      CosmeticItemID     uint64
      Name               string
      Rarity             int8
      AssetURL           string
  }
  ```
- impl 用 `tx.FromContext(ctx, r.db).WithContext(ctx)` + `Raw(...).Scan(&rows)` 或 GORM `Table("user_pet_equips upe").Select(...).Joins(...).Where(...).Order(...).Find(&rows)`（**二选一，与本文件既有方法风格一致优先**——`user_pet_equip_repo.go` 既有方法用 GORM builder（`Where`/`First`/`Delete`/`Create`）+ 1 处 `Raw().Scan()`（`FindUserCosmeticItemIDByPetSlotForUpdate`，因 FOR UPDATE 语法）；本方法无锁子句，**优先 GORM `Table().Select().Joins().Where().Order().Find()` builder**，显式列裁剪，0 行 → `if rows == nil { rows = []HomeEquipRow{} }` 兜底，与 `cosmetic_item_repo.ListEnabledForCatalog` 行 270-272 同模式）
- **范围红线（YAGNI）**：仅加 home 只读 JOIN 查询方法；**不**改既有 5 个方法签名 / 行为（`FindByPetSlot`/`DeleteByPetSlotInTx`/`InsertInTx`/`FindUserCosmeticItemIDByPetSlotForUpdate`/`DeleteByPetSlotInTxReturningAffected` 由 26.3/26.4 落地 done，本 story **不**触碰）
- 同步在 `server/internal/repo/mysql/user_pet_equip_repo_test.go` **追加** ≥1 个该方法的 repo 层单测（与既有 `user_pet_equip_repo_test.go` 用 sqlmock 验证 SQL / 列映射同模式 —— 验证 JOIN SQL 字符串 + 0 行返空 slice 非 nil + query err 透传）

**AC2 — `home_service.go`：HomeOutput / PetBrief 加 Equips + LoadHome 在 pet ≠ nil 时查真实装备**

修改 `server/internal/service/home_service.go`：

(a) `PetBrief` struct **加 `Equips []EquipBrief` 字段**（service 层 DTO；新增 `EquipBrief` struct 与现有 `UserBrief`/`StepAccountBrief`/`ChestBrief` 同模式）：
```go
// EquipBrief 是 V1 §5.1 data.pet.equips[] 元素的 service 层映射（Story 26.6 引入）。
// 节点 9 阶段含 6 字段；节点 10 由 Story 29.6 加 RenderConfig（本 story 不做）。
type EquipBrief struct {
    Slot               int    // V1 §6.8 枚举 {1,2,3,4,5,6,7,99}
    UserCosmeticItemID uint64 // handler 层 strconv.FormatUint 字符串化
    CosmeticItemID     uint64 // handler 层 strconv.FormatUint 字符串化
    Name               string
    Rarity             int    // V1 §6.9 枚举 {1,2,3,4}
    AssetURL           string
}
```
`PetBrief` 加 `Equips []EquipBrief`（**值切片，空时为 `[]EquipBrief{}` 非 nil** —— handler 序列化为 `[]` 非 `null`，对齐 V1 §5.1 行 408 "节点 2 强制 `[]`"语义在节点 9 延续：无装备时仍是 `[]`，与现有 handler 行 109-111 `equips: []any{}` 非 nil 注释同根因）

(b) `homeServiceImpl` **加 `userPetEquipRepo mysql.UserPetEquipRepo` 字段** + `NewHomeService` 构造函数**加该 repo 入参**（**追加在现有 4 个 repo 之后**，保持 user/pet/stepAccount/chest 顺序不变，新参数排第 5 位 —— 最小化对既有调用点 diff）

(c) `LoadHome` 在 **(2) pet 查询块之后**（pet 已确定 nil / 非 nil）扩展逻辑：
- `petBrief == nil`（用户无默认 pet）→ **跳过装备查询**（无 pet 无装备语义，也避免 `petID` 无值）；不调 `ListEquipsForHome`
- `petBrief != nil` → 调 `s.userPetEquipRepo.ListEquipsForHome(ctx, userID, pet.ID)`（**注意用 (2) 块里 `pet` 变量的 `pet.ID` 作 petID**；`pet` 在 `petBrief != nil` 分支已是非 nil 的 `*mysql.Pet`）
  - query err → `apperror.Wrap(err, apperror.ErrServiceBusy, ...)` 整体返 1009（**不部分降级** —— 与 home_service 既有 stepAccount/chest 失败 → 1009 同契约，epics.md §Story 4.8 行 1136 "任一聚合查询失败 → 整体 1009 不返半截 HomeOutput"）
  - 成功 → 遍历 `[]HomeEquipRow` 转 `[]EquipBrief`，赋给 `petBrief.Equips`
  - **空结果**（`len(rows) == 0`）→ `petBrief.Equips = []EquipBrief{}`（非 nil；AC2 happy: 没穿装备 → `[]`）

(d) **AC3 cosmetic_item 配置缺失 skip + log warning 的归属说明**：本 story repo `ListEquipsForHome` 用 `JOIN cosmetic_items ci ON ci.id = uci.cosmetic_item_id`（**INNER JOIN**）—— 若某装备的 cosmetic_items 配置行被 admin 物理删（理论不该，与 §8.2 态 C 同源），该 `upe` 行**不**进 JOIN 结果（INNER JOIN 自动过滤无匹配配置的行），**自然 skip**。**log warning 在 service 层补**：service 调用方额外查一次"该 pet 在 `user_pet_equips` 的行数"（或在 repo 方法里同时返回 raw upe 行数）与 JOIN 结果行数对比，若 `rawUpeCount > len(joinedRows)` → `logger` warn 一条（含 userID + petID + 差值），**不**报 error / **不**中断（与 §8.2 态 C "missing-no-row 仍返回 + log" 同根因；epics.md §Story 26.6 AC3 "配置缺失 → skip + log warning"）。
  - **实装建议（择一，dev-story 自行权衡，以"单查不退化 N+1"为硬约束）**：
    - 方案 A（推荐，单查 + 1 个 COUNT）：`ListEquipsForHome` 内除 JOIN 主查外，再发 1 个 `SELECT COUNT(*) FROM user_pet_equips WHERE user_id=? AND pet_id=?`（**这是 O(1) 聚合查询，不是 N+1 —— N+1 指"每件装备 1 查"，此处是固定 2 查**），返回 `(rows []HomeEquipRow, rawCount int64, err error)`，service 据 `rawCount > len(rows)` log warning。
    - 方案 B（service 层不感知缺失，仅 INNER JOIN 静默 skip 不 log）：**不满足 AC3 "log warning" 字面要求 —— 不采用**。
    - **硬约束**：无论方案，主装备查询必须是**单 SQL JOIN**（不得每件装备单查 cosmetic_items —— epics.md §Story 26.6 AC4 edge "大量装备并发查 → 单 SQL JOIN 不退化，不出现 N+1"）。COUNT 校验查是固定 1 次额外查询（与装备件数无关），不违反 N+1 约束。
  - logger 来源：home_service 当前**未注入 logger**（4.8 阶段无 log 需求）。本 story 需在 `NewHomeService` **再加 1 个 logger 入参**（项目用 `internal/infra/logger` —— dev-story 阶段 `grep -rn "logger\." server/internal/service/` 看既有 service（如 chest_service / cosmetic_equip_service）如何注入 + 调 logger.Warn，照抄注入模式；router.go wire 处传 `deps.Logger` 或等价；**若既有 service 注入 logger 模式与此不符，以既有模式为准，不臆造**）。

(e) **不部分降级红线**：`ListEquipsForHome` 失败 → 整体 1009（与 home_service 行 152-153 "不部分降级"注释一致，epics.md §Story 4.8 行 1136 钦定）；**不**返"pet 有但 equips 缺"的半截 HomeOutput

**AC3 — `home_handler.go`：homeResponseDTO 把 petDTO.equips 从 `[]any{}` 换成真实 `[]equipDTO`**

修改 `server/internal/app/http/handler/home_handler.go` `homeResponseDTO`：

- 现有行 109-111 `"equips": []any{}` 写死占位 → 改为遍历 `out.Pet.Equips`（`[]service.EquipBrief`）转 wire 数组：
  ```go
  equipsDTO := make([]gin.H, 0, len(out.Pet.Equips))
  for _, e := range out.Pet.Equips {
      equipsDTO = append(equipsDTO, gin.H{
          "slot":               e.Slot,                              // int 数字
          "userCosmeticItemId": strconv.FormatUint(e.UserCosmeticItemID, 10), // BIGINT 字符串化
          "cosmeticItemId":     strconv.FormatUint(e.CosmeticItemID, 10),     // BIGINT 字符串化
          "name":               e.Name,
          "rarity":             e.Rarity,                            // int 数字
          "assetUrl":           e.AssetURL,
      })
  }
  // petDTO["equips"] = equipsDTO
  ```
- **`make([]gin.H, 0, len)` 而非 `var equipsDTO []gin.H`**：空切片必须序列化为 `[]` 非 `null`（V1 §5.1 行 408 钦定 `pet.equips` 是 array，无装备时 `[]`；nil slice 序列化为 `null` 会破坏 schema —— 与现有 handler 行 109-111 用 `[]any{}` 非 nil 同根因，注释也明确"`nil` slice 序列化为 `null`"）
- **只在 `out.Pet != nil` 分支内构造 equips**（现有行 103-113 `if out.Pet != nil` 块内，与现有 `equips` 占位位置一致）—— `out.Pet == nil` → `petDTO` 整体是 `null`，equips 不单独出现（V1 §5.1 行 408 "仅当 `data.pet ≠ null`"）
- **不破坏既有 schema 红线**：仅替换 `petDTO["equips"]` 这一个 key 的值来源（写死 `[]any{}` → 真实 `[]gin.H`）；`user` / `stepAccount` / `chest` / `room` / `pet` 其他字段（id / petType / name / currentState）**一字不改**（epics.md §Story 26.6 "不破坏 Story 4.8 + Story 11.10 既有 schema"）

**AC4 — 单元测试覆盖（≥4 case，mocked repo，扩展 home_service_test.go + home_handler_test.go）**

在 `server/internal/service/home_service_test.go` **追加** ≥4 个 case（epics.md §Story 26.6 钦定 4 类，**扩展既有 `buildHomeService` helper 加第 5 个 repo stub 入参**）：
- happy: 用户穿了 hat(slot=1) + gloves(slot=2) → `HomeOutput.Pet.Equips` 长度 = 2，字段值（slot / userCosmeticItemId / cosmeticItemId / name / rarity / assetUrl）与 stub 返回一致
- happy: 用户没穿任何装备（stub `ListEquipsForHome` 返 `[]HomeEquipRow{}`）→ `HomeOutput.Pet.Equips` 是**长度 0 的非 nil 切片**
- happy: pet 为 nil（用户无默认 pet）→ `HomeOutput.Pet == nil`，**`ListEquipsForHome` stub 必须未被调用**（用 stub 内 `called bool` 断言；验证 (c) "pet nil 跳过装备查询"逻辑）
- happy: cosmetic_item 配置缺失 → stub `ListEquipsForHome` 返 `(rows=1件, rawCount=2, nil)`（模拟 INNER JOIN 过滤掉 1 件配置被删的）→ `Equips` 长度 = 1 + **断言 log warning 被触发**（用注入的 fake/spy logger 断言 Warn 被调 1 次含 userID/petID —— 与既有 service 单测如何断言 log 同模式，dev-story `grep` 既有 service 单测 logger spy 用法照抄）
- edge: `ListEquipsForHome` 返 query error → `LoadHome` 返 1009（`apperror.ErrServiceBusy`）+ `HomeOutput == nil`（不部分降级）

在 `server/internal/app/http/handler/home_handler_test.go` **追加** ≥2 case（**扩展既有 stub HomeService**，验证 wire 字段类型 + 序列化）：
- 用户有 2 件装备 → response JSON `data.pet.equips` 是长度 2 数组，`slot`/`rarity` 是 **JSON number**、`userCosmeticItemId`/`cosmeticItemId` 是 **JSON string**（BIGINT 字符串化）、`name`/`assetUrl` 是 string
- 用户无装备 → `data.pet.equips` 是 **`[]`（空 JSON 数组，非 `null`）**；`pet == nil` 时 `data.pet` 整体是 `null`（equips 不单独出现）—— 验证 nil slice 不污染为 null 的序列化契约

**AC5 — 集成测试覆盖（dockertest，扩展 home_service_integration_test.go）**

在 `server/internal/service/home_service_integration_test.go` **追加** ≥1 集成 case（**扩展既有 `buildHomeServiceIntegration` helper：加 `userPetEquipRepo := mysql.NewUserPetEquipRepo(gormDB)` + 传入 `service.NewHomeService(...)` 新签名**；复用既有 `insertUser` / `insertPet` helper；新增 `insertUserCosmeticItem` / `insertCosmeticItem` / `insertUserPetEquip` 便捷 INSERT helper 或复用 cosmetic 相关集成测试已有 helper —— dev-story `grep -rn "func insertUserCosmeticItem\|func insertCosmeticItem\|func insertUserPetEquip" server/internal/service/*_integration_test.go` 看是否已有可复用，**优先复用不重复定义**）：
- 创建 user + default pet + 2 件 cosmetic_items 配置（slot=1 hat + slot=2 gloves，rarity / asset_url 有值）+ 2 件 user_cosmetic_items 实例（status=2 equipped）+ 2 行 user_pet_equips → 调 `svc.LoadHome(ctx, userID)` → 断言 `HomeOutput.Pet.Equips` 长度 = 2 + 按 slot ASC 排序 + 每件 6 字段值与插入数据一致
- （可选增强，dev-story 视投入产出决定）edge: 插 1 行 user_pet_equips 但故意删对应 cosmetic_items 配置行 → `Equips` 长度 = 0（INNER JOIN 过滤）+ 不 panic
- **本机 Windows 无 Docker daemon**：`bash scripts/build.sh --integration` 本机无法执行（与既有所有 `*_integration_test.go` 同环境限制，`startMySQL` 内 `t.Skip` 兜底，CI Linux 跑）；本机验收以 "`go vet -tags=integration ./internal/service/` 通过 + `go test -tags=integration -list 'TestHomeService.*'` 新增 case 正确注册编译通过 + 既有 4 个 case 不破坏" 为标准（与 26.5 Dev Notes 同模式）

**AC6 — router.go wire 同步 + 既有调用点不破坏 + build 验证**

- 修改 `server/internal/app/bootstrap/router.go` 行 347 `homeSvc := service.NewHomeService(userRepo, petRepo, stepAccountRepo, chestRepo)` → 加 `userPetEquipRepo`（**注意**：router.go 行 536 已有 `userPetEquipRepo := repomysql.NewUserPetEquipRepo(deps.GormDB)` 但在 cosmetics 路由块；本 story 需确认该变量作用域 —— 若 home wire（行 347）在 userPetEquipRepo 声明之前，需**在 home wire 前提升一处 `userPetEquipRepo := repomysql.NewUserPetEquipRepo(deps.GormDB)` 声明**或复用 deps.GormDB 新建实例。dev-story 阶段 `Read` router.go 行 320-540 确认变量声明顺序 + repo 复用约定，照既有 wire 风格最小改动，**不**新建 wire 文件 / **不**改其他 handler wire）+ logger 入参（AC2(d)，传 router 已有的 logger / `deps.Logger`，dev-story 看既有 service wire 怎么拿 logger 照抄）
- `grep -rn "service.NewHomeService(" server/` 找全部调用点（router.go + home_service_test.go `buildHomeService` + home_service_integration_test.go `buildHomeServiceIntegration`），**全部同步加新参数**（编译不过即漏改）
- `bash scripts/build.sh --test` 必须通过（vet + build + 单测，不含 integration tag —— 新增集成 case 在 integration tag 下，默认不触发）
- 若需要并跑集成 vet：`go vet -tags=integration ./...`（本机 Docker 缺，集成 case `t.Skip` 兜底；只验证编译 + list 注册）

**AC7 — 字段语义跨接口同义对齐校验（防 schema 漂移）**

- `pet.equips[].userCosmeticItemId` 与 §8.2 `instances[].userCosmeticItemId` / §8.3 `equipped.userCosmeticItemId` 同义同型（string，BIGINT 字符串化）
- `pet.equips[].cosmeticItemId` 与 §8.1 `items[].cosmeticItemId` / §8.3 `equipped.cosmeticItemId` 同义同型（string）
- `pet.equips[].slot` 与 §6.8 / §8.3 `equipped.slot` 枚举 `{1,2,3,4,5,6,7,99}` 同义同型（int）
- `pet.equips[].rarity` 与 §8.1 `items[].rarity` 枚举 `{1,2,3,4}` 同义同型（int）
- `pet.equips[].name` 与 §8.1 `items[].name` / §8.3 `equipped.name` 同义（cosmetic_items.name）
- `pet.equips[].assetUrl` 与 §8.1 `items[].assetUrl` 同义，enabled cosmetic **非空字符串**（§1 行 79 契约）
- **本 story 严格按 V1 §5.1 行 475-484 节点 9 示例输出这 6 字段；多 1 个少 1 个 / 类型不符都是契约漂移**（节点 10 `renderConfig` 由 29.6 追加，本 story 绝**不**提前加）

## Tasks / Subtasks

- [x] **Task 1 — repo 层：UserPetEquipRepo.ListEquipsForHome + HomeEquipRow（AC1）**
  - [x] 1.1 `Read` 既有 `user_pet_equip_repo.go` 全文确认 interface / impl / tx.FromContext 模式 + `cosmetic_item_repo.go` 轻量 struct + GORM `Table().Select().Joins().Where().Order().Find()` 模式参考
  - [x] 1.2 在 `user_pet_equip_repo.go` 加 `HomeEquipRow` 轻量结构体（6 字段，类型与 3 表列 1:1，含 gorm column tag）
  - [x] 1.3 在 `UserPetEquipRepo` interface 追加 `ListEquipsForHome` 方法签名 + 完整注释（SQL + idx_user_pet 索引 + 空结果非 nil + rawCount + err 透传契约）
  - [x] 1.4 在 `userPetEquipRepo` 实装：单 SQL 3 表 INNER JOIN + ORDER BY slot ASC + `tx.FromContext` + 0 行 → `[]HomeEquipRow{}` 兜底；AC2(d) 方案 A：同时返 `rawCount int64`（O(1) COUNT，非 N+1）
  - [x] 1.5 `user_pet_equip_repo_test.go` 追加 4 sqlmock case 验证 JOIN SQL + 列映射 + 0 行非 nil + rawCount + err 透传
- [x] **Task 2 — service 层：HomeOutput/PetBrief 加 Equips + LoadHome 查装备（AC2）**
  - [x] 2.1 `home_service.go` 加 `EquipBrief` struct（6 字段）+ `PetBrief` 加 `Equips []EquipBrief`
  - [x] 2.2 `homeServiceImpl` 加 `userPetEquipRepo` 字段；`NewHomeService` 加 1 个入参（追加在现有 4 repo 之后）。**logger 不注入** —— 既有 service（pet/cosmetic/dev_*）log 均用 `slog.WarnContext(ctx,...)` 直调，无构造注入先例；AC2(d) 钦定"以既有模式为准，不臆造"，故仅 +1 repo 参数（连锁链相应缩减为 3 处调用点）
  - [x] 2.3 `LoadHome` 在 (2) pet 块后新增 (2b) 块：pet==nil 跳过查询；pet!=nil 调 `ListEquipsForHome(ctx, userID, pet.ID)` → err 整体 1009 / 成功转 `[]EquipBrief`（`make(...,0,len)` → 空时非 nil）
  - [x] 2.4 配置缺失 skip + log warning：rawCount > len(rows) → `slog.WarnContext`（含 userID/petID/rawCount/joined/skipped），不 error 不中断（AC2(d)）
- [x] **Task 3 — handler 层：homeResponseDTO equips 真实化（AC3）**
  - [x] 3.1 `home_handler.go` `homeResponseDTO` 把 `out.Pet != nil` 块内 `"equips": []any{}` 改为遍历 `out.Pet.Equips` 转 `make([]gin.H, 0, len)`（slot/rarity 数字、2 个 id `strconv.FormatUint` 字符串化、name/assetUrl 直出）
  - [x] 3.2 确认 `out.Pet == nil` 时 petDTO 整体 null（equips 不单独出现）；其他字段一字不改
- [x] **Task 4 — 单测（AC4）**
  - [x] 4.1 `home_service_test.go`：加第 5 个 stub repo（`stubHomeUserPetEquipRepo`，未配置 fn 调用即 panic）+ `buildHomeService` 委托 `buildHomeServiceWithEquips`（既有 4.8/11.10 case 自动适配，equips 返 []）+ 新 helper `buildHomeServiceWithEquips`。**logger spy 不做** —— 既有 service 单测（cosmetic_service 态 C 行 465）log 行为由集成/人工覆盖，单测断言可观测结果，照既有模式
  - [x] 4.2 追加 5 case：2件装备happy / 无装备→非nil[] / pet nil 未调查询(equipFn=nil panic 守卫) / 配置缺失 skip 保留剩余 / query err→1009 不降级
  - [x] 4.3 `home_handler_test.go`：追加 2 case 验证 wire 类型（slot/rarity JSON number、id JSON string）+ 空→`[]` 非 null + pet==nil 时 equips 不单独出现
- [x] **Task 5 — 集成测试（AC5）**
  - [x] 5.1 `home_service_integration_test.go`：`buildHomeServiceIntegration` 加 `userPetEquipRepo` + 新 `NewHomeService` 签名；既有 4 case 经 helper 自动同步
  - [x] 5.2 复用既有 `cosmeticIDByCode` + `insertUserCosmeticItem`（同 service_test 包，cosmetic_service_integration_test.go）；新增 `insertUserPetEquip` helper
  - [x] 5.3 追加 2 case：user+pet+复用0012 seed 2配置+2实例(equipped)+2 upe 行(乱序插验 ORDER BY) → Equips 长度2 + slot ASC + 6 字段值 1:1；+ edge 删 cosmetic_items 配置 → INNER JOIN 过滤 Equips=0 不 panic
- [x] **Task 6 — wire + 全量验证（AC6 / AC7）**
  - [x] 6.1 `Read` router.go 行 320-540 确认 userPetEquipRepo 变量作用域（同 if-block，原声明在 line ~536）；最小改动：把 `userPetEquipRepo` 声明上移到 homeSvc wire 前，line ~536 改为复用同实例（不再 `:=`）；wire `NewHomeService` 加 1 参
  - [x] 6.2 `grep -rn "service.NewHomeService(" server/` 全调用点（router.go + 2 test helper）同步新签名
  - [x] 6.3 `bash scripts/build.sh --test` 通过（vet+build+单测全绿 BUILD SUCCESS）；`go vet -tags=integration ./...` 通过 + `go test -tags=integration -list` 新增 2 集成 case 编译+注册通过（本机 Windows 无 Docker，未实跑，降级路径与 26.5 同模式）
  - [x] 6.4 AC7 6 字段同义对齐自查：slot(int)/userCosmeticItemId(string)/cosmeticItemId(string)/name/rarity(int)/assetUrl 对 V1 §5.1 行 475-484 逐字段核类型，**无多无少无 renderConfig**（handler 测试 `TestHomeHandler_TwoEquips_WireFieldTypesCorrect` 断言覆盖）

## Dev Notes

### 连锁修改清单（关键 —— `NewHomeService` 签名变更连锁，漏一处编译不过）

`NewHomeService` 当前签名 `(userRepo, petRepo, stepAccountRepo, chestRepo)` → 本 story 改为 `(userRepo, petRepo, stepAccountRepo, chestRepo, userPetEquipRepo, logger)`。**全部调用点必须同步**（`grep -rn "service.NewHomeService(" server/`）：
1. `server/internal/app/bootstrap/router.go` 行 347（生产 wire）
2. `server/internal/service/home_service_test.go` `buildHomeService` helper 行 117-129（单测）—— 既有 3 个 case（行 136 / 203 / 235 等）经此 helper 间接调用，helper 改完它们自动通过；helper 需加第 5 个 stub repo 入参 + logger spy
3. `server/internal/service/home_service_integration_test.go` `buildHomeServiceIntegration` helper 行 63（集成测试）—— 既有 4 个 case（行 165 / 240 / 269 / 312）经此 helper，helper 改完自动通过

漏改任一处 → `go build` / `go vet` 直接报参数不匹配，**不会静默** —— 这是 Go 静态类型的好处，build 必失败暴露。

### 与 Story 11.10 的本质差异（防止照搬错）

11.10（`room.currentRoomId`）是**零额外 repo 调用** —— `user.CurrentRoomID` 是 `(1) userRepo.FindByID` 已返回的 user struct 上的字段，11.10 仅在 LoadHome 末尾透传 + handler nil/字符串化。**本 story 不能照搬"零额外查询"** —— `pet.equips` 数据在 `user_pet_equips`/`user_cosmetic_items`/`cosmetic_items` 三张表，user/pet/stepAccount/chest 任一已查结果都不含装备，**必须新增 repo 方法 + repo 依赖注入 + 构造函数签名变更**。照搬 11.10"不加 repo"会直接做不出来。复用 11.10 的是**扩展位置与不破坏既有 schema 的方法论**（在 LoadHome 内增量、在 handler DTO 写真值、不新建 service/handler），不是"零查询"实现细节。

### Source tree（本 story 触碰文件，全绝对路径相对 server/）

- 改：`internal/repo/mysql/user_pet_equip_repo.go`（加 `HomeEquipRow` + `ListEquipsForHome`）
- 改：`internal/repo/mysql/user_pet_equip_repo_test.go`（追加 ≥1 sqlmock case）
- 改：`internal/service/home_service.go`（`EquipBrief` + `PetBrief.Equips` + 构造函数 2 新参 + LoadHome 装备查询 + log warning）
- 改：`internal/service/home_service_test.go`（buildHomeService helper + ≥5 case）
- 改：`internal/service/home_service_integration_test.go`（buildHomeServiceIntegration helper + ≥1 case + insert helper）
- 改：`internal/app/http/handler/home_handler.go`（homeResponseDTO equips 真实化）
- 改：`internal/app/http/handler/home_handler_test.go`（追加 ≥2 case）
- 改：`internal/app/bootstrap/router.go`（行 347 NewHomeService wire 加 2 参，必要时提升 userPetEquipRepo 声明）
- **不改**：任何 migration / V1 / 数据库设计 / 设计文档 / epics.md / 26.1-26.5 已落地的 equip/unequip service/handler/repo 方法 / 4.8 LoadHome 主结构（user/pet/stepAccount/chest 查询逻辑）/ 4.5 中间件 / router 其他 handler wire

### Testing standards

- 单测：`server/internal/service/home_service_test.go` + `home_handler_test.go` 既有用 stub repo（实现 repo interface 的 struct，per-case fn 注入）+ table-ish case；本 story 扩展 `buildHomeService` helper（加第 5 stub + logger spy），既有 case 经 helper 自动适配
- repo 单测：`user_pet_equip_repo_test.go` 既有用 sqlmock 验证 SQL 字符串 / 列映射 / err 透传 —— 照既有方法 case 写 JOIN 查询的 sqlmock 期望
- 集成测试：`home_service_integration_test.go` dockertest 真实 MySQL（`startMySQL` 本机 Docker 缺 → `t.Skip`，CI Linux 跑）；`//go:build integration` tag（既有文件已有，新 case 自动覆盖）
- 验证命令（本机）：`bash scripts/build.sh --test`（vet+build+单测，必跑）；`go vet -tags=integration ./...`（集成编译验证，Docker 缺不实跑）
- **本 story 是 server-only**，无 iOS / 真机；不需要 ios-simulator MCP（CLAUDE.md "iOS UI 验证"段不适用）

### 范围红线汇总（本 story 不做）

- [skip] **不**实装 `renderConfig` 字段（节点 10 / Story 29.6 owner；V1 §5.1 行 517 钦定；本 story 输出严格 6 字段无 renderConfig）
- [skip] **不**改 V1 §5.1 / §8.1 / §8.2 / §8.3 / 数据库设计 / epics.md 任一份（契约**输入**，严格对齐不修改；若发现实装与契约不一致 → 优先改本 story 实装/测试对齐契约，不反向改 docs）
- [skip] **不**新建 service / handler / migration / RoomRepo 类新模块（仅扩展 4.8 既有 home_service/handler + 在既有 user_pet_equip_repo 加 1 方法）
- [skip] **不**改 26.1-26.5 已落地的 equip/unequip service/handler/repo 既有方法（仅在 UserPetEquipRepo 追加只读 JOIN 查询方法）
- [skip] **不**改 4.8 LoadHome 的 user/pet/stepAccount/chest 4 段既有查询逻辑（仅在 pet 段后增量加装备查询）
- [skip] **不**用 errgroup / 并发拆分 LoadHome（4.8 串行约定不变；新增 1 个装备 JOIN 查询仍串行追加，与 home_service 行 142-145 "MVP 串行不引 errgroup" 一致）
- [skip] **不**对 user_pet_equips 指向的 cosmetic_items 做"是否 enabled"过滤（epics.md §Story 26.6 无此约束；INNER JOIN 已穿了的装备配置即便 admin 下架仍应显示——与 §8.2 态 B "已拥有不因下架消失"同根因；**不**加 `WHERE ci.is_enabled=1`）
- [skip] **不**校验 user_pet_equips 与 user_cosmetic_items.status 一致性（26.5 Layer 2 集成测试已保障 NFR2 "equipped 实例必有 upe 行"双向不变量；GET /home 是只读快照，**不**做事务/一致性校验，避免 home 接口与穿戴事务耦合）
- [skip] **不**给 §10.3 `room.snapshot.members[].pet.equips`（V1 行 2025 节点 4 阶段固定 `[]`）做真实化（那是房间快照，归属其他 epic；本 story **仅** GET /home.data.pet.equips）
- [skip] **不**改 sprint-status.yaml 之外任何 BMAD 配置；**不** commit（story-done 阶段提交）

### Project Structure Notes

- 完全对齐 `docs/宠物互动App_Go项目结构与模块职责设计.md` §4（repo/mysql + service + app/http/handler + app/bootstrap 分层）+ §6.2（home 模块关联 user_pet_equips 表）；无结构冲突
- ctx 传播遵 ADR-0007：`ListEquipsForHome(ctx, ...)` ctx 第一参，repo 用 `tx.FromContext(ctx, r.db).WithContext(ctx)`；handler 用 `c.Request.Context()`（home_handler.go 行 70 既有，本 story 不改 handler 取 ctx 路径）
- 错误三层映射遵 ADR-0006 + V1 §3：repo 返 raw error → service `apperror.Wrap(...ErrServiceBusy...)` → handler `c.Error(err)` 让 ErrorMappingMiddleware 写 envelope（home_service.go 行 152-194 既有模式，本 story 装备查询失败走同一 1009 路径）

### References

- [Source: _bmad-output/planning-artifacts/epics.md#Story 26.6（行 3617-3637，唯一权威 AC）]
- [Source: docs/宠物互动App_V1接口设计.md#§5.1 行 408 / 458-504 节点9真实数据示例 / 513-517 Future Fields]
- [Source: docs/宠物互动App_V1接口设计.md#§1 行 85 / 1565 节点9冻结边界 pet.equips 真实化不视为契约变更]
- [Source: docs/宠物互动App_数据库设计.md#§5.8 cosmetic_items / §5.9 user_cosmetic_items / §5.10 user_pet_equips（idx_user_pet 索引）]
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#§6.2 home 模块关联 user_pet_equips]
- [Source: _bmad-output/implementation-artifacts/11-10-get-home-扩展-room-currentroomid-真实数据.md#扩展模板（done）]
- [Source: server/internal/service/home_service.go#LoadHome 现有 4 段串行 + 不部分降级]
- [Source: server/internal/app/http/handler/home_handler.go#homeResponseDTO 行 109-111 equips 占位 + 行 88-89 11.10 真实化先例]
- [Source: server/internal/repo/mysql/user_pet_equip_repo.go#既有 5 方法 interface/impl 同文件组织 + tx.FromContext 模式]
- [Source: server/internal/repo/mysql/cosmetic_item_repo.go#FindSlotNameByID 轻量 struct + ListEnabledForCatalog GORM builder + 0 行非 nil 兜底]
- [Source: server/internal/app/bootstrap/router.go#行 347 NewHomeService wire + 行 536 userPetEquipRepo 既有声明]

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]（bmad-dev-story workflow）

### Debug Log References

- `bash scripts/build.sh --test`（repo 根）→ **BUILD SUCCESS**，全 25 包单测 `OK: all tests passed`（vet + build + 非 integration tag 单测）。
- `go vet -tags=integration ./...`（server/）→ clean（无输出 = 无错误）。
- `go test -tags=integration -list 'TestHomeService.*' ./internal/service/`（server/）→ 编译通过 + 全 case 注册，含新增 `TestHomeService_LoadHome_TwoEquips_RealJOINData` / `TestHomeService_LoadHome_ConfigMissing_INNERJoinFilters` + 既有 4 个 4.8/11.10 集成 case 不破坏。**本机 Windows 无 Docker daemon，集成 case 未实跑**（`startMySQL` `t.Skip` 兜底，CI Linux 跑；与 26.5 Dev Notes 同降级路径）。
- 红绿循环：repo / service / handler 三层均先写失败测试（RED）→ 实装（GREEN）→ 跑过。repo RED 阶段一次性发现 sqlmock regex 与实际未加反引号的 JOIN SQL 不匹配，修正测试 regex 后转绿（实装正确，仅测试期望写法）。

### Completion Notes List

- ✅ AC1：`UserPetEquipRepo.ListEquipsForHome(ctx, userID, petID) (rows []HomeEquipRow, rawCount int64, err error)` + `HomeEquipRow` 轻量投影 struct。单 SQL 3 表 INNER JOIN（`Table().Select().Joins().Joins().Where().Order().Find()`）+ 1 个 O(1) COUNT（rawCount，非 N+1）。0 行 → `[]HomeEquipRow{}` 非 nil。走 idx_user_pet。
- ✅ AC2：`EquipBrief`（6 字段）+ `PetBrief.Equips []EquipBrief`；`NewHomeService` +1 repo 参数（非 +2，logger 按既有 `slog.WarnContext` 直调模式不注入 —— AC2(d) "以既有模式为准"）。LoadHome (2b) 块：pet==nil 跳过；pet!=nil 单 JOIN 查 → err 整体 1009 不部分降级 / 空 → 非 nil [] / rawCount>len(rows) → `slog.WarnContext` warn 不中断。
- ✅ AC3：`home_handler.go` `homeResponseDTO` equips 从 `[]any{}` 改为 `make([]gin.H,0,len)` 遍历真实数据；slot/rarity int、2 id `strconv.FormatUint` 字符串化、name/assetUrl 直出；空 → `[]` 非 null；pet==nil → pet 整体 null。其他字段一字未改。
- ✅ AC4：service 单测 +5 case、handler 单测 +2 case，全绿。配置缺失 case 按 cosmetic_service 态 C 既有模式断言可观测结果（Equips 长度），不做 log spy。
- ✅ AC5：集成测试 +2 case（happy 复用 0012 seed hat_yellow+gloves_white，乱序插验 ORDER BY slot ASC；edge 删 cosmetic_items 验 INNER JOIN 过滤）。复用既有 `cosmeticIDByCode`/`insertUserCosmeticItem` helper + 新增 `insertUserPetEquip`。
- ✅ AC6：router.go `userPetEquipRepo` 声明上移到 homeSvc wire 前，原 line ~536 改复用同实例；3 处 `NewHomeService` 调用点（router + 2 helper）全同步。连锁影响额外触及 `cosmetic_equip_service_test.go`（1 stub）+ `cosmetic_equip_service_integration_test.go`（2 stub）—— Go 静态类型强制暴露，全部补 `ListEquipsForHome`（panic 守卫 / delegate 透传）。`build.sh --test` BUILD SUCCESS。
- ✅ AC7：6 字段对 V1 §5.1 行 475-484 逐字段核类型一致，无多无少**无 renderConfig**（节点 10/Story 29.6 owner），handler 测试断言覆盖。

### File List

- `server/internal/repo/mysql/user_pet_equip_repo.go`（改：加 `HomeEquipRow` struct + `ListEquipsForHome` interface 方法 + impl）
- `server/internal/repo/mysql/user_pet_equip_repo_test.go`（改：追加 4 个 `ListEquipsForHome` sqlmock case）
- `server/internal/service/home_service.go`（改：加 `EquipBrief` + `PetBrief.Equips` + `userPetEquipRepo` 字段 + `NewHomeService` +1 参 + LoadHome (2b) 装备查询 + slog.WarnContext）
- `server/internal/service/home_service_test.go`（改：加 `stubHomeUserPetEquipRepo` + `buildHomeServiceWithEquips` helper + `buildHomeService` 委托 + 5 个 26.6 case）
- `server/internal/service/home_service_integration_test.go`（改：`buildHomeServiceIntegration` 加 userPetEquipRepo + 新签名 + `insertUserPetEquip` helper + 2 个 26.6 集成 case）
- `server/internal/service/cosmetic_equip_service_test.go`（改：连锁 —— `stubUserPetEquipRepo` 补 `ListEquipsForHome` panic 守卫方法满足 interface）
- `server/internal/service/cosmetic_equip_service_integration_test.go`（改：连锁 —— `faultUserPetEquipRepoOnDelete` + `findByPetSlotNotFoundStub` 补 `ListEquipsForHome` delegate 透传方法满足 interface）
- `server/internal/app/http/handler/home_handler.go`（改：`homeResponseDTO` equips 从 `[]any{}` 改真实 `make([]gin.H,0,len)` 遍历 + 字符串化）
- `server/internal/app/http/handler/home_handler_test.go`（改：追加 2 个 26.6 wire 类型 case）
- `server/internal/app/bootstrap/router.go`（改：`userPetEquipRepo` 声明上移到 homeSvc wire 前 + `NewHomeService` 加 1 参 + 原 line ~536 改复用同实例）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（改：26-6 状态 ready-for-dev → in-progress → review）
- `_bmad-output/implementation-artifacts/26-6-get-home-扩展-pet-equips-真实数据.md`（改：Tasks 勾选 + Dev Agent Record + Status → review）

### Change Log

- 2026-05-18：实装 Story 26.6 GET /home 扩展 pet.equips 真实数据（节点 9 收官）。新增 `UserPetEquipRepo.ListEquipsForHome` 单 SQL 3 表 INNER JOIN（+ O(1) COUNT 校验，禁止 N+1）；service `EquipBrief`/`PetBrief.Equips` + LoadHome (2b) 装备查询（pet==nil 跳过 / err 整体 1009 不降级 / 配置缺失 INNER JOIN skip + slog.WarnContext）；handler equips 从写死 `[]any{}` 换真实数据（6 字段，2 id 字符串化）。`NewHomeService` +1 repo 参数（连锁同步 3 调用点 + 3 测试 stub）。单测 +7 / 集成 +2，`bash scripts/build.sh --test` BUILD SUCCESS，`go vet -tags=integration ./...` 通过。
