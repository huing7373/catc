---
date: 2026-04-30
source_review: file:/tmp/epic-loop-review-37-2-r2.md (codex review --base <baseline>, round 2, Story 37.2)
story: 37-2-adr-0010-appstate
commit: <pending>
lesson_count: 1
---

# Review Lessons — 2026-04-30 — 路径 B ADR §6 验证语义的 inline forward annotation（codex round 2 协调）

## 背景

Story 37.2「ADR-0010 AppState（决策落地、非实装）」codex review round 2 重申了 round 1 已 wontfix 的同一项 finding（ADR-0010 §6 第 2-4 条 checkbox 物理依赖 Story 37.4，但 37.4 仍 backlog）。round 2 的论点比 round 1 更细：「README 已正确改未来时 + sprint-status 仍标 37-4 backlog → ADR §6 marked done 形成 internally inconsistent state，对未来 reviewer 是 'false-success signal'，可能让 Story 37.4 跳过验证」。

round 1 lesson `docs/lessons/2026-04-30-doc-tense-vs-path-b-adr-acceptance.md` Lesson 2 已经详细论证了路径 B 钦定状态 + 同类先例（Story 37.1 + ADR-0008 v2）+ 下游 codex review 兜底物理验证三要素。round 2 真正的新 surface area 是：codex 已经看到了 README 改未来时 + sprint-status 标 backlog 的事实，路径 B 解释如果只在 round 1 lesson 里、不在 ADR §6 内就近可见，「future reviewer 误导」担忧仍站得住。

本 lesson 记录 **option C 应对模式**：保留 §6 checkbox 勾选（路径 B 钦定不动），但在 §6 段顶加 inline annotation 把路径 B 验证语义 forward 到 ADR 内，让未来 reviewer 在 ADR §6 现场就能读到「checkbox 验证语义 = 契约级签字 + 下游 codex review 兜底」，回应 codex 的 false-success 担忧而不让步路径 B。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | ADR-0010 §6 checkbox 2-4 路径 B 钦定状态需就地说明（防止 future reviewer 误读 + codex round 3 复发） | P2 / medium | architecture / docs | fix（option C：保留勾选 + inline annotation） | `_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md:312-315` |

## Lesson 1: round 1 wontfix lesson + round 2 同 finding 复发 → option C 把 lesson 解释 inline forward 到原文

- **Severity**: P2 / medium
- **Category**: architecture / docs / process
- **分诊**: fix（option C）
- **位置**: `_bmad-output/implementation-artifacts/decisions/0010-iphone-app-state.md` §6 段顶

### 症状（Symptom）

round 1 给同一 finding 判 wontfix，理由记入 lesson；round 2 codex review 复述同一 finding 且论点加强。这意味着：

1. round 1 lesson 不在 codex 的 review 上下文里（lesson 是 commit 后归档物，codex 拿 baseline..HEAD diff）
2. ADR §6 现场没有就地说明路径 B 验证语义，仅靠 ADR Status: Accepted + Sprint Change Proposal v2.5 commit reference 隐含传递
3. 5 round cap 内若每 round 都判 wontfix 而 lesson 不 forward 到 ADR 现场，理论上 codex 可以无限复述（每次只看 baseline..HEAD diff，看不到 lesson 变化）

### 根因（Root cause）

路径 B 的验证语义本应在 ADR 自身就地写明，而不是只放在 lesson 文件里。lesson 是「未来 Claude 学习材料」，不是「面向 ADR 当前 reviewer 的语义说明」。两者目标读者不同：

- lesson → 未来 Claude 写新 story 时学习避坑
- ADR §6 inline annotation → 当前 / 未来 codex review、人工 review、Story 37.4 实装者就地理解 §6 checkbox 语义

round 1 fix-review 漏了这个区分，把路径 B 解释只 sink 到 lesson，没 forward 到 ADR。round 2 codex 论点的"false-success signal"担忧本质是 ADR 现场缺少语义说明 → 把 lesson 内容 inline forward 是直接对症。

### 修复（Fix）

ADR-0010 §6 标题下、checkbox 列表上方插入 1 段 blockquote annotation：

```markdown
## 6. 验收（本 ADR 改 Accepted 的标准）

> **路径 B 验证模型说明**：本节 checkbox 在 ADR Accepted 时勾选，记录 Sprint Change Proposal v2.5 终审时的 architect/PM **契约级签字** = checkbox 验证语义的等价物。下表第 2-4 项的"Story 37.4 落地后…"等措辞描述的是**契约级前置依赖已确认**（架构决策已 freeze、解锁下游开工），**不是**已物理验证。物理验证（build + test 通过、AppStateTests / LoadHomeUseCase 集成测试存在）由 Story 37.4 实装期 codex review 兜底；若届时发现 ADR-0010 §3 决策有偏差，走"ADR 修订 patch + 改 v2 Accepted"路径（参考 ADR-0008 v2 commit `ec5beb3` 先例）。本说明系 fix-review round 2 codex 担忧"future reviewer 把 ADR §6 当成 false-success signal、Story 37.4 跳过验证"的就地 forward reference；详见 `docs/lessons/2026-04-30-doc-tense-vs-path-b-adr-acceptance.md` 与本 lesson 文件 `docs/lessons/2026-04-30-adr-section-6-path-b-inline-semantics.md`。

- [x] 用户终审通过 Sprint Change Proposal v2
- [x] Story 37.4 落地后跑 `bash iphone/scripts/build.sh --test` 通过
- [x] AppStateTests.swift 含 ≥6 case（hydrate / reset / 各 update mutation）
- [x] LoadHomeUseCase 集成测试改为断言 appState.* 而非 homeViewModel.homeData
- [x] codex 对 Sprint Change Proposal v2 verdict ≥ Accept with revisions
```

变化点：
1. checkbox 列表保留全勾（路径 B 钦定不动）
2. 新增 blockquote 把 round 1 lesson Lesson 2 的核心论证（契约级签字 = 验证等价物 + 下游 codex review 兜底 + ADR-0008 v2 修订路径）就地 inline
3. 显式 cross-link 到 round 1 lesson + 本 lesson —— 任何 reviewer / 未来 Claude 在 ADR §6 现场就能跳到完整论证

不修 §6 checkbox 勾选状态：避免与 Story 37.1 ADR-0009 §6 全勾的先例打架（37.1 codex round 1 已通过同模式，回退 §6 会引发「同类 story 不同处理」工作流不一致）。

### 预防规则（Rule for future Claude）⚡

> **一句话**：未来 Claude 在 **fix-review round N 对路径 B / 决策契约类 finding 判 wontfix 后，下一 round codex review 复发同一 finding** 时，**必须**把 wontfix 论证 inline forward 到原文（ADR § / 文档段）现场作为 blockquote annotation，而非仅靠 lesson 文件 sink；论证就地可见才能终结 codex review 复发循环。
>
> **展开**：
> - codex review 上下文只有 baseline..HEAD diff，**不**含 lesson 文件、不含历史 review 决议、不含 dev story Dev Notes —— 任何不在原文现场的论证 codex 都看不到
> - lesson 文件是「未来 Claude 学习材料」，目标读者 = 未来 Claude；ADR / 文档现场的 inline annotation 是「面向当前 + 未来 reviewer 的语义说明」，目标读者 = 当前 / 未来 codex / 人工 reviewer / 实装者。两者**不是**冗余，目标读者不同
> - **option C 模式**（推荐）：保留路径 B 钦定 checkbox / Status / 状态字段不动，在最近段顶加 blockquote inline annotation，cross-link 到 lesson 文件 —— 同时保护路径 B + 终结 review 复发
> - **判定优先级**：option A（再 wontfix 不改 ADR）只有当原 lesson 已被 codex 看到 / 不在 baseline..HEAD diff 里时才有效；通常情况下 round 2+ 复发就该转 option C
> - **反例**：round 1 把 wontfix 论证只放进 lesson，期望 codex round 2 自动看到 → codex 看不到 → round 2 复发同一 finding → 5 round cap 内反复打架；正确做法是 round 1 fix-review 时就同步 forward 到原文现场
> - **判定边界**：option C 适用于路径 B / 决策契约 / "Accepted 早于实装"模式；如果 finding 是真技术 bug（如代码错误 / 测试缺失），option C 不适用——该改代码就改代码

## Meta: 路径 B + codex review 协同的工作流改进

本 round 暴露的工作流盲点：fix-review 命令的 lesson 归档与 ADR / 文档现场的 inline annotation 是**两个不同 artifact**，不能互相替代。路径 B 类 wontfix 必须**同时**生成两份产物：

1. **lesson 文件**（`docs/lessons/<date>-<slug>.md`）：未来 Claude 学习材料，目标读者 = 跨 story 跨 epic 的未来实施者
2. **ADR / 文档现场 inline annotation**：当前 / 未来 reviewer 就地语义说明，目标读者 = 看到 §X 现场的所有人

round 1 fix-review 漏了 #2，round 2 用 option C 补齐。未来路径 B / "Accepted 早于实装" / 决策契约类 wontfix 应当默认走 option C（即一开始就同时生成 lesson + inline annotation），不要等 round 2 复发再补。

附：本 round 完成后，sprint-status.yaml 中 `37-2-adr-0010-appstate` 状态保持 `review`（fix-review 不动状态，由 epic-loop 决定下一 round 是否复跑 codex review）。
