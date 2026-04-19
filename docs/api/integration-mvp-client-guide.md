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
