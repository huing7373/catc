---
date: 2026-05-12
source_review: /tmp/epic-loop-review-14-1-r11.md (codex review, epic-loop r11)
story: 14-1-接口契约最终化
commit: 28a1d0e
lesson_count: 2
---

# Review Lessons — 2026-05-12 — Story 文件描述下游 Story 落地范围时必须三处统一（GET + room.snapshot + member.joined）+ References 错误码描述清理 1003 残留

## 背景

Story 14.1（V1 接口契约最终化）已经在 r1-r10 把 V1 文档 §5.2 / §12.3 / §1 锚定完毕，并显式约束 Story 14.3（RoomSnapshotBuilder 真实驱动）的落地范围 = `GET /rooms/{roomId}.data.members[].pet.currentState` + `room.snapshot.payload.members[].pet.currentState` + `member.joined.payload.pet.currentState` 三处同步切真实值。但本轮 codex review (r11) 抓到 story 文件本体 line 20 描述 Story 14.3 落地范围时只列了 GET + room.snapshot 两处，漏 `member.joined`；同时 References line 487 仍残留 "错误码 1002+1003"（r7 已锁定 §5.2 不再触发 1003，pet-less 走 200 noop）。两处都是 V1 doc 已对齐但 story 文件没同步的 drift，属于 r8/r9 的"story-vs-V1-doc resync sweep"延伸。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | Story 14.3 落地范围漏 member.joined | P2 / medium | docs | fix | `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md:20` |
| 2 | References 中 Story 14.2 错误码描述仍写 "1002+1003" | P3 / low | docs | fix | `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md:487` |

额外 sweep 命中（同次 fix 内对齐）：

| # | 位置 | 现状 | 修法 |
|---|---|---|---|
| s1 | line 448 "§10.3 / §12.3 三处 currentState" | 与 line 299 / 532 用的"四处"不一致，且未显式列 member.joined | 改为"四处"+ 逐字段显式列出（含 member.joined） |

## Lesson 1: Story 14.3 落地范围描述必须三处统一（GET + room.snapshot + member.joined）

- **Severity**: P2 / medium
- **Category**: docs
- **分诊**: fix
- **位置**: `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md:20`

### 症状（Symptom）

Story 14.1 文件 line 20 描述 Story 14.3 落地范围（"修改 RoomSnapshotBuilder - snapshot 含真实 pet.currentState"）时，只列了 `GET /rooms/{roomId}.data.members[].pet.currentState` 与 `room.snapshot.payload.members[].pet.currentState` 两处由 Story 14.3 切真实值，**漏掉** `member.joined.payload.pet.currentState`。但 V1 doc line 46 / 49 / 608 明确 Story 14.3 同时也切 `member.joined`（三处一起，同一落地点，避免 join 房间后 stale `1` 风险）。

风险：Story 14.3 实装者按 story 文件 line 20 只改两处 → 漏 `member.joined` → 用户在房间外切到 walk/run 后加入房间时，房间内已在场成员通过 `member.joined.payload.pet.currentState` 看到 stale `1`，直到下一次 `state-sync` 才纠正。

### 根因（Root cause）

Story 文件 line 20 是 14.1 的"下游立即依赖"段落，列举 Story 14.3 落地范围用的语言是从 r1 时段沿用 —— 当时 r1 / r2 的 V1 doc 描述还只覆盖两处（GET + room.snapshot）。后续 r7 / r8 在 V1 doc 把 `member.joined.payload.pet.currentState` 也补上 placeholder `1` + Story 14.3 真实驱动注释（V1 doc line 46 / 49 / 608），但 story 文件本体的"下游依赖描述段"没同步对齐。这是典型的"V1 doc 被 review 反复改，story 文件的 narrative 段落没跟上"漂移。

CLAUDE 在写 story 文件时，描述下游 story 范围用了"列举形态"（GET + room.snapshot），而没有在 V1 doc 锁定第三处 placeholder 后回头同步 story narrative。同时 line 448 也有类似遗漏（"三处" vs line 299 的"四处"）。

### 修复（Fix）

line 20 描述 Story 14.3 落地范围改为三处统一：

Before:
```
本 story 文档侧仅在 §5.2 末尾**引用**（"`GET /rooms/{roomId}.data.members[].pet.currentState` 与 `room.snapshot.payload.members[].pet.currentState` 由 Story 14.3 切换为真实读 `pets.current_state`..."）
```

After:
```
本 story **不**改 §12.3 `### 成员加入（member.joined）` schema（...）；本 story 文档侧仅在 §5.2 末尾**引用**（"`GET /rooms/{roomId}.data.members[].pet.currentState` 与 `room.snapshot.payload.members[].pet.currentState` 与 `member.joined.payload.pet.currentState` 三处统一由 Story 14.3 切换为真实读 `pets.current_state`..."）
```

同步对齐 line 448："§10.3 / §12.3 三处 currentState" → "§10.3 `data.members[].pet.currentState` / §12.3 `room.snapshot.payload.members[].pet.currentState` / §12.3 `pet.state.changed.payload.currentState` / §12.3 `member.joined.payload.pet.currentState` 四处"。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在**纯文档契约 story**（如 14.1 / 11.1 / 10.1 / 7.1 / 4.1）描述"下游 Story 落地范围"时，**必须**把 V1 doc 已锁定的"同一落地点"涉及的**所有** REST + WS 字段路径**逐字段列举**，**禁止**用"GET + room.snapshot"这种部分列举的简写。
>
> **展开**：
> - 同一落地点（如 Story 14.3 切换 `pets.current_state` 真实读）会触及多个 schema 字段（§10.3 GET / §12.3 room.snapshot / §12.3 member.joined）—— **每一个**都必须在 story narrative 显式列出。
> - 列举次数 vs 文档其他段落不一致是典型 drift 信号 —— line 20 用"两处"、line 299 用"四处"、line 448 用"三处" → 立即触发 sweep 把所有"X 处"措辞回校到 V1 doc 的真实锁定状态。
> - 反例 1：story line 20 只列 "GET + room.snapshot 两处" → 14.3 实装者只改 RoomSnapshotBuilder + GET handler，漏 member.joined broadcast pet.currentState → join 房间瞬间 stale `1`。
> - 反例 2：story line 448 写"§10.3 / §12.3 三处" 但 line 299 写"四处" → review 反复 ping 哪个数字才对。

## Lesson 2: References 段错误码描述必须与最新契约对齐（不能保留已废弃错误码）

- **Severity**: P3 / low
- **Category**: docs
- **分诊**: fix
- **位置**: `_bmad-output/implementation-artifacts/14-1-接口契约最终化.md:487`

### 症状（Symptom）

Story 14.1 文件 References 段 line 487 描述 Story 14.2 下游 AC 时，错误码列写"1002+1003"。但 r7 已锁定 §5.2 不再触发 1003（pet-less 走 200 noop）。这条 References 描述还停留在 r7 之前的契约语义。

风险：未来 dev-story Story 14.2 的 sub-agent 读 References 看到"1002+1003" → 在 handler/service 实装时把 1003 分支带回去 → 与 §5.2 锁定的 pet-less 200 noop 路径冲突。

### 根因（Root cause）

References 段是"指向 epics.md 的外部引用"，措辞描述了"目标文档的 AC 内容摘要"。r7 改了 §5.2 服务端逻辑（pet-less 不再走 1003，走 noop），但 References 段的 narrative 摘要没跟上。这是 r9 的"line 15 顶层 1003 引用"sweep 漏掉的第二个 1003 残留点。

### 修复（Fix）

line 487 描述措辞更新：

Before:
```
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 14.2 (行 2297-2319)] — 下游 state-sync 接口实装 AC（service 层 / 错误码 1002+1003 / 幂等性）
```

After:
```
- [Source: `_bmad-output/planning-artifacts/epics.md` §Story 14.2 (行 2297-2319)] — 下游 state-sync 接口实装 AC（service 层 / 错误码 1002（参数错误）—— r7 后 §5.2 不再触发 1003，pet-less 走 200 noop / 幂等性）
```

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 fix-review 改了 §5.2 错误码触发条件（如 r7 移除 1003）之后，**必须** sweep story 文件**所有引用错误码的 narrative 段落**，包括 References / Story 段 / 故事定位段 / Change Log 段，**禁止**只改"AC X 错误码全局对齐"段而漏掉 narrative 描述。
>
> **展开**：
> - 错误码描述漂移点常见区：(1) Story 顶部 narrative（line 11 / 15 / 19）；(2) 下游依赖描述（line 20 / 23 等）；(3) References 摘要（line 487）；(4) Change Log Decision 行；(5) AC 章节"全局对齐"段。任一处漏改都会让下游 dev-story sub-agent 困惑。
> - 反例：r7 改了 V1 doc + AC6 章节，但 References "错误码 1002+1003" 没改 → r11 codex 才抓到。

---

## Meta: 本次 review 的宏观教训

r8 / r9 / r11 三轮 codex review 都在抓"story 文件 vs V1 doc 漂移"。教训：**纯文档契约 story 在 review fix 改完 V1 doc 后，story 文件本体的"narrative 段（故事定位 / 下游依赖 / References / 既往 review 段）"必须做一次完整 sweep，不只改 AC 章节 + Change Log Decision 行**。Story 14.1 的 narrative 段引用了大量"§X.Y 由 Story Z.W 落地"措辞 / 错误码列表 / 字段路径枚举，每一处都要和 V1 doc 当前锁定语义对齐。

**预防 checklist**（写完 V1 doc 改动后，在 commit 前 sweep story 文件）：
1. `grep "1003\|1002+1003\|1002, 1003"` → 排除 §3 全局表 + "**不**触发 1003" 上下文外，所有命中必须改
2. `grep "Story X.Y"` 对每条 review 反复改过的下游 story → 验证 narrative 描述与 V1 doc 一致
3. `grep "X 处"` / `grep "三处\|四处"` → 数字一致性
4. References 段每条 entry 的"摘要措辞"与 V1 doc 当前锁定语义对齐
