# Story 11.10: GET /home 扩展 - room.currentRoomId 真实数据（节点 4 收尾，替换 4.8 节点 2 阶段写死的 null）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As an iPhone 用户,
I want 主界面"进入房间"按钮能反映我当前是否已在房间（如已在，按钮文案变"返回房间 #xxx"）,
so that 启动 App 后立即知道自己房间状态，不需要先点按钮才发现 / 也不需要在 §10.2 `GET /rooms/current` 之外串一次 server 调用 —— 节点 4 阶段直接通过 `GET /home.data.room.currentRoomId` 一次拉到首页全部数据（含房间归属），与 V1 §5.1 行 374 + Future Fields §5.1 行 475 钦定的 "节点 4 起由 Story 11.10 注入真实数据" 收口对齐。

## 故事定位（Epic 11 第十条 = 节点 4 收尾性 server 接口扩展；上承 11.9 Layer 2 集成测试 + 4.8 GetHome 节点 2 框架；下启 Epic 12 iOS 端房间页 UI 复用真实 currentRoomId 字段）

- **Epic 11 进度**：11.1 (契约定稿，done) → 11.2 (rooms / room_members migration，done) → 11.3 (POST /rooms 创建房间事务，done) → 11.4 (POST /rooms/{roomId}/join 加入房间事务，done) → 11.5 (POST /rooms/{roomId}/leave 退出房间事务，done) → 11.6 (GET /rooms/current + GET /rooms/{roomId} 房间详情查询，done) → 11.7 (room.snapshot 真实实装替换 E10.7 placeholder，done) → 11.8 (成员加入/离开 WS 广播 + close 4007 unregister leaver Session，done) → 11.9 (Layer 2 集成测试 - 房间生命周期全流程，done) → **11.10（本 story，GET /home 扩展 room.currentRoomId 真实数据）**。
- **物理执行顺序与逻辑编号一致**：本 story 编号 11.10，物理上**第十**执行（11.3-11.9 done 后做 11.10）。理由：
  - 本 story 依赖 11.3 / 11.4 / 11.5 写入 `users.current_room_id`（Story 11.3 `users.current_room_id = roomID`，11.5 `users.current_room_id = NULL`）—— 它们**必须先 done** 才有真值可读；空读字段 (节点 2 阶段的 4.8) 是合法但语义无意义状态
  - 本 story 同时是 Epic 11 收官 story，把节点 2 阶段 4.8 写死的 `room.currentRoomId: null` 改成读真实 `users.current_room_id`，最终把 V1 §5.1 + §5.1 Future Fields + 数据库设计.md §5.1 三处对应字段在节点 4 阶段语义闭环
  - sprint-status.yaml 第 155 行已按此顺序排列（11.10 在 11.9 之后、epic-11-retrospective 之前）
  - 本 story **不**实装新业务功能（**不**新建 service / handler / repo / migration），仅**扩展** 4.8 已落地的 home_service.LoadHome + home_handler 输出 1 个字段；范围最小

- **epics.md §Story 11.10 钦定**（`_bmad-output/planning-artifacts/epics.md` 行 2022-2041，**唯一权威 AC 来源**）：
  - **Given** Story 4.8 GET /home 已可用 + Story 11.3 用户加入房间会更新 `users.current_room_id`
  - **When** 完成本 story
  - **Then** 修改 GET /home 实装：
    - room 字段从写死 `{currentRoomId: null}` 改为读 `users.current_room_id`
    - 如有 → `{currentRoomId: "xxx"}` 字符串形式（按 AR21 ID 字符串约定）
    - 如无 → `{currentRoomId: null}`
  - **And** 不破坏 Story 4.8 既有 schema（仅填充 room 字段真实值）
  - **And** **单元测试覆盖**（≥3 case，mocked repo，**扩展** Story 4.8 既有单测）：
    - happy: 用户在房间 → /home response.room.currentRoomId = 该房间 id
    - happy: 用户不在任何房间 → response.room.currentRoomId = null
    - edge: `users.current_room_id` 指向已 closed 的房间（理论不该）→ 仍返回该 id（client 调 /rooms/{id} 时会得到 6005 自行处理）
  - **And** **集成测试覆盖**（dockertest）: 创建 user → join room → curl /home → room.currentRoomId 正确

- **V1 §5.1 钦定 schema 锚点**（`docs/宠物互动App_V1接口设计.md` 行 374 + 行 475）：
  - 行 374：`data.room.currentRoomId` 类型 `string | null`，**节点 2 阶段强制 `null`**（节点 4 由 Story 11.10 注入真实数据）
  - 行 408（节点 2 阶段示例）：`"currentRoomId": null`
  - 行 456（节点 9 之后真实数据示例）：`"currentRoomId": "3001"`（注：节点 4 ~ 节点 8 阶段亦走该真实路径，因为节点 4 起本字段由 11.10 落地真实数据；节点 9-10 示例只是顺带展示节点更后期的 pet.equips / renderConfig 联动，不代表 currentRoomId 节点 4 / 5 / 6 / 7 / 8 还是 null）
  - 行 475：Future Fields 引用块钦定 "`data.room.currentRoomId`：节点 4 起由 Story 11.10 注入真实房间 ID"
  - **关键**：本字段类型已在 4.1 接口契约最终化阶段 + 11.1 锚定阶段冻结为 `string | null` —— 本 story **不**改 schema，仅把"节点 2 阶段写死 null"换成"读真实 `users.current_room_id`，BIGINT 字符串化"

- **V1 §10 末尾 Story 11.10 锚点**（V1 §10 行 1611）：
  - `> §10 房间章节末尾引用：GET /home.data.room.currentRoomId 由 Story 11.10 真实实装（节点 4，Epic 11 收官）—— 本 story（11.1）不改 §5.1 GET /home schema；§5.1 已在 Story 4.1 锚定 + Future Fields 已标注 room.currentRoomId 节点 4 由 Story 11.10 注入真实数据。`
  - **解读**：11.1 在锚定 §10 房间章节时，**显式引用** 11.10 是 §5.1 GET /home 房间字段实装的**唯一权威 story**，并提示：本字段语义在 V1 §5.1 + §5.1 Future Fields 双重收口，节点 4 起由 11.10 一次性落地真实读取 + 字符串化 + 可空处理；后续节点（5/6/7/8）**不**会再改本字段语义。

- **数据库设计层**（`docs/宠物互动App_数据库设计.md` §5.1）：
  - users 表 `current_room_id BIGINT UNSIGNED NULL`：快照字段，由 Story 11.3 / 11.4 写入真实 roomID，11.5 写入 NULL
  - 索引 `KEY idx_current_room_id (current_room_id)`：业务上不通过该索引查 home（home 是 PK 查 user 的衍生数据），但保留该索引服务于 future room_member 反向查询场景
  - 与 V1 §10.2 `GET /rooms/current` 字段语义**等价**：两者都查 `users.current_room_id`，但 `GET /rooms/current` 是房间页内单字段轻量查询，本 story（GET /home）是首页聚合接口（含 user / pet / stepAccount / chest / room 五段）

- **设计文档 §6.2 钦定 home 模块**（`docs/宠物互动App_Go项目结构与模块职责设计.md` §6.2 行 364-389）：
  - 关联接口：`GET /api/v1/me` / `GET /api/v1/home`
  - 关联表：users / pets / user_step_accounts / user_chests / user_pet_equips / **rooms**（节点 4 阶段本 story 不查 rooms 表，仅消费 users.current_room_id 快照字段；不需要 RoomRepo）
  - 钦定：home_service 已经存在（4.8 落地），本 story **直接扩展** 4.8 已建立的 `homeServiceImpl.LoadHome` 函数 —— **不**新建 home_service_v2.go / 不拆分 home_room_service.go

- **下游立即依赖**：
  - **Epic 12 iOS 房间 epic**（节点 4 iOS 端，sprint-status.yaml 行 158-165）：iOS HomeViewModel.loadHome 已经在 4.8 落地时拿到 `data.room.currentRoomId`（节点 2 阶段恒为 null），本 story 上线后该字段会有真实值，iOS 端**不**需要任何 schema 改动，仅 UI 行为根据 `currentRoomId != nil` 决定按钮文案（"进入房间" vs "返回房间 #xxx"）；本 story 上线后 iOS 端 ProfileView 也可以选择性消费该字段（但与 ProfileView 无关 —— 那是 GET /me，详见 V1 §4.3 + Story 4.1 ADR-0006-ish 决策）
  - **Story 26.6**（节点 9 穿戴 epic GET /home 扩展 pet.equips 真实数据）：本 story 落地后 GetHome 已经具备"按字段维度 increment 上线"的扩展模板（节点 2 阶段所有字段写死 → 节点 4 起 room.currentRoomId 真实 → 节点 9 起 pet.equips 真实）；26.6 直接复用本 story 的扩展模式（在 home_service 加 1 个 repo 调用 + 在 home_handler DTO 写真值），**不**改 4.8 已经稳定的 service / handler 主结构

- **范围红线**：本 story **修改** `server/internal/service/home_service.go`（HomeOutput 加 RoomCurrentRoomID *uint64 字段 + LoadHome 在 user 已查到的基础上消费 user.CurrentRoomID 写入 HomeOutput.RoomCurrentRoomID）+ `server/internal/app/http/handler/home_handler.go`（homeResponseDTO 把 `out.Room.CurrentRoomID == nil` 写 `nil` / 否则 strconv.FormatUint 转字符串）+ `server/internal/service/home_service_test.go`（**追加** ≥3 case）+ `server/internal/service/home_service_integration_test.go`（**追加** ≥1 集成 case：user join room 后 LoadHome.RoomCurrentRoomID 是 roomID）+ `server/internal/app/http/handler/home_handler_test.go`（**追加** ≥2 case 覆盖 wire 字段类型 + null 序列化 vs string 序列化）；**不**新建 service / handler / repo / migration / DTO 模块；**不**改 GET /me / 不实装 §10.x 房间任一接口；**不**改 11.3 / 11.4 / 11.5 写 users.current_room_id 的事务路径；**不**改 V1 / 数据库设计 / 设计文档任一份。

**本 story 不做**（明确范围红线）：

- [skip] **不**实装 GET /me handler（V1 §4.3 + Story 4.8 红线已锚定；epics.md §Story 11.10 没有 /me）
- [skip] **不**实装 §10.x 房间任一新接口（POST /rooms / POST /rooms/{id}/join / POST /rooms/{id}/leave / GET /rooms/current / GET /rooms/{id}：11.3 ~ 11.6 已 done，本 story 仅消费 users.current_room_id 字段值，**不**调任一房间接口）
- [skip] **不**修改 11.3 / 11.4 / 11.5 写 `users.current_room_id` 的事务路径（已 done；本 story 仅读取该字段）
- [skip] **不**新建 RoomRepo（home_service **不**通过 rooms 表查询本字段；只通过 users.current_room_id 快照字段，user 已经在 4.8 阶段查过，无需第二次 DB round-trip）
- [skip] **不**新建 home_room_service.go / home_service_v2.go（Story 4.8 已落地 home_service.go，本 story 在已有文件内扩展）
- [skip] **不**修改 4.5 中间件（auth + rate_limit；本 story 不改 router wire）
- [skip] **不**修改 0001 migration 或 users 表结构（current_room_id 字段已存在，由 11.2 ~ 11.5 落地写入路径，本 story 仅读取）
- [skip] **不**修改 4.4 token util / 4.6 auth_service / 4.7 集成测试（本 story 仅消费 4.5 已注入的 userID + 4.8 已落地的 LoadHome 入口，**不**触碰 auth 模块）
- [skip] **不**改 sprint-status.yaml 之外的任何 BMAD 配置
- [skip] **不**改 V1 / 数据库设计 / 设计文档 / epics.md 任一份（本 story 严格按 epics.md §Story 11.10 行 2022-2041 钦定 AC 实装，不修改契约文件）
- [skip] **不**给 GET /me 提前占位返 currentRoomId（V1 §4.3 行 299 钦定 GET /me.user.currentRoomId **始终**返回 null —— 这是**永久 schema 占位**，与本 story GET /home.room.currentRoomId 路径**不**等价；详见 V1 §4.3 行 329 引用块）
- [skip] **不**用并发查询 / errgroup 拆并行调 4 + 1 个 repo（user.CurrentRoomID 是 user struct 已有字段，**零** 额外 repo 调用，串行不变）
- [skip] **不**校验 users.current_room_id 指向的房间是否真实存在（epics.md §Story 11.10 edge case 钦定 "指向已 closed 的房间 → 仍返回该 id"；client 调 /rooms/{id} 时由 11.6 ACL 走 6004 / 6005 自行处理；本 story **不**做 server 端预校验，避免 home 接口与房间业务耦合）

## Acceptance Criteria

**AC1 — `internal/service/home_service.go`：HomeOutput 加 RoomBrief 段 + LoadHome 填真实 currentRoomId**

修改 `server/internal/service/home_service.go`：

(a) 在 `HomeOutput` struct 加 Room 字段（与现有 User / Pet / StepAccount / Chest 同模式）：

```go
// HomeOutput 是 service 层 DTO（**不是** wire DTO，handler 转换为 V1 §5.1 钦定 wire 格式）。
//
// 字段语义：
//   - User: 必有（登录后 user 必然存在；查询失败 → 1009）
//   - Pet: 可空（用户无默认 pet → nil；V1 §5.1 钦定的 edge case）
//   - StepAccount: 必有（登录初始化时已建；缺 → 1009）
//   - Chest: 必有 + Status / RemainingSeconds 已动态计算（不是 DB 原值）
//   - Room: 必有容器（即便用户不在任何房间也是 RoomBrief{} 而非 nil）—— V1 §5.1 行 374
//     钦定 data.room **容器永远存在**，currentRoomId 字段才可空；详见 RoomBrief 注释
type HomeOutput struct {
    User        UserBrief
    Pet         *PetBrief
    StepAccount StepAccountBrief
    Chest       ChestBrief
    Room        RoomBrief
}
```

(b) 新增 `RoomBrief` struct（**只**含 currentRoomId 一个字段；明确**不**含 roomCode 等节点 4 阶段未上线的字段）：

```go
// RoomBrief 是 V1 §5.1 data.room 的 service 层映射。
//
// **节点 4 阶段唯一字段**：CurrentRoomID（*uint64，nil = 用户不在任何房间）。
//
// **不**含 roomCode / room.id / memberCount 等：V1 §5.1 data.room 在节点 4 阶段
// 仅声明 `currentRoomId: string | null` 一个字段（详见 V1 §5.1 行 374）；房间详情
// 由 §10.2 GET /rooms/current + §10.3 GET /rooms/{id} 单独查询。本 struct 故意只含
// 1 个字段而非提前展开 —— 任何字段扩展（如 roomCode）由 future epic 决策（不在 V1 §5.1
// schema 钦定范围内 → 不属于本 story）。
//
// 字段类型 *uint64 而非 uint64：
//   - users.current_room_id 是 BIGINT UNSIGNED NULL（数据库设计 §5.1）
//   - 用户不在任何房间 → user.CurrentRoomID == nil → 本字段也是 nil
//   - 用户在房间 → user.CurrentRoomID == &roomID → 本字段也是 &roomID
//   - 与 mysql.User.CurrentRoomID 字段类型 1:1 对齐，handler 层做 nil → null /
//     非 nil → strconv.FormatUint 转字符串两路分支
type RoomBrief struct {
    CurrentRoomID *uint64 // nil = 用户不在任何房间（V1 §5.1 行 374 可空语义）
}
```

(c) 修改 `LoadHome` 函数：在 (1) 拿到 user 后，把 `user.CurrentRoomID` 直接写入 HomeOutput.Room.CurrentRoomID（**零**额外 repo 调用 —— users.current_room_id 是 users 表字段，FindByID 已经一次性返回；与 4.8 节点 2 阶段红线 "**不**读 users.current_room_id 字段，节点 2 阶段强制 null" 的语义反转）：

```go
// (1) user — 必有
user, err := s.userRepo.FindByID(ctx, userID)
if err != nil {
    return nil, apperror.Wrap(err, apperror.ErrServiceBusy, apperror.DefaultMessages[apperror.ErrServiceBusy])
}

// ... (2) (3) (4) (5) 不变 ...

return &HomeOutput{
    User: UserBrief{
        ID:        user.ID,
        Nickname:  user.Nickname,
        AvatarURL: user.AvatarURL,
    },
    Pet: petBrief,
    StepAccount: StepAccountBrief{
        TotalSteps:     stepAccount.TotalSteps,
        AvailableSteps: stepAccount.AvailableSteps,
        ConsumedSteps:  stepAccount.ConsumedSteps,
    },
    Chest: ChestBrief{
        ID:               chest.ID,
        Status:           chestStatus,
        UnlockAt:         chest.UnlockAt.UTC(),
        OpenCostSteps:    chest.OpenCostSteps,
        RemainingSeconds: remainingSeconds,
    },
    // Room: V1 §5.1 行 374 钦定 currentRoomId 类型 string | null；
    // **节点 4 阶段实装** —— users.current_room_id 是 *uint64 直接转 *uint64 透传 ——
    // 用户不在任何房间 → user.CurrentRoomID == nil → RoomBrief{CurrentRoomID: nil}
    // 用户在房间 X → user.CurrentRoomID == &X → RoomBrief{CurrentRoomID: &X}
    Room: RoomBrief{CurrentRoomID: user.CurrentRoomID},
}, nil
```

**关键设计约束**：

- **零额外 repo 调用**：users.current_room_id 字段已经包含在 4.8 阶段的 `userRepo.FindByID` 返回值里（mysql.User struct 行 38 钦定 `CurrentRoomID *uint64`）；本 story **不**新建 UserRepo.FindCurrentRoomIDByUserID 等单字段查询 —— 与 V1 §10.2 单字段轻量查询走 RoomRepo（11.6 落地）的接口分工不同：home 是聚合接口，所有字段都从已查的 5 个 repo 输出里拼装
- **不**校验 currentRoomId 指向的房间是否存在 / 是否 closed：epics.md §Story 11.10 edge case 钦定 "users.current_room_id 指向已 closed 的房间（理论不该）→ 仍返回该 id（client 调 /rooms/{id} 时会得到 6005 自行处理）"；本 service 严格透传 user.CurrentRoomID，**不**做 rooms 表 cross-check
- **不**改 4.8 已落地的 4 个 repo 调用 / chest 动态判定 / 错误处理路径：本 story 范围最小，仅在 (1) user 查询返回值上多消费 1 个字段 + 在 HomeOutput 上多 1 个 Room 段
- **HomeOutput.Room 是值类型 RoomBrief 而非 *RoomBrief**：V1 §5.1 行 374 钦定 data.room 是**容器**对象（永远存在），data.room.currentRoomId 才是可空字段；handler 永远输出 `"room": {"currentRoomId": ...}` 而非 `"room": null`
- **RoomBrief.CurrentRoomID 是 *uint64 指针类型**：与 mysql.User.CurrentRoomID 字段类型 1:1 对齐；handler 层做 nil → null / 非 nil → strconv.FormatUint 转字符串

**关键反模式**：

- [bad] **不**改 home_service 的依赖列表加 RoomRepo（无 rooms 表查询需求）
- [bad] **不**用并发查询（errgroup）—— 0 额外 repo 调用，并发引入复杂度无收益
- [bad] **不**让 RoomBrief 在用户不在任何房间时返 nil 指针（违反 V1 §5.1 行 374 钦定 data.room **容器永远存在**）
- [bad] **不**返回 `RoomBrief{CurrentRoomID: &0}` 来表达"用户不在任何房间"（uint64(0) 是合法 BIGINT 值不是 sentinel；nil 才是；与 mysql.User.CurrentRoomID *uint64 语义对齐）
- [bad] **不**在 service 层做 currentRoomId 字符串化（service 层用原生 uint64；wire 字符串化是 handler 的职责，与 4.8 既有 user.id / pet.id / chest.id 走同模式）

**AC2 — `internal/app/http/handler/home_handler.go`：homeResponseDTO 把 nil 写 null / 非 nil 写字符串**

修改 `server/internal/app/http/handler/home_handler.go` 的 `homeResponseDTO` 函数（**仅**改 room 段；其他段保持不变）：

```go
func homeResponseDTO(out *service.HomeOutput) gin.H {
    var petDTO any
    if out.Pet != nil {
        petDTO = gin.H{
            "id":           strconv.FormatUint(out.Pet.ID, 10),
            "petType":      out.Pet.PetType,
            "name":         out.Pet.Name,
            "currentState": out.Pet.CurrentState,
            "equips":       []any{},
        }
    }

    // room.currentRoomId: 节点 4 阶段由 Story 11.10 注入真实数据。
    //
    //   - out.Room.CurrentRoomID == nil → wire 写 null（用户不在任何房间）
    //   - out.Room.CurrentRoomID != nil → wire 写 strconv.FormatUint(...)（BIGINT 字符串化）
    //
    // V1 §5.1 行 374 钦定 currentRoomId 类型 string | null；BIGINT 字符串化对齐 V1 §2.5
    // (避免 JS Number 精度丢失 + AR21 ID 字符串约定)；nil 序列化为 JSON null 走 any 显式
    // 类型路径，**不**用 *string 等过度迂回。
    var currentRoomIDDTO any
    if out.Room.CurrentRoomID != nil {
        currentRoomIDDTO = strconv.FormatUint(*out.Room.CurrentRoomID, 10)
    }
    // currentRoomIDDTO 默认零值 = nil interface{} → json 序列化为 null

    return gin.H{
        "user": gin.H{
            "id":        strconv.FormatUint(out.User.ID, 10),
            "nickname":  out.User.Nickname,
            "avatarUrl": out.User.AvatarURL,
        },
        "pet": petDTO,
        "stepAccount": gin.H{
            "totalSteps":     out.StepAccount.TotalSteps,
            "availableSteps": out.StepAccount.AvailableSteps,
            "consumedSteps":  out.StepAccount.ConsumedSteps,
        },
        "chest": gin.H{
            "id":               strconv.FormatUint(out.Chest.ID, 10),
            "status":           out.Chest.Status,
            "unlockAt":         out.Chest.UnlockAt.Format(time.RFC3339),
            "openCostSteps":    out.Chest.OpenCostSteps,
            "remainingSeconds": out.Chest.RemainingSeconds,
        },
        "room": gin.H{
            // 节点 4 阶段（Story 11.10）落地真实数据；之前节点 2 阶段（Story 4.8）写死 nil
            "currentRoomId": currentRoomIDDTO,
        },
    }
}
```

**关键设计约束**：

- **完全替换** 节点 2 阶段 `"currentRoomId": nil` 单行写死；用 `currentRoomIDDTO` 局部变量做条件赋值
- **`var currentRoomIDDTO any` 默认零值 = nil interface{}**：序列化为 JSON null；这与 4.8 既有 `var petDTO any` 走同模式（gin.H value 是 nil interface{} 时 json.Marshal 输出 null）
- **string 化用 strconv.FormatUint**：与 4.8 既有 user.id / pet.id / chest.id 同模式；保持 wire 端 ID 全部字符串
- **不**改 room 段的 key 名 / 段结构（仍是 `"room": {"currentRoomId": ...}`），只改 value 路径
- **不**改 user / pet / stepAccount / chest 任一段：本 story 范围红线锁定 room 段 1 个字段

**关键反模式**：

- [bad] **不**用 `out.Room.CurrentRoomID != nil ? str : nil` 三元式（Go 没有三元；用 if 分支 + 局部变量更清晰）
- [bad] **不**写 `gin.H{"currentRoomId": *out.Room.CurrentRoomID}` 直接 deref（nil 指针 deref panic）
- [bad] **不**写 `"currentRoomId": ""`（空串）当用户不在任何房间（V1 §5.1 行 374 钦定 `string | null`，节点 4 阶段必须是 `null` 不是 `""`）
- [bad] **不**单独建 `roomDTO` 局部变量再 marshal 到 gin.H["room"]：4.8 既有 room 段是 inline gin.H，本 story 保留 inline 模式

**AC3 — service 层单元测试 ≥3 case（追加到 `server/internal/service/home_service_test.go`）**

按 epics.md §Story 11.10 行 2037-2040 钦定的**最少 3 case**追加：

```go
// AC11.10.1 happy: 用户在房间 → HomeOutput.Room.CurrentRoomID = &roomID
//
// **关键**：节点 4 阶段（11.10 落地后）user.CurrentRoomID 不再被 service 层强制
// 视为 nil；service 直接透传 mysql.User.CurrentRoomID 字段值到 HomeOutput.Room.CurrentRoomID。
func TestHomeService_LoadHome_UserInRoom_RoomCurrentRoomIDIsRoomID(t *testing.T) {
    roomID := uint64(3001)
    svc := buildHomeService(
        func(ctx context.Context, id uint64) (*mysql.User, error) {
            return &mysql.User{
                ID: 1, Nickname: "u", AvatarURL: "",
                CurrentRoomID: &roomID, // 用户在房间 3001
            }, nil
        },
        func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
            return &mysql.Pet{ID: 2, UserID: 1, PetType: 1, IsDefault: 1}, nil
        },
        func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
            return &mysql.StepAccount{UserID: 1}, nil
        },
        func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
            return &mysql.UserChest{ID: 5, UserID: 1, Status: 1, UnlockAt: time.Now().UTC().Add(10 * time.Minute)}, nil
        },
    )
    out, err := svc.LoadHome(context.Background(), 1)
    if err != nil {
        t.Fatalf("LoadHome: %v", err)
    }
    if out.Room.CurrentRoomID == nil {
        t.Fatal("Room.CurrentRoomID = nil, want &3001")
    }
    if *out.Room.CurrentRoomID != 3001 {
        t.Errorf("*Room.CurrentRoomID = %d, want 3001", *out.Room.CurrentRoomID)
    }
}

// AC11.10.2 happy: 用户不在任何房间 → HomeOutput.Room.CurrentRoomID = nil
//
// users.current_room_id IS NULL 在 GORM 解析为 *uint64 nil；service 透传到
// HomeOutput.Room.CurrentRoomID = nil；handler 把 nil 序列化为 JSON null。
func TestHomeService_LoadHome_UserNotInAnyRoom_RoomCurrentRoomIDIsNil(t *testing.T) {
    svc := buildHomeService(
        func(ctx context.Context, id uint64) (*mysql.User, error) {
            return &mysql.User{
                ID: 1, Nickname: "u", AvatarURL: "",
                CurrentRoomID: nil, // 用户不在任何房间
            }, nil
        },
        func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
            return &mysql.Pet{ID: 2, UserID: 1, PetType: 1, IsDefault: 1}, nil
        },
        func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
            return &mysql.StepAccount{UserID: 1}, nil
        },
        func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
            return &mysql.UserChest{ID: 5, UserID: 1, Status: 1, UnlockAt: time.Now().UTC().Add(10 * time.Minute)}, nil
        },
    )
    out, err := svc.LoadHome(context.Background(), 1)
    if err != nil {
        t.Fatalf("LoadHome: %v", err)
    }
    if out.Room.CurrentRoomID != nil {
        t.Errorf("Room.CurrentRoomID = %v, want nil", *out.Room.CurrentRoomID)
    }
}

// AC11.10.3 edge: users.current_room_id 指向已 closed 的房间（理论不该）→ 仍返回该 id
//
// epics.md §Story 11.10 行 2040 钦定：service 层**不做** rooms 表 cross-check；client
// 在拿到 currentRoomId 后调 /rooms/{id} 时由 11.6 ACL 走 6004 / 6005 自行处理。
//
// **rationale**：home 是聚合接口性能敏感，强制 cross-check rooms.status 会引入额外 1 次
// rooms 表查询 + 与房间业务耦合；user.current_room_id 的"幻象"由 11.5 退出房间事务的
// `UPDATE users SET current_room_id = NULL` 步骤兜底，正常情况下不会指向 closed room。
func TestHomeService_LoadHome_CurrentRoomIDPointsToClosedRoom_StillReturnsID(t *testing.T) {
    closedRoomID := uint64(9999) // 假设房间 9999 在 DB 中 status=2 closed（service 不知晓）
    svc := buildHomeService(
        func(ctx context.Context, id uint64) (*mysql.User, error) {
            // service 层只看 user.CurrentRoomID 字段值，**不**查 rooms.status
            return &mysql.User{
                ID: 1, Nickname: "u",
                CurrentRoomID: &closedRoomID,
            }, nil
        },
        func(ctx context.Context, userID uint64) (*mysql.Pet, error) {
            return &mysql.Pet{ID: 2, UserID: 1, PetType: 1, IsDefault: 1}, nil
        },
        func(ctx context.Context, userID uint64) (*mysql.StepAccount, error) {
            return &mysql.StepAccount{UserID: 1}, nil
        },
        func(ctx context.Context, userID uint64) (*mysql.UserChest, error) {
            return &mysql.UserChest{ID: 5, UserID: 1, Status: 1, UnlockAt: time.Now().UTC().Add(10 * time.Minute)}, nil
        },
    )
    out, err := svc.LoadHome(context.Background(), 1)
    if err != nil {
        t.Fatalf("LoadHome: %v, want nil err (即便 currentRoomID 指向 closed 房间也不报错)", err)
    }
    if out.Room.CurrentRoomID == nil {
        t.Fatal("Room.CurrentRoomID = nil, want &9999 (即便指向已 closed 房间也透传)")
    }
    if *out.Room.CurrentRoomID != 9999 {
        t.Errorf("*Room.CurrentRoomID = %d, want 9999", *out.Room.CurrentRoomID)
    }
}
```

**关键设计约束**：

- **追加而非替换**：4.8 已落地 8 个 service 单测（`TestHomeService_LoadHome_*`），本 story 在同一份 `home_service_test.go` 文件**追加** ≥3 个新 case，不修改既有 case
- **复用 buildHomeService helper**：4.8 既有 `buildHomeService(userFn, petFn, stepFn, chestFn)` 不变；本 story 在 userFn 里返回带 CurrentRoomID 字段的 mysql.User
- **case 命名前缀** `TestHomeService_LoadHome_`（与 4.8 同模式）+ 业务语义后缀（如 `_UserInRoom_RoomCurrentRoomIDIsRoomID` / `_UserNotInAnyRoom_RoomCurrentRoomIDIsNil` / `_CurrentRoomIDPointsToClosedRoom_StillReturnsID`）
- **三 case 必须断言 HomeOutput.Room.CurrentRoomID 字段**（指针 nil / 非 nil + 解引用值）；**不**断言其他字段（4.8 已覆盖，重复无收益）

**关键反模式**：

- [bad] **不**新建独立 `home_service_room_test.go` 文件（与 4.8 既有 8 case 同包同文件聚合）
- [bad] **不**把 closed room edge case 写成 "service 应该报错 / 6005" —— epics.md §Story 11.10 行 2040 钦定 service **不做** cross-check
- [bad] **不**写 fault injection（mock user repo 抛 error）—— 那是 4.8 已覆盖的 `_UserRepoFails_Returns1009`，本 story 范围红线**不**重叠

**AC4 — handler 层单元测试 ≥2 case（追加到 `server/internal/app/http/handler/home_handler_test.go`）**

按本 story AC2 的 wire 字段类型转换钦定追加：

```go
// AC11.10.4 wire: 用户在房间 → response.data.room.currentRoomId = "3001"（string，**不**是 number）
//
// 验证 handler 把 *uint64 → strconv.FormatUint 字符串化路径正确。
func TestHomeHandler_UserInRoom_CurrentRoomIDIsString(t *testing.T) {
    roomID := uint64(3001)
    uid := uint64(1)
    svc := &stubHomeService{
        loadHomeFn: func(ctx context.Context, userID uint64) (*service.HomeOutput, error) {
            return &service.HomeOutput{
                User: service.UserBrief{ID: 1, Nickname: "u"},
                Pet:  &service.PetBrief{ID: 2, PetType: 1, Name: "p", CurrentState: 1},
                StepAccount: service.StepAccountBrief{},
                Chest: service.ChestBrief{
                    ID: 5, Status: 1, UnlockAt: time.Now().UTC().Add(10 * time.Minute),
                    OpenCostSteps: 1000, RemainingSeconds: 600,
                },
                Room: service.RoomBrief{CurrentRoomID: &roomID},
            }, nil
        },
    }
    r := newHomeHandlerRouter(svc, &uid)
    req := httptest.NewRequest(http.MethodGet, "/api/v1/home", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    if w.Code != http.StatusOK {
        t.Fatalf("status = %d, want 200; body=%s", w.Code, w.Body.String())
    }
    env := decodeHomeEnvelope(t, w.Body.Bytes())
    data, ok := env.Data.(map[string]any)
    if !ok {
        t.Fatalf("envelope.data not object: %T", env.Data)
    }
    room, ok := data["room"].(map[string]any)
    if !ok {
        t.Fatalf("data.room not object: %T", data["room"])
    }
    if room["currentRoomId"] != "3001" {
        t.Errorf("room.currentRoomId = %v (%T), want \"3001\" (string)", room["currentRoomId"], room["currentRoomId"])
    }
    // 字面量验证：必须含 "currentRoomId":"3001" 而非 "currentRoomId":3001（number）
    if !bytes.Contains(w.Body.Bytes(), []byte(`"currentRoomId":"3001"`)) {
        t.Errorf(`body 未含 "currentRoomId":"3001" 字面量；body=%s`, w.Body.String())
    }
}

// AC11.10.5 wire: 用户不在任何房间 → response.data.room.currentRoomId = null（**不**是 ""）
//
// 验证 handler 把 nil *uint64 → JSON null 路径正确（与节点 2 阶段 4.8 同字面量行为，
// 但 service 层语义不同：节点 2 是 service 强制 nil，节点 4 是 service 透传 user.CurrentRoomID nil）。
func TestHomeHandler_UserNotInAnyRoom_CurrentRoomIDIsNull(t *testing.T) {
    uid := uint64(1)
    svc := &stubHomeService{
        loadHomeFn: func(ctx context.Context, userID uint64) (*service.HomeOutput, error) {
            return &service.HomeOutput{
                User: service.UserBrief{ID: 1, Nickname: "u"},
                Pet:  &service.PetBrief{ID: 2, PetType: 1, Name: "p", CurrentState: 1},
                StepAccount: service.StepAccountBrief{},
                Chest: service.ChestBrief{
                    ID: 5, Status: 1, UnlockAt: time.Now().UTC().Add(10 * time.Minute),
                    OpenCostSteps: 1000, RemainingSeconds: 600,
                },
                Room: service.RoomBrief{CurrentRoomID: nil},
            }, nil
        },
    }
    r := newHomeHandlerRouter(svc, &uid)
    req := httptest.NewRequest(http.MethodGet, "/api/v1/home", nil)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)

    env := decodeHomeEnvelope(t, w.Body.Bytes())
    data := env.Data.(map[string]any)
    room := data["room"].(map[string]any)
    if room["currentRoomId"] != nil {
        t.Errorf("room.currentRoomId = %v, want nil (null)", room["currentRoomId"])
    }
    // 字面量验证：必须含 "currentRoomId":null
    if !bytes.Contains(w.Body.Bytes(), []byte(`"currentRoomId":null`)) {
        t.Errorf(`body 未含 "currentRoomId":null 字面量；body=%s`, w.Body.String())
    }
}
```

**关键设计约束**：

- **追加而非替换**：4.8 已落地 ~5 个 handler 单测（`TestHomeHandler_*`），本 story 追加 ≥2 个新 case 并**修改** 4.8 既有 `TestHomeHandler_HappyPath_FirstLogin_ReturnsCompleteSchema` 的"`room.currentRoomId` 必须是 null（节点 2 阶段强制）"断言 —— 由于该 case 入参 stub 返回 `service.HomeOutput{... Room: service.RoomBrief{}}`（CurrentRoomID 默认 nil），断言文案从"节点 2 阶段强制"改为"用户不在任何房间"，行为不变，注释更新即可
- **断言 wire 字段类型**（string vs nil）+ **字面量验证**（`bytes.Contains` 检查 `"currentRoomId":"3001"` / `"currentRoomId":null`）—— 与 4.8 既有 happy path case 验证 `"equips":[]` / `"currentRoomId":null` 字面量同模式
- **case 命名前缀** `TestHomeHandler_`（与 4.8 同模式）+ 业务语义后缀

**关键反模式**：

- [bad] **不**改 4.8 既有的 `TestHomeHandler_HappyPath_FirstLogin_ReturnsCompleteSchema` 入参 stub（保持 4.8 既有行为不变 —— 该 case stub 默认 RoomBrief{} 即 nil currentRoomID，实际wire 输出仍是 null，与 4.8 行 211-217 断言对齐；只更新注释里"节点 2 阶段强制"的措辞为"用户不在任何房间"避免语义漂移）
- [bad] **不**断言 wire 是 number `3001` —— V1 §2.5 钦定 BIGINT 全字符串化，AR21 ID 字符串约定
- [bad] **不**断言 currentRoomId 是空串 `""`（语义错；V1 §5.1 钦定 `string | null`，nil 路径走 null）

**AC5 — 集成测试 ≥1 case（追加到 `server/internal/service/home_service_integration_test.go`）**

按 epics.md §Story 11.10 行 2041 钦定追加 dockertest 集成测试：

```go
// AC11.10.6 集成 happy: 创建 user → 直接 INSERT room + room_member + UPDATE users.current_room_id
//                       → svc.LoadHome → HomeOutput.Room.CurrentRoomID = &roomID
//
// 流程：
//   1. INSERT users（current_room_id=NULL）+ pets + step_account + user_chest（4.8 已有 helper 复用）
//   2. INSERT rooms（id=3001, creator_user_id=1, status=1）
//   3. INSERT room_members（room_id=3001, user_id=1）
//   4. UPDATE users SET current_room_id=3001 WHERE id=1
//   5. svc.LoadHome(ctx, 1) → 断言 out.Room.CurrentRoomID == &3001
//
// **不**走 11.3 / 11.4 的 service.CreateRoom / JoinRoom 路径：本 story 严格只测
// home_service.LoadHome 读 users.current_room_id 字段链路；用直接 INSERT 解耦房间业务
// （与 4.8 集成测试 "**手工 INSERT** 测试数据，**不**调 4.6 auth_service.GuestLogin" 同模式）。
//
// **新增 helper**：
//   - insertRoom(t, sqlDB, id, creatorUserID, status, maxMembers)
//   - insertRoomMember(t, sqlDB, roomID, userID)
//   - updateUserCurrentRoomID(t, sqlDB, userID, roomID)
// 三 helper 与 4.8 既有 insertUser / insertPet / insertStepAccount / insertChest 同模式
// （直接 sql.Exec，不走 GORM）。
func TestHomeService_LoadHome_UserInRoom_CurrentRoomIDFromDB(t *testing.T) {
    svc, sqlDB, cleanup := buildHomeServiceIntegration(t)
    defer cleanup()

    insertUser(t, sqlDB, 1, "uid-room-1", "用户1", "")
    insertPet(t, sqlDB, 2001, 1, 1, "默认小猫", 1, 1)
    insertStepAccount(t, sqlDB, 1, 0, 0, 0)
    insertChest(t, sqlDB, 5001, 1, 1, time.Now().UTC().Add(10*time.Minute), 1000)
    insertRoom(t, sqlDB, 3001, 1, 1, 4) // status=1 active, max_members=4
    insertRoomMember(t, sqlDB, 3001, 1)
    updateUserCurrentRoomID(t, sqlDB, 1, 3001)

    out, err := svc.LoadHome(context.Background(), 1)
    if err != nil {
        t.Fatalf("LoadHome: %v", err)
    }
    if out.Room.CurrentRoomID == nil {
        t.Fatal("Room.CurrentRoomID = nil, want &3001")
    }
    if *out.Room.CurrentRoomID != 3001 {
        t.Errorf("*Room.CurrentRoomID = %d, want 3001", *out.Room.CurrentRoomID)
    }
}

// helper（追加到 home_service_integration_test.go）

func insertRoom(t *testing.T, sqlDB *sql.DB, id, creatorUserID uint64, status, maxMembers int) {
    t.Helper()
    _, err := sqlDB.Exec(
        `INSERT INTO rooms (id, creator_user_id, status, max_members) VALUES (?, ?, ?, ?)`,
        id, creatorUserID, status, maxMembers,
    )
    if err != nil {
        t.Fatalf("insert room: %v", err)
    }
}

func insertRoomMember(t *testing.T, sqlDB *sql.DB, roomID, userID uint64) {
    t.Helper()
    _, err := sqlDB.Exec(
        `INSERT INTO room_members (room_id, user_id) VALUES (?, ?)`,
        roomID, userID,
    )
    if err != nil {
        t.Fatalf("insert room_member: %v", err)
    }
}

func updateUserCurrentRoomID(t *testing.T, sqlDB *sql.DB, userID, roomID uint64) {
    t.Helper()
    _, err := sqlDB.Exec(
        `UPDATE users SET current_room_id=? WHERE id=?`,
        roomID, userID,
    )
    if err != nil {
        t.Fatalf("update users.current_room_id: %v", err)
    }
}
```

**关键设计约束**：

- **build tag** `//go:build integration` + `// +build integration`（与 4.8 既有 home_service_integration_test.go 同 tag，文件已挂；新 case 落同一份文件无需重复声明）
- **追加而非替换**：4.8 已落地 3 个集成 case（`TestHomeService_LoadHome_HappyPath` / `_ChestUnlocked_StatusIs2` / `_NoPet_PetIsNil`），本 story 追加 ≥1 个新 case
- **直接 sql.Exec INSERT**：与 4.8 集成测试 helper 风格 1:1 对齐（避免 GORM 回填字段意外覆盖测试数据）
- **新增 3 个 helper**（`insertRoom` / `insertRoomMember` / `updateUserCurrentRoomID`）：在同一份文件**追加**，与 4.8 既有 4 个 helper（`insertUser` / `insertPet` / `insertStepAccount` / `insertChest`）同模式
- **不**调 11.3 service.CreateRoom / 11.4 JoinRoom：解耦 home 测试与房间业务（home_service 集成测试只验 home 链路）

**关键反模式**：

- [bad] **不**新建 `home_service_room_integration_test.go` 文件（与 4.8 既有 3 case 同包同文件聚合）
- [bad] **不**调 11.3 / 11.4 的 service 方法（引入房间业务依赖；home 测试范围红线）
- [bad] **不**测"用户离开房间后 currentRoomId 变 nil"端到端（那是 11.5 集成测试的事；本 story 验证 home 读字段链路即可）
- [bad] **不**测 closed 房间 edge case 在集成测试层（service 单测 AC11.10.3 已覆盖；集成测试不重复）

## Tasks / Subtasks

- [x] Task 1: 修改 `server/internal/service/home_service.go`（AC1）
  - [x] 1.1 HomeOutput struct 加 `Room RoomBrief` 字段（值类型，非指针）
  - [x] 1.2 新增 `RoomBrief` struct，含 `CurrentRoomID *uint64`（指针类型，对齐 mysql.User.CurrentRoomID）
  - [x] 1.3 修改 `LoadHome` 函数返回值：在拼装 HomeOutput 时增加 `Room: RoomBrief{CurrentRoomID: user.CurrentRoomID}`
  - [x] 1.4 添加注释说明 V1 §5.1 行 374 钦定 + Story 11.10 节点 4 阶段语义反转（节点 2 强制 nil → 节点 4 透传 user.CurrentRoomID）

- [x] Task 2: 修改 `server/internal/app/http/handler/home_handler.go`（AC2）
  - [x] 2.1 修改 `homeResponseDTO` 函数：加 `var currentRoomIDDTO any` 局部变量，根据 `out.Room.CurrentRoomID` 是否 nil 走 `nil interface{}` 或 `strconv.FormatUint(...)` 字符串化两路分支
  - [x] 2.2 把 room 段从硬编码 `"currentRoomId": nil` 改为 `"currentRoomId": currentRoomIDDTO`
  - [x] 2.3 更新注释：room 段的 "节点 2 阶段强制 null" 注释改为 "节点 4 阶段（Story 11.10）落地真实数据"

- [x] Task 3: 追加 service 单元测试 ≥3 case（AC3）
  - [x] 3.1 `TestHomeService_LoadHome_UserInRoom_RoomCurrentRoomIDIsRoomID`：user.CurrentRoomID = &3001 → out.Room.CurrentRoomID = &3001
  - [x] 3.2 `TestHomeService_LoadHome_UserNotInAnyRoom_RoomCurrentRoomIDIsNil`：user.CurrentRoomID = nil → out.Room.CurrentRoomID = nil
  - [x] 3.3 `TestHomeService_LoadHome_CurrentRoomIDPointsToClosedRoom_StillReturnsID`：user.CurrentRoomID = &9999（理论指向 closed 房间）→ service 不做 cross-check，仍返该 id

- [x] Task 4: 追加 handler 单元测试 ≥2 case（AC4）
  - [x] 4.1 `TestHomeHandler_UserInRoom_CurrentRoomIDIsString`：stub 返 RoomBrief{CurrentRoomID: &3001} → wire response.data.room.currentRoomId = "3001"（string）+ 字面量 `"currentRoomId":"3001"`
  - [x] 4.2 `TestHomeHandler_UserNotInAnyRoom_CurrentRoomIDIsNull`：stub 返 RoomBrief{CurrentRoomID: nil} → wire response.data.room.currentRoomId = nil + 字面量 `"currentRoomId":null`
  - [x] 4.3 更新 4.8 既有 `TestHomeHandler_HappyPath_FirstLogin_ReturnsCompleteSchema` 注释里"节点 2 阶段强制"措辞为"用户不在任何房间"（行为不变）

- [x] Task 5: 追加集成测试 ≥1 case（AC5）
  - [x] 5.1 在 `home_service_integration_test.go` 追加 helper：`insertRoom` / `insertRoomMember` / `updateUserCurrentRoomID`
  - [x] 5.2 追加 `TestHomeService_LoadHome_UserInRoom_CurrentRoomIDFromDB`：dockertest INSERT user + pet + step + chest + room + room_member + UPDATE users.current_room_id → svc.LoadHome → 断言 out.Room.CurrentRoomID == &3001

- [x] Task 6: 编译 + 测试验证
  - [x] 6.1 `bash scripts/build.sh --test` 在 server 目录跑通（vet + build + 单测）
  - [x] 6.2 `bash scripts/build.sh --integration` 跑集成测试（dockertest，需 Docker daemon）
  - [x] 6.3 grep 全 codebase 确认 4.8 既有 5 个 home_handler 单测 + 8 个 home_service 单测 + 3 个 home_service 集成测试**全绿**（不破坏 done 状态）

## Dev Notes

### 关键设计决策（**必读**）

**1. RoomBrief 是值类型 vs 指针类型**

- HomeOutput.Room 是 `RoomBrief`（值类型，非 *RoomBrief）
- 理由：V1 §5.1 行 374 钦定 `data.room` 是**容器**对象（永远存在），仅 `data.room.currentRoomId` 字段才可空
- handler 永远输出 `"room": {"currentRoomId": ...}` 而非 `"room": null`；与 `data.pet`（容器可空）形成对比

**2. RoomBrief.CurrentRoomID 是 \*uint64 vs uint64 + sentinel**

- 用 `*uint64`（指针类型）而非 `uint64` + 0 sentinel
- 理由 1：与 `mysql.User.CurrentRoomID *uint64` 字段类型 1:1 对齐，service 层零额外转换（直接 `Room: RoomBrief{CurrentRoomID: user.CurrentRoomID}`）
- 理由 2：BIGINT UNSIGNED 0 是合法值（虽然 AUTO_INCREMENT 通常从 1 起），不能用作 sentinel
- 理由 3：与 V1 §5.1 行 374 `string | null` 语义直接映射（nil → null / 非 nil → string）

**3. 零额外 repo 调用**

- users.current_room_id 是 users 表字段，已经在 4.8 阶段的 `userRepo.FindByID` 一次性返回（mysql.User struct 行 38-40 钦定）
- 本 story **不**新建 UserRepo.FindCurrentRoomIDByUserID 单字段查询方法
- 与 V1 §10.2 `GET /rooms/current` 走 RoomRepo.GetCurrentRoomIDByUserID（11.6 落地）的接口分工不同：home 是聚合接口，所有字段都从已查的 5 个 repo 输出里拼装；§10.2 是单字段轻量查询走独立 repo 方法

**4. 不做 closed-room cross-check**

- epics.md §Story 11.10 行 2040 edge case 钦定："users.current_room_id 指向已 closed 的房间（理论不该）→ 仍返回该 id（client 调 /rooms/{id} 时会得到 6005 自行处理）"
- service 层**不**查 rooms 表 cross-check 该 roomID 是否真实存在 + status==1
- rationale：home 性能敏感（首页加载关键路径），强制 cross-check 会引入额外 1 次 rooms 表查询 + 与房间业务耦合
- "幻象"由 11.5 退出房间事务的 `UPDATE users SET current_room_id = NULL` 步骤兜底；正常情况下 users.current_room_id 不会指向 closed 房间

**5. wire 端字符串化用 strconv.FormatUint**

- BIGINT id 用 `strconv.FormatUint(id, 10)`（**不**用 `fmt.Sprintf("%d", id)`）
- 理由：与 4.8 既有 user.id / pet.id / chest.id 同模式（更快 + 不依赖 fmt reflect）
- AR21 ID 字符串约定 + V1 §2.5（避免 JS Number 精度丢失）

### Source Tree 影响（最终）

```
server/
└─ internal/
   ├─ service/
   │  ├─ home_service.go                    [MODIFIED] HomeOutput 加 Room 段 + RoomBrief struct + LoadHome 透传 user.CurrentRoomID
   │  ├─ home_service_test.go               [MODIFIED] 追加 ≥3 case
   │  └─ home_service_integration_test.go   [MODIFIED] 追加 ≥1 case + 3 helper
   └─ app/http/handler/
      ├─ home_handler.go                    [MODIFIED] homeResponseDTO room 段从写死 nil 改为条件序列化
      └─ home_handler_test.go               [MODIFIED] 追加 ≥2 case + 更新 1 个既有 case 注释
```

**不**新建：service / handler / repo / migration / DTO 模块。

### Testing Standards

- 单元测试用 stub repo（service 层）/ stub HomeService（handler 层）+ table-driven 风格的 case 命名前缀（与 4.8 同模式）
- 集成测试 build tag `//go:build integration` + `// +build integration` + dockertest 真实 mysql:8.0 容器
- handler 单测断言 wire 字段类型（`map[string]any` 解构 + `room["currentRoomId"]` 类型断言）+ **字面量验证**（`bytes.Contains(w.Body.Bytes(), ...)`）
- service 单测断言 `out.Room.CurrentRoomID` 指针 nil / 非 nil + 解引用值

### Project Structure Notes

- 严格遵循 4.8 已建立的 `internal/service/home_service.go` + `internal/app/http/handler/home_handler.go` 分层（设计文档 §6.2 钦定）
- HomeOutput / RoomBrief 是 service 层 DTO（**不是** wire DTO，handler 转换为 V1 §5.1 钦定 wire 格式）
- 不引入 RoomRepo（11.6 RoomRepo.GetCurrentRoomIDByUserID 是 §10.2 GET /rooms/current 的依赖；home 不消费）

### References

- [Source: docs/宠物互动App_V1接口设计.md §5.1 行 374] `data.room.currentRoomId` 类型 `string | null`，节点 4 由 Story 11.10 注入真实数据
- [Source: docs/宠物互动App_V1接口设计.md §5.1 行 408 + 行 456] 节点 2 阶段示例 `"currentRoomId": null` + 节点 4 之后真实数据示例 `"currentRoomId": "3001"`
- [Source: docs/宠物互动App_V1接口设计.md §5.1 行 475] Future Fields 引用块 "data.room.currentRoomId：节点 4 起由 Story 11.10 注入真实房间 ID"
- [Source: docs/宠物互动App_V1接口设计.md §10 行 1611] `> §10 房间章节末尾引用：GET /home.data.room.currentRoomId 由 Story 11.10 真实实装（节点 4，Epic 11 收官）`
- [Source: docs/宠物互动App_V1接口设计.md §10.2 行 1217] `GET /home.data.room.currentRoomId` 与 `GET /rooms/current.data.roomId` 字段语义等价，分工：home 聚合 / current 轻量
- [Source: docs/宠物互动App_数据库设计.md §5.1] users 表 `current_room_id BIGINT UNSIGNED NULL`（migration 0001 已落地）
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md §6.2 行 364-389] User / Home 模块钦定单独 home_service.go 承载首页拼装
- [Source: \_bmad-output/planning-artifacts/epics.md §Story 11.10 行 2022-2041] AC 钦定（**唯一权威 AC 来源**）
- [Source: \_bmad-output/implementation-artifacts/4-8-get-home-聚合接口.md] Story 4.8 GetHome 节点 2 阶段实装（home_service / home_handler 框架；节点 2 阶段强制 currentRoomId=null 红线由本 story 解除）
- [Source: \_bmad-output/implementation-artifacts/11-3-创建房间事务.md] Story 11.3 写入 users.current_room_id 路径（事务最后一步 UpdateCurrentRoomID）
- [Source: \_bmad-output/implementation-artifacts/11-4-加入房间事务.md] Story 11.4 写入 users.current_room_id 路径
- [Source: \_bmad-output/implementation-artifacts/11-5-退出房间事务.md] Story 11.5 写入 users.current_room_id = NULL 路径
- [Source: server/internal/repo/mysql/user_repo.go 行 32-41] mysql.User struct 钦定 `CurrentRoomID *uint64` 字段类型
- [Source: server/internal/service/home_service.go] 4.8 既有 home_service 实装（本 story 扩展对象）
- [Source: server/internal/app/http/handler/home_handler.go] 4.8 既有 home_handler 实装（本 story 扩展对象）

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]

### Debug Log References

- `bash scripts/build.sh --test` → vet + build + 全 24 包单测全绿（含 `internal/service` 11 个 home_service case + `internal/app/http/handler` 7 个 home_handler case）
- `cd server && go test -tags=integration -count=1 -run "TestHomeService" ./internal/service/...` → 4 个 home_service 集成测试全绿（含本 story 新增 1 个 case + 4.8 既有 3 个 case）

### Completion Notes List

- **AC1 落地**：HomeOutput 加 `Room RoomBrief` 字段（值类型，对齐 V1 §5.1 行 374 "data.room 容器永远存在"语义）；新增 `RoomBrief` struct 仅含 `CurrentRoomID *uint64`（与 mysql.User.CurrentRoomID 1:1 对齐）；LoadHome 在 4 个 repo 串行 + chest 动态判定的现有路径上**追加** `Room: RoomBrief{CurrentRoomID: user.CurrentRoomID}` 透传 —— **零额外 repo 调用**（current_room_id 已在 (1) FindByID 一次性返回）。
- **AC2 落地**：`homeResponseDTO` 加 `var currentRoomIDDTO any` 局部变量条件赋值（nil → JSON null / 非 nil → strconv.FormatUint 字符串化），与既有 `petDTO` 走同模式；room 段从硬编码 `"currentRoomId": nil` 改为读 `currentRoomIDDTO`。
- **AC3 落地**：`home_service_test.go` **追加** 3 个 case（`_UserInRoom_RoomCurrentRoomIDIsRoomID` / `_UserNotInAnyRoom_RoomCurrentRoomIDIsNil` / `_CurrentRoomIDPointsToClosedRoom_StillReturnsID`），验证 service 层透传 + 不做 closed-room cross-check。复用 4.8 既有 `buildHomeService` helper，不破坏既有 8 个 case。
- **AC4 落地**：`home_handler_test.go` **追加** 2 个 case（`_UserInRoom_CurrentRoomIDIsString` / `_UserNotInAnyRoom_CurrentRoomIDIsNull`），断言 wire 字段类型（string vs nil）+ 字面量验证（`"currentRoomId":"3001"` / `"currentRoomId":null`）。同步**更新** `_HappyPath_FirstLogin_ReturnsCompleteSchema` 注释里"节点 2 阶段强制"措辞为"用户不在任何房间"（行为不变；其 stub 默认 RoomBrief{}.CurrentRoomID=nil）。
- **AC5 落地**：`home_service_integration_test.go` **追加** 3 个 helper（`insertRoom` / `insertRoomMember` / `updateUserCurrentRoomID`）+ 1 个 case（`_UserInRoom_CurrentRoomIDFromDB`），通过 dockertest 真实 mysql:8.0 容器验证 users.current_room_id 端到端读取链路。**不**调 11.3 / 11.4 service 路径，与 4.8 既有 helper 直接 sql.Exec 同模式。
- **范围红线确认**：未新建 service / handler / repo / migration / DTO 模块；未触碰 GET /me / §10.x 房间任一接口实装；未改 V1 / 数据库设计 / ADR / migrations / sprint-status 之外任一配置；未破坏 4.8 既有 GetHome 实装；未修改 11.3-11.9 既有 room 实装。
- **构建验证**：`bash scripts/build.sh --test` 通过（go vet + go build + 24 包单测全绿）；集成测试 `go test -tags=integration -run "TestHomeService" ./internal/service/...` 通过（4 个 home_service 集成 case 全绿）。

### File List

- `server/internal/service/home_service.go` [MODIFIED] — HomeOutput 加 Room 字段 + 新增 RoomBrief struct + LoadHome 透传 user.CurrentRoomID
- `server/internal/app/http/handler/home_handler.go` [MODIFIED] — homeResponseDTO room 段从写死 nil 改为条件序列化（nil → null / 非 nil → strconv.FormatUint 字符串化）
- `server/internal/service/home_service_test.go` [MODIFIED] — 追加 3 个 case 覆盖 user 在房间 / 不在房间 / 指向 closed 房间
- `server/internal/app/http/handler/home_handler_test.go` [MODIFIED] — 追加 2 个 case + 更新 1 个既有 case 注释
- `server/internal/service/home_service_integration_test.go` [MODIFIED] — 追加 3 个 helper + 1 个 dockertest case
- `_bmad-output/implementation-artifacts/sprint-status.yaml` [MODIFIED] — 11-10 状态 ready-for-dev → in-progress → review
- `_bmad-output/implementation-artifacts/11-10-get-home-扩展-room-currentroomid-真实数据.md` [MODIFIED] — 状态 + Tasks/Subtasks 勾选 + Dev Agent Record 填充 + Change Log

## Change Log

| Date       | Author                  | Change Description                                                                                                                                                                                                                                  |
| ---------- | ----------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| 2026-05-09 | claude-opus-4-7[1m] dev | 实装 Story 11.10 GET /home 扩展 room.currentRoomId 真实数据：HomeOutput 加 Room 段（RoomBrief{CurrentRoomID *uint64}）+ LoadHome 透传 user.CurrentRoomID（零额外 repo 调用）+ homeResponseDTO 条件序列化（nil → null / 非 nil → 字符串化）+ ≥3 单测 + ≥1 集成测试 |
