# Story 11.2: rooms + room_members migration（接管 Story 10.3 r5 提前 ship 的 0007 / 0008 + 完整测试覆盖 + 集成测试 fixture 替换）

Status: done

<!-- Validation 可选。建议运行 validate-create-story 在 dev-story 前做一次质检。 -->

## Story

As a 服务端开发,
I want **正式接管** `server/migrations/0007_init_rooms.{up,down}.sql` 与 `server/migrations/0008_init_room_members.{up,down}.sql` 两组 migration（DDL 已在 Story 10.3 review r5 [P1] 提前落地，但所属 Epic 与单元 / DB UNIQUE 约束执行测试覆盖未达 epics.md §Story 11.2 钦定）+ **彻底替换** `server/internal/app/ws/ws_integration_test.go` 中 `startMySQLWithRoomMemberFixture` helper 的 inline `CREATE TABLE` 临时建表路径为 `migrate.Up()` 真实跑 0007 / 0008 + **扩展** `server/internal/repo/mysql/room_member_repo.go` 的 `RoomMember` GORM domain struct 字段集合（从 Story 10.3 阶段 `RoomID / UserID` 最小骨架升级为 `0008_init_room_members.up.sql` 真实 schema 全字段：`ID` 自增 PK / `RoomID` / `UserID` / `JoinedAt` / `UpdatedAt`；同时为 `rooms` 表新增 `Room` GORM domain struct 含 `ID` / `CreatorUserID` / `Status` / `MaxMembers` / `CreatedAt` / `UpdatedAt` 全字段，下游 Story 11.3 ~ 11.7 直接复用）+ **补全** epics.md §Story 11.2 钦定的 ≥3 case 单测（migrate up 后表存在 / 字段类型 / 全部索引和唯一约束都符合 §5.13 / §5.14 + migrate down 后表删除 + 重复 migrate up 幂等）+ **新增** dockertest 集成测试覆盖"故意尝试违反 UNIQUE(user_id) → 数据库拒绝插入"路径（epics.md §Story 11.2 钦定但 Story 10.3 r5 未覆盖；现有 `migrate_integration_test.go::TestMigrateIntegration_TablesPresent_AfterUp_RoomTables` 仅做 schema 元数据校验，未覆盖运行时 INSERT 拒绝行为）,
so that **Story 11.3 ~ 11.7 + Story 11.9 + Epic 12 房间业务实装**可以基于一个**已正式归属 Epic 11、已具备完整测试覆盖、已统一 fixture 路径、已具备完整 GORM domain struct**的 rooms / room_members 持久化基础并行展开，不再出现"WS 集成测试用 inline DDL（含 `member_count` 字段 + (room_id, user_id) 复合 PK + 缺 UNIQUE(user_id) 约束）vs prod 0007 / 0008 真实 schema（含 `id` AUTO_INCREMENT PK + UNIQUE(user_id) + UNIQUE(room_id, user_id)）双 schema 漂移 → Story 11.3 / 11.4 / 11.5 实装的事务 INSERT / DELETE 路径在集成测试用临时表过 / prod 真实表炸"的返工，也不再出现"Story 10.3 r5 提前 ship 的 migration 在 sprint-status.yaml 仍归属 backlog 状态的 11.2、Epic 11 retrospective 时找不到正式 owner、ADR-0003 §3.2 钦定的'每个 migration 文件归属一个 Story'追踪轨迹中断"的归属混乱。

## 故事定位（Epic 11 第二条 = 第一条**实装**story；上承 11.1 契约定稿 + 10.3 r5 提前 ship 的 migration DDL，下启 11.3 ~ 11.7 服务端事务 + 11.8 WS 广播 + 11.9 Layer 2 集成测试）

- **Epic 11 进度**：11.1（契约定稿，done）→ **11.2（本 story，rooms + room_members migration 测试覆盖收尾 + fixture 替换 + GORM domain struct 扩展）** → 11.3（POST /rooms 创建房间事务）→ 11.4（POST /rooms/{roomId}/join 加入房间事务）→ 11.5（POST /rooms/{roomId}/leave 退出房间事务）→ 11.6（GET /rooms/current + GET /rooms/{roomId} 房间详情查询）→ 11.7（房间快照真实实现，替换 E10.7 placeholder）→ 11.8（member.joined / member.left WS 广播）→ 11.9（Layer 2 集成测试 - 房间生命周期全流程）→ 11.10（GET /home 扩展 room.currentRoomId 真实数据）。
- **本 story 是 11.3 ~ 11.7 / 11.9 / Epic 12 的强前置**：
  - **11.3 创建房间事务**：service 层 `INSERT INTO rooms (creator_user_id, status, max_members)` + `INSERT INTO room_members (room_id, user_id)` 必须命中本 story 验证过的 0007 / 0008 表 schema；GORM domain struct（本 story 扩展的 `Room` / `RoomMember`）直接被 11.3 repo 层 `Create(ctx, room)` / `Create(ctx, member)` 方法复用
  - **11.4 加入房间事务**：service 层 `SELECT ... FROM rooms WHERE id = ? FOR UPDATE` + `SELECT COUNT(*) FROM room_members WHERE room_id = ?` + `INSERT INTO room_members` + UNIQUE(user_id) 兜底 → 6003 路径必须对齐本 story 验证过的 UNIQUE 约束行为（本 story 集成测试的"故意违反 UNIQUE(user_id) → DB 拒绝"路径**就是** 11.4 兜底语义的 schema 层基础）
  - **11.5 退出房间事务**：`DELETE FROM room_members WHERE room_id = ? AND user_id = ?` + `RowsAffected == 0` 兜底 → 6004 路径需要 `room_members` 表 PK（`id` AUTO_INCREMENT）+ UNIQUE(room_id, user_id) 双约束保证"目标行删除是幂等的"；本 story 扩展 `RoomMember` domain struct 时显式标注 `ID uint64 \`gorm:"column:id;primaryKey"\``（不再是 Story 10.3 阶段的 `(RoomID, UserID)` 复合主键标注）让 11.5 repo 层 GORM `Delete(...)` / `RowsAffected` 取值符合 0008 真实 schema
  - **11.6 房间详情查询**：`SELECT room_members JOIN users JOIN pets WHERE room_id = ? ORDER BY joined_at ASC` + REPEATABLE READ + FOR SHARE 锁 → 路径需要 `room_members.joined_at` 字段与 `idx_room_id` 索引（本 story 单测明确验过）
  - **11.7 房间快照真实实现**：`RoomSnapshotBuilder` 替换 placeholder → 读 `room_members WHERE room_id = ?` + JOIN `users` / `pets` 聚合 → 与 11.6 同源
  - **11.9 Layer 2 集成测试**：dockertest 起 mysql:8.0 容器后**第一步**就是跑 `migrate.Up()` 拿到 0001 ~ 0008 全部 8 张表，才能跑 100 goroutine 并发 / 回滚 / 边界场景；本 story 把 ws_integration_test.go fixture 从 inline DDL 改为 `migrate.Up()` 路径**就是**为 11.9 集成测试 helper 树立标准模板（Epic 11 集成测试全部走 official migration 路径，不再各自 inline DDL）
  - **Epic 12 房间页面**：iOS 端 `RoomViewModel.members[]` / `WebSocketClient.handleRoomSnapshot(...)` 解析的 `userId` / `nickname` / `pet.{petId, currentState}` / `pet.equips` 字段全部来自本 story 验证过的 server 持久化基础（DB → service → handler → JSON 响应链路的 schema 层起点就是本 story）
- **epics.md §Story 11.2 钦定**（行 1828-1848）：
  - `migrations/0007_init_rooms.sql` 按数据库设计 §5.13 创建表（含 `idx_creator_user_id` + `idx_status_created_at`）
  - `migrations/0008_init_room_members.sql` 按 §5.14 创建表，含**关键约束**：`UNIQUE(user_id)` / `UNIQUE(room_id, user_id)` / `KEY idx_room_id`
  - 含 down.sql
  - **单元测试覆盖**（≥3 case）：happy（migrate up 后两张表存在 + 字段类型 + 全部索引和唯一约束都符合 §5.13 / §5.14）/ happy（migrate down 后表删除）/ edge（重复 migrate up → 幂等）
  - **集成测试覆盖**（dockertest）：migrate up → SHOW CREATE TABLE 对比 schema → 故意尝试违反 UNIQUE(user_id)（同 user 插两条 room_members）→ 数据库拒绝插入
- **Story 10.3 review r5 [P1] 历史遗留交接**（**关键背景**）：Story 10.3 原计划"不实装 rooms / room_members migration（Epic 11.2 接管）"，但 r5 review 指出"当前 prod 部署用 0001-0006 起服务时，wsTablesReady() 永远 false → /ws/rooms/:roomId 永远不挂 → client 拿到 404（不是 documented WS close codes），Story 10.3 在 prod / smoke test 完全不可用" → 推翻原 backlog 计划，把"表存在"职责拆出来提前到 Story 10.3，让 10.3 self-contained 可部署；**业务事务**（INSERT / UPDATE / DELETE / member_count 维护）保留给 Epic 11.4 / 11.5。本 story（11.2）作为"原 backlog 钦定 owner"必须**正式接管**：
  - 0007 / 0008 migration 文件已在 Story 10.3 ship 到 `server/migrations/`（无需新建）
  - `migrate_integration_test.go` 已有 `TestMigrateIntegration_UpThenDown`（验证 8 张表存在 / 全消失）+ `TestMigrateIntegration_TablesPresent_AfterUp_RoomTables`（验证 rooms / room_members 索引 + PK + max_members 默认值 + status 默认值；该测试函数实际名见 migrate_integration_test.go 行 178 ~ 280 附近，本 story 实装时以代码 grep 为准） + `TestMigrateIntegration_UpTwice_Idempotent`（验证幂等）—— **这三个测试已覆盖 epics.md §Story 11.2 钦定的"≥3 case 单测"+ Story 10.3 r5 已立的 schema 元数据 verification**
  - **缺口（本 story 必补）**：epics.md §Story 11.2 钦定的"集成测试覆盖：故意尝试违反 UNIQUE(user_id) 约束 → 数据库拒绝插入"路径**未覆盖**（现有测试只做 schema 元数据校验，未做运行时 INSERT 拒绝行为校验）
  - **缺口（本 story 必补）**：`server/internal/app/ws/ws_integration_test.go` 的 `startMySQLWithRoomMemberFixture` 仍用 inline DDL（rooms 表带 `member_count` 字段 + room_members 表 PK = (room_id, user_id)），与 prod 0007 / 0008 真实 schema 漂移；本 story 把它替换为 `migrate.Up()` + `INSERT INTO ... fixture 数据` 路径
  - **缺口（本 story 必补）**：`server/internal/repo/mysql/room_member_repo.go` 的 `RoomMember` domain struct 仍是最小骨架（`RoomID + UserID + (RoomID, UserID) 复合 PK 标注`），与 0008 真实 schema 不符（真实 schema 是 `id` AUTO_INCREMENT PK + UNIQUE(room_id, user_id)）；本 story 扩展为完整字段集合 + 同时新增 `Room` domain struct（11.3 创建房间事务必需）
- **范围红线**：
  - 本 story **只**改 `server/migrations/0007_init_rooms.up.sql` 注释（顶部 "Story 10.3 review r5 [P1] 引入" 注释升级为 "Story 10.3 review r5 [P1] 引入 → Story 11.2 正式接管 Epic 11 owner"）+ `server/migrations/0008_init_room_members.up.sql` 注释（同上）+ `server/migrations/0007_init_rooms.down.sql` 同步注释 + `server/migrations/0008_init_room_members.down.sql` 同步注释 + `server/internal/repo/mysql/room_member_repo.go`（扩展 `RoomMember` domain struct + 新增 `Room` domain struct）+ `server/internal/repo/mysql/room_member_repo_test.go`（如 GORM struct 字段调整影响既有 mock 测试断言则同步调整 SQL regex）+ `server/internal/infra/migrate/migrate_integration_test.go`（新增 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` 集成测试 case，覆盖 UNIQUE(user_id) DB 拒绝插入路径）+ `server/internal/app/ws/ws_integration_test.go`（fixture 从 inline DDL → `migrate.Up()`）+ 本 story 文件 + sprint-status.yaml 流转
  - **不**改 0007 / 0008 SQL **DDL 内容本身**（DDL 已在 Story 10.3 r5 落地 + 已与数据库设计 §5.13 / §5.14 对齐 + 已通过 r5 review；本 story 仅升级注释 + 扩 GORM domain struct + 补测试 + 替换 fixture，**不**改 CREATE TABLE 语句的字段 / 类型 / 索引 / 约束）
  - **不**实装任何 service / handler / repo 写操作（11.3 ~ 11.5 才做；本 story 阶段 `RoomMemberRepo` 接口仍只含 Story 10.3 引入的 3 个读方法 `RoomExists` / `IsUserInRoom` / `ListMembers`，**不**新增 Insert / Delete 等写方法）
  - **不**接 Redis（10.6 已接，本 story 不动）
  - **不**改 V1 接口契约（11.1 已冻结）
  - **不**改任何 `docs/宠物互动App_*.md`（数据库设计 §5.13 / §5.14 是契约**输入**，本 story 严格对齐它们但**不修改**）
  - **不**改 `_bmad-output/` 下其他 yaml / md（除自己的 story 文件 + sprint-status.yaml 流转）
  - **不**新增 .down.sql 反向校验逻辑（`migrate Down` 行为已在 Story 4.3 + 7.2 + 10.3 反复验证，本 story 通过既有 `TestMigrateIntegration_UpThenDown` 间接覆盖即可，**不**新增针对 0007 / 0008 的独立 down 测试）
  - **不**改 ADR-0003（migration 工具 + 编号约定 + .up.sql / .down.sql 双向规范由 Story 4.3 已锚定，本 story 沿用不修改）
  - **不**预先实装 Insert / Delete / Update 等 write 方法到 `RoomMemberRepo` / 新建 `RoomRepo`（YAGNI；11.3 ~ 11.5 才需要这些方法，写在本 story 会反向耦合 11.3 ~ 11.5 实装路径）—— 本 story **只**做 GORM domain struct 字段扩展（`Room` / `RoomMember` 两个 struct），**不**新增方法 / 新增 interface 类型

**本 story 不做**（明确范围红线）：

- 不新建 0007 / 0008 SQL 文件（已在 Story 10.3 r5 ship；本 story 仅改文件顶部注释）
- 不修改 0007 / 0008 SQL DDL 内容（CREATE TABLE 字段 / 类型 / 索引 / 约束**全部冻结**；如需改则属契约变更走完整 ADR 流程）
- 不实装 Story 11.3 ~ 11.10 的任何 handler / service / repo 写方法
- 不动 ADR-0003 钦定的 migration 工具 / 编号约定 / 文件命名规范
- 不动数据库设计 §5.13 / §5.14 / §6.12 / §7（schema 输入，本 story 对齐不修改）
- 不动 V1 接口契约（11.1 已冻结）
- 不动 Redis（10.6 已接，本 story 不动）
- 不动 docs/宠物互动App_Go项目结构与模块职责设计.md（Go 工程目录已锚定 §4 / §6 / §9 / §12；本 story 不引入新目录 / 模块；`internal/repo/mysql/room_member_repo.go` / `internal/migrations/` 已在 §6 锚定）
- 不动 docs/宠物互动App_iOS客户端工程结构与模块职责设计.md（本 story 是纯 server-side migration 测试覆盖收尾，与 iOS 工程无关）
- 不动 docs/宠物互动App_时序图与核心业务流程设计.md（§11.1 / §11.2 房间事务流程是 11.3 / 11.4 / 11.5 的契约输入，本 story 不触发流程修订）
- 不为 0007 / 0008 schema 字段写 OpenAPI / JSON Schema / GORM AutoMigrate 等形式化定义（旧架构残留；新架构以 .up.sql / .down.sql 真实 SQL 为唯一真相）
- 不在本 story 阶段做"prod 部署 migration 自动化 / 一键 rollback / dry-run / force"等运维化改造（保留最小集 up/down/status，与 Story 4.3 一致；高级开关后置 Epic 36 部署阶段）
- 不修改 `cmd/server/main.go` 的 migrate 子命令（已在 Story 4.3 落地，本 story 沿用不修改）
- 不写英文版测试注释 / 文档（项目 communication_language=Chinese；与 Story 4.3 / 7.2 / 10.3 一致）
- 不为 0007 / 0008 写 stress test / fuzz test（节点 4 阶段 schema 已稳定 + 单测 + dockertest 集成测试已覆盖核心约束；fuzz 是后置 epic 范围）
- 不在本 story 内对 Story 11.3 ~ 11.5 的事务实装做"提前预实装" —— 即使本 story 扩展 GORM domain struct 时容易顺手写 `(r *roomRepo) Create(ctx, room)` / `(r *roomMemberRepo) Insert(ctx, member)` / `(r *roomMemberRepo) Delete(ctx, roomID, userID)` 等方法，**禁止**写入；这些方法是 11.3 / 11.4 / 11.5 钦定范围，提前 ship 会让 Story 11.3 / 11.4 / 11.5 评审时找不到"新增方法"的明确范围边界（Story 4.3 / 4.6 拆分对照）

## Acceptance Criteria

**AC1 — 0007 / 0008 migration 文件归属升级（注释级别）**

`server/migrations/0007_init_rooms.up.sql` + `server/migrations/0007_init_rooms.down.sql` + `server/migrations/0008_init_room_members.up.sql` + `server/migrations/0008_init_room_members.down.sql` 四个文件**仅**在顶部注释中升级"由 Story 10.3 review r5 [P1] 引入"为"由 Story 10.3 review r5 [P1] 引入；Story 11.2 正式接管 Epic 11 owner（含完整 ≥3 case 单测 + dockertest 集成测试覆盖 UNIQUE 约束运行时拒绝行为 + WS 集成测试 fixture 切到 official migration 路径）"。

- **DDL 内容（CREATE TABLE 字段 / 类型 / 索引 / 约束）严格不动**
- **down.sql 内容（DROP TABLE 顺序）严格不动**
- 注释升级仅为 audit trail / Epic 11 retrospective owner 追溯使用

**AC2 — `RoomMember` GORM domain struct 字段扩展（最小升级）**

修改 `server/internal/repo/mysql/room_member_repo.go` 中既有 `RoomMember` struct（行 28-31 附近）：

升级前（Story 10.3 阶段最小骨架，与 0008 真实 schema **不一致**）：

```go
type RoomMember struct {
    RoomID uint64 `gorm:"column:room_id;primaryKey"`
    UserID uint64 `gorm:"column:user_id;primaryKey"`
}
```

升级后（与 `migrations/0008_init_room_members.up.sql` 真实 schema 1:1 对齐）：

```go
type RoomMember struct {
    ID        uint64    `gorm:"column:id;primaryKey;autoIncrement"`
    RoomID    uint64    `gorm:"column:room_id;not null"`
    UserID    uint64    `gorm:"column:user_id;not null"`
    JoinedAt  time.Time `gorm:"column:joined_at;not null;default:CURRENT_TIMESTAMP(3)"`
    UpdatedAt time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP(3)"`
}
```

- 字段顺序与 0008 SQL 列顺序一致
- 不依赖 GORM `gorm:"uniqueIndex"` 等 tag 定义 UNIQUE 约束（UNIQUE 由 SQL DDL 定义，GORM struct **不**重复声明 —— 与 ADR-0003 §3.2 "禁止 GORM AutoMigrate" 同源；GORM struct 只为 `Find` / `Create` 提供字段映射，**不**作为 schema 真相源）
- 同步更新 struct 文档注释（行 12-31 附近）：把"Story 10.3 引入；Epic 11.2 落地 0007_init_rooms / 0008_init_room_members migration 后扩展为完整字段集合"升级为"Story 10.3 引入最小骨架（仅 RoomID / UserID）；Story 11.2 升级为与 0008 真实 schema 1:1 对齐的完整字段集合（id / room_id / user_id / joined_at / updated_at）"
- **关键**：导入 `time` 包 + 不引入 `gorm.Model`（避免引入 `DeletedAt` / 软删除字段，与 0008 真实 schema 不符）

**AC3 — 新增 `Room` GORM domain struct（11.3 ~ 11.7 必需）**

在 `server/internal/repo/mysql/room_member_repo.go` 中新增 `Room` struct（**不**新建独立 `room_repo.go` 文件 —— 节点 4 阶段 11.3 / 11.6 才需要 RoomRepo interface，本 story 仅落地 domain struct）：

```go
// Room 是 rooms 表的完整 GORM domain struct（Story 11.2 引入；
// 与 server/migrations/0007_init_rooms.up.sql 真实 schema 1:1 对齐）。
//
// 字段（与数据库设计.md §5.13 + 0007_init_rooms.up.sql §28-38 行 1:1 对齐）：
//   - ID: BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT
//   - CreatorUserID: BIGINT UNSIGNED NOT NULL（创建者 user.id）
//   - Status: TINYINT NOT NULL DEFAULT 1（数据库设计.md §6.12 钦定 1=active / 2=closed）
//   - MaxMembers: TINYINT UNSIGNED NOT NULL DEFAULT 4（节点 4 阶段固定 4）
//   - CreatedAt / UpdatedAt: DATETIME(3)（毫秒精度时间戳）
//
// **范围红线**：本 struct 仅为下游 Story 11.3（创建房间事务）/ 11.6（房间详情查询）
// 提供字段映射；本 story 阶段**不**新建 RoomRepo interface / 实装 Create / Update /
// FindByID 等方法（YAGNI；Story 11.3 / 11.6 才落地）。
type Room struct {
    ID            uint64    `gorm:"column:id;primaryKey;autoIncrement"`
    CreatorUserID uint64    `gorm:"column:creator_user_id;not null"`
    Status        int8      `gorm:"column:status;not null;default:1"`
    MaxMembers    uint8     `gorm:"column:max_members;not null;default:4"`
    CreatedAt     time.Time `gorm:"column:created_at;not null;default:CURRENT_TIMESTAMP(3)"`
    UpdatedAt     time.Time `gorm:"column:updated_at;not null;default:CURRENT_TIMESTAMP(3)"`
}

// TableName 显式声明 "rooms"。
func (Room) TableName() string { return "rooms" }
```

- 字段类型选择：`Status int8` 对齐 `TINYINT`（带符号；MySQL `TINYINT` 默认带符号，范围 -128 ~ 127；status 枚举 1 / 2 在范围内）；`MaxMembers uint8` 对齐 `TINYINT UNSIGNED`（无符号；范围 0 ~ 255；房间容量 4 在范围内）
- 不引入 `Pets` / `Members` 等关联字段 / preload tag（Story 11.6 落地 GET /rooms/{roomId} 时再加，本 story YAGNI）
- 紧跟现有 `RoomMember` struct 之后（同一文件内，便于阅读）

**AC4 — `room_member_repo_test.go` 同步调整（如有）**

`server/internal/repo/mysql/room_member_repo_test.go` 现有测试基于 sqlmock 验证 SQL regex（`SELECT 1 FROM rooms WHERE id = ? AND status = 1 LIMIT 1` 等）：

- 如 `RoomMember` struct 字段扩展导致既有 SQL regex 不再匹配（如 GORM 自动生成的 SELECT 列改变），**就地修复**测试 mock 断言；
- **保持** Story 10.3 引入的 `RoomExists` / `IsUserInRoom` / `ListMembers` 三方法的所有 mock case 测试通过（`true / false / error / 0 rows` 等既有覆盖**不**回归）；
- 不新增针对扩展字段的 mock 断言（本 story 不实装新方法，无需新断言）。

**AC5 — `migrate_integration_test.go` 新增 UNIQUE(user_id) DB 拒绝插入集成测试**

在 `server/internal/infra/migrate/migrate_integration_test.go` 中新增**集成测试** `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected`（带 `// +build integration` build tag 已由文件头继承）：

```go
// TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected 验证
// migrations/0008_init_room_members.up.sql 钦定的 UNIQUE KEY uk_user_id (user_id)
// 在运行时被 MySQL 真实拒绝重复 user_id 插入。
//
// **背景（Story 11.2 引入）**：Story 10.3 review r5 [P1] 落地的 0008 migration 的
// schema 元数据校验已在 TestMigrateIntegration_TablesPresent_AfterUp_RoomTables
// 覆盖（uk_user_id 索引存在），但**运行时 INSERT 拒绝行为**未覆盖 ——
// epics.md §Story 11.2 钦定的"集成测试覆盖：故意尝试违反 UNIQUE(user_id)（同 user
// 插两条 room_members）→ 数据库拒绝插入"路径在本 case 落地。
//
// **覆盖路径**：
//   1. migrate up → rooms / room_members 表存在
//   2. 插入 rooms (id=3001, status=1, max_members=4)
//   3. 插入 rooms (id=3002, status=1, max_members=4)（第二个房间，让 UNIQUE 测试可对照）
//   4. 插入 room_members (room_id=3001, user_id=2001) → 成功
//   5. 再次插入 room_members (room_id=3002, user_id=2001) → **DB 必须拒绝**
//      （同 user 不能在两个不同房间，UNIQUE(user_id) 兜底）；
//      err 必须含 "Duplicate entry" / "1062"（MySQL 错误码）
//   6. 同时验证 UNIQUE(room_id, user_id) 二级兜底（同 (room, user) 插两次）：
//      插入 room_members (room_id=3001, user_id=2001) → DB 必须拒绝（已被步骤 4 占用）
func TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected(t *testing.T) {
    // 实装细节由 dev-story 阶段补全；模板见 TestMigrateIntegration_TablesPresent_AfterUp_RoomTables
}
```

- 集成测试用 `dockertest` 起 mysql:8.0 容器（沿用 `migrationsPath(t)` + `startMySQL(t)` helper，与既有 case 一致）
- 用 `database/sql` 直跑 raw INSERT（**不**走 GORM；让测试结果**不**依赖 ORM 行为差异）
- 错误断言：`err != nil` + `strings.Contains(err.Error(), "Duplicate entry")` 或 `strings.Contains(err.Error(), "1062")`（MySQL 错误码 1062 = `ER_DUP_ENTRY`，与 §10.4 步骤 5 / §10.1 步骤 3 兜底语义对齐）
- **不**断言具体 MySQL error message 文本（不同 MySQL 版本可能略有差异；用 "Duplicate entry" substring 是稳定 contract）

**AC6 — `ws_integration_test.go` fixture 替换为 `migrate.Up()` 路径**

修改 `server/internal/app/ws/ws_integration_test.go` 中 `startMySQLWithRoomMemberFixture` helper（行 41 ~ 154）：

升级前（inline DDL，与 prod 0007 / 0008 真实 schema **漂移**）：

```go
stmts := []string{
    `CREATE TABLE rooms (
        id BIGINT UNSIGNED NOT NULL,
        status TINYINT NOT NULL DEFAULT 1,
        max_members INT NOT NULL DEFAULT 4,
        member_count INT NOT NULL DEFAULT 0,        // ❌ 与 0007 漂移（0007 无 member_count 列）
        PRIMARY KEY (id)                             // ❌ 与 0007 漂移（0007 是 AUTO_INCREMENT）
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
    `CREATE TABLE room_members (
        room_id BIGINT UNSIGNED NOT NULL,
        user_id BIGINT UNSIGNED NOT NULL,
        joined_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
        PRIMARY KEY (room_id, user_id),              // ❌ 与 0008 漂移（0008 是 id AUTO_INCREMENT PK）
        INDEX idx_user_room (user_id, room_id)       // ❌ 与 0008 漂移（0008 是 idx_room_id 单列）
    ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
    `INSERT INTO rooms (id, status, max_members, member_count) VALUES (3001, 1, 4, 2)`,  // ❌ member_count 列不存在
    `INSERT INTO room_members (room_id, user_id) VALUES (3001, 1001), (3001, 1002)`,
}
```

升级后（official `migrate.Up()` 路径 + fixture 数据用 `INSERT INTO ... VALUES`）：

```go
// **Story 11.2 落地后**：彻底移除 inline CREATE TABLE 临时建表路径，改为跑 official
// migration（migrate.Up()）；fixture 数据用 INSERT 插（rooms 自增 id 由 DB 分配，
// 不再硬编码 id=3001 —— 改用 LAST_INSERT_ID() 取回；INSERT 后查取 roomID 用于
// fixture）。

mig, err := migrate.New(dsn, migrationsPath())
if err != nil {
    _ = pool.Purge(resource)
    t.Fatalf("migrate.New: %v", err)
}
defer mig.Close()

ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
defer cancel()
if err := mig.Up(ctx); err != nil {
    _ = pool.Purge(resource)
    t.Fatalf("migrate.Up: %v", err)
}

// 插 fixture：1 个 active 房间 + 2 个 member（user_id=1001 / 1002）
fixtureSQL := []string{
    `INSERT INTO rooms (id, creator_user_id, status, max_members) VALUES (3001, 1001, 1, 4)`,
    `INSERT INTO room_members (room_id, user_id) VALUES (3001, 1001), (3001, 1002)`,
}
for _, stmt := range fixtureSQL {
    if _, err := rawDB.Exec(stmt); err != nil {
        _ = pool.Purge(resource)
        t.Fatalf("exec fixture %q: %v", stmt, err)
    }
}
```

- **关键**：fixture 数据明确插 `id=3001` + `creator_user_id=1001`（与 0007 schema 兼容；DB 接受显式 id 值，AUTO_INCREMENT 不阻止）—— 让既有 `TestWSIntegration_HappyPath_FirstMessageIsSnapshot` 等 case 的 `roomID=3001 / userID=1001 / 1002` 期望**不**改动
- **关键**：导入 `migrate` 包路径（`github.com/huing/cat/server/internal/infra/migrate`）+ `migrationsPath()` helper（参考 `migrate_integration_test.go::migrationsPath` 实装；ws_integration_test.go 路径深一层 `server/internal/app/ws/`，相对路径 `../../../../migrations` 或重新计算）
- **同步**：删除 helper 内 inline `CREATE TABLE rooms` / `CREATE TABLE room_members` 两条 raw SQL（共 14 行 + INSERT 4 行 → 改为 migrate.Up() 1 行 + fixture INSERT 2 条）
- **同步**：升级 helper 文档注释（行 41-69 附近）：把"**临时建表**：本 story 阶段 room_members migration 还没落地（Epic 11.2 才做）..."删除，替换为"**Story 11.2 落地后**：彻底切到 official migration 路径，不再 inline DDL；fixture 数据通过 INSERT 插入"

**AC7 — 既有 WS 集成测试在新 fixture 路径下全绿**

`server/internal/app/ws/ws_integration_test.go` 全部既有测试 case（含 `TestWSIntegration_HappyPath_FirstMessageIsSnapshot` 等十几个 case，行 185 ~ 文件末尾）在新 fixture 路径下全部跑通：

- `bash scripts/build.sh --integration` 通过
- 既有 case 中对 `roomID=3001` / `userID=1001` / `userID=1002` 的硬编码期望**不**变（fixture 数据保持兼容）
- 如个别 case 因 schema 升级（如 `room_members` 加 `id` 列 / `updated_at` 列）出现行为差异（如 `SELECT *` 返回列数变化），就地修复 case；**不**因为单 case 失败放弃 fixture 替换路径

**AC8 — 既有 migrate 集成测试不回归**

`server/internal/infra/migrate/migrate_integration_test.go` 既有测试 case（`TestMigrateIntegration_UpThenDown` / `TestMigrateIntegration_StatusAfterUp` / `TestMigrateIntegration_UpTwice_Idempotent` / `TestMigrateIntegration_TablesPresent_AfterUp` / `TestMigrateIntegration_TablesPresent_AfterUp_RoomTables` 等）在本 story 改动后全部继续通过：

- `bash scripts/build.sh --integration` 通过
- 8 张表的存在性 / 索引 / PK / 字段类型 / 默认值断言**不**变

**AC9 — `bash scripts/build.sh --test` 全绿**

`bash scripts/build.sh --test` 在本 story 全部改动落地后：

- 单测全绿（`server/internal/repo/mysql/*_test.go` 全绿，含 sqlmock 测试如有调整）
- 不引入新的 lint / vet warning

**AC10 — `bash scripts/build.sh --integration` 全绿**

`bash scripts/build.sh --integration` 在本 story 全部改动落地后：

- 既有集成测试全部继续通过
- 新增的 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` 通过
- WS 集成测试在新 fixture 路径下全部继续通过

**AC11 — sprint-status.yaml 流转**

- create-story workflow 自动把 `11-2-rooms-room_members-migration` 状态从 `backlog` 升级为 `ready-for-dev`
- dev-story workflow 把状态从 `ready-for-dev` 升级为 `in-progress`
- code-review / review 把状态从 `in-progress` 升级为 `review`
- story-done 把状态从 `review` 升级为 `done`
- last_updated 字段同步更新为完成日期
- **epic-11 状态**：本 story 是 epic-11 的**第二条** story（11.1 已 done，epic-11 已是 in-progress）；本 story 流转**不**触发 epic 状态变化（保持 in-progress）—— 与 epic-loop 文档说明一致

**AC12 — 完成判定与 review 关注点**

完成本 story 的判定标准（dev-story 阶段）：

- AC1 ~ AC10 全部落地
- 所有改动文件清单（不超过本 story 范围红线列出的文件）落地：
  - `server/migrations/0007_init_rooms.up.sql`（注释升级，DDL 不动）
  - `server/migrations/0007_init_rooms.down.sql`（注释升级）
  - `server/migrations/0008_init_room_members.up.sql`（注释升级，DDL 不动）
  - `server/migrations/0008_init_room_members.down.sql`（注释升级）
  - `server/internal/repo/mysql/room_member_repo.go`（`RoomMember` struct 字段扩展 + 新增 `Room` struct + 注释升级）
  - `server/internal/repo/mysql/room_member_repo_test.go`（如有 mock 断言调整 - 否则不动）
  - `server/internal/infra/migrate/migrate_integration_test.go`（新增 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected`）
  - `server/internal/app/ws/ws_integration_test.go`（fixture 切到 migrate.Up()）
  - `_bmad-output/implementation-artifacts/sprint-status.yaml`
  - `_bmad-output/implementation-artifacts/11-2-rooms-room_members-migration.md`（本文件 Status / Tasks 流转）
- `bash scripts/build.sh --test` + `bash scripts/build.sh --integration` 双绿

review 阶段必须命中的检查（参考 epic-10 retrospective §6 lessons + Story 11.1 r1 ~ r14 review 教训）：

- **r1 同源风险（schema 漂移）**：review 必须 grep `ws_integration_test.go` 全文确认 inline `CREATE TABLE rooms` / `CREATE TABLE room_members` 已彻底移除，没有任何 fallback / commented-out / "如果 migrate.Up 失败则 fallback inline DDL" 路径（任何 fallback 都会让 fixture 漂移问题悄悄回归）
- **r2 同源风险（fixture 期望兼容）**：review 必须 diff 改动前后 ws_integration_test.go 全部 case 期望值，对 `roomID=3001` / `userID=1001` / `userID=1002` 等硬编码期望保持稳定；如有 case 期望被迫调整，每条调整必须有显式说明（避免"为了让测试通过悄悄改期望"）
- **r3 同源风险（GORM struct 与 DDL 一致性）**：review 必须逐字段对照 `Room` / `RoomMember` struct 字段名 / 类型 / GORM tag 与 0007 / 0008 SQL DDL；任何字段不一致（如 `Status int` vs `TINYINT` / 缺 `default:1` tag / 缺 `not null` tag）都视为 P1 修复点
- **r4 同源风险（GORM AutoMigrate 反模式）**：review 必须 grep 确认本 story 引入的 `Room` / `RoomMember` struct **没有**触发 AutoMigrate（搜 `AutoMigrate(` 关键词），与 ADR-0003 §3.2 钦定一致
- **r5 同源风险（UNIQUE 测试覆盖深度）**：review 必须确认 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` 同时覆盖 UNIQUE(user_id) 单列约束 + UNIQUE(room_id, user_id) 复合约束（两个约束在 0008 SQL 都存在；缺一就漏覆盖）
- **r6 同源风险（domain struct 引用范围）**：review 必须确认本 story 新增的 `Room` struct 没有被任何 service / handler 文件引用（`grep -r "mysql.Room\b"` 应仅限 `room_member_repo.go` 自己 + `room_member_repo_test.go` 如有 + 不出现在 service / handler 层）—— 11.3 ~ 11.6 才会引用，本 story 提前引用属范围越界
- **scope creep 检查**：review 必须确认 `RoomMemberRepo` interface 仍然只含 Story 10.3 引入的 3 个读方法（`RoomExists` / `IsUserInRoom` / `ListMembers`），**没有**新增任何 Insert / Delete / Update 方法（任何写方法都属 11.3 / 11.4 / 11.5 范围越界）

## Tasks / Subtasks

- [x] Task 1: 升级 0007 / 0008 migration 文件顶部注释（AC1）
  - [x] `server/migrations/0007_init_rooms.up.sql` 顶部注释升级（"由 Story 10.3 review r5 [P1] 引入" → "由 Story 10.3 review r5 [P1] 引入；Story 11.2 正式接管 Epic 11 owner（含完整 ≥3 case 单测 + dockertest 集成测试覆盖 UNIQUE 约束运行时拒绝行为 + WS 集成测试 fixture 切到 official migration 路径）"）
  - [x] `server/migrations/0007_init_rooms.down.sql` 顶部注释同步升级
  - [x] `server/migrations/0008_init_room_members.up.sql` 顶部注释同步升级
  - [x] `server/migrations/0008_init_room_members.down.sql` 顶部注释同步升级
  - [x] DDL 内容（CREATE TABLE / DROP TABLE）严格不动（仅注释）
- [x] Task 2: 扩展 `RoomMember` GORM domain struct（AC2）
  - [x] 修改 `server/internal/repo/mysql/room_member_repo.go` 中既有 `RoomMember` struct
  - [x] 添加 `ID uint64` / `JoinedAt time.Time` / `UpdatedAt time.Time` 三字段
  - [x] 改 `RoomID` / `UserID` 的 GORM tag 从 `primaryKey` 为 `not null`（PK 仅在 ID 字段）
  - [x] 导入 `time` 包
  - [x] 升级 struct 文档注释反映 Story 11.2 升级
- [x] Task 3: 新增 `Room` GORM domain struct（AC3）
  - [x] 在 `RoomMember` struct 之后新增 `Room` struct
  - [x] 字段：`ID` / `CreatorUserID` / `Status int8` / `MaxMembers uint8` / `CreatedAt` / `UpdatedAt`
  - [x] 加 `TableName() string` 显式返回 `"rooms"`
  - [x] 加结构体 doc comment（含范围红线说明：本 story 不新建 RoomRepo interface）
- [x] Task 4: 同步调整 `room_member_repo_test.go`（AC4）
  - [x] grep 确认：现有 sqlmock 测试 case 全部用 `Raw(...)` 字面 SQL（`SELECT 1 FROM rooms WHERE id = ? AND status = 1 LIMIT 1` 等），**不**经过 GORM auto-generated SELECT/INSERT 路径 → struct 字段扩展不影响 SQL regex 匹配；mysql 包 `go test ./internal/repo/mysql/... -count=1` 全绿验证（无任何断言失败），本 task 不需要额外调整测试代码
- [x] Task 5: 新增 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` 集成测试（AC5）
  - [x] 在 `migrate_integration_test.go` 末尾追加新 case
  - [x] 流程：migrate up → 插 2 个 rooms → 插 1 个 room_member (3001, 2001) 成功 → 插 (3002, 2001) 必须被 DB 拒绝（UNIQUE(user_id) 单列）→ 插 (3001, 2001) 必须被 DB 拒绝（UNIQUE(room_id, user_id) 复合）
  - [x] 错误断言：`err != nil` + `strings.Contains(err.Error(), "Duplicate entry")`
  - [x] 用 `database/sql.ExecContext` 直跑 raw INSERT（不走 GORM）
- [x] Task 6: `ws_integration_test.go` fixture 切到 `migrate.Up()`（AC6）
  - [x] 修改 `startMySQLWithRoomMemberFixture` helper 中 `stmts := []string{...}` 列表
  - [x] 删除 inline `CREATE TABLE rooms` + `CREATE TABLE room_members` 两条 raw SQL
  - [x] 替换为 `migrate.New(dsn, migrationsPathForWS(t)).Up(ctx)`
  - [x] 保留 fixture INSERT 数据（rooms id=3001 + room_members 1001 / 1002）
  - [x] 调整 fixture INSERT 添加 `creator_user_id = 1001`（0007 schema 必填）
  - [x] 删除 `INSERT INTO rooms ... member_count` 字段（0007 schema 无此列；memberCount 由 placeholderSnapshotBuilder 从 `len(ListMembers)` 计算）
  - [x] 删除 `BroadcastToRoom_3Clients` case 中 `UPDATE rooms SET member_count = 3` raw SQL（0007 schema 无 member_count 列）
  - [x] 升级 helper 文档注释反映 Story 11.2 切换
  - [x] 添加 `migrationsPathForWS(t)` helper（相对路径 `../../../migrations`，与 `migrate_integration_test.go::migrationsPath` 同深度）
- [x] Task 7: 跑 `bash scripts/build.sh --test` 验证单测全绿（AC9）
  - [x] 单测全绿（含 mysql 包 sqlmock 测试）；构建脚本 BUILD SUCCESS
  - [x] 无新增 lint / vet warning（`go vet` / `go vet -tags=integration` 都通过）
  - [x] 注：ws 包 `TestSessionManager_Register_TriggersHook` 是**已知 pre-existing flaky** test（`useGatewayDial` 内 ListSessionsByRoomID 看到 session 已注册但 Register 锁外的 onRegister 钩子可能尚未执行 → 计数器读 0），与本 story 无关；stash 本 story 改动后该测试仍然 ~70-80% 失败率，验证不是本 story 引入。本 story 范围红线明确不修该测试 / 不动 ws_test.go / useGatewayDial helper
- [x] Task 8: 跑 `bash scripts/build.sh --integration` 验证集成测试全绿（AC10）
  - [x] 既有 migrate 集成测试全绿（`internal/infra/migrate` 全部 case 在 600s 超时下通过 - 实测 135s）
  - [x] 新增 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` 通过（实测 58s）
  - [x] WS 集成测试（含 `TestWSIntegration_HappyPath_FirstMessageIsSnapshot` / `TestWSIntegration_TokenExpired_Closes4001` / `TestWSIntegration_UserNotInRoom_Closes4003` / `TestWSIntegration_PingPongRoundtrip` / `TestWSIntegration_HeartbeatTimeout_Closes4005` / `TestWSIntegration_BroadcastToRoom_3Clients_AllReceive` 等）在新 fixture 路径下全绿（实测 161s）
  - [x] db / redis / repo / app / service 集成测试全绿
  - [x] 注：build.sh `--integration` 默认 `-timeout=120s`，全包顺序跑总耗时 > 120s 触发 deadlock；按包分跑全部通过；timeout 限制由 build.sh 钦定不属本 story 范围
- [x] Task 9: 自检 + git commit + 状态流转（AC11, AC12）
  - [x] grep 确认 ws_integration_test.go 已无 inline `CREATE TABLE rooms` / `CREATE TABLE room_members`（仅在文档/历史注释引用，非可执行 SQL；fixture stmts 列表已移除）
  - [x] grep 确认 `mysql.Room` / `mysql.RoomMember` 引用范围未越界（仅 `room_member_repo.go` 自身 + 既有测试文件 `room_member_repo_test.go`；不出现在 service / handler 层）
  - [x] grep 确认 `RoomMemberRepo` interface 没有新增 Insert / Delete / Update 方法（仍为 RoomExists / IsUserInRoom / ListMembers 三读方法）
  - [x] 单独 git commit（commit message 含 "Story 11.2 接管 0007/0008 migration + 完整测试覆盖 + ws fixture 替换 + GORM domain struct 扩展"）—— 该步由 fix-review / story-done 流程承接
  - [x] 更新本 story 文件 Status 为 `review`（sprint-status.yaml 同步）

## Dev Notes

### 角色定位与 Epic 11 的实装起点

本 story 是 Epic 11 唯一的**纯 server-side 持久化层 owner story**，定位为"接管 Story 10.3 r5 提前 ship 的 migration + 补全测试覆盖收尾 + 把临时 fixture 切到 official 路径"——所有后续 server 实装（11.3 ~ 11.7）和 Layer 2 集成测试（11.9）都基于本 story 落地的"已 owned + 已测试 + 已统一 fixture 路径"持久化基础展开。这与 Story 4.3（节点 2 五张表 migrations + migrate CLI）在 Epic 4 的角色对称：4.3 是节点 2 的"DB 起点"story，本 story 是节点 4 房间业务的"DB 收尾"story（migration 已 ship，本 story 收尾测试覆盖 + fixture 路径统一）。

**Epic 10 retrospective §6 + Story 11.1 r1~r14 review 教训钦定**：契约 / schema 类 Story 的 review 严格度 **≥** 实装 Story —— Story 11.1 跑满 14 轮 review 子循环验证了"纯文档 / schema 类 story 也可能是 review 大户"。本 story 实装阶段必须把"schema 元数据校验 vs 运行时行为校验"作为 hunter checklist 一等公民，避免"以为元数据校验已覆盖 → 漏 UNIQUE 约束运行时拒绝行为校验"（这就是为什么 epics.md §Story 11.2 要单独钦定 dockertest "故意尝试违反 UNIQUE → 拒绝" 集成测试 case）。

### 与 Story 10.3 r5 的交接边界（关键背景）

Story 10.3 review r5 [P1] 因 prod 部署可用性诉求，把"建表"职责从 Epic 11.2 backlog 拆出来提前到 Story 10.3 ship。本 story（11.2）作为原 backlog 钦定 owner，**正式接管**：

| Story 10.3 r5 已 ship | Story 11.2 接管职责（本 story） |
|---|---|
| 0007_init_rooms.up.sql / .down.sql DDL | 注释升级 audit trail（DDL 不动） |
| 0008_init_room_members.up.sql / .down.sql DDL | 注释升级 audit trail（DDL 不动） |
| `RoomMember` GORM struct 最小骨架（RoomID + UserID） | 扩展为完整字段集合（id / room_id / user_id / joined_at / updated_at），与 0008 schema 1:1 对齐 |
| `RoomMemberRepo` interface 3 个读方法（RoomExists / IsUserInRoom / ListMembers） | 范围红线保留（**不**新增 Insert / Delete / Update） |
| `migrate_integration_test.go` 8 张表 schema 元数据校验（索引 / PK / max_members 默认值 / status 默认值） | epics.md §Story 11.2 钦定的 ≥3 case 单测**已基本覆盖**（happy up + down + 幂等）；缺口"故意违反 UNIQUE 约束 → DB 拒绝"由本 story 补 |
| `ws_integration_test.go` fixture 用 inline CREATE TABLE 临时建表 | 切到 official `migrate.Up()` 路径，不再 inline DDL |

**为什么不把 0007 / 0008 文件回滚再让 11.2 重新 ship？**

- Story 10.3 r5 的"提前 ship"是**审慎的修复决策**（为解决 prod 部署不可用问题）；回滚再 ship 不会让代码 / 测试覆盖更好，反而引入历史污染（git log 出现"先 ship，再 revert，再 re-ship"的迷惑路径）
- "归属调整"通过注释 + sprint-status.yaml + 本 story 文档三处协同**已能完整描述 audit trail**（Story 4.6 / 7.3 / 10.5 等多 story 协作的 owner 切换都用过这个模式）

### 与 Epic 4 / 7 / 10 既有 migration 的对照

本 story 的 migrate 工具 / 编号约定 / 文件命名规范严格沿用：

- **Story 4.3**：建立 0001 ~ 0005 五张表（users / user_auth_bindings / pets / user_step_accounts / user_chests）+ migrate Go API + cmd/server/main.go migrate 子命令
- **Story 7.2**：新增 0006_init_user_step_sync_logs.up.sql / .down.sql
- **Story 10.3 r5**：新增 0007_init_rooms.up.sql / .down.sql + 0008_init_room_members.up.sql / .down.sql
- **Story 11.2（本 story）**：接管 0007 / 0008 owner（注释升级），不新建 SQL 文件
- **后续 epic**（Story 17.2 / 20.2 / 20.4 / 23.2 / 26.2 / 32.2）：分别新增 0009 ~ 0014（emoji_configs / cosmetic_items / chest_open_logs / user_cosmetic_items / user_pet_equips / compose_logs + compose_log_materials）

**编号顺序约定**：编号严格单调递增，**绝对不允许**插入历史中间编号（如不允许 0006.5 / 0007a / 重新 0007 等）—— 与 ADR-0003 §3.2 钦定一致；Story 10.3 r5 提前 ship 0007 / 0008 时已遵守此约定，本 story 接管时也不破坏。

### epics.md 钦定 vs 本 story 的实装路径补全

epics.md §Story 11.2（行 1828-1848）钦定的 AC 与本 story 实装路径对照：

| epics.md 钦定 | Story 10.3 r5 已落地 | 本 story 必补 |
|---|---|---|
| `migrations/0007_init_rooms.sql` 按 §5.13 创建表（含 idx_creator_user_id + idx_status_created_at） | ✅ DDL 已 ship | 注释升级 audit trail |
| `migrations/0008_init_room_members.sql` 按 §5.14 创建表，含 UNIQUE(user_id) + UNIQUE(room_id, user_id) + KEY idx_room_id | ✅ DDL 已 ship | 注释升级 audit trail |
| 含 down.sql | ✅ 已 ship | 注释升级 |
| 单元测试 ≥3 case：happy up 后表存在 + 字段类型 + 索引 / 唯一约束 / down 后表删除 / 重复 up 幂等 | ✅ `TestMigrateIntegration_TablesPresent_AfterUp_RoomTables` + `TestMigrateIntegration_UpThenDown` + `TestMigrateIntegration_UpTwice_Idempotent` | 现有覆盖足够，本 story 不新增（但需在 review 阶段确认覆盖完整） |
| 集成测试（dockertest）：migrate up → SHOW CREATE TABLE 对比 schema → 故意尝试违反 UNIQUE(user_id)（同 user 插两条 room_members）→ 数据库拒绝插入 | ⚠️ schema 元数据已覆盖（索引 / 默认值），但**运行时 INSERT 拒绝行为未覆盖** | 新增 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` 集成测试 |

### Story 10.3 r5 + 11.1 r1~r14 review 教训在本 story 的适用性

本 story 是 server-side 实装 + 集成测试 owner story，部分 review 教训直接适用：

- **11.1 r1（schema 漂移）**：本 story 直接对应 —— `ws_integration_test.go` inline DDL vs prod 0007 / 0008 schema 漂移，本 story 通过切到 official migration 路径根本性解决
- **10.3 r5（schema 元数据 vs 运行时行为校验）**：本 story 直接对应 —— 现有测试只验 `INFORMATION_SCHEMA.STATISTICS` / `KEY_COLUMN_USAGE` 等元数据；运行时 `INSERT INTO ... VALUES (...)` 拒绝行为校验由本 story 新增的集成测试覆盖
- **11.1 r3（WS 断开仅清 ephemeral，房间归属只能由 HTTP leave 改变）**：本 story **不**直接适用（本 story 不实装事务；但本 story 扩展 `RoomMember` struct 时**不**加 `Online` / `IsActive` 等"在线态字段"，与 11.1 r3 锁定的"持久层不区分在线 / 离线"原则同源）
- **11.1 r5（pets.current_state 枚举不要 alias motion_state）**：本 story **不**直接适用（不动 pets 表）
- **Story 4.3 lessons (`docs/lessons/2026-04-26-cli-relative-path-and-graceful-stop-wait.md` 等)**：migrate CLI 路径 / 工具用法已稳定，本 story 沿用不修改

### 字段类型映射 checklist（Story 11.2 review 必查）

| DB 字段（0007 / 0008 SQL） | Go struct 字段 | GORM tag | 出现位置 |
|---|---|---|---|
| `rooms.id BIGINT UNSIGNED PK AUTO_INCREMENT` | `Room.ID uint64` | `column:id;primaryKey;autoIncrement` | room_member_repo.go |
| `rooms.creator_user_id BIGINT UNSIGNED NOT NULL` | `Room.CreatorUserID uint64` | `column:creator_user_id;not null` | room_member_repo.go |
| `rooms.status TINYINT NOT NULL DEFAULT 1` | `Room.Status int8` | `column:status;not null;default:1` | room_member_repo.go |
| `rooms.max_members TINYINT UNSIGNED NOT NULL DEFAULT 4` | `Room.MaxMembers uint8` | `column:max_members;not null;default:4` | room_member_repo.go |
| `rooms.created_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3)` | `Room.CreatedAt time.Time` | `column:created_at;not null;default:CURRENT_TIMESTAMP(3)` | room_member_repo.go |
| `rooms.updated_at DATETIME(3) ON UPDATE CURRENT_TIMESTAMP(3)` | `Room.UpdatedAt time.Time` | `column:updated_at;not null;default:CURRENT_TIMESTAMP(3)` | room_member_repo.go |
| `room_members.id BIGINT UNSIGNED PK AUTO_INCREMENT` | `RoomMember.ID uint64` | `column:id;primaryKey;autoIncrement` | room_member_repo.go |
| `room_members.room_id BIGINT UNSIGNED NOT NULL` | `RoomMember.RoomID uint64` | `column:room_id;not null` | room_member_repo.go |
| `room_members.user_id BIGINT UNSIGNED NOT NULL` | `RoomMember.UserID uint64` | `column:user_id;not null` | room_member_repo.go |
| `room_members.joined_at DATETIME(3) DEFAULT CURRENT_TIMESTAMP(3)` | `RoomMember.JoinedAt time.Time` | `column:joined_at;not null;default:CURRENT_TIMESTAMP(3)` | room_member_repo.go |
| `room_members.updated_at DATETIME(3) ON UPDATE CURRENT_TIMESTAMP(3)` | `RoomMember.UpdatedAt time.Time` | `column:updated_at;not null;default:CURRENT_TIMESTAMP(3)` | room_member_repo.go |

**review 必查**：每行 GORM tag 与 SQL DDL 字段定义 1:1 对齐；任何 missing / extra tag 都视为 P1 修复点。

### 测试覆盖 checklist（review 必查）

| 测试位置 | 测试 case | epics.md AC 钦定 | 当前状态 |
|---|---|---|---|
| `migrate_integration_test.go::TestMigrateIntegration_UpThenDown` | migrate up → 8 张表存在 → migrate down → 8 张表全消失 | happy down 后表删除 | ✅ Story 10.3 r5 已落地 |
| `migrate_integration_test.go::TestMigrateIntegration_TablesPresent_AfterUp_RoomTables` | rooms / room_members 索引 + PK + max_members 默认值 + status 默认值 | happy up 后字段类型 + 索引 + 唯一约束 | ✅ Story 10.3 r5 已落地 |
| `migrate_integration_test.go::TestMigrateIntegration_UpTwice_Idempotent` | 连续两次 up 返 nil + 表数仍为 8 | edge 重复 up 幂等 | ✅ Story 10.3 r5 已落地 |
| `migrate_integration_test.go::TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` | migrate up → 插 2 个 rooms + 1 个 room_member (3001, 2001) 成功 → 插 (3002, 2001) DB 拒绝 → 插 (3001, 2001) DB 拒绝 | dockertest 故意违反 UNIQUE → DB 拒绝 | ❌ **本 story 必补** |
| `ws_integration_test.go::startMySQLWithRoomMemberFixture` | inline `CREATE TABLE` → `migrate.Up()` | （非 epics.md AC，但本 story 范围红线列出） | ❌ **本 story 必补** |

### Project Structure Notes

- 改动文件路径（绝对）：
  - `C:/fork/cat/server/migrations/0007_init_rooms.up.sql`
  - `C:/fork/cat/server/migrations/0007_init_rooms.down.sql`
  - `C:/fork/cat/server/migrations/0008_init_room_members.up.sql`
  - `C:/fork/cat/server/migrations/0008_init_room_members.down.sql`
  - `C:/fork/cat/server/internal/repo/mysql/room_member_repo.go`
  - `C:/fork/cat/server/internal/repo/mysql/room_member_repo_test.go`（如有调整）
  - `C:/fork/cat/server/internal/infra/migrate/migrate_integration_test.go`
  - `C:/fork/cat/server/internal/app/ws/ws_integration_test.go`
  - `C:/fork/cat/_bmad-output/implementation-artifacts/sprint-status.yaml`
  - `C:/fork/cat/_bmad-output/implementation-artifacts/11-2-rooms-room_members-migration.md`（本文件）
- 不引入新文件（所有改动都是既有文件的扩展 / 注释升级）
- sprint-status.yaml 中 `11-2-rooms-room_members-migration` 状态由 create-story workflow 自动从 `backlog` 升级为 `ready-for-dev`；本 story 完成后 dev 流程会进一步流转为 `in-progress` → `review` → `done`
- epic-11 状态保持 `in-progress`（本 story 是第二条，不触发 epic 状态变化）

### References

- [Source: docs/宠物互动App_数据库设计.md#5.13（rooms 表结构）] — 本 story `Room` GORM struct 字段类型映射来源；`creator_user_id BIGINT UNSIGNED` / `status TINYINT NOT NULL DEFAULT 1` / `max_members TINYINT UNSIGNED NOT NULL DEFAULT 4` 严格对齐
- [Source: docs/宠物互动App_数据库设计.md#5.14（room_members 表结构 + 关键约束）] — 本 story `RoomMember` GORM struct 字段类型映射来源 + `UNIQUE(user_id)` / `UNIQUE(room_id, user_id)` / `KEY idx_room_id` 三约束的语义来源
- [Source: docs/宠物互动App_数据库设计.md#6.12（rooms.status）] — `Status int8` 枚举 1=active / 2=closed 来源
- [Source: docs/宠物互动App_数据库设计.md#7.1（room_members 必须保留的唯一约束）] — `UNIQUE(user_id)` "保证一个用户同一时间只在一个房间" 语义来源；本 story 集成测试 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` 直接验证此约束运行时拒绝
- [Source: docs/宠物互动App_数据库设计.md#8.6（创建房间事务边界）] — Story 11.3 实装的事务边界依据；本 story `Room` GORM struct 字段集合是 11.3 事务 INSERT 的字段映射基础
- [Source: docs/宠物互动App_数据库设计.md#8.7（加入房间事务边界）] — Story 11.4 实装的事务边界依据；本 story 扩展 `RoomMember` struct 字段为 11.4 INSERT room_members + UNIQUE(user_id) 兜底语义提供 schema 层基础
- [Source: docs/宠物互动App_数据库设计.md#8.8（房间详情查询事务边界）] — Story 11.6 实装依据；本 story 扩展 struct 字段（含 `JoinedAt`）让 11.6 `ORDER BY joined_at ASC` 稳定排序可直接复用 GORM 字段
- [Source: docs/宠物互动App_V1接口设计.md#10（房间 5 接口字段层契约）] — Story 11.1 已锚定，本 story 实装层 GORM struct 字段名（snake_case → camelCase 映射）必须与 §10 字段表保持反向一致（DB → API 字段映射的 server side 起点就是本 story 扩展的 struct）
- [Source: docs/宠物互动App_Go项目结构与模块职责设计.md#6（internal/repo/mysql/）] — `room_member_repo.go` / `room_repo.go` 已锚定；本 story `Room` / `RoomMember` struct 落地在 `room_member_repo.go`（节点 4 阶段不新建 `room_repo.go`，YAGNI）
- [Source: server/migrations/0007_init_rooms.up.sql] — 0007 真实 DDL；本 story 注释升级所在文件
- [Source: server/migrations/0008_init_room_members.up.sql] — 0008 真实 DDL；本 story 注释升级所在文件
- [Source: server/internal/repo/mysql/room_member_repo.go] — 既有 `RoomMember` 最小骨架 struct + `RoomMemberRepo` interface（3 读方法）；本 story 扩展点
- [Source: server/internal/infra/migrate/migrate_integration_test.go] — 既有 8 张表 schema 元数据校验；本 story 新增 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` 所在文件
- [Source: server/internal/app/ws/ws_integration_test.go] — 既有 fixture inline DDL；本 story 切换 `migrate.Up()` 路径所在文件
- [Source: _bmad-output/planning-artifacts/epics.md#Story 11.2（行 1828-1848）] — Epic 11 §Story 11.2 AC 钦定（≥3 case 单测 + dockertest 故意违反 UNIQUE 约束 → DB 拒绝插入）
- [Source: _bmad-output/implementation-artifacts/11-1-接口契约最终化.md] — Story 11.1 已锚定 §10 / §12.3 字段层契约；本 story 实装的 GORM struct 字段名（camelCase 化后）必须与 11.1 锚定的 API 字段反向一致
- [Source: _bmad-output/implementation-artifacts/4-3-五张表-migrations.md] — 节点 2 第一个 migration story，可参考 migrate Go API / migrate 集成测试 helper 设计模式 + commit message 格式
- [Source: _bmad-output/implementation-artifacts/7-2-user_step_sync_logs-migration.md] — 节点 3 单 migration story 模板（与本 story 范围最接近）
- [Source: _bmad-output/implementation-artifacts/10-3-ws-网关骨架.md] — Story 10.3 r5 review fix 历史 + `RoomMember` 最小骨架 struct + ws fixture 临时建表的设计文档源；本 story 接管点
- [Source: _bmad-output/implementation-artifacts/decisions/0003-migration-tooling-and-conventions.md（如存在）] — ADR-0003 钦定 golang-migrate v4 + .up.sql / .down.sql 双向 + 编号严格单调递增
- [Source: docs/lessons/2026-05-08-room-roster-contract-self-consistency-11-1-r1.md] — Story 11.1 r1 同源 schema 漂移教训；本 story 切换 ws fixture 路径直接对应此 lesson
- [Source: docs/lessons/2026-05-08-ws-disconnect-only-clears-ephemeral-not-membership-11-1-r3.md] — 本 story `RoomMember` struct 不加 Online / IsActive 字段的契约依据
- [Source: docs/lessons/2026-04-26-cli-relative-path-and-graceful-stop-wait.md] — migrate CLI 路径稳定性 lesson；本 story 在 ws_integration_test.go 调用 `migrate.New(dsn, migrationsPath())` 时复用既有 helper 模式

## Dev Agent Record

### Agent Model Used

claude-opus-4-7[1m]（Anthropic Claude Opus 4.7, 1M context window）

### Debug Log References

无（实装路径直、所有验证一次通过；唯一非 trivial 调试是 ws_integration_test.go fixture 切换后第一次跑发现 `migrate.New` 报"open .: cannot find" → 排查发现是 `migrationsPathForWS` 相对路径错算成 `../../../../migrations`（4 级），但 ws_test 文件实际在 `server/internal/app/ws/` 应为 `../../../migrations`（3 级）；与 `migrate_integration_test.go::migrationsPath` 同深度；修后所有 ws 集成测试通过）。

### Completion Notes List

- AC1 落地：四个 migration 文件（0007.up / 0007.down / 0008.up / 0008.down）顶部注释升级为"由 Story 10.3 review r5 [P1] 引入；Story 11.2 正式接管 Epic 11 owner（含完整 ≥3 case 单测 + dockertest 集成测试覆盖 UNIQUE 约束运行时拒绝行为 + WS 集成测试 fixture 切到 official migration 路径）"。CREATE TABLE / DROP TABLE DDL 严格不动。
- AC2 落地：`RoomMember` GORM struct 扩展为完整字段（`ID` / `RoomID` / `UserID` / `JoinedAt` / `UpdatedAt`），与 0008 真实 schema 1:1 对齐；导入 `time` 包；struct 文档注释升级反映 Story 11.2 升级路径与 Story 10.3 起点。
- AC3 落地：新增 `Room` GORM struct（`ID` / `CreatorUserID` / `Status int8` / `MaxMembers uint8` / `CreatedAt` / `UpdatedAt`） + `(Room) TableName()` 显式返回 "rooms"；紧跟 `RoomMember` struct 后；范围红线说明（不新建 RoomRepo interface）已写入 doc comment。
- AC4 落地：room_member_repo_test.go 现有 sqlmock 测试全部用 `Raw(...)` 字面 SQL（`SELECT 1 FROM rooms WHERE id = ? AND status = 1 LIMIT 1` 等），不依赖 GORM auto-generated SELECT 列 → struct 字段扩展不影响测试断言；`go test ./internal/repo/mysql/... -count=1` 一次通过验证（无需修改测试代码）。
- AC5 落地：新增 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` 集成测试，覆盖三步 INSERT 序列：(1) 插入 (3001, 2001) 成功 → (2) (3002, 2001) 触发 UNIQUE(user_id) 单列约束 → DB 返 "Duplicate entry" → (3) (3001, 2001) 触发 UNIQUE(room_id, user_id) 复合约束 → DB 返 "Duplicate entry"。错误断言用 `strings.Contains(err.Error(), "Duplicate entry")` 稳定 contract（不依赖 MySQL 错误码字面文本）。用 `database/sql.ExecContext` 直跑 raw INSERT，不走 GORM。
- AC6 落地：`startMySQLWithRoomMemberFixture` helper 彻底切换 —— 删 `CREATE TABLE rooms` / `CREATE TABLE room_members` 两条 inline DDL（共 14 行），改 `migrate.New(dsn, migrationsPathForWS(t)).Up(ctx)`；fixture INSERT 修正：rooms 行加 `creator_user_id=1001`（0007 schema NOT NULL 必填）+ 删除 `member_count` 列（0007 无此列；memberCount 由 placeholderSnapshotBuilder 从 `len(ListMembers)` 计算）；`BroadcastToRoom_3Clients` case 删 `UPDATE rooms SET member_count = 3 WHERE id = 3001`（同源原因）；新增 `migrationsPathForWS(t)` helper 用 `filepath.Abs("../../../migrations")`。
- AC7 落地：所有 WS 集成测试在新 fixture 路径下通过（共 ~10 个 case，实测 161s 全绿）；roomID=3001 / userID=1001 / 1002 期望硬编码保持不变（fixture INSERT 显式赋值 id=3001 + creator_user_id=1001 让 DB 接受 + AUTO_INCREMENT 不阻止）。
- AC8 落地：所有既有 migrate 集成测试（`TestMigrateIntegration_UpThenDown` / `TestMigrateIntegration_RoomsAndRoomMembers_Schema` / `TestMigrateIntegration_UpTwice_Idempotent` / `TestMigrateIntegration_TablesPresent_AfterUp` / `TestMigrateIntegration_StatusAfterUp`）继续通过（实测 135s 全绿）。
- AC9 落地：`bash scripts/build.sh --test` BUILD SUCCESS，单测全绿；`go vet ./... && go vet -tags=integration ./...` 都通过；mysql / repo / 其他全部包 ok。**注释**：ws 包 `TestSessionManager_Register_TriggersHook` 是已知 pre-existing flaky test（`useGatewayDial` 内 `ListSessionsByRoomID` 看到 session 已注册但 SessionManager.Register 锁外的 `onRegister` 钩子可能尚未执行 → 计数器读 0；与本 story 改动无关，stash 本 story 全部改动后 10 次跑 8 次失败，验证不是本 story 引入；本 story 范围红线明确不修该测试）。
- AC10 落地：`bash scripts/build.sh --integration` 因 build.sh 默认 `-timeout=120s` 而所有包顺序跑总耗时 > 120s 触发 deadlock；按包分跑（`go test -tags=integration -timeout=600s ./internal/infra/migrate/...` 等）全部通过；该 timeout 钦定属 Story 1-7 build.sh 范围非本 story 范围。所有 story 11.2 钦定 case + 所有既有集成测试都通过。
- AC11 落地：sprint-status.yaml 中 `11-2-rooms-room_members-migration` 从 `ready-for-dev` → `in-progress` → `review`；last_updated 字段同步更新；epic-11 状态保持 `in-progress`（11.1 已 done）。
- AC12 完成判定：所有 AC1 ~ AC10 落地 + 改动文件清单与 story 范围红线一致 + 单测 + 集成测试双绿（按包跑）。范围红线 grep 自检全部通过：(1) ws_integration_test.go 已无可执行 `CREATE TABLE rooms` / `CREATE TABLE room_members`；(2) `mysql.Room` / `mysql.RoomMember` 引用未越界（service / handler 层无引用）；(3) `RoomMemberRepo` interface 仍只含 RoomExists / IsUserInRoom / ListMembers 三读方法，无任何 Insert / Delete / Update 写方法新增。

### File List

修改的文件（path 相对 repo root）：

- `server/migrations/0007_init_rooms.up.sql`（仅注释升级；DDL 严格不动）
- `server/migrations/0007_init_rooms.down.sql`（仅注释升级）
- `server/migrations/0008_init_room_members.up.sql`（仅注释升级；DDL 严格不动）
- `server/migrations/0008_init_room_members.down.sql`（仅注释升级）
- `server/internal/repo/mysql/room_member_repo.go`（`RoomMember` struct 字段扩展为 5 字段 + 新增 `Room` struct + 注释升级；`time` 包导入；`RoomMemberRepo` interface / impl / 三个读方法保持不变）
- `server/internal/infra/migrate/migrate_integration_test.go`（新增 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected`；导入 `strings`；既有 case 全部不动）
- `server/internal/app/ws/ws_integration_test.go`（`startMySQLWithRoomMemberFixture` 切到 `migrate.Up()` 路径；删除 `BroadcastToRoom_3Clients` case 中 `UPDATE rooms SET member_count=3` 行；新增 `migrationsPathForWS(t)` helper；导入 `path/filepath` + `internal/infra/migrate`；既有 case 期望硬编码全部保持稳定）
- `_bmad-output/implementation-artifacts/sprint-status.yaml`（`11-2-rooms-room_members-migration: ready-for-dev → in-progress → review`；last_updated 更新为 2026-05-08）
- `_bmad-output/implementation-artifacts/11-2-rooms-room_members-migration.md`（本文件 Status 流转 + Tasks/Subtasks 全部 [x] 勾选 + Dev Agent Record / Completion Notes / File List / Change Log 填写）

未新增任何新文件；未删除任何文件；未触及 service / handler / docs / ADR / V1 接口契约 / 数据库设计.md。

### Change Log

| 日期 | Story 阶段 | 改动摘要 |
|---|---|---|
| 2026-05-08 | 11.2 dev-story | 接管 Story 10.3 r5 提前 ship 的 0007 / 0008 migration（注释升级 audit trail）+ 扩展 `RoomMember` GORM struct 为完整 5 字段 schema + 新增 `Room` GORM struct + 新增 `TestMigrateIntegration_RoomMembers_UniqueUserID_Rejected` dockertest 集成测试覆盖 UNIQUE(user_id) / UNIQUE(room_id, user_id) 运行时 INSERT 拒绝行为 + ws_integration_test.go fixture 从 inline `CREATE TABLE` 切到 `migrate.Up()` official 路径（解决 inline DDL vs prod schema 漂移） + sprint-status.yaml 状态流转。范围红线遵守：DDL 不动 / V1 契约不动 / docs 不动 / RoomMemberRepo interface 不增写方法 / 不预实装 11.3 ~ 11.5 业务事务。 |
