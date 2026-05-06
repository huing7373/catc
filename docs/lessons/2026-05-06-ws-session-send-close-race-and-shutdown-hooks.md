---
date: 2026-05-06
source_review: codex review of Story 10.3 r1（review_findings file: /tmp/epic-loop-review-10-3-r1.md，dev-story 留下的 dirty 工作区一并入 commit）
story: 10-3-ws-网关骨架
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-06 — WS Session.Send/Close 并发 panic & SessionManager 关停时 unregister hook 漏调

## 背景

Story 10.3（WS 网关骨架）r1 review。codex 标了两条问题，都是真实并发 / 生命周期 bug，会让整个进程在常见场景下崩或留 stale 外部状态。两条都在本轮 commit 修复。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `Session.Send` 与 `Close` 并发触发 send-on-closed-channel panic | high | concurrency / error-handling | fix | `server/internal/app/ws/session.go` |
| 2 | `SessionManager.Close` 先清索引再关 Session，导致每个 Session 的 onUnregister hook 全部漏调 | medium | architecture / concurrency | fix | `server/internal/app/ws/session_manager.go` |

修了 2 条 / defer 0 / wontfix 0。

## Lesson 1: `Send` 路径用 atomic flag pre-check 不能阻止 send-on-closed-channel panic

- **Severity**: high
- **Category**: concurrency / error-handling
- **分诊**: fix
- **位置**: `server/internal/app/ws/session.go:204-214`（修复前）

### 症状（Symptom）

`Session.Send` 在 `Close` 并发时可能 panic：

```
goroutine A: Send 看到 s.closed.Load()=false，进 select 准备写 sendChan
goroutine B: Close 把 s.closed=true + close(s.sendChan)
goroutine A: case s.sendChan <- msg 命中 → panic: send on closed channel
```

readLoop 错误退出 / 心跳超时 / SessionManager 关停 / 同 user 重连替换…任何 Session 关闭路径都可能与 BroadcastToRoom（10.5）/ ping-pong 回包并发触发，整个进程 crash。

### 根因（Root cause）

把 "atomic flag 预检查 + 立即 send" 当作了"原子 check-then-act"。但 `atomic.Bool.Load()` 之后到 `select { case ch <- msg }` 之间存在任意长的时间窗口，调度器可能让 Close goroutine 在这之间跑完 `close(ch)` —— 此后 send case 会命中已关 chan 触发 panic。**atomic load 只保证读那一瞬间的可见性，不保证后续 channel 操作仍合法**。

Go 语言层面：`close(ch)` 后任何 `ch <- v` 都 panic（spec 钉死，不可 recover 成本可接受）；唯一安全方式是用 mutex 让 send 与 close-channel **互斥**。

### 修复（Fix）

引入 `sendMu sync.RWMutex`：
- `Send` 拿 `RLock` → 检查 closed → select send → 释放 RLock。多 Send 可并发（RLock 可重入）。
- `Close` 拿 `Lock` → set closed=true → `close(sendChan)` → 释放 Lock。Close 与所有正在执行的 Send 互斥。

```go
// 修复后 Send（关键 5 行）
func (s *Session) Send(msg []byte) error {
    s.sendMu.RLock()
    defer s.sendMu.RUnlock()
    if s.closed.Load() {
        return ErrSessionClosed
    }
    select {
    case s.sendChan <- msg:
        return nil
    default:
        return ErrSessionSendBufferFull
    }
}

// 修复后 Close 关键段
s.closeOnce.Do(func() {
    s.sendMu.Lock()
    s.closed.Store(true)
    close(s.sendChan)
    s.sendMu.Unlock()
    // ...
})
```

加测试 `TestSession_Send_Close_Concurrent_NoPanic`：16 个 goroutine 持续 Send，主 goroutine sleep 20ms 后 Close，断言无 panic + 至少有一个 sender 看到 `ErrSessionClosed`（证明 race 窗口确实重叠到了）。**注意 race 检测器在 Windows + Go 1.25.0 toolchain 上有环境级 cgo 失败（pre-existing 问题，HEAD `d0b7d64` 干净 tree 上同样 fail）；但 send-on-closed-channel 在 Go runtime 不需要 -race 也会 panic，普通 `go test` 已经能验证修复正确性**。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写 **goroutine 间共享 channel 的 send + close 路径** 时，**禁止**用 `atomic.Bool` / pre-check 的 closed flag 当 send 安全保证；**必须**用 `sync.RWMutex`（Send=RLock，Close=Lock）让 send 与 close-channel 互斥。
>
> **展开**：
> - `atomic.Bool.Load()` 只是 happens-before 同步原语，**不是**临界区 —— 读完到下一条语句之间任何东西都可能发生
> - Go spec 钉死："close 后向 chan 发送会 panic"；唯一安全模式：让 close 与 send 互斥
> - RWMutex 模式比 sync.Once + recover() 干净，且不吞 panic（recover 会让真正的 bug 沉默）
> - select default 分支模拟 non-blocking send，需要在拿 RLock 之内做（释放 RLock 之后再 send 会绕过保护）
> - **反例 1**（修复前）：`if s.closed.Load() { return err }` 后立即 select send —— 经典 TOCTOU race
> - **反例 2**：用 `defer recover()` "兜底" panic —— 让 race 沉默，且 recover 后 sendChan 仍是关闭状态，下次 Send 又 panic
> - **反例 3**：把 close(ch) 放 atomic flag 翻转之前 —— flag 翻 true 时 ch 已关，Send 从看到 flag 到看到 close 之间还是会 panic
> - **正例**：标准库 / 知名库的"safe channel close"模式无一例外都是 mutex + close-once；任何"我用 atomic 就够了"的提议都是错的

## Lesson 2: 关停 manager 时 "先清索引再关 Session" 让所有 unregister 钩子全部漏调

- **Severity**: medium
- **Category**: architecture / concurrency
- **分诊**: fix
- **位置**: `server/internal/app/ws/session_manager.go:253-273`（修复前）

### 症状（Symptom）

`sessionManager.Close()` 修复前的顺序：
1. 锁内：`closed=true` → 拿出全部 Session 到 `all` slice → **清空** `sessionsByID / sessionsByRoom / userToSessionID`
2. 锁外：遍历 `all`，对每个 Session 调 `s.Close()`
3. `s.Close()` → `notifier.notifyClosed(sessionID)` → `Unregister(ctx, sessionID)` → 锁内 `s, ok := m.sessionsByID[sessionID]; if !ok { return nil }` → **走 no-op 路径直接返回，onUnregister 钩子不触发**

结果：每个 Session 的 onUnregister hook 在 manager.Close 路径下**全部漏调**。Story 10.6 用 `WithUnregisterHook(presenceRepo.RemoveOnline)` 时，shutdown / reload 后 Redis presence 集合里残留所有 user_id，外部状态长期 stale。

### 根因（Root cause）

把"清索引"当成了"幂等 cleanup"，没意识到清索引后 `Unregister` 的存在性检查会让 `Session.Close → notifyClosed → Unregister` 这条**默认路径**走空。架构上 manager 有两条 unregister 路径（外部主动调 `Unregister(id)` vs Session 自闭通过 `notifyClosed` 反向触发），两条共享同一个"id 不存在则 no-op"的幂等保护，但这个保护与"manager.Close 时主动清索引"的策略冲突。

更本质：`Unregister` 的"sessionID 不在索引 → no-op"语义是设计给"重复 Unregister / Session 已被替换" 这类**幂等路径**的；而 manager.Close 路径下索引清空只是 manager 自身的内部状态重置，**不**意味着 Session 已被 unregister，钩子语义还得跑。

### 修复（Fix）

调整 manager.Close 顺序：**保留索引到 Session.Close 跑完之后再清空**：

```go
func (m *sessionManager) Close() error {
    m.closeOnce.Do(func() {
        m.mu.Lock()
        m.closed = true
        all := make([]*Session, 0, len(m.sessionsByID))
        for _, s := range m.sessionsByID {
            all = append(all, s)
        }
        m.mu.Unlock()
        // 不清索引！

        // 锁外逐个 Close → notifyClosed → Unregister(id) → 索引找得到 →
        // removeFromIndicesLocked + onUnregister 钩子触发
        for _, s := range all {
            _ = s.Close()
        }

        // 防御兜底：理论上 Unregister 已清光索引；这一步是兜底
        m.mu.Lock()
        m.sessionsByID = map[string]*Session{}
        m.sessionsByRoom = map[uint64]map[string]*Session{}
        m.userToSessionID = map[uint64]string{}
        m.mu.Unlock()
    })
    return nil
}
```

加测试 `TestSessionManager_Close_TriggersUnregisterHookForAllSessions`：注册 3 个 Session（不同 user，同 room）→ 调 `mgr.Close()` → 断言 onUnregister hook 被调**正好 3 次**，且每个 sessionID 都进了 unregister set。再调一次 mgr.Close 验证 idempotent（hook 计数不再增加）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在写 **registry 的 Close / batch-cleanup 路径** 时，**禁止**先清索引再调成员的 close —— 因为成员的 close 通常会通过 hook / callback 反向走 registry 的 unregister 路径，索引为空让 unregister 走 no-op 把钩子吃掉；**必须**先让所有成员的 close 跑完触发钩子，再做索引兜底重置。
>
> **展开**：
> - "Unregister 见 id 不存在则 no-op" 是给**幂等重复调用**用的；与 "registry 主动清空" 的语义不同，但代码上常常合并在同一个 `Unregister` 里 —— 设计时必须意识到两个调用场景共享同一段代码会有冲突
> - registry.Close 的正确顺序：① 锁内拿快照 + 标 closed 拒绝新注册 → ② 锁外遍历快照，让每个成员的 close 触发自己的 unregister 路径（包括 hook）→ ③ 锁内做索引兜底重置（防御）
> - hook 的 side-effect（Redis cleanup / metrics 计数 / persistence）是观察"成员消失"事件的唯一外部窗口；漏调一次 = 外部 state 长期不一致，重启都修不回来（除非 hook 自身有 reconcile 逻辑）
> - 加测试时用 `sync.Map[sessionID]struct{}` 收集 hook 触发的 ID 集合，断言**集合相等**而非"调用次数 == N"——次数对了但 ID 错了同样是 bug
> - **反例 1**（修复前）：先清 sessionsByID = {} 再调 s.Close() —— hook 全漏
> - **反例 2**：清索引前 collect 一份 hook 列表副本，然后手动遍历调 hook + 调 s.Close —— 重复了 unregister 路径的代码，且如果 Session 是被外部主动 Unregister 走另一条路，hook 又会被调一次（双触发）
> - **反例 3**：在 hook 内部判 "manager.closed==true 跳过 Redis 写"——hook 是给 caller 看 lifecycle 事件的，不应反向感知 manager 内部状态
> - **正例**：保留索引让自然路径走完，最后做防御重置

## Meta: 本次 review 的宏观教训

两条 finding 都是**生命周期 race**：一条是 send/close 时序竞争（goroutine 调度层面），一条是 close-orchestration 顺序问题（架构层面）。共同教训：**写 "shutdown / cleanup / replace" 路径时必须画状态机，把每个外部 caller / hook / 自反路径的进入时刻枚举一遍**，不能依靠"我意识里这个分支不会被并发触发"——并发系统里所有可被并发触发的分支**都会**被并发触发，只是频率问题。

锁顺序与 hook 顺序是两个独立维度：lock acquire/release 顺序决定数据竞态，hook 触发顺序决定**外部观察者**看到的事件序列。两者必须在 close 路径上分别校核 —— lesson 1 修锁，lesson 2 修 hook 顺序，是同一类问题在两个维度的表现。

跨 epic 的连锁影响：本 review 修的 10.3 阶段 hook 默认是 no-op，所以 lesson 2 的 bug 在 10.3 单独不会被观察到；但 10.6 落地 Redis presence 后会立即暴露为线上 stale state。**节点骨架阶段写好 contract（"hook 在每个生命周期事件被精准触发一次"）+ 对应测试，胜过把 bug 留到使用方落地时才被压力测出来**。
