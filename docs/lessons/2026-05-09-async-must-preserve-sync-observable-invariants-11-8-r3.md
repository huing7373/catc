---
date: 2026-05-09
source_review: codex review r3 on Story 11.8 (file: /tmp/epic-loop-review-11-8-r3.md)
story: 11-8-成员加入-离开-ws-广播
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-09 — 异步化必须保留同步可观察 invariants（11.8 r3：joiner self-fanout / leaver stale subscription）

## 背景

Story 11.8 r2 fix 把 `JoinRoom` 的 `broadcastMemberJoined` 与 `LeaveRoom` 的 `closeLeaverSession + broadcastMemberLeft` 整体放进 post-commit goroutine（`runPostCommitAsync`），目的是用 detached ctx + 10s timeout 兜底。但 r2 的整体异步化破坏了两条 V1 协议钦定的同步可观察 invariant：

1. **R1 joiner self-fanout**（V1 §12.3 行 2063 钦定 "joiner 不收自己的 member.joined"）：HTTP join 200 → client 立即建 WS → joiner Session 完成 SessionManager.Register → 异步 goroutine 此时才开始 `BroadcastToRoom` fanout → 列表含 joiner Session → joiner 收到自己的 member.joined。r1 同步路径下这个 race 是被"broadcast 入队时加入者 WS 还没握手"的隐含 race-free 假设兜住的，r2 异步化后假设不再成立。

2. **R2 leaver stale subscription**（V1 §10.5 步骤 7 钦定 "HTTP leave immediately detaches WS"）：HTTP leave 200 → leaver Session 仍在 SessionManager（CloseWithCode 异步进行中）→ 期间任何 broadcast（如另一 user 的 join）仍 fanout 给 stale leaver session → leaver 收到 stale member.joined。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Joiner 收到自己的 member.joined（async broadcastMemberJoined race） | high | architecture | fix | `server/internal/service/room_service.go:658-660`、`server/internal/app/ws/broadcast.go` |
| 2 | LeaveRoom 200 后 leaver 仍在 SessionManager（async closeLeaverSession 整体放后台） | high | architecture | fix | `server/internal/service/room_service.go:776-778` |

## Lesson 1: 异步化 broadcast 必须显式排除事件主体（不能依赖时序假设）

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/service/room_service.go:658-660` + `server/internal/app/ws/broadcast.go`

### 症状（Symptom）

`JoinRoom` 把 `broadcastMemberJoined` 放进 post-commit goroutine 后，client 收到 HTTP 200 立刻建立 WS（join 成功后 client UI 自然过渡到房间页面，前端立即 connect WS），joiner Session 完成 `SessionManager.Register`。然后 post-commit goroutine 才执行 `BroadcastToRoom`，它调 `ListSessionsByRoomID` 拿到的列表此时**已经包含 joiner 自己**。结果：joiner 收到一条自己加入自己的 member.joined（违反 V1 §12.3 行 2063 "广播范围：仅该房间内当前在线的其他 Session（不含加入者自己）"）。

### 根因（Root cause）

r1 的同步实装下 broadcast 在 HTTP 200 返回**之前**完成，所以"join → broadcast → return 200 → client 才有机会建 WS"是同一 goroutine 内的严格顺序。设计文档/代码注释里有"加入者收不到自己的 member.joined"的约定，但**没有把它显式化为 server 责任** —— 反而在代码注释里把它解释为"节点 4 阶段时序事实上加入者尚未建立该 roomID 的 WS 连接"。这是一条隐含的、依赖于"sync ordering"的 race-free 假设。

r2 fix 把 broadcast 改成异步（detached ctx + 独立 goroutine + 10s timeout）以解决"request ctx cancel 误中断 broadcast"问题（codex r2 P2），但**没有意识到**这条隐含假设是 sync 实装的副产品 —— 异步化后 client 完全有时间在 post-commit goroutine 之前建立 WS Register，列表里就有 joiner 自己。

**思维漏洞**：当把同步代码改成异步时，只看了"caller 不阻塞" / "ctx cancel 不影响" 这类**性能/可靠性** invariants，忽略了"调用方可观察的中间状态" / "外部时序对协议语义的影响"这类**协议正确性** invariants。

### 修复（Fix）

在 `ws/broadcast.go` 增加 `BroadcastToRoomExcept(ctx, mgr, roomID, excludeUserID, msg)` primitive + 对应 `BroadcastExceptFn` type alias；fanout 时显式跳过 `Session.UserID() == excludeUserID` 的 Session。`broadcastMemberJoined` / `broadcastMemberLeft` 改用 `s.broadcastExceptFn(ctx, roomID, eventSubjectUserID, msg)` 显式 exclude joiner / leaver UserID。

```go
// ws/broadcast.go r3 加：
type BroadcastExceptFn func(ctx context.Context, roomID, excludeUserID uint64, msg []byte) (sent int, err error)
func BroadcastToRoomExcept(ctx, mgr, roomID, excludeUserID, msg) (sent int, err error)

// service/room_service.go r3 改：
// before: s.broadcastFn(ctx, roomID, msgBytes)
// after:  s.broadcastExceptFn(ctx, roomID, joinerUserID, msgBytes)
```

`broadcastMemberLeft` 也用 `BroadcastToRoomExcept` 排除 leaver UserID（双保险 / belt-and-suspenders）—— 即使 closeLeaverSessionSync 已让 leaver 从 ListSessionsByRoomID 列表消失，broadcast 路径再显式过滤一遍能防御未来潜在 race。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**把同步路径改成异步路径**（goroutine / queue / detached ctx）时，**必须**枚举该路径所有"协议钦定的同步可观察 invariants"并**显式实装**它们 —— **绝不**把"sync 实装的隐含 race-free 假设"当作异步化后仍成立的属性。
>
> **展开**：
> - 改异步前先 grep 该路径上下游所有 spec / docs / V1 协议文档里"广播范围 / 顺序 / 立即生效 / authoritative signal"等关键词，列出每条**外部可观察**约束（client 看到什么 / 不看到什么 / 看到的顺序），逐条评估异步化后是否还能保持
> - 对每条 fail 的约束，设计 server 端**主动**实装路径（如：本案例的 `BroadcastToRoomExcept` 显式 exclude，而不是被动依赖时序）
> - 异步化的"驱动力"通常是**性能/可靠性**（避免阻塞 / 避免 ctx cancel）；这些 invariant 通常**不是性能问题**而是**正确性问题**，两类不能互相代偿
> - **反例**（本案例）：r1 的注释写了"加入者收不到自己的 member.joined（节点 4 阶段时序事实上加入者尚未建立 WS）"—— 看到这种"事实上 / 时序上 / 通常不会"的措辞要警觉，往往代表**没有显式实装**的隐含约束。改异步前必须把这类约束变成显式的 server 行为
> - **反例**（本案例 broadcastMemberLeft 路径）：r2 实装依赖 "closeLeaverSession 在 broadcastMemberLeft 之前调用 → ListSessionsByRoomID 自然不含 leaver"。这是另一条"sync ordering 隐含约束"。即使现在 ordering 仍然正确，本身就是脆弱依赖 —— 应当用 BroadcastToRoomExcept 把它变成 belt-and-suspenders 双重保护

## Lesson 2: 异步化 lifecycle 操作必须切分"瞬时同步段"和"慢异步段"

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/service/room_service.go:776-778`

### 症状（Symptom）

`LeaveRoom` 的 r2 实装把整个 `closeLeaverSession`（含 `ListSessionsByRoomID + Unregister + CloseWithCode`）放进 post-commit goroutine。HTTP 200 返回时 leaver Session 仍在 `SessionManager` 索引中（`Unregister` 还没执行）。期间任何 broadcast（如另一 user 在 leave 完成瞬间 join）仍 fanout 给 stale leaver session → leaver 收到房间事件，违反 V1 §10.5 步骤 7 "HTTP leave immediately detaches WS"。

### 根因（Root cause）

`closeLeaverSession` 包含两类性质截然不同的操作：

1. **`SessionManager.Unregister`**：纯 map 操作 O(1)，**瞬时完成不阻塞**，但**有外部可观察副作用**（让后续 `ListSessionsByRoomID` 立即不包含 leaver）
2. **`Session.CloseWithCode`**：drain write loop 最坏 ~5s（writeTimeout + buffer），**慢路径阻塞**，但**外部可观察副作用是次要的**（写 close frame 给 client；client 已经能从 HTTP leave 200 推断"已离开"）

r2 把两者一起异步化是因为只看了"caller 不阻塞"诉求 → 把整段移到 goroutine。但**只有第二类操作需要异步**（避免 ~5s 阻塞 HTTP），第一类应该保留在同步段（瞬时 + 立即满足 invariant）。

### 修复（Fix）

把 `closeLeaverSession` 切分成 hybrid：

```go
// 同步段（room_service.go 内 LeaveRoom 调用）：
func (s *roomServiceImpl) unregisterLeaverSessionSync(ctx, roomID, leaverUserID) (target *ws.Session, found bool) {
    if s.sessionMgr == nil { return nil, false }
    sessions := s.sessionMgr.ListSessionsByRoomID(ctx, roomID)
    for _, sess := range sessions {
        if sess.UserID() == leaverUserID { target = sess; break }
    }
    if target == nil { return nil, false }
    s.sessionMgr.Unregister(ctx, target.SessionID()) // 同步！
    return target, true
}

// 异步段（在 runPostCommitAsync goroutine 内）：
func (s *roomServiceImpl) closeLeaverSessionAsync(ctx, ..., target *ws.Session) {
    if target == nil { return }
    target.CloseWithCode(4007, "left room via HTTP") // 慢，~5s drain
}

// LeaveRoom 调用：
target, _ := s.unregisterLeaverSessionSync(ctx, in.RoomID, in.UserID)  // sync
s.runPostCommitAsync(ctx, func(detachedCtx context.Context) {
    s.closeLeaverSessionAsync(detachedCtx, in.RoomID, in.UserID, target)  // async
    s.broadcastMemberLeft(detachedCtx, in.RoomID, in.UserID)
})
```

HTTP 200 返回前 leaver 已从 SessionManager 索引消失 → 后续任何 broadcast 不会再 fanout 给 leaver。CloseWithCode 慢路径仍走异步不阻塞 HTTP。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**异步化复合 lifecycle 操作**（如 close+unregister / commit+notify / write+sync 等多步操作）时，**必须**把每一步按 "(a) 同步耗时 vs 慢异步耗时" + "(b) 是否有外部可观察副作用 invariant" 分类，**只把 (a) 慢 + (b) 弱 invariant** 的步骤异步化；**(a) 瞬时 + (b) 强 invariant** 的步骤**必须**保留同步路径。
>
> **展开**：
> - 复合 lifecycle 操作内的子步骤通常是异质的（map 操作 + IO drain，原子提交 + 网络通知，等）。"整段异步化"是把异质子步骤强行同质化的反模式，常见于"为简化代码把 fn 整段挪到 goroutine"
> - 评估分类：每个子步骤问两个问题：
>   1. 它阻塞 caller 多久？（µs / ms / s）—— 决定是否需要异步
>   2. 它的副作用对外部观察方意味着什么？特别是有没有"协议立即生效" / "状态立即清除"之类的 invariant？—— 决定是否能异步
> - **反例**（本案例）：closeLeaverSession 内 Unregister（µs + 强 invariant "立即从索引消失"）+ CloseWithCode（s + 弱 invariant "client 收到 close frame，但 client 也能从 HTTP 200 推断已离开"）—— r2 整段异步化把强 invariant 步骤一起放后台
> - **反例**（一般化）：commit + invalidate-cache + notify-others 这种链路；invalidate-cache 通常是强 invariant（caller 后续读必须看到新值），notify-others 通常是弱 invariant（best-effort）。常见错误是"全异步化简化代码" → invalidate 在 caller return 之后才执行 → caller 自己后续读到 stale cache
> - **正例**：本案例 r3 hybrid 切分；标准 DB 事务的 "commit (sync) + post-commit hook (async)" 模式；HTTP gateway 的 "auth check (sync) + audit log (async)" 模式

---

## Meta: 本次 review 的宏观教训

r2 → r3 的两条 finding 都是**同源**思维漏洞：r2 fix（codex review r2 [P1]/[P2]）正确识别了"post-commit hook 不能阻塞 HTTP 响应 / 不能被 request ctx cancel 误中断"两个性能/可靠性问题，并采用了"整段放进 detached goroutine"的修复。这个修复**只看了一个维度**（caller 阻塞 / ctx cancel），没看另两个维度（外部可观察协议 invariant / 子步骤异质性）。

**根本预防规则**：每次"把 sync 改 async"的 fix 都必须做两轮 review：

- 第一轮（r2 已做）：异步化是否解决了驱动力问题？（不阻塞 / ctx 不传播）
- 第二轮（r2 漏掉，r3 补上）：异步化是否破坏了**任何**外部可观察的 invariant？

第二轮的 checklist：

1. 该路径所有 spec / docs 里有没有"立即生效 / 不应观察到 / 顺序保证 / authoritative signal"之类的钦定语义？逐条评估
2. 该路径异步化后，caller return 时 server 的可观察状态是否与 sync 实装相同？（如本案例：sync 路径 return 时 leaver 已从 SessionManager 消失；async 路径 return 时还在）
3. 该路径异步化后，该路径**之后**触发的其他路径（同 store / 同资源）是否仍能看到正确的 server 状态？（如本案例：另一 user 的 join broadcast 是否仍能"自然不 fanout 给已离开的 leaver"）

任何一条 fail 都说明需要 hybrid 切分（保留同步段）或显式实装（如 BroadcastToRoomExcept），而不是接受异步化的副作用。
