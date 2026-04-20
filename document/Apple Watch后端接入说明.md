# Apple Watch 后端接入说明

## 1. 这份文档解决什么问题

这份文档不是单纯复述后端协议，而是把：

- 当前 `server/` 里已经可用的联调能力
- 当前 `ios/CatWatch` 已有的本地状态代码
- Apple Watch 端后续应该如何接入后端

整理成一份可直接执行的接入说明。

适用范围是当前这版后端的 **联调 MVP**，不是最终正式联机协议。

---

## 2. 当前后端到底提供了什么

当前后端里，和 watch 联调直接相关的能力主要是：

1. `GET /v1/platform/ws-registry`
2. `GET /ws` WebSocket 升级
3. `room.join`
4. `action.update`
5. `action.broadcast`

关键限制：

- 这 3 条联机消息目前都是 **DebugOnly**
- 服务端必须跑在 `debug` 模式
- `release` 模式下这几条消息不会注册
- WebSocket 的鉴权在 `debug` 模式下很特殊：`Authorization: Bearer <任意非空字符串>` 就能通过，而且这个字符串本身会被当成 `userId`

对应后端文件：

- [initialize.go](/Users/boommice/catc/server/cmd/cat/initialize.go)
- [upgrade_handler.go](/Users/boommice/catc/server/internal/ws/upgrade_handler.go)
- [room_mvp.go](/Users/boommice/catc/server/internal/ws/room_mvp.go)
- [ws_messages.go](/Users/boommice/catc/server/internal/dto/ws_messages.go)
- [integration-mvp-client-guide.md](/Users/boommice/catc/docs/api/integration-mvp-client-guide.md)

---

## 3. 当前 watch 端现状

你现在的 watch 端还没有真正的后端同步层，核心还是本地逻辑：

- [CatWatchApp.swift](/Users/boommice/catc/ios/CatWatch/App/CatWatchApp.swift)
- [CatStateMachine.swift](/Users/boommice/catc/ios/CatShared/Sources/CatCore/CatStateMachine.swift)
- [LocalStore.swift](/Users/boommice/catc/ios/CatShared/Sources/CatShared/Persistence/LocalStore.swift)

当前职责大致是：

- `WatchMotionController`
  - 负责 CoreMotion / pedometer
  - 生成本地 `CatState`
  - 维护今日步数
- `CatStateMachine`
  - 负责 idle / walking / running / sleeping 切换
- `StandReminderManager`
  - 负责久坐提醒
- `BlindBoxManager`
  - 负责本地盲盒和点数

这意味着现在 watch 端是“单机版状态猫”，还没有：

- 后端配置层
- HTTP 探测层
- WebSocket 长连接层
- 房间会话层
- 好友猫状态仓库
- 本地状态到联机动作的同步桥接层

---

## 4. 先说结论：watch 端应该怎么接

当前这版后端，watch 端最合适的接法是：

1. **保留本地状态机为真相源**
2. **新增一层联机同步层**
3. **把本地状态变化映射成 `action.update`**
4. **把别人发来的 `action.broadcast` 映射成“好友猫状态”**

也就是说：

- 你的猫怎么动，仍然由本地 `WatchMotionController + CatStateMachine` 决定
- 后端目前不负责判定你的猫状态
- 后端现在只负责“把你的当前动作广播给同房间其他设备”

这是和当前服务端 MVP 最匹配的方式。

---

## 5. 推荐的 watch 接入分层

建议后续把 `CatWatch` 拆成下面这几层。

### 5.1 本地状态层

继续保留现有逻辑：

- `WatchMotionController`
- `CatStateMachine`
- `StandReminderManager`
- `BlindBoxManager`

这层不直接碰网络。

### 5.2 网络基础层

建议新增一个 `BackendConfig` + `BackendEnvironment` 概念，至少包含：

- `baseHTTPURL`
- `webSocketURL`
- `debugToken`
- `roomId`

然后新增一个轻量 HTTP 客户端，只做一件事：

- 请求 `/v1/platform/ws-registry`

用途：

- 启动时确认服务端地址通不通
- 确认当前服务端是不是 `debug`
- 确认 `room.join / action.update / action.broadcast` 是否真的可用

### 5.3 WebSocket 连接层

建议新增一个单独对象，比如：

- `WatchWebSocketClient`

职责：

- 建连 `/ws`
- 带上 `Authorization: Bearer <token>`
- 发送文本帧
- 接收文本帧
- 区分“响应帧”和“推送帧”
- 自动重连

这一层不直接理解业务，只处理传输。

### 5.4 房间会话层

建议新增：

- `WatchRoomSession`

职责：

- `connect()`
- `join(roomId:)`
- `sendAction(_:)`
- 处理 `room.join.result`
- 处理 `action.broadcast`
- 维护当前房间成员快照

这层才真正理解当前后端的 3 条 MVP 消息。

### 5.5 同步桥接层

建议新增：

- `WatchSyncCoordinator`

职责：

- 监听本地 `CatState`
- 监听连接状态
- 首次连接成功后自动 `room.join`
- 本地猫状态变化时，把状态映射成字符串并发 `action.update`
- 重连成功后自动 rejoin，并补发最近一次动作

这一层是“本地猫”和“联机后端”之间的桥。

### 5.6 好友状态层

建议新增：

- `FriendCatStore`

职责：

- 保存当前房间其他成员的最新状态
- 供四猫同屏 UI 使用

这层未来会直接服务你要做的多人同屏。

---

## 6. 当前版本最合理的文件落点

因为你现在 `CatWatch` 绝大部分逻辑都塞在 [CatWatchApp.swift](/Users/boommice/catc/ios/CatWatch/App/CatWatchApp.swift)，建议后续不要继续把网络代码也塞进去。

建议拆成下面这组文件：

- `ios/CatWatch/App/WatchHomeView.swift`
- `ios/CatWatch/App/WatchMotionController.swift`
- `ios/CatWatch/App/StandReminderManager.swift`
- `ios/CatWatch/App/BlindBoxManager.swift`
- `ios/CatWatch/App/Sync/BackendConfig.swift`
- `ios/CatWatch/App/Sync/RegistryClient.swift`
- `ios/CatWatch/App/Sync/WatchWebSocketClient.swift`
- `ios/CatWatch/App/Sync/WatchRoomSession.swift`
- `ios/CatWatch/App/Sync/WatchSyncCoordinator.swift`
- `ios/CatWatch/App/Sync/FriendCatStore.swift`
- `ios/CatWatch/App/Sync/Models/*.swift`

现在先不用一次性全拆完，但正式接后端时，至少应该先把同步层单独抽出来。

---

## 7. watch 端和后端的对接关系

### 7.1 启动时

watch 端启动后：

1. 读取联调环境配置
2. 请求 `GET /v1/platform/ws-registry`
3. 检查是否存在 `room.join`
4. 检查是否存在 `action.update`
5. 检查是否存在 `action.broadcast`
6. 如果都存在，再发起 WebSocket 连接

如果 `messages` 是空数组，基本就说明：

- 服务端跑在 `release`
- 或者你连错环境了

### 7.2 建立连接后

连接成功后：

1. 立刻发送 `room.join`
2. 收到 `room.join.result`
3. 用返回的 `members` 初始化 `FriendCatStore`

### 7.3 本地状态变化时

当本地猫状态变化时：

- `idle`
- `walking`
- `running`
- `sleeping`

同步层把它转换为动作字符串，然后发：

```json
{
  "id": "<uuid>",
  "type": "action.update",
  "payload": {
    "action": "walking"
  }
}
```

### 7.4 收到别人动作时

收到：

```json
{
  "type": "action.broadcast",
  "payload": {
    "userId": "phone-bob",
    "action": "running",
    "tsMs": 1712345678901
  }
}
```

就更新 `FriendCatStore` 里这个用户对应的猫状态。

---

## 8. 建议的状态映射

你现在本地状态枚举是：

- `idle`
- `walking`
- `running`
- `sleeping`
- `microYawn`
- `microStretch`

建议当前 MVP 先这样映射到后端 action 字符串：

| 本地 CatState | 上报给后端的 action |
|---|---|
| `idle` | `idle` |
| `walking` | `walking` |
| `running` | `running` |
| `sleeping` | `sleeping` |
| `microYawn` | `idle` |
| `microStretch` | `idle` |

原因：

- 后端目前只把 `action` 当普通字符串广播
- 但你多人 UI 第一版最好只围绕 4 个主状态做
- 微动作现在不适合直接进入联机协议，否则别人猫会频繁小抖动

后续如果真要同步微动作，再单独扩协议。

---

## 9. `session.resume` 现在要不要接

当前阶段，**不建议 watch 端把 `session.resume` 当重点能力来接**。

原因不是它不存在，而是它现在还是占位版本：

- 在 debug 下有注册
- 但内部 provider 大部分都是 `Empty*Provider`
- 返回的数据更像骨架，不是最终业务快照

这意味着：

- 可以把它留作后续扩展位
- 但现在不要把多人房间恢复、盲盒恢复、用户资料恢复都压在它上面

当前联调阶段，房间状态更靠谱的策略仍然是：

1. 重连成功
2. 重新 `room.join`
3. 用 `room.join.result` 重建其他成员快照
4. 再补发自己最近一次动作

---

## 10. watch 端最重要的重连策略

这部分非常关键，因为当前后端的房间是内存态。

当前 MVP 下：

- WebSocket 一断
- 服务端就会把这个连接从房间里移除
- 不存在后台自动保房间
- 不存在 8 秒宽限
- 不存在正式的 presence 恢复

所以 watch 端必须自己保证：

1. 断线后自动重连
2. 重连后自动 `room.join`
3. `room.join` 成功后自动补发最近一次 action

推荐流程：

```text
connect
  -> join
    -> joined
      -> local state changes
        -> action.update

socket closed
  -> reconnect
    -> join same room
      -> resend latest action
```

如果缺了第 2 步，你会经常收到：

- `VALIDATION_ERROR`
- `user not in any room`

---

## 11. 当前版本不要做的事

为了和后端 MVP 对齐，watch 端当前先不要做下面这些强依赖：

### 11.1 不要依赖成员离开广播

现在没有 `member.leave`。

所以 watch 端不要假设：

- 某个好友离开时服务端会主动告诉你

如果要知道房里谁还在，当前只能：

- 重连后重新 join
- 或者定时 rejoin 拉新 snapshot

### 11.2 不要依赖跨实例

现在房间管理是单进程内存态。

也就是说只有：

- 所有人连的是同一个后端进程

联机才成立。

### 11.3 不要把盲盒、点数、提醒先接后端

你 watch 端现在本地已经有：

- 点数
- 盲盒
- 提醒

但后端当前联调 MVP 并没有这些正式能力。

所以第一阶段接入建议只同步：

- 房间
- 动作
- 好友猫状态

不要同时把盲盒和点数也远程化，不然会把接入范围拉太大。

---

## 12. 对你当前代码的直接落点建议

### 12.1 `WatchMotionController` 继续只负责本地检测

[CatWatchApp.swift](/Users/boommice/catc/ios/CatWatch/App/CatWatchApp.swift) 里的 `WatchMotionController` 不要直接发网络请求。

它继续只做：

- CoreMotion
- pedometer
- 本地 `CatState`
- 本地步数变化

然后通过回调或 publisher 把状态抛给同步层。

### 12.2 `WatchHomeView` 不要直接操作 socket

`WatchHomeView` 现在是 UI 容器，后面也不应该直接持有 WebSocket 细节。

更合适的方式是：

- `WatchHomeView`
  - 持有 `WatchSyncCoordinator`
- `WatchSyncCoordinator`
  - 内部持有 `WatchRoomSession`
- `WatchRoomSession`
  - 内部持有 `WatchWebSocketClient`

### 12.3 四猫同屏直接吃 `FriendCatStore`

你之前已经做过四猫同屏效果验证，后续正式接后端时：

- 自己的猫：继续吃本地 `currentState`
- 其他 3 只猫：吃 `FriendCatStore`

这样 UI 层是稳定的，后面换正式协议时也只需要换同步层，不用重写整个界面。

---

## 13. 推荐的联调顺序

建议我们后面按这个顺序接，不容易乱。

1. 先加 `BackendConfig`
2. 再加 `RegistryClient`
3. 再加 `WatchWebSocketClient`
4. 先把 `debug.echo` 跑通
5. 再接 `room.join`
6. 再接 `action.update`
7. 再接 `action.broadcast`
8. 最后把 `FriendCatStore` 接到四猫 UI

这样每一步都能单独验证。

---

## 14. 本阶段最适合的最小目标

如果只定义一个“第一阶段完成标准”，我建议是这个：

### Apple Watch 联机最小闭环

1. watch 启动后检查 `/v1/platform/ws-registry`
2. watch 成功连上 `/ws`
3. watch 自动 `room.join`
4. watch 本地状态从 `idle -> walking -> running -> sleeping` 变化时，会发 `action.update`
5. 同房间另一端能收到 `action.broadcast`
6. watch 能显示至少 1 只好友猫的实时状态

做到这一步，后面的多人、头像、房间 UI、好友列表都可以往上加。

---

## 15. 一句话判断

当前这版后端对 watch 端最正确的接法，不是“把本地逻辑改成后端驱动”，而是：

**保留本地猫状态机，用 WebSocket 把本地状态同步成房间动作，再把别人动作渲染成好友猫状态。**

这和你现在的代码结构、也和服务端当前的 MVP 能力最匹配。
