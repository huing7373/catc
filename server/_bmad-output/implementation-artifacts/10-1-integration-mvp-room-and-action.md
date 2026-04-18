# Story 10.1: 联调 MVP — 房间 + 动作 3 消息集 + 内存 RoomManager

Status: done

Epic: **Epic 10 — 客户端联调最小集成（手表+iOS MVP）**（横向 epic；2026-04-18 用户决策新增，不在 `epics.md` 原规划档案内）

## Scope 定位

**这是一个故意临时的 story。** 用户的请求原文："想快速做出一个可以和手表端和 iOS 端联调的最小可运行单元，需要包括，加入房间，上传当前动作，下发当前动作，除此之外，其他全部没有"。

目的：让**真机**手表 + iPhone 客户端的联调能**今天**就开始，不用等 Epic 4.1-4.5 的完整 presence / 4 人上限 / 持久化 / D8 断连宽限 / 跨实例广播落地。Epic 4 是正式版，本 story 是**最小可运行骨架**。

**Epic 4 上线时整个 `internal/ws/room_mvp.go` 连同 3 条 `DebugOnly: true` 消息将被**整块删除**，不是"翻 flag 转正"** —— 因为本 story 故意不做的东西（持久化、4 人上限、presence lifecycle、session resume 联动、RedisPubSub 跨实例）全都在 `room_mvp.go` 外面没实现，正式版必须从头重写，本文件留着只会误导。这条"故意临时"的纪律写在 `room_mvp.go` 的 package doc 顶部。

**用户 memory `project_backup_fallback.md` 适用性审视**：用户反对 "backup / fallback 掩盖核心架构风险"。本 story **不是** backup fallback —— 它不和 Epic 4 并列，不是 Epic 4 的"候补方案"；Epic 4 的设计完全独立存在（`architecture.md` §D1 Broadcaster `PushOnConnect` / `BroadcastDiff`，Story 9.1 AC6-AC7 收敛决策会定型）。本 story 仅是联调测试工装 —— 好比 `tools/ws_loadgen`，只是位置在 `internal/ws/` 而非 `tools/`，因为它要被 dispatcher 装配。

## Story

As a backend developer,
I want to ship the smallest possible WS surface that supports 加入房间 / 上传当前动作 / 下发当前动作 on a real Apple Watch + iPhone client,
so that hardware integration testing can start immediately, without waiting for the full Epic 4 presence / room / persistence / lifecycle stack.

## Acceptance Criteria

1. **AC1 — 3 条 `DebugOnly: true` WS 消息登记到 `dto.WSMessages`（Story 0.14 四步走）**：

   | type | Version | Direction | RequiresAuth | RequiresDedup | DebugOnly |
   |---|---|---|---|---|---|
   | `room.join` | v1 | bi | true | false | true |
   | `action.update` | v1 | up | true | false | true |
   | `action.broadcast` | v1 | down | true | false | true |

   `Description` 字段：一行英文摘要说明消息目的 + "MVP only, removed when Epic 4.1 ships"。

   **payload 形状（wire shape，iOS `JSONDecoder` 约定 camelCase）：**

   - `room.join` request payload: `{"roomId": "<string, 1-64 bytes>"}`
   - `room.join.result` payload: `{"roomId": "<string>", "members": [{"userId": "<string>", "action": "<string, possibly empty>", "tsMs": <int64, 0 if no action>}]}`（不含自己）
   - `action.update` request payload: `{"action": "<string, 1-64 bytes>"}`
   - `action.update.result` payload: `{}`（空 ack）
   - `action.broadcast` push payload: `{"userId": "<string>", "action": "<string>", "tsMs": <int64>}`

   Story 0.14 double-gate 漂移守门全部通过（`dto/ws_messages_test.go` + `cmd/cat/initialize_test.go` 新增覆盖本 3 条 + `/v1/platform/ws-registry` 在 debug 模式返回这 3 条、release 模式不返回）。

2. **AC2 — 内存 `RoomManager` + `Room` + `Member`（`internal/ws/room_mvp.go`，首次创建）**：

   - `Member`：`UserID`（复用 `ws.UserID` 类型别名）/ `ConnID` / `LastAction string`（empty = 从未 update）/ `LastActionTs int64`（Unix ms，0 = 从未 update）
   - `Room`：`ID RoomID` / `members map[UserID]*Member`（`RoomID` 新增为 `ws.RoomID` 别名 —— 架构 `internal/ws/broadcaster.go` 行 11 已预留该别名，本 story 仅使用）
   - `RoomManager`：`clock clockx.Clock` / `rooms map[RoomID]*Room` / `userLoc map[UserID]RoomID` / `connMap map[ConnID]UserID` / `sync.RWMutex` 保护全部 map
   - 构造：`NewRoomManager(clock clockx.Clock, broadcaster Broadcaster) *RoomManager`；nil clock / broadcaster → panic（fail-fast startup，架构 §P3）
   - `roomId` / `userId` / `action` 全部做 UTF-8 长度 ≤ 64 字节校验；非空校验
   - **无 4 人上限、无房间数上限、无过期清理** —— MVP 全部跳过

3. **AC3 — `room.join` handler**：

   - 接 `{roomId}` 请求（Envelope.payload 解析 + 校验）
   - 1 步：若 user 已在其他 room：从旧 room.members 移除；若旧 room 变空：删除整个 room（GC）；userLoc 更新
   - 2 步：若目标 room 不存在：创建；加入 user（`Member{UserID, ConnID, LastAction: "", LastActionTs: 0}`）
   - 3 步：snapshot：遍历 room.members，**排除自己**，output `[]MemberSnapshot{UserID, LastAction, LastActionTs}`
   - 4 步：返回 `{roomId, members: snapshot}`（即使 snapshot 为空也发 `"members": []`，nil-safe wire shape，iOS 解码稳定 —— 延续 Story 0.14 AC 决策）
   - 重复 join 同 room：第 1 步无 op（已在目标 room），第 2 步无 op，第 3 步返回 snapshot；**幂等、不重复广播**（本 story 无 join 广播消息）
   - **无 `member.join` 广播消息**（"除此之外全部没有"）

4. **AC4 — `action.update` handler + `action.broadcast` 广播**：

   - 接 `{action}` 请求（校验非空 + ≤ 64 字节）
   - 1 步：查 `userLoc[userID]`；若无 room：返回 `dto.ErrValidationError`（message: `"user not in any room"`）；**不**新增 error code（避免污染 Story 0.6 注册表）
   - 2 步：更新 `member.LastAction = action` + `member.LastActionTs = clock.Now().UnixMilli()`
   - 3 步：快照 room 中**其他**成员的 userID 列表（不含自己）
   - 4 步：释放锁后，对每个其他 userID 调用 `broadcaster.BroadcastToUser(ctx, otherUserID, broadcastMsg)`
     - `broadcastMsg` 是一个 `ws.Push{Type: "action.broadcast", Payload: json{userId, action, tsMs}}` 的 JSON 字节
     - **不走 `BroadcastToRoom`** —— 目前 `InMemoryBroadcaster.BroadcastToRoom` 是 D6 预留 no-op，动它会侵占 Epic 4.3 `roomstatebroadcaster-replace-epic2-noop` 的范围；MVP 用 `BroadcastToUser` 循环即可（预期 N ≤ 4 时 O(N) 微不足道）
   - 5 步：返回空 ack（`{}`）
   - 广播时发起方**不**收到自己的 `action.broadcast`（协议对称性由接收方验证；服务端严格过滤 self）

5. **AC5 — 连接断开自动清理（新增 `ws.ClientObserver` 接口 + `Hub.AddObserver`）**：

   - `internal/ws/hub.go` 新增：
     ```go
     type ClientObserver interface {
         OnDisconnect(connID ConnID, userID UserID)
     }
     ```
   - `Hub` 新增字段 `observers []ClientObserver`（slice，不加锁 —— 仅在 `initialize()` 同步注册，运行期只读；godoc 明确并发前提，延续 `Dispatcher.RegisteredTypes()` 的 pattern）
   - `Hub.AddObserver(obs ClientObserver)` 方法（只在 initialize 调用一次）
   - `Hub.Unregister(connID)` + `Hub.unregisterClient(c)` 两处 LoadAndDelete 成功后**都**调用 `h.notifyDisconnect(c)`；`notifyDisconnect` 遍历 observers fan-out
   - `RoomManager.OnDisconnect(connID, userID)`：RWMutex write lock；查 `connMap[connID]` 确认 user；从 `userLoc[userID]` 找 room；从 room.members 删除 user；room 空则删除 room；清理 userLoc + connMap
   - **无 `member.leave` 广播消息**（"除此之外全部没有"；Epic 4.1 会做 presence D8 宽限 + 广播）
   - **Epic 4.1 正值**：`ClientObserver` 接口**不是**临时设计，Epic 4 Presence 将同样实现并 `AddObserver`。本 story 是第一个 consumer；这是对 Hub 的合法基础设施扩展，**非**throwaway

6. **AC6 — 仅 debug 模式装配 + Story 0.14 registry drift gate 通过**：

   - `cmd/cat/initialize.go` 在现有 `if cfg.Server.Mode == "debug"` 块内（当前 line 117-133 左右）：
     - 构造 `roomManager := ws.NewRoomManager(clk, broadcaster)`
     - `hub.AddObserver(roomManager)`
     - Register 3 个 handler：`dispatcher.Register("room.join", roomManager.HandleJoin)` / `Register("action.update", roomManager.HandleActionUpdate)`；**`action.broadcast` 是 downstream push，不 Register handler**（Dispatcher 只处理 upstream）
   - release 模式：**零装配**（不构造 RoomManager、不 Register、不 AddObserver）
   - `validateRegistryConsistency` 的 `debugOnlyInRelease` bucket 对新 3 条生效：release 模式若误注册立即 Fatal
   - `dto/ws_messages_test.go` `TestWSMessages_ConsistencyWithDispatcher_DebugMode` 的 want slice 加 `room.join` 和 `action.update`；`TestWSMessages_ConsistencyWithDispatcher_ReleaseMode` 依然为空 slice（3 条都 DebugOnly）
   - **注意 `action.broadcast` 不在 dispatcher 注册**，但要在 `dto.WSMessages` 里有条目：这意味着 Story 0.14 的 `unknownRegistered` / `missingInDebug` 校验需要**允许 downstream-only 消息** —— 需要扩展校验逻辑：若 `meta.Direction == WSDirectionDown`，豁免"必须在 dispatcher 注册"的要求（因为 Dispatcher 只处理 up/bi，down 消息不经过 Dispatch）

7. **AC7 — 单元测试（`internal/ws/room_mvp_test.go`）**：

   - `TestRoomManager_JoinRoom_IdempotentSameRoom` — 重复 join 同 room，无副作用
   - `TestRoomManager_JoinRoom_SwitchRoomsLeavesOld` — user 从 roomA 切到 roomB，roomA 正确清理，空 room 被 GC
   - `TestRoomManager_JoinRoom_SnapshotExcludesSelf` — 3 人同 room，snapshot 不含调用者
   - `TestRoomManager_JoinRoom_EmptyRoomSnapshot` — 第一个加入者收到 `"members": []`（非 nil）
   - `TestRoomManager_ActionUpdate_BroadcastsToOthers` — 用 mock `Broadcaster` 验证 `BroadcastToUser` 被调用次数 + 目标 userID 列表正确（不含调用者）
   - `TestRoomManager_ActionUpdate_NoRoomReturnsValidationError` — 返回 `dto.ErrValidationError`
   - `TestRoomManager_ActionUpdate_LengthValidation` — action 超 64 字节返回 `dto.ErrValidationError`
   - `TestRoomManager_OnDisconnect_RemovesMember` — disconnect 后 user 从 room + 全部 map 清理；room 空则删
   - `TestRoomManager_OnDisconnect_IdempotentForUnknownConn` — 未知 connID disconnect 不 panic 不报错
   - `TestRoomManager_ConcurrentJoinAndUpdate` — 10 goroutine 同时 join + action.update，race detector 下无数据竞争
   - **Clock 验证**：用 `clockx.NewFakeClock(fixed)` 注入，断言 `LastActionTs` 精确 = fixed.UnixMilli()（M9 延续 Story 0.7 pattern）

8. **AC8 — 集成测试（`internal/ws/room_mvp_integration_test.go`，build tag `integration`；推荐但非必须）**：

   - `httptest.NewServer` 跑全栈（不需要 Mongo/Redis；debug mode 装配）
   - 3 个 `gorilla/websocket` 客户端带不同 bearer token 连接 → 都 join `"test-room"`
   - Client A 发 `action.update{action: "walking"}` → B 和 C 收到 `action.broadcast{userId: "A", action: "walking", tsMs: ...}`；A 不收
   - B 断开 → A 再发 `action.update` → A 只向 C 广播（已移除 B）
   - 若在 Windows/本机 integration test lane 跑不稳，可标记 `t.Skip()` + 条件 —— 延续 Story 0.13 APNs integration 的 pragmatic pattern

9. **AC9 — `docs/api/ws-message-registry.md` 补 3 段**：

   - 每个 `### <type>` section 开头加粗标注：**"MVP only — to be removed wholesale when Epic 4.1 ships"**
   - wire shape 对齐 AC1 的 payload 描述；错误码列 `VALIDATION_ERROR` 及其 message 文案
   - 文件头部 "**新增消息四步走**" 流程图**不**改动（Story 0.14 已建立的纪律在本 story 里也走一遍）

10. **AC10 — `room_mvp.go` package doc 明示 MVP 性质**：

    - 文件顶部 package doc ≥ 6 行，必须包含：
      1. "Epic 10 Story 10.1 联调 MVP" 出处
      2. "3 DebugOnly messages: room.join / action.update / action.broadcast"
      3. **"EXPECTED to be deleted wholesale when Epic 4.1 ships"** —— 这句话精确出现，便于未来 grep
      4. "NOT a seed for Epic 4" —— 明示不要翻 flag 重用
      5. 列出本文件**故意不做**的东西：persistence / 4-person cap / D8 disconnect grace / session.resume integration / cross-instance broadcast

11. **AC11 — PR checklist（`docs/backend-architecture-guide.md` §19）**：

    - 无 `fmt.Printf` / `log.Printf`（§P5）
    - 所有 I/O 函数接 `ctx`
    - 不直接引用 `*mongo.Client` / `*redis.Client`（本 story 本就无持久化）
    - 无 `context.TODO()` / `context.Background()` 在 business path
    - 所有 exported identifier 有英文 godoc
    - `bash scripts/build.sh --test` 绿（含 race 测试 under `-race` flag；如果 CI 支持）
    - 对 `internal/ws/hub.go` 的改动（新增 `ClientObserver` 接口 + `Hub.AddObserver` + `notifyDisconnect`）有对应 `hub_test.go` 新增用例（至少 1 条验证 observer 被调用）

## Tasks / Subtasks

- [x] **Task 1 — `dto.WSMessages` 追加 3 条 + registry drift 校验调整** (AC: #1, #6)
  - [x] `internal/dto/ws_messages.go` append 3 条 entries（顺序: room.join → action.update → action.broadcast）
  - [x] `internal/dto/ws_messages_test.go` `TestWSMessages_AllFieldsPopulated` 自动通过（循环遍历无需改）；`TestWSMessages_NoDuplicates` 自动通过
  - [x] `TestWSMessages_ConsistencyWithDispatcher_DebugMode` want 构造改为 "Direction != down" 过滤（等价于显式加 room.join + action.update，同时覆盖未来 downstream 新增）
  - [x] `cmd/cat/initialize.go` `validateRegistryConsistency`：扩展逻辑 —— `meta.Direction == dto.WSDirectionDown` 的条目豁免"必须在 dispatcher 注册"的要求；同步更新相关 `initialize_test.go` 用例（新增 `TestValidateRegistryConsistency_DownstreamOnlyExempt`）
- [x] **Task 2 — `ClientObserver` 接口 + `Hub.AddObserver` + `notifyDisconnect` fan-out** (AC: #5)
  - [x] `internal/ws/hub.go` 增 `ClientObserver` 接口定义 + `Hub.observers []ClientObserver` + `Hub.AddObserver(obs)` + `notifyDisconnect(c *Client)` 私有辅助
  - [x] `Hub.Unregister` 和 `Hub.unregisterClient` 两处 `LoadAndDelete` 成功分支都调用 `h.notifyDisconnect(c)`
  - [x] `internal/ws/hub_test.go` 新增 `TestHub_AddObserver_FiresOnDisconnect`（验证 observer 收到正确 connID + userID，且未命中 LoadAndDelete 时不 fire）
- [x] **Task 3 — `internal/ws/room_mvp.go` 核心** (AC: #2, #3, #4, #5, #10)
  - [x] package doc（≥ 6 行 + "EXPECTED to be deleted wholesale when Epic 4.1 ships" + "NOT a seed for Epic 4" 两处精确字面量）
  - [x] `RoomManager` / `Room` / `Member` / `memberSnapshot` 类型 + `NewRoomManager` 构造（nil clock/broadcaster panic）
  - [x] `HandleJoin(ctx, client, env) (json.RawMessage, error)` —— AC3 四步
  - [x] `HandleActionUpdate(ctx, client, env) (json.RawMessage, error)` —— AC4 五步
  - [x] `OnDisconnect(connID, userID)` —— AC5 流程（含 connMap 交叉校验）
  - [x] payload 解析辅助：`joinRequest` / `actionUpdateRequest` / `joinResponse`（含 `memberSnapshot`）/ `actionBroadcastPush` 四个请求/响应 struct，JSON tag 全 camelCase
  - [x] 长度校验辅助 `validateField(s, name string) error`（内联，不 export；同时覆盖空值 / 长度 / UTF-8 三种错误消息）
- [x] **Task 4 — initialize.go 装配（debug 模式）** (AC: #6)
  - [x] `cmd/cat/initialize.go` debug 块构造 `roomManager := ws.NewRoomManager(clk, broadcaster)`（broadcaster 抬到 if 块外，debug 分支唯一消费者）
  - [x] `hub.AddObserver(roomManager)`
  - [x] `dispatcher.Register("room.join", roomManager.HandleJoin)` + `Register("action.update", roomManager.HandleActionUpdate)`
  - [x] 确认 `validateRegistryConsistency` 调用的 drift 校验全绿（build.sh --test PASS）
- [x] **Task 5 — 单元测试** (AC: #7)
  - [x] `internal/ws/room_mvp_test.go` 覆盖 AC7 全部用例（10 条独立 test 函数 + Clock 断言嵌在 broadcast 用例内）
  - [x] 使用 `clockx.NewFakeClock` 注入；测试文件内定义 `fakeRoomBroadcaster` 捕获 `BroadcastToUser` 调用
  - [x] `t.Parallel()` 标注各 sub-test
- [x] **Task 6 — 集成测试（推荐）** (AC: #8)
  - [x] `internal/ws/room_mvp_integration_test.go` with `//go:build integration`
  - [x] 三客户端 WS 握手 + join + action + 断言（含 bob 断开后 alice 再发 → 只 carol 收到的回归）
  - [x] 本地 `go test -tags integration ./internal/ws/...` PASS
- [x] **Task 7 — `ws-message-registry.md` 补 3 段** (AC: #9)
  - [x] 每段开头加 "**MVP only — to be removed wholesale when Epic 4.1 ships**"
  - [x] wire shape 对齐 AC1 描述（含 errors 清单）
- [x] **Task 8 — PR checklist 最终自查 + build 绿** (AC: #11)
  - [x] `bash scripts/build.sh --test` 绿（`go vet` clean + `scripts/check_time_now.sh` 通过均由脚本串起）
  - [x] integration 测试套件 `-tags integration` 全绿（无回归）
  - [x] §19 逐项过（详见 Completion Notes）

## Dev Notes

### 核心定位

- **故意临时的 MVP**。user 对话原文："快速做出一个可以和手表端和 iOS 端联调的最小可运行单元"。整条 story 的成功判据不是架构完美，而是**真机客户端今天就能跑起来 3 个 flow**。
- Epic 4.1 上线时 `room_mvp.go` 整文件删除 + 3 条 DebugOnly 消息从 `dto.WSMessages` 删除（不是翻 flag）。Epic 4.1 开发者**不会**从 room_mvp.go 开始，因为它没有 persistence / cap / presence，正式版必须重写。

### Epic 4 分界线（明确不做）

| 做 | 不做（→ Epic 4） |
|---|---|
| 内存 map + Hub.observer hook | 持久化（Mongo room collection + `roomservice` domain；Epic 4.2）|
| 无上限 | 4 人上限（Epic 4.2）|
| 断开直接清理 | D8 disconnect 宽限期（Epic 4.1 core）|
| 无 join / leave 广播 | `friend.online` / `friend.offline` 广播（Epic 4.4）|
| `session.resume` 响应仍由 Story 0.12 handler（Empty provider 路径）处理，**不感知 room state** | `session.resume` 返回 roomSnapshot（Epic 4.5 + RoomSnapshotProvider 真实装配）|
| `BroadcastToUser` 循环 N 次 | `BroadcastToRoom` 真实装配 + RedisPubSub 跨实例（Epic 4.3）|

### `ClientObserver` 是**正值**，不是技术债

本 story 对 `internal/ws/hub.go` 的改动（`ClientObserver` 接口 + `Hub.AddObserver` + `notifyDisconnect`）Epic 4.1 也需要 —— 因为 disconnect → leave room 的 wiring 就是一样的模式。本 story 是 ClientObserver 的**第一个 consumer**；Epic 4.1 Presence 会是第二个。因此这部分代码**不随 `room_mvp.go` 一起删除**。PR reviewer 应把这部分按"合法基础设施扩展"审，不是"为 MVP 加的特判"。

### Story 0.14 registry drift 扩展：downstream-only 消息豁免

`action.broadcast` 是 server → client 的 downstream push，**不**经过 Dispatcher（Dispatcher 只 Dispatch upstream/bi 消息）。但它必须在 `dto.WSMessages` 登记（否则 Story 0.14 AC15 `unknownInRegistry` 会抱怨）。

`validateRegistryConsistency` 当前逻辑（Story 0.14 round 1 after fix）要求 "debug mode 每条 `WSMessages` 条目都必须在 dispatcher 注册"。本 story 必须扩展：`meta.Direction == dto.WSDirectionDown` 的条目**豁免**此要求 —— downstream 消息压根不该 register 到 Dispatcher。

修改点：`cmd/cat/initialize.go` 的 `validateRegistryConsistency` 函数内，遍历 `WSMessages` 时加一条 `if meta.Direction == dto.WSDirectionDown { continue }` 跳过该条目的 missing 检查。同步更新 `initialize_test.go`：新增 `TestValidateRegistryConsistency_DownstreamOnlyExempt` 用例锁死此行为。

这个扩展在 Epic 4 依然有效（Epic 4 的 `friend.online` / `friend.offline` 同样 downstream），所以**不是**本 story 临时 hack —— 是 Story 0.14 的语义正确性补丁，跟 room_mvp 一起被 review。

### 文件分布

**新建：**
- `internal/ws/room_mvp.go`
- `internal/ws/room_mvp_test.go`
- `internal/ws/room_mvp_integration_test.go`（可选）

**修改（现有代码）：**
- `internal/dto/ws_messages.go` — 追加 3 条
- `internal/dto/ws_messages_test.go` — consistency test want slice 同步
- `internal/ws/hub.go` — 新增 `ClientObserver` + `AddObserver` + `notifyDisconnect`
- `internal/ws/hub_test.go` — 新增 observer fan-out 测试
- `cmd/cat/initialize.go` — debug 块装配 3 个新组件 + `validateRegistryConsistency` downstream 豁免
- `cmd/cat/initialize_test.go` — drift gate 用例扩展
- `docs/api/ws-message-registry.md` — 补 3 段

### 前置 Story 资产（可直接复用）

| 资产 | 来源 | 用法 |
|---|---|---|
| `ws.Hub` + `HubConfig` | Story 0.9 | 加 ClientObserver hook 的主体 |
| `ws.Client.UserID() / ConnID()` | Story 0.9 | handler 内获取会话身份 |
| `ws.Dispatcher.Register` | Story 0.9/0.10 | 注册 2 个 upstream handler |
| `ws.Envelope` + `NewAckResponse` / `NewErrorResponse` / `NewPush` | Story 0.9 | 请求/响应/广播的 wire struct |
| `ws.Broadcaster.BroadcastToUser` + `InMemoryBroadcaster` | Story 0.9 | MVP 唯一用到的 broadcaster 方法 |
| `ws.RoomID` 类型别名 | Story 0.9 `broadcaster.go` 行 11 | 本 story 第一次消费此别名 |
| `dto.WSMessages` + `WSMessagesByType` | Story 0.14 | 4 步走中的第 1 步 |
| `dto.ErrValidationError` | Story 0.6 | payload 校验失败统一返回 |
| `clockx.Clock` + `NewFakeClock` | Story 0.7 | `action.tsMs` 时间源（M9）+ 测试注入 |
| debug-mode bearer validator | Story 0.11 | 任意非空 bearer = userID，无需真 JWT |

### 前置 Story 关键教训

- **Story 0.14 registry drift**：四步走纪律同样适用本 story —— 漏任何一步 CI 挡下
- **Story 0.12 session.resume DebugOnly**：本 story 3 条消息的 DebugOnly 决策 pattern 一致；release 模式零装配
- **Story 0.13 detached writeCtx**：广播路径的 ctx 管理 —— `BroadcastToUser` 接 ctx 不能传 `client.Context()`（client 已 disconnect 时 ctx.Done 会让广播失败），本 story 调用处用 handler 入参 ctx 即可（每次 handler 调用都有 fresh dispatch ctx）
- **Story 0.11 rate limit**：本 story 不新增 rate limit；`action.update` 重放直接落在 Hub 已有的 per-conn 100 msg/s 上，MVP 够用

### 禁止事项 / 常见误区

- **不要**同时改 `InMemoryBroadcaster.BroadcastToRoom`（Epic 4.3 的范畴）—— 本 story 用 `BroadcastToUser` 循环即可
- **不要**在 `session.resume` 返回 roomSnapshot（Epic 4.5 的范畴）—— `RoomSnapshotProvider` 保持 `EmptyRoomSnapshotProvider` 返回 `null`
- **不要**新增 error code（Story 0.6 注册表）—— 复用 `dto.ErrValidationError`
- **不要**写 "plan for migration to Epic 4" 文档 —— Epic 4 规划档案（`epics.md` + `architecture.md`）已有设计；本 story 删除时直接 `git rm`，无需迁移文档
- **不要**把 `room_mvp.go` 放 `internal/room/`（新 domain 目录）—— `internal/ws/` 是临时归宿；一旦成为 `internal/room/` 就暗示"长期 domain"，跟 package doc 的"整块删除"语义冲突
- **不要**把 `ClientObserver` 放 `room_mvp.go` —— 它是 Hub 的合法扩展，放 `internal/ws/hub.go` 或单独 `internal/ws/hub_observer.go`；Epic 4.1 还要用
- **不要**为了"对齐 Epic 4 原计划"而换消息名（e.g. `presence.enter` 之类）—— 用户原话 "加入房间" 直译 `room.join` 是客户端开发者一眼理解的语义；Epic 4 要用 `room.join` 就翻 flag，要用别的名字就另起炉灶

### Project Structure Notes

- `internal/ws/room_mvp.go` 是 `internal/ws/` 下第一个 **domain-level** 文件（此前都是 WS 协议层基础设施：hub / dispatcher / envelope / session_resume / dedup / conn_guard）。语义上 room 更应该在 `internal/room/`（Epic 4 开始），但 MVP 临时性强，且必须直接 import `ws.Client` / `ws.Broadcaster` / `ws.Push`，放 `internal/ws/` 内最少摩擦
- Epic 4.1 开始时，新建 `internal/room/` domain 包；`room_mvp.go` 整块 `git rm`，不迁移

### References

- [Source: `server/_bmad-output/implementation-artifacts/sprint-status.yaml`#epic-10] — Epic 10 本 story 唯一 story
- [Source: `server/_bmad-output/implementation-artifacts/0-14-ws-message-type-registry-and-version-query.md`] — 四步走纪律
- [Source: `server/_bmad-output/implementation-artifacts/0-9-ws-hub-skeleton-envelope-broadcaster-interface.md`] — Hub + Broadcaster interface 基底
- [Source: `server/_bmad-output/implementation-artifacts/0-12-session-resume-cache-throttle.md`] — DebugOnly 装配 pattern
- [Source: `server/_bmad-output/planning-artifacts/epics.md`#Epic 4] — Epic 4.1-4.5 正式 presence/room 范围，本 story 的分界线参照
- [Source: `server/_bmad-output/planning-artifacts/architecture.md`#D1 Broadcaster 行 277-292] — `BroadcastToUser` / `BroadcastToRoom` 语义；本 story 仅用 ToUser
- [Source: `server/internal/ws/broadcaster.go` 行 9-18] — `Broadcaster` interface + `ConnID / UserID / RoomID` 别名
- [Source: `server/internal/ws/hub.go` 行 65-84] — `Register` / `Unregister` / `unregisterClient` 是 ClientObserver hook 的注入点
- [Source: `server/internal/dto/ws_messages.go` 行 49-77] — WSMessages 追加目标
- [Source: `server/cmd/cat/initialize.go` 行 117-133] — debug 块装配位置
- [Source: `docs/backend-architecture-guide.md` §12 WebSocket / §19 PR checklist] — AC11 gate
- [Source: auto-memory `project_backup_fallback.md`] — 本 story 不是 fallback（非 Epic 4 的并列方案；是测试工装）
- [Source: auto-memory `project_claude_coding.md`] — Claude 99.99% 编码本身无问题；瓶颈在真机；本 story 让真机能开工

## Dev Agent Record

### Agent Model Used

claude-opus-4-7 (Opus 4.7, 1M context)

### Debug Log References

- 单元测试套件：`go test ./internal/ws/... -run 'TestRoomManager|TestHub_AddObserver' -count=1 -v` — 11/11 PASS
- 全仓库 `bash scripts/build.sh --test` — vet + check_time_now.sh + go build + go test 全绿
- 集成测试 `go test -tags integration ./internal/ws/... -count=1` — PASS（无回归）
- 一次 test 基线调整：`TestWSRegistryEndpoint_DebugMode` 硬编码期望 3 条消息，随 Story 10.1 新增 3 条后改为 `len(dto.WSMessages)` 并 `assert.Contains` 逐一覆盖（含 3 条新增 + 原 3 条）；非 drift，属正常的新增消息影响。
- 一次 test 内名字冲突：新文件本想叫 `newTestClient`，与 `dispatcher_test.go` 内既有辅助重名，改为 `newRoomTestClient`。

### Completion Notes List

**AC 逐条对照：**

- AC1 — `dto.WSMessages` 追加 3 条：room.join (bi, DebugOnly)、action.update (up, DebugOnly)、action.broadcast (down, DebugOnly)，Description 一行摘要并包含 "MVP only, removed when Epic 4.1 ships"。payload wire shape 与 AC1 表格逐字节一致（camelCase）。
- AC2 — `RoomManager` / `Room` / `Member` / `memberSnapshot` 在新 `internal/ws/room_mvp.go`；`NewRoomManager(clk, broadcaster)` 对 nil 两者 panic（§P3）；`maxFieldBytes = 64` 常量做 UTF-8 长度 + 非空 + `utf8.ValidString` 三重校验；`RoomID` 复用 `broadcaster.go` 行 11 既有别名（本 story 首消费者）。**无**上限、过期清理，符合 MVP 定位。
- AC3 — `HandleJoin` 四步：evict old → ensure room + add self → snapshot excluding self → respond with nil-safe `members` 数组（`json.Marshal` 对 `make([]memberSnapshot, 0)` 输出 `[]`，由 `TestRoomManager_JoinRoom_EmptyRoomSnapshot` 锁死）。重复 join 同 room：更新 connID、保留 LastAction 状态、不广播。
- AC4 — `HandleActionUpdate` 五步：校验 → 查 userLoc 未命中返回 `VALIDATION_ERROR: "user not in any room"`（**不**新增 error code，复用 Story 0.6 sentinel）→ 更新 LastAction + `clock.Now().UnixMilli()` → 锁外 fan-out 到 others（`BroadcastToUser` 循环，不动 `BroadcastToRoom`）→ 空 `{}` ack。self 严格过滤。
- AC5 — `internal/ws/hub.go` 新增 `ClientObserver` 接口 + `Hub.observers []ClientObserver` + `AddObserver`（init-time-only contract，延续 `Dispatcher.RegisteredTypes` godoc 风格）+ `notifyDisconnect` 私有辅助；`Unregister` / `unregisterClient` 两处 LoadAndDelete 成功分支都调用 fan-out；`RoomManager.OnDisconnect` 含 `connMap` 交叉校验防御性 reject stranger。
- AC6 — `cmd/cat/initialize.go` debug 块：`broadcaster := ws.NewInMemoryBroadcaster(wsHub)` 抬到 if 外（release 分支不消费，但构造零成本）；debug 内 `NewRoomManager` + `hub.AddObserver` + 2 个 `dispatcher.Register`。release 零装配。`validateRegistryConsistency` 新增 `Direction == WSDirectionDown` 豁免逻辑；`initialize_test.go` 追加 `TestValidateRegistryConsistency_DownstreamOnlyExempt` 锁死；`TestWSMessages_ConsistencyWithDispatcher_DebugMode` want 改为过滤 Direction != down。
- AC7 — `internal/ws/room_mvp_test.go` 覆盖 AC7 指定的 10 条 case，Clock 精确断言嵌在 `TestRoomManager_ActionUpdate_BroadcastsToOthers` 内（`body.TsMs == fixed.UnixMilli()`）。全 `t.Parallel()`；`fakeRoomBroadcaster` 捕获 BroadcastToUser 调用、拷贝 msg 防共享。
- AC8 — `internal/ws/room_mvp_integration_test.go` with `//go:build integration`，`httptest.NewServer` + 3 个 `gorilla/websocket` 客户端：join → alice 发 walking → bob/carol 收 broadcast、alice 只收 ack → bob Close → alice 再发 waving → 只 carol 收。`time.Sleep(150ms)` 让 Hub read-pump 观察断开；本地 PASS。
- AC9 — `docs/api/ws-message-registry.md` 新增 3 段，每段首行加粗 "MVP only — to be removed wholesale when Epic 4.1 ships"；wire shape 与 AC1 逐字节对齐；含 errors 清单（VALIDATION_ERROR 各条消息文案）。四步走顶部流程图未改。
- AC10 — `room_mvp.go` package doc 30 行，含两处精确字面量："EXPECTED to be deleted wholesale when Epic 4.1 ships"（便于未来 grep）+ "NOT a seed for Epic 4"（明示不要翻 flag 重用）；列出 6 条故意不做的项目；明确 `ClientObserver` 是保留项（Epic 4 会复用）。
- AC11 — §19 PR checklist 逐项：无 fmt.Printf / log.Printf（全 zerolog ctx）；所有 I/O 函数接 ctx；handler/service 不直接引 mongo/redis client（本 story 无持久化）；无 `context.TODO()` / business path 无 `context.Background()`；所有 exported identifier 有英文 godoc；`bash scripts/build.sh --test` 本地绿（vet + check_time_now.sh M9 + 全 test PASS）；`hub.go` 改动有 `TestHub_AddObserver_FiresOnDisconnect` 覆盖。

**Story 0.14 drift gate 扩展的语义正确性说明**：downstream-only 豁免不是 Story 10.1 临时 hack，是补全 Story 0.14 对 WS direction 语义的一处遗漏 —— Dispatcher 只处理 upstream / bi，Direction=down 的条目天然不该 register。Epic 4 的 `friend.online` / `friend.offline`（也是 downstream push）会享受同样豁免。

**不随本 story 删除的代码（Epic 4.1 会继承使用）**：
- `internal/ws/hub.go` 的 `ClientObserver` 接口 + `Hub.AddObserver` + `notifyDisconnect` fan-out
- `internal/dto/ws_messages.go` 的 `Direction == WSDirectionDown` 豁免在 `cmd/cat/initialize.go::validateRegistryConsistency` 中的体现
- `docs/api/ws-message-registry.md` 的 Direction-down 条目格式约定

其余（`room_mvp.go` / `room_mvp_test.go` / `room_mvp_integration_test.go` / `WSMessages` 里 3 条 DebugOnly 条目 / `initialize.go` debug 块 4 行装配）Epic 4.1 ships 时全部 `git rm`。

### File List

新增：
- `server/internal/ws/room_mvp.go`
- `server/internal/ws/room_mvp_test.go`
- `server/internal/ws/room_mvp_integration_test.go`

修改：
- `server/internal/dto/ws_messages.go` — 追加 room.join / action.update / action.broadcast
- `server/internal/dto/ws_messages_test.go` — consistency 测试过滤 Direction != down，debug dispatcher mock register 2 条新 upstream handler
- `server/internal/ws/hub.go` — 新增 `ClientObserver` 接口 + `Hub.observers` 字段 + `AddObserver` + `notifyDisconnect`；`Unregister` / `unregisterClient` 两处调用 fan-out
- `server/internal/ws/hub_test.go` — 新增 `TestHub_AddObserver_FiresOnDisconnect`
- `server/cmd/cat/initialize.go` — debug 块装配 `RoomManager` + `hub.AddObserver` + 2 个 handler；`validateRegistryConsistency` 新增 downstream 豁免；broadcaster 提升到 if 块外
- `server/cmd/cat/initialize_test.go` — 原 `TestValidateRegistryConsistency_DebugModeFullyRegistered` 补 2 条 register + 新增 `TestValidateRegistryConsistency_DownstreamOnlyExempt`
- `server/cmd/cat/ws_registry_test.go` — `TestWSRegistryEndpoint_DebugMode` 硬编码 `Len == 3` 改为 `Len == len(dto.WSMessages)` + `Contains` 逐一覆盖 3 条新增
- `docs/api/ws-message-registry.md` — 新增 room.join / action.update / action.broadcast 三段，每段 MVP 标注
- `server/_bmad-output/implementation-artifacts/sprint-status.yaml` — 10-1 状态 ready-for-dev → in-progress → review；last_updated 2026-04-18

## Change Log

- 2026-04-18 — Story 10.1 实施完成：3 条 DebugOnly WS 消息（room.join / action.update / action.broadcast）+ 内存 `RoomManager` + `ClientObserver` Hub hook 全部落地；10 条 Room 单元测试 + 1 条 Hub observer 测试 + 1 条 WS 集成测试全绿；Story 0.14 drift 校验扩展支持 downstream-only 豁免；`bash scripts/build.sh --test` 本地 PASS。Epic 4.1 上线时 `room_mvp.go` 整块删除，`ClientObserver` 保留。
- 2026-04-18 — Story 10.1 创建。Epic 10（客户端联调最小集成）新建；本 story 承载 3 条 DebugOnly WS 消息（room.join / action.update / action.broadcast）+ 内存 RoomManager + `ClientObserver` Hub hook。Epic 4.1 上线时 `room_mvp.go` 整块删除；`ClientObserver` 保留（Epic 4 presence 会复用）。
