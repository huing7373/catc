# 宠物互动 App 数据库设计

## 1. 文档说明

本文档定义当前版本 MVP 的数据库设计，覆盖：

- 存储边界与设计原则
- 主键、时间、状态等统一约定
- 业务表划分
- 详细字段设计
- 状态枚举
- 索引与唯一约束
- 关键事务建议
- Redis 职责边界

本文档基于当前已确认的产品规则编写，尤其包含以下约束：

- 首版支持游客登录，后续支持绑定微信
- 用户登录后获得默认猫咪
- 步数读取 iPhone 系统步数，并作为可消费资源
- 同时只能存在 1 个当前宝箱，解锁倒计时 10 分钟，开启固定消耗 1000 步
- 装扮为实例化道具，每个玩家持有的道具实例都有唯一 id
- 合成时玩家手动选择 10 个同品质道具实例，产出 1 个高一阶品质随机装扮实例
- 房间最多 4 人，用户同时只能在 1 个房间中
- 房间表情为固定配置，仅做广播提示

---

## 2. 存储设计原则

### 2.1 存储边界

当前系统采用：

- **MySQL 8.0**：主存储，保存账号、资产、房间关系、日志等核心业务数据
- **Redis**：缓存与实时态，保存在线状态、WebSocket 会话、幂等键等临时数据

建议遵循如下原则：

- **资产类数据必须落 MySQL**
- **关系类数据必须落 MySQL**
- **高频在线状态不要只靠 MySQL 维护**
- **房间实时广播不依赖 MySQL 轮询**

### 2.2 领域数据分类

可以将系统数据分为 4 类：

1. **身份数据**
   - 用户主档
   - 登录绑定关系

2. **资产数据**
   - 步数账户
   - 装扮实例
   - 宝箱状态

3. **展示数据**
   - 当前宠物
   - 当前穿戴
   - 最近宠物状态

4. **实时互动数据**
   - 房间成员关系
   - 在线状态
   - WebSocket 会话
   - 表情广播

---

## 3. 统一约定

### 3.1 主键策略

第一版建议全部业务表采用：

- `BIGINT UNSIGNED` 主键
- 自增 id 或统一 id 生成器均可

如果当前服务是单体 Go 应用，MVP 阶段采用数据库自增主键即可。

对前端返回 id 时，建议统一按**字符串**返回，避免客户端对大整数处理不一致。

### 3.2 时间字段

除只记录创建时间的纯日志表外，建议默认保留：

- `created_at DATETIME(3) NOT NULL`
- `updated_at DATETIME(3) NOT NULL`

需要记录事件时间的表，按业务再增加：

- `obtained_at`
- `consumed_at`
- `unlock_at`
- `joined_at`
- `opened_at`

### 3.3 状态字段

建议统一使用 `TINYINT` / `SMALLINT` 存储状态枚举，不直接在表中存英文字符串。

优点：

- 更利于索引与比较
- 更利于 Go / iOS 枚举统一
- 更利于后期规则演进

### 3.4 软删与硬删

首版建议：

- **资产轨迹尽量保留，不直接物理删除**
- **高频临时关系可物理删除**

例如：

- 装扮实例被合成消耗时，建议更新 `status=consumed`，而不是直接删记录
- 房间成员离开房间时，可直接删除 `room_members` 当前成员关系

### 3.5 命名约定

建议统一：

- 表名：复数，下划线风格，例如 `user_auth_bindings`
- 字段名：下划线风格，例如 `current_room_id`
- 布尔语义字段：用 `is_xxx`
- 枚举字段：用业务名，例如 `status`、`rarity`、`slot`

---

## 4. 数据库表总览

### 4.1 账号与用户

- `users`
- `user_auth_bindings`

### 4.2 宠物与装扮

- `pets`
- `cosmetic_items`
- `user_cosmetic_items`
- `user_pet_equips`

### 4.3 步数与宝箱

- `user_step_accounts`
- `user_step_sync_logs`
- `user_chests`
- `chest_open_logs`
- `chest_open_idempotency_records`（r5 review 锁定，详见 §5.16）

### 4.4 合成

- `compose_logs`
- `compose_log_materials`

### 4.5 房间

- `rooms`
- `room_members`

### 4.6 配置

- `emoji_configs`

---

## 5. 详细表设计

---

## 5.1 users

用户主表。

```sql
CREATE TABLE users (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    guest_uid VARCHAR(128) NOT NULL,
    nickname VARCHAR(64) NOT NULL DEFAULT '',
    avatar_url VARCHAR(255) NOT NULL DEFAULT '',
    status TINYINT NOT NULL DEFAULT 1,
    current_room_id BIGINT UNSIGNED NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_guest_uid (guest_uid),
    KEY idx_current_room_id (current_room_id),
    KEY idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 字段说明

- `guest_uid`：客户端生成并保存在 Keychain 的游客身份标识
- `nickname`：用户昵称
- `avatar_url`：用户头像
- `status`：账号状态
- `current_room_id`：当前所在房间 id，可为空

### 设计说明

- `guest_uid` 放在用户表中，便于游客快速查找
- 真正的登录方式绑定关系仍以 `user_auth_bindings` 为准
- `current_room_id` 是用户当前房间快照字段，便于首页快速查询

---

## 5.2 user_auth_bindings

用户登录绑定关系表。

```sql
CREATE TABLE user_auth_bindings (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    auth_type TINYINT NOT NULL,
    auth_identifier VARCHAR(128) NOT NULL,
    auth_extra JSON NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_auth_type_identifier (auth_type, auth_identifier),
    KEY idx_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 字段说明

- `user_id`：关联用户 id
- `auth_type`：登录类型，例如游客、微信
- `auth_identifier`：登录凭证标识
  - 游客时可存 `guest_uid`
  - 微信时可存 `openid` 或 `unionid`
- `auth_extra`：扩展登录信息，例如微信附加资料

### 关键约束

同一个登录身份只能绑定一个账号：

- `UNIQUE(auth_type, auth_identifier)`

---

## 5.3 pets

宠物表。

```sql
CREATE TABLE pets (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    pet_type TINYINT NOT NULL DEFAULT 1,
    name VARCHAR(64) NOT NULL DEFAULT '',
    current_state TINYINT NOT NULL DEFAULT 1,
    is_default TINYINT NOT NULL DEFAULT 1,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_user_default_pet (user_id, is_default),
    KEY idx_user_id (user_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 字段说明

- `pet_type`：宠物类型，当前为猫
- `name`：宠物名称
- `current_state`：最近一次同步的宠物状态
- `is_default`：是否默认宠物

### 设计说明

- 当前虽然是一人一只猫，但表结构不写死“一人只能一只宠物”
- `current_state` 用于最近状态展示，不建议做高频刷库

---

## 5.4 user_step_accounts

步数账户总表。

```sql
CREATE TABLE user_step_accounts (
    user_id BIGINT UNSIGNED PRIMARY KEY,
    total_steps BIGINT UNSIGNED NOT NULL DEFAULT 0,
    available_steps BIGINT UNSIGNED NOT NULL DEFAULT 0,
    consumed_steps BIGINT UNSIGNED NOT NULL DEFAULT 0,
    version BIGINT UNSIGNED NOT NULL DEFAULT 0,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 字段说明

- `total_steps`：累计获得步数
- `available_steps`：当前可消费步数
- `consumed_steps`：累计消耗步数
- `version`：乐观锁版本号，用于扣减并发保护

### 设计说明

步数在当前系统中是可消费资产，不应只做展示值。

建议后端统一按“账户模型”记账，避免后续：

- 开箱扣减
- 活动补偿
- 异常修复

出现账务不清的问题。

---

## 5.5 user_step_sync_logs

步数同步日志表。

```sql
CREATE TABLE user_step_sync_logs (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    sync_date DATE NOT NULL,
    client_total_steps BIGINT UNSIGNED NOT NULL,
    accepted_delta_steps INT NOT NULL DEFAULT 0,
    motion_state TINYINT NOT NULL DEFAULT 1,
    source TINYINT NOT NULL DEFAULT 1,
    client_ts BIGINT UNSIGNED NOT NULL DEFAULT 0,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    KEY idx_user_date (user_id, sync_date),
    KEY idx_user_created_at (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 字段说明

- `sync_date`：客户端上报的自然日
- `client_total_steps`：客户端读取到的“当天系统累计步数”
- `accepted_delta_steps`：服务端实际确认入账的增量
- `motion_state`：同步时客户端活动状态
- `source`：步数来源
- `client_ts`：客户端时间戳

### 设计说明

客户端不直接上传“本次增加了多少步”，而应上传：

- 当前自然日
- 系统当天累计步数

由服务端根据最近同步记录计算差值，这样能更好应对：

- 重复同步
- 异常倒退
- 回前台重新同步

---

## 5.6 user_chests

当前宝箱表。

```sql
CREATE TABLE user_chests (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    status TINYINT NOT NULL,
    unlock_at DATETIME(3) NOT NULL,
    open_cost_steps INT UNSIGNED NOT NULL DEFAULT 1000,
    version BIGINT UNSIGNED NOT NULL DEFAULT 0,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_user_id (user_id),
    KEY idx_status_unlock_at (status, unlock_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 字段说明

- `status`：当前宝箱状态
- `unlock_at`：可开启时间点
- `open_cost_steps`：开启所需步数，当前固定为 1000
- `version`：乐观锁版本号，用于防重复开启

### 设计说明

- 一个用户始终只有一个“当前宝箱”
- 倒计时结束后，服务端可视为 `unlockable`
- 宝箱未开启前不生成下一轮
- 宝箱可开启但未开启时，该记录一直保留

---

## 5.7 chest_open_logs

开箱日志表。

```sql
CREATE TABLE chest_open_logs (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    chest_id BIGINT UNSIGNED NOT NULL,
    cost_steps INT UNSIGNED NOT NULL,
    reward_user_cosmetic_item_id BIGINT UNSIGNED NOT NULL,
    reward_cosmetic_item_id BIGINT UNSIGNED NOT NULL,
    reward_rarity TINYINT NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    KEY idx_user_id_created_at (user_id, created_at),
    KEY idx_reward_cosmetic_item_id (reward_cosmetic_item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 字段说明

- `chest_id`：被开启的宝箱 id
- `cost_steps`：实际消耗步数
- `reward_user_cosmetic_item_id`：产出的装扮实例 id
- `reward_cosmetic_item_id`：产出的装扮配置 id
- `reward_rarity`：奖励品质

### 设计说明

开箱日志建议单独保存，原因：

- 方便追踪掉落问题
- 方便用户历史展示
- 便于运营分析与概率排查

---

## 5.8 cosmetic_items

装扮配置表。

```sql
CREATE TABLE cosmetic_items (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    code VARCHAR(64) NOT NULL,
    name VARCHAR(64) NOT NULL,
    slot TINYINT NOT NULL,
    rarity TINYINT NOT NULL,
    asset_url VARCHAR(255) NOT NULL DEFAULT '',
    icon_url VARCHAR(255) NOT NULL DEFAULT '',
    drop_weight INT UNSIGNED NOT NULL DEFAULT 0,
    is_enabled TINYINT NOT NULL DEFAULT 1,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_code (code),
    KEY idx_slot_rarity (slot, rarity),
    KEY idx_enabled_weight (is_enabled, drop_weight)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 字段说明

- `code`：全局唯一业务编码
- `name`：装扮名称
- `slot`：部位
- `rarity`：品质
- `asset_url`：装扮资源地址
- `icon_url`：图标资源地址
- `drop_weight`：掉落权重
- `is_enabled`：是否启用

### 设计说明

这张表表示“装扮是什么”，不是“玩家拥有哪一件”。

例如：

- `id = 12` 可以代表“小黄帽”这个配置
- 玩家具体获得的小黄帽实例则记录在 `user_cosmetic_items`

---

## 5.9 user_cosmetic_items

玩家装扮实例表。

```sql
CREATE TABLE user_cosmetic_items (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    cosmetic_item_id BIGINT UNSIGNED NOT NULL,
    status TINYINT NOT NULL DEFAULT 1,
    source TINYINT NOT NULL DEFAULT 1,
    source_ref_id BIGINT UNSIGNED NULL,
    obtained_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    consumed_at DATETIME(3) NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    KEY idx_user_id_status (user_id, status),
    KEY idx_user_id_cosmetic_item_id (user_id, cosmetic_item_id),
    KEY idx_source (source, source_ref_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 字段说明

- `id`：玩家道具实例 id，即每一个道具的唯一 id
- `cosmetic_item_id`：对应的装扮配置 id
- `status`：当前实例状态
- `source`：来源类型
- `source_ref_id`：来源关联记录 id
- `obtained_at`：获得时间
- `consumed_at`：消耗时间，未消耗时为空

### 设计说明

当前产品已经明确：

- 每个玩家道具都需要唯一 id
- 合成时玩家要手动选择消耗哪些道具

因此必须使用实例化库存，而不是简单的 `count` 聚合库存。

实例化模型带来的好处：

- 用户可以手动选择具体材料
- 后端可以准确记录消费链路
- 后续支持锁定、回收、活动绑定更自然

---

## 5.10 user_pet_equips

宠物穿戴关系表。

```sql
CREATE TABLE user_pet_equips (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    pet_id BIGINT UNSIGNED NOT NULL,
    slot TINYINT NOT NULL,
    user_cosmetic_item_id BIGINT UNSIGNED NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_pet_slot (pet_id, slot),
    UNIQUE KEY uk_user_cosmetic_item_id (user_cosmetic_item_id),
    KEY idx_user_pet (user_id, pet_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 字段说明

- `slot`：装备槽位
- `user_cosmetic_item_id`：被穿戴的装扮实例 id

### 设计说明

虽然多个实例可能来自同一个配置，但既然玩家道具已经实例化，则穿戴关系也应挂到**实例 id**，而不是配置 id。

关键约束：

- 一个宠物同一部位只能穿一件：`UNIQUE(pet_id, slot)`
- 一件实例同时只能装备一次：`UNIQUE(user_cosmetic_item_id)`

---

## 5.11 compose_logs

合成日志表。

```sql
CREATE TABLE compose_logs (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    from_rarity TINYINT NOT NULL,
    to_rarity TINYINT NOT NULL,
    consumed_count INT UNSIGNED NOT NULL DEFAULT 10,
    reward_user_cosmetic_item_id BIGINT UNSIGNED NOT NULL,
    reward_cosmetic_item_id BIGINT UNSIGNED NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    KEY idx_user_id_created_at (user_id, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 字段说明

- `from_rarity`：材料品质
- `to_rarity`：目标品质
- `consumed_count`：固定为 10
- `reward_user_cosmetic_item_id`：产出实例 id
- `reward_cosmetic_item_id`：产出配置 id

### 设计说明

合成是重要资产行为，建议单独留日志，便于：

- 用户历史查看
- 概率排查
- 异常补单

---

## 5.12 compose_log_materials

合成材料明细表。

```sql
CREATE TABLE compose_log_materials (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    compose_log_id BIGINT UNSIGNED NOT NULL,
    user_cosmetic_item_id BIGINT UNSIGNED NOT NULL,
    cosmetic_item_id BIGINT UNSIGNED NOT NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),

    KEY idx_compose_log_id (compose_log_id),
    KEY idx_user_cosmetic_item_id (user_cosmetic_item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 字段说明

- `compose_log_id`：关联合成日志 id
- `user_cosmetic_item_id`：被消耗的具体实例 id
- `cosmetic_item_id`：该实例对应的配置 id

### 设计说明

既然玩家需要手动选择材料，那么合成日志不仅要记录“合成结果”，还应记录“具体消耗了哪 10 个实例”。

这样更利于审计、排查和补偿。

---

## 5.13 rooms

房间主表。

```sql
CREATE TABLE rooms (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    creator_user_id BIGINT UNSIGNED NOT NULL,
    status TINYINT NOT NULL DEFAULT 1,
    max_members TINYINT UNSIGNED NOT NULL DEFAULT 4,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    KEY idx_creator_user_id (creator_user_id),
    KEY idx_status_created_at (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 字段说明

- `creator_user_id`：创建者用户 id
- `status`：房间状态
- `max_members`：最大成员数，当前固定 4

### 设计说明

房间建议作为轻持久化对象存在：

- 只要还有成员，就保持 active
- 房间没人后，状态可置为 closed

---

## 5.14 room_members

房间当前成员表。

```sql
CREATE TABLE room_members (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    room_id BIGINT UNSIGNED NOT NULL,
    user_id BIGINT UNSIGNED NOT NULL,
    joined_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_user_id (user_id),
    UNIQUE KEY uk_room_user (room_id, user_id),
    KEY idx_room_id (room_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 设计说明

这个表只存“当前成员”，不做历史存档。

关键约束：

- 一个用户同时只能在一个房间：`UNIQUE(user_id)`
- 房间内同一用户只能出现一次：`UNIQUE(room_id, user_id)`

用户退出房间后，当前成员关系可以直接删除。

---

## 5.15 emoji_configs

系统表情配置表。

```sql
CREATE TABLE emoji_configs (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    code VARCHAR(64) NOT NULL,
    name VARCHAR(64) NOT NULL,
    asset_url VARCHAR(255) NOT NULL DEFAULT '',
    sort_order INT NOT NULL DEFAULT 0,
    is_enabled TINYINT NOT NULL DEFAULT 1,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_code (code),
    KEY idx_enabled_sort (is_enabled, sort_order)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 设计说明

表情目前为系统固定集合，不需要单独设计“用户拥有的表情实例”。

---

## 5.16 chest_open_idempotency_records

开箱接口幂等记录表（**r5 review 锁定 DB 持久化方案，r6 review 把预声明纳入业务事务消除 pending 卡死悖论，r7 review 移除 best-effort failed upsert 简化为二态机**，详见 V1接口设计 §7.2 服务端逻辑步骤 3a / 3b / 3k / 5 + §13.3 + 关键约束「r7 移除 best-effort failed upsert 决策」段）。

```sql
CREATE TABLE chest_open_idempotency_records (
    id BIGINT UNSIGNED PRIMARY KEY AUTO_INCREMENT,
    user_id BIGINT UNSIGNED NOT NULL,
    idempotency_key VARCHAR(128) NOT NULL,
    status ENUM('pending', 'success') NOT NULL DEFAULT 'pending',
    response_json JSON NULL,
    created_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3),
    updated_at DATETIME(3) NOT NULL DEFAULT CURRENT_TIMESTAMP(3) ON UPDATE CURRENT_TIMESTAMP(3),

    UNIQUE KEY uk_user_id_key (user_id, idempotency_key),
    KEY idx_status_created_at (status, created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 字段说明

- `user_id`：发起开箱请求的用户 id（与 `users.id` 对齐；不建外键，与本设计其他表保持一致）
- `idempotency_key`：client 传入的幂等键；约束与 V1接口设计 §7.2 一致（`[A-Za-z0-9_:-]` + length 1-128）
- `status`：**二态机**（r7 review 锁定，从 r6 三态机 `('pending', 'success', 'failed')` 简化）—— `pending`（声明已写入，业务事务**持锁执行中**；对其他事务不可见，它们在 InnoDB unique-key X-lock 上阻塞）/ `success`（业务事务已 commit，`response_json` 已落盘）。**无** `failed` 状态：事务 rollback 时 pending 行随之消失（事务原子性保证），同 key 重试在 INSERT 拿 `affected_rows = 1` 等价首次到达；server 端**禁止**写"post-rollback failed 占位行"（详见 V1接口设计 §7.2 r7 决策段：该补偿与 client 同 key 重试的合法 success 路径在 UNIQUE 约束上竞争，可能错误覆盖 success → failed，破坏数据一致性）
- `response_json`：`status = 'success'` 时缓存的完整 V1 响应（`{code, message, data: {reward, stepAccount, nextChest.{id, status, unlockAt, openCostSteps}}}`）；**不**包含时间派生字段如 `nextChest.remainingSeconds`（r6 review 锁定：该字段由 server 在响应序列化时按 `max(0, ceil((unlock_at - now) / 1s))` 实时计算填入，避免同 key 重试回放 stale 倒计时）；**不**包含顶层 `requestId`（r7 review 锁定：`requestId` 是每次请求独立的 trace ID，缓存若包含会导致同 key 重试回放**首次**请求的 trace ID 给**本次**重试响应，破坏 log / trace 关联语义；server 在响应序列化时**重新填**当前请求的 `requestId`，与 `remainingSeconds` 同样作为"上层动态字段"处理）；`status = 'pending'` 时为 NULL
- `created_at` / `updated_at`：标准时间戳，`updated_at` 在 `status` 推进时自动更新

### 索引说明

- `uk_user_id_key`：UNIQUE 约束兼任原子声明依据 + 并发阻塞排队依据 —— V1接口设计 §7.2 步骤 3a 在业务事务内首条语句用 `INSERT ... ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)` 借此 UNIQUE 做 single-statement 原子 claim；同 `(user_id, idempotency_key)` 的并发请求被 InnoDB unique-key X-lock 阻塞排队，首个事务结束（commit / rollback）后其他事务再继续 —— commit → 行已存在 → `affected_rows = 0` 短路；rollback → 行已消失 → `affected_rows = 1` 走全流程
- `idx_status_created_at`：辅助索引，支持运维清理任务按 `status` + `created_at` 范围扫描（如清理 N 天前的 `success` 记录控制表大小）；MVP 阶段无需主动清理（DB 容量足够）

### 设计说明（**契约决策来源**）

本表的引入是 Story 20-1 r5 / r6 / r7 review 锁定的契约层决定：

- **背景**：r3 / r4 review 把开箱幂等设计在 Redis（`idem:{userId}:chest_open:{idempotencyKey}` key + sentinel + final-response 双 TTL）；r5 review 指出 Redis 是非事务存储，与 MySQL 不能形成原子写 —— "MySQL 事务 commit 成功 + Redis SET 失败" case 下 client 无法区分"首次已生效"vs"首次未生效"，可能引发重复扣步数 + 重复出箱
- **r5 决策**：把幂等记录从 Redis 上移到 MySQL；最终化 UPDATE 与业务事务**同事务原子写**；DB 成为"首次是否成功"的单一可信源
- **r6 修订**：r5 把预声明 INSERT 写在业务事务**之前**作独立 INSERT —— 业务事务 rollback 时 pending 行**不**跟着回滚，同 key 重试永久卡 1008（事务 rollback 后状态语义错位悖论）。r6 把预声明 INSERT 也纳入业务事务，作为事务内**首条语句**，借 InnoDB unique-key X-lock 实现并发阻塞排队；rollback 同步清除 pending 行 → 同 key 重试等价于首次到达走全流程，**无 pending 残留卡死**
- **r7 修订**：r6 保留了"post-rollback 可选 best-effort 写 `status='failed'` 占位行"作为 UX 优化（让 client 立即明确换 key）。r7 review 锁定该补偿与"client 立即同 key 重试 → 新事务 commit success"的合法路径在 UNIQUE 约束上 race —— compensation INSERT 因 unique 冲突触发 `ON DUPLICATE KEY UPDATE status='failed'` 会把后到达的 success 行错误覆盖为 failed，破坏数据一致性。r7 决策：彻底**移除** best-effort failed upsert；schema 简化为 `status ENUM('pending', 'success')` 二态机；`response_json` 缓存同时移除顶层 `requestId`（每次请求独立 trace ID，重试请求需重新填当前 trace ID）
- **lesson 文档**：
  - `docs/lessons/2026-05-14-db-same-tx-idempotency-replaces-redis-writeback-fragility-20-1-r5.md`（r5）
  - `docs/lessons/2026-05-14-idempotency-pre-claim-must-be-inside-business-tx-20-1-r6.md`（r6，pending 回滚悖论 + response_json 缓存不含时间派生字段）
  - `docs/lessons/2026-05-14-idempotency-no-async-failed-compensation-and-no-cached-requestId-20-1-r7.md`（r7，移除 best-effort failed upsert race + response_json 缓存不含 requestId）

### 阶段适用

- **节点 7（Epic 20 / Story 20.6 实装）**：本表**仅**服务 `POST /api/v1/chest/open` 接口
- **节点 11 / Epic 32（合成事务）**：若 `POST /api/v1/compose/upgrade` 复用同一持久化模式，**应**起新表 `compose_upgrade_idempotency_records`（schema 同结构，UNIQUE 与索引同款，**采用二态机 `('pending', 'success')`，禁止引入异步 failed 补偿写**）；**或**抽出通用表 `idempotency_records` 加 `api_name` 字段统一兜管 —— 由 Story 32.4 在节点 11 落地前锚定。本 story 20-1 仅钦定节点 7 的 `chest_open_idempotency_records` 表

### migration 归属

本表 schema 由 Story 20-1（契约 finalize）锚定，**migration SQL 由 Story 20.6（开箱事务实装）或独立 follow-up story（如 Story 20.10）落地**；本 story 20-1 不直接产生 migration 文件。详见 V1接口设计 §7.2 / docs/sprint-status.yaml 20-1 follow-up 说明。

---

## 6. 状态枚举汇总

### 6.1 users.status

```text
1 = normal
2 = disabled
3 = banned
```

### 6.2 user_auth_bindings.auth_type

```text
1 = guest
2 = wechat
```

### 6.3 pets.pet_type

```text
1 = cat
```

### 6.4 pets.current_state

```text
1 = rest
2 = walk
3 = run
```

### 6.5 user_step_sync_logs.motion_state

```text
1 = stationary_or_unknown
2 = walking
3 = running
```

### 6.6 user_step_sync_logs.source

```text
1 = healthkit       # 客户端正常上报（POST /api/v1/steps/sync, 见 V1接口设计 §6.1）
2 = admin_grant     # dev / 运营手动发放（POST /dev/grant-steps, 见 Story 7.5）
```

### 6.7 user_chests.status

```text
1 = counting
2 = unlockable
```

### 6.8 cosmetic_items.slot

```text
1  = hat
2  = gloves
3  = glasses
4  = neck
5  = back
6  = body
7  = tail
99 = other
```

### 6.9 cosmetic_items.rarity

```text
1 = common
2 = rare
3 = epic
4 = legendary
```

### 6.10 user_cosmetic_items.status

```text
1 = in_bag
2 = equipped
3 = consumed
4 = invalid
```

### 6.11 user_cosmetic_items.source

```text
1 = chest
2 = compose
3 = admin_grant
4 = event_reward
```

### 6.12 rooms.status

```text
1 = active
2 = closed
```

---

## 7. 索引与唯一约束建议

## 7.1 必须保留的唯一约束

### user_auth_bindings

- `UNIQUE(auth_type, auth_identifier)`

保证同一个登录身份只能绑定一个用户。

### user_chests

- `UNIQUE(user_id)`

保证一个用户同时只有一个当前宝箱。

### room_members

- `UNIQUE(user_id)`

保证一个用户同一时间只在一个房间。

### user_pet_equips

- `UNIQUE(pet_id, slot)`
- `UNIQUE(user_cosmetic_item_id)`

保证一个槽位只能穿一件，一件实例只能被装备一次。

### emoji_configs / cosmetic_items

- `UNIQUE(code)`

保证配置 code 稳定唯一。

### chest_open_idempotency_records

- `UNIQUE(user_id, idempotency_key)`

兼任原子声明依据 + 并发阻塞排队依据：V1接口设计 §7.2 步骤 3a 在业务事务内首条语句用 `INSERT ... ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)` 借此 UNIQUE 做 single-statement 原子 claim；同 `(user_id, idempotency_key)` 的并发请求被 InnoDB unique-key X-lock 阻塞排队，首个事务结束（commit / rollback）后其他事务再继续 —— commit → 行已存在 → `affected_rows = 0` 短路；rollback → 行已消失 → `affected_rows = 1` 走全流程（r5 review 锁定 / r6 review 修订 / r7 review 简化；canonical 文案见 §5.16 索引说明）。

## 7.2 高优先级普通索引

建议保留：

- `user_step_sync_logs(user_id, sync_date)`
- `user_step_sync_logs(user_id, created_at)`
- `chest_open_logs(user_id, created_at)`
- `chest_open_idempotency_records(status, created_at)`（运维清理辅助）
- `compose_logs(user_id, created_at)`
- `room_members(room_id)`
- `user_cosmetic_items(user_id, status)`
- `user_cosmetic_items(user_id, cosmetic_item_id)`
- `user_auth_bindings(user_id)`
- `pets(user_id)`

---

## 8. 关键事务设计

## 8.1 游客登录初始化事务

建议放入同一个事务：

- 创建 `users`
- 创建 `user_auth_bindings`
- 创建 `pets`
- 创建 `user_step_accounts`
- 创建 `user_chests`

这样可以避免：

- 用户创建了，但默认猫没发
- 账号创建了，但宝箱没初始化

---

## 8.2 步数同步事务

建议一个事务内完成：

- 查询最近同步记录
- 计算有效增量
- 更新 `user_step_accounts`
- 写 `user_step_sync_logs`

如有防重需求，可配合 Redis 限频。

---

## 8.3 开箱事务

必须放入一个事务（**预声明 + 业务写入 + 最终化全部同事务**，r6 review 锁定）：

- **预声明 `chest_open_idempotency_records` 行**（事务内**首条语句**：`INSERT ... ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id)`，借 `UNIQUE(user_id, idempotency_key)` 阻塞同 key 并发；详见 V1接口设计 §7.2 步骤 3a）
- 校验宝箱状态与版本
- 校验 `available_steps >= 1000`
- 扣减 `available_steps`
- 增加 `consumed_steps`
- 抽取装扮配置
- 插入一条 `user_cosmetic_items`
- 写 `chest_open_logs`
- 重建或刷新下一轮 `user_chests`
- **最终化 `chest_open_idempotency_records.status = 'success'` + `response_json`**（r5 review 锁定，r6 review 维持，r7 review 收紧 schema 为二态机；详见 V1接口设计 §7.2 步骤 3k）

> **节点 7 vs 节点 8 阶段差异（与 V1接口设计 §7.2 / §14.1 一致）**：本节描述的是**最终契约**（节点 8 / Epic 23 完成后稳态）。**节点 7 阶段（Story 20.6 / Epic 21 验收期）**「插入一条 `user_cosmetic_items`」步骤暂不执行 —— `chest_open_logs.reward_user_cosmetic_item_id` 写占位 `0`；详见 V1接口设计 §7.2.4h。Story 23.5 落地后回归本节最终契约语义。

> **幂等记录同事务原子写（r5 review 锁定 / r6 review 修订 / r7 review 简化）**：`chest_open_idempotency_records` 行的预声明 INSERT（pending）+ 最终化 UPDATE（pending → success）+ `response_json` 写入与业务表写入**全部在同一事务**原子提交（V1接口设计 §7.2 步骤 3a / 3k + §7.2 关键约束「事务边界」）；这构成"幂等状态 + 业务数据"单一可信源，client 同 key 重试始终安全。r5 钦定 UPDATE 同事务，r6 进一步把 INSERT 也纳入同一事务（消除 r5 "业务事务 rollback 时 pending 行不跟随回滚 → 同 key 永久 1008 卡死"悖论）；r7 进一步**移除** r6 保留的"post-rollback best-effort 写 `status='failed'`"补偿（与 client 同 key 重试 success 在 UNIQUE 上 race，可错误覆盖 success → failed），schema 简化为 `status ENUM('pending', 'success')` 二态机；`response_json` 缓存同时**不**包含顶层 `requestId`（每次请求独立 trace ID，重试请求需重新填当前 trace ID）。**移除** r4 钦定的"Redis sentinel + 双 TTL + 异步 SET 重试"路径（详见 §5.16 设计说明 + V1接口设计 §13.3）。

否则容易出现：

- 扣了步数没发奖
- 发了奖没扣步数
- 开奖成功但宝箱没刷新
- **幂等状态与业务数据不一致**（如业务 commit 成功但幂等记录未更新 → client 同 key 重试可能误判"首次未发生" → 重复扣步数 + 重复出箱）
- **pending 残留卡死**（如预声明写在业务事务之外作独立 INSERT → 业务事务 rollback 时 pending 行残留 → client 同 key 重试永久命中 pending 返回 1008）

---

## 8.4 穿戴事务

建议一个事务内完成：

- 校验实例归属与状态
- 查询对应装扮配置槽位
- 若该槽位已有旧装备，则旧装备状态改回 `in_bag`
- 更新 `user_pet_equips`
- 新装备实例状态改为 `equipped`

---

## 8.5 合成事务

必须放入一个事务：

- 锁定 10 个被选择的 `user_cosmetic_items`
- 校验归属、状态、品质
- 将 10 个实例改为 `consumed`
- 随机生成 1 个更高品质装扮配置
- 插入新的 `user_cosmetic_items`
- 写 `compose_logs`
- 写 `compose_log_materials`

这是当前系统最典型的“资产消耗 + 产出”事务。

---

## 8.6 加入房间事务

事务内必须严格按顺序完成（详见 V1 接口设计 §10.4 服务端逻辑）：

1. 开事务
2. **`SELECT ... FROM rooms WHERE id = ? FOR UPDATE`**（对 rooms 行加排他锁；与并发 §8.7 leave 跨事务串行化）
3. 校验房间存在且 `status = 1 active`（否则回滚 → 6001 / 6005）
4. `SELECT COUNT(*) FROM room_members WHERE room_id = ?` 校验未满（否则回滚 → 6002）
5. 插入 `room_members`（撞 UNIQUE → 回滚 → 6003）
6. 更新 `users.current_room_id`
7. 提交事务

**`FOR UPDATE` 的双重职责**（r9 P1#2 锁定）：

- 同房间并发 join 串行化（5 个 user 抢 1 个空位，只有 1 个成功）
- **跨事务与 §8.7 leave 串行化**：阻止 "leave A 删 row → join B 锁 rooms 看 status=1 → join B insert + commit → leave A UPDATE rooms.status=2" 这种 timeline 产生的 `rooms.status=closed` 但 `room_members` 非空 drift。两类事务在 `rooms` 行 lock 上排队后，B join 必须等 A leave 提交（含 status=2）后才能进入，B 看到 status=2 → 直接返回 6005，状态保持一致。

---

## 8.7 退出房间事务

事务内必须严格按顺序完成（详见 V1 接口设计 §10.5 服务端逻辑）：

1. 开事务
2. **`SELECT ... FROM rooms WHERE id = ? FOR UPDATE`**（对 rooms 行加排他锁；与并发 §8.6 join 跨事务串行化，确保后续 step 4 remaining-count + step 5 status update 与并发 join 不交错）
3. `DELETE FROM room_members WHERE room_id = ? AND user_id = ?` + 检查 `RowsAffected == 0`（同一 user 并发两次 leave 输家兜底 → 回滚 → 6004）
4. 更新 `users.current_room_id = NULL`
5. `SELECT COUNT(*) FROM room_members WHERE room_id = ?`，若 `== 0` → 更新 `rooms.status = 2 closed`
6. 提交事务

**`FOR UPDATE` 与 `RowsAffected == 0` 兜底正交**：前者解决"跨事务 leave-vs-join 状态 drift"（r9 P1#2），后者解决"同一 user 并发两次 leave 输家走完后续步骤产生重复广播"（r4 已锁定）。两者都是**必须**项，锁对象不同（`rooms` 行 vs `room_members` 行），不冲突。

---

## 8.8 房间详情读快照事务（含 ACL 共享锁）

`GET /api/v1/rooms/{roomId}`（V1 接口设计 §10.3）虽然为只读查询，但需要在同一 MySQL 事务内（隔离级别 = **REPEATABLE READ**，InnoDB 默认）完成多步操作。**snapshot 隔离 + FOR SHARE 行锁两个机制是互补的，缺一不可**：

- 步骤 1a：`SELECT users.current_room_id` —— ACL 校验（caller 是否仍是该房间成员）
- 步骤 1b：**`SELECT 1 FROM room_members WHERE room_id = ? AND user_id = caller FOR SHARE`** —— 取共享锁锁定 caller 自己的成员行，**阻止并发 leave 的 DELETE 在本读事务期间提交**（DELETE 需要排他锁，与 FOR SHARE 互斥，必须等本读事务 commit 后才能继续）
- 步骤 3：`SELECT room_members JOIN users JOIN pets` —— 受 ACL 保护的 roster 隐私数据

**rationale**：

- **snapshot 隔离仅提供"事务内部一致性"**：两次 SELECT 看到同一时刻的状态。但**不**保证 "caller 在事务持续期间仍是成员" —— 具体 race（r9 P1#1 锁定）：(t0) 本 GET 事务开始，snapshot 锁定在 t0 状态（caller 是成员）→ (t1) 并发 `POST /rooms/{roomId}/leave` 提交 DELETE → (t2) 本 GET 步骤 3 用 t0 snapshot SELECT 看到 caller 仍是成员，照返完整 roster → (t3) 事务 commit + HTTP 响应发出，但 caller 已离开。snapshot 没有阻止 t1 的 DELETE 提交，因此**外部一致性**被破坏。
- **FOR SHARE 行锁提供"外部一致性"**：让并发 leave 的 DELETE 必须等本读事务 commit / rollback 才能继续，从而保证**事务持续期间（含 SELECT roster 步骤；commit 时 lock 释放）**ACL 仍然成立。
- 两个机制缺一不可：仅 snapshot 没锁 → race 仍存在；仅 FOR SHARE 没 snapshot → 步骤 1 与步骤 3 可能看到不同状态（read committed 即每次 SELECT 看到最新 commit）。

**关于 commit-after-response 残留窗口（r12 锁定）**：FOR SHARE 锁在事务 commit 时释放，而 handler 通常执行顺序是 `commit tx → 序列化 JSON → flush response body`，因此 **commit → flush** 之间存在 μs 量级窗口，并发 leave 可能在该窗口内 commit DELETE → roster 字节流到达对端时 caller 已离开。本协议层面**承认**该 best-effort 残留 race，**不**强制 server 在 commit 前先序列化 + flush 后再 commit（实装代价大，需重构 handler 模式）。该窗口的端到端实质安全由 **client 防御性 discard**（client 已 leave 后收到的 stale roster 直接丢弃）兜底，而非 server 端协议保证。详见 V1接口设计.md §10.3 "rationale" 段同源说明。

**关于 deadlock 风险**：caller 锁的是 `(room_id, user_id=caller)` 这一**特定行**；并发 leave 也只锁 `user_id=caller` 这一行（取排他锁后 DELETE）；其他成员 / 其他房间走的是不同 `(room_id, user_id)` 锁路径，不存在锁顺序循环 → 无 deadlock 风险。最坏情况是并发 leave 等本读事务 commit 后再提交，等待时长 = 本读事务全步骤时长（μs ~ 数 ms 量级），可忽略。

本读事务与写事务（§8.1 / §8.6 / §8.7）并列，归入"读快照事务（含 ACL 共享锁）"类型，方便后续 audit 时统一识别"哪些只读接口需要 snapshot 一致性 + FOR SHARE 锁"。后续若新增"`/me` 读个人 + 关联资源"等多次 SELECT 形态、且任一次 SELECT 是 ACL 校验 / 任一次 SELECT 是受 ACL 保护的数据，应同样开读快照事务并对 ACL row 加 FOR SHARE 锁。

---

## 9. Redis 职责边界

## 9.1 建议放 Redis 的数据

### 在线态与会话映射

例如：

- `room:{roomId}:online_users`
- `user:{userId}:ws_session`

### 心跳状态

例如：

- `user:{userId}:last_ping_ts`

### 幂等与防重键

**注（r5 review 锁定）**：`POST /api/v1/chest/open` 的幂等记录**不**在 Redis，已上移到 MySQL `chest_open_idempotency_records` 表（与业务事务同事务原子写）；详见 §5.16 + V1接口设计 §13.3。`POST /api/v1/compose/upgrade` 预期复用同一 DB 持久化模式，由 Story 32.4 在节点 11 落地前锚定（起新表或共用通用表）。

**Redis 在幂等路径上的角色**已**移除**（r4 的"sentinel TTL 60s + final-response TTL 24h"双 TTL 方案在 r5 全面废弃）。如未来引入其他不需要"同事务原子性"的轻量幂等场景（如 GET 防重、客户端去抖），可重新评估 Redis 方案，但**资产类**操作（开箱 / 合成 / 穿戴等会改 MySQL 业务数据的接口）**禁止**使用 Redis 做幂等存储 —— Redis 非事务存储的特性与"幂等记录 + 业务数据原子"诉求根本冲突。

### 限频控制

例如：

- 步数同步频率限制
- WebSocket 表情发送频率限制

## 9.2 不建议只放 Redis 的数据

以下数据必须以 MySQL 为主存储：

- 用户账号
- 步数账户
- 当前宝箱
- 装扮实例
- 合成日志
- 当前房间成员关系

---

## 10. 未来扩展建议

### 10.1 宝箱类型扩展

如果未来引入不同宝箱，可在 `user_chests` 中增加：

- `chest_type`
- `reward_pool_id`

### 10.2 掉落池扩展

如果未来需要按宝箱或活动配置不同掉落池，可新增：

- `reward_pools`
- `reward_pool_items`

当前 MVP 只在 `cosmetic_items.drop_weight` 上做统一权重即可。

### 10.3 房间历史事件

如果未来需要做房间行为回放或审计，可新增：

- `room_event_logs`
- `room_emoji_events`

当前 MVP 不强制要求落库。

### 10.4 更多实例属性

如果未来装扮实例需要新增属性，例如：

- 绑定状态
- 锁定状态
- 限时有效期
- 染色/强化信息

当前实例化结构已支持自然扩展。

---

## 11. 当前数据库设计结论

当前数据库模型已经可以支撑以下 MVP 功能：

- 游客登录与后续绑定微信
- 默认猫咪宠物初始化
- iPhone 步数同步与步数账户记账
- 单宝箱倒计时与 1000 步开箱
- 装扮配置 + 装扮实例模型
- 穿戴 / 卸下装扮
- 玩家手动选择 10 个同品质装扮实例进行合成
- 4 人房间创建、加入、退出与状态维护
- 系统固定表情配置

同时，这套设计也保留了后续扩展空间，适合作为第一版正式落库方案。
