# Story 10.1: 联调 MVP — 房间 + 动作 3 消息集 + 内存 RoomManager — 实现总结

让**真机 Apple Watch + iPhone** 客户端今天就能开始 WS 联调：实现 3 条 `DebugOnly: true` 消息（`room.join` / `action.update` / `action.broadcast`）+ 内存 `RoomManager`，不等 Epic 4.1-4.5 的完整 presence / 房间 / 持久化落地。Epic 4.1 上线时本 story 的核心文件（`internal/ws/room_mvp.go` + `WSMessages` 里 3 条 DebugOnly 条目）**整块 `git rm`**，不是翻 DebugOnly flag 转正——正式版从零重写。

**本 story 也给 `internal/ws/hub.go` 加了一个长期保留的基础设施**：`ClientObserver` 接口 + `Hub.AddObserver` + `notifyDisconnect` fan-out。这部分**不随 room_mvp.go 删除**——Epic 4.1 Presence 会是它的第二个消费者，同样需要"Hub 断开 → domain 清理"的 hook。

## 做了什么

### `dto.WSMessages` 追加 3 条（AC1）

`internal/dto/ws_messages.go` 新增 3 条 entries，全部 `DebugOnly: true`：

| Type | Version | Direction | RequiresAuth | RequiresDedup | DebugOnly |
|---|---|---|---|---|---|
| `room.join` | v1 | bi | true | false | true |
| `action.update` | v1 | up | true | false | true |
| `action.broadcast` | v1 | down | true | false | true |

`Description` 字段一律以 "MVP only, removed when Epic 4.1 ships" 结尾，对未来扫代码的人是明确信号。

**wire shape**（iOS `JSONDecoder` camelCase 约定）：

- `room.join` request：`{"roomId": "<string, 1-64 bytes>"}`
- `room.join.result`：`{"roomId": "<string>", "members": [{"userId": "<string>", "action": "<string>", "tsMs": <int64>}]}`（**不含自己**；空时为 `[]` 非 `null`）
- `action.update` request：`{"action": "<string, 1-64 bytes>"}`
- `action.update.result`：`{}`（空 ack）
- `action.broadcast` push：`{"userId": "<string>", "action": "<string>", "tsMs": <int64>}`

### `internal/ws/room_mvp.go` — 首次创建（AC2-5, AC10）

全文件约 350 行，30 行 package doc 明示 MVP 定位——两条**精确字面量**便于未来 grep：

- `"EXPECTED to be deleted wholesale when Epic 4.1 ships"`
- `"NOT a seed for Epic 4"`

package doc 还列出 6 条**故意不做**的东西（persistence / 4-person cap / D8 grace / session.resume 联动 / cross-instance / member.join / leave 广播），方便 Epic 4.1 开发者对账。

**三个核心类型**：

- `Member`：`UserID` / `ConnID` / `LastAction string` / `LastActionTs int64`（Unix ms，0 = 从未 update）
- `Room`：`ID RoomID` / `members map[UserID]*Member`
- `RoomManager`：`clock` / `broadcaster` / 3 个 map（`rooms` / `userLoc` / `connMap`）+ `sync.RWMutex`

**构造函数**：`NewRoomManager(clock, broadcaster)` 对 nil 两者 panic（§P3 fail-fast startup）。

**三个 handler + 1 个 observer 方法**：

- `HandleJoin(ctx, client, env)` —— AC3 四步：evict old room → ensure target room + 添加 self → snapshot 排除 self → 返回 `{roomId, members}`（`members` 始终序列化为 `[]` 非 `null`）
- `HandleActionUpdate(ctx, client, env)` —— AC4 五步：校验 → userLoc 未命中返 `VALIDATION_ERROR: "user not in any room"` → 更新 `member.LastAction` + `clock.Now().UnixMilli()` → **锁外** fan-out 到 others（`BroadcastToUser` 循环）→ 空 `{}` ack。self 严格过滤
- `OnDisconnect(connID, userID)` —— AC5 流程：cross-check connMap → 查 userLoc → **确认 `member.ConnID == connID`**（双层防御，防 reconnect race）→ `leaveRoomLocked`

**长度/编码校验**：`validateField(s, name)` 统一覆盖非空 + ≤ 64 字节 + `utf8.ValidString` 三种错误。复用 `dto.ErrValidationError`（Story 0.6 sentinel），**不新增 error code**——避免污染注册表。

### `internal/ws/hub.go` — `ClientObserver` 基础设施（AC5，长期保留）

```go
type ClientObserver interface {
    OnDisconnect(connID ConnID, userID UserID)
}
```

- `Hub.observers []ClientObserver` 字段（slice，init-time-only，运行期只读；godoc 明确并发前提，延续 `Dispatcher.RegisteredTypes` pattern）
- `Hub.AddObserver(obs)` 方法（仅 initialize 调用一次）
- `Hub.Unregister` 和 `Hub.unregisterClient` 两处 `LoadAndDelete` 成功分支都调用 `h.notifyDisconnect(c)`
- `notifyDisconnect(c)` 遍历 observers fan-out

这部分是 Epic 4.1 Presence 会复用的合法扩展，**不随 `room_mvp.go` 一起删除**。本 story 是 ClientObserver 的第一个 consumer，Epic 4.1 是第二个。

### `cmd/cat/initialize.go` — debug 模式装配（AC6）

- 抬出 `broadcaster := ws.NewInMemoryBroadcaster(wsHub)` 到 debug/release 分支共用位置（release 分支不消费但构造零成本）
- debug 块追加 4 行：
  ```go
  roomManager := ws.NewRoomManager(clk, broadcaster)
  wsHub.AddObserver(roomManager)
  dispatcher.Register("room.join", roomManager.HandleJoin)
  dispatcher.Register("action.update", roomManager.HandleActionUpdate)
  ```
- **`action.broadcast` 不 Register**——它是 Direction=down 的 server 推送，Dispatcher 只处理 upstream/bi
- release 模式：零装配（不构造 RoomManager、不 AddObserver、不 Register）

### Story 0.14 drift 校验扩展：downstream-only 豁免（AC6）

这是 Story 0.14 语义正确性的一个小补丁，不属于本 story 临时 hack：

- `validateRegistryConsistency` 在遍历 `dto.WSMessages` 时，**`meta.Direction == dto.WSDirectionDown` 的条目豁免"必须在 dispatcher 注册"的要求**——downstream 消息不经过 Dispatch，天然不该 Register。
- debug 和 release 两个分支都加这条豁免
- `initialize_test.go` 新增 `TestValidateRegistryConsistency_DownstreamOnlyExempt` 用例锁死
- `dto/ws_messages_test.go` 的 consistency 测试 want 构造改为**过滤 Direction != down**（而不是硬编码添加 room.join + action.update，这样未来新增 downstream 消息自动覆盖）

这个扩展在 Epic 4 依然有效——`friend.online` / `friend.offline` 也是 downstream push。

### 测试 + 文档（AC7-9, AC11）

- **单元测试** `internal/ws/room_mvp_test.go` —— 10 条独立用例（+ Clock 精确断言嵌在 broadcast 用例内，M9 延续 Story 0.7 pattern）
- **Hub observer 测试** —— `TestHub_AddObserver_FiresOnDisconnect`（含 "LoadAndDelete 未命中时不 fire" 的反向断言）
- **集成测试** `internal/ws/room_mvp_integration_test.go` with `//go:build integration` —— 3 个 `gorilla/websocket` 客户端握手 + join + action + **bob 断开后 alice 再发只 carol 收到** 的断开回归
- `docs/api/ws-message-registry.md` 补 3 段（每段首行加粗 **"MVP only — to be removed wholesale when Epic 4.1 ships"**，含完整 errors 清单）

## 怎么实现的

### 为什么用 `BroadcastToUser` 循环而不是 `BroadcastToRoom`

`InMemoryBroadcaster.BroadcastToRoom` 当前是 D6 预留的 no-op（warn log + return nil）。把它实现成真 fan-out 属于 **Epic 4.3** `roomstatebroadcaster-replace-epic2-noop` 的范围——本 story 动它就是侵占其他 story 的 scope，且 N ≤ 4 时 O(N) 循环微不足道，没有性能顾虑。保持边界清晰。

### 为什么 `action.update` 拿锁只管状态、fan-out 在锁外

`BroadcastToUser` 最终会调 Hub.FindByUser 再 `client.trySend`——这条路径上持 `RoomManager.mu` 没必要，还会把 Hub 的读锁和 RoomManager 的写锁搅在一起，未来加持久化或跨实例广播时容易踩死锁。做法：在锁内快照 `others []UserID`，释放锁，再循环 broadcast。语义允许极小窗口内 `action` 已更新但尚未广播完毕——MVP 不需要严格同步，Epic 4 如需就上 `BroadcastToRoom` 的 Redis Pub/Sub 版本。

### 为什么 `OnDisconnect` 要校验 `member.ConnID == connID`（review round 1 修复）

**reconnect race**：同一用户在旧 socket 尚未完成 disconnect 前用新 socket 重连并 `room.join`。毫秒级之后旧 socket 的 `OnDisconnect` 触发，按原逻辑：
1. `connMap[oldConn]` 还是指向该 user（join 时只**新增** `connMap[newConn]`，没清理旧的）
2. `userLoc[user]` 指向当前房间（新 session 正常在用的）
3. 直接走 `leaveRoomLocked` → user 被**自己的新 session**踢出房间

真机联调场景这非常容易触发（Wi-Fi 切换、watch 从 sleep 唤醒、app process relaunch 都能在毫秒级重建 socket）。

**双层防御修复**：
1. **`HandleJoin`**：switch-room 和 same-room 两个分支都主动清理旧 `Member.ConnID` 对应的 `connMap` 条目
2. **`OnDisconnect`**：在 `leaveRoomLocked` 之前加 `if member.ConnID != connID { return }` ——即使 connMap 清理路径遗漏，room 层仍能识别 stale 信号拒绝 eviction

两条回归测试 `TestRoomManager_Rejoin_SameRoom_StaleDisconnectDoesNotEvict` + `TestRoomManager_Rejoin_SwitchRoom_StaleDisconnectDoesNotEvict` 锁死该路径。

### 为什么 members snapshot 用 `make([]..., 0)` 而不是 `nil`

Go 的 `json.Marshal(nil slice)` 输出 `null`，`json.Marshal([]T{})` 输出 `[]`。iOS `JSONDecoder` 对 `Array.decode` 遇到 `null` 会 throw。这是 Story 0.14 已建立的纪律（`WSMessages` 空 slice 也是 `[]` 而非 `null`），本 story 延续：用 `make([]memberSnapshot, 0, len(...))` 保证 wire shape 稳定。`TestRoomManager_JoinRoom_EmptyRoomSnapshot` 直接 `assert.Contains(string(raw), `"members":[]`)` 锁死。

### 为什么 `ClientObserver` 不放 `room_mvp.go`

它是 Hub 的合法基础设施扩展，不是 MVP 特有的 hack。Epic 4.1 Presence 会是它的第二个消费者，到时如果它在 `room_mvp.go` 里就得跨文件移动才能避免删除。放 `internal/ws/hub.go` 最直接。PR reviewer 应把这部分按"合法扩展"审，不是"为 MVP 加的特判"。

## 怎么验证的

```bash
# 基础构建 + 单元测试（含 vet + check_time_now.sh M9）
bash scripts/build.sh --test
# 结果：OK: all tests passed / BUILD SUCCESS

# 集成测试
go test -tags integration ./internal/ws/... -count=1
# 结果：ok  github.com/huing/cat/server/internal/ws  0.376s

# RoomManager + Hub observer 用例独立跑（12 条）
go test ./internal/ws/... -run 'TestRoomManager|TestHub_AddObserver' -count=1 -v
# 全 PASS
```

**覆盖的场景**：

- 加入同一房间幂等（不重复广播、不重复计数）
- 切换房间清理旧房间 + 空房间 GC
- snapshot 排除 self
- 空房间 snapshot 为 `[]` 非 `null`
- 广播严格排除发送方、`tsMs == clock.UnixMilli()`
- 未 join 直接 `action.update` 返回 `VALIDATION_ERROR: "user not in any room"`
- 空 / 超长 / 非 UTF-8 action 返回 `VALIDATION_ERROR`
- 断开清理 member + 空房间 GC + 全 map 清理
- 断开未知 connID 无 panic 无副作用
- 10 goroutine 并发 join + update（race detector 不可用于 Windows cgo，靠并发 + 断言兜底）
- **reconnect race**：同用户旧/新 conn，旧 conn 断开不踢新 session（两条 round 1 新增）
- 集成：3 客户端握手 + alice 发 walking → bob/carol 收、alice 只收 ack → bob 断开 → alice 再发 waving → 只 carol 收

## 后续 story 怎么用

### Epic 4.1 上线时的拆除清单

**整块 `git rm` 的文件**：
- `server/internal/ws/room_mvp.go`
- `server/internal/ws/room_mvp_test.go`
- `server/internal/ws/room_mvp_integration_test.go`

**从 `dto.WSMessages` 删除**：`room.join` / `action.update` / `action.broadcast` 三条 entries（Epic 4.1 用什么新消息名由 Epic 4 设计档案决定；如果仍叫 `room.join` 就翻 `DebugOnly` flag 并重新实现 handler，不是复用 room_mvp 的代码）

**从 `cmd/cat/initialize.go` debug 块删除**：4 行 `NewRoomManager` / `AddObserver` / `Register` / `Register` 装配

**`docs/api/ws-message-registry.md`** 删除 3 段（或改写为 Epic 4 正式版语义）

### **保留的东西**（Epic 4.1 会继承使用）

- `internal/ws/hub.go` 的 `ClientObserver` 接口 + `Hub.observers` 字段 + `Hub.AddObserver` + `notifyDisconnect` fan-out
- `cmd/cat/initialize.go::validateRegistryConsistency` 的 `Direction == WSDirectionDown` 豁免逻辑（Epic 4.4 的 `friend.online` / `friend.offline` 也是 downstream push，同样享受豁免）
- `docs/api/ws-message-registry.md` 的 Direction-down 条目格式约定

### 联调开工说明

**debug 模式启动服务端**（config 的 `[server].mode = "debug"`）后，真机客户端可立刻开始：

1. WS Upgrade：任意非空 `Authorization: Bearer <token>`（Story 0.11 debug validator 接受一切非空 bearer 作为 userID）
2. 发 `{"id":"1","type":"room.join","payload":{"roomId":"test-room"}}` → 收 `room.join.result` + members snapshot
3. 发 `{"id":"2","type":"action.update","payload":{"action":"walking"}}` → 收空 ack + 其他房间成员收到 `action.broadcast` push
4. 断开 WS → 服务端自动从房间移除（无 D8 宽限期）
