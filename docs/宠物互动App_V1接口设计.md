# 宠物互动 App V1 接口设计

## 1. 文档说明

本文档定义当前版本 MVP 的 API 设计，包括：

- 统一协议约定
- 鉴权方案
- HTTP 接口
- WebSocket 消息协议
- 错误码
- 幂等与事务建议

**契约冻结策略**：

- 自 2026-04-26（Story 4.1 完成日，对应 git commit hash 见 commit message）起，§4.1（POST /auth/guest-login）/ §4.3（GET /me）/ §5.1（GET /home）三个节点 2 接口的 schema 进入**冻结**状态。
- 任何字段名 / 字段类型 / 错误码的修改都必须：
  1. 触发 iOS Epic 5 重新评审（影响 Story 5.2 / 5.5）
  2. 触发 server Epic 4 已完成 story 的回归（影响 Story 4.6 / 4.8 已落地的 handler）
  3. 在本 story 文件 + epics.md 同步标注变更原因 + 影响范围
- 节点 3+ 接口（如 §6 步数 / §7 宝箱 / §8 装扮 / §9 合成 / §10 房间 / §11 表情）的契约锚定由对应 epic 的 §X.1 story 负责（如 Story 7.1 / 11.1 / 17.1 等），不在本 story 范围内。
- Future Fields 标记的字段（`pet.equips` / `pet.equips[].renderConfig` / `room.currentRoomId` / `user.currentRoomId` 等）**不**视为契约变更，由对应节点 epic 自然激活。

---

## 2. 统一约定

## 2.1 协议

- 普通业务接口：`HTTPS + JSON`
- 实时互动：`WebSocket`

## 2.2 接口前缀

```text
/api/v1
```

## 2.3 鉴权方式

除登录接口外，统一使用：

```http
Authorization: Bearer <token>
```

## 2.4 通用响应结构

```json
{
  "code": 0,
  "message": "ok",
  "data": {},
  "requestId": "req_123456"
}
```

字段说明：

- `code`：业务状态码，`0` 表示成功
- `message`：错误提示或状态说明
- `data`：业务数据
- `requestId`：链路追踪 id

## 2.5 字段类型与编码约定

- **主键 / 外键 ID**：所有 BIGINT 主键 / 外键在 JSON 里以**字符串**形式下发（如 `"id": "1001"`），避免 JavaScript 端 `Number.MAX_SAFE_INTEGER` 精度丢失；客户端解析为 `String`，如需算术运算自行转换为 `Int64` 或等价类型。
- **时间字段**：**response 中的 datetime 类型字段**统一使用 ISO 8601 UTC 字符串（如 `"unlockAt": "2026-04-23T10:20:00Z"`），客户端按本地时区显示；**request 中的 epoch 时间戳类字段**（如 `POST /steps/sync` 的 `clientTimestamp`）保留 number 类型（毫秒整数），不做 ISO 字符串化 —— 该类字段在各接口章节单独标注。
- **可空字段**：可空字段在 JSON 中显式以 `null` 表示（如 `"currentRoomId": null`），不省略 key；客户端解析为 `Optional<T>` / `T?`。
- **占位字段（节点 2 阶段）**：节点 2 阶段尚未实装的字段（如 `pet.equips` / `room.currentRoomId` / `chest.unlockable` 等动态状态）按 "future fields" 标注，文档中会在每个接口章节末尾以 `> Future Fields (节点 X 落地)` 引用块列出。
- **字符串长度约束**：所有字符串字段的最大长度以**字符数**计（与 MySQL 表 `VARCHAR(N)` 语义一致 —— `N` 是字符数 limit，**不是**字节数；utf8mb4 编码下 1 个字符在底层存储占 1-4 字节，但长度校验按字符计数）。客户端 / server 实施长度校验时**必须**按 Unicode 字符数（如 Go `utf8.RuneCountInString` / Swift `String.count`）判断，**不**按字节数判断，否则会误拒合法的多字节输入。
- **枚举字段**：枚举字段以 `TINYINT` 数值下发（如 `petType: 1`），不下发字符串字面量；具体取值见各接口章节。

---

## 3. 错误码定义

```text
0       成功
1001    未登录 / token 无效
1002    参数错误
1003    资源不存在
1004    权限不足
1005    操作过于频繁
1006    状态不允许当前操作
1007    数据冲突
1008    幂等冲突
1009    服务繁忙

2001    游客账号不存在
2002    微信已绑定其他账号
2003    当前账号已绑定微信

3001    步数同步数据异常
3002    可用步数不足

4001    当前宝箱不存在
4002    宝箱尚未解锁
4003    宝箱开启条件不满足

5001    道具不存在
5002    道具不属于当前用户
5003    道具状态不可用
5004    装备槽位不匹配
5005    合成材料数量错误
5006    合成材料品质不一致
5007    合成目标品质不合法
5008    装扮已装备

6001    房间不存在
6002    房间已满
6003    用户已在房间中
6004    用户不在房间中
6005    房间状态异常

7001    表情不存在
7002    WebSocket 未连接
```

---

## 4. 认证与账号接口

## 4.1 游客登录

### `POST /api/v1/auth/guest-login`

用于：

- 首次创建游客账号
- 已存在游客账号自动登录

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | POST |
| Path | /api/v1/auth/guest-login |
| 认证 | **不需要**（登录接口本身免 auth 中间件，但走 rate_limit 中间件） |
| 限频 | 默认每客户端 IP 每分钟 60 次（按 Story 4.5 rate_limit 中间件全局默认值） |
| 幂等 | 幂等（同一 guestUid 重复调用 → 同一 user_id；不需要 idempotencyKey） |
| 节点 | 节点 2（Epic 4 落地，Epic 5 客户端集成） |

#### 请求体

| 字段 | 类型 | 必填 | 长度约束 | 说明 |
|---|---|---|---|---|
| `guestUid` | string | 必填 | 1 ≤ length ≤ 128 字节 | 客户端 Keychain 持久化的游客身份 UID（推荐 UUID v4 字符串）。空字符串或 > 128 字节 → 1002 参数错误 |
| `device` | object | 必填 | - | 设备信息对象，子字段见下 |
| `device.platform` | string | 必填 | enum: `"ios"` / `"android"`（节点 2 仅 `"ios"`） | 客户端平台标识 |
| `device.appVersion` | string | 必填 | 1 ≤ length ≤ 32 字节 | 客户端版本号（如 `"1.0.0"`） |
| `device.deviceModel` | string | 必填 | 1 ≤ length ≤ 64 字节 | 设备型号（如 `"iPhone15,2"`） |

JSON 示例：

```json
{
  "guestUid": "ios_keychain_unique_id",
  "device": {
    "platform": "ios",
    "appVersion": "1.0.0",
    "deviceModel": "iPhone15,2"
  }
}
```

#### 服务端行为

- 根据 `guestUid` 查找已有绑定
- 若存在则直接登录
- 若不存在则初始化用户、默认猫咪、步数账户、当前宝箱
- 详细事务流程见 Story 4.6 + 数据库设计 §8.1

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `data.token` | string | JWT token，HS256 + auth.token_secret 签名（见 Story 4.4 token util）；默认过期 7 天 |
| `data.user.id` | string | 用户主键（BIGINT 序列化为字符串，见 §2.5） |
| `data.user.nickname` | string | 自动生成的昵称 `"用户{id}"`（首次创建时由 server 写入） |
| `data.user.avatarUrl` | string | 头像 URL；首次创建为 `""`（空字符串而非 null） |
| `data.user.hasBoundWechat` | boolean | 是否已绑定微信；游客首次创建为 `false` |
| `data.pet.id` | string | 默认猫主键（BIGINT 序列化为字符串） |
| `data.pet.petType` | number | 宠物类型枚举，节点 2 固定 `1`（猫） |
| `data.pet.name` | string | 宠物名，首次创建为 `"默认小猫"` |

JSON 示例：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "token": "xxx",
    "user": {
      "id": "1001",
      "nickname": "用户1001",
      "avatarUrl": "",
      "hasBoundWechat": false
    },
    "pet": {
      "id": "2001",
      "petType": 1,
      "name": "默认小猫"
    }
  },
  "requestId": "req_xxx"
}
```

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1002 | 参数错误 | `guestUid` 缺失 / 为空 / 长度超过 128 字节；`device` 字段缺失或子字段不全；`device.platform` 不在枚举中 |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截（同 IP 每分钟 > 60 次） |
| 1009 | 服务繁忙 | 数据库异常 / 事务回滚 / 内部 panic（见 Story 4.6 事务实装） |

---

## 4.2 绑定微信

### `POST /api/v1/auth/bind-wechat`

#### 请求体

```json
{
  "wechatCode": "wx_auth_code"
}
```

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "hasBoundWechat": true
  },
  "requestId": "req_xxx"
}
```

---

## 4.3 获取当前用户信息

### `GET /api/v1/me`

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | GET |
| Path | /api/v1/me |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | 默认（按 Story 4.5 rate_limit 默认值） |
| 节点 | 节点 2（Epic 4 落地，且 `currentRoomId` **始终**返回 `null` —— 该字段保留为 schema 占位但不计划在后续节点回填真实数据；获取真实房间归属请改用 `GET /home`，详见本节 Future Fields） |

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `data.user.id` | string | 用户主键 |
| `data.user.nickname` | string | 用户昵称 |
| `data.user.avatarUrl` | string | 头像 URL，首次创建为 `""` |
| `data.user.hasBoundWechat` | boolean | 是否已绑定微信 |
| `data.user.currentRoomId` | string \| null | 当前房间主键。**始终返回 `null`** —— schema 占位字段，无后续节点回填计划（详见本节 Future Fields）；客户端必须按 `Optional<String>` 解析 |

JSON 示例（服务端始终返回 `null`；获取真实房间归属请改用 `GET /home`，详见 Future Fields）：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "user": {
      "id": "1001",
      "nickname": "用户1001",
      "avatarUrl": "",
      "hasBoundWechat": true,
      "currentRoomId": null
    }
  },
  "requestId": "req_xxx"
}
```

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截 |
| 1009 | 服务繁忙 | DB 查询失败 |

#### Future Fields

> **关于 `data.user.currentRoomId`**：本字段**保留为 schema 占位**但**不计划在后续节点回填真实数据**（planning artifacts 中无对应 story）。服务端**始终**返回 `null`，客户端**必须**按可空解析；获取真实房间归属请改用 `GET /home` 的 `data.room.currentRoomId`（节点 4 由 Story 11.10 回填真实数据）。

---

## 5. 首页与宠物接口

## 5.1 获取首页聚合数据

### `GET /api/v1/home`

用于首页一次拉取主要展示内容。

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | GET |
| Path | /api/v1/home |
| 认证 | **需要** Bearer token |
| 限频 | 默认 |
| 节点 | 节点 2 initial 版（含 user + pet + stepAccount + chest + room 占位）；后续节点 increment 填充 pet.equips / room.currentRoomId / pet.equips[].renderConfig |

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 节点 2 阶段值 | 说明 |
|---|---|---|---|
| `data.user.id` | string | 真实数据 | 用户主键 |
| `data.user.nickname` | string | 真实数据 | 用户昵称 |
| `data.user.avatarUrl` | string | `""` | 头像 URL |
| `data.pet.id` | string | 真实数据 | 默认猫主键 |
| `data.pet.petType` | number | `1` | 宠物类型枚举 |
| `data.pet.name` | string | `"默认小猫"` | 宠物名 |
| `data.pet.currentState` | number | `1`（rest） | 宠物当前状态枚举（1=rest, 2=walk, 3=run）；节点 2 阶段读 pets.current_state，初始为 `1` |
| `data.pet.equips` | array | `[]` | 装扮列表；**节点 2 阶段强制返回 `[]`**（穿戴在节点 9 由 Story 26.6 落地） |
| `data.stepAccount.totalSteps` | number | `0`（首次登录） | 累计步数 |
| `data.stepAccount.availableSteps` | number | `0`（首次登录） | 可用步数 |
| `data.stepAccount.consumedSteps` | number | `0`（首次登录） | 已消耗步数 |
| `data.chest.id` | string | 真实数据 | 当前宝箱主键 |
| `data.chest.status` | number | `1`（counting） | 宝箱状态枚举（1=counting, 2=unlockable —— 见 §7.1 status 枚举 + 数据库设计 §6.7 user_chests.status）；节点 2 阶段所有宝箱在登录初始化后均为 `1`（counting），到 `unlockAt` 后服务端**动态判定**为 `2`（unlockable）—— 见 Story 4.8 happy path 第 2 case。**节点 2 不存在 `opened` 状态**：开箱功能在节点 7 / Epic 20 才上线，届时开箱后会立即重建下一轮 chest（仍为 `counting`），故 `/home` 永远不返回 opened 状态 |
| `data.chest.unlockAt` | string (ISO 8601 UTC) | now + 10 min | 解锁时间 |
| `data.chest.openCostSteps` | number | `1000` | 开启所需步数（节点 2 固定 1000） |
| `data.chest.remainingSeconds` | number | 600 ~ 0 | 距离 unlockAt 的剩余秒数；> 0 表示 counting，≤ 0 表示已可开启 |
| `data.room.currentRoomId` | string \| null | `null` | 当前房间主键；**节点 2 阶段强制 `null`**（节点 4 由 Story 11.10 注入真实数据） |

JSON 示例（节点 2 阶段示例 — 首次登录后立即调用）：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "user": {
      "id": "1001",
      "nickname": "用户1001",
      "avatarUrl": ""
    },
    "pet": {
      "id": "2001",
      "petType": 1,
      "name": "默认小猫",
      "currentState": 1,
      "equips": []
    },
    "stepAccount": {
      "totalSteps": 0,
      "availableSteps": 0,
      "consumedSteps": 0
    },
    "chest": {
      "id": "5001",
      "status": 1,
      "unlockAt": "2026-04-23T10:20:00Z",
      "openCostSteps": 1000,
      "remainingSeconds": 600
    },
    "room": {
      "currentRoomId": null
    }
  },
  "requestId": "req_xxx"
}
```

JSON 示例（节点 9 / 节点 10 之后的真实数据，pet.equips 由 Story 26.6 填充，pet.equips[].renderConfig 由 Story 29.6 填充）：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "user": {
      "id": "1001",
      "nickname": "用户1001",
      "avatarUrl": ""
    },
    "pet": {
      "id": "2001",
      "petType": 1,
      "name": "默认小猫",
      "currentState": 2,
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
    },
    "stepAccount": {
      "totalSteps": 12560,
      "availableSteps": 840,
      "consumedSteps": 300
    },
    "chest": {
      "id": "5001",
      "status": 1,
      "unlockAt": "2026-04-23T10:20:00Z",
      "openCostSteps": 1000,
      "remainingSeconds": 253
    },
    "room": {
      "currentRoomId": "3001"
    }
  },
  "requestId": "req_xxx"
}
```

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截 |
| 1009 | 服务繁忙 | 任一聚合查询失败（按 epics.md §Story 4.8 AC：各部分 repo 错误 → 整体 1009 服务繁忙，不部分降级） |

#### Future Fields

> **Future Fields**（按节点 increment）：
> - `data.pet.equips[]`：节点 9（Epic 26）穿戴链路落地后，由 Story 26.6 把 user_pet_equips JOIN cosmetic_items 的真实数据填充。每个元素 schema 见上方"节点 9 / 节点 10 之后的真实数据"示例，含 `slot / userCosmeticItemId / cosmeticItemId / name / rarity / assetUrl`。
> - `data.pet.equips[].renderConfig`：节点 10（Epic 29）渲染配置落地后，由 Story 29.6 在每个 equips 元素上追加 `renderConfig: { offsetX, offsetY, scale, rotation, zLayer }` 子对象。具体字段在 Epic 29 落地时再展开。
> - `data.room.currentRoomId`：节点 4 起由 Story 11.10 注入真实房间 ID。

---

## 5.2 同步宠物当前展示状态

### `POST /api/v1/pets/current/state-sync`

#### 请求体

```json
{
  "state": 2
}
```

#### state 枚举

- `1 = rest`
- `2 = walk`
- `3 = run`

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "state": 2
  },
  "requestId": "req_xxx"
}
```

---

## 6. 步数接口

## 6.1 同步步数

### `POST /api/v1/steps/sync`

#### 请求体

```json
{
  "syncDate": "2026-04-23",
  "clientTotalSteps": 3580,
  "motionState": 2,
  "clientTimestamp": 1776920345000
}
```

#### motionState 枚举

- `1 = stationary_or_unknown`
- `2 = walking`
- `3 = running`

#### 服务端逻辑

- 读取当日最近一次同步记录
- 根据 `clientTotalSteps` 与最近记录计算增量
- 仅接收正增量
- 更新步数账户与同步日志

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "acceptedDeltaSteps": 120,
    "stepAccount": {
      "totalSteps": 12560,
      "availableSteps": 840,
      "consumedSteps": 300
    }
  },
  "requestId": "req_xxx"
}
```

---

## 6.2 获取步数账户

### `GET /api/v1/steps/account`

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "totalSteps": 12560,
    "availableSteps": 840,
    "consumedSteps": 300
  },
  "requestId": "req_xxx"
}
```

---

## 7. 宝箱接口

## 7.1 获取当前宝箱

### `GET /api/v1/chest/current`

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "id": "5001",
    "status": 1,
    "unlockAt": "2026-04-23T10:20:00Z",
    "openCostSteps": 1000,
    "remainingSeconds": 253
  },
  "requestId": "req_xxx"
}
```

#### status 枚举

- `1 = counting`
- `2 = unlockable`

---

## 7.2 开启宝箱

### `POST /api/v1/chest/open`

#### 请求体

```json
{
  "idempotencyKey": "open_chest_20260423_001"
}
```

#### 服务端逻辑

- 校验当前宝箱存在
- 校验宝箱已经解锁
- 校验可用步数大于等于 1000
- 扣除 1000 步数
- 抽取一个装扮配置
- 创建一条装扮实例
- 写开箱日志
- 刷新下一轮宝箱

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "reward": {
      "userCosmeticItemId": "91001",
      "cosmeticItemId": "24",
      "name": "星星围巾",
      "slot": 4,
      "rarity": 2,
      "assetUrl": "https://..."
    },
    "stepAccount": {
      "totalSteps": 12560,
      "availableSteps": 740,
      "consumedSteps": 400
    },
    "nextChest": {
      "id": "5002",
      "status": 1,
      "unlockAt": "2026-04-23T10:35:00Z",
      "openCostSteps": 1000,
      "remainingSeconds": 600
    }
  },
  "requestId": "req_xxx"
}
```

---

## 8. 装扮与背包接口

## 8.1 获取装扮配置目录

### `GET /api/v1/cosmetics/catalog`

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [
      {
        "cosmeticItemId": "12",
        "code": "hat_yellow_01",
        "name": "小黄帽",
        "slot": 1,
        "rarity": 1,
        "iconUrl": "https://...",
        "assetUrl": "https://..."
      }
    ]
  },
  "requestId": "req_xxx"
}
```

---

## 8.2 获取背包

### `GET /api/v1/cosmetics/inventory`

返回“聚合展示 + 实例列表”。

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "groups": [
      {
        "cosmeticItemId": "12",
        "name": "小黄帽",
        "slot": 1,
        "rarity": 1,
        "iconUrl": "https://...",
        "assetUrl": "https://...",
        "count": 3,
        "instances": [
          {
            "userCosmeticItemId": "90001",
            "status": 1
          },
          {
            "userCosmeticItemId": "90005",
            "status": 1
          },
          {
            "userCosmeticItemId": "90008",
            "status": 2
          }
        ]
      }
    ]
  },
  "requestId": "req_xxx"
}
```

#### 实例状态

- `1 = in_bag`
- `2 = equipped`
- `3 = consumed`

---

## 8.3 穿戴装扮

### `POST /api/v1/cosmetics/equip`

#### 请求体

```json
{
  "petId": "2001",
  "userCosmeticItemId": "90001"
}
```

#### 服务端逻辑

- 校验实例属于当前用户
- 校验实例当前可装备
- 查询配置槽位
- 若槽位已有装备，则先卸下旧装备
- 绑定到宠物对应槽位
- 更新实例状态为 equipped

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "petId": "2001",
    "equipped": {
      "slot": 1,
      "userCosmeticItemId": "90001",
      "cosmeticItemId": "12",
      "name": "小黄帽"
    }
  },
  "requestId": "req_xxx"
}
```

---

## 8.4 卸下装扮

### `POST /api/v1/cosmetics/unequip`

#### 请求体

```json
{
  "petId": "2001",
  "slot": 1
}
```

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "petId": "2001",
    "slot": 1,
    "unequipped": true
  },
  "requestId": "req_xxx"
}
```

---

## 9. 合成接口

当前规则：

- 玩家手动选择要消耗的道具实例
- 必须正好 10 个
- 必须同品质
- 不要求相同部位
- 不要求相同配置 id

## 9.1 获取合成概览

### `GET /api/v1/compose/overview`

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "rarities": [
      {
        "rarity": 1,
        "availableCount": 24,
        "canCompose": true
      },
      {
        "rarity": 2,
        "availableCount": 8,
        "canCompose": false
      },
      {
        "rarity": 3,
        "availableCount": 12,
        "canCompose": true
      },
      {
        "rarity": 4,
        "availableCount": 2,
        "canCompose": false
      }
    ]
  },
  "requestId": "req_xxx"
}
```

---

## 9.2 合成升级

### `POST /api/v1/compose/upgrade`

#### 请求体

```json
{
  "fromRarity": 1,
  "userCosmeticItemIds": [
    "90001",
    "90002",
    "90008",
    "90110",
    "90111",
    "90120",
    "90201",
    "90202",
    "90333",
    "90340"
  ],
  "idempotencyKey": "compose_20260423_001"
}
```

#### 服务端校验规则

- `userCosmeticItemIds` 长度必须为 10
- 不能有重复 id
- 这 10 个实例必须都属于当前用户
- 这 10 个实例必须都是 `in_bag`
- 这 10 个实例对应配置的品质必须都等于 `fromRarity`
- `fromRarity` 必须可升级：
  - `1 -> 2`
  - `2 -> 3`
  - `3 -> 4`
  - `4` 不允许

#### 服务端执行逻辑

- 锁定这 10 个实例
- 将 10 个实例更新为 consumed
- 从高一阶品质装扮池中随机抽取 1 个配置
- 创建 1 个新的装扮实例
- 写合成日志与材料日志

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "fromRarity": 1,
    "toRarity": 2,
    "consumedItemIds": [
      "90001",
      "90002",
      "90008",
      "90110",
      "90111",
      "90120",
      "90201",
      "90202",
      "90333",
      "90340"
    ],
    "reward": {
      "userCosmeticItemId": "99001",
      "cosmeticItemId": "61",
      "name": "月光围巾",
      "slot": 4,
      "rarity": 2,
      "assetUrl": "https://..."
    }
  },
  "requestId": "req_xxx"
}
```

---

## 10. 房间接口

## 10.1 创建房间

### `POST /api/v1/rooms`

#### 请求体

```json
{}
```

#### 服务端逻辑

- 校验当前用户不在其他房间
- 创建房间
- 自动加入自己

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "room": {
      "id": "3001",
      "creatorUserId": "1001",
      "maxMembers": 4,
      "memberCount": 1,
      "status": 1
    }
  },
  "requestId": "req_xxx"
}
```

---

## 10.2 获取当前所在房间

### `GET /api/v1/rooms/current`

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "roomId": "3001"
  },
  "requestId": "req_xxx"
}
```

未加入房间时：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "roomId": null
  },
  "requestId": "req_xxx"
}
```

---

## 10.3 获取房间详情

### `GET /api/v1/rooms/{roomId}`

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "room": {
      "id": "3001",
      "creatorUserId": "1001",
      "maxMembers": 4,
      "memberCount": 3,
      "status": 1
    },
    "members": [
      {
        "userId": "1001",
        "nickname": "A",
        "avatarUrl": "",
        "pet": {
          "petId": "2001",
          "currentState": 2,
          "equips": []
        }
      },
      {
        "userId": "1002",
        "nickname": "B",
        "avatarUrl": "",
        "pet": {
          "petId": "2002",
          "currentState": 1,
          "equips": []
        }
      }
    ]
  },
  "requestId": "req_xxx"
}
```

---

## 10.4 加入房间

### `POST /api/v1/rooms/{roomId}/join`

#### 请求体

```json
{}
```

#### 服务端逻辑

- 校验房间存在
- 校验房间未满
- 校验当前用户不在其他房间
- 加入房间

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "roomId": "3001",
    "joined": true
  },
  "requestId": "req_xxx"
}
```

---

## 10.5 退出房间

### `POST /api/v1/rooms/{roomId}/leave`

#### 请求体

```json
{}
```

#### 服务端逻辑

- 删除房间成员关系
- 清空用户 `currentRoomId`
- 若房间为空，则关闭房间

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "roomId": "3001",
    "left": true
  },
  "requestId": "req_xxx"
}
```

---

## 11. 表情接口

## 11.1 获取系统表情配置

### `GET /api/v1/emojis`

#### 返回示例

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "items": [
      {
        "code": "wave",
        "name": "挥手",
        "assetUrl": "https://...",
        "sortOrder": 1
      },
      {
        "code": "love",
        "name": "爱心",
        "assetUrl": "https://...",
        "sortOrder": 2
      }
    ]
  },
  "requestId": "req_xxx"
}
```

---

## 12. WebSocket 协议设计

## 12.1 连接地址

```text
GET /ws/rooms/{roomId}?token=xxx
```

服务端连接建立后需要校验：

- token 合法
- 用户确实在该房间中

成功后返回房间快照。

---

## 12.2 客户端 -> 服务端消息

### 发送表情

```json
{
  "type": "emoji.send",
  "requestId": "msg_001",
  "payload": {
    "emojiCode": "wave"
  }
}
```

### 心跳

```json
{
  "type": "ping",
  "requestId": "ping_001",
  "payload": {}
}
```

---

## 12.3 服务端 -> 客户端消息

### 房间快照

```json
{
  "type": "room.snapshot",
  "requestId": "",
  "payload": {
    "room": {
      "id": "3001",
      "maxMembers": 4,
      "memberCount": 2
    },
    "members": [
      {
        "userId": "1001",
        "nickname": "A",
        "pet": {
          "petId": "2001",
          "currentState": 2
        }
      }
    ]
  },
  "ts": 1776920345000
}
```

### 成员加入

```json
{
  "type": "member.joined",
  "payload": {
    "userId": "1002",
    "nickname": "B"
  },
  "ts": 1776920345000
}
```

### 成员离开

```json
{
  "type": "member.left",
  "payload": {
    "userId": "1002"
  },
  "ts": 1776920345000
}
```

### 收到表情广播

```json
{
  "type": "emoji.received",
  "payload": {
    "userId": "1002",
    "emojiCode": "wave"
  },
  "ts": 1776920345000
}
```

### 心跳响应

```json
{
  "type": "pong",
  "payload": {},
  "ts": 1776920345000
}
```

### 错误消息

```json
{
  "type": "error",
  "payload": {
    "code": 7001,
    "message": "emoji not found"
  },
  "ts": 1776920345000
}
```

---

## 13. 幂等与防重

## 13.1 需要幂等的接口

建议以下接口支持 `idempotencyKey`：

- `POST /api/v1/chest/open`
- `POST /api/v1/compose/upgrade`

## 13.2 幂等规则

同一用户、同一接口、同一 `idempotencyKey`：

- 第一次成功，后续重复请求返回第一次结果
- 第一次处理中，返回幂等冲突
- 第一次失败，可按服务端策略决定是否允许重试

## 13.3 Redis Key 建议

```text
idem:{userId}:{apiName}:{idempotencyKey}
```

---

## 14. 关键事务建议

## 14.1 开箱事务

必须放在一个事务里：

- 校验宝箱状态
- 扣 1000 步数
- 发放装扮实例
- 写开箱日志
- 刷新下一轮宝箱

## 14.2 穿戴事务

建议放在一个事务里：

- 校验实例状态
- 校验槽位
- 卸下旧装备
- 装备新实例
- 更新实例状态

## 14.3 合成事务

必须放在一个事务里：

- 锁定 10 个实例
- 校验归属、状态、品质
- 标记 consumed
- 创建奖励实例
- 写合成日志
- 写材料日志

## 14.4 加入房间事务

建议放在一个事务里：

- 校验用户不在其他房间
- 校验房间人数
- 插入成员关系
- 更新用户当前房间

---

## 15. 前端推荐调用顺序

## 15.1 App 启动

- `POST /api/v1/auth/guest-login`
- `GET /api/v1/home`

## 15.2 首页展示

- `GET /api/v1/home`
- 本地运行倒计时
- 关键时机用 `GET /api/v1/chest/current` 纠正状态

## 15.3 开箱前

推荐顺序：

1. `POST /api/v1/steps/sync`
2. `POST /api/v1/chest/open`

## 15.4 进入装扮页

- `GET /api/v1/cosmetics/inventory`

## 15.5 进入合成页

- `GET /api/v1/compose/overview`
- `GET /api/v1/cosmetics/inventory`

## 15.6 进入房间

- `GET /api/v1/rooms/{roomId}`
- 建立 WebSocket 连接

---

## 16. 当前 V1 接口清单

```text
POST   /api/v1/auth/guest-login
POST   /api/v1/auth/bind-wechat
GET    /api/v1/me

GET    /api/v1/home
POST   /api/v1/pets/current/state-sync

POST   /api/v1/steps/sync
GET    /api/v1/steps/account

GET    /api/v1/chest/current
POST   /api/v1/chest/open

GET    /api/v1/cosmetics/catalog
GET    /api/v1/cosmetics/inventory
POST   /api/v1/cosmetics/equip
POST   /api/v1/cosmetics/unequip

GET    /api/v1/compose/overview
POST   /api/v1/compose/upgrade

POST   /api/v1/rooms
GET    /api/v1/rooms/current
GET    /api/v1/rooms/{roomId}
POST   /api/v1/rooms/{roomId}/join
POST   /api/v1/rooms/{roomId}/leave

GET    /api/v1/emojis

GET    /ws/rooms/{roomId}
```

---

## 17. 后续文档建议

建议在后续继续拆分以下 Markdown 文档：

1. `数据库详细设计.md`
2. `实时通信协议设计.md`
3. `iOS工程设计.md`
4. `Go服务端工程设计.md`
5. `接口错误码规范.md`

