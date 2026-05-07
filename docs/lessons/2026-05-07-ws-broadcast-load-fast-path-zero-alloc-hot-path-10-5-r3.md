---
date: 2026-05-07
source_review: codex review output (/tmp/epic-loop-review-10-5-r3.md)
story: 10-5-broadcasttoroom-primitive
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-07 — WS BroadcastToRoom 同 room hot path 必须 Load fast path 而非每次 LoadOrStore alloc 新 mutex（10-5 r3）

## 背景

Story 10.5（BroadcastToRoom primitive）r3 codex review。r2 修复加了包级 `roomBroadcastMu sync.Map[uint64]*sync.Mutex` 串行化同 room 并发 broadcast。r3 review 指出：每次 BroadcastToRoom 的 `LoadOrStore(roomID, &sync.Mutex{})` 都会**先 alloc** 一个新 *sync.Mutex 实例，hit 时该实例立即被丢弃 GC。BroadcastToRoom 是 room 事件 hot path，active room 持续广播 = 持续 GC pressure。

P2，单条 fix。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | LoadOrStore 每次 alloc *sync.Mutex 增 GC 压力，改 Load fast path | P2 | perf | fix | `server/internal/app/ws/broadcast.go:181`<br>`server/internal/app/ws/broadcast_perf_internal_test.go`（新增 R3 test） |

## Lesson 1: sync.Map 的 LoadOrStore 第二参在 hit 时被丢弃 —— hot path 上必须先 Load 再 LoadOrStore

- **Severity**: P2
- **Category**: perf
- **分诊**: fix
- **位置**: `server/internal/app/ws/broadcast.go:181`

### 症状（Symptom）

`broadcastToRoomFanout` 每次进入：

```go
muVal, _ := roomBroadcastMu.LoadOrStore(roomID, &sync.Mutex{})
mu := muVal.(*sync.Mutex)
mu.Lock()
defer mu.Unlock()
```

无论 roomID 是否已经在 sync.Map 中，**每次调用**都会先 alloc 一个 `&sync.Mutex{}` 字面量（新 heap 实例）传给 LoadOrStore。Map hit 时该新 alloc 的 mutex 立即被丢弃 GC。Active room 每次 broadcast 都额外多 1 个 mutex alloc + 1 个 interface{} 装箱 alloc，稳态 GC pressure。

testing.AllocsPerRun 实测：修复前 hot path ≈ 15 allocs/op，修复后 ≈ 13 allocs/op（少 2 个）。

### 根因（Root cause）

误解 sync.Map.LoadOrStore 语义。LoadOrStore 是 **"如果不存在则存"**，第二参数**总要在调用前求值**（Go 调用约定：参数在 callee 进入前完成 evaluation），不管最终 store 还是 discard。所以 `LoadOrStore(roomID, &sync.Mutex{})` 即使 hit，第二参 `&sync.Mutex{}` **已经 alloc**，只是 LoadOrStore 内部检测到 key 已存在就丢弃返回值不持有它。

要真正"按需 alloc"必须显式两段式：先 Load，miss 才 LoadOrStore。

### 修复（Fix）

`broadcastToRoomFanout` 改成 Load fast path：

```go
muVal, ok := roomBroadcastMu.Load(roomID)
if !ok {
    muVal, _ = roomBroadcastMu.LoadOrStore(roomID, &sync.Mutex{})
}
mu := muVal.(*sync.Mutex)
mu.Lock()
defer mu.Unlock()
```

- Load hit（active room 第二次起的 broadcast）→ 不进入 if 分支，零 mutex alloc
- Load miss（room 首次 broadcast）→ LoadOrStore alloc 一个 mutex 写入 map（一次性）
- LoadOrStore 自身的双重 check 仍保证多 goroutine 同时首次 Load miss 进入 roomX 时只留一个 mutex（sync.Map 内部 atomic）

新增测试 `TestBroadcastToRoom_R3_PerRoomMutex_LoadFastPath_NoExtraAllocs`：
- 段1：同 room 调 100 次，roomBroadcastMu 内本 roomID 的 entry size 应为 1（mutex 复用，不重复创建）
- 段2：用 `testing.AllocsPerRun(100, ...)` 验证 hot path（room 已暖）每次 BroadcastToRoom alloc < 14（baseline 修复前 ≈ 15，修复后 ≈ 13；阈值 14 卡在中间精确捕获回归）

### 预防规则（Rule for future Claude）⚡

> **一句话**：在 hot path 上访问 sync.Map 拿"如果不存在就创建"的资源时，**必须先 Load 再 LoadOrStore**（双段式），不能直接 `LoadOrStore(key, NewExpensiveValue())` —— 因为 Go 参数在调用前求值，hit 时新 alloc 的资源被丢弃浪费一次 GC 周期。
>
> **展开**：
> - sync.Map.LoadOrStore 的第二参在 hit 时**仍会被求值**（Go 调用约定）—— 把 `&sync.Mutex{}` / `make(chan ...)` / `&MyStruct{}` 当第二参传入 = 每次调用都 alloc 一个被立即丢弃的 heap 实例
> - hot path 模式：先 Load fast path（hit 零分配），miss 才 LoadOrStore（一次性 alloc）。amortized 0 alloc：仅首次 miss alloc，之后所有调用零 alloc
> - 这条规则**仅对 hot path 重要**：低频路径 / 启动期一次性初始化 / map 内大概率 miss 的场景，直接 LoadOrStore 更简洁，省一次 Load 调用
> - 同类陷阱：`sync.Pool.Get` + 立即 Put 不需要的 obj、`map[K]V` 用 `m[k] = newV()` 覆写替换、`atomic.Value.Store(newV)` 无条件 store —— 共同点是"**新值的构造在条件之前完成**"。一律先检查再决定是否构造
> - **反例 1（修复前的样子）**：
>   ```go
>   muVal, _ := mp.LoadOrStore(key, &sync.Mutex{}) // 每次都 alloc 一个新 mutex
>   ```
>   测试用 testing.AllocsPerRun 跑 hot path benchmark，每次 ≥ 1 alloc（被丢弃的 mutex）。
> - **反例 2**（同类陷阱）：
>   ```go
>   chVal, _ := mp.LoadOrStore(key, make(chan T, 16)) // 每次 alloc 一个新 channel
>   ```
>   即使 mp 中已经有 channel，新 make 出来的 channel 也已分配缓冲区。修法相同：先 Load。
> - **反例 3**（构造昂贵）：
>   ```go
>   v, _ := mp.LoadOrStore(key, expensiveCompute(key)) // 每次都跑昂贵计算
>   ```
>   修法：先 Load，miss 才跑 compute。
> - 验证手段：用 `testing.AllocsPerRun` 把 hot path 包起来跑 100 次，对比修复前后 allocs/op 差值。差值 ≥ 1 = 实锤修对了一个 alloc。

## Meta: 本次 review 的宏观教训

sync 包的 API 在第二参/默认值是"懒 evaluation"还是"立即 evaluation"上很容易踩坑：

- **Go 调用约定**：参数永远在 callee 入口前完成 evaluation。无 lazy 参数。
- **sync.Map.LoadOrStore(key, value)**：value 立即求值，hit 时丢弃
- **map[k] = v if not exists**（手写）：v 也立即求值，需要先 `if _, ok := m[k]; !ok { m[k] = ... }` 才 lazy
- **defaultmap pattern**：`m.GetOrCreate(key, factory func() V)` 比 `m.GetOrCreate(key, V)` 好，因为 factory 仅在 miss 时被调

iOS / Swift 同类反例：`dict[key, default: ExpensiveStruct()]` 在 Swift 中也会**每次**求值默认值（不管是否 hit），需要 `dict[key] ?? { ExpensiveStruct() }()` 才 lazy。跨语言通用规则：**默认值是表达式时永远是立即求值，是 closure 时才 lazy**。
