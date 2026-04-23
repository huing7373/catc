# 宠物互动 App 总体架构设计

## 1. 文档说明

本文档用于沉淀当前版本已经确认的产品规则、总体架构、模块划分、核心数据模型与关键业务流程。

当前技术方向：

- iPhone 客户端：Swift + SwiftUI
- 后端：Go 单体应用（模块化单体）
- 接口协议：REST + WebSocket
- 主存储：MySQL 8.0
- 缓存与实时态：Redis

---

## 2. 产品定位

这是一个围绕“宠物陪伴 + 运动成长 + 轻联机互动”的 iPhone App。

用户登录后会获得一只默认猫咪作为宠物。猫咪会根据用户现实中的运动状态切换展示动作；用户通过步行、跑步获得步数；主界面存在一个定时解锁的宝箱，用户可消耗步数开启宝箱并获得装扮；用户还能进入房间与其他玩家进行轻量实时互动，并发送系统表情。

---

## 3. 当前已确认的产品规则

### 3.1 账号系统

- 首版支持 **游客登录**
- 后续支持 **绑定微信登录**
- 游客账号升级为微信账号后，原有数据不能丢失
- 一个真实用户在服务端始终对应一个稳定的 `user_id`

### 3.2 宠物系统

- 用户登录后获得一只默认猫咪
- 猫咪会根据用户运动状态表现不同动作：
  - 静止/休息 -> 猫休息
  - 步行 -> 猫移动
  - 跑步 -> 猫跑步
- 当前产品语义里的“坐下休息”，技术上按“静止状态”处理

### 3.3 步数系统

- 步数读取 iPhone 系统统计步数
- 跑步与步行获得的步数收益一致
- 步数既是成长记录，也是可消耗资源
- 开宝箱固定消耗 **1000 步数**

### 3.4 宝箱系统

- 同时只能存在 **1 个当前宝箱**
- 宝箱倒计时为 **10 分钟**
- 倒计时完成后，宝箱变为可开启状态
- 若宝箱已经可开启但用户未开启，则该宝箱一直保留
- 开启后生成下一轮宝箱

### 3.5 装扮系统

- 装扮可用于给猫穿戴
- 装扮存在不同部位，例如：帽子、手套等
- 每个装扮有稀有度与掉落权重
- **每一个玩家持有的道具实例都有唯一 id**
- 同一种装扮配置可被用户拥有多件，每一件都是独立实例

### 3.6 合成系统

- 玩家合成时，必须 **手动选择要消耗的道具**
- 合成材料必须是 **10 个同品质道具实例**
- 不要求相同部位
- 不要求相同配置 id
- 合成结果为 **1 个高一阶品质的随机装扮实例**
- 例如：
  - 10 个 common -> 1 个 rare
  - 10 个 rare -> 1 个 epic
  - 10 个 epic -> 1 个 legendary
  - legendary 不可继续合成

### 3.7 房间系统

- 房间最多 **4 人**
- 用户同时只能加入 **1 个房间**
- 用户可以主动退出房间
- 用户不能踢出其他成员
- 只要房间内还有成员，房间就不解散
- 房间更适合定义为“轻社交互动空间”，而不是强管理型房间

### 3.8 表情系统

- 表情为系统固定配置
- 表情仅用于房间内广播提示
- 表情不会改变宠物属性或触发复杂玩法

---

## 4. 总体技术架构

## 4.1 架构结论

第一阶段建议采用：

- **客户端模块化架构**
- **Go 模块化单体后端**
- **REST 处理普通业务请求**
- **WebSocket 处理房间实时互动**

当前不建议直接做微服务，原因如下：

- 需求仍在快速变化
- 规则还在持续细化
- 实时模块和资产模块边界尚未演化成熟
- MVP 重点应放在快速验证核心玩法，而不是增加系统复杂度

## 4.2 总体结构

```text
[iPhone App]
  ├─ Auth Module
  ├─ Home / Pet Module
  ├─ Step & Motion Module
  ├─ Chest Module
  ├─ Wardrobe Module
  ├─ Compose Module
  ├─ Room Module
  ├─ Emoji Module
  ├─ REST Client
  ├─ WebSocket Client
  └─ Local Cache / Session / Timer

                │
                │ HTTPS / WebSocket
                ▼

[Go Backend]
  ├─ Auth Domain
  ├─ User Domain
  ├─ Pet Domain
  ├─ Step Domain
  ├─ Chest Domain
  ├─ Cosmetic Domain
  ├─ Compose Domain
  ├─ Room Domain
  ├─ Emoji Domain
  └─ Realtime Gateway

                │
      ┌─────────┴─────────┐
      ▼                   ▼
 [MySQL]               [Redis]
```

---

## 5. 客户端架构设计

## 5.1 技术选型

- 语言：Swift
- UI：SwiftUI
- 架构：MVVM + UseCase + Repository
- 网络：REST API
- 实时通信：WebSocket
- 运动能力：HealthKit / CoreMotion

## 5.2 客户端模块划分

```text
App
├─ AppEntry
├─ Core
│  ├─ Networking
│  ├─ Realtime
│  ├─ Storage
│  ├─ Session
│  ├─ Motion
│  ├─ Health
│  └─ DesignSystem
├─ FeatureAuth
├─ FeatureHome
├─ FeaturePet
├─ FeatureChest
├─ FeatureWardrobe
├─ FeatureCompose
├─ FeatureRoom
├─ FeatureEmoji
└─ SharedModels
```

## 5.3 分层职责

### UI 层

负责：

- 页面展示
- 动画与交互
- 本地倒计时显示
- 错误提示与状态反馈

### ViewModel 层

负责：

- 页面状态管理
- 调用 UseCase
- 处理 loading / error / empty
- 接收 WebSocket 推送后刷新 UI

### UseCase 层

负责封装明确业务动作，例如：

- `GuestLoginUseCase`
- `SyncStepsUseCase`
- `OpenChestUseCase`
- `EquipCosmeticUseCase`
- `ComposeUpgradeUseCase`
- `CreateRoomUseCase`
- `SendEmojiUseCase`

### Repository 层

负责统一数据来源：

- REST API
- WebSocket
- 本地缓存
- 运动与健康数据读取

## 5.4 页面建议

- 启动页 / 登录页
- 主界面
- 开箱结果弹窗
- 背包 / 装扮页
- 合成页
- 房间页
- 表情选择弹层
- 微信绑定页

---

## 6. 后端架构设计

## 6.1 技术选型

- 语言：Go
- 运行形态：模块化单体
- HTTP：Gin / Echo / Chi 任选其一
- ORM / DB：GORM 或 sqlx
- 存储：MySQL
- 缓存与在线态：Redis

## 6.2 后端分层

### Handler 层

负责：

- HTTP 路由
- 参数校验
- 响应封装
- WebSocket 接入

### Service 层

负责：

- 业务逻辑
- 事务控制
- 规则判断
- 跨模块编排

### Repository 层

负责：

- MySQL 读写
- Redis 读写
- 查询封装

### Domain 层

负责：

- 实体
- 状态枚举
- 领域规则
- 领域服务

## 6.3 推荐目录

```text
project/
├─ cmd/
│  └─ server/
│     └─ main.go
├─ internal/
│  ├─ app/
│  │  ├─ http/
│  │  ├─ ws/
│  │  └─ bootstrap/
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
│  ├─ repo/
│  │  ├─ mysql/
│  │  └─ redis/
│  ├─ infra/
│  └─ pkg/
├─ migrations/
├─ configs/
└─ docs/
```

---

## 7. 核心业务模块职责

## 7.1 Auth 模块

负责：

- 游客登录
- 微信绑定
- Token 签发与校验
- 身份升级而非数据迁移

## 7.2 User 模块

负责：

- 用户基础资料
- 当前房间关系
- 当前默认宠物引用

## 7.3 Pet 模块

负责：

- 默认猫咪发放
- 宠物当前展示状态
- 穿戴后的外观聚合

## 7.4 Step 模块

负责：

- 步数同步
- 步数账户记账
- 防重复同步
- 异常增量过滤

## 7.5 Chest 模块

负责：

- 当前宝箱状态
- 10 分钟倒计时
- 开箱资格校验
- 扣步数
- 掉落奖励
- 刷新下一轮宝箱

## 7.6 Cosmetic 模块

负责：

- 装扮静态配置
- 装扮实例管理
- 背包查询
- 穿戴 / 卸下

## 7.7 Compose 模块

负责：

- 手动选材合成
- 材料校验
- 实例消耗
- 随机产出高阶实例
- 合成日志记录

## 7.8 Room 模块

负责：

- 创建房间
- 加入房间
- 退出房间
- 房间成员管理
- 房间生命周期维护

## 7.9 Realtime / Emoji 模块

负责：

- WebSocket 连接管理
- 房间快照广播
- 成员进出通知
- 表情广播

---

## 8. 数据存储边界

## 8.1 MySQL 存储

必须落库的数据：

- 用户账号
- 登录绑定关系
- 宠物信息
- 步数账户
- 步数同步日志
- 当前宝箱状态
- 开箱日志
- 装扮配置
- 装扮实例
- 宠物穿戴关系
- 合成日志
- 房间与房间成员
- 表情配置

## 8.2 Redis 存储

建议放 Redis 的数据：

- 房间在线会话映射
- WebSocket 心跳状态
- 临时房间快照
- 幂等键
- 高频在线态

---

## 9. 核心数据模型

## 9.1 用户与登录

### users

用户主表，包含：

- `id`
- `guest_uid`
- `nickname`
- `avatar_url`
- `status`
- `current_room_id`

### user_auth_bindings

登录方式绑定表，包含：

- `user_id`
- `auth_type`（guest / wechat）
- `auth_identifier`

## 9.2 宠物

### pets

- `id`
- `user_id`
- `pet_type`
- `name`
- `current_state`
- `is_default`

## 9.3 步数

### user_step_accounts

- `user_id`
- `total_steps`
- `available_steps`
- `consumed_steps`
- `version`

### user_step_sync_logs

- `user_id`
- `sync_date`
- `client_total_steps`
- `accepted_delta_steps`
- `motion_state`
- `client_ts`

## 9.4 宝箱

### user_chests

- `user_id`
- `status`
- `unlock_at`
- `open_cost_steps`
- `version`

### chest_open_logs

- `user_id`
- `chest_id`
- `cost_steps`
- `reward_item_id`
- `reward_count`
- `reward_rarity`

## 9.5 装扮配置与实例

### cosmetic_items

静态装扮配置表，包含：

- `id`
- `code`
- `name`
- `slot`
- `rarity`
- `asset_url`
- `icon_url`
- `drop_weight`
- `is_enabled`

### user_cosmetic_items

玩家装扮实例表，包含：

- `id`（即 `userCosmeticItemId`）
- `user_id`
- `cosmetic_item_id`
- `status`
- `source`
- `source_ref_id`
- `obtained_at`
- `consumed_at`

> 这里的 `id` 是每一件道具实例的唯一 id。

### user_pet_equips

宠物穿戴关系表，包含：

- `user_id`
- `pet_id`
- `slot`
- `user_cosmetic_item_id`

## 9.6 合成

### compose_logs

- `user_id`
- `from_rarity`
- `to_rarity`
- `consumed_count`
- `reward_item_id`

### compose_log_materials

- `compose_log_id`
- `user_cosmetic_item_id`
- `cosmetic_item_id`

## 9.7 房间

### rooms

- `id`
- `creator_user_id`
- `status`
- `max_members`

### room_members

- `room_id`
- `user_id`
- `joined_at`

## 9.8 表情

### emoji_configs

- `code`
- `name`
- `asset_url`
- `sort_order`
- `is_enabled`

---

## 10. 状态与枚举建议

### pets.current_state

- `1 = rest`
- `2 = walk`
- `3 = run`

### user_chests.status

- `1 = counting`
- `2 = unlockable`

### cosmetic_items.rarity

- `1 = common`
- `2 = rare`
- `3 = epic`
- `4 = legendary`

### user_cosmetic_items.status

- `1 = in_bag`
- `2 = equipped`
- `3 = consumed`
- `4 = deleted`

### rooms.status

- `1 = active`
- `2 = closed`

---

## 11. 关键业务流程

## 11.1 游客登录初始化

首次登录时，服务端在一个事务中完成：

- 创建用户
- 建立 guest 绑定关系
- 创建默认宠物
- 初始化步数账户
- 创建初始宝箱
- 返回 token

## 11.2 步数同步

客户端上传当天系统累计步数，由服务端通过“今日总步数差值”计算入账增量。

推荐触发时机：

- App 启动
- 回前台
- 首页驻留期间定时同步
- 开箱前主动同步一次

## 11.3 宝箱生命周期

- 若用户没有当前宝箱，则创建一条倒计时宝箱
- 到达 `unlock_at` 后进入可开启状态
- 若用户未开启，则一直保留
- 开启成功后立即进入下一轮倒计时

## 11.4 开箱流程

在一个事务里完成：

- 校验宝箱状态
- 校验步数余额
- 扣除 1000 步数
- 抽取奖励配置
- 创建奖励实例
- 写开箱日志
- 刷新下一个宝箱

## 11.5 穿戴流程

- 校验实例归属当前用户
- 校验实例状态可装备
- 查询配置槽位
- 若槽位已存在旧装备，则先卸下旧装备
- 建立/更新装备关系
- 更新新实例状态为 equipped

## 11.6 合成流程

玩家主动选择 10 个实例作为材料，服务端在事务中完成：

- 锁定这 10 个实例
- 校验归属、状态、品质
- 标记 10 个实例为 consumed
- 从高一阶品质池中随机产出 1 个配置
- 创建奖励实例
- 写合成日志
- 写材料日志

## 11.7 房间流程

### 创建房间

- 用户当前不在房间中
- 创建房间
- 自动加入自己

### 加入房间

- 房间存在
- 房间未满 4 人
- 用户当前不在其他房间

### 退出房间

- 删除成员关系
- 清理用户当前房间引用
- 若房间空了则关闭房间

## 11.8 表情广播流程

- 用户进入房间后建立 WebSocket
- 用户点击自己的猫后选择表情
- 客户端发送 `emoji.send`
- 服务端校验用户在房间内且表情合法
- 广播给该房间成员

---

## 12. 关键工程原则

## 12.1 游客账号必须可恢复

- 客户端把 `guestUid` 保存在 Keychain
- 服务端把游客身份视为一种登录绑定方式，而不是临时身份

## 12.2 步数按“资产”设计

后端必须保留：

- 总获得
- 当前可用
- 总消耗

## 12.3 宝箱判定以服务端为准

- 前端只做倒计时展示
- 服务端判定是否已经可开启

## 12.4 道具采用实例化模型

这样后续更容易支持：

- 手动选材合成
- 锁定道具
- 收藏 / 标记
- 后续更复杂的道具属性

## 12.5 房间是轻社交，不做强权限管理

MVP 不做：

- 踢人
- 管理员体系
- 复杂房间权限

---

## 13. 当前 MVP 范围

### 必做

- 游客登录
- 默认猫咪
- 首页展示
- 步数同步
- 宝箱倒计时与开箱
- 装扮实例化背包
- 穿戴 / 卸下
- 手动选材合成
- 房间创建 / 加入 / 退出
- 房间内发表情

### 暂不做

- 复杂好友系统
- 房间聊天
- 排行榜
- 交易系统
- 多宠物体系
- 工会/社区

---

## 14. 后续文档规划

建议拆分为以下 Markdown 文档持续维护：

1. `总体架构设计.md`
2. `V1接口设计.md`
3. `数据库详细设计.md`
4. `iOS工程设计.md`
5. `Go服务端工程设计.md`
6. `实时通信协议设计.md`

