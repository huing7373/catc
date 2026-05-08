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
- **server / client active message set 的冻结范围**：本 story（Story 10.1）冻结的"server → client 仅 `room.snapshot` / `pong` / `error`"与"client → server 仅 `ping`"声明（见 §12.1.3 / §12.2 / §12.3 对应注解块）**仅适用于 Epic 10 阶段**（Story 10.1 ~ 10.7）；Epic 11 / 14 / 17 起，对应 §X.1 锚定 story（Story 11.1 / 14.1 / 17.1）按各自语义把新业务消息（如 `member.joined` / `member.left` / `pet.state.changed` / `emoji.received` / `emoji.send`）合法加入对应方向的 active message set，**不**视为本 story 冻结的违反。Epic 10 之后的 server / client active message set 由对应 epic §X.1 story 各自负责，本 story 不预先锁定。
- 自 2026-05-08（Story 11.1 完成日，对应 git commit hash 见 commit message）起，§10.1（POST /rooms）/ §10.2（GET /rooms/current）/ §10.3（GET /rooms/{roomId}）/ §10.4（POST /rooms/{roomId}/join）/ §10.5（POST /rooms/{roomId}/leave）五个节点 4 房间 REST 接口的 schema + §12.3 `### 成员加入`（`member.joined`）/ `### 成员离开`（`member.left`）两个节点 4 业务 WS 消息的字段表进入**冻结**状态。
- 任何字段名 / 字段类型 / 错误码（6001 ~ 6005）触发条件 / 业务 WS 消息 payload 字段的修改都必须：
  1. 触发 iOS Epic 12 重新评审（影响 Story 12.3 / 12.4 / 12.7 所有房间 client / WS 集成 story）
  2. 触发 server Epic 11 已完成 story 的回归（影响 Story 11.3 ~ 11.10 已落地的 handler / service / 广播路径）
  3. 触发后续业务 Epic 14 / 17 的契约 story（14.1 / 17.1）回归（业务消息基于本骨架扩展）
  4. 在本 story 文件 + epics.md 同步标注变更原因 + 影响范围
- `rooms.max_members` 默认值 **4** 属契约一部分：**prod 部署必须使用默认值**，不允许通过 schema migration / 配置覆盖 —— 否则不同 prod 实例会在不同容量上拒绝 join，重新引入跨端契约漂移；**dev / test 环境**可通过 fixture 覆盖默认值（仅用于单测 / 调试 / fixture），**不**视为契约变更；**修改默认值本身**视为契约变更走完整冻结流程。
- `rooms.status` 枚举（`1 = active` / `2 = closed`）属契约一部分；新增枚举值（如 `3 = paused`）视为契约变更。
- Future Fields 标记的字段（`members[].pet.equips` 节点 9 落地 / `members[].pet.equips[].renderConfig` 节点 10 落地 / `members[].pet.currentState` 节点 5 真实驱动）**不**视为契约变更，由对应节点 epic 自然激活；具体过渡见 §10.3.5 五阶段过渡表。
- WS 业务消息（`pet.state.changed` / `emoji.send` / `emoji.received` 等）的字段层契约**不**在节点 4 房间业务冻结范围内，由对应 Epic 的 §X.1 story（Story 14.1 / 17.1）独立锚定 + 独立冻结。

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

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | POST |
| Path | /api/v1/rooms |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | 默认（按 Story 4.5 rate_limit 默认值 60 次/分；房间创建是低频操作，无需特殊限频） |
| 幂等 | **非幂等**（每次调用都创建一个新房间；用户已在房间中再调将返回 6003）；**不**接受 `idempotencyKey` |
| 节点 | 节点 4（Epic 11 落地，Epic 12 客户端集成） |

#### 请求体

空对象（`{}`）—— 当前用户身份从 `Authorization: Bearer <token>` 解析，房间属性（`max_members=4` / `status=1`）由 server 端写死，client 不传任何业务字段。

JSON 示例：

```json
{}
```

> 注：未来若引入"自定义房间名 / 私有房间标识"等扩展字段，将在新 epic 的 §X.1 story 锚定（不在本节点 4 范围内）。

#### 服务端逻辑

事务内严格按顺序执行（详见 Story 11.3 实装）：

1. 查 `users.current_room_id`，**非 null** → 立即返回 **6003 用户已在房间中**（不开事务）
2. 插入 `rooms`（`creator_user_id = 当前 user`, `status = 1`, `max_members = 4`），拿到 `room_id`
3. 插入 `room_members`（`room_id`, `user_id = 当前 user`）；如果遇到 DB UNIQUE(`user_id`) 兜底冲突（理论不会，因为步骤 1 已查过 `users.current_room_id`，但同一用户并发两次 `POST /rooms` 时可能发生 —— 两请求都通过步骤 1，赢家先插 `room_members`，输家在步骤 3 撞 UNIQUE）→ 回滚 + 返回 **6003**（与 §10.4 join 接口步骤 5 兜底语义对齐）
4. 更新 `users.current_room_id = room_id`
5. 提交事务
6. 返回 `data.room` 见下文响应体字段表

**事务边界规则**：步骤 2 ~ 4 必须在同一 MySQL 事务中（参见数据库设计 §8.6 房间事务边界）；步骤 3 撞 `room_members.UNIQUE(user_id)` 时回滚并返回 **6003**（不是 1009 —— 该路径是"用户已在某房间"的正常 race，不是服务异常，client 仅识别 6003 即可统一处理）；其他事务失败（DB 异常 / 内部 panic）→ 全部回滚 → 返回 1009 服务繁忙。

**WS 广播**：本接口**不**触发 `member.joined` 广播 —— 房间刚创建、当前用户是房间内**唯一**成员，无其他在线成员需要被通知；`member.joined` 仅在 `POST /rooms/{roomId}/join` 成功后由 Story 11.8 触发（详见 §12.3 `### 成员加入` 触发条件段）。

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `data.room.id` | string | 必填 | 房间主键（BIGINT 字符串化下发，遵循 §2.5 全局约定） |
| `data.room.creatorUserId` | string | 必填 | 创建者 user 主键（BIGINT 字符串化）；来自 `rooms.creator_user_id` |
| `data.room.maxMembers` | number (int) | 必填 | 房间容量上限；节点 4 阶段固定 `4`（来源数据库设计 §5.13 `max_members TINYINT UNSIGNED NOT NULL DEFAULT 4`） |
| `data.room.memberCount` | number (int) | 必填 | 当前成员数；本接口创建后必为 `1`（自动加入了创建者自己） |
| `data.room.status` | number (int) | 必填 | 房间状态枚举：`1 = active` / `2 = closed`（来源数据库设计 §6.12 `rooms.status`）；本接口创建后必为 `1` |

JSON 示例：

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

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截（无 Authorization 头 / token 非法 / token 过期） |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截（同 IP 每分钟 > 60 次） |
| 6003 | 用户已在房间中 | 服务端逻辑步骤 1（预检）或步骤 3（DB UNIQUE 兜底）：用户已在任意房间（含并发 race 场景） |
| 1009 | 服务繁忙 | 事务回滚 / DB 异常 / 内部 panic（**不**含 `room_members.UNIQUE(user_id)` 撞冲突 —— 那条走 6003，详见 Story 11.3 实装） |

> **注**：本接口**不**会触发 6001 / 6002 / 6004 / 6005 —— 房间在事务内才被创建，不存在"找不到 / 已满 / 状态异常"场景；6004（用户不在房间中）仅在 `POST /rooms/{roomId}/leave` 出现。

**关键约束**：

- 字段表中所有 BIGINT 主键 / 外键（`id` / `creatorUserId`）严格按 §2.5 全局约定字符串化下发
- `memberCount` 与 `maxMembers` 类型为 `number (int)`，**不**字符串化（数值字段不受 §2.5 BIGINT 字符串化规则约束 —— `memberCount` ≤ `maxMembers` ≤ 4，远低于 `Number.MAX_SAFE_INTEGER`，client `Int` / Swift `Int` 解析无精度风险）
- 6003 在两条路径下触发（步骤 1 预检 + 步骤 3 DB UNIQUE 兜底），但 client **不**应区分这两种场景（都是"用户已在某房间"，UX 处理一致）；这与 §10.4 join 接口的双路径 6003 语义对称
- 6003 预检路径不消耗事务资源；6003 兜底路径（步骤 3 撞 UNIQUE）会回滚已开事务，但**不**降级为 1009 —— race 是正常并发，不是服务异常

---

## 10.2 获取当前所在房间

### `GET /api/v1/rooms/current`

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | GET |
| Path | /api/v1/rooms/current |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | 默认（按 Story 4.5 rate_limit 默认值 60 次/分） |
| 幂等 | 幂等（GET，无副作用） |
| 节点 | 节点 4（Epic 11 落地，Epic 12 客户端集成） |

#### 请求

无请求体（GET 方法）；当前用户身份从 `Authorization: Bearer <token>` 解析。

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `data.roomId` | string \| null | 必填（值可为 null） | 当前用户所在房间主键（BIGINT 字符串化），即 `users.current_room_id` 当前值；用户不在任何房间时为 `null`（**显式** null，**不**省略 key） |

JSON 示例（用户当前在房间）：

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

JSON 示例（用户当前不在任何房间）：

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

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截 |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截 |
| 1009 | 服务繁忙 | DB 异常 / 内部 panic |

> **注**：本接口**不**会触发 6001 ~ 6005 —— 用户不在房间是合法场景（返回 `roomId: null` + code = 0），不视为业务错误。

**关键约束**：

- 与 `GET /me.user.currentRoomId` **不**等价 —— `GET /me` 中的 `currentRoomId` 是**永久 schema 占位**始终返回 `null`（详见 §4.3 Future Fields 引用块），本接口才是获取真实房间归属的权威路径
- 与 `GET /home.data.room.currentRoomId` 字段语义**等价**（两者都查 `users.current_room_id`），但 `GET /home` 是首页聚合接口（含 user / pet / stepAccount / chest / room 五段），本接口是单字段轻量查询；client 在房间页内用本接口、在首页加载用 `GET /home`（节点 4 由 Story 11.10 真实实装 `room.currentRoomId`）
- 客户端 WS 重连时**不**应优先用本接口拿 roomId（按 §12.1 客户端 roomId 来源规则：`本地持久化最近一次 WS roomId` 优先；本接口可作为冷启场景兜底，但不是首选）

---

## 10.3 获取房间详情

### `GET /api/v1/rooms/{roomId}`

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | GET |
| Path | /api/v1/rooms/{roomId} |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | 默认（按 Story 4.5 rate_limit 默认值 60 次/分） |
| 幂等 | 幂等（GET，无副作用） |
| 节点 | 节点 4（Epic 11 落地，Epic 12 客户端集成） |

#### 请求

| 参数位置 | 字段 | 类型 | 必填 | 长度/范围约束 | 说明 |
|---|---|---|---|---|---|
| Path | `roomId` | string | 必填 | 必须是 BIGINT 数字字符串（如 `"3001"`），1 ≤ length ≤ 20 字符 | 目标房间主键；server 内部转 `uint64` |

> **注**：当前节点 4 阶段**不**强制要求 `roomId` 是当前用户所在房间 —— 用户**可以**查询任意 `roomId` 的房间详情（用于"加入前预览房间"等未来场景）；后续节点若需要"私有房间禁止非成员查看"语义，由对应 epic §X.1 story 加 ACL 校验，不在本 story 范围。

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `data.room.id` | string | 必填 | 房间主键（BIGINT 字符串化） |
| `data.room.creatorUserId` | string | 必填 | 创建者 user 主键（BIGINT 字符串化） |
| `data.room.maxMembers` | number (int) | 必填 | 房间容量上限（节点 4 阶段固定 `4`） |
| `data.room.memberCount` | number (int) | 必填 | 当前成员总数；**必须严格等于** `data.members[]` 数组长度（不变量，详见本节末"不变量"小节） |
| `data.room.status` | number (int) | 必填 | 房间状态枚举：`1 = active` / `2 = closed` |
| `data.members` | array | 必填 | 房间全成员列表 = `room_members WHERE room_id = ?` 全部行 + JOIN `users` / `pets` 聚合；**不做"WS 此刻是否连接"层的过滤**（即 service 实装层**禁止**只返回 SessionManager 当前在线的 user）—— roster 反映的是 server 端"仍在房间"的状态（详见本节末"roster 语义与 WS 断线交互"小节）；节点 4 阶段**不**下发"online / offline"区分字段，client 不感知具体连接态 |
| `data.members[].userId` | string | 必填 | 成员 user 主键（BIGINT 字符串化） |
| `data.members[].nickname` | string | 必填 | 成员昵称；来自 `users.nickname`（节点 2 阶段首次创建时 server 写入 `"用户{id}"`，可被未来"修改昵称"功能覆盖） |
| `data.members[].avatarUrl` | string | 必填 | 成员头像 URL；首次创建为空字符串 `""`（不为 null）；空字符串语义 = "暂无头像"，client 渲染时降级为占位头像 |
| `data.members[].pet.petId` | string | 必填 | 成员当前宠物主键（BIGINT 字符串化）；来自 `pets.id`（每个 user 节点 2 阶段唯一 1 只默认猫，详见 Story 4.6 首次初始化事务） |
| `data.members[].pet.currentState` | number (int) | 必填 | 宠物当前状态枚举：`1 = rest` / `2 = walk` / `3 = run`（来源数据库设计 §6.4 `pets.current_state`）；**节点 4 阶段固定返回 `1`**（Epic 14 才真实驱动 motion_state） |
| `data.members[].pet.equips` | array | 必填 | 成员当前装备数组；**节点 4 阶段固定返回 `[]`**（Future Fields，详见本节末"五阶段过渡表"） |

JSON 示例（节点 4 阶段，3 成员房间）：

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
        "nickname": "用户1001",
        "avatarUrl": "",
        "pet": {
          "petId": "2001",
          "currentState": 1,
          "equips": []
        }
      },
      {
        "userId": "1002",
        "nickname": "用户1002",
        "avatarUrl": "",
        "pet": {
          "petId": "2002",
          "currentState": 1,
          "equips": []
        }
      },
      {
        "userId": "1003",
        "nickname": "用户1003",
        "avatarUrl": "",
        "pet": {
          "petId": "2003",
          "currentState": 1,
          "equips": []
        }
      }
    ]
  },
  "requestId": "req_xxx"
}
```

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截 |
| 1002 | 参数错误 | `roomId` 路径参数格式错（非数字 / 长度 > 20 字符） |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截 |
| 6001 | 房间不存在 | `rooms WHERE id = ?` 查不到记录 |
| 1009 | 服务繁忙 | DB 异常 / 内部 panic |

> **注**：本接口**不**触发 6002 / 6003 / 6004 —— 这三个错误码是房间生命周期变更（join / leave）路径上的错误，纯查询不涉及；6005（房间状态异常）在节点 4 阶段**不**用于本接口（即使 `rooms.status = 2 closed` 也允许查询，client 拿到 `status = 2` 后由 UX 决定如何展示，不视为查询失败）。

#### `data.members[]` 字段五阶段过渡表

各字段在 MVP 不同节点 / Epic 落地后的下发值演进：

| 字段 | 节点 4 placeholder（Story 10.7） | 节点 4 真实（Story 11.6 / 11.7） | 节点 5 真实（Epic 14） | 节点 9 真实（Epic 26） | 节点 10 真实（Epic 29） |
|---|---|---|---|---|---|
| `userId` | `room_members.user_id` 字符串化 | 同 placeholder | 同 placeholder | 同 placeholder | 同 placeholder |
| `nickname` | `""`（不 JOIN `users`，详见 §12.3 placeholder 说明） | `users.nickname`（GET /rooms/{roomId} 始终走真实路径；WS room.snapshot 由 Story 11.7 SnapshotBuilder 真实实装） | 同节点 4 真实 | 同节点 4 真实 | 同节点 4 真实 |
| `avatarUrl` | **不下发**（§12.3 字段表 placeholder 不含此字段） | `users.avatar_url`（首次创建为 `""`，本接口始终走真实路径） | 同节点 4 真实 | 同节点 4 真实 | 同节点 4 真实 |
| `pet.petId` | `""`（不 JOIN `pets`） | `pets.id` 字符串化 | 同节点 4 真实 | 同节点 4 真实 | 同节点 4 真实 |
| `pet.currentState` | 固定 `1` | 固定 `1` | `pets.current_state`（真实 1/2/3，由 Epic 14 状态机驱动） | 同节点 5 | 同节点 5 |
| `pet.equips` | **不下发**（§12.3 字段表 placeholder 不含此字段） | `[]`（节点 4 阶段 Story 11.6 GET /rooms/{roomId} 固定返回空数组；WS room.snapshot 同样不下发） | 同节点 4 真实 | `user_pet_equips JOIN cosmetic_items` 聚合（Story 26.6 真实回填，**不**含 `renderConfig` 子字段） | 同节点 9 + 加 `renderConfig` 子字段（Story 29.6 真实回填 `pet.equips[].renderConfig`） |

**关键解读**：

1. **GET /rooms/{roomId}（本接口）从节点 4 起就走"真实"路径**（Story 11.6 实装时 JOIN `users` / `pets`）；`pet.currentState` / `pet.equips` 仍按上表节点 4 列返回固定值，但 `nickname` / `avatarUrl` / `pet.petId` 在节点 4 真实路径下就是真实值（与 placeholder 不同）。即"REST 接口本身节点 4 真实，仅 pet 状态字段后续 epic 真实驱动"。
2. **WS `room.snapshot` placeholder（Story 10.7）/ 真实（Story 11.7）路径**与 REST 接口路径独立：placeholder 阶段允许 `nickname` / `pet.petId` 空字符串（不 JOIN），真实阶段填真实值；详见 §12.3 placeholder 字段值来源说明。
3. **client merge contract**（§12.3 末尾 client merge contract 引用块）保证：client 通过 GET /rooms/{roomId}（本接口节点 4 真实路径）拿到的真实 nickname / petId / avatarUrl，在收到 `room.snapshot` placeholder 的空字符串字段时**保留**真实值，不被空串覆盖。这条契约对本接口的节点 4 真实路径同样适用。
4. **`pet.equips` 节点 4 阶段固定 `[]`**：与 §5.1 GET /home 的 `pet.equips` 节点 4 阶段语义一致（详见 §5.1 字段表 + Future Fields）；本接口 `members[].pet.equips` 与 §5.1 单 user `pet.equips` 是两个独立字段路径，但占位语义相同。

#### 不变量（response 内部一致性）

- `data.room.memberCount` **必须严格等于** `data.members[]` 数组长度
- `data.members[]` 必须包含**全部** `room_members WHERE room_id = ?` 行；service 实装层**禁止**在 query 后做"WS 此刻是否连接"层的过滤（与 §12.3 `room.snapshot` 不变量一致：snapshot / GET /rooms 都返回当前 `room_members` 全行 —— full roster view，不是 online-only view）。**注**：roster 反映的是 server 端"仍在房间"状态，不区分"WS 当前在线 / 已断开但 row 还在"；任何 WS 断开（含心跳超时 / 客户端主动 close / TCP 1006 / app 关闭）**只**清 ephemeral 层（SessionManager + Redis presence），**不**触动 `room_members` 行 —— 只有 HTTP `POST /rooms/{roomId}/leave` 才删 row + 触发 `member.left` 广播；详见本节末"roster 语义与 WS 断线交互"小节
- `data.members[]` 顺序由 server 决定（建议按 `room_members.joined_at ASC` 稳定排序，便于 client 渲染顺序稳定）；client 解析层**不**应假设特定顺序（除"稳定"外的语义）
- `data.room.status` 与 `data.members[]` 的关系：`status = 2 closed` 时 `members[]` 必为 `[]`（因为退出房间事务在最后一人离开时同时删除最后一行 `room_members` + 更新 `rooms.status = 2`，详见 §10.5 服务端逻辑）；`status = 1 active` 时 `members[]` 必非空（房间至少有 1 个成员，否则按业务规则 status 应已变 closed）

**关键约束**：

- 字段表中所有 BIGINT 主键 / 外键（`id` / `creatorUserId` / `userId` / `petId`）严格按 §2.5 全局约定字符串化下发
- `data.members[].avatarUrl` 必须存在且为 string（可空字符串 `""`，**不**为 null）；与 §4.1 / §4.3 / §5.1 中 `user.avatarUrl` 处理一致
- `data.members[].pet.equips` 节点 4 阶段必须为 `[]`（空数组，**不**省略 key、**不**为 null）；client 解析层按 `[<EquipDTO>]` 解析，节点 4 阶段数组永远为空，节点 9 由 Epic 26 真实回填非空数组
- 节点 4 阶段**不**下发 `members[].pet.equips[].renderConfig`（Future Fields，节点 10 由 Epic 29 / Story 29.6 加；本字段表**不**列入）

#### roster 语义与 WS 断线交互

节点 4 阶段 `data.members[]` 的"成员"定义 = "**server 端仍认为在房间**的成员"（即 `room_members WHERE room_id = ?` 当前存在的行）；**不**在字段层引入"online / offline"区分。

**核心钦定（持久层 vs ephemeral 层职责分离）**：

- **持久层 = `room_members` 行 + `users.current_room_id`**：**唯一**变更路径是 HTTP `POST /api/v1/rooms/{roomId}/join`（添加行）和 HTTP `POST /api/v1/rooms/{roomId}/leave`（删除行）—— 详见 §10.4 / §10.5 服务端逻辑。任何 WS 层事件（含心跳超时、TCP 1006、客户端主动 close、app 关闭 / 切后台）**禁止**修改持久层
- **ephemeral 层 = SessionManager 内存映射 + Redis presence**：由 WS 连接生命周期管理（onRegister / onUnregister 钩子）；任何 WS 断开（含主动 close / 心跳超时 / TCP 异常）**只**清理 ephemeral 层（撤销 Session、清 presence），**不**触动持久层

→ "WS 断开 = 离开房间" **不**成立；只有 HTTP leave（或被 server 通过 close 4007 通知 client 协议层确认完成的同一路径）才是真正的"离开房间"。这条钦定是 §10.5 / §12.1 4005 retryable 语义 / §12.3 `### 成员离开` 触发条件三处协同的契约基础。

**各类 WS 断开场景的语义**：

- **WS 当前连接但暂时停留**（如 client 网络抖动、TCP 重传中）：心跳尚未超时，Session 仍活跃，`room_members` 行仍在；roster 不变
- **WS 心跳超时**（默认 60s 无 pong → server close 4005，reason = `"heartbeat timeout"`）：server 撤销 Session（onUnregister 清 ephemeral），**不**改 `room_members` / `users.current_room_id`，**不**广播 `member.left`；client 按 §12.1 close code 表 4005 行钦定 transient 语义自动重连，重连握手时 §12.1 校验顺序步骤 5（`room_members` 表查询）**通过**（行仍在），4005 reconnect 设计**有意义且自洽**
- **client 主动 close / app 关闭 / 前台切后台超过保活窗口 / TCP 异常断开（1006）**：与心跳超时同路径 —— 仅清 ephemeral 层，**不**改 `room_members` / `users.current_room_id`，**不**广播 `member.left`；用户后续若重新打开 app 或 reconnect，`room_members` 行仍在，可直接 WS 握手回到原房间
- **HTTP leave**（用户主动通过 `POST /rooms/{roomId}/leave` 请求离开）：删 `room_members` 行 + `users.current_room_id = NULL` + 广播 `member.left` + close leaver 自己的 WS Session（close code 4007）—— 详见 §10.5 服务端逻辑

**memberCount 与 reconnect 语义的 self-consistency**：`memberCount = members[].length = 当前 room_members 行数`；只有 HTTP leave 会让 `memberCount` 递减并广播 `member.left`；任何 WS 断开（含心跳超时）**不**改变 `memberCount`，roster 在 client 端保持稳定，重连后 server 推送的 `room.snapshot` 与 reconnect 前一致 —— 三层（持久层、ephemeral 层、广播事件）语义对齐，无 drift。

> **设计 rationale**：r1 review 指出"transient WS disconnect 不应整人踢出房间，否则 `含离线成员` 契约自相矛盾"；r2 修复尝试通过"心跳超时 = 删行 + 广播 member.left"路径让 roster 不区分在线 / 离线但仍正确清理僵尸用户。但 r3 review 指出该方案与 §12.1 4005 retryable 语义内在冲突 —— client 按钦定 transient 语义重连，握手时 §12.1 步骤 5 立即 fail 4003（行已删），reconnect 形同虚设。本契约最终定位（r3 锁定）：**WS 断开仅清 ephemeral，房间归属只能由 HTTP leave 改变**；transient 容错（含心跳超时窗口）由 ephemeral 层独立处理，行不删 → 4005 reconnect 自洽 → 跨文档（V1接口设计.md / 时序图设计.md）只有一种 disconnect 语义可遵循。"含离线成员"概念依然不引入（roster 字段层不区分），但保留 row 的实现机制变成"WS 断开不删 row"而非"心跳超时阈值"。

---

## 10.4 加入房间

### `POST /api/v1/rooms/{roomId}/join`

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | POST |
| Path | /api/v1/rooms/{roomId}/join |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | 默认（按 Story 4.5 rate_limit 默认值 60 次/分） |
| 幂等 | **非幂等**（同一用户重复 join 自己已在的房间 → 返回 6003；不能用 `idempotencyKey` 跳过校验） |
| 节点 | 节点 4（Epic 11 落地，Epic 12 客户端集成） |

#### 请求

| 参数位置 | 字段 | 类型 | 必填 | 长度/范围约束 | 说明 |
|---|---|---|---|---|---|
| Path | `roomId` | string | 必填 | 必须是 BIGINT 数字字符串（如 `"3001"`），1 ≤ length ≤ 20 字符 | 目标房间主键 |
| Body | - | object | 必填 | - | 空对象 `{}`；当前用户身份从 token 解析 |

JSON 示例（请求体）：

```json
{}
```

#### 服务端逻辑

事务内严格按顺序执行（详见 Story 11.4 实装）：

1. 查 `users.current_room_id`，**非 null** → 立即返回 **6003 用户已在房间中**（不开事务）；**特例**：若 `current_room_id == 当前请求的 roomId`（用户尝试加入自己已在的房间）也返回 6003 —— 用同一码统一处理"用户已在某房间"语义，client 仅需识别 6003 即可，不需要区分"已在目标房间" vs "已在其他房间"
2. 开事务，加 `SELECT ... FOR UPDATE` 行锁（避免并发 join 跑过容量校验）；查 `rooms WHERE id = ?`，**找不到** → 回滚 + 返回 **6001 房间不存在**
3. 查到的 `rooms.status != 1` → 回滚 + 返回 **6005 房间状态异常**（`status = 2 closed` 房间禁止加入）
4. 查 `room_members WHERE room_id = ?` 数量，`>= 4` → 回滚 + 返回 **6002 房间已满**
5. 插入 `room_members`（`room_id`, `user_id = 当前 user`）；如果遇到 DB UNIQUE(`user_id`) 兜底冲突（理论不会，因为步骤 1 已查过 `users.current_room_id`，但并发竞态下可能发生）→ 回滚 + 返回 **6003**
6. 更新 `users.current_room_id = roomId`
7. 提交事务
8. **事务成功提交后**触发 WS 广播 `member.joined`（payload 见 §12.3 `### 成员加入`，由 Story 11.8 实装；广播失败 fire-and-forget 仅 log，不影响 HTTP 200 响应）
9. 返回 `data.{roomId, joined: true}`

**事务边界规则**：步骤 2 ~ 6 必须在同一 MySQL 事务中（参见数据库设计 §8.7 加入房间事务边界）；步骤 8 在事务**外**触发（fire-and-forget，与 Story 10.5 BroadcastToRoom primitive 既有语义一致 —— 广播是事件通知，不参与事务原子性）。

**并发保护**：步骤 2 的 `SELECT ... FOR UPDATE` 行锁是关键 —— 4 个用户已在房间，5 个用户同时调用 join，必须只有 1 个成功（或 0 个，若房间已 closed），其他全部返回 6002（或 6005）。详见 Story 11.4 单测 / 11.9 集成测试并发场景。

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `data.roomId` | string | 必填 | 加入的房间主键（BIGINT 字符串化）；与请求 path 中 `roomId` 一致，回带方便 client 校验 |
| `data.joined` | boolean | 必填 | 固定 `true`（成功路径必返）；**不**为 `false` —— 失败时返回错误码而非 `joined: false` |

JSON 示例：

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

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截 |
| 1002 | 参数错误 | `roomId` 路径参数格式错（非数字 / 长度 > 20 字符） |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截 |
| 6001 | 房间不存在 | 服务端逻辑步骤 2：`rooms WHERE id = ?` 找不到 |
| 6002 | 房间已满 | 服务端逻辑步骤 4：`room_members` 当前数 `>= 4` |
| 6003 | 用户已在房间中 | 服务端逻辑步骤 1（预检）或步骤 5（DB UNIQUE 兜底）：用户已在任意房间（含目标房间） |
| 6005 | 房间状态异常 | 服务端逻辑步骤 3：`rooms.status != 1`（如已 closed） |
| 1009 | 服务繁忙 | 事务回滚 / DB 异常 / 内部 panic |

> **注**：本接口**不**触发 6004（用户不在房间中）—— 6004 仅用于 leave 接口。

**关键约束**：

- 错误码触发顺序严格按服务端逻辑步骤：步骤 1 → 6003（预检）；步骤 2 → 6001；步骤 3 → 6005；步骤 4 → 6002；步骤 5 → 6003（兜底）。**不**允许实装层重排顺序（如先查容量再查 status，会让"closed 房间满员"场景错误返回 6002 而非 6005）
- 6003 在两条路径下触发（步骤 1 预检 + 步骤 5 DB UNIQUE 兜底），但 client **不**应区分这两种场景（都是"用户已在某房间"，UX 处理一致）
- `data.joined` 固定 `true`：成功路径必返；失败路径返回错误码 + 不返回 `joined` 字段（按 §2.4 通用响应结构，错误时 `data` 为空对象 / 不含业务字段）

---

## 10.5 退出房间

### `POST /api/v1/rooms/{roomId}/leave`

#### 接口元信息

| 字段 | 值 |
|---|---|
| HTTP Method | POST |
| Path | /api/v1/rooms/{roomId}/leave |
| 认证 | **需要** Bearer token（auth 中间件） |
| 限频 | 默认（按 Story 4.5 rate_limit 默认值 60 次/分） |
| 幂等 | **非幂等**（同一用户重复 leave 同一房间 → 第二次返回 6004；client 应在收到 200 后清本地状态，不要重试） |
| 节点 | 节点 4（Epic 11 落地，Epic 12 客户端集成） |

#### 请求

| 参数位置 | 字段 | 类型 | 必填 | 长度/范围约束 | 说明 |
|---|---|---|---|---|---|
| Path | `roomId` | string | 必填 | 必须是 BIGINT 数字字符串，1 ≤ length ≤ 20 字符 | 目标房间主键 |
| Body | - | object | 必填 | - | 空对象 `{}` |

JSON 示例（请求体）：

```json
{}
```

#### 服务端逻辑

事务内严格按顺序执行（详见 Story 11.5 实装）：

1. 查 `users.current_room_id`，**与请求的 `roomId` 不一致**（含 `current_room_id` 为 null）→ 立即返回 **6004 用户不在房间中**（不开事务）
2. 开事务；删除 `room_members WHERE room_id = ? AND user_id = ?`（删除当前 user 的成员行）；**检查 `RowsAffected`**：若 `== 0`（同一用户两次并发 leave 都通过步骤 1，赢家已先删该行，输家此处 0 行受影响）→ 回滚 + 返回 **6004 用户不在房间中**（与步骤 1 同语义统一）；该兜底是关键并发护栏，否则输家会继续走完步骤 3 ~ 7 产生重复广播 + 重复 close 4007
3. 更新 `users.current_room_id = NULL`
4. 查 `room_members WHERE room_id = ?` 剩余数量；若 `== 0`（最后一人离开）→ 更新 `rooms.status = 2 closed`
5. 提交事务
6. **事务成功提交后**触发 WS 广播 `member.left`（payload 见 §12.3 `### 成员离开`，由 Story 11.8 实装；广播失败 fire-and-forget 仅 log，不影响 HTTP 200 响应）；**特例**：若步骤 4 触发了 closed 转换（房间已无在线广播对象）—— 广播路径仍调用 `BroadcastToRoom`，但 fanout 时房间内已无其他在线 Session，广播自然 no-op（详见 Story 10.5 BroadcastToRoom primitive 实装：空房间走 fast path 直接返回）
7. **关闭 leaver 自己的 WS Session**（若 leaver 仍持有该 `roomId` 的 WS 连接）：从 SessionManager 撤销 (unregister) leaver 在该 `roomId` 的 Session + close underlying WebSocket（close code = `4007`，reason = `"left room via HTTP"`，详见 §12.1 close code 表 4007 行；client 解析层按 4xxx 业务级终态语义处理，**不**自动重连）；该步骤**必须**发生在步骤 6 广播之后 —— 顺序由"广播 fanout 时排除 leaver 自己 Session"语义钦定（§12.3 `### 成员离开` 关键约束），但 leaver 的 Session 撤销不能拖到心跳超时（默认 60s）才触发，否则在 close 前的窗口内 leaver 仍会收到该 roomId 的 `member.joined` / `member.left` / 后续 epic 广播消息（如 Story 14.x `pet.state.changed` / Story 17.x `emoji.received`），违反"HTTP leave 后立即与房间 WS 解耦"语义。Session 撤销失败（leaver 未持 WS 连接 / 已断开）→ no-op，不影响 HTTP 200 响应（fire-and-forget）
8. 返回 `data.{roomId, left: true}`

**事务边界规则**：步骤 2 ~ 4 必须在同一 MySQL 事务中（参见数据库设计 §8.6 房间事务边界）；步骤 2 的 `RowsAffected == 0` 兜底**必须**在事务内回滚（不允许在事务外做 SELECT 校验后再开事务 —— 那种实现仍存在 step1-SELECT 与 DELETE 之间的竞态窗口，无法消除并发 race）；步骤 6 在事务**外**触发（fire-and-forget）；步骤 7 同样在事务**外**触发（fire-and-forget），且**必须**在步骤 6 广播之后（顺序保证 leaver 不会在自己 Session 被关闭后再收到由本次 leave 触发的 `member.left` —— fanout 已经物理上跳过 leaver Session，二者顺序仅影响清理副作用语义）。

**WS 断线场景与本接口的关系**（r3 锁定语义）：心跳超时（→ close 4005）、client 主动 close、app 关闭 / 切后台、TCP 1006 异常断开等任何 WS 层断开**都不**走本接口的事务路径（不删 `room_members` 行、不改 `users.current_room_id`、不广播 `member.left`）—— 它们仅清理 ephemeral 层（SessionManager 撤销 Session + Redis presence 清理），由 Story 10.4 onUnregister 钩子统一负责；详见 §10.3 "roster 语义与 WS 断线交互" 小节。换言之，**只有本接口（HTTP leave）+ 步骤 7 的 close 4007** 是真正的"离开房间"路径。WS 断线后 `room_members` 行仍在，user 仍在 roster 中；用户通过 reconnect / 重新打开 app 即可回到原房间，无需重新 join —— 这是 §12.1 4005 retryable 语义的契约基础。

#### 响应体

成功（code = 0）：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `data.roomId` | string | 必填 | 离开的房间主键（BIGINT 字符串化）；与请求 path 中 `roomId` 一致 |
| `data.left` | boolean | 必填 | 固定 `true`（成功路径必返） |

JSON 示例：

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

> **注**：响应中**不**包含"房间是否已 closed"字段 —— client 不直接感知房间状态变化，仅 `data.left = true` 表示当前 user 已离开；房间状态变化是 server 内部副作用，**不**对外暴露（避免 client 围绕"我是否是最后一人"做特殊 UX 分支）。

#### 可能的错误码

| code | message | 触发条件 |
|---|---|---|
| 1001 | 未登录 / token 无效 | auth 中间件拦截 |
| 1002 | 参数错误 | `roomId` 路径参数格式错（非数字 / 长度 > 20 字符） |
| 1005 | 操作过于频繁 | rate_limit 中间件拦截 |
| 6004 | 用户不在房间中 | 服务端逻辑步骤 1（预检）：`users.current_room_id != roomId`（含 null）；或步骤 2（事务内兜底）：`DELETE room_members ... RowsAffected == 0`（并发 race 输家） |
| 1009 | 服务繁忙 | 事务回滚 / DB 异常 / 内部 panic（**不**含步骤 2 的 `RowsAffected == 0` —— 那条走 6004） |

> **注**：本接口**不**触发 6001 / 6002 / 6003 / 6005 —— leave 仅校验"用户与房间归属"，不校验房间是否存在（即使 `rooms` 已被某种方式删除，只要 `users.current_room_id` 与 path `roomId` 一致仍允许 leave；6001 不触发因为 leave 不查 `rooms` 表存在性，仅查 `users.current_room_id`）。

**关键约束**：

- 6004 触发条件包含三种场景（client UX 一致处理）：(a) `users.current_room_id == NULL`（步骤 1 预检，用户当前不在任何房间）+ (b) `users.current_room_id != path roomId`（步骤 1 预检，用户在其他房间）+ (c) 同一用户并发两次 leave 同一房间，赢家事务内已删除 `room_members` 行，输家步骤 2 `DELETE` 0 行受影响（事务内兜底回滚）—— 都返回 6004，client 收到 6004 就清本地房间状态、不重试
- 步骤 2 的 `RowsAffected == 0` 兜底是**必须**项（不是优化）：缺失则两次并发 leave 中输家会继续走完步骤 3 ~ 7 —— 即使步骤 3 把已 NULL 的 `current_room_id` 再次写 NULL（idempotent）、步骤 4 房间剩余数已经被赢家算过、步骤 6 广播会在房间内产生**重复** `member.left` 事件、步骤 7 会试图关闭已不属于该房间的 leaver Session —— 全部是错误副作用。**不**用 `SELECT ... FOR UPDATE` 替代 `RowsAffected == 0` 兜底（虽然两者都能消除 race，但 `RowsAffected == 0` 更轻量、单条 DELETE 即可、不引入额外行锁）
- "最后一人离开 → 房间 closed"是 server 端**单向**状态变化：closed 房间无法被重新激活（节点 4 阶段无"重启房间"接口）；这条规则确保 `rooms.status` 严格单调（1 → 2，无回退路径），简化 server 状态机
- `data.left` 固定 `true`：与 §10.4 `data.joined` 设计对称，避免引入"left: false" 模糊语义

---

> **§10 房间章节末尾引用**：`GET /home.data.room.currentRoomId` 由 Story 11.10 真实实装（节点 4，Epic 11 收官）—— 本 story（11.1）**不**改 §5.1 GET /home schema；§5.1 已在 Story 4.1 锚定 + Future Fields 已标注 `room.currentRoomId` 节点 4 由 Story 11.10 注入真实数据。


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
- `roomId`：路径参数，必须是用户当前所在房间的 ID。client 端 roomId 来源**按场景分两路**（**不要**使用 `GET /me.user.currentRoomId` —— 该字段是永久 `null` schema 占位，无后续节点回填计划，详见 §4.3 Future Fields 引用块）：
  - **首次连接 / 刚完成房间动作（热路径）**：client 直接从 **本次会话内的房间动作响应** 取 roomId —— `POST /rooms` 创建房间响应的 `data.room.id`、`POST /rooms/{roomId}/join` 加入房间响应的 `data.roomId`、用户点击外部分享链接 / 主动输入 roomId 等显式输入路径，建连前 client 内存中已经持有 roomId
  - **冷启 / token 刷新 reconnect（Epic 12 reconnect 场景）**：client **本地持久化**最近一次成功 WS 连接的 roomId（iOS Story 12.5 实装：UserDefaults / Keychain 存最近一次 `roomId`，由"成功握手并接受 `room.snapshot`"事件触发写入；用户主动 `POST /rooms/{roomId}/leave` 或服务端 close 4003 / 4004 时清除）。app 冷启 / token 刷新后从本地读取，发起 WS 连接验证：服务端按 §12.1 校验顺序判定，4003 = 用户已不在该房间（按 close code 表回退到主界面），4004 = 房间已不存在（同样回退）
  - **未来增强**：节点 4 之后 Story 11.10 真实实装 `GET /home.room.currentRoomId`（详见 §5.1 `GET /home` 字段表 + Future Fields）后，client 可以用 server 端权威来源**替代**本地持久化作为冷启场景的 roomId 来源；在此之前，**禁止**依赖该字段，因为它返回 `null`
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

1. **第一条**：`room.snapshot`（payload schema 见 §12.3，节点 4 阶段为 placeholder：room 三字段 + members 数组反映 `room_members` 全部成员行 —— 单表查询不 JOIN，丰富字段 `nickname` / `pet.petId` 在 placeholder 阶段允许空字符串，详见 §12.3 placeholder 示例与字段表 placeholder 行为）

> **权威性 + Merge 语义（client 必读）**：`room.snapshot` 是握手成功后**必发**的第一条 authoritative 消息，但其权威性是 **enrich/correct** 而**非** wipe-out —— client **必须**对每个 `members[]` entry 做**字段级 merge**（详见 §12.3 "client merge contract" 小节）。简言之：snapshot 中**非空**字段覆盖 client 已有值（这是真实 authoritative 数据）；snapshot 中**空字符串**字段（`""`）保留 client 已有值（空字符串 = "我不知道这个值" 的 placeholder 信号，**不是** "请清空")；snapshot 中**未出现**的字段保留 client 已有值。该契约让 Story 10.7 placeholder 阶段（部分字段空）和 Story 11.7 真实阶段（全字段非空）行为一致：snapshot 永远只能 enrich/correct client state，**禁止** wipe out client 已通过 §15.6 推荐流程从 `GET /api/v1/rooms/{roomId}` 加载的真实丰富字段。

> **Epic 10 阶段**（即本 story 范围 / Story 10.1 ~ 10.7）服务端 → 客户端**只**会主动发送 `room.snapshot` / `pong` / `error` 三种消息（与本节末 §12.3 业务消息延后锚定块 + §1 节点 4 协议骨架冻结声明一致）；`member.joined` 等业务广播消息的字段层契约与"是否在握手时广播"的语义由 Story 11.1（Epic 11）锚定 —— 即 Epic 10 阶段服务端**不**在握手时广播 `member.joined`，但 Epic 11 起按对应 story 锚定的语义扩展消息集合（Story 11.1 锚定 `member.joined` / `member.left` 加入消息集，Story 14.1 锚定 `pet.state.changed`，Story 17.1 锚定 `emoji.received`）；本声明仅冻结 Epic 10 阶段的 server-active message set，不阻止后续 epic 的合法扩展。

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
| 4005 | server 主动 close | 心跳超时（60 秒未收到任何 client 消息含 ping，由 Story 10.4 心跳框架触发；详见 §12.2 `ping` 小节） | 是（reason = `"heartbeat timeout"`） | **应**自动重连（指数退避；视为 transient network failure，与 1006 / 1011 同等对待 —— 不视为业务级拒绝，因为底层多半是网络抖动 / 客户端切后台超时）；reconnect 自洽性 by §10.3 "roster 语义与 WS 断线交互" + §10.5 "WS 断线场景与本接口的关系"：心跳超时**不**删 `room_members` 行 / **不**广播 `member.left`，仅清 ephemeral 层 → reconnect 握手时 §12.1 校验顺序步骤 5（`room_members` 表查询）通过，用户保留原房间座位 |
| 1000 | server 或 client 任一方主动 close | 服务端 / 客户端正常关闭 | 否 | 客户端主动 close 不重连；服务端主动 close（如 graceful shutdown）客户端可选重连 |
| 1001 | server 或 client 任一方主动 close | going away（服务端重启 / 客户端切后台） | 否 | 服务端重启 → 客户端**应**自动重连（指数退避，参考 iOS Story 12.5）；客户端切后台 → 客户端自身决定 |
| 4006 | server 主动 close | 客户端违反协议策略 —— 节点 4 阶段唯一触发条件：单条消息 frame 超过 `ws.max_message_size_bytes`（默认 16 KB） | 是（reason = `"message too large"`） | **不**自动重连（视为客户端实装 bug，重连仍会被 close）；记 log error 后回退 |
| 4007 | server 主动 close | leaver 通过 HTTP `POST /rooms/{roomId}/leave` 主动离开房间，且该 leaver 仍持有同一 `roomId` 的 WS 连接 —— server 在 leave 事务成功提交 + `member.left` 广播之后立即触发本 close；详见 §10.5 服务端逻辑步骤 7 | 是（reason = `"left room via HTTP"`） | **不**自动重连（业务级拒绝，leaver 已主动离开房间，重连会因 §12.1 校验顺序步骤 5"用户房间归属校验"失败被 close 4003）；client 收到本 close code **应**视为"自己的 HTTP leave 已被 server 端确认完成"的协议层信号 —— 用于触发本地 RoomView 退出 / RoomViewModel 清理等 UX 路径，client 实装层细节由 iOS Story 12.7 LeaveRoomUseCase 锚定 |
| 1011 | server 主动 close | 服务端内部错误（panic / 不可恢复异常）；**含**握手完成后 SnapshotBuilder 构建初始 `room.snapshot` 失败的场景（reason = `"snapshot build failed"`） —— 因为 §12.1 钦定 `room.snapshot` 是握手成功后必发的第一条消息，构建失败若不 close 而仅推 `error`，client 会永远等待一个永不到达的 snapshot，房间页无法初始化 | 是（reason 应包含简短错误提示但**不**泄漏 stack trace） | 客户端**应**自动重连（指数退避，但限制最大重试次数避免雪崩） |

**关键约束**：

- 4001 / 4002 / 4003 / 4004 / 4006 / 4007 是**业务级**拒绝（4xxx 段是应用自定义；WebSocket RFC 6455 规定 4000-4999 为应用保留段），重试无意义，客户端**不**应自动重连；客户端应展示明确 UX 提示并回退（4006 = 客户端实装 bug，记 log error 后回退；4007 = 自身 HTTP leave 完成的协议层确认，触发 RoomView 退出 UX，不需提示错误）
- **4005 是 4xxx 段中的例外**：虽然位于 4xxx 应用自定义段，但语义是 transient network failure（心跳超时多半是网络抖动 / 客户端切后台），不是业务级拒绝；客户端**应**自动重连，与 1006 / 1011 同等对待（指数退避 + 最大重试次数限制）。该 retryable 语义的可行性由 §10.3 / §10.5 / §12.3 钦定共同保证："WS 断开（含心跳超时）仅清 ephemeral 层；持久层 `room_members` / `users.current_room_id` 仅由 HTTP join / leave 改变" —— 因此 4005 reconnect 握手时 `room_members` 行必然仍在，§12.1 校验顺序步骤 5 通过，client 重新进入原房间无需重新 join
- 1000 / 1001 / 1011 是**协议 / 网络**级断开（1xxx 段是 RFC 标准段，可由服务端主动 emit），客户端**应**自动重连（除 1000 主动关闭外）
- **不使用 RFC close code 1006 / 1008 / 1009**（**不**出现在上方 close code 表内，**也不**由服务端主动 emit，原因分两类）：
  - **1006**：RFC 6455 §7.1.5 reserved status code —— **MUST NOT** be set as a status code in a Close control frame。1006 仅由客户端 WebSocket runtime 在底层 TCP 异常断开 / 网络抖动且未收到 close frame 时**本地合成**通知上层；服务端实装层（Story 10.3 / 12.5）**禁止**写 1006 到 close frame。客户端收到 1006 时按 transient network failure 处理 —— 与 1011 / 4005 同等对待（指数退避 + 最大重试次数限制自动重连）。1006 不进入 close code 表的另一个理由：§3 全局错误码表已将 `1006`（状态不允许当前操作）用作应用层错误码（在 §12.3 `error.payload.code` 中出现），与其它"1xxx 段被 §3 占用"的 code 同类，避免数字空间冲突
  - **1008 / 1009**：RFC 6455 §7.4.1 分别定义 1008 (Policy Violation) / 1009 (Message Too Big) 为标准 close code，但本协议**禁止**这两个 1xxx 段值用于 close frame，原因是 §3 全局错误码表已将 `1008`（幂等冲突）/ `1009`（服务繁忙）用作应用层错误码（在 §12.3 `error.payload.code` 中出现）。"消息超大"场景统一改用 **4006**（4xxx 应用自定义段，与 4001-4005 同段位，数字空间不与 §3 重叠）；其他 policy violation 场景同样走 4xxx 段
  - **共同根因**：同一数值同时出现在 close frame 和 application error frame 会让客户端、日志、监控无法仅凭数字区分 transport-level fatal 与 application-level transient/retryable（前者要 close + 不重连或固定重连分类，后者要忽略 + 保连接）。因此 §3 已占用的 1xxx 段值（含 1006 / 1008 / 1009）一律**禁止**作为 close code；服务端主动 emit 的 close code 限定在 1000 / 1001 / 1011 + 4001 / 4002 / 4003 / 4004 / 4005 / 4006 / 4007 这 10 个值内
- 4001 触发时 server **不**写 log error（这是常态，token 过期是正常业务），写 log info；4003 / 4004 写 log warn（疑似客户端实装 bug 或数据不一致）；4005 写 log info（这是常态，心跳超时多半是网络抖动 / 切后台；写 warn 会让正常网络抖动场景下日志噪声暴涨）；4006 写 log error（必排查，疑似客户端 bug 或恶意流量）；4007 写 log info（这是常态，用户主动 leave 的协议确认）；1011 写 log error（必排查）

#### 服务端校验顺序

握手期间服务端必须按以下顺序校验，任一步失败立即 close 并使用对应 close code：

1. **解析 query**：缺 `token` 参数 → close 4001（reason = `"missing token"`）
2. **路径参数校验**：`roomId` 非数字 / 缺失 → close 4002
3. **token 校验**（不查 DB，仅本地校验签名 + 过期，复用 Story 4.4 token util）：失败 → close 4001
4. **room 存在性校验**（查 `rooms` WHERE `id = ?`）：失败 → close 4004
5. **用户房间归属校验**（查 `room_members` WHERE `user_id = ? AND room_id = ?`）：失败 → close 4003
6. **内部错误**（panic / DB 异常等）：close 1011

校验通过后**严格按以下顺序**执行（顺序由 §12.1.3"`room.snapshot` 是握手成功后必发的**第一条** authoritative 消息"契约钦定，**禁止**调换）：

1. 创建 Session 对象（详见 Go 项目结构 §9.1）
2. 注册到 SessionManager
3. **同步**构建并发送 `room.snapshot`（§12.3 schema） —— 必须在握手响应路径里**同步**写入 underlying `*websocket.Conn`（或 enqueue 到 send buffer 后**flush**完成）；**不**通过"投递到尚未启动的写 goroutine 队列"完成，因为本步骤完成时写 goroutine 尚未启动，消息将永不被发送
4. 启动读 / 写 goroutine —— 此步骤**必须**在第 3 步 snapshot 写入完成（write call 返回 nil error）之后才启动；snapshot 写失败时**不**启动 goroutine，按 §12.1 close code 表 1011 行（reason = `"snapshot build failed"`）close 连接
5. 在 Redis presence 记录在线（详见 Story 10.6）

> **顺序 rationale**：若先启动读 goroutine 再推 snapshot，client 在 upgrade 完成的下一个 tick 即可发送合法 `ping`（client 不知道 server 还没推过 snapshot），server 写 goroutine 已经启动 → 直接回 `pong`，client 收到 `pong` 在 `room.snapshot` 之前，违反 §12.1.3 "first must be snapshot" 契约；尤其是 SnapshotBuilder 命中 DB 慢路径 / 锁等待场景下窗口被放大。把 snapshot 写入放在 goroutine 启动**之前**的同步段执行 = 物理上保证 snapshot frame 必定先于任何"对 client 消息的响应 frame"出现在 wire 上（写 goroutine 此时还不存在，无法产生任何竞争性写入）。

**注意**：第 5 步是**协议层面强制的授权环节**，必须以**持久化 membership 数据**（即 `room_members` 表）为 single source of truth —— **禁止**使用 Redis presence 替代该校验。理由：presence 仅表示 ephemeral 在线态，不代表 user 是房间合法成员；以下两种常见场景下，"用 presence 做 membership 校验"会错误返回"非成员"并 close 4003：(a) server 重启 / Redis 清空后第一个合法成员重连（presence 全空）；(b) 合法成员的 presence entry TTL 过期后重连。如未来需要为该热路径降低 MySQL 读压（Story 10.6 / 11.4 实装阶段评估），**必须**引入与 presence 区分的"durable membership cache"（持久层语义、由加入 / 退出房间事务原子失效），而**不是**复用 presence；本协议层面的 §12.1 校验顺序不因任何缓存方案而改变。

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
- 单条消息 frame **大小上限 16 KB**（默认值；配置 `ws.max_message_size_bytes`，**prod 必须使用默认值不可覆盖**，dev/test 可覆盖）；超限服务端 close **4006**（policy violation, 4xxx 应用自定义段，reason = `"message too large"`） + 不重连（客户端实装 bug）。**不使用 RFC 1008 / 1009**：本协议 §3 全局错误码表已将 `1008`（幂等冲突）/ `1009`（服务繁忙）用作应用层错误码（在 §12.3 `error.payload.code` 中出现），为避免 close frame code 与 application error code 数字冲突，"消息超大"统一走 4006（与 4001-4005 同段位，数字空间不与 §3 重叠）；详见 §12.1 close code 表关键约束

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
> **Epic 10 阶段**（即本 story 范围 / Story 10.1 ~ 10.7）客户端 → 服务端**只**有 `ping` 一种合法消息；客户端**不**应在 Epic 10 阶段发送 `emoji.send`（即使协议草稿示例存在），server 收到非 `ping` 消息当前阶段**忽略**（log warn + 不 close 连接，避免兼容性问题；具体行为 Story 10.3 实装时锁定）。Epic 17 起按 Story 17.1 锚定将 `emoji.send` 加入合法 client-active 消息集合 —— 本声明仅冻结 Epic 10 阶段的 client-active message set，不阻止后续 epic 合法扩展。

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
- 特例：`error` 消息是**双重语义**的 —— 当它作为某 client 请求的响应（如 `emoji.send` 失败）时按"响应类"语义回带 `requestId`；当它是 server 主动产生（如运行时业务状态异常 —— 注意**不含**握手期 SnapshotBuilder 失败，该场景统一走 close 1011 而非 `error`，详见 §12.1 close code 表 1011 行 + §12.3 `error` 小节末尾"snapshot 构建失败处理路径"注）时按"主动推送类"语义固定 `""`。客户端解析层（Story 12.3）**应**把 `error` 与 `pong` 同等对待：`requestId != ""` → 走 request-response 配对；`requestId == ""` → 走全局错误事件总线。详见 §12.3 `error` 小节
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
| `payload.room.memberCount` | number (int) | 必填 | 房间总成员数 = 当前 `room_members WHERE room_id = ?` 行数；与下文 `payload.members` 数组长度严格相等（见本节末"不变量"小节）；节点 4 阶段 Story 10.7 placeholder 实现 = `SELECT COUNT(*) FROM room_members WHERE roomId=?` 的真实行数（或同一次 query 直接取 `len(members)`，二者必须一致），**禁止**写死为 1；Story 11.7 真实实现 = 同样的 `room_members` 行数，差异仅在于 placeholder 阶段不 JOIN `users` / `pets`（丰富字段降级），**不**在成员数量上与真实实装区分 |
| `payload.members` | array | 必填 | 房间全成员列表 = 当前 `room_members WHERE room_id = ?` 全部行（**不**做"WS 此刻是否连接"层的过滤，roster 反映 server 端"仍在房间"的成员；与 §10.3 `data.members[]` 同语义，详见 §10.3 "roster 语义与 WS 断线交互"小节）；节点 4 阶段 Story 10.7 placeholder 实现 = `SELECT * FROM room_members WHERE roomId=?` 的全部行（**禁止**只返回当前握手用户自己 —— 房间已有 ≥2 成员时漏返其他成员会让 client 把 snapshot 当 authoritative state 错误清空已加载的 roster），单表查询不依赖 JOIN，本身已足够简单；丰富字段在 placeholder 阶段降级（`nickname` / `pet.*` 行为见各字段 placeholder 行）；Story 11.7 真实实现按 `room_members` JOIN `users` JOIN `pets` 聚合，**仅丰富字段差异**，成员条目数量与 placeholder 一致 |
| `payload.members[].userId` | string | 必填 | 成员用户 ID（BIGINT 字符串化）；node-4 placeholder 阶段直接来自 `room_members.userId`，**所有成员行都返回**（不限于握手用户） |
| `payload.members[].nickname` | string | 必填 | 成员昵称；node-4 placeholder 阶段（Story 10.7）允许返回**空字符串** `""`（不 JOIN `users` 表，避免 placeholder 过度耦合 Story 11.7 的多表 JOIN）；**空字符串语义 = "我不知道这个值"**，client 按本节末"client merge contract"**保留** client 已有真实昵称（如来自 `GET /api/v1/rooms/{roomId}` 响应），**禁止**用空串覆盖；client 渲染时若本地无真实值，空串可降级为占位文案；Story 11.x（具体由 Story 11.7 真实 SnapshotBuilder 实装）由 `users.nickname` 真实回填 |
| `payload.members[].pet.petId` | string | 必填 | 成员当前宠物 ID（BIGINT 字符串化）；node-4 placeholder 阶段（Story 10.7）允许返回**空字符串** `""`（不 JOIN `pets` 表）；**空字符串语义 = "我不知道这个值"**，client 按本节末"client merge contract"**保留** client 已有真实 petId（如来自 `GET /api/v1/rooms/{roomId}` 响应），**禁止**用空串覆盖；Story 14.x（pet 真实驱动时由 Story 11.7 同步扩展）回填真实 `pets.id` |
| `payload.members[].pet.currentState` | number (int) | 必填 | 宠物当前状态枚举：`1 = rest` / `2 = walk` / `3 = run`（与数据库设计 §6.4 `pets.current_state` 同义）；node-4 placeholder 阶段（Story 10.7）固定返回 `1`；Story 11.7 真实实现亦固定返回 `1`（Epic 14 才真实驱动） |
| `ts` | number (int64) | 必填 | 服务端发送时间戳（ms） |

> **Future Fields（节点 4 阶段为占位 / 节点 5 / 9 落地）**：
>
> - `payload.members[].avatarUrl`（成员头像 URL）：Story 11.7 真实实现时启用；节点 4 阶段 placeholder 不返回该字段（不要在 §12.3 字段表添加；client 解析为 `String?` 可选字段）；按本节末 "client merge contract" 中"未出现字段保留 client 已有值"规则，client **不**清空已通过 `GET /api/v1/rooms/{roomId}` 加载的真实 avatarUrl
> - `payload.members[].pet.equips`（成员当前装备）：Epic 26 / Story 26.6 落地后由 Story 11.7 同步扩展；节点 4 阶段不返回（按 merge contract 保留 client 已有值）
> - `payload.members[].pet.equips[].renderConfig`（装备渲染配置）：Epic 29 / Story 29.6 落地后由 Story 11.7 同步扩展；节点 4 阶段不返回（按 merge contract 保留 client 已有值）

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
      },
      {
        "userId": "1002",
        "nickname": "B",
        "pet": {
          "petId": "2002",
          "currentState": 1
        }
      }
    ]
  },
  "ts": 1776920345000
}
```

> **不变量（snapshot 内部一致性）**：`memberCount` 必须**严格等于** `members[]` 数组长度。两者**统一表示当前 `room_members` 行数**（即 server 端"仍在房间"的全部成员），**不**做"WS 此刻是否连接"层的过滤 —— 即 snapshot 是房间的 full roster view（按 `room_members` 全行），**不是** WS-online-only view。理由：(a) 若 `memberCount` 与 `members[].length` 任一方做"WS 在线过滤"而另一方不做，违反不变量；(b) 节点 4 阶段服务端**不**在握手时广播 `member.joined`（详见 §12.1 末尾 placeholder 注），客户端无法靠后续推送补齐 row 还在但 WS 暂未连上的成员，snapshot 必须自包含 `room_members` 全行；(c) 节点 4 阶段**不**引入"online / offline" 字段层区分 —— `room_members` 行的删除**唯一**路径是 HTTP `POST /rooms/{roomId}/leave` 事务（删行后触发 `member.left` 广播，该 user 不再出现在后续 snapshot / GET /rooms 响应中）；任何 WS 层断开（含心跳超时 / TCP 1006 / app 关闭）**仅**清 ephemeral 层（SessionManager + Redis presence），**不**触动 `room_members` 行 —— 因此该 user 在断线期间仍出现在后续 snapshot / GET /rooms roster 中，与 §12.1 4005 retryable 语义自洽（详见 §10.3 末尾"roster 语义与 WS 断线交互"小节）。节点 4 阶段 Story 10.7 placeholder 实装时 `members[]` 必须**反映 `room_members` 全部成员行**（最少 1 个，即握手用户自己；房间已有 ≥2 成员时必须返回全部）—— **禁止**写"全零 placeholder"（`memberCount: 0` + `members: []`），也**禁止**写"单成员快照"（仅当前握手用户自己，`memberCount` 写死为 1），因为：(i) §12.1 第 5 步握手成功**充分条件**只校验"当前用户已在 `room_members` 表中"，**不**保证房间只有 1 个成员；房间已有 ≥2 成员时单成员 snapshot 会让 client 把首条 authoritative 消息当成"房间被清空"，错误清空已加载的合法 roster；(ii) 推荐房间进入流程要求 client 先 `GET /api/v1/rooms/{roomId}` 加载房间状态后再开 WS（详见 §11.5 客户端推荐调用顺序），client 已经持有合法 roster 视图，再收到一个比真实成员少的 snapshot 同样会错误覆盖（无论是零成员还是单成员都属于"少返"）；(iii) §12.1.3 钦定 `room.snapshot` 是握手成功后**必发**的第一条 authoritative 消息，client 把它作为权威态采用，placeholder 必须给出**结构上真实**的快照（成员条目齐全），仅在丰富字段（`nickname` / `pet.*`）层面降级为占位默认值，**不**在成员数量上偷工。Story 11.7 真实实装时由**同一次** `room_members` JOIN `users` JOIN `pets` 聚合产出 `members[]`，`memberCount` 取该数组长度（或同一次 query 的 `COUNT(*)`），server 实装层面**禁止**让两者出现 drift；与 Story 10.7 placeholder 的差异**仅在丰富字段**（`nickname` / `pet.*` 真实回填），**不在成员数量**。

**节点 4 阶段 placeholder 示例**（Story 10.7 实装；与上述真实示例的差异**仅在丰富字段**（`nickname` / `pet.petId` 在 placeholder 阶段允许空字符串），`members[]` 必须反映 `room_members` 全部成员行；下例为房间已有 2 成员的场景 —— `userId: "1001"` 为当前握手用户、`userId: "1002"` 为另一已加入仍在 `room_members` 表中（WS 当前是否连接不影响 roster）的成员；`memberCount` = `members[].length` = 2，不变量不破）。**注意**：下例中 `nickname: ""` / `pet.petId: ""` 是"server 不知道"的 placeholder 信号 —— client 按下方 "client merge contract" **保留** 已通过 `GET /api/v1/rooms/{roomId}` 加载的真实值，**禁止**用空串覆盖；若 client 此前未加载真实值（如直接由 `POST /rooms/{roomId}/join` 进入未走 §15.6 流程），则保留空值，渲染时降级为占位文案：

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
        "nickname": "",
        "pet": {
          "petId": "",
          "currentState": 1
        }
      },
      {
        "userId": "1002",
        "nickname": "",
        "pet": {
          "petId": "",
          "currentState": 1
        }
      }
    ]
  },
  "ts": 1776920345000
}
```

> **placeholder 字段值来源说明**：上例 `members[]` 直接来自 `SELECT * FROM room_members WHERE roomId=?` 的全部行（**单表查询，不 JOIN `users` / `pets`**）—— `userId` 取 `room_members.userId`；`nickname` 在 placeholder 阶段返回空字符串 `""`（避免 JOIN `users`，由 Story 11.7 真实实装时由 `users.nickname` 回填）；`pet.petId` 在 placeholder 阶段返回空字符串 `""`（避免 JOIN `pets`，由 Story 14.x 真实驱动时由 Story 11.7 同步扩展）；`pet.currentState` 节点 4 阶段固定 `1`（rest，与数据库设计 §6.4 `pets.current_state` 同义；Epic 14 才真实驱动）。Story 10.7 SnapshotBuilder placeholder 实装路径：用单表查询 `SELECT * FROM room_members WHERE roomId=?` 取全部成员行（这是 placeholder 必须做的，**禁止**只取当前握手用户）；JOIN `users` / `pets` 由 Story 11.7 真实实装时再加，**不**在 Story 10.7 范围内 —— 这样 placeholder 反映真实 roster **结构**（成员 ID 全到位），仅丰富字段降级为 placeholder 默认值。

> **Client merge contract（client 解析 `room.snapshot` 时必须遵守）**：snapshot 是握手后**必发**的第一条 authoritative 消息（§12.1 握手成功流程），但其权威性是 **enrich/correct** 而**非** wipe-out。client 在收到 `room.snapshot` 时，**禁止**做 "把 `members[]` 整体替换 client 当前 roster" 的暴力赋值，**必须**对每个 member entry 做**字段级 merge**，规则如下：
>
> 1. **roster 集合层（`members[]` 数组本身）**：以 snapshot 的 `userId` 集合为权威集合 —— snapshot 没有的 `userId` → client 应**移除**（视为已离开房间的真实 authoritative 信号）；snapshot 有但 client 之前没有的 `userId` → client 应**新增** entry（用 snapshot 的字段值填充，空字符串字段保持空，由后续 `member.joined` / 重新 `GET /api/v1/rooms/{roomId}` enrich）。"成员存在性"是结构信息，snapshot 在这一层是 authoritative。
> 2. **字段级（每个 entry 的字段值）**：
>    - **非空值**（如 `nickname: "Alice"` / `pet.petId: "2002"` / `pet.currentState: 2`）：用 snapshot 的值**覆盖** client 已有值（这是真实 authoritative 数据，覆盖正确）。
>    - **空字符串**（`""`）：**保留** client 已有值。空字符串 = "server 不知道这个值"的 placeholder 信号（节点 4 placeholder 阶段 / 任何未来未 enrich 的字段同义），**不是** "请清空" 的指令；若 client 通过 §15.6 推荐流程从 `GET /api/v1/rooms/{roomId}` 已加载真实昵称 / petId，必须保留这些真实值，避免"每次重连退化为空昵称 / 空 petId"。
>    - **`null` 值**：与空字符串语义不同 —— `null` 在本协议中保留给"明确无值"语义（如 §4.3 `currentRoomId: null` = 用户当前不在任何房间）；snapshot 字段表中**未出现** `null` 取值的字段（成员级别字段不 null），故节点 4 阶段不需要处理 null case。
>    - **未出现的字段**（如 placeholder 阶段不下发 `avatarUrl` / `pet.equips`）：**保留** client 已有值（与空字符串等价处理）。
> 3. **数值字段**（`pet.currentState`）：节点 4 阶段 placeholder 与 Story 11.7 真实实装均固定 `1`（Epic 14 才真实驱动 motion_state）；client 收到该值时**应**直接覆盖 client 已有值（无 placeholder 信号约定 —— 数值字段不存在"空字符串"语义；当未来 Epic 14 真实驱动后，server 下发 2/3 即真实值）。
>
> **rationale**：在 placeholder 阶段（Story 10.7）和真实阶段（Story 11.7）应用同一套 merge 规则，行为一致：snapshot 永远只能 enrich/correct client state。Story 11.7 真实实装时，`nickname` / `pet.petId` 等字段都会下发非空值，触发 "覆盖" 路径；placeholder 阶段下发空字符串，触发 "保留" 路径 —— **server 实装层无需为两个阶段写不同 client merge 代码**，只需 client 始终按本契约执行。
>
> **client 实装位置**：iOS 侧由 Story 12.3（WS 消息解码层）+ Story 12.4（房间 ViewModel 状态合并层）实装；server 实装层（Story 10.7 SnapshotBuilder placeholder / Story 11.7 真实实装）只保证下发字段语义符合本契约，**不**关心 client 如何 merge。

### 成员加入（member.joined）

**触发**：

- `POST /api/v1/rooms/{roomId}/join` 加入房间事务**成功提交后**，server 调用 `BroadcastToRoom(roomID, {type: "member.joined", payload: {userId, nickname, avatarUrl, pet: {petId, currentState}}})` 广播给该房间内所有**其他**在线成员（不发给加入者自己 —— 加入者自己应收 HTTP 响应即知自己已加入，且加入者后续如建立 WS 连接，握手时 server 会下发含自己的 `room.snapshot`，与已连接成员通过 `member.joined` enrich roster 的路径互补）；payload **必须**含 `avatarUrl` + `pet.{petId, currentState}`，不能简化为仅 `userId + nickname` —— 已连接成员仅在握手时收一次 `room.snapshot`（§12.1.3 钦定），后续无新 snapshot 触发，**唯一**enrich 新成员展示字段（头像、宠物 ID、宠物状态）的路径就是 `member.joined` 自身 payload；若 trigger 实装层简化为仅下发 `userId + nickname`，已连接成员将永远看不到新成员的头像 / 宠物，违反"`member.joined` 必须自包含展示所需全部字段"语义（详见下方"字段"表 + Story 11.8 实装锚定）
- 节点 4 阶段**仅** Story 11.8 一处触发；任何 WS 层事件（含握手、心跳超时、断开重连）**都不**触发 `member.joined` —— `member.joined` 与 `room_members` 行的"新增"语义严格 1:1 对应，而新增持久层行**仅**通过 HTTP join 接口完成（详见 §10.4 服务端逻辑）；后续 epic 不在本路径加新触发条件（如 Story 14.x 状态广播走独立 `pet.state.changed`）

**字段**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | string | 必填 | 固定值 `"member.joined"` |
| `requestId` | string | 必填 | 固定 `""`（主动推送类消息，遵循 §12.3 通用信封） |
| `payload.userId` | string | 必填 | 加入的成员 user 主键（BIGINT 字符串化）；来自加入事务的当前 user |
| `payload.nickname` | string | 必填 | 加入的成员昵称；来自 `users.nickname`；**必非空字符串**（节点 2 阶段首次创建时 server 写入 `"用户{id}"`，必有真实值；不存在 placeholder 阶段空字符串场景） |
| `payload.avatarUrl` | string | 必填 | 加入的成员头像 URL；来自 `users.avatar_url`；可空字符串 `""`（首次创建用户时 `users.avatar_url` 为空；节点 4 阶段 server 端未做头像上传链路），**不**为 null —— client 解析层按 `String` 处理（与 §10.3 `data.members[].avatarUrl` 一致） |
| `payload.pet.petId` | string | 必填 | 加入的成员当前宠物 ID（BIGINT 字符串化）；来自加入事务前 server 已查询该 user 的活跃 `pets.id`；**必非空字符串**（节点 2 阶段首次注册时 server 写入默认 pet 行，每个 user 必有活跃 pet） |
| `payload.pet.currentState` | number (int) | 必填 | 加入时刻宠物当前状态枚举（`1 = rest` / `2 = walk` / `3 = run`，与数据库设计 §6.4 `pets.current_state` 同义）；节点 4 阶段固定 `1`（与 §10.3 `data.members[].pet.currentState` / §12.3 `room.snapshot` 同语义，Epic 14 才真实驱动） |
| `ts` | number (int64) | 必填 | 服务端发送时间戳（ms） |

> **Future Fields（节点 4 阶段为占位 / 节点 9 / 10 落地）**：
>
> - `payload.pet.equips`（成员当前装备）：Epic 26 / Story 26.6 落地后由 Story 11.8 同步扩展；节点 4 阶段**不**下发该字段；client 解析层按 `[<EquipDTO>]?` 可选数组处理，未出现时不视为契约违反
> - `payload.pet.equips[].renderConfig`（装备渲染配置）：Epic 29 / Story 29.6 落地后由 Story 11.8 同步扩展；节点 4 阶段不下发

JSON 示例：

```json
{
  "type": "member.joined",
  "requestId": "",
  "payload": {
    "userId": "1002",
    "nickname": "用户1002",
    "avatarUrl": "",
    "pet": {
      "petId": "2002",
      "currentState": 1
    }
  },
  "ts": 1776920345000
}
```

**关键约束**：

- `payload.nickname` **必非空字符串** —— 与 `room.snapshot.payload.members[].nickname` 在 placeholder 阶段允许空字符串的语义**不同**：`member.joined` 在加入事务**成功提交后**触发，server 必有真实 nickname（`users` 表已被读取过用于事务决策），无 placeholder 阶段；client 解析层应按 `nickname != ""` 假设处理（节点 4 阶段所有 `member.joined` 消息字段值都是真实数据）
- 广播范围：**仅**该房间内当前在线的其他 Session（不含加入者自己）—— 由 BroadcastToRoom primitive（Story 10.5）实装的 fanout 路径决定；加入者自己 = 当前 HTTP 请求方，自己收 HTTP 200 响应已知道结果，不需要再收一份 WS 通知
- `payload` 字段**完整自包含成员展示所需的全部丰富字段**（`userId` / `nickname` / `avatarUrl` / `pet.petId` / `pet.currentState`），client 收到 `member.joined` 后**可直接** append 一条完整的 roster entry，**不**需要走 `GET /api/v1/rooms/{roomId}` 二次拉取 —— 这是因为 `room.snapshot` 仅在握手时下发一次（§12.1.3），server **不**会在 `member.joined` 之后给已连接成员追发 snapshot；若 `member.joined` 仅含 `userId` / `nickname`，已连接成员将永远拿不到该新成员的 `avatarUrl` / `pet.*` 真实值（避坑：r1 review 指出过这条不一致）
- `payload.avatarUrl` 与 `payload.pet.petId` 的填充语义与 `GET /api/v1/rooms/{roomId}.data.members[]` 同字段一致（都是 `users.avatar_url` / 该 user 活跃 `pets.id` 的真实回填），**不**走"placeholder 空字符串"路径 —— 加入事务**成功提交后**才广播，server 必有完整真实值
- client 收到 `member.joined` 后**应**走 §12.3 client merge contract 字段级 merge：(a) roster 中已存在该 `userId` entry → 字段级覆盖（按 client merge contract"非空覆盖、空字符串保留 client 已有值"规则；本 story 中 `avatarUrl` 可能下发空串，按规则**保留** client 已有真实值）；(b) roster 中不存在该 `userId` entry → 新增完整 entry（含本字段表所有字段），渲染层立即可用，无需等待二次 snapshot 或额外 HTTP 拉取
- `payload.userId` / `payload.nickname` / `payload.avatarUrl` / `payload.pet.petId` / `payload.pet.currentState` 都必填（**禁止** payload 为 `{}` 或仅 `userId` / `nickname`）；缺字段视为契约违反，client 解析层**应**走"安全忽略 + log warn"路径（与 Story 10.1 `安全忽略未识别 type` 相同的容错策略，避免单条 malformed 消息把房间页搞崩）
- 节点 4 阶段加入者**不**主动收到自己的 `member.joined`：server 实装上应从 fanout 列表中排除加入者自己的 Session，client 解析层不需要做"自己 != 自己"的过滤（防御性编程层面 client 仍**应**对 `payload.userId == 当前 user.id` 走 noop 安全路径）

### 成员离开（member.left）

**触发**（r3 锁定）：

- `POST /api/v1/rooms/{roomId}/leave` 退出房间事务**成功提交后**，server 调用 `BroadcastToRoom(roomID, {type: "member.left", payload: {userId}})` 广播给该房间内所有**其他**在线成员（不发给离开者自己 —— 离开者已收 HTTP 响应；leaver 自己的 WS Session 由 §10.5 步骤 7 通过 close 4007 协议层确认完成）
- **唯一触发条件**就是上一条 HTTP leave 路径；任何 WS 层断开（含心跳超时 → close 4005、client 主动 close、app 关闭 / 切后台、TCP 1006 异常断开）**都不**触发 `member.left` —— WS 断开仅清 ephemeral 层（SessionManager + Redis presence），不改 `room_members` / `users.current_room_id`，不广播 `member.left`；详见 §10.3 "roster 语义与 WS 断线交互" 小节 + §10.5 "WS 断线场景与本接口的关系" 注解块
- 这条钦定与 §12.1 close code 4005 行"client 应自动重连"的 transient 语义自洽：心跳超时不删 row → reconnect 握手时 §12.1 校验顺序步骤 5 通过 → 用户保留座位

**字段**：

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `type` | string | 必填 | 固定值 `"member.left"` |
| `requestId` | string | 必填 | 固定 `""`（主动推送类消息，遵循 §12.3 通用信封） |
| `payload.userId` | string | 必填 | 离开的成员 user 主键（BIGINT 字符串化）；来自 HTTP leave 退出事务的当前 user |
| `ts` | number (int64) | 必填 | 服务端发送时间戳（ms） |

JSON 示例：

```json
{
  "type": "member.left",
  "requestId": "",
  "payload": {
    "userId": "1002"
  },
  "ts": 1776920345000
}
```

**关键约束**：

- `payload` 字段**精简**为仅 `userId`（与 `member.joined` 含 `nickname` 不同）—— 离开事件 client UX 不需要显示昵称（"X 离开了房间"中的 X 可由 client 从已有 roster 查到 nickname；即使没查到，UX 文案降级为"有人离开"也可接受），减少 server 加载压力
- 广播范围：**仅**该房间内当前在线的其他 Session（不含离开者自己）；离开者已收 HTTP 200 响应 + 由 §10.5 步骤 7 close 4007 协议层确认完成，不需要也无法再收 WS `member.left` 消息
- client 解析层收到 `member.left` 时**应**按 §12.3 client merge contract 集合层规则：从 client roster 中**移除** `payload.userId` 对应 entry（这是 authoritative 的离开信号，与 snapshot 集合层 authoritative 一致）；client **不**应等下一次 `room.snapshot` 才更新 roster
- 节点 4 阶段离开者**不**主动收到自己的 `member.left`：server 实装上应从 fanout 列表中排除离开者自己的 Session（且离开者 Session 在 §10.5 步骤 7 close 4007 后已不在 SessionManager 列表中，自然 fanout 不到）；client 解析层防御性走 `payload.userId == 当前 user.id` 的 noop 安全路径
- **触发不重复**：server 实装层应保证 `member.left` 广播严格 1:1 对应"`room_members` 行被删除"事件 —— 由于唯一删行路径是 HTTP leave 事务（详见 §10.5 服务端逻辑），同一 user 在同一 leave 事件中**只触发一次** `member.left` 广播；任何 WS 断开（含心跳超时）**禁止**走"被动 leave"路径删 row + 广播 `member.left`（详见 §10.3 / §10.5 + Story 11.8 实装）

### 收到表情广播

```json
{
  "type": "emoji.received",
  "requestId": "",
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
| `payload.code` | number (int) | 必填 | 错误码，复用 §3 全局错误码定义（如 `7001` 表情不存在 / `1009` 服务繁忙；**注**：`6005` 房间状态异常**不**用于握手后初始 `room.snapshot` 构建失败 —— 该场景走 close 1011，详见 §12.1 close code 表 1011 行 + 本节末"节点 4 阶段适用错误码"表下的注） |
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

> 注：当 error 是某 client 请求的响应（如 `emoji.send` 失败）时，`requestId` 回带原请求 `requestId`；当 error 是 server 主动推送（如运行时业务状态异常）时，`requestId` 固定 `""`。该示例可作为 Story 10.3 / Story 12.3 的复制粘贴样本使用。
>
> **不在 `error` 范围**：握手后初始 `room.snapshot` 构建失败**不**走 `error` 路径，统一走 close 1011（详见 §12.1 close code 表 1011 行 + 本节末"节点 4 阶段适用错误码"表下的注）。本节 `error` 仅承载"连接已可用、业务侧出问题但不致死"的非致命错误。

**节点 4 阶段适用错误码**（仅 Epic 10 协议骨架范围内）：

| code | 触发条件（节点 4 阶段） |
|---|---|
| 1009 | 服务繁忙（Session 内部异常但不致死，如临时 Redis 慢；client 应忽略并继续） |

> 业务错误码（如 `7001` 表情不存在）由对应 Epic 各自的 §X.1 锚定（如 Story 17.1）；本 story 不预先列入节点 4 阶段适用错误码表。
>
> **注（snapshot 构建失败的处理路径）**：握手完成后 Story 10.7 SnapshotBuilder 构建初始 `room.snapshot` 抛 error 时，server **不**走 "推 `error` 消息" 路径 —— 因为 §12.1 钦定 `room.snapshot` 是握手成功后必发的第一条消息，若仅推 `error` 而保持连接，client 会永远等待一个永不到达的 snapshot，房间页无法初始化、auto-reconnect 也不会触发。该场景统一走 close 路径：close **1011**（reason = `"snapshot build failed"`，详见 §12.1 close code 表 1011 行），客户端按 1011 语义自动重连（指数退避 + 最大重试次数限制）。错误码 `6005`（房间状态异常）保留给后续业务 epic 在房间已可用之后的运行时状态错误推送（如 Story 11.x / 14.x 业务流程中），**不**用于初始 snapshot 失败场景。

> **业务消息延后锚定**：上文 `### 成员加入` / `### 成员离开` / `### 收到表情广播` 三个小节中，**前两个**（`member.joined` / `member.left`）已由 **Story 11.1**（Epic 11 房间业务契约，节点 4 中段；锚定 revision 见 `git log --grep='story-11-1'` 检出的 Story 11.1 收官 commit `chore(story-11-1): ...`，以及 `_bmad-output/implementation-artifacts/sprint-status.yaml` 中 `11-1-接口契约最终化` 状态行）正式锚定字段表 + 触发条件 + 关键约束 —— 自此**接入** Epic 11 / 节点 4 server / client active message set。`### 收到表情广播`（`emoji.received`）保持"草稿示例"状态，待 Story 17.1 锚定。
>
> 各消息字段层契约的正式锚定 epic 如下（**已升级状态以粗体标注**）：
>
> - `member.joined` / `member.left` → **Story 11.1（已锚定）**（Epic 11 房间业务契约，节点 4 中段）
> - `pet.state.changed` → **Story 14.1**（Epic 14 宠物状态同步契约，节点 5）—— **注意**：现有 §12.3 草稿中**不存在** `pet.state.changed` 消息示例（仅在 epics.md 业务消息列表里出现），Story 14.1 落地时**需新增**该小节
> - `emoji.received` → **Story 17.1**（Epic 17 表情广播契约，节点 6）
>
> **server / client active message set 升级**（自 Story 11.1 起生效，覆盖 Story 10.1 r10 钉死的"Epic 10 阶段冻结"边界）：
>
> - **server → client active message set**：`room.snapshot` / `pong` / `error`（Epic 10 锚定）+ **`member.joined` / `member.left`（本 story 锚定）**
> - **client → server active message set**：`ping`（Epic 10 锚定）—— 节点 4 阶段 client 不主动发新业务消息（`emoji.send` 由 Story 17.1 锚定加入，非节点 4）
>
> 客户端 Story 12.3 / 12.4 解析层在节点 4 阶段**应**对未识别的 `type` 值（如收到不该到达的 `emoji.received` / `pet.state.changed`）走"安全忽略 + log warn"路径，**不**因未识别消息 close 连接 / crash app（Story 10.1 既有规则，本 story 沿用）。

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

