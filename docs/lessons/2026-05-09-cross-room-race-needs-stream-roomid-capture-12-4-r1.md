---
date: 2026-05-09
source_review: codex review --uncommitted output（/tmp/epic-loop-review-12-4-r1.md round 1，最后一个 ^codex$ 段落 line 3989+）
story: 12-4-成员加入-离开-ws-消息处理
commit: 8841b92
lesson_count: 1
---

# Review Lessons — 2026-05-09 — cross-room race 必须用 stream-startup 时刻捕获的 roomId 守护（payload 不带 room.id 的 WS 消息）

## 背景

Story 12.4（成员加入 / 离开 WS 消息处理）round 1 的 codex review 指出：`RealRoomViewModel` 的 `member.joined` / `member.left` 处理器只用 `lastObservedRoomId != nil` 守护。当用户从 room A → room B 切换时，旧 consumer task 已经 dequeue 但尚未投递到 main actor 的 room A `member.*` 事件仍会被应用到 room B 的 members（`Task.cancel()` 不能立即中断 in-flight `await MainActor.run`）。`room.snapshot` 已有 stale-room 校验（12.1 r3 加的），但 `member.*` 没有同等的 cross-room 守护 → A→B 切换瞬间会被 stale event 污染 roster。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `member.joined` / `member.left` 跨 room race —— A→B 切换时旧 consumer dequeue 的 room A 事件被 apply 到 room B | high | architecture | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:320-339` |

## Lesson 1: cross-room race 必须用 stream-startup 时刻捕获的 roomId 守护（payload 不带 room.id 的 WS 消息）

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:307-348`（handle 与 startConsumingMessages）

### 症状（Symptom）

`RealRoomViewModel` 通过 `client.messages` AsyncStream consume WS 消息；A→B 切换路径下 `subscribeRoomIdConnect` 会 cancel 旧 consumer task + restart 新 task。但：

- `Task.cancel()` 不会立即中断已经被 `for await` dequeue 但还没跑到 `await MainActor.run { handle(message:) }` 的那条 message
- 旧 task 在 cancel 前可能已 dequeue 一条 room A 的 `member.joined` 或 `member.left`，待新 room B 已 mutate `lastObservedRoomId` 后，旧 task pending main-actor work 投递到 handle，仍会被 apply 到 room B 的 members
- 旧实装仅用 `lastObservedRoomId != nil` 守护 → A→B 切换后 lastObservedRoomId 是 B（非 nil），守护不挡 → stale event 污染 roster

具体后果：
- room A 的 `member.joined u_alice` 在 A→B 切换后被 apply → room B 错误增加 u_alice
- room A 的 `member.left u_charlie` 在 A→B 切换且 room B 也有 u_charlie 时，错误把 room B 的 u_charlie 移除

### 根因（Root cause）

**协议层契约**：V1 §12.3 钦定 `member.joined` / `member.left` payload **不**含 room.id：
- `member.joined` 仅 `userId / nickname / avatarUrl / pet`
- `member.left` 仅 `userId`

server 端按 fanout 范围保证只投递到该房间的 sessions —— 这在 server 视角是充分的，但 client 视角下"消息从 stream 投到 vm"的链路会跨越 task 切换，cancel 不能阻断 in-flight delivery。

**client 视角的盲区**：把"server fanout 已正确"等价于"client 不会错 apply"。这忽略了 client 自己 task lifecycle 与 message dispatch 之间的 race —— 即使消息内容来自对的 room（server 投得对），client 也可能在错的时刻 apply（client 的 lastObservedRoomId 已切到下一个 room）。

**类比**：12.1 r3 的 stale snapshot 漏洞同精神 —— `room.snapshot` 加了 payload-level 的 `payload.room.id == lastObservedRoomId` 校验。但 `member.*` 因为 payload 没有 room.id，无法套用同样的精确校验。需要换成 stream-lifecycle 层的守护：**stream 启动时刻捕获 lastObservedRoomId 作为 streamRoomId，handle 时校验 streamRoomId == lastObservedRoomId**。

### 修复（Fix）

1. `startConsumingMessages` 改：在启动 task 时先 `let streamRoomId = self.lastObservedRoomId`（捕获快照），传给 handle：

   ```swift
   private func startConsumingMessages() {
       messageConsumerTask?.cancel()
       guard let client = webSocketClient else { return }
       let streamRoomId = self.lastObservedRoomId   // ← 捕获启动时刻 roomId
       messageConsumerTask = Task { [weak self] in
           for await message in client.messages {
               guard let self else { return }
               await MainActor.run {
                   self.handle(message: message, streamRoomId: streamRoomId)
               }
           }
       }
   }
   ```

2. `handle(message:)` → `handle(message:streamRoomId:)`，在 `member.joined` / `member.left` 分支用 `streamRoomId == lastObservedRoomId` 守护：

   ```swift
   case .memberJoined(let payload):
       guard streamRoomId != nil, streamRoomId == lastObservedRoomId else {
           os_log(.debug, "discard stale member.joined ...")
           return
       }
       applyMemberJoined(payload)
   ```

3. `room.snapshot` 保留原 payload-level 的 `payload.room.id == lastObservedRoomId` 校验（更精确），但 streamRoomId 也作为参数一并传入接口签名，保持 dispatch route 一致.

4. `handle` 可见性从 `private` 提升到 `internal`，让 `@testable import PetApp` 测试能直接调 `handle(message:streamRoomId:)` 验证守护契约 —— cross-room race 在 mock `disconnect()` 同步 `finish()` 旧 stream 的模型下不易构造端到端时序，最 robust 的回归测试是直接调内部 API 模拟瞬间状态.

5. 加 2 条回归测试（`testCrossRoomMemberJoinedFromOldStreamIsDiscarded` + `testHandleWithBothStreamRoomIdAndLastObservedRoomIdNilDiscardsMemberMessages`）覆盖：
   - room A 的 stale `member.joined` 不被 apply 到 room B
   - room A 的 stale `member.left` 不会移除 room B 的同 userId 成员
   - streamRoomId=nil + lastObservedRoomId=nil 边界（防御性兜底）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 client 端处理 **server 经房间级 fanout 投递、payload 不携带 room.id 的 WS 业务消息**（如 `member.joined` / `member.left` / `pet.state.changed` / `emoji.received`）时，**必须**在 consumer task 启动时**捕获**当时的 `lastObservedRoomId` 作为 `streamRoomId` 局部变量，handle 时**强制校验** `streamRoomId == lastObservedRoomId`，不匹配则丢弃 + log debug。**禁止**仅用 "`lastObservedRoomId != nil`" 这种"还在某房间"的弱守护。
>
> **展开**：
> - **判定条件**：触发本规则的 message 类型同时满足两条 ——
>   1. server 端按 fanout 范围保证"只投到该房间的 sessions"（即 server 不重复发到错误房间）
>   2. payload **不**含 room.id 字段（因协议钦定精简）
>   两条满足时即不能做 per-event payload-level room.id 校验，必须用 stream-lifecycle 层的 streamRoomId 守护
>
> - **why streamRoomId 而非 lastObservedRoomId**：A→B 切换瞬间的 `cancel + restart` 不能阻断 in-flight `await MainActor.run`；旧 task 若已 dequeue message + 已 await main-actor，投递时 `lastObservedRoomId` 已是新值（B），但 message 来自旧 stream（A）。`streamRoomId` 在 task 启动时一次性捕获，旧 task 持有的永远是旧值；与新 task 启动后的 `lastObservedRoomId`（新值）不匹配即丢弃，race 自然 resolve。
>
> - **streamRoomId == nil 边界处理**：`streamRoomId == nil && lastObservedRoomId == nil` 也应丢弃（已离开房间起的 task 不应发生但防御性兜底）—— guard 写法 `guard streamRoomId != nil, streamRoomId == lastObservedRoomId else { return }` 同时挡住"两边都 nil（stray task）"和"streamRoomId=A vs lastObservedRoomId=B（cross-room race）"两类 stale。
>
> - **room.snapshot 不要套用本规则**：`room.snapshot` payload 自带 `payload.room.id`，更精确的校验是 `payload.room.id == lastObservedRoomId`（12.1 r3 已落地），不要降级成 streamRoomId 守护 —— 后者粒度更粗（无法区分"同 room 内重连后旧 stream 残留 vs cross-room race"，但也不会误判，仅信息丢失）。混用：member.* 用 streamRoomId（无 payload room.id 可用），snapshot 用 payload-level room.id（更精确），dispatch 接口签名统一接 streamRoomId 参数即可（snapshot 分支不消费它）。
>
> - **测试构造提示**：cross-room race 的端到端时序在 mock `disconnect()` 同步 finish 旧 stream 的模型下**不可构造**（finish 后旧 task 立即退出 for-await，pending message 投递窗口极短无法稳定复现）。最 robust 的回归测试是把 handle 从 private 提升到 internal，让 `@testable import` 测试**直接调** `handle(message:streamRoomId: <旧 room>)` 模拟瞬间状态，断言 vm.members 未被 mutate。这种"暴露 internal hook 给单测"的做法在 race 路径不可观测的场景下是 acceptable trade-off（污染面小：仅多一个参数 + 可见性松一档）。
>
> - **反例**：
>   - 看到 review 说"cross-room race 也加 payload.room.id 校验"就盲目把 client 的 model layer 加 room.id 字段 → server 协议层不动的话，DTO 里没有 room.id，加了无源；而擅自要 server 在 payload 里加 room.id 又违反"协议精简"钦定。正确做法是**stream-lifecycle 层守护**，client 内部解决.
>   - 用"换房后立刻清理旧 stream / cancel 旧 task"以为可以避免 race → cancel 异步、in-flight delivery 不可阻断，必须配合"streamRoomId 校验"的不变量层防御。
>   - 把 `streamRoomId` 校验放到 `applyMemberJoined` / `applyMemberLeft` 内部 → 错；要在 `handle` dispatch 层就 reject，否则单测要测 apply 函数 + dispatch 函数两层路径，回归覆盖更难写.

## Meta: 本次 review 的宏观教训

**12.x 系列累计的 race 模式认知**（2026-05-09 同日 12.1 r1~r6 + 12.4 r1）：iOS 端通过 AsyncStream + `for await` + `Task` 消费 WS 消息是 actor-friendly 的，但任何**涉及房间状态切换的 mutation**（`lastObservedRoomId`、members 数组、wsState）都要意识到"task lifecycle 与 dispatch 之间存在不可消除的 race window"。守护策略要分层：

1. **payload 层**（最精确）：消息自带房间归属（如 `room.snapshot.room.id`）→ 用 payload 字段校验
2. **stream 层**：消息不带房间归属（payload 精简）→ 在 stream 启动时刻捕获 `lastObservedRoomId` 当 streamRoomId，dispatch 时校验
3. **state 层**（最弱）：仅靠 "`lastObservedRoomId != nil`" 这种"还在某房间"的存在性 —— 只够挡"已离开房间"的场景，挡不住 cross-room race，**不能**作为唯一防线

设计 client message handler 时应主动 enumerate 这三层，按消息类型选最强可用的那层；不要默认"server fanout 正确就够了"——server-correct ≠ client-correct，因为 client 自己的 task / actor 调度也会引入 race。
