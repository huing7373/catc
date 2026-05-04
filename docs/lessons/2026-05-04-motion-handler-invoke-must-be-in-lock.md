---
date: 2026-05-04
source_review: codex review round 3 — /tmp/epic-loop-review-8-2-r3.md
story: 8-2-coremotion-接入
commit: d875b7d
lesson_count: 1
---

# Review Lessons — 2026-05-04 — CoreMotion subscribe/stop 契约：handler invoke 必须与 generation check 共享同一锁段

## 背景

Story 8.2 round 2 修复后，`MotionProviderImpl` 已经把 generation token + lock 内 check 写好；但 round 3 codex 指出仍有 race：generation check 在 lock 内做、handler invoke 在 unlock 之后做——`stopUpdates()` 在 unlock 与 invoke 的间隙里跑完，"stopUpdates 返回后保证不再 fire" 的契约依然破裂。本 lesson 记录从"check & invoke 分段"到"check & invoke 同段"的最后一步，同时把 handler 的轻量同步约束写进 source comment。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | callback invoke 应在 lock 持有期间同步进行 | P2 (medium) | architecture | fix | `iphone/PetApp/Core/Motion/MotionProviderImpl.swift:88-94` |

## Lesson 1: 订阅取消语义要求 generation check 与 user handler invoke 共享同一锁段

- **Severity**: medium (P2)
- **Category**: architecture / concurrency
- **分诊**: fix
- **位置**: `iphone/PetApp/Core/Motion/MotionProviderImpl.swift:83-95`

### 症状（Symptom）

CoreMotion callback 在 OperationQueue.main 上抵达后：
1. `lock.lock()`
2. 检查 `generation == myGen` 且 `currentHandler` 非 nil → 拷贝 handler 到 local var
3. `lock.unlock()`
4. 调用 local handler（已不在 lock 内）

如果第 3 步与第 4 步之间另一个线程跑了 `stopUpdates()`：清掉 `currentHandler`、自增 `generation`、调 `manager.stopActivityUpdates()`——返回后调用方按契约相信"不会再 fire"，但当前线程已经持有 local handler 引用，仍会 invoke 一次。订阅取消语义破裂。

### 根因（Root cause）

之所以最初把 invoke 写在 lock 外，是出于"避免 user handler 阻塞 lock"的本能直觉。但这条直觉在以下两个前提下都站不住：

1. **CoreMotion handler 是同步 closure**——不是 async 操作；不会 await 也不会回调系统（不像写文件 / 网络请求）。
2. **本类的 user handler 文档约束已经是"轻量同步操作"**（写 @Published / @State / array.append），用户也按这个约束实现 HomeViewModel 等调用点。

这两个前提下，"lock 内同步 invoke" 的代价是 sub-microsecond 级别的 CPU 时间（user closure 本身的 latency），但收益是把"check 通过 → invoke 完成"打成原子段，订阅取消语义自然成立。最初的拆段直觉错误地把"通用并发 lock 最佳实践（不在 lock 内做 user code）"套用到这个特化场景。

更深层的根因：**异步订阅类（subscribe + cancel）的契约本质上要求"check & invoke 原子"**——cancel 后能否再 fire，决定于 cancel 路径与 fire 路径之间是否存在"check 已过但 invoke 未发"的窗口；只要存在这个窗口，无论用什么 token（generation / sequence number / nonce）都救不回来，因为 token 本身的 read 与"实际 invoke"必须在同一段临界区内。

### 修复（Fix）

`MotionProviderImpl.startUpdates` 中 callback 闭包改写：

before（round 2）：
```swift
self.lock.lock()
guard self.generation == myGen, let captured = self.currentHandler else {
    self.lock.unlock()
    return
}
self.lock.unlock()
captured(activity)   // ← 在 lock 外 invoke
```

after（round 3）：
```swift
self.lock.lock()
defer { self.lock.unlock() }
guard self.generation == myGen, let captured = self.currentHandler else {
    return
}
captured(activity)   // ← 在 lock 内同步 invoke
```

同时在 source comment 显式声明：
- "user handler 必须是轻量同步操作（写 @Published / @State 等）"
- "user handler 禁止在闭包内调 startUpdates / stopUpdates（NSLock 不可重入，会死锁）"

mock（`MotionProviderMock`）不需要改动——mock 是同步 in-process 实现，injectActivity 本来就在调用线程同步 invoke，不存在 race。round 1 添加的 stop/restart race 单测继续覆盖 mock 行为不变。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在实现"subscribe + cancel"语义类（包含 KVO / Combine wrapper / Notification observer / CoreMotion callback / CoreLocation delegate / WebSocket onMessage…）的取消分支时，**必须**让"判断订阅是否仍有效"与"调用 user callback"处于同一临界区内（lock / actor 隔离 / serial queue 同段），否则 cancel 返回后仍可能多发一次事件。
>
> **展开**：
> - 任何"读 token / handler ref → 释放锁 → invoke handler"的两段写法都是 race，无论 token 设计多精巧；token 只能保证"丢弃 stale 事件"，但保证不了"cancel 之后绝对不再 fire"——后者要求 invoke 也在锁内。
> - 如果担心 user handler 阻塞 lock：先**显式约定** user handler 的执行预算（"轻量同步"/"≤1ms"/"仅写 @Published"），写进 source comment + 文档；然后接受"lock 内 invoke"的代价。这是**契约 trade-off**，不是性能 bug。
> - 如果 user handler 真的可能慢（例如 user 想在 callback 里发网络请求），不要走"lock 外 invoke"的路——改成 user handler 自己 hop 到自己的 queue 异步处理，库内只保证"派单瞬间 cancel 已生效"。
> - NSLock 不可重入：lock 内 invoke user closure 时必须**禁止** user 在 closure 里再调本类的 lock-acquiring 公开方法（startUpdates / stopUpdates），并把这条约束写进 doc comment。
> - **反例**：写 `lock(); read token & handler; unlock(); handler()` 然后认为 "generation token 解决了 race"——这是 round 2 的错误，generation 只解决了 stale dispatch（已 enqueue 的事件丢弃），没解决 cancel 时序契约（unlock 与 invoke 之间的 cancel 窗口）。
> - **反例 2**：用 OperationQueue.addOperationAndWait / DispatchQueue.sync 作为"等待 in-flight callback 完成"的同步点——如果 caller 自己在同一个 queue 上跑就死锁（main thread 调 stopUpdates，callback 也在 main queue → 经典死锁）。"lock 内 invoke" 比这个简单得多。

## Meta: round 1 → round 2 → round 3 的递进教训

本 story 的并发治理走过三轮：
- **round 1**：完全没考虑 stop 后 stale callback 的 generation 问题——典型新手并发疏漏。
- **round 2**：加了 generation token，但 token check 在 lock 内、invoke 在 lock 外——典型"半完成的并发修复"。
- **round 3**：把 invoke 也搬进 lock 内——并发契约才完整。

宏观规律：**并发 bug 经常是"修了一半就以为修完了"**。每轮修复后，必须重新问一次："cancel 返回后，能否在任何线程调度下再观察到一次 fire？" 如果答案不是绝对的"不能"，bug 还在。
