# 宠物互动 App Go 项目结构与模块职责设计

## 1. 文档说明

本文档用于定义当前版本后端工程的落地结构，重点说明：

- Go 后端总体工程组织方式
- 分层边界与模块职责
- HTTP / WebSocket 接入方式
- 各业务模块与接口、数据库之间的对应关系
- 事务、配置、日志、错误处理、测试建议
- 后续从模块化单体演进到服务拆分的方向

本文档基于当前已经确认的业务规则编写，并与以下文档保持一致：

- `宠物互动App_总体架构设计.md`
- `宠物互动App_V1接口设计.md`
- `宠物互动App_数据库设计.md`

---

## 2. 后端工程设计目标

当前阶段后端最合适的形态不是微服务，而是**模块化单体**。

这样设计的目标是：

1. **先保证业务快速落地**
   - 当前需求还在不断细化
   - 宝箱、装扮、合成、房间的规则仍可能继续调整

2. **让代码天然按领域拆分**
   - 虽然部署是单体，但内部边界要清晰
   - 避免所有业务都堆进 `handler` 或一个大 `service`

3. **为后续拆服务预留空间**
   - Room / Realtime
   - Reward / Chest
   - Asset / Inventory
   后续如果用户量增长，可以从模块边界自然拆出独立服务

---

## 3. 总体工程结论

当前建议采用：

- **Go 单体应用**
- **按领域划分模块**
- **按分层组织代码**
- **REST 处理普通业务请求**
- **WebSocket 处理房间实时互动**
- **MySQL 保存主业务数据**
- **Redis 保存实时态、幂等、防重、会话信息**

### 3.1 后端总体结构

```text
[HTTP API Layer]
  ├─ Auth Handler
  ├─ Home Handler
  ├─ Step Handler
  ├─ Chest Handler
  ├─ Cosmetic Handler
  ├─ Compose Handler
  ├─ Room Handler
  └─ Emoji Handler

[WebSocket Gateway]
  ├─ Room Session Manager
  ├─ Room Event Dispatcher
  ├─ Presence Manager
  └─ Broadcast Publisher

[Service Layer]
  ├─ Auth Service
  ├─ User Service
  ├─ Pet Service
  ├─ Step Service
  ├─ Chest Service
  ├─ Cosmetic Service
  ├─ Compose Service
  ├─ Room Service
  └─ Emoji Service

[Repository Layer]
  ├─ MySQL Repositories
  └─ Redis Repositories

[Infrastructure]
  ├─ Config
  ├─ Logger
  ├─ DB
  ├─ Redis
  ├─ ID Generator
  ├─ Clock
  └─ Middleware
```

---

## 4. 项目目录建议

建议目录如下：

```text
project/
├─ cmd/
│  └─ server/
│     └─ main.go
├─ configs/
│  ├─ local.yaml
│  ├─ dev.yaml
│  ├─ staging.yaml
│  └─ prod.yaml
├─ migrations/
│  ├─ 0001_init_users.sql
│  ├─ 0002_init_pet_and_step.sql
│  ├─ 0003_init_cosmetics.sql
│  ├─ 0004_init_room.sql
│  └─ ...
├─ internal/
│  ├─ app/
│  │  ├─ bootstrap/
│  │  │  ├─ config.go
│  │  │  ├─ logger.go
│  │  │  ├─ mysql.go
│  │  │  ├─ redis.go
│  │  │  ├─ router.go
│  │  │  └─ server.go
│  │  ├─ http/
│  │  │  ├─ middleware/
│  │  │  │  ├─ auth.go
│  │  │  │  ├─ request_id.go
│  │  │  │  ├─ recover.go
│  │  │  │  └─ logging.go
│  │  │  ├─ handler/
│  │  │  │  ├─ auth_handler.go
│  │  │  │  ├─ home_handler.go
│  │  │  │  ├─ step_handler.go
│  │  │  │  ├─ chest_handler.go
│  │  │  │  ├─ cosmetic_handler.go
│  │  │  │  ├─ compose_handler.go
│  │  │  │  ├─ room_handler.go
│  │  │  │  └─ emoji_handler.go
│  │  │  ├─ request/
│  │  │  └─ response/
│  │  └─ ws/
│  │     ├─ gateway.go
│  │     ├─ session_manager.go
│  │     ├─ room_hub.go
│  │     ├─ event_dispatcher.go
│  │     └─ message_types.go
│  ├─ domain/
│  │  ├─ auth/
│  │  ├─ user/
│  │  ├─ pet/
│  │  ├─ step/
│  │  ├─ chest/
│  │  ├─ cosmetic/
│  │  ├─ compose/
│  │  ├─ room/
│  │  └─ emoji/
│  ├─ service/
│  │  ├─ auth_service.go
│  │  ├─ home_service.go
│  │  ├─ step_service.go
│  │  ├─ chest_service.go
│  │  ├─ cosmetic_service.go
│  │  ├─ compose_service.go
│  │  ├─ room_service.go
│  │  └─ emoji_service.go
│  ├─ repo/
│  │  ├─ mysql/
│  │  │  ├─ user_repo.go
│  │  │  ├─ pet_repo.go
│  │  │  ├─ step_repo.go
│  │  │  ├─ chest_repo.go
│  │  │  ├─ cosmetic_repo.go
│  │  │  ├─ compose_repo.go
│  │  │  └─ room_repo.go
│  │  ├─ redis/
│  │  │  ├─ presence_repo.go
│  │  │  ├─ idempotency_repo.go
│  │  │  ├─ room_session_repo.go
│  │  │  └─ ws_repo.go
│  │  └─ tx/
│  │     └─ manager.go
│  ├─ infra/
│  │  ├─ config/
│  │  ├─ db/
│  │  ├─ redis/
│  │  ├─ logger/
│  │  ├─ clock/
│  │  └─ idgen/
│  └─ pkg/
│     ├─ errors/
│     ├─ response/
│     ├─ auth/
│     ├─ pointer/
│     └─ utils/
├─ docs/
│  └─ architecture/
├─ Makefile
├─ go.mod
└─ go.sum
```

---

## 5. 分层职责设计

为了避免业务代码散落，建议严格区分以下几层。

## 5.1 Handler 层

负责：

- 路由注册
- 请求参数解析与基础校验
- 从上下文中提取用户身份
- 调用 service
- 统一返回结构
- 处理 HTTP 状态码与业务码映射

不负责：

- 编写复杂业务逻辑
- 直接操作数据库
- 跨多个 repo 手动拼事务

### 适合放在 handler 的逻辑

- `BindJSON` / 参数校验
- `path/query/header` 提取
- `Authorization` 读取
- 调用 `svc.OpenChest(...)`
- 返回 `response.Success(...)`

---

## 5.2 Service 层

负责：

- 核心业务逻辑
- 跨模块编排
- 事务边界控制
- 幂等控制
- 业务规则校验
- 将 repo 读写组合成完整动作

不负责：

- 解析 HTTP
- 直接依赖 Gin / Echo 等具体框架类型
- 直接依赖 Redis / MySQL 底层连接细节

### 适合放在 service 的逻辑

- 游客登录初始化
- 开箱扣步数并发奖
- 手动选材合成
- 房间加入/退出
- 首页聚合数据组装

---

## 5.3 Repository 层

负责：

- 数据读写
- SQL 查询与更新
- Redis key 读写
- 行锁、条件更新、批量写入等底层操作封装

不负责：

- 决定业务规则
- 决定是否允许用户开箱
- 决定合成是否合法

### 适合放在 repo 的逻辑

- `FindUserByGuestUID`
- `CreateUser`
- `LockUserStepAccount`
- `CreateChestOpenLog`
- `BatchMarkCosmeticsConsumed`
- `AddRoomMember`

---

## 5.4 Domain 层

负责：

- 领域模型
- 枚举定义
- 规则常量
- 领域内纯逻辑判断

例如：

- 装扮品质枚举
- 宝箱状态枚举
- 房间人数上限常量
- `CanUpgradeRarity(fromRarity)`

### 领域层不应该依赖

- HTTP 框架
- ORM 实现
- Redis client

---

## 5.5 Infrastructure 层

负责：

- 配置读取
- 数据库初始化
- Redis 初始化
- Logger
- Server 启动
- 时钟、id 生成器、trace 等基础能力

---

## 6. 业务模块职责划分

当前建议至少拆成以下模块。

---

## 6.1 Auth 模块

职责：

- 游客登录
- Token 签发与校验
- 微信绑定
- 登录方式绑定关系管理

关联接口：

- `POST /api/v1/auth/guest-login`
- `POST /api/v1/auth/bind-wechat`
- `GET /api/v1/me`

关联表：

- `users`
- `user_auth_bindings`

### 核心职责边界

Auth 负责“用户如何进入系统”，但不负责首页聚合、背包、开箱等业务。

---

## 6.2 User / Home 模块

职责：

- 当前用户信息读取
- 首页聚合数据拼装
- 当前房间快照
- 基础用户资料读取

关联接口：

- `GET /api/v1/me`
- `GET /api/v1/home`

关联表：

- `users`
- `pets`
- `user_step_accounts`
- `user_chests`
- `user_pet_equips`
- `rooms`

### 说明

`/home` 是典型聚合接口，建议单独有 `home_service.go`，不要把首页拼装逻辑散在多个 handler 中。

---

## 6.3 Pet 模块

职责：

- 默认宠物初始化
- 当前宠物状态同步
- 当前宠物穿戴展示聚合

关联接口：

- `POST /api/v1/pets/current/state-sync`

关联表：

- `pets`
- `user_pet_equips`
- `user_cosmetic_items`
- `cosmetic_items`

### 说明

宠物状态是展示态，不是资产态。

因此：

- 可以写库，但不要高频强依赖
- 更适合“关键时机同步”而不是传感器变化就刷库

---

## 6.4 Step 模块

职责：

- 步数同步
- 步数资产记账
- 日志记录
- 可用步数读取

关联接口：

- `POST /api/v1/steps/sync`
- `GET /api/v1/steps/account`

关联表：

- `user_step_accounts`
- `user_step_sync_logs`

### 说明

Step 是资产模块。

其核心责任不是“展示用户今天走了多少步”，而是：

- 安全记账
- 处理重复同步
- 为开箱消耗提供可信余额

---

## 6.5 Chest 模块

职责：

- 当前宝箱状态查询
- 宝箱解锁判定
- 开箱消耗步数
- 奖励发放
- 下一轮宝箱创建

关联接口：

- `GET /api/v1/chest/current`
- `POST /api/v1/chest/open`

关联表：

- `user_chests`
- `user_step_accounts`
- `chest_open_logs`
- `user_cosmetic_items`
- `cosmetic_items`

### 说明

Chest 不是单纯配置模块，而是资产消耗与发奖模块。

开箱必须由 `chest_service` 统一处理，不能让 handler 分散调多个 repo。

---

## 6.6 Cosmetic 模块

职责：

- 装扮目录配置读取
- 玩家背包读取
- 单件实例穿戴 / 卸下
- 装扮实例与配置聚合

关联接口：

- `GET /api/v1/cosmetics/catalog`
- `GET /api/v1/cosmetics/inventory`
- `POST /api/v1/cosmetics/equip`
- `POST /api/v1/cosmetics/unequip`

关联表：

- `cosmetic_items`
- `user_cosmetic_items`
- `user_pet_equips`

### 说明

当前系统里“装扮”已经是实例化资产。

因此 Cosmetic 模块必须同时理解两层含义：

- 配置层：这是什么装扮
- 实例层：玩家实际拥有的这件装扮

---

## 6.7 Compose 模块

职责：

- 手动选材合成
- 校验 10 个实例是否合法
- 校验同品质规则
- 消耗材料实例
- 发放高阶实例
- 记录合成日志

关联接口：

- `GET /api/v1/compose/overview`
- `POST /api/v1/compose/upgrade`

关联表：

- `user_cosmetic_items`
- `cosmetic_items`
- `compose_logs`
- `compose_log_materials`

### 说明

Compose 目前是独立业务模块，不建议放进 Cosmetic Service 中直接处理。

原因：

- 有独立规则
- 有独立事务
- 有独立日志
- 后续可能继续扩展保底、活动加成、特殊材料等

---

## 6.8 Room 模块

职责：

- 创建房间
- 加入房间
- 退出房间
- 房间详情读取
- 成员数量校验

关联接口：

- `POST /api/v1/rooms`
- `GET /api/v1/rooms/current`
- `GET /api/v1/rooms/{roomId}`
- `POST /api/v1/rooms/{roomId}/join`
- `POST /api/v1/rooms/{roomId}/leave`

关联表：

- `rooms`
- `room_members`
- `users`

### 说明

Room 模块负责“关系与状态”，但不负责实时广播的连接管理。

---

## 6.9 Emoji 模块

职责：

- 系统表情配置读取
- 表情合法性校验

关联接口：

- `GET /api/v1/emojis`

关联表：

- `emoji_configs`

### 说明

当前表情只是固定配置，逻辑很轻，不需要做复杂状态管理。

---

## 6.10 Realtime / WS 模块

职责：

- WebSocket 连接管理
- 房间内事件广播
- 用户在线状态管理
- 心跳与断线处理
- 房间快照推送
- 表情消息派发

关联入口：

- `GET /ws/rooms/{roomId}`

关联存储：

- Redis presence
- Redis room session

### 说明

Realtime 模块与 Room 模块要协作，但不能完全混在一起。

建议职责边界为：

- Room：成员关系合法性、创建/加入/退出
- Realtime：连接、广播、在线态

---

## 7. 模块调用关系建议

建议遵循以下方向：

```text
Handler -> Service -> Repo
Handler -> Service -> Domain Rule
WS Gateway -> Service -> Repo
```

避免以下情况：

```text
Handler -> Repo
Handler -> Redis Client
Handler -> 多表事务
```

### 7.1 典型流程：开箱

```text
ChestHandler
  -> ChestService.OpenCurrentChest(userID, idempotencyKey)
      -> IdempotencyRepo.CheckAndLock(...)
      -> ChestRepo.LockCurrentChest(userID)
      -> StepRepo.LockStepAccount(userID)
      -> CosmeticRepo.PickRewardByWeight(...)
      -> CosmeticRepo.CreateUserCosmeticItem(...)
      -> ChestRepo.CreateChestOpenLog(...)
      -> ChestRepo.ResetOrCreateNextChest(...)
      -> IdempotencyRepo.SaveResult(...)
```

### 7.2 典型流程：手动合成

```text
ComposeHandler
  -> ComposeService.Upgrade(userID, fromRarity, itemIDs, idempotencyKey)
      -> IdempotencyRepo.CheckAndLock(...)
      -> CosmeticRepo.LockUserCosmeticItems(userID, itemIDs)
      -> ComposeDomain.ValidateMaterials(...)
      -> CosmeticRepo.BatchConsumeItems(itemIDs)
      -> CosmeticRepo.PickRewardByRarity(nextRarity)
      -> CosmeticRepo.CreateUserCosmeticItem(...)
      -> ComposeRepo.CreateComposeLog(...)
      -> ComposeRepo.BatchCreateComposeMaterials(...)
      -> IdempotencyRepo.SaveResult(...)
```

### 7.3 典型流程：房间内发表情

```text
WS Gateway
  -> Auth validate
  -> RoomService.CheckUserInRoom(userID, roomID)
  -> EmojiService.ValidateEmojiCode(code)
  -> RoomHub.Broadcast(roomID, emojiEvent)
```

---

## 8. HTTP 层设计建议

## 8.1 路由分组

建议：

```text
/api/v1/auth
/api/v1/me
/api/v1/home
/api/v1/pets
/api/v1/steps
/api/v1/chest
/api/v1/cosmetics
/api/v1/compose
/api/v1/rooms
/api/v1/emojis
/ws/rooms
```

## 8.2 中间件建议

至少包含：

- `request_id`
- `recover`
- `logging`
- `auth`
- `rate_limit`（至少对登录、开箱、合成做保护）

## 8.3 参数结构体建议

建议把请求体和响应体单独放在：

- `internal/app/http/request`
- `internal/app/http/response`

优点：

- 避免 handler 入参和 domain model 混用
- 对外协议演进更清晰

---

## 9. WebSocket 结构建议

当前房间实时互动比较轻，建议做一个简洁的 room hub 模型。

## 9.1 核心对象建议

### Session

表示单个用户的单个连接。

建议包含：

- `userID`
- `roomID`
- `conn`
- `sendChan`
- `lastHeartbeatAt`

### RoomHub

表示一个房间的在线广播中心。

建议包含：

- `roomID`
- `sessions`
- `broadcastChan`
- `joinChan`
- `leaveChan`

### EventDispatcher

负责解析消息类型并转给对应业务动作。

---

## 9.2 WS 事件边界

建议：

- 消息解析在 `ws/event_dispatcher.go`
- 房间连接与广播在 `ws/room_hub.go`
- 业务合法性校验仍通过 service 完成

例如：

- `emoji.send` -> `emoji_service + room_service`
- `ping` -> ws 层直接回复 `pong`

---

## 10. 事务边界建议

项目中建议抽一层事务管理器，例如：

- `repo/tx/manager.go`

提供统一调用方式：

```go
err := txManager.WithTx(ctx, func(txCtx context.Context) error {
    // do business
    return nil
})
```

## 10.1 必须放事务的操作

### 游客登录初始化

需要原子完成：

- 创建用户
- 创建 auth binding
- 创建默认宠物
- 创建步数账户
- 创建初始宝箱

### 开箱

需要原子完成：

- 锁宝箱
- 锁步数账户
- 扣步数
- 发奖励实例
- 写开箱日志
- 刷新下一轮宝箱

### 穿戴

建议原子完成：

- 读取实例
- 校验槽位
- 卸旧装备
- 设新装备
- 更新实例状态

### 合成

需要原子完成：

- 锁定 10 个实例
- 校验品质与状态
- 标记消耗
- 发高阶实例
- 写合成日志
- 写材料明细

### 加入房间

建议原子完成：

- 判断用户不在房间
- 判断目标房间未满
- 写 room_members
- 写 users.current_room_id

---

## 11. 错误码与错误处理建议

建议统一定义业务错误类型，例如：

- `ErrInvalidParam`
- `ErrUnauthorized`
- `ErrChestNotUnlockable`
- `ErrInsufficientSteps`
- `ErrComposeInvalidMaterials`
- `ErrRoomFull`

并统一在：

- `internal/pkg/errors`

中维护。

### 建议模式

- repo 返回底层错误
- service 将底层错误转成业务错误
- handler 再映射为统一响应结构

这样可以避免：

- SQL 错误直接暴露到接口层
- 各 handler 自己散乱地写错误码

---

## 12. 配置与环境管理建议

## 12.1 配置来源

建议支持：

- YAML 文件
- 环境变量覆盖

## 12.2 配置模块划分

建议至少包括：

- server
- mysql
- redis
- log
- auth
- ws
- feature_flags

例如：

```yaml
server:
  http_port: 8080
  read_timeout_sec: 5
  write_timeout_sec: 10

mysql:
  dsn: xxx
  max_open_conns: 50
  max_idle_conns: 10

redis:
  addr: 127.0.0.1:6379
  password: ""
  db: 0

auth:
  token_secret: xxx
  token_expire_sec: 604800

ws:
  heartbeat_timeout_sec: 60
```

---

## 13. 日志与可观测性建议

## 13.1 日志要求

至少输出：

- request id
- user id
- api path
- latency
- business result
- error detail

## 13.2 关键操作日志

以下动作建议打印结构化业务日志：

- 游客登录初始化
- 微信绑定
- 步数同步
- 开箱
- 合成
- 房间创建/加入/退出
- WS 连接建立/断开

## 13.3 监控指标建议

至少预留以下指标：

- 接口 QPS
- 接口耗时
- 错误率
- 开箱成功次数
- 合成成功次数
- 当前在线房间数
- 当前在线连接数

---

## 14. 测试建议

## 14.1 单元测试重点

建议优先覆盖：

- `CanUpgradeRarity`
- 合成材料校验
- 宝箱状态判断
- 房间人数判断
- 步数增量判断

## 14.2 Service 测试重点

重点测试：

- `GuestLogin`
- `OpenCurrentChest`
- `ComposeUpgrade`
- `JoinRoom`
- `LeaveRoom`

## 14.3 集成测试重点

建议至少为以下流程补集成测试：

- 游客登录初始化全流程
- 开箱事务全流程
- 合成事务全流程
- 房间创建/加入/退出全流程

---

## 15. 与数据库设计的映射关系

为了便于开发阶段对照，建议模块与核心表的关系如下：

| 模块 | 核心表 |
|---|---|
| Auth | `users`, `user_auth_bindings` |
| Home | `users`, `pets`, `user_step_accounts`, `user_chests`, `user_pet_equips` |
| Pet | `pets`, `user_pet_equips`, `user_cosmetic_items`, `cosmetic_items` |
| Step | `user_step_accounts`, `user_step_sync_logs` |
| Chest | `user_chests`, `user_step_accounts`, `chest_open_logs`, `user_cosmetic_items`, `cosmetic_items` |
| Cosmetic | `cosmetic_items`, `user_cosmetic_items`, `user_pet_equips` |
| Compose | `user_cosmetic_items`, `cosmetic_items`, `compose_logs`, `compose_log_materials` |
| Room | `rooms`, `room_members`, `users` |
| Emoji | `emoji_configs` |

---

## 16. 后续演进建议

当业务扩大后，可以按当前模块边界逐步拆分：

## 16.1 第一优先可拆模块

### Realtime / Room

当房间在线人数与 WS 连接明显增长时，可优先拆：

- 房间连接服务
- 广播服务
- Presence 服务

### Reward / Chest

当宝箱、活动、掉落池开始变复杂时，可独立拆：

- 掉落规则
- 活动奖池
- 奖励发放服务

## 16.2 暂时不建议提前拆的模块

- Auth
- Cosmetic
- Compose
- Step

这些模块当前强依赖业务一致性，拆早了收益不大。

---

## 17. 当前工程设计结论

当前最合适的 Go 后端落地方案是：

- 采用**模块化单体**
- 严格区分 `handler / service / repo / domain / infra`
- 普通业务用 HTTP
- 房间互动用 WebSocket
- 资产动作全部通过 service 统一编排事务
- 房间实时态放 Redis，不让 MySQL 承担高频在线状态职责

这套结构足以支撑当前 MVP 的完整实现，并且能在未来用户量增长时自然向服务拆分演进。
