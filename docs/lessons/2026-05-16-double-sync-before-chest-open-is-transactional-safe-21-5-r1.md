---
date: 2026-05-16
source_review: codex review round 1 of Story 21-5-开箱前主动同步步数（/tmp/epic-loop-review-21-5-r1.md）
story: 21-5-开箱前主动同步步数
commit: <pending>
lesson_count: 1
related_lessons:
  - 2026-05-04-manual-trigger-must-await-in-flight.md
  - 2026-05-04-await-then-recheck-single-flight-gate.md
  - 2026-05-15-driver-sync-init-on-sink-21-1-r2.md
---

# Review Lessons — 2026-05-16 — 开箱前 await in-flight + 自己再 sync 一次的「双 sync」是事务正确性的 safe choice，不是可被消除的浪费（21-5 r1）

## 背景

Story 21-5（开箱前主动同步步数）round 1 codex review。21-5 的全部代码改动只是 DI wire（让 `OpenChestUseCase` 拿到 RootView 持有的唯一 `StepSyncTriggerService` 实例，从而 `execute()` Step 0 的 `await stepSync.triggerManual()` 真正生效）+ 「同步步数中…」loading 提示 + 测试。**没改** `StepSyncTriggerService` 任何一行，**没改** `OpenChestUseCase.execute()` 任何一行（两者都是 21-5 story 钦定红线）。

codex 输出里附带的 `** BUILD FAILED **` 是 codex agentic 环境尝试 arm64 device build 的签名/环境差异（非真实代码缺陷）；主 agent 已用项目 canonical `bash iphone/scripts/build.sh`（simulator destination，CLAUDE.md 钦定）验证 BUILD SUCCEEDED。本轮唯一实质 finding 是 1 条 [P2]。

## 分诊总表

| # | 标题 | Severity | Category | 分诊 | 关联文件 |
|---|---|---|---|---|---|
| 1 | 开箱时若 launch/foreground sync in-flight，会发 2 个 sequential `/steps/sync` 并延迟开箱 | P2 | architecture | **wontfix** | `iphone/PetApp/App/RootView.swift:420-423` → `OpenChestUseCase.swift:65-67` → `StepSyncTriggerService.swift:138-162` |

## Lesson 1: 「开箱前 await in-flight 再自己 sync 一次」的双 sync 是 8.5 round-3 race fix 的钦定语义，不是可优化掉的浪费

- **Severity**: medium (P2)
- **Category**: architecture
- **分诊**: wontfix
- **位置**: `iphone/PetApp/App/RootView.swift:420-423`（DI 注入点）；语义实装在 `StepSyncTriggerService.swift:138-162`（8.5 冻结契约）

### 症状（Symptom）

codex 描述：用户在 launch/foreground step sync 还 in-flight 时点开箱 → 注入的 service 让 `OpenChestUseCase.execute()` 调 `triggerManual()`；`triggerManual()` 会先 `await` in-flight task **然后再起一个新 sync** 才开箱 → "app ready 后立即开箱" 常见路径发 2 个 sequential `/steps/sync` + 延迟开箱。codex 的 framing：「despite the intent here to share the same in-flight gate **rather than duplicate work**」——即认为意图是「只复用 in-flight gate（只等不重发）」，双 sync 是实现偏离意图的浪费。

### 根因（Root cause）

codex 的 framing 与本 story / Story 8.5 的钦定意图**正好相反**，是对 `triggerManual()` 契约的误读：

1. **`triggerManual()` 的契约 = 「我返回时，一次 sync 一定刚跑完且 `appState.currentStepAccount` 是 fresh」**，不是「best-effort 复用 in-flight 结果」。`triggerManual()` 历经 8.5 review **round 3 + round 4 两轮 race fix**：
   - round 3（`2026-05-04-manual-trigger-must-await-in-flight.md`）：曾经的「复用 fire-and-forget 短路 gate」写法（≈ codex 现在建议的「只等不重发」）正是被命中的 **bug** —— caller（开箱）会拿到 stale `currentStepAccount`。fix = manual 必须「先 await in-flight 完，**再自己跑一次新 sync**，再 await 自己完」。
   - round 4（`2026-05-04-await-then-recheck-single-flight-gate.md`）：await 后必须 while-loop re-check single-flight gate（main actor 让出期间 automatic trigger 可能又 spawn 新 task）。
2. **「再自己 sync 一次」是事务正确性所需，不是浪费**：开箱是 MySQL 事务，server 端校验步数余额。一个 in-flight 的 automatic sync 可能在「用户最新步数被记录之前」就启动了；若只 await 它而不重新 sync，开箱用的余额可能 stale —— 这正是 8.5 round-3 fix 掉的 stale-state bug 在事务路径上的复现。
3. **21-5 story 文件 line 381 明确把这个双 sync 命名为「单 in-flight gate 共享的价值」**，并作为 AC1 红线（必须复用 RootView 唯一实例）的核心理由。codex flag 的行为是**已文档化的钦定设计决策**，不是缺陷。

### 修复（Fix）

**不修，理由见「预防规则」**。任何 fix 都比「多一次 idempotent 网络请求」风险更大：

- 选项 A（caller 侧「只等不重发」）需要：① 改 `triggerManual()` 语义 —— 21-5 story line 380 明确红线「本 story 不需要也不允许动 triggerManual」，且会重新引入 8.5 round-3 修掉的 stale-state race；或 ② 给 `StepSyncTriggerService` 新增「wait-only」API —— 侵入 8.5 冻结契约（2 轮 race fix），同样是高风险动一个稳定组件。
- 「多一次 `/steps/sync`」的成本极低：请求 idempotent、失败被 `runSync` catch 静默吞（不阻塞开箱，AC3 不变量）、走 background。**成本 ≪ 重新引入 8.5 race 的成本**。
- 这与 **21-1 over-correction chain 教训**同构（`2026-05-15-driver-sync-init-on-sink-21-1-r2.md` 等）：不要为消除一个「理论上的浪费」去动一个历经多轮 race-fix 的稳定组件，引入更大稳定性风险。

### 预防规则（Rule for future Claude）⚡

> **一句话**：当 review 把「await-to-completion 的公开 async API（如 `triggerManual` / refresh / drain）在 in-flight 时仍跑一次自己的工作」当作「应消除的 duplicate work」时，**禁止**盲从去改成「只等不重发」——先核查该 API 是否历经过 race fix 把「只等」判定为 bug；若是，**wontfix + 引用前次 race-fix lesson + 引用事务/happens-before 正确性依据**。
>
> **展开**：
> - **触发条件**：finding 针对一个「caller 用返回时机当 happens-before barrier」的 async API（开箱前 sync、提交前 flush、读前 refresh），claim 是「in-flight 时不该再做一次」。
> - **核查动作**（按顺序，全做完再下结论）：
>   1. 读该 API 源码顶部注释 + 关联 lesson —— 找「round N race fix」字样。`triggerManual` 注释 line 14-27 明示 round 3/4 两轮 fix + 两份 lesson 路径。
>   2. 读当前 story 文件 —— 找该行为是否被显式命名为「价值 / 钦定折中 / 不变量」。21-5 line 381「单 in-flight gate 共享的价值」即是。
>   3. 判断「只等不重发」是否会让 caller 拿 stale state（事务校验 / 余额 / 时间窗）。会 → review 的 framing 错。
> - **「duplicate work」vs「正确性 barrier」是正交的两件事**：codex 只看到「2 个 `/steps/sync`」的网络层表象，没看到「第 2 个 sync 是开箱事务读 fresh 余额的前提」。性能直觉不能凌驾事务正确性。
> - **idempotent + swallow-on-fail + background 的多一次请求 = 近零成本**；用它换「不动 2 轮 race-fixed 组件」永远划算。
> - **反例 1**：codex 说「应共享 in-flight gate 而非 duplicate work」→ Claude 改 `triggerManual` 为「有 in-flight 就只 await 它、不再自己 sync」→ 重新引入 8.5 round-3 的 caller-gets-stale-state bug，且违反 21-5 红线。
> - **反例 2**：在 caller 侧（`OpenChestUseCase` / `RealHomeViewModel`）加「if in-flight 则跳过 triggerManual」分支 → 同样让开箱用 stale 余额，且把 8.5 单点契约逻辑散到 caller，破坏封装。
> - **类似坑**：任何「N 轮 review 把『naive 复用 gate 短路』判定为 bug、改成『等完再自己跑一次』」的组件，后续 review 必然反复 surface「为什么跑两次」。处置恒为 wontfix + 链式引用 race-fix lesson，**不**改组件。

---

## Meta: review 的「性能浪费」直觉常与「正确性 barrier」冲突，正确性优先

codex 这条 finding 技术描述（2 个 sequential `/steps/sync` + 延迟开箱）是**事实正确**的，但**结论错误** —— 它把一个事务正确性必需的 barrier 误判为可优化掉的 duplicate work。这类 finding 的危险在于：事实部分站得住，容易让人顺着「确实多发了一个请求」的直觉去 fix，从而踩进被 fix 过的 race。

未来遇到「review 事实正确但建议会回退某轮 race fix」时：保留 review 的事实观察（确实多一次请求），驳回其结论（这是必需的），并在 wontfix lesson 里把「事实 vs 结论」拆开写清楚，供未来 Claude 区分「review 看到的现象」与「review 给出的（错误）解释」。
