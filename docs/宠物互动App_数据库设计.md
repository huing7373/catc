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
1 = healthkit
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

## 7.2 高优先级普通索引

建议保留：

- `user_step_sync_logs(user_id, sync_date)`
- `user_step_sync_logs(user_id, created_at)`
- `chest_open_logs(user_id, created_at)`
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

必须放入一个事务：

- 校验宝箱状态与版本
- 校验 `available_steps >= 1000`
- 扣减 `available_steps`
- 增加 `consumed_steps`
- 抽取装扮配置
- 插入一条 `user_cosmetic_items`
- 写 `chest_open_logs`
- 重建或刷新下一轮 `user_chests`

否则容易出现：

- 扣了步数没发奖
- 发了奖没扣步数
- 开奖成功但宝箱没刷新

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

建议一个事务内完成：

- 校验目标房间存在且未满
- 校验当前用户不在其他房间
- 插入 `room_members`
- 更新 `users.current_room_id`

---

## 8.7 退出房间事务

建议一个事务内完成：

- 删除 `room_members`
- 清空 `users.current_room_id`
- 如房间无人，则将 `rooms.status` 改为 `closed`

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

例如：

- `idem:{userId}:chest_open:{idempotencyKey}`
- `idem:{userId}:compose_upgrade:{idempotencyKey}`

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
