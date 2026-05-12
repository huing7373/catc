---
date: 2026-05-12
source_review: codex review (round 2) — /tmp/epic-loop-review-15-2-r2.md（末尾 ^codex$ 段，重新 flag r1 已 defer 的同一个 P2）
story: 15-2-pet-state-changed-ws-消息处理
commit: 850d31f
lesson_count: 1
related_lesson: 2026-05-12-pet-state-changed-stream-roomid-guard-residual-risk-same-room-rejoin-15-2-r1.md
---

# Review Lessons — 2026-05-12 — per-stream generation guard 修同房间 rejoin race，决策反转：从 r1 defer 升级为 r2 fix（15-2 r2）

## 背景

上轮 codex r1 flagged P2 = `streamRoomId == lastObservedRoomId` 守护在 same-room rejoin / same-room reconnect 路径下失效。当时基于"同坑跨 4 case + WebSocketClient protocol 缺 stream-id API + Story 15.5 snapshot 重对齐为兜底"三条理由 defer 到 15.5 统一重做（见 r1 lesson）。

本轮 codex 重新 flag 同一处。代码 review 看不到 lesson 文档（不在 codex 默认 context 里）—— 它只看 diff，发现 r2 仍未修复，再次给出 P2。**P2 不属于 fix-review 工作流的"次要 finding"白名单（仅 nit / style 允许 defer-twice）**；同一 P2 finding 不能 defer 两次 → 本轮决策反转：升级为 fix。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | per-stream generation guard 修 same-room rejoin / reconnect race（4 case 同时改：memberJoined / memberLeft / petStateChanged / connectionStateChanged） | P2 | architecture (concurrency) | **fix** | `iphone/PetApp/Core/Networking/WebSocketClient.swift`, `WebSocketClientImpl.swift`, `WebSocketClientMock.swift`, `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift` |

## Lesson 1: per-stream generation guard 修同房间 rejoin race —— 决策反转 + 跨 4 case 统一升级

- **Severity**: P2
- **Category**: architecture (concurrency)
- **分诊**: **fix**（推翻 r1 defer 决定）
- **位置**:
  - `iphone/PetApp/Core/Networking/WebSocketClient.swift:28-46`（protocol 新增 `streamGeneration: Int { get }`）
  - `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift:202` + `:303`（`streamGenerationStorage` 字段重命名 + lock-protected getter）
  - `iphone/PetApp/Core/Networking/WebSocketClientMock.swift:25-30` + `:113`（`_streamGeneration` 字段 + `prepareForReconnect()` 翻 +1）
  - `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:541-565`（`startConsumingMessages` 同时捕获 streamRoomId + streamGeneration）
  - `iphone/PetApp/Features/Room/ViewModels/RealRoomViewModel.swift:595-636`（`handle(message:streamRoomId:streamGeneration:)` 新增 generation gate）

### 症状（Symptom）

见 r1 lesson §症状（不重复）。简言之：same-room leave-rejoin（A → nil → A）/ same-room reconnect（A → 网络断 → 自动重连 A）路径下，新旧 stream 的 `streamRoomId` 都是同一个 room，仅靠 roomId 维度的 guard 完全无差别化能力 → 旧 stream 的 stale `.petStateChanged` / `.member.*` / `.connectionStateChanged` 错误覆盖新 snapshot 派生的 state.

### 根因（Root cause）

参见 r1 lesson §根因（不重复）。补充 r1 → r2 的**决策路径**根因：

1. **r1 defer 三理由的实际成本评估**：
   - "同坑跨 4 case，单点修引入不一致" —— 反过来看，这恰好是**应该一次修 4 case** 的理由，而非 defer 的理由。本轮一次性修 4 case 即解决.
   - "WebSocketClient protocol 缺 stream-id API" —— 实际上 `WebSocketClientImpl` 内部早已有 `streamGeneration: Int` 字段（fix-review round 4 P2 引入，Story 12.5），仅未通过 protocol 暴露。本轮通过 protocol getter 暴露 + Mock 同步实装即可，**不是新增"无中生有"的概念**.
   - "Story 15.5 snapshot 重对齐为兜底" —— snapshot 兜底确实存在，但它要求"下一次 snapshot / state-sync 触发"才自愈；race window 内 UI 显示错误状态对用户可见.
2. **codex 不读 lesson 文档**：lesson 是给 future Claude 的蒸馏语料，但 codex review 是 stateless / 仅看 diff。defer 决策只能挡住"同一份 review 上下文里的 future 提醒"，挡不住"下一轮 review 重新 flag"。fix-review 工作流明确 P2 不能 defer-twice.
3. **per-stream generation 是 cheap 的设计**：相比 stream UUID（额外字段 + Equatable / Hashable / Sendable）/ stream-bound enum case payload（破坏现有 WSMessage enum）等方案，generation `Int` 是最小侵入的方式 —— `Impl` 内部本来就有 + `Mock` 加 1 行 + `RealRoomViewModel.startConsumingMessages` 多捕获 1 个 `Int` + `handle` 入口加一个前置 gate.

### 修复（Fix）

**1. `WebSocketClient` protocol（WebSocketClient.swift:28-46）**：新增 `var streamGeneration: Int { get }`，doc 锁定 "仅 `prepareForReconnect()` 翻 +1，`connect/disconnect` 不动" 语义.

**2. `WebSocketClientImpl`（WebSocketClientImpl.swift:202 / :297-307）**：
- 把私有字段 `streamGeneration` 重命名为 `streamGenerationStorage`（避免与 protocol getter 同名 ambiguity）
- 新增 public `var streamGeneration: Int` 计算属性，lock-protected 读 storage
- 内部 4 处 raw 字段访问（receive-loop launch 抓 myStreamGen / `prepareForReconnect()` += 1 / `yieldIfCurrent` / `finishStreamIfCurrent`）全部改为 `streamGenerationStorage`，**避免持锁状态下重入获取同一 NSLock 引发死锁**.

**3. `WebSocketClientMock`（WebSocketClientMock.swift:19-30 / :109-118）**：
- 新增 `private var _streamGeneration: Int = 0` + `public var streamGeneration: Int { _streamGeneration }`
- `prepareForReconnect()` 在 swap stream 时同时 `_streamGeneration += 1`

**4. `RealRoomViewModel.startConsumingMessages`（RealRoomViewModel.swift:541-565）**：
- consumer task 启动前同时 snapshot `let streamRoomId = self.lastObservedRoomId` **和** `let streamGeneration = client.streamGeneration`
- handle 调用时同传两个 captured snapshot

**5. `RealRoomViewModel.handle(message:streamRoomId:streamGeneration:)`（RealRoomViewModel.swift:595-636）**：
- 签名新增 `streamGeneration: Int? = nil`（默认 nil 保证既有 test fixture 不破）
- switch 之前先做 generation gate（仅当 caller 传入 non-nil 且当前 client.streamGeneration 不一致时丢弃）
- 仅对 4 个守护 case 应用（memberJoined / memberLeft / petStateChanged / connectionStateChanged）；roomSnapshot 走 payload.room.id 校验 + 测试覆盖 generation=nil 路径，跳过 generation gate；pong / error / unknown 无副作用，不挡

**6. 测试（RealRoomViewModelTests.swift）**：
- 新增 `testSameRoomRejoinPetStateChangedFromOldGenerationIsDiscarded`：验证 generation 不匹配时 pet.state.changed 被丢弃 + generation 匹配时正常 apply
- 新增 `testSameRoomRejoinAllFourCasesAreDiscardedByGenerationGate`：验证 4 个 case 全部受 generation gate 保护
- 测试 robust 处理 baseline generation 起点（vm 初始化路径可能已经调过 prepareForReconnect → 不假设 generation 从 0 起，用 `oldGen = mockWS.streamGeneration` snapshot baseline）

**7. r1 lesson 文档**：保留不删；本 r2 lesson 在 frontmatter `related_lesson` 字段反链 r1，未来读 r1 的 future Claude 通过此链能找到 r2 的实际修复.

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **WS / async-stream 类 ViewModel 设计跨 stream race 守护**时，**必须**用**多维 identity**（roomId + generation）而非单一 roomId 维度，**且**任何"翻 stream identity 的 client 内部 swap"都必须通过 protocol getter 暴露给 ViewModel 守护层使用.
>
> **展开**：
> - **守护维度判定矩阵**：列出所有"会导致旧 stream 残留"的路径（cross-room A→B / same-room leave-rejoin A→A / same-room transparent reconnect / app 切后台 → 重连 / token 失效后 reconnect 等），逐路径校验"现有 guard 维度能否区分新旧 stream"。任何一路径下新旧 stream 的某守护字段相等 → 该字段不够 → 必须加新维度.
> - **client 内部 swap 必须暴露**：`WebSocketClientImpl` 已有 `streamGeneration` / `sessionGeneration` 等内部字段是用来防 client 自己内部 race 的；ViewModel 也有 race（与 stream 生命周期同源），所以这些字段**必须**通过 protocol 接口暴露给 ViewModel，否则 ViewModel 写不出对称的 guard.
> - **generation Int 是最便宜的 stream-identity**：相比 UUID / Date / stream-bound payload，`Int` 字段 cheap、单调、Sendable、跨 actor 安全；只要保证"swap 时 +1，read 时 == compare" 即可.
> - **lock-protected getter 必须用 storage 别名**：在 NSLock-based class 里，public computed property 走 `lock.lock` → `read storage` → `lock.unlock`；内部已持锁路径**必须**直接读 storage（重命名为 `xxxStorage`），否则在持锁块内访问 public getter → 同 lock 重入 → deadlock.
> - **Mock 必须实装与 production 同语义的 generation 翻新点**：`prepareForReconnect()` 是 protocol 契约 swap 点；Mock / Impl 都在此点 +1；任何 mock 漏 +1 → ViewModel 单测无法复现 same-room race，bug 漏到端到端.
> - **新增可选参数 + 默认值保护既有测试**：`handle(message:streamRoomId:streamGeneration:)` 把新参数设默认 `nil`，既有 60+ 测试 fixture 不需改；production caller `startConsumingMessages` 必传非 nil；测试新写专项 case 显式覆盖 generation gate.
> - **defer 决策必须考虑 codex stateless 重审风险**：codex / 任何无文档读取能力的 reviewer 不读 lesson；只看 diff. P2 finding defer 一轮可以（一次性 review-respond），defer 两轮就是 fix-review 工作流违规. 真正"应该 defer"的 finding 应该满足：① P3/nit 而非 P2+；② 修复显著超出当前 scope（>1 commit / 跨 epic）；③ defer 时已开新 story 跟踪.
> - **反例（旧实装）**：
>   ```swift
>   guard streamRoomId != nil, streamRoomId == lastObservedRoomId else { return }  // 仅 roomId 维度
>   ```
>   — same-room rejoin（A→A）下新旧 stream 的 streamRoomId 都是 A → 守护无差别化能力 → 旧 stale event 错误 apply.
> - **正例（修复后）**：
>   ```swift
>   // 1) 启动 task 时同时 snapshot roomId 和 generation
>   let streamRoomId = self.lastObservedRoomId
>   let streamGeneration = client.streamGeneration
>   // 2) handle 入口前置 generation gate（4 case 全覆盖）
>   if let myGen = streamGeneration,
>      let curGen = webSocketClient?.streamGeneration,
>      myGen != curGen {
>       switch message {
>       case .memberJoined, .memberLeft, .petStateChanged, .connectionStateChanged:
>           return  // stale stream，丢弃
>       default: break
>       }
>   }
>   // 3) per-case 仍保留 roomId guard（防 cross-room race）
>   ```

## Meta: 本轮的宏观教训

- **fix-review 工作流的 defer 政策需要语义升级**：原文档允许 defer 但未明示"同一 P2 重审必须 fix"。本轮主 agent override（task 描述里）补全了这一规则。未来 fix-review 命令本身可考虑硬编码"P2+ 不允许 defer-twice"checks.
- **lesson 文档要给当下的 codex 也读**：当前 codex review 不在 context 里看 lesson；解法路径：
  1. 在 commit message body 里 inline 引用相关 lesson 关键结论（让看 git log 的下一轮 reviewer 看到）
  2. 在源码注释里 inline lesson 摘要 + 文件路径反链（让看 diff 的 reviewer 看到 —— 本轮已在 RealRoomViewModel.swift `handle` doc 中加 fix-review round 2 P2 章节）
  3. 长期方向：把 lessons 蒸馏成 cheatsheet 注入到 review 的系统 prompt（需要 epic-loop 工作流改）
