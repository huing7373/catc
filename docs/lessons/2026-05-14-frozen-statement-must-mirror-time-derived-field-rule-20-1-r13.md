---
date: 2026-05-14
source_review: codex review round 13 of story 20-1（/tmp/epic-loop-review-20-1-r13.md）
story: 20-1-接口契约最终化
commit: b81a772
lesson_count: 2
---

# Review Lessons — 2026-05-14 — 节点冻结声明 + 跨段 summary 必须随核心步骤迭代（20-1 r13）

## 背景

Story 20-1 经过 r1 dev-story + r3 ~ r12 共 12 轮 fix-review，§7.2（POST /chest/open）的服务端逻辑在 r9 / r10 / r11 三轮做了关键重构：

- r9：把 `nextChest.status` 加入"不持久化到 `response_json` + 回放时同源同时刻实时计算"列表（与 `remainingSeconds` 同处理）
- r10：rate_limit 检查从全局 middleware 挪到 handler 内层、置于幂等命中预检之后
- r11：committed success replay 短路点重排到**步骤 3 (committed success precheck)** —— 不进入业务事务；r10 时的"步骤 5b cached success 分支"被吸收 / 重命名

r12 review 之后，r13 codex review 锁定了仍残留的两处文本漂移：

> [P2] Stop freezing `nextChest.status` as a constant `1` — V1 line 69 ... This freeze note hard-codes `nextChest.status=1`, but later in the same contract `nextChest.status` is explicitly recomputed from `unlockAt` on cached replays and can legitimately become `2` once the next chest has matured.
>
> [P2] Keep the success-path replay summary aligned with step 3 — V1 line 1007 ... This summary regresses the cached-success rules by saying a same-key retry returns through `5a/5b` and only rehydrates `remainingSeconds`. The normative flow above says normal committed-success retries short-circuit at step 3 and must recompute both `nextChest.status` and `nextChest.remainingSeconds`.

两条都是 r9 / r11 重构的"漏网 summary 段"—— canonical 规范段（§7.1 GET / §7.2 步骤 3 + 关键约束 r11 锁定段）已经全部讲对，但**节点 7 冻结声明**和**「写后保证 → 成功路径」摘要段**两个跨节点 summary 仍停留在 r9 前 / r11 前的旧描述。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | §1 节点 7 冻结声明把 `nextChest.status` hard-code 成 `1`（r9 已锁定为 time-derived） | medium | docs / architecture | fix | `docs/宠物互动App_V1接口设计.md:69` |
| 2 | §7.2 关键约束「写后保证 → 成功路径」段 summary 仍写"5a + 5b"路径（r11 已重排到步骤 3 短路） | medium | docs / architecture | fix | `docs/宠物互动App_V1接口设计.md:1007` |

## Lesson 1: 节点冻结声明里的字段语义必须与契约规范段同步精度

- **Severity**: medium
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:69`（§1 节点 7 冻结声明列出"必须冻结"字段集时）

### 症状（Symptom）

§1 节点 7 冻结声明在列举本接口需冻结的契约项时，把 `nextChest` 字段写成：

> `nextChest` 永远非 null 且 server 端**固定 status=1** / unlock_at=now+10min ...

这与 §7.2 服务端逻辑 + 关键约束「r9 决策段」+「r11 锁定段」的契约不一致 —— 后者已经把 `nextChest.status` 锁定为 **time-derived 字段**：首次返回 1，同 key 重试时 server 端按 `(unlock_at > now) ? 1 : 2` 实时计算。如果下游 server (Story 20.6) / iOS (Epic 21.x) 实装时按"§1 冻结声明 = 节点级权威 SOP"读契约，会把 `nextChest.status` 实装成常量 1（持久化进 `response_json`）→ 重试发生在新 chest 已解锁时刻自然漂移成 stale `status=1` + 实时 `remainingSeconds=0` 不可能组合，与 §7.1 GET 同一秒查询结果撕裂，恰好回到 r9 review 想要消除的 stale 组合。

### 根因（Root cause）

**"契约规范段（normative）随多轮 review 滚动迭代，但节点级 summary 段（freeze declaration）停留在初版"的偏差结构**。具体触发链：

1. r0 写节点 7 冻结声明时，把 `nextChest.status` 当成"server 端首发即固定 1"的简化模型（首次响应确实就是 1）
2. r9 review 锁定 `status` 是 time-derived（与 `remainingSeconds` 同处理）→ §7.2 关键约束 / `response_json` 缓存范围 / r9 决策段三处都同步了，但 **§1 冻结声明仍写"固定 status=1"**
3. r10 / r11 review 都只关注 §7.2 服务端逻辑步骤，不再回看 §1 节点 7 冻结声明 → 漂移累积到 r12 仍未发现
4. r13 codex 把 §1 冻结声明与 §7.2 关键约束对照读，立刻命中矛盾

更深层根因：**冻结声明在契约文档中承担"节点级一句话权威 SOP"的角色** —— 下游实装时若只读 §1 冻结声明这种 high-level summary（而不深入读 §7.2 服务端逻辑步骤详情），会被该 summary 误导。它和 normative section 必须 1:1 mirror，不允许在精度 / 抽象级别上漂移。

### 修复（Fix）

把 `docs/宠物互动App_V1接口设计.md:69` 节点 7 冻结声明中关于 `nextChest` 的部分改成与 §7.2 关键约束 r9 / r11 锁定段一致：

```diff
- **节点 7 阶段 `reward.userCosmeticItemId` 占位值 `"0"` 契约** / `nextChest` 永远非 null
- 且 server 端固定 status=1 / unlock_at=now+10min 的修改都必须 ...
+ **节点 7 阶段 `reward.userCosmeticItemId` 占位值 `"0"` 契约** / `nextChest` 永远非 null
+ 且 server 端 `unlock_at = now + 10min`、`nextChest.status` 首次返回 1 但同 key 重试时
+ server 端按 `(unlock_at > now) ? 1 : 2` 实时计算（**time-derived**，与 §7.1 GET /chest/current
+ 同语义，详见 §7.2 关键约束 r9 / r11 锁定段） 的修改都必须 ...
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **修改契约文档的某个 normative section（如服务端逻辑 / 关键约束 / 数据库 schema 字段范围）** 时，**必须**显式 grep 全文档检查该字段 / 状态 / 路径是否出现在 **任何节点级冻结声明 / 跨段 summary / 字段表 / Change Log** 中，**有则同步修改、无则在 Change Log 注明"仅 normative 段，无 summary 引用"**。

> **展开**：
> - 契约文档典型有三层叙述：**(a) 字段表 / 服务端逻辑步骤（normative 权威）**、**(b) 关键约束段 / 决策段（解释 + 不变量）**、**(c) 节点冻结声明 / 跨章节 summary（high-level SOP）**。三层必须 1:1 mirror，不允许在精度 / 抽象级别上漂移。
> - 触发 grep 的关键字范围：被改字段名（`nextChest.status` / `remainingSeconds`）+ 状态枚举值（`status=1` / `'success'`）+ 路径标识（"步骤 3" / "步骤 5a" / "5b"）+ 错误码（`1008` / `1009`）+ 设计决策名（"committed success replay" / "MVCC pending 不可见"）。
> - 用 `grep -n` + 浏览匹配前后文 5 行决定是否需要 propagate；不能只看行号 hit count，要看每个 hit 的语义角色（normative / summary / 历史 Change Log 已归档）。
> - 历史 Change Log（如 "round 9：把 status 也加入..."）**不**改 —— 它记录的是历史事实；要改的是"当前生效的契约描述"。
> - **反例**：r9 锁定 `nextChest.status` 为 time-derived 后，只改了 §7.2 关键约束 + `response_json` 字段范围 + §13.3 数据库设计 §5.16 三处，**漏掉** §1 节点 7 冻结声明的 "server 端固定 status=1" —— 这次 r13 review 才发现，已经累计漂移 4 轮（r9 / r10 / r11 / r12）。

## Lesson 2: 重排服务端逻辑步骤号 / 短路点后，所有跨段 summary 必须同步重写

- **Severity**: medium
- **Category**: docs / architecture
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1007`（§7.2 关键约束「DB 持久化幂等的写后保证 → 成功路径」段）

### 症状（Symptom）

§7.2 关键约束「DB 持久化幂等的写后保证」段「成功路径」子段写：

> 同 key 重试在步骤 5a 拿到 affected_rows=0 + 步骤 5b 走 status='success' 分支 ... 实时补算 remainingSeconds + 填 requestId

这是 r5 ~ r10 的描述（r10 锁定 "rate_limit 在 middleware 之外、handler 内层、幂等命中预检之后"，但仍把 cached success 路径放在步骤 5b）。**r11 review 显式重构**：committed success replay 短路点移到**步骤 3 (committed success precheck)** —— 不进入业务事务、跳过 rate_limit、跳过 5a INSERT；同时 r9 锁定 cached 路径必须**同时**补算 `nextChest.status` + `nextChest.remainingSeconds` 两字段。

旧 summary 文字误导面有三：
1. 短路位置错（5b 而非 3）→ 暗示重试必经业务事务 + rate_limit
2. 仅讲补算 `remainingSeconds`，漏 `status`（r9 锁定的双字段）
3. 让"步骤 5b"看起来仍是 cached success 的承重路径（r11 已经把 5b 重新定位为"X-lock 阻塞 + affected_rows = 0 后 short-read 到 success → 返回 cached"的兜底分支，不是首选路径）

下游 server (Story 20.6) 若按本段实装，会写错 idempotency 命中预检短路点 / 漏 status 补算 / rate_limit 配额计算错位，与 §7.2 服务端逻辑步骤 3 + §7.2「rate_limit 位置 r10 调整 + r11 修订」段双双冲突。

### 根因（Root cause）

**"normative section 重排步骤号 / 短路点后，跨段 summary 没有跟随重写"的偏差结构**。具体触发链：

1. r11 review 重构 §7.2 服务端逻辑步骤 3 / 4 / 5 的功能划分（步骤 3 从"读检查 + 走 5b 短路"重命名为"committed success precheck 直接短路返回"；步骤 4 改成"未命中 idempotency 才计入 rate_limit"；步骤 5b 降级为兜底分支）
2. r11 fix 同步了 §7.2 服务端逻辑详细步骤 + §7.2 关键约束「rate_limit 位置 r10 调整 + r11 修订」段 + §7.2 关键约束「MVCC 下 pending 不可见 r11 决策」段 + 数据库设计 §5.16 r11 锁定段
3. 但「DB 持久化幂等的写后保证 → 成功路径」段（一段早期 summary）**没改** —— 它的语义在 r5/r6/r7 时是正确的，r11 时只是"路径描述层"过时了，r11 修订时未 grep 到该 summary 段
4. r12 review 关注 story 文件漂移，不重审跨段 summary；r13 codex 把 §7.2 关键约束 r11 锁定段 vs 「写后保证」段对照读时立刻命中冲突

更深层根因：**当 normative section 的"步骤号 / 短路位置 / 配额配置"这类"结构性元数据"变化时，跨段 summary 的引用整段语义都会失效**，不像字段名 / 状态枚举值漂移那么"局部"。这类重构必须把所有引用同一结构的 summary 段全部找出来重写。

### 修复（Fix）

把 §7.2 关键约束「写后保证 → 成功路径」段重写为：

```diff
- - **成功路径**：步骤 5l commit 成功 → idempotency 行的 `status = 'success'` + `response_json`
-   （**不含** `nextChest.remainingSeconds` / **不含** `requestId`，避免回放 stale 倒计时 /
-   破坏 trace 语义）已**原子**落盘 → 同 key 重试在步骤 5a 拿到 `affected_rows = 0` + 步骤 5b
-   走 `status = 'success'` 分支直接返回缓存的 `response_json` + 实时补算 `remainingSeconds`
-   + 填入**当前**请求的 `requestId`；**零 Redis 依赖**、**零"SET 失败 client 卡死"风险**、
-   **零回放 stale 倒计时**、**零回放 stale requestId**
+ - **成功路径**：步骤 5l commit 成功 → idempotency 行的 `status = 'success'` + `response_json`
+   （**不含** `nextChest.status` / **不含** `nextChest.remainingSeconds` / **不含** `requestId`，
+   避免回放 stale 状态 / stale 倒计时 / 破坏 trace 语义）已**原子**落盘 → 同 key 重试在
+   **步骤 3 (committed success precheck)** 命中 `status = 'success'` 行 → 直接返回缓存的
+   `response_json` + **同时**补算 `nextChest.status` 和 `nextChest.remainingSeconds`
+   （**同 now 快照**，同源同时刻）+ 填入**当前**请求的 `requestId` → 跳过步骤 4 rate_limit +
+   跳过步骤 5 业务事务；**零 Redis 依赖**、**零"SET 失败 client 卡死"风险**、
+   **零回放 stale 状态**、**零回放 stale 倒计时**、**零回放 stale requestId**
```

同时把段头标签从 "r7 review 修订" 扩成 "r7 review 修订 / r11 review 锁定：成功路径短路点重排到步骤 3"，方便后续 review 快速定位。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **重排 normative section 的结构性元数据**（步骤号 / 短路位置 / 流程分支 / 配额计算位置 / 中间件层级）时，**必须**用结构性关键字（"步骤 N" / "5a" / "5b" / "committed success replay" / "X-lock 阻塞" / 错误码"1008"）grep 全文，**逐 hit 评估**是否需要同步重写，**不能仅改步骤详情段就声称重排完成**。

> **展开**：
> - 结构性元数据漂移**比字段语义漂移更危险** —— 字段语义错可能只影响单个值的实装，结构性元数据错会让整条业务路径走错（如 cached success 该走步骤 3 走成了步骤 5）。
> - grep 关键字优先级（从最高优）：**步骤号 + 状态枚举值组合**（"步骤 5b 走 status='success'"）> **错误码 + 触发条件组合**（"1008 在节点 7 阶段"）> **设计决策代号**（"r11 锁定 MVCC 下 pending 不可见"）> **字段名**（`requestId` / `nextChest.status`）。
> - 跨段 summary 通常出现在三个位置：**(a) "关键约束"段内**（每个段头 r7/r9/r11 标记的就是这类 summary）、**(b) 节点级冻结声明**（§1 末尾）、**(c) Change Log 末尾的"r-轮次"逐行回顾段**。前两类需要随核心改动同步重写；第三类记录历史不改。
> - 重排完成的自检 SOP：随便选一个"重构前的旧词"（如 "5b 走 status='success'"）grep 一遍，预期 hit 数 = 历史 Change Log 段数（不算需修订段）；若 hit 数高于预期 → 有遗漏段。
> - **反例**：r11 重排 cached success 短路点从"步骤 5b 短路"到"步骤 3 短路 + 步骤 5b 降级为兜底"时，只改了 §7.2 服务端逻辑步骤 3 / 4 / 5 详情段 + §7.2「rate_limit 位置 r10/r11」段 + §7.2「MVCC pending 不可见」段 + 数据库设计 §5.16 r11 锁定段，**漏掉**§7.2「写后保证 → 成功路径」段（一段早期写在"关键约束"段里的 summary）—— 这段在 r5 / r6 / r7 时是对的，r11 时短路位置改了它就过时了。r13 review 才发现。

---

## Meta: 本次 review 的宏观教训

**契约文档跨段同步：核心步骤改了之后，所有 summary 段都要找出来同步。**

r9 / r11 两轮的核心 normative 改动都正确落地了，但**跨段 summary 引用**（§1 节点 7 冻结声明 + §7.2「写后保证 → 成功路径」段）**没有同步迭代**。这是 r12 lesson "story 文件自身需追踪契约漂移" 之外的**另一种漂移**：**契约文档内部的 summary 段相对于 normative 段的漂移**。

两条 finding 的共性根因可以蒸馏为一条 cross-cutting 规则：

> **契约文档随每一轮 review 滚动时，应该把"normative section 改了什么"和"summary section 需要 mirror 什么"作为两个独立的修订维度处理。前者 fix-review 流程自然覆盖（review 直接 surface），后者需要主动 grep + 逐 hit 评估。**

具体可执行的自检 SOP（建议未来 fix-review 在改完 normative 段后跑一遍）：

1. 列出本轮 review 改了哪些"结构性元数据"（步骤号 / 状态枚举 / 路径短路点 / 错误码语义 / 字段范围）
2. 对每个元数据，挑一个最具区分度的"旧词"（重构前的精确措辞，如 "步骤 5b 走 status='success'"），grep 全文档
3. 逐 hit 评估：normative / summary / 历史 Change Log 三类角色，只改前两类
4. 自检通过条件：grep 结果中**所有 summary 类 hit** 都已同步到当前轮次的语义

这条 meta 规则也适用于本仓库其他长契约文档（数据库设计 / 时序图 / 总体架构）的多轮迭代。
