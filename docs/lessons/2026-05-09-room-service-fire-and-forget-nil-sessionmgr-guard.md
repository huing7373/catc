---
date: 2026-05-09
source_review: codex review r1 — /tmp/epic-loop-review-11-8-r1.md
story: 11-8-成员加入-离开-ws-广播
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-09 — RoomService fire-and-forget 路径 nil sessionMgr guard 必须与 broadcastFn closure 同模式

## 背景

Story 11.8（房间成员加入/离开 WS 广播）codex review r1 唯一 P1 finding：`closeLeaverSession` 在 LeaveRoom 事务 commit 后被 fire-and-forget 调用，方法体首行直接访问 `s.sessionMgr.ListSessionsByRoomID(...)`，**未做 nil guard**。RoomService 的 sessionMgr 字段是可选注入（HTTP-only / 集成测试 wiring 场景下不传），与之配套的 `broadcastFn` closure（router.go:387-391）已有 `if deps.SessionMgr == nil { return 0, nil }` 兜底，但 `closeLeaverSession` 这条平行路径漏了同模式 guard —— 当 sessionMgr 为 nil 时 LeaveRoom 事务 commit 后会 panic 在事务外的 fire-and-forget 调用点。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `closeLeaverSession` 缺 nil sessionMgr guard，破坏 fire-and-forget 路径 nil-tolerance 一致性 | high (P1) | error-handling | fix | `server/internal/service/room_service.go:923` |

## Lesson 1: fire-and-forget 路径上每个直接访问可选依赖的方法都必须有 nil guard，且必须与 closure 兜底**同模式**

- **Severity**: high (P1)
- **Category**: error-handling
- **分诊**: fix
- **位置**: `server/internal/service/room_service.go:923` (`closeLeaverSession` 入口)

### 症状（Symptom）

LeaveRoom HTTP 请求在事务 commit 成功后调用两个 fire-and-forget hook：
1. `s.closeLeaverSession(ctx, roomID, userID)` — 直接访问 `s.sessionMgr.ListSessionsByRoomID(...)`
2. `s.broadcastMemberLeft(ctx, roomID, userID)` — 走 `s.broadcastFn` closure（closure 内部已 guard `deps.SessionMgr == nil`）

当 RoomService 在 HTTP-only / 集成测试 wiring 中没有注入 sessionMgr（`s.sessionMgr == nil`），路径 (2) 安全 no-op，但路径 (1) 第一行就解引用 nil interface → panic。Panic 发生在 LeaveRoom 事务 commit 之**后**，HTTP handler 已准备返 200，但 panic 会沿调用栈向上传播，造成 500 / goroutine crash。

### 根因（Root cause）

写 `closeLeaverSession` 时把它当成"和 broadcastMemberLeft 平级的 fire-and-forget hook"，但**遗漏了平级语义里更深一层的等价性**：broadcastMemberLeft 走 `s.broadcastFn` closure，closure **本身**就是 router 注入时构造的 nil-tolerant 包装；closeLeaverSession 没有对应的 closure 抽象，直接持 `s.sessionMgr` 引用 + 直接调方法 → guard 责任落在 closeLeaverSession 自身。

更抽象的失误：把"broadcastFn 已经 nil-safe"的兜底位置（在 closure 内部）当成了"sessionMgr 这条依赖整体已经 nil-safe"。**closure 包装的 nil guard 只保护通过该 closure 走的调用，不保护任何其他直接持有底层依赖引用的代码路径**。

### 修复（Fix）

在 `closeLeaverSession` 入口、logger 初始化前加同模式 guard：

```go
func (s *roomServiceImpl) closeLeaverSession(ctx context.Context, roomID, leaverUserID uint64) {
    // **nil sessionMgr guard**：HTTP-only / test wiring 场景下 RoomService 可能不
    // 注入 sessionMgr（与 router.go broadcastFn closure `if deps.SessionMgr == nil`
    // 同模式）—— 此时 LeaveRoom 事务 commit 后 fire-and-forget 调用本方法不应 panic，
    // 直接 no-op 返回（fire-and-forget 严格语义：永远不返 error，no-op 不影响 HTTP 200）。
    if s.sessionMgr == nil {
        return
    }
    // ... 原 logger / ListSessionsByRoomID 逻辑
}
```

注释里**显式 cross-reference** router.go broadcastFn closure 的 nil guard，让两处 nil-tolerance 强制视觉对齐。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 service 层添加任何 fire-and-forget 路径的方法时，**只要该方法直接访问可选注入字段（如 sessionMgr / metricsClient / broadcaster）**，**必须**在方法入口加 `if s.<field> == nil { return }` guard，**并且**该 guard 必须与同链路其他 fire-and-forget 钩子（closure / wrapper）的 nil-tolerance 模式**逐字对齐**。
>
> **展开**：
> - fire-and-forget 路径的强契约是"永远不返 error 不 panic"——nil guard 是该契约的实现细节，不是可选优化
> - 当一条 service 方法和一条 closure-wrapped 方法**平级**调用（比如 LeaveRoom commit 后串行 hook 1 + hook 2），二者的 nil-tolerance 必须对齐：要么都 guard 要么都不 guard。closure 的 nil guard 只保护 closure 自己内部
> - 加 guard 时注释**必须**显式 cross-reference 到对侧的 nil guard 位置（如 `router.go broadcastFn closure` / `xxx wrapper closure`），让两处强制视觉对齐 —— 未来重构时 reviewer 能一眼对账两边一致
> - **反例 1**：`func (s *svc) doFireAndForget() { logger := slog.Default()...; s.optionalDep.SomeMethod() }` —— 直接调方法，sessionMgr nil 时 panic
> - **反例 2**：`func (s *svc) doFireAndForget() { if s.optionalDep != nil { logger := ...; s.optionalDep.SomeMethod() } }` —— guard 位置 OK 但写法和 closure 不对齐（closure 是 early return，service 方法用 if 包裹），未来重构容易漏改
> - **反例 3**：只在 closure 路径加 guard 不在直接路径加 —— 测试覆盖 closure 走过去看着 OK，但 production wiring 一变（移除 sessionMgr 注入或加了新的可选 dep），panic 立即暴露
> - **正例**：service 方法入口 early return guard + 注释 cross-ref + 与 closure 同模式（`if x == nil { return }`，**不**是 `if x != nil { ... }` 包大块）—— 与本仓 router.go broadcastFn closure 一致

## Meta: 本次 review 的宏观教训

可选依赖（pointer / interface）的 nil guard **不是单点责任** ——它是路径责任。每条**最终会解引用该字段的调用路径**都必须有 guard，无论是直接调用还是经过 closure / wrapper / DI 容器。Code review 阶段对照"同链路其他路径如何处理 nil"是发现这类缺口最快的启发式（codex review r1 正是用这个启发式抓到 P1）。
