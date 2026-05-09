---
date: 2026-05-09
source_review: codex review r8 输出（/tmp/epic-loop-review-11-8-r8.md）
story: 11-8-成员加入-离开-ws-广播
commit: a23eae5
lesson_count: 1
defer_tech_debt: true
---

# Review Lessons — 2026-05-09 — per-room worker lifecycle 不在 11-8 修：MVP 节点 4 demo 阶段量化上界可控，留作 future epic 单独 story（11-8 r8）

## 背景

Story 11.8 在 r5 引入 "per-roomID worker queue + commit-time lock" 模式后（lessons r4-r7 详述），存在一个长期运行 prod 部署下的内存/goroutine 泄漏：

```go
// room_service.go:541-545
func (s *roomServiceImpl) enqueueRoomEvent(ctx context.Context, roomID uint64, fn func(detachedCtx context.Context)) {
    qIface, _ := s.roomQueues.LoadOrStore(roomID, &roomQueue{ch: make(chan func(), 256)})
    q := qIface.(*roomQueue)
    q.once.Do(func() {
        go s.runRoomQueueWorker(q)  // ← 永不退出（worker for-range 在 ch 上 block 等下个事件）
    })
    ...
}
```

每个曾经触发过 join/leave 的 roomID 都在 `s.roomQueues` / `s.roomCommitLocks` 留下永久 entry + 永久阻塞的 worker goroutine。短生命房间多的长期运行场景下，goroutine count 与 memory 单调增长。

codex r8 [P2] 准确指出此问题。本次 review 决定 **defer + tech-debt + 量化上界**，不在 11-8 内修。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | per-room queue worker + roomQueues / roomCommitLocks map 永不 reclaim | P2 (medium) | perf / architecture | **defer (tech-debt)** | `server/internal/service/room_service.go:541-545` |

## Lesson 1: per-room worker lifecycle 优化是独立架构议题，不与 join/leave 广播功能正确性混修

- **Severity**: P2 (medium)
- **Category**: perf / architecture
- **分诊**: **defer** —— 本 story 不修，记入 tech-debt，留作 future epic 单独 story 处理
- **位置**: `server/internal/service/room_service.go:541-545` (enqueueRoomEvent / runRoomQueueWorker)

### 症状（Symptom）

`s.roomQueues` 是 `sync.Map[uint64]*roomQueue`，每个 distinct roomID 第一次 enqueue 时通过 `LoadOrStore` 创建并由 `q.once.Do` 启动 worker goroutine。worker 是 `for fn := range q.ch` 的无限 loop —— channel 不会被关闭，所以 worker 永远阻塞在 `<-q.ch`。`roomCommitLocks` 同理。

长期运行 server + 大量短生命房间场景下：
- 每个 distinct roomID 残留 1 个 worker goroutine（栈 ~2KB）+ 1 个 `*roomQueue`（结构 + 256 容量 channel buffer）+ 1 个 `*sync.Mutex`
- 数量随 "曾出现过的 roomID 数" 单调增长，进程不重启不释放

### 根因（Root cause）

r5 引入 "per-roomID lock + queue" 是为了解决 **causal ordering**（lessons r4 / r5 / r6 / r7）—— 关注点是 "同一 roomID 的 commit-order 必须 = client 接收 order"。**worker lifecycle 不在 r5 引入时的设计目标里**。

r5-r7 五轮 review 把 "lock 段 / enqueue 时机 / unregister 位置 / 三象限分类" 全部敲定，但都假设 worker 永远活着。这种 "先把 ordering invariants 锁死，后续再考虑 lifecycle 优化" 是合理的渐进式设计 —— 但确实留下了 r8 codex 抓到的 tech-debt。

### 修复（Fix）

**不修。defer 至 future epic / future story。**

#### 为什么 defer 而不在 11-8 修

1. **scope 不匹配**：本 story AC 是 "成员加入 / 离开广播功能正确"，不是 "worker 生命周期管理"。worker reclaim 是独立的架构议题（可能涉及全局清理协议 / TTL / 引用计数），应当作为独立 story 设计而非夹塞进 11-8 第 8 轮。
2. **路径 A（idle timeout）race-prone**：实装路径需要在 worker idle 退出 vs concurrent caller enqueue 之间做精细 atomic CAS 协调（参见 review 文末 "race 注意" 段）。一旦 cleanup 不 atomic，会破坏 r4-r7 五轮敲定的 commit-order = enqueue-order = causal-order 不变量 —— 等于砸了 5 轮 lessons 的果实。
3. **路径 B（LeaveRoom-triggered cleanup）只是局部解**：仅清理 closed room，对长寿命 active room 无效。
4. **量化上界可控（节点 4 demo 阶段）**：
   - **房间数 ≈ 用户数**（每用户最多 1 房间，主动 join/leave 后 room 关闭但 entry 留下）
   - 假设：30 天 prod 持续运行，每天 100 个新房间 → 累计 3000 个 zombie worker
   - **goroutine 数**：3000 远低于 Go scheduler 实际承受上限（10K+ 通常无问题）
   - **内存**：每 worker ~2KB stack + roomQueue struct（~50B + 256-cap channel buffer 约 2KB）+ commitLock（24B）≈ 4KB，3000 个 ≈ 12MB —— 可接受
   - **结论**：MVP 阶段不构成实际故障风险
5. **review 第 8 轮**：r2-r7 已经修了 7 个真问题；r8 只剩这 1 个 P2 且属于优化议题。继续在 11-8 内死磕的边际收益低于打开新 story 单独设计的收益。

#### Tech-debt 登记（traceability）

- **issue title**: "per-room worker / map entry lifecycle 回收：idle timeout + race-safe cleanup"
- **owner story（建议）**: 节点 4 完成后 / 节点 5+ epic 内单独立 story（如 "11-X 房间事件 worker pool 优化" 或归到 "性能 / 长期运行可靠性" 主题 epic）
- **触发条件**: 当 metrics 显示 zombie worker 数量超过 5K 或 memory 单调增长且经验证关联到 room_service 时启动
- **设计建议**: 优先实装 review 段建议的 "idle timeout（30s）+ atomic CAS active 标志 + caller 端检测 dead worker 后通过 LoadOrStore 重建" 模式（路径 A）；与新 story 同时蒸馏 lifecycle 测试矩阵（active room 不被 cleanup / inactive room 30s 后 reclaim / cleanup-and-rejoin race / cleanup-and-enqueue race）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **N 轮 review fix 中遇到非 P0/P1 优化议题且本 story 已敲定 N-1 个 invariants** 时，**优先 defer + 量化 tech-debt + 归档 lesson**，禁止在同一 story 内再加复杂 lifecycle 改动。
>
> **展开**：
> - **scope 检查先行**：每条 finding 进 fix 队列前先问 "这是 story AC 范围内的 correctness bug，还是独立的优化议题？"。lifecycle / reclaim / pool / TTL / idle-timeout 这类关键词 = 独立优化议题信号。
> - **量化优先于直觉**：defer 决策必须给 "上界 = 数量 × 单位资源 × 时间窗口" 三段式量化，而不是 "我觉得没事"。本例：3000 zombie × 4KB ≈ 12MB / 30 天 → 可接受。
> - **review 轮次越后，fix 风险越高**：r5+ 已经把多条 invariants 写死，新增 lifecycle 操作会 race 到这些 invariants（worker 退出 vs caller enqueue / map.Delete vs LoadOrStore / mutex 引用过期）。把这种风险写到 lesson 里 → 未来 Claude 不会被 "P2 也是 finding 也得修" 的压力推动盲目实装。
> - **tech-debt traceability 不是占位符**：必须给（a）issue title（b）owner story 建议位置（c）触发条件 metric（d）设计建议起点 + 测试矩阵 sketch，让未来打开 story 时不用从零思考。
> - **反例**：在 r5/r6/r7 这种已经把 ordering invariants 锁死的语境下，r8 不做量化、不写 tech-debt、直接往 enqueueRoomEvent 里塞 idle-timer 和 atomic CAS —— 大概率破坏 r4-r7 之一的 invariant，触发 r9 round。
> - **正例**：本 lesson —— defer + 量化（30 天 / 100 房间/天 / 12MB）+ tech-debt（owner / 触发 metric / 设计起点）+ 不动代码，工作区交回 review state 让 epic-loop 进 done 决策。

## Meta: 本次 review 的宏观教训

**review 修复不是无限游戏**：r2 → r8 七轮 fix 已经把 11-8 的 functional correctness（join broadcast / leave detach / causal ordering / commit-order / lock 段时机 / 三象限分类）全部敲定。第 8 轮的 P2 是 architectural optimization，不是 correctness bug。

继续修 vs defer 的判定准则：
- **必须修**：finding 直接违反 story AC / 客户端可观察 invariant / 数据正确性
- **可以 defer**：finding 是 long-running prod 优化、不影响 MVP demo 阶段功能、且量化上界可控

defer 不是 "懒得修"，是 **"识别此 finding 属于另一个 story 的 scope"** —— 写明 traceability 后比硬塞进 11-8 更专业。
