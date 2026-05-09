---
date: 2026-05-09
source_review: codex review (round 3) — Story 12.5 reconnect 状态机
story: 12-5-自动重连
commit: 355fbe5
lesson_count: 1
---

# Review Lessons — 2026-05-09 — WS reconnect: connectGate 也要 generation 守护（Story 12.5 r3）

## 背景

r2 已经把 reconnect 状态机里的 `currentContinuation` / `scheduleReconnect` / `attemptReconnect` 三条
共享状态写入路径都收口到 `*IfCurrent(mySession:)` 包装里，每条入口持锁做 `mySession == sessionGeneration`
generation 校验。

r3 codex review 又找到一条**同 family 但漏网**的共享状态写入入口：`connectGate`（即 connect 握手的
`CheckedContinuation<Void, Error>`）—— 旧实装的 `resolveConnectGate(success:error:)` 没有 generation 校验，
任何持有 `WebSocketClientImpl` 弱引用的 stale receive-task 在 defer / catch 块里调它都会无差别 resolve
当前 in-flight connect 的 gate。

具体复现路径：

1. 第一次 connect 成功 → transient close 4005 → schedule reconnect attempt 1。
2. attempt 1 的 receive-task 卡在 `receive()` 等首帧。
3. caller 调 `disconnect()` —— cancel attempt 1 的 receive-task + 翻 generation，但尚未触发 in-flight
   connect 的 gate（disconnect 时 connectGate 是 nil，因为没有正在 await 的 connect）。
4. caller 立即 `connect(roomId: "ROOM_C")` → fresh connectInternal install 新 gate (mySession = N+1)。
5. attempt 1 的 receive-task 因为 cancel 信号 throw `URLError(.cancelled)` → 走 receive-loop 的 defer
   块 → defer 块在 `!firstFrameReceived` 分支调 `resolveConnectGate(success: false, ...)` →
   旧实装无 generation check → resolve 新 session 的 gate → fresh connect 抛
   `WSError.connectionFailed("receive task cancelled before first frame")`。

这是典型的"r2 修了大头，留了一个未被 audit 的子入口"漏网漏洞 —— `connectGate` 不在 r2 列出的"三条
共享状态写入路径"里，因为它的语义是"connect 握手的 one-shot 同步原语"而非"长寿命 stream"，但它的
**写入接缝**（`resolveConnectGate`）和长寿命 stream 一样会被 stale receive-task 持有并跨 generation 调用。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | stale receive-task defer / catch 路径 resolve 新 session 的 connectGate | P1 | architecture | fix | `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift` |

## Lesson 1: generation counter 模式必须覆盖**所有**共享状态写入入口，不能漏掉"one-shot 同步原语"

- **Severity**: P1 / high
- **Category**: architecture (concurrency)
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Networking/WebSocketClientImpl.swift` `resolveConnectGate(...)` 及所有 callsite

### 症状

stale receive-task 的 defer 在新 connect 已 install 新 gate 之后才跑，stale defer 调
`resolveConnectGate(success: false, error: ...cancelled before first frame...)` 直接 resolve 新 session
的 gate → fresh `connect(roomId:)` 抛"receive task cancelled before first frame" 这种与 fresh session
无关的 stale failure。

用户视角：刚刚自己手动断开换房间，新房间立刻报"连接失败：receive task cancelled" —— 完全错位的 UX。

### 根因

`connectGate` 在 r2 设计 generation 守护时被忽略，原因有二：

1. **语义错位**：r2 review 关注的是"long-lived stream 的 emit/schedule 接缝"——
   `currentContinuation` 是 AsyncStream 的 yield endpoint（多次写）；`reconnectTask` 是 retry 调度的
   handle（多次 swap）；`scheduleReconnect` 是 retry 流的入口（多次 schedule）。`connectGate` 在
   r2 mental model 里属于"connect 自己用一次就丢"的 one-shot 同步原语，不属于"长寿命共享状态"那一档。
2. **写入入口不在审计 checklist**：r2 的修法是把所有 emit / yield / schedule 接缝替换成
   `*IfCurrent(mySession:)` 包装。但 `resolveConnectGate` 不在这三个语义类别里，被 grep
   `emitConnectionState\|scheduleReconnect` 漏过去；review 也没列出它。

但 `connectGate` 实际上**是**共享状态：它是 instance field（`private var connectGate: ...?`），任何
持有 `self` 弱引用的 task 都能写它。stale receive-task 的 defer 持 weak self 跨 generation 调用
`resolveConnectGate` 时，与 stale receive-loop 跨 generation 调用 `emitConnectionState` 是**一模一样的
race 模式**——只是被 r2 的语义分类挡住了视野。

更深层根因：**generation counter 是一个"任何会跨 generation 被 stale task 调用的写入入口都要守护"
的统一模式**。一旦决定用 generation counter，就要审计**所有**实例字段的写入接缝 —— 包括 one-shot 同步
原语 `CheckedContinuation`、包括"看起来 disposable" 的中间字段、包括"似乎只有当前 session 会调"的
private helper。**有没有 stale caller 取决于 task lifecycle，不取决于字段语义**。

### 修复

1. 新增 `private var connectGateOwnerSession: Int?` 字段：与 `connectGate` 配对的 owner generation。
2. `connectInternal` install gate 时持锁记录 `connectGateOwnerSession = mySession`。
3. 把 `resolveConnectGate(success:error:)` 改名 + 加 `mySession: Int` 参数 →
   `resolveConnectGate(success:error:mySession:)`。函数体持锁内 `guard connectGateOwnerSession == mySession
   else { silent drop }`，不匹配 silent drop（log debug）+ 不写 `connectGate`。
4. 所有 receive-loop / catch path 内的 `resolveConnectGate` callsite 都传 `mySession`（即 receive-task
   launch 时捕获的 generation 闭包常量）。
5. 新增 `resolveConnectGateUnconditionally(success:error:)`：专供 `disconnect()` / `prepareForReconnect()`
   / `deinit` 三个**显式**想 fail 当前 in-flight connect 的路径调用（这三类 caller 已经在持锁内
   `sessionGeneration += 1` 翻 generation；如果用 generation-gated 的版本会被自己翻新的 generation 拦下，
   语义错）。
6. success path（receive 到首帧）的 `resolveConnectGate(success: true, ...)` 也一并改成 generation-gated
   版本 —— 极端时序下 stale receive-task 的 first frame 可能在新 connect 已 install 新 gate 之后才到，
   stale `resume(())` 会让 fresh connect 拿到 spurious success（连错 session）。

测试覆盖（新增 1 个 case）：

- `test_reconnect_staleReceiveTaskDeferDoesNotResolveFreshConnectGate`：firstTask snapshot 解 latch +
  4005 transient close → attempt 1 receive-task 卡在 receive 永久 block → caller `disconnect()` cancel
  attempt 1 + 翻 gen → fresh `connect(roomId: "ROOM_C")` 进 connectInternal install 新 gate → 等
  1 秒让 stale defer 跑完 → 验证 fresh connect **没有提前 finish**（既不 success 也不 throw stale
  failure）—— 旧实装会 throw "receive task cancelled before first frame"，修复后 silent drop fresh
  connect 仍在 await。

### 预防规则（Rule for future Claude）

> **一句话**：决定用 generation counter 隔离 stale task 时，未来 Claude **必须** grep / 审计**所有**
> 实例字段的写入入口（包括 `CheckedContinuation` resolve / one-shot 同步原语 / 中间状态 swap），把
> "long-lived stream 写入"和"短命同步原语 resolve" **同等对待** —— 二者都会被 stale task 跨 generation
> 调用，race 模式相同。
>
> **展开**：
>
> - 引入 generation counter 时建立审计 checklist。每个实例字段问三个问题：
>   1. 谁会写这个字段？（grep `self.<field> =` / `self?.<method>(`）
>   2. 写入路径里有没有 launch 出去的长寿命 task？（receive-loop / scheduled retry / detached cancel
>      handler）
>   3. 这些 task 会不会跨 caller 的 reset 边界存活？（disconnect / prepareForReconnect / fresh connect
>      之间的窗口）
>   三个问题中任意一个答 yes —— 这个字段的写入接缝**必须**有 generation 守护。
> - **不要按"语义分类"排除字段**。"这是 one-shot 的"、"这只在 connect 内用"、"这只 success path 写"
>   都不是排除依据。决定权在**字段的 lifetime**和**调用者的 lifetime**有没有跨 generation 重叠的可能，
>   不在字段的 conceptual role。
> - **`CheckedContinuation` resolve 是隐藏的写入接缝**：它没有显式赋值语法（`cont.resume()` 看起来不
>   像写共享状态），但语义上等价于"把 result 写到 caller 的 await 现场" —— 这是最强的共享状态写入。
>   stale resolve = stale write to caller's await result。
> - **owner generation 字段的命名约定**：每个被 generation-gated 的 continuation / handle 字段配一个
>   `<field>OwnerSession: Int?`，install 时一同写，resolve 时一同读 + 校验 + 一同清。`nil` 表示"无
>   owner / 无 in-flight"。
> - **拆分 conditional vs unconditional resolve**：generation-gated 的版本服务"stale task 自动跌出"
>   场景；unconditional 的版本服务"caller 主动放弃当前 in-flight"场景（`disconnect` / `prepareForReconnect` /
>   `deinit`）。两个版本不能合并 —— caller 主动放弃的路径已经翻 gen，再走 generation-gated 版本会被自己
>   翻新的 generation 拦下，永远 silent drop，语义错。
> - **success path 也要 generation-gated**：极端时序下 stale task 的"成功"也是 stale 成功，spurious
>   success 比 spurious failure 更难 debug（fresh connect 拿到不属于自己的 session 的 OK，后续业务
>   逻辑全错位）。
> - **反例**：
>   - "这个 continuation 只 connect 内 resolve，不会被 stale task 调到" —— 错。`resolveConnectGate`
>     callsite 在 receive-loop 的 defer 块，receive-loop task 持 weak self，跨 generation 存活的概率
>     与 receive-loop 主体一样。
>   - "stale task 已经被 cancel 了，cancel 之后不会跑代码" —— 错。`Task` cancel 是协作式的，cancel 信号
>     传过去之前 task 的 unwind 会跑 defer / catch 块 N 行代码，这 N 行里任何写共享状态的入口都会触发
>     race。
>   - "把 generation check 加到 callsite 而不加到 resolve 函数本身" —— 不行。callsite 是分散的（defer /
>     catch / multiple branches），逐 callsite 加 check 容易漏；把 check 收到 resolve 函数入口（持锁
>     guard）是单点防御。

---

## Meta: 本次 review 的宏观教训

r2 → r3 暴露了一个 review 流程上的盲点：**"按 finding 修 finding"是不够的，必须做 family-wide audit**。

r2 的两条 finding 都属于"stale task 跨 generation 写共享状态"family。修完之后 review 应当主动问：
"这个 family 还有哪些其他实例？" —— 实例字段维度的 grep（`private var .*Continuation` /
`private var .*Task` / `private var .*Gate`）会立刻暴露 `connectGate` 没被守护。

future Claude 在修 generation/race 类 finding 时，套路：

1. 修单个 finding 之后，列出本类的"通用模式"（这次是"实例字段写入接缝必须 generation-gated"）。
2. 用通用模式 grep 全文，列出所有候选写入接缝。
3. 对每个候选问"是否会被 stale task 跨 generation 调用"。
4. 漏掉任意一个 → 下一轮 review 必复现。

这个 audit 应当在 r2 就做完，省一轮 r3。下次起手提醒自己：**generation counter 一旦引入就是全字段
mode，不是单点修复 mode**。
