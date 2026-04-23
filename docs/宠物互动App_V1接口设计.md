# 宠物互动 App V1 接口设计

## 1. 文档说明

本文档定义当前版本 MVP 的 API 设计，包括：

- 统一协议约定
- 鉴权方案
- HTTP 接口
- WebSocket 消息协议
- 错误码
- 幂等与事务建议

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

#### 请求体

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

#### 返回示例

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

#### 返回示例

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
      "currentRoomId": "3001"
    }
  },
  "requestId": "req_xxx"
}
```

---

## 5. 首页与宠物接口

## 5.1 获取首页聚合数据

### `GET /api/v1/home`

用于首页一次拉取主要展示内容。

#### 返回示例

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

