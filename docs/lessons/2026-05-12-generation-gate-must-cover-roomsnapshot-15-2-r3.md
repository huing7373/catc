---
date: 2026-05-12
source_review: codex review round 3（epic-loop 自动派发；review 原文 `/tmp/epic-loop-review-15-2-r3.md`）
story: 15-2-pet-state-changed-ws-消息处理
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-12 — Generation gate 必须**全覆盖** state-mutating message，整体覆盖型 message 不能"凭其它校验"豁免

## 背景

Story 15.2 在 r2 决策反转后加了 `streamGeneration` per-stream identity gate，挡住 same-room rejoin（A→A leave-rejoin / 同房间 reconnect）race 中"旧 stream 的 stale event 投递到新 stream 已存在的 vm"的污染路径。r2 实装显式 exempt `.roomSnapshot` —— 理由是"snapshot 有 payload.room.id 校验 + 测试覆盖 generation=nil 路径下仍 apply"。

codex r3 指出该 exemption 是 r2 的**疏忽**，且 `.roomSnapshot` 反而是同根问题更**严重**的 vector：snapshot 是整体覆盖（applySnapshot 重写 members + memberPetStates），stale snapshot 一次性把新 stream 已 apply 的 roster + pet state 全部回滚 —— 危害远大于单字段 incremental event。本轮纠正该疏忽。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `.roomSnapshot` 未被 streamGeneration gate 守护 → same-room rejoin stale snapshot 整体覆盖新 stream 已 apply 的 roster/pet state | high | architecture | fix | `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:629-644` |

## Lesson 1: 整体覆盖型 message 必须**第一个**进 generation gate，不能"凭 payload-level 校验"豁免

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:629-644`

### 症状（Symptom）

Same-room rejoin（A→A leave-rejoin / 同房间 reconnect）路径下：
- 旧 stream 的 task 在 cancel 前已 dequeue 了一个 stale `.roomSnapshot` for room A（payload.room.id="room_A"）；
- 新 stream 启动后 vm 已 apply fresh snapshot（roster=u_alice+u_charlie，pet state=.walk）；
- 旧 task 把 stale snapshot 投递到 main actor 时：
  - generation gate 前置 switch case 仅守护 `.memberJoined / .memberLeft / .petStateChanged / .connectionStateChanged`，**显式跳过** `.roomSnapshot`；
  - 后续 per-case 校验 `payload.room.id == lastObservedRoomId` 两端都是 "room_A" → 放行；
  - `applySnapshot(staleV1)` 把 roster 整体回滚到 [u_alice, u_bob]，u_charlie 消失、u_bob 死灰复燃，pet state 回滚到 .rest。

### 根因（Root cause）

R2 实装写 `.roomSnapshot` exempt 时的隐含假设是"snapshot 有 payload.room.id 校验，且 testCase#10 钦定 generation=nil 路径下 stale-room-A snapshot 也要被丢弃"——所以"snapshot 已被守护，不需要再过 generation gate"。

漏洞在于：
1. **payload.room.id 校验只能挡跨房间 race（A→B），挡不住 same-room race（A→A）** —— 两层守护针对的 race 维度不同，不可互相替代；
2. **整体覆盖型 message 的 stale 危害 ≠ incremental message 的 stale 危害** —— `.memberJoined` stale 只多塞一个成员，`.roomSnapshot` stale 会一次性回滚整个 roster + pet state，应被**优先**而非**豁免**守护；
3. **测试兼容性 vs 正确性的优先级颠倒** —— "保持 testCase#10 钦定 generation=nil 路径下 stale snapshot 仍要被 payload-level 守护拦下" 是测试**兼容性**问题，应通过"generation gate 仅在 caller 显式传 generation 时启用（默认 nil 跳过）"来满足，不应通过把 `.roomSnapshot` 永久 exempt 在 gate 外来满足。两层 gate 都该在 production 路径生效，测试路径靠 caller 不传 generation 自然跳过 generation gate 即可。

### 修复（Fix）

把 `.roomSnapshot` 纳入 generation gate 守护范围：

```swift
// before
switch message {
case .memberJoined, .memberLeft, .petStateChanged, .connectionStateChanged:
    return  // discard
case .roomSnapshot, .pong, .error, .unknown:
    break  // 走原 per-case 校验路径
}

// after
switch message {
case .memberJoined, .memberLeft, .petStateChanged, .connectionStateChanged, .roomSnapshot:
    return  // discard
case .pong, .error, .unknown:
    break  // 走原 per-case 校验路径（pong/error/unknown 无 state mutation，无需 gate）
}
```

- `.roomSnapshot` 的 per-case `payload.room.id == lastObservedRoomId` 守护**保留** —— 它仍负责跨房间 race（A→B）维度的拦截，与 generation gate 形成两层防御深度；
- 加测试 `testSameRoomRejoinRoomSnapshotFromOldGenerationIsDiscarded`：snapshot v1（baseline）→ prepareForReconnect 翻 gen → snapshot v2（fresh）→ 模拟旧 task 投递 stale v1 (with oldGen) → 断言 roster + pet state 保持 v2，不被回滚；
- production 路径（`startConsumingMessages` 启动 task 时捕获 client.streamGeneration）不变，依然给 5 个 state-mutating case 都注入 generation；
- 既有测试（多数直调 handle(...) 时不传 streamGeneration，保持默认 nil）继续工作 —— generation gate 仅在 caller 显式传 generation 时启用。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在为 stream consumer 的"per-stream identity gate"做 case allowlist 时，**禁止**以"该 case 已有其它（payload-level / streamRoomId）校验"为由把它 exempt —— **必须**把所有 state-mutating message 全部纳入 gate，不同维度的守护互相**叠加**（防御深度）而非**替代**。
>
> **展开**：
> - **判定标准**：一个 message case 是否需要进 generation gate，唯一标准是"它是否 mutate vm 状态" —— `.pong / .error / .unknown` 是无副作用 case（仅 log），可豁免；任何 mutate state 的 case（`.roomSnapshot / .memberJoined / .memberLeft / .petStateChanged / .connectionStateChanged / ...`）**必须**进 gate；
> - **整体覆盖型 message 优先级最高**：snapshot / 全量刷新这类整体覆盖 message 的 stale 危害远大于 incremental delta message —— 一次性回滚多个字段，UI 失真更明显；在 gate allowlist 设计阶段应**第一个**列入，不能因"snapshot 已有 payload-level 校验"就豁免；
> - **不同守护维度互补不互斥**：`streamRoomId` 挡跨房间 race（A→B，roomId 维度），`streamGeneration` 挡同房间 race（A→A leave-rejoin，stream-instance 维度），`payload.room.id` 挡 cross-stream 跨房间 race —— 三者维度正交，必须**同时**生效，不能"凭其中一种就跳过另一种"；
> - **测试兼容性问题不应用"豁免"解决**：若新 gate 想保持向后兼容已有测试（不传 generation），用"caller 显式传非 nil 才启用 gate"的 opt-in 机制（gate condition 加 `if let streamGen = streamGeneration, ...`），而**不是**把某个 case 永久 exempt 在 gate 外；前者只影响测试路径，后者会污染 production 路径；
> - **反例（必须避免）**：
>   - "snapshot 的 payload 自带 room.id，已有 payload-level guard，不需要进 generation gate" → 错。Same-room (A→A) race 下 payload.room.id == lastObservedRoomId 两端都对，guard 直接放行；
>   - "snapshot 整体覆盖语义本身就是 server 权威，不应丢" → 错。Server 权威的前提是**当前 stream**的 snapshot；旧 stream 的 snapshot 已经过期（server 端继续 broadcast 是因为 client 还没 close），按 fresh stream 优先；
>   - "为了保持已有测试不传 generation 路径下 stale snapshot 仍被丢弃" → 错。让 gate opt-in（caller 显式传 generation 才启用），测试默认 nil 自然走原 per-case 路径，production 必传非 nil 必走 gate。

## Meta: 本次 review 的宏观教训

R1 → R2 → R3 三轮 review 揭示了 streamGeneration gate 的设计演化：
- **R1**：发现 same-room rejoin race（defer，未实装）；
- **R2**：实装 streamGeneration gate，但只盖了 4 个 incremental case（`.memberJoined / .memberLeft / .petStateChanged / .connectionStateChanged`），显式 exempt `.roomSnapshot`；
- **R3**：纠正 R2 的 exemption —— `.roomSnapshot` 必须同样进 gate，且应优先列入。

**meta lesson**：**设计新的 cross-cutting 守护机制（gate / interceptor / middleware）时，allowlist 必须**默认覆盖全部 state-mutating 路径**，而不是"先盖一部分，等出问题再补"——后者会把 review 拆成多轮，每轮只发现一个 case，浪费 review bandwidth；且每次 exemption 都会留下"再下一个 reviewer 才能发现"的尾巴**。

具体到 generation gate 设计原则（写入 ADR / 注释，便于未来 Claude 抄）：
1. 列出当前 protocol 所有 message case；
2. 标记每个 case 的副作用类型（state mutation / IO / no-op）；
3. **所有 state mutation case 必须进 gate**；no-op case（log / metrics）可豁免；
4. 不要根据"这个 case 有其它守护"做豁免 —— 不同守护的 race 维度通常不正交。
