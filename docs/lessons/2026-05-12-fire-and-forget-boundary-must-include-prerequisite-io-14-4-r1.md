---
date: 2026-05-12
source_review: file:/tmp/epic-loop-review-14-4-r1-retry.md (codex P1)
story: 14-4-pet-state-changed-ws-广播
commit: 0f1bf12
lesson_count: 1
---

# Review Lessons — 2026-05-12 — fire-and-forget 边界必须包住"决定是否 broadcast 的前置 IO"（14-4 r1）

## 背景

Story 14.4 给 `POST /pets/current/state-sync` 接 pet.state.changed WS 广播。dev-story 把
最终 fanout（`broadcastFn`）放进 `context.WithoutCancel(ctx) + WithTimeout(10s) +
go func() {...}` 的 detached goroutine，但**保留** `userRepo.FindByID(ctx, ...)` 在请求
ctx 上同步调用 —— 因为"高频路径不浪费 goroutine 启动开销 + FindByID <10ms 可忽略"。
codex review 指出这破坏 fire-and-forget 契约：FindByID 决定**是否**广播，本身是
broadcast-side IO，必须一起搬进 detached path。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | fire-and-forget 边界必须包住决定是否 broadcast 的前置 IO | high | architecture | fix | `server/internal/service/pet_service.go:230-244` |

## Lesson 1: fire-and-forget 边界必须包住"决定是否 broadcast 的前置 IO"

- **Severity**: high
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/service/pet_service.go:203-249`

### 症状（Symptom）

dev-story 提交的 SyncCurrentState 实装在请求 ctx 上同步调
`userRepo.FindByID(ctx, in.UserID)`，再根据 `user.CurrentRoomID` 决定是否启动
detached goroutine 调 `broadcastFn`。Detached ctx + 10s timeout 只保护最终 fanout，
不保护"是否 fanout"的 user 查询。两个 contract 破坏：

1. **client 断开后事件丢失**：UPDATE 已 commit 但 client 在 FindByID 返回前断开 →
   请求 ctx cancel，FindByID 返 `ctx.Err()` → 走 `log warn + skip broadcast` 路径 →
   DB pets.current_state 已落地但其他房间成员**永远收不到** pet.state.changed 事件
2. **HTTP 响应被 broadcast 侧 IO 阻塞**：FindByID 慢（DB 卡 / 网络抖 / 连接池满）
   → SyncCurrentState 主线程被卡住 → HTTP 200 ack 延迟。违反"broadcast 是 detached
   async，不阻塞 HTTP 响应"契约

### 根因（Root cause）

写 fire-and-forget broadcast 路径时只把"最贵 / 最易失败"的 IO（`broadcastFn` 本身
带 Session fanout + WS write）搬进 detached goroutine，**漏掉**了"决定是否进入广播
路径的前置查询"也是 broadcast-side IO。

错误的心智模型：「FindByID 是 DB 普通查询、<10ms、属于主路径」。
正确的心智模型：「**任何**为了 broadcast 而做的查询都是 broadcast-side IO，必须和
fanout 同生死 —— 同 detached ctx、同 goroutine、同 timeout 边界」。

类比反例：如果 `broadcastFn` 失败要"不影响 HTTP 200"，那么"为了能调 broadcastFn
而做的 FindByID"失败也必须不影响 HTTP 200；反过来若 FindByID 在主路径上失败影响
HTTP 响应（虽然现实装是 log warn 不返 error），那它就**没有**真的实现 detached。
判定标准：把 broadcast 路径里所有 IO 全部杀掉，HTTP 响应路径必须毫秒级返回 200。

### 修复（Fix）

`server/internal/service/pet_service.go`：把 user 查询整体下沉进 detached goroutine。

Before：

```go
user, err := s.userRepo.FindByID(ctx, in.UserID)  // ← 请求 ctx
if err != nil {
    slog.WarnContext(ctx, ...)
} else if user.CurrentRoomID != nil {
    roomID := *user.CurrentRoomID
    detached := context.WithoutCancel(ctx)
    timedCtx, cancel := context.WithTimeout(detached, petBroadcastTimeout)
    go func() {
        defer cancel()
        s.broadcastPetStateChanged(timedCtx, roomID, in.UserID, pet.ID, in.State)
    }()
}
```

After：

```go
detached := context.WithoutCancel(ctx)
timedCtx, cancel := context.WithTimeout(detached, petBroadcastTimeout)
go func() {
    defer cancel()
    // 在 detached ctx 内查 user.CurrentRoomID
    user, err := s.userRepo.FindByID(timedCtx, in.UserID)
    if err != nil {
        slog.WarnContext(timedCtx, "pet state-sync: load user for broadcast failed; skip broadcast", ...)
        return
    }
    if user.CurrentRoomID == nil {
        return // 合法路径，不广播也不 log warn
    }
    s.broadcastPetStateChanged(timedCtx, *user.CurrentRoomID, in.UserID, pet.ID, in.State)
}()
```

同时改写 doc-comment（line 203-249）：
- 删除"先查 user 而后启 goroutine 的考虑：高频路径不浪费 goroutine 启动开销"段落
  —— 这条优化已经放弃，fire-and-forget 边界优先级更高
- 新增"完全 detached（14-4 r1 lesson 钦定）"段落，明示所有 broadcast 相关 IO（含
  user 查询）都在 detached path 内
- 写入两个反例（client 断开后事件丢失 / FindByID 卡死阻塞 HTTP）作 few-shot 负例

**顺带改动**：`pet_service_test.go` 给 `stubUserRepoForPetService` 加 optional
`findByIDDone *sync.WaitGroup` 钩子。FindByID 调用点切换到 detached goroutine 后，
case 8（CurrentRoomID nil，broadcast 0 次） / case 11（FindByID err，broadcast 0
次）无法靠 broadcastRecorder.wg 间接同步 —— broadcastFn 不会被调，wg.Done 永不
触发。findByIDDone 提供"FindByID 返回时同步信号"，让单测能稳定断言
`findByIDCalls == 1`。case 7 / 10 / 12 不需要 findByIDDone（broadcastFn 被调，wg
间接同步已够）。

### 预防规则（Rule for future Claude）

> **一句话**：未来 Claude 实装 **fire-and-forget broadcast / event 路径** 时，**必须**
> 把**所有**为决定"是否 / 怎么 broadcast"而做的 IO（含 user / room / member 查询、
> permission check、payload precondition）**全部**放进同一个 detached goroutine 内，
> 用 detached ctx 调用；**禁止**在请求 ctx 上同步调 prerequisite 查询然后才启
> goroutine 调最终 fanout。
>
> **展开**：
> - **判定标准**：把 broadcast 路径里所有 IO 全部假设为"无限慢 / 必失败"，HTTP
>   响应路径必须仍能毫秒级 return 200。任何因 broadcast-side IO 失败而被跳过的
>   分支同样**不**影响 HTTP 200
> - **结构模板**（11.8 / 14.4 同模式）：
>   ```go
>   detached := context.WithoutCancel(ctx)
>   timedCtx, cancel := context.WithTimeout(detached, broadcastTimeout)
>   go func() {
>       defer cancel()
>       // 所有 broadcast prerequisite IO 在这里调，全用 timedCtx
>       prereq, err := repo.FindXxx(timedCtx, ...)
>       if err != nil { log warn + return }
>       if !shouldBroadcast(prereq) { return }
>       broadcastFn(timedCtx, ...) // 或 wrapper
>   }()
>   ```
> - **doc-comment 必须明示**：fire-and-forget broadcast 路径的 doc 必须列出"所有
>   broadcast-side IO 都在 detached path 内"的契约，杜绝下一位实装者再做"高频
>   路径同步优化"
> - **反例 1**：在请求 ctx 上 `repo.FindByID(ctx, ...)` 查权限/成员关系，然后才
>   `go func() { broadcastFn(detached, ...) }()`。client 断 → 请求 ctx cancel →
>   prerequisite 查询返 ctx.Err → broadcast 被跳过 → 事件丢失但 DB 已 commit
> - **反例 2**：用"<10ms 可忽略"或"高频路径优化"作借口把 prerequisite 留在主
>   路径。可观测性 / 极端情况下（DB 卡 / 连接池满）依然会阻塞 HTTP 响应；性能
>   理由**永远不**能凌驾于 fire-and-forget 契约
> - **单测同步影响**：当 prerequisite IO 从主路径下沉到 goroutine 内，原本"主
>   路径同步执行 + 主线程直接断言 calls 计数"的 case 必须改用 wg / channel /
>   atomic 等同步机制；不能依赖 `time.Sleep` 兜底（CI flake 源）。给 stub repo
>   加 optional wg hook 是稳定方案（参见 stubUserRepoForPetService.findByIDDone）

## Meta: 本次 review 的宏观教训

fire-and-forget 边界是一条"全有或全无"的契约线，**不**是"主要 IO detached + 边角
IO 主路径"的混合体。判断哪些 IO 属于 broadcast-side 不靠"贵不贵"或"频率高低"，
而靠**用途**：只要 IO 的存在意义是"为 broadcast 服务"（含决定是否广播、构造
payload、解析路由目标），就属于 broadcast-side，必须一起 detached。这条原则对
未来 Epic 14 / 11.x / 26 / 29 等所有 WS 广播 story 通用。
