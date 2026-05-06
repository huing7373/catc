---
date: 2026-05-06
source_review: codex /tmp/epic-loop-review-10-3-r9.md
story: 10-3-ws-网关骨架
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-06 — sessionID 截断 8 字符 = birthday paradox 内存腐败（10-3 r9）

## 背景

Story 10-3（WS 网关骨架）r9 review。codex 指出 `sessionManager.shortUUID()` 用 `uuid.NewString()[:8]` 把 sessionID 截到 8 字符前缀，等于把熵从 128 bit 砍到 32 bit；几千活跃 session 起 birthday paradox 碰撞概率不可忽略，碰撞会让 `sessionsByID` / `sessionsByRoom` 静默 overwrite 旧 entry，后续 `Unregister` / replacement 误删错的 session → 原始连接永久 leak（无人 Unregister，占内存 + 端口）。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | sessionID 用 `uuid.NewString()[:8]` 截 8 字符 = 32 bit 熵 → 内存腐败风险 | medium | architecture | fix | `server/internal/app/ws/session_manager.go:356-359` |

修了 1 条 / defer 0 条 / wontfix 0 条。

## Lesson 1: 进程内 map key 不能用截断 UUID —— 必须保留全 128 bit 熵

- **Severity**: medium
- **Category**: architecture
- **分诊**: fix
- **位置**: `server/internal/app/ws/session_manager.go:356-359`

### 症状（Symptom）

`shortUUID()` 返回 `uuid.NewString()[:8]`：8 个十六进制字符 = 32 bit 熵。`Register` 用这个值作为 `sessionsByID` 和 `sessionsByRoom[roomID]` 的 map key。当进程并发活跃 session 数到达几千 ~ 几万级别时，birthday paradox 碰撞概率从"可忽略"迅速上升到非平凡：

- 1000 session：~1.2e-4 概率（~0.012%；远不到非平凡区，但接近）
- 4096 session：~0.2%
- 40000 session：~50%（birthday paradox 拐点）

碰撞后果（不是"重连占用同 ID"那种良性场景，而是**两个不同 user 的不同 session 撞上同一 ID**）：

1. 第二个 Register 把 NEW session 写入 `sessionsByID[id]`，**静默覆盖** OLD session 的 entry
2. OLD session 仍持有 `closeNotifier`、仍在 readLoop / writeLoop 跑
3. OLD session 后续 Close → notifyClosed → Unregister(id) → 删的是 NEW session 的 entry（**误删**）
4. NEW session 在 manager 索引里"凭空消失"但底层 conn 还在；OLD session 没人管理但 conn 也还在 → 双重 leak

### 根因（Root cause）

设计时的注释写"节点 4 阶段单实例 + 单进程同时活跃 Session 远小于 4 万 → 32 bit 碰撞概率可忽略"，把"节点 4 阶段流量小"当成长期承诺。这是典型的**短期假设污染长期 contract** 反模式：

- 写注释当时确实合理（节点 4 阶段几十 / 几百 session）
- 但 sessionID 是**进程内全局 map key**，一旦实装就会被 10.4 / 10.5 / 10.6 / 10.7 / Epic 36+ graceful shutdown 全部依赖；一旦有人在节点 9 多实例部署时把 session 数堆到几千，bug 直接命中
- 节省的成本（log 行短 28 字符）vs 风险（map state 静默腐败 + 双 leak）严重不对等

更深层：**只要数据结构把"截断后的低熵字符串"作为 map key / index 用**，birthday paradox 就是悬剑。除非你有强约束在 Register 时**主动**做碰撞检测 + retry（一种补丁式修法），否则永远会被未来流量增长打脸。

### 修复（Fix）

`server/internal/app/ws/session_manager.go`：把 `shortUUID()` 改成 `newSessionID()`，返完整 36 字符 UUID v4（128 bit 熵）。同步更新所有提到"短串 / uuid 前 8 字符"的注释，对齐到"完整 uuid v4 = 36 字符 / 128 bit 熵 / 单实例进程内可视碰撞为 0"。

```diff
-// shortUUID 返回 uuid v4 前 8 个字符（约 32 bit 熵；节点 4 阶段单实例 + 单进程
-// 同时活跃 Session 远小于 4 万 → 32 bit 碰撞概率可忽略）。
-func shortUUID() string {
-    return uuid.NewString()[:8]
-}
+// newSessionID 返回完整的 uuid v4（36 字符，128 bit 熵；单实例进程内永不碰撞）。
+func newSessionID() string {
+    return uuid.NewString()
+}
```

加同包测试 `session_manager_internal_test.go`：

1. `TestNewSessionID_FullUUIDFormat`：断言 `len == 36` + 含 4 个 hyphen（防止有人改回截断）
2. `TestNewSessionID_NoDuplicatesIn10k`：直接调 `newSessionID` 1 万次，断言无重复（statistical sanity）
3. `TestSessionManager_Register_NoCollisionUnder1kSessions`：构造 1000 个不同 userID 的 Session 走 Register，断言每个返回的 sessionID 唯一 + `len(sessionsByID) == 1000`（验证没有 silent overwrite 导致 entry 丢失）

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **设计任何"作为 map key / index / 唯一标识符"用的 ID** 时，**禁止** 截断 UUID / hash 取前缀，**必须**保留完整熵（UUID v4 = 36 字符全留 / SHA-256 hash 全留 64 字符）。
>
> **展开**：
> - "log 短一点"不是有效理由：UUID v4 完整 36 字符，省 28 字符 vs map state 腐败风险严重不对等；grep 仍能用前缀模糊匹配（`grep "abcd1234"` 仍命中以该 8 字符开头的全 UUID）
> - "节点 X 阶段流量小所以碰撞可忽略"不是有效理由：ID 一旦实装就被下游模块 lock-in，未来流量增长会让它从安全变陷阱
> - 唯一允许低熵 ID 的场景：① 短期一次性、不进 map / 不做唯一索引（如 trace ID 仅用于日志关联且接受偶尔碰撞）；② 主动做"生成 + 碰撞检测 + retry"的 IDgen，且 retry 在 Register 锁内做（保证原子）。这是补丁式修法，结构性修法是直接全 UUID
> - **反例 1**（本次踩坑）：`return uuid.NewString()[:8]`，注释里写"32 bit 熵节点 4 够用"—— birthday paradox 在 4 万 session ~50% 碰撞，节点 9 流量上来直接打脸
> - **反例 2**：用 `time.Now().UnixNano()` 当 ID —— 高并发下同一 ns 多请求 collision；不是熵问题是 generator 问题但症状一样
> - **反例 3**：`md5(userInput)[:8]` 当资源 key —— 32 bit 熵 + 多用户共享空间，碰撞概率 + adversarial collision 双重风险
> - **正例**：`uuid.NewString()` 返 36 字符全保留；或 `crypto/rand` 出 16 字节 hex = 32 字符 = 128 bit；或 ULID（26 字符 lex-sortable + 80 bit randomness 单 ms 内仍稳）
> - **回归测试**：任何 ID generator 都应配 `len == expected` + `1k/10k 样本无重复`两个 sanity test，防止未来有人手贱加 `[:N]`

## Meta: 短期假设的注释陷阱

本次踩坑的更深一层教训：**注释里写"X 阶段够用"等于把短期流量假设刻进代码 contract，未来流量上来时既不会自动失效也不会触发任何告警**。

更安全的做法：

1. 不在代码注释里写"节点 X 阶段假设"作为正确性论据；要么直接做对（全 UUID），要么把假设挂到 ADR 上并写"未来超过 N 时必须 revisit"+ 指明 revisit 的具体 trigger
2. 给"假设依赖"加可观测信号：例如 manager 注册时，活跃 session 数超过某阈值（如 1000）触发一次 warn 日志，让流量爆发时有人工介入机会
3. 优先选**结构性修法**（全 UUID）而非补丁式（碰撞检测 + retry）—— 前者无需依赖未来人记得"哦原来这个 ID 有熵限制"

本次实装走结构性修法（全 UUID），代价仅日志多 28 字符，无任何运行时 / 兼容性 cost。
