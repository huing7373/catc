---
date: 2026-05-12
source_review: /tmp/epic-loop-review-14-1-r6.md (codex)
story: 14-1-接口契约最终化
commit: <pending>
lesson_count: 2
---

# Review Lessons — 2026-05-12 — state-sync `err` 二分锁定 & placeholder 例外白名单必须覆盖 self HTTP ack（14-1 r6）

## 背景

Story 14.1 r6（接口契约最终化）codex review，针对 `docs/宠物互动App_V1接口设计.md` 第 §5.2 `state-sync` 服务端逻辑步骤 4 和 §12.3 `room.snapshot` client merge contract 临时窗口例外两处仍残留的"实装层无法落地"歧义。

两条 finding 都属同一类问题：**契约文档把"互斥的二分"写成了"二分 + 一个对冲表述"**，导致 Story 14.2 实装侧或 iOS Story 15.x 实装侧都能从同一段文字里 cite 出两种相互冲突的实装路径。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联位置 |
|---|---|---|---|---|---|
| 1 | §5.2 步骤 4 "err==nil 即成功" 与 "pet row 消失 → 1009" 自相矛盾 | P2 / medium | docs | fix | `docs/宠物互动App_V1接口设计.md:532-537` |
| 2 | line 2092 placeholder 例外白名单未覆盖 self-entry HTTP 200 ack 来源 | P2 / medium | docs | fix | `docs/宠物互动App_V1接口设计.md:2094-2097` |

## Lesson 1: §5.2 步骤 4 "err==nil 一律成功" 必须是真正的二分锁定，不能并存"row 消失 → 1009"对冲表述

- **Severity**: P2 / medium
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:532-537`

### 症状（Symptom）

r1 已修过"`RowsAffected` 矛盾"，把成功判定锁定在 `err == nil`；但同段紧接着又写了一句"如果 pet row 在步骤 3 到步骤 4 之间消失 → 应走 1009 路径并 log error"。这两句话在实装层是**不可同时遵守**的：

- 一个工程师按 r1 锁定的 `err == nil ⇒ 200`、不读 `RowsAffected` 落地，pet row 消失时 MySQL 返回 `RowsAffected == 0` 但 `err == nil` → 走 200 OK
- 另一个工程师按"row 消失 → 1009"那句落地，必须读 `RowsAffected` 才能判定 row 是否消失 → 实装层就回到了 r1 之前的状态

两者都能合法引用同一份契约文档，造成 14.2 service 实装无单一权威路径。

### 根因（Root cause）

修一处歧义时**只删错的、没扫同段的"补丁式辩护话术"**。原句"步骤 3 SELECT 已确认 pet 行存在，从步骤 3 到步骤 4 之间的'被删'窗口在节点 5 阶段无业务路径触发，且即便发生也属 DB 层不一致 —— 应走 1009 路径"是 r1 之前为了"如果 row 消失怎么办"加的兜底解释，r1 把主规则换成 `err == nil ⇒ 200` 之后，这段兜底解释**逻辑上已经失效**（因为新规则下不读 `RowsAffected`，根本不区分"row 消失"和"幂等同值"），但文字残留在原段中，构成对冲。

这是 review 修复中的**典型遗漏**：把"主规则"改了，但同段为旧主规则做辩护的"边角解释"没扫干净，导致旧规则的逃生通道在文档里残存。

### 修复（Fix）

`docs/宠物互动App_V1接口设计.md:532-537` — 把步骤 4 改写为真正的二分锁定：

1. **`err == nil`** → 一律 200 OK + code = 0，service 层**不**读 `RowsAffected`、**不**根据该值分支
2. **`err != nil`** → 1009

并新增一段"关于 step 3 → step 4 之间 pet 行消失"的**显式取舍**：
- 节点 5 阶段无业务路径触发 DELETE pets，理论不可达
- 即便发生，实装仍走 `err == nil ⇒ 200` 路径，**不**降级 1009、**不**降级 1003、**不**读 `RowsAffected`
- client 端的影响仅为"单次 self UI 状态未真正落库"短窗口偏差，下一次 state-sync 调用会重新写入
- **契约层不为该 0 概率分支预留任何错误码出口**，避免 service 层为兜底而引入 `RowsAffected` 判定

最后加一行**实装锁定**："1009 ⇔ `err != nil`；200 OK + code = 0 ⇔ `err == nil`；这是两个互斥的二分，service 层不存在第三条路径"。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **修一段契约里"主规则歧义"** 时，**必须**同步扫掉同段为旧主规则做辩护的"边角解释 / 兜底话术 / 极端 race 兜底"，确保新主规则下这些话术**逻辑上已失效**的部分被显式删除或改写成对新主规则的支持。
>
> **展开**：
> - 修主规则时第一时间在同段做"残留辩护话术"扫描 —— 旧主规则的辩护语在新规则下是否还成立？不成立则同删
> - 对于"无错误码出口"的极端分支（如 0 概率 race），契约层**应显式声明"不预留错误码出口"**，而不是写"应走 XXX 错误"留尾巴
> - 锁定二分时用 **"⇔" 符号 + "互斥的二分" + "不存在第三条路径"** 这种斩钉截铁的措辞，避免 reader cite 出"还有 case C 怎么办"
> - 同段同时存在两种处置路径时，下游实装方就能合法 cite 两种实装 —— review 必须查这种二分对冲
> - **反例**：修了"err==nil ⇒ 200"主规则但同段仍保留"row 消失 → 1009 路径并 log error"的辩护话术，让 14.2 service 实装能合法 cite 出"读 RowsAffected 走 1009"的路径，等价于主规则未修

## Lesson 2: 临时窗口 placeholder 例外的"真实值来源白名单"必须按 self / others 分桶 + self 桶必须包含 HTTP ack 来源

- **Severity**: P2 / medium
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:2094-2097`

### 症状（Symptom）

r5 加的临时窗口 placeholder 例外把"非 1 真实值"的来源限定为"`pet.state.changed` WS 广播"，但 r5 同时新加的 §5.2 line 547 self-broadcast 兜底规则允许 self entry **从 HTTP 200 ack 立即更新**（而非等 WS）。这两段在 self-broadcast 丢失场景下打架：

1. caller 发 `POST /pets/current/state-sync` → 收到 HTTP 200 OK `data.state: 2` → self UI 立即更新为 `walk`
2. self-broadcast WS 消息丢失（fire-and-forget 不重试，line 2230 锁定）
3. 之后任意时刻 caller 因重连收到 `room.snapshot` 或新成员触发 `member.joined` → payload 里自己的 `currentState: 1`（14.3 前 placeholder）
4. client 走 line 2092 例外判定："已有非 1 值的来源是 HTTP 200，**不**在 WS 白名单内" → 走"直接覆盖"路径 → self state 回退到 `rest`

这就是临时例外**本意要避免**的 stale-state 风险，仍在 self-broadcast 丢失 + HTTP 200 ack 单信号场景下成立。

### 根因（Root cause）

r5 在写临时窗口例外时，把"真实值来源"默认等同于"WS pet.state.changed 广播"，**没意识到 r5 自己同时新加的 self-broadcast 兜底规则把 HTTP 200 ack 也提拔成了 self-only 权威信号源**。两段（self-broadcast 兜底 + placeholder 例外白名单）在同一轮 review 修复中并行落地，但白名单语句的来源枚举没和兜底规则的来源枚举对齐，留下 self entry 从 HTTP 200 走的路径在白名单里"无家可归"。

更深层的根因：**契约里凡是说"非 X 真实值"，都隐含一个"真实值来源集合"**。这个集合应该枚举出**所有合法来源**，并按 entry 类型（self vs others）分桶 —— self entry 可能有 HTTP ack 来源（caller 自己发的请求响应），others entry 没有（caller 自己的 HTTP 响应不承载别人状态）。只写"WS 广播是真实值来源"等于隐式假设 entry 不分 self / others，会漏 self-only 路径。

### 修复（Fix）

`docs/宠物互动App_V1接口设计.md:2094-2097` — 给临时窗口例外加显式"真实值来源白名单"分桶：

- **others entry**：非 1 真实值**仅**来自 `pet.state.changed` WS 广播 `payload.currentState`（others 不存在 HTTP ack 来源 —— caller 自己的 HTTP 响应不承载别人状态，引用 §5.2 line 545）
- **self entry**：非 1 真实值可来自 **(i) `pet.state.changed` WS 广播** 或 **(ii) `POST /pets/current/state-sync` HTTP 200 OK `response.data.state`**（即 self-broadcast 例外规则下的 self-only 权威 HTTP ack 信号，引用 §5.2 line 547）

并新增一段**漏洞封堵说明**显式描述：如果不把 HTTP 200 ack 纳入 self 桶白名单，self-broadcast 丢失场景下 self state 会被晚到的 placeholder snapshot/member.joined 回退到 rest，违反 §5.2 line 547 self-broadcast 兜底规则的 invariant；纳入后，self UI 在两路 server → client 信号（HTTP 200 / pet.state.changed）任一到达后即获 placeholder override 保护。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **同一轮 review 修复中同时新增"信号来源 X"和"信号来源白名单 Y"** 时，**必须**主动 cross-check 两段的来源枚举集合是否**对称封闭**（X 出现的所有路径都在 Y 的白名单里有对应桶），否则在 X 单信号场景下白名单会反过来打掉 X 写入的真实值。
>
> **展开**：
> - 在写"X 是真实值"时，**同步**问"X 是否被所有读取 X 的下游契约（如白名单 / 优先级排序 / 等价桶声明）正确接纳"
> - 白名单类语句应**按 entry 类型 / 视角分桶**枚举来源（self vs others / caller vs callee / inbound vs outbound），不能只写"来自 X" —— 隐式假设"所有 entry 类型都只有 X 一种来源"，会漏掉部分 entry 类型独有的来源
> - 多信号路径设计时，每条信号路径应在所有"权威信号 / 真实值 / 等价桶"等下游契约段里**显式枚举到位**，不能让某些信号路径"通过对称推理隐式成立"—— 隐式成立的路径在 review 阶段一定会被打掉
> - 检查"丢一路保一路"场景：如果两路信号中只到达一路（典型：WS 丢失但 HTTP 到达），单路信号写入的真实值能否被后续 placeholder / snapshot / 第三方信号源正确尊重？
> - **反例**：写"self entry 可从 HTTP 200 立即更新"（信号源 A）+ 写 placeholder 例外"已有非 1 值来自 WS 广播则跳过覆盖"（白名单只列信号源 B）→ self-broadcast 丢失场景下 HTTP 200 写入的真实值被 placeholder 覆盖回 `rest`，A 单信号路径名存实亡

## Meta: 本次 review 的宏观教训

两条 finding 表面看是两个独立位置的契约表述漏洞，但**同源**：都是**前几轮 review 修复中"主规则换新 / 新增信号路径"时没扫尾"为旧主规则辩护的边角解释 / 新主规则下应同步扩张的下游枚举集合"**。即"修主条但没收尾"。

具体来说：

1. r1 把 step 4 主规则改成 `err == nil ⇒ 200`，但旧规则下"row 消失走 1009"的辩护话术没删 → Finding 1
2. r5 加 self-broadcast HTTP 200 兜底，但临时窗口例外白名单的来源枚举没扩到 HTTP 200 → Finding 2

**未来 Claude 修契约文档的扫尾纪律**：
- 改主规则 → 用主规则的关键字符串（如 `RowsAffected` / `pet.state.changed`）反向 grep 整个文档，逐处确认是否需要同步更新
- 加新信号路径 → 列出新路径要被引用的所有下游契约（优先级排序 / 等价桶 / 白名单 / merge 规则），逐处检查是否完整枚举
- review fix 不只是"加新段说清楚"，还要"删掉 / 改写新主规则下逻辑失效的旧段"
