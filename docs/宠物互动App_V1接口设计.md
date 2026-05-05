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
- Future Fields 标记的字段（`pet.equips` / `pet.equips[].renderConfig` / `room.currentRoomId` 等）**不**视为契约变更，由对应节点 epic 自然激活。注：`user.currentRoomId`（仅出现在 `GET /me`）**不**属于 Future Fields —— 该字段是**永久 schema 占位**（始终 `null`，无后续节点回填计划），见 §4.3 Future Fields 引用块。
- 自 2026-05-02（Story 7.1 完成日，对应 git commit hash 见 commit message）起，§6.1（POST /steps/sync）/ §6.2（GET /steps/account）两个节点 3 接口的 schema 进入**冻结**状态。
- 任何字段名 / 字段类型 / 错误码 / 防作弊阈值默认值的修改都必须：
  1. 触发 iOS Epic 8 重新评审（影响 Story 8.5 步数同步触发器）
  2. 触发 server Epic 7 已完成 story 的回归（影响 Story 7.3 / 7.4 已落地的 handler）
  3. 在本 story 文件 + epics.md 同步标注变更原因 + 影响范围
- `steps.single_sync_cap` / `steps.daily_cap` 两个配置 key 的**默认值**（5000 / 50000）属契约一部分：**prod 部署必须使用默认值**（5000 / 50000），不允许通过配置文件覆盖 —— 否则不同 prod 实例会在不同阈值上 truncate / 返 3001，重新引入本 story 想消除的客户端/服务端契约漂移；**dev / test 环境**可通过配置文件覆盖默认值（仅用于单测 / 调试 / fixture），**不**视为契约变更（这些环境不对外提供 prod 体验，跨端契约一致性不受影响）；**修改默认值本身**视为契约变更走完整冻结流程。
- 自 2026-05-05（Story 10.1 完成日，对应 git commit hash 见 commit message）起，§12.1（连接地址 + close code 表 + 服务端校验顺序）/ §12.2（客户端 → 服务端通用消息信封 + `ping`）/ §12.3（服务端 → 客户端通用消息信封 + `room.snapshot` + `pong` + `error`）三个节点 4 WS 协议骨架章节进入**冻结**状态。
- 任何字段名 / 字段类型 / close code / 信封字段（`type` / `requestId` / `payload` / `ts`）的修改都必须：
  1. 触发 iOS Epic 12 重新评审（影响 Story 12.2 ~ 12.6 所有 WS 集成 story）
  2. 触发 server Epic 10 已完成 story 的回归（影响 Story 10.3 / 10.4 / 10.7 已落地的 gateway / heartbeat / snapshot framework）
  3. 触发后续业务 Epic 11 / 14 / 17 的契约 story（11.1 / 14.1 / 17.1）回归（业务消息基于本骨架扩展）
  4. 在本 story 文件 + epics.md 同步标注变更原因 + 影响范围
- `ws.heartbeat_timeout_sec`（默认 60s）/ `ws.max_message_size_bytes`（默认 16 KB）两个配置 key 的**默认值**属契约一部分：**prod 部署必须使用默认值**，不允许通过配置文件覆盖 —— 否则不同 prod 实例会在不同心跳超时 / message 大小上限上踢人，重新引入跨端契约漂移；**dev / test 环境**可通过配置文件覆盖默认值（仅用于单测 / 调试 / fixture），**不**视为契约变更（这些环境不对外提供 prod 体验，跨端契约一致性不受影响）；**修改默认值本身**视为契约变更走完整冻结流程。
- WS 业务消息（`member.joined` / `member.left` / `pet.state.changed` / `emoji.send` / `emoji.received` 等）的字段层契约**不**在节点 4 协议骨架冻结范围内，由对应 Epic 的 §X.1 story（Story 11.1 / 14.1 / 17.1）独立锚定 + 独立冻结 —— 即"协议骨架在 Epic 10 冻结，业务消息按 epic 顺序逐步冻结"。

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
| `guestUid` | string | 必填 | 1 ≤ length ≤ 128 字符 | 客户端 Keychain 持久化的游客身份 UID（推荐 UUID v4 字符串）。空字符串或 > 128 字符 → 1002 参数错误 |
| `device` | object | 必填 | - | 设备信息对象，子字段见下 |
| `device.platform` | string | 必填 | enum: `"ios"` / `"android"`（节点 2 仅 `"ios"`） | 客户端平台标识 |
| `device.appVersion` | string | 必填 | 1 ≤ length ≤ 32 字符 | 客户端版本号（如 `"1.0.0"`） |
| `device.deviceModel` | string | 必填 | 1 ≤ length ≤ 64 字符 | 设备型号（如 `"iPhone15,2"`） |

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
| 1002 | 参数错误 | `guestUid` 缺失 / 为空 / 长度超过 128 字符；`device` 字段缺失或子字段不全；`device.platform` 不在枚举中 |
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
| `data.pet` | object \| null | 真实数据 | 默认猫信息容器；用户**无默认 pet（理论不该发生，但 Story 4.8 edge case 强制覆盖）时返回 `null`**。客户端必须按可空对象解析（iOS 端 `Optional<PetDTO>` / Go 端 `*PetDTO`），不得假设 pet 永远非空。下方 `data.pet.*` 子字段**仅当 `data.pet ≠ null` 时存在** |
| `data.pet.id` | string | 真实数据 | 默认猫主键（仅当 `data.pet ≠ null`） |
| `data.pet.petType` | number | `1` | 宠物类型枚举（仅当 `data.pet ≠ null`） |
| `data.pet.name` | string | `"默认小猫"` | 宠物名（仅当 `data.pet ≠ null`） |
| `data.pet.currentState` | number | `1`（rest） | 宠物当前状态枚举（1=rest, 2=walk, 3=run）；节点 2 阶段读 pets.current_state，初始为 `1`（仅当 `data.pet ≠ null`） |
| `data.pet.equips` | array | `[]` | 装扮列表；**节点 2 阶段强制返回 `[]`**（穿戴在节点 9 由 Story 26.6 落地）（仅当 `data.pet ≠ null`） |
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

JSON 示例（节点 9 之后的真实数据，pet.equips 由 Story 26.6 填充；节点 10 起 pet.equips[] 还会带 renderConfig 子对象，本示例**不**展示 —— 待 Story 29.6 落地时单独补充节点 10 版本示例）：

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
> - `data.pet.equips[]`：节点 9（Epic 26）穿戴链路落地后，由 Story 26.6 把 user_pet_equips JOIN cosmetic_items 的真实数据填充。每个元素 schema 见上方"节点 9 之后的真实数据"示例，含 `slot / userCosmeticItemId / cosmeticItemId / name / rarity / assetUrl`。
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

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | POST |
| Path | /api/v1/steps/sync |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | 默认（按 Story 4.5 rate_limit 默认值 60 次/分） |
| 幂等 | **非幂等**（每次调用都会写一条 sync_log；但同一 clientTotalSteps 重复同步 → delta = 0 + 仍写日志） |
| 节点 | 节点 3（Epic 7 落地，Epic 8 客户端集成） |

#### 请求体

| 字段 | 类型 | 必填 | 范围/约束 | 说明 |
|---|---|---|---|---|
| `syncDate` | string | 必填 | ISO 8601 date 格式 `YYYY-MM-DD`（如 `"2026-04-23"`），长度严格 10 字符；**且 ∈ [server today - 2 days, server today + 2 days]**（跨时区合理容忍窗口；超出 → 1002） | 客户端按**本机时区**算出的"今天"。**server 直接采用不二次转换**（GAP E：避免跨时区漂移）；同时 **server 校验 syncDate ∈ [server today - 2 days, server today + 2 days] 范围内**（覆盖极端 PST↔JST 17h 时差 + 客户端时钟轻微漂移；防止恶意客户端旋转日期重复入账绕过 daily_cap，Story 7.3 review r7 [P1] anti-cheat）；超出此范围 → 1002 参数错误 |
| `clientTotalSteps` | number (int) | 必填 | `value ≥ 0`（int32 自然上限即可，**不**设业务硬上限；server 不对总值本身做防作弊判断，所有反作弊只针对 delta，见下方"防作弊阈值"） | 客户端读取到的"当天系统累计步数"（如 HealthKit 累计当日 steps）。**不**是增量；增量由 server 按上次同步记录差值计算 |
| `motionState` | number (int) | 必填 | enum: `1` / `2` / `3` | 同步时客户端活动状态。`1 = stationary_or_unknown`, `2 = walking`, `3 = running`（与下方 motionState 枚举一致） |
| `clientTimestamp` | number (int64) | 必填 | Unix 毫秒时间戳，> 0 | 客户端调用接口时的本机时间（毫秒）。仅写 sync_log 审计用，**不**参与差值计算逻辑 |

JSON 示例：

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

1. **认证 & 限频**：auth 中间件校验 Bearer token；rate_limit 中间件按默认配置限频
2. **参数校验**：`syncDate` 格式 + `clientTotalSteps` ≥ 0 + `motionState` ∈ {1,2,3} + `clientTimestamp` > 0；任何不通过 → 返回 1002 参数错误
3. **差值计算**（Story 7.3 service 层实装）：
   - 查 `user_step_sync_logs WHERE user_id=? AND sync_date=?` 取最近一条
   - 若**无**最近记录（首次同步） → `delta = clientTotalSteps`
   - 若**有**最近记录 → `delta = max(0, clientTotalSteps - lastClientTotalSteps)`
   - 倒退场景（clientTotalSteps < lastClientTotalSteps）→ delta = 0，仍**写日志**
   - 跨自然日（sync_date 不同） → 按新一天起算，**不**读上一天的 lastTotal
4. **防作弊阈值**（Story 7.3 增量 AC GAP K 修补；server 权威实施，**iOS 端不加限制**）：
   - **单次截断**：若 `delta > 5000`（配置 `steps.single_sync_cap`） → delta 截断为 5000 + log warning（**不**返回错误，避免误伤真实跑步用户）；接口仍返回 200 OK + `acceptedDeltaSteps = 5000`
   - **当日封顶**：若同 sync_date 当日 `accepted_delta_steps` 历史累计 + 本次截断后 delta **将超过** 50000（配置 `steps.daily_cap`，即 `prevAccepted + curDelta > 50000`） → 当次 delta 强制 = 0 + log warning + **返回 3001 步数同步数据异常**（注：判断条件是"本次入账后是否越界"，**不**是"已达上限"，否则会放过最后一次跨界 sync，例如 prev=49000 + cur=4000 应被拒）
   - 阈值通过配置文件可调；本 story 文档侧只锚定**默认值** 5000 / 50000；具体配置 key 见 Story 7.3 实装
5. **事务**（一个 MySQL 事务内完成，对应数据库设计 §8.2）：
   - UPDATE `user_step_accounts` SET `total_steps += delta, available_steps += delta, version += 1` WHERE user_id=?
   - INSERT `user_step_sync_logs` (user_id, sync_date, client_total_steps, accepted_delta_steps=delta, motion_state, source=1, client_ts=clientTimestamp)
   - 注：`source=1` 代表客户端正常上报；dev grant 走 source=2（admin_grant），见数据库设计 §6.6 + Story 7.5
   - 任一步失败 → 整体回滚，返回 1009 服务繁忙
6. **响应**：返回 `acceptedDeltaSteps`（实际入账增量，可能因截断 / 封顶 ≠ 客户端预期）+ 最新 `stepAccount` 三字段（见下方响应体）

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `data.acceptedDeltaSteps` | number (int) | 实际入账的增量步数（**可能 < clientTotalSteps - lastClientTotalSteps**，因截断 / 封顶 / 倒退场景）；客户端**不**应用此值反推自己的步数（server 权威），仅做日志展示 |
| `data.stepAccount.totalSteps` | number (int64) | 用户累计步数（更新后） |
| `data.stepAccount.availableSteps` | number (int64) | 用户可用步数（更新后） |
| `data.stepAccount.consumedSteps` | number (int64) | 用户已消耗步数（本接口不修改此值，仅返回当前值） |

JSON 示例：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "acceptedDeltaSteps": 120,
    "stepAccount": {
      "totalSteps": 1140,
      "availableSteps": 840,
      "consumedSteps": 300
    }
  },
  "requestId": "req_xxx"
}
```

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截（无 Bearer token / token 过期 / token 解析失败） |
| 1002 | 参数错误 | `syncDate` 格式不符 `YYYY-MM-DD` / `syncDate` 越出 server today ± 2 天容忍窗口（review r7 anti-cheat）/ `clientTotalSteps` 缺失或为负数 / `motionState` 不在 {1,2,3} / `clientTimestamp` ≤ 0 / 任一字段缺失 |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截（**已认证路由**按 user_id 限频，每用户每分钟 > 60 次；按 Story 4.5 默认值，配置可调） |
| 3001 | 步数同步数据异常 | 本次同步入账后当日 `accepted_delta_steps` 累计**将超过** 50000 上限（即 `prevAccepted + curDelta > 50000`），本次 delta 被强制 = 0（防作弊封顶；GAP K 修补） |
| 1009 | 服务繁忙 | DB 异常 / 事务回滚 / 内部 panic（见 Story 7.3 service 实装） |

**关键约束**：

- 错误码 3001 与 1002 的语义差异**必须**明确：1002 是**参数本身**不合法（客户端错），3001 是**参数合法但触发服务端业务限制**（防作弊封顶，本次 delta 被强制 = 0）；客户端处理策略不同（1002 应停止重试并修正参数；3001 应静默接受本次返回 + UX 提示"今日步数已达上限"）。**注意**：3001**不是粘性错误码** —— 当日只要 `prevAccepted + curDelta > 50000` 触发条件成立才返 3001；若客户端后续 sync 是倒退（`clientTotalSteps < lastClientTotalSteps`）或重复（`clientTotalSteps == lastClientTotalSteps`），按"差值计算"步骤 delta 自然 = 0，**仍返 code = 0**，不会再次触发 3001。客户端**不应**假设当日首次 3001 后所有 sync 都失败而停止上报（仍需上报维持 sync_log 审计）
- 单次截断（`delta > 5000`）**不**返回错误，仅 log warning + 截断；客户端从 `acceptedDeltaSteps` 字段感知实际入账值，**不**通过错误码感知截断（avoid false negative：真实跑步用户单次同步可能合法超 5000，体验上不应被错误码打断）

---

## 6.2 获取步数账户

### `GET /api/v1/steps/account`

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | GET |
| Path | /api/v1/steps/account |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | **已认证路由**按 `user_id` 每分钟 60 次（按 Story 4.5 默认值，配置可调；与 §6.1 `POST /steps/sync` 同语义） |
| 幂等 | 幂等（纯查询） |
| 节点 | 节点 3（Epic 7 落地） |

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 说明 |
|---|---|---|
| `data.totalSteps` | number (int64) | 用户累计步数（来自 `user_step_accounts.total_steps`） |
| `data.availableSteps` | number (int64) | 用户可用步数（来自 `user_step_accounts.available_steps`） |
| `data.consumedSteps` | number (int64) | 用户已消耗步数（来自 `user_step_accounts.consumed_steps`） |

> **schema 嵌套差异**：本接口 `data` 字段直接是三档值对象，**不**包一层 `stepAccount`（区别于 §6.1 `POST /steps/sync` 响应的 `data.stepAccount.totalSteps` 嵌套结构）。两端不同设计的原因：§6.1 是"动作型"接口（响应需附带本次动作影响的"账户最新态"，故聚合在 `stepAccount` 子对象下避免与 `acceptedDeltaSteps` 等动作返回值平铺混淆）；§6.2 是"纯读型"接口（响应**只**含账户三字段，无嵌套必要）。客户端 Codable 解析时**必须**区分 `StepsSyncResponse.data.stepAccount.*` 与 `StepsAccountResponse.data.*` 两种结构。

JSON 示例：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "totalSteps": 1140,
    "availableSteps": 840,
    "consumedSteps": 300
  },
  "requestId": "req_xxx"
}
```

#### 服务端行为

1. **认证 & 限频**：auth 中间件校验 Bearer token；rate_limit 中间件按 `user_id` 限频（每用户每分钟 60 次，按 Story 4.5 默认值，配置可调；与 §6.1 `POST /steps/sync` 同 scope —— 已认证业务路由统一按 `user_id` 而非 IP）
2. **查询**：SELECT `total_steps, available_steps, consumed_steps` FROM `user_step_accounts` WHERE `user_id` = ? (token 解析的用户)
3. **响应**：返回三档值；理论上**所有**已登录用户都已在游客登录初始化事务（数据库设计 §8.1 + Story 4.6）创建 step_account 行，故**正常情况**下不会触发 1003
4. **edge case**：如查询返回 0 行（理论不该发生，但作为兜底）→ 返回 1003 资源不存在

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截 |
| 1003 | 资源不存在 | step_account 行不存在（理论不该发生 —— 登录时由 Story 4.6 五表事务初始化；触发即数据 invariant 已损坏，应该 log error） |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截（**已认证路由**按 `user_id` 限频，每用户每分钟 > 60 次；按 Story 4.5 默认值，配置可调） |
| 1009 | 服务繁忙 | DB 查询失败 |

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

#### 连接元信息

| 字段 | 值 |
|---|---|
| Protocol | WebSocket（生产 wss / 本地 dev ws） |
| Path | /ws/rooms/{roomId} |
| Query | `token`（必填，URL-encoded Bearer token，复用 §2.3 `Authorization: Bearer <token>` 的 token util，详见 Story 4.4） |
| 鉴权 | **需要**（连接握手时校验 token + 用户在该房间） |
| 限频 | **不**走 HTTP rate_limit（Story 4.5 中间件不挂在 WS 路由），但 Session 创建后受限于 Story 10.4 心跳超时（60s 静默自动清理） |
| 节点 | 节点 4（Epic 10 落地骨架，Epic 11 / 12 业务集成） |

连接 URL 模板：

```text
{ws_scheme}://{host}/ws/rooms/{roomId}?token={url_encoded_token}
```

- `ws_scheme`：生产环境 **`wss`**（TLS）；dev / test 环境 **`ws`**（明文，仅本地）
- `host`：与 HTTP 接口同 host（不分离 WS 域名，简化 Info.plist / CORS 配置）
- `roomId`：路径参数，必须是用户当前所在房间的 ID（client 从 `GET /home.room.currentRoomId` 拿 —— **不要**使用 `GET /me.user.currentRoomId`，该字段是永久 `null` schema 占位，详见 §4.3 Future Fields 引用块；当前节点 4 阶段也可以用 client 已知的 roomId，如刚 `POST /rooms/{roomId}/join` 成功后立即建连）
- `token`：query 参数，URL-encoded（必须 url-encode，避免 token 中 `+` `/` `=` 等字符影响 query 解析）；token 来自 §2.3 Bearer token 同源（`POST /auth/guest-login` 返回的 `token`）

兼容性 / 简化形式（保留旧 README 中的写法，等价于上方完整模板）：

```text
GET /ws/rooms/{roomId}?token=xxx
```

> 兼容性提示：上述裸 URL 与"完整 URL 模板"小节是同一契约的两种书写，二者并存不冲突；以"完整 URL 模板"+ 字段说明为字段层锚定权威。

服务端连接建立后需要校验：

- token 合法（解码 + 签名校验 + 未过期）
- 用户确实在该房间中（查 `room_members` 表，节点 4 阶段 Story 10.3 实装查询路径）

成功后返回房间快照。

#### 握手成功流程

成功握手后服务端必须**主动**推送：

1. **第一条**：`room.snapshot`（payload schema 见 §12.3，节点 4 阶段为 placeholder：room 三字段 + members 空数组）

> 节点 4 阶段服务端 → 客户端**只**会主动发送 `room.snapshot` / `pong` / `error` 三种消息（与本节末 §12.3 业务消息延后锚定块 + §1 节点 4 协议骨架冻结声明一致）；`member.joined` 等业务广播消息的字段层契约与"是否在握手时广播"的语义由 Story 11.1（Epic 11）锚定，**不**在本 story 范围内 —— 即节点 4 阶段服务端**不**在握手时广播 `member.joined`。

服务端不要求 client 在握手后发送任何"我准备好了"的初始化消息，握手成功 = 服务端可以推快照。

时序图见 `docs/宠物互动App_时序图与核心业务流程设计.md` §13.2（本 story **不**重画时序图，只引用）。

#### close code 表

握手 / 连接期间任一校验失败，服务端必须主动 close 连接，并使用以下 close code：

| close code | 由谁产生 | 触发条件 | 服务端是否带 reason | 客户端推荐处理 |
|---|---|---|---|---|
| 4001 | server 主动 close | token 缺失 / 解码失败 / 签名不匹配 / token 已过期 | 是（reason = `"invalid token"` / `"token expired"`） | 触发"无效 token 静默重新登录"流程（参考 iOS Story 5.4）：清 keychain 旧 token → guest-login 拿新 token → 重连；**不**自动无限重试 |
| 4003 | server 主动 close | token 合法但 user 不在该 roomId 的 `room_members` 表中 | 是（reason = `"user not in room"`） | **不**自动重连（业务级拒绝，重试无意义）；UX 提示用户"未加入该房间"，回退到主界面 |
| 4002 | server 主动 close | roomId 路径参数格式错（非数字 / 缺失） | 是（reason = `"invalid roomId"`） | **不**自动重连；视为客户端实装 bug，记 log 后回退 |
| 4004 | server 主动 close | room 不存在（`rooms` 表无该 ID 或已 archived） | 是（reason = `"room not found"`） | **不**自动重连；UX 提示"房间不存在"，回退到主界面 |
| 4005 | server 主动 close | 心跳超时（60 秒未收到任何 client 消息含 ping，由 Story 10.4 心跳框架触发；详见 §12.2 `ping` 小节） | 是（reason = `"heartbeat timeout"`） | **应**自动重连（指数退避；视为 transient network failure，与 1006 / 1011 同等对待 —— 不视为业务级拒绝，因为底层多半是网络抖动 / 客户端切后台超时） |
| 1000 | server 或 client 任一方主动 close | 服务端 / 客户端正常关闭 | 否 | 客户端主动 close 不重连；服务端主动 close（如 graceful shutdown）客户端可选重连 |
| 1001 | server 或 client 任一方主动 close | going away（服务端重启 / 客户端切后台） | 否 | 服务端重启 → 客户端**应**自动重连（指数退避，参考 iOS Story 12.5）；客户端切后台 → 客户端自身决定 |
| 1006 | **仅 client 侧观测**（不可被任一端 emit） | 异常断开（无 close frame，TCP 中断 / 网络抖动）—— RFC 6455 §7.1.5 规定 1006 为 reserved 状态码，**禁止**出现在 close frame 内；客户端 WebSocket runtime 在底层 TCP 断开且未收到 close frame 时本地合成该 code 通知上层 | 不适用（无 close frame，因此服务端**不**emit 该 code） | 客户端**应**自动重连（指数退避） |
| 1008 | server 主动 close | 客户端违反协议策略 —— 节点 4 阶段唯一触发条件：单条消息 frame 超过 `ws.max_message_size_bytes`（默认 16 KB） | 是（reason = `"message too large"`） | **不**自动重连（视为客户端实装 bug，重连仍会被 close）；记 log error 后回退 |
| 1011 | server 主动 close | 服务端内部错误（panic / 不可恢复异常） | 是（reason 应包含简短错误提示但**不**泄漏 stack trace） | 客户端**应**自动重连（指数退避，但限制最大重试次数避免雪崩） |

**关键约束**：

- 4001 / 4002 / 4003 / 4004 是**业务级**拒绝（4xxx 段是应用自定义；WebSocket RFC 6455 规定 4000-4999 为应用保留段），重试无意义，客户端**不**应自动重连；客户端应展示明确 UX 提示并回退
- **4005 是 4xxx 段中的例外**：虽然位于 4xxx 应用自定义段，但语义是 transient network failure（心跳超时多半是网络抖动 / 客户端切后台），不是业务级拒绝；客户端**应**自动重连，与 1006 / 1011 同等对待（指数退避 + 最大重试次数限制）
- 1000 / 1001 / 1008 / 1011 是**协议 / 网络**级断开（1xxx 段是 RFC 标准段，可由服务端主动 emit），客户端**应**自动重连（除 1000 主动关闭、1008 客户端 bug 不重连）
- **1006 例外**：1006 是 RFC 6455 §7.1.5 reserved status code，**MUST NOT** be set as a status code in a Close control frame；服务端实装层（Story 10.3 / 12.5）**禁止**写 1006 到 close frame，必须由客户端 WebSocket runtime 在 TCP 异常断开时本地合成；本表保留 1006 行是为了完整描述客户端侧重连决策，**不**意味着服务端会主动 emit
- **不使用 RFC close code 1009**：RFC 6455 §7.4.1 定义 1009 为 "Message Too Big" 的标准 close code，但本协议**禁止**使用，原因是 §3 全局错误码表已将 `1009` 用于应用层 `服务繁忙`（在 §12.3 `error.payload.code` 中出现），同一数值出现在 close frame 和 application error frame 会让客户端无法仅凭数字区分 transport-level fatal 与 application-level transient（前者要 close + 不重连，后者要忽略 + 保连接）。"消息超大"场景统一改用 1008（policy violation），保留语义清晰
- 4001 触发时 server **不**写 log error（这是常态，token 过期是正常业务），写 log info；4003 / 4004 写 log warn（疑似客户端实装 bug 或数据不一致）；4005 写 log info（这是常态，心跳超时多半是网络抖动 / 切后台；写 warn 会让正常网络抖动场景下日志噪声暴涨）；1008 写 log error（必排查，疑似客户端 bug 或恶意流量）；1011 写 log error（必排查）

#### 服务端校验顺序

握手期间服务端必须按以下顺序校验，任一步失败立即 close 并使用对应 close code：

1. **解析 query**：缺 `token` 参数 → close 4001（reason = `"missing token"`）
2. **路径参数校验**：`roomId` 非数字 / 缺失 → close 4002
3. **token 校验**（不查 DB，仅本地校验签名 + 过期，复用 Story 4.4 token util）：失败 → close 4001
4. **room 存在性校验**（查 `rooms` WHERE `id = ?`）：失败 → close 4004
5. **用户房间归属校验**（查 `room_members` WHERE `user_id = ? AND room_id = ?`）：失败 → close 4003
6. **内部错误**（panic / DB 异常等）：close 1011

校验通过后：

- 创建 Session 对象（详见 Go 项目结构 §9.1）
- 注册到 SessionManager
- 启动读 / 写 goroutine
- 推送 `room.snapshot`（§12.3 schema）
- 在 Redis presence 记录在线（详见 Story 10.6）

**注意**：第 5 步的 DB 查询是热点路径（每次连接 / 重连都查一次），Story 10.3 / 10.6 实装时**应**将 `room_members` 在线态缓存到 Redis presence（避免每次连接都查 MySQL）；但**协议层面**第 5 步仍是契约一部分（client 不可见，但 server 必须保证语义一致 —— Redis 缓存与 MySQL 一致性由 Story 10.6 + Story 11.4 加入房间事务保证）。

---

## 12.2 客户端 -> 服务端消息

#### 通用消息信封（client → server）

所有客户端 → 服务端 WS 消息共享同一信封结构：

| 字段 | 类型 | 必填 | 范围/约束 | 说明 |
|---|---|---|---|---|
| `type` | string | 必填 | 1 ≤ length ≤ 64 字符；只允许 `[a-z0-9.]` | 消息类型，全小写点分（如 `"ping"` / `"emoji.send"`）；服务端按 `type` 路由到对应 handler |
| `requestId` | string | 选填 | 0 ≤ length ≤ 64 字符；客户端定义格式（推荐 `<short-prefix>_<timestamp_ms>` 或 UUID） | 客户端生成的请求 ID，用于追踪 / 日志关联；服务端**响应**当前请求时**应**带回同一 `requestId`（如 `pong` 回带 `ping` 的 `requestId`）；**广播消息**（如 `emoji.received`）的 `requestId` 由服务端置空字符串 `""` |
| `payload` | object | 必填 | 任意 JSON 对象；具体 schema 由 `type` 决定 | 业务负载；空 payload 也必须**显式**写 `"payload": {}`，**不**省略 key |

JSON 通用骨架：

```json
{
  "type": "<message_type>",
  "requestId": "<optional_client_id>",
  "payload": { "...": "..." }
}
```

**关键约束**：

- 信封**不**带 `ts` 字段（client → server 方向不需要 client 时间戳；如某业务消息需要 client 时间戳，由该业务消息 payload 自带，如 `emoji.send.payload.clientTs`，详见 Story 17.1）
- `requestId` **不**进 server 持久化日志的关键字段，仅用于本次连接 session 内的 request-response 配对（避免无意义的 cross-session 追踪）
- `type` 字段值大小写敏感：服务端 strict match；客户端**必须**按文档锚定的字面量发送（如 `"ping"` 不是 `"PING"` 或 `"Ping"`）
- 消息 frame 必须是 WebSocket text frame（opcode 0x1）+ UTF-8 JSON；**不**使用 binary frame（opcode 0x2）
- 单条消息 frame **大小上限 16 KB**（默认值；配置 `ws.max_message_size_bytes`，**prod 必须使用默认值不可覆盖**，dev/test 可覆盖）；超限服务端 close **1008**（policy violation，reason = `"message too large"`） + 不重连（客户端实装 bug）。**不使用 RFC 1009**：本协议 §3 全局错误码表已将 `1009` 用于应用层 `服务繁忙`（在 §12.3 `error.payload.code` 中出现），为避免 close frame code 与 application error code 数字冲突，"消息超大"统一走 1008；详见 §12.1 close code 表关键约束

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

> **业务消息延后锚定**：上文 `### 发送表情`（`emoji.send`）为节点 4 之前的协议草稿示例，**不**在节点 4 / Epic 10 范围内。该消息字段层契约的正式锚定由 **Story 17.1**（Epic 17 表情广播契约）负责，节点 6 落地。
>
> 节点 4 阶段（Epic 10 ~ 13）客户端 → 服务端**只**有 `ping` 一种合法消息；客户端**不**应在节点 4 阶段发送 `emoji.send`（即使协议草稿示例存在），server 收到非 `ping` 消息当前阶段**忽略**（log warn + 不 close 连接，避免兼容性问题；具体行为 Story 10.3 实装时锁定）。

### ping（心跳）

**心跳间隔**：客户端**应**每 30 秒发送一次 `ping`（默认值；配置 `ws.heartbeat_interval_sec` 在 client 侧）；服务端 60 秒未收到任何消息（含 ping）→ Session 被 Story 10.4 心跳框架自动清理 + close **4005**（reason = `"heartbeat timeout"`），客户端**应**自动重连（指数退避，参考 iOS Story 12.5；视为 transient network failure，与 1006 / 1011 同等对待）。该 code 由本 story 锚定，Story 10.4 实装时**必须**使用，**不**可改用其他 code。

**字段**：本消息无业务字段，`payload` 为空对象 `{}`。

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | string | 必填 | 固定值 `"ping"` |
| `requestId` | string | 选填 | 客户端可生成（如 `"ping_<seq>"` / `"ping_<ts_ms>"`），服务端 `pong` 必须回带；省略 → server `pong.requestId = ""` |
| `payload` | object | 必填 | 固定空对象 `{}` |

JSON 示例：

```json
{
  "type": "ping",
  "requestId": "ping_001",
  "payload": {}
}
```

服务端响应：见 §12.3 `pong` 消息。

---

## 12.3 服务端 -> 客户端消息

#### 通用消息信封（server → client）

所有服务端 → 客户端 WS 消息共享同一信封结构：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | string | 必填 | 消息类型（如 `"room.snapshot"` / `"pong"` / `"error"`）；客户端按 `type` 路由解析 |
| `requestId` | string | 必填 | **响应类**消息（如 `pong`，及作为某 client 请求响应的 `error`）回带 client 请求的 `requestId`（client 未提供 → 回 `""`）；**广播 / 主动推送类**消息（如 `room.snapshot` / `member.joined` / `emoji.received`，及 server 主动产生的 `error`）固定 `""`；**不**省略 key（即使值为空）；`error` 的双重语义详见 §12.3 `error` 小节字段表 |
| `payload` | object | 必填 | 业务负载；空 payload 也必须**显式**写 `"payload": {}` |
| `ts` | number (int64) | 必填 | 服务端发送时的 Unix 毫秒时间戳，> 0；客户端用于本地排序 / 日志，**不**用于业务时序判断 |

JSON 通用骨架：

```json
{
  "type": "<message_type>",
  "requestId": "<optional_request_correlation_or_empty>",
  "payload": { "...": "..." },
  "ts": 1776920345000
}
```

**关键约束**：

- server → client 信封**多**一个 `ts` 字段（client → server 没有），用途是客户端日志关联 + UI 排序（如多条 `member.joined` 接连到达时按 `ts` 排序避免乱序）；**不**用作业务时序判断（业务时序仍由 server 单调时钟保证；client 不应基于 `ts` 比较推断"谁先发生"，因为不同 server 实例时钟可能漂移 —— 节点 4 单实例无此风险，但接口契约层面提前规避）
- `requestId` 在响应类消息中是**回带**（必须等于 client 请求的 `requestId`，包括 client 未提供时回 `""`）；在广播 / 主动推送类消息中是固定 `""`（不要让客户端误以为这是一个新 request 的 ID）
- 特例：`error` 消息是**双重语义**的 —— 当它作为某 client 请求的响应（如 `emoji.send` 失败）时按"响应类"语义回带 `requestId`；当它是 server 主动产生（如内部状态异常 / SnapshotBuilder 抛错）时按"主动推送类"语义固定 `""`。客户端解析层（Story 12.3）**应**把 `error` 与 `pong` 同等对待：`requestId != ""` → 走 request-response 配对；`requestId == ""` → 走全局错误事件总线。详见 §12.3 `error` 小节
- frame 类型 + 编码 + 大小上限 同 §12.2 通用信封（text frame / UTF-8 JSON / 16 KB 默认上限）

### 房间快照（room.snapshot）

**触发**：

- WS 握手成功后，服务端**主动**推送（必发）
- 服务端**可选**在 Session 生命周期内重复推送（如房间状态变化触发全量重发；节点 4 阶段不实装，仅 Story 10.7 placeholder 阶段在握手时发一次）

**字段**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | string | 必填 | 固定值 `"room.snapshot"` |
| `requestId` | string | 必填 | 固定 `""`（主动推送类消息） |
| `payload.room.id` | string | 必填 | 房间 ID（BIGINT 字符串化下发，遵循 §2.5 全局约定） |
| `payload.room.maxMembers` | number (int) | 必填 | 房间容量上限（节点 4 阶段固定 4，由 Story 11.3 创建房间事务写入；本接口仅返回当前值） |
| `payload.room.memberCount` | number (int) | 必填 | 当前房间在线成员数；节点 4 阶段 Story 10.7 placeholder 实现固定返回 0；Story 11.7 真实实现按 `room_members` 行数 + Redis presence 在线态计算 |
| `payload.members` | array | 必填 | 成员列表；节点 4 阶段 Story 10.7 placeholder 实现固定返回 `[]`；Story 11.7 真实实现按 `room_members` JOIN `users` JOIN `pets` 聚合 |
| `payload.members[].userId` | string | 必填 | 成员用户 ID（BIGINT 字符串化） |
| `payload.members[].nickname` | string | 必填 | 成员昵称 |
| `payload.members[].pet.petId` | string | 必填 | 成员当前宠物 ID（BIGINT 字符串化） |
| `payload.members[].pet.currentState` | number (int) | 必填 | 宠物当前状态枚举：`1 = stationary_or_unknown` / `2 = walking` / `3 = running`（与数据库设计 §6.5 motion_state 同义，复用枚举不另起）；**节点 4 阶段 Story 11.7 真实实现固定返回 1**（Epic 14 才真实驱动） |
| `ts` | number (int64) | 必填 | 服务端发送时间戳（ms） |

> **Future Fields（节点 4 阶段为占位 / 节点 5 / 9 落地）**：
>
> - `payload.members[].avatarUrl`（成员头像 URL）：Story 11.7 真实实现时启用；节点 4 阶段 placeholder 不返回该字段（不要在 §12.3 字段表添加；client 解析为 `String?` 可选字段）
> - `payload.members[].pet.equips`（成员当前装备）：Epic 26 / Story 26.6 落地后由 Story 11.7 同步扩展；节点 4 阶段不返回
> - `payload.members[].pet.equips[].renderConfig`（装备渲染配置）：Epic 29 / Story 29.6 落地后由 Story 11.7 同步扩展；节点 4 阶段不返回

JSON 示例（真实示例，Story 11.7 落地后形态）：

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

**节点 4 阶段 placeholder 示例**（Story 10.7 实装；与上述真实示例的差异是 `members: []` + `memberCount: 0`）：

```json
{
  "type": "room.snapshot",
  "requestId": "",
  "payload": {
    "room": {
      "id": "3001",
      "maxMembers": 4,
      "memberCount": 0
    },
    "members": []
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

### pong（心跳响应）

**触发**：服务端收到客户端 `ping`（§12.2 ping 小节）后**立即**回复 `pong`（同步路径，不走异步队列）。

**字段**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | string | 必填 | 固定值 `"pong"` |
| `requestId` | string | 必填 | **必须**回带 client `ping.requestId`；client 未提供 → 回 `""` |
| `payload` | object | 必填 | 固定空对象 `{}` |
| `ts` | number (int64) | 必填 | 服务端响应时间戳（ms） |

JSON 示例（与本节字段表对齐，含 `requestId`）：

```json
{
  "type": "pong",
  "requestId": "msg_001",
  "payload": {},
  "ts": 1776920345000
}
```

> 注：`requestId` 回带客户端 `ping.requestId`；客户端 `ping` 未提供时回 `""`。该示例可作为 Story 10.3（server 实装） / Story 12.3（iOS 解析层 Codable 结构与 mock fixture）的复制粘贴样本使用。

**关键约束**：

- `pong` 是同步响应（同一 ping 事件触发，server 立即回；不通过 broadcast / 业务路径）
- `pong.requestId` 必须等于 `ping.requestId`（含 `""` 情况），方便 client 测心跳 RTT
- `pong` **不**是广播（仅回给发 ping 的连接），server 实装层面与 `BroadcastToRoom`（Story 10.5）走不同路径

### error（错误消息）

**触发**：服务端处理 client 消息或主动事件时遇到业务错误，但**不**够严重到 close 连接（如表情 code 不存在、临时性资源问题等）。严重错误（如 Session 内部 panic）走 close 流程而非 error 消息（详见 §12.1 close code 表）。

**字段**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | string | 必填 | 固定值 `"error"` |
| `requestId` | string | 必填 | 如该 error 是某 client 请求的响应（如 `emoji.send` 失败），回带原 `requestId`；如是 server 主动错误（如内部状态异常），固定 `""` |
| `payload.code` | number (int) | 必填 | 错误码，复用 §3 全局错误码定义（如 `7001` 表情不存在 / `6005` 房间状态异常 / `1009` 服务繁忙） |
| `payload.message` | string | 必填 | 错误描述，可读字符串（不做国际化，与 §3 message 字段一致） |
| `ts` | number (int64) | 必填 | 服务端发送时间戳（ms） |

JSON 示例（与本节字段表对齐，含 `requestId`）：

```json
{
  "type": "error",
  "requestId": "msg_001",
  "payload": {
    "code": 7001,
    "message": "emoji not found"
  },
  "ts": 1776920345000
}
```

> 注：当 error 是某 client 请求的响应（如 `emoji.send` 失败）时，`requestId` 回带原请求 `requestId`；当 error 是 server 主动推送（如内部状态异常 / Story 10.7 SnapshotBuilder 抛错）时，`requestId` 固定 `""`。该示例可作为 Story 10.3 / Story 12.3 的复制粘贴样本使用。

**节点 4 阶段适用错误码**（仅 Epic 10 协议骨架范围内）：

| code | 触发条件（节点 4 阶段） |
|---|---|
| 1009 | 服务繁忙（Session 内部异常但不致死，如临时 Redis 慢；client 应忽略并继续） |
| 6005 | 房间状态异常（Story 10.7 SnapshotBuilder 抛 error 时 → server 主动推 error，code = 6005） |

> 业务错误码（如 `7001` 表情不存在）由对应 Epic 各自的 §X.1 锚定（如 Story 17.1）；本 story 不预先列入节点 4 阶段适用错误码表。

> **业务消息延后锚定**：上文 `### 成员加入` / `### 成员离开` / `### 收到表情广播` 三个小节为节点 4 之前的协议草稿示例，**不**在节点 4 / Epic 10 范围内。各消息字段层契约的正式锚定 epic 如下：
>
> - `member.joined` / `member.left` → **Story 11.1**（Epic 11 房间业务契约，节点 4 中段）
> - `pet.state.changed` → **Story 14.1**（Epic 14 宠物状态同步契约，节点 5）—— **注意**：现有 §12.3 草稿中**不存在** `pet.state.changed` 消息示例（仅在 epics.md 业务消息列表里出现），Story 14.1 落地时**需新增**该小节
> - `emoji.received` → **Story 17.1**（Epic 17 表情广播契约，节点 6）
>
> 节点 4 阶段（Epic 10 ~ 13）服务端 → 客户端**只**会主动发送 `room.snapshot` / `pong` / `error` 三种消息（其中 `error` 仅在节点 4 阶段适用错误码场景下出现）；客户端 Story 12.3 解析层在节点 4 阶段**应**对未识别的 `type` 值（如收到不该到达的 `emoji.received`）走"安全忽略 + log warn"路径，**不**因未识别消息 close 连接 / crash app（避免后续 epic 灰度上线时跨版本兼容问题）。

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

