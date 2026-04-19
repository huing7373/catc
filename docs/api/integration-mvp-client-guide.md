# 联调 MVP 客户端对接指南（手表 + iPhone）

**适用范围：** Story 10.1 落地的 3 条 `DebugOnly` WS 消息（`room.join` / `action.update` / `action.broadcast`），让 Apple Watch (watchOS) + iPhone (iOS) 可以**今天**就跑联调，不等 Epic 4.1-4.5 的完整 presence / 房间栈。

**生命周期：** 本 MVP 在 Epic 4.1 上线时**整块废弃**（服务端 `room_mvp.go` + 3 条 DebugOnly 条目 `git rm`，**不**翻 DebugOnly flag 转正）。客户端基于本指南写的对接代码，到 Epic 4.1 上线时也需要相应重写以对接正式消息。详见 [§10 版本演进](#10-版本演进与-epic-41-迁移预告)。

**权威来源：**

- **wire shape 单一真相源：** `server/internal/dto/ws_messages.go` + `docs/api/ws-message-registry.md`
- **服务端行为单一真相源：** `server/internal/ws/room_mvp.go`
- **端点发现：** `GET /v1/platform/ws-registry`（无鉴权；debug 模式列全部 6 条消息，release 模式当前列 0 条）

如果本指南与上面三处任一出现分歧，**以服务端代码为准**，并请立即反馈文档问题到后端仓库。

---

## 1. 前置条件

### 1.1 服务端启动模式

服务端配置必须是 **debug 模式**（`[server].mode = "debug"`）。release 模式下**零装配**——WS 不接受 `room.join` / `action.update`，客户端发送会收到 `UNKNOWN_MESSAGE_TYPE`。

校验方法：任何客户端都可先请求 `GET /v1/platform/ws-registry`（HTTP，无鉴权），响应 `messages` 数组里出现 `room.join` / `action.update` / `action.broadcast` 表示 debug 模式。

### 1.2 WS 端点

```
GET  /ws          ← WebSocket upgrade
```

协议：**WebSocket over HTTP(S)**，文本帧（TextMessage）。

### 1.3 Authorization（debug 模式特殊约定）

- header：`Authorization: Bearer <token>`
- debug 模式下 `<token>` 的**任意非空字符串**都会被接受，并且**`<token>` 本身即 userID**
- 真机联调建议：用 `watch-alice` / `phone-bob` 这种语义 id，日志里好识别
- 无鉴权或空 bearer：服务端返回 `401 Unauthorized`

> **Epic 4 迁移提示：** Epic 1.1 上线 Sign in with Apple JWT 后，此处会换成真实 JWT。服务端对 WS upgrade 用的是独立的 `TokenValidator` 接口（`internal/ws/token_validator.go`），客户端侧只需要换 token 内容，连接流程不变。

### 1.4 Ping / Pong 心跳

服务端每 30 秒发一次 `PingMessage`，pong 超时 60 秒。`gorilla/websocket` / 标准 `URLSessionWebSocketTask` 默认都会自动回 pong，客户端通常无需手动处理。

---

## 2. 连接建立

### 2.1 URL 模板

```
ws://<host>:<port>/ws      ← 开发环境（本机 / 局域网 debug）
wss://<host>/ws            ← 真机联调走 HTTPS（Apple Watch 要求 ATS，详见 §2.3）
```

### 2.2 握手示例（HTTP 层）

```http
GET /ws HTTP/1.1
Host: dev.example.local:8080
Upgrade: websocket
Connection: Upgrade
Sec-WebSocket-Version: 13
Sec-WebSocket-Key: <client-generated>
Authorization: Bearer watch-alice
```

握手成功：`HTTP/1.1 101 Switching Protocols`。握手失败：
- `401` → Authorization 缺失 / 空
- `403` → 设备被拉黑名单（Story 0.11，debug 模式一般不会触发）
- `429` → 连接限流（Story 0.11）

### 2.3 Apple Watch / iOS ATS（App Transport Security）注意

- 真机 `watchOS` 默认**不允许**非 HTTPS 的 `ws://`
- 联调期如果服务端只能提供 `ws://`，需要在 `Info.plist` 里给联调域加 `NSAppTransportSecurity` 豁免（仅 debug build，发布前必须移除）
- 建议优先部署一个开着 HTTPS 的联调环境（本地 nginx / Caddy 前置自签名证书即可）

### 2.4 Envelope 信封格式

所有上行（up）和双向（bi）消息，`payload` 封装在统一 envelope 里：

**上行（client → server）：**

```json
{
  "id": "<client-generated unique id>",
  "type": "<domain.action>",
  "payload": { ... }
}
```

- `id`：客户端生成，**每条消息必须全局唯一**（建议 UUIDv4）。响应会通过 `id` echo 回来，客户端据此匹配请求/响应
- `type`：`"room.join"` / `"action.update"` 等
- `payload`：具体请求体，见下面每条消息说明

**下行响应（server → client，回应上行请求）：**

```json
{
  "id": "<echo of request id>",
  "ok": true,
  "type": "<domain.action>.result",
  "payload": { ... }
}
```

或错误：

```json
{
  "id": "<echo>",
  "ok": false,
  "type": "<domain.action>.result",
  "error": { "code": "VALIDATION_ERROR", "message": "..." }
}
```

**下行推送（server → client，无请求触发）：**

```json
{
  "type": "action.broadcast",
  "payload": { ... }
}
```

⚠️ **下行推送没有 `id` 和 `ok` 字段**（信封更简洁）。客户端解码时要能识别这两种 downstream 形状。

---

## 3. 消息 ①：`room.join`

**用途：** 加入一个房间，并收到当前房间里其他成员的快照（含他们最后上报的 action）。如果用户之前在别的房间，服务端会先把他从旧房间移除。

**方向：** bi（upstream request + downstream response）
**幂等：** 重复 join 同一房间是安全的（服务端 no-op；不会广播；不会重复计数）

### 3.1 Request payload

```json
{ "roomId": "<string, 1-64 bytes>" }
```

| 字段 | 类型 | 约束 |
|---|---|---|
| `roomId` | string | 1-64 字节（UTF-8 byte length），必填 |

### 3.2 Response payload（ok）

```json
{
  "roomId": "<string>",
  "members": [
    {
      "userId": "<string>",
      "action": "<string>",
      "tsMs":   1712345678901
    }
  ]
}
```

| 字段 | 类型 | 语义 |
|---|---|---|
| `roomId` | string | echo 自请求，便于客户端确认 |
| `members` | array | 当前房间**其他**成员（**不含自己**）。**永远序列化为数组**，空房间是 `[]` 不是 `null` |
| `members[].userId` | string | 其他成员的 userID |
| `members[].action` | string | 该成员最后一次 `action.update` 上报的值；**从未 update 时为空串 `""`** |
| `members[].tsMs` | int64 | 该成员最后一次 action 的服务端时间戳（Unix ms）；**从未 update 时为 `0`** |

### 3.3 Errors

| code | message | 原因 |
|---|---|---|
| `VALIDATION_ERROR` | `roomId required` | `roomId` 字段缺失或为空串 |
| `VALIDATION_ERROR` | `roomId exceeds 64 bytes` | `roomId` 超过 64 字节 |
| `VALIDATION_ERROR` | `roomId must be valid UTF-8` | `roomId` 不是合法 UTF-8 |
| `VALIDATION_ERROR` | `invalid room.join payload` | payload JSON 解析失败 |

### 3.4 Swift Decodable 建议

```swift
// 信封（复用）
struct WSRequest<P: Encodable>: Encodable {
    let id: String
    let type: String
    let payload: P
}

struct WSResponse<P: Decodable>: Decodable {
    let id: String
    let ok: Bool
    let type: String
    let payload: P?
    let error: WSError?
}

struct WSError: Decodable {
    let code: String
    let message: String
}

// room.join 专属
struct RoomJoinRequest: Encodable {
    let roomId: String
}

struct RoomJoinResponse: Decodable {
    let roomId: String
    let members: [RoomMemberSnapshot]
}

struct RoomMemberSnapshot: Decodable {
    let userId: String
    let action: String      // empty = 该成员从未发过 action.update
    let tsMs: Int64         // 0 = 该成员从未发过 action.update
}
```

### 3.5 调用示例

```swift
let req = WSRequest(
    id: UUID().uuidString,
    type: "room.join",
    payload: RoomJoinRequest(roomId: "test-room")
)
let data = try JSONEncoder().encode(req)
try await webSocketTask.send(.string(String(data: data, encoding: .utf8)!))

// 等待响应（推荐：用一个 requestId → CheckedContinuation 的 map 匹配）
let resp: WSResponse<RoomJoinResponse> = ...
if resp.ok, let payload = resp.payload {
    // UI 填充 members
} else if let error = resp.error {
    // 展示 error.message 或按 error.code 分支
}
```

---

## 4. 消息 ②：`action.update`

**用途：** 上传当前动作（如 `"walking"` / `"sitting"` / 任意自定义字符串）。服务端会更新你在房间里的状态，并把一条 `action.broadcast` 推送给房间里**其他**成员（不含你自己）。

**方向：** up（upstream request + empty ack response）
**幂等：** **不幂等** —— 每次 `action.update` 都会重新广播一次。客户端可以重放同一个 action 作为"心跳式"广播，但要注意这会给房间里其他成员制造大量重复推送。

### 4.1 Request payload

```json
{ "action": "<string, 1-64 bytes>" }
```

| 字段 | 类型 | 约束 |
|---|---|---|
| `action` | string | 1-64 字节（UTF-8 byte length），必填 |

### 4.2 Response payload（ok）

```json
{}
```

空对象 `{}`（不是 `null`）。仅用于确认"服务端已收到并处理"。

### 4.3 Errors

| code | message | 原因 |
|---|---|---|
| `VALIDATION_ERROR` | `action required` | `action` 字段缺失或为空串 |
| `VALIDATION_ERROR` | `action exceeds 64 bytes` | 超过 64 字节 |
| `VALIDATION_ERROR` | `action must be valid UTF-8` | 非合法 UTF-8 |
| `VALIDATION_ERROR` | `invalid action.update payload` | payload JSON 解析失败 |
| `VALIDATION_ERROR` | `user not in any room` | **你没 `room.join` 过就直接 `action.update`** —— 客户端必须保证 join 先于 update |

`UNKNOWN_MESSAGE_TYPE`：服务端 release 模式下 `action.update` 未注册会返回此错误；debug 模式下正常情况不应出现。

### 4.4 Swift Decodable 建议

```swift
struct ActionUpdateRequest: Encodable {
    let action: String
}

// Response payload 是空对象 — 不需要专门的 struct
// 直接检查 WSResponse<EmptyPayload>.ok 即可
struct EmptyPayload: Decodable {}
```

### 4.5 `VALIDATION_ERROR: user not in any room` 处理建议

这是最常见的应用层错误。常见触发路径：
- app 刚启动，还没发 `room.join`
- 网络闪断后重连，客户端**没重发 `room.join`**（服务端只有 WS 会话断开→自动 leave，重连后状态归零）

**客户端处理建议：** 收到此错误时自动触发一次 `room.join`（用上次记住的 `roomId`），成功后重试 `action.update`。或者更彻底：把"reconnect → join → 以队列式 flush 缓存的 action"写成一个状态机。

---

## 5. 消息 ③：`action.broadcast`

**用途：** 服务端主动推送，携带房间里**别人**的最新 action。**你永远不会收到自己的 broadcast**（服务端严格过滤 self）。

**方向：** down（server → client push；**客户端绝对不能上行发送此消息**，Dispatcher 也没注册 handler）

### 5.1 Push payload

```json
{
  "userId": "<string>",
  "action": "<string>",
  "tsMs":   1712345678901
}
```

| 字段 | 类型 | 语义 |
|---|---|---|
| `userId` | string | 发送方的 userID |
| `action` | string | 发送方本次上报的 action 字符串（原样透传，未裁剪） |
| `tsMs` | int64 | **服务端时钟**在处理 `action.update` 那一刻的 UnixMilli（非客户端发送时间） |

### 5.2 下行推送信封

```json
{
  "type": "action.broadcast",
  "payload": { ... }
}
```

⚠️ **没有 `id`、没有 `ok`、没有 `error`**（与响应信封不同）。客户端判定规则建议：

```swift
// 伪代码
if let ok = json["ok"] as? Bool {
    // 这是对某个 request 的响应（ok==true/false）
} else {
    // 这是下行推送（action.broadcast 或未来其他 Direction=down 消息）
    switch json["type"] as? String {
    case "action.broadcast":
        // 解码 ActionBroadcastPush
    default:
        // 未知下行推送类型 —— 忽略或上报
    }
}
```

### 5.3 Swift Decodable 建议

```swift
struct WSDownstreamPush<P: Decodable>: Decodable {
    let type: String
    let payload: P
}

struct ActionBroadcastPush: Decodable {
    let userId: String
    let action: String
    let tsMs: Int64
}
```

### 5.4 时钟漂移说明

`tsMs` 是**服务端**时钟（Go `time.Now().UnixMilli()`，UTC）。客户端如果要显示"X 秒前"这样的相对时间，建议：

- **优先用客户端收到该帧的时刻** 作为 "now" 基准（避免服务端/客户端时钟漂移引发的"未来时间"显示）
- 仅把 `tsMs` 用作服务端排序 / 去重的标识，不作为 UI "多久前" 的精确数字

---

## 6. 典型场景时序

### 6.1 场景 A：Alice 首次加入空房间

```
Alice ──▶ [room.join {roomId: "test"}]  ──▶ Server
Alice ◀── [room.join.result {roomId: "test", members: []}]
```

Alice 看到 members 是 `[]`，说明她是房间第一个人。

### 6.2 场景 B：Bob 加入 Alice 的房间，然后 Alice 发 action

```
Bob   ──▶ [room.join {roomId: "test"}]   ──▶ Server
Bob   ◀── [room.join.result {roomId: "test", members: [{userId:"alice", action:"", tsMs:0}]}]
                                              (Alice 还没发过 action，所以空串 + 0)

Alice ──▶ [action.update {action: "walking"}] ──▶ Server
Alice ◀── [action.update.result {}]
Bob   ◀── [action.broadcast {userId:"alice", action:"walking", tsMs:1712345678901}]
          (Alice 不收自己的广播)
```

### 6.3 场景 C：Carol 后加入，snapshot 里带回 Alice 最新 action

```
Carol ──▶ [room.join {roomId: "test"}] ──▶ Server
Carol ◀── [room.join.result {
    roomId: "test",
    members: [
        {userId:"alice", action:"walking", tsMs:1712345678901},
        {userId:"bob",   action:"",        tsMs:0}
    ]
}]
```

Carol 从 snapshot 直接知道 Alice 的最新状态，无需等下一次 Alice 发 `action.update`。

### 6.4 场景 D：Bob 断开连接

```
Bob's WebSocket closes (network drop / app background / Watch sleep)
       │
       ▼
Server unregisters Bob from Hub → RoomManager.OnDisconnect fires
       │
       ▼
Server removes Bob from room; if room becomes empty, GC the room
       │
       ▼
(Alice / Carol 收不到任何显式通知 —— MVP 没有 member.leave 广播；
 她们只在下次 room.join 拉新 snapshot 时发现 Bob 没了)
```

**客户端感知"谁离开了房间"的建议（MVP 期）：** 定期 re-join（比如每 30 秒）刷新 members 列表，比较前后差集。或等 Epic 4.1 上线用正式的 `friend.offline` 推送。

### 6.5 场景 E：Alice 切房间

```
Alice ──▶ [room.join {roomId: "room-a"}] ──▶ Server
Alice ◀── [room.join.result ...]
...
Alice ──▶ [room.join {roomId: "room-b"}] ──▶ Server
       (服务端先把 Alice 从 room-a 移除；如果 room-a 空了就 GC；
        然后加入 room-b，返回 room-b 的 members snapshot)
Alice ◀── [room.join.result {roomId: "room-b", members: [...]}]
```

---

## 7. 错误处理与重试策略

### 7.1 错误分类

| 错误类型 | 处理建议 |
|---|---|
| **协议层** 握手失败（401/403/429） | 按 HTTP 状态码分支 —— 401 需要换 token、403 设备被封、429 指数退避重连 |
| **协议层** WS 帧解析失败 | 服务端返回 `{"id":"","ok":false,"type":".result","error":{"code":"VALIDATION_ERROR","message":"invalid envelope format"}}` —— 客户端应日志告警、不重试原帧 |
| **业务层** `VALIDATION_ERROR: user not in any room` | 自动 rejoin（详见 §4.5） |
| **业务层** `VALIDATION_ERROR: <字段> required / exceeds / ...` | 本地参数校验漏了 —— 修 bug，不该到这一步 |
| **业务层** `UNKNOWN_MESSAGE_TYPE` | 服务端不是 debug 模式，或服务端没升级到带本 MVP 的版本 —— 不要重试 |
| **连接层** WS 断开 | 指数退避重连 → 重新 `room.join` → flush 本地缓存的 action 队列 |

### 7.2 重连策略建议

```
第 1 次：立即重连
第 2 次：1s 后重连
第 3 次：2s 后重连
第 4 次：5s 后重连
第 5 次及以后：10s 固定间隔
```

每次重连成功后**必须重新 `room.join`**（服务端的 RoomManager 是纯内存状态，断开即清零）。

### 7.3 本地缓存 action 的建议

如果在断开期间用户又发了 action（比如 watch 后台仍有动作检测），建议客户端：
- **合并策略：** 只保留最后一次 action（旧 action 已过时）—— 这是服务端语义匹配的做法
- **不建议**缓存一串 action 等恢复后 flush 全部 —— 服务端不会把中间状态广播出去，其他人只会看到最新那条

---

## 8. 本 MVP**不做**的东西（重要契约）

客户端不要依赖下面这些行为——它们在 Epic 4 才会实现：

| 能力 | 本 MVP 不做 | 对应 Epic |
|---|---|---|
| 持久化房间成员 | 服务端重启后所有房间清零 | Epic 4.2 |
| 4 人上限 | 房间可以进无限多人 | Epic 4.2 |
| 断开宽限期（D8） | 断开即离开，不等 8 秒 | Epic 4.1 |
| `member.join` / `member.leave` 广播 | 不通知他人有人加入 / 离开 | Epic 4.1 |
| `friend.online` / `friend.offline` 广播 | 同上 | Epic 4.4 |
| `session.resume` 返回房间 snapshot | `session.resume` 响应里 `room: null` | Epic 4.5 |
| 跨服务器实例广播 | 仅同一 Go 进程内的客户端能互相看到 | Epic 4.3 |
| per-action rate limit | 仅靠 Hub 底层 100 msg/s/conn 限流兜底 | Epic 5.3 pattern |

**客户端在 MVP 阶段典型的"替代方案"**：

- 需要感知成员变动 → 周期性 re-join 对比前后 snapshot
- 需要断开感知 → 自己的 WS 断开即视为已离线；他人的需等下次 re-join 发现其不在 snapshot
- 需要跨服务器 → 联调阶段只跑单进程，不要部署多实例

---

## 9. 联调检查清单

开始联调前，客户端开发者请逐项确认：

- [ ] 服务端 `[server].mode = "debug"`，`GET /v1/platform/ws-registry` 能看到 3 条 Story 10.1 消息
- [ ] WS URL 填对（`ws://` vs `wss://`，主机、端口、`/ws` 路径）
- [ ] `Authorization: Bearer <非空字符串>` header 已设置；各客户端用不同 token（区分日志）
- [ ] 每条上行请求 `id` 字段全局唯一（推荐 UUIDv4）
- [ ] 客户端能区分"响应帧"（有 `id`/`ok`）和"推送帧"（只有 `type`/`payload`）
- [ ] 处理 `VALIDATION_ERROR: user not in any room` → 自动 rejoin 已实现
- [ ] 断线重连 → 重新 `room.join` 已实现
- [ ] 真机 watchOS / iOS 的 ATS 白名单已配置（如服务端用 HTTP）或已部署 HTTPS 环境

---

## 10. 版本演进与 Epic 4.1 迁移预告

### 10.1 本 MVP 的生命周期

本接口是**刻意临时**的。Epic 4.1 上线时：
- 服务端的 `internal/ws/room_mvp.go` + `dto.WSMessages` 里 3 条 DebugOnly 条目会被**整块删除**
- 不是"翻 `DebugOnly` flag 转正"——正式版**从头重写**，因为 MVP 故意不做的东西（持久化、4 人上限、D8 宽限、presence lifecycle、session.resume 联动、跨实例广播）全部需要重新设计

### 10.2 Epic 4.1 上线时客户端**需要**改什么（当前预期）

- 消息名**可能**换（`room.join` 可能变成 `room.enter` 或 `presence.enter`，具体由 Epic 4 设计档案定）
- `room.join.result` payload 将增加 `memberCap` / 在场状态 / 上线时间等字段
- 新增 `member.join` / `member.leave` / `friend.online` / `friend.offline` 下行推送
- 新增 `session.resume` 响应里的 `room` snapshot 字段
- `action.update` / `action.broadcast` 语义可能整体被重构为更通用的"状态同步" RPC（目前规划未定）

### 10.3 减少迁移痛的写法建议

- 客户端对接代码**不要散落**在 UI 层；集中封装一层 `RoomSession` / `ActionStream` 对象，Epic 4.1 到时只换这一层内部实现
- Swift Decodable struct 建议带 `@propertyWrapper` 兼容字段增减（Apple 的 `CodingKeys` 忽略未知字段，但多个字段用 custom decoder 会更脆弱）
- 客户端**不要硬编码** `"room.join"` / `"action.update"` 字面量散落在业务代码里；抽成常量
- 客户端**不要**依赖"服务端不会推送 `member.join`"这种**负面契约**——Epic 4.1 上线会推送 → 今天客户端要能对未知下行推送 type 做 graceful ignore

### 10.4 迁移期如何协调

Epic 4.1 开发启动时，后端会：
1. 先部署一个**带 Epic 4 正式消息 + 保留 Story 10.1 MVP 消息**的过渡版本
2. 客户端按节奏切到新消息
3. 客户端切完后再部署"删除 MVP 消息"的版本

详细迁移时间线由 Epic 4.1 story 创建时再同步；本文档不保证该时间线。

---

## 11. Sign in with Apple 客户端流程（Story 1.1）

> **新增于 Story 1.1**：服务端正式支持 Sign in with Apple，签发 per-device 的 access + refresh JWT。本节描述客户端从「拿到 Apple identityToken」到「持有可用 JWT」的完整流程。

### 11.1 端点

```
POST /auth/apple    ← 无鉴权 bootstrap，挂在 /v1/* JWT group 之外
```

请求体（`Content-Type: application/json`）：

```json
{
  "identityToken":     "eyJhbGciOiJSUzI1NiIs...",
  "authorizationCode": "c_xxx (可选, MVP 不消费)",
  "deviceId":          "f47ac10b-58cc-4372-a567-0e02b2c3d479",
  "platform":          "watch",
  "nonce":             "32-byte-base64-encoded-random-string"
}
```

成功响应（HTTP 200）：

```json
{
  "accessToken":  "eyJhbGciOiJSUzI1NiIs... (~15min 有效)",
  "refreshToken": "eyJhbGciOiJSUzI1NiIs... (~30day 有效)",
  "user": {
    "id":          "uuid-v4-string",
    "displayName": null,
    "timezone":    null
  }
}
```

错误响应（HTTP 400 / 401 / 500）：

```json
{ "error": { "code": "AUTH_INVALID_IDENTITY_TOKEN", "message": "invalid identity token" } }
```

完整错误码语义见 [`docs/error-codes.md`](../error-codes.md)。

### 11.2 客户端流程（iOS / watchOS 通用）

1. **生成 raw nonce + 计算 SHA-256 hex**
   ```swift
   import CryptoKit
   func randomNonce(length: Int = 32) -> String {
       var data = Data(count: length)
       _ = data.withUnsafeMutableBytes { SecRandomCopyBytes(kSecRandomDefault, length, $0.baseAddress!) }
       return data.base64EncodedString()
   }
   func sha256Hex(_ s: String) -> String {
       SHA256.hash(data: Data(s.utf8)).map { String(format: "%02x", $0) }.joined()
   }
   let rawNonce = randomNonce()
   let hashedNonce = sha256Hex(rawNonce)
   ```

2. **发起 Apple SIWA 授权请求**，把 `hashedNonce` 作为 `nonce` 参数传 Apple：
   ```swift
   let provider = ASAuthorizationAppleIDProvider()
   let request = provider.createRequest()
   request.requestedScopes = [.fullName, .email]   // 仅 first sign-in 返回这两项
   request.nonce = hashedNonce                     // ← 注意：传 hash 给 Apple
   let controller = ASAuthorizationController(authorizationRequests: [request])
   controller.delegate = self
   controller.performRequests()
   ```

3. **拿到 Apple 回调中的 `identityToken`**：
   ```swift
   func authorizationController(controller: ASAuthorizationController,
                                didCompleteWithAuthorization authorization: ASAuthorization) {
       guard let credential = authorization.credential as? ASAuthorizationAppleIDCredential,
             let tokenData = credential.identityToken,
             let identityToken = String(data: tokenData, encoding: .utf8) else { return }
       // identityToken 的 nonce claim 是服务端期望的 sha256(rawNonce) 的 hex；服务端会自己重做这一步比对
       sendToServer(identityToken: identityToken, rawNonce: rawNonce)
   }
   ```

4. **生成 / 复用 deviceId**（**首次启动生成 UUID v4 写入 Keychain**，后续登录复用）：
   ```swift
   func loadOrCreateDeviceID() -> String {
       if let existing = Keychain.string(for: "cat.deviceId") { return existing }
       let id = UUID().uuidString
       Keychain.set(id, for: "cat.deviceId")
       return id
   }
   ```

5. **POST `/auth/apple`**，把**原始 nonce**（不是 hash）作为 `nonce` 字段：
   ```swift
   let body: [String: Any] = [
       "identityToken": identityToken,
       "deviceId":      loadOrCreateDeviceID(),
       "platform":      "watch",     // iOS app 传 "iphone"
       "nonce":         rawNonce      // ← 原始 nonce, 不是 hash
   ]
   var request = URLRequest(url: URL(string: "https://api.example.com/auth/apple")!)
   request.httpMethod = "POST"
   request.setValue("application/json", forHTTPHeaderField: "Content-Type")
   request.httpBody = try JSONSerialization.data(withJSONObject: body)
   let (data, response) = try await URLSession.shared.data(for: request)
   ```

6. **持久化 accessToken / refreshToken 到 Keychain**（per-device 隔离）：
   - `cat.accessToken`、`cat.refreshToken` 加 access group 跨 watch / phone 时**不要共享**——它们是 per-device 的，watch 与 phone 分别独立登录。
   - 后续业务请求加 header：`Authorization: Bearer <accessToken>`
   - access token 过期（约 15 分钟）→ 用 refresh token 调 `POST /auth/refresh`（Story 1.2 上线）

### 11.3 关键约束（很容易踩坑）

| 字段 | 客户端发什么 | 服务端期望什么 |
|---|---|---|
| `nonce` | **原始** raw nonce | 自己 SHA-256 后比 `claims.nonce` |
| Apple SIWA `request.nonce` | `sha256(rawNonce)` 的 hex | Apple 把它原样写进 token 的 `nonce` claim |
| `deviceId` | UUID v4，存 Keychain，**不变** | 用于 Story 1.2 refresh / 1.4 APNs 绑定 |
| `platform` | `"watch"` 或 `"iphone"`，小写 | enum 校验，其他值 → `VALIDATION_ERROR` |
| `aud` | （客户端无需关心；由 Apple 自动塞 Bundle ID） | 服务端配置 `apple.bundle_id` 必须与之精确一致 |

### 11.4 错误处理建议

| HTTP / Code | 客户端建议 |
|---|---|
| 200 | 持久化 token，进入主界面 |
| 400 `VALIDATION_ERROR` | 检查请求体字段；这是客户端 bug 而非用户问题 |
| 401 `AUTH_INVALID_IDENTITY_TOKEN` | 让用户重新发起 SIWA（Apple 可能让 user revoke 了授权 / token 过期） |
| 500 `INTERNAL_ERROR` | 客户端展示「服务暂不可用」，按指数回退重试 |

服务端**不**在错误响应里泄漏具体 `Cause` —— 「sub mismatch」「audience invalid」等只在服务端日志里，客户端只看到大类 code。这是 NFR-SEC 信息泄漏防御。

### 11.5 与 WS 通道的关系

- 本端点是**HTTP**，与 WS 通道完全独立。
- 拿到 access token 后，建立 WS 连接的 header 仍是 `Authorization: Bearer <accessToken>`（替换调试期的 `<userId>` bearer）。
- Story 1.3 上线 JWT middleware 后，所有 WS upgrade 都通过真实 JWT 校验；本指南的 §1.3 debug bearer 约定仅在 Story 1.3 之前有效。

---

## 12. Refresh token 使用流程（Story 1.2）

Story 1.2 上线 `POST /auth/refresh`，实现 **rolling-rotation + stolen-token reuse detection**。本节是客户端开发者实现 refresh 流程的**唯一权威来源**。

### 12.1 何时调用

- 客户端调 `/v1/*` 受保护 API 拿到 `401 AUTH_TOKEN_EXPIRED` 时：access token 已过期（~15 min 后必然发生），**客户端立即 POST `/auth/refresh`**，body = `{"refreshToken": "<Keychain 里存的 refresh>"}`。
- **不要**主动预刷新（no silent refresh on timer）—— 只在收到 `AUTH_TOKEN_EXPIRED` 时触发；避免撞上服务端 rolling-rotation 的正常重放。
- WS 升级失败（access 过期同样会被新版 JWT middleware 拒绝）时也走同一路径。

### 12.2 端点契约

- **Method**：`POST`
- **URL**：`/auth/refresh`（与 `/auth/apple` 同级，`bootstrap` 路由，**不**经 JWT middleware）
- **Content-Type**：`application/json`
- **Body**：`{"refreshToken": "<RS256 JWT 字符串>"}`
- **200**：`{"accessToken": "...", "refreshToken": "..."}`（**两把都换新**）
- **401 `AUTH_INVALID_IDENTITY_TOKEN`**：token 签名 / iss / alg / kid / exp / ttype 任一失败
- **401 `AUTH_REFRESH_TOKEN_REVOKED`**：token 已被吊销 / reuse detection / session 未初始化
- **500 `INTERNAL_ERROR`**：Redis / Mongo / 签名服务 fail-closed

### 12.3 成功流程（必须严格按步骤）

1. 读 Keychain 取出当前 `refreshToken`。
2. POST `/auth/refresh`，body 见 §12.2。
3. 收到 `200 {accessToken, refreshToken}`：
   - **立即**把**两把 token 一起**写回 Keychain（**旧 refresh 已失效**，不能留任何副本）。
   - 用新的 access token 重试刚才失败的 `/v1/*` 请求。
4. 重建 WS 连接时使用新的 access token（见 §11.5）。

### 12.4 失败处理 —— **核心契约**

| 服务端响应 | 含义 | 客户端必须做 |
|---|---|---|
| `401 AUTH_REFRESH_TOKEN_REVOKED` | 这把 refresh token 已死（可能服务端 rotate / 可能被盗重放） | **清 Keychain + 跳回 SIWA 登录流程**，不允许 silent 重试 |
| `401 AUTH_INVALID_IDENTITY_TOKEN` | 这把 token 本身无效（签名 / 过期 / ttype 错） | 同上：清 Keychain + 跳回 SIWA |
| `500 INTERNAL_ERROR` | 服务端自身挂了 | 展示「服务暂不可用」，可按指数回退重试**同一 token**（这是少数允许重试的情况 —— 服务端并未成功 rotate） |
| 网络层失败 | 请求未到达服务端 | 按 §7.2 重连策略重试**同一 token**，但**限制**重试次数（3 次以内） |

### 12.5 **绝对禁止** —— rolling-rotation 安全铁则

1. **客户端不得并发刷新**：多个请求同时拿到 `AUTH_TOKEN_EXPIRED` 时，**只允许一次** `/auth/refresh` 请求在途（mutex / actor / semaphore 任意实现），后续请求**等待第一次的结果**再用新 access 重试。两个并发刷新 = 其中一个命中 reuse detection = **用户被踢**。
2. **刷新失败不得重试同一 token（除 500）**：服务端 reuse detection 已把当前 jti 吊销，同一 token 再试必然 401 并把活着的新 token 也烧掉。
3. **Keychain 写必须原子**：新的 access + refresh **一起**替换旧值，不能先写 access 再写 refresh（部分写入 + 崩溃 = 下次用混配）。
4. **不得把 refresh token 记录到除 Keychain 以外的位置**（日志 / 埋点 / crash report 全部禁止）。

### 12.6 端侧独立：Watch 和 iPhone 各自管理

- Watch 与 iPhone **各自独立** SIWA，各自独立 `deviceId`，各自独立 refresh token。
- 一台设备的 refresh 失败 **不影响**另一台；用户在 Watch 被踢下线时 iPhone 仍然登录着，反之亦然（FR5）。
- **禁止**跨端共享 refresh token（任何形式的 iCloud Keychain 同步都必须关闭 —— refresh token **不是** shared credential）。

### 12.7 观察性

- 服务端每次 refresh 会落一条 `action=refresh_token` 审计日志（不含任何 token 原文）。
- reuse detection 命中时落 `action=refresh_token_reuse_detected` **Warn**级别日志，含 `oldJti` + `currentJti`，便于后端排查客户端并发 bug / 可能的 token 泄漏。
- 客户端**不**需要向后端上报 refresh 相关事件（服务端审计足够）。

---

## 13. HTTP 鉴权流程（Story 1.3）

Story 1.3 上线 `internal/middleware/jwt_auth.go`，挂在 `/v1/*` 路由组上。本节是客户端开发者实现「每次调 `/v1/*` 时怎么带 access token」的**唯一权威来源**。

### 13.1 哪些 endpoint 需要带 Bearer

- **需要**：`/v1/*` 下所有 endpoint（Story 1.4 起逐步上线 `POST /v1/devices/apns-token`、`DELETE /v1/users/me`、`POST /v1/state` 等）。
- **不需要**（bootstrap，**显式排除**）：
  - `POST /auth/apple`（凭证就是 body 里的 Apple identityToken）
  - `POST /auth/refresh`（凭证就是 body 里的 refresh token）
  - `GET /v1/platform/ws-registry`（pre-auth 协议探针；客户端启动后还没登录就要查 server 支持哪些 WS 消息类型）
  - `GET /healthz` / `GET /readyz`（infra 探针）
  - `GET /ws`（鉴权在 upgrade handler 内部完成，与本节路径相同 token，不重复挂 middleware）

### 13.2 Header 格式

```
Authorization: Bearer <accessToken>
```

- 必须**严格** `Bearer`（大小写不敏感，但建议保持 `Bearer` 首字母大写）+ **单个空格** + 完整 access token 字符串。
- token 末尾不要带换行 / 多空格（服务端会 `TrimSpace` 但仍建议客户端先 trim）。
- **不要**把 access token 拼到 query string / cookie / 自定义 header。

### 13.3 失败响应矩阵

| 服务端响应 | 触发条件 | 客户端必须做 |
|---|---|---|
| `401 AUTH_TOKEN_EXPIRED` | header 缺失 / 非 Bearer / Bearer 后空 token | 走 §12 refresh 流程；服务端把「无凭证」与「access 过期」**统一**映射到此码 |
| `401 AUTH_INVALID_IDENTITY_TOKEN` | token 验签失败 / iss / alg / kid / exp 不通过 / `ttype != "access"` / claim 残缺（uid 或 deviceId 为空） | 走 §12 refresh；若 refresh 也 401 → 清 Keychain 跳回 SIWA |
| `403 DEVICE_BLACKLISTED` | 设备在 abuse 拦截名单 | 中止业务流程，提示用户联系支持；**不要**重试 |
| `429 RATE_LIMIT_EXCEEDED` | 触发 per-user / per-route 限流 | 按 `Retry-After` header 退避后重试 |
| `5xx INTERNAL_ERROR` | 服务端 fail-closed（Redis / Mongo / 签名服务挂） | 按 §7.2 重连/退避策略重试；**不需要**刷新 token |

### 13.4 **绝对禁止** —— rolling-rotation 安全铁则的延伸

1. **绝对不要**把 refresh token 当 Bearer 发给 `/v1/*`。服务端 middleware 校验 `claims.TokenType == "access"`，refresh token 会被拒为 `AUTH_INVALID_IDENTITY_TOKEN` 并丢失下次 refresh 的机会（与 §12.5 一致）。
2. **不要**手工构造 / 篡改 access token。任何 claims（uid / did / plat）由服务端签发，客户端**只读**。
3. **不要**把 access token 写进除 Keychain / 内存以外的位置（日志 / 埋点 / crash report 一律禁止）。
4. **不要**用 access token 做客户端逻辑判断的「身份信源」—— access token 是 opaque bearer，业务侧需要 userId 时从 `/auth/apple` 响应里 `user.id` 字段拿，不是从 token 里解析。

### 13.5 与 WS 鉴权的关系

- **同一把 access token** 既用于 HTTP `/v1/*`，也用于 WS `/ws` 升级（见 §11.5 / §2.1）。
- WS 升级一旦成功，连接**存续期间**服务端**不会**因 access token 过期而主动断开（性能与体验取舍：mid-connection re-verify 会抵消 session.resume cache 的意义）。
- 所以正常使用 flow 是：客户端建立 WS → 长连接里持续收发消息；HTTP `/v1/*` 调用按需 → 收到 401 时按 §13.3 处理（走 refresh + 重试 HTTP；**不需要**重连 WS）。
- 主动断开的唯一场景是账号注销，详见 §14。

### 13.6 观察性

- 服务端每次拒绝 `/v1/*` 请求落一条 `action=jwt_auth_reject` 审计日志，字段 `reason` 取值：`missing_header` / `not_bearer` / `empty_token` / `verify_failed` / `token_type_mismatch` / `claims_missing_uid` / `claims_missing_device_id`。
- 拒绝日志**不**带 userId（claims 未通过校验，不可信）；happy path 通过后续 access log 自然带上 userId。
- 客户端**不**需要向后端上报鉴权相关事件。

---

## 14. APNs device token 注册（Story 1.4）

Epic 1 第四个 story。负责把 Watch / iPhone 上**操作系统**下发的 APNs device token **绑定**到已登录用户账户上，让未来的 offline push（touch fallback / blindbox drop / cold-start recall）能精确送到这个设备。

### 14.1 Endpoint

- **URL**：`POST /v1/devices/apns-token`
- **Auth**：Bearer access token（即 `/auth/apple` 或 `/auth/refresh` 的返回值）——**不是** refresh token。走 Story 1.3 的 `/v1/*` JWT middleware。
- **Body**：
  ```json
  {
    "deviceToken": "<hex string — Apple 原样给什么就上报什么>",
    "platform": "watch"   // 可选：建议省略；服务端从 JWT 读
  }
  ```
- **Response 200**：`{ "ok": true }`
- **幂等性**：同 userId + 同 platform 连续调用会**覆盖**上次的 token（upsert）；跨 platform（Watch + iPhone）保留两条独立记录。

### 14.2 触发时机

客户端在以下任一情形调用：

1. 首次成功 SIWA 后，iOS / watchOS `didRegisterForRemoteNotificationsWithDeviceToken` 回调里拿到 token。
2. iOS / watchOS 下发**新** device token（Apple 保留随时换 token 的权利 —— 系统会再次回调上面那个 method）。
3. 用户在 App 设置里重新启用通知权限后系统重新下发 token。

**不要**在每次启动都调；没有 token 变化时调只是白烧服务端限流配额。

### 14.3 Body 字段细则

| 字段 | 必填 | 说明 |
|---|---|---|
| `deviceToken` | ✅ | Apple 原样给的 hex 字符串。**不要**做大小写转换 / 去空格 / 截断。典型长度 64 hex 字符（= 32 字节原始 token），服务端接受 8-200 字符区间（Apple 未来可能加长）。 |
| `platform` | ❌ | `"watch"` 或 `"iphone"`。**强烈建议省略** —— 服务端从 JWT `plat` claim 读，权威来源。若客户端执意携带，必须和 JWT 一致；不一致 → 400 `VALIDATION_ERROR`。 |

**不要**发明别的字段（没有 `appVersion` / `locale` / `badge` 等）；服务端 binding 用的是 `required,min=8,max=200,hexadecimal` 严格 validator，任何多余字段被 gin 忽略，但多余字段里的 typo 可能掩盖真正的 `deviceToken` 传错。

### 14.4 失败响应

| HTTP | Code | 客户端应对 |
|---|---|---|
| 400 | `VALIDATION_ERROR` | 客户端 bug —— `deviceToken` 不是 hex / `platform` 和 JWT 不匹配 / body 格式错。记 log，**不要**重试；联系后端。 |
| 401 | `AUTH_TOKEN_EXPIRED` | access token 缺失 / 过期 —— 走 Story 1.2 refresh flow，成功后**重试一次**（同一个 deviceToken）。 |
| 401 | `AUTH_INVALID_IDENTITY_TOKEN` | access token 损坏 / 错类型 / 无 platform claim（极罕见，仅在 JWT pre-1.2 无 plat 场景）。走 refresh flow；若 refresh 也 401 则引导用户回 SIWA 登录。 |
| 429 | `RATE_LIMIT_EXCEEDED` | 读 `Retry-After` header（秒），按其值退避后重试。默认阈值 5 次 / 60 秒，已经远高于正常客户端触发频率；触发 429 一般意味着客户端实现 bug。 |
| 500 | `INTERNAL_ERROR` | 服务端 Redis / Mongo 故障。指数退避重试最多 3 次（1s / 3s / 9s），仍失败则放弃 —— 下一次 token 变化（或 App 重启）会重新触发注册。 |

### 14.5 安全与隐私

- **服务端**：device token 在 Mongo 以 AES-256-GCM 字段级加密存储（NFR-SEC-7），任何落库备份 / 数据迁移导出的密文离开原 key 都不可解密。
- **日志**：服务端任何 INFO+ 日志里的 device token 都走 `logx.MaskAPNsToken`（首 8 字符 + `...`）。客户端**同样建议** —— 不要把完整 deviceToken 打到 Crashlytics / Sentry 等远端日志。
- **重置**：用户在系统设置里关闭通知权限 → iOS / watchOS 不再回调 → 服务端会在下一次 APNs 推送拿到 `410 Unregistered` 响应时自动清除该 row（Story 0.13 FR43）。客户端**不需要**主动调"删除 token" API；Story 1.6 账号注销会一并清。
- **跨设备**：同一 Apple Account 在 Watch + iPhone 两台设备上**独立**存储两条 row，推送时服务端会同时往两者发（Story 5.2+ 场景）。

### 14.6 推荐的 Swift 伪代码

```swift
// 1. 在 SIWA 成功、且 access token 拿到以后注册 remote notifications。
UIApplication.shared.registerForRemoteNotifications()

// 2. 在 didRegisterForRemoteNotificationsWithDeviceToken 里
func application(_ app: UIApplication,
                 didRegisterForRemoteNotificationsWithDeviceToken deviceToken: Data) {
    let hex = deviceToken.map { String(format: "%02x", $0) }.joined()
    Task {
        do {
            try await API.registerApnsToken(hex: hex)   // POST /v1/devices/apns-token
        } catch APIError.authTokenExpired, APIError.authInvalidIdentityToken {
            try? await AuthManager.refresh()            // Story 1.2
            try? await API.registerApnsToken(hex: hex)  // 重试一次
        } catch APIError.rateLimited(let retryAfter) {
            try? await Task.sleep(nanoseconds: UInt64(retryAfter) * 1_000_000_000)
            try? await API.registerApnsToken(hex: hex)
        } catch {
            // 500 等：交给下一次 token 变化 / App 重启
        }
    }
}

// 3. Body 不必带 platform —— 服务端从 JWT 读。
func registerApnsToken(hex: String) async throws {
    try await http.post("/v1/devices/apns-token", body: ["deviceToken": hex])
}
```

### 14.7 观察性

- 服务端成功注册会落 `action=apns_token_register` INFO 日志，字段含 userId / deviceId / platform（**不**含 deviceToken 明文）。
- Rate-limit 拒绝路径不带 userId 的独立 audit 日志 —— 直接在 Story 1.1 全局 logger 里以 `AppError` 形式输出 `code=RATE_LIMIT_EXCEEDED`。
- 客户端**不**需要额外向后端上报"已注册"事件。

---

## 15. Profile 更新与时区自动上报（Story 1.5）

### 15.1 端点（WS RPC）

- 类型：`profile.update`（WS；`Direction=bi`，RequireAuth=true，RequireDedup=true）
- 首次可用版本：openapi `1.5.0-epic1`
- 用户必须已完成 Story 1.1 SIWA 并持有有效 access token；WS 连接必须已通过鉴权（HTTP `Authorization: Bearer …`）或 debug 模式的 query-token fallback。

### 15.2 何时调用

1. 用户在 App 设置页修改 displayName、timezone 或 quietHours（勿扰时段）其中任意一个或多个字段。
2. **FR50 自动上报**：客户端（iOS + watchOS）**监听** `TimeZone.current` 的变化（`NSSystemTimeZoneDidChange` 通知或 `NotificationCenter.default.publisher(for: .NSSystemTimeZoneDidChange)`），一旦检测到变化，**主动**发送 `profile.update`，payload 仅含 `timezone` 字段（不动 displayName / quietHours）。

> 服务端**不**区分"用户手动设置"与"自动上报" —— 同一个 endpoint 两种场景都走。差异仅在客户端 UI 是否展示了确认对话框。

### 15.3 Request payload（最小示例：三字段全部）

```json
{
  "id": "d5f0c6d0-0000-4000-8000-abc123456789",
  "type": "profile.update",
  "payload": {
    "displayName": "Alice",
    "timezone": "Asia/Shanghai",
    "quietHours": { "start": "23:00", "end": "07:00" }
  }
}
```

- 三个字段**都 optional** —— 可以只发其中任一。**至少一个**非空，否则服务端返 `VALIDATION_ERROR`。
- `displayName`：trim 后 UTF-8 字符数 ∈ [1,32]，不含 ASCII 控制字符；服务端会 trim 前后空白再落库。
- `timezone`：IANA 时区（`time.LoadLocation` 能解析：`Asia/Shanghai` / `America/New_York` / `UTC` / `Europe/London` / …）。
- `quietHours.start` 与 `quietHours.end`：24h 制 `HH:MM`，锁死 `00:00-23:59`。
  - 区间**左闭右开**：`[start, end)` —— start 那一分钟算 quiet，end 那一分钟不算 quiet（设 07:00 结束 ⇒ 07:00 响）。
  - `start == end` 合法，表示"24 小时静默"。
  - `start > end` 表示跨日窗口（如 `23:00-07:00`）。

### 15.4 Response payload（ok）

```json
{
  "id": "d5f0c6d0-0000-4000-8000-abc123456789",
  "ok": true,
  "type": "profile.update.result",
  "payload": {
    "user": {
      "id": "u-uuid",
      "displayName": "Alice",
      "timezone": "Asia/Shanghai",
      "preferences": {
        "quietHours": { "start": "23:00", "end": "07:00" }
      }
    }
  }
}
```

客户端**应该**用 response.payload.user 整条替换本地缓存 —— 这是 authoritative post-write 的最新快照（在某些实现细节上可能比你请求时发的字段更规范化，比如 displayName 已 trim）。

### 15.5 Errors

- `VALIDATION_ERROR` —— `at least one of displayName/timezone/quietHours must be provided` / `displayName must be at least 1 character after trim` / `timezone %q is not a valid IANA zone` / `quietHours.start %q must be HH:MM ...` / `quietHours requires timezone to be set; include 'timezone' in this request or set it before updating quietHours` / `invalid profile.update payload`（JSON decode 失败）。客户端**不要**重试同一 payload，需要用户修正输入。
  > **quietHours ↔ timezone 耦合（1.5.1 起）**：fresh-SIWA 用户 `timezone=null`，服务端会拒绝"只发 quietHours"的请求（因为 APNs quiet-hours 解析器在 tz 缺失时短路返 "not quiet"，静默失效）。客户端要么**先**单独发 `timezone` 然后再发 `quietHours`，要么**同一请求**带上两者。FR50 自动时区上报本来就应该在首次 SIWA 后立即触发，通常这个耦合对用户不可见。
- `EVENT_PROCESSING` —— 同一 `envelope.id` 正在处理或已处理过；替换为新 `envelope.id` 重发（非幂等错误）。
- `INTERNAL_ERROR` —— 服务端 Mongo 写失败或其他不可恢复故障。客户端以指数退避（1s → 3s → 10s）最多重试 3 次，仍失败则提示用户稍后再试。

### 15.6 默认 quietHours

新账户首次 SIWA 登录时，服务端默认写入 `quietHours = 23:00-07:00`（`domain.DefaultPreferences` 的 seed 值）。客户端 UI 在用户首次覆盖前应**显示默认值**（可通过 `session.resume` 的 `user.preferences.quietHours` 拿到）。

### 15.7 Session.resume 与 profile.update 的关系

- `session.resume` 的 `user.*` 字段（含 `preferences.quietHours`）被一层 **60s TTL Redis 缓存**（Story 0.12）。
- `profile.update` 成功后，服务端**同步 invalidate** 该用户的 resume cache —— 同一 userId 下次 session.resume 必然重建快照，反映最新 profile。
- 因此客户端**不需要**在 profile.update 成功后主动 resend session.resume（response 已带最新 user 字段）。

### 15.8 观察性

- 服务端成功落 `action=profile_update` INFO 日志，字段含 `userId` + `fields=[...]`（字段名枚举）。
- 服务端**绝不**记 displayName **原值**（PII §M13）。客户端日志可以记，但**不**建议上传未脱敏到后台分析系统。
- timezone / quietHours 原值服务端可能在排错时临时记录，非 PII。

### 15.9 Swift 伪代码（FR50 自动时区上报）

```swift
// 一次性注册监听
NotificationCenter.default.addObserver(
    forName: .NSSystemTimeZoneDidChange,
    object: nil, queue: .main
) { _ in
    let newTZ = TimeZone.current.identifier // e.g. "Asia/Shanghai"
    Task {
        do {
            try await wsClient.sendDedup(
                type: "profile.update",
                payload: ["timezone": newTZ]
            )
        } catch {
            // fail silently; 下次变化再尝试
        }
    }
}
```

> `wsClient.sendDedup` 必须生成**新的** `envelope.id`（UUID v4）并等待 `*.result` —— 与其他 `RegisterDedup` 消息一致。

---

## 16. 账户注销场景的 WS 主动断开（Story 1.6 预告）

Story 1.6 (DELETE `/v1/users/me`) 上线后会触发以下流程，本节让客户端**提前**做好准备：

1. 用户在 App 内确认「注销账号」。
2. 客户端 POST 到 `/v1/users/me` 的 DELETE（带 access token）。
3. 服务端：
   - 标记 `users.deletion_requested = true`（24h 后真正删除，期间内同一 Apple `sub` 再 SIWA 会触发 §11 的 resurrection 流程）。
   - 调 `RevokeAllUserTokens(userId)` —— 把该 userId 全部 device 的 refresh token 加入黑名单（Story 1.2）。
   - 调 `Hub.DisconnectUser(userId)` —— 把该 userId **所有**活动 WS 连接（可能跨 Watch + iPhone）发送 `Close 1000 "revoked"` 帧并关闭。
4. 客户端表现：
   - HTTP DELETE 收到 `200 OK`（或 200 + `{ok: true}` 类响应，最终 schema 等 Story 1.6 落地）。
   - 现有 WS 连接**几乎同时**收到 `CloseError{Code: 1000, Text: "revoked"}` —— 客户端**不要**当成网络异常自动重连。
5. 客户端必做：
   - 清空 Keychain 里所有 token（access + refresh）+ deviceId（重新登录会生成新的 deviceId）。
   - 跳回 SIWA 登录入口（如果用户决定再回来）。
   - **不要**尝试用旧 access token 重连 WS —— 服务端的 access blacklist 还没上线（Story 1.3 接受这个 ~100ms 量级的 race window，详见 server Story 1.3 Dev Notes）。

**已知 race window**：在服务端 `RevokeAllUserTokens` 完成与 `DisconnectUser` 完成之间的极短窗口内（同进程内基本不可见，跨实例可能 < 100ms），如果同一 userId 在另一台设备上**新建**了 WS 连接（access token 还没过期），可能会短暂连上来。客户端不需要为此做任何防御 —— 业务下游（profile / state / push）会查 Mongo 发现用户已 `deletion_requested = true` 自然 fail-closed。

---

## 附录 A：快速参考卡

| # | Type | Direction | 场景 |
|---|---|---|---|
| 1 | `room.join` | bi | 客户端进入 / 切换房间，收 members 快照 |
| 2 | `action.update` | up | 客户端上报当前动作，收空 ack |
| 3 | `action.broadcast` | down | 服务端推送其他成员的动作 |

**最短可运行 flow**（伪代码）：

```
connect ws://host/ws with Authorization: Bearer alice
  → send {"id":"1","type":"room.join","payload":{"roomId":"test"}}
  ← recv {"id":"1","ok":true,"type":"room.join.result","payload":{"roomId":"test","members":[]}}
  → send {"id":"2","type":"action.update","payload":{"action":"walking"}}
  ← recv {"id":"2","ok":true,"type":"action.update.result","payload":{}}
  ← recv {"type":"action.broadcast","payload":{"userId":"bob","action":"sitting","tsMs":...}}   ← 当另一个客户端 bob 在同房间发 action.update 时
```

## 附录 B：相关文档

- [`docs/api/ws-message-registry.md`](ws-message-registry.md) — 全部 WS 消息的权威 registry（含 Story 10.1 之外的消息）
- [`docs/api/openapi.yaml`](openapi.yaml) — HTTP 端点的 OpenAPI 3.0 规格（含 `GET /v1/platform/ws-registry`）
- [`docs/backend-architecture-guide.md`](../backend-architecture-guide.md) — 后端整体架构（§12 WebSocket 章节）
- [`server/human_docs/story-10-1-summary.md`](../../server/human_docs/story-10-1-summary.md) — 服务端实现总结（面向后端开发者）
- [`server/_bmad-output/implementation-artifacts/10-1-integration-mvp-room-and-action.md`](../../server/_bmad-output/implementation-artifacts/10-1-integration-mvp-room-and-action.md) — Story 规格 + AC 验收详单

## 附录 C：问题反馈

发现本文档与服务端实际行为不一致，**以服务端为准**，并请反馈：
- 附 WS 帧原文（request + response / push 完整 JSON）
- 附服务端日志里对应的 `conn_id`
- 附 `GET /v1/platform/ws-registry` 当前响应
