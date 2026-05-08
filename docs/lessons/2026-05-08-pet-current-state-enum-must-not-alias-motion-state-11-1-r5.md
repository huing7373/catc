---
date: 2026-05-08
source_review: file:/tmp/epic-loop-review-11-1-r5.md (codex review round 5)
story: 11-1-接口契约最终化
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-05-08 — `pet.currentState` 枚举跨章节必须对齐 `pets.current_state` 而非 `motion_state`（11-1 r5）

## 背景

Story 11.1（节点 4 接口契约最终化）r5 review 由 codex 给出，仅 1 条 P2 finding：WS 章节 §12.3 把 `pet.currentState` 字段的 enum 标签写成了 `1 = stationary_or_unknown / 2 = walking / 3 = running`（"§6.5 motion_state 同义"），与 §10.3 REST 章节的 `1 = rest / 2 = walk / 3 = run`（"§6.4 pets.current_state 同义"）不一致。同一个字段名 `pet.currentState` 在 REST 和 WS 两条传输路径下分别给出两套语义不兼容的枚举命名，会让 server / iOS 实现拿不准用哪一套。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | `pet.currentState` enum labels 跨章节不一致（WS 章节误用 §6.5 motion_state 命名） | low (P2) | docs | fix | `docs/宠物互动App_V1接口设计.md:1839, 1920, 1953` |

## Lesson 1: `pet.currentState` 字段语义锚定到数据库 §6.4 `pets.current_state`（rest/walk/run），不要因为 §6.5 `user_step_sync_logs.motion_state` 同样是 1/2/3 枚举就误以为可复用其命名

- **Severity**: low (P2)
- **Category**: docs
- **分诊**: fix
- **位置**: `docs/宠物互动App_V1接口设计.md:1839, 1920, 1953`

### 症状（Symptom）

V1 接口设计文档里同一个字段 `pet.currentState` 出现在三处：

1. §10.3 GET /rooms/{roomId} 的 `data.members[].pet.currentState`（行 1261）：`1 = rest / 2 = walk / 3 = run`，注 "来源数据库设计 §6.4 `pets.current_state`"
2. §12.3 room.snapshot 的 `payload.members[].pet.currentState`（行 1839）：原文档误写为 `1 = stationary_or_unknown / 2 = walking / 3 = running`，注 "与数据库设计 §6.5 motion_state 同义"
3. §12.3 member.joined 的 `payload.pet.currentState`（行 1953）：原文档误写为 `1 = stationary_or_unknown / 2 = walking / 3 = running`

REST 和 WS 是同一个字段名 `pet.currentState`，从 client 视角它们必须共享同一套枚举语义；客户端不会因为消息走 HTTP 还是 WebSocket 就维护两套字段-枚举映射表。文档里给出两套不兼容命名，等于把语义歧义埋进契约。

### 根因（Root cause）

数据库设计文档里有**两个独立的 1/2/3 枚举**长得很像，命名却完全不同：

- §6.4 `pets.current_state` —— 用 `1 = rest / 2 = walk / 3 = run`（pet 自身的活动状态）
- §6.5 `user_step_sync_logs.motion_state` —— 用 `1 = stationary_or_unknown / 2 = walking / 3 = running`（client 上报步数同步时的瞬时运动态）

字段值范围都是 1/2/3 这种"看起来像同一个枚举"是陷阱：当 V1 设计文档作者写 §12.3 room 协议时，可能因为 "1/2/3 → 不就是 step sync 同一套吗？复用就好" 的捷径思维，把 motion_state 命名搬过来注成 "同义"。**问题在于：字段名 `pet.currentState` ↔ DB 列 `pets.current_state`（同名，仅大小写/下划线差），归属是 §6.4，不是 §6.5。** §6.5 的 motion_state 是给 step sync 用的，是 client → server 的瞬时上报，**不**是 pet 域的状态字段。两个枚举的取值范围相同纯属巧合（都是离散三态），不构成"同义"。

跨章节字段一致性的"金标准"应是：**字段名 ↔ DB 列**的映射先建立，再从 DB 列反推 enum 命名；**不能**走"取值都是 1/2/3 → 套就近的同样三态枚举"的歪路。

### 修复（Fix）

把 §12.3 三处 `pet.currentState` 的 enum 标签 + 注释统一改成 `1 = rest / 2 = walk / 3 = run` 并注 "（与数据库设计 §6.4 `pets.current_state` 同义）"。

具体改动：

- 行 1839 `payload.members[].pet.currentState`：`stationary_or_unknown / walking / running` → `rest / walk / run`，注释从 "§6.5 motion_state 同义" 改为 "§6.4 `pets.current_state` 同义"
- 行 1953 `payload.pet.currentState`：同上替换 + 补 "与数据库设计 §6.4 `pets.current_state` 同义" 说明
- 行 1920 placeholder 字段值来源说明里的 inline 注释 `1（stationary_or_unknown，Epic 14 才真实驱动 motion_state）` → `1（rest，与数据库设计 §6.4 pets.current_state 同义；Epic 14 才真实驱动）`

§12.4 `motionState` 字段（行 535-553）继续用 `stationary_or_unknown / walking / running`（这是 step sync 接口，归属 §6.5 motion_state，不动）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **跨 REST/WS 协议章节里写"含枚举字段"的契约表** 时，**必须** 先用 "字段名 ↔ DB 列名" 锚定枚举归属，**禁止** 仅凭 "取值范围长得像（如同样是 1/2/3）" 就把另一个 DB 表的枚举命名搬过来当 "同义" 注释。
>
> **展开**：
> - 协议字段命名 ≠ 取值范围；同一取值范围（如 1/2/3 三态）在 DB 设计里可能对应**多个独立的 enum 列**，每个列各自有自己的命名约定；这些命名是**不可互换**的契约名词。
> - 写跨章节字段表时，对每个 enum 字段做这个 zip 校验：① 字段名 → 找到对应 DB 列名（同名或下划线变体）→ 找到该列在 DB 设计里的命名 enum；② 该字段在所有章节（REST 响应 / WS payload / placeholder 注释 / future-fields 说明）里的 enum 命名必须**完全一致**。
> - 跨章节"复用枚举"是个反模式信号：当看到自己想写"与 §X.Y 某 enum 同义复用，不另起"时，先停一下问自己 "这两个字段是不是真的在 DB 层就是同一列？" —— 如果不是同一列，就是两个独立 enum，命名不可借用，即使取值范围相同。
> - 协议字段的"权威命名"必须落在 DB 设计文档里（单一 source of truth）；接口设计文档对每个 enum 字段都应有明确的 "（与 DB 设计 §X.Y 列名 同义）" 反查指针，便于 review 阶段做 zip 对齐。
> - **反例**：在 V1 接口设计 §12.3 写 `pet.currentState: 1 = stationary_or_unknown / 2 = walking / 3 = running（与 §6.5 motion_state 同义）` —— 错在 `pet.currentState` 对应的 DB 列是 §6.4 `pets.current_state`（rest/walk/run），不是 §6.5 `user_step_sync_logs.motion_state`；两列只是取值范围相同，命名是两套独立 vocab。
> - **反例**：在跨章节同步 enum 命名时，只检查"取值范围（如都是 1/2/3）一致"就放过 —— 必须检查"命名 vocab"也一致；否则 client 实现拿到 `currentState=2` 会困惑：到底叫 `walk` 还是 `walking`？两套命名意味着两套 client enum 类型，必然有一套实现是错的。
