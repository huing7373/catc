---
date: 2026-05-14
source_review: /tmp/epic-loop-review-18-3-r2.md (codex round 2)
story: 18-3-选中表情-本地立即动效-ws-发送-emoji-send
commit: 0cad4e6
lesson_count: 1
---

# Review Lessons — 2026-05-14 — Outgoing WS send 必须在 async window 后 recheck snapshot roomId（payload 不带 roomId 的 emoji.send）（18-3 r2）

## 背景

Story 18.3 r1 fix-review 已经收口"room-scoped transient state 在 room transition 时 reset"（参 lesson `2026-05-14-room-transient-state-must-reset-on-room-transition.md`）。round 2 codex review 又指出了**同一函数 `onEmojiSelected` 的另一类 race**：catalog 校验的 async window 期间用户切换 room，导致 `useCase.execute(emojiCode:)` 调用的是 **B 时刻的 WS 连接**，emoji.send 被路由到错误的房间（V1 §12.2 emoji.send payload 不含 roomId，server 端按 session-room 路由，无法在 server 端按 message 内字段分流）。

review 原文：`/tmp/epic-loop-review-18-3-r2.md` 末尾（行 5954-5966）`^codex$` 段 —— 唯一 [P2] 项。

review 钦定修复方向：
1. `onEmojiSelected` 入口（Step A 之后，Task 启动之前）capture `snapshotRoomId = appState.currentRoomId`
2. Task 内 await catalog 之后、调 `useCase.execute(emojiCode:)` 之前，比对 `appState.currentRoomId == snapshotRoomId`
3. 不等：log info + return；本地动效（Step A）保留不回滚

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `onEmojiSelected` cross-room race —— catalog await 期间 `currentRoomId` 切换后 `useCase.execute` 仍调当前 WS（指向 B），emoji 飞到 B 的房间 | P2 / medium | architecture | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:1175-1280` |

## Lesson 1: Outgoing fire-and-forget WS send 跨 async window 必须 recheck snapshot identity（roomId / streamGen / sessionId）

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` —— `onEmojiSelected(code:)` Task 体内 catalog await 之后、`useCase.execute` 之前

### 症状（Symptom）

`RealRoomViewModel.onEmojiSelected(code:)` 是 fire-and-forget 三步链路（V1 §12.2 钦定）：

1. **Step A**（同步）：本地动效入队 → `activeEmojis.append(...)`
2. **Step B**（Task 内 async）：`emojiCatalogLoader.execute()` 校验 emojiCode 是否在缓存里
3. **Step C**（Task 内 async）：`sendEmojiUseCase.execute(emojiCode:)` 通过 WebSocketClient 发 emoji.send 帧

Step B 的 `await` 几十 ms（cache miss 路径上 HTTP 拉 `/emojis` 可能 200ms+）。期间用户可能：

- 主动 leave room → `appState.setCurrentRoomId(nil)`
- 切换到另一个 room → `appState.setCurrentRoomId("room_B")` + WebSocketClient 重连到 B 的 session
- 网络抖动后 WS 自动重连到 B（外部 reconnect 路径）

任何一种切换之后 catalog `await` 才 resume，Step C 调 `useCase.execute` 走的是**当前 WS 连接**（已指向 B 或断开）。后果：

- `currentRoomId` = "room_B" 路径：emoji.send 帧被 server 端按 session-room 路由发到 room B → room A 选的 wave 飞到 room B 的房间，其他成员看到错误来源的 emoji
- `currentRoomId` = nil 路径：`useCase.execute` 通常会抛 `.notConnected` 走 toast 路径 "网络不佳，对方可能看不到" —— 文案语义不对（用户已主动 leave，不是网络问题）

### 根因（Root cause）

**协议层根因**：V1 §12.2 钦定 emoji.send wire schema **不**含 roomId 字段：

```json
{ "type": "emoji.send", "requestId": "...", "payload": { "emojiCode": "wave" } }
```

server 端 fanout 时按"该 session 当前所属 room"决定广播范围 —— 这在 server 视角是充分的，但 **client 视角**下 "frame encode 时刻"与"frame 实际写到 socket 时刻"之间会跨越 async window，client 的 `currentRoomId` 可能在 window 内变化。server 没有任何 message 内字段可以 reject 错 room 的 send。

**client 层根因**：fire-and-forget 路径只关心"send 抛错 vs 不抛错"，没在 send 之前 recheck identity。这与 **incoming consume 路径**（12.4 r1 lesson `cross-room-race-needs-stream-roomid-capture`）是镜像对称的两类 race：

| 方向 | 风险窗口 | 已知 lesson | 守护策略 |
|---|---|---|---|
| **Incoming**（server → client） | `for await message in stream` dequeue 到 main-actor `handle` 投递之间，旧 task 仍在 in-flight | 12.4 r1 | stream 启动时 capture `streamRoomId`，handle 时比对 `streamRoomId == lastObservedRoomId` |
| **Outgoing**（client → server）| `await` 业务前置（catalog 校验 / encode）之后 | **本 lesson** | 调用入口 capture `snapshotRoomId`，send 前比对 `currentRoomId == snapshotRoomId` |

两者都源于"WS message payload 不含足够 identity 字段"的协议特征。

**为什么 r1 fix 不够**：r1 fix 让 `subscribeRoomIdConnect` 的三个 reset 分支同步清 `activeEmojis`，但只解决了"显示层 stale state 残留"。outgoing send 是另一条路径，**不**走 reset 分支（Task 自己 hold 着 `useCase` / `loader` 引用），r1 的清空动作影响不到 Task 内 closure。

### 修复（Fix）

**改动 1**（`RealRoomViewModel.onEmojiSelected`）：入口 capture snapshot + Task 内 await 后 recheck。

```swift
// 入口同步 section (Step A 之后, Task 启动之前)
let snapshotRoomId = self.appState?.currentRoomId
let snapshotAppState = self.appState

Task { [weak self] in
    guard self != nil else { return }

    // Step B: catalog await ...
    if let loader = loader { ... try await loader.execute() ... }

    // Step B.5 (r2 fix): cross-room race recheck
    let currentRoomIdNow = await MainActor.run { snapshotAppState?.currentRoomId }
    guard currentRoomIdNow == snapshotRoomId else {
        os_log(.info,
               "RealRoomViewModel.onEmojiSelected: roomId changed during async window (snapshot=%{public}@ → now=%{public}@); skip WS send (local animation already played, no rollback)",
               String(describing: snapshotRoomId),
               String(describing: currentRoomIdNow))
        return
    }

    // Step C: WS send
    try await useCase.execute(emojiCode: code)
}
```

**关键设计选择**：
- `snapshotRoomId == nil` 也参与比对（user 在 leave 状态选 emoji 的 fail-safe 路径 —— 不应该到达；但若到达，nil == nil 仍走 send 分支，由 useCase.execute 抛 `.notConnected` 让 toast 提示）。
- recheck 用 `await MainActor.run { ... }` 包裹读 `currentRoomId`，因为 `AppState` 是 `@Published` + main-actor-owned，跨 actor 直读不保证一致。
- 本地动效（Step A 入队）**不回滚** —— 与 WS 失败 toast 路径同精神：用户已经看到 wave，回滚视觉只会更困惑。

**改动 2**（`RoomViewModelEmojiSendTests` 新增 2 case）：

- `test_realOnEmojiSelected_roomSwitchDuringCatalogAwait_skipsWSSend`
- `test_realOnEmojiSelected_leaveRoomToNilDuringCatalogAwait_skipsWSSend`

两个测试都用：
- `GatedMockLoadEmojisUseCase`（actor，`execute()` 挂在 continuation 上直到 `resume()` 才返结果）—— 让 catalog `await` 卡住，模拟"async window"
- `RecordingSendEmojiUseCase`（actor，记录 `execute` 调用次数）—— 断言 cross-room race fix 命中后 `executeCount == 0`

**测试断言注意**：r1 fix 在 `subscribeRoomIdConnect` 的 A→B / A→nil 分支会清 `activeEmojis`，所以"本地动效保留"的断言要放在 `setCurrentRoomId` 切换**之前**完成；切换后只断言 `useCase.execute` 未被调用（r2 fix 核心断言）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 RealRoomViewModel / 任何 WS-bound ViewModel 上**实装 fire-and-forget outgoing send** 且 send 之前有 **async `await`**（catalog 校验 / encode 异步 / HTTP 前置）时，**必须**在调用入口 capture room-scoped identity（`snapshotRoomId` / `snapshotStreamGen`），并在 `await` 之后、send 之前 recheck `currentIdentity == snapshot`；不等则 `return`（log info）。
>
> **展开**：
> - **触发条件**（满足任一就要做）：
>   1. WS outgoing message wire schema **不含** roomId / streamId / sessionId（payload 无法在 server 端按字段分流）
>   2. send 路径上 send 之前有任何 `await` 调用（不只是 HTTP；包括 actor 跨 hop、catalog 加载、本地 async 校验等）
>   3. 调用方是 ViewModel 而非 stateless transport 层（ViewModel hold appState 这种"动态 identity"引用，async window 期间值可变）
> - **守护点放在 Task 体内还是入口**：入口 capture（Step A 之后的 main-actor 同步 section）+ Task 内 recheck（await 之后）。**不**在 Task 外面做 capture 是错的（Task 启动那一瞬间已经是异步 hop 后了，可能已经晚于 setCurrentRoomId）。
> - **recheck identity 字段选择**：
>   - 优先 `appState.currentRoomId`（room 边界最显式）
>   - 房间内 stream 复用场景叠加 `roomNavigationGeneration` token（参 lesson `room-navigation-generation-token-not-room-id-equality-12-7-r10`）让 ABA cycle（A→nil→A）也能识别
>   - WS 重连场景叠加 stream/session generation（参 lesson `ws-reconnect-stream-vs-session-generations-must-decouple-12-5-r4`）
> - **本地副作用是否回滚**：默认 **不回滚** —— Step A 的"本地立即反馈"是产品 KPI（V1 §12.2 weak-network degradation 钦定），回滚反而让用户更困惑。仅当本地副作用是"unsafe write"（如改持久化、扣库存）才需 rollback；emoji 本地动效是 transient view state，不算 unsafe。
> - **反例 1**：把 capture 放在 Task 内（`Task { let snap = self.appState?.currentRoomId; ... }`） —— Task 启动那一瞬间已是异步 hop，无法保证读到的是 Step A 时刻的值。
> - **反例 2**：只 capture，不 recheck —— capture 后从不比对就等于没 capture，typical "拍脑袋以为 capture 自带 guard"。
> - **反例 3**：在 send 抛错路径上做"判断 currentRoomId 推出该不该 toast"。这是事后补救，已经发出错 room 的帧。守护必须在 **发之前**。
> - **正例对照**：
>   - 本 lesson（outgoing send）
>   - 12.4 r1 lesson（incoming consume 镜像版）
>   - 11.8 r5 lesson `async-ordering-must-enqueue-in-caller-sync-section`（"snapshot 必须在 caller sync section 抓"同精神）
>
> **测试方法的工业化模板**：fire-and-forget cross-async-window race 的回归测试必须用 **gated mock** —— 用 actor + `withCheckedContinuation` 让 await 卡在外部可控点，测试代码在卡住期间 mutate state（`setCurrentRoomId`），然后 `resume()` 让 await 完成；断言 send mock 的调用计数为 0。本仓库参考实现：`GatedMockEmojiRepository`（`LoadEmojisUseCaseTests`）→ `GatedMockLoadEmojisUseCase`（本测试）。**反例**：用 `Task.sleep` 试图"假装"async window —— 不可控、flaky、且无法保证测试在 sleep 期间已经走到 await suspend 点。
